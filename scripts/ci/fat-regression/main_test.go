package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRun_WithPreflightCommand_WritesAggregateResult(t *testing.T) {
	repoPath, commitSHA := setupRegressionRepository(t)
	activationPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{
		activationManifestRepo(repoPath, commitSHA),
	}})
	scanPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{
		scanManifestRepo(repoPath, commitSHA),
	}})
	var output bytes.Buffer

	err := run([]string{"preflight", "--activation", activationPath, "--scan", scanPath, "--minimum", "1"}, &output)

	require.NoError(t, err)
	var result preflightResult
	require.NoError(t, json.Unmarshal(output.Bytes(), &result))
	assert.Equal(t, preflightSchema, result.Schema)
	assert.Equal(t, 1, result.Repositories)
}

func TestRun_WithPassingSummaryCommand_WritesPublicResultFile(t *testing.T) {
	activationPath := writeJSONFixture(t, activationFixture(2, 2, 0))
	baselinePath := writeJSONFixture(t, scanFixture(scanCase{RepoID: "private-baseline", Command: "scan", Status: "passed"}))
	candidatePath := writeJSONFixture(t, scanFixture(scanCase{RepoID: "private-candidate", Command: "scan", Status: "passed"}))
	outputPath := filepath.Join(t.TempDir(), "summary.json")
	var output bytes.Buffer

	err := run([]string{
		"summarize",
		"--activation-result", activationPath,
		"--baseline-scan-result", baselinePath,
		"--candidate-scan-result", candidatePath,
		"--baseline-ref", "v1.4.0",
		"--candidate-ref", "abcdef1",
		"--output", outputPath,
	}, &output)

	require.NoError(t, err)
	assert.FileExists(t, outputPath)
	var result publicSummary
	require.NoError(t, json.Unmarshal(output.Bytes(), &result))
	assert.Equal(t, "passed", result.Gate.Status)
	assert.Equal(t, "v1.4.0", result.BaselineRef)
	assert.Equal(t, "abcdef1", result.CandidateRef)
}

func TestRun_WithReviewSummaryCommand_WritesResultBeforeReturningReview(t *testing.T) {
	activationPath := writeJSONFixture(t, activationFixture(2, 1, 1))
	baselinePath := writeJSONFixture(t, scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"}))
	candidatePath := writeJSONFixture(t, scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"}))
	outputPath := filepath.Join(t.TempDir(), "summary.json")
	var output bytes.Buffer

	err := run([]string{
		"summarize",
		"--activation-result", activationPath,
		"--baseline-scan-result", baselinePath,
		"--candidate-scan-result", candidatePath,
		"--output", outputPath,
	}, &output)

	require.ErrorIs(t, err, errReviewRequired)
	assert.FileExists(t, outputPath)
	var result publicSummary
	require.NoError(t, json.Unmarshal(output.Bytes(), &result))
	assert.Equal(t, "needs_review", result.Gate.Status)
}

func TestRun_WithoutCommand_ReturnsUsageError(t *testing.T) {
	err := run(nil, &bytes.Buffer{})

	require.Error(t, err)
	assert.Equal(t, "usage: fat-regression <preflight|summarize> [flags]", err.Error())
}

func TestRun_WithUnknownCommand_ReturnsActionableError(t *testing.T) {
	err := run([]string{"other"}, &bytes.Buffer{})

	require.Error(t, err)
	assert.Equal(t, `unknown command "other"; expected preflight or summarize`, err.Error())
}

func TestRun_WithPreflightMissingScanManifest_ReturnsRequiredArgumentError(t *testing.T) {
	err := run([]string{"preflight", "--activation", "manifest.yaml"}, &bytes.Buffer{})

	require.Error(t, err)
	assert.Equal(t, "--activation and --scan are required", err.Error())
}

func TestRun_WithSummarizeMissingScanResults_ReturnsRequiredArgumentError(t *testing.T) {
	err := run([]string{"summarize", "--activation-result", "result.json"}, &bytes.Buffer{})

	require.Error(t, err)
	assert.Equal(t, "activation and both scan result paths are required", err.Error())
}

func TestRun_WithMalformedActivationResult_ReturnsParseError(t *testing.T) {
	activationPath := filepath.Join(t.TempDir(), "activation.json")
	require.NoError(t, os.WriteFile(activationPath, []byte("{"), 0o644))
	baselinePath := writeJSONFixture(t, scanResult{})
	candidatePath := writeJSONFixture(t, scanResult{})

	err := run([]string{
		"summarize",
		"--activation-result", activationPath,
		"--baseline-scan-result", baselinePath,
		"--candidate-scan-result", candidatePath,
	}, &bytes.Buffer{})

	require.Error(t, err)
	assert.Equal(t, "parse activation result", err.Error())
}

func TestValidateCorpora_WithMatchingPinnedFullHistoryManifests_AcceptsCorpus(t *testing.T) {
	repoPath, commitSHA := setupRegressionRepository(t)
	activationPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{
		activationManifestRepo(repoPath, commitSHA),
	}})
	scanPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{
		scanManifestRepo(repoPath, commitSHA),
	}})

	result, err := validateCorpora(activationPath, scanPath, 1)

	require.NoError(t, err)
	assert.Equal(t, preflightSchema, result.Schema)
	assert.Equal(t, 1, result.Repositories)
	assert.Equal(t, 1, result.ExactHead)
	assert.Equal(t, 1, result.FullHistory)
	assert.Equal(t, 2, result.ActivationCommand)
	assert.Equal(t, 1, result.ScanCommands)
}

