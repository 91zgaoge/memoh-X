#!/usr/bin/env python3
"""
WrenAI Query Skill Script - Python Implementation
Natural language to SQL query interface for WrenAI
"""

import sys
import json
import urllib.request
import urllib.error
import time
import os
import re
from typing import Optional, Dict, Any, Tuple

# Configuration
WRENAI_HOST = os.environ.get('WRENAI_HOST', '10.62.239.13')
WRENAI_PG_URL = os.environ.get('WRENAI_PG_URL', f'http://{WRENAI_HOST}:5555')
WRENAI_MSSQL_URL = os.environ.get('WRENAI_MSSQL_URL', f'http://{WRENAI_HOST}:6555')
WRENAI_HR_URL = os.environ.get('WRENAI_HR_URL', f'http://{WRENAI_HOST}:7555')

# WrenUI URLs for SQL execution
WRENUI_PG_URL = os.environ.get('WRENUI_PG_URL', f'http://{WRENAI_HOST}:3000')
WRENUI_MSSQL_URL = os.environ.get('WRENUI_MSSQL_URL', f'http://{WRENAI_HOST}:4000')
WRENUI_HR_URL = os.environ.get('WRENUI_HR_URL', f'http://{WRENAI_HOST}:5000')

# Query limit
WRENAI_QUERY_LIMIT = int(os.environ.get('WRENAI_QUERY_LIMIT', '500'))

def get_instance_url(instance: str) -> str:
    """Get instance URL based on instance name"""
    instance = instance.lower().strip()
    if instance in ('pg', 'postgresql', 'postgres'):
        return WRENAI_PG_URL
    elif instance in ('mssql', 'fanwei', 'sqlserver'):
        return WRENAI_MSSQL_URL
    elif instance in ('hr', 'hongjing'):
        return WRENAI_HR_URL
    else:
        raise ValueError(f"Unknown instance: {instance}. Valid: pg, mssql, hr")

def get_wrenui_url(instance: str) -> str:
    """Get WrenUI URL for SQL execution"""
    instance = instance.lower().strip()
    if instance in ('pg', 'postgresql', 'postgres'):
        return WRENUI_PG_URL
    elif instance in ('mssql', 'fanwei', 'sqlserver'):
        return WRENUI_MSSQL_URL
    elif instance in ('hr', 'hongjing'):
        return WRENUI_HR_URL
    else:
        return WRENUI_PG_URL

def execute_sql(ui_url: str, sql: str, limit: int = WRENAI_QUERY_LIMIT) -> Optional[Dict[str, Any]]:
    """Execute SQL via WrenUI API"""
    run_sql_url = f"{ui_url}/api/v1/run_sql"

    request_body = {
        'sql': sql,
        'limit': limit
    }

    try:
        req = urllib.request.Request(
            run_sql_url,
            data=json.dumps(request_body).encode('utf-8'),
            headers={'Content-Type': 'application/json'},
            method='POST'
        )
        with urllib.request.urlopen(req, timeout=60) as resp:
            return json.loads(resp.read().decode('utf-8'))
    except Exception as e:
        print(f"  ⚠️ SQL execution error: {e}", file=sys.stderr)
        return None

def check_health(url: str) -> bool:
    """Check WrenAI health"""
    try:
        req = urllib.request.Request(f"{url}/health", method='GET')
        req.add_header('User-Agent', 'WrenAI-Python-Client/1.0')
        with urllib.request.urlopen(req, timeout=5) as resp:
            return resp.status == 200
    except Exception:
        return False

def get_mdl_hash(ui_port: int) -> Optional[str]:
    """Get MDL hash from WrenUI GraphQL"""
    graphql_url = f"http://localhost:{ui_port}/api/graphql"
    query = '{"query": "{ getCurrentDeployLog { hash } }"}'

    try:
        req = urllib.request.Request(
            graphql_url,
            data=query.encode('utf-8'),
            headers={'Content-Type': 'application/json'},
            method='POST'
        )
        with urllib.request.urlopen(req, timeout=5) as resp:
            data = json.loads(resp.read().decode('utf-8'))
            if data.get('data') and data['data'].get('getCurrentDeployLog'):
                return data['data']['getCurrentDeployLog'].get('hash')
    except Exception:
        pass
    return None

