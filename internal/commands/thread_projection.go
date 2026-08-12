package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
)

const (
	threadSourceDefinition        = "definition"
	threadSourceTaskManifest      = "task_manifest"
	threadSourceWorkspaceManifest = "workspace_manifest"
	threadSourceWorkspaceChange   = "workspace_change"
	threadSourceCheckpointEvent   = "checkpoint_event"
)

type threadOwnerLocation struct {
	OwnerID        string
	Kind           string
	TaskID         string
	WorkspaceID    string
	ChangeID       string
	RepoRoot       string
	TaskWorkspace  string
	WorkspaceRoot  string
	DefinitionPath string
}

type threadProjectionLoadStats struct {
	EventsParsed int
	EventsReused int
}

type threadCheckpointLoad struct {
	path     string
	metadata threadSourceSnapshot
	record   taskCheckpointRecord
	reused   bool
	err      error
}

func repoThreadOwnerLocation(repoRoot, taskWorkspace string, manifest taskManifest) threadOwnerLocation {
	return threadOwnerLocation{
		OwnerID:        threadOwnerIdentity(threadOwnerTask, repoRoot, manifest.TaskID),
		Kind:           threadOwnerTask,
		TaskID:         manifest.TaskID,
		RepoRoot:       repoRoot,
		TaskWorkspace:  taskWorkspace,
		DefinitionPath: repoThreadDefinitionPath(taskWorkspace),
	}
}

func workspaceThreadOwnerLocation(workspaceRoot string, manifest workspaceManifest, changeID string) threadOwnerLocation {
	return threadOwnerLocation{
		OwnerID:        threadOwnerIdentity(threadOwnerWorkspaceChange, workspaceRoot, changeID),
		Kind:           threadOwnerWorkspaceChange,
		WorkspaceID:    manifest.ID,
		ChangeID:       changeID,
		WorkspaceRoot:  workspaceRoot,
		DefinitionPath: workspaceThreadDefinitionPath(workspaceRoot, manifest, changeID),
	}
}

func threadOwnerIdentity(kind, root, id string) string {
	identity := canonicalThreadPath(root)
	digest := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(kind)) + "\x00" + identity + "\x00" + strings.ToLower(strings.TrimSpace(id))))
	return "thread_" + hex.EncodeToString(digest[:])
}

func canonicalThreadPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err == nil {
		path = absolute
	}
	path = filepath.Clean(path)
	if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil {
		path = filepath.Clean(resolved)
	}
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

func loadThreadOwnerSnapshot(location threadOwnerLocation, prior *store.ThreadProjection) (threadOwnerSnapshot, threadProjectionLoadStats, error) {
	switch location.Kind {
	case threadOwnerTask:
		return loadRepoThreadOwnerSnapshot(location, prior)
	case threadOwnerWorkspaceChange:
		return loadWorkspaceThreadOwnerSnapshot(location, prior)
	default:
		return threadOwnerSnapshot{}, threadProjectionLoadStats{}, fmt.Errorf("unsupported thread owner kind %q", location.Kind)
	}
}

func loadRepoThreadOwnerSnapshot(location threadOwnerLocation, prior *store.ThreadProjection) (threadOwnerSnapshot, threadProjectionLoadStats, error) {
	definition, err := readThreadDefinition(location.DefinitionPath)
	if err != nil {
		return threadOwnerSnapshot{}, threadProjectionLoadStats{}, err
	}
	definitionSource, err := readThreadSource(location.DefinitionPath, threadSourceDefinition, "definition")
	if err != nil {
		return threadOwnerSnapshot{}, threadProjectionLoadStats{}, err
	}
	manifestPath := filepath.Join(location.TaskWorkspace, taskManifestFilename)
	manifest, err := readTaskManifest(manifestPath)
	if err != nil {
		return threadOwnerSnapshot{}, threadProjectionLoadStats{}, err
	}
	if err := validateRepoThreadDefinition(definition, manifest); err != nil {
		return threadOwnerSnapshot{}, threadProjectionLoadStats{}, fmt.Errorf("validate repo thread definition: %w", err)
	}
	manifestSource, err := readThreadSource(manifestPath, threadSourceTaskManifest, threadTaskSourceKey("", manifest.TaskID))
	if err != nil {
		return threadOwnerSnapshot{}, threadProjectionLoadStats{}, err
	}
	events, eventSources, stats, err := readThreadTaskEventsIncremental("", location.TaskWorkspace, manifest.TaskID, prior)
	if err != nil {
		return threadOwnerSnapshot{}, stats, err
	}
	snapshot := threadOwnerSnapshot{
		OwnerID:        location.OwnerID,
		Kind:           threadOwnerTask,
		TaskID:         manifest.TaskID,
		RepoRoot:       location.RepoRoot,
		DefinitionPath: location.DefinitionPath,
		Definition:     definition,
		Tasks: []threadTaskSnapshot{{
			RepoRoot:     location.RepoRoot,
			Workspace:    location.TaskWorkspace,
			ManifestPath: manifestPath,
			Manifest:     manifest,
			Events:       events,
		}},
		Sources: append([]threadSourceSnapshot{definitionSource, manifestSource}, eventSources...),
	}
	return snapshot, stats, nil
}

