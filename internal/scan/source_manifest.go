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

const sourceManifestMaxImportsPerFile = 8

func (s *Scanner) rebuildSourceManifest(ctx context.Context, repoRoot, repoID, now string) (*SourceManifestDiagnostics, error) {
	diagnostics := &SourceManifestDiagnostics{
		Enabled:         true,
		IgnoredByReason: map[string]int{},
		RowsByRoot:      map[string]int{},
		RowsByLanguage:  map[string]int{},
		RowsByRole:      map[string]int{},
	}
	inventory, err := collectFileInventory(ctx, repoRoot)
	if err != nil {
		return diagnostics, err
	}
	diagnostics.InventoryFiles = len(inventory)
	roots := detectFirstPartySourceRoots(inventory)
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
		body, err := os.ReadFile(file.primaryPath)
		if err != nil {
			diagnostics.IgnoredByReason["read_error"]++
			continue
		}
		fileID := sourceManifestFileID(repoID, rel)
		language := sourcecontext.LanguageForPath(rel)
		if language == "" {
			language = strings.TrimPrefix(strings.ToLower(filepath.Ext(filepath.Base(rel))), ".")
		}
		if language == "" {
			language = "unknown"
		}
		hash := sha256.Sum256(body)
		files = append(files, store.SourceManifestFileInput{
			FileID:          fileID,
			RepoID:          repoID,
			Path:            rel,
			ContentHash:     hex.EncodeToString(hash[:]),
			SizeBytes:       file.size,
			Language:        language,
			SourceRoot:      root.path,
			SourceRootKind:  root.kind,
			SourceRole:      role,
			FirstPartyScore: firstPartySourceDiscoveryScore(role, root.kind),
		})
		diagnostics.IndexedFiles++
		if role == "test" || role == "test_doc_example" {
			diagnostics.IndexedTests++
		}
		diagnostics.RowsByRoot[root.path]++
		diagnostics.RowsByLanguage[language]++
		diagnostics.RowsByRole[role]++

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
			Path:       rel,
			PathTerms:  sourceManifestPathTerms(rel),
			SourceRoot: root.path,
			Language:   language,
			SourceRole: role,
			Symbols:    strings.Join(symbolValues, "\n"),
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
