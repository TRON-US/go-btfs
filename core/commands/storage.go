package commands

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/core/escrow"
	"github.com/TRON-US/go-btfs/core/hub"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/TRON-US/interface-go-btfs-core/path"
	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	hubpb "github.com/tron-us/go-btfs-common/protos/hub"
	ledgerpb "github.com/tron-us/go-btfs-common/protos/ledger"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/gogo/protobuf/proto"
	cidlib "github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
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
	testOnlyOptionName            = "host-search-local"

	defaultRepFactor = 3

	// retry limit
	RetryLimit = 3
	FailLimit  = 3
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
		"upload":   storageUploadCmd,
		"hosts":    storageHostsCmd,
		"info":     storageInfoCmd,
		"announce": storageAnnounceCmd,
	},
}

var storageUploadCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Store files on BTFS network nodes through BTT payment.",
		ShortDescription: `
By default, BTFS will select hosts based on overall score according to current client's environment.
To upload a file, <file-hash> must refer to a reed-solomon encoded file.

To create a reed-solomon encoded file from a normal file:

    $ btfs add --chunker=reed-solomon <file>
    added <file-hash> <file>

Run command to upload:

    $ btfs storage upload <file-hash>

To customly upload and store a file on specific hosts:
    Use -m with 'custom' mode, and put host identifiers in -s, with multiple hosts separated by ','.

    # Upload a file to a set of hosts
    # Total # of hosts must match # of shards in the first DAG level of root file hash
    $ btfs storage upload <file-hash> -m=custom -s=<host_address1>,<host_address2>

    # Upload specific chunks to a set of hosts
    # Total # of hosts must match # of chunks given
    $ btfs storage upload <chunk-hash1> <chunk-hash2> -l -m=custom -s=<host_address1>,<host_address2>

Receive proofs as collateral evidence after selected nodes agree to store the file.`,
	},
	Subcommands: map[string]*cmds.Command{
		"init":         storageUploadInitCmd,
		"reqc":         storageUploadRequestChallengeCmd,
		"respc":        storageUploadResponseChallengeCmd,
		"recvcontract": storageUploadRecvContractCmd,
		"status":       storageUploadStatusCmd,
		"proof":        storageUploadProofCmd,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, true, "Add hash of file to upload.").EnableStdin(),
	},
	Options: []cmds.Option{
		cmds.BoolOption(leafHashOptionName, "l", "Flag to specify given hash(es) is leaf hash(es).").WithDefault(false),
		cmds.Int64Option(uploadPriceOptionName, "p", "Max price per GB of storage in BTT."),
		cmds.Int64Option(replicationFactorOptionName, "r", "Replication factor for the file with erasure coding built-in.").WithDefault(defaultRepFactor),
		cmds.StringOption(hostSelectModeOptionName, "m", "Based on mode to select the host and upload automatically.").WithDefault(storage.HostModeDefault),
		cmds.StringOption(hostSelectionOptionName, "s", "Use only these selected hosts in order on 'custom' mode. Use ',' as delimiter."),
		cmds.BoolOption(testOnlyOptionName, "t", "Enable host search under all domains 0.0.0.0 (useful for local test).").WithDefault(true),
	},
	RunTimeout: 5 * time.Minute, // TODO: handle large file uploads?
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
			chunkHashes []string
			rootHash    cidlib.Cid
		)
		lf, _ := req.Options[leafHashOptionName].(bool)
		if lf == false {
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
			if err != nil {
				return err
			}
			for _, h := range hashes {
				chunkHashes = append(chunkHashes, h.String())
			}
		} else {
			rootHash = cidlib.Undef
			chunkHashes = req.Arguments
		}
		// start new session
		sm := storage.GlobalSession
		ssID, err := storage.NewSessionID()
		// start new session
		if err != nil {
			return err
		}
		ss := sm.GetOrDefault(ssID)
		ss.SetFileHash(rootHash)
		ss.SetStatus(storage.InitStatus)
		go controlSessionTimeout(ss)

		// get hosts/peers
		var peers []string
		// init retry queue
		retryQueue := storage.NewRetryQueue(int64(len(peers)))
		// set price limit, the price is default when host doesn't provide price
		price, found := req.Options[uploadPriceOptionName].(int64)
		if found && price < HostPriceLowBoundary {
			return fmt.Errorf("price is smaller than minimum setting price")
		}
		if !found {
			price = 10
		}
		mode, _ := req.Options[hostSelectModeOptionName].(string)
		if mode == "custom" {
			// get host list as user specified
			hosts, found := req.Options[hostSelectionOptionName].(string)
			if !found {
				return fmt.Errorf("custom mode needs input host lists")
			}
			peers = strings.Split(hosts, ",")
			if len(peers) != len(chunkHashes) {
				return fmt.Errorf("custom mode hosts length must match chunk hashes length")
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
			hosts, err := storage.GetHostsFromDatastore(req.Context, n, mode, len(chunkHashes))
			if err != nil {
				return err
			}
			for _, ni := range hosts {
				// use host askingPrice instead if provided
				if int64(ni.StoragePriceAsk) != HostPriceLowBoundary {
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

		// retry queue need to be reused in proof cmd
		ss.SetRetryQueue(retryQueue)

		// add chunks into session
		for _, singleChunk := range chunkHashes {
			ss.GetOrDefault(singleChunk)
		}
		testFlag := req.Options[testOnlyOptionName].(bool)
		go retryMonitor(context.Background(), api, ss, n, ssID, testFlag)

		seRes := &UploadRes{
			ID: ssID,
		}
		return res.Emit(seRes)
	},
	Type: UploadRes{},
}

func controlSessionTimeout(ss *storage.Session) {
	// error is special std flow, will not be counted in here
	// and complete status don't need to wait for signal coming
	for curStatus := 0; curStatus < len(storage.StdSessionStateFlow)-2; {
		select {
		case sessionState := <-ss.SessionStatusChan:
			if sessionState.Succeed {
				curStatus = sessionState.CurrentStep + 1
				ss.SetStatus(curStatus)
			} else {
				// TODO: ADD error info to status
				log.Error(sessionState.Err)
				ss.SetStatus(storage.ErrStatus)
				return
			}
		case <-time.After(storage.StdSessionStateFlow[curStatus].TimeOut):
			ss.SetStatus(storage.ErrStatus)
			// sending each chunk channel with terminate signal if they are in progress
			// otherwise no chunk channel would be there receiving
			for _, chunkInfo := range ss.ChunkInfo {
				go func(chunkInfo *storage.Chunk) {
					if chunkInfo.GetState() != storage.StdChunkStateFlow[storage.CompleteState].State {
						chunkInfo.RetryChan <- &storage.StepRetryChan{
							Succeed:           false,
							SessionTimeOutErr: fmt.Errorf("session timeout"),
						}
					}
				}(chunkInfo)
			}
			return
		}
	}
}

func retryMonitor(ctx context.Context, api coreiface.CoreAPI, ss *storage.Session, n *core.IpfsNode, ssID string, test bool) {
	retryQueue := ss.GetRetryQueue()
	if retryQueue == nil {
		log.Error("retry queue is nil")
		return
	}

	// loop over each chunk
	for chunkHash, chunkInfo := range ss.ChunkInfo {
		go func(chunkHash string, chunkInfo *storage.Chunk) {
			candidateHost, err := getValidHost(ctx, retryQueue, api, n, test)
			if err != nil {
				sendSessionStatusChan(ss.SessionStatusChan, storage.InitStatus, false, err)
				return
			}
			cfg, err := n.Repo.Config()
			if err != nil {
				sendSessionStatusChan(ss.SessionStatusChan, storage.InitStatus, false, err)
				return
			}
			contract := escrow.NewContract(cfg, chunkHash, n.Identity.Pretty(), candidateHost.Identity, candidateHost.Price)
			halfSignedContract, err := escrow.SignContractAndMarshal(contract, nil, n.PrivateKey, true)
			if err != nil {
				sendSessionStatusChan(ss.SessionStatusChan, storage.InitStatus, false, err)
				return
			}
			// build connection with host, init step, could be error or timeout
			go retryProcess(ctx, api, candidateHost, ss, n, halfSignedContract, chunkHash, ssID, test)

			// monitor each steps if error or time out happens, retry
			// TODO: Change steps
			for curState := 0; curState < len(storage.StdChunkStateFlow); {
				select {
				case chunkRes := <-chunkInfo.RetryChan:
					if !chunkRes.Succeed {
						// receiving session time out signal, directly return
						if chunkRes.SessionTimeOutErr != nil {
							log.Error(chunkRes.SessionTimeOutErr)
							return
						}
						// if client itself has some error, no matter how many times it tries,
						// it will fail again, in this case, we don't need retry
						if chunkRes.ClientErr != nil {
							log.Error(chunkRes.ClientErr)
							sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, false, chunkRes.ClientErr)
							return
						}
						// if host error, retry
						if chunkRes.HostErr != nil {
							log.Error(chunkRes.HostErr)
							// increment current host's retry times
							candidateHost.IncrementRetry()
							// if reach retry limit, in retry process will select another host
							// so in channel receiving should also return to 'init'
							curState = storage.InitState
							go retryProcess(ctx, api, candidateHost, ss, n, halfSignedContract, chunkHash, ssID, test)
						}
					} else {
						// if success with current state, move on to next
						log.Debug("succeed to pass state ", storage.StdChunkStateFlow[curState].State)
						// upload status change happens when one of the chunks turning from init to upload
						// only the first chunk changing state from init can change session status
						if curState == storage.InitState && ss.CompareAndSwap(storage.InitStatus, storage.UploadStatus) {
							// notice monitor current init status finish and to start calculating upload timeout
							sendSessionStatusChan(ss.SessionStatusChan, storage.InitStatus, true, nil)
						}
						curState = chunkRes.CurrentStep + 1
						if curState <= storage.CompleteState {
							chunkInfo.SetState(curState)
						}
					}
				case <-time.After(storage.StdChunkStateFlow[curState].TimeOut):
					{
						log.Errorf("Time Out on %s with state %s", chunkHash, storage.StdChunkStateFlow[curState])
						curState = storage.InitState // reconnect to the host to start over
						candidateHost.IncrementRetry()
						go retryProcess(ctx, api, candidateHost, ss, n, halfSignedContract, chunkHash, ssID, test)
					}
				}
			}
		}(chunkHash, chunkInfo)
	}
}

func sendSessionStatusChan(channel chan storage.StatusChan, status int, succeed bool, err error) {
	channel <- storage.StatusChan{
		CurrentStep: status,
		Succeed:     succeed,
		Err:         err,
	}
}

func sendStepStateChan(channel chan *storage.StepRetryChan, state int, succeed bool, clientErr error, hostErr error) {
	channel <- &storage.StepRetryChan{
		CurrentStep: state,
		Succeed:     succeed,
		ClientErr:   clientErr,
		HostErr:     hostErr,
	}
}

func retryProcess(ctx context.Context, api coreiface.CoreAPI, candidateHost *storage.HostNode, ss *storage.Session,
	n *core.IpfsNode, halfSignedContract []byte, chunkHash string, ssID string, test bool) {
	// record chunk info in session
	chunk := ss.GetOrDefault(chunkHash)

	// if candidate host passed in is not valid, fetch next valid one
	if candidateHost == nil || candidateHost.FailTimes >= FailLimit || candidateHost.RetryTimes >= RetryLimit {
		otherValidHost, err := getValidHost(ctx, ss.RetryQueue, api, n, test)
		// either retry queue is empty or something wrong with retry queue
		if err != nil {
			sendStepStateChan(chunk.RetryChan, storage.InitState, false, fmt.Errorf("no host available %v", err), nil)
			return
		}
		candidateHost = otherValidHost
	}

	// parse candidate host's IP and get connected
	_, hostPid, err := ParsePeerParam(candidateHost.Identity)
	if err != nil {
		sendStepStateChan(chunk.RetryChan, storage.InitState, false, err, nil)
		return
	}

	chunk.UpdateChunk(n.Identity, hostPid, candidateHost.Price)
	chunk.SetState(storage.InitState)
	// send over contract
	_, err = p2pCall(n, hostPid, "/storage/upload/init",
		ssID,
		ss.GetFileHash().String(),
		chunkHash,
		strconv.FormatInt(candidateHost.Price, 10),
		halfSignedContract)
	// fail to connect with retry
	if err != nil {
		sendStepStateChan(chunk.RetryChan, storage.InitState, false, nil, err)
	} else {
		sendStepStateChan(chunk.RetryChan, storage.InitState, true, nil, nil)
	}
}

// find next available host
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
			pi, err := findPeer(ctx, n, nextHost.Identity)
			if err != nil {
				// it's normal to fail in finding peer,
				// would give host another chance
				nextHost.IncrementFail()
				log.Error(err)
				err = retryQueue.Offer(nextHost)
				if err != nil {
					return nil, err
				}
				continue
			}
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

func findPeer(ctx context.Context, n *core.IpfsNode, pid string) (*peer.AddrInfo, error) {
	id, err := peer.IDB58Decode(pid)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	pinfo, err := n.Routing.FindPeer(ctx, id)
	if err != nil {
		return nil, err
	}
	return &pinfo, nil
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
		cmds.StringArg("contract", true, false, "Signed Contract."),
		cmds.StringArg("session-id", true, false, "session ID which render used to store all chunksInfo"),
		cmds.StringArg("chunk-hash", true, false, "Chunk the storage node should fetch."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		// receive contracts
		signedContract := []byte(req.Arguments[0])
		ssID := req.Arguments[1]
		ss, err := storage.GlobalSession.GetSession(ssID)
		if err != nil {
			return err
		}
		chunkHash := req.Arguments[2]
		err = ss.IncrementContract(chunkHash, signedContract)
		if err != nil {
			return err
		}
		// TODO: Modify client err return status
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		var contractRequest *escrowpb.EscrowContractRequest

		if ss.GetCompleteContractNum() == len(ss.ChunkInfo) {
			contracts, price, err := storage.PrepareContractFromChunk(ss.ChunkInfo)
			if err != nil {
				return err
			}
			contractRequest, err = escrow.NewContractRequest(cfg, contracts, price)
			if err != nil {
				log.Error(err)
				return err
			}
			submitContractRes, err := escrow.SubmitContractToEscrow(cfg, contractRequest)
			if err != nil {
				return err
			}
			go payFullToEscrow(submitContractRes, cfg)
		}
		return nil
	},
}

func payFullToEscrow(response *escrowpb.SignedSubmitContractResult, configuration *config.Config) {
	privKeyStr := configuration.Identity.PrivKey
	payerPrivKey, err := crypto.ToPrivKey(privKeyStr)
	if err != nil {
		log.Error(err)
		return
	}
	payerPubKey := payerPrivKey.GetPublic()
	payinRequest, err := escrow.NewPayinRequest(response, payerPubKey, payerPrivKey)
	if err != nil {
		log.Error(err)
		return
	}
	err = escrow.PayInToEscrow(configuration, payinRequest)
	if err != nil {
		log.Error(err)
		return
	}
}

var storageUploadProofCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "For client to receive collateral proof.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("proof", true, false, "Collateral Proof."),
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("chunk-hash", true, false, "Chunk the storage node should fetch."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ssID := req.Arguments[1]
		ss, err := storage.GlobalSession.GetSession(ssID)
		if err != nil {
			return err
		}
		chunkHash := req.Arguments[2]
		// get info from session
		// previous step should have information with channel id and price
		chunkInfo, err := ss.GetChunk(chunkHash)
		if err != nil {
			if chunkInfo != nil {
				sendStepStateChan(chunkInfo.RetryChan, storage.CompleteState, false, err, nil)
			}
			return err
		}
		chunkInfo.SetProof(req.Arguments[0])
		ss.UpdateCompleteChunkNum(1)
		sendStepStateChan(chunkInfo.RetryChan, storage.CompleteState, true, nil, nil)

		// check whether all chunk is complete
		if ss.GetCompleteChunks() == len(ss.ChunkInfo) {
			// only if all chunks upload success, send the signal to finish current status
			sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, true, nil)
		}
		return nil
	},
}

