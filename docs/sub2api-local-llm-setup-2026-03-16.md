# Sub2API 本地 LLM 服务配置文档

**日期**: 2026-03-16
**文档类型**: 运维配置记录
**相关人员**: Claude Code

---

## 1. 背景与目标

### 1.1 问题描述
- 本地部署的 LLaMA 模型服务（qwen35、embedding）需要通过 API 网关统一管理和访问
- Cherry Studio 等客户端需要能够自动发现本地模型
- 原 sub2API 只显示内置 Claude/GPT 模型，不显示本地模型

### 1.2 目标
- 配置 sub2API 作为本地 LLM 的 API 网关
- 使本地模型出现在 `/v1/models` 列表中
- 支持 Cherry Studio 等客户端连接使用

---

## 2. 架构说明

```
Cherry Studio/客户端
    │
    │ HTTP/28000
    ▼
┌─────────────┐
│   sub2API   │  ← API 网关、认证、路由
│  (Docker)   │
└──────┬──────┘
       │
       ├─────────────────┐
       │                 │
       ▼                 ▼
┌─────────────┐   ┌─────────────┐
│ llama-qwen35│   │llama-embed  │
│  :17099     │   │  :8089      │
└─────────────┘   └─────────────┘
```

---

## 3. 配置详情

### 3.1 代码修改

**修改文件**: `/tmp/sub2api/backend/internal/handler/gateway_handler.go`

**修改内容**: 在 `Models` 函数中添加从 Group 的 `ModelRouting` 获取模型的逻辑

```go
// 第 824-826 行添加：
// If no models from accounts, check group's model_routing
if len(availableModels) == 0 && apiKey != nil && apiKey.Group != nil && apiKey.Group.ModelRoutingEnabled && len(apiKey.Group.ModelRouting) > 0 {
    for modelID := range apiKey.Group.ModelRouting {
        availableModels = append(availableModels, modelID)
    }
}
```

**原因**: 原代码只从账户的 `model_mapping` 获取模型，本地 LLM 账户没有配置 mapping，导致返回默认模型列表。修改后优先检查 Group 的 `model_routing` 配置。

### 3.2 重新构建镜像

```bash
cd /tmp/sub2api
docker build -t sub2api:local-models -f Dockerfile .
```

**镜像标签**: `sub2api:local-models`

### 3.3 数据库配置

#### 3.3.1 创建 local-llama 分组

```sql
INSERT INTO groups (
    name,
    platform,
    status,
    model_routing,
    model_routing_enabled,
    supported_model_scopes
) VALUES (
    'local-llama',
    'openai',
    'active',
    '{"qwen3.5-35B-A3B": [1], "bge-m3": [2]}'::jsonb,
    true,
    '["openai"]'::jsonb
);
```

**字段说明**:
- `model_routing`: 模型名称 -> 账户 ID 数组的映射
- `model_routing_enabled`: 启用模型路由
- `[1]`: llama-qwen35 账户 ID
- `[2]`: llama-embedding 账户 ID

#### 3.3.2 关联上游账户

```sql
-- 将 llama-qwen35 (ID=1) 和 llama-embedding (ID=2) 关联到 local-llama 组 (ID=34)
INSERT INTO account_groups (account_id, group_id, priority)
VALUES
    (1, 34, 50),  -- llama-qwen35
    (2, 34, 50);  -- llama-embedding
```

#### 3.3.3 配置 API Key

```sql
-- 将 fishwood API Key 分配到 local-llama 组
UPDATE api_keys
SET group_id = 34, updated_at = NOW()
WHERE name = 'fishwood';
```

### 3.4 防火墙配置

```bash
# 放行 28000 端口给所有 IP
ufw allow 28000/tcp comment "Sub2API for all"
```

**原有规则限制**: 只允许 Docker 网络 (172.26.0.0/16) 访问，其他机器无法连接。

### 3.5 Docker Compose 更新

**文件**: `/data2/sub2api/docker-compose.yml`

```yaml
services:
  sub2api:
    image: sub2api:local-models  # 使用自定义镜像
    container_name: sub2api
    network_mode: host
    ports:
      - "28000:28000"
    # ... 其他配置
```

---

## 4. 访问信息

### 4.1 服务地址

| 项目 | 地址 |
|------|------|
| API 端点 | `http://10.62.239.13:28000/v1` |
| Web UI | `http://10.62.239.13:28000` |
| 健康检查 | `http://10.62.239.13:28000/health` |

