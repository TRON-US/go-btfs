package upload

import (
	"context"
	"fmt"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/cmdenv"
	"github.com/TRON-US/go-btfs/core/commands/storage"

	cmds "github.com/TRON-US/go-btfs-cmds"
	config "github.com/TRON-US/go-btfs-config"
	iface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/TRON-US/interface-go-btfs-core/path"

	"github.com/alecthomas/units"
	"github.com/cenkalti/backoff/v3"
	"github.com/google/uuid"
	cidlib "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
)

const (
	uploadPriceOptionName   = "price"
	storageLengthOptionName = "storage-length"
)

var (
	log = logging.Logger("upload")

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
)

type ContextParams struct {
	req *cmds.Request
	ctx context.Context
	n   *core.IpfsNode
	cfg *config.Config
	api iface.CoreAPI
}

func extractContextParams(req *cmds.Request, env cmds.Environment) (*ContextParams, error) {
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
	ctx, _ := storage.NewGoContext(req.Context)
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
	cids, fileSize, err := storage.CheckAndGetReedSolomonShardHashes(params.ctx, params.n, params.api, fileCid)
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
	ns, err := storage.GetHostStorageConfig(params.ctx, params.n)
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

func getNodeSizeFromCid(ctx context.Context, hash cidlib.Cid, api iface.CoreAPI) (uint64, error) {
	leafPath := path.IpfsPath(hash)
	ipldNode, err := api.ResolveNode(ctx, leafPath)
	if err != nil {
		return 0, err
	}
	return ipldNode.Size()
}
