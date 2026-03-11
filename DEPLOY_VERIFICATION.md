# 部署验证报告

**部署时间**: 2026-03-10
**部署版本**: a36ad55d (sync: 移植上游高优先级功能 + dumb-init 修复)

---

## 服务状态

| 服务 | 容器名 | 状态 | 端口 |
|------|--------|------|------|
| PostgreSQL | memoh-postgres | ✅ Healthy | 5432 |
| Qdrant | memoh-qdrant | ✅ Healthy | 6333-6334 |
| SearXNG | memoh-searxng | ✅ Healthy | 8080 |
| Containerd | memoh-containerd | ✅ Healthy | - |
| Server | memoh-server | ✅ Healthy | 8080 |
| Agent | memoh-agent | ✅ Healthy | 8081 |
| Web | memoh-web | ✅ Healthy | 8082 |

---

## 已部署的更新

### 1. MCP 僵尸进程修复 ✅

**涉及文件**: 
- `docker/Dockerfile.mcp`
- `docker/Dockerfile.containerd`

**变更**:
- 安装 `dumb-init` 包
- 使用 `dumb-init` 作为 PID 1 来正确回收僵尸进程

**验证方法**:
```bash
# 启动新的 Bot 容器后，检查进程树
docker exec memoh-containerd ctr tasks ls
```

### 2. 前端优化 ✅

**涉及文件**:
- `packages/web/src/router.ts`
- `packages/web/vite.config.ts`

**变更**:
- 添加 chunk load error 自动刷新
- 添加 Vite optimizeDeps 配置

**访问地址**: http://localhost:8082

---

## 访问信息

| 端点 | 地址 | 说明 |
|------|------|------|
| Web UI | http://localhost:8082 | 管理界面 |
| API | http://localhost:8080 | REST API |
| Agent | http://localhost:8081 | AI Agent Gateway |

---

## 验证步骤

### 1. 基础功能测试

```bash
# 检查所有服务健康
docker compose ps

# 测试 API 健康
curl http://localhost:8080/healthz

# 测试 Web 界面
curl http://localhost:8082/
```

### 2. 企业微信适配器测试

1. 登录 Web UI: http://localhost:8082
2. 进入 Bot 设置 → 频道
3. 配置企业微信
4. 发送测试消息

### 3. MCP 僵尸进程修复验证

创建一个新 Bot 并执行命令，然后检查容器中是否有僵尸进程：

```bash
# 进入 containerd 容器
docker exec -it memoh-containerd sh

# 列出所有任务
ctr tasks ls

# 检查特定容器的进程（替换 <container-id>）
ctr task ps <container-id>
```

---

## 回滚方案

如需回滚到部署前版本：

```bash
# 停止服务
docker compose down

# 回滚代码
git log --oneline -5  # 查看历史
git reset --hard 30e89428  # 回滚到部署前

# 重新启动
docker compose up -d
```

---

## 监控和日志

```bash
# 查看所有服务日志
docker compose logs -f

# 查看特定服务日志
docker compose logs -f server
docker compose logs -f agent
docker compose logs -f web
```

---

**部署状态**: ✅ 成功
