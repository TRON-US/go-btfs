package commands

import (
	"context"
	"encoding/json"
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

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/TRON-US/interface-go-btfs-core/path"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	hubpb "github.com/tron-us/go-btfs-common/protos/hub"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"

	"github.com/Workiva/go-datastructures/set"
	"github.com/alecthomas/units"
	"github.com/gogo/protobuf/proto"
	cidlib "github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	leafHashOptionName               = "leaf-hash"
	uploadPriceOptionName            = "price"
	replicationFactorOptionName      = "replication-factor"
	hostSelectModeOptionName         = "host-select-mode"
	hostSelectionOptionName          = "host-selection"
	hostInfoModeOptionName           = "host-info-mode"
	hostSyncModeOptionName           = "host-sync-mode"
	hostStoragePriceOptionName       = "host-storage-price"
	hostBandwidthPriceOptionName     = "host-bandwidth-price"
	hostCollateralPriceOptionName    = "host-collateral-price"
	hostBandwidthLimitOptionName     = "host-bandwidth-limit"
	hostStorageTimeMinOptionName     = "host-storage-time-min"
	hostStorageMaxOptionName         = "host-storage-max"
	testOnlyOptionName               = "host-search-local"
	storageLengthOptionName          = "storage-length"
	repairModeOptionName             = "repair-mode"
	customizedPayoutOptionName       = "customize-payout"
	customizedPayoutPeriodOptionName = "customize-payout-period"

	defaultRepFactor     = 3
	defaultStorageLength = 30

	// retry limit
	RetryLimit = 3
	FailLimit  = 3

	bttTotalSupply uint64 = 990_000_000_000
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
    # Total # of hosts (N) must match # of shards in the first DAG level of root file hash
    $ btfs storage upload <file-hash> -m=custom -s=<host1-peer-id>,<host2-peer-id>,...,<hostN-peer-id>

    # Upload specific shards to a set of hosts
    # Total # of hosts (N) must match # of shards given
	$ btfs storage upload <shard-hash1> <shard-hash2> ... <shard-hashN> -l -m=custom -s=<host1-peer-id>,<host2-peer-id>,...,<hostN-peer-id>

Use status command to check for completion:
    $ btfs storage upload status <session-id> | jq`,
	},
	Subcommands: map[string]*cmds.Command{
		"init":         storageUploadInitCmd,
		"recvcontract": storageUploadRecvContractCmd,
		"status":       storageUploadStatusCmd,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, false, "Hash of file to upload."),
		cmds.StringArg("repair-shards", false, false, "Shard hashes to repair."),
		cmds.StringArg("renter-pid", false, false, "Original renter peer ID."),
		cmds.StringArg("blacklist", false, false, "Blacklist of hosts during upload."),
	},
	Options: []cmds.Option{
		cmds.BoolOption(leafHashOptionName, "l", "Flag to specify given hash(es) is leaf hash(es).").WithDefault(false),
		cmds.Int64Option(uploadPriceOptionName, "p", "Max price per GiB per day of storage in BTT."),
		cmds.IntOption(replicationFactorOptionName, "r", "Replication factor for the file with erasure coding built-in.").WithDefault(defaultRepFactor),
		cmds.StringOption(hostSelectModeOptionName, "m", "Based on this mode to select hosts and upload automatically. Default: mode set in config option Experimental.HostsSyncMode."),
		cmds.StringOption(hostSelectionOptionName, "s", "Use only these selected hosts in order on 'custom' mode. Use ',' as delimiter."),
		cmds.BoolOption(testOnlyOptionName, "t", "Enable host search under all domains 0.0.0.0 (useful for local test)."),
		cmds.IntOption(storageLengthOptionName, "len", "File storage period on hosts in days.").WithDefault(defaultStorageLength),
		cmds.BoolOption(repairModeOptionName, "rm", "Enable repair mode.").WithDefault(false),
		cmds.BoolOption(customizedPayoutOptionName, "Enable file storage customized payout schedule.").WithDefault(false),
		cmds.IntOption(customizedPayoutPeriodOptionName, "Period of customized payout schedule.").WithDefault(1),
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
			shardHashes []string
			rootHash    cidlib.Cid
			shardSize   uint64
			blacklist   = set.New()
			renterPid   = n.Identity
		)
		isRepairMode := req.Options[repairModeOptionName].(bool)
		lf := req.Options[leafHashOptionName].(bool)
		if isRepairMode {
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
		} else if !lf {
			if len(req.Arguments) != 1 {
				return fmt.Errorf("need one and only one root file hash")
			}
			// get leaf hashes
			hashStr := req.Arguments[0]
			// convert to cid
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
		ss.SetStatus(storage.InitStatus)
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
		customizedPayout := req.Options[customizedPayoutOptionName].(bool)
		p := req.Options[customizedPayoutPeriodOptionName].(int)

		go retryMonitor(context.Background(), api, ss, n, ssID, testFlag,
			isRepairMode, renterPid.Pretty(), customizedPayout, p)

		seRes := &UploadRes{
			ID: ssID,
		}
		return res.Emit(seRes)
	},
	Type: UploadRes{},
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
	// error is special std flow, will not be counted in here
	// and complete status don't need to wait for signal coming
	for curStatus := 0; curStatus < len(stateFlow)-2; {
		select {
		case sessionState := <-ss.SessionStatusChan:
			if sessionState.Succeed {
				curStatus = sessionState.CurrentStep + 1
				ss.SetStatus(curStatus)
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

type paramsForPrepareContractsForShard struct {
	ctx                context.Context
	rq                 *storage.RetryQueue
	api                coreiface.CoreAPI
	ss                 *storage.FileContracts
	n                  *core.IpfsNode
	test               bool
	isRepair           bool
	renterPid          string
	shardKey           string
	shard              *storage.Shard
	customizedSchedule bool
	period             int
}

func prepareSignedContractsForShard(param *paramsForPrepareContractsForShard, candidateHost *storage.HostNode) error {
	shard := param.shard
	shardIndex := shard.ShardIndex

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
	escrowContract, err := escrow.NewContract(cfg, shard.ContractID, param.n, pid,
		shard.TotalPay, param.customizedSchedule, param.period)
	if err != nil {
		return fmt.Errorf("create escrow contract failed: [%v] ", err)
	}
	halfSignedEscrowContract, err := escrow.SignContractAndMarshal(escrowContract, nil, param.n.PrivateKey, true)
	if err != nil {
		return fmt.Errorf("sign escrow contract and maorshal failed: [%v] ", err)
	}
	shard.UpdateShard(pid)
	guardContractMeta, err := guard.NewContract(param.ss, cfg, param.shardKey, int32(shardIndex), param.renterPid)
	if err != nil {
		return fmt.Errorf("fail to new contract meta: [%v] ", err)
	}
	halfSignGuardContract, err := guard.SignedContractAndMarshal(guardContractMeta, nil, param.n.PrivateKey, true,
		param.isRepair, param.renterPid, param.n.Identity.Pretty())
	if err != nil {
		return fmt.Errorf("fail to sign guard contract and marshal: [%v] ", err)
	}

	// Set the output of this function.
	shard.CandidateHost = candidateHost
	shard.HalfSignedEscrowContract = halfSignedEscrowContract
	shard.HalfSignedGuardContract = halfSignGuardContract

	return nil
}

func retryMonitor(ctx context.Context, api coreiface.CoreAPI, ss *storage.FileContracts, n *core.IpfsNode,
	ssID string, test bool, isRepair bool, renterPid string, customizedSchedule bool, period int) {
	retryQueue := ss.GetRetryQueue()
	if retryQueue == nil {
		log.Error("retry queue is nil")
		return
	}

	// loop over each shard
	for shardKey, shard := range ss.ShardInfo {
		go func(shardKey string, shard *storage.Shard) {
			param := &paramsForPrepareContractsForShard{
				ctx:       ctx,
				rq:        retryQueue,
				api:       api,
				ss:        ss,
				n:         n,
				test:      test,
				isRepair:  isRepair,
				renterPid: renterPid,
				shardKey:  shardKey,
				shard:     shard,
			}

			// build connection with host, init step, could be error or timeout
			go retryProcess(param, nil, true)

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
							shard.CandidateHost.IncrementRetry()
							// if reach retry limit, in retry process will select another host
							// so in channel receiving should also return to 'init'
							curState = storage.InitState
							go retryProcess(param, shard.CandidateHost, false)
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
						log.Errorf("upload timed out with state %s", storage.StdStateFlow[curState].State)
						if shard.CandidateHost != nil {
							shard.CandidateHost.IncrementRetry()
						}
						curState = storage.InitState // reconnect to the host to start over

						go retryProcess(param, shard.CandidateHost, false)
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

func retryProcess(param *paramsForPrepareContractsForShard, candidateHost *storage.HostNode, initial bool) {
	ss := param.ss
	shard := param.shard

	// TODO: maybe if current session status is ErrStatus return. but need test

	// check if current shard has been contacting and receiving results
	if shard.GetState() >= storage.ContractState {
		return
	}

	// if candidate host passed in is not valid, fetch next valid one
	var hostChanged bool
	if initial || candidateHost == nil || candidateHost.FailTimes >= FailLimit || candidateHost.RetryTimes >= RetryLimit {
		otherValidHost, err := getValidHost(param.ctx, ss.RetryQueue, param.api, param.n, param.test)
		// either retry queue is empty or something wrong with retry queue
		if err != nil {
			sendStepStateChan(shard.RetryChan, storage.InitState, false, fmt.Errorf("no host available %v", err), nil)
			return
		}
		candidateHost = otherValidHost
		hostChanged = true
	}

	// if host is changed for the "shard", move the shard state to 0 and retry
	if hostChanged {
		err := prepareSignedContractsForShard(param, candidateHost)
		if err != nil {
			sendStepStateChan(shard.RetryChan, storage.InitState, false, err, nil)
			return
		}
	}
	// parse candidate host's IP and get connected
	_, hostPid, err := ParsePeerParam(candidateHost.Identity)
	if err != nil {
		sendStepStateChan(shard.RetryChan, storage.InitState, false, err, nil)
		return
	}

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

// find next available host
func getValidHost(ctx context.Context, retryQueue *storage.RetryQueue, api coreiface.CoreAPI,
	n *core.IpfsNode, test bool) (*storage.HostNode, error) {

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
			if err := api.Swarm().Connect(ctx, *pi); err != nil {
				// in test mode, change the ip to 0.0.0.0
				if test {
					if err = changeAddress(pi); err == nil {
						err = api.Swarm().Connect(ctx, *pi)
					}
				}
				if err != nil {
					log.Error(err)
					nextHost.IncrementFail()
					err = retryQueue.Offer(nextHost)
					if err != nil {
						return nil, err
					}
					continue
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
		// TODO: For more secure, check if contracts are right or not
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
			// collecting all signed contracts means init status finished
			sendSessionStatusChan(ss.SessionStatusChan, storage.InitStatus, true, nil)
			contracts, totalPrice, err := storage.PrepareContractFromShard(ss.ShardInfo)
			if err != nil {
				sendSessionStatusChan(ss.SessionStatusChan, storage.SubmitStatus, false, err)
				return err
			}
			// check account balance, if not enough for the totalPrice do not submit to escrow
			balance, err := escrow.Balance(req.Context, cfg)
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

			contractRequest, err = escrow.NewContractRequest(cfg, contracts, totalPrice)
			if err != nil {
				sendSessionStatusChan(ss.SessionStatusChan, storage.SubmitStatus, false, err)
				return err
			}
			submitContractRes, err := escrow.SubmitContractToEscrow(req.Context, cfg, contractRequest)
			if err != nil {
				err = fmt.Errorf("failed to submit contracts to escrow: [%v]", err)
				sendSessionStatusChan(ss.SessionStatusChan, storage.SubmitStatus, false, err)
				return err
			}
			sendSessionStatusChan(ss.SessionStatusChan, storage.SubmitStatus, true, nil)

			// get core api
			api, err := cmdenv.GetApi(env, req)
			if err != nil {
				sendSessionStatusChan(ss.SessionStatusChan, storage.PayStatus, false, err)
				return err
			}

			go payFullToEscrowAndSubmitToGuard(context.Background(), n, api, submitContractRes, cfg, ss)
		}
		return nil
	},
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
		if err != nil {
			return err
		}
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
		cmds.StringOption(hostInfoModeOptionName, "m", "Hosts info showing mode. Default: mode set in config option Experimental.HostsSyncMode."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		mode, ok := req.Options[hostInfoModeOptionName].(string)
		if !ok {
			mode = cfg.Experimental.HostsSyncMode
		}

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
		cmds.StringOption(hostSyncModeOptionName, "m", "Hosts syncing mode. Default: mode set in config option Experimental.HostsSyncMode."),
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
		cmds.StringArg("peer-id", false, false, "Peer ID to show storage-related information. Default to self.").EnableStdin(),
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
		if err != nil {
			return err
		}
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
