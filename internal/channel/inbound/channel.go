package inbound

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"

	"encoding/base64"
	"os"
	"path/filepath"

	"github.com/Kxiandaoyan/Memoh-v2/internal/auth"
	"github.com/Kxiandaoyan/Memoh-v2/internal/channel"
	"github.com/Kxiandaoyan/Memoh-v2/internal/channel/route"
	"github.com/Kxiandaoyan/Memoh-v2/internal/conversation"
	"github.com/Kxiandaoyan/Memoh-v2/internal/conversation/flow"
	"github.com/Kxiandaoyan/Memoh-v2/internal/fileparse"
	messagepkg "github.com/Kxiandaoyan/Memoh-v2/internal/message"
)

const (
	silentReplyToken        = "NO_REPLY"
	minDuplicateTextLength  = 10
	processingStatusTimeout = 60 * time.Second
)

var (
	whitespacePattern = regexp.MustCompile(`\s+`)
)

// RouteResolver resolves and manages channel routes.
type RouteResolver interface {
	ResolveConversation(ctx context.Context, input route.ResolveInput) (route.ResolveConversationResult, error)
}

// Broadcaster sends an outbound message to a specific channel.
type Broadcaster interface {
	Send(ctx context.Context, botID string, channelType channel.ChannelType, req channel.SendRequest) error
}

// RouteLister lists all routes for a bot.
type RouteLister interface {
	List(ctx context.Context, conversationID string) ([]route.Route, error)
}

// ChannelInboundProcessor routes channel inbound messages to the chat gateway.
type ChannelInboundProcessor struct {
	runner         flow.Runner
	routeResolver  RouteResolver
	message        messagepkg.Writer
	registry       *channel.Registry
	logger         *slog.Logger
	jwtSecret      string
	tokenTTL       time.Duration
	identity       *IdentityResolver
	policyService  PolicyService
	groupDebouncer *messagepkg.GroupDebouncer
	broadcaster    Broadcaster
	routeLister    RouteLister
	dataRoot       string // Data root path where files are actually saved (e.g., /opt/memoh/data)
}

// NewChannelInboundProcessor creates a processor with channel identity-based resolution.
// dataRoot is the actual path where files are saved (e.g., /opt/memoh/data).
// Bot containers access files via /shared which is mounted from {dataRoot}/shared.
func NewChannelInboundProcessor(
	log *slog.Logger,
	registry *channel.Registry,
	routeResolver RouteResolver,
	messageWriter messagepkg.Writer,
	runner flow.Runner,
	channelIdentityService ChannelIdentityService,
	memberService BotMemberService,
	policyService PolicyService,
	preauthService PreauthService,
	bindService BindService,
	jwtSecret string,
	tokenTTL time.Duration,
	dataRoot string,
) *ChannelInboundProcessor {
	if log == nil {
		log = slog.Default()
	}
	if tokenTTL <= 0 {
		tokenTTL = 5 * time.Minute
	}
	if dataRoot == "" {
		dataRoot = "/opt/memoh/data"
	}
	identityResolver := NewIdentityResolver(log, registry, channelIdentityService, memberService, policyService, preauthService, bindService, "", "")
	return &ChannelInboundProcessor{
		runner:        runner,
		routeResolver: routeResolver,
		message:       messageWriter,
		registry:      registry,
		logger:        log.With(slog.String("component", "channel_router")),
		jwtSecret:     strings.TrimSpace(jwtSecret),
		tokenTTL:      tokenTTL,
		identity:      identityResolver,
		policyService: policyService,
		dataRoot:      dataRoot,
	}
}

// IdentityMiddleware returns the identity resolution middleware.
// SetGroupDebouncer enables group message debouncing. When set, messages
// from non-DM conversations are buffered for the debounce window and merged
// into a single agent invocation. Pass nil to disable.
func (p *ChannelInboundProcessor) SetGroupDebouncer(d *messagepkg.GroupDebouncer) {
	p.groupDebouncer = d
}

// SetBroadcaster enables cross-channel broadcast of assistant replies.
func (p *ChannelInboundProcessor) SetBroadcaster(b Broadcaster, rl RouteLister) {
	p.broadcaster = b
	p.routeLister = rl
}

func (p *ChannelInboundProcessor) IdentityMiddleware() channel.Middleware {
	if p == nil || p.identity == nil {
		return nil
	}
	return p.identity.Middleware()
}

