package settlement

import (
	"math/big"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/bigint"
	"github.com/TRON-US/go-btfs/chain"
)

type settlementResponse struct {
	Peer               string         `json:"peer"`
	SettlementReceived *bigint.BigInt `json:"received"`
	SettlementSent     *bigint.BigInt `json:"sent"`
}

type settlementsResponse struct {
	TotalSettlementReceived *bigint.BigInt       `json:"totalReceived"`
	TotalSettlementSent     *bigint.BigInt       `json:"totalSent"`
	Settlements             []settlementResponse `json:"settlements"`
}

var ListSettlementCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "list all settlements.",
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		settlementsSent, err := chain.SettleObject.SwapService.SettlementsSent()
		if err != nil {
			return err
		}
		settlementsReceived, err := chain.SettleObject.SwapService.SettlementsReceived()
		if err != nil {
			return err
		}

		totalReceived := big.NewInt(0)
		totalSent := big.NewInt(0)

		settlementResponses := make(map[string]settlementResponse)

		for a, b := range settlementsSent {
			settlementResponses[a] = settlementResponse{
				Peer:               a,
				SettlementSent:     bigint.Wrap(b),
				SettlementReceived: bigint.Wrap(big.NewInt(0)),
			}
			totalSent.Add(b, totalSent)
		}

		for a, b := range settlementsReceived {
			if _, ok := settlementResponses[a]; ok {
				t := settlementResponses[a]
				t.SettlementReceived = bigint.Wrap(b)
				settlementResponses[a] = t
			} else {
				settlementResponses[a] = settlementResponse{
					Peer:               a,
					SettlementSent:     bigint.Wrap(big.NewInt(0)),
					SettlementReceived: bigint.Wrap(b),
				}
			}
			totalReceived.Add(b, totalReceived)
		}
		settlementResponsesArray := make([]settlementResponse, len(settlementResponses))
		i := 0
		for k := range settlementResponses {
			settlementResponsesArray[i] = settlementResponses[k]
			i++
		}

		rsp := settlementsResponse{
			TotalSettlementReceived: bigint.Wrap(totalReceived),
			TotalSettlementSent:     bigint.Wrap(totalSent),
			Settlements:             settlementResponsesArray,
		}

		return cmds.EmitOnce(res, &rsp)
	},
	Type: &settlementsResponse{},
}
