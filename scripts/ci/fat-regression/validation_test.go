package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSummary_WithEmptyResults_RequiresReview(t *testing.T) {
	summary := buildSummary(activationResult{}, scanResult{}, scanResult{}, "v1.4.0", "candidate", time.Time{})

	assert.Equal(t, "needs_review", summary.Gate.Status)
	require.NotEmpty(t, summary.Gate.Reasons)
	assert.Equal(t, "activation result schema or execution mode is invalid", summary.Gate.Reasons[0])
}

func TestValidateResultSets_WithCompleteCases_AcceptsResults(t *testing.T) {
	activation := activationFixture(2, 2, 0)
	scan := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"})

	err := validateResultSets(activation, scan, scan)

	require.NoError(t, err)
}

func TestValidateResultSets_WithMissingActivationCase_RejectsResults(t *testing.T) {
	activation := activationFixture(2, 2, 0)
	require.Len(t, activation.Cases, 2)
	activation.Cases = activation.Cases[:1]
	scan := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"})

	err := validateResultSets(activation, scan, scan)

	require.EqualError(t, err, "activation result is empty or incomplete")
}

func TestValidateResultSets_WithDuplicateActivationCase_RejectsResults(t *testing.T) {
	activation := activationFixture(2, 2, 0)
	require.Len(t, activation.Cases, 2)
	activation.Cases[1] = activation.Cases[0]
	scan := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"})

	err := validateResultSets(activation, scan, scan)

	require.EqualError(t, err, "activation result contains duplicate cases")
}

func TestValidateResultSets_WithFalsePassingActivationCounter_RejectsResults(t *testing.T) {
	activation := activationFixture(2, 1, 1)
	activation.Summary.Passed = 2
	activation.Summary.Failed = 0
	scan := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"})

	err := validateResultSets(activation, scan, scan)

	require.EqualError(t, err, "activation summary does not match its cases")
}

func TestValidateResultSets_WithWarmScan_RejectsResults(t *testing.T) {
	activation := activationFixture(2, 2, 0)
	baseline := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"})
	candidate := baseline
	candidate.IndexState = "warm"

	err := validateResultSets(activation, baseline, candidate)

	require.EqualError(t, err, "scan result schema or execution mode is invalid")
}

func TestValidateResultSets_WithMissingScanCases_RejectsResults(t *testing.T) {
	activation := activationFixture(2, 2, 0)
	baseline := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"})
	candidate := baseline
	candidate.Cases = nil

	err := validateResultSets(activation, baseline, candidate)

	require.EqualError(t, err, "scan result is empty or incomplete")
}

func TestValidateResultSets_WithDifferentScanRepository_RejectsWithoutLeakingIdentity(t *testing.T) {
	activation := activationFixture(2, 2, 0)
	baseline := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"})
	candidate := scanFixture(scanCase{RepoID: "private-replacement", Command: "scan", Status: "passed"})

	err := validateResultSets(activation, baseline, candidate)

	require.EqualError(t, err, "scan result contains mismatched or duplicate cases")
	assert.NotContains(t, err.Error(), "private-replacement")
}

func TestValidateResultSets_WithFalsePassingScanCounter_RejectsResults(t *testing.T) {
	activation := activationFixture(2, 2, 0)
	baseline := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"})
	candidate := scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "failed"})
	candidate.Summary.Passed = 1
	candidate.Summary.Failed = 0

	err := validateResultSets(activation, baseline, candidate)

	require.EqualError(t, err, "scan summary does not match its cases")
}

func TestRunSummarize_WithNonzeroExit_RequiresReviewDespitePassingResults(t *testing.T) {
	args := passingSummaryArgs(t)
	args = append(args, "--candidate-scan-exit", "2")
	var output bytes.Buffer

	err := run(args, &output)

	require.ErrorIs(t, err, errReviewRequired)
	var summary publicSummary
	require.NoError(t, json.Unmarshal(output.Bytes(), &summary))
	assert.Equal(t, "needs_review", summary.Gate.Status)
	require.Len(t, summary.Gate.Reasons, 1)
	assert.Equal(t, "command exit status was missing or nonzero", summary.Gate.Reasons[0])
}

