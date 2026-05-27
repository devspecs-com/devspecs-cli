package scan

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/gitfacts"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

const (
	conceptKindWorkstreamAnchor = "workstream_anchor"

	edgeTypeSameWorkstreamAnchor = "same_workstream_anchor"

	sourceSignalWorkstreamAnchor = "workstream_anchor"

	maxWorkstreamAnchorsPerArtifact  = 24
	maxWorkstreamAnchorsPerValue     = 8
	maxWorkstreamFormsPerAnchor      = 8
	maxWorkstreamCommitExamples      = 4
	maxWorkstreamArtifactExamples    = 5
	maxWorkstreamArtifactsPerAnchor  = 8
	maxWorkstreamAnchorsMaterialized = 250
	maxWorkstreamEdges               = 600
	maxWorkstreamEdgesPerAnchor      = 12
	maxWorkstreamRejectedExamples    = 8
	maxWorkstreamTopClusters         = 10
	maxWorkstreamBodyBytes           = 128 * 1024
	workstreamHighDFRatio            = 0.35

	workstreamPackStrengthStrong       = "strong"
	workstreamPackStrengthSupportCross = "support_cross_role"
	workstreamPackStrengthSupportLocal = "support_locality"
	workstreamPackStrengthWeak         = "weak"

	workstreamDialectTicketLikeUpper    = "ticket_like_upper"
	workstreamDialectDocumentNumberRef  = "document_number_ref"
	workstreamDialectExplicitPRRef      = "explicit_pr_ref"
	workstreamDialectExplicitIssueRef   = "explicit_issue_ref"
	workstreamDialectExplicitGHRef      = "explicit_gh_ref"
	workstreamDialectBareHashRef        = "bare_hash_ref"
	workstreamDialectOpenSpecChangeSlug = "openspec_change_slug"
	workstreamDialectBranchSlug         = "branch_slug"
	workstreamDialectCommitSlug         = "commit_slug"
	workstreamDialectPathComponentSlug  = "path_or_component_slug"
	workstreamDialectTitleHeadingSlug   = "title_or_heading_slug"
	workstreamDialectGenericTechnical   = "generic_technical_term"
	workstreamDialectUnknown            = "unknown"

	workstreamTrustWeak     = "weak"
	workstreamTrustModerate = "moderate"
	workstreamTrustStrong   = "strong"
)

var (
	workstreamTaskIDPattern = regexp.MustCompile(`\b[A-Z][A-Z0-9]{1,9}-[0-9]{1,6}\b`)
	workstreamRefPattern    = regexp.MustCompile(`(?i)(?:\b(GH|PR|ISSUE)-|(?:\b(issues|pull)/)|#)([0-9]{1,6})\b`)
)

// WorkstreamEvidenceDiagnostics is emitted by ds scan --json when workstream evidence is enabled.
type WorkstreamEvidenceDiagnostics struct {
	Enabled               bool                                 `json:"enabled"`
	AnchorsSeen           int                                  `json:"anchors_seen,omitempty"`
	AnchorsMaterialized   int                                  `json:"anchors_materialized,omitempty"`
	MentionsIndexed       int                                  `json:"mentions_indexed,omitempty"`
	EdgesIndexed          int                                  `json:"edges_indexed,omitempty"`
	EdgesByType           map[string]int                       `json:"edges_by_type,omitempty"`
	EdgesByPackStrength   map[string]int                       `json:"edges_by_pack_strength,omitempty"`
	DialectCounts         map[string]int                       `json:"dialect_counts,omitempty"`
	CappedDialectCounts   map[string]int                       `json:"capped_dialect_counts,omitempty"`
	ClustersCapped        int                                  `json:"clusters_capped,omitempty"`
	PackStrengthCounts    map[string]int                       `json:"pack_strength_counts,omitempty"`
	CappedPackStrengths   map[string]int                       `json:"capped_pack_strength_counts,omitempty"`
	CappedRoleFamilies    map[string]int                       `json:"capped_role_family_counts,omitempty"`
	StrongOrCrossClusters int                                  `json:"strong_or_cross_clusters,omitempty"`
	StrongOrCrossEdges    int                                  `json:"strong_or_cross_edges,omitempty"`
	DialectProfile        *WorkstreamDialectProfileDiagnostics `json:"dialect_profile,omitempty"`
	NoisyAnchorsRejected  int                                  `json:"noisy_anchors_rejected,omitempty"`
	RejectedByReason      map[string]int                       `json:"rejected_by_reason,omitempty"`
	TopRejectedAnchors    []WorkstreamRejectedAnchorExample    `json:"top_rejected_anchors,omitempty"`
	TopClusters           []WorkstreamClusterExample           `json:"top_clusters,omitempty"`
	TopCappedClusters     []WorkstreamClusterExample           `json:"top_capped_clusters,omitempty"`
}

// WorkstreamDialectProfileDiagnostics summarizes repo-local work-reference conventions.
type WorkstreamDialectProfileDiagnostics struct {
	DialectCounts   map[string]int            `json:"dialect_counts,omitempty"`
	CrossRoleCounts map[string]int            `json:"cross_role_counts,omitempty"`
	TrustedDialects map[string]string         `json:"trusted_dialects,omitempty"`
	RiskFlags       map[string]int            `json:"risk_flags,omitempty"`
	EvidenceSources map[string]map[string]int `json:"evidence_sources,omitempty"`
	RoleFamilies    map[string]map[string]int `json:"role_families,omitempty"`
}

// WorkstreamClusterExample gives a compact receipt for a materialized anchor cluster.
type WorkstreamClusterExample struct {
	Anchor           string                      `json:"anchor"`
	AnchorType       string                      `json:"anchor_type"`
	Dialect          string                      `json:"dialect,omitempty"`
	PackStrength     string                      `json:"pack_strength,omitempty"`
	Confidence       float64                     `json:"confidence"`
	ConfidenceRule   string                      `json:"confidence_rule"`
	ArtifactCount    int                         `json:"artifact_count"`
	EvidenceCount    int                         `json:"evidence_count"`
	RoleMix          map[string]int              `json:"role_mix,omitempty"`
	RoleFamilyMix    map[string]int              `json:"role_family_mix,omitempty"`
	EvidenceSources  []string                    `json:"evidence_sources,omitempty"`
	ExampleArtifacts []WorkstreamArtifactExample `json:"example_artifacts,omitempty"`
	ExampleCommits   []string                    `json:"example_commits,omitempty"`
	Capped           bool                        `json:"capped,omitempty"`
	Caveats          []string                    `json:"caveats,omitempty"`
}

