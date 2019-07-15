package commands

import (
	"fmt"
	"io"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/namesys/resolve"
	cmds "github.com/ipfs/go-ipfs-cmds"
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

		// Remove pins recursively
		enc, err := cmdenv.GetCidEncoder(req)
		if err != nil {
			return err
		}

		pins := make([]string, 0, len(req.Arguments))
		for _, b := range req.Arguments {
			rp, err := api.ResolvePath(req.Context, path.New(b))
			if err != nil {
				return err
			}

			id := enc.Encode(rp.Cid())
			pins = append(pins, id)
			if err := api.Pin().Rm(req.Context, rp, options.Pin.RmRecursive(recursive)); err != nil {
				return err
			}
		}

		rm, _ := req.Options[RemoveOnUnpin].(bool)

		if rm {
			// Run garbage collection
			RepoCmd.Subcommands["gc"].Run(req, res, env)
		} else {
			// Surgincal approach
			p, err := path2.ParsePath(req.Arguments[0])
			if err != nil {
				return err
			}

			object, err := resolve.Resolve(req.Context, n.Namesys, n.Resolver, p)
			if err != nil {
				return err
			}

			for _, cid := range object.Links() {
				n.Blockstore.DeleteBlock(cid.Cid)
			}
		}

		return nil
	},
	Type: GcResult{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, gcr *GcResult) error {
			if gcr.Error != "" {
				_, err := fmt.Fprintf(w, "Error: %s\n", gcr.Error)
				return err
			}

			prefix := "removed "

			_, err := fmt.Fprintf(w, "%s%s\n", prefix, gcr.Key)
			return err
		}),
	},
}
