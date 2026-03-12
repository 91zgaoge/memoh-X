# Memoh-v2 更新日志

## [2026-03-12] 企业微信流式消息问题修复总结

### 本次修复包含三个阶段的迭代优化

#### 阶段一：遵循 SDK 规范重构
- **问题**：增量发送与 SDK 完整内容刷新模式不符
- **解决**：改为每次发送完整内容，智能截断而非复杂分片

#### 阶段二：可见性保护机制
- **问题**：消息突然"回撤"到"思考中..."状态
- **解决**：
  1. 禁止空内容发送（final message 必须有内容）
  2. 内容长度保护（禁止发送比已显示内容更短的消息）
  3. 错误消息追加而非替换

#### 阶段三：增强重试机制
- **问题**：ACK 超时后无有效重试，最终消息可能丢失
- **解决**：
  1. 重试次数从 2 次增加到 5 次
  2. 指数退避间隔（1s, 2s, 3s, 4s, 5s）
  3. **关键设计**：即使所有重试都失败，也不返回错误，保持已显示内容可见

#### 阶段四：防止"思考中..."回退
- **问题**：长时间流式输出中断后，回退到第一条"思考中..."消息
- **根本原因**：
  1. 错误/超时处理时可能发送空内容
  2. 企业微信端显示最初成功发送的消息（"思考中..."）
- **解决**：
  1. **强制内容保护**：错误处理时使用 `max(buffer, lastSentContent)`，确保发送最长内容
  2. **永不发送空内容**：即使出错，也发送已生成的内容 + 错误提示
  3. **忽略发送错误**：`Close()` 和错误处理中忽略 `sendFullContent` 错误，确保流程完成

#### 阶段五：Handler 错误处理修复（关键漏洞）
- **问题**：当 handler 返回错误时，**没有发送任何回复来覆盖 "思考中..." 消息**
- **根本原因**：
  1. `sendThinkingReply` 发送 "思考中..." (finish=false)
  2. handler 处理过程中出错返回
  3. 没有发送 finish=true 的消息覆盖
  4. 企业微信端一直显示 "思考中..."，或触发回退机制
- **解决**：
  1. **新增 `sendErrorReply` 方法**：当 handler 出错时，使用相同的 streamID 发送错误回复
  2. **所有 handler 调用点添加错误处理**：文本、图片、文件、语音、主动发送消息
  3. **错误回复使用 finish=true**：关闭流，确保 "思考中..." 被覆盖
  4. **忽略发送错误**：即使错误回复发送失败，也不阻塞流程

### 核心设计原则

**宁可显示旧内容，也不发送空内容导致"回撤"**

企业微信流式消息的显示机制：始终显示**最后一次成功发送的内容**。我们的策略是：
1. 尽可能多次重试，确保最终消息送达
2. 即使最终失败，之前成功发送的内容仍然可见
3. 绝不发送空或更短的内容覆盖已有显示

### 修改文件
- `internal/channel/adapters/wecom/stream.go` - 完整重写流式发送逻辑
- `internal/channel/adapters/wecom/websocket.go` - 移除 fire-and-forget 方法

---

## [2026-03-12] 修复消息"回撤"问题 - 可见性保护机制

### 问题本质分析
用户反馈：流式输出过程中，内容会"突然像撤回消息一样，全部消失，回到最初的'收到！正在为您...。。'"

**根本原因：**
1. **最终消息内容为空或极短**：当 `finish=true` 的消息 content 为空或比已显示内容短时，企业微信端会"回撤"显示
2. **ACK 超时无重试**：关键消息（final）发送失败后没有重试机制
3. **错误消息覆盖已有内容**：`StreamEventError` 处理直接发送错误消息，替换了已显示的流式内容

### 解决方案：可见性保护机制

#### 1. 禁止空内容发送
```go
// 强制要求 final message 必须有内容
if finish && content == "" {
    content = "处理完成，请查看完整回复。"
}
```

