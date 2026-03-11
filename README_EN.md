<div align="center">

# Memoh-v2

**Real Isolation · Real Memory · Real Evolution — The Uncompromising AI Agent Platform**

[![License](https://img.shields.io/badge/license-AGPL--v3-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D?logo=vuedotjs)](https://vuejs.org)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker)](https://docs.docker.com/compose/)

Others share a runtime — we give each bot its own containerd container.<br>
Others use SQLite vector search — we use Qdrant + BM25 + LLM triple extraction.<br>
Others edit memory files by hand — our bots reflect, experiment, and review on their own.

[中文](./README.md) · [Feature Guide](./doc/features_en.md) · [Installation](./doc/installation_en.md) · [Tutorials](./doc/README.md) · [Screenshots](./doc/screenshots.md)

</div>

---

## Quick Start

**Requires Docker:**

```bash
curl -fsSL https://raw.githubusercontent.com/91zgaoge/memoh-X/main/scripts/install.sh | sh
```

> Silent install (skip prompts): `curl -fsSL ... | sh -s -- -y`

Or manually:

```bash
git clone --depth 1 https://github.com/91zgaoge/memoh-X.git
cd Memoh-v2
docker compose up -d
```

Visit **http://localhost:8082**. Default login: `admin` / `admin123`

After installation, configure in this order:

```
1. Settings -> Provider    Add API provider, enter API Key and Base URL
       |
2. Provider -> Models      Add models (chat or embedding type)
       |
3. New Bot                 Select a template or start blank, set name and type
       |
4. Bot -> Settings         Choose Chat model, Embedding model, language, etc.
       |
5. Bot -> Channels         Connect Telegram / Lark messaging platforms (optional)
```

> For detailed installation, upgrade, uninstall, and data migration guides, see **[Installation & Upgrade](./doc/installation_en.md)**.

---

## Multi-Platform Channel Support

One Bot serving multiple platforms simultaneously with cross-platform message synchronization:

| Platform | DM | Group | @ Mention | Passive Sync | Special Features |
|----------|:--:|:-----:|:---------:|:------------:|------------------|
| **Telegram** | ✅ | ✅ | ✅ | ✅ | Typing indicator |
| **Lark (Feishu)** | ✅ | ✅ | ✅ | ✅ | Card messages |
| **WeCom** | ✅ | ✅ | ✅ | ✅ | **Instant "Thinking..." reply** |
| **QQ Channel** | ✅ | ✅ | ✅ | ✅ | Official Bot API |
| **Discord** | ✅ | ✅ | ✅ | ✅ | Streaming output |
| **Web Chat** | ✅ | — | — | — | Real-time streaming |
| **CLI** | ✅ | — | — | — | Local debugging |

- **Cross-platform Identity**: Users can bind the same identity across different platforms
- **Group Passive Sync**: Non-@ messages in groups are automatically stored in conversation history
- **Processing Status**: Platforms like Lark show "Thinking..." status feedback

---

## Architecture Overview

```
                    ┌──────────────┐
                    │   Web UI     │ :8082
                    │  Vue 3       │
                    └──────┬───────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
      ┌───────▼──────┐ ┌──▼──────────┐ │
      │   Server     │ │ Agent       │ │
      │   Go + Echo  │ │ Gateway     │ │
      │   :8080      │ │ Bun + Elysia│ │
      └──┬────┬──────┘ │ :8081       │ │
         │    │         └──┬──────────┘ │
         │    │            │            │
    ┌────▼┐ ┌─▼─────┐  ┌──▼──────────┐ │
    │ PG  │ │Qdrant │  │ Containerd  │◄┘
    │     │ │       │  │ (per-bot    │
    │     │ │       │  │  containers)│
    └─────┘ └───────┘  └─────────────┘
```

| Service | Responsibility |
|---|---|
| **Server** (Go) | REST API, auth, database, container management, conversation routing, memory retrieval |
| **Agent Gateway** (Bun) | AI inference, system prompt assembly, tool execution, streaming, subagent dispatch |
| **Web** (Vue 3) | Management UI: bots, models, channels, skills, files, evolution, heartbeat visualization |
| **PostgreSQL** | Relational data (users, bots, messages, configs, evolution logs) |
| **Qdrant** | Vector database (memory semantic search) |
| **Containerd** | Container runtime (one isolated container per bot) |

---

## Screenshot Preview

<p align="center">
  <img src="./doc/1.png" width="100%" />
</p>

<p align="center">
  <a href="./doc/screenshots.md">👉 Click for more screenshots</a>
</p>

---

## Core Features

> Full details for each feature in the **[Feature Guide](./doc/features_en.md)**.

- **Bot Management & Templates** — 13 mental-model templates (10 real thought-leaders), two-step professional bot creation
- **Conversation & Streaming** — SSE real-time streaming + synchronous modes, automatic context management and memory recall
- **Three-Layer Memory** — Vector semantic search + BM25 keywords + LLM smart extraction, auto-indexed after each turn
- **Isolated Container Sandbox** — Each bot gets a containerd container with file I/O, shell, browser, and snapshot rollback
- **Multi-Platform Channels** — Telegram / Lark / WeCom / QQ / Discord / Web chat / CLI with cross-platform identity unification
- **MCP Tool System** — 15 built-in tools + any external MCP server, Stdio and Remote transport
- **Heartbeat & Scheduling** — Periodic + event-driven dual-mode triggers; bots act proactively
- **Self-Evolution** — Three-phase organic cycle (Reflect / Experiment / Review) with full evolution log tracking
- **Subagents & Skills** — Auto-dispatched subagents, one-click skill install from ClawHub marketplace
- **OpenViking Tiered Context** — L0/L1/L2 structured memory layers, dramatically reducing token consumption
- **Token Usage & Diagnostics** — Dashboard charts + multi-bot comparison, one-click service health check
- **Cross-Bot Collaboration** — `/shared` workspace with file-based coordination

---

## Documentation

| Document | Description |
|----------|-------------|
| **[Feature Guide](./doc/features_en.md)** | Full details on all 12 core features |
| **[Concepts Guide](./doc/concepts_en.md)** | Model types, persona system, Provider configuration |
| **[Installation & Upgrade](./doc/installation_en.md)** | Install / upgrade / uninstall / data migration |
| **[Known Limitations](./doc/known-limitations_en.md)** | Current shortcomings and workarounds |
| **[OpenClaw Comparison](./doc/comparison_en.md)** | 42-item comprehensive comparison |
| **[Tutorials](./doc/README.md)** | 18 step-by-step tutorials (quick start to advanced tips) |
| **[Screenshots](./doc/screenshots.md)** | More UI screenshots |
| **[Feature Audit](./doc/FEATURE_AUDIT.md)** | 74-item feature audit |
| **[Prompts Inventory](./doc/PROMPTS_INVENTORY.md)** | Complete prompts reference |

---

## Tech Stack

| Service | Stack | Port |
|---|---|---|
| Server (Backend) | Go + Echo + Uber FX + pgx/v5 + sqlc | 8080 |
| Agent Gateway | Bun + Elysia + Vercel AI SDK | 8081 |
| Web (Frontend) | Vue 3 + Vite + Tailwind CSS + Pinia | 8082 |

| Dependency | Version | Purpose |
|-----------|---------|---------|
| PostgreSQL | 18 | Relational data storage |
| Qdrant | latest | Vector database |
| Containerd | v2 | Container runtime |

---

## 🚀 Secondary Development Features

This project extends [Memoh](https://github.com/memohai/Memoh) with significant new features:

### 🏢 WeCom (Enterprise WeChat) Adapter (New)

- **WebSocket Real-time Connection**: Based on WeCom WebSocket API for low-latency messaging
- **"Thinking..." Instant Reply**: Immediately sends "Thinking..." message upon receiving user input for better UX
- **Streaming Message Output**: Supports streaming responses for real-time AI content generation
- **Smart Rate Limiting**: Built-in 30 msg/min and 1000 msg/hour rate limiting to avoid WeCom API limits
- **Rich Message Types**: Supports text, images, Markdown, cards, and more

### 🐧 QQ Channel Adapter

- **Official Bot API Support**: Based on QQ's official Bot API
- **WebSocket Event Reception**: Real-time channel messages, member changes, and other events
- **Rich Interaction Capabilities**: Message sending, emoji reactions, member management
- **Robust Error Handling**: Includes reconnection mechanism, heartbeat keepalive, and recovery

### 🔧 System Stability Enhancements

- **DNS Resolution Fix**: Fixed DNS configuration issues in container environments using systemd-resolved upstream DNS
- **Discord Adapter Fixes**: Fixed image message handling and streaming output issues
- **Model List Incremental Rendering**: Optimized model list loading with incremental rendering support

---

## Acknowledgments

This project is a secondary development based on [Memoh](https://github.com/memohai/Memoh). Thanks to the original authors for their excellent work.
