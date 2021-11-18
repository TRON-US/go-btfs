package cheque

import (
	"fmt"
	"io"
	"math/big"
	"strconv"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
	"go4.org/sort"
	"golang.org/x/net/context"
)

var ListReceiveChequeCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "List cheque(s) received from peers.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("offset", true, false, "page offset"),
		cmds.StringArg("limit", true, false, "page limit."),
	},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		offset, err := strconv.Atoi(req.Arguments[0])
		if err != nil {
			return fmt.Errorf("parse offset:%v failed", req.Arguments[0])
		}
		limit, err := strconv.Atoi(req.Arguments[1])
		if err != nil {
			return fmt.Errorf("parse limit:%v failed", req.Arguments[1])
		}

		var listRet ListChequeRet
		cheques, err := chain.SettleObject.SwapService.LastReceivedCheques()
		fmt.Println("ListReceiveChequeCmd ", cheques, err)
		if err != nil {
			return err
		}
		peerIds := make([]string, 0, len(cheques))
		for key := range cheques {
			peerIds = append(peerIds, key)
		}
		sort.Strings(peerIds)
		//[offset:offset+limit]
		if len(peerIds) < offset+1 {
			peerIds = peerIds[0:0]
		} else {
			peerIds = peerIds[offset:]
		}

		if len(peerIds) > limit {
			peerIds = peerIds[:limit]
		}

		for _, k := range peerIds {
			v := cheques[k]
			var record cheque
			record.PeerID = k
			record.Beneficiary = v.Beneficiary.String()
			record.Chequebook = v.Chequebook.String()
			record.Payout = v.CumulativePayout

			cashStatus, err := chain.SettleObject.CashoutService.CashoutStatus(context.Background(), v.Chequebook)
			if err != nil {
				return err
			}
			if cashStatus.UncashedAmount != nil {
				record.CashedAmount = big.NewInt(0).Sub(v.CumulativePayout, cashStatus.UncashedAmount)
			}

			listRet.Cheques = append(listRet.Cheques, record)
		}

		listRet.Len = len(listRet.Cheques)
		return cmds.EmitOnce(res, &listRet)
	},
	Type: ListChequeRet{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *ListChequeRet) error {
			fmt.Fprintf(w, "\t%-55s\t%-46s\t%-46s\t%-46s\tamount: \n", "peerID:", "chequebook:", "beneficiary:", "cashout_amount:")
			for iter := 0; iter < out.Len; iter++ {
				fmt.Fprintf(w, "\t%-55s\t%-46s\t%-46s\t%d\t%d \n",
					out.Cheques[iter].PeerID,
					out.Cheques[iter].Beneficiary,
					out.Cheques[iter].Chequebook,
					out.Cheques[iter].Payout.Uint64(),
					out.Cheques[iter].CashedAmount.Uint64(),
				)
			}

			return nil
		}),
	},
}