// HandleInbound processes an inbound channel message through identity resolution and chat gateway.
func (p *ChannelInboundProcessor) HandleInbound(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, sender channel.StreamReplySender) error {
	if p.runner == nil {
		return fmt.Errorf("channel inbound processor not configured")
	}
	if sender == nil {
		return fmt.Errorf("reply sender not configured")
	}
	text := buildInboundQuery(msg.Message)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	state, err := p.requireIdentity(ctx, cfg, msg)
	if err != nil {
		return err
	}
	if state.Decision != nil && state.Decision.Stop {
		if !state.Decision.Reply.IsEmpty() {
			return sender.Send(ctx, channel.OutboundMessage{
				Target:  strings.TrimSpace(msg.ReplyTarget),
				Message: state.Decision.Reply,
			})
		}
		if p.logger != nil {
			p.logger.Info(
				"inbound dropped by identity policy (no reply sent)",
				slog.String("channel", msg.Channel.String()),
				slog.String("bot_id", strings.TrimSpace(state.Identity.BotID)),
				slog.String("conversation_type", strings.TrimSpace(msg.Conversation.Type)),
				slog.String("conversation_id", strings.TrimSpace(msg.Conversation.ID)),
			)
		}
		return nil
	}

	identity := state.Identity

	// Resolve or create the route via channel_routes.
	if p.routeResolver == nil {
		return fmt.Errorf("route resolver not configured")
	}
	resolved, err := p.routeResolver.ResolveConversation(ctx, route.ResolveInput{
		BotID:             identity.BotID,
		Platform:          msg.Channel.String(),
		ConversationID:    msg.Conversation.ID,
		ThreadID:          extractThreadID(msg),
		ConversationType:  msg.Conversation.Type,
		ChannelIdentityID: identity.UserID,
		ChannelConfigID:   identity.ChannelConfigID,
		ReplyTarget:       strings.TrimSpace(msg.ReplyTarget),
	})
	if err != nil {
		return fmt.Errorf("resolve route conversation: %w", err)
	}
	// Bot-centric history container:
	// always persist channel traffic under bot_id so WebUI can view unified cross-platform history.
	activeChatID := strings.TrimSpace(identity.BotID)
	if activeChatID == "" {
		activeChatID = strings.TrimSpace(resolved.ChatID)
	}
	groupRequireMention := true
	if p.policyService != nil && strings.TrimSpace(identity.BotID) != "" {
		if val, err := p.policyService.GroupRequireMention(ctx, identity.BotID); err == nil {
			groupRequireMention = val
		}
	}
	if !shouldTriggerAssistantResponse(msg, groupRequireMention) && !identity.ForceReply {
		if p.logger != nil {
			p.logger.Info(
				"inbound not triggering assistant (group trigger condition not met)",
				slog.String("channel", msg.Channel.String()),
				slog.String("bot_id", strings.TrimSpace(identity.BotID)),
				slog.String("route_id", strings.TrimSpace(resolved.RouteID)),
				slog.Bool("is_mentioned", metadataBool(msg.Metadata, "is_mentioned")),
				slog.Bool("is_reply_to_bot", metadataBool(msg.Metadata, "is_reply_to_bot")),
				slog.Bool("group_require_mention", groupRequireMention),
				slog.String("conversation_type", strings.TrimSpace(msg.Conversation.Type)),
			)
		}
		p.persistInboundUser(ctx, resolved.RouteID, identity, msg, text, "passive_sync")
		return nil
	}
	userMessagePersisted := p.persistInboundUser(ctx, resolved.RouteID, identity, msg, text, "active_chat")

	// For group conversations with the debouncer enabled, buffer the message text
	// and fire the agent with a merged query after the window expires.
	if p.groupDebouncer != nil && !isDirectConversationType(msg.Conversation.Type) {
		// Use the actual conversation ID (group chatID or single-chat userID) rather than botID,
		// so messages from different groups/users are debounced in separate buckets.
		debounceKey := strings.TrimSpace(identity.BotID) + ":" + strings.TrimSpace(msg.Conversation.ID)
		capturedMsg := msg
		capturedCfg := cfg
		capturedSender := sender
		capturedIdentity := identity
		capturedResolved := resolved
		capturedActiveChatID := activeChatID
		capturedUserMsgPersisted := userMessagePersisted

		// Read per-bot debounce window from bot metadata (group_debounce_ms).
		// Falls back to the debouncer's global default when not set.
		var debounceWindow time.Duration
		if p.policyService != nil && strings.TrimSpace(identity.BotID) != "" {
			if w, err := p.policyService.GroupDebounceWindow(ctx, identity.BotID); err == nil {
				debounceWindow = w
			}
		}

		p.groupDebouncer.SubmitWithWindow(debounceKey, text, debounceWindow, func(mergedText string) {
			go func() {
				bgCtx := context.Background()
				_ = p.dispatchGroupChat(bgCtx, capturedCfg, capturedMsg, mergedText, capturedSender,
					capturedIdentity, capturedResolved, capturedActiveChatID, capturedUserMsgPersisted)
			}()
		})
		return nil
	}

	// Issue chat token for reply routing.
	chatToken := ""
	if p.jwtSecret != "" && strings.TrimSpace(msg.ReplyTarget) != "" {
		signed, _, err := auth.GenerateChatToken(auth.ChatToken{
			BotID:             identity.BotID,
			ChatID:            activeChatID,
			RouteID:           resolved.RouteID,
			UserID:            identity.UserID,
			ChannelIdentityID: identity.ChannelIdentityID,
		}, p.jwtSecret, p.tokenTTL)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("issue chat token failed", slog.Any("error", err))
			}
		} else {
			chatToken = signed
		}
	}

	// Issue user JWT for downstream calls (MCP, schedule, etc.). For guests use chat token as Bearer.
	token := ""
	if identity.UserID != "" && p.jwtSecret != "" {
		signed, _, err := auth.GenerateToken(identity.UserID, p.jwtSecret, p.tokenTTL)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("issue channel token failed", slog.Any("error", err))
			}
		} else {
			token = "Bearer " + signed
		}
	}
	if token == "" && chatToken != "" {
		token = "Bearer " + chatToken
	}

	var desc channel.Descriptor
	if p.registry != nil {
		desc, _ = p.registry.GetDescriptor(msg.Channel) //nolint:errcheck // descriptor lookup is best-effort
	}
	statusInfo := channel.ProcessingStatusInfo{
		BotID:             identity.BotID,
		ChatID:            activeChatID,
		RouteID:           resolved.RouteID,
		ChannelIdentityID: identity.ChannelIdentityID,
		UserID:            identity.UserID,
		Query:             text,
		ReplyTarget:       strings.TrimSpace(msg.ReplyTarget),
		SourceMessageID:   strings.TrimSpace(msg.Message.ID),
	}
	statusNotifier := p.resolveProcessingStatusNotifier(msg.Channel)
	statusHandle := channel.ProcessingStatusHandle{}
	if statusNotifier != nil {
		handle, notifyErr := p.notifyProcessingStarted(ctx, statusNotifier, cfg, msg, statusInfo)
		if notifyErr != nil {
			p.logProcessingStatusError("processing_started", msg, identity, notifyErr)
		} else {
			statusHandle = handle
		}
	}
	target := strings.TrimSpace(msg.ReplyTarget)
	if target == "" {
		err := fmt.Errorf("reply target missing")
		if statusNotifier != nil {
			if notifyErr := p.notifyProcessingFailed(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle, err); notifyErr != nil {
				p.logProcessingStatusError("processing_failed", msg, identity, notifyErr)
			}
		}
		return err
	}
	sourceMessageID := strings.TrimSpace(msg.Message.ID)
	replyRef := &channel.ReplyRef{Target: target}
	if sourceMessageID != "" {
		replyRef.MessageID = sourceMessageID
	}
	// Build metadata for stream options, including req_id for WeCom and other channels
	streamMetadata := map[string]any{
		"route_id": resolved.RouteID,
	}
	if msg.Metadata != nil {
		if v, ok := msg.Metadata["req_id"].(string); ok && v != "" {
			streamMetadata["req_id"] = v
		}
	}
	stream, err := sender.OpenStream(ctx, target, channel.StreamOptions{
		Reply:           replyRef,
		SourceMessageID: sourceMessageID,
		Metadata:        streamMetadata,
		ReceivedAt:      msg.ReceivedAt,
	})
	if err != nil {
		if statusNotifier != nil {
			if notifyErr := p.notifyProcessingFailed(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle, err); notifyErr != nil {
				p.logProcessingStatusError("processing_failed", msg, identity, notifyErr)
			}
		}
		return err
	}
	defer func() {
		_ = stream.Close(context.WithoutCancel(ctx))
	}()

	if err := stream.Push(ctx, channel.StreamEvent{
		Type:   channel.StreamEventStatus,
		Status: channel.StreamStatusStarted,
	}); err != nil {
		if statusNotifier != nil {
			if notifyErr := p.notifyProcessingFailed(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle, err); notifyErr != nil {
				p.logProcessingStatusError("processing_failed", msg, identity, notifyErr)
			}
		}
		return err
	}

	// Build input attachments and extract file content for supported types
	if p.logger != nil && len(msg.Message.Attachments) > 0 {
		p.logger.Info("processing message attachments", slog.Int("count", len(msg.Message.Attachments)))
		for i, att := range msg.Message.Attachments {
			p.logger.Info("attachment details", slog.Int("index", i), slog.String("type", string(att.Type)), slog.String("name", att.Name), slog.String("mime", att.Mime), slog.Int("dataSize", len(att.Data)))
		}
	}
	inputAttachments, fileContext := buildInputAttachments(msg.Message.Attachments, p.dataRoot, p.logger)
	if fileContext != "" {
		if p.logger != nil {
			origText := text
			if len(origText) > 100 {
				origText = origText[:100] + "..."
			}
			p.logger.Info("adding file context to query", slog.Int("contextLength", len(fileContext)), slog.String("originalText", origText))
		}
		text = text + fileContext
		if p.logger != nil {
			p.logger.Info("query with file context", slog.Int("totalLength", len(text)))
		}
	}

	chunkCh, streamErrCh := p.runner.StreamChat(ctx, conversation.ChatRequest{
		BotID:                   identity.BotID,
		ChatID:                  activeChatID,
		Token:                   token,
		UserID:                  identity.UserID,
		SourceChannelIdentityID: identity.ChannelIdentityID,
		DisplayName:             identity.DisplayName,
		RouteID:                 resolved.RouteID,
		ChatToken:               chatToken,
		ExternalMessageID:       sourceMessageID,
		ConversationType:        msg.Conversation.Type,
		ReplyTarget:             strings.TrimSpace(msg.ReplyTarget),
		Query:                   text,
		CurrentChannel:          msg.Channel.String(),
		Channels:                []string{msg.Channel.String()},
		UserMessagePersisted:    userMessagePersisted,
		InputAttachments:        inputAttachments,
	})

	var (
		finalMessages  []conversation.ModelMessage
		streamErr      error
		collectedUsage *gatewayUsage
	)
	for chunkCh != nil || streamErrCh != nil {
		select {
		case chunk, ok := <-chunkCh:
			if !ok {
				chunkCh = nil
				continue
			}
			events, messages, usage, parseErr := mapStreamChunkToChannelEvents(chunk)
			if parseErr != nil {
				if p.logger != nil {
					p.logger.Warn(
						"stream chunk parse failed",
						slog.String("channel", msg.Channel.String()),
						slog.String("channel_identity_id", identity.ChannelIdentityID),
						slog.String("user_id", identity.UserID),
						slog.Any("error", parseErr),
					)
				}
				continue
			}
			// Collect the latest usage data (prefer agent_end/done events)
			if usage != nil && usage.TotalTokens > 0 {
				collectedUsage = usage
			}
			for _, event := range events {
				if pushErr := stream.Push(ctx, event); pushErr != nil {
					streamErr = pushErr
					break
				}
			}
			if len(messages) > 0 {
				finalMessages = messages
			}
		case err, ok := <-streamErrCh:
			if !ok {
				streamErrCh = nil
				continue
			}
			if err != nil {
				streamErr = err
			}
		}
		if streamErr != nil {
			break
		}
	}

	if streamErr != nil {
		if p.logger != nil {
			p.logger.Error(
				"chat gateway stream failed",
				slog.String("channel", msg.Channel.String()),
				slog.String("channel_identity_id", identity.ChannelIdentityID),
				slog.String("user_id", identity.UserID),
				slog.Any("error", streamErr),
			)
		}
		_ = stream.Push(ctx, channel.StreamEvent{
			Type:  channel.StreamEventError,
			Error: streamErr.Error(),
		})
		if statusNotifier != nil {
			if notifyErr := p.notifyProcessingFailed(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle, streamErr); notifyErr != nil {
				p.logProcessingStatusError("processing_failed", msg, identity, notifyErr)
			}
		}
		return streamErr
	}

	sentTexts, suppressReplies := collectMessageToolContext(p.registry, finalMessages, msg.Channel, target)
	if suppressReplies {
		if err := stream.Push(ctx, channel.StreamEvent{
			Type:   channel.StreamEventStatus,
			Status: channel.StreamStatusCompleted,
		}); err != nil {
			return err
		}
		if statusNotifier != nil {
			if notifyErr := p.notifyProcessingCompleted(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle); notifyErr != nil {
				p.logProcessingStatusError("processing_completed", msg, identity, notifyErr)
			}
		}
		return nil
	}

	outputs := flow.ExtractAssistantOutputs(finalMessages)
	for i, output := range outputs {
		outMessage := buildChannelMessage(output, desc.Capabilities)
		if outMessage.IsEmpty() {
			continue
		}
		plainText := strings.TrimSpace(outMessage.PlainText())
		if isSilentReplyText(plainText) {
			continue
		}
		if isMessagingToolDuplicate(plainText, sentTexts) {
			continue
		}
		// Append token usage to the last assistant message
		isLastOutput := i == len(outputs)-1
		if isLastOutput && collectedUsage != nil && collectedUsage.TotalTokens > 0 {
			tokenText := formatTokenUsage(collectedUsage)
			if tokenText != "" {
				outMessage.Text = strings.TrimSpace(outMessage.Text) + "\n\n" + tokenText
			}
		}
		if outMessage.Reply == nil && sourceMessageID != "" {
			outMessage.Reply = &channel.ReplyRef{
				Target:    target,
				MessageID: sourceMessageID,
			}
		}
		if err := stream.Push(ctx, channel.StreamEvent{
			Type: channel.StreamEventFinal,
			Final: &channel.StreamFinalizePayload{
				Message: outMessage,
			},
		}); err != nil {
			return err
		}
	}
	if err := stream.Push(ctx, channel.StreamEvent{
		Type:   channel.StreamEventStatus,
		Status: channel.StreamStatusCompleted,
	}); err != nil {
		return err
	}
	if statusNotifier != nil {
		if notifyErr := p.notifyProcessingCompleted(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle); notifyErr != nil {
			p.logProcessingStatusError("processing_completed", msg, identity, notifyErr)
		}
	}
	// Broadcast disabled: bot-centric chatID (= botID) causes ListChatRoutes to return ALL routes
	// for this bot, which would mirror every reply to every other user/group. Each route must
	// receive only its own response. Cross-platform sync should be re-implemented with proper
	// per-conversation scoping before re-enabling.
	// go p.broadcastToOtherChannels(identity.BotID, activeChatID, strings.ToLower(msg.Channel.String()), target, outputs)
	return nil
}

