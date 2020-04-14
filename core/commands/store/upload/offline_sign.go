package upload

import (
	cmds "github.com/TRON-US/go-btfs-cmds"
	renterpb "github.com/TRON-US/go-btfs/protos/renter"
)

var storageUploadSignCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Return the signed data to the upload session.",
		ShortDescription: `
This command returns the signed data (From BTFS SDK application's perspective)
to the upload session.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("peer-id", true, false, "Offline signs needed for this particular client."),
		cmds.StringArg("nonce-timestamp", true, false, "Nonce timestamp string for this upload signing."),
		cmds.StringArg("upload-session-signature", true, false, "Private key-signed string of peer-id:nonce-timestamp"),
		cmds.StringArg("session-status", true, false, "current upload session status."),
		cmds.StringArg("signed", true, false, "signed json data."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ssId := req.Arguments[0]
		ctxParams, err := extractContextParams(req, env)
		if err != nil {
			return err
		}
		rss, err := GetRenterSession(ctxParams, ssId, "", make([]string, 0))
		if err != nil {
			return err
		}
		err = verifyReceivedMessage(req, rss)
		if err != nil {
			return err
		}
		bytes, err := stringToBytes(req.Arguments[5], Base64)
		if err != nil {
			return err
		}
		return rss.saveOfflineSigning(&renterpb.OfflineSigning{
			Sig: bytes,
		})
	},
}
