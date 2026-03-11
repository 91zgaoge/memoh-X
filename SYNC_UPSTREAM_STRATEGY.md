# Memoh-v2 与上游同步策略指南

## 概述

本指南帮助您在保持二次开发功能（企业微信适配器等）的同时，同步上游 https://github.com/memohai/Memoh 的最新更新。

## 关键发现

### 架构差异对比

| 组件 | 上游 (memohai/Memoh) | 我们 (Kxiandaoyan/Memoh-v2) | 差异程度 |
|------|---------------------|---------------------------|---------|
| Agent Gateway | `apps/agent/` | `agent/` | 轻微 |
| Browser 服务 | `apps/browser/` | **不存在** | **新增** |
| Web 前端 | `apps/web/` | `packages/web/` | 中等 |
| Packages | `packages/agent/`, `cli/`, `config/`, `sdk/`, `ui/` | `packages/web/`, `ui/`, `sdk/`, `cli/`, `config/` | 中等 |
| 后端 | `internal/` 重组 | `internal/` | 较大 |

### 上游新增功能（v0.4.2）

根据提交历史，上游新增的主要功能：

1. **消息中止功能** (`feat: message abort and web socket support`)
2. **Gmail OAuth2 支持** (`feat(email/oauth): implement OAuth2 support`)
3. **QQ 频道支持** (`docs(channels): add QQ channel configuration guide`)
4. **Discord 修复** (`fix(discord): rm reason in final message`)
5. **聊天图片冻结修复** (`fix(web): resolve chat image freeze`)
6. **MCP 僵尸进程修复** (`fix(mcp): use dumb-init as PID 1`)
7. **浏览器服务独立化** (`apps/browser/`)

### 我们的二次开发功能

1. **企业微信适配器** (`internal/channel/adapters/wecom/`)
2. **多模态消息处理修复**
3. **视觉模型支持扩展**
4. **文件解析增强** (docx/xlsx/pdf)
5. **Docker 配置优化**
6. **DNS 解析修复**

## 同步策略选择

### 策略 A: 手动移植新功能（推荐）

**适用场景**: 需要精确控制，保留所有二次开发功能

**步骤**:
1. 检出上游特定新功能文件
2. 手动适配到我们的代码结构
3. 测试验证

**优点**:
- 100% 保留二次开发功能
- 精确控制每个变更
- 避免架构冲突

**缺点**:
- 需要更多手动工作
- 需要理解上游代码变更

### 策略 B: 反向合并（我们 → 上游）

**适用场景**: 上游版本有重大改进，值得迁移二次开发功能

**步骤**:
1. 从 upstream/main 创建新分支
2. 手动添加我们的 wecom 适配器
3. 重新应用其他二次开发修改

**优点**:
- 获得最新上游架构
- 代码结构更现代化

**缺点**:
- 需要重新测试所有功能
- 工作量较大

### 策略 C: 选择性合并脚本（自动化）

**适用场景**: 定期同步，接受部分手动解决

**步骤**:
1. 使用提供的脚本自动识别可合并文件
2. 手动处理冲突文件
3. 测试并提交

## 推荐实施方案

### 第一步：环境准备

```bash
# 确保已添加上游仓库
git remote add upstream https://github.com/memohai/Memoh.git
git fetch upstream

# 创建同步工作分支
git checkout -b sync-upstream-$(date +%Y%m%d)
```

### 第二步：使用自动化脚本

运行提供的脚本 `./scripts/sync-upstream.sh`，该脚本会：

1. **自动合并安全文件**（无冲突的文件）
2. **识别并保留我们的核心修改**（wecom适配器等）
3. **生成冲突报告**，列出需要手动审查的文件

### 第三步：手动移植关键功能

对于每个上游新功能，按照以下清单操作：

#### 3.1 消息中止功能 (message abort)

**涉及文件**:
- `apps/agent/src/modules/chat.ts` → 移植到 `agent/src/modules/chat.ts`
- `apps/web/src/pages/chat/` → 移植到 `packages/web/src/pages/chat/`

**操作步骤**:
```bash
# 1. 比较差异
git diff upstream/main~10:apps/agent/src/modules/chat.ts upstream/main:apps/agent/src/modules/chat.ts > /tmp/abort_feature.patch

# 2. 手动应用到我们的代码
# 需要修改路径后应用
```

