package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/evalharness"
	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvalCommand_TextOutputLabelsRetrieverAndTokenCounter(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), "--no-save"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Contains(t, out, "Fixture version: agentic-saas-fragmented-v1",
		"missing %q in output:\n%s", "Fixture version: agentic-saas-fragmented-v1", out)
	assert.Contains(t, out, "Eval stage: seed_smoke",
		"missing %q in output:\n%s", "Eval stage: seed_smoke", out)
	assert.Contains(t, out, "Corpus source: sqlite_index",
		"missing %q in output:\n%s", "Corpus source: sqlite_index", out)
	assert.Contains(t, out, "Product path: indexed_harness",
		"missing %q in output:\n%s", "Product path: indexed_harness", out)
	assert.Contains(t, out, "Retriever: eval_weighted_files_v0",
		"missing %q in output:\n%s", "Retriever: eval_weighted_files_v0", out)
	assert.Contains(t, out, "Token counter: approx_chars_div_4",
		"missing %q in output:\n%s", "Token counter: approx_chars_div_4", out)
	assert.Contains(t, out, "Pricing profile: none",
		"missing %q in output:\n%s", "Pricing profile: none", out)
	assert.Contains(t, out, "Corpus",
		"missing %q in output:\n%s", "Corpus", out)
	assert.Contains(t, out, "Mean must-have recall:",
		"missing %q in output:\n%s", "Mean must-have recall:", out)
	assert.Contains(t, out, "Context sufficiency pass rate:",
		"missing %q in output:\n%s", "Context sufficiency pass rate:", out)
	assert.Contains(t, out, "Must-hit@3:",
		"missing %q in output:\n%s", "Must-hit@3:", out)
	assert.Contains(t, out, "Pareto:",
		"missing %q in output:\n%s", "Pareto:", out)
	assert.Contains(t, out, "Lane Metrics",
		"missing %q in output:\n%s", "Lane Metrics", out)
	assert.Contains(t, out, "Diagnostics",
		"missing %q in output:\n%s", "Diagnostics", out)
	assert.Contains(t, out, "Discovery coverage:",
		"missing %q in output:\n%s", "Discovery coverage:", out)
	assert.Contains(t, out, "Role summaries:",
		"missing %q in output:\n%s", "Role summaries:", out)
	assert.Contains(t, out, "Case: resume-entitlement-sync",
		"missing %q in output:\n%s", "Case: resume-entitlement-sync", out)
	assert.Contains(t, out, "Graded precision:",
		"missing %q in output:\n%s", "Graded precision:", out)

}

func TestEvalCommand_JSONOutput(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), "--json", "--no-save"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var got map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &got)
		require.NoError(t, err)
	}
	assert.Equal(t, "eval_weighted_files_v0", got["retriever"],
		"retriever = %#v", got["retriever"])
	assert.Equal(t, "approx_chars_div_4", got["token_counter"],
		"token_counter = %#v", got["token_counter"])
	assert.Equal(t, "seed_smoke", got["eval_stage"],
		"eval_stage = %#v", got["eval_stage"])
	assert.Equal(t, "sqlite_index", got["corpus_source"],
		"corpus_source = %#v", got["corpus_source"])
	assert.Equal(t, "indexed_harness", got["product_path"],
		"product_path = %#v", got["product_path"])
	{

		_, ok := got["corpus"].(map[string]any)
		assert.True(t, ok,
			"missing corpus summary: %#v", got["corpus"])
	}

	summary, ok := got["summary"].(map[string]any)
	require.True(t, ok,
		"missing summary: %#v", got["summary"])
	{

		_, ok := summary["pareto"].(map[string]any)
		assert.True(t, ok,
			"missing pareto summary: %#v", summary["pareto"])
	}
	{

		_, ok := summary["context_sufficiency_pass_rate"].(float64)
		assert.True(t, ok,
			"missing sufficiency pass rate: %#v", summary["context_sufficiency_pass_rate"])
	}
	{

		_, ok := summary["agent_metrics"].(map[string]any)
		assert.True(t, ok,
			"missing summary agent metrics: %#v", summary["agent_metrics"])
	}
	{

		_, ok := got["agent_metrics"].(map[string]any)
		assert.True(t, ok,
			"missing agent metrics: %#v", got["agent_metrics"])
	}
	{

		lanes, ok := got["lane_metrics"].([]any)
		require.True(t, ok, "missing lane metrics: %#v", got["lane_metrics"])
		require.NotEmpty(t, lanes, "missing lane metrics: %#v", got["lane_metrics"])
	}

	cases, ok := got["cases"].([]any)
	require.True(t, ok, "missing cases: %#v", got["cases"])
	require.NotEmpty(t, cases, "missing cases: %#v", got["cases"])

	firstCase, ok := cases[0].(map[string]any)
	require.True(t, ok, "invalid first case: %#v", cases[0])
	{
		_, ok := firstCase["agent_metrics"].(map[string]any)
		assert.True(t, ok,
			"missing case agent metrics: %#v", firstCase["agent_metrics"])
	}
	{

		grades, ok := firstCase["artifact_grades"].([]any)
		require.True(t, ok, "missing artifact grades: %#v", firstCase["artifact_grades"])
		require.NotEmpty(t, grades, "missing artifact grades: %#v", firstCase["artifact_grades"])
	}

	diagnostics, ok := got["diagnostics"].(map[string]any)
	require.True(t, ok,
		"missing diagnostics: %#v", got["diagnostics"])
	{

		_, ok := diagnostics["discovery_coverage"].(float64)
		assert.True(t, ok,
			"missing discovery coverage: %#v", diagnostics["discovery_coverage"])
	}
	{

		summaries, ok := diagnostics["role_summaries"].([]any)
		require.True(t, ok, "missing role summaries: %#v", diagnostics["role_summaries"])
		require.NotEmpty(t, summaries, "missing role summaries: %#v", diagnostics["role_summaries"])
	}

}

