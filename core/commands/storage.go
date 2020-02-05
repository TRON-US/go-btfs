package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/core/escrow"
	"github.com/TRON-US/go-btfs/core/guard"
	"github.com/TRON-US/go-btfs/core/hub"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/TRON-US/interface-go-btfs-core/path"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	hubpb "github.com/tron-us/go-btfs-common/protos/hub"
	ledgerpb "github.com/tron-us/go-btfs-common/protos/ledger"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"

	"github.com/Workiva/go-datastructures/set"
	"github.com/alecthomas/units"
	"github.com/gogo/protobuf/proto"
	cidlib "github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	leafHashOptionName            = "leaf-hash"
	uploadPriceOptionName         = "price"
	replicationFactorOptionName   = "replication-factor"
	hostSelectModeOptionName      = "host-select-mode"
	hostSelectionOptionName       = "host-selection"
	hostInfoModeOptionName        = "host-info-mode"
	hostSyncModeOptionName        = "host-sync-mode"
	hostStoragePriceOptionName    = "host-storage-price"
	hostBandwidthPriceOptionName  = "host-bandwidth-price"
	hostCollateralPriceOptionName = "host-collateral-price"
	hostBandwidthLimitOptionName  = "host-bandwidth-limit"
	hostStorageTimeMinOptionName  = "host-storage-time-min"
	hostStorageMaxOptionName      = "host-storage-max"
	testOnlyOptionName            = "host-search-local"
	storageLengthOptionName       = "storage-length"
	repairModeOptionName          = "repair-mode"
	offlinesignModeOptionName     = "offline-sign-mode"

	defaultRepFactor     = 3
	defaultStorageLength = 30

	// retry limit
	RetryLimit = 3
	FailLimit  = 3

	bttTotalSupply uint64 = 990_000_000_000
)

const (
	regularMode = iota
	customMode
	repairMode
	offlineSignMode
)

// TODO: get/set the value from/in go-btfs-common
var HostPriceLowBoundary = int64(10)

var StorageCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Interact with storage services on BTFS.",
		ShortDescription: `
Storage services include client upload operations, host storage operations,
host information sync/display operations, and BTT payment-related routines.`,
	},
	Subcommands: map[string]*cmds.Command{
		"upload":    storageUploadCmd,
		"hosts":     storageHostsCmd,
		"info":      storageInfoCmd,
		"announce":  storageAnnounceCmd,
		"challenge": storageChallengeCmd,
	},
}

var storageUploadCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Store files on BTFS network nodes through BTT payment.",
		ShortDescription: `
By default, BTFS selects hosts based on overall score according to the current client's environment.
To upload a file, <file-hash> must refer to a reed-solomon encoded file.

To create a reed-solomon encoded file from a normal file:

    $ btfs add --chunker=reed-solomon <file>
    added <file-hash> <file>

Run command to upload:

    $ btfs storage upload <file-hash>

To custom upload and store a file on specific hosts:
    Use -m with 'custom' mode, and put host identifiers in -s, with multiple hosts separated by ','.

    # Upload a file to a set of hosts
    # Total # of hosts must match # of shards in the first DAG level of root file hash
    $ btfs storage upload <file-hash> -m=custom -s=<host_address1>,<host_address2>

    # Upload specific shards to a set of hosts
    # Total # of hosts must match # of shards given
    $ btfs storage upload <shard-hash1> <shard-hash2> -l -m=custom -s=<host_address1>,<host_address2>

Use status command to check for completion:
    $ btfs storage upload status <session-id> | jq`,
	},
	Subcommands: map[string]*cmds.Command{
		"init":             storageUploadInitCmd,
		"recvcontract":     storageUploadRecvContractCmd,
		"status":           storageUploadStatusCmd,
		"getcontractbatch": storageUploadGetContractBatchCmd,
		"signbatch":        storageUploadSignbatchCmd,
		"getunsigned":      storageUploadGetUnsignedCmd,
		"sign":             storageUploadSignCmd,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, false, "Add hash of file to upload."),
		cmds.StringArg("repair-shards", false, false, "Shard hashes to repair."),
		cmds.StringArg("renter-pid", false, false, "Original renter pid."),
		cmds.StringArg("blacklist", false, false, "Blacklist of hosts when upload."),
		cmds.StringArg("offline-peerid", false, false, "Peer id when offline upload."),
		cmds.StringArg("offline-nouncets", false, false, "Nounce timestamp when offline upload."),
		cmds.StringArg("offline-signature", false, false, "Session signature when offline upload."),
	},
	Options: []cmds.Option{
		cmds.BoolOption(leafHashOptionName, "l", "Flag to specify given hash(es) is leaf hash(es).").WithDefault(false),
		cmds.Int64Option(uploadPriceOptionName, "p", "Max price per GiB per day of storage in BTT."),
		cmds.IntOption(replicationFactorOptionName, "r", "Replication factor for the file with erasure coding built-in.").WithDefault(defaultRepFactor),
		cmds.StringOption(hostSelectModeOptionName, "m", "Based on mode to select the host and upload automatically."),
		cmds.StringOption(hostSelectionOptionName, "s", "Use only these selected hosts in order on 'custom' mode. Use ',' as delimiter."),
		cmds.BoolOption(testOnlyOptionName, "t", "Enable host search under all domains 0.0.0.0 (useful for local test)."),
		cmds.IntOption(storageLengthOptionName, "len", "Store file for certain length in days.").WithDefault(defaultStorageLength),
		cmds.BoolOption(repairModeOptionName, "repair mode").WithDefault(false),
		cmds.BoolOption(offlinesignModeOptionName, "offline sign mode").WithDefault(false),
	},
	RunTimeout: 15 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		// get config settings
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}
		// get node
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		// get core api
		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}
		// check file hash
		var (
			shardHashes         []string
			rootHash            cidlib.Cid
			shardSize           uint64
			blacklist           = set.New()
			renterPid           = n.Identity
			offPeerId           peer.ID
			offNonceTimestamp   uint64
			offSessionSignature string
		)
		var runMode int
		if isRepairMode := req.Options[repairModeOptionName].(bool); isRepairMode {
			runMode = repairMode
		}

		lf := req.Options[leafHashOptionName].(bool)
		if isOfflineSign := req.Options[offlinesignModeOptionName].(bool); isOfflineSign {
			runMode = offlineSignMode
		}
		if runMode == repairMode {
			if len(req.Arguments) > 3 {
				blacklistStr := req.Arguments[3]
				for _, s := range strings.Split(blacklistStr, ",") {
					blacklist.Add(s)
				}
			}
			rootHash, err = cidlib.Parse(req.Arguments[0])
			if err != nil {
				return err
			}
			renterPid, err = peer.IDB58Decode(req.Arguments[2])
			if err != nil {
				return err
			}
			shardHashes = strings.Split(req.Arguments[1], ",")
			for _, h := range shardHashes {
				_, err := cidlib.Parse(h)
				if err != nil {
					return err
				}
			}
			shardCid, err := cidlib.Parse(shardHashes[0])
			if err != nil {
				return err
			}
			shardSize, err = getContractSizeFromCid(req.Context, shardCid, api)
			if err != nil {
				return err
			}
		} else if runMode == offlineSignMode {
			if len(req.Arguments) != 4 {
				return fmt.Errorf("need file hash, offline-peer-id, offline-nounce-timestamp, and session-signature")
			}
			hashStr := req.Arguments[0]
			rootHash, err = cidlib.Parse(hashStr)
			if err != nil {
				return err
			}
			hashes, err := storage.CheckAndGetReedSolomonShardHashes(req.Context, n, api, rootHash)
			if err != nil || len(hashes) == 0 {
				return fmt.Errorf("invalid hash: %s", err)
			}
			// get shard size
			shardSize, err = getContractSizeFromCid(req.Context, hashes[0], api)
			if err != nil {
				return err
			}
			for _, h := range hashes {
				shardHashes = append(shardHashes, h.String())
			}
			offPeerId, err = peer.IDB58Decode(req.Arguments[1])
			if err != nil {
				return err
			}
			offNonceTimestamp, err = strconv.ParseUint(req.Arguments[2], 10, 64)
			if err != nil {
				return err
			} else {
				if offNonceTimestamp >= math.MaxUint64 {
					return fmt.Errorf("nounce timestamp should be smaller than max of uint64")
				}
			}
			offSessionSignature = req.Arguments[3]
		} else if !lf {
			if len(req.Arguments) != 1 {
				return fmt.Errorf("need one and only one root file hash")
			}
			rootHash, err = cidlib.Parse(req.Arguments[0])
			if err != nil {
				return err
			}
			hashes, err := storage.CheckAndGetReedSolomonShardHashes(req.Context, n, api, rootHash)
			if err != nil || len(hashes) == 0 {
				return fmt.Errorf("invalid hash: %s", err)
			}
			// get shard size
			shardSize, err = getContractSizeFromCid(req.Context, hashes[0], api)
			if err != nil {
				return err
			}
			for _, h := range hashes {
				shardHashes = append(shardHashes, h.String())
			}
		} else {
			rootHash = cidlib.Undef
			shardHashes = req.Arguments
			shardCid, err := cidlib.Parse(shardHashes[0])
			if err != nil {
				return err
			}
			shardSize, err = getContractSizeFromCid(req.Context, shardCid, api)
			if err != nil {
				return err
			}
		}
		// start new session
		// TODO: Genereate session ID on new
		sm := storage.GlobalSession
		ssID, err := storage.NewSessionID()
		// start new session
		if err != nil {
			return err
		}
		ss := sm.GetOrDefault(ssID, n.Identity)
		ss.SetFileHash(rootHash)
		ss.SetRunMode(runMode)
		if runMode == offlineSignMode {
			ss.OfflineCB = new(storage.OfflineControlBlock)
			ss.SetStatus(storage.UninitializedStatus)
			initOfflineSignChannels(ss)
		} else {
			ss.SetStatus(storage.InitStatus)
		}
		go controlSessionTimeout(ss, storage.StdSessionStateFlow[0:])

		// get hosts/peers
		var peers []string
		// init retry queue
		retryQueue := storage.NewRetryQueue(int64(len(peers)))
		// set price limit, the price is default when host doesn't provide price
		price, found := req.Options[uploadPriceOptionName].(int64)
		if found && price < HostPriceLowBoundary {
			return fmt.Errorf("price is smaller than minimum setting price")
		}
		if found && price >= math.MaxInt64 {
			return fmt.Errorf("price should be smaller than max int64")
		}
		ns, err := hub.GetSettings(req.Context, cfg.Services.HubDomain,
			n.Identity.String(), n.Repo.Datastore())
		if err != nil {
			return err
		}
		if !found {
			price = int64(ns.StoragePriceAsk)
		}
		mode, ok := req.Options[hostSelectModeOptionName].(string)
		if ok && mode == "custom" {
			// get host list as user specified
			hosts, found := req.Options[hostSelectionOptionName].(string)
			if !found {
				return fmt.Errorf("custom mode needs input host lists")
			}
			peers = strings.Split(hosts, ",")
			if len(peers) != len(shardHashes) {
				return fmt.Errorf("custom mode hosts length must match shard hashes length")
			}
			for _, ip := range peers {
				host := &storage.HostNode{
					Identity:   ip,
					RetryTimes: 0,
					FailTimes:  0,
					Price:      price,
				}
				if err := retryQueue.Offer(host); err != nil {
					return err
				}
			}
		} else {
			// Use default setting if not set
			if !ok {
				mode = cfg.Experimental.HostsSyncMode
			}
			hosts, err := storage.GetHostsFromDatastore(req.Context, n, mode, len(shardHashes))
			if err != nil {
				return err
			}
			for _, ni := range hosts {
				if blacklist.Exists(ni) {
					continue
				}
				// use host askingPrice instead if provided
				if int64(ni.StoragePriceAsk) > HostPriceLowBoundary {
					price = int64(ni.StoragePriceAsk)
				}
				// add host to retry queue
				host := &storage.HostNode{
					Identity:   ni.NodeId,
					RetryTimes: 0,
					FailTimes:  0,
					Price:      price,
				}
				if err := retryQueue.Offer(host); err != nil {
					return err
				}
			}
		}
		storageLength := req.Options[storageLengthOptionName].(int)
		if uint64(storageLength) < ns.StorageTimeMin {
			return fmt.Errorf("invalid storage len. want: >= %d, got: %d",
				ns.StorageTimeMin, storageLength)
		}

		// retry queue need to be reused in proof cmd
		ss.SetRetryQueue(retryQueue)

		// set offline info items
		if runMode == offlineSignMode {
			ss.SetFOfflinePeerID(offPeerId)
			ss.SetFOfflineNonceTimestamp(offNonceTimestamp)
			ss.SetFOfflineSessionSignature(offSessionSignature)
		}

		// add shards into session
		for shardIndex, shardHash := range shardHashes {
			_, err := ss.GetOrDefault(shardHash, shardIndex, int64(shardSize), int64(storageLength), "")
			if err != nil {
				return err
			}
		}
		// set to false if not specified
		testFlag := false
		if req.Options[testOnlyOptionName] != nil {
			testFlag = req.Options[testOnlyOptionName].(bool)
		}

		if runMode == offlineSignMode {
			go retryMonitorOffSign(cfg, context.Background(), api, ss, n, ssID, testFlag, runMode, renterPid.Pretty())
		} else {
			go retryMonitor(context.Background(), api, ss, n, ssID, testFlag, runMode, renterPid.Pretty())
		}

		seRes := &UploadRes{
			ID: ssID,
		}
		return res.Emit(seRes)
	},
	Type: UploadRes{},
}

