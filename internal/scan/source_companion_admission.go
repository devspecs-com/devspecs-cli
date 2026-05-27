package scan

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

const (
	sourceCompanionAdapterName       = "source_context"
	testCompanionAdapterName         = "test_case"
	sourceCompanionAdmissionReason   = "test_source_companion"
	maxSourceCompanionFileBytes      = 256 * 1024
	maxSourceCompanionsPerRepo       = 500
	maxSourceCompanionsPerTest       = 3
	maxStemSourceCompanionsPerTest   = 1
	maxRawSourceCompanionsPerTest    = 10
	maxCompanionTestPathsPerArtifact = 8
	maxCompanionExamples             = 10
)

type rawSourceCompanionCandidate struct {
	path       string
	testPath   string
	signal     string
	confidence string
	score      int
}

type sourceCompanionAggregate struct {
	path       string
	signals    map[string]bool
	testPaths  map[string]bool
	confidence string
	score      int
}

var sourceCompanionImportPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)\bfrom\s+([A-Za-z0-9_./:-]+)\s+import\b`),
	regexp.MustCompile(`(?m)\bimport\s+([A-Za-z0-9_./:-]+)(?:\s+as\s+[A-Za-z_][A-Za-z0-9_]*)?$`),
	regexp.MustCompile(`(?m)\bimport(?:\s+type)?(?:[^'"` + "`" + `\n]+?\s+from\s*)?\s*['"]([^'"` + "`" + `]+)['"]`),
	regexp.MustCompile(`(?m)\brequire\(\s*['"]([^'"]+)['"]\s*\)`),
	regexp.MustCompile(`(?m)\buse\s+([A-Za-z0-9_:]+)`),
	regexp.MustCompile(`(?m)^\s*mod\s+([A-Za-z_][A-Za-z0-9_]*)\s*;`),
	regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z0-9_.]+);`),
}

