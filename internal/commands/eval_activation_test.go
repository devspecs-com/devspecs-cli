package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEvalActivationMatrixUpdateAndCompare(t *testing.T) {
	skinnyRepo := setupActivationMatrixRepo(t, "credentials", "feat: credentials rotation context")
	fatRepo := setupActivationMatrixRepo(t, "billing", "feat: billing analytics export")
	manifest := writeActivationMatrixManifest(t, skinnyRepo, fatRepo)
	goldenDir := filepath.Join(t.TempDir(), "goldens")

	updateCmd := NewEvalCmd()
	updateCmd.SetArgs([]string{
		manifest,
		"--activation-matrix",
		"--activation-profile", "skinny",
		"--activation-golden-dir", goldenDir,
		"--activation-update",
		"--json",
	})
	updateBuf := &bytes.Buffer{}
	updateCmd.SetOut(updateBuf)
	if err := updateCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var update activationMatrixResult
	if err := json.Unmarshal(updateBuf.Bytes(), &update); err != nil {
		t.Fatalf("activation update JSON: %v\n%s", err, updateBuf.String())
	}
	if update.Summary.Total != 3 || update.Summary.Updated != 3 {
		t.Fatalf("unexpected update summary: %#v", update.Summary)
	}
	if strings.Contains(readActivationGolden(t, update.Cases[0].GoldenPath), filepath.ToSlash(skinnyRepo)) {
		t.Fatalf("golden output leaked repo path: %s", update.Cases[0].GoldenPath)
	}
	seenGoldens := map[string]bool{}
	for _, c := range update.Cases {
		if seenGoldens[c.GoldenPath] {
			t.Fatalf("activation matrix reused golden path for repeated command: %s", c.GoldenPath)
		}
		seenGoldens[c.GoldenPath] = true
	}

	compareCmd := NewEvalCmd()
	compareCmd.SetArgs([]string{
		manifest,
		"--activation-matrix",
		"--activation-profile", "skinny",
		"--activation-golden-dir", goldenDir,
		"--json",
	})
	compareBuf := &bytes.Buffer{}
	compareCmd.SetOut(compareBuf)
	if err := compareCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var compare activationMatrixResult
	if err := json.Unmarshal(compareBuf.Bytes(), &compare); err != nil {
		t.Fatalf("activation compare JSON: %v\n%s", err, compareBuf.String())
	}
	if compare.Summary.Total != 3 || compare.Summary.Passed != 3 {
		t.Fatalf("unexpected compare summary: %#v", compare.Summary)
	}
	for _, c := range compare.Cases {
		if c.Profile != "skinny" || c.Status != "passed" {
			t.Fatalf("unexpected compare case: %#v", c)
		}
		if len(c.Args) == 0 || !containsActivationArg(c.Args, "--json") || !containsActivationArg(c.Args, "--quiet") || !containsActivationArg(c.Args, "--path") {
			t.Fatalf("activation args did not enforce quiet JSON path contract: %#v", c.Args)
		}
		joinedArgs := strings.Join(c.Args, "\x00")
		if strings.Contains(joinedArgs, "--json=false") || strings.Contains(joinedArgs, "--quiet=false") || strings.Contains(joinedArgs, "ignored") {
			t.Fatalf("activation args kept manifest values that should be overridden: %#v", c.Args)
		}
	}
}

func TestEvalActivationMatrixProfileFilteringAndMissingGoldens(t *testing.T) {
	skinnyRepo := setupActivationMatrixRepo(t, "credentials", "feat: credentials rotation context")
	fatRepo := setupActivationMatrixRepo(t, "billing", "feat: billing analytics export")
	manifest := writeActivationMatrixManifest(t, skinnyRepo, fatRepo)
	goldenDir := filepath.Join(t.TempDir(), "goldens")

	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		manifest,
		"--activation-matrix",
		"--activation-profile", "fat",
		"--activation-golden-dir", goldenDir,
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "activation matrix regressions failed") {
		t.Fatalf("expected missing fat goldens to fail, got %v", err)
	}
	var result activationMatrixResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("activation missing JSON: %v\n%s", err, buf.String())
	}
	if result.Summary.Total != 1 || result.Summary.Missing != 1 {
		t.Fatalf("fat profile should select only one missing command, got %#v", result.Summary)
	}
	if result.Cases[0].RepoID != "fat-repo" || result.Cases[0].Command != "map" {
		t.Fatalf("unexpected fat case: %#v", result.Cases[0])
	}
}

