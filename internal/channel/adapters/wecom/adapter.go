package wecom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Kxiandaoyan/Memoh-v2/internal/channel"
	"github.com/Kxiandaoyan/Memoh-v2/internal/channel/route"
	"github.com/Kxiandaoyan/Memoh-v2/internal/message"
	"golang.org/x/time/rate"
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

// GroupMemberCache stores information about a group chat member
// This is used for passive member collection since WeCom AI Bot SDK
// does not provide an API to fetch group member lists directly.
type GroupMemberCache struct {
	UserID    string    // User ID
	Name      string    // User name (if available)
	FirstSeen time.Time // First time this member was seen in the group
	LastSeen  time.Time // Last time this member sent a message
	MsgCount  int       // Number of messages sent by this member
}

// GroupInfo stores information about a group chat and its members
type GroupInfo struct {
	ChatID      string                        // Group chat ID
	Name        string                        // Group chat name (manual set, since SDK doesn't provide it)
	FirstSeen   time.Time                     // First time this group was discovered
	LastActive  time.Time                     // Last time there was activity in this group
	Members     map[string]*GroupMemberCache  // Members map: user_id -> member info
	MemberCount int                           // Total member count
}

// Adapter implements the WeCom channel adapter
type Adapter struct {
	logger         *slog.Logger
	clients        map[string]*WebSocketClient // BotID -> Client
	mu             sync.RWMutex
	httpClient     *http.Client
	messageService message.Service // Optional: for clearing history on new chat
	routeService   route.Service   // Optional: for per-route history clearing on new chat

	// Rate limiters for send message
	// WeCom限制：30条/分钟，1000条/小时
	minuteLimiter *rate.Limiter // 30条/分钟
	hourLimiter   *rate.Limiter // 1000条/小时

	// Message deduplication manager
	dedupManager *DedupManager

	// Token cache for contacts API
	tokenCache *tokenCache

	// Group chat cache: chat_name -> chat_id
	groupCache   map[string]string // chat_name -> chat_id
	groupCacheMu sync.RWMutex

	// Group member cache: chat_id -> GroupInfo (被动收集群聊成员)
	groupMemberCache   map[string]*GroupInfo
	groupMemberCacheMu sync.RWMutex
}

// NewWeComAdapter creates a new WeCom adapter (alias for NewAdapter for compatibility)
func NewWeComAdapter(logger *slog.Logger) *Adapter {
	return NewAdapter(logger)
}

// SetMessageService sets the message service for history management
func (a *Adapter) SetMessageService(svc message.Service) {
	a.messageService = svc
}

// SetRouteService sets the route service used to resolve per-user/group route IDs.
func (a *Adapter) SetRouteService(svc route.Service) {
	a.routeService = svc
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
		// 初始化频率限制器：30条/分钟，1000条/小时
		// burst设置为1，确保严格限速
		minuteLimiter: rate.NewLimiter(rate.Every(2*time.Second), 1), // 30条/分钟 = 每2秒1条
		hourLimiter:   rate.NewLimiter(rate.Every(3600*time.Second/1000), 1), // 1000条/小时
		// 初始化消息去重管理器
		dedupManager: NewDedupManager(),
		// 初始化群聊缓存
		groupCache: make(map[string]string),
		// 初始化群聊成员缓存（被动收集）
		groupMemberCache: make(map[string]*GroupInfo),
	}
}

// Type returns the channel type identifier for WeCom.
func (a *Adapter) Type() channel.ChannelType {
	return Type
}

// Descriptor returns the channel descriptor containing metadata and configuration schema.
func (a *Adapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        Type,
		DisplayName: "企业微信",
		Configless:  false,
		Capabilities: channel.ChannelCapabilities{
			Text:           true,
			Markdown:       true,
			RichText:       true,
			Attachments:    true,
			Media:          true,
			Reactions:      false,
			Reply:          true,
			Streaming:      true,
			BlockStreaming: false,
		},
		OutboundPolicy: channel.OutboundPolicy{
			// 企业微信 AI Bot SDK 限制：单条消息最长 20480 字节
			// UTF-8 中文字符通常占 3 字节，设置 6800 字符约等于 20400 字节（全中文场景）
			// 实际分片逻辑在 stream.go 中按字节精确处理
			TextChunkLimit: 6800,
			ChunkerMode:    channel.ChunkerModeMarkdown,
			MediaOrder:     channel.OutboundOrderTextFirst,
			RetryMax:       3,
			RetryBackoffMs: 1000,
		},
		ConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"bot_id": {
					Type:        channel.FieldString,
					Required:    true,
					Title:       "Bot ID",
					Description: "企业微信机器人ID",
				},
				"secret": {
					Type:        channel.FieldSecret,
					Required:    true,
					Title:       "Secret",
					Description: "企业微信机器人Secret",
				},
				"websocket_url": {
					Type:        channel.FieldString,
					Required:    false,
					Title:       "WebSocket URL",
					Description: "WebSocket连接地址（默认：wss://openws.work.weixin.qq.com）",
					Example:     "wss://openws.work.weixin.qq.com",
				},
				"group_chat_enabled": {
					Type:        channel.FieldBool,
					Required:    false,
					Title:       "启用群聊",
					Description: "是否允许在群聊中响应",
				},
				"require_mention": {
					Type:        channel.FieldBool,
					Required:    false,
					Title:       "需要@提及",
					Description: "群聊中是否需要@机器人才响应",
				},
				"corp_id": {
					Type:        channel.FieldString,
					Required:    false,
					Title:       "企业ID (CorpID)",
					Description: "企业微信自建应用的企业ID，用于通讯录查询功能",
				},
				"corp_secret": {
					Type:        channel.FieldSecret,
					Required:    false,
					Title:       "应用凭证 (CorpSecret)",
					Description: "企业微信自建应用的Secret，用于获取通讯录访问权限",
				},
				"agent_id": {
					Type:        channel.FieldString,
					Required:    false,
					Title:       "应用ID (AgentID)",
					Description: "企业微信自建应用的AgentID，用于通过自建应用发送消息",
				},
			},
		},
		UserConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"user_id": {Type: channel.FieldString},
				"name":    {Type: channel.FieldString},
			},
		},
		TargetSpec: channel.TargetSpec{
			Format: "user_id:xxx | name:xxx | chat_name:xxx",
			Hints: []channel.TargetHint{
				{Label: "User ID", Example: "user_id:USER_ID"},
				{Label: "Name", Example: "name:用户名"},
				{Label: "Chat Name", Example: "chat_name:群聊名称"},
			},
		},
	}
}

// sendThinkingReply sends an immediate "thinking" response to improve user experience
// CRITICAL: Must use the same streamID as the actual response stream
func (a *Adapter) sendThinkingReply(ctx context.Context, wsClient *WebSocketClient, reqID string, streamID string) {
	if wsClient == nil || reqID == "" {
		return
	}

	// If no streamID provided, generate one
	if streamID == "" {
		streamID = generateStreamID()
	}

	// CRITICAL FIX: Send thinking reply immediately without queuing or ACK wait
	// This ensures we respond within the 5-second ACK window to prevent interruption
	a.sendThinkingWithAnimation(ctx, wsClient, reqID, streamID)
}

// sendThinkingWithAnimation sends an animated "thinking" message with wave effect dots
// The dots animate like a wave: . → .. → ...
func (a *Adapter) sendThinkingWithAnimation(ctx context.Context, wsClient *WebSocketClient, reqID string, streamID string) {
	// Animation frames for the dots
	frames := []string{".", "..", "..."}
	frameIndex := 0

	// Send first frame immediately
	a.sendThinkingFrame(wsClient, reqID, streamID, "⏳ 正在思考中"+frames[frameIndex])

	// Start animation goroutine
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond) // Update every 500ms
		defer ticker.Stop()

		// Maximum animation duration (30 seconds), then stop to prevent infinite loop
		maxDuration := 30 * time.Second
		timeout := time.After(maxDuration)

		for {
			select {
			case <-ticker.C:
				frameIndex = (frameIndex + 1) % len(frames)
				content := "⏳ 正在思考中" + frames[frameIndex]
				if err := a.sendThinkingFrame(wsClient, reqID, streamID, content); err != nil {
					// Stop animation if send fails (message likely already updated)
					return
				}
			case <-ctx.Done():
				// Context cancelled, stop animation
				return
			case <-timeout:
				// Max duration reached, stop animation
				a.logger.Debug("thinking animation timeout reached", slog.String("req_id", reqID), slog.String("stream_id", streamID))
				return
			}
		}
	}()
}

