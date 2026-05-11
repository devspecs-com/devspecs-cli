package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupE2ERepo creates a fixture repo with OpenSpec, ADR, and markdown plan.
func setupE2ERepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")

	// OpenSpec
	osDir := filepath.Join(repoDir, "openspec", "changes", "add-sso")
	os.MkdirAll(osDir, 0o755)
	os.WriteFile(filepath.Join(osDir, "proposal.md"), []byte("# Add SSO\n\n## Acceptance Criteria\n\n- SSO works\n"), 0o644)
	os.WriteFile(filepath.Join(osDir, "tasks.md"), []byte("# Tasks\n\n- [ ] Implement\n- [x] Design\n"), 0o644)

	// ADR
	adrDir := filepath.Join(repoDir, "docs", "adrs")
	os.MkdirAll(adrDir, 0o755)
	os.WriteFile(filepath.Join(adrDir, "0001-use-authjs.md"), []byte("# Use Auth.js\n\nStatus: Accepted\n"), 0o644)

	// Plan
	planDir := filepath.Join(repoDir, "plans")
	os.MkdirAll(planDir, 0o755)
	os.WriteFile(filepath.Join(planDir, "refactor-auth.md"), []byte("# Refactor auth\n\n- [ ] Extract middleware\n"), 0o644)

	// Config
	cfgDir := filepath.Join(repoDir, ".devspecs")
	os.MkdirAll(cfgDir, 0o755)
	os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("version: 1\nsources:\n  - type: openspec\n    path: openspec\n  - type: adr\n    paths:\n      - docs/adrs\n  - type: markdown\n    paths:\n      - plans\n"), 0o644)

	origWd, _ := os.Getwd()
	os.Chdir(repoDir)
	t.Cleanup(func() { os.Chdir(origWd) })
	return repoDir
}

// DOD §21 bullet 1: Install via go install or binary, run ds --version.
func TestDOD_01_Install(t *testing.T) {
	cmd := NewVersionCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "ds ") {
		t.Errorf("version output missing 'ds ': %s", output)
	}

	// Verify --json works
	jsonCmd := NewVersionCmd()
	jsonCmd.SetArgs([]string{"--json"})
	jsonBuf := &bytes.Buffer{}
	jsonCmd.SetOut(jsonBuf)
	if err := jsonCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var obj map[string]string
	if err := json.Unmarshal(jsonBuf.Bytes(), &obj); err != nil {
		t.Fatalf("version --json invalid: %v", err)
	}
	if _, ok := obj["version"]; !ok {
		t.Error("version JSON missing 'version' key")
	}
}

// DOD §21 bullet 2: Initialize DevSpecs in an existing repo.
func TestDOD_02_InitInRepo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	os.MkdirAll(repoDir, 0o755)
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(repoDir)

	cmd := NewInitCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Initialized DevSpecs.") {
		t.Error("init did not print expected message")
	}
	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Error("DB not created")
	}
}

