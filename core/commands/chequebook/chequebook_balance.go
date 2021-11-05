package chequebook

import (
	"fmt"
	"io"
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
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *ChequeBookBalanceCmdRet) error {
			_, err := fmt.Fprintf(w, "the chequebook available balance: %s", out.Balance)
			return err
		}),
	},
}
