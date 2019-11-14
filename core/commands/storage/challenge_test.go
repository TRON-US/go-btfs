package storage

import (
	"context"
	"testing"

	unixtest "github.com/TRON-US/go-btfs/core/coreunix/test"
)

func TestGenAndSolveChallenge(t *testing.T) {
	node, api, root := unixtest.HelpTestAddWithReedSolomonMetadata(t)

	sc, err := NewStorageChallenge(context.Background(), node, api, root)
	if err != nil {
		t.Fatal(err)
	}

	// simulate 100 different challenges for the same file
	chs := map[string]bool{}
	for i := 0; i < 100; i++ {
		err := sc.GenChallenge()
		if err != nil {
			t.Fatal(err)
		}

		// should not re-generate duplicate challenges
		if _, ok := chs[sc.Hash]; ok {
			t.Fatal("Duplicate challenge generated")
		}
		chs[sc.Hash] = true

		scr := NewStorageChallengeResponse(context.Background(), node, api, sc.ID)

		err = scr.SolveChallenge(sc.CID, sc.Nonce)
		if err != nil {
			t.Fatal(err)
		}

		if scr.Hash != sc.Hash {
			t.Fatal("Challenge is not solved, not matching original hash")
		}
	}
}
