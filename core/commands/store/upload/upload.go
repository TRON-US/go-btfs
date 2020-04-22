package upload

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	"github.com/TRON-US/go-btfs/core/commands/store/upload/ds"
	"github.com/TRON-US/go-btfs/core/commands/store/upload/helper"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/TRON-US/go-btfs/core/escrow"
	"github.com/TRON-US/go-btfs/core/guard"

	"github.com/tron-us/go-btfs-common/crypto"
	escrowpb "github.com/tron-us/go-btfs-common/protos/escrow"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/alecthomas/units"
	"github.com/cenkalti/backoff/v3"
	cidlib "github.com/ipfs/go-cid"
	ic "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
)

var waitUploadBo = func() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 1 * time.Second
	bo.MaxElapsedTime = 24 * time.Hour
	bo.Multiplier = 1.5
	bo.MaxInterval = 1 * time.Minute
	return bo
}()

var handleShardBo = func() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 1 * time.Second
	bo.MaxElapsedTime = 300 * time.Second
	bo.Multiplier = 1
	bo.MaxInterval = 1 * time.Second
	return bo
}()

var waitingForPeersBo = func() *backoff.ExponentialBackOff {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 1 * time.Second
	bo.MaxElapsedTime = 300 * time.Second
	bo.Multiplier = 1.2
	bo.MaxInterval = 5 * time.Second
	return bo
}()

const (
	uploadPriceOptionName            = "price"
	replicationFactorOptionName      = "replication-factor"
	hostSelectModeOptionName         = "host-select-mode"
	hostSelectionOptionName          = "host-selection"
	testOnlyOptionName               = "host-search-local"
	storageLengthOptionName          = "storage-length"
	customizedPayoutOptionName       = "customize-payout"
	customizedPayoutPeriodOptionName = "customize-payout-period"

	defaultRepFactor       = 3
	defaultStorageLength   = 30
	thresholdContractsNums = 20
)

