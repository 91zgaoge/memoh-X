# AI Bot SDK 代码合规性检查报告

**检查日期**: 2026-03-26 03:06:37
**检查目录**: /data2/memoh-v2/internal/channel/adapters/wecom

## 1. 类型定义检查

### 1.1 WebSocket 命令常量

| 命令 | SDK 标准 | 代码实现 | 状态 |
|------|----------|----------|------|
| aibot_subscribe | ✅ | ✅ 已实现 |
| aibot_respond_msg | ✅ | ✅ 已实现 |
| aibot_send_msg | ✅ | ✅ 已实现 |
| ping/pong | ✅ | ✅ 已实现 |

### 1.2 消息类型常量

| 类型 | SDK 标准 | 代码实现 | 状态 |
|------|----------|----------|------|
| text | ✅ | ✅ |
| markdown | ✅ | ✅ |
| image | ✅ | ✅ |
| file | ✅ | ✅ |
| stream | ✅ | ✅ |

## 2. 关键实现检查

### 2.1 req_id 使用规范

- **CmdRespondMsg 使用原始 req_id**: ❓ 需检查
- **CmdSendMsg 生成新 req_id**: ✅

### 2.2 流式消息实现

- **Stream ID 生成**: ✅
- **6分钟超时检查**: ✅
- **Finish 标志设置**: ✅

### 2.3 ACK 处理

- **ReplyAckTimeout 设置**: ❌
- **ACK 超时处理**: ✅

### 2.4 文件下载解密

- **AES-256-CBC 解密**: ❌
- **Content-Disposition 文件名解析**: ✅

## 3. 频率限制实现

- **30条/分钟限制**: ✅
- **消息间隔控制**: ✅

## 4. 长文本分段发送

- **MaxContentBytes 定义**: ✅
- **分段发送函数**: ✅

## 5. 待确认项目


### 需要人工审查的代码位置

**req_id 使用位置**:
- 30:	reqID           string
- 101:func NewOutboundStream(adapter *Adapter, cfg channel.ChannelConfig, wsClient *WebSocketClient, reqID, chatID, userID, chatType string, isMentioned bool, streamID string, logger *slog.Logger, receivedAt time.Time, cmd ...string) *OutboundStream {
- 117:		reqID:                reqID,
- 124:		logger:               logger.With(slog.String("component", "wecom_stream"), slog.String("req_id", reqID), slog.String("user_id", userID), slog.String("chat_id", chatID), slog.String("chat_type", chatType)),
- 134:		slog.String("req_id", reqID),
- 498:		Headers: MessageHeaders{ReqID: s.reqID},
- 587:	reqID := s.reqID
- 610:			slog.String("req_id", reqID))
- 638:	// CRITICAL: CmdSendMsg (proactive send) must use a NEW req_id, not the original message's req_id
- 639:	// CmdRespondMsg must use the original req_id from the triggering message
- 641:	sendReqID := reqID // Default: use original req_id for respond
- 644:		// Generate a new req_id for proactive send - this is critical for SDK compliance
- 645:		// WeCom SDK requires new req_id for proactive messages (aibot_send_msg)
- 646:		// Using original req_id will cause ACK timeout because WeCom won't recognize it
- 652:			slog.String("original_req_id", reqID),
- 653:			slog.String("send_req_id", sendReqID))
- 660:			slog.String("send_req_id", sendReqID))
- 723:				slog.String("send_req_id", sendReqID),
- 742:				slog.String("send_req_id", sendReqID),
- 1014:	// 后续段：使用独立消息（CmdSendMsg，新的 req_id）

**sendReqID 生成位置**:
- 641:	sendReqID := reqID // Default: use original req_id for respond
- 647:		sendReqID = generateReqID(CmdSendMsg)
- 653:			slog.String("send_req_id", sendReqID))
- 660:			slog.String("send_req_id", sendReqID))
- 688:		if err := wsClient.SendStream(ctx, sendReqID, body, cmd); err != nil {
- 723:				slog.String("send_req_id", sendReqID),
- 742:				slog.String("send_req_id", sendReqID),
- 1105:	sendReqID := reqID
- 1108:		sendReqID = generateReqID(CmdSendMsg)
- 1110:			slog.String("send_req_id", sendReqID),