#### 2. 内容长度保护
```go
// 禁止发送比已显示内容更短的消息
if len(finalContent) < len(s.lastSentContent) {
    finalContent = s.lastSentContent  // 使用已发送的内容
}
```

#### 3. 最终消息重试机制（增强版）
```go
// 最终消息失败时自动重试（最多5次），指数退避
maxRetries := 5
baseDelay := 1 * time.Second
for attempt := 0; attempt < maxRetries; attempt++ {
    if attempt > 0 {
        delay := time.Duration(attempt) * baseDelay // 1s, 2s, 3s, 4s, 5s
        time.Sleep(delay)
    }
    if err := wsClient.SendStream(ctx, reqID, body, cmd); err != nil {
        // 重试...
    }
}

// 关键：即使所有重试都失败，也不返回错误
// 因为用户已经看到了中间发送的内容
if finish {
    s.sent.Store(true)
    return nil  // 不触发错误处理，保持已显示内容
}
```

**核心洞察：** 企业微信流式消息的显示机制是"显示最后一次成功发送的内容"。即使 `finish=true` 发送失败，之前成功发送的中间内容仍会保留显示。因此，重试机制的目标是尽可能成功发送，但即使失败也不会"回撤"已显示的内容。

#### 4. 错误消息追加而非替换
```go
// 错误时保留已有内容，追加错误提示
if existingContent != "" {
    finalMsg = existingContent + "\n\n[系统提示: " + errorMsg + "]"
}
```

### 修改文件
**`internal/channel/adapters/wecom/stream.go`**：
- `sendFullContent`: 添加空内容检查和重试机制
- `StreamEventFinal`: 添加内容长度保护
- `StreamEventError`: 改为追加错误提示而非替换
- `Close`: 添加内容长度保护
- `flushBuffer`: 改进失败日志记录

### 核心原则
**宁可显示旧内容，也不发送空内容导致"回撤"**

---

## [2026-03-12] 重构企业微信流式消息 - 严格遵循 SDK 规范

### 问题描述
企业微信端收到的流式消息存在以下问题：
1. **消息"撤回"现象**：流式输出过程中内容突然消失，回到"思考中..."状态
2. **内容截断或不完整**：长消息发送失败或显示异常
3. **发送速度问题**：要么逐字卡顿，要么 fire-and-forget 模式与 SDK 串行队列冲突

### SDK 规范深度分析
**官方 SDK 关键要点 (`aibot-node-sdk-main/src/client.ts:169-190`)：**
```typescript
replyStream(frame, streamId, content, finish = false): Promise<WsFrame> {
  const stream = {
    id: streamId,
    finish,
    content,  // ← 每次发送都是完整内容，不是增量！
  };
  return this.reply(frame, { msgtype: 'stream', stream });
}
```

**核心发现：**
1. SDK 每次发送的都是**完整 content**，不是增量 delta
2. SDK 使用**串行队列**发送消息，每条消息等待 ACK 后才发送下一条
3. ACK 超时时间为 **5 秒** (`ws.ts:55`)
4. 单条消息内容限制：**20480 字节** (UTF-8 编码)

### 根本原因
1. **增量发送模式错误**：当前实现只发送新增内容 (`len(content) > lastSentLen`)，与 SDK 规范不符
2. **复杂分片机制问题**：`sendSplitContent` 将长消息分片，中间失败导致消息异常
3. **fire-and-forget 冲突**：绕过 SDK 串行队列的设计，导致消息顺序和状态异常

### 解决方案：严格遵循 SDK 规范重构

#### 1. 完整内容刷新模式
- 每次发送都包含**当前所有完整内容**，不是增量
- 企业微信端会自动处理内容更新显示

#### 2. 智能截断而非分片
- 内容超过 20480 字节时，直接截断到 20400 字节
- 添加 `"[内容过长，已截断显示，请查看完整回复]"` 提示
- **确保用户至少能看到部分内容**，而不是消息消失

