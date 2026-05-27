package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/gitfacts"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

const (
	edgeTypeCoChangedWith   = "co_changed_with"
	edgeTypeRecentlyChanged = "recently_changed"

	sourceSignalGitCoChange     = "git_co_change"
	sourceSignalGitRecentCommit = "git_recent_commit"

	defaultGitMaxCommits        = gitfacts.DefaultMaxCommits
	defaultGitMaxFilesPerCommit = gitfacts.DefaultMaxFilesPerCommit
	maxGitCoChangeEdges         = 1500
	maxGitRecentlyChangedEdges  = 80
	maxGitCommitExamples        = 3
	maxGitEdgeExamples          = 8

	maxGitRepresentativesPerPath      = 3
	maxGitPathPairsPerCommit          = 12
	maxGitPathPairsToMaterialize      = 250
	maxGitCoChangeEdgesPerArtifact    = 20
	maxGitCoChangeEdgesPerSourcePair  = 4
	gitHighDensitySourcePathThreshold = 12
	maxGitEvidenceExampleValueLength  = 120
)

// GitEvidenceDiagnostics is emitted by ds scan --json when git evidence is enabled.
type GitEvidenceDiagnostics struct {
	Enabled                    bool                     `json:"enabled"`
	HistoryShape               string                   `json:"history_shape"`
	IsShallow                  bool                     `json:"is_shallow,omitempty"`
	Branch                     string                   `json:"branch,omitempty"`
	MaxCommits                 int                      `json:"max_commits"`
	MaxFilesPerCommit          int                      `json:"max_files_per_commit"`
	TotalCommits               int                      `json:"total_commits,omitempty"`
	CommitsRead                int                      `json:"commits_read,omitempty"`
	CommitsStored              int                      `json:"commits_stored,omitempty"`
	FilesStored                int                      `json:"files_stored,omitempty"`
	BodiesTruncated            int                      `json:"commit_bodies_truncated,omitempty"`
	EdgesIndexed               int                      `json:"edges_indexed,omitempty"`
	EdgesByType                map[string]int           `json:"edges_by_type,omitempty"`
	SkippedLargeCommits        int                      `json:"skipped_large_commits,omitempty"`
	SkippedMergeCommits        int                      `json:"skipped_merge_commits,omitempty"`
	SkippedLockfileOnlyCommits int                      `json:"skipped_lockfile_only_commits,omitempty"`
	SkippedNoMappedArtifacts   int                      `json:"skipped_no_mapped_artifacts,omitempty"`
	PathPairsEvaluated         int                      `json:"path_pairs_evaluated,omitempty"`
	PathPairsMaterialized      int                      `json:"path_pairs_materialized,omitempty"`
	CappedSourcePaths          int                      `json:"capped_source_paths,omitempty"`
	HighDensitySourcePaths     int                      `json:"high_density_source_paths,omitempty"`
	CappedCommitPathPairs      int                      `json:"capped_commit_path_pairs,omitempty"`
	CappedPathPairs            int                      `json:"capped_path_pairs,omitempty"`
	CappedArtifactEdges        int                      `json:"capped_artifact_edges,omitempty"`
	GitError                   string                   `json:"git_error,omitempty"`
	TopEdges                   []GitEvidenceEdgeExample `json:"top_edges,omitempty"`
}

// GitEvidenceEdgeExample is a compact receipt for manual edge audit.
type GitEvidenceEdgeExample struct {
	EdgeType          string   `json:"edge_type"`
	Source            string   `json:"source"`
	Target            string   `json:"target"`
	SourceTitle       string   `json:"source_title,omitempty"`
	TargetTitle       string   `json:"target_title,omitempty"`
	SourceKind        string   `json:"source_kind,omitempty"`
	TargetKind        string   `json:"target_kind,omitempty"`
	SourceSubtype     string   `json:"source_subtype,omitempty"`
	TargetSubtype     string   `json:"target_subtype,omitempty"`
	SourcePath        string   `json:"source_path,omitempty"`
	TargetPath        string   `json:"target_path,omitempty"`
	SourceSignal      string   `json:"source_signal"`
	Explanation       string   `json:"explanation"`
	Confidence        float64  `json:"confidence"`
	EvidenceCount     int      `json:"evidence_count"`
	Commits           []string `json:"commits,omitempty"`
	FileExamples      []string `json:"file_examples,omitempty"`
	HistoryShape      string   `json:"history_shape,omitempty"`
	ConfidenceRule    string   `json:"confidence_rule,omitempty"`
	SourcePathDensity int      `json:"source_path_density,omitempty"`
	TargetPathDensity int      `json:"target_path_density,omitempty"`
}

