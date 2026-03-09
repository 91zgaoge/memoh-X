package wecom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Kxiandaoyan/Memoh-v2/internal/channel"
	"github.com/Kxiandaoyan/Memoh-v2/internal/message"
)

// newChatCommands contains keywords that trigger a new conversation (clear history)
var newChatCommands = []string{
	"新对话", "新聊天", "新会话", "重新开始",
	"/new", "/clear", "/reset", "/start",
	"清空", "清空对话", "清空历史", "清除历史",
}

// isNewChatCommand checks if the content is a new chat command
func isNewChatCommand(content string) bool {
	trimmed := strings.TrimSpace(content)
	for _, cmd := range newChatCommands {
		if strings.EqualFold(trimmed, cmd) {
			return true
		}
	}
	return false
}

// Adapter implements the WeCom channel adapter
type Adapter struct {
	logger         *slog.Logger
	clients        map[string]*WebSocketClient // BotID -> Client
	mu             sync.RWMutex
	httpClient     *http.Client
	messageService message.Service // Optional: for clearing history on new chat
}

// NewWeComAdapter creates a new WeCom adapter (alias for NewAdapter for compatibility)
func NewWeComAdapter(logger *slog.Logger) *Adapter {
	return NewAdapter(logger)
}

// SetMessageService sets the message service for history management
func (a *Adapter) SetMessageService(svc message.Service) {
	a.messageService = svc
}

// NewAdapter creates a new WeCom adapter
func NewAdapter(logger *slog.Logger) *Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{
		logger:  logger.With(slog.String("adapter", "wecom")),
		clients: make(map[string]*WebSocketClient),
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

// Type returns the channel type
func (a *Adapter) Type() channel.ChannelType {
	return Type
}

// Descriptor returns the channel descriptor
func (a *Adapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        Type,
		DisplayName: "WeCom",
		Capabilities: channel.ChannelCapabilities{
			Text:           true,
			Markdown:       true,
			Attachments:    true,
			Media:          true,
			Streaming:      true,
			BlockStreaming: false,
			Reply:          true,
			ChatTypes:      []string{"single", "group"},
		},
	}
}

// Connect establishes a connection to WeCom WebSocket
func (a *Adapter) Connect(ctx context.Context, cfg channel.ChannelConfig, handler channel.InboundHandler) (channel.Connection, error) {
	config, err := ParseConfig(cfg.Credentials)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Create WebSocket client with proper handler
	wsClient := NewWebSocketClient(config, a.logger, func(ctx context.Context, msg *WebsocketMessage) error {
		return a.handleWebSocketMessage(ctx, cfg, config, handler, msg)
	})

	// Start WebSocket connection
	if err := wsClient.Start(ctx); err != nil {
		return nil, fmt.Errorf("start websocket: %w", err)
	}

	// Store client
	a.mu.Lock()
	a.clients[cfg.BotID] = wsClient
	a.mu.Unlock()

	// Create connection
	conn := &connection{
		configID:    cfg.ID,
		botID:       cfg.BotID,
		channelType: Type,
		stop:        wsClient.Stop,
		client:      wsClient,
	}
	conn.running.Store(true)

	a.logger.Info("WeCom connection established",
		slog.String("bot_id", cfg.BotID),
		slog.String("config_id", cfg.ID))

	return conn, nil
}

// OpenStream creates an outbound stream for sending messages
func (a *Adapter) OpenStream(ctx context.Context, cfg channel.ChannelConfig, target string, opts channel.StreamOptions) (channel.OutboundStream, error) {
	// Get WebSocket client for this bot
	a.mu.RLock()
	wsClient, exists := a.clients[cfg.BotID]
	a.mu.RUnlock()

	if !exists || !wsClient.IsConnected() {
		return nil, fmt.Errorf("websocket client not connected for bot %s", cfg.BotID)
	}

	// Extract req_id from options metadata
	reqID := ""
	chatID := ""
	userID := ""
	if opts.Metadata != nil {
		if v, ok := opts.Metadata["req_id"].(string); ok {
			reqID = v
		}
		if v, ok := opts.Metadata["chat_id"].(string); ok {
			chatID = v
		}
		if v, ok := opts.Metadata["user_id"].(string); ok {
			userID = v
		}
	}

	if reqID == "" {
		return nil, fmt.Errorf("req_id is required for WeCom responses")
	}

	// Parse target for chat info
	if strings.HasPrefix(target, "chat_id:") {
		chatID = strings.TrimPrefix(target, "chat_id:")
	} else if strings.HasPrefix(target, "user_id:") {
		userID = strings.TrimPrefix(target, "user_id:")
	}

	return NewOutboundStream(a, cfg, wsClient, reqID, chatID, userID, a.logger), nil
}

