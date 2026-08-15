package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	preflightSchema = "devspecs.fat_regression_preflight.v1"
	summarySchema   = "devspecs.fat_regression_summary.v1"
)

var errReviewRequired = errors.New("fat regression requires review")

type regressionManifest struct {
	Version int              `yaml:"version"`
	Repos   []regressionRepo `yaml:"repos"`
}

type regressionRepo struct {
	ID        string              `yaml:"id"`
	Path      string              `yaml:"path"`
	CommitSHA string              `yaml:"commit_sha"`
	CloneMode string              `yaml:"clone_mode"`
	Profiles  []string            `yaml:"profiles"`
	Commands  []regressionCommand `yaml:"commands"`
}

type regressionCommand struct {
	Name string `yaml:"name"`
}

type preflightResult struct {
	Schema            string `json:"schema"`
	Repositories      int    `json:"repositories"`
	ExactHead         int    `json:"exact_head"`
	FullHistory       int    `json:"full_history"`
	ActivationCommand int    `json:"activation_commands"`
	ScanCommands      int    `json:"scan_commands"`
}

type activationResult struct {
	Summary struct {
		Total          int `json:"total"`
		Passed         int `json:"passed"`
		Missing        int `json:"missing"`
		Failed         int `json:"failed"`
		DurationMillis int `json:"duration_ms"`
	} `json:"summary"`
	RepoSet struct {
		Total int `json:"total"`
	} `json:"repo_set"`
	Timing struct {
		Baseline  timingStats `json:"baseline_ms"`
		Candidate timingStats `json:"candidate_ms"`
	} `json:"timing"`
}

type scanResult struct {
	Summary struct {
		Total              int `json:"total"`
		Passed             int `json:"passed"`
		Failed             int `json:"failed"`
		RegressionFailures int `json:"regression_failures"`
		DurationMillis     int `json:"duration_ms"`
	} `json:"summary"`
	RepoSet struct {
		Total int `json:"total"`
	} `json:"repo_set"`
	Timing timingStats `json:"scan_ms"`
	Cases  []scanCase  `json:"cases"`
}

type scanCase struct {
	RepoID      string `json:"repo_id"`
	Command     string `json:"command"`
	Status      string `json:"status"`
	Error       string `json:"error"`
	StderrBytes int    `json:"stderr_bytes"`
	StderrPath  string `json:"stderr_path"`
}

type timingStats struct {
	Total int `json:"total"`
	P50   int `json:"p50"`
	P95   int `json:"p95"`
	Max   int `json:"max"`
}

type publicSummary struct {
	Schema       string           `json:"schema"`
	GeneratedAt  string           `json:"generated_at"`
	BaselineRef  string           `json:"baseline_ref"`
	CandidateRef string           `json:"candidate_ref"`
	Corpus       publicCorpus     `json:"corpus"`
	Activation   publicActivation `json:"activation"`
	Scan         publicScan       `json:"scan"`
	Gate         publicGate       `json:"gate"`
}

type publicCorpus struct {
	Repositories int `json:"repositories"`
}

type publicActivation struct {
	Cases           int         `json:"cases"`
	Exact           int         `json:"exact"`
	ChangedOrFailed int         `json:"changed_or_failed"`
	Missing         int         `json:"missing"`
	DurationMillis  int         `json:"duration_ms"`
	BaselineTiming  timingStats `json:"baseline_ms"`
	CandidateTiming timingStats `json:"candidate_ms"`
}

type publicScan struct {
	Baseline              publicScanRole `json:"baseline"`
	Candidate             publicScanRole `json:"candidate"`
	SharedFailures        int            `json:"shared_failures"`
	BaselineOnlyFailures  int            `json:"baseline_only_failures"`
	CandidateOnlyFailures int            `json:"candidate_only_failures"`
	RegressionFailures    int            `json:"regression_failures"`
}

type publicScanRole struct {
	Cases          int         `json:"cases"`
	Passed         int         `json:"passed"`
	Failed         int         `json:"failed"`
	DurationMillis int         `json:"duration_ms"`
	Timing         timingStats `json:"scan_ms"`
}

