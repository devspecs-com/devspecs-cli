package commands

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	activationMatrixSchemaVersion          = "devspecs.activation_matrix.v1"
	activationMatrixNormalizationVersion   = "devspecs.activation_normalize.v1"
	activationMapActionQualitySchema       = "devspecs.map_action_quality.v1"
	activationMatrixDefaultResultFilename  = "activation-matrix-result.json"
	activationMatrixRunnerGolden           = "golden"
	activationMatrixRunnerBinaryComparison = "binary-compare"
	activationIndexStateCold               = "cold"
	activationIndexStateWarm               = "warm"
	activationCompareModeExact             = "exact"
	activationCompareModeJSONSmoke         = "json_smoke"
	activationTaskDirPlaceholder           = "<activation-task-dir>"
)

type activationMatrixOptions struct {
	Profile             string
	GoldenDir           string
	ResultDir           string
	CloneMode           string
	BaselineBin         string
	CandidateBin        string
	IndexState          string
	BaselineIndexState  string
	CandidateIndexState string
	MapStructured       bool
	Quiet               bool
	Update              bool
	Generated           time.Time
}

type activationMatrixManifest struct {
	Version          int                    `json:"version" yaml:"version"`
	SuiteID          string                 `json:"suite_id,omitempty" yaml:"suite_id,omitempty"`
	SuiteVersion     string                 `json:"suite_version,omitempty" yaml:"suite_version,omitempty"`
	GoldenSetVersion string                 `json:"golden_set_version,omitempty" yaml:"golden_set_version,omitempty"`
	Repos            []activationMatrixRepo `json:"repos" yaml:"repos"`
}

type activationMatrixRepo struct {
	ID        string                    `json:"id" yaml:"id"`
	Path      string                    `json:"path" yaml:"path"`
	URL       string                    `json:"url,omitempty" yaml:"url,omitempty"`
	CommitSHA string                    `json:"commit_sha,omitempty" yaml:"commit_sha,omitempty"`
	CloneMode string                    `json:"clone_mode,omitempty" yaml:"clone_mode,omitempty"`
	Depth     int                       `json:"depth,omitempty" yaml:"depth,omitempty"`
	Filter    string                    `json:"filter,omitempty" yaml:"filter,omitempty"`
	Profiles  []string                  `json:"profiles" yaml:"profiles"`
	Commands  []activationMatrixCommand `json:"commands" yaml:"commands"`
}

type activationMatrixCommand struct {
	Name    string   `json:"name" yaml:"name"`
	Args    []string `json:"args" yaml:"args"`
	Compare string   `json:"compare,omitempty" yaml:"compare,omitempty"`
}

type activationMatrixResult struct {
	Schema               string                       `json:"schema"`
	NormalizationVersion string                       `json:"normalization_version"`
	ManifestPath         string                       `json:"manifest_path"`
	SuiteID              string                       `json:"suite_id,omitempty"`
	SuiteVersion         string                       `json:"suite_version,omitempty"`
	GoldenSetVersion     string                       `json:"golden_set_version,omitempty"`
	Profile              string                       `json:"profile"`
	CloneMode            string                       `json:"clone_mode"`
	RunnerMode           string                       `json:"runner_mode"`
	GoldenDir            string                       `json:"golden_dir,omitempty"`
	ResultDir            string                       `json:"result_dir,omitempty"`
	ResultPath           string                       `json:"result_path,omitempty"`
	BaselineBin          string                       `json:"baseline_bin,omitempty"`
	BaselineBinaryID     string                       `json:"baseline_binary_id,omitempty"`
	CandidateBin         string                       `json:"candidate_bin,omitempty"`
	CandidateBinaryID    string                       `json:"candidate_binary_id,omitempty"`
	IndexState           string                       `json:"index_state"`
	BaselineIndexState   string                       `json:"baseline_index_state,omitempty"`
	CandidateIndexState  string                       `json:"candidate_index_state,omitempty"`
	MapStructured        bool                         `json:"map_structured,omitempty"`
	Quiet                bool                         `json:"quiet"`
	Update               bool                         `json:"update"`
	GeneratedAt          string                       `json:"generated_at"`
	RepoSet              activationMatrixRepoSet      `json:"repo_set"`
	Summary              activationMatrixSummary      `json:"summary"`
	Timing               activationMatrixTiming       `json:"timing"`
	SlowestCases         []activationMatrixSlowCase   `json:"slowest_cases,omitempty"`
	Cases                []activationMatrixCaseResult `json:"cases"`
	Notes                []string                     `json:"notes,omitempty"`
}

type activationMatrixSummary struct {
	Total          int `json:"total"`
	Passed         int `json:"passed"`
	Updated        int `json:"updated"`
	Missing        int `json:"missing"`
	Failed         int `json:"failed"`
	DurationMillis int `json:"duration_ms"`
}

type activationMatrixRepoSet struct {
	Total          int `json:"total"`
	Full           int `json:"full"`
	Blobless       int `json:"blobless"`
	Shallow        int `json:"shallow"`
	MissingGit     int `json:"missing_git"`
	MissingCommit  int `json:"missing_commit"`
	MismatchedHead int `json:"mismatched_head"`
}

type activationMatrixTiming struct {
	Command   activationMatrixTimingStats `json:"command_ms,omitempty"`
	Baseline  activationMatrixTimingStats `json:"baseline_ms,omitempty"`
	Candidate activationMatrixTimingStats `json:"candidate_ms,omitempty"`
}

type activationMatrixTimingStats struct {
	Total int `json:"total"`
	P50   int `json:"p50"`
	P95   int `json:"p95"`
	Max   int `json:"max"`
}

type activationMatrixSlowCase struct {
	RepoID             string `json:"repo_id"`
	Command            string `json:"command"`
	Status             string `json:"status"`
	DurationMillis     int    `json:"duration_ms"`
	BaselineMillis     int    `json:"baseline_ms,omitempty"`
	CandidateMillis    int    `json:"candidate_ms,omitempty"`
	CandidateDeltaMS   int    `json:"candidate_delta_ms,omitempty"`
	CandidateSpeedupMS int    `json:"candidate_speedup_ms,omitempty"`
}

type activationMatrixCaseResult struct {
	RepoID           string                                `json:"repo_id"`
	RepoPath         string                                `json:"repo_path"`
	RepoURL          string                                `json:"repo_url,omitempty"`
	RepoCommitSHA    string                                `json:"repo_commit_sha,omitempty"`
	RepoCloneMode    string                                `json:"repo_clone_mode,omitempty"`
	RepoIsShallow    bool                                  `json:"repo_is_shallow,omitempty"`
	Profile          string                                `json:"profile"`
	Command          string                                `json:"command"`
	Args             []string                              `json:"args"`
	CompareMode      string                                `json:"compare_mode,omitempty"`
	IndexState       string                                `json:"index_state"`
	GoldenPath       string                                `json:"golden_path,omitempty"`
	Status           string                                `json:"status"`
	StdoutBytes      int                                   `json:"stdout_bytes"`
	StderrBytes      int                                   `json:"stderr_bytes"`
	StdoutSHA256     string                                `json:"stdout_sha256,omitempty"`
	DurationMillis   int                                   `json:"duration_ms"`
	StdoutMatch      bool                                  `json:"stdout_match,omitempty"`
	Baseline         *activationMatrixCommandRun           `json:"baseline,omitempty"`
	Candidate        *activationMatrixCommandRun           `json:"candidate,omitempty"`
	MapActionQuality *activationMapActionQualityComparison `json:"map_action_quality,omitempty"`
	Error            string                                `json:"error,omitempty"`
	Diff             string                                `json:"diff,omitempty"`
}

