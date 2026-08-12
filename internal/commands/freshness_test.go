package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{
			out, err := cmd.CombinedOutput()
			require.NoError(t, err,
				"git %v: %v\n%s", args, err, out)
		}

	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
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
	oldWd := testWorkingDirectory(t)
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir})
	scanBuf := &bytes.Buffer{}
	scanCmd.SetOut(scanBuf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
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
	{
		err := listCmd.Execute()
		require.NoError(t, err)
	}
	assert.Contains(t, // Verify the new artifact appears in output
		outBuf.String(), "New Plan",
		"auto-scan didn't pick up new plan.\nOutput: %s\nStderr: %s", outBuf.String(), errBuf.String())
	assert.Contains(t, errBuf.String(), "Index updated",
		"expected 'Index updated' message on stderr, got: %s", errBuf.String())

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

	oldWd := testWorkingDirectory(t)
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	// Scan
	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir})
	scanCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, scanCmd.Execute())

	// List — should NOT show "Index updated"
	listCmd := NewListCmd()
	listCmd.SetArgs([]string{})
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	listCmd.SetOut(outBuf)
	listCmd.SetErr(errBuf)
	{
		err := listCmd.Execute()
		require.NoError(t, err)
	}
	assert.NotContains(t, errBuf.String(), "Index updated",
		"unexpected 'Index updated' message when fresh: %s", errBuf.String())

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

	oldWd := testWorkingDirectory(t)
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	// Initial scan (empty)
	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir})
	scanCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, scanCmd.Execute())

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
	{
		err := listCmd.Execute()
		require.NoError(t, err)
	}
	assert.NotContains(t, errBuf.String(), "Index updated",
		"--no-refresh should skip auto-scan, got: %s", errBuf.String())
	assert.NotContains(t, outBuf.String(), "Newer",
		"--no-refresh should not pick up new artifact")

}

func TestDefaultPaths_IncludesCursorPlans(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	cursorDir := filepath.Join(dir, ".cursor", "plans")
	os.MkdirAll(cursorDir, 0o755)
	os.WriteFile(filepath.Join(cursorDir, "cursor-plan.md"), []byte("# Cursor Plan\n\nDetails here.\n"), 0o644)

	oldWd := testWorkingDirectory(t)
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir})
	outBuf := &bytes.Buffer{}
	scanCmd.SetOut(outBuf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}

	listCmd := NewListCmd()
	listCmd.SetArgs([]string{"--no-refresh"})
	listBuf := &bytes.Buffer{}
	listCmd.SetOut(listBuf)
	{
		err := listCmd.Execute()
		require.NoError(t, err)
	}
	assert.Contains(t, listBuf.String(), "Cursor Plan",
		"expected .cursor/plans/ to be discovered, output: %s", listBuf.String())

}

func TestRootGlobs_SpecAndPlan(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	os.WriteFile(filepath.Join(dir, "v0.spec.md"), []byte("# V0 Spec\n\nVersion zero spec.\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "roadmap.plan.md"), []byte("# Roadmap Plan\n\nRoadmap content.\n"), 0o644)

	oldWd := testWorkingDirectory(t)
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir})
	scanCmd.SetOut(&bytes.Buffer{})
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}

	listCmd := NewListCmd()
	listCmd.SetArgs([]string{"--no-refresh"})
	listBuf := &bytes.Buffer{}
	listCmd.SetOut(listBuf)
	{
		err := listCmd.Execute()
		require.NoError(t, err)
	}

	output := listBuf.String()
	assert.Contains(t, output, "V0 Spec",
		"expected *.spec.md to be discovered, output: %s", output)
	assert.Contains(t, output, "Roadmap Plan",
		"expected *.plan.md to be discovered, output: %s", output)

}

func TestHookInstall(t *testing.T) {
	dir := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	oldWd := testWorkingDirectory(t)
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	initCmd := NewInitCmd()
	initCmd.SetArgs([]string{"--hooks"})
	outBuf := &bytes.Buffer{}
	initCmd.SetOut(outBuf)
	{
		err := initCmd.Execute()
		require.NoError(t, err)
	}

	hookPath := filepath.Join(dir, ".git", "hooks", "post-commit")
	content, err := os.ReadFile(hookPath)
	require.NoError(t, err,
		"post-commit hook not created: %v", err)
	assert.Contains(t, string(content), "scan --quiet --if-changed",
		"hook content missing expected command: %s", string(content))
	assert.Contains(t, string(content), "DevSpecs auto-index",
		"hook missing marker: %s", string(content))

}

