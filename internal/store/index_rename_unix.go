//go:build !windows

package store

import "os"

func replaceIndexFile(sourcePath, destinationPath string) error {
	return os.Rename(sourcePath, destinationPath)
}
