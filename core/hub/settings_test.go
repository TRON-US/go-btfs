package hub

import (
	"context"
	"fmt"
	"log"
	"testing"

	"github.com/ipfs/go-datastore"
	syncds "github.com/ipfs/go-datastore/sync"
)

func TestGetSettings(t *testing.T) {
	d := syncds.MutexWrap(datastore.NewMapDatastore())
	ns, err := GetSettings(context.Background(), "https://hub-dev.btfs.io",
		"16Uiu2HAm9P1cur6Nhd542y7pM2EoXgVvGeNqdUCSLFAMooBeQqWy", d)
	if err != nil {
		log.Panic(err)
	}
	fmt.Println("settings", ns)
}
