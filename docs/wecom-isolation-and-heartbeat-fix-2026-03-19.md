# WeCom 消息对话隔离 + 心跳任务隔离修复

## 背景

2026-03-19 期间，Memoh 企业微信机器人出现严重的消息串扰问题：
1. 用户 A 的消息被回复给用户 B
2. 单聊用户共享同一条路由，历史消息混合
3. `/new` 命令清空所有用户的历史
4. 心跳维护任务内容混入用户正常回复
5. 上下文大小超限（288K tokens > 256K limit）

---

## 一、WeCom 消息串扰修复

### 1.1 问题分析

**根本原因**: WeCom SDK 规范中，`chatid` 在单聊时为空字符串，群聊时为群 ID。原有代码直接使用 `body.ChatID` 作为 `Conversation.ID`，导致所有单聊用户共享空字符串 ID。

```go
// 原有代码（有 bug）
Conversation: channel.Conversation{
    ID:   body.ChatID,   // 单聊时 = "" （空）
    Type: body.ChatType,
}
```

**影响**:
- `bot_channel_routes` 表中 `external_conversation_id = ""`，所有单聊用户共享同一条路由
- Agent 历史通过 `route_id` 分区，但所有人用同一个 `route_id`
- `/new` 历史清除无法精确针对某个用户

### 1.2 修复方案

**文件**: `internal/channel/adapters/wecom/adapter.go`

**主消息处理**（约 940 行）：
```go
// 单聊时用 UserID 作为 Conversation.ID
conversationID := body.ChatID
if conversationID == "" {
    conversationID = body.From.UserID
}
isGroup := body.ChatType == "group"

senderMeta := map[string]any{
    "req_id":       wsMsg.Headers.ReqID,  // 关键：用于区分 CmdRespondMsg vs CmdSendMsg
    "from_user_id": body.From.UserID,
    "user_id":      body.From.UserID,
    "chat_type":    body.ChatType,
    "chattype":     body.ChatType,
    "chat_id":      body.ChatID,
    "is_group":     isGroup,
}
if body.From.Name != "" {
    senderMeta["sender_name"] = body.From.Name
}
if body.From.CorpID != "" {
    senderMeta["corp_id"] = body.From.CorpID
}

msg := channel.InboundMessage{
    Conversation: channel.Conversation{
        ID:   conversationID,  // userid for single; group chatid for group
        Type: body.ChatType,
        Metadata: map[string]any{
            "chat_type":    body.ChatType,
            "chattype":     body.ChatType,
            "chat_id":      body.ChatID,
            "from_user_id": body.From.UserID,
            "is_group":     isGroup,
        },
    },
    Metadata: senderMeta,
    Message: channel.Message{
        ID:       body.MsgID,
        Metadata: senderMeta,
    },
    // ...
}
```

**handleMixedContent 函数**（约 1740 行）：应用同样的修复。

### 1.3 broadcastToOtherChannels 跨用户广播禁用

**文件**: `internal/channel/inbound/channel.go`

**问题**: `activeChatID = botID`，`ListChatRoutes(botID)` 返回所有路由，回复被发给所有用户。

**修复**: 注释掉两处 `go p.broadcastToOtherChannels(...)` 调用（L537、L1440）。

### 1.4 群聊 debounce key 混用修复

**文件**: `internal/channel/inbound/channel.go` L218

```go
// 修复前
// debounceKey := botID + ":" + activeChatID  // activeChatID = botID，所有群共享

// 修复后
debounceKey := strings.TrimSpace(identity.BotID) + ":" + strings.TrimSpace(msg.Conversation.ID)
```

### 1.5 对话身份 Metadata 补全

在 `InboundMessage` 的 `Conversation.Metadata`、`Message.Metadata`、顶层 `Metadata` 中加入：

| 字段 | 说明 |
|------|------|
| `from_user_id` | 发送者 userid（主字段） |
| `user_id` | 同上（兼容旧代码） |
| `sender_name` | 发送者姓名（如有） |
| `corp_id` | 企业 ID（如有） |
| `chat_type` | `"single"` 或 `"group"` |
| `chattype` | 同上（兼容旧代码） |
| `chat_id` | 原始 SDK chatid（单聊为空，群聊为群 ID） |
| `is_group` | bool，方便下游判断 |
| `req_id` | WebSocket 请求 ID，用于 WeCom 回复模式选择 |

