package upload

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
)

var RenewCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Renew the contracts of specific file with additional period.",
		ShortDescription: `
The functionality facilitates users to renew the contract for next storage period 
without uploading the file again when the contract was expired, the extended duration
and file hash need to be specified and passed on the command.
`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, false, "Hash of file to renew."),
		cmds.StringArg("renew-length", true, false, "New File storage period on hosts in days."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ctxParams, err := uh.ExtractContextParams(req, env)
		if err != nil {
			return err
		}
		fileHash := req.Arguments[0]
		renterId := ctxParams.N.Identity.String()
		metaReq := &guardpb.CheckFileStoreMetaRequest{
			FileHash:     fileHash,
			RenterPid:    renterId,
			RequesterPid: renterId,
			RequestTime:  time.Now().UTC(),
		}
		sig, err := crypto.Sign(ctxParams.N.PrivateKey, metaReq)
		if err != nil {
			return err
		}
		metaReq.Signature = sig
		ctx, _ := helper.NewGoContext(req.Context)
		var meta *guardpb.FileStoreStatus
		err = grpc.GuardClient(ctxParams.Cfg.Services.GuardDomain).WithContext(ctx, func(ctx context.Context,
			client guardpb.GuardServiceClient) error {
			meta, err = client.CheckFileStoreMeta(ctx, metaReq)
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
		contracts := meta.Contracts
		if len(contracts) <= 0 {
			return errors.New("length of contracts is 0")
		}
		ssId, _ := uh.SplitContractId(contracts[0].ContractId)

		shardIndexes := make([]int, 0)
		shardHashes := make([]string, 0)

		for _, contract := range contracts {
			if contract.State == guardpb.Contract_UPLOADED {
				shardHashes = append(shardHashes, contract.ShardHash)
				shardIndexes = append(shardIndexes, int(contract.ShardIndex))
			}
		}

		renewPeriod, err := strconv.ParseInt(req.Arguments[1], 10, 0)
		if err != nil {
			return err
		}

		rss, err := sessions.GetRenterSession(ctxParams, ssId, fileHash, shardHashes)
		if err != nil {
			return err
		}

		m := contracts[0].ContractMeta
		renterPid, err := peer.IDB58Decode(renterId)
		if err != nil {
			return err
		}

		// Build new contracts here
		CreateNewContracts(rss, m.Price, m.ShardFileSize, m.RentEnd, int(renewPeriod), false, renterPid, -1, shardIndexes, nil)

		seRes := &Res{
			ID: ssId,
		}
		return res.Emit(seRes)
	},
	Type: Res{},
}

// create new contracts and send to Guard, since Proto changed will send another PR for this
func CreateNewContracts(rss *sessions.RenterSession, price int64, shardSize int64, preRentEnd time.Time,
	storageLength int, offlineSigning bool, renterId peer.ID, fileSize int64, shardIndexes []int, rp *RepairParams) {
}

