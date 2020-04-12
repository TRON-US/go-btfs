package upload

import (
	"errors"
	"sync"

	"github.com/TRON-US/go-btfs/core/commands/storage"

	hubpb "github.com/tron-us/go-btfs-common/protos/hub"

	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	numHosts = 100
)

type HostsProvider struct {
	cp *ContextParams
	sync.Mutex
	mode    string
	current int
	hosts   []*hubpb.Host
	filter  func() bool
}

func getHostsProvider(cp *ContextParams) *HostsProvider {
	p := &HostsProvider{
		cp:      cp,
		mode:    cp.cfg.Experimental.HostsSyncMode,
		current: -1,
		filter: func() bool {
			return false
		},
	}
	p.init()
	return p
}

func (p *HostsProvider) init() (err error) {
	p.hosts, err = storage.GetHostsFromDatastore(p.cp.ctx, p.cp.n, p.mode, numHosts)
	return err
}

func (p *HostsProvider) AddIndex() (int, error) {
	p.Lock()
	defer p.Unlock()
	p.current++
	if p.current >= len(p.hosts) {
		return -1, errors.New("Index exceeds array bounds.")
	}
	return p.current, nil
}

func (p *HostsProvider) NextValidHost(price int64) (string, error) {
	needHigherPrice := false
	for true {
		if index, err := p.AddIndex(); err == nil {
			host := p.hosts[index]
			//id, err := peer.IDB58Decode("16Uiu2HAkxQ6QAPuQXLoFhcWo7aWmKjLjq1THLNjoL5HM6Xpmm7i9")
			id, err := peer.IDB58Decode(host.NodeId)
			if err != nil || int64(host.StoragePriceAsk) > price {
				needHigherPrice = true
				continue
			}
			if err := p.cp.api.Swarm().Connect(p.cp.ctx, peer.AddrInfo{ID: id}); err != nil {
				continue
			}
			return host.NodeId, nil
			//return "16Uiu2HAkxQ6QAPuQXLoFhcWo7aWmKjLjq1THLNjoL5HM6Xpmm7i9", nil
		} else {
			break
		}
	}
	msg := "failed to find more valid hosts, please try again later"
	if needHigherPrice {
		msg += " or raise price"
	}
	return "", errors.New(msg)
}
