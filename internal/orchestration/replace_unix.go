//go:build !windows

package orchestration

import "os"

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
