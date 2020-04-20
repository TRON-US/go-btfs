package upload

import (
	"context"
	"errors"
	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"strings"
	"time"
)

var storageUploadRepairCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Repair specific shards of a file.",
		ShortDescription: `
This command repairs the given shards of a file.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, false, "Hash of file to upload."),
		cmds.StringArg("repair-shards", true, false, "Shard hashes to repair."),
		cmds.StringArg("renter-pid", true, false, "Original renter peer ID."),
		cmds.StringArg("blacklist", true, false, "Blacklist of hosts during upload."),
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ctxParams, err := ExtractContextParams(req, env)
		if err != nil {
			return err
		}
		fileHash := req.Arguments[0]
		metaReq := &guardpb.CheckFileStoreMetaRequest{
			FileHash:     fileHash,
			RenterPid:    ctxParams.n.Identity.String(),
			RequesterPid: ctxParams.n.Identity.String(),
			RequestTime:  time.Now().UTC(),
		}
		sig, err := crypto.Sign(ctxParams.n.PrivateKey, metaReq)
		if err != nil {
			return err
		}
		metaReq.Signature = sig
		ctx, _ := helper.NewGoContext(req.Context)
		var meta *guardpb.FileStoreStatus
		err = grpc.GuardClient(ctxParams.cfg.Services.GuardDomain).WithContext(ctx, func(ctx context.Context,
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
		ssId, _ := splitContractId(contracts[0].ContractId)
		shardIndexes := make([]int, 0)
		i := 0
		shardHashes := strings.Split(req.Arguments[1], ",")
		for _, contract := range contracts {
			if contract.ShardHash == shardHashes[i] {
				shardIndexes = append(shardIndexes, int(contract.ShardIndex))
				i++
			}
		}
		rss, err := GetRenterSession(ctxParams, ssId, fileHash, shardHashes)
		if err != nil {
			return err
		}
		hp := getHostsProvider(ctxParams, strings.Split(req.Arguments[3], ","))
		m := contracts[0].ContractMeta
		renterPid, err := peer.IDB58Decode(req.Arguments[2])
		if err != nil {
			return err
		}
		rss.uploadShard(hp, m.Price, m.ShardFileSize, -1, false, renterPid, -1,
			shardIndexes, &RepairParams{
				RenterStart: m.RentStart,
				RenterEnd:   m.RentEnd,
			})
		seRes := &Res{
			ID: ssId,
		}
		return res.Emit(seRes)
	},
	Type: Res{},
}
