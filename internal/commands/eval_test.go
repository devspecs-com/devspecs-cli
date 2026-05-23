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
		"Must-hit@3:",
		"Pareto:",
		"Lane Metrics",
		"Diagnostics",
		"Discovery coverage:",
		"Role summaries:",
		"Case: resume-entitlement-sync",
		"Graded precision:",
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
	if _, ok := summary["agent_metrics"].(map[string]any); !ok {
		t.Fatalf("missing summary agent metrics: %#v", summary["agent_metrics"])
	}
	if _, ok := got["agent_metrics"].(map[string]any); !ok {
		t.Fatalf("missing agent metrics: %#v", got["agent_metrics"])
	}
	if lanes, ok := got["lane_metrics"].([]any); !ok || len(lanes) == 0 {
		t.Fatalf("missing lane metrics: %#v", got["lane_metrics"])
	}
	cases, ok := got["cases"].([]any)
	if !ok || len(cases) == 0 {
		t.Fatalf("missing cases: %#v", got["cases"])
	}
	firstCase := cases[0].(map[string]any)
	if _, ok := firstCase["agent_metrics"].(map[string]any); !ok {
		t.Fatalf("missing case agent metrics: %#v", firstCase["agent_metrics"])
	}
	if grades, ok := firstCase["artifact_grades"].([]any); !ok || len(grades) == 0 {
		t.Fatalf("missing artifact grades: %#v", firstCase["artifact_grades"])
	}
	diagnostics, ok := got["diagnostics"].(map[string]any)
	if !ok {
		t.Fatalf("missing diagnostics: %#v", got["diagnostics"])
	}
	if _, ok := diagnostics["discovery_coverage"].(float64); !ok {
		t.Fatalf("missing discovery coverage: %#v", diagnostics["discovery_coverage"])
	}
	if summaries, ok := diagnostics["role_summaries"].([]any); !ok || len(summaries) == 0 {
		t.Fatalf("missing role summaries: %#v", diagnostics["role_summaries"])
	}
}

func TestEvalCommand_ClassifierTextOutput(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), "--classifier", "--no-save"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"DevSpecs Classifier Eval:",
		"Eval stage: seed_smoke",
		"Evaluator: declarative_document_models_v0",
		"Classifier profile: builtin_intent_docs_v1",
		"Model accuracy:",
		"Generic fallback rate:",
		"Case: adr-webhook-idempotency-nygard",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestEvalCommand_ClassifierJSONOutput(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), "--classifier", "--json", "--no-save"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["evaluator"] != "declarative_document_models_v0" {
		t.Fatalf("evaluator = %#v", got["evaluator"])
	}
	if got["classifier_profile"] != "builtin_intent_docs_v1" {
		t.Fatalf("classifier_profile = %#v", got["classifier_profile"])
	}
	if got["eval_stage"] != "seed_smoke" {
		t.Fatalf("eval_stage = %#v", got["eval_stage"])
	}
	summary, ok := got["summary"].(map[string]any)
	if !ok {
		t.Fatalf("missing summary: %#v", got["summary"])
	}
	if _, ok := summary["accuracy"].(float64); !ok {
		t.Fatalf("missing accuracy: %#v", summary["accuracy"])
	}
}

