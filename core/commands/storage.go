package commands

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/core/ledger"
	ledgerPb "github.com/TRON-US/go-btfs/core/ledger/pb"

	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/interface-go-ipfs-core/path"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	uploadPriceOptionName       = "price"
	replicationFactorOptionName = "replication-factor"
	hostSelectModeOptionName    = "host-select-mode"
	hostSelectionOptionName     = "host-selection"
)

var (
	channelID *ledgerPb.ChannelID
	price     int64
)

var StorageCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "pay to store a file on btfs node.",
		ShortDescription: `
To UPLOAD a file using select hosts: 
    using -m with "custom" mode, and put host identifier in -l, multiple hosts separate by ','
For example:

    btfs storage upload -m=custom -l=host_address1,address2

Or it will select a node based on reputation for you.
And receiving a Collateral Proof as evidence when selected node stores your file.
	`,
	},
	Subcommands: map[string]*cmds.Command{
		"upload": storageUploadCmd,
	},
}

var storageUploadCmd = &cmds.Command{
	Subcommands: map[string]*cmds.Command{
		"init":  storageUploadInitCmd,
		"reqc":  storageUploadRequestChallengeCmd,
		"respc": storageUploadResponseChallengeCmd,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, false, "add hash of file to upload").EnableStdin(),
	},
	Options: []cmds.Option{
		cmds.Int64Option(uploadPriceOptionName, "p", "Max price per GB of storage in BTT."),
		cmds.Int64Option(replicationFactorOptionName, "r", "Replication factor for the file with erasure coding built-in.").WithDefault(int64(3)),
		cmds.StringOption(hostSelectModeOptionName, "m", "Based on mode to select the host and upload automatically.").WithDefault("score"),
		cmds.StringOption(hostSelectionOptionName, "l", "Use only these hosts in order on 'custom' mode. Use ',' as delimiter."),
	},
	PreRun: func(req *cmds.Request, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("client remoteAPI is not ENABLED")
		}
		price, found := req.Options[uploadPriceOptionName].(int64)
		if found && price < 0 {
			return fmt.Errorf("cannot input a negative price")
		} else if !found {
			// TODO: Select best price from top candidates
			req.Options[uploadPriceOptionName] = int64(10)
		}

		mode, _ := req.Options[hostSelectModeOptionName].(string)
		_, found = req.Options[hostSelectionOptionName].(string)
		if mode == "custom" && !found {
			return fmt.Errorf("custom mode needs input host lists")
		}
		return nil
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		fileHash := req.Arguments[0]
		price, _ = req.Options[uploadPriceOptionName].(int64)
		list, _ := req.Options[hostSelectionOptionName].(string)
		peers := strings.Split(list, ",")

		// get self key pair
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		selfPrivKey := n.PrivateKey
		selfPubKey := selfPrivKey.GetPublic()

		// get other node's public key as address
		// create channel between them
		var pid peer.ID
		for _, str := range peers {
			_, pid, err = ParsePeerParam(str)
			if err != nil {
				return fmt.Errorf("failed to parse peer address '%s': %s", str, err)
			}
			peerPubKey, err := pid.ExtractPublicKey()
			if err != nil {
				return fmt.Errorf("fail to extract public key from peer ID: %s", err)
			}
			channelID, err = initChannel(req.Context, selfPubKey, selfPrivKey, peerPubKey, price)
			if err != nil {
				continue
			}
			if channelID != nil {
				break
			}
		}
		if channelID == nil || channelID.GetId() == 0 {
			return fmt.Errorf("fail to create channel ID")
		}

		// call server
		respBody, err := p2pCall(n, pid, "/storage/upload/init", strconv.FormatInt(channelID.Id, 10), fileHash)
		if err != nil {
			return fmt.Errorf("fail to get response from: %s", err)
		}
		log.Info("Upload success, get proof: ", respBody)
		return nil
	},
}

