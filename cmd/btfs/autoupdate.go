package main

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	DefaultDownloadPath = "/tmp"

	NewVersionFile   = "new_version.txt"
	NowVersionFile   = "version.txt"
	UpdateShell      = "update.sh"
	LatestBtfsBinary = "btfs-latest"
	NowBtfsBinary    = "btfs"

	rm   = "rm"
	wget = "wget"
	cmp  = "cmp"
	bash = "bash"
)

var url = [1]string{
	"http://13.59.36.232:8080/btns/QmaCa4qNJizZD5uwmprNoZ4MSHsGosK4oLDL6eqnVLQhTC/",
}

// Auto update function.
func update() {
	for {
		log.Info("BTFS node AutoUpdater begin.")

		time.Sleep(time.Second * 20)

		rand.Seed(time.Now().UnixNano())
		randNum := rand.Intn(len(url))

		// Get current program execution path.
		defaultBtfsPath, err := getCurrentPath()
		if err != nil {
			log.Errorf("Get current program execution path error, reasons: [%v]", err)
			continue
		}

		newVersionPath := fmt.Sprint(DefaultDownloadPath, "/", NewVersionFile)
		nowVersionPath := fmt.Sprint(defaultBtfsPath, NowVersionFile)
		nowBtfsBinaryPath := fmt.Sprint(defaultBtfsPath, NowBtfsBinary)

		if pathExists(newVersionPath) {
			// Delete the btfs-latest file.
			execCommand(rm, newVersionPath)
		}

		// Get binary version.
		if !execCommand(wget, "-P", DefaultDownloadPath, fmt.Sprint(url[randNum], NewVersionFile)) {
			log.Error("Download version.txt file failed.")
			continue
		}

		// Get new version string.
		newVersion, err := getVersion(newVersionPath)
		if err != nil {
			log.Error("Open new version file error.")
			continue
		}

		var nowVersion string
		// Get now version string.
		nowVersion, err = getVersion(nowVersionPath)
		if err != nil {
			nowVersion = "0.0.0"
		}

		// Compare version.
		flg, err := versionCompare(newVersion, nowVersion)
		if err != nil {
			log.Errorf("Version compare error, reasons: [%v]", err)
			continue
		}

		if flg <= 0 {
			log.Info("Btfs binary from btns version level is small than now version.")
			continue
		}

		latestBtfsBinaryPath := fmt.Sprint(DefaultDownloadPath, "/", LatestBtfsBinary)
		updateShellPath := fmt.Sprint(DefaultDownloadPath, "/", UpdateShell)

		// Determine if the btfs-latest file exists.
		if pathExists(latestBtfsBinaryPath) {
			// Delete the btfs-latest file.
			execCommand(rm, latestBtfsBinaryPath)
		}

		// Get the btfs-latest file from btns.
		if execCommand(wget, "-P", DefaultDownloadPath, fmt.Sprint(url[randNum], LatestBtfsBinary)) {
			// Determine if it's a new version.
			if execCommand(cmp, latestBtfsBinaryPath, nowBtfsBinaryPath) {
				log.Info("same")
			} else {
				log.Info("different test")
				if pathExists(updateShellPath) {
					// Delete the btfs-latest file.
					execCommand(rm, updateShellPath)
				}

				// Get the update.sh file from btns.
				if !execCommand(wget, "-P", DefaultDownloadPath, fmt.Sprint(url[randNum], UpdateShell)) {
					log.Error("Download update.sh file failed.")
					continue
				}

				// Start the btfs-updater binary process.
				cmd := exec.Command(bash, updateShellPath, "-p", defaultBtfsPath, "-d", fmt.Sprint(DefaultDownloadPath, "/"))
				err := cmd.Start()
				if err != nil {
					log.Error(err)
					continue
				}
				os.Exit(0)
			}
		}
		log.Info("BTFS node AutoUpdater end.")
	}
}

// Execute external methods.
func execCommand(name string, arg ...string) bool {
	cmd := exec.Command(name, arg...)
	_, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Function %s exec err, reasons: [%v]", name, err)
		return false
	}

	return true
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

// Get version from file.
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
	file, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	i := strings.LastIndex(path, "/")
	if i < 0 {
		i = strings.LastIndex(path, "\\")
	}
	if i < 0 {
		return "", errors.New(`error: Can't find "/" or "\".`)
	}
	return string(path[0 : i+1]), nil
}
