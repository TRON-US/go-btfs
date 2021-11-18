package chequebook

import (
	"fmt"
	"io"
	"math/big"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/net/context"
)

type ChequeBookWbttBalanceCmdRet struct {
	Balance *big.Int `json:"balance"`
}

var ChequeBookWbttBalanceCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get wbtt balance by addr.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("addr", true, false, "wbtt token address"),
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		addr := req.Arguments[0]
		balance, err := chain.SettleObject.ChequebookService.WBTTBalanceOf(context.Background(), common.HexToAddress(addr))
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &ChequeBookBalanceCmdRet{
			Balance: balance,
		})
	},
	Type: &ChequeBookWbttBalanceCmdRet{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *ChequeBookWbttBalanceCmdRet) error {
			_, err := fmt.Fprintf(w, "the balance is: %v\n", out.Balance)
			return err
		}),
	},
}
