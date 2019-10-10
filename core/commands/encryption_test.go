package commands

import (
	"bytes"
	"context"
	"errors"
	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/bootstrap"
	"github.com/TRON-US/go-btfs/core/coreapi"
	coremock "github.com/TRON-US/go-btfs/core/mock"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/interface-go-ipfs-core/options"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/libp2p/go-testutil"
	"io"
	"math"
	"testing"
	"time"
)

func TestEncryption(t *testing.T) {
	addOpts := []options.UnixfsAddOption{
		options.Unixfs.Encrypt(true),
		options.Unixfs.PeerId("16Uiu2HAmRih4otzcxyZ428QoQim8SptHZq5sjBxiUuiKr7ctPmMG"),
	}
	getOpts := []options.UnixfsGetOption{
		options.Unixfs.Decrypt(true),
		options.Unixfs.PrivateKey("1b69f9edb6f20ab64814cffdb7d76e7531f8e41664d943b849edabb78e3e5041"),
	}

	msg := "btt to da moon"
	conf := testutil.LatencyConfig{NetworkLatency: 400 * time.Millisecond}
	error := DirectAddCat([]byte(msg), conf, addOpts, getOpts)
	if error != nil {
		t.Fatal(error)
	}
}

func DirectAddCat(data []byte, conf testutil.LatencyConfig, addOpts []options.UnixfsAddOption,
	getOpts []options.UnixfsGetOption) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create network
	mn := mocknet.New(ctx)
	mn.SetLinkDefaults(mocknet.LinkOptions{
		Latency: conf.NetworkLatency,
		// TODO add to conf. This is tricky because we want 0 values to be functional.
		Bandwidth: math.MaxInt32,
	})

	adder, err := core.NewNode(ctx, &core.BuildCfg{
		Online: true,
		Host:   coremock.MockHostOption(mn),
	})
	if err != nil {
		return err
	}
	defer adder.Close()

	catter, err := core.NewNode(ctx, &core.BuildCfg{
		Online: true,
		Host:   coremock.MockHostOption(mn),
	})
	if err != nil {
		return err
	}
	defer catter.Close()

	adderApi, err := coreapi.NewCoreAPI(adder)
	if err != nil {
		return err
	}

	catterApi, err := coreapi.NewCoreAPI(catter)
	if err != nil {
		return err
	}

	err = mn.LinkAll()
	if err != nil {
		return err
	}

	bs1 := []peer.AddrInfo{adder.Peerstore.PeerInfo(adder.Identity)}
	bs2 := []peer.AddrInfo{catter.Peerstore.PeerInfo(catter.Identity)}

	if err := catter.Bootstrap(bootstrap.BootstrapConfigWithPeers(bs1)); err != nil {
		return err
	}
	if err := adder.Bootstrap(bootstrap.BootstrapConfigWithPeers(bs2)); err != nil {
		return err
	}

	added, err := adderApi.Unixfs().Add(ctx, files.NewBytesFile(data), addOpts...)
	if err != nil {
		return err
	}

	readerCatted, err := catterApi.Unixfs().Get(ctx, added, getOpts...)
	if err != nil {
		return err
	}

	// verify
	var bufout bytes.Buffer
	_, err = io.Copy(&bufout, readerCatted.(io.Reader))
	if err != nil {
		return err
	}
	if !bytes.Equal(bufout.Bytes(), data) {
		return errors.New("catted data does not match added data")
	}

	return nil
}
