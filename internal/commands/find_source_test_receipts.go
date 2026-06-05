package commands

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

const (
	findSourceTestReceiptsModeOff       = "off"
	findSourceTestReceiptsModeReceiptV0 = "receipt_v0"

	findSourceTestReceiptsMaxRows      = 4
	findSourceTestReceiptsMaxPerFamily = 2
)

type FindRelatedTestContext struct {
	Mode  string                   `json:"mode"`
	Count int                      `json:"count"`
	Items []FindRelatedTestReceipt `json:"items,omitempty"`
}

type FindRelatedTestReceipt struct {
	Path        string   `json:"path"`
	Score       float64  `json:"score,omitempty"`
	Reasons     []string `json:"reasons,omitempty"`
	SourcePaths []string `json:"source_paths,omitempty"`
	TestNames   []string `json:"test_names,omitempty"`
}

type findSourceTestManifestRow struct {
	FileID         string
	RepoID         string
	Path           string
	Language       string
	SourceRole     string
	SourceRoot     string
	SourceRootKind string
	Symbols        string
	TestNames      string
}

type findSourceTestSourceContext struct {
	Paths        []string
	Stems        map[string]bool
	Families     map[string][]string
	Prefixes     map[string]bool
	QueryTokens  map[string]bool
	SourceTokens map[string]bool
}

type findSourceTestScoredReceipt struct {
	item   FindRelatedTestReceipt
	family string
	score  float64
	strong bool
}

func normalizeFindSourceTestReceiptsMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	mode = strings.ReplaceAll(mode, "-", "_")
	switch mode {
	case "", "none", "false", "0", findSourceTestReceiptsModeOff:
		return findSourceTestReceiptsModeOff
	case "receipt", "receipts", "related", "related_tests", "related_tests_v0", "test_receipts", findSourceTestReceiptsModeReceiptV0:
		return findSourceTestReceiptsModeReceiptV0
	default:
		return ""
	}
}

func validFindSourceTestReceiptsModes() []string {
	return []string{findSourceTestReceiptsModeOff, findSourceTestReceiptsModeReceiptV0}
}

func buildFindSourceTestReceipts(db *store.DB, fp store.FilterParams, query string, pack retrieval.RoleGroupedPack, mode string) (*FindRelatedTestContext, error) {
	mode = normalizeFindSourceTestReceiptsMode(mode)
	if mode == "" || mode == findSourceTestReceiptsModeOff {
		return nil, nil
	}
	ctx := findSourceTestSelectedSourceContext(pack, query)
	if len(ctx.Paths) == 0 {
		return &FindRelatedTestContext{Mode: mode}, nil
	}
	rows, err := loadFindSourceTestManifestRows(db, fp)
	if err != nil {
		return nil, err
	}
	seen := findSourceTestPackPathSet(pack)
	scored := make([]findSourceTestScoredReceipt, 0, len(rows))
	for _, row := range rows {
		path := normalizeFindGitReceiptPath(row.Path)
		if path == "" || seen[path] || !findSourceTestBehaviorTestRow(row) {
			continue
		}
		score, reasons, sources, strong := scoreFindSourceTestReceipt(row, ctx)
		if score < 12 {
			continue
		}
		if !strong {
			score *= 0.35
			reasons = append(reasons, "weak_guardrail")
		}
		if score < 12 {
			continue
		}
		scored = append(scored, findSourceTestScoredReceipt{
			item: FindRelatedTestReceipt{
				Path:        path,
				Score:       roundFindSourceTestScore(score),
				Reasons:     reasons,
				SourcePaths: sources,
				TestNames:   findSourceTestReceiptList(row.TestNames, 4),
			},
			family: findSourceTestPathFamily(path),
			score:  score,
			strong: strong,
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].item.Path < scored[j].item.Path
		}
		return scored[i].score > scored[j].score
	})
	var items []FindRelatedTestReceipt
	familyCounts := map[string]int{}
	for _, candidate := range scored {
		if len(items) >= findSourceTestReceiptsMaxRows {
			break
		}
		if !candidate.strong {
			continue
		}
		if familyCounts[candidate.family] >= findSourceTestReceiptsMaxPerFamily {
			continue
		}
		familyCounts[candidate.family]++
		items = append(items, candidate.item)
	}
	return &FindRelatedTestContext{
		Mode:  mode,
		Count: len(items),
		Items: items,
	}, nil
}

