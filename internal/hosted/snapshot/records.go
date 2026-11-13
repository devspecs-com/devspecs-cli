package snapshot

import (
	"bytes"
	"encoding/json"
	"io"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

// These projection readers intentionally do not replace command-private SDK
// mutation models. All original bytes are retained; only identity, membership
// and graph fields are interpreted here.
type taskProjection struct {
	TaskID               string          `json:"task_id"`
	SchemaVersion        json.RawMessage `json:"schema_version"`
	Series               string          `json:"series"`
	ParentChange         string          `json:"parent_change"`
	LatestCheckpointID   string          `json:"latest_checkpoint_id"`
	LatestCheckpoint     string          `json:"latest_checkpoint"`
	LatestCheckpointJSON string          `json:"latest_checkpoint_json"`
	Artifacts            struct {
		Series     string `json:"series"`
		Index      string `json:"index"`
		FirstSlice string `json:"first_slice"`
		Result     string `json:"result"`
		Slices     []struct {
			ID                   string `json:"id"`
			Plan                 string `json:"plan"`
			Result               string `json:"result"`
			ParentID             string `json:"parent_id"`
			Kind                 string `json:"kind"`
			LatestCheckpointID   string `json:"latest_checkpoint_id"`
			LatestCheckpoint     string `json:"latest_checkpoint"`
			LatestCheckpointJSON string `json:"latest_checkpoint_json"`
		} `json:"slices"`
	} `json:"artifacts"`
}

type checkpointProjection struct {
	SchemaVersion int      `json:"schema_version"`
	EventKind     string   `json:"event_kind"`
	CheckpointID  string   `json:"checkpoint_id"`
	TaskID        string   `json:"task_id"`
	Target        string   `json:"target"`
	Slice         string   `json:"slice"`
	Stage         string   `json:"stage"`
	Decision      string   `json:"decision"`
	CreatedAt     string   `json:"created_at"`
	Supersedes    []string `json:"supersedes_checkpoint_ids"`
}

func decodeProjection(file File, into any) error {
	if !utf8.Valid(file.Content) || CheckJSON(file.Content) != nil || json.Unmarshal(file.Content, into) != nil {
		return ErrInvalid
	}
	return nil
}

func (c *gitCorpus) tasks(root Root, repoID string) ([]Record, error) {
	owners := make(map[string]bool)
	for name, entry := range c.entries {
		if entry.kind == "tree" || !strings.HasPrefix(name, root.Path+"/") {
			continue
		}
		if entry.mode == "160000" || entry.mode == "120000" {
			return nil, ErrProvenance
		}
		rel := strings.TrimPrefix(name, root.Path+"/")
		owner, _, found := strings.Cut(rel, "/")
		if !found || !ValidPath(owner) {
			return nil, ErrUnsupported
		}
		owners[path.Join(root.Path, owner)] = true
	}
	var records []Record
	for owner := range owners {
		parsed, err := c.task(owner, repoID)
		if err != nil {
			return nil, err
		}
		records = append(records, parsed...)
	}
	return records, nil
}

func (c *gitCorpus) task(owner, repoID string) ([]Record, error) {
	manifest, err := c.read(path.Join(owner, "task.json"))
	if err != nil {
		return nil, err
	}
	var task taskProjection
	if decodeProjection(manifest, &task) != nil || !ValidID(task.TaskID) || len(task.SchemaVersion) != 0 || task.Artifacts.Index == "" {
		return nil, ErrUnsupported
	}
	taskID := strings.ToLower(task.TaskID)
	if task.Series == "" {
		task.Series = task.Artifacts.Series
	}
	task.Series = strings.ToUpper(strings.TrimSpace(task.Series))
	if task.Series == "" {
		task.Series = "A"
	}
	members := map[string]bool{manifest.Path: true}
	files := []File{manifest}
	add := func(relative string) error {
		if relative == "" {
			return nil
		}
		if !ValidPath(relative) || relative == "task.json" || relative == "threads.yaml" || strings.HasPrefix(relative, "checkpoints/") {
			return ErrInvalid
		}
		name := owner + "/" + relative
		if members[name] {
			return nil
		}
		file, err := c.read(name)
		if err != nil {
			return err
		}
		members[name] = true
		files = append(files, file)
		return nil
	}
	if add(task.Artifacts.Index) != nil || add(task.Artifacts.FirstSlice) != nil || add(task.Artifacts.Result) != nil {
		return nil, ErrProvenance
	}
	targets := make(map[string]bool)
	threadTargets := make(map[string]bool)
	for _, slice := range task.Artifacts.Slices {
		if !ValidID(slice.ID) || targets[strings.ToLower(slice.ID)] || slice.Plan == "" || slice.Result == "" {
			return nil, ErrUnsupported
		}
		targets[strings.ToLower(slice.ID)] = true
		if slice.ParentID == "" && slice.Kind != "iteration" {
			threadTargets[strings.ToLower(slice.ID)] = true
		}
		if err := add(slice.Plan); err != nil {
			return nil, err
		}
		if err := add(slice.Result); err != nil {
			return nil, err
		}
	}
	records := []Record{{Kind: "task", OwnerKind: "repository", OwnerID: repoID, RecordID: taskID, SchemaVersion: 1, Files: files}}
	threadName := owner + "/threads.yaml"
	if _, found := c.entries[threadName]; found {
		if task.ParentChange != "" {
			return nil, ErrUnsupported
		}
		thread, err := c.read(threadName)
		if err != nil {
			return nil, err
		}
		if err := readTaskThreads(thread.Content, taskID, threadTargets); err != nil {
			return nil, err
		}
		members[threadName] = true
		records = append(records, Record{Kind: "thread", OwnerKind: "task", OwnerID: taskID, RecordID: "threads", SchemaVersion: 1, Files: []File{thread}})
	}
	events := make(map[string]checkpointProjection)
	eventPaths := make(map[string][2]string)
	for name, entry := range c.entries {
		if !strings.HasPrefix(name, owner+"/checkpoints/") || entry.kind == "tree" || path.Ext(name) != ".json" {
			continue
		}
		if path.Dir(name) != owner+"/checkpoints" {
			return nil, ErrUnsupported
		}
		event, err := c.read(name)
		if err != nil {
			return nil, err
		}
		var cp checkpointProjection
		if decodeProjection(event, &cp) != nil || cp.SchemaVersion != 3 || cp.EventKind != "task_checkpoint" || !ValidID(cp.CheckpointID) || !strings.EqualFold(cp.TaskID, taskID) || !validStage(cp.Stage) || !validDecision(cp.Decision) {
			return nil, ErrUnsupported
		}
		if _, err := time.Parse(time.RFC3339, cp.CreatedAt); err != nil {
			return nil, ErrInvalid
		}
		if cp.Target == "" {
			cp.Target = cp.Slice
		}
		cp.Target = taskCheckpointTarget(task, cp.Target)
		if cp.Target == "" || (!targets[cp.Target] && cp.Target != strings.ToLower(task.Series)+"00") {
			return nil, ErrInvalid
		}
		if cp.Slice != "" && taskCheckpointTarget(task, cp.Slice) != cp.Target {
			return nil, ErrInvalid
		}
		key := strings.ToLower(cp.CheckpointID)
		if _, exists := events[key]; exists {
			return nil, ErrConflict
		}
		events[key] = cp
		markdown, err := c.read(strings.TrimSuffix(name, ".json") + ".md")
		if err != nil {
			return nil, err
		}
		members[event.Path], members[markdown.Path] = true, true
		eventPaths[key] = [2]string{event.Path, markdown.Path}
		records = append(records, Record{Kind: "checkpoint", OwnerKind: "task", OwnerID: taskID, RecordID: key, SchemaVersion: 3, Files: []File{event, markdown}})
	}
	if err := validateCheckpointGraph(events); err != nil {
		return nil, err
	}
	checkLatest := func(id, eventPath, markdownPath, target string) error {
		if id == "" && eventPath == "" && markdownPath == "" {
			return nil
		}
		key := strings.ToLower(id)
		cp, found := events[key]
		paths := eventPaths[key]
		if !ValidID(id) || !found || !ValidPath(eventPath) || !ValidPath(markdownPath) || paths[0] != owner+"/"+eventPath || paths[1] != owner+"/"+markdownPath || cp.Target != taskCheckpointTarget(task, target) {
			return ErrProvenance
		}
		return nil
	}
	if checkLatest(task.LatestCheckpointID, task.LatestCheckpointJSON, task.LatestCheckpoint, task.Series+"00") != nil {
		return nil, ErrProvenance
	}
	for _, slice := range task.Artifacts.Slices {
		if checkLatest(slice.LatestCheckpointID, slice.LatestCheckpointJSON, slice.LatestCheckpoint, slice.ID) != nil {
			return nil, ErrProvenance
		}
	}
	for name, entry := range c.entries {
		if entry.kind != "tree" && strings.HasPrefix(name, owner+"/") && !members[name] {
			return nil, ErrUnsupported
		}
	}
	return records, nil
}

func validStage(stage string) bool {
	switch stage {
	case "packed", "planned", "started", "implemented", "validated", "done", "blocked", "completed", "split", "superseded", "cancelled", "rolled_back":
		return true
	}
	return false
}

func validDecision(decision string) bool {
	// Exact taskDecisions from CLI internal/commands/task.go, plus its optional empty value.
	switch decision {
	case "", "promote", "improve", "rework", "rollback", "block", "blocked", "complete", "completed", "split", "supersede", "superseded", "cancel", "cancelled", "continue":
		return true
	}
	return false
}

func taskCheckpointTarget(task taskProjection, target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if strings.EqualFold(target, task.Series) || strings.EqualFold(target, task.Series+"00") || strings.EqualFold(target, task.Artifacts.Index) {
		return strings.ToLower(task.Series) + "00"
	}
	if !ValidID(target) {
		return ""
	}
	return strings.ToLower(target)
}

func validateCheckpointGraph(events map[string]checkpointProjection) error {
	edges := make(map[string][]string)
	for id, cp := range events {
		seen := make(map[string]bool)
		for _, predecessor := range cp.Supersedes {
			key := strings.ToLower(predecessor)
			prior, found := events[key]
			if !found || key == id || seen[key] || !strings.EqualFold(prior.Target, cp.Target) {
				return ErrInvalid
			}
			seen[key] = true
			edges[id] = append(edges[id], key)
		}
	}
	return acyclic(edges)
}

type threadProjection struct {
	SchemaVersion int `yaml:"schema_version"`
	Revision      int `yaml:"revision"`
	Owner         struct {
		Kind        string `yaml:"kind"`
		TaskID      string `yaml:"task_id"`
		WorkspaceID string `yaml:"workspace_id"`
		ChangeID    string `yaml:"change_id"`
	} `yaml:"owner"`
	Threads []struct {
		Key     string `yaml:"key"`
		Name    string `yaml:"name"`
		Targets []struct {
			Repo   string `yaml:"repo"`
			Task   string `yaml:"task"`
			Target string `yaml:"target"`
		} `yaml:"targets"`
		After []string `yaml:"after"`
	} `yaml:"threads"`
}

func readTaskThreads(data []byte, taskID string, targets map[string]bool) error {
	var definition threadProjection
	if decodeYAML(data, &definition, true) != nil || definition.SchemaVersion != 1 || definition.Revision < 1 || definition.Owner.Kind != "task" || !strings.EqualFold(definition.Owner.TaskID, taskID) || definition.Owner.WorkspaceID != "" || definition.Owner.ChangeID != "" || len(definition.Threads) == 0 {
		return ErrUnsupported
	}
	lanes := make(map[string]bool)
	assigned := make(map[string]bool)
	edges := make(map[string][]string)
	for _, lane := range definition.Threads {
		key := strings.ToLower(lane.Key)
		if !ValidID(lane.Key) || lanes[key] || len(lane.Targets) == 0 {
			return ErrInvalid
		}
		lanes[key] = true
		for _, target := range lane.Targets {
			id := strings.ToLower(target.Target)
			if target.Repo != "" || target.Task != "" || !targets[id] || assigned[id] {
				return ErrInvalid
			}
			assigned[id] = true
		}
	}
	for _, lane := range definition.Threads {
		key := strings.ToLower(lane.Key)
		seen := make(map[string]bool)
		for _, after := range lane.After {
			dependency := strings.ToLower(after)
			if !lanes[dependency] || dependency == key || seen[dependency] {
				return ErrInvalid
			}
			seen[dependency] = true
			edges[key] = append(edges[key], dependency)
		}
	}
	return acyclic(edges)
}

func acyclic(edges map[string][]string) error {
	state := make(map[string]int)
	var visit func(string, int) error
	visit = func(id string, depth int) error {
		if depth > MaxRecords || state[id] == 1 {
			return ErrInvalid
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, prior := range edges[id] {
			if err := visit(prior, depth+1); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range edges {
		if err := visit(id, 0); err != nil {
			return err
		}
	}
	return nil
}

func decodeYAML(data []byte, into any, strict bool) error {
	if !utf8.Valid(data) {
		return ErrInvalid
	}
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&document) != nil {
		return ErrInvalid
	}
	var extra yaml.Node
	if decoder.Decode(&extra) != io.EOF {
		return ErrInvalid
	}
	nodes := 0
	if err := checkYAMLNode(&document, 0, &nodes); err != nil {
		return err
	}
	decoder = yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(strict)
	if decoder.Decode(into) != nil {
		return ErrInvalid
	}
	return nil
}

func checkYAMLNode(node *yaml.Node, depth int, nodes *int) error {
	*nodes++
	if depth > MaxJSONDepth || *nodes > 50000 || node.Kind == yaml.AliasNode || node.Anchor != "" {
		return ErrInvalid
	}
	if node.Kind == yaml.MappingNode {
		keys := make(map[string]bool)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || keys[key.Value] {
				return ErrInvalid
			}
			keys[key.Value] = true
		}
	}
	for _, child := range node.Content {
		if err := checkYAMLNode(child, depth+1, nodes); err != nil {
			return err
		}
	}
	return nil
}

func (c *gitCorpus) documents(root Root, repoID string) ([]Record, error) {
	var names []string
	for name, entry := range c.entries {
		if entry.kind == "tree" || !strings.HasPrefix(name, root.Path+"/") {
			continue
		}
		if entry.mode == "160000" || entry.mode == "120000" {
			return nil, ErrProvenance
		}
		if path.Ext(name) != ".md" {
			return nil, ErrUnsupported
		}
		names = append(names, name)
	}
	sort.Strings(names)
	var records []Record
	for _, name := range names {
		file, err := c.read(name)
		if err != nil {
			return nil, err
		}
		id, err := documentIdentity(file.Content)
		if err != nil {
			return nil, err
		}
		if id == "" {
			id = root.Label + "/" + strings.TrimPrefix(name, root.Path+"/")
		}
		if !ValidPath(id) {
			return nil, ErrInvalid
		}
		records = append(records, Record{Kind: root.Kind, OwnerKind: "repository", OwnerID: repoID, RecordID: id, SchemaVersion: 1, Files: []File{file}})
	}
	return records, nil
}

func splitFrontmatter(data []byte) ([]byte, []byte, error) {
	if !utf8.Valid(data) || bytes.Contains(data, []byte{0}) {
		return nil, nil, ErrInvalid
	}
	body := data
	var header []byte
	first, remainder, found := bytes.Cut(data, []byte{'\n'})
	if found && bytes.Equal(bytes.TrimSuffix(first, []byte{'\r'}), []byte("---")) {
		closed := false
		for len(remainder) != 0 {
			line, rest, more := bytes.Cut(remainder, []byte{'\n'})
			if bytes.Equal(bytes.TrimSuffix(line, []byte{'\r'}), []byte("---")) {
				body, closed = rest, true
				break
			}
			header = append(header, line...)
			header = append(header, '\n')
			remainder = rest
			if !more {
				break
			}
		}
		if !closed {
			return nil, nil, ErrInvalid
		}
	}
	return header, body, nil
}

func documentIdentity(data []byte) (string, error) {
	header, body, err := splitFrontmatter(data)
	if err != nil {
		return "", err
	}
	id := ""
	if header != nil {
		var metadata map[string]any
		if decodeYAML(header, &metadata, false) != nil {
			return "", ErrInvalid
		}
		if version, present := metadata["schema_version"]; present && version != 1 {
			return "", ErrUnsupported
		}
		if explicit, present := metadata["id"]; present {
			value, ok := explicit.(string)
			if !ok || !ValidID(value) {
				return "", ErrInvalid
			}
			id = value
		}
	}
	root := goldmark.New().Parser().Parse(text.NewReader(body))
	headings, nodes, depth := 0, 0, 0
	err = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			depth--
			return ast.WalkContinue, nil
		}
		depth++
		nodes++
		if depth > MaxJSONDepth || nodes > 50000 {
			return ast.WalkStop, ErrInvalid
		}
		if heading, ok := node.(*ast.Heading); ok && heading.Level == 1 && heading.Parent() == root {
			hasText, err := documentHeadingHasText(heading, body)
			if err != nil {
				return ast.WalkStop, err
			}
			if hasText {
				headings++
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	if headings != 1 {
		return "", ErrUnsupported
	}
	return id, nil
}

// The outer walk still validates the complete tree after this early text check.
func documentHeadingHasText(heading *ast.Heading, source []byte) (bool, error) {
	found, nodes, depth := false, 0, 0
	err := ast.Walk(heading, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			depth--
			return ast.WalkContinue, nil
		}
		depth++
		nodes++
		if depth > MaxJSONDepth || nodes > 50000 {
			return ast.WalkStop, ErrInvalid
		}
		var value []byte
		switch node := node.(type) {
		case *ast.Text:
			value = node.Value(source)
		case *ast.String:
			value = node.Value
		case *ast.AutoLink:
			value = node.Label(source)
		case *ast.RawHTML:
			value = node.Segments.Value(source)
		}
		if len(bytes.TrimSpace(value)) != 0 {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found, err
}
