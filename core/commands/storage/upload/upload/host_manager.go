package upload

import (
	"fmt"
	config "github.com/TRON-US/go-btfs-config"
	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	"github.com/tron-us/go-btfs-common/protos/node"

	"github.com/ipfs/go-datastore"
)

type HostManager struct {
	low       int
	high      int
	threshold int64
}

func NewHostManster(cfg *config.Config) *HostManager {
	return &HostManager{
		low:       cfg.UI.Host.ContractManager.LowWater,
		high:      cfg.UI.Host.ContractManager.HighWater,
		threshold: cfg.UI.Host.ContractManager.Threshold,
	}
}

func (h *HostManager) AcceptContract(ds datastore.Datastore, peerId string, shardSize int64) (bool, error) {
	count, err := h.count(ds, peerId, guardpb.Contract_READY_CHALLENGE)
	if err != nil {
		return true, err
	}
	fmt.Println("count", count, "low", h.low, "high", h.high)
	if count <= h.low {
		return true, nil
	} else if count >= h.high {
		return false, nil
	} else {
		return shardSize <= h.threshold, nil
	}
}

func (h *HostManager) count(ds datastore.Datastore, peerId string, status guardpb.Contract_ContractState) (int, error) {
	contracts, err := ListContracts(ds, peerId, node.ContractStat_HOST.String())
	if err != nil {
		return 0, err
	}
	c := 0
	for i := 0; i < len(contracts); i++ {
		if contracts[i].Status == status {
			c++
		}
	}
	return c, nil
}
