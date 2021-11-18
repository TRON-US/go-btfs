package cheque

import (
	"math/big"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
)

const (
	CashoutStatusSuccess  = "success"
	CashoutStatusFail     = "fail"
	CashoutStatusPending  = "pending"
	CashoutStatusNotFound = "not_found"
)

type CashOutStatusRet struct {
	Status         string   `json:"status"` // pending,fail,success,not_found
	TotalPayout    *big.Int `json:"total_payout"`
	UncashedAmount *big.Int `json:"uncashed_amount"` // amount not yet cashed out
}

var ChequeCashStatusCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get cash status by peerID.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("peer-id", true, false, "Peer id tobe cashed."),
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {

		// get the peer id
		peerID := req.Arguments[0]
		cashStatus, err := chain.SettleObject.SwapService.CashoutStatus(req.Context, peerID)
		if err != nil {
			return err
		}

		status := CashoutStatusSuccess
		totalPayout := big.NewInt(0)
		if cashStatus.Last == nil {
			status = CashoutStatusNotFound
		} else if cashStatus.Last.Reverted {
			status = CashoutStatusFail
		} else if cashStatus.Last.Result == nil {
			status = CashoutStatusPending
		} else {
			totalPayout = cashStatus.Last.Result.TotalPayout
		}

		return cmds.EmitOnce(res, &CashOutStatusRet{
			UncashedAmount: cashStatus.UncashedAmount,
			Status:         status,
			TotalPayout:    totalPayout,
		})
	},
	Type: &CashOutStatusRet{},
}
