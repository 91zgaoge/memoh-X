package wecom

import (
	"log/slog"

	"github.com/Kxiandaoyan/Memoh-v2/internal/channel"
)

// ProvideWeComAdapter creates and returns a WeCom channel adapter.
func ProvideWeComAdapter(log *slog.Logger) channel.Adapter {
	return NewAdapter(log)
}
