# Memoh-v2 上游同步快速入门

本文档提供与上游 https://github.com/memohai/Memoh 同步的快速指南。

## 现状分析

### 架构差异

| 项目 | 上游 (memohai/Memoh) | 我们 (Kxiandaoyan/Memoh-v2) |
|------|---------------------|---------------------------|
| **Agent Gateway** | `apps/agent/` | `agent/` |
| **Browser 服务** | `apps/browser/` (新增) | **无** |
| **Web 前端** | `apps/web/` | `packages/web/` |
| **二次开发** | 无 | ✅ 企业微信适配器、多模态修复等 |

### 我们的核心二次开发功能

1. **企业微信适配器** (`internal/channel/adapters/wecom/`)
2. **多模态消息处理修复** (图片、文件上传)
3. **文件解析增强** (docx/xlsx/pdf)
4. **视觉模型支持** (llama.cpp vision)
5. **Docker/DNS 优化**

## 推荐同步策略

由于架构差异较大，**推荐使用"功能级移植"而非"全量合并"**。

## 使用方法

### 步骤 1: 查看上游更新

```bash
# 获取上游最新提交
git fetch upstream

# 查看上游最近更新
git log upstream/main --oneline -20
```

### 步骤 2: 提取特定功能

使用提供的脚本提取上游特定功能：

```bash
# 提取消息中止功能
./scripts/extract-upstream-feature.sh "23d49a1c^..23d49a1c" "message-abort"

# 提取 Gmail OAuth2 支持
./scripts/extract-upstream-feature.sh "a5c36491^..a5c36491" "gmail-oauth"

# 提取聊天图片冻结修复
./scripts/extract-upstream-feature.sh "ef7ed961^..ef7ed961" "chat-image-freeze-fix"
```

### 步骤 3: 查看生成的移植指南

脚本会在 `upstream-patches/` 目录生成两个文件：
- `message-abort-YYYYMMDD.patch` - 补丁文件
- `message-abort-YYYYMMDD-README.md` - 移植指南

```bash
# 查看移植指南
cat upstream-patches/message-abort-YYYYMMDD-README.md

# 查看补丁内容
cat upstream-patches/message-abort-YYYYMMDD.patch
```

### 步骤 4: 手动移植

根据移植指南，**手动**将变更应用到我们的代码：

1. **路径映射**:
   - `apps/agent/` → `agent/`
   - `apps/web/` → `packages/web/`
   - `apps/browser/` → 需要评估是否移植

2. **应用变更**:
   - 阅读补丁文件理解变更内容
   - 在我们的代码中找到对应位置
   - 手动应用逻辑变更（不要直接打补丁，路径不同）

### 步骤 5: 测试验证

移植后必须测试：

```bash
# 1. 编译测试
docker compose build

# 2. 企业微信适配器测试
# - 文本消息收发
# - 群聊@提及
# - 图片上传
# - "新建对话"命令

# 3. 基础功能测试
# - 普通对话
# - 流式响应
# - 记忆系统
```

## 上游新功能清单

### 高优先级（推荐移植）

| 功能 | 提交 | 说明 |
|------|------|------|
| 消息中止 | `23d49a1c` | 用户可中止正在生成的回复 |
| 聊天图片冻结修复 | `ef7ed961` | 修复聊天界面图片卡顿 |
| MCP 僵尸进程修复 | `8ce5243e` | 使用 dumb-init 解决僵尸进程 |

### 中优先级（可选移植）

| 功能 | 提交 | 说明 |
|------|------|------|
| Gmail OAuth2 | `a5c36491` | 邮件发送 OAuth 支持 |
| Discord 修复 | `a2cb5939` | Discord 适配器修复 |
| QQ 频道支持 | 文档更新 | QQ 频道配置指南 |

### 低优先级（暂缓移植）

| 功能 | 说明 |
|------|------|
| 独立 Browser 服务 | 架构变化大，当前 MCP browser 功能可用 |

## 保留二次开发功能的技巧

### 关键文件保护清单

这些文件/目录**必须保留我们的版本**：

```
internal/channel/adapters/wecom/     # 企业微信适配器
internal/fileparse/                  # 文件解析增强
internal/skills/defaults/*/          # 文档生成技能
cmd/agent/main.go                    # 适配器注册
internal/conversation/flow/resolver.go  # 多模态处理
```

### 使用脚本保护

运行同步脚本时会自动保护这些文件：

```bash
./scripts/sync-upstream.sh
```

## 常见问题

### Q: 直接合并 upstream/main 会怎样？

**A**: 会产生大量冲突（288+文件），且会覆盖我们的二次开发功能。**不推荐**直接合并。

### Q: 如何只获取上游的 bugfix？

**A**: 使用 `extract-upstream-feature.sh` 脚本提取特定提交，然后手动移植。

```bash
# 示例：提取 MCP 僵尸进程修复
./scripts/extract-upstream-feature.sh "8ce5243e^..8ce5243e" "mcp-zombie-fix"
```

### Q: 企业微信适配器会被覆盖吗？

**A**: 如果按照本文档的方法操作，**不会**。`internal/channel/adapters/wecom/` 目录是我们在主分支独有的，upstream 没有这些文件。

### Q: 如何回滚同步操作？

**A**: 同步前会自动创建备份分支：

```bash
# 查看备份分支
git branch | grep backup

# 回滚到同步前
git checkout main
git reset --hard pre-sync-backup-YYYYMMDD
```

## 快速命令参考

```bash
# 查看上游最新提交
git fetch upstream && git log upstream/main --oneline -10

# 提取特定功能
./scripts/extract-upstream-feature.sh "COMMIT^..COMMIT" "feature-name"

# 运行完整同步流程（带保护）
./scripts/sync-upstream.sh

# 查看生成的报告
cat sync-report-*.md
```

## 获取帮助

- 详细策略文档: `SYNC_UPSTREAM_STRATEGY.md`
- 更新日志: `doc/CHANGELOG.md`
- 企业微信文档: `doc/wecom-integration.md`
