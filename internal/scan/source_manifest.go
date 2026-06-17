package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/devspecs-com/devspecs-cli/internal/adapters/sourcecontext"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

var sourceManifestImportPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z0-9_./:\-@]+)`),
	regexp.MustCompile(`(?m)^\s*from\s+([A-Za-z0-9_./:\-@]+)\s+import\b`),
	regexp.MustCompile(`(?m)\b(?:import|require)\s*\(\s*['"` + "`" + `]([^'"` + "`" + `]{1,160})['"` + "`" + `]`),
	regexp.MustCompile(`(?m)\bfrom\s+['"` + "`" + `]([^'"` + "`" + `]{1,160})['"` + "`" + `]`),
	regexp.MustCompile(`(?m)^\s*use\s+([A-Za-z0-9_:]+)`),
	regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([A-Za-z0-9_.*]+)\s*;`),
	regexp.MustCompile(`(?m)^\s*#include\s+[<"]([^>"]+)[>"]`),
}

const sourceManifestMaxImportsPerFile = 6

var (
	sourceManifestMaxFTSSymbolsPerFile       = 16
	sourceManifestModuleRootSoftFullFiles    = 5000
	sourceManifestMinModuleRootFilesPerRepo  = 1500
	sourceManifestMaxModuleRootFilesPerRepo  = 8000
	sourceManifestModuleRootBudgetPercent    = 50
	sourceManifestModuleRootSeedFilesPerRoot = 2
)

type sourceManifestCandidate struct {
	file fileInventoryEntry
	rel  string
	root firstPartySourceRoot
	role string
}

