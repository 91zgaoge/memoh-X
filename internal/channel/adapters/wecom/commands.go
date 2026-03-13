package wecom

import (
	"strings"
)

// DefaultCommandAllowlist 默认命令白名单
var DefaultCommandAllowlist = []string{
	"/new",
	"/clear",
	"/help",
}

// HighPriorityCommands 高优先级命令，始终允许执行
var HighPriorityCommands = map[string]bool{
	"/new":   true,
	"/clear": true,
}

// CommandCheckResult 命令检查结果
type CommandCheckResult struct {
	IsCommand bool   // 是否是命令
	Allowed   bool   // 是否允许执行
	Command   string // 命令名称（小写）
}

// CheckCommandAllowlist 检查命令是否在白名单中
// message: 用户消息内容
// allowlist: 允许执行的命令列表
// 返回: CommandCheckResult 包含检查结果
func CheckCommandAllowlist(message string, allowlist []string) CommandCheckResult {
	trimmed := strings.TrimSpace(message)

	// 不是斜杠命令
	if !strings.HasPrefix(trimmed, "/") {
		return CommandCheckResult{
			IsCommand: false,
			Allowed:   true,
			Command:   "",
		}
	}

	// 使用第一个 token 作为命令
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return CommandCheckResult{
			IsCommand: false,
			Allowed:   true,
			Command:   "",
		}
	}

	command := strings.ToLower(parts[0])

	// 高优先级命令始终允许
	if HighPriorityCommands[command] {
		return CommandCheckResult{
			IsCommand: true,
			Allowed:   true,
			Command:   command,
		}
	}

	// 检查白名单
	allowed := false
	for _, cmd := range allowlist {
		if strings.ToLower(cmd) == command {
			allowed = true
			break
		}
	}

	return CommandCheckResult{
		IsCommand: true,
		Allowed:   allowed,
		Command:   command,
	}
}

// IsHighPriorityCommand 检查命令是否是高优先级命令
func IsHighPriorityCommand(command string) bool {
	if command == "" {
		return false
	}
	return HighPriorityCommands[strings.ToLower(command)]
}

// ExtractLeadingSlashCommand 提取消息开头的斜杠命令
func ExtractLeadingSlashCommand(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "/") {
		return ""
	}

	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return ""
	}

	return strings.ToLower(parts[0])
}

// IsCommandBlocked 检查命令是否被阻止（用于回复提示）
// 返回被阻止的命令名称，如果没有被阻止则返回空字符串
func IsCommandBlocked(content string, allowlist []string) string {
	result := CheckCommandAllowlist(content, allowlist)
	if result.IsCommand && !result.Allowed {
		return result.Command
	}
	return ""
}

// BuildBlockMessage 构建命令阻止提示消息
func BuildBlockMessage(command string) string {
	return "该命令不可用。可用的命令：/new, /clear, /help"
}
