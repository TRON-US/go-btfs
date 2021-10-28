package cheque

import (
	"fmt"
	"math/big"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
	"golang.org/x/net/context"
)

type ChequeBookWithdrawCmdRet struct {
	Hash string `json:"hash"`
}

var ChequeBookWithdrawCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Withdraw from chequebook contract account to beneficiary.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("amount", true, false, "withdraw amount."),
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		amount, ok := new(big.Int).SetString(req.Arguments[0], 10)
		if !ok {
			return fmt.Errorf("amount:%s cannot be parsed", req.Arguments[0])
		}
		hash, err := chain.SettleObject.ChequebookService.Withdraw(context.Background(), amount)
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &ChequeBookWithdrawCmdRet{
			Hash: hash.String(),
		})
	},
	Type: &ChequeBookWithdrawCmdRet{},
}