func TestHookInstall_Idempotent(t *testing.T) {
	dir := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	oldWd := testWorkingDirectory(t)
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	hookPath := filepath.Join(dir, ".git", "hooks", "post-commit")
	require.NoError(t, os.MkdirAll(filepath.Dir(hookPath), 0o755))
	require.NoError(t, os.WriteFile(hookPath, []byte(hookScriptContent()), 0o755))
	initCmd := NewInitCmd()
	initCmd.SetArgs([]string{"--hooks", "--force"})
	initCmd.SetOut(&bytes.Buffer{})

	err := initCmd.Execute()

	require.NoError(t, err)
	content, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(content), "DevSpecs auto-index"),
		"hook marker should appear exactly once, got:\n%s", string(content))

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

	oldWd := testWorkingDirectory(t)
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir, "--if-changed"})
	outBuf := &bytes.Buffer{}
	scanCmd.SetOut(outBuf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}
	assert.Equal(t, // With --if-changed, since app.go is not in source paths, scan should be skipped (no output)
		"", outBuf.String(),
		"expected no output when --if-changed with unrelated file, got: %s", outBuf.String())

}

func TestScanQuiet(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	planDir := filepath.Join(dir, "plans")
	os.MkdirAll(planDir, 0o755)
	os.WriteFile(filepath.Join(planDir, "test.md"), []byte("# Test\n"), 0o644)

	oldWd := testWorkingDirectory(t)
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir, "--quiet"})
	outBuf := &bytes.Buffer{}
	scanCmd.SetOut(outBuf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}
	assert.Equal(t, "", outBuf.String(),
		"--quiet should suppress output, got: %s", outBuf.String())

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

	oldWd := testWorkingDirectory(t)
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	// Initial scan from root
	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", dir})
	scanCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, scanCmd.Execute())

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
	{
		err := listCmd.Execute()
		require.NoError(t, err)
	}
	assert.Contains(t, outBuf.String(), "Subdir Plan",
		"auto-scan from subdirectory didn't discover new plan.\nOutput: %s\nStderr: %s", outBuf.String(), errBuf.String())
	assert.Contains(t, errBuf.String(), "Index updated",
		"expected 'Index updated' from subdirectory auto-scan, got stderr: %s", errBuf.String())

}

func TestFindAutoScan_TriggersWhenIndexMissing(t *testing.T) {
	dir := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	planDir := filepath.Join(dir, "plans")
	os.MkdirAll(planDir, 0o755)
	os.WriteFile(filepath.Join(planDir, "credentials-plan.md"), []byte("# Credentials Rotation\n\nRotate credentials for webhook ingestion.\n"), 0o644)
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
	run("commit", "-m", "add credentials plan")

	oldWd := testWorkingDirectory(t)
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	findCmd := NewFindCmd()
	findCmd.SetArgs([]string{"credentials rotation"})
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	findCmd.SetOut(outBuf)
	findCmd.SetErr(errBuf)
	{
		err := findCmd.Execute()
		require.NoError(t, err)
	}
	assert.Contains(t, errBuf.String(), "Index updated",
		"expected missing-index find to auto-scan, stderr: %s", errBuf.String())

	output := outBuf.String()
	assert.Contains(t, output, "Working set: credentials rotation", "find output missing auto-scanned plan.\nOutput: %s\nStderr: %s", output, errBuf.String())
	assert.Contains(t, output, "Credentials Rotation", "find output missing auto-scanned plan.\nOutput: %s\nStderr: %s", output, errBuf.String())

}

