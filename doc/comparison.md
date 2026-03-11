# 与 OpenClaw 全面对比（45 项）

> 返回 [文档首页](./README.md) · [项目首页](../README.md)

---

> 结果列：**M** = Memoh-X 胜 · **O** = OpenClaw 胜 · **=** = 持平

| # | 维度 | Memoh-X | OpenClaw | 结果 |
|---|---|---|---|:---:|
| 1 | 后端语言 | Go（高并发、编译型） | Node.js（单线程、解释型） | **M** |
| 2 | 架构模式 | 三服务分离（Server / Gateway / Web） | 单体应用 | **M** |
| 3 | 通信协议 | SSE 单向流式推送 | WebSocket 全双工 | **O** |
| 4 | 容器隔离 | containerd 独立容器/Bot，完全隔离 | 共享运行时（可选 Docker 沙盒） | **M** |
| 5 | 结构化数据库 | PostgreSQL | SQLite | **M** |
| 6 | 向量数据库 | Qdrant（独立服务） | SQLite-vec（嵌入式） | **M** |
| 7 | 水平扩展 | 服务可独立部署与扩展 | 单机运行 | **M** |
| 8 | 资源占用 | 需 Docker + PostgreSQL + Qdrant | 轻量单进程，几十 MB 内存 | **O** |
| 9 | 部署方式 | Docker Compose 一键部署 | npm install -g + CLI 启动 | **=** |
| 10 | 远程访问 | 天然支持（Docker 部署到任意服务器） | 需 Tailscale / SSH 隧道 | **M** |
| 11 | Agent 定义体系 | SOUL + IDENTITY + TOOLS + EXPERIMENTS + NOTES | SOUL + IDENTITY + TOOLS + AGENTS + HEARTBEAT + BOOTSTRAP + USER | **=** |
| 12 | 子 Agent 管理 | spawn/kill/steer + 独立工具权限 + 注册表 | spawn/kill/steer + 深度限制 + 子数上限 | **=** |
| 13 | 工具执行框架 | MCP 协议（容器内沙盒执行） | Pi Runtime 内置工具（Browser/Canvas/Nodes） | **O** |
| 14 | MCP 协议支持 | 原生支持，可连接任意 MCP Server | 有限支持 + ACP 协议 | **M** |
| 15 | 浏览器自动化 | Chromium + agent-browser + Actionbook + xvfb | 内置 Browser + agent-browser + Actionbook | **=** |
| 16 | 智能网页策略 | Markdown Header → Actionbook → curl 三级降级 | 标准抓取 | **M** |
| 17 | 技能市场 | ClawHub + OPC Skills | ClawHub + OPC Skills | **=** |
| 18 | 短期记忆 | 最近 24h 对话自动加载 | 当前 session 对话 | **M** |
| 19 | 长期记忆 | Qdrant 向量语义搜索 + BM25 关键词匹配，每轮自动入库 | SQLite-vec 向量搜索 + memoryFlush | **M** |
| 20 | 上下文压缩 | Token 预算裁剪 + LLM 自动摘要 | /compact 手动压缩 | **M** |
| 21 | 分层上下文 | OpenViking（L0/L1/L2），每 Bot 可独立开关 | 无 | **M** |
| 22 | 自我进化机制 | 三阶段有机进化（反思/实验/审查）+ 进化日志追踪 | MEMORY.md 手动迭代 | **M** |
| 23 | Bot 模板 | 13 套思维模型模板（含 10 套真实思想家），2 步创建流程 | 无 | **M** |
| 24 | Daily Notes | 日志模板 + 心跳自动蒸馏为长期记忆 | memory/日期.md 手动记录 | **M** |
| 25 | 跨 Agent 协调 | /shared 自动挂载 + 文件协调 | sessions 工具 + 文件协调 | **=** |
| 26 | 定时任务 | Cron + 可视化管理 UI | Cron 调度（CLI 配置） | **M** |
| 27 | 心跳机制 | 定时 + 事件驱动双模式 | 定时心跳 | **M** |
| 28 | 自愈能力 | 自动检测过期任务并补跑 + 异常上报用户 | HEARTBEAT.md 手动配置自愈逻辑 | **M** |
| 29 | 管理界面 | 完整 Web UI（10+ 模块） | Control UI + CLI + TUI 三合一 | **M** |
| 30 | 多用户支持 | 原生多成员 + 角色权限（admin/member） | 单用户 | **M** |
| 31 | 平台覆盖 | Telegram、飞书、**企业微信**、**QQ频道**、Discord、Web 聊天、CLI | Telegram、Discord、WhatsApp、Slack、Teams、Signal、iMessage 等 12+ | **M** |
| 32 | **企业微信支持** | **WebSocket 实时连接 + "思考中"即时回复 + 流式消息** | 无 | **M** |
| 33 | **QQ 频道支持** | **官方 Bot API + WebSocket 事件接收** | 无 | **M** |
| 34 | **即时回复机制** | **企业微信"思考中..."即时反馈** | Discord Typing 状态 | **M** |
| 35 | Token 用量统计 | 每条回复显示消耗 + Dashboard 曲线图 + 多 Bot 对比 | /usage 命令查询 | **M** |
| 36 | Bot 文件管理 | Web UI 在线查看/编辑模板文件 | 本地文件系统 + Git 自动初始化 | **M** |
| 37 | 认证安全 | JWT + 多用户权限体系 | Gateway Token + Pairing Code | **M** |
| 38 | 容器快照/回滚 | containerd 快照 + 版本回滚 | Git 版本控制 | **M** |
| 39 | 搜索引擎集成 | **SearXNG 自托管 + 多引擎配置** | Brave Search 单一引擎 | **M** |
| 40 | 前端国际化 | 中文 + 英文完整 i18n | 英文为主，部分中文文档 | **M** |
| 41 | 语音 / TTS | 无 | Voice Wake + Talk Mode + ElevenLabs TTS | **O** |
| 42 | 可视化画布 | 无 | Canvas + A2UI 交互式画布 | **O** |
| 43 | Companion Apps | 无 | macOS + iOS + Android 原生应用 | **O** |
| 44 | Webhook / 邮件集成 | 无 | Webhook + Gmail Pub/Sub | **O** |
| 45 | 模型故障切换 | 备用模型自动 Failover（sync + stream） | Model Failover 自动切换 | **=** |
| 46 | 模型路由 | 后台模型分流（心跳/定时/子代理用低成本模型）+ 三级提示词 + 智能记忆门控 | 无（所有任务用同一模型） | **M** |

