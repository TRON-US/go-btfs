// +build !windows,nofuse

package node

import (
	"errors"

	core "github.com/TRON-US/go-btfs/core"
)

func Mount(node *core.IpfsNode, fsdir, nsdir string) error {
	return errors.New("not compiled in")
}
