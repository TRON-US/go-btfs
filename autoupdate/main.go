package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

func main() {
	time.Sleep(time.Second * 5)
	defaultProjectPath := *flag.String("project", "/usr/local/bin/", "default project path")
	defaultDownloadPath := *flag.String("download", fmt.Sprint(os.TempDir(), "/"), "default download path")

	flag.Parse()

	currentVersionPath := fmt.Sprint(defaultProjectPath, "version.txt")
	latestVersionPath := fmt.Sprint(defaultDownloadPath, "version-latest.txt")
	btfsBackupPath := fmt.Sprint(defaultDownloadPath, "btfs.bk")
	btfsBinaryPath := fmt.Sprint(defaultProjectPath, "btfs")
	latestBtfsBinaryPath := fmt.Sprint(defaultDownloadPath, "btfs-latest")

	var err error

	// Delete current version file if exists.
	if pathExists(currentVersionPath) {
		err = os.Remove(currentVersionPath)
		if err != nil {
			fmt.Printf("Delete current version file error, reasons: [%v]\n", err)
			return
		}
	}

	// Move latest version file to current version file.
	err = os.Rename(latestVersionPath, currentVersionPath)
	if err != nil {
		fmt.Printf("Move file error, reasons: [%v]\n", err)
		return
	}

	// Delete btfs backup file.
	if pathExists(btfsBackupPath) {
		err = os.Remove(btfsBackupPath)
		if err != nil {
			fmt.Printf("Move file error, reasons: [%v]\n", err)
			return
		}
	}

	// Backup btfs binary file.
	err = os.Rename(btfsBinaryPath, btfsBackupPath)
	if err != nil {
		return
	}

	// Move latest btfs binary file to current btfs binary file.
	err = os.Rename(latestBtfsBinaryPath, btfsBinaryPath)
	if err != nil {
		return
	}

	// Add executable permissions to btfs binary.
	err = os.Chmod(btfsBinaryPath, 0775)
	if err != nil {
		return
	}

	cmd := exec.Command("sudo", btfsBinaryPath, "daemon")
	err = cmd.Start()
}

// Determine if the path file exists.
func pathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}
