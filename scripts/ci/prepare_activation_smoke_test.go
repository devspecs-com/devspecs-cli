package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShortSHA_WithShortValue_ReturnsUnchangedValue(t *testing.T) {
	short := shortSHA("abc123")

	assert.Equal(t, "abc123", short)
}

func TestShortSHA_WithLongValue_ReturnsTwelveCharacters(t *testing.T) {
	short := shortSHA("1234567890abcdef")

	assert.Equal(t, "1234567890ab", short)
}

func TestPrepareRepo_WithPinnedLocalRepository_ClonesFullHistoryAtCommit(t *testing.T) {
	origin, commitSHA := setupActivationOrigin(t)
	root := filepath.Join(t.TempDir(), "smoke")
	require.NoError(t, os.MkdirAll(root, 0o755))

	prepareRepo(root, activationRepo{ID: "sample", URL: origin, CommitSHA: commitSHA})

	target := filepath.Join(root, "sample")
	assert.FileExists(t, filepath.Join(target, "README.md"))
	assert.Equal(t, commitSHA, gitOutput("-C", target, "rev-parse", "HEAD"))
	assert.Equal(t, "false", gitOutput("-C", target, "rev-parse", "--is-shallow-repository"))
}

func TestMain_WithPinnedManifest_PreparesRepository(t *testing.T) {
	if os.Getenv("DEVSPECS_TEST_PREPARE_MAIN") == "1" {
		os.Args = []string{"prepare_activation_smoke", os.Getenv("DEVSPECS_TEST_MANIFEST")}
		main()
		return
	}
	origin, commitSHA := setupActivationOrigin(t)
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	manifestData, err := json.Marshal(activationManifest{Repos: []activationRepo{{
		ID:        "sample",
		URL:       origin,
		CommitSHA: commitSHA,
	}}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, manifestData, 0o644))
	root := filepath.Join(t.TempDir(), "prepared")
	cmd := exec.Command(os.Args[0], "-test.run=^TestMain_WithPinnedManifest_PreparesRepository$")
	cmd.Env = append(os.Environ(),
		"DEVSPECS_TEST_PREPARE_MAIN=1",
		"DEVSPECS_TEST_MANIFEST="+manifestPath,
		"DEVSPECS_SMOKE_ROOT="+root,
	)

	output, err := cmd.CombinedOutput()

	require.NoError(t, err, string(output))
	assert.Contains(t, string(output), "prepared sample at "+shortSHA(commitSHA))
	assert.FileExists(t, filepath.Join(root, "sample", "README.md"))
}

func TestMain_WithoutSmokeRoot_ExitsWithActionableMessage(t *testing.T) {
	if os.Getenv("DEVSPECS_TEST_MISSING_ROOT") == "1" {
		os.Args = []string{"prepare_activation_smoke", "manifest.json"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMain_WithoutSmokeRoot_ExitsWithActionableMessage$")
	cmd.Env = append(withoutEnvironmentVariable(os.Environ(), "DEVSPECS_SMOKE_ROOT"), "DEVSPECS_TEST_MISSING_ROOT=1")

	output, err := cmd.CombinedOutput()

	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.ExitCode())
	assert.Contains(t, string(output), "DEVSPECS_SMOKE_ROOT must be set")
}

func TestPrepareRepo_WithInvalidRepositoryID_ExitsBeforeGit(t *testing.T) {
	if os.Getenv("DEVSPECS_TEST_INVALID_REPO_ID") == "1" {
		prepareRepo(t.TempDir(), activationRepo{ID: "../escape", URL: "unused", CommitSHA: "abc123"})
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPrepareRepo_WithInvalidRepositoryID_ExitsBeforeGit$")
	cmd.Env = append(os.Environ(), "DEVSPECS_TEST_INVALID_REPO_ID=1")

	output, err := cmd.CombinedOutput()

	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.ExitCode())
	assert.Contains(t, string(output), `invalid repository id "../escape"`)
}

func setupActivationOrigin(t *testing.T) (string, string) {
	t.Helper()
	_, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is required for activation smoke preparation tests")
	}
	origin := filepath.Join(t.TempDir(), "origin")
	require.NoError(t, os.MkdirAll(origin, 0o755))
	runTestGit(t, origin, "init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(origin, "README.md"), []byte("# Fixture\n"), 0o644))
	runTestGit(t, origin, "add", ".")
	runTestGit(t, origin, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
	commitSHA := strings.TrimSpace(runTestGit(t, origin, "rev-parse", "HEAD"))
	return origin, commitSHA
}

func runTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	return string(output)
}

func withoutEnvironmentVariable(environment []string, key string) []string {
	prefix := strings.ToUpper(key) + "="
	filtered := make([]string, 0, len(environment))
	for _, value := range environment {
		if strings.HasPrefix(strings.ToUpper(value), prefix) {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}
