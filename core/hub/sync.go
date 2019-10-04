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
	hubUrl = "https://query-btfs-dev.bt.co/hosts"
)

type hostsQuery struct {
	Hosts []*info.Node `json:"hosts"`
	// Ignore other fields
}

// CheckValidMode checks if a given host selection/sync mode is valid or not.
func CheckValidMode(mode string) error {
	switch mode {
	case HubModeAll, HubModeScore, HubModeGeo, HubModeRep, HubModePrice, HubModeSpeed:
		return nil
	}
	return fmt.Errorf("Invalid host mode: %s", mode)
}

// QueryHub queries the BTFS-Hub to retrieve the latest list of hosts info
// according to a certain mode.
func QueryHub(nodeID, mode string) ([]*info.Node, error) {
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