func getShardInfo(req *cmds.Request, api coreiface.CoreAPI, n *core.IpfsNode, hashStr string) ([]string, uint64, error) {
	var shardHashes []string
	var shardSize uint64
	// convert to cid
	rootHash, err := cidlib.Parse(hashStr)
	if err != nil {
		return nil, 0, err
	}
	hashes, err := storage.CheckAndGetReedSolomonShardHashes(req.Context, n, api, rootHash)
	if err != nil || len(hashes) == 0 {
		return nil, 0, fmt.Errorf("invalid hash: %s", err)
	}
	// get shard size
	shardSize, err = getContractSizeFromCid(req.Context, hashes[0], api)
	if err != nil {
		return nil, 0, err
	}
	for _, h := range hashes {
		shardHashes = append(shardHashes, h.String())
	}
	return shardHashes, shardSize, nil
}

func getContractSizeFromCid(ctx context.Context, hash cidlib.Cid, api coreiface.CoreAPI) (uint64, error) {
	leafPath := path.IpfsPath(hash)
	ipldNode, err := api.ResolveNode(ctx, leafPath)
	if err != nil {
		return 0, err
	}
	return ipldNode.Size()
}

func controlSessionTimeout(ss *storage.FileContracts, stateFlow []*storage.FlowControl) {
	curStatus := 0  // 0 == storage.Uninitialized
	LOOP:
	for {
		select {
		case sessionState := <-ss.SessionStatusChan:
			if sessionState.Succeed {
				curStatus = moveToNextSessionStatus(ss.RunMode, curStatus, sessionState)
				if curStatus > len(stateFlow) {
					panic("stateFlow array access is not correct")
				}
				ss.SetStatus(curStatus)
				if curStatus == storage.CompleteStatus {
					break LOOP
				}
			} else {
				if sessionState.Err == nil {
					sessionState.Err = fmt.Errorf("unknown error, please file a bug report")
				}
				ss.SetStatusWithError(storage.ErrStatus, sessionState.Err)
				return
			}
		case <-time.After(stateFlow[curStatus].TimeOut):
			ss.SetStatusWithError(storage.ErrStatus, fmt.Errorf("timed out"))
			return
		}
	}
}

func moveToNextSessionStatus(runMode int, currStatus int, sessionState storage.StatusChan) (int) {
	if runMode != offlineSignMode {
		// If the current runMode is not offlineSignMode, then
		// we need to skip offline sign mode related steps from the session status table.
		// The following switch is doing that task.
		switch sessionState.CurrentStep {
		case storage.InitStatus:
			return storage.SubmitStatus
		case storage.SubmitStatus:
			return storage.PayStatus
		case storage.PayStatus:
			return storage.GuardStatus
		case storage.GuardStatus:
			fmt.Println("complete")
			return storage.CompleteStatus
		}
	}
	return sessionState.CurrentStep + 1
}

// retryMonitor creates "retryQueue", and
// loops over each shard: By preparing signed contracts, and
// running each anonymous goroutine per Shard/Host to send
// the contracts and set step status channel (i.e., Shard.RetryChan).
func retryMonitor(ctx context.Context, api coreiface.CoreAPI, ss *storage.FileContracts, n *core.IpfsNode,
	ssID string, test bool, runMode int, renterPid string) {

	retryQueue := ss.GetRetryQueue()
	if retryQueue == nil {
		log.Error("retry queue is nil")
		return
	}

	// loop over each shard
	for shardKey, shard := range ss.ShardInfo {
		go func(shardKey string, shard *storage.Shard) {
			shardIndex := shard.ShardIndex
			candidateHost, err := getValidHost(ctx, retryQueue, api, n, test)
			if err != nil {
				return
			}

			shard.SetPrice(candidateHost.Price)
			cfg, err := n.Repo.Config()
			if err != nil {
				return
			}

			// init escrow Contract
			_, pid, err := ParsePeerParam(candidateHost.Identity)
			if err != nil {
				return
			}
			escrowContract, err := escrow.NewContract(cfg, shard.ContractID, n, pid, shard.TotalPay)
			if err != nil {
				log.Error("create escrow contract failed. ", err)
				return
			}
			halfSignedEscrowContract, err := escrow.SignContractAndMarshal(escrowContract, nil, n.PrivateKey, true)
			if err != nil {
				log.Error("sign escrow contract and marshal failed ")
				return
			}
			shard.UpdateShard(pid)
			guardContractMeta, err := guard.NewContract(ss, cfg, shardKey, int32(shardIndex), renterPid)
			if err != nil {
				log.Error("fail to new contract meta")
				return
			}
			halfSignGuardContract, err := guard.SignedContractAndMarshal(guardContractMeta, nil, n.PrivateKey, true,
				runMode == repairMode, renterPid, n.Identity.Pretty())
			if err != nil {
				log.Error("fail to sign guard contract and marshal")
				return
			}
			// build connection with host, init step, could be error or timeout
			go retryProcess(ctx, n, api, candidateHost, ss, shard,
				halfSignedEscrowContract, halfSignGuardContract, test)

			// monitor each steps if error or time out happens, retry
			// TODO: Change steps
			for curState := 0; curState < len(storage.StdStateFlow); {
				select {
				case shardRes := <-shard.RetryChan:
					if !shardRes.Succeed {
						// receiving session time out signal, directly return
						if shardRes.SessionTimeOutErr != nil {
							log.Error(shardRes.SessionTimeOutErr)
							return
						}
						// if client itself has some error, no matter how many times it tries,
						// it will fail again, in this case, we don't need retry
						if shardRes.ClientErr != nil {
							sendSessionStatusChan(ss.SessionStatusChan, storage.InitStatus, false, shardRes.ClientErr)
							return
						}
						// if host error, retry
						if shardRes.HostErr != nil {
							log.Error(shardRes.HostErr)
							// increment current host's retry times
							candidateHost.IncrementRetry()
							// if reach retry limit, in retry process will select another host
							// so in channel receiving should also return to 'init'
							curState = storage.InitState
							go retryProcess(ctx, n, api, candidateHost, ss, shard,
								halfSignedEscrowContract, halfSignGuardContract, test)
						}
					} else {
						// if success with current state, move on to next
						log.Debug("succeed to pass state ", storage.StdStateFlow[curState].State)
						curState = shardRes.CurrentStep + 1
						if curState <= storage.CompleteState {
							shard.SetState(curState)
						}
					}
				case <-time.After(storage.StdStateFlow[curState].TimeOut):
					{
						log.Errorf("upload timed out %s with state %s", shardKey, storage.StdStateFlow[curState].State)
						curState = storage.InitState // reconnect to the host to start over
						candidateHost.IncrementRetry()
						go retryProcess(ctx, n, api, candidateHost, ss, shard,
							halfSignedEscrowContract, halfSignGuardContract, test)
					}
				}
			}
		}(shardKey, shard)
	}
}

type paramsForPrepareContractsForShard struct {
	ctx       context.Context
	rq        *storage.RetryQueue
	api       coreiface.CoreAPI
	ss        *storage.FileContracts
	n         *core.IpfsNode
	test      bool
	runMode   int
	renterPid string
	shardKey  string
	shard     *storage.Shard
}

// prepareSignedContractsForShard, for online signing, computes and sets, as OUTPUT,
// the given "param.shard"s entries: CandidateHost, HalfSignedEscrowContract, HalfSignGuardContract.
func prepareSignedContractsForShard(param *paramsForPrepareContractsForShard) (error) {
	shard := param.shard
	shardIndex := shard.ShardIndex
	candidateHost, err := getValidHost(param.ctx, param.rq, param.api, param.n, param.test)
	if err != nil {
		return err
	}

	shard.SetPrice(candidateHost.Price)
	cfg, err := param.n.Repo.Config()
	if err != nil {
		return err
	}

	// init escrow Contract
	_, pid, err := ParsePeerParam(candidateHost.Identity)
	if err != nil {
		return err
	}
	escrowContract, err := escrow.NewContract(cfg, shard.ContractID, param.n, pid, shard.TotalPay)
	if err != nil {
		log.Error("create escrow contract failed. ", err)
		return err
	}
	halfSignedEscrowContract, err := escrow.SignContractAndMarshal(escrowContract, nil, param.n.PrivateKey, true)
	if err != nil {
		log.Error("sign escrow contract and maorshal failed ")
		return err
	}
	shard.UpdateShard(pid)
	guardContractMeta, err := guard.NewContract(param.ss, cfg, param.shardKey, int32(shardIndex), param.renterPid)
	if err != nil {
		log.Error("fail to new contract meta")
		return err
	}
	halfSignGuardContract, err := guard.SignedContractAndMarshal(guardContractMeta, nil, param.n.PrivateKey, true,
		param.runMode == repairMode, param.renterPid, param.n.Identity.Pretty())
	if err != nil {
		log.Error("fail to sign guard contract and marshal")
		return err
	}

	// Output for this function is set here
	shard.CandidateHost = candidateHost
	shard.HalfSignedEscrowContract = halfSignedEscrowContract
	shard.HalfSignedGuardContract = halfSignGuardContract

	return nil
}


