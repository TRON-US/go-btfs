// This auto update project linux, mac OS or windows operating system.
package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	btfs_version "github.com/TRON-US/go-btfs"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

const (
	LatestConfigFile  = "config_%s_%s.yaml"
	CurrentConfigFile = "config.yaml"
	UpdateBinary      = "update-%s-%s%s"
	LatestBtfsBinary  = "btfs-%s-%s%s"
	url               = "localhost:5001"
)

type Config struct {
	Version          string `yaml:"version"`
	Md5Check         string `yaml:"md5"`
	AutoupdateFlg    bool   `yaml:"autoupdateFlg"`
	SleepTimeSeconds int    `yaml:"sleepTimeSeconds"`
	BeginNumber      int    `yaml:"beginNumber"`
	EndNumber        int    `yaml:"endNumber"`
}

var configRepoUrl = "https://raw.githubusercontent.com/TRON-US/btfs-binary-releases/master/"

// Auto update function.
func update() {
	fmt.Println("in cmd.btfs.autoupdate.update")
	// Get current program execution path.
	defaultBtfsPath, err := getCurrentPath()
	if err != nil {
		log.Errorf("Get current program execution path error, reasons: [%v]", err)
		return
	}

	var latestConfigFile string
	var updateBinary string
	var latestBtfsBinary string

	var latestConfigPath string
	var currentConfigPath string
	var latestBtfsBinaryPath string
	var updateBinaryPath string

	// Select binary files based on operating system.
	if (runtime.GOOS == "darwin" || runtime.GOOS == "linux" || runtime.GOOS == "windows") && (runtime.GOARCH == "amd64" || runtime.GOARCH == "386") {
		ext := ""
		sep := "/"
		if runtime.GOOS == "windows" {
			ext = ".exe"
			sep = "\\"
		}

		latestConfigFile = fmt.Sprintf(LatestConfigFile, runtime.GOOS, runtime.GOARCH)
		updateBinary = fmt.Sprintf(UpdateBinary, runtime.GOOS, runtime.GOARCH, ext)
		latestBtfsBinary = fmt.Sprintf(LatestBtfsBinary, runtime.GOOS, runtime.GOARCH, ext)

		latestConfigPath = fmt.Sprint(defaultBtfsPath, sep, latestConfigFile)
		currentConfigPath = fmt.Sprint(defaultBtfsPath, sep, CurrentConfigFile)
		latestBtfsBinaryPath = fmt.Sprint(defaultBtfsPath, sep, latestBtfsBinary)
		updateBinaryPath = fmt.Sprint(defaultBtfsPath, sep, updateBinary)
	} else {
		log.Errorf("Operating system [%s], arch [%s] does not support automatic updates", runtime.GOOS, runtime.GOARCH)
		return
	}

	sleepTimeSeconds := 20
	version := "0.0.0"
	autoupdateFlg := true
	routePath := fmt.Sprint(runtime.GOOS, "/", runtime.GOARCH, "/")

	for {
		time.Sleep(time.Second * time.Duration(sleepTimeSeconds))

		// Get current config file.
		currentConfig, err := getConfigure(currentConfigPath)
		if err == nil {
			autoupdateFlg = currentConfig.AutoupdateFlg
			version = currentConfig.Version
			sleepTimeSeconds = currentConfig.SleepTimeSeconds
		} else {
			version = btfs_version.CurrentVersionNumber
		}

		if !autoupdateFlg {
			continue
		}

		if pathExists(latestConfigPath) {
			// Delete the latest btfs config file.
			err = os.Remove(latestConfigPath)
			if err != nil {
				log.Errorf("Remove latest btfs config file error, reasons: [%v]", err)
				continue
			}
		}

		// Get latest btfs config file.
		err = download(latestConfigPath, fmt.Sprint(configRepoUrl, routePath, latestConfigFile))
		if err != nil {
			log.Errorf("Download latest btfs config file error, reasons: [%v]", err)
			continue
		}

		// Get latest config file.
		latestConfig, err := getConfigure(latestConfigPath)
		if err != nil {
			log.Errorf("Get latest config file error, reasons: [%v]", err)
			continue
		}
		time.Sleep(time.Second * 5)

		// Where your local node is running on localhost:5001
		sh := shell.NewShell(url)

		// Get btfs id.
		idOutput, err := sh.ID()
		if err != nil {
			log.Errorf("Get btfs id error, reasons: [%v]", err)
			return
		}

		// beginNumber   endNumber         range
		//      0            0       no nodes updated
		//      0            1       [0, 1) 1% updated
		//      0           100      [0, 100)100% updated
		//     100          100      no nodes updated
		if convertStringToInt(idOutput.ID)%100 < latestConfig.BeginNumber || convertStringToInt(idOutput.ID)%100 >= latestConfig.EndNumber {
			fmt.Println("This node is not in the scope of this automatic update.")
			continue
		}

		// Compare version.
		flg, err := versionCompare(latestConfig.Version, version)
		if err != nil {
			log.Errorf("Version compare error, reasons: [%v]", err)
			continue
		}

		if flg <= 0 {
			fmt.Println("BTFS is up-to-date.")
			sleepTimeSeconds = latestConfig.SleepTimeSeconds
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

		fmt.Println("BTFS auto update begin.")

		// Get the btfs latest binary file from btns.
		err = download(latestBtfsBinaryPath, fmt.Sprint(configRepoUrl, routePath, latestBtfsBinary))
		if err != nil {
			log.Errorf("Download btfs latest binary file error, reasons: [%v]", err)
			continue
		}

		fmt.Println("Download btfs binary file success!")

		// Md5 encode file.
		latestMd5Hash, err := md5Encode(latestBtfsBinaryPath)
		if err != nil {
			log.Error("Md5 encode file failed.")
			continue
		}

		if latestMd5Hash != latestConfig.Md5Check {
			log.Error("Md5 verify failed.")
			continue
		}

		fmt.Println("Md5 check btfs binary file success!")

		// Delete the update binary file if exists.
		if pathExists(updateBinaryPath) {
			err = os.Remove(updateBinaryPath)
			if err != nil {
				log.Errorf("Remove update binary file error, reasons: [%v]", err)
				continue
			}
		}

		// Get the update binary file from btns.
		err = download(updateBinaryPath, fmt.Sprint(configRepoUrl, routePath, updateBinary))
		if err != nil {
			log.Error("Download update binary file failed.")
			continue
		}

		fmt.Println("Download update binary file success!")

		// Add executable permissions to update binary file.
		err = os.Chmod(updateBinaryPath, 0775)
		if err != nil {
			log.Error("Chmod file 775 error, reasons: [%v]", err)
			continue
		}

		if runtime.GOOS == "windows" {
			// Start the btfs-updater binary process.
			cmd := exec.Command(updateBinaryPath, "-project", fmt.Sprint(defaultBtfsPath, "\\"), "-download", fmt.Sprint(defaultBtfsPath, "\\"))
			err = cmd.Start()
			if err != nil {
				log.Error(err)
				continue
			}
		} else {
			// Start the btfs-updater binary process.
			cmd := exec.Command(updateBinaryPath, "-project", fmt.Sprint(defaultBtfsPath, "/"), "-download", fmt.Sprint(defaultBtfsPath, "/"))
			err = cmd.Start()
			if err != nil {
				log.Error(err)
				continue
			}
		}
		fmt.Println("Process will exit now and restart after the update completes.")
		os.Exit(0)
	}
}

// Determine if the path file exists.
func pathExists(path string) bool {
	fmt.Println("in cmd.btfs.autoupdate.pathExists")
	fmt.Println("path is: [%v]", path)
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

// Get config struct from yaml file.
func getConfigure(fileName string) (*Config, error) {
	fmt.Println("in cmd.btfs.autoupdate.getConfigure")
	fmt.Println("fileName is: [%v]", fileName)
	yamlFile, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	conf := new(Config)
	err = yaml.Unmarshal(yamlFile, conf)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// Compare version.
func versionCompare(version1, version2 string) (int, error) {
	fmt.Println("in cmd.btfs.autoupdate.versionCompare")
	fmt.Println("version1 is: [%v]", version1)
	fmt.Println("version2 is: [%v]", version2)

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
	fmt.Println("in cmd.btfs.autoupdate.getCurrentPath")
	ex, err := os.Executable()
	if err != nil {
		return "", err
	}
	exPath := filepath.Dir(ex)
	return exPath, nil
}

// http get download function.
func download(downloadPath, url string) error {
	fmt.Println("in cmd.btfs.autoupdate.download")
	// http get
	fmt.Println("downloadPath is: [%v]", downloadPath)
	fmt.Println("url is: [%v]", url)
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

	defer func() {
		_ = f.Close()
	}()

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
	fmt.Println("in cmd.btfs.autoupdate.md5Encode")
	fmt.Println("name is: [%v]", name)
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

// Convert string to int.
func convertStringToInt(s string) int {
	fmt.Println("in cmd.btfs.autoupdate.convertStringToInt")
	fmt.Println("s is: [%v]", s)

	sum := 0
	for _, v := range []byte(s) {
		sum += int(v)
	}
	return sum
}
