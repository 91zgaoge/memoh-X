# AI Bot SDK 代码审核报告

**审核日期**: 2026-03-14
**Node.js SDK 版本**: 1.0.2
**Python SDK 版本**: 1.0.0
**审核人**: 自动审核脚本

## 概述

本报告基于企业微信 AI Bot SDK 最新版本 (v1.0.2) 对 Memoh 项目的企业微信适配器代码进行全面审核，找出差异并提出更新建议。

**官方文档最后更新**: 2026/03/13
**文档地址**: https://developer.work.weixin.qq.com/document/path/101463

---

## 1. 新增功能对比

### 1.1 媒体素材上传功能 (重要)

**SDK v1.0.2 新增功能**:
```typescript
// 三步分片上传临时素材
uploadMedia(fileBuffer: Buffer, options: UploadMediaOptions): Promise<UploadMediaFinishResult>

// 相关命令
UPLOAD_MEDIA_INIT: "aibot_upload_media_init"
UPLOAD_MEDIA_CHUNK: "aibot_upload_media_chunk"
UPLOAD_MEDIA_FINISH: "aibot_upload_media_finish"
```

**当前 Memoh 实现**: ❌ **未实现**

**影响**: 无法发送媒体消息（除 base64 图片外）

**建议**:
- 添加媒体素材上传功能
- 实现三步分片上传逻辑
- 支持 file/image/voice/video 类型

**实现优先级**: 🔴 **高**

---

### 1.2 媒体消息发送功能

**SDK v1.0.2 新增方法**:
```typescript
// 被动回复媒体消息
replyMedia(frame: WsFrameHeaders, mediaType: WeComMediaType, mediaId: string, videoOptions?: {...})

// 主动发送媒体消息
sendMediaMessage(chatid: string, mediaType: WeComMediaType, mediaId: string, videoOptions?: {...})

// 媒体类型
WeComMediaType = 'file' | 'image' | 'voice' | 'video'
```

**当前 Memoh 实现**: ❌ **未实现**

**当前仅支持**:
- ✅ 流式消息 (stream)
- ✅ Markdown 消息
- ✅ Base64 图片 (通过 msg_item)
- ❌ 文件消息 (file)
- ❌ 语音消息 (voice)
- ❌ 视频消息 (video)

**建议**:
- 实现 replyMedia 方法
- 实现 sendMediaMessage 方法
- 添加媒体类型常量

**实现优先级**: 🟡 **中**

---

### 1.3 模板卡片功能完善

**SDK 定义的完整模板卡片类型**:

| 卡片类型 | SDK 支持 | Memoh 支持 | 状态 |
|---------|---------|-----------|------|
| text_notice (文本通知) | ✅ | ✅ | 已实现 |
| news_notice (图文展示) | ✅ | ⚠️ | 部分实现 |
| button_interaction (按钮交互) | ✅ | ❌ | 未实现 |
| vote_interaction (投票选择) | ✅ | ❌ | 未实现 |
| multiple_interaction (多项选择) | ✅ | ❌ | 未实现 |

**SDK 模板卡片字段**:
```typescript
interface TemplateCard {
    card_type: string;
    source?: TemplateCardSource;
    action_menu?: TemplateCardActionMenu;  // ❌ 未实现
    main_title?: TemplateCardMainTitle;
    emphasis_content?: TemplateCardEmphasisContent;
    quote_area?: TemplateCardQuoteArea;
    sub_title_text?: string;
    horizontal_content_list?: TemplateCardHorizontalContent[];
    jump_list?: TemplateCardJumpAction[];
    card_action?: TemplateCardAction;
    card_image?: TemplateCardImage;        // ❌ 未实现
    image_text_area?: TemplateCardImageTextArea;  // ❌ 未实现
    vertical_content_list?: TemplateCardVerticalContent[];  // ❌ 未实现
    button_selection?: TemplateCardSelectionItem;  // ❌ 未实现
    button_list?: TemplateCardButton[];    // ❌ 未实现
    checkbox?: TemplateCardCheckbox;       // ❌ 未实现
    select_list?: TemplateCardSelectionItem[];  // ❌ 未实现
    submit_button?: TemplateCardSubmitButton;   // ❌ 未实现
    task_id?: string;                      // ⚠️ 部分实现
    feedback?: ReplyFeedback;              // ❌ 未实现
}
```