type activationMapActionQualityComparison struct {
	Schema        string   `json:"schema"`
	Accepted      bool     `json:"accepted"`
	Status        string   `json:"status"`
	Reason        string   `json:"reason,omitempty"`
	ActionDiffs   []string `json:"action_diffs,omitempty"`
	AcceptedDiffs []string `json:"accepted_diffs,omitempty"`
}

type activationMatrixCommandRun struct {
	Binary             string   `json:"binary,omitempty"`
	BinaryID           string   `json:"binary_id,omitempty"`
	Args               []string `json:"args"`
	ExitCode           int      `json:"exit_code"`
	StdoutBytes        int      `json:"stdout_bytes"`
	StderrBytes        int      `json:"stderr_bytes"`
	StdoutSHA256       string   `json:"stdout_sha256,omitempty"`
	StderrSHA256       string   `json:"stderr_sha256,omitempty"`
	StdoutPath         string   `json:"stdout_path,omitempty"`
	StderrPath         string   `json:"stderr_path,omitempty"`
	IndexState         string   `json:"index_state,omitempty"`
	WarmupMillis       int      `json:"warmup_ms,omitempty"`
	WarmupStdoutBytes  int      `json:"warmup_stdout_bytes,omitempty"`
	WarmupStderrBytes  int      `json:"warmup_stderr_bytes,omitempty"`
	ValidJSON          bool     `json:"valid_json"`
	DurationMillis     int      `json:"duration_ms"`
	Error              string   `json:"error,omitempty"`
	normalizationPaths []string
}

type activationRepoMetadata struct {
	URL            string
	CommitSHA      string
	ExpectedSHA    string
	CloneMode      string
	IsShallow      bool
	GitAvailable   bool
	CommitMismatch bool
	Error          string
}

