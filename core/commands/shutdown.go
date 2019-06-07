package commands

import (
	cmdenv "github.com/TRON-US/go-btfs/core/commands/cmdenv"

	"github.com/ipfs/go-ipfs-cmdkit"
	cmds "github.com/ipfs/go-ipfs-cmds"
)

var daemonShutdownCmd = &cmds.Command{
	Helptext: cmdkit.HelpText{
		Tagline: "Shut down the btfs daemon",
	},
	Run: func(req *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) error {
		nd, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		if !nd.IsDaemon {
			return cmdkit.Errorf(cmdkit.ErrClient, "daemon not running")
		}

		if err := nd.Process().Close(); err != nil {
			log.Error("error while shutting down btfs daemon:", err)
		}

		return nil
	},
}
