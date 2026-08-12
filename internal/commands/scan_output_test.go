package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	scanpkg "github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestScanHuman_OutputUsesDisplayLabels(t *testing.T) {
	setupE2ERepo(t)
	require.NoError(t, NewInitCmd().Execute())

	scanCmd := NewScanCmd()
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Contains(t, out, "Indexed by source:",
		"missing Indexed by source header:\n%s", out)
	assert.Contains(t, out, "Planning docs",
		"expected Planning docs label, got:\n%s", out)
	assert.Contains(t, out, "OpenSpec",
		"expected OpenSpec label, got:\n%s", out)
	assert.Contains(t, out, "ADRs",
		"expected ADRs label, got:\n%s", out)
	assert.NotContains(t, out, "\nFound:",
		"old Found block should be removed, got:\n%s", out)

}

func TestScanJSON_ConsecutiveRunsIdentical(t *testing.T) {
	setupE2ERepo(t)
	require.NoError(t, NewInitCmd().Execute())

	// Establish index so JSON snapshots are both "steady state" (same New/Updated/Unchanged).
	scanWarmup := NewScanCmd()
	scanWarmup.SetOut(&bytes.Buffer{})
	{
		err := scanWarmup.Execute()
		require.NoError(t, err)
	}

	first := runScanJSONBytes(t)
	second := runScanJSONBytes(t)
	assert.Equal(t, string(second), string(first),
		"consecutive --json scans differ:\n%s\n---\n%s", first, second)

}

func TestScanJSON_PhaseTimingIsOptIn(t *testing.T) {
	setupE2ERepo(t)
	require.NoError(t, NewInitCmd().Execute())

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}

	var out map[string]json.RawMessage
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err)
	}
	{

		_, ok := out["phase_timing"]
		assert.False(t, ok,
			"default scan JSON should not include phase_timing: %s", buf.String())
	}

}

func TestScanJSON_PhaseTimingIncludesSourceManifestBreakdown(t *testing.T) {
	repoDir := setupE2ERepo(t)
	srcDir := filepath.Join(repoDir, "internal", "auth")
	{
		err := os.MkdirAll(srcDir, 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(srcDir, "service.go"), []byte("package auth\n\nfunc LoginUser() bool { return true }\n"), 0o644)
		require.NoError(t, err)
	}

	require.NoError(t, NewInitCmd().Execute())

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json", "--quiet", "--phase-timing", "--experimental-source-manifest"})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	scanCmd.SetOut(stdout)
	scanCmd.SetErr(stderr)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}
	assert.NotContains(t, stderr.String(), "Scan progress",
		"quiet phase-timing scan should not emit progress: %s", stderr.String())

	var out struct {
		PhaseTiming *struct {
			Enabled bool `json:"enabled"`
			Phases  []struct {
				Name   string         `json:"name"`
				Counts map[string]int `json:"counts"`
			} `json:"phases"`
		} `json:"phase_timing"`
		SourceManifest *struct {
			IndexedFiles int              `json:"indexed_files"`
			PhaseMS      map[string]int64 `json:"phase_ms"`
		} `json:"source_manifest"`
		EvidenceGraph *struct {
			ConceptsIndexed int              `json:"concepts_indexed"`
			PhaseMS         map[string]int64 `json:"phase_ms"`
		} `json:"evidence_graph"`
	}
	{
		err := json.Unmarshal(stdout.Bytes(), &out)
		require.NoError(t, err,
			"scan --phase-timing JSON invalid: %v\n%s", err, stdout.String())
	}
	require.NotNil(t, out.PhaseTiming, "missing phase timing diagnostics: %s", stdout.String())
	assert.True(t, out.PhaseTiming.Enabled, "missing phase timing diagnostics: %s", stdout.String())
	require.NotEmpty(t, out.PhaseTiming.Phases, "missing phase timing diagnostics: %s", stdout.String())
	assert.True(t, scanPhaseTimingHas(out.PhaseTiming.Phases, "shared_discovery"), "missing expected phase timing rows: %#v", out.PhaseTiming.Phases)
	assert.True(t, scanPhaseTimingHas(out.PhaseTiming.Phases, "source_manifest"), "missing expected phase timing rows: %#v", out.PhaseTiming.Phases)
	require.NotNil(t, out.SourceManifest, "missing source manifest phase breakdown: %#v", out.SourceManifest)
	assert.NotEqual(t, 0, out.SourceManifest.IndexedFiles, "missing source manifest phase breakdown: %#v", out.SourceManifest)
	require.NotEmpty(t, out.SourceManifest.PhaseMS, "missing source manifest phase breakdown: %#v", out.SourceManifest)
	require.NotNil(t, out.EvidenceGraph, "missing evidence graph phase breakdown: %#v", out.EvidenceGraph)
	assert.NotEqual(t, 0, out.EvidenceGraph.ConceptsIndexed, "missing evidence graph phase breakdown: %#v", out.EvidenceGraph)
	require.NotEmpty(t, out.EvidenceGraph.PhaseMS, "missing evidence graph phase breakdown: %#v", out.EvidenceGraph)
	{

		_, ok := out.EvidenceGraph.PhaseMS["persist_mentions"]
		assert.True(t, ok,
			"missing evidence graph persist mention timing: %#v", out.EvidenceGraph.PhaseMS)
	}

}