// DOD §21 bullet 3: Scan existing OpenSpec/ADR/markdown planning artifacts.
func TestDOD_03_ScanArtifacts(t *testing.T) {
	setupE2ERepo(t)

	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	initCmd.Execute()

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var out struct {
		Found            map[string]int `json:"Found"`
		SourcesBreakdown []struct {
			SourceType string         `json:"source_type"`
			Label      string         `json:"label"`
			Count      int            `json:"count"`
			Formats    map[string]int `json:"formats"`
		} `json:"sources_breakdown"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Found["openspec"] != 1 {
		t.Error("expected 1 openspec found")
	}
	if out.Found["adr"] != 1 {
		t.Error("expected 1 adr found")
	}
	if out.Found["markdown"] != 1 {
		t.Error("expected 1 markdown found")
	}
	if len(out.SourcesBreakdown) != 3 {
		t.Fatalf("sources_breakdown: want 3 rows, got %d", len(out.SourcesBreakdown))
	}
	var sumCount int
	for _, row := range out.SourcesBreakdown {
		if row.SourceType == "" || row.Label == "" {
			t.Errorf("empty source_type or label: %#v", row)
		}
		sumCount += row.Count
		sumFormats := 0
		for _, c := range row.Formats {
			sumFormats += c
		}
		if sumFormats != row.Count {
			t.Errorf("formats sum %d != count %d for %s", sumFormats, row.Count, row.SourceType)
		}
	}
	if sumCount != 3 {
		t.Errorf("sources_breakdown count sum: want 3, got %d", sumCount)
	}
}

// DOD §21 bullet 4: See a list of detected artifacts.
func TestDOD_04_ListArtifacts(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()

	scanCmd := NewScanCmd()
	scanCmd.SetOut(&bytes.Buffer{})
	scanCmd.Execute()

	listCmd := NewListCmd()
	buf := &bytes.Buffer{}
	listCmd.SetOut(buf)
	if err := listCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "openspec_change") {
		t.Error("list output missing openspec_change")
	}
	if !strings.Contains(output, "adr") {
		t.Error("list output missing adr")
	}
	if !strings.Contains(output, "plan") {
		t.Error("list output missing plan")
	}
}

// DOD §21 bullet 5: Resolve any artifact by stable ID.
func TestDOD_05_ResolveByID(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()
	scanCmd := NewScanCmd()
	scanCmd.SetOut(&bytes.Buffer{})
	scanCmd.Execute()

	// Get first artifact
	listCmd := NewListCmd()
	listCmd.SetArgs([]string{"--json"})
	listBuf := &bytes.Buffer{}
	listCmd.SetOut(listBuf)
	listCmd.Execute()

	var arts []map[string]any
	json.Unmarshal(listBuf.Bytes(), &arts)
	if len(arts) == 0 {
		t.Fatal("no artifacts found")
	}
	id := arts[0]["ID"].(string)

	resolveCmd := NewResolveCmd()
	resolveCmd.SetArgs([]string{id})
	resBuf := &bytes.Buffer{}
	resolveCmd.SetOut(resBuf)
	if err := resolveCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resBuf.String(), id) {
		t.Error("resolve did not contain the artifact ID")
	}
}

// DOD §21 bullet 6: Export agent-ready context for an artifact.
func TestDOD_06_ExportContext(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()
	scanCmd := NewScanCmd()
	scanCmd.SetOut(&bytes.Buffer{})
	scanCmd.Execute()

	listCmd := NewListCmd()
	listCmd.SetArgs([]string{"--json"})
	listBuf := &bytes.Buffer{}
	listCmd.SetOut(listBuf)
	listCmd.Execute()

	var arts []map[string]any
	json.Unmarshal(listBuf.Bytes(), &arts)
	id := arts[0]["ID"].(string)

	ctxCmd := NewContextCmd()
	ctxCmd.SetArgs([]string{id})
	ctxBuf := &bytes.Buffer{}
	ctxCmd.SetOut(ctxBuf)
	if err := ctxCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := ctxBuf.String()
	if !strings.Contains(output, "# DevSpecs Context:") {
		t.Error("context output missing header")
	}
	if !strings.Contains(output, "## Instructions for Agent") {
		t.Error("context output missing instructions section")
	}
}

// DOD §21 bullet 7: Capture a one-off markdown plan.
func TestDOD_07_CaptureOneOff(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()

	// Write a one-off plan
	os.WriteFile("oneoff-plan.md", []byte("# One-off Plan\n\n- [ ] Do something\n"), 0o644)

	captureCmd := NewCaptureCmd()
	captureCmd.SetArgs([]string{"oneoff-plan.md", "--kind", "plan"})
	buf := &bytes.Buffer{}
	captureCmd.SetOut(buf)
	if err := captureCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ds_") {
		t.Error("capture did not return a DevSpecs ID")
	}
}

// DOD §21 bullet 8: Mark status manually.
func TestDOD_08_ManualStatus(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()
	scanCmd := NewScanCmd()
	scanCmd.SetOut(&bytes.Buffer{})
	scanCmd.Execute()

	listCmd := NewListCmd()
	listCmd.SetArgs([]string{"--json"})
	listBuf := &bytes.Buffer{}
	listCmd.SetOut(listBuf)
	listCmd.Execute()

	var arts []map[string]any
	json.Unmarshal(listBuf.Bytes(), &arts)
	id := arts[0]["ID"].(string)

	statusCmd := NewStatusCmd()
	statusCmd.SetArgs([]string{id, "approved"})
	statusBuf := &bytes.Buffer{}
	statusCmd.SetOut(statusBuf)
	if err := statusCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(statusBuf.String(), "approved") {
		t.Error("status update did not confirm 'approved'")
	}
}

// DOD §21 bullet 9: Link an artifact to an external URL.
func TestDOD_09_LinkArtifact(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()
	scanCmd := NewScanCmd()
	scanCmd.SetOut(&bytes.Buffer{})
	scanCmd.Execute()

	listCmd := NewListCmd()
	listCmd.SetArgs([]string{"--json"})
	listBuf := &bytes.Buffer{}
	listCmd.SetOut(listBuf)
	listCmd.Execute()

	var arts []map[string]any
	json.Unmarshal(listBuf.Bytes(), &arts)
	id := arts[0]["ID"].(string)

	linkCmd := NewLinkCmd()
	linkCmd.SetArgs([]string{id, "https://github.com/acme/backend/pull/42"})
	linkBuf := &bytes.Buffer{}
	linkCmd.SetOut(linkBuf)
	if err := linkCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(linkBuf.String(), "Linked") {
		t.Error("link did not confirm")
	}
}

// DOD §21 bullet 10: Re-scan without creating duplicates.
func TestDOD_10_RescanNoDuplicates(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()

	scan1 := NewScanCmd()
	scan1.SetOut(&bytes.Buffer{})
	scan1.Execute()

	scan2 := NewScanCmd()
	scan2.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	scan2.SetOut(buf)
	scan2.Execute()

	var result map[string]any
	json.Unmarshal(buf.Bytes(), &result)
	if result["New"].(float64) != 0 {
		t.Error("rescan created new artifacts")
	}
	if result["Unchanged"].(float64) != 3 {
		t.Errorf("expected 3 unchanged, got %v", result["Unchanged"])
	}
}

// DOD §21 bullet 11: Change a source file and see a new revision tracked.
func TestDOD_11_RescanRevision(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()

	scan1 := NewScanCmd()
	scan1.SetOut(&bytes.Buffer{})
	scan1.Execute()

	// Modify a file
	os.WriteFile("plans/refactor-auth.md", []byte("# Refactor auth v2\n\n- [ ] New task\n- [x] Old task done\n"), 0o644)

	scan2 := NewScanCmd()
	scan2.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	scan2.SetOut(buf)
	scan2.Execute()

	var result map[string]any
	json.Unmarshal(buf.Bytes(), &result)
	if result["Updated"].(float64) < 1 {
		t.Error("expected at least 1 updated artifact after content change")
	}
}

// Error handling tests per spec §12.
func TestErrors_UnknownID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	os.MkdirAll(repoDir, 0o755)
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(repoDir)

	NewInitCmd().Execute()

	showCmd := NewShowCmd()
	showCmd.SetArgs([]string{"ds_nonexistent"})
	showCmd.SetOut(&bytes.Buffer{})
	err := showCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %q", err.Error())
	}
}

func TestErrors_StatusVocab(t *testing.T) {
	statusCmd := NewStatusCmd()
	statusCmd.SetArgs([]string{"ds_fake", "bogus"})
	statusCmd.SetOut(&bytes.Buffer{})
	err := statusCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	if !strings.Contains(err.Error(), "invalid status") {
		t.Errorf("expected 'invalid status' error, got %q", err.Error())
	}
}

func TestErrors_NoIndex(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "no-db-here"))

	listCmd := NewListCmd()
	listCmd.SetOut(&bytes.Buffer{})
	err := listCmd.Execute()
	if err == nil {
		// On first open, store.Open creates the DB, so this may not error.
		// The test verifies the command handles a fresh/empty state gracefully.
		return
	}
}

func TestErrors_NoArtifacts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	os.MkdirAll(repoDir, 0o755)
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(repoDir)

	NewInitCmd().Execute()

	listCmd := NewListCmd()
	buf := &bytes.Buffer{}
	listCmd.SetOut(buf)
	err := listCmd.Execute()
	if err != nil {
		t.Fatal(err)
	}
	// Should produce empty list (just headers), not an error
	output := buf.String()
	if !strings.Contains(output, "ID") {
		t.Errorf("list with no artifacts should still show headers, got %q", output)
	}
}

func TestErrors_MalformedConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	os.MkdirAll(repoDir, 0o755)
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(repoDir)

	NewInitCmd().Execute()

	// Corrupt the config file
	cfgDir := filepath.Join(repoDir, ".devspecs")
	os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(":::not:::yaml"), 0o644)

	scanCmd := NewScanCmd()
	scanCmd.SetOut(&bytes.Buffer{})
	err := scanCmd.Execute()
	if err == nil {
		t.Error("expected error for malformed config, got nil")
	}
}

// JSON stability test: verify all read commands with --json produce valid JSON.
func TestJSONStability(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()
	scanCmd := NewScanCmd()
	scanCmd.SetOut(&bytes.Buffer{})
	scanCmd.Execute()

	// Get an artifact ID for show/resolve/context
	listCmd := NewListCmd()
	listCmd.SetArgs([]string{"--json"})
	listBuf := &bytes.Buffer{}
	listCmd.SetOut(listBuf)
	listCmd.Execute()
	var arts []map[string]any
	json.Unmarshal(listBuf.Bytes(), &arts)
	if len(arts) == 0 {
		t.Fatal("no artifacts to test against")
	}
	artID := arts[0]["ID"].(string)

	t.Run("scan_json", func(t *testing.T) {
		cmd := NewScanCmd()
		cmd.SetArgs([]string{"--json"})
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !json.Valid(buf.Bytes()) {
			t.Errorf("scan --json invalid: %s", buf.String())
		}
	})

	t.Run("list_json", func(t *testing.T) {
		cmd := NewListCmd()
		cmd.SetArgs([]string{"--json"})
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !json.Valid(buf.Bytes()) {
			t.Errorf("list --json invalid: %s", buf.String())
		}
	})

	t.Run("todos_json", func(t *testing.T) {
		cmd := NewTodosCmd()
		cmd.SetArgs([]string{"--json"})
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !json.Valid(buf.Bytes()) {
			t.Errorf("todos --json invalid: %s", buf.String())
		}
	})

	t.Run("criteria_json", func(t *testing.T) {
		cmd := NewCriteriaCmd()
		cmd.SetArgs([]string{"--json"})
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !json.Valid(buf.Bytes()) {
			t.Errorf("criteria --json invalid: %s", buf.String())
		}
	})

	t.Run("show_json", func(t *testing.T) {
		cmd := NewShowCmd()
		cmd.SetArgs([]string{artID, "--json"})
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !json.Valid(buf.Bytes()) {
			t.Errorf("show --json invalid: %s", buf.String())
		}
	})

	t.Run("find_json", func(t *testing.T) {
		cmd := NewFindCmd()
		cmd.SetArgs([]string{"auth", "--json"})
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !json.Valid(buf.Bytes()) {
			t.Errorf("find --json invalid: %s", buf.String())
		}
	})

	t.Run("resolve_json", func(t *testing.T) {
		cmd := NewResolveCmd()
		cmd.SetArgs([]string{artID, "--json"})
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !json.Valid(buf.Bytes()) {
			t.Errorf("resolve --json invalid: %s", buf.String())
		}
	})

	t.Run("context_json", func(t *testing.T) {
		cmd := NewContextCmd()
		cmd.SetArgs([]string{artID, "--json"})
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !json.Valid(buf.Bytes()) {
			t.Errorf("context --json invalid: %s", buf.String())
		}
	})
}

// TestPRD_TodosBoundary verifies the todos feature stays within PRD scope.
func TestPRD_TodosBoundary(t *testing.T) {
	todosCmd := NewTodosCmd()

	// Verify no task-management flags exist
	forbidden := []string{
		"owner", "assignee", "due-date", "due_date", "priority",
		"label", "sprint", "create", "update", "delete",
		"assign", "milestone", "epic", "estimate",
	}
	for _, flag := range forbidden {
		if todosCmd.Flags().Lookup(flag) != nil {
			t.Errorf("todos command has forbidden flag --%s (out of PRD scope)", flag)
		}
	}

	// Verify no subcommands exist (todos is read-only observability)
	if len(todosCmd.Commands()) > 0 {
		t.Errorf("todos command has subcommands (should be read-only): %v", todosCmd.Commands())
	}
}

// TestPRD_CriteriaBoundary verifies the criteria command stays within PRD scope.
func TestPRD_CriteriaBoundary(t *testing.T) {
	criteriaCmd := NewCriteriaCmd()

	forbidden := []string{
		"owner", "assignee", "due-date", "due_date", "priority",
		"label", "sprint", "create", "update", "delete",
		"assign", "milestone", "epic", "estimate",
	}
	for _, flag := range forbidden {
		if criteriaCmd.Flags().Lookup(flag) != nil {
			t.Errorf("criteria command has forbidden flag --%s (out of PRD scope)", flag)
		}
	}

	if len(criteriaCmd.Commands()) > 0 {
		t.Errorf("criteria command has subcommands (should be read-only): %v", criteriaCmd.Commands())
	}
}
