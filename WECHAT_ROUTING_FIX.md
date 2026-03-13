# 企业微信消息路由问题修复

## 问题描述
企业微信适配器中存在消息路由错误：A 私聊的回复被错误地发送给了 B。

## 修复内容

### 1. 添加详细的消息路由日志 (stream.go)
- 在 `NewOutboundStream` 中添加 `[MSG_ROUTE] OutboundStream created` 日志，记录目标用户ID、聊天ID、聊天类型等信息
- 在 `sendFullContent` 中添加 `[MSG_ROUTE]` 日志，记录发送命令类型、目标用户ID、req_id 等信息
- 在 `sendStreamContent` 中添加 `[MSG_ROUTE]` 日志，记录流式消息发送的目标信息
- 在 `sendStandaloneMessage` 中添加 `[MSG_ROUTE]` 日志，记录独立消息发送的目标信息
- 在 `sendSegmentedContent` 中添加 `[MSG_ROUTE]` 日志，记录分段消息发送的目标信息

### 2. 添加详细的消息路由日志 (adapter.go)
- 在 `OpenStream` 中添加 `[MSG_ROUTE] OpenStream creating outbound stream` 日志，记录从 metadata 提取的用户ID、聊天ID等信息
- 在 `Send` 中添加 `[MSG_ROUTE] Send (non-streaming)` 日志
- 在 `handleMessageCallback` 中增强日志，添加 `[MSG_ROUTE]` 标记和 reply_target 信息
- 在 handler 调用前后添加 `[MSG_ROUTE]` 日志，记录消息来源和回复目标

### 3. 日志标记
所有与消息路由相关的日志都添加了 `[MSG_ROUTE]` 标记，便于搜索和排查问题：
```bash
# 查看消息路由日志
docker logs memoh-server --tail 200 | grep "MSG_ROUTE"
```

## 如何使用这些日志排查问题

当再次出现消息路由错误时，通过以下步骤排查：

1. 找到错误消息的发送日志：
```bash
docker logs memoh-server --tail 500 | grep "MSG_ROUTE" | grep "target_user_id"
```

2. 对比发送者和接收者：
- 检查 `[MSG_ROUTE] message received` 中的 `from_user_id`
- 检查 `[MSG_ROUTE] OutboundStream created` 中的 `target_user_id`
- 如果两者不一致，说明消息路由有问题

3. 检查消息流：
```bash
# 追踪单个 req_id 的完整流程
docker logs memoh-server --tail 500 | grep "req_id=xxx"
```

## 可能的根本原因（待验证）

通过添加的日志，可以验证以下假设：

1. **req_id 混乱**：如果不同消息的 req_id 相同，可能导致消息队列混淆
2. **metadata 传递错误**：如果 user_id 或 chat_id 在传递过程中被覆盖
3. **并发问题**：如果多个消息同时处理，可能存在竞态条件

## 下一步建议

1. 部署后观察日志，确认消息路由是否正确
2. 如果问题再次出现，收集 `[MSG_ROUTE]` 日志进行分析
3. 根据日志分析结果，确定是数据问题还是代码逻辑问题
