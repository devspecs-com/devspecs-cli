package scan

import (
	"context"
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
)

// GitEvidenceDiagnostics is emitted by ds scan --json when git evidence is enabled.
type GitEvidenceDiagnostics struct {
	Enabled                    bool                  `json:"enabled"`
	HistoryShape               string                `json:"history_shape"`
	IsShallow                  bool                  `json:"is_shallow,omitempty"`
	Branch                     string                `json:"branch,omitempty"`
	MaxCommits                 int                   `json:"max_commits"`
	MaxFilesPerCommit          int                   `json:"max_files_per_commit"`
	TotalCommits               int                   `json:"total_commits,omitempty"`
	CommitsRead                int                   `json:"commits_read,omitempty"`
	CommitsStored              int                   `json:"commits_stored,omitempty"`
	FilesStored                int                   `json:"files_stored,omitempty"`
	EdgesIndexed               int                   `json:"edges_indexed,omitempty"`
	EdgesByType                map[string]int        `json:"edges_by_type,omitempty"`
	SkippedLargeCommits        int                   `json:"skipped_large_commits,omitempty"`
	SkippedMergeCommits        int                   `json:"skipped_merge_commits,omitempty"`
	SkippedLockfileOnlyCommits int                   `json:"skipped_lockfile_only_commits,omitempty"`
	SkippedNoMappedArtifacts   int                   `json:"skipped_no_mapped_artifacts,omitempty"`
	GitError                   string                `json:"git_error,omitempty"`
	TopEdges                   []EvidenceEdgeExample `json:"top_edges,omitempty"`
}

type gitCommitEvidence struct {
	commit gitfacts.Commit
	files  []gitfacts.FileChange
}

type gitPairAccumulator struct {
	src          string
	dst          string
	evidence     int
	commits      []string
	latest       string
	fileExamples []string
}

type gitRecentAccumulator struct {
	artifactID string
	evidence   int
	commits    []string
	latest     string
	path       string
}

func (s *Scanner) rebuildGitEvidence(ctx context.Context, repoRoot, repoID, now string, opts RunOptions) (*GitEvidenceDiagnostics, error) {
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
		return nil, err
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
		return nil, err
	}
	edges, edgeDiag, err := s.materializeGitEdges(repoID, facts)
	if err != nil {
		return nil, err
	}
	for _, edge := range edges {
		if err := s.db.UpsertArtifactEdge(edge, now); err != nil {
			return nil, err
		}
	}
	diag.EdgesIndexed = len(edges)
	diag.EdgesByType = edgeDiag.edgesByType
	diag.SkippedMergeCommits = edgeDiag.skippedMerge
	diag.SkippedLockfileOnlyCommits = edgeDiag.skippedLockfileOnly
	diag.SkippedNoMappedArtifacts = edgeDiag.skippedNoMappedArtifacts
	diag.TopEdges = topGitEdgeExamples(edges)
	return diag, nil
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
}

func (s *Scanner) materializeGitEdges(repoID string, facts gitfacts.Facts) ([]store.ArtifactEdgeInput, gitEdgeDiagnostics, error) {
	diag := gitEdgeDiagnostics{edgesByType: map[string]int{}}
	sourceRows, err := s.db.GetArtifactSourcePaths(repoID)
	if err != nil {
		return nil, diag, err
	}
	artifactsByPath := artifactsByGitPath(sourceRows)
	if len(artifactsByPath) == 0 {
		return nil, diag, nil
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
	pairs := map[string]*gitPairAccumulator{}
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
		artifactIDsByFile := map[string][]string{}
		for _, file := range entry.files {
			path := normalizeGitEvidencePath(file.FilePath)
			if path == "" || gitEvidenceNoisyPath(path) {
				continue
			}
			ids := artifactsByPath[path]
			if len(ids) == 0 {
				continue
			}
			artifactIDsByFile[path] = ids
			for _, id := range ids {
				acc := recents[id]
				if acc == nil {
					acc = &gitRecentAccumulator{artifactID: id, path: path}
					recents[id] = acc
				}
				acc.evidence++
				acc.latest = maxString(acc.latest, commit.CommittedAt)
				acc.commits = appendLimitedUnique(acc.commits, shortSHA(commit.SHA), maxGitCommitExamples)
			}
		}
		if len(artifactIDsByFile) == 0 {
			diag.skippedNoMappedArtifacts++
			continue
		}
		paths := sortedStringKeys(artifactIDsByFile)
		if len(paths) < 2 {
			continue
		}
		for i := 0; i < len(paths); i++ {
			for j := i + 1; j < len(paths); j++ {
				for _, leftID := range artifactIDsByFile[paths[i]] {
					for _, rightID := range artifactIDsByFile[paths[j]] {
						src, dst := orderedArtifactPair(leftID, rightID)
						key := src + "\x00" + dst
						acc := pairs[key]
						if acc == nil {
							acc = &gitPairAccumulator{src: src, dst: dst}
							pairs[key] = acc
						}
						acc.evidence++
						acc.latest = maxString(acc.latest, commit.CommittedAt)
						acc.commits = appendLimitedUnique(acc.commits, shortSHA(commit.SHA), maxGitCommitExamples)
						acc.fileExamples = appendLimitedUnique(acc.fileExamples, paths[i]+" + "+paths[j], 2)
					}
				}
			}
		}
	}
	edges := buildGitCoChangeEdges(repoID, facts.Diagnostics.HistoryShape, pairs)
	edges = append(edges, buildGitRecentlyChangedEdges(repoID, facts.Diagnostics.HistoryShape, recents)...)
	sortGitEdges(edges)
	if len(edges) > maxGitCoChangeEdges+maxGitRecentlyChangedEdges {
		edges = edges[:maxGitCoChangeEdges+maxGitRecentlyChangedEdges]
	}
	for _, edge := range edges {
		diag.edgesByType[edge.EdgeType]++
	}
	return edges, diag, nil
}