// retryMonitorOffSign creates "retryQueue", and
// loops over each shard: By preparing unsigned contracts, and
// running each anonymous goroutine per Shard/Host
// to do offline signing based on session status with `OfflineSignChan` for communication,
// and to send the contracts setting step status channel (i.e., Shard.RetryChan).
// The current implementation covers the scenario that shard to host mapping is changed by retryProcessOffSign(),
// in other words, host changes for a shard due to an error, for example, connecting to the host or init api fails.
func retryMonitorOffSign(configuration *config.Config, ctx context.Context, api coreiface.CoreAPI, ss *storage.FileContracts, n *core.IpfsNode,
	ssID string, test bool, runMode int, renterPid string) {

	if runMode != offlineSignMode {
		log.Error("Unexpected runMode parameter value.")
		return
	}

	retryQueue := ss.GetRetryQueue()
	if retryQueue == nil {
		log.Error("retry queue is nil")
		return
	}

	// loop over each shard
	for shardKey, shard := range ss.ShardInfo {
		go func(shardKey string, shard *storage.Shard) {
			candidateHost, err := getValidHost(ctx, retryQueue, api, n, test)
			if err != nil {
				return
			}
			shard.CandidateHost = candidateHost
			param := &paramsForPrepareContractsForShard{
				ctx:       ctx,
				rq:        retryQueue,
				api:       api,
				ss:        ss,
				n:         n,
				test:      test,
				runMode:   runMode,
				renterPid: renterPid,
				shardKey:  shardKey,
				shard: shard,
			}

			err = prepareSignedContractsForEscrowOffSign(param, false)
			if err != nil {
				return
			}

			err = prepareSignedGuardContractForShardOffSign(ss, shard, n, false)
			if err != nil {
				return
			}

			err = buildSignedGuardContractForShardOffSign(ss, shard, n, runMode, renterPid, false)
			if err != nil {
				return
			}

			// build connection with host, init step, could be error or timeout
			go retryProcessOffSign(configuration, param, candidateHost)

			// monitor each shard state if error or time out happens, retry
			for curState := storage.OfflineInitState; curState < len(storage.StdOffStateFlow); {
				select {
				case shardRes := <-shard.RetryChan:
					if !shardRes.Succeed {
						// receiving session time out signal, directly return
						if shardRes.SessionTimeOutErr != nil {
							log.Error(shardRes.SessionTimeOutErr)
							return
						}
						// if client itself has some error, no matter how many times it tries,
						// it will fail again, in this case, we don't need retry
						if shardRes.ClientErr != nil {
							log.Error(shardRes.ClientErr)
							sendSessionStatusChan(ss.SessionStatusChan, storage.OfflineInitState, false, shardRes.ClientErr)
							return
						}
						// if host error, retry
						if shardRes.HostErr != nil {
							log.Error(shardRes.HostErr)
							// increment current host's retry times
							candidateHost.IncrementRetry()
							// if reach retry limit, in retry process will select another host
							// so in channel receiving should also return to 'init'
							curState = storage.OfflineInitState
							go retryProcessOffSign(configuration, param, candidateHost)
						}
					} else {
						// if success with current state, move on to next
						log.Debug("succeed to pass state ", storage.StdOffStateFlow[curState].State)
						curState = shardRes.CurrentStep + 1
						if curState <= storage.OfflineCompleteState {
							shard.SetState(curState)
						}
					}
				case <-time.After(storage.StdOffStateFlow[curState].TimeOut):
					{
						log.Errorf("upload timed out %s with state %s", shardKey, storage.StdOffStateFlow[curState].State)
						curState = storage.OfflineInitState // reconnect to the host to start over
						candidateHost.IncrementRetry()
						go retryProcessOffSign(configuration, param, candidateHost)
					}
				}
			}
		}(shardKey, shard)
	}
}

func sendSessionStatusChan(channel chan storage.StatusChan, status int, succeed bool, err error) {
	if err != nil {
		log.Error("session error:", err)
	}
	channel <- storage.StatusChan{
		CurrentStep: status,
		Succeed:     succeed,
		Err:         err,
	}
}

func sendStepStateChan(channel chan *storage.StepRetryChan, state int, succeed bool, clientErr error, hostErr error) {
	if clientErr != nil {
		log.Error("renter error:", clientErr)
	}
	if hostErr != nil {
		log.Error("host error:", hostErr)
	}
	channel <- &storage.StepRetryChan{
		CurrentStep: state,
		Succeed:     succeed,
		ClientErr:   clientErr,
		HostErr:     hostErr,
	}
}

func retryProcess(ctx context.Context, n *core.IpfsNode, api coreiface.CoreAPI, candidateHost *storage.HostNode,
	ss *storage.FileContracts, shard *storage.Shard,
	halfSignedEscrowContract, halfSignedGuardContract []byte,
	test bool) {
	// check if current shard has been contacting and receiving results
	if shard.GetState() >= storage.ContractState {
		return
	}
	// if candidate host passed in is not valid, fetch next valid one
	if candidateHost == nil || candidateHost.FailTimes >= FailLimit || candidateHost.RetryTimes >= RetryLimit {
		otherValidHost, err := getValidHost(ctx, ss.RetryQueue, api, n, test)
		// either retry queue is empty or something wrong with retry queue
		if err != nil {
			sendStepStateChan(shard.RetryChan, storage.InitState, false, fmt.Errorf("no host available %v", err), nil)
			return
		}
		candidateHost = otherValidHost
	}

	// parse candidate host's IP and get connected
	_, hostPid, err := ParsePeerParam(candidateHost.Identity)
	if err != nil {
		sendStepStateChan(shard.RetryChan, storage.InitState, false, err, nil)
		return
	}
	// send over contract
	c := new(guardpb.Contract)
	err = proto.Unmarshal(halfSignedGuardContract, c)
	if err != nil {
		return
	}
	_, err = remote.P2PCall(ctx, n, hostPid, "/storage/upload/init",
		ss.ID,
		ss.GetFileHash().String(),
		shard.ShardHash.String(),
		strconv.FormatInt(candidateHost.Price, 10),
		halfSignedEscrowContract,
		halfSignedGuardContract,
		strconv.FormatInt(shard.StorageLength, 10),
		strconv.FormatInt(shard.ShardSize, 10),
		strconv.Itoa(shard.ShardIndex),
	)
	// fail to connect with retry
	if err != nil {
		sendStepStateChan(shard.RetryChan, storage.InitState, false, nil, err)
	} else {
		sendStepStateChan(shard.RetryChan, storage.InitState, true, nil, nil)
	}
}

// retryProcessOffSign() behaves differently based on whether the host for shard is
// changed inside this function regarding offline signing shard state transitions: i.e.,
// it goes back to 0 if changed. Otherwise starts from 5(OfflineInitState).
// If 0, retryProcessOffSign() performs init-sign.
// TODO: check the possibility of a race condition like multiple instances for a shard. If yes, shard.Lock()/shard.Unlock()
func retryProcessOffSign(configuration *config.Config, param *paramsForPrepareContractsForShard, candidateHost *storage.HostNode) {
	ss := param.ss
	shard := param.shard

	status := ss.GetCurrentStatus()
	if status == storage.ErrStatus {
		return
	}

	// check if current shard has been contacting and receiving results
	if param.shard.GetState() >= storage.OfflineContractState {
		return
	}
	// if candidate host passed in is not valid, fetch next valid one
	var hostChanged bool
	if candidateHost == nil || candidateHost.FailTimes >= FailLimit || candidateHost.RetryTimes >= RetryLimit {
		otherValidHost, err := getValidHost(param.ctx, ss.RetryQueue, param.api, param.n, param.test)
		// either retry queue is empty or something wrong with retry queue
		if err != nil {
			sendStepStateChan(shard.RetryChan, storage.InitState, false, fmt.Errorf("no host available %v", err), nil)
			return
		}
		candidateHost = otherValidHost
		hostChanged = true
	}

	// If host is changed for the "shard", move the shard state to 0 and retry
	if hostChanged  {
		param.shard.CandidateHost = candidateHost

		sendStepStateChan(shard.RetryChan, storage.OfflineUninitializedState, true, nil, nil)

		// sendStepStateChan(shard.RetryChan, storage.InitState
		err := prepareSignedContractsForEscrowOffSign(param, true)
		if err != nil {
			return
		}

		err = prepareSignedGuardContractForShardOffSign(ss, shard, param.n, true)
		if err != nil {
			return
		}

		err = buildSignedGuardContractForShardOffSign(ss, shard, param.n, param.runMode, param.renterPid, true)
		if err != nil {
			return
		}
	}

	// parse candidate host's IP and get connected
	_, hostPid, err := ParsePeerParam(candidateHost.Identity)
	if err != nil {
		sendStepStateChan(shard.RetryChan, storage.InitState, false, err, nil)
		return
	}
	// send over contract
	c := new(guardpb.Contract)
	err = proto.Unmarshal(shard.HalfSignedGuardContract, c)
	if err != nil {
		return
	}
	// temporary code
	/*
	privKey, err := configuration.Identity.DecodePrivateKey("")
	if err != nil {
		return
	}
	shard.HalfSignedEscrowContract, err = crypto.Sign(privKey, shard.UnsignedEscrowContract)
	if err != nil {
		return
	}
	shard.HalfSignedGuardContract, err = crypto.Sign(privKey, shard.UnsignedGuardContract)
	if err != nil {
		return
	}
	 */
	_, err = remote.P2PCall(param.ctx, param.n, hostPid, "/storage/upload/init",
		ss.ID,
		ss.GetFileHash().String(),
		shard.ShardHash.String(),
		strconv.FormatInt(candidateHost.Price, 10),
		shard.HalfSignedEscrowContract,
		shard.HalfSignedGuardContract,
		strconv.FormatInt(shard.StorageLength, 10),
		strconv.FormatInt(shard.ShardSize, 10),
		strconv.Itoa(shard.ShardIndex),
	)
	// fail to connect with retry
	if err != nil {
		sendStepStateChan(shard.RetryChan, storage.InitState, false, nil, err)
	} else {
		sendStepStateChan(shard.RetryChan, storage.InitState, true, nil, nil)
	}
}

