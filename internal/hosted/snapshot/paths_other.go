//go:build !windows

package snapshot

import "path/filepath"

func resolvedPath(name string) (string, error) { return filepath.EvalSymlinks(name) }
