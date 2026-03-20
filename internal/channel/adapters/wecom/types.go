package wecom

import (
	"encoding/json"
	"time"
)

// ========== 常量定义 ==========

// Channel type constant
const (
	Type           = "wecom"
	WebhookPath    = "/webhook/wecom"
	DefaultTimeout = 30 * time.Second
)

// WebSocket命令类型常量
const (
	// 开发者 → 企业微信
	CmdSubscribe      = "aibot_subscribe"
	CmdHeartbeat      = "ping"
	CmdRespondMsg     = "aibot_respond_msg"
	CmdRespondWelcome = "aibot_respond_welcome_msg"
	CmdRespondUpdate  = "aibot_respond_update_msg"
	CmdSendMsg        = "aibot_send_msg"

	// 企业微信 → 开发者
	CmdMsgCallback   = "aibot_msg_callback"
	CmdEventCallback = "aibot_event_callback"
	CmdPong          = "pong"

	// 媒体上传命令
	CmdUploadMediaInit   = "aibot_upload_media_init"
	CmdUploadMediaChunk  = "aibot_upload_media_chunk"
	CmdUploadMediaFinish = "aibot_upload_media_finish"
)

// 消息类型常量
const (
	MsgTypeText     = "text"
	MsgTypeMarkdown = "markdown"
	MsgTypeImage    = "image"
	MsgTypeFile     = "file"
	MsgTypeVoice    = "voice"
	MsgTypeMixed    = "mixed"
	MsgTypeEvent    = "event"
	MsgTypeStream   = "stream"

	// 媒体文件类型常量（用于上传）
	MediaTypeFile  = "file"
	MediaTypeImage = "image"
	MediaTypeVoice = "voice"
	MediaTypeVideo = "video"
)

// 事件类型常量
const (
	EventTypeEnterChat    = "enter_chat"
	EventTypeDisconnected = "disconnected_event"
	EventTypeTemplateCard = "template_card_event"
	EventTypeFeedback     = "feedback_event"
)

// ========== WebSocket帧结构 ==========

// WebsocketMessage 是WebSocket通信的基础消息格式
type WebsocketMessage struct {
	Cmd     string          `json:"cmd,omitempty"`
	Headers MessageHeaders  `json:"headers"`
	Body    json.RawMessage `json:"body,omitempty"`
	ErrCode int             `json:"errcode,omitempty"`
	ErrMsg  string          `json:"errmsg,omitempty"`
}

// MessageHeaders 包含消息元数据
type MessageHeaders struct {
	ReqID string `json:"req_id"`
}

// ResponseBody 是一般响应体
type ResponseBody struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// ========== 订阅和认证 ==========

// SubscribeBody 是订阅请求体
type SubscribeBody struct {
	BotID  string `json:"bot_id"`
	Secret string `json:"secret"`
}

// ========== 消息回调体 ==========

// MsgCallbackBody 是消息回调体
type MsgCallbackBody struct {
	MsgID    string          `json:"msgid"`
	AIBotID  string          `json:"aibotid"`
	ChatID   string          `json:"chatid"`
	ChatType string          `json:"chattype"`
	From     FromInfo        `json:"from"`
	MsgType  string          `json:"msgtype"`
	Text     *TextContent    `json:"text,omitempty"`
	Image    *ImageContent   `json:"image,omitempty"`
	File     *FileContent    `json:"file,omitempty"`
	Voice    *VoiceContent   `json:"voice,omitempty"`
	Mixed    *MixedContent   `json:"mixed,omitempty"`
	Event    *EventContent   `json:"event,omitempty"`
	Quote    *QuoteContent   `json:"quote,omitempty"`
	ResponseURL string       `json:"response_url,omitempty"`
}

// FromInfo 表示发送者信息
type FromInfo struct {
	UserID string `json:"userid"`
	CorpID string `json:"corpid,omitempty"`
	Name   string `json:"name,omitempty"`
}

// ========== 消息内容结构 ==========

// TextContent 表示文本消息内容
type TextContent struct {
	Content string `json:"content"`
}

// MarkdownContent 表示Markdown消息内容
type MarkdownContent struct {
	Content string `json:"content"`
}

// ImageContent 表示图片消息内容（带解密密钥）
type ImageContent struct {
	URL    string `json:"url"`
	AESKey string `json:"aeskey,omitempty"`
}

// FileContent 表示文件消息内容（带解密密钥）
type FileContent struct {
	URL      string `json:"url"`
	AESKey   string `json:"aeskey,omitempty"`
	FileName string `json:"filename,omitempty"`
}

// VoiceContent 表示语音消息内容（已转文本）
type VoiceContent struct {
	Content string `json:"content"`
}

// MixedContent 表示混合（图文）消息内容
type MixedContent struct {
	MsgItem []MixedMsgItem `json:"msg_item,omitempty"`
}

