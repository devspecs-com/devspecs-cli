package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	scanpkg "github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

const (
	activationScanBenchmarkSchemaVersion = "devspecs.activation_scan_benchmark.v1"
	activationScanBenchmarkResultName    = "activation-scan-benchmark-result.json"
)

type activationScanBenchmarkOptions struct {
	Profile            string
	ResultDir          string
	CloneMode          string
	IndexState         string
	BaselineResult     string
	MaxRegressionRatio float64
	MaxRegressionMS    int
	MaxCaseMS          int
	Generated          time.Time
}

type activationScanBenchmarkResult struct {
	Schema             string                         `json:"schema"`
	ManifestPath       string                         `json:"manifest_path"`
	SuiteID            string                         `json:"suite_id,omitempty"`
	SuiteVersion       string                         `json:"suite_version,omitempty"`
	Profile            string                         `json:"profile"`
	CloneMode          string                         `json:"clone_mode"`
	ResultDir          string                         `json:"result_dir,omitempty"`
	ResultPath         string                         `json:"result_path,omitempty"`
	IndexState         string                         `json:"index_state"`
	BaselineResult     string                         `json:"baseline_result,omitempty"`
	MaxRegressionRatio float64                        `json:"max_regression_ratio,omitempty"`
	MaxRegressionMS    int                            `json:"max_regression_ms,omitempty"`
	MaxCaseMS          int                            `json:"max_case_ms,omitempty"`
	GeneratedAt        string                         `json:"generated_at"`
	RepoSet            activationMatrixRepoSet        `json:"repo_set"`
	Summary            activationScanBenchmarkSummary `json:"summary"`
	Timing             activationMatrixTimingStats    `json:"scan_ms,omitempty"`
	PhaseTiming        []activationScanPhaseStats     `json:"phase_ms,omitempty"`
	SlowestCases       []activationScanSlowCase       `json:"slowest_cases,omitempty"`
	Cases              []activationScanBenchmarkCase  `json:"cases"`
	Failures           []string                       `json:"failures,omitempty"`
	Notes              []string                       `json:"notes,omitempty"`
}

type activationScanBenchmarkSummary struct {
	Total              int `json:"total"`
	Passed             int `json:"passed"`
	Failed             int `json:"failed"`
	RegressionFailures int `json:"regression_failures,omitempty"`
	DurationMillis     int `json:"duration_ms"`
}

type activationScanBenchmarkCase struct {
	RepoID         string                              `json:"repo_id"`
	RepoPath       string                              `json:"repo_path"`
	RepoURL        string                              `json:"repo_url,omitempty"`
	RepoCommitSHA  string                              `json:"repo_commit_sha,omitempty"`
	RepoCloneMode  string                              `json:"repo_clone_mode,omitempty"`
	RepoIsShallow  bool                                `json:"repo_is_shallow,omitempty"`
	Profile        string                              `json:"profile"`
	Command        string                              `json:"command"`
	Args           []string                            `json:"args"`
	IndexState     string                              `json:"index_state"`
	Status         string                              `json:"status"`
	DurationMillis int                                 `json:"duration_ms"`
	WarmupMillis   int                                 `json:"warmup_ms,omitempty"`
	StdoutBytes    int                                 `json:"stdout_bytes"`
	StderrBytes    int                                 `json:"stderr_bytes"`
	StdoutPath     string                              `json:"stdout_path,omitempty"`
	StderrPath     string                              `json:"stderr_path,omitempty"`
	DBSizeBytes    int64                               `json:"db_size_bytes,omitempty"`
	TableRows      map[string]int                      `json:"table_rows,omitempty"`
	TableGroups    map[string]int                      `json:"table_groups,omitempty"`
	Found          map[string]int                      `json:"found,omitempty"`
	New            int                                 `json:"new,omitempty"`
	Updated        int                                 `json:"updated,omitempty"`
	Unchanged      int                                 `json:"unchanged,omitempty"`
	Traversal      *scanpkg.TraversalDiagnostics       `json:"traversal,omitempty"`
	EvidenceGraph  *scanpkg.EvidenceGraphDiagnostics   `json:"evidence_graph,omitempty"`
	SourceManifest *scanpkg.SourceManifestDiagnostics  `json:"source_manifest,omitempty"`
	PhaseTiming    *scanpkg.PhaseTimingDiagnostics     `json:"phase_timing,omitempty"`
	Regression     *activationScanRegressionComparison `json:"regression,omitempty"`
	Error          string                              `json:"error,omitempty"`
}

