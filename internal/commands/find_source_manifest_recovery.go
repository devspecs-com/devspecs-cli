package commands

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

const (
	findSourceManifestRecoveryMaxAdditions = 4
	findSourceManifestRecoveryMaxPerFamily = 2
)

type findSourceManifestRecoveryCandidate struct {
	candidate retrieval.Candidate
	family    string
	score     int
	reasons   []string
}

func applyFindSourceManifestConsumptionV1Scout(db *store.DB, fp store.FilterParams, query string, matches, all []retrieval.Candidate) []retrieval.Candidate {
	base := applyFindSourceManifestConsumptionScout(query, matches, all)
	if len(base) == 0 {
		return base
	}
	repoRoot := fp.RepoRoot
	if repoRoot == "" && db != nil {
		repoRoot = findSourceManifestRecoveryRepoRoot(db, base)
	}
	base = append(base, selectFindFilesystemSourceRecoveryCandidates(repoRoot, query, base, findSourceManifestRecoveryMaxAdditions)...)
	if db == nil {
		return base
	}
	rows, err := loadFindSourceTestManifestRows(db, fp)
	if err != nil || len(rows) == 0 {
		return base
	}
	additions := selectFindSourceManifestRecoveryCandidates(query, base, all, rows, findSourceManifestRecoveryMaxAdditions)
	if len(additions) == 0 {
		return base
	}
	out := make([]retrieval.Candidate, 0, len(base)+len(additions))
	out = append(out, base...)
	out = append(out, additions...)
	return out
}

func findSourceManifestRecoveryRepoRoot(db *store.DB, candidates []retrieval.Candidate) string {
	if db == nil {
		return ""
	}
	repoIDs := map[string]bool{}
	for _, candidate := range candidates {
		if candidate.Metadata == nil {
			continue
		}
		if repoID := strings.TrimSpace(candidate.Metadata["repo_id"]); repoID != "" {
			repoIDs[repoID] = true
		}
	}
	if len(repoIDs) == 1 {
		for repoID := range repoIDs {
			var root string
			if err := db.QueryRow("SELECT root_path FROM repos WHERE id = ?", repoID).Scan(&root); err == nil {
				return root
			}
		}
	}
	var root string
	var count int
	rows, err := db.Query("SELECT root_path FROM repos LIMIT 2")
	if err != nil {
		return ""
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&root); err != nil {
			return ""
		}
		count++
	}
	if count == 1 {
		return root
	}
	return ""
}

