package renter

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/commands/storage/host"
	"github.com/TRON-US/go-btfs/core/commands/storage/renter/guard"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/core/escrow"
	"github.com/TRON-US/go-btfs/core/hub"

	cmds "github.com/TRON-US/go-btfs-cmds"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/TRON-US/interface-go-btfs-core/path"
	shardpb "github.com/tron-us/go-btfs-common/protos/storage/shard"

	"github.com/alecthomas/units"
	"github.com/google/uuid"
	cidlib "github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	uploadPriceOptionName   = "price"
	storageLengthOptionName = "storage-length"
	defaultStorageLength    = 30
)

// TODO: get/set the value from/in go-btfs-common
var HostPriceLowBoundary = int64(10)

type UploadRes struct {
	ID string
}

var StorageUploadCmd = &cmds.Command{
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
		"status":       storageUploadStatusCmd,
		"recvcontract": StorageUploadRecvContractCmd,
		"init":         host.StorageUploadInitCmd,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, false, "Hash of file to upload."),
	},
	Options: []cmds.Option{
		cmds.Int64Option(uploadPriceOptionName, "p", "Max price per GiB per day of storage in BTT."),
		cmds.IntOption(storageLengthOptionName, "len", "File storage period on hosts in days.").WithDefault(defaultStorageLength),
	},
	RunTimeout: 15 * time.Minute,
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		// get hosts
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
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

		// get shardes
		if len(req.Arguments) != 1 {
			return fmt.Errorf("need one and only one root file hash")
		}
		hashStr := req.Arguments[0]
		rootHash, err := cidlib.Parse(hashStr)
		if err != nil {
			return err
		}
		cids, _, err := storage.CheckAndGetReedSolomonShardHashes(req.Context, n, api, rootHash)
		if err != nil || len(cids) == 0 {
			return fmt.Errorf("invalid hash: %s", err)
		}

		hp := GetHostProvider(req.Context, n, cfg.Experimental.HostsSyncMode, api)

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

		hashes := make([]string, 0)
		for _, cid := range cids {
			hashes = append(hashes, cid.String())
		}
		// init
		ctx, _ := context.WithTimeout(req.Context, req.Command.RunTimeout)
		session, err := GetSession(ctx, n.Repo.Datastore(), cfg, n, api, n.Identity.String(), "")
		if err != nil {
			return err
		}
		err = session.ToInit(n.Identity.String(), hashStr, hashes)
		if err != nil {
			return err
		}
		shardHashes := make([]string, 0)
		shardSize, err := getContractSizeFromCid(req.Context, cids[0], api)
		if err != nil {
			return err
		}
		storageLength := req.Options[storageLengthOptionName].(int)
		if uint64(storageLength) < ns.StorageTimeMin {
			return fmt.Errorf("invalid storage len. want: >= %d, got: %d",
				ns.StorageTimeMin, storageLength)
		}
		for i, h := range hashes {
			go func(i int, h string) error {
				s, err := GetShard(req.Context, n.Repo.Datastore(), n.Identity.String(), session.Id, h)
				if err != nil {
					return err
				}
				shardHashes = append(shardHashes, h)
				host, err := hp.NextValidHost()
				if err != nil {
					return err
				}
				peerId, err := peer.IDB58Decode(host)
				if err != nil {
					return err
				}
				totalPay := int64(float64(shardSize) / float64(units.GiB) * float64(price) * float64(storageLength))
				if totalPay == 0 {
					totalPay = 1
				}
				contract, err := escrow.NewContract(cfg, uuid.New().String(), n, peerId, totalPay, false, 0, "")
				if err != nil {
					return fmt.Errorf("create escrow contract failed: [%v] ", err)
				}
				halfSignedEscrowContract, err := escrow.SignContractAndMarshal(contract, nil, n.PrivateKey, true)
				if err != nil {
					return fmt.Errorf("sign escrow contract and maorshal failed: [%v] ", err)
				}

				metadata, err := session.GetMetadata()
				if err != nil {
					return err
				}
				md := &shardpb.Metadata{
					Index:          int32(i),
					SessionId:      session.Id,
					FileHash:       metadata.FileHash,
					ShardFileSize:  int64(shardSize),
					StorageLength:  int64(storageLength),
					ContractId:     session.Id,
					Receiver:       host,
					Price:          price,
					TotalPay:       totalPay,
					StartTime:      time.Now().UTC(),
					ContractLength: time.Duration(storageLength*24) * time.Hour,
				}
				s.ToInit(md)
				guardContractMeta, err := guard.NewContract(md, h, cfg, n.Identity.String())
				if err != nil {
					return fmt.Errorf("fail to new contract meta: [%v] ", err)
				}
				halfSignGuardContract, err := guard.SignedContractAndMarshal(guardContractMeta, nil, n.PrivateKey, true,
					false, n.Identity.Pretty(), n.Identity.Pretty())
				if err != nil {
					return fmt.Errorf("fail to sign guard contract and marshal: [%v] ", err)
				}

				sc := &shardpb.Contracts{}
				sc.HalfSignedEscrowContract = halfSignedEscrowContract
				sc.HalfSignedGuardContract = halfSignGuardContract

				_, err = remote.P2PCall(req.Context, n, peerId, "/storage/upload/init",
					session.Id,
					metadata.FileHash,
					h,
					strconv.FormatInt(md.Price, 10),
					halfSignedEscrowContract,
					halfSignGuardContract,
					strconv.FormatInt(md.StorageLength, 10),
					strconv.FormatInt(md.ShardFileSize, 10),
					strconv.Itoa(i),
				)
				if err != nil {
					s.ToError(err)
				} else {
					s.ToContract(sc)
				}
				return nil
			}(i, h)
		}
		seRes := &UploadRes{
			ID: session.Id,
		}
		return res.Emit(seRes)
	},
	Type: UploadRes{},
}

