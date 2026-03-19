# 52.4万上下文配置调整

## 背景

llama-server 上下文从 26.2万 扩展到 **52.4万 tokens**，使用 YaRN 技术实现。

```bash
# llama-server 配置变更
-c 524288                    # 52.4万上下文 (原 26.2万)
--cache-type-k q4_0         # KV cache 4-bit量化 (原 8-bit)
--cache-type-v q4_0         # KV cache 4-bit量化 (原 8-bit)
```

## 调整策略

### 1. 上下文分配 (52.4万总计)

| 用途 | 分配 | 说明 |
|-----|------|------|
| 系统提示 (System Prompt) | ~5万 | Bot Identity/Soul/Tools |
| 回复生成 (Response) | ~10万 | 模型输出 |
| 消息历史 (Messages) | **~25万** | 对话历史 + 图片 |
| 缓冲余量 (Buffer) | ~12.4万 | 安全余量 |

### 2. 图片压缩调整

**文件**: `internal/channel/inbound/image_compress.go`

| 参数 | 原值 (26万上下文) | 新值 (52万上下文) | 说明 |
|-----|------------------|------------------|------|
| smallImageThreshold | 200KB | **500KB** | 不压缩阈值提高 |
| largeImageThreshold | 500KB | **2MB** | 大图片阈值提高 |
| maxDimension | 768px | **1024px** | 最大尺寸提高 |
| jpegQuality | 80% | **85%** | 质量提高 |

**目标**: 单张图片 ~100-200KB (10-20万 tokens)

### 3. 消息历史限制调整

**文件**: `internal/settings/types.go`

| 场景 | 原值 | 新值 | 说明 |
|-----|------|------|------|
| DM 对话 | 10轮 | **20轮** | 翻倍 |
| 群聊对话 | 6轮 | **12轮** | 翻倍 |
| Evolution | 10轮 | **20轮** | 翻倍 |

**文件**: `internal/conversation/flow/resolver.go`

| 限制 | 原值 | 新值 | 说明 |
|-----|------|------|------|
| maxTotalTokens | 5万 | **25万** | 消息总token限制 |

## 部署

```bash
# 1. 重启 llama-server (已完成)
systemctl restart llama-qwen35

# 2. 构建并重启 server
docker compose build server
docker compose stop server && docker compose rm -f server
docker compose up -d server
```

## 服务状态

- llama-qwen35: ✅ active (524288 context, q4_0 KV cache)
- memoh-server: ✅ healthy

## 预期效果

### 图片处理
- **< 500KB**: 原图发送，不压缩
- **500KB - 2MB**: 压缩至 1024x1024，质量 85%
- **> 2MB**: 二次压缩（质量降至 60%）

### 对话容量
- 可支持 **20轮** 带图片的对话
- 或 **30-40轮** 纯文字对话
- 单张图片清晰度显著提升（1024px vs 768px）

## 监控建议

观察以下指标，如有需要进一步调整：

```bash
# 查看图片压缩日志
docker logs memoh-server 2>&1 | grep "image compression"

# 查看消息加载日志
docker logs memoh-server 2>&1 | grep "loadMessages"

# 监控 llama-server 显存
nvidia-smi
```

## 回滚

如出现问题，可回滚到 26万上下文：

```bash
# 修改 /etc/systemd/system/llama-qwen35.service.d/override.conf
-c 262144
--cache-type-k q8_0
--cache-type-v q8_0

systemctl restart llama-qwen35
```
