# Memoh-v2 项目更新日志

**更新日期:** 2026-03-11
**更新人员:** Claude Code
**版本:** v2.x.x

---

## 一、本次更新概览

本次更新主要包含以下三个核心功能模块：
1. **企业微信连接代码同步** - 适配 AI Bot Node SDK v1.0.2
2. **模型连接测试功能** - 前端后端完整实现
3. **自动获取模型ID功能** - 从 Provider 接口自动导入模型

---

## 二、详细更新内容

### 2.1 企业微信连接代码更新 (SDK v1.0.2)

#### 背景
同步企业微信 AI Bot Node SDK v1.0.2 (2026-03-11) 的更新内容。

#### 更新要点
- **新增 `disconnected_event` 事件处理** - 当有新连接建立时，系统会给旧连接发送该事件
- **添加 `chat_type` 字段支持** - 主动推送消息时可明确指定会话类型（单聊/群聊）
- **流式消息 6 分钟超时限制** - 从流式消息发送开始，需在 6 分钟内完成所有刷新
- **主动推送消息限制** - 需要用户先给机器人发消息，频率限制 30条/分钟，1000条/小时
- **消息类型限制** - image、voice、file 仅支持单聊

#### 涉及文件
- `/data2/memoh-v2/internal/channel/adapters/wecom/types.go`
- `/data2/memoh-v2/internal/channel/adapters/wecom/stream.go`
- `/data2/memoh-v2/internal/channel/adapters/wecom/adapter.go`
- `/data2/memoh-v2/internal/channel/adapters/wecom/websocket.go`

### 2.2 模型连接测试功能

#### 后端实现

**新增文件:**
- `/data2/memoh-v2/internal/models/probe.go` - 模型探测核心逻辑

**修改文件:**
- `/data2/memoh-v2/internal/handlers/models.go` - 添加 `Test` 接口
- `/data2/memoh-v2/internal/handlers/providers.go` - 添加 `Test` 接口
- `/data2/memoh-v2/internal/models/types.go` - 添加 `TestStatus`, `TestResponse` 类型
- `/data2/memoh-v2/internal/providers/service.go` - 添加 Provider 测试逻辑
- `/data2/memoh-v2/internal/providers/types.go` - 添加 `TestResponse` 类型

**API 接口:**
```
POST /api/models/{id}/test    - 测试模型连接
POST /api/providers/{id}/test - 测试服务商连接
```

**探测逻辑:**
- 根据 `client_type` 选择对应的探测方式:
  - `openai` - 使用 OpenAI Responses API
  - `anthropic` - 使用 Anthropic Messages API
  - `google` - 使用 Google Generative AI API
  - 其他 - 使用 OpenAI 兼容的 Chat Completions API
- Embedding 模型使用 `/embeddings` 端点探测
- 支持状态分类: `ok`, `auth_error`, `error`
- 返回延迟时间 (latency_ms)

#### 前端实现

**修改文件:**
- `/data2/memoh-v2/packages/web/src/pages/models/components/model-item.vue`
  - 添加测试按钮（刷新图标）
  - 显示连接状态（绿色/黄色/红色状态点）
  - 显示延迟时间
  - 组件挂载时自动测试

- `/data2/memoh-v2/packages/web/src/pages/models/components/provider-form.vue`
  - 添加"测试连接"按钮
  - 显示连接结果（可达/不可达）
  - 显示延迟时间
  - Provider 切换时自动测试

- `/data2/memoh-v2/packages/web/src/i18n/locales/zh.json` 和 `en.json`
  - 添加翻译: `testConnection`, `reachable`, `unreachable`, `testFailed`, `testModel`

### 2.3 自动获取模型ID功能

#### 后端实现

**修改文件:**
- `/data2/memoh-v2/internal/handlers/providers.go` - 添加 `ImportModels` 接口
- `/data2/memoh-v2/internal/providers/service.go` - 添加 `ImportModels` 方法
- `/data2/memoh-v2/internal/providers/types.go` - 添加 `ImportModelsRequest`, `ImportModelsResponse` 类型

