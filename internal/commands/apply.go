package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

type applyOptions struct {
	Dir    string
	Repo   string
	Target string
	AsJSON bool
}

type applyPromptOutput struct {
	Command            string             `json:"command"`
	TaskID             string             `json:"task_id"`
	Target             string             `json:"target"`
	Prompt             string             `json:"prompt"`
	TargetContext      taskTargetOutput   `json:"target_context"`
	SiblingTargets     []string           `json:"sibling_targets,omitempty"`
	PriorSliceEvidence []taskAdvisoryFile `json:"prior_slice_evidence,omitempty"`
}

type applyTaskCandidate struct {
	TaskID string
	Target string
	Title  string
}

// NewApplyCmd creates the ds apply command.
func NewApplyCmd() *cobra.Command {
	var opts applyOptions
	opts.Dir = defaultTaskWorkspaceDir

	cmd := &cobra.Command{
		Use:   "apply [task-id|target]",
		Short: "Emit a one-slice DevSpecs apply prompt",
		Long: `Emit an agent prompt for exactly one DevSpecs task target.

This command is prompt-only: it resolves the next or requested slice and prints
the bounded instruction an agent should follow. It does not launch an agent,
mark the target started, or advance lifecycle state.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()
			identifier := "next"
			implicitNext := true
			if len(args) > 0 {
				identifier = args[0]
				implicitNext = false
			}
			err := runApply(cmd, identifier, opts, implicitNext)
			telemetry.RecordCommand("apply", err == nil, time.Since(start), map[string]any{
				"json":          opts.AsJSON,
				"next":          strings.EqualFold(strings.TrimSpace(identifier), "next"),
				"implicit_next": implicitNext,
				"repo":          opts.Repo != "",
			})
			return err
		},
	}

	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().StringVar(&opts.Repo, repoTargetFlagName, "", "Target repository path for repo-local DevSpecs artifacts and context")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Slice or follow-up target; useful when the first argument is a task id")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func runApply(cmd *cobra.Command, identifier string, opts applyOptions, implicitNext bool) error {
	ctx, command, err := resolveApplyTargetContext(opts.Dir, identifier, opts.Target, opts.Repo)
	if err != nil {
		return err
	}
	if implicitNext {
		command = applyCommandLabel("", "", opts.Repo)
	}
	target := taskTargetOutputFromContext(ctx, true)
	priorEvidence := taskPriorSliceEvidenceForPrompt(ctx.RepoRoot, ctx.Manifest.TaskID, ctx.Slice.ID)
	prompt := renderTaskAgentPrompt(ctx, target, priorEvidence)
	out := applyPromptOutput{
		Command:            command,
		TaskID:             target.TaskID,
		Target:             target.Target,
		Prompt:             prompt,
		TargetContext:      target,
		SiblingTargets:     target.SiblingTargets,
		PriorSliceEvidence: priorEvidence,
	}
	if opts.AsJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), prompt)
	return err
}

func resolveApplyTargetContext(baseDir, identifier, selector, repoPath string) (taskTargetContext, string, error) {
	identifier = strings.TrimSpace(identifier)
	selector = strings.TrimSpace(selector)
	if identifier == "" {
		return taskTargetContext{}, "", fmt.Errorf("apply identifier is empty")
	}
	if strings.EqualFold(identifier, "next") {
		if selector != "" {
			return taskTargetContext{}, "", fmt.Errorf("ds apply next does not accept --target; use ds apply <task-id> --target <target>")
		}
		taskID, err := resolveApplyNextTaskID(baseDir, repoPath)
		if err != nil {
			return taskTargetContext{}, "", err
		}
		ctx, err := loadTaskTargetContextForRepo(baseDir, taskID, "", repoPath)
		return ctx, applyCommandLabel(identifier, selector, repoPath), err
	}

	ctx, err := loadResolvedTaskTargetContextForRepo(baseDir, identifier, selector, repoPath)
	if err == nil {
		return ctx, applyCommandLabel(identifier, selector, repoPath), nil
	}
	if selector == "" {
		if taskID, target, seriesErr := resolveApplySeriesTarget(baseDir, identifier, repoPath); seriesErr == nil {
			ctx, loadErr := loadTaskTargetContextForRepo(baseDir, taskID, target, repoPath)
			return ctx, applyCommandLabel(identifier, selector, repoPath), loadErr
		} else if shouldPreferApplySeriesError(identifier, err) {
			return taskTargetContext{}, "", seriesErr
		}
	}
	return taskTargetContext{}, "", err
}

func applyCommandLabel(identifier, selector, repoPath string) string {
	identifier = strings.TrimSpace(identifier)
	selector = strings.TrimSpace(selector)
	repoPath = strings.TrimSpace(repoPath)
	var command string
	if identifier == "" && selector == "" {
		command = "ds apply"
	} else if selector == "" {
		command = "ds apply " + identifier
	} else {
		command = "ds apply " + identifier + " --target " + selector
	}
	if repoPath != "" {
		command += " --repo " + commandArg(repoPath)
	}
	return command
}

func resolveApplyNextTaskID(baseDir, repoPath string) (string, error) {
	candidates, blocked, err := findApplyNextTaskCandidates(baseDir, repoPath)
	if err != nil {
		return "", err
	}
	switch len(candidates) {
	case 0:
		if len(blocked) == 1 {
			return "", blocked[0]
		}
		if len(blocked) > 1 {
			var labels []string
			for _, err := range blocked {
				labels = append(labels, err.Error())
			}
			return "", fmt.Errorf("multiple DevSpecs tasks are blocked from automatic next: %s; use `ds apply <task-id> --target <target>`", strings.Join(labels, "; "))
		}
		return "", fmt.Errorf("no non-terminal DevSpecs task targets found; run `ds task \"goal\"` or use `ds apply <task-id>`")
	case 1:
		return candidates[0].TaskID, nil
	default:
		var labels []string
		for _, candidate := range candidates {
			label := candidate.TaskID + ":" + candidate.Target
			if candidate.Title != "" {
				label += " (" + candidate.Title + ")"
			}
			labels = append(labels, label)
		}
		return "", fmt.Errorf("ambiguous next task target; matches: %s; use `ds apply <task-id>` or `ds apply <target-id>`", strings.Join(labels, ", "))
	}
}

