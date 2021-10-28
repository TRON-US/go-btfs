package cheque

import (
	"fmt"
	"math/big"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
	"golang.org/x/net/context"
)

type ChequeBookDepositCmdRet struct {
	Hash string `json:"hash"`
}

var ChequeBookDepositCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Deposit from beneficiary to chequebook contract account.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("amount", true, false, "deposit amount."),
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		amount, ok := new(big.Int).SetString(req.Arguments[0], 10)
		if !ok {
			return fmt.Errorf("amount:%s cannot be parsed", req.Arguments[0])
		}
		hash, err := chain.SettleObject.ChequebookService.Deposit(context.Background(), amount)
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &ChequeBookDepositCmdRet{
			Hash: hash.String(),
		})
	},
	Type: &ChequeBookDepositCmdRet{},
}