type gitCommitEvidence struct {
	commit gitfacts.Commit
	files  []gitfacts.FileChange
}

type gitPathPairAccumulator struct {
	leftPath     string
	rightPath    string
	evidence     int
	commits      []string
	latest       string
	leftDensity  int
	rightDensity int
	highDensity  bool
}

type gitRecentAccumulator struct {
	artifactID string
	evidence   int
	commits    []string
	latest     string
	path       string
}

type gitArtifactRef struct {
	id             string
	kind           string
	subtype        string
	title          string
	path           string
	sourceIdentity string
	density        int
	highDensity    bool
}

func (s *Scanner) rebuildGitEvidence(ctx context.Context, repoRoot, repoID, now string, opts RunOptions) (*GitEvidenceDiagnostics, *WorkstreamEvidenceDiagnostics, error) {
	maxCommits := opts.GitMaxCommits
	if maxCommits <= 0 {
		maxCommits = defaultGitMaxCommits
	}
	maxFiles := opts.GitMaxFilesPerCommit
	if maxFiles <= 0 {
		maxFiles = defaultGitMaxFilesPerCommit
	}
	facts, err := gitfacts.Collect(ctx, repoRoot, gitfacts.Options{
		MaxCommits:        maxCommits,
		MaxFilesPerCommit: maxFiles,
	})
	if err != nil {
		return nil, nil, err
	}
	diag := gitEvidenceDiagnosticsFromFacts(facts.Diagnostics)
	commits := make([]store.GitCommitInput, 0, len(facts.Commits))
	for _, c := range facts.Commits {
		commits = append(commits, store.GitCommitInput{
			RepoID:       repoID,
			SHA:          c.SHA,
			Branch:       c.Branch,
			AuthorName:   c.AuthorName,
			AuthorEmail:  c.AuthorEmail,
			Message:      c.Message,
			BodyPreview:  c.BodyPreview,
			CommittedAt:  c.CommittedAt,
			FilesChanged: c.FilesChanged,
			IsMerge:      c.IsMerge,
			HistoryShape: c.HistoryShape,
		})
	}
	files := make([]store.GitCommitFileInput, 0, len(facts.Files))
	for _, f := range facts.Files {
		files = append(files, store.GitCommitFileInput{
			RepoID:     repoID,
			CommitSHA:  f.CommitSHA,
			FilePath:   f.FilePath,
			ChangeType: f.ChangeType,
			OldPath:    f.OldPath,
		})
	}
	if err := s.db.ReplaceRepoGitFacts(repoID, commits, files, now); err != nil {
		return nil, nil, err
	}
	edges, edgeDiag, err := s.materializeGitEdges(repoID, facts)
	if err != nil {
		return nil, nil, err
	}
	for _, edge := range edges {
		if err := s.db.UpsertArtifactEdge(edge, now); err != nil {
			return nil, nil, err
		}
	}
	diag.EdgesIndexed = len(edges)
	diag.EdgesByType = edgeDiag.edgesByType
	diag.SkippedMergeCommits = edgeDiag.skippedMerge
	diag.SkippedLockfileOnlyCommits = edgeDiag.skippedLockfileOnly
	diag.SkippedNoMappedArtifacts = edgeDiag.skippedNoMappedArtifacts
	diag.PathPairsEvaluated = edgeDiag.pathPairsEvaluated
	diag.PathPairsMaterialized = edgeDiag.pathPairsMaterialized
	diag.CappedSourcePaths = edgeDiag.cappedSourcePaths
	diag.HighDensitySourcePaths = edgeDiag.highDensitySourcePaths
	diag.CappedCommitPathPairs = edgeDiag.cappedCommitPathPairs
	diag.CappedPathPairs = edgeDiag.cappedPathPairs
	diag.CappedArtifactEdges = edgeDiag.cappedArtifactEdges
	diag.TopEdges = edgeDiag.topEdges
	var workstreamDiag *WorkstreamEvidenceDiagnostics
	if opts.IncludeWorkstreamEvidence {
		var err error
		workstreamDiag, err = s.rebuildWorkstreamEvidence(repoID, now, facts)
		if err != nil {
			return nil, nil, err
		}
	}
	return diag, workstreamDiag, nil
}

