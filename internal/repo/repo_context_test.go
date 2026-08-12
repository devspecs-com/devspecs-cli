package repo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileFirstCommitDatesContext_WhenCanceled_ReturnsNoHistory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dates := FileFirstCommitDatesContext(ctx, t.TempDir(), []string{"docs/plan.md"})

	assert.Empty(t, dates)
}

func TestFileFirstCommitDateContext_WhenCanceled_ReturnsNoHistory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	date := FileFirstCommitDateContext(ctx, t.TempDir(), "docs/plan.md")

	assert.Empty(t, date)
}

func TestRootCommitContext_WhenCanceled_ReturnsNoRootCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rootCommit := RootCommitContext(ctx, t.TempDir())

	assert.Empty(t, rootCommit)
}

func TestDetectContext_WhenCanceled_ReturnsOnlyFilesystemMetadata(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(repoRoot, ".git"), 0o755))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	info := DetectContext(ctx, repoRoot)

	assert.True(t, info.IsGit)
	assert.Equal(t, repoRoot, info.RootPath)
	assert.Empty(t, info.RemoteURL)
	assert.Empty(t, info.CurrentBranch)
}