def get_ui_port(instance: str) -> int:
    """Get UI port from instance"""
    instance = instance.lower().strip()
    if instance in ('pg', 'postgresql', 'postgres'):
        return 3000
    elif instance in ('mssql', 'fanwei', 'sqlserver'):
        return 4000
    elif instance in ('hr', 'hongjing'):
        return 5000
    return 3000

def read_query(input_str: str) -> str:
    """Read query from file or string"""
    if input_str.startswith('@'):
        file_path = input_str[1:]
        if not os.path.exists(file_path):
            raise FileNotFoundError(f"Query file not found: {file_path}")
        with open(file_path, 'r', encoding='utf-8') as f:
            return f.read()
    return input_str

def submit_query(url: str, query: str, mdl_hash: Optional[str]) -> str:
    """Submit query to WrenAI"""
    api_url = f"{url}/v1/asks"

    request_body = {
        'query': query,
        'mdl_hash': mdl_hash
    }

    data = json.dumps(request_body).encode('utf-8')

    try:
        req = urllib.request.Request(
            api_url,
            data=data,
            headers={'Content-Type': 'application/json'},
            method='POST'
        )
        with urllib.request.urlopen(req, timeout=30) as resp:
            response_data = json.loads(resp.read().decode('utf-8'))
            query_id = response_data.get('query_id') or response_data.get('id')
            if not query_id:
                raise ValueError(f"No query_id returned: {response_data}")
            return query_id
    except urllib.error.URLError as e:
        raise ConnectionError(f"Failed to submit query: {e}")

def poll_results(url: str, query_id: str, max_attempts: int = 180, interval: int = 2) -> Dict[str, Any]:
    """Poll for results"""
    result_url = f"{url}/v1/asks/{query_id}/result"

    print(f"⏳ Waiting for results (query_id: {query_id})...", file=sys.stderr)

    for i in range(max_attempts):
        try:
            req = urllib.request.Request(result_url, method='GET')
            with urllib.request.urlopen(req, timeout=10) as resp:
                data = json.loads(resp.read().decode('utf-8'))
                status = data.get('status', 'unknown')

                if status in ('finished', 'succeeded', 'completed'):
                    return data
                elif status in ('failed', 'error'):
                    raise RuntimeError(f"Query execution failed: {data}")
                elif status == 'stopped':
                    raise RuntimeError("Query was stopped")

                # Still processing
                elapsed = (i + 1) * interval
                if (i + 1) % 3 == 0:
                    print(f"  ⏳ WrenAI is analyzing... ({elapsed}s)", file=sys.stderr)

                time.sleep(interval)
        except urllib.error.URLError as e:
            print(f"  ⚠️  Error polling results: {e}", file=sys.stderr)
            time.sleep(interval)

    raise TimeoutError(f"Query timed out after {max_attempts * interval} seconds")

def format_results(response: Dict[str, Any], query: str, ui_url: Optional[str] = None) -> str:
    """Format and display results with SQL execution"""
    # Validate response
    if response is None:
        return json.dumps({
            'success': False,
            'error': {
                'code': 'INVALID_RESPONSE',
                'message': 'No response received from WrenAI'
            }
        }, ensure_ascii=False, indent=2)

    # Check for errors
    error_obj = response.get('error') if isinstance(response.get('error'), dict) else {}
    error_msg = error_obj.get('message') if error_obj else response.get('error')
    if error_msg:
        return json.dumps({
            'success': False,
            'error': {
                'code': 'WRENAI_ERROR',
                'message': error_msg
            }
        }, ensure_ascii=False, indent=2)

    # Extract SQL from response array
    sql = None
    if response.get('response') and len(response['response']) > 0:
        sql = response['response'][0].get('sql')
    if not sql:
        sql = response.get('sql') or response.get('generated_sql')

    if not sql:
        return json.dumps({
            'success': False,
            'error': {
                'code': 'NO_SQL_GENERATED',
                'message': 'WrenAI did not generate any SQL for this query'
            }
        }, ensure_ascii=False, indent=2)

    # Execute SQL to get actual results
    execution_success = False
    execution_error = ''
    execution_result = None

    if ui_url:
        print("▶️  Executing SQL via WrenUI...", file=sys.stderr)
        exec_response = execute_sql(ui_url, sql, WRENAI_QUERY_LIMIT)

        if exec_response:
            if 'records' in exec_response:
                execution_result = exec_response
                execution_success = True
                print("✅ SQL executed successfully", file=sys.stderr)
            else:
                execution_error = exec_response.get('message', exec_response.get('error', 'Unknown execution error'))
                print(f"⚠️  SQL execution warning: {execution_error}", file=sys.stderr)
        else:
            print("⚠️  Could not execute SQL (WrenUI may not be available)", file=sys.stderr)

    # Extract analysis from reasoning fields
    analysis = response.get('sql_generation_reasoning') or response.get('intent_reasoning') or 'N/A'

    # Build output
    if execution_success and execution_result:
        return json.dumps({
            'success': True,
            'query': query,
            'sql': sql,
            'results': {
                'records': execution_result.get('records', []),
                'columns': execution_result.get('columns', []),
                'total_rows': execution_result.get('totalRows', 0)
            },
            'execution_success': True,
            'analysis': analysis,
            'raw_response': response
        }, ensure_ascii=False, indent=2)
    else:
        return json.dumps({
            'success': True,
            'query': query,
            'sql': sql,
            'results': None,
            'execution_success': False,
            'execution_error': execution_error or 'SQL execution not available',
            'analysis': analysis,
            'raw_response': response
        }, ensure_ascii=False, indent=2)