// WorkstreamArtifactExample identifies one artifact in a cluster receipt.
type WorkstreamArtifactExample struct {
	ID      string `json:"id"`
	Title   string `json:"title,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Subtype string `json:"subtype,omitempty"`
	Path    string `json:"path,omitempty"`
}

// WorkstreamRejectedAnchorExample explains a suppressed anchor.
type WorkstreamRejectedAnchorExample struct {
	Anchor  string `json:"anchor"`
	Type    string `json:"type,omitempty"`
	Dialect string `json:"dialect,omitempty"`
	Reason  string `json:"reason"`
	Source  string `json:"source,omitempty"`
}

type workstreamEvidenceBuildResult struct {
	concepts    []store.ConceptInput
	mentions    []store.ConceptMentionInput
	edges       []store.ArtifactEdgeInput
	diagnostics *WorkstreamEvidenceDiagnostics
}

type workstreamAnchor struct {
	canonical  string
	display    string
	anchorType string
	dialect    string
	raw        string
	source     string
	context    string
	weight     float64
}

type workstreamRejectedAnchor struct {
	anchor     string
	anchorType string
	dialect    string
	reason     string
	source     string
}

type workstreamExtractResult struct {
	anchors  []workstreamAnchor
	rejected []workstreamRejectedAnchor
}

type workstreamAnchorAccumulator struct {
	canonical string
	display   string
	types     map[string]bool
	dialects  map[string]bool
	forms     map[string]bool
	sources   map[string]bool
	contexts  map[string]bool
	artifacts map[string]*workstreamArtifactAccumulator
	commits   []string
	files     []string
	latest    string
}

type workstreamArtifactAccumulator struct {
	ref      gitArtifactRef
	sources  map[string]bool
	fields   map[string]bool
	forms    map[string]bool
	commits  []string
	weight   float64
	evidence int
}

type workstreamAcceptedCluster struct {
	acc            *workstreamAnchorAccumulator
	artifactIDs    []string
	weight         float64
	confidence     float64
	confidenceRule string
	packStrength   string
	dialect        string
	capped         bool
	caveats        []string
}

type workstreamPairAccumulator struct {
	src           string
	dst           string
	anchors       []map[string]any
	weight        float64
	confidence    float64
	packStrength  string
	evidence      int
	freshness     string
	roleMix       map[string]int
	roleFamilyMix map[string]int
}

type workstreamDialectProfile struct {
	stats map[string]*workstreamDialectProfileStat
	trust map[string]string
	risks map[string]int
}

type workstreamDialectProfileStat struct {
	anchors      int
	crossRole    int
	sources      map[string]int
	roleFamilies map[string]int
}

func (s *Scanner) rebuildWorkstreamEvidence(repoID, now string, facts gitfacts.Facts) (*WorkstreamEvidenceDiagnostics, error) {
	artifacts, err := s.loadEvidenceArtifacts(repoID)
	if err != nil {
		return nil, err
	}
	sourceRows, err := s.db.GetArtifactSourcePaths(repoID)
	if err != nil {
		return nil, err
	}
	artifactsByPath, artifactsByID := gitArtifactMaps(sourceRows)
	built := buildWorkstreamEvidence(repoID, artifacts, artifactsByPath, artifactsByID, facts)
	if err := s.db.ReplaceRepoEvidenceScope(repoID, conceptKindWorkstreamAnchor, edgeTypeSameWorkstreamAnchor, built.concepts, built.mentions, built.edges, now); err != nil {
		return nil, err
	}
	return built.diagnostics, nil
}

func buildWorkstreamEvidence(repoID string, artifacts []evidenceArtifact, artifactsByPath map[string][]gitArtifactRef, artifactsByID map[string]gitArtifactRef, facts gitfacts.Facts) workstreamEvidenceBuildResult {
	diag := &WorkstreamEvidenceDiagnostics{
		Enabled:             true,
		EdgesByType:         map[string]int{},
		EdgesByPackStrength: map[string]int{},
		RejectedByReason:    map[string]int{},
	}
	accs := map[string]*workstreamAnchorAccumulator{}
	addRejected := func(rej workstreamRejectedAnchor) {
		if rej.reason == "" {
			return
		}
		diag.NoisyAnchorsRejected++
		diag.RejectedByReason[rej.reason]++
		if len(diag.TopRejectedAnchors) < maxWorkstreamRejectedExamples {
			diag.TopRejectedAnchors = append(diag.TopRejectedAnchors, WorkstreamRejectedAnchorExample{
				Anchor:  rej.anchor,
				Type:    rej.anchorType,
				Dialect: rej.dialect,
				Reason:  rej.reason,
				Source:  rej.source,
			})
		}
	}
	ensure := func(anchor workstreamAnchor) *workstreamAnchorAccumulator {
		if anchor.canonical == "" {
			return nil
		}
		acc := accs[anchor.canonical]
		if acc == nil {
			acc = &workstreamAnchorAccumulator{
				canonical: anchor.canonical,
				display:   anchor.display,
				types:     map[string]bool{},
				dialects:  map[string]bool{},
				forms:     map[string]bool{},
				sources:   map[string]bool{},
				contexts:  map[string]bool{},
				artifacts: map[string]*workstreamArtifactAccumulator{},
			}
			accs[anchor.canonical] = acc
		}
		if acc.display == "" {
			acc.display = anchor.display
		}
		if anchor.anchorType != "" {
			acc.types[anchor.anchorType] = true
		}
		dialect := anchor.dialect
		if dialect == "" {
			dialect = workstreamDialectForAnchorType(anchor.anchorType)
		}
		if dialect != "" {
			acc.dialects[dialect] = true
		}
		if anchor.raw != "" {
			acc.forms[anchor.raw] = true
		}
		if anchor.source != "" {
			acc.sources[anchor.source] = true
		}
		if anchor.context != "" {
			acc.contexts[anchor.context] = true
		}
		return acc
	}
	addArtifact := func(anchor workstreamAnchor, ref gitArtifactRef, field, source, commit string, weight float64) {
		if ref.id == "" {
			return
		}
		acc := ensure(anchor)
		if acc == nil {
			return
		}
		art := acc.artifacts[ref.id]
		if art == nil {
			art = &workstreamArtifactAccumulator{
				ref:     ref,
				sources: map[string]bool{},
				fields:  map[string]bool{},
				forms:   map[string]bool{},
				weight:  weight,
			}
			acc.artifacts[ref.id] = art
		}
		if gitArtifactRepresentativeScore(ref) > gitArtifactRepresentativeScore(art.ref) {
			art.ref = ref
		}
		if source != "" {
			art.sources[source] = true
			acc.sources[source] = true
		}
		if field != "" {
			art.fields[field] = true
		}
		if anchor.raw != "" {
			art.forms[anchor.raw] = true
		}
		if weight > art.weight {
			art.weight = weight
		}
		art.evidence++
		if commit != "" {
			short := shortSHA(commit)
			art.commits = appendLimitedUnique(art.commits, short, maxWorkstreamCommitExamples)
			acc.commits = appendLimitedUnique(acc.commits, short, maxWorkstreamCommitExamples)
		}
	}
	addGit := func(anchor workstreamAnchor, source, commit, filePath, committedAt string) {
		acc := ensure(anchor)
		if acc == nil {
			return
		}
		if source != "" {
			acc.sources[source] = true
		}
		if commit != "" {
			acc.commits = appendLimitedUnique(acc.commits, shortSHA(commit), maxWorkstreamCommitExamples)
		}
		if filePath != "" {
			acc.files = appendLimitedUnique(acc.files, truncateGitEvidenceValue(filePath), maxWorkstreamCommitExamples)
		}
		acc.latest = maxString(acc.latest, committedAt)
	}

	refsByArtifact := refsByArtifactID(artifacts, artifactsByID)
	for _, artifact := range artifacts {
		ref := refsByArtifact[artifact.id]
		if ref.id == "" {
			continue
		}
		perArtifact := 0
		for _, src := range artifact.sources {
			pathExtracted := extractWorkstreamAnchorsFromPath(src.Path, "artifact_path")
			for _, extracted := range pathExtracted.anchors {
				if perArtifact >= maxWorkstreamAnchorsPerArtifact {
					break
				}
				addArtifact(extracted, ref, "path", "artifact_path", "", extracted.weight)
				perArtifact++
			}
			for _, rej := range pathExtracted.rejected {
				addRejected(rej)
			}
		}
		titleExtracted := extractWorkstreamAnchorsFromSlugText(artifact.title, "artifact_title")
		for _, rej := range titleExtracted.rejected {
			addRejected(rej)
		}
		for _, anchor := range titleExtracted.anchors {
			if perArtifact >= maxWorkstreamAnchorsPerArtifact {
				break
			}
			addArtifact(anchor, ref, "title", "artifact_title", "", anchor.weight)
			perArtifact++
		}
		if changeID := evidenceString(artifact.extracted["openspec_change_id"]); changeID != "" {
			metaExtracted := extractWorkstreamAnchorsFromSlugText(changeID, "metadata")
			for _, rej := range metaExtracted.rejected {
				addRejected(rej)
			}
			for _, anchor := range metaExtracted.anchors {
				if perArtifact >= maxWorkstreamAnchorsPerArtifact {
					break
				}
				anchor.anchorType = "change_slug"
				anchor.dialect = workstreamDialectOpenSpecChangeSlug
				addArtifact(anchor, ref, "openspec_change_id", "metadata", "", 0.92)
				perArtifact++
			}
		}
		for _, section := range artifact.sections {
			sectionExtracted := extractWorkstreamAnchorsFromSlugText(section.Title, "heading")
			for _, rej := range sectionExtracted.rejected {
				addRejected(rej)
			}
			for _, anchor := range sectionExtracted.anchors {
				if perArtifact >= maxWorkstreamAnchorsPerArtifact {
					break
				}
				addArtifact(anchor, ref, "heading", "heading", "", 0.68)
				perArtifact++
			}
		}
		body := artifact.body
		if len(body) > maxWorkstreamBodyBytes {
			body = body[:maxWorkstreamBodyBytes]
		}
		bodyExtracted := extractFormalWorkstreamAnchors(body, "body")
		for _, rej := range bodyExtracted.rejected {
			addRejected(rej)
		}
		for _, anchor := range bodyExtracted.anchors {
			if perArtifact >= maxWorkstreamAnchorsPerArtifact {
				break
			}
			addArtifact(anchor, ref, "body", "body", "", 0.62)
			perArtifact++
		}
	}

	branchExtracted := extractWorkstreamAnchorsFromBranch(facts.Diagnostics.Branch)
	for _, rej := range branchExtracted.rejected {
		addRejected(rej)
	}
	for _, anchor := range branchExtracted.anchors {
		addGit(anchor, "branch", "", "", "")
	}

	filesByCommit := map[string][]gitfacts.FileChange{}
	for _, file := range facts.Files {
		filesByCommit[file.CommitSHA] = append(filesByCommit[file.CommitSHA], file)
	}
	for _, commit := range facts.Commits {
		if commit.IsMerge {
			continue
		}
		files := filesByCommit[commit.SHA]
		mappedRefs := refsForGitFiles(files, artifactsByPath)
		messageExtracted := extractWorkstreamAnchorsFromCommitMessage(commit.Message)
		for _, rej := range messageExtracted.rejected {
			addRejected(rej)
		}
		for _, anchor := range messageExtracted.anchors {
			addGit(anchor, "commit_message", commit.SHA, "", commit.CommittedAt)
			for _, ref := range mappedRefs {
				addArtifact(anchor, ref, "commit_message", "commit_message_changed_file", commit.SHA, 0.74)
			}
		}
		for _, file := range files {
			path := normalizeGitEvidencePath(file.FilePath)
			if path == "" || gitEvidenceNoisyPath(path) {
				continue
			}
			fileExtracted := extractWorkstreamAnchorsFromPath(path, "git_changed_file")
			for _, rej := range fileExtracted.rejected {
				addRejected(rej)
			}
			for _, anchor := range fileExtracted.anchors {
				addGit(anchor, "git_changed_file", commit.SHA, path, commit.CommittedAt)
				for _, ref := range limitGitArtifactRefs(artifactsByPath[path]) {
					addArtifact(anchor, ref, "git_changed_file", "git_changed_file", commit.SHA, 0.72)
				}
			}
		}
	}

	diag.AnchorsSeen = len(accs)
	profile := buildWorkstreamDialectProfile(accs)
	diag.DialectProfile = profile.Diagnostics()
	accepted := acceptedWorkstreamClusters(accs, len(artifacts), profile, diag, addRejected)
	concepts, mentions := workstreamConceptsAndMentions(repoID, accepted, len(artifacts))
	edges := workstreamEdges(repoID, accepted, refsByArtifact, diag)
	diag.AnchorsMaterialized = len(concepts)
	diag.MentionsIndexed = len(mentions)
	diag.EdgesIndexed = len(edges)
	for _, edge := range edges {
		diag.EdgesByType[edge.EdgeType]++
		meta := decodeGitEvidenceMetadata(edge.MetadataJSON)
		if pack := evidenceString(meta["pack_strength"]); pack != "" {
			diag.EdgesByPackStrength[pack]++
			if pack == workstreamPackStrengthStrong || pack == workstreamPackStrengthSupportCross {
				diag.StrongOrCrossEdges++
			}
		}
	}
	diag.TopClusters = topWorkstreamClusters(accepted, refsByArtifact)
	if len(diag.RejectedByReason) == 0 {
		diag.RejectedByReason = nil
	}
	if len(diag.EdgesByType) == 0 {
		diag.EdgesByType = nil
	}
	if len(diag.EdgesByPackStrength) == 0 {
		diag.EdgesByPackStrength = nil
	}
	return workstreamEvidenceBuildResult{
		concepts:    concepts,
		mentions:    mentions,
		edges:       edges,
		diagnostics: diag,
	}
}

func buildWorkstreamDialectProfile(accs map[string]*workstreamAnchorAccumulator) workstreamDialectProfile {
	profile := workstreamDialectProfile{
		stats: map[string]*workstreamDialectProfileStat{},
		trust: map[string]string{},
		risks: map[string]int{},
	}
	for _, acc := range accs {
		dialect := acc.primaryDialect()
		if dialect == "" {
			dialect = workstreamDialectUnknown
		}
		stat := profile.stats[dialect]
		if stat == nil {
			stat = &workstreamDialectProfileStat{
				sources:      map[string]int{},
				roleFamilies: map[string]int{},
			}
			profile.stats[dialect] = stat
		}
		stat.anchors++
		artifactIDs := sortedWorkstreamArtifactIDs(acc.artifacts)
		roleFamilies := workstreamRoleFamilyMix(acc, artifactIDs)
		if workstreamHasDocBackedFamilyMix(roleFamilies) {
			stat.crossRole++
		}
		for source := range acc.sources {
			stat.sources[source]++
		}
		for family, count := range roleFamilies {
			stat.roleFamilies[family] += count
		}
	}
	for dialect, stat := range profile.stats {
		profile.trust[dialect] = workstreamDialectTrust(dialect, stat)
	}
	if stat := profile.stats[workstreamDialectBareHashRef]; stat != nil && stat.anchors >= 50 {
		profile.risks["bare_hash_ref_dominant"] = stat.anchors
	}
	if stat := profile.stats[workstreamDialectPathComponentSlug]; stat != nil && stat.anchors >= 100 {
		profile.risks["component_slug_dominant"] = stat.anchors
	}
	return profile
}

func workstreamDialectTrust(dialect string, stat *workstreamDialectProfileStat) string {
	if stat == nil || stat.anchors == 0 {
		return workstreamTrustWeak
	}
	switch dialect {
	case workstreamDialectTicketLikeUpper:
		if stat.crossRole >= 2 {
			return workstreamTrustStrong
		}
		if stat.crossRole >= 1 || stat.anchors >= 3 {
			return workstreamTrustModerate
		}
	case workstreamDialectDocumentNumberRef:
		if stat.crossRole >= 2 {
			return workstreamTrustModerate
		}
	case workstreamDialectOpenSpecChangeSlug:
		if stat.crossRole >= 1 {
			return workstreamTrustStrong
		}
		if stat.anchors >= 2 {
			return workstreamTrustModerate
		}
	case workstreamDialectExplicitPRRef, workstreamDialectExplicitIssueRef, workstreamDialectExplicitGHRef:
		if stat.crossRole >= 2 {
			return workstreamTrustStrong
		}
		if stat.crossRole >= 1 {
			return workstreamTrustModerate
		}
	case workstreamDialectPathComponentSlug, workstreamDialectTitleHeadingSlug:
		if stat.crossRole >= 10 {
			return workstreamTrustStrong
		}
		if stat.crossRole >= 2 {
			return workstreamTrustModerate
		}
	case workstreamDialectBareHashRef, workstreamDialectBranchSlug, workstreamDialectCommitSlug, workstreamDialectGenericTechnical:
		return workstreamTrustWeak
	}
	return workstreamTrustWeak
}

func (profile workstreamDialectProfile) Diagnostics() *WorkstreamDialectProfileDiagnostics {
	if len(profile.stats) == 0 {
		return nil
	}
	out := &WorkstreamDialectProfileDiagnostics{
		DialectCounts:   map[string]int{},
		CrossRoleCounts: map[string]int{},
		TrustedDialects: map[string]string{},
		RiskFlags:       map[string]int{},
		EvidenceSources: map[string]map[string]int{},
		RoleFamilies:    map[string]map[string]int{},
	}
	for dialect, stat := range profile.stats {
		out.DialectCounts[dialect] = stat.anchors
		if stat.crossRole > 0 {
			out.CrossRoleCounts[dialect] = stat.crossRole
		}
		if trust := profile.trust[dialect]; trust != "" {
			out.TrustedDialects[dialect] = trust
		}
		if len(stat.sources) > 0 {
			out.EvidenceSources[dialect] = copyIntMap(stat.sources)
		}
		if len(stat.roleFamilies) > 0 {
			out.RoleFamilies[dialect] = copyIntMap(stat.roleFamilies)
		}
	}
	for risk, count := range profile.risks {
		out.RiskFlags[risk] = count
	}
	if len(out.CrossRoleCounts) == 0 {
		out.CrossRoleCounts = nil
	}
	if len(out.RiskFlags) == 0 {
		out.RiskFlags = nil
	}
	return out
}

func acceptedWorkstreamClusters(accs map[string]*workstreamAnchorAccumulator, artifactCount int, profile workstreamDialectProfile, diag *WorkstreamEvidenceDiagnostics, reject func(workstreamRejectedAnchor)) []workstreamAcceptedCluster {
	values := make([]*workstreamAnchorAccumulator, 0, len(accs))
	for _, acc := range accs {
		values = append(values, acc)
	}
	sort.Slice(values, func(i, j int) bool {
		if len(values[i].artifacts) == len(values[j].artifacts) {
			return values[i].canonical < values[j].canonical
		}
		return len(values[i].artifacts) > len(values[j].artifacts)
	})
	var accepted []workstreamAcceptedCluster
	for _, acc := range values {
		sourceCount := len(acc.sources)
		artifactIDs := sortedWorkstreamArtifactIDs(acc.artifacts)
		if len(artifactIDs) == 0 {
			reject(workstreamRejectedAnchor{anchor: acc.display, anchorType: acc.primaryType(), dialect: acc.primaryDialect(), reason: "no_artifacts", source: "materialization"})
			continue
		}
		if sourceCount < 2 {
			reject(workstreamRejectedAnchor{anchor: acc.display, anchorType: acc.primaryType(), dialect: acc.primaryDialect(), reason: "single_evidence_source", source: "materialization"})
			continue
		}
		if acc.gitOnlySupport() && !acc.formalAnchor() {
			reject(workstreamRejectedAnchor{anchor: acc.display, anchorType: acc.primaryType(), dialect: acc.primaryDialect(), reason: "git_only_slug", source: "materialization"})
			continue
		}
		if !acc.formalAnchor() && acc.nativeArtifactSourceCount() == 0 {
			reject(workstreamRejectedAnchor{anchor: acc.display, anchorType: acc.primaryType(), dialect: acc.primaryDialect(), reason: "no_native_artifact_source", source: "materialization"})
			continue
		}
		if len(artifactIDs) > maxWorkstreamArtifactsPerAnchor {
			if artifactCount >= 10 && float64(len(artifactIDs))/float64(artifactCount) > workstreamHighDFRatio {
				reject(workstreamRejectedAnchor{anchor: acc.display, anchorType: acc.primaryType(), dialect: acc.primaryDialect(), reason: "high_document_frequency", source: "materialization"})
				continue
			}
			diag.ClustersCapped++
			artifactIDs = selectWorkstreamArtifactIDs(acc, artifactIDs, maxWorkstreamArtifactsPerAnchor)
		}
		weight, confidence, rule, packStrength := workstreamScore(acc, artifactIDs, profile)
		cluster := workstreamAcceptedCluster{
			acc:            acc,
			artifactIDs:    artifactIDs,
			weight:         weight,
			confidence:     confidence,
			confidenceRule: rule,
			packStrength:   packStrength,
			dialect:        acc.primaryDialect(),
			capped:         len(acc.artifacts) > len(artifactIDs),
		}
		if acc.gitOnlySupport() {
			cluster.caveats = append(cluster.caveats, "supported mostly by git/file evidence")
		}
		if packStrength == workstreamPackStrengthWeak {
			cluster.caveats = append(cluster.caveats, "weak pack candidate; evidence is mostly single-family code/test locality")
		} else if packStrength == workstreamPackStrengthSupportLocal {
			cluster.caveats = append(cluster.caveats, "locality support only; no doc/model-to-implementation bridge")
		}
		if acc.primaryDialect() == workstreamDialectBareHashRef {
			cluster.caveats = append(cluster.caveats, "bare hash reference; not eligible for strong evidence")
		}
		if acc.primaryDialect() == workstreamDialectGenericTechnical {
			cluster.caveats = append(cluster.caveats, "generic technical term; requires a specific work anchor for strong evidence")
		}
		if acc.primaryDialect() == workstreamDialectDocumentNumberRef {
			cluster.caveats = append(cluster.caveats, "document number reference; not eligible for strong evidence without a more specific work anchor")
		}
		accepted = append(accepted, cluster)
	}
	sort.Slice(accepted, func(i, j int) bool {
		if workstreamPackStrengthRank(accepted[i].packStrength) != workstreamPackStrengthRank(accepted[j].packStrength) {
			return workstreamPackStrengthRank(accepted[i].packStrength) > workstreamPackStrengthRank(accepted[j].packStrength)
		}
		if accepted[i].confidence == accepted[j].confidence {
			if len(accepted[i].artifactIDs) == len(accepted[j].artifactIDs) {
				return accepted[i].acc.canonical < accepted[j].acc.canonical
			}
			return len(accepted[i].artifactIDs) > len(accepted[j].artifactIDs)
		}
		return accepted[i].confidence > accepted[j].confidence
	})
	if len(accepted) > maxWorkstreamAnchorsMaterialized {
		capped := append([]workstreamAcceptedCluster(nil), accepted[maxWorkstreamAnchorsMaterialized:]...)
		diag.ClustersCapped += len(capped)
		diag.CappedPackStrengths = map[string]int{}
		diag.CappedRoleFamilies = map[string]int{}
		diag.CappedDialectCounts = map[string]int{}
		for _, cluster := range capped {
			diag.CappedPackStrengths[cluster.packStrength]++
			diag.CappedDialectCounts[cluster.dialect]++
			for family, count := range workstreamRoleFamilyMix(cluster.acc, cluster.artifactIDs) {
				diag.CappedRoleFamilies[family] += count
			}
		}
		diag.TopCappedClusters = workstreamClusterExamples(capped, maxWorkstreamTopClusters)
		accepted = accepted[:maxWorkstreamAnchorsMaterialized]
	}
	diag.PackStrengthCounts = map[string]int{}
	diag.DialectCounts = map[string]int{}
	for _, cluster := range accepted {
		diag.PackStrengthCounts[cluster.packStrength]++
		diag.DialectCounts[cluster.dialect]++
		if cluster.packStrength == workstreamPackStrengthStrong || cluster.packStrength == workstreamPackStrengthSupportCross {
			diag.StrongOrCrossClusters++
		}
	}
	return accepted
}

func workstreamConceptsAndMentions(repoID string, clusters []workstreamAcceptedCluster, artifactCount int) ([]store.ConceptInput, []store.ConceptMentionInput) {
	totalDocs := float64(maxInt(1, artifactCount))
	var concepts []store.ConceptInput
	var mentions []store.ConceptMentionInput
	for _, cluster := range clusters {
		acc := cluster.acc
		conceptID := stableEvidenceID("concept", repoID, conceptKindWorkstreamAnchor, acc.canonical)
		concepts = append(concepts, store.ConceptInput{
			ID:                       conceptID,
			RepoID:                   repoID,
			Canonical:                acc.canonical,
			Kind:                     conceptKindWorkstreamAnchor,
			Forms:                    limitedSortedMapKeys(acc.forms, maxWorkstreamFormsPerAnchor),
			DocumentFrequency:        len(cluster.artifactIDs),
			InverseDocumentFrequency: math.Log((1.0+totalDocs)/(1.0+float64(len(cluster.artifactIDs)))) + 1.0,
		})
		for _, artifactID := range cluster.artifactIDs {
			art := acc.artifacts[artifactID]
			mentions = append(mentions, store.ConceptMentionInput{
				ID:         stableEvidenceID("mention", conceptID, artifactID, "workstream_anchor"),
				ConceptID:  conceptID,
				ArtifactID: artifactID,
				Field:      "workstream_anchor",
				Weight:     art.weight,
				EvidenceJSON: evidenceJSON(map[string]any{
					"anchor":          acc.display,
					"anchor_type":     acc.primaryType(),
					"dialect":         cluster.dialect,
					"confidence":      cluster.confidence,
					"confidence_rule": cluster.confidenceRule,
					"pack_strength":   cluster.packStrength,
					"role_family":     workstreamRoleFamily(art.ref),
					"forms":           limitedSortedMapKeys(art.forms, maxWorkstreamFormsPerAnchor),
					"sources":         sortedMapKeys(art.sources),
					"fields":          sortedMapKeys(art.fields),
					"contexts":        sortedMapKeys(acc.contexts),
					"commits":         art.commits,
					"evidence":        art.evidence,
				}),
			})
		}
	}
	sort.Slice(concepts, func(i, j int) bool { return concepts[i].Canonical < concepts[j].Canonical })
	sort.Slice(mentions, func(i, j int) bool {
		if mentions[i].ArtifactID == mentions[j].ArtifactID {
			return mentions[i].ConceptID < mentions[j].ConceptID
		}
		return mentions[i].ArtifactID < mentions[j].ArtifactID
	})
	return concepts, mentions
}

func workstreamEdges(repoID string, clusters []workstreamAcceptedCluster, refsByArtifact map[string]gitArtifactRef, diag *WorkstreamEvidenceDiagnostics) []store.ArtifactEdgeInput {
	pairs := map[string]*workstreamPairAccumulator{}
	for _, cluster := range clusters {
		if len(cluster.artifactIDs) < 2 {
			continue
		}
		edgeLimit := maxWorkstreamEdgesForCluster(cluster.packStrength)
		if edgeLimit <= 0 {
			continue
		}
		emittedForAnchor := 0
		for i := 0; i < len(cluster.artifactIDs); i++ {
			for j := i + 1; j < len(cluster.artifactIDs); j++ {
				if emittedForAnchor >= edgeLimit {
					diag.ClustersCapped++
					break
				}
				src, dst := orderedArtifactPair(cluster.artifactIDs[i], cluster.artifactIDs[j])
				key := src + "\x00" + dst
				pair := pairs[key]
				if pair == nil {
					pair = &workstreamPairAccumulator{src: src, dst: dst, roleMix: map[string]int{}, roleFamilyMix: map[string]int{}}
					pairs[key] = pair
				}
				anchorMeta := map[string]any{
					"anchor":          cluster.acc.display,
					"canonical":       cluster.acc.canonical,
					"anchor_type":     cluster.acc.primaryType(),
					"dialect":         cluster.dialect,
					"confidence":      cluster.confidence,
					"weight":          cluster.weight,
					"pack_strength":   cluster.packStrength,
					"confidence_rule": cluster.confidenceRule,
					"sources":         sortedMapKeys(cluster.acc.sources),
					"contexts":        sortedMapKeys(cluster.acc.contexts),
					"commits":         cluster.acc.commits,
				}
				pair.anchors = append(pair.anchors, anchorMeta)
				pair.evidence += cluster.acc.evidenceCount()
				pair.freshness = maxString(pair.freshness, cluster.acc.latest)
				if cluster.weight > pair.weight {
					pair.weight = cluster.weight
				}
				if cluster.confidence > pair.confidence {
					pair.confidence = cluster.confidence
				}
				if workstreamPackStrengthRank(cluster.packStrength) > workstreamPackStrengthRank(pair.packStrength) {
					pair.packStrength = cluster.packStrength
				}
				pair.roleMix[workstreamArtifactRole(refsByArtifact[src])]++
				pair.roleMix[workstreamArtifactRole(refsByArtifact[dst])]++
				pair.roleFamilyMix[workstreamRoleFamily(refsByArtifact[src])]++
				pair.roleFamilyMix[workstreamRoleFamily(refsByArtifact[dst])]++
				emittedForAnchor++
			}
		}
	}
	values := make([]*workstreamPairAccumulator, 0, len(pairs))
	for _, pair := range pairs {
		values = append(values, pair)
	}
	sort.Slice(values, func(i, j int) bool {
		if workstreamPackStrengthRank(values[i].packStrength) != workstreamPackStrengthRank(values[j].packStrength) {
			return workstreamPackStrengthRank(values[i].packStrength) > workstreamPackStrengthRank(values[j].packStrength)
		}
		if values[i].confidence == values[j].confidence {
			if values[i].evidence == values[j].evidence {
				if values[i].src == values[j].src {
					return values[i].dst < values[j].dst
				}
				return values[i].src < values[j].src
			}
			return values[i].evidence > values[j].evidence
		}
		return values[i].confidence > values[j].confidence
	})
	if len(values) > maxWorkstreamEdges {
		diag.ClustersCapped += len(values) - maxWorkstreamEdges
		values = values[:maxWorkstreamEdges]
	}
	edges := make([]store.ArtifactEdgeInput, 0, len(values))
	for _, pair := range values {
		explanation := "shares workstream anchor"
		if len(pair.anchors) == 1 {
			if anchor, ok := pair.anchors[0]["anchor"].(string); ok && anchor != "" {
				explanation = fmt.Sprintf("shares workstream anchor %q", anchor)
			}
		} else if len(pair.anchors) > 1 {
			explanation = fmt.Sprintf("shares %d workstream anchors", len(pair.anchors))
		}
		edges = append(edges, store.ArtifactEdgeInput{
			ID:            stableEvidenceID("edge", repoID, pair.src, pair.dst, edgeTypeSameWorkstreamAnchor, sourceSignalWorkstreamAnchor),
			RepoID:        repoID,
			SrcArtifactID: pair.src,
			DstArtifactID: pair.dst,
			EdgeType:      edgeTypeSameWorkstreamAnchor,
			Weight:        pair.weight,
			Confidence:    pair.confidence,
			EvidenceCount: pair.evidence,
			Freshness:     pair.freshness,
			SourceSignal:  sourceSignalWorkstreamAnchor,
			Explanation:   explanation,
			MetadataJSON: evidenceJSON(map[string]any{
				"anchors":         pair.anchors,
				"pack_strength":   pair.packStrength,
				"role_mix":        pair.roleMix,
				"role_family_mix": pair.roleFamilyMix,
			}),
		})
	}
	return edges
}

func topWorkstreamClusters(clusters []workstreamAcceptedCluster, refsByArtifact map[string]gitArtifactRef) []WorkstreamClusterExample {
	return workstreamClusterExamples(clusters, maxWorkstreamTopClusters)
}

func workstreamClusterExamples(clusters []workstreamAcceptedCluster, limit int) []WorkstreamClusterExample {
	limit = minInt(limit, len(clusters))
	out := make([]WorkstreamClusterExample, 0, limit)
	for _, cluster := range clusters[:limit] {
		roleMix := map[string]int{}
		roleFamilyMix := map[string]int{}
		var examples []WorkstreamArtifactExample
		for _, artifactID := range cluster.artifactIDs {
			art := cluster.acc.artifacts[artifactID]
			if art == nil {
				continue
			}
			ref := art.ref
			roleMix[workstreamArtifactRole(ref)]++
			roleFamilyMix[workstreamRoleFamily(ref)]++
			if len(examples) < maxWorkstreamArtifactExamples {
				examples = append(examples, WorkstreamArtifactExample{
					ID:      artifactID,
					Title:   truncateGitEvidenceValue(ref.title),
					Kind:    ref.kind,
					Subtype: ref.subtype,
					Path:    truncateGitEvidenceValue(ref.path),
				})
			}
		}
		out = append(out, WorkstreamClusterExample{
			Anchor:           cluster.acc.display,
			AnchorType:       cluster.acc.primaryType(),
			Dialect:          cluster.dialect,
			PackStrength:     cluster.packStrength,
			Confidence:       roundEvidence(cluster.confidence),
			ConfidenceRule:   cluster.confidenceRule,
			ArtifactCount:    len(cluster.artifactIDs),
			EvidenceCount:    cluster.acc.evidenceCount(),
			RoleMix:          roleMix,
			RoleFamilyMix:    roleFamilyMix,
			EvidenceSources:  sortedMapKeys(cluster.acc.sources),
			ExampleArtifacts: examples,
			ExampleCommits:   cluster.acc.commits,
			Capped:           cluster.capped,
			Caveats:          cluster.caveats,
		})
	}
	return out
}

func extractFormalWorkstreamAnchors(text, source string) workstreamExtractResult {
	var out workstreamExtractResult
	if strings.TrimSpace(text) == "" {
		return out
	}
	for _, raw := range workstreamTaskIDPattern.FindAllString(text, 40) {
		canonical := strings.ToUpper(raw)
		dialect := workstreamTicketLikeDialect(canonical)
		out.anchors = append(out.anchors, workstreamAnchor{
			canonical:  canonical,
			display:    canonical,
			anchorType: "task_id",
			dialect:    dialect,
			raw:        raw,
			source:     source,
			weight:     0.96,
		})
	}
	for _, match := range workstreamRefPattern.FindAllStringSubmatchIndex(text, 40) {
		if len(match) < 8 {
			continue
		}
		raw := strings.TrimSpace(text[match[0]:match[1]])
		num := strings.TrimSpace(workstreamRegexGroup(text, match, 3))
		prefix := strings.ToLower(firstNonEmpty(workstreamRegexGroup(text, match, 1), workstreamRegexGroup(text, match, 2)))
		if prefix == "" && dateLikeNumber(num) {
			out.rejected = append(out.rejected, workstreamRejectedAnchor{anchor: raw, anchorType: "github_ref", dialect: workstreamDialectBareHashRef, reason: "date_like_number", source: source})
			continue
		}
		anchorType := "github_ref"
		canonicalPrefix := "gh"
		dialect := workstreamDialectBareHashRef
		switch prefix {
		case "pr", "pull":
			canonicalPrefix = "pr"
			dialect = workstreamDialectExplicitPRRef
		case "issue", "issues":
			canonicalPrefix = "issue"
			dialect = workstreamDialectExplicitIssueRef
		case "gh":
			dialect = workstreamDialectExplicitGHRef
		}
		canonical := canonicalPrefix + "-" + num
		context := ""
		if dialect == workstreamDialectBareHashRef && workstreamBareHashHasIssueContext(text, match[0], match[1]) {
			context = "issue_or_pr_context"
		}
		out.anchors = append(out.anchors, workstreamAnchor{
			canonical:  canonical,
			display:    canonical,
			anchorType: anchorType,
			dialect:    dialect,
			raw:        raw,
			source:     source,
			context:    context,
			weight:     0.82,
		})
	}
	out.anchors = uniqueWorkstreamAnchors(out.anchors)
	return out
}

func extractWorkstreamAnchorsFromSlugText(text, source string) workstreamExtractResult {
	out := extractFormalWorkstreamAnchors(text, source)
	slugs, rejected := slugWorkstreamAnchors(text, source)
	out.anchors = append(out.anchors, slugs...)
	out.rejected = append(out.rejected, rejected...)
	out.anchors = uniqueWorkstreamAnchors(out.anchors)
	return out
}

func extractWorkstreamAnchorsFromPath(path, source string) workstreamExtractResult {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return workstreamExtractResult{}
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	parent := filepath.Base(filepath.Dir(path))
	text := base
	if parent != "." && parent != "" && !workstreamGenericSlugTerm(parent) {
		text = parent + " " + base
	}
	out := extractWorkstreamAnchorsFromSlugText(text, source)
	for i := range out.anchors {
		switch out.anchors[i].anchorType {
		case "title_slug":
			out.anchors[i].anchorType = "path_slug"
			if out.anchors[i].dialect != workstreamDialectGenericTechnical {
				out.anchors[i].dialect = workstreamDialectPathComponentSlug
			}
		}
		out.anchors[i].source = source
	}
	return out
}

func extractWorkstreamAnchorsFromCommitMessage(message string) workstreamExtractResult {
	subject := strings.Split(strings.TrimSpace(message), "\n")[0]
	subject = stripConventionalCommitPrefix(subject)
	out := extractWorkstreamAnchorsFromSlugText(subject, "commit_message")
	for i := range out.anchors {
		if out.anchors[i].anchorType == "title_slug" {
			out.anchors[i].anchorType = "commit_slug"
			if out.anchors[i].dialect != workstreamDialectGenericTechnical {
				out.anchors[i].dialect = workstreamDialectCommitSlug
			}
		}
		out.anchors[i].source = "commit_message"
		if out.anchors[i].weight < 0.74 {
			out.anchors[i].weight = 0.74
		}
	}
	return out
}

func extractWorkstreamAnchorsFromBranch(branch string) workstreamExtractResult {
	branch = strings.TrimSpace(branch)
	if branch == "" || workstreamGenericBranch(branch) {
		return workstreamExtractResult{}
	}
	parts := strings.Split(branch, "/")
	if len(parts) > 1 && workstreamGenericBranch(parts[0]) {
		branch = strings.Join(parts[1:], " ")
	}
	out := extractWorkstreamAnchorsFromSlugText(branch, "branch")
	for i := range out.anchors {
		if out.anchors[i].anchorType == "title_slug" || out.anchors[i].anchorType == "path_slug" {
			out.anchors[i].anchorType = "branch_slug"
			if out.anchors[i].dialect != workstreamDialectGenericTechnical {
				out.anchors[i].dialect = workstreamDialectBranchSlug
			}
		}
		out.anchors[i].source = "branch"
		if out.anchors[i].weight < 0.7 {
			out.anchors[i].weight = 0.7
		}
	}
	return out
}

func slugWorkstreamAnchors(text, source string) ([]workstreamAnchor, []workstreamRejectedAnchor) {
	words := workstreamSlugWords(text)
	if len(words) == 0 {
		return nil, nil
	}
	if len(words) < 2 {
		return nil, []workstreamRejectedAnchor{{anchor: strings.Join(words, "-"), anchorType: "title_slug", reason: "too_short", source: source}}
	}
	if !workstreamWordsDistinctive(words) {
		return nil, []workstreamRejectedAnchor{{anchor: strings.Join(words, "-"), anchorType: "title_slug", reason: "generic_slug", source: source}}
	}
	var anchors []workstreamAnchor
	addSlug := func(parts []string) {
		if len(anchors) >= maxWorkstreamAnchorsPerValue {
			return
		}
		if len(parts) == 2 && !workstreamBigramDistinctive(parts) {
			return
		}
		if !workstreamWordsDistinctive(parts) {
			return
		}
		canonical := strings.Join(parts, "-")
		if canonical == "" {
			return
		}
		dialect := workstreamDialectTitleHeadingSlug
		if workstreamGenericTechnicalAnchor(canonical) {
			dialect = workstreamDialectGenericTechnical
		}
		anchors = append(anchors, workstreamAnchor{
			canonical:  canonical,
			display:    canonical,
			anchorType: "title_slug",
			dialect:    dialect,
			raw:        text,
			source:     source,
			weight:     0.7,
		})
	}
	if len(words) <= 7 {
		addSlug(words)
	} else {
		addSlug(words[:7])
	}
	if len(words) >= 4 {
		for i := 0; i+4 <= len(words); i++ {
			addSlug(words[i : i+4])
		}
	} else if len(words) == 3 {
		addSlug(words)
	}
	if len(words) >= 2 {
		for i := 0; i+2 <= len(words); i++ {
			addSlug(words[i : i+2])
		}
	}
	return uniqueWorkstreamAnchors(anchors), nil
}

func workstreamSlugWords(text string) []string {
	words := conceptWords(text)
	words = stripLeadingDateWords(words)
	var out []string
	for _, word := range words {
		if workstreamGenericSlugTerm(word) {
			continue
		}
		out = append(out, word)
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

func stripLeadingDateWords(words []string) []string {
	if len(words) >= 3 && len(words[0]) == 4 && dateLikeNumber(words[0]) && monthLikeNumber(words[1]) && dayLikeNumber(words[2]) {
		return words[3:]
	}
	return words
}

func workstreamWordsDistinctive(words []string) bool {
	meaningful := 0
	hasDigit := false
	for _, word := range words {
		if containsDigit(word) {
			hasDigit = true
		}
		if !workstreamGenericSlugTerm(word) && !allDigits(word) {
			meaningful++
		}
	}
	if meaningful >= 2 {
		return true
	}
	return meaningful >= 1 && hasDigit && len(words) >= 3
}

func workstreamBigramDistinctive(words []string) bool {
	if len(words) != 2 || !workstreamWordsDistinctive(words) {
		return false
	}
	for _, word := range words {
		if containsDigit(word) || len(word) >= 7 {
			return true
		}
	}
	return false
}

func uniqueWorkstreamAnchors(values []workstreamAnchor) []workstreamAnchor {
	seen := map[string]bool{}
	out := make([]workstreamAnchor, 0, len(values))
	for _, value := range values {
		if value.canonical == "" || seen[value.canonical] {
			continue
		}
		seen[value.canonical] = true
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].anchorType == out[j].anchorType {
			return out[i].canonical < out[j].canonical
		}
		return out[i].anchorType < out[j].anchorType
	})
	return out
}

func refsByArtifactID(artifacts []evidenceArtifact, known map[string]gitArtifactRef) map[string]gitArtifactRef {
	out := map[string]gitArtifactRef{}
	for _, artifact := range artifacts {
		if ref := known[artifact.id]; ref.id != "" {
			out[artifact.id] = ref
			continue
		}
		ref := gitArtifactRef{
			id:      artifact.id,
			kind:    artifact.kind,
			subtype: artifact.subtype,
			title:   artifact.title,
		}
		for _, src := range artifact.sources {
			if ref.path == "" && src.Path != "" {
				ref.path = normalizeGitEvidencePath(src.Path)
			}
			if ref.sourceIdentity == "" && src.SourceIdentity != "" {
				ref.sourceIdentity = src.SourceIdentity
			}
		}
		out[artifact.id] = ref
	}
	return out
}

func refsForGitFiles(files []gitfacts.FileChange, artifactsByPath map[string][]gitArtifactRef) []gitArtifactRef {
	seen := map[string]bool{}
	var out []gitArtifactRef
	for _, file := range files {
		path := normalizeGitEvidencePath(file.FilePath)
		if path == "" || gitEvidenceNoisyPath(path) {
			continue
		}
		for _, ref := range limitGitArtifactRefs(artifactsByPath[path]) {
			if ref.id == "" || seen[ref.id] {
				continue
			}
			seen[ref.id] = true
			out = append(out, ref)
		}
	}
	sortGitArtifactRefs(out)
	return out
}

func sortedWorkstreamArtifactIDs(values map[string]*workstreamArtifactAccumulator) []string {
	out := make([]string, 0, len(values))
	for id := range values {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func selectWorkstreamArtifactIDs(acc *workstreamAnchorAccumulator, artifactIDs []string, limit int) []string {
	if len(artifactIDs) <= limit {
		return artifactIDs
	}
	ordered := append([]string(nil), artifactIDs...)
	sort.Slice(ordered, func(i, j int) bool {
		left := acc.artifacts[ordered[i]]
		right := acc.artifacts[ordered[j]]
		leftScore := workstreamArtifactSelectionScore(left)
		rightScore := workstreamArtifactSelectionScore(right)
		if leftScore == rightScore {
			return ordered[i] < ordered[j]
		}
		return leftScore > rightScore
	})

	var selected []string
	seen := map[string]bool{}
	add := func(id string) {
		if id == "" || seen[id] || len(selected) >= limit {
			return
		}
		seen[id] = true
		selected = append(selected, id)
	}
	addBest := func(match func(string) bool) {
		for _, id := range ordered {
			art := acc.artifacts[id]
			if art == nil || !match(workstreamRoleFamily(art.ref)) {
				continue
			}
			add(id)
			return
		}
	}

	addBest(workstreamRoleFamilyDocLike)
	addBest(workstreamRoleFamilyImplementationLike)
	for _, id := range ordered {
		add(id)
	}
	return selected
}

func workstreamArtifactSelectionScore(art *workstreamArtifactAccumulator) float64 {
	if art == nil {
		return 0
	}
	score := art.weight + math.Min(float64(art.evidence), 8)*0.01
	family := workstreamRoleFamily(art.ref)
	if workstreamRoleFamilyDocLike(family) {
		score += 0.18
	}
	if workstreamRoleFamilyImplementationLike(family) {
		score += 0.08
	}
	return score
}

func workstreamScore(acc *workstreamAnchorAccumulator, artifactIDs []string, profile workstreamDialectProfile) (float64, float64, string, string) {
	sourceCount := len(acc.sources)
	artifactCount := len(acc.artifacts)
	anchorType := acc.primaryType()
	dialect := acc.primaryDialect()
	hasGit := acc.hasGitSource()
	roleFamilies := workstreamRoleFamilyMix(acc, artifactIDs)
	roleDiverse := len(roleFamilies) >= 2
	docBacked := workstreamHasDocBackedFamilyMix(roleFamilies)
	packStrength := workstreamPackStrength(acc, roleFamilies, profile)
	switch {
	case dialect == workstreamDialectGenericTechnical:
		if docBacked {
			return 0.48, 0.6, "low_generic_technical_cross_role", packStrength
		}
		return 0.36, 0.5, "low_generic_technical_term", packStrength
	case dialect == workstreamDialectBareHashRef && docBacked && acc.hasContext("issue_or_pr_context") && artifactCount >= 2:
		return 0.52, 0.64, "support_bare_hash_with_issue_context", packStrength
	case dialect == workstreamDialectBareHashRef:
		return 0.36, 0.5, "low_bare_hash_ref", packStrength
	case dialect == workstreamDialectDocumentNumberRef && docBacked:
		return 0.58, 0.66, "support_document_number_ref", packStrength
	case dialect == workstreamDialectDocumentNumberRef:
		return 0.42, 0.54, "low_document_number_ref", packStrength
	case dialect == workstreamDialectTicketLikeUpper && hasGit && artifactCount >= 2 && roleDiverse:
		return 0.88, 0.93, "high_task_id_role_diverse_git", packStrength
	case dialect == workstreamDialectTicketLikeUpper && hasGit && artifactCount >= 2:
		return 0.82, 0.9, "high_task_id_with_git_and_artifacts", packStrength
	case dialect == workstreamDialectTicketLikeUpper && artifactCount >= 2:
		return 0.78, 0.86, "high_task_id_across_artifacts", packStrength
	case workstreamExplicitWorkRefDialect(dialect) && hasGit && artifactCount >= 2 && roleDiverse:
		return 0.74, 0.82, "medium_explicit_work_ref_role_diverse_git", packStrength
	case workstreamExplicitWorkRefDialect(dialect) && hasGit && artifactCount >= 2:
		return 0.66, 0.76, "medium_explicit_work_ref_with_git", packStrength
	case dialect == workstreamDialectOpenSpecChangeSlug && docBacked && artifactCount >= 2:
		return 0.82, 0.9, "high_openspec_change_cross_role", packStrength
	case docBacked && hasGit && sourceCount >= 3 && artifactCount >= 2:
		return 0.72, 0.8, "medium_slug_docbacked_git", packStrength
	case docBacked && sourceCount >= 3 && artifactCount >= 2:
		return 0.64, 0.72, "medium_slug_docbacked", packStrength
	case roleDiverse && hasGit && sourceCount >= 3 && artifactCount >= 2:
		return 0.56, 0.66, "support_slug_source_test_git", packStrength
	case hasGit && sourceCount >= 3 && artifactCount >= 2:
		return 0.5, 0.62, "low_slug_single_family_git", packStrength
	case sourceCount >= 3 && artifactCount >= 2:
		return 0.46, 0.58, "low_slug_source_mix", packStrength
	default:
		_ = anchorType
		return 0.42, 0.54, "low_bounded_anchor", packStrength
	}
}

func workstreamPackStrength(acc *workstreamAnchorAccumulator, roleFamilies map[string]int, profile workstreamDialectProfile) string {
	dialect := acc.primaryDialect()
	docBacked := workstreamHasDocBackedFamilyMix(roleFamilies)
	trust := profile.trust[dialect]
	switch dialect {
	case workstreamDialectBareHashRef:
		if docBacked && acc.hasContext("issue_or_pr_context") && trust != "" && trust != workstreamTrustWeak {
			return workstreamPackStrengthSupportCross
		}
		if acc.formalAnchor() || len(roleFamilies) >= 2 {
			return workstreamPackStrengthSupportLocal
		}
		return workstreamPackStrengthWeak
	case workstreamDialectGenericTechnical:
		if docBacked {
			return workstreamPackStrengthSupportCross
		}
		if len(roleFamilies) >= 2 {
			return workstreamPackStrengthSupportLocal
		}
		return workstreamPackStrengthWeak
	case workstreamDialectDocumentNumberRef:
		if docBacked {
			return workstreamPackStrengthSupportCross
		}
		if acc.formalAnchor() || len(roleFamilies) >= 2 {
			return workstreamPackStrengthSupportLocal
		}
		return workstreamPackStrengthWeak
	case workstreamDialectBranchSlug:
		if docBacked && acc.nativeArtifactSourceCount() > 0 && (acc.sources["commit_message"] || acc.sources["artifact_title"] || acc.sources["artifact_path"] || acc.sources["metadata"]) {
			return workstreamPackStrengthSupportCross
		}
		if len(roleFamilies) >= 2 {
			return workstreamPackStrengthSupportLocal
		}
		return workstreamPackStrengthWeak
	case workstreamDialectCommitSlug:
		if docBacked && acc.nativeArtifactSourceCount() > 0 {
			return workstreamPackStrengthSupportCross
		}
		if len(roleFamilies) >= 2 {
			return workstreamPackStrengthSupportLocal
		}
		return workstreamPackStrengthWeak
	case workstreamDialectExplicitPRRef, workstreamDialectExplicitIssueRef, workstreamDialectExplicitGHRef:
		if docBacked && trust == workstreamTrustStrong && acc.hasGitSource() {
			return workstreamPackStrengthStrong
		}
		if docBacked {
			return workstreamPackStrengthSupportCross
		}
		if acc.formalAnchor() || len(roleFamilies) >= 2 {
			return workstreamPackStrengthSupportLocal
		}
		return workstreamPackStrengthWeak
	case workstreamDialectOpenSpecChangeSlug, workstreamDialectTicketLikeUpper:
		if docBacked {
			return workstreamPackStrengthStrong
		}
		if acc.formalAnchor() || len(roleFamilies) >= 2 {
			return workstreamPackStrengthSupportLocal
		}
		return workstreamPackStrengthWeak
	case workstreamDialectTitleHeadingSlug:
		if docBacked && len(acc.sources) >= 3 && (acc.sources["artifact_path"] || acc.sources["metadata"] || acc.hasGitSource()) && len(acc.artifacts) >= 2 {
			return workstreamPackStrengthStrong
		}
		if docBacked && roleFamilies["source"] > 0 && (len(acc.sources) >= 2 || acc.sources["heading"] || acc.sources["body"] || acc.sources["artifact_path"] || acc.hasGitSource()) {
			return workstreamPackStrengthSupportCross
		}
		if len(roleFamilies) >= 2 {
			return workstreamPackStrengthSupportLocal
		}
		return workstreamPackStrengthWeak
	case workstreamDialectPathComponentSlug:
		if docBacked && roleFamilies["source"] > 0 && len(acc.sources) >= 3 && (acc.sources["artifact_path"] || acc.sources["metadata"] || acc.hasGitSource()) && len(acc.artifacts) >= 2 && (trust == workstreamTrustStrong || acc.evidenceCount() >= 8 || len(acc.artifacts) >= 4) {
			return workstreamPackStrengthStrong
		}
		if docBacked {
			return workstreamPackStrengthSupportCross
		}
		if len(roleFamilies) >= 2 {
			return workstreamPackStrengthSupportLocal
		}
		return workstreamPackStrengthWeak
	}
	if acc.formalAnchor() && docBacked {
		return workstreamPackStrengthStrong
	}
	if docBacked && len(acc.sources) >= 3 && len(acc.artifacts) >= 2 {
		return workstreamPackStrengthStrong
	}
	if docBacked {
		return workstreamPackStrengthSupportCross
	}
	if acc.formalAnchor() || len(roleFamilies) >= 2 {
		return workstreamPackStrengthSupportLocal
	}
	return workstreamPackStrengthWeak
}

func workstreamRoleFamilyMix(acc *workstreamAnchorAccumulator, artifactIDs []string) map[string]int {
	out := map[string]int{}
	for _, artifactID := range artifactIDs {
		art := acc.artifacts[artifactID]
		if art == nil {
			continue
		}
		out[workstreamRoleFamily(art.ref)]++
	}
	return out
}

func workstreamHasDocBackedFamilyMix(roleFamilies map[string]int) bool {
	hasDocLike := false
	hasImplementationLike := false
	for family := range roleFamilies {
		if workstreamRoleFamilyDocLike(family) {
			hasDocLike = true
		}
		if workstreamRoleFamilyImplementationLike(family) {
			hasImplementationLike = true
		}
	}
	return hasDocLike && hasImplementationLike
}

func workstreamRoleFamilyDocLike(family string) bool {
	switch family {
	case "adr", "design", "doc", "intent", "model", "plan", "requirements", "spec", "trace":
		return true
	default:
		return false
	}
}

func workstreamRoleFamilyImplementationLike(family string) bool {
	switch family {
	case "config", "source", "test":
		return true
	default:
		return false
	}
}

func workstreamPackStrengthRank(value string) int {
	switch value {
	case workstreamPackStrengthStrong:
		return 4
	case workstreamPackStrengthSupportCross:
		return 3
	case workstreamPackStrengthSupportLocal:
		return 2
	case workstreamPackStrengthWeak:
		return 1
	default:
		return 0
	}
}

func maxWorkstreamEdgesForCluster(packStrength string) int {
	switch packStrength {
	case workstreamPackStrengthStrong:
		return maxWorkstreamEdgesPerAnchor
	case workstreamPackStrengthSupportCross:
		return maxInt(1, maxWorkstreamEdgesPerAnchor*2/3)
	default:
		return 0
	}
}

func (acc *workstreamAnchorAccumulator) gitOnlySupport() bool {
	artifactSources := 0
	for _, art := range acc.artifacts {
		for source := range art.sources {
			if workstreamNativeArtifactSource(source) {
				artifactSources++
			}
		}
	}
	return artifactSources == 0 && len(acc.artifacts) > 0
}

func (acc *workstreamAnchorAccumulator) formalAnchor() bool {
	switch acc.primaryDialect() {
	case workstreamDialectTicketLikeUpper, workstreamDialectDocumentNumberRef, workstreamDialectExplicitPRRef, workstreamDialectExplicitIssueRef, workstreamDialectExplicitGHRef, workstreamDialectBareHashRef:
		return true
	default:
		return false
	}
}

func (acc *workstreamAnchorAccumulator) hasContext(value string) bool {
	return acc != nil && acc.contexts[value]
}

func (acc *workstreamAnchorAccumulator) nativeArtifactSourceCount() int {
	count := 0
	for _, art := range acc.artifacts {
		for source := range art.sources {
			if workstreamNativeArtifactSource(source) {
				count++
			}
		}
	}
	return count
}

func workstreamNativeArtifactSource(source string) bool {
	switch source {
	case "artifact_path", "artifact_title", "heading", "body", "metadata":
		return true
	default:
		return false
	}
}

func (acc *workstreamAnchorAccumulator) primaryType() string {
	priority := []string{"task_id", "github_ref", "change_slug", "branch_slug", "path_slug", "commit_slug", "title_slug"}
	for _, value := range priority {
		if acc.types[value] {
			return value
		}
	}
	for value := range acc.types {
		return value
	}
	return ""
}

func (acc *workstreamAnchorAccumulator) primaryDialect() string {
	priority := []string{
		workstreamDialectDocumentNumberRef,
		workstreamDialectTicketLikeUpper,
		workstreamDialectExplicitPRRef,
		workstreamDialectExplicitIssueRef,
		workstreamDialectExplicitGHRef,
		workstreamDialectBareHashRef,
		workstreamDialectOpenSpecChangeSlug,
		workstreamDialectBranchSlug,
		workstreamDialectPathComponentSlug,
		workstreamDialectCommitSlug,
		workstreamDialectTitleHeadingSlug,
		workstreamDialectGenericTechnical,
	}
	for _, value := range priority {
		if acc.dialects[value] {
			return value
		}
	}
	for value := range acc.dialects {
		return value
	}
	return workstreamDialectForAnchorType(acc.primaryType())
}

func workstreamDialectForAnchorType(anchorType string) string {
	switch anchorType {
	case "task_id":
		return workstreamDialectTicketLikeUpper
	case "github_ref":
		return workstreamDialectBareHashRef
	case "change_slug":
		return workstreamDialectOpenSpecChangeSlug
	case "branch_slug":
		return workstreamDialectBranchSlug
	case "commit_slug":
		return workstreamDialectCommitSlug
	case "path_slug":
		return workstreamDialectPathComponentSlug
	case "title_slug":
		return workstreamDialectTitleHeadingSlug
	default:
		return workstreamDialectUnknown
	}
}

func workstreamExplicitWorkRefDialect(dialect string) bool {
	switch dialect {
	case workstreamDialectExplicitPRRef, workstreamDialectExplicitIssueRef, workstreamDialectExplicitGHRef:
		return true
	default:
		return false
	}
}

func workstreamTicketLikeDialect(canonical string) string {
	prefix, _, ok := strings.Cut(strings.ToUpper(canonical), "-")
	if !ok || prefix == "" {
		return workstreamDialectTicketLikeUpper
	}
	switch prefix {
	case "ADR", "RFC", "PEP":
		return workstreamDialectDocumentNumberRef
	case "GPT", "IGRF", "HTTP", "TLS", "SHA", "MD", "CRC", "UTF":
		return workstreamDialectGenericTechnical
	default:
		return workstreamDialectTicketLikeUpper
	}
}

func (acc *workstreamAnchorAccumulator) hasGitSource() bool {
	for _, source := range []string{"commit_message", "commit_message_changed_file", "git_changed_file", "branch"} {
		if acc.sources[source] {
			return true
		}
	}
	return false
}

func (acc *workstreamAnchorAccumulator) evidenceCount() int {
	count := len(acc.sources) + len(acc.commits) + len(acc.files)
	for _, art := range acc.artifacts {
		count += art.evidence
	}
	if count <= 0 {
		return 1
	}
	return count
}

func workstreamArtifactRole(ref gitArtifactRef) string {
	if ref.kind == "" {
		return "unknown"
	}
	if ref.subtype != "" {
		return ref.kind + ":" + ref.subtype
	}
	return ref.kind
}

func workstreamRoleFamily(ref gitArtifactRef) string {
	kind := strings.ToLower(strings.TrimSpace(ref.kind))
	subtype := strings.ToLower(strings.TrimSpace(ref.subtype))
	path := strings.ToLower(strings.TrimSpace(ref.path))
	switch {
	case subtype == "test_case" || strings.Contains(path, "_test."):
		return "test"
	case kind == "source_context":
		return "source"
	case subtype == "agent_instruction" || subtype == "skill" || strings.Contains(path, "/skills/") || strings.HasSuffix(path, "/skill.md"):
		return "protocol"
	case subtype == "schema_model" || strings.Contains(subtype, "model"):
		return "model"
	case subtype == "template" || strings.Contains(path, "template"):
		return "template"
	case subtype == "trace":
		return "trace"
	case subtype == "plan":
		return "plan"
	case kind == "config" || strings.Contains(path, ".github/workflows/") || strings.HasSuffix(path, "makefile"):
		return "config"
	case kind == "markdown_artifact" || kind == "intent":
		return "doc"
	case kind != "":
		return kind
	default:
		return "unknown"
	}
}

func stripConventionalCommitPrefix(subject string) string {
	subject = strings.TrimSpace(subject)
	if idx := strings.Index(subject, ":"); idx > 0 && idx <= 32 {
		prefix := subject[:idx]
		ok := true
		for _, r := range prefix {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '(' || r == ')' || r == '-' || r == '_' || r == '!' {
				continue
			}
			ok = false
			break
		}
		if ok {
			return strings.TrimSpace(subject[idx+1:])
		}
	}
	return subject
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func workstreamRegexGroup(text string, indexes []int, group int) string {
	pos := group * 2
	if pos+1 >= len(indexes) || indexes[pos] < 0 || indexes[pos+1] < 0 {
		return ""
	}
	return text[indexes[pos]:indexes[pos+1]]
}

func workstreamBareHashHasIssueContext(text string, start, end int) bool {
	left := maxInt(0, start-48)
	right := minInt(len(text), end+48)
	window := strings.ToLower(text[left:right])
	for _, marker := range []string{"fixes", "fix", "fixed", "closes", "close", "closed", "resolves", "resolve", "resolved", "issue", "issues", "pull", "pr", "github", "gh-"} {
		if strings.Contains(window, marker) {
			return true
		}
	}
	return false
}

func workstreamGenericTechnicalAnchor(value string) bool {
	value = strings.Trim(strings.ToLower(value), "._-/ ")
	switch value {
	case "sha-1", "sha-224", "sha-256", "sha-384", "sha-512", "md5", "crc32", "utf-8", "http-1", "http-2", "http-3":
		return true
	default:
		return false
	}
}

func copyIntMap(values map[string]int) map[string]int {
	out := make(map[string]int, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func dateLikeNumber(value string) bool {
	n, err := strconv.Atoi(value)
	return err == nil && n >= 1900 && n <= 2100
}

func monthLikeNumber(value string) bool {
	n, err := strconv.Atoi(value)
	return err == nil && n >= 1 && n <= 12
}

func dayLikeNumber(value string) bool {
	n, err := strconv.Atoi(value)
	return err == nil && n >= 1 && n <= 31
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func workstreamGenericBranch(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "", "main", "master", "dev", "develop", "development", "test", "tests", "release", "wip", "head":
		return true
	default:
		return false
	}
}

func workstreamGenericSlugTerm(value string) bool {
	value = strings.Trim(strings.ToLower(value), "._-/ ")
	if value == "" {
		return true
	}
	if evidenceStopWords[value] || genericEdgeTerms[value] {
		return true
	}
	switch value {
	case "add", "added", "adds", "change", "changes", "chore", "cmd", "codex", "docs", "feat", "fix", "fixed", "index", "init", "main", "new", "pkg", "plan", "readme", "refactor", "repo", "scan", "src", "test", "tests", "update", "updated", "updates", "worktree":
		return true
	default:
		return false
	}
}
