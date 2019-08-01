package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/ledger"
	ledgerPb "github.com/TRON-US/go-btfs/core/ledger/pb"

	cmds "github.com/ipfs/go-ipfs-cmds"
	//libcrypto "github.com/libp2p/go-libp2p-core/crypto"
	ic "github.com/libp2p/go-libp2p-crypto"
)

const (
	uploadPriceOptionName = "price"
	replicationFactorOptionName = "replication-factor"
	hostSelectModeOptionName = "host-select-mode"
	hostSelectionOptionName = "host-selection"
)
var StorageCmd = &cmds.Command{
	Subcommands: map[string]*cmds.Command{
		"upload": storageUploadCmd,
	},
}

var storageUploadCmd = &cmds.Command{
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

		mode, _ := req.Options[hostSelectModeOptionName].(string)
		list, found := req.Options[hostSelectionOptionName].(string)
		if strings.EqualFold(mode, "custom") {
			if !found {
				return fmt.Errorf("custom mode needs input host lists")
			} else {
				req.Options[hostSelectionOptionName] = strings.Split(list, ",")
			}
		} else {
			req.Options[hostSelectionOptionName] = nil
		}
		return nil
	},
	Run: func(req *cmds.Request, emitter cmds.ResponseEmitter, env cmds.Environment) error {
		fileHash := req.Arguments[0]
		price, _ := req.Options[uploadPriceOptionName].(int64)
		// get self key pair
		n, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		selfPubKey, err := n.Identity.ExtractPublicKey()
		if err != nil {
			return fmt.Errorf("fail to extract self peer ID's public key: %v", err)
		}
		selfPrivKey := n.PrivateKey
		// get other node's public key as address and create channel between them
		var channelID *ledgerPb.ChannelID
		for _, str := range req.Options[hostSelectionOptionName].([]string) {
			_, pid, err := ParsePeerParam(str)
			if err != nil {
				return fmt.Errorf("failed to parse peer address '%s': %s", str, err)
			}
			peerPubKey, err := pid.ExtractPublicKey()
			channelID, err = initChannel(selfPubKey, selfPrivKey, peerPubKey, price)
			if err != nil {
				continue
			}
			if channelID != nil {
				break
			}
		}
		// TODO: Remote call other Node with fileHash

		//replica, _ := req.Options[replicationFactorOptionName].(int64)
		//mode, _ := req.Options[hostSelectModeOptionName].(string)
		return emitter.Emit(fmt.Sprintf("channel ID: %d \n file-hash: %s", channelID.GetId(), fileHash))
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
	ledgerClient := ledger.NewClient(clientConn)
	// create account
	_, err = ledger.ImportAccount(ctx, payerPubKey, ledgerClient)
	if err != nil {
		log.Error("fail to create account with peer ID", err)
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
