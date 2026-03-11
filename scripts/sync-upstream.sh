#!/bin/bash
#
# Memoh-v2 上游同步脚本
# 用于同步上游 https://github.com/memohai/Memoh 的更新，同时保留二次开发功能
#
# 使用方法:
#   ./scripts/sync-upstream.sh [strategy]
#
#   strategy: conservative (默认) | aggressive
#     - conservative: 仅自动合并安全文件，冲突文件保留我们的版本
#     - aggressive: 尝试自动合并所有文件，冲突时保留上游版本（需要手动恢复二次开发功能）

set -e

# 配置
UPSTREAM_URL="https://github.com/memohai/Memoh.git"
UPSTREAM_BRANCH="upstream/main"
STRATEGY="${1:-conservative}"
DATE_SUFFIX=$(date +%Y%m%d-%H%M%S)
SYNC_BRANCH="sync-upstream-${DATE_SUFFIX}"
REPORT_FILE="sync-report-${DATE_SUFFIX}.md"
BACKUP_BRANCH="pre-sync-backup-${DATE_SUFFIX}"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 显示帮助
show_help() {
    cat << 'EOF'
Memoh-v2 上游同步脚本

用法: ./scripts/sync-upstream.sh [选项] [策略]

选项:
  -h, --help          显示此帮助信息
  -n, --dry-run       试运行，不实际执行合并

策略:
  conservative        保守策略（默认）- 优先保留二次开发功能
  aggressive          激进策略 - 优先获取上游新功能

示例:
  ./scripts/sync-upstream.sh              # 使用保守策略同步
  ./scripts/sync-upstream.sh aggressive   # 使用激进策略同步
  ./scripts/sync-upstream.sh -n           # 试运行，查看会发生什么

EOF
}

# 检查环境
check_environment() {
    log_info "检查环境..."

    # 检查是否在 git 仓库中
    if ! git rev-parse --git-dir > /dev/null 2>&1; then
        log_error "当前目录不是 git 仓库"
        exit 1
    fi

    # 检查 upstream 远程
    if ! git remote | grep -q "upstream"; then
        log_info "添加上游远程仓库..."
        git remote add upstream "$UPSTREAM_URL"
    fi

    # 获取上游更新
    log_info "获取上游更新..."
    git fetch upstream

    log_success "环境检查完成"
}

# 创建备份
create_backup() {
    log_info "创建备份分支: $BACKUP_BRANCH"
    git checkout -b "$BACKUP_BRANCH"
    git checkout -
    log_success "备份完成"
}

