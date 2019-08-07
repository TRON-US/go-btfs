package republisher_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/bootstrap"
	mock "github.com/TRON-US/go-btfs/core/mock"
	namesys "github.com/TRON-US/go-btfs/namesys"
	. "github.com/TRON-US/go-btfs/namesys/republisher"
	path "github.com/ipfs/go-path"

	goprocess "github.com/jbenet/goprocess"
	pstore "github.com/libp2p/go-libp2p-peerstore"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
)

func TestRepublish(t *testing.T) {
	// set cache life to zero for testing low-period repubs

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create network
	mn := mocknet.New(ctx)

	var nodes []*core.IpfsNode
	for i := 0; i < 10; i++ {
		nd, err := core.NewNode(ctx, &core.BuildCfg{
			Online: true,
			Host:   mock.MockHostOption(mn),
		})
		if err != nil {
			t.Fatal(err)
		}

		nd.Namesys = namesys.NewNameSystem(nd.Routing, nd.Repo.Datastore(), 0)

		nodes = append(nodes, nd)
	}

	if err := mn.LinkAll(); err != nil {
		t.Fatal(err)
	}

	bsinf := bootstrap.BootstrapConfigWithPeers(
		[]pstore.PeerInfo{
			nodes[0].Peerstore.PeerInfo(nodes[0].Identity),
		},
	)

	for _, n := range nodes[1:] {
		if err := n.Bootstrap(bsinf); err != nil {
			t.Fatal(err)
		}
	}

	// have one node publish a record that is valid for 1 second
	publisher := nodes[3]
	p := path.FromString("/btfs/QmUNLLsPACCz1vLxQVkXqqLX5R1X345qqfHbsf67hvA3Nn") // does not need to be valid
	rp := namesys.NewIpnsPublisher(publisher.Routing, publisher.Repo.Datastore())
	name := "/btns/" + publisher.Identity.Pretty()

	// Retry in case the record expires before we can fetch it. This can
	// happen when running the test on a slow machine.
	var expiration time.Time
	timeout := time.Second
	for {
		expiration = time.Now().Add(time.Second)
		err := rp.PublishWithEOL(ctx, publisher.PrivateKey, p, expiration)
		if err != nil {
			t.Fatal(err)
		}

		err = verifyResolution(nodes, name, p)
		if err == nil {
			break
		}

		if time.Now().After(expiration) {
			timeout *= 2
			continue
		}
		t.Fatal(err)
	}

	// Now wait a second, the records will be invalid and we should fail to resolve
	time.Sleep(timeout)
	if err := verifyResolutionFails(nodes, name); err != nil {
		t.Fatal(err)
	}

	// The republishers that are contained within the nodes have their timeout set
	// to 12 hours. Instead of trying to tweak those, we're just going to pretend
	// they dont exist and make our own.
	repub := NewRepublisher(rp, publisher.Repo.Datastore(), publisher.PrivateKey, publisher.Repo.Keystore())
	repub.Interval = time.Second
	repub.RecordLifetime = time.Second * 5

	proc := goprocess.Go(repub.Run)
	defer proc.Close()

	// now wait a couple seconds for it to fire
	time.Sleep(time.Second * 2)

	// we should be able to resolve them now
	if err := verifyResolution(nodes, name, p); err != nil {
		t.Fatal(err)
	}
}

func verifyResolution(nodes []*core.IpfsNode, key string, exp path.Path) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, n := range nodes {
		val, err := n.Namesys.Resolve(ctx, key)
		if err != nil {
			return err
		}

		if val != exp {
			return errors.New("resolved wrong record")
		}
	}
	return nil
}

func verifyResolutionFails(nodes []*core.IpfsNode, key string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, n := range nodes {
		_, err := n.Namesys.Resolve(ctx, key)
		if err == nil {
			return errors.New("expected resolution to fail")
		}
	}
	return nil
}
