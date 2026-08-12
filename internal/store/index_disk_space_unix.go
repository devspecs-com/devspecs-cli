//go:build !windows

package store

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func indexDiskFreeBytes(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("inspect destination free space: %w", err)
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}
