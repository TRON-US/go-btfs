package upload

import (
	"context"
	"fmt"
	"math/big"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/settlement/swap/swapprotocol"
)

var StorageUploadChequeCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "receive upload cheque, do with cheque, and return it.",
		ShortDescription: `receive upload cheque, deal it and return it.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("encoded-cheque", true, false, "encoded-cheque from peer-id."),
		cmds.StringArg("amount", true, false, "amount"),
		cmds.StringArg("contract-id", false, false, "contract-id."),
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ctxParams, err := uh.ExtractContextParams(req, env)
		if err != nil {
			return err
		}
		if !ctxParams.Cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}

		requestPid, ok := remote.GetStreamRequestRemotePeerID(req, ctxParams.N)
		if !ok {
			return fmt.Errorf("fail to get peer ID from request")
		}

		exchangeRate, ok := new(big.Int).SetString(req.Arguments[1], 10)
		if !ok {
			return fmt.Errorf("exchangeRate:%s cannot be parsed, err:%s", req.Arguments[2], err)
		}

		// decode and deal the cheque
		encodedCheque := req.Arguments[0]
		err = swapprotocol.SwapProtocol.Handler(context.Background(), requestPid.String(), encodedCheque, exchangeRate)
		if err != nil {
			return err
		}

		fmt.Printf("****** receive upload cheque, contractId =  %+v len=%v \n", req.Arguments[2], len(req.Arguments))

		// if receive cheque of contractId, set shard paid status.
		if contractId := req.Arguments[2]; len(contractId) > 0 {
			err := setPaidStatus(ctxParams, contractId)
			if err != nil{
				return err
			}
		}

		return nil
	},
}