type publicGate struct {
	Status  string   `json:"status"`
	Reasons []string `json:"reasons,omitempty"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: fat-regression <preflight|summarize> [flags]")
	}
	switch args[0] {
	case "preflight":
		return runPreflight(args[1:], out)
	case "summarize":
		return runSummarize(args[1:], out)
	default:
		return fmt.Errorf("unknown command %q; expected preflight or summarize", args[0])
	}
}

func runPreflight(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("preflight", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	activationPath := flags.String("activation", "", "activation manifest")
	scanPath := flags.String("scan", "", "scan manifest")
	minimum := flags.Int("minimum", 100, "minimum repositories")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*activationPath) == "" || strings.TrimSpace(*scanPath) == "" {
		return errors.New("--activation and --scan are required")
	}
	result, err := validateCorpora(*activationPath, *scanPath, *minimum)
	if err != nil {
		return err
	}
	return writeJSON(out, result)
}

func runSummarize(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("summarize", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	activationPath := flags.String("activation-result", "", "activation result JSON")
	baselineScanPath := flags.String("baseline-scan-result", "", "baseline scan result JSON")
	candidateScanPath := flags.String("candidate-scan-result", "", "candidate scan result JSON")
	outputPath := flags.String("output", "", "summary output JSON")
	baselineRef := flags.String("baseline-ref", "", "public baseline ref")
	candidateRef := flags.String("candidate-ref", "", "public candidate ref")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*activationPath) == "" || strings.TrimSpace(*baselineScanPath) == "" || strings.TrimSpace(*candidateScanPath) == "" {
		return errors.New("activation and both scan result paths are required")
	}
	activation, err := readJSON[activationResult](*activationPath, "activation")
	if err != nil {
		return err
	}
	baselineScan, err := readJSON[scanResult](*baselineScanPath, "baseline scan")
	if err != nil {
		return err
	}
	candidateScan, err := readJSON[scanResult](*candidateScanPath, "candidate scan")
	if err != nil {
		return err
	}
	summary := buildSummary(activation, baselineScan, candidateScan, *baselineRef, *candidateRef, time.Now().UTC())
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("encode public summary: %w", err)
	}
	encoded = append(encoded, '\n')
	if strings.TrimSpace(*outputPath) != "" {
		if err := os.WriteFile(*outputPath, encoded, 0o644); err != nil {
			return errors.New("write public summary")
		}
	}
	if _, err := out.Write(encoded); err != nil {
		return fmt.Errorf("write public summary output: %w", err)
	}
	if summary.Gate.Status != "passed" {
		return errReviewRequired
	}
	return nil
}

func validateCorpora(activationPath, scanPath string, minimum int) (preflightResult, error) {
	if minimum < 1 {
		return preflightResult{}, errors.New("minimum repositories must be positive")
	}
	activation, err := readManifest(activationPath, "activation")
	if err != nil {
		return preflightResult{}, err
	}
	scan, err := readManifest(scanPath, "scan")
	if err != nil {
		return preflightResult{}, err
	}
	if len(activation.Repos) < minimum {
		return preflightResult{}, fmt.Errorf("activation corpus has %d repositories; require at least %d", len(activation.Repos), minimum)
	}
	if len(scan.Repos) != len(activation.Repos) {
		return preflightResult{}, fmt.Errorf("manifest repository counts differ: activation=%d scan=%d", len(activation.Repos), len(scan.Repos))
	}
	scanByID := make(map[string]regressionRepo, len(scan.Repos))
	for index, repo := range scan.Repos {
		if strings.TrimSpace(repo.ID) == "" {
			return preflightResult{}, fmt.Errorf("scan repository %d has no id", index+1)
		}
		if _, exists := scanByID[repo.ID]; exists {
			return preflightResult{}, fmt.Errorf("scan repository %d duplicates an id", index+1)
		}
		scanByID[repo.ID] = repo
	}
	seen := make(map[string]struct{}, len(activation.Repos))
	for index, repo := range activation.Repos {
		position := index + 1
		if strings.TrimSpace(repo.ID) == "" {
			return preflightResult{}, fmt.Errorf("activation repository %d has no id", position)
		}
		if _, exists := seen[repo.ID]; exists {
			return preflightResult{}, fmt.Errorf("activation repository %d duplicates an id", position)
		}
		seen[repo.ID] = struct{}{}
		if err := validateManifestRepo(repo, position, "activation", "recent", "map"); err != nil {
			return preflightResult{}, err
		}
		scanRepo, exists := scanByID[repo.ID]
		if !exists {
			return preflightResult{}, fmt.Errorf("activation repository %d is missing from scan manifest", position)
		}
		if filepath.Clean(scanRepo.Path) != filepath.Clean(repo.Path) || !strings.EqualFold(scanRepo.CommitSHA, repo.CommitSHA) {
			return preflightResult{}, fmt.Errorf("repository %d differs between activation and scan manifests", position)
		}
		if err := validateManifestRepo(scanRepo, position, "scan", "scan"); err != nil {
			return preflightResult{}, err
		}
		if err := validateCheckout(repo, position); err != nil {
			return preflightResult{}, err
		}
	}
	return preflightResult{
		Schema:            preflightSchema,
		Repositories:      len(activation.Repos),
		ExactHead:         len(activation.Repos),
		FullHistory:       len(activation.Repos),
		ActivationCommand: len(activation.Repos) * 2,
		ScanCommands:      len(scan.Repos),
	}, nil
}

func readManifest(path, label string) (regressionManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return regressionManifest{}, fmt.Errorf("read %s manifest", label)
	}
	var manifest regressionManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return regressionManifest{}, fmt.Errorf("parse %s manifest", label)
	}
	if manifest.Version != 1 {
		return regressionManifest{}, fmt.Errorf("%s manifest version must be 1", label)
	}
	return manifest, nil
}

func validateManifestRepo(repo regressionRepo, position int, label string, requiredCommands ...string) error {
	if !validCommitSHA(repo.CommitSHA) {
		return fmt.Errorf("%s repository %d requires an exact 40-character commit", label, position)
	}
	if repo.CloneMode != "full" {
		return fmt.Errorf("%s repository %d must use clone_mode full", label, position)
	}
	if !contains(repo.Profiles, "fat") {
		return fmt.Errorf("%s repository %d must include the fat profile", label, position)
	}
	for _, required := range requiredCommands {
		if !containsCommand(repo.Commands, required) {
			return fmt.Errorf("%s repository %d is missing the %s command", label, position, required)
		}
	}
	return nil
}

func validateCheckout(repo regressionRepo, position int) error {
	info, err := os.Stat(repo.Path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("repository %d checkout is unavailable", position)
	}
	root, err := gitOutput(repo.Path, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("repository %d is not a Git checkout", position)
	}
	expectedRoot, err := filepath.Abs(repo.Path)
	if err != nil {
		return fmt.Errorf("repository %d checkout path is invalid", position)
	}
	actualRoot, err := filepath.Abs(root)
	if err != nil || !samePath(actualRoot, expectedRoot) {
		return fmt.Errorf("repository %d path is not its Git root", position)
	}
	head, err := gitOutput(repo.Path, "rev-parse", "HEAD")
	if err != nil || !strings.EqualFold(head, repo.CommitSHA) {
		return fmt.Errorf("repository %d HEAD does not match its pinned commit", position)
	}
	shallow, err := gitOutput(repo.Path, "rev-parse", "--is-shallow-repository")
	if err != nil || shallow != "false" {
		return fmt.Errorf("repository %d does not have full history", position)
	}
	tracked, err := exec.Command("git", "-C", repo.Path, "ls-files", "-z").Output()
	if err != nil || len(tracked) == 0 {
		return fmt.Errorf("repository %d has no tracked files", position)
	}
	return nil
}

func buildSummary(activation activationResult, baselineScan, candidateScan scanResult, baselineRef, candidateRef string, generated time.Time) publicSummary {
	shared, baselineOnly, candidateOnly := classifyScanFailures(baselineScan.Cases, candidateScan.Cases)
	reasons := make([]string, 0, 4)
	if activation.Summary.Failed > 0 || activation.Summary.Missing > 0 {
		reasons = append(reasons, "activation output changed or failed")
	}
	if candidateScan.Summary.RegressionFailures > 0 {
		reasons = append(reasons, "cold scan performance crossed the regression threshold")
	}
	if candidateOnly > 0 {
		reasons = append(reasons, "candidate scan introduced command failures")
	}
	if baselineOnly > 0 {
		reasons = append(reasons, "baseline scan failed where the candidate did not")
	}
	status := "passed"
	if len(reasons) > 0 {
		status = "needs_review"
	}
	return publicSummary{
		Schema:       summarySchema,
		GeneratedAt:  generated.UTC().Format(time.RFC3339),
		BaselineRef:  baselineRef,
		CandidateRef: candidateRef,
		Corpus:       publicCorpus{Repositories: activation.RepoSet.Total},
		Activation: publicActivation{
			Cases:           activation.Summary.Total,
			Exact:           activation.Summary.Passed,
			ChangedOrFailed: activation.Summary.Failed,
			Missing:         activation.Summary.Missing,
			DurationMillis:  activation.Summary.DurationMillis,
			BaselineTiming:  activation.Timing.Baseline,
			CandidateTiming: activation.Timing.Candidate,
		},
		Scan: publicScan{
			Baseline: publicScanRole{
				Cases:          baselineScan.Summary.Total,
				Passed:         baselineScan.Summary.Passed,
				Failed:         baselineScan.Summary.Failed,
				DurationMillis: baselineScan.Summary.DurationMillis,
				Timing:         baselineScan.Timing,
			},
			Candidate: publicScanRole{
				Cases:          candidateScan.Summary.Total,
				Passed:         candidateScan.Summary.Passed,
				Failed:         candidateScan.Summary.Failed,
				DurationMillis: candidateScan.Summary.DurationMillis,
				Timing:         candidateScan.Timing,
			},
			SharedFailures:        shared,
			BaselineOnlyFailures:  baselineOnly,
			CandidateOnlyFailures: candidateOnly,
			RegressionFailures:    candidateScan.Summary.RegressionFailures,
		},
		Gate: publicGate{Status: status, Reasons: reasons},
	}
}

func classifyScanFailures(baseline, candidate []scanCase) (int, int, int) {
	baselineFailures := make(map[string]string)
	candidateFailures := make(map[string]string)
	for _, item := range baseline {
		if item.Status != "passed" {
			baselineFailures[scanCaseKey(item)] = scanFailureFingerprint(item)
		}
	}
	for _, item := range candidate {
		if item.Status != "passed" {
			candidateFailures[scanCaseKey(item)] = scanFailureFingerprint(item)
		}
	}
	shared := 0
	baselineOnly := 0
	for key, baselineError := range baselineFailures {
		candidateError, exists := candidateFailures[key]
		if exists && candidateError == baselineError {
			shared++
			continue
		}
		baselineOnly++
	}
	candidateOnly := 0
	for key, candidateError := range candidateFailures {
		baselineError, exists := baselineFailures[key]
		if !exists || baselineError != candidateError {
			candidateOnly++
		}
	}
	return shared, baselineOnly, candidateOnly
}

func scanCaseKey(item scanCase) string {
	return item.RepoID + "\x00" + item.Command
}

func scanFailureFingerprint(item scanCase) string {
	fingerprint := fmt.Sprintf("%s\x00%d", item.Error, item.StderrBytes)
	if strings.TrimSpace(item.StderrPath) == "" {
		return fingerprint
	}
	data, err := os.ReadFile(item.StderrPath)
	if err != nil {
		return fingerprint + "\x00unreadable:" + item.StderrPath
	}
	sum := sha256.Sum256(data)
	return fingerprint + "\x00" + hex.EncodeToString(sum[:])
}

func readJSON[T any](path, label string) (T, error) {
	var value T
	data, err := os.ReadFile(path)
	if err != nil {
		return value, fmt.Errorf("read %s result", label)
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("parse %s result", label)
	}
	return value, nil
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func validCommitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func containsCommand(commands []regressionCommand, expected string) bool {
	for _, command := range commands {
		if command.Name == expected {
			return true
		}
	}
	return false
}

func gitOutput(path string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", path}, args...)
	output, err := exec.Command("git", commandArgs...).Output()
	return strings.TrimSpace(string(output)), err
}

func samePath(left, right string) bool {
	if filepath.Separator == '\\' {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
