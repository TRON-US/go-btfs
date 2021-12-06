package cheque

import (
	"fmt"
	"io"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
)

type ReceiveTotalCountRet struct {
	Count int `json:"count"`
}

var ReceiveChequesCountCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "send cheque(s) count",
	},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		count, err := chain.SettleObject.ChequebookService.TotalIssuedCount()

		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &ReceiveTotalCountRet{Count: count})
	},
	Type: ReceiveTotalCountRet{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, c *ReceiveTotalCountRet) error {
			fmt.Println("receive cheque(s) count: ", c.Count)

			return nil
		}),
	},
}