func shouldTriggerAssistantResponse(msg channel.InboundMessage, groupRequireMention bool) bool {
	if isDirectConversationType(msg.Conversation.Type) {
		return true
	}
	// Explicit @mention or direct reply always triggers, regardless of settings.
	if metadataBool(msg.Metadata, "is_mentioned") {
		return true
	}
	if metadataBool(msg.Metadata, "is_reply_to_bot") {
		return true
	}
	if hasCommandPrefix(msg.Message.PlainText(), msg.Metadata) {
		return true
	}
	if !groupRequireMention {
		// Respond to all human messages, but skip other bots' messages
		// to prevent infinite response loops.
		return !metadataBool(msg.Metadata, "is_from_bot")
	}
	return false
}

func isDirectConversationType(conversationType string) bool {
	ct := strings.ToLower(strings.TrimSpace(conversationType))
	return ct == "" || ct == "p2p" || ct == "private" || ct == "direct" || ct == "single"
}

func hasCommandPrefix(text string, metadata map[string]any) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	prefixes := []string{"/"}
	if metadata != nil {
		if raw, ok := metadata["command_prefix"]; ok {
			if value := strings.TrimSpace(fmt.Sprint(raw)); value != "" {
				prefixes = []string{value}
			}
		}
		if raw, ok := metadata["command_prefixes"]; ok {
			if parsed := parseCommandPrefixes(raw); len(parsed) > 0 {
				prefixes = parsed
			}
		}
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func parseCommandPrefixes(raw any) []string {
	if items, ok := raw.([]string); ok {
		result := make([]string, 0, len(items))
		for _, item := range items {
			value := strings.TrimSpace(item)
			if value == "" {
				continue
			}
			result = append(result, value)
		}
		return result
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(fmt.Sprint(item))
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	return result
}

func metadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	raw, ok := metadata[key]
	if !ok {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func (p *ChannelInboundProcessor) persistInboundUser(ctx context.Context, routeID string, identity InboundIdentity, msg channel.InboundMessage, query string, triggerMode string) bool {
	if p.message == nil {
		return false
	}
	botID := strings.TrimSpace(identity.BotID)
	if botID == "" {
		return false
	}
	payload, err := json.Marshal(conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(query),
	})
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("marshal inbound user message failed", slog.Any("error", err))
		}
		return false
	}
	meta := map[string]any{
		"route_id":     strings.TrimSpace(routeID),
		"platform":     msg.Channel.String(),
		"trigger_mode": strings.TrimSpace(triggerMode),
	}
	if _, err := p.message.Persist(ctx, messagepkg.PersistInput{
		BotID:                   botID,
		RouteID:                 strings.TrimSpace(routeID),
		SenderChannelIdentityID: strings.TrimSpace(identity.ChannelIdentityID),
		SenderUserID:            strings.TrimSpace(identity.UserID),
		Platform:                msg.Channel.String(),
		ExternalMessageID:       strings.TrimSpace(msg.Message.ID),
		Role:                    "user",
		Content:                 payload,
		Metadata:                meta,
	}); err != nil && p.logger != nil {
		p.logger.Warn("persist inbound user message failed", slog.Any("error", err))
		return false
	}
	return true
}