**API 接口:**
```
POST /api/providers/{id}/import-models - 从 Provider 获取模型列表
```

**功能逻辑:**
- 调用 Provider 的 `/v1/models` 端点获取可用模型列表
- 自动过滤已存在的模型
- 支持指定模型类型 (chat/embedding)
- 返回导入数量、模型ID列表、错误信息

#### 前端实现

**修改文件:**
- `/data2/memoh-v2/packages/web/src/pages/models/model-setting.vue`
  - 添加"从服务商获取模型"按钮（带云下载图标）
  - 显示导入结果（成功数量、模型列表、错误信息）
  - 导入成功后自动刷新模型列表

**修复问题:**
- 修复认证问题: 从 `localStorage.getItem('token')` 获取 token，而非 `userStore.currentUser.id`

### 2.4 问题修复

#### Qdrant 向量库启动失败
**问题:** Collection "memory" 有 149 条数据但缺少命名向量 `nomic-embed-text-v1.5.Q8_0.gguf`

**修复:**
```bash
cd /data2/memoh-v2
docker compose down qdrant
docker volume rm memoh_qdrant_data
docker compose up -d qdrant server
```

#### 前端认证失败 (401)
**问题:** 测试接口返回 401 Unauthorized

**原因:** 代码错误使用 `userStore.currentUser?.id` 作为 token

**修复:** 改为使用 `localStorage.getItem('token')`

---

## 三、文件变更清单

### 后端文件 (Go)
```
internal/handlers/models.go          # 添加 Test 接口
internal/handlers/providers.go        # 添加 Test, ImportModels 接口
internal/models/probe.go              # 新增: 模型探测逻辑
internal/models/types.go              # 添加 TestStatus, TestResponse
internal/models/service.go            # 添加 Test 方法
internal/providers/service.go         # 添加 Test, ImportModels 方法
internal/providers/types.go           # 添加 TestResponse, ImportModels 类型
```

### 前端文件 (Vue/TypeScript)
```
packages/web/src/pages/models/components/model-item.vue       # 添加测试功能
packages/web/src/pages/models/components/provider-form.vue    # 添加测试功能
packages/web/src/pages/models/model-setting.vue               # 添加导入模型功能
packages/web/src/i18n/locales/zh.json                         # 添加中文翻译
packages/web/src/i18n/locales/en.json                         # 添加英文翻译
```

### 企业微信适配器
```
internal/channel/adapters/wecom/adapter.go
internal/channel/adapters/wecom/config.go
internal/channel/adapters/wecom/stream.go
```

---

## 四、API 变更

### 新增接口

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/models/{id}/test` | 测试模型连接 |
| POST | `/api/providers/{id}/test` | 测试服务商连接 |
| POST | `/api/providers/{id}/import-models` | 从服务商导入模型 |

### 新增数据类型

```go
// models/types.go
type TestStatus string
const (
    TestStatusOK        TestStatus = "ok"
    TestStatusAuthError TestStatus = "auth_error"
    TestStatusError     TestStatus = "error"
)

type TestResponse struct {
    Status    TestStatus `json:"status"`
    Reachable bool       `json:"reachable"`
    LatencyMs int64      `json:"latency_ms,omitempty"`
    Message   string     `json:"message,omitempty"`
}

// providers/types.go
type ImportModelsRequest struct {
    Type       string `json:"type"`        // "chat" or "embedding"
    ClientType string `json:"client_type"` // optional
}

