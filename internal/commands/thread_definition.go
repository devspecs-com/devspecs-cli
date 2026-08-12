package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

const (
	threadDefinitionSchemaVersion = 1
	threadDefinitionFilename      = "threads.yaml"
	threadOwnerTask               = "task"
	threadOwnerWorkspaceChange    = "workspace_change"
)

type threadDefinition struct {
	SchemaVersion int                    `yaml:"schema_version" json:"schema_version"`
	Revision      int                    `yaml:"revision" json:"revision"`
	Owner         threadDefinitionOwner  `yaml:"owner" json:"owner"`
	Threads       []threadDefinitionLane `yaml:"threads" json:"threads"`
}

type threadDefinitionOwner struct {
	Kind        string `yaml:"kind" json:"kind"`
	TaskID      string `yaml:"task_id,omitempty" json:"task_id,omitempty"`
	WorkspaceID string `yaml:"workspace_id,omitempty" json:"workspace_id,omitempty"`
	ChangeID    string `yaml:"change_id,omitempty" json:"change_id,omitempty"`
}

type threadDefinitionLane struct {
	Key     string                  `yaml:"key" json:"key"`
	Name    string                  `yaml:"name,omitempty" json:"name,omitempty"`
	Targets []threadTargetReference `yaml:"targets" json:"targets"`
	After   []string                `yaml:"after,omitempty,flow" json:"after,omitempty"`
}

type threadTargetReference struct {
	Repo   string `yaml:"repo,omitempty" json:"repo,omitempty"`
	Task   string `yaml:"task,omitempty" json:"task,omitempty"`
	Target string `yaml:"target" json:"target"`
}

type workspaceThreadValidationContext struct {
	Manifest             workspaceManifest
	Change               workspaceChangeFrontmatter
	Links                []workspaceChangeRepoSlice
	TaskManifests        map[string]taskManifest
	RepoDefinitionExists map[string]bool
}

func repoThreadDefinitionPath(taskWorkspace string) string {
	return filepath.Join(taskWorkspace, threadDefinitionFilename)
}

func workspaceThreadDefinitionPath(workspaceRoot string, manifest workspaceManifest, changeID string) string {
	return filepath.Join(workspaceChangesDir(workspaceRoot, manifest), strings.TrimSpace(changeID)+".threads.yaml")
}

func readThreadDefinition(path string) (threadDefinition, error) {
	var definition threadDefinition
	data, err := os.ReadFile(path)
	if err != nil {
		return definition, fmt.Errorf("read thread definition: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&definition); err != nil {
		return definition, fmt.Errorf("parse thread definition %s: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err != nil {
			return definition, fmt.Errorf("parse thread definition %s: %w", path, err)
		}
		return definition, fmt.Errorf("parse thread definition %s: multiple YAML documents are not supported", path)
	}
	normalizeThreadDefinition(&definition)
	if err := validateThreadDefinitionEnvelope(definition); err != nil {
		return definition, fmt.Errorf("validate thread definition %s: %w", path, err)
	}
	return definition, nil
}

func writeThreadDefinition(path string, definition threadDefinition) error {
	normalizeThreadDefinition(&definition)
	if err := validateThreadDefinitionEnvelope(definition); err != nil {
		return err
	}
	data, err := yaml.Marshal(definition)
	if err != nil {
		return fmt.Errorf("marshal thread definition: %w", err)
	}
	if err := writeAtomicFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write thread definition: %w", err)
	}
	return nil
}

func publishRepoThreadDefinition(ctx context.Context, taskWorkspace string, definition threadDefinition, manifest taskManifest, checkpointedTargets map[string]bool) (err error) {
	path := repoThreadDefinitionPath(taskWorkspace)
	lease, err := acquireTaskMutation(ctx, taskWorkspace)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Release()) }()
	if err := validateRepoThreadDefinition(definition, manifest); err != nil {
		return err
	}
	return publishThreadDefinitionRevision(path, definition, checkpointedTargets)
}

