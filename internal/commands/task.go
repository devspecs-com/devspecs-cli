package commands

import (
	"bytes"
	"context"
	"database/sql"
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
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

const (
	defaultTaskWorkspaceDir = ".devspecs/tasks"
	taskManifestFilename    = "task.json"
	taskProfileCodeChange   = "code-change"
	taskProfileGreenfield   = "greenfield"
)

var taskLifecycleStages = []string{
	"packed",
	"planned",
	"started",
	"implemented",
	"validated",
	"done",
	"blocked",
	"completed",
	"split",
	"superseded",
	"cancelled",
	"rolled_back",
}

var taskDecisions = []string{
	"promote",
	"improve",
	"rework",
	"rollback",
	"block",
	"blocked",
	"complete",
	"completed",
	"split",
	"supersede",
	"superseded",
	"cancel",
	"cancelled",
	"continue",
}

type taskStartOptions struct {
	ID        string
	Dir       string
	Series    string
	Profile   string
	Slices    []string
	NoRefresh bool
	AsJSON    bool
	Force     bool
	Index     bool
}

type taskArtifactAddOptions struct {
	Dir    string
	Slice  string
	Reason string
	AsJSON bool
	Index  bool
}

type taskStatusOptions struct {
	Dir    string
	AsJSON bool
}

type taskSyncOptions struct {
	Dir    string
	AsJSON bool
}

type taskTargetOptions struct {
	Dir    string
	Target string
	AsJSON bool
}

type taskTargetStateOptions struct {
	Dir      string
	Target   string
	Stage    string
	Decision string
	Index    bool
	AsJSON   bool
}

type taskAuditOptions struct {
	Dir     string
	Target  string
	GitDiff bool
	AsJSON  bool
}

type taskDecideOptions struct {
	Dir      string
	Target   string
	Stage    string
	Decision string
	Index    bool
	AsJSON   bool
}

type taskCheckpointOptions struct {
	Dir          string
	Slice        string
	Stage        string
	Decision     string
	Note         string
	Description  string
	Goal         string
	Resources    []string
	FilesRead    []string
	FilesEdited  []string
	TestsRead    []string
	TestsRun     []string
	MissedFiles  []string
	NoiseFiles   []string
	Tasks        []string
	Learnings    []string
	NextTarget   string
	NextDecision string
	GitDiff      bool
	GitDiffMax   int
	TestOutput   bool
	TestMax      int
	Index        bool
	AsJSON       bool
}

type taskStartOutput struct {
	TaskID            string                 `json:"task_id"`
	Series            string                 `json:"series"`
	Profile           string                 `json:"profile"`
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
	RiskCards         []taskRiskCard         `json:"risk_cards,omitempty"`
	PackCompleteness  string                 `json:"pack_completeness"`
}

type taskStartSliceOutput struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	PlanPath   string `json:"plan_path"`
	ResultPath string `json:"result_path"`
}

type taskArtifactAddOutput struct {
	TaskID       string               `json:"task_id"`
	Series       string               `json:"series"`
	Slice        taskStartSliceOutput `json:"slice"`
	ManifestPath string               `json:"manifest_path"`
	IndexPath    string               `json:"index_path"`
	IndexedPaths []string             `json:"indexed_paths,omitempty"`
}

type taskStatusOutput struct {
	TaskID               string                  `json:"task_id"`
	Series               string                  `json:"series"`
	Profile              string                  `json:"profile,omitempty"`
	Status               string                  `json:"status"`
	Decision             string                  `json:"decision,omitempty"`
	UpdatedAt            string                  `json:"updated_at,omitempty"`
	LatestCheckpointID   string                  `json:"latest_checkpoint_id,omitempty"`
	LatestCheckpoint     string                  `json:"latest_checkpoint,omitempty"`
	LatestCheckpointJSON string                  `json:"latest_checkpoint_json,omitempty"`
	Slices               []taskStatusSliceOutput `json:"slices,omitempty"`
	ArtifactFreshness    []taskArtifactFreshness `json:"artifact_freshness,omitempty"`
}

type taskStatusSliceOutput struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	Kind                 string `json:"kind,omitempty"`
	ParentID             string `json:"parent_id,omitempty"`
	Reason               string `json:"reason,omitempty"`
	Stage                string `json:"stage,omitempty"`
	Decision             string `json:"decision,omitempty"`
	UpdatedAt            string `json:"updated_at,omitempty"`
	LatestCheckpointID   string `json:"latest_checkpoint_id,omitempty"`
	LatestCheckpoint     string `json:"latest_checkpoint,omitempty"`
	LatestCheckpointJSON string `json:"latest_checkpoint_json,omitempty"`
	Plan                 string `json:"plan"`
	Result               string `json:"result"`
}

type taskDecideOutput struct {
	TaskID       string   `json:"task_id"`
	Series       string   `json:"series"`
	Target       string   `json:"target"`
	Stage        string   `json:"stage,omitempty"`
	Decision     string   `json:"decision"`
	ManifestPath string   `json:"manifest_path"`
	IndexPath    string   `json:"index_path"`
	IndexedPaths []string `json:"indexed_paths,omitempty"`
}

type taskCheckpointOutput struct {
	TaskID             string   `json:"task_id"`
	Series             string   `json:"series,omitempty"`
	Slice              string   `json:"slice,omitempty"`
	CheckpointID       string   `json:"checkpoint_id,omitempty"`
	Stage              string   `json:"stage"`
	Decision           string   `json:"decision,omitempty"`
	CheckpointPath     string   `json:"checkpoint_path"`
	CheckpointJSONPath string   `json:"checkpoint_json_path"`
	ResultPath         string   `json:"result_path"`
	IndexedPaths       []string `json:"indexed_paths,omitempty"`
	GitDiffFiles       []string `json:"git_diff_files,omitempty"`
	LearningCount      int      `json:"learning_count,omitempty"`
	FactIndexed        bool     `json:"fact_indexed,omitempty"`
	TestEvidenceCount  int      `json:"test_evidence_count,omitempty"`
}

type taskSyncOutput struct {
	TaskID            string                  `json:"task_id"`
	Series            string                  `json:"series"`
	Profile           string                  `json:"profile,omitempty"`
	Workspace         string                  `json:"workspace"`
	ManifestPath      string                  `json:"manifest_path"`
	IndexedPaths      []string                `json:"indexed_paths,omitempty"`
	ArtifactFreshness []taskArtifactFreshness `json:"artifact_freshness,omitempty"`
}

type taskArtifactFreshness struct {
	Path           string `json:"path"`
	Kind           string `json:"kind,omitempty"`
	ModifiedAt     string `json:"modified_at"`
	StateUpdatedAt string `json:"state_updated_at"`
	Reason         string `json:"reason"`
}

type taskTargetOutput struct {
	TaskID            string                  `json:"task_id"`
	Series            string                  `json:"series"`
	Profile           string                  `json:"profile,omitempty"`
	Query             string                  `json:"query"`
	Workspace         string                  `json:"workspace"`
	IndexPath         string                  `json:"index_path"`
	Target            string                  `json:"target"`
	Title             string                  `json:"title"`
	Kind              string                  `json:"kind,omitempty"`
	ParentID          string                  `json:"parent_id,omitempty"`
	Reason            string                  `json:"reason,omitempty"`
	Stage             string                  `json:"stage,omitempty"`
	Decision          string                  `json:"decision,omitempty"`
	PlanPath          string                  `json:"plan_path"`
	ResultPath        string                  `json:"result_path"`
	SiblingTargets    []string                `json:"sibling_targets,omitempty"`
	ArtifactFreshness []taskArtifactFreshness `json:"artifact_freshness,omitempty"`
	PlanBody          string                  `json:"plan_body,omitempty"`
}

type taskPromptOutput struct {
	TaskID         string           `json:"task_id"`
	Target         string           `json:"target"`
	Prompt         string           `json:"prompt"`
	TargetContext  taskTargetOutput `json:"target_context"`
	SiblingTargets []string         `json:"sibling_targets,omitempty"`
}

type taskAuditOutput struct {
	TaskID          string   `json:"task_id"`
	Series          string   `json:"series"`
	Target          string   `json:"target"`
	Title           string   `json:"title"`
	Recommendation  string   `json:"recommendation"`
	ObservedEdited  []string `json:"observed_edited,omitempty"`
	GitDiffFiles    []string `json:"git_diff_files,omitempty"`
	InScopePaths    []string `json:"in_scope_paths,omitempty"`
	ReviewPaths     []string `json:"review_paths,omitempty"`
	OutOfScopePaths []string `json:"out_of_scope_paths,omitempty"`
	AllowedSurface  []string `json:"allowed_surface,omitempty"`
	ReviewSurface   []string `json:"review_surface,omitempty"`
	Checkpoints     []string `json:"checkpoints,omitempty"`
	Notes           []string `json:"notes,omitempty"`
}

type taskManifest struct {
	TaskID               string                 `json:"task_id"`
	Series               string                 `json:"series,omitempty"`
	Profile              string                 `json:"profile,omitempty"`
	Query                string                 `json:"query"`
	Status               string                 `json:"status"`
	Decision             string                 `json:"decision,omitempty"`
	CreatedAt            string                 `json:"created_at"`
	UpdatedAt            string                 `json:"updated_at,omitempty"`
	LatestCheckpointID   string                 `json:"latest_checkpoint_id,omitempty"`
	LatestCheckpoint     string                 `json:"latest_checkpoint,omitempty"`
	LatestCheckpointJSON string                 `json:"latest_checkpoint_json,omitempty"`
	RepoRoot             string                 `json:"repo_root"`
	Workspace            string                 `json:"workspace"`
	Artifacts            taskArtifactPaths      `json:"artifacts"`
	Predicted            taskPredictedContext   `json:"predicted_context"`
	FreshnessWarnings    []taskFreshnessWarning `json:"freshness_warnings,omitempty"`
	RiskCards            []taskRiskCard         `json:"risk_cards,omitempty"`
	Confidence           taskConfidence         `json:"confidence"`
}

type taskArtifactPaths struct {
	Series     string              `json:"series,omitempty"`
	Index      string              `json:"index"`
	FirstSlice string              `json:"first_slice,omitempty"`
	Result     string              `json:"result,omitempty"`
	Slices     []taskSliceArtifact `json:"slices,omitempty"`
}

type taskSliceArtifact struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	Plan                 string `json:"plan"`
	Result               string `json:"result"`
	Kind                 string `json:"kind,omitempty"`
	ParentID             string `json:"parent_id,omitempty"`
	Reason               string `json:"reason,omitempty"`
	Stage                string `json:"stage,omitempty"`
	Decision             string `json:"decision,omitempty"`
	UpdatedAt            string `json:"updated_at,omitempty"`
	LatestCheckpointID   string `json:"latest_checkpoint_id,omitempty"`
	LatestCheckpoint     string `json:"latest_checkpoint,omitempty"`
	LatestCheckpointJSON string `json:"latest_checkpoint_json,omitempty"`
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

type taskRiskCard struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Severity   string   `json:"severity"`
	Source     string   `json:"source"`
	Evidence   []string `json:"evidence,omitempty"`
	AgentCheck string   `json:"agent_check"`
	Count      int      `json:"count,omitempty"`
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
	SchemaVersion            int                              `json:"schema_version"`
	CheckpointID             string                           `json:"checkpoint_id,omitempty"`
	TaskID                   string                           `json:"task_id"`
	Target                   string                           `json:"target,omitempty"`
	Series                   string                           `json:"series,omitempty"`
	Query                    string                           `json:"query,omitempty"`
	Slice                    string                           `json:"slice,omitempty"`
	SliceTitle               string                           `json:"slice_title,omitempty"`
	ParentSlice              string                           `json:"parent_slice,omitempty"`
	Iteration                string                           `json:"iteration,omitempty"`
	Stage                    string                           `json:"stage"`
	Decision                 string                           `json:"decision,omitempty"`
	CreatedAt                string                           `json:"created_at"`
	Goal                     string                           `json:"goal,omitempty"`
	Description              string                           `json:"description,omitempty"`
	Note                     string                           `json:"note,omitempty"`
	Resources                []string                         `json:"resources,omitempty"`
	FilesRead                []string                         `json:"files_read,omitempty"`
	FilesEdited              []string                         `json:"files_edited,omitempty"`
	TestsRead                []string                         `json:"tests_read,omitempty"`
	TestsRun                 []string                         `json:"tests_run,omitempty"`
	MissedFiles              []string                         `json:"missed_files,omitempty"`
	NoiseFiles               []string                         `json:"noise_files,omitempty"`
	Tasks                    []string                         `json:"tasks,omitempty"`
	ActualContext            taskCheckpointActualContext      `json:"actual_context,omitempty"`
	PredictedContextFeedback taskPredictedContextFeedback     `json:"predicted_context_feedback,omitempty"`
	Evidence                 taskCheckpointEvidence           `json:"evidence,omitempty"`
	Learnings                []taskCheckpointLearning         `json:"learnings,omitempty"`
	Next                     taskCheckpointNextRecommendation `json:"next,omitempty"`
}

type taskCheckpointActualContext struct {
	FilesRead   []string `json:"files_read,omitempty"`
	FilesEdited []string `json:"files_edited,omitempty"`
	TestsRead   []string `json:"tests_read,omitempty"`
	TestsRun    []string `json:"tests_run,omitempty"`
}

type taskPredictedContextFeedback struct {
	RelevantFound       []string `json:"relevant_found,omitempty"`
	CriticalMissed      []string `json:"critical_missed,omitempty"`
	DistractingIncluded []string `json:"distracting_included,omitempty"`
}

