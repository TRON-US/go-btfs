package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/TRON-US/go-btfs/core"

	"github.com/libp2p/go-libp2p-core/peer"
	p2phttp "github.com/libp2p/go-libp2p-http"
)

type P2PRemoteCall struct {
	Node *core.IpfsNode
	ID   peer.ID
}

type ErrorMessage struct {
	Message string
	Code    int
	Type    string
}

const P2PRemoteCallProto = "/rapi"

// P2PCall is a wrapper for creating a client and calling a get
// If passed a nil context, a new one will be created
func P2PCall(ctx context.Context, n *core.IpfsNode, pid peer.ID, api string, args ...interface{}) ([]byte, error) {
	// new context if not caller-passed down
	if ctx == nil {
		ctx = context.Background()
	}
	remoteCall := &P2PRemoteCall{
		Node: n,
		ID:   pid,
	}
	return remoteCall.CallGet(ctx, api, args)
}

// P2PCallStrings is a helper to pass string arguments to P2PCall
func P2PCallStrings(ctx context.Context, n *core.IpfsNode, pid peer.ID, api string, strs ...string) ([]byte, error) {
	var args []interface{}
	for _, str := range strs {
		args = append(args, str)
	}
	return P2PCall(ctx, n, pid, api, args...)
}

func (r *P2PRemoteCall) CallGet(ctx context.Context, api string, args []interface{}) ([]byte, error) {
	var sb strings.Builder
	for i, arg := range args {
		if i == 0 {
			sb.WriteString("?")
		} else {
			sb.WriteString("&")
		}
		sb.WriteString("arg=")
		switch arg.(type) {
		case []byte:
			s := url.QueryEscape(string(arg.([]byte)))
			sb.WriteString(s)
		case string:
			sb.WriteString(arg.(string))
		}
	}
	// setup url
	reqUrl := fmt.Sprintf("libp2p://%s%s%s%s", r.ID.Pretty(), apiPrefix, api, sb.String())
	// perform context setup
	req, err := http.NewRequestWithContext(ctx, "GET", reqUrl, nil)
	if err != nil {
		return nil, err
	}
	// libp2p protocol register
	tr := &http.Transport{}
	tr.RegisterProtocol("libp2p",
		p2phttp.NewTransport(r.Node.PeerHost, p2phttp.ProtocolOption(P2PRemoteCallProto)))
	client := &http.Client{Transport: tr}
	// call
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fail to read response body: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		e := &ErrorMessage{}
		if err = json.Unmarshal(body, e); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf(e.Message)
	}
	return body, nil
}

func (r *P2PRemoteCall) CallPost() {
}