// Send sends a message directly (non-streaming)
func (a *Adapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.OutboundMessage) error {
	// Get WebSocket client for this bot
	a.mu.RLock()
	wsClient, exists := a.clients[cfg.BotID]
	a.mu.RUnlock()

	if !exists || !wsClient.IsConnected() {
		return fmt.Errorf("websocket client not connected for bot %s", cfg.BotID)
	}

	// Extract req_id from message metadata
	reqID := ""
	if msg.Message.Metadata != nil {
		if v, ok := msg.Message.Metadata["req_id"].(string); ok {
			reqID = v
		}
	}

	if reqID == "" {
		return fmt.Errorf("req_id is required for WeCom responses")
	}

	// Build response content
	content := msg.Message.Text
	if content == "" && len(msg.Message.Attachments) > 0 {
		content = "[附件消息]"
	}

	// Send as stream with finish=true
	body := StreamMsgBody{
		MsgType: MsgTypeStream,
		Stream: StreamResponse{
			ID:      generateStreamID(),
			Finish:  true,
			Content: content,
		},
	}

	return wsClient.SendReply(ctx, reqID, body, CmdRespondMsg)
}

// handleWebSocketMessage handles incoming WebSocket messages
func (a *Adapter) handleWebSocketMessage(ctx context.Context, cfg channel.ChannelConfig, config *Config, handler channel.InboundHandler, wsMsg *WebsocketMessage) error {
	a.logger.Info("handleWebSocketMessage called",
		slog.String("cmd", wsMsg.Cmd),
		slog.String("req_id", wsMsg.Headers.ReqID))

	switch wsMsg.Cmd {
	case CmdMsgCallback:
		return a.handleMessageCallback(ctx, cfg, config, handler, wsMsg)
	case CmdEventCallback:
		return a.handleEventCallback(ctx, cfg, config, handler, wsMsg)
	default:
		a.logger.Debug("unknown message command", slog.String("cmd", wsMsg.Cmd))
	}
	return nil
}