func buildChannelMessage(output conversation.AssistantOutput, capabilities channel.ChannelCapabilities) channel.Message {
	msg := channel.Message{}
	if strings.TrimSpace(output.Content) != "" {
		msg.Text = strings.TrimSpace(output.Content)
		if containsMarkdown(msg.Text) && (capabilities.Markdown || capabilities.RichText) {
			msg.Format = channel.MessageFormatMarkdown
		}
	}
	if len(output.Parts) == 0 {
		return msg
	}
	if capabilities.RichText {
		parts := make([]channel.MessagePart, 0, len(output.Parts))
		for _, part := range output.Parts {
			if !contentPartHasValue(part) {
				continue
			}
			partType := normalizeContentPartType(part.Type)
			parts = append(parts, channel.MessagePart{
				Type:              partType,
				Text:              part.Text,
				URL:               part.URL,
				Styles:            normalizeContentPartStyles(part.Styles),
				Language:          part.Language,
				ChannelIdentityID: part.ChannelIdentityID,
				Emoji:             part.Emoji,
			})
		}
		if len(parts) > 0 {
			msg.Parts = parts
			msg.Format = channel.MessageFormatRich
		}
		return msg
	}
	textParts := make([]string, 0, len(output.Parts))
	for _, part := range output.Parts {
		if !contentPartHasValue(part) {
			continue
		}
		textParts = append(textParts, strings.TrimSpace(contentPartText(part)))
	}
	if len(textParts) > 0 {
		msg.Text = strings.Join(textParts, "\n")
		if msg.Format == "" && containsMarkdown(msg.Text) && (capabilities.Markdown || capabilities.RichText) {
			msg.Format = channel.MessageFormatMarkdown
		}
	}
	return msg
}

func containsMarkdown(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	patterns := []string{
		`\\*\\*[^*]+\\*\\*`,
		`\\*[^*]+\\*`,
		`~~[^~]+~~`,
		"`[^`]+`",
		"```[\\s\\S]*```",
		`\\[.+\\]\\(.+\\)`,
		`(?m)^#{1,6}\\s`,
		`(?m)^[-*]\\s`,
		`(?m)^\\d+\\.\\s`,
	}
	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			return true
		}
	}
	return false
}

func contentPartHasValue(part conversation.ContentPart) bool {
	if strings.TrimSpace(part.Text) != "" {
		return true
	}
	if strings.TrimSpace(part.URL) != "" {
		return true
	}
	if strings.TrimSpace(part.Emoji) != "" {
		return true
	}
	return false
}

func contentPartText(part conversation.ContentPart) string {
	if strings.TrimSpace(part.Text) != "" {
		return part.Text
	}
	if strings.TrimSpace(part.URL) != "" {
		return part.URL
	}
	if strings.TrimSpace(part.Emoji) != "" {
		return part.Emoji
	}
	return ""
}

type gatewayStreamEnvelope struct {
	Type     string                      `json:"type"`
	Delta    string                      `json:"delta"`
	Error    string                      `json:"error"`
	Message  string                      `json:"message"`
	Data     json.RawMessage             `json:"data"`
	Messages []conversation.ModelMessage `json:"messages"`
	Usage    *gatewayUsage              `json:"usage,omitempty"`
}

type gatewayUsage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

type gatewayStreamDoneData struct {
	Messages []conversation.ModelMessage `json:"messages"`
}

func mapStreamChunkToChannelEvents(chunk conversation.StreamChunk) ([]channel.StreamEvent, []conversation.ModelMessage, *gatewayUsage, error) {
	if len(chunk) == 0 {
		return nil, nil, nil, nil
	}
	var envelope gatewayStreamEnvelope
	if err := json.Unmarshal(chunk, &envelope); err != nil {
		return nil, nil, nil, err
	}
	finalMessages := make([]conversation.ModelMessage, 0, len(envelope.Messages))
	finalMessages = append(finalMessages, envelope.Messages...)
	if len(finalMessages) == 0 && len(envelope.Data) > 0 {
		var done gatewayStreamDoneData
		if err := json.Unmarshal(envelope.Data, &done); err == nil && len(done.Messages) > 0 {
			finalMessages = append(finalMessages, done.Messages...)
		}
	}
	eventType := strings.ToLower(strings.TrimSpace(envelope.Type))
	switch eventType {
	case "text_delta":
		if envelope.Delta == "" {
			return nil, finalMessages, envelope.Usage, nil
		}
		return []channel.StreamEvent{
			{
				Type:  channel.StreamEventDelta,
				Delta: envelope.Delta,
			},
		}, finalMessages, envelope.Usage, nil
	case "reasoning_delta":
		if envelope.Delta == "" {
			return nil, finalMessages, envelope.Usage, nil
		}
		return []channel.StreamEvent{
			{
				Type:  channel.StreamEventDelta,
				Delta: envelope.Delta,
				Metadata: map[string]any{
					"phase": "reasoning",
				},
			},
		}, finalMessages, envelope.Usage, nil
	case "error":
		streamError := strings.TrimSpace(envelope.Error)
		if streamError == "" {
			streamError = strings.TrimSpace(envelope.Message)
		}
		if streamError == "" {
			streamError = "stream error"
		}
		return []channel.StreamEvent{
			{
				Type:  channel.StreamEventError,
				Error: streamError,
			},
		}, finalMessages, envelope.Usage, nil
	default:
		return nil, finalMessages, envelope.Usage, nil
	}
}

func buildInboundQuery(message channel.Message) string {
	text := strings.TrimSpace(message.PlainText())
	if len(message.Attachments) == 0 {
		return text
	}
	lines := make([]string, 0, len(message.Attachments)+1)
	if text != "" {
		lines = append(lines, text)
	}
	for _, att := range message.Attachments {
		label := strings.TrimSpace(att.Name)
		if label == "" {
			label = strings.TrimSpace(att.Reference())
		}
		if label == "" {
			label = "unknown"
		}
		lines = append(lines, fmt.Sprintf("[attachment:%s] %s", att.Type, label))
	}
	return strings.Join(lines, "\n")
}

func normalizeContentPartType(raw string) channel.MessagePartType {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "link":
		return channel.MessagePartLink
	case "code_block":
		return channel.MessagePartCodeBlock
	case "mention":
		return channel.MessagePartMention
	case "emoji":
		return channel.MessagePartEmoji
	default:
		return channel.MessagePartText
	}
}

