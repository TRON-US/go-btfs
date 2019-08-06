package commands

import (
	"bytes"
	"context"
	"fmt"
	"github.com/gogo/protobuf/proto"
	peer "github.com/libp2p/go-libp2p-peer"
	"strconv"
	"strings"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/core/ledger"
	ledgerPb "github.com/TRON-US/go-btfs/core/ledger/pb"

	cmds "github.com/ipfs/go-ipfs-cmds"
	ic "github.com/libp2p/go-libp2p-crypto"
)

const (
	uploadPriceOptionName = "price"
	replicationFactorOptionName = "replication-factor"
	hostSelectModeOptionName = "host-select-mode"
	hostSelectionOptionName = "host-selection"
)

var (
	channelID *ledgerPb.ChannelID
	ledgerClient ledgerPb.ChannelsClient
	price int64
	)

const (
	userURL = "http://127.0.0.1:5001"
	nodeURL = "http://127.0.0.1:5101"
)

var StorageCmd = &cmds.Command{
	Subcommands: map[string]*cmds.Command{
		"upload": storageUploadCmd,
	},
}

var storageUploadCmd = &cmds.Command{
	Subcommands: map[string]*cmds.Command{
		"init": storageUploadInitCmd,
		"reqc": storageUploadRequestCmd,
		"respc": storageUploadResponseCmd,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, false, "add hash of file to upload").EnableStdin(),
	},
	Options: []cmds.Option{
		cmds.Int64Option(uploadPriceOptionName, "p", "max price per GB of storage in BTT"),
		cmds.Int64Option(replicationFactorOptionName, "replica", "replication factor for the file with erasure coding built-in").WithDefault(int64(3)),
		cmds.StringOption(hostSelectModeOptionName, "mode", "based on mode to select the host and upload automatically").WithDefault("score"),
		cmds.StringOption(hostSelectionOptionName, "list", "use only these hosts in order on CUSTOM mode"),
	},
	PreRun: func(req *cmds.Request, env cmds.Environment) error {
		price, found := req.Options[uploadPriceOptionName].(int64)
		if found {
			if price < 0 {
				return fmt.Errorf("cannot input a negative price")
			}
		} else {
			// TODO: Select best price from top candidates
			req.Options[uploadPriceOptionName] = int64(10)
		}

		return nil
	},
	Run: func(req *cmds.Request, emitter cmds.ResponseEmitter, env cmds.Environment) error {
		fileHash := req.Arguments[0]
		price, _ = req.Options[uploadPriceOptionName].(int64)
		mode, _ := req.Options[hostSelectModeOptionName].(string)
		list, found := req.Options[hostSelectionOptionName].(string)
		var peers []string
		if strings.EqualFold(mode, "custom") {
			if !found {
				return fmt.Errorf("custom mode needs input host lists")
			} else {
				peers = strings.Split(list, ",")
			}
		}

		// get self key pair
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		selfPrivKey := n.PrivateKey
		selfPubKey := selfPrivKey.GetPublic()
		log.Infof("Private Key: %v \n Public Key: %v\n", selfPrivKey, selfPubKey)

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
				log.Error("fail to extract public key from peer ID: %s")
				return fmt.Errorf("fail to extract public key from peer ID: %s", err)
			}
			channelID, err = initChannel(selfPubKey, selfPrivKey, peerPubKey, price)
			if err != nil {
				log.Error("fail to init channel with peer id:", pid, err)
				continue
			}
			if channelID != nil {
				break
			}
		}
		log.Info("Create Channel Success: ", channelID)

		// Remote call other Node with fileHash
		remoteCall := &remote.RemoteCall{
			URL: nodeURL,
			ID: pid.Pretty(),
		}
		var argInit []string
		argInit = append(argInit, n.Identity.Pretty(), strconv.FormatInt(channelID.Id, 10), fileHash)
		respBody, err := remoteCall.CallGet(remote.APIprefix+"/storage/upload/init?", argInit)
		if err != nil {
			log.Error("fail to get response from: ", err)
			return err
		}
		resp, err := remote.UnmarshalResp(respBody)
		if err != nil {
			log.Error("resp body marshal err: ", err)
			return err
		}
		return emitter.Emit(fmt.Sprintf("Upload Success!\n response from upload init: %v", resp))
	},
}

