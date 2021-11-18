package chequebook

import (
	"context"
	"fmt"
	"io"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
	"github.com/ethereum/go-ethereum/common"
)

type ChequeBookReceiverCmdRet struct {
	Addr string `json:"addr"`
}

var ChequeBookReceiverCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get chequebook receiver address.",
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		addr, err := chain.SettleObject.ChequebookService.Receiver()
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &ChequeBookReceiverCmdRet{
			Addr: addr.String(),
		})
	},
	Type: &ChequeBookReceiverCmdRet{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *ChequeBookReceiverCmdRet) error {
			_, err := fmt.Fprintf(w, "the receiver addr : %s", out.Addr)
			return err
		}),
	},
}

type ChequeBookSetReceiverCmdRet struct {
	TxHash string `json:"tx_hash"`
}

var ChequeBookSetReceiverCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Set chequebook receiver address.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("addr", true, false, "new receiver address"),
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		newReceiver := req.Arguments[0]
		hash, err := chain.SettleObject.ChequebookService.SetReceiver(context.Background(), common.HexToAddress(newReceiver))
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &ChequeBookSetReceiverCmdRet{
			TxHash: hash.String(),
		})
	},
	Type: &ChequeBookSetReceiverCmdRet{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *ChequeBookSetReceiverCmdRet) error {
			_, err := fmt.Fprintf(w, "set new receiver tx hash : %s", out.TxHash)
			return err
		}),
	},
}
