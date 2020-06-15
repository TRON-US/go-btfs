package upload

import (
	"context"
	"errors"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v3"
	"github.com/gogo/protobuf/proto"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	"github.com/tron-us/go-btfs-common/config"
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
		ctx, _ := helper.NewGoContext(req.Context)
		var meta *guardpb.FileStoreStatus
		err = grpc.GuardClient(cfg.Services.GuardDomain).WithContext(ctx, func(ctx context.Context,
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
		err = grpc.HubQueryClient(cfg.Services.HubDomain).WithContext(ctx,
			func(ctx context.Context, client hubpb.HubQueryServiceClient) error {
				req := new(hubpb.SettingsReq)
				req.Id = renterId
				resp, err := client.GetSettings(ctx, req)
				if err != nil {
					return err
				}
				price = int64(resp.SettingsData.StoragePriceAsk)
				return nil
			})
		if err != nil {
			return err
		}
		//Build new contracts here
		CreateNewContracts(rss, price, m.ShardFileSize, m.RentEnd, int(renewPeriod), false, renterPid, -1,
			shardIndexes, nil)

		seRes := &Res{
			ID: ssId,
		}
		return res.Emit(seRes)
	},
	Type: Res{},
}

func CreateNewContracts(rss *sessions.RenterSession, price int64, shardSize int64, preRentEnd time.Time,
	storageLength int, offlineSigning bool, renterId peer.ID, fileSize int64, shardIndexes []int, rp *RepairParams) {
	for index, shardHash := range rss.ShardHashes {
		go func(i int, h string) {
			err := backoff.Retry(func() error {
				select {
				case <-rss.Ctx.Done():
					return nil
				default:
					break
				}
				contractId := uh.NewContractID(rss.SsId)
				tp := uh.TotalPay(shardSize, price, storageLength)
				percent := config.GetRenewContingencyPercentage()
				ca := int64(float64(tp) * float64(percent) / 100)
				tp += ca
				var renewEscrowContractBytes []byte
				errChan := make(chan error, 2)
				go func() {
					tmp := func() error {
						escrowContractBytes, err := renterSignEscrowContract(rss, h, i, "", tp, ca, offlineSigning, true,
							renterId, contractId)
						if err != nil {
							log.Errorf("shard %s signs escrow_contract error: %s", h, err.Error())
							return err
						}
						renewEscrowContractBytes = escrowContractBytes
						return nil
					}()
					errChan <- tmp
				}()

				var renewGuardContractBytes []byte
				go func() {
					tmp := func() error {
						guardContractBytes, err := RenterSignGuardContract(rss, &ContractParams{
							ContractId:       contractId,
							RenterPid:        renterId.Pretty(),
							HostPid:          "",
							ShardIndex:       int32(i),
							ShardHash:        h,
							ShardSize:        shardSize,
							FileHash:         rss.Hash,
							StartTime:        preRentEnd.Add(time.Duration(24) * time.Hour),
							StorageLength:    int64(storageLength),
							Price:            price,
							TotalPay:         tp,
							ContingentAmount: ca,
						}, offlineSigning, true, rp)
						if err != nil {
							log.Errorf("shard %s signs guard_contract error: %s", h, err.Error())
							return err
						}
						renewGuardContractBytes = guardContractBytes
						return nil
					}()
					errChan <- tmp
				}()
				c := 0
				for err := range errChan {
					c++
					if err != nil {
						return err
					}
					if c == 2 {
						break
					}
				}
				guardContract := new(guardpb.Contract)
				err := proto.Unmarshal(renewGuardContractBytes, guardContract)
				if err != nil {
					return err
				}
				shard, err := sessions.GetRenterShard(rss.CtxParams, rss.SsId, shardHash, index)
				if err != nil {
					return err
				}
				_ = shard.Contract(renewEscrowContractBytes, guardContract)
				return nil
			}, uh.HandleShardBo)
			if err != nil {
				_ = rss.To(sessions.RssToErrorEvent, err)
			}
		}(shardIndexes[index], shardHash)
	}
	// waiting for contracts of 30(n) shards and submit to escrow and pay
	go func(rss *sessions.RenterSession, numShards int) {
		tick := time.Tick(5 * time.Second)
		for true {
			select {
			case <-tick:
				completeNum, errorNum, err := rss.GetCompleteShardsNum()
				if err != nil {
					continue
				}
				log.Info("session", rss.SsId, "contractNum", completeNum, "errorNum", errorNum)
				if completeNum == numShards {
					err := Submit(rss, fileSize, offlineSigning, true)
					if err != nil {
						_ = rss.To(sessions.RssToErrorEvent, err)
					}
					return
				} else if errorNum > 0 {
					_ = rss.To(sessions.RssToErrorEvent, errors.New("there are some error shards"))
					log.Error("session:", rss.SsId, ",errorNum:", errorNum)
					return
				}
			case <-rss.Ctx.Done():
				log.Infof("session %s done", rss.SsId)
				return
			}
		}
	}(rss, len(rss.ShardHashes))
}
