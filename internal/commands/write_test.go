package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const capturePlanContent = "# My Plan\n\n## Tasks\n\n- [ ] Task one\n- [x] Task two\n\n## Acceptance Criteria\n\n- [ ] Output is stable\n"

type capturedPlanFixture struct {
	repoDir    string
	planPath   string
	artifactID string
	revisionID string
	db         *store.DB
}

func setupCaptureEnv(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "plan.md"), []byte("# My Plan\n\n- [ ] Task one\n- [x] Task two\n"), 0o644))

	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() {
		assert.NoError(t, os.Chdir(origWd))
	})

	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, initCmd.Execute())
	return repoDir
}

func setupCapturedPlan(t *testing.T) capturedPlanFixture {
	t.Helper()
	repoDir := setupCaptureEnv(t)
	planPath := filepath.Join(repoDir, "plan.md")
	require.NoError(t, os.WriteFile(planPath, []byte(capturePlanContent), 0o644))

	cmd := NewCaptureCmd()
	cmd.SetArgs([]string{"plan.md", "--kind", "spec", "--title", "Initial title", "--status", "draft"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	require.NoError(t, cmd.Execute())
	artifactID := capturedArtifactID(t, buf.String())

	db, err := store.Open(filepath.Join(os.Getenv("DEVSPECS_HOME"), "devspecs.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, db.Close())
	})

	var revisionID string
	require.NoError(t, db.QueryRow("SELECT current_revision_id FROM artifacts WHERE id = ?", artifactID).Scan(&revisionID))
	return capturedPlanFixture{
		repoDir:    repoDir,
		planPath:   planPath,
		artifactID: artifactID,
		revisionID: revisionID,
		db:         db,
	}
}

func setupStaleCapturedPlan(t *testing.T) capturedPlanFixture {
	t.Helper()
	fixture := setupCapturedPlan(t)
	_, err := fixture.db.Exec("UPDATE artifacts SET subtype = 'stale', last_observed_at = '2000-01-01T00:00:00Z' WHERE id = ?", fixture.artifactID)
	require.NoError(t, err)
	_, err = fixture.db.Exec("UPDATE artifact_todos SET text = 'stale todo' WHERE artifact_id = ?", fixture.artifactID)
	require.NoError(t, err)
	_, err = fixture.db.Exec("UPDATE artifact_criteria SET text = 'stale criterion' WHERE artifact_id = ?", fixture.artifactID)
	require.NoError(t, err)
	return fixture
}

func capturedArtifactID(t *testing.T, output string) string {
	t.Helper()
	var artifactID string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ds_") {
			artifactID = trimmed
			break
		}
	}
	require.NotEmpty(t, artifactID, "capture output did not contain an artifact ID: %s", output)
	return artifactID
}