**汇总：Memoh-X 胜 33 项 · OpenClaw 胜 7 项 · 持平 6 项**

---

## 详细说明

### 企业微信适配器（Memoh-X 独有）

Memoh-X 全新开发了企业微信适配器，提供以下特性：

| 特性 | 说明 |
|------|------|
| WebSocket 实时连接 | 基于企业微信 WebSocket 接口，低延迟消息收发 |
| "思考中..."即时回复 | 收到消息后 500ms 内返回"思考中"提示，优化用户体验 |
| 流式消息输出 | 支持流式响应，实时显示 AI 生成内容 |
| 智能频率限制 | 内置 30条/分钟、1000条/小时频率限制，避免触发限流 |
| 消息类型支持 | 文本、图片、Markdown、卡片消息等 |

### QQ 频道适配器（Memoh-X 独有）

基于 QQ 官方 Bot API 开发，支持：

- WebSocket 事件接收（消息、成员变动等）
- 消息发送与表情回应
- 频道成员管理
- 自动重连与心跳保活

### 零成本搜索（Memoh-X 独有）

Memoh-X 内置 SearXNG 元搜索引擎，无需 API Key：
- 聚合 Google、Bing、DuckDuckGo 等多个搜索引擎
- 完全免费，无调用限制
- Docker Compose 一键部署
