# Memoh 对话中断及无回复内容问题修复记录

**日期**: 2026-03-16
**问题**:
1. 对话中出现"处理过程中断，请重试"错误
2. 机器人只回复"处理完成，请查看完整回复"，不显示实际内容

---

## 第一部分：对话中断问题修复

### 问题分析

#### 根本原因
1. **sub2api 格式转换问题**: sub2api 将 OpenAI Chat Completions API 请求转换为 OpenAI Responses API 格式，但 llama-server 只支持标准 Chat Completions API
2. **message role 不兼容**: Qwen3.5 模板的 chat template 不支持 `function` role，只支持 `tool` role
3. **架构复杂性**: 请求链路为 Memoh → sub2api → llama-server，中间环节增加了故障点

#### 错误信息
```
Cannot determine type of 'item'
Jinja Exception: Unexpected message role
```

### 修复措施

#### 1. sub2api 增加透传模式（备用方案）
**文件**: `/tmp/sub2api/backend/internal/service/openai_gateway_chat_completions.go`

- 为本地 LLM 服务器（非 OpenAI 官方 API）增加直接透传模式
- 跳过 Responses API 格式转换，直接转发原始 Chat Completions 请求
- 自动将 `function` role 转换为 `tool` role（Qwen3.5 模板兼容）

**关键代码**:
```go
// 检查是否为本地/非 OpenAI 上游服务器
if account.IsOpenAI() && account.Type == AccountTypeAPIKey {
    baseURL := account.GetOpenAIBaseURL()
    if isNonOpenAIUpstream(baseURL) {
        return s.forwardChatCompletionsDirectly(ctx, c, account, body, startTime)
    }
}

// 转换 function role 为 tool role
for i := range chatReq.Messages {
    if chatReq.Messages[i].Role == "function" {
        chatReq.Messages[i].Role = "tool"
        bodyModified = true
    }
}
```

#### 2. 配置 Memoh 直连 llama-server（实施方案）
**数据库更新**:
```sql
-- 将模型提供商从 sub2api 改为直连 llama-server
UPDATE llm_providers
SET base_url = 'http://172.17.0.1:17099/v1',
    api_key = '76e0e2bc84c45374999a1d5e66962544c09cc00ae42ad25cd6a2a07a9d7fe330'
WHERE id = '243a7f8a-728e-4813-8e0b-f5096ef46dbd';
```

**配置变更**:
| 配置项 | 原值 | 新值 |
|--------|------|------|
| base_url | http://172.26.0.1:28000/v1 | http://172.17.0.1:17099/v1 |
| 连接方式 | 通过 sub2api | 直连 llama-server |

#### 3. 重启服务
```bash
cd /data2/memoh-v2 && docker compose restart server agent
```

---

## 第二部分：无回复内容问题修复

### 问题分析

**现象**: 机器人只回复"处理完成，请查看完整回复"，不显示实际对话内容

#### 根本原因

1. **代码 Bug**: `/data2/memoh-v2/agent/src/agent.ts` 中 `stream` 函数引用了未定义的变量 `result`
   - 位置：第 875 行 `stripAttachmentsFromMessages(result.messages)`
   - 影响：导致代码运行时抛出异常，无法正常返回 LLM 生成的内容

2. **网络隔离**: Agent 容器无法访问 `172.17.0.1`
   - Agent 容器在 `memoh_memoh-network` 网络（172.26.0.0/16）
   - LLM 服务监听在 `172.17.0.1`（docker0 网关）
   - 两个网络之间无法直接通信

3. **HTTP 代理问题**: `HTTP_PROXY` 环境变量导致本地 LLM 请求被转发到外部代理
   - Agent 服务配置了 `HTTP_PROXY=http://ccd:88152353@10.40.31.69:10810`
   - 所有 HTTP 请求（包括本地 LLM 请求）都被转发到代理服务器
   - 代理服务器返回 503 错误，导致请求失败

#### 错误信息
```
[Agent stream] Error: LLM API error: 503
error: LLM API error: 503
      at G$ (/app/dist/index.js:787:10303)
```

### 修复措施

#### 1. 修复 agent.ts 代码 Bug

