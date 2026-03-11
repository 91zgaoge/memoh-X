# 上游功能移植指南: chat-image-freeze-fix

**提取时间**: Tue Mar 10 04:45:29 AM CST 2026
**上游提交**: ef7ed961^..ef7ed961
**补丁文件**: upstream-patches/chat-image-freeze-fix-20260310.patch

## 功能描述

fix(web): resolve chat image freeze and dynamic import failures

## 涉及文件

apps/web/src/pages/chat/components/attachment-block.vue
apps/web/src/router.ts
apps/web/vite.config.ts

## 移植步骤

### 1. 审查变更

查看补丁文件了解具体变更：
```bash
cat upstream-patches/chat-image-freeze-fix-20260310.patch
```

### 2. 路径映射

上游路径 → 我们的路径映射：

- `apps/agent/` → `agent/`
- `apps/web/` → `packages/web/`
- `apps/browser/` → (需要新建或集成到现有代码)
- `packages/ui/` → `packages/ui/` (相同)
- `packages/sdk/` → `packages/sdk/` (相同)

### 3. 逐步移植

对于每个涉及文件：

1. 找到对应的我们的代码文件
2. 手动应用相关变更
3. 解决路径和导入差异
4. 测试编译

### 4. 冲突解决提示

#### 常见冲突类型

**路径冲突**: 上游使用 `apps/` 结构，我们使用 `packages/` 和 `agent/`
- 解决: 修改补丁中的路径后应用

**导入冲突**: 包名或导入路径不同
- 解决: 手动修改导入语句

**代码风格冲突**: 格式化或命名差异
- 解决: 保持我们的代码风格

#### 安全修改 vs 需要审查的修改

✅ **通常可以安全应用**:
- 新增文件
- Bug 修复（小范围变更）
- 新增配置选项

⚠️ **需要仔细审查**:
- 核心逻辑变更
- 数据库结构变更
- API 接口变更
- 依赖版本升级

### 5. 测试验证

移植完成后验证:

- [ ] 代码编译通过
- [ ] 功能正常工作
- [ ] 不影响现有功能（特别是企业微信适配器）
- [ ] 没有引入新的错误

## 参考信息

### 上游完整提交信息

```
commit ef7ed961a91da39e6b6ce8c555684d97f70af3b9
Author: Acbox <acbox0328@gmail.com>
Date:   Tue Mar 10 00:19:14 2026 +0800

    fix(web): resolve chat image freeze and dynamic import failures
    
    - attachment-block: use loading=eager with explicit dimensions to prevent Chromium lazy-load intervention from freezing the page
    - router: add error handler for chunk load failures to auto-reload
    - vite: pre-bundle route page dependencies to improve initial load speed
```

### 相关文件清单

```
apps/web/src/pages/chat/components/attachment-block.vue
apps/web/src/router.ts
apps/web/vite.config.ts
```