### 4.2 认证信息

| 项目 | 值 |
|------|-----|
| Admin 账号 | `admin@sub2api.local` |
| Admin 密码 | `admin123456` |
| API Key (fishwood) | `sk-ba874ae23f943c4c2c8f4053cc51ba36f26f09dce59ea7125968b2461387e1eb` |

### 4.3 可用模型

| 模型 ID | 类型 | 后端服务 | 状态 |
|---------|------|---------|------|
| `qwen3.5-35B-A3B` | Chat | llama-qwen35 (:17099) | ✅ active |
| `bge-m3` | Embedding | llama-embedding (:8089) | ⚠️ error |

**注意**: bge-m3 是 Embedding 模型，主要用于生成文本向量。当前 embedding 服务状态为 error，可能需要检查服务状态。

---

## 5. 客户端配置

### 5.1 Cherry Studio

1. **打开设置** → Provider → 添加 OpenAI 兼容 Provider
2. **填写配置**:
   - 名称: `Sub2API Local`
   - Base URL: `http://10.62.239.13:28000/v1`
   - API Key: `sk-ba8...87e1eb`
3. **获取模型列表**: 点击「获取模型列表」按钮
4. **选择模型**: 选择 `qwen3.5-35B-A3B`

### 5.2 OpenAI 客户端示例

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://10.62.239.13:28000/v1",
    api_key="sk-ba874ae23f943c4c2c8f4053cc51ba36f26f09dce59ea7125968b2461387e1eb"
)

response = client.chat.completions.create(
    model="qwen3.5-35B-A3B",
    messages=[{"role": "user", "content": "你好"}]
)
print(response.choices[0].message.content)
```

### 5.3 curl 测试

```bash
# 获取模型列表
curl http://10.62.239.13:28000/v1/models \
  -H "Authorization: Bearer sk-ba8...87e1eb"

# 聊天 completion
curl http://10.62.239.13:28000/v1/chat/completions \
  -H "Authorization: Bearer sk-ba8...87e1eb" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3.5-35B-A3B",
    "messages": [{"role": "user", "content": "你好"}]
  }'
```

---

## 6. 文件位置

| 类型 | 路径 |
|------|------|
| Sub2API 源码 | `/tmp/sub2api/` |
| Sub2API 数据 | `/data2/sub2api/` |
| Docker Compose | `/data2/sub2api/docker-compose.yml` |
| 环境变量 | `/data2/sub2api/.env` |
| 模型定价配置 | `/data2/sub2api/data/model_pricing.json` |
| PostgreSQL 数据 | `/data2/sub2api/postgres_data/` |
| Redis 数据 | `/data2/sub2api/redis_data/` |

---

## 7. 常用命令

```bash
# 查看服务状态
cd /data2/sub2api && docker compose ps

# 重启服务
cd /data2/sub2api && docker compose restart

# 查看日志
docker logs sub2api --tail 50

# 健康检查
curl http://localhost:28000/health

# 检查模型列表
curl http://localhost:28000/v1/models \
  -H "Authorization: Bearer sk-ba8...87e1eb"

# 数据库查询
docker exec -i sub2api-postgres psql -U sub2api -d sub2api -c "
  SELECT id, name, model_routing FROM groups WHERE name = 'local-llama';
"
```

---

## 8. 故障排查

### 8.1 连接失败

**现象**: Cherry Studio 无法连接 `http://10.62.239.13:28000`

**检查**:
1. 防火墙是否放行 28000 端口: `ufw status | grep 28000`
2. 服务是否运行: `docker ps | grep sub2api`
3. 端口监听: `ss -tlnp | grep 28000`

### 8.2 模型列表不显示本地模型

**检查**:
1. API Key 是否分配到 local-llama 组
2. Group 的 model_routing 是否配置正确
3. 重启服务清除缓存: `docker restart sub2api`

### 8.3 调用失败

**检查日志**:
```bash
docker logs sub2api --tail 100 | grep error
```

---

## 9. 相关文档

- **本文档**: `/data2/memoh-v2/docs/sub2api-local-llm-setup-2026-03-16.md`
- **安全变更通知**: `/data2/memoh-v2/docs/SECURITY_CHANGE_NOTICE_2026-03-15.md`
- **LLaMA 服务管理**: `/root/.claude/projects/-root/memory/llama-server-management.md`

---

**最后更新**: 2026-03-16
**维护人员**: Claude Code
