package chain

import (
	"path/filepath"

	"github.com/TRON-US/go-btfs/statestore/leveldb"
	"github.com/TRON-US/go-btfs/statestore/mock"
	"github.com/TRON-US/go-btfs/transaction/storage"
	logging "github.com/ipfs/go-log"
)

func InitStateStore(dataDir string) (ret storage.StateStorer, err error) {
	if dataDir == "" {
		ret = mock.NewStateStore()
		log.Warning("using in-mem state store, no node state will be persisted")
		return ret, nil
	}

	var log = logging.Logger("chain:statestore")
	return leveldb.NewStateStore(filepath.Join(dataDir, "statestore"), log)
}
