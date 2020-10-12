package path

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

func list(root string) ([]string, error) {
	root = filepath.Clean(root)
	result := make([]string, 0)
	files, err := ioutil.ReadDir(root)
	if err != nil {
		return result, err
	}
	for _, f := range files {
		if hid, err := isHidden(filepath.Join(root, f.Name())); err != nil || hid {
			continue
		}
		if !f.IsDir() {
			continue
		}
		result = append(result, f.Name())
	}
	return result, nil
}

func add(parent string, name string) error {
	dst := filepath.Clean(filepath.Join(parent, name))
	b, err := exist(dst)
	if err != nil {
		return err
	}
	if b {
		return fmt.Errorf("dir %s already exists", dst)
	}
	return os.MkdirAll(dst, os.ModeDir)
}

func exist(name string) (bool, error) {
	_, err := os.Stat(name)
	if err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

func isDir(name string) (bool, error) {
	stat, err := os.Stat(name)
	if err != nil {
		return false, err
	}
	return stat.IsDir(), nil
}