// sendThinkingFrame sends a single thinking frame
func (a *Adapter) sendThinkingFrame(wsClient *WebSocketClient, reqID string, streamID string, content string) error {
	thinkingBody := StreamMsgBody{
		MsgType: MsgTypeStream,
		Stream: StreamResponse{
			ID:      streamID,
			Finish:  false,
			Content: content,
		},
	}

	bodyBytes, err := json.Marshal(thinkingBody)
	if err != nil {
		return err
	}

	frame := WebsocketMessage{
		Cmd:     CmdRespondMsg,
		Headers: MessageHeaders{ReqID: reqID},
		Body:    bodyBytes,
	}

	return wsClient.sendFrame(frame)
}

// sendErrorReply sends an error response to cover the "thinking..." message
// CRITICAL: This prevents the "thinking..." message from being left visible when handler fails
func (a *Adapter) sendErrorReply(ctx context.Context, wsClient *WebSocketClient, reqID string, streamID string, errorMsg string) {
	if wsClient == nil || reqID == "" {
		return
	}

	// If no streamID provided, generate one (should not happen in normal flow)
	if streamID == "" {
		streamID = generateStreamID()
	}

	a.logger.Info("sending error reply to cover thinking message",
		slog.String("req_id", reqID),
		slog.String("stream_id", streamID),
		slog.String("error", errorMsg))

	// Send error message with finish=true to close the stream
	errorBody := StreamMsgBody{
		MsgType: MsgTypeStream,
		Stream: StreamResponse{
			ID:      streamID,
			Finish:  true, // Finish the stream
			Content: "[系统提示: " + errorMsg + "]",
		},
	}

	// Try to send error reply, ignore error to prevent blocking
	if err := wsClient.SendStream(ctx, reqID, errorBody); err != nil {
		a.logger.Warn("failed to send error reply", slog.String("req_id", reqID), slog.Any("error", err))
	} else {
		a.logger.Info("error reply sent successfully", slog.String("req_id", reqID))
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
	chatType := ""
	isMentioned := false
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
		if v, ok := opts.Metadata["chattype"].(string); ok {
			chatType = v
		}
		if v, ok := opts.Metadata["is_mentioned"].(bool); ok {
			isMentioned = v
		}
	}

	// Determine command type based on req_id availability
	// - If req_id is present: use CmdRespondMsg (reply to specific message)
	// - If req_id is empty: use CmdSendMsg (proactive send) with new req_id
	cmd := CmdRespondMsg
	if reqID == "" {
		reqID = generateReqID(CmdSendMsg)
		cmd = CmdSendMsg
		a.logger.Info("[MSG_ROUTE] OpenStream - no req_id, using proactive send",
			slog.String("generated_req_id", reqID),
			slog.String("target", target),
			slog.String("bot_id", cfg.BotID))
	}

	// Parse target for chat info
	if strings.HasPrefix(target, "chat_id:") {
		chatID = strings.TrimPrefix(target, "chat_id:")
	} else if strings.HasPrefix(target, "user_id:") {
		userID = strings.TrimPrefix(target, "user_id:")
	}

	// Extract stream_id from metadata (set by sendThinkingReply)
	// CRITICAL: Must use the same streamID as the thinking reply to update the same message
	streamID := ""
	if opts.Metadata != nil {
		if v, ok := opts.Metadata["stream_id"].(string); ok {
			streamID = v
		}
	}
	// If no stream_id in metadata (fallback), generate a new one
	if streamID == "" {
		streamID = generateStreamID()
	}

	// CRITICAL: Log the routing information for debugging message routing issues
	a.logger.Info("[MSG_ROUTE] OpenStream creating outbound stream",
		slog.String("req_id", reqID),
		slog.String("stream_id", streamID),
		slog.String("target", target),
		slog.String("extracted_user_id", userID),
		slog.String("extracted_chat_id", chatID),
		slog.String("chat_type", chatType),
		slog.Bool("is_mentioned", isMentioned),
		slog.String("bot_id", cfg.BotID))

	return NewOutboundStream(a, cfg, wsClient, reqID, chatID, userID, chatType, isMentioned, streamID, a.logger, opts.ReceivedAt, cmd), nil
}

// Send sends a message directly (non-streaming)
// getUploadManager returns an UploadManager for the given bot's WebSocket client.
func (a *Adapter) getUploadManager(botID string) (*UploadManager, error) {
	a.mu.RLock()
	wsClient, exists := a.clients[botID]
	a.mu.RUnlock()
	if !exists || !wsClient.IsConnected() {
		return nil, fmt.Errorf("websocket client not connected for bot %s", botID)
	}
	return NewUploadManager(wsClient, a.logger), nil
}