func runActivationMatrix(manifestPath string, opts activationMatrixOptions) (*activationMatrixResult, error) {
	start := time.Now()
	if strings.TrimSpace(opts.Profile) == "" {
		opts.Profile = "skinny"
	}
	opts.CloneMode = normalizeActivationCloneMode(opts.CloneMode)
	if opts.CloneMode == "" {
		opts.CloneMode = "full"
	}
	if strings.TrimSpace(opts.GoldenDir) == "" {
		opts.GoldenDir = filepath.Join(defaultEvalResultsDir, "activation-goldens")
	}
	opts.IndexState = normalizeActivationIndexState(opts.IndexState)
	if opts.IndexState == "" {
		opts.IndexState = activationIndexStateCold
	}
	opts.BaselineIndexState = activationRoleIndexState(opts.IndexState, opts.BaselineIndexState)
	opts.CandidateIndexState = activationRoleIndexState(opts.IndexState, opts.CandidateIndexState)
	if opts.Generated.IsZero() {
		opts.Generated = nowUTC()
	}
	manifestAbs, err := filepath.Abs(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("resolve activation matrix manifest: %w", err)
	}
	manifest, err := loadActivationMatrixManifest(manifestAbs)
	if err != nil {
		return nil, err
	}
	goldenDir, err := filepath.Abs(opts.GoldenDir)
	if err != nil {
		return nil, fmt.Errorf("resolve activation matrix golden dir: %w", err)
	}
	resultDir := ""
	if strings.TrimSpace(opts.ResultDir) != "" {
		resultDir, err = filepath.Abs(opts.ResultDir)
		if err != nil {
			return nil, fmt.Errorf("resolve activation matrix result dir: %w", err)
		}
	}
	runnerMode := activationMatrixRunnerGolden
	if activationBinaryCompare(opts) {
		runnerMode = activationMatrixRunnerBinaryComparison
	}
	baselineBin, baselineID, err := activationBinaryPathAndID(opts.BaselineBin)
	if err != nil {
		return nil, err
	}
	candidateBin, candidateID, err := activationBinaryPathAndID(opts.CandidateBin)
	if err != nil {
		return nil, err
	}
	opts.BaselineBin = baselineBin
	opts.CandidateBin = candidateBin
	result := &activationMatrixResult{
		Schema:               activationMatrixSchemaVersion,
		NormalizationVersion: activationMatrixNormalizationVersion,
		ManifestPath:         filepath.ToSlash(manifestAbs),
		SuiteID:              manifest.SuiteID,
		SuiteVersion:         manifest.SuiteVersion,
		GoldenSetVersion:     manifest.GoldenSetVersion,
		Profile:              opts.Profile,
		CloneMode:            opts.CloneMode,
		RunnerMode:           runnerMode,
		GoldenDir:            filepath.ToSlash(goldenDir),
		ResultDir:            filepath.ToSlash(resultDir),
		BaselineBin:          filepath.ToSlash(baselineBin),
		BaselineBinaryID:     baselineID,
		CandidateBin:         filepath.ToSlash(candidateBin),
		CandidateBinaryID:    candidateID,
		IndexState:           opts.IndexState,
		BaselineIndexState:   opts.BaselineIndexState,
		CandidateIndexState:  opts.CandidateIndexState,
		MapStructured:        opts.MapStructured,
		Quiet:                opts.Quiet,
		Update:               opts.Update,
		GeneratedAt:          opts.Generated.UTC().Format(time.RFC3339),
	}
	if resultDir != "" {
		result.ResultPath = filepath.ToSlash(filepath.Join(resultDir, activationMatrixDefaultResultFilename))
	}
	for _, repo := range manifest.Repos {
		if !activationRepoMatchesProfile(repo, opts.Profile) {
			continue
		}
		repoPath, err := activationRepoPath(manifestAbs, repo)
		if err != nil {
			return nil, err
		}
		meta := activationReadRepoMetadata(repoPath, repo, opts.CloneMode)
		recordActivationRepoSet(&result.RepoSet, meta)
		commands := repo.Commands
		if len(commands) == 0 {
			commands = []activationMatrixCommand{{Name: "recent"}, {Name: "map"}}
		}
		for _, spec := range commands {
			caseResult := runActivationMatrixCase(repo, repoPath, meta, spec, opts, goldenDir, resultDir)
			result.Cases = append(result.Cases, caseResult)
			result.Summary.Total++
			switch caseResult.Status {
			case "passed":
				result.Summary.Passed++
			case "updated":
				result.Summary.Updated++
			case "missing":
				result.Summary.Missing++
			default:
				result.Summary.Failed++
			}
		}
	}
	if result.Summary.Total == 0 {
		result.Notes = append(result.Notes, fmt.Sprintf("no repos matched activation profile %q", opts.Profile))
	}
	result.Summary.DurationMillis = int(time.Since(start).Milliseconds())
	result.Timing = activationTimingSummary(result.Cases)
	result.SlowestCases = activationSlowestCases(result.Cases, 5)
	if resultDir != "" {
		if err := writeActivationMatrixResultFile(resultDir, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func loadActivationMatrixManifest(path string) (activationMatrixManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return activationMatrixManifest{}, fmt.Errorf("read activation matrix manifest: %w", err)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var manifest activationMatrixManifest
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		err = json.Unmarshal(data, &manifest)
	default:
		err = yaml.Unmarshal(data, &manifest)
	}
	if err != nil {
		return activationMatrixManifest{}, fmt.Errorf("parse activation matrix manifest: %w", err)
	}
	if manifest.Version != 1 {
		return activationMatrixManifest{}, fmt.Errorf("activation matrix manifest version must be 1, got %d", manifest.Version)
	}
	return manifest, nil
}

func activationRepoMatchesProfile(repo activationMatrixRepo, profile string) bool {
	if len(repo.Profiles) == 0 {
		return true
	}
	for _, candidate := range repo.Profiles {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(profile)) {
			return true
		}
	}
	return false
}

func activationRepoPath(manifestPath string, repo activationMatrixRepo) (string, error) {
	if strings.TrimSpace(repo.ID) == "" {
		return "", fmt.Errorf("activation matrix repo id is required")
	}
	if strings.TrimSpace(repo.Path) == "" {
		return "", fmt.Errorf("activation matrix repo %q path is required", repo.ID)
	}
	path := os.ExpandEnv(repo.Path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(manifestPath), path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve activation matrix repo %q: %w", repo.ID, err)
	}
	return filepath.Clean(abs), nil
}

func runActivationMatrixCase(repo activationMatrixRepo, repoPath string, meta activationRepoMetadata, spec activationMatrixCommand, opts activationMatrixOptions, goldenDir, resultDir string) activationMatrixCaseResult {
	start := time.Now()
	commandName := strings.ToLower(strings.TrimSpace(spec.Name))
	compareMode := activationCompareMode(spec.Compare)
	args := activationCommandArgs(commandName, spec.Args, repoPath, opts.Quiet)
	goldenPath := activationGoldenPath(goldenDir, opts.Profile, repo.ID, commandName, spec.Args)
	out := activationMatrixCaseResult{
		RepoID:        repo.ID,
		RepoPath:      filepath.ToSlash(repoPath),
		RepoURL:       meta.URL,
		RepoCommitSHA: meta.CommitSHA,
		RepoCloneMode: meta.CloneMode,
		RepoIsShallow: meta.IsShallow,
		Profile:       opts.Profile,
		Command:       commandName,
		Args:          args,
		CompareMode:   compareMode,
		IndexState:    opts.IndexState,
		GoldenPath:    filepath.ToSlash(goldenPath),
	}
	if meta.Error != "" {
		out.Status = "failed"
		out.Error = meta.Error
		return out
	}
	if !validActivationCompareMode(compareMode) {
		out.Status = "failed"
		out.Error = fmt.Sprintf("unsupported activation compare mode %q; valid values: %s, %s", compareMode, activationCompareModeExact, activationCompareModeJSONSmoke)
		return out
	}
	if activationBinaryCompare(opts) {
		return runActivationMatrixBinaryCompareCase(out, repoPath, commandName, args, opts, resultDir, start)
	}
	stdout, stderr, normalizationPaths, err := runActivationCommandIsolated(commandName, args, repoPath, opts.IndexState)
	out.StdoutBytes = len(stdout)
	out.StderrBytes = len(stderr)
	out.DurationMillis = int(time.Since(start).Milliseconds())
	normalized := normalizeActivationOutput(stdout, repoPath, normalizationPaths...)
	out.StdoutSHA256 = activationSHA256(normalized)
	if err != nil {
		out.Status = "failed"
		out.Error = err.Error()
		return out
	}
	if activationCommandRequiresStrictStderr(commandName, opts.Quiet) && len(stderr) > 0 {
		out.Status = "failed"
		out.Error = "quiet command wrote stderr"
		out.Diff = firstActivationOutputBytes(stderr, 600)
		return out
	}
	if compareMode == activationCompareModeJSONSmoke {
		if !json.Valid(bytes.TrimSpace(stdout)) {
			out.Status = "failed"
			out.Error = "stdout was not valid JSON"
			out.Diff = firstActivationOutputBytes(stdout, 600)
			return out
		}
		out.Status = "passed"
		return out
	}
	if opts.Update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			out.Status = "failed"
			out.Error = err.Error()
			return out
		}
		if err := os.WriteFile(goldenPath, normalized, 0o644); err != nil {
			out.Status = "failed"
			out.Error = err.Error()
			return out
		}
		out.Status = "updated"
		return out
	}
	expected, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		out.Status = "missing"
		out.Error = "golden file missing; rerun with --activation-update"
		return out
	}
	if err != nil {
		out.Status = "failed"
		out.Error = err.Error()
		return out
	}
	expected = bytes.ReplaceAll(expected, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(normalized, expected) {
		out.Status = "failed"
		out.Error = "stdout differed from golden"
		out.Diff = activationFirstDiff(expected, normalized)
		return out
	}
	out.Status = "passed"
	return out
}

