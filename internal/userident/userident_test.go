package userident

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetect_GitUser(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=TestGitUser",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=TestGitUser",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %v\n%s", args, err, out)
	}

	run("init", "-b", "main")
	run("config", "user.name", "TestGitUser")

	name := Detect(dir)
	assert.Equal(t, "TestGitUser", name,
		"expected 'TestGitUser', got %q", name)

}

func TestDetect_OSUser(t *testing.T) {
	dir := t.TempDir()
	name := Detect(dir)
	assert.NotEqual(t, "", name,
		"Detect() returned empty string in non-git dir")

}

func TestGeneratedFallback_WithoutExistingIdentity_CreatesIdentity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	name := generatedFallback()

	require.NotEmpty(t, name)
	assert.Len(t, name, 8)
	idFile := filepath.Join(home, "identity")
	data, err := os.ReadFile(idFile)
	require.NoError(t, err)
	assert.Equal(t, name, strings.TrimSpace(string(data)))
}

func TestGeneratedFallback_WithExistingIdentity_ReturnsExistingValue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	require.NoError(t, os.WriteFile(filepath.Join(home, "identity"), []byte("existing-id\n"), 0o600))

	name := generatedFallback()

	assert.Equal(t, "existing-id", name)
}

func TestGitUserName_NoRepo(t *testing.T) {
	dir := t.TempDir()
	// In a non-git directory with no global git config, gitUserName returns "".
	// If a global git config exists, it may return the global user.name — that's expected.
	name := gitUserName(dir)
	_ = name // Just verify it doesn't panic
}
