package qq

import (
	"log/slog"

	"github.com/Kxiandaoyan/Memoh-v2/internal/channel"
	"github.com/Kxiandaoyan/Memoh-v2/internal/channel/identities"
	"github.com/Kxiandaoyan/Memoh-v2/internal/channel/route"
	"github.com/Kxiandaoyan/Memoh-v2/internal/media"
)

func ProvideQQAdapter(log *slog.Logger, mediaService *media.Service, identityService *identities.Service, routeService *route.DBService) channel.Adapter {
	adapter := NewQQAdapter(log)
	adapter.SetAssetOpener(mediaService)
	adapter.SetChannelIdentityResolver(identityService)
	adapter.SetRouteResolver(routeService)
	return adapter
}
