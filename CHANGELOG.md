# Memoh-v2 更新日志

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
