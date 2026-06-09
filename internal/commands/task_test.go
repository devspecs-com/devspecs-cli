package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func setupTaskCommandRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")

	mustMkdirAll(t, filepath.Join(repoDir, ".devspecs"))
	mustWriteFile(t, filepath.Join(repoDir, ".devspecs", "config.yaml"), `version: 1
artifacts:
  test_cases: true
sources:
  - type: markdown
    paths:
      - docs/plans
  - type: source_context
`)
	mustMkdirAll(t, filepath.Join(repoDir, "internal", "retrieval"))
	mustWriteFile(t, filepath.Join(repoDir, "internal", "retrieval", "ranking.go"), `package retrieval

func ImproveTestCompanionRecall(query string) string {
	return "test companion recall " + query
}
`)
	mustWriteFile(t, filepath.Join(repoDir, "internal", "retrieval", "ranking_test.go"), `package retrieval

import "testing"

func TestImproveTestCompanionRecall(t *testing.T) {
	if ImproveTestCompanionRecall("pack") == "" {
		t.Fatal("missing recall")
	}
}
`)
	mustMkdirAll(t, filepath.Join(repoDir, "docs", "plans"))
	mustWriteFile(t, filepath.Join(repoDir, "docs", "plans", "test-companion-recall.md"), `# Test companion recall

## Success Criteria

- [ ] Primary retrieval file is found.
- [ ] Test companion file is found or the miss is recorded.
`)

	origWd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origWd) })

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--quiet"})
	scanCmd.SetOut(&bytes.Buffer{})
	if err := scanCmd.Execute(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return repoDir
}

func taskGitCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Env = cleanTaskGitTestEnv()
	return cmd
}

func cleanTaskGitTestEnv() []string {
	blocked := map[string]bool{
		"GIT_DIR":                          true,
		"GIT_WORK_TREE":                    true,
		"GIT_INDEX_FILE":                   true,
		"GIT_PREFIX":                       true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	}
	var env []string
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok && blocked[key] {
			continue
		}
		env = append(env, entry)
	}
	return env
}

func TestTask_StartCreatesUncertaintyAwareWorkspace(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "spike-test", "--no-refresh", "--json", "improve test companion recall"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var out taskStartOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("task json: %v\n%s", err, buf.String())
	}
	if out.TaskID != "spike-test" {
		t.Fatalf("task id = %q", out.TaskID)
	}
	if !strings.HasPrefix(out.Workspace, filepath.Join(repoDir, "devspecs", "tasks", "spike-test")) {
		t.Fatalf("workspace = %q", out.Workspace)
	}
	if len(out.Slices) != 1 {
		t.Fatalf("expected one default slice, got %#v", out.Slices)
	}
	if filepath.Base(out.FirstSlicePath) != "A01-improve-test-companion-recall-plan.md" {
		t.Fatalf("first slice path = %q", out.FirstSlicePath)
	}
	if filepath.Base(out.ResultPath) != "A01-improve-test-companion-recall-result.md" {
		t.Fatalf("result path = %q", out.ResultPath)
	}
	for _, path := range []string{out.IndexPath, out.FirstSlicePath, out.ResultPath, out.ManifestPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}

	indexBody := mustReadFile(t, out.IndexPath)
	for _, want := range []string{
		"## Known Knowns",
		"## Known Unknowns",
		"## Confidence Summary",
		"## Task Slices",
		"## Agent Preflight Checklist",
		"A01-improve-test-companion-recall-plan.md",
		"Pack completeness",
	} {
		if !strings.Contains(indexBody, want) {
			t.Fatalf("A00 missing %q:\n%s", want, indexBody)
		}
	}

	firstSlice := mustReadFile(t, out.FirstSlicePath)
	for _, want := range []string{
		"## Goal",
		"## Description",
		"## Resources",
		"## Success Criteria",
		"## Tasks",
		"## Decision Gates",
	} {
		if !strings.Contains(firstSlice, want) {
			t.Fatalf("A01 missing %q:\n%s", want, firstSlice)
		}
	}

	resultTemplate := mustReadFile(t, out.ResultPath)
	for _, want := range []string{
		"## Files Actually Read",
		"## Critical Files DevSpecs Missed",
		"## Distracting Files DevSpecs Included",
		"## Decision Gates",
	} {
		if !strings.Contains(resultTemplate, want) {
			t.Fatalf("A01-1 missing %q:\n%s", want, resultTemplate)
		}
	}

	manifest := mustReadFile(t, out.ManifestPath)
	if !strings.Contains(manifest, `"predicted_context"`) || !strings.Contains(manifest, `"confidence"`) {
		t.Fatalf("manifest missing predicted context/confidence:\n%s", manifest)
	}
	if !strings.Contains(manifest, `"slices"`) || !strings.Contains(manifest, `"A01-improve-test-companion-recall-plan.md"`) {
		t.Fatalf("manifest missing slice artifacts:\n%s", manifest)
	}
	if !containsPath(out.PrimaryFiles, "internal/retrieval/ranking.go") {
		t.Fatalf("task preflight missing primary source from shared pack assembly: %#v", out.PrimaryFiles)
	}
	if !containsPath(out.TestFiles, "internal/retrieval/ranking_test.go") {
		t.Fatalf("task preflight missing test companion from shared pack assembly: %#v", out.TestFiles)
	}

	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	artifacts, err := db.ListArtifacts(store.FilterParams{RepoRoot: repoDir, SourceType: "capture"})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) < 2 {
		t.Fatalf("expected A00/A01 to be captured, got %d", len(artifacts))
	}
}

func TestTask_StartGreenfieldProfileUsesPlanningTemplate(t *testing.T) {
	setupTaskCommandRepo(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "greenfield-test",
		"--profile", "greenfield",
		"--no-refresh",
		"--index=false",
		"--json",
		"plan claims zone provider adapters",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var out taskStartOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("task json: %v\n%s", err, buf.String())
	}
	if out.Profile != taskProfileGreenfield {
		t.Fatalf("profile = %q", out.Profile)
	}

	var manifest taskManifest
	if err := json.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest); err != nil {
		t.Fatalf("manifest json: %v", err)
	}
	if manifest.Profile != taskProfileGreenfield {
		t.Fatalf("manifest profile = %q", manifest.Profile)
	}

	indexBody := mustReadFile(t, out.IndexPath)
	for _, want := range []string{
		"## Profile",
		"greenfield",
		"Treat predicted files as evidence, not required edit targets.",
		"before implementation scope expands",
	} {
		if !strings.Contains(indexBody, want) {
			t.Fatalf("greenfield index missing %q:\n%s", want, indexBody)
		}
	}

	planBody := mustReadFile(t, out.FirstSlicePath)
	for _, want := range []string{
		"bounded planning slice",
		"Test or Evaluation Signals",
		"Planning artifacts, acceptance checks, interface notes, eval cards, or test design.",
		"Draft the smallest useful planning artifact",
	} {
		if !strings.Contains(planBody, want) {
			t.Fatalf("greenfield plan missing %q:\n%s", want, planBody)
		}
	}
	for _, unwanted := range []string{
		"Inspect the predicted primary files.",
		"Implement the smallest useful change.",
		"Primary implementation surface is verified before edits.",
	} {
		if strings.Contains(planBody, unwanted) {
			t.Fatalf("greenfield plan contains code-change boilerplate %q:\n%s", unwanted, planBody)
		}
	}
}

func TestTask_StartSurfacesCheckpointFactRiskCards(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repoID := taskTestRepoID(t, db, repoDir)
	now := "2026-06-07T00:00:00Z"
	if err := db.UpsertTaskCheckpointFact(store.TaskCheckpointFact{
		RepoID:             repoID,
		TaskID:             "prior-risk-task",
		CheckpointID:       "cp_prior",
		Target:             "A01",
		Series:             "A",
		Stage:              "implemented",
		Decision:           "improve",
		CheckpointPath:     "checkpoints/prior.md",
		CheckpointJSONPath: "checkpoints/prior.json",
		CreatedAt:          now,
		ActualContextJSON:  `{}`,
		FeedbackJSON:       `{"critical_missed":["internal/retrieval/ranking_test.go"],"distracting_included":["fixtures/noisy-plan.md"]}`,
		EvidenceJSON:       `{}`,
		LearningsJSON:      `[{"learning_type":"validation_gap","summary":"focused retrieval validation was missing","evidence_refs":["internal/retrieval/ranking_test.go"],"applies_to":"internal/retrieval","confidence":"high"}]`,
		NextJSON:           `{}`,
		IndexedAt:          now,
	}); err != nil {
		t.Fatal(err)
	}

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "risk-card-test", "--no-refresh", "--index=false", "--json", "improve test companion recall"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out taskStartOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("task json: %v\n%s", err, buf.String())
	}
	for _, id := range []string{"prior-test-miss", "prior-noise", "validation-gap"} {
		if taskRiskCardByID(out.RiskCards, id) == nil {
			t.Fatalf("missing risk card %q: %#v", id, out.RiskCards)
		}
	}
	testMiss := taskRiskCardByID(out.RiskCards, "prior-test-miss")
	if testMiss == nil || !strings.Contains(strings.Join(testMiss.Evidence, "\n"), "internal/retrieval/ranking_test.go") {
		t.Fatalf("prior test miss evidence = %#v", testMiss)
	}

	var manifest taskManifest
	if err := json.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest); err != nil {
		t.Fatalf("manifest json: %v", err)
	}
	if taskRiskCardByID(manifest.RiskCards, "prior-test-miss") == nil {
		t.Fatalf("manifest missing risk cards: %#v", manifest.RiskCards)
	}
	indexBody := mustReadFile(t, out.IndexPath)
	for _, want := range []string{
		"## Risk Cards",
		"Prior checkpoint missed a related test",
		"Search same-package and same-stem tests before editing.",
	} {
		if !strings.Contains(indexBody, want) {
			t.Fatalf("index missing risk card %q:\n%s", want, indexBody)
		}
	}
	planBody := mustReadFile(t, out.FirstSlicePath)
	if !strings.Contains(planBody, "Prior checkpoint missed a related test") {
		t.Fatalf("plan missing risk card:\n%s", planBody)
	}

	promptCmd := NewTaskCmd()
	promptCmd.SetArgs([]string{"prompt", "risk-card-test", "--json"})
	promptBuf := &bytes.Buffer{}
	promptCmd.SetOut(promptBuf)
	if err := promptCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var promptOut taskPromptOutput
	if err := json.Unmarshal(promptBuf.Bytes(), &promptOut); err != nil {
		t.Fatalf("prompt json: %v\n%s", err, promptBuf.String())
	}
	for _, want := range []string{
		"Risk cards:",
		"Treat these as evidence-backed checks, not required edit targets.",
		"Prior checkpoint missed a related test",
	} {
		if !strings.Contains(promptOut.Prompt, want) {
			t.Fatalf("prompt missing risk card text %q:\n%s", want, promptOut.Prompt)
		}
	}
	if strings.Index(promptOut.Prompt, "Risk cards:") > strings.Index(promptOut.Prompt, "Target plan:") {
		t.Fatalf("risk cards should appear before target plan:\n%s", promptOut.Prompt)
	}
}

