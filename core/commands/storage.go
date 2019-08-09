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
		price, found := req.Options[uploadPriceOptionName].(int64);
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
			return res.Emit("fail to create channel ID")
		}

		// call server
		remoteCall := &remote.P2PRemoteCall{
			Node: n,
			ID:   pid,
		}
		argInit := []string{strconv.FormatInt(channelID.Id, 10), fileHash}
		respBody, err := remoteCall.CallGet("/storage/upload/init", argInit)
		if err != nil {
			return fmt.Errorf("fail to get response from: %s", err)
		}
		return res.Emit(fmt.Sprintf("Upload Success!\n Get response from upload init: %v", string(respBody)))
	},
}

var storageUploadInitCmd = &cmds.Command{
	Arguments: []cmds.Argument{
		//cmds.StringArg("peer-id", true, false, "peer to initiate storage upload with").EnableStdin(),
		cmds.StringArg("channel-id", true, false, "open channel id for payment").EnableStdin(),
		cmds.StringArg("chunk-hash", true, false, "chunk the storage node should fetch").EnableStdin(),
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
		clientConn, err := ledger.LedgerConnection()
		defer ledger.CloseConnection(clientConn)
		if err != nil {
			log.Error("fail to connect", err)
			return err
		}
		ledgerClient := ledger.NewClient(clientConn)
		cidInt64, err := strconv.ParseInt(cid, 10, 64)
		if err != nil {
			return fmt.Errorf("fail to convert channel ID to int64: %s", err)
		}
		channelID := ledgerPb.ChannelID{Id: cidInt64}
		channelInfo, err := ledgerClient.GetChannelInfo(req.Context, &channelID)
		if err != nil {
			log.Error("fail to get channel info", err)
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
			log.Error("fail to get chunk file: \n", err)
			return err
		}

		// RemoteCall(user, hash) to api/v0/storage/upload/reqc to get chid and ch
		remoteCall := &remote.P2PRemoteCall{
			Node: n,
			ID:   pid,
		}
		argReqc := []string{hash}
		reqcBody, err := remoteCall.CallGet("/storage/upload/reqc", argReqc)
		if err != nil {
			log.Error("fail to remote call reqc", err)
			return err
		}
		r := ChallengeRes{}
		if err := json.Unmarshal(reqcBody, &r); err != nil {
			return err
		}

		// TODO: Verify ch to get CHR

		// RemoteCall(user, CHID, CHR) to get signedPayment
		argRespc := []string{hash, r.ID, r.Challenge}
		signedPaymentBody, err := remoteCall.CallGet("/storage/upload/respc", argRespc)
		if err != nil {
			log.Error("fail to get resp with signedPayment: ", err)
			return err
		}
		var halfSignedChannelState ledgerPb.SignedChannelState
		err = proto.Unmarshal(signedPaymentBody, &halfSignedChannelState)
		if err != nil {
			log.Error("fail to unmarshal signed payment: ", err)
			return err
		}
		channelState := halfSignedChannelState.GetChannel()

		// Verify payment
		pk, err := pid.ExtractPublicKey()
		if err != nil {
			return err
		}
		ok, err = ledger.Verify(pk, channelState, halfSignedChannelState.GetFromSignature())
		if err != nil || !ok {
			log.Error("fail to verify channel state: ", err)
			return err
		}

		// sign with private key
		selfPrivKey := n.PrivateKey
		sig, err := ledger.Sign(selfPrivKey, channelState)
		if err != nil {
			log.Error("fail to sign: ", err)
			return err
		}
		halfSignedChannelState.ToSignature = sig
		SignedchannelState := halfSignedChannelState

		// Close channel
		err = ledger.CloseChannel(req.Context, ledgerClient, &SignedchannelState)
		if err != nil {
			log.Error("fail to close channel: ", err)
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
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		out := &ChallengeRes{
			ID:        "1234567",
			Challenge: "Data",
		}
		return cmds.EmitOnce(res, out)
	},
}

var storageUploadResponseChallengeCmd = &cmds.Command{
	Arguments: []cmds.Argument{
		//cmds.StringArg("peer-id", true, false, "peer to initiate storage upload with").EnableStdin(),
		cmds.StringArg("chunk-hash", true, false, "chunk the storage node should fetch").EnableStdin(),
		cmds.StringArg("challenge-id", true, false, "challenge id from uploader").EnableStdin(),
		cmds.StringArg("challenge", true, false, "challenge response back to uploader.").EnableStdin(),
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

		fromAccount, err := ledger.NewAccount(n.PrivateKey.GetPublic(), 0)
		if err != nil {
			log.Error("fail to create payer account,", err)
			return err
		}
		// prepare money receiver account
		toPubKey, err := pid.ExtractPublicKey()
		if err != nil {
			log.Error("fail to extract public key from id: ", err)
			return err
		}
		toAccount, err := ledger.NewAccount(toPubKey, price)
		if err != nil {
			log.Error("fail to create receiver account,", err)
			return err
		}
		// create channel state wait for both side to agree on
		channelState := ledger.NewChannelState(channelID, 0, fromAccount, toAccount)
		// sign channel state
		sig, err := ledger.Sign(n.PrivateKey, channelState)
		if err != nil {
			log.Error("fail to sign on channel state: ", err)
			return err
		}
		signedPayment := ledger.NewSignedChannelState(channelState, sig, nil)
		signedBytes, err := proto.Marshal(signedPayment)
		if err != nil {
			log.Error("fail to marshal signed payment: ", err)
			return nil
		}
		r := bytes.NewReader(signedBytes)
		return cmds.EmitOnce(res, r)
	},
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

func stringPeerIdToPublicKey(str string) (ic.PubKey, error) {
	_, pid, err := ParsePeerParam(str)
	if err != nil {
		return nil, err
	}
	pubKey, err := pid.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	return pubKey, nil
}
