package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultWorkspaceArtifactDir = "devspecs"
	workspaceManifestFilename   = "workspace.yaml"
	workspaceDocumentFilename   = "workspace.md"
	workspaceChangesDirName     = "changes"
	workspaceIndexStatus        = "not_indexed"
	workspaceIndexReason        = "workspace artifact indexing is deferred until trace/index relationship support"
)

var workspaceAliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

type workspaceManifest struct {
	ID          string                   `yaml:"id" json:"id"`
	Name        string                   `yaml:"name" json:"name"`
	ArtifactDir string                   `yaml:"artifact_dir" json:"artifact_dir"`
	Repos       map[string]workspaceRepo `yaml:"repos" json:"repos"`
}

type workspaceRepo struct {
	Path           string `yaml:"path" json:"path"`
	Responsibility string `yaml:"responsibility,omitempty" json:"responsibility,omitempty"`
}

func workspaceManifestPath(root string) string {
	return filepath.Join(root, defaultWorkspaceArtifactDir, workspaceManifestFilename)
}

func workspaceDocumentPath(root string) string {
	return filepath.Join(root, defaultWorkspaceArtifactDir, workspaceDocumentFilename)
}

func workspaceChangesDir(root string, manifest workspaceManifest) string {
	artifactDir := strings.TrimSpace(manifest.ArtifactDir)
	if artifactDir == "" {
		artifactDir = defaultWorkspaceArtifactDir
	}
	return filepath.Join(root, filepath.FromSlash(artifactDir), workspaceChangesDirName)
}

func resolveWorkspaceRootForInit(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	return filepath.Clean(abs), nil
}

func resolveWorkspaceRoot(workspacePath string) (string, error) {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath != "" {
		abs, err := filepath.Abs(workspacePath)
		if err != nil {
			return "", fmt.Errorf("resolve workspace root: %w", err)
		}
		abs = filepath.Clean(abs)
		if _, err := os.Stat(workspaceManifestPath(abs)); err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("no DevSpecs workspace found at %s; run `ds workspace init %s` first", abs, abs)
			}
			return "", err
		}
		return abs, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	wd, err = filepath.Abs(wd)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(workspaceManifestPath(wd)); err == nil {
			return wd, nil
		} else if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return "", fmt.Errorf("no DevSpecs workspace found; run `ds workspace init <path>` or pass --workspace <path>")
}

func newWorkspaceManifest(root string) (workspaceManifest, error) {
	root = filepath.Clean(root)
	id := sanitizeWorkspaceID(filepath.Base(root))
	if id == "" {
		id = "workspace"
	}
	repos, err := detectWorkspaceRepos(root)
	if err != nil {
		return workspaceManifest{}, err
	}
	return workspaceManifest{
		ID:          id,
		Name:        strings.ToUpper(id),
		ArtifactDir: defaultWorkspaceArtifactDir,
		Repos:       repos,
	}, nil
}

func readWorkspaceManifest(root string) (workspaceManifest, error) {
	data, err := os.ReadFile(workspaceManifestPath(root))
	if err != nil {
		return workspaceManifest{}, err
	}
	var manifest workspaceManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return workspaceManifest{}, fmt.Errorf("parse workspace manifest: %w", err)
	}
	return normalizeWorkspaceManifest(manifest), nil
}

func writeWorkspaceManifest(path string, manifest workspaceManifest) error {
	manifest = normalizeWorkspaceManifest(manifest)
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func normalizeWorkspaceManifest(manifest workspaceManifest) workspaceManifest {
	manifest.ID = sanitizeWorkspaceID(manifest.ID)
	if manifest.ID == "" {
		manifest.ID = "workspace"
	}
	manifest.Name = strings.TrimSpace(manifest.Name)
	if manifest.Name == "" {
		manifest.Name = strings.ToUpper(manifest.ID)
	}
	manifest.ArtifactDir = strings.Trim(filepath.ToSlash(strings.TrimSpace(manifest.ArtifactDir)), "/")
	if manifest.ArtifactDir == "" {
		manifest.ArtifactDir = defaultWorkspaceArtifactDir
	}
	if manifest.Repos == nil {
		manifest.Repos = map[string]workspaceRepo{}
	}
	for alias, repo := range manifest.Repos {
		repo.Path = normalizeWorkspaceRepoPath(repo.Path)
		repo.Responsibility = strings.TrimSpace(repo.Responsibility)
		manifest.Repos[alias] = repo
	}
	return manifest
}

func detectWorkspaceRepos(root string) (map[string]workspaceRepo, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]workspaceRepo{}, nil
		}
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	repos := map[string]workspaceRepo{}
	for _, entry := range entries {
		if !entry.IsDir() || shouldSkipWorkspaceRepoDir(entry.Name()) {
			continue
		}
		abs := filepath.Join(root, entry.Name())
		if !hasWorkspaceGitMarker(abs) {
			if _, err := os.Stat(filepath.Join(abs, ".devspecs", "config.yaml")); err != nil {
				continue
			}
		}
		alias := workspaceRepoAlias(entry.Name())
		if alias == "" {
			continue
		}
		if _, exists := repos[alias]; exists {
			alias = sanitizeWorkspaceID(entry.Name())
		}
		if alias == "" {
			continue
		}
		if _, exists := repos[alias]; exists {
			return nil, fmt.Errorf("duplicate workspace repo alias %q", alias)
		}
		repos[alias] = workspaceRepo{Path: "./" + filepath.ToSlash(entry.Name())}
	}
	return repos, nil
}

func shouldSkipWorkspaceRepoDir(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	lower := strings.ToLower(name)
	return strings.HasPrefix(name, ".") ||
		lower == defaultWorkspaceArtifactDir ||
		lower == "node_modules" ||
		lower == "vendor"
}

func workspaceRepoAlias(name string) string {
	name = sanitizeWorkspaceID(name)
	parts := strings.Split(name, "-")
	if len(parts) > 1 {
		last := strings.TrimSpace(parts[len(parts)-1])
		if last != "" {
			return last
		}
	}
	return name
}

func normalizeWorkspaceRepoPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return "."
	}
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return "."
	}
	return "./" + strings.Trim(path, "/")
}

func sanitizeWorkspaceID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func sortedWorkspaceRepoAliases(repos map[string]workspaceRepo) []string {
	aliases := make([]string, 0, len(repos))
	for alias := range repos {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

func validateWorkspaceRepoAliases(manifest workspaceManifest, aliases []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if !workspaceAliasPattern.MatchString(alias) {
			return nil, fmt.Errorf("invalid workspace repo alias %q", alias)
		}
		if seen[alias] {
			return nil, fmt.Errorf("duplicate workspace repo alias %q", alias)
		}
		if _, ok := manifest.Repos[alias]; !ok {
			return nil, fmt.Errorf("workspace repo alias %q not found in %s", alias, workspaceManifestFilename)
		}
		seen[alias] = true
		out = append(out, alias)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--repos is required")
	}
	return out, nil
}

func escapeMarkdownTableCell(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "|", `\|`)
}
