package cheque

import (
	"fmt"
	"io"
	"math/big"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
	"github.com/TRON-US/go-btfs/settlement/swap/chequebook"
)

type CashChequeRet struct {
	TxHash string
}

type ListChequeRet struct {
	Beneficiary string
	Chequebook  string
	Payout      *big.Int
}

type ChequeRecords struct {
	Records []chequebook.ChequeRecord
	Len     int
}

var ChequeCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Interact with chequebook services on BTFS.",
		ShortDescription: `
Chequebook services include issue cheque to peer, receive cheque and store operations.`,
	},
	Subcommands: map[string]*cmds.Command{
		"cash":    CashChequeCmd,
		"list":    ListChequeCmd,
		"history": ChequeHistoryCmd,
		//"info": ChequeInfo,
	},
}

var ChequeHistoryCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Display the cheque records.",
	},
	Subcommands: map[string]*cmds.Command{
		//"send":    ChequeSendHistoryCmd,
		"receive": ChequeReceiveHistoryCmd,
	},
}

var CashChequeCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Cash a cheque by peerID.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("peer-id", true, false, "Peer id tobe cashed."),
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

var ChequeReceiveHistoryCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Display the received cheques from peer.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("peer-id", true, false, "The peer id of cheques received."),
	},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {

		var listRet ChequeRecords
		peer_id := req.Arguments[0]

		records, err := chain.SettleObject.SwapService.ReceivedChequeRecords(peer_id)
		if err != nil {
			return err
		}

		listRet.Records = records
		listRet.Len = len(records)

		return cmds.EmitOnce(res, &listRet)
	},
	Type: ChequeRecords{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *ChequeRecords) error {
			var tm time.Time
			fmt.Fprintf(w, "\t%-46s\t%-46s\t%-10s\ttimestamp: \n", "beneficiary:", "chequebook:", "amount:")
			for index := 0; index < out.Len; index++ {
				tm = time.Unix(out.Records[index].ReceiveTime, 0)
				year, mon, day := tm.Date()
				h, m, s := tm.Clock()
				fmt.Fprintf(w, "\t%-46s\t%-46s\t%-10d\t%d-%d-%d %02d:%02d:%02d \n",
					out.Records[index].Beneficiary,
					out.Records[index].Chequebook,
					out.Records[index].Amount.Uint64(),
					year, mon, day, h, m, s)
			}

			return nil
		}),
	},
}
