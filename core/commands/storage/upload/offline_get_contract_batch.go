package upload

import (
	"errors"
	"fmt"

	cmds "github.com/TRON-US/go-btfs-cmds"

	cmap "github.com/orcaman/concurrent-map"
)

const (
	contractsTypeGuard  = "guard"
	contractsTypeEscrow = "escrow"
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
		cmds.StringArg("nonce-timestamp", true, false, "Nonce timestamp string for this upload signing."),
		cmds.StringArg("upload-session-signature", true, false, "Private key-signed string of <peer-id>:<nonce-timstamp>"),
		cmds.StringArg("contracts-type", true, false, "get guard or escrow contracts"),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ssId := req.Arguments[0]
		ctxParams, err := ExtractContextParams(req, env)
		if err != nil {
			return err
		}
		if !ctxParams.cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}
		rss, err := GetRenterSession(ctxParams, ssId, "", make([]string, 0))
		if err != nil {
			return err
		}
		status, err := rss.status()
		if err != nil {
			return err
		}
		err = verifyReceivedMessage(req, rss)
		if err != nil {
			return err
		}
		contracts := make([]*contract, 0)
		var cm cmap.ConcurrentMap
		if cm = escrowContractMaps; req.Arguments[4] == contractsTypeGuard {
			cm = guardContractMaps
		}
		for i, h := range status.ShardHashes {
			shardId := getShardId(ssId, h, i)
			c := &contract{
				Key: shardId,
			}
			if bytes, ok := cm.Get(shardId); ok {
				c.ContractData, err = bytesToString(bytes.([]byte), Base64)
			} else {
				return errors.New("some contracts haven't ready yet")
			}
			contracts = append(contracts, c)
		}
		return res.Emit(&getContractBatchRes{
			Contracts: contracts,
		})
	},
	Type: getContractBatchRes{},
}

type getContractBatchRes struct {
	Contracts []*contract
}

type contract struct {
	Key          string `json:"key"`
	ContractData string `json:"contract"`
}
