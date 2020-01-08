package hub

import (
	"context"
	"fmt"
	"strings"

	"github.com/TRON-US/go-btfs/core"

	hubpb "github.com/tron-us/go-btfs-common/protos/hub"
	"github.com/tron-us/go-btfs-common/utils/grpc"
)

const (
	HubModeAll = "all" // special all case (for local reading)

	AllModeHelpText = `
- "score":   top overall score
- "geo":     closest location
- "rep":     highest reputation
- "price":   lowest price
- "speed":   highest transfer speed
- "testnet": testnet-specific
- "all":     all existing hosts`
)

// CheckValidMode takes in a raw mode string and returns the host request mode type
// if valid, and if local is true and mode is empty, return prefix for storing such
// information into local datastore.
func CheckValidMode(mode string, local bool) (hubpb.HostsReq_Mode, string, error) {
	if mode == HubModeAll && local {
		return -1, "", nil
	}
	if m, ok := hubpb.HostsReq_Mode_value[strings.ToUpper(mode)]; ok {
		return hubpb.HostsReq_Mode(m), mode, nil
	}
	return -1, "", fmt.Errorf("Invalid Hub query mode: %s", mode)
}

// QueryHub queries the BTFS-Hub to retrieve the latest list of hosts info
// according to a certain mode.
func QueryHub(ctx context.Context, node *core.IpfsNode, mode string) ([]*hubpb.Host, error) {
	hrm, _, err := CheckValidMode(mode, false)
	if err != nil {
		return nil, err
	}
	config, err := node.Repo.Config()
	if err != nil {
		return nil, err
	}
	var resp *hubpb.HostsResp
	err = grpc.HubQueryClient(config.Services.HubDomain).WithContext(ctx, func(ctx context.Context,
		client hubpb.HubQueryServiceClient) error {
		resp, err = client.GetHosts(ctx, &hubpb.HostsReq{
			Id:   node.Identity.Pretty(),
			Mode: hrm,
		})
		if err != nil {
			return err
		}
		if resp.Code != hubpb.ResponseCode_SUCCESS {
			return fmt.Errorf(resp.Message)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to query BTFS-Hub service: %v", err)
	}

	return resp.Hosts.Hosts, nil
}