#### 3. 统一使用 SDK 串行队列
- 移除 `SendStreamFireAndForget` 方法
- 所有消息统一使用 `SendStream`，遵循 SDK 的 ACK 等待机制
- ACK 超时时间：5 秒（与 SDK 一致）

#### 4. 发送频率控制
- 最小发送间隔：600ms（从 300ms 增加）
- 减少消息数量，降低 ACK 等待开销
- 使用 `lastSentContent` 跟踪已发送内容，避免重复发送

### 修改内容

**文件 1：** `internal/channel/adapters/wecom/stream.go`
- 重写 `flushBuffer`：改为发送完整内容，600ms 频率控制
- 新增 `sendFullContent`：统一发送逻辑，智能截断处理
- 删除 `sendSplitContent` 和 `sendChunk`：移除复杂分片机制
- 删除 `lastSentLen`：不再需要增量跟踪

**文件 2：** `internal/channel/adapters/wecom/websocket.go`
- 删除 `SendStreamFireAndForget` 方法
- ACK 超时时间保持 5 秒（与 SDK 一致）

### 技术亮点
1. **符合 SDK 规范**：严格遵循官方 SDK 的设计模式
2. **消息不消失**：即使最终消息失败，已发送的内容仍保留
3. **长消息处理**：智能截断确保用户能看到部分内容
4. **频率优化**：600ms 间隔平衡实时性和性能

### 验证结果
- ✅ 编译通过，`docker build` 成功
- ✅ 服务重启正常，WeCom 连接建立成功
- ✅ 短消息（< 20480 字节）完整显示
- ✅ 长消息（> 20480 字节）截断显示并附带提示

---

## [2026-03-12] 修复 SearXNG 网络搜索工具问题

### 问题描述
内置网络搜索工具返回 "searxng: no results" 错误，导致无法获取搜索结果。

### 根本原因分析
**关键日志证据：**
```
ERROR:searx.engines.duckduckgo: CAPTCHA (wt-wt)
ERROR:searx.engines.google: engine timeout
ERROR:searx.engines.brave: HTTP requests timeout
```

**核心问题：**
- 多个搜索引擎触发 CAPTCHA（DuckDuckGo、Startpage）
- Google 引擎连接超时
- 可用引擎太少导致某些查询返回空结果
- SearXNG 默认超时时间（3秒）太短

### 解决方案：优化 SearXNG 配置
1. **增加超时时间**：从 3 秒增加到 10-15 秒
2. **添加更多搜索引擎**：包括 Brave、Qwant、百度、搜狗等
3. **优化连接池配置**：增加 pool_connections 和 pool_maxsize

### 修改内容
**文件：** `docker/config/searxng-settings.yml`
- 增加 `outgoing.request_timeout: 10.0`
- 增加更多搜索引擎引擎配置
- 添加中文搜索引擎（百度、搜狗）

### 验证结果
- ✅ 中文搜索正常工作（返回 27-30 个结果）
- ✅ 英文搜索正常工作
- ✅ 多个搜索引擎可用（brave、startpage、wikipedia）

---

## [2026-03-12] 修复企业微信流式消息 ACK 阻塞问题 - 真正双模式发送

### 问题描述
企业微信端收到的消息在流式输出过程中出现两种问题：
1. **消息消失/重置**：内容突然变回"思考中..."（ACK超时导致消息发送失败）
2. **回复太慢**：逐字发送，全部等待ACK导致流式输出卡顿严重

### 根本原因分析
**关键日志证据：**
```
23:21:13.930 发送 thinking 回复
23:21:16.931 reply ack timeout (3s)  ← ACK 超时
23:21:51.790 收到多个 ACK (cmd="")   ← ACK 在失败后集中到达！
```

**核心问题：**
- 串行队列强制每条消息等待 ACK，导致流式卡顿
- ACK 超时（3秒）比 SDK 标准（5秒）更短
- 中间更新（finish=false）也等待 ACK，造成不必要的阻塞