func publishWorkspaceThreadDefinition(ctx context.Context, workspaceRoot string, definition threadDefinition, validation workspaceThreadValidationContext, checkpointedTargets map[string]bool) (err error) {
	path := workspaceThreadDefinitionPath(workspaceRoot, validation.Manifest, validation.Change.ID)
	lease, err := acquireTaskMutation(ctx, filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Release()) }()
	if err := validateWorkspaceThreadDefinition(definition, validation); err != nil {
		return err
	}
	return publishThreadDefinitionRevision(path, definition, checkpointedTargets)
}

func publishThreadDefinitionRevision(path string, definition threadDefinition, checkpointedTargets map[string]bool) error {
	previous, err := readThreadDefinition(path)
	if errors.Is(err, os.ErrNotExist) {
		if definition.Revision != 1 {
			return fmt.Errorf("new thread definition revision must be 1")
		}
		return writeThreadDefinition(path, definition)
	}
	if err != nil {
		return err
	}
	if err := validateThreadDefinitionMutation(previous, definition, checkpointedTargets); err != nil {
		return err
	}
	return writeThreadDefinition(path, definition)
}

func normalizeThreadDefinition(definition *threadDefinition) {
	if definition == nil {
		return
	}
	definition.Owner.Kind = strings.ToLower(strings.TrimSpace(definition.Owner.Kind))
	definition.Owner.TaskID = strings.TrimSpace(definition.Owner.TaskID)
	definition.Owner.WorkspaceID = strings.TrimSpace(definition.Owner.WorkspaceID)
	definition.Owner.ChangeID = strings.TrimSpace(definition.Owner.ChangeID)
	for laneIndex := range definition.Threads {
		lane := &definition.Threads[laneIndex]
		lane.Key = strings.TrimSpace(lane.Key)
		lane.Name = strings.TrimSpace(lane.Name)
		for targetIndex := range lane.Targets {
			lane.Targets[targetIndex].Repo = strings.TrimSpace(lane.Targets[targetIndex].Repo)
			lane.Targets[targetIndex].Task = strings.TrimSpace(lane.Targets[targetIndex].Task)
			lane.Targets[targetIndex].Target = strings.TrimSpace(lane.Targets[targetIndex].Target)
		}
		for dependencyIndex := range lane.After {
			lane.After[dependencyIndex] = strings.TrimSpace(lane.After[dependencyIndex])
		}
	}
}

func validateThreadDefinitionEnvelope(definition threadDefinition) error {
	if definition.SchemaVersion != threadDefinitionSchemaVersion {
		return fmt.Errorf("unsupported thread definition schema_version %d; expected %d", definition.SchemaVersion, threadDefinitionSchemaVersion)
	}
	if definition.Revision < 1 {
		return fmt.Errorf("thread definition revision must be at least 1")
	}
	switch definition.Owner.Kind {
	case threadOwnerTask:
		if definition.Owner.TaskID == "" {
			return fmt.Errorf("task thread owner requires task_id")
		}
		if definition.Owner.WorkspaceID != "" || definition.Owner.ChangeID != "" {
			return fmt.Errorf("task thread owner cannot include workspace_id or change_id")
		}
	case threadOwnerWorkspaceChange:
		if definition.Owner.WorkspaceID == "" || definition.Owner.ChangeID == "" {
			return fmt.Errorf("workspace_change thread owner requires workspace_id and change_id")
		}
		if definition.Owner.TaskID != "" {
			return fmt.Errorf("workspace_change thread owner cannot include task_id")
		}
	default:
		return fmt.Errorf("invalid thread owner kind %q", definition.Owner.Kind)
	}
	if len(definition.Threads) == 0 {
		return fmt.Errorf("thread definition has no threads")
	}
	return validateThreadDefinitionGraph(definition, nil)
}

