package wecom

import (
	"regexp"
	"strings"
)

// ThinkTagVariants 支持的思考标签变体
var ThinkTagVariants = []string{
	"think",      // 标准 <think>
	"thinking",   // <thinking>
	"thought",    // <thought>
	"reasoning",  // <reasoning>
	"reason",     // <reason>
}

// thinkTagRegex 缓存编译后的正则表达式
var thinkTagRegex *regexp.Regexp

func init() {
	// 编译匹配各种思考标签的正则表达式
	// 匹配 <think>...</think>, <thinking>...</thinking> 等
	patterns := make([]string, 0, len(ThinkTagVariants)*2)
	for _, tag := range ThinkTagVariants {
		patterns = append(patterns, `<`+tag+`>`, `</`+tag+`>`)
	}
	thinkTagRegex = regexp.MustCompile(`(?i)(` + strings.Join(patterns, "|") + `)`)
}

// NormalizeThinkTags 将所有思考标签变体规范化为 <think> 标签
// 例如：<thinking>...</thinking> -> <think>...</think>
func NormalizeThinkTags(content string) string {
	if content == "" {
		return ""
	}

	result := content

	// 将所有变体标签替换为标准标签
	for _, variant := range ThinkTagVariants[1:] { // 跳过第一个 "think"
		// 替换开始标签
		result = regexp.MustCompile(`(?i)<`+variant+`(?:\s+[^>]*)?>`).ReplaceAllString(result, "<think>")
		// 替换结束标签
		result = regexp.MustCompile(`(?i)</`+variant+`>`).ReplaceAllString(result, "</think>")
	}

	return result
}

// StripThinkTags 移除所有思考标签及其内容
// 这是 stream.go 中 stripThinkTags 的增强版本，支持更多变体
func StripThinkTags(content string) string {
	if content == "" {
		return ""
	}

	// 首先规范化标签
	normalized := NormalizeThinkTags(content)

	// 移除 <think>...</think> 及其内容
	result := normalized
	maxIterations := 100 // 防止无限循环

	for i := 0; i < maxIterations; i++ {
		startIdx := strings.Index(result, "<think>")
		if startIdx == -1 {
			break
		}

		endIdx := strings.Index(result[startIdx:], "</think>")
		if endIdx == -1 {
			// 没有结束标签，只移除开始标签
			result = result[:startIdx] + result[startIdx+7:]
			continue
		}
		endIdx += startIdx + 8 // 加上 </think> 的长度

		// 移除整个 think 标签及其内容
		result = result[:startIdx] + result[endIdx:]
	}

	return result
}

// ExtractThinkContent 提取思考标签内的内容
// 返回思考内容和剩余内容
func ExtractThinkContent(content string) (thinkContent string, remainingContent string) {
	if content == "" {
		return "", ""
	}

	// 首先规范化标签
	normalized := NormalizeThinkTags(content)

	var thinks []string
	remaining := normalized

	for {
		startIdx := strings.Index(remaining, "<think>")
		if startIdx == -1 {
			break
		}

		endIdx := strings.Index(remaining[startIdx:], "</think>")
		if endIdx == -1 {
			break
		}

		// 提取思考内容
		innerStart := startIdx + 7
		innerEnd := startIdx + endIdx
		if innerEnd > innerStart {
			thinks = append(thinks, remaining[innerStart:innerEnd])
		}

		// 从剩余内容中移除这个 think 标签
		endIdx += startIdx + 8
		remaining = remaining[:startIdx] + remaining[endIdx:]
	}

	return strings.Join(thinks, "\n"), remaining
}

// IsThinkTag 检查字符串是否是思考标签（开头）
func IsThinkTag(content string) bool {
	trimmed := strings.TrimSpace(content)
	lower := strings.ToLower(trimmed)

	for _, variant := range ThinkTagVariants {
		if strings.HasPrefix(lower, "<"+variant+">") {
			return true
		}
	}
	return false
}

// ContainsThinkTags 检查内容是否包含思考标签
func ContainsThinkTags(content string) bool {
	if content == "" {
		return false
	}
	return thinkTagRegex.MatchString(content)
}

// CountThinkTags 统计思考标签的数量
func CountThinkTags(content string) int {
	if content == "" {
		return 0
	}
	return len(thinkTagRegex.FindAllString(content, -1))
}

// ProcessThinkTags 处理思考标签
// 根据配置决定是否保留、移除或提取思考内容
type ThinkTagMode int

const (
	ThinkTagModeStrip ThinkTagMode = iota // 移除思考标签及其内容（默认）
	ThinkTagModePreserve                  // 保留思考标签
	ThinkTagModeExtract                   // 只保留思考内容
)

// ProcessThinkTagsWithMode 根据模式处理思考标签
func ProcessThinkTagsWithMode(content string, mode ThinkTagMode) string {
	switch mode {
	case ThinkTagModePreserve:
		// 只规范化标签，不移除
		return NormalizeThinkTags(content)
	case ThinkTagModeExtract:
		// 只保留思考内容
		think, _ := ExtractThinkContent(content)
		return think
	default: // ThinkTagModeStrip
		// 移除思考标签及其内容
		return StripThinkTags(content)
	}
}