type taskCheckpointLearning struct {
	LearningType string   `json:"learning_type"`
	Summary      string   `json:"summary"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	AppliesTo    string   `json:"applies_to,omitempty"`
	Confidence   string   `json:"confidence,omitempty"`
}

type taskCheckpointNextRecommendation struct {
	RecommendedTarget   string `json:"recommended_target,omitempty"`
	RecommendedDecision string `json:"recommended_decision,omitempty"`
}

type taskCheckpointEvidence struct {
	GitDiff      *taskGitDiffEvidence     `json:"git_diff,omitempty"`
	TestCommands []taskCommandRunEvidence `json:"test_commands,omitempty"`
	GitDiffPaths []string                 `json:"git_diff_paths,omitempty"`
	PlanRefs     []string                 `json:"plan_refs,omitempty"`
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
	opts.Profile = taskProfileCodeChange
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
	cmd.Flags().StringVar(&opts.Series, "series", "A", "Series prefix for generated artifacts, e.g. A or B")
	cmd.Flags().StringVar(&opts.Profile, "profile", taskProfileCodeChange, "Task generation profile: code-change or greenfield")
	cmd.Flags().StringArrayVar(&opts.Slices, "slice", nil, "Task slice title to scaffold; may be repeated")
	cmd.Flags().BoolVar(&opts.NoRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Overwrite an existing task workspace")
	cmd.Flags().BoolVar(&opts.Index, "index", true, "Capture the series index and slice plans into the DevSpecs index")

	cmd.AddCommand(newTaskSliceCmd())
	cmd.AddCommand(newTaskIterationCmd())
	cmd.AddCommand(newTaskSyncCmd())
	cmd.AddCommand(newTaskNextCmd())
	cmd.AddCommand(newTaskShowCmd())
	cmd.AddCommand(newTaskPromptCmd())
	cmd.AddCommand(newTaskStartTargetCmd())
	cmd.AddCommand(newTaskFinishCmd())
	cmd.AddCommand(newTaskAuditCmd())
	cmd.AddCommand(newTaskStatusCmd())
	cmd.AddCommand(newTaskDecideCmd())
	cmd.AddCommand(newTaskCheckpointCmd())
	cmd.AddCommand(newTaskEvaluateCmd())
	return cmd
}

func newTaskSliceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "slice",
		Short: "Manage task slice artifacts",
	}
	cmd.AddCommand(newTaskSliceAddCmd())
	return cmd
}

func newTaskSliceAddCmd() *cobra.Command {
	var opts taskArtifactAddOptions
	opts.Dir = defaultTaskWorkspaceDir
	opts.Index = true
	cmd := &cobra.Command{
		Use:   "add <task-id> <title>",
		Short: "Add a new slice plan/result pair to a task workspace",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskSliceAdd(cmd, args[0], args[1], opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().BoolVar(&opts.Index, "index", true, "Capture the updated index and new slice plan into the DevSpecs index")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func newTaskIterationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "iteration",
		Short: "Manage task slice iteration artifacts",
	}
	cmd.AddCommand(newTaskIterationAddCmd())
	return cmd
}

func newTaskIterationAddCmd() *cobra.Command {
	var opts taskArtifactAddOptions
	opts.Dir = defaultTaskWorkspaceDir
	opts.Reason = "improve"
	opts.Index = true
	cmd := &cobra.Command{
		Use:   "add <task-id> <title>",
		Short: "Add a new iteration plan/result pair under an existing slice",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskIterationAdd(cmd, args[0], args[1], opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().StringVar(&opts.Slice, "slice", "", "Parent slice ID/title/plan/result; defaults to the first slice")
	cmd.Flags().StringVar(&opts.Reason, "reason", opts.Reason, "Iteration reason, usually improve or rework")
	cmd.Flags().BoolVar(&opts.Index, "index", true, "Capture the updated index and new iteration plan into the DevSpecs index")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func newTaskSyncCmd() *cobra.Command {
	var opts taskSyncOptions
	opts.Dir = defaultTaskWorkspaceDir
	cmd := &cobra.Command{
		Use:   "sync <task-id>",
		Short: "Recapture task workspace artifacts into the DevSpecs index",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskSync(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func newTaskNextCmd() *cobra.Command {
	var opts taskTargetOptions
	opts.Dir = defaultTaskWorkspaceDir
	cmd := &cobra.Command{
		Use:   "next <task-id>",
		Short: "Show the next bounded task target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskNext(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func newTaskShowCmd() *cobra.Command {
	var opts taskTargetOptions
	opts.Dir = defaultTaskWorkspaceDir
	cmd := &cobra.Command{
		Use:   "show <task-id>",
		Short: "Show exact context for one task target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskShow(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Slice/iteration target; defaults to the next target")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func newTaskPromptCmd() *cobra.Command {
	var opts taskTargetOptions
	opts.Dir = defaultTaskWorkspaceDir
	cmd := &cobra.Command{
		Use:   "prompt <task-id>",
		Short: "Emit an agent prompt bounded to one task target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskPrompt(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Slice/iteration target; defaults to the next target")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func newTaskStartTargetCmd() *cobra.Command {
	var opts taskTargetStateOptions
	opts.Dir = defaultTaskWorkspaceDir
	opts.Index = true
	cmd := &cobra.Command{
		Use:   "start <task-id>",
		Short: "Mark one task target as started",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskStartTarget(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Slice/iteration target; defaults to the next target")
	cmd.Flags().BoolVar(&opts.Index, "index", true, "Capture the updated task index into the DevSpecs index")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func newTaskFinishCmd() *cobra.Command {
	var opts taskTargetStateOptions
	opts.Dir = defaultTaskWorkspaceDir
	opts.Index = true
	cmd := &cobra.Command{
		Use:   "finish <task-id>",
		Short: "Finish one task target with a decision gate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskFinish(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Slice/iteration target; defaults to the started or next target")
	cmd.Flags().StringVar(&opts.Stage, "stage", "", "Lifecycle stage to set; inferred from decision when omitted")
	cmd.Flags().StringVar(&opts.Decision, "decision", "", "Decision gate: promote, improve, rework, rollback, block, complete, split, supersede, cancel, continue")
	cmd.Flags().BoolVar(&opts.Index, "index", true, "Capture the updated task index into the DevSpecs index")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func newTaskAuditCmd() *cobra.Command {
	var opts taskAuditOptions
	opts.Dir = defaultTaskWorkspaceDir
	cmd := &cobra.Command{
		Use:   "audit <task-id>",
		Short: "Audit whether observed edits stayed inside one task target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskAudit(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Slice/iteration target; defaults to the next target")
	cmd.Flags().BoolVar(&opts.GitDiff, "git-diff", false, "Include current git diff changed files in the audit")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func newTaskStatusCmd() *cobra.Command {
	var opts taskStatusOptions
	opts.Dir = defaultTaskWorkspaceDir
	cmd := &cobra.Command{
		Use:   "status <task-id>",
		Short: "Show task series, slice, and iteration state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskStatus(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func newTaskDecideCmd() *cobra.Command {
	var opts taskDecideOptions
	opts.Dir = defaultTaskWorkspaceDir
	opts.Index = true
	cmd := &cobra.Command{
		Use:   "decide <task-id>",
		Short: "Update a task series, slice, or iteration decision gate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTaskDecide(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Series, slice, or iteration target ID/title/plan/result")
	cmd.Flags().StringVar(&opts.Stage, "stage", "", "Lifecycle stage to set; inferred from terminal decisions when omitted")
	cmd.Flags().StringVar(&opts.Decision, "decision", "", "Decision gate: promote, improve, rework, rollback, block, complete, split, supersede, cancel, continue")
	cmd.Flags().BoolVar(&opts.Index, "index", true, "Capture the updated task index into the DevSpecs index")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
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
	cmd.Flags().StringVar(&opts.Stage, "stage", opts.Stage, "Lifecycle stage: packed, planned, started, implemented, validated, done, blocked, completed, split, superseded, cancelled, rolled_back")
	cmd.Flags().StringVar(&opts.Decision, "decision", opts.Decision, "Decision gate: promote, improve, rework, rollback, block, complete, split, supersede, cancel, continue")
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
	cmd.Flags().StringArrayVar(&opts.Learnings, "learning", nil, "Compact learning as type: summary or type|summary|confidence|applies_to|refs; may be repeated")
	cmd.Flags().StringVar(&opts.NextTarget, "next-target", "", "Recommended next task target")
	cmd.Flags().StringVar(&opts.NextDecision, "next-decision", "", "Recommended next decision gate")
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
	series, err := normalizeTaskSeries(opts.Series)
	if err != nil {
		return err
	}
	profile, err := normalizeTaskProfile(opts.Profile)
	if err != nil {
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
	slices := taskSliceArtifacts(series, query, opts.Slices)
	firstSlice := firstTaskSliceArtifact(taskArtifactPaths{Slices: slices})
	relArtifacts := taskArtifactPaths{
		Series:     series,
		Index:      taskSeriesIndexFilename(series),
		FirstSlice: firstSlice.Plan,
		Result:     firstSlice.Result,
		Slices:     slices,
	}
	manifest := taskManifest{
		TaskID:            taskID,
		Series:            series,
		Profile:           profile,
		Query:             query,
		Status:            "packed",
		CreatedAt:         now.Format(time.RFC3339),
		RepoRoot:          repoRoot,
		Workspace:         filepath.ToSlash(workspace),
		Artifacts:         relArtifacts,
		Predicted:         preflight.Predicted,
		FreshnessWarnings: preflight.FreshnessWarnings,
		RiskCards:         preflight.RiskCards,
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
		Series:            series,
		Profile:           profile,
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
		RiskCards:         preflight.RiskCards,
		PackCompleteness:  preflight.Confidence.PackCompleteness,
	}
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	return writeTaskStartHuman(cmd.OutOrStdout(), out, preflight.Confidence)
}

func runTaskSliceAdd(cmd *cobra.Command, taskID, title string, opts taskArtifactAddOptions) error {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("slice title is empty")
	}
	repoRoot, workspace, manifest, err := loadTaskWorkspaceManifest(opts.Dir, taskID)
	if err != nil {
		return err
	}
	series := defaultTaskSeries(manifest.Series)
	slice := newTaskSliceArtifact(series, nextTaskSliceOrdinal(manifest), title, taskUsedSliceSlugs(manifest))
	manifest.Artifacts.Slices = append(manifest.Artifacts.Slices, slice)
	if manifest.Artifacts.FirstSlice == "" {
		manifest.Artifacts.FirstSlice = slice.Plan
	}
	if manifest.Artifacts.Result == "" {
		manifest.Artifacts.Result = slice.Result
	}
	return writeAddedTaskArtifact(cmd, repoRoot, workspace, manifest, slice, opts)
}

func runTaskIterationAdd(cmd *cobra.Command, taskID, title string, opts taskArtifactAddOptions) error {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("iteration title is empty")
	}
	reason := strings.TrimSpace(opts.Reason)
	if reason == "" {
		reason = "improve"
	}
	repoRoot, workspace, manifest, err := loadTaskWorkspaceManifest(opts.Dir, taskID)
	if err != nil {
		return err
	}
	parent, err := taskSliceForCheckpoint(manifest, opts.Slice)
	if err != nil {
		return err
	}
	if strings.Contains(parent.ID, "-") {
		return fmt.Errorf("cannot add an iteration under iteration %q; choose a parent slice", parent.ID)
	}
	iteration := newTaskIterationArtifact(parent, nextTaskIterationOrdinal(manifest, parent.ID), title, reason, taskUsedSliceSlugs(manifest))
	manifest.Artifacts.Slices = append(manifest.Artifacts.Slices, iteration)
	return writeAddedTaskArtifact(cmd, repoRoot, workspace, manifest, iteration, opts)
}

func loadTaskWorkspaceManifest(baseDir, taskID string) (string, string, taskManifest, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", "", taskManifest{}, err
	}
	repoRoot := canonicalRepoRoot(resolveRepoRootFromWd(wd))
	workspace := taskWorkspacePath(repoRoot, baseDir, taskID)
	manifest, err := readTaskManifest(filepath.Join(workspace, taskManifestFilename))
	if err != nil {
		return "", "", taskManifest{}, err
	}
	return repoRoot, workspace, manifest, nil
}

func writeAddedTaskArtifact(cmd *cobra.Command, repoRoot, workspace string, manifest taskManifest, slice taskSliceArtifact, opts taskArtifactAddOptions) error {
	indexPath := filepath.Join(workspace, manifest.Artifacts.Index)
	planPath := filepath.Join(workspace, slice.Plan)
	resultPath := filepath.Join(workspace, slice.Result)
	files := map[string]string{
		indexPath:  renderTaskIndex(manifest),
		planPath:   renderTaskSlicePlan(manifest, slice),
		resultPath: renderTaskSliceResultTemplate(manifest, slice),
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
	var err error
	if opts.Index {
		indexed, err = captureTaskArtifacts(cmd, repoRoot, []taskCaptureRequest{
			{Path: indexPath, Title: "Task " + manifest.TaskID + " preflight", Status: "implementing"},
			{Path: planPath, Title: "Task " + manifest.TaskID + " " + slice.ID + " plan: " + slice.Title, Status: "implementing"},
		})
		if err != nil {
			return err
		}
	}

	out := taskArtifactAddOutput{
		TaskID:       manifest.TaskID,
		Series:       defaultTaskSeries(manifest.Series),
		Slice:        taskStartSliceOutput{ID: slice.ID, Title: slice.Title, PlanPath: planPath, ResultPath: resultPath},
		ManifestPath: manifestPath,
		IndexPath:    indexPath,
		IndexedPaths: indexed,
	}
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Added %s: %s\n", slice.ID, slice.Title)
	fmt.Fprintf(cmd.OutOrStdout(), "%s plan: %s\n", slice.ID, planPath)
	fmt.Fprintf(cmd.OutOrStdout(), "%s result: %s\n", slice.ID, resultPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated index: %s\n", indexPath)
	if len(indexed) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Indexed: %s\n", strings.Join(indexed, ", "))
	}
	return nil
}

func runTaskSync(cmd *cobra.Command, taskID string, opts taskSyncOptions) error {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	repoRoot, workspace, manifest, err := loadTaskWorkspaceManifest(opts.Dir, taskID)
	if err != nil {
		return err
	}
	before := taskArtifactFreshnessWarnings(workspace, manifest)
	requests := taskSyncCaptureRequests(workspace, manifest)
	if len(requests) == 0 {
		return fmt.Errorf("no task artifacts found to sync for %s", taskID)
	}
	indexed, err := captureTaskArtifacts(cmd, repoRoot, requests)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	manifest.UpdatedAt = now.Format(time.RFC3339)
	manifestPath := filepath.Join(workspace, taskManifestFilename)
	if err := writeTaskManifest(manifestPath, manifest); err != nil {
		return err
	}

	out := taskSyncOutput{
		TaskID:            manifest.TaskID,
		Series:            defaultTaskSeries(manifest.Series),
		Profile:           defaultTaskProfile(manifest.Profile),
		Workspace:         workspace,
		ManifestPath:      manifestPath,
		IndexedPaths:      indexed,
		ArtifactFreshness: before,
	}
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Synced task workspace: %s\n", workspace)
	fmt.Fprintf(cmd.OutOrStdout(), "Task ID: %s\n", out.TaskID)
	fmt.Fprintf(cmd.OutOrStdout(), "Series: %s\n", out.Series)
	fmt.Fprintf(cmd.OutOrStdout(), "Profile: %s\n", out.Profile)
	fmt.Fprintf(cmd.OutOrStdout(), "Manifest: %s\n", manifestPath)
	if len(before) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Freshened %d edited task artifact(s).\n", len(before))
	}
	if len(indexed) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Indexed: %s\n", strings.Join(indexed, ", "))
	}
	return nil
}

func runTaskNext(cmd *cobra.Command, taskID string, opts taskTargetOptions) error {
	ctx, err := loadTaskTargetContext(opts.Dir, taskID, "")
	if err != nil {
		return err
	}
	out := taskTargetOutputFromContext(ctx, false)
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	return writeTaskTargetHuman(cmd.OutOrStdout(), "Next task target", out, false)
}

func runTaskShow(cmd *cobra.Command, taskID string, opts taskTargetOptions) error {
	ctx, err := loadTaskTargetContext(opts.Dir, taskID, opts.Target)
	if err != nil {
		return err
	}
	out := taskTargetOutputFromContext(ctx, true)
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	return writeTaskTargetHuman(cmd.OutOrStdout(), "Task target", out, true)
}

func runTaskPrompt(cmd *cobra.Command, taskID string, opts taskTargetOptions) error {
	ctx, err := loadTaskTargetContext(opts.Dir, taskID, opts.Target)
	if err != nil {
		return err
	}
	target := taskTargetOutputFromContext(ctx, true)
	prompt := renderTaskAgentPrompt(ctx, target)
	out := taskPromptOutput{
		TaskID:         target.TaskID,
		Target:         target.Target,
		Prompt:         prompt,
		TargetContext:  target,
		SiblingTargets: target.SiblingTargets,
	}
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), prompt)
	return err
}

func runTaskStartTarget(cmd *cobra.Command, taskID string, opts taskTargetStateOptions) error {
	ctx, err := loadTaskTargetContext(opts.Dir, taskID, opts.Target)
	if err != nil {
		return err
	}
	return runTaskDecide(cmd, taskID, taskDecideOptions{
		Dir:      opts.Dir,
		Target:   ctx.Slice.ID,
		Stage:    "started",
		Decision: "continue",
		Index:    opts.Index,
		AsJSON:   opts.AsJSON,
	})
}

func runTaskFinish(cmd *cobra.Command, taskID string, opts taskTargetStateOptions) error {
	decision := strings.TrimSpace(opts.Decision)
	if decision == "" {
		return fmt.Errorf("decision is required")
	}
	if !isAllowedValue(decision, taskDecisions) {
		return fmt.Errorf("invalid decision %q; valid values: %s", decision, strings.Join(taskDecisions, ", "))
	}
	ctx, err := loadTaskTargetContext(opts.Dir, taskID, opts.Target)
	if err != nil {
		return err
	}
	stage := strings.TrimSpace(opts.Stage)
	if stage == "" {
		stage = taskStageForDecision(decision)
	}
	if stage == "" {
		if strings.EqualFold(decision, "continue") {
			stage = "started"
		} else {
			stage = "implemented"
		}
	}
	return runTaskDecide(cmd, taskID, taskDecideOptions{
		Dir:      opts.Dir,
		Target:   ctx.Slice.ID,
		Stage:    stage,
		Decision: decision,
		Index:    opts.Index,
		AsJSON:   opts.AsJSON,
	})
}

func runTaskAudit(cmd *cobra.Command, taskID string, opts taskAuditOptions) error {
	ctx, err := loadTaskTargetContext(opts.Dir, taskID, opts.Target)
	if err != nil {
		return err
	}
	out, err := buildTaskAuditOutput(ctx, opts.GitDiff)
	if err != nil {
		return err
	}
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	return writeTaskAuditHuman(cmd.OutOrStdout(), out)
}

func runTaskStatus(cmd *cobra.Command, taskID string, opts taskStatusOptions) error {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	_, workspace, manifest, err := loadTaskWorkspaceManifest(opts.Dir, taskID)
	if err != nil {
		return err
	}
	out := taskStatusFromManifest(manifest)
	out.ArtifactFreshness = taskArtifactFreshnessWarnings(workspace, manifest)
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	return writeTaskStatusHuman(cmd.OutOrStdout(), out)
}

func taskStatusFromManifest(manifest taskManifest) taskStatusOutput {
	out := taskStatusOutput{
		TaskID:               manifest.TaskID,
		Series:               defaultTaskSeries(manifest.Series),
		Profile:              defaultTaskProfile(manifest.Profile),
		Status:               manifest.Status,
		Decision:             manifest.Decision,
		UpdatedAt:            manifest.UpdatedAt,
		LatestCheckpointID:   manifest.LatestCheckpointID,
		LatestCheckpoint:     manifest.LatestCheckpoint,
		LatestCheckpointJSON: manifest.LatestCheckpointJSON,
	}
	for _, slice := range manifest.Artifacts.Slices {
		out.Slices = append(out.Slices, taskStatusSliceOutput{
			ID:                   slice.ID,
			Title:                slice.Title,
			Kind:                 slice.Kind,
			ParentID:             slice.ParentID,
			Reason:               slice.Reason,
			Stage:                slice.Stage,
			Decision:             slice.Decision,
			UpdatedAt:            slice.UpdatedAt,
			LatestCheckpointID:   slice.LatestCheckpointID,
			LatestCheckpoint:     slice.LatestCheckpoint,
			LatestCheckpointJSON: slice.LatestCheckpointJSON,
			Plan:                 slice.Plan,
			Result:               slice.Result,
		})
	}
	return out
}

func writeTaskStatusHuman(out io.Writer, status taskStatusOutput) error {
	fmt.Fprintf(out, "Task ID: %s\n", status.TaskID)
	fmt.Fprintf(out, "Series: %s\n", status.Series)
	if status.Profile != "" {
		fmt.Fprintf(out, "Profile: %s\n", status.Profile)
	}
	fmt.Fprintf(out, "Status: %s\n", emptyAsDash(status.Status))
	if status.Decision != "" {
		fmt.Fprintf(out, "Decision: %s\n", status.Decision)
	}
	if status.UpdatedAt != "" {
		fmt.Fprintf(out, "Updated At: %s\n", status.UpdatedAt)
	}
	if status.LatestCheckpoint != "" {
		fmt.Fprintf(out, "Latest Checkpoint: %s\n", status.LatestCheckpoint)
	}
	if status.LatestCheckpointID != "" {
		fmt.Fprintf(out, "Latest Checkpoint ID: %s\n", status.LatestCheckpointID)
	}
	if status.LatestCheckpointJSON != "" {
		fmt.Fprintf(out, "Latest Checkpoint JSON: %s\n", status.LatestCheckpointJSON)
	}
	if len(status.ArtifactFreshness) > 0 {
		fmt.Fprintln(out, "Stale task artifacts:")
		for _, warning := range status.ArtifactFreshness {
			fmt.Fprintf(out, "  - %s changed after task state; run `ds task sync %s`.\n", warning.Path, status.TaskID)
		}
	}
	for _, slice := range status.Slices {
		fmt.Fprintf(out, "%s: %s", slice.ID, slice.Title)
		if slice.Kind != "" {
			fmt.Fprintf(out, " [%s]", slice.Kind)
		}
		if slice.ParentID != "" {
			fmt.Fprintf(out, " parent=%s", slice.ParentID)
		}
		if slice.Stage != "" {
			fmt.Fprintf(out, " stage=%s", slice.Stage)
		}
		if slice.Decision != "" {
			fmt.Fprintf(out, " decision=%s", slice.Decision)
		}
		if slice.LatestCheckpoint != "" {
			fmt.Fprintf(out, " checkpoint=%s", slice.LatestCheckpoint)
		}
		if slice.LatestCheckpointID != "" {
			fmt.Fprintf(out, " checkpoint_id=%s", slice.LatestCheckpointID)
		}
		fmt.Fprintln(out)
	}
	return nil
}

func runTaskDecide(cmd *cobra.Command, taskID string, opts taskDecideOptions) error {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return err
	}
	target := strings.TrimSpace(opts.Target)
	if target == "" {
		return fmt.Errorf("decision target is required")
	}
	decision := strings.TrimSpace(opts.Decision)
	if decision == "" {
		return fmt.Errorf("decision is required")
	}
	if !isAllowedValue(decision, taskDecisions) {
		return fmt.Errorf("invalid decision %q; valid values: %s", decision, strings.Join(taskDecisions, ", "))
	}
	stage := strings.TrimSpace(opts.Stage)
	if stage == "" {
		stage = taskStageForDecision(decision)
	}
	if stage != "" && !isAllowedValue(stage, taskLifecycleStages) {
		return fmt.Errorf("invalid stage %q; valid values: %s", stage, strings.Join(taskLifecycleStages, ", "))
	}

	repoRoot, workspace, manifest, err := loadTaskWorkspaceManifest(opts.Dir, taskID)
	if err != nil {
		return err
	}
	targetID := target
	if isTaskSeriesTarget(manifest, target) {
		targetID = defaultTaskSeries(manifest.Series) + "00"
	} else {
		slice, err := taskSliceForCheckpoint(manifest, target)
		if err != nil {
			return err
		}
		targetID = slice.ID
	}

	now := time.Now().UTC()
	applyTaskTargetState(&manifest, targetID, stage, decision, now)
	stage, decision = taskStateForTarget(manifest, targetID)

	manifestPath := filepath.Join(workspace, taskManifestFilename)
	if err := writeTaskManifest(manifestPath, manifest); err != nil {
		return err
	}
	indexPath := filepath.Join(workspace, manifest.Artifacts.Index)
	if err := os.WriteFile(indexPath, []byte(renderTaskIndex(manifest)), 0o644); err != nil {
		return fmt.Errorf("write task index: %w", err)
	}

	var indexed []string
	if opts.Index {
		indexed, err = captureTaskArtifacts(cmd, repoRoot, []taskCaptureRequest{{
			Path:   indexPath,
			Title:  "Task " + taskID + " preflight",
			Status: taskArtifactStatus(stage, decision),
		}})
		if err != nil {
			return err
		}
	}

	out := taskDecideOutput{
		TaskID:       manifest.TaskID,
		Series:       defaultTaskSeries(manifest.Series),
		Target:       targetID,
		Stage:        stage,
		Decision:     decision,
		ManifestPath: manifestPath,
		IndexPath:    indexPath,
		IndexedPaths: indexed,
	}
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Updated %s: stage=%s decision=%s\n", targetID, emptyAsDash(stage), decision)
	fmt.Fprintf(cmd.OutOrStdout(), "Manifest: %s\n", manifestPath)
	fmt.Fprintf(cmd.OutOrStdout(), "Index: %s\n", indexPath)
	if len(indexed) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Indexed: %s\n", strings.Join(indexed, ", "))
	}
	return nil
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

type taskTargetContext struct {
	RepoRoot          string
	Workspace         string
	ManifestPath      string
	IndexPath         string
	Manifest          taskManifest
	Slice             taskSliceArtifact
	SiblingTargets    []string
	ArtifactFreshness []taskArtifactFreshness
}

func loadTaskTargetContext(baseDir, taskID, selector string) (taskTargetContext, error) {
	taskID = strings.TrimSpace(taskID)
	if err := validateTaskID(taskID); err != nil {
		return taskTargetContext{}, err
	}
	repoRoot, workspace, manifest, err := loadTaskWorkspaceManifest(baseDir, taskID)
	if err != nil {
		return taskTargetContext{}, err
	}
	var slice taskSliceArtifact
	if strings.TrimSpace(selector) == "" {
		slice, err = taskNextSlice(manifest)
	} else {
		slice, err = taskSliceForCheckpoint(manifest, selector)
	}
	if err != nil {
		return taskTargetContext{}, err
	}
	return taskTargetContext{
		RepoRoot:          repoRoot,
		Workspace:         workspace,
		ManifestPath:      filepath.Join(workspace, taskManifestFilename),
		IndexPath:         filepath.Join(workspace, manifest.Artifacts.Index),
		Manifest:          manifest,
		Slice:             slice,
		SiblingTargets:    taskSiblingTargetIDs(manifest, slice.ID),
		ArtifactFreshness: taskArtifactFreshnessWarnings(workspace, manifest),
	}, nil
}

func taskTargetOutputFromContext(ctx taskTargetContext, includePlanBody bool) taskTargetOutput {
	planPath := filepath.Join(ctx.Workspace, filepath.FromSlash(ctx.Slice.Plan))
	resultPath := filepath.Join(ctx.Workspace, filepath.FromSlash(ctx.Slice.Result))
	out := taskTargetOutput{
		TaskID:            ctx.Manifest.TaskID,
		Series:            defaultTaskSeries(ctx.Manifest.Series),
		Profile:           defaultTaskProfile(ctx.Manifest.Profile),
		Query:             ctx.Manifest.Query,
		Workspace:         ctx.Workspace,
		IndexPath:         ctx.IndexPath,
		Target:            ctx.Slice.ID,
		Title:             ctx.Slice.Title,
		Kind:              ctx.Slice.Kind,
		ParentID:          ctx.Slice.ParentID,
		Reason:            ctx.Slice.Reason,
		Stage:             ctx.Slice.Stage,
		Decision:          ctx.Slice.Decision,
		PlanPath:          planPath,
		ResultPath:        resultPath,
		SiblingTargets:    ctx.SiblingTargets,
		ArtifactFreshness: ctx.ArtifactFreshness,
	}
	if includePlanBody {
		if data, err := os.ReadFile(planPath); err == nil {
			out.PlanBody = string(data)
		}
	}
	return out
}

func writeTaskTargetHuman(out io.Writer, title string, target taskTargetOutput, includePlanBody bool) error {
	fmt.Fprintf(out, "%s: %s\n", title, target.Target)
	fmt.Fprintf(out, "Task ID: %s\n", target.TaskID)
	fmt.Fprintf(out, "Series: %s\n", target.Series)
	if target.Profile != "" {
		fmt.Fprintf(out, "Profile: %s\n", target.Profile)
	}
	fmt.Fprintf(out, "Title: %s\n", target.Title)
	fmt.Fprintf(out, "Stage: %s\n", emptyAsDash(target.Stage))
	fmt.Fprintf(out, "Decision: %s\n", emptyAsDash(target.Decision))
	fmt.Fprintf(out, "Plan: %s\n", target.PlanPath)
	fmt.Fprintf(out, "Result: %s\n", target.ResultPath)
	if len(target.SiblingTargets) > 0 {
		fmt.Fprintf(out, "Out-of-scope sibling targets: %s\n", strings.Join(target.SiblingTargets, ", "))
	}
	if len(target.ArtifactFreshness) > 0 {
		fmt.Fprintln(out, "Stale task artifacts:")
		for _, warning := range target.ArtifactFreshness {
			fmt.Fprintf(out, "  - %s changed after task state; run `ds task sync %s`.\n", warning.Path, target.TaskID)
		}
	}
	if includePlanBody && target.PlanBody != "" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Plan body:")
		fmt.Fprintln(out, target.PlanBody)
	}
	return nil
}

func renderTaskAgentPrompt(ctx taskTargetContext, target taskTargetOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are working on DevSpecs task %s target %s only.\n\n", target.TaskID, target.Target)
	fmt.Fprintln(&b, "Boundary:")
	fmt.Fprintln(&b, "```yaml")
	fmt.Fprintln(&b, "devspecs:")
	fmt.Fprintf(&b, "  task_id: %s\n", target.TaskID)
	fmt.Fprintf(&b, "  target: %s\n", target.Target)
	fmt.Fprintf(&b, "  allowed_scope: %s\n", strings.TrimSpace(firstNonEmptyTaskString(target.Kind, "slice")))
	fmt.Fprintf(&b, "  plan: %s\n", filepath.ToSlash(taskRelativePath(ctx.RepoRoot, target.PlanPath)))
	fmt.Fprintf(&b, "  result: %s\n", filepath.ToSlash(taskRelativePath(ctx.RepoRoot, target.ResultPath)))
	if len(target.SiblingTargets) > 0 {
		fmt.Fprintln(&b, "  must_not_implement:")
		for _, sibling := range target.SiblingTargets {
			fmt.Fprintf(&b, "    - %s\n", sibling)
		}
	}
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Goal: %s\n\n", target.Title)
	writeTaskPromptRiskCards(&b, ctx.Manifest.RiskCards)
	fmt.Fprintln(&b, "Do not implement sibling slices, future slices, or the full task track. Stop after this target's acceptance checks are satisfied.")
	fmt.Fprintf(&b, "Record the outcome in `%s` or with `ds task checkpoint %s --slice %s`.\n", filepath.ToSlash(taskRelativePath(ctx.RepoRoot, target.ResultPath)), target.TaskID, target.Target)
	fmt.Fprintln(&b, "At the end, recommend exactly one decision: promote, improve, rework, rollback, or block.")
	fmt.Fprintln(&b)
	if target.PlanBody != "" {
		fmt.Fprintln(&b, "Target plan:")
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, target.PlanBody)
	}
	return b.String()
}