func (s *Scanner) rebuildSourceManifest(ctx context.Context, repoRoot, repoID, now string) (*SourceManifestDiagnostics, error) {
	diagnostics := &SourceManifestDiagnostics{
		Enabled:         true,
		IgnoredByReason: map[string]int{},
		RowsByRoot:      map[string]int{},
		RowsByLanguage:  map[string]int{},
		RowsByRole:      map[string]int{},
	}
	inventoryResult, err := collectFileInventory(ctx, repoRoot)
	if err != nil {
		return diagnostics, err
	}
	inventory := inventoryResult.files
	diagnostics.InventoryFiles = len(inventory)
	roots := detectFirstPartySourceRoots(inventory)
	var candidates []sourceManifestCandidate
	var moduleRootCandidates []sourceManifestCandidate
	var files []store.SourceManifestFileInput
	var symbols []store.SourceManifestSymbolInput
	var tests []store.SourceManifestTestInput
	var imports []store.SourceManifestImportInput
	var ftsRows []store.SourceManifestFTSInput

	for _, file := range inventory {
		if err := ctx.Err(); err != nil {
			return diagnostics, err
		}
		rel := normalizeFirstPartySourceRel(file.relPath)
		if rel == "" || !firstPartySourceLooksSourcePath(rel) {
			continue
		}
		diagnostics.SourceLikeFiles++
		if firstPartySourceLooksTestPath(rel) {
			diagnostics.TestLikeFiles++
		}
		if file.size > firstPartySourceMaxFileBytes {
			diagnostics.IgnoredByReason["too_large"]++
			continue
		}
		root := bestFirstPartySourceRoot(rel, roots)
		if root.path == "" {
			diagnostics.IgnoredByReason["no_source_root"]++
			continue
		}
		role := firstPartySourceRole(rel)
		if role == "" {
			diagnostics.IgnoredByReason["missing_role"]++
			continue
		}
		if firstPartySourceLooksNoisePath(rel) {
			diagnostics.IgnoredByReason["noise_path"]++
			continue
		}
		if firstPartySourceLooksDocumentationExample(rel) && role != "test_doc_example" {
			diagnostics.IgnoredByReason["documentation_example"]++
			continue
		}
		candidate := sourceManifestCandidate{file: file, rel: rel, root: root, role: role}
		if root.kind == "module_root" {
			moduleRootCandidates = append(moduleRootCandidates, candidate)
			continue
		}
		candidates = append(candidates, candidate)
	}
	moduleRootLimit := sourceManifestModuleRootCandidateLimit(len(moduleRootCandidates))
	selectedModuleRootCandidates, skippedModuleRootCandidates := capSourceManifestModuleRootCandidates(moduleRootCandidates, moduleRootLimit)
	if skippedModuleRootCandidates > 0 {
		diagnostics.IgnoredByReason["module_root_cap"] += skippedModuleRootCandidates
	}
	candidates = append(candidates, selectedModuleRootCandidates...)
	sortSourceManifestCandidates(candidates)

	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return diagnostics, err
		}
		body, err := os.ReadFile(candidate.file.primaryPath)
		if err != nil {
			diagnostics.IgnoredByReason["read_error"]++
			continue
		}
		fileID := sourceManifestFileID(repoID, candidate.rel)
		language := sourcecontext.LanguageForPath(candidate.rel)
		if language == "" {
			language = strings.TrimPrefix(strings.ToLower(filepath.Ext(filepath.Base(candidate.rel))), ".")
		}
		if language == "" {
			language = "unknown"
		}
		files = append(files, store.SourceManifestFileInput{
			FileID:          fileID,
			RepoID:          repoID,
			Path:            candidate.rel,
			ContentHash:     sourceManifestContentHash(body),
			SizeBytes:       candidate.file.size,
			Language:        language,
			SourceRoot:      candidate.root.path,
			SourceRootKind:  candidate.root.kind,
			SourceRole:      candidate.role,
			FirstPartyScore: firstPartySourceDiscoveryScore(candidate.role, candidate.root.kind),
		})
		diagnostics.IndexedFiles++
		if candidate.role == "test" || candidate.role == "test_doc_example" {
			diagnostics.IndexedTests++
		}
		diagnostics.RowsByRoot[candidate.root.path]++
		diagnostics.RowsByLanguage[language]++
		diagnostics.RowsByRole[candidate.role]++

		symbolValues := sourcecontext.ExtractSymbols(string(body))
		for _, symbol := range symbolValues {
			symbols = append(symbols, store.SourceManifestSymbolInput{FileID: fileID, Symbol: symbol, Kind: "symbol"})
		}
		testValues := sourcecontext.ExtractTestNames(string(body))
		for _, testName := range testValues {
			tests = append(tests, store.SourceManifestTestInput{FileID: fileID, TestName: testName})
		}
		importValues := compactSourceManifestImports(extractSourceManifestImports(string(body)))
		for _, importRef := range importValues {
			imports = append(imports, store.SourceManifestImportInput{FileID: fileID, ImportRef: importRef})
		}
		ftsRows = append(ftsRows, store.SourceManifestFTSInput{
			FileID:     fileID,
			Path:       candidate.rel,
			PathTerms:  sourceManifestPathTerms(candidate.rel),
			SourceRoot: candidate.root.path,
			Language:   language,
			SourceRole: candidate.role,
			Symbols:    strings.Join(compactSourceManifestFTSSymbols(symbolValues), "\n"),
			TestNames:  strings.Join(testValues, "\n"),
			Imports:    strings.Join(importValues, "\n"),
		})
	}
	if err := s.db.ReplaceRepoSourceManifest(repoID, files, symbols, tests, imports, ftsRows, now); err != nil {
		return diagnostics, err
	}
	diagnostics.SymbolRows = len(symbols)
	diagnostics.TestRows = len(tests)
	diagnostics.ImportRows = len(imports)
	diagnostics.FTSRows = len(ftsRows)
	if len(diagnostics.IgnoredByReason) == 0 {
		diagnostics.IgnoredByReason = nil
	}
	return diagnostics, nil
}

func sourceManifestFileID(repoID, rel string) string {
	sum := sha256.Sum256([]byte(repoID + "\x00" + filepath.ToSlash(rel)))
	return "srcm_" + hex.EncodeToString(sum[:])[:24]
}

func sourceManifestContentHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])[:24]
}

func sourceManifestModuleRootCandidateLimit(count int) int {
	if count <= 0 {
		return 0
	}
	if sourceManifestModuleRootSoftFullFiles > 0 && count <= sourceManifestModuleRootSoftFullFiles {
		return count
	}
	limit := (count * sourceManifestModuleRootBudgetPercent) / 100
	if limit < sourceManifestMinModuleRootFilesPerRepo {
		limit = sourceManifestMinModuleRootFilesPerRepo
	}
	if sourceManifestMaxModuleRootFilesPerRepo > 0 && limit > sourceManifestMaxModuleRootFilesPerRepo {
		limit = sourceManifestMaxModuleRootFilesPerRepo
	}
	if limit > count {
		limit = count
	}
	return limit
}