func TestEvalCommand_FirstIndexReportTextOutput(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"),
		"--first-index-report",
		"--classifier-fixture", filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"),
		"--classifier-fixture", filepath.Join("..", "..", "fixtures", "mined-intent-samples"),
		"--input-usd-per-1m", "0.15",
		"--no-save",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"DevSpecs First-Index Eval Report",
		"North Star",
		"Token reduction:",
		"saved",
		"Retrieval: precision",
		"Agent:",
		"Sufficiency:",
		"Discovery:",
		"Classifier:",
		"Retrieval And Tokens",
		"Lane metrics:",
		"Classifier Fixtures",
		"Model adr:",
		"Residual Risks",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestEvalCommand_FirstIndexReportJSONOutput(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"),
		"--first-index-report",
		"--classifier-fixture", filepath.Join("..", "..", "fixtures", "mined-intent-samples"),
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
	if _, ok := got["generated_at"].(string); !ok {
		t.Fatalf("missing generated_at: %#v", got["generated_at"])
	}
	summary, ok := got["summary"].(map[string]any)
	if !ok {
		t.Fatalf("missing summary: %#v", got["summary"])
	}
	for _, key := range []string{
		"mean_token_reduction_vs_full_planning",
		"mean_artifact_precision",
		"mean_artifact_recall",
		"context_sufficiency_pass_rate",
		"saved_input_tokens_vs_full_planning",
		"classifier_accuracy",
	} {
		if _, ok := summary[key]; !ok {
			t.Fatalf("missing summary[%s]: %#v", key, summary)
		}
	}
	retrieval, ok := got["retrieval"].(map[string]any)
	if !ok {
		t.Fatalf("missing retrieval: %#v", got["retrieval"])
	}
	if retrieval["retriever"] != "eval_weighted_files_v0" {
		t.Fatalf("retriever = %#v", retrieval["retriever"])
	}
	if _, ok := retrieval["agent_metrics"].(map[string]any); !ok {
		t.Fatalf("missing retrieval agent metrics: %#v", retrieval["agent_metrics"])
	}
	if lanes, ok := retrieval["lane_metrics"].([]any); !ok || len(lanes) == 0 {
		t.Fatalf("missing retrieval lane metrics: %#v", retrieval["lane_metrics"])
	}
	classifiers, ok := got["classifiers"].([]any)
	if !ok || len(classifiers) != 1 {
		t.Fatalf("expected one classifier summary: %#v", got["classifiers"])
	}
	first := classifiers[0].(map[string]any)
	if _, ok := first["models"].([]any); !ok {
		t.Fatalf("missing classifier models: %#v", first["models"])
	}
}

func TestEvalCommand_FirstIndexBatchReportJSONOutput(t *testing.T) {
	root := t.TempDir()
	writeBatchEvalFixture(t, filepath.Join(root, "repos", "repo-a"), "alpha", "billing retry plan")
	writeBatchEvalFixture(t, filepath.Join(root, "repos", "repo-b"), "beta", "session auth decision")

	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		root,
		"--first-index-report",
		"--batch-fixtures",
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
	summary, ok := got["summary"].(map[string]any)
	if !ok {
		t.Fatalf("missing summary: %#v", got["summary"])
	}
	if summary["mean_artifact_recall"].(float64) <= 0 {
		t.Fatalf("expected positive recall: %#v", summary)
	}
	retrievals, ok := got["retrievals"].([]any)
	if !ok || len(retrievals) != 2 {
		t.Fatalf("expected two retrieval reports: %#v", got["retrievals"])
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

func TestEvalCommand_IndexCacheFlags(t *testing.T) {
	root := t.TempDir()
	writeBatchEvalFixture(t, root, "alpha", "billing retry plan")
	cacheDir := filepath.Join(t.TempDir(), "cache")

	for i := 0; i < 2; i++ {
		cmd := NewEvalCmd()
		cmd.SetArgs([]string{
			root,
			"--json",
			"--no-save",
			"--eval-index-cache-dir", cacheDir,
			"--eval-max-source-files", "3",
			"--eval-max-case-seconds", "30",
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
		cache, ok := got["index_cache"].(map[string]any)
		if !ok {
			t.Fatalf("missing index_cache: %#v", got["index_cache"])
		}
		if cache["enabled"] != true {
			t.Fatalf("cache not enabled: %#v", cache)
		}
		if i == 1 && cache["hit"] != true {
			t.Fatalf("second run should hit cache: %#v", cache)
		}
		if phases, ok := got["phase_telemetry"].([]any); !ok || len(phases) == 0 {
			t.Fatalf("missing phase telemetry: %#v", got["phase_telemetry"])
		}
	}
}

func writeBatchEvalFixture(t *testing.T, root, id, phrase string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "docs", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "plans", id+".md"), []byte("# "+phrase+"\n\nThis plan covers "+phrase+" for implementation.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := strings.Join([]string{
		"fixture_version: " + id + "-v0",
		"eval_stage: real_repo_batch_smoke",
		"",
		"cases:",
		"  - id: " + id + "-case",
		"    query: \"" + phrase + "\"",
		"    expected_relevant:",
		"      - path: docs/plans/" + id + ".md",
		"        importance: must",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "cases.yaml"), []byte(cases), 0o644); err != nil {
		t.Fatal(err)
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

func TestEvalCommand_ClassifierSavesTimestampedResultFile(t *testing.T) {
	oldNow := nowUTC
	nowUTC = func() time.Time {
		return time.Date(2026, 5, 13, 12, 34, 56, 0, time.UTC)
	}
	defer func() { nowUTC = oldNow }()

	resultsDir := t.TempDir()
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"),
		"--classifier",
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

	matches, err := filepath.Glob(filepath.Join(resultsDir, "agentic-saas-fragmented", "20260513T123456Z_agentic-saas-fragmented_seed_smoke_classifier_declarative_document_models_v0_builtin_intent_docs_v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one timestamped classifier result file, got %d", len(matches))
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
	if got["evaluator"] != "declarative_document_models_v0" {
		t.Fatalf("evaluator = %#v", got["evaluator"])
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
