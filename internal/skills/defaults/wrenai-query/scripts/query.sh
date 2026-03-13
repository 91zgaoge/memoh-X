#!/bin/bash
#
# WrenAI Query Skill Script
# Natural language to SQL query interface for WrenAI
#

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKILL_DIR="$(dirname "$SCRIPT_DIR")"

# Default WrenAI instance URLs
WRENAI_PG_URL="${WRENAI_PG_URL:-http://localhost:5555}"
WRENAI_MSSQL_URL="${WRENAI_MSSQL_URL:-http://localhost:6555}"
WRENAI_HR_URL="${WRENAI_HR_URL:-http://localhost:7555}"

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
    response=$(curl -sf -X POST \
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
    local max_attempts=60
    local interval=2

    local result_url="${url}/v1/asks/${query_id}/result"

    echo -e "${BLUE}⏳ Waiting for results (query_id: $query_id)...${NC}" >&2

    for ((i=1; i<=max_attempts; i++)); do
        local response
        response=$(curl -sf "$result_url" 2>&1) || {
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
                # Still processing
                if (( i % 5 == 0 )); then
                    echo -e "${BLUE}  Still processing... ($i/$max_attempts)${NC}" >&2
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

    # Extract SQL
    local sql
    sql=$(echo "$response" | jq -r '.sql // .generated_sql // empty')

    # Extract results
    local results
    results=$(echo "$response" | jq '.results // .data // empty')

    # Extract analysis/answer
    local analysis
    analysis=$(echo "$response" | jq -r '.analysis // .answer // .response // empty')

    # Build output
    cat << EOF
{
  "success": true,
  "query": $(echo "$query" | jq -Rs .),
  "sql": $(echo "${sql:-N/A}" | jq -Rs .),
  "results": ${results:-null},
  "analysis": $(echo "${analysis:-N/A}" | jq -Rs .),
  "raw_response": $response
}
EOF
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

    # Format and output results
    format_results "$results" "$query"
}

# Run main function
main "$@"
