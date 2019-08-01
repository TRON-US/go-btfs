package node

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	peer "github.com/libp2p/go-libp2p-peer"
)

type Call interface {
	CallGet()
	CallPost()
}

type RemoteCall struct {
	URL string
	ID peer.ID
	Call
}

func (r *RemoteCall) CallGet(prefix string, args []string) (map[string]interface{}, error) {
	var arg string
	for _, str := range args {
		arg += fmt.Sprintf("arg=%s&", str)
	}
	resp, err := http.Get(r.URL+r.ID.Pretty()+prefix+arg)
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