type UploadRes struct {
	ID string
}

type ChunkRes struct {
	Hash string
	Err  error
}

var storageUploadInitCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Initialize storage handshake with inquiring client.",
		ShortDescription: `
Storage host opens this endpoint to accept incoming upload/storage requests,
If current host is interested and all validation checks out, host downloads
the chunk and replies back to client for the next challenge step.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("file-hash", true, false, "Root file storage node should fetch (the DAG)."),
		cmds.StringArg("chunk-hash", true, false, "Chunk the storage node should fetch."),
		cmds.StringArg("price", true, false, "Price per GB in BTT for storing this chunk offered by client."),
		cmds.StringArg("contract", true, false, "Client's initial contract data."),
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
		chunkHash := req.Arguments[2]
		price, err := strconv.ParseInt(req.Arguments[3], 10, 64)
		if err != nil {
			return err
		}
		halfSignedCont := req.Arguments[4]
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		pid, ok := remote.GetStreamRequestRemotePeerID(req, n)
		if !ok {
			return fmt.Errorf("fail to get peer ID from request")
		}

		// build session
		sm := storage.GlobalSession
		ss := sm.GetOrDefault(ssID)
		ss.SetFileHash(fileHash)
		ss.SetStatus(storage.InitStatus)
		go controlSessionTimeout(ss)
		chunkInfo := ss.GetOrDefault(chunkHash)
		chunkInfo.UpdateChunk(pid, n.Identity, price)
		chunkInfo.SetState(storage.InitState)

		sendSessionStatusChan(ss.SessionStatusChan, storage.InitStatus, true, nil)
		// review contract and send back to client
		go reviewContractAndSign(chunkInfo, chunkHash, ssID, n, pid, req, env, []byte(halfSignedCont))
		//go downloadChunkFromClient(chunkInfo, chunkHash, ssID, n, pid, req, env)
		return nil
	},
}

func reviewContractAndSign(chunkInfo *storage.Chunk, chunkHash string, ssID string, n *core.IpfsNode,
	pid peer.ID, req *cmds.Request, env cmds.Environment, payerSig []byte) {
	// TODO: Check if renter is paid, if so, download file
	go downloadChunkFromClient(chunkInfo, chunkHash, ssID, n, pid, req, env)
	// review contract
	halfSignedContract, err := escrow.UnmarshalEscrowContract(payerSig)
	if err != nil {
		log.Error(err)
		return
	}
	contract := halfSignedContract.GetContract()
	// get render's public key
	payerPubKey, err := pid.ExtractPublicKey()
	if err != nil {
		log.Error(err)
		return
	}
	ok, err := crypto.Verify(payerPubKey, contract, halfSignedContract.GetBuyerSignature())
	if !ok {
		log.Error("Can't verify contract")
		return
	}
	// Sign on the contract
	signedContract, err := escrow.SignContractAndMarshal(contract, halfSignedContract, n.PrivateKey, false)
	if err != nil {
		log.Error(err)
		return
	}
	_, err = p2pCall(n, pid, "/storage/upload/recvcontract", signedContract, ssID, chunkHash)
	if err != nil {
		log.Error(err)
		return
	}
}

// call escrow service to check if payment is received or not
func periodicallyCheckPaymentFromClient() {
	var isReceivedWithTimeOut bool
	// TODO: isReceivedWithTimeOut := escrow.pay()
	if !isReceivedWithTimeOut {
		//TODO: delete file
	}
}

func downloadChunkFromClient(chunkInfo *storage.Chunk, chunkHash string, ssID string, n *core.IpfsNode, pid peer.ID, req *cmds.Request, env cmds.Environment) {
	sm := storage.GlobalSession
	ss := sm.GetOrDefault(ssID)

	chunkInfo.SetState(storage.UploadState)
	api, err := cmdenv.GetApi(env, req)
	if err != nil {
		log.Error(err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, false, err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	p := path.New(chunkHash)
	file, err := api.Unixfs().Get(context.Background(), p, false)
	if err != nil {
		log.Error(err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, false, err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	_, err = fileArchive(file, p.String(), false, gzip.NoCompression)
	if err != nil {
		log.Error(err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, false, err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}

	// RemoteCall(user, hash) to api/v0/storage/upload/reqc to get chid and ch
	chunkInfo.SetState(storage.ChallengeState)
	reqcBody, err := p2pCall(n, pid, "/storage/upload/reqc", ssID, chunkHash)
	if err != nil {
		log.Error(err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, false, err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	go solveChallenge(chunkInfo, chunkHash, ssID, reqcBody, n, pid, api, req)
}

func solveChallenge(chunkInfo *storage.Chunk, chunkHash string, ssID string, resBytes []byte, n *core.IpfsNode, pid peer.ID, api coreiface.CoreAPI, req *cmds.Request) {
	sm := storage.GlobalSession
	ss := sm.GetOrDefault(ssID)

	// get challenge object from params
	r := ChallengeRes{}
	if err := json.Unmarshal(resBytes, &r); err != nil {
		log.Error(err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, false, err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	// find chunk hash cid
	chunkCid, err := cidlib.Parse(chunkHash)
	if err != nil {
		log.Error(err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, false, err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	// compute challenge on host
	chunkInfo.SetState(storage.SolveState)
	sc, err := storage.NewStorageChallengeResponse(context.Background(), n, api, ss.GetFileHash(), chunkCid, r.ID)
	if err != nil {
		log.Error(err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, false, err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	if err := sc.SolveChallenge(r.ChunkIndex, r.Nonce); err != nil {
		log.Error(err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, false, err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	// update session to store challenge info there
	chunkInfo.UpdateChallenge(sc)

	// RemoteCall(user, CHID, CHR) to get signedPayment
	chunkInfo.SetState(storage.VerifyState)
	//signedPaymentBody, err := p2pCall(n, pid, "/storage/upload/respc", ssID, sc.Hash, chunkHash)
	//if err != nil {
	//	log.Error(err)
	//	sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, false, err)
	//	storage.GlobalSession.Remove(ssID, chunkHash)
	//	return
	//}
	//go completePayment(chunkInfo, chunkHash, ssID, signedPaymentBody, n, pid)
}

func completePayment(chunkInfo *storage.Chunk, chunkHash string, ssID string, resBytes []byte, n *core.IpfsNode, pid peer.ID) {
	sm := storage.GlobalSession
	ss := sm.GetOrDefault(ssID)

	payment := PaymentRes{}
	if err := json.Unmarshal(resBytes, &payment); err != nil {
		log.Error(err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, false, err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	var halfSignedChannelState ledgerpb.SignedChannelState
	err := proto.Unmarshal(payment.SignedPayment, &halfSignedChannelState)
	if err != nil {
		log.Error(err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, false, err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}

	chunkInfo.SetState(storage.CompleteState)
	_, err = p2pCall(n, pid, "/storage/upload/proof", "proof", ssID, chunkHash)
	if err != nil {
		log.Error(err)
		sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, false, err)
		return
	}
	sendSessionStatusChan(ss.SessionStatusChan, storage.UploadStatus, true, nil)
}

type ChallengeRes struct {
	ID         string
	ChunkIndex int
	Nonce      string
}

// TODO: refactor the code for guard to use
var storageUploadRequestChallengeCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Request for a client challenge from storage host.",
		ShortDescription: `
