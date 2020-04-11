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
func sendChallengeQuestions(ctx context.Context, cfg *config.Config,
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

// SubmitFileStatus opens a grpc connection, sends the file meta, and closes (short) connection
func SubmitFileStatus(ctx context.Context, cfg *config.Config,
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

// ListHostContracts opens a grpc connection, sends the host contract list request and
// closes (short) connection
// Returns list of contracts, and whether this is the last page
func ListHostContracts(ctx context.Context, cfg *config.Config,
	listReq *guardpb.ListHostContractsRequest) ([]*guardpb.Contract, bool, error) {
	cb := cgrpc.GuardClient(cfg.Services.GuardDomain)
	cb.Timeout(guardTimeout)
	var contracts []*guardpb.Contract
	var lastPage bool
	err := cb.WithContext(ctx, func(ctx context.Context, client guardpb.GuardServiceClient) error {
		res, err := client.ListHostContracts(ctx, listReq)
		if err != nil {
			return err
		}
		contracts = res.Contracts
		lastPage = res.Count < listReq.RequestPageSize
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return contracts, lastPage, nil
}
