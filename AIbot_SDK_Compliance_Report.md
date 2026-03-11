# AIbot SDK 技术合规性审查报告

## 审查日期
2026-03-10

## SDK 版本
@wecom/aibot-node-sdk (TypeScript)

---

## 1. 类型定义对比

### 1.1 FileContent 文件内容结构

| 字段 | SDK 定义 | 当前实现 | 差异 |
|------|---------|---------|------|
| `url` | ✅ string | ✅ string | 一致 |
| `aeskey` | ✅ string (可选) | ✅ string (可选) | 一致 |
| `filename` | ❌ 无此字段 | ⚠️ 有 `FileName` 字段 | **扩展** |

**说明**：虽然 SDK 类型定义中没有 `filename`，但实际 WeCom 回调可能会提供此字段。当前实现保留此字段作为扩展，这是合理的。

### 1.2 MixedMsgItem 图文混排项

| SDK 定义 | 当前实现 | 差异 |
|---------|---------|------|
| 仅支持 `text` 和 `image` | 支持 `text`, `image`, `file` | **扩展** |
| `msgtype: 'text' \| 'image'` | `msgtype: string` | **更宽松** |

**建议**：图文混排消息中确实不应该包含文件类型（这是 WeCom 协议的限制），但当前实现作为扩展功能保留也无妨。

### 1.3 QuoteContent 引用消息

| SDK 支持 | 当前实现 | 状态 |
|---------|---------|------|
| text, image, mixed, voice, file | text, image, mixed, voice, file | ✅ 一致 |

---

## 2. 文件下载处理对比

### 2.1 文件名获取方式

| 方式 | SDK 实现 | 当前实现 | 建议 |
|------|---------|---------|------|
| HTTP Content-Disposition 头 | ✅ 支持 RFC5987 解码 | ❌ 未实现 | **应添加** |
| URL 路径提取 | ❌ 未使用 | ✅ 已实现 | 作为备用 |
| 消息体 filename 字段 | ⚠️ SDK 类型无此字段 | ✅ 已使用 | 合理扩展 |

**SDK 代码参考**：
```typescript
// 从 Content-Disposition 头中解析文件名
const utf8Match = contentDisposition.match(/filename\*=UTF-8''([^;\s]+)/i);
if (utf8Match) {
  filename = decodeURIComponent(utf8Match[1]);
} else {
  const match = contentDisposition.match(/filename="?([^";\s]+)"?/i);
  if (match) {
    filename = decodeURIComponent(match[1]);
  }
}
```

### 2.2 文件解密实现

| 项目 | SDK (Node.js) | 当前实现 (Go) | 状态 |
|------|---------------|---------------|------|
| 算法 | AES-256-CBC | AES-256-CBC | ✅ 一致 |
| IV 来源 | aesKey 解码后前 16 字节 | aesKey 解码后前 16 字节 | ✅ 一致 |
| Padding | 手动去除 PKCS#7 | 手动去除 PKCS#7 | ✅ 一致 |
| Base64 解码 | 标准 Base64 | 支持多种 Base64 变体 | ✅ 更健壮 |

---

## 3. 消息处理对比

### 3.1 消息类型支持

| 消息类型 | SDK 支持 | 当前实现 | 状态 |
|---------|---------|---------|------|
| text | ✅ | ✅ | 已实现 |
| image | ✅ | ✅ | 已实现 |
| mixed | ✅ | ✅ | 已实现 |
| voice | ✅ | ✅ | 已实现 |
| file | ✅ | ✅ | 已实现 |
| event | ✅ | ✅ | 已实现 |

### 3.2 事件类型支持

| 事件类型 | SDK 定义 | 当前实现 | 状态 |
|---------|---------|---------|------|
| enter_chat | ✅ | ✅ | 已处理 |
| template_card_event | ✅ | ⚠️ | 仅记录日志 |
| feedback_event | ✅ | ⚠️ | 仅记录日志 |

---

## 4. 待完善功能清单

### 🔴 高优先级

#### 4.1 从 HTTP 头获取文件名 ✅ 已完成

**文件**: `internal/channel/adapters/wecom/adapter.go`

**修改内容**：
- 添加 `DownloadResult` 结构体包含文件名信息
- 修改 `downloadAndDecrypt` 函数返回 `*DownloadResult`
- 新增 `extractFileNameFromHeaders` 函数实现 RFC5987 解码

**实现细节**：
1. 优先从 `Content-Disposition` 头提取文件名（RFC5987 UTF-8 编码）
2. 支持 `filename*=UTF-8''xxx` 格式
3. 支持 `filename="xxx"` 或 `filename=xxx` 格式
4. 如果头信息中没有文件名，回退到 URL 提取

**优先级顺序**：
1. HTTP `Content-Disposition` 头（SDK 标准方式）
2. URL 路径提取（当前实现的备用方式）
3. 消息体中的 `filename` 字段（扩展方式）

### 🟡 中优先级

#### 4.2 引用消息处理

**文件**: `internal/channel/adapters/wecom/adapter.go`

**现状**：`QuoteContent` 结构已定义，但 `handleCallback` 中未处理引用消息内容。

**建议**：在构建消息时，如果存在 `Quote` 字段，应将引用内容附加到消息文本中。

#### 4.3 模板卡片事件处理

**文件**: `internal/channel/adapters/wecom/adapter.go`

**现状**：仅记录日志，未实际处理。

**建议**：根据业务需求实现卡片点击事件的处理逻辑。

### 🟢 低优先级

#### 4.4 文件下载超时配置

**SDK 默认**: 10 秒

**建议**: 添加可配置的文件下载超时时间。

---

## 5. 已验证功能

### ✅ 文件类型检测（Magic Number）

当前实现已超越 SDK 基础功能，支持通过文件内容检测类型：

| 文件类型 | Magic Number | 状态 |
|---------|---------------|------|
| PDF | `%PDF` | ✅ 已支持 |
| ZIP (DOCX/XLSX/PPTX) | `PK\x03\x04` | ✅ 已支持 |
| 纯文本 | 可打印 ASCII | ✅ 已支持 |

### ✅ 文件内容提取

| 格式 | 状态 |
|------|------|
| PDF | ✅ 支持 |
| DOCX | ✅ 支持 |
| XLSX | ✅ 支持 |
| TXT/MD/JSON/CSV | ✅ 支持 |

---

## 6. 总结

### 符合 SDK 规范的实现

1. ✅ WebSocket 连接管理
2. ✅ 消息类型定义
3. ✅ 事件类型定义
4. ✅ AES-256-CBC 解密
5. ✅ 文件下载和处理流程
6. ✅ 流式回复机制
7. ✅ HTTP Content-Disposition 头文件名解析 (RFC5987)

### 需要完善的功能

1. ✅ **已完成**: 从 HTTP `Content-Disposition` 头获取文件名
2. 🟡 **中优**: 引用消息内容处理
3. 🟡 **中优**: 模板卡片事件完整处理
4. 🟢 **低优**: 文件下载超时配置

### 当前实现的扩展（非 SDK 标准但合理）

1. 文件内容自动检测（Magic Number）
2. 文件内容提取（PDF/DOCX/XLSX）
3. 图文混排中的文件类型支持
4. 消息体中的 `FileName` 字段

---

## 7. 下一步行动建议

1. **立即实施**: 添加 HTTP 头文件名解析功能
2. **短期**: 完善引用消息处理
3. **中期**: 根据业务需求完善事件处理
4. **长期**: 考虑贡献文件内容检测功能回 SDK（如果是开源项目）
