package upload

import (
	"context"
	"fmt"
	"github.com/TRON-US/go-btfs-common/utils/grpc"
	"strconv"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	uh "github.com/TRON-US/go-btfs/core/commands/storage/upload/helper"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"

	"github.com/tron-us/go-btfs-common/crypto"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	cmds "github.com/TRON-US/go-btfs-cmds"
)

var StorageDcRepairRoutingCmd = &cmds.Command{
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
		pi, err := remote.FindPeer(req.Context, n, req.Arguments[0])
		if err != nil {
			return err
		}

		peerIds := strings.Split(req.Arguments[0], ",")
		for _, peerId := range peerIds {
			pi, err := remote.FindPeer(req.Context, n, peerId)
			if err != nil {
				return err
			}
			_, err = remote.P2PCallStrings(req.Context, n, api, pi.ID, "/storage/upload/dcrepairrouting/response",
				req.Arguments[1:]...)
			if err != nil {
				return err
			}
		}
		return nil
	},
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
		ctxParams, err := uh.ExtractContextParams(req, env)
		repairId := req.Arguments[5]
		hostId := ctxParams.N.Identity.Pretty()
		if repairId != hostId {
			return fmt.Errorf("repair id %s is mismatched with host id %s", repairId, hostId)
		}
		ns, err := helper.GetHostStorageConfig(ctxParams.Ctx, ctxParams.N)
		var price uint64
		if ns.RepairCustomizedPricing {
			price = ns.RepairPriceCustomized
		} else {
			price = ns.RepairPriceDefault
		}
		requestPrice, err := strconv.ParseInt(req.Arguments[4], 10, 64)
		if err != nil {
			return nil
		}
		if price < uint64(requestPrice) {
			return fmt.Errorf("host desired price %d is greater than request price %d", price, requestPrice)
		}
		fileHash := req.Arguments[0]
		downloadContractId := req.Arguments[6]
		repairContractId := req.Arguments[7]
		lostShardHashes := strings.Split(req.Arguments[1], ",")
		fileSize, err := strconv.ParseInt(req.Arguments[2], 10, 64)
		downloadRewardAmout, err := strconv.ParseInt(req.Arguments[3], 10, 64)
		repairRewardAmount, err := strconv.ParseInt(req.Arguments[4], 10, 64)
		repaireReq := &guardpb.RepairContract{
			FileHash:             fileHash,
			LostShardHash:        lostShardHashes,
			FileSize:             fileSize,
			DownloadRewardAmount: downloadRewardAmout,
			RepairRewardAmount:   repairRewardAmount,
			RepairPid:            repairId,
			RepairSignTime:       time.Now().UTC(),
			DownloadContractId:   downloadContractId,
			RepairContractId:     repairContractId,
		}
		sig, err := crypto.Sign(ctxParams.N.PrivateKey, repaireReq)
		if err != nil {
			return err
		}
		repaireReq.RepairSignature = sig

		var reapirResp *guardpb.RepairContractResponse
		err = grpc.GuardClient(ctxParams.Cfg.Services.GuardDomain).WithContext(req.Context, func(ctx context.Context,
			client guardpb.GuardServiceClient) error {
			reapirResp, err = client.SubmitRepairContract(ctx, repaireReq)
			if err != nil {
				return err
			}
			return nil
		})
		return nil
	},
}
