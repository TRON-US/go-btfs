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
	defaultDownloadPath := *flag.String("download", "/tmp/", "default download path")

	flag.Parse()

	currentVersionPath := fmt.Sprint(defaultProjectPath, "version.txt")
	latestVersionPath := fmt.Sprint(defaultDownloadPath, "version-latest.txt")
	btfsBackupPath := fmt.Sprint(defaultDownloadPath, "btfs.bk")
	btfsBinaryPath := fmt.Sprint(defaultProjectPath, "btfs")
	latestBtfsBinaryPath := fmt.Sprint(defaultDownloadPath, "btfs-latest")

	err := os.Remove(currentVersionPath)
	if err != nil {
		return
	}

	err = os.Rename(latestVersionPath, currentVersionPath)
	if err != nil {
		return
	}

	err = os.Remove(btfsBackupPath)
	if err != nil {
		return
	}

	err = os.Rename(btfsBinaryPath, btfsBackupPath)
	if err != nil {
		return
	}

	err = os.Rename(latestBtfsBinaryPath, btfsBinaryPath)
	if err != nil {
		return
	}

	cmd := exec.Command(btfsBinaryPath, "daemon")
	err = cmd.Start()
}
