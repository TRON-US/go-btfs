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

const (
	rmForceOptionName = "force"
)

var RmCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "Remove files or directories from a local btfs node.",
		ShortDescription: `Removes all blocks under <hash> recursively from a local btfs node.`,
	},

	Arguments: []cmds.Argument{
		cmds.StringArg("hash", true, true, "The hash(es) of the file(s)/directory(s) to be removed from the local btfs node.").EnableStdin(),
	},
	Options: []cmds.Option{
		cmds.BoolOption(rmForceOptionName, "f", "Forcibly remove the object, even if there are constraints (such as host-stored file).").WithDefault(false),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		// do not perform online operations for local delete
		api, err := cmdenv.GetApi(env, req, options.Api.Offline(true), options.Api.FetchBlocks(false))
		if err != nil {
			return err
		}

		force, _ := req.Options[rmForceOptionName].(bool)

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
				err = api.Pin().Rm(req.Context, p, options.Pin.RmRecursive(true), options.Pin.RmForce(force))
				if err != nil {
					return err
				}
			}

			// Rm all child links
			err = rmAllDags(req.Context, api, node, &results, map[string]bool{})
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

func rmAllDags(ctx context.Context, api coreiface.CoreAPI, node ipld.Node, res *stringList,
	removed map[string]bool) error {
	for _, nl := range node.Links() {
		// Skipped just removed nodes
		if _, ok := removed[nl.Cid.String()]; ok {
			continue
		}
		// Resolve, recurse, then finally remove
		rn, err := api.ResolveNode(ctx, path.IpfsPath(nl.Cid))
		if err != nil {
			res.Strings = append(res.Strings, fmt.Sprintf("Error resolving object %s", nl.Cid))
			return err
		}
		if err := rmAllDags(ctx, api, rn, res, removed); err != nil {
			return err
		}
	}
	ncid := node.Cid()
	if err := api.Dag().Remove(ctx, ncid); err != nil {
		res.Strings = append(res.Strings, fmt.Sprintf("Error removing object %s", ncid))
		return err
	} else {
		res.Strings = append(res.Strings, fmt.Sprintf("Removed %s", ncid))
		removed[ncid.String()] = true
	}
	return nil
}