func TestTask_StartSurfacesAdvisoryFilesFromCheckpointFacts(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repoID := taskTestRepoID(t, db, repoDir)
	now := "2026-06-07T00:00:00Z"
	if err := db.UpsertTaskCheckpointFact(store.TaskCheckpointFact{
		RepoID:             repoID,
		TaskID:             "prior-discount-task",
		CheckpointID:       "cp_discount",
		Target:             "P01",
		Series:             "P",
		Stage:              "implemented",
		Decision:           "improve",
		CheckpointPath:     "checkpoints/prior.md",
		CheckpointJSONPath: "checkpoints/prior.json",
		CreatedAt:          now,
		ActualContextJSON:  `{"files_read":["internal/invoice/pricing.go"],"files_edited":["internal/invoice/pricing.go"]}`,
		FeedbackJSON:       `{"critical_missed":["internal/invoice/pricing_test.go"],"distracting_included":["docs/legacy/discount-rounding-notes.md"]}`,
		EvidenceJSON:       `{}`,
		LearningsJSON:      `[{"learning_type":"validation_gap","summary":"discount rounding needed an explicit package test","evidence_refs":["internal/invoice/pricing_test.go"],"applies_to":"internal/invoice","confidence":"high"}]`,
		NextJSON:           `{}`,
		IndexedAt:          now,
	}); err != nil {
		t.Fatal(err)
	}

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "advisory-file-test", "--no-refresh", "--index=false", "--json", "fix discount rounding"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out taskStartOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("task json: %v\n%s", err, buf.String())
	}
	for _, path := range []string{
		"internal/invoice/pricing.go",
		"internal/invoice/pricing_test.go",
		"docs/legacy/discount-rounding-notes.md",
	} {
		if taskAdvisoryFileByPath(out.AdvisoryFiles, path) == nil {
			t.Fatalf("missing advisory file %q: %#v", path, out.AdvisoryFiles)
		}
	}
	if file := taskAdvisoryFileByPath(out.AdvisoryFiles, "internal/invoice/pricing_test.go"); file == nil || file.Kind != "prior-missed-test" {
		t.Fatalf("missed test advisory = %#v", file)
	}

	var manifest taskManifest
	if err := json.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest); err != nil {
		t.Fatalf("manifest json: %v", err)
	}
	if taskAdvisoryFileByPath(manifest.AdvisoryFiles, "internal/invoice/pricing.go") == nil {
		t.Fatalf("manifest missing advisory files: %#v", manifest.AdvisoryFiles)
	}
	planBody := mustReadFile(t, out.FirstSlicePath)
	for _, want := range []string{
		"Checkpoint Leads",
		"not files the initial pack ranked as primary",
		"No pack-ranked primary file. Verify these checkpoint leads",
		"internal/invoice/pricing.go",
		"internal/invoice/pricing_test.go",
	} {
		if !strings.Contains(planBody, want) {
			t.Fatalf("plan missing advisory text %q:\n%s", want, planBody)
		}
	}

	promptCmd := NewTaskCmd()
	promptCmd.SetArgs([]string{"prompt", "advisory-file-test", "--json"})
	promptBuf := &bytes.Buffer{}
	promptCmd.SetOut(promptBuf)
	if err := promptCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var promptOut taskPromptOutput
	if err := json.Unmarshal(promptBuf.Bytes(), &promptOut); err != nil {
		t.Fatalf("prompt json: %v\n%s", err, promptBuf.String())
	}
	for _, want := range []string{
		"Checkpoint leads:",
		"verification leads only",
		"internal/invoice/pricing.go",
		"internal/invoice/pricing_test.go",
	} {
		if !strings.Contains(promptOut.Prompt, want) {
			t.Fatalf("prompt missing advisory text %q:\n%s", want, promptOut.Prompt)
		}
	}
}

func TestTask_AdvisoryFilesAreCappedAndPrioritized(t *testing.T) {
	facts := []store.TaskCheckpointFact{{
		TaskID:            "prior-wide-task",
		CheckpointID:      "cp_wide",
		ActualContextJSON: `{"files_read":["src/a.go","src/b.go"],"files_edited":["src/c.go"],"tests_read":["src/a_test.go"]}`,
		FeedbackJSON:      `{"critical_missed":["src/missed_test.go","src/missed.go"],"distracting_included":["docs/noise-a.md","docs/noise-b.md"]}`,
		LearningsJSON:     `[{"learning_type":"validation_gap","summary":"discount rounding needed package tests","evidence_refs":["src/learned_test.go","src/learned.go"],"applies_to":"src","confidence":"high"}]`,
	}}
	files := taskAdvisoryFilesFromCheckpointFacts("fix discount rounding", taskPredictedContext{}, facts)
	if len(files) > taskAdvisoryFileLimit {
		t.Fatalf("advisory files exceed cap: %d > %d: %#v", len(files), taskAdvisoryFileLimit, files)
	}
	wantKinds := []string{"prior-source", "prior-missed-test", "prior-noise", "prior-missed-file", "prior-test-evidence"}
	if len(files) != len(wantKinds) {
		t.Fatalf("advisory files len = %d, want %d: %#v", len(files), len(wantKinds), files)
	}
	for i, want := range wantKinds {
		if files[i].Kind != want {
			t.Fatalf("advisory kind[%d] = %q, want %q: %#v", i, files[i].Kind, want, files)
		}
	}

	strongPredicted := taskPredictedContext{
		PrimaryFiles: []taskPredictedFile{{Path: "src/main.go"}},
		Tests:        []taskPredictedFile{{Path: "src/main_test.go"}},
	}
	if got := taskAdvisoryFilesFromCheckpointFacts("fix discount rounding", strongPredicted, facts); len(got) != 0 {
		t.Fatalf("strong predicted context should suppress checkpoint leads, got %#v", got)
	}
}

func TestTask_PromptCarriesPriorSliceCheckpointEvidence(t *testing.T) {
	setupTaskCommandRepo(t)
	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "prior-slice-evidence-test",
		"--no-refresh",
		"--index=false",
		"--slice", "trace test companion recall",
		"--slice", "wire test companion recall",
		"improve test companion recall",
	})
	startCmd.SetOut(&bytes.Buffer{})
	if err := startCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "prior-slice-evidence-test",
		"--slice", "A01",
		"--stage", "validated",
		"--decision", "promote",
		"--file-read", "internal/retrieval/ranking.go",
		"--test-read", "internal/retrieval/ranking_test.go",
		"--missed-file", "internal/retrieval/ranking_test.go",
		"--noise-file", ".devspecs/tasks/prior-slice-evidence-test/A00-index.md",
		"--learning", "context_gap|trace found the companion test|high|A01|internal/retrieval/ranking_test.go",
		"--json",
	})
	checkpointCmd.SetOut(&bytes.Buffer{})
	if err := checkpointCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	promptCmd := NewTaskCmd()
	promptCmd.SetArgs([]string{"prompt", "prior-slice-evidence-test", "--target", "A02", "--json"})
	promptBuf := &bytes.Buffer{}
	promptCmd.SetOut(promptBuf)
	if err := promptCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var promptOut taskPromptOutput
	if err := json.Unmarshal(promptBuf.Bytes(), &promptOut); err != nil {
		t.Fatalf("prompt json: %v\n%s", err, promptBuf.String())
	}
	for _, want := range []string{
		"Prior slice evidence:",
		"checkpointed by earlier targets",
		"internal/retrieval/ranking.go",
		"internal/retrieval/ranking_test.go",
	} {
		if !strings.Contains(promptOut.Prompt, want) {
			t.Fatalf("prompt missing prior evidence %q:\n%s", want, promptOut.Prompt)
		}
	}
	if taskAdvisoryFileByPath(promptOut.PriorSliceEvidence, "internal/retrieval/ranking_test.go") == nil {
		t.Fatalf("prompt json missing prior test evidence: %#v", promptOut.PriorSliceEvidence)
	}
	if taskAdvisoryFileByPath(promptOut.PriorSliceEvidence, ".devspecs/tasks/prior-slice-evidence-test/A00-index.md") != nil {
		t.Fatalf("prompt evidence should filter task workspace paths: %#v", promptOut.PriorSliceEvidence)
	}
}

func TestTask_PreflightFiltersTaskWorkspaceCandidates(t *testing.T) {
	got := filterTaskPreflightCandidates([]retrieval.Candidate{
		{Path: ".devspecs/tasks/task-one/A00-index.md"},
		{Path: "devspecs/tasks/task-one/A00-index.md"},
		{Path: "internal/retrieval/ranking.go"},
		{Path: "C:/repo/.devspecs/tasks/task-one/A01-plan.md"},
		{Path: "C:/repo/devspecs/tasks/task-one/A01-plan.md"},
	})
	if len(got) != 1 || got[0].Path != "internal/retrieval/ranking.go" {
		t.Fatalf("filtered candidates = %#v", got)
	}
}

func TestTask_LifecycleAutoDetectsLegacyWorkspace(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--dir", ".devspecs/tasks",
		"--id", "legacy-compat",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "legacy first slice",
		"legacy task compatibility",
	})
	startCmd.SetOut(&bytes.Buffer{})
	if err := startCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	statusCmd := NewTaskCmd()
	statusCmd.SetArgs([]string{"status", "legacy-compat", "--json"})
	statusBuf := &bytes.Buffer{}
	statusCmd.SetOut(statusBuf)
	if err := statusCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var statusOut taskStatusOutput
	if err := json.Unmarshal(statusBuf.Bytes(), &statusOut); err != nil {
		t.Fatalf("status json: %v\n%s", err, statusBuf.String())
	}
	if statusOut.TaskID != "legacy-compat" || len(statusOut.Slices) != 1 {
		t.Fatalf("legacy status output = %#v", statusOut)
	}

	showCmd := NewTaskCmd()
	showCmd.SetArgs([]string{"show", "A01", "--json"})
	showBuf := &bytes.Buffer{}
	showCmd.SetOut(showBuf)
	if err := showCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var showOut taskTargetOutput
	if err := json.Unmarshal(showBuf.Bytes(), &showOut); err != nil {
		t.Fatalf("show json: %v\n%s", err, showBuf.String())
	}
	if showOut.TaskID != "legacy-compat" || showOut.Target != "A01" {
		t.Fatalf("legacy show target output = %#v", showOut)
	}
	if !strings.Contains(filepath.ToSlash(showOut.Workspace), ".devspecs/tasks/legacy-compat") {
		t.Fatalf("legacy workspace was not resolved: %q", showOut.Workspace)
	}

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "legacy-compat",
		"--slice", "A01",
		"--stage", "validated",
		"--decision", "promote",
		"--file-read", "internal/retrieval/ranking.go",
		"--index=false",
		"--json",
	})
	checkpointBuf := &bytes.Buffer{}
	checkpointCmd.SetOut(checkpointBuf)
	if err := checkpointCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var checkpointOut taskCheckpointOutput
	if err := json.Unmarshal(checkpointBuf.Bytes(), &checkpointOut); err != nil {
		t.Fatalf("checkpoint json: %v\n%s", err, checkpointBuf.String())
	}
	if !strings.Contains(filepath.ToSlash(checkpointOut.CheckpointPath), ".devspecs/tasks/legacy-compat") {
		t.Fatalf("legacy checkpoint path was not resolved: %#v", checkpointOut)
	}

	evaluateCmd := NewTaskCmd()
	evaluateCmd.SetArgs([]string{"evaluate", "legacy-compat", "--json"})
	evaluateBuf := &bytes.Buffer{}
	evaluateCmd.SetOut(evaluateBuf)
	if err := evaluateCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var evaluateOut taskEvaluationOutput
	if err := json.Unmarshal(evaluateBuf.Bytes(), &evaluateOut); err != nil {
		t.Fatalf("evaluate json: %v\n%s", err, evaluateBuf.String())
	}
	if evaluateOut.TaskID != "legacy-compat" {
		t.Fatalf("legacy evaluate output = %#v", evaluateOut)
	}

	if _, err := os.Stat(filepath.Join(repoDir, ".devspecs", "tasks", "legacy-compat", taskManifestFilename)); err != nil {
		t.Fatalf("legacy manifest missing: %v", err)
	}
}