// Supports text messages and media attachments (uploaded via three-step media upload protocol)
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

	// Determine command type based on req_id availability
	// - If req_id is present: use CmdRespondMsg (reply to specific message)
	// - If req_id is empty: use CmdSendMsg (proactive send) with new req_id
	cmd := CmdRespondMsg
	if reqID == "" {
		reqID = generateReqID(CmdSendMsg)
		cmd = CmdSendMsg
		a.logger.Info("[MSG_ROUTE] Send (non-streaming) - no req_id, using proactive send",
			slog.String("generated_req_id", reqID),
			slog.String("target", msg.Target),
			slog.String("bot_id", cfg.BotID))
	} else {
		a.logger.Info("[MSG_ROUTE] Send (non-streaming) - using respond",
			slog.String("req_id", reqID),
			slog.String("target", msg.Target),
			slog.String("bot_id", cfg.BotID))
	}

	// Build response content
	content := msg.Message.Text
	if content == "" && len(msg.Message.Attachments) > 0 {
		content = "[媒体文件]"
	}

	// Upload attachments and collect media_ids
	type pendingMedia struct {
		mediaID   string
		mediaType string
		attType   channel.AttachmentType
	}
	var pendingMediaList []pendingMedia
	if len(msg.Message.Attachments) > 0 {
		upMgr := NewUploadManager(wsClient, a.logger)
		for _, att := range msg.Message.Attachments {
			if len(att.Data) == 0 {
				a.logger.Warn("[SEND] skipping attachment with no data", slog.String("name", att.Name))
				continue
			}
			mediaType := attachmentTypeToMediaType(att.Type)
			mediaID, err := upMgr.UploadMedia(ctx, att.Data, att.Name, mediaType)
			if err != nil {
				a.logger.Error("[SEND] failed to upload attachment",
					slog.String("name", att.Name),
					slog.String("media_type", mediaType),
					slog.Any("error", err))
				continue
			}
			pendingMediaList = append(pendingMediaList, pendingMedia{
				mediaID:   mediaID,
				mediaType: mediaType,
				attType:   att.Type,
			})
			a.logger.Info("[SEND] attachment uploaded",
				slog.String("name", att.Name),
				slog.String("media_id", mediaID))
		}
	}

	// CRITICAL: Use different message body based on command type
	// CmdSendMsg (proactive) requires SendMarkdownMsgBody with chatid and chat_type
	// CmdRespondMsg (reply) uses StreamMsgBody
	if cmd == CmdSendMsg {
		// Parse target to determine chatid and chat_type
		// Target format: "user_id:xxx", "chat_id:xxx", or "name:xxx" (lookup by name)
		chatID := ""
		chatTypeInt := ChatTypeAuto // 0 = auto (default)

		target := strings.TrimSpace(msg.Target)

		// Handle name:xxx format - lookup user by name
		if strings.HasPrefix(target, "name:") {
			name := strings.TrimPrefix(target, "name:")
			name = strings.TrimSpace(name)
			if name != "" {
				// Lookup user by name using directory adapter
				entry, err := a.ResolveEntry(ctx, cfg, name, channel.DirectoryEntryUser)
				if err != nil {
					a.logger.Error("[SEND] failed to resolve user by name",
						slog.String("name", name),
						slog.Any("error", err))
					return fmt.Errorf("user not found by name: %s", name)
				}
				// Extract user ID from entry (format: "userid:xxx")
				userID := strings.TrimPrefix(entry.ID, "userid:")
				target = "user_id:" + userID
				a.logger.Info("[SEND] resolved name to user_id",
					slog.String("name", name),
					slog.String("user_id", userID))
			}
		}

		// Handle chat_name:xxx format - lookup group by name from cache
		if strings.HasPrefix(target, "chat_name:") {
			chatName := strings.TrimPrefix(target, "chat_name:")
			chatName = strings.TrimSpace(chatName)
			if chatName != "" {
				a.groupCacheMu.RLock()
				chatID, found := a.groupCache[chatName]
				a.groupCacheMu.RUnlock()
				if !found {
					a.logger.Error("[SEND] group not found in cache",
						slog.String("chat_name", chatName))
					return fmt.Errorf("group not found by name: %s (bot needs to be added to the group first)", chatName)
				}
				target = "chat_id:" + chatID
				a.logger.Info("[SEND] resolved chat_name to chat_id",
					slog.String("chat_name", chatName),
					slog.String("chat_id", chatID))
			}
		}

		if strings.HasPrefix(target, "user_id:") {
			chatID = strings.TrimPrefix(target, "user_id:")
			chatTypeInt = ChatTypeSingle // 1 = single chat
		} else if strings.HasPrefix(target, "chat_id:") {
			chatID = strings.TrimPrefix(target, "chat_id:")
			chatTypeInt = ChatTypeGroup // 2 = group chat
		} else if target != "" {
			// Fallback: use target directly as chatid with auto type
			chatID = target
		}

		a.logger.Info("[MSG_ROUTE] Send proactive message",
			slog.String("req_id", reqID),
			slog.String("target", target),
			slog.String("chat_id", chatID),
			slog.Int("chat_type", chatTypeInt),
			slog.Int("content_bytes", len([]byte(content))))

		// Use SendMarkdownMsgBody for proactive send (CmdSendMsg)
		// SDK requires: chatid (target), chat_type (1=single, 2=group)
		body := SendMarkdownMsgBody{
			MsgType: MsgTypeMarkdown,
			Markdown: MarkdownContent{
				Content: content,
			},
			ChatID:   chatID,
			ChatType: chatTypeInt,
		}

		if err := wsClient.SendReply(ctx, reqID, body, cmd); err != nil {
			return err
		}

		// Send pending media messages
		for _, pm := range pendingMediaList {
			if err := a.sendMediaMessage(ctx, wsClient, chatID, chatTypeInt, pm.mediaID, pm.attType); err != nil {
				a.logger.Error("[SEND] failed to send media message (proactive)",
					slog.String("media_id", pm.mediaID),
					slog.Any("error", err))
			}
		}
		return nil
	}

	// Use StreamMsgBody for respond (CmdRespondMsg)
	body := StreamMsgBody{
		MsgType: MsgTypeStream,
		Stream: StreamResponse{
			ID:      generateStreamID(),
			Finish:  true,
			Content: content,
		},
	}

	if err := wsClient.SendReply(ctx, reqID, body, cmd); err != nil {
		return err
	}

	// Send pending media messages as proactive sends (CmdSendMsg)
	if len(pendingMediaList) > 0 {
		chatID, chatTypeInt := parseSendTarget(msg.Target)
		for _, pm := range pendingMediaList {
			if err := a.sendMediaMessage(ctx, wsClient, chatID, chatTypeInt, pm.mediaID, pm.attType); err != nil {
				a.logger.Error("[SEND] failed to send media message",
					slog.String("media_id", pm.mediaID),
					slog.Any("error", err))
				// Continue sending remaining media
			}
		}
	}
	return nil
}

// attachmentTypeToMediaType maps a channel.AttachmentType to a WeCom media type string.
func attachmentTypeToMediaType(t channel.AttachmentType) string {
	switch t {
	case channel.AttachmentImage, channel.AttachmentGIF:
		return MediaTypeImage
	case channel.AttachmentAudio, channel.AttachmentVoice:
		return MediaTypeVoice
	case channel.AttachmentVideo:
		return MediaTypeVideo
	default:
		return MediaTypeFile
	}
}

// parseSendTarget parses a target string into a chatID and chat_type integer.
// It handles user_id:xxx, chat_id:xxx, and bare ID formats.
func parseSendTarget(target string) (chatID string, chatType int) {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "user_id:") {
		return strings.TrimPrefix(target, "user_id:"), ChatTypeSingle
	}
	if strings.HasPrefix(target, "chat_id:") {
		return strings.TrimPrefix(target, "chat_id:"), ChatTypeGroup
	}
	return target, ChatTypeAuto
}

// sendMediaMessage sends a single media message using CmdSendMsg.
func (a *Adapter) sendMediaMessage(ctx context.Context, wsClient *WebSocketClient, chatID string, chatType int, mediaID string, attType channel.AttachmentType) error {
	mediaType := attachmentTypeToMediaType(attType)
	ref := &MediaIDRef{MediaID: mediaID}
	body := SendMediaMsgBody{
		MsgType:  mediaType,
		ChatID:   chatID,
		ChatType: chatType,
	}
	switch mediaType {
	case MediaTypeImage:
		body.Image = ref
	case MediaTypeVoice:
		body.Voice = ref
	case MediaTypeVideo:
		body.Video = ref
	default:
		body.File = ref
	}
	reqID := generateReqID(CmdSendMsg)
	a.logger.Info("[SEND] sending media message",
		slog.String("media_id", mediaID),
		slog.String("media_type", mediaType),
		slog.String("chat_id", chatID),
		slog.Int("chat_type", chatType))
	return wsClient.SendReply(ctx, reqID, body, CmdSendMsg)
}

