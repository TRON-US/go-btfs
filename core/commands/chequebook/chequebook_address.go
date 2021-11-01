package chequebook

import (
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
)

type ChequeBookAddrCmdRet struct {
	Addr string `json:"addr"`
}

var ChequeBookAddrCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get chequebook address.",
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		addr := chain.SettleObject.ChequebookService.Address()

		return cmds.EmitOnce(res, &ChequeBookAddrCmdRet{
			Addr: addr.String(),
		})
	},
	Type: &ChequeBookAddrCmdRet{},
}
