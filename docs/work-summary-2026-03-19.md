# 2026-03-19 工作要点总结

## 零、WeCom 消息隔离 + 心跳任务隔离 + 上下文修复

### 问题汇总
- 用户 A 的消息被回复给用户 B（跨用户广播）
- 单聊用户共享同一条路由，历史混合
- `/new` 命令清空所有用户历史
- 心跳维护内容混入用户回复
- 上下文 288K tokens 超出 256K 限制

### 核心修复

#### 1. WeCom 消息隔离
| 文件 | 修改 |
|------|------|
| `wecom/adapter.go` | 单聊用 `UserID` 作为 `Conversation.ID` |
| `inbound/channel.go` | 禁用 `broadcastToOtherChannels`；修复 debounce key |
| `message/service.go` | 新增 `DeleteByRoute()` 精确清除 |
| `cmd/agent/main.go` | 注入 `routeService` |

#### 2. 心跳任务隔离（`resolver.go`）
- 设置 `MaxContextLoadTime: -1`，**禁止加载**用户历史
- 禁止调用 `storeRound()`，**不存储**结果到历史表
- 数据清理：删除 11 条已存在的心跳污染消息

#### 3. 上下文大小修复
| 文件 | 修改 |
|------|------|
| `resolver.go` | `maxTotalTokens` 250000 → **150000** |
| `settings/types.go` | DM: 20→**10**；Channel: 12→**6** |

### 场景矩阵
| 场景 | Conversation.ID | 历史隔离 |
|------|----------------|---------|
| 用户 A 单聊 | A 的 userid | ✅ 独立 |
| 用户 B 单聊 | B 的 userid | ✅ 独立 |
| 群聊 | 群 chatid | ✅ 独立 |
| Heartbeat | 无状态 | ✅ 不加载/不存储 |

### 详细文档
- `docs/wecom-isolation-and-heartbeat-fix-2026-03-19.md`

---

## 一、删除对话消息缓存功能

### 问题
Memoh 对话中出现"处理过程中断，请重试"错误，缓存功能与其他功能冲突。

### 解决方案
完全删除 `internal/conversation/flow/cache.go` 及 `resolver.go` 中的缓存相关代码。

### 修改文件
1. **删除**: `internal/conversation/flow/cache.go`
2. **修改**: `internal/conversation/flow/resolver.go`
   - 删除 `cache *ResponseCache` 字段
   - 删除 `NewResponseCache(1000, 5*time.Minute, log)` 初始化
   - 删除 `Chat()` 方法中的缓存查询逻辑
   - 删除响应存入缓存的逻辑

### 相关文档
- `/data2/memoh-v2/docs/remove-conversation-cache-2026-03-19.md`

---

## 二、修复图片 Token 爆炸问题

### 问题
用户发送图片时出现错误：
```
request (864477 tokens) exceeds the available context size (262144 tokens)
```

### 解决方案

#### 1. 新增图片智能压缩模块
**文件**: `internal/channel/inbound/image_compress.go`

核心功能：
```go
// 压缩策略
- 小图 (<500KB): 保持原样
- 中图 (500KB-2MB): 压缩至 1024x1024, 质量 85%
- 大图 (>2MB): 二次压缩至更低质量

// 压缩后目标: ~100-200KB (10-20万 tokens)
```

#### 2. 修改消息附件处理
**文件**: `internal/channel/inbound/channel.go`

在 `buildInputAttachments()` 中调用压缩：
```go
compressedData, mimeType, wasCompressed := compressImageIfNeeded(att.Data, att.Mime, logger)
```

#### 3. 参数调整历程
| 阶段 | 小图阈值 | 大图阈值 | 最大尺寸 | 质量 | 备注 |
|-----|---------|---------|---------|------|------|
| 初始 | 100KB | 1MB | 768px | 80% | 过于激进 |
| 调整1 | 800KB | 2MB | 1536px | 90% | 过于宽松 |
| **最终** | **500KB** | **2MB** | **1024px** | **85%** | 平衡方案 |

### 相关文档
- `/data2/memoh-v2/docs/fix-image-token-explosion-2026-03-19.md`
- `/data2/memoh-v2/docs/fix-image-compression-2026-03-19.md`
- `/data2/memoh-v2/docs/fix-image-compression-adjusted-2026-03-19.md`

---

## 三、扩展 LLM 上下文窗口

### 问题
即使图片压缩后，26.2万上下文仍不够用（系统提示 + 图片 + 历史消息）。

### 解决方案

#### 1. 扩展 llama-server 上下文
**文件**: `/etc/systemd/system/llama-qwen35.service.d/override.conf`

```bash
# 修改前
-c 262144
--cache-type-k q8_0
--cache-type-v q8_0

# 修改后
-c 524288                    # 52.4万上下文 (2倍)
--cache-type-k q4_0          # 4-bit KV cache
--cache-type-v q4_0          # 4-bit KV cache
```