func writeTaskPromptRiskCards(b *strings.Builder, cards []taskRiskCard) {
	if len(cards) == 0 {
		return
	}
	fmt.Fprintln(b, "Risk cards:")
	fmt.Fprintln(b, "Treat these as evidence-backed checks, not required edit targets.")
	for _, card := range cards {
		fmt.Fprintf(b, "- %s [%s]: %s\n", card.Title, card.Severity, card.AgentCheck)
		if len(card.Evidence) > 0 {
			fmt.Fprintf(b, "  Evidence: %s\n", strings.Join(firstStrings(card.Evidence, 2), "; "))
		}
	}
	fmt.Fprintln(b)
}

func firstNonEmptyTaskString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func taskNextSlice(manifest taskManifest) (taskSliceArtifact, error) {
	slices := taskSyncSlices(manifest)
	for _, slice := range slices {
		if strings.EqualFold(strings.TrimSpace(slice.Stage), "started") && !taskTargetTerminal(slice) {
			return slice, nil
		}
	}
	for _, slice := range slices {
		if !taskTargetTerminal(slice) {
			return slice, nil
		}
	}
	if len(slices) == 0 {
		return taskSliceArtifact{}, fmt.Errorf("task has no slice targets")
	}
	return taskSliceArtifact{}, fmt.Errorf("all task targets are terminal")
}

