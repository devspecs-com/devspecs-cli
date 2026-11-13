//go:build !windows

package devhome

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_WithMissingDescendantOfStableSymlink_RejectsOverride(t *testing.T) {
	// Arrange
	userHome := t.TempDir()
	stable := filepath.Join(userHome, stableRootName)
	require.NoError(t, os.MkdirAll(stable, 0o700))
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(stable, alias))
	opts := Options{Channel: "preview", SourceRoot: createSourceRoot(t), UserHome: userHome, DevRoot: filepath.Join(alias, "missing", "child")}

	// Act
	selection, err := Resolve(opts)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "must not be the stable home")
	assert.Empty(t, selection)
}

func TestResolve_WithStableHomeSymlink_RejectsPhysicalDescendant(t *testing.T) {
	// Arrange
	userHome := t.TempDir()
	stable := t.TempDir()
	require.NoError(t, os.Symlink(stable, filepath.Join(userHome, stableRootName)))
	opts := Options{Channel: "preview", SourceRoot: createSourceRoot(t), UserHome: userHome, DevRoot: filepath.Join(stable, "missing", "child")}

	// Act
	selection, err := Resolve(opts)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "must not be the stable home")
	assert.Empty(t, selection)
}