func loadFindSourceTestManifestRows(db *store.DB, fp store.FilterParams) ([]findSourceTestManifestRow, error) {
	query := `SELECT sm.file_id, sm.repo_id, sm.path, sm.language, sm.source_role, sm.source_root, sm.source_root_kind,
			COALESCE(GROUP_CONCAT(DISTINCT sms.symbol), ''),
			COALESCE(GROUP_CONCAT(DISTINCT smt.test_name), '')
		FROM source_manifest sm
		LEFT JOIN source_manifest_symbols sms ON sms.file_id = sm.file_id
		LEFT JOIN source_manifest_tests smt ON smt.file_id = sm.file_id`
	var conditions []string
	var args []any
	if fp.RepoRoot != "" || fp.Branch != "" || fp.User != "" {
		query += " JOIN repos r ON r.id = sm.repo_id"
	}
	conditions = append(conditions, "sm.ignored_reason = ''")
	if fp.RepoRoot != "" {
		conditions = append(conditions, "r.root_path = ?")
		args = append(args, fp.RepoRoot)
	}
	if fp.Branch != "" {
		conditions = append(conditions, "r.git_current_branch = ?")
		args = append(args, fp.Branch)
	}
	if fp.User != "" {
		conditions = append(conditions, "r.scanned_by = ?")
		args = append(args, fp.User)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += ` GROUP BY sm.file_id, sm.repo_id, sm.path, sm.language, sm.source_role, sm.source_root, sm.source_root_kind
		LIMIT 5000`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("load source test receipts: %w", err)
	}
	defer rows.Close()
	var out []findSourceTestManifestRow
	for rows.Next() {
		var row findSourceTestManifestRow
		if err := rows.Scan(&row.FileID, &row.RepoID, &row.Path, &row.Language, &row.SourceRole, &row.SourceRoot, &row.SourceRootKind, &row.Symbols, &row.TestNames); err != nil {
			return nil, fmt.Errorf("scan source test receipt row: %w", err)
		}
		row.Path = filepath.ToSlash(row.Path)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source test receipt rows: %w", err)
	}
	return out, nil
}

func findSourceTestSelectedSourceContext(pack retrieval.RoleGroupedPack, query string) findSourceTestSourceContext {
	ctx := findSourceTestSourceContext{
		Stems:        map[string]bool{},
		Families:     map[string][]string{},
		Prefixes:     map[string]bool{},
		QueryTokens:  findSourceTestTokenSet(findSourceManifestConsumptionTerms(query)),
		SourceTokens: map[string]bool{},
	}
	for _, group := range pack.Groups {
		if group.Role != retrieval.PackRoleImplementation {
			continue
		}
		for _, item := range group.Items {
			path := normalizeFindGitReceiptPath(item.Path)
			if path == "" || findSourceTestLooksBehaviorTestPath(path) {
				continue
			}
			ctx.Paths = append(ctx.Paths, path)
			ctx.Stems[findSourceTestStem(path)] = true
			family := findSourceTestPathFamily(path)
			ctx.Families[family] = appendUniqueString(ctx.Families[family], path)
			for prefix := range findSourceTestPathPrefixes(path) {
				ctx.Prefixes[prefix] = true
			}
			for token := range findSourceTestTokens(path + " " + item.Title) {
				ctx.SourceTokens[token] = true
			}
		}
	}
	sort.Strings(ctx.Paths)
	return ctx
}

func findSourceTestPackPathSet(pack retrieval.RoleGroupedPack) map[string]bool {
	out := map[string]bool{}
	for _, group := range pack.Groups {
		for _, item := range group.Items {
			if path := normalizeFindGitReceiptPath(item.Path); path != "" {
				out[path] = true
			}
		}
	}
	for _, item := range pack.ExcludedNoise {
		if path := normalizeFindGitReceiptPath(item.Path); path != "" {
			out[path] = true
		}
	}
	return out
}