func TestTask_StartSkipsUnrelatedCheckpointFactRiskCards(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repoID := taskTestRepoID(t, db, repoDir)
	now := "2026-06-07T00:00:00Z"
	if err := db.UpsertTaskCheckpointFact(store.TaskCheckpointFact{
		RepoID:             repoID,
		TaskID:             "prior-unrelated-task",
		CheckpointID:       "cp_unrelated",
		Target:             "A01",
		Series:             "A",
		Stage:              "implemented",
		Decision:           "improve",
		CheckpointPath:     "checkpoints/unrelated.md",
		CheckpointJSONPath: "checkpoints/unrelated.json",
		CreatedAt:          now,
		ActualContextJSON:  `{}`,
		FeedbackJSON:       `{"critical_missed":["services/billing/webhook_test.go"]}`,
		EvidenceJSON:       `{}`,
		LearningsJSON:      `[]`,
		NextJSON:           `{}`,
		IndexedAt:          now,
	}); err != nil {
		t.Fatal(err)
	}

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "unrelated-risk-card-test", "--no-refresh", "--index=false", "--json", "improve test companion recall"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out taskStartOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("task json: %v\n%s", err, buf.String())
	}
	if taskRiskCardByID(out.RiskCards, "prior-test-miss") != nil || taskRiskCardByID(out.RiskCards, "prior-critical-miss") != nil {
		t.Fatalf("unrelated fact should not create miss risk cards: %#v", out.RiskCards)
	}
}

func TestTask_RiskCardsUseQueryMatchedLearningWhenPredictedContextWeak(t *testing.T) {
	cards := taskRiskCardsFromCheckpointFacts("fix discount rounding", taskPredictedContext{}, []store.TaskCheckpointFact{{
		TaskID:        "prior-discount-task",
		CheckpointID:  "cp_discount",
		FeedbackJSON:  `{"critical_missed":["internal/invoice/pricing_test.go"]}`,
		LearningsJSON: `[{"learning_type":"validation_gap","summary":"discount rounding needed an explicit package test","evidence_refs":["internal/invoice/pricing_test.go"],"applies_to":"internal/invoice","confidence":"high"}]`,
	}})
	if taskRiskCardByID(cards, "prior-test-miss") == nil {
		t.Fatalf("expected query-matched prior-test-miss card, got %#v", cards)
	}
	if taskRiskCardByID(cards, "validation-gap") == nil {
		t.Fatalf("expected validation-gap card, got %#v", cards)
	}

	unrelated := taskRiskCardsFromCheckpointFacts("fix discount rounding", taskPredictedContext{}, []store.TaskCheckpointFact{{
		TaskID:        "prior-billing-task",
		CheckpointID:  "cp_billing",
		FeedbackJSON:  `{"critical_missed":["services/billing/webhook_test.go"]}`,
		LearningsJSON: `[{"learning_type":"validation_gap","summary":"webhook retries needed a package test","evidence_refs":["services/billing/webhook_test.go"],"applies_to":"services/billing","confidence":"high"}]`,
	}})
	if taskRiskCardByID(unrelated, "prior-test-miss") != nil {
		t.Fatalf("unrelated query learning should not create prior-test-miss: %#v", unrelated)
	}
}

func TestTask_StartBootstrapsRepeatedSlices(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "multi-slice-test",
		"--no-refresh",
		"--json",
		"--slice", "scout current workflow",
		"--slice", "tighten checkpoint evidence",
		"task workflow ux",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var out taskStartOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("task json: %v\n%s", err, buf.String())
	}
	if len(out.Slices) != 2 {
		t.Fatalf("expected two slices, got %#v", out.Slices)
	}
	wantPlans := []string{
		"A01-scout-current-workflow-plan.md",
		"A02-tighten-checkpoint-evidence-plan.md",
	}
	wantResults := []string{
		"A01-scout-current-workflow-result.md",
		"A02-tighten-checkpoint-evidence-result.md",
	}
	for i, slice := range out.Slices {
		if filepath.Base(slice.PlanPath) != wantPlans[i] {
			t.Fatalf("slice %d plan = %q", i, slice.PlanPath)
		}
		if filepath.Base(slice.ResultPath) != wantResults[i] {
			t.Fatalf("slice %d result = %q", i, slice.ResultPath)
		}
		for _, path := range []string{slice.PlanPath, slice.ResultPath} {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("expected %s: %v", path, err)
			}
		}
	}
	if filepath.Base(out.FirstSlicePath) != wantPlans[0] || filepath.Base(out.ResultPath) != wantResults[0] {
		t.Fatalf("first slice aliases did not point at A01: %#v", out)
	}

	var manifest taskManifest
	if err := json.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest); err != nil {
		t.Fatalf("manifest json: %v", err)
	}
	if len(manifest.Artifacts.Slices) != 2 {
		t.Fatalf("manifest slices = %#v", manifest.Artifacts.Slices)
	}
	if manifest.Artifacts.FirstSlice != wantPlans[0] || manifest.Artifacts.Result != wantResults[0] {
		t.Fatalf("manifest first aliases = %#v", manifest.Artifacts)
	}
	indexBody := mustReadFile(t, out.IndexPath)
	for _, want := range []string{
		"## Task Slices",
		"A01: scout current workflow",
		"A02: tighten checkpoint evidence",
	} {
		if !strings.Contains(indexBody, want) {
			t.Fatalf("index missing %q:\n%s", want, indexBody)
		}
	}
	if !strings.HasPrefix(out.Workspace, filepath.Join(repoDir, "devspecs", "tasks", "multi-slice-test")) {
		t.Fatalf("workspace = %q", out.Workspace)
	}
}

func TestTask_BoundaryPrimitivesResolveOneTarget(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "boundary-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "first bounded slice",
		"--slice", "second bounded slice",
		"task boundary primitives",
	})
	startCmd.SetOut(&bytes.Buffer{})
	if err := startCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	nextCmd := NewTaskCmd()
	nextCmd.SetArgs([]string{"next", "boundary-test", "--json"})
	nextBuf := &bytes.Buffer{}
	nextCmd.SetOut(nextBuf)
	if err := nextCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var nextOut taskTargetOutput
	if err := json.Unmarshal(nextBuf.Bytes(), &nextOut); err != nil {
		t.Fatalf("next json: %v\n%s", err, nextBuf.String())
	}
	if nextOut.Target != "A01" || !containsString(nextOut.SiblingTargets, "A02") {
		t.Fatalf("next output = %#v", nextOut)
	}

	promptCmd := NewTaskCmd()
	promptCmd.SetArgs([]string{"prompt", "boundary-test", "--json"})
	promptBuf := &bytes.Buffer{}
	promptCmd.SetOut(promptBuf)
	if err := promptCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var promptOut taskPromptOutput
	if err := json.Unmarshal(promptBuf.Bytes(), &promptOut); err != nil {
		t.Fatalf("prompt json: %v\n%s", err, promptBuf.String())
	}
	for _, want := range []string{
		"target A01 only",
		"must_not_implement",
		"- A02",
		"Do not implement sibling slices",
		"Checklist edits are useful notes",
	} {
		if !strings.Contains(promptOut.Prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, promptOut.Prompt)
		}
	}

	startTargetCmd := NewTaskCmd()
	startTargetCmd.SetArgs([]string{"start", "boundary-test", "--index=false", "--json"})
	startTargetBuf := &bytes.Buffer{}
	startTargetCmd.SetOut(startTargetBuf)
	if err := startTargetCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var startTargetOut taskDecideOutput
	if err := json.Unmarshal(startTargetBuf.Bytes(), &startTargetOut); err != nil {
		t.Fatalf("start target json: %v\n%s", err, startTargetBuf.String())
	}
	if startTargetOut.Target != "A01" || startTargetOut.Stage != "started" || startTargetOut.Decision != "continue" {
		t.Fatalf("start target output = %#v", startTargetOut)
	}

	indexPath := filepath.Join(repoDir, "devspecs", "tasks", "boundary-test", "A00-index.md")
	authoredIndexBody := mustReadFile(t, indexPath) + "\n## Human Master Notes\n\nKeep this richer A00 content intact.\n"
	mustWriteFile(t, indexPath, authoredIndexBody)

	finishCmd := NewTaskCmd()
	finishCmd.SetArgs([]string{"finish", "boundary-test", "--decision", "promote", "--index=false", "--json"})
	finishBuf := &bytes.Buffer{}
	finishCmd.SetOut(finishBuf)
	if err := finishCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var finishOut taskDecideOutput
	if err := json.Unmarshal(finishBuf.Bytes(), &finishOut); err != nil {
		t.Fatalf("finish json: %v\n%s", err, finishBuf.String())
	}
	if finishOut.Target != "A01" || finishOut.Stage != "completed" || finishOut.Decision != "promote" {
		t.Fatalf("finish output = %#v", finishOut)
	}
	if got := mustReadFile(t, indexPath); got != authoredIndexBody {
		t.Fatalf("finish rewrote authored task index.\nGot:\n%s\nWant:\n%s", got, authoredIndexBody)
	}

	nextAfterCmd := NewTaskCmd()
	nextAfterCmd.SetArgs([]string{"next", "boundary-test", "--json"})
	nextAfterBuf := &bytes.Buffer{}
	nextAfterCmd.SetOut(nextAfterBuf)
	if err := nextAfterCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var nextAfter taskTargetOutput
	if err := json.Unmarshal(nextAfterBuf.Bytes(), &nextAfter); err != nil {
		t.Fatalf("next after json: %v\n%s", err, nextAfterBuf.String())
	}
	if nextAfter.Target != "A02" {
		t.Fatalf("next after finish = %#v", nextAfter)
	}

	showCmd := NewTaskCmd()
	showCmd.SetArgs([]string{"show", "boundary-test", "--target", "A02", "--json"})
	showBuf := &bytes.Buffer{}
	showCmd.SetOut(showBuf)
	if err := showCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var showOut taskTargetOutput
	if err := json.Unmarshal(showBuf.Bytes(), &showOut); err != nil {
		t.Fatalf("show json: %v\n%s", err, showBuf.String())
	}
	if showOut.Target != "A02" || !strings.Contains(showOut.PlanBody, "second bounded slice") {
		t.Fatalf("show output = %#v", showOut)
	}
}

