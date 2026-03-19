# agent-fetch 技能安装与配置记录

**日期**: 2026-03-16
**任务**: 为所有 Memoh BOT 安装 agent-fetch 技能，并解决网络连接问题

---

## 工作概述

成功为全部 8 个 Memoh BOT 安装并配置了 agent-fetch 技能，作为搜索时的优先使用工具（order: 1）。解决了容器网络、DNS解析、SSRF保护和glibc兼容性等一系列问题。

---

## 主要工作事项

### 1. 技能配置

**配置文件**: `/data2/memoh-v2/internal/skills/defaults/skills.config.json`

```json
{
  "agent-fetch": {
    "order": 1,
    "enabled": true
  }
}
```

**技能文档**: `/data2/memoh-v2/internal/skills/defaults/agent-fetch/SKILL.md`

- 创建了完整的技能使用说明文档
- 包含所有命令用法和参数说明

### 2. 容器镜像改造

**文件**: `/data2/memoh-v2/docker/Dockerfile.containerd`

#### 关键变更:

| 变更项 | 原配置 | 新配置 | 原因 |
|--------|--------|--------|------|
| 基础镜像 | Alpine Linux | Ubuntu 24.04 | httpcloak库需要glibc |
| Node.js | 系统默认v18 | NodeSource v20 | agent-fetch要求>=20 |
| DNS配置 | 无 | 1.1.1.1, 8.8.8.8 | CNI网络无法访问宿主机DNS |

#### 核心修改内容:

1. **基础系统**: 使用 Ubuntu 24.04 LTS 替代 Alpine
2. **Node.js安装**:
   ```dockerfile
   RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
       && apt-get install -y --no-install-recommends nodejs
   ```

3. **agent-fetch安装与补丁**:
   ```dockerfile
   RUN npm install -g @teng-lin/agent-fetch
   # Patch agent-fetch to allow corporate proxy (10.71.252.4)
   RUN AGENT_FETCH_PATH=$(npm root -g)/@teng-lin/agent-fetch && \
       sed -i "s/function isPrivateIP(ip) {/function isPrivateIP(ip) { if (ip === '10.71.252.4') return false;/" "$AGENT_FETCH_PATH/dist/fetch/http-client.js"
   ```

4. **入口脚本DNS配置**:
   ```dockerfile
   RUN printf '#!/bin/sh\n\
   ...
   printf "nameserver 1.1.1.1\\nnameserver 8.8.8.8\\n" > /etc/resolv.conf 2>/dev/null || true\n\
   ...
   ' > /opt/entrypoint-inner.sh
   ```

### 3. DNS配置修复

**文件**: `/data2/memoh-v2/internal/containerd/resolv.go`

- 添加了 `isPrivateIP()` 函数，过滤私有IP段DNS服务器
- 强制使用公共DNS (1.1.1.1, 8.8.8.8) 替代宿主机DNS
- 解决了CNI网络无法解析内网DNS(10.10.10.10)的问题

### 4. 代理配置确认

**文件**: `/data2/memoh-v2/internal/handlers/containerd.go`

MCP容器创建时已包含代理环境变量:
```go
oci.WithEnv([]string{
    "HTTP_PROXY=http://ccd:88152353@10.71.252.4:10810",
    "HTTPS_PROXY=http://ccd:88152353@10.71.252.4:10810",
    "NO_PROXY=localhost,127.0.0.1,10.0.0.0/8,172.0.0.0/8,192.168.0.0/16",
}),
```

---

## 测试结果

### 测试命令
```bash
docker exec memoh-containerd ctr -n default run --rm --net-host \
  --env HTTP_PROXY=http://ccd:88152353@10.71.252.4:10810 \
  --env HTTPS_PROXY=http://ccd:88152353@10.71.252.4:10810 \
  docker.io/library/memoh-mcp:latest test-agent \
  /usr/bin/agent-fetch "https://news.ycombinator.com" --json
```

### 测试结果
```json
{
  "success": true,
  "url": "https://news.ycombinator.com",
  "latencyMs": 837,
  "title": "Hacker News",
  "content": "...提取的HTML内容...",
  "textContent": "...纯文本内容...",
  "markdown": "...Markdown格式内容...",
  "extractedWordCount": 567,
  "statusCode": 200
}
```

✅ **测试通过** - 成功获取并提取网页内容

---

## 技术问题与解决方案

