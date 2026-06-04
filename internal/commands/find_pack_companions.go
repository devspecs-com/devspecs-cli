package commands

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

const (
	findPackCompanionMaxTotal       = 8
	findPackCompanionMaxCochanged   = 4
	findPackCompanionMaxExactCommit = 2
	findPackCompanionGitMaxCommits  = 24

	findPackCompanionModeOff        = "off"
	findPackCompanionModeGeneric    = "generic"
	findPackCompanionModeGenericGit = "generic_git"
	findPackCompanionModeAll        = "all"
)

func normalizeFindPackCompanionMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	mode = strings.ReplaceAll(mode, "-", "_")
	switch mode {
	case "", findPackCompanionModeAll:
		return findPackCompanionModeAll
	case "none", "disabled", "disable", findPackCompanionModeOff:
		return findPackCompanionModeOff
	case findPackCompanionModeGeneric:
		return findPackCompanionModeGeneric
	case "git", "generic+git", "generic_git", "genericgit":
		return findPackCompanionModeGenericGit
	default:
		return ""
	}
}

func validFindPackCompanionModes() []string {
	return []string{
		findPackCompanionModeOff,
		findPackCompanionModeGeneric,
		findPackCompanionModeGenericGit,
		findPackCompanionModeAll,
	}
}

func findPackCompanionModeIncludesGeneric(mode string) bool {
	mode = normalizeFindPackCompanionMode(mode)
	return mode == findPackCompanionModeGeneric || mode == findPackCompanionModeGenericGit || mode == findPackCompanionModeAll
}

func findPackCompanionModeIncludesGit(mode string) bool {
	mode = normalizeFindPackCompanionMode(mode)
	return mode == findPackCompanionModeGenericGit || mode == findPackCompanionModeAll
}

func findPackCompanionModeIncludesCommandFamily(mode string) bool {
	return normalizeFindPackCompanionMode(mode) == findPackCompanionModeAll
}

func addFindPackCompanionCandidates(ctx context.Context, repoRoot, query string, matches, all []retrieval.Candidate, mode string) []retrieval.Candidate {
	mode = normalizeFindPackCompanionMode(mode)
	if mode == "" || mode == findPackCompanionModeOff || len(matches) == 0 || len(all) == 0 {
		return matches
	}
	byPath := findPackCandidatePathIndex(all)
	if len(byPath) == 0 {
		return matches
	}
	seen := findPackCandidatePathSet(matches)
	var additions []retrieval.Candidate
	add := func(path, reason string) {
		if len(additions) >= findPackCompanionMaxTotal {
			return
		}
		path = normalizeFindGitReceiptPath(path)
		if path == "" || seen[path] {
			return
		}
		candidate, ok := byPath[path]
		if !ok {
			candidate, ok = findPackFilesystemCompanionCandidate(repoRoot, path)
		}
		if !ok {
			return
		}
		seen[path] = true
		candidate.Metadata = copyFindPackCompanionMetadata(candidate.Metadata)
		candidate.Metadata["retrieval_expansion_reason"] = reason
		if candidate.Metadata["pack_tier"] == "" {
			candidate.Metadata["pack_tier"] = retrieval.PackTierRelated
			candidate.Metadata["pack_tier_reason"] = reason
		}
		additions = append(additions, candidate)
	}

	for _, match := range firstFindPackCandidates(matches, 8) {
		path := normalizeFindGitReceiptPath(match.Path)
		if path == "" {
			continue
		}
		if findPackCompanionModeIncludesGeneric(mode) {
			for _, companion := range findPackDirectTestCompanionPaths(path, byPath) {
				add(companion, "test_companion")
			}
			for _, companion := range findPackSameDirectoryTestCompanionPaths(path, byPath) {
				add(companion, "same_directory_test_companion")
			}
			for _, companion := range findPackFilesystemSameDirectoryTestCompanionPaths(repoRoot, path) {
				add(companion, "same_directory_test_companion")
			}
		}
		if findPackCompanionModeIncludesCommandFamily(mode) {
			for _, companion := range findPackCommandFamilyCompanionPaths(path, query, byPath) {
				add(companion, "command_family_companion")
			}
		}
	}
	if findPackCompanionModeIncludesCommandFamily(mode) {
		for _, companion := range findPackQueryCommandCompanionPaths(query, byPath) {
			add(companion, "query_command_family_companion")
		}
	}
	if findPackCompanionModeIncludesGit(mode) {
		for _, companion := range findPackCochangedTestCompanionPaths(ctx, repoRoot, query, matches, byPath, seen) {
			add(companion, "git_cochanged_test_companion")
		}
		if findPackMatchesUseCodeTaskFamily(matches) {
			for _, companion := range findPackExactCommitTouchedCompanionPaths(ctx, repoRoot, query, matches, seen) {
				add(companion, "exact_commit_touched_companion")
			}
		}
	}
	if len(additions) == 0 {
		return matches
	}
	out := make([]retrieval.Candidate, 0, len(matches)+len(additions))
	out = append(out, matches...)
	out = append(out, additions...)
	return out
}