// find next available host in a forever loop.
// Assumption is that the given retryQueue has enough hosts so as for the caller
// to get a host everytime.
func getValidHost(ctx context.Context, retryQueue *storage.RetryQueue, api coreiface.CoreAPI, n *core.IpfsNode, test bool) (*storage.HostNode, error) {
	var candidateHost *storage.HostNode
	for candidateHost == nil {
		if retryQueue.Empty() {
			return nil, fmt.Errorf("retry queue is empty")
		}
		nextHost, err := retryQueue.Poll()
		if err != nil {
			return nil, err
		} else if nextHost.FailTimes >= FailLimit {
			log.Info("Remove Host: ", nextHost)
		} else if nextHost.RetryTimes >= RetryLimit {
			nextHost.IncrementFail()
			err = retryQueue.Offer(nextHost)
			if err != nil {
				return nil, err
			}
		} else {
			// find peer
			pi, err := remote.FindPeer(ctx, n, nextHost.Identity)
			if err != nil {
				// it's normal to fail in finding peer,
				// would give host another chance
				nextHost.IncrementFail()
				log.Error(err, "host", nextHost.Identity)
				err = retryQueue.Offer(nextHost)
				if err != nil {
					return nil, err
				}
				continue
			}
			addr, err := ma.NewMultiaddr("/p2p-circuit/btfs/" + pi.ID.String())
			if err != nil {
				return nil, err
			}
			pi.Addrs = append(pi.Addrs, addr)
			if err := api.Swarm().Connect(ctx, *pi); err != nil {
				// force connect again
				if err := api.Swarm().Connect(ctx, *pi); err != nil {
					var errTest error
					// in test mode, change the ip to 0.0.0.0
					if test {
						if errTest = changeAddress(pi); errTest == nil {
							errTest = api.Swarm().Connect(ctx, *pi)
						}
					}
					// err can be from change address in test mode or connect again in test mode
					if errTest != nil {
						log.Error(errTest)
						nextHost.IncrementFail()
						err = retryQueue.Offer(nextHost)
						if err != nil {
							return nil, err
						}
						continue
					}
				}
			}
			// if connect successfully, return
			candidateHost = nextHost
		}
	}
	return candidateHost, nil
}

func changeAddress(pinfo *peer.AddrInfo) error {
	parts := ma.Split(pinfo.Addrs[0])
	// change address to 0.0.0.0
	newIP, err := ma.NewMultiaddr("/ip4/0.0.0.0")
	if err != nil {
		return err
	}
	parts[0] = newIP
	newMa := ma.Join(parts[0], parts[1])
	pinfo.Addrs[0] = newMa
	return nil
}

// for client to receive all the half signed contracts
var storageUploadRecvContractCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "For renter client to receive half signed contracts.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "Session ID which renter uses to store all shards information."),
		cmds.StringArg("shard-hash", true, false, "Shard the storage node should fetch."),
		cmds.StringArg("shard-index", true, false, "Index of shard within the encoding scheme."),
		cmds.StringArg("escrow-contract", true, false, "Signed Escrow contract."),
		cmds.StringArg("guard-contract", true, false, "Signed Guard contract."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		// receive contracts
		escrowContractBytes := []byte(req.Arguments[3])
		guardContractBytes := []byte(req.Arguments[4])
		ssID := req.Arguments[0]
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		ss, err := storage.GlobalSession.GetSession(n, storage.RenterStoragePrefix, ssID)
		if err != nil {
			return err
		}
		shardHash := req.Arguments[1]
		sidx := req.Arguments[2]
		shardIndex, err := strconv.Atoi(sidx)
		if err != nil {
			return err
		}
		shard, err := ss.GetShard(shardHash, shardIndex)
		if err != nil {
			return err
		}
		sendStepStateChan(shard.RetryChan, storage.ContractState, true, nil, nil)
		guardContract, err := guard.UnmarshalGuardContract(guardContractBytes)
		if err != nil {
			sendStepStateChan(shard.RetryChan, storage.CompleteState, false, err, nil)
			return err
		}
		err = ss.IncrementContract(shard, escrowContractBytes, guardContract)
		if err != nil {
			sendStepStateChan(shard.RetryChan, storage.CompleteState, false, err, nil)
			return err
		}
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			sendStepStateChan(shard.RetryChan, storage.CompleteState, false, err, nil)
			return err
		}
		sendStepStateChan(shard.RetryChan, storage.CompleteState, true, nil, nil)

		var contractRequest *escrowpb.EscrowContractRequest
		if ss.GetCompleteContractNum() == len(ss.ShardInfo) {
			if ss.RunMode != offlineSignMode {
				err := payWithOnlineSign(req, cfg, n, env, ss, contractRequest)
				if err != nil {
					return err
				}
			} else {
				err := payWithOfflineSign(req, cfg, n, env, ss, contractRequest)
				if err != nil {
					return err
				}
			}
		}
		return nil
	},
}

func payWithOnlineSign(req *cmds.Request, cfg *config.Config, n *core.IpfsNode, env cmds.Environment,
	ss *storage.FileContracts, contractRequest *escrowpb.EscrowContractRequest) error {
	// collecting all signed contracts means init status finished
	sendSessionStatusChan(ss.SessionStatusChan, storage.InitStatus, true, nil)
	stepIdx := 0
	fmt.Println("step ", stepIdx)
	stepIdx++
	contracts, totalPrice, err := storage.PrepareContractFromShard(ss.ShardInfo)
	if err != nil {
		sendSessionStatusChan(ss.SessionStatusChan, storage.SubmitStatus, false, err)
		return err
	}
	fmt.Println("step ", stepIdx)
	stepIdx++
	// check account balance, if not enough for the totalPrice do not submit to escrow
	balance, _ := escrow.Balance(req.Context, cfg)
	if err != nil {
		err = fmt.Errorf("get renter account balance failed [%v]", err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.SubmitStatus, false, err)
		return err
	}
	if balance < totalPrice {
		err = fmt.Errorf("not enough balance to submit contract, current balance is [%v]", balance)
		sendSessionStatusChan(ss.SessionStatusChan, storage.SubmitStatus, false, err)
		return err
	}
	fmt.Println("step ", stepIdx)
	stepIdx++
	contractRequest, err = escrow.NewContractRequest(cfg, contracts, totalPrice)
	if err != nil {
		sendSessionStatusChan(ss.SessionStatusChan, storage.SubmitStatus, false, err)
		return err
	}
	fmt.Println("step ", stepIdx)
	stepIdx++
	submitContractRes, err := escrow.SubmitContractToEscrow(req.Context, cfg, contractRequest)
	if err != nil {
		err = fmt.Errorf("failed to submit contracts to escrow: [%v]", err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.SubmitStatus, false, err)
		return err
	}
	fmt.Println("step ", stepIdx)
	stepIdx++
	sendSessionStatusChan(ss.SessionStatusChan, storage.SubmitStatus, true, nil)

	// get core api
	api, err := cmdenv.GetApi(env, req)
	if err != nil {
		sendSessionStatusChan(ss.SessionStatusChan, storage.PayStatus, false, err)
		return err
	}
	fmt.Println("step ", stepIdx)
	stepIdx++
	go payFullToEscrowAndSubmitToGuard(context.Background(), n, api, submitContractRes, cfg, ss)
	return nil
}

func payFullToEscrowAndSubmitToGuard(ctx context.Context, n *core.IpfsNode, api coreiface.CoreAPI,
	response *escrowpb.SignedSubmitContractResult, cfg *config.Config, ss *storage.FileContracts) {
	privKeyStr := cfg.Identity.PrivKey
	payerPrivKey, err := crypto.ToPrivKey(privKeyStr)
	if err != nil {
		sendSessionStatusChan(ss.SessionStatusChan, storage.PayStatus, false, err)
		return
	}
	payerPubKey := payerPrivKey.GetPublic()
	payinRequest, err := escrow.NewPayinRequest(response, payerPubKey, payerPrivKey)
	if err != nil {
		sendSessionStatusChan(ss.SessionStatusChan, storage.PayStatus, false, err)
		return
	}
	payinRes, err := escrow.PayInToEscrow(ctx, cfg, payinRequest)
	if err != nil {
		err = fmt.Errorf("failed to pay in to escrow: [%v]", err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.PayStatus, false, err)
		return
	}
	sendSessionStatusChan(ss.SessionStatusChan, storage.PayStatus, true, nil)
	fsStatus, err := guard.PrepAndUploadFileMeta(ctx, ss, response, payinRes, payerPrivKey, cfg)
	if err != nil {
		err = fmt.Errorf("failed to send file meta to guard: [%v]", err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.GuardStatus, false, err)
		return
	}
	err = storage.PersistFileMetaToDatastore(n, storage.RenterStoragePrefix, ss.ID)
	if err != nil {
		sendSessionStatusChan(ss.SessionStatusChan, storage.GuardStatus, false, err)
		return
	}
	qs, err := guard.PrepFileChallengeQuestions(ctx, n, api, ss, fsStatus)
	if err != nil {
		sendSessionStatusChan(ss.SessionStatusChan, storage.GuardStatus, false, err)
		return
	}
	err = guard.SendChallengeQuestions(ctx, cfg, ss.FileHash, qs)
	if err != nil {
		err = fmt.Errorf("failed to send challenge questions to guard: [%v]", err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.GuardStatus, false, err)
		return
	}
	sendSessionStatusChan(ss.SessionStatusChan, storage.GuardStatus, true, nil)
}

func payWithOfflineSign(req *cmds.Request, cfg *config.Config, n *core.IpfsNode, env cmds.Environment,
	ss *storage.FileContracts, contractRequest *escrowpb.EscrowContractRequest) error {
	// collecting all signed contracts means init status finished
	ss.NewOfflineUnsigned()
	sendSessionStatusChan(ss.SessionStatusChan, storage.InitStatus, true, nil)
	contracts, totalPrice, err := storage.PrepareContractFromShard(ss.ShardInfo)
	if err != nil {
		sendSessionStatusChan(ss.SessionStatusChan, ss.GetCurrentStatus(), false, err)
		return err
	}
	// check account balance, if not enough for the totalPrice do not submit to escrow
	balance, err := BalanceWithOffSign(req.Context, cfg, ss)
	if err != nil {
		err = fmt.Errorf("get renter account balance failed [%v]", err)
		sendSessionStatusChan(ss.SessionStatusChan, ss.GetCurrentStatus(), false, err)
		return err
	}
	if balance < totalPrice {
		err = fmt.Errorf("not enough balance to submit contract, current balance is [%v]", balance)
		sendSessionStatusChan(ss.SessionStatusChan, ss.GetCurrentStatus(), false, err)
		return err
	}

	contractRequest, err = NewContractRequestWithOffSign(cfg, ss, contracts, totalPrice)
	if err != nil {
		sendSessionStatusChan(ss.SessionStatusChan, ss.GetCurrentStatus(), false, err)
		return err
	}
	submitContractRes, err := escrow.SubmitContractToEscrow(req.Context, cfg, contractRequest)
	if err != nil {
		err = fmt.Errorf("failed to submit contracts to escrow: [%v]", err)
		sendSessionStatusChan(ss.SessionStatusChan, ss.GetCurrentStatus(), false, err)
		return err
	}
	sendSessionStatusChan(ss.SessionStatusChan, storage.PayChannelSignProcessStatus, true, nil)

	// get core api
	api, err := cmdenv.GetApi(env, req)
	if err != nil {
		sendSessionStatusChan(ss.SessionStatusChan, ss.GetCurrentStatus(), false, err)
		return err
	}

	go payFullToEscrowAndSubmitToGuard(context.Background(), n, api, submitContractRes, cfg, ss)
	return nil
}

