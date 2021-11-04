package chequebook

import (
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
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
}
