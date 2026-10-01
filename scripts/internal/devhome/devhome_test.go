package devhome

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve_WithPreviewChannel_UsesPersistentChannelHome(t *testing.T) {
	// Arrange
	userHome := t.TempDir()
	sourceRoot := createSourceRoot(t)

	// Act
	selection, err := Resolve(Options{
		Channel:    "preview",
		SourceRoot: sourceRoot,
		UserHome:   userHome,
	})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, ScopeChannel, selection.Scope)
	assert.Equal(t, "preview", selection.Key)
	assert.Equal(t, filepath.Join(userHome, ".devspecs-dev", "channels", "preview"), selection.Home)
	assert.Equal(t, sourceRoot, selection.SourceRoot)
	assert.Len(t, selection.SourceRootHash, 12)
	assert.Equal(t, "disabled", selection.TelemetryDefault)
}

func TestResolve_WithReservedChannel_ReturnsError(t *testing.T) {
	// Arrange
	userHome := t.TempDir()
	sourceRoot := createSourceRoot(t)

	// Act
	_, err := Resolve(Options{
		Channel:    "stable",
		SourceRoot: sourceRoot,
		UserHome:   userHome,
	})

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "reserved")
}

func TestResolve_WithInvalidChannel_ReturnsError(t *testing.T) {
	// Arrange
	userHome := t.TempDir()
	sourceRoot := createSourceRoot(t)

	// Act
	_, err := Resolve(Options{
		Channel:    "Preview Lane",
		SourceRoot: sourceRoot,
		UserHome:   userHome,
	})

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "channel key must match")
}

func TestResolve_WithChannelAndWorktree_ReturnsError(t *testing.T) {
	// Arrange
	userHome := t.TempDir()
	sourceRoot := createSourceRoot(t)

	// Act
	_, err := Resolve(Options{
		Channel:    "preview",
		Worktree:   true,
		SourceRoot: sourceRoot,
		UserHome:   userHome,
	})

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "mutually exclusive")
}

func TestResolve_WithWorktree_UsesDeterministicSourceKey(t *testing.T) {
	// Arrange
	userHome := t.TempDir()
	sourceRoot := createSourceRoot(t)
	expectedHash := sourceRootHash(sourceRoot, runtime.GOOS == "windows")
	expectedKey := sourceSlug(sourceRoot) + "-" + expectedHash

	// Act
	selection, err := Resolve(Options{
		Worktree:   true,
		SourceRoot: sourceRoot,
		UserHome:   userHome,
	})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, ScopeWorktree, selection.Scope)
	assert.Equal(t, expectedKey, selection.Key)
	assert.Equal(t, filepath.Join(userHome, ".devspecs-dev", "worktrees", expectedKey), selection.Home)
}

func TestResolve_WithSameWorktree_ReturnsSameHome(t *testing.T) {
	// Arrange
	userHome := t.TempDir()
	sourceRoot := createSourceRoot(t)
	first, err := Resolve(Options{Worktree: true, SourceRoot: sourceRoot, UserHome: userHome})
	require.NoError(t, err)

	// Act
	second, err := Resolve(Options{Worktree: true, SourceRoot: sourceRoot, UserHome: userHome})

	// Assert
	require.NoError(t, err)
	assert.Equal(t, first.Key, second.Key)
	assert.Equal(t, first.Home, second.Home)
}

func TestResolve_WithDifferentWorktree_UsesDifferentHome(t *testing.T) {
	// Arrange
	userHome := t.TempDir()
	firstSourceRoot := createNamedSourceRoot(t, "first")
	secondSourceRoot := createNamedSourceRoot(t, "second")
	first, err := Resolve(Options{Worktree: true, SourceRoot: firstSourceRoot, UserHome: userHome})
	require.NoError(t, err)

	// Act
	second, err := Resolve(Options{Worktree: true, SourceRoot: secondSourceRoot, UserHome: userHome})

	// Assert
	require.NoError(t, err)
	assert.NotEqual(t, first.Key, second.Key)
	assert.NotEqual(t, first.Home, second.Home)
}

func TestResolve_WithStableRootOverride_ReturnsError(t *testing.T) {
	// Arrange
	userHome := t.TempDir()
	sourceRoot := createSourceRoot(t)
	stableRoot := filepath.Join(userHome, ".devspecs")

	// Act
	_, err := Resolve(Options{
		Channel:    "preview",
		SourceRoot: sourceRoot,
		DevRoot:    stableRoot,
		UserHome:   userHome,
	})

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "must not be the stable home")
}

func TestSourceRootHash_WithWindowsPathCase_UsesSameIdentity(t *testing.T) {
	// Arrange
	upperPath := `C:\Source\DevSpecs-CLI`
	lowerPath := `c:\source\devspecs-cli`

	// Act
	upperHash := sourceRootHash(upperPath, true)
	lowerHash := sourceRootHash(lowerPath, true)

	// Assert
	assert.Equal(t, upperHash, lowerHash)
}

