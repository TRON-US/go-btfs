package hub

import (
	"context"
	"errors"
	"fmt"

	"github.com/TRON-US/go-btfs/repo"
	hubpb "github.com/tron-us/go-btfs-common/protos/hub"
	nodepb "github.com/tron-us/go-btfs-common/protos/node"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/ipfs/go-datastore"
)

var (
	keyFormat = "/btfs/%s/settings/v1"
)

func GetSettings(ctx context.Context, addr string, peerId string, rds datastore.Datastore) (*nodepb.Node_Settings, error) {
	k := fmt.Sprintf(keyFormat, peerId)
	s := new(nodepb.Node_Settings)
	settings, err := repo.Get(rds, k, s)
	if err == nil {
		n := settings.(*nodepb.Node_Settings)
		return n, err
	}

	// get from remote
	ns := new(nodepb.Node_Settings)
	err = grpc.HubQueryClient(addr).WithContext(ctx, func(ctx context.Context, client hubpb.HubQueryServiceClient) error {
		req := new(hubpb.SettingsReq)
		req.Id = peerId
		resp, err := client.GetSettings(ctx, req)
		if err != nil {
			return err
		}
		if resp.Code != hubpb.ResponseCode_SUCCESS {
			return errors.New(resp.Message)
		}
		ns.StorageTimeMin = uint64(resp.SettingsData.StorageTimeMin)
		ns.StoragePriceAsk = uint64(resp.SettingsData.StoragePriceAsk)
		ns.BandwidthLimit = resp.SettingsData.BandwidthLimit
		ns.BandwidthPriceAsk = uint64(resp.SettingsData.BandwidthPriceAsk)
		ns.CollateralStake = uint64(resp.SettingsData.CollateralStake)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ns, repo.Put(rds, k, ns)
}
