package wecom

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// WebSocket default URL
	DefaultWebsocketURL = "wss://openws.work.weixin.qq.com"

	// Heartbeat interval (10 seconds for better connection stability)
	// WeCom SDK 推荐 30 秒，但为了保持连接活跃，使用 10 秒
	HeartbeatInterval = 10 * time.Second

	// Connection timeout
	ConnectionTimeout = 30 * time.Second

	// Reconnect delay
	ReconnectDelay = 5 * time.Second

	// Max reconnect attempts
	MaxReconnectAttempts = 10

	// Max missed pong before reconnect
	MaxMissedPong = 3

	// Reply ack timeout - 缩短为 3 秒以加快响应
	// WeCom 通常响应很快，3 秒足够，超时后继续处理队列
	ReplyAckTimeout = 3 * time.Second

	// Max reply queue size per req_id
	MaxReplyQueueSize = 100

	// Read deadline - 必须大于心跳间隔，给足够的时间接收响应
	ReadDeadlineTimeout = 30 * time.Second
)

// MessageHandler is the callback for processing received messages
type MessageHandler func(ctx context.Context, msg *WebsocketMessage) error

// WebSocketClient manages the WebSocket connection to WeCom
type WebSocketClient struct {
	config     *Config
	logger     *slog.Logger
	handler    MessageHandler

	// Connection state
	conn       *websocket.Conn
	mu         sync.RWMutex
	connected  bool
	subscribed bool

	// Control channels
	stopCh      chan struct{}
	reconnectCh chan struct{}

	// Reconnect state
	reconnectAttempts int
	missedPongCount   int
	isManualClose     bool

	// Credentials
	botID    string
	secret   string

	// Reply queue management (per req_id)
	replyQueues map[string][]*ReplyQueueItem
	pendingAcks map[string]*PendingAck
	queueMu     sync.Mutex
}

// PendingAck represents a pending acknowledgment
type PendingAck struct {
	Resolve func(frame WebsocketMessage)
	Reject  func(reason error)
	Timer   *time.Timer
}

// NewWebSocketClient creates a new WebSocket client
func NewWebSocketClient(config *Config, logger *slog.Logger, handler MessageHandler) *WebSocketClient {
	wsURL := config.WebsocketURL
	if wsURL == "" {
		wsURL = DefaultWebsocketURL
	}

	return &WebSocketClient{
		config:      config,
		logger:      logger.With(slog.String("component", "wecom_ws")),
		handler:     handler,
		stopCh:      make(chan struct{}),
		reconnectCh: make(chan struct{}),
		replyQueues: make(map[string][]*ReplyQueueItem),
		pendingAcks: make(map[string]*PendingAck),
		botID:       config.BotID,
		secret:      config.Secret,
	}
}

// Start initiates the WebSocket connection and starts the message loop
func (c *WebSocketClient) Start(ctx context.Context) error {
	c.logger.Info("starting websocket client",
		slog.String("bot_id", c.botID),
		slog.String("ws_url", c.config.WebsocketURL),
		slog.Bool("group_chat_enabled", c.config.GroupChatEnabled),
		slog.Bool("require_mention", c.config.RequireMention))

	// Retry initial connection with exponential backoff
	var lastErr error
	for attempt := 1; attempt <= MaxReconnectAttempts; attempt++ {
		if err := c.connect(ctx); err != nil {
			lastErr = err
			c.logger.Warn("initial connection attempt failed",
				slog.Int("attempt", attempt),
				slog.Int("max_attempts", MaxReconnectAttempts),
				slog.Any("error", err))

			if attempt < MaxReconnectAttempts {
				// 指数退避算法：1s -> 2s -> 4s -> ... -> 30s 上限
				delay := time.Duration(1<<uint(attempt-1)) * time.Second
				if delay < time.Second {
					delay = time.Second
				}
				if delay > 30*time.Second {
					delay = 30 * time.Second
				}
				c.logger.Info("retrying connection", slog.Duration("delay", delay))
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
					continue
				}
			}
		} else {
			// Connection successful
			go c.run(ctx)
			return nil
		}
	}

	return fmt.Errorf("initial connection failed after %d attempts: %w", MaxReconnectAttempts, lastErr)
}

// Stop closes the WebSocket connection
func (c *WebSocketClient) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Prevent double close
	if c.isManualClose {
		return
	}

	c.isManualClose = true
	close(c.stopCh)

	c.clearPendingMessages("Connection manually closed")

	if c.conn != nil {
		c.conn.Close()
	}
	c.connected = false
	c.subscribed = false
}