// handleMessageCallback processes message callbacks
func (a *Adapter) handleMessageCallback(ctx context.Context, cfg channel.ChannelConfig, config *Config, handler channel.InboundHandler, wsMsg *WebsocketMessage) error {
	var body MsgCallbackBody
	if err := json.Unmarshal(wsMsg.Body, &body); err != nil {
		return fmt.Errorf("unmarshal message body: %w", err)
	}

	// Get content preview based on message type
	contentPreview := ""
	switch body.MsgType {
	case MsgTypeText:
		if body.Text != nil {
			contentPreview = body.Text.Content
		}
	case MsgTypeImage:
		contentPreview = "[图片]"
	case MsgTypeFile:
		contentPreview = "[文件]"
	case MsgTypeVoice:
		if body.Voice != nil {
			contentPreview = "[语音] " + body.Voice.Content
		}
	case MsgTypeMixed:
		contentPreview = "[图文混排]"
	}

	a.logger.Info("message received",
		slog.String("msg_id", body.MsgID),
		slog.String("msg_type", body.MsgType),
		slog.String("from", body.From.UserID),
		slog.String("chat_type", body.ChatType),
		slog.String("req_id", wsMsg.Headers.ReqID),
		slog.String("content_preview", truncateString(contentPreview, 50)))

	// Determine reply target based on chat type
	replyTarget := ""
	if body.ChatType == "group" {
		replyTarget = "chat_id:" + body.ChatID
	} else {
		replyTarget = "user_id:" + body.From.UserID
	}

	// Convert to internal message
	msg := channel.InboundMessage{
		Channel: Type,
		BotID:   cfg.BotID,
		Sender: channel.Identity{
			SubjectID: body.From.UserID,
		},
		Conversation: channel.Conversation{
			ID:   body.ChatID,
			Type: body.ChatType,
			Metadata: map[string]any{
				"chattype": body.ChatType,
				"chat_id":  body.ChatID,
			},
		},
		ReplyTarget: replyTarget,
		Message: channel.Message{
			ID: body.MsgID,
			Metadata: map[string]any{
				"req_id":   wsMsg.Headers.ReqID,
				"chat_id":  body.ChatID,
				"user_id":  body.From.UserID,
				"chattype": body.ChatType,
			},
		},
		ReceivedAt: time.Now(),
		Metadata: map[string]any{
			"req_id":   wsMsg.Headers.ReqID,
			"chat_id":  body.ChatID,
			"user_id":  body.From.UserID,
			"chattype": body.ChatType,
		},
	}

	// Process based on message type
	switch body.MsgType {
	case MsgTypeText:
		if body.Text != nil {
			originalContent := body.Text.Content
			content := originalContent

			// Debug: log group chat message content
			if body.ChatType == "group" {
				a.logger.Info("group text message received",
					slog.String("content", originalContent),
					slog.Bool("should_trigger", config.ShouldTriggerGroupResponse(originalContent)))
			}

			// Check if should trigger response in group chat (before removing mention prefix)
			shouldTrigger := body.ChatType == "single" || config.ShouldTriggerGroupResponse(originalContent)

			if body.ChatType == "group" {
				content = config.ExtractGroupMessageContent(content)
			}

			// Check for new chat command
			if isNewChatCommand(content) {
				return a.handleNewChatCommand(ctx, cfg, wsMsg, body)
			}

			msg.Message.Text = content
			msg.Message.Format = channel.MessageFormatPlain

			// Add is_mentioned metadata for group chats
			if body.ChatType == "group" {
				msg.Metadata["is_mentioned"] = config.ShouldTriggerGroupResponse(originalContent)
			}

			// Check if should trigger response
			if shouldTrigger {
				return handler(ctx, cfg, msg)
			}
			a.logger.Info("skipping group message (no mention)")
			return nil
		}

	case MsgTypeImage:
		if body.Image != nil {
			// For group chats, mark as mentioned (users typically expect response when sending images)
			if body.ChatType == "group" {
				msg.Metadata["is_mentioned"] = true
			}

			// Download and decrypt image
			data, err := a.downloadAndDecrypt(body.Image.URL, body.Image.AESKey)
			if err != nil {
				a.logger.Error("failed to download/decrypt image", slog.Any("error", err))
				// Continue with URL only
				msg.Message.Attachments = append(msg.Message.Attachments, channel.Attachment{
					Type: channel.AttachmentImage,
					URL:  body.Image.URL,
					Metadata: map[string]any{
						"aeskey": body.Image.AESKey,
					},
				})
			} else {
				msg.Message.Attachments = append(msg.Message.Attachments, channel.Attachment{
					Type:     channel.AttachmentImage,
					URL:      body.Image.URL,
					Data:     data,
					Metadata: map[string]any{
						"aeskey": body.Image.AESKey,
						"size":   len(data),
					},
				})
			}
			return handler(ctx, cfg, msg)
		}

	case MsgTypeFile:
		if body.File != nil {
			// For group chats, mark as mentioned (users typically expect response when sending files)
			if body.ChatType == "group" {
				msg.Metadata["is_mentioned"] = true
			}

			// Download and decrypt file
			data, err := a.downloadAndDecrypt(body.File.URL, body.File.AESKey)
			if err != nil {
				a.logger.Error("failed to download/decrypt file", slog.Any("error", err))
				msg.Message.Attachments = append(msg.Message.Attachments, channel.Attachment{
					Type: channel.AttachmentFile,
					URL:  body.File.URL,
					Name: body.File.FileName,
					Metadata: map[string]any{
						"aeskey": body.File.AESKey,
					},
				})
			} else {
				msg.Message.Attachments = append(msg.Message.Attachments, channel.Attachment{
					Type:     channel.AttachmentFile,
					URL:      body.File.URL,
					Name:     body.File.FileName,
					Data:     data,
					Metadata: map[string]any{
						"aeskey": body.File.AESKey,
						"size":   len(data),
					},
				})
			}
			return handler(ctx, cfg, msg)
		}

	case MsgTypeVoice:
		if body.Voice != nil {
			// For group chats, mark as mentioned
			if body.ChatType == "group" {
				msg.Metadata["is_mentioned"] = true
			}

			// Voice content is the transcribed text from WeCom
			msg.Message.Text = body.Voice.Content
			msg.Message.Format = channel.MessageFormatPlain
			return handler(ctx, cfg, msg)
		}

	case MsgTypeMixed:
		if body.Mixed != nil {
			return a.handleMixedContent(ctx, cfg, config, handler, wsMsg.Headers.ReqID, body)
		}

	default:
		a.logger.Warn("unknown message type", slog.String("msg_type", body.MsgType))
	}

	return nil
}

