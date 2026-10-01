//go:build windows

package store

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func indexDiskFreeBytes(path string) (uint64, error) {
	volumePath, err := filepath.Abs(path)
	if err != nil {
		return 0, fmt.Errorf("resolve destination volume: %w", err)
	}
	pointer, err := windows.UTF16PtrFromString(volumePath)
	if err != nil {
		return 0, fmt.Errorf("encode destination volume: %w", err)
	}
	var available uint64
	if err := windows.GetDiskFreeSpaceEx(pointer, &available, nil, nil); err != nil {
		return 0, fmt.Errorf("inspect destination free space: %w", err)
	}
	return available, nil
}
