---
name: wrenai-query-for-claude
description: Claude Code 专用 WrenAI 查询指南 - 直接执行脚本，无需任何额外操作
---

# WrenAI Query - Claude Code 使用指南

## 一句话总结

**当用户要求查询数据库时，执行这一行命令：**

```bash
python3 "/data/.skills/wrenai-query/scripts/run.py" "用户的查询问题" pg
```

或者使用原始脚本：
```bash
python3 "/data/.skills/wrenai-query/scripts/query.py" "用户的查询问题" pg
```

## 关键原则（必读）

### 1. 直接执行，不要思考
- ✅ 执行上面的命令
- ❌ 不要自己调用任何 API
- ❌ 不要自己写 SQL 执行代码
- ❌ 不要尝试安装任何 Python 包

### 2. 脚本已经做了一切
脚本内部自动处理：
- 调用 WrenAI 生成 SQL
- 通过 HTTP API 执行 SQL（无需数据库驱动）
- 返回 JSON 格式的完整结果

### 3. 常见错误（你肯定犯过）
```bash
# ❌ 错误：自己调用 WrenAI API
curl -X POST http://10.62.239.13:5555/v1/asks ...
# 然后自己想办法执行返回的 SQL

# ❌ 错误：尝试安装 psycopg2
pip install psycopg2
# 或者 apk add py3-psycopg2

# ❌ 错误：自己写数据库连接代码
import psycopg2
conn = psycopg2.connect(...)

# ✅ 正确：直接执行脚本，一次搞定
python3 "/data/.skills/wrenai-query/scripts/query.py" "查询上个月KPI指标" pg
```

## 实际使用示例

### 示例 1：用户问"查询上个月的KPI指标"

```bash
python3 "/data/.skills/wrenai-query/scripts/query.py" "查询上个月的KPI指标" pg
```

然后解析返回的 JSON，提取数据展示给用户。

### 示例 2：用户问"统计各渠道的业务办理量"

```bash
python3 "/data/.skills/wrenai-query/scripts/query.py" "统计各渠道的业务办理量" pg
```

### 示例 3：用户问"查询员工信息"

```bash
python3 "/data/.skills/wrenai-query/scripts/query.py" "查询员工信息" hr
```

## 参数说明

| 参数 | 说明 | 示例 |
|------|------|------|
| 第1个参数 | 自然语言查询问题（必须用引号包裹） | "查询上个月江西省的KPI指标" |
| 第2个参数 | 数据库实例（可选，默认pg） | pg / mssql / hr |

## 返回结果格式

```json
{
  "success": true,
  "query": "查询上个月江西省的KPI指标",
  "sql": "SELECT ...",  // 生成的 SQL
  "execution_success": true,
  "results": {
    "records": [...],   // 查询结果数据
    "columns": [...],   // 列定义
    "total_rows": 10
  },
  "analysis": "AI分析..."
}
```

## 完整工作流

```bash
# Step 1: 执行查询（直接复制这行，修改查询内容即可）
RESULT=$(python3 "/data/.skills/wrenai-query/scripts/query.py" "查询上个月江西省的KPI指标" pg 2>/dev/null)

# Step 2: 检查是否成功
echo "$RESULT" | python3 -c "import json,sys; d=json.load(sys.stdin); print('成功' if d.get('success') else '失败')"

# Step 3: 提取数据展示给用户
echo "$RESULT" | python3 -c "
import json,sys
d = json.load(sys.stdin)
if d.get('success') and d.get('execution_success'):
    records = d.get('results',{}).get('records',[])
    print(f'查询成功，返回 {len(records)} 条记录')
    print(f'SQL: {d.get(\"sql\", \"\")[:100]}...')
else:
    print('查询失败:', d.get('error',{}).get('message','未知错误'))
"
```

## 记住这个模板

每当用户要求查询数据库时，使用这个模板：

```bash
# 执行查询
result=$(python3 "/data/.skills/wrenai-query/scripts/query.py" "【用户的查询问题】" pg 2>/dev/null)

# 检查成功
if echo "$result" | python3 -c "import json,sys; d=json.load(sys.stdin); sys.exit(0 if d.get('success') and d.get('execution_success') else 1)"; then
    # 成功，提取数据
    records=$(echo "$result" | python3 -c "import json,sys; d=json.load(sys.stdin); print(json.dumps(d.get('results',{}).get('records',[]), ensure_ascii=False))")
    echo "查询成功！返回数据：$records"
else
    # 失败
    error=$(echo "$result" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('error',{}).get('message','未知错误'))")
    echo "查询失败：$error"
fi
```

## 常见问题

### Q: 脚本需要 psycopg2 吗？
**A: 不需要！** 脚本使用 WrenUI HTTP API 执行 SQL，完全不需要数据库驱动。

### Q: 我可以自己调用 WrenAI API 吗？
**A: 不要！** 直接执行脚本即可，脚本已经封装了所有逻辑。

### Q: 查询超时了怎么办？
**A:** 脚本默认最多等待 6 分钟。如果超时，可以简化查询条件再试。

### Q: 返回 SQL 但没有数据？
**A:** 可能是查询条件太严格（如未来的日期）。脚本已经成功执行，只是数据库没有匹配的数据。

## 总结

**最简单的方法：**
```bash
python3 "/data/.skills/wrenai-query/scripts/query.py" "用户的查询" pg
```

**记住：**
- 直接执行，不要自己实现任何逻辑
- 脚本使用 HTTP API，不需要任何 Python 包
- 返回的 JSON 已经包含所有需要的数据
