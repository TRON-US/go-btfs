package hosts

import (
	"context"
	"fmt"
	"sync"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/coreapi"
	"github.com/TRON-US/go-btfs/core/hub"

	cmds "github.com/TRON-US/go-btfs-cmds"
	logging "github.com/ipfs/go-log"
	hubpb "github.com/tron-us/go-btfs-common/protos/hub"

	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	hostInfoModeOptionName = "host-info-mode"
	hostSyncModeOptionName = "host-sync-mode"
)

var log = logging.Logger("hosts")

var StorageHostsCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "Interact with information on hosts.",
		ShortDescription: `Allows interaction with information on hosts. Host information is synchronized from btfs-hub and saved in local datastore.`,
	},
	Subcommands: map[string]*cmds.Command{
		"info": storageHostsInfoCmd,
		"sync": storageHostsSyncCmd,
	},
}

var storageHostsInfoCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Display saved host information.",
		ShortDescription: `
This command displays saved information from btfs-hub under multiple modes.
Each mode ranks hosts based on its criteria and is randomized based on current node location.

Mode options include:` + hub.AllModeHelpText,
	},
	Options: []cmds.Option{
		cmds.StringOption(hostInfoModeOptionName, "m", "Hosts info showing mode. Default: mode set in config option Experimental.HostsSyncMode."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		mode, ok := req.Options[hostInfoModeOptionName].(string)
		if !ok {
			mode = cfg.Experimental.HostsSyncMode
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		nodes, err := storage.GetHostsFromDatastore(req.Context, n, mode, 0)
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &HostInfoRes{nodes})
	},
	Type: HostInfoRes{},
}

type HostInfoRes struct {
	Nodes []*hubpb.Host
}

var storageHostsSyncCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Synchronize host information from btfs-hub.",
		ShortDescription: `
This command synchronizes information from btfs-hub using multiple modes.
Each mode ranks hosts based on its criteria and is randomized based on current node location.

Mode options include:` + hub.AllModeHelpText,
	},
	Options: []cmds.Option{
		cmds.StringOption(hostSyncModeOptionName, "m", "Hosts syncing mode. Default: mode set in config option Experimental.HostsSyncMode."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		mode, ok := req.Options[hostSyncModeOptionName].(string)
		if !ok {
			mode = cfg.Experimental.HostsSyncMode
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		return SyncHosts(req.Context, n, mode)
	},
}

func SyncHosts(ctx context.Context, node *core.IpfsNode, mode string) error {
	nodes, err := hub.QueryHosts(ctx, node, mode)
	if err != nil {
		return err
	}
	go sortHosts(ctx, node, nodes, mode)
	return storage.SaveHostsIntoDatastore(ctx, node, mode, nodes)
}

func sortHosts(ctx context.Context, n *core.IpfsNode, nodes []*hubpb.Host, mode string) {
	api, err := coreapi.NewCoreAPI(n)
	if err != nil {
		log.Errorf("failed to sort the hosts: %v", err)
		return
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	gs := make([]*hubpb.Host, 0)
	bs := make([]*hubpb.Host, 0)
	for _, h := range nodes {
		wg.Add(1)
		go func(h *hubpb.Host) {
			if err := api.Swarm().Connect(ctx, peer.AddrInfo{ID: peer.ID(h.NodeId)}); err != nil {
				mu.Lock()
				// push back
				bs = append(bs, h)
				mu.Unlock()
			} else {
				mu.Lock()
				// push front
				gs = append(gs, h)
				mu.Unlock()
			}
			wg.Done()
		}(h)
	}
	wg.Wait()
	gs = append(gs, bs...)
	err = storage.SaveHostsIntoDatastore(ctx, n, mode, nodes)
	if err != nil {
		log.Errorf("failed to sort the hosts: %v", err)
		return
	}
}