#### 2. 修复数据库配置
**问题**: 数据库中 `context_window` 仍为 128000

```sql
-- 修复命令
UPDATE models
SET context_window = 524288
WHERE model_id = 'Qwen3.5-35B-A3B-UD-Q4_K_XL.gguf';
```

#### 3. 显存使用分析
| 组件 | 显存占用 |
|-----|---------|
| 模型 (35B @ 4-bit) | ~20 GB |
| KV cache (26万 @ q4_0) | ~1.5 GB |
| 其他 | ~2 GB |
| **总计** | **~23.5 GB / 48 GB** |

**注意**: Qwen3..5-35B-A3B 原生支持 26.2万上下文，`n_ctx_train=262144` 为硬限制。

### 相关文档
- `/data2/memoh-v2/docs/adjust-for-524k-context-2026-03-19.md`
- `/data2/memoh-v2/docs/fix-context-window-config-2026-03-19.md`

---

## 四、调整消息历史限制

### 背景
配合 52.4万上下文，增加对话轮数。

### 修改内容

#### 1. 默认历史限制
**文件**: `internal/settings/types.go`

```go
// 修改历程
DefaultDMHistoryLimit       = 10 → 20 → 10  (最终: 10)
DefaultChannelHistoryLimit  = 6  → 12 → 6   (最终: 6)

// 原因: llama-server 实际仅支持 256K 上下文，需降低历史限制防止超限
```

#### 2. Token 限制
**文件**: `internal/conversation/flow/resolver.go`

```go
// 修改历程
const maxTotalTokens = 50000  // 初始: 5万
const maxTotalTokens = 250000 // 中间: 25万 (配合52万上下文)
const maxTotalTokens = 150000 // 最终: 15万 (适配256K实际限制)
```

#### 3. Token 估算函数
新增 `estimateMessageTokens()` 函数，实时估算消息 token：
```go
func estimateMessageTokens(msg conversation.ModelMessage) int {
    // 文本: 1.3 tokens/字符
    // 图片 base64: 1 token/字符
    // 开销: 4 tokens/消息
}
```

### 上下文分配策略 (25.6万总计 - 实际)

| 用途 | 分配 | 说明 |
|-----|------|------|
| 系统提示 | ~3万 | Bot Identity/Soul/Tools |
| 回复生成 | ~3万 | 模型输出空间 |
| 消息历史 | ~15万 | 对话历史 (maxTotalTokens) |
| 缓冲余量 | ~5.6万 | 安全余量 |

**注意**: llama-server 实际仅支持 256K 上下文（模型 `n_ctx_train=262144`），512K 配置无效。

---

## 五、完整配置清单

### llama-server
```bash
# /etc/systemd/system/llama-qwen35.service.d/override.conf
ExecStart=/root/llama.cpp/build/bin/llama-server \
  -m /data1/models/Qwen3.5-35B-A3B-UD-Q4_K_XL.gguf \
  --mmproj /data1/models/mmproj-F16.gguf \
  --jinja \
  --host 0.0.0.0 \
  --port 17099 \
  --api-key-file /root/.llama/api_keys \
  -ngl 99 \
  --flash-attn on \
  --tensor-split 1,1 \
  -c 262144 \                    # ← 25.6万上下文 (模型硬限制 n_ctx_train=262144)
  --cache-type-k q4_0 \          # ← 4-bit K cache
  --cache-type-v q4_0 \          # ← 4-bit V cache
  -t 32 \
  --temp 0.6 \
  --top-k 20 \
  --top-p 0.95 \
  --min-p 0.05 \
  --repeat-penalty 1.05 \
  --chat-template-kwargs '{"enable_thinking": false}' \
  --reasoning-budget 0 \
  --no-mmap
```

### 数据库
```sql
-- 模型上下文配置 (实际限制 256K，数据库配置应与实际一致)
UPDATE models
SET context_window = 262144
WHERE model_id = 'Qwen3.5-35B-A3B-UD-Q4_K_XL.gguf';

-- 验证
SELECT name, model_id, context_window FROM models;
-- Qwen3.5-35B-A3B | Qwen3.5-35B-A3B-UD-Q4_K_XL.gguf | 262144
```

### 环境变量
无特殊环境变量，所有配置通过代码和数据库管理。

---

## 六、关键文件变更

