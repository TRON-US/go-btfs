package spin

import (
	"context"
	"time"
)

func periodicSync(period, timeout time.Duration, msg string, syncFunc func(context.Context) error) {
	tick := time.NewTicker(period)
	defer tick.Stop()
	// Force tick on immediate start
	for ; true; <-tick.C {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		err := syncFunc(ctx)
		if err != nil {
			log.Errorf("Failed to sync %s: %s", msg, err)
		}
	}
}

func periodicChallengeReq(period time.Duration, msg string, syncFunc func(context.Context) error) {
	tick := time.NewTicker(period)
	defer tick.Stop()
	for ; true; <-tick.C {
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		err := syncFunc(ctx)
		if err != nil {
			log.Errorf("Failed to challenge %s: %s", msg, err)
		}
	}
}
