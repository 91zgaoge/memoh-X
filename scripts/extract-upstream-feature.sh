#!/bin/bash
#
# 提取上游特定功能的补丁
# 用于将上游的新功能移植到我们的代码库
#
# 使用方法:
#   ./scripts/extract-upstream-feature.sh <commit-range> <feature-name>
#
# 示例:
#   # 提取消息中止功能
#   ./scripts/extract-upstream-feature.sh "23d49a1c^..23d49a1c" "message-abort"
#
#   # 提取 Gmail OAuth 支持
#   ./scripts/extract-upstream-feature.sh "a5c36491^..a5c36491" "gmail-oauth"

set -e

if [ $# -lt 2 ]; then
    echo "用法: $0 <commit-range> <feature-name>"
    echo ""
    echo "示例:"
    echo "  $0 \"23d49a1c^..23d49a1c\" message-abort"
    echo ""
    echo "常用上游提交:"
    echo "  23d49a1c - 消息中止和 WebSocket 支持"
    echo "  a5c36491 - Gmail OAuth2 支持"
    echo "  ef7ed961 - 聊天图片冻结修复"
    echo "  8ce5243e - MCP 僵尸进程修复"
    echo "  a2cb5939 - Discord 修复"
    exit 1
fi

COMMIT_RANGE="$1"
FEATURE_NAME="$2"
PATCH_DIR="upstream-patches"
DATE_SUFFIX=$(date +%Y%m%d)

# 创建补丁目录
mkdir -p "$PATCH_DIR"

PATCH_FILE="$PATCH_DIR/${FEATURE_NAME}-${DATE_SUFFIX}.patch"
README_FILE="$PATCH_DIR/${FEATURE_NAME}-${DATE_SUFFIX}-README.md"

echo "========================================"
echo "提取上游功能: $FEATURE_NAME"
echo "提交范围: $COMMIT_RANGE"
echo "========================================"
echo ""

# 生成补丁
echo "正在生成补丁..."
git diff "$COMMIT_RANGE" > "$PATCH_FILE" 2>/dev/null || \
git show "$COMMIT_RANGE" --patch > "$PATCH_FILE"

if [ ! -s "$PATCH_FILE" ]; then
    echo "错误: 无法生成补丁，检查提交范围是否正确"
    exit 1
fi

echo "✓ 补丁已保存: $PATCH_FILE"

# 统计变更
echo ""
echo "变更统计:"
git diff --stat "$COMMIT_RANGE" 2>/dev/null || git show --stat "$COMMIT_RANGE"

# 生成移植指南
cat > "$README_FILE" << EOF
# 上游功能移植指南: $FEATURE_NAME

**提取时间**: $(date)
**上游提交**: $COMMIT_RANGE
**补丁文件**: $PATCH_FILE

## 功能描述

$(git log --oneline -1 "$COMMIT_RANGE" 2>/dev/null | cut -d' ' -f2- || echo "请手动填写")

## 涉及文件

$(git diff --name-only "$COMMIT_RANGE" 2>/dev/null || git show --name-only --pretty="" "$COMMIT_RANGE")

## 移植步骤

### 1. 审查变更

查看补丁文件了解具体变更：
\`\`\`bash
cat $PATCH_FILE
\`\`\`

### 2. 路径映射

上游路径 → 我们的路径映射：

- \`apps/agent/\` → \`agent/\`
- \`apps/web/\` → \`packages/web/\`
- \`apps/browser/\` → (需要新建或集成到现有代码)
- \`packages/ui/\` → \`packages/ui/\` (相同)
- \`packages/sdk/\` → \`packages/sdk/\` (相同)

### 3. 逐步移植

对于每个涉及文件：

1. 找到对应的我们的代码文件
2. 手动应用相关变更
3. 解决路径和导入差异
4. 测试编译

### 4. 冲突解决提示

#### 常见冲突类型

**路径冲突**: 上游使用 \`apps/\` 结构，我们使用 \`packages/\` 和 \`agent/\`
- 解决: 修改补丁中的路径后应用

**导入冲突**: 包名或导入路径不同
- 解决: 手动修改导入语句

**代码风格冲突**: 格式化或命名差异
- 解决: 保持我们的代码风格

#### 安全修改 vs 需要审查的修改

✅ **通常可以安全应用**:
- 新增文件
- Bug 修复（小范围变更）
- 新增配置选项

⚠️ **需要仔细审查**:
- 核心逻辑变更
- 数据库结构变更
- API 接口变更
- 依赖版本升级

### 5. 测试验证

移植完成后验证:

- [ ] 代码编译通过
- [ ] 功能正常工作
- [ ] 不影响现有功能（特别是企业微信适配器）
- [ ] 没有引入新的错误

## 参考信息

### 上游完整提交信息

\`\`\`
$(git show "$COMMIT_RANGE" --quiet 2>/dev/null || echo "无法获取提交信息")
\`\`\`

### 相关文件清单

\`\`\`
$(git diff --name-only "$COMMIT_RANGE" 2>/dev/null || git show --name-only --pretty="" "$COMMIT_RANGE")
\`\`\`
EOF

echo ""
echo "✓ 移植指南已保存: $README_FILE"
echo ""
echo "========================================"
echo "下一步操作:"
echo "========================================"
echo ""
echo "1. 阅读移植指南:"
echo "   cat $README_FILE"
echo ""
echo "2. 查看补丁内容:"
echo "   cat $PATCH_FILE"
echo ""
echo "3. 根据指南逐步移植到我们的代码库"
echo ""
echo "4. 测试验证后提交更改"
echo ""