func scoreFindSourceTestReceipt(row findSourceTestManifestRow, ctx findSourceTestSourceContext) (float64, []string, []string, bool) {
	path := normalizeFindGitReceiptPath(row.Path)
	pathTokens := findSourceTestTokens(path)
	stemTokens := findSourceTestTokens(findSourceTestStem(path))
	testTokens := findSourceTestTokens(row.TestNames)
	symbolTokens := findSourceTestTokens(row.Symbols)
	family := findSourceTestPathFamily(path)

	var score float64
	var reasons []string
	var sources []string
	strong := false

	if ctx.Stems[findSourceTestStem(path)] {
		score += 30
		reasons = append(reasons, "same_stem")
		strong = true
	}
	if sourcePaths := ctx.Families[family]; len(sourcePaths) > 0 {
		score += 14
		reasons = append(reasons, "same_family")
		sources = append(sources, sourcePaths...)
	}
	if len(sources) == 0 {
		shared := 0
		for prefix := range findSourceTestPathPrefixes(path) {
			if ctx.Prefixes[prefix] {
				shared++
			}
		}
		if shared > 0 {
			add := float64(shared * 2)
			if add > 8 {
				add = 8
			}
			score += add
			reasons = append(reasons, "shared_path_prefix")
		}
	}

	pathHits := findSourceTestTokenIntersection(ctx.QueryTokens, pathTokens)
	stemHits := findSourceTestTokenIntersection(ctx.QueryTokens, stemTokens)
	testHits := findSourceTestTokenIntersection(ctx.QueryTokens, testTokens)
	symbolHits := findSourceTestTokenIntersection(ctx.QueryTokens, symbolTokens)
	sourceSharedHits := findSourceTestSharedSourceHits(ctx, pathTokens, stemTokens, testTokens, symbolTokens)
	if len(stemHits) > 0 {
		score += float64(len(stemHits)) * 8
		reasons = append(reasons, "stem_anchor:"+strings.Join(stemHits, ","))
	}
	if len(testHits) > 0 {
		score += float64(len(testHits)) * 7
		reasons = append(reasons, "test_name_anchor:"+strings.Join(testHits, ","))
	}
	if len(pathHits) > 0 {
		score += float64(len(pathHits)) * 4
		reasons = append(reasons, "path_anchor:"+strings.Join(pathHits, ","))
	}
	if len(symbolHits) > 0 {
		score += float64(len(symbolHits)) * 2
		reasons = append(reasons, "symbol_anchor:"+strings.Join(symbolHits, ","))
	}
	if len(sourceSharedHits) > 0 {
		score += float64(len(sourceSharedHits)) * 4
		reasons = append(reasons, "selected_source_anchor:"+strings.Join(sourceSharedHits, ","))
	}
	if len(ctx.Families[family]) > 0 && (len(pathHits)+len(stemHits)+len(testHits)+len(sourceSharedHits) > 0) {
		strong = true
	}
	if len(testHits) >= 2 || len(sourceSharedHits) >= 2 {
		strong = true
	}
	if len(sources) == 0 && len(ctx.Paths) > 0 {
		sources = append(sources, firstStrings(ctx.Paths, 2)...)
	}
	return score, reasons, firstStrings(sources, 3), strong
}

func findSourceTestBehaviorTestRow(row findSourceTestManifestRow) bool {
	return findSourceTestLooksBehaviorTestPath(row.Path)
}

func findSourceTestLooksBehaviorTestPath(path string) bool {
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
	return strings.Contains(path, "/tests/") || strings.Contains(path, "/__tests__/")
}

func findSourceTestPathFamily(path string) string {
	path = strings.Trim(normalizeFindGitReceiptPath(path), "/")
	if path == "" {
		return ""
	}
	segs := strings.Split(filepath.ToSlash(filepath.Dir(path)), "/")
	if len(segs) == 0 || segs[0] == "." {
		return "."
	}
	for i, seg := range segs {
		if seg == "components" && len(segs) > i+1 {
			return strings.Join(segs[:i+2], "/")
		}
	}
	if len(segs) >= 5 && segs[0] == "apps" {
		return strings.Join(segs[:5], "/")
	}
	if len(segs) >= 4 {
		switch segs[0] {
		case "api", "server", "web", "web_src":
			return strings.Join(segs[:4], "/")
		}
	}
	if len(segs) >= 3 {
		switch segs[0] {
		case "models", "services", "routers", "modules", "packages", "store":
			return strings.Join(segs[:3], "/")
		}
	}
	limit := len(segs)
	if limit > 3 {
		limit = 3
	}
	return strings.Join(segs[:limit], "/")
}

