package storage

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/TRON-US/go-btfs-cmds"
	"github.com/dustin/go-humanize"
	logging "github.com/ipfs/go-log"
	"github.com/mitchellh/go-homedir"
	"github.com/shirou/gopsutil/disk"
)

const (
	defaultPath = "~/.btfs"
	fileName    = "~/.btfs.properties"
	key         = "BTFS_PATH"
)

var Excutable = func() string {
	if ex, err := os.Executable(); err == nil {
		return ex
	}
	return "btfs"
}()

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
		"status":   PathStatusCmd,
		"capacity": PathCapacityCmd,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("path-name", true, false,
			"New BTFS Path.Should be absolute path."),
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
		if StorePath, err = homedir.Expand(StorePath); err != nil {
			return err
		}
		if !filepath.IsAbs(StorePath) {
			StorePath, err = filepath.Abs(StorePath)
			if err != nil {
				return err
			}
		}
		if btfsPath != "" {
			if btfsPath != StorePath {
				OriginPath = btfsPath
			} else {
				return fmt.Errorf("specifed path is same with current path")
			}
		} else if envBtfsPath := os.Getenv(key); envBtfsPath != "" {
			OriginPath = envBtfsPath
		} else if home, err := homedir.Expand(defaultPath); err == nil && home != "" {
			OriginPath = home
		} else {
			return fmt.Errorf("can not find the original stored path")
		}

		if err := validatePath(OriginPath, StorePath); err != nil {
			return err
		}

		usage, err := disk.Usage(filepath.Dir(StorePath))
		if err != nil {
			return err
		}
		promisedStorageSize, err := humanize.ParseBytes(req.Arguments[1])
		if err != nil {
			return err
		}
		if usage.Free < promisedStorageSize {
			return fmt.Errorf("Not enough disk space, expect: ge %v bytes, actual: %v bytes",
				promisedStorageSize, usage.Free)
		}

		restartCmd := exec.Command(Excutable, "restart", "-p")
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
		return cmds.EmitOnce(res, PathStatus{
			Resetting: !tryLock,
			Path:      StorePath,
		})
	},
	Type: PathStatus{},
}

type PathStatus struct {
	Resetting bool
	Path      string
}

var PathCapacityCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "Get free space of passed path.",
		ShortDescription: "Get free space of passed path.",
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("path-name", true, true,
			"New BTFS Path. Should be absolute path."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		path := strings.Trim(req.Arguments[0], " ")
		if path == "" {
			return fmt.Errorf("path is not defined")
		}
		var err error
		if !filepath.IsAbs(path) {
			path, err = filepath.Abs(path)
			if err != nil {
				return err
			}
		}
		if btfsPath != "" {
			if btfsPath != StorePath {
				OriginPath = btfsPath
			} else {
				return fmt.Errorf("specifed path is same with current path")
			}
		} else if envBtfsPath := os.Getenv(key); envBtfsPath != "" {
			OriginPath = envBtfsPath
		} else if home, err := homedir.Expand(defaultPath); err == nil && home != "" {
			OriginPath = home
		} else {
			return fmt.Errorf("can not find the original stored path")
		}
		if err := validatePath(OriginPath, path); err != nil {
			return err
		}
		if !CheckDirEmpty(path) {
			return fmt.Errorf("path %s is not empty", path)
		}
		valid := true
		usage, err := disk.Usage(filepath.Dir(path))
		if err != nil {
			return err
		}
		humanizedFreeSpace := humanize.Bytes(usage.Free)
		return cmds.EmitOnce(res, &PathCapacity{
			FreeSpace:          usage.Free,
			Valid:              valid,
			HumanizedFreeSpace: humanizedFreeSpace,
		})
	},
	Type: &PathCapacity{},
}

func validatePath(src string, dest string) error {
	log.Debug("src", src, "dest", dest)
	// clean: /abc/ => /abc
	src = filepath.Clean(src)
	dest = filepath.Clean(dest)
	if src == dest || strings.HasPrefix(dest, src+string(filepath.Separator)) {
		return errors.New("invalid path")
	}
	dir := filepath.Dir(src)
	if !CheckExist(src) {
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("mkdir: %s", err)
		}
	}
	return nil
}

type PathCapacity struct {
	FreeSpace          uint64
	Valid              bool
	HumanizedFreeSpace string
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
	err := os.Rename(OriginPath, StorePath)
	// src and dest dir are not in the same partition
	if err != nil {
		err := move(OriginPath, StorePath)
		if err != nil {
			return err
		}
	}
	return nil
}

func move(src string, dst string) error {
	if err := copyDir(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func copyDir(src string, dst string) error {
	var err error
	var fds []os.FileInfo
	var srcinfo os.FileInfo

	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}

	if fds, err = ioutil.ReadDir(src); err != nil {
		return err
	}
	for _, fd := range fds {
		srcfp := filepath.Join(src, fd.Name())
		dstfp := filepath.Join(dst, fd.Name())

		if fd.IsDir() {
			if err = copyDir(srcfp, dstfp); err != nil {
				fmt.Println(err)
			}
		} else {
			if err = copyFile(srcfp, dstfp); err != nil {
				fmt.Println(err)
			}
		}
	}
	return nil
}

// File copies a single file from src to dst
func copyFile(src, dst string) error {
	var err error
	var srcfd *os.File
	var dstfd *os.File
	var srcinfo os.FileInfo

	if srcfd, err = os.Open(src); err != nil {
		return err
	}
	defer srcfd.Close()

	if dstfd, err = os.Create(dst); err != nil {
		return err
	}
	defer dstfd.Close()

	if _, err = io.Copy(dstfd, srcfd); err != nil {
		return err
	}
	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}
	return os.Chmod(dst, srcinfo.Mode())
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
				newPath := btfsPath
				_, b := os.LookupEnv(key)
				if !b {
					err := os.Setenv(key, newPath)
					if err != nil {
						log.Errorf("cannot set env variable of BTFS_PATH: [%v] \n", err)
					}
				}
			}
		}
	}
}

func CheckExist(pathName string) bool {
	_, err := os.Stat(pathName)
	return !os.IsNotExist(err)
}
