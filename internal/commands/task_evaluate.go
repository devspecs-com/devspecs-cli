package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type taskEvaluateOptions struct {
	Dir    string
	AsJSON bool
}

type taskEvaluationOutput struct {
	TaskID             string                    `json:"task_id"`
	Query              string                    `json:"query"`
	Hits               []string                  `json:"hits,omitempty"`
	Misses             []string                  `json:"misses,omitempty"`
	Noise              []string                  `json:"noise,omitempty"`
	CompanionMisses    []string                  `json:"companion_misses,omitempty"`
	ReceiptMisses      []string                  `json:"receipt_misses,omitempty"`
	ConfidenceMismatch bool                      `json:"confidence_mismatch"`
	Metrics            taskMetrics               `json:"metrics"`
	UsefulnessClass    string                    `json:"usefulness_class"`
	Notes              []string                  `json:"notes,omitempty"`
	Observed           taskObservedPaths         `json:"observed_context"`
	CheckpointSummary  taskCheckpointReadSummary `json:"checkpoint_summary"`
}

type taskMetrics struct {
	PrimaryFileHit        bool   `json:"primary_file_hit"`
	CriticalPathRecall    string `json:"critical_path_recall"`
	TestCompanionRecall   string `json:"test_companion_recall"`
	NoiseCount            int    `json:"noise_count"`
	RelatedCommitSurfaced bool   `json:"related_commit_surfaced"`
	ContextUsefulToStart  string `json:"context_useful_to_start"`
}

type taskObservedPaths struct {
	FilesRead    []string `json:"files_read,omitempty"`
	FilesEdited  []string `json:"files_edited,omitempty"`
	TestsRead    []string `json:"tests_read,omitempty"`
	TestsRun     []string `json:"tests_run,omitempty"`
	MissedFiles  []string `json:"missed_files,omitempty"`
	NoiseFiles   []string `json:"noise_files,omitempty"`
	GitDiffFiles []string `json:"git_diff_files,omitempty"`
	TestCommands []string `json:"test_commands,omitempty"`
}

type taskCheckpointReadSummary struct {
	JSONRecords              int      `json:"json_records"`
	MarkdownFallbacks        int      `json:"markdown_fallbacks"`
	EvidenceOnlyGitDiffFiles []string `json:"evidence_only_git_diff_files,omitempty"`
	EvidenceOnlyTestCommands []string `json:"evidence_only_test_commands,omitempty"`
	Notes                    []string `json:"notes,omitempty"`
}

func newTaskEvaluateCmd() *cobra.Command {
	var opts taskEvaluateOptions
	opts.Dir = defaultTaskWorkspaceDir
	cmd := &cobra.Command{
		Use:   "evaluate <task-id>",
		Short: "Compare predicted task context with checkpointed actual context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskEvaluate(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func runTaskEvaluate(cmd *cobra.Command, taskID string, opts taskEvaluateOptions) error {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	_, workspace, manifest, err := loadTaskWorkspaceManifest(opts.Dir, taskID)
	if err != nil {
		return err
	}
	observed, checkpointSummary, err := readTaskObservedPathsDetailed(workspace)
	if err != nil {
		return err
	}
	evaluation := evaluateTaskContext(manifest, observed)
	evaluation.CheckpointSummary = checkpointSummary
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(evaluation)
	}
	return writeTaskEvaluationHuman(cmd.OutOrStdout(), evaluation)
}

func readTaskObservedPathsDetailed(workspace string) (taskObservedPaths, taskCheckpointReadSummary, error) {
	var observed taskObservedPaths
	var summary taskCheckpointReadSummary
	checkpointDir := filepath.Join(workspace, "checkpoints")
	entries, err := os.ReadDir(checkpointDir)
	if os.IsNotExist(err) {
		return observed, summary, nil
	}
	if err != nil {
		return observed, summary, fmt.Errorf("read checkpoints: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	jsonStems := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			jsonStems[checkpointEntryStem(entry.Name())] = true
		}
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		lowerName := strings.ToLower(entry.Name())
		path := filepath.Join(checkpointDir, entry.Name())
		if strings.HasSuffix(lowerName, ".json") {
			record, err := readTaskCheckpointRecord(path)
			if err != nil {
				return observed, summary, err
			}
			summary.JSONRecords++
			appendCheckpointReadSummary(&summary, record)
			appendObservedFromCheckpointRecord(&observed, record)
			continue
		}
		if strings.HasSuffix(lowerName, ".md") {
			if jsonStems[checkpointEntryStem(entry.Name())] {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return observed, summary, err
			}
			summary.MarkdownFallbacks++
			appendObservedFromCheckpointMarkdown(&observed, string(data))
		}
	}
	finalizeTaskCheckpointReadSummary(&summary)
	return observed, summary, nil
}