func TestValidateCorpora_WithTooFewRepositories_RejectsCorpus(t *testing.T) {
	activationPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{{ID: "sample"}}})
	scanPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{{ID: "sample"}}})

	_, err := validateCorpora(activationPath, scanPath, 2)

	require.Error(t, err)
	assert.Equal(t, "activation corpus has 1 repositories; require at least 2", err.Error())
}

func TestValidateCorpora_WithUnsupportedManifestVersion_RejectsCorpus(t *testing.T) {
	activationPath := writeRegressionManifest(t, regressionManifest{Version: 2})
	scanPath := writeRegressionManifest(t, regressionManifest{Version: 1})

	_, err := validateCorpora(activationPath, scanPath, 1)

	require.Error(t, err)
	assert.Equal(t, "activation manifest version must be 1", err.Error())
}

func TestValidateCorpora_WithNonFullCloneMode_RejectsCorpus(t *testing.T) {
	repoPath, commitSHA := setupRegressionRepository(t)
	activationRepo := activationManifestRepo(repoPath, commitSHA)
	activationRepo.CloneMode = "shallow"
	activationPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{activationRepo}})
	scanPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{scanManifestRepo(repoPath, commitSHA)}})

	_, err := validateCorpora(activationPath, scanPath, 1)

	require.Error(t, err)
	assert.Equal(t, "activation repository 1 must use clone_mode full", err.Error())
}

func TestValidateCorpora_WithoutFatProfile_RejectsCorpus(t *testing.T) {
	repoPath, commitSHA := setupRegressionRepository(t)
	activationRepo := activationManifestRepo(repoPath, commitSHA)
	activationRepo.Profiles = []string{"skinny"}
	activationPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{activationRepo}})
	scanPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{scanManifestRepo(repoPath, commitSHA)}})

	_, err := validateCorpora(activationPath, scanPath, 1)

	require.Error(t, err)
	assert.Equal(t, "activation repository 1 must include the fat profile", err.Error())
}

func TestValidateCorpora_WithDifferentPinnedCommit_RejectsCorpus(t *testing.T) {
	repoPath, commitSHA := setupRegressionRepository(t)
	activationPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{
		activationManifestRepo(repoPath, commitSHA),
	}})
	differentSHA := strings.Repeat("a", 40)
	scanPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{
		scanManifestRepo(repoPath, differentSHA),
	}})

	_, err := validateCorpora(activationPath, scanPath, 1)

	require.Error(t, err)
	assert.Equal(t, "repository 1 differs between activation and scan manifests", err.Error())
}

func TestValidateCorpora_WithDriftedCheckout_RejectsCorpus(t *testing.T) {
	repoPath, commitSHA := setupRegressionRepository(t)
	createRegressionCommit(t, repoPath, "second")
	activationPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{
		activationManifestRepo(repoPath, commitSHA),
	}})
	scanPath := writeRegressionManifest(t, regressionManifest{Version: 1, Repos: []regressionRepo{
		scanManifestRepo(repoPath, commitSHA),
	}})

	_, err := validateCorpora(activationPath, scanPath, 1)

	require.Error(t, err)
	assert.Equal(t, "repository 1 HEAD does not match its pinned commit", err.Error())
}

func TestBuildSummary_WithExactActivationAndSharedScanWarning_Passes(t *testing.T) {
	activation := activationFixture(2, 2, 0)
	baseline := scanFixture(scanCase{RepoID: "private-repo", Command: "scan", Status: "failed", Error: "scan benchmark wrote stderr"})
	candidate := scanFixture(scanCase{RepoID: "private-repo", Command: "scan", Status: "failed", Error: "scan benchmark wrote stderr"})

	summary := buildSummary(activation, baseline, candidate, "v1.4.0", "abcdef1", time.Date(2026, 8, 15, 8, 0, 0, 0, time.UTC))

	assert.Equal(t, "passed", summary.Gate.Status)
	assert.Empty(t, summary.Gate.Reasons)
	assert.Equal(t, 1, summary.Scan.SharedFailures)
	assert.Equal(t, 0, summary.Scan.BaselineOnlyFailures)
	assert.Equal(t, 0, summary.Scan.CandidateOnlyFailures)
}

func TestBuildSummary_WithActivationDelta_RequiresReview(t *testing.T) {
	activation := activationFixture(2, 1, 1)
	baseline := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"})
	candidate := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"})

	summary := buildSummary(activation, baseline, candidate, "v1.4.0", "abcdef1", time.Now())

	assert.Equal(t, "needs_review", summary.Gate.Status)
	assert.Len(t, summary.Gate.Reasons, 1)
	assert.Equal(t, "activation output changed or failed", summary.Gate.Reasons[0])
}

