package guard

import (
	"context"
	"fmt"
	"time"

	guardpb "github.com/tron-us/go-btfs-common/protos/guard"
	cgrpc "github.com/tron-us/go-btfs-common/utils/grpc"

	config "github.com/TRON-US/go-btfs-config"
)

const (
	guardTimeout = 360 * time.Second
)

// sendChallengeQuestions opens a grpc connection, sends questions, and closes (short) connection
func SendChallengeQuestions(ctx context.Context, cfg *config.Config,
	req *guardpb.FileChallengeQuestions) error {
	cb := cgrpc.GuardClient(cfg.Services.GuardDomain)
	cb.Timeout(guardTimeout)
	return cb.WithContext(ctx, func(ctx context.Context, client guardpb.GuardServiceClient) error {
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

// submitFileStatus opens a grpc connection, sends the file meta, and closes (short) connection
func submitFileStatus(ctx context.Context, cfg *config.Config,
	fileStatus *guardpb.FileStoreStatus) error {
	cb := cgrpc.GuardClient(cfg.Services.GuardDomain)
	cb.Timeout(guardTimeout)
	return cb.WithContext(ctx, func(ctx context.Context, client guardpb.GuardServiceClient) error {
		res, err := client.SubmitFileStoreMeta(ctx, fileStatus)
		if err != nil {
			return err
		}
		if res.Code != guardpb.ResponseCode_SUCCESS {
			return fmt.Errorf("failed to execute submit file status to gurad: %v", res.Message)
		}
		return nil
	})
}
