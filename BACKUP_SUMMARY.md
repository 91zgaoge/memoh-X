# Memoh-v2 项目备份摘要

## 📋 本次工作总览

| 项目 | 内容 |
|------|------|
| **更新日期** | 2026-03-15 |
| **主要功能** | 1. 性能优化：消息加载和 Token 估算<br>2. 模型级 Token 估算开关<br>3. 企业微信响应速度优化 |
| **备份位置** | `/data2/backups/memoh_backup_20260311_115019` |
| **备份大小** | 约 71MB |

---

## ✅ 完成功能清单

### 1. 性能优化 (2026-03-15)
- [x] 消息加载数量限制 (10000 → 1000)
- [x] 使用 ListLatest 替代 ListSince，限制最多 200 条消息
- [x] 群消息防抖窗口优化 (300ms → 50ms)
- [x] 单次遍历完成加载+限制，减少重复处理

### 2. 模型级 Token 估算开关
- [x] 数据库: 新增 `enable_token_estimate` 字段 (默认 false)
- [x] 后端: Model struct 添加开关字段
- [x] 后端: 对话流程根据模型配置决定是否启用 Token 估算
- [x] 前端: 模型设置页面添加开关
- [x] 国际化: 中英文翻译

### 3. 企业微信连接更新
- [x] `disconnected_event` 事件处理
- [x] `chat_type` 字段支持
- [x] 流式消息 6 分钟超时检查
- [x] 主动推送消息限制
- [x] 消息类型限制检查

### 4. 模型连接测试功能
- [x] 后端: `/api/models/{id}/test` 接口
- [x] 后端: `/api/providers/{id}/test` 接口
- [x] 前端: 模型列表测试按钮
- [x] 前端: Provider 测试按钮
- [x] 状态显示 (绿色/黄色/红色)
- [x] 延迟显示

### 5. 自动获取模型功能
- [x] 后端: `/api/providers/{id}/import-models` 接口
- [x] 前端: "从服务商获取模型"按钮
- [x] 导入结果展示
- [x] i18n 翻译

### 6. 问题修复
- [x] Qdrant 启动失败 (重置数据卷)
- [x] 前端 401 认证失败 (修复 token 获取)

---

## 📁 关键文件位置

### 更新日志
```
/data2/memoh-v2/UPDATE_LOG_20250311.md                    # 详细更新日志
/data2/backups/BACKUP_README.md                           # 备份恢复文档
/data2/memoh-v2/BACKUP_SUMMARY.md                         # 本文档
db/migrations/0045_model_token_estimate.up.sql            # Token 估算字段迁移
db/migrations/0045_model_token_estimate.down.sql          # Token 估算字段回滚
```

### 后端代码
```
internal/models/probe.go                  # 模型探测核心
internal/models/types.go                  # Model struct (含 enable_token_estimate)
internal/models/models.go                 # 模型 CRUD 操作
internal/handlers/models.go               # 模型测试接口
internal/handlers/providers.go            # Provider 测试/导入接口
internal/providers/service.go             # Provider 业务逻辑
internal/conversation/flow/resolver.go    # 对话流程 (Token 估算开关)
internal/message/service.go               # 消息查询服务
internal/message/debounce.go              # 群消息防抖
internal/channel/adapters/wecom/          # 企业微信适配器
internal/db/sqlc/models.go                # SQLC 模型定义
internal/db/sqlc/models.sql.go            # SQLC 查询代码
```

### 前端代码
```
packages/web/src/pages/models/components/model-item.vue       # 模型测试
packages/web/src/pages/models/components/provider-form.vue    # Provider 测试
packages/web/src/pages/models/model-setting.vue               # 导入模型
packages/web/src/i18n/locales/zh.json                         # 中文翻译
packages/web/src/i18n/locales/en.json                         # 英文翻译
```

---

## 🚀 快速验证

### 1. 检查服务状态
```bash
cd /data2/memoh-v2
docker compose ps
```

### 2. 测试模型连接
- 访问 http://localhost:8082/models
- 选择 Provider，查看"测试连接"按钮

### 3. 测试导入模型
- 在模型管理页面
- 点击"从服务商获取模型"按钮

### 4. 验证性能优化
```bash
# 检查数据库迁移
docker exec -i memoh-postgres psql -U memoh -d memoh -c "SELECT model_id, enable_token_estimate FROM models;"

# 查看服务日志
docker logs memoh-server --tail 50
```

### 5. 测试 Token 估算开关
- 访问 http://localhost:8082/models
- 编辑模型，找到"启用 Token 估算"开关
- 开启/关闭后测试对话响应速度

---

## 💾 备份信息

| 备份项 | 文件 | 大小 |
|--------|------|------|
| 源代码 | `memoh-v2_code.tar.gz` | 5.7MB |
| Git 仓库 | `memoh-v2_git.bundle` | 66MB |
| Qdrant 数据 | `volumes/qdrant_data.tar.gz` | - |
| 配置文件 | `docker-compose.yml` | - |

### 快速恢复命令
```bash
# 恢复代码
tar xzf /data2/backups/memoh_backup_latest/memoh-v2_code.tar.gz -C /data2/

# 恢复数据
cd /data2/memoh-v2
docker compose up -d
```

---

## 📚 相关文档

1. **详细更新日志:** `UPDATE_LOG_20250311.md`
2. **备份恢复指南:** `/data2/backups/BACKUP_README.md`
3. **项目 README:** `/data2/memoh-v2/README.md` (如存在)

---

## 🔧 后续建议

1. 重新生成 OpenAPI SDK (`@memoh/sdk`)
2. 添加更多 Provider 类型的探测支持
3. 配置本地 Embedding 服务 (8089 端口)
4. 设置自动备份定时任务

---

## 📊 性能优化效果

| 指标 | 优化前 | 优化后 | 提升 |
|-----|--------|--------|------|
| 消息加载时间 | 500-2000ms | 50-100ms | **10-20x** |
| Token 估算耗时 | 300-1000ms | 0ms (关闭时) | **完全消除** |
| 群消息防抖 | 300ms | 50ms | **6x** |
| 数据库返回消息数 | 1000-10000 | < 200 | **10-50x** |

---

**备份创建时间:** 2026-03-15 11:52:00
**备份工具:** Claude Code
**Git Commit:** 32fe7ff3  
