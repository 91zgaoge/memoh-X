# 修复模型上下文窗口配置

## 问题

扩展 llama-server 到 52.4万上下文后，仍然报错：
```
request (288548 tokens) exceeds the available context size (262144 tokens)
```

## 根本原因

数据库中 `models` 表的 `context_window` 字段值不正确：

```sql
-- 修改前
SELECT name, model_id, context_window FROM models;
-- Qwen3.5-35B-A3B | Qwen3.5-35B-A3B-UD-Q4_K_XL.gguf | 128000  (错误！)
```

Memoh Server 从数据库读取上下文限制，而不是从 llama-server 获取。

## 修复

```sql
UPDATE models
SET context_window = 524288
WHERE model_id = 'Qwen3.5-35B-A3B-UD-Q4_K_XL.gguf';
```

## 验证

```sql
SELECT name, model_id, context_window FROM models;
-- Qwen3.5-35B-A3B | Qwen3.5-35B-A3B-UD-Q4_K_XL.gguf | 524288  (正确)
```

## 重启服务

```bash
docker compose restart server
```

## 完整配置 checklist

| 组件 | 配置项 | 值 |
|-----|--------|-----|
| llama-server | `-c` | 524288 |
| llama-server | `--cache-type-k` | q4_0 |
| llama-server | `--cache-type-v` | q4_0 |
| 数据库 | `context_window` | 524288 |
| Memoh | `maxTotalTokens` | 250000 |
| 图片压缩 | `maxDimension` | 1024px |
| 图片压缩 | `jpegQuality` | 85% |
