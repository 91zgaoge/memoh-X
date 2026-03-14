# AI Bot SDK 规范摘要

> 自动生成于: $(date '+%Y-%m-%d %H:%M:%S')
> 文档来源: https://developer.work.weixin.qq.com/document/path/101463

## 官方 SDK 下载地址

| 语言 | 下载地址 | 说明 |
|------|----------|------|
| Node.js | https://www.npmjs.com/package/@wecom/aibot-node-sdk | npm 包 |
| Python | https://dldir1.qq.com/wework/wwopen/bot/aibot-python-sdk-1.0.0.zip | 官方 ZIP |

## 核心规范

### 1. WebSocket 连接

- **服务器地址**: `wss://openws.work.weixin.qq.com`
- **心跳间隔**: 30 秒（SDK 推荐）
- **认证方式**: bot_id + secret

### 2. 命令类型

| 命令 | 方向 | 说明 |
|------|------|------|
| `aibot_subscribe` | 开发者 → 企业微信 | 订阅/认证 |
| `ping` | 开发者 → 企业微信 | 心跳 |
| `pong` | 企业微信 → 开发者 | 心跳响应 |
| `aibot_msg_callback` | 企业微信 → 开发者 | 消息回调 |
| `aibot_event_callback` | 企业微信 → 开发者 | 事件回调 |
| `aibot_respond_msg` | 开发者 → 企业微信 | 被动回复消息 |
| `aibot_send_msg` | 开发者 → 企业微信 | 主动发送消息 |
| `aibot_respond_welcome_msg` | 开发者 → 企业微信 | 回复欢迎语 |
| `aibot_respond_update_msg` | 开发者 → 企业微信 | 更新模板卡片 |

### 3. 关键规范

#### req_id 使用规则

- **CmdRespondMsg**: 必须使用原始消息的 req_id
- **CmdSendMsg**: 必须生成新的 req_id

#### 流式消息 (Stream)

- **stream.id**: 首次回复设置，后续使用相同 id 刷新内容
- **超时**: 6 分钟内必须完成（finish=true）
- **频率限制**: 30条/分钟，1000条/小时

#### ACK 机制

- **ACK 超时**: 不代表消息发送失败，只是确认丢失
- **超时时间**: 5 秒

### 4. 消息类型

| 类型 | 说明 |
|------|------|
| text | 文本消息 |
| image | 图片消息（仅单聊） |
| voice | 语音消息（仅单聊，已转文本） |
| file | 文件消息（仅单聊） |
| mixed | 图文混排消息 |
| event | 事件消息 |

### 5. 文件下载与解密

- **算法**: AES-256-CBC
- **IV 来源**: aesKey 解码后前 16 字节
- **文件名获取**: 优先从 Content-Disposition 头提取（RFC5987）