def main():
    if len(sys.argv) < 2 or sys.argv[1] in ('-h', '--help'):
        print("""Usage: python3 query.py <query> [instance]

Arguments:
  query       Natural language question to ask the database
              Can be a string or @file path for complex queries
  instance    WrenAI instance to query (default: pg)
              - pg, postgresql  : PostgreSQL instance (localhost:5555)
              - mssql, fanwei   : MSSQL Fanwei instance (localhost:6555)
              - hr, hongjing    : MSSQL HR instance (localhost:7555)

Examples:
  python3 query.py "查询上个月的KPI指标"
  python3 query.py "统计各渠道的业务办理量" pg
  python3 query.py "查询员工信息" hr
  python3 query.py @/tmp/complex-query.txt mssql

Environment Variables:
  WRENAI_PG_URL      PostgreSQL instance URL (default: http://10.62.239.13:5555)
  WRENAI_MSSQL_URL   MSSQL instance URL (default: http://10.62.239.13:6555)
  WRENAI_HR_URL      HR instance URL (default: http://10.62.239.13:7555)
  WRENUI_PG_URL      WrenUI PostgreSQL URL for SQL execution (default: http://10.62.239.13:3000)
  WRENUI_MSSQL_URL   WrenUI MSSQL URL for SQL execution (default: http://10.62.239.13:4000)
  WRENUI_HR_URL      WrenUI HR URL for SQL execution (default: http://10.62.239.13:5000)
  WRENAI_QUERY_LIMIT Max rows to return from SQL execution (default: 500)
  WRENAI_HOST        WrenAI host IP (optional, default: 10.62.239.13)""")
        sys.exit(0)

    query_input = sys.argv[1]
    instance = sys.argv[2] if len(sys.argv) > 2 else 'pg'

    try:
        # Read query
        query = read_query(query_input)
        if not query.strip():
            print(json.dumps({
                'success': False,
                'error': {'code': 'EMPTY_QUERY', 'message': 'Empty query'}
            }))
            sys.exit(1)

        # Get instance URL
        url = get_instance_url(instance)

        print(f"🔍 Querying WrenAI ({instance}: {url})...", file=sys.stderr)
        print(f"❓ Question: {query}", file=sys.stderr)

        # Check health
        if not check_health(url):
            print(json.dumps({
                'success': False,
                'error': {
                    'code': 'CONNECTION_ERROR',
                    'message': f'WrenAI service is not available at {url}. Please check if the service is running.',
                    'recoverable': True
                }
            }))
            sys.exit(1)

        # Get MDL hash
        ui_port = get_ui_port(instance)
        mdl_hash = get_mdl_hash(ui_port)

        if mdl_hash:
            print(f"📋 MDL Hash: {mdl_hash[:16]}...", file=sys.stderr)
        else:
            print("⚠️  Warning: Could not get MDL hash, using null", file=sys.stderr)

        # Submit query
        query_id = submit_query(url, query, mdl_hash)

        # Poll for results
        results = poll_results(url, query_id)

        # Get WrenUI URL for SQL execution
        ui_url = get_wrenui_url(instance)

        # Format and output results
        print(format_results(results, query, ui_url))

    except Exception as e:
        print(json.dumps({
            'success': False,
            'error': {
                'code': 'INTERNAL_ERROR',
                'message': str(e)
            }
        }))
        sys.exit(1)

if __name__ == '__main__':
    main()
