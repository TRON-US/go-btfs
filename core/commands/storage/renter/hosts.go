package renter

import (
	"context"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage"

	coreiface "github.com/TRON-US/interface-go-btfs-core"
	hubpb "github.com/tron-us/go-btfs-common/protos/hub"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/prometheus/common/log"
)

const (
	numHosts = 100
)

type HostProvider struct {
	ctx     context.Context
	node    *core.IpfsNode
	mode    string
	current int
	api     coreiface.CoreAPI
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

func (p *HostProvider) NextValidHost() (string, error) {
	//for p.current < len(p.hosts) {
	for false {
		host := p.hosts[p.current]
		p.current++
		//id, err := peer.IDB58Decode(host.NodeId)
		id, err := peer.IDB58Decode("16Uiu2HAmVGndWgJEG2ZXnhdRXbRXS7a1XGMuozdidw8kuwQ9wDKX")
		if err != nil {
			log.Error("invalid host", host, err.Error())
			continue
		}
		if err := p.api.Swarm().Connect(p.ctx, peer.AddrInfo{ID: id}); err != nil {
			log.Error("failed to connect to host", host.NodeId, err.Error())
			continue
		}
		return host.NodeId, nil
	}
	return "16Uiu2HAmVGndWgJEG2ZXnhdRXbRXS7a1XGMuozdidw8kuwQ9wDKX", nil
	//return "", errors.New("failed to find more valid hosts")
}