// processQuoteContent processes quote content and returns text and attachments
func (a *Adapter) processQuoteContent(ctx context.Context, quote *QuoteContent, reqID string) (string, []channel.Attachment) {
	if quote == nil {
		return "", nil
	}

	var textParts []string
	var attachments []channel.Attachment

	switch quote.MsgType {
	case MsgTypeText:
		if quote.Text != nil && quote.Text.Content != "" {
			textParts = append(textParts, "[引用文本] "+quote.Text.Content)
		}

	case MsgTypeImage:
		if quote.Image != nil {
			a.logger.Info("processing quote image",
				slog.String("req_id", reqID),
				slog.String("url", quote.Image.URL))
			result, err := a.downloadAndDecrypt(quote.Image.URL, quote.Image.AESKey)
			if err != nil {
				a.logger.Error("failed to download/decrypt quote image",
					slog.String("req_id", reqID),
					slog.Any("error", err))
				// Add reference without data
				attachments = append(attachments, channel.Attachment{
					Type: channel.AttachmentImage,
					URL:  quote.Image.URL,
					Metadata: map[string]any{
						"aeskey": quote.Image.AESKey,
						"source": "quote",
					},
				})
				textParts = append(textParts, "[引用图片 (无法下载)]")
			} else {
				attachments = append(attachments, channel.Attachment{
					Type:     channel.AttachmentImage,
					URL:      quote.Image.URL,
					Data:     result.Data,
					Metadata: map[string]any{
						"aeskey": quote.Image.AESKey,
						"size":   len(result.Data),
						"source": "quote",
					},
				})
				textParts = append(textParts, "[引用图片]")
			}
		}

	case MsgTypeFile:
		if quote.File != nil {
			fileName := quote.File.FileName
			if fileName == "" {
				fileName = extractFileNameFromURL(quote.File.URL)
			}
			a.logger.Info("processing quote file",
				slog.String("req_id", reqID),
				slog.String("filename", fileName),
				slog.String("url", quote.File.URL))

			result, err := a.downloadAndDecrypt(quote.File.URL, quote.File.AESKey)
			if err != nil {
				a.logger.Error("failed to download/decrypt quote file",
					slog.String("req_id", reqID),
					slog.String("filename", fileName),
					slog.Any("error", err))
				// Add reference without data
				attachments = append(attachments, channel.Attachment{
					Type: channel.AttachmentFile,
					URL:  quote.File.URL,
					Name: fileName,
					Mime: getMimeTypeFromFileName(fileName),
					Metadata: map[string]any{
						"aeskey": quote.File.AESKey,
						"source": "quote",
					},
				})
				textParts = append(textParts, "[引用文件: "+fileName+" (无法下载)]")
			} else {
				// Use filename from Content-Disposition if available
				if result.FileName != "" {
					fileName = result.FileName
				}
				attachments = append(attachments, channel.Attachment{
					Type:     channel.AttachmentFile,
					URL:      quote.File.URL,
					Name:     fileName,
					Mime:     getMimeTypeFromFileName(fileName),
					Data:     result.Data,
					Metadata: map[string]any{
						"aeskey": quote.File.AESKey,
						"size":   len(result.Data),
						"source": "quote",
					},
				})
				textParts = append(textParts, "[引用文件: "+fileName+"]")
			}
		}

	case MsgTypeVoice:
		if quote.Voice != nil {
			a.logger.Info("processing quote voice",
				slog.String("req_id", reqID))
			// Voice messages in WeCom are transcribed to text
			// The VoiceContent only contains the transcribed text, no audio URL
			if quote.Voice.Content != "" {
				textParts = append(textParts, "[引用语音转文字] "+quote.Voice.Content)
			} else {
				textParts = append(textParts, "[引用语音]")
			}
		}

	case MsgTypeMixed:
		if quote.Mixed != nil {
			a.logger.Info("processing quote mixed content",
				slog.String("req_id", reqID),
				slog.Int("item_count", len(quote.Mixed.MsgItem)))

			var mixedParts []string
			for i, item := range quote.Mixed.MsgItem {
				switch item.MsgType {
				case MsgTypeText:
					if item.Text != nil && item.Text.Content != "" {
						mixedParts = append(mixedParts, item.Text.Content)
					}
				case MsgTypeImage:
					if item.Image != nil {
						result, err := a.downloadAndDecrypt(item.Image.URL, item.Image.AESKey)
						if err != nil {
							a.logger.Error("failed to download/decrypt quote mixed image",
								slog.String("req_id", reqID),
								slog.Int("item_index", i),
								slog.Any("error", err))
							attachments = append(attachments, channel.Attachment{
								Type: channel.AttachmentImage,
								URL:  item.Image.URL,
								Metadata: map[string]any{
									"aeskey": item.Image.AESKey,
									"source": "quote_mixed",
									"index":  i,
								},
							})
							mixedParts = append(mixedParts, "[图片]")
						} else {
							attachments = append(attachments, channel.Attachment{
								Type: channel.AttachmentImage,
								URL:  item.Image.URL,
								Data: result.Data,
								Metadata: map[string]any{
									"aeskey": item.Image.AESKey,
									"size":   len(result.Data),
									"source": "quote_mixed",
									"index":  i,
								},
							})
							mixedParts = append(mixedParts, "[图片]")
						}
					}
				}
			}

			if len(mixedParts) > 0 {
				textParts = append(textParts, "[引用图文] "+strings.Join(mixedParts, " "))
			}
		}

	default:
		a.logger.Warn("unknown quote message type",
			slog.String("quote_type", quote.MsgType),
			slog.String("req_id", reqID))
		textParts = append(textParts, "[引用消息 (未知类型)]")
	}

	return strings.Join(textParts, "\n"), attachments
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
	// [PERF] 记录消息处理开始时间
	processingStartTime := time.Now()
	reqID := wsMsg.Headers.ReqID

	var body MsgCallbackBody
	if err := json.Unmarshal(wsMsg.Body, &body); err != nil {
		return fmt.Errorf("unmarshal message body: %w", err)
	}

	// Check for duplicate messages using req_id + msgid
	if a.dedupManager.IsDuplicate(wsMsg.Headers.ReqID, body.MsgID) {
		a.logger.Info("duplicate message detected, skipping",
			slog.String("req_id", wsMsg.Headers.ReqID),
			slog.String("msg_id", body.MsgID),
			slog.String("from_user", body.From.UserID))
		return nil
	}

	// Get WebSocket client for sending replies
	wsClient := a.getWebSocketClient(cfg.BotID)

	// Process quote message if present
	var quoteText string
	var quoteAttachments []channel.Attachment
	if body.Quote != nil {
		a.logger.Info("message contains quote",
			slog.String("quote_type", body.Quote.MsgType),
			slog.String("req_id", wsMsg.Headers.ReqID))
		quoteText, quoteAttachments = a.processQuoteContent(ctx, body.Quote, wsMsg.Headers.ReqID)
		if quoteText != "" {
			a.logger.Info("quote content extracted",
				slog.String("quote_text_preview", truncateString(quoteText, 100)),
				slog.Int("quote_attachments", len(quoteAttachments)),
				slog.String("req_id", wsMsg.Headers.ReqID))
		}
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

	// Determine reply target based on chat type
	replyTarget := ""
	if body.ChatType == "group" {
		replyTarget = "chat_id:" + body.ChatID
		// Cache group chat info for later lookup by name
		a.groupCacheMu.Lock()
		if a.groupCache == nil {
			a.groupCache = make(map[string]string)
		}
		// Use chat_id as the key (user can later alias it via API if needed)
		a.groupCache[body.ChatID] = body.ChatID
		a.groupCacheMu.Unlock()
		a.logger.Debug("[GROUP_CACHE] cached group chat",
			slog.String("chat_id", body.ChatID))

		// 被动收集群聊成员信息
		a.updateGroupMemberCache(body.ChatID, body.From.UserID, body.From.Name)
	} else {
		replyTarget = "user_id:" + body.From.UserID
	}

	a.logger.Info("[MSG_ROUTE] message received",
		slog.String("msg_id", body.MsgID),
		slog.String("msg_type", body.MsgType),
		slog.String("from_user_id", body.From.UserID),
		slog.String("chat_type", body.ChatType),
		slog.String("chat_id", body.ChatID),
		slog.String("req_id", wsMsg.Headers.ReqID),
		slog.String("reply_target", replyTarget),
		slog.String("content_preview", truncateString(contentPreview, 100)),
		slog.Bool("group_chat_enabled", config.GroupChatEnabled),
		slog.Bool("require_mention", config.RequireMention))

	// According to the WeCom AI Bot SDK spec, BaseMessage.chatid is only present
	// for group chats. For single chats the field is absent (empty string).
	// We use the sender's userid as the conversation ID for single chats so that
	// each user gets an isolated bot_channel_routes entry and independent history.
	conversationID := body.ChatID
	if conversationID == "" {
		conversationID = body.From.UserID
	}
	isGroup := body.ChatType == "group"

	// Collect full sender + scene identity for downstream context.
	// IMPORTANT: req_id must be present so inbound/channel.go can pass it to OpenStream,
	// which uses it to select CmdRespondMsg (reply to message) vs CmdSendMsg (proactive).
	senderMeta := map[string]any{
		"req_id":       wsMsg.Headers.ReqID, // critical for WeCom respond-mode routing
		"from_user_id": body.From.UserID,
		"user_id":      body.From.UserID, // legacy key kept for compatibility
		"chat_type":    body.ChatType,
		"chattype":     body.ChatType, // legacy key kept for compatibility
		"chat_id":      body.ChatID,   // raw SDK chatid (empty for single chat)
		"is_group":     isGroup,
	}
	if body.From.Name != "" {
		senderMeta["sender_name"] = body.From.Name
	}
	if body.From.CorpID != "" {
		senderMeta["corp_id"] = body.From.CorpID
	}

	// Convert to internal message
	msg := channel.InboundMessage{
		Channel: Type,
		BotID:   cfg.BotID,
		Sender: channel.Identity{
			SubjectID: body.From.UserID,
		},
		Conversation: channel.Conversation{
			ID:   conversationID, // userid for single chat; group chatid for group chat
			Type: body.ChatType,
			Metadata: map[string]any{
				"chat_type":    body.ChatType,
				"chattype":     body.ChatType, // legacy
				"chat_id":      body.ChatID,   // raw SDK chatid
				"from_user_id": body.From.UserID,
				"is_group":     isGroup,
			},
		},
		ReplyTarget: replyTarget,
		Message: channel.Message{
			ID:       body.MsgID,
			Metadata: senderMeta,
		},
		ReceivedAt: time.Now(),
		Metadata:   senderMeta,
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
					slog.String("bot_id", cfg.BotID),
					slog.Bool("group_chat_enabled", config.GroupChatEnabled),
					slog.Bool("require_mention", config.RequireMention),
					slog.Bool("should_trigger", config.ShouldTriggerGroupResponse(originalContent)))
			}

			// Check if should trigger response in group chat (before removing mention prefix)
			shouldTrigger := body.ChatType == "single" || config.ShouldTriggerGroupResponse(originalContent)

			if body.ChatType == "group" {
				content = config.ExtractGroupMessageContent(content)
			}

			// Check command allowlist (admin bypass)
			_, blocked, cmd := config.CanExecuteCommand(body.From.UserID, content)
			if blocked {
				a.logger.Info("command blocked by allowlist",
					slog.String("user_id", body.From.UserID),
					slog.String("command", cmd),
					slog.Bool("is_admin", config.IsAdmin(body.From.UserID)))
				// Send block message to user
				if wsClient != nil {
					blockBody := StreamMsgBody{
						MsgType: MsgTypeStream,
						Stream: StreamResponse{
							ID:      generateStreamID(),
							Finish:  true,
							Content: BuildBlockMessage(cmd),
						},
					}
					if err := wsClient.SendReply(ctx, wsMsg.Headers.ReqID, blockBody, CmdRespondMsg); err != nil {
						a.logger.Warn("failed to send command block message", slog.Any("error", err))
					}
				}
				return nil
			}

			// Check for new chat command
			if isNewChatCommand(content) {
				return a.handleNewChatCommand(ctx, cfg, wsMsg, body)
			}

			// Add quote content if present
			finalContent := content
			if quoteText != "" {
				finalContent = quoteText + "\n\n---\n" + content
				a.logger.Info("quote content prepended to message",
					slog.String("quote_text_preview", truncateString(quoteText, 50)),
					slog.String("final_content_preview", truncateString(finalContent, 100)),
					slog.String("req_id", wsMsg.Headers.ReqID))
			}
			msg.Message.Text = finalContent
			msg.Message.Format = channel.MessageFormatPlain

			// Add quote attachments if present
			if len(quoteAttachments) > 0 {
				msg.Message.Attachments = append(msg.Message.Attachments, quoteAttachments...)
				a.logger.Info("quote attachments added to message",
					slog.Int("quote_attachment_count", len(quoteAttachments)),
					slog.Int("total_attachment_count", len(msg.Message.Attachments)),
					slog.String("req_id", wsMsg.Headers.ReqID))
			}

			// Add is_mentioned metadata for group chats
			if body.ChatType == "group" {
				msg.Metadata["is_mentioned"] = config.ShouldTriggerGroupResponse(originalContent)
			}

			// Check if should trigger response
			if shouldTrigger {
				// Send immediate "thinking" reply for better UX
				// CRITICAL: Generate streamID here and pass to both thinking reply and handler
				streamID := generateStreamID()
				// [PERF] 记录发送 thinking 回复前的时间
				thinkingStartTime := time.Now()
				a.sendThinkingReply(ctx, wsClient, wsMsg.Headers.ReqID, streamID)
				a.logger.Info("[PERF] thinking reply sent",
					slog.String("req_id", reqID),
					slog.Duration("time_since_receive", time.Since(processingStartTime)),
					slog.Duration("thinking_send_time", time.Since(thinkingStartTime)))
				// Store streamID in message metadata so CreateOutboundStream can use it
				msg.Metadata["stream_id"] = streamID
				a.logger.Info("[MSG_ROUTE] calling handler for text message",
					slog.String("req_id", wsMsg.Headers.ReqID),
					slog.String("stream_id", streamID),
					slog.String("from_user_id", body.From.UserID),
					slog.String("reply_target", msg.ReplyTarget),
					slog.String("content_preview", truncateString(content, 100)))
				err := handler(ctx, cfg, msg)
				totalProcessingTime := time.Since(processingStartTime)
				if err != nil {
					a.logger.Error("[MSG_ROUTE] handler returned error",
						slog.String("req_id", wsMsg.Headers.ReqID),
						slog.String("from_user_id", body.From.UserID),
						slog.String("reply_target", msg.ReplyTarget),
						slog.Any("error", err))
					// CRITICAL: Send error reply to cover "thinking..." message
					a.sendErrorReply(ctx, wsClient, wsMsg.Headers.ReqID, streamID, "处理出错，请重试")
				} else {
					a.logger.Info("[MSG_ROUTE] handler completed successfully",
						slog.String("req_id", wsMsg.Headers.ReqID),
						slog.String("from_user_id", body.From.UserID),
						slog.String("reply_target", msg.ReplyTarget),
						slog.Duration("[PERF] total_processing_time", totalProcessingTime))
				}
				return err
			}
			a.logger.Info("skipping group message (no mention)")
			return nil
		}

	case MsgTypeImage:
		if body.Image != nil {
			// SDK限制：图片消息仅支持单聊
			if body.ChatType == "group" {
				a.logger.Warn("image message received in group chat, but image type only supports single chat according to SDK docs",
					slog.String("chat_id", body.ChatID),
					slog.String("from_user", body.From.UserID))
				msg.Metadata["is_mentioned"] = true
			}

			// Download and decrypt image
			result, err := a.downloadAndDecrypt(body.Image.URL, body.Image.AESKey)
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
					Data:     result.Data,
					Metadata: map[string]any{
						"aeskey": body.Image.AESKey,
						"size":   len(result.Data),
					},
				})
			}
			// Add quote content if present
			imageText := "[用户发送了一张图片]"
			if quoteText != "" {
				imageText = quoteText + "\n\n---\n" + imageText
			}
			msg.Message.Text = imageText
			msg.Message.Format = channel.MessageFormatPlain

			// Add quote attachments if present
			if len(quoteAttachments) > 0 {
				msg.Message.Attachments = append(msg.Message.Attachments, quoteAttachments...)
				a.logger.Info("quote attachments added to image message",
					slog.Int("quote_attachment_count", len(quoteAttachments)),
					slog.String("req_id", wsMsg.Headers.ReqID))
			}

			// Send immediate "thinking" reply for better UX
			// CRITICAL: Generate streamID here and pass to both thinking reply and handler
			streamID := generateStreamID()
			a.sendThinkingReply(ctx, wsClient, wsMsg.Headers.ReqID, streamID)
			// Store streamID in message metadata so CreateOutboundStream can use it
			msg.Metadata["stream_id"] = streamID
			a.logger.Info("calling handler for image message", slog.String("req_id", wsMsg.Headers.ReqID), slog.String("stream_id", streamID))
			err = handler(ctx, cfg, msg)
			if err != nil {
				a.logger.Error("handler returned error for image", slog.String("req_id", wsMsg.Headers.ReqID), slog.Any("error", err))
				// CRITICAL: Send error reply to cover "thinking..." message
				a.sendErrorReply(ctx, wsClient, wsMsg.Headers.ReqID, streamID, "处理图片出错，请重试")
			} else {
				a.logger.Info("handler completed successfully for image", slog.String("req_id", wsMsg.Headers.ReqID))
			}
			return err
		}

	case MsgTypeFile:
		if body.File != nil {
			// SDK限制：文件消息仅支持单聊
			if body.ChatType == "group" {
				a.logger.Warn("file message received in group chat, but file type only supports single chat according to SDK docs",
					slog.String("chat_id", body.ChatID),
					slog.String("from_user", body.From.UserID))
				msg.Metadata["is_mentioned"] = true
			}

			// Get filename - use provided name or extract from URL
			fileName := body.File.FileName
			a.logger.Info("processing file message", slog.String("providedFileName", fileName), slog.String("url", body.File.URL))
			if fileName == "" {
				fileName = extractFileNameFromURL(body.File.URL)
				a.logger.Info("extracted filename from URL", slog.String("fileName", fileName))
			}

			// Download and decrypt file (filename from Content-Disposition header takes precedence)
			result, err := a.downloadAndDecrypt(body.File.URL, body.File.AESKey)
			if err != nil {
				a.logger.Error("failed to download/decrypt file", slog.Any("error", err))
				// Get MIME type based on file extension
				mimeType := getMimeTypeFromFileName(fileName)
				a.logger.Info("file download failed, using mime type from filename", slog.String("fileName", fileName), slog.String("mimeType", mimeType))
				msg.Message.Attachments = append(msg.Message.Attachments, channel.Attachment{
					Type: channel.AttachmentFile,
					URL:  body.File.URL,
					Name: fileName,
					Mime: mimeType,
					Metadata: map[string]any{
						"aeskey": body.File.AESKey,
					},
				})
			} else {
				// Use filename from Content-Disposition header if available (SDK compliant)
				if result.FileName != "" {
					a.logger.Info("using filename from Content-Disposition header", slog.String("fileName", result.FileName))
					fileName = result.FileName
				}
				// Get MIME type based on file extension
				mimeType := getMimeTypeFromFileName(fileName)
				a.logger.Info("file download success", slog.String("fileName", fileName), slog.String("mimeType", mimeType), slog.Int("dataSize", len(result.Data)))
				msg.Message.Attachments = append(msg.Message.Attachments, channel.Attachment{
					Type:     channel.AttachmentFile,
					URL:      body.File.URL,
					Name:     fileName,
					Mime:     mimeType,
					Data:     result.Data,
					Metadata: map[string]any{
						"aeskey": body.File.AESKey,
						"size":   len(result.Data),
					},
				})
			}
			// Set a default text for pure file messages so buildInboundQuery doesn't return empty
			displayName := fileName
			if displayName == "" {
				displayName = "未知文件"
			}
			fileText := "[用户发送了一个文件: " + displayName + "]"
			if quoteText != "" {
				fileText = quoteText + "\n\n---\n" + fileText
			}
			msg.Message.Text = fileText
			msg.Message.Format = channel.MessageFormatPlain

			// Add quote attachments if present
			if len(quoteAttachments) > 0 {
				msg.Message.Attachments = append(msg.Message.Attachments, quoteAttachments...)
				a.logger.Info("quote attachments added to file message",
					slog.Int("quote_attachment_count", len(quoteAttachments)),
					slog.String("req_id", wsMsg.Headers.ReqID))
			}

			// Send immediate "thinking" reply for better UX
			// CRITICAL: Generate streamID here and pass to both thinking reply and handler
			streamID := generateStreamID()
			a.sendThinkingReply(ctx, wsClient, wsMsg.Headers.ReqID, streamID)
			// Store streamID in message metadata so CreateOutboundStream can use it
			msg.Metadata["stream_id"] = streamID
			err = handler(ctx, cfg, msg)
			if err != nil {
				a.logger.Error("handler returned error for file", slog.String("req_id", wsMsg.Headers.ReqID), slog.Any("error", err))
				// CRITICAL: Send error reply to cover "thinking..." message
				a.sendErrorReply(ctx, wsClient, wsMsg.Headers.ReqID, streamID, "处理文件出错，请重试")
			}
			return err
		}

	case MsgTypeVoice:
		if body.Voice != nil {
			// SDK限制：语音消息仅支持单聊
			if body.ChatType == "group" {
				a.logger.Warn("voice message received in group chat, but voice type only supports single chat according to SDK docs",
					slog.String("chat_id", body.ChatID),
					slog.String("from_user", body.From.UserID))
				msg.Metadata["is_mentioned"] = true
			}

			// Voice content is the transcribed text from WeCom
			voiceText := "[语音] " + body.Voice.Content
			if quoteText != "" {
				voiceText = quoteText + "\n\n---\n" + voiceText
			}
			msg.Message.Text = voiceText
			msg.Message.Format = channel.MessageFormatPlain

			// Add quote attachments if present
			if len(quoteAttachments) > 0 {
				msg.Message.Attachments = append(msg.Message.Attachments, quoteAttachments...)
				a.logger.Info("quote attachments added to voice message",
					slog.Int("quote_attachment_count", len(quoteAttachments)),
					slog.String("req_id", wsMsg.Headers.ReqID))
			}

			// Send immediate "thinking" reply for better UX
			// CRITICAL: Generate streamID here and pass to both thinking reply and handler
			streamID := generateStreamID()
			a.sendThinkingReply(ctx, wsClient, wsMsg.Headers.ReqID, streamID)
			// Store streamID in message metadata so CreateOutboundStream can use it
			msg.Metadata["stream_id"] = streamID
			err := handler(ctx, cfg, msg)
			if err != nil {
				a.logger.Error("handler returned error for voice", slog.String("req_id", wsMsg.Headers.ReqID), slog.Any("error", err))
				// CRITICAL: Send error reply to cover "thinking..." message
				a.sendErrorReply(ctx, wsClient, wsMsg.Headers.ReqID, streamID, "处理语音出错，请重试")
			}
			return err
		}

	case MsgTypeMixed:
		if body.Mixed != nil {
			return a.handleMixedContent(ctx, cfg, config, handler, wsMsg.Headers.ReqID, body, quoteText, quoteAttachments)
		}

	default:
		a.logger.Warn("unknown message type", slog.String("msg_type", body.MsgType))
	}

	return nil
}