var storageUploadInitCmd = &cmds.Command{
	Arguments: []cmds.Argument{
		cmds.StringArg("peer-id", true, false, "peer to initiate storage upload with").EnableStdin(),
		cmds.StringArg("channel-id", true, false, "open channel id for payment").EnableStdin(),
		cmds.StringArg("chunk-hash", true, false, "chunk the storage node should fetch").EnableStdin(),
	},
	Run: func(req *cmds.Request, emit cmds.ResponseEmitter, env cmds.Environment) error {
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		pid := req.Arguments[0]
		cid := req.Arguments[1]
		chunk := req.Arguments[2]

		log.Info("Verifying Channel Info to establish payment")
		ctx := context.Background()
		// build connection with ledger
		clientConn, err := ledger.LedgerConnection()
		if err != nil {
			log.Error("fail to connect", err)
			return err
		}
		ledgerClient = ledger.NewClient(clientConn)
		cidInt64, err := strconv.ParseInt(cid, 10, 64)
		if err != nil {
			return fmt.Errorf("fail to convert channel ID to int64: %s", err)
		}
		channelID := ledgerPb.ChannelID{Id: cidInt64}
		channelInfo, err := ledgerClient.GetChannelInfo(ctx, &channelID)
		if err != nil {
			log.Error("fail to get channel info", err)
			return err
		}
		log.Info("Verified channel info: ", channelInfo)

		// RemoteCall to api/v0/get/file-hash=hash
		remoteCall := &remote.RemoteCall{
			URL: userURL,
			ID: pid,
		}
		var argsGet []string
		argsGet = append(argsGet, chunk)
		fileBytes, err := remoteCall.CallGet(remote.APIprefix+"/get?", argsGet)
		if err != nil {
			log.Error("fail to get chunk file: \n", err)
			return err
		}
		log.Info("Successfully get file! \n", string(fileBytes))

		// RemoteCall(user, hash) to api/v0/storage/upload/reqc to get chid and ch
		var argReqc []string
		argReqc = append(argReqc, n.Identity.Pretty(), chunk)
		respChanllengeBody, err := remoteCall.CallGet(remote.APIprefix+"/storage/upload/reqc?", argReqc)
		if err != nil {
			log.Error("fail to remote call reqc", err)
			return err
		}
		bodyJSON, err := remote.UnmarshalResp(respChanllengeBody)
		if err != nil {
			log.Error("")
			return err
		}
		log.Info("Successful unmarshal json from reqc:", bodyJSON)

		// TODO: Verify ch to get CHR

		// RemoteCall(user, CHID, CHR) to get signedPayment
		var argRespc []string
		argRespc = append(argRespc, n.Identity.Pretty(), chunk, "ChallengeID", "ChallengeData")
		signedPaymentBody, err := remoteCall.CallGet(remote.APIprefix+"/storage/upload/respc?", argRespc)
		if err != nil {
			log.Error("fail to get resp with signedPayment: ", err)
			return err
		}
		log.Info("Received signed payment:", signedPaymentBody)
		var halfSignedChannelState ledgerPb.SignedChannelState
		err = proto.Unmarshal(signedPaymentBody, &halfSignedChannelState)
		if err != nil {
			log.Error("fail to unmarshal signed payment: ", err)
			return err
		}
		channelState := halfSignedChannelState.GetChannel()
		log.Info("Get current channel state: ", channelState)

		// Verify payment
		pk, err := stringPeerIdToPublicKey(req.Arguments[0])
		if err != nil {
			return err
		}
		ok, err := ledger.Verify(pk, channelState, halfSignedChannelState.GetFromSignature())
		if err != nil || !ok {
			log.Error("fail to verify channel state: ", err)
			return err
		}
		log.Info("Successfully verify channel state!")

		// sign with private key
		log.Info("Sign and Complete transfer")
		selfPrivKey := n.PrivateKey
		sig, err := ledger.Sign(selfPrivKey, channelState)
		if err != nil {
			log.Error("fail to sign: ", err)
			return err
		}
		halfSignedChannelState.ToSignature = sig
		SignedchannelState := halfSignedChannelState

		// Close channel
		err = ledger.CloseChannel(context.Background(), ledgerClient, &SignedchannelState)
		if err != nil {
			log.Error("fail to close channel: ", err)
			return err
		}
		log.Info("Successfully close channel")

		// prepare result
		// TODO: CollateralProof
		res := make(map[string]interface{})
		res["CollateralProof"] = "proof"
		return cmds.EmitOnce(emit, res)
	},
}

