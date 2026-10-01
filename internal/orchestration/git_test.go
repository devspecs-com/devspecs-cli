package orchestration

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeRemote_WithHTTPSCredentials_RemovesSecrets(t *testing.T) {
	remote := "https://alice:secret@github.com/example/project.git?token=secret#secret"

	got := normalizeRemote(remote)

	assert.Equal(t, "https://github.com/example/project", got)
}

func TestNormalizeRemote_WithSSHURL_RemovesUsername(t *testing.T) {
	remote := "ssh://alice@github.com/example/project.git"

	got := normalizeRemote(remote)

	assert.Equal(t, "https://github.com/example/project", got)
}

func TestNormalizeRemote_WithSCPRemote_RemovesArbitraryUsername(t *testing.T) {
	remote := "alice@github.com:example/project.git"

	got := normalizeRemote(remote)

	assert.Equal(t, "https://github.com/example/project", got)
}

func TestNormalizeRemote_WithLocalWindowsPath_PreservesRepository(t *testing.T) {
	remote := "C:/repos/project.git"

	got := normalizeRemote(remote)

	assert.Equal(t, "C:/repos/project", got)
}

func TestNormalizeRemote_WithMalformedCredentialURL_DoesNotExposeRemote(t *testing.T) {
	remote := "https://alice:secret@%invalid/project.git"

	got := normalizeRemote(remote)

	assert.Equal(t, "root", got)
}

func TestResultRepositoryState_WithChangedUntrackedBytes_ChangesDigest(t *testing.T) {
	// Arrange
	repo := newGitStateTestRepository(t)
	file := filepath.Join(repo, "result with spaces.md")
	require.NoError(t, os.WriteFile(file, []byte("first result"), 0o600))
	_, originalDigest, err := resultRepositoryState(context.Background(), ExecRunner{}, repo)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, []byte("other result"), 0o600))

	// Act
	commit, digest, err := resultRepositoryState(context.Background(), ExecRunner{}, repo)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, commit)
	assert.NotEmpty(t, *commit)
	assert.NotEqual(t, originalDigest, digest)
}

func TestResultRepositoryState_WithUnchangedUntrackedFile_PreservesDigest(t *testing.T) {
	// Arrange
	repo := newGitStateTestRepository(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "result.md"), []byte("same result"), 0o600))
	_, originalDigest, err := resultRepositoryState(context.Background(), ExecRunner{}, repo)
	require.NoError(t, err)

	// Act
	_, digest, err := resultRepositoryState(context.Background(), ExecRunner{}, repo)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, originalDigest, digest)
}

func TestResultRepositoryState_WithUntrackedListingFailure_ReturnsNoDigest(t *testing.T) {
	runner := &waspflowRunner{results: []waspflowRun{
		{result: CommandResult{Stdout: []byte("commit\n")}}, {}, {}, {},
		{result: CommandResult{ExitCode: 1, Stderr: []byte("listing failed")}},
	}}

	commit, digest, err := resultRepositoryState(context.Background(), runner, t.TempDir())

	require.Error(t, err)
	assert.ErrorContains(t, err, "listing failed")
	assert.Nil(t, commit)
	assert.Empty(t, digest)
}

func TestResultRepositoryState_WithMissingUntrackedFile_ReturnsNoDigest(t *testing.T) {
	runner := &waspflowRunner{results: []waspflowRun{
		{result: CommandResult{Stdout: []byte("commit\n")}}, {}, {}, {},
		{result: CommandResult{Stdout: []byte("missing.md\x00")}},
	}}

	commit, digest, err := resultRepositoryState(context.Background(), runner, t.TempDir())

	require.Error(t, err)
	assert.ErrorContains(t, err, "inspect untracked result")
	assert.Nil(t, commit)
	assert.Empty(t, digest)
}

func TestResultRepositoryState_WithUntrackedHashFailure_ReturnsNoDigest(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repo, "result.md"), []byte("result"), 0o600))
	runner := &waspflowRunner{results: []waspflowRun{
		{result: CommandResult{Stdout: []byte("commit\n")}}, {}, {}, {},
		{result: CommandResult{Stdout: []byte("result.md\x00")}},
		{result: CommandResult{ExitCode: 1, Stderr: []byte("hash failed")}},
	}}

	commit, digest, err := resultRepositoryState(context.Background(), runner, repo)

	require.Error(t, err)
	assert.ErrorContains(t, err, "hash untracked result")
	assert.Nil(t, commit)
	assert.Empty(t, digest)
}

func TestResultRepositoryState_WithChangedUntrackedSymlink_ChangesDigest(t *testing.T) {
	// Arrange
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires privileges on Windows")
	}
	repo := newGitStateTestRepository(t)
	link := filepath.Join(repo, "result")
	require.NoError(t, os.Symlink("first-target", link))
	_, originalDigest, err := resultRepositoryState(context.Background(), ExecRunner{}, repo)
	require.NoError(t, err)
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink("other-target", link))

	// Act
	_, digest, err := resultRepositoryState(context.Background(), ExecRunner{}, repo)

	// Assert
	require.NoError(t, err)
	assert.NotEqual(t, originalDigest, digest)
}

func newGitStateTestRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	_, err := runGitBytes(context.Background(), ExecRunner{}, repo, "init")
	require.NoError(t, err)
	_, err = runGitBytes(context.Background(), ExecRunner{}, repo,
		"-c", "user.name=Test", "-c", "user.email=test@example.invalid",
		"-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "Initial state")
	require.NoError(t, err)
	return repo
}