func taskTargetTerminal(slice taskSliceArtifact) bool {
	decision := strings.ToLower(strings.TrimSpace(slice.Decision))
	if decision != "" && decision != "continue" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(slice.Stage)) {
	case "done", "completed", "blocked", "split", "superseded", "cancelled", "rolled_back":
		return true
	default:
		return false
	}
}

func taskSiblingTargetIDs(manifest taskManifest, targetID string) []string {
	var out []string
	for _, slice := range taskSyncSlices(manifest) {
		if strings.EqualFold(slice.ID, targetID) {
			continue
		}
		out = appendUniqueString(out, slice.ID)
	}
	return out
}

func buildTaskAuditOutput(ctx taskTargetContext, includeCurrentGitDiff bool) (taskAuditOutput, error) {
	observed, checkpoints, err := readTaskObservedPathsForTarget(ctx.Workspace, ctx.Slice.ID)
	if err != nil {
		return taskAuditOutput{}, err
	}
	gitDiffFiles := appendNormalizedUnique(nil, observed.GitDiffFiles...)
	var notes []string
	if includeCurrentGitDiff {
		current := collectTaskGitDiffEvidence(ctx.RepoRoot, 12000)
		gitDiffFiles = appendNormalizedUnique(gitDiffFiles, current.ChangedFiles...)
		if current.Error != "" {
			notes = append(notes, "Current git diff evidence could not be fully collected: "+current.Error)
		}
	}
	allowed := taskAuditAllowedSurface(ctx)
	reviewSurface := taskAuditReviewSurface(ctx)
	observedEdited := appendNormalizedUnique(nil, observed.FilesEdited...)
	observedChanged := appendNormalizedUnique(nil, observedEdited...)
	observedChanged = appendNormalizedUnique(observedChanged, gitDiffFiles...)

	var inScope []string
	var review []string
	var outOfScope []string
	for _, path := range observedChanged {
		switch {
		case containsPath(allowed, path):
			inScope = appendNormalizedUnique(inScope, path)
		case containsPath(reviewSurface, path) || taskAuditPathInWorkspace(ctx, path):
			review = appendNormalizedUnique(review, path)
		default:
			outOfScope = appendNormalizedUnique(outOfScope, path)
		}
	}

	recommendation := "pass"
	if len(outOfScope) > 0 {
		recommendation = "drift"
	} else if len(review) > 0 {
		recommendation = "review"
	}
	if len(observedChanged) == 0 {
		notes = append(notes, "No edited-file or git-diff evidence found for this target yet.")
		recommendation = "review"
	}
	if len(checkpoints) == 0 {
		notes = append(notes, "No structured checkpoint JSON matched this target.")
	}
	if includeCurrentGitDiff {
		notes = append(notes, "Current git diff files were included in the scope audit.")
	}

	return taskAuditOutput{
		TaskID:          ctx.Manifest.TaskID,
		Series:          defaultTaskSeries(ctx.Manifest.Series),
		Target:          ctx.Slice.ID,
		Title:           ctx.Slice.Title,
		Recommendation:  recommendation,
		ObservedEdited:  observedEdited,
		GitDiffFiles:    gitDiffFiles,
		InScopePaths:    inScope,
		ReviewPaths:     review,
		OutOfScopePaths: outOfScope,
		AllowedSurface:  allowed,
		ReviewSurface:   reviewSurface,
		Checkpoints:     checkpoints,
		Notes:           uniqueStrings(notes),
	}, nil
}

func writeTaskAuditHuman(out io.Writer, audit taskAuditOutput) error {
	fmt.Fprintf(out, "Scope audit: %s %s\n", audit.TaskID, audit.Target)
	fmt.Fprintf(out, "Recommendation: %s\n", audit.Recommendation)
	if len(audit.InScopePaths) > 0 {
		fmt.Fprintln(out, "In scope:")
		for _, path := range audit.InScopePaths {
			fmt.Fprintf(out, "  - %s\n", path)
		}
	}
	if len(audit.ReviewPaths) > 0 {
		fmt.Fprintln(out, "Review:")
		for _, path := range audit.ReviewPaths {
			fmt.Fprintf(out, "  - %s\n", path)
		}
	}
	if len(audit.OutOfScopePaths) > 0 {
		fmt.Fprintln(out, "Out of scope:")
		for _, path := range audit.OutOfScopePaths {
			fmt.Fprintf(out, "  - %s\n", path)
		}
	}
	if len(audit.Notes) > 0 {
		fmt.Fprintln(out, "Notes:")
		for _, note := range audit.Notes {
			fmt.Fprintf(out, "  - %s\n", note)
		}
	}
	return nil
}

func readTaskObservedPathsForTarget(workspace, targetID string) (taskObservedPaths, []string, error) {
	var observed taskObservedPaths
	var checkpoints []string
	checkpointDir := filepath.Join(workspace, "checkpoints")
	entries, err := os.ReadDir(checkpointDir)
	if os.IsNotExist(err) {
		return observed, checkpoints, nil
	}
	if err != nil {
		return observed, checkpoints, fmt.Errorf("read checkpoints: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(checkpointDir, entry.Name())
		record, err := readTaskCheckpointRecord(path)
		if err != nil {
			return observed, checkpoints, err
		}
		if !strings.EqualFold(strings.TrimSpace(record.Slice), targetID) {
			continue
		}
		appendObservedFromCheckpointRecord(&observed, record)
		checkpoints = append(checkpoints, taskRelativePath(workspace, path))
	}
	return observed, checkpoints, nil
}

func taskAuditAllowedSurface(ctx taskTargetContext) []string {
	var allowed []string
	allowed = appendNormalizedUnique(allowed, predictedFilePaths(ctx.Manifest.Predicted.PrimaryFiles)...)
	allowed = appendNormalizedUnique(allowed, predictedFilePaths(ctx.Manifest.Predicted.Tests)...)
	allowed = appendNormalizedUnique(allowed, taskAuditWorkspaceArtifactPaths(ctx)...)
	return allowed
}

func taskAuditReviewSurface(ctx taskTargetContext) []string {
	var review []string
	review = appendNormalizedUnique(review, predictedFilePaths(ctx.Manifest.Predicted.DocsPlansConfig)...)
	review = appendNormalizedUnique(review, predictedFilePaths(ctx.Manifest.Predicted.SupportingContext)...)
	for _, path := range taskAuditWorkspaceArtifactPaths(ctx) {
		review = appendNormalizedUnique(review, path)
	}
	return review
}

func taskAuditWorkspaceArtifactPaths(ctx taskTargetContext) []string {
	var out []string
	workspaceRel := taskRelativePath(ctx.RepoRoot, ctx.Workspace)
	add := func(path string) {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" {
			return
		}
		out = appendNormalizedUnique(out, path)
		if workspaceRel != "" && workspaceRel != "." {
			out = appendNormalizedUnique(out, filepath.ToSlash(filepath.Join(workspaceRel, filepath.FromSlash(path))))
		}
	}
	add(taskManifestFilename)
	add(ctx.Manifest.Artifacts.Index)
	add(ctx.Slice.Plan)
	add(ctx.Slice.Result)
	return out
}

