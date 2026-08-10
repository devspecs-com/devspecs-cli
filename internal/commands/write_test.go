package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func setupCaptureEnv(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	os.MkdirAll(repoDir, 0o755)
	os.WriteFile(filepath.Join(repoDir, "plan.md"), []byte("# My Plan\n\n- [ ] Task one\n- [x] Task two\n"), 0o644)

	// Init the DB
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	origWd, _ := os.Getwd()
	os.Chdir(repoDir)
	t.Cleanup(func() { os.Chdir(origWd) })
	initCmd.Execute()
	return repoDir
}

func TestCapture_Idempotent(t *testing.T) {
	repoDir := setupCaptureEnv(t)
	planPath := filepath.Join(repoDir, "plan.md")
	content := "# My Plan\n\n## Tasks\n\n- [ ] Task one\n- [x] Task two\n\n## Acceptance Criteria\n\n- [ ] Output is stable\n"
	if err := os.WriteFile(planPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// First capture
	cmd1 := NewCaptureCmd()
	cmd1.SetArgs([]string{"plan.md", "--kind", "spec", "--title", "Initial title", "--status", "draft"})
	buf1 := &bytes.Buffer{}
	cmd1.SetOut(buf1)
	if err := cmd1.Execute(); err != nil {
		t.Fatal(err)
	}
	out1 := buf1.String()

	// Extract ID
	lines := strings.Split(strings.TrimSpace(out1), "\n")
	var id1 string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ds_") {
			id1 = trimmed
			break
		}
	}

	db, err := store.Open(filepath.Join(os.Getenv("DEVSPECS_HOME"), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var firstRevisionID string
	if err := db.QueryRow("SELECT current_revision_id FROM artifacts WHERE id = ?", id1).Scan(&firstRevisionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE artifacts SET subtype = 'stale', last_observed_at = '2000-01-01T00:00:00Z' WHERE id = ?", id1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE artifact_todos SET text = 'stale todo' WHERE artifact_id = ?", id1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE artifact_criteria SET text = 'stale criterion' WHERE artifact_id = ?", id1); err != nil {
		t.Fatal(err)
	}

	// Second capture keeps the same content but refreshes derived metadata.
	cmd2 := NewCaptureCmd()
	cmd2.SetArgs([]string{"plan.md", "--kind", "plan", "--title", "Refreshed title", "--status", "approved"})
	buf2 := &bytes.Buffer{}
	cmd2.SetOut(buf2)
	if err := cmd2.Execute(); err != nil {
		t.Fatal(err)
	}
	out2 := buf2.String()

	var id2 string
	for _, line := range strings.Split(strings.TrimSpace(out2), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ds_") {
			id2 = trimmed
			break
		}
	}

	if id1 != id2 {
		t.Errorf("capture not idempotent: %q vs %q", id1, id2)
	}
	var secondRevisionID, title, kind, subtype, status, observedAt string
	if err := db.QueryRow(`SELECT current_revision_id, title, kind, subtype, status, last_observed_at
		FROM artifacts WHERE id = ?`, id1).Scan(&secondRevisionID, &title, &kind, &subtype, &status, &observedAt); err != nil {
		t.Fatal(err)
	}
	if secondRevisionID != firstRevisionID {
		t.Fatalf("unchanged capture revision changed: %q -> %q", firstRevisionID, secondRevisionID)
	}
	if title != "Refreshed title" || kind != "plan" || subtype != "" || status != "approved" || observedAt == "2000-01-01T00:00:00Z" {
		t.Fatalf("capture metadata not refreshed: title=%q kind=%q subtype=%q status=%q observed=%q", title, kind, subtype, status, observedAt)
	}
	assertCaptureDerivedRow(t, db, "artifact_todos", id1, secondRevisionID, "Task one")
	assertCaptureDerivedRow(t, db, "artifact_criteria", id1, secondRevisionID, "Output is stable")
	var ftsTitle, ftsBody string
	if err := db.QueryRow("SELECT title, body FROM artifacts_fts WHERE artifact_id = ?", id1).Scan(&ftsTitle, &ftsBody); err != nil {
		t.Fatal(err)
	}
	if ftsTitle != "Refreshed title" || !strings.Contains(ftsBody, "Output is stable") {
		t.Fatalf("capture FTS not refreshed: title=%q body=%q", ftsTitle, ftsBody)
	}
	var revisionCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM artifact_revisions WHERE artifact_id = ?", id1).Scan(&revisionCount); err != nil {
		t.Fatal(err)
	}
	if revisionCount != 1 {
		t.Fatalf("unchanged capture created revisions: %d", revisionCount)
	}

	if err := os.WriteFile(planPath, []byte(content+"\nChanged body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd3 := NewCaptureCmd()
	cmd3.SetArgs([]string{"plan.md", "--kind", "plan"})
	cmd3.SetOut(&bytes.Buffer{})
	if err := cmd3.Execute(); err != nil {
		t.Fatal(err)
	}
	var thirdRevisionID string
	if err := db.QueryRow("SELECT current_revision_id FROM artifacts WHERE id = ?", id1).Scan(&thirdRevisionID); err != nil {
		t.Fatal(err)
	}
	if thirdRevisionID == secondRevisionID {
		t.Fatal("changed capture did not create a revision")
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM artifact_revisions WHERE artifact_id = ?", id1).Scan(&revisionCount); err != nil {
		t.Fatal(err)
	}
	if revisionCount != 2 {
		t.Fatalf("changed capture revision count = %d, want 2", revisionCount)
	}
}

func assertCaptureDerivedRow(t *testing.T, db *store.DB, table, artifactID, revisionID, text string) {
	t.Helper()
	var gotRevisionID, gotText string
	query := "SELECT revision_id, text FROM " + table + " WHERE artifact_id = ? ORDER BY ordinal LIMIT 1"
	if err := db.QueryRow(query, artifactID).Scan(&gotRevisionID, &gotText); err != nil {
		t.Fatal(err)
	}
	if gotRevisionID != revisionID || gotText != text {
		t.Fatalf("%s row = revision %q text %q, want revision %q text %q", table, gotRevisionID, gotText, revisionID, text)
	}
}

func TestStatus_RejectsUnknownVocab(t *testing.T) {
	setupCaptureEnv(t)

	// Capture first
	cmd := NewCaptureCmd()
	cmd.SetArgs([]string{"plan.md"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.Execute()

	// Try invalid status
	statusCmd := NewStatusCmd()
	statusCmd.SetArgs([]string{"ds_", "invalid_status"})
	statusCmd.SetOut(&bytes.Buffer{})
	err := statusCmd.Execute()
	if err == nil {
		t.Error("expected error for invalid status")
	}
	if !strings.Contains(err.Error(), "invalid status") {
		t.Errorf("expected 'invalid status' error, got %q", err.Error())
	}
}

func TestLink_RejectsUnknownType(t *testing.T) {
	setupCaptureEnv(t)

	cmd := NewCaptureCmd()
	cmd.SetArgs([]string{"plan.md"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.Execute()

	// Extract artifact ID
	var artID string
	for _, line := range strings.Split(buf.String(), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ds_") {
			artID = trimmed
			break
		}
	}

	linkCmd := NewLinkCmd()
	linkCmd.SetArgs([]string{artID, "https://example.com", "--type", "invalid_type"})
	linkCmd.SetOut(&bytes.Buffer{})
	err := linkCmd.Execute()
	if err == nil {
		t.Error("expected error for invalid link type")
	}
	if !strings.Contains(err.Error(), "invalid link type") {
		t.Errorf("expected 'invalid link type' error, got %q", err.Error())
	}
}

func TestShow_RendersLinks(t *testing.T) {
	setupCaptureEnv(t)

	// Capture
	captureCmd := NewCaptureCmd()
	captureCmd.SetArgs([]string{"plan.md"})
	capBuf := &bytes.Buffer{}
	captureCmd.SetOut(capBuf)
	captureCmd.Execute()

	var artID string
	for _, line := range strings.Split(capBuf.String(), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ds_") {
			artID = trimmed
			break
		}
	}

	// Add link
	linkCmd := NewLinkCmd()
	linkCmd.SetArgs([]string{artID, "https://github.com/acme/backend/pull/42", "--type", "implements"})
	linkCmd.SetOut(&bytes.Buffer{})
	linkCmd.Execute()

	// Show should include link
	showCmd := NewShowCmd()
	showCmd.SetArgs([]string{artID})
	showBuf := &bytes.Buffer{}
	showCmd.SetOut(showBuf)
	if err := showCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := showBuf.String()
	if !strings.Contains(output, "https://github.com/acme/backend/pull/42") {
		t.Error("show output does not contain link target")
	}
	if !strings.Contains(output, "implements") {
		t.Error("show output does not contain link type")
	}
}

func TestStatus_PersistsAcrossProcess(t *testing.T) {
	setupCaptureEnv(t)

	captureCmd := NewCaptureCmd()
	captureCmd.SetArgs([]string{"plan.md"})
	capBuf := &bytes.Buffer{}
	captureCmd.SetOut(capBuf)
	captureCmd.Execute()

	var artID string
	for _, line := range strings.Split(capBuf.String(), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ds_") {
			artID = trimmed
			break
		}
	}

	// Update status
	statusCmd := NewStatusCmd()
	statusCmd.SetArgs([]string{artID, "approved"})
	statusCmd.SetOut(&bytes.Buffer{})
	if err := statusCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Re-read using show --json and verify
	showCmd := NewShowCmd()
	showCmd.SetArgs([]string{artID, "--json"})
	showBuf := &bytes.Buffer{}
	showCmd.SetOut(showBuf)
	if err := showCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(showBuf.String(), `"status": "approved"`) {
		t.Errorf("status not persisted, got: %s", showBuf.String())
	}
}
