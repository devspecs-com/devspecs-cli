package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestScanHuman_OutputUsesDisplayLabels(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()

	scanCmd := NewScanCmd()
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Indexed by source:") {
		t.Fatalf("missing Indexed by source header:\n%s", out)
	}
	if !strings.Contains(out, "Planning docs") {
		t.Errorf("expected Planning docs label, got:\n%s", out)
	}
	if !strings.Contains(out, "OpenSpec") {
		t.Errorf("expected OpenSpec label, got:\n%s", out)
	}
	if !strings.Contains(out, "ADRs") {
		t.Errorf("expected ADRs label, got:\n%s", out)
	}
	if strings.Contains(out, "\nFound:") {
		t.Errorf("old Found block should be removed, got:\n%s", out)
	}
}

func TestScanJSON_ConsecutiveRunsIdentical(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()

	// Establish index so JSON snapshots are both "steady state" (same New/Updated/Unchanged).
	scanWarmup := NewScanCmd()
	scanWarmup.SetOut(&bytes.Buffer{})
	if err := scanWarmup.Execute(); err != nil {
		t.Fatal(err)
	}

	first := runScanJSONBytes(t)
	second := runScanJSONBytes(t)
	if string(first) != string(second) {
		t.Errorf("consecutive --json scans differ:\n%s\n---\n%s", first, second)
	}
}

func runScanJSONBytes(t *testing.T) []byte {
	t.Helper()
	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, buf.Bytes()); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	return compact.Bytes()
}

func TestScan_QuietWithJSON_WritesJSONSuppressesHuman(t *testing.T) {
	setupE2ERepo(t)
	NewInitCmd().Execute()
	warm := NewScanCmd()
	warm.SetOut(&bytes.Buffer{})
	if err := warm.Execute(); err != nil {
		t.Fatal(err)
	}

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json", "--quiet"})
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"Found"`) {
		t.Fatalf("expected JSON with Found, got: %q", out)
	}
	if strings.Contains(out, "Indexed by source") {
		t.Fatalf("human summary should be suppressed with --quiet, got: %q", out)
	}
}

func TestScanJSONProgressUsesStderrAndReportsTraversalDiagnostics(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".devspecs"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\nsources:\n  - type: markdown\n    paths:\n      - docs/plans\n"
	if err := os.WriteFile(filepath.Join(repoDir, ".devspecs", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < scanProgressInventoryThreshold+5; i++ {
		path := filepath.Join(repoDir, "docs", "plans", "plan-"+strconv.Itoa(i)+".md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("# Plan\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "node_modules", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "node_modules", "pkg", "ignored.md"), []byte("# Ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".git", "objects", "ignored"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json", "--path", repoDir})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	scanCmd.SetOut(stdout)
	scanCmd.SetErr(stderr)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "Scan progress: discovered") {
		t.Fatalf("expected progress on stderr, got: %s", stderr.String())
	}
	if strings.Contains(stdout.String(), "Scan progress") {
		t.Fatalf("progress leaked into JSON stdout:\n%s", stdout.String())
	}
	var out struct {
		Found     map[string]int `json:"Found"`
		Traversal *struct {
			InventoryFiles  int            `json:"inventory_files"`
			SkippedDirs     int            `json:"skipped_dirs"`
			SkippedByReason map[string]int `json:"skipped_by_reason"`
			TopSkippedDirs  []struct {
				Path   string `json:"path"`
				Reason string `json:"reason"`
			} `json:"top_skipped_dirs"`
		} `json:"traversal"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("scan --json stdout should be valid JSON: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if out.Found["markdown"] != scanProgressInventoryThreshold+5 {
		t.Fatalf("markdown found = %d, want %d", out.Found["markdown"], scanProgressInventoryThreshold+5)
	}
	if out.Traversal == nil || out.Traversal.InventoryFiles < scanProgressInventoryThreshold || out.Traversal.SkippedByReason["generated_vendor_or_build"] < 2 {
		t.Fatalf("missing traversal diagnostics: %#v", out.Traversal)
	}
}

func TestScanTraversalErrorNamesRootAndNarrowingAction(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	missingRoot := filepath.Join(tmp, "missing")

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", missingRoot})
	if err := scanCmd.Execute(); err == nil {
		t.Fatal("expected scan error for missing root")
	} else {
		msg := err.Error()
		if !strings.Contains(msg, missingRoot) {
			t.Fatalf("expected error to name scanned root, got: %s", msg)
		}
		if !strings.Contains(msg, "focused project root") || !strings.Contains(msg, "--path <repo-dir>") {
			t.Fatalf("expected narrowing guidance, got: %s", msg)
		}
	}
}

