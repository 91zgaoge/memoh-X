#!/bin/bash
#
# WrenAI Query Skill Script
# Natural language to SQL query interface for WrenAI
#
# This script automatically falls back to Python implementation if jq is not available
#

set -e

# Get script directory for fallback
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Check if jq is available, if not use Python implementation
if ! command -v jq &>/dev/null; then
    PYTHON_SCRIPT="${SCRIPT_DIR}/query.py"

    if [[ -f "$PYTHON_SCRIPT" ]]; then
        exec python3 "$PYTHON_SCRIPT" "$@"
    else
        echo '{"success":false,"error":{"code":"MISSING_DEPENDENCY","message":"jq is required but not installed, and Python fallback (query.py) is not available"}}'
        exit 1
    fi
fi

# Configuration
SKILL_DIR="$(dirname "$SCRIPT_DIR")"

# Function: Auto-detect WrenAI host
# Tries multiple addresses to handle different network environments
detect_wrenai_host() {
    local default_host="${WRENAI_HOST:-}"
    if [[ -n "$default_host" ]]; then
        echo "$default_host"
        return 0
    fi

    # Try different host addresses in order of preference
    # 10.62.239.13 is the host IP that works from both host and containers
    local hosts=("10.62.239.13" "172.17.0.1" "host.docker.internal" "localhost")

    for host in "${hosts[@]}"; do
        if curl -sf --max-time 3 "http://$host:5555/health" &>/dev/null; then
            echo "$host"
            return 0
        fi
    done

    # Fallback to 10.62.239.13 (host IP)
    echo "10.62.239.13"
}

# Auto-detect WrenAI host
WRENAI_HOST=$(detect_wrenai_host)

# Default WrenAI instance URLs
WRENAI_PG_URL="${WRENAI_PG_URL:-http://${WRENAI_HOST}:5555}"
WRENAI_MSSQL_URL="${WRENAI_MSSQL_URL:-http://${WRENAI_HOST}:6555}"
WRENAI_HR_URL="${WRENAI_HR_URL:-http://${WRENAI_HOST}:7555}"

# Default WrenUI instance URLs (for SQL execution)
WRENUI_PG_URL="${WRENUI_PG_URL:-http://${WRENAI_HOST}:3000}"
WRENUI_MSSQL_URL="${WRENUI_MSSQL_URL:-http://${WRENAI_HOST}:4000}"
WRENUI_HR_URL="${WRENUI_HR_URL:-http://${WRENAI_HOST}:5000}"

# Default limit for SQL execution results
WRENAI_QUERY_LIMIT="${WRENAI_QUERY_LIMIT:-500}"

# Colors for output (disabled if not terminal)
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    NC='\033[0m' # No Color
else
    RED='' GREEN='' YELLOW='' BLUE='' NC=''
fi

