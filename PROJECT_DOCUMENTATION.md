# Memoh-v2 项目文档

## 项目概述

**项目名称**: Memoh-v2  
**类型**: AI 助手/Agent 平台  
**部署位置**: `/data2/memoh-v2`  
**备份位置**: `/data1/backup/memoh_backup_20260313_212717`  
**备份时间**: 2026-03-13 21:27:17

---

## 一、近期工作要点 (2026-03)

### 1. 关键故障修复 (2026-03-16)

#### 1.1 对话中断及无回复内容问题修复

**问题现象**:
- 对话中出现"处理过程中断，请重试"错误
- 机器人只回复"处理完成，请查看完整回复"，不显示实际内容

**根本原因**:
| 问题 | 原因 | 影响 |
|------|------|------|
| 代码 Bug | `agent.ts` 引用未定义变量 `result` | 无法返回 LLM 生成的内容 |
| 网络隔离 | Agent 容器无法访问 `172.17.0.1` | LLM 请求失败 |
| HTTP 代理 | 代理转发本地请求返回 503 | 请求被错误路由 |

**修复措施**:
1. **修复 Agent 代码**: 修正 `agent/src/agent.ts` 中 `stream` 函数的错误
2. **更新网络配置**: 使用 `host.docker.internal` 替代 IP 地址
3. **配置 NO_PROXY**: 排除本地地址不被代理
4. **重建 Agent**: 重新构建并部署 Agent 容器

**关键文件**:
```
agent/src/agent.ts                        # 修复代码 Bug
docker-compose.yml                        # 添加 extra_hosts 和 NO_PROXY
docs/fix-conversation-interruption-2026-03-16.md  # 详细修复记录
docs/UPGRADE_GUIDE.md                     # 升级维护手册
docs/QUICK_REFERENCE.md                   # 快速参考卡片
```

**快速修复命令**:
```bash
# 1. 更新 LLM Provider 配置
docker exec -e PGPASSWORD=memoh123 memoh-postgres psql -U memoh -d memoh -c \
  "UPDATE llm_providers SET base_url = 'http://host.docker.internal:17099/v1' WHERE name = 'local-qwen35-direct';"

# 2. 重建并重启 Agent
cd /data2/memoh-v2 && docker compose build agent
docker compose stop agent && docker compose rm -f agent
docker compose up -d agent

# 3. 验证
docker logs memoh-agent --tail 20
```

---

### 2. 性能优化 (2026-03-15)

针对企业微信适配器响应速度慢的问题进行深度优化。

#### 1.1 核心优化项
| 优化项 | 原状态 | 优化后 | 效果 |
|--------|--------|--------|------|
| 消息加载 | MaxCount: 10000 | MaxCount: 1000, 硬限制 200 条 | 加载时间 500ms→50ms |
| 处理流程 | 遍历 5-6 次 | 单次遍历完成 | 减少 80% 处理时间 |
| 群消息防抖 | 300ms | 50ms | 减少 250ms 延迟 |
| Token 估算 | 强制启用 | 模型级开关（默认关闭） | 估算耗时 0ms |

#### 1.2 模型级 Token 估算开关
- **功能**: 每个模型可独立配置是否启用 Token 估算
- **默认**: 关闭（使用快速模式）
- **开启时**: 精确 Token 估算（较慢但更精确）
- **关闭时**: 保留最近 30 条消息（快速模式）
- **数据库字段**: `models.enable_token_estimate` (BOOLEAN, DEFAULT false)
- **前端位置**: 模型设置页面 → "启用 Token 估算" 开关

#### 1.3 适用场景建议
| 场景 | Token 估算设置 | 原因 |
|------|---------------|------|
| 大上下文模型 (32K+) | 开启 | 精确控制上下文，避免超出限制 |
| 性能敏感场景 | 关闭 | 减少延迟，提升响应速度 |
| 小模型/短对话 | 关闭 | 简单轮数限制已足够 |
| 长对话/复杂任务 | 开启 | 精确管理上下文 |

