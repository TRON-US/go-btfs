package upload

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	"github.com/TRON-US/go-btfs/core/corehttp/remote"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	iface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/TRON-US/interface-go-btfs-core/path"

	"github.com/alecthomas/units"
	"github.com/cenkalti/backoff/v3"
	"github.com/google/uuid"
	cidlib "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	uploadPriceOptionName   = "price"
	storageLengthOptionName = "storage-length"
)

var (
	log = logging.Logger("upload")

	// retry strategies
	waitUploadBo = func() *backoff.ExponentialBackOff {
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = 1 * time.Second
		bo.MaxElapsedTime = 24 * time.Hour
		bo.Multiplier = 1.5
		bo.MaxInterval = 1 * time.Minute
		return bo
	}()
	handleShardBo = func() *backoff.ExponentialBackOff {
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = 1 * time.Second
		bo.MaxElapsedTime = 300 * time.Second
		bo.Multiplier = 1
		bo.MaxInterval = 1 * time.Second
		return bo
	}()
	checkPaymentBo = func() *backoff.ExponentialBackOff {
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = 10 * time.Second
		bo.MaxElapsedTime = 5 * time.Minute
		bo.Multiplier = 1.5
		bo.MaxInterval = 60 * time.Second
		return bo
	}()
	downloadShardBo = func(maxTime time.Duration) *backoff.ExponentialBackOff {
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = 10 * time.Second
		bo.MaxElapsedTime = maxTime
		bo.Multiplier = 1.5
		bo.MaxInterval = 30 * time.Minute
		return bo
	}
)

type ContextParams struct {
	req *cmds.Request
	ctx context.Context
	n   *core.IpfsNode
	cfg *config.Config
	api iface.CoreAPI
}

func ExtractContextParams(req *cmds.Request, env cmds.Environment) (*ContextParams, error) {
	// get config settings
	cfg, err := cmdenv.GetConfig(env)
	if err != nil {
		return nil, err
	}
	// get node
	n, err := cmdenv.GetNode(env)
	if err != nil {
		return nil, err
	}
	// get core api
	api, err := cmdenv.GetApi(env, req)
	if err != nil {
		return nil, err
	}
	ctx, _ := helper.NewGoContext(req.Context)
	return &ContextParams{
		req: req,
		ctx: ctx,
		n:   n,
		cfg: cfg,
		api: api,
	}, nil
}

func getShardHashes(params *ContextParams, fileHash string) (shardHashes []string, fileSize int64,
	shardSize int64, err error) {
	fileCid, err := cidlib.Parse(fileHash)
	if err != nil {
		return nil, -1, -1, err
	}
	cids, fileSize, err := helper.CheckAndGetReedSolomonShardHashes(params.ctx, params.n, params.api, fileCid)
	if err != nil || len(cids) == 0 {
		return nil, -1, -1, fmt.Errorf("invalid hash: %s", err)
	}

	shardHashes = make([]string, 0)
	for _, c := range cids {
		shardHashes = append(shardHashes, c.String())
	}
	shardCid, err := cidlib.Parse(shardHashes[0])
	if err != nil {
		return nil, -1, -1, err
	}
	sz, err := getNodeSizeFromCid(params.ctx, shardCid, params.api)
	if err != nil {
		return nil, -1, -1, err
	}
	shardSize = int64(sz)
	return
}

func getPriceAndMinStorageLength(params *ContextParams) (price int64, storageLength int, err error) {
	ns, err := helper.GetHostStorageConfig(params.ctx, params.n)
	if err != nil {
		return -1, -1, err
	}
	price, found := params.req.Options[uploadPriceOptionName].(int64)
	if !found {
		price = int64(ns.StoragePriceAsk)
	}
	storageLength = params.req.Options[storageLengthOptionName].(int)
	if uint64(storageLength) < ns.StorageTimeMin {
		return -1, -1, fmt.Errorf("invalid storage len. want: >= %d, got: %d",
			ns.StorageTimeMin, storageLength)
	}
	return
}

func totalPay(shardSize int64, price int64, storageLength int) int64 {
	totalPay := int64(float64(shardSize) / float64(units.GiB) * float64(price) * float64(storageLength))
	if totalPay <= 0 {
		totalPay = 1
	}
	return totalPay
}