// MixedMsgItem 表示混合内容中的单项
type MixedMsgItem struct {
	MsgType string        `json:"msgtype"`
	Text    *TextContent  `json:"text,omitempty"`
	Image   *ImageContent `json:"image,omitempty"`
	File    *FileContent  `json:"file,omitempty"`
}

// QuoteContent 表示引用消息内容
type QuoteContent struct {
	MsgType  string        `json:"msgtype"`
	Text     *TextContent  `json:"text,omitempty"`
	Image    *ImageContent `json:"image,omitempty"`
	Mixed    *MixedContent `json:"mixed,omitempty"`
	Voice    *VoiceContent `json:"voice,omitempty"`
	File     *FileContent  `json:"file,omitempty"`
}

// EventContent 表示事件回调内容
type EventContent struct {
	EventType string `json:"eventtype"`
	EventKey  string `json:"event_key,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
}

// DisconnectedEventData 表示连接断开事件数据
// 当有新连接建立时，系统会给旧连接发送该事件
type DisconnectedEventData struct {
	EventType string `json:"eventtype"`
}

// ========== 回复消息体 ==========

// RespondMsgBody 用于发送回复（非流式）
type RespondMsgBody struct {
	MsgType  string           `json:"msgtype"`
	Text     *TextContent     `json:"text,omitempty"`
	Markdown *MarkdownContent `json:"markdown,omitempty"`
}

// StreamMsgBody 用于流式回复
type StreamMsgBody struct {
	MsgType string          `json:"msgtype"`
	Stream  StreamResponse  `json:"stream"`
}

// StreamResponse 表示流式响应
type StreamResponse struct {
	ID       string `json:"id"`
	Finish   bool   `json:"finish"`
	Content  string `json:"content,omitempty"`
	MsgItem  []ReplyMsgItem `json:"msg_item,omitempty"`
	Feedback *ReplyFeedback `json:"feedback,omitempty"`
}

// ReplyMsgItem 表示回复中的图文混排项
type ReplyMsgItem struct {
	MsgType string `json:"msgtype"`
	Image   struct {
		Base64 string `json:"base64"`
		MD5    string `json:"md5"`
	} `json:"image,omitempty"`
}

// ReplyFeedback 表示回复中的反馈信息
type ReplyFeedback struct {
	ID string `json:"id"`
}

// ========== 模板卡片 ==========

// TemplateCardReplyBody 表示模板卡片回复体
type TemplateCardReplyBody struct {
	MsgType      string       `json:"msgtype"`
	TemplateCard TemplateCard `json:"template_card"`
}

// TemplateCard 表示模板卡片
type TemplateCard struct {
	CardType                string                        `json:"card_type"`
	Source                  *TemplateCardSource           `json:"source,omitempty"`
	MainTitle               *TemplateCardMainTitle        `json:"main_title,omitempty"`
	EmphasisContent         *TemplateCardEmphasisContent  `json:"emphasis_content,omitempty"`
	QuoteArea               *TemplateCardQuoteArea        `json:"quote_area,omitempty"`
	SubTitleText            string                        `json:"sub_title_text,omitempty"`
	HorizontalContentList   []TemplateCardHorizontalItem  `json:"horizontal_content_list,omitempty"`
	JumpList                []TemplateCardJumpItem        `json:"jump_list,omitempty"`
	CardAction              *TemplateCardAction           `json:"card_action,omitempty"`
	TaskID                  string                        `json:"task_id,omitempty"`
	Feedback                *ReplyFeedback                `json:"feedback,omitempty"`
}

// TemplateCardSource 表示卡片来源
type TemplateCardSource struct {
	Desc      string `json:"desc,omitempty"`
	DescColor int    `json:"desc_color,omitempty"`
	IconURL   string `json:"icon_url,omitempty"`
}

// TemplateCardMainTitle 表示卡片主标题
type TemplateCardMainTitle struct {
	Title string `json:"title,omitempty"`
	Desc  string `json:"desc,omitempty"`
}

// TemplateCardEmphasisContent 表示关键数据样式
type TemplateCardEmphasisContent struct {
	Title string `json:"title,omitempty"`
	Desc  string `json:"desc,omitempty"`
}

// TemplateCardQuoteArea 表示引用区域
type TemplateCardQuoteArea struct {
	Type       int    `json:"type,omitempty"`
	URL        string `json:"url,omitempty"`
	Title      string `json:"title,omitempty"`
	QuoteText  string `json:"quote_text,omitempty"`
}

// TemplateCardHorizontalItem 表示二级标题+文本
type TemplateCardHorizontalItem struct {
	Type    int    `json:"type,omitempty"`
	KeyName string `json:"keyname"`
	Value   string `json:"value,omitempty"`
	URL     string `json:"url,omitempty"`
}

// TemplateCardJumpItem 表示跳转指引
type TemplateCardJumpItem struct {
	Type  int    `json:"type,omitempty"`
	Title string `json:"title"`
	URL   string `json:"url,omitempty"`
}

// TemplateCardAction 表示卡片点击事件
type TemplateCardAction struct {
	Type int    `json:"type"`
	URL  string `json:"url,omitempty"`
}

// ========== 主动发送消息体 ==========

// ChatType 会话类型，用于主动发送消息时指定会话类型
const (
	ChatTypeAuto   = 0 // 兼容单聊/群聊类型，优先按照群聊会话类型去发送消息（默认）
	ChatTypeSingle = 1 // 单聊（用户 userid）
	ChatTypeGroup  = 2 // 群聊
)

// SendMarkdownMsgBody 主动发送 Markdown 消息体
type SendMarkdownMsgBody struct {
	MsgType  string          `json:"msgtype"`
	Markdown MarkdownContent `json:"markdown"`
	// ChatID 会话 ID，用于指定消息发送目标
	// 单聊时为用户 userid，群聊时为群 chatid
	ChatID string `json:"chatid,omitempty"`
	// ChatType 会话类型，用于指定 chatid 的解析方式
	// 1：单聊（用户 userid）；2：群聊；0 或不填：兼容单聊/群聊类型，优先按照群聊会话类型去发送消息
	// 建议开发者设置具体的单聊或者群聊来使用
	ChatType int `json:"chat_type,omitempty"`
}

// SendTemplateCardMsgBody 主动发送模板卡片消息体
type SendTemplateCardMsgBody struct {
	MsgType      string       `json:"msgtype"`
	TemplateCard TemplateCard `json:"template_card"`
	// ChatType 会话类型，用于指定 chatid 的解析方式
	// 1：单聊（用户 userid）；2：群聊；0 或不填：兼容单聊/群聊类型，优先按照群聊会话类型去发送消息
	// 建议开发者设置具体的单聊或者群聊来使用
	ChatType int `json:"chat_type,omitempty"`
}

// ========== 媒体上传消息体 ==========

// UploadMediaInitBody 媒体上传初始化请求体
type UploadMediaInitBody struct {
	Filename    string `json:"filename"`
	FileSize    int    `json:"total_size"`
	MediaType   string `json:"type"`          // file/image/voice/video
	ChunkNum    int    `json:"total_chunks"`
	MD5         string `json:"md5,omitempty"`
}

// UploadMediaInitResult 媒体上传初始化响应体
type UploadMediaInitResult struct {
	ErrCode   int    `json:"errcode"`
	ErrMsg    string `json:"errmsg,omitempty"`
	UploadID  string `json:"upload_id"`
	ChunkSize int    `json:"chunk_size,omitempty"` // 服务器指定的分片大小
}

// UploadMediaChunkBody 媒体分片上传请求体
type UploadMediaChunkBody struct {
	UploadID   string `json:"upload_id"`
	ChunkIndex int    `json:"chunk_index"` // 0-based (官方文档确认从0开始)
	ChunkData  string `json:"base64_data"` // base64-encoded
}

// UploadMediaFinishBody 媒体上传完成请求体
type UploadMediaFinishBody struct {
	UploadID string `json:"upload_id"`
	MD5      string `json:"md5,omitempty"` // 完整文件 MD5
}

// UploadMediaFinishResult 媒体上传完成响应体
type UploadMediaFinishResult struct {
	ErrCode   int    `json:"errcode"`
	ErrMsg    string `json:"errmsg,omitempty"`
	MediaID   string `json:"media_id"`
	MediaType string `json:"type,omitempty"`      // 媒体类型
	CreatedAt int64  `json:"created_at,omitempty"` // 创建时间（Unix 时间戳）
}

// MediaIDRef 媒体 ID 引用
type MediaIDRef struct {
	MediaID string `json:"media_id"`
}

// SendMediaMsgBody 发送媒体消息体（使用 media_id）
type SendMediaMsgBody struct {
	MsgType  string      `json:"msgtype"` // "image"/"file"/"voice"/"video"
	Image    *MediaIDRef `json:"image,omitempty"`
	File     *MediaIDRef `json:"file,omitempty"`
	Voice    *MediaIDRef `json:"voice,omitempty"`
	Video    *MediaIDRef `json:"video,omitempty"`
	ChatID   string      `json:"chatid,omitempty"`
	ChatType int         `json:"chat_type,omitempty"`
}

// ========== 辅助结构 ==========

// TargetInfo 表示回复目标信息
type TargetInfo struct {
	ChatID   string
	UserID   string
	ChatType string
	ReqID    string
}

// IsSingle 返回是否为单聊
func (t *TargetInfo) IsSingle() bool {
	return t.ChatType == "single" || t.ChatType == ""
}

// GetTargetID 返回适当的回复ID
func (t *TargetInfo) GetTargetID() string {
	if t.IsSingle() {
		return t.UserID
	}
	return t.ChatID
}

// ReplyQueueItem 表示回复队列中的单个任务
type ReplyQueueItem struct {
	Frame   WebsocketMessage
	Resolve func(frame WebsocketMessage)
	Reject  func(reason error)
}