func gitEvidenceDiagnosticsFromFacts(d gitfacts.Diagnostics) *GitEvidenceDiagnostics {
	return &GitEvidenceDiagnostics{
		Enabled:             d.Enabled,
		HistoryShape:        d.HistoryShape,
		IsShallow:           d.IsShallow,
		Branch:              d.Branch,
		MaxCommits:          d.MaxCommits,
		MaxFilesPerCommit:   d.MaxFilesPerCommit,
		TotalCommits:        d.TotalCommits,
		CommitsRead:         d.CommitsRead,
		CommitsStored:       d.CommitsStored,
		FilesStored:         d.FilesStored,
		BodiesTruncated:     d.BodiesTruncated,
		SkippedLargeCommits: d.SkippedLargeCommits,
		GitError:            d.GitError,
		EdgesByType:         map[string]int{},
	}
}

type gitEdgeDiagnostics struct {
	edgesByType              map[string]int
	skippedMerge             int
	skippedLockfileOnly      int
	skippedNoMappedArtifacts int
	pathPairsEvaluated       int
	pathPairsMaterialized    int
	cappedSourcePaths        int
	highDensitySourcePaths   int
	cappedCommitPathPairs    int
	cappedPathPairs          int
	cappedArtifactEdges      int
	topEdges                 []GitEvidenceEdgeExample
}

