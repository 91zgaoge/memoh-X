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
	lastSentLen     int
	streamStartTime time.Time // 流式消息开始时间，用于6分钟超时检查

	// Buffered sending mechanism
	ticker          *time.Ticker
	stopTicker      chan struct{}
	pendingSend     atomic.Bool
}

// StreamTimeout 流式消息超时时间（6分钟）
const StreamTimeout = 6 * time.Minute

// MaxContentBytes 单条消息最大字节数（企业微信 AI Bot SDK 限制：20480 字节）
const MaxContentBytes = 20480

// SendInterval 流式消息发送间隔（合并多个delta后批量发送）
const SendInterval = 300 * time.Millisecond // 300ms合并发送一次

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
	}

	// Start background ticker for buffered sending
	s.ticker = time.NewTicker(SendInterval)
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
// Uses fire-and-forget mode for intermediate updates to ensure smooth streaming
func (s *OutboundStream) flushBuffer() {
	if s.closed.Load() {
		return
	}

	s.mu.Lock()
	content := s.buffer.String()
	s.mu.Unlock()

	// Only send if there's new content since last send
	if len(content) > s.lastSentLen {
		// Use background context with short timeout for fire-and-forget
		// This ensures we don't block the stream waiting for ACK
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Send as intermediate update (finish=false) using fire-and-forget mode
		// Errors are logged but don't interrupt the stream
		if err := s.sendStreamUpdate(ctx, content, false); err != nil {
			s.logger.Debug("buffered send failed (non-critical)", slog.Any("error", err))
		}
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
			// Send final response with timeout message
			s.mu.Lock()
			content := s.buffer.String()
			if content == "" {
				content = "处理超时，请稍后再试。"
			}
			s.mu.Unlock()
			s.closed.Store(true)
			close(s.stopTicker)
			return s.sendStreamUpdate(ctx, content, true)
		}

		s.mu.Lock()
		s.buffer.WriteString(event.Delta)
		s.mu.Unlock()

		s.logger.Debug("stream delta buffered",
			slog.Int("delta_len", len(event.Delta)))

	case channel.StreamEventFinal:
		// Stop the ticker first
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

		// Send final response with finish=true
		return s.sendStreamUpdate(ctx, finalContent, true)

	case channel.StreamEventError:
		// Stop the ticker
		close(s.stopTicker)

		errorMsg := event.Error
		if errorMsg == "" {
			errorMsg = "处理消息时出错，请稍后再试。"
		}

		s.logger.Error("stream error", slog.String("error", event.Error))

		// Send error response immediately with finish=true
		if !s.sent.Load() {
			if err := s.sendStreamUpdate(ctx, errorMsg, true); err != nil {
				s.logger.Error("failed to send error response", slog.Any("error", err))
				return err
			}
			s.logger.Info("error response sent successfully")
			s.sent.Store(true)
		}

		s.closed.Store(true)
	}

	return nil
}

// sendStreamUpdate sends a stream update to WeCom
func (s *OutboundStream) sendStreamUpdate(ctx context.Context, content string, finish bool) error {
	// Don't send empty content unless it's the final message
	if content == "" && !finish {
		return nil
	}

	// Check if content exceeds byte limit (WeCom AI Bot SDK limit: 20480 bytes)
	contentBytes := []byte(content)
	if len(contentBytes) > MaxContentBytes {
		// Content too long, need to split and send in chunks
		// Note: sendSplitContent handles its own locking
		return s.sendSplitContent(ctx, content, finish)
	}

	// Send normally
	return s.sendSingleUpdate(ctx, content, finish)
}

