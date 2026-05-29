package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestLiveScanRunOptions_FreshIndexOnlyForEmptyIndex(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	opts, err := liveScanRunOptions(db)
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

	opts, err = liveScanRunOptions(db)
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
