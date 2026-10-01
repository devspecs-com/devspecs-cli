package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/scripts/internal/devhome"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_WithJSONChannel_PrintsSelectionAndCreatesMarker(t *testing.T) {
	// Arrange
	devRoot := filepath.Join(t.TempDir(), "development")
	sourceRoot := filepath.Join(t.TempDir(), "devspecs-cli")
	require.NoError(t, os.MkdirAll(sourceRoot, 0o755))
	t.Setenv("DEVSPECS_DEV_HOME_ROOT", devRoot)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Act
	exitCode := run(context.Background(), []string{
		"--channel", "preview",
		"--source-root", sourceRoot,
		"--json",
	}, bytes.NewReader(nil), stdout, stderr)

	// Assert
	assert.Equal(t, 0, exitCode, stderr.String())
	var selection devhome.Selection
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &selection))
	assert.Equal(t, devhome.ScopeChannel, selection.Scope)
	assert.Equal(t, "preview", selection.Key)
	assert.Equal(t, filepath.Join(devRoot, "channels", "preview"), selection.Home)
	assert.FileExists(t, filepath.Join(selection.Home, devhome.MarkerFilename))
}

func TestRun_WithJSONAndChildCommand_ReturnsUsageError(t *testing.T) {
	// Arrange
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Act
	exitCode := run(context.Background(), []string{"--json", "--", "ds", "version"}, bytes.NewReader(nil), stdout, stderr)

	// Assert
	assert.Equal(t, 2, exitCode)
	assert.Contains(t, stderr.String(), "--json cannot be combined with a child command")
}

func TestRun_WithChildCommand_PassesIsolatedEnvironmentWithoutChangingParent(t *testing.T) {
	// Arrange
	devRoot := filepath.Join(t.TempDir(), "development")
	sourceRoot := filepath.Join(t.TempDir(), "devspecs-cli")
	childDir := filepath.Join(t.TempDir(), "target-repo")
	capturePath := filepath.Join(t.TempDir(), "environment.json")
	require.NoError(t, os.MkdirAll(sourceRoot, 0o755))
	require.NoError(t, os.MkdirAll(childDir, 0o755))
	t.Setenv("DEVSPECS_DEV_HOME_ROOT", devRoot)
	t.Setenv("DEVSPECS_HOME", filepath.Join(t.TempDir(), "stable"))
	t.Setenv("DEVSPECS_TELEMETRY", "")
	t.Setenv("DEVSPECS_DEV_ENV_CAPTURE", capturePath)
	parentHome := os.Getenv("DEVSPECS_HOME")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Act
	exitCode := run(context.Background(), []string{
		"--channel", "preview",
		"--source-root", sourceRoot,
		"--child-dir", childDir,
		"--quiet",
		"--", os.Args[0], "-test.run=^TestDevEnvChildProcess$",
	}, bytes.NewReader(nil), stdout, stderr)

	// Assert
	assert.Equal(t, 0, exitCode, stderr.String())
	data, err := os.ReadFile(capturePath)
	require.NoError(t, err)
	var environment map[string]string
	require.NoError(t, json.Unmarshal(data, &environment))
	assert.Equal(t, filepath.Join(devRoot, "channels", "preview"), environment["devspecs_home"])
	assert.Equal(t, "0", environment["devspecs_telemetry"])
	assert.Equal(t, childDir, environment["working_directory"])
	assert.Equal(t, parentHome, os.Getenv("DEVSPECS_HOME"))
}

func TestDevEnvChildProcess(t *testing.T) {
	// Arrange
	capturePath := os.Getenv("DEVSPECS_DEV_ENV_CAPTURE")
	if capturePath == "" {
		t.Skip("helper process")
	}
	environment := map[string]string{
		"devspecs_home":      os.Getenv("DEVSPECS_HOME"),
		"devspecs_telemetry": os.Getenv("DEVSPECS_TELEMETRY"),
		"working_directory":  mustWorkingDirectory(t),
	}
	data, err := json.Marshal(environment)
	require.NoError(t, err)

	// Act
	err = os.WriteFile(capturePath, data, 0o600)

	// Assert
	require.NoError(t, err)
}

func mustWorkingDirectory(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	require.NoError(t, err)
	return workingDirectory
}
