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

// Config holds the WeCom channel configuration
type Config struct {
	// WebSocket settings (primary mode)
	BotID        string `json:"bot_id"`
	Secret       string `json:"secret"`
	WebsocketURL string `json:"websocket_url"`

	// Group chat settings
	GroupChatEnabled bool `json:"group_chat_enabled"`
	RequireMention   bool `json:"require_mention"`

	// Admin users who bypass restrictions
	AdminUsers []string `json:"admin_users"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		WebsocketURL:     "wss://openws.work.weixin.qq.com",
		GroupChatEnabled: true,
		RequireMention:   true,
		AdminUsers:       []string{},
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
	if strings.Contains(content, "@") {
		return true
	}

	return false
}

// ExtractGroupMessageContent removes @mention markers from content
func (c *Config) ExtractGroupMessageContent(content string) string {
	// Remove @_user_ prefix if present
	content = strings.TrimPrefix(content, "@_user_")
	return strings.TrimSpace(content)
}

// GetWelcomeMessage returns the welcome message for enter_chat event
func (c *Config) GetWelcomeMessage() string {
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

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
