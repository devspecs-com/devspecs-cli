//go:build windows

package devhome

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathIsSameOrDescendant_WithDifferentWindowsVolumes_IsDisjoint(t *testing.T) {
	// Arrange
	path := `D:\managed\preview`
	parent := `C:\user\.devspecs`

	// Act
	inside, err := pathIsSameOrDescendant(path, parent)

	// Assert
	require.NoError(t, err)
	assert.False(t, inside)
}

func TestPathIsSameOrDescendant_WithCaseInsensitiveWindowsIdentity_IsContained(t *testing.T) {
	// Arrange
	path := `c:\USER\.DEVSPECS`
	parent := `C:\user\.devspecs`

	// Act
	inside, err := pathIsSameOrDescendant(path, parent)

	// Assert
	require.NoError(t, err)
	assert.True(t, inside)
}

func TestPathIsSameOrDescendant_WithCaseInsensitiveWindowsDescendant_IsContained(t *testing.T) {
	// Arrange
	path := `c:\USER\.DEVSPECS\preview`
	parent := `C:\user\.devspecs`

	// Act
	inside, err := pathIsSameOrDescendant(path, parent)

	// Assert
	require.NoError(t, err)
	assert.True(t, inside)
}

func TestPathIsSameOrDescendant_WithDriveRelativeWindowsPath_PreservesError(t *testing.T) {
	// Arrange
	path := `D:managed`
	parent := `C:\user\.devspecs`

	// Act
	inside, err := pathIsSameOrDescendant(path, parent)

	// Assert
	require.Error(t, err)
	assert.False(t, inside)
}

func TestResolve_WithMissingDescendantOfStableJunction_RejectsOverride(t *testing.T) {
	// Arrange
	userHome := t.TempDir()
	stable := filepath.Join(userHome, stableRootName)
	require.NoError(t, os.MkdirAll(stable, 0o700))
	alias := filepath.Join(t.TempDir(), "alias")
	createComparisonJunction(t, alias, stable)
	opts := Options{Channel: "preview", SourceRoot: createSourceRoot(t), UserHome: userHome, DevRoot: filepath.Join(alias, "missing", "child")}

	// Act
	selection, err := Resolve(opts)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "must not be the stable home")
	assert.Empty(t, selection)
	entries, readErr := os.ReadDir(stable)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func TestResolve_WithStableHomeJunction_RejectsPhysicalDescendant(t *testing.T) {
	// Arrange
	userHome := t.TempDir()
	stable := t.TempDir()
	createComparisonJunction(t, filepath.Join(userHome, stableRootName), stable)
	opts := Options{Channel: "preview", SourceRoot: createSourceRoot(t), UserHome: userHome, DevRoot: filepath.Join(stable, "missing", "child")}

	// Act
	selection, err := Resolve(opts)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "must not be the stable home")
	assert.Empty(t, selection)
	entries, readErr := os.ReadDir(stable)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func createComparisonJunction(t *testing.T, link, target string) {
	t.Helper()
	output, err := exec.Command("cmd", "/d", "/c", "mklink", "/J", link, target).CombinedOutput()
	require.NoError(t, err, string(output))
}