func TestEvalCommand_ClassifierTextOutput(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), "--classifier", "--no-save"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Contains(t, out, "DevSpecs Classifier Eval:",
		"missing %q in output:\n%s", "DevSpecs Classifier Eval:", out)
	assert.Contains(t, out, "Eval stage: seed_smoke",
		"missing %q in output:\n%s", "Eval stage: seed_smoke", out)
	assert.Contains(t, out, "Evaluator: declarative_document_models_v0",
		"missing %q in output:\n%s", "Evaluator: declarative_document_models_v0", out)
	assert.Contains(t, out, "Classifier profile: builtin_intent_docs_v1",
		"missing %q in output:\n%s", "Classifier profile: builtin_intent_docs_v1", out)
	assert.Contains(t, out, "Model accuracy:",
		"missing %q in output:\n%s", "Model accuracy:", out)
	assert.Contains(t, out, "Generic fallback rate:",
		"missing %q in output:\n%s", "Generic fallback rate:", out)
	assert.Contains(t, out, "Case: adr-webhook-idempotency-nygard",
		"missing %q in output:\n%s", "Case: adr-webhook-idempotency-nygard", out)

}

func TestEvalCommand_ClassifierJSONOutput(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), "--classifier", "--json", "--no-save"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var got map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &got)
		require.NoError(t, err)
	}
	assert.Equal(t, "declarative_document_models_v0", got["evaluator"],
		"evaluator = %#v", got["evaluator"])
	assert.Equal(t, "builtin_intent_docs_v1", got["classifier_profile"],
		"classifier_profile = %#v", got["classifier_profile"])
	assert.Equal(t, "seed_smoke", got["eval_stage"],
		"eval_stage = %#v", got["eval_stage"])

	summary, ok := got["summary"].(map[string]any)
	require.True(t, ok,
		"missing summary: %#v", got["summary"])
	{

		_, ok := summary["accuracy"].(float64)
		assert.True(t, ok,
			"missing accuracy: %#v", summary["accuracy"])
	}

}