func payFullToEscrowAndSubmitToGuardOffSign(ctx context.Context, n *core.IpfsNode, api coreiface.CoreAPI,
	response *escrowpb.SignedSubmitContractResult, cfg *config.Config, ss *storage.FileContracts) {
	payinRequest, err := NewPayinRequestOffSign(ss, response)
	if err != nil {
		sendSessionStatusChan(ss.SessionStatusChan, ss.GetCurrentStatus(), false, err)
		return
	}
	payinRes, err := escrow.PayInToEscrow(ctx, cfg, payinRequest)
	if err != nil {
		err = fmt.Errorf("failed to pay in to escrow: [%v]", err)
		sendSessionStatusChan(ss.SessionStatusChan, ss.GetCurrentStatus(), false, err)
		return
	}
	fsStatus, err := PrepAndUploadFileMetaOffSign(ctx, ss, response, payinRes, cfg)
	if err != nil {
		err = fmt.Errorf("failed to send file meta to guard: [%v]", err)
		sendSessionStatusChan(ss.SessionStatusChan, ss.GetCurrentStatus(), false, err)
		return
	}
	sendSessionStatusChan(ss.SessionStatusChan, storage.PayReqSignProcessStatus, true, nil)
	err = storage.PersistFileMetaToDatastore(n, storage.RenterStoragePrefix, ss.ID)
	if err != nil {
		sendSessionStatusChan(ss.SessionStatusChan, ss.GetCurrentStatus(), false, err)
		return
	}
	qs, err := guard.PrepFileChallengeQuestions(ctx, n, api, ss, fsStatus)
	if err != nil {
		sendSessionStatusChan(ss.SessionStatusChan, ss.GetCurrentStatus(), false, err)
		return
	}
	err = guard.SendChallengeQuestions(ctx, cfg, ss.FileHash, qs)
	if err != nil {
		err = fmt.Errorf("failed to send challenge questions to guard: [%v]", err)
		sendSessionStatusChan(ss.SessionStatusChan, ss.GetCurrentStatus(), false, err)
		return
	}
	sendSessionStatusChan(ss.SessionStatusChan, storage.GuardStatus, true, nil)
}

type UploadRes struct {
	ID string
}

var storageUploadInitCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Initialize storage handshake with inquiring client.",
		ShortDescription: `
Storage host opens this endpoint to accept incoming upload/storage requests,
If current host is interested and all validation checks out, host downloads
the shard and replies back to client for the next challenge step.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("file-hash", true, false, "Root file storage node should fetch (the DAG)."),
		cmds.StringArg("shard-hash", true, false, "Shard the storage node should fetch."),
		cmds.StringArg("price", true, false, "Per GiB per day in BTT for storing this shard offered by client."),
		cmds.StringArg("escrow-contract", true, false, "Client's initial escrow contract data."),
		cmds.StringArg("guard-contract-meta", true, false, "Client's initial guard contract meta."),
		cmds.StringArg("storage-length", true, false, "Store file for certain length in days."),
		cmds.StringArg("shard-size", true, false, "Size of each shard received in bytes."),
		cmds.StringArg("shard-index", true, false, "Index of shard within the encoding scheme."),
	},
	RunTimeout: 5 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		// check flags
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}

		ssID := req.Arguments[0]
		fileHash, err := cidlib.Parse(req.Arguments[1])
		if err != nil {
			return err
		}
		shardHash := req.Arguments[2]
		shardIndex, err := strconv.Atoi(req.Arguments[8])
		if err != nil {
			return err
		}
		shardSize, err := strconv.ParseInt(req.Arguments[7], 10, 64)
		if err != nil {
			return err
		}
		price, err := strconv.ParseInt(req.Arguments[3], 10, 64)
		if err != nil {
			return err
		}
		halfSignedEscrowContBytes := req.Arguments[4]
		halfSignedGuardContBytes := req.Arguments[5]
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		settings, err := hub.GetSettings(req.Context, cfg.Services.HubDomain, n.Identity.Pretty(), n.Repo.Datastore())
		if err != nil {
			return err
		}
		if uint64(price) < settings.StoragePriceAsk {
			return fmt.Errorf("price invalid: want: >=%d, got: %d", settings.StoragePriceAsk, price)
		}
		pid, ok := remote.GetStreamRequestRemotePeerID(req, n)
		if !ok {
			return fmt.Errorf("fail to get peer ID from request")
		}
		storeLen, err := strconv.Atoi(req.Arguments[6])
		if err != nil {
			return err
		}
		if uint64(storeLen) < settings.StorageTimeMin {
			return fmt.Errorf("store length invalid: want: >=%d, got: %d", settings.StorageTimeMin, storeLen)
		}
		// build session
		sm := storage.GlobalSession
		ss, _ := sm.GetSession(n, storage.HostStoragePrefix, ssID)
		// TODO: GetOrDefault and GetSession should be one call
		if ss == nil {
			ss = sm.GetOrDefault(ssID, n.Identity)
		}
		ss.SetFileHash(fileHash)
		// TODO: set host shard state in the following steps
		// TODO: maybe extract code on renter step timeout control and reuse it here
		go controlSessionTimeout(ss, storage.StdStateFlow[0:])
		halfSignedGuardContract, err := guard.UnmarshalGuardContract([]byte(halfSignedGuardContBytes))
		shardInfo, err := ss.GetOrDefault(shardHash, shardIndex, shardSize, int64(storeLen), halfSignedGuardContract.ContractId)
		if err != nil {
			return err
		}
		shardInfo.UpdateShard(n.Identity)
		shardInfo.SetPrice(price)

		// review contract and send back to client
		halfSignedEscrowContract, err := escrow.UnmarshalEscrowContract([]byte(halfSignedEscrowContBytes))
		if err != nil {
			return err
		}
		if err != nil {
			return err
		}
		escrowContract := halfSignedEscrowContract.GetContract()
		guardContractMeta := halfSignedGuardContract.ContractMeta
		// get renter's public key
		payerPubKey, err := pid.ExtractPublicKey()
		if err != nil {
			return err
		}
		ok, err = crypto.Verify(payerPubKey, escrowContract, halfSignedEscrowContract.GetBuyerSignature())
		if !ok || err != nil {
			return fmt.Errorf("can't verify escrow contract: %v", err)
		}
		s := halfSignedGuardContract.GetRenterSignature()
		if s == nil {
			s = halfSignedGuardContract.GetPreparerSignature()
		}
		ok, err = crypto.Verify(payerPubKey, &guardContractMeta, s)
		if !ok || err != nil {
			return fmt.Errorf("can't verify guard contract: %v", err)
		}

		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}

		go signContractAndCheckPayment(context.Background(), n, api, cfg, ss, shardInfo, pid,
			halfSignedEscrowContract, halfSignedGuardContract)
		return nil
	},
}

func signContractAndCheckPayment(ctx context.Context, n *core.IpfsNode, api coreiface.CoreAPI, cfg *config.Config,
	ss *storage.FileContracts, shardInfo *storage.Shard, pid peer.ID,
	escrowSignedContract *escrowpb.SignedEscrowContract, guardSignedContract *guardpb.Contract) {
	escrowContract := escrowSignedContract.GetContract()
	guardContractMeta := guardSignedContract.ContractMeta
	// Sign on the contract
	signedEscrowContractBytes, err := escrow.SignContractAndMarshal(escrowContract, escrowSignedContract, n.PrivateKey, false)
	if err != nil {
		log.Error(err)
		return
	}
	signedGuardContractBytes, err := guard.SignedContractAndMarshal(&guardContractMeta, guardSignedContract,
		n.PrivateKey, false, false, pid.Pretty(), pid.Pretty())
	if err != nil {
		log.Error(err)
		return
	}

	_, err = remote.P2PCall(ctx, n, pid, "/storage/upload/recvcontract",
		ss.ID,
		shardInfo.ShardHash.String(),
		strconv.Itoa(shardInfo.ShardIndex),
		signedEscrowContractBytes,
		signedGuardContractBytes,
	)
	if err != nil {
		log.Error(err)
		return
	}

	// set contracts since renter has received contracts
	err = ss.IncrementContract(shardInfo, signedEscrowContractBytes, guardSignedContract)
	if err != nil {
		log.Error(err)
		return
	}

	// persist file meta
	err = storage.PersistFileMetaToDatastore(n, storage.HostStoragePrefix, ss.ID)
	if err != nil {
		log.Error(err)
		return
	}

	// check payment
	signedContractID, err := escrow.SignContractID(escrowContract.ContractId, n.PrivateKey)
	if err != nil {
		log.Error(err)
		return
	}

	paidIn := make(chan bool)
	go checkPaymentFromClient(ctx, paidIn, signedContractID, cfg)
	paid := <-paidIn
	if !paid {
		log.Error("contract is not paid", escrowContract.ContractId)
		return
	}

	downloadShardFromClient(n, api, ss, guardSignedContract, shardInfo)
}

// call escrow service to check if payment is received or not
func checkPaymentFromClient(ctx context.Context, paidIn chan bool, contractID *escrowpb.SignedContractID, configuration *config.Config) {
	timeout := 3 * time.Second
	newCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(300 * time.Millisecond)
	paid := false
	var err error

	for {
		select {
		case t := <-ticker.C:
			log.Debug("Check escrow IsPaidin, tick at", t.UTC())
			paid, err = escrow.IsPaidin(ctx, configuration, contractID)
			if err != nil {
				// too verbose on errors, only err log the last err
				log.Debug("Check escrow IsPaidin error", err)
			}
			if paid {
				paidIn <- true
				return
			}
		case <-newCtx.Done():
			log.Debug("Check escrow IsPaidin timeout, tick stopped at", time.Now().UTC())
			if err != nil {
				log.Error("Check escrow IsPaidin failed", err)
			}
			ticker.Stop()
			paidIn <- paid
			return
		}
	}
}

func downloadShardFromClient(n *core.IpfsNode, api coreiface.CoreAPI, ss *storage.FileContracts,
	guardContract *guardpb.Contract, shardInfo *storage.Shard) {
	expir := uint64(guardContract.RentEnd.Unix())
	// Get + pin to make sure it does not get accidentally deleted
	// Sharded scheme as special pin logic to add
	// file root dag + shard root dag + metadata full dag + only this shard dag
	_, err := shardInfo.GetChallengeResponseOrNew(context.Background(), n, api, ss.FileHash, true, expir)
	if err != nil {
		log.Error(err)
		storage.GlobalSession.Remove(ss.ID, shardInfo.ShardHash.String())
		return
	}
}

var storageHostsCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "Interact with information on hosts.",
		ShortDescription: `Allows interaction with information on hosts. Host information is synchronized from btfs-hub and saved in local datastore.`,
	},
	Subcommands: map[string]*cmds.Command{
		"info": storageHostsInfoCmd,
		"sync": storageHostsSyncCmd,
	},
}

var storageHostsInfoCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Display saved host information.",
		ShortDescription: `
