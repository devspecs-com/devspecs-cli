package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

type threadSetOptions struct {
	Name   string
	After  []string
	AsJSON bool
}

type threadRemoveOptions struct {
	AsJSON bool
}

type threadMutationAuthority struct {
	Location            threadOwnerLocation
	RepoManifest        taskManifest
	WorkspaceValidation workspaceThreadValidationContext
	Tasks               []threadTaskSnapshot
}

type threadMutationOutput struct {
	Owner             threadStatusOwner `json:"owner"`
	Action            string            `json:"action"`
	Thread            string            `json:"thread"`
	Revision          int               `json:"revision"`
	DefinitionPath    string            `json:"definition_path"`
	ProjectionStatus  string            `json:"projection_status"`
	ProjectionWarning string            `json:"projection_warning,omitempty"`
}

func newThreadSetCmd(parent *threadStatusOptions) *cobra.Command {
	var opts threadSetOptions
	cmd := &cobra.Command{
		Use:   "set <owner> <key> <target>...",
		Short: "Create or replace one named execution thread",
		Long: `Create or replace one lane for task:<task-id> or change:<change-id>.

Repo-owned targets use a task target such as F01. Workspace-owned targets use
repo:target when the repo has one linked task, or repo:task:target otherwise.`,
		Args: cobra.MinimumNArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			started := time.Now()
			err := runThreadSet(cmd, args[0], args[1], args[2:], *parent, opts)
			telemetry.RecordCommand("thread_set", err == nil, time.Since(started), map[string]any{
				"json":         opts.AsJSON,
				"target_count": len(args) - 2,
			})
			return err
		},
	}
	cmd.Flags().StringVar(&opts.Name, "name", "", "Human-readable thread name")
	cmd.Flags().StringSliceVar(&opts.After, "after", nil, "Thread keys that must complete first")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func newThreadRemoveCmd(parent *threadStatusOptions) *cobra.Command {
	var opts threadRemoveOptions
	cmd := &cobra.Command{
		Use:   "remove <owner> <key>",
		Short: "Remove an unstarted named execution thread",
		Long:  "Remove one lane from task:<task-id> or change:<change-id> while checkpoint history still permits the mutation.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			started := time.Now()
			err := runThreadRemove(cmd, args[0], args[1], *parent, opts)
			telemetry.RecordCommand("thread_remove", err == nil, time.Since(started), map[string]any{
				"json": opts.AsJSON,
			})
			return err
		},
	}
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	return cmd
}

func runThreadSet(cmd *cobra.Command, owner, key string, targets []string, parent threadStatusOptions, opts threadSetOptions) error {
	location, err := resolveThreadOwnerArtifactLocation(cmd, owner, parent)
	if err != nil {
		return err
	}
	authority, err := loadThreadMutationAuthority(location)
	if err != nil {
		return err
	}
	references, err := resolveThreadTargetReferences(authority, targets)
	if err != nil {
		return err
	}
	definition, err := readOrCreateThreadDefinition(authority)
	if err != nil {
		return err
	}
	lane := threadDefinitionLane{
		Key:     strings.TrimSpace(key),
		Name:    strings.TrimSpace(opts.Name),
		Targets: references,
		After:   normalizeList(opts.After),
	}
	replaced := false
	for index := range definition.Threads {
		if strings.EqualFold(definition.Threads[index].Key, lane.Key) {
			definition.Threads[index] = lane
			replaced = true
			break
		}
	}
	if !replaced {
		definition.Threads = append(definition.Threads, lane)
	}
	if err := publishThreadMutation(cmd, authority, definition); err != nil {
		return err
	}
	out := newThreadMutationOutput(authority.Location, "set", lane.Key, definition.Revision)
	out.ProjectionWarning = refreshThreadProjection(authority.Location)
	if out.ProjectionWarning == "" {
		out.ProjectionStatus = "refreshed"
	} else {
		out.ProjectionStatus = "deferred"
	}
	return writeThreadMutationOutput(cmd, out, opts.AsJSON)
}

func runThreadRemove(cmd *cobra.Command, owner, key string, parent threadStatusOptions, opts threadRemoveOptions) error {
	location, err := resolveThreadOwnerLocation(cmd, owner, parent)
	if err != nil {
		return err
	}
	authority, err := loadThreadMutationAuthority(location)
	if err != nil {
		return err
	}
	definition, err := readThreadDefinition(location.DefinitionPath)
	if err != nil {
		return err
	}
	removed := false
	remaining := make([]threadDefinitionLane, 0, len(definition.Threads)-1)
	for _, lane := range definition.Threads {
		if strings.EqualFold(lane.Key, strings.TrimSpace(key)) {
			removed = true
			continue
		}
		remaining = append(remaining, lane)
	}
	if !removed {
		return fmt.Errorf("thread %q not found", strings.TrimSpace(key))
	}
	definition.Revision++
	definition.Threads = remaining
	if err := publishThreadMutation(cmd, authority, definition); err != nil {
		return err
	}
	out := newThreadMutationOutput(authority.Location, "remove", strings.TrimSpace(key), definition.Revision)
	out.ProjectionWarning = refreshThreadProjection(authority.Location)
	if out.ProjectionWarning == "" {
		out.ProjectionStatus = "refreshed"
	} else {
		out.ProjectionStatus = "deferred"
	}
	return writeThreadMutationOutput(cmd, out, opts.AsJSON)
}

