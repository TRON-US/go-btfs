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
	cidlib "github.com/ipfs/go-cid"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"
	query "github.com/ipfs/go-datastore/query"
	"github.com/TRON-US/interface-go-btfs-core/path"
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

	challengeTimeOut = time.Second

	hostStorePrefix       = "/hosts/"        // from btfs-hub
	hostStorageInfoPrefix = "/host_storage/" // self or from network

	defaultRepFactor = 3
)

var (
	channelID *ledgerPb.ChannelID
	price     int64
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
		"init":  storageUploadInitCmd,
		"reqc":  storageUploadRequestChallengeCmd,
		"respc": storageUploadResponseChallengeCmd,
	},
	Arguments: []cmds.Argument{
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
		price, found := req.Options[uploadPriceOptionName].(int64)
		if found && price < 0 {
			return fmt.Errorf("cannot input a negative price")
		} else if !found {
			// TODO: Select best price from top candidates
			price = int64(1)
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
		ss := &storage.Session{}
		ssID, err := storage.NewSessionID()
		if err != nil {
			return err
		}
		if err = ss.NewSession(ssID, fileHash); err != nil {
			return err
		}
		// get self key pair
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		selfPrivKey := n.PrivateKey
		selfPubKey := selfPrivKey.GetPublic()

		// get other node's public key as address
		// create channel between them
		chunkChan := make(chan *storage.Chunks)
		// FIXME: Fake multiple chunk hash as chunk hash
		for i, chunk := range req.Arguments {
			go func(chunkHash string, i int) {
				_, hostPid, err := ParsePeerParam(peers[i])
				if err != nil {
					chunkChan <- &storage.Chunks{
						ChunkHash: chunkHash,
						Err:       err,
					}
					return
				}
				peerPubKey, err := hostPid.ExtractPublicKey()
				if err != nil {
					chunkChan <- &storage.Chunks{
						ChunkHash: chunkHash,
						Err:       err,
					}
					return
				}
				channelID, err = initChannel(req.Context, selfPubKey, selfPrivKey, peerPubKey, price)
				if err != nil {
					chunkChan <- &storage.Chunks{
						ChunkHash: chunkHash,
						Err:       err,
					}
					return
				}
				_, err = p2pCall(n, hostPid, "/storage/upload/init", ssID, strconv.FormatInt(channelID.Id, 10), chunkHash)
				if err != nil {
					chunkChan <- &storage.Chunks{
						ChunkHash: chunkHash,
						Err:       err,
					}
					return
				} else {
					chunkChan <- &storage.Chunks{
						ChunkHash: chunkHash,
						Err:       nil,
					}
				}
			}(chunk, i)
		}
		for range req.Arguments {
			chunk := <-chunkChan
			if chunk.Err != nil {
				return chunk.Err
			}
			ss.Chunk = append(ss.Chunk, chunk)
		}

		seRes := &UploadRes{
			ID: ssID,
		}
		return res.Emit(seRes)
	},
	Type: UploadRes{},
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
the chunk and replies back to client for the next challenge step.`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session.").EnableStdin(),
		cmds.StringArg("channel-id", true, false, "Open channel id for payment.").EnableStdin(),
		cmds.StringArg("chunk-hash", true, false, "Chunk the storage node should fetch.").EnableStdin(),
	},
	RunTimeout: 5 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage host api not enabled")
		}

		ssID := req.Arguments[0]
		channelID := req.Arguments[1]
		hash := req.Arguments[2]
		hashToCid, err := cidlib.Parse(hash)
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

		// build connection with ledger
		channelInfo, err := getChannelInfo(req.Context, channelID)
		if err != nil {
			return err
		}
		log.Debug("Verified channel info: ", channelInfo)

		// Get file
		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}
		p := path.New(hash)
		file, err := api.Unixfs().Get(req.Context, p)
		if err != nil {
			return err
		}
		_, err = fileArchive(file, p.String(), false, gzip.NoCompression)
		if err != nil {
			return err
		}

		// RemoteCall(user, hash) to api/v0/storage/upload/reqc to get chid and ch
		reqcBody, err := p2pCall(n, pid, "/storage/upload/reqc", ssID, hash)
		if err != nil {
			return err
		}
		r := ChallengeRes{}
		if err := json.Unmarshal(reqcBody, &r); err != nil {
			return err
		}

		// compute challenge on host
		sc := storage.NewStorageChallengeResponse(req.Context, n, api, r.ID)
		if err = sc.SolveChallenge(hashToCid, r.Nonce); err != nil {
			return err
		}

		// RemoteCall(user, CHID, CHR) to get signedPayment
		signedPaymentBody, err := p2pCall(n, pid, "/storage/upload/respc", ssID, r.Hash, hash)
		if err != nil {
			return err
		}
		payment := PaymentRes{}
		if err := json.Unmarshal(signedPaymentBody, &payment); err != nil {
			return err
		}
		var halfSignedChannelState ledgerPb.SignedChannelState
		err = proto.Unmarshal(payment.SignedPayment, &halfSignedChannelState)
		if err != nil {
			return err
		}

		// verify and sign
		signedchannelState, err := verifyAndSign(pid, n, &halfSignedChannelState)
		if err != nil {
			return err
		}
		// Close channel
		channelInfo, err = getChannelInfo(req.Context, channelID)
		if err != nil {
			return err
		}

		err = ledger.CloseChannel(req.Context, signedchannelState)
		if err != nil {
			return err
		}

		// prepare result
		// TODO: CollateralProof
		proof := &ProofRes{
			CollateralProof: "proof",
		}
		return cmds.EmitOnce(res, proof)
	},
	Type: ProofRes{},
}

type ProofRes struct {
	CollateralProof interface{}
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
		cmds.StringArg("session-id", true, false, "ID for the entire storage upload session.").EnableStdin(),
		cmds.StringArg("chunk-hash", true, false, "Chunk the storage node should fetch.").EnableStdin(),
	},
	RunTimeout: 3 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		ssID := req.Arguments[0]
		if storage.SessionMap[ssID] == nil {
			return fmt.Errorf("session id doesn't exist")
		}
		ss := storage.SessionMap[ssID]
		chunkHash := req.Arguments[1]

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}
		cid, err := cidlib.Parse(chunkHash)
		if err != nil {
			return err
		}

		// when multi-process talking to multiple hosts, different cids can only generate one storage challenge,
		// and stored the latest one in session map
		sch, err := ss.SetChallenge(req.Context, n, api, cid)
		if err != nil {
			return err
		}
		out := &ChallengeRes{
			ID:    sch.ID,
			Hash:  sch.Hash,
			Nonce: sch.Nonce,
		}
		return cmds.EmitOnce(res, out)
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
		cmds.StringArg("session-id", true, false, "Chunk the storage node should fetch.").EnableStdin(),
		//cmds.StringArg("challenge-id", true, false, "Challenge id from uploader.").EnableStdin(),
		cmds.StringArg("challenge-hash", true, false, "Challenge response back to uploader.").EnableStdin(),
		cmds.StringArg("chunk-hash", true, false, "Chunk the storage node should fetch.").EnableStdin(),
	},
	RunTimeout: 3 * time.Second,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		// pre-check
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("storage client api not enabled")
		}

		// verify challenge
		ssID := req.Arguments[0]
		challengeHash := req.Arguments[1]
		chunkHash := req.Arguments[2]
		if ss := storage.SessionMap[ssID]; ss == nil {
			return fmt.Errorf("session id doesn't exist")
		} else {
			now := time.Now()
			if now.After(ss.Time.Add(challengeTimeOut)) {
				return fmt.Errorf("challenge verification time out")
			}
			if c := ss.Challenge[chunkHash]; c != nil {
				if c.Hash != challengeHash {
					return fmt.Errorf("fail to verify challenge")
				}
			} else {
				return fmt.Errorf("chunk related challenge doesn't exist")
			}
		}

		// prepare payment
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		pid, ok := remote.GetStreamRequestRemotePeerID(req, n)
		if !ok {
			return fmt.Errorf("fail to get peer ID from request")
		}

		channelState, err := prepareChannelState(n, pid)
		if err != nil {
			return err
		}

		signedPayment, err := signChannelState(n.PrivateKey, channelState)
		if err != nil {
			return err
		}

		signedBytes, err := proto.Marshal(signedPayment)
		if err != nil {
			return nil
		}
		r := &PaymentRes{
			SignedPayment: signedBytes,
		}
		return cmds.EmitOnce(res, r)
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

func prepareChannelState(n *core.IpfsNode, pid peer.ID) (*ledgerPb.ChannelState, error) {
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