# Function: Print usage
usage() {
    cat << EOF
Usage: $0 <query> [instance]

Arguments:
  query       Natural language question to ask the database
              Can be a string or @file path for complex queries
  instance    WrenAI instance to query (default: pg)
              - pg, postgresql  : PostgreSQL instance (localhost:5555)
              - mssql, fanwei   : MSSQL Fanwei instance (localhost:6555)
              - hr, hongjing    : MSSQL HR instance (localhost:7555)

Examples:
  $0 "查询上个月的KPI指标"
  $0 "统计各渠道的业务办理量" pg
  $0 "查询员工信息" hr
  $0 @/tmp/complex-query.txt mssql

Environment Variables:
  WRENAI_PG_URL      PostgreSQL instance URL (default: http://localhost:5555)
  WRENAI_MSSQL_URL   MSSQL instance URL (default: http://localhost:6555)
  WRENAI_HR_URL      HR instance URL (default: http://localhost:7555)
  WRENUI_PG_URL      WrenUI PostgreSQL URL for SQL execution (default: http://localhost:3000)
  WRENUI_MSSQL_URL   WrenUI MSSQL URL for SQL execution (default: http://localhost:4000)
  WRENUI_HR_URL      WrenUI HR URL for SQL execution (default: http://localhost:5000)
  WRENAI_QUERY_LIMIT Max rows to return from SQL execution (default: 500)
EOF
}

# Function: Check dependencies
check_deps() {
    if ! command -v curl &> /dev/null; then
        echo -e "${RED}Error: curl is required but not installed${NC}"
        exit 1
    fi

    if ! command -v jq &> /dev/null; then
        echo -e "${RED}Error: jq is required but not installed${NC}"
        exit 1
    fi
}

# Function: Get instance URL
get_instance_url() {
    local instance="${1:-pg}"
    case "$instance" in
        pg|postgresql|postgres)
            echo "$WRENAI_PG_URL"
            ;;
        mssql|fanwei|sqlserver)
            echo "$WRENAI_MSSQL_URL"
            ;;
        hr|hongjing)
            echo "$WRENAI_HR_URL"
            ;;
        *)
            echo -e "${RED}Error: Unknown instance '$instance'${NC}" >&2
            echo -e "${YELLOW}Valid instances: pg, mssql, hr${NC}" >&2
            exit 1
            ;;
    esac
}

# Function: Get WrenUI URL for SQL execution
get_wrenui_url() {
    local instance="${1:-pg}"
    case "$instance" in
        pg|postgresql|postgres)
            echo "$WRENUI_PG_URL"
            ;;
        mssql|fanwei|sqlserver)
            echo "$WRENUI_MSSQL_URL"
            ;;
        hr|hongjing)
            echo "$WRENUI_HR_URL"
            ;;
        *)
            echo "$WRENUI_PG_URL"
            ;;
    esac
}

# Function: Execute SQL via WrenUI API
execute_sql() {
    local ui_url="$1"
    local sql="$2"
    local limit="${3:-$WRENAI_QUERY_LIMIT}"

    local run_sql_url="${ui_url}/api/v1/run_sql"

    # Escape SQL for JSON
    local escaped_sql
    escaped_sql=$(echo "$sql" | jq -Rs '.[:-1]')

    local request_body="{\"sql\": $escaped_sql, \"limit\": $limit}"

    local response
    response=$(curl -sf --max-time 60 -X POST \
        -H "Content-Type: application/json" \
        -d "$request_body" \
        "$run_sql_url" 2>&1) || {
        echo "null"
        return 1
    }

    echo "$response"
}

# Function: Check WrenAI health
check_health() {
    local url="$1"
    local health_url="${url}/health"

    if ! curl -sf "$health_url" &> /dev/null; then
        return 1
    fi
    return 0
}

# Function: Get mdl_hash from WrenUI GraphQL
# Note: mdl_hash can be null for WrenAI to use default deployment
get_mdl_hash() {
    local ui_port="$1"
    local graphql_url="http://localhost:${ui_port}/api/graphql"

    # Try to get the latest deploy log hash
    local response
    response=$(curl -sf -X POST \
        -H "Content-Type: application/json" \
        -d '{"query": "{ getCurrentDeployLog { hash } }"}' \
        "$graphql_url" 2>/dev/null)

    if [[ -n "$response" ]]; then
        local mdl_hash
        mdl_hash=$(echo "$response" | jq -r '.data.getCurrentDeployLog.hash // empty')
        if [[ -n "$mdl_hash" && "$mdl_hash" != "null" ]]; then
            echo "$mdl_hash"
            return 0
        fi
    fi

    # WrenAI accepts null mdl_hash, so we return empty string
    # This allows WrenAI to use the default/current deployment
    return 0
}

# Function: Get UI port from instance
get_ui_port() {
    local instance="$1"
    case "$instance" in
        pg|postgresql|postgres)
            echo "3000"
            ;;
        mssql|fanwei|sqlserver)
            echo "4000"
            ;;
        hr|hongjing)
            echo "5000"
            ;;
        *)
            echo "3000"
            ;;
    esac
}

# Function: Read query from file or string
read_query() {
    local input="$1"
    if [[ "$input" == @* ]]; then
        local file="${input#@}"
        if [[ ! -f "$file" ]]; then
            echo -e "${RED}Error: Query file not found: $file${NC}" >&2
            exit 1
        fi
        cat "$file"
    else
        echo "$input"
    fi
}