func TestTask_TargetAddressingResolvesUniqueSlice(t *testing.T) {
	setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "target-address-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "first target slice",
		"--slice", "second target slice",
		"task target addressing",
	})
	startCmd.SetOut(&bytes.Buffer{})
	if err := startCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	showCmd := NewTaskCmd()
	showCmd.SetArgs([]string{"show", "A02", "--json"})
	showBuf := &bytes.Buffer{}
	showCmd.SetOut(showBuf)
	if err := showCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var showOut taskTargetOutput
	if err := json.Unmarshal(showBuf.Bytes(), &showOut); err != nil {
		t.Fatalf("show json: %v\n%s", err, showBuf.String())
	}
	if showOut.TaskID != "target-address-test" || showOut.Target != "A02" {
		t.Fatalf("show resolved wrong target: %#v", showOut)
	}
	if !containsString(showOut.SiblingTargets, "A01") {
		t.Fatalf("show sibling targets = %#v", showOut.SiblingTargets)
	}

	promptCmd := NewTaskCmd()
	promptCmd.SetArgs([]string{"prompt", "A02", "--json"})
	promptBuf := &bytes.Buffer{}
	promptCmd.SetOut(promptBuf)
	if err := promptCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var promptOut taskPromptOutput
	if err := json.Unmarshal(promptBuf.Bytes(), &promptOut); err != nil {
		t.Fatalf("prompt json: %v\n%s", err, promptBuf.String())
	}
	for _, want := range []string{
		"task target-address-test target A02 only",
		"Checklist edits are useful notes",
	} {
		if !strings.Contains(promptOut.Prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, promptOut.Prompt)
		}
	}

	startTargetCmd := NewTaskCmd()
	startTargetCmd.SetArgs([]string{"start", "A02", "--index=false", "--json"})
	startTargetBuf := &bytes.Buffer{}
	startTargetCmd.SetOut(startTargetBuf)
	if err := startTargetCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var startTargetOut taskDecideOutput
	if err := json.Unmarshal(startTargetBuf.Bytes(), &startTargetOut); err != nil {
		t.Fatalf("start json: %v\n%s", err, startTargetBuf.String())
	}
	if startTargetOut.TaskID != "target-address-test" || startTargetOut.Target != "A02" || startTargetOut.Stage != "started" {
		t.Fatalf("start resolved wrong target: %#v", startTargetOut)
	}

	finishCmd := NewTaskCmd()
	finishCmd.SetArgs([]string{"finish", "A02", "--decision", "promote", "--index=false", "--json"})
	finishBuf := &bytes.Buffer{}
	finishCmd.SetOut(finishBuf)
	if err := finishCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var finishOut taskDecideOutput
	if err := json.Unmarshal(finishBuf.Bytes(), &finishOut); err != nil {
		t.Fatalf("finish json: %v\n%s", err, finishBuf.String())
	}
	if finishOut.TaskID != "target-address-test" || finishOut.Target != "A02" || finishOut.Decision != "promote" {
		t.Fatalf("finish resolved wrong target: %#v", finishOut)
	}
}

func TestTask_TargetAddressingRequiresUnambiguousSlice(t *testing.T) {
	setupTaskCommandRepo(t)

	for _, taskID := range []string{"ambiguous-target-a", "ambiguous-target-b"} {
		startCmd := NewTaskCmd()
		startCmd.SetArgs([]string{
			"--id", taskID,
			"--series", "A",
			"--no-refresh",
			"--index=false",
			"--json",
			"--slice", "shared first slice",
			"task target ambiguity",
		})
		startCmd.SetOut(&bytes.Buffer{})
		if err := startCmd.Execute(); err != nil {
			t.Fatal(err)
		}
	}

	showCmd := NewTaskCmd()
	showCmd.SetArgs([]string{"show", "A01", "--json"})
	showCmd.SetOut(&bytes.Buffer{})
	showCmd.SetErr(&bytes.Buffer{})
	err := showCmd.Execute()
	if err == nil {
		t.Fatal("expected ambiguous target error")
	}
	for _, want := range []string{
		"ambiguous task target",
		"ambiguous-target-a:A01",
		"ambiguous-target-b:A01",
		"use a task id with --target",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ambiguous error missing %q: %v", want, err)
		}
	}
}

func TestTask_NextTaskAlphaSeriesRollovers(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: "A"},
		{in: "A", want: "B"},
		{in: "Y", want: "Z"},
		{in: "Z", want: "AA"},
		{in: "AA", want: "AB"},
		{in: "AZ", want: "BA"},
		{in: "ZZ", want: "AAA"},
		{in: "AAA", want: "AAB"},
	}
	for _, tt := range tests {
		if got := nextTaskAlphaSeries(tt.in); got != tt.want {
			t.Fatalf("nextTaskAlphaSeries(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTask_StartAutoIncrementsDefaultSeries(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	writeExistingTaskSeries(t, repoDir, "existing-a", "A")
	writeExistingTaskSeries(t, repoDir, "explicit-r09", "R09")

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "auto-series-b",
		"--no-refresh",
		"--index=false",
		"--json",
		"auto series chooses next alpha",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var out taskStartOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("task json: %v\n%s", err, buf.String())
	}
	if out.Series != "B" {
		t.Fatalf("series = %q", out.Series)
	}
	if filepath.Base(out.IndexPath) != "B00-index.md" {
		t.Fatalf("index path = %q", out.IndexPath)
	}
	if len(out.Slices) != 1 || out.Slices[0].ID != "B01" {
		t.Fatalf("slices = %#v", out.Slices)
	}
}

func TestTask_StartAutoSeriesSeesLegacyWorkspace(t *testing.T) {
	setupTaskCommandRepo(t)

	legacyCmd := NewTaskCmd()
	legacyCmd.SetArgs([]string{
		"--dir", ".devspecs/tasks",
		"--id", "legacy-series-a",
		"--series", "A",
		"--no-refresh",
		"--index=false",
		"legacy series a",
	})
	legacyCmd.SetOut(&bytes.Buffer{})
	if err := legacyCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "visible-series-b",
		"--no-refresh",
		"--index=false",
		"--json",
		"visible series should skip legacy a",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out taskStartOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("task json: %v\n%s", err, buf.String())
	}
	if out.Series != "B" || filepath.Base(out.IndexPath) != "B00-index.md" {
		t.Fatalf("expected B after legacy A, got %#v", out)
	}
}

func TestTask_StartAutoSeriesRollsPastZAndZZ(t *testing.T) {
	t.Run("Z to AA", func(t *testing.T) {
		repoDir := setupTaskCommandRepo(t)
		writeExistingTaskSeriesRange(t, repoDir, "Z")

		cmd := NewTaskCmd()
		cmd.SetArgs([]string{
			"--id", "auto-series-aa",
			"--no-refresh",
			"--index=false",
			"--json",
			"auto series rolls past z",
		})
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}

		var out taskStartOutput
		if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
			t.Fatalf("task json: %v\n%s", err, buf.String())
		}
		if out.Series != "AA" || filepath.Base(out.IndexPath) != "AA00-index.md" {
			t.Fatalf("expected AA rollover, got %#v", out)
		}
		if len(out.Slices) != 1 || out.Slices[0].ID != "AA01" {
			t.Fatalf("slices = %#v", out.Slices)
		}
	})

	t.Run("ZZ to AAA", func(t *testing.T) {
		repoDir := setupTaskCommandRepo(t)
		writeExistingTaskSeriesRange(t, repoDir, "ZZ")

		cmd := NewTaskCmd()
		cmd.SetArgs([]string{
			"--id", "auto-series-aaa",
			"--no-refresh",
			"--index=false",
			"--json",
			"auto series rolls past zz",
		})
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}

		var out taskStartOutput
		if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
			t.Fatalf("task json: %v\n%s", err, buf.String())
		}
		if out.Series != "AAA" || filepath.Base(out.IndexPath) != "AAA00-index.md" {
			t.Fatalf("expected AAA rollover, got %#v", out)
		}
		if len(out.Slices) != 1 || out.Slices[0].ID != "AAA01" {
			t.Fatalf("slices = %#v", out.Slices)
		}
	})
}

func TestTask_StartAutoRefreshesTaskSubstrate(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "task-substrate-refresh-test", "--json", "improve test companion recall"})
	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repoID := taskTestRepoID(t, db, repoDir)
	counts, err := db.CountSourceManifest(repoID)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Files == 0 {
		t.Fatalf("expected task auto-refresh to populate source manifest, got %#v", counts)
	}
	var testCases int
	if err := db.QueryRow("SELECT COUNT(DISTINCT artifact_id) FROM sources WHERE repo_id = ? AND source_type = 'test_case'", repoID).Scan(&testCases); err != nil {
		t.Fatal(err)
	}
	if testCases == 0 {
		t.Fatal("expected task auto-refresh to index test cases")
	}
}

func TestTask_StartGeneratesRequestedSeriesArtifacts(t *testing.T) {
	setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "b-series-test",
		"--series", "b",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "define lifecycle model",
		"--slice", "repair checkpoint state",
		"task workflow ux",
	})
	buf := &bytes.Buffer{}
	startCmd.SetOut(buf)
	if err := startCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var out taskStartOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("task json: %v\n%s", err, buf.String())
	}
	if out.Series != "B" {
		t.Fatalf("series = %q", out.Series)
	}
	if filepath.Base(out.IndexPath) != "B00-index.md" {
		t.Fatalf("index path = %q", out.IndexPath)
	}
	wantPlans := []string{
		"B01-define-lifecycle-model-plan.md",
		"B02-repair-checkpoint-state-plan.md",
	}
	for i, slice := range out.Slices {
		if slice.ID != strings.TrimSuffix(wantPlans[i], "-"+sanitizeTaskFilename(slice.Title)+"-plan.md") {
			t.Fatalf("slice %d id = %q", i, slice.ID)
		}
		if filepath.Base(slice.PlanPath) != wantPlans[i] {
			t.Fatalf("slice %d plan = %q", i, slice.PlanPath)
		}
	}

	var manifest taskManifest
	if err := json.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest); err != nil {
		t.Fatalf("manifest json: %v", err)
	}
	if manifest.Series != "B" || manifest.Artifacts.Series != "B" {
		t.Fatalf("manifest series fields = %#v", manifest)
	}
	if manifest.Artifacts.Index != "B00-index.md" || manifest.Artifacts.FirstSlice != wantPlans[0] {
		t.Fatalf("manifest artifacts = %#v", manifest.Artifacts)
	}
	indexBody := mustReadFile(t, out.IndexPath)
	for _, want := range []string{
		"## Series",
		"B",
		"B01: define lifecycle model",
		"B02: repair checkpoint state",
	} {
		if !strings.Contains(indexBody, want) {
			t.Fatalf("B00 missing %q:\n%s", want, indexBody)
		}
	}
	planBody := mustReadFile(t, out.Slices[0].PlanPath)
	if !strings.Contains(planBody, "`B00-index.md`") {
		t.Fatalf("B01 plan should reference B00 index:\n%s", planBody)
	}

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "b-series-test",
		"--slice", "B02",
		"--stage", "validated",
		"--decision", "complete",
		"--note", "B-series checkpoint",
		"--index=false",
		"--json",
	})
	checkpointBuf := &bytes.Buffer{}
	checkpointCmd.SetOut(checkpointBuf)
	if err := checkpointCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var checkpointOut taskCheckpointOutput
	if err := json.Unmarshal(checkpointBuf.Bytes(), &checkpointOut); err != nil {
		t.Fatalf("checkpoint json: %v\n%s", err, checkpointBuf.String())
	}
	if checkpointOut.Series != "B" || checkpointOut.Slice != "B02" {
		t.Fatalf("checkpoint output = %#v", checkpointOut)
	}
	checkpointBody := mustReadFile(t, checkpointOut.CheckpointPath)
	for _, want := range []string{
		"series: B",
		"slice: B02",
		"`../B00-index.md`",
		"`../B02-repair-checkpoint-state-plan.md`",
	} {
		if !strings.Contains(checkpointBody, want) {
			t.Fatalf("checkpoint missing %q:\n%s", want, checkpointBody)
		}
	}
	var record taskCheckpointRecord
	if err := json.Unmarshal([]byte(mustReadFile(t, checkpointOut.CheckpointJSONPath)), &record); err != nil {
		t.Fatalf("checkpoint record json: %v", err)
	}
	if record.Series != "B" || record.Slice != "B02" || record.Stage != "validated" || record.Decision != "complete" {
		t.Fatalf("checkpoint record = %#v", record)
	}
}