func TestEvalActivationMatrixAcceptsUTF8BOMJSONManifest(t *testing.T) {
	skinnyRepo := setupActivationMatrixRepo(t, "credentials", "feat: credentials rotation context")
	manifestPath := filepath.Join(t.TempDir(), "activation-matrix.json")
	manifest := activationMatrixManifest{
		Version: 1,
		Repos: []activationMatrixRepo{{
			ID:       "skinny-repo",
			Path:     filepath.ToSlash(skinnyRepo),
			Profiles: []string{"skinny"},
			Commands: []activationMatrixCommand{{
				Name: "map",
				Args: []string{"--max-areas", "3"},
			}},
		}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	data = append([]byte{0xEF, 0xBB, 0xBF}, data...)
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		manifestPath,
		"--activation-matrix",
		"--activation-profile", "skinny",
		"--activation-golden-dir", filepath.Join(t.TempDir(), "goldens"),
		"--activation-update",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result activationMatrixResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("activation BOM JSON: %v\n%s", err, buf.String())
	}
	if result.Summary.Total != 1 || result.Summary.Updated != 1 {
		t.Fatalf("unexpected BOM manifest summary: %#v", result.Summary)
	}
}

func TestEvalActivationMatrixResultMetadataAndResultDir(t *testing.T) {
	skinnyRepo := setupActivationMatrixRepo(t, "credentials", "feat: credentials rotation context")
	fatRepo := setupActivationMatrixRepo(t, "billing", "feat: billing analytics export")
	manifest := writeActivationMatrixManifest(t, skinnyRepo, fatRepo)
	goldenDir := filepath.Join(t.TempDir(), "goldens")
	resultDir := filepath.Join(t.TempDir(), "results")

	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		manifest,
		"--activation-matrix",
		"--activation-profile", "skinny",
		"--activation-clone-mode", "full",
		"--activation-golden-dir", goldenDir,
		"--activation-result-dir", resultDir,
		"--activation-update",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result activationMatrixResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("activation result metadata JSON: %v\n%s", err, buf.String())
	}
	if result.CloneMode != "full" || result.RunnerMode != activationMatrixRunnerGolden || result.NormalizationVersion == "" {
		t.Fatalf("activation metadata missing clone/runner/normalization: %#v", result)
	}
	if result.IndexState != activationIndexStateCold {
		t.Fatalf("default activation index state = %q", result.IndexState)
	}
	if result.ResultDir != filepath.ToSlash(resultDir) || result.ResultPath == "" {
		t.Fatalf("activation result dir/path not recorded: %#v", result)
	}
	if result.RepoSet.Total != 1 || result.RepoSet.Full != 1 || result.RepoSet.Shallow != 0 {
		t.Fatalf("activation repo set metadata = %#v", result.RepoSet)
	}
	if result.Timing.Command.Total <= 0 || result.Timing.Command.P50 <= 0 || result.Timing.Command.Max <= 0 {
		t.Fatalf("activation timing missing command stats: %#v", result.Timing)
	}
	if len(result.SlowestCases) == 0 || result.SlowestCases[0].RepoID == "" || result.SlowestCases[0].DurationMillis <= 0 {
		t.Fatalf("activation slowest case metadata missing: %#v", result.SlowestCases)
	}
	if _, err := os.Stat(filepath.Join(resultDir, activationMatrixDefaultResultFilename)); err != nil {
		t.Fatalf("activation result file not written: %v", err)
	}
	_ = fatRepo
}

func TestEvalActivationMatrixWarmIndexState(t *testing.T) {
	skinnyRepo := setupActivationMatrixRepo(t, "credentials", "feat: credentials rotation context")
	manifest := writeActivationMatrixManifest(t, skinnyRepo, setupActivationMatrixRepo(t, "billing", "feat: billing analytics export"))
	goldenDir := filepath.Join(t.TempDir(), "goldens")

	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		manifest,
		"--activation-matrix",
		"--activation-profile", "skinny",
		"--activation-index-state", "warm",
		"--activation-golden-dir", goldenDir,
		"--activation-update",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result activationMatrixResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("activation warm JSON: %v\n%s", err, buf.String())
	}
	if result.IndexState != activationIndexStateWarm {
		t.Fatalf("result index state = %q", result.IndexState)
	}
	for _, c := range result.Cases {
		if c.IndexState != activationIndexStateWarm {
			t.Fatalf("case index state = %q in %#v", c.IndexState, c)
		}
	}
}

