# 图片智能压缩方案

## 问题回顾

用户发送图片时出现 token 爆炸错误：
```
request (864477 tokens) exceeds the available context size (262144 tokens)
```

**原因**：3MB 图片 base64 编码后约 400 万字符，导致 LLM token 数爆炸。

## 解决方案：Server 端智能图片压缩

### 核心设计

采用分层压缩策略，根据原图大小智能选择压缩参数：

| 原图大小 | 处理方式 | 最大尺寸 | JPEG 质量 | 预期输出 |
|---------|---------|---------|----------|---------|
| < 100KB | 不压缩 | 原尺寸 | - | 保持原样 |
| 100KB - 1MB | 中等压缩 | 1024x1024 | 85% | ~100-300KB |
| > 1MB | 强力压缩 | 512x512 | 70% | ~50-150KB |

### 技术实现

**新文件**: `internal/channel/inbound/image_compress.go`

```go
// 压缩流程
1. 检查图片大小，小图直接跳过
2. 解码图片获取尺寸
3. 计算目标尺寸（保持宽高比）
4. 使用简单缩放算法调整大小
5. 编码为 JPEG 格式
6. 如果压缩后更小则使用，否则保留原图
```

**修改**: `internal/channel/inbound/channel.go`

在 `buildInputAttachments` 函数中，图片处理逻辑改为：
```go
// 压缩图片如果必要
compressedData, mimeType, wasCompressed := compressImageIfNeeded(att.Data, att.Mime, logger)
if wasCompressed {
    logger.Info("image compressed for LLM",
        slog.String("originalSize", formatBytes(len(att.Data))),
        slog.String("compressedSize", formatBytes(len(compressedData))))
}
out = append(out, conversation.InputAttachment{
    Type:   "image",
    Base64: base64.StdEncoding.EncodeToString(compressedData),
})
```

### 相比暴力截断的优势

| 方案 | 优点 | 缺点 |
|-----|------|------|
| 暴力截断 | 简单 | 图片损坏无法显示 |
| **智能压缩** | 图片完整可用、可调节压缩策略 | 需要 CPU 资源 |

### 回滚暴力截断

同时回滚 agent 端的暴力截断代码，恢复原逻辑：

**文件**: `agent/src/agent.ts`

- 删除 `MAX_IMAGE_BASE64_LENGTH` 常量
- 删除图片截断逻辑
- 恢复原图片处理

## 部署

```bash
docker compose build server
docker compose stop server && docker compose rm -f server
docker compose up -d server
```

## 验证

1. 服务启动正常：`healthy`
2. 发送小图片（<100KB）：保持原样
3. 发送中等图片（100KB-1MB）：压缩至 1024x1024
4. 发送大图片（>1MB）：压缩至 512x512

## 日志示例

```
image compressed for LLM
  originalSize: 3.2 MB
  compressedSize: 245 KB
  mimeType: image/jpeg
```