# Function: Submit query to WrenAI
submit_query() {
    local url="$1"
    local query="$2"
    local mdl_hash="$3"

    local api_url="${url}/v1/asks"

    # Build request body
    local request_body
    if [[ -n "$mdl_hash" ]]; then
        request_body=$(jq -n \
            --arg query "$query" \
            --arg mdl_hash "$mdl_hash" \
            '{query: $query, mdl_hash: $mdl_hash}')
    else
        request_body=$(jq -n \
            --arg query "$query" \
            '{query: $query, mdl_hash: null}')
    fi

    # Submit query
    local response
    response=$(curl -sf --max-time 30 -X POST \
        -H "Content-Type: application/json" \
        -d "$request_body" \
        "$api_url" 2>&1) || {
        echo -e "${RED}Error: Failed to submit query to $api_url${NC}" >&2
        echo -e "${YELLOW}Details: $response${NC}" >&2
        return 1
    }

    # Extract query_id
    local query_id
    query_id=$(echo "$response" | jq -r '.query_id // .id // empty')

    if [[ -z "$query_id" ]]; then
        echo -e "${RED}Error: No query_id returned from WrenAI${NC}" >&2
        echo -e "${YELLOW}Response: $response${NC}" >&2
        return 1
    fi

    echo "$query_id"
}

# Function: Poll for results
poll_results() {
    local url="$1"
    local query_id="$2"
    local max_attempts=180    # Increased from 60 to 180 (6 minutes max)
    local interval=2

    local result_url="${url}/v1/asks/${query_id}/result"

    echo -e "${BLUE}⏳ Waiting for results (query_id: $query_id)...${NC}" >&2

    for ((i=1; i<=max_attempts; i++)); do
        local response
        response=$(curl -sf --max-time 10 "$result_url" 2>&1) || {
            echo -e "${RED}Error: Failed to fetch results${NC}" >&2
            return 1
        }

        local status
        status=$(echo "$response" | jq -r '.status // "unknown"')

        case "$status" in
            "finished"|"succeeded"|"completed")
                echo "$response"
                return 0
                ;;
            "failed"|"error")
                echo -e "${RED}Error: Query execution failed${NC}" >&2
                echo "$response"
                return 1
                ;;
            "stopped")
                echo -e "${YELLOW}Query was stopped${NC}" >&2
                echo "$response"
                return 1
                ;;
            *)
                # Still processing - output progress to keep connection alive
                local elapsed=$((i * interval))
                if (( i % 3 == 0 )); then
                    echo -e "${BLUE}  ⏳ WrenAI 正在分析查询... (${elapsed}秒)${NC}" >&2
                fi
                sleep $interval
                ;;
        esac
    done

    echo -e "${RED}Error: Query timed out after $((max_attempts * interval)) seconds${NC}" >&2
    return 1
}

