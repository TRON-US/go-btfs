package path

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/TRON-US/go-btfs-cmds"

	logging "github.com/ipfs/go-log"
)

const (
	storeDir = ".btfs"
	fileName = "path.properties"
)

var btfsPath string
var filePath string
var storePath string
var originPath string
var log = logging.Logger("core/commands/store/path")

var PathCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Modify the Host storage folder path for BTFS client.",
		ShortDescription: `
The default local repository path is located at ~/.btfs folder, in order to
improve the hard disk space usage, provide the function to change the original 
storage location, a specified path as a parameter need to be passed.
`,
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("path_name", true, true, "New BTFS Path.").EnableStdin(),
	},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		storePath = strings.Trim(req.Arguments[0], " ")

		if storePath == "" {
			return fmt.Errorf("path is not defined")
		}

		if btfsPath != "" {
			if btfsPath != storePath {
				originPath = btfsPath
			} else {
				return fmt.Errorf("specifed path is same with current path")
			}
		} else if home, err := Home(); err == nil && home != "" {
			originPath = home
		} else {
			return fmt.Errorf("can not find the original stored path")
		}

		if CheckExist(storePath) == false {
			err := os.MkdirAll(storePath, os.ModePerm)
			if err != nil {
				return fmt.Errorf("mkdir: %s", err)
			}
		} else if CheckDirEmpty(filepath.Join(storePath, storeDir)) == false {
			return fmt.Errorf("path is occupied")
		}

		restartCmd := exec.Command("btfs", "restart")
		if err := restartCmd.Run(); err != nil {
			return fmt.Errorf("restart command: %s", err)
		}
		return nil
	},
}

func init() {
	GetPropertiesPath()
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
	data := []byte(storePath)
	err := ioutil.WriteFile(filePath, data, 0666)
	if err == nil {
		fmt.Printf("Storage location was reset in %v\n", storePath)
	}
	return err
}

func MoveFolder() error {
	// make dir does not contain .btfs, but move need to specify .btfs
	err := os.Rename(filepath.Join(originPath, storeDir), filepath.Join(storePath, storeDir))
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
	dir, _ := ioutil.ReadDir(dirname)
	if len(dir) == 0 {
		return true
	} else {
		return false
	}
}

func Home() (string, error) {
	user, err := user.Current()
	if nil == err {
		return user.HomeDir, nil
	}
	if "windows" == runtime.GOOS {
		return homeWindows()
	}
	return homeUnix()
}

func homeUnix() (string, error) {
	// First prefer the HOME environmental variable
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	// If that fails, try the shell
	var stdout bytes.Buffer
	cmd := exec.Command("sh", "-c", "eval echo ~$USER")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		return "", errors.New("blank output when reading home directory")
	}
	return result, nil
}

func homeWindows() (string, error) {
	drive := os.Getenv("HOMEDRIVE")
	path := os.Getenv("HOMEPATH")
	home := drive + path
	if drive == "" || path == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		return "", errors.New("HOMEDRIVE, HOMEPATH, and USERPROFILE are blank")
	}
	return home, nil
}

func GetPropertiesPath() {
	var propertiesPath string
	if home, err := Home(); err == nil {
		if "windows" == runtime.GOOS {
			propertiesPath = filepath.Join(home, "btfs")
		} else {
			propertiesPath = filepath.Join(home, "btfs/bin")
		}
	}

	if CheckExist(propertiesPath) == false {
		err := os.MkdirAll(propertiesPath, os.ModePerm)
		if err != nil {
			log.Fatal(err)
			return
		}
	}
	filePath = filepath.Join(propertiesPath, fileName)
}

func SetEnvVariables() {
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

func CheckExist(pathName string) bool {
	_, err := os.Stat(pathName)
	if os.IsNotExist(err) {
		return false
	}
	return true
}