This command displays saved information from btfs-hub under multiple modes.
Each mode ranks hosts based on its criteria and is randomized based on current node location.

Mode options include:` + hub.AllModeHelpText,
	},
	Options: []cmds.Option{
		cmds.StringOption(hostInfoModeOptionName, "m", "Hosts info showing mode.").WithDefault(hub.HubModeAll),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		mode, _ := req.Options[hostInfoModeOptionName].(string)

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		nodes, err := storage.GetHostsFromDatastore(req.Context, n, mode, 0)
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &HostInfoRes{nodes})
	},
	Type: HostInfoRes{},
}

type HostInfoRes struct {
	Nodes []*hubpb.Host
}

var storageHostsSyncCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Synchronize host information from btfs-hub.",
		ShortDescription: `
This command synchronizes information from btfs-hub using multiple modes.
Each mode ranks hosts based on its criteria and is randomized based on current node location.

Mode options include:` + hub.AllModeHelpText,
	},
	Options: []cmds.Option{
		cmds.StringOption(hostSyncModeOptionName, "m", "Hosts syncing mode."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		mode, ok := req.Options[hostSyncModeOptionName].(string)
		if !ok {
			mode = cfg.Experimental.HostsSyncMode
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		return SyncHosts(req.Context, n, mode)
	},
}

func SyncHosts(ctx context.Context, node *core.IpfsNode, mode string) error {
	nodes, err := hub.QueryHub(ctx, node, mode)
	if err != nil {
		return err
	}
	return storage.SaveHostsIntoDatastore(ctx, node, mode, nodes)
}

var storageInfoCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Show storage host information.",
		ShortDescription: `
This command displays host information synchronized from the BTFS network.
By default it shows local host node information.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("peer-id", false, false, "Peer ID to show storage-related information. Default to self").EnableStdin(),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if len(req.Arguments) > 0 {
			if !cfg.Experimental.StorageClientEnabled {
				return fmt.Errorf("storage client api not enabled")
			}
		} else if !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		// Default to self
		var peerID string
		if len(req.Arguments) > 0 {
			peerID = req.Arguments[0]
		} else {
			peerID = n.Identity.Pretty()
		}

		data, err := hub.GetSettings(req.Context, cfg.Services.HubDomain, peerID, n.Repo.Datastore())
		if err != nil {
			return err
		}
		return cmds.EmitOnce(res, data)
	},
	Type: nodepb.Node_Settings{},
}

var storageAnnounceCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Update and announce storage host information.",
		ShortDescription: `
This command updates host information and broadcasts to the BTFS network. 

Examples

To set the max price per GiB to 1 BTT:
$ btfs storage announce --host-storage-price=1`,
	},
	Options: []cmds.Option{
		cmds.Uint64Option(hostStoragePriceOptionName, "s", "Max price per GiB of storage per day in BTT."),
		cmds.Uint64Option(hostBandwidthPriceOptionName, "b", "Max price per MiB of bandwidth in BTT."),
		cmds.Uint64Option(hostCollateralPriceOptionName, "cl", "Max collateral stake per hour per GiB in BTT."),
		cmds.FloatOption(hostBandwidthLimitOptionName, "l", "Max bandwidth limit per MB/s."),
		cmds.Uint64Option(hostStorageTimeMinOptionName, "d", "Min number of days for storage."),
		cmds.Uint64Option(hostStorageMaxOptionName, "m", "Max number of GB this host provides for storage."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}

		sp, spFound := req.Options[hostStoragePriceOptionName].(uint64)
		bp, bpFound := req.Options[hostBandwidthPriceOptionName].(uint64)
		cp, cpFound := req.Options[hostCollateralPriceOptionName].(uint64)
		bl, blFound := req.Options[hostBandwidthLimitOptionName].(float64)
		stm, stmFound := req.Options[hostStorageTimeMinOptionName].(uint64)
		sm, smFound := req.Options[hostStorageMaxOptionName].(uint64)

		if sp > bttTotalSupply || cp > bttTotalSupply || bp > bttTotalSupply {
			return fmt.Errorf("maximum price is %d", bttTotalSupply)
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		rds := n.Repo.Datastore()
		peerId := n.Identity.Pretty()

		ns, err := hub.GetSettings(req.Context, cfg.Services.HubDomain, peerId, rds)
		if err != nil {
			return err
		}

		// Update fields if set
		if spFound {
			ns.StoragePriceAsk = sp
		}
		if bpFound {
			ns.BandwidthPriceAsk = bp
		}
		if cpFound {
			ns.CollateralStake = cp
		}
		if blFound {
			ns.BandwidthLimit = bl
		}
		if stmFound {
			ns.StorageTimeMin = stm
		}
		// Storage size max is set in config instead of dynamic store
		if smFound {
			cfgRoot, err := cmdenv.GetConfigRoot(env)
			if err != nil {
				return err
			}
			sm = sm * uint64(units.GB)
			_, err = storage.CheckAndValidateHostStorageMax(cfgRoot, n.Repo, &sm, false)
			if err != nil {
				return err
			}
		}

		nb, err := proto.Marshal(ns)
		if err != nil {
			return err
		}

		err = rds.Put(storage.GetHostStorageKey(peerId), nb)
		if err != nil {
			return err
		}

		return nil
	},
}

type StatusRes struct {
	Status   string
	Message  string
	RetrySignStatus string
	FileHash string
	Shards   map[string]*ShardStatus
}

type ShardStatus struct {
	ContractID string
	Price      int64
	Host       string
	Status     string
}

var storageUploadStatusCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Check storage upload and payment status (From client's perspective).",
		ShortDescription: `
This command print upload and payment status by the time queried.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session.").EnableStdin(),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		status := &StatusRes{}
		// check and get session info from sessionMap
		ssID := req.Arguments[0]
		n, err := cmdenv.GetNode(env)
		ss, err := storage.GlobalSession.GetSessionWithoutLock(n, storage.RenterStoragePrefix, ssID)
		if err != nil {
			return err
		}

		// check if checking request from host or client
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled && !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage client/host api not enabled")
		}

		// get shards info from session
		status.Status, status.Message = ss.GetStatusAndMessage()
		status.RetrySignStatus = ss.RetrySignStatus
		status.FileHash = ss.GetFileHash().String()
		shards := make(map[string]*ShardStatus)
		status.Shards = shards
		for hash, info := range ss.ShardInfo {
			c := &ShardStatus{
				ContractID: info.ContractID,
				Price:      info.Price,
				Host:       info.Receiver.String(),
				Status:     info.GetStateStr(),
			}
			shards[hash] = c
		}
		return res.Emit(status)
	},
	Type: StatusRes{},
}

type GetContractBatchRes struct {
	Contracts []*storage.Contract
}

var storageUploadGetContractBatchCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get all the contracts from the upload session	(From BTFS SDK application's perspective).",
		ShortDescription: `
This command (on client) reads the unsigned contracts and returns 
the contracts to the caller.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session.").EnableStdin(),
		cmds.StringArg("peer-id", true, false, "Offline signs needed for this particular client.").EnableStdin(),
		cmds.StringArg("nonce-timestamp", true, false, "Nonce timestamp string for this offline signing."),
		cmds.StringArg("upload-session-signature", true, false, "Private key-signed string of peer-id:nounce-timestamp"),
		cmds.StringArg("session-status", true, false, "Current upload session status."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		batchRes := &GetContractBatchRes{}
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		ssID := req.Arguments[0]
		ss, err := storage.GlobalSession.GetSession(n, storage.RenterStoragePrefix, ssID)
		if err != nil {
			return err
		}

		status, _ := ss.GetStatus(ss.GetCurrentStatus())
		if req.Arguments[4] != status {
			return errors.New("unexpected session status from SDK during communication in offline signing")
		}

		// Get relevant contracts from ss and ss.ShardInfo
		contracts := make([]*storage.Contract, len(ss.ShardInfo))
		batchRes.Contracts = contracts
		index := 0
		for k, info := range ss.ShardInfo {
			c := &storage.Contract{
				Key: k,
			}
			// Set `c.Contract` according to the session status
			switch status {
			case "initSignReadyEscrow":
				by, err := MarshalForSign(info.UnsignedEscrowContract)
				if err != nil {
					return err
				}
				c.ContractData = string(by)
			case "initSignReadyGuard":
				by, err := MarshalForSign(info.UnsignedGuardContract)
				if err != nil {
					return err
				}
				c.ContractData = string(by)
			default:
				return fmt.Errorf("unexpected session status %s in renter node", status)
			}
			contracts[index] = c
			index++
		}

		// Change the status to the next to prevent another call of this endponnt by SDK library
		sendSessionStatusChan(ss.SessionStatusChan, ss.GetCurrentStatus(), true, nil)

		return res.Emit(batchRes)
	},
	Type: GetContractBatchRes{},
}

