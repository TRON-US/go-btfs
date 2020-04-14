package node

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/TRON-US/go-btfs/core/node/libp2p"
	"github.com/TRON-US/go-btfs/p2p"

	"github.com/tron-us/go-btfs-common/crypto"

	"github.com/TRON-US/go-btfs-config"
	uio "github.com/TRON-US/go-unixfs/io"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	offroute "github.com/ipfs/go-ipfs-routing/offline"
	util "github.com/ipfs/go-ipfs-util"
	"github.com/ipfs/go-path/resolver"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"go.uber.org/fx"
)

var BaseLibP2P = fx.Options(
	fx.Provide(libp2p.PNet),
	fx.Provide(libp2p.ConnectionManager),
	fx.Provide(libp2p.DefaultTransports),

	fx.Provide(libp2p.Host),

	fx.Provide(libp2p.DiscoveryHandler),

	fx.Invoke(libp2p.PNetChecker),
)

func LibP2P(bcfg *BuildCfg, cfg *config.Config) fx.Option {
	// parse ConnMgr config

	grace := config.DefaultConnMgrGracePeriod
	low := config.DefaultConnMgrLowWater
	high := config.DefaultConnMgrHighWater

	connmgr := fx.Options()

	if cfg.Swarm.ConnMgr.Type != "none" {
		switch cfg.Swarm.ConnMgr.Type {
		case "":
			// 'default' value is the basic connection manager
			break
		case "basic":
			var err error
			grace, err = time.ParseDuration(cfg.Swarm.ConnMgr.GracePeriod)
			if err != nil {
				return fx.Error(fmt.Errorf("parsing Swarm.ConnMgr.GracePeriod: %s", err))
			}

			low = cfg.Swarm.ConnMgr.LowWater
			high = cfg.Swarm.ConnMgr.HighWater
		default:
			return fx.Error(fmt.Errorf("unrecognized ConnMgr.Type: %q", cfg.Swarm.ConnMgr.Type))
		}

		connmgr = fx.Provide(libp2p.ConnectionManager(low, high, grace))
	}

	// parse PubSub config

	ps := fx.Options()
	if bcfg.getOpt("pubsub") || bcfg.getOpt("ipnsps") {
		var pubsubOptions []pubsub.Option
		pubsubOptions = append(
			pubsubOptions,
			pubsub.WithMessageSigning(!cfg.Pubsub.DisableSigning),
			pubsub.WithStrictSignatureVerification(cfg.Pubsub.StrictSignatureVerification),
		)

		switch cfg.Pubsub.Router {
		case "":
			fallthrough
		case "gossipsub":
			ps = fx.Provide(libp2p.GossipSub(pubsubOptions...))
		case "floodsub":
			ps = fx.Provide(libp2p.FloodSub(pubsubOptions...))
		default:
			return fx.Error(fmt.Errorf("unknown pubsub router %s", cfg.Pubsub.Router))
		}
	}

	// Gather all the options

	opts := fx.Options(
		BaseLibP2P,

		fx.Provide(libp2p.AddrFilters(cfg.Swarm.AddrFilters)),
		fx.Provide(libp2p.AddrsFactory(cfg.Addresses.Announce, cfg.Addresses.NoAnnounce)),
		fx.Provide(libp2p.SmuxTransport(bcfg.getOpt("mplex"))),
		fx.Provide(libp2p.Relay(cfg.Swarm.DisableRelay, cfg.Swarm.EnableRelayHop)),
		fx.Invoke(libp2p.StartListening(cfg.Addresses.Swarm)),
		fx.Invoke(libp2p.SetupDiscovery(cfg.Discovery.MDNS.Enabled, cfg.Discovery.MDNS.Interval)),

		fx.Provide(libp2p.Security(!bcfg.DisableEncryptedConnections, cfg.Experimental.PreferTLS)),

		fx.Provide(libp2p.Routing),
		fx.Provide(libp2p.BaseRouting),
		maybeProvide(libp2p.PubsubRouter, bcfg.getOpt("ipnsps")),

		maybeProvide(libp2p.BandwidthCounter, !cfg.Swarm.DisableBandwidthMetrics),
		maybeProvide(libp2p.NatPortMap, !cfg.Swarm.DisableNatPortMap),
		maybeProvide(libp2p.AutoRealy, cfg.Swarm.EnableAutoRelay),
		maybeProvide(libp2p.QUIC, cfg.Experimental.QUIC),
		maybeInvoke(libp2p.AutoNATService(cfg.Experimental.QUIC), cfg.Swarm.EnableAutoNATService),
		connmgr,
		ps,
	)

	return opts
}

// Storage groups units which setup datastore based persistence and blockstore layers
func Storage(bcfg *BuildCfg, cfg *config.Config) fx.Option {
	cacheOpts := blockstore.DefaultCacheOpts()
	cacheOpts.HasBloomFilterSize = cfg.Datastore.BloomFilterSize
	if !bcfg.Permanent {
		cacheOpts.HasBloomFilterSize = 0
	}

	finalBstore := fx.Provide(GcBlockstoreCtor)
	if cfg.Experimental.FilestoreEnabled || cfg.Experimental.UrlstoreEnabled {
		finalBstore = fx.Provide(FilestoreBlockstoreCtor)
	}

	return fx.Options(
		fx.Provide(RepoConfig),
		fx.Provide(Datastore),
		fx.Provide(BaseBlockstoreCtor(cacheOpts, bcfg.NilRepo, cfg.Datastore.HashOnRead)),
		finalBstore,
	)
}

