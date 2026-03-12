package wecom

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Kxiandaoyan/Memoh-v2/internal/channel"
)

// OutboundStream implements channel.OutboundStream for WeCom
// Supports real-time streaming output using WeCom's stream message format
// Following SDK specification: always send full content, not delta
type OutboundStream struct {
	adapter         *Adapter
	cfg             channel.ChannelConfig
	wsClient        *WebSocketClient
	reqID           string
	chatID          string
	userID          string
	chatType        string // 会话类型：single 或 group
	isMentioned     bool   // 是否被@提及（群聊时有效）
	logger          *slog.Logger

	buffer          strings.Builder
	closed          atomic.Bool
	sent            atomic.Bool
	streamID        string
	mu              sync.Mutex
	streamStartTime time.Time // 流式消息开始时间，用于6分钟超时检查

	// Buffered sending mechanism
	ticker          *time.Ticker
	stopTicker      chan struct{}
	pendingSend     atomic.Bool

	// Track last sent content and time for rate limiting
	lastSentContent string
	lastSendTime    time.Time
}

// StreamTimeout 流式消息超时时间（6分钟）
const StreamTimeout = 6 * time.Minute

// MaxContentBytes 单条消息最大字节数（企业微信 AI Bot SDK 限制：20480 字节）
const MaxContentBytes = 20480

// MinSendInterval 流式消息最小发送间隔（控制发送频率，避免过多消息）
const MinSendInterval = 600 * time.Millisecond // 600ms，与流畅性和实时性平衡

// NewOutboundStream creates a new outbound stream
func NewOutboundStream(adapter *Adapter, cfg channel.ChannelConfig, wsClient *WebSocketClient, reqID, chatID, userID, chatType string, isMentioned bool, streamID string, logger *slog.Logger) *OutboundStream {
	// If no streamID provided, generate a new one
	if streamID == "" {
		streamID = generateStreamID()
	}

	s := &OutboundStream{
		adapter:         adapter,
		cfg:             cfg,
		wsClient:        wsClient,
		reqID:           reqID,
		chatID:          chatID,
		userID:          userID,
		chatType:        chatType,
		isMentioned:     isMentioned,
		streamID:        streamID,
		logger:          logger.With(slog.String("component", "wecom_stream"), slog.String("req_id", reqID)),
		streamStartTime: time.Now(),
		stopTicker:      make(chan struct{}),
		lastSendTime:    time.Now(),
	}

	// Start background ticker for buffered sending
	s.ticker = time.NewTicker(MinSendInterval)
	go s.sendLoop()

	return s
}

// sendLoop runs in background to send buffered content periodically
func (s *OutboundStream) sendLoop() {
	for {
		select {
		case <-s.ticker.C:
			s.flushBuffer()
		case <-s.stopTicker:
			s.ticker.Stop()
			return
		}
	}
}

// flushBuffer sends the current buffer content if there's anything new
// Following SDK spec: send full content, not delta
func (s *OutboundStream) flushBuffer() {
	if s.closed.Load() {
		return
	}

	s.mu.Lock()
	content := s.buffer.String()
	lastSent := s.lastSentContent
	s.mu.Unlock()

	// Only send if content has changed since last send
	if content == "" || content == lastSent {
		return
	}

	// Check if enough time has passed since last send
	if time.Since(s.lastSendTime) < MinSendInterval {
		return
	}

	// Use background context with timeout for sending
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Send as intermediate update (finish=false) with full content
	if err := s.sendFullContent(ctx, content, false); err != nil {
		// Log error but DON'T update lastSentContent so we can retry on next tick
		// This ensures we don't "retract" content by failing to send updates
		s.logger.Warn("buffered send failed, will retry on next tick",
			slog.Any("error", err),
			slog.Int("content_len", len(content)))
	} else {
		s.mu.Lock()
		s.lastSentContent = content
		s.lastSendTime = time.Now()
		s.mu.Unlock()
	}
}