func scanPhaseTimingHas(phases []struct {
	Name   string         `json:"name"`
	Counts map[string]int `json:"counts"`
}, name string) bool {
	for _, phase := range phases {
		if phase.Name == name {
			return true
		}
	}
	return false
}

func runScanJSONBytes(t *testing.T) []byte {
	t.Helper()
	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}

	var compact bytes.Buffer
	{
		err := json.Compact(&compact, buf.Bytes())
		require.NoError(t, err,
			"invalid json: %v", err)
	}

	return compact.Bytes()
}

func TestScan_QuietWithJSON_WritesJSONSuppressesHuman(t *testing.T) {
	setupE2ERepo(t)
	require.NoError(t, NewInitCmd().Execute())
	warm := NewScanCmd()
	warm.SetOut(&bytes.Buffer{})
	{
		err := warm.Execute()
		require.NoError(t, err)
	}

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json", "--quiet"})
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Contains(t, out, `"Found"`,
		"expected JSON with Found, got: %q", out)
	assert.NotContains(t, out, "Indexed by source",
		"human summary should be suppressed with --quiet, got: %q", out)

}

func TestScanJSONProgressUsesStderrAndReportsTraversalDiagnostics(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	{
		err := os.MkdirAll(filepath.Join(repoDir, ".devspecs"), 0o755)
		require.NoError(t, err)
	}

	cfg := "version: 1\nsources:\n  - type: markdown\n    paths:\n      - docs/plans\n"
	{
		err := os.WriteFile(filepath.Join(repoDir, ".devspecs", "config.yaml"), []byte(cfg), 0o644)
		require.NoError(t, err)
	}

	for i := 0; i < scanProgressInventoryThreshold+5; i++ {
		path := filepath.Join(repoDir, "docs", "plans", "plan-"+strconv.Itoa(i)+".md")
		{
			err := os.MkdirAll(filepath.Dir(path), 0o755)
			require.NoError(t, err)
		}
		{

			err := os.WriteFile(path, []byte("# Plan\n"), 0o644)
			require.NoError(t, err)
		}

	}
	{
		err := os.MkdirAll(filepath.Join(repoDir, "node_modules", "pkg"), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(repoDir, "node_modules", "pkg", "ignored.md"), []byte("# Ignored\n"), 0o644)
		require.NoError(t, err)
	}
	{

		err := os.MkdirAll(filepath.Join(repoDir, ".git", "objects"), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(repoDir, ".git", "objects", "ignored"), []byte("ignored\n"), 0o644)
		require.NoError(t, err)
	}

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json", "--path", repoDir})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	scanCmd.SetOut(stdout)
	scanCmd.SetErr(stderr)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}
	assert.Contains(t, stderr.String(), "Scan progress: discovered",
		"expected progress on stderr, got: %s", stderr.String())
	assert.NotContains(t, stdout.String(), "Scan progress",
		"progress leaked into JSON stdout:\n%s", stdout.String())

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
	{
		err := json.Unmarshal(stdout.Bytes(), &out)
		require.NoError(t, err,
			"scan --json stdout should be valid JSON: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	assert.Equal(t, scanProgressInventoryThreshold+5, out.Found["markdown"],
		"markdown found = %d, want %d", out.Found["markdown"], scanProgressInventoryThreshold+5)
	require.NotNil(t, out.Traversal, "missing traversal diagnostics: %#v", out.Traversal)
	assert.GreaterOrEqual(t, out.Traversal.InventoryFiles, scanProgressInventoryThreshold, "missing traversal diagnostics: %#v", out.Traversal)
	assert.GreaterOrEqual(t, out.Traversal.SkippedByReason["generated_vendor_or_build"], 2, "missing traversal diagnostics: %#v", out.Traversal)

}