# Function: Format and display results
format_results() {
    local response="$1"
    local query="$2"
    local ui_url="${3:-}"

    # Check if response is valid JSON
    if ! echo "$response" | jq -e . &> /dev/null; then
        echo -e "${RED}Error: Invalid response from WrenAI${NC}"
        echo "$response"
        return 1
    fi

    # Check for errors
    local error_msg
    error_msg=$(echo "$response" | jq -r '.error.message // .error // empty')
    if [[ -n "$error_msg" ]]; then
        cat << EOF
{
  "success": false,
  "error": {
    "code": "WRENAI_ERROR",
    "message": $(echo "$error_msg" | jq -Rs .)
  }
}
EOF
        return 1
    fi

    # Extract SQL from response array
    local sql
    sql=$(echo "$response" | jq -r '.response[0].sql // .sql // .generated_sql // empty')

    if [[ -z "$sql" || "$sql" == "null" ]]; then
        cat << EOF
{
  "success": false,
  "error": {
    "code": "NO_SQL_GENERATED",
    "message": "WrenAI did not generate any SQL for this query"
  }
}
EOF
        return 1
    fi

    # Execute SQL to get actual results
    local execution_result="null"
    local execution_error=""
    local execution_success=false

    if [[ -n "$ui_url" ]]; then
        echo -e "${BLUE}▶️  Executing SQL via WrenUI...${NC}" >&2
        local exec_response
        exec_response=$(execute_sql "$ui_url" "$sql" "$WRENAI_QUERY_LIMIT")

        if [[ "$exec_response" != "null" ]]; then
            # Check if execution returned valid results
            if echo "$exec_response" | jq -e '.records' &> /dev/null; then
                execution_result="$exec_response"
                execution_success=true
                echo -e "${GREEN}✅ SQL executed successfully${NC}" >&2
            else
                # Extract error message
                execution_error=$(echo "$exec_response" | jq -r '.message // .error // "Unknown execution error"')
                echo -e "${YELLOW}⚠️  SQL execution warning: $execution_error${NC}" >&2
            fi
        else
            echo -e "${YELLOW}⚠️  Could not execute SQL (WrenUI may not be available)${NC}" >&2
        fi
    fi

    # Extract analysis/answer from reasoning fields
    local analysis
    analysis=$(echo "$response" | jq -r '.sql_generation_reasoning // .intent_reasoning // empty')

    # Build output
    if [[ "$execution_success" == true ]]; then
        # Parse execution results
        local records
        local columns
        local total_rows
        records=$(echo "$execution_result" | jq '.records // []')
        columns=$(echo "$execution_result" | jq '.columns // []')
        total_rows=$(echo "$execution_result" | jq -r '.totalRows // 0')

        cat << EOF
{
  "success": true,
  "query": $(echo "$query" | jq -Rs .),
  "sql": $(echo "$sql" | jq -Rs .),
  "results": {
    "records": $records,
    "columns": $columns,
    "total_rows": $total_rows
  },
  "execution_success": true,
  "analysis": $(echo "${analysis:-N/A}" | jq -Rs .),
  "raw_response": $response
}
EOF
    else
        # Return without execution results
        cat << EOF
{
  "success": true,
  "query": $(echo "$query" | jq -Rs .),
  "sql": $(echo "$sql" | jq -Rs .),
  "results": null,
  "execution_success": false,
  "execution_error": $(echo "${execution_error:-SQL execution not available}" | jq -Rs .),
  "analysis": $(echo "${analysis:-N/A}" | jq -Rs .),
  "raw_response": $response
}
EOF
    fi
}

# Main function
main() {
    # Check arguments
    if [[ $# -lt 1 ]] || [[ "$1" == "-h" ]] || [[ "$1" == "--help" ]]; then
        usage
        exit 0
    fi

    local query_input="$1"
    local instance="${2:-pg}"

    # Check dependencies
    check_deps

    # Read query
    local query
    query=$(read_query "$query_input")

    if [[ -z "$query" ]]; then
        echo -e "${RED}Error: Empty query${NC}" >&2
        exit 1
    fi

    # Get instance URL
    local url
    url=$(get_instance_url "$instance")

    echo -e "${BLUE}🔍 Querying WrenAI ($instance: $url)...${NC}" >&2
    echo -e "${BLUE}❓ Question: $query${NC}" >&2

    # Check health
    if ! check_health "$url"; then
        cat << EOF
{
  "success": false,
  "error": {
    "code": "CONNECTION_ERROR",
    "message": "WrenAI service is not available at $url. Please check if the service is running.",
    "recoverable": true
  }
}
EOF
        exit 1
    fi

    # Get mdl_hash
    local ui_port
    ui_port=$(get_ui_port "$instance")

    local mdl_hash
    mdl_hash=$(get_mdl_hash "$ui_port")

    if [[ -n "$mdl_hash" ]]; then
        echo -e "${BLUE}📋 MDL Hash: ${mdl_hash:0:16}...${NC}" >&2
    else
        echo -e "${YELLOW}⚠️ Warning: Could not get MDL hash, query may fail${NC}" >&2
    fi

    # Submit query
    local query_id
    if ! query_id=$(submit_query "$url" "$query" "$mdl_hash"); then
        exit 1
    fi

    # Poll for results
    local results
    if ! results=$(poll_results "$url" "$query_id"); then
        exit 1
    fi

    # Get WrenUI URL for SQL execution
    local ui_url
    ui_url=$(get_wrenui_url "$instance")

    # Format and output results
    format_results "$results" "$query" "$ui_url"
}

# Run main function
main "$@"