**文件**: `/data2/memoh-v2/agent/src/agent.ts`

**修改内容**:

修复前（有 Bug）:
```typescript
await close()

} catch (err) {
  console.error('[Agent stream] Error:', err)
  yield { type: 'text_delta', delta: `Error: ${err instanceof Error ? err.message : String(err)}` }
  yield { type: 'text_end', metadata: {} }
}

const { messages: strippedMessages } = stripAttachmentsFromMessages(
  result.messages,  // ❌ result 未定义！
)
const cleanedMessages = stripReasoningFromMessages(
  truncateMessagesForTransport(strippedMessages),
) as ModelMessage[]
yield {
  type: 'agent_end',
  messages: cleanedMessages,
  reasoning: result.reasoning,  // ❌ result 未定义！
  usage: normalizeUsage(result.usage),  // ❌ result 未定义！
  skills: getEnabledSkills(),
}
```

修复后:
```typescript
await close()

// Build final messages with assistant response
const assistantMessage: ModelMessage = {
  role: 'assistant',
  content,
}
const finalMessages = [...messages, assistantMessage] as ModelMessage[]

const { messages: strippedMessages } = stripAttachmentsFromMessages(finalMessages)
const cleanedMessages = stripReasoningFromMessages(
  truncateMessagesForTransport(strippedMessages),
) as ModelMessage[]

yield {
  type: 'agent_end',
  messages: cleanedMessages,
  reasoning: [],
  usage: { promptTokens: 0, completionTokens: 0, totalTokens: content.length },
  skills: getEnabledSkills(),
}
} catch (err) {
  console.error('[Agent stream] Error:', err)
  yield { type: 'text_delta', delta: `Error: ${err instanceof Error ? err.message : String(err)}` }
  yield { type: 'text_end', metadata: {} }

  // Even on error, yield agent_end with current messages
  const { messages: strippedMessages } = stripAttachmentsFromMessages(messages as ModelMessage[])
  const cleanedMessages = stripReasoningFromMessages(
    truncateMessagesForTransport(strippedMessages),
  ) as ModelMessage[]

  yield {
    type: 'agent_end',
    messages: cleanedMessages,
    reasoning: [],
    usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 },
    skills: getEnabledSkills(),
  }
}
```

**关键修复点**:
1. 在 `try` 块内正确构建 `assistantMessage` 和 `finalMessages`
2. 使用 LLM 返回的 `content` 变量替代未定义的 `result`
3. 在 `catch` 块中也添加 `agent_end` 事件，确保错误时也能正常结束
4. 添加详细的错误日志输出

#### 2. 更新 LLM Provider 配置

**数据库更新**:
```sql
-- 将 base_url 从 172.17.0.1 改为 host.docker.internal
UPDATE llm_providers
SET base_url = 'http://host.docker.internal:17099/v1'
WHERE name = 'local-qwen35-direct';
```

**配置变更**:
| 配置项 | 原值 | 新值 |
|--------|------|------|
| base_url | http://172.17.0.1:17099/v1 | http://host.docker.internal:17099/v1 |

#### 3. 更新 Docker Compose 配置

**文件**: `/data2/memoh-v2/docker-compose.yml`

**修改内容**:

```yaml
agent:
  build:
    context: .
    dockerfile: docker/Dockerfile.agent
    args:
      - TZ=${TZ:-UTC}
  container_name: memoh-agent
  environment:
    - HTTP_PROXY=http://ccd:88152353@10.40.31.69:10810
    - HTTPS_PROXY=http://ccd:88152353@10.40.31.69:10810
    - NO_PROXY=host.docker.internal,localhost,127.0.0.1,172.26.0.0/16,memoh-server,memoh-postgres,memoh-qdrant
  volumes:
    - ${MEMOH_CONFIG:-./docker/config/config.docker.toml}:/config.toml:ro
  ports:
    - "8081:8081"
  depends_on:
    - server
  restart: unless-stopped
  extra_hosts:
    - "host.docker.internal:host-gateway"
  networks:
    - memoh-network
```

