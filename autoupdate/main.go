package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	shell "github.com/ipfs/go-ipfs-api"

	"github.com/natefinch/lumberjack"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const url = "localhost:5001"
const testfile = "QmZHeNJTU4jFzgBAouHSqbT2tyYJxgk6i15e7x5pudBune"
const testfilecontent = "Hello BTFS!"

// Log print initialization, get *zap.Logger Info.
func initLogger(logPath string) *zap.Logger {
	fmt.Println("in autoupdate.main.initLogger")
	fmt.Println("logPath is: [%v]", logPath)
	hook := lumberjack.Logger{
		Filename:   logPath, // log file path
		MaxSize:    128,     // megabytes
		MaxBackups: 30,      // max backup
		MaxAge:     7,       // days
		Compress:   true,    // is Compress, disabled by default
	}

	w := zapcore.AddSync(&hook)

	encoderConfig := zap.NewProductionEncoderConfig()
	// time format
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout), w), // this line enables log outputs to multiple destinations: log file/stdout
		zap.InfoLevel,
	)

	logger := zap.New(core, zap.AddStacktrace(zap.ErrorLevel))
	return logger
}

// Rollback function of auto update.

func rollback(log *zap.Logger, wg *sync.WaitGroup, defaultProjectPath, defaultDownloadPath string) {
	fmt.Println("in autoupdate.main.rollback")
	fmt.Println("defaultProjectPath is: [%v]", defaultProjectPath)
	fmt.Println("defaultDownloadPath is: [%v]", defaultDownloadPath)
	defer func() {
		wg.Done()
	}()

	// Check if the BTFS daemon server is up every 5 seconds, checked a total of five times.
	for i := 0; i < 5; i++ {
		time.Sleep(time.Second * 5)
		// Where your local node is running on localhost:5001
		sh := shell.NewShell(url)
		if sh.IsUp() {
			log.Info("BTFS node started successfully!")
			return
		}
	}

	log.Info("BTFS node failed to start, rollback begin!")

	// Select binary files and configure file path based on operating system.
	currentConfigPath, backupConfigPath, _, btfsBinaryPath, btfsBackupPath, _, err := getProjectPath(defaultProjectPath, defaultDownloadPath)
	if err != nil {
		log.Error(fmt.Sprintf("Operating system [%s], arch [%s] does not support rollback\n", runtime.GOOS, runtime.GOARCH))
		return
	}

	// Check if the backup binary file exists.
	if !pathExists(btfsBackupPath) {
		log.Error(fmt.Sprintf("BTFS backup binary is not exists."))
		return
	}

	// Check if the current configure file exists.
	if pathExists(currentConfigPath) {
		// Delete current configure file.
		err = os.Remove(currentConfigPath)
		if err != nil {
			log.Error(fmt.Sprintf("Delete backup configure file error, reasons: [%v]\n", err))
			return
		}
	}

	// Check if the backup configure file exists.
	if pathExists(backupConfigPath) {
		// Move backup configure file to current configure file.
		err = os.Rename(backupConfigPath, currentConfigPath)
		if err != nil {
			log.Error(fmt.Sprintf("Move backup configure file error, reasons: [%v]\n", err))
			return
		}
	}

	// Check if the btfs binary file exists.
	if pathExists(btfsBinaryPath) {
		// Delete the btfs binary file.
		err = os.Remove(btfsBinaryPath)
		if err != nil {
			log.Error(fmt.Sprintf("Delete btfs binary file error, reasons: [%v]\n", err))
			return
		}
	}

	// Move backup btfs binary file to current btfs binary file.
	err = os.Rename(btfsBackupPath, btfsBinaryPath)
	if err != nil {
		log.Error(fmt.Sprintf("Move backup btfs binary file error, reasons: [%v]\n", err))
		return
	}

	// Add executable permissions to btfs binary.
	err = os.Chmod(btfsBinaryPath, 0775)
	if err != nil {
		log.Error(fmt.Sprintf("Chmod file error, reasons: [%v]\n", err))
		return
	}

	// Start the btfs daemon according to different operating systems.
	if runtime.GOOS == "windows" {
		cmd := exec.Command(btfsBinaryPath, "daemon")
		err = cmd.Start()
	} else {
		cmd := exec.Command(btfsBinaryPath, "daemon")
		err = cmd.Start()
	}

	// Check if the btfs daemon start success.
	if err != nil {
		log.Error(fmt.Sprintf("BTFS rollback failed, reasons: [%v]", err))
		return
	}

	log.Info("BTFS rollback SUCCESS!")
}

