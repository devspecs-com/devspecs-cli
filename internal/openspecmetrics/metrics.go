// Package openspecmetrics computes structural OpenSpec indexing diagnostics.
package openspecmetrics

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Artifact struct {
	Path           string
	SourceType     string
	SourceIdentity string
	Subtype        string
	ArtifactScope  string
	OpenSpecRole   string
}

type Metrics struct {
	ExpectedBundles        int            `json:"expected_bundles"`
	IndexedBundles         int            `json:"indexed_bundles"`
	BundleRecall           float64        `json:"bundle_recall"`
	MissingBundles         []string       `json:"missing_bundles,omitempty"`
	ExpectedChildRoles     map[string]int `json:"expected_child_roles,omitempty"`
	IndexedChildRoles      map[string]int `json:"indexed_child_roles,omitempty"`
	ExpectedChildArtifacts int            `json:"expected_child_artifacts"`
	IndexedChildArtifacts  int            `json:"indexed_child_artifacts"`
	ChildRoleRecall        float64        `json:"child_role_recall"`
	MissingChildRoles      []string       `json:"missing_child_roles,omitempty"`
	DuplicatePressure      float64        `json:"duplicate_pressure"`
	MarkdownLeakage        int            `json:"markdown_leakage"`
	MarkdownLeakagePaths   []string       `json:"markdown_leakage_paths,omitempty"`
}

func (m Metrics) HasData() bool {
	return m.ExpectedBundles > 0 ||
		m.IndexedBundles > 0 ||
		m.ExpectedChildArtifacts > 0 ||
		m.IndexedChildArtifacts > 0 ||
		m.MarkdownLeakage > 0
}

func Analyze(repoRoot string, artifacts []Artifact) Metrics {
	expected := discoverExpected(repoRoot)
	indexedBundles := map[string]bool{}
	indexedChildren := map[string]string{}
	leakage := map[string]bool{}

	for _, artifact := range artifacts {
		path := normalizePath(artifact.Path)
		if path == "" {
			continue
		}
		if strings.EqualFold(artifact.SourceType, "markdown") && isOpenSpecPath(path) {
			leakage[path] = true
		}
		if isBundleArtifact(artifact, path) {
			indexedBundles[path] = true
			continue
		}
		if role := childRole(artifact, path); role != "" {
			indexedChildren[path] = role
		}
	}

	var out Metrics
	out.ExpectedBundles = len(expected.bundles)
	out.IndexedBundles = len(indexedBundles)
	out.ExpectedChildRoles = countRoles(expected.children)
	out.IndexedChildRoles = countRoles(indexedChildren)
	out.ExpectedChildArtifacts = len(expected.children)
	out.IndexedChildArtifacts = len(indexedChildren)
	out.MarkdownLeakage = len(leakage)
	out.MarkdownLeakagePaths = sortedSet(leakage)

	indexedExpectedBundles := 0
	for bundle := range expected.bundles {
		if indexedBundles[bundle] {
			indexedExpectedBundles++
		} else {
			out.MissingBundles = append(out.MissingBundles, bundle)
		}
	}
	sort.Strings(out.MissingBundles)
	if out.ExpectedBundles > 0 {
		out.BundleRecall = float64(indexedExpectedBundles) / float64(out.ExpectedBundles)
	}

	indexedExpectedChildren := 0
	for path, role := range expected.children {
		if indexedChildren[path] == role {
			indexedExpectedChildren++
			continue
		}
		out.MissingChildRoles = append(out.MissingChildRoles, role+":"+path)
	}
	sort.Strings(out.MissingChildRoles)
	if out.ExpectedChildArtifacts > 0 {
		out.ChildRoleRecall = float64(indexedExpectedChildren) / float64(out.ExpectedChildArtifacts)
	}
	if out.IndexedBundles > 0 {
		out.DuplicatePressure = float64(out.IndexedChildArtifacts) / float64(out.IndexedBundles)
	}
	return out
}

type expectedOpenSpec struct {
	bundles  map[string]bool
	children map[string]string
}