func TestScanProgressStderrReportsGranularColdIndexPhases(t *testing.T) {
	var stderr bytes.Buffer
	progress := scanProgressStderr(&stderr, "Auto-index", true)
	progress(scanpkg.ProgressEvent{Phase: "scan", Event: "start"})
	progress(scanpkg.ProgressEvent{
		Phase:           "shared_discovery",
		Event:           "inventory_done",
		InventoryFiles:  scanProgressInventoryThreshold,
		FilesTotal:      scanProgressInventoryThreshold,
		SkippedDirs:     1,
		SkippedByReason: map[string]int{"generated_vendor_or_build": 1},
	})
	progress(scanpkg.ProgressEvent{
		Phase:                "shared_discovery",
		Event:                "done",
		FilesTotal:           scanProgressInventoryThreshold,
		FilesScanned:         scanProgressInventoryThreshold,
		CandidatesDiscovered: map[string]int{"markdown": 5, "source_context": 2},
	})
	progress(scanpkg.ProgressEvent{Phase: "parse_upsert", Event: "adapter_start", CurrentAdapter: "source_context", CandidatesTotal: 2})
	progress(scanpkg.ProgressEvent{Phase: "extract", Event: "adapter_done", CurrentAdapter: "source_context", CandidatesTotal: 2, CandidatesProcessed: 2})
	progress(scanpkg.ProgressEvent{Phase: "persist", Event: "adapter_done", CurrentAdapter: "source_context", CandidatesTotal: 2, CandidatesProcessed: 2})
	progress(scanpkg.ProgressEvent{Phase: "fresh_index_writer", Event: "rows_flushed", RowsWritten: map[string]int{"artifacts": 7, "sources": 7}})
	progress(scanpkg.ProgressEvent{Phase: "evidence_graph", Event: "start"})
	progress(scanpkg.ProgressEvent{Phase: "evidence_graph", Event: "done", RowsWritten: map[string]int{"concepts": 3, "mentions": 4, "edges": 5}})
	progress(scanpkg.ProgressEvent{Phase: "source_manifest", Event: "start"})
	progress(scanpkg.ProgressEvent{Phase: "source_manifest", Event: "done", FilesTotal: 12, FilesScanned: 8, RowsWritten: map[string]int{"files": 8, "symbols": 10}})
	progress(scanpkg.ProgressEvent{Phase: "fresh_index_fts", Event: "start", DeferredFTSRows: 7})
	progress(scanpkg.ProgressEvent{Phase: "fresh_index_fts", Event: "done", RowsWritten: map[string]int{"artifacts_fts": 7}})
	progress(scanpkg.ProgressEvent{Phase: "scan", Event: "done"})

	out := stderr.String()
	assert.Contains(t, out, "Auto-index progress: preparing index substrate",
		"progress output missing %q:\n%s", "Auto-index progress: preparing index substrate", out)
	assert.Contains(t, out, "Auto-index progress: discovered 200 candidate file(s)",
		"progress output missing %q:\n%s", "Auto-index progress: discovered 200 candidate file(s)", out)
	assert.Contains(t, out, "Auto-index progress: discovery complete",
		"progress output missing %q:\n%s", "Auto-index progress: discovery complete", out)
	assert.Contains(t, out, "Auto-index progress: extracting source context",
		"progress output missing %q:\n%s", "Auto-index progress: extracting source context", out)
	assert.Contains(t, out, "Auto-index progress: extracted source context",
		"progress output missing %q:\n%s", "Auto-index progress: extracted source context", out)
	assert.Contains(t, out, "Auto-index progress: persisted source context",
		"progress output missing %q:\n%s", "Auto-index progress: persisted source context", out)
	assert.Contains(t, out, "Auto-index progress: wrote index rows",
		"progress output missing %q:\n%s", "Auto-index progress: wrote index rows", out)
	assert.Contains(t, out, "Auto-index progress: building evidence graph",
		"progress output missing %q:\n%s", "Auto-index progress: building evidence graph", out)
	assert.Contains(t, out, "Auto-index progress: evidence graph complete",
		"progress output missing %q:\n%s", "Auto-index progress: evidence graph complete", out)
	assert.Contains(t, out, "Auto-index progress: building source manifest",
		"progress output missing %q:\n%s", "Auto-index progress: building source manifest", out)
	assert.Contains(t, out, "Auto-index progress: source manifest complete",
		"progress output missing %q:\n%s", "Auto-index progress: source manifest complete", out)
	assert.Contains(t, out, "Auto-index progress: updating search index",
		"progress output missing %q:\n%s", "Auto-index progress: updating search index", out)
	assert.Contains(t, out, "Auto-index progress: search index complete",
		"progress output missing %q:\n%s", "Auto-index progress: search index complete", out)
	assert.Contains(t, out, "Auto-index progress: complete",
		"progress output missing %q:\n%s", "Auto-index progress: complete", out)

}