**建议**: 根据业务需求逐步完善模板卡片类型

**实现优先级**: 🟡 **中**

---

## 2. 命令类型对比

### 2.1 WebSocket 命令

| 命令 | SDK 定义 | Memoh 定义 | 状态 |
|------|---------|-----------|------|
| aibot_subscribe | ✅ | ✅ CmdSubscribe | ✅ |
| ping/pong | ✅ | ✅ CmdHeartbeat | ✅ |
| aibot_msg_callback | ✅ | ✅ CmdMsgCallback | ✅ |
| aibot_event_callback | ✅ | ✅ CmdEventCallback | ✅ |
| aibot_respond_msg | ✅ | ✅ CmdRespondMsg | ✅ |
| aibot_respond_welcome_msg | ✅ | ✅ CmdRespondWelcome | ✅ |
| aibot_respond_update_msg | ✅ | ✅ CmdRespondUpdate | ✅ |
| aibot_send_msg | ✅ | ✅ CmdSendMsg | ✅ |
| **aibot_upload_media_init** | ✅ | ❌ | **缺失** |
| **aibot_upload_media_chunk** | ✅ | ❌ | **缺失** |
| **aibot_upload_media_finish** | ✅ | ❌ | **缺失** |

**建议**: 添加缺失的上传素材命令常量

```go
const (
    CmdUploadMediaInit   = "aibot_upload_media_init"
    CmdUploadMediaChunk  = "aibot_upload_media_chunk"
    CmdUploadMediaFinish = "aibot_upload_media_finish"
)
```

---

## 3. 类型定义对比

### 3.1 MixedMsgItem (图文混排子项)

**SDK 定义**:
```typescript
interface MixedMsgItem {
    msgtype: 'text' | 'image';  // ⚠️ 仅支持 text/image
    text?: TextContent;
    image?: ImageContent;
}
```

**Memoh 当前**:
```go
type MixedMsgItem struct {
    MsgType string        `json:"msgtype"`  // ⚠️ 太宽松
    Text    *TextContent  `json:"text,omitempty"`
    Image   *ImageContent `json:"image,omitempty"`
    File    *FileContent  `json:"file,omitempty"`  // ❌ SDK 不支持
}
```

**问题**: Memoh 实现包含了 `File` 字段，但 SDK 不支持

**建议**: 可选：保持 File 字段作为扩展功能，但需注意官方可能不识别

---

### 3.2 FileContent (文件内容)

**SDK 定义**:
```typescript
interface FileContent {
    url: string;
    aeskey?: string;
    // ❌ 无 filename 字段
}
```

**Memoh 当前**:
```go
type FileContent struct {
    URL      string `json:"url"`
    AESKey   string `json:"aeskey,omitempty"`
    FileName string `json:"filename,omitempty"`  // ✅ 扩展字段
}
```

**问题**: Memoh 添加了 `filename` 字段作为扩展

**建议**: ✅ 合理扩展，保留此字段便于文件处理

---

### 3.3 QuoteContent (引用消息)

**SDK 定义**:
```typescript
interface QuoteContent {
    msgtype: 'text' | 'image' | 'mixed' | 'voice' | 'file';
    text?: TextContent;
    image?: ImageContent;
    mixed?: MixedContent;
    voice?: VoiceContent;
    file?: FileContent;
}
```

**Memoh 当前**: ✅ 已完整实现

---

## 4. 方法实现对比

### 4.1 流式消息

**SDK 方法签名**:
```typescript
replyStream(
    frame: WsFrameHeaders,
    streamId: string,
    content: string,
    finish?: boolean,
    msgItem?: ReplyMsgItem[],
    feedback?: ReplyFeedback
): Promise<WsFrame>
```

**Memoh 实现状态**: ✅ **已实现**

**差异点**:
- ✅ 支持 streamId
- ✅ 支持 finish 标志
- ✅ 支持 msg_item (图文混排)
- ❌ 不支持 feedback (反馈信息)

**建议**: 可选添加 feedback 参数支持

---

### 4.2 模板卡片消息