func (s *Scanner) materializeGitEdges(repoID string, facts gitfacts.Facts) ([]store.ArtifactEdgeInput, gitEdgeDiagnostics, error) {
	diag := gitEdgeDiagnostics{edgesByType: map[string]int{}}
	sourceRows, err := s.db.GetArtifactSourcePaths(repoID)
	if err != nil {
		return nil, diag, err
	}
	artifactsByPath, artifactsByID := gitArtifactMaps(sourceRows)
	if len(artifactsByPath) == 0 {
		return nil, diag, nil
	}
	for _, refs := range artifactsByPath {
		if len(refs) > maxGitRepresentativesPerPath {
			diag.cappedSourcePaths++
		}
		if len(refs) > gitHighDensitySourcePathThreshold {
			diag.highDensitySourcePaths++
		}
	}
	commitsBySHA := map[string]gitCommitEvidence{}
	for _, commit := range facts.Commits {
		commitsBySHA[commit.SHA] = gitCommitEvidence{commit: commit}
	}
	for _, file := range facts.Files {
		entry := commitsBySHA[file.CommitSHA]
		entry.files = append(entry.files, file)
		commitsBySHA[file.CommitSHA] = entry
	}
	pathPairs := map[string]*gitPathPairAccumulator{}
	recents := map[string]*gitRecentAccumulator{}
	commitOrder := append([]gitfacts.Commit(nil), facts.Commits...)
	sort.SliceStable(commitOrder, func(i, j int) bool {
		return commitOrder[i].CommittedAt > commitOrder[j].CommittedAt
	})
	for _, commit := range commitOrder {
		entry := commitsBySHA[commit.SHA]
		if commit.IsMerge {
			diag.skippedMerge++
			continue
		}
		if len(entry.files) == 0 {
			continue
		}
		if lockfileOnlyCommit(entry.files) {
			diag.skippedLockfileOnly++
			continue
		}
		mappedPaths := map[string]bool{}
		for _, file := range entry.files {
			path := normalizeGitEvidencePath(file.FilePath)
			if path == "" || gitEvidenceNoisyPath(path) {
				continue
			}
			refs := artifactsByPath[path]
			if len(refs) == 0 {
				continue
			}
			mappedPaths[path] = true
			for _, ref := range limitGitArtifactRefs(refs) {
				acc := recents[ref.id]
				if acc == nil {
					acc = &gitRecentAccumulator{artifactID: ref.id, path: path}
					recents[ref.id] = acc
				}
				acc.evidence++
				acc.latest = maxString(acc.latest, commit.CommittedAt)
				acc.commits = appendLimitedUnique(acc.commits, shortSHA(commit.SHA), maxGitCommitExamples)
			}
		}
		if len(mappedPaths) == 0 {
			diag.skippedNoMappedArtifacts++
			continue
		}
		paths := sortedBoolMapKeys(mappedPaths)
		if len(paths) < 2 {
			continue
		}
		totalPairs := len(paths) * (len(paths) - 1) / 2
		if totalPairs > maxGitPathPairsPerCommit {
			diag.cappedCommitPathPairs += totalPairs - maxGitPathPairsPerCommit
		}
		emittedForCommit := 0
		for i := 0; i < len(paths); i++ {
			for j := i + 1; j < len(paths); j++ {
				if emittedForCommit >= maxGitPathPairsPerCommit {
					break
				}
				leftPath, rightPath := paths[i], paths[j]
				key := leftPath + "\x00" + rightPath
				acc := pathPairs[key]
				if acc == nil {
					leftDensity := len(artifactsByPath[leftPath])
					rightDensity := len(artifactsByPath[rightPath])
					acc = &gitPathPairAccumulator{
						leftPath:     leftPath,
						rightPath:    rightPath,
						leftDensity:  leftDensity,
						rightDensity: rightDensity,
						highDensity:  leftDensity > gitHighDensitySourcePathThreshold || rightDensity > gitHighDensitySourcePathThreshold,
					}
					pathPairs[key] = acc
				}
				acc.evidence++
				acc.latest = maxString(acc.latest, commit.CommittedAt)
				acc.commits = appendLimitedUnique(acc.commits, shortSHA(commit.SHA), maxGitCommitExamples)
				emittedForCommit++
			}
		}
	}
	diag.pathPairsEvaluated = len(pathPairs)
	coChangeEdges := buildGitCoChangeEdges(repoID, facts.Diagnostics.HistoryShape, pathPairs, artifactsByPath, &diag)
	recentEdges := buildGitRecentlyChangedEdges(repoID, facts.Diagnostics.HistoryShape, recents)
	edges := append(coChangeEdges, recentEdges...)
	sortGitEdges(edges)
	if len(edges) > maxGitCoChangeEdges+maxGitRecentlyChangedEdges {
		diag.cappedArtifactEdges += len(edges) - (maxGitCoChangeEdges + maxGitRecentlyChangedEdges)
		edges = edges[:maxGitCoChangeEdges+maxGitRecentlyChangedEdges]
	}
	for _, edge := range edges {
		diag.edgesByType[edge.EdgeType]++
	}
	diag.topEdges = topGitEdgeExamples(edges, artifactsByID)
	return edges, diag, nil
}