func TestTask_SliceAndIterationAddGenerateLifecycleArtifacts(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "lifecycle-add-test",
		"--series", "B",
		"--no-refresh",
		"--index=false",
		"--slice", "first lifecycle slice",
		"task lifecycle flow",
	})
	startCmd.SetOut(&bytes.Buffer{})
	if err := startCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	sliceCmd := NewTaskCmd()
	sliceCmd.SetArgs([]string{
		"slice", "add", "lifecycle-add-test", "second lifecycle slice",
		"--index=false",
		"--json",
	})
	sliceBuf := &bytes.Buffer{}
	sliceCmd.SetOut(sliceBuf)
	if err := sliceCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var sliceOut taskArtifactAddOutput
	if err := json.Unmarshal(sliceBuf.Bytes(), &sliceOut); err != nil {
		t.Fatalf("slice add json: %v\n%s", err, sliceBuf.String())
	}
	if sliceOut.Series != "B" || sliceOut.Slice.ID != "B02" {
		t.Fatalf("slice add output = %#v", sliceOut)
	}
	if filepath.Base(sliceOut.Slice.PlanPath) != "B02-second-lifecycle-slice-plan.md" {
		t.Fatalf("slice plan = %q", sliceOut.Slice.PlanPath)
	}

	iterationCmd := NewTaskCmd()
	iterationCmd.SetArgs([]string{
		"iteration", "add", "lifecycle-add-test", "repair lifecycle status",
		"--slice", "B01",
		"--reason", "improve",
		"--index=false",
		"--json",
	})
	iterationBuf := &bytes.Buffer{}
	iterationCmd.SetOut(iterationBuf)
	if err := iterationCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var iterationOut taskArtifactAddOutput
	if err := json.Unmarshal(iterationBuf.Bytes(), &iterationOut); err != nil {
		t.Fatalf("iteration add json: %v\n%s", err, iterationBuf.String())
	}
	if iterationOut.Series != "B" || iterationOut.Slice.ID != "B01-1" {
		t.Fatalf("iteration add output = %#v", iterationOut)
	}
	if filepath.Base(iterationOut.Slice.PlanPath) != "B01-1-repair-lifecycle-status-plan.md" {
		t.Fatalf("iteration plan = %q", iterationOut.Slice.PlanPath)
	}

	workspace := filepath.Join(repoDir, "devspecs", "tasks", "lifecycle-add-test")
	indexBody := mustReadFile(t, filepath.Join(workspace, "B00-index.md"))
	for _, want := range []string{
		"B02: second lifecycle slice",
		"B01-1: repair lifecycle status (iteration of B01, reason: improve)",
	} {
		if !strings.Contains(indexBody, want) {
			t.Fatalf("index missing %q:\n%s", want, indexBody)
		}
	}
	indexPath := filepath.Join(workspace, "B00-index.md")
	authoredIndexBody := indexBody + "\n## Human Master Notes\n\nKeep lifecycle state in task.json and result artifacts.\n"
	mustWriteFile(t, indexPath, authoredIndexBody)

	var manifest taskManifest
	if err := json.Unmarshal([]byte(mustReadFile(t, filepath.Join(workspace, taskManifestFilename))), &manifest); err != nil {
		t.Fatalf("manifest json: %v", err)
	}
	if len(manifest.Artifacts.Slices) != 3 {
		t.Fatalf("manifest slices = %#v", manifest.Artifacts.Slices)
	}
	iteration := manifest.Artifacts.Slices[2]
	if iteration.ID != "B01-1" || iteration.Kind != "iteration" || iteration.ParentID != "B01" || iteration.Reason != "improve" {
		t.Fatalf("iteration manifest entry = %#v", iteration)
	}

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "lifecycle-add-test",
		"--slice", "B01-1",
		"--stage", "implemented",
		"--decision", "promote",
		"--index=false",
		"--json",
	})
	checkpointBuf := &bytes.Buffer{}
	checkpointCmd.SetOut(checkpointBuf)
	if err := checkpointCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var checkpointOut taskCheckpointOutput
	if err := json.Unmarshal(checkpointBuf.Bytes(), &checkpointOut); err != nil {
		t.Fatalf("checkpoint json: %v\n%s", err, checkpointBuf.String())
	}
	if checkpointOut.Slice != "B01-1" {
		t.Fatalf("checkpoint output = %#v", checkpointOut)
	}
	if filepath.Base(checkpointOut.ResultPath) != "B01-1-repair-lifecycle-status-result.md" {
		t.Fatalf("checkpoint result path = %q", checkpointOut.ResultPath)
	}

	statusCmd := NewTaskCmd()
	statusCmd.SetArgs([]string{
		"status", "lifecycle-add-test",
		"--json",
	})
	statusBuf := &bytes.Buffer{}
	statusCmd.SetOut(statusBuf)
	if err := statusCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var statusOut taskStatusOutput
	if err := json.Unmarshal(statusBuf.Bytes(), &statusOut); err != nil {
		t.Fatalf("status json: %v\n%s", err, statusBuf.String())
	}
	if statusOut.Series != "B" || statusOut.Status != "packed" {
		t.Fatalf("status output = %#v", statusOut)
	}
	var promoted taskStatusSliceOutput
	for _, slice := range statusOut.Slices {
		if slice.ID == "B01-1" {
			promoted = slice
			break
		}
	}
	if promoted.ID == "" || promoted.Stage != "implemented" || promoted.Decision != "promote" || promoted.UpdatedAt == "" {
		t.Fatalf("promoted iteration status = %#v", promoted)
	}
	if !strings.HasPrefix(promoted.LatestCheckpoint, "checkpoints/") || !strings.HasSuffix(promoted.LatestCheckpoint, "-implemented.md") {
		t.Fatalf("promoted iteration checkpoint = %#v", promoted)
	}
	if !strings.HasPrefix(promoted.LatestCheckpointJSON, "checkpoints/") || !strings.HasSuffix(promoted.LatestCheckpointJSON, "-implemented.json") {
		t.Fatalf("promoted iteration checkpoint json = %#v", promoted)
	}

	var updatedManifest taskManifest
	if err := json.Unmarshal([]byte(mustReadFile(t, filepath.Join(workspace, taskManifestFilename))), &updatedManifest); err != nil {
		t.Fatalf("updated manifest json: %v", err)
	}
	if updatedManifest.UpdatedAt == "" {
		t.Fatalf("manifest updated_at was empty: %#v", updatedManifest)
	}
	var manifestIteration taskSliceArtifact
	for _, slice := range updatedManifest.Artifacts.Slices {
		if slice.ID == "B01-1" {
			manifestIteration = slice
			break
		}
	}
	if manifestIteration.Stage != "implemented" || manifestIteration.Decision != "promote" || manifestIteration.UpdatedAt == "" {
		t.Fatalf("manifest iteration state = %#v", manifestIteration)
	}
	if manifestIteration.LatestCheckpoint != promoted.LatestCheckpoint || manifestIteration.LatestCheckpointJSON != promoted.LatestCheckpointJSON {
		t.Fatalf("manifest iteration checkpoint refs = %#v", manifestIteration)
	}

	if got := mustReadFile(t, indexPath); got != authoredIndexBody {
		t.Fatalf("checkpoint rewrote authored task index.\nGot:\n%s\nWant:\n%s", got, authoredIndexBody)
	}

	decideSliceCmd := NewTaskCmd()
	decideSliceCmd.SetArgs([]string{
		"decide", "lifecycle-add-test",
		"--target", "B01",
		"--decision", "complete",
		"--index=false",
		"--json",
	})
	decideSliceBuf := &bytes.Buffer{}
	decideSliceCmd.SetOut(decideSliceBuf)
	if err := decideSliceCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var decideSliceOut taskDecideOutput
	if err := json.Unmarshal(decideSliceBuf.Bytes(), &decideSliceOut); err != nil {
		t.Fatalf("slice decide json: %v\n%s", err, decideSliceBuf.String())
	}
	if decideSliceOut.Target != "B01" || decideSliceOut.Stage != "completed" || decideSliceOut.Decision != "complete" {
		t.Fatalf("slice decide output = %#v", decideSliceOut)
	}

	decideSeriesCmd := NewTaskCmd()
	decideSeriesCmd.SetArgs([]string{
		"decide", "lifecycle-add-test",
		"--target", "B00",
		"--decision", "complete",
		"--index=false",
		"--json",
	})
	decideSeriesBuf := &bytes.Buffer{}
	decideSeriesCmd.SetOut(decideSeriesBuf)
	if err := decideSeriesCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var decideSeriesOut taskDecideOutput
	if err := json.Unmarshal(decideSeriesBuf.Bytes(), &decideSeriesOut); err != nil {
		t.Fatalf("series decide json: %v\n%s", err, decideSeriesBuf.String())
	}
	if decideSeriesOut.Target != "B00" || decideSeriesOut.Stage != "completed" || decideSeriesOut.Decision != "complete" {
		t.Fatalf("series decide output = %#v", decideSeriesOut)
	}

	decidedStatusCmd := NewTaskCmd()
	decidedStatusCmd.SetArgs([]string{
		"status", "lifecycle-add-test",
		"--json",
	})
	decidedStatusBuf := &bytes.Buffer{}
	decidedStatusCmd.SetOut(decidedStatusBuf)
	if err := decidedStatusCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var decidedStatus taskStatusOutput
	if err := json.Unmarshal(decidedStatusBuf.Bytes(), &decidedStatus); err != nil {
		t.Fatalf("decided status json: %v\n%s", err, decidedStatusBuf.String())
	}
	if decidedStatus.Status != "completed" || decidedStatus.Decision != "complete" {
		t.Fatalf("decided series status = %#v", decidedStatus)
	}
	var completedSlice taskStatusSliceOutput
	for _, slice := range decidedStatus.Slices {
		if slice.ID == "B01" {
			completedSlice = slice
			break
		}
	}
	if completedSlice.Stage != "completed" || completedSlice.Decision != "complete" {
		t.Fatalf("completed slice status = %#v", completedSlice)
	}

	if got := mustReadFile(t, indexPath); got != authoredIndexBody {
		t.Fatalf("decide rewrote authored task index.\nGot:\n%s\nWant:\n%s", got, authoredIndexBody)
	}
}

func TestTask_StartWarnsAboutOnDiskAnchorMissingFromIndex(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	stalePath := filepath.Join(repoDir, "internal", "retrieval", "companion_recall_new.go")
	mustWriteFile(t, stalePath, `package retrieval

func ImproveCompanionRecallNew() {}
`)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "freshness-warning-test", "--no-refresh", "--json", "improve companion recall"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var out taskStartOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("task json: %v\n%s", err, buf.String())
	}
	if !taskWarningsContainPath(out.FreshnessWarnings, "internal/retrieval/companion_recall_new.go") {
		t.Fatalf("expected freshness warning for stale on-disk anchor, got %#v", out.FreshnessWarnings)
	}
	if len(out.FreshnessWarnings) > taskFreshnessMaxWarnings {
		t.Fatalf("freshness warnings were not capped: %#v", out.FreshnessWarnings)
	}
	staleCard := taskRiskCardByID(out.RiskCards, "stale-index")
	if staleCard == nil || !strings.Contains(strings.Join(staleCard.Evidence, "\n"), "internal/retrieval/companion_recall_new.go") {
		t.Fatalf("expected stale-index risk card, got %#v", out.RiskCards)
	}
	if staleCard.Title != "On-disk paths matched the task but were not indexed" {
		t.Fatalf("stale-index title = %q", staleCard.Title)
	}

	indexBody := mustReadFile(t, out.IndexPath)
	for _, want := range []string{
		"## Freshness Warnings",
		"## Risk Cards",
		"internal/retrieval/companion_recall_new.go",
		"On-disk paths matched the task but were not indexed",
	} {
		if !strings.Contains(indexBody, want) {
			t.Fatalf("A00 missing freshness warning %q:\n%s", want, indexBody)
		}
	}

	var manifest taskManifest
	if err := json.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest); err != nil {
		t.Fatalf("manifest json: %v", err)
	}
	if !taskWarningsContainPath(manifest.FreshnessWarnings, "internal/retrieval/companion_recall_new.go") {
		t.Fatalf("manifest missing freshness warning: %#v", manifest.FreshnessWarnings)
	}
}