**SDK 方法**:
```typescript
replyTemplateCard(frame: WsFrameHeaders, templateCard: TemplateCard, feedback?: ReplyFeedback)
replyStreamWithCard(frame: WsFrameHeaders, streamId: string, content: string, finish?: boolean, options?: {...})
updateTemplateCard(frame: WsFrameHeaders, templateCard: TemplateCard, userids?: string[])
```

**Memoh 实现状态**: ⚠️ **部分实现**

**当前实现**:
- ✅ TemplateCard 基础结构
- ⚠️ replyWelcome 方法
- ❌ replyTemplateCard 方法
- ❌ replyStreamWithCard 方法
- ❌ updateTemplateCard 方法

**建议**: 根据业务需求实现完整的模板卡片功能

---

### 4.3 主动发送消息

**SDK 方法**:
```typescript
sendMessage(chatid: string, body: SendMsgBody): Promise<WsFrame>
```

**Memoh 实现**: ✅ **已实现** (通过 sendStandaloneMessage)

---

### 4.4 文件下载

**SDK 方法**:
```typescript
downloadFile(url: string, aesKey?: string): Promise<{ buffer: Buffer; filename?: string }>
```

**Memoh 实现**: ✅ **已实现** (通过 downloadAndDecrypt)

**差异**:
- ✅ Memoh 实现了从 Content-Disposition 头提取文件名
- ✅ Memoh 实现了 RFC5987 UTF-8 编码解码
- ✅ Memoh 支持 URL 路径提取备用方案

**评价**: Memoh 实现优于 SDK 基础功能

---

## 5. 配置参数对比

### 5.1 WebSocket 配置

| 参数 | SDK 默认值 | Memoh 当前值 | 建议 |
|------|-----------|-------------|------|
| 心跳间隔 | 30秒 | 10秒 | ✅ 更稳定 |
| 重连基础延迟 | 1秒 | 1秒 | ✅ 一致 |
| 最大重连次数 | 10 | 10 | ✅ 一致 |
| ACK 超时 | 5秒 | 5秒 | ✅ 一致 |

**评价**: Memoh 配置合理，心跳间隔更短有助于更快发现连接问题

---

## 6. 代码规范对比

### 6.1 req_id 使用规则

**SDK 规范**:
- `aibot_respond_msg`: 必须使用原始消息的 req_id
- `aibot_send_msg`: 必须生成新的 req_id

**Memoh 实现**:
```go
// ✅ 正确实现
if chatType == "group" && !isMentioned {
    cmd = CmdSendMsg
    sendReqID = generateReqID(CmdSendMsg)  // 生成新 req_id
} else {
    cmd = CmdRespondMsg
    sendReqID = reqID  // 使用原始 req_id
}
```

**评价**: ✅ **符合规范**

---

### 6.2 流式消息规则

**SDK 规范**:
- stream.id 首次设置后保持不变
- 6分钟内必须完成（finish=true）
- 单条消息最大 20480 字节

**Memoh 实现**: ✅ **符合规范**

```go
const (
    StreamTimeout   = 6 * time.Minute
    MaxContentBytes = 20480
)
```

---

### 6.3 ACK 机制

**SDK 规范**:
- 超时时间: 5秒
- ACK 超时不代表发送失败

**Memoh 实现**: ✅ **符合规范**

```go
const ReplyAckTimeout = 5 * time.Second

// 正确处理 ACK 超时
if pending != nil {
    pending.Resolve(WebsocketMessage{}) // 继续处理而不是失败
}
```

---

## 7. 更新建议汇总

### 🔴 高优先级

1. **添加媒体素材上传功能**
   - 文件: `internal/channel/adapters/wecom/types.go`, `websocket.go`
   - 新增命令常量: `CmdUploadMediaInit`, `CmdUploadMediaChunk`, `CmdUploadMediaFinish`
   - 新增类型: `UploadMediaInitBody`, `UploadMediaChunkBody`, `UploadMediaFinishBody`
   - 新增方法: `UploadMedia()`

2. **添加媒体类型常量**
   ```go
   const (
       MediaTypeFile  = "file"
       MediaTypeImage = "image"
       MediaTypeVoice = "voice"
       MediaTypeVideo = "video"
   )
   ```

### 🟡 中优先级

