package commands

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0o644)
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func TestAutoScan_TriggersOnStaleIndex(t *testing.T) {
	dir := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	// Create a plan and devspecs config
	planDir := filepath.Join(dir, "plans")
	os.MkdirAll(planDir, 0o755)
	cfgDir := filepath.Join(dir, ".devspecs")
	os.MkdirAll(cfgDir, 0o755)
	os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("version: 1\nsources:\n  - type: markdown\n    paths:\n      - plans\n"), 0o644)

	// Initial scan
	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir})
	scanBuf := &bytes.Buffer{}
	scanCmd.SetOut(scanBuf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Now commit a new plan file (makes HEAD differ from last_scan_commit)
	os.WriteFile(filepath.Join(planDir, "new-plan.md"), []byte("# New Plan\n\n- [ ] Do something\n"), 0o644)
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		cmd.CombinedOutput()
	}
	run("add", ".")
	run("commit", "-m", "add plan")

	// Run ds list — should trigger auto-scan
	listCmd := NewListCmd()
	listCmd.SetArgs([]string{})
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	listCmd.SetOut(outBuf)
	listCmd.SetErr(errBuf)
	if err := listCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Verify the new artifact appears in output
	if !strings.Contains(outBuf.String(), "New Plan") {
		t.Errorf("auto-scan didn't pick up new plan.\nOutput: %s\nStderr: %s", outBuf.String(), errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "Index updated") {
		t.Errorf("expected 'Index updated' message on stderr, got: %s", errBuf.String())
	}
}

func TestAutoScan_NoOpWhenFresh(t *testing.T) {
	dir := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	planDir := filepath.Join(dir, "plans")
	os.MkdirAll(planDir, 0o755)
	os.WriteFile(filepath.Join(planDir, "plan.md"), []byte("# A Plan\n"), 0o644)
	cfgDir := filepath.Join(dir, ".devspecs")
	os.MkdirAll(cfgDir, 0o755)
	os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("version: 1\nsources:\n  - type: markdown\n    paths:\n      - plans\n"), 0o644)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	// Scan
	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir})
	scanCmd.SetOut(&bytes.Buffer{})
	scanCmd.Execute()

	// List — should NOT show "Index updated"
	listCmd := NewListCmd()
	listCmd.SetArgs([]string{})
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	listCmd.SetOut(outBuf)
	listCmd.SetErr(errBuf)
	if err := listCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(errBuf.String(), "Index updated") {
		t.Errorf("unexpected 'Index updated' message when fresh: %s", errBuf.String())
	}
}

func TestAutoScan_SkippedWithNoRefresh(t *testing.T) {
	dir := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	planDir := filepath.Join(dir, "plans")
	os.MkdirAll(planDir, 0o755)
	cfgDir := filepath.Join(dir, ".devspecs")
	os.MkdirAll(cfgDir, 0o755)
	os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("version: 1\nsources:\n  - type: markdown\n    paths:\n      - plans\n"), 0o644)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	// Initial scan (empty)
	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir})
	scanCmd.SetOut(&bytes.Buffer{})
	scanCmd.Execute()

	// Commit a new plan
	os.WriteFile(filepath.Join(planDir, "new.md"), []byte("# Newer\n"), 0o644)
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		cmd.CombinedOutput()
	}
	run("add", ".")
	run("commit", "-m", "new plan")

	// List with --no-refresh should NOT trigger auto-scan
	listCmd := NewListCmd()
	listCmd.SetArgs([]string{"--no-refresh"})
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	listCmd.SetOut(outBuf)
	listCmd.SetErr(errBuf)
	if err := listCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(errBuf.String(), "Index updated") {
		t.Errorf("--no-refresh should skip auto-scan, got: %s", errBuf.String())
	}
	if strings.Contains(outBuf.String(), "Newer") {
		t.Error("--no-refresh should not pick up new artifact")
	}
}

func TestDefaultPaths_IncludesCursorPlans(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	cursorDir := filepath.Join(dir, ".cursor", "plans")
	os.MkdirAll(cursorDir, 0o755)
	os.WriteFile(filepath.Join(cursorDir, "cursor-plan.md"), []byte("# Cursor Plan\n\nDetails here.\n"), 0o644)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir})
	outBuf := &bytes.Buffer{}
	scanCmd.SetOut(outBuf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	listCmd := NewListCmd()
	listCmd.SetArgs([]string{"--no-refresh"})
	listBuf := &bytes.Buffer{}
	listCmd.SetOut(listBuf)
	if err := listCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(listBuf.String(), "Cursor Plan") {
		t.Errorf("expected .cursor/plans/ to be discovered, output: %s", listBuf.String())
	}
}