// handleNewChatCommand handles the "new chat" command to clear conversation history
func (a *Adapter) handleNewChatCommand(ctx context.Context, cfg channel.ChannelConfig, wsMsg *WebsocketMessage, body MsgCallbackBody) error {
	a.logger.Info("new chat command received",
		slog.String("user_id", body.From.UserID),
		slog.String("bot_id", cfg.BotID))

	// Clear history if message service is available
	if a.messageService != nil {
		if err := a.messageService.DeleteByBot(ctx, cfg.BotID); err != nil {
			a.logger.Error("failed to clear history", slog.Any("error", err))
			// Continue to send confirmation even if delete failed
		}
	}

	// Get WebSocket client
	wsClient := a.getWebSocketClient(cfg.BotID)
	if wsClient == nil {
		return fmt.Errorf("websocket client not connected")
	}

	// Send confirmation message
	confirmBody := StreamMsgBody{
		MsgType: MsgTypeStream,
		Stream: StreamResponse{
			ID:      generateStreamID(),
			Finish:  true,
			Content: "✅ 已开启新对话\n\n历史上下文已清空，让我们开始新的对话吧！",
		},
	}

	if err := wsClient.SendReply(ctx, wsMsg.Headers.ReqID, confirmBody, CmdRespondMsg); err != nil {
		a.logger.Error("failed to send new chat confirmation", slog.Any("error", err))
		return err
	}

	a.logger.Info("new chat confirmation sent",
		slog.String("user_id", body.From.UserID),
		slog.String("bot_id", cfg.BotID))

	return nil
}

// handleEventCallback processes event callbacks
func (a *Adapter) handleEventCallback(ctx context.Context, cfg channel.ChannelConfig, config *Config, handler channel.InboundHandler, wsMsg *WebsocketMessage) error {
	var body MsgCallbackBody
	if err := json.Unmarshal(wsMsg.Body, &body); err != nil {
		return fmt.Errorf("unmarshal event body: %w", err)
	}

	if body.Event == nil {
		return nil
	}

	eventType := body.Event.EventType
	a.logger.Info("event received", slog.String("event_type", eventType))

	switch eventType {
	case EventTypeEnterChat:
		// Send welcome message
		if wsClient := a.getWebSocketClient(cfg.BotID); wsClient != nil {
			welcomeBody := StreamMsgBody{
				MsgType: MsgTypeStream,
				Stream: StreamResponse{
					ID:      generateStreamID(),
					Finish:  true,
					Content: config.GetWelcomeMessage(),
				},
			}
			if err := wsClient.SendReply(ctx, wsMsg.Headers.ReqID, welcomeBody, CmdRespondWelcome); err != nil {
				a.logger.Error("failed to send welcome message", slog.Any("error", err))
			}
		}

	case EventTypeDisconnected:
		a.logger.Info("received disconnected event")

	default:
		a.logger.Debug("unhandled event type", slog.String("event_type", eventType))
	}

	return nil
}

