# Memoh 升级与维护操作手册

**版本**: v2.x
**更新时间**: 2026-03-16
**适用场景**: Docker Compose 部署环境

---

## 目录

1. [快速修复指南](#快速修复指南)
2. [Agent 服务重建](#agent-服务重建)
3. [LLM Provider 配置更新](#llm-provider-配置更新)
4. [网络问题排查](#网络问题排查)
5. [常见问题速查](#常见问题速查)

---

## 快速修复指南

### 场景1: 机器人只回复"处理完成，请查看完整回复"

**症状**: 对话无实际内容返回

**快速修复步骤**:

```bash
# 1. 进入项目目录
cd /data2/memoh-v2

# 2. 更新 LLM provider 配置（使用 host.docker.internal）
docker exec -e PGPASSWORD=memoh123 memoh-postgres psql -U memoh -d memoh -c "
UPDATE llm_providers
SET base_url = 'http://host.docker.internal:17099/v1'
WHERE name = 'local-qwen35-direct';
"

# 3. 重建并重启 agent
docker compose build agent
docker compose stop agent && docker compose rm -f agent
docker compose up -d agent

# 4. 验证修复
docker logs memoh-agent --tail 20
```

---

### 场景2: "处理过程中断，请重试"错误

**症状**: 对话中断，出现错误提示

**快速修复步骤**:

```bash
# 1. 检查 llama-server 状态
systemctl is-active llama-qwen35
curl -s http://localhost:17099/v1/models | head -5

# 2. 检查网络连通性
docker exec memoh-agent wget -qO- --timeout=5 http://host.docker.internal:17099/v1/models

# 3. 如网络不通，更新配置并重启
# 参考场景1的步骤

# 4. 重启服务
docker compose restart server agent
```

---

### 场景3: API Key 认证失败

**症状**: 日志中出现 `Unauthorized: Invalid API Key`

**快速修复步骤**:

```bash
# 1. 获取正确的 API key
cat /root/.llama/api_keys

# 2. 更新数据库
docker exec -e PGPASSWORD=memoh123 memoh-postgres psql -U memoh -d memoh -c "
UPDATE llm_providers
SET api_key = 'YOUR_API_KEY_HERE'
WHERE name = 'local-qwen35-direct';
"

# 3. 重启服务
docker compose restart server agent
```

---

## Agent 服务重建

### 完整重建流程

当 agent 代码有重大更新时，需要完整重建：

```bash
# 1. 进入项目目录
cd /data2/memoh-v2

# 2. 备份当前状态（可选）
docker logs memoh-agent > /tmp/agent-backup-$(date +%Y%m%d-%H%M%S).log 2>&1

# 3. 停止并删除旧容器
docker compose stop agent
docker compose rm -f agent

# 4. 清理旧镜像（可选，释放空间）
docker images | grep memoh-agent
docker image prune -f

# 5. 构建新镜像
docker compose build agent

# 6. 启动新容器
docker compose up -d agent

# 7. 验证状态
docker ps | grep memoh-agent
docker logs memoh-agent --tail 30
```

### 热更新（仅配置变更）

当只有环境变量或配置变更时，无需重建镜像：

```bash
# 1. 停止容器
docker compose stop agent

# 2. 重新创建容器（使用新配置）
docker compose up -d agent

# 3. 验证
docker logs memoh-agent --tail 20
```

---

## LLM Provider 配置更新

### 查看当前配置

```bash
# 查看所有 provider
docker exec -e PGPASSWORD=memoh123 memoh-postgres psql -U memoh -d memoh -c "
SELECT id, name, base_url, substr(api_key, 1, 20) || '...' as api_key_preview
FROM llm_providers
ORDER BY name;
"
```

### 更新 Base URL

```bash
# 更新为使用 host.docker.internal
docker exec -e PGPASSWORD=memoh123 memoh-postgres psql -U memoh -d memoh -c "
UPDATE llm_providers
SET base_url = 'http://host.docker.internal:17099/v1'
WHERE name = 'local-qwen35-direct';
"

# 或使用 IP 地址（如果 host.docker.internal 不可用）
docker exec -e PGPASSWORD=memoh123 memoh-postgres psql -U memoh -d memoh -c "
UPDATE llm_providers
SET base_url = 'http://172.17.0.1:17099/v1'
WHERE name = 'local-qwen35-direct';
"
```

### 更新 API Key

```bash
# 设置新的 API key
docker exec -e PGPASSWORD=memoh123 memoh-postgres psql -U memoh -d memoh -c "
UPDATE llm_providers
SET api_key = 'YOUR_NEW_API_KEY'
WHERE name = 'local-qwen35-direct';
"
```

---

## 网络问题排查

### 诊断流程图

```
机器人无回复
    │
    ├─► 检查 agent 日志
    │   docker logs memoh-agent --tail 50
    │
    ├─► 看到 "LLM API error: 503"
    │   │
    │   ├─► 检查代理设置
    │   │   docker exec memoh-agent env | grep -i proxy
    │   │
    │   └─► 检查 NO_PROXY 是否包含 host.docker.internal
    │
    ├─► 看到 "Cannot connect to host.docker.internal"
    │   │
    │   ├─► 检查 extra_hosts 配置
    │   │   docker inspect memoh-agent | grep -A 5 ExtraHosts
    │   │
    │   └─► 检查 host.docker.internal 解析
    │       docker exec memoh-agent getent hosts host.docker.internal
    │
    └─► 看到 "result is not defined" 或类似 JS 错误
        │
        └─► 需要重建 agent 镜像
            docker compose build agent
```

### 网络连通性测试

```bash
# 1. 测试从 agent 容器到 LLM 的连通性
docker exec memoh-agent wget -qO- http://host.docker.internal:17099/v1/models

# 2. 测试数据库连接
docker exec memoh-agent wget -qO- --timeout=5 http://memoh-postgres:5432 2>&1 | head -5

# 3. 检查容器网络配置
docker inspect memoh-agent | grep -A 30 NetworkSettings

# 4. 查看 memoh 网络
docker network inspect memoh_memoh-network
```

### 代理问题排查

```bash
# 1. 检查代理环境变量
docker exec memoh-agent env | grep -i proxy

# 预期输出:
# HTTP_PROXY=http://ccd:88152353@10.40.31.69:10810
# HTTPS_PROXY=http://ccd:88152353@10.40.31.69:10810
# NO_PROXY=host.docker.internal,localhost,127.0.0.1,172.26.0.0/16,...

# 2. 如果 NO_PROXY 不存在，需要更新 docker-compose.yml
# 添加:
# environment:
#   - NO_PROXY=host.docker.internal,localhost,127.0.0.1,172.26.0.0/16

# 3. 重启 agent 应用配置
docker compose stop agent && docker compose up -d agent
```

---

## 常见问题速查

### Q1: 如何查看 agent 实时日志？

```bash
# 实时跟踪日志
docker logs memoh-agent --tail 50 -f

# 查看特定时间段的日志
docker logs memoh-agent --since "10m"

# 导出日志到文件
docker logs memoh-agent > /tmp/agent-$(date +%Y%m%d-%H%M%S).log 2>&1
```

### Q2: 如何验证 LLM 服务正常？

```bash
# 测试 llama-server
curl -s http://localhost:17099/v1/models | head -20

# 测试 chat completions
curl -s http://localhost:17099/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $(cat /root/.llama/api_keys)" \
  -d '{
    "model": "Qwen3.5-35B-A3B-UD-Q4_K_XL.gguf",
    "messages": [{"role": "user", "content": "hello"}]
  }' | head -50
```

### Q3: 如何回滚 agent 到之前版本？

```bash
# 1. 查看历史镜像
docker images | grep memoh-agent

# 2. 使用特定镜像标签回滚（如果有）
docker compose stop agent
docker compose rm -f agent
docker run -d --name memoh-agent OLD_IMAGE_TAG

# 3. 或者从 git 历史恢复代码
git log --oneline -10
git checkout COMMIT_HASH -- agent/src/
docker compose build agent
docker compose up -d agent
```

### Q4: 如何添加新的 LLM Provider？

```bash
# 1. 插入新 provider
docker exec -e PGPASSWORD=memoh123 memoh-postgres psql -U memoh -d memoh -c "
INSERT INTO llm_providers (id, name, base_url, api_key, created_at, updated_at)
VALUES (
  gen_random_uuid(),
  'new-provider',
  'http://host.docker.internal:NEW_PORT/v1',
  'YOUR_API_KEY',
  NOW(),
  NOW()
);
"

# 2. 在 Web 界面中为新 provider 创建模型配置
```

### Q5: 如何清理 Docker 空间？

```bash
# 清理未使用的容器
docker container prune -f

# 清理未使用的镜像
docker image prune -f

# 清理构建缓存
docker builder prune -f

# 全面清理（谨慎使用）
docker system prune -f
```

### Q6: 服务启动顺序是什么？

```bash
# 正确启动顺序

# 1. 首先确保 llama-server 运行
systemctl is-active llama-qwen35 || systemctl start llama-qwen35

# 2. 启动 Memoh 基础服务
docker compose up -d postgres qdrant

# 3. 等待数据库就绪（约 10-30 秒）
sleep 20

# 4. 启动 server
docker compose up -d server

# 5. 等待 server 就绪
sleep 10

# 6. 启动 agent
docker compose up -d agent

# 7. 启动 web
docker compose up -d web

# 或者一键启动（使用 depends_on）
docker compose up -d
```

---

## Server 服务重建（含新功能部署）

### 场景4: 部署对话时长统计功能

**功能描述**: 在每次对话的回复末尾自动添加耗时统计

**部署步骤**:

```bash
# 1. 进入项目目录
cd /data2/memoh-v2

# 2. 拉取最新代码（如从 git 更新）
# git pull origin main

# 3. 构建新镜像
docker compose build server

# 4. 停止并删除旧容器
docker compose stop server
docker compose rm -f server

# 5. 启动新容器
docker compose up -d server

# 6. 验证状态
docker ps | grep memoh-server
docker logs memoh-server --tail 30
```

**验证功能**:

向机器人发送测试消息，检查回复末尾是否包含时长统计：
```
这是机器人的回复内容...

---
⏱️ 本次对话耗时: 2.5s
```

**相关文件**:
- `internal/channel/types.go` - StreamOptions 添加 ReceivedAt
- `internal/channel/adapters/wecom/stream.go` - 时长统计逻辑
- `internal/channel/adapters/wecom/adapter.go` - 传递 ReceivedAt
- `internal/channel/inbound/channel.go` - 设置 ReceivedAt

---

## 附录

### A. 关键文件路径

| 文件 | 说明 |
|------|------|
| `/data2/memoh-v2/docker-compose.yml` | Docker Compose 主配置 |
| `/data2/memoh-v2/agent/src/agent.ts` | Agent 核心代码 |
| `/root/.llama/api_keys` | LLM API Key 文件 |
| `/etc/systemd/system/llama-qwen35.service` | LLM 服务配置 |
| `/data2/memoh-v2/docs/fix-conversation-interruption-2026-03-16.md` | 详细修复文档 |

### B. 常用命令速查表

```bash
# 服务管理
docker compose ps                    # 查看服务状态
docker compose logs -f agent         # 查看 agent 日志
docker compose restart agent         # 重启 agent
docker compose stop agent            # 停止 agent
docker compose up -d agent           # 启动 agent

# 数据库操作
docker exec memoh-postgres psql -U memoh -d memoh -c "SELECT * FROM llm_providers;"

# 网络测试
docker exec memoh-agent wget -qO- http://host.docker.internal:17099/v1/models
curl http://localhost:17099/v1/models

# 资源监控
docker stats memoh-agent
docker system df
```

### C. 相关文档

- [对话中断修复详细记录](./fix-conversation-interruption-2026-03-16.md)
- [CHANGELOG](../CHANGELOG.md)
- [README](../README.md)

---

**维护人员**: Claude Code
**最后更新**: 2026-03-16