func normalizeContentPartStyles(styles []string) []channel.MessageTextStyle {
	if len(styles) == 0 {
		return nil
	}
	result := make([]channel.MessageTextStyle, 0, len(styles))
	for _, style := range styles {
		switch strings.TrimSpace(strings.ToLower(style)) {
		case "bold":
			result = append(result, channel.MessageStyleBold)
		case "italic":
			result = append(result, channel.MessageStyleItalic)
		case "strikethrough", "lineThrough":
			result = append(result, channel.MessageStyleStrikethrough)
		case "code":
			result = append(result, channel.MessageStyleCode)
		default:
			continue
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

type sendMessageToolArgs struct {
	Platform          string           `json:"platform"`
	Target            string           `json:"target"`
	ChannelIdentityID string           `json:"channel_identity_id"`
	Text              string           `json:"text"`
	Message           *channel.Message `json:"message"`
}

func collectMessageToolContext(registry *channel.Registry, messages []conversation.ModelMessage, channelType channel.ChannelType, replyTarget string) ([]string, bool) {
	if len(messages) == 0 {
		return nil, false
	}
	var sentTexts []string
	suppressReplies := false
	for _, msg := range messages {
		for _, tc := range msg.ToolCalls {
			if tc.Function.Name != "send" && tc.Function.Name != "send_message" {
				continue
			}
			var args sendMessageToolArgs
			if !parseToolArguments(tc.Function.Arguments, &args) {
				continue
			}
			if text := strings.TrimSpace(extractSendMessageText(args)); text != "" {
				sentTexts = append(sentTexts, text)
			}
			if shouldSuppressForToolCall(registry, args, channelType, replyTarget) {
				suppressReplies = true
			}
		}
		// Vercel AI SDK format: tool-call parts inside content array
		if msg.Role == "assistant" && len(msg.Content) > 0 {
			var parts []struct {
				Type     string          `json:"type"`
				ToolName string          `json:"toolName"`
				Args     json.RawMessage `json:"args"`
			}
			if err := json.Unmarshal(msg.Content, &parts); err == nil {
				for _, p := range parts {
					if p.Type != "tool-call" || (p.ToolName != "send" && p.ToolName != "send_message") {
						continue
					}
					var args sendMessageToolArgs
					if json.Unmarshal(p.Args, &args) != nil {
						continue
					}
					if text := strings.TrimSpace(extractSendMessageText(args)); text != "" {
						sentTexts = append(sentTexts, text)
					}
					if shouldSuppressForToolCall(registry, args, channelType, replyTarget) {
						suppressReplies = true
					}
				}
			}
		}
	}
	return sentTexts, suppressReplies
}

func parseToolArguments(raw string, out any) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	if err := json.Unmarshal([]byte(raw), out); err == nil {
		return true
	}
	var decoded string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return false
	}
	if strings.TrimSpace(decoded) == "" {
		return false
	}
	return json.Unmarshal([]byte(decoded), out) == nil
}

func extractSendMessageText(args sendMessageToolArgs) string {
	if strings.TrimSpace(args.Text) != "" {
		return strings.TrimSpace(args.Text)
	}
	if args.Message == nil {
		return ""
	}
	return strings.TrimSpace(args.Message.PlainText())
}

func shouldSuppressForToolCall(registry *channel.Registry, args sendMessageToolArgs, channelType channel.ChannelType, replyTarget string) bool {
	platform := strings.TrimSpace(args.Platform)
	if platform == "" {
		platform = string(channelType)
	}
	if !strings.EqualFold(platform, string(channelType)) {
		return false
	}
	target := strings.TrimSpace(args.Target)
	if target == "" && strings.TrimSpace(args.ChannelIdentityID) == "" {
		target = replyTarget
	}
	if strings.TrimSpace(target) == "" || strings.TrimSpace(replyTarget) == "" {
		return false
	}
	normalizedTarget := normalizeReplyTarget(registry, channelType, target)
	normalizedReply := normalizeReplyTarget(registry, channelType, replyTarget)
	if normalizedTarget == "" || normalizedReply == "" {
		return false
	}
	return normalizedTarget == normalizedReply
}

func normalizeReplyTarget(registry *channel.Registry, channelType channel.ChannelType, target string) string {
	if registry == nil {
		return strings.TrimSpace(target)
	}
	normalized, ok := registry.NormalizeTarget(channelType, target)
	if ok && strings.TrimSpace(normalized) != "" {
		return strings.TrimSpace(normalized)
	}
	return strings.TrimSpace(target)
}

func isSilentReplyText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	token := []rune(silentReplyToken)
	value := []rune(trimmed)
	if len(value) < len(token) {
		return false
	}
	if hasTokenPrefix(value, token) {
		return true
	}
	if hasTokenSuffix(value, token) {
		return true
	}
	return false
}

func hasTokenPrefix(value []rune, token []rune) bool {
	if len(value) < len(token) {
		return false
	}
	for i := range token {
		if value[i] != token[i] {
			return false
		}
	}
	if len(value) == len(token) {
		return true
	}
	return !isWordChar(value[len(token)])
}

func hasTokenSuffix(value []rune, token []rune) bool {
	if len(value) < len(token) {
		return false
	}
	start := len(value) - len(token)
	for i := range token {
		if value[start+i] != token[i] {
			return false
		}
	}
	if start == 0 {
		return true
	}
	return !isWordChar(value[start-1])
}

func isWordChar(value rune) bool {
	return value == '_' || unicode.IsLetter(value) || unicode.IsDigit(value)
}

func normalizeTextForComparison(text string) string {
	trimmed := strings.TrimSpace(strings.ToLower(text))
	if trimmed == "" {
		return ""
	}
	return strings.TrimSpace(whitespacePattern.ReplaceAllString(trimmed, " "))
}

func isMessagingToolDuplicate(text string, sentTexts []string) bool {
	if len(sentTexts) == 0 {
		return false
	}
	normalized := normalizeTextForComparison(text)
	if len(normalized) < minDuplicateTextLength {
		return false
	}
	for _, sent := range sentTexts {
		sentNormalized := normalizeTextForComparison(sent)
		if len(sentNormalized) < minDuplicateTextLength {
			continue
		}
		if strings.Contains(normalized, sentNormalized) || strings.Contains(sentNormalized, normalized) {
			return true
		}
	}
	return false
}

// requireIdentity resolves identity for the current message. Always resolves from msg so each sender is identified correctly (no reuse of context state across messages).
func (p *ChannelInboundProcessor) requireIdentity(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage) (IdentityState, error) {
	if p.identity == nil {
		return IdentityState{}, fmt.Errorf("identity resolver not configured")
	}
	return p.identity.Resolve(ctx, cfg, msg)
}

func (p *ChannelInboundProcessor) resolveProcessingStatusNotifier(channelType channel.ChannelType) channel.ProcessingStatusNotifier {
	if p == nil || p.registry == nil {
		return nil
	}
	notifier, ok := p.registry.GetProcessingStatusNotifier(channelType)
	if !ok {
		return nil
	}
	return notifier
}

func (p *ChannelInboundProcessor) notifyProcessingStarted(
	ctx context.Context,
	notifier channel.ProcessingStatusNotifier,
	cfg channel.ChannelConfig,
	msg channel.InboundMessage,
	info channel.ProcessingStatusInfo,
) (channel.ProcessingStatusHandle, error) {
	if notifier == nil {
		return channel.ProcessingStatusHandle{}, nil
	}
	statusCtx, cancel := context.WithTimeout(ctx, processingStatusTimeout)
	defer cancel()
	return notifier.ProcessingStarted(statusCtx, cfg, msg, info)
}