---

## 二、/new 命令精确清除历史

### 2.1 问题
原有 `/new` 命令调用 `messageService.DeleteByBot(ctx, cfg.BotID)`，清空该 Bot 下所有用户的历史。

### 2.2 修复

**文件**: `internal/channel/adapters/wecom/adapter.go`

**新增接口**:
- `message.Service.DeleteByRoute(ctx context.Context, routeID string) error`
- `sqlc.Queries.DeleteMessagesByRoute(ctx context.Context, routeID pgtype.UUID) error`

**handleNewChatCommand** 修改:
```go
if a.routeService != nil {
    convID := body.ChatID
    if convID == "" {
        convID = body.From.UserID
    }
    r, err := a.routeService.Find(ctx, cfg.BotID, "wecom", convID, "")
    if err == nil && r.ID != "" {
        if delErr := a.messageService.DeleteByRoute(ctx, r.ID); delErr == nil {
            cleared = true
        }
    }
}
if !cleared {
    a.messageService.DeleteByBot(ctx, cfg.BotID) // fallback
}
```

**依赖注入**:
- `Adapter.SetRouteService()` 方法
- `cmd/agent/main.go provideChannelRegistry` 传入 `routeService`

---

## 三、心跳任务隔离修复

### 3.1 问题分析

**现象**: 用户发送 "你好啊"，Bot 回复心跳维护任务内容：
```
收到心跳维护任务。正在执行后台检查：
检查待办任务状态
回顾近期对话摘要
评估是否需要更新笔记或日程
...
维护完成。系统运行正常。
```

**根本原因链**:
1. Heartbeat `fire()` → `TriggerHeartbeat()` 使用 `ChatID = botID`（与用户 DM 相同）
2. `executeTrigger` 调用 `storeRound()` 将维护结果写入 `bot_history_messages`
3. 用户下次消息加载历史时，`ListLatest(botID)` 返回包含心跳消息的混合历史
4. LLM 看到维护内容并重复它

### 3.2 修复方案

#### 3.2.1 禁止心跳任务加载用户历史

**文件**: `internal/conversation/flow/resolver.go` `executeTrigger()`

```go
isHeartbeat := p.schedule.TriggerType == "heartbeat"
taskType := "schedule"
if isHeartbeat {
    taskType = "heartbeat"
}
// Heartbeat tasks must NOT load user conversation history
maxCtxLoadTime := 0
if isHeartbeat {
    maxCtxLoadTime = -1  // -1 触发 skipHistory = true
}

req := conversation.ChatRequest{
    BotID:                p.botID,
    ChatID:               p.botID,
    Query:                p.query,
    UserID:               p.ownerUserID,
    Token:                token,
    HistoryLimitOverride: p.historyLimitOverride,
    CurrentChannel:       p.platform,
    ReplyTarget:          p.replyTarget,
    TaskType:             taskType,
    MaxContextLoadTime:   maxCtxLoadTime,  // 心跳任务禁用历史加载
}
```

#### 3.2.2 禁止心跳任务存储结果到历史

**文件**: `internal/conversation/flow/resolver.go` `executeTrigger()`

```go
// Skip persisting heartbeat rounds: maintenance results must not pollute user
// conversation history. Heartbeat tasks are stateless background jobs.
if !isHeartbeat {
    if err := r.storeRound(ctx, req, resp.Messages, resp.Usage); err != nil {
        r.logger.Warn("executeTrigger: storeRound failed", ...)
        return err
    }
}
```

### 3.3 数据清理

删除已存在的心跳污染消息：
```sql
DELETE FROM bot_history_messages
WHERE bot_id = 'ae1e38dc-b530-4b99-982b-ecdf77ffd9cc'
  AND route_id IS NULL;
-- 删除 11 条记录
```

---

## 四、上下文大小超限修复

### 4.1 问题
错误信息：
```
Error: LLM API error: 400 {"error":{"code":400,"message":"request (288494 tokens)
exceeds the available context size (262144 tokens)"...}}
```