func buildTestSourceCompanionCandidates(ctx context.Context, repoRoot string, testCandidates, existingSourceCandidates []adapters.Candidate) (*SourceCompanionAdmissionDiagnostics, []adapters.Candidate) {
	if len(testCandidates) == 0 {
		return nil, nil
	}
	testFiles := uniqueTestCandidatePaths(testCandidates)
	diagnostics := &SourceCompanionAdmissionDiagnostics{
		Enabled:                  true,
		TestFiles:                len(testFiles),
		ExistingSourceCandidates: len(existingSourceCandidates),
		RejectedByReason:         map[string]int{},
	}
	existingSources := map[string]bool{}
	for _, candidate := range existingSourceCandidates {
		if rel := normalizeCompanionRel(candidate.RelPath); rel != "" {
			existingSources[rel] = true
		}
	}

	goModule := readGoModulePath(repoRoot)
	aggregates := map[string]*sourceCompanionAggregate{}
	alreadyPresent := map[string]bool{}
	for _, testPath := range testFiles {
		if err := ctx.Err(); err != nil {
			recordCompanionRejection(diagnostics, testPath, "context_cancelled", "")
			break
		}
		body, ok := readCompanionFile(repoRoot, testPath)
		if !ok {
			recordCompanionRejection(diagnostics, testPath, "test_read_failed", "")
			continue
		}
		raw := deriveRawSourceCompanions(testPath, body, goModule)
		sortRawSourceCompanions(raw)
		admittedForTest := 0
		stemForTest := 0
		for _, candidate := range raw {
			if admittedForTest >= maxSourceCompanionsPerTest {
				recordCompanionRejection(diagnostics, candidate.path, "per_test_cap", candidate.signal)
				continue
			}
			if candidate.signal == "stem" && stemForTest >= maxStemSourceCompanionsPerTest {
				recordCompanionRejection(diagnostics, candidate.path, "stem_per_test_cap", candidate.signal)
				continue
			}
			rel, reason := validateSourceCompanionPath(ctx, repoRoot, candidate.path)
			if reason != "" {
				recordCompanionRejection(diagnostics, candidate.path, reason, candidate.signal)
				continue
			}
			diagnostics.CandidatesConsidered++
			if existingSources[rel] {
				if !alreadyPresent[rel] {
					diagnostics.AlreadyPresent++
					alreadyPresent[rel] = true
				}
				continue
			}
			aggregate := aggregates[rel]
			if aggregate == nil {
				aggregate = &sourceCompanionAggregate{
					path:       rel,
					signals:    map[string]bool{},
					testPaths:  map[string]bool{},
					confidence: candidate.confidence,
				}
				aggregates[rel] = aggregate
			}
			aggregate.signals[candidate.signal] = true
			aggregate.testPaths[candidate.testPath] = true
			if candidate.score > aggregate.score {
				aggregate.score = candidate.score
			}
			if confidenceRank(candidate.confidence) > confidenceRank(aggregate.confidence) {
				aggregate.confidence = candidate.confidence
			}
			admittedForTest++
			if candidate.signal == "stem" {
				stemForTest++
			}
		}
	}

	selected := sortedSourceCompanionAggregates(aggregates)
	if len(selected) > maxSourceCompanionsPerRepo {
		for _, rejected := range selected[maxSourceCompanionsPerRepo:] {
			recordCompanionRejection(diagnostics, rejected.path, "repo_cap", "")
		}
		selected = selected[:maxSourceCompanionsPerRepo]
	}

	candidates := make([]adapters.Candidate, 0, len(selected))
	for _, aggregate := range selected {
		body, ok := readCompanionFile(repoRoot, aggregate.path)
		if !ok {
			recordCompanionRejection(diagnostics, aggregate.path, "source_read_failed", "")
			continue
		}
		abs := filepath.Join(repoRoot, filepath.FromSlash(aggregate.path))
		signals := sortedMapKeys(aggregate.signals)
		testPaths := limitedSortedMapKeys(aggregate.testPaths, maxCompanionTestPathsPerArtifact)
		candidates = append(candidates, adapters.Candidate{
			PrimaryPath:      abs,
			RelPath:          aggregate.path,
			AdapterName:      sourceCompanionAdapterName,
			UnitBody:         body,
			DiscoveryScore:   float64(aggregate.score) / 100,
			DiscoveryReasons: append([]string{sourceCompanionAdmissionReason}, signals...),
			Metadata: map[string]any{
				"admission_reason":     sourceCompanionAdmissionReason,
				"companion_signals":    signals,
				"companion_test_paths": testPaths,
				"companion_confidence": aggregate.confidence,
				"source_path":          aggregate.path,
			},
		})
		diagnostics.Admitted++
		if len(diagnostics.TopAdmitted) < maxCompanionExamples {
			diagnostics.TopAdmitted = append(diagnostics.TopAdmitted, SourceCompanionAdmissionExample{
				Path:       aggregate.path,
				Signals:    signals,
				Confidence: aggregate.confidence,
				TestPaths:  testPaths,
			})
		}
	}
	if len(diagnostics.RejectedByReason) == 0 {
		diagnostics.RejectedByReason = nil
	}
	return diagnostics, candidates
}

func uniqueTestCandidatePaths(candidates []adapters.Candidate) []string {
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if rel := normalizeCompanionRel(candidate.RelPath); rel != "" {
			seen[rel] = true
		}
	}
	return sortedMapKeys(seen)
}

func deriveRawSourceCompanions(testPath, body, goModule string) []rawSourceCompanionCandidate {
	var out []rawSourceCompanionCandidate
	add := func(path, signal, confidence string, score int) {
		path = normalizeCompanionRel(path)
		if path == "" {
			return
		}
		out = append(out, rawSourceCompanionCandidate{path: path, testPath: testPath, signal: signal, confidence: confidence, score: score})
	}
	for _, path := range stemSourceCompanionPaths(testPath) {
		score := 90
		signal := "stem"
		confidence := "high"
		if strings.Contains(filepath.ToSlash(testPath), "/src/test/java/") {
			signal = "mirrored_layout"
			score = 94
		}
		add(path, signal, confidence, score)
	}
	for _, importRef := range extractSourceCompanionImports(body) {
		for _, path := range importSourceCompanionPaths(testPath, importRef, goModule) {
			score := 92
			confidence := "high"
			if !strings.HasPrefix(importRef, ".") && !strings.Contains(importRef, "/") {
				score = 78
				confidence = "medium"
			}
			add(path, "direct_import", confidence, score)
		}
	}
	return dedupeRawSourceCompanions(out)
}