func taskAuditPathInWorkspace(ctx taskTargetContext, path string) bool {
	path = normalizeSinglePath(path)
	workspaceRel := normalizeSinglePath(taskRelativePath(ctx.RepoRoot, ctx.Workspace))
	if workspaceRel == "" || workspaceRel == "." {
		return false
	}
	return path == workspaceRel || strings.HasPrefix(path, strings.TrimRight(workspaceRel, "/")+"/")
}

func taskSliceArtifacts(series, query string, values []string) []taskSliceArtifact {
	series = defaultTaskSeries(series)
	titles := normalizeTaskSliceTitles(query, values)
	usedSlugs := map[string]int{}
	var out []taskSliceArtifact
	for i, title := range titles {
		out = append(out, taskSliceArtifactWithSlug(fmt.Sprintf("%s%02d", series, i+1), title, uniqueTaskSliceSlug(title, usedSlugs), "slice", "", ""))
	}
	return out
}

func newTaskSliceArtifact(series string, ordinal int, title string, usedSlugs map[string]int) taskSliceArtifact {
	if ordinal <= 0 {
		ordinal = 1
	}
	id := fmt.Sprintf("%s%02d", defaultTaskSeries(series), ordinal)
	return taskSliceArtifactWithSlug(id, title, uniqueTaskSliceSlug(title, usedSlugs), "slice", "", "")
}

func newTaskIterationArtifact(parent taskSliceArtifact, ordinal int, title, reason string, usedSlugs map[string]int) taskSliceArtifact {
	if ordinal <= 0 {
		ordinal = 1
	}
	id := fmt.Sprintf("%s-%d", parent.ID, ordinal)
	return taskSliceArtifactWithSlug(id, title, uniqueTaskSliceSlug(title, usedSlugs), "iteration", parent.ID, reason)
}

func taskSliceArtifactWithSlug(id, title, slug, kind, parentID, reason string) taskSliceArtifact {
	if kind == "" {
		kind = "slice"
	}
	return taskSliceArtifact{
		ID:       id,
		Title:    title,
		Plan:     fmt.Sprintf("%s-%s-plan.md", id, slug),
		Result:   fmt.Sprintf("%s-%s-result.md", id, slug),
		Kind:     kind,
		ParentID: parentID,
		Reason:   reason,
	}
}

func nextTaskSliceOrdinal(manifest taskManifest) int {
	series := defaultTaskSeries(manifest.Series)
	maxOrdinal := 0
	for _, slice := range manifest.Artifacts.Slices {
		if slice.ParentID != "" || strings.Contains(slice.ID, "-") {
			continue
		}
		id := strings.ToUpper(strings.TrimSpace(slice.ID))
		if !strings.HasPrefix(id, series) {
			continue
		}
		ordinal, ok := parseTaskOrdinal(id[len(series):])
		if ok && ordinal > maxOrdinal {
			maxOrdinal = ordinal
		}
	}
	return maxOrdinal + 1
}

func nextTaskIterationOrdinal(manifest taskManifest, parentID string) int {
	prefix := strings.ToUpper(strings.TrimSpace(parentID)) + "-"
	maxOrdinal := 0
	for _, slice := range manifest.Artifacts.Slices {
		id := strings.ToUpper(strings.TrimSpace(slice.ID))
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		ordinal, ok := parseTaskOrdinal(strings.TrimPrefix(id, prefix))
		if ok && ordinal > maxOrdinal {
			maxOrdinal = ordinal
		}
	}
	return maxOrdinal + 1
}

func parseTaskOrdinal(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	ordinal := 0
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return 0, false
		}
		ordinal = ordinal*10 + int(r-'0')
	}
	return ordinal, true
}

func taskUsedSliceSlugs(manifest taskManifest) map[string]int {
	used := map[string]int{}
	for _, slice := range manifest.Artifacts.Slices {
		path := slice.Plan
		if path == "" {
			path = slice.Result
		}
		base := filepath.Base(filepath.ToSlash(path))
		base = strings.TrimSuffix(base, "-plan.md")
		base = strings.TrimSuffix(base, "-result.md")
		parts := strings.SplitN(base, "-", 2)
		if len(parts) != 2 || parts[1] == "" {
			continue
		}
		used[parts[1]]++
	}
	return used
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
	series := defaultTaskSeries(paths.Series)
	return taskSliceArtifact{
		ID:     series + "01",
		Title:  "First slice",
		Plan:   paths.FirstSlice,
		Result: paths.Result,
	}
}

func taskSeriesIndexFilename(series string) string {
	return fmt.Sprintf("%s00-index.md", defaultTaskSeries(series))
}

func defaultTaskSeries(series string) string {
	series = strings.ToUpper(strings.TrimSpace(series))
	if series == "" {
		return "A"
	}
	return series
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
	index := manifest.Artifacts.Index
	if index == "" {
		index = taskSeriesIndexFilename(manifest.Series)
	}
	resources := []string{"../" + filepath.ToSlash(index)}
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
	RiskCards         []taskRiskCard
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
	riskCards := buildTaskRiskCards(db, repoRoot, query, predicted, freshnessWarnings)
	return taskPreflight{Predicted: predicted, FreshnessWarnings: freshnessWarnings, RiskCards: riskCards, Confidence: confidence}, nil
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
	taskRiskCardLimit     = 6
	taskRiskEvidenceLimit = 4
	taskRiskFactLimit     = 50
)

func buildTaskRiskCards(db *store.DB, repoRoot, query string, predicted taskPredictedContext, warnings []taskFreshnessWarning) []taskRiskCard {
	var cards []taskRiskCard
	if len(warnings) > 0 {
		var evidence []string
		for _, warning := range warnings {
			if warning.Path == "" {
				continue
			}
			item := "`" + warning.Path + "`"
			if warning.Reason != "" {
				item += " - " + warning.Reason
			}
			evidence = appendUniqueString(evidence, item)
		}
		cards = appendTaskRiskCard(cards, taskRiskCard{
			ID:         "stale-index",
			Title:      "On-disk task anchors were not in the indexed candidate set",
			Severity:   "medium",
			Source:     "freshness",
			Evidence:   firstStrings(evidence, taskRiskEvidenceLimit),
			AgentCheck: "Inspect the warned files or refresh the index before trusting missing context.",
			Count:      len(evidence),
		})
	}
	facts := recentTaskCheckpointFacts(db, repoRoot, taskRiskFactLimit)
	cards = append(cards, taskRiskCardsFromCheckpointFacts(query, predicted, facts)...)
	if len(cards) > taskRiskCardLimit {
		return cards[:taskRiskCardLimit]
	}
	return cards
}

func recentTaskCheckpointFacts(db *store.DB, repoRoot string, limit int) []store.TaskCheckpointFact {
	if db == nil || strings.TrimSpace(repoRoot) == "" {
		return nil
	}
	var repoID string
	err := db.QueryRow("SELECT id FROM repos WHERE root_path = ?", repoRoot).Scan(&repoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return nil
	}
	facts, err := db.ListRecentTaskCheckpointFacts(repoID, limit)
	if err != nil {
		return nil
	}
	return facts
}

func taskRiskCardsFromCheckpointFacts(query string, predicted taskPredictedContext, facts []store.TaskCheckpointFact) []taskRiskCard {
	var testMissEvidence []string
	var criticalMissEvidence []string
	var noiseEvidence []string
	var validationEvidence []string
	queryTerms := taskFreshnessQueryTerms(query)
	predictedWeak := taskRiskPredictedContextWeak(predicted)
	for _, fact := range facts {
		feedback := parseTaskCheckpointFeedbackJSON(fact.FeedbackJSON)
		learnings := parseTaskCheckpointLearningsJSON(fact.LearningsJSON)
		queryMatchedFact := predictedWeak && taskRiskFactMatchesQuery(queryTerms, feedback, learnings)
		for _, path := range feedback.CriticalMissed {
			if !taskRiskPathMatchesPredicted(path, predicted) && !queryMatchedFact {
				continue
			}
			evidence := taskRiskFactPathEvidence(fact, "missed", path)
			if looksLikeTestPath(path) {
				testMissEvidence = appendUniqueString(testMissEvidence, evidence)
			} else {
				criticalMissEvidence = appendUniqueString(criticalMissEvidence, evidence)
			}
		}
		for _, path := range feedback.DistractingIncluded {
			if path == "" {
				continue
			}
			noiseEvidence = appendUniqueString(noiseEvidence, taskRiskFactPathEvidence(fact, "called distracting", path))
		}
		for _, learning := range learnings {
			switch strings.ToLower(strings.TrimSpace(learning.LearningType)) {
			case "validation_gap":
				validationEvidence = appendUniqueString(validationEvidence, taskRiskFactLearningEvidence(fact, learning))
			case "noise", "risk_false_positive":
				noiseEvidence = appendUniqueString(noiseEvidence, taskRiskFactLearningEvidence(fact, learning))
			case "miss", "substrate_gap":
				if taskRiskLearningMatchesPredicted(learning, predicted) || (predictedWeak && taskRiskLearningMatchesQuery(queryTerms, learning)) {
					criticalMissEvidence = appendUniqueString(criticalMissEvidence, taskRiskFactLearningEvidence(fact, learning))
				}
			}
		}
	}
	var cards []taskRiskCard
	if len(testMissEvidence) > 0 {
		cards = appendTaskRiskCard(cards, taskRiskCard{
			ID:         "prior-test-miss",
			Title:      "Prior checkpoint missed a related test",
			Severity:   "medium",
			Source:     "checkpoint_fact",
			Evidence:   firstStrings(testMissEvidence, taskRiskEvidenceLimit),
			AgentCheck: "Search same-package and same-stem tests before editing.",
			Count:      len(testMissEvidence),
		})
	}
	if len(criticalMissEvidence) > 0 {
		cards = appendTaskRiskCard(cards, taskRiskCard{
			ID:         "prior-critical-miss",
			Title:      "Prior checkpoint recorded a critical miss in a related area",
			Severity:   "medium",
			Source:     "checkpoint_fact",
			Evidence:   firstStrings(criticalMissEvidence, taskRiskEvidenceLimit),
			AgentCheck: "Inspect the related area before assuming the initial pack is complete.",
			Count:      len(criticalMissEvidence),
		})
	}
	if len(noiseEvidence) > 0 {
		cards = appendTaskRiskCard(cards, taskRiskCard{
			ID:         "prior-noise",
			Title:      "Prior checkpoint recorded distracting context",
			Severity:   "low",
			Source:     "checkpoint_fact",
			Evidence:   firstStrings(noiseEvidence, taskRiskEvidenceLimit),
			AgentCheck: "Keep that family as reference-only unless this task verifies it.",
			Count:      len(noiseEvidence),
		})
	}
	if len(validationEvidence) > 0 {
		cards = appendTaskRiskCard(cards, taskRiskCard{
			ID:         "validation-gap",
			Title:      "Prior checkpoint recorded a validation gap",
			Severity:   "medium",
			Source:     "checkpoint_fact",
			Evidence:   firstStrings(validationEvidence, taskRiskEvidenceLimit),
			AgentCheck: "Identify the first validation command or eval artifact before implementation scope expands.",
			Count:      len(validationEvidence),
		})
	}
	return cards
}

func appendTaskRiskCard(cards []taskRiskCard, card taskRiskCard) []taskRiskCard {
	if strings.TrimSpace(card.ID) == "" || strings.TrimSpace(card.Title) == "" {
		return cards
	}
	for _, existing := range cards {
		if strings.EqualFold(existing.ID, card.ID) {
			return cards
		}
	}
	return append(cards, card)
}

func parseTaskCheckpointFeedbackJSON(text string) taskPredictedContextFeedback {
	var feedback taskPredictedContextFeedback
	_ = json.Unmarshal([]byte(strings.TrimSpace(text)), &feedback)
	return feedback
}

func parseTaskCheckpointLearningsJSON(text string) []taskCheckpointLearning {
	var learnings []taskCheckpointLearning
	_ = json.Unmarshal([]byte(strings.TrimSpace(text)), &learnings)
	return learnings
}

func taskRiskPathMatchesPredicted(path string, predicted taskPredictedContext) bool {
	path = normalizeSinglePath(path)
	if path == "" {
		return false
	}
	for _, current := range taskRiskPredictedPaths(predicted) {
		if taskRiskPathsRelated(path, current) {
			return true
		}
	}
	return false
}

func taskRiskLearningMatchesPredicted(learning taskCheckpointLearning, predicted taskPredictedContext) bool {
	for _, ref := range learning.EvidenceRefs {
		if taskRiskPathMatchesPredicted(ref, predicted) {
			return true
		}
	}
	return taskRiskPathMatchesPredicted(learning.AppliesTo, predicted)
}

func taskRiskPredictedContextWeak(predicted taskPredictedContext) bool {
	return len(predicted.PrimaryFiles) == 0 || len(taskRiskPredictedPaths(predicted)) == 0
}

func taskRiskFactMatchesQuery(queryTerms []string, feedback taskPredictedContextFeedback, learnings []taskCheckpointLearning) bool {
	if len(queryTerms) == 0 {
		return false
	}
	for _, path := range feedback.CriticalMissed {
		if taskRiskTextMatchesQuery(queryTerms, path) {
			return true
		}
	}
	for _, path := range feedback.DistractingIncluded {
		if taskRiskTextMatchesQuery(queryTerms, path) {
			return true
		}
	}
	for _, learning := range learnings {
		if taskRiskLearningMatchesQuery(queryTerms, learning) {
			return true
		}
	}
	return false
}

func taskRiskLearningMatchesQuery(queryTerms []string, learning taskCheckpointLearning) bool {
	if len(queryTerms) == 0 {
		return false
	}
	parts := []string{
		learning.LearningType,
		learning.Summary,
		learning.AppliesTo,
	}
	parts = append(parts, learning.EvidenceRefs...)
	return taskRiskTextMatchesQuery(queryTerms, strings.Join(parts, " "))
}