func update(log *zap.Logger) int {
	fmt.Println("in autoupdate.main.update")
	time.Sleep(time.Second * 5)
	defaultProjectPath := flag.String("project", "", "default project path")
	defaultDownloadPath := flag.String("download", "", "default download path")

	flag.Parse()

	log.Info("BTFS auto update begin.")

	if *defaultProjectPath == "" || *defaultDownloadPath == "" {
		log.Error("Request param is nil.")
		return 1
	}

	// Select binary files and configure file path based on operating system.
	currentConfigPath, backupConfigPath, latestConfigPath, btfsBinaryPath, btfsBackupPath, latestBtfsBinaryPath, err := getProjectPath(*defaultProjectPath, *defaultDownloadPath)
	if err != nil {
		log.Error(fmt.Sprintf("Operating system [%s], arch [%s] does not support rollback\n", runtime.GOOS, runtime.GOARCH))
		return 1
	}

	// Delete backup configure file.
	if pathExists(backupConfigPath) {
		err = os.Remove(backupConfigPath)
		if err != nil {
			log.Error(fmt.Sprintf("Delete backup config file error, reasons: [%v]\n", err))
			return 1
		}
	}

	// Move current config file if existed.
	if pathExists(currentConfigPath) {
		err = os.Rename(currentConfigPath, backupConfigPath)
		if err != nil {
			log.Error(fmt.Sprintf("Move current config file error, reasons: [%v]\n", err))
			return 1
		}
	}

	// Move latest configure file to current configure file.
	err = os.Rename(latestConfigPath, currentConfigPath)
	if err != nil {
		log.Error(fmt.Sprintf("Move file error, reasons: [%v]\n", err))
		return 1
	}

	// Delete btfs backup file.
	if pathExists(btfsBackupPath) {
		err = os.Remove(btfsBackupPath)
		if err != nil {
			log.Error(fmt.Sprintf("Move file error, reasons: [%v]\n", err))
			return 1
		}
	}

	// Backup btfs binary file.
	err = os.Rename(btfsBinaryPath, btfsBackupPath)
	if err != nil {
		log.Error(fmt.Sprintf("Move file error, reasons: [%v]\n", err))
		return 1
	}

	// Move latest btfs binary file to current btfs binary file.
	err = os.Rename(latestBtfsBinaryPath, btfsBinaryPath)
	if err != nil {
		log.Error(fmt.Sprintf("Move file error, reasons: [%v]\n", err))
		return 1
	}

	// Add executable permissions to btfs binary.
	err = os.Chmod(btfsBinaryPath, 0775)
	if err != nil {
		log.Error(fmt.Sprintf("Chmod file error, reasons: [%v]\n", err))
		return 1
	}

	wg := &sync.WaitGroup{}

	wg.Add(1)

	go rollback(log, wg, *defaultProjectPath, *defaultDownloadPath)

	// prepare functional test before start btfs daemon
	ready_to_test := prepare_test(log, btfsBinaryPath)

	if runtime.GOOS == "windows" {
		cmd := exec.Command(btfsBinaryPath, "daemon")
		err = cmd.Start()
	} else {
		cmd := exec.Command(btfsBinaryPath, "daemon")
		err = cmd.Start()
	}

	// Wait for the rollback program to complete.
	wg.Wait()

	// start btfs function test
	if ready_to_test {
		if get_functest(log, btfsBinaryPath) {
			log.Info("BTFS daemon get file test succeeded!")
		} else {
			log.Error("BTFS daemon get file test failed!")
		}
		if add_functest(log, btfsBinaryPath) {
			log.Info("BTFS daemon add file test succeeded!")
		} else {
			log.Error("BTFS daemon add file test failed!")
		}
	} else {
		log.Info("BTFS daemon test skipped")
	}

	log.Info("BTFS auto update SUCCESS!")

	return 0
}

func main() {
	fmt.Println("in autoupdate.main.main")
	log := initLogger("update.log")
	os.Exit(update(log))
}