func TestEvalCommand_FirstIndexReportTextOutput(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"),
		"--first-index-report",
		"--classifier-fixture", filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"),
		"--input-usd-per-1m", "0.15",
		"--no-save",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Contains(t, out, "DevSpecs First-Index Eval Report",
		"missing %q in output:\n%s", "DevSpecs First-Index Eval Report", out)
	assert.Contains(t, out, "North Star",
		"missing %q in output:\n%s", "North Star", out)
	assert.Contains(t, out, "Token reduction:",
		"missing %q in output:\n%s", "Token reduction:", out)
	assert.Contains(t, out, "saved",
		"missing %q in output:\n%s", "saved", out)
	assert.Contains(t, out, "Retrieval: precision",
		"missing %q in output:\n%s", "Retrieval: precision", out)
	assert.Contains(t, out, "Agent:",
		"missing %q in output:\n%s", "Agent:", out)
	assert.Contains(t, out, "Sufficiency:",
		"missing %q in output:\n%s", "Sufficiency:", out)
	assert.Contains(t, out, "Discovery:",
		"missing %q in output:\n%s", "Discovery:", out)
	assert.Contains(t, out, "Classifier:",
		"missing %q in output:\n%s", "Classifier:", out)
	assert.Contains(t, out, "Retrieval And Tokens",
		"missing %q in output:\n%s", "Retrieval And Tokens", out)
	assert.Contains(t, out, "Lane metrics:",
		"missing %q in output:\n%s", "Lane metrics:", out)
	assert.Contains(t, out, "Classifier Fixtures",
		"missing %q in output:\n%s", "Classifier Fixtures", out)
	assert.Contains(t, out, "Model adr:",
		"missing %q in output:\n%s", "Model adr:", out)
	assert.Contains(t, out, "Residual Risks",
		"missing %q in output:\n%s", "Residual Risks", out)

}

func TestEvalCommand_FirstIndexReportJSONOutput(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"),
		"--first-index-report",
		"--classifier-fixture", filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"),
		"--json",
		"--no-save",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var got map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &got)
		require.NoError(t, err)
	}
	{

		_, ok := got["generated_at"].(string)
		assert.True(t, ok,
			"missing generated_at: %#v", got["generated_at"])
	}

	summary, ok := got["summary"].(map[string]any)
	require.True(t, ok,
		"missing summary: %#v", got["summary"])

	{
		key := "mean_token_reduction_vs_full_planning"

		{
			_, ok := summary[key]
			assert.True(t, ok,
				"missing summary[%s]: %#v", key, summary)
		}

	}
	{
		key := "mean_artifact_precision"

		{
			_, ok := summary[key]
			assert.True(t, ok,
				"missing summary[%s]: %#v", key, summary)
		}

	}
	{
		key := "mean_artifact_recall"

		{
			_, ok := summary[key]
			assert.True(t, ok,
				"missing summary[%s]: %#v", key, summary)
		}

	}
	{
		key := "context_sufficiency_pass_rate"

		{
			_, ok := summary[key]
			assert.True(t, ok,
				"missing summary[%s]: %#v", key, summary)
		}

	}
	{
		key := "saved_input_tokens_vs_full_planning"

		{
			_, ok := summary[key]
			assert.True(t, ok,
				"missing summary[%s]: %#v", key, summary)
		}

	}
	{
		key := "classifier_accuracy"

		{
			_, ok := summary[key]
			assert.True(t, ok,
				"missing summary[%s]: %#v", key, summary)
		}

	}

	retrieval, ok := got["retrieval"].(map[string]any)
	require.True(t, ok,
		"missing retrieval: %#v", got["retrieval"])
	assert.Equal(t, "eval_weighted_files_v0", retrieval["retriever"],
		"retriever = %#v", retrieval["retriever"])
	{

		_, ok := retrieval["agent_metrics"].(map[string]any)
		assert.True(t, ok,
			"missing retrieval agent metrics: %#v", retrieval["agent_metrics"])
	}
	{

		lanes, ok := retrieval["lane_metrics"].([]any)
		require.True(t, ok, "missing retrieval lane metrics: %#v", retrieval["lane_metrics"])
		require.NotEmpty(t, lanes, "missing retrieval lane metrics: %#v", retrieval["lane_metrics"])
	}

	classifiers, ok := got["classifiers"].([]any)
	require.True(t, ok, "expected one classifier summary: %#v", got["classifiers"])
	require.Len(t, classifiers, 1, "expected one classifier summary: %#v", got["classifiers"])

	first, ok := classifiers[0].(map[string]any)
	require.True(t, ok, "invalid classifier summary: %#v", classifiers[0])
	{
		_, ok := first["models"].([]any)
		assert.True(t, ok,
			"missing classifier models: %#v", first["models"])
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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var got map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &got)
		require.NoError(t, err)
	}

	summary, ok := got["summary"].(map[string]any)
	require.True(t, ok,
		"missing summary: %#v", got["summary"])
	recall, ok := summary["mean_artifact_recall"].(float64)
	require.True(t, ok, "missing mean artifact recall: %#v", summary["mean_artifact_recall"])
	assert.Greater(t, recall, 0.0,
		"expected positive recall: %#v", summary)

	retrievals, ok := got["retrievals"].([]any)
	require.True(t, ok, "expected two retrieval reports: %#v", got["retrievals"])
	require.Len(t, retrievals, 2, "expected two retrieval reports: %#v", got["retrievals"])

}