func (p *ChannelInboundProcessor) notifyProcessingCompleted(
	ctx context.Context,
	notifier channel.ProcessingStatusNotifier,
	cfg channel.ChannelConfig,
	msg channel.InboundMessage,
	info channel.ProcessingStatusInfo,
	handle channel.ProcessingStatusHandle,
) error {
	if notifier == nil {
		return nil
	}
	statusCtx, cancel := context.WithTimeout(ctx, processingStatusTimeout)
	defer cancel()
	return notifier.ProcessingCompleted(statusCtx, cfg, msg, info, handle)
}

func (p *ChannelInboundProcessor) notifyProcessingFailed(
	ctx context.Context,
	notifier channel.ProcessingStatusNotifier,
	cfg channel.ChannelConfig,
	msg channel.InboundMessage,
	info channel.ProcessingStatusInfo,
	handle channel.ProcessingStatusHandle,
	cause error,
) error {
	if notifier == nil {
		return nil
	}
	statusCtx, cancel := context.WithTimeout(ctx, processingStatusTimeout)
	defer cancel()
	return notifier.ProcessingFailed(statusCtx, cfg, msg, info, handle, cause)
}

func (p *ChannelInboundProcessor) logProcessingStatusError(
	stage string,
	msg channel.InboundMessage,
	identity InboundIdentity,
	err error,
) {
	if p == nil || p.logger == nil || err == nil {
		return
	}
	p.logger.Warn(
		"processing status notify failed",
		slog.String("stage", stage),
		slog.String("channel", msg.Channel.String()),
		slog.String("channel_identity_id", identity.ChannelIdentityID),
		slog.String("user_id", identity.UserID),
		slog.Any("error", err),
	)
}

// formatTokenUsage formats token usage into a compact string like "⚡ X.Xk"
// Only shows completion tokens as that's the main cost indicator.
func formatTokenUsage(usage *gatewayUsage) string {
	if usage == nil || usage.TotalTokens == 0 {
		return ""
	}
	// Show completion tokens in k format (e.g., "⚡ 1.2k")
	kTokens := float64(usage.CompletionTokens) / 1000.0
	return fmt.Sprintf("⚡ %.1fk", kTokens)
}

// broadcastToOtherChannels sends assistant outputs to all other bound channels
// for the same bot. Fire-and-forget: errors are logged, never returned.
// It skips all routes on the same platform as the origin, and web routes.
func (p *ChannelInboundProcessor) broadcastToOtherChannels(
	botID, chatID, originPlatform, originTarget string,
	outputs []conversation.AssistantOutput,
) {
	if p.broadcaster == nil || p.routeLister == nil || len(outputs) == 0 {
		return
	}
	ctx := context.Background()
	routes, err := p.routeLister.List(ctx, chatID)
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("broadcast: list routes failed",
				slog.String("chat_id", chatID), slog.Any("error", err))
		}
		return
	}
	for _, r := range routes {
		platform := strings.ToLower(strings.TrimSpace(r.Platform))
		if platform == "web" {
			continue
		}
		target := strings.TrimSpace(r.ReplyTarget)
		if platform == originPlatform {
			continue
		}
		if target == "" {
			continue
		}
		for _, output := range outputs {
			text := strings.TrimSpace(output.Content)
			if text == "" || isSilentReplyText(text) {
				continue
			}
			if err := p.broadcaster.Send(ctx, botID,
				channel.ChannelType(platform),
				channel.SendRequest{
					Target:  target,
					Message: channel.Message{Text: text},
				}); err != nil {
				if p.logger != nil {
					p.logger.Warn("broadcast: send failed",
						slog.String("platform", platform),
						slog.String("bot_id", botID),
						slog.Any("error", err))
				}
			}
		}
	}
}

// dispatchGroupChat is called by the GroupDebouncer after the debounce window
// expires. It runs the full chat processing pipeline (token generation, stream
// open, runner.StreamChat) with the given merged text.
func (p *ChannelInboundProcessor) dispatchGroupChat(
	ctx context.Context,
	cfg channel.ChannelConfig,
	msg channel.InboundMessage,
	text string,
	sender channel.StreamReplySender,
	identity InboundIdentity,
	resolved route.ResolveConversationResult,
	activeChatID string,
	userMessagePersisted bool,
) error {
	chatToken := ""
	if p.jwtSecret != "" && strings.TrimSpace(msg.ReplyTarget) != "" {
		signed, _, err := auth.GenerateChatToken(auth.ChatToken{
			BotID:             identity.BotID,
			ChatID:            activeChatID,
			RouteID:           resolved.RouteID,
			UserID:            identity.UserID,
			ChannelIdentityID: identity.ChannelIdentityID,
		}, p.jwtSecret, p.tokenTTL)
		if err == nil {
			chatToken = signed
		}
	}
	token := ""
	if identity.UserID != "" && p.jwtSecret != "" {
		signed, _, err := auth.GenerateToken(identity.UserID, p.jwtSecret, p.tokenTTL)
		if err == nil {
			token = "Bearer " + signed
		}
	}
	if token == "" && chatToken != "" {
		token = "Bearer " + chatToken
	}

	target := strings.TrimSpace(msg.ReplyTarget)
	if target == "" {
		return fmt.Errorf("reply target missing for group debounce dispatch")
	}
	sourceMessageID := strings.TrimSpace(msg.Message.ID)
	replyRef := &channel.ReplyRef{Target: target}
	if sourceMessageID != "" {
		replyRef.MessageID = sourceMessageID
	}
	// Build metadata for stream options, including req_id for WeCom and other channels
	streamMetadata := map[string]any{
		"route_id": resolved.RouteID,
	}
	if msg.Metadata != nil {
		if v, ok := msg.Metadata["req_id"].(string); ok && v != "" {
			streamMetadata["req_id"] = v
		}
	}
	stream, err := sender.OpenStream(ctx, target, channel.StreamOptions{
		Reply:           replyRef,
		SourceMessageID: sourceMessageID,
		Metadata:        streamMetadata,
		ReceivedAt:      msg.ReceivedAt,
	})
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close(context.WithoutCancel(ctx)) }()

	if err := stream.Push(ctx, channel.StreamEvent{
		Type:   channel.StreamEventStatus,
		Status: channel.StreamStatusStarted,
	}); err != nil {
		return err
	}

	// Build input attachments and extract file content for supported types
	if p.logger != nil && len(msg.Message.Attachments) > 0 {
		p.logger.Info("processing message attachments", slog.Int("count", len(msg.Message.Attachments)))
		for i, att := range msg.Message.Attachments {
			p.logger.Info("attachment details", slog.Int("index", i), slog.String("type", string(att.Type)), slog.String("name", att.Name), slog.String("mime", att.Mime), slog.Int("dataSize", len(att.Data)))
		}
	}
	inputAttachments, fileContext := buildInputAttachments(msg.Message.Attachments, p.dataRoot, p.logger)
	if fileContext != "" {
		if p.logger != nil {
			origText := text
			if len(origText) > 100 {
				origText = origText[:100] + "..."
			}
			p.logger.Info("adding file context to query", slog.Int("contextLength", len(fileContext)), slog.String("originalText", origText))
		}
		text = text + fileContext
		if p.logger != nil {
			p.logger.Info("query with file context", slog.Int("totalLength", len(text)))
		}
	}

	chunkCh, streamErrCh := p.runner.StreamChat(ctx, conversation.ChatRequest{
		BotID:                   identity.BotID,
		ChatID:                  activeChatID,
		Token:                   token,
		UserID:                  identity.UserID,
		SourceChannelIdentityID: identity.ChannelIdentityID,
		DisplayName:             identity.DisplayName,
		RouteID:                 resolved.RouteID,
		ChatToken:               chatToken,
		ExternalMessageID:       sourceMessageID,
		ConversationType:        msg.Conversation.Type,
		ReplyTarget:             strings.TrimSpace(msg.ReplyTarget),
		Query:                   text,
		CurrentChannel:          msg.Channel.String(),
		Channels:                []string{msg.Channel.String()},
		UserMessagePersisted:    userMessagePersisted,
		InputAttachments:        inputAttachments,
	})

	var (
		finalMessages  []conversation.ModelMessage
		streamErr      error
		collectedUsage *gatewayUsage
	)
	for chunkCh != nil || streamErrCh != nil {
		select {
		case chunk, ok := <-chunkCh:
			if !ok {
				chunkCh = nil
				continue
			}
			events, messages, usage, parseErr := mapStreamChunkToChannelEvents(chunk)
			if parseErr != nil {
				if p.logger != nil {
					p.logger.Warn("group debounce stream chunk parse failed",
						slog.String("channel", msg.Channel.String()),
						slog.Any("error", parseErr),
					)
				}
				continue
			}
			if usage != nil && usage.TotalTokens > 0 {
				collectedUsage = usage
			}
			for _, event := range events {
				if pushErr := stream.Push(ctx, event); pushErr != nil {
					if p.logger != nil {
						p.logger.Warn("group debounce stream push failed", slog.Any("error", pushErr))
					}
				}
			}
			if messages != nil {
				finalMessages = append(finalMessages, messages...)
			}
		case err, ok := <-streamErrCh:
			if !ok {
				streamErrCh = nil
				continue
			}
			streamErr = err
		}
	}

	if streamErr != nil {
		return streamErr
	}
	outputs := flow.ExtractAssistantOutputs(finalMessages)
	// Broadcast disabled: see comment above the other broadcastToOtherChannels call site.
	// go p.broadcastToOtherChannels(identity.BotID, activeChatID, strings.ToLower(msg.Channel.String()), target, outputs)
	_ = outputs
	_ = collectedUsage
	return nil
}

