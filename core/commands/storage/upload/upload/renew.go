package upload

import (
	"context"
	"errors"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"strconv"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/TRON-US/go-btfs-cmds"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	hubpb "github.com/tron-us/go-btfs-common/protos/hub"
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
	Subcommands: map[string]*cmds.Command{
		"inquiry":   StorageRenewInquiryCmd,
		"renewinit": StorageRenewInitCmd,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, false, "Hash of file to renew."),
		cmds.StringArg("renew-length", true, false, "New File storage period on hosts in days."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		cfg, err := n.Repo.Config()
		if err != nil {
			return err
		}
		fileHash := req.Arguments[0]
		renterId := n.Identity.String()
		metaReq := &guardpb.CheckFileStoreMetaRequest{
			FileHash:     fileHash,
			RenterPid:    renterId,
			RequesterPid: renterId,
			RequestTime:  time.Now().UTC(),
		}
		sig, err := crypto.Sign(n.PrivateKey, metaReq)
		if err != nil {
			return err
		}
		metaReq.Signature = sig
		var meta *guardpb.FileStoreStatus
		err = grpc.GuardClient(cfg.Services.GuardDomain).WithContext(req.Context, func(ctx context.Context,
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
		ctxParams, err := uh.ExtractContextParams(req, env)
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
		var price int64
		err = grpc.HubQueryClient(cfg.Services.HubDomain).WithContext(req.Context,
			func(ctx context.Context, client hubpb.HubQueryServiceClient) error {
				req := new(hubpb.SettingsReq)
				//unknow host id
				req.Id = ""
				resp, err := client.GetSettings(ctx, req)
				if err != nil {
					return err
				}
				if resp.Code != hubpb.ResponseCode_SUCCESS {
					return errors.New(resp.Message)
				}
				price = int64(resp.SettingsData.StoragePriceAsk)
				return nil
			})
		if err != nil {
			return err
		}
		newRentStart := m.RentEnd.Add(time.Duration(24) * time.Hour)
		UploadShard(rss, nil, price, m.ShardFileSize, int(renewPeriod), newRentStart, false, true, renterPid, -1,
			shardIndexes, nil)
		seRes := &Res{
			ID: ssId,
		}
		return res.Emit(seRes)
	},
	Type: Res{},
}
