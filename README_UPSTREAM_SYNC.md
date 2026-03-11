# Memoh-v2 上游代码同步方案

> 本方案解决如何在保留二次开发功能（企业微信适配器等）的同时，同步上游 https://github.com/memohai/Memoh 的最新更新。

---

## 结论先行

✅ **完全可以做到！** 但需要采用**功能级移植**策略，而非直接合并。

---

## 关键发现

### 1. 架构差异

| 组件 | 上游 (memohai/Memoh) | 我们 (Kxiandaoyan/Memoh-v2) | 差异程度 |
|------|---------------------|---------------------------|---------|
| Agent Gateway | `apps/agent/` | `agent/` | 轻微 |
| Browser 服务 | `apps/browser/` | **不存在** | 新增 |
| Web 前端 | `apps/web/` | `packages/web/` | 中等 |
| 目录结构 | `apps/` + `packages/` | `agent/` + `packages/` | 中等 |

### 2. 历史无关

两个仓库的 Git 历史**没有共同祖先**，意味着当前 Fork 是基于原代码重新创建的，而非直接 Fork。这导致：
- `git merge` 会产生 288+ 文件冲突
- 必须采用**选择性合并**策略

### 3. 二次开发功能现状

| 功能 | 位置 | 状态 |
|------|------|------|
| 企业微信适配器 | `internal/channel/adapters/wecom/` | ✅ 独有，安全 |
| 文件解析增强 | `internal/fileparse/` | ✅ 独有，安全 |
| 文档生成技能 | `internal/skills/defaults/` | ✅ 独有，安全 |
| 模型扩展 | `internal/models/` | ⚠️ 需要保护 |

---

## 推荐方案：功能级移植

### 为什么不直接合并？

```
直接合并 upstream/main
    ↓
288+ 文件冲突
    ↓
需要手动解决每个冲突
    ↓
极易覆盖企业微信适配器等核心功能
    ↓
❌ 高风险，不推荐
```

### 功能级移植流程

```
1. 查看上游提交历史
    ↓
2. 选择需要的功能
    ↓
3. 提取该功能的补丁
    ↓
4. 手动移植到我们的代码（考虑路径差异）
    ↓
5. 测试验证
    ↓
6. ✅ 安全获取新功能，保留二次开发
```

---

## 工具与脚本

已为您创建以下工具：

### 1. 功能提取脚本

```bash
# 提取特定上游功能
./scripts/extract-upstream-feature.sh "COMMIT_RANGE" "feature-name"

# 示例：提取 MCP 僵尸进程修复
./scripts/extract-upstream-feature.sh "8ce5243e^..8ce5243e" "mcp-zombie-fix"
```

**输出**:
- `upstream-patches/mcp-zombie-fix-YYYYMMDD.patch` - 补丁文件
- `upstream-patches/mcp-zombie-fix-YYYYMMDD-README.md` - 移植指南

### 2. 同步分析脚本

```bash
# 运行同步分析（保护二次开发功能）
./scripts/sync-upstream.sh

# 试运行（不实际修改）
./scripts/sync-upstream.sh --dry-run
```

**功能**:
- 自动识别上游独有文件（安全合并）
- 自动识别并保留我们的关键二次开发文件
- 生成详细的同步报告和移植任务清单

---

## 快速开始

### 步骤 1: 查看上游更新

```bash
git fetch upstream
git log upstream/main --oneline -20
```

### 步骤 2: 提取并移植功能

以 **MCP 僵尸进程修复** 为例：

```bash
# 1. 提取功能
./scripts/extract-upstream-feature.sh "8ce5243e^..8ce5243e" "mcp-zombie-fix"

# 2. 查看补丁（理解变更内容）
cat upstream-patches/mcp-zombie-fix-20260310.patch

# 3. 手动应用到我们的代码
# 编辑 docker/Dockerfile.mcp
# - 添加 dumb-init 安装
# - 修改 ENTRYPOINT

# 4. 测试
./scripts/check_memoh_health.sh
```

### 步骤 3: 验证企业微信适配器

```bash
# 确保以下目录仍然存在且内容正确
ls internal/channel/adapters/wecom/
```

---

## 上游新功能优先级

### 🔴 高优先级（推荐立即移植）

| 功能 | 提交 | 文件数 | 难度 |
|------|------|--------|------|
| MCP 僵尸进程修复 | `8ce5243e` | 1 | ⭐ 简单 |
| 聊天图片冻结修复 | `ef7ed961` | ~5 | ⭐⭐ 中等 |
| 消息中止功能 | `23d49a1c` | ~10 | ⭐⭐ 中等 |

### 🟡 中优先级（可选移植）

| 功能 | 提交 | 说明 |
|------|------|------|
| Gmail OAuth2 | `a5c36491` | 邮件功能增强 |
| Discord 修复 | `a2cb5939` | 如使用 Discord |
| QQ 频道 | 文档 | 配置指南 |

