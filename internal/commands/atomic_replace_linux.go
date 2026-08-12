//go:build linux

package commands

import (
	"os"

	"golang.org/x/sys/unix"
)

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}

func publishNewFile(source, destination string) error {
	err := unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE)
	if err != unix.ENOSYS && err != unix.EINVAL {
		return err
	}
	if _, statErr := os.Lstat(destination); statErr == nil {
		return os.ErrExist
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	return os.Rename(source, destination)
}
