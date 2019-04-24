// This auto update project just for ubuntu operating system.
package main

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	LatestVersionFile  = "version-latest.txt"
	CurrentVersionFile = "version.txt"
	UpdateBinary       = "update"
	LatestBtfsBinary   = "btfs-latest"
)

// You can add multiple addresses inside this array.
var url = [1]string{
	"http://13.59.36.232:8080/btns/QmaCa4qNJizZD5uwmprNoZ4MSHsGosK4oLDL6eqnVLQhTC/",
}

// Auto update function.
func update() {
	// Get file download path.
	defaultDownloadPath := os.TempDir()

	// Get current program execution path.
	defaultBtfsPath, err := getCurrentPath()
	if err != nil {
		log.Errorf("Get current program execution path error, reasons: [%v]", err)
		return
	}

	latestVersionPath := fmt.Sprint(defaultDownloadPath, "/", LatestVersionFile)
	currentVersionPath := fmt.Sprint(defaultBtfsPath, "/", CurrentVersionFile)
	latestBtfsBinaryPath := fmt.Sprint(defaultDownloadPath, "/", LatestBtfsBinary)
	updateBinaryPath := fmt.Sprint(defaultDownloadPath, "/", UpdateBinary)

	for {
		log.Info("BTFS node AutoUpdater begin.")

		time.Sleep(time.Second * 20)

		rand.Seed(time.Now().UnixNano())
		randNum := rand.Intn(len(url))

		if pathExists(latestVersionPath) {
			// Delete the btfs-latest file.
			err = os.Remove(latestVersionPath)
			if err != nil {
				log.Errorf("Remove version-latest.txt file error, reasons: [%v]", err)
				continue
			}
		}

		// Get binary version.
		err := download(latestVersionPath, fmt.Sprint(url[randNum], LatestVersionFile))
		if err != nil {
			log.Error("Download version-latest.txt file failed.")
			continue
		}

		// Get latest version string.
		latestVersion, err := getVersion(latestVersionPath)
		if err != nil {
			log.Error("Open latest version file error.")
			continue
		}

		var currentVersion string
		// Get current version string.
		currentVersion, err = getVersion(currentVersionPath)
		if err != nil {
			currentVersion = "0.0.0"
		}

		// Compare version.
		flg, err := versionCompare(latestVersion, currentVersion)
		if err != nil {
			log.Errorf("Version compare error, reasons: [%v]", err)
			continue
		}

		if flg <= 0 {
			log.Info("Btfs binary from btns version level is small than current version.")
			continue
		}

		// Determine if the btfs-latest file exists.
		if pathExists(latestBtfsBinaryPath) {
			// Delete the btfs-latest file.
			err = os.Remove(latestBtfsBinaryPath)
			if err != nil {
				log.Errorf("Remove btfs-latest file error, reasons: [%v]", err)
				continue
			}
		}

		// Get the btfs-latest file from btns.
		err = download(latestBtfsBinaryPath, fmt.Sprint(url[randNum], LatestBtfsBinary))
		if err != nil {
			log.Error("Download btfs-latest file failed.")
			continue
		}

		// Delete the btfs-latest file if exists.
		if pathExists(updateBinaryPath) {
			err = os.Remove(updateBinaryPath)
			if err != nil {
				log.Errorf("Remove update.sh file error, reasons: [%v]", err)
				continue
			}
		}

		// Get the update.sh file from btns.
		err = download(updateBinaryPath, fmt.Sprint(url[randNum], UpdateBinary))
		if err != nil {
			log.Error("Download update.sh file failed.")
			continue
		}

		// Add executable permissions to update binary.
		err = os.Chmod(updateBinaryPath, 0775)
		if err != nil {
			log.Error("Chmod file 775 error, reasons: [%v]", err)
			continue
		}

		// Start the btfs-updater binary process.
		cmd := exec.Command("sudo", updateBinaryPath, "-project", fmt.Sprint(defaultBtfsPath, "/"), "-download", fmt.Sprint(defaultDownloadPath, "/"))
		err = cmd.Start()
		if err != nil {
			log.Error(err)
			continue
		}
		os.Exit(0)
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

// Get version from file, e.g.(1.0.0).
func getVersion(file string) (string, error) {
	// Read file.
	versionFile, err := os.Open(file)
	if err != nil {
		log.Errorf("Open file failed, reasons: [%v]", err)
		return "", err
	}
	defer func() {
		_ = versionFile.Close()
	}()

	// New reader of file.
	buffer := bufio.NewReader(versionFile)

	// Read line.
	version, _, c := buffer.ReadLine()
	if c == io.EOF {
		log.Error("Version line is nil")
		return "", errors.New("version line is nil")
	}

	return string(version), nil
}

// Compare version.
func versionCompare(version1, version2 string) (int, error) {
	// Split string of version1.
	s1 := strings.Split(version1, ".")
	if s1 == nil || len(s1) != 3 {
		log.Error("String fo version1 has wrong format.")
		return 0, errors.New("string fo version1 has wrong format")
	}

	// Split string of version2.
	s2 := strings.Split(version2, ".")
	if s2 == nil || len(s2) != 3 {
		log.Error("String fo version2 has wrong format.")
		return 0, errors.New("string fo version2 has wrong format")
	}

	for i := 0; i < 3; i++ {
		// Convert version1 from string to int.
		int1, err := strconv.Atoi(s1[i])
		if err != nil {
			log.Errorf("Convert version1 from string to int error, reasons: [%v]", err)
			return 0, err
		}

		// Convert version2 from string to int.
		int2, err := strconv.Atoi(s2[i])
		if err != nil {
			log.Errorf("Convert version2 from string to int error, reasons: [%v]", err)
			return 0, err
		}

		if int1 > int2 {
			return 1, nil
		} else if int1 < int2 {
			return -1, nil
		}
	}
	return 0, nil
}

// Get current program execution path.
func getCurrentPath() (string, error) {
	ex, err := os.Executable()
	if err != nil {
		return "", err
	}
	exPath := filepath.Dir(ex)
	return exPath, nil
}

// http get download function.
func download(downloadPath, url string) error {
	// http get.
	res, err := http.Get(url)
	if err != nil {
		log.Errorf("Http get error, reasons: [%v]", err)
		return err
	}

	// Create file on local.
	f, err := os.Create(downloadPath)
	if err != nil {
		log.Errorf("Create file error, reasons: [%v]", err)
		return err
	}

	// Copy file from response body to local file.
	written, err := io.Copy(f, res.Body)
	if err != nil {
		log.Errorf("Copy file error, reasons: [%v]", err)
		return err
	}

	log.Infof("Download success, file size :[%f]M", float32(written)/(1024*1024))
	return nil
}
