# WeCom 群聊文件发送修复（2026-03-21）

## 问题描述

在群聊中@bot使用 agent-fetch 技能生成报告后：
1. 文字能正常生成和发送（3831字节）
2. 但 MD 文件没有发送到群聊
3. 需要在发送文件时@提及请求者

## 根本原因分析

### 问题1：文件未发送
当 `channel.go` 调用 `Push(StreamEventFinal{text})` 发送文字后，又调用 `sendSharedFileAttachments` 推送 `StreamEventFinal{attachments}`，但此时：

1. 第一次 `Push` 调用 `sendFullContent(finish=true)` 成功，设置 `s.sent = true`
2. 第二次 `Push` 调用 `sendFullContent(finish=true)` 尝试再次 finalize 同一个流
3. WeCom 拒绝了第二次 finalize（流已结束）
4. 虽然文件被上传到 `s.pendingMediaMsgs`，但由于流发送失败，`sendPendingMedia` 没有被调用

### 问题2：Context 取消导致事件丢失
`resolver.go` 中的 `chunkCh <- chunk` 是阻塞发送，如果 context 在此时被取消，attachment_delta 事件会被丢弃。

## 修复方案

### 1. 检测已发送的流并直接处理附件

**文件**: `internal/channel/adapters/wecom/stream.go`

在 `Push(StreamEventFinal)` 中添加检测逻辑：

```go
// 捕获附件后，检测流是否已发送
if s.sent.Load() {
    hasNewText := event.Final != nil && event.Final.Message.Text != "" &&
                  event.Final.Message.Text != s.lastSentContent
    if !hasNewText && len(s.attachments) > 0 {
        // 流已发送且只有附件，直接处理
        s.uploadAttachments(ctx, s.attachments)
        s.sendPendingMediaWithMention(ctx, "")
        return nil
    }
}
```

### 2. 新增带@提及的文件发送函数

**文件**: `internal/channel/adapters/wecom/stream.go`

新增 `sendPendingMediaWithMention` 函数：

```go
// sendPendingMediaWithMention sends pending media messages with optional @mention for group chats.
// When the bot is responding to an @mention in a group chat, it will @mention the requesting user
// when sending the file.
func (s *OutboundStream) sendPendingMediaWithMention(ctx context.Context, extraText string) {
    // ... 发送文本 fallback ...

    // For group chats triggered by @mention, send a @mention message first
    if s.chatType == "group" && s.isMentioned && s.userID != "" {
        mentionText := fmt.Sprintf("<%@s> 已为您生成文件", s.userID)
        // 发送@提及消息
    }

    // 发送所有待处理的媒体文件
}
```

WeCom @提及格式：`<@userid>`

### 3. 修复 Context 取消问题

**文件**: `internal/conversation/flow/resolver.go`

将阻塞发送改为带 context 检查的发送：

```go
select {
case chunkCh <- conversation.StreamChunk([]byte(data)):
case <-ctx.Done():
    return ctx.Err()
}
```

### 4. 添加诊断日志

**文件**: `internal/channel/inbound/channel.go`

添加了以下日志：
- `appendAttachmentDeltaPaths: collected new paths` - 收集到新的附件路径
- `sendSharedFileAttachments: sending files` - 开始发送文件
- `sendSharedFileAttachments: no files to send` - 没有文件需要发送

## 修改文件列表

| 文件 | 修改内容 |
|------|---------|
| `internal/channel/adapters/wecom/stream.go` | 1. 在 `Push(StreamEventFinal)` 中添加检测逻辑<br>2. 新增 `sendPendingMediaWithMention` 函数支持@提及 |
| `internal/channel/inbound/channel.go` | 添加诊断日志 |
| `internal/conversation/flow/resolver.go` | 修复 context 取消时的阻塞发送问题 |

## 测试方法

1. 在群聊中@bot，请求生成文件（如新闻汇总报告）
2. 验证：
   - 文字回复正常发送
   - MD 文件成功发送到群聊
   - 文件发送时带有 @提及请求者

## 相关提交

- 修复 WeCom 群聊文件发送问题
- 添加群聊文件发送时的@提及功能
