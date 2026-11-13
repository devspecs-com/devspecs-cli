package commands

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func remoteAuthPTY(t *testing.T, system string) *os.File {
	t.Helper()
	name := fmt.Sprintf(`\\.\pipe\%s-%x-pty0-from-master`, system, os.Getpid())
	path, err := windows.UTF16PtrFromString(name)
	require.NoError(t, err)
	handle, err := windows.CreateNamedPipe(path, windows.PIPE_ACCESS_INBOUND, windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT, 1, 4096, 4096, 0, nil)
	require.NoError(t, err)
	stdin := os.NewFile(uintptr(handle), name)
	t.Cleanup(func() { _ = stdin.Close() })
	return stdin
}

func TestNativeRemoteAuthDeps_WithMSYSTerminalStdin_IsInteractive(t *testing.T) {
	// Arrange
	remoteAuthStdin(t, remoteAuthPTY(t, "msys"))
	deps := nativeRemoteAuthDeps()

	// Act
	interactive := deps.interactive()

	// Assert
	assert.True(t, interactive)
}

func TestNativeRemoteAuthDeps_WithCygwinTerminalStdin_IsInteractive(t *testing.T) {
	// Arrange
	remoteAuthStdin(t, remoteAuthPTY(t, "cygwin"))
	deps := nativeRemoteAuthDeps()

	// Act
	interactive := deps.interactive()

	// Assert
	assert.True(t, interactive)
}

func TestNativeRemoteAuthDeps_WithNativeConsoleStdin_IsInteractive(t *testing.T) {
	// Arrange
	stdin, err := os.Open("CONIN$")
	if err != nil {
		t.Skipf("native Windows console unavailable: %v", err)
	}
	t.Cleanup(func() { _ = stdin.Close() })
	remoteAuthStdin(t, stdin)
	deps := nativeRemoteAuthDeps()

	// Act
	interactive := deps.interactive()

	// Assert
	assert.True(t, interactive)
}