func TestEvalActivationMatrixBinaryCompare(t *testing.T) {
	skinnyRepo := setupActivationMatrixRepo(t, "credentials", "feat: credentials rotation context")
	fatRepo := setupActivationMatrixRepo(t, "billing", "feat: billing analytics export")
	manifest := writeActivationMatrixManifest(t, skinnyRepo, fatRepo)
	resultDir := filepath.Join(t.TempDir(), "results")
	binary := writeActivationFakeBinary(t)

	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		manifest,
		"--activation-matrix",
		"--activation-profile", "skinny",
		"--activation-clone-mode", "full",
		"--activation-baseline-bin", binary,
		"--activation-candidate-bin", binary,
		"--activation-result-dir", resultDir,
		"--activation-quiet=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result activationMatrixResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("activation binary compare JSON: %v\n%s", err, buf.String())
	}
	if result.RunnerMode != activationMatrixRunnerBinaryComparison {
		t.Fatalf("expected binary comparison runner, got %q", result.RunnerMode)
	}
	if result.BaselineIndexState != activationIndexStateCold || result.CandidateIndexState != activationIndexStateCold {
		t.Fatalf("default binary index states missing: baseline=%q candidate=%q", result.BaselineIndexState, result.CandidateIndexState)
	}
	if result.Summary.Total != 3 || result.Summary.Passed != 3 {
		t.Fatalf("unexpected binary compare summary: %#v", result.Summary)
	}
	if result.BaselineBin == "" || result.CandidateBin == "" || result.BaselineBinaryID == "" || result.CandidateBinaryID == "" {
		t.Fatalf("binary metadata missing: %#v", result)
	}
	if len(result.SlowestCases) == 0 || result.SlowestCases[0].BaselineMillis <= 0 || result.SlowestCases[0].CandidateMillis <= 0 {
		t.Fatalf("binary slowest case metadata missing: %#v", result.SlowestCases)
	}
	for _, c := range result.Cases {
		if c.Baseline == nil || c.Candidate == nil || !c.StdoutMatch {
			t.Fatalf("binary comparison run metadata missing: %#v", c)
		}
		if len(c.Baseline.Args) == 0 || c.Baseline.Args[0] != c.Command {
			t.Fatalf("baseline args should include command name: %#v", c.Baseline.Args)
		}
		if c.Baseline.StdoutPath == "" || c.Candidate.StdoutPath == "" {
			t.Fatalf("binary outputs were not written: %#v", c)
		}
	}
	_ = fatRepo
}

func TestEvalActivationMatrixBinaryCompareRecordsDifferentIndexStates(t *testing.T) {
	skinnyRepo := setupActivationMatrixRepo(t, "credentials", "feat: credentials rotation context")
	manifest := writeActivationMatrixManifest(t, skinnyRepo, setupActivationMatrixRepo(t, "billing", "feat: billing analytics export"))
	resultDir := filepath.Join(t.TempDir(), "results")
	binary := writeActivationFakeBinary(t)

	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		manifest,
		"--activation-matrix",
		"--activation-profile", "skinny",
		"--activation-baseline-bin", binary,
		"--activation-candidate-bin", binary,
		"--activation-baseline-index-state", "cold",
		"--activation-candidate-index-state", "warm",
		"--activation-result-dir", resultDir,
		"--activation-quiet=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result activationMatrixResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("activation binary state JSON: %v\n%s", err, buf.String())
	}
	if result.BaselineIndexState != activationIndexStateCold || result.CandidateIndexState != activationIndexStateWarm {
		t.Fatalf("binary index states = baseline %q candidate %q", result.BaselineIndexState, result.CandidateIndexState)
	}
	for _, c := range result.Cases {
		if c.Baseline == nil || c.Candidate == nil {
			t.Fatalf("missing binary runs: %#v", c)
		}
		if c.Baseline.IndexState != activationIndexStateCold || c.Candidate.IndexState != activationIndexStateWarm {
			t.Fatalf("case index states = baseline %q candidate %q", c.Baseline.IndexState, c.Candidate.IndexState)
		}
		if c.Candidate.WarmupMillis < 0 {
			t.Fatalf("warmup millis should be non-negative: %#v", c.Candidate)
		}
	}
}

