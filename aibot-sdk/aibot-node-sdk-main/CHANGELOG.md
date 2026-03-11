# 更新日志

## 1.0.2 (2026-03-11)

### 更新内容

本次更新使 SDK 与企业微信官方文档（2026/03/10 更新版本）保持一致。

### 新增功能

#### 1. 添加 `disconnected_event` 事件类型
- 当有新连接建立时，系统会给旧连接发送该事件
- 每个机器人同时只能保持一个有效长连接，新连接会踢掉旧连接
- 新增 `EventType.DisconnectedEvent` 枚举值
- 新增 `DisconnectedEventData` 接口
- 新增 `event.disconnected_event` 事件监听

#### 2. 添加 `chat_type` 字段支持（主动推送消息）
- 新增 `ChatType` 枚举：
  - `ChatType.Auto = 0` - 兼容模式（默认，优先按群聊解析）
  - `ChatType.Single = 1` - 单聊（用户 userid）
  - `ChatType.Group = 2` - 群聊（chatid）
- 更新 `SendMarkdownMsgBody` 和 `SendTemplateCardMsgBody` 接口，添加可选的 `chat_type` 字段
- 建议开发者明确指定会话类型，避免混淆

#### 3. 添加流式消息限制说明
- 从流式消息发送开始，需在 **6 分钟内** 完成所有刷新并设置 `finish=true`
- 在 `replyStream` 和 `replyStreamWithCard` 方法文档中添加超时说明
- 官方文档标注 `msg_item` 字段暂不支持，已更新相关注释

#### 4. 添加主动推送消息限制说明
- **前提条件**：需要用户在会话中先给机器人发消息，后续机器人才能主动推送消息给对应会话
- **频率限制**：30条/分钟，1000条/小时（包含回复和主动推送）
- 在 `sendMessage` 方法文档中添加限制说明

#### 5. 更新消息类型限制说明
- 明确标注 `image`（图片消息）、`voice`（语音消息）、`file`（文件消息）**仅支持单聊**

### 文件变更

| 文件 | 变更内容 |
|------|---------|
| `src/types/event.ts` | 添加 `DisconnectedEvent` 事件类型和 `DisconnectedEventData` 接口 |
| `src/types/api.ts` | 添加 `ChatType` 枚举，更新 `SendMarkdownMsgBody` 和 `SendTemplateCardMsgBody` 接口 |
| `src/types/index.ts` | 导出新增的类型 |
| `src/index.ts` | 导出 `ChatType` 和 `DisconnectedEventData` |
| `src/client.ts` | 更新方法文档注释，添加限制说明 |
| `README.md` | 更新文档，添加新功能和限制说明 |
| `examples/basic.ts` | 添加新功能使用示例 |

### 升级指南

#### 1. 监听连接断开事件
```typescript
wsClient.on('event.disconnected_event', (frame) => {
  console.log('检测到新连接建立，当前连接将被关闭');
});
```

#### 2. 使用 chat_type 明确指定会话类型
```typescript
import { ChatType } from '@wecom/aibot-node-sdk';

// 发送消息到群聊
await wsClient.sendMessage('chatid', {
  msgtype: 'markdown',
  markdown: { content: '群消息' },
  chat_type: ChatType.Group,
});

// 发送消息到单聊
await wsClient.sendMessage('userid', {
  msgtype: 'markdown',
  markdown: { content: '单聊消息' },
  chat_type: ChatType.Single,
});
```

#### 3. 注意流式消息 6 分钟超时限制
```typescript
const startTime = Date.now();
const SIX_MINUTES = 6 * 60 * 1000;

// 发送流式消息...
if (Date.now() - startTime > SIX_MINUTES) {
  console.warn('流式消息已超时，无法再更新');
  return;
}
```
