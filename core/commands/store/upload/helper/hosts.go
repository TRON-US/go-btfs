package helper

import (
	"context"
	"errors"
	"fmt"
	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage"
	coreiface "github.com/TRON-US/interface-go-btfs-core"
	"github.com/libp2p/go-libp2p-core/peer"
	hubpb "github.com/tron-us/go-btfs-common/protos/hub"
	"sync"
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
	fmt.Println("p.current a")
	p.Lock()
	defer p.Unlock()
	fmt.Println("p.current b")
	fmt.Println("p.current", p.current, len(p.hosts))
	if p.current >= len(p.hosts) {
		return -1, errors.New("Index exceeds array bounds.")
	}
	p.current++
	return p.current, nil
}

func (p *HostProvider) NextValidHost(price int64) (string, error) {
	for true {
		if index, err := p.AddIndex(); err == nil {
			host := p.hosts[index]
			id, err := peer.IDB58Decode(host.NodeId)
			if err != nil || int64(host.StoragePriceAsk) > price {
				fmt.Println("err", err, "host.StoragePriceAsk", host.StoragePriceAsk)
				continue
			}
			if err := p.api.Swarm().Connect(p.ctx, peer.AddrInfo{ID: id}); err != nil {
				continue
			}
			return host.NodeId, nil
		} else {
			fmt.Println("abcabc")
			break
		}
	}
	return "", errors.New("failed to find more valid hosts")
}