func buildGitCoChangeEdges(repoID, shape string, pairs map[string]*gitPathPairAccumulator, artifactsByPath map[string][]gitArtifactRef, diag *gitEdgeDiagnostics) []store.ArtifactEdgeInput {
	values := make([]*gitPathPairAccumulator, 0, len(pairs))
	for _, acc := range pairs {
		values = append(values, acc)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].evidence == values[j].evidence {
			if values[i].latest == values[j].latest {
				if values[i].leftPath == values[j].leftPath {
					return values[i].rightPath < values[j].rightPath
				}
				return values[i].leftPath < values[j].leftPath
			}
			return values[i].latest > values[j].latest
		}
		return values[i].evidence > values[j].evidence
	})
	if len(values) > maxGitPathPairsToMaterialize {
		diag.cappedPathPairs += len(values) - maxGitPathPairsToMaterialize
		values = values[:maxGitPathPairsToMaterialize]
	}
	diag.pathPairsMaterialized = len(values)

	var out []store.ArtifactEdgeInput
	edgeCountsByArtifact := map[string]int{}
	seenEdgeKeys := map[string]bool{}
	for _, acc := range values {
		leftRefs := limitGitArtifactRefs(artifactsByPath[acc.leftPath])
		rightRefs := limitGitArtifactRefs(artifactsByPath[acc.rightPath])
		emittedForPair := 0
		for _, left := range leftRefs {
			for _, right := range rightRefs {
				if emittedForPair >= maxGitCoChangeEdgesPerSourcePair {
					diag.cappedArtifactEdges++
					continue
				}
				if left.id == right.id {
					continue
				}
				src, dst := orderedArtifactPair(left.id, right.id)
				if edgeCountsByArtifact[src] >= maxGitCoChangeEdgesPerArtifact || edgeCountsByArtifact[dst] >= maxGitCoChangeEdgesPerArtifact {
					diag.cappedArtifactEdges++
					continue
				}
				edgeKey := src + "\x00" + dst
				if seenEdgeKeys[edgeKey] {
					diag.cappedArtifactEdges++
					continue
				}
				seenEdgeKeys[edgeKey] = true
				weight, confidence, rule := gitCoChangeScore(shape, acc)
				out = append(out, store.ArtifactEdgeInput{
					ID:            stableEvidenceID("edge", repoID, src, dst, edgeTypeCoChangedWith, sourceSignalGitCoChange),
					RepoID:        repoID,
					SrcArtifactID: src,
					DstArtifactID: dst,
					EdgeType:      edgeTypeCoChangedWith,
					Weight:        weight,
					Confidence:    confidence,
					EvidenceCount: acc.evidence,
					Freshness:     acc.latest,
					SourceSignal:  sourceSignalGitCoChange,
					Explanation:   fmt.Sprintf("source paths co-changed in %d local git commit(s)", acc.evidence),
					MetadataJSON: evidenceJSON(map[string]any{
						"commits":             acc.commits,
						"file_examples":       []string{truncateGitEvidenceValue(acc.leftPath) + " + " + truncateGitEvidenceValue(acc.rightPath)},
						"source_paths":        []string{acc.leftPath, acc.rightPath},
						"history_shape":       shape,
						"confidence_rule":     rule,
						"source_path_density": acc.leftDensity,
						"target_path_density": acc.rightDensity,
						"high_density":        acc.highDensity,
					}),
				})
				edgeCountsByArtifact[src]++
				edgeCountsByArtifact[dst]++
				emittedForPair++
			}
		}
	}
	if len(out) > maxGitCoChangeEdges {
		diag.cappedArtifactEdges += len(out) - maxGitCoChangeEdges
		out = out[:maxGitCoChangeEdges]
	}
	return out
}

func gitCoChangeScore(shape string, acc *gitPathPairAccumulator) (float64, float64, string) {
	if shape != gitfacts.ShapeFull {
		return 0.36, 0.48, "low_non_full_history"
	}
	if acc.highDensity {
		if acc.evidence >= 2 {
			return 0.58, 0.72, "medium_repeated_high_density_path"
		}
		return 0.42, 0.56, "low_single_high_density_path"
	}
	if acc.evidence >= 2 {
		return 0.74, 0.86, "high_repeated_full_history_file_pair"
	}
	return 0.56, 0.64, "medium_single_full_history_file_pair"
}

func buildGitRecentlyChangedEdges(repoID, shape string, recents map[string]*gitRecentAccumulator) []store.ArtifactEdgeInput {
	values := make([]*gitRecentAccumulator, 0, len(recents))
	for _, acc := range recents {
		values = append(values, acc)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].latest == values[j].latest {
			return values[i].artifactID < values[j].artifactID
		}
		return values[i].latest > values[j].latest
	})
	if len(values) > maxGitRecentlyChangedEdges {
		values = values[:maxGitRecentlyChangedEdges]
	}
	out := make([]store.ArtifactEdgeInput, 0, len(values))
	for _, acc := range values {
		out = append(out, store.ArtifactEdgeInput{
			ID:            stableEvidenceID("edge", repoID, acc.artifactID, acc.artifactID, edgeTypeRecentlyChanged, sourceSignalGitRecentCommit),
			RepoID:        repoID,
			SrcArtifactID: acc.artifactID,
			DstArtifactID: acc.artifactID,
			EdgeType:      edgeTypeRecentlyChanged,
			Weight:        0.42,
			Confidence:    0.62,
			EvidenceCount: acc.evidence,
			Freshness:     acc.latest,
			SourceSignal:  sourceSignalGitRecentCommit,
			Explanation:   "changed recently in local git history",
			MetadataJSON: evidenceJSON(map[string]any{
				"commits":         acc.commits,
				"path":            acc.path,
				"history_shape":   shape,
				"confidence_rule": "recent_git_touch",
			}),
		})
	}
	return out
}

