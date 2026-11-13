//go:build windows

package devhome

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

func comparisonAncestor(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	const volumeNameGUID = 0x1
	buffer := make([]uint16, 32768)
	length, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), volumeNameGUID)
	if err != nil {
		return "", err
	}
	if length == 0 || length >= uint32(len(buffer)) {
		return "", fmt.Errorf("unable to determine a bounded physical development home path")
	}
	resolved := windows.UTF16ToString(buffer[:length])
	if !strings.HasPrefix(strings.ToLower(resolved), `\\?\volume{`) {
		return "", fmt.Errorf("development home volume identity is not proven")
	}
	return resolved, nil
}