// IsConnected returns the connection status
func (c *WebSocketClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && c.subscribed
}

// connect establishes the WebSocket connection
func (c *WebSocketClient) connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// Configure TLS with more permissive settings to handle various server configurations
	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		MinVersion:         tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	dialer := websocket.Dialer{
		TLSClientConfig:    tlsConfig,
		HandshakeTimeout:   ConnectionTimeout,
		ReadBufferSize:     64 * 1024,
		WriteBufferSize:    64 * 1024,
		EnableCompression:  true,
	}

	headers := http.Header{}
	headers.Set("User-Agent", "WeCom-Bot-Client/1.0")

	c.logger.Info("Connecting to WebSocket",
		slog.String("url", c.config.WebsocketURL),
		slog.Duration("timeout", ConnectionTimeout))

	conn, resp, err := dialer.DialContext(ctx, c.config.WebsocketURL, headers)
	if err != nil {
		c.logger.Error("websocket dial failed",
			slog.Any("error", err),
			slog.String("url", c.config.WebsocketURL))
		if resp != nil {
			c.logger.Error("websocket handshake response",
				slog.Int("status", resp.StatusCode),
				slog.Any("headers", resp.Header))
		}
		return fmt.Errorf("dial failed: %w", err)
	}

	c.conn = conn
	c.connected = true
	c.reconnectAttempts = 0
	c.missedPongCount = 0

	// Set up WebSocket keepalive
	// This is critical to detect connection drops early
	conn.SetReadDeadline(time.Now().Add(ReadDeadlineTimeout))
	conn.SetPongHandler(func(string) error {
		c.mu.Lock()
		if c.conn != nil {
			c.conn.SetReadDeadline(time.Now().Add(ReadDeadlineTimeout))
		}
		c.mu.Unlock()
		c.missedPongCount = 0
		c.logger.Debug("websocket pong received")
		return nil
	})

	c.logger.Info("websocket connected")

	// Send subscription request
	if err := c.subscribe(); err != nil {
		conn.Close()
		c.connected = false
		return fmt.Errorf("subscription failed: %w", err)
	}

	return nil
}

// subscribe sends the subscription request
func (c *WebSocketClient) subscribe() error {
	reqID := generateReqID(CmdSubscribe)

	msg := WebsocketMessage{
		Cmd: CmdSubscribe,
		Headers: MessageHeaders{
			ReqID: reqID,
		},
		Body: mustMarshal(SubscribeBody{
			BotID:  c.botID,
			Secret: c.secret,
		}),
	}

	if err := c.conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("write subscribe request: %w", err)
	}

	c.logger.Debug("subscription request sent", slog.String("req_id", reqID))

	// Wait for subscription response
	var resp WebsocketMessage
	if err := c.conn.ReadJSON(&resp); err != nil {
		return fmt.Errorf("read subscribe response: %w", err)
	}

	c.logger.Debug("subscription response received",
		slog.String("req_id", resp.Headers.ReqID),
		slog.Int("errcode", resp.ErrCode),
		slog.String("errmsg", resp.ErrMsg))

	if resp.ErrCode != 0 {
		return fmt.Errorf("subscription failed: %s (code: %d)", resp.ErrMsg, resp.ErrCode)
	}

	c.subscribed = true
	c.logger.Info("subscription successful")
	return nil
}

// run is the main message loop
func (c *WebSocketClient) run(ctx context.Context) {
	heartbeatTicker := time.NewTicker(HeartbeatInterval)
	defer heartbeatTicker.Stop()

	// Start message reader in a separate goroutine
	// This is critical because readMessage blocks and would block the select if in default case
	messageCh := make(chan error, 1)
	readerCtx, cancelReader := context.WithCancel(ctx)
	defer cancelReader()
	go c.messageReader(readerCtx, messageCh)

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("context cancelled, stopping websocket client")
			return

		case <-c.stopCh:
			c.logger.Info("stop signal received")
			return

		case <-c.reconnectCh:
			c.logger.Info("reconnect signal received")
			// Cancel old reader before reconnect
			cancelReader()
			if err := c.reconnect(ctx); err != nil {
				c.logger.Error("reconnect failed", slog.Any("error", err))
			}
			// Create new reader context and restart message reader after reconnect
			readerCtx, cancelReader = context.WithCancel(ctx)
			go c.messageReader(readerCtx, messageCh)

		case <-heartbeatTicker.C:
			if err := c.sendHeartbeat(); err != nil {
				c.logger.Error("heartbeat failed", slog.Any("error", err))
				go c.triggerReconnect()
			}

		case err := <-messageCh:
			if err != nil {
				c.logger.Error("read message error", slog.Any("error", err))
				// Cancel old reader before triggering reconnect
				cancelReader()
				go c.triggerReconnect()
				// Create new reader context and restart message reader after error
				readerCtx, cancelReader = context.WithCancel(ctx)
				go c.messageReader(readerCtx, messageCh)
			}
		}
	}
}