func readTaskCheckpointRecord(path string) (taskCheckpointRecord, error) {
	var record taskCheckpointRecord
	data, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, fmt.Errorf("parse checkpoint JSON %s: %w", path, err)
	}
	normalizeTaskCheckpointRecord(&record)
	return record, nil
}

func appendObservedFromCheckpointRecord(observed *taskObservedPaths, record taskCheckpointRecord) {
	observed.FilesRead = appendNormalizedUnique(observed.FilesRead, record.FilesRead...)
	observed.FilesEdited = appendNormalizedUnique(observed.FilesEdited, record.FilesEdited...)
	observed.TestsRead = appendNormalizedUnique(observed.TestsRead, record.TestsRead...)
	observed.TestsRun = appendUniqueValues(observed.TestsRun, record.TestsRun...)
	observed.MissedFiles = appendNormalizedUnique(observed.MissedFiles, record.MissedFiles...)
	observed.NoiseFiles = appendNormalizedUnique(observed.NoiseFiles, record.NoiseFiles...)
	if record.Evidence.GitDiff != nil {
		observed.GitDiffFiles = appendNormalizedUnique(observed.GitDiffFiles, record.Evidence.GitDiff.ChangedFiles...)
	}
	observed.GitDiffFiles = appendNormalizedUnique(observed.GitDiffFiles, record.Evidence.GitDiffPaths...)
	for _, command := range record.Evidence.TestCommands {
		observed.TestCommands = appendUniqueString(observed.TestCommands, command.Command)
	}
}

func appendCheckpointReadSummary(summary *taskCheckpointReadSummary, record taskCheckpointRecord) {
	if record.Evidence.GitDiff != nil {
		summary.EvidenceOnlyGitDiffFiles = appendNormalizedUnique(summary.EvidenceOnlyGitDiffFiles, record.Evidence.GitDiff.ChangedFiles...)
	}
	summary.EvidenceOnlyGitDiffFiles = appendNormalizedUnique(summary.EvidenceOnlyGitDiffFiles, record.Evidence.GitDiffPaths...)
	for _, command := range record.Evidence.TestCommands {
		summary.EvidenceOnlyTestCommands = appendUniqueString(summary.EvidenceOnlyTestCommands, command.Command)
	}
}

func finalizeTaskCheckpointReadSummary(summary *taskCheckpointReadSummary) {
	if summary.JSONRecords > 0 {
		summary.Notes = appendUniqueString(summary.Notes, "Structured checkpoint JSON was preferred over markdown twins.")
	}
	if summary.MarkdownFallbacks > 0 {
		summary.Notes = appendUniqueString(summary.Notes, "Markdown checkpoint fallback was used for legacy records without JSON twins.")
	}
	if len(summary.EvidenceOnlyGitDiffFiles) > 0 || len(summary.EvidenceOnlyTestCommands) > 0 {
		summary.Notes = appendUniqueString(summary.Notes, "Git diff and test command receipts were kept as evidence-only substrate.")
	}
}

func appendObservedFromCheckpointMarkdown(observed *taskObservedPaths, body string) {
	sections := markdownSections(body)
	observed.FilesRead = appendNormalizedUnique(observed.FilesRead, pathsFromSection(sections["files actually read"])...)
	observed.FilesEdited = appendNormalizedUnique(observed.FilesEdited, pathsFromSection(sections["files actually edited"])...)
	observed.TestsRead = appendNormalizedUnique(observed.TestsRead, pathsFromSection(sections["tests actually read"])...)
	observed.TestsRun = appendUniqueValues(observed.TestsRun, pathsFromSection(sections["tests actually run"])...)
	observed.MissedFiles = appendNormalizedUnique(observed.MissedFiles, pathsFromSection(sections["critical files devspecs missed"])...)
	observed.NoiseFiles = appendNormalizedUnique(observed.NoiseFiles, pathsFromSection(sections["distracting files devspecs included"])...)
}

