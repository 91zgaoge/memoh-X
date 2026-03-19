# Kimi Code 多模态支持修复

**日期**: 2026-03-19

## 背景

Memoh 在之前的更新（2026-03-19）中添加了 `kimi-coding` provider 类型支持，但仅实现了文本对话，未能使用 Kimi Code 的多模态（图片）能力。

通过分析 openclaw 项目（`/data2/openclaw`）的实现，发现 Kimi Code API 使用的是 **Anthropic Messages API 格式**，而非 OpenAI Chat Completions 格式。openclaw 的 `k2p5` 模型配置中明确声明 `input: ["text", "image"]`，API endpoint 为 `https://api.kimi.com/coding`。

## 问题分析

### 问题 1：错误的 API 格式

原实现（`agent/src/model.ts`）使用 `createOpenAI` 配合自定义 `X-Kimi-Client` 请求头连接 `https://api.moonshot.cn/v1`，这是 OpenAI-compatible 接口。

但 Kimi Code 实际上暴露的是 **Anthropic Messages API**：
- endpoint: `https://api.kimi.com/coding/v1/messages`
- 认证: `x-api-key` 头（而非 `Authorization: Bearer`）
- 需要 `anthropic-version: 2023-06-01` 头
- 图片格式: Anthropic base64 blocks（而非 OpenAI image_url）

### 问题 2：stream() 路径图片内容损坏

`agent/src/agent.ts` 的 `stream()` 函数是直接 `fetch()` 实现（绕过 AI SDK），对消息内容的序列化逻辑为：

```ts
content: typeof m.content === 'string' ? m.content : JSON.stringify(m.content)
```

当消息包含图片时，content 是数组 `[{type:'text', text:'...'}, {type:'image', image:'<base64>'}]`，会被 `JSON.stringify()` 序列化成 JSON 字符串发给 API，导致图片无法被识别。

### 问题 3：isMultimodal 配置错误

`model-catalog.ts` 中 kimi-coding 的两个模型均设为 `isMultimodal: false`，导致前端不允许用户为这些模型的 bot 上传图片。

## 修复方案

### 修改 1：`packages/web/src/data/model-catalog.ts`

```ts
// 修改前
'kimi-coding': {
  label: 'Kimi Code',
  defaultBaseUrl: 'https://api.moonshot.cn/v1',
  models: [
    { modelId: 'kimi-k2.5', ..., isMultimodal: false },
    { modelId: 'kimi-k2',   ..., isMultimodal: false },
  ],
},

// 修改后
'kimi-coding': {
  label: 'Kimi Code',
  defaultBaseUrl: 'https://api.kimi.com/coding',
  models: [
    { modelId: 'kimi-k2.5', ..., isMultimodal: true },
    { modelId: 'kimi-k2',   ..., isMultimodal: true },
  ],
},
```

### 修改 2：`agent/src/model.ts`

```ts
// 修改前：使用 createOpenAI
case ClientType.KimiCoding: {
  const customFetch = async (url, options) => {
    const headers = new Headers(options.headers)
    headers.set('X-Kimi-Client', 'kimi-cli')
    ...
  }
  const provider = createOpenAI({ apiKey, baseURL, fetch: customFetch })
  return provider.chat(modelId)
}

// 修改后：使用 createAnthropic
case ClientType.KimiCoding: {
  return createAnthropic({
    apiKey,
    baseURL,
    headers: {
      'X-Kimi-Client': 'kimi-cli',
      'X-Kimi-Client-Version': '1.0.0',
    },
  })(modelId)
}
```

`@ai-sdk/anthropic` 已安装（v3.0.9），`createAnthropic` 已在文件顶部导入，无需新增依赖。

### 修改 3：`agent/src/agent.ts`（stream() 函数）

在原 `stream()` 函数的 try 块中，根据 `clientType` 判断使用 Anthropic 还是 OpenAI 格式：

```ts
const isAnthropicFormat = (modelConfig as any).clientType === 'kimi-coding'

// 图片序列化辅助函数
const serializeContentForAnthropic = (content: unknown): unknown => {
  if (typeof content === 'string') return content
  if (!Array.isArray(content)) return String(content)
  return content.map((part: any) => {
    if (part.type === 'image') {
      return {
        type: 'image',
        source: { type: 'base64', media_type: 'image/jpeg', data: part.image },
      }
    }
    return part
  })
}

if (isAnthropicFormat) {
  fetchUrl     = `${baseUrl}/messages`
  fetchHeaders = { 'x-api-key': apiKey, 'anthropic-version': '2023-06-01', 'X-Kimi-Client': 'kimi-cli', ... }
  fetchBody    = {
    system: systemPrompt,  // 顶级字段，非消息数组元素
    messages: nonSystemMsgs.map(m => ({
      role: m.role,
      content: serializeContentForAnthropic(m.content),
    })),
    max_tokens: 8192,
  }
  // 响应解析
  content = data.content?.find(b => b.type === 'text')?.text || ''
} else {
  // 原 OpenAI 路径不变
}
```

## OpenAI vs Anthropic 格式对比

| 项目 | OpenAI | Anthropic |
|------|--------|-----------|
| endpoint | `/chat/completions` | `/messages` |
| 认证头 | `Authorization: Bearer {key}` | `x-api-key: {key}` |
| 版本头 | 无 | `anthropic-version: 2023-06-01` |
| 系统消息 | `{role: "system", content: "..."}` 在 messages 数组中 | 顶级 `system` 字段 |
| 图片格式 | `{type: "image_url", image_url: {url: "data:image/jpeg;base64,..."}}` | `{type: "image", source: {type: "base64", media_type: "image/jpeg", data: "..."}}` |
| 响应路径 | `choices[0].message.content` | `content[].text` |

## 数据库配置说明

本次修改的 `model-catalog.ts` 仅影响**新建** provider 时的默认值。

现有数据库中的 kimi-coding 配置：
- `base_url`: `https://api.kimi.com/coding/v1`（已正确，含 `/v1`）
- `is_multimodal`: `true`（已正确）
- `model_id`: `kimi-for-coding`

Anthropic SDK 的 baseURL 处理：SDK 会在 baseURL 后追加 `/messages`，因此 `https://api.kimi.com/coding/v1` → 请求发到 `https://api.kimi.com/coding/v1/messages`，与 `stream()` 函数的拼接逻辑一致。

## 验证方法

1. 向使用 kimi-coding bot 的 WeCom 账号发送含图片消息，确认 bot 能正确描述图片内容
2. 检查 agent 日志中 `[Agent stream] Fetch URL:` 应显示 `.../messages`（而非 `.../chat/completions`）
3. 纯文本消息依然正常响应

## 参考

- openclaw 项目实现：`/data2/openclaw/app/node_modules/@mariozechner/pi-ai/dist/providers/anthropic.js`
- openclaw 模型定义：`models.generated.js` 中 `kimi-coding.k2p5` 条目（`api: "anthropic-messages"`, `input: ["text", "image"]`）
