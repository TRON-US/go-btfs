package hub

import (
	"context"
	"reflect"
	"testing"

	nodepb "github.com/tron-us/go-btfs-common/protos/node"
)

func TestGetSettings(t *testing.T) {
	ns, err := GetHostSettings(context.Background(), "https://hub.btfs.io",
		"QmWJWGxKKaqZUW4xga2BCzT5FBtYDL8Cc5Q5jywd6xPt1g")
	if err != nil {
		t.Fatal(err)
	}
	defNs := &nodepb.Node_Settings{StoragePriceAsk: 250000, StorageTimeMin: 30, StoragePriceDefault: 250000}
	if !reflect.DeepEqual(ns, defNs) {
		t.Fatal("default settings not equal")
	}
}