func buildGitCoChangeEdges(repoID, shape string, pairs map[string]*gitPairAccumulator) []store.ArtifactEdgeInput {
	values := make([]*gitPairAccumulator, 0, len(pairs))
	for _, acc := range pairs {
		values = append(values, acc)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].evidence == values[j].evidence {
			if values[i].src == values[j].src {
				return values[i].dst < values[j].dst
			}
			return values[i].src < values[j].src
		}
		return values[i].evidence > values[j].evidence
	})
	if len(values) > maxGitCoChangeEdges {
		values = values[:maxGitCoChangeEdges]
	}
	out := make([]store.ArtifactEdgeInput, 0, len(values))
	for _, acc := range values {
		confidence := 0.64
		weight := 0.56
		if acc.evidence >= 2 {
			confidence = 0.86
			weight = 0.74
		}
		if shape == gitfacts.ShapeSingleCommit || shape == gitfacts.ShapeShallow {
			confidence = minFloat(confidence, 0.68)
			weight = minFloat(weight, 0.58)
		}
		out = append(out, store.ArtifactEdgeInput{
			ID:            stableEvidenceID("edge", repoID, acc.src, acc.dst, edgeTypeCoChangedWith, sourceSignalGitCoChange),
			RepoID:        repoID,
			SrcArtifactID: acc.src,
			DstArtifactID: acc.dst,
			EdgeType:      edgeTypeCoChangedWith,
			Weight:        weight,
			Confidence:    confidence,
			EvidenceCount: acc.evidence,
			Freshness:     acc.latest,
			SourceSignal:  sourceSignalGitCoChange,
			Explanation:   fmt.Sprintf("co-changed in %d local git commit(s)", acc.evidence),
			MetadataJSON: evidenceJSON(map[string]any{
				"commits":       acc.commits,
				"file_examples": acc.fileExamples,
				"history_shape": shape,
			}),
		})
	}
	return out
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
				"commits":       acc.commits,
				"path":          acc.path,
				"history_shape": shape,
			}),
		})
	}
	return out
}

func artifactsByGitPath(rows []store.ArtifactSourcePathRow) map[string][]string {
	out := map[string][]string{}
	for _, row := range rows {
		for _, path := range []string{row.Path, row.SourceIdentity} {
			path = sourceIdentityPath(path)
			path = normalizeGitEvidencePath(path)
			if path == "" {
				continue
			}
			out[path] = appendUnique(out[path], row.ArtifactID)
		}
	}
	for path := range out {
		sort.Strings(out[path])
	}
	return out
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

func topGitEdgeExamples(edges []store.ArtifactEdgeInput) []EvidenceEdgeExample {
	limit := 8
	if len(edges) < limit {
		limit = len(edges)
	}
	out := make([]EvidenceEdgeExample, 0, limit)
	for _, edge := range edges[:limit] {
		out = append(out, EvidenceEdgeExample{
			EdgeType:      edge.EdgeType,
			Source:        edge.SrcArtifactID,
			Target:        edge.DstArtifactID,
			SourceSignal:  edge.SourceSignal,
			Explanation:   edge.Explanation,
			Confidence:    roundEvidence(edge.Confidence),
			EvidenceCount: edge.EvidenceCount,
		})
	}
	return out
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

func sortedStringKeys(values map[string][]string) []string {
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
