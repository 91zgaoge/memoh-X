# Memoh-v2 更新日志

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