// messageReader continuously reads messages and sends errors to the channel
func (c *WebSocketClient) messageReader(ctx context.Context, ch chan<- error) {
	for {
		if !c.IsConnected() {
			select {
			case <-time.After(ReconnectDelay):
				continue
			case <-ctx.Done():
				return
			}
		}

		if err := c.readMessage(ctx); err != nil {
			// Check if this is a connection closed error (normal shutdown)
			errStr := err.Error()
			if strings.Contains(errStr, "use of closed network connection") ||
				strings.Contains(errStr, "websocket: close sent") {
				c.logger.Debug("message reader stopping due to closed connection")
				return
			}

			select {
			case ch <- err:
				return // Exit after sending error to trigger reconnect
			case <-ctx.Done():
				return
			}
		}
	}
}

// readMessage reads and processes a single message
func (c *WebSocketClient) readMessage(ctx context.Context) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("connection is nil")
	}

	// Reset read deadline - give enough time to receive next message
	// This detects connection drops while allowing for idle periods
	conn.SetReadDeadline(time.Now().Add(ReadDeadlineTimeout))

	var msg WebsocketMessage
	if err := conn.ReadJSON(&msg); err != nil {
		return fmt.Errorf("read json: %w", err)
	}

	// Reset read deadline after successful read
	conn.SetReadDeadline(time.Now().Add(ReadDeadlineTimeout))

	c.logger.Info("websocket message received",
		slog.String("cmd", msg.Cmd),
		slog.String("req_id", msg.Headers.ReqID),
		slog.Int("errcode", msg.ErrCode),
		slog.Int("body_len", len(msg.Body)))

	// Handle based on cmd type
	// CRITICAL: Use context.WithoutCancel to ensure message processing
	// continues even if the WebSocket connection is closed/reconnected.
	// The handler should not be affected by connection lifecycle.
	handlerCtx := context.WithoutCancel(ctx)
	switch msg.Cmd {
	case CmdMsgCallback:
		if err := c.handler(handlerCtx, &msg); err != nil {
			c.logger.Error("handle message callback failed", slog.Any("error", err))
		}

	case CmdEventCallback:
		if err := c.handler(handlerCtx, &msg); err != nil {
			c.logger.Error("handle event callback failed", slog.Any("error", err))
		}

	case "":
		// No cmd: could be ack for reply, heartbeat, or subscription
		c.handleAck(msg)

	case CmdPong:
		c.missedPongCount = 0
		c.logger.Debug("pong received")

	default:
		c.logger.Debug("unknown command", slog.String("cmd", msg.Cmd))
	}

	return nil
}

// handleAck handles acknowledgment frames
func (c *WebSocketClient) handleAck(frame WebsocketMessage) {
	reqID := frame.Headers.ReqID

	// Check if it's a reply ack
	c.queueMu.Lock()
	pending, exists := c.pendingAcks[reqID]
	c.queueMu.Unlock()

	if exists {
		c.handleReplyAck(reqID, frame, pending)
		return
	}

	// Heartbeat ack
	if frame.ErrCode == 0 {
		c.missedPongCount = 0
		c.logger.Debug("heartbeat ack received", slog.String("req_id", reqID))
	} else {
		c.logger.Warn("heartbeat ack error", slog.Int("errcode", frame.ErrCode), slog.String("errmsg", frame.ErrMsg))
	}
}

