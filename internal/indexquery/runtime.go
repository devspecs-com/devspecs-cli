package indexquery

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

type RuntimeMode string

const (
	RuntimeModeFull            RuntimeMode = "full"
	RuntimeModePreselectShadow RuntimeMode = "preselect_shadow"
	RuntimeModePreselectActive RuntimeMode = "preselect_active"
)

type PreselectOptions struct {
	PreselectLimit              int
	MaxRepoSizeForFullHydration int
	FallbackFullHydrationBelow  int
}

type PreselectReport struct {
	FullArtifactCount int
	SelectedCount     int
	LaneCounts        map[string]int
	FallbackReason    string
}

type CandidateLoadReport struct {
	RuntimeMode       string
	EffectiveMode     string
	FullArtifactCount int
	PreselectedCount  int
	HydratedCount     int
	FallbackReason    string
	LaneCounts        map[string]int
	PreselectMS       int64
	HydrateMS         int64
	FullLoadMS        int64
	OptimizedError    string
}

type CandidateLoadResult struct {
	Candidates []retrieval.Candidate
	Report     CandidateLoadReport
}

func ParseRuntimeMode(value string) (RuntimeMode, error) {
	switch RuntimeMode(strings.ToLower(strings.TrimSpace(value))) {
	case "", RuntimeModeFull:
		return RuntimeModeFull, nil
	case RuntimeModePreselectShadow:
		return RuntimeModePreselectShadow, nil
	case RuntimeModePreselectActive:
		return RuntimeModePreselectActive, nil
	default:
		return "", fmt.Errorf("unknown find runtime %q; valid values: full, preselect_shadow, preselect_active", value)
	}
}

func DefaultPreselectOptions() PreselectOptions {
	return PreselectOptions{
		PreselectLimit:              1500,
		MaxRepoSizeForFullHydration: 500,
		FallbackFullHydrationBelow:  1,
	}
}

func LoadCandidatesForQueryWithRuntime(db *store.DB, fp store.FilterParams, query string, mode RuntimeMode) (CandidateLoadResult, error) {
	if mode == "" {
		mode = RuntimeModeFull
	}
	switch mode {
	case RuntimeModeFull:
		return loadCandidatesForQueryFull(db, fp, query, mode)
	case RuntimeModePreselectActive:
		return LoadCandidatesForQueryOptimized(db, fp, query, mode)
	case RuntimeModePreselectShadow:
		shadow, shadowErr := LoadCandidatesForQueryOptimized(db, fp, query, mode)
		if shadowErr == nil && shadow.Report.EffectiveMode == string(RuntimeModeFull) {
			shadow.Report.RuntimeMode = string(mode)
			return shadow, nil
		}
		full, err := loadCandidatesForQueryFull(db, fp, query, mode)
		if shadowErr != nil {
			full.Report.OptimizedError = shadowErr.Error()
		}
		if len(shadow.Report.LaneCounts) > 0 || shadow.Report.PreselectedCount > 0 || shadow.Report.FallbackReason != "" {
			full.Report.FullArtifactCount = shadow.Report.FullArtifactCount
			full.Report.PreselectedCount = shadow.Report.PreselectedCount
			full.Report.HydratedCount = shadow.Report.HydratedCount
			full.Report.FallbackReason = shadow.Report.FallbackReason
			full.Report.LaneCounts = shadow.Report.LaneCounts
			full.Report.PreselectMS = shadow.Report.PreselectMS
			full.Report.HydrateMS = shadow.Report.HydrateMS
		}
		full.Report.RuntimeMode = string(mode)
		full.Report.EffectiveMode = string(RuntimeModeFull)
		return full, err
	default:
		return CandidateLoadResult{}, fmt.Errorf("unknown find runtime %q", mode)
	}
}

func loadCandidatesForQueryFull(db *store.DB, fp store.FilterParams, query string, mode RuntimeMode) (CandidateLoadResult, error) {
	start := time.Now()
	candidates, err := LoadCandidatesForQuery(db, fp, query)
	report := CandidateLoadReport{
		RuntimeMode:       string(mode),
		EffectiveMode:     string(RuntimeModeFull),
		HydratedCount:     len(candidates),
		FullArtifactCount: len(candidates),
		FullLoadMS:        time.Since(start).Milliseconds(),
	}
	if err != nil {
		return CandidateLoadResult{Report: report}, err
	}
	return CandidateLoadResult{Candidates: candidates, Report: report}, nil
}

