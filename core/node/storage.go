package node

import (
	"context"
	"os"
	"syscall"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/retrystore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	config "github.com/ipfs/go-ipfs-config"
	"go.uber.org/fx"

	"github.com/TRON-US/go-btfs/core/node/helpers"
	"github.com/TRON-US/go-btfs/filestore"
	"github.com/TRON-US/go-btfs/repo"
	"github.com/TRON-US/go-btfs/thirdparty/cidv0v1"
	"github.com/TRON-US/go-btfs/thirdparty/verifbs"
)

func isTooManyFDError(err error) bool {
	perr, ok := err.(*os.PathError)
	if ok && perr.Err == syscall.EMFILE {
		return true
	}

	return false
}

// RepoConfig loads configuration from the repo
func RepoConfig(repo repo.Repo) (*config.Config, error) {
	return repo.Config()
}

// Datastore provides the datastore
func Datastore(repo repo.Repo) datastore.Datastore {
	return repo.Datastore()
}

// BaseBlocks is the lower level blockstore without GC or Filestore layers
type BaseBlocks blockstore.Blockstore

// BaseBlockstoreCtor creates cached blockstore backed by the provided datastore
func BaseBlockstoreCtor(cacheOpts blockstore.CacheOpts, nilRepo bool, hashOnRead bool) func(mctx helpers.MetricsCtx, repo repo.Repo, lc fx.Lifecycle) (bs BaseBlocks, err error) {
	return func(mctx helpers.MetricsCtx, repo repo.Repo, lc fx.Lifecycle) (bs BaseBlocks, err error) {
		rds := &retrystore.Datastore{
			Batching:    repo.Datastore(),
			Delay:       time.Millisecond * 200,
			Retries:     6,
			TempErrFunc: isTooManyFDError,
		}
		// hash security
		bs = blockstore.NewBlockstore(rds)
		bs = &verifbs.VerifBS{Blockstore: bs}

		if !nilRepo {
			ctx, cancel := context.WithCancel(mctx)

			lc.Append(fx.Hook{
				OnStop: func(context context.Context) error {
					cancel()
					return nil
				},
			})
			bs, err = blockstore.CachedBlockstore(ctx, bs, cacheOpts)
			if err != nil {
				return nil, err
			}
		}

		bs = blockstore.NewIdStore(bs)
		bs = cidv0v1.NewBlockstore(bs)

		if hashOnRead { // TODO: review: this is how it was done originally, is there a reason we can't just pass this directly?
			bs.HashOnRead(true)
		}

		return
	}
}

// GcBlockstoreCtor wraps the base blockstore with GC and Filestore layers
func GcBlockstoreCtor(bb BaseBlocks) (gclocker blockstore.GCLocker, gcbs blockstore.GCBlockstore, bs blockstore.Blockstore) {
	gclocker = blockstore.NewGCLocker()
	gcbs = blockstore.NewGCBlockstore(bb, gclocker)

	bs = gcbs
	return
}

// GcBlockstoreCtor wraps GcBlockstore and adds Filestore support
func FilestoreBlockstoreCtor(repo repo.Repo, bb BaseBlocks) (gclocker blockstore.GCLocker, gcbs blockstore.GCBlockstore, bs blockstore.Blockstore, fstore *filestore.Filestore) {
	gclocker = blockstore.NewGCLocker()

	// hash security
	fstore = filestore.NewFilestore(bb, repo.FileManager())
	gcbs = blockstore.NewGCBlockstore(fstore, gclocker)
	gcbs = &verifbs.VerifBSGC{GCBlockstore: gcbs}

	bs = gcbs
	return
}
