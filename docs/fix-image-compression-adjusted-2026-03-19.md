# 图片压缩参数调整

## 问题反馈
用户反馈图片压缩太厉害，LLM 识别不清或出现严重误读。

## 调整内容

### 调整前（过于激进）
```go
smallImageThreshold  = 100 * 1024   // 100KB
mediumImageThreshold = 1024 * 1024  // 1MB
maxSmallDimension    = 1024
maxLargeDimension    = 512
jpegQualityMedium    = 85
jpegQualityHigh      = 70
```

### 调整后（更保守）
```go
smallImageThreshold  = 800 * 1024    // 800KB (提升 8 倍)
mediumImageThreshold = 2 * 1024 * 1024 // 2MB (提升 2 倍)
maxSmallDimension    = 1536         // 1536px (提升 50%)
maxLargeDimension    = 1024         // 1024px (提升 100%)
jpegQualityMedium    = 90           // 提升 5%
jpegQualityHigh      = 85           // 提升 15%
```

## 预期效果

| 图片大小 | 处理方式 |
|---------|---------|
| < 800KB | 不压缩，保持原样 |
| 800KB - 2MB | 压缩至 1536x1536，质量 90% |
| > 2MB | 压缩至 1024x1024，质量 85% |

## 部署

```bash
docker compose build server
docker compose stop server && docker compose rm -f server
docker compose up -d server
```

## 服务状态

- memoh-server: ✅ healthy

## 后续建议

如需进一步提升图片质量，可考虑：
1. 引入 `golang.org/x/image/draw` 库使用双线性/双三次插值
2. 使用支持更高分辨率的模型（如 GPT-4V 支持更高分辨率）
3. 对文字类图片使用无损压缩策略
