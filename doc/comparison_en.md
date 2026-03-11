# Comprehensive Comparison with OpenClaw (46 Items)

> Back to [Documentation](./README.md) · [Project Home](../README_EN.md)

---

> Result column: **M** = Memoh-X wins · **O** = OpenClaw wins · **=** = Tied

| # | Dimension | Memoh-X | OpenClaw | Result |
|---|---|---|---|:---:|
| 1 | Backend Language | Go (high concurrency, compiled) | Node.js (single-threaded, interpreted) | **M** |
| 2 | Architecture | Three-service split (Server / Gateway / Web) | Monolithic application | **M** |
| 3 | Communication | SSE unidirectional streaming | WebSocket full-duplex | **O** |
| 4 | Container Isolation | containerd isolated container per bot | Shared runtime (optional Docker sandbox) | **M** |
| 5 | Structured Database | PostgreSQL | SQLite | **M** |
| 6 | Vector Database | Qdrant (standalone service) | SQLite-vec (embedded) | **M** |
| 7 | Horizontal Scaling | Services deploy and scale independently | Single-machine only | **M** |
| 8 | Resource Usage | Requires Docker + PostgreSQL + Qdrant | Lightweight single process, ~tens of MB RAM | **O** |
| 9 | Deployment | Docker Compose one-click | npm install -g + CLI start | **=** |
| 10 | Remote Access | Native (Docker deploys to any server) | Requires Tailscale / SSH tunnel | **M** |
| 11 | Agent Definition | SOUL + IDENTITY + TOOLS + EXPERIMENTS + NOTES | SOUL + IDENTITY + TOOLS + AGENTS + HEARTBEAT + BOOTSTRAP + USER | **=** |
| 12 | Sub-Agent Management | spawn/kill/steer + independent tool perms + registry | spawn/kill/steer + depth limit + max children | **=** |
| 13 | Tool Execution | MCP protocol (sandboxed in container) | Pi Runtime built-in (Browser/Canvas/Nodes) | **O** |
| 14 | MCP Protocol | Native, connects to any MCP Server | Limited + ACP protocol | **M** |
| 15 | Browser Automation | Chromium + agent-browser + Actionbook + xvfb | Built-in Browser + agent-browser + Actionbook | **=** |
| 16 | Smart Web Strategy | Markdown Header → Actionbook → curl 3-tier fallback | Standard fetching | **M** |
| 17 | Skill Marketplace | ClawHub + OPC Skills | ClawHub + OPC Skills | **=** |
| 18 | Short-term Memory | Last 24h auto-loaded | Current session only | **M** |
| 19 | Long-term Memory | Qdrant vector semantic + BM25 keyword, auto-indexed per turn | SQLite-vec vector + memoryFlush | **M** |
| 20 | Context Compression | Token-budget pruning + LLM auto-summarization | /compact manual compression | **M** |
| 21 | Tiered Context | OpenViking (L0/L1/L2), toggleable per bot | None | **M** |
| 22 | Self-Evolution | Three-phase organic cycle (Reflect/Experiment/Review) + evolution log tracking | MEMORY.md manual iteration | **M** |
| 23 | Bot Templates | 13 mental-model templates (10 real thought-leaders), 2-step creation | None | **M** |
| 24 | Daily Notes | Template + heartbeat auto-distillation to long-term memory | memory/date.md manual logging | **M** |
| 25 | Cross-Agent Coordination | /shared auto-mounted + file coordination | sessions tools + file coordination | **=** |
| 26 | Scheduled Tasks | Cron + visual management UI | Cron scheduling (CLI config) | **M** |
| 27 | Heartbeat | Periodic + event-driven dual mode | Periodic heartbeat | **M** |
| 28 | Self-Healing | Auto-detect stale tasks + force re-run + report to user | HEARTBEAT.md manual config | **M** |
| 29 | Management UI | Full Web UI (10+ modules) | Control UI + CLI + TUI combo | **M** |
| 30 | Multi-User | Native multi-member + role permissions (admin/member) | Single-user | **M** |
| 31 | Platform Coverage | Telegram, Lark, **WeCom**, **QQ Channel**, Discord, Web chat, CLI | Telegram, Discord, WhatsApp, Slack, Teams, Signal, iMessage, etc. 12+ | **M** |
| 32 | **WeCom Support** | **WebSocket real-time + "Thinking..." instant reply + streaming** | None | **M** |
| 33 | **QQ Channel Support** | **Official Bot API + WebSocket events** | None | **M** |
| 34 | **Instant Reply Mechanism** | **WeCom "Thinking..." immediate feedback** | Discord Typing indicator | **M** |
| 35 | Token Usage | Per-response display + Dashboard charts + multi-bot comparison | /usage command query | **M** |
| 36 | Bot File Management | Web UI online view/edit | Local filesystem + Git auto-init | **M** |
| 37 | Auth Security | JWT + multi-user permission system | Gateway Token + Pairing Code | **M** |
| 38 | Snapshots / Rollback | containerd snapshots + version rollback | Git version control | **M** |
| 39 | Search Engine | **SearXNG self-hosted + multi-engine config** | Brave Search only | **M** |
| 40 | Frontend i18n | Full Chinese + English i18n | English primary, partial Chinese docs | **M** |
| 41 | Voice / TTS | None | Voice Wake + Talk Mode + ElevenLabs TTS | **O** |
| 42 | Visual Canvas | None | Canvas + A2UI interactive workspace | **O** |
| 43 | Companion Apps | None | macOS + iOS + Android native apps | **O** |
| 44 | Webhook / Email | None | Webhook + Gmail Pub/Sub | **O** |
| 45 | Model Failover | Fallback model auto-failover (sync + stream) | Automatic model failover | **=** |
| 46 | Model Routing | Background model routing (heartbeat/schedule/subagent use cheaper model) + 3-tier prompts + smart memory gate | None (single model for all tasks) | **M** |

**Summary: Memoh-X wins 33 · OpenClaw wins 7 · Tied 6**

---

## Detailed Explanations

### WeCom (Enterprise WeChat) Adapter (Memoh-X Exclusive)

Memoh-X features a newly developed WeCom adapter with the following capabilities:

| Feature | Description |
|---------|-------------|
| WebSocket Real-time | Based on WeCom WebSocket API for low-latency messaging |
| "Thinking..." Instant Reply | Returns "Thinking..." within 500ms after receiving message for better UX |
| Streaming Output | Supports streaming responses for real-time AI content generation |
| Smart Rate Limiting | Built-in 30 msg/min and 1000 msg/hour limits to avoid API throttling |
| Message Types | Text, images, Markdown, cards, and more |

### QQ Channel Adapter (Memoh-X Exclusive)

Based on QQ's official Bot API, supporting:

- WebSocket event reception (messages, member changes, etc.)
- Message sending and emoji reactions
- Channel member management
- Auto-reconnection and heartbeat keepalive

### Zero-Cost Search (Memoh-X Exclusive)

Memoh-X includes SearXNG meta-search engine, no API Key required:
- Aggregates Google, Bing, DuckDuckGo, and more
- Completely free, no usage limits
- Docker Compose one-click deployment
