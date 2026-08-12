package commands

import (
	"fmt"
	"sort"
	"strings"
)

const (
	threadStateReady     = "ready"
	threadStateActive    = "active"
	threadStateWaiting   = "waiting"
	threadStateBlocked   = "blocked"
	threadStateCompleted = "completed"
)

type threadOwnerSnapshot struct {
	OwnerID        string
	Kind           string
	TaskID         string
	WorkspaceID    string
	ChangeID       string
	RepoRoot       string
	WorkspaceRoot  string
	DefinitionPath string
	Definition     threadDefinition
	Links          []threadSnapshotLink
	Tasks          []threadTaskSnapshot
	Sources        []threadSourceSnapshot
}

type threadSnapshotLink struct {
	Position      int
	RepoAlias     string
	TaskID        string
	Target        string
	Name          string
	Status        string
	RepoRoot      string
	TaskWorkspace string
}

type threadTaskSnapshot struct {
	RepoAlias    string
	RepoRoot     string
	Workspace    string
	ManifestPath string
	Manifest     taskManifest
	Events       []taskCheckpointEvent
}

type threadSourceSnapshot struct {
	Kind        string
	Key         string
	Path        string
	SizeBytes   int64
	ModifiedNS  int64
	ContentHash string
}

type threadStatusOutput struct {
	Owner               threadStatusOwner    `json:"owner"`
	Threads             []threadStatusLane   `json:"threads"`
	ReadyThreads        []string             `json:"ready_threads"`
	ActiveThreads       []string             `json:"active_threads"`
	UnassignedTargets   []threadStatusTarget `json:"unassigned_targets"`
	StateSource         string               `json:"state_source"`
	ProjectionFreshness string               `json:"projection_freshness"`
	ProjectionRefreshed bool                 `json:"projection_refreshed"`
	ProjectionWarning   string               `json:"projection_warning,omitempty"`
}

type threadStatusOwner struct {
	Kind          string `json:"kind"`
	TaskID        string `json:"task_id,omitempty"`
	WorkspaceID   string `json:"workspace_id,omitempty"`
	ChangeID      string `json:"change_id,omitempty"`
	RepoRoot      string `json:"repo_root,omitempty"`
	WorkspaceRoot string `json:"workspace_root,omitempty"`
}

type threadStatusLane struct {
	Key           string               `json:"key"`
	Name          string               `json:"name,omitempty"`
	State         string               `json:"state"`
	After         []string             `json:"after"`
	Targets       []threadStatusTarget `json:"targets"`
	CurrentTarget *threadStatusTarget  `json:"current_target,omitempty"`
	Reasons       []string             `json:"reasons"`
}

type threadStatusTarget struct {
	RepoAlias        string   `json:"repo_alias,omitempty"`
	TaskID           string   `json:"task_id"`
	DefinitionTarget string   `json:"definition_target"`
	Target           string   `json:"target"`
	Title            string   `json:"title"`
	Stage            string   `json:"stage,omitempty"`
	Decision         string   `json:"decision,omitempty"`
	CheckpointID     string   `json:"checkpoint_id,omitempty"`
	Conflicted       bool     `json:"conflicted,omitempty"`
	ConflictHeads    []string `json:"conflict_heads,omitempty"`
	Completed        bool     `json:"completed"`
	Blocked          bool     `json:"blocked"`
}