#### 1.4 关键文件
```
internal/conversation/flow/resolver.go    # 核心优化逻辑
internal/message/service.go               # 消息查询限制
internal/message/debounce.go              # 防抖延迟
internal/models/types.go                  # Token 估算字段
packages/web/src/components/create-model/index.vue  # 前端开关
```

### 2. 企业微信 (WeCom) 适配器升级

#### 1.1 新增功能
| 功能 | 状态 | 说明 |
|------|------|------|
| 消息去重 | ✅ | reqId + msgId 双机制去重，防止重复处理 |
| 命令白名单 | ✅ | 限制普通用户可执行的 slash 命令 |
| Admin 绕过 | ✅ | 管理员可绕过命令白名单限制 |
| Pending Reply 重试 | ✅ | WS 断连后通过 API 补发未送达消息 |
| Think 标签规范化 | ✅ | 支持 `<thinking>`/`<thought>`/`<reasoning>` 变体 |
| 配额感知 | ✅ | 24h 被动回复窗口追踪（30条上限） |
| 入群欢迎语 | ✅ | enter_chat 事件自动发送欢迎消息 |
| 流式节流 | ✅ | Reasoning 阶段 800ms 节流，防止队列溢出 |
| 图片发送 | ✅ | WebSocket 长连接支持发送 base64 图片 |

#### 1.2 关键 Bug 修复

**消息截断问题 (2026-03-13)**
- **问题**: 被动回复模式下，长消息分段发送时后续段落内容被覆盖
- **根因**: 后续段落使用与第一段相同的 streamID，企业微信端会覆盖之前的内容
- **修复**: 为后续段落生成新的 streamID，作为独立消息发送
- **文件**: `internal/channel/adapters/wecom/stream.go:959-985`

**panic: close of closed channel (2026-03-13)**
- **问题**: WebSocket 重连时可能触发 "close of closed channel" panic
- **修复**: 添加 `isManualClose` 标志位和 `sync.Once` 确保 channel 只关闭一次
- **文件**: `internal/channel/adapters/wecom/websocket.go`

**req_id 缺失错误 (2026-03-13)**
- **问题**: 主动发送消息 (CmdSendMsg) 时缺少 req_id
- **修复**: OpenStream 方法为 CmdSendMsg 模式生成新的 req_id
- **文件**: `internal/channel/adapters/wecom/adapter.go:405-415`

**图片发送功能修复 (2026-03-13)**
- **问题**: MCP 图片生成后无法发送给用户，WeCom 适配器只发送 `[附件消息]` 文本
- **根因**: `Send` 方法未处理 `Attachments` 字段，忽略了图片数据
- **修复**:
  - 修改 `Send` 方法支持图片附件
  - 将图片转为 base64 编码，通过 `msg_item` 字段发送
  - 计算图片 MD5 值用于企业微信校验
- **文件**: `internal/channel/adapters/wecom/adapter.go:361-415`
- **图片生成流程**:
  1. 用户发送 "画一只可爱的猫咪"
  2. Bot 调用 `z-image.generate_image_tool` 工具
  3. 生成图片并保存到 `/opt/memoh/data/bots/{bot_id}/media/`
  4. `generateAndSend` 调用 `channelManager.Send` 发送消息
  5. WeCom 适配器将图片转为 base64 并通过 WebSocket 发送

### 2. 基础设施修复

#### 2.1 性能优化数据库迁移 (2026-03-15)
- **迁移文件**: `0045_model_token_estimate.up.sql`
- **变更**: 添加 `enable_token_estimate` 字段到 models 表
- **默认**: false（优先性能）

#### 2.2 SearXNG 联网搜索 (2026-03-12 修复, 2026-03-15 扩展引擎)
- **问题**: SearXNG 容器无法访问外网（搜索引擎超时）
- **修复**:
  - 配置 HTTP 代理: `http://ccd:88152353@10.71.252.4:10810`
  - 升级 SearXNG 镜像到最新版
  - 添加 `SEARXNG_DEFAULT_LANG=en` 修复 Google 引擎语言代码问题