func TestTask_StatusWarnsAndSyncRecapturesEditedArtifacts(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "sync-freshness-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"improve test companion recall",
	})
	startBuf := &bytes.Buffer{}
	startCmd.SetOut(startBuf)
	if err := startCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var startOut taskStartOutput
	if err := json.Unmarshal(startBuf.Bytes(), &startOut); err != nil {
		t.Fatalf("start json: %v\n%s", err, startBuf.String())
	}

	manifest, err := readTaskManifest(startOut.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest.UpdatedAt = "2026-01-01T00:00:00Z"
	if err := writeTaskManifest(startOut.ManifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, startOut.FirstSlicePath, mustReadFile(t, startOut.FirstSlicePath)+"\n\n## Dogfood Notes\n- edited after task creation\n")

	statusCmd := NewTaskCmd()
	statusCmd.SetArgs([]string{"status", "sync-freshness-test", "--json"})
	statusBuf := &bytes.Buffer{}
	statusCmd.SetOut(statusBuf)
	if err := statusCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var statusOut taskStatusOutput
	if err := json.Unmarshal(statusBuf.Bytes(), &statusOut); err != nil {
		t.Fatalf("status json: %v\n%s", err, statusBuf.String())
	}
	if !taskArtifactFreshnessContainsPath(statusOut.ArtifactFreshness, "A01-improve-test-companion-recall-plan.md") {
		t.Fatalf("expected stale plan warning, got %#v", statusOut.ArtifactFreshness)
	}

	syncCmd := NewTaskCmd()
	syncCmd.SetArgs([]string{"sync", "sync-freshness-test", "--json"})
	syncBuf := &bytes.Buffer{}
	syncCmd.SetOut(syncBuf)
	if err := syncCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var syncOut taskSyncOutput
	if err := json.Unmarshal(syncBuf.Bytes(), &syncOut); err != nil {
		t.Fatalf("sync json: %v\n%s", err, syncBuf.String())
	}
	if syncOut.TaskID != "sync-freshness-test" || syncOut.ManifestPath == "" {
		t.Fatalf("sync output = %#v", syncOut)
	}
	for _, want := range []string{
		"devspecs/tasks/sync-freshness-test/A00-index.md",
		"devspecs/tasks/sync-freshness-test/A01-improve-test-companion-recall-plan.md",
		"devspecs/tasks/sync-freshness-test/A01-improve-test-companion-recall-result.md",
	} {
		if !containsPath(syncOut.IndexedPaths, want) {
			t.Fatalf("sync indexed paths missing %q: %#v", want, syncOut.IndexedPaths)
		}
	}
	if !taskArtifactFreshnessContainsPath(syncOut.ArtifactFreshness, "A01-improve-test-companion-recall-plan.md") {
		t.Fatalf("sync should report what it freshened, got %#v", syncOut.ArtifactFreshness)
	}

	afterManifest, err := readTaskManifest(startOut.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if afterManifest.UpdatedAt == "" || afterManifest.UpdatedAt == "2026-01-01T00:00:00Z" {
		t.Fatalf("sync did not update manifest timestamp: %#v", afterManifest)
	}

	afterStatusCmd := NewTaskCmd()
	afterStatusCmd.SetArgs([]string{"status", "sync-freshness-test", "--json"})
	afterStatusBuf := &bytes.Buffer{}
	afterStatusCmd.SetOut(afterStatusBuf)
	if err := afterStatusCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var afterStatus taskStatusOutput
	if err := json.Unmarshal(afterStatusBuf.Bytes(), &afterStatus); err != nil {
		t.Fatalf("after status json: %v\n%s", err, afterStatusBuf.String())
	}
	if len(afterStatus.ArtifactFreshness) != 0 {
		t.Fatalf("expected sync to clear stale warnings, got %#v", afterStatus.ArtifactFreshness)
	}

	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	artifacts, err := db.ListArtifacts(store.FilterParams{RepoRoot: repoDir, SourceType: "capture"})
	if err != nil {
		t.Fatal(err)
	}
	foundResult := false
	for _, art := range artifacts {
		if strings.Contains(art.Title, "sync-freshness-test A01 result") {
			foundResult = true
			break
		}
	}
	if !foundResult {
		t.Fatalf("sync did not capture result artifact: %#v", artifacts)
	}
}

func TestTask_AuditReportsPassAndDrift(t *testing.T) {
	setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "audit-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"improve test companion recall",
	})
	startCmd.SetOut(&bytes.Buffer{})
	if err := startCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	passCheckpointCmd := NewTaskCmd()
	passCheckpointCmd.SetArgs([]string{
		"checkpoint", "audit-test",
		"--stage", "implemented",
		"--decision", "continue",
		"--file-edited", "internal/retrieval/ranking.go",
		"--file-edited", "internal/retrieval/ranking_test.go",
		"--index=false",
		"--json",
	})
	passCheckpointCmd.SetOut(&bytes.Buffer{})
	if err := passCheckpointCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	auditCmd := NewTaskCmd()
	auditCmd.SetArgs([]string{"audit", "audit-test", "--target", "A01", "--json"})
	auditBuf := &bytes.Buffer{}
	auditCmd.SetOut(auditBuf)
	if err := auditCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var auditOut taskAuditOutput
	if err := json.Unmarshal(auditBuf.Bytes(), &auditOut); err != nil {
		t.Fatalf("audit json: %v\n%s", err, auditBuf.String())
	}
	if auditOut.Recommendation != "pass" || len(auditOut.OutOfScopePaths) != 0 {
		t.Fatalf("expected pass audit, got %#v", auditOut)
	}
	if !containsPath(auditOut.InScopePaths, "internal/retrieval/ranking.go") {
		t.Fatalf("audit missing in-scope source: %#v", auditOut.InScopePaths)
	}
	if !containsPath(auditOut.InScopePaths, "internal/retrieval/ranking_test.go") {
		t.Fatalf("audit missing in-scope test: %#v", auditOut.InScopePaths)
	}

	driftCheckpointCmd := NewTaskCmd()
	driftCheckpointCmd.SetArgs([]string{
		"checkpoint", "audit-test",
		"--stage", "implemented",
		"--decision", "continue",
		"--file-edited", "internal/other/unrelated.go",
		"--index=false",
		"--json",
	})
	driftCheckpointCmd.SetOut(&bytes.Buffer{})
	if err := driftCheckpointCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	driftAuditCmd := NewTaskCmd()
	driftAuditCmd.SetArgs([]string{"audit", "audit-test", "--target", "A01", "--json"})
	driftAuditBuf := &bytes.Buffer{}
	driftAuditCmd.SetOut(driftAuditBuf)
	if err := driftAuditCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var driftOut taskAuditOutput
	if err := json.Unmarshal(driftAuditBuf.Bytes(), &driftOut); err != nil {
		t.Fatalf("drift audit json: %v\n%s", err, driftAuditBuf.String())
	}
	if driftOut.Recommendation != "drift" || !containsPath(driftOut.OutOfScopePaths, "internal/other/unrelated.go") {
		t.Fatalf("expected drift audit, got %#v", driftOut)
	}
}

func TestTask_StartUsesGitWorktreeRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	mainRepo := filepath.Join(tmp, "main")
	worktree := filepath.Join(tmp, "linked")

	if err := taskGitCmd("init", "-b", "main", mainRepo).Run(); err != nil {
		t.Fatal(err)
	}
	mustMkdirAll(t, filepath.Join(mainRepo, ".devspecs"))
	mustWriteFile(t, filepath.Join(mainRepo, ".devspecs", "config.yaml"), `version: 1
sources:
  - type: source_context
`)
	mustWriteFile(t, filepath.Join(mainRepo, "go.mod"), "module example.com/worktree\n")
	mustMkdirAll(t, filepath.Join(mainRepo, "internal", "taskroot"))
	mustWriteFile(t, filepath.Join(mainRepo, "internal", "taskroot", "root.go"), `package taskroot

func RootTask() {}
`)
	if err := taskGitCmd("-C", mainRepo, "add", ".").Run(); err != nil {
		t.Fatal(err)
	}
	if err := taskGitCmd("-C", mainRepo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "init").Run(); err != nil {
		t.Fatal(err)
	}
	if err := taskGitCmd("-C", mainRepo, "worktree", "add", "-b", "linked-branch", worktree).Run(); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(worktree, "internal", "taskroot")
	origWd, _ := os.Getwd()
	if err := os.Chdir(subdir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origWd) })

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--quiet"})
	scanCmd.SetOut(&bytes.Buffer{})
	if err := scanCmd.Execute(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "worktree-root-test", "--no-refresh", "--json", "taskroot"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out taskStartOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("task json: %v\n%s", err, buf.String())
	}
	wantPrefix := filepath.Join(worktree, "devspecs", "tasks", "worktree-root-test")
	if !strings.HasPrefix(out.Workspace, wantPrefix) {
		t.Fatalf("workspace = %q, want prefix %q", out.Workspace, wantPrefix)
	}
	if strings.HasPrefix(out.Workspace, filepath.Join(mainRepo, "devspecs")) {
		t.Fatalf("workspace used main repo instead of worktree: %q", out.Workspace)
	}
}