**关键配置说明**:
1. **`extra_hosts`**: 使 Linux Docker 支持 `host.docker.internal` 解析
2. **`NO_PROXY`**: 排除本地地址，防止代理转发本地请求
   - `host.docker.internal`: LLM 服务地址
   - `localhost`, `127.0.0.1`: 本地回环
   - `172.26.0.0/16`: Memoh 容器网络
   - `memoh-*`: 其他 Memoh 服务

#### 4. 重建并重启 Agent

```bash
# 1. 构建新镜像
cd /data2/memoh-v2 && docker compose build agent

# 2. 停止并删除旧容器
docker compose stop agent
docker compose rm -f agent

# 3. 启动新容器
docker compose up -d agent

# 4. 验证状态
docker ps | grep memoh-agent
docker logs memoh-agent --tail 20
```

---

## 当前架构

### 请求链路（最终修复后）
```
Memoh Server → Memoh Agent → host.docker.internal:17099 → llama-server
                    ↓
              数据库读取配置
```

### 服务地址
| 服务 | 地址 | 说明 |
|------|------|------|
| llama-qwen35 | http://host.docker.internal:17099 | 主 LLM 服务 |
| llama-embedding | http://host.docker.internal:8089 | Embedding 服务 |
| memoh-server | http://172.26.0.1:8080 | Memoh API |
| memoh-agent | http://172.26.0.1:8081 | Agent Gateway |

### 网络架构
```
┌─────────────────────────────────────────────────────────────────┐
│                         Docker Host                              │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  memoh_memoh-network (172.26.0.0/16)                     │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │   │
│  │  │ memoh-server │  │ memoh-agent  │  │ memoh-postgres│   │   │
│  │  │ 172.26.0.x   │  │ 172.26.0.x   │  │ 172.26.0.x   │   │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘   │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                   │
│                              │ extra_hosts                        │
│                              ▼                                   │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  host.docker.internal → 172.17.0.1 (docker0 gateway)     │   │
│  └──────────────────────────────────────────────────────────┘   │
│                              │                                   │
│                              ▼                                   │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │  llama-server (host network)                             │   │
│  │  - 0.0.0.0:17099 (v1/chat/completions)                   │   │
│  │  - 0.0.0.0:8089 (embeddings)                             │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 验证步骤

### 1. 服务状态检查
```bash
# 检查 llama-server
curl http://localhost:17099/health

# 检查 Memoh
curl http://localhost:8080/health

# 检查 agent
curl http://localhost:8081/health

# 从 agent 容器测试 LLM 连通性
docker exec memoh-agent wget -qO- http://host.docker.internal:17099/v1/models
```

### 2. 网络连通性验证
```bash
# 验证 host.docker.internal 解析
docker exec memoh-agent getent hosts host.docker.internal
# 预期输出: 172.17.0.1 host.docker.internal

# 验证 NO_PROXY 生效
docker exec memoh-agent env | grep -i proxy
# 预期输出应包含 NO_PROXY=host.docker.internal,...
```

### 3. 对话测试
- 在企业微信中向机器人发送消息
- 验证是否显示实际回复内容（而非"处理完成，请查看完整回复"）
- 验证工具调用是否正常工作
- 检查是否还有"处理过程中断"错误

### 4. 日志监控
```bash
# 实时监控 agent 日志
docker logs memoh-agent --tail 50 -f

# 预期看到的成功日志:
# [Agent stream] Built messages: X
# [Agent stream] Starting fetch to http://host.docker.internal:17099/v1
# [Agent stream] Fetch URL: http://host.docker.internal:17099/v1/chat/completions
# [Agent stream] LLM response received successfully
```

---

## 参考配置

### llama-server 服务
```
/etc/systemd/system/llama-qwen35.service
- 端口: 17099
- 模型: Qwen3.5-35B-A3B-UD-Q4_K_XL.gguf
- 参数: --jinja --chat-template-kwargs '{"enable_thinking": false}'
```

### 数据库模型配置
```sql
-- 查看当前配置
SELECT p.id, p.name, p.base_url, p.api_key, m.model_id
FROM llm_providers p
JOIN models m ON p.id = m.llm_provider_id
WHERE m.model_id = 'Qwen3.5-35B-A3B-UD-Q4_K_XL.gguf';
```

### Docker 网络配置
```bash
# 查看 memoh 网络
docker network inspect memoh_memoh-network