func capSourceManifestModuleRootCandidates(candidates []sourceManifestCandidate, limit int) ([]sourceManifestCandidate, int) {
	if limit <= 0 {
		return nil, len(candidates)
	}
	if len(candidates) <= limit {
		sortSourceManifestCandidates(candidates)
		return candidates, 0
	}
	selected, selectedByRel := seedSourceManifestModuleRoots(candidates, limit)
	if len(selected) >= limit {
		sortSourceManifestCandidates(selected)
		return selected, len(candidates) - len(selected)
	}
	remainingCandidates := make([]sourceManifestCandidate, 0, len(candidates)-len(selected))
	for _, candidate := range candidates {
		if !selectedByRel[candidate.rel] {
			remainingCandidates = append(remainingCandidates, candidate)
		}
	}
	selected = append(selected, selectSourceManifestCandidatesByRoleBudget(remainingCandidates, limit-len(selected))...)
	sortSourceManifestCandidates(selected)
	return selected, len(candidates) - len(selected)
}

func seedSourceManifestModuleRoots(candidates []sourceManifestCandidate, limit int) ([]sourceManifestCandidate, map[string]bool) {
	selectedByRel := map[string]bool{}
	if limit <= 0 || sourceManifestModuleRootSeedFilesPerRoot <= 0 {
		return nil, selectedByRel
	}
	byRoot := map[string][]sourceManifestCandidate{}
	for _, candidate := range candidates {
		root := candidate.root.path
		if root == "" {
			root = "."
		}
		byRoot[root] = append(byRoot[root], candidate)
	}
	roots := make([]string, 0, len(byRoot))
	for root := range byRoot {
		roots = append(roots, root)
		sortSourceManifestCandidatesByPriority(byRoot[root])
	}
	sort.Strings(roots)
	var selected []sourceManifestCandidate
	for _, root := range roots {
		if len(selected) >= limit {
			break
		}
		rows := byRoot[root]
		takenForRoot := 0
		for _, role := range []string{"implementation", "test", "test_doc_example", "fixture"} {
			if len(selected) >= limit || takenForRoot >= sourceManifestModuleRootSeedFilesPerRoot {
				break
			}
			candidate, ok := firstSourceManifestCandidateForRole(rows, role, selectedByRel)
			if !ok {
				continue
			}
			selected = append(selected, candidate)
			selectedByRel[candidate.rel] = true
			takenForRoot++
		}
		for _, candidate := range rows {
			if len(selected) >= limit || takenForRoot >= sourceManifestModuleRootSeedFilesPerRoot {
				break
			}
			if selectedByRel[candidate.rel] {
				continue
			}
			selected = append(selected, candidate)
			selectedByRel[candidate.rel] = true
			takenForRoot++
		}
	}
	return selected, selectedByRel
}

func firstSourceManifestCandidateForRole(candidates []sourceManifestCandidate, role string, selectedByRel map[string]bool) (sourceManifestCandidate, bool) {
	for _, candidate := range candidates {
		if selectedByRel[candidate.rel] || candidate.role != role {
			continue
		}
		return candidate, true
	}
	return sourceManifestCandidate{}, false
}