var storageUploadRequestCmd = &cmds.Command{
	Arguments: []cmds.Argument{
		cmds.StringArg("peer-id", true, false, "peer to initiate storage upload with").EnableStdin(),
		cmds.StringArg("chunk-hash", true, false, "chunk the storage node should fetch").EnableStdin(),
	},
	Run: func(req *cmds.Request, emit cmds.ResponseEmitter, env cmds.Environment) error {
		log.Info("Reqc Received Call.")
		cid := "challengeID"
		ch := "challengeData"
		res := make(map[string]interface{})
		res["ChallengeID"] = cid
		res["Challenge"] = ch
		return cmds.EmitOnce(emit, res)
	},
}

var storageUploadResponseCmd = &cmds.Command{
	Arguments: []cmds.Argument{
		cmds.StringArg("peer-id", true, false, "peer to initiate storage upload with").EnableStdin(),
		cmds.StringArg("chunk-hash", true, false, "chunk the storage node should fetch").EnableStdin(),
		cmds.StringArg("challenge-id", true, false, "challenge id from uploader").EnableStdin(),
		cmds.StringArg("challenge", true, false, "challenge response back to uploader.").EnableStdin(),
	},
	Run: func(req *cmds.Request, emit cmds.ResponseEmitter, env cmds.Environment) error {
		log.Info("Repsc Received Call.")
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		//clientConn, err := ledger.LedgerConnection()
		//if err != nil {
		//	return err
		//}
		//ledgerClient = ledger.NewClient(clientConn)
		fromAccount := ledger.NewAccount(n.PrivateKey.GetPublic(), 0)
		// prepare money receiver account
		toPubKey, err := stringPeerIdToPublicKey(req.Arguments[0])
		if err != nil {
			log.Error("fail to extract public key from input string: ", err)
			return err
		}
		toAccount := ledger.NewAccount(toPubKey, price)
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
		log.Info("Sending signed payment", signedBytes)
		return cmds.EmitOnce(emit, r)
	},
}

func initChannel(payerPubKey ic.PubKey, payerPrivKey ic.PrivKey, recvPubKey ic.PubKey, amount int64) (*ledgerPb.ChannelID, error) {
	ctx := context.Background()
	// build connection with ledger
	clientConn, err := ledger.LedgerConnection()
	if err != nil {
		log.Error("fail to connect", err)
		return nil, err
	}
	// new ledger client
	ledgerClient = ledger.NewClient(clientConn)
	// create account
	_, err = ledger.ImportAccount(ctx, payerPubKey, ledgerClient)
	if err != nil {
		log.Error("fail to import account with peer ID: ", err)
		return nil, err
	}
	_, err = ledger.ImportAccount(ctx, recvPubKey, ledgerClient)
	if err != nil {
		log.Error("fail to import account with peer ID: ", err)
		return nil, err
	}
	// prepare channel commit and sign
	cc := ledger.NewChannelCommit(payerPubKey, recvPubKey, amount)
	sig, err := ledger.Sign(payerPrivKey, cc)
	if err != nil {
		log.Error("fail to sign channel commit with private key: ", err)
		return nil, err
	}
	return ledger.CreateChannel(ctx, ledgerClient, cc, sig)
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
