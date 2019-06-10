package libp2p

import (
	host "github.com/libp2p/go-libp2p-host"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/fx"

	"github.com/TRON-US/go-btfs/core/node/helpers"
)

func FloodSub(pubsubOptions ...pubsub.Option) interface{} {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host) (service *pubsub.PubSub, err error) {
		return pubsub.NewFloodSub(helpers.LifecycleCtx(mctx, lc), host, pubsubOptions...)
	}
}

func GossipSub(pubsubOptions ...pubsub.Option) interface{} {
	return func(mctx helpers.MetricsCtx, lc fx.Lifecycle, host host.Host) (service *pubsub.PubSub, err error) {
		return pubsub.NewGossipSub(helpers.LifecycleCtx(mctx, lc), host, pubsubOptions...)
	}
}
