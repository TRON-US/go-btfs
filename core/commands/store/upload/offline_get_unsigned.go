package upload

import (
	"errors"

	cmds "github.com/TRON-US/go-btfs-cmds"

	cmap "github.com/orcaman/concurrent-map"
)

var status = map[string]cmap.ConcurrentMap{}

var storageUploadGetUnsignedCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get the input data for upload signing.",
		ShortDescription: `
This command obtains the upload signing input data for from the upload session 
(From BTFS SDK application's perspective) and returns to the caller.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("peer-id", true, false, "Offline signs needed for this particular client."),
		cmds.StringArg("nonce-timestamp", true, false, "Nonce timestamp string for this upload signing."),
		cmds.StringArg("upload-session-signature", true, false, "Private key-signed string of peer-id:nonce-timestamp"),
		cmds.StringArg("session-status", true, false, "current upload session status."),
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
		rssStatus, err := rss.status()
		if err != nil {
			return err
		}
		if req.Arguments[4] != rssStatus.Status {
			return errors.New("unexpected session status from SDK during communication in upload signing")
		}
		signing, err := rss.offlineSigning()
		if err != nil {
			return err
		}
		status, err := rss.status()
		if err != nil {
			return err
		}
		rawString, err := bytesToString(signing.Raw, Base64)
		if err != nil {
			return err
		}
		return res.Emit(&getUnsignedRes{Unsigned: rawString, CurrentStatus: status.Status})
	},
	Type: getUnsignedRes{},
}

type getUnsignedRes struct {
	Unsigned      string
	CurrentStatus string
}