// handleReplyAck handles reply acknowledgment
func (c *WebSocketClient) handleReplyAck(reqID string, frame WebsocketMessage, pending *PendingAck) {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()

	// Clear timer
	if pending.Timer != nil {
		pending.Timer.Stop()
	}
	delete(c.pendingAcks, reqID)

	queue := c.replyQueues[reqID]

	ackReceiveTime := time.Now()
	if frame.ErrCode != 0 {
		c.logger.Warn("[PERF] reply ack error",
			slog.String("req_id", reqID),
			slog.Int("errcode", frame.ErrCode),
			slog.String("errmsg", frame.ErrMsg),
			slog.Time("ack_time", ackReceiveTime))
		if queue != nil && len(queue) > 0 {
			queue[0].Reject(fmt.Errorf("reply failed: %s (code: %d)", frame.ErrMsg, frame.ErrCode))
		}
	} else {
		c.logger.Info("[PERF] reply ack received",
			slog.String("req_id", reqID),
			slog.Time("ack_time", ackReceiveTime))
		if queue != nil && len(queue) > 0 {
			queue[0].Resolve(frame)
		}
	}

	// Continue processing queue
	if queue != nil {
		if len(queue) > 0 {
			c.replyQueues[reqID] = queue[1:]
		}
		if len(c.replyQueues[reqID]) > 0 {
			go c.processReplyQueue(reqID)
		} else {
			delete(c.replyQueues, reqID)
		}
	}
}

// sendHeartbeat sends websocket ping and application heartbeat
func (c *WebSocketClient) sendHeartbeat() error {
	c.mu.RLock()
	conn := c.conn
	connected := c.connected
	c.mu.RUnlock()

	if !connected || conn == nil {
		return fmt.Errorf("not connected")
	}

	// Check missed pongs
	if c.missedPongCount >= MaxMissedPong {
		c.logger.Warn("max missed pongs reached, triggering reconnect")
		go c.triggerReconnect()
		return fmt.Errorf("max missed pongs reached")
	}

	// Send WebSocket native ping frame first
	// This triggers a pong response which helps detect connection health
	if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)); err != nil {
		c.logger.Warn("websocket ping failed", slog.Any("error", err))
		// Continue to application heartbeat even if websocket ping fails
	}

	c.missedPongCount++

	// Also send application-level heartbeat
	c.mu.RLock()
	if c.connected && c.conn != nil {
		msg := WebsocketMessage{
			Cmd: CmdHeartbeat,
			Headers: MessageHeaders{
				ReqID: generateReqID(CmdHeartbeat),
			},
		}
		if err := c.conn.WriteJSON(msg); err != nil {
			c.mu.RUnlock()
			return fmt.Errorf("write ping: %w", err)
		}
	}
	c.mu.RUnlock()

	c.logger.Debug("heartbeat sent")
	return nil
}

// sendFrame sends a frame directly without queuing or waiting for ACK.
// This is used for time-critical messages like the "thinking" reply
// that must be sent within the 5-second ACK window.
func (c *WebSocketClient) sendFrame(frame WebsocketMessage) error {
	c.mu.RLock()
	conn := c.conn
	connected := c.connected
	c.mu.RUnlock()

	if !connected || conn == nil {
		return fmt.Errorf("websocket not connected")
	}

	if err := conn.WriteJSON(frame); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	c.logger.Debug("frame sent directly",
		slog.String("req_id", frame.Headers.ReqID),
		slog.String("cmd", frame.Cmd))
	return nil
}

// triggerReconnect triggers a reconnection with a small delay
// to avoid rapid reconnection loops
func (c *WebSocketClient) triggerReconnect() {
	select {
	case c.reconnectCh <- struct{}{}:
	default:
	}
}

// reconnect attempts to reconnect with exponential backoff
func (c *WebSocketClient) reconnect(ctx context.Context) error {
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connected = false
	c.subscribed = false
	c.mu.Unlock()

	// Clear pending messages
	c.clearPendingMessages("Reconnecting")

	for {
		c.reconnectAttempts++
		if c.reconnectAttempts > MaxReconnectAttempts {
			return fmt.Errorf("max reconnect attempts (%d) exceeded", MaxReconnectAttempts)
		}

		c.logger.Info("attempting to reconnect", slog.Int("attempt", c.reconnectAttempts))

		if err := c.connect(ctx); err != nil {
			c.logger.Error("reconnect attempt failed",
				slog.Int("attempt", c.reconnectAttempts),
				slog.Any("error", err))
			// 指数退避算法：1s -> 2s -> 4s -> ... -> 30s 上限
			delay := time.Duration(1<<uint(c.reconnectAttempts-1)) * time.Second
			if delay < time.Second {
				delay = time.Second
			}
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
			c.logger.Info("waiting before next reconnect attempt", slog.Duration("delay", delay))
			time.Sleep(delay)
			continue
		}

		c.logger.Info("reconnect successful")
		return nil
	}
}

