package indexquery

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

type SourceManifestCandidateMode string

const (
	SourceManifestCandidateModeOff      SourceManifestCandidateMode = "off"
	SourceManifestCandidateModeMetadata SourceManifestCandidateMode = "metadata"
	SourceManifestCandidateModeWindow   SourceManifestCandidateMode = "window"
)

type SourceManifestCandidateOptions struct {
	Mode        SourceManifestCandidateMode
	Limit       int
	WindowBytes int
}

type SourceManifestCandidateReport struct {
	Mode           string
	Match          string
	SelectedCount  int
	FallbackReason string
}

func ParseSourceManifestCandidateMode(value string) (SourceManifestCandidateMode, error) {
	switch SourceManifestCandidateMode(strings.ToLower(strings.TrimSpace(value))) {
	case "", SourceManifestCandidateModeOff:
		return SourceManifestCandidateModeOff, nil
	case SourceManifestCandidateModeMetadata:
		return SourceManifestCandidateModeMetadata, nil
	case SourceManifestCandidateModeWindow:
		return SourceManifestCandidateModeWindow, nil
	default:
		return "", fmt.Errorf("unknown source manifest candidate mode %q; valid values: off, metadata, window", value)
	}
}

func DefaultSourceManifestCandidateOptions() SourceManifestCandidateOptions {
	return SourceManifestCandidateOptions{
		Mode:        SourceManifestCandidateModeOff,
		Limit:       120,
		WindowBytes: 1800,
	}
}

func LoadSourceManifestCandidatesForQuery(db *store.DB, fp store.FilterParams, query string, opts SourceManifestCandidateOptions) ([]retrieval.Candidate, SourceManifestCandidateReport, error) {
	if opts.Mode == "" {
		opts.Mode = SourceManifestCandidateModeOff
	}
	if opts.Limit <= 0 {
		opts.Limit = DefaultSourceManifestCandidateOptions().Limit
	}
	if opts.WindowBytes <= 0 {
		opts.WindowBytes = DefaultSourceManifestCandidateOptions().WindowBytes
	}
	report := SourceManifestCandidateReport{Mode: string(opts.Mode)}
	if opts.Mode == SourceManifestCandidateModeOff {
		report.FallbackReason = "off"
		return nil, report, nil
	}
	if !sourceManifestCandidateFiltersCompatible(fp) {
		report.FallbackReason = "unsupported_filters"
		return nil, report, nil
	}
	terms := retrievalRuntimeTerms(query)
	if len(terms) == 0 {
		report.FallbackReason = "no_query_terms"
		return nil, report, nil
	}
	match := retrievalRuntimeFTSQuery(terms)
	report.Match = match
	rows, err := db.SearchSourceManifestFTS(match, fp, opts.Limit)
	if err != nil {
		return nil, report, err
	}
	candidates := make([]retrieval.Candidate, 0, len(rows))
	for _, row := range rows {
		candidate := sourceManifestCandidate(row)
		if fp.Subtype == "test_case" && candidate.Subtype != "test_case" {
			continue
		}
		if opts.Mode == SourceManifestCandidateModeWindow && fp.RepoRoot != "" {
			if snippet := sourceManifestBodyWindow(fp.RepoRoot, row.Path, terms, opts.WindowBytes); snippet != "" {
				candidate.Body = strings.TrimRight(candidate.Body, "\r\n") + "\n\nContent excerpt:\n" + snippet
				candidate.Metadata["source_manifest_body_window"] = "true"
			}
		}
		candidates = append(candidates, candidate)
	}
	report.SelectedCount = len(candidates)
	if len(candidates) == 0 {
		report.FallbackReason = "no_manifest_matches"
	}
	return candidates, report, nil
}

func sourceManifestCandidateFiltersCompatible(fp store.FilterParams) bool {
	if fp.Kind != "" && fp.Kind != "source_context" {
		return false
	}
	if fp.Subtype != "" && fp.Subtype != "test_case" {
		return false
	}
	if fp.Status != "" || fp.Tag != "" {
		return false
	}
	if fp.SourceType != "" && fp.SourceType != "source_context" {
		return false
	}
	return true
}

func sourceManifestCandidate(row store.SourceManifestSearchRow) retrieval.Candidate {
	subtype := ""
	if sourceManifestRowIsTest(row) {
		subtype = "test_case"
	}
	metadata := map[string]string{
		"repo_id":                 row.RepoID,
		"short_id":                row.FileID,
		"token_counter":           TokenCounterName,
		"retrieval_candidate":     "source_manifest",
		"source_context_scope":    "compact_manifest",
		"source_type":             "source_context",
		"source_path":             filepath.ToSlash(row.Path),
		"source_role":             row.SourceRole,
		"source_root":             row.SourceRoot,
		"source_root_kind":        row.SourceRootKind,
		"language":                row.Language,
		"content_hash":            row.ContentHash,
		"size_bytes":              strconv.FormatInt(row.SizeBytes, 10),
		"first_party_score":       fmt.Sprintf("%.3f", row.FirstPartyScore),
		"source_manifest_rank":    fmt.Sprintf("%.4f", row.Rank),
		"source_manifest_indexed": row.IndexedAt,
	}
	if row.Symbols != "" {
		metadata["source_symbols"] = row.Symbols
		metadata["symbols"] = row.Symbols
	}
	if row.TestNames != "" {
		metadata["test_name"] = row.TestNames
	}
	body := renderSourceManifestCandidateBody(row, subtype)
	return retrieval.Candidate{
		ID:       "source_manifest:" + row.FileID,
		Path:     filepath.ToSlash(row.Path),
		Kind:     "source_context",
		Subtype:  subtype,
		Title:    filepath.ToSlash(row.Path),
		Status:   "indexed",
		Body:     body,
		Source:   filepath.ToSlash(row.Path),
		Metadata: metadata,
	}
}

