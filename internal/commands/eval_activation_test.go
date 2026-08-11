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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestEvalActivationMatrixUpdate_WritesUniqueNormalizedGoldens(t *testing.T) {
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
	{
		err := updateCmd.Execute()
		require.NoError(t, err)
	}

	var update activationMatrixResult
	{
		err := json.Unmarshal(updateBuf.Bytes(), &update)
		require.NoError(t, err,
			"activation update JSON: %v\n%s", err, updateBuf.String())
	}
	assert.Equal(t, 3, update.Summary.Total, "unexpected update summary: %#v", update.Summary)
	assert.Equal(t, 3, update.Summary.Updated, "unexpected update summary: %#v", update.Summary)
	assert.NotContains(t, readActivationGolden(t, update.Cases[0].GoldenPath), filepath.ToSlash(skinnyRepo),
		"golden output leaked repo path: %s", update.Cases[0].GoldenPath)

	seenGoldens := map[string]bool{}
	for _, c := range update.Cases {
		assert.False(t, seenGoldens[c.GoldenPath],
			"activation matrix reused golden path for repeated command: %s", c.GoldenPath)

		seenGoldens[c.GoldenPath] = true
	}
}

func TestEvalActivationMatrixCompare_UsesForcedSkinnyArgs(t *testing.T) {
	skinnyRepo := setupActivationMatrixRepo(t, "credentials", "feat: credentials rotation context")
	fatRepo := setupActivationMatrixRepo(t, "billing", "feat: billing analytics export")
	manifest := writeActivationMatrixManifest(t, skinnyRepo, fatRepo)
	goldenDir := filepath.Join(t.TempDir(), "goldens")
	writeActivationMatrixGoldens(t, manifest, goldenDir)

	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		manifest,
		"--activation-matrix",
		"--activation-profile", "skinny",
		"--activation-golden-dir", goldenDir,
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var compare activationMatrixResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &compare), "activation compare JSON:\n%s", buf.String())
	assert.Equal(t, 3, compare.Summary.Total, "unexpected compare summary: %#v", compare.Summary)
	assert.Equal(t, 3, compare.Summary.Passed, "unexpected compare summary: %#v", compare.Summary)

	for _, c := range compare.Cases {
		assert.Equal(t, "skinny", c.Profile, "unexpected compare case: %#v", c)
		assert.Equal(t, "passed", c.Status, "unexpected compare case: %#v", c)
		require.NotEmpty(t, c.Args, "activation args did not enforce quiet JSON path contract: %#v", c.Args)
		assert.True(t, containsActivationArg(c.Args, "--json"), "activation args did not enforce quiet JSON path contract: %#v", c.Args)
		assert.True(t, containsActivationArg(c.Args, "--quiet"), "activation args did not enforce quiet JSON path contract: %#v", c.Args)
		assert.True(t, containsActivationArg(c.Args, "--path"), "activation args did not enforce quiet JSON path contract: %#v", c.Args)

		joinedArgs := strings.Join(c.Args, "\x00")
		assert.NotContains(t, joinedArgs, "--json=false", "activation args kept manifest values that should be overridden: %#v", c.Args)
		assert.NotContains(t, joinedArgs, "--quiet=false", "activation args kept manifest values that should be overridden: %#v", c.Args)
		assert.NotContains(t, joinedArgs, "ignored", "activation args kept manifest values that should be overridden: %#v", c.Args)

	}
}