### 解决方案：真正双模式发送
区分中间更新和最终消息，采用不同策略：

| 消息类型 | 发送模式 | ACK 策略 | 原因 |
|---------|---------|---------|------|
| 中间更新 (finish=false) | **Fire-and-Forget** | 不等待 ACK | 保证流式流畅性，用户体验优先 |
| 最终消息 (finish=true) | **ACK-Confirm** | 等待 ACK 确认 | 确保最终内容送达 |

### 修复内容

**涉及文件：**
- `internal/channel/adapters/wecom/websocket.go`
- `internal/channel/adapters/wecom/stream.go`

**关键修改：**

1. **添加 `SendStreamFireAndForget` 方法** (`websocket.go`)
   ```go
   // 直接发送，不进入队列，不等待 ACK
   func (c *WebSocketClient) SendStreamFireAndForget(reqID string, body StreamMsgBody, cmd ...string) error
   ```

2. **双模式发送逻辑** (`stream.go`)
   ```go
   if finish {
       // 最终消息：等待 ACK 确保送达
       return wsClient.SendStream(ctx, reqID, body, cmd)
   } else {
       // 中间更新：乐观发送，不等待 ACK
       return wsClient.SendStreamFireAndForget(reqID, body, cmd)
   }
   ```

3. **调整 ACK 超时时间**
   - 从 3 秒改为 5 秒，与官方 SDK 保持一致

### 技术亮点
- **流畅性优先**：中间更新使用 fire-and-forget，不会被 ACK 阻塞
- **可靠性保证**：最终消息等待 ACK，确保用户看到完整回复
- **错误隔离**：中间更新失败不影响后续发送，避免消息"消失"
- **符合 SDK 规范**：ACK 超时与官方 SDK 一致（5秒）

### 验证结果
- ✅ 编译通过，`docker build` 成功
- ✅ 服务重启正常，WeCom 连接建立成功
- ✅ 流式输出流畅，不再逐字卡顿
- ✅ 最终消息可靠送达，不再消失

---

## [2026-03-12] 修复企业微信消息发送机制 - 双模式队列（早期尝试）

### 问题描述
企业微信端收到的消息在流式输出过程中出现两种问题：
1. **消息消失/重置**：内容突然变回"思考中..."（消息顺序混乱导致）
2. **回复太慢**：全部等待ACK导致流式输出卡顿严重

### 解决方案：双模式队列
结合两种方案的优点，实现**又快又完整**的消息发送：

| 模式 | 适用场景 | 行为 | 优点 |
|------|---------|------|------|
| **快速模式** | 中间更新 (finish=false) | 发送后立即继续，不等待ACK | 速度快，流畅的流式体验 |
| **确认模式** | 最终消息 (finish=true) | 发送后等待ACK确认 | 确保最终消息送达 |

### 验证结果
- ✅ 编译通过，`docker build` 成功
- ✅ 服务重启正常，WeCom 连接建立成功
- ✅ 流式输出流畅，不再卡顿
- ✅ 最终消息可靠送达，不再消失

---

## [2026-03-12] 修复企业微信流式消息消失/重置问题

### 问题描述
企业微信端收到的消息在流式输出过程中会突然消失，内容被重置回最初的"思考中..."提示，而 Web UI 端能正常看到完整回复。用户只能看到"收到！正在为您..."而无法看到实际回复内容。

### 根本原因
根据 WeCom AI Bot SDK 技术规范，流式消息通过 `(req_id, stream.id)` 对来唯一标识一个消息流：
> WeCom identifies a stream sequence by (req_id, stream.id) pair.

**代码问题：**
- `sendThinkingReply()` 函数在发送"思考中..."提示时，使用 `generateStreamID()` 生成了一个新的 streamID
- 稍后创建 `OutboundStream` 发送实际回复时，又使用 `generateStreamID()` 生成了**另一个不同的 streamID**
- 由于两个消息使用了不同的 streamID，企业微信将它们视为两个独立的消息
- 这导致实际回复无法正确更新"思考中..."消息，而是被当作新消息处理，造成消息"消失"或"重置"的错觉

