package fsrepo

import (
	"os"

	"github.com/mitchellh/go-homedir"
)

// BestKnownPath returns the best known fsrepo path. If the ENV override is
// present, this function returns that value. Otherwise, it returns the default
// repo path.
func BestKnownPath() (string, error) {
	ipfsPath := "~/.btfs"
	if os.Getenv("BTFS_PATH") != "" {
		ipfsPath = os.Getenv("BTFS_PATH")
	}
	ipfsPath, err := homedir.Expand(ipfsPath)
	if err != nil {
		return "", err
	}
	return ipfsPath, nil
}
