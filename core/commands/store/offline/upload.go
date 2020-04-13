package upload

import (
	"errors"
	"fmt"
	cmds "github.com/TRON-US/go-btfs-cmds"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"
	"github.com/cenkalti/backoff/v3"
	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p-core/peer"
	cmap "github.com/orcaman/concurrent-map"
	"time"
)

const (
	replicationFactorOptionName      = "replication-factor"
	hostSelectModeOptionName         = "host-select-mode"
	hostSelectionOptionName          = "host-selection"
	testOnlyOptionName               = "host-search-local"
	customizedPayoutOptionName       = "customize-payout"
	customizedPayoutPeriodOptionName = "customize-payout-period"

	defaultRepFactor     = 3
	defaultStorageLength = 30
)

var (
	shardErrChanMap = cmap.New()
)

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
		//"status":       StorageUploadStatusCmd,
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
		ssId := uuid.New().String()
		ctxParams, err := extractContextParams(req, env)
		if err != nil {
			return err
		}
		fileHash := req.Arguments[0]
		offlinePeerId := ctxParams.n.Identity.Pretty()
		offlineSigning := false
		if len(req.Arguments) > 1 {
			offlinePeerId = req.Arguments[1]
			offlineSigning = true
		}
		shardHashes, fileSize, shardSize, err := getShardHashes(ctxParams, fileHash)
		if err != nil {
			return err
		}
		price, storageLength, err := getPriceAndMinStorageLength(ctxParams)
		if err != nil {
			return err
		}
		hp := getHostsProvider(ctxParams)
		rss, err := GetRenterSession(ctxParams, ssId, fileHash, shardHashes)
		if err != nil {
			return err
		}
		for shardIndex, shardHash := range shardHashes {
			go func(i int, h string) error {
				backoff.Retry(func() error {
					select {
					case <-rss.ctx.Done():
						return nil
					default:
						break
					}
					host, err := hp.NextValidHost(price)
					if err != nil {
						fmt.Println("next valid host", err)
						rss.to(rssErrorStatus, err)
						rss.cancel()
						return nil
					}
					contractId := newContractID(ssId)
					cb := make(chan error)
					shardErrChanMap.Set(contractId, cb)
					tp := totalPay(shardSize, price, storageLength)
					escrowCotractBytes, err := renterSignEscrowContract(rss, host, tp, offlineSigning, contractId)
					if err != nil {
						log.Errorf("shard %s signs escrow_contract error: %s", h, err.Error())
						return err
					}
					guardContractBytes, err := renterSignGuardContract(ctxParams, &ContractParams{
						ContractId:    contractId,
						RenterPid:     ctxParams.n.Identity.Pretty(),
						HostPid:       host,
						ShardIndex:    int32(i),
						ShardHash:     h,
						ShardSize:     shardSize,
						FileHash:      fileHash,
						StartTime:     time.Now(),
						StorageLength: int64(storageLength),
						Price:         price,
						TotalPay:      tp,
					}, offlineSigning)
					if err != nil {
						log.Errorf("shard %s signs guard_contract error: %s", h, err.Error())
						return err
					}
					hostPid, err := peer.IDB58Decode(host)
					if err != nil {
						log.Errorf("shard %s decodes host_pid error: %s", h, err.Error())
						return err
					}
					fmt.Println(ssId, fileHash, h, price, len(escrowCotractBytes), len(guardContractBytes),
						storageLength, shardSize, i, offlinePeerId)
					go func() {
						_, err = remote.P2PCall(ctxParams.ctx, ctxParams.n, hostPid, "/storage/upload/init",
							ssId,
							fileHash,
							h,
							price,
							escrowCotractBytes,
							guardContractBytes,
							storageLength,
							shardSize,
							i,
							offlinePeerId,
						)
						fmt.Println("done init...", err)
						if err != nil {
							switch err.(type) {
							case remote.IoError:
								// NOP
								log.Error("io error", err)
							case remote.BusinessError:
								fmt.Println("write remote.BusinessError", h, err)
								cb <- err
							default:
								fmt.Println("write default err", h, err)
								cb <- err
							}
						}
					}()
					// host needs to send recv in 30 seconds, or the contract will be invalid.
					tick := time.Tick(30 * time.Second)
					select {
					case err = <-cb:
						shardErrChanMap.Remove(contractId)
						return err
					case <-tick:
						return errors.New("host timeout")
					}
					return nil
				}, handleShardBo)
				return nil
			}(shardIndex, shardHash)
		}
		// waiting for contracts of 30 shards
		go func(rss *RenterSession, numShards int) {
			tick := time.Tick(5 * time.Second)
			for true {
				select {
				case <-tick:
					completeNum, errorNum, err := rss.GetCompleteShardsNum()
					if err != nil {
						continue
					}
					log.Info("session", rss.ssId, "contractNum", completeNum, "errorNum", errorNum)
					if completeNum == numShards {
						submit(rss, fileSize, offlineSigning)
						return
					} else if errorNum > 0 {
						rss.to(rssErrorStatus, errors.New("there are error shards"))
						log.Error("session:", rss.ssId, ",errorNum:", errorNum)
						return
					}
				case <-rss.ctx.Done():
					log.Infof("session %s done", rss.ssId)
					return
				}
			}
		}(rss, len(shardHashes))
		seRes := &Res{
			ID: ssId,
		}
		return res.Emit(seRes)
	},
	Type: Res{},
}

type Res struct {
	ID string
}
