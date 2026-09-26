//go:build !windows

package hubstore

import (
	"fmt"
	"os"
	"syscall"
)

func platformFileIdentity(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ErrBindingConflict
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
