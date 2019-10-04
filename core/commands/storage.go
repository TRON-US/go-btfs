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
	"github.com/TRON-US/go-btfs/core/hub"
	"github.com/TRON-US/go-btfs/core/ledger"
	ledgerPb "github.com/TRON-US/go-btfs/core/ledger/pb"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/gogo/protobuf/proto"
	cidlib "github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/path"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/tron-us/go-btfs-common/info"
)

const (
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

	hostStorePrefix       = "/hosts/"        // from btfs-hub
	hostStorageInfoPrefix = "/host_storage/" // self or from network

	defaultRepFactor = 3
	// session status
	initStatus     = "init"
	uploadStatus   = "upload"
	completeStatus = "complete"
	errStatus      = "error"

	// chunk state
	initState      = 0
	uploadState    = 1
	challengeState = 2
	solveState     = 3
	verifyState    = 4
	paymentState   = 5
	completeState  = 6

	// retry limit
	RetryLimit = 3
	FailLimit  = 3
)

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
To upload and store a file on specific hosts:
    use -m with 'custom' mode, and put host identifiers in -l, with multiple hosts separated by ','

For example:

    btfs storage upload -m=custom -l=host_address1,host_address2

If no hosts are given, BTFS will select nodes based on overall score according to current client's environment.

Receive proofs as collateral evidence after selected nodes agree to store the file.`,
	},
	Subcommands: map[string]*cmds.Command{
		"init":   storageUploadInitCmd,
		"reqc":   storageUploadRequestChallengeCmd,
		"respc":  storageUploadResponseChallengeCmd,
		"status": storageUploadStatusCmd,
		"proof":  storageUploadProofCmd,
	},
	Arguments: []cmds.Argument{
		// FIXME: change file hash to limit 1
		cmds.StringArg("file-hash", true, true, "Add hash of file to upload.").EnableStdin(),
	},
	Options: []cmds.Option{
		cmds.Int64Option(uploadPriceOptionName, "p", "Max price per GB of storage in BTT."),
		cmds.Int64Option(replicationFactorOptionName, "r", "Replication factor for the file with erasure coding built-in.").WithDefault(defaultRepFactor),
		cmds.StringOption(hostSelectModeOptionName, "m", "Based on mode to select the host and upload automatically.").WithDefault("score"),
		cmds.StringOption(hostSelectionOptionName, "l", "Use only these hosts in order on 'custom' mode. Use ',' as delimiter."),
	},
	RunTimeout: 5 * time.Minute, // TODO: handle large file uploads?
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		p, found := req.Options[uploadPriceOptionName].(int64)
		if found && p < 0 {
			return fmt.Errorf("cannot input a negative price")
		} else if !found {
			// TODO: Select best price from top candidates
			p = int64(1)
		}

		mode, _ := req.Options[hostSelectModeOptionName].(string)
		hosts, found := req.Options[hostSelectionOptionName].(string)
		if mode == "custom" && !found {
			return fmt.Errorf("custom mode needs input host lists")
		}
		fileHash := req.Arguments[0]
		peers := strings.Split(hosts, ",")

		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		// start new session
		sm := storage.GlobalSession
		ssID, err := storage.NewSessionID()
		if err != nil {
			return err
		}
		ss := sm.GetOrDefault(ssID)
		ss.SetFileHash(fileHash)
		ss.SetStatus(initStatus)

		// init retry queue
		retryQueue := storage.New(int64(len(peers)))
		// FIXME: getting all hosts ip from hub, for now it is from input
		hostList := &storage.Hosts{}
		for i, ip := range peers {
			host := &storage.HostNode{
				Identity:   ip,
				Score:      retryQueue.Size() - int64(i),
				RetryTimes: 0,
				FailTimes:  0,
			}
			hostList.Add(host)
		}
		err = retryQueue.AddAll(*hostList)
		if err != nil {
			return err
		}
		// retry queue need to be reused in proof cmd
		ss.SetRetryQueue(retryQueue)

		// get self key pair
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		// get other node's public key as address
		// create channel to receive error if server don't have response
		//chunkChan := make(chan *ChunkRes)
		// FIXME: Fake multiple chunk hash as chunk hash
		ss.SetStatus(uploadStatus)
		// add chunks into session
		for _, singleChunk := range req.Arguments {
			ss.GetOrDefault(singleChunk)
		}
		go retryMinotor(ss, n, p, ssID, context.Background())

		seRes := &UploadRes{
			ID: ssID,
		}
		return res.Emit(seRes)
	},
	Type: UploadRes{},
}

func retryMinotor(ss *storage.Session, n *core.IpfsNode, p int64, ssID string, ctx context.Context) {
	retryQueue := ss.GetRetryQueue()
	if retryQueue == nil {
		log.Panic("retry queue is nil")
	}
	// loop over each chunk
	for chunkHash, chunkInfo := range ss.ChunkInfo {
		go func(chunkHash string, chunkInfo *storage.Chunk) {
			candidateHost, err := getValidHost(retryQueue)
			if err != nil {
				ss.SetStatus(errStatus)
				return
			}

			// build connection with host, init step, could be error or timeout
			go retryProcess(candidateHost, ss, n, p, chunkHash, ssID, ctx)

			// monitor each steps if error or time out happens, retry
			for curState := 0; curState < len(storage.StdChunkStateFlow); {
				select {
				case chunkRes := <-chunkInfo.RetryChan:
					if !chunkRes.Succeed {
						// if client itself has some error, no matter how many times it tries,
						// it will fail again, in this case, we don't need retry
						if chunkRes.ClientErr != nil {
							log.Error(chunkRes.ClientErr)
							ss.SetStatus(errStatus)
							return
						}
						// if host error, retry
						if chunkRes.HostErr != nil {
							// increment current host's retry times
							candidateHost.IncrementRetry()
							// if reach retry limit, in retry process will select another host
							// so in channel receiving should also return to 'init'
							if candidateHost.GetRetryTimes() >= RetryLimit {
								curState = initState
								go retryProcess(candidateHost, ss, n, p, chunkHash, ssID, ctx)
							} else {
								// TODO: How to let single step retry?
								curState = chunkRes.CurrentStep
							}
						}
					} else {
						// if success with current state, move on to next
						// log.Error("succeed to pass state ", storage.StdChunkStateFlow[curState].State)
						curState = chunkRes.CurrentStep + 1
						chunkInfo.SetState(storage.StdChunkStateFlow[curState].State)
					}
				case <-time.After(storage.StdChunkStateFlow[curState].TimeOut):
					{
						log.Errorf("Time Out on %s with state %s", chunkHash, storage.StdChunkStateFlow[curState])
						curState = initState // reconnect to the host to start over
						candidateHost.IncrementRetry()
						go retryProcess(candidateHost, ss, n, p, chunkHash, ssID, ctx)
					}
				}
			}
		}(chunkHash, chunkInfo)
	}
}

func retryProcess(candidateHost *storage.HostNode, ss *storage.Session, n *core.IpfsNode, p int64, chunkHash string, ssID string, ctx context.Context) {
	// record chunk info in session
	chunk := ss.GetOrDefault(chunkHash)

	// if candidate host passed in is not valid, fetch next valid one
	if candidateHost == nil || candidateHost.FailTimes >= FailLimit || candidateHost.RetryTimes >= RetryLimit {
		otherValidHost, err := getValidHost(ss.RetryQueue)
		// either retry queue is empty or something wrong with retry queue
		if err != nil {
			chunk.RetryChan <- &storage.StepRetryChan{
				CurrentStep: initState,
				Succeed: false,
				ClientErr: fmt.Errorf("no host available %v", err),
			}
			return
		}
		candidateHost = otherValidHost
	}
	// get node's key pair
	selfPrivKey := n.PrivateKey
	selfPubKey := selfPrivKey.GetPublic()

	// parse candidate host's IP and get connected
	_, hostPid, err := ParsePeerParam(candidateHost.Identity)
	if err != nil {
		log.Error(err)
		chunk.RetryChan <- &storage.StepRetryChan{
			CurrentStep: initState,
			Succeed: false,
			ClientErr: fmt.Errorf("fail to parse host id %v", err),
		}
		return
	}
	peerPubKey, err := hostPid.ExtractPublicKey()
	if err != nil {
		chunk.RetryChan <- &storage.StepRetryChan{
			CurrentStep: initState,
			Succeed: false,
			ClientErr: fmt.Errorf("fail to extract public key %v", err),
		}
		return
	}
	// establish connection with ledger
	channelID, err := initChannel(ctx, selfPubKey, selfPrivKey, peerPubKey, p)
	if err != nil {
		chunk.RetryChan <- &storage.StepRetryChan{
			CurrentStep: initState,
			Succeed: false,
			ClientErr: fmt.Errorf("fail to connect with ledger %v", err),
		}
		return
	}

	chunk.NewChunk(n.Identity, hostPid, channelID, p)
	chunk.SetState(storage.StdChunkStateFlow[0].State)

	_, err = p2pCall(n, hostPid, "/storage/upload/init", ssID, strconv.FormatInt(channelID.Id, 10), chunkHash, strconv.FormatInt(p, 10))
	// fail to connect with retry
	if err != nil {
		chunk.RetryChan <- &storage.StepRetryChan{
			CurrentStep: initState,
			Succeed: false,
			HostErr: fmt.Errorf("fail on host %s with %d", hostPid, candidateHost.RetryTimes),
		}
	} else {
		chunk.RetryChan <- &storage.StepRetryChan{
			CurrentStep:initState, // init state success
			Succeed: true,
		}
	}
}

// find next available host
func getValidHost(retryQueue *storage.RetryQueue) (*storage.HostNode, error) {
	var candidateHost *storage.HostNode
	for ; candidateHost == nil; {
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
			candidateHost = nextHost
		}
	}
	return candidateHost, nil
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
				chunkInfo.RetryChan <- &storage.StepRetryChan{
					CurrentStep: completeState,
					Succeed: false,
					ClientErr: err, // client can't get chunkInfo
				}
			}
			return err
		}
		chunkInfo.SetProof(req.Arguments[0])
		ss.UpdateCompleteChunkNum(1)
		chunkInfo.RetryChan <- &storage.StepRetryChan{
			CurrentStep: completeState,
			Succeed: true,
		}
		// check whether all chunk is complete
		if ss.GetCompleteChunks() == len(ss.ChunkInfo) {
			ss.SetStatus(completeStatus)
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
		cmds.StringArg("channel-id", true, false, "Open channel id for payment."),
		cmds.StringArg("chunk-hash", true, false, "Chunk the storage node should fetch."),
		cmds.StringArg("price", true, false, "Price per GB in BTT for storing this chunk offered by client."),
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
		channelID := req.Arguments[1]
		chunkHash := req.Arguments[2]
		price, err := strconv.ParseInt(req.Arguments[3], 10, 64)
		if err != nil {
			return err
		}
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
		ss.SetStatus(initStatus)
		chidInt64, err := strconv.ParseInt(channelID, 10, 64)
		if err != nil {
			sm.Remove(ssID, "")
			return err
		}
		chID := &ledgerPb.ChannelID{
			Id: chidInt64,
		}
		chunkInfo := ss.GetOrDefault(chunkHash)
		chunkInfo.NewChunk(pid, n.Identity, chID, price)
		chunkInfo.SetState(storage.StdChunkStateFlow[initState].State)

		// build connection with ledger
		channelInfo, err := getChannelInfo(req.Context, channelID)
		if err != nil {
			sm.Remove(ssID, "")
			return err
		}
		log.Debug("Verified channel:", channelInfo)

		go downloadChunkFromClient(chunkInfo, chunkHash, ssID, n, pid, req, env)
		return nil
	},
}

func downloadChunkFromClient(chunkInfo *storage.Chunk, chunkHash string, ssID string, n *core.IpfsNode, pid peer.ID, req *cmds.Request, env cmds.Environment) {
	chunkInfo.SetState(storage.StdChunkStateFlow[uploadState].State)
	api, err := cmdenv.GetApi(env, req)
	if err != nil {
		log.Error(err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	p := path.New(chunkHash)
	file, err := api.Unixfs().Get(context.Background(), p)
	if err != nil {
		log.Error(err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	_, err = fileArchive(file, p.String(), false, gzip.NoCompression)
	if err != nil {
		log.Error(err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}

	// RemoteCall(user, hash) to api/v0/storage/upload/reqc to get chid and ch
	chunkInfo.SetState(storage.StdChunkStateFlow[challengeState].State)
	reqcBody, err := p2pCall(n, pid, "/storage/upload/reqc", ssID, chunkHash)
	if err != nil {
		log.Error(err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	go solveChallenge(chunkInfo, chunkHash, ssID, reqcBody, n, pid, api, req)
}

func solveChallenge(chunkInfo *storage.Chunk, chunkHash string, ssID string, resBytes []byte, n *core.IpfsNode, pid peer.ID, api coreiface.CoreAPI, req *cmds.Request) {
	r := ChallengeRes{}
	if err := json.Unmarshal(resBytes, &r); err != nil {
		log.Error(err)
		storage.GlobalSession.Remove(ssID, chunkHash)
	}
	// compute challenge on host
	chunkInfo.SetState(storage.StdChunkStateFlow[solveState].State)
	sc := storage.NewStorageChallengeResponse(req.Context, n, api, r.ID)
	hashToCid, err := cidlib.Parse(chunkHash)
	if err != nil {
		log.Error(err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	if err := sc.SolveChallenge(hashToCid, r.Nonce); err != nil {
		log.Error(err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	// update session to store challenge info there
	chunkInfo.UpdateChallenge(sc)

	// RemoteCall(user, CHID, CHR) to get signedPayment
	chunkInfo.SetState(storage.StdChunkStateFlow[verifyState].State)
	signedPaymentBody, err := p2pCall(n, pid, "/storage/upload/respc", ssID, r.Hash, chunkHash)
	if err != nil {
		log.Error(err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}

	go completePayment(chunkInfo, chunkHash, ssID, signedPaymentBody, n, pid, req)
}

func completePayment(chunkInfo *storage.Chunk, chunkHash string, ssID string, resBytes []byte, n *core.IpfsNode, pid peer.ID, req *cmds.Request) {
	payment := PaymentRes{}
	if err := json.Unmarshal(resBytes, &payment); err != nil {
		log.Error(err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}
	var halfSignedChannelState ledgerPb.SignedChannelState
	err := proto.Unmarshal(payment.SignedPayment, &halfSignedChannelState)
	if err != nil {
		log.Error(err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}

	// verify and sign
	chunkInfo.SetState(storage.StdChunkStateFlow[paymentState].State)
	signedchannelState, err := verifyAndSign(pid, n, &halfSignedChannelState)
	if err != nil {
		log.Error("fail to verify and sign", err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}

	// Close channel
	channelstate := signedchannelState.GetChannel()
	log.Debug("channel state before closing: %v", channelstate)
	// timeout mechanism with context cancel, can't reuse req.context here
	err = ledger.CloseChannel(context.Background(), signedchannelState)
	if err != nil {
		log.Error("fail to close channel", err)
		storage.GlobalSession.Remove(ssID, chunkHash)
		return
	}

	chunkInfo.SetState(storage.StdChunkStateFlow[completeState].State)
	_, err = p2pCall(n, pid, "/storage/upload/proof", "proof", ssID, chunkHash)
	if err != nil {
		log.Error(err)
		return
	}
}

type ChallengeRes struct {
	ID    string
	Hash  string
	Nonce string
}

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
		chunkInfo.RetryChan <- &storage.StepRetryChan{
			CurrentStep: uploadState,
			Succeed: true,
		}

		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: challengeState,
				Succeed: false,
				ClientErr:err,
			}
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			err := fmt.Errorf("storage client api not enabled")
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: challengeState,
				Succeed: false,
				ClientErr:err,
			}
			return err
		}

		n, err := cmdenv.GetNode(env)
		if err != nil {
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: challengeState,
				Succeed: false,
				ClientErr:err,
			}
			return err
		}
		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: challengeState,
				Succeed: false,
				ClientErr:err,
			}
			return err
		}
		cid, err := cidlib.Parse(chunkHash)
		if err != nil {
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: challengeState,
				Succeed: false,
				ClientErr:err,
			}
			return err
		}

		// when multi-process talking to multiple hosts, different cids can only generate one storage challenge,
		// and stored the latest one in session map
		sch, err := chunkInfo.SetChallenge(context.Background(), n, api, cid)
		if err != nil {
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: challengeState,
				Succeed: false,
				ClientErr:err,
			}
			return err
		}
		// challenge state finish, and waits for host to solve challenge
		chunkInfo.RetryChan <- &storage.StepRetryChan{
			CurrentStep: challengeState,
			Succeed: true,
		}

		//// set a time to verify challenge, since client cannot know the time host is solving
		//chunkInfo.SetTime(time.Now())
		out := &ChallengeRes{
			ID:    sch.ID,
			Hash:  sch.Hash,
			Nonce: sch.Nonce,
		}
		log.Error("challenge reqc finished")
		return res.Emit(out)
	},
	Type: ChallengeRes{},
}

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
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: solveState,
				Succeed: false,
				ClientErr:err,
			}
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: solveState,
				Succeed: false,
				ClientErr:err,
			}
			return fmt.Errorf("storage client api not enabled")
		}

		// time out check
		//now := time.Now()
		//if now.After(chunkInfo.GetTime().Add(challengeTimeOut)) {
		//	return fmt.Errorf("challenge verification time out")
		//}
		// the time host solved the challenge,
		// is the time we used call challengeTimeOut
		// if client didn't receive succeed state in time,
		// monitor would notice the timeout
		chunkInfo.RetryChan <- &storage.StepRetryChan{
			CurrentStep: solveState,
			Succeed: true,
		}
		// verify challenge
		if chunkInfo.Challenge.Hash != challengeHash {
			err := fmt.Errorf("fail to verify challenge")
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: verifyState,
				Succeed: false,
				HostErr: err,
			}
			return err
		}
		chunkInfo.RetryChan <- &storage.StepRetryChan{
			CurrentStep: verifyState,
			Succeed: true,
		}

		// prepare payment
		n, err := cmdenv.GetNode(env)
		if err != nil {
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: paymentState,
				Succeed: false,
				ClientErr:err,
			}
			return err
		}
		pid, ok := remote.GetStreamRequestRemotePeerID(req, n)
		if !ok {
			err := fmt.Errorf("fail to get peer ID from request")
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: paymentState,
				Succeed: false,
				ClientErr:err,
			}
			return err
		}

		channelState, err := prepareChannelState(n, pid, chunkInfo.Price, chunkInfo.ChannelID)
		if err != nil {
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: paymentState,
				Succeed: false,
				ClientErr:err,
			}
			return err
		}

		signedPayment, err := signChannelState(n.PrivateKey, channelState)
		if err != nil {
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: paymentState,
				Succeed: false,
				ClientErr:err,
			}
			return err
		}

		signedBytes, err := proto.Marshal(signedPayment)
		if err != nil {
			chunkInfo.RetryChan <- &storage.StepRetryChan{
				CurrentStep: paymentState,
				Succeed: false,
				ClientErr:err,
			}
			return err
		}

		// from client's perspective, prepared payment finished
		// but the actual payment does not.
		// only the complete state timeOut or receiving error means
		// host having trouble with either agreeing on payment or closing channel
		chunkInfo.RetryChan <- &storage.StepRetryChan{
			CurrentStep: paymentState,
			Succeed: true,
		}
		r := &PaymentRes{
			SignedPayment: signedBytes,
		}
		return res.Emit(r)
	},
	Type: PaymentRes{},
}

type PaymentRes struct {
	SignedPayment []byte
}

func p2pCall(n *core.IpfsNode, pid peer.ID, api string, arg ...string) ([]byte, error) {
	remoteCall := &remote.P2PRemoteCall{
		Node: n,
		ID:   pid,
	}
	return remoteCall.CallGet(api, arg)
}

func getChannelInfo(ctx context.Context, chanID string) (*ledgerPb.ChannelInfo, error) {
	clientConn, err := ledger.LedgerConnection()
	defer ledger.CloseConnection(clientConn)
	if err != nil {
		return nil, err
	}
	ledgerClient := ledger.NewClient(clientConn)
	cidInt64, err := strconv.ParseInt(chanID, 10, 64)
	if err != nil {
		return nil, err
	}
	channelID := ledgerPb.ChannelID{Id: cidInt64}
	return ledgerClient.GetChannelInfo(ctx, &channelID)
}

func verifyAndSign(pid peer.ID, n *core.IpfsNode, signedChannelState *ledgerPb.SignedChannelState) (*ledgerPb.SignedChannelState, error) {
	pk, err := pid.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	channelState := signedChannelState.GetChannel()
	ok, err := ledger.Verify(pk, channelState, signedChannelState.GetFromSignature())
	if err != nil || !ok {
		return nil, fmt.Errorf("fail to verify channel state, %v", err)
	}

	selfPrivKey := n.PrivateKey
	sig, err := ledger.Sign(selfPrivKey, channelState)
	if err != nil {
		return nil, err
	}
	signedChannelState.ToSignature = sig
	return signedChannelState, nil
}

func prepareChannelState(n *core.IpfsNode, pid peer.ID, price int64, channelID *ledgerPb.ChannelID) (*ledgerPb.ChannelState, error) {
	fromAccount, err := ledger.NewAccount(n.PrivateKey.GetPublic(), 0)
	if err != nil {
		return nil, err
	}
	toPubKey, err := pid.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	toAccount, err := ledger.NewAccount(toPubKey, price)
	if err != nil {
		return nil, err
	}
	// create channel state wait for both side to agree on
	return ledger.NewChannelState(channelID, 0, fromAccount, toAccount), nil
}

func signChannelState(privKey ic.PrivKey, channelState *ledgerPb.ChannelState) (*ledgerPb.SignedChannelState, error) {
	sig, err := ledger.Sign(privKey, channelState)
	if err != nil {
		return nil, err
	}
	return ledger.NewSignedChannelState(channelState, sig, nil), nil
}

func initChannel(ctx context.Context, payerPubKey ic.PubKey, payerPrivKey ic.PrivKey, recvPubKey ic.PubKey, amount int64) (*ledgerPb.ChannelID, error) {
	// build connection with ledger
	clientConn, err := ledger.LedgerConnection()
	defer ledger.CloseConnection(clientConn)
	if err != nil {
		return nil, err
	}
	// new ledger client
	ledgerClient := ledger.NewClient(clientConn)
	// create account
	_, err = ledger.ImportAccount(ctx, payerPubKey, ledgerClient)
	if err != nil {
		return nil, err
	}
	_, err = ledger.ImportAccount(ctx, recvPubKey, ledgerClient)
	if err != nil {
		return nil, err
	}
	// prepare channel commit and sign
	cc, err := ledger.NewChannelCommit(payerPubKey, recvPubKey, amount)
	if err != nil {
		return nil, err
	}
	sig, err := ledger.Sign(payerPrivKey, cc)
	if err != nil {
		return nil, err
	}
	return ledger.CreateChannel(ctx, ledgerClient, cc, sig)
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

		rds := n.Repo.Datastore()

		// All = display everything
		if mode == hub.HubModeAll {
			mode = ""
		}
		qr, err := rds.Query(query.Query{Prefix: hostStorePrefix + mode})
		if err != nil {
			return err
		}

		var nodes []*info.Node
		for r := range qr.Next() {
			if r.Error != nil {
				return r.Error
			}
			var ni info.Node
			err := json.Unmarshal(r.Entry.Value, &ni)
			if err != nil {
				return err
			}
			nodes = append(nodes, &ni)
		}

		return cmds.EmitOnce(res, &HostInfoRes{nodes})
	},
	Type: HostInfoRes{},
}

type HostInfoRes struct {
	Nodes []*info.Node
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

		nodes, err := hub.QueryHub(n.Identity.Pretty(), mode)
		if err != nil {
			return err
		}

		rds := n.Repo.Datastore()

		// Dumb strategy right now: remove all existing and add the new ones
		// TODO: Update by timestamp and only overwrite updated
		qr, err := rds.Query(query.Query{Prefix: hostStorePrefix + mode})
		if err != nil {
			return err
		}

		for r := range qr.Next() {
			if r.Error != nil {
				return r.Error
			}
			err := rds.Delete(ds.NewKey(r.Entry.Key))
			if err != nil {
				return err
			}
		}

		for _, ni := range nodes {
			b, err := json.Marshal(ni)
			if err != nil {
				return err
			}
			err = rds.Put(ds.NewKey(fmt.Sprintf("%s%s/%s", hostStorePrefix, mode, ni.NodeID)), b)
			if err != nil {
				return err
			}
		}

		return nil
	},
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

		b, err := rds.Get(ds.NewKey(fmt.Sprintf("%s%s", hostStorageInfoPrefix, peerID)))
		if err != nil {
			return err
		}

		var ns info.NodeStorage
		err = json.Unmarshal(b, &ns)
		if err != nil {
			return err
		}

		return cmds.EmitOnce(res, &StorageHostInfoRes{
			StoragePrice:    ns.StoragePriceAsk,
			BandwidthPrice:  ns.BandwidthPriceAsk,
			CollateralPrice: ns.CollateralStake,
			BandwidthLimit:  ns.BandwidthLimit,
			StorageTimeMin:  ns.StorageTimeMin,
		})
	},
	Type: StorageHostInfoRes{},
}

type StorageHostInfoRes struct {
	StoragePrice    uint64
	BandwidthPrice  uint64
	CollateralPrice uint64
	BandwidthLimit  float64
	StorageTimeMin  uint64
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

		selfKey := ds.NewKey(fmt.Sprintf("%s%s", hostStorageInfoPrefix, n.Identity.Pretty()))
		b, err := rds.Get(selfKey)
		// If key not found, create new
		if err != nil && err != ds.ErrNotFound {
			return err
		}

		var ns info.NodeStorage
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
		status.FileHash = ss.GetFileHash()
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