func validateRepoThreadDefinition(definition threadDefinition, manifest taskManifest) error {
	normalizeThreadDefinition(&definition)
	if err := validateThreadDefinitionEnvelope(definition); err != nil {
		return err
	}
	if definition.Owner.Kind != threadOwnerTask {
		return fmt.Errorf("repo thread definition owner kind must be task")
	}
	if !strings.EqualFold(definition.Owner.TaskID, manifest.TaskID) {
		return fmt.Errorf("repo thread owner task_id %q does not match task %q", definition.Owner.TaskID, manifest.TaskID)
	}
	if strings.TrimSpace(manifest.ParentChange) != "" {
		return fmt.Errorf("task %q is linked to workspace change %q; that change owns its thread graph", manifest.TaskID, manifest.ParentChange)
	}
	return validateThreadDefinitionGraph(definition, func(reference threadTargetReference) error {
		if reference.Repo != "" || reference.Task != "" {
			return fmt.Errorf("repo task target %q cannot include repo or task qualifiers", reference.Target)
		}
		return validateThreadTargetInManifest(reference.Target, manifest)
	})
}

func validateWorkspaceThreadDefinition(definition threadDefinition, context workspaceThreadValidationContext) error {
	normalizeThreadDefinition(&definition)
	if err := validateThreadDefinitionEnvelope(definition); err != nil {
		return err
	}
	if definition.Owner.Kind != threadOwnerWorkspaceChange {
		return fmt.Errorf("workspace thread definition owner kind must be workspace_change")
	}
	if !strings.EqualFold(definition.Owner.WorkspaceID, context.Manifest.ID) {
		return fmt.Errorf("workspace thread owner workspace_id %q does not match workspace %q", definition.Owner.WorkspaceID, context.Manifest.ID)
	}
	if !strings.EqualFold(definition.Owner.ChangeID, context.Change.ID) {
		return fmt.Errorf("workspace thread owner change_id %q does not match change %q", definition.Owner.ChangeID, context.Change.ID)
	}
	links := make(map[string]workspaceChangeRepoSlice, len(context.Links))
	for _, link := range context.Links {
		key := workspaceThreadTaskKey(link.RepoAlias, link.TaskID)
		links[key] = link
		if context.RepoDefinitionExists[key] {
			return fmt.Errorf("linked task %q has competing repo-local thread authority", link.TaskID)
		}
	}
	return validateThreadDefinitionGraph(definition, func(reference threadTargetReference) error {
		if reference.Repo == "" || reference.Task == "" {
			return fmt.Errorf("workspace target %q requires repo and task", reference.Target)
		}
		if _, exists := context.Manifest.Repos[reference.Repo]; !exists {
			return fmt.Errorf("workspace target %s:%s references unknown repo alias %q", reference.Task, reference.Target, reference.Repo)
		}
		key := workspaceThreadTaskKey(reference.Repo, reference.Task)
		if _, exists := links[key]; !exists {
			return fmt.Errorf("workspace target %s:%s is not linked to change %s", reference.Repo, reference.Target, context.Change.ID)
		}
		manifest, exists := context.TaskManifests[key]
		if !exists {
			return fmt.Errorf("workspace target %s:%s has no readable linked task manifest", reference.Repo, reference.Target)
		}
		if !strings.EqualFold(manifest.WorkspaceID, context.Manifest.ID) ||
			!strings.EqualFold(manifest.ParentChange, context.Change.ID) ||
			!strings.EqualFold(manifest.RepoAlias, reference.Repo) {
			return fmt.Errorf("linked task %q does not point back to workspace change %s in repo %s", reference.Task, context.Change.ID, reference.Repo)
		}
		return validateThreadTargetInManifest(reference.Target, manifest)
	})
}

