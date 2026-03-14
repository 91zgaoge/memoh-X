package wecom

import (
	"fmt"
	"strings"
)

// WebSocketConfig holds WeCom WebSocket-specific configuration
type WebSocketConfig struct {
	BotID        string `json:"bot_id"`
	Secret       string `json:"secret"`
	WebsocketURL string `json:"websocket_url,omitempty"`
}

// CommandConfig 命令白名单配置
type CommandConfig struct {
	Enabled   bool     `json:"enabled"`
	Allowlist []string `json:"allowlist"`
}

// Config holds the WeCom channel configuration
type Config struct {
	// WebSocket settings (primary mode)
	BotID        string `json:"bot_id"`
	Secret       string `json:"secret"`
	WebsocketURL string `json:"websocket_url"`

	// Corp credentials for contacts API (directory lookup)
	CorpID     string `json:"corp_id"`
	CorpSecret string `json:"corp_secret"`

	// Agent ID for self-built app messaging
	AgentID string `json:"agent_id"`

	// Group chat settings
	GroupChatEnabled bool `json:"group_chat_enabled"`
	RequireMention   bool `json:"require_mention"`

	// Admin users who bypass restrictions
	AdminUsers []string `json:"admin_users"`

	// Command allowlist settings
	Commands CommandConfig `json:"commands"`

	// Welcome message for enter_chat event
	WelcomeMessage string `json:"welcome_message"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		WebsocketURL:     "wss://openws.work.weixin.qq.com",
		GroupChatEnabled: true,
		RequireMention:   true,
		AdminUsers:       []string{},
		Commands: CommandConfig{
			Enabled:   false, // 默认不启用命令白名单限制
			Allowlist: DefaultCommandAllowlist,
		},
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.BotID == "" {
		return fmt.Errorf("bot_id is required")
	}
	if c.Secret == "" {
		return fmt.Errorf("secret is required")
	}

	// Set default WebSocket URL if not provided
	if c.WebsocketURL == "" {
		c.WebsocketURL = "wss://openws.work.weixin.qq.com"
	}

	return nil
}

// IsAdmin checks if a user is an admin
func (c *Config) IsAdmin(userID string) bool {
	if userID == "" {
		return false
	}

	normalized := strings.ToLower(strings.TrimSpace(userID))
	for _, admin := range c.AdminUsers {
		if strings.ToLower(strings.TrimSpace(admin)) == normalized {
			return true
		}
	}
	return false
}

// CanExecuteCommand checks if a user can execute a command
// Admins bypass the allowlist check
func (c *Config) CanExecuteCommand(userID string, message string) (allowed bool, blocked bool, command string) {
	// Check if it's a command
	cmd := ExtractLeadingSlashCommand(message)
	if cmd == "" {
		// Not a command, always allowed
		return true, false, ""
	}

	// Admin bypass - admins can execute any command
	if c.IsAdmin(userID) {
		return true, false, cmd
	}

	// If command allowlist is not enabled, allow all commands
	if !c.Commands.Enabled {
		return true, false, cmd
	}

	// Check command allowlist
	result := CheckCommandAllowlist(message, c.Commands.Allowlist)
	if result.IsCommand && !result.Allowed {
		return false, true, cmd
	}

	return true, false, cmd
}

// ShouldTriggerGroupResponse checks if a group message should trigger a response
func (c *Config) ShouldTriggerGroupResponse(content string) bool {
	if !c.GroupChatEnabled {
		return false
	}

	if !c.RequireMention {
		return true
	}

	// Check for @mention indicators in the content
	// WeCom formats: @_user_, @<username>, 或者包含"@"的任何格式
	contentLower := strings.ToLower(content)
	mentionIndicators := []string{
		"@_user_",     // 默认格式
		"@<",          // 某些格式使用 <@userid>
		"<@",          // Slack风格
		"@mention",    // 提及标记
	}

	for _, indicator := range mentionIndicators {
		if strings.Contains(contentLower, indicator) {
			return true
		}
	}

	// 检查是否包含 @ 符号（用于群聊中@机器人）
	// 企业微信可能使用不同的格式，如 <@userid> 或 @用户名
	if strings.Contains(content, "@") {
		return true
	}

	// 检查是否包含 < 和 > 组合（可能是富文本@格式）
	if strings.Contains(content, "<") && strings.Contains(content, ">") {
		return true
	}

	return false
}

// ExtractGroupMessageContent removes @mention markers from content
func (c *Config) ExtractGroupMessageContent(content string) string {
	originalContent := content
	// Remove @_user_ prefix if present
	content = strings.TrimPrefix(content, "@_user_")

	// Handle <@userid> format (Slack/WeCom style)
	// Find and remove patterns like <@USERID> or <@USERID|nickname>
	if idx := strings.Index(content, "<@"); idx != -1 {
		endIdx := strings.Index(content[idx:], ">")
		if endIdx != -1 {
			// Remove the entire <@...> part
			before := content[:idx]
			after := content[idx+endIdx+1:]
			content = before + after
		}
	}

	// Handle @mention format
	content = strings.ReplaceAll(content, "@mention", "")

	// If content is empty after removing mentions, return original
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return strings.TrimSpace(originalContent)
	}

	return trimmed
}

// GetWelcomeMessage returns the welcome message for enter_chat event
func (c *Config) GetWelcomeMessage() string {
	if c.WelcomeMessage != "" {
		return c.WelcomeMessage
	}
	return "您好！我是智能助手，有什么可以帮您的吗？"
}

// ParseConfig parses configuration from map
func ParseConfig(raw map[string]any) (*Config, error) {
	cfg := DefaultConfig()

	if botID, ok := raw["bot_id"].(string); ok {
		cfg.BotID = botID
	}
	if secret, ok := raw["secret"].(string); ok {
		cfg.Secret = secret
	}
	if wsURL, ok := raw["websocket_url"].(string); ok {
		cfg.WebsocketURL = wsURL
	}
	if corpID, ok := raw["corp_id"].(string); ok {
		cfg.CorpID = corpID
	}
	if corpSecret, ok := raw["corp_secret"].(string); ok {
		cfg.CorpSecret = corpSecret
	}
	if agentID, ok := raw["agent_id"].(string); ok {
		cfg.AgentID = agentID
	}
	if groupEnabled, ok := raw["group_chat_enabled"].(bool); ok {
		cfg.GroupChatEnabled = groupEnabled
	}
	if requireMention, ok := raw["require_mention"].(bool); ok {
		cfg.RequireMention = requireMention
	}
	if admins, ok := raw["admin_users"].([]any); ok {
		cfg.AdminUsers = make([]string, 0, len(admins))
		for _, item := range admins {
			if s, ok := item.(string); ok {
				cfg.AdminUsers = append(cfg.AdminUsers, s)
			}
		}
	}

	// Parse command allowlist settings
	if commands, ok := raw["commands"].(map[string]any); ok {
		if enabled, ok := commands["enabled"].(bool); ok {
			cfg.Commands.Enabled = enabled
		}
		if allowlist, ok := commands["allowlist"].([]any); ok {
			cfg.Commands.Allowlist = make([]string, 0, len(allowlist))
			for _, item := range allowlist {
				if s, ok := item.(string); ok {
					cfg.Commands.Allowlist = append(cfg.Commands.Allowlist, s)
				}
			}
		}
	}

	// Parse welcome message
	if welcomeMsg, ok := raw["welcome_message"].(string); ok {
		cfg.WelcomeMessage = welcomeMsg
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
