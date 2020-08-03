package storage

import (
	"fmt"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"

	cmds "github.com/TRON-US/go-btfs-cmds"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"
)

const (
	RoleHost   = "host"
	RoleRenter = "renter"
)

var StorageInfoCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Show storage host information.",
		ShortDescription: `
This command displays host information synchronized from the BTFS network.
By default it shows local host node information.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("peer-id", false, false, "Peer ID to show storage-related information. Default to self.").EnableStdin(),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if len(req.Arguments) > 0 {
			if !cfg.Experimental.StorageClientEnabled {
				return fmt.Errorf("storage client api not enabled")
			}
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		// Default to self
		var data *nodepb.Node_Settings
		var peerID string
		if len(req.Arguments) > 0 {
			peerID = req.Arguments[0]
			data, err = helper.GetHostStorageConfigForPeer(n, peerID)
		} else {
			data, err = helper.GetHostStorageConfig(req.Context, n)
		}
		if err != nil {
			return err
		}
		if cfg.Experimental.StorageHostEnabled {
			data.Role = RoleHost
		} else if cfg.Experimental.StorageClientEnabled {
			data.Role = RoleRenter
		}
		return cmds.EmitOnce(res, data)
	},
	Type: nodepb.Node_Settings{},
}
