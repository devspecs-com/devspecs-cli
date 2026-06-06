package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

const (
	defaultTaskWorkspaceDir = ".devspecs/tasks"
	taskManifestFilename    = "task.json"
)

var taskLifecycleStages = []string{
	"packed",
	"planned",
	"started",
	"implemented",
	"done",
	"blocked",
	"completed",
	"split",
	"superseded",
}

var taskDecisions = []string{
	"promote",
	"improve",
	"rework",
	"rollback",
	"blocked",
	"completed",
	"split",
	"superseded",
	"continue",
}

type taskStartOptions struct {
	ID        string
	Dir       string
	Slices    []string
	NoRefresh bool
	AsJSON    bool
	Force     bool
	Index     bool
}

type taskCheckpointOptions struct {
	Dir         string
	Slice       string
	Stage       string
	Decision    string
	Note        string
	Description string
	Goal        string
	Resources   []string
	FilesRead   []string
	FilesEdited []string
	TestsRead   []string
	TestsRun    []string
	MissedFiles []string
	NoiseFiles  []string
	Tasks       []string
	GitDiff     bool
	GitDiffMax  int
	TestOutput  bool
	TestMax     int
	Index       bool
	AsJSON      bool
}

type taskStartOutput struct {
	TaskID            string                 `json:"task_id"`
	Query             string                 `json:"query"`
	Workspace         string                 `json:"workspace"`
	IndexPath         string                 `json:"index_path"`
	FirstSlicePath    string                 `json:"first_slice_path"`
	ResultPath        string                 `json:"result_path"`
	Slices            []taskStartSliceOutput `json:"slices,omitempty"`
	ManifestPath      string                 `json:"manifest_path"`
	IndexedPaths      []string               `json:"indexed_paths,omitempty"`
	PrimaryFiles      []string               `json:"primary_files,omitempty"`
	TestFiles         []string               `json:"test_files,omitempty"`
	DocsConfigFiles   []string               `json:"docs_config_files,omitempty"`
	NoiseRiskFiles    []string               `json:"noise_risk_files,omitempty"`
	FreshnessWarnings []taskFreshnessWarning `json:"freshness_warnings,omitempty"`
	PackCompleteness  string                 `json:"pack_completeness"`
}

type taskStartSliceOutput struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	PlanPath   string `json:"plan_path"`
	ResultPath string `json:"result_path"`
}

type taskCheckpointOutput struct {
	TaskID             string   `json:"task_id"`
	Slice              string   `json:"slice,omitempty"`
	Stage              string   `json:"stage"`
	Decision           string   `json:"decision,omitempty"`
	CheckpointPath     string   `json:"checkpoint_path"`
	CheckpointJSONPath string   `json:"checkpoint_json_path"`
	ResultPath         string   `json:"result_path"`
	IndexedPaths       []string `json:"indexed_paths,omitempty"`
	GitDiffFiles       []string `json:"git_diff_files,omitempty"`
	TestEvidenceCount  int      `json:"test_evidence_count,omitempty"`
}

type taskManifest struct {
	TaskID            string                 `json:"task_id"`
	Query             string                 `json:"query"`
	Status            string                 `json:"status"`
	CreatedAt         string                 `json:"created_at"`
	RepoRoot          string                 `json:"repo_root"`
	Workspace         string                 `json:"workspace"`
	Artifacts         taskArtifactPaths      `json:"artifacts"`
	Predicted         taskPredictedContext   `json:"predicted_context"`
	FreshnessWarnings []taskFreshnessWarning `json:"freshness_warnings,omitempty"`
	Confidence        taskConfidence         `json:"confidence"`
}

type taskArtifactPaths struct {
	Index      string              `json:"index"`
	FirstSlice string              `json:"first_slice,omitempty"`
	Result     string              `json:"result,omitempty"`
	Slices     []taskSliceArtifact `json:"slices,omitempty"`
}

type taskSliceArtifact struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Plan   string `json:"plan"`
	Result string `json:"result"`
}

type taskPredictedContext struct {
	RelevantAreas       []string            `json:"relevant_areas,omitempty"`
	PrimaryFiles        []taskPredictedFile `json:"primary_files,omitempty"`
	Tests               []taskPredictedFile `json:"tests,omitempty"`
	DocsPlansConfig     []taskPredictedFile `json:"docs_plans_config,omitempty"`
	SupportingContext   []taskPredictedFile `json:"supporting_context,omitempty"`
	NoiseRisks          []taskPredictedFile `json:"noise_risks,omitempty"`
	RelatedGitReceipts  []taskGitReceipt    `json:"related_git_receipts,omitempty"`
	ReceiptMissingFiles []string            `json:"receipt_missing_files,omitempty"`
}