func findPackFilesystemCompanionCandidate(repoRoot, path string) (retrieval.Candidate, bool) {
	repoRoot = strings.TrimSpace(repoRoot)
	path = normalizeFindGitReceiptPath(path)
	if repoRoot == "" || path == "" || filepath.IsAbs(path) || strings.HasPrefix(path, "../") || strings.Contains(path, "/../") {
		return retrieval.Candidate{}, false
	}
	if !findPackLooksTestPath(path) && !findPackLooksSourcePath(path) {
		return retrieval.Candidate{}, false
	}
	absRoot, rootErr := filepath.Abs(repoRoot)
	absPath, pathErr := filepath.Abs(filepath.Join(repoRoot, filepath.FromSlash(path)))
	if rootErr != nil || pathErr != nil || !findPackPathWithinRoot(absRoot, absPath) {
		return retrieval.Candidate{}, false
	}
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() || info.Size() > 256*1024 {
		return retrieval.Candidate{}, false
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return retrieval.Candidate{}, false
	}
	subtype := ""
	if findPackLooksTestPath(path) {
		subtype = "test_case"
	}
	return retrieval.Candidate{
		ID:       "companion:" + path,
		Path:     path,
		Kind:     "source_context",
		Subtype:  subtype,
		Title:    path + " (" + strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".") + ")",
		Status:   "unknown",
		Body:     string(data),
		Source:   path,
		Metadata: map[string]string{"admission_reason": "query_time_pack_companion"},
	}, true
}

