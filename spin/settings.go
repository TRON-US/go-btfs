package spin

import (
	"context"

	"github.com/TRON-US/go-btfs/core"
	"github.com/TRON-US/go-btfs/core/commands/storage"

	hubpb "github.com/tron-us/go-btfs-common/protos/hub"
	"github.com/tron-us/go-btfs-common/utils/grpc"

	"github.com/gogo/protobuf/proto"
)

func Settings(node *core.IpfsNode) error {
	c, err := node.Repo.Config()
	if err != nil {
		log.Errorf("Failed to get configuration %s", err)
		return err
	}

	if c.Experimental.StorageHostEnabled {
		rds := node.Repo.Datastore()
		peerId := node.Identity.Pretty()
		if exists, err := rds.Has(storage.GetHostStorageKey(peerId)); err != nil || exists {
			return err
		}
		go func() {
			log.Info("Fetch host storage from remote")
			var resp *hubpb.SettingsResp
			// TODO: retry
			err := grpc.HubQueryClient(c.Services.HubDomain).WithContext(context.Background(),
				func(ctx context.Context, client hubpb.HubQueryServiceClient) error {
					req := new(hubpb.SettingsReq)
					req.Id = peerId
					resp, err = client.GetSettings(ctx, req)
					return err
				})
			if err != nil {
				return
			}
			bytes, err := proto.Marshal(resp.SettingsData)
			if err != nil {
				return
			}
			rds.Put(storage.GetHostStorageKey(peerId), bytes)
			if err != nil {
				return
			}
		}()
	}

	return nil
}
