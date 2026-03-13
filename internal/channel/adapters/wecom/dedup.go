package wecom

import (
	"sync"
	"time"
)

const (
	// defaultCapacity 默认缓存容量
	defaultCapacity = 1000
	// defaultTTL 默认消息过期时间
	defaultTTL = 5 * time.Minute
)

// DedupManager 消息去重管理器
// 使用 LRU + TTL 机制防止重复处理相同消息
type DedupManager struct {
	mu       sync.RWMutex
	cache    map[string]time.Time // 消息ID -> 过期时间
	capacity int                  // 最大容量
	ttl      time.Duration        // 过期时间
}

// NewDedupManager 创建新的去重管理器
func NewDedupManager() *DedupManager {
	return &DedupManager{
		cache:    make(map[string]time.Time),
		capacity: defaultCapacity,
		ttl:      defaultTTL,
	}
}

// NewDedupManagerWithOptions 创建带有自定义选项的去重管理器
func NewDedupManagerWithOptions(capacity int, ttl time.Duration) *DedupManager {
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &DedupManager{
		cache:    make(map[string]time.Time),
		capacity: capacity,
		ttl:      ttl,
	}
}

// IsDuplicate 检查消息是否重复
// reqID: WebSocket 请求的 req_id
// msgID: 企业微信消息的 msgid
// 返回: true 表示是重复消息，false 表示新消息
func (d *DedupManager) IsDuplicate(reqID, msgID string) bool {
	// 使用 reqID 和 msgID 的组合作为键
	key := d.makeKey(reqID, msgID)

	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// 清理过期条目
	d.cleanupExpired(now)

	// 检查是否存在
	if expireTime, exists := d.cache[key]; exists && expireTime.After(now) {
		return true
	}

	// 添加新条目
	d.cache[key] = now.Add(d.ttl)
	return false
}

// makeKey 生成缓存键
func (d *DedupManager) makeKey(reqID, msgID string) string {
	// 优先使用 msgID，如果为空则使用 reqID
	if msgID != "" {
		return "msg:" + msgID
	}
	return "req:" + reqID
}

// cleanupExpired 清理过期条目
func (d *DedupManager) cleanupExpired(now time.Time) {
	// 当容量超过 80% 时触发清理
	if len(d.cache) < int(float64(d.capacity)*0.8) {
		return
	}

	for key, expireTime := range d.cache {
		if expireTime.Before(now) {
			delete(d.cache, key)
		}
	}

	// 如果仍然超过容量，移除最老的条目（简单的 LRU 实现）
	if len(d.cache) > d.capacity {
		d.removeOldest(len(d.cache) - d.capacity)
	}
}

// removeOldest 移除最老的 n 个条目
func (d *DedupManager) removeOldest(n int) {
	type item struct {
		key    string
		expire time.Time
	}

	// 收集所有条目
	items := make([]item, 0, len(d.cache))
	for k, v := range d.cache {
		items = append(items, item{key: k, expire: v})
	}

	// 按过期时间排序（最早的在前）
	for i := 0; i < n && i < len(items); i++ {
		oldestIdx := i
		for j := i + 1; j < len(items); j++ {
			if items[j].expire.Before(items[oldestIdx].expire) {
				oldestIdx = j
			}
		}
		// 删除最老的
		delete(d.cache, items[oldestIdx].key)
		// 交换到已处理位置
		items[i], items[oldestIdx] = items[oldestIdx], items[i]
	}
}

// Size 返回当前缓存大小
func (d *DedupManager) Size() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.cache)
}

// Clear 清空缓存
func (d *DedupManager) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cache = make(map[string]time.Time)
}
