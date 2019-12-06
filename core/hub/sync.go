package hub

import (
	"context"
	"fmt"

	"github.com/TRON-US/go-btfs/core"

	hubpb "github.com/tron-us/go-btfs-common/protos/hub"
	"github.com/tron-us/go-btfs-common/utils/grpc"
)

const (
	HubModeAll   = "all"
	HubModeScore = "score"
	HubModeGeo   = "geo"
	HubModeRep   = "rep"
	HubModePrice = "price"
	HubModeSpeed = "speed"
)

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
func QueryHub(node *core.IpfsNode, mode string) ([]*hubpb.Host, error) {
	switch mode {
	case HubModeScore:
		// Already the default on hub api
	default:
		return nil, fmt.Errorf(`Mode "%s" is not yet supported`, mode)
	}

	config, err := node.Repo.Config()
	if err != nil {
		return nil, err
	}
	var resp *hubpb.HostsResp
	err = grpc.HubQueryClient(config.Services.HubDomain).WithContext(context.Background(), func(ctx context.Context,
		client hubpb.HubQueryServiceClient) error {
		resp, err = client.GetHosts(ctx, &hubpb.HostsReq{
			Id: node.Identity.Pretty(),
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to query BTFS-Hub service: %v", err)
	}

	return resp.Hosts.Hosts, nil
}
