package ipns

import (
	"context"

	"github.com/TRON-US/go-btfs/core"
	nsys "github.com/TRON-US/go-btfs/namesys"
	path "github.com/ipfs/go-path"
	ft "github.com/TRON-US/go-unixfs"
	ci "github.com/libp2p/go-libp2p-core/crypto"
)

// InitializeKeyspace sets the ipns record for the given key to
// point to an empty directory.
func InitializeKeyspace(n *core.IpfsNode, key ci.PrivKey) error {
	ctx, cancel := context.WithCancel(n.Context())
	defer cancel()

	emptyDir := ft.EmptyDirNode()

	err := n.Pinning.Pin(ctx, emptyDir, false)
	if err != nil {
		return err
	}

	err = n.Pinning.Flush()
	if err != nil {
		return err
	}

	pub := nsys.NewIpnsPublisher(n.Routing, n.Repo.Datastore())

	return pub.Publish(ctx, key, path.FromCid(emptyDir.Cid()))
}