func discoverExpected(repoRoot string) expectedOpenSpec {
	out := expectedOpenSpec{
		bundles:  map[string]bool{},
		children: map[string]string{},
	}
	addChange := func(changeDir string) {
		rel, err := filepath.Rel(repoRoot, changeDir)
		if err != nil {
			return
		}
		rel = normalizePath(rel)
		out.bundles[rel] = true
		addChild := func(name, role string) {
			path := filepath.Join(changeDir, name)
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				return
			}
			childRel, err := filepath.Rel(repoRoot, path)
			if err == nil {
				out.children[normalizePath(childRel)] = role
			}
		}
		addChild("proposal.md", "proposal")
		addChild("design.md", "design")
		addChild("tasks.md", "tasks")
		specsDir := filepath.Join(changeDir, "specs")
		_ = filepath.WalkDir(specsDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Name() != "spec.md" {
				return nil
			}
			childRel, relErr := filepath.Rel(repoRoot, path)
			if relErr == nil {
				out.children[normalizePath(childRel)] = "spec_delta"
			}
			return nil
		})
	}
	for _, changesDir := range discoverChangesDirs(repoRoot) {
		entries, err := os.ReadDir(changesDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if entry.Name() == "archive" {
				archiveDir := filepath.Join(changesDir, "archive")
				archivedEntries, err := os.ReadDir(archiveDir)
				if err != nil {
					continue
				}
				for _, archived := range archivedEntries {
					if archived.IsDir() {
						addChange(filepath.Join(archiveDir, archived.Name()))
					}
				}
				continue
			}
			addChange(filepath.Join(changesDir, entry.Name()))
		}
	}
	return out
}

func discoverChangesDirs(repoRoot string) []string {
	var out []string
	_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if isIgnoredDir(d.Name()) {
			return filepath.SkipDir
		}
		if strings.EqualFold(d.Name(), "openspec") {
			changesDir := filepath.Join(path, "changes")
			if info, statErr := os.Stat(changesDir); statErr == nil && info.IsDir() {
				out = append(out, changesDir)
			}
			return filepath.SkipDir
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func isBundleArtifact(artifact Artifact, path string) bool {
	scope := strings.TrimSpace(artifact.ArtifactScope)
	if scope == "bundle" || artifact.Subtype == "openspec_change_bundle" {
		return strings.Contains(path, "/changes/") && !strings.HasSuffix(path, ".md")
	}
	return strings.Contains(path, "/changes/") && !strings.HasSuffix(path, ".md")
}

func childRole(artifact Artifact, path string) string {
	if strings.Contains(artifact.SourceIdentity, "|openspec_bundle_source") {
		return ""
	}
	switch strings.TrimSpace(artifact.OpenSpecRole) {
	case "proposal", "design", "tasks", "spec_delta":
		return strings.TrimSpace(artifact.OpenSpecRole)
	}
	switch {
	case strings.HasSuffix(path, "/proposal.md") && strings.Contains(path, "/changes/"):
		return "proposal"
	case strings.HasSuffix(path, "/design.md") && strings.Contains(path, "/changes/"):
		return "design"
	case strings.HasSuffix(path, "/tasks.md") && strings.Contains(path, "/changes/"):
		return "tasks"
	case strings.HasSuffix(path, "/spec.md") && strings.Contains(path, "/changes/") && strings.Contains(path, "/specs/"):
		return "spec_delta"
	default:
		return ""
	}
}

func countRoles(items map[string]string) map[string]int {
	out := map[string]int{}
	for _, role := range items {
		out[role]++
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedSet(items map[string]bool) []string {
	out := make([]string, 0, len(items))
	for item := range items {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func isOpenSpecPath(path string) bool {
	path = strings.Trim(normalizePath(path), "/")
	if path == "openspec" || strings.HasPrefix(path, "openspec/") || strings.HasSuffix(path, "/openspec") {
		return true
	}
	return strings.Contains(path, "/openspec/")
}

func normalizePath(path string) string {
	return strings.Trim(filepath.ToSlash(path), "/")
}

func isIgnoredDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "node_modules", "dist", "build", ".next", "coverage", "tmp", "vendor":
		return true
	default:
		return false
	}
}
