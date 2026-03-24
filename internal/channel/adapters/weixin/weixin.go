package weixin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Kxiandaoyan/Memoh-v2/internal/channel"
)

// WeixinAdapter implements channel.Adapter and channel.Sender for WeChat personal accounts.
// WeChat personal uses a bridge/webhook mode for receiving messages (handled by weixin-bridge service),
// so this adapter only needs to implement Send for outbound messages (no-op in bridge mode).
type WeixinAdapter struct {
	logger *slog.Logger
}

// NewWeixinAdapter creates a WeixinAdapter with the given logger.
func NewWeixinAdapter(log *slog.Logger) *WeixinAdapter {
	if log == nil {
		log = slog.Default()
	}
	return &WeixinAdapter{
		logger: log.With(slog.String("adapter", "weixin")),
	}
}

// Type returns the WeChat personal channel type.
func (a *WeixinAdapter) Type() channel.ChannelType {
	return Type
}

// Descriptor returns the WeChat personal channel metadata.
func (a *WeixinAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        Type,
		DisplayName: "微信（个人）",
		Capabilities: channel.ChannelCapabilities{
			Text:        true,
			Markdown:    false,
			Reply:       false,
			Attachments: true,
			Media:       true,
			Streaming:   false,
		},
		ConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields:  map[string]channel.FieldSchema{
				// Empty — api_key is system-generated for webhook bridge mode
			},
		},
		UserConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"weixin_id": {
					Type:     channel.FieldString,
					Required: true,
					Title:    "微信 ID",
				},
			},
		},
		TargetSpec: channel.TargetSpec{
			Format: "weixin_id",
			Hints: []channel.TargetHint{
				{Label: "微信 ID", Example: "wxid_xxxxxxxxxxxxxxx"},
			},
		},
	}
}

// NormalizeConfig validates and normalizes a WeChat personal channel configuration map.
func (a *WeixinAdapter) NormalizeConfig(raw map[string]any) (map[string]any, error) {
	return normalizeConfig(raw)
}

// NormalizeUserConfig validates and normalizes a WeChat personal user-binding configuration map.
func (a *WeixinAdapter) NormalizeUserConfig(raw map[string]any) (map[string]any, error) {
	return normalizeUserConfig(raw)
}

// NormalizeTarget normalizes a WeChat personal delivery target string.
func (a *WeixinAdapter) NormalizeTarget(raw string) string {
	return normalizeTarget(raw)
}

// ResolveTarget derives a delivery target from a WeChat personal user-binding configuration.
func (a *WeixinAdapter) ResolveTarget(userConfig map[string]any) (string, error) {
	return resolveTarget(userConfig)
}

// MatchBinding reports whether a WeChat personal user binding matches the given criteria.
func (a *WeixinAdapter) MatchBinding(config map[string]any, criteria channel.BindingCriteria) bool {
	return matchBinding(config, criteria)
}

// BuildUserConfig constructs a WeChat personal user-binding config from an Identity.
func (a *WeixinAdapter) BuildUserConfig(identity channel.Identity) map[string]any {
	return buildUserConfig(identity)
}

// Send delivers an outbound message via the weixin-bridge service.
// In bridge/webhook mode the reply is returned by the bridge polling mechanism, so Send is a no-op.
func (a *WeixinAdapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.OutboundMessage) error {
	_, err := parseConfig(cfg.Credentials)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("decode config failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
		return err
	}

	to := strings.TrimSpace(msg.Target)
	if to == "" {
		return fmt.Errorf("weixin target (weixin_id) is required")
	}

	if msg.Message.IsEmpty() {
		return fmt.Errorf("message is required")
	}

	// WeChat personal bridge mode uses webhook polling reply.
	// Outbound Send is a no-op — the reply is captured by weixinSyncReplySender in the webhook handler.
	if a.logger != nil {
		a.logger.Info("weixin send (bridge mode, no-op)",
			slog.String("config_id", cfg.ID),
			slog.String("target", to),
		)
	}

	return nil
}
