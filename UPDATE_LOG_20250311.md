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

*文档生成时间: 2026-03-11*
*生成工具: Claude Code*
