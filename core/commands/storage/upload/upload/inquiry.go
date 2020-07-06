package upload

import (
	"context"
	"errors"
	"github.com/TRON-US/go-btfs/core/hub"
	"time"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"
	"github.com/tron-us/protobuf/proto"

	"github.com/cenkalti/backoff/v4"
	"github.com/libp2p/go-libp2p-core/peer"
)

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
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		cfg, err := n.Repo.Config()
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
		contractList := make([]*guardpb.Contract, 0)
		storageLength := (int)(contracts[0].RentEnd.Sub(contracts[0].RentStart).Hours() / 24)
		for _, contract := range contracts {
			if contract.State == guardpb.Contract_UPLOADED {
				hostPidMap[contract.ShardHash] = contract.HostPid
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
				askTotalPrice += uh.TotalPay(contract.ShardFileSize, askPrice, int(storageLength))
				askDailyPrice += uh.TotalPay(contract.ShardFileSize, askPrice, 1)
				if contract.Price != askPrice {
					contract.Price = askPrice
				}
				bidPrice := contract.Amount
				bidTotalPrice += bidPrice
				contractList = append(contractList, contract)
			}
		}

		signedGuardContracts := make([]*guardpb.Contract, 0)
		if askTotalPrice <= bidTotalPrice {
			signedGuardContracts, err = agreeProcess(req, n, renterPid, fileHash, storageLength, hostPidMap, contractList)
		} else {
			storageLength = (int)(bidTotalPrice / askDailyPrice)
			for _, contract := range contractList {
				contract.RentEnd = contract.RentStart.Add(time.Duration(storageLength*24) * time.Hour)
			}
			signedGuardContracts, err = agreeProcess(req, n, renterPid, fileHash, storageLength, hostPidMap, contractList)
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
		if v, ok := cmap[contract.ContractId]; ok {
			contract = v
		}
	}
}

func agreeProcess(req *cmds.Request, n *core.IpfsNode, renterPid string, fileHash string, storageLength int, hostPidMap map[string]string, guardContracts []*guardpb.Contract) ([]*guardpb.Contract, error) {
	cb := make(chan error)
	inquiryResult := make(chan *guardpb.Contract)
	rejectedHosts := make([]*guardpb.Contract, 0)
	signedContracts := make([]*guardpb.Contract, 0)
	ssId, _ := uh.SplitContractId(guardContracts[0].ContractId)

	for _, contract := range guardContracts {
		signedGuardContract := &guardpb.Contract{}
		go func(hostId string, shardIndex int32, shardHash string) {
			err := backoff.Retry(func() error {
				hostPid, err := peer.IDB58Decode(hostId)
				if err != nil {
					log.Errorf("shard %s decodes host_pid error: %s", shardHash, err.Error())
					return err
				} else {
					contract.HostPid = hostId
				}
				go func() {
					ctx, _ := context.WithTimeout(req.Context, 10*time.Second)
					signedGuardContractBytes, err := remote.P2PCall(ctx, n, hostPid, "/storage/renew/renewinit",
						ssId,
						fileHash,
						shardHash,
						contract,
						storageLength,
						shardIndex,
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
		}(hostPidMap[contract.ShardHash], contract.ShardIndex, contract.ShardHash)
		if len(rejectedHosts) > 0 {
			// TODO host reject process to find new hosts, collect the signed contracts and integrate with agreed contracts
			replacedHostContracts, err := rejectProcess(rejectedHosts)
			if err != nil {
				return nil, errors.New("can not find appropriate hosts to replace rejected hosts")
			}
			signedContracts = append(signedContracts, replacedHostContracts...)
		}
	}
	return signedContracts, nil
}

// find new host, download shards and return signed contracts
func rejectProcess(rejectHosts []*guardpb.Contract) ([]*guardpb.Contract, error) {
	var replacedHostContracts []*guardpb.Contract
	// TODO add logic here
	return replacedHostContracts, nil
}
