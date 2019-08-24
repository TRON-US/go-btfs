package hub

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tron-us/go-btfs-common/info"
)

const (
	HubModeAll   = "all"
	HubModeScore = "score"
	HubModeGeo   = "geo"
	HubModeRep   = "rep"
	HubModePrice = "price"
	HubModeSpeed = "speed"
)

var (
	// TODO: Use domain url
	hubUrl = "http://35.165.30.189:5100/hosts"
)

type hostsQuery struct {
	Hosts []*info.Node `json:"hosts"`
	// Ignore other fields
}

// QueryHub queries the BTFS-Hub to retrieve the latest list of hosts info
// according to a certain mode.
func QueryHub(nodeID, mode string) ([]*info.Node, error) {
	// FIXME: Current hub does not support real node id yet
	nodeID = "1"
	params := "?id=" + nodeID
	switch mode {
	case HubModeScore:
		// Already the default on hub api
	default:
		return nil, fmt.Errorf(`Mode "%s" is not yet supported`, mode)
	}

	resp, err := http.Get(hubUrl + params)
	if err != nil {
		return nil, fmt.Errorf("Failed to query BTFS-Hub service: %v", err)
	}

	var hq hostsQuery
	if err := json.NewDecoder(resp.Body).Decode(&hq); err != nil {
		return nil, err
	}

	return hq.Hosts, nil
}
