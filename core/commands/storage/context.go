package storage

import (
	"context"
	"time"
)

const (
	// Normally each op has its own timeout, but in case of buggy paths
	// this timeout serves as a catch-all to prevent mem leaks
	DefaultStorageTimeout = 5 * time.Minute
)

// NewGoContext creates a new context with remaining timeout from the existing
// request context, so that request can be cancelled and new goroutine should
// use this new context.
func NewGoContext(reqCtx context.Context) context.Context {
	if dl, ok := reqCtx.Deadline(); ok {
		ctx, _ := context.WithDeadline(context.Background(), dl)
		return ctx
	}
	ctx, _ := context.WithTimeout(context.Background(), DefaultStorageTimeout)
	return ctx
}

func NewCancelableGoContext(reqCtx context.Context) (context.Context, context.CancelFunc) {
	if dl, ok := reqCtx.Deadline(); ok {
		return context.WithDeadline(context.Background(), dl)
	}
	return context.WithTimeout(context.Background(), DefaultStorageTimeout)
}