// Identity groups units providing cryptographic identity
func Identity(cfg *config.Config) fx.Option {
	// PeerID

	cid := cfg.Identity.PeerID
	if cid == "" {
		return fx.Error(errors.New("identity was not set in config (was 'btfs init' run?)"))
	}
	if len(cid) == 0 {
		return fx.Error(errors.New("no peer ID in config! (was 'btfs init' run?)"))
	}

	id, err := peer.IDB58Decode(cid)
	if err != nil {
		return fx.Error(fmt.Errorf("peer ID invalid: %s", err))
	}

	// Private Key
	// Use env override if available
	pk := os.Getenv("BTFS_PRIV_KEY")
	if pk != "" {
		// Override stored peer id
		cfg.Identity.PrivKey = pk
	}

	if cfg.Identity.PrivKey == "" {
		return fx.Options( // No PK (usually in tests)
			fx.Provide(PeerID(id)),
			fx.Provide(pstoremem.NewPeerstore),
		)
	}

	sk, err := crypto.GetPrivKeyFromHexOrBase64(cfg.Identity.PrivKey)
	if err != nil {
		return fx.Error(err)
	}

	// Set correct peer id from overriden private key
	if pk != "" {
		pid, err := peer.IDFromPublicKey(sk.GetPublic())
		if err != nil {
			return fx.Error(err)
		}
		cfg.Identity.PeerID = pid.String()
		id = pid
	}

	return fx.Options( // Full identity
		fx.Provide(PeerID(id)),
		fx.Provide(PrivateKey(sk)),
		fx.Provide(pstoremem.NewPeerstore),

		fx.Invoke(libp2p.PstoreAddSelfKeys),
	)
}

// IPNS groups namesys related units
var IPNS = fx.Options(
	fx.Provide(RecordValidator),
)

// Online groups online-only units
func Online(bcfg *BuildCfg, cfg *config.Config) fx.Option {

	// Namesys params

	ipnsCacheSize := cfg.Ipns.ResolveCacheSize
	if ipnsCacheSize == 0 {
		ipnsCacheSize = DefaultIpnsCacheSize
	}
	if ipnsCacheSize < 0 {
		return fx.Error(fmt.Errorf("cannot specify negative resolve cache size"))
	}

	// Republisher params

	var repubPeriod, recordLifetime time.Duration

	if cfg.Ipns.RepublishPeriod != "" {
		d, err := time.ParseDuration(cfg.Ipns.RepublishPeriod)
		if err != nil {
			return fx.Error(fmt.Errorf("failure to parse config setting BTNS.RepublishPeriod: %s", err))
		}

		if !util.Debug && (d < time.Minute || d > (time.Hour*24)) {
			return fx.Error(fmt.Errorf("config setting BTNS.RepublishPeriod is not between 1min and 1day: %s", d))
		}

		repubPeriod = d
	}

	if cfg.Ipns.RecordLifetime != "" {
		d, err := time.ParseDuration(cfg.Ipns.RecordLifetime)
		if err != nil {
			return fx.Error(fmt.Errorf("failure to parse config setting BTNS.RecordLifetime: %s", err))
		}

		recordLifetime = d
	}

	/* don't provide from bitswap when the strategic provider service is active */
	shouldBitswapProvide := !cfg.Experimental.StrategicProviding

	return fx.Options(
		fx.Provide(OnlineExchange(shouldBitswapProvide)),
		fx.Provide(Namesys(ipnsCacheSize)),

		fx.Invoke(IpnsRepublisher(repubPeriod, recordLifetime)),

		fx.Provide(p2p.New),

		LibP2P(bcfg, cfg),
		OnlineProviders(cfg.Experimental.StrategicProviding, cfg.Reprovider.Strategy, cfg.Reprovider.Interval),
	)
}

// Offline groups offline alternatives to Online units
func Offline(cfg *config.Config) fx.Option {
	return fx.Options(
		fx.Provide(offline.Exchange),
		fx.Provide(Namesys(0)),
		fx.Provide(offroute.NewOfflineRouter),
		OfflineProviders(cfg.Experimental.StrategicProviding, cfg.Reprovider.Strategy, cfg.Reprovider.Interval),
	)
}

// Core groups basic BTFS services
var Core = fx.Options(
	fx.Provide(BlockService),
	fx.Provide(Dag),
	fx.Provide(resolver.NewBasicResolver),
	fx.Provide(Pinning),
	fx.Provide(Files),
)

func Networked(bcfg *BuildCfg, cfg *config.Config) fx.Option {
	if bcfg.Online {
		return Online(bcfg, cfg)
	}
	return Offline(cfg)
}

// BTFS builds a group of fx Options based on the passed BuildCfg
func IPFS(ctx context.Context, bcfg *BuildCfg) fx.Option {
	if bcfg == nil {
		bcfg = new(BuildCfg)
	}

	bcfgOpts, cfg := bcfg.options(ctx)
	if cfg == nil {
		return bcfgOpts // error
	}

	// TEMP: setting global sharding switch here
	uio.UseHAMTSharding = cfg.Experimental.ShardingEnabled

	return fx.Options(
		bcfgOpts,

		fx.Provide(baseProcess),

		Storage(bcfg, cfg),
		Identity(cfg),
		IPNS,
		Networked(bcfg, cfg),

		Core,
	)
}
