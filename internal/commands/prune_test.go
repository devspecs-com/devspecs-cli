package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPruneCommand_WithDryRunJSON_ReportsStaleDataWithoutDeleting(t *testing.T) {
	home := setupPruneStaleIndex(t)

	cmd := NewPruneCmd()
	cmd.SetArgs([]string{"--dry-run", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var report store.PruneReport
	require.NoError(t, json.Unmarshal(buf.Bytes(), &report))
	assert.True(t, report.DryRun)
	assert.Equal(t, 1, report.RepositoriesPruned)
	assert.Equal(t, 1, report.ArtifactsPruned)
	db, err := store.Open(filepath.Join(home, "devspecs.db"))
	require.NoError(t, err)
	defer db.Close()
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM repos`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestPruneCommand_WithJSON_DeletesStaleData(t *testing.T) {
	home := setupPruneStaleIndex(t)

	cmd := NewPruneCmd()
	cmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var report store.PruneReport
	require.NoError(t, json.Unmarshal(buf.Bytes(), &report))
	assert.False(t, report.DryRun)
	assert.Equal(t, 1, report.RepositoriesPruned)
	assert.Equal(t, 1, report.ArtifactsPruned)
	db, err := store.Open(filepath.Join(home, "devspecs.db"))
	require.NoError(t, err)
	defer db.Close()
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM repos`).Scan(&count))
	assert.Zero(t, count)
}

func setupPruneStaleIndex(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	t.Setenv("DEVSPECS_TELEMETRY", "0")
	db, err := store.Open(filepath.Join(home, "devspecs.db"))
	require.NoError(t, err)

	now := time.Now().UTC().Format(time.RFC3339)
	staleRoot := filepath.Join(t.TempDir(), "deleted")
	{
		_, err := db.Exec(`INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('stale', ?, ?, ?)`, staleRoot, now, now)
		require.NoError(t, err)
	}
	{

		_, err := db.Exec(`INSERT INTO artifacts
		(id, repo_id, kind, subtype, title, status, created_at, updated_at, last_observed_at, authored_at)
		VALUES ('artifact', 'stale', 'plan', '', 'Plan', 'draft', ?, ?, ?, ?)`, now, now, now, now)
		require.NoError(t, err)
	}
	{

		err := db.Close()
		require.NoError(t, err)
	}

	return home
}

func TestPruneCommand_NoIndexDoesNotCreateOne(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	t.Setenv("DEVSPECS_TELEMETRY", "0")
	cmd := NewPruneCmd()
	cmd.SetArgs([]string{"--json"})
	cmd.SetOut(&bytes.Buffer{})
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	{

		_, err := os.Stat(filepath.Join(home, "devspecs.db"))
		assert.True(t, os.IsNotExist(err),
			"prune created an index unexpectedly: %v", err)
	}

}

func TestPruneCommand_VacuumProgressUsesStderr(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	t.Setenv("DEVSPECS_TELEMETRY", "0")
	db, err := store.Open(filepath.Join(home, "devspecs.db"))
	require.NoError(t, err)
	{

		err := db.Close()
		require.NoError(t, err)
	}

	cmd := NewPruneCmd()
	cmd.SetArgs([]string{"--vacuum"})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.Contains(t, stderr.String(), "Prune progress: compacting index", "vacuum progress missing from stderr: %q", stderr.String())
	assert.Contains(t, stderr.String(), "Prune progress: complete", "vacuum progress missing from stderr: %q", stderr.String())
	assert.NotContains(t, stdout.String(), "Prune progress:",
		"progress leaked into stdout: %q", stdout.String())

}

func TestPruneCommand_JSONSuppressesProgress(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	t.Setenv("DEVSPECS_TELEMETRY", "0")
	db, err := store.Open(filepath.Join(home, "devspecs.db"))
	require.NoError(t, err)
	{

		err := db.Close()
		require.NoError(t, err)
	}

	cmd := NewPruneCmd()
	cmd.SetArgs([]string{"--vacuum", "--json"})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var report store.PruneReport
	{
		err := json.Unmarshal(stdout.Bytes(), &report)
		require.NoError(t, err,
			"invalid JSON stdout: %v\n%s", err, stdout.String())
	}
	assert.True(t, report.Vacuumed,
		"expected vacuumed report: %#v", report)
	assert.Equal(t, 0, stderr.Len(),
		"JSON progress should be suppressed, got stderr: %q", stderr.String())

}

func TestPruneProgressReporter_DelaysShortOperations(t *testing.T) {
	var out bytes.Buffer
	progress := newPruneProgressReporter(&out, true, time.Hour)
	progress.setPhase("inspect")
	progress.setPhase("complete")
	progress.stop()
	assert.Equal(t, 0, out.Len(),
		"short operation emitted progress: %q", out.String())

}
