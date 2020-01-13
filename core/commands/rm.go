package commands

import (
	"context"
	"fmt"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"

	cmds "github.com/TRON-US/go-btfs-cmds"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/TRON-US/interface-go-btfs-core/options"
	"github.com/TRON-US/interface-go-btfs-core/path"
	ipld "github.com/ipfs/go-ipld-format"
)

var RmCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "Remove files or directories from a local btfs node.",
		ShortDescription: `Removes all blocks under <hash> recursively from a local btfs node.`,
	},

	Arguments: []cmds.Argument{
		cmds.StringArg("hash", true, true, "The hash(es) of the file(s)/directory(s) to be removed from the local btfs node.").EnableStdin(),
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

		var results stringList
		for _, b := range req.Arguments {
			// Make sure node exists
			p := path.New(b)
			node, err := api.ResolveNode(req.Context, p)
			if err != nil {
				return err
			}

			_, pinned, err := n.Pinning.IsPinned(node.Cid())
			if err != nil {
				return err
			}
			if pinned {
				// Since we are removing a file, we need to set recursive flag to true
				err = api.Pin().Rm(req.Context, p, options.Pin.RmRecursive(true))
				if err != nil {
					return err
				}
			}

			// Rm all child links
			err = rmAllDags(req.Context, api, node, &results)
			if err != nil {
				return err
			}
		}

		return cmds.EmitOnce(res, results)
	},
	Type: stringList{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(stringListEncoder),
	},
}

func rmAllDags(ctx context.Context, api coreiface.CoreAPI, node ipld.Node, res *stringList) error {
	for _, nl := range node.Links() {
		// Resolve, recurse, then finally remove
		rn, err := api.ResolveNode(ctx, path.IpfsPath(nl.Cid))
		if err != nil {
			return err
		}
		if err := rmAllDags(ctx, api, rn, res); err != nil {
			return err
		}
	}
	ncid := node.Cid()
	if err := api.Dag().Remove(ctx, ncid); err != nil {
		res.Strings = append(res.Strings, fmt.Sprintf("Error removing object %s", ncid))
		return err
	} else {
		res.Strings = append(res.Strings, fmt.Sprintf("Removed %s", ncid))
	}
	return nil
}
