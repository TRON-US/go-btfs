package commands

import (
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/wallet"
	walletpb "github.com/TRON-US/go-btfs/protos/wallet"

	cmds "github.com/TRON-US/go-btfs-cmds"
)

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
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			fmt.Println("get config failed")
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
	},

	Arguments: []cmds.Argument{
		cmds.StringArg("amount", true, false, "amount to deposit."),
	},
	Options: []cmds.Option{
		cmds.BoolOption(asyncOptionName, "a", "Deposit asynchronously."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}

		amount, err := strconv.ParseInt(req.Arguments[0], 10, 64)
		if err != nil {
			return err
		}

		async, _ := req.Options[asyncOptionName].(bool)

		runDaemon := false
		currentNode, err := cmdenv.GetNode(env)
		if err != nil {
			log.Error("Wrong while get current Node information", err)
			return err
		}
		runDaemon = currentNode.IsDaemon

		// get node
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		err = wallet.WalletDeposit(req.Context, cfg, n, amount, runDaemon, async)
		if err != nil {
			log.Error("wallet deposit failed, ERR: ", err)
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
	},

	Arguments: []cmds.Argument{
		cmds.StringArg("amount", true, false, "amount to deposit."),
	},
	Options: []cmds.Option{},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}

		amount, err := strconv.ParseInt(req.Arguments[0], 10, 64)
		if err != nil {
			return err
		}

		// get node
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		err = wallet.WalletWithdraw(req.Context, cfg, n, amount)
		if err != nil {
			log.Error("wallet withdraw failed, ERR: ", err)
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
	},

	Arguments: []cmds.Argument{},
	Options:   []cmds.Option{},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}

		tronBalance, lederBalance, err := wallet.GetBalance(req.Context, cfg)
		if err != nil {
			log.Error("wallet get balance failed, ERR: ", err)
			return err
		}
		s := fmt.Sprintf("BTFS wallet tron balance '%d', ledger balance '%d'\n", tronBalance, lederBalance)
		log.Info(s)
		return cmds.EmitOnce(res, &BalanceResponse{
			BtfsWalletBalance: tronBalance,
			BttWalletBalance:  lederBalance,
		})
	},
	Type: BalanceResponse{},
}

type BalanceResponse struct {
	BtfsWalletBalance int64
	BttWalletBalance  int64
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
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if cfg.UI.Wallet.Initialized {
			return errors.New("Already init, cannot set pasword again.")
		}
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		cfg.Identity.Password = req.Arguments[0]
		cfg.UI.Wallet.Initialized = true
		err = n.Repo.SetConfig(cfg)
		if err != nil {
			return err
		}
		return cmds.EmitOnce(res, &MessageOutput{"Password set."})
	},
	Type: MessageOutput{},
}

const passwordOptionName = "password"

var walletKeysCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "BTFS wallet keys",
		ShortDescription: "get keys of BTFS wallet",
	},
	Arguments: []cmds.Argument{},
	Options: []cmds.Option{
		cmds.StringOption(passwordOptionName, "p", "password of BTFS wallet"),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.UI.Wallet.Initialized {
			return errors.New("Wallet not init yet.")
		}
		password, _ := req.Options[passwordOptionName].(string)
		if password != cfg.Identity.Password {
			return errors.New("Wrong password. Please try again.")
		}
		return cmds.EmitOnce(res, &Keys{
			PrivateKey: cfg.Identity.PrivKey,
			Mnemonic:   cfg.Identity.Mnemonic,
		})
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
	Type: []*walletpb.Transaction{},
}