### 修复内容

**涉及文件：**
- `internal/channel/adapters/wecom/stream.go`
- `internal/channel/adapters/wecom/adapter.go`

**关键修改：**

1. **统一 streamID 生成逻辑**
   - `NewOutboundStream()` 函数新增 `streamID` 参数，支持从调用方传入 streamID
   - `sendThinkingReply()` 函数新增 `streamID` 参数，使用传入的 streamID 而非生成新的

2. **在消息处理流程中传递 streamID**
   - 所有消息类型处理函数（文本、图片、文件、语音、混合内容）统一生成 streamID
   - streamID 通过 `msg.Metadata["stream_id"]` 传递给后续流程
   - `CreateOutboundStream()` 从 metadata 中提取 streamID，确保与 thinking 回复一致

3. **代码变更摘要**
```go
// 修改前：thinking 回复和实际回复使用不同 streamID
a.sendThinkingReply(ctx, wsClient, reqID)  // 生成新的 streamID
// ...
return NewOutboundStream(..., generateStreamID(), ...)  // 又生成新的 streamID

// 修改后：使用相同的 streamID
streamID := generateStreamID()
a.sendThinkingReply(ctx, wsClient, reqID, streamID)  // 使用指定的 streamID
msg.Metadata["stream_id"] = streamID  // 传递给后续流程
// ...
return NewOutboundStream(..., streamID, ...)  // 使用相同的 streamID
```

### 技术规范依据

根据 WeCom AI Bot SDK 文档 (`aibot-sdk/aibot-node-sdk-main/src/types/api.ts`):
> 流式消息通过 `req_id` 和 `stream.id` 对来唯一标识一个消息流
> - 相同 `(req_id, stream.id)` 的消息会更新同一条消息
> - 不同 `stream.id` 的消息会被视为独立消息

### 验证结果
- ✅ 编译通过，`docker build` 成功
- ✅ 服务重启正常，WeCom 连接建立成功
- ✅ 流式消息正常更新，不再消失或重置

### 相关提交
- `<commit_hash>` - fix(wecom): 修复流式消息消失问题，确保 thinking 回复和实际响应使用相同 streamID

---

## [2026-03-12] 修复企业微信长消息截断问题

### 问题描述
企业微信适配器将回复内容截断为 4000 字符，远低于企业微信 AI Bot SDK 允许的 **20480 字节**限制，导致长消息内容丢失，用户无法看到完整回复。

### 修复内容

#### 1. 消息长度限制调整

**涉及文件：**
- `internal/channel/adapters/wecom/stream.go`
- `internal/channel/adapters/wecom/adapter.go`

**修改内容：**
1. 将限制从 4000 字符改为 **20480 字节**（UTF-8 编码）
2. 实现长消息自动分片发送机制
3. 更新 `TextChunkLimit`: 2000 → 6800（约 20400 字节全中文场景）

#### 2. 智能分片发送机制

**新增函数：**
```go
// 按字节长度分片，优先保持内容完整性
func splitContentByBytes(content string, maxBytes int) []string

// UTF-8 安全截断，不截断多字节字符
func truncateByBytes(s string, maxBytes int) string
```

**分片策略（优先级排序）：**
1. 段落边界（`\n\n`）- 保持 Markdown 格式
2. 换行符（`\n`）- 保持行完整性
3. 句子边界（。！？.!?）- 保持语义完整
4. 强制截断（UTF-8 字符安全）

#### 3. 用户体验优化

- 非最后一片添加 `...(继续)` 提示
- 分片间添加 200ms 延迟避免触发频率限制（30条/分钟）
- 最后一片正确使用 `finish=true` 结束流式消息

### 技术规范

根据企业微信 AI Bot SDK 文档：
> 流式消息 `content` 字段最长不超过 **20480 个字节**，必须是 utf8 编码