func MarshalForSign(message proto.Message) ([]byte, error){
	raw, err := proto.Marshal(message)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

var storageUploadSignbatchCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get all the contracts from the upload session	(From BTFS SDK application's perspective).",
		ShortDescription: `
This command (on client) reads the unsigned contracts and returns 
the contracts to the caller.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session.").EnableStdin(),
		cmds.StringArg("peer-id", true, false, "Offline signs needed for this particular client.").EnableStdin(),
		cmds.StringArg("nonce-timestamp", true, false, "Nonce timestamp string for this offline signing."),
		cmds.StringArg("upload-session-signature", true, false, "Private key-signed string of peer-id:nounce-timestamp"),
		cmds.StringArg("session-status", true, false, "current upload session status."),
		cmds.StringArg("signed-data-items", true, false, "signed data items."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		ssID := req.Arguments[0]
		ss, err := storage.GlobalSession.GetSession(n, storage.RenterStoragePrefix, ssID)
		if err != nil {
			return err
		}
		err = verifyReceivedMessage(req, ss)
		if err != nil {
			return err
		}

		// Set all the `shard.SignedBytes` from received signed data items.
		var signedContracts []storage.Contract
		receivedItemsString := req.Arguments[5]
		err = json.Unmarshal([]byte(receivedItemsString), &signedContracts)
		if err != nil {
			return err
		}
		if len(signedContracts) != len(ss.ShardInfo) {
			return fmt.Errorf("number of received signed data items %d does not match number of shards %d",
				len(signedContracts), len(ss.ShardInfo))
		}
		for i := 0; i<len(ss.ShardInfo); i++ {
			k := signedContracts[i].Key
			shard, found := ss.ShardInfo[k]
			if !found {
				return fmt.Errorf("can not find an entry for key %s from ShardInfo map", k)
			}
			shard.SignedBytes = []byte(signedContracts[i].ContractData)
		}

		// Broadcast
		currentStatus := ss.GetCurrentStatus()
		switch currentStatus {
		case storage.InitSignProcessForEscrowStatus:
			close(ss.OfflineCB.OfflineSignEscrowChan)
		case storage.InitSignProcessForGuardStatus:
			close(ss.OfflineCB.OfflineSignGuardChan)
		default:
			return fmt.Errorf("unexpected session status %d", currentStatus)
		}

		return nil
	},
}

func verifyReceivedMessage(req *cmds.Request, ss *storage.FileContracts) error {
	offlinePeerID, err := peer.IDB58Decode(req.Arguments[1])
	if err != nil {
		return err
	}
	if ss.OfflineCB.OfflinePeerID != offlinePeerID {
		return errors.New("peerIDs do not match")
	}
	offlineNonceTimestamp, err := strconv.ParseUint(req.Arguments[2], 10, 64)
	if err != nil {
		return err
	}
	if ss.OfflineCB.OfflineNonceTimestamp != offlineNonceTimestamp {
		return errors.New("Nonce timestamps do not match")
	}
	offlineSessionSignature := req.Arguments[3]
	if ss.OfflineCB.OfflineSessionSignature != offlineSessionSignature {
		return errors.New("Session signature do not match")
	}
	return nil
}

var storageUploadGetUnsignedCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get all the contracts from the upload session	(From BTFS SDK application's perspective).",
		ShortDescription: `
This command (on client) reads the unsigned contracts and returns 
the contracts to the caller.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session.").EnableStdin(),
		cmds.StringArg("peer-id", true, false, "Offline signs needed for this particular client.").EnableStdin(),
		cmds.StringArg("nonce-timestamp", true, false, "Nonce timestamp string for this offline signing."),
		cmds.StringArg("upload-session-signature", true, false, "Private key-signed string of peer-id:nounce-timestamp"),
		cmds.StringArg("session-status", true, false, "current upload session status."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		unsignedRes := &storage.GetUnsignedRes{}
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		ssID := req.Arguments[0]
		ss, err := storage.GlobalSession.GetSession(n, storage.RenterStoragePrefix, ssID)
		if err != nil {
			return err
		}
		err = verifyReceivedMessage(req, ss)
		if err != nil {
			return err
		}

		status, _ := ss.GetStatus(ss.GetCurrentStatus())
		if req.Arguments[4] != status {
			return errors.New("unexpected session status from SDK during communication in offline signing")
		}

		// Change the status to the next to prevent another call of this endponnt by SDK library
		currentSessionStatus := ss.GetCurrentStatus()
		sendSessionStatusChan(ss.SessionStatusChan, currentSessionStatus, true, nil)

		return res.Emit(unsignedRes)
	},
	Type: storage.GetUnsignedRes{},
}

var storageUploadSignCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Get all the contracts from the upload session	(From BTFS SDK application's perspective).",
		ShortDescription: `
This command (on client) reads the unsigned contracts and returns 
the contracts to the caller.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session.").EnableStdin(),
		cmds.StringArg("peer-id", true, false, "Offline signs needed for this particular client.").EnableStdin(),
		cmds.StringArg("nonce-timestamp", true, false, "Nonce timestamp string for this offline signing."),
		cmds.StringArg("upload-session-signature", true, false, "Private key-signed string of peer-id:nounce-timestamp"),
		cmds.StringArg("session-status", true, false, "current upload session status."),
		cmds.StringArg("signed", true, false, "signed json data."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		ssID := req.Arguments[0]
		ss, err := storage.GlobalSession.GetSession(n, storage.RenterStoragePrefix, ssID)
		if err != nil {
			return err
		}
		err = verifyReceivedMessage(req, ss)
		if err != nil {
			return err
		}

		// Set the received `signed`.
		if ss.OfflineCB == nil {
			return errors.New("offline control block is nil")
		}
		ss.OfflineCB.OfflineSigned = req.Arguments[5]

		// Broadcast
		close(ss.OfflineCB.OfflinePaySignChan)

		return nil
	},
}

var storageChallengeCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Interact with storage challenge requests and responses.",
		ShortDescription: `
These commands contain both client-side and host-side challenge functions.

btfs storage challenge request <peer-id> <contract-id> <file-hash> <shard-hash> <chunk-index> <nonce>
btfs storage challenge response <contract-id> <file-hash> <shard-hash> <chunk-index> <nonce>`,
	},
	Subcommands: map[string]*cmds.Command{
		"request":  storageChallengeRequestCmd,
		"response": storageChallengeResponseCmd,
	},
}

var storageChallengeRequestCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Challenge storage hosts with Proof-of-Storage requests.",
		ShortDescription: `
This command challenges storage hosts on behalf of a client to see if hosts
still store a piece of file (usually a shard) as agreed in storage contract.`,
	},
	Arguments: append([]cmds.Argument{
		cmds.StringArg("peer-id", true, false, "Host Peer ID to send challenge requests."),
	}, storageChallengeResponseCmd.Arguments...), // append pass-through arguments
	RunTimeout: 5 * time.Second, // TODO: consider slow networks?
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		// Check if peer is reachable
		pi, err := remote.FindPeer(req.Context, n, req.Arguments[0])
		if err != nil {
			return err
		}
		// Pass arguments through to host response endpoint
		resp, err := remote.P2PCallStrings(req.Context, n, pi.ID, "/storage/challenge/response",
			req.Arguments[1:]...)
		if err != nil {
			return err
		}

		var scr StorageChallengeRes
		err = json.Unmarshal(resp, &scr)
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &scr)
	},
	Type: StorageChallengeRes{},
}

type StorageChallengeRes struct {
	Answer string
}

var storageChallengeResponseCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Storage host responds to Proof-of-Storage requests.",
		ShortDescription: `
This command (on host) reads the challenge question and returns the answer to
the challenge request back to the caller.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("contract-id", true, false, "Contract ID associated with the challenge requests."),
		cmds.StringArg("file-hash", true, false, "File root multihash for the data stored at this host."),
		cmds.StringArg("shard-hash", true, false, "Shard multihash for the data stored at this host."),
		cmds.StringArg("chunk-index", true, false, "Chunk index for this challenge. Chunks available on this host include root + metadata + shard chunks."),
		cmds.StringArg("nonce", true, false, "Nonce for this challenge. A random UUIDv4 string."),
	},
	RunTimeout: 3 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}

		fileHash, err := cidlib.Parse(req.Arguments[1])
		if err != nil {
			return err
		}
		sh := req.Arguments[2]
		shardHash, err := cidlib.Parse(sh)
		if err != nil {
			return err
		}
		// Check if host store has this contract id
		shardInfo, err := storage.GetShardInfoFromDatastore(n, storage.HostStoragePrefix, sh)
		if err != nil {
			return err
		}
		ctID := req.Arguments[0]
		if ctID != shardInfo.ContractID {
			return fmt.Errorf("contract id does not match, has [%s], needs [%s]", shardInfo.ContractID, ctID)
		}
		if !shardHash.Equals(shardInfo.ShardHash) {
			return fmt.Errorf("datastore internal error: mismatched existing shard hash %s with %s",
				shardInfo.ShardHash.String(), shardHash.String())
		}
		chunkIndex, err := strconv.Atoi(req.Arguments[3])
		if err != nil {
			return err
		}
		nonce := req.Arguments[4]
		// Get (cached) challenge response object and solve challenge
		sc, err := shardInfo.GetChallengeResponseOrNew(req.Context, n, api, fileHash, false, 0)
		if err != nil {
			return err
		}
		err = sc.SolveChallenge(chunkIndex, nonce)
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &StorageChallengeRes{Answer: sc.Hash})
	},
	Type: StorageChallengeRes{},
}

// prepareSignedContractsForEscrowOffSign gets a valid host and
// prepares unsigned escrow contract and unsigned quard contract.
// Moves the session status to `InitSignReadyForEscrowStatus`.
func prepareSignedContractsForEscrowOffSign(param *paramsForPrepareContractsForShard, retryCalling bool) (error) {
	ss := param.ss
	shard := param.shard
	shardIndex := shard.ShardIndex


	shard.SetPrice(shard.CandidateHost.Price)
	cfg, err := param.n.Repo.Config()
	if err != nil {
		return err
	}

	// init escrow Contract
	_, pid, err := ParsePeerParam(shard.CandidateHost.Identity)
	if err != nil {
		return err
	}
	escrowContract, err := escrow.NewContract(cfg, shard.ContractID, param.n, pid, shard.TotalPay)
	if err != nil {
		log.Error("create escrow contract failed. ", err)
		return err
	}
	shard.UpdateShard(pid)
	guardContractMeta, err := guard.NewContract(param.ss, cfg, param.shardKey, int32(shardIndex), param.renterPid)
	if err != nil {
		log.Error("fail to new contract meta")
		return err
	}

	// Output for this function is set here
	shard.UnsignedEscrowContract = escrowContract
	shard.UnsignedGuardContract = guardContractMeta

	if !retryCalling {
		// Change the session status to `InitSignReadyForEscrowStatus`.
		ss.IncrementOffSignReadyShards()
		if ss.GetOffSignReadyShards() == len(ss.ShardInfo) {
			currentStatus := ss.GetCurrentStatus()
			if currentStatus != storage.UninitializedStatus {
				return fmt.Errorf("current status %d does not match expected UninitializedStatus", currentStatus)
			}
			// Reset variables for the next offline signing for the session
			ss.SetOffSignReadyShards(0)
			sendSessionStatusChan(ss.SessionStatusChan, currentStatus, true, nil)
		}
	} else {
		// Build a Contract and offer to the OffSignQueue

		// Change shard status
		// TODO: steve Do at the next .. Change offlineSigningStatus to ready
	}

	return nil
}

func initOfflineSignChannels(ss *storage.FileContracts) error {
	if ss.RunMode != offlineSignMode {
		return errors.New("it is not offline sign mode")
	}
	if ss.OfflineCB == nil {
		return errors.New("offline control block is nil")
	}
	offCB := ss.OfflineCB
	offCB.OfflineSignEscrowChan = nil
	offCB.OfflineSignEscrowChan = make(chan string)
	offCB.OfflineSignGuardChan = nil
	offCB.OfflineSignGuardChan = make(chan string)
	offCB.OfflineInitSigDoneChan = nil
	offCB.OfflineInitSigDoneChan = make(chan string)
	offCB.OfflinePaySignChan = nil
	offCB.OfflinePaySignChan = make(chan string)

	return nil
}

func resetPaySignChannel(ss *storage.FileContracts) error {
	if ss.OfflineCB == nil {
		return errors.New("offline control block is nil")
	}
	ss.OfflineCB.OfflinePaySignChan = nil
	ss.OfflineCB.OfflinePaySignChan = make(chan string)

	return nil
}

