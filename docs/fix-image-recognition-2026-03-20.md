# 修复图片识别失败问题 - 2026-03-20

## 问题现象

WeCom 对话中发送图片后，模型无法识别图片内容，始终将图片当作文本处理。

---

## 根本原因分析

### Bug 1（致命）：OpenAI 格式模型收到 JSON 字符串而非图片内容

**文件**: `agent/src/agent.ts` — `stream()` 函数（第 ~890 行）

WeCom 渠道使用 `stream()` 直接 fetch 路径（非 Vercel AI SDK 的 `ask()` 路径）。该路径对 OpenAI 兼容模型（Qwen、llama 等所有非 kimi-coding 模型）在序列化消息内容时使用了：

```ts
content: typeof m.content === 'string' ? m.content : JSON.stringify(m.content),
```

导致图片内容数组被序列化为 JSON 字符串字面量：

```
"[{\"type\":\"image\",\"image\":\"<base64>\",...}]"
```

模型收到的是一段 JSON 文本，而非 OpenAI 规范要求的 `image_url` 内容块。

### Bug 2（次要）：MIME 类型丢失，始终硬编码为 image/jpeg

图片经 `compressImageIfNeeded()` 压缩后，正确的 MIME 类型（如 `image/png`、`image/webp`）在构建 `InputAttachment` 时被丢弃，导致：
- 未超过 500KB 阈值的 PNG/WebP 图片不经压缩直接传递，但 MIME 类型仍错误地被标记为 `image/jpeg`
- Kimi Code（Anthropic 格式）path 的 `media_type` 字段始终硬编码为 `image/jpeg`

---

## 修复方案

### Fix 1：为 OpenAI 路径添加正确的图片序列化

在 `stream()` 函数中添加 `serializeContentForOpenAI()` 函数，将 ImagePart 转换为 OpenAI 规范格式：

```ts
const serializeContentForOpenAI = (content: unknown): unknown => {
  if (typeof content === 'string') return content
  if (!Array.isArray(content)) return String(content)
  return content.map((part: any) => {
    if (part.type === 'image') {
      const mimeType = part.mediaType || 'image/jpeg'
      return {
        type: 'image_url',
        image_url: { url: `data:${mimeType};base64,${part.image}` },
      }
    }
    return part
  })
}
```

替换原来的 `JSON.stringify(m.content)` 为 `serializeContentForOpenAI(m.content)`。

### Fix 2：MIME 类型全链路传递

| 层级 | 文件 | 修改 |
|------|------|------|
| Go 类型 | `internal/conversation/types.go` | `InputAttachment` 增加 `MimeType string` 字段 |
| Go 压缩层 | `internal/channel/inbound/channel.go` | `buildInputAttachments()` 传递 `MimeType: mimeType` |
| Go 网关层 | `internal/conversation/flow/resolver.go` | `buildGatewayAttachments()` 输出 `mime_type` 键 |
| TS 类型 | `agent/src/types/attachment.ts` | `ImageAttachment` 增加 `mimeType?: string` |
| TS 校验 | `agent/src/models.ts` | `ImageAttachmentModel` 增加 `mimeType: z.string().optional()` |
| TS 序列化 | `agent/src/agent.ts` | `generateUserPrompt()` 传递 `mediaType`；两个序列化函数使用动态 MIME |

---

## 修改文件清单

| 文件 | 修改说明 |
|------|---------|
| `agent/src/agent.ts` | 添加 `serializeContentForOpenAI()`；修复 OpenAI path；`serializeContentForAnthropic` 使用动态 MIME；`generateUserPrompt` 传递 `mediaType` |
| `agent/src/models.ts` | `ImageAttachmentModel` 添加 `mimeType` 可选字段 |
| `agent/src/types/attachment.ts` | `ImageAttachment` 接口添加 `mimeType?: string` |
| `internal/conversation/types.go` | `InputAttachment` 添加 `MimeType` 字段 |
| `internal/channel/inbound/channel.go` | `buildInputAttachments()` 写入 `MimeType` |
| `internal/conversation/flow/resolver.go` | `buildGatewayAttachments()` 输出 `mime_type` |

---

## 架构说明

```
WeCom → adapter.go (下载/解密图片)
      → channel.go buildInputAttachments() (压缩 + 保留 MimeType)
      → conversation.InputAttachment{Base64, MimeType}
      → resolver.go buildGatewayAttachments() (转为 JSON map，含 mime_type)
      → agent HTTP 请求 body
      → agent.ts generateUserPrompt() (构建 ImagePart，含 mediaType)
      → stream() serializeContentForOpenAI/Anthropic()
      → 正确格式的 LLM API 请求
```

**关键区别**：
- WeCom 使用 `stream()` 直接 fetch 路径（本次修复的目标）
- Web UI 使用 Vercel AI SDK `ask()` 路径（此路径原本正常）

---

## 部署

```bash
cd /data2/memoh-v2
docker compose build agent server
docker compose stop agent server && docker compose up -d agent server
```

## Git 提交

`26bce0b3` — `fix(image): fix image recognition failure in WeCom conversations`