var storageUploadInitCmd = &cmds.Command{
	Arguments: []cmds.Argument{
		//cmds.StringArg("peer-id", true, false, "peer to initiate storage upload with").EnableStdin(),
		cmds.StringArg("channel-id", true, false, "open channel id for payment").EnableStdin(),
		cmds.StringArg("chunk-hash", true, false, "chunk the storage node should fetch").EnableStdin(),
	},
	PreRun: func(req *cmds.Request, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("host remoteAPI is not ENABLED")
		}
		return nil
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		cid := req.Arguments[0]
		hash := req.Arguments[1]

		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		pid, ok := remote.GetStreamRequestRemotePeerID(req, n)
		if !ok {
			return fmt.Errorf("fail to get peer ID from request")
		}

		// build connection with ledger
		channelInfo, err := getChannelInfo(req.Context, cid)
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
		reqcBody, err := p2pCall(n, pid, "/storage/upload/reqc", hash)
		if err != nil {
			return err
		}
		r := ChallengeRes{}
		if err := json.Unmarshal(reqcBody, &r); err != nil {
			return err
		}

		// TODO: Verify ch to get CHR

		// RemoteCall(user, CHID, CHR) to get signedPayment
		signedPaymentBody, err := p2pCall(n, pid, "/storage/upload/respc", hash, r.ID, r.Challenge)
		if err != nil {
			return err
		}
		var halfSignedChannelState ledgerPb.SignedChannelState
		err = proto.Unmarshal(signedPaymentBody, &halfSignedChannelState)
		if err != nil {
			return err
		}

		signedchannelState, err := verifyAndSign(pid, n, &halfSignedChannelState)

		// Close channel
		err = ledger.CloseChannel(req.Context, signedchannelState)
		if err != nil {
			return err
		}
		log.Info("Successfully close channel")

		// prepare result
		// TODO: CollateralProof
		proof := &ProofRes{
			CollateralProof: "proof",
		}
		return res.Emit(proof)
	},
	Type: ProofRes{},
}

type ProofRes struct {
	CollateralProof interface{}
}

type ChallengeRes struct {
	ID        string
	Challenge string
}

var storageUploadRequestChallengeCmd = &cmds.Command{
	Arguments: []cmds.Argument{
		//cmds.StringArg("peer-id", true, false, "peer to initiate storage upload with").EnableStdin(),
		cmds.StringArg("chunk-hash", true, false, "chunk the storage node should fetch").EnableStdin(),
	},
	PreRun: func(req *cmds.Request, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("client remote API is not enabled")
		}
		return nil
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		out := &ChallengeRes{
			ID:        "1234567",
			Challenge: "Data",
		}
		return cmds.EmitOnce(res, out)
	},
	Type: ChallengeRes{},
}

var storageUploadResponseChallengeCmd = &cmds.Command{
	Arguments: []cmds.Argument{
		//cmds.StringArg("peer-id", true, false, "peer to initiate storage upload with").EnableStdin(),
		cmds.StringArg("chunk-hash", true, false, "chunk the storage node should fetch").EnableStdin(),
		cmds.StringArg("challenge-id", true, false, "challenge id from uploader").EnableStdin(),
		cmds.StringArg("challenge", true, false, "challenge response back to uploader.").EnableStdin(),
	},
	PreRun: func(req *cmds.Request, env cmds.Environment) error {
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
		}
		if !cfg.Experimental.StorageClientEnabled {
			return fmt.Errorf("client remote API is not enabled")
		}
		return nil
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
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
			log.Error("fail to marshal signed payment: ", err)
			return nil
		}
		r := bytes.NewReader(signedBytes)
		return cmds.EmitOnce(res, r)
	},
}

func p2pCall(n *core.IpfsNode, pid peer.ID, api string, arg ...string) ([]byte, error) {
	remoteCall := &remote.P2PRemoteCall{
		Node: n,
		ID:   pid,
	}
	return remoteCall.CallGet(api, arg)
}

func getChannelInfo(ctx context.Context, cid string) (*ledgerPb.ChannelInfo, error) {
	clientConn, err := ledger.LedgerConnection()
	defer ledger.CloseConnection(clientConn)
	if err != nil {
		return nil, err
	}
	ledgerClient := ledger.NewClient(clientConn)
	cidInt64, err := strconv.ParseInt(cid, 10, 64)
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
	cid, err := ledger.CreateChannel(ctx, ledgerClient, cc, sig)
	if err != nil {
		return nil, err
	}
	return cid, nil
}