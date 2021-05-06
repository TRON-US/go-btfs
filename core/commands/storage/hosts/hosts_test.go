package hosts

import (
	"context"
	coremock "github.com/TRON-US/go-btfs/core/mock"
	"testing"
)

func TestSyncHostsMixture(t *testing.T) {
	n, err := coremock.NewMockNode()
	if err != nil {
		t.Fatal(err)
	}
	_, err = SyncHostsMixture(context.Background(), n, "{\"score\":1,\"geo\":2}")
	if err != nil {
		t.Fatal(err)
	}
}