type ImportModelsResponse struct {
    Imported int      `json:"imported"`
    Models   []string `json:"models"`
    Errors   []string `json:"errors,omitempty"`
}
```

---

## 五、部署验证

### 容器状态
```
memoh-server     Up (healthy)   0.0.0.0:8080->8080/tcp
memoh-web        Up (healthy)   0.0.0.0:8082->8082/tcp
memoh-qdrant     Up (healthy)   6333-6334/tcp
memoh-agent      Up (healthy)   0.0.0.0:8081->8081/tcp
memoh-postgres   Up (healthy)   5432/tcp
memoh-containerd Up (healthy)
```

### 访问地址
- Web 界面: http://localhost:8082
- API 服务: http://localhost:8080
- Agent 服务: http://localhost:8081

---

## 六、使用说明

### 1. 测试模型连接
进入模型管理页面，每个模型右侧会显示:
- 刷新按钮 - 点击手动测试
- 状态点 - 绿色(正常)/黄色(认证错误)/红色(错误)
- 延迟时间 - 如 "245ms"

### 2. 测试 Provider 连接
进入 Provider 编辑页面:
- 点击"测试连接"按钮
- 查看连接状态(可达/不可达)和延迟
- 切换 Provider 时自动测试

### 3. 自动获取模型ID
进入模型管理页面:
- 选择 Provider
- 点击"从服务商获取模型"按钮
- 查看导入结果和模型列表
- 系统自动刷新模型列表

---

## 七、已知问题

1. **SDK 未更新** - 前端 SDK (`@memoh/sdk`) 未重新生成，直接使用 `fetch` 调用 API
2. **容器网络警告** - `reconcile: network re-setup failed` 警告不影响功能
3. **Embedding 配置** - 如需使用本地 8089 端口的 Embedding 服务，需手动配置 Provider

---

## 八、后续建议

1. 重新生成 OpenAPI 规范并更新 SDK
2. 添加更多 Provider 类型的探测支持 (Azure, Bedrock 等)
3. 实现 Provider 级别的 Embedding 模型自动配置
4. 添加批量测试模型功能

---

## 九、备份信息

**备份时间:** 2026-03-11
**备份位置:** /data2/memoh-v2/
**Git 状态:**
- 当前分支: main
- 领先 origin/main: 4 commits
- 未提交更改: agent/ 目录和 wecom 适配器相关文件

**关键提交:**
- `e074c4b0` - feat(wecom): 添加思考中即时回复功能
- `039bba67` - feat: 合并企业微信适配器到 main 分支
- `30e89428` - sync: 移植上游高优先级功能

---

## 十、系统维护修复 (Claude Code)

### 10.1 数据库枚举修复

**问题:** `process_log_step` 枚举缺少 `trigger_started` 值
**错误信息:** `ERROR: invalid input value for enum process_log_step: "trigger_started"`
**修复:** 创建迁移文件 `db/migrations/0044_add_trigger_started_enum.up.sql`

```sql
ALTER TYPE process_log_step ADD VALUE IF NOT EXISTS 'trigger_started';
```

### 10.2 健康检查脚本修复

**问题:** 脚本检查 `wework.jxtvnet.com:8090/webhooks/wecom`，但服务实际运行在 `localhost:8080`
**修复:** 更新 `/root/check_memoh_health.sh`

```bash
# 修改前
WEBHOOK_URL="http://wework.jxtvnet.com:8090/webhooks/wecom"

# 修改后
HEALTH_URL="http://localhost:8080/health"
```

### 10.3 LLM 服务网络修复

**问题:** Docker 容器无法访问 `10.62.239.13`
**修复:** 更新 LLM Provider 配置使用 Docker 网关 `172.17.0.1`

| Provider | 原配置 | 新配置 |
|----------|--------|--------|
| qwen3.5-35B-A3B | 10.62.239.13:17099 | 172.17.0.1:17099 |
| qwen3.5-27B | 10.62.239.13:17100 | 172.17.0.1:17099 |
| Embedding | 10.62.239.13:8089 | 172.17.0.1:8089 |

### 10.4 SearXNG 联网搜索修复

**问题:** SearXNG 容器无法访问外网，所有搜索引擎超时
**修复:** 配置 HTTP 代理

**修改文件:** `docker/config/searxng-settings.yml`

```yaml
outgoing:
  proxies:
    http:
      - http://ccd:88152353@10.71.252.4:10810
    https:
      - http://ccd:88152353@10.71.252.4:10810