func TestRunSummarize_WithMissingExitEvidence_RequiresReview(t *testing.T) {
	args := passingSummaryArgs(t)
	require.Len(t, args, 15)
	args = args[:len(args)-6]
	var output bytes.Buffer

	err := run(args, &output)

	require.ErrorIs(t, err, errReviewRequired)
	assert.Contains(t, output.String(), "command exit status was missing or nonzero")
}

func TestRunSummarize_WithMissingPreflight_RequiresReview(t *testing.T) {
	args := passingSummaryArgs(t)
	args = append(args, "--preflight-result", filepath.Join(t.TempDir(), "absent.json"))
	var output bytes.Buffer

	err := run(args, &output)

	require.ErrorIs(t, err, errReviewRequired)
	assert.Contains(t, output.String(), "result corpus does not match a successful preflight")
}

func TestRunSummarize_WithTruncatedCohort_RequiresReview(t *testing.T) {
	args := passingSummaryArgs(t)
	preflight := validPreflightFixture()
	preflight.Repositories, preflight.ExactHead, preflight.FullHistory = 2, 2, 2
	preflight.ActivationCommand, preflight.ScanCommands = 4, 2
	args = append(args, "--preflight-result", writeJSONFixture(t, preflight))
	var output bytes.Buffer

	err := run(args, &output)

	require.ErrorIs(t, err, errReviewRequired)
	assert.Contains(t, output.String(), "result corpus does not match a successful preflight")
}

func TestValidateCheckout_WithTrackedEdit_RejectsCorpus(t *testing.T) {
	path, sha := setupRegressionRepository(t)
	require.NoError(t, os.WriteFile(filepath.Join(path, "README.md"), []byte("changed\n"), 0o644))

	err := validateCheckout(activationManifestRepo(path, sha), 1)

	require.EqualError(t, err, "repository 1 checkout is not clean")
}

func TestValidateCheckout_WithUntrackedSource_RejectsCorpus(t *testing.T) {
	path, sha := setupRegressionRepository(t)
	require.NoError(t, os.WriteFile(filepath.Join(path, "extra.go"), []byte("package extra\n"), 0o644))

	err := validateCheckout(activationManifestRepo(path, sha), 1)

	require.EqualError(t, err, "repository 1 checkout is not clean")
}

func TestValidateCheckout_WithIgnoredSource_RejectsCorpus(t *testing.T) {
	path, sha := setupRegressionRepository(t)
	require.NoError(t, os.WriteFile(filepath.Join(path, ".git", "info", "exclude"), []byte("generated.go\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(path, "generated.go"), []byte("package generated\n"), 0o644))

	err := validateCheckout(activationManifestRepo(path, sha), 1)

	require.EqualError(t, err, "repository 1 checkout is not clean")
}

func validPreflightFixture() preflightResult {
	return preflightResult{Schema: preflightSchema, Repositories: 1, ExactHead: 1, FullHistory: 1, ActivationCommand: 2, ScanCommands: 1}
}

func passingSummaryArgs(t *testing.T) []string {
	t.Helper()
	return []string{
		"summarize",
		"--activation-result", writeJSONFixture(t, activationFixture(2, 2, 0)),
		"--baseline-scan-result", writeJSONFixture(t, scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"})),
		"--candidate-scan-result", writeJSONFixture(t, scanFixture(scanCase{RepoID: "sample", Command: "scan", Status: "passed"})),
		"--preflight-result", writeJSONFixture(t, validPreflightFixture()),
		"--activation-exit", "0", "--baseline-scan-exit", "0", "--candidate-scan-exit", "0",
	}
}