func TestBuildSummary_WithCandidateScanRegression_RequiresReview(t *testing.T) {
	activation := activationFixture(2, 2, 0)
	baseline := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"})
	candidate := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "failed", Error: "scan duration regression"})
	candidate.Summary.RegressionFailures = 1

	summary := buildSummary(activation, baseline, candidate, "v1.4.0", "abcdef1", time.Now())

	assert.Equal(t, "needs_review", summary.Gate.Status)
	assert.Len(t, summary.Gate.Reasons, 2)
	assert.Equal(t, "cold scan performance crossed the regression threshold", summary.Gate.Reasons[0])
	assert.Equal(t, "candidate scan introduced command failures", summary.Gate.Reasons[1])
	assert.Equal(t, 1, summary.Scan.RegressionFailures)
	assert.Equal(t, 1, summary.Scan.CandidateOnlyFailures)
}

func TestPublicSummary_WithPrivateInputs_OmitsRepositoryIdentity(t *testing.T) {
	activation := activationFixture(2, 2, 0)
	baseline := scanFixture(scanCase{RepoID: "secret-company-repo", Command: "scan", Status: "failed", Error: "shared warning"})
	candidate := scanFixture(scanCase{RepoID: "secret-company-repo", Command: "scan", Status: "failed", Error: "shared warning"})
	summary := buildSummary(activation, baseline, candidate, "v1.4.0", "abcdef1", time.Now())

	encoded, err := json.Marshal(summary)

	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret-company-repo")
	assert.NotContains(t, string(encoded), "shared warning")
}

func TestBuildSummary_WithDifferentCapturedWarnings_RequiresReview(t *testing.T) {
	activation := activationFixture(2, 2, 0)
	baselineStderr := filepath.Join(t.TempDir(), "baseline.stderr")
	candidateStderr := filepath.Join(t.TempDir(), "candidate.stderr")
	require.NoError(t, os.WriteFile(baselineStderr, []byte("existing warning"), 0o644))
	require.NoError(t, os.WriteFile(candidateStderr, []byte("new warning text"), 0o644))
	baseline := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "failed", Error: "scan benchmark wrote stderr", StderrBytes: 16, StderrPath: baselineStderr})
	candidate := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "failed", Error: "scan benchmark wrote stderr", StderrBytes: 16, StderrPath: candidateStderr})

	summary := buildSummary(activation, baseline, candidate, "v1.4.0", "abcdef1", time.Now())

	assert.Equal(t, "needs_review", summary.Gate.Status)
	assert.Len(t, summary.Gate.Reasons, 2)
	assert.Equal(t, "candidate scan introduced command failures", summary.Gate.Reasons[0])
	assert.Equal(t, "baseline scan failed where the candidate did not", summary.Gate.Reasons[1])
	assert.Equal(t, 0, summary.Scan.SharedFailures)
	assert.Equal(t, 1, summary.Scan.BaselineOnlyFailures)
	assert.Equal(t, 1, summary.Scan.CandidateOnlyFailures)
}

func activationManifestRepo(path, commitSHA string) regressionRepo {
	return regressionRepo{
		ID:        "sample",
		Path:      path,
		CommitSHA: commitSHA,
		CloneMode: "full",
		Profiles:  []string{"fat"},
		Commands: []regressionCommand{
			{Name: "recent"},
			{Name: "map"},
		},
	}
}

func scanManifestRepo(path, commitSHA string) regressionRepo {
	return regressionRepo{
		ID:        "sample",
		Path:      path,
		CommitSHA: commitSHA,
		CloneMode: "full",
		Profiles:  []string{"fat"},
		Commands:  []regressionCommand{{Name: "scan"}},
	}
}

func writeRegressionManifest(t *testing.T, manifest regressionManifest) string {
	t.Helper()
	data, err := yaml.Marshal(manifest)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

func writeJSONFixture(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "result.json")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

func setupRegressionRepository(t *testing.T) (string, string) {
	t.Helper()
	_, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is required for fat regression tests")
	}
	repoPath := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(repoPath, 0o755))
	runRegressionGit(t, repoPath, "init", "-b", "main")
	commitSHA := createRegressionCommit(t, repoPath, "first")
	return repoPath, commitSHA
}

func createRegressionCommit(t *testing.T, repoPath, contents string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "README.md"), []byte(contents+"\n"), 0o644))
	runRegressionGit(t, repoPath, "add", "README.md")
	runRegressionGit(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", contents)
	return strings.TrimSpace(runRegressionGit(t, repoPath, "rev-parse", "HEAD"))
}

func runRegressionGit(t *testing.T, repoPath string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return string(output)
}

func activationFixture(total, passed, failed int) activationResult {
	result := activationResult{}
	result.RepoSet.Total = 1
	result.Summary.Total = total
	result.Summary.Passed = passed
	result.Summary.Failed = failed
	return result
}

func scanFixture(item scanCase) scanResult {
	result := scanResult{Cases: []scanCase{item}}
	result.RepoSet.Total = 1
	result.Summary.Total = 1
	if item.Status == "passed" {
		result.Summary.Passed = 1
	} else {
		result.Summary.Failed = 1
	}
	return result
}
