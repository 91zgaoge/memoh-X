---
name: wrenai-query
description: Intelligent data analysis using WrenAI natural language to SQL. Use this skill when you need to query structured data from PostgreSQL or SQL Server databases using natural language.
---

# WrenAI Query Skill

## Overview

This skill enables natural language querying of structured databases through WrenAI, which converts natural language questions into SQL queries and returns results with AI-powered analysis.

## Supported Data Sources

| Instance | Database Type | Connection | Use Case |
|----------|---------------|------------|----------|
| WrenAI-PostgreSQL | PostgreSQL | 10.60.45.45:5432/ods | CBNBI business data, KPIs |
| WrenAI-MSSQL-Fanwei | SQL Server | 10.60.41.127 | Fanwei system data |
| WrenAI-MSSQL-Hongjing | SQL Server | 10.60.41.143 | HR system data |

## Environment Variables

- `WRENAI_PG_URL` - WrenAI PostgreSQL instance URL (default: auto-detect)
- `WRENAI_MSSQL_URL` - WrenAI MSSQL instance URL (default: auto-detect)
- `WRENAI_HR_URL` - WrenAI HR instance URL (default: auto-detect)
- `WRENAI_HOST` - WrenAI host IP (optional, auto-detect if not set)

**Network Configuration:**
The script automatically detects the correct WrenAI host address by trying:
1. `10.62.239.13` (host IP - works from both host and containers)
2. `172.17.0.1` (Docker gateway)
3. `host.docker.internal` (Docker Desktop)
4. `localhost` (fallback)

To manually specify the host, set `WRENAI_HOST` environment variable.

## When to Use This Skill

Use the wrenai-query skill when you need to:

- **Query business databases** using natural language instead of SQL
- **Analyze KPI metrics** from the CBNBI system
- **Query customer/account data** from CBNDM data mart
- **Search HR records** from Hongjing HR system
- **Analyze billing/payment data** from SIDSS
- **Generate data reports** with automatic SQL generation

**Examples of when to use:**
- User: "查询上个月江西省的KPI指标完成情况"
- User: "统计各渠道的业务办理量"
- User: "查找流失用户的明细"
- User: "分析账户账单数据"
- User: "查询集团客户信息"

## How It Works

```
┌──────────┐    Bash/HTTP    ┌─────────────┐    HTTP    ┌──────────────┐    SQL    ┌──────────┐
│  Claude  │────────────────▶│ wrenai-query│───────────▶│ WrenAI       │──────────▶│ Database │
│          │   Skill Script  │   Script    │            │ AI Service   │           │          │
└──────────┘                 └─────────────┘            └──────────────┘           └──────────┘
                                                               │
                                                               ▼
                                                        ┌──────────────┐
                                                        │ LLM Service  │
                                                        │ Qwen3.5-35B  │
                                                        └──────────────┘
```

**Architecture:**
1. **Skill Script** - Bash interface that calls WrenAI REST API
2. **WrenAI Service** - Converts natural language to SQL
3. **LLM Service** - Local Qwen3.5-35B model for SQL generation
4. **Database** - PostgreSQL or SQL Server data sources

## Usage

### Basic Query

```bash
bash "$SKILLS_ROOT/wrenai-query/scripts/query.sh" "your natural language question" [instance]
```

**Python Fallback:**
The script automatically detects if `jq` is available. If not, it falls back to a pure Python implementation (`query.py`) that requires no external dependencies beyond Python 3 standard library.

**Instances:**
- `pg` or `postgresql` - PostgreSQL instance (default)
- `mssql` or `fanwei` - MSSQL Fanwei instance
- `hr` or `hongjing` - MSSQL HR instance

### Examples

```bash
# Query PostgreSQL (default)
bash "$SKILLS_ROOT/wrenai-query/scripts/query.sh" "查询上个月的KPI指标"

# Query specific instance
bash "$SKILLS_ROOT/wrenai-query/scripts/query.sh" "统计各渠道的业务办理量" pg

# Query HR system
bash "$SKILLS_ROOT/wrenai-query/scripts/query.sh" "查询员工信息" hr

# Query with Chinese
bash "$SKILLS_ROOT/wrenai-query/scripts/query.sh" "查找江西省的集团客户"
```