# 查看 agent 容器网络配置
docker inspect memoh-agent | grep -A 20 NetworkSettings
```

---

## 故障排除

### 问题1: Agent 无法连接到 LLM
**症状**: `LLM API error: 503`

**排查步骤**:
1. 验证 `host.docker.internal` 解析:
   ```bash
   docker exec memoh-agent getent hosts host.docker.internal
   ```
2. 验证 NO_PROXY 设置:
   ```bash
   docker exec memoh-agent env | grep NO_PROXY
   ```
3. 测试直连 LLM:
   ```bash
   docker exec memoh-agent wget -qO- http://host.docker.internal:17099/v1/models
   ```

### 问题2: API Key 认证失败
**症状**: `Unauthorized: Invalid API Key`

**排查步骤**:
1. 检查数据库中的 API key:
   ```sql
   SELECT api_key FROM llm_providers WHERE name = 'local-qwen35-direct';
   ```
2. 检查 llama-server 的 API key 文件:
   ```bash
   cat /root/.llama/api_keys
   ```
3. 确保两者一致（64 位十六进制字符串）

### 问题3: 回复内容为空
**症状**: "处理完成，请查看完整回复"

**排查步骤**:
1. 检查 agent 日志中是否有 JavaScript 错误
2. 验证 `agent.ts` 代码已正确修复（使用定义的变量）
3. 重新构建 agent 镜像:
   ```bash
   docker compose build agent
   docker compose stop agent && docker compose up -d agent
   ```

---

## 经验教训

### 1. Docker 网络隔离
- 不同 Docker 网络之间默认无法通信
- `host.docker.internal` 是跨网络访问宿主机的推荐方式
- Linux Docker 需要 `extra_hosts` 配置才能支持 `host.docker.internal`

### 2. HTTP 代理陷阱
- `HTTP_PROXY` 会影响所有 HTTP 请求，包括本地请求
- 必须使用 `NO_PROXY` 排除本地地址
- 代理问题表现为 503 错误，容易与服务器错误混淆

### 3. 代码审查
- 引用未定义变量是常见的 JavaScript/TypeScript 错误
- 编译时检查（如 `bun build`）可能无法捕获这类运行时错误
- 需要充分的日志记录来帮助诊断问题

---

## 第三部分：新增对话时长统计功能

### 功能概述
在每次对话的回复文本末尾自动添加耗时统计，让用户了解从发送消息到收到完整回复的总耗时。

### 实现原理

#### 数据流
1. **消息接收**: `InboundMessage.ReceivedAt` 记录消息最初接收时间
2. **数据传递**: 通过 `StreamOptions.ReceivedAt` 传递到 WeCom Stream
3. **时长计算**: 在 `OutboundStream` 中保存 `receivedAt`，在发送最终消息时计算耗时
4. **防重复**: 使用 `timingAppended` 标志确保时长统计只添加一次

#### 关键路径
```
InboundMessage → StreamOptions → OutboundStream → 最终消息附加时长统计
```

### 修改文件

#### 1. internal/channel/types.go
**修改**: 在 `StreamOptions` 结构中添加 `ReceivedAt` 字段

```go
type StreamOptions struct {
    Reply           *ReplyRef      `json:"reply,omitempty"`
    SourceMessageID string         `json:"source_message_id,omitempty"`
    Metadata        map[string]any `json:"metadata,omitempty"`
    ReceivedAt      time.Time      `json:"received_at,omitempty"` // 新增
}
```

#### 2. internal/channel/adapters/wecom/stream.go
**修改 1**: 在 `OutboundStream` 结构中添加字段

```go
type OutboundStream struct {
    // ... 现有字段 ...
    receivedAt     time.Time // 消息接收时间
    timingAppended bool      // 防止重复添加时长统计
}
```

**修改 2**: 更新 `NewOutboundStream` 函数签名

```go
func NewOutboundStream(adapter *Adapter, cfg channel.ChannelConfig,
    wsClient *WebSocketClient, reqID, chatID, userID, chatType string,
    isMentioned bool, streamID string, logger *slog.Logger,
    receivedAt time.Time, cmd ...string) *OutboundStream {
    // ...
    s := &OutboundStream{
        // ... 现有字段 ...
        receivedAt:     receivedAt,
        timingAppended: false,
    }
    // ...
}
```

**修改 3**: 在 `StreamEventFinal` 处理中添加时长统计（主要路径）

```go
case channel.StreamEventFinal:
    // ... 现有代码 ...

    // Append response time statistics to the final message (only once)
    if !s.receivedAt.IsZero() && !s.timingAppended {
        totalDuration := time.Since(s.receivedAt)
        durationStr := formatDuration(totalDuration)
        finalContent += fmt.Sprintf("\n\n---\n⏱️ 本次对话耗时: %s", durationStr)
        s.timingAppended = true
    }

    // ... 发送消息 ...
