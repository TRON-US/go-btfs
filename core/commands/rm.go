package commands

import (
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/namesys/resolve"
	cmds "github.com/TRON-US/go-btfs-cmds"
	path2 "github.com/ipfs/go-path"
	"github.com/ipfs/interface-go-ipfs-core/options"
	"github.com/ipfs/interface-go-ipfs-core/path"
)

var RmCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "Remove a file or directory from a local btfs node.",
		ShortDescription: `Removes contents of <hash> from a local btfs node.`,
	},

	Arguments: []cmds.Argument{
		cmds.StringArg("hash", true, true, "The hash of the file to be removed from local btfs node.").EnableStdin(),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}

		// Since we are removing a file, we need to set recursive flag to true
		recursive := true

		if err := req.ParseBodyArgs(); err != nil {
			return err
		}

		for _, b := range req.Arguments {
			rp, err := api.ResolvePath(req.Context, path.New(b))
			if err != nil {
				return err
			}

			api.Pin().Rm(req.Context, rp, options.Pin.RmRecursive(recursive))
		}

		// Surgincal approach
		p, err := path2.ParsePath(req.Arguments[0])
		if err != nil {
			return err
		}

		object, err := resolve.Resolve(req.Context, n.Namesys, n.Resolver, p)
		if err != nil {
			return err
		}

		// rm all child links
		for _, cid := range object.Links() {
			if err := api.Dag().Remove(req.Context, cid.Cid); err != nil {
				res.Emit("Error removing object " + cid.Cid.String())
			} else {
				res.Emit("Removed " + cid.Cid.String())
			}
		}

		// rm parent node
		if err := api.Dag().Remove(req.Context, object.Cid()); err == nil {
			res.Emit("Removed " + object.Cid().String())
		}

		return nil
	},
}