// Push handles stream events
func (s *OutboundStream) Push(ctx context.Context, event channel.StreamEvent) error {
	if s.closed.Load() {
		if event.Type == channel.StreamEventStatus {
			return nil
		}
		return fmt.Errorf("stream is closed")
	}

	switch event.Type {
	case channel.StreamEventStatus:
		s.logger.Debug("stream status", slog.String("status", string(event.Status)))

	case channel.StreamEventDelta:
		// Check for 6-minute timeout
		if time.Since(s.streamStartTime) > StreamTimeout {
			s.logger.Warn("stream timeout: exceeding 6 minute limit, forcing finish",
				slog.Duration("elapsed", time.Since(s.streamStartTime)))
			s.mu.Lock()
			content := s.buffer.String()
			lastSent := s.lastSentContent
			s.mu.Unlock()

			// Use the longer content to prevent "thinking..." fallback
			if len(lastSent) > len(content) {
				content = lastSent
			}

			// CRITICAL: Never send empty content on timeout
			if content == "" {
				content = "处理超时，请稍后再试。"
			} else {
				content = content + "\n\n[系统提示: 处理超时，以上是已生成的回复]"
			}

			s.closed.Store(true)
			close(s.stopTicker)
			s.sendFullContent(ctx, content, true)
			return nil
		}

		s.mu.Lock()
		s.buffer.WriteString(event.Delta)
		s.mu.Unlock()

		s.logger.Debug("stream delta buffered",
			slog.Int("delta_len", len(event.Delta)))

	case channel.StreamEventFinal:
		// Stop the ticker first to prevent any further intermediate updates
		close(s.stopTicker)

		var finalContent string
		if event.Final != nil && event.Final.Message.Text != "" {
			finalContent = event.Final.Message.Text
			s.mu.Lock()
			s.buffer.Reset()
			s.buffer.WriteString(finalContent)
			s.mu.Unlock()
		} else {
			s.mu.Lock()
			finalContent = s.buffer.String()
			s.mu.Unlock()
		}

		// CRITICAL: Ensure final content is never shorter than what was already sent
		// This prevents "retracting" the message
		s.mu.Lock()
		if len(finalContent) < len(s.lastSentContent) {
			s.logger.Warn("final content shorter than last sent, using last sent content",
				slog.Int("final_len", len(finalContent)),
				slog.Int("last_sent_len", len(s.lastSentContent)))
			finalContent = s.lastSentContent
		}
		s.mu.Unlock()

		// CRITICAL: Ensure final content is never empty
		if finalContent == "" {
			finalContent = "处理完成，请查看完整回复。"
			s.logger.Warn("final content was empty, using default message")
		}

		s.logger.Info("sending final message",
			slog.Int("content_len", len(finalContent)),
			slog.String("stream_id", s.streamID))

		// Send final response with finish=true (with retries)
		if err := s.sendFullContent(ctx, finalContent, true); err != nil {
			s.logger.Error("failed to send final message even after retries", slog.Any("error", err))
			return err
		}

		// Update lastSentContent to final content
		s.mu.Lock()
		s.lastSentContent = finalContent
		s.mu.Unlock()

		return nil

	case channel.StreamEventError:
		// Stop the ticker
		close(s.stopTicker)

		errorMsg := event.Error
		if errorMsg == "" {
			errorMsg = "处理中断，以下是已生成的回复"
		}

		s.logger.Error("stream error", slog.String("error", event.Error))

		// CRITICAL: Always send finish=true with existing content to prevent "thinking..." fallback
		if !s.sent.Load() {
			s.mu.Lock()
			existingContent := s.buffer.String()
			lastSent := s.lastSentContent
			s.mu.Unlock()

			// Use the longer of existingContent or lastSentContent
			// This ensures we never send empty or shorter content
			finalContent := existingContent
			if len(lastSent) > len(finalContent) {
				finalContent = lastSent
			}

			var finalMsg string
			if finalContent != "" {
				// Append error notice to existing content
				finalMsg = finalContent + "\n\n[系统提示: " + errorMsg + "]"
			} else {
				// Only if truly empty, use error message with explicit content
				finalMsg = "处理过程中断，请重试。"
			}

			s.logger.Info("sending error finish message with content",
				slog.Int("content_len", len(finalContent)),
				slog.Int("final_msg_len", len(finalMsg)))

			// Force send with finish=true - ignore errors to prevent blocking
			s.sendFullContent(ctx, finalMsg, true)
			s.sent.Store(true)
		}

		s.closed.Store(true)
	}

	return nil
}