// prepareSignedGuardContractForShardOffSign waits for broadcast signal and
// moves the session status to `InitSignReadyForGuardStatus`
func prepareSignedGuardContractForShardOffSign(ss *storage.FileContracts, shard *storage.Shard, n *core.IpfsNode, retryCalling bool) error {
	// "/storage/upload/getcontractbatch" and "/storage/upload/signedbatch" handlers perform responses
	// to SDK application's requests and sets each `shard.HalfSignedEscrowContract` with signed bytes.
	// The corresponding endpoint for `signedbatch` closes "ss.OfflineSignChan" to broadcast
	// Here we wait for the broadcast signal.
	select {
	case <- ss.OfflineCB.OfflineSignEscrowChan:
	}
	var err error
	shard.HalfSignedEscrowContract, err = escrow.SignContractAndMarshalOffSign(shard.UnsignedEscrowContract, shard.SignedBytes, nil, true)
	if err != nil {
		log.Error("sign escrow contract and maorshal failed ")
		return err
	}

	// Output for this function is set here
	//shard.HalfSignedEscrowContract = halfSignedEscrowContract
	ss.IncrementOffSignReadyShards()
	// moves the session status to `InitSignReadyForGuardStatus`
	if ss.GetOffSignReadyShards() == len(ss.ShardInfo) {
		if err != nil {
			return err
		}
		currentStatus := ss.GetCurrentStatus()
		if currentStatus != storage.InitSignProcessForEscrowStatus {
			return fmt.Errorf("current status %d does not match expected InitSignProcessForEscrowStatus", currentStatus)
		}
		ss.SetOffSignReadyShards(0)
		sendSessionStatusChan(ss.SessionStatusChan, currentStatus, true, nil)
	}
	return nil
}

// buildSignedGuardContractForShardOffSign waits for broadcast signal,
// builds halfSignedEscrowContract and moves the session status to `InitSignStatus`
// moves the session status to `InitSignStatus`
func buildSignedGuardContractForShardOffSign(ss *storage.FileContracts,
	shard *storage.Shard, n *core.IpfsNode, runMode int, renterPid string, retryCalling bool) error {
	// Wait for the broadcast signal by "/storage/upload/signedbatch" handler.
	select {
	case <- ss.OfflineCB.OfflineSignGuardChan:
	}
	var err error
	shard.HalfSignedGuardContract, err =
		guard.SignedContractAndMarshalOffSign(shard.UnsignedGuardContract, shard.SignedBytes, nil, true, runMode == repairMode, renterPid, n.Identity.Pretty())
	if err != nil {
		log.Error("sign guard contract and maorshal failed ")
		return err
	}

	// moves the session status to `InitSignStatus`
	ss.IncrementOffSignReadyShards()
	if ss.GetOffSignReadyShards() == len(ss.ShardInfo) {
		currentStatus := ss.GetCurrentStatus()
		if currentStatus != storage.InitSignProcessForGuardStatus {
			return fmt.Errorf("current status %d does not match expected InitSignProcessForGuardStatus", currentStatus)
		}
		sendSessionStatusChan(ss.SessionStatusChan, currentStatus, true, nil)
		close(ss.OfflineCB.OfflineInitSigDoneChan)
	} else {
		// Wait for the broadcast signal by the last goroutine
		select {
		case <- ss.OfflineCB.OfflineInitSigDoneChan:
		}
	}
	return nil
}

// PerformBalanceOffSign moves the session status to
// `BalanceSignReadyStatus` and get and return balance signed bytes
func PerformBalanceOffSign(ss *storage.FileContracts) ([]byte, error) {
	ss.ResetOfflineUnsigned()
	ss.OfflineCB.OfflineUnsigned.Opcode = "balance"
	currentStatus := ss.GetCurrentStatus()
	if currentStatus != storage.SubmitStatus {
		return nil, fmt.Errorf("current status %d does not match expected SubmitStatus", currentStatus)
	}
	sendSessionStatusChan(ss.SessionStatusChan, currentStatus, true, nil)
	// Wait for the signal that indicates signed bytes are received.
	select {
	case <- ss.OfflineCB.OfflinePaySignChan:
	}

	resetPaySignChannel(ss)
	return []byte(ss.OfflineCB.OfflineSigned), nil
}

// PerformPayChannelOffSign set up `ss.OfflineUnsigned`,
// moves the session status to `PayChannelSignReadyStatus`,
// and get and return pay channel signed bytes
func PerformPayChannelOffSign(ss *storage.FileContracts, escrowAddress []byte, totalPrice int64) ([]byte, error) {
	ss.ResetOfflineUnsigned()
	ss.OfflineCB.OfflineUnsigned.Opcode = "paychannel"
	ss.OfflineCB.OfflineUnsigned.Unsigned = string(escrowAddress)
	ss.OfflineCB.OfflineUnsigned.Price = totalPrice
	currentStatus := ss.GetCurrentStatus()
	if currentStatus != storage.BalanceSignProcessStatus {
		return nil, fmt.Errorf("current status %d does not match expected BalanceSignProcessStatus", currentStatus)
	}
	sendSessionStatusChan(ss.SessionStatusChan, currentStatus, true, nil)
	// Wait for the signal that indicates signed bytes are received.
	select {
	case <- ss.OfflineCB.OfflinePaySignChan:
	}

	resetPaySignChannel(ss)
	return []byte(ss.OfflineCB.OfflineSigned), nil
}

func PerformPayinRequestOffSign(ss *storage.FileContracts, result *escrowpb.SignedSubmitContractResult) ([]byte, error) {
	ss.ResetOfflineUnsigned()
	ss.OfflineCB.OfflineUnsigned.Opcode = "payrequest"
	b, err := proto.Marshal(result)
	if err != nil {
		return nil, err
	}
	ss.OfflineCB.OfflineUnsigned.Unsigned = string(b)

	currentStatus := ss.GetCurrentStatus()
	if currentStatus != storage.PayStatus {
		return nil, fmt.Errorf("current status %d does not match expected PayStatus", currentStatus)
	}
	sendSessionStatusChan(ss.SessionStatusChan, currentStatus, true, nil)

	// Wait for the signal that indicates signed bytes are received.
	select {
	case <- ss.OfflineCB.OfflinePaySignChan:
	}

	resetPaySignChannel(ss)
	return []byte(ss.OfflineCB.OfflineSigned), nil
}

func PerformGuardFileMetaOffSign(ss *storage.FileContracts, meta *guardpb.FileStoreMeta) ([]byte, error) {
	ss.ResetOfflineUnsigned()
	ss.OfflineCB.OfflineUnsigned.Opcode = "guard"
	b, err := proto.Marshal(meta)
	if err != nil {
		return nil, err
	}
	ss.OfflineCB.OfflineUnsigned.Unsigned = string(b)

	currentStatus := ss.GetCurrentStatus()
	if currentStatus != storage.PayReqSignProcessStatus {
		return nil, fmt.Errorf("current status %d does not match expected PayReqSignProcessStatus", currentStatus)
	}
	sendSessionStatusChan(ss.SessionStatusChan, currentStatus, true, nil)

	// Wait for the signal that indicates signed bytes are received.
	select {
	case <- ss.OfflineCB.OfflinePaySignChan:
	}

	resetPaySignChannel(ss)
	return []byte(ss.OfflineCB.OfflineSigned), nil
}

func BalanceWithOffSign(ctx context.Context, configuration *config.Config, ss *storage.FileContracts) (int64, error) {
	signed, err := PerformBalanceOffSign(ss)
	if err != nil {
		return 0, err
	}

	var lgSignedPubKey ledgerpb.SignedPublicKey
	err = json.Unmarshal(signed, &lgSignedPubKey)
	if err != nil {
		return 0, err
	}

	var balance int64 = 0
	err = grpc.EscrowClient(configuration.Services.EscrowDomain).WithContext(ctx,
		func(ctx context.Context, client escrowpb.EscrowServiceClient) error {
			res, err := client.BalanceOf(ctx, &lgSignedPubKey)
			if err != nil {
				return err
			}
			err = escrow.VerifyEscrowRes(configuration, res.Result, res.EscrowSignature)
			if err != nil {
				return err
			}
			balance = res.Result.Balance
			log.Debug("balanceof account is ", balance)
			return nil
		})
	if err != nil {
		return 0, err
	}
	return balance, nil
}

func NewContractRequestWithOffSign(configuration *config.Config, ss *storage.FileContracts, signedContracts []*escrowpb.SignedEscrowContract, totalPrice int64) (*escrowpb.EscrowContractRequest, error) {
	escrowAddress, err := escrow.ConvertToAddress(configuration.Services.EscrowPubKeys[0])
	if err != nil {
		return nil, err
	}
	signed, err := PerformPayChannelOffSign(ss, escrowAddress, totalPrice)
	if err != nil {
		return nil, err
	}

	var signedChanCommit ledgerpb.SignedChannelCommit
	err = proto.Unmarshal(signed, &signedChanCommit)
	if err != nil {
		return nil, err
	}

	return &escrowpb.EscrowContractRequest{
		Contract:     signedContracts,
		BuyerChannel: &signedChanCommit,
	}, nil
}

func NewPayinRequestOffSign(ss *storage.FileContracts, result *escrowpb.SignedSubmitContractResult) (*escrowpb.SignedPayinRequest, error) {
	signed, err := PerformPayinRequestOffSign(ss, result)
	if err != nil {
		return nil, err
	}

	signedPayInRequest := new(escrowpb.SignedPayinRequest)
	err = proto.Unmarshal(signed, signedPayInRequest)
	if err != nil {
		return nil, err
	}

	return signedPayInRequest, nil
}

func PrepAndUploadFileMetaOffSign(ctx context.Context, ss *storage.FileContracts,
	escrowResults *escrowpb.SignedSubmitContractResult, payinRes *escrowpb.SignedPayinResult,
	configuration *config.Config) (*guardpb.FileStoreStatus, error) {
	// get escrow sig, add them to guard
	contracts := ss.GetGuardContracts()
	sig := payinRes.EscrowSignature
	for _, guardContract := range contracts {
		guardContract.EscrowSignature = sig
		guardContract.EscrowSignedTime = payinRes.Result.EscrowSignedTime
		guardContract.LastModifyTime = time.Now()
	}

	fileStatus, err := guard.NewFileStatus(ss, contracts, configuration)
	if err != nil {
		return nil, err
	}

	signed, err := PerformGuardFileMetaOffSign(ss, &fileStatus.FileStoreMeta)
	if err != nil {
		return nil, err
	}

	if fileStatus.PreparerPid == fileStatus.RenterPid {
		fileStatus.RenterSignature = signed
	} else {
		fileStatus.RenterSignature = signed
		fileStatus.PreparerSignature = signed
	}

	err = guard.SubmitFileStatus(ctx, configuration, fileStatus)
	if err != nil {
		return nil, err
	}

	return fileStatus, nil
}
