package cheque

import (
	"fmt"
	"io"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/chain"
)

var ChequeReceiveHistoryPeerCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Display the received cheques from peer.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("peer-id", true, false, "The peer id of cheques received."),
	},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {

		var listRet ChequeRecords
		peer_id := req.Arguments[0]
		fmt.Println("ChequeReceiveHistoryPeerCmd peer_id = ", peer_id)

		records, err := chain.SettleObject.SwapService.ReceivedChequeRecordsByPeer(peer_id)
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