func taskRiskTextMatchesQuery(queryTerms []string, text string) bool {
	text = strings.ToLower(filepath.ToSlash(text))
	if strings.TrimSpace(text) == "" {
		return false
	}
	needed := 1
	if len(queryTerms) >= 2 {
		needed = 2
	}
	matches := 0
	for _, term := range queryTerms {
		if strings.Contains(text, strings.ToLower(term)) {
			matches++
			if matches >= needed {
				return true
			}
		}
	}
	return false
}

func taskRiskPredictedPaths(predicted taskPredictedContext) []string {
	paths := appendNormalizedUnique(nil, predictedFilePaths(predicted.PrimaryFiles)...)
	paths = appendNormalizedUnique(paths, predictedFilePaths(predicted.Tests)...)
	paths = appendNormalizedUnique(paths, predictedFilePaths(predicted.DocsPlansConfig)...)
	paths = appendNormalizedUnique(paths, predictedFilePaths(predicted.SupportingContext)...)
	return paths
}

func taskRiskPathsRelated(left, right string) bool {
	left = normalizeSinglePath(left)
	right = normalizeSinglePath(right)
	if left == "" || right == "" {
		return false
	}
	if containsPath([]string{left}, right) || containsPath([]string{right}, left) {
		return true
	}
	if pathArea(left) != "" && pathArea(left) == pathArea(right) {
		return true
	}
	if filepath.Dir(left) == filepath.Dir(right) {
		return true
	}
	return testCompanionStem(left) != "" && testCompanionStem(left) == testCompanionStem(right)
}

func taskRiskFactPathEvidence(fact store.TaskCheckpointFact, verb, path string) string {
	return fmt.Sprintf("task %s checkpoint %s %s `%s`", fact.TaskID, fact.CheckpointID, verb, normalizeSinglePath(path))
}

