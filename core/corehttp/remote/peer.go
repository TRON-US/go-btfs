package remote

import (
	"context"
	cmds "github.com/TRON-US/go-btfs-cmds"
	cmdsHttp "github.com/TRON-US/go-btfs-cmds/http"
	"github.com/TRON-US/go-btfs/core"
	peer "github.com/libp2p/go-libp2p-core/peer"
)

// GetStreamRequestRemotePeerID checks to see if current request is part of a streamedd
// libp2p connection, if yes, return the remote peer id, otherwise return false.
func GetStreamRequestRemotePeerID(req *cmds.Request, node *core.IpfsNode) (peer.ID, bool) {
	remoteAddr, ok := cmdsHttp.GetRequestRemoteAddr(req.Context)
	if !ok {
		return "", false
	}
	return node.P2P.Streams.GetStreamRemotePeerID(remoteAddr)
}

// FindPeer decodes a string-based peer id and tries to find it in the current routing
// table (if not connected, will retry).
func FindPeer(ctx context.Context, n *core.IpfsNode, pid string) (*peer.AddrInfo, error) {
	id, err := peer.IDB58Decode(pid)
	if err != nil {
		log.Error("error decode:", pid, err)
		return nil, err
	}
	pinfo, err := n.Routing.FindPeer(ctx, id)
	if err != nil {
		log.Error("error finding peer:", pinfo, err)
		return nil, err
	}
	return &pinfo, nil
}