### Advanced Usage

```bash
# Save query to file for complex queries
cat > /tmp/wrenai-query.txt <<'TXT'
分析上个月各城市的账户欠费情况，并按欠费金额排序
TXT

bash "$SKILLS_ROOT/wrenai-query/scripts/query.sh" @/tmp/wrenai-query.txt pg
```

## Output Format

### Success Response

```json
{
  "success": true,
  "query": "分析上个月各城市的账户欠费情况",
  "sql": "SELECT city_id, SUM(total_owe_amt) as total_owe FROM cbndm.ti_acc_daily_bill_info_d WHERE statis_ymd BETWEEN '20240101' AND '20240131' GROUP BY city_id ORDER BY total_owe DESC",
  "results": {
    "columns": ["city_id", "total_owe"],
    "rows": [
      ["南昌市", 1250000.50],
      ["九江市", 890000.30],
      ["赣州市", 650000.20]
    ]
  },
  "analysis": "南昌市欠费金额最高，达125万元，建议重点关注...",
  "latency_ms": 4500
}
```

### Error Response

```json
{
  "success": false,
  "error": {
    "code": "CONNECTION_ERROR",
    "message": "无法连接到WrenAI服务",
    "recoverable": true
  }
}
```

## Error Codes

| Code | Meaning | Solution |
|------|---------|----------|
| `CONNECTION_ERROR` | Cannot connect to WrenAI service | Check if WrenAI containers are running |
| `TIMEOUT_ERROR` | Query execution timeout | Simplify the query or check network |
| `SQL_GENERATION_ERROR` | Failed to generate SQL | Rephrase the question more clearly |
| `DATABASE_ERROR` | Database connection failed | Check database connectivity |
| `NO_DATA_FOUND` | Query returned no results | Verify data exists for the criteria |

## Best Practices

### 1. Be Specific

**Good:** "查询2024年1月南昌市的KPI指标"
**Bad:** "查一下数据"

### 2. Use Business Terms

WrenAI understands the configured data models, so use table/field names from the documentation.

### 3. Handle Large Results

For queries that may return large datasets, the system automatically limits results.

### 4. Check Instance Availability

Before querying, verify the target instance is running:

```bash
# Check all WrenAI instances
docker ps | grep wrenai
curl -s http://localhost:5555/health
curl -s http://localhost:6555/health
curl -s http://localhost:7555/health
```

## Data Models Reference

### PostgreSQL Instance (19 Models)

| Schema | Models | Description |
|--------|--------|-------------|
| cbnbi | kpi_dict, kpi_value, kpi_dimension_dict | KPI definitions and values |
| cbndm | ti_acc_daily_bill_info_d, ti_acc_pay_dtl_d, ti_cust_view_cust_info_d | Account and customer data |
| siboss | dbcustadm_ct_grpcust_info, dbcustadm_ur_user_info | User and group info |
| sidss | pmrt_user_lqmx, t2_ia_loginopr_pay_yyyy | Payment and churn data |

Full documentation: `/data2/WrenAI/docker/MODELS_DOCUMENTATION.md`

## Troubleshooting

### Connection Refused

```
Error: Connection refused to localhost:5555
```

**Solution:**
```bash
# Check if WrenAI PostgreSQL instance is running
docker ps | grep wrenai
cd /data2/WrenAI/docker && docker-compose up -d
```

### LLM Service Unavailable

```
Error: AI service timeout
```

**Solution:**
```bash
# Check LLM services
systemctl status llama-qwen35
curl http://172.17.0.1:17099/health
```

### Database Connection Error

```
Error: Database connection failed
```

**Solution:** Check network connectivity to database servers:
```bash
nc -zv 10.60.45.45 5432  # PostgreSQL
nc -zv 10.60.41.127 1433  # MSSQL Fanwei
nc -zv 10.60.41.143 1433  # MSSQL HR
```

## Related Documentation

- WrenAI Project Guide: `/data2/WrenAI/PROJECT_MAINTENANCE.md`
- Data Models Doc: `/data2/WrenAI/docker/MODELS_DOCUMENTATION.md`
- WrenAI GitHub: https://github.com/Canner/WrenAI
