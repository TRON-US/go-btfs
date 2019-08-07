package remote

import (
	"github.com/TRON-US/go-btfs/core"

	cmds "github.com/TRON-US/go-btfs-cmds"
	cmdsHttp "github.com/TRON-US/go-btfs-cmds/http"
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