### 🟢 低优先级（暂缓）

| 功能 | 说明 |
|------|------|
| 独立 Browser 服务 | 架构变化大，当前功能可用 |

---

## 文件保护清单

以下文件/目录**在同步时必须保留我们的版本**：

```
# 核心二次开发
internal/channel/adapters/wecom/        # 企业微信适配器
internal/fileparse/                     # 文件解析增强
internal/skills/defaults/docx/          # Word 生成技能
internal/skills/defaults/xlsx/          # Excel 生成技能
internal/skills/defaults/pptx/          # PPT 生成技能
internal/skills/defaults/pdf/           # PDF 处理技能

# 修改过的核心文件
cmd/agent/main.go                       # 适配器注册
internal/channel/inbound/channel.go     # 通道处理
internal/conversation/flow/resolver.go  # 对话解析
internal/handlers/models.go             # 模型处理
internal/models/models.go               # 模型定义
internal/models/types.go                # 类型定义

# 前端修改
packages/web/src/data/model-catalog.ts  # 模型目录

# Docker 配置
docker-compose.yml                      # 代理等配置
```

---

## 文档导航

| 文档 | 内容 |
|------|------|
| `UPSTREAM_SYNC_QUICKSTART.md` | 快速入门指南 |
| `SYNC_UPSTREAM_STRATEGY.md` | 详细策略分析 |
| `upstream-patches/*.patch` | 提取的补丁文件 |
| `upstream-patches/*-README.md` | 各功能的移植指南 |

---

## 示例：移植 MCP 僵尸进程修复

### 1. 提取功能

```bash
./scripts/extract-upstream-feature.sh "8ce5243e^..8ce5243e" "mcp-zombie-fix"
```

### 2. 查看补丁

```diff
diff --git a/docker/Dockerfile.mcp b/docker/Dockerfile.mcp
index e6b1c337..770344d4 100644
--- a/docker/Dockerfile.mcp
+++ b/docker/Dockerfile.mcp
@@ -24,7 +24,7 @@ RUN --mount=type=cache,target=/go/pkg/mod \
  FROM alpine:latest

  # Base utilities
-RUN apk add --no-cache grep curl bash
+RUN apk add --no-cache grep curl bash dumb-init

  # Node.js + npm (provides npx for JS/TS MCP servers)
  RUN apk add --no-cache nodejs npm
@@ -40,4 +40,4 @@ COPY --from=build /out/mcp /opt/mcp
  COPY cmd/mcp/template /opt/mcp-template
  COPY cmd/mcp/entrypoint.sh /opt/entrypoint.sh
  RUN chmod +x /opt/entrypoint.sh
-ENTRYPOINT ["/opt/entrypoint.sh"]
+ENTRYPOINT ["/usr/bin/dumb-init", "--", "/opt/entrypoint.sh"]
```

### 3. 应用到我们的代码

```bash
# 找到我们的 MCP Dockerfile
find . -name "Dockerfile.mcp" -o -name "*mcp*Dockerfile*"

# 在我们的 Dockerfile 中应用相同修改：
# 1. 添加 dumb-init 安装
# 2. 修改 ENTRYPOINT 使用 dumb-init
```

### 4. 提交

```bash
git add docker/Dockerfile.mcp
git commit -m "fix: 移植上游 MCP 僵尸进程修复

- 添加 dumb-init 作为 PID 1
- 解决 MCP 容器产生僵尸进程的问题

上游提交: memohai/Memoh@8ce5243e"
```

---

## 常见问题

### Q1: 如何知道上游有哪些新功能？

```bash
# 查看上游最近提交
git log upstream/main --oneline -30

# 筛选功能性提交（排除文档、格式等）
git log upstream/main --oneline -30 | grep -E "feat|fix"
```

### Q2: 企业微信适配器会被覆盖吗？

**不会。** `internal/channel/adapters/wecom/` 目录是我们在主分支独有的文件，上游没有这些文件，因此不会冲突。

### Q3: 如果误覆盖了二次开发功能怎么办？

```bash
# 从备份分支恢复
git checkout wecom-dev-snapshot-20260310 -- internal/channel/adapters/wecom/

# 或从保存的 patch 恢复
git apply CHANGES_20250310.patch
```

### Q4: 多久同步一次上游？

建议：
- **Bugfix**: 发现相关问题时立即同步
- **功能更新**: 每月检查一次
- **大版本更新**: 评估后决定是否迁移

---

## 总结

通过本方案，您可以：

✅ **及时获取上游更新**（bugfix、新功能）
✅ **100% 保留二次开发功能**（企业微信适配器等）
✅ **精确控制每个变更**（选择性移植）
✅ **可追溯可回滚**（每次移植都有记录）

**下一步**: 查看 `UPSTREAM_SYNC_QUICKSTART.md` 开始实际操作。
