package wecom

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	// defaultPendingTTL 默认 pending 消息过期时间
	defaultPendingTTL = 5 * time.Minute
	// defaultMaxPending 默认最大 pending 消息数量
	defaultMaxPending = 50
	// defaultCleanupInterval 默认清理间隔
	defaultCleanupInterval = 30 * time.Second
)

// PendingMessage 表示一个待确认发送的消息
type PendingMessage struct {
	ID        string                 // 消息唯一标识
	ReqID     string                 // 原始请求的 req_id
	ChatID    string                 // 目标聊天 ID
	UserID    string                 // 目标用户 ID
	ChatType  string                 // 聊天类型
	Content   string                 // 消息内容
	IsStream  bool                   // 是否是流式消息
	StreamID  string                 // 流式消息 ID
	Finish    bool                   // 是否是结束消息
	Metadata  map[string]interface{} // 额外元数据
	CreatedAt time.Time              // 创建时间
	ExpiresAt time.Time              // 过期时间
	RetryCount int                   // 重试次数
}

// PendingReplyManager 管理待确认的发送消息
// 当 WebSocket 断连时，这些消息会在重连后通过 Agent API 补发
type PendingReplyManager struct {
	mu       sync.RWMutex
	pending  map[string]*PendingMessage // ID -> PendingMessage
	ttl      time.Duration
	maxCount int
	logger   *slog.Logger

	// 清理定时器
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// NewPendingReplyManager 创建新的 Pending Reply 管理器
func NewPendingReplyManager(logger *slog.Logger) *PendingReplyManager {
	return NewPendingReplyManagerWithOptions(logger, defaultMaxPending, defaultPendingTTL)
}

// NewPendingReplyManagerWithOptions 创建带有自定义选项的管理器
func NewPendingReplyManagerWithOptions(logger *slog.Logger, maxCount int, ttl time.Duration) *PendingReplyManager {
	if maxCount <= 0 {
		maxCount = defaultMaxPending
	}
	if ttl <= 0 {
		ttl = defaultPendingTTL
	}

	m := &PendingReplyManager{
		pending:     make(map[string]*PendingMessage),
		ttl:         ttl,
		maxCount:    maxCount,
		logger:      logger.With(slog.String("component", "pending_reply")),
		stopCleanup: make(chan struct{}),
	}

	// 启动后台清理任务
	m.startCleanup()

	return m
}

// startCleanup 启动后台清理任务
func (m *PendingReplyManager) startCleanup() {
	m.cleanupTicker = time.NewTicker(defaultCleanupInterval)
	go func() {
		for {
			select {
			case <-m.cleanupTicker.C:
				m.cleanup()
			case <-m.stopCleanup:
				m.cleanupTicker.Stop()
				return
			}
		}
	}()
}

// Stop 停止管理器
func (m *PendingReplyManager) Stop() {
	close(m.stopCleanup)
}

// Add 添加一个 pending 消息
func (m *PendingReplyManager) Add(msg *PendingMessage) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果达到最大数量，移除最老的消息
	if len(m.pending) >= m.maxCount {
		m.removeOldest()
	}

	// 生成唯一 ID
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("pending_%d_%s", time.Now().UnixNano(), msg.ReqID)
	}

	// 设置时间
	now := time.Now()
	msg.CreatedAt = now
	msg.ExpiresAt = now.Add(m.ttl)

	m.pending[msg.ID] = msg

	m.logger.Debug("pending message added",
		slog.String("id", msg.ID),
		slog.String("req_id", msg.ReqID),
		slog.String("chat_id", msg.ChatID),
		slog.Int("total_pending", len(m.pending)))

	return msg.ID
}

// Remove 移除一个 pending 消息
func (m *PendingReplyManager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.pending[id]; exists {
		delete(m.pending, id)
		m.logger.Debug("pending message removed",
			slog.String("id", id),
			slog.Int("total_pending", len(m.pending)))
	}
}

// Get 获取一个 pending 消息
func (m *PendingReplyManager) Get(id string) (*PendingMessage, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	msg, exists := m.pending[id]
	return msg, exists
}

// GetAllPending 获取所有未过期的 pending 消息
func (m *PendingReplyManager) GetAllPending() []*PendingMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	result := make([]*PendingMessage, 0)

	for _, msg := range m.pending {
		if msg.ExpiresAt.After(now) {
			result = append(result, msg)
		}
	}

	return result
}

// GetPendingByChatID 获取指定聊天 ID 的所有 pending 消息
func (m *PendingReplyManager) GetPendingByChatID(chatID string) []*PendingMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	result := make([]*PendingMessage, 0)

	for _, msg := range m.pending {
		if msg.ChatID == chatID && msg.ExpiresAt.After(now) {
			result = append(result, msg)
		}
	}

	return result
}

// MarkAsSent 标记消息为已发送（成功收到 ACK）
func (m *PendingReplyManager) MarkAsSent(reqID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 查找匹配的 req_id
	for id, msg := range m.pending {
		if msg.ReqID == reqID {
			delete(m.pending, id)
			m.logger.Debug("pending message marked as sent",
				slog.String("id", id),
				slog.String("req_id", reqID))
			return
		}
	}
}

// cleanup 清理过期的 pending 消息
func (m *PendingReplyManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	expiredCount := 0

	for id, msg := range m.pending {
		if msg.ExpiresAt.Before(now) {
			delete(m.pending, id)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		m.logger.Debug("cleaned up expired pending messages",
			slog.Int("expired_count", expiredCount),
			slog.Int("remaining", len(m.pending)))
	}
}

// removeOldest 移除最老的 pending 消息
func (m *PendingReplyManager) removeOldest() {
	var oldestID string
	var oldestTime time.Time

	for id, msg := range m.pending {
		if oldestID == "" || msg.CreatedAt.Before(oldestTime) {
			oldestID = id
			oldestTime = msg.CreatedAt
		}
	}

	if oldestID != "" {
		delete(m.pending, oldestID)
		m.logger.Warn("removed oldest pending message due to capacity limit",
			slog.String("id", oldestID))
	}
}

// Size 返回当前 pending 消息数量
func (m *PendingReplyManager) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.pending)
}

// Clear 清空所有 pending 消息
func (m *PendingReplyManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pending = make(map[string]*PendingMessage)
	m.logger.Info("all pending messages cleared")
}

// RetryFailedMessages 重试发送失败的 pending 消息
// 返回成功重试的数量和错误
func (m *PendingReplyManager) RetryFailedMessages(ctx context.Context, sendFunc func(*PendingMessage) error) (int, error) {
	pending := m.GetAllPending()
	if len(pending) == 0 {
		return 0, nil
	}

	m.logger.Info("retrying failed pending messages",
		slog.Int("count", len(pending)))

	successCount := 0
	var lastErr error

	for _, msg := range pending {
		if msg.RetryCount >= 3 {
			m.logger.Warn("pending message exceeded max retries, removing",
				slog.String("id", msg.ID),
				slog.Int("retry_count", msg.RetryCount))
			m.Remove(msg.ID)
			continue
		}

		// 增加重试计数
		msg.RetryCount++

		if err := sendFunc(msg); err != nil {
			m.logger.Error("failed to retry pending message",
				slog.String("id", msg.ID),
				slog.Int("retry_count", msg.RetryCount),
				slog.Any("error", err))
			lastErr = err
		} else {
			m.Remove(msg.ID)
			successCount++
		}
	}

	m.logger.Info("retry completed",
		slog.Int("success", successCount),
		slog.Int("failed", len(pending)-successCount))

	return successCount, lastErr
}