func findPackPathWithinRoot(rootAbs, absPath string) bool {
	rootAbs = filepath.Clean(rootAbs)
	absPath = filepath.Clean(absPath)
	if absPath == rootAbs {
		return true
	}
	rel, err := filepath.Rel(rootAbs, absPath)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func findPackCandidatePathIndex(candidates []retrieval.Candidate) map[string]retrieval.Candidate {
	out := map[string]retrieval.Candidate{}
	for _, candidate := range candidates {
		path := normalizeFindGitReceiptPath(candidate.Path)
		if path == "" {
			path = normalizeFindGitReceiptPath(candidate.Source)
		}
		if path == "" {
			continue
		}
		if _, seen := out[path]; !seen {
			out[path] = candidate
		}
	}
	return out
}

func findPackCandidatePathSet(candidates []retrieval.Candidate) map[string]bool {
	out := map[string]bool{}
	for _, candidate := range candidates {
		path := normalizeFindGitReceiptPath(candidate.Path)
		if path != "" {
			out[path] = true
		}
	}
	return out
}

func copyFindPackCompanionMetadata(metadata map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range metadata {
		out[k] = v
	}
	return out
}

func findPackDirectTestCompanionPaths(path string, byPath map[string]retrieval.Candidate) []string {
	if findPackLooksTestPath(path) || !findPackLooksSourcePath(path) {
		return nil
	}
	dir := filepath.ToSlash(filepath.Dir(path))
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	name := strings.TrimSuffix(base, ext)
	candidates := []string{}
	add := func(rel string) {
		rel = normalizeFindGitReceiptPath(rel)
		if rel != "" {
			candidates = append(candidates, rel)
		}
	}
	join := func(file string) string {
		if dir == "." || dir == "" {
			return file
		}
		return dir + "/" + file
	}
	switch ext {
	case ".go":
		add(join(name + "_test.go"))
	case ".py":
		add(join("test_" + name + ".py"))
		add(join(name + "_test.py"))
	case ".rb":
		add(join(name + "_spec.rb"))
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		add(join(name + ".test" + ext))
		add(join(name + ".spec" + ext))
		add(join("__tests__/" + name + ".test" + ext))
		add(join("__tests__/" + name + ".spec" + ext))
	case ".java":
		add(join(name + "Test.java"))
		add(join(name + "Tests.java"))
		add(join(name + "IT.java"))
	case ".kt", ".kts":
		add(join(name + "Test" + ext))
		add(join(name + "Spec" + ext))
	case ".rs":
		add(join(name + "_test.rs"))
	}
	return findPackUniqueSorted(candidates)
}

func findPackSameDirectoryTestCompanionPaths(path string, byPath map[string]retrieval.Candidate) []string {
	if findPackLooksTestPath(path) || !findPackLooksSourcePath(path) {
		return nil
	}
	dir := filepath.ToSlash(filepath.Dir(path))
	stem := findPackComparableStem(path)
	if stem == "" {
		return nil
	}
	var out []string
	for candidatePath := range byPath {
		if !findPackLooksTestPath(candidatePath) || filepath.ToSlash(filepath.Dir(candidatePath)) != dir {
			continue
		}
		if findPackStemsRelated(stem, findPackComparableStem(candidatePath)) {
			out = append(out, candidatePath)
		}
	}
	sort.Strings(out)
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

func findPackCommandFamilyCompanionPaths(path, query string, byPath map[string]retrieval.Candidate) []string {
	name, ok := findPackCommandNameFromPath(path)
	if !ok {
		return nil
	}
	var out []string
	add := func(path string) {
		out = append(out, path)
	}
	add("internal/commands/" + name + ".go")
	add("internal/commands/" + name + "_test.go")
	queryLower := strings.ToLower(query)
	if findPackQueryAny(queryLower, "output", "outputs", "cache", "cached", "json", "drilldown", "drilldowns", "consistency", "read", "reads") {
		add("internal/commands/read_commands_test.go")
	}
	if findPackQueryAny(queryLower, "fresh", "freshness", "refresh", "auto", "auto-scan", "autoscan") || strings.Contains(queryLower, "first use") {
		add("internal/commands/refresh.go")
		add("internal/commands/freshness_test.go")
	}
	if name == "find" || findPackQueryAny(queryLower, "pack", "packs", "packing") {
		add("internal/commands/find_pack.go")
		add("internal/commands/find_pack_test.go")
	}
	if name == "scan" || strings.Contains(queryLower, "scan output") {
		add("internal/commands/scan_output_test.go")
	}
	return findPackUniqueSorted(out)
}

func findPackQueryCommandCompanionPaths(query string, byPath map[string]retrieval.Candidate) []string {
	queryLower := strings.ToLower(query)
	aliases := map[string]string{
		"capture":    "capture",
		"context":    "context",
		"eval":       "eval",
		"evaluation": "eval",
		"find":       "find",
		"init":       "init",
		"map":        "map",
		"refresh":    "refresh",
		"resume":     "resume",
		"scan":       "scan",
		"show":       "show",
		"status":     "status",
	}
	var out []string
	for term, command := range aliases {
		if !findPackQueryHasWord(queryLower, term) {
			continue
		}
		out = append(out,
			"internal/commands/"+command+".go",
			"internal/commands/"+command+"_test.go",
		)
	}
	return findPackUniqueSorted(out)
}

func findPackFilesystemSameDirectoryTestCompanionPaths(repoRoot, path string) []string {
	if repoRoot == "" || findPackLooksTestPath(path) || !findPackLooksSourcePath(path) {
		return nil
	}
	dir := filepath.ToSlash(filepath.Dir(path))
	stem := findPackComparableStem(path)
	if stem == "" {
		return nil
	}
	absDir := filepath.Join(repoRoot, filepath.FromSlash(dir))
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		candidatePath := normalizeFindGitReceiptPath(dir + "/" + entry.Name())
		if !findPackLooksTestPath(candidatePath) {
			continue
		}
		if findPackStemsRelated(stem, findPackComparableStem(candidatePath)) {
			out = appendUniqueString(out, candidatePath)
		}
	}
	sort.Strings(out)
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

func findPackCochangedTestCompanionPaths(ctx context.Context, repoRoot, query string, matches []retrieval.Candidate, byPath map[string]retrieval.Candidate, seen map[string]bool) []string {
	if repoRoot == "" {
		return nil
	}
	paths := findPackCandidatePaths(matches, 8)
	if len(paths) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, findGitReceiptTimeout)
	defer cancel()
	if !findGitRepoAvailable(ctx, repoRoot) {
		return nil
	}
	commits, ok := findGitLogForPaths(ctx, repoRoot, paths)
	if !ok || len(commits) == 0 {
		return nil
	}
	pathSet := map[string]bool{}
	for _, path := range paths {
		pathSet[path] = true
	}
	anchors := findGitReceiptQueryAnchors(query)
	var out []string
	for _, commit := range firstParsedFindGitCommits(commits, findPackCompanionGitMaxCommits) {
		if findGitReceiptCommitNoisy(commit) || len(commit.paths) > 80 || len(commitMatchedPackPaths(commit.paths, pathSet)) == 0 {
			continue
		}
		if !findPackCommitMentionsAnyAnchor(commit, anchors) && len(commitMatchedPackPaths(commit.paths, pathSet)) < 2 {
			continue
		}
		for _, path := range commit.paths {
			path = normalizeFindGitReceiptPath(path)
			if path == "" || seen[path] || !findPackLooksTestPath(path) {
				continue
			}
			out = appendUniqueString(out, path)
			if len(out) >= findPackCompanionMaxCochanged {
				return out
			}
		}
	}
	return out
}

func findPackMatchesUseCodeTaskFamily(matches []retrieval.Candidate) bool {
	for _, match := range matches {
		if match.Metadata != nil && retrieval.AnchorFirstModeUsesCodeTaskFamilyRanking(match.Metadata["source_family_mode"]) {
			return true
		}
	}
	return false
}

func findPackExactCommitTouchedCompanionPaths(ctx context.Context, repoRoot, query string, matches []retrieval.Candidate, seen map[string]bool) []string {
	if repoRoot == "" {
		return nil
	}
	paths := findPackCandidatePaths(matches, 8)
	if len(paths) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, findGitReceiptTimeout)
	defer cancel()
	if !findGitRepoAvailable(ctx, repoRoot) {
		return nil
	}
	commits, ok := findGitLogForPaths(ctx, repoRoot, paths)
	if !ok || len(commits) == 0 {
		return nil
	}
	pathSet := map[string]bool{}
	for _, path := range paths {
		pathSet[path] = true
	}
	anchors := findGitReceiptQueryAnchors(query)
	selectedFamilies := findPackBroadPathFamilies(paths)
	var out []string
	for _, commit := range firstParsedFindGitCommits(commits, findPackCompanionGitMaxCommits) {
		if len(out) >= findPackCompanionMaxExactCommit {
			return out
		}
		if findGitReceiptCommitNoisy(commit) || len(commitMatchedPackPaths(commit.paths, pathSet)) == 0 {
			continue
		}
		if !findPackCommitMentionsAnyAnchor(commit, anchors) {
			continue
		}
		allTouched := findGitShowCommitFiles(ctx, repoRoot, commit.sha)
		if len(allTouched) == 0 || len(allTouched) > 80 {
			continue
		}
		scoredPaths := scoreFindPackExactCommitTouchedPaths(allTouched, anchors, selectedFamilies, seen)
		for _, scored := range scoredPaths {
			path := scored.path
			path = normalizeFindGitReceiptPath(path)
			out = appendUniqueString(out, path)
			if len(out) >= findPackCompanionMaxExactCommit {
				return out
			}
		}
	}
	return out
}