func TestCaptureWithUnchangedContentRefreshesDerivedStateWithoutNewRevision(t *testing.T) {
	fixture := setupStaleCapturedPlan(t)
	cmd := NewCaptureCmd()
	cmd.SetArgs([]string{"plan.md", "--kind", "plan", "--title", "Refreshed title", "--status", "approved"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, fixture.artifactID, capturedArtifactID(t, buf.String()))

	var revisionID, title, kind, subtype, status, observedAt string
	err = fixture.db.QueryRow(`SELECT current_revision_id, title, kind, subtype, status, last_observed_at
		FROM artifacts WHERE id = ?`, fixture.artifactID).Scan(&revisionID, &title, &kind, &subtype, &status, &observedAt)
	require.NoError(t, err)
	assert.Equal(t, fixture.revisionID, revisionID)
	assert.Equal(t, "Refreshed title", title)
	assert.Equal(t, "plan", kind)
	assert.Empty(t, subtype)
	assert.Equal(t, "approved", status)
	assert.NotEqual(t, "2000-01-01T00:00:00Z", observedAt)

	assertCaptureDerivedRow(t, fixture.db, "artifact_todos", fixture.artifactID, revisionID, "Task one")
	assertCaptureDerivedRow(t, fixture.db, "artifact_criteria", fixture.artifactID, revisionID, "Output is stable")

	var ftsTitle, ftsBody string
	require.NoError(t, fixture.db.QueryRow("SELECT title, body FROM artifacts_fts WHERE artifact_id = ?", fixture.artifactID).Scan(&ftsTitle, &ftsBody))
	assert.Equal(t, "Refreshed title", ftsTitle)
	assert.Contains(t, ftsBody, "Output is stable")

	var revisionCount int
	require.NoError(t, fixture.db.QueryRow("SELECT COUNT(*) FROM artifact_revisions WHERE artifact_id = ?", fixture.artifactID).Scan(&revisionCount))
	assert.Equal(t, 1, revisionCount)
}

func TestCaptureWithChangedContentCreatesNewRevision(t *testing.T) {
	fixture := setupCapturedPlan(t)
	require.NoError(t, os.WriteFile(fixture.planPath, []byte(capturePlanContent+"\nChanged body.\n"), 0o644))
	cmd := NewCaptureCmd()
	cmd.SetArgs([]string{"plan.md", "--kind", "plan"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.NoError(t, err)
	var revisionID string
	require.NoError(t, fixture.db.QueryRow("SELECT current_revision_id FROM artifacts WHERE id = ?", fixture.artifactID).Scan(&revisionID))
	assert.NotEqual(t, fixture.revisionID, revisionID)
	var revisionCount int
	require.NoError(t, fixture.db.QueryRow("SELECT COUNT(*) FROM artifact_revisions WHERE artifact_id = ?", fixture.artifactID).Scan(&revisionCount))
	assert.Equal(t, 2, revisionCount)
}

func assertCaptureDerivedRow(t *testing.T, db *store.DB, table, artifactID, revisionID, text string) {
	t.Helper()
	var actualRevisionID, actualText string
	query := "SELECT revision_id, text FROM " + table + " WHERE artifact_id = ? ORDER BY ordinal LIMIT 1"
	require.NoError(t, db.QueryRow(query, artifactID).Scan(&actualRevisionID, &actualText))
	assert.Equal(t, revisionID, actualRevisionID)
	assert.Equal(t, text, actualText)
}

func TestStatusRejectsUnknownVocabulary(t *testing.T) {
	fixture := setupCapturedPlan(t)
	cmd := NewStatusCmd()
	cmd.SetArgs([]string{fixture.artifactID, "invalid_status"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid status")
}

func TestLinkRejectsUnknownType(t *testing.T) {
	fixture := setupCapturedPlan(t)
	cmd := NewLinkCmd()
	cmd.SetArgs([]string{fixture.artifactID, "https://example.com", "--type", "invalid_type"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid link type")
}

func TestShowRendersLinks(t *testing.T) {
	fixture := setupCapturedPlan(t)
	linkCmd := NewLinkCmd()
	linkCmd.SetArgs([]string{fixture.artifactID, "https://github.com/acme/backend/pull/42", "--type", "implements"})
	linkCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, linkCmd.Execute())
	showCmd := NewShowCmd()
	showCmd.SetArgs([]string{fixture.artifactID})
	buf := &bytes.Buffer{}
	showCmd.SetOut(buf)

	err := showCmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "https://github.com/acme/backend/pull/42")
	assert.Contains(t, buf.String(), "implements")
}

func TestStatusPersistsApprovedValue(t *testing.T) {
	fixture := setupCapturedPlan(t)
	cmd := NewStatusCmd()
	cmd.SetArgs([]string{fixture.artifactID, "approved"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.NoError(t, err)
	var status string
	require.NoError(t, fixture.db.QueryRow("SELECT status FROM artifacts WHERE id = ?", fixture.artifactID).Scan(&status))
	assert.Equal(t, "approved", status)
}
