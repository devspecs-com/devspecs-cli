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
	Dir       string
	Repo      string
	Workspace string
	Target    string
	Thread    string
	AsJSON    bool
}

type applyPromptOutput struct {
	Command            string              `json:"command"`
	TaskID             string              `json:"task_id"`
	Target             string              `json:"target"`
	Prompt             string              `json:"prompt"`
	TargetContext      taskTargetOutput    `json:"target_context"`
	SiblingTargets     []string            `json:"sibling_targets,omitempty"`
	PriorSliceEvidence []taskAdvisoryFile  `json:"prior_slice_evidence,omitempty"`
	ThreadContext      *applyThreadContext `json:"thread_context,omitempty"`
}

type applyThreadContext struct {
	Owner         threadStatusOwner   `json:"owner"`
	Key           string              `json:"key,omitempty"`
	Name          string              `json:"name,omitempty"`
	State         string              `json:"state"`
	TargetAddress string              `json:"target_address,omitempty"`
	Target        *threadStatusTarget `json:"target,omitempty"`
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
		Use:   "apply [task-id|change-id|target]",
		Short: "Emit a one-slice DevSpecs apply prompt",
		Long: `Emit an agent prompt for exactly one DevSpecs task target.

This command is prompt-only: it resolves the next or requested slice and prints
the bounded instruction an agent should follow. It does not launch an agent,
mark the target started, or advance lifecycle state.

For a task with named threads, argument-free apply continues only when exactly
one lane is runnable. If several lanes are runnable, inspect ds thread <owner>
and select one with ds apply <owner> --thread <key>. Ordinary tasks are
repo-owned; explicitly linked workspace changes use the same commands.`,
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
				"thread":        opts.Thread != "",
			})
			return err
		},
	}

	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().StringVar(&opts.Repo, repoTargetFlagName, "", "Target repository path for repo-local DevSpecs artifacts and context")
	cmd.Flags().StringVar(&opts.Workspace, "workspace", "", "Workspace root path for a workspace-owned thread")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Slice or follow-up target; useful when the first argument is a task id")
	cmd.Flags().StringVar(&opts.Thread, "thread", "", "Named execution thread to apply")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func runApply(cmd *cobra.Command, identifier string, opts applyOptions, implicitNext bool) error {
	if strings.EqualFold(strings.TrimSpace(identifier), "next") && strings.TrimSpace(opts.Target) != "" {
		return fmt.Errorf("ds apply next does not accept --target; use ds apply <task-id> --target <target>")
	}
	threaded, found, err := resolveThreadApply(cmd, identifier, opts, implicitNext)
	if err != nil {
		return err
	}
	if found {
		return writeApplyPrompt(cmd, threaded.Context, threaded.Command, opts.AsJSON, threaded.Thread)
	}
	ctx, command, err := resolveApplyTargetContext(opts.Dir, identifier, opts.Target, opts.Repo)
	if err != nil {
		return err
	}
	if implicitNext {
		command = applyCommandLabel("", "", opts.Repo)
	}
	return writeApplyPrompt(cmd, ctx, command, opts.AsJSON, nil)
}

type resolvedThreadApply struct {
	Context taskTargetContext
	Command string
	Thread  *applyThreadContext
}

func writeApplyPrompt(cmd *cobra.Command, ctx taskTargetContext, command string, asJSON bool, thread *applyThreadContext) error {
	target := taskTargetOutputFromContext(ctx, true)
	priorEvidence := taskPriorSliceEvidenceForPrompt(ctx.RepoRoot, ctx.Manifest.TaskID, ctx.Slice.ID)
	prompt := renderTaskAgentPrompt(ctx, target, priorEvidence)
	if thread != nil {
		prompt = renderThreadApplyContext(*thread) + prompt
	}
	out := applyPromptOutput{
		Command:            command,
		TaskID:             target.TaskID,
		Target:             target.Target,
		Prompt:             prompt,
		TargetContext:      target,
		SiblingTargets:     target.SiblingTargets,
		PriorSliceEvidence: priorEvidence,
		ThreadContext:      thread,
	}
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	_, err := fmt.Fprint(cmd.OutOrStdout(), prompt)
	return err
}