func renderSourceManifestCandidateBody(row store.SourceManifestSearchRow, subtype string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", filepath.ToSlash(row.Path))
	fmt.Fprintln(&b, "Kind: source_context")
	if subtype != "" {
		fmt.Fprintf(&b, "Subtype: %s\n", subtype)
	}
	fmt.Fprintln(&b, "Status: indexed")
	fmt.Fprintf(&b, "Source: %s\n", filepath.ToSlash(row.Path))
	if row.Language != "" {
		fmt.Fprintf(&b, "Language: %s\n", row.Language)
	}
	if row.SourceRole != "" {
		fmt.Fprintf(&b, "Source role: %s\n", row.SourceRole)
	}
	if row.SourceRoot != "" {
		fmt.Fprintf(&b, "Source root: %s\n", filepath.ToSlash(row.SourceRoot))
	}
	writeManifestBodyBlock(&b, "Symbols", row.Symbols)
	writeManifestBodyBlock(&b, "Test names", row.TestNames)
	writeManifestBodyBlock(&b, "Imports", row.Imports)
	return b.String()
}

func writeManifestBodyBlock(b *strings.Builder, title, raw string) {
	values := compactManifestBodyLines(raw, 12)
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s:\n", title)
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
}

func compactManifestBodyLines(raw string, limit int) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func sourceManifestRowIsTest(row store.SourceManifestSearchRow) bool {
	role := strings.ToLower(row.SourceRole)
	if strings.Contains(role, "test") {
		return true
	}
	path := strings.Trim(strings.ToLower(filepath.ToSlash(row.Path)), "/")
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	name := strings.TrimSuffix(base, ext)
	switch {
	case strings.Contains(path, "/tests/") || strings.Contains(path, "/__tests__/"):
		return true
	case strings.HasSuffix(base, "_test.go"):
		return true
	case ext == ".py" && (strings.HasPrefix(base, "test_") || strings.HasSuffix(name, "_test")):
		return true
	case (ext == ".js" || ext == ".jsx" || ext == ".ts" || ext == ".tsx" || ext == ".mjs" || ext == ".cjs") &&
		(strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".spec")):
		return true
	case ext == ".rs" && strings.HasSuffix(name, "_test"):
		return true
	case ext == ".rb" && strings.HasSuffix(base, "_spec.rb"):
		return true
	default:
		return false
	}
}

func sourceManifestBodyWindow(repoRoot, relPath string, terms []string, maxBytes int) string {
	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return ""
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(relPath)))
	if err != nil {
		return ""
	}
	if rel, err := filepath.Rel(rootAbs, targetAbs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	file, err := os.Open(targetAbs)
	if err != nil {
		return ""
	}
	defer file.Close()
	const maxRead = 262144
	buf := make([]byte, maxRead)
	n, _ := file.Read(buf)
	if n <= 0 {
		return ""
	}
	text := string(buf[:n])
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lower := strings.ToLower(text)
	center := 0
	bestScore := -1
	for _, term := range sourceManifestWindowTermsByPriority(terms) {
		term = strings.ToLower(strings.TrimSpace(term))
		if len(term) < 3 {
			continue
		}
		if idx := strings.Index(lower, term); idx >= 0 {
			center = idx
			bestScore = sourceManifestWindowTermScore(term)
			break
		}
	}
	if bestScore < 0 {
		center = 0
	}
	if maxBytes <= 0 {
		maxBytes = 1800
	}
	start := center - maxBytes/3
	if start < 0 {
		start = 0
	}
	end := start + maxBytes
	if end > len(text) {
		end = len(text)
	}
	if start > 0 {
		if newline := strings.Index(text[start:end], "\n"); newline >= 0 && newline+start < end {
			start += newline + 1
		}
	}
	if end < len(text) {
		if newline := strings.LastIndex(text[start:end], "\n"); newline > 0 {
			end = start + newline
		}
	}
	snippet := strings.TrimSpace(text[start:end])
	if snippet == "" {
		return ""
	}
	if start > 0 {
		snippet = "...\n" + snippet
	}
	if end < len(text) {
		snippet += "\n..."
	}
	return snippet
}

func sourceManifestWindowTermsByPriority(terms []string) []string {
	out := append([]string(nil), terms...)
	sort.SliceStable(out, func(i, j int) bool {
		left := sourceManifestWindowTermScore(out[i])
		right := sourceManifestWindowTermScore(out[j])
		if left == right {
			return len(out[i]) > len(out[j])
		}
		return left > right
	})
	return out
}

func sourceManifestWindowTermScore(term string) int {
	term = strings.ToLower(strings.TrimSpace(term))
	score := len(term)
	if strings.ContainsAny(term, "_.-0123456789") {
		score += 40
	}
	if len(term) >= 8 {
		score += 12
	}
	if strings.Contains(term, "oauth") || strings.Contains(term, "swagger") || strings.Contains(term, "passphrase") || strings.Contains(term, "print0") {
		score += 12
	}
	switch term {
	case "docs", "custom", "default", "client", "headers", "context", "source", "files":
		score -= 18
	case "make", "serve", "allow", "users", "using", "when":
		score -= 24
	}
	return score
}
