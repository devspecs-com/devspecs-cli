package commands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/gofrs/flock"
)

const (
	taskCheckpointSchemaVersion = 3
	taskCheckpointEventKind     = "task_checkpoint"
	taskMutationRetryDelay      = 100 * time.Millisecond
)

type taskCheckpointEvent struct {
	Record       taskCheckpointRecord
	JSONPath     string
	MarkdownPath string
}

type taskLifecycleConflict struct {
	Target            string   `json:"target"`
	HeadCheckpointIDs []string `json:"head_checkpoint_ids"`
}

type taskMutationLease struct {
	lock *flock.Flock
}

type taskMutationLeaseSet struct {
	leases []*taskMutationLease
}

func taskMutationIdentity(workspace string) (string, error) {
	absoluteWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve task workspace for mutation: %w", err)
	}
	identity := filepath.Clean(absoluteWorkspace)
	if resolvedWorkspace, resolveErr := filepath.EvalSymlinks(identity); resolveErr == nil {
		identity = filepath.Clean(resolvedWorkspace)
	}
	if runtime.GOOS == "windows" {
		identity = strings.ToLower(identity)
	}
	return identity, nil
}

func acquireTaskMutation(ctx context.Context, workspace string) (*taskMutationLease, error) {
	identity, err := taskMutationIdentity(workspace)
	if err != nil {
		return nil, err
	}
	return acquireTaskMutationIdentity(ctx, identity)
}

func acquireTaskMutationIdentity(ctx context.Context, identity string) (*taskMutationLease, error) {
	home, err := config.HomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve DevSpecs home for task mutation: %w", err)
	}
	digest := sha256.Sum256([]byte(identity))
	lockDir := filepath.Join(home, "locks", "tasks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, fmt.Errorf("create task mutation lock directory: %w", err)
	}
	lockPath := filepath.Join(lockDir, hex.EncodeToString(digest[:])+".lock")
	fileLock := flock.New(lockPath)
	locked, err := fileLock.TryLockContext(ctx, taskMutationRetryDelay)
	if err != nil {
		_ = fileLock.Close()
		return nil, fmt.Errorf("wait for task mutation lock: %w", err)
	}
	if !locked {
		_ = fileLock.Close()
		return nil, fmt.Errorf("wait for task mutation lock: %w", ctx.Err())
	}
	return &taskMutationLease{lock: fileLock}, nil
}