func checkpointEntryStem(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func evaluateTaskContext(manifest taskManifest, observed taskObservedPaths) taskEvaluationOutput {
	predictedPrimary := predictedFilePaths(manifest.Predicted.PrimaryFiles)
	predictedTests := predictedFilePaths(manifest.Predicted.Tests)
	predictedDocs := predictedFilePaths(manifest.Predicted.DocsPlansConfig)
	predictedSupporting := predictedFilePaths(manifest.Predicted.SupportingContext)
	predictedAll := appendNormalizedUnique(nil, predictedPrimary...)
	predictedAll = appendNormalizedUnique(predictedAll, predictedTests...)
	predictedAll = appendNormalizedUnique(predictedAll, predictedDocs...)
	predictedAll = appendNormalizedUnique(predictedAll, predictedSupporting...)

	actualFiles := appendNormalizedUnique(nil, observed.FilesRead...)
	actualFiles = appendNormalizedUnique(actualFiles, observed.FilesEdited...)
	actualFiles = appendNormalizedUnique(actualFiles, observed.TestsRead...)
	metricActualFiles := taskMetricPaths(manifest, actualFiles)
	metricTestsRead := taskMetricPaths(manifest, observed.TestsRead)

	var hits []string
	var misses []string
	for _, path := range metricActualFiles {
		if containsPath(predictedAll, path) {
			hits = appendNormalizedUnique(hits, path)
		} else {
			misses = appendNormalizedUnique(misses, path)
		}
	}
	for _, path := range observed.MissedFiles {
		if taskPathExcludedFromMetrics(manifest, path) {
			continue
		}
		if !containsPath(predictedAll, path) {
			misses = appendNormalizedUnique(misses, path)
		}
	}

	noise := appendNormalizedUnique(nil, observed.NoiseFiles...)
	companionMisses := taskCompanionMisses(misses, predictedPrimary)
	receiptMisses := taskReceiptMisses(misses, manifest.Predicted.ReceiptMissingFiles)
	primaryFileHit := anyPathOverlap(predictedPrimary, metricActualFiles)
	testHits := intersectionPaths(predictedTests, metricTestsRead)
	testTotal := len(normalizePathList(metricTestsRead))
	criticalTotal := len(metricActualFiles)
	criticalHits := len(hits)
	confidenceMismatch := taskConfidenceMismatch(manifest, misses, companionMisses)
	usefulness := taskUsefulnessClass(primaryFileHit, misses, noise, confidenceMismatch)
	notes := taskEvaluationNotes(manifest, observed, misses, companionMisses, receiptMisses, confidenceMismatch)

	return taskEvaluationOutput{
		TaskID:             manifest.TaskID,
		Query:              manifest.Query,
		Hits:               hits,
		Misses:             misses,
		Noise:              noise,
		CompanionMisses:    companionMisses,
		ReceiptMisses:      receiptMisses,
		ConfidenceMismatch: confidenceMismatch,
		Metrics: taskMetrics{
			PrimaryFileHit:        primaryFileHit,
			CriticalPathRecall:    ratioString(criticalHits, criticalTotal),
			TestCompanionRecall:   ratioString(len(testHits), testTotal),
			NoiseCount:            len(noise),
			RelatedCommitSurfaced: len(manifest.Predicted.RelatedGitReceipts) > 0,
			ContextUsefulToStart:  taskContextUseful(primaryFileHit, misses, noise),
		},
		UsefulnessClass: usefulness,
		Notes:           notes,
		Observed:        observed,
	}
}

func taskConfidenceMismatch(manifest taskManifest, misses, companionMisses []string) bool {
	if len(misses) == 0 && len(companionMisses) == 0 {
		return false
	}
	if manifest.Confidence.PackCompleteness == "high" {
		return true
	}
	if manifest.Confidence.TestCoverageConfidence == "high" && len(companionMisses) > 0 {
		return true
	}
	return false
}

func taskUsefulnessClass(primaryHit bool, misses, noise []string, confidenceMismatch bool) string {
	if confidenceMismatch {
		return "D"
	}
	if primaryHit && len(misses) == 0 && len(noise) == 0 {
		return "A"
	}
	if primaryHit {
		return "B"
	}
	if len(misses) > 0 || len(noise) > 0 {
		return "C"
	}
	return "B"
}

func taskContextUseful(primaryHit bool, misses, noise []string) string {
	if primaryHit && len(misses) == 0 && len(noise) == 0 {
		return "yes"
	}
	if primaryHit {
		return "maybe"
	}
	return "unknown"
}

func taskEvaluationNotes(manifest taskManifest, observed taskObservedPaths, misses, companionMisses, receiptMisses []string, confidenceMismatch bool) []string {
	var notes []string
	if len(observed.FilesRead)+len(observed.FilesEdited)+len(observed.TestsRead)+len(observed.MissedFiles) == 0 {
		notes = append(notes, "No checkpointed actual file context found yet.")
	}
	if len(misses) > 0 {
		notes = append(notes, "Some actual context paths were not predicted by the initial task pack.")
	}
	if len(companionMisses) > 0 {
		notes = append(notes, "At least one missed file appears to be a test companion.")
	}
	if len(receiptMisses) > 0 {
		notes = append(notes, "At least one missed file was already visible as a receipt-touched related path.")
	}
	if confidenceMismatch {
		notes = append(notes, "Confidence mismatch: initial confidence was stronger than observed context completeness.")
	}
	if manifest.Confidence.PackCompleteness != "high" {
		notes = append(notes, "Initial pack completeness was not high, so misses should feed the next retrieval/template iteration.")
	}
	return uniqueStrings(notes)
}

func taskMetricPaths(manifest taskManifest, paths []string) []string {
	var out []string
	for _, path := range paths {
		if taskPathExcludedFromMetrics(manifest, path) {
			continue
		}
		out = appendNormalizedUnique(out, path)
	}
	return out
}

func taskPathExcludedFromMetrics(manifest taskManifest, path string) bool {
	path = strings.ToLower(taskMetricComparablePath(path))
	if path == "" {
		return false
	}
	for _, prefix := range taskMetricWorkspacePrefixes(manifest) {
		prefix = strings.ToLower(strings.Trim(taskMetricComparablePath(prefix), "/"))
		if prefix == "" {
			continue
		}
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	for _, artifact := range taskMetricArtifactPaths(manifest) {
		artifact = strings.ToLower(taskMetricComparablePath(artifact))
		if artifact != "" && path == artifact {
			return true
		}
	}
	return strings.HasPrefix(path, "checkpoints/")
}

func taskMetricWorkspacePrefixes(manifest taskManifest) []string {
	var out []string
	workspace := taskMetricComparablePath(manifest.Workspace)
	repoRoot := taskMetricComparablePath(manifest.RepoRoot)
	if workspace != "" {
		out = appendUniqueString(out, workspace)
	}
	if workspace != "" && repoRoot != "" && strings.HasPrefix(strings.ToLower(workspace), strings.ToLower(strings.TrimRight(repoRoot, "/"))+"/") {
		rel := strings.TrimPrefix(workspace, strings.TrimRight(repoRoot, "/")+"/")
		out = appendUniqueString(out, rel)
	}
	return out
}

func taskMetricArtifactPaths(manifest taskManifest) []string {
	var out []string
	out = appendTaskMetricArtifactPath(out, taskManifestFilename)
	out = appendTaskMetricArtifactPath(out, manifest.Artifacts.Index)
	out = appendTaskMetricArtifactPath(out, manifest.Artifacts.FirstSlice)
	out = appendTaskMetricArtifactPath(out, manifest.Artifacts.Result)
	for _, slice := range manifest.Artifacts.Slices {
		out = appendTaskMetricArtifactPath(out, slice.Plan)
		out = appendTaskMetricArtifactPath(out, slice.Result)
	}
	return out
}

func appendTaskMetricArtifactPath(out []string, path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return out
	}
	out = appendUniqueString(out, path)
	out = appendUniqueString(out, "../"+path)
	return out
}

func taskMetricComparablePath(path string) string {
	path = normalizeSinglePath(path)
	path = strings.Trim(path, "/")
	return path
}

func taskCompanionMisses(misses, predictedPrimary []string) []string {
	var out []string
	for _, missed := range misses {
		if !looksLikeTestPath(missed) {
			continue
		}
		if len(predictedPrimary) == 0 || hasLikelySourceCompanion(missed, predictedPrimary) {
			out = appendNormalizedUnique(out, missed)
		}
	}
	return out
}

func taskReceiptMisses(misses, receiptMissing []string) []string {
	var out []string
	for _, missed := range misses {
		if containsPath(receiptMissing, missed) {
			out = appendNormalizedUnique(out, missed)
		}
	}
	return out
}

func hasLikelySourceCompanion(testPath string, sources []string) bool {
	testStem := testCompanionStem(testPath)
	if testStem == "" {
		return false
	}
	for _, source := range sources {
		sourceStem := strings.TrimSuffix(strings.ToLower(filepath.Base(filepath.ToSlash(source))), strings.ToLower(filepath.Ext(source)))
		if sourceStem == testStem {
			return true
		}
	}
	return false
}

func testCompanionStem(path string) string {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(path)))
	ext := strings.ToLower(filepath.Ext(base))
	stem := strings.TrimSuffix(base, ext)
	for _, suffix := range []string{"_test", ".test", ".spec", "-test", "-spec"} {
		if strings.HasSuffix(stem, suffix) {
			return strings.TrimSuffix(stem, suffix)
		}
	}
	if strings.HasPrefix(stem, "test_") {
		return strings.TrimPrefix(stem, "test_")
	}
	return stem
}

