package remote

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("corehttp/remote")

type RemoteCall struct {
	URL string
	ID  string
}

// APIPath is the path at which the API is mounted.
const APIprefix = "/api/v0"

func (r *RemoteCall) CallGet(api string, args []string) ([]byte, error) {
	var arg string
	for i, str := range args {
		if i == 0 {
			arg += fmt.Sprintf("?arg=%s", str)
		} else {
			arg += fmt.Sprintf("&arg=%s", str)
		}
	}
	curURL := r.URL + api + arg
	resp, err := http.Get(curURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET fail: %v", err)
	}
	//defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fail to read response body: %s", err)
	}
	return body, nil
}

func UnmarshalResp(body []byte) (map[string]interface{}, error) {
	jsonResp := make(map[string]interface{})
	if err := json.Unmarshal(body, &jsonResp); err != nil {
		return nil, fmt.Errorf("fail to unmarshal json body: %s", err)
	}
	return jsonResp, nil
}

func (r *RemoteCall) CallPost() {
}
