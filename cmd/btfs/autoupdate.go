// This auto update project linux, mac OS or windows operating system.
package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	LatestVersionFile  = "version-%s-%s.txt"
	CurrentVersionFile = "version.txt"
	UpdateBinary       = "update-%s-%s"
	LatestBtfsBinary   = "btfs-%s-%s"
)

// You can add multiple addresses inside this array.
var url = [1]string{
	"http://13.59.36.232:8080/btns/QmaCa4qNJizZD5uwmprNoZ4MSHsGosK4oLDL6eqnVLQhTC/",
}

// Auto update function.
func update() {
	// Get download path.
	var defaultDownloadPath = os.TempDir()
	if defaultDownloadPath[len(defaultDownloadPath)-1:] == "/" {
		defaultDownloadPath = defaultDownloadPath[:len(defaultDownloadPath)-1]
	}

	// Get current program execution path.
	defaultBtfsPath, err := getCurrentPath()
	if err != nil {
		log.Errorf("Get current program execution path error, reasons: [%v]", err)
		return
	}

	var latestVersionFile string
	var updateBinary string
	var latestBtfsBinary string

	// Select binary files based on operating system.
	if (runtime.GOOS == "darwin" && runtime.GOARCH == "amd64") || (runtime.GOOS == "linux" && runtime.GOARCH == "amd64") {
		latestVersionFile = fmt.Sprintf(LatestVersionFile, runtime.GOOS, runtime.GOARCH)
		updateBinary = fmt.Sprintf(UpdateBinary, runtime.GOOS, runtime.GOARCH)
		latestBtfsBinary = fmt.Sprintf(LatestBtfsBinary, runtime.GOOS, runtime.GOARCH)
	} else if runtime.GOOS == "windows" && runtime.GOARCH == "386" {
		latestVersionFile = fmt.Sprintf(LatestVersionFile, runtime.GOOS, runtime.GOARCH)
		updateBinary = fmt.Sprint(fmt.Sprintf(UpdateBinary, runtime.GOOS, runtime.GOARCH), ".exe")
		latestBtfsBinary = fmt.Sprint(fmt.Sprintf(LatestBtfsBinary, runtime.GOOS, runtime.GOARCH), ".exe")
	} else {
		log.Errorf("Operating system [%s], arch [%s] does not support automatic updates", runtime.GOOS, runtime.GOARCH)
		return
	}

	latestVersionPath := fmt.Sprint(defaultDownloadPath, "/", latestVersionFile)
	currentVersionPath := fmt.Sprint(defaultBtfsPath, "/", CurrentVersionFile)
	latestBtfsBinaryPath := fmt.Sprint(defaultDownloadPath, "/", latestBtfsBinary)
	updateBinaryPath := fmt.Sprint(defaultDownloadPath, "/", updateBinary)

	for {
		log.Info("BTFS node AutoUpdater begin.")

		time.Sleep(time.Second * 20)

		// Chose random host from the list of btns.
		rand.Seed(time.Now().UnixNano())
		randNum := rand.Intn(len(url))

		if pathExists(latestVersionPath) {
			// Delete the latest btfs version file.
			err = os.Remove(latestVersionPath)
			if err != nil {
				log.Errorf("Remove latest btfs version file error, reasons: [%v]", err)
				continue
			}
		}

		// Get latest btfs version file.
		err := download(latestVersionPath, fmt.Sprint(url[randNum], latestVersionFile))
		if err != nil {
			log.Errorf("Download latest btfs version file error, reasons: [%v]", err)
			continue
		}

		// Get latest btfs version string.
		latestVersion, latestVersionMd5Hash, err := getFileMessage(latestVersionPath)
		if err != nil {
			log.Error("Open latest btfs version file error.")
			continue
		}

		var currentVersion string
		// Get current version string.
		currentVersion, _, err = getFileMessage(currentVersionPath)
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

		// Determine if the btfs latest binary file exists.
		if pathExists(latestBtfsBinaryPath) {
			// Delete the btfs latest binary file.
			err = os.Remove(latestBtfsBinaryPath)
			if err != nil {
				log.Errorf("Remove btfs latest binary file error, reasons: [%v]", err)
				continue
			}
		}

		// Get the btfs latest binary file from btns.
		err = download(latestBtfsBinaryPath, fmt.Sprint(url[randNum], latestBtfsBinary))
		if err != nil {
			log.Errorf("Download btfs latest binary file error, reasons: [%v]", err)
			continue
		}

		// Md5 encode file.
		latestMd5Hash, err := md5Encode(latestBtfsBinaryPath)
		if err != nil {
			log.Error("Md5 encode file failed.")
			continue
		}

		if latestMd5Hash != latestVersionMd5Hash {
			log.Error("Md5 verify failed.")
			continue
		}

		// Delete the update binary file if exists.
		if pathExists(updateBinaryPath) {
			err = os.Remove(updateBinaryPath)
			if err != nil {
				log.Errorf("Remove update binary file error, reasons: [%v]", err)
				continue
			}
		}

		// Get the update binary file from btns.
		err = download(updateBinaryPath, fmt.Sprint(url[randNum], updateBinary))
		if err != nil {
			log.Error("Download update binary file failed.")
			continue
		}

		// Add executable permissions to update binary file.
		err = os.Chmod(updateBinaryPath, 0775)
		if err != nil {
			log.Error("Chmod file 775 error, reasons: [%v]", err)
			continue
		}

		if runtime.GOOS == "windows" {
			// Start the btfs-updater binary process.
			cmd := exec.Command(updateBinaryPath, "-project", fmt.Sprint(defaultBtfsPath, "/"), "-download", fmt.Sprint(defaultDownloadPath, "/"))
			err = cmd.Start()
			if err != nil {
				log.Error(err)
				continue
			}
		} else {
			// Start the btfs-updater binary process.
			cmd := exec.Command("sudo", updateBinaryPath, "-project", fmt.Sprint(defaultBtfsPath, "/"), "-download", fmt.Sprint(defaultDownloadPath, "/"))
			err = cmd.Start()
			if err != nil {
				log.Error(err)
				continue
			}
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
func getFileMessage(file string) (string, string, error) {
	// Read file.
	versionFile, err := os.Open(file)
	if err != nil {
		log.Errorf("Open file failed, reasons: [%v]", err)
		return "", "", err
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
		return "", "", errors.New("version line is nil")
	}

	// Read line.
	md5Hash, _, c := buffer.ReadLine()
	if c == io.EOF {
		log.Error("Md5 line is nil")
		return "", "", errors.New("md5 line is nil")
	}

	return string(version), string(md5Hash), nil
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

// Md5 encode file by file path.
func md5Encode(name string) (string, error) {
	// Open file.
	file, err := os.Open(name)
	if err != nil {
		log.Errorf("Open file failed, reasons: [%v]", err)
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	// New reader of file.
	buffer := bufio.NewReader(file)

	// New md5.
	md5Hash := md5.New()

	// Copy file stream to md5 hash.
	_, err = io.Copy(md5Hash, buffer)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(md5Hash.Sum(nil)), nil
}
