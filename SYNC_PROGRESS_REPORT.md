# Memoh-v2 上游同步进度报告

**日期**: 2026-03-11
**同步分支**: `sync-upstream-20250311`
**备份分支**: `backup-pre-sync-20250311`

---

## 已完成功能移植

### 1. MCP 僵尸进程修复 ✅
- **上游提交**: `8ce5243e`
- **状态**: 已存在（本地代码已包含）
- **说明**: Dockerfile.mcp 已使用 dumb-init 作为 PID 1

### 2. 聊天图片冻结修复 ✅
- **上游提交**: `ef7ed961`
- **状态**: 已完成
- **修改文件**:
  - `packages/web/src/pages/chat/components/message-item.vue` - 添加 `loading="eager"`
  - `packages/web/src/pages/chat/components/file-preview-dialog.vue` - 添加 `loading="eager"`
  - `packages/web/src/router.ts` - 已存在 chunk load error 处理
  - `packages/web/vite.config.ts` - 已存在 optimizeDeps 配置

### 3. QQ 频道支持 ✅
- **上游提交**: `e6a6dbe3`
- **状态**: 已完成
- **新增文件**: `internal/channel/adapters/qq/` (16个文件)
- **修改文件**: `cmd/agent/main.go` - 添加 QQ 适配器导入

### 4. Discord 修复 ✅
- **上游提交**: `a2cb5939`
- **状态**: 已完成
- **修改文件**: `internal/channel/adapters/discord/stream.go`

### 5. 模型列表增量渲染 ✅
- **上游提交**: `93ddf3c6`
- **状态**: 已完成
- **修改文件**:
  - `packages/web/src/pages/models/components/model-list.vue`
  - `packages/web/src/i18n/locales/zh.json`
  - `packages/web/src/i18n/locales/en.json`

---

## 待完成/需要仔细处理的功能

### 1. 消息中止功能 ⚠️
- **上游提交**: `23d49a1c`
- **状态**: 待处理
- **复杂度**: 高
- **原因**:
  - 本地 `resolver.go` (4156行) 与上游版本 (2191行) 差异巨大
  - 本地版本有大量定制代码（记忆系统、重复响应检测等）
  - 需要仔细合并才能保留本地功能的同时添加上游中止功能
- **涉及文件**:
  - `internal/conversation/flow/resolver.go`
  - `internal/handlers/local_channel.go`
  - `agent/src/modules/chat.ts`
  - `agent/src/agent.ts`
  - `packages/web/src/composables/api/useChat.ws.ts` (新增)
  - `packages/web/src/composables/api/useChat.ts`
  - `packages/web/src/store/chat-list.ts`

### 2. Gmail OAuth2 支持 📋
- **上游提交**: `a5c36491`
- **状态**: 可选移植
- **说明**: 邮件功能增强，如果需要可以后续移植

---

## 本地核心功能状态

| 功能 | 状态 | 说明 |
|------|------|------|
| 企业微信适配器 | ✅ 保留 | `internal/channel/adapters/wecom/` - 本地定制版本 |
| 文件解析增强 | ✅ 保留 | `internal/fileparse/` - docx, xlsx, pdf, pptx |
| 文档生成技能 | ✅ 保留 | `internal/skills/defaults/` - docx, xlsx, pdf, pptx |
| 模型连接测试 | ✅ 保留 | `internal/models/probe.go` |
| 自动获取模型 | ✅ 保留 | `internal/handlers/providers.go` |
| DNS 解析修复 | ✅ 保留 | containerd 相关修复 |

---

## 验证检查清单

### 后端检查
- [x] QQ 适配器文件已添加
- [x] Discord stream.go 已更新
- [x] cmd/agent/main.go 已导入 QQ 适配器
- [ ] 消息中止功能待实现

### 前端检查
- [x] message-item.vue 已添加 loading="eager"
- [x] file-preview-dialog.vue 已添加 loading="eager"
- [x] model-list.vue 已添加增量渲染
- [x] i18n 翻译已更新
- [ ] useChat.ws.ts 待添加
- [ ] useChat.ts 待更新
- [ ] chat-list.ts 待更新

### 企业微信适配器检查
- [x] adapter.go (本地定制版本)
- [x] config.go
- [x] crypto.go
- [x] stream.go
- [x] types.go
- [x] websocket.go

---

## 建议后续工作

### 短期（本周内）
1. **测试已移植功能**
   - QQ频道适配器
   - Discord修复
   - 模型列表增量渲染
   - 图片冻结修复

2. **评估消息中止功能**
   - 分析本地 resolver.go 和上游版本的差异
   - 设计合并方案
   - 优先保证本地功能不受影响

### 中期（本月内）
1. 完成消息中止功能移植
2. 移植 Gmail OAuth2 支持（如需要）
3. 全面测试所有功能

### 长期
1. 建立定期同步流程
2. 考虑将企业微信适配器模块化，便于维护
3. 添加自动化测试覆盖

---

## 回滚方案

如果需要回滚到同步前状态：

```bash
# 方法1: 切换到备份分支
git checkout backup-pre-sync-20250311

# 方法2: 删除同步分支，回到 main
git checkout main
git branch -D sync-upstream-20250311
```

---

## Git 状态

```bash
# 当前分支
git branch --show-current
# sync-upstream-20250311

# 最近的提交
git log --oneline -3
# 82bc1264 sync: 移植上游功能（QQ频道、Discord修复、图片冻结修复、模型增量渲染）
# c72353b6 chore: 提交当前工作进度，准备同步上游更新
# e074c4b0 feat(wecom): 添加思考中即时回复功能
```

---

**报告生成时间**: 2026-03-11
**报告生成工具**: Claude Code