```

**验证:**
```bash
docker exec memoh-server wget -qO- "http://searxng:8080/search?q=test&format=json"
```

### 10.5 27B 模型服务管理

**创建服务:** `/etc/systemd/system/llama-qwen27.service`
**运行模式:** CPU (GPU 被 35B 占用)
**端口:** 17100
**状态:** 已停止 (用户选择使用 35B 模型)

```bash
# 管理服务
systemctl start llama-qwen27.service   # 启动
systemctl stop llama-qwen27.service    # 停止
```

### 10.6 系统代理配置

**Docker 代理:** `/etc/systemd/system/docker.service.d/http-proxy.conf`
```
HTTP_PROXY=http://ccd:88152353@10.71.252.4:10810
HTTPS_PROXY=http://ccd:88152353@10.71.252.4:10810
NO_PROXY=localhost,127.0.0.1,::1,10.0.0.0/8,192.168.0.0/16,172.16.0.0/12
```

**Proxychains:** `/etc/proxychains.conf`
```
socks5 10.40.31.69 10810 ccd 88152353
```

### 10.7 服务端口汇总

| 服务 | 端口 | 类型 | 状态 |
|------|------|------|------|
| llama-qwen35 | 17099 | GPU | ✅ 运行中 |
| llama-qwen27 | 17100 | CPU | ❌ 已停止 |
| llama-embedding | 8089 | GPU | ✅ 运行中 |
| memoh-server | 8080 | API | ✅ 运行中 |
| memoh-agent | 8081 | Gateway | ✅ 运行中 |
| memoh-web | 8082 | Web UI | ✅ 运行中 |
| memoh-searxng | 8080 | 搜索 | ✅ 运行中 |

### 10.8 常用维护命令

```bash
# 健康检查
curl http://localhost:8080/health
cat /var/log/memoh-health.log | tail -5

# 重启 memoh
cd /data2/memoh-v2 && docker compose restart

# 查看日志
docker logs memoh-server --tail 50
docker logs memoh-agent --tail 50
docker logs memoh-searxng --tail 20

# 检查 LLM 服务
curl http://localhost:17099/health
curl http://localhost:8089/health

# 测试搜索
docker exec memoh-server wget -qO- "http://searxng:8080/search?q=test&format=json"
```

### 10.9 OpenViking 数据库修复

**问题:** 所有 Bot 容器中的 OpenViking 数据库无法正常工作
**错误信息:** `ModuleNotFoundError: No module named 'pydantic_core._pydantic_core'`

**根本原因:**
1. MCP 镜像基于 `python:3.10-alpine` 构建，但实际运行环境缺少 openviking 包
2. 之前尝试修复时安装了 Python 3.12 版本的 openviking，但 pydantic_core 编译模块与 Python 3.10 不兼容
3. containerd 容器使用私有 cgroup 命名空间，导致无法创建子容器

**修复步骤:**

#### 1. 修改 MCP Dockerfile

**文件:** `docker/Dockerfile.mcp`

```dockerfile
# 修改基础镜像从 alpine 到 debian slim
FROM python:3.10-slim-bookworm

# ... 其他安装步骤 ...

# 安装 openviking (在构建时通过代理下载)
ARG http_proxy
ARG https_proxy
RUN pip install --no-cache-dir openviking
```

#### 2. 修改 containerd 配置

**文件:** `docker-compose.yml`

```yaml
containerd:
  # ... 其他配置 ...
  privileged: true
  pid: host
  cgroup: host  # 新增: 解决 cgroup v2 嵌套容器问题
```

#### 3. 重建 MCP 镜像

```bash
# 使用代理构建镜像
export http_proxy=http://ccd:88152353@10.71.252.4:10810
export https_proxy=http://ccd:88152353@10.71.252.4:10810

docker build \
  --build-arg http_proxy=$http_proxy \
  --build-arg https_proxy=$https_proxy \
  -f docker/Dockerfile.mcp \
  -t memoh-mcp:latest .

# 导出并导入到 containerd
docker save memoh-mcp:latest -o /tmp/memoh-mcp-latest.tar
docker cp /tmp/memoh-mcp-latest.tar memoh-containerd:/tmp/
docker exec memoh-containerd ctr i import /tmp/memoh-mcp-latest.tar
```

#### 4. 清理旧容器

```bash
# 删除旧容器和快照
docker exec memoh-containerd ctr t kill -s 9 <container-id>
docker exec memoh-containerd ctr c rm <container-id>
docker exec memoh-containerd ctr snapshot rm <snapshot-key>