// updateGroupMemberCache updates the group member cache when a message is received in a group chat.
// This is a passive collection mechanism since WeCom AI Bot SDK does not provide an API
// to fetch group member lists directly.
func (a *Adapter) updateGroupMemberCache(chatID, userID, userName string) {
	a.groupMemberCacheMu.Lock()
	defer a.groupMemberCacheMu.Unlock()

	if a.groupMemberCache == nil {
		a.groupMemberCache = make(map[string]*GroupInfo)
	}

	group, exists := a.groupMemberCache[chatID]
	if !exists {
		group = &GroupInfo{
			ChatID:    chatID,
			FirstSeen: time.Now(),
			Members:   make(map[string]*GroupMemberCache),
		}
		a.groupMemberCache[chatID] = group
		a.logger.Info("[GROUP_MEMBER_CACHE] new group discovered",
			slog.String("chat_id", chatID))
	}

	// Update group activity
	group.LastActive = time.Now()

	// Update or add member
	member, exists := group.Members[userID]
	if !exists {
		member = &GroupMemberCache{
			UserID:    userID,
			Name:      userName,
			FirstSeen: time.Now(),
		}
		group.Members[userID] = member
		group.MemberCount = len(group.Members)
		a.logger.Info("[GROUP_MEMBER_CACHE] new member discovered",
			slog.String("chat_id", chatID),
			slog.String("user_id", userID),
			slog.String("user_name", userName),
			slog.Int("total_members", group.MemberCount))
	}

	member.LastSeen = time.Now()
	member.MsgCount++
}