- **配置文件**: `docker/config/searxng-settings.yml`
- **可用引擎** (17个):
  - 国际综合: Google, Bing, Yahoo, Yandex, Mojeek
  - 中文综合: 百度, 搜狗, 360搜索
  - 知识: Wikipedia, Wikidata
  - 开发者: GitHub, GitLab
  - 学术: arXiv
  - 其他: APKMirror

#### 2.2 LLM 服务配置 (2026-03-11)
- **问题**: Docker 容器无法访问 LLM provider (`10.62.239.13`)
- **修复**: 更新为 Docker 网关地址 `172.17.0.1`
- **配置**:
  - qwen3.5-35B-A3B: `http://172.17.0.1:17099`
  - qwen3.5-27B: `http://172.17.0.1:17099`
  - Embedding: `http://172.17.0.1:8089`

#### 2.3 数据库枚举修复 (2026-03-11)
- **问题**: `process_log_step` 枚举缺少 `trigger_started` 值
- **修复**: 创建迁移文件 `0044_add_trigger_started_enum.up.sql`

#### 2.4 健康检查脚本修复 (2026-03-11)
- **问题**: 脚本检查错误的 URL
- **修复**: 更新为 `http://localhost:8080/health`
- **文件**: `/root/check_memoh_health.sh`

### 3. 模型配置更新

#### 3.1 嵌入模型
- **模型**: `bge-m3-q4_k_m.gguf`
- **向量维度**: 1024
- **端口**: 8089 (llama-embedding 服务)

#### 3.2 LLM 模型
- **主模型**: qwen3.5-35B-A3B (端口 17099, GPU)
- **备用模型**: qwen3.5-27B-UD-Q4_K_XL (已停止，CPU 模式)

---

## 二、系统架构

### 2.1 核心组件
| 组件 | 端口 | 说明 |
|------|------|------|
| memoh-server | 8080 | API 服务 |
| memoh-agent | 8081 | Agent Gateway |
| memoh-web | 8082 | Web UI |
| memoh-searxng | 8080 | 元搜索引擎 |
| memoh-postgres | 5432 | PostgreSQL 数据库 |
| memoh-qdrant | 6333-6334 | 向量数据库 |

### 2.2 外部服务
| 服务 | 端口 | 类型 | 状态 |
|------|------|------|------|
| llama-qwen35 | 17099 | GPU | 运行中 |
| llama-qwen27 | 17100 | CPU | 已停止 |
| llama-embedding | 8089 | GPU | 运行中 |

### 2.3 网络配置
- **Docker 网关**: `172.17.0.1`
- **HTTP 代理**: `http://ccd:88152353@10.71.252.4:10810`
- **Proxychains**: `socks5 10.40.31.69 10810 ccd 88152353`

---

## 三、关键文件位置

### 3.1 配置文件
```
/data2/memoh-v2/
├── docker-compose.yml              # Docker Compose 主配置
├── docker/config/searxng-settings.yml  # SearXNG 配置
├── config.toml                     # 应用配置
└── .env                            # 环境变量 (如存在)
```

### 3.2 企业微信适配器
```
/data2/memoh-v2/internal/channel/adapters/wecom/
├── adapter.go          # 主适配器逻辑
├── stream.go           # 流式消息处理
├── websocket.go        # WebSocket 连接管理
├── dedup.go            # 消息去重 (新增)
├── commands.go         # 命令白名单 (新增)
├── pending_reply.go    # Pending 回复管理 (新增)
├── quota.go            # 配额追踪 (新增)
├── think_parser.go     # Think 标签解析 (新增)
├── types.go            # 类型定义
└── config.go           # 配置解析
```

### 3.3 数据库迁移
```
/data2/memoh-v2/db/migrations/
├── 0044_add_trigger_started_enum.up.sql      # 枚举修复
├── 0045_model_token_estimate.up.sql          # Token 估算字段
└── 0045_model_token_estimate.down.sql        # Token 估算字段回滚
```