type ShardObj struct {
	index int
	hash  string
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
		"init":         StorageUploadInitCmd,
		"recvcontract": StorageUploadRecvContractCmd,
		"status":       StorageUploadStatusCmd,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("file-hash", true, false, "Hash of file to upload."),
	},
	Options: []cmds.Option{
		cmds.Int64Option(uploadPriceOptionName, "p", "Max price per GiB per day of storage in JUST."),
		cmds.IntOption(replicationFactorOptionName, "r", "Replication factor for the file with erasure coding built-in.").WithDefault(defaultRepFactor),
		cmds.StringOption(hostSelectModeOptionName, "m", "Based on this mode to select hosts and upload automatically. Default: mode set in config option Experimental.HostsSyncMode."),
		cmds.StringOption(hostSelectionOptionName, "s", "Use only these selected hosts in order on 'custom' mode. Use ',' as delimiter."),
		cmds.BoolOption(testOnlyOptionName, "t", "Enable host search under all domains 0.0.0.0 (useful for local test)."),
		cmds.IntOption(storageLengthOptionName, "len", "File storage period on hosts in days.").WithDefault(defaultStorageLength),
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
		err = backoff.Retry(func() error {
			peersLen := len(n.PeerHost.Network().Peers())
			if peersLen <= 0 {
				err = errors.New("failed to find any peer in table")
				log.Error(err)
				return err
			}
			return nil
		}, waitingForPeersBo)
		if err != nil {
			return errors.New("please check your network")
		}

		// get core api
		api, err := cmdenv.GetApi(env, req)
		if err != nil {
			return err
		}
		fileHash := req.Arguments[0]
		fileCid, err := cidlib.Parse(fileHash)
		if err != nil {
			return err
		}
		cids, fileSize, err := storage.CheckAndGetReedSolomonShardHashes(req.Context, n, api, fileCid)
		if err != nil || len(cids) == 0 {
			return fmt.Errorf("invalid hash: %s", err)
		}

		shardHashes := make([]string, 0)
		for _, c := range cids {
			shardHashes = append(shardHashes, c.String())
		}

		// set price limit, the price is default when host doesn't provide price
		ns, err := storage.GetHostStorageConfig(req.Context, n)
		if err != nil {
			return err
		}
		price, found := req.Options[uploadPriceOptionName].(int64)
		if !found {
			price = int64(ns.StoragePriceAsk)
		}
		shardCid, err := cidlib.Parse(shardHashes[0])
		if err != nil {
			return err
		}
		shardSize, err := helper.GetNodeSizeFromCid(req.Context, shardCid, api)
		if err != nil {
			return err
		}
		storageLength := req.Options[storageLengthOptionName].(int)
		if uint64(storageLength) < ns.StorageTimeMin {
			return fmt.Errorf("invalid storage len. want: >= %d, got: %d",
				ns.StorageTimeMin, storageLength)
		}
		ss, err := ds.GetSession("", nodepb.ContractStat_RENTER.String(), n.Identity.Pretty(), &ds.SessionInitParams{
			Context:     req.Context,
			Config:      cfg,
			N:           n,
			Api:         api,
			Datastore:   n.Repo.Datastore(),
			RenterId:    n.Identity.Pretty(),
			FileHash:    fileHash,
			ShardHashes: shardHashes,
		})
		if err != nil {
			return err
		}

		mode, ok := req.Options[hostSelectModeOptionName].(string)
		if !ok {
			mode = cfg.Experimental.HostsSyncMode
		}
		var hostIDs []string
		if mode == "custom" {
			if hosts, ok := req.Options[hostSelectionOptionName].(string); ok {
				hostIDs = strings.Split(hosts, ",")
			}
			if len(hostIDs) != len(shardHashes) {
				return fmt.Errorf("custom mode hosts length must match shard hashes length")
			}
		}
		hp := helper.GetHostProvider(ss.Context, n, mode, api, hostIDs)
		for shardIndex, shardHash := range shardHashes {
			go func(i int, h string, f *ds.Session) {
				backoff.Retry(func() error {
					select {
					case <-f.Context.Done():
						return nil
					default:
						break
					}
					host, err := hp.NextValidHost(price)
					if err != nil {
						f.Error(err)
						return nil
					}
					totalPay := int64(float64(shardSize) / float64(units.GiB) * float64(price) * float64(storageLength))
					if totalPay <= 0 {
						totalPay = 1
					}
					hostPid, err := peer.IDB58Decode(host)
					if err != nil {
						return err
					}
					// Pass in session id and generate final contract id inside NewContract
					escrowContract, err := escrow.NewContract(cfg, ss.Id, n, hostPid, totalPay, false, 0, "")
					if err != nil {
						return fmt.Errorf("create escrow contract failed: [%v] ", err)
					}
					guardContractMeta, err := helper.NewContract(cfg, &helper.ContractParams{
						ContractId:    escrowContract.ContractId,
						RenterPid:     n.Identity.Pretty(),
						HostPid:       host,
						ShardIndex:    int32(i),
						ShardHash:     h,
						ShardSize:     int64(shardSize),
						FileHash:      fileHash,
						StartTime:     time.Now(),
						StorageLength: int64(storageLength),
						Price:         price,
						TotalPay:      totalPay,
					})
					if err != nil {
						return fmt.Errorf("fail to new contract meta: [%v] ", err)
					}

					// online signing
					halfSignedEscrowContract, err := escrow.SignContractAndMarshal(escrowContract, nil, n.PrivateKey, true)
					if err != nil {
						return fmt.Errorf("sign escrow contract and maorshal failed: [%v] ", err)
					}
					halfSignGuardContract, err := guard.SignedContractAndMarshal(guardContractMeta, nil, nil, n.PrivateKey, true,
						false, n.Identity.Pretty(), n.Identity.Pretty())
					if err != nil {
						return fmt.Errorf("fail to sign guard contract and marshal: [%v] ", err)
					}
					_, err = remote.P2PCall(req.Context, n, hostPid, "/storage/upload/init",
						ss.Id,
						fileHash,
						h,
						price,
						halfSignedEscrowContract,
						halfSignGuardContract,
						storageLength,
						shardSize,
						i,
						n.Identity.Pretty(), //TODO: offline
					)
					if err != nil {
						return err
					}
					//TODO: retry when recv contract timeout
					return nil
				}, handleShardBo)
			}(shardIndex, shardHash, ss)
		}

		// waiting for contracts of 30 shards
		go func(f *ds.Session, numShards int) {
			tick := time.Tick(5 * time.Second)
			for true {
				select {
				case <-tick:
					completeNum, errorNum, err := f.GetCompleteShardsNum()
					if err != nil {
						continue
					}
					if completeNum == numShards {
						doSubmit(f, fileSize)
						return
					} else if errorNum > 0 {
						f.Error(errors.New("there are error shards"))
						return
					}
				case <-f.Context.Done():
					return
				}
			}
		}(ss, len(shardHashes))

		seRes := &UploadRes{
			ID: ss.Id,
		}
		return res.Emit(seRes)
	},
	Type: UploadRes{},
}

