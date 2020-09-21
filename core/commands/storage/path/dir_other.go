// +build !windows

package path

import "strings"

func isHidden(name string) (bool, error) {
	return strings.HasPrefix(name, "."), nil
}