type findPackScoredTouchedPath struct {
	path  string
	score int
}

func scoreFindPackExactCommitTouchedPaths(paths []string, anchors []string, selectedFamilies map[string]bool, seen map[string]bool) []findPackScoredTouchedPath {
	var scored []findPackScoredTouchedPath
	for _, path := range paths {
		path = normalizeFindGitReceiptPath(path)
		if path == "" || seen[path] || findGitReceiptRelatedPathNoise(path) {
			continue
		}
		if !findPackLooksSourcePath(path) && !findPackLooksTestPath(path) {
			continue
		}
		score := findPackExactCommitTouchedPathScore(path, anchors, selectedFamilies)
		if score < 5 {
			continue
		}
		scored = append(scored, findPackScoredTouchedPath{path: path, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].path < scored[j].path
	})
	return scored
}

func findGitShowCommitFiles(ctx context.Context, repoRoot, sha string) []string {
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return nil
	}
	out, err := runFindGitShowNameOnly(ctx, repoRoot, sha)
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		path := normalizeFindGitReceiptPath(line)
		if path != "" {
			paths = appendUniqueString(paths, path)
		}
	}
	return paths
}

func runFindGitShowNameOnly(ctx context.Context, repoRoot, sha string) (string, error) {
	args := []string{
		"-C", filepath.Clean(repoRoot),
		"show",
		"--pretty=format:",
		"--name-only",
		"--no-renames",
		sha,
	}
	out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	return string(out), err
}