func stemSourceCompanionPaths(testPath string) []string {
	testPath = normalizeCompanionRel(testPath)
	dir := filepath.ToSlash(filepath.Dir(testPath))
	base := filepath.Base(testPath)
	ext := strings.ToLower(filepath.Ext(base))
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	sourceStem, ok := stripTestStem(stem)
	if !ok || sourceStem == "" {
		return nil
	}
	exts := sourceExtensionsForTest(ext)
	var out []string
	addDirCandidates := func(targetDir string) {
		targetDir = filepath.ToSlash(strings.Trim(targetDir, "/"))
		for _, sourceExt := range exts {
			if targetDir == "." || targetDir == "" {
				out = append(out, sourceStem+sourceExt)
				continue
			}
			out = append(out, targetDir+"/"+sourceStem+sourceExt)
		}
	}
	addDirCandidates(dir)
	if strings.HasSuffix(dir, "/__tests__") || dir == "__tests__" {
		addDirCandidates(filepath.ToSlash(filepath.Dir(dir)))
	}
	if strings.Contains(testPath, "/src/test/java/") || strings.HasPrefix(testPath, "src/test/java/") {
		target := strings.Replace(testPath, "/src/test/java/", "/src/main/java/", 1)
		if strings.HasPrefix(testPath, "src/test/java/") {
			target = "src/main/java/" + strings.TrimPrefix(testPath, "src/test/java/")
		}
		out = append(out, filepath.ToSlash(filepath.Dir(target))+"/"+sourceStem+".java")
	}
	for _, mirrored := range mirroredTestSourceDirs(dir) {
		addDirCandidates(mirrored)
	}
	return uniqueSorted(out)
}

func stripTestStem(stem string) (string, bool) {
	original := stem
	for _, suffix := range []string{"_test", "-test", ".test", "_spec", "-spec", ".spec", "_tests", "-tests", ".tests"} {
		if strings.HasSuffix(stem, suffix) {
			return strings.TrimSuffix(stem, suffix), true
		}
	}
	for _, prefix := range []string{"test_", "test-", "spec_", "spec-"} {
		if strings.HasPrefix(stem, prefix) {
			return strings.TrimPrefix(stem, prefix), true
		}
	}
	if strings.HasSuffix(stem, "Test") && len(stem) > len("Test") {
		return strings.TrimSuffix(stem, "Test"), true
	}
	return original, false
}

func sourceExtensionsForTest(ext string) []string {
	switch ext {
	case ".go":
		return []string{".go"}
	case ".py":
		return []string{".py"}
	case ".rs":
		return []string{".rs"}
	case ".java":
		return []string{".java"}
	case ".ts":
		return []string{".ts", ".tsx", ".js", ".jsx"}
	case ".tsx":
		return []string{".tsx", ".ts", ".jsx", ".js"}
	case ".js", ".mjs", ".cjs":
		return []string{".js", ".jsx", ".ts", ".tsx"}
	case ".jsx":
		return []string{".jsx", ".js", ".tsx", ".ts"}
	default:
		return nil
	}
}

func mirroredTestSourceDirs(dir string) []string {
	dir = filepath.ToSlash(strings.Trim(dir, "/"))
	if dir == "tests" || dir == "test" {
		return []string{"src"}
	}
	parts := strings.Split(dir, "/")
	var out []string
	for i, part := range parts {
		switch part {
		case "tests", "test", "__tests__":
			next := append([]string(nil), parts...)
			next[i] = "src"
			out = append(out, strings.Join(next, "/"))
			if part == "__tests__" {
				out = append(out, strings.Join(append([]string(nil), parts[:i]...), "/"))
			}
		}
	}
	return uniqueSorted(out)
}

func extractSourceCompanionImports(body string) []string {
	if len(body) > 128*1024 {
		body = body[:128*1024]
	}
	seen := map[string]bool{}
	for _, pattern := range sourceCompanionImportPatterns {
		for _, match := range pattern.FindAllStringSubmatch(body, 80) {
			if len(match) < 2 {
				continue
			}
			value := strings.Trim(strings.TrimSpace(match[1]), `"'`)
			if value == "" || len(value) > 180 || strings.ContainsAny(value, " \t\r\n") {
				continue
			}
			seen[value] = true
		}
	}
	return sortedMapKeys(seen)
}

