package remote

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/TRON-US/go-btfs/core"

	peer "github.com/libp2p/go-libp2p-core/peer"
	p2phttp "github.com/libp2p/go-libp2p-http"
)

type P2PRemoteCall struct {
	Node *core.IpfsNode
	ID   peer.ID
}

const P2PRemoteCallProto = "/rapi"

func (r *P2PRemoteCall) CallGet(api string, args []string) (map[string]interface{}, error) {
	var sb strings.Builder
	for i, str := range args {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString("arg=")
		sb.WriteString(str)
	}
	tr := &http.Transport{}
	tr.RegisterProtocol("libp2p",
		p2phttp.NewTransport(r.Node.PeerHost, p2phttp.ProtocolOption(P2PRemoteCallProto)))
	client := &http.Client{Transport: tr}
	resp, err := client.Get(fmt.Sprintf("libp2p://%s%s%s%s",
		r.ID.Pretty(), apiPrefix, api, sb.String()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fail to read response body: %s", err)
	}
	jsonResp := map[string]interface{}{}
	if err := json.Unmarshal([]byte(body), &jsonResp); err != nil {
		return nil, fmt.Errorf("fail to unmarshal json body: %s", err)
	}
	return jsonResp, nil
}

func (r *P2PRemoteCall) CallPost() {
}