#### 3.2 Gmail OAuth2 支持

**涉及文件**:
- `internal/` 下的邮件相关代码
- 数据库迁移文件

**操作步骤**:
1. 检查 `db/migrations/` 新增文件
2. 移植邮件提供商配置

#### 3.3 浏览器服务 (apps/browser)

**说明**: 上游将浏览器功能独立为单独服务

**决策选项**:
- **选项1**: 保持当前架构（浏览器作为MCP工具）
- **选项2**: 移植新的独立浏览器服务

### 第四步：验证测试

**必须测试的功能**:

1. **企业微信适配器** (最高优先级)
   - 文本消息收发
   - 群聊@提及
   - 图片/文件上传
   - "新建对话"命令

2. **核心对话功能**
   - 流式响应
   - 记忆系统
   - 工具调用

3. **新增功能** (如移植)
   - 消息中止
   - WebSocket 连接

## 文件对比参考

### 关键文件映射表

| 上游文件 | 我们的文件 | 同步策略 |
|---------|----------|---------|
| `apps/agent/src/agent.ts` | `agent/src/agent.ts` | 手动审查后合并 |
| `apps/agent/src/modules/chat.ts` | `agent/src/modules/chat.ts` | 手动移植新功能 |
| `apps/web/src/pages/chat/` | `packages/web/src/pages/chat/` | 手动审查后合并 |
| `internal/channel/adapters/telegram/` | `internal/channel/adapters/telegram/` | 保留上游修复 |
| `internal/channel/adapters/discord/` | 不存在 | 可选添加 |
| `internal/channel/adapters/qq/` | 不存在 | 可选添加 |
| `apps/browser/` | 不存在 | 可选移植 |
| `internal/channel/adapters/wecom/` | `internal/channel/adapters/wecom/` | **保留我们的** |
| `internal/fileparse/` | `internal/fileparse/` | **保留我们的** |

## 自动化脚本使用指南

### 脚本: `scripts/sync-upstream.sh`

**功能**: 自动化同步流程

**使用方法**:
```bash
# 1. 配置脚本
export UPSTREAM_BRANCH="upstream/main"
export SYNC_STRATEGY="conservative"  # 或 aggressive

# 2. 运行脚本
./scripts/sync-upstream.sh

# 3. 查看报告
cat sync-report-$(date +%Y%m%d).md
```

**输出**:
- `sync-report-YYYYMMDD.md`: 详细同步报告
- `conflicts-to-resolve.txt`: 需要手动解决的文件列表
- `auto-merged.txt`: 自动合并的文件列表

## 风险与回滚

### 风险点

1. **企业微信适配器被覆盖** - 高影响
2. **多模态处理逻辑丢失** - 高影响
3. **数据库迁移冲突** - 中影响
4. **前端 API 不兼容** - 中影响

### 回滚方案

**回滚前请确保**:
- 已创建备份分支 `wecom-dev-snapshot-YYYYMMDD`
- 已保存完整 patch 文件

**回滚命令**:
```bash
# 放弃同步分支
git checkout main
git branch -D sync-upstream-YYYYMMDD

# 如需从备份恢复
git checkout wecom-dev-snapshot-YYYYMMDD
```

## 长期维护建议

### 建立定期同步流程

1. **每月检查**: 查看上游更新
   ```bash
   git fetch upstream
   git log upstream/main --oneline -20
   ```

2. **评估变更**: 判断是否有重要修复或功能

3. **小规模同步**: 优先同步 bugfix，暂缓大重构

4. **文档更新**: 每次同步后更新本文档

### 代码组织优化建议

考虑将二次开发功能模块化，便于长期维护：

```
internal/channel/adapters/
├── wecom/          # 企业微信适配器（我们的）
├── telegram/       # 上游同步
├── feishu/         # 上游同步
├── discord/        # 上游新增（可选）
└── qq/             # 上游新增（可选）
```

## 获取帮助

- 上游项目: https://github.com/memohai/Memoh
- 我们的 Fork: https://github.com/Kxiandaoyan/Memoh-v2
- 文档: `./doc/` 目录

---

**最后更新**: 2026-03-10
