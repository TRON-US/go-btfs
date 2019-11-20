package guard

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	guardpb "github.com/tron-us/go-btfs-common/protos/guard"

	config "github.com/TRON-US/go-btfs-config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	dialTimeout = 30 * time.Second
	callTimeout = 1 * time.Minute
)

// getGrpcConn returns a connected grpc client to the guard service
func getGrpcConn(ctx context.Context, cfg *config.Config) (*grpc.ClientConn, error) {
	var dialOptions []grpc.DialOption
	if strings.HasPrefix(cfg.Services.GuardDomain, "https://") {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	} else {
		dialOptions = append(dialOptions, grpc.WithInsecure())
	}
	ctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel() // by default, DialContext is non-blocking, so won't use this ctx
	conn, err := grpc.DialContext(ctx, cfg.Services.GuardDomain, dialOptions...)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// sendChallengeQuestions opens a grpc connection, sends questions, and closes (short) connection
func sendChallengeQuestions(ctx context.Context, cfg *config.Config,
	req *guardpb.FileChallengeQuestions) error {
	conn, err := getGrpcConn(ctx, cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	client := guardpb.NewGuardServiceClient(conn)
	res, err := client.SendQuestions(ctx, req)
	if err != nil {
		return err
	}
	if res.Code != guardpb.ResponseCode_SUCCESS {
		return fmt.Errorf("failed to send questions: %v", res.Message)
	}
	return nil
}
