package helper

import (
	"context"
	"errors"
	"sync"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage"

	coreiface "github.com/TRON-US/interface-go-btfs-core"
	hubpb "github.com/tron-us/go-btfs-common/protos/hub"

	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	numHosts = 100
)

type HostProvider struct {
	ctx  context.Context
	node *core.IpfsNode
	api  coreiface.CoreAPI
	sync.Mutex
	mode    string
	current int
	hosts   []*hubpb.Host
	filter  func() bool
}

func GetHostProvider(ctx context.Context, node *core.IpfsNode, mode string,
	api coreiface.CoreAPI) *HostProvider {
	p := &HostProvider{
		ctx:     ctx,
		node:    node,
		mode:    mode,
		api:     api,
		current: 0,
		filter: func() bool {
			return false
		},
	}
	p.init()
	return p
}

func (p *HostProvider) init() (err error) {
	p.hosts, err = storage.GetHostsFromDatastore(p.ctx, p.node, p.mode, numHosts)
	if err != nil {
		return err
	}
	return nil
}

func (p *HostProvider) AddIndex() (int, error) {
	p.Lock()
	defer p.Unlock()
	if p.current >= len(p.hosts) {
		return -1, errors.New("Index exceeds array bounds.")
	}
	p.current++
	return p.current, nil
}

func (p *HostProvider) NextValidHost(price int64) (string, error) {
	for true {
		if index, err := p.AddIndex(); err == nil {
			host := p.hosts[index-1]
			id, err := peer.IDB58Decode(host.NodeId)
			//id, err := peer.IDB58Decode("16Uiu2HAmVGndWgJEG2ZXnhdRXbRXS7a1XGMuozdidw8kuwQ9wDKX")
			if err != nil || int64(host.StoragePriceAsk) > price {
				continue
			}
			if err := p.api.Swarm().Connect(p.ctx, peer.AddrInfo{ID: id}); err != nil {
				continue
			}
			return host.NodeId, nil
			//return "16Uiu2HAmVGndWgJEG2ZXnhdRXbRXS7a1XGMuozdidw8kuwQ9wDKX", nil
		} else {
			break
		}
	}
	return "", errors.New("failed to find more valid hosts")
}