// handleMixedContent handles mixed content (text + image) messages
func (a *Adapter) handleMixedContent(ctx context.Context, cfg channel.ChannelConfig, config *Config, handler channel.InboundHandler, reqID string, body MsgCallbackBody) error {
	replyTarget := ""
	if body.ChatType == "group" {
		replyTarget = "chat_id:" + body.ChatID
	} else {
		replyTarget = "user_id:" + body.From.UserID
	}

	// Build message with all content
	var textContent strings.Builder
	var attachments []channel.Attachment

	for _, item := range body.Mixed.MsgItem {
		switch item.MsgType {
		case MsgTypeText:
			if item.Text != nil {
				textContent.WriteString(item.Text.Content)
			}
		case MsgTypeImage:
			if item.Image != nil {
				// Download and decrypt image
				data, err := a.downloadAndDecrypt(item.Image.URL, item.Image.AESKey)
				if err != nil {
					a.logger.Error("failed to download/decrypt mixed image", slog.Any("error", err))
					attachments = append(attachments, channel.Attachment{
						Type: channel.AttachmentImage,
						URL:  item.Image.URL,
						Metadata: map[string]any{
							"aeskey": item.Image.AESKey,
						},
					})
				} else {
					attachments = append(attachments, channel.Attachment{
						Type:     channel.AttachmentImage,
						URL:      item.Image.URL,
						Data:     data,
						Metadata: map[string]any{
							"aeskey": item.Image.AESKey,
							"size":   len(data),
						},
					})
				}
			}
		}
	}

	content := textContent.String()
	if body.ChatType == "group" {
		content = config.ExtractGroupMessageContent(content)
	}

	msg := channel.InboundMessage{
		Channel: Type,
		BotID:   cfg.BotID,
		Sender: channel.Identity{
			SubjectID: body.From.UserID,
		},
		Conversation: channel.Conversation{
			ID:   body.ChatID,
			Type: body.ChatType,
			Metadata: map[string]any{
				"chattype": body.ChatType,
				"chat_id":  body.ChatID,
			},
		},
		ReplyTarget: replyTarget,
		Message: channel.Message{
			ID:          body.MsgID,
			Text:        content,
			Format:      channel.MessageFormatPlain,
			Attachments: attachments,
			Metadata: map[string]any{
				"req_id":  reqID,
				"chat_id": body.ChatID,
				"user_id": body.From.UserID,
			},
		},
		ReceivedAt: time.Now(),
		Metadata: map[string]any{
			"req_id":   reqID,
			"chat_id":  body.ChatID,
			"user_id":  body.From.UserID,
			"chattype": body.ChatType,
			// For group chats with mixed content, mark as mentioned (contains images)
			"is_mentioned": body.ChatType == "group",
		},
	}

	return handler(ctx, cfg, msg)
}

// downloadAndDecrypt downloads and decrypts a file from WeCom
func (a *Adapter) downloadAndDecrypt(url, aesKey string) ([]byte, error) {
	if url == "" {
		return nil, fmt.Errorf("url is empty")
	}

	a.logger.Info("downloading file", slog.String("url", url))

	// Download file
	resp, err := a.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download file: status code %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read file data: %w", err)
	}

	a.logger.Info("file downloaded", slog.Int("size", len(data)))

	// If no AES key, return raw data
	if aesKey == "" {
		return data, nil
	}

	// Decrypt file
	decrypted, err := decryptFile(data, aesKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt file: %w", err)
	}

	a.logger.Info("file decrypted", slog.Int("decrypted_size", len(decrypted)))
	return decrypted, nil
}

// getWebSocketClient returns the WebSocket client for a bot
func (a *Adapter) getWebSocketClient(botID string) *WebSocketClient {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.clients[botID]
}

// connection implements channel.Connection
type connection struct {
	configID    string
	botID       string
	channelType channel.ChannelType
	running     atomic.Bool
	stop        func()
	client      *WebSocketClient
}

func (c *connection) ConfigID() string {
	return c.configID
}

func (c *connection) BotID() string {
	return c.botID
}

func (c *connection) ChannelType() channel.ChannelType {
	return c.channelType
}

func (c *connection) Stop(ctx context.Context) error {
	c.running.Store(false)
	c.stop()
	return nil
}

func (c *connection) Running() bool {
	return c.running.Load() && c.client.IsConnected()
}

// truncateString truncates a string to maxLen with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Ensure Adapter implements required interfaces
var _ channel.Adapter = (*Adapter)(nil)
var _ channel.Receiver = (*Adapter)(nil)
var _ channel.StreamSender = (*Adapter)(nil)
var _ channel.Sender = (*Adapter)(nil)
var _ channel.Connection = (*connection)(nil)