func buildThreadStatus(snapshot threadOwnerSnapshot) (threadStatusOutput, error) {
	if err := validateThreadSnapshot(snapshot); err != nil {
		return threadStatusOutput{}, err
	}
	tasks, err := reconciledThreadTasks(snapshot.Tasks)
	if err != nil {
		return threadStatusOutput{}, err
	}
	out := threadStatusOutput{
		Owner: threadStatusOwner{
			Kind:          snapshot.Kind,
			TaskID:        snapshot.TaskID,
			WorkspaceID:   snapshot.WorkspaceID,
			ChangeID:      snapshot.ChangeID,
			RepoRoot:      snapshot.RepoRoot,
			WorkspaceRoot: snapshot.WorkspaceRoot,
		},
	}
	assigned := make(map[string]bool)
	for _, laneDefinition := range snapshot.Definition.Threads {
		lane := threadStatusLane{
			Key:     laneDefinition.Key,
			Name:    laneDefinition.Name,
			After:   append([]string(nil), laneDefinition.After...),
			Reasons: []string{},
		}
		lane.Targets = make([]threadStatusTarget, 0, len(laneDefinition.Targets))
		for _, reference := range laneDefinition.Targets {
			target, err := buildThreadStatusTarget(snapshot.Kind, reference, tasks)
			if err != nil {
				return threadStatusOutput{}, fmt.Errorf("thread %s: %w", laneDefinition.Key, err)
			}
			lane.Targets = append(lane.Targets, target)
			assigned[threadStatusTargetIdentity(target.RepoAlias, target.TaskID, target.DefinitionTarget)] = true
		}
		out.Threads = append(out.Threads, lane)
	}
	deriveThreadLaneStates(out.Threads)
	for index := range out.Threads {
		switch out.Threads[index].State {
		case threadStateReady:
			out.ReadyThreads = append(out.ReadyThreads, out.Threads[index].Key)
		case threadStateActive:
			out.ActiveThreads = append(out.ActiveThreads, out.Threads[index].Key)
		}
	}
	out.UnassignedTargets = unassignedThreadTargets(tasks, assigned)
	if out.Threads == nil {
		out.Threads = []threadStatusLane{}
	}
	if out.ReadyThreads == nil {
		out.ReadyThreads = []string{}
	}
	if out.ActiveThreads == nil {
		out.ActiveThreads = []string{}
	}
	if out.UnassignedTargets == nil {
		out.UnassignedTargets = []threadStatusTarget{}
	}
	return out, nil
}

func validateThreadSnapshot(snapshot threadOwnerSnapshot) error {
	if snapshot.Kind != threadOwnerTask && snapshot.Kind != threadOwnerWorkspaceChange {
		return fmt.Errorf("invalid thread snapshot owner kind %q", snapshot.Kind)
	}
	if snapshot.Definition.Owner.Kind != snapshot.Kind {
		return fmt.Errorf("thread snapshot owner and definition disagree")
	}
	if err := validateThreadDefinitionEnvelope(snapshot.Definition); err != nil {
		return err
	}
	if len(snapshot.Tasks) == 0 {
		return fmt.Errorf("thread snapshot has no tasks")
	}
	if snapshot.Kind == threadOwnerTask && len(snapshot.Tasks) != 1 {
		return fmt.Errorf("repo thread snapshot must contain exactly one task")
	}
	return nil
}

func reconciledThreadTasks(tasks []threadTaskSnapshot) (map[string]threadTaskSnapshot, error) {
	resolved := make(map[string]threadTaskSnapshot, len(tasks))
	for _, task := range tasks {
		manifest, err := reconcileThreadTaskLifecycle(task)
		if err != nil {
			return nil, fmt.Errorf("reconcile task %s: %w", task.Manifest.TaskID, err)
		}
		task.Manifest = manifest
		key := workspaceThreadTaskKey(task.RepoAlias, manifest.TaskID)
		if _, exists := resolved[key]; exists {
			return nil, fmt.Errorf("duplicate thread task %s:%s", task.RepoAlias, manifest.TaskID)
		}
		resolved[key] = task
	}
	return resolved, nil
}