func TestEvalActivationScanBenchmarkCapturesPhaseAndDBStats(t *testing.T) {
	repo := setupActivationMatrixRepo(t, "billing", "feat: billing analytics export")
	manifest := writeActivationScanBenchmarkManifest(t, repo)
	resultDir := filepath.Join(t.TempDir(), "results")

	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		manifest,
		"--activation-scan-benchmark",
		"--activation-profile", "fat",
		"--activation-result-dir", resultDir,
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result activationScanBenchmarkResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("activation scan benchmark JSON: %v\n%s", err, buf.String())
	}
	if result.Schema != activationScanBenchmarkSchemaVersion {
		t.Fatalf("schema = %q", result.Schema)
	}
	if result.Summary.Total != 1 || result.Summary.Passed != 1 {
		t.Fatalf("unexpected summary: %#v", result.Summary)
	}
	if result.Timing.Max <= 0 || len(result.PhaseTiming) == 0 {
		t.Fatalf("missing aggregate timing: %#v phases=%#v", result.Timing, result.PhaseTiming)
	}
	c := result.Cases[0]
	if c.Command != "scan" || !containsActivationArg(c.Args, "--phase-timing") || !containsActivationArg(c.Args, "--quiet") || !containsActivationArg(c.Args, "--path") {
		t.Fatalf("scan benchmark args not forced: %#v", c.Args)
	}
	if c.DBSizeBytes <= 0 || c.TableRows["artifacts"] <= 0 || c.TableGroups["artifacts"] <= 0 {
		t.Fatalf("missing DB stats: size=%d rows=%#v groups=%#v", c.DBSizeBytes, c.TableRows, c.TableGroups)
	}
	if c.Found["markdown"] <= 0 || c.PhaseTiming == nil || len(c.PhaseTiming.Phases) == 0 {
		t.Fatalf("missing scan summary/phase timing: %#v", c)
	}
	if c.SourceManifest == nil || !c.SourceManifest.Enabled || c.SourceManifest.IndexedFiles <= 0 {
		t.Fatalf("missing source manifest summary: %#v", c.SourceManifest)
	}
	if c.StdoutPath == "" || c.StderrPath == "" {
		t.Fatalf("expected retained stdout/stderr paths: %#v", c)
	}
	if _, err := os.Stat(filepath.Join(resultDir, activationScanBenchmarkResultName)); err != nil {
		t.Fatalf("benchmark result file not written: %v", err)
	}
}

func TestActivationScanBenchmarkThresholdsFailDurationRegression(t *testing.T) {
	current := activationScanBenchmarkCase{
		RepoID:         "repo",
		Command:        "scan",
		Args:           []string{"--json", "--quiet", "--phase-timing", "--path", "/repo"},
		IndexState:     activationIndexStateCold,
		Status:         "passed",
		DurationMillis: 200,
		DBSizeBytes:    120,
		TableGroups:    map[string]int{"artifacts": 7, "source_manifest": 3},
	}
	baselineCase := current
	baselineCase.DurationMillis = 100
	baselineCase.DBSizeBytes = 90
	baselineCase.TableGroups = map[string]int{"artifacts": 5, "source_manifest": 3}
	baseline := map[string]activationScanBenchmarkCase{
		activationScanCaseKey(baselineCase): baselineCase,
	}

	applyActivationScanBenchmarkThresholds(&current, baseline, activationScanBenchmarkOptions{
		MaxRegressionRatio: 1.1,
	})

	if current.Status != "failed" || current.Regression == nil || current.Regression.Status != "failed" {
		t.Fatalf("expected regression failure, got %#v", current)
	}
	if current.Regression.DurationDeltaMS != 100 || current.Regression.DBSizeDeltaBytes != 30 {
		t.Fatalf("unexpected regression deltas: %#v", current.Regression)
	}
	if got := current.Regression.TableGroupDeltas["artifacts"]; got != 2 {
		t.Fatalf("artifact table group delta = %d, want 2", got)
	}
}