// sendSplitContent splits long content into multiple messages and sends them sequentially.
// CRITICAL: All chunks must use the SAME streamID and reqID as the original message.
// WeCom identifies a stream sequence by (req_id, stream.id) pair.
func (s *OutboundStream) sendSplitContent(ctx context.Context, content string, finish bool) error {
	chunks := splitContentByBytes(content, MaxContentBytes-100) // Reserve space for continuation indicator

	s.logger.Info("splitting long content into chunks",
		slog.Int("total_chunks", len(chunks)),
		slog.Int("total_bytes", len(content)),
		slog.String("stream_id", s.streamID),
		slog.String("req_id", s.reqID))

	// IMPORTANT: Use the original streamID for all chunks
	// This ensures WeCom recognizes them as the same stream sequence
	for i, chunk := range chunks {
		isLastChunk := (i == len(chunks)-1)

		// Add continuation indicator if not the last chunk
		if !isLastChunk {
			chunk = chunk + "\n\n...(继续)"
		}

		// For split content:
		// - Intermediate chunks: finish=false (will wait for ACK)
		// - Last chunk: use the original finish value
		chunkFinish := isLastChunk && finish

		s.logger.Debug("sending chunk",
			slog.Int("chunk_index", i+1),
			slog.Int("total_chunks", len(chunks)),
			slog.Bool("is_last", isLastChunk),
			slog.Bool("finish", chunkFinish),
			slog.Int("content_bytes", len(chunk)))

		if err := s.sendChunk(ctx, chunk, chunkFinish); err != nil {
			s.logger.Error("failed to send chunk",
				slog.Int("chunk_index", i+1),
				slog.Any("error", err))
			return fmt.Errorf("send chunk %d/%d: %w", i+1, len(chunks), err)
		}

		// Add delay between chunks to avoid rate limiting
		// WeCom limit: 30 messages/minute
		if !isLastChunk {
			time.Sleep(300 * time.Millisecond)
		}
	}

	s.logger.Info("all chunks sent successfully",
		slog.Int("total_chunks", len(chunks)),
		slog.String("stream_id", s.streamID))

	return nil
}

// sendChunk sends a single chunk using the original streamID
// Uses dual-mode strategy:
//   - Intermediate chunks (finish=false): fire-and-forget for smooth streaming
//   - Final chunk (finish=true): wait for ACK to ensure delivery
func (s *OutboundStream) sendChunk(ctx context.Context, content string, finish bool) error {
	s.mu.Lock()

	if s.wsClient == nil {
		s.mu.Unlock()
		return fmt.Errorf("websocket client is nil")
	}

	wsClient := s.wsClient
	streamID := s.streamID
	reqID := s.reqID
	chatType := s.chatType
	isMentioned := s.isMentioned

	s.mu.Unlock()

	// Send stream update using WeCom stream format
	body := StreamMsgBody{
		MsgType: MsgTypeStream,
		Stream: StreamResponse{
			ID:      streamID, // Use the original streamID!
			Finish:  finish,
			Content: content,
		},
	}

	// Determine which command to use based on chat type and mention status
	cmd := CmdRespondMsg
	if chatType == "group" && !isMentioned {
		cmd = CmdSendMsg
	}

	// Dual-mode sending strategy for chunks
	if finish {
		// Final chunk: wait for ACK to ensure delivery
		if err := wsClient.SendStream(ctx, reqID, body, cmd); err != nil {
			return fmt.Errorf("send chunk: %w", err)
		}
		s.sent.Store(true)
		s.logger.Info("final chunk sent successfully",
			slog.String("stream_id", streamID),
			slog.Int("content_bytes", len(content)))
	} else {
		// Intermediate chunk: fire-and-forget for speed
		if err := wsClient.SendStreamFireAndForget(reqID, body, cmd); err != nil {
			return fmt.Errorf("send chunk: %w", err)
		}
	}

	return nil
}