func newContractID(sessionId string) string {
	id := uuid.New().String()
	return sessionId + "," + id
}

func splitContractId(contractId string) (ssId string, shardHash string) {
	splits := strings.Split(contractId, ",")
	ssId = splits[0]
	shardHash = splits[1]
	return
}

func getNodeSizeFromCid(ctx context.Context, hash cidlib.Cid, api iface.CoreAPI) (uint64, error) {
	leafPath := path.IpfsPath(hash)
	ipldNode, err := api.ResolveNode(ctx, leafPath)
	if err != nil {
		return 0, err
	}
	return ipldNode.Size()
}

func (rss *RenterSession) uploadShard(hp *HostsProvider, price int64, shardSize int64, storageLength int,
	offlineSigning bool, renterId peer.ID, fileSize int64, shardIndexes []int, rp *RepairParams) {
	for index, shardHash := range rss.shardHashes {
		go func(i int, h string) {
			err := backoff.Retry(func() error {
				select {
				case <-rss.ctx.Done():
					return nil
				default:
					break
				}
				host, err := hp.NextValidHost(price)
				if err != nil {
					_ = rss.to(rssErrorStatus, err)
					return nil
				}
				contractId := newContractID(rss.ssId)
				cb := make(chan error)
				shardErrChanMap.Set(contractId, cb)
				tp := totalPay(shardSize, price, storageLength)
				var escrowCotractBytes []byte
				errChan := make(chan error, 2)
				go func() {
					tmp := func() error {
						escrowCotractBytes, err = renterSignEscrowContract(rss, h, i, host, tp, offlineSigning,
							renterId, contractId)
						if err != nil {
							log.Errorf("shard %s signs escrow_contract error: %s", h, err.Error())
							return err
						}
						return nil
					}()
					errChan <- tmp
				}()
				var guardContractBytes []byte
				go func() {
					tmp := func() error {
						guardContractBytes, err = renterSignGuardContract(rss, &ContractParams{
							ContractId:    contractId,
							RenterPid:     renterId.Pretty(),
							HostPid:       host,
							ShardIndex:    int32(i),
							ShardHash:     h,
							ShardSize:     shardSize,
							FileHash:      rss.hash,
							StartTime:     time.Now(),
							StorageLength: int64(storageLength),
							Price:         price,
							TotalPay:      tp,
						}, offlineSigning, rp)
						if err != nil {
							log.Errorf("shard %s signs guard_contract error: %s", h, err.Error())
							return err
						}
						return nil
					}()
					errChan <- tmp
				}()
				c := 0
				for err := range errChan {
					c++
					if err != nil {
						return err
					}
					if c == 2 {
						break
					}
				}

				hostPid, err := peer.IDB58Decode(host)
				if err != nil {
					log.Errorf("shard %s decodes host_pid error: %s", h, err.Error())
					return err
				}
				go func() {
					_, err := remote.P2PCall(rss.ctxParams.ctx, rss.ctxParams.n, hostPid, "/storage/upload/init",
						rss.ssId,
						rss.hash,
						h,
						price,
						escrowCotractBytes,
						guardContractBytes,
						storageLength,
						shardSize,
						i,
						renterId,
					)
					if err != nil {
						switch err.(type) {
						case remote.IoError:
							// NOP
							log.Error("io error", err)
						case remote.BusinessError:
							log.Error("write remote.BusinessError", h, err)
							cb <- err
						default:
							log.Error("write default err", h, err)
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
			}, handleShardBo)
			if err != nil {
				_ = rss.to(rssErrorStatus, err)
			}
		}(shardIndexes[index], shardHash)
	}
	// waiting for contracts of 30(n) shards
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
					err := submit(rss, fileSize, offlineSigning)
					if err != nil {
						_ = rss.to(rssErrorStatus, err)
					}
					return
				} else if errorNum > 0 {
					_ = rss.to(rssErrorStatus, errors.New("there are some error shards"))
					log.Error("session:", rss.ssId, ",errorNum:", errorNum)
					return
				}
			case <-rss.ctx.Done():
				log.Infof("session %s done", rss.ssId)
				return
			}
		}
	}(rss, len(rss.shardHashes))
}