func TestActivationMapActionQualityAcceptsNonActionDeltas(t *testing.T) {
	baseline := activationMapActionQualityFixture(`ds find "layout panel talking"`)
	candidate := activationMapActionQualityFixture(`ds find "layout panel talking"`)
	baseline.Caveats = []string{mapIndexRequiredCaveat}
	candidate.Caveats = nil
	baseline.Diagnostics.RawClusterCount = 12
	candidate.Diagnostics.RawClusterCount = 13
	baseline.Areas[0].BoundaryPaths = []string{"components/**", "app/**"}
	candidate.Areas[0].BoundaryPaths = []string{"app/**", "components/**"}
	baseline.Areas[0].TraceReceipts = []mapTraceReceipt{{SHA: "aaa1111", Subject: "feat: app shell"}, {SHA: "bbb2222", Subject: "feat: component panels"}}
	candidate.Areas[0].TraceReceipts = []mapTraceReceipt{{SHA: "bbb2222", Subject: "feat: component panels"}, {SHA: "aaa1111", Subject: "feat: app shell"}}
	baseline.Areas[0].Diagnostics.RawAnchors = []string{"components", "app"}
	candidate.Areas[0].Diagnostics.RawAnchors = []string{"app", "components"}
	baseline.Areas[0].Diagnostics.Packability.IndexedQueryAnchorCount = 2
	candidate.Areas[0].Diagnostics.Packability.IndexedQueryAnchorCount = 3

	comparison := compareActivationMapActionQuality(mustActivationMapJSON(t, baseline), mustActivationMapJSON(t, candidate))
	if !comparison.Accepted {
		t.Fatalf("expected non-action map deltas to be accepted: %#v", comparison)
	}
	if comparison.Status != "accepted_non_action_deltas" {
		t.Fatalf("expected accepted non-action status, got %#v", comparison)
	}
	if len(comparison.ActionDiffs) != 0 || len(comparison.AcceptedDiffs) == 0 {
		t.Fatalf("unexpected comparison detail: %#v", comparison)
	}
}

func TestActivationMapActionQualityRejectsTryDelta(t *testing.T) {
	baseline := activationMapActionQualityFixture(`ds find "website ia pivot homepage"`)
	candidate := activationMapActionQualityFixture(`ds find "website ia pivot homepage"`)
	candidate.Areas[0].Try = `ds find "launch website skeleton"`

	comparison := compareActivationMapActionQuality(mustActivationMapJSON(t, baseline), mustActivationMapJSON(t, candidate))
	if comparison.Accepted {
		t.Fatalf("expected try delta to be rejected: %#v", comparison)
	}
	if !strings.Contains(strings.Join(comparison.ActionDiffs, "\n"), "areas[0].try") {
		t.Fatalf("expected try diff to be reported: %#v", comparison)
	}
}

func TestActivationMapActionQualityAcceptsSuppressedTryDiagnosticDelta(t *testing.T) {
	baseline := activationMapActionQualityFixture("")
	candidate := activationMapActionQualityFixture("")
	baseline.Areas[0].Diagnostics.Packability = &mapPackabilityDiagnostics{
		KeyPathCount:        2,
		Decision:            "suppressed_no_indexed_support",
		SuppressedTry:       `ds find "llms txt markdown links"`,
		SuppressedTrySource: "trace_task",
		TrySuppressed:       true,
	}
	candidate.Areas[0].Diagnostics.Packability = &mapPackabilityDiagnostics{
		KeyPathCount:        2,
		Decision:            "suppressed_no_indexed_support",
		SuppressedTry:       `ds find "llms robots"`,
		SuppressedTrySource: "path",
		TrySuppressed:       true,
	}

	comparison := compareActivationMapActionQuality(mustActivationMapJSON(t, baseline), mustActivationMapJSON(t, candidate))
	if !comparison.Accepted {
		t.Fatalf("expected suppressed diagnostic delta to be accepted: %#v", comparison)
	}
	if len(comparison.ActionDiffs) != 0 || len(comparison.AcceptedDiffs) == 0 {
		t.Fatalf("unexpected comparison detail: %#v", comparison)
	}
}