// GetGroupInfo returns cached group info if available
func (a *Adapter) GetGroupInfo(chatID string) (*GroupInfo, bool) {
	a.groupMemberCacheMu.RLock()
	defer a.groupMemberCacheMu.RUnlock()

	group, exists := a.groupMemberCache[chatID]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid external modification
	return &GroupInfo{
		ChatID:      group.ChatID,
		Name:        group.Name,
		FirstSeen:   group.FirstSeen,
		LastActive:  group.LastActive,
		MemberCount: group.MemberCount,
		// Note: Members map is not copied for simplicity; add method if needed
	}, true
}

// ListCachedGroups returns a list of all cached group chat IDs
func (a *Adapter) ListCachedGroups() []string {
	a.groupMemberCacheMu.RLock()
	defer a.groupMemberCacheMu.RUnlock()

	groups := make([]string, 0, len(a.groupMemberCache))
	for chatID := range a.groupMemberCache {
		groups = append(groups, chatID)
	}
	return groups
}

// GroupSummary represents a summary of cached group info for listing
type GroupSummary struct {
	ChatID      string    `json:"chat_id"`
	Name        string    `json:"name"`
	MemberCount int       `json:"member_count"`
	FirstSeen   time.Time `json:"first_seen"`
	LastActive  time.Time `json:"last_active"`
}