type activationScanRegressionComparison struct {
	BaselineDurationMillis int            `json:"baseline_duration_ms,omitempty"`
	DurationDeltaMS        int            `json:"duration_delta_ms,omitempty"`
	DurationRatio          float64        `json:"duration_ratio,omitempty"`
	BaselineDBSizeBytes    int64          `json:"baseline_db_size_bytes,omitempty"`
	DBSizeDeltaBytes       int64          `json:"db_size_delta_bytes,omitempty"`
	TableGroupDeltas       map[string]int `json:"table_group_deltas,omitempty"`
	Status                 string         `json:"status"`
	Reason                 string         `json:"reason,omitempty"`
}

type activationScanPhaseStats struct {
	Phase  string                      `json:"phase"`
	Timing activationMatrixTimingStats `json:"timing"`
}

type activationScanSlowCase struct {
	RepoID         string `json:"repo_id"`
	Command        string `json:"command"`
	Status         string `json:"status"`
	DurationMillis int    `json:"duration_ms"`
	WarmupMillis   int    `json:"warmup_ms,omitempty"`
}

func runActivationScanBenchmark(manifestPath string, opts activationScanBenchmarkOptions) (*activationScanBenchmarkResult, error) {
	start := time.Now()
	if strings.TrimSpace(opts.Profile) == "" {
		opts.Profile = "skinny"
	}
	opts.CloneMode = normalizeActivationCloneMode(opts.CloneMode)
	if opts.CloneMode == "" {
		opts.CloneMode = "full"
	}
	opts.IndexState = normalizeActivationIndexState(opts.IndexState)
	if opts.IndexState == "" {
		opts.IndexState = activationIndexStateCold
	}
	if opts.Generated.IsZero() {
		opts.Generated = nowUTC()
	}
	manifestAbs, err := filepath.Abs(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("resolve activation scan benchmark manifest: %w", err)
	}
	manifest, err := loadActivationMatrixManifest(manifestAbs)
	if err != nil {
		return nil, err
	}
	resultDir := ""
	if strings.TrimSpace(opts.ResultDir) != "" {
		resultDir, err = filepath.Abs(opts.ResultDir)
		if err != nil {
			return nil, fmt.Errorf("resolve activation scan benchmark result dir: %w", err)
		}
	}
	baselinePath := strings.TrimSpace(opts.BaselineResult)
	baselineByCase, err := loadActivationScanBenchmarkBaseline(baselinePath)
	if err != nil {
		return nil, err
	}
	if baselinePath != "" {
		baselineAbs, err := filepath.Abs(os.ExpandEnv(baselinePath))
		if err != nil {
			return nil, fmt.Errorf("resolve activation scan benchmark baseline result: %w", err)
		}
		baselinePath = filepath.ToSlash(baselineAbs)
	}
	result := &activationScanBenchmarkResult{
		Schema:             activationScanBenchmarkSchemaVersion,
		ManifestPath:       filepath.ToSlash(manifestAbs),
		SuiteID:            manifest.SuiteID,
		SuiteVersion:       manifest.SuiteVersion,
		Profile:            opts.Profile,
		CloneMode:          opts.CloneMode,
		ResultDir:          filepath.ToSlash(resultDir),
		IndexState:         opts.IndexState,
		BaselineResult:     baselinePath,
		MaxRegressionRatio: opts.MaxRegressionRatio,
		MaxRegressionMS:    opts.MaxRegressionMS,
		MaxCaseMS:          opts.MaxCaseMS,
		GeneratedAt:        opts.Generated.UTC().Format(time.RFC3339),
	}
	if resultDir != "" {
		result.ResultPath = filepath.ToSlash(filepath.Join(resultDir, activationScanBenchmarkResultName))
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
		for _, spec := range activationScanBenchmarkCommands(repo.Commands) {
			caseResult := runActivationScanBenchmarkCase(repo, repoPath, meta, spec, opts, resultDir)
			applyActivationScanBenchmarkThresholds(&caseResult, baselineByCase, opts)
			result.Cases = append(result.Cases, caseResult)
			result.Summary.Total++
			if caseResult.Status == "passed" {
				result.Summary.Passed++
			} else {
				result.Summary.Failed++
				if caseResult.Regression != nil && caseResult.Regression.Status == "failed" {
					result.Summary.RegressionFailures++
				}
				result.Failures = append(result.Failures, activationScanFailureSummary(caseResult))
			}
		}
	}
	if result.Summary.Total == 0 {
		result.Notes = append(result.Notes, fmt.Sprintf("no repos matched activation profile %q", opts.Profile))
	}
	result.Summary.DurationMillis = int(time.Since(start).Milliseconds())
	result.Timing = activationScanBenchmarkTiming(result.Cases)
	result.PhaseTiming = activationScanPhaseTimingStats(result.Cases)
	result.SlowestCases = activationScanSlowestCases(result.Cases, 5)
	if resultDir != "" {
		if err := writeActivationScanBenchmarkResultFile(resultDir, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func activationScanBenchmarkCommands(commands []activationMatrixCommand) []activationMatrixCommand {
	var out []activationMatrixCommand
	for _, command := range commands {
		if strings.EqualFold(strings.TrimSpace(command.Name), "scan") {
			out = append(out, command)
		}
	}
	if len(out) == 0 {
		return []activationMatrixCommand{{Name: "scan"}}
	}
	return out
}

func runActivationScanBenchmarkCase(repo activationMatrixRepo, repoPath string, meta activationRepoMetadata, spec activationMatrixCommand, opts activationScanBenchmarkOptions, resultDir string) activationScanBenchmarkCase {
	commandName := "scan"
	args := activationScanCommandArgs(spec.Args, repoPath)
	out := activationScanBenchmarkCase{
		RepoID:        repo.ID,
		RepoPath:      filepath.ToSlash(repoPath),
		RepoURL:       meta.URL,
		RepoCommitSHA: meta.CommitSHA,
		RepoCloneMode: meta.CloneMode,
		RepoIsShallow: meta.IsShallow,
		Profile:       opts.Profile,
		Command:       commandName,
		Args:          args,
		IndexState:    opts.IndexState,
	}
	if meta.Error != "" {
		out.Status = "failed"
		out.Error = meta.Error
		return out
	}
	tempHome, err := os.MkdirTemp("", "devspecs-activation-scan-*")
	if err != nil {
		out.Status = "failed"
		out.Error = fmt.Sprintf("create activation scan home: %v", err)
		return out
	}
	defer os.RemoveAll(tempHome)
	if opts.IndexState == activationIndexStateWarm {
		warmupStart := time.Now()
		warmupStdout, warmupStderr, warmupErr := runActivationScanCommandWithHome([]string{"--path", repoPath, "--quiet"}, tempHome)
		out.WarmupMillis = int(time.Since(warmupStart).Milliseconds())
		if warmupErr != nil {
			out.Status = "failed"
			out.StdoutBytes = len(warmupStdout)
			out.StderrBytes = len(warmupStderr)
			out.Error = "warm index setup failed: " + warmupErr.Error()
			return out
		}
		if len(warmupStderr) > 0 {
			out.Status = "failed"
			out.StdoutBytes = len(warmupStdout)
			out.StderrBytes = len(warmupStderr)
			out.Error = "warm index setup wrote stderr"
			return out
		}
	}
	start := time.Now()
	stdout, stderr, runErr := runActivationScanCommandWithHome(args, tempHome)
	out.DurationMillis = int(time.Since(start).Milliseconds())
	out.StdoutBytes = len(stdout)
	out.StderrBytes = len(stderr)
	if resultDir != "" {
		stdoutPath, stderrPath, writeErr := writeActivationScanOutputs(resultDir, repo.ID, spec.Args, stdout, stderr)
		if writeErr != nil && runErr == nil {
			runErr = writeErr
		}
		out.StdoutPath = filepath.ToSlash(stdoutPath)
		out.StderrPath = filepath.ToSlash(stderrPath)
	}
	if runErr != nil {
		out.Status = "failed"
		out.Error = runErr.Error()
		return out
	}
	if len(stderr) > 0 {
		out.Status = "failed"
		out.Error = "scan benchmark wrote stderr"
		return out
	}
	var scanResult scanpkg.Result
	if err := json.Unmarshal(bytes.TrimSpace(stdout), &scanResult); err != nil {
		out.Status = "failed"
		out.Error = "parse scan JSON: " + err.Error()
		return out
	}
	out.Found = activationCloneIntMap(scanResult.Found)
	out.New = scanResult.New
	out.Updated = scanResult.Updated
	out.Unchanged = scanResult.Unchanged
	out.Traversal = scanResult.Traversal
	out.EvidenceGraph = scanResult.EvidenceGraph
	out.SourceManifest = scanResult.SourceManifest
	out.PhaseTiming = scanResult.PhaseTiming
	dbPath := filepath.Join(tempHome, "devspecs.db")
	out.DBSizeBytes = activationIndexFileBytes(dbPath)
	out.TableRows, out.TableGroups = activationScanDBRows(dbPath)
	out.Status = "passed"
	return out
}

func activationScanCommandArgs(args []string, repoPath string) []string {
	out := append([]string{}, args...)
	out = stripActivationFlag(out, "json", false)
	out = stripActivationFlag(out, "quiet", false)
	out = stripActivationFlag(out, "path", true)
	out = stripActivationFlag(out, "phase-timing", false)
	out = append(out, "--json", "--quiet", "--phase-timing", "--path", repoPath)
	return out
}

func runActivationScanCommandWithHome(args []string, devspecsHome string) ([]byte, []byte, error) {
	oldHome, hadHome := os.LookupEnv("DEVSPECS_HOME")
	if err := os.Setenv("DEVSPECS_HOME", devspecsHome); err != nil {
		return nil, nil, err
	}
	defer func() {
		if hadHome {
			os.Setenv("DEVSPECS_HOME", oldHome)
		} else {
			os.Unsetenv("DEVSPECS_HOME")
		}
	}()
	cmd := NewScanCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetArgs(args)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	return stdout.Bytes(), stderr.Bytes(), err
}

func activationIndexFileBytes(dbPath string) int64 {
	var total int64
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
	}
	return total
}

func activationScanDBRows(dbPath string) (map[string]int, map[string]int) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, nil
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return nil, nil
	}
	defer db.Close()
	tables := []string{
		"repos", "artifacts", "artifact_revisions", "sources", "links",
		"artifact_todos", "artifact_criteria", "artifact_tags", "artifact_sections", "artifact_sections_fts",
		"concepts", "concept_mentions", "artifact_edges",
		"git_commits", "git_commit_files",
		"source_manifest", "source_manifest_symbols", "source_manifest_tests", "source_manifest_imports", "source_manifest_fts",
		"task_checkpoint_facts",
	}
	rows := map[string]int{}
	for _, table := range tables {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err == nil {
			rows[table] = count
		}
	}
	groups := map[string]int{
		"metadata":        rows["repos"],
		"artifacts":       rows["artifacts"] + rows["artifact_revisions"] + rows["sources"],
		"artifact_detail": rows["links"] + rows["artifact_todos"] + rows["artifact_criteria"] + rows["artifact_tags"] + rows["artifact_sections"] + rows["artifact_sections_fts"],
		"evidence_graph":  rows["concepts"] + rows["concept_mentions"] + rows["artifact_edges"],
		"git":             rows["git_commits"] + rows["git_commit_files"],
		"source_manifest": rows["source_manifest"] + rows["source_manifest_symbols"] + rows["source_manifest_tests"] + rows["source_manifest_imports"] + rows["source_manifest_fts"],
		"task_facts":      rows["task_checkpoint_facts"],
	}
	return rows, groups
}