func TestScanProgressStderrDefaultReportsHighLevelColdIndexPhases(t *testing.T) {
	var stderr bytes.Buffer
	progress := scanProgressStderr(&stderr, "Auto-index")
	progress(scanpkg.ProgressEvent{Phase: "scan", Event: "start"})
	progress(scanpkg.ProgressEvent{
		Phase:           "shared_discovery",
		Event:           "inventory_done",
		InventoryFiles:  scanProgressInventoryThreshold,
		FilesTotal:      scanProgressInventoryThreshold,
		SkippedDirs:     1,
		SkippedByReason: map[string]int{"generated_vendor_or_build": 1},
	})
	progress(scanpkg.ProgressEvent{
		Phase:                "shared_discovery",
		Event:                "done",
		FilesTotal:           scanProgressInventoryThreshold,
		FilesScanned:         scanProgressInventoryThreshold,
		CandidatesDiscovered: map[string]int{"markdown": 5, "source_context": 2},
	})
	progress(scanpkg.ProgressEvent{Phase: "parse_upsert", Event: "adapter_start", CurrentAdapter: "source_context", CandidatesTotal: 2})
	progress(scanpkg.ProgressEvent{Phase: "extract", Event: "adapter_done", CurrentAdapter: "source_context", CandidatesTotal: 2, CandidatesProcessed: 2})
	progress(scanpkg.ProgressEvent{Phase: "persist", Event: "adapter_done", CurrentAdapter: "source_context", CandidatesTotal: 2, CandidatesProcessed: 2})
	progress(scanpkg.ProgressEvent{Phase: "fresh_index_writer", Event: "rows_flushed", RowsWritten: map[string]int{"artifacts": 7, "sources": 7}})
	progress(scanpkg.ProgressEvent{Phase: "evidence_graph", Event: "start"})
	progress(scanpkg.ProgressEvent{Phase: "evidence_graph", Event: "done", RowsWritten: map[string]int{"concepts": 3, "mentions": 4, "edges": 5}})
	progress(scanpkg.ProgressEvent{Phase: "source_manifest", Event: "start"})
	progress(scanpkg.ProgressEvent{Phase: "source_manifest", Event: "done", FilesTotal: 12, FilesScanned: 8, RowsWritten: map[string]int{"files": 8, "symbols": 10}})
	progress(scanpkg.ProgressEvent{Phase: "fresh_index_fts", Event: "start", DeferredFTSRows: 7})
	progress(scanpkg.ProgressEvent{Phase: "fresh_index_fts", Event: "done", RowsWritten: map[string]int{"artifacts_fts": 7}})
	progress(scanpkg.ProgressEvent{Phase: "scan", Event: "done"})

	out := stderr.String()
	assert.Contains(t, out, "Auto-index progress: preparing index substrate",
		"progress output missing %q:\n%s", "Auto-index progress: preparing index substrate", out)
	assert.Contains(t, out, "Auto-index progress: discovered 200 candidate file(s)",
		"progress output missing %q:\n%s", "Auto-index progress: discovered 200 candidate file(s)", out)
	assert.Contains(t, out, "Auto-index progress: discovery complete",
		"progress output missing %q:\n%s", "Auto-index progress: discovery complete", out)
	assert.Contains(t, out, "Auto-index progress: extracting and indexing artifacts",
		"progress output missing %q:\n%s", "Auto-index progress: extracting and indexing artifacts", out)
	assert.Contains(t, out, "Auto-index progress: index rows written",
		"progress output missing %q:\n%s", "Auto-index progress: index rows written", out)
	assert.Contains(t, out, "Auto-index progress: building evidence graph",
		"progress output missing %q:\n%s", "Auto-index progress: building evidence graph", out)
	assert.Contains(t, out, "Auto-index progress: evidence graph complete",
		"progress output missing %q:\n%s", "Auto-index progress: evidence graph complete", out)
	assert.Contains(t, out, "Auto-index progress: building source manifest",
		"progress output missing %q:\n%s", "Auto-index progress: building source manifest", out)
	assert.Contains(t, out, "Auto-index progress: source manifest complete",
		"progress output missing %q:\n%s", "Auto-index progress: source manifest complete", out)
	assert.Contains(t, out, "Auto-index progress: updating search index",
		"progress output missing %q:\n%s", "Auto-index progress: updating search index", out)
	assert.Contains(t, out, "Auto-index progress: search index complete",
		"progress output missing %q:\n%s", "Auto-index progress: search index complete", out)
	assert.Contains(t, out, "Auto-index progress: complete",
		"progress output missing %q:\n%s", "Auto-index progress: complete", out)

	assert.NotContains(t, out, "Auto-index progress: extracting source context",
		"default progress output should not include verbose detail %q:\n%s", "Auto-index progress: extracting source context", out)
	assert.NotContains(t, out, "Auto-index progress: extracted source context",
		"default progress output should not include verbose detail %q:\n%s", "Auto-index progress: extracted source context", out)
	assert.NotContains(t, out, "Auto-index progress: persisted source context",
		"default progress output should not include verbose detail %q:\n%s", "Auto-index progress: persisted source context", out)
	assert.NotContains(t, out, "Auto-index progress: wrote index rows (",
		"default progress output should not include verbose detail %q:\n%s", "Auto-index progress: wrote index rows (", out)
	assert.NotContains(t, out, "rows: files",
		"default progress output should not include verbose detail %q:\n%s", "rows: files", out)
	assert.NotContains(t, out, "deferred row",
		"default progress output should not include verbose detail %q:\n%s", "deferred row", out)

}

