package commands

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	storage "github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/wallet"
	walletpb "github.com/TRON-US/go-btfs/protos/wallet"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs-cmds/http"
)

func init() {
	http.RegisterNonLocalCmds(
		"/wallet/init",
		"/wallet/deposit",
		"/wallet/withdraw",
		"/wallet/password",
		"/wallet/keys",
		"/wallet/import",
		"/wallet/transfer")
}

var WalletCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "BTFS wallet",
		ShortDescription: `'btfs wallet' is a set of commands to interact with block chain and ledger.`,
		LongDescription: `'btfs wallet' is a set of commands interact with block chain and ledger to deposit,
withdraw and query balance of token used in BTFS.`,
	},

	Subcommands: map[string]*cmds.Command{
		"init":         walletInitCmd,
		"deposit":      walletDepositCmd,
		"withdraw":     walletWithdrawCmd,
		"balance":      walletBalanceCmd,
		"password":     walletPasswordCmd,
		"keys":         walletKeysCmd,
		"transactions": walletTransactionsCmd,
		"import":       walletImportCmd,
		"transfer":     walletTransferCmd,
	},
}

var walletInitCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "Init BTFS wallet",
		ShortDescription: "Init BTFS wallet.",
	},

	Arguments: []cmds.Argument{},
	Options:   []cmds.Option{},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		cfg, err := n.Repo.Config()
		if err != nil {
			return err
		}
		wallet.Init(req.Context, cfg)
		return nil
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *MessageOutput) error {
			fmt.Fprint(w, out.Message)
			return nil
		}),
	},
	Type: MessageOutput{},
}

const asyncOptionName = "async"

var walletDepositCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "BTFS wallet deposit",
		ShortDescription: "BTFS wallet deposit from block chain to ledger.",
		Options:          "unit is µBTT (=0.000001BTT)",
	},

	Arguments: []cmds.Argument{
		cmds.StringArg("amount", true, false, "amount to deposit."),
	},
	Options: []cmds.Option{
		cmds.BoolOption(asyncOptionName, "a", "Deposit asynchronously."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		cfg, err := n.Repo.Config()
		if err != nil {
			return err
		}

		async, _ := req.Options[asyncOptionName].(bool)

		amount, err := strconv.ParseInt(req.Arguments[0], 10, 64)
		if err != nil {
			return err
		}

		runDaemon := false

		currentNode, err := cmdenv.GetNode(env)
		if err != nil {
			log.Error("Wrong while get current Node information", err)
			return err
		}
		runDaemon = currentNode.IsDaemon

		err = wallet.WalletDeposit(req.Context, cfg, n, amount, runDaemon, async)
		if err != nil {
			if strings.Contains(err.Error(), "Please deposit at least") {
				err = errors.New("Please deposit at least 10,000,000µBTT(=10BTT)")
			}
			return err
		}
		s := fmt.Sprintf("BTFS wallet deposit submitted. Please wait one minute for the transaction to confirm.")
		if !runDaemon {
			s = fmt.Sprintf("BTFS wallet deposit Done.")
		}
		return cmds.EmitOnce(res, &MessageOutput{s})
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *MessageOutput) error {
			fmt.Fprint(w, out.Message)
			return nil
		}),
	},
	Type: MessageOutput{},
}

var walletWithdrawCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "BTFS wallet withdraw",
		ShortDescription: "BTFS wallet withdraw from ledger to block chain.",
		Options:          "unit is µBTT (=0.000001BTT)",
	},

	Arguments: []cmds.Argument{
		cmds.StringArg("amount", true, false, "amount to deposit."),
	},
	Options: []cmds.Option{},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		cfg, err := n.Repo.Config()
		if err != nil {
			return err
		}

		amount, err := strconv.ParseInt(req.Arguments[0], 10, 64)
		if err != nil {
			return err
		}

		err = wallet.WalletWithdraw(req.Context, cfg, n, amount)
		if err != nil {
			if strings.Contains(err.Error(), "Please withdraw at least") {
				err = errors.New("Please withdraw at least 1,000,000,000µBTT(=1000BTT)")
			}
			return err
		}

		s := fmt.Sprintf("BTFS wallet withdraw submitted. Please wait one minute for the transaction to confirm.")
		return cmds.EmitOnce(res, &MessageOutput{s})
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *MessageOutput) error {
			fmt.Fprint(w, out.Message)
			return nil
		}),
	},
	Type: MessageOutput{},
}

var walletBalanceCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "BTFS wallet balance",
		ShortDescription: "Query BTFS wallet balance in ledger and block chain.",
		Options:          "unit is µBTT (=0.000001BTT)",
	},

	Arguments: []cmds.Argument{},
	Options:   []cmds.Option{},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		cfg, err := n.Repo.Config()
		if err != nil {
			return err
		}

		tronBalance, ledgerBalance, err := wallet.GetBalance(req.Context, cfg)
		if err != nil {
			log.Error("wallet get balance failed, ERR: ", err)
			return err
		}
		s := fmt.Sprintf("BTFS wallet tron balance '%d', ledger balance '%d'\n", tronBalance, ledgerBalance)
		log.Info(s)

		return cmds.EmitOnce(res, &BalanceResponse{
			BtfsWalletBalance: uint64(ledgerBalance),
			BttWalletBalance:  uint64(tronBalance),
		})
	},
	Type: BalanceResponse{},
}

type BalanceResponse struct {
	BtfsWalletBalance uint64
	BttWalletBalance  uint64
}

var walletPasswordCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "BTFS wallet password",
		ShortDescription: "set password for BTFS wallet",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("password", true, false, "password of BTFS wallet."),
	},
	Options: []cmds.Option{},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		cfg, err := n.Repo.Config()
		if err != nil {
			return err
		}
		if cfg.UI.Wallet.Initialized {
			return errors.New("Already init, cannot set password again.")
		}
		cipherMnemonic, err := wallet.EncryptWithAES(req.Arguments[0], cfg.Identity.Mnemonic)
		if err != nil {
			return err
		}
		cipherPrivKey, err := wallet.EncryptWithAES(req.Arguments[0], cfg.Identity.PrivKey)
		if err != nil {
			return err
		}
		cfg.Identity.EncryptedMnemonic = cipherMnemonic
		cfg.Identity.EncryptedPrivKey = cipherPrivKey
		err = n.Repo.SetConfig(cfg)
		if err != nil {
			return err
		}
		return cmds.EmitOnce(res, &MessageOutput{"Password set."})
	},
	Type: MessageOutput{},
}

var walletKeysCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "BTFS wallet keys",
		ShortDescription: "get keys of BTFS wallet",
	},
	Arguments: []cmds.Argument{},
	Options:   []cmds.Option{},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		cfg, err := n.Repo.Config()
		if err != nil {
			return err
		}
		var keys *Keys
		if !cfg.UI.Wallet.Initialized {
			keys = &Keys{
				PrivateKey: cfg.Identity.PrivKey,
				Mnemonic:   cfg.Identity.Mnemonic,
			}
		} else {
			keys = &Keys{
				PrivateKey: cfg.Identity.EncryptedPrivKey,
				Mnemonic:   cfg.Identity.EncryptedMnemonic,
			}
		}
		return cmds.EmitOnce(res, keys)
	},
	Type: Keys{},
}

type Keys struct {
	PrivateKey string
	Mnemonic   string
}

var walletTransactionsCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "BTFS wallet transactions",
		ShortDescription: "get transactions of BTFS wallet",
	},
	Arguments: []cmds.Argument{},
	Options:   []cmds.Option{},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		txs, err := wallet.GetTransactions(n.Repo.Datastore(), n.Identity.Pretty())
		if err != nil {
			return err
		}
		return cmds.EmitOnce(res, txs)
	},
	Type: []*walletpb.TransactionV1{},
}

var walletTransferCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "Send to another BTT wallet",
		ShortDescription: "Send to another BTT wallet from current BTT wallet",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("to", true, false, "address of another BTFS wallet to transfer to."),
		cmds.StringArg("amount", true, false, "amount of µBTT (=0.000001BTT) to transfer."),
	},
	Options: []cmds.Option{},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		cfg, err := n.Repo.Config()
		if err != nil {
			return err
		}
		amount, err := strconv.ParseInt(req.Arguments[1], 10, 64)
		if err != nil {
			return err
		}
		ret, err := wallet.TransferBTT(req.Context, n, cfg, nil, "", req.Arguments[0], amount)
		if err != nil {
			return err
		}
		return cmds.EmitOnce(res, &TransferResult{
			Result:  ret.Result,
			Message: ret.Message,
		})
	},
	Type: &TransferResult{},
}

type TransferResult struct {
	Result  bool
	Message string
}

const privateKeyOptionName = "privateKey"
const mnemonicOptionName = "mnemonic"

var walletImportCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "BTFS wallet import",
		ShortDescription: "import BTFS wallet",
	},
	Arguments: []cmds.Argument{},
	Options: []cmds.Option{
		cmds.StringOption(privateKeyOptionName, "p", "Private Key to import."),
		cmds.StringOption(mnemonicOptionName, "m", "Mnemonic to import."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		privKey, _ := req.Options[privateKeyOptionName].(string)
		mnemonic, _ := req.Options[mnemonicOptionName].(string)
		err = wallet.ImportKeys(n, privKey, mnemonic)
		if err != nil {
			return err
		}
		go func() error {
			restartCmd := exec.Command(storage.Excutable, "restart")
			if err := restartCmd.Run(); err != nil {
				log.Errorf("restart error, %v", err)
				return err
			}
			return nil
		}()
		return nil
	},
}