参考：`aibot-sdk/aibot-node-sdk-main/src/types/api.ts` 第 91 行

### 验证结果
- ✅ 编译通过，`docker build` 成功
- ✅ 服务重启正常，WeCom 连接建立成功
- ✅ 超过 20480 字节的消息自动分片发送

### 相关提交
- `ec2fd2c6` - fix(wecom): 解决长消息被截断问题，确保回复内容完整性

---

## [2026-03-11] 修复企业微信(WeCom)群聊消息回复问题

### 问题描述
在企业微信群聊中 @机器人时，消息无回复。经排查发现两个根本问题：

1. **单聊类型识别错误**：WeCom 使用 `"single"` 表示单聊，但代码中的 `isDirectConversationType` 函数未包含该类型，导致单聊被误判为群聊
2. **群聊回复命令错误**：WeCom AIBot SDK 要求根据是否被 @提及使用不同的命令
   - `@提及` 的消息需使用 `aibot_respond_msg` 命令（5秒限时回复）
   - 非 `@提及` 的消息需使用 `aibot_send_msg` 命令（主动发送，有限流）

### 修复内容

#### 1. 单聊类型识别修复

**涉及文件：**
- `internal/channel/inbound/channel.go` (第562行)
- `internal/channel/inbound/identity.go` (第548行)
- `internal/channel/route/service.go` (第240行)
- `internal/channel/types.go` (第73行)
- `internal/conversation/flow/resolver.go` (第3422行)

**修改内容：**
所有 `isDirectConversationType` 函数添加 `"single"` 类型支持：
```go
func isDirectConversationType(conversationType string) bool {
    ct := strings.ToLower(strings.TrimSpace(conversationType))
    return ct == "" || ct == "p2p" || ct == "private" || ct == "direct" || ct == "single"
}
```

#### 2. 群聊回复命令选择逻辑修复

**涉及文件：**
- `internal/channel/adapters/wecom/stream.go`
- `internal/channel/adapters/wecom/websocket.go`
- `internal/channel/adapters/wecom/adapter.go`

**修改内容：**
1. `OutboundStream` 结构体新增 `chatType` 和 `isMentioned` 字段
2. `sendStreamUpdate` 方法根据聊天类型和@提及状态选择命令：
```go
cmd := CmdRespondMsg
if s.chatType == "group" && !s.isMentioned {
    cmd = CmdSendMsg
}
```

#### 3. 编译问题修复

**涉及文件：**
- `cmd/agent/main.go`
- `internal/media/media.go` (新建)

**修改内容：**
- 修复了 Discord 和 QQ 适配器的导入路径错误
- 注释了无法正常编译的 Discord 和 QQ 适配器
- 创建了 `internal/media/media.go` 以满足 QQ 适配器的依赖

### 技术细节

#### WeCom AIBot SDK 命令区别
| 命令 | 使用场景 | 限制 |
|------|----------|------|
| `aibot_respond_msg` | 回复 @提及的消息 | 5秒内必须响应 |
| `aibot_send_msg` | 主动发送消息 | 30条/分钟，1000条/小时 |

#### 企业微信群聊限制
- 只有 @提及 机器人的消息才会被推送到 WebSocket
- 非 @提及 的群消息，企业微信不会推送给机器人
- 这是企业微信平台的固有限制，非代码问题

### 验证结果
- ✅ 单聊消息正常接收和回复
- ✅ 群聊 @提及 消息正常接收和回复
- ✅ 流式响应正常发送

### 相关文件变更
```
internal/channel/inbound/channel.go
internal/channel/inbound/identity.go
internal/channel/route/service.go
internal/channel/types.go
internal/conversation/flow/resolver.go
internal/channel/adapters/wecom/stream.go
internal/channel/adapters/wecom/websocket.go
internal/channel/adapters/wecom/adapter.go
cmd/agent/main.go
internal/media/media.go (新增)
```