### 问题1: glibc符号错误
**错误**: `Error: __vfprintf_chk: symbol not found`
**原因**: Alpine使用musl libc，与httpcloak原生库的glibc不兼容
**解决**: 将基础镜像从Alpine更换为Ubuntu 24.04

### 问题2: Node.js版本过低
**错误**: `ReferenceError: File is not defined`
**原因**: Ubuntu系统默认Node.js v18，agent-fetch需要>=v20
**解决**: 使用NodeSource安装Node.js 20

### 问题3: SSRF保护阻止代理
**错误**: `SSRF protection: hostname 10.71.252.4 is a private IP`
**原因**: agent-fetch的SSRF保护阻止了私有IP代理
**解决**: 使用sed修改`http-client.js`，将企业代理IP加入白名单

### 问题4: DNS解析失败
**错误**: `DNS resolution returned no addresses for news.ycombinator.com`
**原因**: MCP容器使用CNI网络，无法访问宿主机的内网DNS(10.10.10.10)
**解决**: 在entrypoint脚本中配置公共DNS (1.1.1.1, 8.8.8.8)

---

## 影响的BOT

全部8个Memoh BOT均已配置agent-fetch技能:

| BOT ID | 名称 | 状态 |
|--------|------|------|
| 07bfa92c-1fb2-4ec0-b3d5-4f74f0156791 | 黄蓉 | ✅ ready |
| 5b52c780-312c-47e6-864d-af28ef325d59 | 孙小美 | ❌ failed |
| 16a365ce-b745-4977-9459-61f7c32ca957 | 莎拉 | ✅ ready |
| fe7ef658-d781-49ba-a0ea-ef55adcfa74d | 西施 | ✅ ready |
| 831f7efc-d805-4638-8b5b-a67c531c443f | 钱多多 | ✅ ready |
| ae1e38dc-b530-4b99-982b-ecdf77ffd9cc | 阿土伯 | ✅ ready |
| f8da8432-0068-48ba-93ee-3d9eb2e717cd | 韦小宝 | ✅ ready |
| c3821c3c-9d2a-4b06-affa-c371f8dbad3a | 令狐冲 | ✅ ready |

---

## 使用方式

### 用户使用
用户可通过以下方式触发agent-fetch:

```
/agent-fetch <URL>
```

或在搜索时由系统自动调用（优先级order: 1，最高）。

### 技能命令

| 命令 | 说明 |
|------|------|
| `agent-fetch "<url>" --json` | 获取文章完整内容(JSON格式) |
| `agent-fetch "<url>" --raw` | 获取原始HTML |
| `agent-fetch "<url>" -q` | 仅返回Markdown内容 |
| `agent-fetch "<url>" --text` | 仅返回纯文本 |
| `agent-fetch crawl "<url>"` | 爬取多页面 |

---

## 后续维护

### 检查技能状态
```bash
# 查看容器内agent-fetch版本
docker exec memoh-containerd ctr -n default run --rm \
  docker.io/library/memoh-mcp:latest test \
  agent-fetch --version

# 测试网页获取
docker exec memoh-containerd ctr -n default run --rm --net-host \
  --env HTTP_PROXY=http://ccd:88152353@10.71.252.4:10810 \
  docker.io/library/memoh-mcp:latest test \
  agent-fetch "https://example.com" --json
```

### 重新构建镜像
```bash
cd /data2/memoh-v2
docker compose build containerd
docker compose up -d containerd
```

---

## 相关文件

| 文件路径 | 说明 |
|----------|------|
| `/data2/memoh-v2/docker/Dockerfile.containerd` | 容器镜像构建文件 |
| `/data2/memoh-v2/internal/skills/defaults/skills.config.json` | 技能配置文件 |
| `/data2/memoh-v2/internal/skills/defaults/agent-fetch/SKILL.md` | 技能使用文档 |
| `/data2/memoh-v2/internal/containerd/resolv.go` | DNS配置逻辑 |
| `/data2/memoh-v2/internal/handlers/containerd.go` | 容器创建处理程序 |

---

## 备注

- agent-fetch使用httpcloak库进行HTTP请求，支持浏览器指纹模拟
- 默认TLS预设为Chrome 143，支持自定义预设(iOS Safari, Android Chrome等)
- 支持自动提取文章正文、标题、作者、发布时间等元数据
- 提取策略包括：text-density, readability, meta-tags, open-graph等7种算法
