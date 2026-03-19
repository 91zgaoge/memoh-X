# Memoh 添加 Kimi Code 模型支持修复记录

**日期**: 2026-03-19
**问题**: Memoh 项目连接 Kimi Code 模型受到客户端限制，无法添加和使用 kimi-coding 类型的 Provider

---

## 问题分析

### 现象
1. 前端添加 Kimi Code Provider 后没有显示在列表中
2. 导入模型时返回 404 错误：`{"message":"fetch models: unexpected status: 404"}`
3. 部分组件已支持 `kimi-coding`，但多处配置缺失导致功能不完整

### 根本原因
`kimi-coding` 客户端类型在部分组件中已实现，但在以下关键位置缺失：

1. **前端配置缺失**: `providerCatalog` 中无 `kimi-coding` 配置
2. **前端类型过滤缺失**: 硬编码的 `CLIENT_TYPES` 数组未包含 `kimi-coding`
3. **SDK 类型定义缺失**: `ProvidersClientType` 联合类型未包含 `kimi-coding`
4. **后端验证缺失**: `isValidClientType()` 函数未包含 `kimi-coding`
5. **CLI 选项缺失**: provider 创建命令的 choices 数组未包含 `kimi-coding`
6. **Agent 安全提供商缺失**: `SYSTEM_SAFE_PROVIDERS` 未包含 `kimi-coding`

---

## 修复措施

### 1. 前端配置 (/data2/memoh-v2/packages/web/src/data/model-catalog.ts)

在 `providerCatalog` 中添加 `kimi-coding` 配置：

```typescript
'kimi-coding': {
  label: 'Kimi Code',
  defaultBaseUrl: 'https://api.moonshot.cn/v1',
  models: [
    { modelId: 'kimi-k2.5', name: 'Kimi K2.5', contextWindow: 256000, maxTokens: 8192, reasoning: false, isMultimodal: false },
    { modelId: 'kimi-k2', name: 'Kimi K2', contextWindow: 256000, maxTokens: 8192, reasoning: false, isMultimodal: false },
  ],
},
```

### 2. 前端类型列表 (/data2/memoh-v2/packages/web/src/pages/models/index.vue)

扩展硬编码的 `CLIENT_TYPES` 数组：

```typescript
const CLIENT_TYPES: ProvidersClientType[] = [
  'openai', 'openai-compat', 'anthropic', 'google',
  'azure', 'bedrock', 'mistral', 'xai', 'ollama', 'dashscope',
  'deepseek', 'zai-global', 'zai-cn', 'zai-coding-global', 'zai-coding-cn',
  'minimax-global', 'minimax-cn', 'moonshot-global', 'moonshot-cn',
  'volcengine', 'volcengine-coding', 'qianfan', 'groq', 'openrouter',
  'together', 'fireworks', 'perplexity', 'zhipu', 'siliconflow', 'nvidia',
  'bailing', 'xiaomi', 'longcat', 'modelscope', 'kimi-coding',
]
```

### 3. SDK 类型定义 (/data2/memoh-v2/packages/sdk/src/types.gen.ts)

更新 `ProvidersClientType` 联合类型：

```typescript
export type ProvidersClientType = 'openai' | 'openai-compat' | ... | 'kimi-coding';
```

### 4. 后端验证 (/data2/memoh-v2/internal/providers/service.go)

在 `isValidClientType()` 函数中添加缺失的类型：

```go
case ClientTypeZhipu, ClientTypeSiliconflow, ClientTypeNvidia, ClientTypeBailing,
    ClientTypeXiaomi, ClientTypeLongcat, ClientTypeModelScope, ClientTypeKimiCoding:
    return true
```

### 5. CLI 选项 (/data2/memoh-v2/packages/cli/src/cli/index.ts)

更新 provider 创建命令的 choices 数组。

### 6. Agent 安全提供商 (/data2/memoh-v2/agent/src/types/model.ts)

在 `SYSTEM_SAFE_PROVIDERS` 中添加 `kimi-coding`。

---

## 关键发现

### Kimi API 的模型导入
Kimi API 的 `/v1/models` 端点在 Base URL 不含 `/v1` 路径时会返回 404。

**解决方案**: 配置 Provider 时，Base URL 需要包含 `/v1`，例如：
- ✅ `https://api.moonshot.cn/v1`
- ❌ `https://api.moonshot.cn` (会返回 404)

---

## 修改文件清单

| 文件路径 | 修改内容 |
|---------|---------|
| `/data2/memoh-v2/packages/web/src/data/model-catalog.ts` | 添加 `kimi-coding` provider 配置 |
| `/data2/memoh-v2/packages/web/src/pages/models/index.vue` | 扩展 `CLIENT_TYPES` 数组 |
| `/data2/memoh-v2/packages/sdk/src/types.gen.ts` | 更新 `ProvidersClientType` 类型 |
| `/data2/memoh-v2/packages/cli/src/cli/index.ts` | 更新 CLI choices 数组 |
| `/data2/memoh-v2/internal/providers/service.go` | 更新 `isValidClientType()` 函数 |
| `/data2/memoh-v2/agent/src/types/model.ts` | 更新 `SYSTEM_SAFE_PROVIDERS` |

---

## 验证步骤

1. 在 Memoh UI 中点击 "Add Provider"
2. Client Type 选择 "Kimi Code"
3. Base URL 填写 `https://api.moonshot.cn/v1`
4. 填写 API Key 并提交
5. 在 Provider 列表中应该能看到新添加的 Kimi Code
6. 点击 "Import Models" 应能成功导入模型列表

---

## 后续维护建议

添加新的 LLM Provider 客户端类型时，需要同步更新以下位置：

1. **前端配置**: `packages/web/src/data/model-catalog.ts`
2. **前端类型列表**: `packages/web/src/pages/models/index.vue` 的 `CLIENT_TYPES`
3. **SDK 类型**: `packages/sdk/src/types.gen.ts` 的 `ProvidersClientType`
4. **后端验证**: `internal/providers/service.go` 的 `isValidClientType()`
5. **CLI 选项**: `packages/cli/src/cli/index.ts` 的 choices 数组
6. **数据库约束**: 如需新的枚举值，创建迁移文件
7. **Agent 支持**: `agent/src/types/model.ts` 和 `agent/src/model.ts`