func findPackExactCommitTouchedPathScore(path string, anchors []string, selectedFamilies map[string]bool) int {
	lower := strings.ToLower(normalizeFindGitReceiptPath(path))
	if lower == "" {
		return 0
	}
	score := 0
	if findPackLooksTestPath(lower) {
		score += 7
	} else if findPackLooksSourcePath(lower) {
		score += 3
	}
	for _, anchor := range anchors {
		if len(anchor) >= 4 && strings.Contains(lower, strings.ToLower(anchor)) {
			score += 5
			break
		}
	}
	for family := range selectedFamilies {
		if family != "" && (strings.HasPrefix(lower, family+"/") || lower == family) {
			score += 2
			break
		}
	}
	base := strings.ToLower(filepath.Base(lower))
	if base == "__init__.py" || base == "index.ts" || base == "index.tsx" || base == "index.js" || base == "index.jsx" {
		score -= 3
	}
	return score
}

func findPackBroadPathFamilies(paths []string) map[string]bool {
	out := map[string]bool{}
	for _, path := range paths {
		path = strings.ToLower(normalizeFindGitReceiptPath(path))
		if path == "" {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		segments := strings.Split(strings.Trim(dir, "/"), "/")
		if len(segments) == 0 || segments[0] == "" || segments[0] == "." {
			continue
		}
		switch segments[0] {
		case "pkg", "src", "lib", "app", "apps", "packages", "internal", "fastapi":
			out[segments[0]] = true
			if len(segments) >= 2 {
				out[segments[0]+"/"+segments[1]] = true
			}
		default:
			if len(segments) >= 2 {
				out[segments[0]+"/"+segments[1]] = true
			} else {
				out[segments[0]] = true
			}
		}
	}
	return out
}

func findPackCandidatePaths(candidates []retrieval.Candidate, limit int) []string {
	var out []string
	for _, candidate := range candidates {
		if limit > 0 && len(out) >= limit {
			break
		}
		path := normalizeFindGitReceiptPath(candidate.Path)
		if path != "" {
			out = appendUniqueString(out, path)
		}
	}
	return out
}

func firstFindPackCandidates(values []retrieval.Candidate, limit int) []retrieval.Candidate {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func firstParsedFindGitCommits(values []parsedFindGitCommit, limit int) []parsedFindGitCommit {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func findPackCommitMentionsAnyAnchor(commit parsedFindGitCommit, anchors []string) bool {
	if len(anchors) == 0 {
		return false
	}
	text := strings.ToLower(commit.subject + "\n" + commit.body + "\n" + strings.Join(commit.paths, "\n"))
	for _, anchor := range anchors {
		if anchor != "" && strings.Contains(text, strings.ToLower(anchor)) {
			return true
		}
	}
	return false
}

func findPackUniqueSorted(values []string) []string {
	var out []string
	for _, value := range values {
		out = appendUniqueString(out, value)
	}
	sort.Strings(out)
	return out
}

func findPackCommandNameFromPath(path string) (string, bool) {
	path = normalizeFindGitReceiptPath(path)
	if !strings.HasPrefix(path, "internal/commands/") || findPackLooksTestPath(path) {
		return "", false
	}
	base := filepath.Base(path)
	if filepath.Ext(base) != ".go" {
		return "", false
	}
	name := strings.TrimSuffix(base, ".go")
	if name == "" {
		return "", false
	}
	if helper := findPackCommandHelperNameFromPath(path); helper != "" {
		return helper, true
	}
	if strings.Contains(name, "_") {
		return "", false
	}
	return name, true
}

func findPackCommandHelperNameFromPath(path string) string {
	path = normalizeFindGitReceiptPath(path)
	base := filepath.Base(path)
	if strings.ToLower(filepath.Ext(base)) != ".go" {
		return ""
	}
	name := strings.TrimSuffix(base, ".go")
	before, _, ok := strings.Cut(name, "_")
	if !ok || before == "" {
		return ""
	}
	switch before {
	case "capture", "context", "eval", "find", "init", "map", "refresh", "resume", "scan", "show", "status":
		return before
	default:
		return ""
	}
}

func findPackComparableStem(path string) string {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(path)))
	ext := strings.ToLower(filepath.Ext(base))
	stem := strings.TrimSuffix(base, ext)
	for _, suffix := range []string{"_test", ".test", ".spec", "_spec", "test", "tests", "it"} {
		stem = strings.TrimSuffix(stem, suffix)
	}
	stem = strings.TrimPrefix(stem, "test_")
	replacer := strings.NewReplacer("_", "", "-", "", ".", "")
	return replacer.Replace(stem)
}

func findPackStemsRelated(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	return left == right || strings.Contains(left, right) || strings.Contains(right, left)
}

func findPackLooksSourcePath(path string) bool {
	path = normalizeFindGitReceiptPath(path)
	if path == "" {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".rb", ".php", ".java", ".kt", ".kts", ".rs":
		return true
	default:
		return false
	}
}

func findPackLooksTestPath(path string) bool {
	path = strings.ToLower(normalizeFindGitReceiptPath(path))
	if path == "" {
		return false
	}
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	ext := strings.ToLower(filepath.Ext(base))
	switch {
	case ext == ".go" && strings.HasSuffix(base, "_test.go"):
		return true
	case ext == ".py" && (strings.HasPrefix(base, "test_") || strings.HasSuffix(name, "_test")):
		return true
	case ext == ".rb" && strings.HasSuffix(base, "_spec.rb"):
		return true
	case ext == ".php" && strings.HasSuffix(base, "test.php"):
		return true
	case (ext == ".js" || ext == ".jsx" || ext == ".ts" || ext == ".tsx" || ext == ".mjs" || ext == ".cjs") &&
		(strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".spec")):
		return true
	case ext == ".java" && (strings.HasSuffix(name, "test") || strings.HasSuffix(name, "tests") || strings.HasSuffix(name, "it")):
		return true
	case (ext == ".kt" || ext == ".kts") && (strings.HasSuffix(name, "test") || strings.HasSuffix(name, "spec")):
		return true
	case ext == ".rs" && strings.HasSuffix(name, "_test"):
		return true
	}
	for _, segment := range strings.Split(path, "/") {
		switch segment {
		case "tests", "__tests__", "spec", "cypress", "e2e":
			return true
		}
	}
	return false
}

func findPackQueryAny(queryLower string, words ...string) bool {
	for _, word := range words {
		if findPackQueryHasWord(queryLower, word) {
			return true
		}
	}
	return false
}

func findPackQueryHasWord(queryLower, word string) bool {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return false
	}
	if strings.Contains(word, "-") {
		return strings.Contains(queryLower, word)
	}
	for _, token := range strings.FieldsFunc(queryLower, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-')
	}) {
		if strings.EqualFold(strings.Trim(token, "_-"), word) {
			return true
		}
	}
	return false
}