func selectFindFilesystemSourceRecoveryCandidates(repoRoot, query string, selected []retrieval.Candidate, limit int) []retrieval.Candidate {
	if repoRoot == "" || limit <= 0 || len(selected) == 0 {
		return nil
	}
	terms := findSourceManifestRecoveryTerms(query)
	seen := findPackCandidatePathSet(selected)
	var scored []findSourceManifestRecoveryCandidate
	for _, candidate := range selected {
		for _, selectedPath := range []string{candidate.Source, candidate.Path} {
			for _, path := range findSourceRecoveryPathsFromSelectedTest(selectedPath) {
				if seen[path] {
					continue
				}
				companion, ok := findPackFilesystemCompanionCandidate(repoRoot, path)
				if !ok || findPackLooksTestPath(companion.Path) {
					continue
				}
				score := findSourceManifestConsumptionScore(companion, terms) + 18
				if score < 20 {
					continue
				}
				reasons := []string{"same_stem_source_recovery", "selected_test_source_companion"}
				companion = annotateFindSourceManifestRecoveryCandidate(companion, score, reasons)
				companion.Metadata["retrieval_expansion_reason"] = "filesystem_source_family_recovery"
				companion.Metadata["source_manifest_recovery_reasons"] = strings.Join(reasons, "\n")
				scored = append(scored, findSourceManifestRecoveryCandidate{
					candidate: companion,
					family:    findSourceTestPathFamily(path),
					score:     score,
					reasons:   reasons,
				})
				seen[path] = true
			}
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].candidate.Path < scored[j].candidate.Path
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	out := make([]retrieval.Candidate, 0, len(scored))
	for _, candidate := range scored {
		out = append(out, candidate.candidate)
	}
	return out
}

func findSourceRecoveryPathsFromSelectedTest(path string) []string {
	path = normalizeFindGitReceiptPath(path)
	if path == "" || !findPackLooksTestPath(path) {
		return nil
	}
	path = strings.SplitN(path, "#", 2)[0]
	dir := filepath.ToSlash(filepath.Dir(path))
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	name := strings.TrimSuffix(base, ext)
	stem := findSourceRecoverySourceStemFromTestName(name, ext)
	if stem == "" {
		return nil
	}
	sourceDir := findSourceRecoverySourceDirFromTestDir(dir)
	var out []string
	add := func(rel string) {
		rel = normalizeFindGitReceiptPath(rel)
		if rel != "" && !findPackLooksTestPath(rel) {
			out = appendUniqueString(out, rel)
		}
	}
	join := func(d, file string) string {
		if d == "." || d == "" {
			return file
		}
		return d + "/" + file
	}
	switch ext {
	case ".go":
		add(join(sourceDir, stem+".go"))
	case ".py":
		add(join(sourceDir, stem+".py"))
		if strings.HasPrefix(stem, "test_") {
			add(join(sourceDir, strings.TrimPrefix(stem, "test_")+".py"))
		}
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		add(join(sourceDir, stem+ext))
	case ".rs":
		add(join(sourceDir, stem+".rs"))
	case ".rb":
		add(join(sourceDir, stem+".rb"))
	case ".java":
		add(join(sourceDir, stem+".java"))
	case ".kt", ".kts":
		add(join(sourceDir, stem+ext))
	}
	sort.Strings(out)
	return out
}

func findSourceRecoverySourceStemFromTestName(name, ext string) string {
	stem := strings.TrimSpace(name)
	switch ext {
	case ".go":
		stem = strings.TrimSuffix(stem, "_test")
	case ".py":
		stem = strings.TrimPrefix(strings.TrimSuffix(stem, "_test"), "test_")
	case ".rb":
		stem = strings.TrimSuffix(stem, "_spec")
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		for _, suffix := range []string{".test", ".spec"} {
			stem = strings.TrimSuffix(stem, suffix)
		}
	case ".rs":
		stem = strings.TrimSuffix(stem, "_test")
	case ".java":
		for _, suffix := range []string{"Test", "Tests", "IT", "test", "tests", "it"} {
			stem = strings.TrimSuffix(stem, suffix)
		}
	case ".kt", ".kts":
		for _, suffix := range []string{"Test", "Spec", "test", "spec"} {
			stem = strings.TrimSuffix(stem, suffix)
		}
	}
	return strings.Trim(stem, "._-")
}

func findSourceRecoverySourceDirFromTestDir(dir string) string {
	dir = filepath.ToSlash(strings.Trim(dir, "/"))
	if dir == "" || dir == "." {
		return dir
	}
	parts := strings.Split(dir, "/")
	if len(parts) == 0 {
		return dir
	}
	last := strings.ToLower(parts[len(parts)-1])
	switch last {
	case "__tests__", "tests", "test", "spec", "specs", "e2e":
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}

func selectFindSourceManifestRecoveryCandidates(query string, selected, all []retrieval.Candidate, rows []findSourceTestManifestRow, limit int) []retrieval.Candidate {
	if limit <= 0 || len(rows) == 0 {
		return nil
	}
	ctx := findSourceManifestRecoveryContext(selected, query)
	if len(ctx.Paths) == 0 {
		return nil
	}
	terms := findSourceManifestRecoveryTerms(query)
	if len(terms) == 0 {
		return nil
	}
	seen := findPackCandidatePathSet(selected)
	byPath := findPackCandidatePathIndex(all)
	scored := make([]findSourceManifestRecoveryCandidate, 0, len(rows))
	for _, row := range rows {
		path := normalizeFindGitReceiptPath(row.Path)
		if path == "" || seen[path] {
			continue
		}
		candidate := findSourceManifestRecoveryCandidateFromRow(row, byPath)
		score, reasons := scoreFindSourceManifestRecoveryCandidate(candidate, row, ctx, terms)
		if score < findSourceManifestRecoveryThreshold(row, reasons) {
			continue
		}
		candidate = annotateFindSourceManifestRecoveryCandidate(candidate, score, reasons)
		scored = append(scored, findSourceManifestRecoveryCandidate{
			candidate: candidate,
			family:    findSourceTestPathFamily(path),
			score:     score,
			reasons:   reasons,
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].candidate.Path < scored[j].candidate.Path
	})
	var out []retrieval.Candidate
	familyCounts := map[string]int{}
	for _, candidate := range scored {
		if len(out) >= limit {
			break
		}
		if familyCounts[candidate.family] >= findSourceManifestRecoveryMaxPerFamily {
			continue
		}
		familyCounts[candidate.family]++
		out = append(out, candidate.candidate)
	}
	return out
}

func findSourceManifestRecoveryContext(candidates []retrieval.Candidate, query string) findSourceTestSourceContext {
	ctx := findSourceTestSourceContext{
		Stems:        map[string]bool{},
		Families:     map[string][]string{},
		Prefixes:     map[string]bool{},
		QueryTokens:  findSourceTestTokenSet(findSourceManifestRecoveryTerms(query)),
		SourceTokens: map[string]bool{},
	}
	for _, candidate := range candidates {
		if candidate.Kind != "source_context" && candidate.Metadata["source_type"] != "source_context" {
			continue
		}
		path := normalizeFindGitReceiptPath(candidate.Path)
		if path == "" {
			continue
		}
		ctx.Paths = appendUniqueString(ctx.Paths, path)
		ctx.Stems[findSourceTestStem(path)] = true
		family := findSourceTestPathFamily(path)
		ctx.Families[family] = appendUniqueString(ctx.Families[family], path)
		for prefix := range findSourceTestPathPrefixes(path) {
			ctx.Prefixes[prefix] = true
		}
		for token := range findSourceTestTokens(path + " " + candidate.Title + " " + candidate.Body) {
			ctx.SourceTokens[token] = true
		}
	}
	sort.Strings(ctx.Paths)
	return ctx
}

func findSourceManifestRecoveryCandidateFromRow(row findSourceTestManifestRow, byPath map[string]retrieval.Candidate) retrieval.Candidate {
	path := normalizeFindGitReceiptPath(row.Path)
	if candidate, ok := byPath[path]; ok {
		return candidate
	}
	subtype := ""
	if findSourceTestBehaviorTestRow(row) {
		subtype = "test_case"
	}
	metadata := map[string]string{
		"repo_id":              row.RepoID,
		"short_id":             row.FileID,
		"retrieval_candidate":  "source_manifest",
		"source_context_scope": "compact_manifest",
		"source_type":          "source_context",
		"source_path":          path,
		"source_role":          row.SourceRole,
		"source_root":          row.SourceRoot,
		"source_root_kind":     row.SourceRootKind,
		"language":             row.Language,
	}
	if row.Symbols != "" {
		metadata["source_symbols"] = row.Symbols
		metadata["symbols"] = row.Symbols
	}
	if row.TestNames != "" {
		metadata["test_name"] = row.TestNames
	}
	return retrieval.Candidate{
		ID:       "source_manifest_recovery:" + row.FileID,
		Path:     path,
		Kind:     "source_context",
		Subtype:  subtype,
		Title:    path + findSourceManifestRecoveryTitleSuffix(row),
		Status:   "indexed",
		Body:     renderFindSourceManifestRecoveryBody(row),
		Source:   path,
		Metadata: metadata,
	}
}

func findSourceManifestRecoveryTitleSuffix(row findSourceTestManifestRow) string {
	if row.Language == "" {
		return ""
	}
	return " (" + row.Language + ")"
}

func renderFindSourceManifestRecoveryBody(row findSourceTestManifestRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", normalizeFindGitReceiptPath(row.Path))
	fmt.Fprintln(&b, "Kind: source_context")
	if findSourceTestBehaviorTestRow(row) {
		fmt.Fprintln(&b, "Subtype: test_case")
	}
	if row.Language != "" {
		fmt.Fprintf(&b, "Language: %s\n", row.Language)
	}
	if row.SourceRole != "" {
		fmt.Fprintf(&b, "Source role: %s\n", row.SourceRole)
	}
	if row.SourceRoot != "" {
		fmt.Fprintf(&b, "Source root: %s\n", filepath.ToSlash(row.SourceRoot))
	}
	writeFindSourceManifestRecoveryBlock(&b, "Symbols", row.Symbols)
	writeFindSourceManifestRecoveryBlock(&b, "Test names", row.TestNames)
	return b.String()
}

func writeFindSourceManifestRecoveryBlock(b *strings.Builder, title, raw string) {
	values := findSourceTestReceiptList(raw, 12)
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s:\n", title)
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
}

func scoreFindSourceManifestRecoveryCandidate(candidate retrieval.Candidate, row findSourceTestManifestRow, ctx findSourceTestSourceContext, terms []string) (int, []string) {
	baseScore := findSourceManifestConsumptionScore(candidate, terms)
	receiptScore, receiptReasons, _, strong := scoreFindSourceTestReceipt(row, ctx)
	score := baseScore + int(receiptScore*0.45)
	reasons := []string{}
	if baseScore > 0 {
		reasons = append(reasons, "direct_query_score:"+strconv.Itoa(baseScore))
	}
	if receiptScore > 0 {
		reasons = append(reasons, "family_context_score:"+strconv.Itoa(int(receiptScore)))
		reasons = append(reasons, firstStrings(receiptReasons, 3)...)
	}
	if strong {
		score += 4
		reasons = append(reasons, "strong_family_context")
	}
	path := strings.ToLower(normalizeFindGitReceiptPath(candidate.Path))
	if findSourceManifestRecoveryWeakPath(path) {
		score -= 14
		reasons = append(reasons, "weak_path_penalty")
	}
	if findSourceTestBehaviorTestRow(row) {
		score += 2
		reasons = append(reasons, "test_row")
	} else if strings.Contains(strings.ToLower(row.SourceRole), "implementation") || strings.Contains(strings.ToLower(row.SourceRole), "source") {
		score += 3
		reasons = append(reasons, "implementation_row")
	}
	if sameStem := ctx.Stems[findSourceTestStem(candidate.Path)]; sameStem && !findSourceTestBehaviorTestRow(row) {
		score += 14
		reasons = append(reasons, "same_stem_source_recovery")
	}
	return score, uniqueStringList(reasons)
}

func findSourceManifestRecoveryThreshold(row findSourceTestManifestRow, reasons []string) int {
	path := strings.ToLower(normalizeFindGitReceiptPath(row.Path))
	if findSourceManifestRecoveryWeakPath(path) {
		return 28
	}
	for _, reason := range reasons {
		if reason == "same_stem_source_recovery" {
			return 16
		}
	}
	if findSourceTestBehaviorTestRow(row) {
		return 22
	}
	return 18
}

func annotateFindSourceManifestRecoveryCandidate(c retrieval.Candidate, score int, reasons []string) retrieval.Candidate {
	metadata := map[string]string{}
	for key, value := range c.Metadata {
		metadata[key] = value
	}
	metadata["retrieval_expansion_reason"] = "source_manifest_family_recovery"
	metadata["source_manifest_consumption"] = "true"
	metadata["source_manifest_recovery"] = "true"
	metadata["source_manifest_recovery_score"] = strconv.Itoa(score)
	metadata["source_manifest_recovery_reasons"] = strings.Join(firstStrings(reasons, 5), "\n")
	metadata["pack_tier"] = retrieval.PackTierPrimary
	metadata["pack_tier_reason"] = "bounded source-family candidate recovery"
	c.Metadata = metadata
	return c
}

func findSourceManifestRecoveryTerms(query string) []string {
	terms := append([]string(nil), findSourceManifestConsumptionTerms(query)...)
	seen := map[string]bool{}
	var out []string
	add := func(term string) {
		term = strings.ToLower(strings.Trim(term, "_-."))
		if len(term) < 3 || seen[term] {
			return
		}
		seen[term] = true
		out = append(out, term)
	}
	for _, term := range terms {
		add(term)
		for _, alias := range findSourceManifestRecoveryAliases(term) {
			add(alias)
		}
	}
	return out
}

func findSourceManifestRecoveryAliases(term string) []string {
	switch strings.ToLower(strings.TrimSpace(term)) {
	case "tls":
		return []string{"ssl", "cert", "certificate"}
	case "ssl":
		return []string{"tls", "cert", "certificate"}
	case "certificate", "certificates":
		return []string{"cert", "certs", "pem", "ssl", "tls"}
	case "cert", "certs":
		return []string{"certificate", "certificates", "pem", "ssl", "tls"}
	case "passphrase":
		return []string{"password", "passwd"}
	case "oauth2":
		return []string{"oauth"}
	case "oauth":
		return []string{"oauth2"}
	case "cachedir":
		return []string{"cache", "config"}
	case "node_modules":
		return []string{"nodemodules"}
	case "dotfiles":
		return []string{"dotfile", "hidden"}
	case "hidden":
		return []string{"dotfile", "dotfiles"}
	default:
		return nil
	}
}

func findSourceManifestRecoveryWeakPath(path string) bool {
	path = strings.ToLower(normalizeFindGitReceiptPath(path))
	return path == "" ||
		strings.Contains(path, "/docs_src/") ||
		strings.HasPrefix(path, "docs_src/") ||
		strings.Contains(path, "/tutorial") ||
		strings.Contains(path, "/examples/") ||
		strings.HasPrefix(path, "examples/") ||
		strings.Contains(path, "/fixtures/") ||
		strings.Contains(path, "/testdata/") ||
		strings.Contains(path, "/vendor/") ||
		strings.Contains(path, "/node_modules/") ||
		strings.Contains(path, "/generated/") ||
		strings.Contains(path, "/dist/")
}

func uniqueStringList(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