// sendSingleUpdate sends a single stream update to WeCom
// Uses dual-mode strategy:
//   - Intermediate updates (finish=false): fire-and-forget for smooth streaming
//   - Final message (finish=true): wait for ACK to ensure delivery
func (s *OutboundStream) sendSingleUpdate(ctx context.Context, content string, finish bool) error {
	s.mu.Lock()

	if s.wsClient == nil {
		s.mu.Unlock()
		return fmt.Errorf("websocket client is nil")
	}

	wsClient := s.wsClient
	streamID := s.streamID
	reqID := s.reqID
	chatType := s.chatType
	isMentioned := s.isMentioned

	s.mu.Unlock()

	s.logger.Debug("sending stream update",
		slog.String("stream_id", streamID),
		slog.Int("content_bytes", len(content)),
		slog.Bool("finish", finish))

	// Send stream update using WeCom stream format
	body := StreamMsgBody{
		MsgType: MsgTypeStream,
		Stream: StreamResponse{
			ID:      streamID,
			Finish:  finish,
			Content: content,
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

	// Dual-mode sending strategy:
	// - Intermediate updates: use fire-and-forget for smooth streaming experience
	// - Final message: use ACK-wait to ensure delivery
	if finish {
		// Final message: wait for ACK to ensure delivery
		if err := wsClient.SendStream(ctx, reqID, body, cmd); err != nil {
			return fmt.Errorf("send stream update: %w", err)
		}
		s.sent.Store(true)
		s.logger.Info("final stream response sent successfully",
			slog.String("stream_id", streamID),
			slog.String("cmd", cmd),
			slog.Int("content_bytes", len(content)))
	} else {
		// Intermediate update: fire-and-forget for speed
		// Errors are logged but not returned to avoid disrupting the stream
		if err := wsClient.SendStreamFireAndForget(reqID, body, cmd); err != nil {
			s.logger.Debug("intermediate update send failed (non-critical)",
				slog.Any("error", err),
				slog.String("stream_id", streamID))
			// Don't return error for intermediate updates - keep streaming
		}
	}

	s.mu.Lock()
	s.lastSentLen = len(content)
	s.mu.Unlock()

	return nil
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
	s.mu.Unlock()

	if content == "" {
		content = "处理完成，但没有生成回复内容。"
	}

	// Send final response with finish=true
	return s.sendStreamUpdate(ctx, content, true)
}

// generateStreamID generates a unique stream ID
func generateStreamID() string {
	return fmt.Sprintf("stream_%d", time.Now().UnixNano())
}

// splitContentByBytes splits content into chunks respecting byte limit.
// It prioritizes splitting at paragraph boundaries, then line boundaries,
// then sentence boundaries, and finally at word boundaries.
func splitContentByBytes(content string, maxBytes int) []string {
	contentBytes := []byte(content)
	if len(contentBytes) <= maxBytes {
		return []string{content}
	}

	var chunks []string
	remaining := content

	for len(remaining) > 0 {
		remainingBytes := []byte(remaining)
		if len(remainingBytes) <= maxBytes {
			chunks = append(chunks, remaining)
			break
		}

		// Find the best split point within maxBytes
		chunk := truncateByBytes(remaining, maxBytes)

		// Try to split at paragraph boundary (\n\n)
		if idx := strings.LastIndex(chunk, "\n\n"); idx > maxBytes/2 {
			chunk = remaining[:idx]
			chunks = append(chunks, chunk)
			remaining = remaining[idx+2:]
			continue
		}

		// Try to split at line boundary (\n)
		if idx := strings.LastIndex(chunk, "\n"); idx > maxBytes/2 {
			chunk = remaining[:idx]
			chunks = append(chunks, chunk)
			remaining = remaining[idx+1:]
			continue
		}

		// Try to split at sentence boundary (Chinese and English punctuation)
		if idx := strings.LastIndexAny(chunk, "。！？.!?"); idx > maxBytes/2 {
			chunk = remaining[:idx+1]
			chunks = append(chunks, chunk)
			remaining = remaining[idx+1:]
			continue
		}

		// Force split (ensure we don't cut a UTF-8 character)
		chunks = append(chunks, chunk)
		remaining = remaining[len(chunk):]
	}

	return chunks
}

// truncateByBytes truncates a string to maxBytes without breaking UTF-8 characters.
// It looks backward from maxBytes to find a valid UTF-8 character boundary.
func truncateByBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	// Look backward to find a valid UTF-8 character boundary
	// In UTF-8, continuation bytes start with 10xxxxxx (0x80-0xBF)
	// Non-continuation bytes start with 0xxxxxxx (0x00-0x7F) or 11xxxxxx (0xC0-0xFF)
	for i := maxBytes; i > maxBytes-4 && i > 0; i-- {
		if (s[i] & 0xC0) != 0x80 {
			return s[:i]
		}
	}
	return s[:maxBytes]
}
