package commands

import (
	"sort"
	"strconv"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/adapters/sourcecontext"
	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

const (
	findSourceManifestRelationshipMaxAdditions  = 4
	findSourceManifestSelectedTestCoverageLimit = 5
)

type findSourceManifestRelationshipEdge struct {
	from string
	to   string
}

type findSourceManifestRelationshipCandidate struct {
	candidate retrieval.Candidate
	score     int
	reasons   []string
}

func selectFindSourceManifestRelationshipCandidates(query string, selected, all []retrieval.Candidate, rows []findSourceTestManifestRow, limit int) []retrieval.Candidate {
	if limit <= 0 || len(selected) == 0 || len(rows) == 0 {
		return nil
	}
	terms := findSourceManifestRecoveryTerms(query)
	if len(terms) == 0 {
		return nil
	}

	paths := make([]string, 0, len(rows))
	rowsByPath := make(map[string]findSourceTestManifestRow, len(rows))
	for _, row := range rows {
		path := normalizeFindGitReceiptPath(row.Path)
		if path == "" {
			continue
		}
		paths = append(paths, path)
		rowsByPath[strings.ToLower(path)] = row
	}
	resolver := sourcecontext.NewShellImportResolver(paths)
	edges := findSourceManifestRelationshipEdges(rows, resolver)
	selectedPaths := map[string]bool{}
	selectedByPath := map[string]retrieval.Candidate{}
	for _, candidate := range selected {
		path := strings.ToLower(normalizeFindGitReceiptPath(candidate.Path))
		if path == "" {
			continue
		}
		selectedPaths[path] = true
		selectedByPath[path] = candidate
	}
	byPath := findPackCandidatePathIndex(all)
	scored := map[string]*findSourceManifestRelationshipCandidate{}
	add := func(path string, score int, reason string, promoteSelected bool) {
		key := strings.ToLower(normalizeFindGitReceiptPath(path))
		row, ok := rowsByPath[key]
		if !ok || (!promoteSelected && selectedPaths[key]) || findSourceManifestRecoveryWeakPath(key) {
			return
		}
		entry := scored[key]
		if entry == nil {
			candidate, selectedCandidate := selectedByPath[key]
			if !selectedCandidate {
				candidate = findSourceManifestRecoveryCandidateFromRow(row, byPath)
			}
			entry = &findSourceManifestRelationshipCandidate{candidate: candidate}
			scored[key] = entry
		}
		entry.score += score
		entry.reasons = appendUniqueString(entry.reasons, reason)
	}

	selectedImportTargets := map[string]bool{}
	for _, edge := range edges {
		switch {
		case selectedPaths[edge.from]:
			add(edge.to, 38, "selected_file_import_target", false)
			selectedImportTargets[edge.to] = true
		case selectedPaths[edge.to]:
			add(edge.from, 34, "selected_file_importer", false)
		}
	}
	for _, edge := range edges {
		if selectedImportTargets[edge.to] && !selectedPaths[edge.from] {
			add(edge.from, 18, "shared_import_target", false)
		}
	}

	selectedTestTokens := findSourceManifestSelectedTestTokens(selected)
	coveredTestPaths := findSourceManifestSelectedTestPaths(selected)
	queryTokens := findSourceTestTokenSet(terms)
	for _, row := range rows {
		if !findSourceTestBehaviorTestRow(row) {
			continue
		}
		path := strings.ToLower(normalizeFindGitReceiptPath(row.Path))
		if path == "" || coveredTestPaths[path] || findSourceManifestRecoveryWeakPath(path) {
			continue
		}
		pathHits := findSourceManifestUncoveredTokenHits(queryTokens, selectedTestTokens, findSourceTestTokens(row.Path))
		nameHits := findSourceManifestUncoveredTokenHits(queryTokens, selectedTestTokens, findSourceTestTokens(row.TestNames))
		hits := append([]string(nil), pathHits...)
		for _, hit := range nameHits {
			hits = appendUniqueString(hits, hit)
		}
		sort.Strings(hits)
		if len(hits) == 0 {
			continue
		}
		add(path, 24+len(pathHits)*16+len(nameHits)*8, "uncovered_behavior_anchor:"+strings.Join(hits, ","), true)
	}

	values := make([]findSourceManifestRelationshipCandidate, 0, len(scored))
	for _, entry := range scored {
		entry.score += findSourceManifestConsumptionScore(entry.candidate, terms) / 3
		entry.candidate = annotateFindSourceManifestRelationshipCandidate(entry.candidate, entry.score, entry.reasons)
		values = append(values, *entry)
	}
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].score != values[j].score {
			return values[i].score > values[j].score
		}
		return values[i].candidate.Path < values[j].candidate.Path
	})
	if len(values) > limit {
		values = values[:limit]
	}
	out := make([]retrieval.Candidate, 0, len(values))
	for _, value := range values {
		out = append(out, value.candidate)
	}
	return out
}