func findApplyNextTaskCandidates(baseDir, repoPath string) ([]applyTaskCandidate, []error, error) {
	repoRoot, err := applyRepoRoot(repoPath)
	if err != nil {
		return nil, nil, err
	}
	var candidates []applyTaskCandidate
	var blocked []error
	for _, parent := range taskWorkspaceSearchParents(repoRoot, baseDir) {
		entries, err := os.ReadDir(parent)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, nil, fmt.Errorf("read task workspaces: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			manifestPath := filepath.Join(parent, entry.Name(), taskManifestFilename)
			if _, err := os.Stat(manifestPath); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, nil, err
			}
			manifest, err := readTaskManifest(manifestPath)
			if err != nil {
				continue
			}
			slice, err := taskNextSlice(manifest)
			if err != nil {
				if applyNextErrorBlocksAutomaticNext(err) {
					blocked = append(blocked, err)
				}
				continue
			}
			taskID := strings.TrimSpace(manifest.TaskID)
			if taskID == "" {
				taskID = entry.Name()
			}
			candidates = append(candidates, applyTaskCandidate{
				TaskID: taskID,
				Target: slice.ID,
				Title:  slice.Title,
			})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].TaskID == candidates[j].TaskID {
			return candidates[i].Target < candidates[j].Target
		}
		return candidates[i].TaskID < candidates[j].TaskID
	})
	return candidates, blocked, nil
}

func applyNextErrorBlocksAutomaticNext(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return !strings.Contains(msg, "all task targets are terminal") &&
		!strings.Contains(msg, "task has no slice targets")
}

func resolveApplySeriesTarget(baseDir, selector, repoPath string) (string, string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", "", fmt.Errorf("series target is empty")
	}
	repoRoot, err := applyRepoRoot(repoPath)
	if err != nil {
		return "", "", err
	}
	var matches []applyTaskCandidate
	var blocked []error
	for _, parent := range taskWorkspaceSearchParents(repoRoot, baseDir) {
		entries, err := os.ReadDir(parent)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", "", fmt.Errorf("read task workspaces: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			manifestPath := filepath.Join(parent, entry.Name(), taskManifestFilename)
			if _, err := os.Stat(manifestPath); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return "", "", err
			}
			manifest, err := readTaskManifest(manifestPath)
			if err != nil || !taskSeriesSelectorMatches(manifest, selector) {
				continue
			}
			slice, err := taskNextSlice(manifest)
			if err != nil {
				if applyNextErrorBlocksAutomaticNext(err) {
					blocked = append(blocked, err)
				}
				continue
			}
			taskID := strings.TrimSpace(manifest.TaskID)
			if taskID == "" {
				taskID = entry.Name()
			}
			matches = append(matches, applyTaskCandidate{TaskID: taskID, Target: slice.ID, Title: slice.Title})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].TaskID == matches[j].TaskID {
			return matches[i].Target < matches[j].Target
		}
		return matches[i].TaskID < matches[j].TaskID
	})
	switch len(matches) {
	case 0:
		if len(blocked) == 1 {
			return "", "", blocked[0]
		}
		if len(blocked) > 1 {
			var labels []string
			for _, err := range blocked {
				labels = append(labels, err.Error())
			}
			return "", "", fmt.Errorf("multiple DevSpecs task series matches are blocked from automatic next: %s; use a task id with --target", strings.Join(labels, "; "))
		}
		return "", "", fmt.Errorf("no task series matched %q", selector)
	case 1:
		return matches[0].TaskID, matches[0].Target, nil
	default:
		var labels []string
		for _, match := range matches {
			labels = append(labels, match.TaskID+":"+match.Target)
		}
		return "", "", fmt.Errorf("ambiguous task series %q; matches: %s; use a task id with --target", selector, strings.Join(labels, ", "))
	}
}

func taskSeriesSelectorMatches(manifest taskManifest, selector string) bool {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return false
	}
	series := defaultTaskSeries(manifest.Series)
	return strings.EqualFold(selector, series) ||
		strings.EqualFold(selector, series+"00") ||
		strings.EqualFold(selector, taskSeriesIndexFilename(series)) ||
		strings.EqualFold(selector, manifest.Artifacts.Index)
}

func shouldPreferApplySeriesError(identifier string, resolvedErr error) bool {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || resolvedErr == nil {
		return false
	}
	if !strings.HasSuffix(strings.ToUpper(identifier), "00") {
		return false
	}
	return true
}

func applyRepoRoot(repoPath string) (string, error) {
	return resolveTargetRepoRoot(repoPath)
}
