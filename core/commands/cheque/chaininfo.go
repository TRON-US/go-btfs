package cheque

import (
	"fmt"
	"io"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
)

type ChainInfoRet struct {
	ChainId  int64
	NodeAddr string
}

var ChequeChainInfoCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Show current chain info.",
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {

		return cmds.EmitOnce(res, &ChainInfoRet{
			ChainId:  chain.ChainObject.ChainID,
			NodeAddr: chain.ChainObject.OverlayAddress.String(),
		})
	},
	Type: &ChainInfoRet{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *ChainInfoRet) error {
			_, err := fmt.Fprintf(w, "chain id: %d\t node addr: %s\n", out.ChainId, out.NodeAddr)
			return err
		}),
	},
}