func TestEvalActivationMatrixCrossCommandUpdate_UsesExactTaskGoldens(t *testing.T) {
	repo := setupActivationMatrixRepo(t, "billing", "feat: billing analytics export")
	manifest := writeActivationCrossCommandManifest(t, repo)
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
	{
		err := updateCmd.Execute()
		require.NoError(t, err)
	}

	var update activationMatrixResult
	{
		err := json.Unmarshal(updateBuf.Bytes(), &update)
		require.NoError(t, err,
			"activation cross-command update JSON: %v\n%s", err, updateBuf.String())
	}
	assert.Equal(t, 2, update.Summary.Total, "unexpected cross-command update summary: %#v", update.Summary)
	assert.Equal(t, 1, update.Summary.Passed, "unexpected cross-command update summary: %#v", update.Summary)
	assert.Equal(t, 1, update.Summary.Updated, "unexpected cross-command update summary: %#v", update.Summary)

	taskUpdate := activationCaseByCommand(t, update.Cases, "task")
	assert.Equal(t, activationCompareModeExact, taskUpdate.CompareMode, "task update case = %#v", taskUpdate)
	assert.Equal(t, "updated", taskUpdate.Status, "task update case = %#v", taskUpdate)
	{

		body := readActivationGolden(t, taskUpdate.GoldenPath)
		assert.NotContains(t, body, filepath.ToSlash(repo), "task golden leaked repo path or placeholder:\n%s", body)
		assert.NotContains(t, body, activationTaskDirPlaceholder, "task golden leaked repo path or placeholder:\n%s", body)
	}
}

func TestEvalActivationMatrixCrossCommandCompare_SupportsFindAndTaskSmoke(t *testing.T) {
	repo := setupActivationMatrixRepo(t, "billing", "feat: billing analytics export")
	manifest := writeActivationCrossCommandManifest(t, repo)
	goldenDir := filepath.Join(t.TempDir(), "goldens")
	writeActivationMatrixGoldens(t, manifest, goldenDir)

	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		manifest,
		"--activation-matrix",
		"--activation-profile", "skinny",
		"--activation-golden-dir", goldenDir,
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var compare activationMatrixResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &compare), "activation cross-command compare JSON:\n%s", buf.String())
	assert.Equal(t, 2, compare.Summary.Total, "unexpected cross-command compare summary: %#v", compare.Summary)
	assert.Equal(t, 2, compare.Summary.Passed, "unexpected cross-command compare summary: %#v", compare.Summary)

	findCase := activationCaseByCommand(t, compare.Cases, "find")
	assert.Equal(t, activationCompareModeJSONSmoke, findCase.CompareMode, "find smoke case = %#v", findCase)
	assert.Equal(t, "passed", findCase.Status, "find smoke case = %#v", findCase)
	assert.False(t, containsActivationArg(findCase.Args, "--path"), "find activation args should use cwd plus json only: %#v", findCase.Args)
	assert.False(t, containsActivationArg(findCase.Args, "--quiet"), "find activation args should use cwd plus json only: %#v", findCase.Args)
	assert.True(t, containsActivationArg(findCase.Args, "--json"), "find activation args should use cwd plus json only: %#v", findCase.Args)

	taskCase := activationCaseByCommand(t, compare.Cases, "task")
	assert.True(t, containsActivationArg(taskCase.Args, "--"+repoTargetFlagName), "task activation args missing repo/dir/json isolation: %#v", taskCase.Args)
	assert.True(t, containsActivationArg(taskCase.Args, "--dir"), "task activation args missing repo/dir/json isolation: %#v", taskCase.Args)
	assert.True(t, containsActivationArg(taskCase.Args, "--json"), "task activation args missing repo/dir/json isolation: %#v", taskCase.Args)

}