func TestScanTraversalErrorNamesRootAndNarrowingAction(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	missingRoot := filepath.Join(tmp, "missing")

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--path", missingRoot})

	err := scanCmd.Execute()

	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, missingRoot)
	assert.Contains(t, msg, "focused project root")
	assert.Contains(t, msg, "--path <repo-dir>")
}

func TestScanIncludeTestsIndexesTestUnits(t *testing.T) {
	repoDir := setupE2ERepo(t)
	testDir := filepath.Join(repoDir, "tests")
	{
		err := os.MkdirAll(testDir, 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(testDir, "webhook_test.go"), []byte("package tests\n\nfunc TestWebhookReplayProtection(t *testing.T) {\n\trequire.NoError(t, err)\n}\n"), 0o644)
		require.NoError(t, err)
	}

	require.NoError(t, NewInitCmd().Execute())

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json", "--include-tests"})
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}

	var out struct {
		Found            map[string]int `json:"Found"`
		SourcesBreakdown []struct {
			SourceType string `json:"source_type"`
			Count      int    `json:"count"`
		} `json:"sources_breakdown"`
	}
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, out.Found["test_case"],
		"test_case found = %d, want 1: %s", out.Found["test_case"], buf.String())

	var sawBreakdown bool
	for _, row := range out.SourcesBreakdown {
		if row.SourceType == "test_case" {
			sawBreakdown = true
			assert.Equal(t, 1, row.Count,
				"test_case breakdown count = %d, want 1", row.Count)

		}
	}
	assert.True(t, sawBreakdown,
		"missing test_case source breakdown: %#v", out.SourcesBreakdown)

}