Client opens this endpoint for interested hosts to ask for a challenge.
A challenge contains a random file chunk hash and a nonce for hosts to hash
the contents and nonce together to produce a final challenge response.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session."),
		cmds.StringArg("chunk-hash", true, false, "Chunk the storage node should fetch."),
	},
	RunTimeout: 3 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ssID := req.Arguments[0]
		ss, err := storage.GlobalSession.GetSession(ssID)
		if err != nil {
			return err
		}
		chunkHash := req.Arguments[1]
		// previous step should have information with channel id and price
		chunkInfo, err := ss.GetChunk(chunkHash)
		if err != nil {
			return err
		}
		// if client receive this call, means at least finish upload state
		sendStepStateChan(chunkInfo.RetryChan, storage.UploadState, true, nil, nil)

		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			sendStepStateChan(chunkInfo.RetryChan, storage.ChallengeState, false, err, nil)
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			err := fmt.Errorf("storage client api not enabled")
			sendStepStateChan(chunkInfo.RetryChan, storage.ChallengeState, false, err, nil)
			return err
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			sendStepStateChan(chunkInfo.RetryChan, storage.ChallengeState, false, err, nil)
			return err
		}
		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			sendStepStateChan(chunkInfo.RetryChan, storage.ChallengeState, false, err, nil)
			return err
		}
		cid, err := cidlib.Parse(chunkHash)
		if err != nil {
			sendStepStateChan(chunkInfo.RetryChan, storage.ChallengeState, false, err, nil)
			return err
		}

		// when multi-process talking to multiple hosts, different cids can only generate one storage challenge,
		// and stored the latest one in session map
		sch, err := chunkInfo.SetChallenge(req.Context, n, api, ss.FileHash, cid)
		if err != nil {
			sendStepStateChan(chunkInfo.RetryChan, storage.ChallengeState, false, err, nil)
			return err
		}
		// challenge state finish, and waits for host to solve challenge
		sendStepStateChan(chunkInfo.RetryChan, storage.ChallengeState, true, nil, nil)

		out := &ChallengeRes{
			ID:         sch.ID,
			ChunkIndex: sch.CIndex,
			Nonce:      sch.Nonce,
		}
		return cmds.EmitOnce(res, out)
	},
	Type: ChallengeRes{},
}

