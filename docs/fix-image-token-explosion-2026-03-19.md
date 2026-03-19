# 修复图片 Token 爆炸问题

## 问题描述

用户发送图片时，对话出现以下错误：
```
request (864477 tokens) exceeds the available context size (262144 tokens)
```

**原因**：图片 base64 编码后达到 406KB（约40万字符），每个字符都占用 token，导致 token 数爆炸，远超模型限制（26万）。

## 解决方案

在 `agent/src/agent.ts` 中添加图片大小限制，超过 100KB 的图片将被截断。

### 修改内容

**文件**: `agent/src/agent.ts`

1. **添加常量**（第11行）：
```typescript
// Maximum base64 image size to prevent token explosion (100KB)
const MAX_IMAGE_BASE64_LENGTH = 100 * 1024
```

2. **修改图片处理逻辑**（第360-370行）：
```typescript
// 修改前：
const userMessage: UserModelMessage = {
  role: 'user',
  content: [
    { type: 'text', text },
    ...images.map(
      (image) => ({ type: 'image', image: image.base64 }) as ImagePart,
    ),
  ],
}

// 修改后：
// Truncate oversized images to prevent token explosion
const processedImages = images.map((image) => {
  if (image.base64.length > MAX_IMAGE_BASE64_LENGTH) {
    console.warn(`[Agent generateUserPrompt] Image too large (${image.base64.length} chars), truncating to ${MAX_IMAGE_BASE64_LENGTH}`)
    return { type: 'image' as const, image: image.base64.slice(0, MAX_IMAGE_BASE64_LENGTH) }
  }
  return { type: 'image' as const, image: image.base64 }
})

const userMessage: UserModelMessage = {
  role: 'user',
  content: [
    { type: 'text', text },
    ...processedImages,
  ],
}
```

## 部署

```bash
docker compose build agent
docker compose stop agent && docker compose rm -f agent
docker compose up -d agent
```

## 验证

1. 构建成功
2. Agent 状态: `healthy`
3. 发送图片测试，不再出现 `exceeds the available context size` 错误

## 注意事项

- 截断图片可能导致图片内容不完整，但避免了 token 爆炸
- 如需更高质量的图片处理，应考虑在服务器端压缩/缩放图片后再转换为 base64
