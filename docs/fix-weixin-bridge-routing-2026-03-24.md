# 微信个人 Bridge 功能修复记录

## 日期
2026-03-24

## 问题描述
前端页面 BOT 连接微信（个人）渠道时，点击启动 bridge 按钮迅速弹出 "not found" 错误通知，桥接服务状态检测一直显示"未知"。

## 根本原因

### 问题 1: Nginx 路径匹配问题
Nginx 配置中使用 `location /api/weixin-bridge/`（带尾部斜杠），导致：
1. 前端 POST `/api/weixin-bridge` 请求被 Nginx 返回 301 重定向到 `/api/weixin-bridge/`
2. 浏览器跟随重定向时将 POST 方法改为 GET
3. GET `/api/weixin-bridge/` 返回 404，因为后端没有注册该路由

### 问题 2: fx 依赖注入配置
`WeixinBridgeManager` handler 需要通过 fx 框架正确注册为 `server.Handler` 接口。

## 修复内容

### 1. Nginx 配置修复
**文件**: `docker/config/nginx.conf`

```nginx
# 修改前
location /api/weixin-bridge/ {
    ...
}

# 修改后 - 移除尾部斜杠
location /api/weixin-bridge {
    ...
}
```

### 2. fx Provider 函数
**文件**: `internal/handlers/weixin_bridge_provider.go`（新增）

```go
func NewWeixinBridgeManagerFunc(logger *slog.Logger) *WeixinBridgeManager {
    return NewWeixinBridgeManager(logger)
}
```

**文件**: `cmd/agent/main.go`

```go
provideServerHandler(handlers.NewWeixinBridgeManagerFunc),
```

### 3. 前端状态检测修复
**文件**: `packages/web/src/pages/bots/components/channel-settings-panel.vue`

修改 `refreshWeixinStatus` 函数，从调用 `/status`（代理到容器）改为调用 `/info`（后端直接处理）：

```typescript
async function refreshWeixinStatus() {
  const resp = await fetch(`${weixinBridgeUrl.value}/info`, {
    headers: { 'Authorization': `Bearer ${token}` },
    signal: AbortSignal.timeout(5000),
  })
  if (resp.ok) {
    const data = await resp.json()
    weixinBridgeStatus.value = data.info?.status ?? 'unknown'
  } else if (resp.status === 404) {
    weixinBridgeStatus.value = 'stopped'
  }
}
```

### 4. 后端路由注册
**文件**: `internal/handlers/weixin_bridge_manager.go`

```go
func (h *WeixinBridgeManager) Register(e *echo.Echo) {
    group := e.Group("/api/weixin-bridge")

    // 管理端点
    group.POST("", h.StartBridge)
    group.DELETE("/:bot_id", h.StopBridge)
    group.GET("/:bot_id/info", h.GetBridgeInfo)

    // 代理端点
    group.GET("/:bot_id/status", h.ProxyToBridge)
    group.GET("/:bot_id/qrcode", h.ProxyToBridge)
    group.GET("/:bot_id/health", h.ProxyToBridge)
    group.GET("/:bot_id/qrcode.txt", h.ProxyToBridge)
}
```

### 5. JWT 认证放行
**文件**: `internal/server/server.go`

```go
e.Use(auth.JWTMiddleware(jwtSecret, func(c echo.Context) bool {
    // ...
    // Weixin bridge proxy endpoints - bridge has its own simple auth
    if strings.HasPrefix(path, "/api/weixin-bridge/") &&
        (strings.HasSuffix(path, "/status") ||
         strings.HasSuffix(path, "/qrcode") ||
         strings.HasSuffix(path, "/health") ||
         strings.HasSuffix(path, "/qrcode.txt")) {
        return true
    }
    return false
}))
```

## 新增文件
- `internal/handlers/weixin_bridge_manager.go` - Bridge 管理器
- `internal/handlers/weixin_bridge_provider.go` - fx provider 函数
- `internal/handlers/weixin_webhook.go` - 微信 webhook 处理
- `internal/channel/adapters/weixin/` - 微信适配器
- `weixin-bridge/` - Bridge 容器 Dockerfile 及相关文件

## 修改文件
- `cmd/agent/main.go` - 添加 fx provider 注册
- `docker/config/nginx.conf` - 修复 location 路径匹配
- `internal/server/server.go` - 添加 JWT 放行规则
- `packages/web/src/pages/bots/components/channel-settings-panel.vue` - 修复状态检测
- `packages/web/src/pages/bots/components/bot-channels.vue` - 添加微信渠道支持
- `packages/web/src/i18n/locales/zh.json` / `en.json` - 添加国际化
- `docker-compose.yml` - 添加 weixin-bridge 服务配置

## 验证步骤
1. 打开 BOT 的微信（个人）渠道设置页面
2. 点击"启动 Bridge"按钮，应返回 202 Accepted
3. 状态应正确显示为"pending" -> "running" 或 "stopped"
4. 二维码链接应能正常打开

## 相关提交
- 修复 Nginx location 路径匹配问题
- 添加 WeixinBridgeManager handler 注册
- 前端状态检测改为调用 /info 端点