func findSourceTestPathPrefixes(path string) map[string]bool {
	path = strings.Trim(normalizeFindGitReceiptPath(path), "/")
	segs := strings.Split(filepath.ToSlash(filepath.Dir(path)), "/")
	out := map[string]bool{}
	for i := 1; i <= len(segs) && i <= 5; i++ {
		prefix := strings.Join(segs[:i], "/")
		if prefix != "" && prefix != "." {
			out[prefix] = true
		}
	}
	return out
}

func findSourceTestStem(path string) string {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(path)))
	ext := strings.ToLower(filepath.Ext(base))
	stem := strings.TrimSuffix(base, ext)
	for _, suffix := range []string{"_test", ".test", ".spec", "_spec"} {
		stem = strings.TrimSuffix(stem, suffix)
	}
	stem = strings.TrimPrefix(stem, "test_")
	return stem
}

func findSourceTestTokens(text string) map[string]bool {
	out := map[string]bool{}
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		token := strings.ToLower(b.String())
		b.Reset()
		for _, part := range splitFindSourceTestToken(token) {
			if len(part) >= 2 && !findSourceTestReceiptStopWord(part) {
				out[part] = true
			}
		}
	}
	var previous rune
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if previous != 0 && unicode.IsLower(previous) && unicode.IsUpper(r) {
				flush()
			}
			b.WriteRune(unicode.ToLower(r))
			previous = r
			continue
		}
		flush()
		previous = 0
	}
	flush()
	return out
}

func splitFindSourceTestToken(token string) []string {
	parts := []string{token}
	if strings.HasSuffix(token, "ies") && len(token) > 4 {
		parts = append(parts, strings.TrimSuffix(token, "ies")+"y")
	} else if strings.HasSuffix(token, "es") && len(token) > 4 {
		parts = append(parts, strings.TrimSuffix(token, "es"))
	} else if strings.HasSuffix(token, "s") && len(token) > 3 {
		parts = append(parts, strings.TrimSuffix(token, "s"))
	}
	return parts
}

func findSourceTestTokenSet(tokens []string) map[string]bool {
	out := map[string]bool{}
	for _, token := range tokens {
		for part := range findSourceTestTokens(token) {
			out[part] = true
		}
	}
	return out
}

func findSourceTestTokenIntersection(left, right map[string]bool) []string {
	var out []string
	for token := range left {
		if right[token] {
			out = append(out, token)
		}
	}
	sort.Strings(out)
	return out
}

func findSourceTestSharedSourceHits(ctx findSourceTestSourceContext, sets ...map[string]bool) []string {
	combined := map[string]bool{}
	for _, set := range sets {
		for token := range set {
			combined[token] = true
		}
	}
	var out []string
	for token := range combined {
		if ctx.QueryTokens[token] && ctx.SourceTokens[token] {
			out = append(out, token)
		}
	}
	sort.Strings(out)
	return out
}

func findSourceTestReceiptList(value string, limit int) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = appendUniqueString(out, part)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func findSourceTestReceiptStopWord(term string) bool {
	switch strings.ToLower(strings.TrimSpace(term)) {
	case "a", "an", "and", "are", "as", "be", "before", "by", "can", "change",
		"context", "do", "does", "for", "from", "handle", "if", "in", "into",
		"make", "on", "or", "preserve", "reject", "renaming", "so", "support",
		"the", "to", "update", "updating", "when", "while", "with", "without",
		"code", "field", "fields", "preserving":
		return true
	default:
		return false
	}
}

func roundFindSourceTestScore(score float64) float64 {
	return float64(int(score*1000+0.5)) / 1000
}