func loadThreadMutationAuthority(location threadOwnerLocation) (threadMutationAuthority, error) {
	authority := threadMutationAuthority{Location: location}
	switch location.Kind {
	case threadOwnerTask:
		manifest, err := readTaskManifest(filepath.Join(location.TaskWorkspace, taskManifestFilename))
		if err != nil {
			return authority, err
		}
		events, err := readTaskCheckpointEvents(location.TaskWorkspace, manifest.TaskID)
		if err != nil {
			return authority, err
		}
		authority.RepoManifest = manifest
		authority.Tasks = []threadTaskSnapshot{{
			RepoRoot: location.RepoRoot, Workspace: location.TaskWorkspace, Manifest: manifest, Events: events,
		}}
		return authority, nil
	case threadOwnerWorkspaceChange:
		return loadWorkspaceThreadMutationAuthority(authority)
	default:
		return authority, fmt.Errorf("unsupported thread owner kind %q", location.Kind)
	}
}

func loadWorkspaceThreadMutationAuthority(authority threadMutationAuthority) (threadMutationAuthority, error) {
	location := authority.Location
	manifest, err := readWorkspaceManifest(location.WorkspaceRoot)
	if err != nil {
		return authority, err
	}
	_, change, body, err := findWorkspaceChange(location.WorkspaceRoot, manifest, location.ChangeID)
	if err != nil {
		return authority, err
	}
	validation := workspaceThreadValidationContext{
		Manifest: manifest, Change: change, Links: parseWorkspaceChangeRepoSlices(body),
		TaskManifests: make(map[string]taskManifest), RepoDefinitionExists: make(map[string]bool),
	}
	seen := make(map[string]bool)
	for _, link := range validation.Links {
		repo, exists := manifest.Repos[link.RepoAlias]
		if !exists {
			return authority, fmt.Errorf("workspace change links unknown repo alias %q", link.RepoAlias)
		}
		repoRoot := workspaceRelativeAbs(location.WorkspaceRoot, repo.Path)
		taskWorkspace, taskManifest, _, err := readThreadTaskManifest(repoRoot, link.TaskID)
		if err != nil {
			return authority, err
		}
		key := workspaceThreadTaskKey(link.RepoAlias, taskManifest.TaskID)
		validation.TaskManifests[key] = taskManifest
		if _, err := os.Stat(repoThreadDefinitionPath(taskWorkspace)); err == nil {
			validation.RepoDefinitionExists[key] = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return authority, err
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		events, err := readTaskCheckpointEvents(taskWorkspace, taskManifest.TaskID)
		if err != nil {
			return authority, err
		}
		authority.Tasks = append(authority.Tasks, threadTaskSnapshot{
			RepoAlias: link.RepoAlias, RepoRoot: repoRoot, Workspace: taskWorkspace, Manifest: taskManifest, Events: events,
		})
	}
	authority.WorkspaceValidation = validation
	return authority, nil
}

func readOrCreateThreadDefinition(authority threadMutationAuthority) (threadDefinition, error) {
	definition, err := readThreadDefinition(authority.Location.DefinitionPath)
	if err == nil {
		definition.Revision++
		return definition, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return threadDefinition{}, err
	}
	definition = threadDefinition{SchemaVersion: threadDefinitionSchemaVersion, Revision: 1}
	if authority.Location.Kind == threadOwnerTask {
		definition.Owner = threadDefinitionOwner{Kind: threadOwnerTask, TaskID: authority.RepoManifest.TaskID}
	} else {
		definition.Owner = threadDefinitionOwner{
			Kind: threadOwnerWorkspaceChange, WorkspaceID: authority.WorkspaceValidation.Manifest.ID,
			ChangeID: authority.WorkspaceValidation.Change.ID,
		}
	}
	return definition, nil
}

func resolveThreadTargetReferences(authority threadMutationAuthority, targets []string) ([]threadTargetReference, error) {
	references := make([]threadTargetReference, 0, len(targets))
	for _, target := range targets {
		reference, err := resolveThreadTargetReference(authority, strings.TrimSpace(target))
		if err != nil {
			return nil, err
		}
		references = append(references, reference)
	}
	return references, nil
}

func resolveThreadTargetReference(authority threadMutationAuthority, target string) (threadTargetReference, error) {
	if target == "" {
		return threadTargetReference{}, fmt.Errorf("thread target is empty")
	}
	if authority.Location.Kind == threadOwnerTask {
		if strings.Contains(target, ":") {
			return threadTargetReference{}, fmt.Errorf("repo thread target %q must be an unqualified task target", target)
		}
		return threadTargetReference{Target: target}, nil
	}
	parts := strings.Split(target, ":")
	switch len(parts) {
	case 2:
		repoAlias := strings.TrimSpace(parts[0])
		taskID, err := uniqueWorkspaceThreadTask(authority.WorkspaceValidation.Links, repoAlias)
		if err != nil {
			return threadTargetReference{}, err
		}
		return threadTargetReference{Repo: repoAlias, Task: taskID, Target: strings.TrimSpace(parts[1])}, nil
	case 3:
		return threadTargetReference{Repo: strings.TrimSpace(parts[0]), Task: strings.TrimSpace(parts[1]), Target: strings.TrimSpace(parts[2])}, nil
	default:
		return threadTargetReference{}, fmt.Errorf("workspace thread target %q must use repo:target or repo:task:target", target)
	}
}

func uniqueWorkspaceThreadTask(links []workspaceChangeRepoSlice, repoAlias string) (string, error) {
	taskID := ""
	for _, link := range links {
		if !strings.EqualFold(link.RepoAlias, repoAlias) {
			continue
		}
		if taskID != "" && !strings.EqualFold(taskID, link.TaskID) {
			return "", fmt.Errorf("repo %q links multiple tasks; use repo:task:target", repoAlias)
		}
		taskID = link.TaskID
	}
	if taskID == "" {
		return "", fmt.Errorf("workspace change has no linked task for repo %q", repoAlias)
	}
	return taskID, nil
}

func publishThreadMutation(cmd *cobra.Command, authority threadMutationAuthority, definition threadDefinition) error {
	history, err := checkpointedThreadTargets(authority.Location.Kind, authority.Tasks)
	if err != nil {
		return err
	}
	if authority.Location.Kind == threadOwnerTask {
		return publishRepoThreadDefinition(cmd.Context(), authority.Location.TaskWorkspace, definition, authority.RepoManifest, history)
	}
	return publishWorkspaceThreadDefinition(cmd.Context(), authority.Location.WorkspaceRoot, definition, authority.WorkspaceValidation, history)
}

func checkpointedThreadTargets(ownerKind string, tasks []threadTaskSnapshot) (map[string]bool, error) {
	history := make(map[string]bool)
	for _, task := range tasks {
		for _, event := range task.Events {
			target := firstNonEmptyTaskString(event.Record.Target, event.Record.Slice)
			if target == "" || isTaskSeriesTarget(task.Manifest, target) {
				continue
			}
			slice, err := taskSliceForCheckpoint(task.Manifest, target)
			if err != nil {
				return nil, fmt.Errorf("resolve checkpointed target %s in task %s: %w", target, task.Manifest.TaskID, err)
			}
			reference := threadTargetReference{Repo: task.RepoAlias, Task: task.Manifest.TaskID, Target: taskCheckpointParentSlice(slice)}
			if ownerKind == threadOwnerTask {
				reference.Repo = ""
				reference.Task = ""
			}
			history[threadTargetIdentity(ownerKind, reference)] = true
		}
	}
	return history, nil
}

func refreshThreadProjection(location threadOwnerLocation) string {
	db, err := openDB()
	if err != nil {
		return err.Error()
	}
	defer db.Close()
	var prior *store.ThreadProjection
	projection, found, err := db.GetThreadProjection(location.OwnerID)
	if err == nil && found {
		prior = &projection
	}
	if err != nil {
		return err.Error()
	}
	snapshot, _, err := loadThreadOwnerSnapshot(location, prior)
	if err != nil {
		return err.Error()
	}
	projection, err = threadProjectionFromSnapshot(snapshot)
	if err == nil {
		err = db.ReplaceThreadProjection(projection)
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func newThreadMutationOutput(location threadOwnerLocation, action, key string, revision int) threadMutationOutput {
	return threadMutationOutput{
		Owner: threadStatusOwner{
			Kind: location.Kind, TaskID: location.TaskID, WorkspaceID: location.WorkspaceID,
			ChangeID: location.ChangeID, RepoRoot: location.RepoRoot, WorkspaceRoot: location.WorkspaceRoot,
		},
		Action: action, Thread: key, Revision: revision, DefinitionPath: location.DefinitionPath,
	}
}

func writeThreadMutationOutput(cmd *cobra.Command, out threadMutationOutput, asJSON bool) error {
	if asJSON {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(out)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Thread %s: %s\n", out.Action, out.Thread)
	fmt.Fprintf(cmd.OutOrStdout(), "Definition: %s (revision %d)\n", out.DefinitionPath, out.Revision)
	if out.ProjectionWarning != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "Thread projection warning: durable definition was updated; projection refresh was deferred (%s)\n", out.ProjectionWarning)
	}
	return nil
}