func gitArtifactMaps(rows []store.ArtifactSourcePathRow) (map[string][]gitArtifactRef, map[string]gitArtifactRef) {
	byPath := map[string][]gitArtifactRef{}
	byID := map[string]gitArtifactRef{}
	for _, row := range rows {
		for _, path := range []string{row.Path, row.SourceIdentity} {
			path = sourceIdentityPath(path)
			path = normalizeGitEvidencePath(path)
			if path == "" {
				continue
			}
			ref := gitArtifactRef{
				id:             row.ArtifactID,
				kind:           row.Kind,
				subtype:        row.Subtype,
				title:          row.Title,
				path:           path,
				sourceIdentity: row.SourceIdentity,
			}
			byPath[path] = appendUniqueGitArtifactRef(byPath[path], ref)
			if existing, ok := byID[ref.id]; !ok || gitArtifactRepresentativeScore(ref) > gitArtifactRepresentativeScore(existing) {
				byID[ref.id] = ref
			}
		}
	}
	for path, refs := range byPath {
		sortGitArtifactRefs(refs)
		density := len(refs)
		highDensity := density > gitHighDensitySourcePathThreshold
		for i := range refs {
			refs[i].density = density
			refs[i].highDensity = highDensity
			if existing := byID[refs[i].id]; existing.path == refs[i].path || existing.path == "" {
				byID[refs[i].id] = refs[i]
			}
		}
		byPath[path] = refs
	}
	return byPath, byID
}

func appendUniqueGitArtifactRef(values []gitArtifactRef, value gitArtifactRef) []gitArtifactRef {
	for _, existing := range values {
		if existing.id == value.id {
			return values
		}
	}
	return append(values, value)
}

func sortGitArtifactRefs(refs []gitArtifactRef) {
	sort.Slice(refs, func(i, j int) bool {
		leftScore := gitArtifactRepresentativeScore(refs[i])
		rightScore := gitArtifactRepresentativeScore(refs[j])
		if leftScore == rightScore {
			if refs[i].title == refs[j].title {
				return refs[i].id < refs[j].id
			}
			return refs[i].title < refs[j].title
		}
		return leftScore > rightScore
	})
}

func limitGitArtifactRefs(refs []gitArtifactRef) []gitArtifactRef {
	if len(refs) <= maxGitRepresentativesPerPath {
		return refs
	}
	return refs[:maxGitRepresentativesPerPath]
}

func gitArtifactRepresentativeScore(ref gitArtifactRef) int {
	score := 0
	switch ref.kind {
	case "spec", "design", "requirements", "plan":
		score = 100
	case "markdown_artifact":
		score = 70
	case "source_context":
		switch ref.subtype {
		case "code_comment":
			score = 55
			title := strings.ToLower(ref.title)
			if strings.Contains(title, "todo") || strings.Contains(title, "invariant") {
				score += 10
			}
		case "test_case":
			score = 45
		default:
			score = 35
		}
	default:
		score = 30
	}
	if ref.subtype != "" {
		score += 5
	}
	return score
}

func sourceIdentityPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, "#"); idx >= 0 {
		value = value[:idx]
	}
	return value
}

func normalizeGitEvidencePath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	return path
}

func lockfileOnlyCommit(files []gitfacts.FileChange) bool {
	saw := false
	for _, file := range files {
		path := normalizeGitEvidencePath(file.FilePath)
		if path == "" || gitEvidenceNoisyPath(path) {
			continue
		}
		saw = true
		if !gitEvidenceLockfile(path) {
			return false
		}
	}
	return saw
}

func gitEvidenceNoisyPath(path string) bool {
	path = strings.ToLower(normalizeGitEvidencePath(path))
	if path == "" {
		return true
	}
	if gitEvidenceGeneratedPath(path) {
		return true
	}
	return false
}