func TestActivationMapActionQualityRejectsKeyPathDelta(t *testing.T) {
	baseline := activationMapActionQualityFixture(`ds find "layout panel talking"`)
	candidate := activationMapActionQualityFixture(`ds find "layout panel talking"`)
	candidate.Areas[0].KeyPaths = []string{"app/page.tsx", "components/other-panel.tsx"}

	comparison := compareActivationMapActionQuality(mustActivationMapJSON(t, baseline), mustActivationMapJSON(t, candidate))
	if comparison.Accepted {
		t.Fatalf("expected key path delta to be rejected: %#v", comparison)
	}
	if !strings.Contains(strings.Join(comparison.ActionDiffs, "\n"), "areas[0].key_paths") {
		t.Fatalf("expected key path diff to be reported: %#v", comparison)
	}
}

func activationMapActionQualityFixture(tryCommand string) mapOutput {
	return mapOutput{
		Schema: "devspecs.map.v1",
		Repo: mapRepo{
			Name:       "fixture",
			Path:       "<REPO_ROOT>",
			Confidence: "high",
		},
		EvidenceAvailability: mapEvidenceAvailability{
			Markdown: 2,
			Source:   5,
			Test:     1,
			Trace:    true,
		},
		Areas: []mapArea{{
			ID:             "area_layout",
			Label:          "Layout",
			Class:          "workstream",
			AreaType:       "source",
			BoundaryRole:   mapBoundaryRoleProductCapability,
			Purpose:        "Layout activation",
			BoundaryPaths:  []string{"app/**", "components/**"},
			Confidence:     "high",
			Covers:         []string{"app", "components"},
			EvidenceCounts: map[string]int{"source": 4, "test": 1, "trace": 2},
			KeyPaths:       []string{"app/page.tsx", "components/panel.tsx"},
			TraceReceipts:  []mapTraceReceipt{{SHA: "aaa1111", Subject: "feat: app shell"}, {SHA: "bbb2222", Subject: "feat: component panels"}},
			Try:            tryCommand,
			Diagnostics: mapAreaDiagnostics{
				Key:              "layout",
				RawAnchors:       []string{"app", "components"},
				LabelEvidence:    []string{"app/page.tsx", "components/panel.tsx"},
				TraceTerms:       []string{"layout", "panel"},
				TraceReceiptMode: "recent",
				Packability: &mapPackabilityDiagnostics{
					KeyPathCount:            2,
					IndexedKeyPathCount:     2,
					IndexedQueryAnchorCount: 2,
					Decision:                "supported",
					SelectedTrySource:       "path",
				},
			},
		}},
		Diagnostics: mapDiagnostics{
			RawClusterCount:  12,
			MatchedAreaCount: 1,
		},
	}
}

