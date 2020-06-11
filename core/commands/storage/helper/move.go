package helper

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

const DirChildBatchSize = 100

type MoveFileInfo struct {
	SrcFile string
	DestDir string
	Error   error
}

func MoveDirectory(moveLimiter chan int, srcDir string, destParentDir string) error {
	srcDir = strings.ReplaceAll(srcDir, "\\", "/")
	srcDir = strings.ReplaceAll(srcDir, "//", "/")
	destParentDir = strings.ReplaceAll(destParentDir, "\\", "/")
	destParentDir = strings.ReplaceAll(destParentDir, "//", "/")
	srcDirInfo, err := os.Stat(srcDir)
	if err != nil {
		return err
	}

	if !srcDirInfo.IsDir() {
		return errors.New(fmt.Sprintf("Source directory '%s' is not a directory", srcDir))
	}

	destParentDirInfo, err := os.Stat(destParentDir)
	if err != nil {
		return err
	}

	if !destParentDirInfo.IsDir() {
		return errors.New(fmt.Sprintf("Destination parent directory '%s' is not a directory", destParentDir))
	}

	destDir := path.Join(destParentDir, path.Base(srcDir))
	destDirInfo, err := os.Lstat(destDir)
	if err == nil {
		if !destDirInfo.IsDir() {
			return errors.New(fmt.Sprintf("Destination parent directory '%s' exists and is not a directory", destDir))
		}
	} else {
		err = os.Mkdir(destDir, srcDirInfo.Mode()&os.ModePerm)
		if err != nil {
			return err
		}

		destDirInfo, err = os.Stat(destDir)
		if err != nil {
			return err
		}
	}

	moveErrors := make([]error, 0)

	done := false
	for !done {
		didWork := false

		dirFile, err := os.Open(srcDir)
		if err != nil {
			return err
		}

		var dirChildrenInfo []os.FileInfo
		for dirChildrenInfo, err = dirFile.Readdir(DirChildBatchSize); err == nil; dirChildrenInfo, err = dirFile.Readdir(DirChildBatchSize) {
			resultChans := make([]chan MoveFileInfo, 0)

			for _, dirChildInfo := range dirChildrenInfo {
				if dirChildInfo.IsDir() {
					childDir := path.Join(srcDir, dirChildInfo.Name())
					err = MoveDirectory(moveLimiter, childDir, destDir)
					if err != nil {
						moveErrors = append(moveErrors, err)
					}
				} else {
					childFile := path.Join(srcDir, dirChildInfo.Name())
					resultChan := ProcessFile(moveLimiter, childFile, destDir)
					resultChans = append(resultChans, resultChan)
				}
			}

			for _, resultChan := range resultChans {
				moveFileInfo := <-resultChan
				if moveFileInfo.Error != nil {
					fmt.Println("moveFileInfo.Error", moveFileInfo.Error,
						"src", moveFileInfo.SrcFile, "dst", moveFileInfo.DestDir)
					moveErrors = append(moveErrors, moveFileInfo.Error)
				}
			}

			didWork = true
		}

		if err != io.EOF {
			return CloseFilesAfterErr(err, dirFile)
		}

		err = dirFile.Close()
		if err != nil {
			return err
		}

		done = !didWork || !(len(moveErrors) == 0)
	}

	if len(moveErrors) > 0 {
		return errors.New(fmt.Sprintf("Errors moving children of directory '%s'", srcDir))
	}

	err = os.Remove(srcDir)
	if err != nil {
		return err
	}

	return nil
}

func ProcessFile(moveLimiter chan int, srcFile string, destFile string) chan MoveFileInfo {
	moveLimiter <- 1

	resultChan := make(chan MoveFileInfo, 1)
	moveFileInfo := MoveFileInfo{srcFile, destFile, nil}
	go func() {
		moveFileInfo.Error = MoveFile(moveFileInfo.SrcFile, moveFileInfo.DestDir)
		resultChan <- moveFileInfo
		<-moveLimiter
	}()
	return resultChan
}

func MoveFile(srcFile string, destDir string) error {
	destFile := path.Join(destDir, path.Base(srcFile))

	srcFileInfo, err := os.Lstat(srcFile)
	if err != nil {
		return err
	}

	srcFileMode := srcFileInfo.Mode()
	isSrcSymLink := srcFileMode&os.ModeSymlink != 0

	if !isSrcSymLink && !srcFileMode.IsRegular() {
		return errors.New(fmt.Sprintf("Source file '%s' is not a regular file or symlink", srcFile))
	}

	destFileInfo, err := os.Lstat(destFile)
	if err == nil {
		if os.SameFile(srcFileInfo, destFileInfo) {
			return nil
		}

		destFileMode := destFileInfo.Mode()
		if isSrcSymLink && destFileMode&os.ModeSymlink != 0 {
			err = os.Remove(destFile)
			if err != nil {
				return err
			}
		} else if !destFileMode.IsRegular() {
			return errors.New(fmt.Sprintf("Destination file '%s' exists and is not a regular file", destFile))
		}
	}

	// copy the symbolic link
	if isSrcSymLink {
		linkDest, err := os.Readlink(srcFile)
		if err != nil {
			return err
		}

		err = os.Symlink(linkDest, destFile)
		if err != nil {
			return err
		}

		return os.Remove(srcFile)
	}

	// copy the file contents
	file, err := os.Open(srcFile)
	if err != nil {
		return err
	}

	newFile, err := os.Create(destFile)
	if err != nil {
		return CloseFilesAfterErr(err, file)
	}

	_, err = io.Copy(newFile, file)
	if err != nil {
		return CloseFilesAfterErr(err, file, newFile)
	}

	err = newFile.Close()
	if err != nil {
		return CloseFilesAfterErr(err, file)
	}

	err = file.Close()
	if err != nil {
		return err
	}

	err = os.Chtimes(destFile, srcFileInfo.ModTime(), srcFileInfo.ModTime())
	if err != nil {
		return err
	}

	err = os.Remove(srcFile)
	if err != nil {
		return err
	}

	return nil
}

func CloseFilesAfterErr(err error, files ...*os.File) error {
	for _, file := range files {
		otherErr := file.Close()
		if otherErr != nil {
			// ignore
		}
	}
	return err
}