func writeActivationMatrixGoldens(t *testing.T, manifest, goldenDir string) {
	t.Helper()
	cmd := NewEvalCmd()
	cmd.SetArgs([]string{
		manifest,
		"--activation-matrix",
		"--activation-profile", "skinny",
		"--activation-golden-dir", goldenDir,
		"--activation-update",
		"--json",
	})
	cmd.SetOut(&bytes.Buffer{})

	require.NoError(t, cmd.Execute())
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
	require.NotNil(t, err, "expected missing fat goldens to fail, got %v", err)
	assert.Contains(t, err.Error(), "activation matrix regressions failed", "expected missing fat goldens to fail, got %v", err)

	var result activationMatrixResult
	{
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err,
			"activation missing JSON: %v\n%s", err, buf.String())
	}
	assert.Equal(t, 1, result.Summary.Total, "fat profile should select only one missing command, got %#v", result.Summary)
	assert.Equal(t, 1, result.Summary.Missing, "fat profile should select only one missing command, got %#v", result.Summary)
	assert.Equal(t, "fat-repo", result.Cases[0].RepoID, "unexpected fat case: %#v", result.Cases[0])
	assert.Equal(t, "map", result.Cases[0].Command, "unexpected fat case: %#v", result.Cases[0])

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
	require.NoError(t, err)

	data = append([]byte{0xEF, 0xBB, 0xBF}, data...)
	{
		err := os.WriteFile(manifestPath, data, 0o644)
		require.NoError(t, err)
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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var result activationMatrixResult
	{
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err,
			"activation BOM JSON: %v\n%s", err, buf.String())
	}
	assert.Equal(t, 1, result.Summary.Total, "unexpected BOM manifest summary: %#v", result.Summary)
	assert.Equal(t, 1, result.Summary.Updated, "unexpected BOM manifest summary: %#v", result.Summary)

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var result activationMatrixResult
	{
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err,
			"activation result metadata JSON: %v\n%s", err, buf.String())
	}
	assert.Equal(t, "full", result.CloneMode, "activation metadata missing clone/runner/normalization: %#v", result)
	assert.Equal(t, activationMatrixRunnerGolden, result.RunnerMode, "activation metadata missing clone/runner/normalization: %#v", result)
	assert.NotEqual(t, "", result.NormalizationVersion, "activation metadata missing clone/runner/normalization: %#v", result)
	assert.Equal(t, activationIndexStateCold, result.IndexState,
		"default activation index state = %q", result.IndexState)
	assert.Equal(t, filepath.ToSlash(resultDir), result.ResultDir, "activation result dir/path not recorded: %#v", result)
	assert.NotEqual(t, "", result.ResultPath, "activation result dir/path not recorded: %#v", result)
	assert.Equal(t, 1, result.RepoSet.Total, "activation repo set metadata = %#v", result.RepoSet)
	assert.Equal(t, 1, result.RepoSet.Full, "activation repo set metadata = %#v", result.RepoSet)
	assert.Equal(t, 0, result.RepoSet.Shallow, "activation repo set metadata = %#v", result.RepoSet)
	assert.Greater(t, result.Timing.Command.Total, 0, "activation timing missing command stats: %#v", result.Timing)
	assert.Greater(t, result.Timing.Command.P50, 0, "activation timing missing command stats: %#v", result.Timing)
	assert.Greater(t, result.Timing.Command.Max, 0, "activation timing missing command stats: %#v", result.Timing)
	require.NotEmpty(t, result.SlowestCases, "activation slowest case metadata missing: %#v", result.SlowestCases)
	assert.NotEqual(t, "", result.SlowestCases[0].RepoID, "activation slowest case metadata missing: %#v", result.SlowestCases)
	assert.Greater(t, result.SlowestCases[0].DurationMillis, 0, "activation slowest case metadata missing: %#v", result.SlowestCases)
	{

		_, err := os.Stat(filepath.Join(resultDir, activationMatrixDefaultResultFilename))
		require.NoError(t, err,
			"activation result file not written: %v", err)
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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var result activationMatrixResult
	{
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err,
			"activation warm JSON: %v\n%s", err, buf.String())
	}
	assert.Equal(t, activationIndexStateWarm, result.IndexState,
		"result index state = %q", result.IndexState)

	for _, c := range result.Cases {
		assert.Equal(t, activationIndexStateWarm, c.IndexState,
			"case index state = %q in %#v", c.IndexState, c)

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var result activationMatrixResult
	{
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err,
			"activation binary compare JSON: %v\n%s", err, buf.String())
	}
	assert.Equal(t, activationMatrixRunnerBinaryComparison, result.RunnerMode,
		"expected binary comparison runner, got %q", result.RunnerMode)
	assert.Equal(t, activationIndexStateCold, result.BaselineIndexState, "default binary index states missing: baseline=%q candidate=%q", result.BaselineIndexState, result.CandidateIndexState)
	assert.Equal(t, activationIndexStateCold, result.CandidateIndexState, "default binary index states missing: baseline=%q candidate=%q", result.BaselineIndexState, result.CandidateIndexState)
	assert.Equal(t, 3, result.Summary.Total, "unexpected binary compare summary: %#v", result.Summary)
	assert.Equal(t, 3, result.Summary.Passed, "unexpected binary compare summary: %#v", result.Summary)
	assert.NotEqual(t, "", result.BaselineBin, "binary metadata missing: %#v", result)
	assert.NotEqual(t, "", result.CandidateBin, "binary metadata missing: %#v", result)
	assert.NotEqual(t, "", result.BaselineBinaryID, "binary metadata missing: %#v", result)
	assert.NotEqual(t, "", result.CandidateBinaryID, "binary metadata missing: %#v", result)
	require.NotEmpty(t, result.SlowestCases, "binary slowest case metadata missing: %#v", result.SlowestCases)
	assert.Greater(t, result.SlowestCases[0].BaselineMillis, 0, "binary slowest case metadata missing: %#v", result.SlowestCases)
	assert.Greater(t, result.SlowestCases[0].CandidateMillis, 0, "binary slowest case metadata missing: %#v", result.SlowestCases)

	for _, c := range result.Cases {
		require.NotNil(t, c.Baseline, "binary comparison run metadata missing: %#v", c)
		require.NotNil(t, c.Candidate, "binary comparison run metadata missing: %#v", c)
		assert.True(t, c.StdoutMatch, "binary comparison run metadata missing: %#v", c)
		require.NotEmpty(t, c.Baseline.Args, "baseline args should include command name: %#v", c.Baseline.Args)
		assert.Equal(t, c.Command, c.Baseline.Args[0], "baseline args should include command name: %#v", c.Baseline.Args)
		assert.NotEqual(t, "", c.Baseline.StdoutPath, "binary outputs were not written: %#v", c)
		assert.NotEqual(t, "", c.Candidate.StdoutPath, "binary outputs were not written: %#v", c)

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var result activationMatrixResult
	{
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err,
			"activation binary state JSON: %v\n%s", err, buf.String())
	}
	assert.Equal(t, activationIndexStateCold, result.BaselineIndexState, "binary index states = baseline %q candidate %q", result.BaselineIndexState, result.CandidateIndexState)
	assert.Equal(t, activationIndexStateWarm, result.CandidateIndexState, "binary index states = baseline %q candidate %q", result.BaselineIndexState, result.CandidateIndexState)

	for _, c := range result.Cases {
		require.NotNil(t, c.Baseline, "missing binary runs: %#v", c)
		require.NotNil(t, c.Candidate, "missing binary runs: %#v", c)
		assert.Equal(t, activationIndexStateCold, c.Baseline.IndexState, "case index states = baseline %q candidate %q", c.Baseline.IndexState, c.Candidate.IndexState)
		assert.Equal(t, activationIndexStateWarm, c.Candidate.IndexState, "case index states = baseline %q candidate %q", c.Baseline.IndexState, c.Candidate.IndexState)
		assert.GreaterOrEqual(t, c.Candidate.WarmupMillis, 0,
			"warmup millis should be non-negative: %#v", c.Candidate)

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var result activationScanBenchmarkResult
	{
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err,
			"activation scan benchmark JSON: %v\n%s", err, buf.String())
	}
	assert.Equal(t, activationScanBenchmarkSchemaVersion, result.Schema,
		"schema = %q", result.Schema)
	assert.Equal(t, 1, result.Summary.Total, "unexpected summary: %#v", result.Summary)
	assert.Equal(t, 1, result.Summary.Passed, "unexpected summary: %#v", result.Summary)
	assert.Greater(t, result.Timing.Max, 0, "missing aggregate timing: %#v phases=%#v", result.Timing, result.PhaseTiming)
	require.NotEmpty(t, result.PhaseTiming, "missing aggregate timing: %#v phases=%#v", result.Timing, result.PhaseTiming)

	c := result.Cases[0]
	assert.Equal(t, "scan", c.Command, "scan benchmark args not forced: %#v", c.Args)
	assert.True(t, containsActivationArg(c.Args, "--phase-timing"), "scan benchmark args not forced: %#v", c.Args)
	assert.True(t, containsActivationArg(c.Args, "--quiet"), "scan benchmark args not forced: %#v", c.Args)
	assert.True(t, containsActivationArg(c.Args, "--path"), "scan benchmark args not forced: %#v", c.Args)
	assert.Greater(t, c.DBSizeBytes, int64(0), "missing DB stats: size=%d rows=%#v groups=%#v", c.DBSizeBytes, c.TableRows, c.TableGroups)
	assert.Greater(t, c.TableRows["artifacts"], 0, "missing DB stats: size=%d rows=%#v groups=%#v", c.DBSizeBytes, c.TableRows, c.TableGroups)
	assert.Greater(t, c.TableGroups["artifacts"], 0, "missing DB stats: size=%d rows=%#v groups=%#v", c.DBSizeBytes, c.TableRows, c.TableGroups)
	assert.Greater(t, c.Found["markdown"], 0, "missing scan summary/phase timing: %#v", c)
	require.NotNil(t, c.PhaseTiming, "missing scan summary/phase timing: %#v", c)
	require.NotEmpty(t, c.PhaseTiming.Phases, "missing scan summary/phase timing: %#v", c)
	require.NotNil(t, c.SourceManifest, "missing source manifest summary: %#v", c.SourceManifest)
	assert.True(t, c.SourceManifest.Enabled, "missing source manifest summary: %#v", c.SourceManifest)
	assert.Greater(t, c.SourceManifest.IndexedFiles, 0, "missing source manifest summary: %#v", c.SourceManifest)
	assert.NotEqual(t, "", c.StdoutPath, "expected retained stdout/stderr paths: %#v", c)
	assert.NotEqual(t, "", c.StderrPath, "expected retained stdout/stderr paths: %#v", c)
	{

		_, err := os.Stat(filepath.Join(resultDir, activationScanBenchmarkResultName))
		require.NoError(t, err,
			"benchmark result file not written: %v", err)
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
	assert.Equal(t, "failed", current.Status, "expected regression failure, got %#v", current)
	require.NotNil(t, current.Regression, "expected regression failure, got %#v", current)
	assert.Equal(t, "failed", current.Regression.Status, "expected regression failure, got %#v", current)
	assert.Equal(t, 100, current.Regression.DurationDeltaMS, "unexpected regression deltas: %#v", current.Regression)
	assert.Equal(t, int64(30), current.Regression.DBSizeDeltaBytes, "unexpected regression deltas: %#v", current.Regression)
	{

		got := current.Regression.TableGroupDeltas["artifacts"]
		assert.Equal(t, 2, got,
			"artifact table group delta = %d, want 2", got)
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
	assert.True(t, comparison.Accepted,
		"expected non-action map deltas to be accepted: %#v", comparison)
	assert.Equal(t, "accepted_non_action_deltas", comparison.Status,
		"expected accepted non-action status, got %#v", comparison)
	assert.Empty(t, comparison.ActionDiffs, "unexpected comparison detail: %#v", comparison)
	require.NotEmpty(t, comparison.AcceptedDiffs, "unexpected comparison detail: %#v", comparison)

}

func TestActivationMapActionQualityRejectsTryDelta(t *testing.T) {
	baseline := activationMapActionQualityFixture(`ds find "website ia pivot homepage"`)
	candidate := activationMapActionQualityFixture(`ds find "website ia pivot homepage"`)
	candidate.Areas[0].Try = `ds find "launch website skeleton"`

	comparison := compareActivationMapActionQuality(mustActivationMapJSON(t, baseline), mustActivationMapJSON(t, candidate))
	assert.False(t, comparison.Accepted,
		"expected try delta to be rejected: %#v", comparison)
	assert.Contains(t, strings.Join(comparison.ActionDiffs, "\n"), "areas[0].try",
		"expected try diff to be reported: %#v", comparison)

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
	assert.True(t, comparison.Accepted,
		"expected suppressed diagnostic delta to be accepted: %#v", comparison)
	assert.Empty(t, comparison.ActionDiffs, "unexpected comparison detail: %#v", comparison)
	require.NotEmpty(t, comparison.AcceptedDiffs, "unexpected comparison detail: %#v", comparison)

}

func TestActivationMapActionQualityRejectsKeyPathDelta(t *testing.T) {
	baseline := activationMapActionQualityFixture(`ds find "layout panel talking"`)
	candidate := activationMapActionQualityFixture(`ds find "layout panel talking"`)
	candidate.Areas[0].KeyPaths = []string{"app/page.tsx", "components/other-panel.tsx"}

	comparison := compareActivationMapActionQuality(mustActivationMapJSON(t, baseline), mustActivationMapJSON(t, candidate))
	assert.False(t, comparison.Accepted,
		"expected key path delta to be rejected: %#v", comparison)
	assert.Contains(t, strings.Join(comparison.ActionDiffs, "\n"), "areas[0].key_paths",
		"expected key path diff to be reported: %#v", comparison)

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
	require.NoError(t, err)

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
	{
		err := os.WriteFile(path, []byte(body), 0o644)
		require.NoError(t, err)
	}

	return path
}

func writeActivationCrossCommandManifest(t *testing.T, repo string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "activation-cross-command.yaml")
	body := fmt.Sprintf(`version: 1
repos:
  - id: cross-command-repo
    path: %q
    profiles: [skinny]
    commands:
      - name: find
        args: ["billing", "--plain"]
        compare: json_smoke
      - name: task
        args: ["quick", "trace billing activation", "--id", "activation-task-smoke", "--index=false"]
`, filepath.ToSlash(repo))
	{
		err := os.WriteFile(path, []byte(body), 0o644)
		require.NoError(t, err)
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
	{
		err := os.WriteFile(path, []byte(body), 0o644)
		require.NoError(t, err)
	}

	return path
}

func activationCaseByCommand(t *testing.T, cases []activationMatrixCaseResult, command string) activationMatrixCaseResult {
	t.Helper()
	matches := make([]activationMatrixCaseResult, 0, 1)
	for _, c := range cases {
		if c.Command == command {
			matches = append(matches, c)
		}
	}
	require.Len(t, matches, 1, "activation cases for command %q in %#v", command, cases)

	return matches[0]
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
	require.NoError(t, err,
		"read eval registry: %v", err)

	var registry evalRegressionSetRegistry
	{
		err := yaml.Unmarshal(data, &registry)
		require.NoError(t, err,
			"parse eval registry: %v", err)
	}
	assert.Equal(t, 1, registry.Version,
		"unexpected registry version %d", registry.Version)

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
		assert.NotEqual(t, "", set.ID, "registry set has missing identity fields: %#v", set)
		assert.NotEqual(t, "", set.Status, "registry set has missing identity fields: %#v", set)
		assert.NotEqual(t, "", set.Tier, "registry set has missing identity fields: %#v", set)
		assert.True(t, allowedStatus[set.Status],
			"registry set %s has unsupported status %q", set.ID, set.Status)

		seen[set.ID] = true
		minRepos := set.MinimumRepoCount
		if requiredMin, ok := requiredSets[set.ID]; ok && requiredMin > minRepos {
			minRepos = requiredMin
		}
		assert.GreaterOrEqual(t, set.RepoCount, minRepos,
			"registry set %s has repo_count %d below minimum %d", set.ID, set.RepoCount, minRepos)
		assert.NotEqual(t, "", set.CanonicalDir,
			"registry set %s missing canonical_dir", set.ID)
		require.NotEmpty(t, set.RequiredPaths,
			"registry set %s has no required_paths", set.ID)

		for _, p := range set.RequiredPaths {
			full := evalRegistryPath(root, p)
			{
				_, err := os.Stat(full)
				require.NoError(t, err,
					"registry set %s required path %s missing: %v", set.ID, p, err)
			}

		}
	}
	for id := range requiredSets {
		assert.True(t, seen[id],
			"registry missing required set %s", id)

	}
}

func evalActivationRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok,
		"resolve test file")

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
	require.NoError(t, err)

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
		{
			err := os.WriteFile(path, []byte(body), 0o755)
			require.NoError(t, err)
		}

		return path
	}
	path := filepath.Join(dir, "fake-ds")
	body := "#!/bin/sh\nprintf '%s\\n' '{\"schema\":\"fake.activation.v1\",\"ok\":true}'\n"
	{
		err := os.WriteFile(path, []byte(body), 0o755)
		require.NoError(t, err)
	}

	return path
}