func mustActivationMapJSON(t *testing.T, output mapOutput) []byte {
	t.Helper()
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func setupActivationMatrixRepo(t *testing.T, topic, subject string) string {
	t.Helper()
	repoRoot := setupGitRepo(t)
	writeMapTestFile(t, repoRoot, "docs/plans/"+topic+".md", "# "+topic+" plan\n\nThis plan covers "+topic+" activation.\n")
	writeMapTestFile(t, repoRoot, "internal/"+topic+"/"+topic+".go", "package "+topic+"\n\nfunc Run() {}\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", subject)
	return repoRoot
}

func writeActivationMatrixManifest(t *testing.T, skinnyRepo, fatRepo string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "activation-matrix.yaml")
	body := fmt.Sprintf(`version: 1
repos:
  - id: skinny-repo
    path: %q
    profiles: [skinny]
    commands:
      - name: recent
        args: ["credentials", "--max-areas", "3"]
      - name: recent
        args: ["rotation", "--json=false", "--quiet=false", "--path", "ignored", "--max-areas", "3"]
      - name: map
        args: ["--max-areas", "3"]
  - id: fat-repo
    path: %q
    profiles: [fat]
    commands:
      - name: map
        args: ["--max-areas", "3"]
`, filepath.ToSlash(skinnyRepo), filepath.ToSlash(fatRepo))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeActivationScanBenchmarkManifest(t *testing.T, repo string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "activation-scan-benchmark.yaml")
	body := fmt.Sprintf(`version: 1
suite_id: activation-scan-benchmark-test
suite_version: test
repos:
  - id: fat-repo
    path: %q
    profiles: [fat]
    commands:
      - name: scan
        args: ["--include-tests", "--experimental-source-manifest"]
`, filepath.ToSlash(repo))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

type evalRegressionSetRegistry struct {
	Version int                     `yaml:"version"`
	Sets    []evalRegressionSetSpec `yaml:"sets"`
}

type evalRegressionSetSpec struct {
	ID               string   `yaml:"id"`
	Status           string   `yaml:"status"`
	Tier             string   `yaml:"tier"`
	RepoCount        int      `yaml:"repo_count"`
	MinimumRepoCount int      `yaml:"minimum_repo_count"`
	CanonicalDir     string   `yaml:"canonical_dir"`
	PrimaryManifest  string   `yaml:"primary_manifest"`
	LatestGoodResult string   `yaml:"latest_good_result"`
	RequiredPaths    []string `yaml:"required_paths"`
}

func TestEvalRegressionSetRegistryIfPresent(t *testing.T) {
	root := evalActivationRepoRoot(t)
	registryPath := filepath.Join(root, ".devspecs", "eval-runs", "_registry", "manifest-registry.yaml")
	data, err := os.ReadFile(registryPath)
	if os.IsNotExist(err) {
		t.Skip("local private eval registry is absent")
	}
	if err != nil {
		t.Fatalf("read eval registry: %v", err)
	}

	var registry evalRegressionSetRegistry
	if err := yaml.Unmarshal(data, &registry); err != nil {
		t.Fatalf("parse eval registry: %v", err)
	}
	if registry.Version != 1 {
		t.Fatalf("unexpected registry version %d", registry.Version)
	}

	requiredSets := map[string]int{
		"smoke-5-recent-quality":               5,
		"skinny-25-recent-legacy":              20,
		"fat-100-vps-daily-target":             100,
		"fat-156-recent-legacy":                150,
		"fat-100-map-full-checkout-historical": 100,
	}
	allowedStatus := map[string]bool{
		"active":      true,
		"candidate":   true,
		"historical":  true,
		"retired":     true,
		"quarantined": true,
	}

	seen := map[string]bool{}
	for _, set := range registry.Sets {
		if set.ID == "" || set.Status == "" || set.Tier == "" {
			t.Fatalf("registry set has missing identity fields: %#v", set)
		}
		if !allowedStatus[set.Status] {
			t.Fatalf("registry set %s has unsupported status %q", set.ID, set.Status)
		}
		seen[set.ID] = true
		minRepos := set.MinimumRepoCount
		if requiredMin, ok := requiredSets[set.ID]; ok && requiredMin > minRepos {
			minRepos = requiredMin
		}
		if set.RepoCount < minRepos {
			t.Fatalf("registry set %s has repo_count %d below minimum %d", set.ID, set.RepoCount, minRepos)
		}
		if set.CanonicalDir == "" {
			t.Fatalf("registry set %s missing canonical_dir", set.ID)
		}
		if len(set.RequiredPaths) == 0 {
			t.Fatalf("registry set %s has no required_paths", set.ID)
		}
		for _, p := range set.RequiredPaths {
			full := evalRegistryPath(root, p)
			if _, err := os.Stat(full); err != nil {
				t.Fatalf("registry set %s required path %s missing: %v", set.ID, p, err)
			}
		}
	}
	for id := range requiredSets {
		if !seen[id] {
			t.Fatalf("registry missing required set %s", id)
		}
	}
}

func evalActivationRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func evalRegistryPath(root, p string) string {
	p = filepath.FromSlash(p)
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}

func readActivationGolden(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func containsActivationArg(args []string, want string) bool {
	for i, arg := range args {
		if arg == want || strings.HasPrefix(arg, want+"=") {
			return true
		}
		if want == "--path" && i > 0 && args[i-1] == "--path" {
			return true
		}
	}
	return false
}

func writeActivationFakeBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "fake-ds.cmd")
		body := "@echo off\r\necho {\"schema\":\"fake.activation.v1\",\"ok\":true}\r\n"
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path := filepath.Join(dir, "fake-ds")
	body := "#!/bin/sh\nprintf '%s\\n' '{\"schema\":\"fake.activation.v1\",\"ok\":true}'\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
