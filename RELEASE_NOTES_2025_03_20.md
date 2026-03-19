# Memoh v2 更新日志 - 2025-03-20

## 概述

本次更新同步上游 `memohai/Memoh` 仓库的两大核心功能：
- **Phase A**: MCP OAuth 认证支持
- **Phase B**: Workspace 架构重构

---

## Phase A: MCP OAuth 认证支持

### 背景
此前本地 Memoh 无法连接需要 Bearer Token 认证的 MCP 服务器。上游已实现完整的 OAuth 2.0 认证流程，包括自动发现、PKCE 授权码流程和 Token 刷新。

### 主要变更

#### 1. 数据库迁移 (0046)
**文件**: `db/migrations/0046_mcp_probe_and_oauth.up.sql`

新增字段到 `mcp_connections` 表:
- `status` - 连接状态 (active/inactive/error/unknown)
- `tools_cache` - 工具列表缓存 (JSONB)
- `last_probed_at` - 最后探测时间
- `status_message` - 状态消息
- `auth_type` - 认证类型 (none/oauth/api_key)

新增 `mcp_oauth_tokens` 表:
- 存储 OAuth 2.0 Token 信息
- 支持 PKCE 流程的 code_verifier
- 自动刷新 Token 的 refresh_token

#### 2. OAuthService 实现
**文件**: `internal/mcp/oauth.go` (新增)

实现完整的 OAuth 2.0 流程:
- `Discover()` - OAuth 发现 (从 401 响应获取 Protected Resource Metadata)
- `Authorize()` - 生成授权 URL (PKCE S256)
- `ExchangeCode()` - 授权码换 Token
- `GetValidToken()` - 获取有效 Token (自动刷新)
- `Revoke()` - 撤销 Token

#### 3. MCP Federation Gateway 更新
**文件**: `internal/handlers/mcp_federation_gateway.go`

- 添加 `oauthService` 字段和 `SetOAuthService()` 方法
- `connectionHTTPClient()` 现在注入 OAuth Token 到请求头
- HTTP 客户端超时从 3600s 改为 30s

#### 4. OAuth HTTP 处理器
**文件**: `internal/handlers/mcp_oauth.go` (新增)

新增 API 端点:
- `GET /bots/:bot_id/mcp/:id/oauth/status` - 获取 OAuth 状态
- `POST /bots/:bot_id/mcp/:id/oauth/discover` - 发现 OAuth 端点
- `POST /bots/:bot_id/mcp/:id/oauth/authorize` - 开始授权
- `POST /bots/:bot_id/mcp/:id/oauth/callback` - 授权回调
- `DELETE /bots/:bot_id/mcp/:id/oauth` - 撤销 OAuth

#### 5. Connection 结构体更新
**文件**: `internal/mcp/connections.go`

```go
type Connection struct {
    // ... 原有字段 ...
    Status        string           `json:"status"`
    ToolsCache    []ToolDescriptor `json:"tools_cache"`
    LastProbedAt  *time.Time       `json:"last_probed_at,omitempty"`
    StatusMessage string           `json:"status_message"`
    AuthType      string           `json:"auth_type"`
}
```

---

## Phase B: Workspace 架构重构

### 背景
上游已将容器管理从 `internal/mcp/manager.go` 重构为 `internal/workspace/` 包，引入以下改进：
- 容器前缀从 `mcp-{id}` 改为 `workspace-{id}`
- 通信方式从 TCP gRPC 改为 Unix Domain Socket
- 引入 `WorkspaceService` 接口解耦 containerd 依赖

### 主要变更

#### 1. Workspace 包移植
**新增目录**: `internal/workspace/`

包含文件:
- `manager.go` - Workspace 管理器核心
- `manager_lifecycle.go` - 容器生命周期管理
- `versioning.go` - 快照/版本管理
- `dataio.go` - 数据 I/O 操作
- `identity.go` - 容器标识管理
- `image_preference.go` - 镜像偏好设置
- `bridge/client.go` - gRPC 桥接客户端
- `bridge/errors.go` - 桥接错误定义
- `bridgepb/bridge.proto` - 桥接协议定义
- `bridgepb/bridge.pb.go` - 生成的 Go 代码
- `bridgepb/bridge_grpc.pb.go` - 生成的 gRPC 代码

#### 2. Containerd 适配层
**文件**: `internal/containerd/workspace_adapter.go` (新增)

实现 `WorkspaceService` 接口，将原有的 `DefaultService` 适配为 Workspace 包期望的 API:

```go
type WorkspaceService interface {
    PullImage(ctx context.Context, ref string, opts *PullImageOptions) (ImageInfo, error)
    CreateContainer(ctx context.Context, req CreateContainerRequest) (ContainerInfo, error)
    StartContainer(ctx context.Context, containerID string, opts *StartTaskOptions) error
    StopContainer(ctx context.Context, containerID string, opts *StopTaskOptions) error
    SetupNetwork(ctx context.Context, req NetworkSetupRequest) (NetworkResult, error)
    // ... 更多方法
}
```

#### 3. 值类型定义
**文件**: `internal/containerd/workspace_types.go` (新增)

新增上游使用的值类型，解耦对 containerd 客户端类型的依赖:
- `TaskStatus` - 任务状态枚举
- `ContainerInfo` - 容器信息 (含 RuntimeInfo)
- `ImageInfo` - 镜像信息
- `WorkspaceTaskInfo` - 任务信息
- `NetworkSetupRequest/NetworkResult` - 网络配置
- `SnapshotInfo` - 快照信息
- `MountSpec/ContainerSpec` - 容器规格

#### 4. 时区支持
**文件**: `internal/containerd/timezone.go` (新增)

```go
func TimezoneSpec() ([]MountSpec, []string)
```
自动挂载 `/etc/localtime` 并设置 `TZ` 环境变量。

#### 5. 数据库迁移 (0047)
**文件**: `db/migrations/0047_workspace_snapshot_meta.up.sql`

`a snapshots` 表新增字段:
- `runtime_snapshot_name` - 运行时快照名称 (唯一索引)
- `source` - 快照来源 (manual/pre_exec/rollback)
- `display_name` - 显示名称

新增 SQLC 查询:
- `ListSnapshotsWithVersionByContainerID` - 联表查询快照版本
- `UpsertSnapshot` - 插入或更新快照
- `GetVersionSnapshotRuntimeName` - 获取版本对应快照名

#### 6. Agent 主程序更新
**文件**: `cmd/agent/main.go`

新增依赖注入:
```go
provideWorkspaceAdapter,   // 提供 WorkspaceAdapter
provideWorkspaceManager,   // 提供 workspace.Manager
```

#### 7. gRPC 桥接修复
**文件**: `internal/workspace/bridgepb/bridge.pb.go` (重新生成)

原文件从上游复制时包含损坏的 protobuf raw descriptor，导致运行时 panic:
```
panic: runtime error: slice bounds out of range [-4:]
```

使用本地 `protoc v3.13.0` + `protoc-gen-go v1.36.11` 重新生成，修复描述符兼容性。

---

## 部署说明

### 数据库迁移

```bash
# 运行迁移脚本
cd /data2/memoh-v2
bash scripts/db-up.sh

# 或手动执行
for f in db/migrations/0046*.up.sql db/migrations/0047*.up.sql; do
  docker compose exec -T postgres psql -U memoh -d memoh -f /dev/stdin < "$f"
done
```

### 服务重启

```bash
docker compose build server
docker compose stop server
docker compose up -d server
```

---

## 验证

### OAuth 功能验证
1. 在 Web UI 添加需要 OAuth 的 MCP 服务器
2. 设置 `auth_type` 为 `oauth`
3. 触发 OAuth 授权流程
4. 验证 Token 获取和自动刷新

### Workspace 功能验证
1. 检查容器创建是否正常（前缀 `workspace-`）
2. 测试快照创建和回滚
3. 验证 gRPC 桥接通信

---

## 文件清单

### 新增文件
```
db/migrations/0046_mcp_probe_and_oauth.up.sql
db/migrations/0046_mcp_probe_and_oauth.down.sql
db/migrations/0047_workspace_snapshot_meta.up.sql
db/migrations/0047_workspace_snapshot_meta.down.sql
internal/mcp/oauth.go
internal/handlers/mcp_oauth.go
internal/db/sqlc/mcp_oauth.sql.go
internal/containerd/workspace_types.go
internal/containerd/workspace_adapter.go
internal/containerd/timezone.go
internal/workspace/ (完整目录)
```

### 修改文件
```
cmd/agent/main.go
internal/config/config.go
internal/containerd/service.go
internal/db/sqlc/mcp.sql.go
internal/db/sqlc/models.go
internal/db/sqlc/snapshots.sql.go
internal/db/sqlc/versions.sql.go
internal/handlers/mcp_federation_gateway.go
internal/mcp/connections.go
```

---

## 已知限制

1. **向后兼容性**: 旧版 `mcp-{id}` 前缀容器仍可通过 `LegacyContainerPrefix` 识别
2. **OAuth 提供商**: 目前仅测试过标准 OAuth 2.0 + PKCE 流程
3. **Snapshot 迁移**: 现有快照需要重新创建才能使用版本管理功能

---

## 参考

- 上游仓库: https://github.com/memohai/Memoh
- Fork 仓库: https://github.com/Kxiandaoyan/Memoh-v2
