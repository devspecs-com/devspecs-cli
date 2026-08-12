//go:build !windows && !linux

package commands

import (
	"fmt"
	"os"
)

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}

func publishNewFile(source, destination string) error {
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("publish new file: %w", os.ErrExist)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(source, destination)
}