func TestTask_CheckpointAppendsResultAndIndexesCheckpoint(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{"--id", "checkpoint-test", "--no-refresh", "improve test companion recall"})
	startCmd.SetOut(&bytes.Buffer{})
	if err := startCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "checkpoint-test",
		"--stage", "implemented",
		"--decision", "improve",
		"--note", "found a missing same-package test",
		"--file-read", "internal/retrieval/ranking.go",
		"--file-edited", "internal/retrieval/ranking.go",
		"--test-read", "internal/retrieval/ranking_test.go",
		"--test-run", "go test ./internal/retrieval",
		"--missed-file", "internal/retrieval/ranking_test.go",
		"--noise-file", "fixtures/noisy-plan.md",
		"--learning", "retrieval|same-package tests are important rescue evidence|high|A01|internal/retrieval/ranking_test.go",
		"--next-target", "A01-1",
		"--next-decision", "improve",
		"--json",
	})
	buf := &bytes.Buffer{}
	checkpointCmd.SetOut(buf)
	if err := checkpointCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out taskCheckpointOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("checkpoint json: %v\n%s", err, buf.String())
	}
	if out.Stage != "implemented" || out.Decision != "improve" {
		t.Fatalf("unexpected checkpoint output: %#v", out)
	}
	if out.Slice != "A01" {
		t.Fatalf("checkpoint output slice = %q", out.Slice)
	}
	if out.CheckpointJSONPath == "" {
		t.Fatalf("expected structured checkpoint path in output: %#v", out)
	}
	if out.CheckpointID == "" || out.LearningCount != 1 || !out.FactIndexed {
		t.Fatalf("expected checkpoint id, learning count, and indexed fact in output: %#v", out)
	}
	checkpointBody := mustReadFile(t, out.CheckpointPath)
	for _, want := range []string{
		"---",
		"schema_version: 2",
		"checkpoint_id:",
		"target: A01",
		"slice: A01",
		"parent_slice: A01",
		"stage: implemented",
		"decision: improve",
		"created_at:",
		"checkpoint_json:",
		"## Structured Evidence",
		"Checkpoint ID:",
		"## Files Actually Read",
		"`internal/retrieval/ranking.go`",
		"## Critical Files DevSpecs Missed",
		"`internal/retrieval/ranking_test.go`",
		"## Distracting Files DevSpecs Included",
		"`fixtures/noisy-plan.md`",
	} {
		if !strings.Contains(checkpointBody, want) {
			t.Fatalf("checkpoint missing %q:\n%s", want, checkpointBody)
		}
	}
	for _, unwanted := range []string{"\n## Stage\n", "\n## Decision\n", "\n## Created At\n"} {
		if strings.Contains(checkpointBody, unwanted) {
			t.Fatalf("checkpoint should keep metadata heading %q in frontmatter, not body:\n%s", unwanted, checkpointBody)
		}
	}
	var record taskCheckpointRecord
	if err := json.Unmarshal([]byte(mustReadFile(t, out.CheckpointJSONPath)), &record); err != nil {
		t.Fatalf("checkpoint record json: %v", err)
	}
	if record.TaskID != "checkpoint-test" || record.Stage != "implemented" || record.Decision != "improve" {
		t.Fatalf("unexpected checkpoint record: %#v", record)
	}
	if record.Slice != "A01" {
		t.Fatalf("checkpoint record slice = %q", record.Slice)
	}
	if record.SchemaVersion != 2 {
		t.Fatalf("checkpoint record schema version = %d", record.SchemaVersion)
	}
	if record.CheckpointID != out.CheckpointID || record.Target != "A01" || record.ParentSlice != "A01" {
		t.Fatalf("checkpoint record identity = %#v", record)
	}
	if !containsPath(record.FilesEdited, "internal/retrieval/ranking.go") {
		t.Fatalf("checkpoint record missing edited file: %#v", record.FilesEdited)
	}
	if !containsPath(record.ActualContext.FilesEdited, "internal/retrieval/ranking.go") {
		t.Fatalf("checkpoint record missing actual context edited file: %#v", record.ActualContext)
	}
	if !containsPath(record.MissedFiles, "internal/retrieval/ranking_test.go") {
		t.Fatalf("checkpoint record missing missed file: %#v", record.MissedFiles)
	}
	if !containsPath(record.PredictedContextFeedback.CriticalMissed, "internal/retrieval/ranking_test.go") {
		t.Fatalf("checkpoint record missing predicted feedback: %#v", record.PredictedContextFeedback)
	}
	if len(record.Learnings) != 1 || !strings.Contains(record.Learnings[0].Summary, "same-package tests") {
		t.Fatalf("checkpoint record learnings = %#v", record.Learnings)
	}
	if record.Next.RecommendedTarget != "A01-1" || record.Next.RecommendedDecision != "improve" {
		t.Fatalf("checkpoint next recommendation = %#v", record.Next)
	}
	resultBody := mustReadFile(t, out.ResultPath)
	for _, want := range []string{
		"### Checkpoint",
		"Stage: implemented",
		"Decision: improve",
		"Structured Evidence:",
		"Missed files:",
		"`internal/retrieval/ranking_test.go`",
	} {
		if !strings.Contains(resultBody, want) {
			t.Fatalf("result missing %q:\n%s", want, resultBody)
		}
	}

	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	artifacts, err := db.ListArtifacts(store.FilterParams{RepoRoot: repoDir, SourceType: "capture"})
	if err != nil {
		t.Fatal(err)
	}
	foundCheckpoint := false
	for _, art := range artifacts {
		if strings.Contains(art.Title, "checkpoint-test checkpoint implemented") {
			foundCheckpoint = true
			if art.Status != "implemented" {
				t.Fatalf("checkpoint status = %q", art.Status)
			}
		}
	}
	if !foundCheckpoint {
		t.Fatalf("checkpoint capture artifact not found in %#v", artifacts)
	}
	var repoID string
	if err := db.QueryRow("SELECT id FROM repos WHERE root_path = ?", repoDir).Scan(&repoID); err != nil {
		t.Fatal(err)
	}
	facts, err := db.ListTaskCheckpointFacts(repoID, "checkpoint-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("checkpoint facts = %#v", facts)
	}
	if facts[0].CheckpointID != out.CheckpointID || facts[0].Target != "A01" || facts[0].Stage != "implemented" {
		t.Fatalf("checkpoint fact identity = %#v", facts[0])
	}
	if !strings.Contains(facts[0].ActualContextJSON, "internal/retrieval/ranking.go") {
		t.Fatalf("checkpoint fact actual context = %s", facts[0].ActualContextJSON)
	}
	if !strings.Contains(facts[0].LearningsJSON, "same-package tests") {
		t.Fatalf("checkpoint fact learnings = %s", facts[0].LearningsJSON)
	}

	if err := os.WriteFile(out.CheckpointPath, []byte("# scrubbed markdown checkpoint\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	evalCmd := NewTaskCmd()
	evalCmd.SetArgs([]string{"evaluate", "checkpoint-test", "--json"})
	evalBuf := &bytes.Buffer{}
	evalCmd.SetOut(evalBuf)
	if err := evalCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var evalOut taskEvaluationOutput
	if err := json.Unmarshal(evalBuf.Bytes(), &evalOut); err != nil {
		t.Fatalf("evaluate json: %v\n%s", err, evalBuf.String())
	}
	if evalOut.TaskID != "checkpoint-test" {
		t.Fatalf("evaluation task id = %q", evalOut.TaskID)
	}
	if !containsPath(evalOut.Hits, "internal/retrieval/ranking_test.go") {
		t.Fatalf("expected shared pack assembly to count test as a hit, got %#v", evalOut)
	}
	if containsPath(evalOut.Misses, "internal/retrieval/ranking_test.go") {
		t.Fatalf("predicted test should not remain an evaluation miss, got %#v", evalOut.Misses)
	}
	if evalOut.Metrics.TestCompanionRecall != "1/1" {
		t.Fatalf("test companion recall = %q", evalOut.Metrics.TestCompanionRecall)
	}
	if !containsPath(evalOut.Noise, "fixtures/noisy-plan.md") {
		t.Fatalf("expected noise file in evaluation, got %#v", evalOut.Noise)
	}
	if evalOut.CheckpointSummary.JSONRecords != 1 || evalOut.CheckpointSummary.MarkdownFallbacks != 0 {
		t.Fatalf("expected JSON checkpoint read summary, got %#v", evalOut.CheckpointSummary)
	}
}

func TestTask_CheckpointTargetsSelectedSlice(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "slice-target-test",
		"--no-refresh",
		"--index=false",
		"--slice", "first pass",
		"--slice", "second pass",
		"improve test companion recall",
	})
	startCmd.SetOut(&bytes.Buffer{})
	if err := startCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(repoDir, "devspecs", "tasks", "slice-target-test")
	firstResult := filepath.Join(workspace, "A01-first-pass-result.md")
	secondResult := filepath.Join(workspace, "A02-second-pass-result.md")

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "slice-target-test",
		"--slice", "A02",
		"--stage", "implemented",
		"--decision", "promote",
		"--note", "targeted checkpoint",
		"--file-read", "internal/retrieval/ranking.go",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	checkpointCmd.SetOut(buf)
	if err := checkpointCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out taskCheckpointOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("checkpoint json: %v\n%s", err, buf.String())
	}
	if out.Slice != "A02" {
		t.Fatalf("checkpoint output slice = %q", out.Slice)
	}
	if filepath.Base(out.ResultPath) != "A02-second-pass-result.md" {
		t.Fatalf("checkpoint result path = %q", out.ResultPath)
	}
	firstBody := mustReadFile(t, firstResult)
	if strings.Contains(firstBody, "targeted checkpoint") {
		t.Fatalf("first slice result should not receive A02 checkpoint:\n%s", firstBody)
	}
	secondBody := mustReadFile(t, secondResult)
	for _, want := range []string{
		"targeted checkpoint",
		"Stage: implemented",
		"Decision: promote",
		"`internal/retrieval/ranking.go`",
	} {
		if !strings.Contains(secondBody, want) {
			t.Fatalf("second slice result missing %q:\n%s", want, secondBody)
		}
	}
	checkpointBody := mustReadFile(t, out.CheckpointPath)
	for _, want := range []string{
		"slice: A02",
		"`../A02-second-pass-plan.md`",
		"`../A02-second-pass-result.md`",
	} {
		if !strings.Contains(checkpointBody, want) {
			t.Fatalf("checkpoint body missing %q:\n%s", want, checkpointBody)
		}
	}
	var record taskCheckpointRecord
	if err := json.Unmarshal([]byte(mustReadFile(t, out.CheckpointJSONPath)), &record); err != nil {
		t.Fatalf("checkpoint record json: %v", err)
	}
	if record.Slice != "A02" || record.SliceTitle != "second pass" {
		t.Fatalf("unexpected checkpoint record slice fields: %#v", record)
	}
}

