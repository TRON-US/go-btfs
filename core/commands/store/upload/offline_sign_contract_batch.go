package upload

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/TRON-US/go-btfs/core/commands/storage"

	cmds "github.com/TRON-US/go-btfs-cmds"

	cmap "github.com/orcaman/concurrent-map"
)

var storageUploadSignContractBatchCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get the unsigned contracts from the upload session.",
		ShortDescription: `
This command reads all the unsigned contracts from the upload session 
(From BTFS SDK application's perspective) and returns the contracts to the caller.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("peer-id", true, false, "Offline signs needed for this particular client."),
		cmds.StringArg("nonce-timestamp", true, false, "Nonce timestamp string for this upload signing."),
		cmds.StringArg("upload-session-signature", true, false, "Private key-signed string of peer-id:nonce-timestamp"),
		cmds.StringArg("contracts-type", true, false, "get guard or escrow contracts"),
		cmds.StringArg("signed-data-items", true, false, "signed data items."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ssID := req.Arguments[0]
		ctxParams, err := extractContextParams(req, env)
		if err != nil {
			return err
		}
		rss, err := GetRenterSession(ctxParams, ssID, "", make([]string, 0))
		if err != nil {
			return err
		}
		err = verifyReceivedMessage(req, rss)
		if err != nil {
			return err
		}

		var signedContracts []storage.Contract
		signedContractsString := req.Arguments[5]
		err = json.Unmarshal([]byte(signedContractsString), &signedContracts)
		if err != nil {
			return err
		}
		if len(signedContracts) != len(rss.shardHashes) {
			return fmt.Errorf("number of received signed data items %d does not match number of shards %d",
				len(signedContracts), len(rss.shardHashes))
		}
		var cm cmap.ConcurrentMap
		if cm = escrowChanMaps; req.Arguments[4] == contractsTypeGuard {
			cm = guardChanMaps
		}
		for i := 0; i < len(rss.shardHashes); i++ {
			shardId := signedContracts[i].Key
			ch, found := cm.Get(shardId)
			if !found {
				return fmt.Errorf("can not find an entry for key %s", shardId)
			}
			by, err := stringToBytes(signedContracts[i].ContractData, Base64)
			if err != nil {
				return err
			}
			ch.(chan []byte) <- by
		}
		return nil
	},
}

func verifyReceivedMessage(req *cmds.Request, rss *RenterSession) error {
	meta, err := rss.offlineMeta()
	if err != nil {
		return err
	}
	if meta.OfflinePeerId != req.Arguments[1] {
		return errors.New("peerIDs do not match")
	}
	offlineNonceTimestamp, err := strconv.ParseUint(req.Arguments[2], 10, 64)
	if err != nil {
		return err
	}
	if meta.OfflineNonceTs != offlineNonceTimestamp {
		return errors.New("Nonce timestamps do not match")
	}
	if meta.OfflineSignature != req.Arguments[3] {
		return errors.New("Session signature do not match")
	}
	return nil
}