func TestPrepare_WithNewChannelHome_WritesManagedMarker(t *testing.T) {
	// Arrange
	selection := channelSelection(t)
	now := time.Date(2026, 8, 13, 17, 30, 0, 0, time.UTC)

	// Act
	marker, err := Prepare(selection, now)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, marker.SchemaVersion)
	assert.Equal(t, "devspecs-cli", marker.ManagedBy)
	assert.Equal(t, ScopeChannel, marker.Scope)
	assert.Equal(t, "preview", marker.Key)
	assert.Equal(t, "2026-08-13T17:30:00Z", marker.CreatedAt)
	assert.Equal(t, "2026-08-13T17:30:00Z", marker.LastUsedAt)
	assert.FileExists(t, filepath.Join(selection.Home, MarkerFilename))
}

func TestPrepare_WithExistingManagedChannel_PreservesCreatedAt(t *testing.T) {
	// Arrange
	selection := channelSelection(t)
	require.NoError(t, os.MkdirAll(selection.Home, 0o700))
	existing := Marker{
		SchemaVersion:    1,
		ManagedBy:        "devspecs-cli",
		Scope:            ScopeChannel,
		Key:              "preview",
		SourceRoot:       selection.SourceRoot,
		SourceRootHash:   selection.SourceRootHash,
		CreatedAt:        "2026-08-12T10:00:00Z",
		LastUsedAt:       "2026-08-12T10:00:00Z",
		TelemetryDefault: "disabled",
	}
	writeMarkerFixture(t, selection.Home, existing)
	now := time.Date(2026, 8, 13, 18, 0, 0, 0, time.UTC)

	// Act
	marker, err := Prepare(selection, now)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "2026-08-12T10:00:00Z", marker.CreatedAt)
	assert.Equal(t, "2026-08-13T18:00:00Z", marker.LastUsedAt)
}

func TestPrepare_WithNonEmptyUnmarkedHome_ReturnsError(t *testing.T) {
	// Arrange
	selection := channelSelection(t)
	require.NoError(t, os.MkdirAll(selection.Home, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(selection.Home, "devspecs.db"), []byte("existing"), 0o600))

	// Act
	_, err := Prepare(selection, time.Now())

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "refusing to adopt non-empty unmarked development home")
}

func TestPrepare_WithMismatchedMarker_ReturnsError(t *testing.T) {
	// Arrange
	selection := channelSelection(t)
	require.NoError(t, os.MkdirAll(selection.Home, 0o700))
	mismatched := Marker{
		SchemaVersion:    1,
		ManagedBy:        "devspecs-cli",
		Scope:            ScopeChannel,
		Key:              "release-candidate",
		SourceRoot:       selection.SourceRoot,
		SourceRootHash:   selection.SourceRootHash,
		CreatedAt:        "2026-08-12T10:00:00Z",
		LastUsedAt:       "2026-08-12T10:00:00Z",
		TelemetryDefault: "disabled",
	}
	writeMarkerFixture(t, selection.Home, mismatched)

	// Act
	_, err := Prepare(selection, time.Now())

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "does not match selected scope")
}

func TestChildEnvironment_WithoutTelemetryMode_ReplacesHomeAndDisablesTelemetry(t *testing.T) {
	// Arrange
	base := []string{
		"PATH=/usr/bin",
		"DEVSPECS_HOME=/stable",
		"DEVSPECS_TELEMETRY=",
	}

	// Act
	environment := ChildEnvironment(base, "/development/preview")

	// Assert
	assert.Equal(t, "/development/preview", environmentValue(environment, "DEVSPECS_HOME"))
	assert.Equal(t, "0", environmentValue(environment, "DEVSPECS_TELEMETRY"))
	assert.Equal(t, "/usr/bin", environmentValue(environment, "PATH"))
}

func TestChildEnvironment_WithExplicitTelemetryMode_PreservesOptIn(t *testing.T) {
	// Arrange
	base := []string{
		"DEVSPECS_HOME=/stable",
		"DEVSPECS_TELEMETRY=debug",
	}

	// Act
	environment := ChildEnvironment(base, "/development/preview")

	// Assert
	assert.Equal(t, "/development/preview", environmentValue(environment, "DEVSPECS_HOME"))
	assert.Equal(t, "debug", environmentValue(environment, "DEVSPECS_TELEMETRY"))
}

func createSourceRoot(t *testing.T) string {
	t.Helper()
	return createNamedSourceRoot(t, "devspecs-cli")
}

func createNamedSourceRoot(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.MkdirAll(root, 0o755))
	return root
}

func channelSelection(t *testing.T) Selection {
	t.Helper()
	selection, err := Resolve(Options{
		Channel:    "preview",
		SourceRoot: createSourceRoot(t),
		UserHome:   t.TempDir(),
	})
	require.NoError(t, err)
	return selection
}

func writeMarkerFixture(t *testing.T, home string, marker Marker) {
	t.Helper()
	data, err := json.MarshalIndent(marker, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(home, MarkerFilename), append(data, '\n'), 0o600))
}
