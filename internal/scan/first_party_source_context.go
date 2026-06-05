package scan

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

const (
	firstPartySourceAdmissionReason = "first_party_source_context"
	firstPartySourceMaxFileBytes    = 256 * 1024
	firstPartySourceMaxCandidates   = 2500
)

type firstPartySourceRoot struct {
	path string
	kind string
}

func buildFirstPartySourceContextCandidates(ctx context.Context, repoRoot string, existing []adapters.Candidate) []adapters.Candidate {
	if strings.TrimSpace(repoRoot) == "" {
		return nil
	}
	inventory, err := collectFileInventory(ctx, repoRoot)
	if err != nil || len(inventory) == 0 {
		return nil
	}
	existingPaths := map[string]bool{}
	for _, candidate := range existing {
		if rel := normalizeFirstPartySourceRel(candidate.RelPath); rel != "" {
			existingPaths[rel] = true
		}
	}
	roots := detectFirstPartySourceRoots(inventory)
	if len(roots) == 0 {
		return nil
	}
	var candidates []adapters.Candidate
	for _, file := range inventory {
		rel := normalizeFirstPartySourceRel(file.relPath)
		if rel == "" || existingPaths[rel] || file.size > firstPartySourceMaxFileBytes {
			continue
		}
		root := bestFirstPartySourceRoot(rel, roots)
		if root.path == "" {
			continue
		}
		role := firstPartySourceRole(rel)
		if role == "" || firstPartySourceLooksNoisePath(rel) {
			continue
		}
		if firstPartySourceLooksDocumentationExample(rel) && role != "test_doc_example" {
			continue
		}
		if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(rel, false) {
			continue
		}
		candidates = append(candidates, adapters.Candidate{
			PrimaryPath:      filepath.Join(repoRoot, filepath.FromSlash(rel)),
			RelPath:          rel,
			AdapterName:      sourceCompanionAdapterName,
			DiscoveryScore:   firstPartySourceDiscoveryScore(role, root.kind),
			DiscoveryReasons: []string{firstPartySourceAdmissionReason, role, root.kind},
			Metadata: map[string]any{
				"admission_reason": firstPartySourceAdmissionReason,
				"source_role":      role,
				"source_root":      root.path,
				"source_root_kind": root.kind,
				"source_path":      rel,
			},
		})
	}
	if len(candidates) <= firstPartySourceMaxCandidates {
		sortCandidates(candidates)
		return candidates
	}
	return capFirstPartySourceCandidates(candidates, firstPartySourceMaxCandidates)
}

func detectFirstPartySourceRoots(inventory []fileInventoryEntry) []firstPartySourceRoot {
	roots := map[string]string{}
	add := func(path, kind string) {
		path = normalizeFirstPartySourceRoot(path)
		if path == "" {
			return
		}
		if existing := roots[path]; existing != "" && firstPartySourceRootRank(existing) >= firstPartySourceRootRank(kind) {
			return
		}
		roots[path] = kind
	}

	pyCounts := map[string]int{}
	initDirs := map[string]bool{}
	sourceCountsByDir := map[string]int{}
	moduleMarkerDirs := map[string]bool{}
	rootSourceCount := 0
	rootMarkers := map[string]bool{}
	for _, file := range inventory {
		rel := normalizeFirstPartySourceRel(file.relPath)
		if rel == "" {
			continue
		}
		parts := strings.Split(rel, "/")
		base := strings.ToLower(filepath.Base(rel))
		if len(parts) == 1 {
			if isFirstPartyRepoRootMarker(base) {
				rootMarkers[base] = true
			}
			if firstPartySourceLooksSourcePath(rel) {
				rootSourceCount++
			}
		}
		if isFirstPartyRepoRootMarker(base) && !firstPartySourceLooksNoisePath(rel) {
			dir := normalizeFirstPartySourceRoot(filepath.ToSlash(filepath.Dir(rel)))
			if dir != "" {
				moduleMarkerDirs[dir] = true
			}
		}
		if firstPartySourceLooksSourcePath(rel) && !firstPartySourceLooksNoisePath(rel) {
			for _, dir := range firstPartyAncestorDirs(rel) {
				sourceCountsByDir[dir]++
			}
		}
		if strings.ToLower(filepath.Ext(base)) == ".py" {
			for _, dir := range firstPartyAncestorDirs(rel) {
				pyCounts[dir]++
			}
			if base == "__init__.py" {
				initDirs[filepath.ToSlash(filepath.Dir(rel))] = true
			}
		}
		if len(parts) > 0 && isFirstPartyCommonRoot(parts[0]) {
			add(parts[0], "common_root")
		}
		if len(parts) > 0 && isFirstPartyTestRoot(parts[0]) {
			add(parts[0], "test_root")
		}
		for i, part := range parts[:len(parts)-1] {
			if part == "src" {
				add(strings.Join(parts[:i+1], "/"), "src_root")
			}
			if isFirstPartyTestRoot(part) {
				add(strings.Join(parts[:i+1], "/"), "test_root")
			}
		}
	}
	for dir := range initDirs {
		dir = normalizeFirstPartySourceRoot(dir)
		if dir == "" {
			continue
		}
		parts := strings.Split(dir, "/")
		if len(parts) == 1 && pyCounts[dir] >= 2 {
			add(dir, "python_package_root")
		}
		if len(parts) >= 2 && strings.Contains("/"+dir+"/", "/src/") && pyCounts[dir] >= 2 {
			add(dir, "python_package_root")
		}
	}
	if len(rootMarkers) > 0 && rootSourceCount >= 2 {
		add(".", "repo_root")
	}
	for dir := range moduleMarkerDirs {
		if dir == "." {
			continue
		}
		if sourceCountsByDir[dir] >= 1 {
			add(dir, "module_root")
		}
	}

	out := make([]firstPartySourceRoot, 0, len(roots))
	for path, kind := range roots {
		out = append(out, firstPartySourceRoot{path: path, kind: kind})
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].path) == len(out[j].path) {
			return out[i].path < out[j].path
		}
		return len(out[i].path) > len(out[j].path)
	})
	return out
}