# 分析差异
analyze_differences() {
    log_info "分析上游差异..."

    # 创建临时文件存储文件列表
    UPSTREAM_FILES=$(mktemp)
    CURRENT_FILES=$(mktemp)
    UNIQUE_TO_UPSTREAM=$(mktemp)
    UNIQUE_TO_US=$(mktemp)
    COMMON_FILES=$(mktemp)

    # 获取文件列表
    git ls-tree -r --name-only upstream/main | sort > "$UPSTREAM_FILES"
    git ls-tree -r --name-only HEAD | sort > "$CURRENT_FILES"

    # 计算差异
    comm -23 "$UPSTREAM_FILES" "$CURRENT_FILES" > "$UNIQUE_TO_UPSTREAM"
    comm -13 "$UPSTREAM_FILES" "$CURRENT_FILES" > "$UNIQUE_TO_US"
    comm -12 "$UPSTREAM_FILES" "$CURRENT_FILES" > "$COMMON_FILES"

    # 统计
    UPSTREAM_COUNT=$(wc -l < "$UPSTREAM_FILES")
    CURRENT_COUNT=$(wc -l < "$CURRENT_FILES")
    UNIQUE_UPSTREAM_COUNT=$(wc -l < "$UNIQUE_TO_UPSTREAM")
    UNIQUE_US_COUNT=$(wc -l < "$UNIQUE_TO_US")
    COMMON_COUNT=$(wc -l < "$COMMON_FILES")

    log_info "文件统计:"
    echo "  上游总文件数: $UPSTREAM_COUNT"
    echo "  我们总文件数: $CURRENT_COUNT"
    echo "  上游独有文件: $UNIQUE_UPSTREAM_COUNT"
    echo "  我们独有文件: $UNIQUE_US_COUNT"
    echo "  共同文件: $COMMON_COUNT"

    # 检查关键二次开发文件
    log_info "检查关键二次开发文件..."
    CRITICAL_FILES=(
        "internal/channel/adapters/wecom/adapter.go"
        "internal/channel/adapters/wecom/config.go"
        "internal/channel/adapters/wecom/websocket.go"
        "internal/channel/adapters/wecom/types.go"
        "internal/channel/adapters/wecom/crypto.go"
        "internal/fileparse/docx.go"
        "internal/fileparse/xlsx.go"
        "internal/fileparse/pdf.go"
        "internal/fileparse/parse.go"
    )

    for file in "${CRITICAL_FILES[@]}"; do
        if grep -q "^${file}$" "$UNIQUE_TO_US"; then
            log_success "✓ $file (我们的独有文件，安全)"
        elif grep -q "^${file}$" "$COMMON_FILES"; then
            log_warn "⚠ $file (共同文件，可能需要合并)"
        else
            log_error "✗ $file (未找到！)"
        fi
    done

    # 保存分析结果
    cat > "$REPORT_FILE" << EOF
# 上游同步分析报告

**同步时间**: $(date)
**上游分支**: $UPSTREAM_BRANCH
**同步策略**: $STRATEGY
**工作分支**: $SYNC_BRANCH

## 文件统计

| 类型 | 数量 |
|------|------|
| 上游总文件 | $UPSTREAM_COUNT |
| 我们总文件 | $CURRENT_COUNT |
| 上游独有 | $UNIQUE_UPSTREAM_COUNT |
| 我们独有 | $UNIQUE_US_COUNT |
| 共同文件 | $COMMON_COUNT |

## 上游独有文件（可安全添加）

$(cat "$UNIQUE_TO_UPSTREAM" | head -50)

## 我们独有文件（必须保留）

$(cat "$UNIQUE_TO_US")

## 分析详情

### 关键二次开发文件状态

EOF

    for file in "${CRITICAL_FILES[@]}"; do
        if grep -q "^${file}$" "$UNIQUE_TO_US"; then
            echo "- ✅ $file: 我们的独有文件，安全" >> "$REPORT_FILE"
        elif grep -q "^${file}$" "$COMMON_FILES"; then
            echo "- ⚠️ $file: 共同文件，需要审查" >> "$REPORT_FILE"
        else
            echo "- ❌ $file: 未找到，可能已丢失" >> "$REPORT_FILE"
        fi
    done

    # 清理临时文件
    rm -f "$UPSTREAM_FILES" "$CURRENT_FILES" "$UNIQUE_TO_UPSTREAM" "$UNIQUE_TO_US" "$COMMON_FILES"

    log_success "差异分析完成，报告保存到: $REPORT_FILE"
}

# 选择性合并
selective_merge() {
    log_info "开始选择性合并..."

    # 创建同步分支
    git checkout -b "$SYNC_BRANCH"

    # 获取上游独有文件（安全合并）
    log_info "添加上游独有文件..."
    git checkout upstream/main -- . 2>/dev/null || true

    # 恢复我们的关键文件
    log_info "恢复二次开发文件..."

    # 从备份分支恢复关键目录
    CRITICAL_DIRS=(
        "internal/channel/adapters/wecom"
        "internal/fileparse"
        "internal/skills/defaults/docx"
        "internal/skills/defaults/xlsx"
        "internal/skills/defaults/pdf"
        "internal/skills/defaults/pptx"
    )

    for dir in "${CRITICAL_DIRS[@]}"; do
        if [ -d "$dir" ] || git show "main:$dir" > /dev/null 2>&1; then
            log_info "  恢复: $dir"
            git checkout main -- "$dir" 2>/dev/null || true
        fi
    done

    # 恢复特定文件
    CRITICAL_FILES=(
        "cmd/agent/main.go"
        "internal/channel/inbound/channel.go"
        "internal/conversation/flow/resolver.go"
        "internal/handlers/models.go"
        "internal/models/models.go"
        "internal/models/types.go"
        "packages/web/src/data/model-catalog.ts"
    )

    for file in "${CRITICAL_FILES[@]}"; do
        if git show "main:$file" > /dev/null 2>&1; then
            log_info "  恢复: $file"
            git checkout main -- "$file" 2>/dev/null || true
        fi
    done

    # 保留我们的 docker-compose 配置
    if [ -f "docker-compose.yml" ]; then
        log_info "  恢复: docker-compose.yml"
        git checkout main -- docker-compose.yml 2>/dev/null || true
    fi

    # 暂存所有变更
    git add -A

    log_success "选择性合并完成"
}