// SendReply sends a reply message via WebSocket with queue management
func (c *WebSocketClient) SendReply(ctx context.Context, reqID string, body interface{}, cmd string) error {
	if cmd == "" {
		cmd = CmdRespondMsg
	}

	return newPromise(func(resolve func(WebsocketMessage), reject func(error)) {
		frame := WebsocketMessage{
			Cmd:     cmd,
			Headers: MessageHeaders{ReqID: reqID},
		}

		// Marshal body based on type
		var bodyBytes []byte
		var err error
		switch b := body.(type) {
		case StreamMsgBody:
			bodyBytes, err = json.Marshal(b)
		case RespondMsgBody:
			bodyBytes, err = json.Marshal(b)
		case map[string]interface{}:
			bodyBytes, err = json.Marshal(b)
		default:
			bodyBytes, err = json.Marshal(b)
		}

		if err != nil {
			reject(fmt.Errorf("marshal body failed: %w", err))
			return
		}
		frame.Body = bodyBytes

		item := &ReplyQueueItem{
			Frame:   frame,
			Resolve: resolve,
			Reject:  reject,
		}

		c.queueMu.Lock()
		defer c.queueMu.Unlock()

		// Get or create queue
		queue, exists := c.replyQueues[reqID]
		if !exists {
			queue = []*ReplyQueueItem{}
		}

		// Check queue size
		if len(queue) >= MaxReplyQueueSize {
			reject(fmt.Errorf("reply queue for req_id %s exceeds max size (%d)", reqID, MaxReplyQueueSize))
			return
		}

		// Add to queue
		queue = append(queue, item)
		c.replyQueues[reqID] = queue

		// If queue has only this item, start processing
		if len(queue) == 1 {
			go c.processReplyQueue(reqID)
		}
	})
}

// SendStream sends a stream message.
// All messages are sent serially via queue, waiting for ACK before sending next.
// This ensures message order and delivery reliability.
// The cmd parameter specifies the command to use (CmdRespondMsg for replies, CmdSendMsg for proactive sends).
// Use this for final messages (finish=true) to ensure delivery.
func (c *WebSocketClient) SendStream(ctx context.Context, reqID string, body StreamMsgBody, cmd ...string) error {
	// Determine which command to use (default to CmdRespondMsg for backward compatibility)
	cmdToUse := CmdRespondMsg
	if len(cmd) > 0 && cmd[0] != "" {
		cmdToUse = cmd[0]
	}

	return newPromise(func(resolve func(WebsocketMessage), reject func(error)) {
		frame := WebsocketMessage{
			Cmd:     cmdToUse,
			Headers: MessageHeaders{ReqID: reqID},
		}

		bodyBytes, err := json.Marshal(body)
		if err != nil {
			reject(fmt.Errorf("marshal stream body failed: %w", err))
			return
		}
		frame.Body = bodyBytes

		item := &ReplyQueueItem{
			Frame:   frame,
			Resolve: resolve,
			Reject:  reject,
		}

		c.queueMu.Lock()
		defer c.queueMu.Unlock()

		queue, exists := c.replyQueues[reqID]
		if !exists {
			queue = []*ReplyQueueItem{}
		}

		if len(queue) >= MaxReplyQueueSize {
			reject(fmt.Errorf("reply queue for req_id %s exceeds max size (%d)", reqID, MaxReplyQueueSize))
			return
		}

		queue = append(queue, item)
		c.replyQueues[reqID] = queue

		c.logger.Debug("stream message queued",
			slog.String("req_id", reqID),
			slog.Int("queue_len", len(queue)),
			slog.Bool("finish", body.Stream.Finish))

		if len(queue) == 1 {
			go c.processReplyQueue(reqID)
		}
	})
}