var StorageUploadRecvContractCmd = &cmds.Command{
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
		shardHash := req.Arguments[1]
		s, err := GetShard(req.Context, n.Repo.Datastore(), n.Identity.String(), ssID, shardHash)
		if err != nil {
			return err
		}
		guardContract, err := guard.UnmarshalGuardContract(guardContractBytes)
		if err != nil {
			s.ToError(err)
			return err
		}
		s.ToComplete(escrowContractBytes, guardContract)
		return nil
	},
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
		// get hosts
		cfg, err := cmdenv.GetConfig(env)
		if err != nil {
			return err
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

		session, err := GetSession(req.Context, n.Repo.Datastore(), cfg, n, api, n.Identity.String(), ssID)
		if err != nil {
			return err
		}
		sessionStatus, err := session.GetStatus()
		if err != nil {
			return err
		}
		status.Status = sessionStatus.Status
		status.Message = sessionStatus.Message

		// check if checking request from host or client
		if !cfg.Experimental.StorageClientEnabled && !cfg.Experimental.StorageHostEnabled {
			return fmt.Errorf("storage client/host api not enabled")
		}

		// get shards info from session
		shards := make(map[string]*ShardStatus)
		metadata, err := session.GetMetadata()
		if err != nil {
			return err
		}
		status.FileHash = metadata.FileHash
		for _, h := range metadata.ShardHashes {
			shard, err := GetShard(req.Context, n.Repo.Datastore(), n.Identity.String(), session.Id, h)
			if err != nil {
				return err
			}
			st, err := shard.Status()
			if err != nil {
				return err
			}
			md, err := shard.Metadata()
			if err != nil {
				return err
			}
			c := &ShardStatus{
				ContractID: md.ContractId,
				Price:      md.Price,
				Host:       md.Receiver,
				Status:     st.Status,
				Message:    st.Message,
			}
			shards[h] = c
		}
		status.Shards = shards
		return res.Emit(status)
	},
	Type: StatusRes{},
}

func getContractSizeFromCid(ctx context.Context, hash cidlib.Cid, api coreiface.CoreAPI) (uint64, error) {
	leafPath := path.IpfsPath(hash)
	ipldNode, err := api.ResolveNode(ctx, leafPath)
	if err != nil {
		return 0, err
	}
	return ipldNode.Size()
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
	Message    string
}