func runActivationMatrixBinaryCompareCase(out activationMatrixCaseResult, repoPath, commandName string, args []string, opts activationMatrixOptions, resultDir string, start time.Time) activationMatrixCaseResult {
	baseline, baselineStdout := runExternalActivationCommandIsolated(opts.BaselineBin, commandName, args, repoPath, out.RepoID, "baseline", resultDir, opts.BaselineIndexState)
	candidate, candidateStdout := runExternalActivationCommandIsolated(opts.CandidateBin, commandName, args, repoPath, out.RepoID, "candidate", resultDir, opts.CandidateIndexState)
	out.Baseline = &baseline
	out.Candidate = &candidate
	out.StdoutBytes = candidate.StdoutBytes
	out.StderrBytes = candidate.StderrBytes
	out.DurationMillis = int(time.Since(start).Milliseconds())
	normalizedBaseline := normalizeActivationOutput(baselineStdout, repoPath, baseline.normalizationPaths...)
	normalizedCandidate := normalizeActivationOutput(candidateStdout, repoPath, candidate.normalizationPaths...)
	out.StdoutSHA256 = activationSHA256(normalizedCandidate)
	out.StdoutMatch = bytes.Equal(normalizedBaseline, normalizedCandidate)
	if baseline.Error != "" {
		out.Status = "failed"
		out.Error = "baseline command failed: " + baseline.Error
		return out
	}
	if candidate.Error != "" {
		out.Status = "failed"
		out.Error = "candidate command failed: " + candidate.Error
		return out
	}
	if baseline.ExitCode != 0 || candidate.ExitCode != 0 {
		out.Status = "failed"
		out.Error = fmt.Sprintf("baseline exit=%d candidate exit=%d", baseline.ExitCode, candidate.ExitCode)
		return out
	}
	if !baseline.ValidJSON || !candidate.ValidJSON {
		out.Status = "failed"
		out.Error = fmt.Sprintf("baseline valid_json=%t candidate valid_json=%t", baseline.ValidJSON, candidate.ValidJSON)
		return out
	}
	if activationCommandRequiresStrictStderr(commandName, opts.Quiet) && (baseline.StderrBytes > 0 || candidate.StderrBytes > 0) {
		out.Status = "failed"
		out.Error = fmt.Sprintf("quiet binary comparison wrote stderr: baseline=%d candidate=%d", baseline.StderrBytes, candidate.StderrBytes)
		return out
	}
	if out.CompareMode == activationCompareModeJSONSmoke {
		out.Status = "passed"
		return out
	}
	if !out.StdoutMatch {
		if commandName == "map" {
			comparison := compareActivationMapActionQuality(normalizedBaseline, normalizedCandidate)
			out.MapActionQuality = &comparison
			if opts.MapStructured && comparison.Accepted {
				out.Status = "passed"
				out.Diff = activationFirstDiff(normalizedBaseline, normalizedCandidate)
				return out
			}
			if comparison.Reason != "" {
				out.Error = "baseline and candidate stdout differed; map action-quality gate " + comparison.Status + ": " + comparison.Reason
			} else {
				out.Error = "baseline and candidate stdout differed"
			}
			out.Status = "failed"
			out.Diff = activationFirstDiff(normalizedBaseline, normalizedCandidate)
			return out
		}
		out.Status = "failed"
		out.Error = "baseline and candidate stdout differed"
		out.Diff = activationFirstDiff(normalizedBaseline, normalizedCandidate)
		return out
	}
	out.Status = "passed"
	return out
}

func compareActivationMapActionQuality(baselineOutput, candidateOutput []byte) activationMapActionQualityComparison {
	out := activationMapActionQualityComparison{
		Schema: activationMapActionQualitySchema,
		Status: "accepted",
	}
	var baseline, candidate mapOutput
	if err := json.Unmarshal(bytes.TrimSpace(baselineOutput), &baseline); err != nil {
		out.Status = "parse_error"
		out.Reason = "baseline map JSON could not be parsed"
		out.ActionDiffs = append(out.ActionDiffs, fmt.Sprintf("baseline parse error: %v", err))
		return out
	}
	if err := json.Unmarshal(bytes.TrimSpace(candidateOutput), &candidate); err != nil {
		out.Status = "parse_error"
		out.Reason = "candidate map JSON could not be parsed"
		out.ActionDiffs = append(out.ActionDiffs, fmt.Sprintf("candidate parse error: %v", err))
		return out
	}

	compareActivationValue(&out, "schema", baseline.Schema, candidate.Schema)
	compareActivationValue(&out, "repo", baseline.Repo, candidate.Repo)
	if baseline.EvidenceAvailability != candidate.EvidenceAvailability {
		addActivationMapActionDiff(&out, "evidence_availability", baseline.EvidenceAvailability, candidate.EvidenceAvailability)
	}
	compareActivationMapCaveats(&out, "caveats", baseline.Caveats, candidate.Caveats)
	compareActivationMapTopDiagnostics(&out, baseline.Diagnostics, candidate.Diagnostics)

	if len(baseline.Areas) != len(candidate.Areas) {
		addActivationMapActionDiff(&out, "areas.length", len(baseline.Areas), len(candidate.Areas))
	} else {
		for i := range baseline.Areas {
			compareActivationMapArea(&out, i, baseline.Areas[i], candidate.Areas[i])
		}
	}

	if len(out.ActionDiffs) > 0 {
		out.Accepted = false
		out.Status = "rejected"
		out.Reason = out.ActionDiffs[0]
		return out
	}
	out.Accepted = true
	if len(out.AcceptedDiffs) > 0 {
		out.Status = "accepted_non_action_deltas"
		out.Reason = strings.Join(out.AcceptedDiffs, "; ")
	} else {
		out.Status = "identical"
	}
	return out
}

func compareActivationMapArea(out *activationMapActionQualityComparison, index int, baseline, candidate mapArea) {
	prefix := fmt.Sprintf("areas[%d]", index)
	compareActivationValue(out, prefix+".id", baseline.ID, candidate.ID)
	compareActivationValue(out, prefix+".label", baseline.Label, candidate.Label)
	compareActivationValue(out, prefix+".class", baseline.Class, candidate.Class)
	compareActivationValue(out, prefix+".area_type", baseline.AreaType, candidate.AreaType)
	compareActivationValue(out, prefix+".boundary_role", baseline.BoundaryRole, candidate.BoundaryRole)
	compareActivationValue(out, prefix+".purpose", baseline.Purpose, candidate.Purpose)
	compareActivationOrderedStrings(out, prefix+".adjacent_systems", baseline.AdjacentSystems, candidate.AdjacentSystems)
	compareActivationValue(out, prefix+".confidence", baseline.Confidence, candidate.Confidence)
	compareActivationValue(out, prefix+".is_repo_root_umbrella", baseline.IsRepoRootUmbrella, candidate.IsRepoRootUmbrella)
	compareActivationOrderedStrings(out, prefix+".covers", baseline.Covers, candidate.Covers)
	compareActivationIntMap(out, prefix+".evidence_counts", baseline.EvidenceCounts, candidate.EvidenceCounts)
	compareActivationOrderedStrings(out, prefix+".key_paths", baseline.KeyPaths, candidate.KeyPaths)
	compareActivationValue(out, prefix+".try", baseline.Try, candidate.Try)
	compareActivationValue(out, prefix+".caveats", baseline.Caveats, candidate.Caveats)

	compareActivationDisplayStrings(out, prefix+".boundary_paths", baseline.BoundaryPaths, candidate.BoundaryPaths)
	compareActivationTraceDisplay(out, prefix+".trace_receipts", baseline.TraceReceipts, candidate.TraceReceipts)
	compareActivationMapAreaDiagnostics(out, prefix+".diagnostics", baseline.Diagnostics, candidate.Diagnostics)
}