func loadActivationScanBenchmarkBaseline(path string) (map[string]activationScanBenchmarkCase, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	abs, err := filepath.Abs(os.ExpandEnv(path))
	if err != nil {
		return nil, fmt.Errorf("resolve activation scan benchmark baseline result: %w", err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read activation scan benchmark baseline result: %w", err)
	}
	var baseline activationScanBenchmarkResult
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("parse activation scan benchmark baseline result: %w", err)
	}
	out := map[string]activationScanBenchmarkCase{}
	for _, c := range baseline.Cases {
		out[activationScanCaseKey(c)] = c
	}
	return out, nil
}

func applyActivationScanBenchmarkThresholds(c *activationScanBenchmarkCase, baseline map[string]activationScanBenchmarkCase, opts activationScanBenchmarkOptions) {
	if c.Status != "passed" {
		return
	}
	if opts.MaxCaseMS > 0 && c.DurationMillis > opts.MaxCaseMS {
		c.Status = "failed"
		c.Error = fmt.Sprintf("scan duration %dms exceeded --activation-scan-max-case-ms=%d", c.DurationMillis, opts.MaxCaseMS)
		c.Regression = &activationScanRegressionComparison{Status: "failed", Reason: c.Error}
		return
	}
	if len(baseline) == 0 {
		return
	}
	base, ok := baseline[activationScanCaseKey(*c)]
	if !ok {
		c.Regression = &activationScanRegressionComparison{Status: "missing_baseline", Reason: "no matching baseline case"}
		return
	}
	comparison := activationScanRegressionComparison{
		BaselineDurationMillis: base.DurationMillis,
		DurationDeltaMS:        c.DurationMillis - base.DurationMillis,
		BaselineDBSizeBytes:    base.DBSizeBytes,
		DBSizeDeltaBytes:       c.DBSizeBytes - base.DBSizeBytes,
		TableGroupDeltas:       activationIntDeltas(base.TableGroups, c.TableGroups),
		Status:                 "passed",
	}
	if base.DurationMillis > 0 {
		comparison.DurationRatio = float64(c.DurationMillis) / float64(base.DurationMillis)
	}
	ratioLimit := opts.MaxRegressionRatio
	if ratioLimit > 0 && base.DurationMillis > 0 {
		maxAllowed := int(float64(base.DurationMillis)*ratioLimit) + opts.MaxRegressionMS
		if c.DurationMillis > maxAllowed {
			comparison.Status = "failed"
			comparison.Reason = fmt.Sprintf("scan duration %dms exceeded baseline %dms with ratio %.2f and slack %dms", c.DurationMillis, base.DurationMillis, ratioLimit, opts.MaxRegressionMS)
			c.Status = "failed"
			c.Error = comparison.Reason
		}
	}
	c.Regression = &comparison
}

