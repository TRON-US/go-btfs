package guard

import (
	"context"
	"fmt"

	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	cgrpc "github.com/tron-us/go-btfs-common/utils/grpc"

	config "github.com/TRON-US/go-btfs-config"
)

// sendChallengeQuestions opens a grpc connection, sends questions, and closes (short) connection
func sendChallengeQuestions(ctx context.Context, cfg *config.Config,
	req *guardpb.FileChallengeQuestions) error {
	cb := cgrpc.GuardClient(cfg.Services.GuardDomain)
	return cb.WithContext(context.Background(), func(ctx context.Context, client guardpb.GuardServiceClient) error {
		res, err := client.SendQuestions(ctx, req)
		if err != nil {
			return err
		}
		if res.Code != guardpb.ResponseCode_SUCCESS {
			return fmt.Errorf("failed to send questions: %v", res.Message)
		}
		return nil
	})
}
