package upload

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io/ioutil"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/commands/storage/upload/sessions"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"

	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	shell "github.com/TRON-US/go-btfs-api"
	cmds "github.com/TRON-US/go-btfs-cmds"

	"github.com/alecthomas/units"
	"github.com/cenkalti/backoff/v4"
	//"github.com/google/uuid"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/tron-us/protobuf/proto"

	"go.uber.org/zap"
)

type RepairContractParams struct {
	FileHash             string
	FileSize             int64
	RepairPid            string
	LostShardHashes      []string
	RepairRewardAmount   int64
	DownloadRewardAmount int64
}

const requestInterval = 5 //minutes

var requestFileHash string
var StorageDcRepairRouterCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Interact with Host repair requests and responses for negotiation of shards repair.",
		ShortDescription: `
Guard broadcasts repair request to potential nodes, after negotiation, nodes prepare the contracts for 
such repair job, sign and send to the guard for confirmation `,
	},
	Subcommands: map[string]*cmds.Command{
		"request":  hostRepairRequestCmd,
		"response": HostRepairResponseCmd,
	},
}

var hostRepairRequestCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Negotiate with hosts for repair jobs.",
		ShortDescription: `
This command sends request to mining host to negotiate the repair works.`,
	},
	Arguments: append([]cmds.Argument{
		cmds.StringArg("peer-ids", true, false, "Host Peer IDs to send repair requests."),
	}, HostRepairResponseCmd.Arguments...), // append pass-through arguments
	RunTimeout: 20 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {

		timeStr := time.Now().Format("2006-01-02 15:04:05")
		fmt.Println("========== get into /storage/dcrepair/request", timeStr, "==========")

		fileHash := req.Arguments[1]
		lostShardHashes := strings.Split(req.Arguments[2], ",")
		fileSize, err := strconv.ParseInt(req.Arguments[3], 10, 64)
		if err != nil {
			fmt.Println("fileSize parse err", err)
			return err
		}
		downloadRewardAmount, err := strconv.ParseInt(req.Arguments[4], 10, 64)
		if err != nil {
			fmt.Println("downloadRewardAmount parse err", err)
			return err
		}
		repairRewardAmount, err := strconv.ParseInt(req.Arguments[5], 10, 64)
		if err != nil {
			return err
		}
		fmt.Println("fileHash", fileHash, "lostShardHashes", lostShardHashes, "fileSize", fileSize, "downloadRewardAmount", downloadRewardAmount, "repairRewardAmount", repairRewardAmount)
		err = emptyCheck(fileHash, lostShardHashes, fileSize, downloadRewardAmount, repairRewardAmount)
		if err != nil {
			fmt.Println("Router empty check with err", err)
			return err
		}
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}
		peerIds := strings.Split(req.Arguments[0], ",")
		repairIds := make([]string, len(peerIds))
		var wg sync.WaitGroup
		wg.Add(len(peerIds))
		for index, peerId := range peerIds {
			go func() {
				err = func() error {
					pi, err := remote.FindPeer(req.Context, n, peerId)
					if err != nil {
						fmt.Println("remote.FindPeer with err", err)
						return err
					}
					_, err = remote.P2PCallStrings(req.Context, n, api, pi.ID, "/storage/dcrepair/response",
						req.Arguments[1:]...)
					if err != nil {
						fmt.Println("remote.P2PCallStrings with err", err)
						return err
					}
					repairIds[index] = peerId
					return nil
				}()
				if err != nil {
					fmt.Println("P2P call error", zap.Error(err), zap.String("peerId", peerId))
					log.Error("P2P call error", zap.Error(err), zap.String("peerId", peerId))
				}
				wg.Done()
			}()
		}
		wg.Wait()
		return cmds.EmitOnce(res, &peerIdList{repairIds})
	},
	Type: peerIdList{},
}

type peerIdList struct {
	Strings []string
}

var HostRepairResponseCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Host responds to repair jobs.",
		ShortDescription: `
This command enquires the repairer with the contract, if agrees with the contract after negotiation,
returns the repairer's signed contract to the invoker.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, false, "File hash for the host prepares to repair."),
		cmds.StringArg("repair-shard-hash", true, false, "Shard hash for the host prepares to repair."),
		cmds.StringArg("file-size", true, false, "Size of the repair file."),
		cmds.StringArg("download-reward-amount", true, false, "Reward amount for download workload."),
		cmds.StringArg("repair-reward-amount", true, false, "Reward amount for repair workload."),
	},
	RunTimeout: 1 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		timeStr := time.Now().Format("2006-01-02 15:04:05")
		fmt.Println("========== get into /storage/dcrepair/response", timeStr, "==========")
		ctxParams, err := uh.ExtractContextParams(req, env)
		repairId := ctxParams.N.Identity.Pretty()
		fileHash := req.Arguments[0]
		if requestFileHash == fileHash {
			fmt.Printf("file {%s} has been repairing on the host {%s}", fileHash, repairId)
			return fmt.Errorf("file {%s} has been repairing on the host {%s}", fileHash, repairId)
		}
		requestFileHash = fileHash
		lostShardHashes := strings.Split(req.Arguments[1], ",")
		fileSize, err := strconv.ParseInt(req.Arguments[2], 10, 64)
		if err != nil {
			return err
		}
		downloadRewardAmount, err := strconv.ParseInt(req.Arguments[3], 10, 64)
		if err != nil {
			return err
		}
		repairRewardAmount, err := strconv.ParseInt(req.Arguments[4], 10, 64)
		if err != nil {
			return err
		}
		err = emptyCheck(fileHash, lostShardHashes, fileSize, downloadRewardAmount, repairRewardAmount)
		if err != nil {
			fmt.Println("Repairer empty check with err", err)
			return err
		}
		if err != nil {
			return err
		}
		if !ctxParams.Cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}
		if !ctxParams.Cfg.Experimental.HostRepairEnabled {
			return fmt.Errorf("storage repair api not enabled")
		}
		ns, err := helper.GetHostStorageConfig(ctxParams.Ctx, ctxParams.N)
		if err != nil {
			return err
		}
		var price uint64
		if ns.RepairCustomizedPricing {
			price = ns.RepairPriceCustomized
		} else {
			price = ns.RepairPriceDefault
		}
		totalPrice := uint64(float64(fileSize) / float64(units.GiB) * float64(price))
		if totalPrice <= 0 {
			totalPrice = 1
		}
		if totalPrice > uint64(repairRewardAmount) {
			return fmt.Errorf("host desired price %d is greater than request price %d", totalPrice, repairRewardAmount)
		}
		fmt.Println("fileHash", fileHash, "lostShardHashes", lostShardHashes, "fileSize", fileSize, "downloadRewardAmount", downloadRewardAmount, "repairRewardAmount", repairRewardAmount)
		doRepair(ctxParams, &RepairContractParams{
			FileHash:             fileHash,
			FileSize:             fileSize,
			RepairPid:            repairId,
			LostShardHashes:      lostShardHashes,
			RepairRewardAmount:   repairRewardAmount,
			DownloadRewardAmount: downloadRewardAmount,
		})
		fmt.Println("decentralized repair process done")
		return nil
	},
}

func doRepair(ctxParams *uh.ContextParams, params *RepairContractParams) {
	go func() {
		err := func() error {
			repairContractResp, err := submitSignedRepairContract(ctxParams, params)
			fmt.Println("repairContractResp status", repairContractResp.Status)
			fmt.Println("repairContractResp contract", repairContractResp.Contract)
			if err != nil {
				fmt.Println("submitSignedRepairContract err", err)
				return err
			}
			if repairContractResp.Status == guardpb.RepairContractResponse_BOTH_SIGNED {
				// check payment
				repairContract := repairContractResp.Contract
				signedContractID, err := signContractID(repairContract.RepairContractId, ctxParams.N.PrivateKey)
				if err != nil {
					fmt.Println("obtain signContractID err", err)
					return err
				}
				fmt.Println("signedContractID", signedContractID)
				paidIn := make(chan bool)
				go checkPaymentFromClient(ctxParams, paidIn, signedContractID)
				paid := <-paidIn
				if !paid {
					fmt.Println("contract is not paid", repairContract.RepairContractId)
					//return fmt.Errorf("contract is not paid: %s", repairContract.RepairContractId)
				} else {
					fmt.Println("contract has paid", repairContract.RepairContractId)
				}
				_, err = downloadAndRebuildFile(ctxParams, repairContract.FileHash, repairContract.LostShardHash)
				if err != nil {
					fmt.Println("downloadAndRebuildFile err", err)
					return err
				}
				err = challengeLostShards(ctxParams, repairContract)
				if err != nil {
					fmt.Println("challengeLostShards err", err)
					return err
				}
				timeStr := time.Now().Format("2006-01-02 15:04:05")
				fmt.Println("========== repairer download done time", timeStr, "==========")
				fileStatus, err := uploadShards(ctxParams, repairContract)
				if err != nil {
					fmt.Println("uploadShards err", err)
					return err
				}
				if fileStatus == nil {
					return nil
				}
				timeStr = time.Now().Format("2006-01-02 15:04:05")
				fmt.Println("========== challenge repairer done time", timeStr, "==========")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				err = submitFileStatus(ctx, ctxParams.Cfg, fileStatus)
				if err != nil {
					fmt.Println("submitFileStatus err", err)
					return err
				}
				timeStr = time.Now().Format("2006-01-02 15:04:05")
				fmt.Println("==========repair done end time", timeStr)
			}
			return nil
		}()
		if err != nil {
			fmt.Println("repair error:", err.Error())
			log.Errorf("repair error: %s", err.Error())
		}
	}()
}

func submitSignedRepairContract(ctxParams *uh.ContextParams, params *RepairContractParams) (*guardpb.RepairContractResponse, error) {
	repairReq := &guardpb.RepairContract{
		FileHash:             params.FileHash,
		FileSize:             params.FileSize,
		RepairPid:            params.RepairPid,
		LostShardHash:        params.LostShardHashes,
		RepairSignTime:       time.Now().UTC(),
		RepairRewardAmount:   params.RepairRewardAmount,
		DownloadRewardAmount: params.DownloadRewardAmount,
	}
	sig, err := crypto.Sign(ctxParams.N.PrivateKey, repairReq)
	if err != nil {
		fmt.Println("crypto.Sign err", err)
		return nil, err
	}
	repairReq.RepairSignature = sig
	var repairResp *guardpb.RepairContractResponse
	err = grpc.GuardClient(ctxParams.Cfg.Services.GuardDomain).WithContext(ctxParams.Ctx, func(ctx context.Context,
		client guardpb.GuardServiceClient) error {
		repairResp, err = client.SubmitRepairContract(ctx, repairReq)
		if err != nil {
			fmt.Println("client.SubmitRepairContract", err)
			return err
		}
		return nil
	})
	if err != nil {
		fmt.Println("grpc.GuardClient err", err)
		return nil, err
	}
	return repairResp, nil
}

func uploadShards(ctxParams *uh.ContextParams, repairContract *guardpb.RepairContract) (*guardpb.FileStoreStatus, error) {
	repairContractReq := &guardpb.RequestRepairContracts{
		FileHash:       repairContract.FileHash,
		RepairNode:     repairContract.RepairPid,
		RepairSignTime: repairContract.RepairSignTime,
	}
	repairSignature, err := crypto.Sign(ctxParams.N.PrivateKey, repairContractReq)
	if err != nil {
		fmt.Println("repairContractReq err", err)
		return nil, err
	}
	repairContractReq.RepairSignature = repairSignature
	var statusMeta *guardpb.FileStoreStatus
	var repairContractResp *guardpb.ResponseRepairContracts
	duration := time.Duration(requestInterval)
	tick := time.Tick(duration * time.Minute)
	//ctx := context.Background()
	ctxParams.Ctx = context.Background()

FOR:
	for true {
		select {
		case <-tick:
			//ctx, _ := context.WithTimeout(context.Background(), 5 * time.Minute)
			err = grpc.GuardClient(ctxParams.Cfg.Services.GuardDomain).WithContext(ctxParams.Ctx, func(ctx context.Context,
				client guardpb.GuardServiceClient) error {
				repairContractResp, err = client.RequestForRepairContracts(ctx, repairContractReq)
				if err != nil {
					fmt.Println("RequestForRepairContracts err", err)
					return err
				}
				return nil
			})
			if err != nil {
				fmt.Println("grpc.GuardClient err", err)
				return nil, err
			}
			fmt.Println("client.RequestForRepairContract state", repairContractResp.State)
			if repairContractResp.State == guardpb.ResponseRepairContracts_REQUEST_AGAIN {
				fmt.Println("repairer in REQUEST_AGAIN status", repairContract.RepairPid)
				log.Info(fmt.Sprintf("request repair contract again in %d minutes", requestInterval))
				//cancel()
				continue
			}
			if repairContractResp.State == guardpb.ResponseRepairContracts_CONTRACT_READY {
				fmt.Println("repairer in CONTRACT_READY status", repairContract.RepairPid)
				statusMeta = repairContractResp.Status
				break FOR
			}
			if repairContractResp.State == guardpb.ResponseRepairContracts_DOWNLOAD_NOT_DONE {
				fmt.Println("repairer in DOWNLOAD_NOT_DONE status", repairContract.RepairPid)
				unpinLocalStorage(ctxParams, repairContract.FileHash)
				log.Info("download and challenge can not be completed for lost shards", zap.String("repairer id", repairContract.RepairPid))
				return nil, nil
			}
			if repairContractResp.State == guardpb.ResponseRepairContracts_CONTRACT_CLOSED {
				fmt.Println("repairer in CONTRACT_CLOSED status", repairContract.RepairPid)
				unpinLocalStorage(ctxParams, repairContract.FileHash)
				log.Info("repair contract has been closed", zap.String("contract id", repairContract.RepairContractId))
				return nil, nil
			}
		}
	}

	timeStr := time.Now().Format("2006-01-02 15:04:05")
	fmt.Println("========== start to reapir", timeStr, "==========")

	if statusMeta == nil {
		return nil, fmt.Errorf("file store status is nil")
	}
	contracts := statusMeta.Contracts
	shardHashes := repairContract.LostShardHash

	fmt.Println("Guard return contracts of RequestForRepairContracts with length", contracts, len(contracts))

	if len(contracts) != len(shardHashes) {
		return nil, errors.New("contracts size is not equal to lost shard hashes count")
	}
	//statusMeta.CheckFrequency = 0
	//statusMeta.CheckFrequencyWarn = 0
	//statusMeta.WarnChallengeTimesLimit = 0
	//statusMeta.SuccessChallengeTimesLimit = 0
	//statusMeta.ShardCount = int32(len(contracts))
	//statusMeta.PreparerPid = ctxParams.N.Identity.Pretty()
	//statusMeta.RentalState = guardpb.FileStoreStatus_PARTIAL_NEW

	if len(contracts) <= 0 {
		fmt.Println("length of contracts is 0")
		return nil, errors.New("length of contracts is 0")
	}
	fmt.Println("length of original contracts is", len(contracts))
	fmt.Println("origin contracts", contracts)
	ssId := uuid.New().String()
	fmt.Println("repairContract.LostShardHash", shardHashes)

	//---
	rss, err := sessions.GetRenterSession(ctxParams, ssId, repairContract.FileHash, shardHashes)
	if err != nil {
		fmt.Println("get rss err", err)
		return nil, err
	}

	hp := uh.GetHostsProvider(ctxParams, make([]string, 0))
	fmt.Println("uh.GetHostsProvider method hp", hp)

	shardMap := make(map[int]string)

	fmt.Println("sessions.GetRenterSession method rss", rss)
	fmt.Println("downloadAndSignContracts context", rss.Ctx)

	str1 := [...]string{
		"16Uiu2HAm5qk1jCB3fuB6FJU9CL9cD3nbVtsercfe9gX9yXsooqyG", "16Uiu2HAmVsYfPnwYyZ3bR5uYWdKzDoyBUaqfokmQSzkQ9pHpkfrk", "16Uiu2HAmMMZRg8yk24YVqiBhnXbfPPYzb2RBUzq4TfQePAtHuu91",
		"16Uiu2HAmBAspopbZYasfsZ8wTzdfpu3uPW7wPweuk73TPECDk4iL", "16Uiu2HAm2B5FK2ZMRCVhaAyc8vbteRD9bbeRi9BbKbJXV9CYbDCe", "16Uiu2HAm3imiDppF1pM1kRo8KmP4KXXebdTDAkEuE2CMCdXE3mUY",
		"16Uiu2HAkx14DyfvhU29cTPkXDvmkyctxAUqwwXxQJwZkXWLjF2YH", "16Uiu2HAmQXGfjV1c9QUZWoduV2c7ncXs8k28YGkiTBCKryqNPP2T", "16Uiu2HAkuWhstXeoopKP81kp9K66gTQ5urxnEC1ZCWCiLRZyUMT2",
		"16Uiu2HAm6ZkTUTpzHyUN3EDj6UpcyyyZ88z8cHRMatUHeGNVxuvH", "16Uiu2HAmTSgfM8LMQQvmamNwR138757AC4dUXjRWfFeRfSMvCdAq", "16Uiu2HAkzrR9aJokqkg3nUrpL4XSQoo7rw31ZNjbp2kUoU8w4gH4",
		"16Uiu2HAmNFmLkVKCP19xfBMvBDem7xRL2YbYnzNGVHNsCwmCVJLS", "16Uiu2HAmQqdw81Xm7STx8FsJP8ZSzam3EWtepeUfPnFoGnBjJHKn", "16Uiu2HAkxTqPmKLmrFGNV6UqPtfFixYc8zCEpLgmZVRouFf43wyu",
		"16Uiu2HAmVRgeLHT12bpRwxQTdTMXkS7TgATe2gXm8ygubcrfjqaR", "16Uiu2HAmJwATqx6K7RLYaopj6m4caTsCvg2BqPySGTrNJd7KBtUq",
		"16Uiu2HAm5qk1jCB3fuB6FJU9CL9cD3nbVtsercfe9gX9yXsooqyG", "16Uiu2HAmVsYfPnwYyZ3bR5uYWdKzDoyBUaqfokmQSzkQ9pHpkfrk", "16Uiu2HAmMMZRg8yk24YVqiBhnXbfPPYzb2RBUzq4TfQePAtHuu91",
		"16Uiu2HAmBAspopbZYasfsZ8wTzdfpu3uPW7wPweuk73TPECDk4iL", "16Uiu2HAm2B5FK2ZMRCVhaAyc8vbteRD9bbeRi9BbKbJXV9CYbDCe", "16Uiu2HAm3imiDppF1pM1kRo8KmP4KXXebdTDAkEuE2CMCdXE3mUY",
		"16Uiu2HAkx14DyfvhU29cTPkXDvmkyctxAUqwwXxQJwZkXWLjF2YH", "16Uiu2HAmQXGfjV1c9QUZWoduV2c7ncXs8k28YGkiTBCKryqNPP2T", "16Uiu2HAkuWhstXeoopKP81kp9K66gTQ5urxnEC1ZCWCiLRZyUMT2",
		"16Uiu2HAm6ZkTUTpzHyUN3EDj6UpcyyyZ88z8cHRMatUHeGNVxuvH", "16Uiu2HAmTSgfM8LMQQvmamNwR138757AC4dUXjRWfFeRfSMvCdAq", "16Uiu2HAkzrR9aJokqkg3nUrpL4XSQoo7rw31ZNjbp2kUoU8w4gH4",
		"16Uiu2HAmNFmLkVKCP19xfBMvBDem7xRL2YbYnzNGVHNsCwmCVJLS", "16Uiu2HAmQqdw81Xm7STx8FsJP8ZSzam3EWtepeUfPnFoGnBjJHKn", "16Uiu2HAkxTqPmKLmrFGNV6UqPtfFixYc8zCEpLgmZVRouFf43wyu",
		"16Uiu2HAmVRgeLHT12bpRwxQTdTMXkS7TgATe2gXm8ygubcrfjqaR", "16Uiu2HAmJwATqx6K7RLYaopj6m4caTsCvg2BqPySGTrNJd7KBtUq",
		"16Uiu2HAm5qk1jCB3fuB6FJU9CL9cD3nbVtsercfe9gX9yXsooqyG", "16Uiu2HAmVsYfPnwYyZ3bR5uYWdKzDoyBUaqfokmQSzkQ9pHpkfrk", "16Uiu2HAmMMZRg8yk24YVqiBhnXbfPPYzb2RBUzq4TfQePAtHuu91",
		"16Uiu2HAmBAspopbZYasfsZ8wTzdfpu3uPW7wPweuk73TPECDk4iL", "16Uiu2HAm2B5FK2ZMRCVhaAyc8vbteRD9bbeRi9BbKbJXV9CYbDCe", "16Uiu2HAm3imiDppF1pM1kRo8KmP4KXXebdTDAkEuE2CMCdXE3mUY",
		"16Uiu2HAkx14DyfvhU29cTPkXDvmkyctxAUqwwXxQJwZkXWLjF2YH", "16Uiu2HAmQXGfjV1c9QUZWoduV2c7ncXs8k28YGkiTBCKryqNPP2T", "16Uiu2HAkuWhstXeoopKP81kp9K66gTQ5urxnEC1ZCWCiLRZyUMT2",
		"16Uiu2HAm6ZkTUTpzHyUN3EDj6UpcyyyZ88z8cHRMatUHeGNVxuvH", "16Uiu2HAmTSgfM8LMQQvmamNwR138757AC4dUXjRWfFeRfSMvCdAq", "16Uiu2HAkzrR9aJokqkg3nUrpL4XSQoo7rw31ZNjbp2kUoU8w4gH4",
		"16Uiu2HAmNFmLkVKCP19xfBMvBDem7xRL2YbYnzNGVHNsCwmCVJLS", "16Uiu2HAmQqdw81Xm7STx8FsJP8ZSzam3EWtepeUfPnFoGnBjJHKn", "16Uiu2HAkxTqPmKLmrFGNV6UqPtfFixYc8zCEpLgmZVRouFf43wyu",
		"16Uiu2HAmVRgeLHT12bpRwxQTdTMXkS7TgATe2gXm8ygubcrfjqaR", "16Uiu2HAmJwATqx6K7RLYaopj6m4caTsCvg2BqPySGTrNJd7KBtUq",
	}

	ctx, _ := context.WithTimeout(rss.Ctx, 10*time.Minute)
	//for _, contract := range contracts {
	for index, contract := range contracts {
		if isContained(shardHashes, contract.ShardHash) {
			fmt.Println("get into downloadAndSignContracts loops")
			shardMap[int(contract.ShardIndex)] = contract.ShardHash
			//find new host here and set rs with new context
			downloadAndSignContracts(contract, rss, ctx, hp, str1[index])
		}
	}

	updatedContracts, err := retrieveUpdatedContracts(shardMap, rss)
	if err != nil {
		fmt.Printf("appendUpdatedContracts with err: %v", err)
		return nil, err
	}

	fmt.Println("---updatedContracts", updatedContracts)
	fmt.Println("====================")
	fmt.Println("update statusMeta contracts done", len(updatedContracts))
	fileStatus, err := NewFileStatus(updatedContracts, rss.CtxParams.Cfg, updatedContracts[0].ContractMeta.RenterPid, rss.Hash, statusMeta.FileSize)
	if err != nil {
		return nil, err
	}
	sign, err := crypto.Sign(rss.CtxParams.N.PrivateKey, &fileStatus.FileStoreMeta)
	if err != nil {
		return nil, err
	}
	fileStatus.PreparerSignature = sign
	fmt.Println("---new statusMeta.Contracts", fileStatus.Contracts)
	return fileStatus, nil
}

func downloadAndSignContracts(contract *guardpb.Contract, rss *sessions.RenterSession, ctx context.Context, hp uh.IHostsProvider, str1 string) { //(map[int]string, error){
	fmt.Println("get into downloadAndSignContracts method 1")
	signContractErr := make([]string, 0)
	go func() {
		err := backoff.Retry(func() error {
			timeStr := time.Now().Format("2006-01-02 15:04:05")
			fmt.Println("start time of downloadAndSignContracts", timeStr)
			fmt.Println("rss.ctx of downloadAndSignContracts is", rss.Ctx)
			select {
			case <-ctx.Done():
				timeStr = time.Now().Format("2006-01-02 15:04:05")
				fmt.Println("end time of downloadAndSignContracts", timeStr)
				fmt.Println("ctx.Done() for downloadAndSignContracts")
				return nil
			default:
				break
			}
			fmt.Println("update contracts for new host")
			price := contract.ContractMeta.Price
			shardHash := contract.ContractMeta.ShardHash
			shardIndex := contract.ContractMeta.ShardIndex
			shardFileSize := contract.ContractMeta.ShardFileSize
			fmt.Println("start to find new host")

			// check host pid and replace by testing host
			/*host, err := hp.NextValidHost(price)
			fmt.Println("===host id is", host)
			if err != nil {
				fmt.Println("hp.NextValidHost(price) err", err)
				terr := rss.To(sessions.RssToErrorEvent, err)
				if terr != nil {
				}
				return nil
			}*/
			//host := "16Uiu2HAm5qk1jCB3fuB6FJU9CL9cD3nbVtsercfe9gX9yXsooqyG"
			host := str1
			//host := "16Uiu2HAkvoFrXHaxNyaGt1LX6Hzg2ZmAPAyawkFjeyKSq9g8pCeK"
			fmt.Println("===host peer id", host)
			repairPid := rss.CtxParams.N.Identity
			//contractId := uh.NewContractID(rss.SsId)
			//contract.ContractMeta.ContractId = contractId
			contract.ContractMeta.HostPid = host
			contract.State = guardpb.Contract_RENEWED
			contract.PreparerPid = repairPid.Pretty()
			// add repairer signature
			sign, err := crypto.Sign(rss.CtxParams.N.PrivateKey, &contract.ContractMeta)
			if err != nil {
				return fmt.Errorf("falled to sign updated contract {%s}", contract.ContractMeta.ContractId)
			}
			contract.PreparerSignature = sign
			contract.RenterSignature = nil
			contract.GuardSignature = nil

			fmt.Println("====== updated contract", contract, "======")
			guardContractBytes, err := proto.Marshal(contract)
			if err != nil {
				return nil
			}
			hostPid, err := peer.IDB58Decode(host)
			fmt.Println("peer.IDB58Decode(host) host pid is", hostPid)
			if err != nil {
				fmt.Println("shard decodes host_pid error", shardHash, err)
				log.Errorf("shard %s decodes host_pid error: %s", shardHash, err.Error())
				return err
			}
			cb := make(chan error)
			ShardErrChanMap.Set(contract.ContractMeta.ContractId, cb)
			go func() {
				fmt.Println("p2p call with host Pid", hostPid, "fileHash", rss.Hash, "shardHash", shardHash, "shardIndex", shardIndex, "price", price, "")
				fmt.Println("remote.P2PCall", hostPid, shardHash, err)

				timeStr = time.Now().Format("2006-01-02 15:04:05")
				fmt.Println("======start remote.P2PCall at time======", timeStr)

				newCtx, _ := context.WithTimeout(ctx, 10*time.Second)
				_, err := remote.P2PCall(newCtx, rss.CtxParams.N, rss.CtxParams.Api, hostPid, "/storage/upload/init",
					rss.SsId,
					rss.Hash,
					shardHash,
					price,
					"",
					guardContractBytes,
					-1,
					shardFileSize,
					shardIndex,
					repairPid,
				)
				if err != nil {
					fmt.Println("remote.P2PCall", hostPid, shardHash, err)

					timeStr = time.Now().Format("2006-01-02 15:04:05")
					fmt.Println("======generate p2p call error at time======", timeStr)

					cb <- err
				}
			}()
			// host needs to send recv in 30 seconds, or the contract will be invalid.
			tick := time.Tick(30 * time.Second)
			select {
			case err = <-cb:
				ShardErrChanMap.Remove(contract.ContractMeta.ContractId)
				fmt.Println("remote.P2PCall err", err)

				timeStr = time.Now().Format("2006-01-02 15:04:05")
				fmt.Println("======p2p error pop up time======", timeStr)

				return err
			case <-tick:
				fmt.Println("host timeout")
				return errors.New("host timeout")
			}
		}, uh.HandleShardBo)
		if err != nil {
			signContractErr = append(signContractErr, contract.ShardHash)
			fmt.Println("timeout: failed to setup contract in " + uh.HandleShardBo.MaxElapsedTime.String())
			_ = rss.To(sessions.RssToErrorEvent,
				errors.New("timeout: failed to setup contract in "+uh.HandleShardBo.MaxElapsedTime.String()))
		}
		fmt.Println("done p2p call for init shard hash", contract.ShardHash)
	}()
	//}
	//}
	//add log for failed shard hash
	if len(signContractErr) > 0 {
		log.Debugf("sign contract error for shard hashes: %s", strings.Join(signContractErr, ","))
	}
	/*if len(signContractErr) > 0 {
		return nil, fmt.Errorf("sign contract error for shard hashes: {%s}", strings.Join(signContractErr, ","))
	} else {
		return shardMap, nil
	}*/
}

func retrieveUpdatedContracts(shardMap map[int]string, rss *sessions.RenterSession) ([]*guardpb.Contract, error) {
	tick := time.Tick(5 * time.Second)
	updatedContracts := make([]*guardpb.Contract, 0)
	fmt.Println("---get into appendUpdatedContracts(), shardMap", shardMap)
	timeStr := time.Now().Format("2006-01-02 15:04:05")
	fmt.Println("start time of appendUpdatedContracts", timeStr)
	fmt.Println("rss.ctx of appendUpdatedContracts is", rss.Ctx)
	//ctx, cancel := context.WithTimeout(context.Background(), 10 * time.Minute)
	ctx, cancel := context.WithTimeout(rss.Ctx, 10*time.Minute)
	fmt.Println("setting rss.ctx of appendUpdatedContracts is", ctx)
	defer cancel()
	for true {
		select {
		case <-tick:
			fmt.Println("---will get into queryUpdatedContracts, shardMap", shardMap)
			remainNum, queryContracts, err := queryUpdatedContracts(shardMap, rss)
			if err != nil {
				return nil, err
			}
			updatedContracts = append(updatedContracts, queryContracts...)
			fmt.Println("---queryUpdatedContracts(), shardMap", remainNum)
			if remainNum == 0 {
				return updatedContracts, nil
			}
			//return nil
		case <-ctx.Done(): //rss.Ctx.Done():
			log.Infof("session %s done", rss.SsId)
			timeStr = time.Now().Format("2006-01-02 15:04:05")
			fmt.Println("end time of appendUpdatedContracts", timeStr)
			fmt.Println("rss.Ctx.Done() for appendUpdatedContracts()")
			return updatedContracts, nil
		}
	}
	return updatedContracts, nil
}

func queryUpdatedContracts(shardMap map[int]string, rss *sessions.RenterSession) (int, []*guardpb.Contract, error) {
	fmt.Println("---get into queryUpdatedContracts() shardMap", shardMap)
	queryContracts := make([]*guardpb.Contract, 0)
	contractStatus := "contract"
	for index, shardHash := range shardMap {
		shard, err := sessions.GetRenterShard(rss.CtxParams, rss.SsId, shardHash, index)
		if err != nil {
			fmt.Println("---sessions.GetRenterShard err", err)
			continue
		}
		fmt.Println("---sessions.GetRenterShard", shard)
		s, err := shard.Status()
		if err != nil {
			fmt.Println("---shard.Status() error", err)
			return -1, nil, err
		}
		fmt.Println("---shard.Status()", s)
		if s.Status == contractStatus {
			contracts, err := shard.Contracts()
			if err != nil {
				return -1, nil, err
			}
			guardContract := contracts.SignedGuardContract
			if guardContract != nil {
				fmt.Println("---shard.Contracts()", contracts)
				queryContracts = append(queryContracts, guardContract)
				delete(shardMap, index)
			}
		}
	}
	return len(shardMap), queryContracts, nil
}

/*func updateRepairContract(contracts []*guardpb.Contract, contractMap map[string]*guardpb.Contract) {
	for _, contract := range contracts {
		if contract.State == guardpb.Contract_LOST {
			if v, ok := contractMap[contract.ShardHash]; ok {
				contract = v
				fmt.Println("renew contract", contract)
				delete(contractMap, contract.ShardHash)
			}
		}
	}
}*/

func downloadAndRebuildFile(ctxParams *uh.ContextParams, fileHash string, repairShards []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	url := fmt.Sprint(strings.Split(ctxParams.Cfg.Addresses.API[0], "/")[2], ":", strings.Split(ctxParams.Cfg.Addresses.API[0], "/")[4])
	resp, err := shell.NewShell(url).Request("get", fileHash).Option("rs", strings.Join(repairShards, ",")).Send(ctx)
	if err != nil {
		fmt.Println("shell.NewShell err", err)
		return nil, err
	}
	defer resp.Close()

	if resp.Error != nil {
		fmt.Println("Http resp.Error", resp.Error)
		return nil, resp.Error
	}
	return ioutil.ReadAll(resp.Output)
}

func unpinLocalStorage(ctxParams *uh.ContextParams, fishHash string) error {
	url := fmt.Sprint(strings.Split(ctxParams.Cfg.Addresses.API[0], "/")[2], ":", strings.Split(ctxParams.Cfg.Addresses.API[0], "/")[4])
	err := shell.NewShell(url).Request("pin/rm", fishHash).Option("recursive", true).Exec(ctxParams.Ctx, nil)
	if err != nil {
		return err
	}
	return nil
}

func challengeLostShards(ctxParams *uh.ContextParams, repairReq *guardpb.RepairContract) error {
	errChan := make(chan error, len(repairReq.LostShardHash))
	for _, lostShardHash := range repairReq.LostShardHash {
		go func(shardHash string) {
			err := func() error {
				fileHash := repairReq.FileHash
				err := challengeShard(ctxParams, fileHash, true, &guardpb.ContractMeta{
					//RenterPid:   repairReq.RepairPid,
					FileHash:   fileHash,
					ShardHash:  shardHash,
					ContractId: repairReq.RepairContractId,
					HostPid:    repairReq.RepairPid,
					RentStart:  repairReq.RepairSignTime,
				})
				if err != nil {
					fmt.Println("challenge Shard err", err)
					fmt.Println("ContractId", repairReq.RepairContractId, "ShardHash", shardHash, "FileHash", repairReq.FileHash, "HostPid", repairReq.RepairPid)
					return err
				}
				return nil
			}()
			errChan <- err
		}(lostShardHash)
	}
	count := 0
	for err := range errChan {
		count++
		if err != nil {
			return err
		}
		if count == len(repairReq.LostShardHash) {
			break
		}
	}
	fmt.Println("challenge Shards without err")
	return nil
}

func emptyCheck(fileHash string, lostShardHashes []string, fileSize int64, DownloadRewardAmount int64, RepairRewardAmount int64) error {
	if fileHash == "" {
		return fmt.Errorf("file Hash is empty")
	}
	if fileSize == 0 {
		return fmt.Errorf("file size is 0")
	}
	if len(lostShardHashes) == 0 {
		return fmt.Errorf("lost shard hashes do not specify")
	}
	if DownloadRewardAmount <= 0 {
		return fmt.Errorf("download reward amount should be bigger than 0")
	}
	if RepairRewardAmount <= 0 {
		return fmt.Errorf("repair reward amount should be bigger than 0")
	}
	return nil
}

func isContained(lostShardHashes []string, shardHash string) bool {
	for _, lostShardHash := range lostShardHashes {
		if lostShardHash == shardHash {
			return true
		}
	}
	return false
}
