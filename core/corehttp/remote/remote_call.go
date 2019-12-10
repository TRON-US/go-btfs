package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

type RemoteCall struct {
	URL string
	ID  string
}

func (r *RemoteCall) CallGet(ctx context.Context, api string, args []string) ([]byte, error) {
	var arg string
	for i, str := range args {
		if i == 0 {
			arg += fmt.Sprintf("?arg=%s", str)
		} else {
			arg += fmt.Sprintf("&arg=%s", str)
		}
	}
	curURL := r.URL + api + arg
	req, err := http.NewRequestWithContext(ctx, "GET", curURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET fail: %v", err)
	}
	defer resp.Body.Close()
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
