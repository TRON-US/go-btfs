package upload

import (
	"fmt"
	cmds "github.com/TRON-US/go-btfs-cmds"
)

var storageUploadGetContractBatchCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get all the contracts from the upload session	(From BTFS SDK application's perspective).",
		ShortDescription: `
This command (on client) reads the unsigned contracts and returns 
the contracts to the caller.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("peer-id", true, false, "Offline signs needed for this particular client."),
		cmds.StringArg("nonce-timestamp", true, false, "Nonce timestamp string for this offline signing."),
		cmds.StringArg("upload-session-signature", true, false, "Private key-signed string of <peer-id>:<nonce-timstamp>"),
		cmds.StringArg("session-status", true, false, "Current upload session status."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ssId := req.Arguments[0]
		fmt.Println("ssID", ssId)
		return nil
	},
	Type: GetContractBatchRes{},
}

type GetContractBatchRes struct {
	Contracts []*Contract
}

type Contract struct {
	Key          string `json:"key"`
	ContractData string `json:"contract"`
}