func TestRootGlobs_SpecAndPlan(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	os.WriteFile(filepath.Join(dir, "v0.spec.md"), []byte("# V0 Spec\n\nVersion zero spec.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "roadmap.plan.md"), []byte("# Roadmap Plan\n\nRoadmap content.\n"), 0o644)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir})
	scanCmd.SetOut(&bytes.Buffer{})
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	listCmd := NewListCmd()
	listCmd.SetArgs([]string{"--no-refresh"})
	listBuf := &bytes.Buffer{}
	listCmd.SetOut(listBuf)
	if err := listCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := listBuf.String()
	if !strings.Contains(output, "V0 Spec") {
		t.Errorf("expected *.spec.md to be discovered, output: %s", output)
	}
	if !strings.Contains(output, "Roadmap Plan") {
		t.Errorf("expected *.plan.md to be discovered, output: %s", output)
	}
}

func TestHookInstall(t *testing.T) {
	dir := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	initCmd := NewInitCmd()
	initCmd.SetArgs([]string{"--hooks"})
	outBuf := &bytes.Buffer{}
	initCmd.SetOut(outBuf)
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	hookPath := filepath.Join(dir, ".git", "hooks", "post-commit")
	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("post-commit hook not created: %v", err)
	}

	if !strings.Contains(string(content), "scan --quiet --if-changed") {
		t.Errorf("hook content missing expected command: %s", string(content))
	}
	if !strings.Contains(string(content), "DevSpecs auto-index") {
		t.Errorf("hook missing marker: %s", string(content))
	}
}

func TestHookInstall_Idempotent(t *testing.T) {
	dir := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	// Install hook twice
	for i := 0; i < 2; i++ {
		initCmd := NewInitCmd()
		initCmd.SetArgs([]string{"--hooks", "--force"})
		initCmd.SetOut(&bytes.Buffer{})
		initCmd.Execute()
	}

	hookPath := filepath.Join(dir, ".git", "hooks", "post-commit")
	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(string(content), "DevSpecs auto-index") != 1 {
		t.Errorf("hook marker should appear exactly once, got:\n%s", string(content))
	}
}

func TestScanIfChanged_SkipsUnrelated(t *testing.T) {
	dir := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	// Create a non-spec file and commit it
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\n"), 0o644)
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		cmd.CombinedOutput()
	}
	run("add", ".")
	run("commit", "-m", "add app")

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir, "--if-changed"})
	outBuf := &bytes.Buffer{}
	scanCmd.SetOut(outBuf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// With --if-changed, since app.go is not in source paths, scan should be skipped (no output)
	if outBuf.String() != "" {
		t.Errorf("expected no output when --if-changed with unrelated file, got: %s", outBuf.String())
	}
}

func TestScanQuiet(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	planDir := filepath.Join(dir, "plans")
	os.MkdirAll(planDir, 0o755)
	os.WriteFile(filepath.Join(planDir, "test.md"), []byte("# Test\n"), 0o644)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir, "--quiet"})
	outBuf := &bytes.Buffer{}
	scanCmd.SetOut(outBuf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if outBuf.String() != "" {
		t.Errorf("--quiet should suppress output, got: %s", outBuf.String())
	}
}

func TestAutoScan_WorksFromSubdirectory(t *testing.T) {
	dir := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	planDir := filepath.Join(dir, "plans")
	os.MkdirAll(planDir, 0o755)
	os.WriteFile(filepath.Join(planDir, "plan.md"), []byte("# Initial Plan\n"), 0o644)
	cfgDir := filepath.Join(dir, ".devspecs")
	os.MkdirAll(cfgDir, 0o755)
	os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("version: 1\nsources:\n  - type: markdown\n    paths:\n      - plans\n"), 0o644)

	oldWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	// Initial scan from root
	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir})
	scanCmd.SetOut(&bytes.Buffer{})
	scanCmd.Execute()

	// Now commit a new plan
	os.WriteFile(filepath.Join(planDir, "subdir-plan.md"), []byte("# Subdir Plan\n"), 0o644)
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		cmd.CombinedOutput()
	}
	run("add", ".")
	run("commit", "-m", "add subdir plan")

	// cd into a SUBDIRECTORY — this is the key part of the test
	subDir := filepath.Join(dir, "plans")
	os.Chdir(subDir)

	// ds list should still detect staleness and auto-scan
	listCmd := NewListCmd()
	listCmd.SetArgs([]string{})
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	listCmd.SetOut(outBuf)
	listCmd.SetErr(errBuf)
	if err := listCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(outBuf.String(), "Subdir Plan") {
		t.Errorf("auto-scan from subdirectory didn't discover new plan.\nOutput: %s\nStderr: %s", outBuf.String(), errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "Index updated") {
		t.Errorf("expected 'Index updated' from subdirectory auto-scan, got stderr: %s", errBuf.String())
	}
}

func TestSchemaMigration_V2(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Verify the new columns exist
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 2").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected migration v2 to be applied, got count=%d", count)
	}

	// Verify columns work
	now := "2024-01-01T00:00:00Z"
	_, err = db.Exec("INSERT INTO repos (id, root_path, last_scan_commit, last_scan_at, created_at, updated_at) VALUES ('test', '/tmp', 'abc123', ?, ?, ?)", now, now, now)
	if err != nil {
		t.Fatalf("failed to insert with new columns: %v", err)
	}

	meta := db.GetRepoByRoot("/tmp")
	if meta == nil {
		t.Fatal("expected to find repo")
	}
	if meta.LastScanCommit != "abc123" {
		t.Errorf("expected last_scan_commit=abc123, got %s", meta.LastScanCommit)
	}
	if meta.LastScanAt != now {
		t.Errorf("expected last_scan_at=%s, got %s", now, meta.LastScanAt)
	}
}
