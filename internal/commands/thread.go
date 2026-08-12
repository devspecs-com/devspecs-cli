package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

const threadProjectionProgressDelay = 250 * time.Millisecond

type threadStatusOptions struct {
	Dir       string
	Workspace string
	AsJSON    bool
}

// NewThreadCmd creates the experimental named execution-thread command.
func NewThreadCmd() *cobra.Command {
	opts := threadStatusOptions{Dir: defaultTaskWorkspaceDir}
	cmd := &cobra.Command{
		Use:   "thread [task:<task-id>|change:<change-id>]",
		Short: "Show runnable named task threads and joins",
		Long: `Show named execution threads for a repo task or linked workspace change.

Threads schedule existing task slices. Ordinary tasks own repo-local threads
without requiring a workspace. Only a task explicitly linked to a workspace
change joins that change's cross-repo thread graph; merely living below a
workspace does not change ownership.

Use this command to inspect runnable lanes, then use ds apply <owner> --thread
<key> to emit one bounded prompt. The same root commands work for repo and
workspace owners; there is no separate workspace thread command family.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner := ""
			if len(args) == 1 {
				owner = args[0]
			}
			started := time.Now()
			err := runThreadStatus(cmd, owner, opts)
			telemetry.RecordCommand("thread_status", err == nil, time.Since(started), map[string]any{
				"json":      opts.AsJSON,
				"workspace": opts.Workspace != "",
				"explicit":  owner != "",
			})
			return err
		},
	}
	addRepoTargetPersistentFlag(cmd)
	cmd.PersistentFlags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.PersistentFlags().StringVar(&opts.Workspace, "workspace", "", "Workspace root path")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	cmd.AddCommand(newThreadSetCmd(&opts))
	cmd.AddCommand(newThreadRemoveCmd(&opts))
	return cmd
}

func runThreadStatus(cmd *cobra.Command, owner string, opts threadStatusOptions) error {
	location, err := resolveThreadOwnerLocation(cmd, owner, opts)
	if err != nil {
		return err
	}
	out, err := deriveThreadStatus(cmd, location)
	if err != nil {
		return err
	}
	return writeThreadStatus(cmd, out, opts.AsJSON)
}

func resolveThreadOwnerLocation(cmd *cobra.Command, owner string, opts threadStatusOptions) (threadOwnerLocation, error) {
	return resolveThreadOwnerLocationMode(cmd, owner, opts, true)
}

func resolveThreadOwnerArtifactLocation(cmd *cobra.Command, owner string, opts threadStatusOptions) (threadOwnerLocation, error) {
	if strings.TrimSpace(owner) == "" {
		return threadOwnerLocation{}, fmt.Errorf("thread owner is required; pass task:<task-id> or change:<change-id>")
	}
	return resolveThreadOwnerLocationMode(cmd, owner, opts, false)
}

func resolveThreadOwnerLocationMode(cmd *cobra.Command, owner string, opts threadStatusOptions, requireDefinition bool) (threadOwnerLocation, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		if !requireDefinition {
			return threadOwnerLocation{}, fmt.Errorf("thread owner is required; pass task:<task-id> or change:<change-id>")
		}
		return inferThreadOwnerLocation(cmd, opts)
	}
	kind, id := splitThreadOwnerSelector(owner)
	var taskLocation threadOwnerLocation
	var taskFound bool
	var taskErr error
	if kind != threadOwnerWorkspaceChange {
		taskLocation, taskFound, taskErr = findRepoThreadOwnerLocationMode(cmd, id, opts.Dir, requireDefinition)
		if taskErr != nil && (kind == threadOwnerTask || !errors.Is(taskErr, os.ErrNotExist)) {
			return threadOwnerLocation{}, taskErr
		}
		if taskFound && taskLocation.WorkspaceRoot != "" {
			return taskLocation, nil
		}
	}
	var changeLocation threadOwnerLocation
	var changeFound bool
	var changeErr error
	if kind != threadOwnerTask {
		changeLocation, changeFound, changeErr = findWorkspaceThreadOwnerLocationMode(id, opts.Workspace, requireDefinition)
		if kind == threadOwnerWorkspaceChange && changeErr != nil {
			return threadOwnerLocation{}, changeErr
		}
	}
	if kind == "" && taskFound && changeFound {
		return threadOwnerLocation{}, fmt.Errorf("thread owner %q is ambiguous; use task:%s or change:%s", id, id, id)
	}
	if taskFound {
		return taskLocation, nil
	}
	if changeFound {
		return changeLocation, nil
	}
	if taskErr != nil && changeErr != nil {
		return threadOwnerLocation{}, fmt.Errorf("thread owner %q was not found as a repo task or workspace change", id)
	}
	if taskErr != nil {
		return threadOwnerLocation{}, taskErr
	}
	if changeErr != nil {
		return threadOwnerLocation{}, changeErr
	}
	return threadOwnerLocation{}, fmt.Errorf("thread owner %q not found", id)
}

func splitThreadOwnerSelector(owner string) (string, string) {
	parts := strings.SplitN(owner, ":", 2)
	if len(parts) != 2 {
		return "", owner
	}
	switch strings.ToLower(strings.TrimSpace(parts[0])) {
	case "task":
		return threadOwnerTask, strings.TrimSpace(parts[1])
	case "change":
		return threadOwnerWorkspaceChange, strings.TrimSpace(parts[1])
	default:
		return "", owner
	}
}

func findRepoThreadOwnerLocationMode(cmd *cobra.Command, taskID, baseDir string, requireDefinition bool) (threadOwnerLocation, bool, error) {
	repoRoot, err := resolveTargetRepoRootContext(cmd.Context(), commandRepoTarget(cmd))
	if err != nil {
		return threadOwnerLocation{}, false, err
	}
	taskWorkspace, manifest, _, err := readThreadTaskManifestWithBase(repoRoot, baseDir, taskID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return threadOwnerLocation{}, false, nil
		}
		return threadOwnerLocation{}, false, err
	}
	if strings.TrimSpace(manifest.ParentChange) != "" {
		location, linkedErr := linkedWorkspaceThreadOwnerLocationMode(manifest, requireDefinition)
		return location, linkedErr == nil, linkedErr
	}
	location := repoThreadOwnerLocation(repoRoot, taskWorkspace, manifest)
	if !requireDefinition {
		return location, true, nil
	}
	if _, err := os.Stat(location.DefinitionPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return threadOwnerLocation{}, false, fmt.Errorf("task %q has no thread definition at %s", taskID, location.DefinitionPath)
		}
		return threadOwnerLocation{}, false, err
	}
	return location, true, nil
}

func readThreadTaskManifestWithBase(repoRoot, baseDir, taskID string) (string, taskManifest, string, error) {
	var firstErr error
	for _, taskWorkspace := range taskWorkspaceSearchPaths(repoRoot, baseDir, taskID) {
		manifestPath := filepath.Join(taskWorkspace, taskManifestFilename)
		manifest, err := readTaskManifest(manifestPath)
		if err == nil {
			return taskWorkspace, manifest, manifestPath, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", taskManifest{}, "", err
		}
	}
	return "", taskManifest{}, "", firstErr
}

func linkedWorkspaceThreadOwnerLocation(task taskManifest) (threadOwnerLocation, error) {
	return linkedWorkspaceThreadOwnerLocationMode(task, true)
}

func linkedWorkspaceThreadOwnerLocationMode(task taskManifest, requireDefinition bool) (threadOwnerLocation, error) {
	workspaceRoot := strings.TrimSpace(task.WorkspaceRoot)
	if workspaceRoot == "" {
		return threadOwnerLocation{}, fmt.Errorf("task %q links change %q without workspace_root", task.TaskID, task.ParentChange)
	}
	workspaceRoot, err := resolveWorkspaceRoot(workspaceRoot)
	if err != nil {
		return threadOwnerLocation{}, err
	}
	manifest, err := readWorkspaceManifest(workspaceRoot)
	if err != nil {
		return threadOwnerLocation{}, err
	}
	if task.WorkspaceID != "" && !strings.EqualFold(task.WorkspaceID, manifest.ID) {
		return threadOwnerLocation{}, fmt.Errorf("task %q workspace_id %q does not match workspace %q", task.TaskID, task.WorkspaceID, manifest.ID)
	}
	return requireWorkspaceThreadOwnerLocationMode(workspaceRoot, manifest, task.ParentChange, requireDefinition)
}

func findWorkspaceThreadOwnerLocationMode(changeID, workspacePath string, requireDefinition bool) (threadOwnerLocation, bool, error) {
	workspaceRoot, err := resolveWorkspaceRoot(workspacePath)
	if err != nil {
		return threadOwnerLocation{}, false, err
	}
	manifest, err := readWorkspaceManifest(workspaceRoot)
	if err != nil {
		return threadOwnerLocation{}, false, err
	}
	location, err := requireWorkspaceThreadOwnerLocationMode(workspaceRoot, manifest, changeID, requireDefinition)
	if err != nil {
		return threadOwnerLocation{}, false, err
	}
	return location, true, nil
}

func requireWorkspaceThreadOwnerLocationMode(workspaceRoot string, manifest workspaceManifest, changeID string, requireDefinition bool) (threadOwnerLocation, error) {
	_, change, _, err := findWorkspaceChange(workspaceRoot, manifest, changeID)
	if err != nil {
		return threadOwnerLocation{}, err
	}
	location := workspaceThreadOwnerLocation(workspaceRoot, manifest, change.ID)
	if !requireDefinition {
		return location, nil
	}
	if _, err := os.Stat(location.DefinitionPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return threadOwnerLocation{}, fmt.Errorf("workspace change %q has no thread definition at %s", change.ID, location.DefinitionPath)
		}
		return threadOwnerLocation{}, err
	}
	return location, nil
}

func inferThreadOwnerLocation(cmd *cobra.Command, opts threadStatusOptions) (threadOwnerLocation, error) {
	repoRoot, err := resolveTargetRepoRootContext(cmd.Context(), commandRepoTarget(cmd))
	if err != nil {
		return threadOwnerLocation{}, err
	}
	var candidates []threadOwnerLocation
	seen := make(map[string]bool)
	for _, parent := range taskWorkspaceSearchParents(repoRoot, opts.Dir) {
		entries, readErr := os.ReadDir(parent)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return threadOwnerLocation{}, readErr
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			taskWorkspace := filepath.Join(parent, entry.Name())
			manifest, manifestErr := readTaskManifest(filepath.Join(taskWorkspace, taskManifestFilename))
			if manifestErr != nil {
				if _, definitionErr := os.Stat(repoThreadDefinitionPath(taskWorkspace)); definitionErr == nil {
					return threadOwnerLocation{}, manifestErr
				}
				continue
			}
			var location threadOwnerLocation
			if strings.TrimSpace(manifest.ParentChange) != "" {
				location, manifestErr = linkedWorkspaceThreadOwnerLocation(manifest)
			} else {
				location = repoThreadOwnerLocation(repoRoot, taskWorkspace, manifest)
				_, manifestErr = os.Stat(location.DefinitionPath)
			}
			if manifestErr != nil || seen[location.OwnerID] {
				continue
			}
			seen[location.OwnerID] = true
			candidates = append(candidates, location)
		}
	}
	active := candidates[:0]
	for _, candidate := range candidates {
		snapshot, _, loadErr := loadThreadOwnerSnapshot(candidate, nil)
		if loadErr != nil {
			return threadOwnerLocation{}, loadErr
		}
		status, statusErr := buildThreadStatus(snapshot)
		if statusErr != nil {
			return threadOwnerLocation{}, statusErr
		}
		if threadStatusHasOpenWork(status) {
			active = append(active, candidate)
		}
	}
	candidates = active
	switch len(candidates) {
	case 0:
		return threadOwnerLocation{}, fmt.Errorf("no task with a thread definition is active in %s; pass a task or change ID", repoRoot)
	case 1:
		return candidates[0], nil
	default:
		var commands []string
		for _, candidate := range candidates {
			commands = append(commands, threadOwnerStatusCommand(threadStatusOwnerFromLocation(candidate)))
		}
		sort.Strings(commands)
		return threadOwnerLocation{}, fmt.Errorf("multiple thread owners are available; inspect one explicitly:\n  %s", strings.Join(commands, "\n  "))
	}
}

func threadStatusOwnerFromLocation(location threadOwnerLocation) threadStatusOwner {
	return threadStatusOwner{
		Kind:          location.Kind,
		TaskID:        location.TaskID,
		WorkspaceID:   location.WorkspaceID,
		ChangeID:      location.ChangeID,
		RepoRoot:      location.RepoRoot,
		WorkspaceRoot: location.WorkspaceRoot,
	}
}

func threadOwnerSelector(owner threadStatusOwner) string {
	if owner.Kind == threadOwnerWorkspaceChange {
		return "change:" + owner.ChangeID
	}
	return "task:" + owner.TaskID
}

func threadOwnerStatusCommand(owner threadStatusOwner) string {
	command := "ds thread " + commandArg(threadOwnerSelector(owner))
	if owner.Kind == threadOwnerWorkspaceChange && strings.TrimSpace(owner.WorkspaceRoot) != "" {
		return command + " --workspace " + commandArg(owner.WorkspaceRoot)
	}
	if strings.TrimSpace(owner.RepoRoot) != "" {
		return command + " --repo " + commandArg(owner.RepoRoot)
	}
	return command
}

func threadOwnerApplyCommand(owner threadStatusOwner, key string) string {
	command := "ds apply " + commandArg(threadOwnerSelector(owner)) + " --thread " + commandArg(key)
	if owner.Kind == threadOwnerWorkspaceChange && strings.TrimSpace(owner.WorkspaceRoot) != "" {
		return command + " --workspace " + commandArg(owner.WorkspaceRoot)
	}
	if strings.TrimSpace(owner.RepoRoot) != "" {
		return command + " --repo " + commandArg(owner.RepoRoot)
	}
	return command
}

func threadRunnableApplyCommands(status threadStatusOutput) []string {
	commands := make([]string, 0, len(status.Threads))
	for _, lane := range status.Threads {
		if lane.State == threadStateReady || lane.State == threadStateActive {
			commands = append(commands, threadOwnerApplyCommand(status.Owner, lane.Key))
		}
	}
	sort.Strings(commands)
	return commands
}

func threadStatusHasOpenWork(status threadStatusOutput) bool {
	for _, lane := range status.Threads {
		if lane.State != threadStateCompleted {
			return true
		}
	}
	return len(status.UnassignedTargets) > 0
}

func deriveThreadStatus(cmd *cobra.Command, location threadOwnerLocation) (threadStatusOutput, error) {
	db, dbErr := openDB()
	if dbErr != nil {
		snapshot, _, err := reconstructThreadSnapshotWithProgress(cmd, location, nil)
		if err != nil {
			return threadStatusOutput{}, err
		}
		out, err := buildThreadStatus(snapshot)
		if err != nil {
			return threadStatusOutput{}, err
		}
		out.StateSource = "artifact_reconstruction"
		out.ProjectionFreshness = "unavailable"
		out.ProjectionWarning = dbErr.Error()
		return out, nil
	}
	defer db.Close()
	projection, found, loadErr := db.GetThreadProjection(location.OwnerID)
	if loadErr == nil && found {
		fresh, freshnessErr := threadProjectionIsFresh(projection)
		if freshnessErr == nil && fresh {
			snapshot, decodeErr := threadSnapshotFromProjection(projection)
			if decodeErr == nil {
				out, statusErr := buildThreadStatus(snapshot)
				if statusErr == nil {
					out.StateSource = "sqlite_projection"
					out.ProjectionFreshness = "fresh"
					return out, nil
				}
			}
		}
	}
	var prior *store.ThreadProjection
	if loadErr == nil && found {
		prior = &projection
	}
	snapshot, _, err := reconstructThreadSnapshotWithProgress(cmd, location, prior)
	if err != nil {
		return threadStatusOutput{}, err
	}
	out, err := buildThreadStatus(snapshot)
	if err != nil {
		return threadStatusOutput{}, err
	}
	out.StateSource = "artifact_reconstruction"
	out.ProjectionRefreshed = false
	if found {
		out.ProjectionFreshness = "refreshed"
	} else {
		out.ProjectionFreshness = "rebuilt"
	}
	projection, projectionErr := threadProjectionFromSnapshot(snapshot)
	if projectionErr == nil {
		projectionErr = db.ReplaceThreadProjection(projection)
	}
	if projectionErr != nil {
		out.ProjectionWarning = projectionErr.Error()
	} else {
		out.ProjectionRefreshed = true
	}
	if loadErr != nil {
		out.ProjectionWarning = loadErr.Error()
	}
	return out, nil
}

func reconstructThreadSnapshotWithProgress(cmd *cobra.Command, location threadOwnerLocation, prior *store.ThreadProjection) (threadOwnerSnapshot, threadProjectionLoadStats, error) {
	done := make(chan struct{})
	finished := make(chan struct{})
	timer := time.NewTimer(threadProjectionProgressDelay)
	go func() {
		defer close(finished)
		select {
		case <-timer.C:
			fmt.Fprintln(cmd.ErrOrStderr(), "Thread status progress: reconstructing durable task state")
		case <-done:
		}
	}()
	snapshot, stats, err := loadThreadOwnerSnapshot(location, prior)
	close(done)
	timer.Stop()
	<-finished
	return snapshot, stats, err
}

func writeThreadStatus(cmd *cobra.Command, out threadStatusOutput, asJSON bool) error {
	if asJSON {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(out)
	}
	if out.ProjectionWarning != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "Thread status warning: local projection unavailable; durable artifacts were used (%s)\n", out.ProjectionWarning)
	}
	if out.Owner.Kind == threadOwnerTask {
		fmt.Fprintf(cmd.OutOrStdout(), "Task: %s\n", out.Owner.TaskID)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Change: %s\n", out.Owner.ChangeID)
	}
	for _, group := range []struct {
		title string
		state string
	}{
		{"Ready threads", threadStateReady},
		{"Active threads", threadStateActive},
		{"Waiting threads", threadStateWaiting},
		{"Blocked threads", threadStateBlocked},
		{"Completed threads", threadStateCompleted},
	} {
		writeThreadStatusGroup(cmd, out.Threads, group.title, group.state)
	}
	if len(out.UnassignedTargets) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Unassigned targets")
		for _, target := range out.UnassignedTargets {
			fmt.Fprintf(cmd.OutOrStdout(), "  %-16s %s\n", threadStatusTargetAddress(target), target.Title)
		}
	}
	commands := threadRunnableApplyCommands(out)
	if len(commands) == 1 {
		fmt.Fprintf(cmd.OutOrStdout(), "Run: %s\n", commands[0])
	} else if len(commands) > 1 {
		fmt.Fprintln(cmd.OutOrStdout(), "Run one:")
		for _, command := range commands {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", command)
		}
	}
	return nil
}

func writeThreadStatusGroup(cmd *cobra.Command, lanes []threadStatusLane, title, state string) {
	count := 0
	for _, lane := range lanes {
		if lane.State == state {
			count++
		}
	}
	if count == 0 {
		return
	}
	fmt.Fprintln(cmd.OutOrStdout(), title)
	for _, lane := range lanes {
		if lane.State != state {
			continue
		}
		target := threadStatusTarget{}
		if lane.CurrentTarget != nil {
			target = *lane.CurrentTarget
		} else if len(lane.Targets) > 0 {
			target = lane.Targets[len(lane.Targets)-1]
		}
		line := fmt.Sprintf("  %-16s %-16s %s", lane.Key, threadStatusTargetAddress(target), target.Title)
		if len(lane.Reasons) > 0 {
			line += "  " + strings.Join(lane.Reasons, "; ")
		}
		fmt.Fprintln(cmd.OutOrStdout(), strings.TrimRight(line, " "))
	}
}
