package cheque

import (
	"fmt"
	"io"
	"math/big"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
	"github.com/TRON-US/go-btfs/settlement/swap/chequebook"
	"golang.org/x/net/context"
)

type StorePriceRet struct {
	Price string `json:"price"`
}

type CashChequeRet struct {
	TxHash string
}

type cheque struct {
	PeerID       string
	Beneficiary  string
	Chequebook   string
	Payout       *big.Int
	CashedAmount *big.Int
}

type ListChequeRet struct {
	Cheques []cheque
	Len     int
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
		"cash":        CashChequeCmd,
		"cashstatus":  ChequeCashStatusCmd,
		"price":       StorePriceCmd,
		"prewithdraw": PreWithdrawCmd,
		//"info":      ChequeInfo,

		"send":                 SendChequeCmd,
		"sendlist":             ListSendChequesCmd,
		"receive":              ReceiveChequeCmd,
		"receivelist":          ListReceiveChequeCmd,
		"receive-history-peer": ChequeReceiveHistoryPeerCmd,
		"receive-history-list": ChequeReceiveHistoryListCmd,
	},
}

var StorePriceCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get btfs store price.",
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		price, err := chain.SettleObject.OracleService.GetPrice(context.Background())
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &StorePriceRet{
			Price: price.String(),
		})
	},
	Type: StorePriceRet{},
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

var PreWithdrawCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Pre withdraw chequebook balance",
	},

	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		tx_hash, err := chain.SettleObject.ChequebookService.PreWithdraw(req.Context)
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
