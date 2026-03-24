package weixin

import (
	"fmt"
	"strings"

	"github.com/Kxiandaoyan/Memoh-v2/internal/channel"
)

// Config holds the WeChat personal account credentials extracted from a channel configuration.
// WeChat personal uses bridge/webhook mode, so the API key is system-generated.
type Config struct {
	// No user-provided credentials needed for webhook mode
}

// UserConfig holds the identifiers used to target a WeChat personal account user.
type UserConfig struct {
	WeixinID string
}

func normalizeConfig(raw map[string]any) (map[string]any, error) {
	// WeChat personal uses webhook mode — preserve the system-generated api_key
	result := map[string]any{}
	if v := channel.ReadString(raw, "api_key", "apiKey"); v != "" {
		result["api_key"] = v
	}
	return result, nil
}

func normalizeUserConfig(raw map[string]any) (map[string]any, error) {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return nil, err
	}
	result := map[string]any{}
	if cfg.WeixinID != "" {
		result["weixin_id"] = cfg.WeixinID
	}
	return result, nil
}

func resolveTarget(raw map[string]any) (string, error) {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return "", err
	}
	if cfg.WeixinID != "" {
		return cfg.WeixinID, nil
	}
	return "", fmt.Errorf("weixin binding is incomplete: weixin_id is required in the channel binding configuration")
}

func matchBinding(raw map[string]any, criteria channel.BindingCriteria) bool {
	cfg, err := parseUserConfig(raw)
	if err != nil {
		return false
	}
	if value := strings.TrimSpace(criteria.Attribute("weixin_id")); value != "" && value == cfg.WeixinID {
		return true
	}
	if criteria.SubjectID != "" && criteria.SubjectID == cfg.WeixinID {
		return true
	}
	return false
}

func buildUserConfig(identity channel.Identity) map[string]any {
	result := map[string]any{}
	if value := strings.TrimSpace(identity.Attribute("weixin_id")); value != "" {
		result["weixin_id"] = value
	} else if identity.SubjectID != "" {
		result["weixin_id"] = identity.SubjectID
	}
	return result
}

func parseConfig(_ map[string]any) (Config, error) {
	// No credentials needed for webhook mode
	return Config{}, nil
}

func parseUserConfig(raw map[string]any) (UserConfig, error) {
	weixinID := strings.TrimSpace(channel.ReadString(raw, "weixin_id", "weixinId", "wxid"))
	if weixinID == "" {
		return UserConfig{}, fmt.Errorf("weixin user config requires weixin_id")
	}
	return UserConfig{WeixinID: weixinID}, nil
}

func normalizeTarget(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "weixin:")
	value = strings.TrimPrefix(value, "wx:")
	return strings.TrimSpace(value)
}
