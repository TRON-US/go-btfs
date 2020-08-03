package helper

import (
	"context"
	"errors"
	iface "github.com/TRON-US/interface-go-btfs-core"
	"sort"
	"sync"
	"time"

	"github.com/TRON-US/go-btfs/core/commands/storage/helper"
	hubpb "github.com/tron-us/go-btfs-common/protos/hub"

	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	minimumHosts = 30
	failMsg      = "failed to find more valid hosts, please try again later"
)

type IHostsProvider interface {
	NextValidHost(price int64) (string, error)
}

type CustomizedHostsProvider struct {
	cp      *ContextParams
	current int
	hosts   []string
	sync.Mutex
}

func (p *CustomizedHostsProvider) NextValidHost(price int64) (string, error) {
	for true {
		if index, err := p.AddIndex(); err == nil {
			id, err := peer.IDB58Decode(p.hosts[index])
			if err != nil {
				continue
			}
			if err := p.cp.Api.Swarm().Connect(p.cp.Ctx, peer.AddrInfo{ID: id}); err != nil {
				p.hosts = append(p.hosts, p.hosts[index])
				continue
			}
			return p.hosts[index], nil
		} else {
			break
		}
	}
	return "", errors.New(failMsg)
}

func GetCustomizedHostsProvider(cp *ContextParams, hosts []string) IHostsProvider {
	return &CustomizedHostsProvider{
		cp:      cp,
		current: -1,
		hosts:   hosts,
	}
}

func (p *CustomizedHostsProvider) AddIndex() (int, error) {
	p.Lock()
	defer p.Unlock()
	p.current++
	if p.current >= len(p.hosts) {
		return -1, errors.New("Index exceeds array bounds.")
	}
	return p.current, nil
}

type HostsProvider struct {
	cp *ContextParams
	sync.Mutex
	mode           string
	current        int
	hosts          []*hubpb.Host
	blacklist      []string
	backupList     []string
	backupListLock sync.Locker
	ctx            context.Context
	cancel         context.CancelFunc
}

func GetHostsProvider(cp *ContextParams, blacklist []string) IHostsProvider {
	ctx, cancel := context.WithTimeout(cp.Ctx, 10*time.Minute)
	p := &HostsProvider{
		cp:        cp,
		mode:      cp.Cfg.Experimental.HostsSyncMode,
		current:   -1,
		blacklist: blacklist,
		ctx:       ctx,
		cancel:    cancel,
	}
	p.init()
	return p
}

func (p *HostsProvider) init() (err error) {
	p.hosts, err = helper.GetHostsFromDatastore(p.cp.Ctx, p.cp.N, p.mode, minimumHosts)
	if err != nil {
		return err
	}
	peers, err := p.cp.Api.Swarm().Peers(p.cp.Ctx)
	if err != nil {
		log.Debug(err)
		return nil
	}
	var prs Peers = peers
	sort.Sort(prs)
	p.backupList = make([]string, 0)
	for _, h := range prs {
		for _, ph := range p.hosts {
			if h.ID().String() == ph.NodeId {
				continue
			}
		}
		p.backupList = append(p.backupList, h.ID().String())
	}
	return nil
}

type Peers []iface.ConnectionInfo

func (p Peers) Len() int {
	return len(p)
}

func (p Peers) Less(i int, j int) bool {
	first, err := p[i].Latency()
	if err != nil {
		return true
	}
	second, err := p[j].Latency()
	if err != nil {
		return true
	}
	return first <= second
}

func (p Peers) Swap(i int, j int) {
	p[i], p[j] = p[j], p[i]
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

func (p *HostsProvider) PickFromBackupHosts() (string, error) {
	for true {
		host, err := func() (string, error) {
			p.backupListLock.Lock()
			defer p.backupListLock.Unlock()
			if len(p.backupList) > 0 {
				host := p.backupList[0]
				p.backupList = p.backupList[1:]
				return host, nil
			} else {
				return "", errors.New("end of the backup host")
			}
		}()
		if err != nil {
			return "", err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		id, err := peer.IDB58Decode(host)
		if err != nil {
			continue
		}
		if err := p.cp.Api.Swarm().Connect(ctx, peer.AddrInfo{ID: id}); err != nil {
			continue
		}
		return host, nil
	}
	return "", errors.New("can't reach here")
}

func (p *HostsProvider) NextValidHost(price int64) (string, error) {
	needHigherPrice := false
LOOP:
	for true {
		select {
		case <-p.ctx.Done():
			p.cancel()
			return "", errors.New(failMsg)
		default:
		}
		if index, err := p.AddIndex(); err == nil {
			host := p.hosts[index]
			for _, h := range p.blacklist {
				if h == host.NodeId {
					continue LOOP
				}
			}
			id, err := peer.IDB58Decode(host.NodeId)
			if err != nil || int64(host.StoragePriceAsk) > price {
				needHigherPrice = true
				continue
			}
			ctx, _ := context.WithTimeout(p.ctx, 3*time.Second)
			if err := p.cp.Api.Swarm().Connect(ctx, peer.AddrInfo{ID: id}); err != nil {
				p.Lock()
				p.hosts = append(p.hosts, host)
				p.Unlock()
				continue
			}
			return host.NodeId, nil
		} else if h, err := p.PickFromBackupHosts(); err == nil {
			return h, nil
		} else {
			break
		}
	}
	msg := failMsg
	if needHigherPrice {
		msg += " or raise price"
	}
	return "", errors.New(msg)
}