func TestScan_WithGitignore_SkipsIgnoredConfiguredPaths(t *testing.T) {
	repoDir := setupScanIgnoredConfiguredPath(t)

	cmd := NewScanCmd()
	cmd.SetArgs([]string{"--path", repoDir, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out struct {
		Found map[string]int `json:"Found"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Zero(t, out.Found["markdown"])
}

func TestScan_WithNoGitignore_IncludesIgnoredConfiguredPaths(t *testing.T) {
	repoDir := setupScanIgnoredConfiguredPath(t)

	cmd := NewScanCmd()
	cmd.SetArgs([]string{"--path", repoDir, "--json", "--no-gitignore"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out struct {
		Found map[string]int `json:"Found"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, 1, out.Found["markdown"])
}

func setupScanIgnoredConfiguredPath(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".devspecs"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "ignored-plans"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte("ignored-plans/\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".devspecs", "config.yaml"), []byte("version: 1\nsources:\n  - type: markdown\n    paths:\n      - ignored-plans\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "ignored-plans", "one-off-runbook.md"), []byte("# One-off Runbook\n"), 0o644))

	return repoDir
}

func TestScanIncludeCodeCommentsIndexesIntentComments(t *testing.T) {
	repoDir := setupE2ERepo(t)
	srcDir := filepath.Join(repoDir, "services", "billing")
	{
		err := os.MkdirAll(srcDir, 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(srcDir, "webhook.go"), []byte("package billing\n\n// Invariant: stripe_event_id must always be checked before applying credits.\nfunc applyCredit() {}\n"), 0o644)
		require.NoError(t, err)
	}

	require.NoError(t, NewInitCmd().Execute())

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json", "--include-code-comments"})
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}

	var out struct {
		Found            map[string]int `json:"Found"`
		SourcesBreakdown []struct {
			SourceType string `json:"source_type"`
			Count      int    `json:"count"`
		} `json:"sources_breakdown"`
	}
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, out.Found["code_comment"],
		"code_comment found = %d, want 1: %s", out.Found["code_comment"], buf.String())

	var sawBreakdown bool
	for _, row := range out.SourcesBreakdown {
		if row.SourceType == "code_comment" {
			sawBreakdown = true
			assert.Equal(t, 1, row.Count,
				"code_comment breakdown count = %d, want 1", row.Count)

		}
	}
	assert.True(t, sawBreakdown,
		"missing code_comment source breakdown: %#v", out.SourcesBreakdown)

}