func TestTask_EvaluateReportsStructuredEvidenceWithoutInflatingActualContext(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	taskID := "json-evidence-test"
	workspace := filepath.Join(repoDir, "devspecs", "tasks", taskID)
	mustMkdirAll(t, filepath.Join(workspace, "checkpoints"))
	manifest := taskManifest{
		TaskID:    taskID,
		Query:     "structured checkpoint evidence",
		Status:    "packed",
		CreatedAt: "2026-06-04T00:00:00Z",
		RepoRoot:  repoDir,
		Workspace: filepath.ToSlash(workspace),
		Artifacts: taskArtifactPaths{
			Index:      "A00-index.md",
			FirstSlice: "A01-first-slice.md",
			Result:     "A01-1-result.md",
		},
		Predicted: taskPredictedContext{
			PrimaryFiles: []taskPredictedFile{{Path: "internal/retrieval/ranking.go"}},
			Tests:        []taskPredictedFile{{Path: "internal/retrieval/ranking_test.go"}},
		},
		Confidence: taskConfidence{
			PrimaryFileConfidence:  "medium",
			TestCoverageConfidence: "medium",
			DocsConfigConfidence:   "low",
			GitReceiptConfidence:   "low",
			NoiseRisk:              "low",
			PackCompleteness:       "medium",
		},
	}
	if err := writeTaskManifest(filepath.Join(workspace, taskManifestFilename), manifest); err != nil {
		t.Fatal(err)
	}
	record := taskCheckpointRecord{
		SchemaVersion: 1,
		TaskID:        taskID,
		Query:         manifest.Query,
		Stage:         "implemented",
		Decision:      "improve",
		CreatedAt:     "2026-06-04T00:00:01Z",
		FilesRead:     []string{"internal/retrieval/ranking.go"},
		TestsRead:     []string{"internal/retrieval/ranking_test.go"},
		MissedFiles:   []string{"internal/commands/task_evaluate.go"},
		NoiseFiles:    []string{"fixtures/noisy-plan.md"},
		Evidence: taskCheckpointEvidence{
			GitDiff: &taskGitDiffEvidence{
				Command:      "git diff --stat -- .; git diff --name-only -- .",
				ChangedFiles: []string{"internal/commands/task.go"},
				MaxBytes:     12000,
			},
			TestCommands: []taskCommandRunEvidence{{
				Command:  "go test ./internal/retrieval -run TestImproveTestCompanionRecall -count=1",
				ExitCode: 0,
				Output:   "ok",
				MaxBytes: 12000,
			}},
		},
	}
	if err := writeTaskCheckpointRecord(filepath.Join(workspace, "checkpoints", "20260604-000001-implemented.json"), record); err != nil {
		t.Fatal(err)
	}

	evalCmd := NewTaskCmd()
	evalCmd.SetArgs([]string{"evaluate", taskID, "--json"})
	evalBuf := &bytes.Buffer{}
	evalCmd.SetOut(evalBuf)
	if err := evalCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var evalOut taskEvaluationOutput
	if err := json.Unmarshal(evalBuf.Bytes(), &evalOut); err != nil {
		t.Fatalf("evaluate json: %v\n%s", err, evalBuf.String())
	}
	if !containsPath(evalOut.Hits, "internal/retrieval/ranking.go") {
		t.Fatalf("expected predicted source hit, got %#v", evalOut.Hits)
	}
	if containsPath(evalOut.Misses, "internal/commands/task.go") {
		t.Fatalf("git diff evidence should not inflate actual misses, got %#v", evalOut.Misses)
	}
	if !containsPath(evalOut.Observed.GitDiffFiles, "internal/commands/task.go") {
		t.Fatalf("expected git diff evidence in observed context, got %#v", evalOut.Observed.GitDiffFiles)
	}
	if !containsPath(evalOut.Misses, "internal/commands/task_evaluate.go") {
		t.Fatalf("expected explicit JSON missed file, got %#v", evalOut.Misses)
	}
	if !containsPath(evalOut.Noise, "fixtures/noisy-plan.md") {
		t.Fatalf("expected explicit JSON noise file, got %#v", evalOut.Noise)
	}
	if !containsString(evalOut.Observed.TestCommands, "go test ./internal/retrieval -run TestImproveTestCompanionRecall -count=1") {
		t.Fatalf("expected structured test command evidence, got %#v", evalOut.Observed.TestCommands)
	}
	if containsString(evalOut.Observed.TestsRun, "go test ./internal/retrieval -run TestImproveTestCompanionRecall -count=1") {
		t.Fatalf("test command receipt should stay evidence-only unless recorded as actual context, got %#v", evalOut.Observed.TestsRun)
	}
	if !containsPath(evalOut.CheckpointSummary.EvidenceOnlyGitDiffFiles, "internal/commands/task.go") {
		t.Fatalf("expected evidence-only git diff summary, got %#v", evalOut.CheckpointSummary)
	}
	if !containsString(evalOut.CheckpointSummary.EvidenceOnlyTestCommands, "go test ./internal/retrieval -run TestImproveTestCompanionRecall -count=1") {
		t.Fatalf("expected evidence-only test command summary, got %#v", evalOut.CheckpointSummary)
	}
	if evalOut.CheckpointSummary.JSONRecords != 1 || evalOut.CheckpointSummary.MarkdownFallbacks != 0 {
		t.Fatalf("expected structured checkpoint summary, got %#v", evalOut.CheckpointSummary)
	}
}

func TestTask_GitChangedFileParserIgnoresWarnings(t *testing.T) {
	files := appendTaskGitChangedFiles(nil, strings.Join([]string{
		"warning: in the working copy of 'internal/retrieval/ranking.go', LF will be replaced by CRLF the next time Git touches it",
		"internal/retrieval/ranking.go",
		"",
		".devspecs/tasks/p02/P00-index.md",
	}, "\n"))
	if containsPath(files, "warning: in the working copy of 'internal/retrieval/ranking.go', LF will be replaced by CRLF the next time Git touches it") {
		t.Fatalf("warning line should not be parsed as changed file: %#v", files)
	}
	if !containsPath(files, "internal/retrieval/ranking.go") {
		t.Fatalf("expected real changed file, got %#v", files)
	}
	if !containsPath(files, ".devspecs/tasks/p02/P00-index.md") {
		t.Fatalf("expected task artifact changed file, got %#v", files)
	}
}

func TestTask_EvaluateExcludesTaskWorkspaceReadsFromMissMetrics(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	taskID := "workspace-filter-test"
	workspace := filepath.Join(repoDir, ".devspecs", "tasks", taskID)
	mustMkdirAll(t, filepath.Join(workspace, "checkpoints"))
	manifest := taskManifest{
		TaskID:    taskID,
		Query:     "workspace filtering",
		Status:    "packed",
		CreatedAt: "2026-06-04T00:00:00Z",
		RepoRoot:  repoDir,
		Workspace: filepath.ToSlash(workspace),
		Artifacts: taskArtifactPaths{
			Index:      "A00-index.md",
			FirstSlice: "A01-workspace-filter-plan.md",
			Result:     "A01-workspace-filter-result.md",
			Slices: []taskSliceArtifact{{
				ID:     "A01",
				Title:  "workspace filter",
				Plan:   "A01-workspace-filter-plan.md",
				Result: "A01-workspace-filter-result.md",
			}},
		},
		Predicted: taskPredictedContext{
			PrimaryFiles: []taskPredictedFile{{Path: "internal/commands/task.go"}},
		},
		Confidence: taskConfidence{
			PrimaryFileConfidence:  "high",
			TestCoverageConfidence: "low",
			DocsConfigConfidence:   "low",
			GitReceiptConfidence:   "low",
			NoiseRisk:              "low",
			PackCompleteness:       "high",
		},
	}
	if err := writeTaskManifest(filepath.Join(workspace, taskManifestFilename), manifest); err != nil {
		t.Fatal(err)
	}
	workspaceIndex := filepath.ToSlash(filepath.Join(".devspecs", "tasks", taskID, "A00-index.md"))
	workspacePlan := filepath.Join(workspace, "A01-workspace-filter-plan.md")
	workspaceJSON := filepath.Join(workspace, taskManifestFilename)
	record := taskCheckpointRecord{
		SchemaVersion: 1,
		TaskID:        taskID,
		Query:         manifest.Query,
		Stage:         "implemented",
		Decision:      "improve",
		CreatedAt:     "2026-06-04T00:00:01Z",
		FilesRead: []string{
			workspaceIndex,
			workspaceJSON,
			"internal/commands/task.go",
		},
		TestsRead: []string{
			workspacePlan,
		},
		MissedFiles: []string{
			workspaceIndex,
			"A01-workspace-filter-plan.md",
			"internal/commands/task_evaluate.go",
		},
	}
	if err := writeTaskCheckpointRecord(filepath.Join(workspace, "checkpoints", "20260604-000001-implemented.json"), record); err != nil {
		t.Fatal(err)
	}

	evalCmd := NewTaskCmd()
	evalCmd.SetArgs([]string{"evaluate", taskID, "--dir", ".devspecs/tasks", "--json"})
	evalBuf := &bytes.Buffer{}
	evalCmd.SetOut(evalBuf)
	if err := evalCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var evalOut taskEvaluationOutput
	if err := json.Unmarshal(evalBuf.Bytes(), &evalOut); err != nil {
		t.Fatalf("evaluate json: %v\n%s", err, evalBuf.String())
	}
	if !containsPath(evalOut.Observed.FilesRead, workspaceIndex) {
		t.Fatalf("raw observed context should keep task workspace read, got %#v", evalOut.Observed.FilesRead)
	}
	if !containsPath(evalOut.Observed.FilesRead, workspaceJSON) {
		t.Fatalf("raw observed context should keep absolute task workspace read, got %#v", evalOut.Observed.FilesRead)
	}
	if containsPath(evalOut.Misses, workspaceIndex) {
		t.Fatalf("task workspace read should not become a miss: %#v", evalOut.Misses)
	}
	if containsPath(evalOut.Misses, workspacePlan) || containsPath(evalOut.Misses, "A01-workspace-filter-plan.md") {
		t.Fatalf("task workspace plan should not become a miss: %#v", evalOut.Misses)
	}
	if !containsPath(evalOut.Hits, "internal/commands/task.go") {
		t.Fatalf("normal implementation file should still count as hit, got %#v", evalOut.Hits)
	}
	if !containsPath(evalOut.Misses, "internal/commands/task_evaluate.go") {
		t.Fatalf("normal explicit missed file should still count, got %#v", evalOut.Misses)
	}
	if evalOut.Metrics.CriticalPathRecall != "1/1" {
		t.Fatalf("critical path recall should ignore task workspace reads, got %q", evalOut.Metrics.CriticalPathRecall)
	}
	if !evalOut.ConfidenceMismatch {
		t.Fatalf("normal miss should still drive confidence mismatch when initial completeness is high")
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeExistingTaskSeriesRange(t *testing.T, repoDir, last string) {
	t.Helper()
	for series := "A"; ; series = nextTaskAlphaSeries(series) {
		writeExistingTaskSeries(t, repoDir, "existing-series-"+strings.ToLower(series), series)
		if series == last {
			return
		}
		if series == "" || len(series) > 4 {
			t.Fatalf("series range did not reach %q", last)
		}
	}
}

func writeExistingTaskSeries(t *testing.T, repoDir, taskID, series string) {
	t.Helper()
	workspace := filepath.Join(repoDir, "devspecs", "tasks", taskID)
	mustMkdirAll(t, workspace)
	manifest := taskManifest{
		TaskID:    taskID,
		Series:    series,
		Query:     "existing task series " + series,
		Status:    "packed",
		CreatedAt: "2026-06-09T00:00:00Z",
		RepoRoot:  repoDir,
		Workspace: filepath.ToSlash(workspace),
		Artifacts: taskArtifactPaths{
			Series: series,
			Index:  taskSeriesIndexFilename(series),
			Slices: []taskSliceArtifact{
				taskSliceArtifactWithSlug(series+"01", "existing slice", "existing-slice", "slice", "", ""),
			},
		},
	}
	if err := writeTaskManifest(filepath.Join(workspace, taskManifestFilename), manifest); err != nil {
		t.Fatal(err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func taskRiskCardByID(cards []taskRiskCard, id string) *taskRiskCard {
	for i := range cards {
		if cards[i].ID == id {
			return &cards[i]
		}
	}
	return nil
}

func taskAdvisoryFileByPath(files []taskAdvisoryFile, path string) *taskAdvisoryFile {
	path = filepath.ToSlash(path)
	for i := range files {
		if filepath.ToSlash(files[i].Path) == path {
			return &files[i]
		}
	}
	return nil
}

func taskTestRepoID(t *testing.T, db *store.DB, repoDir string) string {
	t.Helper()
	var repoID string
	if err := db.QueryRow("SELECT id FROM repos WHERE root_path = ?", repoDir).Scan(&repoID); err != nil {
		t.Fatal(err)
	}
	return repoID
}

func taskWarningsContainPath(warnings []taskFreshnessWarning, want string) bool {
	want = filepath.ToSlash(want)
	for _, warning := range warnings {
		if filepath.ToSlash(warning.Path) == want {
			return true
		}
	}
	return false
}

func taskArtifactFreshnessContainsPath(warnings []taskArtifactFreshness, want string) bool {
	want = filepath.ToSlash(want)
	for _, warning := range warnings {
		if filepath.ToSlash(warning.Path) == want {
			return true
		}
	}
	return false
}
