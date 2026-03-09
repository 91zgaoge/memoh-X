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
	adapter     *Adapter
	cfg         channel.ChannelConfig
	wsClient    *WebSocketClient
	reqID       string
	chatID      string
	userID      string
	logger      *slog.Logger

	buffer      strings.Builder
	closed      atomic.Bool
	sent        atomic.Bool
	streamID    string
	mu          sync.Mutex
	lastSentLen int
	lastSendTime time.Time
	minInterval  time.Duration
}

// NewOutboundStream creates a new outbound stream
func NewOutboundStream(adapter *Adapter, cfg channel.ChannelConfig, wsClient *WebSocketClient, reqID, chatID, userID string, logger *slog.Logger) *OutboundStream {
	return &OutboundStream{
		adapter:      adapter,
		cfg:          cfg,
		wsClient:     wsClient,
		reqID:        reqID,
		chatID:       chatID,
		userID:       userID,
		streamID:     generateStreamID(),
		logger:       logger.With(slog.String("component", "wecom_stream"), slog.String("req_id", reqID)),
		minInterval:  10 * time.Millisecond,
		lastSendTime: time.Now(),
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
		s.mu.Lock()
		s.buffer.WriteString(event.Delta)
		currentContent := s.buffer.String()
		s.mu.Unlock()

		s.logger.Debug("stream delta",
			slog.Int("buffer_len", len(currentContent)),
			slog.Int("delta_len", len(event.Delta)))

		// Send streaming update immediately for low latency
		// WeCom will overwrite previous messages with same stream ID
		if err := s.sendStreamUpdate(ctx, currentContent, false); err != nil {
			s.logger.Warn("failed to send stream update", slog.Any("error", err))
		}

	case channel.StreamEventFinal:
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

// shouldSendUpdate checks if enough time has passed since last send
func (s *OutboundStream) shouldSendUpdate() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if now.Sub(s.lastSendTime) >= s.minInterval {
		return true
	}
	return false
}

// sendStreamUpdate sends a stream update to WeCom
func (s *OutboundStream) sendStreamUpdate(ctx context.Context, content string, finish bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.wsClient == nil {
		return fmt.Errorf("websocket client is nil")
	}

	// Don't send empty content unless it's the final message
	if content == "" && !finish {
		return nil
	}

	// Truncate if too long (WeCom has limits)
	if len(content) > 4000 {
		content = content[:4000] + "\n\n... (内容已截断)"
	}

	s.logger.Debug("sending stream update",
		slog.String("stream_id", s.streamID),
		slog.Int("content_len", len(content)),
		slog.Bool("finish", finish))

	// Send stream update using WeCom stream format
	body := StreamMsgBody{
		MsgType: MsgTypeStream,
		Stream: StreamResponse{
			ID:      s.streamID,
			Finish:  finish,
			Content: content,
		},
	}

	// Use fast path for streaming (no ACK wait) to improve latency
	// Intermediate updates use async SendStream for low latency
	// Final message uses SendReply to ensure delivery (waits for ACK)
	if finish {
		// Final message - use SendReply which waits for ACK to ensure delivery
		// This prevents message truncation issues
		if err := s.wsClient.SendReply(ctx, s.reqID, body, CmdRespondMsg); err != nil {
			return fmt.Errorf("send stream response: %w", err)
		}
		s.sent.Store(true)
		s.logger.Info("final stream response sent successfully",
			slog.String("stream_id", s.streamID),
			slog.Int("content_len", len(content)))
	} else {
		// Intermediate update - use SendStream for low latency
		if err := s.wsClient.SendStream(ctx, s.reqID, body); err != nil {
			return fmt.Errorf("send stream update: %w", err)
		}
	}

	s.lastSendTime = time.Now()
	s.lastSentLen = len(content)

	return nil
}

// Close sends the final response and closes the stream
func (s *OutboundStream) Close(ctx context.Context) error {
	if s.closed.Load() || s.sent.Load() {
		return nil
	}
	s.closed.Store(true)

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