### 4.2 原因分析
1. llama-server 实际只支持 262144 (256K) tokens（模型 `n_ctx_train=262144` 硬限制）
2. Memoh 代码按 512K 配置，`maxTotalTokens = 250000`，填满了可用空间
3. 加上系统提示词后必然超限

### 4.3 修复

**文件**: `internal/conversation/flow/resolver.go`

```go
// 修改前
const maxTotalTokens = 250000 // ~250K tokens for messages

// 修改后
const maxTotalTokens = 150000 // safe cap for 256K context window
// Budget: system prompt ~30K + response ~30K + overhead ~10K = 70K reserved
// Max available for history: 262144 - 70K*4/3(chars→tokens) ≈ 150K tokens
```

**文件**: `internal/settings/types.go`

```go
// 修改前
DefaultDMHistoryLimit        = 20
DefaultChannelHistoryLimit   = 12
DefaultEvolutionHistoryLimit = 20

// 修改后
DefaultDMHistoryLimit        = 10
DefaultChannelHistoryLimit   = 6
DefaultEvolutionHistoryLimit = 10
```

---

## 五、场景矩阵（修复后）

| 场景 | Conversation.ID | route 唯一键 | 历史隔离 |
|------|----------------|--------------|---------|
| 用户 A 单聊 Bot | A 的 userid | `(botID, wecom, userA_id)` | ✅ 独立 |
| 用户 B 单聊 Bot | B 的 userid | `(botID, wecom, userB_id)` | ✅ 独立 |
| 用户 A 在群 G | 群 chatid (G) | `(botID, wecom, group_G_id)` | ✅ 独立 |
| 用户 A 在群 H | 群 chatid (H) | `(botID, wecom, group_H_id)` | ✅ 独立 |
| Heartbeat 任务 | botID (不存储) | N/A | ✅ 无状态 |

---

## 六、关键文件变更

| 文件 | 修改类型 | 说明 |
|------|---------|------|
| `internal/channel/adapters/wecom/adapter.go` | 修改 | 单聊 Conversation.ID 回落到 UserID；Metadata 补全 |
| `internal/channel/inbound/channel.go` | 修改 | 禁用 broadcastToOtherChannels；修复 debounce key |
| `internal/conversation/flow/resolver.go` | 修改 | 心跳禁用历史加载和存储；降低 maxTotalTokens |
| `internal/settings/types.go` | 修改 | 降低默认历史限制 |
| `internal/message/service.go` | 新增方法 | `DeleteByRoute()` |
| `internal/db/sqlc/messages.sql.go` | 新增方法 | `DeleteMessagesByRoute()` |
| `cmd/agent/main.go` | 修改 | 注入 routeService 到 WeCom adapter |

---

## 七、部署验证

### 7.1 构建部署
```bash
cd /data2/memoh-v2
docker compose build server
docker compose stop server && docker compose up -d server
```

### 7.2 验证单聊隔离
```sql
-- 检查路由表，确认单聊用户使用 userid 作为 external_conversation_id
SELECT bot_id, channel_type, external_conversation_id, conversation_type
FROM bot_channel_routes
WHERE channel_type='wecom' AND external_conversation_id != '';
```

### 7.3 验证心跳不加载历史
查看日志中 `history_loaded` 步骤：
```sql
SELECT step, message, data
FROM process_logs
WHERE step = 'history_loaded'
  AND data->>'task_type' = 'heartbeat';
-- 应该无记录（心跳跳过历史加载，不记录此步骤）
```

### 7.4 验证 /new 精确清除
1. 用户 A 发送消息，Bot 回复
2. 用户 A 发送 `/new`
3. 用户 A 再次发送消息，Bot 应该不知道之前的对话
4. 用户 B 继续对话，上下文应该保留

---

## 八、相关文档

- `/root/.claude/projects/-root/memory/wecom-isolation.md` - 持久化记忆
- `/data2/memoh-v2/docs/aibot-sdk/sdks/node/source/README.md` - WeCom SDK 规范

---

**记录时间**: 2026-03-19
**操作人员**: Claude Code