func TestLiveScanRunOptions_FreshIndexForEmptyOrUnindexedRepo(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()

	opts, err := liveScanRunOptions(db, "/tmp/repo")
	require.NoError(t, err)
	assert.True(t, opts.UseTransaction,
		"live scan should use a transaction")
	assert.True(t, opts.FreshIndex,
		"empty live index should use the fresh-index writer")
	assert.True(t, opts.SkipAuthoredAtLookup,
		"fresh live index should skip per-artifact authored_at lookup")

	now := "2026-05-24T00:00:00Z"
	{
		_, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('r1', '/tmp/repo', ?, ?)", now, now)
		require.NoError(t, err)
	}
	{

		_, err := db.Exec(`INSERT INTO artifacts
		(id, repo_id, short_id, kind, subtype, title, status, current_revision_id, created_at, updated_at, last_observed_at, authored_at)
		VALUES ('ds_EXISTING', 'r1', 'existing', 'plan', '', 'Existing', 'draft', '', ?, ?, ?, ?)`,
			now, now, now, now)
		require.NoError(t, err)
	}

	opts, err = liveScanRunOptions(db, "/tmp/repo")
	require.NoError(t, err)
	assert.True(t, opts.UseTransaction,
		"populated live scan should still use a transaction")
	assert.False(t, opts.FreshIndex,
		"populated live index must not use fresh-index writer")
	assert.False(t, opts.SkipAuthoredAtLookup,
		"populated live index should keep canonical authored_at lookup behavior")

	opts, err = liveScanRunOptions(db, "/tmp/second-repo")
	require.NoError(t, err)
	assert.True(t, opts.UseTransaction,
		"new repo append should still use a transaction")
	assert.True(t, opts.FreshIndex,
		"unindexed target repo should use fresh-index writer even when another repo is indexed")
	assert.True(t, opts.SkipAuthoredAtLookup,
		"fresh repo append should skip per-artifact authored_at lookup")

}

func TestScan_GitWorktreeReusesLogicalRepositoryIndex(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)
	t.Setenv("DEVSPECS_TELEMETRY", "0")
	mainRepo := filepath.Join(tmp, "main")
	worktree := filepath.Join(tmp, "worktree")
	runGitCommand(t, "init", "-b", "main", mainRepo)
	runGitCommand(t, "-C", mainRepo, "remote", "add", "origin", "git@github.com:acme/worktree-fixture.git")
	{
		err := os.MkdirAll(filepath.Join(mainRepo, "plans"), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(mainRepo, "plans", "activation.md"), []byte("# Activation Plan\n\nKeep first-run output useful.\n"), 0o644)
		require.NoError(t, err)
	}

	runGitCommand(t, "-C", mainRepo, "add", ".")
	runGitCommand(t, "-C", mainRepo, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "initial")

	first := NewScanCmd()
	first.SetArgs([]string{"--path", mainRepo, "--quiet"})
	{
		err := first.Execute()
		require.NoError(t, err)
	}

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

	var firstArtifacts int
	{
		err := db.QueryRow(`SELECT COUNT(*) FROM artifacts`).Scan(&firstArtifacts)
		require.NoError(t, err)
	}
	assert.NotEqual(t, 0, firstArtifacts,
		"fixture scan produced no artifacts")
	{

		err := db.Close()
		require.NoError(t, err)
	}

	runGitCommand(t, "-C", mainRepo, "worktree", "add", "-b", "agent-lane", worktree)
	db, err = store.Open(dbPath)
	require.NoError(t, err)

	opts, err := liveScanRunOptions(db, worktree)
	require.NoError(t, err)
	assert.False(t, opts.FreshIndex,
		"known logical repository worktree selected fresh-index insertion")
	{

		err := db.Close()
		require.NoError(t, err)
	}

	second := NewScanCmd()
	second.SetArgs([]string{"--path", worktree, "--quiet"})
	{
		err := second.Execute()
		require.NoError(t, err)
	}

	db, err = store.Open(dbPath)
	require.NoError(t, err)

	defer db.Close()
	var repoCount, rootCount, secondArtifacts int
	{
		err := db.QueryRow(`SELECT COUNT(*) FROM repos`).Scan(&repoCount)
		require.NoError(t, err)
	}
	{

		err := db.QueryRow(`SELECT COUNT(*) FROM repo_roots`).Scan(&rootCount)
		require.NoError(t, err)
	}
	{

		err := db.QueryRow(`SELECT COUNT(*) FROM artifacts`).Scan(&secondArtifacts)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, repoCount, "worktree duplicated index: repos=%d roots=%d artifacts=%d first_artifacts=%d", repoCount, rootCount, secondArtifacts, firstArtifacts)
	assert.Equal(t, 2, rootCount, "worktree duplicated index: repos=%d roots=%d artifacts=%d first_artifacts=%d", repoCount, rootCount, secondArtifacts, firstArtifacts)
	assert.Equal(t, firstArtifacts, secondArtifacts, "worktree duplicated index: repos=%d roots=%d artifacts=%d first_artifacts=%d", repoCount, rootCount, secondArtifacts, firstArtifacts)

}