### 3.4 性能优化相关文件
```
/data2/memoh-v2/
├── internal/conversation/flow/resolver.go     # 对话流程优化
├── internal/message/service.go                # 消息查询优化
├── internal/message/debounce.go               # 防抖延迟优化
├── internal/models/types.go                   # Model struct
├── internal/models/models.go                  # 模型 CRUD
├── internal/db/sqlc/models.go                 # SQLC 模型
├── internal/db/sqlc/models.sql.go             # SQLC 查询
└── db/queries/models.sql                      # SQL 查询

# 前端
packages/web/src/components/create-model/index.vue  # 模型设置表单
packages/web/src/i18n/locales/zh.json               # 中文翻译
packages/web/src/i18n/locales/en.json               # 英文翻译
```

---

## 四、运维命令

### 4.1 服务管理
```bash
# 重启服务
cd /data2/memoh-v2 && docker compose restart

# 查看日志
docker logs memoh-server --tail 50
docker logs memoh-agent --tail 50
docker logs memoh-searxng --tail 20

# 检查健康状态
curl http://localhost:8080/health
```

### 4.2 备份恢复
```bash
# 执行备份
cd /data2/memoh-v2 && bash backup.sh /data1/backup

# 恢复代码
tar xzf memoh-v2_code.tar.gz

# 恢复数据库 (使用 SQL dump)
docker exec -i memoh-postgres psql -U memoh < volumes/postgres_dump.sql
```

---

## 五、GitHub 提交记录

| 时间 | 提交 | 说明 |
|------|------|------|
| 2026-03-13 | fe8009e5 | 支持 WebSocket 发送图片附件 |
| 2026-03-13 | 5b75a71d | 添加项目文档 |
| 2026-03-13 | 454f6e5a | 更新数据库恢复说明 |
| 2026-03-13 | 4aad5888 | 修复分段消息 streamID 覆盖问题 |
| 2026-03-13 | d9ab32eb | 修复主动发送消息缺少 req_id |
| 2026-03-13 | fe7f72c0 | 修复 panic: close of closed channel |
| 2026-03-13 | 402f8af9 | 修复消息截断问题 |
| 2026-03-13 | ce5e0b3b | 移除未使用的变量 |
| 2026-03-13 | 26e8499d | 企业微信适配器增强升级 |

**仓库**: https://github.com/91zgaoge/memoh-X

---

## 六、注意事项

1. **企业微信 SDK**: 遵循 AI Bot SDK 官方规范
   - `CmdRespondMsg`: 使用原始消息的 req_id
   - `CmdSendMsg`: 必须生成新的 req_id
   - 流式消息: 首次回复设置 stream.id，后续使用相同 id 刷新内容

2. **消息分段**: 单条消息最大 20480 字节，超出需分段发送

3. **频率限制**: 30条/分钟，实际使用 20条/分钟留有余量

4. **配额限制**: 被动回复 30条/24小时/会话

---

## 七、文档索引

### 故障修复文档
| 文档 | 说明 | 日期 |
|------|------|------|
| [docs/fix-conversation-interruption-2026-03-16.md](docs/fix-conversation-interruption-2026-03-16.md) | 对话中断及无回复内容修复（详细） | 2026-03-16 |
| [docs/UPGRADE_GUIDE.md](docs/UPGRADE_GUIDE.md) | 升级与维护操作手册 | 2026-03-16 |
| [docs/QUICK_REFERENCE.md](docs/QUICK_REFERENCE.md) | 快速参考卡片 | 2026-03-16 |
| [docs/sub2api-local-llm-setup-2026-03-16.md](docs/sub2api-local-llm-setup-2026-03-16.md) | Sub2API 本地 LLM 配置 | 2026-03-16 |
| [docs/agent-fetch-installation-2026-03-16.md](docs/agent-fetch-installation-2026-03-16.md) | Agent Fetch 安装指南 | 2026-03-16 |
| [docs/SECURITY_CHANGE_NOTICE_2026-03-15.md](docs/SECURITY_CHANGE_NOTICE_2026-03-15.md) | 安全配置变更通知 | 2026-03-15 |

### 项目文档
| 文档 | 说明 |
|------|------|
| [CHANGELOG.md](CHANGELOG.md) | 更新日志 |
| [README.md](README.md) | 项目说明 |
| [AGENTS.md](AGENTS.md) | Agent 配置指南 |

---

*文档生成时间: 2026-03-16*
*备份版本: memoh_backup_20260313_212717*
