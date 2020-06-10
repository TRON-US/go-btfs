package storage

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/TRON-US/go-btfs-cmds"

	logging "github.com/ipfs/go-log"
	"github.com/mitchellh/go-homedir"
	"github.com/shirou/gopsutil/disk"
)

const (
	storeDir    = ".btfs"
	defaultPath = "~/.btfs"
	fileName    = "~/.btfs.properties"
)

var log = logging.Logger("core/commands/path")

var (
	btfsPath   string
	filePath   string
	StorePath  string
	OriginPath string
	lock       Mutex
)

const mutexLocked = 1 << iota

type Mutex struct {
	sync.Mutex
}

func (m *Mutex) TryLock() bool {
	return atomic.CompareAndSwapInt32((*int32)(unsafe.Pointer(&m.Mutex)), 0, mutexLocked)
}

var PathCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Modify the Host storage folder path for BTFS client.",
		ShortDescription: `
The default local repository path is located at ~/.btfs folder, in order to
improve the hard disk space usage, provide the function to change the original 
storage location, a specified path as a parameter need to be passed.
`,
	},
	Subcommands: map[string]*cmds.Command{
		"status": PathStatusCmd,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("path-name", true, false,
			"New BTFS Path. Should be absolute path."),
		cmds.StringArg("storage-size", true, false, "Storage Commitment Size"),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		locked := lock.TryLock()
		if locked {
			defer lock.Unlock()
		} else {
			return errors.New("Cannot set path concurrently.")
		}
		StorePath = strings.Trim(req.Arguments[0], " ")

		if StorePath == "" {
			return fmt.Errorf("path is not defined")
		}
		var err error
		if !filepath.IsAbs(StorePath) {
			StorePath, err = filepath.Abs(StorePath)
			if err != nil {
				return err
			}
		}
		if btfsPath != "" {
			if btfsPath != StorePath {
				OriginPath = filepath.Join(btfsPath, storeDir)
			} else {
				return fmt.Errorf("specifed path is same with current path")
			}
		} else if home, err := homedir.Expand(defaultPath); err == nil && home != "" {
			OriginPath = home
		} else {
			return fmt.Errorf("can not find the original stored path")
		}

		if !CheckExist(StorePath) {
			err := os.MkdirAll(StorePath, os.ModePerm)
			if err != nil {
				return fmt.Errorf("mkdir: %s", err)
			}
		} else if !CheckDirEmpty(filepath.Join(StorePath, storeDir)) {
			return fmt.Errorf("path is invalid")
		}
		usage, err := disk.Usage(StorePath)
		if err != nil {
			return err
		}
		promisedStorageSize, err := strconv.ParseUint(req.Arguments[1], 10, 64)
		if err != nil {
			return err
		}
		if usage.Free < promisedStorageSize {
			return fmt.Errorf("Not enough disk space, expect: ge %v bytes, actual: %v bytes",
				promisedStorageSize, usage.Free)
		}
		restartCmd := exec.Command("btfs", "restart")
		if err := restartCmd.Run(); err != nil {
			return fmt.Errorf("restart command: %s", err)
		}
		return nil
	},
}

var PathStatusCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "Get status of resetting path.",
		ShortDescription: "Get status of resetting path.",
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		tryLock := lock.TryLock()
		if tryLock {
			lock.Unlock()
		}
		if !tryLock {
			return cmds.EmitOnce(res, PathStatus{
				Resetting: true,
				Path:      StorePath,
			})
		}
		return cmds.EmitOnce(res, PathStatus{
			Resetting: false,
			Path:      StorePath,
		})
	},
	Type: PathStatus{},
}

type PathStatus struct {
	Resetting bool
	Path      string
}

func init() {
	SetEnvVariables()
}

func WriteProperties() error {
	if CheckExist(filePath) == false {
		newFile, err := os.Create(filePath)
		defer newFile.Close()
		if err != nil {
			return err
		}
	}
	data := []byte(StorePath)
	err := ioutil.WriteFile(filePath, data, 0666)
	if err == nil {
		fmt.Printf("Storage location was reset in %v\n", StorePath)
	}
	return err
}

func MoveFolder() error {
	// make dir does not contain .btfs, but move need to specify .btfs
	err := os.Rename(OriginPath, filepath.Join(StorePath, storeDir))
	if err != nil {
		return err
	}
	return nil
}

func ReadProperties(filePath string) string {
	f, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Errorf("Read properties fail: [%v]\n", err)
	}
	return string(f)
}

func CheckDirEmpty(dirname string) bool {
	dir, err := ioutil.ReadDir(dirname)
	if err != nil {
		log.Debug("Read directory fail: [%v]\n", err)
	}
	return len(dir) == 0
}

func SetEnvVariables() {
	if propertiesHome, err := homedir.Expand(fileName); err == nil {
		filePath = propertiesHome
		if CheckExist(filePath) {
			btfsPath = ReadProperties(filePath)
			if btfsPath != "" {
				newPath := filepath.Join(btfsPath, storeDir)
				err := os.Setenv("BTFS_PATH", newPath)
				if err != nil {
					log.Errorf("cannot set env variable of BTFS_PATH: [%v] \n", err)
				}
			}
		}
	}
}

func CheckExist(pathName string) bool {
	_, err := os.Stat(pathName)
	if os.IsNotExist(err) {
		return false
	}
	return true
}
