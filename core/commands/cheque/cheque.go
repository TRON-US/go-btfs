package cheque

import (
	"fmt"
	"io"
	"math/big"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
)

type CashChequeRet struct {
	TxHash string
}

type ListChequeRet struct {
	Beneficiary string
	Chequebook  string
	Payout      *big.Int
}

var ChequeCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Interact with chequebook services on BTFS.",
		ShortDescription: `
Chequebook services include issue cheque to peer, receive cheque and store operations.`,
	},
	Subcommands: map[string]*cmds.Command{
		"cash": CashChequeCmd,
		"list": ListChequeCmd,
		//"info": ChequeInfo,
	},
}

var CashChequeCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Cash a cheque by peerID.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("peer-id", false, false, "Peer id tobe cashed."),
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {

		// get the peer id
		peerID := req.Arguments[0]
		tx_hash, err := chain.SettleObject.SwapService.CashCheque(req.Context, peerID)
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &CashChequeRet{
			TxHash: tx_hash.String(),
		})
	},
	Type: CashChequeRet{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *CashChequeRet) error {
			_, err := fmt.Fprintf(w, "the hash of transaction: %s", out.TxHash)
			return err
		}),
	},
}

var ListChequeCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "List cheque(s) received from peers.",
	},
	Options: []cmds.Option{
		cmds.StringOption("peer-id", "", "The peer id of the cheque issued by."),
	},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {

		var listRet ListChequeRet
		peer_id, ok := req.Options["peer-id"].(string)
		if ok {
			cheque, err := chain.SettleObject.SwapService.LastReceivedCheque(peer_id)
			if err != nil {
				return err
			}

			listRet.Beneficiary = cheque.Beneficiary.String()
			listRet.Chequebook = cheque.Chequebook.String()
			listRet.Payout = cheque.CumulativePayout

		} else {
			//TODO

		}

		return cmds.EmitOnce(res, &listRet)
	},
	Type: ListChequeRet{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *ListChequeRet) error {
			fmt.Fprintln(w, "cheque status:")
			fmt.Fprintf(w, "\tBeneficiary address: %s\n", out.Beneficiary)
			fmt.Fprintf(w, "\tchequebook address: %s\n", out.Chequebook)
			fmt.Fprintf(w, "\tcumulativePayout: %d\n", out.Payout.Uint64())

			return nil
		}),
	},
}