func reconcileThreadTaskLifecycle(task threadTaskSnapshot) (taskManifest, error) {
	manifest := task.Manifest
	manifest.LifecycleConflicts = nil
	manifest.LifecycleDiagnostics = nil
	resolved, err := resolveTaskCheckpointHeads(task.Events)
	if err != nil {
		return manifest, err
	}
	var targets []string
	for target := range resolved {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	for _, target := range targets {
		heads := resolved[target]
		if len(heads) > 1 {
			conflict := taskLifecycleConflict{Target: firstNonEmptyTaskString(heads[0].Record.Target, heads[0].Record.Slice)}
			for _, head := range heads {
				conflict.HeadCheckpointIDs = append(conflict.HeadCheckpointIDs, head.Record.CheckpointID)
			}
			manifest.LifecycleConflicts = append(manifest.LifecycleConflicts, conflict)
			continue
		}
		if len(heads) == 0 {
			continue
		}
		event := heads[0]
		record := event.Record
		targetID := firstNonEmptyTaskString(record.Target, record.Slice)
		createdAt := parseTaskCheckpointCreatedAt(record.CreatedAt)
		applyTaskTargetState(&manifest, targetID, record.Stage, record.Decision, createdAt)
		applyTaskTargetCheckpointRefs(
			&manifest,
			targetID,
			record.CheckpointID,
			taskRelativePath(task.Workspace, event.MarkdownPath),
			taskRelativePath(task.Workspace, event.JSONPath),
		)
		applyThreadDurabilityRecord(&manifest, record, createdAt.Format("2006-01-02T15:04:05Z"))
	}
	return manifest, nil
}

func applyThreadDurabilityRecord(manifest *taskManifest, record taskCheckpointRecord, updatedAt string) {
	disposition := strings.ToLower(strings.TrimSpace(record.DurableRecord))
	if disposition == "" {
		return
	}
	manifest.Durability.Disposition = disposition
	manifest.Durability.Artifacts = append([]string(nil), record.DurableArtifacts...)
	manifest.Durability.DeferredTarget = ""
	if disposition == "deferred" {
		manifest.Durability.DeferredTarget = record.Next.RecommendedTarget
	}
	manifest.Durability.UpdatedAt = updatedAt
}

func buildThreadStatusTarget(ownerKind string, reference threadTargetReference, tasks map[string]threadTaskSnapshot) (threadStatusTarget, error) {
	repoAlias := reference.Repo
	taskID := reference.Task
	if ownerKind == threadOwnerTask {
		repoAlias = ""
		for _, task := range tasks {
			taskID = task.Manifest.TaskID
			break
		}
	}
	task, exists := tasks[workspaceThreadTaskKey(repoAlias, taskID)]
	if !exists {
		return threadStatusTarget{}, fmt.Errorf("task %s:%s is unavailable", repoAlias, taskID)
	}
	group, err := threadTargetProgressionGroup(task.Manifest, reference.Target)
	if err != nil {
		return threadStatusTarget{}, err
	}
	current, completed, blocked := currentThreadTarget(group)
	conflictHeads := threadTargetConflictHeads(task.Manifest, group)
	conflicted := len(conflictHeads) > 0
	if conflicted {
		blocked = true
		completed = false
	}
	return threadStatusTarget{
		RepoAlias:        repoAlias,
		TaskID:           task.Manifest.TaskID,
		DefinitionTarget: reference.Target,
		Target:           current.ID,
		Title:            current.Title,
		Stage:            current.Stage,
		Decision:         current.Decision,
		CheckpointID:     current.LatestCheckpointID,
		Conflicted:       conflicted,
		ConflictHeads:    conflictHeads,
		Completed:        completed,
		Blocked:          blocked,
	}, nil
}

func threadTargetProgressionGroup(manifest taskManifest, target string) ([]taskSliceArtifact, error) {
	for _, group := range taskSliceProgressionGroups(manifest) {
		if len(group) > 0 && strings.EqualFold(group[0].ID, target) {
			return group, nil
		}
	}
	return nil, fmt.Errorf("target %q is not a base slice in task %q", target, manifest.TaskID)
}

func currentThreadTarget(group []taskSliceArtifact) (taskSliceArtifact, bool, bool) {
	current := group[0]
	for _, target := range group {
		current = target
		if !taskTargetTerminal(target) {
			return current, false, false
		}
	}
	if taskDecisionAllowsSiblingAdvance(current) {
		return current, true, false
	}
	return current, false, true
}

func threadTargetConflictHeads(manifest taskManifest, group []taskSliceArtifact) []string {
	for _, conflict := range manifest.LifecycleConflicts {
		for _, target := range group {
			if strings.EqualFold(conflict.Target, target.ID) {
				return append([]string(nil), conflict.HeadCheckpointIDs...)
			}
		}
	}
	return nil
}

func deriveThreadLaneStates(lanes []threadStatusLane) {
	byKey := make(map[string]int, len(lanes))
	for index := range lanes {
		byKey[strings.ToLower(lanes[index].Key)] = index
		deriveThreadLaneOwnState(&lanes[index])
	}
	for index := range lanes {
		if lanes[index].State == threadStateCompleted || lanes[index].State == threadStateBlocked {
			continue
		}
		var waiting []string
		for _, dependency := range lanes[index].After {
			dependencyLane := lanes[byKey[strings.ToLower(dependency)]]
			if dependencyLane.State != threadStateCompleted {
				waiting = append(waiting, fmt.Sprintf("after %s (%s)", dependencyLane.Key, dependencyLane.State))
			}
		}
		if len(waiting) > 0 {
			lanes[index].State = threadStateWaiting
			lanes[index].Reasons = waiting
		}
	}
}

func deriveThreadLaneOwnState(lane *threadStatusLane) {
	for index := range lane.Targets {
		target := &lane.Targets[index]
		if target.Completed {
			continue
		}
		lane.CurrentTarget = target
		if target.Conflicted {
			lane.State = threadStateBlocked
			lane.Reasons = []string{fmt.Sprintf("%s has conflicting checkpoint heads: %s", threadStatusTargetAddress(*target), strings.Join(target.ConflictHeads, ", "))}
			return
		}
		if target.Blocked {
			lane.State = threadStateBlocked
			lane.Reasons = []string{fmt.Sprintf("%s ended with %s", threadStatusTargetAddress(*target), firstNonEmptyTaskString(target.Decision, target.Stage, "an unresolved gate"))}
			return
		}
		if threadTargetIsActive(*target) {
			lane.State = threadStateActive
			return
		}
		lane.State = threadStateReady
		return
	}
	lane.State = threadStateCompleted
}

func threadTargetIsActive(target threadStatusTarget) bool {
	switch strings.ToLower(strings.TrimSpace(target.Stage)) {
	case "started", "implemented", "validated":
		return true
	default:
		return false
	}
}

func unassignedThreadTargets(tasks map[string]threadTaskSnapshot, assigned map[string]bool) []threadStatusTarget {
	keys := make([]string, 0, len(tasks))
	for key := range tasks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var targets []threadStatusTarget
	for _, key := range keys {
		task := tasks[key]
		for _, group := range taskSliceProgressionGroups(task.Manifest) {
			if len(group) == 0 {
				continue
			}
			base := group[0]
			identity := threadStatusTargetIdentity(task.RepoAlias, task.Manifest.TaskID, base.ID)
			if assigned[identity] {
				continue
			}
			current, completed, blocked := currentThreadTarget(group)
			conflictHeads := threadTargetConflictHeads(task.Manifest, group)
			targets = append(targets, threadStatusTarget{
				RepoAlias:        task.RepoAlias,
				TaskID:           task.Manifest.TaskID,
				DefinitionTarget: base.ID,
				Target:           current.ID,
				Title:            current.Title,
				Stage:            current.Stage,
				Decision:         current.Decision,
				CheckpointID:     current.LatestCheckpointID,
				Conflicted:       len(conflictHeads) > 0,
				ConflictHeads:    conflictHeads,
				Completed:        completed && len(conflictHeads) == 0,
				Blocked:          blocked || len(conflictHeads) > 0,
			})
		}
	}
	return targets
}

func threadStatusTargetIdentity(repoAlias, taskID, target string) string {
	return strings.ToLower(strings.TrimSpace(repoAlias) + "\x00" + strings.TrimSpace(taskID) + "\x00" + strings.TrimSpace(target))
}

func threadStatusTargetAddress(target threadStatusTarget) string {
	if target.RepoAlias == "" {
		return target.Target
	}
	return target.RepoAlias + ":" + target.Target
}