```

**修改 4**: 在 `Close` 方法中添加时长统计（备用路径）

```go
func (s *OutboundStream) Close(ctx context.Context) error {
    // ... 现有代码 ...

    // Append response time statistics to the final message (only once)
    if !s.receivedAt.IsZero() && !s.timingAppended {
        totalDuration := time.Since(s.receivedAt)
        durationStr := formatDuration(totalDuration)
        content += fmt.Sprintf("\n\n---\n⏱️ 本次对话耗时: %s", durationStr)
        s.timingAppended = true
    }

    // ... 发送消息 ...
}
```

**修改 5**: 添加 `formatDuration` 辅助函数

```go
func formatDuration(d time.Duration) string {
    if d < time.Second {
        return fmt.Sprintf("%dms", d.Milliseconds())
    }
    if d < time.Minute {
        return fmt.Sprintf("%.1fs", d.Seconds())
    }
    if d < time.Hour {
        seconds := int(d.Seconds()) % 60
        return fmt.Sprintf("%dm%ds", int(d.Minutes()), seconds)
    }
    hours := int(d.Hours())
    minutes := int(d.Minutes()) % 60
    seconds := int(d.Seconds()) % 60
    return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
}
```

#### 3. internal/channel/adapters/wecom/adapter.go
**修改**: 在 `OpenStream` 中传递 `ReceivedAt`

```go
func (a *Adapter) OpenStream(ctx context.Context, target string, opts channel.StreamOptions) (channel.OutboundStream, error) {
    // ... 现有代码 ...
    return NewOutboundStream(a, cfg, wsClient, reqID, chatID, userID, chatType,
        isMentioned, streamID, a.logger, opts.ReceivedAt, cmd), nil
}
```

#### 4. internal/channel/inbound/channel.go
**修改**: 在调用 `OpenStream` 时设置 `ReceivedAt`

```go
stream, err := sender.OpenStream(ctx, target, channel.StreamOptions{
    Reply:           replyRef,
    SourceMessageID: sourceMessageID,
    Metadata:        streamMetadata,
    ReceivedAt:      msg.ReceivedAt, // 新增
})
```

### 显示格式

在回复文本末尾自动添加：
```
这是机器人的回复内容...

---
⏱️ 本次对话耗时: 2.5s
```

### 时长格式化规则

| 时长范围 | 显示格式 | 示例 |
|---------|---------|------|
| < 1秒 | 毫秒 | 850ms |
| < 1分钟 | 秒 | 2.5s |
| < 1小时 | 分秒 | 1m23s |
| ≥ 1小时 | 时分秒 | 1h2m3s |

### 部署

```bash
# 1. 构建 server 镜像
cd /data2/memoh-v2 && docker compose build server

# 2. 重启 server 服务
docker compose stop server && docker compose up -d server

# 3. 验证
docker logs memoh-server --tail 20
```

### 注意事项

1. **兼容性**: 如果 `ReceivedAt` 为零值（未设置），则不添加时长统计
2. **防重复**: 使用 `timingAppended` 标志确保时长统计只添加一次，避免重复发送时的重复添加
3. **主要路径**: 大多数消息通过 `StreamEventFinal` 处理添加时长统计
4. **备用路径**: `Close()` 方法作为备用，确保所有消息都能添加时长统计

---

**记录人**: Claude Code
**创建时间**: 2026-03-16
**更新时间**: 2026-03-16