// buildInputAttachments converts channel attachments with binary data into
// conversation.InputAttachment entries for the LLM gateway.
// It also extracts text content from supported file types (PDF, DOCX, XLSX, etc.)
// and returns it as fileContext to be appended to the query.
// Files are saved to {dataRoot}/shared/attachments/ which is mounted at /shared in bot containers.
// dataRoot is the actual path where files are saved (e.g., /opt/memoh/data).
func buildInputAttachments(attachments []channel.Attachment, dataRoot string, logger *slog.Logger) ([]conversation.InputAttachment, string) {
	if len(attachments) == 0 {
		return nil, ""
	}
	var out []conversation.InputAttachment
	var fileContext strings.Builder
	hasExtractableFiles := false

	if logger != nil {
		logger.Info("building input attachments", slog.Int("count", len(attachments)))
	}

	for _, att := range attachments {
		switch att.Type {
		case channel.AttachmentImage:
			if len(att.Data) > 0 {
				// Compress image if needed to prevent token explosion
				compressedData, mimeType, wasCompressed := compressImageIfNeeded(att.Data, att.Mime, logger)
				if wasCompressed && logger != nil {
					logger.Info("image compressed for LLM",
						slog.String("originalSize", formatBytes(len(att.Data))),
						slog.String("compressedSize", formatBytes(len(compressedData))),
						slog.String("mimeType", mimeType))
				}
				out = append(out, conversation.InputAttachment{
					Type:   "image",
					Base64: base64.StdEncoding.EncodeToString(compressedData),
				})
			}
		case channel.AttachmentFile:
			// For files, save to data directory and pass path to agent
			if len(att.Data) > 0 {
				if logger != nil {
					logger.Info("processing file attachment", slog.String("name", att.Name), slog.String("mime", att.Mime))
				}

				// Save file to dataRoot/shared/attachments/ directory for bot container access
				// Bot containers mount this as /shared/attachments/
				savePath, err := saveAttachmentToDataDir(att, dataRoot, logger)
				if err != nil {
					if logger != nil {
						logger.Error("failed to save attachment", slog.String("name", att.Name), slog.String("error", err.Error()))
					}
					// Still add attachment with original name if save fails
					out = append(out, conversation.InputAttachment{
						Type:   "file",
						Base64: base64.StdEncoding.EncodeToString(att.Data),
						Path:   att.Name,
					})
				} else {
					// Try to extract text from saved file (using actual path)
					extractedText := tryExtractFileContent(savePath, att.Mime, att.Name, logger)

					// Bot-visible path: /shared/attachments/filename.xls
					// The shared directory is mounted at /shared in bot containers
					botVisiblePath := "/shared/attachments/" + filepath.Base(savePath)

					// Always include file info in context, even if extraction fails
					if !hasExtractableFiles {
						fileContext.WriteString("\n\n[Attached files]\n")
						hasExtractableFiles = true
					}

					if extractedText != "" {
						// Successfully extracted content
						fileContext.WriteString(fmt.Sprintf("\n### File: %s\nPath: %s\nSize: %d bytes\n\n```\n%s\n```\n",
							att.Name, botVisiblePath, len(att.Data), extractedText))
					} else {
						// Extraction failed or file type not supported for extraction
						// Include file metadata and explicit skill usage instruction
						ext := strings.ToLower(filepath.Ext(att.Name))
						skillHint := ""
						switch ext {
						case ".xls", ".xlsx", ".xlsm":
							skillHint = "\n⚠️ ACTION REQUIRED: Use the 'xlsx' skill IMMEDIATELY to read this Excel file. Use use_skill with skillName='xlsx'. DO NOT use rag-documents skill for Excel files - it requires external RAG service."
						case ".pdf":
							skillHint = "\n⚠️ ACTION REQUIRED: Use an appropriate PDF skill if available, or ask the user what they need from this PDF."
						case ".docx", ".doc":
							skillHint = "\n⚠️ ACTION REQUIRED: Use the 'docx' skill if available to read this Word document."
						case ".pptx", ".ppt":
							skillHint = "\n⚠️ ACTION REQUIRED: Use the 'pptx' skill if available to read this PowerPoint file."
						default:
							skillHint = "\n⚠️ ACTION REQUIRED: This file type may require using a skill to analyze."
						}
						fileContext.WriteString(fmt.Sprintf("\n### File: %s\nPath: %s\nSize: %d bytes\nType: %s%s\n",
							att.Name, botVisiblePath, len(att.Data), att.Mime, skillHint))
					}

					// Add attachment with bot-visible file path
					out = append(out, conversation.InputAttachment{
						Type:   "file",
						Base64: base64.StdEncoding.EncodeToString(att.Data),
						Path:   botVisiblePath,
					})
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, ""
	}
	if logger != nil {
		logger.Info("buildInputAttachments completed", slog.Int("attachmentCount", len(out)), slog.Bool("hasExtractableFiles", hasExtractableFiles), slog.Int("contextLength", fileContext.Len()))
		if hasExtractableFiles {
			preview := fileContext.String()
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			logger.Info("extracted file context preview", slog.String("contextPreview", preview))
		}
	}
	return out, fileContext.String()
}

// saveAttachmentToDataDir saves the attachment to {dataRoot}/shared/attachments/ directory
// where bot containers can access it via /shared/attachments/. Returns the full path to the saved file.
// dataRoot should be the path configured in mcp.data_root (e.g., /opt/memoh/data).
func saveAttachmentToDataDir(att channel.Attachment, dataRoot string, logger *slog.Logger) (string, error) {
	// Use the configured data root path
	if dataRoot == "" {
		dataRoot = "/opt/memoh/data"
	}
	// Save to shared directory so all bot containers can access
	// The shared directory is mounted to /shared in bot containers
	attachmentsDir := filepath.Join(dataRoot, "shared", "attachments")
	if err := os.MkdirAll(attachmentsDir, 0755); err != nil {
		return "", fmt.Errorf("create attachments directory: %w", err)
	}

	// Generate a safe filename
	safeName := sanitizeFileName(att.Name)
	if safeName == "" {
		safeName = "unnamed_attachment"
	}

	// Add timestamp to avoid conflicts
	timestamp := time.Now().UnixNano()
	ext := filepath.Ext(safeName)
	baseName := strings.TrimSuffix(safeName, ext)
	fileName := fmt.Sprintf("%s_%d%s", baseName, timestamp, ext)
	filePath := filepath.Join(attachmentsDir, fileName)

	// Write file data
	if err := os.WriteFile(filePath, att.Data, 0644); err != nil {
		return "", fmt.Errorf("write attachment file: %w", err)
	}

	if logger != nil {
		logger.Info("saved attachment to data directory", slog.String("originalName", att.Name), slog.String("savedPath", filePath), slog.Int("size", len(att.Data)))
	}

	return filePath, nil
}

// sanitizeFileName removes or replaces unsafe characters from filename
func sanitizeFileName(name string) string {
	// Remove path separators and other unsafe characters
	unsafe := []string{"/", "\\", "..", "<", ">", ":", "\"", "|", "?", "*"}
	result := name
	for _, c := range unsafe {
		result = strings.ReplaceAll(result, c, "_")
	}
	return result
}

// tryExtractFileContent attempts to extract text content from a saved file.
// It uses fileparse.ExtractText for supported types.
func tryExtractFileContent(filePath string, mimeType string, originalName string, logger *slog.Logger) string {
	// Check if this is a supported file type based on MIME type or extension
	mime := strings.ToLower(strings.TrimSpace(mimeType))
	ext := strings.ToLower(filepath.Ext(originalName))

	if logger != nil {
		logger.Info("trying to extract file content", slog.String("name", originalName), slog.String("path", filePath), slog.String("mime", mime), slog.String("ext", ext))
	}

	// If no extension, try to detect from file content (magic numbers)
	if ext == "" {
		data, err := os.ReadFile(filePath)
		if err == nil && len(data) > 0 {
			detectedExt := detectFileTypeByContent(data)
			if detectedExt != "" {
				ext = detectedExt
				if logger != nil {
					logger.Info("detected file type by content", slog.String("name", originalName), slog.String("detectedExt", detectedExt))
				}
			}
		}
	}

	// Check if it's a supported type (rely more on extension since WeCom doesn't set MIME)
	isSupported := false
	switch {
	case mime == "application/pdf" || ext == ".pdf":
		isSupported = true
		if mime == "" || mime == "application/octet-stream" {
			mime = "application/pdf"
		}
	case mime == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" || ext == ".docx":
		isSupported = true
		if mime == "" || mime == "application/octet-stream" {
			mime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		}
	case mime == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
		mime == "application/vnd.ms-excel" || ext == ".xlsx" || ext == ".xls":
		isSupported = true
		if mime == "" || mime == "application/octet-stream" {
			mime = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		}
	case mime == "application/vnd.openxmlformats-officedocument.presentationml.presentation" ||
		mime == "application/vnd.ms-powerpoint" || ext == ".pptx" || ext == ".ppt":
		isSupported = true
		if mime == "" || mime == "application/octet-stream" {
			mime = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
		}
	case mime == "text/plain" || ext == ".txt" || ext == ".md" || ext == ".json" || ext == ".csv":
		isSupported = true
		if mime == "" || mime == "application/octet-stream" {
			mime = "text/plain"
		}
	}

	if logger != nil {
		logger.Info("file type check result", slog.String("name", originalName), slog.String("mime", mime), slog.String("ext", ext), slog.Bool("isSupported", isSupported))
	}

	if !isSupported {
		if logger != nil {
			logger.Info("file type not supported for extraction", slog.String("name", originalName), slog.String("mime", mime), slog.String("ext", ext))
		}
		return ""
	}

	// Extract text using fileparse from the saved file
	if logger != nil {
		logger.Info("calling fileparse.ExtractText", slog.String("name", originalName), slog.String("mime", mime), slog.String("path", filePath))
	}
	text, err := fileparse.ExtractText(filePath, mime)
	if err != nil {
		if logger != nil {
			logger.Error("failed to extract text", slog.String("name", originalName), slog.String("mime", mime), slog.String("error", err.Error()))
		}
		// Return empty string to let the skill handle the file
		// The file is still passed as an attachment to the agent
		return ""
	}
	if logger != nil {
		preview := text
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		logger.Info("successfully extracted text", slog.String("name", originalName), slog.String("mime", mime), slog.Int("textLength", len(text)), slog.String("textPreview", preview))
	}
	return text
}

// detectFileTypeByContent detects file type by examining the file's magic numbers (first few bytes).
// Returns the file extension including the dot (e.g., ".pdf") or empty string if unknown.
func detectFileTypeByContent(data []byte) string {
	if len(data) < 4 {
		return ""
	}

	// Check for PDF: starts with "%PDF"
	if len(data) >= 4 && string(data[0:4]) == "%PDF" {
		return ".pdf"
	}

	// Check for ZIP (DOCX, XLSX, PPTX are all ZIP-based): starts with "PK\x03\x04" or "PK\x05\x06"
	if len(data) >= 4 && data[0] == 0x50 && data[1] == 0x4B {
		// Check for Office Open XML formats by examining the ZIP content
		// DOCX, XLSX, PPTX all start with PK and contain specific file entries
		// For simplicity, we'll try to distinguish by looking for characteristic strings
		dataStr := string(data)
		// Check for DOCX (contains word/ directory)
		if containsAny(dataStr, []string{"word/document.xml", "word/_rels"}) {
			return ".docx"
		}
		// Check for XLSX (contains xl/ directory)
		if containsAny(dataStr, []string{"xl/workbook.xml", "xl/_rels"}) {
			return ".xlsx"
		}
		// Check for PPTX (contains ppt/ directory)
		if containsAny(dataStr, []string{"ppt/presentation.xml", "ppt/_rels"}) {
			return ".pptx"
		}
		// Generic ZIP-based Office file
		return ".zip"
	}

	// Check for legacy Excel 97-2003 (.xls) - OLE compound document format
	// Signature: D0 CF 11 E0 A1 B1 1A E1
	if len(data) >= 8 &&
		data[0] == 0xD0 && data[1] == 0xCF &&
		data[2] == 0x11 && data[3] == 0xE0 &&
		data[4] == 0xA1 && data[5] == 0xB1 &&
		data[6] == 0x1A && data[7] == 0xE1 {
		return ".xls"
	}

	// Check for plain text (printable ASCII characters)
	isText := true
	checkLen := min(len(data), 512)
	for i := 0; i < checkLen; i++ {
		b := data[i]
		// Allow printable ASCII, tabs, newlines, carriage returns
		if b < 32 && b != 9 && b != 10 && b != 13 {
			isText = false
			break
		}
	}
	if isText {
		return ".txt"
	}

	return ""
}

// containsAny checks if any of the substrings exist in the given string
func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