func taskRiskFactLearningEvidence(fact store.TaskCheckpointFact, learning taskCheckpointLearning) string {
	summary := strings.TrimSpace(learning.Summary)
	if summary == "" {
		summary = strings.TrimSpace(learning.LearningType)
	}
	if summary == "" {
		return ""
	}
	return fmt.Sprintf("task %s checkpoint %s learned: %s", fact.TaskID, fact.CheckpointID, summary)
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
	fmt.Fprintln(&b, "## Series")
	fmt.Fprintf(&b, "%s\n\n", defaultTaskSeries(manifest.Series))
	fmt.Fprintln(&b, "## Profile")
	fmt.Fprintf(&b, "%s\n\n", defaultTaskProfile(manifest.Profile))
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
			fmt.Fprintf(&b, "- %s: %s", slice.ID, slice.Title)
			if slice.ParentID != "" {
				fmt.Fprintf(&b, " (iteration of %s", slice.ParentID)
				if slice.Reason != "" {
					fmt.Fprintf(&b, ", reason: %s", slice.Reason)
				}
				fmt.Fprint(&b, ")")
			}
			var state []string
			if slice.Stage != "" {
				state = append(state, "stage: "+slice.Stage)
			}
			if slice.Decision != "" {
				state = append(state, "decision: "+slice.Decision)
			}
			if slice.LatestCheckpoint != "" {
				state = append(state, "checkpoint: "+slice.LatestCheckpoint)
			}
			if slice.LatestCheckpointID != "" {
				state = append(state, "checkpoint_id: "+slice.LatestCheckpointID)
			}
			if len(state) > 0 {
				fmt.Fprintf(&b, " [%s]", strings.Join(state, ", "))
			}
			fmt.Fprintf(&b, ". Plan: `%s`. Result: `%s`.\n", slice.Plan, slice.Result)
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
	writeTaskRiskCards(&b, manifest.RiskCards)
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
	confidence := manifest.Confidence
	if taskProfileIsGreenfield(manifest) {
		confidence.AgentInstruction = "Use the evidence to define the first bounded planning artifact, evaluation signal, and next-slice decision before implementation scope expands."
	}
	writeConfidenceSummary(&b, confidence)
	first := firstTaskSliceArtifact(manifest.Artifacts)
	fmt.Fprintln(&b, "## Suggested Starting Slice")
	if first.Plan != "" {
		if taskProfileIsGreenfield(manifest) {
			fmt.Fprintf(&b, "Use `%s` as the first bounded planning slice in this task thread. Refine claims, interfaces, evaluation shape, and unknowns before committing to implementation scope.\n", first.Plan)
		} else {
			fmt.Fprintf(&b, "Use `%s` as the first bounded plan in this task thread. Refine it before editing if primary files, tests, or integration points look incomplete.\n", first.Plan)
		}
	} else {
		if taskProfileIsGreenfield(manifest) {
			fmt.Fprintln(&b, "Start by refining the first bounded planning slice in this task thread before committing to implementation scope.")
		} else {
			fmt.Fprintln(&b, "Start by refining the first bounded plan in this task thread before editing.")
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Agent Preflight Checklist")
	if taskProfileIsGreenfield(manifest) {
		fmt.Fprintln(&b, "- [ ] Treat predicted files as evidence, not required edit targets.")
		fmt.Fprintln(&b, "- [ ] Identify the claim, interface, adapter, data model, or evaluation shape this slice should settle.")
		fmt.Fprintln(&b, "- [ ] Record assumptions, known unknowns, and the chosen next artifact before widening scope.")
	} else {
		fmt.Fprintln(&b, "- [ ] Verify the likely primary files against the repo before editing.")
		fmt.Fprintln(&b, "- [ ] Search for same-package or same-command tests if test confidence is not high.")
		fmt.Fprintln(&b, "- [ ] Check receipt-touched related files before assuming the pack is complete.")
	}
	if first.Result != "" {
		if taskProfileIsGreenfield(manifest) {
			fmt.Fprintf(&b, "- [ ] Record evidence reviewed, decisions made, open questions, and next-slice recommendation in `%s` or `ds task checkpoint`.\n", first.Result)
		} else {
			fmt.Fprintf(&b, "- [ ] Record files actually read, edited, tests run, misses, and noise in `%s` or `ds task checkpoint`.\n", first.Result)
		}
	} else {
		if taskProfileIsGreenfield(manifest) {
			fmt.Fprintln(&b, "- [ ] Record evidence reviewed, decisions made, open questions, and next-slice recommendation in the slice result or `ds task checkpoint`.")
		} else {
			fmt.Fprintln(&b, "- [ ] Record files actually read, edited, tests run, misses, and noise in the slice result or `ds task checkpoint`.")
		}
	}
	return b.String()
}

func renderTaskSlicePlan(manifest taskManifest, slice taskSliceArtifact) string {
	if taskProfileIsGreenfield(manifest) {
		return renderTaskGreenfieldSlicePlan(manifest, slice)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Task %s %s Plan\n\n", manifest.TaskID, slice.ID)
	fmt.Fprintln(&b, "## Goal")
	fmt.Fprintf(&b, "%s\n\n", slice.Title)
	fmt.Fprintln(&b, "## Description")
	fmt.Fprintf(&b, "Create a bounded implementation slice for `%s`. This plan is grounded by the task index preflight, but it is not authoritative; confirm predicted files and tests before making edits.\n", manifest.Query)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Resources")
	if manifest.Artifacts.Index != "" {
		fmt.Fprintf(&b, "- `%s`\n", manifest.Artifacts.Index)
	}
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
	writeTaskRiskCardBullets(&b, manifest.RiskCards)
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

func renderTaskGreenfieldSlicePlan(manifest taskManifest, slice taskSliceArtifact) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Task %s %s Plan\n\n", manifest.TaskID, slice.ID)
	fmt.Fprintln(&b, "## Goal")
	fmt.Fprintf(&b, "%s\n\n", slice.Title)
	fmt.Fprintln(&b, "## Description")
	fmt.Fprintf(&b, "Create a bounded planning slice for `%s`. Use the task index as evidence, then settle the claim, interface, evaluation shape, and known unknowns needed before implementation scope expands.\n", manifest.Query)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Resources")
	if manifest.Artifacts.Index != "" {
		fmt.Fprintf(&b, "- `%s`\n", manifest.Artifacts.Index)
	}
	if slice.Result != "" {
		fmt.Fprintf(&b, "- `%s`\n", slice.Result)
	}
	fmt.Fprintln(&b, "- `task.json`")
	for _, file := range firstPredictedResources(manifest.Predicted) {
		fmt.Fprintf(&b, "- `%s`\n", file)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Starting Context")
	writeGreenfieldEvidenceFiles(&b, "Evidence to Review", manifest.Predicted.PrimaryFiles, "No likely primary files were found; identify the first artifact from the repo and task goal.")
	writeGreenfieldEvidenceFiles(&b, "Test or Evaluation Signals", manifest.Predicted.Tests, "No likely tests were found; define the first useful validation signal.")
	fmt.Fprintln(&b, "## Expected Change Surface")
	fmt.Fprintln(&b, "- Planning artifacts, acceptance checks, interface notes, eval cards, or test design.")
	fmt.Fprintln(&b, "- Implementation code only if the slice explicitly narrows to one low-risk first artifact.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Out-of-Scope Areas")
	fmt.Fprintln(&b, "- Treating a greenfield planning slice as permission to implement the full thread.")
	fmt.Fprintln(&b, "- Broad retrieval or pack-ranking changes unless the slice is explicitly about DevSpecs itself.")
	fmt.Fprintln(&b, "- Assuming the generated context is complete without recording verification.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Risks")
	for _, unknown := range taskKnownUnknowns(manifest) {
		fmt.Fprintf(&b, "- %s\n", unknown)
	}
	writeTaskRiskCardBullets(&b, manifest.RiskCards)
	if len(manifest.Predicted.NoiseRisks) > 0 {
		fmt.Fprintln(&b, "- Initial pack includes downgraded noise candidates; keep them as reference only unless verification supports them.")
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Success Criteria")
	fmt.Fprintln(&b, "- [ ] The slice states the product or engineering claim being settled.")
	fmt.Fprintln(&b, "- [ ] Interfaces, adapters, data model, or evaluation shape are named at the right level of detail.")
	fmt.Fprintln(&b, "- [ ] Known unknowns and assumptions are recorded.")
	fmt.Fprintln(&b, "- [ ] The next slice recommendation is promote, improve, rework, rollback, or block.")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Tasks")
	fmt.Fprintln(&b, "- [ ] Review the task index and any likely evidence files.")
	fmt.Fprintln(&b, "- [ ] Define the first claim, interface, adapter, data model, or evaluation target.")
	fmt.Fprintln(&b, "- [ ] Draft the smallest useful planning artifact for that target.")
	fmt.Fprintln(&b, "- [ ] Decide whether the next slice should implement, evaluate, improve, rework, rollback, or block.")
	if slice.Result != "" {
		fmt.Fprintf(&b, "- [ ] Update `%s` or run `ds task checkpoint`.\n", slice.Result)
	} else {
		fmt.Fprintln(&b, "- [ ] Update the slice result or run `ds task checkpoint`.")
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Decision Gates")
	fmt.Fprintln(&b, "- Promote: the planning slice gives the next agent a bounded, useful unit of work.")
	fmt.Fprintln(&b, "- Improve: the slice is directionally useful but needs another planning iteration.")
	fmt.Fprintln(&b, "- Rework: the plan chose the wrong claim, artifact, or evaluation target.")
	fmt.Fprintln(&b, "- Rollback: the scaffold added noise or false confidence.")
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
	if manifest.Artifacts.Index != "" {
		fmt.Fprintf(&b, "- `%s`\n", manifest.Artifacts.Index)
	}
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

func writeTaskRiskCards(b *strings.Builder, cards []taskRiskCard) {
	if len(cards) == 0 {
		return
	}
	fmt.Fprintln(b, "## Risk Cards")
	fmt.Fprintln(b, "Evidence-backed checks to run before trusting the initial task context. These are not required edit targets.")
	fmt.Fprintln(b)
	for _, card := range cards {
		fmt.Fprintf(b, "- %s [%s, %s]\n", card.Title, card.Severity, card.Source)
		if card.AgentCheck != "" {
			fmt.Fprintf(b, "  Agent check: %s\n", card.AgentCheck)
		}
		if len(card.Evidence) > 0 {
			fmt.Fprintf(b, "  Evidence: %s\n", strings.Join(firstStrings(card.Evidence, taskRiskEvidenceLimit), "; "))
		}
	}
	fmt.Fprintln(b)
}

func writeTaskRiskCardBullets(b *strings.Builder, cards []taskRiskCard) {
	for _, card := range cards {
		if card.Title == "" {
			continue
		}
		fmt.Fprintf(b, "- %s: %s", card.Title, card.AgentCheck)
		if len(card.Evidence) > 0 {
			fmt.Fprintf(b, " Evidence: %s.", strings.Join(firstStrings(card.Evidence, 2), "; "))
		}
		fmt.Fprintln(b)
	}
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

func writeGreenfieldEvidenceFiles(b *strings.Builder, title string, files []taskPredictedFile, empty string) {
	fmt.Fprintf(b, "### %s\n", title)
	if len(files) == 0 {
		fmt.Fprintf(b, "- %s\n\n", empty)
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
		if taskProfileIsGreenfield(manifest) {
			out = append(out, "Pack completeness is not high; verify the working set before committing to implementation scope.")
		} else {
			out = append(out, "Pack completeness is not high; verify the working set before editing.")
		}
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
	checkpointID := taskCheckpointID(selectedSlice.ID, opts.Stage, now)
	checkpointPath := filepath.Join(checkpointDir, checkpointStem+".md")
	checkpointJSONPath := filepath.Join(checkpointDir, checkpointStem+".json")
	record := buildTaskCheckpointRecord(manifest, opts, selectedSlice, checkpointID, now, repoRoot)
	jsonRel := taskRelativePath(workspace, checkpointJSONPath)
	body := renderTaskCheckpoint(manifest, selectedSlice, opts, now, checkpointID, jsonRel)
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
	applyTaskTargetState(&manifest, selectedSlice.ID, opts.Stage, opts.Decision, now)
	applyTaskTargetCheckpointRefs(&manifest, selectedSlice.ID, checkpointID, taskRelativePath(workspace, checkpointPath), jsonRel)
	if err := writeTaskManifest(filepath.Join(workspace, taskManifestFilename), manifest); err != nil {
		return err
	}
	indexPath := filepath.Join(workspace, manifest.Artifacts.Index)
	if err := os.WriteFile(indexPath, []byte(renderTaskIndex(manifest)), 0o644); err != nil {
		return fmt.Errorf("write task index: %w", err)
	}

	var indexed []string
	factIndexed := false
	if opts.Index {
		indexed, err = captureTaskArtifacts(cmd, repoRoot, []taskCaptureRequest{
			{
				Path:   indexPath,
				Title:  "Task " + taskID + " preflight",
				Status: taskArtifactStatus(manifest.Status, manifest.Decision),
			},
			{
				Path:   checkpointPath,
				Title:  "Task " + taskID + " checkpoint " + opts.Stage,
				Status: taskArtifactStatus(opts.Stage, opts.Decision),
			},
		})
		if err != nil {
			return err
		}
		if err := indexTaskCheckpointFact(repoRoot, manifest, record, checkpointPath, checkpointJSONPath, workspace, now); err != nil {
			return err
		}
		factIndexed = true
	}

	out := taskCheckpointOutput{
		TaskID:             taskID,
		Series:             defaultTaskSeries(manifest.Series),
		Slice:              selectedSlice.ID,
		CheckpointID:       checkpointID,
		Stage:              opts.Stage,
		Decision:           opts.Decision,
		CheckpointPath:     checkpointPath,
		CheckpointJSONPath: checkpointJSONPath,
		ResultPath:         resultPath,
		IndexedPaths:       indexed,
		LearningCount:      len(record.Learnings),
		FactIndexed:        factIndexed,
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
	opts.Learnings = normalizeList(opts.Learnings)
	opts.NextTarget = strings.TrimSpace(opts.NextTarget)
	opts.NextDecision = strings.TrimSpace(opts.NextDecision)
	if opts.GitDiffMax <= 0 {
		opts.GitDiffMax = 12000
	}
	if opts.TestMax <= 0 {
		opts.TestMax = 12000
	}
	return opts
}

func taskCheckpointID(target, stage string, now time.Time) string {
	parts := []string{
		"cp",
		now.Format("20060102T150405Z"),
		sanitizeTaskFilename(target),
		sanitizeTaskFilename(stage),
	}
	var kept []string
	for _, part := range parts {
		part = strings.Trim(part, "-")
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "_")
}

func taskCheckpointParentSlice(slice taskSliceArtifact) string {
	if strings.TrimSpace(slice.ParentID) != "" {
		return slice.ParentID
	}
	return slice.ID
}

func taskCheckpointIteration(slice taskSliceArtifact) string {
	if strings.TrimSpace(slice.ParentID) != "" || strings.Contains(slice.ID, "-") {
		return slice.ID
	}
	return ""
}

func applyTaskTargetState(manifest *taskManifest, targetID, stage, decision string, now time.Time) {
	stage = strings.TrimSpace(stage)
	decision = strings.TrimSpace(decision)
	timestamp := now.Format(time.RFC3339)
	if targetID == "" || isTaskSeriesTarget(*manifest, targetID) {
		if stage != "" {
			manifest.Status = stage
		}
		manifest.Decision = decision
		manifest.UpdatedAt = timestamp
		return
	}
	for i := range manifest.Artifacts.Slices {
		if !strings.EqualFold(manifest.Artifacts.Slices[i].ID, targetID) {
			continue
		}
		if stage != "" {
			manifest.Artifacts.Slices[i].Stage = stage
		}
		manifest.Artifacts.Slices[i].Decision = decision
		manifest.Artifacts.Slices[i].UpdatedAt = timestamp
		manifest.UpdatedAt = timestamp
		return
	}
	manifest.UpdatedAt = timestamp
}

func applyTaskTargetCheckpointRefs(manifest *taskManifest, targetID, checkpointID, checkpoint, checkpointJSON string) {
	checkpointID = strings.TrimSpace(checkpointID)
	checkpoint = strings.TrimSpace(checkpoint)
	checkpointJSON = strings.TrimSpace(checkpointJSON)
	if checkpointID == "" && checkpoint == "" && checkpointJSON == "" {
		return
	}
	if targetID == "" || isTaskSeriesTarget(*manifest, targetID) {
		manifest.LatestCheckpointID = checkpointID
		manifest.LatestCheckpoint = checkpoint
		manifest.LatestCheckpointJSON = checkpointJSON
		return
	}
	for i := range manifest.Artifacts.Slices {
		if !strings.EqualFold(manifest.Artifacts.Slices[i].ID, targetID) {
			continue
		}
		manifest.Artifacts.Slices[i].LatestCheckpointID = checkpointID
		manifest.Artifacts.Slices[i].LatestCheckpoint = checkpoint
		manifest.Artifacts.Slices[i].LatestCheckpointJSON = checkpointJSON
		return
	}
}

func isTaskSeriesTarget(manifest taskManifest, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	series := defaultTaskSeries(manifest.Series)
	return strings.EqualFold(target, series) ||
		strings.EqualFold(target, series+"00") ||
		strings.EqualFold(target, manifest.Artifacts.Index)
}

func taskStageForDecision(decision string) string {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "promote", "complete", "completed":
		return "completed"
	case "block", "blocked":
		return "blocked"
	case "split":
		return "split"
	case "supersede", "superseded":
		return "superseded"
	case "cancel", "cancelled":
		return "cancelled"
	case "rollback":
		return "rolled_back"
	default:
		return ""
	}
}

func taskStateForTarget(manifest taskManifest, targetID string) (string, string) {
	if targetID == "" || isTaskSeriesTarget(manifest, targetID) {
		return manifest.Status, manifest.Decision
	}
	for _, slice := range manifest.Artifacts.Slices {
		if strings.EqualFold(slice.ID, targetID) {
			return slice.Stage, slice.Decision
		}
	}
	return "", ""
}

func buildTaskCheckpointRecord(manifest taskManifest, opts taskCheckpointOptions, slice taskSliceArtifact, checkpointID string, now time.Time, repoRoot string) taskCheckpointRecord {
	goal := strings.TrimSpace(opts.Goal)
	if goal == "" {
		goal = fmt.Sprintf("Record progress for `%s`.", manifest.Query)
	}
	description := strings.TrimSpace(opts.Description)
	if description == "" {
		description = "Checkpoint generated by `ds task checkpoint`."
	}
	actual := taskCheckpointActualContext{
		FilesRead:   opts.FilesRead,
		FilesEdited: opts.FilesEdited,
		TestsRead:   opts.TestsRead,
		TestsRun:    opts.TestsRun,
	}
	feedback := taskPredictedContextFeedback{
		RelevantFound:       taskCheckpointRelevantFound(manifest, actual),
		CriticalMissed:      opts.MissedFiles,
		DistractingIncluded: opts.NoiseFiles,
	}
	record := taskCheckpointRecord{
		SchemaVersion:            2,
		CheckpointID:             checkpointID,
		TaskID:                   manifest.TaskID,
		Target:                   slice.ID,
		Series:                   defaultTaskSeries(manifest.Series),
		Query:                    manifest.Query,
		Slice:                    slice.ID,
		SliceTitle:               slice.Title,
		ParentSlice:              taskCheckpointParentSlice(slice),
		Iteration:                taskCheckpointIteration(slice),
		Stage:                    opts.Stage,
		Decision:                 opts.Decision,
		CreatedAt:                now.Format(time.RFC3339),
		Goal:                     goal,
		Description:              description,
		Note:                     strings.TrimSpace(opts.Note),
		Resources:                appendUniqueValues(taskCheckpointResourcePaths(manifest, slice), opts.Resources...),
		FilesRead:                opts.FilesRead,
		FilesEdited:              opts.FilesEdited,
		TestsRead:                opts.TestsRead,
		TestsRun:                 opts.TestsRun,
		MissedFiles:              opts.MissedFiles,
		NoiseFiles:               opts.NoiseFiles,
		Tasks:                    opts.Tasks,
		ActualContext:            actual,
		PredictedContextFeedback: feedback,
		Learnings:                parseTaskCheckpointLearnings(opts.Learnings),
		Next: taskCheckpointNextRecommendation{
			RecommendedTarget:   opts.NextTarget,
			RecommendedDecision: opts.NextDecision,
		},
	}
	record.Evidence.PlanRefs = appendUniqueValues(nil, record.Resources...)
	if opts.GitDiff {
		gitDiff := collectTaskGitDiffEvidence(repoRoot, opts.GitDiffMax)
		record.Evidence.GitDiff = &gitDiff
		record.Evidence.GitDiffPaths = appendNormalizedUnique(record.Evidence.GitDiffPaths, gitDiff.ChangedFiles...)
	}
	if opts.TestOutput {
		for _, command := range opts.TestsRun {
			record.Evidence.TestCommands = append(record.Evidence.TestCommands, runTaskCommandEvidence(repoRoot, command, opts.TestMax))
		}
	}
	return record
}

func taskCheckpointRelevantFound(manifest taskManifest, actual taskCheckpointActualContext) []string {
	predicted := appendNormalizedUnique(nil, predictedFilePaths(manifest.Predicted.PrimaryFiles)...)
	predicted = appendNormalizedUnique(predicted, predictedFilePaths(manifest.Predicted.Tests)...)
	predicted = appendNormalizedUnique(predicted, predictedFilePaths(manifest.Predicted.DocsPlansConfig)...)
	predicted = appendNormalizedUnique(predicted, predictedFilePaths(manifest.Predicted.SupportingContext)...)
	actualPaths := appendNormalizedUnique(nil, actual.FilesRead...)
	actualPaths = appendNormalizedUnique(actualPaths, actual.FilesEdited...)
	actualPaths = appendNormalizedUnique(actualPaths, actual.TestsRead...)
	var found []string
	for _, path := range actualPaths {
		if containsPath(predicted, path) {
			found = appendNormalizedUnique(found, path)
		}
	}
	return found
}

func parseTaskCheckpointLearnings(values []string) []taskCheckpointLearning {
	var out []taskCheckpointLearning
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.Split(raw, "|")
		var learning taskCheckpointLearning
		if len(parts) > 1 {
			learning.LearningType = strings.TrimSpace(parts[0])
			learning.Summary = strings.TrimSpace(parts[1])
			if len(parts) > 2 {
				learning.Confidence = strings.TrimSpace(parts[2])
			}
			if len(parts) > 3 {
				learning.AppliesTo = strings.TrimSpace(parts[3])
			}
			if len(parts) > 4 {
				learning.EvidenceRefs = normalizePathList(strings.Split(parts[4], ","))
			}
		} else if typ, summary, ok := strings.Cut(raw, ":"); ok {
			learning.LearningType = strings.TrimSpace(typ)
			learning.Summary = strings.TrimSpace(summary)
		} else {
			learning.LearningType = "workflow_friction"
			learning.Summary = raw
		}
		if learning.LearningType == "" {
			learning.LearningType = "workflow_friction"
		}
		if learning.Summary == "" {
			continue
		}
		if learning.Confidence == "" {
			learning.Confidence = "medium"
		}
		out = append(out, learning)
	}
	return out
}

func normalizeTaskCheckpointRecord(record *taskCheckpointRecord) {
	if record == nil {
		return
	}
	if record.Target == "" {
		record.Target = record.Slice
	}
	if record.Slice == "" {
		record.Slice = record.Target
	}
	if record.ParentSlice == "" {
		record.ParentSlice = record.Slice
	}
	if record.CheckpointID == "" {
		record.CheckpointID = taskCheckpointID(firstNonEmptyTaskString(record.Target, record.Slice), record.Stage, parseTaskCheckpointCreatedAt(record.CreatedAt))
	}
	if len(record.ActualContext.FilesRead)+len(record.ActualContext.FilesEdited)+len(record.ActualContext.TestsRead)+len(record.ActualContext.TestsRun) == 0 {
		record.ActualContext = taskCheckpointActualContext{
			FilesRead:   record.FilesRead,
			FilesEdited: record.FilesEdited,
			TestsRead:   record.TestsRead,
			TestsRun:    record.TestsRun,
		}
	}
	record.FilesRead = appendNormalizedUnique(record.FilesRead, record.ActualContext.FilesRead...)
	record.FilesEdited = appendNormalizedUnique(record.FilesEdited, record.ActualContext.FilesEdited...)
	record.TestsRead = appendNormalizedUnique(record.TestsRead, record.ActualContext.TestsRead...)
	record.TestsRun = appendUniqueValues(record.TestsRun, record.ActualContext.TestsRun...)
	if len(record.PredictedContextFeedback.CriticalMissed)+len(record.PredictedContextFeedback.DistractingIncluded) == 0 {
		record.PredictedContextFeedback.CriticalMissed = record.MissedFiles
		record.PredictedContextFeedback.DistractingIncluded = record.NoiseFiles
	}
	record.MissedFiles = appendNormalizedUnique(record.MissedFiles, record.PredictedContextFeedback.CriticalMissed...)
	record.NoiseFiles = appendNormalizedUnique(record.NoiseFiles, record.PredictedContextFeedback.DistractingIncluded...)
	if record.Evidence.GitDiff != nil {
		record.Evidence.GitDiffPaths = appendNormalizedUnique(record.Evidence.GitDiffPaths, record.Evidence.GitDiff.ChangedFiles...)
	}
	if len(record.Evidence.PlanRefs) == 0 {
		record.Evidence.PlanRefs = appendUniqueValues(nil, record.Resources...)
	}
}

func parseTaskCheckpointCreatedAt(createdAt string) time.Time {
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(createdAt)); err == nil {
		return parsed
	}
	return time.Unix(0, 0).UTC()
}

func indexTaskCheckpointFact(repoRoot string, manifest taskManifest, record taskCheckpointRecord, checkpointPath, checkpointJSONPath, workspace string, now time.Time) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	repoID, err := ensureTaskFactRepo(db, repoRoot, now.Format(time.RFC3339))
	if err != nil {
		return err
	}
	actualJSON, err := marshalTaskFactJSON(record.ActualContext, "{}")
	if err != nil {
		return err
	}
	feedbackJSON, err := marshalTaskFactJSON(record.PredictedContextFeedback, "{}")
	if err != nil {
		return err
	}
	evidenceJSON, err := marshalTaskFactJSON(record.Evidence, "{}")
	if err != nil {
		return err
	}
	learningsJSON, err := marshalTaskFactJSON(record.Learnings, "[]")
	if err != nil {
		return err
	}
	nextJSON, err := marshalTaskFactJSON(record.Next, "{}")
	if err != nil {
		return err
	}
	return db.UpsertTaskCheckpointFact(store.TaskCheckpointFact{
		RepoID:             repoID,
		TaskID:             manifest.TaskID,
		CheckpointID:       record.CheckpointID,
		Target:             firstNonEmptyTaskString(record.Target, record.Slice),
		Series:             defaultTaskSeries(record.Series),
		Stage:              record.Stage,
		Decision:           record.Decision,
		CheckpointPath:     taskRelativePath(workspace, checkpointPath),
		CheckpointJSONPath: taskRelativePath(workspace, checkpointJSONPath),
		CreatedAt:          record.CreatedAt,
		ActualContextJSON:  actualJSON,
		FeedbackJSON:       feedbackJSON,
		EvidenceJSON:       evidenceJSON,
		LearningsJSON:      learningsJSON,
		NextJSON:           nextJSON,
		IndexedAt:          now.Format(time.RFC3339),
	})
}

func ensureTaskFactRepo(db *store.DB, repoRoot, now string) (string, error) {
	var repoID string
	err := db.QueryRow("SELECT id FROM repos WHERE root_path = ?", repoRoot).Scan(&repoID)
	if err == nil {
		return repoID, nil
	}
	ids := idgen.NewFactory()
	repoID = ids.NewWithPrefix("repo_")
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", repoID, repoRoot, now, now); err != nil {
		return "", err
	}
	return repoID, nil
}

func marshalTaskFactJSON(value any, fallback string) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	text := string(data)
	if text == "null" || text == "" {
		return fallback, nil
	}
	return text, nil
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
		evidence.ChangedFiles = appendTaskGitChangedFiles(evidence.ChangedFiles, output)
	}
	return evidence
}

func appendTaskGitChangedFiles(paths []string, output string) []string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !isTaskGitChangedFileLine(line) {
			continue
		}
		paths = appendNormalizedUnique(paths, line)
	}
	return paths
}

func isTaskGitChangedFileLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	return !strings.HasPrefix(strings.ToLower(line), "warning:")
}