func resolveThreadApply(cmd *cobra.Command, identifier string, opts applyOptions, implicitNext bool) (resolvedThreadApply, bool, error) {
	statusOpts := threadStatusOptions{Dir: opts.Dir, Workspace: opts.Workspace}
	location, resolvedSelector, found, err := resolveDefinedThreadApplyOwner(cmd, identifier, opts, implicitNext, statusOpts)
	if err != nil || !found {
		return resolvedThreadApply{}, found, err
	}
	status, err := deriveThreadStatus(cmd, location)
	if err != nil {
		return resolvedThreadApply{}, true, err
	}
	if implicitNext && strings.TrimSpace(opts.Thread) == "" {
		if err := rejectImplicitThreadLegacyAmbiguity(status, opts); err != nil {
			return resolvedThreadApply{}, true, err
		}
	}
	selector := strings.TrimSpace(opts.Target)
	if selector == "" {
		selector = resolvedSelector
	}
	if location.Kind == threadOwnerWorkspaceChange && strings.TrimSpace(opts.Thread) == "" &&
		allThreadLanesCompleted(status.Threads) && len(status.UnassignedTargets) == 0 {
		closeout, found, closeoutErr := resolveWorkspaceTaskCloseout(identifier, opts, location, status)
		if closeoutErr != nil {
			return resolvedThreadApply{}, true, closeoutErr
		}
		if found {
			return closeout, true, nil
		}
	}
	lane, closeout, err := selectThreadApplyLane(status, opts.Thread, selector)
	if err != nil {
		return resolvedThreadApply{}, true, err
	}
	if closeout {
		if location.Kind != threadOwnerTask {
			return resolvedThreadApply{}, true, fmt.Errorf("workspace change %q has completed all assigned threads", location.ChangeID)
		}
		repoPath := threadApplyRepoPath(location, opts)
		ctx, err := loadTaskTargetContextForRepo(opts.Dir, location.TaskID, "", repoPath)
		if err != nil {
			return resolvedThreadApply{}, true, err
		}
		thread := &applyThreadContext{Owner: status.Owner, State: threadStateCompleted}
		return resolvedThreadApply{Context: ctx, Command: threadApplyCommand(identifier, "", opts), Thread: thread}, true, nil
	}
	target := *lane.CurrentTarget
	repoRoot, err := threadTargetRepoRoot(location, target)
	if err != nil {
		return resolvedThreadApply{}, true, err
	}
	repoPath := repoRoot
	if location.Kind == threadOwnerTask && strings.TrimSpace(opts.Repo) != "" {
		repoPath = opts.Repo
	}
	ctx, err := loadTaskTargetContextForRepo(opts.Dir, target.TaskID, target.Target, repoPath)
	if err != nil {
		return resolvedThreadApply{}, true, err
	}
	thread := &applyThreadContext{
		Owner: status.Owner, Key: lane.Key, Name: lane.Name, State: lane.State,
		TargetAddress: threadStatusTargetAddress(target), Target: &target,
	}
	return resolvedThreadApply{
		Context: ctx, Command: threadApplyCommand(identifier, lane.Key, opts), Thread: thread,
	}, true, nil
}

func resolveWorkspaceTaskCloseout(identifier string, opts applyOptions, location threadOwnerLocation, status threadStatusOutput) (resolvedThreadApply, bool, error) {
	kind, taskID := splitThreadOwnerSelector(strings.TrimSpace(identifier))
	if kind == threadOwnerWorkspaceChange || taskID == "" {
		return resolvedThreadApply{}, false, nil
	}
	ctx, err := loadResolvedTaskTargetContextForRepo(opts.Dir, taskID, opts.Target, opts.Repo)
	if err != nil {
		if kind == threadOwnerTask {
			return resolvedThreadApply{}, false, err
		}
		return resolvedThreadApply{}, false, nil
	}
	if !isTaskSeriesTarget(ctx.Manifest, ctx.Slice.ID) ||
		!strings.EqualFold(ctx.Manifest.ParentChange, location.ChangeID) ||
		!strings.EqualFold(ctx.Manifest.WorkspaceID, location.WorkspaceID) ||
		!samePath(ctx.Manifest.WorkspaceRoot, location.WorkspaceRoot) {
		return resolvedThreadApply{}, false, nil
	}
	thread := &applyThreadContext{Owner: status.Owner, State: threadStateCompleted}
	return resolvedThreadApply{
		Context: ctx, Command: threadApplyCommand(identifier, "", opts), Thread: thread,
	}, true, nil
}

func rejectImplicitThreadLegacyAmbiguity(status threadStatusOutput, opts applyOptions) error {
	candidates, _, err := findApplyNextTaskCandidates(opts.Dir, opts.Repo)
	if err != nil {
		return err
	}
	threadTasks := make(map[string]bool)
	for _, lane := range status.Threads {
		for _, target := range lane.Targets {
			threadTasks[strings.ToLower(target.TaskID)] = true
		}
	}
	var legacy []string
	for _, candidate := range candidates {
		if !threadTasks[strings.ToLower(candidate.TaskID)] {
			legacy = append(legacy, candidate.TaskID+":"+candidate.Target)
		}
	}
	if len(legacy) == 0 {
		return nil
	}
	sort.Strings(legacy)
	commands := threadRunnableApplyCommands(status)
	guidance := threadOwnerStatusCommand(status.Owner)
	if len(commands) > 0 {
		guidance += "\nRun a named lane explicitly:\n  " + strings.Join(commands, "\n  ")
	}
	return fmt.Errorf("implicit apply is ambiguous between a named thread and legacy targets %s\nInspect named threads: %s", strings.Join(legacy, ", "), guidance)
}