func TestScan_BackfillsLegacyRepositoryIdentityOnce(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	repoRoot := filepath.Join(tmp, "repo")
	t.Setenv("DEVSPECS_HOME", home)
	t.Setenv("DEVSPECS_TELEMETRY", "0")
	runGitCommand(t, "init", "-b", "main", repoRoot)
	runGitCommand(t, "-C", repoRoot, "remote", "add", "origin", "https://github.com/acme/legacy-fixture.git")
	{
		err := os.WriteFile(filepath.Join(repoRoot, "plan.md"), []byte("# Legacy Plan\n\nBackfill identity.\n"), 0o644)
		require.NoError(t, err)
	}

	runGitCommand(t, "-C", repoRoot, "add", ".")
	runGitCommand(t, "-C", repoRoot, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", "initial")

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

	now := "2026-08-06T00:00:00Z"
	{
		_, err := db.Exec(`INSERT INTO repos (id, root_path, git_remote_url, created_at, updated_at) VALUES ('legacy', ?, ?, ?, ?)`, repoRoot, "https://github.com/acme/legacy-fixture.git", now, now)
		require.NoError(t, err)
	}
	{

		_, err := db.Exec(`INSERT INTO artifacts
		(id, repo_id, kind, subtype, title, status, created_at, updated_at, last_observed_at, authored_at)
		VALUES ('legacy-artifact', 'legacy', 'plan', '', 'Legacy', 'draft', ?, ?, ?, ?)`, now, now, now, now)
		require.NoError(t, err)
	}

	opts, err := liveScanRunOptions(db, repoRoot)
	require.NoError(t, err)
	assert.False(t, opts.FreshIndex, "legacy repo was not prepared for identity backfill: %#v", opts.RepositoryInfo)
	require.NotNil(t, opts.RepositoryInfo, "legacy repo was not prepared for identity backfill: %#v", opts.RepositoryInfo)
	assert.NotEqual(t, "", opts.RepositoryInfo.GitIdentity, "legacy repo was not prepared for identity backfill: %#v", opts.RepositoryInfo)
	{

		err := db.Close()
		require.NoError(t, err)
	}

	cmd := NewScanCmd()
	cmd.SetArgs([]string{"--path", repoRoot, "--quiet"})
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	db, err = store.Open(dbPath)
	require.NoError(t, err)

	defer db.Close()
	var identity string
	{
		err := db.QueryRow(`SELECT COALESCE(git_identity, '') FROM repos WHERE id = 'legacy'`).Scan(&identity)
		require.NoError(t, err)
	}
	assert.NotEqual(t, "", identity,
		"legacy repository identity was not persisted by scan")

}

func runGitCommand(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	{
		output, err := cmd.CombinedOutput()
		require.NoError(t, err,
			"git %s: %v\n%s", strings.Join(args, " "), err, output)
	}

}

func TestLooksLikeTestArtifactPath(t *testing.T) {
	{
		path := "pkg/webhook_test.go"

		assert.True(t, looksLikeTestArtifactPath(path),
			"%s should be treated as a test artifact path", path)

	}
	{
		path := "tests/test_billing.py"

		assert.True(t, looksLikeTestArtifactPath(path),
			"%s should be treated as a test artifact path", path)

	}
	{
		path := "src/__tests__/billing.spec.ts"

		assert.True(t, looksLikeTestArtifactPath(path),
			"%s should be treated as a test artifact path", path)

	}
	{
		path := "spec/billing_spec.rb"

		assert.True(t, looksLikeTestArtifactPath(path),
			"%s should be treated as a test artifact path", path)

	}
	{
		path := "tests/BillingTest.php"

		assert.True(t, looksLikeTestArtifactPath(path),
			"%s should be treated as a test artifact path", path)

	}

	assert.False(t, looksLikeTestArtifactPath("docs/plans/billing.md"),
		"plan markdown should not be treated as a test artifact path")

}