func loadWorkspaceThreadOwnerSnapshot(location threadOwnerLocation, prior *store.ThreadProjection) (threadOwnerSnapshot, threadProjectionLoadStats, error) {
	workspace, err := readWorkspaceManifest(location.WorkspaceRoot)
	if err != nil {
		return threadOwnerSnapshot{}, threadProjectionLoadStats{}, err
	}
	changePath, change, body, err := findWorkspaceChange(location.WorkspaceRoot, workspace, location.ChangeID)
	if err != nil {
		return threadOwnerSnapshot{}, threadProjectionLoadStats{}, err
	}
	definition, err := readThreadDefinition(location.DefinitionPath)
	if err != nil {
		return threadOwnerSnapshot{}, threadProjectionLoadStats{}, err
	}
	sources := make([]threadSourceSnapshot, 0, 3)
	for _, item := range []struct {
		path string
		kind string
		key  string
	}{
		{workspaceManifestPath(location.WorkspaceRoot), threadSourceWorkspaceManifest, "workspace"},
		{changePath, threadSourceWorkspaceChange, "change"},
		{location.DefinitionPath, threadSourceDefinition, "definition"},
	} {
		source, sourceErr := readThreadSource(item.path, item.kind, item.key)
		if sourceErr != nil {
			return threadOwnerSnapshot{}, threadProjectionLoadStats{}, sourceErr
		}
		sources = append(sources, source)
	}
	links := parseWorkspaceChangeRepoSlices(body)
	validation := workspaceThreadValidationContext{
		Manifest:             workspace,
		Change:               change,
		Links:                links,
		TaskManifests:        make(map[string]taskManifest),
		RepoDefinitionExists: make(map[string]bool),
	}
	snapshot := threadOwnerSnapshot{
		OwnerID:        location.OwnerID,
		Kind:           threadOwnerWorkspaceChange,
		WorkspaceID:    workspace.ID,
		ChangeID:       change.ID,
		WorkspaceRoot:  location.WorkspaceRoot,
		DefinitionPath: location.DefinitionPath,
		Definition:     definition,
		Sources:        sources,
	}
	seenTasks := make(map[string]bool)
	var stats threadProjectionLoadStats
	for position, link := range links {
		repo, exists := workspace.Repos[link.RepoAlias]
		if !exists {
			return threadOwnerSnapshot{}, stats, fmt.Errorf("workspace change links unknown repo alias %q", link.RepoAlias)
		}
		repoRoot := workspaceRelativeAbs(location.WorkspaceRoot, repo.Path)
		taskWorkspace, manifest, manifestPath, loadErr := readThreadTaskManifest(repoRoot, link.TaskID)
		if loadErr != nil {
			return threadOwnerSnapshot{}, stats, loadErr
		}
		key := workspaceThreadTaskKey(link.RepoAlias, manifest.TaskID)
		validation.TaskManifests[key] = manifest
		if _, statErr := os.Stat(repoThreadDefinitionPath(taskWorkspace)); statErr == nil {
			validation.RepoDefinitionExists[key] = true
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return threadOwnerSnapshot{}, stats, statErr
		}
		snapshot.Links = append(snapshot.Links, threadSnapshotLink{
			Position:      position,
			RepoAlias:     link.RepoAlias,
			TaskID:        manifest.TaskID,
			Target:        link.Target,
			Name:          link.Name,
			Status:        link.Status,
			RepoRoot:      repoRoot,
			TaskWorkspace: taskWorkspace,
		})
		if seenTasks[key] {
			continue
		}
		seenTasks[key] = true
		manifestSource, sourceErr := readThreadSource(manifestPath, threadSourceTaskManifest, threadTaskSourceKey(link.RepoAlias, manifest.TaskID))
		if sourceErr != nil {
			return threadOwnerSnapshot{}, stats, sourceErr
		}
		events, eventSources, taskStats, eventErr := readThreadTaskEventsIncremental(link.RepoAlias, taskWorkspace, manifest.TaskID, prior)
		stats.EventsParsed += taskStats.EventsParsed
		stats.EventsReused += taskStats.EventsReused
		if eventErr != nil {
			return threadOwnerSnapshot{}, stats, eventErr
		}
		snapshot.Tasks = append(snapshot.Tasks, threadTaskSnapshot{
			RepoAlias:    link.RepoAlias,
			RepoRoot:     repoRoot,
			Workspace:    taskWorkspace,
			ManifestPath: manifestPath,
			Manifest:     manifest,
			Events:       events,
		})
		snapshot.Sources = append(snapshot.Sources, manifestSource)
		snapshot.Sources = append(snapshot.Sources, eventSources...)
	}
	if err := validateWorkspaceThreadDefinition(definition, validation); err != nil {
		return threadOwnerSnapshot{}, stats, fmt.Errorf("validate workspace thread definition: %w", err)
	}
	sortThreadSnapshot(snapshot.Tasks, snapshot.Sources)
	return snapshot, stats, nil
}