func TestFindJSONAutoScanKeepsResultStreamsClean(t *testing.T) {
	dir := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	planDir := filepath.Join(dir, "plans")
	os.MkdirAll(planDir, 0o755)
	os.WriteFile(filepath.Join(planDir, "credentials-plan.md"), []byte("# Credentials Rotation\n\nRotate credentials for webhook ingestion.\n"), 0o644)
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
	run("commit", "-m", "add credentials plan")

	oldWd := testWorkingDirectory(t)
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	findCmd := NewFindCmd()
	findCmd.SetArgs([]string{"credentials rotation", "--json"})
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	findCmd.SetOut(outBuf)
	findCmd.SetErr(errBuf)
	{
		err := findCmd.Execute()
		require.NoError(t, err)
	}
	assert.Equal(t, 0, errBuf.Len(),
		"find --json should suppress auto-scan stderr, got: %s", errBuf.String())

	var payload map[string]any
	{
		err := json.Unmarshal(outBuf.Bytes(), &payload)
		require.NoError(t, err,
			"find --json stdout should remain valid JSON: %v\nstdout=%s\nstderr=%s", err, outBuf.String(), errBuf.String())
	}
	assert.Contains(t, outBuf.String(), "Credentials Rotation",
		"find --json output missing auto-scanned plan.\nOutput: %s", outBuf.String())

}

func TestFindAutoScan_SkippedWithNoRefreshWhenIndexMissing(t *testing.T) {
	dir := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	planDir := filepath.Join(dir, "plans")
	os.MkdirAll(planDir, 0o755)
	os.WriteFile(filepath.Join(planDir, "credentials-plan.md"), []byte("# Credentials Rotation\n\nRotate credentials for webhook ingestion.\n"), 0o644)
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
	run("commit", "-m", "add credentials plan")

	oldWd := testWorkingDirectory(t)
	os.Chdir(dir)
	defer os.Chdir(oldWd)

	findCmd := NewFindCmd()
	findCmd.SetArgs([]string{"credentials rotation", "--no-refresh"})
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	findCmd.SetOut(outBuf)
	findCmd.SetErr(errBuf)
	{
		err := findCmd.Execute()
		require.NoError(t, err)
	}
	assert.NotContains(t, errBuf.String(), "Index updated",
		"--no-refresh should skip missing-index auto-scan, stderr: %s", errBuf.String())

	output := outBuf.String()
	assert.Contains(t, output, "No matching artifacts found.", "--no-refresh should not discover unindexed plan.\nOutput: %s\nStderr: %s", output, errBuf.String())
	assert.NotContains(t, output, "Credentials Rotation", "--no-refresh should not discover unindexed plan.\nOutput: %s\nStderr: %s", output, errBuf.String())

}

func TestSchemaVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

	defer db.Close()

	var version int
	err = db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	require.NoError(t, err)
	assert.Equal(t, store.SchemaVersion, version,
		"expected schema version %d, got %d", store.SchemaVersion, version)

	// Verify v0.1 columns exist
	now := "2024-01-01T00:00:00Z"
	_, err = db.Exec("INSERT INTO repos (id, root_path, last_scan_commit, last_scan_at, scanned_by, created_at, updated_at) VALUES ('test', '/tmp', 'abc123', ?, 'testuser', ?, ?)", now, now, now)
	require.NoError(t, err,
		"failed to insert with v0.1 columns: %v", err)

	meta := db.GetRepoByRoot("/tmp")
	require.NotNil(t, meta,
		"expected to find repo")
	assert.Equal(t, "abc123", meta.LastScanCommit,
		"expected last_scan_commit=abc123, got %s", meta.LastScanCommit)
	assert.Equal(t, "testuser", meta.ScannedBy,
		"expected scanned_by=testuser, got %s", meta.ScannedBy)

	// Verify artifact_tags table exists by inserting with a valid artifact_id
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r2', '/tags', ?, ?)", now, now)
	db.Exec("INSERT INTO artifacts (id, repo_id, kind, title, status, created_at, updated_at, last_observed_at, authored_at) VALUES ('ds_TAG', 'r2', 'plan', 'Tag Test', 'draft', ?, ?, ?, ?)", now, now, now, now)
	_, err = db.Exec("INSERT INTO artifact_tags (artifact_id, tag, source, created_at) VALUES ('ds_TAG', 'test', 'manual', ?)", now)
	require.NoError(t, err,
		"artifact_tags table missing or broken: %v", err)

}
