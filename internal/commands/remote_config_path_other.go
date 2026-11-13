//go:build !windows

package commands

import "os"

func remoteConfigReparsePoint(os.FileInfo) bool { return false }
