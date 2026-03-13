package wecom

import (
	"log/slog"
	"sync"
	"time"
)

const (
	// QuotaWindow 配额追踪窗口（24小时）
	QuotaWindow = 24 * time.Hour
	// MaxPassiveReplies 24小时内最大被动回复数量（企业微信限制）
	MaxPassiveReplies = 30
	// WarningThreshold 警告阈值（达到此值时记录警告）
	WarningThreshold = 24
	// QuotaCleanupInterval 配额清理间隔
	QuotaCleanupInterval = 5 * time.Minute
)

// ChatQuota 记录单个聊天的配额使用情况
type ChatQuota struct {
	ChatID       string
	ReplyCount   int
	FirstReplyAt time.Time
	LastReplyAt  time.Time
}

// QuotaTracker 被动回复配额追踪器
// 追踪每个 chat 在 24 小时窗口内的被动回复数量
type QuotaTracker struct {
	mu     sync.RWMutex
	quotas map[string]*ChatQuota // chat_id -> ChatQuota
	logger *slog.Logger
}

// NewQuotaTracker 创建新的配额追踪器
func NewQuotaTracker(logger *slog.Logger) *QuotaTracker {
	return &QuotaTracker{
		quotas: make(map[string]*ChatQuota),
		logger: logger.With(slog.String("component", "quota_tracker")),
	}
}

// RecordReply 记录一次被动回复
// 返回当前配额状态和是否超出限制
func (q *QuotaTracker) RecordReply(chatID string) (quota *ChatQuota, remaining int, atLimit bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	quota, exists := q.quotas[chatID]

	if !exists || now.Sub(quota.FirstReplyAt) > QuotaWindow {
		// 新窗口或首次记录
		quota = &ChatQuota{
			ChatID:       chatID,
			ReplyCount:   1,
			FirstReplyAt: now,
			LastReplyAt:  now,
		}
		q.quotas[chatID] = quota
	} else {
		// 同一窗口内
		quota.ReplyCount++
		quota.LastReplyAt = now
	}

	remaining = MaxPassiveReplies - quota.ReplyCount
	atLimit = quota.ReplyCount >= MaxPassiveReplies

	// 记录日志
	if quota.ReplyCount == WarningThreshold {
		q.logger.Warn("chat approaching passive reply limit",
			slog.String("chat_id", chatID),
			slog.Int("used", quota.ReplyCount),
			slog.Int("remaining", remaining),
			slog.Int("limit", MaxPassiveReplies))
	} else if atLimit {
		q.logger.Error("chat reached passive reply limit",
			slog.String("chat_id", chatID),
			slog.Int("used", quota.ReplyCount),
			slog.Int("limit", MaxPassiveReplies),
			slog.Time("window_reset", quota.FirstReplyAt.Add(QuotaWindow)))
	} else {
		q.logger.Debug("reply quota recorded",
			slog.String("chat_id", chatID),
			slog.Int("used", quota.ReplyCount),
			slog.Int("remaining", remaining))
	}

	return quota, remaining, atLimit
}

// GetQuota 获取指定聊天的配额信息
func (q *QuotaTracker) GetQuota(chatID string) *ChatQuota {
	q.mu.RLock()
	defer q.mu.RUnlock()

	quota, exists := q.quotas[chatID]
	if !exists {
		return nil
	}

	// 检查窗口是否过期
	if time.Since(quota.FirstReplyAt) > QuotaWindow {
		return nil
	}

	return quota
}

// GetRemaining 获取指定聊天的剩余配额
func (q *QuotaTracker) GetRemaining(chatID string) int {
	quota := q.GetQuota(chatID)
	if quota == nil {
		return MaxPassiveReplies
	}
	remaining := MaxPassiveReplies - quota.ReplyCount
	if remaining < 0 {
		return 0
	}
	return remaining
}

// IsAtLimit 检查指定聊天是否已达到配额限制
func (q *QuotaTracker) IsAtLimit(chatID string) bool {
	quota := q.GetQuota(chatID)
	if quota == nil {
		return false
	}
	return quota.ReplyCount >= MaxPassiveReplies
}

// CanSendReply 检查是否可以发送被动回复
func (q *QuotaTracker) CanSendReply(chatID string) bool {
	return !q.IsAtLimit(chatID)
}

// GetQuotaInfo 获取配额详细信息
func (q *QuotaTracker) GetQuotaInfo(chatID string) map[string]interface{} {
	quota := q.GetQuota(chatID)
	if quota == nil {
		return map[string]interface{}{
			"chat_id":        chatID,
			"used":           0,
			"limit":          MaxPassiveReplies,
			"remaining":      MaxPassiveReplies,
			"at_limit":       false,
			"window_start":   nil,
			"window_reset":   nil,
			"usage_percent":  0,
		}
	}

	remaining := MaxPassiveReplies - quota.ReplyCount
	if remaining < 0 {
		remaining = 0
	}

	usagePercent := float64(quota.ReplyCount) * 100 / float64(MaxPassiveReplies)

	return map[string]interface{}{
		"chat_id":        chatID,
		"used":           quota.ReplyCount,
		"limit":          MaxPassiveReplies,
		"remaining":      remaining,
		"at_limit":       quota.ReplyCount >= MaxPassiveReplies,
		"window_start":   quota.FirstReplyAt,
		"window_reset":   quota.FirstReplyAt.Add(QuotaWindow),
		"last_reply":     quota.LastReplyAt,
		"usage_percent":  usagePercent,
	}
}

// CleanupExpired 清理过期的配额记录
func (q *QuotaTracker) CleanupExpired() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	expiredCount := 0

	for chatID, quota := range q.quotas {
		if now.Sub(quota.FirstReplyAt) > QuotaWindow {
			delete(q.quotas, chatID)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		q.logger.Debug("cleaned up expired quota records",
			slog.Int("count", expiredCount),
			slog.Int("remaining", len(q.quotas)))
	}

	return expiredCount
}

// GetAllQuotas 获取所有活跃的配额记录
func (q *QuotaTracker) GetAllQuotas() map[string]*ChatQuota {
	q.mu.RLock()
	defer q.mu.RUnlock()

	now := time.Now()
	result := make(map[string]*ChatQuota)

	for chatID, quota := range q.quotas {
		if now.Sub(quota.FirstReplyAt) <= QuotaWindow {
			// 创建副本
			result[chatID] = &ChatQuota{
				ChatID:       quota.ChatID,
				ReplyCount:   quota.ReplyCount,
				FirstReplyAt: quota.FirstReplyAt,
				LastReplyAt:  quota.LastReplyAt,
			}
		}
	}

	return result
}

// ResetQuota 重置指定聊天的配额
func (q *QuotaTracker) ResetQuota(chatID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.quotas, chatID)
	q.logger.Info("quota reset", slog.String("chat_id", chatID))
}

// Clear 清空所有配额记录
func (q *QuotaTracker) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.quotas = make(map[string]*ChatQuota)
	q.logger.Info("all quota records cleared")
}

// Size 返回当前配额记录数量
func (q *QuotaTracker) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.quotas)
}

// GetQuotaStatusMessage 获取配额状态提示消息
func GetQuotaStatusMessage(remaining int) string {
	if remaining <= 0 {
		return "⚠️ 今日被动回复配额已用完（30条/24小时），请明日再试或使用主动发送功能。"
	}
	if remaining <= 5 {
		return "⚠️ 今日被动回复配额即将用完（剩余 " + string(rune('0'+remaining)) + " 条/30条），请注意使用。"
	}
	return ""
}
