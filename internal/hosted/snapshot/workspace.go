package snapshot

import (
	"path"
	"strings"
)

type workspaceProjection struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	ArtifactDir string `yaml:"artifact_dir"`
	Repos       map[string]struct {
		Path           string `yaml:"path"`
		Responsibility string `yaml:"responsibility"`
	} `yaml:"repos"`
}

type changeProjection struct {
	ID            string   `yaml:"id"`
	Type          string   `yaml:"type"`
	Workspace     string   `yaml:"workspace"`
	Status        string   `yaml:"status"`
	Title         string   `yaml:"title"`
	RequiredRepos []string `yaml:"required_repos"`
	OptionalRepos []string `yaml:"optional_repos"`
}

func (c *gitCorpus) workspace(root Root, repoID string) ([]Record, error) {
	const suffix = "devspecs/workspace.yaml"
	if root.Path != suffix && !strings.HasSuffix(root.Path, "/"+suffix) {
		return nil, ErrUnsupported
	}
	if _, exists := c.entries[root.Path]; !exists {
		return []Record{}, nil
	}
	manifestFile, err := c.read(root.Path)
	if err != nil {
		return nil, err
	}
	var manifest workspaceProjection
	if decodeYAML(manifestFile.Content, &manifest, true) != nil || !ValidID(manifest.ID) || manifest.Name == "" {
		return nil, ErrUnsupported
	}
	if manifest.ArtifactDir == "" {
		manifest.ArtifactDir = "devspecs"
	}
	manifest.ArtifactDir = strings.TrimSpace(manifest.ArtifactDir)
	if manifest.ArtifactDir == "" {
		manifest.ArtifactDir = "devspecs"
	}
	if path.IsAbs(manifest.ArtifactDir) || strings.ContainsAny(manifest.ArtifactDir, "\\:\x00\r\n") {
		return nil, ErrProvenance
	}
	base := strings.TrimSuffix(root.Path, suffix)
	artifactRoot := path.Join(base, manifest.ArtifactDir)
	if !ValidPath(artifactRoot) {
		return nil, ErrProvenance
	}
	bound := path.Dir(root.Path)
	if root.ArtifactRoot != "" {
		if artifactRoot != root.ArtifactRoot {
			return nil, ErrProvenance
		}
	} else if artifactRoot != bound && !strings.HasPrefix(artifactRoot, bound+"/") {
		return nil, ErrUnsupported
	}
	if err := c.checkRoot(Root{Kind: "task_parent", Path: artifactRoot}); err != nil {
		return nil, err
	}
	for alias, repo := range manifest.Repos {
		if !ValidID(alias) || repo.Path == "" {
			return nil, ErrInvalid
		}
	}
	workspaceID := strings.ToLower(manifest.ID)
	files := []File{manifestFile}
	if _, exists := c.entries[path.Join(bound, "workspace.md")]; exists {
		document, err := c.read(path.Join(bound, "workspace.md"))
		if err != nil {
			return nil, err
		}
		if _, err = documentIdentity(document.Content); err != nil {
			return nil, err
		}
		files = append(files, document)
	}
	records := []Record{{Kind: "workspace", OwnerKind: "repository", OwnerID: repoID, RecordID: workspaceID, SchemaVersion: 1, Files: files}}
	changesRoot := path.Join(artifactRoot, "changes")
	if err := c.checkRoot(Root{Kind: "task_parent", Path: changesRoot}); err != nil {
		return nil, err
	}
	consumed := make(map[string]bool)
	changes := make(map[string]bool)
	for name, entry := range c.entries {
		if entry.kind == "tree" || !strings.HasPrefix(name, changesRoot+"/") {
			continue
		}
		if entry.mode == "120000" || entry.mode == "160000" {
			return nil, ErrProvenance
		}
		if path.Ext(name) != ".md" || path.Dir(name) != changesRoot {
			continue
		}
		file, err := c.read(name)
		if err != nil {
			return nil, err
		}
		header, _, err := splitFrontmatter(file.Content)
		if err != nil || len(header) == 0 {
			return nil, ErrUnsupported
		}
		var change changeProjection
		if decodeYAML(header, &change, true) != nil || change.Type != "workspace_change" || !strings.EqualFold(change.Workspace, workspaceID) || !ValidID(change.ID) || change.Title == "" || change.Status == "" {
			return nil, ErrUnsupported
		}
		if _, err := documentIdentity(file.Content); err != nil {
			return nil, err
		}
		id := strings.ToLower(change.ID)
		if changes[id] {
			return nil, ErrConflict
		}
		changes[id] = true
		aliases := make(map[string]bool)
		for _, alias := range append(append([]string(nil), change.RequiredRepos...), change.OptionalRepos...) {
			if _, exists := manifest.Repos[alias]; !exists || aliases[alias] {
				return nil, ErrInvalid
			}
			aliases[alias] = true
		}
		if len(change.RequiredRepos) == 0 {
			return nil, ErrInvalid
		}
		consumed[name] = true
		records = append(records, Record{Kind: "change", OwnerKind: "workspace", OwnerID: workspaceID, RecordID: id, SchemaVersion: 1, Files: []File{file}})
		threadName := path.Join(changesRoot, change.ID+".threads.yaml")
		if _, exists := c.entries[threadName]; exists {
			file, err := c.read(threadName)
			if err != nil {
				return nil, err
			}
			if err := readWorkspaceThreads(file.Content, manifest, change); err != nil {
				return nil, err
			}
			consumed[threadName] = true
			// Both owner IDs are required to disambiguate a change across workspaces.
			ownerID := workspaceID + ":" + id
			if !ValidID(ownerID) {
				return nil, ErrInvalid
			}
			records = append(records, Record{Kind: "thread", OwnerKind: "workspace_change", OwnerID: ownerID, RecordID: "threads", SchemaVersion: 1, Files: []File{file}})
		}
	}
	for name, entry := range c.entries {
		if entry.kind != "tree" && strings.HasPrefix(name, changesRoot+"/") && !consumed[name] {
			return nil, ErrUnsupported
		}
	}
	return records, nil
}

func readWorkspaceThreads(data []byte, workspace workspaceProjection, change changeProjection) error {
	var definition threadProjection
	if decodeYAML(data, &definition, true) != nil || definition.SchemaVersion != 1 || definition.Revision < 1 || definition.Owner.Kind != "workspace_change" || definition.Owner.TaskID != "" || !strings.EqualFold(definition.Owner.WorkspaceID, workspace.ID) || !strings.EqualFold(definition.Owner.ChangeID, change.ID) || len(definition.Threads) == 0 {
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
			if _, exists := workspace.Repos[target.Repo]; !exists || !ValidID(target.Task) || !ValidID(target.Target) {
				return ErrInvalid
			}
			identity := strings.ToLower(target.Repo + ":" + target.Task + ":" + target.Target)
			if assigned[identity] {
				return ErrInvalid
			}
			assigned[identity] = true
		}
	}
	for _, lane := range definition.Threads {
		key := strings.ToLower(lane.Key)
		seen := make(map[string]bool)
		for _, after := range lane.After {
			dependency := strings.ToLower(after)
			if dependency == key || !lanes[dependency] || seen[dependency] {
				return ErrInvalid
			}
			seen[dependency] = true
			edges[key] = append(edges[key], dependency)
		}
	}
	return acyclic(edges)
}