func activationScanCaseKey(c activationScanBenchmarkCase) string {
	key, _ := json.Marshal(struct {
		RepoID     string   `json:"repo_id"`
		Command    string   `json:"command"`
		Args       []string `json:"args"`
		IndexState string   `json:"index_state"`
	}{
		RepoID:     c.RepoID,
		Command:    c.Command,
		Args:       stripActivationFlag(c.Args, "path", true),
		IndexState: c.IndexState,
	})
	return string(key)
}

func activationIntDeltas(base, current map[string]int) map[string]int {
	keys := map[string]bool{}
	for key := range base {
		keys[key] = true
	}
	for key := range current {
		keys[key] = true
	}
	if len(keys) == 0 {
		return nil
	}
	out := map[string]int{}
	for key := range keys {
		out[key] = current[key] - base[key]
	}
	return out
}

func activationCloneIntMap(values map[string]int) map[string]int {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]int, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func activationScanFailureSummary(c activationScanBenchmarkCase) string {
	if c.Error != "" {
		return fmt.Sprintf("%s %s failed: %s", c.RepoID, c.Command, c.Error)
	}
	return fmt.Sprintf("%s %s failed", c.RepoID, c.Command)
}

func activationScanBenchmarkTiming(cases []activationScanBenchmarkCase) activationMatrixTimingStats {
	var durations []int
	for _, c := range cases {
		if c.DurationMillis > 0 {
			durations = append(durations, c.DurationMillis)
		}
	}
	return activationTimingStats(durations)
}

