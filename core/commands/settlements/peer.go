package settlement

import (
	"errors"
	"fmt"
	"math/big"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
	"github.com/TRON-US/go-btfs/settlement"
)

var PeerSettlementCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get chequebook balance.",
	},
	RunTimeout: 5 * time.Minute,
	Arguments: []cmds.Argument{
		cmds.StringArg("peer-id", true, false, "Peer id."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		peerID := req.Arguments[0]
		peerexists := false

		received, err := chain.SettleObject.SwapService.TotalReceived(peerID)
		if err != nil {
			if !errors.Is(err, settlement.ErrPeerNoSettlements) {
				return err
			} else {
				received = big.NewInt(0)
			}
		}

		if err == nil {
			peerexists = true
		}

		sent, err := chain.SettleObject.SwapService.TotalSent(peerID)
		if err != nil {
			if !errors.Is(err, settlement.ErrPeerNoSettlements) {
				return err
			} else {
				sent = big.NewInt(0)
			}
		}

		if err == nil {
			peerexists = true
		}

		if !peerexists {
			return fmt.Errorf("can not get settlements for peer:%s", peerID)
		}

		rsp := settlementResponse{
			Peer:               peerID,
			SettlementReceived: received,
			SettlementSent:     sent,
		}
		return cmds.EmitOnce(res, &rsp)
	},
	Type: &settlementResponse{},
}