3. **完善模板卡片功能**
   - 添加缺失的模板卡片字段
   - 实现 `replyTemplateCard` 方法
   - 实现 `updateTemplateCard` 方法
   - 添加模板卡片事件完整处理

4. **添加媒体消息发送功能**
   - 实现 `replyMedia` 方法
   - 实现 `sendMediaMessage` 方法
   - 添加 `SendMediaMsgBody` 类型

5. **添加反馈信息支持**
   - 添加 `ReplyFeedback` 类型
   - 在流式消息中支持 feedback 参数

### 🟢 低优先级

6. **完善引用消息处理**
   - 在 `handleCallback` 中处理 Quote 字段
   - 将引用内容附加到消息文本

7. **完善事件处理**
   - `template_card_event`: 完整处理卡片点击事件
   - `feedback_event`: 处理用户反馈事件

---

## 8. 代码质量评价

### ✅ 优点

1. **规范遵循**: 严格遵循 SDK req_id 使用规则
2. **错误处理**: 完善的 ACK 超时和重连机制
3. **功能扩展**: 文件下载功能优于 SDK 基础实现
4. **长文本处理**: 实现了 SDK 未提供的分段发送功能
5. **频率控制**: 严格遵守 30条/分钟限制

### ⚠️ 改进空间

1. **功能覆盖**: 缺少媒体上传和发送功能
2. **模板卡片**: 类型定义不够完整
3. **单元测试**: 建议添加更多测试覆盖

---

## 9. 下一步行动

1. 根据业务需求确定功能优先级
2. 实现高优先级的媒体上传功能
3. 完善模板卡片支持
4. 添加单元测试
5. 持续关注 SDK 版本更新

---

## 10. 官方文档关键变化 (2026/03/13)

### 新增功能
- 媒体素材三步分片上传
- 媒体消息发送（file/image/voice/video）
- 完整的模板卡片类型支持

### 关键规范强调
1. **req_id 使用规则**:
   - `CmdRespondMsg`: 必须使用原始消息的 req_id
   - `CmdSendMsg`: 必须生成新的 req_id

2. **流式消息规则**:
   - stream.id 首次设置后保持不变
   - 6分钟内必须完成（finish=true）
   - 单条消息最大 20480 字节

3. **ACK 机制**:
   - 超时时间: 5秒
   - ACK 超时不代表消息发送失败

---

## 11. SDK 关键类型定义参考

### 11.1 新增命令常量

```typescript
export const WsCmd = {
    SUBSCRIBE: "aibot_subscribe",
    HEARTBEAT: "ping",
    RESPONSE: "aibot_respond_msg",
    RESPONSE_WELCOME: "aibot_respond_welcome_msg",
    RESPONSE_UPDATE: "aibot_respond_update_msg",
    SEND_MSG: "aibot_send_msg",
    UPLOAD_MEDIA_INIT: "aibot_upload_media_init",      // 新增
    UPLOAD_MEDIA_CHUNK: "aibot_upload_media_chunk",    // 新增
    UPLOAD_MEDIA_FINISH: "aibot_upload_media_finish",  // 新增
    CALLBACK: "aibot_msg_callback",
    EVENT_CALLBACK: "aibot_event_callback",
}
```

### 11.2 媒体消息类型

```typescript
export type WeComMediaType = 'file' | 'image' | 'voice' | 'video';

export interface SendMediaMsgBody {
    msgtype: WeComMediaType;
    file?: { media_id: string };
    image?: { media_id: string };
    voice?: { media_id: string };
    video?: {
        media_id: string;
        title?: string;
        description?: string;
    };
}
```

### 11.3 上传素材类型

```typescript
export interface UploadMediaInitBody {
    type: WeComMediaType;
    filename: string;
    total_size: number;
    total_chunks: number;
    md5?: string;
}

export interface UploadMediaChunkBody {
    upload_id: string;
    chunk_index: number;
    base64_data: string;
}

export interface UploadMediaFinishBody {
    upload_id: string;
}

export interface UploadMediaFinishResult {
    type: WeComMediaType;
    media_id: string;
    created_at: string;
}
```

---

*此报告由自动审核脚本生成*
*SDK 版本: @wecom/aibot-node-sdk@1.0.2*
