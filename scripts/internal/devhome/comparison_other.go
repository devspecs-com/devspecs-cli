//go:build !windows

package devhome

import "path/filepath"

func comparisonAncestor(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