func compareActivationMapTopDiagnostics(out *activationMapActionQualityComparison, baseline, candidate mapDiagnostics) {
	compareActivationValue(out, "diagnostics.area_query", baseline.AreaQuery, candidate.AreaQuery)
	compareActivationValue(out, "diagnostics.matched_area_count", baseline.MatchedAreaCount, candidate.MatchedAreaCount)
	if baseline.RawClusterCount != candidate.RawClusterCount ||
		baseline.WorkstreamAnchorsSeen != candidate.WorkstreamAnchorsSeen ||
		baseline.WorkstreamMaterialized != candidate.WorkstreamMaterialized ||
		baseline.TraceNoisyCommitsFiltered != candidate.TraceNoisyCommitsFiltered {
		addActivationMapAcceptedDiff(out, "diagnostics non-action counts differ")
	}
	compareActivationStringSetOnly(out, "diagnostics.suppressed_labels", baseline.SuppressedLabels, candidate.SuppressedLabels)
}

func compareActivationMapAreaDiagnostics(out *activationMapActionQualityComparison, prefix string, baseline, candidate mapAreaDiagnostics) {
	compareActivationValue(out, prefix+".key", baseline.Key, candidate.Key)
	compareActivationValue(out, prefix+".trace_receipt_mode", baseline.TraceReceiptMode, candidate.TraceReceiptMode)
	compareActivationDisplayStrings(out, prefix+".raw_anchors", baseline.RawAnchors, candidate.RawAnchors)
	compareActivationDisplayStrings(out, prefix+".label_evidence", baseline.LabelEvidence, candidate.LabelEvidence)
	compareActivationDisplayStrings(out, prefix+".trace_terms", baseline.TraceTerms, candidate.TraceTerms)
	compareActivationMapPackability(out, prefix+".packability", baseline.Packability, candidate.Packability)
}

func compareActivationMapPackability(out *activationMapActionQualityComparison, prefix string, baseline, candidate *mapPackabilityDiagnostics) {
	if baseline == nil && candidate == nil {
		return
	}
	if baseline == nil || candidate == nil {
		addActivationMapActionDiff(out, prefix, baseline, candidate)
		return
	}
	compareActivationValue(out, prefix+".decision", baseline.Decision, candidate.Decision)
	compareActivationValue(out, prefix+".selected_try_source", baseline.SelectedTrySource, candidate.SelectedTrySource)
	if baseline.TrySuppressed && candidate.TrySuppressed && baseline.Decision == candidate.Decision {
		if baseline.SuppressedTry != candidate.SuppressedTry || baseline.SuppressedTrySource != candidate.SuppressedTrySource {
			addActivationMapAcceptedDiff(out, prefix+" suppressed diagnostic differs")
		}
	} else {
		compareActivationValue(out, prefix+".suppressed_try", baseline.SuppressedTry, candidate.SuppressedTry)
		compareActivationValue(out, prefix+".suppressed_try_source", baseline.SuppressedTrySource, candidate.SuppressedTrySource)
	}
	compareActivationValue(out, prefix+".try_suppressed", baseline.TrySuppressed, candidate.TrySuppressed)
	if baseline.KeyPathCount != candidate.KeyPathCount ||
		baseline.IndexedKeyPathCount != candidate.IndexedKeyPathCount ||
		baseline.PrefixKeyPathCount != candidate.PrefixKeyPathCount ||
		baseline.IndexedQueryAnchorCount != candidate.IndexedQueryAnchorCount {
		addActivationMapAcceptedDiff(out, prefix+" support counts differ")
	}
	compareActivationStringSetOnly(out, prefix+".missing_key_extensions", baseline.MissingKeyExtensions, candidate.MissingKeyExtensions)
}

func compareActivationMapCaveats(out *activationMapActionQualityComparison, field string, baseline, candidate []string) {
	if activationOrderedStringsEqual(baseline, candidate) {
		return
	}
	baselineWithoutIndexCaveat := removeActivationString(baseline, mapIndexRequiredCaveat)
	if len(baselineWithoutIndexCaveat) != len(baseline) && activationOrderedStringsEqual(baselineWithoutIndexCaveat, candidate) {
		addActivationMapAcceptedDiff(out, field+" removed stale missing-index caveat")
		return
	}
	addActivationMapActionDiff(out, field, baseline, candidate)
}

func compareActivationValue(out *activationMapActionQualityComparison, field string, baseline, candidate any) {
	if fmt.Sprintf("%#v", baseline) == fmt.Sprintf("%#v", candidate) {
		return
	}
	addActivationMapActionDiff(out, field, baseline, candidate)
}

func compareActivationOrderedStrings(out *activationMapActionQualityComparison, field string, baseline, candidate []string) {
	if activationOrderedStringsEqual(baseline, candidate) {
		return
	}
	addActivationMapActionDiff(out, field, baseline, candidate)
}

func compareActivationIntMap(out *activationMapActionQualityComparison, field string, baseline, candidate map[string]int) {
	if len(baseline) != len(candidate) {
		addActivationMapActionDiff(out, field, baseline, candidate)
		return
	}
	for key, baselineValue := range baseline {
		if candidate[key] != baselineValue {
			addActivationMapActionDiff(out, field, baseline, candidate)
			return
		}
	}
}

func compareActivationStringSetOnly(out *activationMapActionQualityComparison, field string, baseline, candidate []string) {
	if activationOrderedStringsEqual(baseline, candidate) {
		return
	}
	if activationStringSetEqual(baseline, candidate) {
		addActivationMapAcceptedDiff(out, field+" order differs")
		return
	}
	addActivationMapActionDiff(out, field, baseline, candidate)
}

func compareActivationDisplayStrings(out *activationMapActionQualityComparison, field string, baseline, candidate []string) {
	if activationOrderedStringsEqual(baseline, candidate) {
		return
	}
	addActivationMapAcceptedDiff(out, field+" display differs")
}

func compareActivationTraceDisplay(out *activationMapActionQualityComparison, field string, baseline, candidate []mapTraceReceipt) {
	if len(baseline) == len(candidate) {
		sameOrder := true
		for i := range baseline {
			if baseline[i] != candidate[i] {
				sameOrder = false
				break
			}
		}
		if sameOrder {
			return
		}
	}
	addActivationMapAcceptedDiff(out, field+" display differs")
}

func addActivationMapActionDiff(out *activationMapActionQualityComparison, field string, baseline, candidate any) {
	out.ActionDiffs = append(out.ActionDiffs, fmt.Sprintf("%s differs (baseline=%s candidate=%s)", field, activationCompactJSON(baseline), activationCompactJSON(candidate)))
}

func addActivationMapAcceptedDiff(out *activationMapActionQualityComparison, message string) {
	for _, existing := range out.AcceptedDiffs {
		if existing == message {
			return
		}
	}
	out.AcceptedDiffs = append(out.AcceptedDiffs, message)
}

func activationOrderedStringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func activationStringSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	left := append([]string{}, a...)
	right := append([]string{}, b...)
	sort.Strings(left)
	sort.Strings(right)
	return activationOrderedStringsEqual(left, right)
}

func removeActivationString(values []string, remove string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == remove {
			continue
		}
		out = append(out, value)
	}
	return out
}

func activationCompactJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	text := string(data)
	if len(text) > 180 {
		return text[:180] + "..."
	}
	return text
}

func activationCompareMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", activationCompareModeExact:
		return activationCompareModeExact
	case activationCompareModeJSONSmoke:
		return activationCompareModeJSONSmoke
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func validActivationCompareMode(value string) bool {
	switch value {
	case activationCompareModeExact, activationCompareModeJSONSmoke:
		return true
	default:
		return false
	}
}

func activationCommandArgs(commandName string, args []string, repoPath string, quiet bool) []string {
	out := append([]string{}, args...)
	switch commandName {
	case "recent", "map":
		out = stripActivationFlag(out, "json", false)
		out = stripActivationFlag(out, "quiet", false)
		out = stripActivationFlag(out, "path", true)
		out = append(out, "--json")
		if quiet {
			out = append(out, "--quiet")
		}
		return append(out, "--path", repoPath)
	case "find":
		out = stripActivationFlag(out, "json", false)
		return append(out, "--json")
	case "task":
		out = stripActivationFlag(out, "json", false)
		out = stripActivationFlag(out, repoTargetFlagName, true)
		out = stripActivationFlag(out, "dir", true)
		out = append(out, "--json", "--"+repoTargetFlagName, repoPath, "--dir", activationTaskDirPlaceholder)
		return out
	default:
		out = stripActivationFlag(out, "json", false)
		return append(out, "--json")
	}
}