func readThreadTaskManifest(repoRoot, taskID string) (string, taskManifest, string, error) {
	var firstErr error
	for _, taskWorkspace := range taskWorkspaceSearchPaths(repoRoot, defaultTaskWorkspaceDir, taskID) {
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

func readThreadTaskEventsIncremental(repoAlias, workspace, taskID string, prior *store.ThreadProjection) ([]taskCheckpointEvent, []threadSourceSnapshot, threadProjectionLoadStats, error) {
	checkpointDir := filepath.Join(workspace, "checkpoints")
	entries, err := os.ReadDir(checkpointDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, threadProjectionLoadStats{}, nil
	}
	if err != nil {
		return nil, nil, threadProjectionLoadStats{}, fmt.Errorf("read task checkpoint events: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	priorSources := threadProjectionSourceMap(prior)
	priorEvents := threadProjectionEventMap(prior)
	var loads []threadCheckpointLoad
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(checkpointDir, entry.Name())
		key := threadEventSourceKey(repoAlias, taskID, entry.Name())
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, nil, threadProjectionLoadStats{}, fmt.Errorf("inspect thread source %s: %w", path, infoErr)
		}
		metadata, metadataErr := threadSourceFromFileInfo(path, threadSourceCheckpointEvent, key, info)
		if metadataErr != nil {
			return nil, nil, threadProjectionLoadStats{}, metadataErr
		}
		load := threadCheckpointLoad{path: path, metadata: metadata}
		priorSource, sourceExists := priorSources[threadSourceMapKey(threadSourceCheckpointEvent, key)]
		priorEvent, eventExists := priorEvents[threadEventMapKey(repoAlias, taskID, path)]
		if sourceExists && eventExists && threadSourceMetadataEqual(metadata, priorSource) {
			if err := json.Unmarshal([]byte(priorEvent.RecordJSON), &load.record); err != nil {
				return nil, nil, threadProjectionLoadStats{}, fmt.Errorf("decode projected checkpoint event %s: %w", path, err)
			}
			normalizeTaskCheckpointRecord(&load.record)
			load.metadata.ContentHash = priorSource.ContentHash
			load.reused = true
		} else {
		}
		loads = append(loads, load)
	}
	parseThreadCheckpointLoads(loads)
	seenIDs := make(map[string]string)
	events := make([]taskCheckpointEvent, 0, len(loads))
	sources := make([]threadSourceSnapshot, 0, len(loads))
	var stats threadProjectionLoadStats
	for _, load := range loads {
		if load.err != nil {
			return nil, nil, stats, load.err
		}
		if err := validateTaskCheckpointEventRecord(load.path, taskID, load.record); err != nil {
			return nil, nil, stats, err
		}
		if load.reused {
			stats.EventsReused++
		} else {
			stats.EventsParsed++
		}
		record := load.record
		checkpointKey := strings.ToLower(strings.TrimSpace(record.CheckpointID))
		if previousPath, exists := seenIDs[checkpointKey]; exists {
			return nil, nil, stats, fmt.Errorf("duplicate checkpoint event ID %q in %s and %s", record.CheckpointID, previousPath, load.path)
		}
		seenIDs[checkpointKey] = load.path
		events = append(events, taskCheckpointEvent{
			Record:       record,
			JSONPath:     load.path,
			MarkdownPath: strings.TrimSuffix(load.path, filepath.Ext(load.path)) + ".md",
		})
		sources = append(sources, load.metadata)
	}
	return events, sources, stats, nil
}

func parseThreadCheckpointLoads(loads []threadCheckpointLoad) {
	var indexes []int
	for index := range loads {
		if !loads[index].reused {
			indexes = append(indexes, index)
		}
	}
	if len(indexes) == 0 {
		return
	}
	workerCount := runtime.GOMAXPROCS(0) * 2
	if workerCount > 16 {
		workerCount = 16
	}
	if workerCount > len(indexes) {
		workerCount = len(indexes)
	}
	jobs := make(chan int)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		go func() {
			defer workers.Done()
			for index := range jobs {
				data, err := os.ReadFile(loads[index].path)
				if err != nil {
					loads[index].err = err
					continue
				}
				loads[index].record, err = decodeTaskCheckpointRecord(data, loads[index].path)
				if err != nil {
					loads[index].err = err
					continue
				}
				digest := sha256.Sum256(data)
				loads[index].metadata.ContentHash = hex.EncodeToString(digest[:])
			}
		}()
	}
	for _, index := range indexes {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
}

func readThreadSource(path, kind, key string) (threadSourceSnapshot, error) {
	source, err := statThreadSource(path, kind, key)
	if err != nil {
		return threadSourceSnapshot{}, err
	}
	hash, err := threadFileHash(path)
	if err != nil {
		return threadSourceSnapshot{}, err
	}
	source.ContentHash = hash
	return source, nil
}

func statThreadSource(path, kind, key string) (threadSourceSnapshot, error) {
	info, err := os.Stat(path)
	if err != nil {
		return threadSourceSnapshot{}, fmt.Errorf("inspect thread source %s: %w", path, err)
	}
	return threadSourceFromFileInfo(path, kind, key, info)
}

func threadSourceFromFileInfo(path, kind, key string, info os.FileInfo) (threadSourceSnapshot, error) {
	if !info.Mode().IsRegular() {
		return threadSourceSnapshot{}, fmt.Errorf("thread source is not a regular file: %s", path)
	}
	return threadSourceSnapshot{
		Kind:       kind,
		Key:        key,
		Path:       filepath.Clean(path),
		SizeBytes:  info.Size(),
		ModifiedNS: info.ModTime().UnixNano(),
	}, nil
}

func threadFileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func threadTaskSourceKey(repoAlias, taskID string) string {
	return strings.ToLower(strings.TrimSpace(repoAlias) + "\x00" + strings.TrimSpace(taskID))
}

func threadEventSourceKey(repoAlias, taskID, filename string) string {
	return threadTaskSourceKey(repoAlias, taskID) + "\x00" + strings.ToLower(strings.TrimSpace(filename))
}

func threadSourceMapKey(kind, key string) string {
	return kind + "\x00" + key
}

func threadEventMapKey(repoAlias, taskID, path string) string {
	return threadTaskSourceKey(repoAlias, taskID) + "\x00" + canonicalThreadPath(path)
}

func threadProjectionSourceMap(prior *store.ThreadProjection) map[string]store.ThreadProjectionSource {
	out := make(map[string]store.ThreadProjectionSource)
	if prior == nil {
		return out
	}
	for _, source := range prior.Sources {
		out[threadSourceMapKey(source.SourceKind, source.SourceKey)] = source
	}
	return out
}

func threadProjectionEventMap(prior *store.ThreadProjection) map[string]store.ThreadProjectionEvent {
	out := make(map[string]store.ThreadProjectionEvent)
	if prior == nil {
		return out
	}
	for _, event := range prior.Events {
		out[threadEventMapKey(event.RepoAlias, event.TaskID, event.JSONPath)] = event
	}
	return out
}

func threadSourceMetadataEqual(current threadSourceSnapshot, prior store.ThreadProjectionSource) bool {
	return filepath.Clean(current.Path) == filepath.Clean(prior.Path) &&
		current.SizeBytes == prior.SizeBytes &&
		current.ModifiedNS == prior.ModifiedNS
}

func threadProjectionIsFresh(projection store.ThreadProjection) (bool, error) {
	current, err := inventoryThreadProjectionSources(projection)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if len(current) != len(projection.Sources) {
		return false, nil
	}
	prior := threadProjectionSourceMap(&projection)
	for _, source := range current {
		stored, exists := prior[threadSourceMapKey(source.Kind, source.Key)]
		if !exists || !threadSourceMetadataEqual(source, stored) {
			return false, nil
		}
	}
	return true, nil
}

func inventoryThreadProjectionSources(projection store.ThreadProjection) ([]threadSourceSnapshot, error) {
	var sources []threadSourceSnapshot
	for _, prior := range projection.Sources {
		if prior.SourceKind == threadSourceCheckpointEvent {
			continue
		}
		source, err := statThreadSource(prior.Path, prior.SourceKind, prior.SourceKey)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	for _, task := range projection.Tasks {
		checkpointDir := filepath.Join(task.TaskWorkspace, "checkpoints")
		entries, err := os.ReadDir(checkpointDir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
				continue
			}
			key := threadEventSourceKey(task.RepoAlias, task.TaskID, entry.Name())
			info, infoErr := entry.Info()
			if infoErr != nil {
				return nil, infoErr
			}
			source, statErr := threadSourceFromFileInfo(filepath.Join(checkpointDir, entry.Name()), threadSourceCheckpointEvent, key, info)
			if statErr != nil {
				return nil, statErr
			}
			sources = append(sources, source)
		}
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Kind == sources[j].Kind {
			return sources[i].Key < sources[j].Key
		}
		return sources[i].Kind < sources[j].Kind
	})
	return sources, nil
}

func threadSnapshotFromProjection(projection store.ThreadProjection) (threadOwnerSnapshot, error) {
	var definition threadDefinition
	if err := json.Unmarshal([]byte(projection.DefinitionJSON), &definition); err != nil {
		return threadOwnerSnapshot{}, fmt.Errorf("decode projected thread definition: %w", err)
	}
	normalizeThreadDefinition(&definition)
	snapshot := threadOwnerSnapshot{
		OwnerID:        projection.OwnerID,
		Kind:           projection.OwnerKind,
		TaskID:         projection.TaskID,
		WorkspaceID:    projection.WorkspaceID,
		ChangeID:       projection.ChangeID,
		RepoRoot:       projection.RepoRoot,
		WorkspaceRoot:  projection.WorkspaceRoot,
		DefinitionPath: projection.DefinitionPath,
		Definition:     definition,
	}
	for _, link := range projection.Links {
		snapshot.Links = append(snapshot.Links, threadSnapshotLink{
			Position: link.Position, RepoAlias: link.RepoAlias, TaskID: link.TaskID,
			Target: link.Target, Name: link.Name, Status: link.Status,
			RepoRoot: link.RepoRoot, TaskWorkspace: link.TaskWorkspace,
		})
	}
	taskIndexes := make(map[string]int)
	for _, task := range projection.Tasks {
		var manifest taskManifest
		if err := json.Unmarshal([]byte(task.ManifestJSON), &manifest); err != nil {
			return threadOwnerSnapshot{}, fmt.Errorf("decode projected task %s: %w", task.TaskID, err)
		}
		normalizeTaskManifest(&manifest)
		taskIndexes[workspaceThreadTaskKey(task.RepoAlias, task.TaskID)] = len(snapshot.Tasks)
		snapshot.Tasks = append(snapshot.Tasks, threadTaskSnapshot{
			RepoAlias: task.RepoAlias, RepoRoot: task.RepoRoot, Workspace: task.TaskWorkspace,
			ManifestPath: task.ManifestPath, Manifest: manifest,
		})
	}
	for _, event := range projection.Events {
		var record taskCheckpointRecord
		if err := json.Unmarshal([]byte(event.RecordJSON), &record); err != nil {
			return threadOwnerSnapshot{}, fmt.Errorf("decode projected checkpoint %s: %w", event.CheckpointID, err)
		}
		normalizeTaskCheckpointRecord(&record)
		index, exists := taskIndexes[workspaceThreadTaskKey(event.RepoAlias, event.TaskID)]
		if !exists {
			return threadOwnerSnapshot{}, fmt.Errorf("projected checkpoint %s has no projected task", event.CheckpointID)
		}
		snapshot.Tasks[index].Events = append(snapshot.Tasks[index].Events, taskCheckpointEvent{
			Record: record, JSONPath: event.JSONPath, MarkdownPath: event.MarkdownPath,
		})
	}
	for _, source := range projection.Sources {
		snapshot.Sources = append(snapshot.Sources, threadSourceSnapshot{
			Kind: source.SourceKind, Key: source.SourceKey, Path: source.Path,
			SizeBytes: source.SizeBytes, ModifiedNS: source.ModifiedNS, ContentHash: source.ContentHash,
		})
	}
	return snapshot, nil
}

func threadProjectionFromSnapshot(snapshot threadOwnerSnapshot) (store.ThreadProjection, error) {
	definitionJSON, err := json.Marshal(snapshot.Definition)
	if err != nil {
		return store.ThreadProjection{}, err
	}
	projection := store.ThreadProjection{
		OwnerID: snapshot.OwnerID, OwnerKind: snapshot.Kind, TaskID: snapshot.TaskID,
		WorkspaceID: snapshot.WorkspaceID, ChangeID: snapshot.ChangeID,
		RepoRoot: snapshot.RepoRoot, WorkspaceRoot: snapshot.WorkspaceRoot,
		DefinitionPath: snapshot.DefinitionPath, DefinitionRevision: snapshot.Definition.Revision,
		DefinitionJSON: string(definitionJSON), ProjectedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	for _, link := range snapshot.Links {
		projection.Links = append(projection.Links, store.ThreadProjectionLink{
			Position: link.Position, RepoAlias: link.RepoAlias, TaskID: link.TaskID,
			Target: link.Target, Name: link.Name, Status: link.Status,
			RepoRoot: link.RepoRoot, TaskWorkspace: link.TaskWorkspace,
		})
	}
	for _, task := range snapshot.Tasks {
		manifestJSON, marshalErr := json.Marshal(task.Manifest)
		if marshalErr != nil {
			return store.ThreadProjection{}, marshalErr
		}
		projection.Tasks = append(projection.Tasks, store.ThreadProjectionTask{
			RepoAlias: task.RepoAlias, TaskID: task.Manifest.TaskID, RepoRoot: task.RepoRoot,
			TaskWorkspace: task.Workspace, ManifestPath: task.ManifestPath, ManifestJSON: string(manifestJSON),
		})
		for _, event := range task.Events {
			recordJSON, eventErr := json.Marshal(event.Record)
			if eventErr != nil {
				return store.ThreadProjection{}, eventErr
			}
			projection.Events = append(projection.Events, store.ThreadProjectionEvent{
				RepoAlias: task.RepoAlias, TaskID: task.Manifest.TaskID,
				CheckpointID: event.Record.CheckpointID,
				Target:       firstNonEmptyTaskString(event.Record.Target, event.Record.Slice),
				JSONPath:     event.JSONPath, MarkdownPath: event.MarkdownPath, RecordJSON: string(recordJSON),
			})
		}
	}
	for _, source := range snapshot.Sources {
		projection.Sources = append(projection.Sources, store.ThreadProjectionSource{
			SourceKind: source.Kind, SourceKey: source.Key, Path: source.Path,
			SizeBytes: source.SizeBytes, ModifiedNS: source.ModifiedNS, ContentHash: source.ContentHash,
		})
	}
	return projection, nil
}

func sortThreadSnapshot(tasks []threadTaskSnapshot, sources []threadSourceSnapshot) {
	sort.Slice(tasks, func(i, j int) bool {
		return workspaceThreadTaskKey(tasks[i].RepoAlias, tasks[i].Manifest.TaskID) < workspaceThreadTaskKey(tasks[j].RepoAlias, tasks[j].Manifest.TaskID)
	})
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Kind == sources[j].Kind {
			return sources[i].Key < sources[j].Key
		}
		return sources[i].Kind < sources[j].Kind
	})
}
