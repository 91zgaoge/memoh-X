# 微信个人号扫码登录功能完善

**日期**: 2026-03-25
**提交**: `971aac49`
**作者**: Claude Code

## 概述

本次更新完善了 Memoh 的微信个人号（weixin）桥接功能，实现了多人扫码登录、实时状态更新、二维码图片生成等功能。

## 功能特性

### 1. 多人扫码支持

**问题**: 之前的实现中，一个人扫码登录后二维码就消失了，其他人无法扫码。

**解决**:
- 登录成功后不清除二维码 URL
- 新用户扫码时会触发登录切换
- 显示当前登录用户和切换提示

**使用流程**:
```
用户 A 扫码 → 登录成功 → 二维码仍显示
用户 B 扫码 → 显示切换提示 → 确认后切换登录
```

### 2. SSE 实时状态推送

**新增端点**: `GET /api/weixin-bridge/:bot_id/events`

**功能**:
- 实时推送二维码状态变化
- 推送扫码检测事件
- 推送登录成功事件
- 支持多客户端同时连接

**实现**:
```typescript
// 服务端
app.get('/events', (req, res) => {
  res.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    'Connection': 'keep-alive',
  });

  // 广播状态更新
  broadcastState();
});
```

### 3. 二维码图片生成

**新增端点**: `GET /api/weixin-bridge/:bot_id/qrcode-image`

**功能**:
- 直接生成 PNG 格式二维码图片
- 支持使用 `qrcode` npm 包本地生成
- 降级到外部服务（api.qrserver.com）

**实现**:
```typescript
app.get('/qrcode-image', async (req, res) => {
  if (QRCode) {
    const qrBuffer = await QRCode.toBuffer(dataToEncode, {
      type: 'png',
      width: 400,
    });
    res.send(qrBuffer);
  } else {
    // 降级到外部服务
    res.redirect(qrServiceUrl);
  }
});
```

### 4. 前端扫码弹窗

**功能**:
- 显示实时二维码图片
- 显示当前登录用户
- 显示扫码状态（等待扫码/已扫码/登录成功）
- 显示切换提示（当新用户扫码时）
- 可复制二维码链接分享给他人

**使用说明**:
1. 打开 Bot 设置页面
2. 选择微信个人号渠道
3. 点击"扫码登录"按钮
4. 使用微信扫描二维码
5. 可将弹窗分享给他人

## 技术实现

### 后端修改

#### 1. 路由注册

**文件**: `internal/handlers/weixin_bridge_manager.go`

```go
// 新增代理端点
group.GET("/:bot_id/qrcode-image", h.ProxyToBridge)
group.GET("/:bot_id/events", h.ProxyToBridge)
```

#### 2. JWT 白名单

**文件**: `internal/server/server.go`

```go
// 允许公开访问二维码端点
if strings.HasSuffix(path, "/qrcode-image") ||
   strings.HasSuffix(path, "/events") {
   return true
}
```

#### 3. 桥接服务状态管理

**文件**: `weixin-bridge/src/index.ts`

新增状态字段:
```typescript
interface BridgeState {
  status: 'pending' | 'connecting' | 'connected' | 'disconnected';
  qrcodeUrl: string | null;
  qrcodeData: string | null;
  currentUser: string | null;      // 当前登录用户
  loginHistory: string[];          // 登录历史
  scanDetected: boolean;           // 是否检测到扫码
  confirmationPending: boolean;    // 是否等待确认
}
```

### 前端修改

**文件**: `packages/web/src/pages/bots/components/channel-settings-panel.vue`

新增:
- `qrCodeCurrentUser` - 当前登录用户
- `qrCodeScanDetected` - 扫码检测状态
- SSE 连接管理
- 弹窗 UI 组件

## 文件变更

```
CHANGELOG.md                                          (+70 行)
internal/handlers/weixin_bridge_manager.go            (+2 行)
internal/server/server.go                             (+2 行)
packages/web/src/pages/bots/components/channel-settings-panel.vue    (+250 行)
weixin-bridge/src/index.ts                            (+400 行)
weixin-bridge/Dockerfile.tsx                          (新增)
weixin-bridge/Dockerfile.prebuilt                     (新增)
weixin-bridge/Dockerfile.local                        (新增)
```

