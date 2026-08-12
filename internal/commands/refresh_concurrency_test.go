package commands

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type indexWaitSignalWriter struct {
	once   sync.Once
	waited chan struct{}
}

func (w *indexWaitSignalWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "waiting for another index update") {
		w.once.Do(func() { close(w.waited) })
	}
	return len(p), nil
}

func TestRunScanQuiet_WhenIndexBecameFresh_SkipsRedundantScan(t *testing.T) {
	repoRoot := t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO repos (id, root_path, last_scan_at, created_at, updated_at)
		VALUES ('repo-1', ?, ?, ?, ?)`, canonicalRepoRoot(repoRoot), future, future, future)
	require.NoError(t, err)

	result, scanErr := runScanQuiet(context.Background(), nil, db, repoRoot)
	lease, leaseErr := db.AcquireIndexWriter(context.Background(), nil)
	var releaseErr error
	if lease != nil {
		releaseErr = lease.Release()
	}

	assert.Nil(t, result)
	assert.NoError(t, scanErr)
	assert.NoError(t, leaseErr)
	assert.NoError(t, releaseErr)
}

func TestRunScanQuiet_WhenQueuedScanMadeIndexFresh_SkipsRedundantScan(t *testing.T) {
	repoRoot := t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ownerLease, err := db.AcquireIndexWriter(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ownerLease.Release() })
	waited := make(chan struct{})
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	cmd.SetErr(&indexWaitSignalWriter{waited: waited})
	type scanOutcome struct {
		result *scan.Result
		err    error
	}
	completed := make(chan scanOutcome, 1)
	go func() {
		result, scanErr := runScanQuiet(t.Context(), cmd, db, repoRoot)
		completed <- scanOutcome{result: result, err: scanErr}
	}()
	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "queued auto-index did not report writer contention")
	}
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO repos (id, root_path, last_scan_at, created_at, updated_at)
		VALUES ('repo-queued', ?, ?, ?, ?)`, canonicalRepoRoot(repoRoot), future, future, future)
	require.NoError(t, err)
	require.NoError(t, ownerLease.Release())

	var outcome scanOutcome
	select {
	case outcome = <-completed:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "queued auto-index did not complete after writer release")
	}

	assert.Nil(t, outcome.result)
	assert.NoError(t, outcome.err)
}

func TestRunScanQuiet_WhenContextIsCanceled_ReturnsCancellationAndReleasesWriter(t *testing.T) {
	repoRoot := t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, scanErr := runScanQuiet(ctx, nil, db, repoRoot)
	lease, leaseErr := db.AcquireIndexWriter(context.Background(), nil)
	var releaseErr error
	if lease != nil {
		releaseErr = lease.Release()
	}

	assert.Nil(t, result)
	assert.ErrorIs(t, scanErr, context.Canceled)
	assert.NoError(t, leaseErr)
	assert.NoError(t, releaseErr)
}

func TestAutoIndexDeadline_IsTenMinutes(t *testing.T) {
	deadline := autoIndexDeadline

	assert.Equal(t, 10*time.Minute, deadline)
}

func TestExplicitScanDeadline_IsThirtyMinutes(t *testing.T) {
	deadline := explicitScanDeadline

	assert.Equal(t, 30*time.Minute, deadline)
}

func TestIndexOperationError_WhenAutoIndexDeadlineExpires_ReportsTenMinuteBudget(t *testing.T) {
	err := indexOperationError("auto-index", autoIndexDeadlineLabel, context.DeadlineExceeded)

	assert.ErrorContains(t, err, "auto-index exceeded its 10m deadline")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestIndexOperationError_WhenExplicitScanDeadlineExpires_ReportsThirtyMinuteBudget(t *testing.T) {
	err := indexOperationError("scan", explicitScanDeadlineLabel, context.DeadlineExceeded)

	assert.ErrorContains(t, err, "scan exceeded its 30m deadline")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
