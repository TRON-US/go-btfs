package cheque

import (
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
	"golang.org/x/net/context"
)

type ChequeBookBalanceCmdRet struct {
	Balance string `json:"balance"`
}

var ChequeBookBalanceCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get chequebook balance.",
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		balance, err := chain.SettleObject.ChequebookService.AvailableBalance(context.Background())
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &ChequeBookBalanceCmdRet{
			Balance: balance.String(),
		})
	},
	Type: &ChequeBookBalanceCmdRet{},
}