func gitEvidenceGeneratedPath(path string) bool {
	segments := strings.Split(path, "/")
	for _, segment := range segments {
		switch segment {
		case ".git", "vendor", "node_modules", "dist", "build", "target", ".next", ".turbo", "coverage", "__pycache__":
			return true
		}
	}
	base := filepath.Base(path)
	if strings.HasSuffix(base, ".min.js") || strings.HasSuffix(base, ".generated.go") || strings.HasSuffix(base, ".pb.go") {
		return true
	}
	return false
}

func gitEvidenceLockfile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "go.sum", "cargo.lock", "gemfile.lock", "poetry.lock", "pdm.lock", "uv.lock", "composer.lock":
		return true
	default:
		return false
	}
}

func topGitEdgeExamples(edges []store.ArtifactEdgeInput, artifactsByID map[string]gitArtifactRef) []GitEvidenceEdgeExample {
	limit := maxGitEdgeExamples
	if len(edges) < limit {
		limit = len(edges)
	}
	out := make([]GitEvidenceEdgeExample, 0, limit)
	for _, edge := range edges[:limit] {
		meta := decodeGitEvidenceMetadata(edge.MetadataJSON)
		source := artifactsByID[edge.SrcArtifactID]
		target := artifactsByID[edge.DstArtifactID]
		out = append(out, GitEvidenceEdgeExample{
			EdgeType:          edge.EdgeType,
			Source:            edge.SrcArtifactID,
			Target:            edge.DstArtifactID,
			SourceTitle:       truncateGitEvidenceValue(source.title),
			TargetTitle:       truncateGitEvidenceValue(target.title),
			SourceKind:        source.kind,
			TargetKind:        target.kind,
			SourceSubtype:     source.subtype,
			TargetSubtype:     target.subtype,
			SourcePath:        truncateGitEvidenceValue(source.path),
			TargetPath:        truncateGitEvidenceValue(target.path),
			SourceSignal:      edge.SourceSignal,
			Explanation:       edge.Explanation,
			Confidence:        roundEvidence(edge.Confidence),
			EvidenceCount:     edge.EvidenceCount,
			Commits:           gitEvidenceStringSlice(meta["commits"]),
			FileExamples:      truncateGitEvidenceValues(gitEvidenceStringSlice(meta["file_examples"])),
			HistoryShape:      gitEvidenceString(meta["history_shape"]),
			ConfidenceRule:    gitEvidenceString(meta["confidence_rule"]),
			SourcePathDensity: gitEvidenceInt(meta["source_path_density"]),
			TargetPathDensity: gitEvidenceInt(meta["target_path_density"]),
		})
	}
	return out
}

func decodeGitEvidenceMetadata(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

func gitEvidenceString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func gitEvidenceStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := gitEvidenceString(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func gitEvidenceInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func truncateGitEvidenceValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, truncateGitEvidenceValue(value))
	}
	return out
}

func truncateGitEvidenceValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxGitEvidenceExampleValueLength {
		return value
	}
	if maxGitEvidenceExampleValueLength <= 3 {
		return value[:maxGitEvidenceExampleValueLength]
	}
	return value[:maxGitEvidenceExampleValueLength-3] + "..."
}

func sortGitEdges(edges []store.ArtifactEdgeInput) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].EdgeType == edges[j].EdgeType {
			if edges[i].EvidenceCount == edges[j].EvidenceCount {
				if edges[i].SrcArtifactID == edges[j].SrcArtifactID {
					return edges[i].DstArtifactID < edges[j].DstArtifactID
				}
				return edges[i].SrcArtifactID < edges[j].SrcArtifactID
			}
			return edges[i].EvidenceCount > edges[j].EvidenceCount
		}
		return edges[i].EdgeType < edges[j].EdgeType
	})
}

func sortedBoolMapKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func appendLimitedUnique(values []string, value string, limit int) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	if limit > 0 && len(values) >= limit {
		return values
	}
	return append(values, value)
}

func maxString(a, b string) string {
	if b > a {
		return b
	}
	return a
}

func shortSHA(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}