func importSourceCompanionPaths(testPath, importRef, goModule string) []string {
	importRef = strings.TrimSpace(strings.Trim(importRef, `"'`))
	if importRef == "" {
		return nil
	}
	testDir := filepath.ToSlash(filepath.Dir(testPath))
	if strings.HasPrefix(importRef, ".") {
		base := normalizeCompanionRel(filepath.ToSlash(filepath.Join(testDir, importRef)))
		return importPathVariants(base, testPath)
	}
	if strings.HasPrefix(importRef, "crate::") {
		rest := strings.TrimPrefix(importRef, "crate::")
		rest = strings.ReplaceAll(rest, "::", "/")
		return []string{"src/" + rest + ".rs", "src/" + rest + "/mod.rs"}
	}
	if strings.Contains(importRef, "::") {
		rest := strings.ReplaceAll(importRef, "::", "/")
		return []string{"src/" + rest + ".rs", "src/" + rest + "/mod.rs"}
	}
	if goModule != "" && strings.HasPrefix(importRef, goModule+"/") {
		rest := strings.TrimPrefix(importRef, goModule+"/")
		return []string{rest + ".go", rest + "/" + filepath.Base(rest) + ".go"}
	}
	if strings.Contains(importRef, "/") {
		if !localPackageImportRoot(importRef) {
			return nil
		}
		return importPathVariants(importRef, testPath)
	}
	if strings.Contains(importRef, ".") {
		return dottedImportPathVariants(importRef)
	}
	return nil
}

func importPathVariants(base, testPath string) []string {
	base = normalizeCompanionRel(base)
	base = trimKnownTriangulationExtension(base)
	var out []string
	for _, ext := range importVariantExtensions(testPath) {
		out = append(out, base+ext)
	}
	switch strings.ToLower(filepath.Ext(testPath)) {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		out = append(out, base+"/index.ts", base+"/index.tsx", base+"/index.js")
	case ".rs":
		out = append(out, base+"/mod.rs")
	case ".py":
		out = append(out, base+"/__init__.py")
	}
	return uniqueSorted(out)
}

func importVariantExtensions(testPath string) []string {
	switch strings.ToLower(filepath.Ext(testPath)) {
	case ".go":
		return []string{".go"}
	case ".py":
		return []string{".py"}
	case ".rs":
		return []string{".rs"}
	case ".java":
		return []string{".java"}
	case ".ts":
		return []string{".ts", ".tsx", ".js", ".jsx"}
	case ".tsx":
		return []string{".tsx", ".ts", ".jsx", ".js"}
	case ".js", ".mjs", ".cjs":
		return []string{".js", ".jsx", ".ts", ".tsx"}
	case ".jsx":
		return []string{".jsx", ".js", ".tsx", ".ts"}
	default:
		return []string{".ts", ".tsx", ".js", ".jsx", ".py", ".go", ".rs", ".java", ".vue"}
	}
}

func localPackageImportRoot(importRef string) bool {
	importRef = strings.TrimPrefix(importRef, "./")
	if strings.HasPrefix(importRef, "@") {
		return false
	}
	first := strings.Split(importRef, "/")[0]
	switch first {
	case "app", "apps", "backend", "cmd", "components", "frontend", "internal", "lib", "pkg", "script", "scripts", "service", "services", "src":
		return true
	default:
		return false
	}
}

func dottedImportPathVariants(importRef string) []string {
	path := strings.ReplaceAll(importRef, ".", "/")
	var out []string
	out = append(out, path+".py", path+"/__init__.py")
	out = append(out, "src/main/java/"+path+".java")
	return uniqueSorted(out)
}

func validateSourceCompanionPath(ctx context.Context, repoRoot, rel string) (string, string) {
	rel = normalizeCompanionRel(rel)
	if rel == "" {
		return "", "invalid_path"
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
		return "", "outside_repo"
	}
	if sourceCompanionPathBlocked(rel) {
		return "", "generated_vendor_or_build"
	}
	if sourcePathLooksLikeTest(rel) {
		return "", "test_like_source"
	}
	if !supportedSourceCompanionPath(rel) {
		return "", "unsupported_extension"
	}
	if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(rel, false) {
		return "", "ignored"
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(rel))
	rootAbs, rootErr := filepath.Abs(repoRoot)
	absPath, absErr := filepath.Abs(abs)
	if rootErr != nil || absErr != nil || !pathWithinRoot(rootAbs, absPath) {
		return "", "outside_repo"
	}
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		return "", "unresolved"
	}
	if info.Size() > maxSourceCompanionFileBytes {
		return "", "size"
	}
	return rel, ""
}

