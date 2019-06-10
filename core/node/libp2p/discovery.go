package libp2p

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p-host"
	"github.com/libp2p/go-libp2p-peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery"
	"go.uber.org/fx"

	"github.com/TRON-US/go-btfs/core/node/helpers"
)

const discoveryConnTimeout = time.Second * 30

type discoveryHandler struct {
	ctx  context.Context
	host host.Host
}

func (dh *discoveryHandler) HandlePeerFound(p peerstore.PeerInfo) {
	log.Warning("trying peer info: ", p)
	ctx, cancel := context.WithTimeout(dh.ctx, discoveryConnTimeout)
	defer cancel()
	if err := dh.host.Connect(ctx, p); err != nil {
		log.Warning("Failed to connect to peer found by discovery: ", err)
	}
}

func DiscoveryHandler(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host) *discoveryHandler {
	return &discoveryHandler{
		ctx:  helpers.LifecycleCtx(mctx, lc),
		host: host,
	}
}

func SetupDiscovery(mdns bool, mdnsInterval int) func(helpers.MetricsCtx, fx.Lifecycle, host.Host, *discoveryHandler) error {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host, handler *discoveryHandler) error {
		if mdns {
			if mdnsInterval == 0 {
				mdnsInterval = 5
			}
			service, err := discovery.NewMdnsService(helpers.LifecycleCtx(mctx, lc), host, time.Duration(mdnsInterval)*time.Second, discovery.ServiceTag)
			if err != nil {
				log.Error("mdns error: ", err)
				return nil
			}
			service.RegisterNotifee(handler)
		}
		return nil
	}
}