// ListCachedGroupDetails returns detailed info for all cached groups
func (a *Adapter) ListCachedGroupDetails() []GroupSummary {
	a.groupMemberCacheMu.RLock()
	defer a.groupMemberCacheMu.RUnlock()

	groups := make([]GroupSummary, 0, len(a.groupMemberCache))
	for chatID, group := range a.groupMemberCache {
		name := group.Name
		if name == "" {
			name = chatID // Use chatID as name if not set
		}
		groups = append(groups, GroupSummary{
			ChatID:      chatID,
			Name:        name,
			MemberCount: group.MemberCount,
			FirstSeen:   group.FirstSeen,
			LastActive:  group.LastActive,
		})
	}
	return groups
}

// SetGroupName sets a friendly name for a cached group chat
func (a *Adapter) SetGroupName(chatID, name string) error {
	a.groupMemberCacheMu.Lock()
	defer a.groupMemberCacheMu.Unlock()

	group, exists := a.groupMemberCache[chatID]
	if !exists {
		return fmt.Errorf("group %s not found in cache", chatID)
	}

	group.Name = name
	a.logger.Info("[GROUP_CACHE] group name updated",
		slog.String("chat_id", chatID),
		slog.String("name", name))
	return nil
}

// GetGroupByName finds a group by its friendly name
func (a *Adapter) GetGroupByName(name string) (*GroupInfo, bool) {
	a.groupMemberCacheMu.RLock()
	defer a.groupMemberCacheMu.RUnlock()

	for _, group := range a.groupMemberCache {
		if group.Name == name {
			return &GroupInfo{
				ChatID:      group.ChatID,
				Name:        group.Name,
				FirstSeen:   group.FirstSeen,
				LastActive:  group.LastActive,
				MemberCount: group.MemberCount,
			}, true
		}
	}
	return nil, false
}

