package renew

import (
	"context"
	"errors"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/core/hub"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"github.com/tron-us/protobuf/proto"

	"github.com/cenkalti/backoff/v4"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
)

var log = logging.Logger("renew")

var StorageRenewInquiryCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "inquire hosts for the new duration and return the hosts signed contracts.",
		ShortDescription: `
Communicate with hosts to accept or reject the new contracts, return the signed the contracts if agree with the extended period, 
or find new hosts to download and store the shards.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("renter-pid", true, false, "Original renter peer ID."),
		cmds.StringArg("file-hash", true, false, "Hash of file to upload."),
		cmds.StringArg("entry-key", true, false, "Key for querying meta data from Guard."),
	},
	RunTimeout: 5 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ctxParams, err := uh.ExtractContextParams(req, env)
		if err != nil {
			return err
		}
		cfg := ctxParams.Cfg
		renterPid := req.Arguments[0]
		fileHash := req.Arguments[1]
		entryKey := req.Arguments[2]
		metaReq := &guardpb.CheckFileStoreMetaRequest{
			FileHash:     fileHash,
			RenterPid:    renterPid,
			RequesterPid: entryKey,
			RequestTime:  time.Now().UTC(),
		}
		metaReq.Signature = []byte(entryKey)
		var meta *guardpb.FileStoreStatus
		err = grpc.GuardClient(cfg.Services.GuardDomain).WithContext(req.Context, func(ctx context.Context,
			client guardpb.GuardServiceClient) error {
			meta, err = client.CheckFileStoreMeta(ctx, metaReq)
			if err != nil {
				return err
			}
			return nil
		})

		var bidTotalPrice int64
		var askTotalPrice int64
		var askDailyPrice int64
		contracts := meta.Contracts
		hostPidMap := make(map[string]string)
		askPriceMap := make(map[string]int64)
		contractList := make([]*guardpb.Contract, 0)
		storageLength := (int)(contracts[0].RentEnd.Sub(contracts[0].RentStart).Hours() / 24)
		for _, contract := range contracts {
			if contract.State == guardpb.Contract_UPLOADED {
				hostPidMap[contract.ContractId] = contract.HostPid
			}
		}
		for _, contract := range contracts {
			if contract.State == guardpb.Contract_RECREATED {
				hostPid := hostPidMap[contract.ShardHash]
				ns, err := hub.GetHostSettings(req.Context, cfg.Services.HubDomain, hostPid)
				if err != nil {
					return err
				}
				askPrice := int64(ns.StoragePriceAsk)
				askPriceMap[contract.ContractId] = askPrice
				askDailyPrice += uh.TotalPay(contract.ShardFileSize, askPrice, 1)
				askTotalPrice += uh.TotalPay(contract.ShardFileSize, askPrice, int(storageLength))
				bidPrice := contract.Amount
				bidTotalPrice += bidPrice
				contractList = append(contractList, contract)
			}
		}

		var signedGuardContracts []*guardpb.Contract
		if askTotalPrice <= bidTotalPrice {
			signedGuardContracts, err = renewHostProcess(ctxParams, renterPid, fileHash, storageLength, hostPidMap, askPriceMap, contractList, "agree", nil)
		} else {
			storageLength = (int)(bidTotalPrice / askDailyPrice)
			signedGuardContracts, err = renewHostProcess(ctxParams, renterPid, fileHash, storageLength, hostPidMap, askPriceMap, contractList, "agree", nil)
		}
		if err != nil {
			return err
		}
		updateContract(meta.Contracts, signedGuardContracts)
		return cmds.EmitOnce(res, meta)
	},
	Type: guardpb.FileStoreStatus{},
}

func updateContract(guardContracts []*guardpb.Contract, signedGuardContracts []*guardpb.Contract) {
	cmap := map[string]*guardpb.Contract{}
	for _, signedContract := range signedGuardContracts {
		cmap[signedContract.ContractId] = signedContract
	}
	for _, contract := range guardContracts {
		if contract.State == guardpb.Contract_RECREATED {
			if v, ok := cmap[contract.ContractId]; ok {
				contract = v
			}
		}
	}
}

func renewHostProcess(ctxParams *uh.ContextParams, renterPid string, fileHash string, storageLength int, hostPidMap map[string]string, askPriceMap map[string]int64, guardContracts []*guardpb.Contract, agreeReject string, hp uh.IHostsProvider) ([]*guardpb.Contract, error) {
	cb := make(chan error)
	inquiryResult := make(chan *guardpb.Contract)
	rejectedHosts := make([]*guardpb.Contract, 0)
	signedContracts := make([]*guardpb.Contract, 0)
	ssId, _ := uh.SplitContractId(guardContracts[0].ContractId)

	for _, contract := range guardContracts {
		signedGuardContract := &guardpb.Contract{}
		go func(hostId string, price int64, shardIndex int32, shardHash string, shardSize int64) {
			err := backoff.Retry(func() error {
				if agreeReject == "reject" {
					price = contract.Price
					if hostPid, err := hp.NextValidHost(contract.Price); err != nil {
						log.Debugf("original err: %s", err.Error())
						return nil
					} else {
						hostId = hostPid
					}
				}
				hostPid, err := peer.IDB58Decode(hostId)
				if err != nil {
					log.Errorf("shard %s decodes host_pid error: %s", shardHash, err.Error())
					return err
				}

				go func() {
					ctx, _ := context.WithTimeout(ctxParams.Ctx, 10*time.Second)
					signedGuardContractBytes, err := remote.P2PCall(ctx, ctxParams.N, hostPid, "/storage/upload/init",
						ssId,
						fileHash,
						shardHash,
						price,
						"",
						contract,
						storageLength,
						shardSize,
						shardIndex,
						agreeReject,
						renterPid,
					)
					if err != nil {
						cb <- err
					} else {
						if err := proto.Unmarshal(signedGuardContractBytes, signedGuardContract); err != nil {
							cb <- err
						} else {
							inquiryResult <- signedGuardContract
						}
					}
				}()
				tick := time.Tick(30 * time.Second)
				select {
				case err = <-cb:
					return err
				case <-tick:
					return errors.New("host timeout")
				case signedContract := <-inquiryResult:
					signedContracts = append(signedContracts, signedContract)
					return nil
				}
			}, uh.HandleShardBo)
			if err != nil {
				rejectedHosts = append(rejectedHosts, contract)
				log.Warn("Failed to setup contract with %s", hostId)
			}
		}(hostPidMap[contract.ContractId], askPriceMap[contract.ContractId], contract.ShardIndex, contract.ShardHash, contract.ShardFileSize)
	}

	if len(rejectedHosts) > 0 {
		hp = uh.GetHostsProvider(ctxParams, make([]string, 0))
		replacedHostContracts, err := renewHostProcess(ctxParams, renterPid, fileHash, storageLength, hostPidMap, askPriceMap, rejectedHosts, "reject", hp)
		if err != nil {
			return nil, errors.New("can not find appropriate hosts to replace rejected hosts")
		}
		signedContracts = append(signedContracts, replacedHostContracts...)
	}
	return signedContracts, nil
}
