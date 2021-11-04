package cheque

import (
	"fmt"
	"io"
	"math/big"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
	"golang.org/x/net/context"
)

var ListReceiveChequeCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "List cheque(s) received from peers.",
	},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {

		var listRet ListChequeRet
		cheques, err := chain.SettleObject.SwapService.LastReceivedCheques()
		fmt.Println("ListReceiveChequeCmd ", cheques, err)
		if err != nil {
			return err
		}

		for k, v := range cheques {
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