func TestEvalCommand_FilesystemCorpusDiagnosticFlag(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), "--filesystem", "--json", "--no-save"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var got map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &got)
		require.NoError(t, err)
	}
	assert.Equal(t, "filesystem_fixture", got["corpus_source"],
		"corpus_source = %#v", got["corpus_source"])
	assert.Equal(t, "lab_only", got["product_path"],
		"product_path = %#v", got["product_path"])

	corpus, ok := got["corpus"].(map[string]any)
	require.True(t, ok, "missing corpus: %#v", got["corpus"])
	planning, ok := corpus["planning_artifacts"].(map[string]any)
	require.True(t, ok, "missing planning artifacts: %#v", corpus["planning_artifacts"])
	files, ok := planning["files"].(float64)
	require.True(t, ok, "missing planning file count: %#v", planning["files"])
	assert.NotZero(t, files,
		"filesystem eval should load planning artifacts: %#v", planning)

}

func TestEvalCommand_WithIndexCacheFlags_ReportsEnabledCacheAndTelemetry(t *testing.T) {
	root := t.TempDir()
	writeBatchEvalFixture(t, root, "alpha", "billing retry plan")
	cacheDir := filepath.Join(t.TempDir(), "cache")

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

	err := cmd.Execute()

	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	cache, ok := got["index_cache"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, cache["enabled"])
	phases, ok := got["phase_telemetry"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, phases)
}