type taskPredictedFile struct {
	Path     string   `json:"path"`
	Title    string   `json:"title,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Subtype  string   `json:"subtype,omitempty"`
	Role     string   `json:"role,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

type taskGitReceipt struct {
	SHA          string   `json:"sha"`
	ShortSHA     string   `json:"short_sha,omitempty"`
	CommittedAt  string   `json:"committed_at,omitempty"`
	Subject      string   `json:"subject"`
	MatchedPaths []string `json:"matched_paths,omitempty"`
	RelatedPaths []string `json:"related_paths,omitempty"`
	MatchedTerms []string `json:"matched_terms,omitempty"`
	Signals      []string `json:"signals,omitempty"`
}

type taskFreshnessWarning struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type taskConfidence struct {
	PrimaryFileConfidence  string   `json:"primary_file_confidence"`
	TestCoverageConfidence string   `json:"test_coverage_confidence"`
	DocsConfigConfidence   string   `json:"docs_config_confidence"`
	GitReceiptConfidence   string   `json:"git_receipt_confidence"`
	NoiseRisk              string   `json:"noise_risk"`
	PackCompleteness       string   `json:"pack_completeness"`
	Why                    []string `json:"why,omitempty"`
	AgentInstruction       string   `json:"agent_instruction"`
}

type taskCheckpointRecord struct {
	SchemaVersion int                    `json:"schema_version"`
	TaskID        string                 `json:"task_id"`
	Query         string                 `json:"query,omitempty"`
	Slice         string                 `json:"slice,omitempty"`
	SliceTitle    string                 `json:"slice_title,omitempty"`
	Stage         string                 `json:"stage"`
	Decision      string                 `json:"decision,omitempty"`
	CreatedAt     string                 `json:"created_at"`
	Goal          string                 `json:"goal,omitempty"`
	Description   string                 `json:"description,omitempty"`
	Note          string                 `json:"note,omitempty"`
	Resources     []string               `json:"resources,omitempty"`
	FilesRead     []string               `json:"files_read,omitempty"`
	FilesEdited   []string               `json:"files_edited,omitempty"`
	TestsRead     []string               `json:"tests_read,omitempty"`
	TestsRun      []string               `json:"tests_run,omitempty"`
	MissedFiles   []string               `json:"missed_files,omitempty"`
	NoiseFiles    []string               `json:"noise_files,omitempty"`
	Tasks         []string               `json:"tasks,omitempty"`
	Evidence      taskCheckpointEvidence `json:"evidence,omitempty"`
}

type taskCheckpointEvidence struct {
	GitDiff      *taskGitDiffEvidence     `json:"git_diff,omitempty"`
	TestCommands []taskCommandRunEvidence `json:"test_commands,omitempty"`
}

type taskGitDiffEvidence struct {
	Command      string   `json:"command"`
	Stat         string   `json:"stat,omitempty"`
	ChangedFiles []string `json:"changed_files,omitempty"`
	MaxBytes     int      `json:"max_bytes"`
	Truncated    bool     `json:"truncated"`
	Error        string   `json:"error,omitempty"`
}

type taskCommandRunEvidence struct {
	Command   string `json:"command"`
	ExitCode  int    `json:"exit_code"`
	Output    string `json:"output,omitempty"`
	MaxBytes  int    `json:"max_bytes"`
	Truncated bool   `json:"truncated"`
	TimedOut  bool   `json:"timed_out,omitempty"`
	Error     string `json:"error,omitempty"`
}

// NewTaskCmd creates the ds task command group.
func NewTaskCmd() *cobra.Command {
	var opts taskStartOptions
	opts.Dir = defaultTaskWorkspaceDir
	opts.Index = true

	cmd := &cobra.Command{
		Use:   "task <query>",
		Short: "Create a grounded task workspace",
		Long: `Create a local task workspace from a repo-grounded query.

The workspace is intentionally uncertainty-aware: it separates evidence from
inference, creates named task-slice plan/result artifacts, and leaves checkpoint
templates for recording actual reads, edits, tests, misses, and noise.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskStart(cmd, args[0], opts)
		},
	}

	cmd.Flags().StringVar(&opts.ID, "id", "", "Task ID to use instead of a generated slug")
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().StringArrayVar(&opts.Slices, "slice", nil, "Task slice title to scaffold; may be repeated")
	cmd.Flags().BoolVar(&opts.NoRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Overwrite an existing task workspace")
	cmd.Flags().BoolVar(&opts.Index, "index", true, "Capture A00 and slice plans into the DevSpecs index")

	cmd.AddCommand(newTaskCheckpointCmd())
	cmd.AddCommand(newTaskEvaluateCmd())
	return cmd
}

func newTaskCheckpointCmd() *cobra.Command {
	var opts taskCheckpointOptions
	opts.Dir = defaultTaskWorkspaceDir
	opts.Stage = "implemented"
	opts.Decision = "continue"
	opts.GitDiffMax = 12000
	opts.TestMax = 12000
	opts.Index = true

	cmd := &cobra.Command{
		Use:   "checkpoint <task-id>",
		Short: "Record a task checkpoint and update the task result",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskCheckpoint(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().StringVar(&opts.Slice, "slice", "", "Task slice ID/title/plan/result to append to; defaults to the first slice")
	cmd.Flags().StringVar(&opts.Stage, "stage", opts.Stage, "Lifecycle stage: packed, planned, started, implemented, done, blocked, completed, split, superseded")
	cmd.Flags().StringVar(&opts.Decision, "decision", opts.Decision, "Decision gate: promote, improve, rework, rollback, blocked, completed, split, superseded, continue")
	cmd.Flags().StringVar(&opts.Note, "note", "", "Short checkpoint note")
	cmd.Flags().StringVar(&opts.Description, "description", "", "What changed or was learned")
	cmd.Flags().StringVar(&opts.Goal, "goal", "", "Checkpoint goal")
	cmd.Flags().StringArrayVar(&opts.Resources, "resource", nil, "Resource link/path to record; may be repeated")
	cmd.Flags().StringArrayVar(&opts.FilesRead, "file-read", nil, "File actually read; may be repeated")
	cmd.Flags().StringArrayVar(&opts.FilesEdited, "file-edited", nil, "File actually edited; may be repeated")
	cmd.Flags().StringArrayVar(&opts.TestsRead, "test-read", nil, "Test file actually read; may be repeated")
	cmd.Flags().StringArrayVar(&opts.TestsRun, "test-run", nil, "Test command actually run; may be repeated")
	cmd.Flags().StringArrayVar(&opts.MissedFiles, "missed-file", nil, "Critical file DevSpecs missed; may be repeated")
	cmd.Flags().StringArrayVar(&opts.NoiseFiles, "noise-file", nil, "Distracting file DevSpecs included; may be repeated")
	cmd.Flags().StringArrayVar(&opts.Tasks, "task", nil, "Task/checklist item update; may be repeated")
	cmd.Flags().BoolVar(&opts.GitDiff, "git-diff", false, "Include bounded git diff stat and changed-file evidence in checkpoint JSON")
	cmd.Flags().IntVar(&opts.GitDiffMax, "git-diff-max-bytes", opts.GitDiffMax, "Maximum bytes of git diff stat evidence to keep")
	cmd.Flags().BoolVar(&opts.TestOutput, "capture-test-output", false, "Run --test-run commands and include bounded output evidence in checkpoint JSON")
	cmd.Flags().IntVar(&opts.TestMax, "test-output-max-bytes", opts.TestMax, "Maximum bytes of captured test command output to keep")
	cmd.Flags().BoolVar(&opts.Index, "index", true, "Capture the checkpoint into the DevSpecs index")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func runTaskStart(cmd *cobra.Command, query string, opts taskStartOptions) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("task query is empty")
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	repoRoot := canonicalRepoRoot(resolveRepoRootFromWd(wd))
	if repoRoot == "" {
		repoRoot = canonicalRepoRoot(wd)
	}
	now := time.Now().UTC()
	taskID := strings.TrimSpace(opts.ID)
	if taskID == "" {
		taskID = generatedTaskID(query, now)
	}
	if err := validateTaskID(taskID); err != nil {
		return err
	}

	workspace := taskWorkspacePath(repoRoot, opts.Dir, taskID)
	if err := prepareTaskWorkspace(workspace, opts.Force); err != nil {
		return err
	}

	preflight, err := buildTaskPreflight(cmd, repoRoot, query, opts.NoRefresh)
	if err != nil {
		return err
	}
	slices := taskSliceArtifacts(query, opts.Slices)
	firstSlice := firstTaskSliceArtifact(taskArtifactPaths{Slices: slices})
	relArtifacts := taskArtifactPaths{
		Index:      "A00-index.md",
		FirstSlice: firstSlice.Plan,
		Result:     firstSlice.Result,
		Slices:     slices,
	}
	manifest := taskManifest{
		TaskID:            taskID,
		Query:             query,
		Status:            "packed",
		CreatedAt:         now.Format(time.RFC3339),
		RepoRoot:          repoRoot,
		Workspace:         filepath.ToSlash(workspace),
		Artifacts:         relArtifacts,
		Predicted:         preflight.Predicted,
		FreshnessWarnings: preflight.FreshnessWarnings,
		Confidence:        preflight.Confidence,
	}

	paths := taskAbsoluteArtifactPaths(workspace, relArtifacts)
	files := map[string]string{
		paths.Index: renderTaskIndex(manifest),
	}
	for _, slice := range slices {
		files[filepath.Join(workspace, slice.Plan)] = renderTaskSlicePlan(manifest, slice)
		files[filepath.Join(workspace, slice.Result)] = renderTaskSliceResultTemplate(manifest, slice)
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	manifestPath := filepath.Join(workspace, taskManifestFilename)
	if err := writeTaskManifest(manifestPath, manifest); err != nil {
		return err
	}

	var indexed []string
	if opts.Index {
		requests := []taskCaptureRequest{
			{Path: paths.Index, Title: "Task " + taskID + " preflight", Status: "implementing"},
		}
		for _, slice := range slices {
			requests = append(requests, taskCaptureRequest{
				Path:   filepath.Join(workspace, slice.Plan),
				Title:  "Task " + taskID + " " + slice.ID + " plan: " + slice.Title,
				Status: "implementing",
			})
		}
		indexed, err = captureTaskArtifacts(cmd, repoRoot, requests)
		if err != nil {
			return err
		}
	}

	out := taskStartOutput{
		TaskID:            taskID,
		Query:             query,
		Workspace:         workspace,
		IndexPath:         paths.Index,
		FirstSlicePath:    paths.FirstSlice,
		ResultPath:        paths.Result,
		Slices:            taskStartSliceOutputs(workspace, slices),
		ManifestPath:      manifestPath,
		IndexedPaths:      indexed,
		PrimaryFiles:      predictedFilePaths(preflight.Predicted.PrimaryFiles),
		TestFiles:         predictedFilePaths(preflight.Predicted.Tests),
		DocsConfigFiles:   predictedFilePaths(preflight.Predicted.DocsPlansConfig),
		NoiseRiskFiles:    predictedFilePaths(preflight.Predicted.NoiseRisks),
		FreshnessWarnings: preflight.FreshnessWarnings,
		PackCompleteness:  preflight.Confidence.PackCompleteness,
	}
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	return writeTaskStartHuman(cmd.OutOrStdout(), out, preflight.Confidence)
}

type taskAbsolutePaths struct {
	Index      string
	FirstSlice string
	Result     string
}

func taskAbsoluteArtifactPaths(workspace string, rel taskArtifactPaths) taskAbsolutePaths {
	return taskAbsolutePaths{
		Index:      filepath.Join(workspace, rel.Index),
		FirstSlice: filepath.Join(workspace, rel.FirstSlice),
		Result:     filepath.Join(workspace, rel.Result),
	}
}

func taskSliceArtifacts(query string, values []string) []taskSliceArtifact {
	titles := normalizeTaskSliceTitles(query, values)
	usedSlugs := map[string]int{}
	var out []taskSliceArtifact
	for i, title := range titles {
		id := fmt.Sprintf("A%02d", i+1)
		slug := uniqueTaskSliceSlug(title, usedSlugs)
		out = append(out, taskSliceArtifact{
			ID:     id,
			Title:  title,
			Plan:   fmt.Sprintf("%s-%s-plan.md", id, slug),
			Result: fmt.Sprintf("%s-%s-result.md", id, slug),
		})
	}
	return out
}

func normalizeTaskSliceTitles(query string, values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		out = append(out, strings.TrimSpace(query))
	}
	return out
}

func uniqueTaskSliceSlug(title string, used map[string]int) string {
	slug := sanitizeTaskFilename(title)
	seen := used[slug]
	used[slug] = seen + 1
	if seen == 0 {
		return slug
	}
	return fmt.Sprintf("%s-%d", slug, seen+1)
}

func firstTaskSliceArtifact(paths taskArtifactPaths) taskSliceArtifact {
	if len(paths.Slices) > 0 {
		return paths.Slices[0]
	}
	return taskSliceArtifact{
		ID:     "A01",
		Title:  "First slice",
		Plan:   paths.FirstSlice,
		Result: paths.Result,
	}
}

func taskStartSliceOutputs(workspace string, slices []taskSliceArtifact) []taskStartSliceOutput {
	var out []taskStartSliceOutput
	for _, slice := range slices {
		out = append(out, taskStartSliceOutput{
			ID:         slice.ID,
			Title:      slice.Title,
			PlanPath:   filepath.Join(workspace, slice.Plan),
			ResultPath: filepath.Join(workspace, slice.Result),
		})
	}
	return out
}

func taskCheckpointResourcePaths(manifest taskManifest, slice taskSliceArtifact) []string {
	if slice.ID == "" && slice.Plan == "" && slice.Result == "" {
		slice = firstTaskSliceArtifact(manifest.Artifacts)
	}
	resources := []string{"../A00-index.md"}
	if slice.Plan != "" {
		resources = append(resources, "../"+filepath.ToSlash(slice.Plan))
	}
	if slice.Result != "" {
		resources = append(resources, "../"+filepath.ToSlash(slice.Result))
	}
	resources = append(resources, "../"+taskManifestFilename)
	return resources
}

func taskSliceForCheckpoint(manifest taskManifest, selector string) (taskSliceArtifact, error) {
	selector = strings.TrimSpace(selector)
	first := firstTaskSliceArtifact(manifest.Artifacts)
	if selector == "" {
		return first, nil
	}
	matches := func(slice taskSliceArtifact) bool {
		for _, candidate := range []string{
			slice.ID,
			slice.Title,
			slice.Plan,
			slice.Result,
			sanitizeTaskFilename(slice.Title),
		} {
			if strings.EqualFold(strings.TrimSpace(candidate), selector) {
				return true
			}
		}
		return false
	}
	for _, slice := range manifest.Artifacts.Slices {
		if matches(slice) {
			return slice, nil
		}
	}
	if matches(first) {
		return first, nil
	}
	var available []string
	for _, slice := range manifest.Artifacts.Slices {
		label := slice.ID
		if slice.Title != "" {
			label += " (" + slice.Title + ")"
		}
		available = append(available, label)
	}
	if len(available) == 0 && first.ID != "" {
		available = append(available, first.ID)
	}
	if len(available) == 0 {
		return taskSliceArtifact{}, fmt.Errorf("task has no slice artifacts")
	}
	return taskSliceArtifact{}, fmt.Errorf("unknown task slice %q; available: %s", selector, strings.Join(available, ", "))
}

type taskPreflight struct {
	Predicted         taskPredictedContext
	FreshnessWarnings []taskFreshnessWarning
	Confidence        taskConfidence
}

func buildTaskPreflight(cmd *cobra.Command, repoRoot, query string, noRefresh bool) (taskPreflight, error) {
	db, err := openDB()
	if err != nil {
		return taskPreflight{}, err
	}
	defer db.Close()

	if !noRefresh {
		ensureRepoIndexed(cmd, db, repoRoot)
	}
	fp := store.FilterParams{RepoRoot: repoRoot}
	loadResult, err := loadRetrievalCandidatesForQueryWithReport(db, fp, query)
	if err != nil {
		return taskPreflight{}, fmt.Errorf("task preflight: %w", err)
	}
	candidates := loadResult.Candidates
	retriever := retrieval.WeightedFilesRetrieverV0{
		AnchorFirstRanking: true,
		AnchorFirstMode:    retrieval.DefaultAnchorFirstMode,
	}
	matches := retrieveFindMatches(retriever, candidates, query)
	packResult, err := buildFindPackAssemblyFromMatches(cmd.Context(), db, fp, query, matches, candidates, findPackAssemblyOptions{
		PackCompanions:       taskPackCompanionMode(),
		SourceTestReceipts:   findSourceTestReceiptsModeOff,
		GitReceipts:          true,
		PackPresentationMode: taskPackPresentationMode(),
	})
	if err != nil {
		return taskPreflight{}, fmt.Errorf("task preflight pack assembly: %w", err)
	}
	predicted := predictedContextFromPack(packResult.RolePack, packResult.GitTrust)
	predicted.RelevantAreas = relevantAreasFromPredicted(predicted)
	freshnessWarnings := taskFreshnessWarnings(repoRoot, query, candidates)
	confidence := confidenceForPredicted(predicted)
	return taskPreflight{Predicted: predicted, FreshnessWarnings: freshnessWarnings, Confidence: confidence}, nil
}

func taskPackCompanionMode() string {
	if value := strings.TrimSpace(os.Getenv("DEVSPECS_TASK_PACK_COMPANION_MODE")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("DEVSPECS_PACK_COMPANION_MODE")); value != "" {
		return value
	}
	return findPackCompanionModeAll
}

func taskPackPresentationMode() string {
	if value := strings.TrimSpace(os.Getenv("DEVSPECS_TASK_PACK_PRESENTATION_MODE")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("DEVSPECS_PACK_PRESENTATION_MODE")); value != "" {
		return value
	}
	return findPackPresentationModeOff
}

const (
	taskFreshnessMaxScannedFiles = 5000
	taskFreshnessMaxWarnings     = 8
	taskFreshnessMaxFileBytes    = 512 * 1024
)

type taskFreshnessCandidate struct {
	Path   string
	Reason string
	Score  int
}

func taskFreshnessWarnings(repoRoot, query string, indexedCandidates []retrieval.Candidate) []taskFreshnessWarning {
	queryTerms := taskFreshnessQueryTerms(query)
	if len(queryTerms) == 0 {
		return nil
	}
	indexed := taskIndexedCandidatePaths(indexedCandidates)
	var candidates []taskFreshnessCandidate
	scanned := 0
	_ = filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := taskRelativePath(repoRoot, path)
		if rel == "." {
			return nil
		}
		if entry.IsDir() {
			if taskFreshnessSkipDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if scanned >= taskFreshnessMaxScannedFiles {
			return filepath.SkipAll
		}
		scanned++
		if taskFreshnessSkipFile(rel, entry) {
			return nil
		}
		if indexed[strings.ToLower(rel)] {
			return nil
		}
		matches := taskFreshnessPathTermMatches(rel, queryTerms)
		if len(matches) < 2 && !taskFreshnessLooksLikeCompanion(rel, queryTerms) {
			return nil
		}
		reason := taskFreshnessReason(matches)
		score := len(matches)
		if taskFreshnessLooksLikeCompanion(rel, queryTerms) {
			score += 2
			if len(matches) == 0 {
				reason = "same-stem or same-package test-like path matched task terms"
			}
		}
		candidates = append(candidates, taskFreshnessCandidate{
			Path:   rel,
			Reason: reason,
			Score:  score,
		})
		return nil
	})
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].Score > candidates[j].Score
	})
	var out []taskFreshnessWarning
	for _, candidate := range candidates {
		out = append(out, taskFreshnessWarning{
			Path:   candidate.Path,
			Reason: candidate.Reason,
		})
		if len(out) >= taskFreshnessMaxWarnings {
			break
		}
	}
	return out
}

func taskIndexedCandidatePaths(candidates []retrieval.Candidate) map[string]bool {
	out := map[string]bool{}
	for _, candidate := range candidates {
		for _, path := range []string{candidate.Path, candidate.Source} {
			path = strings.ToLower(strings.Trim(filepath.ToSlash(path), "/"))
			if path != "" {
				out[path] = true
				if base, _, ok := strings.Cut(path, "#"); ok && base != "" {
					out[base] = true
				}
			}
		}
	}
	return out
}

func taskFreshnessQueryTerms(query string) []string {
	stop := map[string]bool{
		"add": true, "and": true, "are": true, "but": true, "can": true, "cli": true, "fix": true,
		"for": true, "from": true, "get": true, "how": true, "make": true, "not": true, "our": true,
		"run": true, "the": true, "this": true, "use": true, "with": true,
	}
	var terms []string
	for _, raw := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		raw = strings.TrimSpace(raw)
		if len(raw) < 3 || stop[raw] {
			continue
		}
		terms = appendUniqueString(terms, raw)
	}
	return terms
}

func taskFreshnessSkipDir(rel string) bool {
	rel = strings.ToLower(strings.Trim(filepath.ToSlash(rel), "/"))
	if rel == "" {
		return false
	}
	if rel == ".git" || rel == "_ignore" || rel == "fixtures" || rel == "testdata" ||
		rel == "vendor" || rel == "node_modules" || rel == "dist" || rel == "build" || rel == "coverage" {
		return true
	}
	return rel == ".devspecs/tasks" || strings.HasPrefix(rel, ".devspecs/tasks/")
}

func taskFreshnessSkipFile(rel string, entry os.DirEntry) bool {
	rel = strings.ToLower(strings.Trim(filepath.ToSlash(rel), "/"))
	if rel == "" || taskFreshnessPathExcluded(rel) {
		return true
	}
	ext := filepath.Ext(rel)
	if !taskFreshnessAllowedExt(ext) {
		return true
	}
	if strings.Contains(rel, ".generated.") || strings.Contains(rel, "/generated/") ||
		strings.HasSuffix(rel, ".gen"+ext) || strings.HasSuffix(rel, ".pb.go") {
		return true
	}
	info, err := entry.Info()
	if err != nil {
		return true
	}
	return info.Size() > taskFreshnessMaxFileBytes
}

func taskFreshnessPathExcluded(rel string) bool {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	return strings.HasPrefix(rel, ".devspecs/tasks/") ||
		strings.HasPrefix(rel, "_ignore/") ||
		strings.HasPrefix(rel, "fixtures/") ||
		strings.HasPrefix(rel, "testdata/") ||
		strings.Contains(rel, "/fixtures/") ||
		strings.Contains(rel, "/testdata/")
}

func taskFreshnessAllowedExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java", ".kt", ".md", ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}

func taskFreshnessPathTermMatches(rel string, queryTerms []string) []string {
	path := strings.ToLower(filepath.ToSlash(rel))
	var matches []string
	for _, term := range queryTerms {
		if strings.Contains(path, term) {
			matches = append(matches, term)
		}
	}
	return matches
}

func taskFreshnessLooksLikeCompanion(rel string, queryTerms []string) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	if !(strings.Contains(rel, "_test.") || strings.Contains(rel, ".test.") || strings.Contains(rel, ".spec.")) {
		return false
	}
	return len(taskFreshnessPathTermMatches(rel, queryTerms)) > 0
}

func taskFreshnessReason(matches []string) string {
	if len(matches) == 0 {
		return "on-disk path looked task-related but was not in the indexed candidate set"
	}
	if len(matches) > 4 {
		matches = matches[:4]
	}
	return "on-disk path matched task terms but was not in the indexed candidate set: " + strings.Join(matches, ", ")
}

func predictedContextFromPack(rolePack retrieval.RoleGroupedPack, gitTrust *FindGitTrustContext) taskPredictedContext {
	var predicted taskPredictedContext
	for _, group := range rolePack.Groups {
		for _, item := range group.Items {
			file := predictedFileFromPackItem(item, group.Role)
			switch group.Role {
			case retrieval.PackRoleImplementation:
				predicted.PrimaryFiles = append(predicted.PrimaryFiles, file)
			case retrieval.PackRoleBehaviorTests:
				predicted.Tests = append(predicted.Tests, file)
			case retrieval.PackRoleBackgroundDecisions, retrieval.PackRoleConfigSchema, retrieval.PackRoleOpenWork:
				predicted.DocsPlansConfig = append(predicted.DocsPlansConfig, file)
			default:
				predicted.SupportingContext = append(predicted.SupportingContext, file)
			}
		}
	}
	for _, item := range rolePack.ExcludedNoise {
		predicted.NoiseRisks = append(predicted.NoiseRisks, predictedFileFromPackItem(item, retrieval.PackRoleExcludedNoise))
	}
	if gitTrust != nil {
		for _, receipt := range gitTrust.Receipts {
			predicted.RelatedGitReceipts = append(predicted.RelatedGitReceipts, taskGitReceipt{
				SHA:          receipt.SHA,
				ShortSHA:     receipt.ShortSHA,
				CommittedAt:  receipt.CommittedAt,
				Subject:      receipt.Subject,
				MatchedPaths: receipt.MatchedPaths,
				RelatedPaths: receipt.RelatedPaths,
				MatchedTerms: receipt.MatchedTerms,
				Signals:      receipt.Signals,
			})
			for _, path := range receipt.RelatedPaths {
				predicted.ReceiptMissingFiles = appendUniqueString(predicted.ReceiptMissingFiles, path)
			}
		}
	}
	return predicted
}

func predictedFileFromPackItem(item retrieval.PackItem, role string) taskPredictedFile {
	path := item.Path
	if path == "" {
		path = item.SourcePath
	}
	return taskPredictedFile{
		Path:     path,
		Title:    item.Title,
		Kind:     item.Kind,
		Subtype:  item.Subtype,
		Role:     role,
		Evidence: firstStrings(item.Reasons, 5),
	}
}

func relevantAreasFromPredicted(predicted taskPredictedContext) []string {
	counts := map[string]int{}
	add := func(files []taskPredictedFile) {
		for _, file := range files {
			area := pathArea(file.Path)
			if area != "" {
				counts[area]++
			}
		}
	}
	add(predicted.PrimaryFiles)
	add(predicted.Tests)
	add(predicted.DocsPlansConfig)
	add(predicted.SupportingContext)
	type areaCount struct {
		Area  string
		Count int
	}
	var ranked []areaCount
	for area, count := range counts {
		ranked = append(ranked, areaCount{Area: area, Count: count})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Count == ranked[j].Count {
			return ranked[i].Area < ranked[j].Area
		}
		return ranked[i].Count > ranked[j].Count
	})
	var out []string
	for _, entry := range ranked {
		out = append(out, entry.Area)
		if len(out) >= 6 {
			break
		}
	}
	return out
}

func pathArea(path string) string {
	path = strings.Trim(filepath.ToSlash(path), "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		return parts[0]
	}
	if parts[0] == "internal" || parts[0] == "cmd" || parts[0] == "pkg" || parts[0] == "apps" || parts[0] == "services" {
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
	}
	return parts[0]
}

func confidenceForPredicted(predicted taskPredictedContext) taskConfidence {
	primary := confidenceByCount(len(predicted.PrimaryFiles), 2, 1)
	tests := confidenceByCount(len(predicted.Tests), 2, 1)
	docs := confidenceByCount(len(predicted.DocsPlansConfig), 2, 1)
	git := confidenceByCount(len(predicted.RelatedGitReceipts), 2, 1)
	noise := "low"
	if len(predicted.NoiseRisks) >= 3 {
		noise = "high"
	} else if len(predicted.NoiseRisks) > 0 {
		noise = "medium"
	}

	completeness := "low"
	if primary == "high" && tests != "low" && noise != "high" {
		completeness = "medium"
	}
	if primary == "high" && tests == "high" && docs != "low" && git != "low" && noise == "low" {
		completeness = "high"
	}

	var why []string
	if len(predicted.PrimaryFiles) > 0 {
		why = append(why, fmt.Sprintf("found %d likely primary file(s)", len(predicted.PrimaryFiles)))
	} else {
		why = append(why, "no clear primary implementation file was found")
	}
	if len(predicted.Tests) > 0 {
		why = append(why, fmt.Sprintf("found %d likely test file(s)", len(predicted.Tests)))
	} else {
		why = append(why, "test companion coverage was not evident from the initial pack")
	}
	if len(predicted.RelatedGitReceipts) > 0 {
		why = append(why, fmt.Sprintf("found %d related Git receipt(s)", len(predicted.RelatedGitReceipts)))
	}
	if len(predicted.NoiseRisks) > 0 {
		why = append(why, fmt.Sprintf("%d file(s) were downgraded as likely noise", len(predicted.NoiseRisks)))
	}

	return taskConfidence{
		PrimaryFileConfidence:  primary,
		TestCoverageConfidence: tests,
		DocsConfigConfidence:   docs,
		GitReceiptConfidence:   git,
		NoiseRisk:              noise,
		PackCompleteness:       completeness,
		Why:                    why,
		AgentInstruction:       "Validate the test and integration surface before editing. Record critical misses and distracting inclusions in the slice result or a task checkpoint.",
	}
}

func confidenceByCount(count, highAt, mediumAt int) string {
	if count >= highAt {
		return "high"
	}
	if count >= mediumAt {
		return "medium"
	}
	return "low"
}

func renderTaskIndex(manifest taskManifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Task %s\n\n", manifest.TaskID)
	fmt.Fprintln(&b, "## Task")
	fmt.Fprintf(&b, "%s\n\n", manifest.Query)
	fmt.Fprintln(&b, "## Status")
	fmt.Fprintf(&b, "%s\n\n", manifest.Status)
	fmt.Fprintln(&b, "## Created At")
	fmt.Fprintf(&b, "%s\n\n", manifest.CreatedAt)
	fmt.Fprintln(&b, "## Original Query")
	fmt.Fprintf(&b, "%s\n\n", manifest.Query)
	fmt.Fprintln(&b, "## Repo / Workspace")
	fmt.Fprintf(&b, "- Repo: `%s`\n", manifest.RepoRoot)
	fmt.Fprintf(&b, "- Workspace: `%s`\n\n", manifest.Workspace)
	fmt.Fprintln(&b, "## Resources")
	fmt.Fprintf(&b, "- `%s`\n", taskManifestFilename)
	for _, slice := range manifest.Artifacts.Slices {
		fmt.Fprintf(&b, "- `%s`\n", slice.Plan)
		fmt.Fprintf(&b, "- `%s`\n", slice.Result)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Task Slices")
	if len(manifest.Artifacts.Slices) == 0 {
		fmt.Fprintln(&b, "No named slice artifacts were recorded in the manifest.")
	} else {
		for _, slice := range manifest.Artifacts.Slices {
			fmt.Fprintf(&b, "- %s: %s. Plan: `%s`. Result: `%s`.\n", slice.ID, slice.Title, slice.Plan, slice.Result)
		}
	}
	fmt.Fprintln(&b)
	writeStringList(&b, "Relevant Map Areas", manifest.Predicted.RelevantAreas, "No strong map area was inferred from the initial pack.")
	writePredictedFiles(&b, "Likely Primary Files", manifest.Predicted.PrimaryFiles)
	writePredictedFiles(&b, "Likely Tests", manifest.Predicted.Tests)
	writePredictedFiles(&b, "Likely Docs / Plans / Config", manifest.Predicted.DocsPlansConfig)
	writePredictedFiles(&b, "Supporting Context", manifest.Predicted.SupportingContext)
	writeGitReceipts(&b, manifest.Predicted)
	writePredictedFiles(&b, "Noise Risks", manifest.Predicted.NoiseRisks)
	writeFreshnessWarnings(&b, manifest.FreshnessWarnings)
	fmt.Fprintln(&b, "## Known Knowns")
	for _, known := range taskKnownKnowns(manifest) {
		fmt.Fprintf(&b, "- %s\n", known)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Known Unknowns")
	for _, unknown := range taskKnownUnknowns(manifest) {
		fmt.Fprintf(&b, "- %s\n", unknown)
	}
	fmt.Fprintln(&b)
	writeConfidenceSummary(&b, manifest.Confidence)
	first := firstTaskSliceArtifact(manifest.Artifacts)
	fmt.Fprintln(&b, "## Suggested Starting Slice")
	if first.Plan != "" {
		fmt.Fprintf(&b, "Use `%s` as the first bounded plan in this task thread. Refine it before editing if primary files, tests, or integration points look incomplete.\n", first.Plan)
	} else {
		fmt.Fprintln(&b, "Start by refining the first bounded plan in this task thread before editing.")
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Agent Preflight Checklist")
	fmt.Fprintln(&b, "- [ ] Verify the likely primary files against the repo before editing.")
	fmt.Fprintln(&b, "- [ ] Search for same-package or same-command tests if test confidence is not high.")
	fmt.Fprintln(&b, "- [ ] Check receipt-touched related files before assuming the pack is complete.")
	if first.Result != "" {
		fmt.Fprintf(&b, "- [ ] Record files actually read, edited, tests run, misses, and noise in `%s` or `ds task checkpoint`.\n", first.Result)
	} else {
		fmt.Fprintln(&b, "- [ ] Record files actually read, edited, tests run, misses, and noise in the slice result or `ds task checkpoint`.")
	}
	return b.String()
}

func renderTaskSlicePlan(manifest taskManifest, slice taskSliceArtifact) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Task %s %s Plan\n\n", manifest.TaskID, slice.ID)
	fmt.Fprintln(&b, "## Goal")
	fmt.Fprintf(&b, "%s\n\n", slice.Title)
	fmt.Fprintln(&b, "## Description")
	fmt.Fprintf(&b, "Create a bounded implementation slice for `%s`. This plan is grounded by the A00 preflight, but it is not authoritative; confirm predicted files and tests before making edits.\n", manifest.Query)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Resources")
	fmt.Fprintln(&b, "- `A00-index.md`")
	if slice.Result != "" {
		fmt.Fprintf(&b, "- `%s`\n", slice.Result)
	}
	fmt.Fprintln(&b, "- `task.json`")
	for _, file := range firstPredictedResources(manifest.Predicted) {
		fmt.Fprintf(&b, "- `%s`\n", file)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Starting Context")
	writeInlinePredictedFiles(&b, "Files to Inspect First", manifest.Predicted.PrimaryFiles)
	writeInlinePredictedFiles(&b, "Tests to Inspect First", manifest.Predicted.Tests)
	fmt.Fprintln(&b, "## Expected Change Surface")
	if len(manifest.Predicted.PrimaryFiles) == 0 {
		fmt.Fprintln(&b, "- Unknown. Identify the primary file before editing.")
	} else {
		for _, file := range manifest.Predicted.PrimaryFiles {
			fmt.Fprintf(&b, "- `%s`\n", file.Path)
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Out-of-Scope Areas")
	fmt.Fprintln(&b, "- Replanning the whole thread unless evidence says this slice should split or be superseded.")
	fmt.Fprintln(&b, "- Broad pack-ranking changes unless they are necessary for this task.")
	fmt.Fprintln(&b, "- Treating the generated context as complete without verification.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Risks")
	for _, unknown := range taskKnownUnknowns(manifest) {
		fmt.Fprintf(&b, "- %s\n", unknown)
	}
	if len(manifest.Predicted.NoiseRisks) > 0 {
		fmt.Fprintln(&b, "- Initial pack includes downgraded noise candidates; avoid editing them unless verification supports it.")
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Success Criteria")
	fmt.Fprintln(&b, "- [ ] Primary implementation surface is verified before edits.")
	fmt.Fprintln(&b, "- [ ] Relevant tests are found or the test-surface miss is recorded.")
	fmt.Fprintln(&b, "- [ ] Changes stay inside the bounded slice.")
	fmt.Fprintln(&b, "- [ ] A checkpoint records actual files, tests, misses, noise, and decision.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Tasks")
	fmt.Fprintln(&b, "- [ ] Inspect the predicted primary files.")
	fmt.Fprintln(&b, "- [ ] Inspect same-package, same-stem, or receipt-related tests.")
	fmt.Fprintln(&b, "- [ ] Refine the slice if context is incomplete.")
	fmt.Fprintln(&b, "- [ ] Implement the smallest useful change.")
	fmt.Fprintln(&b, "- [ ] Run focused validation.")
	if slice.Result != "" {
		fmt.Fprintf(&b, "- [ ] Update `%s` or run `ds task checkpoint`.\n", slice.Result)
	} else {
		fmt.Fprintln(&b, "- [ ] Update the slice result or run `ds task checkpoint`.")
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Decision Gates")
	fmt.Fprintln(&b, "- Promote: the workspace was useful enough and misses are actionable.")
	fmt.Fprintln(&b, "- Improve: useful start, but incomplete/noisy enough to require template or retrieval changes.")
	fmt.Fprintln(&b, "- Rework: task workspace feels like planning overhead or fails to capture useful evidence.")
	fmt.Fprintln(&b, "- Rollback: workspace creates false confidence or worsens agent performance.")
	return b.String()
}

func renderTaskSliceResultTemplate(manifest taskManifest, slice taskSliceArtifact) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Task %s %s Result\n\n", manifest.TaskID, slice.ID)
	fmt.Fprintln(&b, "## Goal")
	fmt.Fprintf(&b, "Record what happened for `%s`.\n\n", slice.Title)
	fmt.Fprintln(&b, "## Description")
	fmt.Fprintln(&b, "Fill this after the implementation attempt or use `ds task checkpoint` to append structured progress.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Resources")
	fmt.Fprintln(&b, "- `A00-index.md`")
	if slice.Plan != "" {
		fmt.Fprintf(&b, "- `%s`\n", slice.Plan)
	}
	fmt.Fprintln(&b, "- `task.json`")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## What Was Attempted")
	fmt.Fprintln(&b, "- ")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Files Actually Read")
	fmt.Fprintln(&b, "- ")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Files Actually Edited")
	fmt.Fprintln(&b, "- ")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Tests Actually Read")
	fmt.Fprintln(&b, "- ")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Tests Actually Run")
	fmt.Fprintln(&b, "- ")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Relevant Files DevSpecs Found")
	for _, file := range predictedFilePaths(manifest.Predicted.PrimaryFiles) {
		fmt.Fprintf(&b, "- `%s`\n", file)
	}
	for _, file := range predictedFilePaths(manifest.Predicted.Tests) {
		fmt.Fprintf(&b, "- `%s`\n", file)
	}
	if len(manifest.Predicted.PrimaryFiles) == 0 && len(manifest.Predicted.Tests) == 0 {
		fmt.Fprintln(&b, "- ")
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Critical Files DevSpecs Missed")
	fmt.Fprintln(&b, "- ")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Distracting Files DevSpecs Included")
	for _, file := range predictedFilePaths(manifest.Predicted.NoiseRisks) {
		fmt.Fprintf(&b, "- `%s`\n", file)
	}
	if len(manifest.Predicted.NoiseRisks) == 0 {
		fmt.Fprintln(&b, "- ")
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Unexpected Discoveries")
	fmt.Fprintln(&b, "- ")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Outcome")
	fmt.Fprintln(&b, "- ")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Success Criteria")
	fmt.Fprintln(&b, "- [ ] Primary file hit evaluated.")
	fmt.Fprintln(&b, "- [ ] Critical-path recall evaluated.")
	fmt.Fprintln(&b, "- [ ] Test companion recall evaluated.")
	fmt.Fprintln(&b, "- [ ] Noise count evaluated.")
	fmt.Fprintln(&b, "- [ ] Usefulness class assigned: A, B, C, or D.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Tasks")
	fmt.Fprintln(&b, "- [ ] Classify hits, misses, noise, companion misses, receipt misses, and confidence mismatch.")
	fmt.Fprintln(&b, "- [ ] Decide whether the next iteration should promote, improve, rework, or rollback.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Decision Gates")
	fmt.Fprintln(&b, "- Promote")
	fmt.Fprintln(&b, "- Improve")
	fmt.Fprintln(&b, "- Rework")
	fmt.Fprintln(&b, "- Rollback")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Next Recommended Slice")
	fmt.Fprintln(&b, "- ")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Checkpoint Notes")
	fmt.Fprintln(&b, "- ")
	return b.String()
}

func writePredictedFiles(b *strings.Builder, title string, files []taskPredictedFile) {
	fmt.Fprintf(b, "## %s\n", title)
	if len(files) == 0 {
		fmt.Fprintln(b, "None found in the initial preflight.")
		fmt.Fprintln(b)
		return
	}
	for _, file := range files {
		if file.Path != "" {
			fmt.Fprintf(b, "- `%s`", file.Path)
		} else {
			fmt.Fprintf(b, "- `%s`", file.Title)
		}
		if file.Title != "" && file.Title != file.Path {
			fmt.Fprintf(b, " - %s", file.Title)
		}
		fmt.Fprintln(b)
		if len(file.Evidence) > 0 {
			fmt.Fprintf(b, "  Evidence: %s\n", strings.Join(firstStrings(file.Evidence, 3), "; "))
		}
	}
	fmt.Fprintln(b)
}

func writeFreshnessWarnings(b *strings.Builder, warnings []taskFreshnessWarning) {
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintln(b, "## Freshness Warnings")
	fmt.Fprintln(b, "These on-disk paths look task-related but were not present in the indexed candidate set. Treat them as stale-index risk, not proof that the initial pack is wrong.")
	fmt.Fprintln(b)
	for _, warning := range warnings {
		fmt.Fprintf(b, "- `%s`", warning.Path)
		if warning.Reason != "" {
			fmt.Fprintf(b, " - %s", warning.Reason)
		}
		fmt.Fprintln(b)
	}
	fmt.Fprintln(b)
}

func writeInlinePredictedFiles(b *strings.Builder, title string, files []taskPredictedFile) {
	fmt.Fprintf(b, "### %s\n", title)
	if len(files) == 0 {
		fmt.Fprintln(b, "- None found. Search before editing.")
		fmt.Fprintln(b)
		return
	}
	for _, file := range files {
		fmt.Fprintf(b, "- `%s`\n", file.Path)
	}
	fmt.Fprintln(b)
}

func writeGitReceipts(b *strings.Builder, predicted taskPredictedContext) {
	fmt.Fprintln(b, "## Related Git Receipts")
	if len(predicted.RelatedGitReceipts) == 0 {
		fmt.Fprintln(b, "None found from packed paths.")
		fmt.Fprintln(b)
		return
	}
	for _, receipt := range predicted.RelatedGitReceipts {
		label := receipt.ShortSHA
		if label == "" {
			label = receipt.SHA
		}
		fmt.Fprintf(b, "- `%s`", label)
		if receipt.CommittedAt != "" {
			fmt.Fprintf(b, " %s", receipt.CommittedAt)
		}
		fmt.Fprintf(b, " - %s\n", receipt.Subject)
		if len(receipt.MatchedPaths) > 0 {
			fmt.Fprintf(b, "  Matched paths: `%s`\n", strings.Join(receipt.MatchedPaths, "`, `"))
		}
		if len(receipt.RelatedPaths) > 0 {
			fmt.Fprintf(b, "  Related touched files not predicted: `%s`\n", strings.Join(receipt.RelatedPaths, "`, `"))
		}
	}
	fmt.Fprintln(b)
}

func writeStringList(b *strings.Builder, title string, values []string, empty string) {
	fmt.Fprintf(b, "## %s\n", title)
	if len(values) == 0 {
		fmt.Fprintf(b, "%s\n\n", empty)
		return
	}
	for _, value := range values {
		fmt.Fprintf(b, "- `%s`\n", value)
	}
	fmt.Fprintln(b)
}

func writeConfidenceSummary(b *strings.Builder, confidence taskConfidence) {
	fmt.Fprintln(b, "## Confidence Summary")
	fmt.Fprintf(b, "- Primary file confidence: %s\n", confidence.PrimaryFileConfidence)
	fmt.Fprintf(b, "- Test coverage confidence: %s\n", confidence.TestCoverageConfidence)
	fmt.Fprintf(b, "- Docs/config coverage confidence: %s\n", confidence.DocsConfigConfidence)
	fmt.Fprintf(b, "- Git receipt confidence: %s\n", confidence.GitReceiptConfidence)
	fmt.Fprintf(b, "- Noise risk: %s\n", confidence.NoiseRisk)
	fmt.Fprintf(b, "- Pack completeness: %s\n", confidence.PackCompleteness)
	if len(confidence.Why) > 0 {
		fmt.Fprintln(b)
		fmt.Fprintln(b, "Why:")
		for _, why := range confidence.Why {
			fmt.Fprintf(b, "- %s\n", why)
		}
	}
	fmt.Fprintln(b)
	fmt.Fprintln(b, "Agent instruction:")
	fmt.Fprintf(b, "%s\n\n", confidence.AgentInstruction)
}

func taskKnownKnowns(manifest taskManifest) []string {
	var out []string
	if len(manifest.Predicted.PrimaryFiles) > 0 {
		out = append(out, "The preflight found likely primary implementation files.")
	}
	if len(manifest.Predicted.Tests) > 0 {
		out = append(out, "The preflight found likely behavior/test artifacts.")
	}
	if len(manifest.Predicted.RelatedGitReceipts) > 0 {
		out = append(out, "Git receipts provide historical trust evidence for packed paths.")
	}
	if len(out) == 0 {
		out = append(out, "The task workspace was created, but the initial evidence is sparse.")
	}
	return out
}

func taskKnownUnknowns(manifest taskManifest) []string {
	var out []string
	if len(manifest.Predicted.PrimaryFiles) == 0 {
		out = append(out, "Primary implementation surface is unknown.")
	}
	if len(manifest.Predicted.Tests) == 0 {
		out = append(out, "Relevant tests may be missing from the initial pack.")
	}
	if len(manifest.Predicted.ReceiptMissingFiles) > 0 {
		out = append(out, "Related Git receipts touched files that were not admitted to the initial context.")
	}
	if len(manifest.FreshnessWarnings) > 0 {
		out = append(out, "On-disk task anchors may be missing from the indexed candidate set.")
	}
	if manifest.Confidence.PackCompleteness != "high" {
		out = append(out, "Pack completeness is not high; verify the working set before editing.")
	}
	return uniqueStrings(out)
}

func firstPredictedResources(predicted taskPredictedContext) []string {
	var out []string
	for _, list := range [][]taskPredictedFile{
		predicted.PrimaryFiles,
		predicted.Tests,
		predicted.DocsPlansConfig,
		predicted.SupportingContext,
	} {
		for _, file := range list {
			if file.Path != "" {
				out = appendUniqueString(out, file.Path)
			}
			if len(out) >= 8 {
				return out
			}
		}
	}
	return out
}

func runTaskCheckpoint(cmd *cobra.Command, taskID string, opts taskCheckpointOptions) error {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	if !isAllowedValue(opts.Stage, taskLifecycleStages) {
		return fmt.Errorf("invalid stage %q; valid values: %s", opts.Stage, strings.Join(taskLifecycleStages, ", "))
	}
	if opts.Decision != "" && !isAllowedValue(opts.Decision, taskDecisions) {
		return fmt.Errorf("invalid decision %q; valid values: %s", opts.Decision, strings.Join(taskDecisions, ", "))
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	repoRoot := canonicalRepoRoot(resolveRepoRootFromWd(wd))
	workspace := taskWorkspacePath(repoRoot, opts.Dir, taskID)
	manifest, err := readTaskManifest(filepath.Join(workspace, taskManifestFilename))
	if err != nil {
		return err
	}
	opts = normalizeTaskCheckpointOptions(opts)
	selectedSlice, err := taskSliceForCheckpoint(manifest, opts.Slice)
	if err != nil {
		return err
	}
	if strings.TrimSpace(selectedSlice.Result) == "" {
		return fmt.Errorf("selected task slice %q has no result artifact", selectedSlice.ID)
	}
	now := time.Now().UTC()
	checkpointDir := filepath.Join(workspace, "checkpoints")
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		return fmt.Errorf("create checkpoint dir: %w", err)
	}
	checkpointStem := fmt.Sprintf("%s-%s", now.Format("20060102-150405"), sanitizeTaskFilename(opts.Stage))
	checkpointPath := filepath.Join(checkpointDir, checkpointStem+".md")
	checkpointJSONPath := filepath.Join(checkpointDir, checkpointStem+".json")
	record := buildTaskCheckpointRecord(manifest, opts, selectedSlice, now, repoRoot)
	jsonRel := taskRelativePath(workspace, checkpointJSONPath)
	body := renderTaskCheckpoint(manifest, selectedSlice, opts, now, jsonRel)
	if err := os.WriteFile(checkpointPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	if err := writeTaskCheckpointRecord(checkpointJSONPath, record); err != nil {
		return err
	}
	resultPath := filepath.Join(workspace, selectedSlice.Result)
	if err := appendTaskCheckpointToResult(resultPath, checkpointPath, checkpointJSONPath, workspace, opts, now); err != nil {
		return err
	}

	var indexed []string
	if opts.Index {
		indexed, err = captureTaskArtifacts(cmd, repoRoot, []taskCaptureRequest{{
			Path:   checkpointPath,
			Title:  "Task " + taskID + " checkpoint " + opts.Stage,
			Status: taskArtifactStatus(opts.Stage, opts.Decision),
		}})
		if err != nil {
			return err
		}
	}

	out := taskCheckpointOutput{
		TaskID:             taskID,
		Slice:              selectedSlice.ID,
		Stage:              opts.Stage,
		Decision:           opts.Decision,
		CheckpointPath:     checkpointPath,
		CheckpointJSONPath: checkpointJSONPath,
		ResultPath:         resultPath,
		IndexedPaths:       indexed,
		TestEvidenceCount:  len(record.Evidence.TestCommands),
	}
	if record.Evidence.GitDiff != nil {
		out.GitDiffFiles = record.Evidence.GitDiff.ChangedFiles
	}
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Recorded checkpoint: %s\n", checkpointPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Structured checkpoint: %s\n", checkpointJSONPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated result: %s\n", resultPath)
	if len(indexed) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Indexed: %s\n", strings.Join(indexed, ", "))
	}
	return nil
}

func normalizeTaskCheckpointOptions(opts taskCheckpointOptions) taskCheckpointOptions {
	opts.Resources = normalizeList(opts.Resources)
	opts.FilesRead = normalizePathList(opts.FilesRead)
	opts.FilesEdited = normalizePathList(opts.FilesEdited)
	opts.TestsRead = normalizePathList(opts.TestsRead)
	opts.TestsRun = normalizeList(opts.TestsRun)
	opts.MissedFiles = normalizePathList(opts.MissedFiles)
	opts.NoiseFiles = normalizePathList(opts.NoiseFiles)
	opts.Tasks = normalizeList(opts.Tasks)
	if opts.GitDiffMax <= 0 {
		opts.GitDiffMax = 12000
	}
	if opts.TestMax <= 0 {
		opts.TestMax = 12000
	}
	return opts
}

func buildTaskCheckpointRecord(manifest taskManifest, opts taskCheckpointOptions, slice taskSliceArtifact, now time.Time, repoRoot string) taskCheckpointRecord {
	goal := strings.TrimSpace(opts.Goal)
	if goal == "" {
		goal = fmt.Sprintf("Record progress for `%s`.", manifest.Query)
	}
	description := strings.TrimSpace(opts.Description)
	if description == "" {
		description = "Checkpoint generated by `ds task checkpoint`."
	}
	record := taskCheckpointRecord{
		SchemaVersion: 1,
		TaskID:        manifest.TaskID,
		Query:         manifest.Query,
		Slice:         slice.ID,
		SliceTitle:    slice.Title,
		Stage:         opts.Stage,
		Decision:      opts.Decision,
		CreatedAt:     now.Format(time.RFC3339),
		Goal:          goal,
		Description:   description,
		Note:          strings.TrimSpace(opts.Note),
		Resources:     appendUniqueValues(taskCheckpointResourcePaths(manifest, slice), opts.Resources...),
		FilesRead:     opts.FilesRead,
		FilesEdited:   opts.FilesEdited,
		TestsRead:     opts.TestsRead,
		TestsRun:      opts.TestsRun,
		MissedFiles:   opts.MissedFiles,
		NoiseFiles:    opts.NoiseFiles,
		Tasks:         opts.Tasks,
	}
	if opts.GitDiff {
		gitDiff := collectTaskGitDiffEvidence(repoRoot, opts.GitDiffMax)
		record.Evidence.GitDiff = &gitDiff
	}
	if opts.TestOutput {
		for _, command := range opts.TestsRun {
			record.Evidence.TestCommands = append(record.Evidence.TestCommands, runTaskCommandEvidence(repoRoot, command, opts.TestMax))
		}
	}
	return record
}

func writeTaskCheckpointRecord(path string, record taskCheckpointRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write checkpoint JSON: %w", err)
	}
	return nil
}

func collectTaskGitDiffEvidence(repoRoot string, maxBytes int) taskGitDiffEvidence {
	commandLabel := "git diff --stat; git diff --name-only; git diff --cached --name-only; git ls-files --others --exclude-standard"
	evidence := taskGitDiffEvidence{
		Command:  commandLabel,
		MaxBytes: maxBytes,
	}
	stat, statTruncated, statErr := runBoundedGitCommand(repoRoot, maxBytes, "diff", "--stat")
	evidence.Stat = stat
	evidence.Truncated = statTruncated
	if statErr != nil {
		evidence.Error = statErr.Error()
	}
	for _, args := range [][]string{
		{"diff", "--name-only"},
		{"diff", "--cached", "--name-only"},
		{"ls-files", "--others", "--exclude-standard"},
	} {
		output, truncated, err := runBoundedGitCommand(repoRoot, maxBytes, args...)
		if truncated {
			evidence.Truncated = true
		}
		if err != nil {
			if evidence.Error == "" {
				evidence.Error = err.Error()
			}
			continue
		}
		for _, line := range strings.Split(output, "\n") {
			evidence.ChangedFiles = appendNormalizedUnique(evidence.ChangedFiles, line)
		}
	}
	return evidence
}

func runBoundedGitCommand(repoRoot string, maxBytes int, args ...string) (string, bool, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	text, truncated := boundedText(string(output), maxBytes)
	if err != nil {
		message := strings.TrimSpace(text)
		if message == "" {
			message = err.Error()
		}
		return text, truncated, errors.New(message)
	}
	return text, truncated, nil
}

func runTaskCommandEvidence(repoRoot, command string, maxBytes int) taskCommandRunEvidence {
	evidence := taskCommandRunEvidence{
		Command:  command,
		ExitCode: 0,
		MaxBytes: maxBytes,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	shell, args := shellCommand(command)
	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	evidence.Output, evidence.Truncated = boundedText(string(output), maxBytes)
	if ctx.Err() == context.DeadlineExceeded {
		evidence.TimedOut = true
		evidence.ExitCode = -1
		evidence.Error = ctx.Err().Error()
		return evidence
	}
	if err != nil {
		evidence.Error = err.Error()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			evidence.ExitCode = exitErr.ExitCode()
		} else {
			evidence.ExitCode = -1
		}
	}
	return evidence
}

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", command}
	}
	return "sh", []string{"-c", command}
}

func boundedText(text string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text, false
	}
	return text[:maxBytes], true
}

func taskRelativePath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func renderTaskCheckpoint(manifest taskManifest, slice taskSliceArtifact, opts taskCheckpointOptions, now time.Time, checkpointJSONRel string) string {
	var b strings.Builder
	fmt.Fprintln(&b, "---")
	fmt.Fprintf(&b, "task_id: %s\n", yamlScalar(manifest.TaskID))
	if slice.ID != "" {
		fmt.Fprintf(&b, "slice: %s\n", yamlScalar(slice.ID))
	}
	fmt.Fprintf(&b, "stage: %s\n", yamlScalar(opts.Stage))
	if strings.TrimSpace(opts.Decision) != "" {
		fmt.Fprintf(&b, "decision: %s\n", yamlScalar(opts.Decision))
	}
	fmt.Fprintf(&b, "created_at: %s\n", yamlScalar(now.Format(time.RFC3339)))
	fmt.Fprintf(&b, "checkpoint_json: %s\n", yamlScalar(checkpointJSONRel))
	fmt.Fprintln(&b, "---")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "# Task %s Checkpoint\n\n", manifest.TaskID)
	fmt.Fprintln(&b, "## Goal")
	if strings.TrimSpace(opts.Goal) != "" {
		fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(opts.Goal))
	} else {
		fmt.Fprintf(&b, "Record progress for `%s`.\n\n", manifest.Query)
	}
	fmt.Fprintln(&b, "## Description")
	if strings.TrimSpace(opts.Description) != "" {
		fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(opts.Description))
	} else {
		fmt.Fprintln(&b, "Checkpoint generated by `ds task checkpoint`.")
		fmt.Fprintln(&b)
	}
	fmt.Fprintln(&b, "## Resources")
	for _, resource := range taskCheckpointResourcePaths(manifest, slice) {
		fmt.Fprintf(&b, "- `%s`\n", resource)
	}
	for _, resource := range normalizeList(opts.Resources) {
		fmt.Fprintf(&b, "- `%s`\n", resource)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Note")
	fmt.Fprintf(&b, "%s\n\n", emptyAsDash(opts.Note))
	fmt.Fprintln(&b, "## Structured Evidence")
	fmt.Fprintf(&b, "- `%s`\n\n", checkpointJSONRel)
	writeMarkdownList(&b, "Files Actually Read", opts.FilesRead)
	writeMarkdownList(&b, "Files Actually Edited", opts.FilesEdited)
	writeMarkdownList(&b, "Tests Actually Read", opts.TestsRead)
	writeMarkdownList(&b, "Tests Actually Run", opts.TestsRun)
	writeMarkdownList(&b, "Critical Files DevSpecs Missed", opts.MissedFiles)
	writeMarkdownList(&b, "Distracting Files DevSpecs Included", opts.NoiseFiles)
	fmt.Fprintln(&b, "## Success Criteria")
	fmt.Fprintln(&b, "- [ ] Checkpoint records actual context used.")
	fmt.Fprintln(&b, "- [ ] Misses and noise are explicit when observed.")
	fmt.Fprintln(&b, "- [ ] Decision gate is recorded.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Tasks")
	if len(opts.Tasks) == 0 {
		fmt.Fprintln(&b, "- [ ] Update the next slice if needed.")
	} else {
		for _, task := range normalizeList(opts.Tasks) {
			fmt.Fprintf(&b, "- [ ] %s\n", task)
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Decision Gates")
	fmt.Fprintln(&b, "- Promote")
	fmt.Fprintln(&b, "- Improve")
	fmt.Fprintln(&b, "- Rework")
	fmt.Fprintln(&b, "- Rollback")
	return b.String()
}

func yamlScalar(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "\"\""
	}
	if strings.ContainsAny(value, " \t:") {
		escaped := strings.ReplaceAll(value, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		return "\"" + escaped + "\""
	}
	return value
}

func appendTaskCheckpointToResult(resultPath, checkpointPath, checkpointJSONPath, workspace string, opts taskCheckpointOptions, now time.Time) error {
	rel, err := filepath.Rel(workspace, checkpointPath)
	if err != nil {
		rel = checkpointPath
	}
	rel = filepath.ToSlash(rel)
	jsonRel := taskRelativePath(workspace, checkpointJSONPath)
	var b strings.Builder
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "### Checkpoint")
	fmt.Fprintf(&b, "- Created At: %s\n", now.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Stage: %s\n", opts.Stage)
	fmt.Fprintf(&b, "- Decision: %s\n", emptyAsDash(opts.Decision))
	fmt.Fprintf(&b, "- Source: `%s`\n", rel)
	fmt.Fprintf(&b, "- Structured Evidence: `%s`\n", jsonRel)
	if strings.TrimSpace(opts.Note) != "" {
		fmt.Fprintf(&b, "- Note: %s\n", strings.TrimSpace(opts.Note))
	}
	writeIndentedResultList(&b, "Files read", opts.FilesRead)
	writeIndentedResultList(&b, "Files edited", opts.FilesEdited)
	writeIndentedResultList(&b, "Tests read", opts.TestsRead)
	writeIndentedResultList(&b, "Tests run", opts.TestsRun)
	writeIndentedResultList(&b, "Missed files", opts.MissedFiles)
	writeIndentedResultList(&b, "Noise files", opts.NoiseFiles)
	return appendFile(resultPath, b.String())
}

func writeIndentedResultList(b *strings.Builder, label string, values []string) {
	values = normalizeList(values)
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "- %s:\n", label)
	for _, value := range values {
		fmt.Fprintf(b, "  - `%s`\n", value)
	}
}

func writeMarkdownList(b *strings.Builder, title string, values []string) {
	fmt.Fprintf(b, "## %s\n", title)
	values = normalizeList(values)
	if len(values) == 0 {
		fmt.Fprintln(b, "- ")
		fmt.Fprintln(b)
		return
	}
	for _, value := range values {
		fmt.Fprintf(b, "- `%s`\n", value)
	}
	fmt.Fprintln(b)
}

func writeTaskStartHuman(out io.Writer, result taskStartOutput, confidence taskConfidence) error {
	fmt.Fprintf(out, "Created task workspace: %s\n", result.Workspace)
	fmt.Fprintf(out, "Task ID: %s\n", result.TaskID)
	fmt.Fprintf(out, "A00: %s\n", result.IndexPath)
	for _, slice := range result.Slices {
		fmt.Fprintf(out, "%s plan: %s\n", slice.ID, slice.PlanPath)
		fmt.Fprintf(out, "%s result: %s\n", slice.ID, slice.ResultPath)
	}
	if len(result.Slices) == 0 {
		fmt.Fprintf(out, "A01 plan: %s\n", result.FirstSlicePath)
		fmt.Fprintf(out, "A01 result: %s\n", result.ResultPath)
	}
	fmt.Fprintf(out, "Confidence: primary=%s tests=%s completeness=%s noise=%s\n",
		confidence.PrimaryFileConfidence,
		confidence.TestCoverageConfidence,
		confidence.PackCompleteness,
		confidence.NoiseRisk,
	)
	if len(result.FreshnessWarnings) > 0 {
		fmt.Fprintf(out, "Freshness warnings: %d on-disk anchor(s) were not in the indexed candidate set\n", len(result.FreshnessWarnings))
		for _, warning := range firstTaskFreshnessWarnings(result.FreshnessWarnings, 3) {
			fmt.Fprintf(out, "  - %s", warning.Path)
			if warning.Reason != "" {
				fmt.Fprintf(out, " (%s)", warning.Reason)
			}
			fmt.Fprintln(out)
		}
	}
	if len(result.IndexedPaths) > 0 {
		fmt.Fprintf(out, "Indexed: %s\n", strings.Join(result.IndexedPaths, ", "))
	}
	return nil
}

func firstTaskFreshnessWarnings(warnings []taskFreshnessWarning, limit int) []taskFreshnessWarning {
	if limit <= 0 || len(warnings) <= limit {
		return warnings
	}
	return warnings[:limit]
}

func writeTaskManifest(path string, manifest taskManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

func readTaskManifest(path string) (taskManifest, error) {
	var manifest taskManifest
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest, fmt.Errorf("read task manifest: %w", err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, fmt.Errorf("parse task manifest: %w", err)
	}
	return manifest, nil
}

func prepareTaskWorkspace(workspace string, force bool) error {
	if _, err := os.Stat(workspace); err == nil {
		if !force {
			return fmt.Errorf("task workspace already exists: %s", workspace)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return fmt.Errorf("create task workspace: %w", err)
	}
	return nil
}

func taskWorkspacePath(repoRoot, baseDir, taskID string) string {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		baseDir = defaultTaskWorkspaceDir
	}
	if filepath.IsAbs(baseDir) {
		return filepath.Join(baseDir, taskID)
	}
	return filepath.Join(repoRoot, filepath.FromSlash(baseDir), taskID)
}

func generatedTaskID(query string, now time.Time) string {
	return now.Format("20060102-150405") + "-" + sanitizeTaskFilename(query)
}

func sanitizeTaskFilename(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "task"
	}
	if len(out) > 48 {
		out = strings.Trim(out[:48], "-")
	}
	return out
}

func validateTaskID(taskID string) error {
	if taskID == "" {
		return fmt.Errorf("task id is empty")
	}
	for _, r := range taskID {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("task id contains unsupported character %q", r)
	}
	return nil
}

func predictedFilePaths(files []taskPredictedFile) []string {
	var out []string
	for _, file := range files {
		if file.Path != "" {
			out = appendUniqueString(out, file.Path)
		}
	}
	return out
}

func normalizeList(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func emptyAsDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func appendFile(path, body string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("append %s: %w", path, err)
	}
	defer f.Close()
	_, err = f.WriteString(body)
	return err
}

type taskCaptureRequest struct {
	Path   string
	Title  string
	Status string
}

func captureTaskArtifacts(cmd *cobra.Command, repoRoot string, requests []taskCaptureRequest) ([]string, error) {
	origWd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(repoRoot); err != nil {
		return nil, fmt.Errorf("chdir repo root for capture: %w", err)
	}
	defer os.Chdir(origWd)

	var indexed []string
	for _, request := range requests {
		silent := &cobra.Command{}
		silent.SetOut(io.Discard)
		silent.SetErr(io.Discard)
		status := request.Status
		if status == "" {
			status = "implementing"
		}
		if err := runCapture(silent, request.Path, config.KindPlan, request.Title, status, false); err != nil {
			return indexed, fmt.Errorf("index task artifact %s: %w", request.Path, err)
		}
		rel, err := filepath.Rel(repoRoot, request.Path)
		if err != nil {
			rel = request.Path
		}
		indexed = append(indexed, filepath.ToSlash(rel))
	}
	return indexed, nil
}

func taskArtifactStatus(stage, decision string) string {
	stage = strings.ToLower(strings.TrimSpace(stage))
	decision = strings.ToLower(strings.TrimSpace(decision))
	switch {
	case decision == "completed" || stage == "completed" || stage == "done" || stage == "implemented":
		return "implemented"
	case decision == "superseded" || decision == "split" || stage == "superseded" || stage == "split":
		return "superseded"
	case stage == "packed" || stage == "planned" || stage == "started":
		return "implementing"
	default:
		return "unknown"
	}
}

func isAllowedValue(value string, allowed []string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	var out []string
	for _, value := range values {
		out = appendUniqueString(out, value)
	}
	return out
}
