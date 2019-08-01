package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/corehttp"
	"github.com/TRON-US/go-btfs/core/ledger"
	ledgerPb "github.com/TRON-US/go-btfs/core/ledger/pb"
	"github.com/TRON-US/go-btfs/core/node"
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
	customProtc = "x/test/http"
	userURL = "http://127.0.0.1:8180/p2p"
	nodeURL = "http://127.0.0.1:8080/p2p"
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
			// TODO: default value to be set as top-selected nodes' price
			// currently set price with 10000
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

		// get other node's public key as address and create channel between them
		for _, str := range peers {
			_, pid, err := ParsePeerParam(str)
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
				log.Error("fail to init channel with peer id", pid, err)
				continue
			}
			if channelID != nil {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("fail to init channel: %s", err)
		}
		// Remote call other Node with fileHash
		remoteCall := &node.RemoteCall{
			URL: nodeURL,
			ID: n.Identity,
		}
		var argInit []string
		argInit = append(argInit, n.Identity.Pretty(), strconv.FormatInt(channelID.Id, 10), fileHash)
		resp, err := remoteCall.CallGet(customProtc+corehttp.APIPath+"/storage/upload/init?", argInit)
		if err != nil {
			return fmt.Errorf("fail to get response from ")
		}
		return emitter.Emit(fmt.Sprintf("channel ID: %v \n resp: %v", channelID, resp))
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
		//pid := req.Arguments[0]
		cid := req.Arguments[1]
		chunk := req.Arguments[2]
		// TODO: Verify channel id
		cidInt64, err := strconv.ParseInt(cid, 10, 64)
		if err != nil {
			return fmt.Errorf("fail to convert channel ID to int64: %s", err)
		}
		channelID := ledgerPb.ChannelID{Id: cidInt64}
		channelInfo, err := ledgerClient.GetChannelInfo(context.Background(), &channelID)
		if err != nil {
			log.Error("fail to get channel info", err)
		}
		emit.Emit(fmt.Sprintf("verified channel info: %v", channelInfo))
		// RemoteCall to api/v0/get/file-hash=hash
		remoteCall := &node.RemoteCall{
			URL: userURL+corehttp.APIPath,
			ID: n.Identity,
		}
		var argsGet []string
		argsGet = append(argsGet, chunk)
		respGetBody, err := remoteCall.CallGet(customProtc+corehttp.APIPath+"/get?", argsGet)
		if err != nil {
			return emit.Emit(fmt.Errorf("fail to get chunk file: %v", err))
		}
		emit.Emit(fmt.Sprintf("response from get http api: %v", respGetBody))

		// RemoteCall(user, hash) to api/v0/storage/upload/reqc to get chid and ch
		var argReqc []string
		argReqc = append(argReqc, n.Identity.Pretty(), chunk)
		respChanllengeBody, err := remoteCall.CallGet(customProtc+corehttp.APIPath+"/storage/upload/reqc?", argReqc)
		if err != nil {
			return emit.Emit(fmt.Errorf("fail to remote call reqc: %v", err))
		}
		emit.Emit(fmt.Sprintf("response from reqc http api: %v", respChanllengeBody))
		chid := respChanllengeBody["ChallengeID"].(string)
		ch := respChanllengeBody["Challenge"].(string)
		// TODO: Verify ch to get CHR

		// RemoteCall(user, CHID, CHR) to get signedPayment
		var argRespc []string
		argRespc = append(argRespc, n.Identity.Pretty(), chunk, chid, ch)
		signedPayment, err := remoteCall.CallGet(customProtc+corehttp.APIPath+"/storage/upload/respc?", argRespc)
		if err != nil {
			return fmt.Errorf("fail to get resp with signedPayment: %s", err)
		}
		halfSignedChannelState := signedPayment["SignedPayment"].(ledgerPb.SignedChannelState)
		channelState := halfSignedChannelState.GetChannel()
		// Verify payment
		pk, err := stringPeerIdToPublicKey(req.Arguments[0])
		if err != nil {
			return err
		}
		ok, err := ledger.Verify(pk, channelState, halfSignedChannelState.GetFromSignature())
		if err != nil || !ok {
			return fmt.Errorf("fail to verify channel state: %v", err)
		}
		// sign with private key
		selfPrivKey := n.PrivateKey
		sig, err := ledger.Sign(selfPrivKey, channelState)
		if err != nil {
			return fmt.Errorf("fail to sign: %s", err)
		}
		halfSignedChannelState.ToSignature = sig
		SignedchannelState := halfSignedChannelState
		// Close channel
		err = ledger.CloseChannel(context.Background(), ledgerClient, &SignedchannelState)
		return err
	},
}

var storageUploadRequestCmd = &cmds.Command{
	Arguments: []cmds.Argument{
		cmds.StringArg("peer-id", true, false, "peer to initiate storage upload with").EnableStdin(),
		cmds.StringArg("chunk-hash", true, false, "chunk the storage node should fetch").EnableStdin(),
	},
	Run: func(req *cmds.Request, emit cmds.ResponseEmitter, env cmds.Environment) error {
		cid := "challenge ID"
		ch := "challenge"
		res := make(map[string]interface{})
		res["ChallengeID"] = cid
		res["Challenge"] = ch
		return emit.Emit(res)
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
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		clientConn, err := ledger.LedgerConnection()
		if err != nil {
			return err
		}
		ledgerClient = ledger.NewClient(clientConn)
		fromAccount := ledger.NewAccount(n.PrivateKey.GetPublic(), 0)
		// prepare money receiver account
		toPubKey, err := stringPeerIdToPublicKey(req.Arguments[0])
		if err != nil {
			return fmt.Errorf("fail to extract public key from input string: %s", err)
		}
		toAccount := ledger.NewAccount(toPubKey, price)
		// create channel state wait for both side to agree on
		channelState := ledger.NewChannelState(channelID, 0, fromAccount, toAccount)
		// sign channel state
		var signedPayment *ledgerPb.SignedChannelState
		sig, err := ledger.Sign(n.PrivateKey, channelState)
		if err != nil {
			return fmt.Errorf("fail to sign on channel state: %v", err)
		}
		signedPayment.FromSignature = sig
		res := make(map[string]interface{})
		res["SignedPayment"] = signedPayment
		return emit.Emit(res)
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
		log.Error("fail to import account with peer ID", err)
		return nil, err
	}
	_, err = ledger.ImportAccount(ctx, recvPubKey, ledgerClient)
	if err != nil {
		log.Error("fail to import account with peer ID", err)
		return nil, err
	}
	// prepare channel commit and sign
	cc := ledger.NewChannelCommit(payerPubKey, recvPubKey, amount)
	sig, err := ledger.Sign(payerPrivKey, cc)
	if err != nil {
		log.Error("fail to sign channel commit with private key", err)
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