func activationScanPhaseTimingStats(cases []activationScanBenchmarkCase) []activationScanPhaseStats {
	byPhase := map[string][]int{}
	for _, c := range cases {
		if c.PhaseTiming == nil {
			continue
		}
		for _, phase := range c.PhaseTiming.Phases {
			key := phase.Name
			if phase.Adapter != "" {
				key += ":" + phase.Adapter
			}
			if phase.DurationMS > 0 {
				byPhase[key] = append(byPhase[key], int(phase.DurationMS))
			}
		}
	}
	out := make([]activationScanPhaseStats, 0, len(byPhase))
	for phase, durations := range byPhase {
		out = append(out, activationScanPhaseStats{Phase: phase, Timing: activationTimingStats(durations)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Timing.Max == out[j].Timing.Max {
			return out[i].Phase < out[j].Phase
		}
		return out[i].Timing.Max > out[j].Timing.Max
	})
	return out
}

func activationScanSlowestCases(cases []activationScanBenchmarkCase, limit int) []activationScanSlowCase {
	if limit <= 0 || len(cases) == 0 {
		return nil
	}
	out := make([]activationScanSlowCase, 0, len(cases))
	for _, c := range cases {
		out = append(out, activationScanSlowCase{
			RepoID:         c.RepoID,
			Command:        c.Command,
			Status:         c.Status,
			DurationMillis: c.DurationMillis,
			WarmupMillis:   c.WarmupMillis,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DurationMillis == out[j].DurationMillis {
			return out[i].RepoID < out[j].RepoID
		}
		return out[i].DurationMillis > out[j].DurationMillis
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func writeActivationScanOutputs(resultDir, repoID string, args []string, stdout, stderr []byte) (string, string, error) {
	dir := filepath.Join(resultDir, "outputs", "scan")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	name := strings.Join([]string{
		safeFilenamePart(repoID),
		"scan",
		activationCommandArgsHash("scan", args),
	}, "-")
	stdoutPath := filepath.Join(dir, name+".stdout.json")
	stderrPath := filepath.Join(dir, name+".stderr.txt")
	if err := os.WriteFile(stdoutPath, stdout, 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(stderrPath, stderr, 0o644); err != nil {
		return "", "", err
	}
	return stdoutPath, stderrPath, nil
}

func writeActivationScanBenchmarkResultFile(resultDir string, result *activationScanBenchmarkResult) error {
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(resultDir, activationScanBenchmarkResultName), data, 0o644)
}

func writeActivationScanBenchmarkJSON(out io.Writer, result *activationScanBenchmarkResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}

func writeActivationScanBenchmarkText(out io.Writer, result *activationScanBenchmarkResult) {
	fmt.Fprintf(out, "Activation scan benchmark: %s profile=%s index_state=%s\n", result.SuiteID, result.Profile, result.IndexState)
	fmt.Fprintf(out, "Cases: %d passed, %d failed", result.Summary.Passed, result.Summary.Failed)
	if result.Summary.RegressionFailures > 0 {
		fmt.Fprintf(out, ", %d regression failure(s)", result.Summary.RegressionFailures)
	}
	fmt.Fprintln(out)
	if result.Timing.Max > 0 {
		fmt.Fprintf(out, "Timing ms: p50=%d p95=%d max=%d total=%d\n", result.Timing.P50, result.Timing.P95, result.Timing.Max, result.Timing.Total)
	}
	if len(result.SlowestCases) > 0 {
		fmt.Fprintln(out, "Slowest cases:")
		for _, c := range result.SlowestCases {
			fmt.Fprintf(out, "- %s %s: %dms status=%s", c.RepoID, c.Command, c.DurationMillis, c.Status)
			if c.WarmupMillis > 0 {
				fmt.Fprintf(out, " warmup=%dms", c.WarmupMillis)
			}
			fmt.Fprintln(out)
		}
	}
	for _, failure := range result.Failures {
		fmt.Fprintf(out, "Failure: %s\n", failure)
	}
}
