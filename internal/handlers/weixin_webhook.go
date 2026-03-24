package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/Kxiandaoyan/Memoh-v2/internal/channel"
	"github.com/Kxiandaoyan/Memoh-v2/internal/db/sqlc"
	"github.com/Kxiandaoyan/Memoh-v2/internal/preauth"
)

// WeixinWebhookHandler handles WeChat personal (weixin) webhook endpoints for bridge integrations.
type WeixinWebhookHandler struct {
	processor      channel.InboundProcessor
	channelService *channel.Service
	preauthService *preauth.Service
	queries        *sqlc.Queries
}

// NewWeixinWebhookHandler creates a new WeChat personal webhook handler.
func NewWeixinWebhookHandler(
	processor channel.InboundProcessor,
	channelService *channel.Service,
	preauthService *preauth.Service,
	queries *sqlc.Queries,
) *WeixinWebhookHandler {
	return &WeixinWebhookHandler{
		processor:      processor,
		channelService: channelService,
		preauthService: preauthService,
		queries:        queries,
	}
}

// Register registers the WeChat personal webhook routes.
func (h *WeixinWebhookHandler) Register(e *echo.Echo) {
	group := e.Group("/channels/weixin/webhook")
	group.POST("/:botID", h.HandleWebhook)
	group.GET("/:botID/poll", h.PollReplies)
}

// WeixinWebhookRequest represents the incoming webhook payload from the weixin-bridge service.
type WeixinWebhookRequest struct {
	APIKey      string `json:"api_key"`
	Message     string `json:"message"`
	Sender      string `json:"sender"`
	SenderName  string `json:"sender_name"`
	ChatID      string `json:"chat_id"`
	ChatType    string `json:"chat_type"`
	MessageID   string `json:"message_id"`
}

// WeixinWebhookResponse represents the response sent back to the weixin-bridge.
type WeixinWebhookResponse struct {
	Success bool   `json:"success"`
	Reply   string `json:"reply,omitempty"`
	Error   string `json:"error,omitempty"`
}

// HandleWebhook processes incoming WeChat personal messages from the weixin-bridge service.
func (h *WeixinWebhookHandler) HandleWebhook(c echo.Context) error {
	botID := strings.TrimSpace(c.Param("botID"))
	if botID == "" {
		return c.JSON(http.StatusBadRequest, WeixinWebhookResponse{
			Success: false,
			Error:   "bot_id is required",
		})
	}

	var req WeixinWebhookRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, WeixinWebhookResponse{
			Success: false,
			Error:   "invalid request payload",
		})
	}

	if strings.TrimSpace(req.APIKey) == "" {
		return c.JSON(http.StatusBadRequest, WeixinWebhookResponse{
			Success: false,
			Error:   "api_key is required",
		})
	}
	if strings.TrimSpace(req.Message) == "" {
		return c.JSON(http.StatusBadRequest, WeixinWebhookResponse{
			Success: false,
			Error:   "message is required",
		})
	}

	ctx := c.Request().Context()
	key, err := h.preauthService.Get(ctx, req.APIKey)
	if err != nil {
		if errors.Is(err, preauth.ErrKeyNotFound) {
			return c.JSON(http.StatusUnauthorized, WeixinWebhookResponse{
				Success: false,
				Error:   "invalid api_key",
			})
		}
		return c.JSON(http.StatusInternalServerError, WeixinWebhookResponse{
			Success: false,
			Error:   "failed to verify api_key",
		})
	}

	if key.BotID != botID {
		return c.JSON(http.StatusUnauthorized, WeixinWebhookResponse{
			Success: false,
			Error:   "api_key does not match bot_id",
		})
	}

	cfg, err := h.channelService.ResolveEffectiveConfig(ctx, botID, channel.ChannelType("weixin"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, WeixinWebhookResponse{
			Success: false,
			Error:   "failed to resolve channel configuration",
		})
	}

	routeKey := generateWeixinRouteKey(botID, req.ChatID, req.ChatType, req.Sender)

	msg := channel.InboundMessage{
		Channel: channel.ChannelType("weixin"),
		Message: channel.Message{
			ID:   req.MessageID,
			Text: req.Message,
		},
		BotID:       botID,
		ReplyTarget: routeKey,
		RouteKey:    routeKey,
		Sender: channel.Identity{
			SubjectID:   req.Sender,
			DisplayName: req.SenderName,
			Attributes: map[string]string{
				"sender_id":   req.Sender,
				"sender_name": req.SenderName,
				"user_id":     key.IssuedByUserID,
			},
		},
		Conversation: channel.Conversation{
			ID:   req.ChatID,
			Type: normalizeWeixinChatType(req.ChatType),
			Metadata: map[string]any{
				"chat_type": req.ChatType,
			},
		},
		ReceivedAt: time.Now().UTC(),
		Source:     "weixin_webhook",
	}

	startCleanupLoop()

	taskID := uuid.New().String()

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		reply, err := h.processWeixinMessageSync(bgCtx, cfg, msg)
		if err != nil {
			slog.Error("weixin async processing failed",
				slog.String("bot_id", botID),
				slog.String("task_id", taskID),
				slog.Any("error", err),
			)
			return
		}

		pr := PendingReply{
			TaskID:  taskID,
			Reply:   reply,
			Sender:  req.Sender,
			ChatID:  req.ChatID,
			Created: time.Now(),
		}
		pendingMu.Lock()
		existing, _ := pendingReplies.Load(botID)
		var list []PendingReply
		if existing != nil {
			list = existing.([]PendingReply)
		}
		pendingReplies.Store(botID, append(list, pr))
		pendingMu.Unlock()
	}()

	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"task_id": taskID,
	})
}

