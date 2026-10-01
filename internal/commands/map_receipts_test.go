package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapTraceReceipts_WithExpiredDeadline_ReturnsErrorInsteadOfEmptySuccess(t *testing.T) {
	repoRoot := setupGitRepo(t)
	writeMapTestFile(t, repoRoot, "command.go", "package command\n")
	runGitForFindPack(t, repoRoot, "add", ".")
	runGitForFindPack(t, repoRoot, "commit", "-m", "fix command usage padding")
	area := &mapAreaInternal{Label: "Command", Artifacts: []mapArtifact{{Path: "command.go"}}}
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancel()

	receipts, err := mapTraceReceipts(ctx, repoRoot, area, nil)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Nil(t, receipts)
}

func TestMapTraceReceipts_WithCommittedEvidence_ReturnsReceipt(t *testing.T) {
	repoRoot := setupGitRepo(t)
	writeMapTestFile(t, repoRoot, "command.go", "package command\n")
	runGitForFindPack(t, repoRoot, "add", ".")
	runGitForFindPack(t, repoRoot, "commit", "-m", "fix command usage padding")
	area := &mapAreaInternal{Label: "Command", Artifacts: []mapArtifact{{Path: "command.go"}}}

	receipts, err := mapTraceReceipts(t.Context(), repoRoot, area, nil)

	require.NoError(t, err)
	require.Len(t, receipts, 1)
	assert.Equal(t, "fix command usage padding", receipts[0].Subject)
	assert.NotEmpty(t, receipts[0].SHA)
}

func TestMapTraceReceipts_WithoutArtifactPaths_ReturnsNoReceipts(t *testing.T) {
	area := &mapAreaInternal{Label: "Command"}

	receipts, err := mapTraceReceipts(t.Context(), t.TempDir(), area, nil)

	require.NoError(t, err)
	assert.Nil(t, receipts)
}

func TestMapGitHistoryAvailable_WithoutGitMetadata_ReturnsUnavailable(t *testing.T) {
	repoRoot := t.TempDir()

	available, err := mapGitHistoryAvailable(t.Context(), repoRoot)

	require.NoError(t, err)
	assert.False(t, available)
}

func TestMapGitHistoryAvailable_WithUnbornBranch_ReturnsUnavailable(t *testing.T) {
	repoRoot := t.TempDir()
	runGitForFindPack(t, repoRoot, "init")

	available, err := mapGitHistoryAvailable(t.Context(), repoRoot)

	require.NoError(t, err)
	assert.False(t, available)
}

func TestMapGitHistoryAvailable_WithInvalidGitMetadata_ReturnsError(t *testing.T) {
	repoRoot := t.TempDir()
	writeMapTestFile(t, repoRoot, ".git", "not a git directory\n")

	available, err := mapGitHistoryAvailable(t.Context(), repoRoot)

	require.ErrorContains(t, err, "inspect map git repository")
	assert.False(t, available)
}

func TestMapGitHistoryAvailable_WithMissingHeadObject_ReturnsError(t *testing.T) {
	repoRoot := setupGitRepo(t)
	headRef := filepath.Join(repoRoot, ".git", "refs", "heads", "main")
	require.NoError(t, os.WriteFile(headRef, []byte("1111111111111111111111111111111111111111\n"), 0o644))

	available, err := mapGitHistoryAvailable(t.Context(), repoRoot)

	require.ErrorContains(t, err, "inspect map git HEAD")
	assert.False(t, available)
}

func TestBuildPathBoundaryMapOutput_WithExpiredDeadline_ReturnsError(t *testing.T) {
	repoRoot := setupGitRepo(t)
	ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer cancel()

	out, err := buildPathBoundaryMapOutput(ctx, repoRoot, mapOptions{NoRefresh: true})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Empty(t, out.Schema)
	assert.Nil(t, out.Areas)
}

func TestReadFindGitLog_WithCanceledContext_ReturnsCancellation(t *testing.T) {
	repoRoot := setupGitRepo(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	commits, err := readFindGitLog(ctx, repoRoot, 10)

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, commits)
}

func TestReadFindGitLog_OutsideRepository_ReturnsError(t *testing.T) {
	repoRoot := t.TempDir()

	commits, err := readFindGitLog(t.Context(), repoRoot, 10)

	require.ErrorContains(t, err, "read git history")
	assert.Nil(t, commits)
}

func TestPublicMapAreas_WithCanceledContextAndExistingReceipts_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	areas := []*mapAreaInternal{{Label: "Command", TraceReceipts: []mapTraceReceipt{{SHA: "abc123", Subject: "fix command"}}}}

	got, err := publicMapAreasContext(ctx, t.TempDir(), "sample", areas, 8, "", nil)

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, got)
}

func TestBuildMapRecentOutput_WithExpiredDeadline_ReturnsError(t *testing.T) {
	repoRoot := setupGitRepo(t)
	ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer cancel()

	out, err := buildMapRecentOutput(ctx, repoRoot, mapOptions{})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Empty(t, out.Schema)
	assert.Nil(t, out.Topics)
}

func TestRunMap_WithCanceledContext_WritesNeitherOutputNorCache(t *testing.T) {
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	repoRoot := setupGitRepo(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	cmd.SetOut(&output)
	cachePath, err := mapOutputCachePath(repoRoot)
	require.NoError(t, err)

	err = runMap(cmd, mapOptions{Path: repoRoot, JSON: true, NoRefresh: true})

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, output.String())
	assert.NoFileExists(t, cachePath)
}

func TestRunMap_WithBrokenHeadAfterIndexing_WritesNeitherOutputNorCache(t *testing.T) {
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	repoRoot := setupGitRepo(t)
	insertFreshMapCacheRepo(t, repoRoot)
	cachePath, err := mapOutputCachePath(repoRoot)
	require.NoError(t, err)
	headRef := filepath.Join(repoRoot, ".git", "refs", "heads", "main")
	require.NoError(t, os.WriteFile(headRef, []byte("1111111111111111111111111111111111111111\n"), 0o644))
	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	cmd.SetOut(&output)

	err = runMap(cmd, mapOptions{Path: repoRoot, JSON: true, NoRefresh: true})

	require.ErrorContains(t, err, "inspect map git HEAD")
	assert.Empty(t, output.String())
	assert.NoFileExists(t, cachePath)
}

func TestMapOutputCache_WithLegacyEvidenceVersion_IsRejected(t *testing.T) {
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	repoRoot := setupGitRepo(t)
	insertFreshMapCacheRepo(t, repoRoot)
	cachePath, err := mapOutputCachePath(repoRoot)
	require.NoError(t, err)
	head := string(bytes.TrimSpace(runMapTestGitOutput(t, repoRoot, "rev-parse", "HEAD")))
	payload := cachedMapOutputFile{
		Schema: mapSchemaVersion, RepoRoot: repoRoot,
		LastScanCommit: head, LastScanAt: "2026-06-01T00:00:00Z", MaxAreas: mapDefaultMaxAreas,
		Output: mapOutput{Schema: mapSchemaVersion, Repo: mapRepo{Path: repoRoot}},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(cachePath), 0o755))
	require.NoError(t, os.WriteFile(cachePath, data, 0o644))

	out, hit, err := loadMapOutputCache(t.Context(), repoRoot, mapDefaultMaxAreas)

	require.NoError(t, err)
	assert.False(t, hit)
	assert.Empty(t, out.Schema)
}