func bestFirstPartySourceRoot(rel string, roots []firstPartySourceRoot) firstPartySourceRoot {
	rel = normalizeFirstPartySourceRel(rel)
	if rel == "" {
		return firstPartySourceRoot{}
	}
	for _, root := range roots {
		if root.path == "." {
			return root
		}
		if rel == root.path || strings.HasPrefix(rel, root.path+"/") {
			return root
		}
	}
	return firstPartySourceRoot{}
}

func capFirstPartySourceCandidates(candidates []adapters.Candidate, limit int) []adapters.Candidate {
	if limit <= 0 || len(candidates) <= limit {
		sortCandidates(candidates)
		return candidates
	}
	groups := map[string][]adapters.Candidate{}
	for _, candidate := range candidates {
		role := strings.TrimSpace(toMetadataString(candidate.Metadata["source_role"]))
		if role == "" {
			role = "implementation"
		}
		groups[role] = append(groups[role], candidate)
	}
	for role := range groups {
		sort.Slice(groups[role], func(i, j int) bool {
			if groups[role][i].DiscoveryScore == groups[role][j].DiscoveryScore {
				return groups[role][i].RelPath < groups[role][j].RelPath
			}
			return groups[role][i].DiscoveryScore > groups[role][j].DiscoveryScore
		})
	}
	quotas := []struct {
		role  string
		limit int
	}{
		{role: "implementation", limit: 1550},
		{role: "test", limit: 800},
		{role: "test_doc_example", limit: 100},
		{role: "fixture", limit: 50},
	}
	var selected []adapters.Candidate
	used := map[string]int{}
	for _, quota := range quotas {
		rows := groups[quota.role]
		n := quota.limit
		if n > len(rows) {
			n = len(rows)
		}
		selected = append(selected, rows[:n]...)
		used[quota.role] = n
	}
	if len(selected) < limit {
		var overflow []adapters.Candidate
		for role, rows := range groups {
			overflow = append(overflow, rows[used[role]:]...)
		}
		sort.Slice(overflow, func(i, j int) bool {
			if overflow[i].DiscoveryScore == overflow[j].DiscoveryScore {
				return overflow[i].RelPath < overflow[j].RelPath
			}
			return overflow[i].DiscoveryScore > overflow[j].DiscoveryScore
		})
		remaining := limit - len(selected)
		if remaining > len(overflow) {
			remaining = len(overflow)
		}
		selected = append(selected, overflow[:remaining]...)
	}
	sortCandidates(selected)
	return selected
}

func firstPartySourceRole(rel string) string {
	rel = normalizeFirstPartySourceRel(rel)
	if rel == "" || !firstPartySourceLooksSourcePath(rel) {
		return ""
	}
	if firstPartySourceLooksTestPath(rel) {
		if firstPartySourceLooksDocumentationExample(rel) {
			return "test_doc_example"
		}
		return "test"
	}
	if firstPartySourceHasSegment(rel, "fixtures", "fixture", "testdata", "__fixtures__") {
		return "fixture"
	}
	return "implementation"
}

func firstPartySourceLooksSourcePath(path string) bool {
	switch strings.ToLower(filepath.Ext(filepath.ToSlash(path))) {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rb", ".rs", ".php",
		".java", ".kt", ".kts", ".cs", ".vue", ".svelte", ".c", ".cc", ".cpp",
		".cxx", ".h", ".hpp", ".mjs", ".cjs", ".mts", ".cts", ".sql", ".lua",
		".dart", ".swift", ".scala", ".clj", ".cljs", ".ex", ".exs", ".erl",
		".hrl", ".zig", ".nim", ".jl", ".r", ".sh", ".bash", ".zsh", ".ps1",
		".proto", ".graphql", ".gql":
		return true
	default:
		return false
	}
}

func firstPartySourceLooksTestPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	name := strings.TrimSuffix(base, ext)
	if firstPartySourceHasSegment(path, "test", "tests", "__tests__", "spec", "e2e", "e2e-tests") {
		return true
	}
	switch {
	case ext == ".go" && strings.HasSuffix(base, "_test.go"):
		return true
	case ext == ".py" && (strings.HasPrefix(base, "test_") || strings.HasSuffix(name, "_test")):
		return true
	case ext == ".rb" && strings.HasSuffix(base, "_spec.rb"):
		return true
	case (ext == ".js" || ext == ".jsx" || ext == ".ts" || ext == ".tsx" || ext == ".mjs" || ext == ".cjs" || ext == ".mts" || ext == ".cts") &&
		(strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".spec") || strings.HasSuffix(name, "-test") || strings.HasSuffix(name, "-spec")):
		return true
	case ext == ".java" && (strings.HasSuffix(name, "test") || strings.HasSuffix(name, "tests") || strings.HasSuffix(name, "it")):
		return true
	case (ext == ".kt" || ext == ".kts") && (strings.HasSuffix(name, "test") || strings.HasSuffix(name, "spec")):
		return true
	case ext == ".php" && strings.HasSuffix(name, "test"):
		return true
	case ext == ".rs" && strings.HasSuffix(name, "_test"):
		return true
	default:
		return false
	}
}

func firstPartySourceLooksNoisePath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(path)
	if strings.HasSuffix(base, ".min.js") ||
		strings.HasSuffix(base, ".map") ||
		strings.HasSuffix(base, ".snap") ||
		strings.HasSuffix(base, ".pb.go") ||
		strings.HasSuffix(base, "_generated.go") ||
		strings.HasSuffix(base, ".gen.go") ||
		strings.Contains(base, "generated") ||
		strings.Contains(base, "bundle") {
		return true
	}
	return firstPartySourceHasSegment(path,
		".git", ".devspecs", "node_modules", "vendor", "vendors", "dist", "build",
		"coverage", "target", "tmp", "temp", ".next", ".nuxt", ".turbo", ".venv",
		"venv", "__pycache__", "__snapshots__", "snapshots", "third_party",
		"third-party", "external",
	)
}

func firstPartySourceLooksDocumentationExample(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	return firstPartySourceHasSegment(path,
		"docs", "doc", "docs_src", "documentation", "examples", "example",
		"tutorial", "tutorials", "samples", "sample",
	)
}

func firstPartySourceDiscoveryScore(role, rootKind string) float64 {
	score := 0.72
	switch role {
	case "test":
		score = 0.9
	case "implementation":
		score = 0.82
	case "test_doc_example":
		score = 0.58
	case "fixture":
		score = 0.48
	}
	switch rootKind {
	case "python_package_root", "src_root", "module_root":
		score += 0.04
	case "test_root":
		score += 0.02
	case "repo_root":
		score -= 0.06
	}
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func firstPartyAncestorDirs(rel string) []string {
	rel = normalizeFirstPartySourceRel(rel)
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir == "." || dir == "" {
		return nil
	}
	parts := strings.Split(dir, "/")
	out := make([]string, 0, len(parts))
	for i := range parts {
		out = append(out, strings.Join(parts[:i+1], "/"))
	}
	return out
}

func isFirstPartyRepoRootMarker(base string) bool {
	switch strings.ToLower(base) {
	case "go.mod", "package.json", "pyproject.toml", "setup.py", "setup.cfg",
		"go.work", "cargo.toml", "pom.xml", "build.gradle", "build.gradle.kts", "mix.exs",
		"composer.json", "gemfile":
		return true
	default:
		return false
	}
}

func isFirstPartyCommonRoot(part string) bool {
	switch strings.ToLower(part) {
	case "src", "lib", "app", "apps", "packages", "crates", "internal", "pkg",
		"cmd", "services", "modules", "components", "server", "client", "api",
		"backend", "frontend", "web", "ui", "core", "plugins", "extensions":
		return true
	default:
		return false
	}
}

func isFirstPartyTestRoot(part string) bool {
	switch strings.ToLower(part) {
	case "test", "tests", "__tests__", "spec", "e2e", "e2e-tests", "integration":
		return true
	default:
		return false
	}
}

func firstPartySourceRootRank(kind string) int {
	switch kind {
	case "python_package_root", "src_root", "module_root":
		return 4
	case "test_root":
		return 3
	case "common_root":
		return 2
	case "repo_root":
		return 1
	default:
		return 0
	}
}

func normalizeFirstPartySourceRel(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	if path == "" || filepath.IsAbs(path) || strings.HasPrefix(path, "../") || strings.Contains(path, "/../") {
		return ""
	}
	return path
}

func normalizeFirstPartySourceRoot(path string) string {
	path = normalizeFirstPartySourceRel(path)
	if path == "" {
		return ""
	}
	if path == "." {
		return "."
	}
	return strings.Trim(path, "/")
}

func firstPartySourceHasSegment(path string, segments ...string) bool {
	parts := strings.Split(strings.ToLower(filepath.ToSlash(path)), "/")
	for _, part := range parts {
		for _, segment := range segments {
			if part == segment {
				return true
			}
		}
	}
	return false
}

func toMetadataString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []string:
		return strings.Join(v, "\n")
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
