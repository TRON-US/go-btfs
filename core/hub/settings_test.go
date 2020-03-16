package hub

import (
	"context"
	"reflect"
	"testing"

	nodepb "github.com/tron-us/go-btfs-common/protos/node"
)

func TestGetSettings(t *testing.T) {
	ns, err := GetHostSettings(context.Background(), "https://hub-staging.btfs.io",
		"16Uiu2HAm9P1cur6Nhd542y7pM2EoXgVvGeNqdUCSLFAMooBeQqWy")
	if err != nil {
		t.Fatal(err)
	}
	defNs := &nodepb.Node_Settings{StoragePriceAsk: 250000, StorageTimeMin: 30}
	if !reflect.DeepEqual(ns, defNs) {
		t.Fatal("default settings not equal")
	}
}
