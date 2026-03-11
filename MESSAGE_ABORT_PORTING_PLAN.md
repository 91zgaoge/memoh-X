# 消息中止功能移植计划

> 上游提交: `23d49a1c feat: message abort and web socket support`
> 涉及文件: 21 个, 1683 行变更

---

## 功能概述

消息中止功能允许用户在生成回复过程中点击"中止"按钮来停止生成。这需要：

1. **WebSocket 支持** - 建立双向通信通道
2. **流式处理修改** - 支持 AbortSignal 来中止生成
3. **UI 更新** - 添加中止按钮和状态显示
4. **后端修改** - 支持流式响应的中止

---

## 移植阶段

### 阶段 1: Agent Gateway 修改 (文件: `agent/src/modules/chat.ts`)

**变更内容**:
- 添加 `buildAgentAndStream` 辅助函数
- 修改 `/stream` 端点以支持 AbortSignal
- 添加 WebSocket 端点 `/ws`

**预估工作量**: 2-3 小时
**风险**: 中等 - 影响核心流式处理

```typescript
// 需要添加的关键代码
const StreamBodyModel = AgentModel.extend({
  query: z.string().optional().default(''),
})

function buildAgentAndStream(body: z.infer<typeof StreamBodyModel>, bearer: string, signal?: AbortSignal) {
  // 创建 Agent 并返回 stream 函数
}

// 在 /stream 端点中使用
const abortController = new AbortController()
for await (const action of buildAgentAndStream(body, bearer!, abortController.signal)) {
  yield sse(JSON.stringify(action))
}

// 添加 WebSocket 支持
.ws('/ws', ...)
```

### 阶段 2: Agent 核心修改 (文件: `agent/src/agent.ts`)

**变更内容**:
- 修改 `stream` 函数以接受外部 AbortSignal
- 添加 `agent_abort` 动作类型

**关键修改**:
```typescript
async function* stream(input: AgentInput, externalSignal?: AbortSignal): AsyncGenerator<AgentAction> {
  // 组合内部和外部 signal
  const combinedSignal = combineSignals(streamAbort.signal, externalSignal)

  // 在 yield 前检查 signal
  if (externalSignal?.aborted) {
    yield { type: 'agent_abort' }
    return
  }
}
```

**预估工作量**: 1-2 小时
**风险**: 高 - 核心逻辑修改

### 阶段 3: 前端 WebSocket 客户端 (新建文件: `packages/web/src/composables/api/useChat.ws.ts`)

**变更内容**:
- 新建 WebSocket 客户端封装
- 实现连接管理和重连逻辑
- 实现消息发送和中止功能

**预估工作量**: 2-3 小时
**风险**: 中等 - 新增功能模块

### 阶段 4: 前端状态管理 (文件: `packages/web/src/store/chat-list.ts`)

**变更内容**:
- 添加中止状态管理
- 添加 WebSocket 连接状态
- 实现中止按钮逻辑

**预估工作量**: 1-2 小时
**风险**: 低

### 阶段 5: 前端 UI 更新 (文件: `packages/web/src/composables/api/useChat.ts`)

**变更内容**:
- 在流式响应中添加中止检测
- 更新事件类型定义 (添加 `agent_abort`)
- 导出 WebSocket 相关函数

**预估工作量**: 1 小时
**风险**: 低

### 阶段 6: 后端流式处理修改 (文件: `internal/conversation/flow/resolver.go`)

**变更内容**:
- 添加对 Agent Gateway WebSocket 的支持
- 修改流式响应处理以支持中止
- 添加连接状态管理

**预估工作量**: 2-3 小时
**风险**: 高 - 后端核心逻辑

### 阶段 7: 类型定义和 SDK 更新

**文件列表**:
- `packages/agent/src/types/action.ts` - 添加 `agent_abort` 类型
- `packages/agent/src/types/agent.ts` - 更新 Agent 类型
- `packages/sdk/src/types.gen.ts` - 更新 SDK 类型

**预估工作量**: 1 小时
**风险**: 低

### 阶段 8: 配置和部署

**变更内容**:
- 更新 `cmd/agent/main.go` - 添加 WebSocket 路由
- 更新 `docker/nginx.conf` - WebSocket 代理配置
- 更新 `packages/web/vite.config.ts` - WebSocket 开发代理

**预估工作量**: 1 小时
**风险**: 中等

---

## 移植建议

### 方案 A: 完整移植（推荐）

按上述阶段逐个移植，每个阶段完成后进行测试。

**优点**: 完整功能，与上游保持一致
**缺点**: 工作量大，需要较多测试

### 方案 B: 简化移植

仅移植核心中止功能，暂时跳过 WebSocket（使用 HTTP 长轮询替代）。

**优点**: 工作量小，实现快速
**缺点**: 不如 WebSocket 实时性好

### 方案 C: 暂缓移植

考虑到消息中止功能虽然用户体验好，但不是核心功能，可以暂缓移植。

**适用场景**: 当前功能已满足需求，资源有限

---

## 测试清单

移植完成后必须测试：

- [ ] WebSocket 连接正常建立
- [ ] 正常对话流不受影响
- [ ] 点击中止按钮能立即停止生成
- [ ] 中止后 UI 状态正确更新
- [ ] 企业微信适配器工作正常
- [ ] 多模态消息处理正常
- [ ] 并发多用户场景测试
- [ ] 断网重连恢复测试

---

## 回滚方案

如果移植出现问题：

```bash
# 回滚到移植前状态
git log --oneline -10  # 找到移植前的提交
git reset --hard <commit-hash-before-porting>

# 或者撤销特定文件的修改
git checkout HEAD -- agent/src/modules/chat.ts
```

---

## 参考资源

- 上游补丁: `upstream-patches/message-abort-20260310.patch`
- 移植指南: `upstream-patches/message-abort-20260310-README.md`
- 上游提交: https://github.com/memohai/Memoh/commit/23d49a1c

---

## 下一步行动

1. **决定移植方案** (A/B/C)
2. **准备测试环境**
3. **按阶段执行移植**
4. **每阶段完成后测试**
5. **最终验证和提交**

---

*计划创建时间: 2026-03-10*
*预计总工作量: 10-15 小时（方案A）*