func selectSourceManifestCandidatesByRoleBudget(candidates []sourceManifestCandidate, limit int) []sourceManifestCandidate {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	if len(candidates) <= limit {
		sortSourceManifestCandidates(candidates)
		return candidates
	}
	groups := map[string][]sourceManifestCandidate{}
	for _, candidate := range candidates {
		role := candidate.role
		if role == "" {
			role = "implementation"
		}
		groups[role] = append(groups[role], candidate)
	}
	for role := range groups {
		sortSourceManifestCandidatesByPriority(groups[role])
	}
	quotas := []struct {
		role    string
		percent int
		min     int
	}{
		{role: "test", percent: 35, min: 1},
		{role: "implementation", percent: 60, min: 1},
		{role: "test_doc_example", percent: 3, min: 0},
		{role: "fixture", percent: 2, min: 0},
	}
	var selected []sourceManifestCandidate
	used := map[string]int{}
	for _, quota := range quotas {
		if len(selected) >= limit {
			break
		}
		rows := groups[quota.role]
		if len(rows) == 0 {
			continue
		}
		n := (limit * quota.percent) / 100
		if n < quota.min {
			n = quota.min
		}
		if remaining := limit - len(selected); n > remaining {
			n = remaining
		}
		if n > len(rows) {
			n = len(rows)
		}
		selected = append(selected, rows[:n]...)
		used[quota.role] = n
	}
	if len(selected) < limit {
		var overflow []sourceManifestCandidate
		for role, rows := range groups {
			overflow = append(overflow, rows[used[role]:]...)
		}
		sortSourceManifestCandidatesByPriority(overflow)
		remaining := limit - len(selected)
		if remaining > len(overflow) {
			remaining = len(overflow)
		}
		selected = append(selected, overflow[:remaining]...)
	}
	sortSourceManifestCandidates(selected)
	return selected
}

func sortSourceManifestCandidates(candidates []sourceManifestCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].rel < candidates[j].rel
	})
}

func sortSourceManifestCandidatesByPriority(candidates []sourceManifestCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		left := firstPartySourceDiscoveryScore(candidates[i].role, candidates[i].root.kind)
		right := firstPartySourceDiscoveryScore(candidates[j].role, candidates[j].root.kind)
		if left == right {
			return candidates[i].rel < candidates[j].rel
		}
		return left > right
	})
}

func sourceManifestPathTerms(rel string) string {
	rel = filepath.ToSlash(rel)
	seen := map[string]bool{}
	var terms []string
	add := func(value string) {
		value = strings.Trim(strings.ToLower(value), "._- ")
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		terms = append(terms, value)
	}
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		add(b.String())
		b.Reset()
	}
	for _, r := range rel {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	add(base)
	sort.Strings(terms)
	return strings.Join(terms, " ")
}

func compactSourceManifestFTSSymbols(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	if sourceManifestMaxFTSSymbolsPerFile <= 0 || len(values) <= sourceManifestMaxFTSSymbolsPerFile {
		return values
	}
	return values[:sourceManifestMaxFTSSymbolsPerFile]
}

func extractSourceManifestImports(body string) []string {
	if len(body) > 160*1024 {
		body = body[:160*1024]
	}
	seen := map[string]bool{}
	var out []string
	for _, pattern := range sourceManifestImportPatterns {
		for _, match := range pattern.FindAllStringSubmatch(body, 160) {
			if len(match) < 2 {
				continue
			}
			value := strings.TrimSpace(match[1])
			value = strings.Trim(value, "\"'`;")
			if value == "" || len(value) > 180 || seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
			if len(out) >= 120 {
				return out
			}
		}
	}
	return out
}

func compactSourceManifestImports(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var local []string
	var other []string
	for _, value := range values {
		value = normalizeSourceManifestImportRef(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		if sourceManifestLooksLocalImport(value) {
			local = append(local, value)
		} else {
			other = append(other, value)
		}
	}
	sort.Strings(local)
	sort.Strings(other)
	out := append(local, other...)
	if len(out) > sourceManifestMaxImportsPerFile {
		out = out[:sourceManifestMaxImportsPerFile]
	}
	return out
}

func normalizeSourceManifestImportRef(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"'`;")
	if value == "" || len(value) > 180 || strings.ContainsAny(value, " \t\r\n") {
		return ""
	}
	return value
}

func sourceManifestLooksLocalImport(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	switch {
	case strings.HasPrefix(value, "."):
		return true
	case strings.HasPrefix(value, "crate::"), strings.HasPrefix(value, "self::"), strings.HasPrefix(value, "super::"):
		return true
	}
	first := value
	for _, sep := range []string{"/", ".", "::"} {
		if idx := strings.Index(first, sep); idx >= 0 {
			first = first[:idx]
		}
	}
	switch strings.ToLower(first) {
	case "app", "apps", "backend", "client", "cmd", "components", "core", "crates",
		"frontend", "internal", "lib", "modules", "packages", "pkg", "plugin",
		"plugins", "script", "scripts", "sdk", "sdks", "service", "services", "src",
		"test", "tests", "ui", "web":
		return true
	default:
		return false
	}
}
