# 删除对话消息缓存功能记录

## 操作信息
- **日期**: 2026-03-19
- **操作人员**: Claude Code
- **原因**: 缓存功能与其他功能冲突，造成严重问题

## 问题描述

Memoh 项目最近添加了对话消息缓存功能（`internal/conversation/flow/cache.go`），用于缓存常见的查询响应以提高性能。但该功能与其他功能冲突，造成严重问题：

- 相同查询返回缓存结果而不是实时生成
- 破坏了对话的上下文感知能力
- 导致某些功能无法正常工作

## 删除的文件

### 1. `internal/conversation/flow/cache.go`
完整删除了该文件，包含：
- `ResponseCache` 结构体定义
- `cacheEntry` 结构体定义
- `NewResponseCache()` 构造函数
- `Get()` 缓存查询方法
- `Set()` 缓存存储方法
- `evictOldest()` 淘汰最旧条目方法
- `cleanupLoop()` 后台清理循环
- `cleanup()` 清理过期条目方法
- `Stats()` 统计方法
- `Clear()` 清空缓存方法

## 修改的文件

### 2. `internal/conversation/flow/resolver.go`

#### 删除的字段（第247行）
```go
// 删除前:
cache            *ResponseCache // [PERF] 响应缓存

// 删除后: 该字段已完全移除
```

#### 删除的初始化代码（第298行）
```go
// 删除前:
cache: NewResponseCache(1000, 5*time.Minute, log), // [PERF] 初始化响应缓存

// 删除后: 该行已完全移除
```

#### 删除的缓存查询逻辑（原第916-924行）
```go
// 删除前:
// [PERF] 检查缓存，如果是常见查询直接返回缓存结果
if r.cache != nil && req.Query != "" {
    if cachedResp, hit := r.cache.Get(req.BotID, req.ChatID, req.Query); hit {
        r.logger.Info("[PERF] Chat: returning cached response",
            slog.String("bot_id", req.BotID),
            slog.String("query", truncateString(req.Query, 50)))
        return cachedResp, nil
    }
}

// 删除后: 该代码块已完全移除
```

#### 删除的缓存存储逻辑（原第1037-1043行）
```go
// 删除前:
// [PERF] 将响应存入缓存
if r.cache != nil && req.Query != "" {
    r.cache.Set(req.BotID, req.ChatID, req.Query, chatResp)
    r.logger.Debug("[PERF] Chat: response cached",
        slog.String("bot_id", req.BotID),
        slog.String("query", truncateString(req.Query, 50)))
}

// 删除后: 该代码块已完全移除
```

## 验证步骤

1. **文件已删除**:
   ```bash
   ls -la internal/conversation/flow/cache.go
   # 输出: No such file or directory
   ```

2. **代码已清理**:
   ```bash
   grep -r "ResponseCache\|r\.cache\|NewResponseCache" internal/conversation/flow/
   # 无匹配结果（除无关注释外）
   ```

3. **Git 状态**:
   ```bash
   git status
   # 显示 cache.go 已删除，resolver.go 已修改
   ```

## 回滚方法

如需恢复缓存功能，可通过以下命令回滚：

```bash
# 从 git 历史恢复 cache.go 文件
git checkout HEAD -- internal/conversation/flow/cache.go

# 然后手动恢复 resolver.go 中的缓存相关代码
# 或从 git 恢复整个文件
git checkout HEAD -- internal/conversation/flow/resolver.go
```

## 后续建议

1. **测试验证**: 部署后测试对话功能，确保相同查询不会返回缓存结果
2. **性能监控**: 观察删除缓存后的性能影响
3. **替代方案**: 如需性能优化，考虑：
   - 优化 LLM 调用响应时间
   - 改进消息加载逻辑
   - 使用更细粒度的缓存策略（如仅缓存特定类型的查询）