func validateThreadDefinitionGraph(definition threadDefinition, validateTarget func(threadTargetReference) error) error {
	lanes := make(map[string]threadDefinitionLane, len(definition.Threads))
	assigned := make(map[string]string)
	for _, lane := range definition.Threads {
		if err := validateThreadKey(lane.Key); err != nil {
			return err
		}
		key := strings.ToLower(lane.Key)
		if _, exists := lanes[key]; exists {
			return fmt.Errorf("duplicate thread key %q", lane.Key)
		}
		if err := validateThreadName(lane.Key, lane.Name); err != nil {
			return err
		}
		if len(lane.Targets) == 0 {
			return fmt.Errorf("thread %q has no targets", lane.Key)
		}
		for _, reference := range lane.Targets {
			if reference.Target == "" {
				return fmt.Errorf("thread %q contains an empty target", lane.Key)
			}
			identity := threadTargetIdentity(definition.Owner.Kind, reference)
			if owner, exists := assigned[identity]; exists {
				return fmt.Errorf("target %q is assigned to both %q and %q", threadTargetLabel(reference), owner, lane.Key)
			}
			if validateTarget != nil {
				if err := validateTarget(reference); err != nil {
					return fmt.Errorf("thread %q: %w", lane.Key, err)
				}
			}
			assigned[identity] = lane.Key
		}
		lanes[key] = lane
	}
	for _, lane := range definition.Threads {
		seen := make(map[string]struct{}, len(lane.After))
		for _, dependency := range lane.After {
			dependencyKey := strings.ToLower(dependency)
			if dependencyKey == "" {
				return fmt.Errorf("thread %q contains an empty dependency", lane.Key)
			}
			if dependencyKey == strings.ToLower(lane.Key) {
				return fmt.Errorf("thread %q cannot depend on itself", lane.Key)
			}
			if _, exists := lanes[dependencyKey]; !exists {
				return fmt.Errorf("thread %q depends on unknown thread %q", lane.Key, dependency)
			}
			if _, exists := seen[dependencyKey]; exists {
				return fmt.Errorf("thread %q repeats dependency %q", lane.Key, dependency)
			}
			seen[dependencyKey] = struct{}{}
		}
	}
	return validateThreadDefinitionCycles(definition.Threads)
}

func validateThreadTargetInManifest(target string, manifest taskManifest) error {
	if isTaskSeriesTarget(manifest, target) {
		return fmt.Errorf("series closeout target %q cannot be assigned to a thread", target)
	}
	slice, err := taskSliceForCheckpoint(manifest, target)
	if err != nil {
		return err
	}
	if strings.TrimSpace(slice.ParentID) != "" || taskSliceIsIteration(slice) {
		return fmt.Errorf("follow-up target %q inherits %q's thread and cannot be assigned directly", slice.ID, taskCheckpointParentSlice(slice))
	}
	return nil
}

func validateThreadKey(key string) error {
	if key == "" {
		return fmt.Errorf("thread key is empty")
	}
	for _, value := range key {
		if unicode.IsLetter(value) || unicode.IsDigit(value) || value == '-' || value == '_' || value == '.' {
			continue
		}
		return fmt.Errorf("thread key %q contains unsupported character %q", key, value)
	}
	return nil
}

func validateThreadName(key, name string) error {
	for _, value := range name {
		if unicode.IsControl(value) {
			return fmt.Errorf("thread %q name contains a control character", key)
		}
	}
	return nil
}

func validateThreadDefinitionCycles(lanes []threadDefinitionLane) error {
	definitions := make(map[string]threadDefinitionLane, len(lanes))
	for _, lane := range lanes {
		definitions[strings.ToLower(lane.Key)] = lane
	}
	states := make(map[string]uint8, len(lanes))
	var visit func(string) error
	visit = func(key string) error {
		switch states[key] {
		case 1:
			return fmt.Errorf("thread dependency graph contains a cycle at %q", definitions[key].Key)
		case 2:
			return nil
		}
		states[key] = 1
		for _, dependency := range definitions[key].After {
			if err := visit(strings.ToLower(dependency)); err != nil {
				return err
			}
		}
		states[key] = 2
		return nil
	}
	for _, lane := range lanes {
		if err := visit(strings.ToLower(lane.Key)); err != nil {
			return err
		}
	}
	return nil
}