//  TODO: refactor the code for guard to use
var storageUploadResponseChallengeCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Respond to client challenge from storage host.",
		ShortDescription: `
Client opens this endpoint for interested hosts to respond with a previous
challenge's response. If response is valid, client returns signed payment
signature back to the host to complete payment.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "Chunk the storage node should fetch."),
		cmds.StringArg("challenge-hash", true, false, "Challenge response back to uploader."),
		cmds.StringArg("chunk-hash", true, false, "Chunk the storage node should fetch."),
	},
	RunTimeout: 3 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		ssID := req.Arguments[0]
		ss, err := storage.GlobalSession.GetSession(ssID)
		if err != nil {
			return err
		}

		challengeHash := req.Arguments[1]
		chunkHash := req.Arguments[2]
		// get info from session
		// previous step should have information with channel id and price
		chunkInfo, err := ss.GetChunk(chunkHash)
		if err != nil {
			return err
		}

		// pre-check
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			sendStepStateChan(chunkInfo.RetryChan, storage.SolveState, false, err, nil)
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			sendStepStateChan(chunkInfo.RetryChan, storage.SolveState, false, err, nil)
			return fmt.Errorf("storage client api not enabled")
		}

		// time out check
		// the time host solved the challenge,
		// is the time we used call challengeTimeOut
		// if client didn't receive succeed state in time,
		// monitor would notice the timeout
		sendStepStateChan(chunkInfo.RetryChan, storage.SolveState, true, nil, nil)
		// verify challenge
		if chunkInfo.Challenge.Hash != challengeHash {
			err := fmt.Errorf("fail to verify challenge")
			sendStepStateChan(chunkInfo.RetryChan, storage.VerifyState, false, nil, err)
			return err
		}
		sendStepStateChan(chunkInfo.RetryChan, storage.VerifyState, true, nil, nil)

		// prepare payment
		//n, err := cmdenv.GetNode(env)
		//if err != nil {
		//	sendStepStateChan(chunkInfo.RetryChan, storage.PaymentState, false, err, nil)
		//	return err
		//}
		//pid, ok := remote.GetStreamRequestRemotePeerID(req, n)
		//if !ok {
		//	err := fmt.Errorf("fail to get peer ID from request")
		//	sendStepStateChan(chunkInfo.RetryChan, storage.PaymentState, false, err, nil)
		//	return err
		//}

		//channelState, err := prepareChannelState(n, pid, chunkInfo.Price, chunkInfo.ChannelID)
		//if err != nil {
		//	sendStepStateChan(chunkInfo.RetryChan, storage.PaymentState, false, err, nil)
		//	return err
		//}
		//
		//signedPayment, err := signChannelState(n.PrivateKey, channelState)
		//if err != nil {
		//	sendStepStateChan(chunkInfo.RetryChan, storage.PaymentState, false, err, nil)
		//	return err
		//}
		//
		//signedBytes, err := proto.Marshal(signedPayment)
		//if err != nil {
		//	sendStepStateChan(chunkInfo.RetryChan, storage.PaymentState, false, err, nil)
		//	return err
		//}

		// from client's perspective, prepared payment finished
		// but the actual payment does not.
		// only the complete state timeOut or receiving error means
		// host having trouble with either agreeing on payment or closing channel
		sendStepStateChan(chunkInfo.RetryChan, storage.PaymentState, true, nil, nil)
		r := &PaymentRes{}
		return cmds.EmitOnce(res, r)
	},
	Type: PaymentRes{},
}

type PaymentRes struct {
	SignedPayment []byte
}

func p2pCall(n *core.IpfsNode, pid peer.ID, api string, arg ...interface{}) ([]byte, error) {
	remoteCall := &remote.P2PRemoteCall{
		Node: n,
		ID:   pid,
	}
	return remoteCall.CallGet(api, arg)
}

var storageHostsCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Interact with information on hosts.",
		ShortDescription: `
