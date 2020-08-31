package upload

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/tron-us/protobuf/proto"

	"go.uber.org/zap"
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
					_, err = remote.P2PCallStrings(req.Context, n, api, pi.ID, "/storage/upload/dcrepairrouter/response",
						req.Arguments[1:]...)
					if err != nil {
						return err
					}
					reapirIds[index] = peerId
					return nil
				}()
				if err != nil {
					log.Error("P2P call error", zap.Error(err), zap.String("peerId", peerId))
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
		if !ctxParams.Cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}
		if !ctxParams.Cfg.Experimental.HostRepairEnabled {
			return fmt.Errorf("host repair api not enabled")
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
		doRepair(ctxParams, &RepairContractParams{
			FileHash:             fileHash,
			FileSize:             fileSize,
			RepairPid:            repairId,
			LostShardHashes:      lostShardHashes,
			RepairContractId:     repairContractId,
			DownloadContractId:   downloadContractId,
			RepairRewardAmount:   repairRewardAmount,
			DownloadRewardAmount: downloadRewardAmount,
		})
		return nil
	},
}

func doRepair(ctxParams *uh.ContextParams, params *RepairContractParams) {
	go func() {
		err := func() error {
			repairContractResp, err := submitSignedRepairContract(ctxParams, params)
			if err != nil {
				return err
			}
			if repairContractResp.Status == guardpb.RepairContractResponse_BOTH_SIGNED {
				// check payment
				repairContract := repairContractResp.Contract
				signedContractID, err := signContractID(repairContract.RepairContractId, ctxParams.N.PrivateKey)
				if err != nil {
					return err
				}
				paidIn := make(chan bool)
				go checkPaymentFromClient(ctxParams, paidIn, signedContractID)
				paid := <-paidIn
				if !paid {
					return fmt.Errorf("contract is not paid: %s", params.RepairContractId)
				}
				_, err = downloadAndRebuildFile(ctxParams, repairContract.FileHash, repairContract.LostShardHash)
				if err != nil {
					return err
				}
				err = challengeLostShards(ctxParams, repairContract)
				if err != nil {
					return err
				}
				fileStatus, err := uploadShards(ctxParams, repairContract)
				if err != nil {
					return err
				}
				err = submitFileStatus(ctxParams.Ctx, ctxParams.Cfg, fileStatus)
				if err != nil {
					return err
				}
			}
			return nil
		}()
		if err != nil {
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
		RepairContractId:     params.RepairContractId,
		DownloadContractId:   params.DownloadContractId,
		RepairRewardAmount:   params.RepairRewardAmount,
		DownloadRewardAmount: params.DownloadRewardAmount,
	}
	sig, err := crypto.Sign(ctxParams.N.PrivateKey, repairReq)
	if err != nil {
		return nil, err
	}
	repairReq.RepairSignature = sig
	var repairResp *guardpb.RepairContractResponse
	err = grpc.GuardClient(ctxParams.Cfg.Services.GuardDomain).WithContext(ctxParams.Ctx, func(ctx context.Context,
		client guardpb.GuardServiceClient) error {
		repairResp, err = client.SubmitRepairContract(ctx, repairReq)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
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
	sig, err := crypto.Sign(ctxParams.N.PrivateKey, repairContractReq)
	if err != nil {
		return nil, err
	}
	repairContractReq.RepairSignature = sig
	var statusMeta *guardpb.FileStoreStatus
	var repairContractResp *guardpb.ResponseRepairContracts

	tick := time.Tick(5 * time.Second)
FOR:
	for true {
		select {
		case <-tick:
			err = grpc.GuardClient(ctxParams.Cfg.Services.GuardDomain).WithContext(ctxParams.Ctx, func(ctx context.Context,
				client guardpb.GuardServiceClient) error {
				repairContractResp, err = client.RequestForRepairContracts(ctx, repairContractReq)
				if err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			if repairContractResp.State == guardpb.ResponseRepairContracts_CONTRACT_READY {
				statusMeta = repairContractResp.Status
				break FOR
			}
		}
	}
	contracts := statusMeta.Contracts
	if len(contracts) <= 0 {
		return nil, errors.New("length of contracts is 0")
	}
	ssId := uuid.New().String()
	shardIndexes := make([]int, 0)
	shardHashes := make([]string, 0)
	contractMap := map[string]*guardpb.Contract{}
	for _, contract := range contracts {
		if contract.State == guardpb.Contract_LOST {
			contractMap[contract.ShardHash] = contract
			shardHashes = append(shardHashes, contract.ShardHash)
			shardIndexes = append(shardIndexes, int(contract.ShardIndex))
		}
	}
	rss, err := sessions.GetRenterSession(ctxParams, ssId, repairContract.FileHash, shardHashes)
	if err != nil {
		return nil, err
	}
	hp := uh.GetHostsProvider(ctxParams, make([]string, 0))
	m := contracts[0].ContractMeta
	for index, shardHash := range rss.ShardHashes {
		go func(i int, h string) {
			err := backoff.Retry(func() error {
				select {
				case <-rss.Ctx.Done():
					return nil
				default:
					break
				}
				host, err := hp.NextValidHost(m.Price)
				if err != nil {
					terr := rss.To(sessions.RssToErrorEvent, err)
					if terr != nil {
						log.Debugf("original err: %s, transition err: %s", err.Error(), terr.Error())
					}
					return nil
				}
				guardContract := contractMap[shardHash]
				guardContract.HostPid = host
				guardContract.State = guardpb.Contract_RENEWED
				guardContractBytes, err := proto.Marshal(guardContract)
				if err != nil {
					return nil
				}
				repairPid := ctxParams.N.Identity
				hostPid, err := peer.IDB58Decode(host)
				if err != nil {
					log.Errorf("shard %s decodes host_pid error: %s", h, err.Error())
					return err
				}
				cb := make(chan error)
				ShardErrChanMap.Set(guardContract.ContractId, cb)
				go func() {
					ctx, _ := context.WithTimeout(rss.Ctx, 10*time.Second)
					_, err := remote.P2PCall(ctx, rss.CtxParams.N, rss.CtxParams.Api, hostPid, "/storage/upload/init",
						rss.SsId,
						rss.Hash,
						h,
						m.Price,
						"",
						guardContractBytes,
						-1,
						m.ShardFileSize,
						i,
						repairPid,
					)
					if err != nil {
						cb <- err
					}
				}()
				// host needs to send recv in 30 seconds, or the contract will be invalid.
				tick := time.Tick(30 * time.Second)
				select {
				case err = <-cb:
					ShardErrChanMap.Remove(guardContract.ContractId)
					return err
				case <-tick:
					return errors.New("host timeout")
				}
			}, uh.HandleShardBo)
			if err != nil {
				_ = rss.To(sessions.RssToErrorEvent,
					errors.New("timeout: failed to setup contract in "+uh.HandleShardBo.MaxElapsedTime.String()))
			}
		}(shardIndexes[index], shardHash)
	}
	updateRepairContract(contracts, contractMap)
	return statusMeta, nil
}

func updateRepairContract(contracts []*guardpb.Contract, contractMap map[string]*guardpb.Contract) {
	for _, contract := range contracts {
		if contract.State == guardpb.Contract_LOST {
			if v, ok := contractMap[contract.ShardHash]; ok {
				contract = v
				delete(contractMap, contract.ShardHash)
			}
		}
	}
}

func downloadAndRebuildFile(ctxParams *uh.ContextParams, fileHash string, rs []string) ([]byte, error) {
	url := fmt.Sprint(strings.Split(ctxParams.Cfg.Addresses.API[0], "/")[2], ":", strings.Split(ctxParams.Cfg.Addresses.API[0], "/")[4])
	resp, err := shell.NewShell(url).Request("get", fileHash).Option("rs", rs).Send(ctxParams.Ctx)
	if err != nil {
		return nil, err
	}
	defer resp.Close()

	if resp.Error != nil {
		return nil, resp.Error
	}
	return ioutil.ReadAll(resp.Output)
}

func challengeLostShards(ctxParams *uh.ContextParams, repairReq *guardpb.RepairContract) error {
	errChan := make(chan error, len(repairReq.LostShardHash))
	for _, lostShardHash := range repairReq.LostShardHash {
		go func() {
			err := func() error {
				err := challengeShard(ctxParams, lostShardHash, true, &guardpb.ContractMeta{
					//RenterPid:   repairReq.RepairPid,
					FileHash:   repairReq.FileHash,
					ShardHash:  lostShardHash,
					ContractId: repairReq.RepairContractId,
					HostPid:    repairReq.RepairPid,
					RentStart:  repairReq.RepairSignTime,
				})
				if err != nil {
					return err
				}
				return nil
			}()
			errChan <- err
		}()
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
	return nil
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
	if DownloadRewardAmount <= 0 {
		return fmt.Errorf("download reward amount should be bigger than 0")
	}
	if RepairRewardAmount <= 0 {
		return fmt.Errorf("repair reward amount should be bigger than 0")
	}
	return nil
}