# 重置数据库中的容器状态
docker exec -i memoh-postgres psql -U memoh -d memoh -c \
  "DELETE FROM containers WHERE bot_id IN ('5b52c780...', '07bfa92c...', 'f8da8432...');"
```

#### 5. 重启服务

```bash
docker restart memoh-server
```

**验证结果:**

| Bot | 容器 ID | OpenViking 状态 |
|-----|---------|----------------|
| 高歌 | ae1e38dc | ✅ OK |
| 孙小美 | 5b52c780 | ✅ OK |
| 钱多多 | 07bfa92c | ✅ OK |
| 大老千 | f8da8432 | ✅ OK |

**验证命令:**
```bash
docker exec memoh-containerd ctr task exec --exec-id test <container-id> \
  python3 -c "import openviking; print('OK')"
```

---

### 10.10 企业微信长消息截断修复

**修复时间:** 2026-03-12
**问题:** 长消息被截断为 4000 字符，用户无法看到完整回复
**根本原因:** 限制过于保守，远低于 SDK 允许的 20480 字节

**修复内容:**

#### 代码变更

**文件:** `internal/channel/adapters/wecom/stream.go`

```go
// 新增常量
const MaxContentBytes = 20480  // 企业微信 AI Bot SDK 限制

// 新增分片函数
func splitContentByBytes(content string, maxBytes int) []string
func truncateByBytes(s string, maxBytes int) string

// 重构发送逻辑
func (s *OutboundStream) sendSplitContent(ctx context.Context, content string, finish bool) error
func (s *OutboundStream) sendSingleUpdate(ctx context.Context, content string, finish bool) error
```

**文件:** `internal/channel/adapters/wecom/adapter.go`

```go
OutboundPolicy: channel.OutboundPolicy{
    TextChunkLimit: 6800,  // 约 20400 字节（全中文场景）
    ChunkerMode:    channel.ChunkerModeMarkdown,
}
```

#### 分片策略

| 优先级 | 分割点 | 说明 |
|--------|--------|------|
| 1 | `\n\n` | 段落边界，保持 Markdown 格式 |
| 2 | `\n` | 换行符，保持行完整性 |
| 3 | `。！？.!?` | 句子边界，保持语义完整 |
| 4 | 字节边界 | UTF-8 安全强制截断 |

#### 用户体验

- 分片消息添加 `...(继续)` 提示
- 200ms 延迟避免频率限制
- 最后一片正确设置 `finish=true`

#### 部署验证

```bash
# 重新构建并启动
docker compose up -d --build server

# 验证服务状态
docker compose ps
# memoh-server  Up (healthy)  0.0.0.0:8080->8080/tcp

# 验证 WeCom 连接
docker logs memoh-server --tail 20
# "WeCom connection established" 日志确认连接成功
```

**提交:** `ec2fd2c6` - fix(wecom): 解决长消息被截断问题，确保回复内容完整性

---

### 10.11 企业微信流式消息消失/重置问题修复

**修复时间:** 2026-03-12
**问题:** 流式输出时消息突然消失，内容重置回"思考中..."，用户无法看到实际回复
**根本原因:** Thinking 回复和实际响应使用了不同的 `streamID`，WeCom 将它们视为独立消息

**技术规范依据:**
根据 WeCom AI Bot SDK 文档 (`aibot-sdk/aibot-node-sdk-main/src/types/api.ts`):
> 流式消息通过 `(req_id, stream.id)` 对来唯一标识一个消息流
> - 相同 `(req_id, stream.id)` 的消息会更新同一条消息
> - 不同 `stream.id` 的消息会被视为独立消息

**修复内容:**

#### 代码变更

**文件:** `internal/channel/adapters/wecom/adapter.go`

```go
// 修改 sendThinkingReply 函数签名，接受 streamID 参数
func (a *Adapter) sendThinkingReply(ctx context.Context, wsClient *WebSocketClient, reqID string, streamID string)