func runBoundedGitCommand(repoRoot string, maxBytes int, args ...string) (string, bool, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	text, truncated := boundedText(stdout.String(), maxBytes)
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(text)
		}
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

func renderTaskCheckpoint(manifest taskManifest, slice taskSliceArtifact, opts taskCheckpointOptions, now time.Time, checkpointID, checkpointJSONRel string) string {
	var b strings.Builder
	fmt.Fprintln(&b, "---")
	fmt.Fprintln(&b, "schema_version: 2")
	fmt.Fprintf(&b, "checkpoint_id: %s\n", yamlScalar(checkpointID))
	fmt.Fprintf(&b, "task_id: %s\n", yamlScalar(manifest.TaskID))
	fmt.Fprintf(&b, "series: %s\n", yamlScalar(defaultTaskSeries(manifest.Series)))
	if slice.ID != "" {
		fmt.Fprintf(&b, "target: %s\n", yamlScalar(slice.ID))
		fmt.Fprintf(&b, "slice: %s\n", yamlScalar(slice.ID))
		fmt.Fprintf(&b, "parent_slice: %s\n", yamlScalar(taskCheckpointParentSlice(slice)))
		if iteration := taskCheckpointIteration(slice); iteration != "" {
			fmt.Fprintf(&b, "iteration: %s\n", yamlScalar(iteration))
		}
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
	fmt.Fprintf(&b, "- Checkpoint ID: `%s`\n", checkpointID)
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
	fmt.Fprintf(out, "Series: %s\n", result.Series)
	fmt.Fprintf(out, "Profile: %s\n", result.Profile)
	fmt.Fprintf(out, "%s00: %s\n", defaultTaskSeries(result.Series), result.IndexPath)
	for _, slice := range result.Slices {
		fmt.Fprintf(out, "%s plan: %s\n", slice.ID, slice.PlanPath)
		fmt.Fprintf(out, "%s result: %s\n", slice.ID, slice.ResultPath)
	}
	if len(result.Slices) == 0 {
		series := defaultTaskSeries(result.Series)
		fmt.Fprintf(out, "%s01 plan: %s\n", series, result.FirstSlicePath)
		fmt.Fprintf(out, "%s01 result: %s\n", series, result.ResultPath)
	}
	fmt.Fprintf(out, "Confidence: primary=%s tests=%s completeness=%s noise=%s\n",
		confidence.PrimaryFileConfidence,
		confidence.TestCoverageConfidence,
		confidence.PackCompleteness,
		confidence.NoiseRisk,
	)
	if len(result.RiskCards) > 0 {
		fmt.Fprintf(out, "Risk cards: %d\n", len(result.RiskCards))
		for _, card := range firstTaskRiskCards(result.RiskCards, 3) {
			fmt.Fprintf(out, "  - %s: %s\n", card.Title, card.AgentCheck)
		}
	}
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

func firstTaskRiskCards(cards []taskRiskCard, limit int) []taskRiskCard {
	if limit <= 0 || len(cards) <= limit {
		return cards
	}
	return cards[:limit]
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
	normalizeTaskManifest(&manifest)
	return manifest, nil
}

func normalizeTaskManifest(manifest *taskManifest) {
	if strings.TrimSpace(manifest.Series) == "" {
		manifest.Series = manifest.Artifacts.Series
	}
	manifest.Series = defaultTaskSeries(manifest.Series)
	if strings.TrimSpace(manifest.Artifacts.Series) == "" {
		manifest.Artifacts.Series = manifest.Series
	}
	if strings.TrimSpace(manifest.Artifacts.Index) == "" {
		manifest.Artifacts.Index = taskSeriesIndexFilename(manifest.Series)
	}
	manifest.Profile = defaultTaskProfile(manifest.Profile)
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

func normalizeTaskSeries(series string) (string, error) {
	series = defaultTaskSeries(series)
	if len(series) > 6 {
		return "", fmt.Errorf("task series %q is too long; use 1-6 letters or digits", series)
	}
	for i, r := range series {
		if i == 0 && !unicode.IsLetter(r) {
			return "", fmt.Errorf("task series %q must start with a letter", series)
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return "", fmt.Errorf("task series contains unsupported character %q", r)
	}
	return series, nil
}

func normalizeTaskProfile(profile string) (string, error) {
	profile = strings.ToLower(strings.TrimSpace(profile))
	switch profile {
	case "", "default", "code", "code-change", "code_change", "implementation":
		return taskProfileCodeChange, nil
	case "greenfield", "planning", "plan":
		return taskProfileGreenfield, nil
	default:
		return "", fmt.Errorf("invalid task profile %q; valid values: %s, %s", profile, taskProfileCodeChange, taskProfileGreenfield)
	}
}

func defaultTaskProfile(profile string) string {
	normalized, err := normalizeTaskProfile(profile)
	if err != nil {
		return taskProfileCodeChange
	}
	return normalized
}

func taskProfileIsGreenfield(manifest taskManifest) bool {
	return defaultTaskProfile(manifest.Profile) == taskProfileGreenfield
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

type taskSyncArtifactCandidate struct {
	RelPath string
	Kind    string
	Title   string
	Status  string
}

func taskSyncCaptureRequests(workspace string, manifest taskManifest) []taskCaptureRequest {
	var requests []taskCaptureRequest
	seen := map[string]bool{}
	add := func(candidate taskSyncArtifactCandidate) {
		relPath := filepath.ToSlash(strings.TrimSpace(candidate.RelPath))
		if relPath == "" || seen[relPath] {
			return
		}
		path := filepath.Join(workspace, filepath.FromSlash(relPath))
		if _, err := os.Stat(path); err != nil {
			return
		}
		status := candidate.Status
		if status == "" || status == "unknown" {
			status = "implementing"
		}
		requests = append(requests, taskCaptureRequest{
			Path:   path,
			Title:  candidate.Title,
			Status: status,
		})
		seen[relPath] = true
	}

	series := defaultTaskSeries(manifest.Series)
	seriesStatus := taskArtifactStatus(manifest.Status, manifest.Decision)
	add(taskSyncArtifactCandidate{
		RelPath: manifest.Artifacts.Index,
		Kind:    "index",
		Title:   "Task " + manifest.TaskID + " preflight",
		Status:  seriesStatus,
	})
	for _, slice := range taskSyncSlices(manifest) {
		status := taskCaptureStatusForSlice(manifest, slice)
		id := slice.ID
		if id == "" {
			id = series + "01"
		}
		title := strings.TrimSpace(slice.Title)
		if title == "" {
			title = "slice"
		}
		add(taskSyncArtifactCandidate{
			RelPath: slice.Plan,
			Kind:    "plan",
			Title:   "Task " + manifest.TaskID + " " + id + " plan: " + title,
			Status:  status,
		})
		add(taskSyncArtifactCandidate{
			RelPath: slice.Result,
			Kind:    "result",
			Title:   "Task " + manifest.TaskID + " " + id + " result: " + title,
			Status:  status,
		})
	}
	return requests
}

func taskArtifactFreshnessWarnings(workspace string, manifest taskManifest) []taskArtifactFreshness {
	stateTime, stateText, ok := taskManifestFreshnessTime(manifest)
	if !ok {
		return nil
	}
	var warnings []taskArtifactFreshness
	seen := map[string]bool{}
	for _, candidate := range taskFreshnessCandidates(manifest) {
		relPath := filepath.ToSlash(strings.TrimSpace(candidate.RelPath))
		if relPath == "" || seen[relPath] {
			continue
		}
		seen[relPath] = true
		path := filepath.Join(workspace, filepath.FromSlash(relPath))
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		modified := info.ModTime().UTC()
		if !modified.After(stateTime.Add(2 * time.Second)) {
			continue
		}
		warnings = append(warnings, taskArtifactFreshness{
			Path:           relPath,
			Kind:           candidate.Kind,
			ModifiedAt:     modified.Format(time.RFC3339),
			StateUpdatedAt: stateText,
			Reason:         "task artifact changed after the task state was last captured; run ds task sync",
		})
	}
	return warnings
}

func taskFreshnessCandidates(manifest taskManifest) []taskSyncArtifactCandidate {
	var out []taskSyncArtifactCandidate
	out = append(out, taskSyncArtifactCandidate{RelPath: manifest.Artifacts.Index, Kind: "index"})
	for _, slice := range taskSyncSlices(manifest) {
		out = append(out,
			taskSyncArtifactCandidate{RelPath: slice.Plan, Kind: "plan"},
			taskSyncArtifactCandidate{RelPath: slice.Result, Kind: "result"},
		)
	}
	return out
}

func taskSyncSlices(manifest taskManifest) []taskSliceArtifact {
	if len(manifest.Artifacts.Slices) > 0 {
		return manifest.Artifacts.Slices
	}
	first := firstTaskSliceArtifact(manifest.Artifacts)
	if first.Plan == "" && first.Result == "" {
		return nil
	}
	return []taskSliceArtifact{first}
}

func taskCaptureStatusForSlice(manifest taskManifest, slice taskSliceArtifact) string {
	status := taskArtifactStatus(slice.Stage, slice.Decision)
	if status == "" || status == "unknown" {
		status = taskArtifactStatus(manifest.Status, manifest.Decision)
	}
	if status == "" || status == "unknown" {
		return "implementing"
	}
	return status
}

func taskManifestFreshnessTime(manifest taskManifest) (time.Time, string, bool) {
	value := strings.TrimSpace(manifest.UpdatedAt)
	if value == "" {
		value = strings.TrimSpace(manifest.CreatedAt)
	}
	if value == "" {
		return time.Time{}, "", false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, "", false
	}
	return parsed.UTC(), value, true
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
	case decision == "promote" || decision == "complete" || decision == "completed" || stage == "completed" || stage == "done" || stage == "implemented" || stage == "validated":
		return "implemented"
	case decision == "superseded" || decision == "split" || stage == "superseded" || stage == "split":
		return "superseded"
	case decision == "block" || decision == "blocked" || decision == "cancel" || decision == "cancelled" || decision == "rollback" || stage == "blocked" || stage == "cancelled" || stage == "rolled_back":
		return "rejected"
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
