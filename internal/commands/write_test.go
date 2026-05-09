package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	setupCaptureEnv(t)

	// First capture
	cmd1 := NewCaptureCmd()
	cmd1.SetArgs([]string{"plan.md", "--kind", "plan"})
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

	// Second capture same path
	cmd2 := NewCaptureCmd()
	cmd2.SetArgs([]string{"plan.md", "--kind", "plan"})
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