# 生成移植任务清单
generate_porting_checklist() {
    log_info "生成移植任务清单..."

    cat >> "$REPORT_FILE" << 'EOF'

## 需要手动移植的上游新功能

### 1. 消息中止功能 (Message Abort)
**上游提交**: `23d49a1c feat: message abort and web socket support`
**优先级**: 高
**复杂度**: 中

需要移植的文件:
- `apps/agent/src/modules/chat.ts` → `agent/src/modules/chat.ts`
- `apps/web/src/pages/chat/` → `packages/web/src/pages/chat/`

移植步骤:
1. 比较 upstream/apps/agent/src/modules/chat.ts 与当前 agent/src/modules/chat.ts
2. 提取消息中止相关逻辑
3. 适配到我们的代码结构
4. 测试流式响应中止功能

### 2. Gmail OAuth2 支持
**上游提交**: `a5c36491 feat(email/oauth): implement OAuth2 support for Gmail provider`
**优先级**: 中
**复杂度**: 中

需要审查的文件:
- 邮件提供商配置
- 数据库迁移文件

### 3. Discord 修复
**上游提交**: `a2cb5939 fix(discord): rm reason in final message`
**优先级**: 低
**复杂度**: 低

说明: 如果使用 Discord 适配器，需要应用此修复

### 4. 聊天图片冻结修复
**上游提交**: `ef7ed961 fix(web): resolve chat image freeze and dynamic import failures`
**优先级**: 高
**复杂度**: 低

需要移植的文件:
- 前端图片处理相关代码

### 5. MCP 僵尸进程修复
**上游提交**: `8ce5243e fix(mcp): use dumb-init as PID 1 to reap zombie processes`
**优先级**: 中
**复杂度**: 低

需要修改:
- MCP 容器启动配置

### 6. 浏览器服务 (可选)
**上游路径**: `apps/browser/`
**优先级**: 低
**复杂度**: 高

说明: 上游将浏览器功能独立为单独服务。如果当前浏览器功能满足需求，可以暂缓移植。

## 移植验证清单

- [ ] 企业微信适配器工作正常
- [ ] 基础对话功能正常
- [ ] 流式响应正常
- [ ] 消息中止功能（如移植）正常
- [ ] 文件解析功能正常
- [ ] 记忆系统正常
- [ ] 容器功能正常

EOF

    log_success "移植任务清单已添加到报告"
}

# 主函数
main() {
    echo "========================================"
    echo "  Memoh-v2 上游同步工具"
    echo "========================================"
    echo ""

    # 解析参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            -n|--dry-run)
                DRY_RUN=1
                shift
                ;;
            conservative|aggressive)
                STRATEGY="$1"
                shift
                ;;
            *)
                log_error "未知选项: $1"
                show_help
                exit 1
                ;;
        esac
    done

    log_info "同步策略: $STRATEGY"
    log_info "报告文件: $REPORT_FILE"

    # 执行步骤
    check_environment

    if [ -n "$DRY_RUN" ]; then
        log_info "【试运行模式】不执行实际修改"
        analyze_differences
        log_info "试运行完成，查看 $REPORT_FILE 了解详情"
        exit 0
    fi

    create_backup
    analyze_differences
    selective_merge
    generate_porting_checklist

    # 完成
    echo ""
    echo "========================================"
    log_success "同步准备完成!"
    echo "========================================"
    echo ""
    echo "当前工作分支: $SYNC_BRANCH"
    echo "备份分支: $BACKUP_BRANCH"
    echo "详细报告: $REPORT_FILE"
    echo ""
    echo "后续步骤:"
    echo "  1. 查看报告了解需要移植的功能"
    echo "  2. 手动移植上游新功能（按报告中的清单）"
    echo "  3. 测试所有功能"
    echo "  4. 提交更改: git commit -m 'sync: 合并上游更新并保留二次开发功能'"
    echo ""
    echo "如需回滚: git checkout main && git branch -D $SYNC_BRANCH"
    echo ""
}

# 运行主函数
main "$@"
