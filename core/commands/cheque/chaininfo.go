package cheque

import (
	"fmt"
	"io"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
)

type ChainInfoRet struct {
	ChainId        int64  `json:"chain_id"`
	NodeAddr       string `json:"node_addr"`
	ChequeBookAddr string `json:"cheque_book_addr"`
	ReceiverAddr   string `json:"receiver_addr"`
}

var ChequeChainInfoCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Show current chain info.",
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		receiver, err := chain.SettleObject.ChequebookService.Receiver()
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &ChainInfoRet{
			ChainId:        chain.ChainObject.ChainID,
			NodeAddr:       chain.ChainObject.OverlayAddress.String(),
			ChequeBookAddr: chain.SettleObject.ChequebookService.Address().String(),
			ReceiverAddr:   receiver.String(),
		})
	},
	Type: &ChainInfoRet{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *ChainInfoRet) error {
			_, err := fmt.Fprintf(w, "chain id: %d\t node addr: %s\t chequebook addr:%s\treceiver addr:%s\n", out.ChainId,
				out.NodeAddr, out.ChequeBookAddr, out.ReceiverAddr)
			return err
		}),
	},
}