func acquireTaskMutations(ctx context.Context, workspaces []string) (*taskMutationLeaseSet, error) {
	identities := make([]string, 0, len(workspaces))
	seen := make(map[string]bool, len(workspaces))
	for _, workspace := range workspaces {
		identity, err := taskMutationIdentity(workspace)
		if err != nil {
			return nil, err
		}
		if seen[identity] {
			continue
		}
		seen[identity] = true
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	set := &taskMutationLeaseSet{leases: make([]*taskMutationLease, 0, len(identities))}
	for _, identity := range identities {
		lease, err := acquireTaskMutationIdentity(ctx, identity)
		if err != nil {
			return nil, errors.Join(err, set.Release())
		}
		set.leases = append(set.leases, lease)
	}
	return set, nil
}

func (lease *taskMutationLease) Release() error {
	if lease == nil || lease.lock == nil {
		return nil
	}
	unlockErr := lease.lock.Unlock()
	closeErr := lease.lock.Close()
	lease.lock = nil
	return errors.Join(unlockErr, closeErr)
}

func (set *taskMutationLeaseSet) Release() error {
	if set == nil {
		return nil
	}
	var releaseErr error
	for index := len(set.leases) - 1; index >= 0; index-- {
		releaseErr = errors.Join(releaseErr, set.leases[index].Release())
	}
	set.leases = nil
	return releaseErr
}

func readTaskCheckpointEvents(workspace, taskID string) ([]taskCheckpointEvent, error) {
	checkpointDir := filepath.Join(workspace, "checkpoints")
	entries, err := os.ReadDir(checkpointDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read task checkpoint events: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	events := make([]taskCheckpointEvent, 0, len(entries))
	seenIDs := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		jsonPath := filepath.Join(checkpointDir, entry.Name())
		record, err := readTaskCheckpointRecord(jsonPath)
		if err != nil {
			return nil, err
		}
		if err := validateTaskCheckpointEventRecord(jsonPath, taskID, record); err != nil {
			return nil, err
		}
		checkpointKey := strings.ToLower(strings.TrimSpace(record.CheckpointID))
		if previousPath, exists := seenIDs[checkpointKey]; exists {
			return nil, fmt.Errorf("duplicate checkpoint event ID %q in %s and %s", record.CheckpointID, previousPath, jsonPath)
		}
		seenIDs[checkpointKey] = jsonPath
		markdownPath := strings.TrimSuffix(jsonPath, filepath.Ext(jsonPath)) + ".md"
		events = append(events, taskCheckpointEvent{
			Record:       record,
			JSONPath:     jsonPath,
			MarkdownPath: markdownPath,
		})
	}
	return events, nil
}

func validateTaskCheckpointEventRecord(path, taskID string, record taskCheckpointRecord) error {
	if record.SchemaVersion > taskCheckpointSchemaVersion {
		return fmt.Errorf("checkpoint event %s uses unsupported schema_version %d", path, record.SchemaVersion)
	}
	if record.SchemaVersion == taskCheckpointSchemaVersion && record.EventKind != taskCheckpointEventKind {
		return fmt.Errorf("checkpoint event %s has invalid event_kind %q", path, record.EventKind)
	}
	if record.SchemaVersion == taskCheckpointSchemaVersion {
		if strings.TrimSpace(record.CheckpointID) == "" {
			return fmt.Errorf("checkpoint event %s has no checkpoint_id", path)
		}
		if strings.TrimSpace(firstNonEmptyTaskString(record.Target, record.Slice)) == "" {
			return fmt.Errorf("checkpoint event %s has no target", path)
		}
		if !isAllowedValue(record.Stage, taskLifecycleStages) {
			return fmt.Errorf("checkpoint event %s has invalid stage %q", path, record.Stage)
		}
		if record.Decision != "" && !isAllowedValue(record.Decision, taskDecisions) {
			return fmt.Errorf("checkpoint event %s has invalid decision %q", path, record.Decision)
		}
		if _, err := time.Parse(time.RFC3339, record.CreatedAt); err != nil {
			return fmt.Errorf("checkpoint event %s has invalid created_at %q", path, record.CreatedAt)
		}
	}
	if !strings.EqualFold(strings.TrimSpace(record.TaskID), strings.TrimSpace(taskID)) {
		return fmt.Errorf("checkpoint event %s belongs to task %q, expected %q", path, record.TaskID, taskID)
	}
	return nil
}

func resolveTaskCheckpointHeads(events []taskCheckpointEvent) (map[string][]taskCheckpointEvent, error) {
	byTarget := make(map[string][]taskCheckpointEvent)
	for _, event := range events {
		target := strings.ToLower(strings.TrimSpace(firstNonEmptyTaskString(event.Record.Target, event.Record.Slice)))
		if target == "" {
			// Early structured checkpoints could carry evidence without a lifecycle target.
			// They remain evaluation input but cannot override task.json lifecycle state.
			continue
		}
		byTarget[target] = append(byTarget[target], event)
	}
	resolved := make(map[string][]taskCheckpointEvent, len(byTarget))
	for target, targetEvents := range byTarget {
		heads, err := resolveTaskCheckpointTargetHeads(targetEvents)
		if err != nil {
			return nil, fmt.Errorf("resolve checkpoint target %s: %w", target, err)
		}
		resolved[target] = heads
	}
	return resolved, nil
}

func resolveTaskCheckpointTargetHeads(events []taskCheckpointEvent) ([]taskCheckpointEvent, error) {
	sort.SliceStable(events, func(i, j int) bool {
		left := parseTaskCheckpointCreatedAt(events[i].Record.CreatedAt)
		right := parseTaskCheckpointCreatedAt(events[j].Record.CreatedAt)
		if !left.Equal(right) {
			return left.Before(right)
		}
		leftID := strings.ToLower(events[i].Record.CheckpointID)
		rightID := strings.ToLower(events[j].Record.CheckpointID)
		if leftID != rightID {
			return leftID < rightID
		}
		return events[i].JSONPath < events[j].JSONPath
	})
	byID := make(map[string]taskCheckpointEvent, len(events))
	for _, event := range events {
		byID[strings.ToLower(event.Record.CheckpointID)] = event
	}
	superseded := make(map[string]bool)
	edges := make(map[string][]string)
	previousLegacyID := ""
	for _, event := range events {
		id := strings.ToLower(event.Record.CheckpointID)
		if event.Record.SchemaVersion < taskCheckpointSchemaVersion {
			if previousLegacyID != "" {
				superseded[previousLegacyID] = true
				edges[id] = append(edges[id], previousLegacyID)
			}
			previousLegacyID = id
			continue
		}
		seenPredecessors := make(map[string]bool)
		for _, predecessorID := range event.Record.SupersedesCheckpointIDs {
			predecessorKey := strings.ToLower(strings.TrimSpace(predecessorID))
			if predecessorKey == "" || seenPredecessors[predecessorKey] {
				continue
			}
			if predecessorKey == id {
				return nil, fmt.Errorf("checkpoint event %q cannot supersede itself", event.Record.CheckpointID)
			}
			if _, exists := byID[predecessorKey]; !exists {
				return nil, fmt.Errorf("checkpoint event %q supersedes unknown event %q", event.Record.CheckpointID, predecessorID)
			}
			seenPredecessors[predecessorKey] = true
			superseded[predecessorKey] = true
			edges[id] = append(edges[id], predecessorKey)
		}
	}
	if err := validateTaskCheckpointEventCycles(edges); err != nil {
		return nil, err
	}
	var heads []taskCheckpointEvent
	for _, event := range events {
		if !superseded[strings.ToLower(event.Record.CheckpointID)] {
			heads = append(heads, event)
		}
	}
	sort.SliceStable(heads, func(i, j int) bool {
		return strings.ToLower(heads[i].Record.CheckpointID) < strings.ToLower(heads[j].Record.CheckpointID)
	})
	return heads, nil
}

func validateTaskCheckpointEventCycles(edges map[string][]string) error {
	states := make(map[string]uint8, len(edges))
	var visit func(string) error
	visit = func(id string) error {
		switch states[id] {
		case 1:
			return fmt.Errorf("checkpoint event graph contains a cycle at %q", id)
		case 2:
			return nil
		}
		states[id] = 1
		for _, predecessor := range edges[id] {
			if err := visit(predecessor); err != nil {
				return err
			}
		}
		states[id] = 2
		return nil
	}
	for id := range edges {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func checkpointSupersedesForWrite(events []taskCheckpointEvent, target string, explicit []string) ([]string, error) {
	resolved, err := resolveTaskCheckpointHeads(events)
	if err != nil {
		return nil, err
	}
	heads := resolved[strings.ToLower(strings.TrimSpace(target))]
	expected := make([]string, 0, len(heads))
	for _, head := range heads {
		expected = append(expected, head.Record.CheckpointID)
	}
	sort.Slice(expected, func(i, j int) bool { return strings.ToLower(expected[i]) < strings.ToLower(expected[j]) })
	explicit = normalizeList(explicit)
	sort.Slice(explicit, func(i, j int) bool { return strings.ToLower(explicit[i]) < strings.ToLower(explicit[j]) })
	if len(explicit) == 0 {
		if len(expected) > 1 {
			return nil, fmt.Errorf("target %s has %d conflicting checkpoint heads (%s); resolve all heads with repeatable --supersedes", target, len(expected), strings.Join(expected, ", "))
		}
		return expected, nil
	}
	if !equalFoldedStrings(explicit, expected) {
		return nil, fmt.Errorf("--supersedes must name every current head for %s; expected %s", target, strings.Join(expected, ", "))
	}
	return expected, nil
}

func reconcileTaskManifestLifecycle(workspace string, manifest taskManifest) (taskManifest, error) {
	events, err := readTaskCheckpointEvents(workspace, manifest.TaskID)
	if err != nil || len(events) == 0 {
		return manifest, err
	}
	latestUpdatedAt, latestUpdatedErr := time.Parse(time.RFC3339, strings.TrimSpace(manifest.UpdatedAt))
	latestUpdatedText := manifest.UpdatedAt
	resolved, err := resolveTaskCheckpointHeads(events)
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
		if len(heads) != 1 {
			conflict := taskLifecycleConflict{Target: firstNonEmptyTaskString(heads[0].Record.Target, heads[0].Record.Slice)}
			for _, head := range heads {
				conflict.HeadCheckpointIDs = append(conflict.HeadCheckpointIDs, head.Record.CheckpointID)
			}
			manifest.LifecycleConflicts = append(manifest.LifecycleConflicts, conflict)
			manifest.LifecycleDiagnostics = append(manifest.LifecycleDiagnostics,
				fmt.Sprintf("target %s has conflicting checkpoint heads: %s", conflict.Target, strings.Join(conflict.HeadCheckpointIDs, ", ")),
			)
			continue
		}
		event := heads[0]
		record := event.Record
		targetID := firstNonEmptyTaskString(record.Target, record.Slice)
		priorStage, priorDecision := taskStateForTarget(manifest, targetID)
		priorCheckpointID := taskCheckpointIDForTarget(manifest, targetID)
		if priorStage != record.Stage || priorDecision != record.Decision || priorCheckpointID != record.CheckpointID {
			manifest.LifecycleDiagnostics = append(manifest.LifecycleDiagnostics,
				fmt.Sprintf("checkpoint event %s is authoritative for %s; task.json lifecycle projection disagreed", record.CheckpointID, targetID),
			)
		}
		createdAt := parseTaskCheckpointCreatedAt(record.CreatedAt)
		if latestUpdatedErr != nil || createdAt.After(latestUpdatedAt) {
			latestUpdatedAt = createdAt
			latestUpdatedText = createdAt.Format(time.RFC3339)
			latestUpdatedErr = nil
		}
		applyTaskTargetState(&manifest, targetID, record.Stage, record.Decision, createdAt)
		jsonRel := taskRelativePath(workspace, event.JSONPath)
		markdownRel := ""
		if _, err := os.Stat(event.MarkdownPath); err == nil {
			markdownRel = taskRelativePath(workspace, event.MarkdownPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return manifest, err
		} else {
			manifest.LifecycleDiagnostics = append(manifest.LifecycleDiagnostics,
				fmt.Sprintf("checkpoint event %s has no Markdown projection", record.CheckpointID),
			)
		}
		applyTaskTargetCheckpointRefs(&manifest, targetID, record.CheckpointID, markdownRel, jsonRel)
	}
	if latestUpdatedErr == nil {
		manifest.UpdatedAt = latestUpdatedText
	}
	return manifest, nil
}

func taskCheckpointIDForTarget(manifest taskManifest, targetID string) string {
	if isTaskSeriesTarget(manifest, targetID) {
		return manifest.LatestCheckpointID
	}
	for _, slice := range manifest.Artifacts.Slices {
		if strings.EqualFold(slice.ID, targetID) {
			return slice.LatestCheckpointID
		}
	}
	return ""
}