func findSourceManifestRelationshipEdges(rows []findSourceTestManifestRow, resolver sourcecontext.ShellImportResolver) []findSourceManifestRelationshipEdge {
	seen := map[string]bool{}
	var edges []findSourceManifestRelationshipEdge
	for _, row := range rows {
		from := strings.ToLower(normalizeFindGitReceiptPath(row.Path))
		if from == "" {
			continue
		}
		for _, importRef := range findSourceTestReceiptList(row.Imports, 0) {
			to := strings.ToLower(normalizeFindGitReceiptPath(resolver.Resolve(row.Path, importRef)))
			if to == "" || to == from {
				continue
			}
			key := from + "\x00" + to
			if seen[key] {
				continue
			}
			seen[key] = true
			edges = append(edges, findSourceManifestRelationshipEdge{from: from, to: to})
		}
	}
	return edges
}

func findSourceManifestSelectedTestTokens(selected []retrieval.Candidate) map[string]bool {
	out := map[string]bool{}
	coveredTests := 0
	for _, candidate := range selected {
		if !findSourceManifestTestCandidate(candidate) && !findSourceTestLooksBehaviorTestPath(candidate.Path) {
			continue
		}
		if coveredTests >= findSourceManifestSelectedTestCoverageLimit {
			break
		}
		coveredTests++
		for token := range findSourceTestTokens(candidate.Path + " " + candidate.Title) {
			out[token] = true
		}
	}
	return out
}

func findSourceManifestSelectedTestPaths(selected []retrieval.Candidate) map[string]bool {
	out := map[string]bool{}
	coveredTests := 0
	for _, candidate := range selected {
		if !findSourceManifestTestCandidate(candidate) && !findSourceTestLooksBehaviorTestPath(candidate.Path) {
			continue
		}
		if coveredTests >= findSourceManifestSelectedTestCoverageLimit {
			break
		}
		path := strings.ToLower(normalizeFindGitReceiptPath(candidate.Path))
		if path != "" {
			out[path] = true
		}
		coveredTests++
	}
	return out
}

func findSourceManifestUncoveredTokenHits(queryTokens, selectedTokens, candidateTokens map[string]bool) []string {
	var hits []string
	for token := range queryTokens {
		if !selectedTokens[token] && candidateTokens[token] {
			hits = append(hits, token)
		}
	}
	sort.Strings(hits)
	return hits
}

func annotateFindSourceManifestRelationshipCandidate(candidate retrieval.Candidate, score int, reasons []string) retrieval.Candidate {
	metadata := map[string]string{}
	for key, value := range candidate.Metadata {
		metadata[key] = value
	}
	metadata["retrieval_expansion_reason"] = "source_manifest_relationship_recovery"
	metadata["source_manifest_relationship_recovery"] = "true"
	metadata["source_manifest_relationship_score"] = strconv.Itoa(score)
	metadata["source_manifest_relationship_reasons"] = strings.Join(firstStrings(reasons, 4), "\n")
	metadata["pack_tier"] = retrieval.PackTierPrimary
	metadata["pack_tier_reason"] = "bounded source-manifest relationship recovery"
	candidate.Metadata = metadata
	return candidate
}