func doSubmit(f *ds.Session, fileSize int64) {
	f.Submit()
	bs, t, err := f.PrepareContractFromShard()
	if err != nil {
		f.Error(err)
		return
	}
	// check account balance, if not enough for the totalPrice do not submit to escrow
	balance, err := escrow.Balance(f.Context, f.Config)
	if err != nil {
		f.Error(err)
		return
	}
	if balance < t {
		f.Error(fmt.Errorf("not enough balance to submit contract, current balance is [%v]", balance))
		return
	}
	req, err := escrow.NewContractRequest(f.Config, bs, t)
	if err != nil {
		f.Error(err)
		return
	}
	var amount int64 = 0
	for _, c := range req.Contract {
		amount += c.Contract.Amount
	}
	submitContractRes, err := escrow.SubmitContractToEscrow(f.Context, f.Config, req)
	if err != nil {
		f.Error(fmt.Errorf("failed to submit contracts to escrow: [%v]", err))
		return
	}
	doPay(f, submitContractRes, fileSize)
	return
}

func doPay(f *ds.Session, response *escrowpb.SignedSubmitContractResult, fileSize int64) {
	f.Pay()
	privKeyStr := f.Config.Identity.PrivKey
	payerPrivKey, err := crypto.ToPrivKey(privKeyStr)
	if err != nil {
		f.Error(err)
		return
	}
	payerPubKey := payerPrivKey.GetPublic()
	payinRequest, err := escrow.NewPayinRequest(response, payerPubKey, payerPrivKey)
	if err != nil {
		f.Error(err)
		return
	}
	payinRes, err := escrow.PayInToEscrow(f.Context, f.Config, payinRequest)
	if err != nil {
		f.Error(fmt.Errorf("failed to pay in to escrow: [%v]", err))
		return
	}
	doGuard(f, payinRes, payerPrivKey, fileSize)
}

func doGuard(f *ds.Session, res *escrowpb.SignedPayinResult, payerPriKey ic.PrivKey, fileSize int64) {
	f.Guard()
	md, err := f.GetMetadata()
	if err != nil {
		f.Error(err)
		return
	}
	cts := make([]*guardpb.Contract, 0)
	for _, h := range md.ShardHashes {
		shard, err := ds.GetShard(f.PeerId, f.Role, f.Id, h, &ds.ShardInitParams{
			Context:   f.Context,
			Datastore: f.Datastore,
		})
		if err != nil {
			f.Error(err)
			return
		}
		contracts, err := shard.SignedCongtracts()
		if err != nil {
			f.Error(err)
			return
		}
		cts = append(cts, contracts.GuardContract)
	}
	fsStatus, err := helper.PrepAndUploadFileMeta(f.Context, cts, res, payerPriKey, f.Config, f.PeerId, md.FileHash,
		fileSize)
	if err != nil {
		f.Error(fmt.Errorf("failed to send file meta to guard: [%v]", err))
		return
	}

	qs, err := helper.PrepFileChallengeQuestions(f.Context, f.N, f.Api, fsStatus, md.FileHash, f.PeerId, f.Id)
	if err != nil {
		f.Error(err)
		return
	}

	fcid, err := cidlib.Parse(md.FileHash)
	if err != nil {
		f.Error(err)
		return
	}
	err = helper.SendChallengeQuestions(f.Context, f.Config, fcid, qs)
	if err != nil {
		f.Error(fmt.Errorf("failed to send challenge questions to guard: [%v]", err))
		return
	}
	doWaitUpload(f, payerPriKey)
}

func doWaitUpload(f *ds.Session, payerPriKey ic.PrivKey) {
	f.WaitUpload()
	md, err := f.GetMetadata()
	if err != nil {
		f.Error(err)
		return
	}
	err = backoff.Retry(func() error {
		select {
		case <-f.Context.Done():
			return errors.New("context closed")
		default:
		}
		err := grpc.GuardClient(f.Config.Services.GuardDomain).WithContext(f.Context,
			func(ctx context.Context, client guardpb.GuardServiceClient) error {
				req := &guardpb.CheckFileStoreMetaRequest{
					FileHash:     md.FileHash,
					RenterPid:    md.RenterId,
					RequesterPid: f.N.Identity.Pretty(),
					RequestTime:  time.Now().UTC(),
				}
				sign, err := crypto.Sign(payerPriKey, req)
				if err != nil {
					return err
				}
				req.Signature = sign
				meta, err := client.CheckFileStoreMeta(ctx, req)
				if err != nil {
					return err
				}
				num := 0
				for _, c := range meta.Contracts {
					if c.State == guardpb.Contract_UPLOADED {
						num++
					}
				}
				if num >= thresholdContractsNums {
					return nil
				}
				return errors.New("uploading")
			})
		return err
	}, waitUploadBo)
	if err != nil {
		f.Error(err)
		return
	}
	f.Complete()
}

type UploadRes struct {
	ID string
}