// sendFullContent sends the full content to WeCom
// Following SDK specification: always send complete content, not delta
// If content exceeds MaxContentBytes, it will be truncated with a notice
func (s *OutboundStream) sendFullContent(ctx context.Context, content string, finish bool) error {
	// CRITICAL: Never send empty content for intermediate updates
	if content == "" && !finish {
		return nil
	}

	// CRITICAL: For final message, ensure content is never empty
	// This prevents "retracting" the message to empty state
	if finish && content == "" {
		content = "处理完成，请查看完整回复。"
		s.logger.Warn("final message content was empty, using default")
	}

	// Check if content exceeds byte limit (WeCom AI Bot SDK limit: 20480 bytes)
	// If so, truncate it and add a notice
	truncatedContent, wasTruncated := s.truncateToMaxBytes(content, MaxContentBytes-100) // Reserve space for notice
	if wasTruncated && finish {
		truncatedContent += "\n\n[内容过长，已截断显示，请查看完整回复]"
	}

	s.mu.Lock()
	wsClient := s.wsClient
	streamID := s.streamID
	reqID := s.reqID
	chatType := s.chatType
	isMentioned := s.isMentioned
	s.mu.Unlock()

	if wsClient == nil {
		return fmt.Errorf("websocket client is nil")
	}

	s.logger.Debug("sending stream update",
		slog.String("stream_id", streamID),
		slog.Int("content_bytes", len(truncatedContent)),
		slog.Int("original_bytes", len(content)),
		slog.Bool("was_truncated", wasTruncated),
		slog.Bool("finish", finish))

	// Send stream update using WeCom stream format
	body := StreamMsgBody{
		MsgType: MsgTypeStream,
		Stream: StreamResponse{
			ID:      streamID,
			Finish:  finish,
			Content: truncatedContent, // Send FULL content, not delta
		},
	}

	// Determine which command to use based on chat type and mention status
	// - 单聊 (single): always use CmdRespondMsg
	// - 群聊且被@ (group + isMentioned): use CmdRespondMsg (reply to the mention)
	// - 群聊未被@ (group + !isMentioned): use CmdSendMsg (proactive send)
	cmd := CmdRespondMsg
	if chatType == "group" && !isMentioned {
		cmd = CmdSendMsg
		s.logger.Debug("using proactive send for group message without mention",
			slog.String("chat_type", chatType),
			slog.Bool("is_mentioned", isMentioned))
	}

	// Send message (wait for ACK as per SDK specification)
	// For final messages, use aggressive retry to ensure visibility
	var lastErr error
	maxRetries := 1
	baseDelay := 500 * time.Millisecond
	if finish {
		maxRetries = 5 // Increased retries for final messages (total ~7.5s max)
		baseDelay = 1 * time.Second
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s, 8s...
			delay := time.Duration(attempt) * baseDelay
			if delay > 5*time.Second {
				delay = 5 * time.Second
			}
			s.logger.Info("retrying send",
				slog.Int("attempt", attempt+1),
				slog.Int("max_retries", maxRetries),
				slog.Duration("delay", delay),
				slog.Bool("finish", finish))
			time.Sleep(delay)
		}

		if err := wsClient.SendStream(ctx, reqID, body, cmd); err != nil {
			lastErr = err
			s.logger.Warn("send attempt failed",
				slog.Int("attempt", attempt+1),
				slog.Bool("finish", finish),
				slog.Any("error", err))
			continue
		}

		// Success
		if finish {
			s.sent.Store(true)
			s.logger.Info("final stream response sent successfully",
				slog.String("stream_id", streamID),
				slog.String("cmd", cmd),
				slog.Int("content_bytes", len(truncatedContent)),
				slog.Bool("was_truncated", wasTruncated),
				slog.Int("attempts", attempt+1))
		} else {
			s.logger.Debug("intermediate stream update sent",
				slog.String("stream_id", streamID),
				slog.Int("content_bytes", len(truncatedContent)))
		}
		return nil
	}

	// CRITICAL: If final message failed after all retries, we still return success
	// because the user has already seen the intermediate content.
	// The finish=true failure won't "retract" the already visible content.
	if finish {
		s.logger.Error("final message failed after all retries, but intermediate content is visible",
			slog.Int("max_retries", maxRetries),
			slog.Any("last_error", lastErr),
			slog.Int("visible_content_len", len(content)))
		// Mark as sent to prevent further attempts
		s.sent.Store(true)
		return nil
	}

	return fmt.Errorf("send stream update failed after %d attempts: %w", maxRetries, lastErr)
}

// Close sends the final response and closes the stream
func (s *OutboundStream) Close(ctx context.Context) error {
	if s.closed.Load() || s.sent.Load() {
		return nil
	}
	s.closed.Store(true)

	// Stop the ticker
	select {
	case <-s.stopTicker:
		// Already closed
	default:
		close(s.stopTicker)
	}

	s.mu.Lock()
	content := s.buffer.String()
	lastSent := s.lastSentContent
	s.mu.Unlock()

	// CRITICAL: Use the longest content available to prevent "thinking..." fallback
	if len(lastSent) > len(content) {
		content = lastSent
	}

	// CRITICAL: Never send empty final message
	if content == "" {
		content = "处理完成，请查看完整回复。"
		s.logger.Warn("close content was empty, using default message")
	}

	// Send final response with finish=true (with retries)
	s.logger.Info("closing stream with final message", slog.Int("content_len", len(content)))

	// Ignore error to ensure we don't block - the message may still be visible
	s.sendFullContent(ctx, content, true)
	s.sent.Store(true)
	return nil
}

// generateStreamID generates a unique stream ID
func generateStreamID() string {
	return fmt.Sprintf("stream_%d", time.Now().UnixNano())
}

// truncateToMaxBytes truncates content to maxBytes while preserving UTF-8 integrity
// Returns the truncated content and a boolean indicating if truncation occurred
func (s *OutboundStream) truncateToMaxBytes(content string, maxBytes int) (string, bool) {
	contentBytes := []byte(content)
	if len(contentBytes) <= maxBytes {
		return content, false
	}

	// Truncate to maxBytes, ensuring we don't cut a UTF-8 character
	// In UTF-8, continuation bytes start with 10xxxxxx (0x80-0xBF)
	// Non-continuation bytes start with 0xxxxxxx (0x00-0x7F) or 11xxxxxx (0xC0-0xFF)
	truncateAt := maxBytes
	for truncateAt > maxBytes-4 && truncateAt > 0 {
		if (contentBytes[truncateAt] & 0xC0) != 0x80 {
			// Found a non-continuation byte, safe to truncate here
			break
		}
		truncateAt--
	}

	return string(contentBytes[:truncateAt]), true
}