Host information is synchronized from btfs-hub and saved in local datastore.`,
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

Mode options include:
- "score": top overall score
- "geo":   closest location
- "rep":   highest reputation
- "price": lowest price
- "speed": highest transfer speed
- "all":   all existing hosts`,
	},
	Options: []cmds.Option{
		cmds.StringOption(hostInfoModeOptionName, "m", "Hosts info showing mode.").WithDefault(hub.HubModeAll),
	},
	PreRun: func(req *cmds.Request, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		mode, _ := req.Options[hostInfoModeOptionName].(string)
		return hub.CheckValidMode(mode)
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
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

Mode options include:
- "score": top overall score
- "geo":   closest location
- "rep":   highest reputation
- "price": lowest price
- "speed": highest transfer speed
- "all":   update existing hosts`,
	},
	Options: []cmds.Option{
		cmds.StringOption(hostSyncModeOptionName, "m", "Hosts syncing mode.").WithDefault(hub.HubModeScore),
	},
	PreRun: func(req *cmds.Request, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		mode, _ := req.Options[hostSyncModeOptionName].(string)
		return hub.CheckValidMode(mode)
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		mode, _ := req.Options[hostSyncModeOptionName].(string)

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
			// TODO: Implement syncing other peers' storage info
			return fmt.Errorf("showing other peer's info not supported yet")
		} else if !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}

		var peerID string

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		// Default to self
		if len(req.Arguments) > 0 {
			peerID = req.Arguments[0]
		} else {
			peerID = n.Identity.Pretty()
		}

		rds := n.Repo.Datastore()

		var ns *hubpb.SettingsData
		b, err := rds.Get(storage.GetHostStorageKey(peerID))
		if err != nil {
			var resp *hubpb.SettingsResp
			err := grpc.HubQueryClient(cfg.Services.HubDomain).WithContext(context.Background(),
				func(ctx context.Context, client hubpb.HubQueryServiceClient) error {
					req := new(hubpb.SettingsReq)
					req.Id = peerID
					resp, err = client.GetSettings(ctx, req)
					return err
				})
			if err != nil {
				return err
			}
			ns = resp.SettingsData
		} else {
			err = json.Unmarshal(b, &ns)
		}

		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, ns)
	},
	Type: hubpb.SettingsData{},
}

var storageAnnounceCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Update and announce storage host information.",
		ShortDescription: `
This command updates host information and broadcasts to the BTFS network.`,
	},
	Options: []cmds.Option{
		cmds.Uint64Option(hostStoragePriceOptionName, "s", "Max price per GB of storage in BTT."),
		cmds.Uint64Option(hostBandwidthPriceOptionName, "b", "Max price per MB of bandwidth in BTT."),
		cmds.Uint64Option(hostCollateralPriceOptionName, "cl", "Max collateral stake per hour per GB in BTT."),
		cmds.FloatOption(hostBandwidthLimitOptionName, "l", "Max bandwidth limit per MB/s."),
		cmds.Uint64Option(hostStorageTimeMinOptionName, "d", "Min number of days for storage."),
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

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		rds := n.Repo.Datastore()

		selfKey := storage.GetHostStorageKey(n.Identity.Pretty())
		b, err := rds.Get(selfKey)
		// If key not found, create new
		if err != nil && err != ds.ErrNotFound {
			return err
		}

		var ns nodepb.Node_Settings
		if err == nil {
			// TODO: Set default values if unset
			err = json.Unmarshal(b, &ns)
			if err != nil {
				return err
			}
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

		nb, err := json.Marshal(ns)
		if err != nil {
			return err
		}

		err = rds.Put(selfKey, nb)
		if err != nil {
			return err
		}

		return nil
	},
}

type StatusRes struct {
	Status   string
	FileHash string
	Chunks   map[string]*ChunkStatus
}

type ChunkStatus struct {
	Price  int64
	Host   string
	Status string
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
		ss, err := storage.GlobalSession.GetSession(ssID)
		if err != nil {
			return err
		}

		// check if checking request from host or client
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled || !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage client/host api not enabled")
		}

		// get chunks info from session
		status.Status = ss.GetStatus()
		status.FileHash = ss.GetFileHash().String()
		chunks := make(map[string]*ChunkStatus)
		status.Chunks = chunks
		for hash, info := range ss.ChunkInfo {
			c := &ChunkStatus{
				Price:  info.GetPrice(),
				Host:   info.Receiver.String(),
				Status: info.GetState(),
			}
			chunks[hash] = c
		}
		return res.Emit(status)
	},
	Type: StatusRes{},
}
