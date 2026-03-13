# Memoh-v2 项目文档

## 项目概述

**项目名称**: Memoh-v2  
**类型**: AI 助手/Agent 平台  
**部署位置**: `/data2/memoh-v2`  
**备份位置**: `/data1/backup/memoh_backup_20260313_212717`  
**备份时间**: 2026-03-13 21:27:17

---

## 一、近期工作要点 (2026-03)

### 1. 企业微信 (WeCom) 适配器升级

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

### 2. 基础设施修复

#### 2.1 SearXNG 联网搜索 (2026-03-12)
- **问题**: SearXNG 容器无法访问外网（搜索引擎超时）
- **修复**:
  - 配置 HTTP 代理: `http://ccd:88152353@10.71.252.4:10810`
  - 升级 SearXNG 镜像到最新版
  - 添加 `SEARXNG_DEFAULT_LANG=en` 修复 Google 引擎语言代码问题
- **配置文件**: `docker/config/searxng-settings.yml`

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
└── 0044_add_trigger_started_enum.up.sql  # 枚举修复
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

# 恢复数据库
docker run --rm --volumes-from memoh-postgres \
    -v $(pwd)/volumes:/backup alpine \
    tar xzf /backup/postgres_data.tar.gz -C /var/lib/postgresql/data
```

---

## 五、GitHub 提交记录

| 时间 | 提交 | 说明 |
|------|------|------|
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

*文档生成时间: 2026-03-13 21:30:00*  
*备份版本: memoh_backup_20260313_212717*