func writeBatchEvalFixture(t *testing.T, root, id, phrase string) {
	t.Helper()
	{
		err := os.MkdirAll(filepath.Join(root, "docs", "plans"), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(root, "docs", "plans", id+".md"), []byte("# "+phrase+"\n\nThis plan covers "+phrase+" for implementation.\n"), 0o644)
		require.NoError(t, err)
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
	{
		err := os.WriteFile(filepath.Join(root, "cases.yaml"), []byte(cases), 0o644)
		require.NoError(t, err)
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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var got map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &got)
		require.NoError(t, err)
	}
	assert.Equal(t, "live_cli_command", got["product_path"],
		"product_path = %#v", got["product_path"])
	assert.Equal(t, "resume-query", got["command_under_test"],
		"command_under_test = %#v", got["command_under_test"])

	cases, ok := got["cases"].([]any)
	require.True(t, ok, "missing cases: %#v", got["cases"])
	require.NotEmpty(t, cases, "missing cases: %#v", got["cases"])

	first, ok := cases[0].(map[string]any)
	require.True(t, ok, "invalid first case: %#v", cases[0])
	{
		_, ok := first["artifact_reasons"].([]any)
		assert.True(t, ok,
			"missing artifact reasons: %#v", first["artifact_reasons"])
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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Contains(t, out, "Results file:",
		"missing results file in output:\n%s", out)

	matches, err := filepath.Glob(filepath.Join(resultsDir, "agentic-saas-fragmented", "20260513T123456Z_agentic-saas-fragmented_seed_smoke_classifier_declarative_document_models_v0_builtin_intent_docs_v1.json"))
	require.NoError(t, err)
	require.Len(t, matches, 1,
		"expected one timestamped classifier result file, got %d", len(matches))

	data, err := os.ReadFile(matches[0])
	require.NoError(t, err)

	var got map[string]any
	{
		err := json.Unmarshal(data, &got)
		require.NoError(t, err)
	}
	assert.NotEqual(t, "", got["results_file"],
		"saved result missing results_file: %#v", got["results_file"])
	assert.Equal(t, "declarative_document_models_v0", got["evaluator"],
		"evaluator = %#v", got["evaluator"])

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
	require.Error(t, err,
		"expected --command with --filesystem to fail")
	assert.Contains(t, err.Error(), "--command requires the indexed eval corpus",
		"unexpected error: %v", err)

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Contains(t, out, "Results file:",
		"missing results file in output:\n%s", out)

	matches, err := filepath.Glob(filepath.Join(resultsDir, "agentic-saas-fragmented", "20260513T123456Z_agentic-saas-fragmented_seed_smoke_eval_weighted_files_v0.json"))
	require.NoError(t, err)
	require.Len(t, matches, 1,
		"expected one timestamped result file, got %d", len(matches))

	data, err := os.ReadFile(matches[0])
	require.NoError(t, err)

	var got map[string]any
	{
		err := json.Unmarshal(data, &got)
		require.NoError(t, err)
	}
	assert.NotEqual(t, "", got["results_file"],
		"saved result missing results_file: %#v", got["results_file"])
	assert.Equal(t, "seed_smoke", got["eval_stage"],
		"eval_stage = %#v", got["eval_stage"])

}

func TestRunFindForEvalUsesLineScopedPath(t *testing.T) {
	repoDir, _ := setupReadEnv(t)
	relPath := seedLineScopedTestArtifacts(t, repoDir)

	db, err := openDB()
	require.NoError(t, err)

	defer db.Close()

	candidates, err := loadRetrievalCandidates(db, store.FilterParams{RepoRoot: canonicalRepoRoot(repoDir)})
	require.NoError(t, err)

	output, err := runFindForEval(evalharness.CaseSpec{
		ID:    "camel-tool-cache",
		Query: "what tests cover testputandgetexposedtool behavior",
	}, candidatesByArtifactPath(candidates), false, retrieval.AnchorFirstModeV1, false)
	require.NoError(t, err)
	require.NotEmpty(t, output.Artifacts,
		"runFindForEval returned no artifacts")

	wantPath := filepath.ToSlash(relPath) + "#L53"
	assert.Equal(t, wantPath, output.Artifacts[0].Path,
		"first eval artifact path = %q, want %q", output.Artifacts[0].Path, wantPath)
	assert.Equal(t, filepath.ToSlash(relPath), output.Artifacts[0].Source,
		"first eval artifact source = %q, want %q", output.Artifacts[0].Source, filepath.ToSlash(relPath))

}

func TestRunFindForEvalRecordsGraphContextSeparately(t *testing.T) {
	repoDir, _ := setupReadEnv(t)
	seedGraphDiagnosticArtifacts(t, repoDir)

	db, err := openDB()
	require.NoError(t, err)

	defer db.Close()

	candidates, err := loadRetrievalCandidates(db, store.FilterParams{RepoRoot: canonicalRepoRoot(repoDir)})
	require.NoError(t, err)

	output, err := runFindForEval(evalharness.CaseSpec{
		ID:    "session-graph",
		Query: "rotatetoken implementation",
	}, candidatesByArtifactPath(candidates), false, retrieval.AnchorFirstModeV1, true)
	require.NoError(t, err)
	require.NotEmpty(t, output.Artifacts,
		"runFindForEval returned no direct artifacts")
	require.NotNil(t, output.GraphContext, "expected graph context and diagnostics: %#v", output)
	require.NotNil(t, output.GraphDiagnostics, "expected graph context and diagnostics: %#v", output)
	assert.Equal(t, 1, output.GraphContext.CandidateCount, "expected one graph context artifact: %#v", output.GraphContext)
	require.Len(t, output.GraphContextArtifacts, 1, "expected one graph context artifact: %#v", output.GraphContext)

	graphPath := output.GraphContextArtifacts[0].Path
	for _, direct := range output.Artifacts {
		assert.NotEqual(t, graphPath, direct.Path,
			"graph context artifact leaked into direct artifacts: %s", graphPath)

	}
	assert.Equal(t, "src/session.test.ts", output.GraphContextArtifacts[0].Source,
		"graph artifact source = %q", output.GraphContextArtifacts[0].Source)

}

func TestEvalCommand_GraphDiagnosticsRequiresFindCommand(t *testing.T) {
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented"), "--graph-diagnostics", "--no-save"})
	cmd.SetOut(&bytes.Buffer{})
	err := cmd.Execute()
	require.NotNil(t, err, "expected graph diagnostics command validation error, got %v", err)
	assert.Contains(t, err.Error(), "--graph-diagnostics requires --command find", "expected graph diagnostics command validation error, got %v", err)

}
