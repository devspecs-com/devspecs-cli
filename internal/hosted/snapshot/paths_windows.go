//go:build windows

package snapshot

import (
	"strings"

	"golang.org/x/sys/windows"
)

func resolvedPath(name string) (string, error) {
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return "", ErrProvenance
	}
	handle, err := windows.CreateFile(path, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", ErrProvenance
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
	if err != nil || n == 0 || n >= uint32(len(buffer)) {
		return "", ErrProvenance
	}
	resolved := windows.UTF16ToString(buffer[:n])
	if strings.HasPrefix(resolved, `\\?\UNC\`) {
		return "", ErrProvenance
	}
	return strings.TrimPrefix(resolved, `\\?\`), nil
}