## 部署说明

### 1. 构建微信桥接镜像

```bash
cd weixin-bridge

# 使用 tsx 方式（推荐，避免构建问题）
docker build -f Dockerfile.tsx -t memoh-weixin-bridge:latest .
```

### 2. 安装可选依赖（本地 QR 生成）

```bash
cd weixin-bridge
npm install qrcode
```

### 3. 构建前端

```bash
cd packages/web
pnpm install
pnpm build
```

### 4. 部署前端

```bash
docker cp dist/. memoh-web:/usr/share/nginx/html/
```

### 5. 重启服务

```bash
docker compose restart server
```

## 配置说明

### API Key 格式

微信桥接使用 preauth key 进行身份验证。建议使用简单的 8 位短 token：

```sql
-- 生成 preauth key
INSERT INTO bot_preauth_keys (bot_id, token, issued_by_user_id, expires_at)
VALUES (
  'your-bot-id',
  'xiaolongnv',  -- 8位短token
  'your-user-id',
  NOW() + INTERVAL '1 year'
);
```

### 环境变量

桥接容器需要以下环境变量：

```yaml
environment:
  MEMOH_BOT_ID: your-bot-id
  MEMOH_API_KEY: your-api-key
  MEMOH_SERVER_URL: http://memoh-server:8080
  TOKEN_PATH: /data/.weixin-bot/credentials.json
```

## 使用指南

### 首次配置

1. **创建 Bot**:
   - 进入 Memoh 管理界面
   - 创建新 Bot（如"小龙女"）

2. **启用微信渠道**:
   - 进入 Bot 设置
   - 选择"微信个人号"渠道
   - 点击"生成 API Key"

3. **启动桥接**:
   - 点击"启动桥接"按钮
   - 等待状态变为"运行中"

4. **扫码登录**:
   - 点击"扫码登录"按钮
   - 使用微信扫描二维码
   - 在手机上确认登录

5. **分享给他人**:
   - 将二维码弹窗链接分享给其他人
   - 其他人打开链接即可看到二维码
   - 新用户扫码后会切换登录

### 多用户切换

当前用户：用户 A
1. 用户 B 打开二维码页面
2. 用户 B 使用微信扫码
3. 页面显示"检测到新用户扫码，确认后切换"
4. 用户 B 在手机上确认登录
5. 当前登录用户变为：用户 B
6. 二维码仍然显示，可继续分享给用户 C

## 故障排查

### 二维码不显示

**检查**:
1. 桥接容器是否运行：`docker ps | grep weixin-bridge`
2. 日志是否有 QR URL：`docker logs memoh-weixin-bridge-xxx | grep "QR code URL"`
3. JWT 白名单是否正确配置

### API Key 无效

**检查**:
1. preauth key 是否存在且未过期
2. key 格式是否为 8 位短 token（不是 64 位哈希）
3. key 是否已使用（used_at 应为空）

```sql
SELECT token, used_at, expires_at > now() as valid
FROM bot_preauth_keys
WHERE bot_id = 'your-bot-id';
```

### 消息不回复

**检查**:
1. 桥接日志是否有消息接收记录
2. API Key 是否验证通过（401 错误）
3. Memoh 服务器是否正常运行
4. Agent 容器是否正常运行

## 已知限制

1. **微信机制限制**: 一个微信账号只能在一个地方保持登录，新用户扫码会踢掉前一个用户
2. **二维码有效期**: 微信二维码有效期约 8 分钟，过期后需要重新生成
3. **单设备限制**: 同一微信账号不能同时在多个设备登录

## 后续优化建议

1. **二维码过期提醒**: 在 UI 上显示二维码剩余有效时间
2. **登录状态持久化**: 将登录状态保存到数据库，重启后恢复
3. **多账号管理**: 支持同时管理多个微信账号
4. **扫码统计**: 记录扫码次数和成功率，便于排查问题

## 参考文档

- [微信个人号 Bridge 功能修复](./CHANGELOG.md)
- [WeChat Personal Account Bridge](./weixin-bridge/README.md)
