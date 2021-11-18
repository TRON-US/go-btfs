package cheque

import (
	"fmt"
	"io"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
	oldcmds "github.com/TRON-US/go-btfs/commands"
	"github.com/tron-us/go-btfs-common/crypto"
)

type ChainInfoRet struct {
	ChainId            int64  `json:"chain_id"`
	NodeAddr           string `json:"node_addr"`
	ChequeBookAddr     string `json:"cheque_book_addr"`
	ReceiverAddr       string `json:"receiver_addr"`
	WalletImportPrvKey string `json:"wallet_import_prv_key"`
}

var ChequeChainInfoCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Show current chain info.",
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		receiver, err := chain.SettleObject.ChequebookService.Receiver()
		if err != nil {
			return err
		}
		cctx := env.(*oldcmds.Context)
		cfg, err := cctx.GetConfig()
		if err != nil {
			return err
		}
		privKey, err := crypto.ToPrivKey(cfg.Identity.PrivKey)
		if err != nil {
			return err
		}
		keys, err := crypto.FromIcPrivateKey(privKey)
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &ChainInfoRet{
			ChainId:            chain.ChainObject.ChainID,
			NodeAddr:           chain.ChainObject.OverlayAddress.String(),
			ChequeBookAddr:     chain.SettleObject.ChequebookService.Address().String(),
			ReceiverAddr:       receiver.String(),
			WalletImportPrvKey: keys.HexPrivateKey,
		})
	},
	Type: &ChainInfoRet{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *ChainInfoRet) error {
			_, err := fmt.Fprintf(w, "chain id:\t%d\nnode addr:\t%s\nchequebook addr:\t%s\nreceiver addr:\t%s\nwallet import private key:\t%s\n", out.ChainId,
				out.NodeAddr, out.ChequeBookAddr, out.ReceiverAddr, out.WalletImportPrvKey)
			return err
		}),
	},
}
