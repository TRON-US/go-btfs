// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock_test

import (
	"testing"

	"github.com/TRON-US/go-btfs/statestore/mock"
	"github.com/TRON-US/go-btfs/statestore/test"
	"github.com/TRON-US/go-btfs/transaction/storage"
)

func TestMockStateStore(t *testing.T) {
	test.Run(t, func(t *testing.T) storage.StateStorer {
		return mock.NewStateStore()
	})
}