func TestScanIncludeTestsIndexesTestUnits(t *testing.T) {
	repoDir := setupE2ERepo(t)
	testDir := filepath.Join(repoDir, "tests")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testDir, "webhook_test.go"), []byte("package tests\n\nfunc TestWebhookReplayProtection(t *testing.T) {\n\trequire.NoError(t, err)\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	NewInitCmd().Execute()

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json", "--include-tests"})
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var out struct {
		Found            map[string]int `json:"Found"`
		SourcesBreakdown []struct {
			SourceType string `json:"source_type"`
			Count      int    `json:"count"`
		} `json:"sources_breakdown"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Found["test_case"] != 1 {
		t.Fatalf("test_case found = %d, want 1: %s", out.Found["test_case"], buf.String())
	}
	var sawBreakdown bool
	for _, row := range out.SourcesBreakdown {
		if row.SourceType == "test_case" {
			sawBreakdown = true
			if row.Count != 1 {
				t.Fatalf("test_case breakdown count = %d, want 1", row.Count)
			}
		}
	}
	if !sawBreakdown {
		t.Fatalf("missing test_case source breakdown: %#v", out.SourcesBreakdown)
	}
}

func TestScanNoGitignoreIncludesIgnoredConfiguredPaths(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".devspecs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "ignored-plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte("ignored-plans/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".devspecs", "config.yaml"), []byte("version: 1\nsources:\n  - type: markdown\n    paths:\n      - ignored-plans\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "ignored-plans", "one-off-runbook.md"), []byte("# One-off Runbook\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	ignoredCmd := NewScanCmd()
	ignoredCmd.SetArgs([]string{"--json"})
	ignoredBuf := &bytes.Buffer{}
	ignoredCmd.SetOut(ignoredBuf)
	if err := ignoredCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var ignoredOut struct {
		Found map[string]int `json:"Found"`
	}
	if err := json.Unmarshal(ignoredBuf.Bytes(), &ignoredOut); err != nil {
		t.Fatal(err)
	}
	if ignoredOut.Found["markdown"] != 0 {
		t.Fatalf("expected gitignored markdown path to be skipped, got: %s", ignoredBuf.String())
	}

	includeCmd := NewScanCmd()
	includeCmd.SetArgs([]string{"--json", "--no-gitignore"})
	includeBuf := &bytes.Buffer{}
	includeCmd.SetOut(includeBuf)
	if err := includeCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var includeOut struct {
		Found map[string]int `json:"Found"`
	}
	if err := json.Unmarshal(includeBuf.Bytes(), &includeOut); err != nil {
		t.Fatal(err)
	}
	if includeOut.Found["markdown"] != 1 {
		t.Fatalf("expected --no-gitignore to include ignored configured path, got: %s", includeBuf.String())
	}
}

func TestScanIncludeCodeCommentsIndexesIntentComments(t *testing.T) {
	repoDir := setupE2ERepo(t)
	srcDir := filepath.Join(repoDir, "services", "billing")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "webhook.go"), []byte("package billing\n\n// Invariant: stripe_event_id must always be checked before applying credits.\nfunc applyCredit() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	NewInitCmd().Execute()

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json", "--include-code-comments"})
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var out struct {
		Found            map[string]int `json:"Found"`
		SourcesBreakdown []struct {
			SourceType string `json:"source_type"`
			Count      int    `json:"count"`
		} `json:"sources_breakdown"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Found["code_comment"] != 1 {
		t.Fatalf("code_comment found = %d, want 1: %s", out.Found["code_comment"], buf.String())
	}
	var sawBreakdown bool
	for _, row := range out.SourcesBreakdown {
		if row.SourceType == "code_comment" {
			sawBreakdown = true
			if row.Count != 1 {
				t.Fatalf("code_comment breakdown count = %d, want 1", row.Count)
			}
		}
	}
	if !sawBreakdown {
		t.Fatalf("missing code_comment source breakdown: %#v", out.SourcesBreakdown)
	}
}

func TestLiveScanRunOptions_FreshIndexForEmptyOrUnindexedRepo(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	opts, err := liveScanRunOptions(db, "/tmp/repo")
	if err != nil {
		t.Fatal(err)
	}
	if !opts.UseTransaction {
		t.Fatal("live scan should use a transaction")
	}
	if !opts.FreshIndex {
		t.Fatal("empty live index should use the fresh-index writer")
	}
	if !opts.SkipAuthoredAtLookup {
		t.Fatal("fresh live index should skip per-artifact authored_at lookup")
	}

	now := "2026-05-24T00:00:00Z"
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp/repo', ?, ?)", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO artifacts
		(id, repo_id, short_id, kind, subtype, title, status, current_revision_id, created_at, updated_at, last_observed_at, authored_at)
		VALUES ('ds_EXISTING', 'r1', 'existing', 'plan', '', 'Existing', 'draft', '', ?, ?, ?, ?)`,
		now, now, now, now); err != nil {
		t.Fatal(err)
	}

	opts, err = liveScanRunOptions(db, "/tmp/repo")
	if err != nil {
		t.Fatal(err)
	}
	if !opts.UseTransaction {
		t.Fatal("populated live scan should still use a transaction")
	}
	if opts.FreshIndex {
		t.Fatal("populated live index must not use fresh-index writer")
	}
	if opts.SkipAuthoredAtLookup {
		t.Fatal("populated live index should keep canonical authored_at lookup behavior")
	}

	opts, err = liveScanRunOptions(db, "/tmp/second-repo")
	if err != nil {
		t.Fatal(err)
	}
	if !opts.UseTransaction {
		t.Fatal("new repo append should still use a transaction")
	}
	if !opts.FreshIndex {
		t.Fatal("unindexed target repo should use fresh-index writer even when another repo is indexed")
	}
	if !opts.SkipAuthoredAtLookup {
		t.Fatal("fresh repo append should skip per-artifact authored_at lookup")
	}
}

func TestLooksLikeTestArtifactPath(t *testing.T) {
	for _, path := range []string{
		"pkg/webhook_test.go",
		"tests/test_billing.py",
		"src/__tests__/billing.spec.ts",
		"spec/billing_spec.rb",
		"tests/BillingTest.php",
	} {
		if !looksLikeTestArtifactPath(path) {
			t.Fatalf("%s should be treated as a test artifact path", path)
		}
	}
	if looksLikeTestArtifactPath("docs/plans/billing.md") {
		t.Fatal("plan markdown should not be treated as a test artifact path")
	}
}
