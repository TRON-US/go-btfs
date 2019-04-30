package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

func main() {
	time.Sleep(time.Second * 5)
	defaultProjectPath := flag.String("project", "", "default project path")
	defaultDownloadPath := flag.String("download", "", "default download path")

	flag.Parse()

	if *defaultProjectPath == "" || *defaultDownloadPath == "" {
		fmt.Println("Request param is nil.")
		return
	}

	var currentVersionPath string
	var latestVersionPath string
	var btfsBackupPath string
	var btfsBinaryPath string
	var latestBtfsBinaryPath string

	// Select binary files based on operating system.
	if (runtime.GOOS == "darwin" && runtime.GOARCH == "amd64") || (runtime.GOOS == "linux" && runtime.GOARCH == "amd64") {
		currentVersionPath = fmt.Sprint(*defaultProjectPath, "config.yaml")
		latestVersionPath = fmt.Sprint(*defaultDownloadPath, fmt.Sprintf("config_%s_%s.yaml", runtime.GOOS, runtime.GOARCH))
		btfsBackupPath = fmt.Sprint(*defaultDownloadPath, "btfs.bk")
		btfsBinaryPath = fmt.Sprint(*defaultProjectPath, "btfs")
		latestBtfsBinaryPath = fmt.Sprint(*defaultDownloadPath, fmt.Sprintf("btfs-%s-%s", runtime.GOOS, runtime.GOARCH))
	} else if runtime.GOOS == "windows" && runtime.GOARCH == "386" {
		currentVersionPath = fmt.Sprint(*defaultProjectPath, "config.yaml")
		latestVersionPath = fmt.Sprint(*defaultDownloadPath, fmt.Sprintf("config_%s_%s.yaml", runtime.GOOS, runtime.GOARCH))
		btfsBackupPath = fmt.Sprint(*defaultDownloadPath, "btfs.exe.bk")
		btfsBinaryPath = fmt.Sprint(*defaultProjectPath, "btfs.exe")
		latestBtfsBinaryPath = fmt.Sprint(*defaultDownloadPath, fmt.Sprintf("btfs-%s-%s.exe", runtime.GOOS, runtime.GOARCH))
	} else {
		fmt.Printf("Operating system [%s], arch [%s] does not support automatic updates\n", runtime.GOOS, runtime.GOARCH)
		return
	}

	var err error

	// Delete current config file if exists.
	if pathExists(currentVersionPath) {
		err = os.Remove(currentVersionPath)
		if err != nil {
			fmt.Printf("Delete current config file error, reasons: [%v]\n", err)
			return
		}
	}

	// Move latest config file to current config file.
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
		fmt.Printf("Move file error, reasons: [%v]\n", err)
		return
	}

	// Move latest btfs binary file to current btfs binary file.
	err = os.Rename(latestBtfsBinaryPath, btfsBinaryPath)
	if err != nil {
		fmt.Printf("Move file error, reasons: [%v]\n", err)
		return
	}

	// Add executable permissions to btfs binary.
	err = os.Chmod(btfsBinaryPath, 0775)
	if err != nil {
		fmt.Printf("Chmod file error, reasons: [%v]\n", err)
		return
	}

	if runtime.GOOS == "windows" {
		cmd := exec.Command(btfsBinaryPath, "daemon")
		err = cmd.Start()
	} else {
		fmt.Println(btfsBinaryPath)
		cmd := exec.Command("sudo", btfsBinaryPath, "daemon")
		err = cmd.Start()
	}
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
