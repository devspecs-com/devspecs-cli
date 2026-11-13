//go:build !windows

package orchestration

import "path/filepath"

func httpDriverResolvedPath(name string) (string, error) {
	return filepath.EvalSymlinks(name)
}