func resolveDefinedThreadApplyOwner(cmd *cobra.Command, identifier string, opts applyOptions, implicitNext bool, statusOpts threadStatusOptions) (threadOwnerLocation, string, bool, error) {
	identifier = strings.TrimSpace(identifier)
	if implicitNext || strings.EqualFold(identifier, "next") {
		location, err := inferThreadOwnerLocation(cmd, statusOpts)
		if err != nil {
			if strings.Contains(err.Error(), "no task with a thread definition is active") {
				if opts.Thread != "" {
					return threadOwnerLocation{}, "", false, err
				}
				return threadOwnerLocation{}, "", false, nil
			}
			return threadOwnerLocation{}, "", false, err
		}
		return location, "", true, nil
	}
	location, err := resolveThreadOwnerArtifactLocation(cmd, identifier, statusOpts)
	if err != nil {
		if opts.Thread != "" || strings.HasPrefix(strings.ToLower(identifier), "change:") {
			return threadOwnerLocation{}, "", false, err
		}
		if strings.Contains(err.Error(), "is ambiguous") {
			return threadOwnerLocation{}, "", false, err
		}
		return resolveThreadApplyOwnerFromTarget(identifier, opts)
	}
	if _, err := os.Stat(location.DefinitionPath); err != nil {
		if os.IsNotExist(err) && opts.Thread == "" {
			return resolveThreadApplyOwnerFromTarget(identifier, opts)
		}
		return threadOwnerLocation{}, "", false, err
	}
	return location, "", true, nil
}

func resolveThreadApplyOwnerFromTarget(identifier string, opts applyOptions) (threadOwnerLocation, string, bool, error) {
	ctx, err := loadResolvedTaskTargetContextForRepo(opts.Dir, identifier, opts.Target, opts.Repo)
	if err != nil {
		return threadOwnerLocation{}, "", false, nil
	}
	var location threadOwnerLocation
	if strings.TrimSpace(ctx.Manifest.ParentChange) != "" {
		location, err = linkedWorkspaceThreadOwnerLocation(ctx.Manifest)
	} else {
		location = repoThreadOwnerLocation(ctx.RepoRoot, ctx.Workspace, ctx.Manifest)
		_, err = os.Stat(location.DefinitionPath)
	}
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "has no thread definition") {
			return threadOwnerLocation{}, "", false, nil
		}
		return threadOwnerLocation{}, "", false, err
	}
	return location, ctx.Slice.ID, true, nil
}

func selectThreadApplyLane(status threadStatusOutput, key, selector string) (threadStatusLane, bool, error) {
	key = strings.TrimSpace(key)
	selector = strings.TrimSpace(selector)
	if key != "" {
		for _, lane := range status.Threads {
			if strings.EqualFold(lane.Key, key) {
				return validateSelectedThreadLane(lane, selector)
			}
		}
		return threadStatusLane{}, false, fmt.Errorf("thread %q not found", key)
	}
	if selector != "" {
		var matched []threadStatusLane
		for _, lane := range status.Threads {
			if threadLaneMatchesCurrentTarget(lane, selector) {
				matched = append(matched, lane)
			}
		}
		if len(matched) == 1 {
			return validateSelectedThreadLane(matched[0], selector)
		}
		if len(matched) == 0 {
			return threadStatusLane{}, false, fmt.Errorf("target %q is not the current target of any thread; dependencies and lane order cannot be bypassed", selector)
		}
		return threadStatusLane{}, false, fmt.Errorf("target %q matches multiple threads", selector)
	}
	var runnable []threadStatusLane
	for _, lane := range status.Threads {
		if lane.State == threadStateReady || lane.State == threadStateActive {
			runnable = append(runnable, lane)
		}
	}
	switch len(runnable) {
	case 0:
		if allThreadLanesCompleted(status.Threads) && len(status.UnassignedTargets) == 0 {
			return threadStatusLane{}, true, nil
		}
		return threadStatusLane{}, false, fmt.Errorf("no thread is runnable; inspect `ds thread` for waiting, blocked, or unassigned work")
	case 1:
		return runnable[0], false, nil
	default:
		keys := make([]string, 0, len(runnable))
		for _, lane := range runnable {
			keys = append(keys, lane.Key)
		}
		sort.Strings(keys)
		return threadStatusLane{}, false, fmt.Errorf(
			"multiple threads are runnable: %s\nInspect: %s\nRun one:\n  %s",
			strings.Join(keys, ", "),
			threadOwnerStatusCommand(status.Owner),
			strings.Join(threadRunnableApplyCommands(status), "\n  "),
		)
	}
}

