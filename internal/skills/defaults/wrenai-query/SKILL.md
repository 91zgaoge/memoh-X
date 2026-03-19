---
name: wrenai-query
description: WrenAI natural language to SQL query tool. IMPORTANT: Use exec tool to run the script directly, DO NOT implement the logic yourself.
---

# WrenAI Query Tool

## ⚠️ CRITICAL INSTRUCTIONS FOR AI AGENTS

**YOU MUST FOLLOW THESE INSTRUCTIONS EXACTLY. DO NOT DEVIATE.**

### When User Asks for Database Query

**USE THE EXEC TOOL. RUN THIS EXACT COMMAND:**

```bash
sh "/data/.skills/wrenai-query/scripts/wrenai" "user's natural language query here" pg
```

**DO NOT:**
- ❌ DO NOT call WrenAI API yourself
- ❌ DO NOT write your own SQL execution code
- ❌ DO NOT try to install psycopg2 or any Python packages
- ❌ DO NOT read the script and reimplement its logic
- ❌ DO NOT use skills tool to "understand" then implement

**WHY:**
The script at `/data/.skills/wrenai-query/scripts/wrenai` already does everything:
1. Calls WrenAI API to generate SQL
2. Executes SQL via HTTP API (no database drivers needed)
3. Returns JSON with results

**Just execute the command and parse the JSON output.**

---

## Usage

### Basic Query (PostgreSQL - default)

```bash
sh "/data/.skills/wrenai-query/scripts/wrenai" "Query last month's KPI indicators" pg
```

### Query MSSQL (Fanwei system)

```bash
sh "/data/.skills/wrenai-query/scripts/wrenai" "统计各渠道的业务办理量" mssql
```

### Query HR System (Hongjing)

```bash
sh "/data/.skills/wrenai-query/scripts/wrenai" "查询员工信息" hr
```

---

## Output Format

The script returns JSON:

```json
{
  "success": true,
  "query": "查询上个月江西省的KPI指标",
  "sql": "SELECT * FROM ...",
  "execution_success": true,
  "results": {
    "records": [...],
    "columns": [...],
    "total_rows": 10
  },
  "analysis": "AI analysis text"
}
```

**Parse this JSON to get the data. DO NOT try to execute the SQL yourself.**

---

## Supported Data Sources

| Instance | Database | Use Case |
|----------|----------|----------|
| pg | PostgreSQL | CBNBI business data, KPIs |
| mssql | SQL Server | Fanwei system data |
| hr | SQL Server | HR system data |

---

## Technical Details (For Reference Only)

**DO NOT USE THIS INFORMATION TO IMPLEMENT YOUR OWN SOLUTION.**

- WrenAI Service: Converts natural language to SQL
- WrenUI Service: Executes SQL via HTTP API at `/api/v1/run_sql`
- The script uses only Python standard library (urllib, json)
- No database drivers (psycopg2, pymssql) required

**If you find yourself writing Python code to call WrenAI API, YOU ARE DOING IT WRONG.**

Use the script. That's it.
