package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvalCommand_TextOutputLabelsRetrieverAndTokenCounter(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), "--no-save"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Fixture version: agentic-saas-fragmented-v1",
		"Eval stage: seed_smoke",
		"Corpus source: sqlite_index",
		"Product path: indexed_harness",
		"Retriever: eval_weighted_files_v0",
		"Token counter: approx_chars_div_4",
		"Pricing profile: none",
		"Corpus",
		"Mean must-have recall:",
		"Context sufficiency pass rate:",
		"Pareto:",
		"Case: resume-entitlement-sync",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestEvalCommand_JSONOutput(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), "--json", "--no-save"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["retriever"] != "eval_weighted_files_v0" {
		t.Fatalf("retriever = %#v", got["retriever"])
	}
	if got["token_counter"] != "approx_chars_div_4" {
		t.Fatalf("token_counter = %#v", got["token_counter"])
	}
	if got["eval_stage"] != "seed_smoke" {
		t.Fatalf("eval_stage = %#v", got["eval_stage"])
	}
	if got["corpus_source"] != "sqlite_index" {
		t.Fatalf("corpus_source = %#v", got["corpus_source"])
	}
	if got["product_path"] != "indexed_harness" {
		t.Fatalf("product_path = %#v", got["product_path"])
	}
	if _, ok := got["corpus"].(map[string]any); !ok {
		t.Fatalf("missing corpus summary: %#v", got["corpus"])
	}
	summary, ok := got["summary"].(map[string]any)
	if !ok {
		t.Fatalf("missing summary: %#v", got["summary"])
	}
	if _, ok := summary["pareto"].(map[string]any); !ok {
		t.Fatalf("missing pareto summary: %#v", summary["pareto"])
	}
	if _, ok := summary["context_sufficiency_pass_rate"].(float64); !ok {
		t.Fatalf("missing sufficiency pass rate: %#v", summary["context_sufficiency_pass_rate"])
	}
}

func TestEvalCommand_FilesystemCorpusDiagnosticFlag(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), "--filesystem", "--json", "--no-save"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["corpus_source"] != "filesystem_fixture" {
		t.Fatalf("corpus_source = %#v", got["corpus_source"])
	}
	if got["product_path"] != "lab_only" {
		t.Fatalf("product_path = %#v", got["product_path"])
	}
	corpus := got["corpus"].(map[string]any)
	planning := corpus["planning_artifacts"].(map[string]any)
	if planning["files"].(float64) == 0 {
		t.Fatalf("filesystem eval should load planning artifacts: %#v", planning)
	}
}

func TestEvalCommand_LiveResumeQueryCommand(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"),
		"--command", "resume-query",
		"--json",
		"--no-save",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["product_path"] != "live_cli_command" {
		t.Fatalf("product_path = %#v", got["product_path"])
	}
	if got["command_under_test"] != "resume-query" {
		t.Fatalf("command_under_test = %#v", got["command_under_test"])
	}
	cases, ok := got["cases"].([]any)
	if !ok || len(cases) == 0 {
		t.Fatalf("missing cases: %#v", got["cases"])
	}
	first := cases[0].(map[string]any)
	if _, ok := first["artifact_reasons"].([]any); !ok {
		t.Fatalf("missing artifact reasons: %#v", first["artifact_reasons"])
	}
}

func TestEvalCommand_CommandRejectsFilesystemCorpus(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"),
		"--command", "find",
		"--filesystem",
		"--no-save",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected --command with --filesystem to fail")
	}
	if !strings.Contains(err.Error(), "--command requires the indexed eval corpus") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEvalCommand_SavesTimestampedResultFile(t *testing.T) {
	oldNow := nowUTC
	nowUTC = func() time.Time {
		return time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	}
	defer func() { nowUTC = oldNow }()

	resultsDir := t.TempDir()
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"),
		"--results-dir", resultsDir,
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Results file:") {
		t.Fatalf("missing results file in output:\n%s", out)
	}

	matches, err := filepath.Glob(filepath.Join(resultsDir, "agentic-saas-fragmented", "20260513T123456Z_agentic-saas-fragmented_seed_smoke_eval_weighted_files_v0.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one timestamped result file, got %d", len(matches))
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["results_file"] == "" {
		t.Fatalf("saved result missing results_file: %#v", got["results_file"])
	}
	if got["eval_stage"] != "seed_smoke" {
		t.Fatalf("eval_stage = %#v", got["eval_stage"])
	}
}