// PollReplies returns and consumes all pending replies for a weixin bot.
func (h *WeixinWebhookHandler) PollReplies(c echo.Context) error {
	botID := strings.TrimSpace(c.Param("botID"))
	if botID == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"success": false, "error": "bot_id is required"})
	}

	apiKey := strings.TrimSpace(c.QueryParam("api_key"))
	if apiKey == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{"success": false, "error": "api_key is required"})
	}

	ctx := c.Request().Context()
	key, err := h.preauthService.Get(ctx, apiKey)
	if err != nil {
		if errors.Is(err, preauth.ErrKeyNotFound) {
			return c.JSON(http.StatusUnauthorized, map[string]any{"success": false, "error": "invalid api_key"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]any{"success": false, "error": "failed to verify api_key"})
	}
	if key.BotID != botID {
		return c.JSON(http.StatusUnauthorized, map[string]any{"success": false, "error": "api_key does not match bot_id"})
	}

	pendingMu.Lock()
	var messages []PendingReply
	if val, ok := pendingReplies.LoadAndDelete(botID); ok {
		messages = val.([]PendingReply)
	}
	pendingMu.Unlock()

	if messages == nil {
		messages = []PendingReply{}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"success":  true,
		"messages": messages,
	})
}

// processWeixinMessageSync processes the inbound message and waits for the complete AI reply.
func (h *WeixinWebhookHandler) processWeixinMessageSync(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage) (string, error) {
	if h.processor == nil {
		return "", fmt.Errorf("processor not configured")
	}

	collector := &replyCollector{
		done:  make(chan struct{}),
		mutex: &sync.Mutex{},
	}

	sender := &weixinSyncReplySender{
		collector: collector,
	}

	go func() {
		if err := h.processor.HandleInbound(ctx, cfg, msg, sender); err != nil {
			collector.mutex.Lock()
			collector.err = err
			collector.mutex.Unlock()
			select {
			case <-collector.done:
			default:
				close(collector.done)
			}
			return
		}
		select {
		case <-collector.done:
		default:
			close(collector.done)
		}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-collector.done:
		collector.mutex.Lock()
		defer collector.mutex.Unlock()
		if collector.err != nil {
			return "", collector.err
		}
		return collector.reply, nil
	}
}

// weixinSyncReplySender captures the final reply from the AI processor.
type weixinSyncReplySender struct {
	collector *replyCollector
}

func (s *weixinSyncReplySender) Send(_ context.Context, msg channel.OutboundMessage) error {
	s.collector.mutex.Lock()
	defer s.collector.mutex.Unlock()

	s.collector.reply = msg.Message.PlainText()

	select {
	case <-s.collector.done:
	default:
		close(s.collector.done)
	}
	return nil
}

func (s *weixinSyncReplySender) OpenStream(_ context.Context, _ string, _ channel.StreamOptions) (channel.OutboundStream, error) {
	return &weixinSyncOutboundStream{collector: s.collector}, nil
}

// weixinSyncOutboundStream accumulates streaming events into a final reply.
type weixinSyncOutboundStream struct {
	collector *replyCollector
}

func (s *weixinSyncOutboundStream) Push(_ context.Context, event channel.StreamEvent) error {
	s.collector.mutex.Lock()
	defer s.collector.mutex.Unlock()

	switch event.Type {
	case channel.StreamEventDelta:
		s.collector.reply += event.Delta
	case channel.StreamEventFinal:
		if event.Final != nil && !event.Final.Message.IsEmpty() {
			s.collector.reply = event.Final.Message.PlainText()
		}
	case channel.StreamEventError:
		s.collector.err = fmt.Errorf("stream error: %s", event.Error)
	}
	return nil
}

func (s *weixinSyncOutboundStream) Close(_ context.Context) error {
	s.collector.mutex.Lock()
	defer s.collector.mutex.Unlock()

	select {
	case <-s.collector.done:
	default:
		close(s.collector.done)
	}
	return nil
}

// generateWeixinRouteKey creates a routing key for WeChat personal conversations.
func generateWeixinRouteKey(botID, chatID, chatType, senderID string) string {
	normalized := normalizeWeixinChatType(chatType)
	if normalized == "group" {
		return fmt.Sprintf("weixin:%s:%s:%s", botID, chatID, senderID)
	}
	return fmt.Sprintf("weixin:%s:%s", botID, chatID)
}

// normalizeWeixinChatType converts chat type to standard format.
func normalizeWeixinChatType(chatType string) string {
	ct := strings.ToLower(strings.TrimSpace(chatType))
	switch ct {
	case "group", "chatroom":
		return "group"
	default:
		return "private"
	}
}