// Determine if the path file exists.
func pathExists(path string) bool {
	fmt.Println("in autoupdate.main.pathExists")
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

// Select binary files and configure file path based on operating system.
func getProjectPath(defaultProjectPath, defaultDownloadPath string) (currentConfigPath string, backupConfigPath string,
	latestConfigPath string, btfsBinaryPath string, btfsBackupPath string, latestBtfsBinaryPath string, err error) {
	fmt.Println("in autoupdate.main.getProjectPath")
	fmt.Println("defaultProjectPath is: [%v]", defaultProjectPath)
	fmt.Println("defaultDownloadPath is: [%v]", defaultDownloadPath)
	fmt.Println("currentConfigPath is: [%v]", currentConfigPath)
	fmt.Println("backupConfigPath is: [%v]", backupConfigPath)
	fmt.Println("latestConfigPath is: [%v]", latestConfigPath)
	fmt.Println("btfsBinaryPath is: [%v]", btfsBinaryPath)
	fmt.Println("btfsBackupPath is: [%v]", btfsBackupPath)
	fmt.Println("latestBtfsBinaryPath is: [%v]", latestBtfsBinaryPath)
	if (runtime.GOOS == "darwin" || runtime.GOOS == "linux" || runtime.GOOS == "windows") && (runtime.GOARCH == "amd64" || runtime.GOARCH == "386") {
		ext := ""
		if runtime.GOOS == "windows" {
			ext = ".exe"
		}

		currentConfigPath = fmt.Sprint(defaultProjectPath, "config.yaml")
		backupConfigPath = fmt.Sprint(defaultDownloadPath, "config.yaml.bk")
		latestConfigPath = fmt.Sprint(defaultDownloadPath, fmt.Sprintf("config_%s_%s.yaml", runtime.GOOS, runtime.GOARCH))
		btfsBinaryPath = fmt.Sprint(defaultProjectPath, fmt.Sprintf("btfs%s", ext))
		btfsBackupPath = fmt.Sprint(defaultDownloadPath, fmt.Sprintf("btfs%s.bk", ext))
		latestBtfsBinaryPath = fmt.Sprint(defaultDownloadPath, fmt.Sprintf("btfs-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext))
	} else {
		fmt.Printf("Operating system [%s], arch [%s] does not support automatic updates\n", runtime.GOOS, runtime.GOARCH)
		return currentConfigPath, backupConfigPath, latestConfigPath, btfsBinaryPath, btfsBackupPath, latestBtfsBinaryPath, errors.New("os does not support automatic updates")
	}
	return
}

// we need to delete the file for get test from last run
func prepare_test(log *zap.Logger, btfsBinaryPath string) bool {
	fmt.Println("in autoupdate.main.prepare_test")
	fmt.Println("btfsBinaryPath is: [%v]", btfsBinaryPath)

	cmd := exec.Command(btfsBinaryPath, "rm", testfile)
	err := cmd.Start()

	if err != nil {
		log.Info(fmt.Sprintf("btfs rm failed with message: [%v]", err))
		return false
	} else {
		log.Info("btfs test preparation succeed")
	}
	return true
}

func get_functest(log *zap.Logger, btfsBinaryPath string) bool {
	fmt.Println("in autoupdate.main.get_functest")
	fmt.Println("btfsBinaryPath is: [%v]", btfsBinaryPath)

	// btfs get file saved to current working directory
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(fmt.Sprintf("get working directory failed: [%v]", err))
		return false
	}

	cmd := exec.Command(btfsBinaryPath, "get", "-o", dir, testfile)
	out, err := cmd.Output()
	if err != nil {
		log.Info(fmt.Sprintf("btfs get test failed: [%v]", err))
		log.Info(string(out))
		return false
	}

	data, err := ioutil.ReadFile(dir + "/" + testfile)
	if err != nil {
		log.Info(fmt.Sprintf("btfs get test: read file failed: [%v]", err))
		return false
	}
	// remote last "\n" before compare
	if string(data[:len(data)-1]) != testfilecontent {
		log.Info("btfs get test: get different content")
		log.Info(string(data))
		return false
	}

	log.Info("btfs get test succeeded")
	return true
}

func add_functest(log *zap.Logger, btfsBinaryPath string) bool {
	fmt.Println("in autoupdate.main.functest")
	fmt.Println("btfsBinaryPath is: [%v]", btfsBinaryPath)

	// write btfs id command output to a file in current working directory
	// then btfs add that file for test
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(fmt.Sprintf("get working directory failed: [%v]", err))
		return false
	}

	cmd := exec.Command(btfsBinaryPath, "id")
	out, err := cmd.Output()
	if err != nil {
		log.Info(fmt.Sprintf("btfs add test: btfs id failed: [%v]", err))
		return false
	}

	// add current time stamp to file content so every time adding-file hash is different
	currentTime := time.Now().String()
	out = append(out, currentTime...)

	origin := out
	filename := dir + "/btfstest.txt"
	err = ioutil.WriteFile(filename, out, 0644)
	if err != nil {
		log.Info(fmt.Sprintf("btfs add test: write file failed: [%v]", err))
		return false
	}

	cmd = exec.Command(btfsBinaryPath, "add", filename)
	out, err = cmd.Output()
	if err != nil {
		log.Info(fmt.Sprintf("btfs add test failed: [%v]", err))
		return false
	}

	s := strings.Split(string(out), " ")
	if len(s) < 2 {
		log.Info("btfs add test failed: invalid add result")
		log.Info(string(out))
		return false
	}

	addfilehash := s[1]
	cmd = exec.Command(btfsBinaryPath, "cat", addfilehash)
	out, err = cmd.Output()

	if string(out) != string(origin) {
		log.Info("btfs add test failed: cat different content")
		log.Info("btfs add file:")
		log.Info(string(origin))
		log.Info("btfs cat file:")
		log.Info(string(out))
		return false
	}

	log.Info("btfs add test succeeded")
	return true
}
