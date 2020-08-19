package upload

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"

	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	cmds "github.com/TRON-US/go-btfs-cmds"
)

var StorageDcRepairRouterCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Interact with Host repair requests and responses for negotiation of shards repair.",
		ShortDescription: `
Guard broadcasts repair request to potential nodes, after negotiation, nodes prepare the contracts for 
such repair job, sign and send to the guard for confirmation `,
	},
	Subcommands: map[string]*cmds.Command{
		"request":  hostRepairRequestCmd,
		"response": hostRepairResponseCmd,
	},
}

type RepairContractParams struct {
	FileHash             string
	FileSize             int64
	RepairPid            string
	RepairSignTime       time.Time
	LostShardHashes      []string
	RepairContractId     string
	RepairRewardAmount   int64
	DownloadContractId   string
	DownloadRewardAmount int64
}

var hostRepairRequestCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Negotiate with hosts for repair jobs.",
		ShortDescription: `
This command sends request to mining host to negotiate the repair works.`,
	},
	Arguments: append([]cmds.Argument{
		cmds.StringArg("peer-ids", true, false, "Host Peer IDs to send repair requests."),
	}, hostRepairResponseCmd.Arguments...), // append pass-through arguments
	RunTimeout: 20 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		fileHash := req.Arguments[1]
		repairId := req.Arguments[6]
		repairContractId := req.Arguments[8]
		lostShardHashes := strings.Split(req.Arguments[2], ",")
		fileSize, err := strconv.ParseInt(req.Arguments[3], 10, 64)
		if err != nil {
			return err
		}
		downloadRewardAmount, err := strconv.ParseInt(req.Arguments[4], 10, 64)
		if err != nil {
			return err
		}
		repairRewardAmount, err := strconv.ParseInt(req.Arguments[5], 10, 64)
		if err != nil {
			return err
		}
		err = emptyCheck(fileHash, lostShardHashes, repairId, repairContractId, fileSize, downloadRewardAmount, repairRewardAmount)
		if err != nil {
			return nil
		}
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}
		if !cfg.Experimental.HostRepairEnabled {
			return fmt.Errorf("host repair api not enabled")
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
		reapirIds := make([]string, len(peerIds))
		var wg sync.WaitGroup
		wg.Add(len(peerIds))
		for index, peerId := range peerIds {
			go func() {
				err = func() error {
					pi, err := remote.FindPeer(req.Context, n, peerId)
					if err != nil {
						return err
					}
					_, err = remote.P2PCallStrings(req.Context, n, api, pi.ID, "/storage/upload/dcrepairrouting/response",
						req.Arguments[1:]...)
					if err != nil {
						return err
					}
					reapirIds[index] = peerId
					return nil
				}()
				if err != nil {
					log.Debug("P2P call error for peerId %s", peerId)
				}
				wg.Done()
			}()
		}
		wg.Wait()
		return cmds.EmitOnce(res, &peerIdList{reapirIds})
	},
	Type: peerIdList{},
}

type peerIdList struct {
	Strings []string
}

var hostRepairResponseCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Host responds to repair requests.",
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
		cmds.StringArg("repair-pid", true, false, "Host Peer ID to send repair requests."),
		cmds.StringArg("download_contract_id", true, false, " Contract ID associated with the download requests."),
		cmds.StringArg("repair_contract_id", true, false, "Contract ID associated with the require requests."),
	},
	RunTimeout: 1 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		fileHash := req.Arguments[0]
		repairId := req.Arguments[5]
		downloadContractId := req.Arguments[6]
		repairContractId := req.Arguments[7]
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
		err = emptyCheck(fileHash, lostShardHashes, repairId, repairContractId, fileSize, downloadRewardAmount, repairRewardAmount)
		if err != nil {
			return err
		}
		ctxParams, err := uh.ExtractContextParams(req, env)
		if err != nil {
			return err
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
		if price > uint64(repairRewardAmount) {
			return fmt.Errorf("host desired price %d is greater than request price %d", price, repairRewardAmount)
		}
		submitSignedRepairContract(ctxParams, &RepairContractParams{
			FileHash:             fileHash,
			FileSize:             fileSize,
			RepairPid:            repairId,
			RepairSignTime:       time.Now(),
			LostShardHashes:      lostShardHashes,
			RepairContractId:     repairContractId,
			DownloadContractId:   downloadContractId,
			RepairRewardAmount:   repairRewardAmount,
			DownloadRewardAmount: downloadRewardAmount,
		})
		return nil
	},
}

func submitSignedRepairContract(ctxParams *uh.ContextParams, params *RepairContractParams) {
	go func() {
		err := func() error {
			repaireReq := &guardpb.RepairContract{
				FileHash:             params.FileHash,
				FileSize:             params.FileSize,
				RepairPid:            params.RepairPid,
				LostShardHash:        params.LostShardHashes,
				RepairSignTime:       time.Now().UTC(),
				RepairContractId:     params.RepairContractId,
				DownloadContractId:   params.DownloadContractId,
				RepairRewardAmount:   params.RepairRewardAmount,
				DownloadRewardAmount: params.DownloadRewardAmount,
			}
			sig, err := crypto.Sign(ctxParams.N.PrivateKey, repaireReq)
			if err != nil {
				return err
			}
			repaireReq.RepairSignature = sig
			err = grpc.GuardClient(ctxParams.Cfg.Services.GuardDomain).WithContext(ctxParams.Ctx, func(ctx context.Context,
				client guardpb.GuardServiceClient) error {
				_, err = client.SubmitRepairContract(ctx, repaireReq)
				if err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return err
			}
			return nil
		}()
		if err != nil {
			log.Errorf("repairer signed and submitted contract error: %s", err.Error())
		}
	}()
}

func emptyCheck(fileHash string, lostShardHashes []string, RepairPid string, repairContractId string, fileSize int64, DownloadRewardAmount int64, RepairRewardAmount int64) error {
	if fileHash == "" {
		return fmt.Errorf("file Hash is empty")
	}
	if RepairPid == "" {
		return fmt.Errorf("repair id is empty")
	}
	if repairContractId == "" {
		return fmt.Errorf("repair contract id is empty")
	}
	if fileSize == 0 {
		return fmt.Errorf("file size is 0")
	}
	if len(lostShardHashes) == 0 {
		return fmt.Errorf("lost shard hashes do not specify")
	}
	if DownloadRewardAmount == 0 {
		return fmt.Errorf("download reward amount is 0")
	}
	if RepairRewardAmount == 0 {
		return fmt.Errorf("repair reward amount is 0")
	}
	return nil
}