func looksLikeTestPath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(lower)
	return strings.Contains(lower, "/test/") ||
		strings.Contains(lower, "/tests/") ||
		strings.Contains(lower, "/__tests__/") ||
		strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasPrefix(base, "test_")
}

func markdownSections(body string) map[string][]string {
	sections := map[string][]string{}
	current := ""
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, "## ") {
			current = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			continue
		}
		if current != "" {
			sections[current] = append(sections[current], line)
		}
	}
	return sections
}

func pathsFromSection(lines []string) []string {
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "-" {
			continue
		}
		for {
			start := strings.Index(line, "`")
			if start < 0 {
				break
			}
			rest := line[start+1:]
			end := strings.Index(rest, "`")
			if end < 0 {
				break
			}
			out = appendNormalizedUnique(out, rest[:end])
			line = rest[end+1:]
		}
		if strings.HasPrefix(line, "- ") && !strings.Contains(line, "`") {
			out = appendNormalizedUnique(out, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
		}
	}
	return out
}

func appendNormalizedUnique(values []string, additions ...string) []string {
	for _, value := range normalizePathList(additions) {
		values = appendUniqueString(values, value)
	}
	return values
}

func appendUniqueValues(values []string, additions ...string) []string {
	for _, value := range normalizeList(additions) {
		values = appendUniqueString(values, value)
	}
	return values
}