// processReplyQueue processes the reply queue for a specific req_id
// All messages wait for ACK before continuing to ensure delivery and order.
func (c *WebSocketClient) processReplyQueue(reqID string) {
	// [PERF] 记录队列处理开始时间
	processStartTime := time.Now()

	c.queueMu.Lock()
	queue, exists := c.replyQueues[reqID]
	if !exists || len(queue) == 0 {
		delete(c.replyQueues, reqID)
		c.queueMu.Unlock()
		return
	}
	item := queue[0]
	queueLen := len(queue)
	c.queueMu.Unlock()

	c.logger.Info("[PERF] processing reply queue",
		slog.String("req_id", reqID),
		slog.Int("queue_len", queueLen))

	c.mu.RLock()
	conn := c.conn
	connected := c.connected
	c.mu.RUnlock()

	if !connected || conn == nil {
		item.Reject(fmt.Errorf("websocket not connected"))
		c.queueMu.Lock()
		if queue, ok := c.replyQueues[reqID]; ok && len(queue) > 0 {
			c.replyQueues[reqID] = queue[1:]
			if len(c.replyQueues[reqID]) > 0 {
				go c.processReplyQueue(reqID)
			}
		}
		c.queueMu.Unlock()
		return
	}

	// Send the frame
	sendTime := time.Now()
	if err := conn.WriteJSON(item.Frame); err != nil {
		c.logger.Error("failed to send reply", slog.String("req_id", reqID), slog.Any("error", err))
		item.Reject(fmt.Errorf("write reply: %w", err))
		c.queueMu.Lock()
		if queue, ok := c.replyQueues[reqID]; ok && len(queue) > 0 {
			c.replyQueues[reqID] = queue[1:]
			if len(c.replyQueues[reqID]) > 0 {
				go c.processReplyQueue(reqID)
			} else {
				delete(c.replyQueues, reqID)
			}
		}
		c.queueMu.Unlock()
		return
	}
	sendElapsed := time.Since(sendTime)

	c.logger.Info("[PERF] reply sent, waiting for ack",
		slog.String("req_id", reqID),
		slog.String("cmd", item.Frame.Cmd),
		slog.Duration("send_elapsed", sendElapsed),
		slog.Duration("queue_process_time", time.Since(processStartTime)))

	// Set up timeout
	timer := time.AfterFunc(ReplyAckTimeout, func() {
		c.queueMu.Lock()
		pending, exists := c.pendingAcks[reqID]
		if exists {
			delete(c.pendingAcks, reqID)
			c.queueMu.Unlock()
			c.logger.Warn("reply ack timeout, assuming message delivered", slog.String("req_id", reqID))
			// CRITICAL: Don't reject on ACK timeout - message may have been delivered
			// Just continue processing queue. This prevents stream interruption.
			if pending != nil {
				pending.Resolve(WebsocketMessage{}) // Resolve with empty frame to continue
			}
			// Continue processing queue
			c.queueMu.Lock()
			if queue, ok := c.replyQueues[reqID]; ok && len(queue) > 0 {
				c.replyQueues[reqID] = queue[1:]
				if len(c.replyQueues[reqID]) > 0 {
					go c.processReplyQueue(reqID)
				} else {
					delete(c.replyQueues, reqID)
				}
			}
			c.queueMu.Unlock()
		} else {
			c.queueMu.Unlock()
		}
	})

	c.queueMu.Lock()
	c.pendingAcks[reqID] = &PendingAck{
		Resolve: item.Resolve,
		Reject:  item.Reject,
		Timer:   timer,
	}
	c.queueMu.Unlock()
}

// clearPendingMessages clears all pending messages and queues
func (c *WebSocketClient) clearPendingMessages(reason string) {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()

	// Clear pending acks
	for reqID, pending := range c.pendingAcks {
		if pending.Timer != nil {
			pending.Timer.Stop()
		}
		pending.Reject(fmt.Errorf("%s, reply cancelled", reason))
		delete(c.pendingAcks, reqID)
	}

	// Clear queues
	for reqID, queue := range c.replyQueues {
		for _, item := range queue {
			item.Reject(fmt.Errorf("%s, reply cancelled", reason))
		}
		delete(c.replyQueues, reqID)
	}
}

// Helper functions

func mustMarshal(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func generateReqID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

// Promise helper
type promiseFunc func(resolve func(WebsocketMessage), reject func(error))

func newPromise(fn promiseFunc) error {
	done := make(chan struct{})
	var resultErr error
	var once sync.Once

	resolve := func(WebsocketMessage) {
		once.Do(func() { close(done) })
	}
	reject := func(err error) {
		once.Do(func() {
			resultErr = err
			close(done)
		})
	}

	fn(resolve, reject)

	<-done
	return resultErr
}
