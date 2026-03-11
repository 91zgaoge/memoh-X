package wecom

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// WebSocket default URL
	DefaultWebsocketURL = "wss://openws.work.weixin.qq.com"

	// Heartbeat interval (30 seconds as per documentation)
	HeartbeatInterval = 30 * time.Second

	// Connection timeout
	ConnectionTimeout = 30 * time.Second

	// Reconnect delay
	ReconnectDelay = 5 * time.Second

	// Max reconnect attempts
	MaxReconnectAttempts = 10

	// Max missed pong before reconnect
	MaxMissedPong = 2

	// Reply ack timeout (10 seconds - balance between reliability and speed)
	// ReplyAckTimeout is the timeout for waiting for ACK from WeCom server.
	// According to WeCom AI Bot SDK, this should be 5000ms.
	ReplyAckTimeout = 5 * time.Second

	// Max reply queue size per req_id
	MaxReplyQueueSize = 100
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
	c.isManualClose = true
	close(c.stopCh)

	c.mu.Lock()
	defer c.mu.Unlock()

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
			if err := c.reconnect(ctx); err != nil {
				c.logger.Error("reconnect failed", slog.Any("error", err))
			}

		case <-heartbeatTicker.C:
			if err := c.sendHeartbeat(); err != nil {
				c.logger.Error("heartbeat failed", slog.Any("error", err))
				go c.triggerReconnect()
			}

		default:
			if !c.IsConnected() {
				time.Sleep(ReconnectDelay)
				continue
			}

			if err := c.readMessage(ctx); err != nil {
				c.logger.Error("read message error", slog.Any("error", err))
				go c.triggerReconnect()
				time.Sleep(ReconnectDelay)
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

	var msg WebsocketMessage
	if err := conn.ReadJSON(&msg); err != nil {
		return fmt.Errorf("read json: %w", err)
	}

	c.logger.Info("websocket message received",
		slog.String("cmd", msg.Cmd),
		slog.String("req_id", msg.Headers.ReqID),
		slog.Int("errcode", msg.ErrCode),
		slog.Int("body_len", len(msg.Body)))

	// Handle based on cmd type
	switch msg.Cmd {
	case CmdMsgCallback:
		if err := c.handler(ctx, &msg); err != nil {
			c.logger.Error("handle message callback failed", slog.Any("error", err))
		}

	case CmdEventCallback:
		if err := c.handler(ctx, &msg); err != nil {
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
		c.logger.Debug("heartbeat ack received")
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

	if frame.ErrCode != 0 {
		c.logger.Warn("reply ack error",
			slog.String("req_id", reqID),
			slog.Int("errcode", frame.ErrCode),
			slog.String("errmsg", frame.ErrMsg))
		if queue != nil && len(queue) > 0 {
			queue[0].Reject(fmt.Errorf("reply failed: %s (code: %d)", frame.ErrMsg, frame.ErrCode))
		}
	} else {
		c.logger.Debug("reply ack received", slog.String("req_id", reqID))
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

// sendHeartbeat sends a ping message
func (c *WebSocketClient) sendHeartbeat() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected || c.conn == nil {
		return fmt.Errorf("not connected")
	}

	// Check missed pongs
	if c.missedPongCount >= MaxMissedPong {
		c.logger.Warn("max missed pongs reached, triggering reconnect")
		go c.triggerReconnect()
		return fmt.Errorf("max missed pongs reached")
	}

	c.missedPongCount++

	msg := WebsocketMessage{
		Cmd: CmdHeartbeat,
		Headers: MessageHeaders{
			ReqID: generateReqID(CmdHeartbeat),
		},
	}

	if err := c.conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("write ping: %w", err)
	}

	c.logger.Debug("heartbeat sent")
	return nil
}

// triggerReconnect triggers a reconnection
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

// SendStream sends a stream message using dual-mode queue:
// - Intermediate updates (finish=false): Fast mode, send without waiting for ACK
// - Final message (finish=true): Ack mode, wait for ACK before sending next
// This ensures both speed and reliability while maintaining message order.
// The cmd parameter specifies the command to use (CmdRespondMsg for replies, CmdSendMsg for proactive sends).
func (c *WebSocketClient) SendStream(ctx context.Context, reqID string, body StreamMsgBody, cmd ...string) error {
	// Determine which command to use (default to CmdRespondMsg for backward compatibility)
	cmdToUse := CmdRespondMsg
	if len(cmd) > 0 && cmd[0] != "" {
		cmdToUse = cmd[0]
	}

	// Determine mode based on finish flag
	waitForAck := body.Stream.Finish // Only wait for ACK on final message

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
			Frame:      frame,
			Resolve:    resolve,
			Reject:     reject,
			WaitForAck: waitForAck,
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
			slog.Bool("finish", body.Stream.Finish),
			slog.Bool("wait_for_ack", waitForAck))

		if len(queue) == 1 {
			go c.processReplyQueue(reqID)
		}
	})
}

// processReplyQueue processes the reply queue for a specific req_id
// Dual-mode processing:
// - Fast mode (WaitForAck=false): Send and immediately continue to next message
// - Ack mode (WaitForAck=true): Send and wait for ACK before continuing
func (c *WebSocketClient) processReplyQueue(reqID string) {
	c.queueMu.Lock()
	queue, exists := c.replyQueues[reqID]
	if !exists || len(queue) == 0 {
		delete(c.replyQueues, reqID)
		c.queueMu.Unlock()
		return
	}
	item := queue[0]
	c.queueMu.Unlock()

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
	if err := conn.WriteJSON(item.Frame); err != nil {
		c.logger.Error("failed to send reply", slog.String("req_id", reqID), slog.Any("error", err))
		item.Reject(fmt.Errorf("write reply: %w", err))
		c.queueMu.Lock()
		if queue, ok := c.replyQueues[reqID]; ok && len(queue) > 0 {
			c.replyQueues[reqID] = queue[1:]
		}
		c.queueMu.Unlock()
		return
	}

	// Dual-mode handling based on WaitForAck flag
	if !item.WaitForAck {
		// Fast mode: Send and immediately continue (for intermediate stream updates)
		c.logger.Debug("reply sent (fast mode), continuing to next", slog.String("req_id", reqID))
		item.Resolve(WebsocketMessage{}) // Resolve immediately without waiting for ACK

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

	// Ack mode: Wait for ACK before continuing (for final messages)
	c.logger.Debug("reply sent, waiting for ack", slog.String("req_id", reqID))

	// Set up timeout
	timer := time.AfterFunc(ReplyAckTimeout, func() {
		c.queueMu.Lock()
		pending, exists := c.pendingAcks[reqID]
		if exists {
			delete(c.pendingAcks, reqID)
			c.queueMu.Unlock()
			c.logger.Warn("reply ack timeout", slog.String("req_id", reqID))
			if pending != nil {
				pending.Reject(fmt.Errorf("reply ack timeout (%v)", ReplyAckTimeout))
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

	resolve := func(WebsocketMessage) {
		close(done)
	}
	reject := func(err error) {
		resultErr = err
		close(done)
	}

	fn(resolve, reject)

	<-done
	return resultErr
}