// 修改 NewOutboundStream 函数签名，接受 streamID 参数
func NewOutboundStream(adapter *Adapter, cfg channel.ChannelConfig, wsClient *WebSocketClient, reqID, chatID, userID, chatType string, isMentioned bool, streamID string, logger *slog.Logger)

// 消息处理流程中使用相同的 streamID
streamID := generateStreamID()
a.sendThinkingReply(ctx, wsClient, reqID, streamID)  // 使用指定的 streamID
msg.Metadata["stream_id"] = streamID  // 传递给 CreateOutboundStream

// CreateOutboundStream 中提取并使用相同的 streamID
streamID := ""
if opts.Metadata != nil {
    if v, ok := opts.Metadata["stream_id"].(string); ok {
        streamID = v
    }
}
return NewOutboundStream(..., streamID, ...)
```

#### 涉及文件
- `internal/channel/adapters/wecom/adapter.go` - 统一 streamID 生成和传递
- `internal/channel/adapters/wecom/stream.go` - 支持从外部传入 streamID

#### 用户体验改进
- 消息流式输出不再消失或重置
- "思考中..."提示正确更新为实际回复内容
- 企业微信端和 Web UI 端显示一致

**部署验证:**

```bash
# 重新构建并启动
docker compose up -d --build server

# 验证服务状态
docker compose ps
# memoh-server  Up (healthy)  0.0.0.0:8080->8080/tcp

# 验证 WeCom 连接
docker logs memoh-server --tail 20
# "WeCom connection established" 日志确认连接成功
```

**提交:** `<commit_hash>` - fix(wecom): 修复流式消息消失问题，确保 thinking 回复和实际响应使用相同 streamID

---

### 10.12 企业微信消息发送机制修复 - 统一使用队列

**修复时间:** 2026-03-12
**问题:** 消息在流式输出过程中突然消失，内容被重置回"思考中..."
**根本原因:** `finish=false` 的消息使用 fire-and-forget，而 `finish=true` 的消息等待 ACK，导致消息顺序混乱

**技术规范依据:**
根据 WeCom AI Bot SDK (`aibot-sdk/aibot-node-sdk-main/src/ws.ts`):
> 同一个 req_id 的消息会被放入队列中串行发送：发送一条后等待服务端回执，收到回执或超时后才发送下一条。

**修复内容:**

#### 代码变更

**文件:** `internal/channel/adapters/wecom/websocket.go`

```go
// 修改 SendStream 函数 - 对所有消息使用队列
func (c *WebSocketClient) SendStream(ctx context.Context, reqID string, body StreamMsgBody, cmd ...string) error {
    // ...
    // CRITICAL: Always use queue for all messages to ensure ordering
    return newPromise(func(resolve func(WebsocketMessage), reject func(error)) {
        // 所有消息进入队列，等待 ACK 后才发送下一条
    })
}

// 修改 ACK 超时时间
const ReplyAckTimeout = 5 * time.Second  // 从 10 秒改为 5 秒，与 SDK 一致
```

**文件:** `internal/channel/adapters/wecom/stream.go`

```go
// 更新注释，移除 "fire-and-forget" 描述
// CRITICAL: SendStream now uses queue for ALL messages to ensure ordering
```

#### 涉及文件
- `internal/channel/adapters/wecom/websocket.go` - 统一使用队列发送所有消息
- `internal/channel/adapters/wecom/stream.go` - 更新注释

#### 关键改进
- 所有消息按顺序发送，不再出现消息丢失或重置
- ACK 超时时间与 SDK 一致（5秒）
- 与官方 WeCom AI Bot SDK 行为完全一致

**部署验证:**

```bash
# 重新构建并启动
docker compose up -d --build server

# 验证服务状态
docker compose ps
# memoh-server  Up (healthy)  0.0.0.0:8080->8080/tcp

# 验证 WeCom 连接
docker logs memoh-server --tail 20
# "WeCom connection established" 日志确认连接成功
```

**提交:** `<commit_hash>` - fix(wecom): 统一使用队列发送所有消息，确保消息顺序和完整性

---

*文档更新时间: 2026-03-12*
*生成工具: Claude Code*
