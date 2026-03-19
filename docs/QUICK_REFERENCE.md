# Memoh 快速参考卡片

**关键操作一键复制**

---

## 🚨 紧急修复

### 机器人无回复内容
```bash
cd /data2/memoh-v2
docker compose build agent
docker compose stop agent && docker compose rm -f agent
docker compose up -d agent
docker logs memoh-agent --tail 20
```

### 对话中断错误
```bash
cd /data2/memoh-v2
docker compose restart server agent
docker logs memoh-agent --tail 50
```

---

## 🔍 诊断命令

### 查看 Agent 日志
```bash
docker logs memoh-agent --tail 50 -f
```

### 测试 LLM 连通性
```bash
# 从宿主机
curl http://localhost:17099/v1/models

# 从容器
docker exec memoh-agent wget -qO- http://host.docker.internal:17099/v1/models
```

### 检查代理设置
```bash
docker exec memoh-agent env | grep -i proxy
```

### 查看数据库配置
```bash
docker exec -e PGPASSWORD=memoh123 memoh-postgres psql -U memoh -d memoh -c "SELECT name, base_url FROM llm_providers;"
```

---

## 🔧 配置更新

### 更新 LLM Base URL
```bash
docker exec -e PGPASSWORD=memoh123 memoh-postgres psql -U memoh -d memoh -c "UPDATE llm_providers SET base_url = 'http://host.docker.internal:17099/v1' WHERE name = 'local-qwen35-direct';"
```

### 重启所有服务
```bash
cd /data2/memoh-v2 && docker compose restart
```

---

## 📁 关键文件

| 文件 | 路径 |
|------|------|
| Docker Compose | `/data2/memoh-v2/docker-compose.yml` |
| Agent 代码 | `/data2/memoh-v2/agent/src/agent.ts` |
| API Key | `/root/.llama/api_keys` |
| 修复文档 | `/data2/memoh-v2/docs/fix-conversation-interruption-2026-03-16.md` |
| 升级手册 | `/data2/memoh-v2/docs/UPGRADE_GUIDE.md` |

---

## 🌐 服务地址

| 服务 | 地址 |
|------|------|
| llama-server | `http://host.docker.internal:17099` |
| memoh-server | `http://localhost:8080` |
| memoh-agent | `http://localhost:8081` |
| memoh-web | `http://localhost:8082` |

---

## 📞 故障速查

| 症状 | 可能原因 | 解决方案 |
|------|----------|----------|
| "处理完成，请查看完整回复" | 代码 Bug / 网络隔离 | 重建 agent + 更新 base_url |
| "处理过程中断，请重试" | LLM 服务异常 / 网络不通 | 检查 llama-server + 网络 |
| "LLM API error: 503" | HTTP 代理问题 | 检查 NO_PROXY 配置 |
| "Unauthorized: Invalid API Key" | API Key 不匹配 | 更新数据库中的 api_key |

---

**打印此页面，贴在显示器旁备用**