func validateSelectedThreadLane(lane threadStatusLane, selector string) (threadStatusLane, bool, error) {
	if lane.State != threadStateReady && lane.State != threadStateActive {
		reason := strings.Join(lane.Reasons, "; ")
		if reason != "" {
			reason = ": " + reason
		}
		return threadStatusLane{}, false, fmt.Errorf("thread %q is %s%s", lane.Key, lane.State, reason)
	}
	if lane.CurrentTarget == nil {
		return threadStatusLane{}, false, fmt.Errorf("thread %q has no current target", lane.Key)
	}
	if selector != "" && !threadTargetMatchesSelector(*lane.CurrentTarget, selector) {
		return threadStatusLane{}, false, fmt.Errorf("target %q is not thread %q's current target %q; lane order cannot be bypassed", selector, lane.Key, threadStatusTargetAddress(*lane.CurrentTarget))
	}
	return lane, false, nil
}

func threadLaneMatchesCurrentTarget(lane threadStatusLane, selector string) bool {
	return lane.CurrentTarget != nil && threadTargetMatchesSelector(*lane.CurrentTarget, selector)
}

func threadTargetMatchesSelector(target threadStatusTarget, selector string) bool {
	return strings.EqualFold(target.Target, selector) ||
		strings.EqualFold(target.DefinitionTarget, selector) ||
		strings.EqualFold(threadStatusTargetAddress(target), selector) ||
		strings.EqualFold(target.TaskID+":"+target.Target, selector) ||
		strings.EqualFold(target.TaskID+":"+target.DefinitionTarget, selector) ||
		(target.RepoAlias != "" && (strings.EqualFold(target.RepoAlias+":"+target.Target, selector) ||
			strings.EqualFold(target.RepoAlias+":"+target.DefinitionTarget, selector) ||
			strings.EqualFold(target.RepoAlias+":"+target.TaskID+":"+target.Target, selector) ||
			strings.EqualFold(target.RepoAlias+":"+target.TaskID+":"+target.DefinitionTarget, selector)))
}

func threadApplyRepoPath(location threadOwnerLocation, opts applyOptions) string {
	if location.Kind == threadOwnerTask && strings.TrimSpace(opts.Repo) != "" {
		return opts.Repo
	}
	return location.RepoRoot
}

func allThreadLanesCompleted(lanes []threadStatusLane) bool {
	if len(lanes) == 0 {
		return false
	}
	for _, lane := range lanes {
		if lane.State != threadStateCompleted {
			return false
		}
	}
	return true
}

func threadTargetRepoRoot(location threadOwnerLocation, target threadStatusTarget) (string, error) {
	if location.Kind == threadOwnerTask {
		return location.RepoRoot, nil
	}
	manifest, err := readWorkspaceManifest(location.WorkspaceRoot)
	if err != nil {
		return "", err
	}
	repo, exists := manifest.Repos[target.RepoAlias]
	if !exists {
		return "", fmt.Errorf("workspace thread target references unknown repo %q", target.RepoAlias)
	}
	return workspaceRelativeAbs(location.WorkspaceRoot, repo.Path), nil
}

func threadApplyCommand(identifier, key string, opts applyOptions) string {
	command := "ds apply"
	if strings.TrimSpace(identifier) != "" && !strings.EqualFold(strings.TrimSpace(identifier), "next") {
		command += " " + commandArg(identifier)
	}
	if strings.TrimSpace(key) != "" {
		command += " --thread " + commandArg(key)
	}
	if strings.TrimSpace(opts.Target) != "" {
		command += " --target " + commandArg(opts.Target)
	}
	if strings.TrimSpace(opts.Repo) != "" {
		command += " --repo " + commandArg(opts.Repo)
	}
	if strings.TrimSpace(opts.Workspace) != "" {
		command += " --workspace " + commandArg(opts.Workspace)
	}
	return command
}

func renderThreadApplyContext(thread applyThreadContext) string {
	owner := "task:" + thread.Owner.TaskID
	if thread.Owner.Kind == threadOwnerWorkspaceChange {
		owner = "change:" + thread.Owner.ChangeID
	}
	var b strings.Builder
	fmt.Fprintln(&b, "Execution thread:")
	fmt.Fprintf(&b, "- Owner: %s\n", owner)
	if thread.Key != "" {
		fmt.Fprintf(&b, "- Thread: %s\n", thread.Key)
	}
	if thread.TargetAddress != "" {
		fmt.Fprintf(&b, "- Current target: %s\n", thread.TargetAddress)
	}
	fmt.Fprintf(&b, "- State: %s\n\n", thread.State)
	return b.String()
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
