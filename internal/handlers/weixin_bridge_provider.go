package handlers

import "log/slog"

// NewWeixinBridgeManagerFunc creates a provider function for WeixinBridgeManager
// that can be used with fx dependency injection
func NewWeixinBridgeManagerFunc(logger *slog.Logger) *WeixinBridgeManager {
	return NewWeixinBridgeManager(logger)
}
