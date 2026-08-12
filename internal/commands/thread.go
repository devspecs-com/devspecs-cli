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
		Use:   "thread [<task-id|change-id>]",
		Short: "Show runnable named task threads and joins",
		Long: `Show named execution threads for a repo task or linked workspace change.

Threads schedule existing task slices. A standalone task is repo-owned; a task
explicitly linked to a workspace change resolves that change's cross-repo
thread graph. SQLite accelerates status, but durable YAML and checkpoint JSON
artifacts remain authoritative and reconstruct the same result after prune.`,
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
	cmd.Flags().StringVar(&opts.Dir, "dir", defaultTaskWorkspaceDir, "Task workspace parent directory")
	cmd.Flags().StringVar(&opts.Workspace, "workspace", "", "Workspace root path")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
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
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return inferThreadOwnerLocation(cmd, opts)
	}
	kind, id := splitThreadOwnerSelector(owner)
	var taskLocation threadOwnerLocation
	var taskFound bool
	var taskErr error
	if kind != threadOwnerWorkspaceChange {
		taskLocation, taskFound, taskErr = findRepoThreadOwnerLocation(cmd, id, opts.Dir)
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
		changeLocation, changeFound, changeErr = findWorkspaceThreadOwnerLocation(id, opts.Workspace)
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

func findRepoThreadOwnerLocation(cmd *cobra.Command, taskID, baseDir string) (threadOwnerLocation, bool, error) {
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
		location, linkedErr := linkedWorkspaceThreadOwnerLocation(manifest)
		return location, linkedErr == nil, linkedErr
	}
	location := repoThreadOwnerLocation(repoRoot, taskWorkspace, manifest)
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
	return requireWorkspaceThreadOwnerLocation(workspaceRoot, manifest, task.ParentChange)
}

func findWorkspaceThreadOwnerLocation(changeID, workspacePath string) (threadOwnerLocation, bool, error) {
	workspaceRoot, err := resolveWorkspaceRoot(workspacePath)
	if err != nil {
		return threadOwnerLocation{}, false, err
	}
	manifest, err := readWorkspaceManifest(workspaceRoot)
	if err != nil {
		return threadOwnerLocation{}, false, err
	}
	location, err := requireWorkspaceThreadOwnerLocation(workspaceRoot, manifest, changeID)
	if err != nil {
		return threadOwnerLocation{}, false, err
	}
	return location, true, nil
}

func requireWorkspaceThreadOwnerLocation(workspaceRoot string, manifest workspaceManifest, changeID string) (threadOwnerLocation, error) {
	_, change, _, err := findWorkspaceChange(workspaceRoot, manifest, changeID)
	if err != nil {
		return threadOwnerLocation{}, err
	}
	location := workspaceThreadOwnerLocation(workspaceRoot, manifest, change.ID)
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
		var labels []string
		for _, candidate := range candidates {
			if candidate.Kind == threadOwnerTask {
				labels = append(labels, "task:"+candidate.TaskID)
			} else {
				labels = append(labels, "change:"+candidate.ChangeID)
			}
		}
		sort.Strings(labels)
		return threadOwnerLocation{}, fmt.Errorf("multiple thread owners are available: %s; pass one explicitly", strings.Join(labels, ", "))
	}
}

func threadStatusHasOpenWork(status threadStatusOutput) bool {
	for _, lane := range status.Threads {
		if lane.State != threadStateCompleted {
			return true
		}
	}
	for _, target := range status.UnassignedTargets {
		if !target.Completed {
			return true
		}
	}
	return false
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
	if len(out.ReadyThreads) == 1 {
		owner := out.Owner.TaskID
		if out.Owner.Kind == threadOwnerWorkspaceChange {
			owner = out.Owner.ChangeID
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Run: ds apply %s --thread %s\n", commandArg(owner), commandArg(out.ReadyThreads[0]))
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