### 新增文件
1. `internal/channel/inbound/image_compress.go` - 图片压缩模块
2. `docs/remove-conversation-cache-2026-03-19.md` - 缓存删除文档
3. `docs/fix-image-token-explosion-2026-03-19.md` - 图片修复文档
4. `docs/fix-image-compression-2026-03-19.md` - 压缩策略文档
5. `docs/fix-image-compression-adjusted-2026-03-19.md` - 参数调整文档
6. `docs/adjust-for-524k-context-2026-03-19.md` - 上下文扩展文档
7. `docs/fix-context-window-config-2026-03-19.md` - 配置修复文档
8. **`docs/wecom-isolation-and-heartbeat-fix-2026-03-19.md`** - **WeCom 隔离 + 心跳隔离文档（本次新增）**
9. `docs/work-summary-2026-03-19.md` - 本文件

### 修改文件
1. `internal/conversation/flow/cache.go` - **已删除**
2. `internal/conversation/flow/resolver.go` - 删除缓存、添加 token 估算、**心跳隔离（本次）**
3. `internal/channel/inbound/channel.go` - 添加图片压缩调用、**禁用广播（本次）**
4. `internal/settings/types.go` - 调整历史限制
5. `agent/src/agent.ts` - 回滚暴力截断代码
6. **`internal/channel/adapters/wecom/adapter.go`** - **单聊 ID 修复、Metadata 补全（本次）**
7. **`internal/message/service.go`** - **新增 `DeleteByRoute()`（本次）**
8. **`cmd/agent/main.go`** - **注入 routeService（本次）**
6. `CHANGELOG.md` - 更新日志

### 配置文件
1. `/etc/systemd/system/llama-qwen35.service.d/override.conf` - llama-server 配置

---

## 七、部署步骤

### 1. 重启 llama-server
```bash
systemctl daemon-reload
systemctl restart llama-qwen35

# 验证
systemctl status llama-qwen35
nvidia-smi  # 检查显存
```

### 2. 更新数据库
```bash
# 更新为实际支持的 256K 上下文
docker exec memoh-postgres psql -U memoh -d memoh -c \
  "UPDATE models SET context_window = 262144 WHERE model_id = 'Qwen3.5-35B-A3B-UD-Q4_K_XL.gguf';"
```

### 3. 构建并重启 Memoh
```bash
cd /data2/memoh-v2

# 构建
docker compose build server
docker compose build agent  # 如需

# 重启
docker compose stop server agent
docker compose rm -f server agent
docker compose up -d server agent

# 验证
docker compose ps
```

---

## 八、验证方法

### 1. 检查 llama-server
```bash
curl -s http://localhost:17099/v1/models \
  -H "Authorization: Bearer $(cat /root/.llama/api_keys | head -1)"
# 应返回 262144 context (256K 实际限制)
```

### 2. 检查数据库
```bash
docker exec memoh-postgres psql -U memoh -d memoh -c \
  "SELECT context_window FROM models WHERE model_id = 'Qwen3.5-35B-A3B-UD-Q4_K_XL.gguf';"
# 应返回 262144 (256K)
```

### 3. 检查服务日志
```bash
# 图片压缩日志
docker logs memoh-server 2>&1 | grep "image compression"

# 消息加载日志
docker logs memoh-server 2>&1 | grep "loadMessages"

# 错误日志
docker logs memoh-server 2>&1 | grep -i error
```

### 4. 功能测试
- 发送小图片 (<500KB): 应不压缩直接发送
- 发送大图片 (>2MB): 应压缩至 1024x1024
- 长对话测试: 应支持 20 轮以上对话

---

## 九、回滚方案

### 回滚到 26.2万上下文
```bash
# 1. 修改 llama-server 配置
sudo systemctl edit llama-qwen35 --full
# 改回: -c 262144 --cache-type-k q8_0 --cache-type-v q8_0

# 2. 更新数据库
docker exec memoh-postgres psql -U memoh -d memoh -c \
  "UPDATE models SET context_window = 262144 WHERE model_id = 'Qwen3.5-35B-A3B-UD-Q4_K_XL.gguf';"

# 3. 重启服务
systemctl restart llama-qwen35
docker compose restart server
```

### 恢复缓存功能
```bash
git checkout HEAD -- internal/conversation/flow/cache.go
git checkout HEAD -- internal/conversation/flow/resolver.go
docker compose build server
docker compose restart server
```

---

## 十、已知限制

1. **上下文限制**: llama-server 实际仅支持 256K 上下文（模型 `n_ctx_train=262144` 硬限制），512K 配置无效
2. **显存使用**: 当前配置使用 ~23.5GB / 48GB，余量充足
3. **图片压缩**: 简单缩放算法，非双线性/双三次插值，压缩后质量有限
4. **Token 估算**: 使用简单公式估算，与实际可能有偏差
5. **WeCom 单聊**: 依赖 `From.UserID` 区分用户，若 SDK 行为变化需重新适配

---

**记录时间**: 2026-03-19
**操作人员**: Claude Code
**相关 Issue**: 对话中断、图片 token 爆炸、上下文不足、WeCom 消息串扰、心跳任务污染历史
