package chequebook

import (
	cmds "github.com/TRON-US/go-btfs-cmds"
)

var ChequeBookCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Interact with chequebook services on BTFS.",
		ShortDescription: `
Chequebook services include balance, address, withdraw, deposit operations.`,
	},
	Subcommands: map[string]*cmds.Command{
		"balance":  ChequeBookBalanceCmd,
		"address":  ChequeBookAddrCmd,
		"withdraw": ChequeBookWithdrawCmd,
		"deposit":  ChequeBookDepositCmd,
	},
}