func stripActivationFlag(args []string, name string, takesValue bool) []string {
	long := "--" + name
	out := make([]string, 0, len(args))
	skipNext := false
	for i, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == long {
			if takesValue && i+1 < len(args) {
				skipNext = true
			}
			continue
		}
		if strings.HasPrefix(arg, long+"=") {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func activationRuntimeCommandArgs(args []string, isolationRoot string) ([]string, []string) {
	out := append([]string{}, args...)
	var normalizationPaths []string
	if strings.TrimSpace(isolationRoot) == "" {
		return out, normalizationPaths
	}
	taskDir := filepath.Join(isolationRoot, "task-workspaces")
	for i, arg := range out {
		if arg == activationTaskDirPlaceholder {
			out[i] = taskDir
			normalizationPaths = append(normalizationPaths, taskDir)
		}
	}
	normalizationPaths = append(normalizationPaths, isolationRoot)
	return out, normalizationPaths
}

func activationCommandRequiresStrictStderr(commandName string, quiet bool) bool {
	return quiet && activationCommandSupportsQuiet(commandName)
}

func activationCommandSupportsQuiet(commandName string) bool {
	switch commandName {
	case "recent", "map":
		return true
	default:
		return false
	}
}

func activationCommandRunsFromRepoRoot(commandName string) bool {
	switch commandName {
	case "find", "task":
		return true
	default:
		return false
	}
}

func runActivationCommand(name string, args []string, repoPath string) ([]byte, []byte, error) {
	var cmd interface {
		SetArgs([]string)
		SetOut(io.Writer)
		SetErr(io.Writer)
		Execute() error
	}
	switch name {
	case "recent":
		cmd = NewRecentCmd()
	case "map":
		cmd = NewMapCmd()
	case "find":
		cmd = NewFindCmd()
	case "task":
		cmd = NewTaskCmd()
	default:
		return nil, nil, fmt.Errorf("unsupported activation matrix command %q; valid values: recent, map, find, task", name)
	}
	if activationCommandRunsFromRepoRoot(name) {
		wd, err := os.Getwd()
		if err != nil {
			return nil, nil, err
		}
		if err := os.Chdir(repoPath); err != nil {
			return nil, nil, err
		}
		defer os.Chdir(wd)
	}
	var stdout, stderr bytes.Buffer
	cmd.SetArgs(args)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	return stdout.Bytes(), stderr.Bytes(), err
}

func runActivationCommandIsolated(name string, args []string, repoPath, indexState string) ([]byte, []byte, []string, error) {
	tempHome, err := os.MkdirTemp("", "devspecs-activation-matrix-*")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create activation matrix home: %w", err)
	}
	defer os.RemoveAll(tempHome)
	oldHome, hadHome := os.LookupEnv("DEVSPECS_HOME")
	if err := os.Setenv("DEVSPECS_HOME", tempHome); err != nil {
		return nil, nil, nil, err
	}
	defer func() {
		if hadHome {
			os.Setenv("DEVSPECS_HOME", oldHome)
		} else {
			os.Unsetenv("DEVSPECS_HOME")
		}
	}()
	if normalizeActivationIndexState(indexState) == activationIndexStateWarm {
		if err := runActivationWarmup(repoPath); err != nil {
			return nil, nil, nil, err
		}
	}
	runtimeArgs, normalizationPaths := activationRuntimeCommandArgs(args, tempHome)
	stdout, stderr, err := runActivationCommand(name, runtimeArgs, repoPath)
	return stdout, stderr, normalizationPaths, err
}

func runExternalActivationCommandIsolated(binary, commandName string, args []string, repoPath, repoID, role, resultDir, indexState string) (activationMatrixCommandRun, []byte) {
	start := time.Now()
	out := activationMatrixCommandRun{
		Binary:     filepath.ToSlash(binary),
		BinaryID:   activationBinaryID(binary),
		IndexState: normalizeActivationIndexState(indexState),
	}
	tempHome, err := os.MkdirTemp("", "devspecs-activation-matrix-*")
	if err != nil {
		out.Error = fmt.Sprintf("create activation matrix home: %v", err)
		return out, nil
	}
	defer os.RemoveAll(tempHome)
	runtimeArgs, normalizationPaths := activationRuntimeCommandArgs(args, tempHome)
	fullArgs := append([]string{commandName}, runtimeArgs...)
	out.Args = fullArgs
	out.normalizationPaths = normalizationPaths
	if out.IndexState == activationIndexStateWarm {
		warmupStart := time.Now()
		warmupStdout, warmupStderr, warmupErr := runExternalActivationWarmup(binary, repoPath, tempHome)
		out.WarmupMillis = int(time.Since(warmupStart).Milliseconds())
		out.WarmupStdoutBytes = len(warmupStdout)
		out.WarmupStderrBytes = len(warmupStderr)
		if warmupErr != nil {
			out.Error = "warm index setup failed: " + warmupErr.Error()
			out.DurationMillis = int(time.Since(start).Milliseconds())
			return out, nil
		}
	}
	cmd := exec.Command(binary, fullArgs...)
	cmd.Env = append(os.Environ(), "DEVSPECS_HOME="+tempHome)
	if activationCommandRunsFromRepoRoot(commandName) {
		cmd.Dir = repoPath
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	out.DurationMillis = int(time.Since(start).Milliseconds())
	out.StdoutBytes = stdout.Len()
	out.StderrBytes = stderr.Len()
	normalized := normalizeActivationOutput(stdout.Bytes(), repoPath, normalizationPaths...)
	out.StdoutSHA256 = activationSHA256(normalized)
	out.StderrSHA256 = activationSHA256(stderr.Bytes())
	out.ValidJSON = json.Valid(bytes.TrimSpace(stdout.Bytes()))
	if exitErr, ok := err.(*exec.ExitError); ok {
		out.ExitCode = exitErr.ExitCode()
	} else if err != nil {
		out.ExitCode = -1
		out.Error = err.Error()
	} else {
		out.ExitCode = 0
	}
	if resultDir != "" {
		stdoutPath, stderrPath, writeErr := writeActivationBinaryOutputs(resultDir, role, repoID, commandName, stdout.Bytes(), stderr.Bytes())
		if writeErr != nil && out.Error == "" {
			out.Error = writeErr.Error()
		}
		out.StdoutPath = filepath.ToSlash(stdoutPath)
		out.StderrPath = filepath.ToSlash(stderrPath)
	}
	return out, stdout.Bytes()
}

func runActivationWarmup(repoPath string) error {
	cmd := NewScanCmd()
	cmd.SetArgs([]string{"--path", repoPath, "--quiet"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func runExternalActivationWarmup(binary, repoPath, devspecsHome string) ([]byte, []byte, error) {
	cmd := exec.Command(binary, "scan", "--path", repoPath, "--quiet")
	cmd.Env = append(os.Environ(), "DEVSPECS_HOME="+devspecsHome)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func writeActivationBinaryOutputs(resultDir, role, repoID, commandName string, stdout, stderr []byte) (string, string, error) {
	dir := filepath.Join(resultDir, "outputs", safeFilenamePart(role))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	prefix := strings.Join([]string{safeFilenamePart(repoID), safeFilenamePart(commandName)}, "-")
	stdoutPath := filepath.Join(dir, prefix+".stdout.json")
	stderrPath := filepath.Join(dir, prefix+".stderr.txt")
	if err := os.WriteFile(stdoutPath, stdout, 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(stderrPath, stderr, 0o644); err != nil {
		return "", "", err
	}
	return stdoutPath, stderrPath, nil
}

func activationGoldenPath(root, profile, repoID, command string, args []string) string {
	name := strings.Join([]string{
		safeFilenamePart(repoID),
		safeFilenamePart(command),
		activationCommandArgsHash(command, args),
	}, "-")
	return filepath.Join(root, safeFilenamePart(profile), name+".json")
}

func activationCommandArgsHash(command string, args []string) string {
	key, _ := json.Marshal(struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}{
		Command: command,
		Args:    args,
	})
	hash := activationSHA256(key)
	if len(hash) > 10 {
		return hash[:10]
	}
	return hash
}

func activationBinaryCompare(opts activationMatrixOptions) bool {
	return strings.TrimSpace(opts.BaselineBin) != "" || strings.TrimSpace(opts.CandidateBin) != ""
}

func activationBinaryPathAndID(path string) (string, string, error) {
	if strings.TrimSpace(path) == "" {
		return "", "", nil
	}
	abs, err := filepath.Abs(os.ExpandEnv(path))
	if err != nil {
		return "", "", fmt.Errorf("resolve activation binary %q: %w", path, err)
	}
	if info, err := os.Stat(abs); err != nil {
		return "", "", fmt.Errorf("stat activation binary %q: %w", abs, err)
	} else if info.IsDir() {
		return "", "", fmt.Errorf("activation binary %q is a directory", abs)
	}
	return filepath.Clean(abs), activationBinaryID(abs), nil
}

func activationBinaryID(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.Base(path) + ":" + activationSHA256(data)
}

func normalizeActivationCloneMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "full", "blobless", "shallow":
		return value
	default:
		return value
	}
}

func validateActivationCloneMode(value string) error {
	switch normalizeActivationCloneMode(value) {
	case "", "full", "blobless", "shallow":
		return nil
	default:
		return fmt.Errorf("--activation-clone-mode must be one of full, blobless, shallow")
	}
}

func normalizeActivationIndexState(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", activationIndexStateCold, activationIndexStateWarm:
		return value
	default:
		return value
	}
}

func validateActivationIndexState(value string) error {
	switch normalizeActivationIndexState(value) {
	case "", activationIndexStateCold, activationIndexStateWarm:
		return nil
	default:
		return fmt.Errorf("activation index state must be one of cold, warm")
	}
}

func activationRoleIndexState(defaultState, roleState string) string {
	roleState = normalizeActivationIndexState(roleState)
	if roleState != "" {
		return roleState
	}
	defaultState = normalizeActivationIndexState(defaultState)
	if defaultState == "" {
		return activationIndexStateCold
	}
	return defaultState
}

func activationReadRepoMetadata(repoPath string, repo activationMatrixRepo, selectedCloneMode string) activationRepoMetadata {
	cloneMode := normalizeActivationCloneMode(repo.CloneMode)
	if cloneMode == "" {
		cloneMode = selectedCloneMode
	}
	meta := activationRepoMetadata{
		URL:         repo.URL,
		ExpectedSHA: strings.TrimSpace(repo.CommitSHA),
		CloneMode:   cloneMode,
	}
	if strings.TrimSpace(meta.URL) == "" {
		meta.URL = activationGitConfig(repoPath, "remote.origin.url")
	}
	sha := activationGitOutput(repoPath, "rev-parse", "HEAD")
	if sha == "" {
		meta.GitAvailable = false
		meta.Error = "repo git metadata unavailable"
		return meta
	}
	meta.GitAvailable = true
	meta.CommitSHA = sha
	if meta.ExpectedSHA != "" && !activationSHAMatches(meta.CommitSHA, meta.ExpectedSHA) {
		meta.CommitMismatch = true
		meta.Error = fmt.Sprintf("repo %q HEAD %s does not match manifest commit %s", repo.ID, meta.CommitSHA, meta.ExpectedSHA)
	}
	shallow := activationGitOutput(repoPath, "rev-parse", "--is-shallow-repository")
	meta.IsShallow = strings.EqualFold(shallow, "true")
	if meta.IsShallow && cloneMode != "shallow" {
		meta.Error = fmt.Sprintf("repo %q is shallow but activation clone mode is %q", repo.ID, cloneMode)
	}
	return meta
}

func activationGitOutput(repoPath string, args ...string) string {
	cmdArgs := append([]string{"-C", repoPath}, args...)
	out, err := exec.Command("git", cmdArgs...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func activationGitConfig(repoPath, key string) string {
	return activationGitOutput(repoPath, "config", "--get", key)
}

func activationSHAMatches(actual, expected string) bool {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	return actual == expected || strings.HasPrefix(actual, expected) || strings.HasPrefix(expected, actual)
}

func recordActivationRepoSet(set *activationMatrixRepoSet, meta activationRepoMetadata) {
	set.Total++
	if !meta.GitAvailable {
		set.MissingGit++
	}
	if meta.CommitSHA == "" {
		set.MissingCommit++
	}
	if meta.CommitMismatch {
		set.MismatchedHead++
	}
	switch meta.CloneMode {
	case "shallow":
		set.Shallow++
	case "blobless":
		set.Blobless++
	default:
		set.Full++
	}
}

func activationTimingSummary(cases []activationMatrixCaseResult) activationMatrixTiming {
	var commandDurations, baselineDurations, candidateDurations []int
	for _, c := range cases {
		if c.DurationMillis > 0 {
			commandDurations = append(commandDurations, c.DurationMillis)
		}
		if c.Baseline != nil {
			baselineDurations = append(baselineDurations, c.Baseline.DurationMillis)
		}
		if c.Candidate != nil {
			candidateDurations = append(candidateDurations, c.Candidate.DurationMillis)
		}
	}
	return activationMatrixTiming{
		Command:   activationTimingStats(commandDurations),
		Baseline:  activationTimingStats(baselineDurations),
		Candidate: activationTimingStats(candidateDurations),
	}
}

func activationTimingStats(values []int) activationMatrixTimingStats {
	if len(values) == 0 {
		return activationMatrixTimingStats{}
	}
	sort.Ints(values)
	total := 0
	for _, value := range values {
		total += value
	}
	return activationMatrixTimingStats{
		Total: total,
		P50:   activationPercentile(values, 50),
		P95:   activationPercentile(values, 95),
		Max:   values[len(values)-1],
	}
}

func activationSlowestCases(cases []activationMatrixCaseResult, limit int) []activationMatrixSlowCase {
	if limit <= 0 || len(cases) == 0 {
		return nil
	}
	slowest := make([]activationMatrixSlowCase, 0, len(cases))
	for _, c := range cases {
		item := activationMatrixSlowCase{
			RepoID:         c.RepoID,
			Command:        c.Command,
			Status:         c.Status,
			DurationMillis: c.DurationMillis,
		}
		if c.Baseline != nil {
			item.BaselineMillis = c.Baseline.DurationMillis
		}
		if c.Candidate != nil {
			item.CandidateMillis = c.Candidate.DurationMillis
		}
		if item.BaselineMillis > 0 && item.CandidateMillis > 0 {
			item.CandidateDeltaMS = item.CandidateMillis - item.BaselineMillis
			item.CandidateSpeedupMS = item.BaselineMillis - item.CandidateMillis
		}
		slowest = append(slowest, item)
	}
	sort.SliceStable(slowest, func(i, j int) bool {
		if slowest[i].DurationMillis == slowest[j].DurationMillis {
			if slowest[i].RepoID == slowest[j].RepoID {
				return slowest[i].Command < slowest[j].Command
			}
			return slowest[i].RepoID < slowest[j].RepoID
		}
		return slowest[i].DurationMillis > slowest[j].DurationMillis
	})
	if len(slowest) > limit {
		slowest = slowest[:limit]
	}
	return slowest
}

func activationPercentile(sorted []int, p int) int {
	if len(sorted) == 0 {
		return 0
	}
	rank := (p*len(sorted) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

func writeActivationMatrixResultFile(resultDir string, result *activationMatrixResult) error {
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(resultDir, activationMatrixDefaultResultFilename), data, 0o644)
}

func normalizeActivationOutput(output []byte, repoPath string, extraPaths ...string) []byte {
	normalized := bytes.ReplaceAll(output, []byte("\r\n"), []byte("\n"))
	replacements := activationPathReplacements(repoPath)
	for _, value := range replacements {
		if value == "" {
			continue
		}
		normalized = bytes.ReplaceAll(normalized, []byte(value), []byte("<REPO_ROOT>"))
	}
	for _, path := range extraPaths {
		for _, value := range activationPathReplacements(path) {
			if value == "" {
				continue
			}
			normalized = bytes.ReplaceAll(normalized, []byte(value), []byte("<ACTIVATION_TMP>"))
		}
	}
	if len(normalized) > 0 && normalized[len(normalized)-1] != '\n' {
		normalized = append(normalized, '\n')
	}
	return normalized
}

func activationPathReplacements(repoPath string) []string {
	replacements := []string{repoPath, filepath.ToSlash(repoPath)}
	for _, value := range append([]string{}, replacements...) {
		if value == "" {
			continue
		}
		encoded, err := json.Marshal(value)
		if err == nil && len(encoded) >= 2 {
			replacements = append(replacements, string(encoded[1:len(encoded)-1]))
		}
	}
	return replacements
}

func activationSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func activationFirstDiff(expected, got []byte) string {
	expectedLines := strings.Split(string(expected), "\n")
	gotLines := strings.Split(string(got), "\n")
	limit := len(expectedLines)
	if len(gotLines) > limit {
		limit = len(gotLines)
	}
	for i := 0; i < limit; i++ {
		want, have := "", ""
		if i < len(expectedLines) {
			want = expectedLines[i]
		}
		if i < len(gotLines) {
			have = gotLines[i]
		}
		if want != have {
			return fmt.Sprintf("first difference at line %d\nwant: %s\ngot:  %s", i+1, trimActivationDiffLine(want), trimActivationDiffLine(have))
		}
	}
	return ""
}

func trimActivationDiffLine(line string) string {
	line = strings.TrimSpace(line)
	if len(line) > 220 {
		return line[:220] + "..."
	}
	return line
}

func firstActivationOutputBytes(data []byte, limit int) string {
	if len(data) <= limit {
		return string(data)
	}
	return string(data[:limit]) + "..."
}

func writeActivationMatrixJSON(out io.Writer, result *activationMatrixResult) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func writeActivationMatrixText(out io.Writer, result *activationMatrixResult) {
	fmt.Fprintf(out, "DevSpecs Activation Matrix (%s)\n", result.Profile)
	fmt.Fprintf(out, "Manifest: %s\n", result.ManifestPath)
	fmt.Fprintf(out, "Goldens: %s\n", result.GoldenDir)
	fmt.Fprintf(out, "Summary: %d passed, %d updated, %d missing, %d failed / %d total\n\n",
		result.Summary.Passed, result.Summary.Updated, result.Summary.Missing, result.Summary.Failed, result.Summary.Total)
	cases := append([]activationMatrixCaseResult{}, result.Cases...)
	sort.SliceStable(cases, func(i, j int) bool {
		if cases[i].RepoID != cases[j].RepoID {
			return cases[i].RepoID < cases[j].RepoID
		}
		return cases[i].Command < cases[j].Command
	})
	for _, c := range cases {
		fmt.Fprintf(out, "- %s/%s %s %s (%d ms)\n", c.Profile, c.RepoID, c.Command, c.Status, c.DurationMillis)
		if c.Error != "" {
			fmt.Fprintf(out, "  Error: %s\n", c.Error)
		}
		if c.MapActionQuality != nil {
			fmt.Fprintf(out, "  Map action-quality: %s\n", c.MapActionQuality.Status)
		}
		if c.Diff != "" {
			fmt.Fprintf(out, "  %s\n", strings.ReplaceAll(c.Diff, "\n", "\n  "))
		}
	}
	for _, note := range result.Notes {
		fmt.Fprintf(out, "\nNote: %s\n", note)
	}
}