// GroupCacheStats returns statistics about the group cache
func (a *Adapter) GroupCacheStats() map[string]any {
	a.groupMemberCacheMu.RLock()
	defer a.groupMemberCacheMu.RUnlock()

	totalGroups := len(a.groupMemberCache)
	totalMembers := 0
	namedGroups := 0

	for _, group := range a.groupMemberCache {
		totalMembers += group.MemberCount
		if group.Name != "" {
			namedGroups++
		}
	}

	return map[string]any{
		"total_groups":   totalGroups,
		"named_groups":   namedGroups,
		"total_members":  totalMembers,
		"avg_members":    float64(totalMembers) / float64(max(1, totalGroups)),
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// handleNewChatCommand handles the "new chat" command to clear conversation history
func (a *Adapter) handleNewChatCommand(ctx context.Context, cfg channel.ChannelConfig, wsMsg *WebsocketMessage, body MsgCallbackBody) error {
	a.logger.Info("new chat command received",
		slog.String("user_id", body.From.UserID),
		slog.String("bot_id", cfg.BotID))

	// Clear history scoped to the invoking user/group's route only.
	// Previously this called DeleteByBot which would erase ALL users' history for this bot.
	if a.messageService != nil {
		cleared := false
		if a.routeService != nil {
			// Determine the conversation ID: for group chats use ChatID, for single chats use the sender's UserID.
			convID := body.ChatID
			if convID == "" {
				convID = body.From.UserID
			}
			r, err := a.routeService.Find(ctx, cfg.BotID, "wecom", convID, "")
			if err == nil && r.ID != "" {
				if delErr := a.messageService.DeleteByRoute(ctx, r.ID); delErr != nil {
					a.logger.Error("failed to clear history by route", slog.String("route_id", r.ID), slog.Any("error", delErr))
				} else {
					a.logger.Info("history cleared by route", slog.String("route_id", r.ID))
					cleared = true
				}
			} else {
				a.logger.Warn("route not found for new chat command, falling back to bot-level clear",
					slog.String("conv_id", convID), slog.Any("error", err))
			}
		}
		if !cleared {
			// Fallback: no routeService or route lookup failed – clear entire bot history.
			if err := a.messageService.DeleteByBot(ctx, cfg.BotID); err != nil {
				a.logger.Error("failed to clear history", slog.Any("error", err))
			}
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
		// 当有新连接建立时，系统会给旧连接发送该事件
		// 每个机器人同时只能保持一个有效长连接，新连接会踢掉旧连接
		a.logger.Warn("received disconnected event: new connection established, this connection will be closed",
			slog.String("bot_id", cfg.BotID),
			slog.String("config_id", cfg.ID))

		// 获取 WebSocket 客户端并触发重连
		if wsClient := a.getWebSocketClient(cfg.BotID); wsClient != nil {
			// 标记为手动关闭以避免自动重连
			wsClient.isManualClose = true
			a.logger.Info("marking connection as manually closed due to disconnected_event")
		}

		// 通知用户连接被替换
		a.logger.Info("connection replaced by new connection, please check for duplicate bot instances")

	default:
		a.logger.Debug("unhandled event type", slog.String("event_type", eventType))
	}

	return nil
}

// handleMixedContent handles mixed content (text + image) messages
func (a *Adapter) handleMixedContent(ctx context.Context, cfg channel.ChannelConfig, config *Config, handler channel.InboundHandler, reqID string, body MsgCallbackBody, quoteText string, quoteAttachments []channel.Attachment) error {
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
				result, err := a.downloadAndDecrypt(item.Image.URL, item.Image.AESKey)
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
						Data:     result.Data,
						Metadata: map[string]any{
							"aeskey": item.Image.AESKey,
							"size":   len(result.Data),
						},
					})
				}
			}
		case MsgTypeFile:
			if item.File != nil {
				// Get filename - use provided name or extract from URL
				fileName := item.File.FileName
				a.logger.Info("processing mixed file message", slog.String("providedFileName", fileName), slog.String("url", item.File.URL))
				if fileName == "" {
					fileName = extractFileNameFromURL(item.File.URL)
					a.logger.Info("extracted filename from URL for mixed content", slog.String("fileName", fileName))
				}

				// Download and decrypt file (filename from Content-Disposition header takes precedence)
				result, err := a.downloadAndDecrypt(item.File.URL, item.File.AESKey)
				if err != nil {
					a.logger.Error("failed to download/decrypt mixed file", slog.Any("error", err))
					// Get MIME type based on file extension
					mimeType := getMimeTypeFromFileName(fileName)
					a.logger.Info("mixed file download failed, using mime type from filename", slog.String("fileName", fileName), slog.String("mimeType", mimeType))
					attachments = append(attachments, channel.Attachment{
						Type: channel.AttachmentFile,
						URL:  item.File.URL,
						Name: fileName,
						Mime: mimeType,
						Metadata: map[string]any{
							"aeskey": item.File.AESKey,
						},
					})
				} else {
					// Use filename from Content-Disposition header if available (SDK compliant)
					if result.FileName != "" {
						a.logger.Info("using filename from Content-Disposition header for mixed content", slog.String("fileName", result.FileName))
						fileName = result.FileName
					}
					// Get MIME type based on file extension
					mimeType := getMimeTypeFromFileName(fileName)
					a.logger.Info("mixed file download success", slog.String("fileName", fileName), slog.String("mimeType", mimeType), slog.Int("dataSize", len(result.Data)))
					attachments = append(attachments, channel.Attachment{
						Type:     channel.AttachmentFile,
						URL:      item.File.URL,
						Name:     fileName,
						Mime:     mimeType,
						Data:     result.Data,
						Metadata: map[string]any{
							"aeskey": item.File.AESKey,
							"size":   len(result.Data),
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

	// Add quote content if present
	finalContent := content
	if quoteText != "" {
		finalContent = quoteText + "\n\n---\n" + content
		a.logger.Info("quote content prepended to mixed message",
			slog.String("quote_text_preview", truncateString(quoteText, 50)),
			slog.String("final_content_preview", truncateString(finalContent, 100)),
			slog.String("req_id", reqID))
	}

	// Add quote attachments if present
	if len(quoteAttachments) > 0 {
		attachments = append(attachments, quoteAttachments...)
		a.logger.Info("quote attachments added to mixed message",
			slog.Int("quote_attachment_count", len(quoteAttachments)),
			slog.Int("total_attachment_count", len(attachments)),
			slog.String("req_id", reqID))
	}

	// Same single-chat fix: use sender userid when chatid is absent (single chat)
	mixedConvID := body.ChatID
	if mixedConvID == "" {
		mixedConvID = body.From.UserID
	}
	mixedIsGroup := body.ChatType == "group"
	mixedMeta := map[string]any{
		"req_id":       reqID,
		"from_user_id": body.From.UserID,
		"user_id":      body.From.UserID, // legacy
		"chat_id":      body.ChatID,      // raw SDK chatid
		"chat_type":    body.ChatType,
		"chattype":     body.ChatType, // legacy
		"is_group":     mixedIsGroup,
		// For group chats with mixed content, mark as mentioned (contains images)
		"is_mentioned": mixedIsGroup,
	}
	if body.From.Name != "" {
		mixedMeta["sender_name"] = body.From.Name
	}
	if body.From.CorpID != "" {
		mixedMeta["corp_id"] = body.From.CorpID
	}

	msg := channel.InboundMessage{
		Channel: Type,
		BotID:   cfg.BotID,
		Sender: channel.Identity{
			SubjectID: body.From.UserID,
		},
		Conversation: channel.Conversation{
			ID:   mixedConvID, // userid for single chat; group chatid for group chat
			Type: body.ChatType,
			Metadata: map[string]any{
				"chat_type":    body.ChatType,
				"chattype":     body.ChatType, // legacy
				"chat_id":      body.ChatID,   // raw SDK chatid
				"from_user_id": body.From.UserID,
				"is_group":     mixedIsGroup,
			},
		},
		ReplyTarget: replyTarget,
		Message: channel.Message{
			ID:          body.MsgID,
			Text:        finalContent,
			Format:      channel.MessageFormatPlain,
			Attachments: attachments,
			Metadata:    mixedMeta,
		},
		ReceivedAt: time.Now(),
		Metadata:   mixedMeta,
	}

	// Send immediate "thinking" reply for better UX
	// CRITICAL: Generate streamID here and pass to both thinking reply and handler
	streamID := generateStreamID()
	wsClient := a.getWebSocketClient(cfg.BotID)
	a.sendThinkingReply(ctx, wsClient, reqID, streamID)
	// Store streamID in message metadata so CreateOutboundStream can use it
	msg.Metadata["stream_id"] = streamID

	err := handler(ctx, cfg, msg)
	if err != nil {
		a.logger.Error("handler returned error for proactive message", slog.String("req_id", reqID), slog.Any("error", err))
		// CRITICAL: Send error reply to cover "thinking..." message
		a.sendErrorReply(ctx, wsClient, reqID, streamID, "发送消息出错，请重试")
	}
	return err
}

// DownloadResult holds the result of a file download including metadata
type DownloadResult struct {
	Data     []byte
	FileName string
}

// downloadAndDecrypt downloads and decrypts a file from WeCom
func (a *Adapter) downloadAndDecrypt(fileURL, aesKey string) (*DownloadResult, error) {
	if fileURL == "" {
		return nil, fmt.Errorf("url is empty")
	}

	a.logger.Info("downloading file", slog.String("url", fileURL))

	// Download file
	resp, err := a.httpClient.Get(fileURL)
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

	// Extract filename from HTTP headers (Content-Disposition) - SDK compliant
	fileName := extractFileNameFromHeaders(resp.Header)
	if fileName == "" {
		// Fallback: extract from URL
		fileName = extractFileNameFromURL(fileURL)
	}

	a.logger.Info("file downloaded",
		slog.Int("size", len(data)),
		slog.String("filename", fileName))

	// If no AES key, return raw data
	if aesKey == "" {
		return &DownloadResult{
			Data:     data,
			FileName: fileName,
		}, nil
	}

	// Decrypt file
	decrypted, err := decryptFile(data, aesKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt file: %w", err)
	}

	a.logger.Info("file decrypted", slog.Int("decrypted_size", len(decrypted)))
	return &DownloadResult{
		Data:     decrypted,
		FileName: fileName,
	}, nil
}

// extractFileNameFromHeaders extracts filename from HTTP Content-Disposition header
// Follows RFC5987 for UTF-8 encoded filenames (SDK compliant)
func extractFileNameFromHeaders(header http.Header) string {
	contentDisposition := header.Get("Content-Disposition")
	if contentDisposition == "" {
		return ""
	}

	// Match filename*=UTF-8''xxx format (RFC5987)
	utf8Regex := regexp.MustCompile(`filename\*=UTF-8''([^;\s]+)`)
	if matches := utf8Regex.FindStringSubmatch(contentDisposition); matches != nil {
		filename, err := url.QueryUnescape(matches[1])
		if err == nil {
			return filepath.Base(filename)
		}
	}

	// Match filename="xxx" or filename=xxx format
	filenameRegex := regexp.MustCompile(`filename="?([^";\s]+)"?`)
	if matches := filenameRegex.FindStringSubmatch(contentDisposition); matches != nil {
		filename, err := url.QueryUnescape(matches[1])
		if err == nil {
			return filepath.Base(filename)
		}
		return filepath.Base(matches[1])
	}

	return ""
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

// extractFileNameFromURL extracts a filename from a URL.
// It tries to get the last path segment or returns a default name.
func extractFileNameFromURL(fileURL string) string {
	parsedURL, err := url.Parse(fileURL)
	if err != nil {
		return ""
	}
	// Get the last path segment
	path := parsedURL.Path
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	// Remove any query parameters from the base
	if idx := strings.Index(base, "?"); idx != -1 {
		base = base[:idx]
	}
	return base
}

// getMimeTypeFromFileName returns a MIME type based on the file extension.
func getMimeTypeFromFileName(fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".doc":
		return "application/msword"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".ppt":
		return "application/vnd.ms-powerpoint"
	case ".txt":
		return "text/plain"
	case ".csv":
		return "text/csv"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".html", ".htm":
		return "text/html"
	case ".md":
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}

// SendMessage 主动向指定会话发送消息
// 注意：需要用户先给机器人发消息，且受频率限制（30条/分钟，1000条/小时）
func (a *Adapter) SendMessage(ctx context.Context, cfg channel.ChannelConfig, chatID string, chatType int, content string) error {
	// 检查频率限制
	if !a.minuteLimiter.Allow() {
		return fmt.Errorf("send message rate limit exceeded: 30 messages per minute")
	}
	if !a.hourLimiter.Allow() {
		return fmt.Errorf("send message rate limit exceeded: 1000 messages per hour")
	}

	// 获取 WebSocket 客户端
	a.mu.RLock()
	wsClient, exists := a.clients[cfg.BotID]
	a.mu.RUnlock()

	if !exists || !wsClient.IsConnected() {
		return fmt.Errorf("websocket client not connected for bot %s", cfg.BotID)
	}

	// 生成新的 req_id 用于主动发送消息
	reqID := generateReqID(CmdSendMsg)

	// 构建消息体
	body := SendMarkdownMsgBody{
		MsgType: MsgTypeMarkdown,
		Markdown: MarkdownContent{
			Content: content,
		},
		ChatType: chatType,
	}

	// 使用 CmdSendMsg 命令发送
	if err := wsClient.SendReply(ctx, reqID, body, CmdSendMsg); err != nil {
		return fmt.Errorf("send message failed: %w", err)
	}

	a.logger.Info("message sent successfully",
		slog.String("chat_id", chatID),
		slog.Int("chat_type", chatType),
		slog.String("bot_id", cfg.BotID))

	return nil
}

// CheckRateLimit 检查当前是否超出频率限制
// 返回 (是否允许发送, 分钟限制是否允许, 小时限制是否允许)
func (a *Adapter) CheckRateLimit() (bool, bool, bool) {
	minuteAllowed := a.minuteLimiter.Allow()
	hourAllowed := a.hourLimiter.Allow()
	return minuteAllowed && hourAllowed, minuteAllowed, hourAllowed
}

// Ensure Adapter implements required interfaces
var _ channel.Adapter = (*Adapter)(nil)
var _ channel.Receiver = (*Adapter)(nil)
var _ channel.StreamSender = (*Adapter)(nil)
var _ channel.Sender = (*Adapter)(nil)
var _ channel.Connection = (*connection)(nil)