func sourceCompanionPathBlocked(rel string) bool {
	rel = filepath.ToSlash(strings.ToLower(rel))
	base := filepath.Base(rel)
	if strings.HasSuffix(base, ".min.js") || strings.HasSuffix(base, ".min.css") {
		return true
	}
	switch base {
	case "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "go.sum", "cargo.lock", "gemfile.lock", "poetry.lock", "composer.lock":
		return true
	}
	if strings.HasSuffix(base, ".pb.go") || strings.HasSuffix(base, "_generated.go") || strings.Contains(base, "generated") {
		return true
	}
	for _, part := range strings.Split(rel, "/") {
		switch part {
		case ".git", ".devspecs", "node_modules", "dist", "build", ".next", "coverage", "tmp", "vendor", ".venv", "__pycache__", "target", "generated", "tests", "test", "__tests__", "e2e", "fixtures", "__fixtures__":
			return true
		}
	}
	return false
}

func supportedSourceCompanionPath(rel string) bool {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(rel)))
	if base == "dockerfile" || base == "containerfile" {
		return false
	}
	switch filepath.Ext(base) {
	case ".go", ".py", ".rs", ".java", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue":
		return true
	default:
		return false
	}
}

func pathWithinRoot(rootAbs, absPath string) bool {
	rootAbs = filepath.Clean(rootAbs)
	absPath = filepath.Clean(absPath)
	if absPath == rootAbs {
		return true
	}
	rel, err := filepath.Rel(rootAbs, absPath)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

func readCompanionFile(repoRoot, rel string) (string, bool) {
	abs := filepath.Join(repoRoot, filepath.FromSlash(normalizeCompanionRel(rel)))
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func readGoModulePath(repoRoot string) string {
	data, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func recordCompanionRejection(d *SourceCompanionAdmissionDiagnostics, path, reason, signal string) {
	if d == nil || reason == "" {
		return
	}
	if d.RejectedByReason == nil {
		d.RejectedByReason = map[string]int{}
	}
	d.RejectedByReason[reason]++
	if strings.Contains(reason, "cap") {
		d.SkippedByCap++
	}
	if len(d.TopRejected) < maxCompanionExamples {
		d.TopRejected = append(d.TopRejected, SourceCompanionRejectionExample{Path: normalizeCompanionRel(path), Reason: reason, Signal: signal})
	}
}

func sortRawSourceCompanions(candidates []rawSourceCompanionCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			if candidates[i].path == candidates[j].path {
				return candidates[i].signal < candidates[j].signal
			}
			return candidates[i].path < candidates[j].path
		}
		return candidates[i].score > candidates[j].score
	})
}

func dedupeRawSourceCompanions(candidates []rawSourceCompanionCandidate) []rawSourceCompanionCandidate {
	byKey := map[string]rawSourceCompanionCandidate{}
	for _, candidate := range candidates {
		key := candidate.path + "\x00" + candidate.signal
		if existing, ok := byKey[key]; !ok || candidate.score > existing.score {
			byKey[key] = candidate
		}
	}
	out := make([]rawSourceCompanionCandidate, 0, len(byKey))
	for _, candidate := range byKey {
		out = append(out, candidate)
	}
	sortRawSourceCompanions(out)
	return out
}

func sortedSourceCompanionAggregates(values map[string]*sourceCompanionAggregate) []*sourceCompanionAggregate {
	out := make([]*sourceCompanionAggregate, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score == out[j].score {
			if confidenceRank(out[i].confidence) == confidenceRank(out[j].confidence) {
				return out[i].path < out[j].path
			}
			return confidenceRank(out[i].confidence) > confidenceRank(out[j].confidence)
		}
		return out[i].score > out[j].score
	})
	return out
}

func confidenceRank(confidence string) int {
	switch confidence {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func normalizeCompanionRel(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "." {
		return ""
	}
	return strings.Trim(path, "/")
}
