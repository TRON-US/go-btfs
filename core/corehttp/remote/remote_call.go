package remote

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	peer "github.com/libp2p/go-libp2p-peer"
)

type RemoteCall struct {
	URL string
	ID peer.ID
	Call
}

const prefix  = "/x/test/http/api/v0"

func (r *RemoteCall) CallGet(api string, args []string) (map[string]interface{}, error) {
	var arg string
	for _, str := range args {
		arg += fmt.Sprintf("arg=%s&", str)
	}
	resp, err := http.Get(r.URL+r.ID.Pretty()+prefix+api+arg)
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