func LoadCandidatesForQueryOptimized(db *store.DB, fp store.FilterParams, query string, mode RuntimeMode) (CandidateLoadResult, error) {
	if mode == "" {
		mode = RuntimeModePreselectActive
	}
	preselectStart := time.Now()
	ids, preselectReport, err := PreselectArtifactIDsForQuery(db, fp, query, DefaultPreselectOptions())
	report := CandidateLoadReport{
		RuntimeMode:       string(mode),
		EffectiveMode:     string(RuntimeModePreselectActive),
		FullArtifactCount: preselectReport.FullArtifactCount,
		PreselectedCount:  preselectReport.SelectedCount,
		FallbackReason:    preselectReport.FallbackReason,
		LaneCounts:        preselectReport.LaneCounts,
		PreselectMS:       time.Since(preselectStart).Milliseconds(),
	}
	if err != nil {
		return CandidateLoadResult{Report: report}, fmt.Errorf("preselect: %w", err)
	}
	if preselectReport.FallbackReason != "" {
		full, err := loadCandidatesForQueryFull(db, fp, query, mode)
		full.Report.FullArtifactCount = preselectReport.FullArtifactCount
		full.Report.PreselectedCount = preselectReport.SelectedCount
		full.Report.FallbackReason = preselectReport.FallbackReason
		full.Report.LaneCounts = preselectReport.LaneCounts
		full.Report.PreselectMS = report.PreselectMS
		full.Report.RuntimeMode = string(mode)
		full.Report.EffectiveMode = string(RuntimeModeFull)
		return full, err
	}

	hydrateStart := time.Now()
	candidates, err := LoadCandidatesByArtifactIDs(db, fp, ids)
	report.HydrateMS = time.Since(hydrateStart).Milliseconds()
	report.HydratedCount = len(candidates)
	if err != nil {
		return CandidateLoadResult{Report: report}, fmt.Errorf("hydrate selected candidates: %w", err)
	}
	return CandidateLoadResult{
		Candidates: retrieval.EnrichCandidatesWithSectionMatches(candidates, query),
		Report:     report,
	}, nil
}

func PreselectArtifactIDsForQuery(db *store.DB, fp store.FilterParams, query string, opts PreselectOptions) ([]string, PreselectReport, error) {
	if opts.PreselectLimit <= 0 {
		opts.PreselectLimit = DefaultPreselectOptions().PreselectLimit
	}
	report := PreselectReport{LaneCounts: map[string]int{}}
	fullCount, err := db.CountArtifacts(fp)
	if err != nil {
		return nil, report, err
	}
	report.FullArtifactCount = fullCount
	if fullCount == 0 {
		report.FallbackReason = "empty_corpus"
		return nil, report, nil
	}
	if opts.MaxRepoSizeForFullHydration > 0 && fullCount <= opts.MaxRepoSizeForFullHydration {
		report.FallbackReason = "small_corpus"
		return nil, report, nil
	}

	terms := retrievalRuntimeTerms(query)
	if len(terms) == 0 {
		report.FallbackReason = "no_query_terms"
		return nil, report, nil
	}
	match := retrievalRuntimeFTSQuery(terms)
	seen := map[string]bool{}
	var selected []string
	addLane := func(name string, ids []string) {
		report.LaneCounts[name] = len(ids)
		for _, id := range ids {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			if len(selected) < opts.PreselectLimit {
				selected = append(selected, id)
			}
		}
	}

	artifactIDs, err := db.FindArtifactIDsFTS(match, fp, opts.PreselectLimit)
	if err != nil {
		return nil, report, fmt.Errorf("artifact fts lane: %w", err)
	}
	addLane("artifact_fts", artifactIDs)
	sectionIDs, err := db.FindArtifactIDsBySectionFTS(match, fp, opts.PreselectLimit)
	if err != nil {
		return nil, report, fmt.Errorf("section fts lane: %w", err)
	}
	addLane("section_fts", sectionIDs)
	likeIDs, err := db.FindArtifactIDsByTitleOrPathTerms(terms, fp, opts.PreselectLimit)
	if err != nil {
		return nil, report, fmt.Errorf("title_path_like lane: %w", err)
	}
	addLane("title_path_like", likeIDs)

	report.SelectedCount = len(selected)
	if len(selected) < opts.FallbackFullHydrationBelow {
		report.FallbackReason = "candidate_pool_too_small"
		return nil, report, nil
	}
	return selected, report, nil
}

func retrievalRuntimeFTSQuery(terms []string) string {
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " OR ")
}

func retrievalRuntimeTerms(query string) []string {
	seen := map[string]bool{}
	var terms []string
	add := func(term string) {
		term = strings.ToLower(strings.Trim(term, "_-."))
		if len(term) < 3 || seen[term] || retrievalRuntimeStopWord(term) {
			return
		}
		seen[term] = true
		terms = append(terms, term)
	}
	flushTerm := func(term string) {
		if term == "" {
			return
		}
		add(term)
		for _, part := range strings.FieldsFunc(term, func(r rune) bool {
			return r == '_' || r == '-' || r == '.'
		}) {
			add(part)
		}
	}

	var b strings.Builder
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flushTerm(b.String())
		b.Reset()
	}
	flushTerm(b.String())
	if len(terms) > 12 {
		return terms[:12]
	}
	return terms
}

func retrievalRuntimeStopWord(term string) bool {
	switch term {
	case "the", "and", "for", "with", "that", "this", "from", "into", "what", "where", "when", "which", "cover", "covers", "context", "artifact", "document", "documents", "behavior", "behaviour":
		return true
	default:
		return false
	}
}