func validateThreadDefinitionMutation(before, after threadDefinition, checkpointedTargets map[string]bool) error {
	if after.Revision != before.Revision+1 {
		return fmt.Errorf("thread definition revision must advance from %d to %d", before.Revision, before.Revision+1)
	}
	if before.Owner != after.Owner {
		return fmt.Errorf("thread definition owner cannot change")
	}
	beforeLanes := make(map[string]threadDefinitionLane, len(before.Threads))
	afterLanes := make(map[string]threadDefinitionLane, len(after.Threads))
	for _, lane := range before.Threads {
		beforeLanes[strings.ToLower(lane.Key)] = lane
	}
	for _, lane := range after.Threads {
		afterLanes[strings.ToLower(lane.Key)] = lane
	}
	for key, previous := range beforeLanes {
		if !threadLaneHasHistory(before.Owner.Kind, previous, checkpointedTargets) {
			continue
		}
		next, exists := afterLanes[key]
		if !exists {
			return fmt.Errorf("thread %q has checkpoint history and cannot be removed", previous.Key)
		}
		if previous.Key != next.Key {
			return fmt.Errorf("thread %q key is pinned by checkpoint history", previous.Key)
		}
		if !equalFoldedStrings(previous.After, next.After) {
			return fmt.Errorf("thread %q dependencies are pinned by checkpoint history", previous.Key)
		}
		if !threadTargetsHavePrefix(before.Owner.Kind, previous.Targets, next.Targets) {
			return fmt.Errorf("thread %q targets with checkpoint history may only be extended", previous.Key)
		}
	}
	for key, previous := range beforeLanes {
		if _, exists := afterLanes[key]; exists {
			continue
		}
		for _, dependent := range before.Threads {
			if !containsFoldedString(dependent.After, previous.Key) {
				continue
			}
			if threadLaneHasHistory(before.Owner.Kind, dependent, checkpointedTargets) {
				return fmt.Errorf("thread %q cannot be removed because started thread %q depends on it", previous.Key, dependent.Key)
			}
		}
	}
	return nil
}

func threadLaneHasHistory(ownerKind string, lane threadDefinitionLane, checkpointedTargets map[string]bool) bool {
	for _, target := range lane.Targets {
		if checkpointedTargets[threadTargetIdentity(ownerKind, target)] {
			return true
		}
	}
	return false
}

func threadTargetsHavePrefix(ownerKind string, before, after []threadTargetReference) bool {
	if len(after) < len(before) {
		return false
	}
	for index := range before {
		if threadTargetIdentity(ownerKind, before[index]) != threadTargetIdentity(ownerKind, after[index]) {
			return false
		}
	}
	return true
}

func threadTargetIdentity(ownerKind string, reference threadTargetReference) string {
	if ownerKind == threadOwnerTask {
		return strings.ToLower(reference.Target)
	}
	return strings.ToLower(reference.Repo + "\x00" + reference.Task + "\x00" + reference.Target)
}

func threadTargetLabel(reference threadTargetReference) string {
	if reference.Repo == "" && reference.Task == "" {
		return reference.Target
	}
	return reference.Repo + ":" + reference.Task + ":" + reference.Target
}

func workspaceThreadTaskKey(repoAlias, taskID string) string {
	return strings.ToLower(strings.TrimSpace(repoAlias) + "\x00" + strings.TrimSpace(taskID))
}

func equalFoldedStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !strings.EqualFold(strings.TrimSpace(left[index]), strings.TrimSpace(right[index])) {
			return false
		}
	}
	return true
}

func containsFoldedString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func writeAtomicFile(path string, data []byte, defaultMode os.FileMode) error {
	mode := defaultMode
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func writeNewAtomicFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := publishNewFile(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("refusing to overwrite existing task artifact")
		}
		return err
	}
	return nil
}