func normalizePathList(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || value == "-" {
			continue
		}
		value = filepath.ToSlash(value)
		value = strings.TrimPrefix(value, "./")
		out = appendUniqueString(out, value)
	}
	return out
}

func containsPath(values []string, path string) bool {
	path = normalizeSinglePath(path)
	for _, value := range values {
		if normalizeSinglePath(value) == path {
			return true
		}
	}
	return false
}

func anyPathOverlap(left, right []string) bool {
	for _, value := range left {
		if containsPath(right, value) {
			return true
		}
	}
	return false
}

func intersectionPaths(left, right []string) []string {
	var out []string
	for _, value := range left {
		if containsPath(right, value) {
			out = appendNormalizedUnique(out, value)
		}
	}
	return out
}

func normalizeSinglePath(path string) string {
	path = strings.TrimSpace(path)
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "./")
	if base, _, ok := strings.Cut(path, "#"); ok {
		path = base
	}
	return path
}

func ratioString(numerator, denominator int) string {
	if denominator <= 0 {
		return "0/0"
	}
	return fmt.Sprintf("%d/%d", numerator, denominator)
}

func writeTaskEvaluationHuman(out interface{ Write([]byte) (int, error) }, evaluation taskEvaluationOutput) error {
	fmt.Fprintf(out, "Task evaluation: %s\n", evaluation.TaskID)
	fmt.Fprintf(out, "Usefulness class: %s\n", evaluation.UsefulnessClass)
	fmt.Fprintf(out, "Primary file hit: %t\n", evaluation.Metrics.PrimaryFileHit)
	fmt.Fprintf(out, "Critical-path recall: %s\n", evaluation.Metrics.CriticalPathRecall)
	fmt.Fprintf(out, "Test companion recall: %s\n", evaluation.Metrics.TestCompanionRecall)
	fmt.Fprintf(out, "Noise count: %d\n", evaluation.Metrics.NoiseCount)
	fmt.Fprintf(out, "Related commit surfaced: %t\n", evaluation.Metrics.RelatedCommitSurfaced)
	if evaluation.CheckpointSummary.JSONRecords > 0 || evaluation.CheckpointSummary.MarkdownFallbacks > 0 {
		fmt.Fprintf(out, "Checkpoint records: json=%d markdown_fallback=%d\n",
			evaluation.CheckpointSummary.JSONRecords,
			evaluation.CheckpointSummary.MarkdownFallbacks,
		)
	}
	if len(evaluation.Hits) > 0 {
		fmt.Fprintln(out, "\nHits")
		for _, path := range evaluation.Hits {
			fmt.Fprintf(out, "- %s\n", path)
		}
	}
	if len(evaluation.Misses) > 0 {
		fmt.Fprintln(out, "\nMisses")
		for _, path := range evaluation.Misses {
			fmt.Fprintf(out, "- %s\n", path)
		}
	}
	if len(evaluation.Noise) > 0 {
		fmt.Fprintln(out, "\nNoise")
		for _, path := range evaluation.Noise {
			fmt.Fprintf(out, "- %s\n", path)
		}
	}
	if len(evaluation.Notes) > 0 {
		fmt.Fprintln(out, "\nNotes")
		for _, note := range evaluation.Notes {
			fmt.Fprintf(out, "- %s\n", note)
		}
	}
	return nil
}
