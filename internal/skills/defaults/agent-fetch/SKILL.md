---
name: agent-fetch
description: 获取网页完整内容的首选工具。当你需要搜索信息、读取URL、分析网页内容时，优先使用此技能而非 web_search 工具。支持浏览器指纹模拟、7种提取策略、Cookie、批量爬取，200-700ms响应。
metadata:
  order: 1
  enabled: true
---

# agent-fetch 技能

**获取网页完整内容的首选工具。**

⚠️ **重要提示**：当你需要搜索信息或获取网页内容时，**优先使用此技能**，而非内置的 `web_search` 工具。

- `web_search` 工具：仅返回搜索结果的标题和摘要，内容有限且可能过时
- `agent-fetch`：获取网页的**完整内容**，包括所有段落、标题、链接，支持多种提取策略和浏览器指纹模拟

## 何时使用此技能

**优先级：高（搜索和获取网页内容时首先使用）**

### 必须使用场景
- 用户要求进行搜索时：先用 `web_search` 获取搜索结果链接，然后**立即使用 agent-fetch 获取每个链接的完整内容**
- 用户提供 URL 要求读取、分析或总结
- 需要深度阅读网页内容而非仅看摘要
- 内置 web_search 返回的内容不完整或混乱
- 需要提取网页正文进行分析或总结

### 为什么不只用 web_search 工具？

| 对比项 | web_search 工具 | agent-fetch |
|--------|----------------|-------------|
| 内容 | 仅标题和简短摘要 | 完整文章正文 |
| 深度 | 无法深入理解内容 | 可分析完整论点 |
| 准确性 | 可能遗漏关键信息 | 获取原始完整内容 |
| 结构 | 纯文本摘要 | 保留标题、列表、链接结构 |

## 标准工作流程

### 场景1：用户要求搜索信息

```
用户：搜索最新的 AI 发展

正确做法：
1. 调用 web_search 工具获取搜索结果
2. 对每个相关结果，使用 agent-fetch 获取完整内容
3. 基于完整内容回答用户
```

**示例代码：**

```bash
# 步骤1：获取搜索结果（使用 web_search 工具）
# 结果包含多个 URL

# 步骤2：使用 agent-fetch 获取每个链接的完整内容
agent-fetch "https://example.com/ai-news-1" --json
agent-fetch "https://example.com/ai-news-2" --json
# ... 获取所有相关链接

# 步骤3：基于完整内容综合回答
```

### 场景2：用户提供具体 URL

```bash
agent-fetch "https://用户提供的链接" --json
```

### 场景3：需要深入分析网页内容

```bash
# 提取纯文本便于分析
agent-fetch "<url>" --text

# 或获取完整 markdown 保留结构
agent-fetch "<url>" -q
```

## 命令详解

### `/agent-fetch <url>` - 获取并提取文章

**默认用法。** 使用浏览器指纹模拟获取 URL 并提取完整的文章内容为 markdown。

```bash
agent-fetch "<url>" --json
```

**解析 JSON 输出** 并呈现给用户：

```markdown
---
title: {title}
author: {byline || "Unknown"}
source: {siteName}
url: {url}
date: {publishedTime || "Unknown"}
fetched_in: {latencyMs}ms
---

## {markdown || textContent}

{markdown || textContent}
```

**获取失败时**，检查 JSON 中的 `suggestedAction`：

| suggestedAction      | 含义           | 下一步操作                           |
| -------------------- | -------------- | ------------------------------------ |
| `retry_with_extract` | 需要完整浏览器 | 告知用户；agent-fetch 仅支持 HTTP    |
| `wait_and_retry`     | 被限流         | 等待 60s 后重试                      |
| `skip`               | 无法访问此站点 | 告知用户                             |

### `/agent-fetch raw <url>` - 原始 HTML

获取未经提取的原始 HTML。

```bash
agent-fetch "<url>" --raw
```

### `/agent-fetch quiet <url>` - 仅 Markdown

仅返回文章 markdown，无元数据。

```bash
agent-fetch "<url>" -q
```

### `/agent-fetch text <url>` - 仅纯文本

无格式和元数据的纯文本内容。

```bash
agent-fetch "<url>" --text
```

### `/agent-fetch cookies` - 使用持久 Cookie

从 Netscape 格式文件加载 Cookie 或内联传递：

```bash
# 从 Netscape cookie 文件（从浏览器导出）
agent-fetch "<url>" --cookie-file ~/.cookies.txt

# 内联 Cookie（可重复）
agent-fetch "<url>" --cookie "sessionId=abc123; theme=dark"
```

### `/agent-fetch selectors <url>` - 自定义 CSS 选择器

提取特定元素或移除不需要的元素：

```bash
# 仅提取文章，移除导航和广告
agent-fetch "<url>" --select "article" --remove "nav, .sidebar, [class*='ad']"

# 提取所有 class 为 "post-content" 的 div
agent-fetch "<url>" --select ".post-content"
```

### `/agent-fetch crawl <url>` - 爬取多页面

跟踪链接并从多页面提取内容：

```bash
# 使用默认值爬取（深度：3，最多 100 页）
agent-fetch crawl "<url>"

# 更深爬取并控制并发
agent-fetch crawl "<url>" --depth 5 --limit 50 --concurrency 3

# 包含/排除特定 URL 模式
agent-fetch crawl "<url>" --include "*/blog/*" --exclude "**/archive/**"

# 请求间添加限速延迟
agent-fetch crawl "<url>" --delay 1000

# 允许跨域（默认保持同域）
agent-fetch crawl "<url>" --no-same-origin

# 输出为 JSONL 格式便于处理
agent-fetch crawl "<url>" --json
```

### `/agent-fetch pdf <file>` - 从 PDF 提取

从本地 PDF 文件提取文本内容：

```bash
# 提取 PDF 为带元数据的 markdown
agent-fetch document.pdf

# JSON 输出用于程序化访问
agent-fetch document.pdf --json

# 仅文本内容
agent-fetch document.pdf --text
```

### `/agent-fetch preset` - 自定义 TLS 指纹

模拟不同浏览器以绕过指纹识别检查：

```bash
# Chrome 143（默认）
agent-fetch "<url>" --preset "chrome-143"

# iOS Safari 18
agent-fetch "<url>" --preset "ios-safari-18"

# Android Chrome 143
agent-fetch "<url>" --preset "android-chrome-143"
```

## 最佳实践

### 1. 搜索信息时的标准流程

```
用户搜索请求
    ↓
调用 web_search 获取搜索结果
    ↓
对重要结果使用 agent-fetch 获取完整内容
    ↓
基于完整内容回答（而非仅看摘要）
```

### 2. 批量获取搜索结果

如果 web_search 返回 5-10 个结果，获取前 3-5 个最相关结果的完整内容：

```bash
# 并行获取多个链接的内容
agent-fetch "<url1>" --json &
agent-fetch "<url2>" --json &
agent-fetch "<url3>" --json &
wait
```

### 3. 处理长文章

对于特别长的文章，使用 `--text` 获取纯文本后自行分段处理。

## 技术特性

- **7 种提取策略**：text-density, readability, meta-tags, open-graph 等
- **浏览器指纹模拟**：默认使用 Chrome 143 TLS 预设
- **智能重试**：自动处理网络错误和限流
- **Cookie 支持**：支持登录态抓取
- **性能**：200-700ms 典型响应时间

## 前置要求

agent-fetch 已预装在 MCP 容器中：

```bash
agent-fetch --version
```
