package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/devspecs-com/devspecs-cli/internal/store"
)

const (
	conceptKindTerm              = "term"
	conceptKindIdentifier        = "identifier"
	conceptKindCompactIdentifier = "compact_identifier"
	conceptKindPhrase            = "phrase"
	conceptKindPathFragment      = "path_fragment"
	conceptKindSymbol            = "symbol"
	conceptKindTestBehavior      = "test_behavior"
	conceptKindArtifactRole      = "artifact_role"

	edgeTypeMentionsSameConcept   = "mentions_same_concept"
	edgeTypeExplicitReference     = "explicit_reference"
	edgeTypeSameFileOrLineVariant = "same_file_or_line_variant"
	edgeTypeSameLayoutGroup       = "same_layout_group"
	edgeTypeOpenSpecCompanion     = "openspec_companion"
	edgeTypeTestsSource           = "tests_source"
	edgeTypeMentionsSymbol        = "mentions_symbol"

	maxConceptForms                    = 6
	maxMentionEvidenceFormLength       = 120
	maxEvidenceMentionsPerArtifact     = 96
	minEvidenceMentionsPerArtifact     = 8
	maxEvidenceMentionsPerRepo         = 60000
	maxSymbolConceptsPerArtifact       = 12
	maxSharedConceptArtifacts          = 6
	maxSharedConceptEdges              = 1500
	maxSharedConceptEdgesPerArtifact   = 48
	maxSharedConceptsPerPair           = 5
	maxSharedConceptMetadataPerEdge    = 3
	maxPathReferenceEdgesPerTarget     = 32
	maxPathReferenceEdgesPerSource     = 4
	maxSameSourcePathEdgesPerPath      = 24
	maxLayoutGroupEdgesPerGroup        = 24
	maxTestSourceEdges                 = 3000
	maxTestSourceEdgesPerTest          = 5
	maxTestSourceCandidatesPerKey      = 16
	maxTriangulationSymbolsPerSource   = 40
	maxRichSymbolSources               = 4
	maxRichSymbolReferenceEdges        = 1200
	maxRichSymbolReferencePerSource    = 24
	maxRichSymbolReferencePerRef       = 8
	maxRichSymbolReferencesPerArtifact = 32
)

// EvidenceGraphDiagnostics is a scan-time summary of persisted graph evidence.
type EvidenceGraphDiagnostics struct {
	RichTypedIndex       bool                     `json:"rich_typed_index,omitempty"`
	ConceptsIndexed      int                      `json:"concepts_indexed"`
	MentionsIndexed      int                      `json:"mentions_indexed"`
	EdgesIndexed         int                      `json:"edges_indexed"`
	ConceptsByKind       map[string]int           `json:"concepts_by_kind,omitempty"`
	MentionsByField      map[string]int           `json:"mentions_by_field,omitempty"`
	EdgesByType          map[string]int           `json:"edges_by_type,omitempty"`
	NoisyConceptsSkipped int                      `json:"noisy_concepts_skipped,omitempty"`
	TopNoisyConcepts     []EvidenceConceptExample `json:"top_noisy_concepts,omitempty"`
	TopEdges             []EvidenceEdgeExample    `json:"top_edges,omitempty"`
	PhaseMS              map[string]int64         `json:"phase_ms,omitempty"`
}

// EvidenceConceptExample explains one concept skipped from edge materialization.
type EvidenceConceptExample struct {
	Kind              string `json:"kind"`
	Canonical         string `json:"canonical"`
	DocumentFrequency int    `json:"document_frequency"`
	Reason            string `json:"reason"`
}

// EvidenceEdgeExample gives a compact receipt-like example for diagnostics.
type EvidenceEdgeExample struct {
	EdgeType      string  `json:"edge_type"`
	Source        string  `json:"source"`
	Target        string  `json:"target"`
	SourceSignal  string  `json:"source_signal"`
	Explanation   string  `json:"explanation"`
	Confidence    float64 `json:"confidence"`
	EvidenceCount int     `json:"evidence_count"`
}

type evidenceBuildResult struct {
	concepts    []store.ConceptInput
	mentions    []store.ConceptMentionInput
	edges       []store.ArtifactEdgeInput
	diagnostics *EvidenceGraphDiagnostics
}

type evidenceGraphBuildOptions struct {
	RichTypedIndex bool
	PhaseTiming    bool
}

type evidenceArtifact struct {
	id            string
	repoID        string
	kind          string
	subtype       string
	title         string
	status        string
	revisionID    string
	body          string
	extractedJSON string
	extracted     map[string]any
	sources       []store.SourceRow
	sections      []store.SectionRow
	links         []store.LinkRow
}

type rawConceptMention struct {
	kind       string
	canonical  string
	form       string
	artifactID string
	sectionID  string
	field      string
	weight     float64
	source     string
}

type conceptAccumulator struct {
	input       store.ConceptInput
	forms       map[string]bool
	artifactIDs map[string]bool
}

type pairEvidence struct {
	src      string
	dst      string
	concepts []edgeConceptEvidence
}

type edgeConceptEvidence struct {
	kind      string
	canonical string
	idf       float64
	strong    bool
}

type edgeAccumulator struct {
	input store.ArtifactEdgeInput
}

type pathReferenceEdge struct {
	src    string
	dst    string
	target string
}

var pathReferencePattern = regexp.MustCompile(`(?i)([A-Za-z0-9_.@+-]+(?:/[A-Za-z0-9_.@+-]+)+\.(?:md|markdown|txt|adoc|rst|go|py|rs|java|kt|ts|tsx|js|jsx|vue|sql|toml|yaml|yml|json|dockerfile))`)
var symbolReferencePattern = regexp.MustCompile(`[A-Za-z_$][A-Za-z0-9_$]*(?:[._-][A-Za-z0-9_$]+)*`)
var testImportPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)\bfrom\s+([A-Za-z0-9_./:-]+)\s+import\b`),
	regexp.MustCompile(`(?m)\bimport\s+([A-Za-z0-9_./:-]+)(?:\s+as\s+[A-Za-z_][A-Za-z0-9_]*)?$`),
	regexp.MustCompile(`(?m)\bimport(?:\s+type)?(?:[^'"` + "`" + `\n]+?\s+from\s*)?['"]([^'"` + "`" + `]+)['"]`),
	regexp.MustCompile(`(?m)\brequire\(\s*['"]([^'"]+)['"]\s*\)`),
	regexp.MustCompile(`(?m)\buse\s+([A-Za-z0-9_:]+)`),
}
var sourceSymbolPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)\bfunc\s+(?:\([^)]+\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	regexp.MustCompile(`(?m)\btype\s+([A-Za-z_][A-Za-z0-9_]*)\b`),
	regexp.MustCompile(`(?m)\b(?:def|class|fn|struct|enum|trait|interface)\s+([A-Za-z_][A-Za-z0-9_]*)\b`),
	regexp.MustCompile(`(?m)\b(?:function|class|interface|type)\s+([A-Za-z_$][A-Za-z0-9_$]*)\b`),
	regexp.MustCompile(`(?m)\b(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\b`),
}

func (s *Scanner) rebuildEvidenceGraph(repoID, now string, opts evidenceGraphBuildOptions) (*EvidenceGraphDiagnostics, error) {
	phaseMS := map[string]int64{}
	recordPhase := func(name string, started time.Time) {
		if opts.PhaseTiming {
			phaseMS[name] = time.Since(started).Milliseconds()
		}
	}
	phaseStarted := time.Now()
	artifacts, err := s.loadEvidenceArtifacts(repoID)
	recordPhase("load_artifacts", phaseStarted)
	if err != nil {
		return nil, err
	}
	built := buildEvidenceGraphWithOptions(repoID, artifacts, opts)
	mergeEvidencePhaseMS(phaseMS, built.diagnostics)
	phaseStarted = time.Now()
	persistPhaseMS, err := s.db.ReplaceRepoEvidenceWithPhaseTiming(repoID, built.concepts, built.mentions, built.edges, now)
	recordPhase("persist_total", phaseStarted)
	if err != nil {
		return nil, err
	}
	if opts.PhaseTiming {
		mergePhaseMS(phaseMS, persistPhaseMS)
		if built.diagnostics.PhaseMS == nil {
			built.diagnostics.PhaseMS = map[string]int64{}
		}
		mergePhaseMS(built.diagnostics.PhaseMS, phaseMS)
	}
	return built.diagnostics, nil
}

func (s *Scanner) loadEvidenceArtifacts(repoID string) ([]evidenceArtifact, error) {
	rows, err := s.db.Query(
		`SELECT a.id, a.repo_id, a.kind, COALESCE(a.subtype,''), a.title, a.status,
			COALESCE(a.current_revision_id,''), COALESCE(rv.body,''), COALESCE(rv.extracted_json,'')
		 FROM artifacts a
		 LEFT JOIN artifact_revisions rv ON rv.id = a.current_revision_id
		 WHERE a.repo_id = ?
		 ORDER BY a.id`,
		repoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var artifacts []evidenceArtifact
	for rows.Next() {
		var a evidenceArtifact
		if err := rows.Scan(&a.id, &a.repoID, &a.kind, &a.subtype, &a.title, &a.status, &a.revisionID, &a.body, &a.extractedJSON); err != nil {
			return nil, err
		}
		a.extracted = decodeEvidenceJSON(a.extractedJSON)
		artifacts = append(artifacts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(artifacts))
	for _, a := range artifacts {
		ids = append(ids, a.id)
	}
	sourcesByArtifact, err := s.db.GetSourcesForArtifacts(ids)
	if err != nil {
		return nil, err
	}
	sectionsByArtifact, err := s.db.GetSectionsForArtifacts(ids)
	if err != nil {
		return nil, err
	}
	linksByArtifact, err := s.db.GetLinksForArtifacts(ids)
	if err != nil {
		return nil, err
	}
	for i := range artifacts {
		artifacts[i].sources = sourcesByArtifact[artifacts[i].id]
		artifacts[i].sections = sectionsByArtifact[artifacts[i].id]
		artifacts[i].links = linksByArtifact[artifacts[i].id]
	}
	return artifacts, nil
}

func buildEvidenceGraph(repoID string, artifacts []evidenceArtifact) evidenceBuildResult {
	return buildEvidenceGraphWithOptions(repoID, artifacts, evidenceGraphBuildOptions{})
}

func buildEvidenceGraphWithOptions(repoID string, artifacts []evidenceArtifact, opts evidenceGraphBuildOptions) evidenceBuildResult {
	phaseMS := map[string]int64{}
	recordPhase := func(name string, started time.Time) {
		if opts.PhaseTiming {
			phaseMS[name] = time.Since(started).Milliseconds()
		}
	}
	artifactCount := len(artifacts)
	phaseStarted := time.Now()
	rawMentions := make([]rawConceptMention, 0, artifactCount*12)
	for _, artifact := range artifacts {
		rawMentions = append(rawMentions, limitArtifactEvidenceMentions(extractEvidenceMentionsWithOptions(artifact, opts))...)
	}
	rawMentions = limitRepoEvidenceMentions(rawMentions)
	recordPhase("derive_mentions", phaseStarted)

	phaseStarted = time.Now()
	conceptsByKey := map[string]*conceptAccumulator{}
	mentionSeen := map[string]bool{}
	for _, mention := range rawMentions {
		if mention.kind == "" || mention.canonical == "" || mention.artifactID == "" {
			continue
		}
		if !persistEvidenceMention(mention) {
			continue
		}
		key := conceptKey(mention.kind, mention.canonical)
		acc := conceptsByKey[key]
		if acc == nil {
			acc = &conceptAccumulator{
				input: store.ConceptInput{
					ID:        stableEvidenceID("concept", repoID, mention.kind, mention.canonical),
					RepoID:    repoID,
					Canonical: mention.canonical,
					Kind:      mention.kind,
				},
				forms:       map[string]bool{},
				artifactIDs: map[string]bool{},
			}
			conceptsByKey[key] = acc
		}
		if mention.form != "" {
			acc.forms[mention.form] = true
		}
		acc.artifactIDs[mention.artifactID] = true
	}

	conceptInputs := make([]store.ConceptInput, 0, len(conceptsByKey))
	conceptByKey := map[string]store.ConceptInput{}
	totalDocs := float64(maxInt(1, artifactCount))
	for key, acc := range conceptsByKey {
		acc.input.Forms = limitedSortedMapKeys(acc.forms, maxConceptForms)
		acc.input.DocumentFrequency = len(acc.artifactIDs)
		acc.input.InverseDocumentFrequency = math.Log((1.0+totalDocs)/(1.0+float64(acc.input.DocumentFrequency))) + 1.0
		conceptInputs = append(conceptInputs, acc.input)
		conceptByKey[key] = acc.input
	}
	sort.Slice(conceptInputs, func(i, j int) bool {
		if conceptInputs[i].Kind == conceptInputs[j].Kind {
			return conceptInputs[i].Canonical < conceptInputs[j].Canonical
		}
		return conceptInputs[i].Kind < conceptInputs[j].Kind
	})
	recordPhase("build_concepts", phaseStarted)

	phaseStarted = time.Now()
	mentionInputs := make([]store.ConceptMentionInput, 0, len(rawMentions))
	for _, mention := range rawMentions {
		if !persistEvidenceMention(mention) {
			continue
		}
		concept, ok := conceptByKey[conceptKey(mention.kind, mention.canonical)]
		if !ok {
			continue
		}
		id := stableEvidenceID("mention", concept.ID, mention.artifactID, mention.sectionID, mention.field, mention.form, mention.source)
		if mentionSeen[id] {
			continue
		}
		mentionSeen[id] = true
		mentionInputs = append(mentionInputs, store.ConceptMentionInput{
			ID:           id,
			ConceptID:    concept.ID,
			ArtifactID:   mention.artifactID,
			SectionID:    mention.sectionID,
			Field:        mention.field,
			Weight:       mention.weight,
			EvidenceJSON: evidenceJSON(compactMentionEvidence(mention)),
		})
	}
	sort.Slice(mentionInputs, func(i, j int) bool {
		if mentionInputs[i].ArtifactID == mentionInputs[j].ArtifactID {
			if mentionInputs[i].Field == mentionInputs[j].Field {
				return mentionInputs[i].ID < mentionInputs[j].ID
			}
			return mentionInputs[i].Field < mentionInputs[j].Field
		}
		return mentionInputs[i].ArtifactID < mentionInputs[j].ArtifactID
	})
	recordPhase("build_mentions", phaseStarted)

	phaseStarted = time.Now()
	edgeBuilder := newEvidenceEdgeBuilder(repoID)
	noisy := materializeSharedConceptEdges(artifacts, conceptsByKey, rawMentions, edgeBuilder)
	materializeLayoutGroupEdges(artifacts, edgeBuilder)
	materializeSameSourcePathEdges(artifacts, edgeBuilder)
	materializeTestSourceTriangulationEdges(artifacts, edgeBuilder, opts)
	if opts.RichTypedIndex {
		materializeRichSymbolReferenceEdges(artifacts, edgeBuilder)
	}
	materializeLinkEdges(artifacts, edgeBuilder)
	materializePathReferenceEdges(artifacts, edgeBuilder)
	edges := edgeBuilder.edges()
	recordPhase("build_edges", phaseStarted)

	phaseStarted = time.Now()
	diagnostics := buildEvidenceDiagnostics(conceptInputs, mentionInputs, edges, noisy, opts)
	recordPhase("build_diagnostics", phaseStarted)
	if opts.PhaseTiming {
		diagnostics.PhaseMS = phaseMS
	}
	return evidenceBuildResult{
		concepts:    conceptInputs,
		mentions:    mentionInputs,
		edges:       edges,
		diagnostics: diagnostics,
	}
}

func mergeEvidencePhaseMS(out map[string]int64, diagnostics *EvidenceGraphDiagnostics) {
	if diagnostics == nil {
		return
	}
	mergePhaseMS(out, diagnostics.PhaseMS)
}

func mergePhaseMS(out map[string]int64, values map[string]int64) {
	if out == nil {
		return
	}
	for key, value := range values {
		out[key] = value
	}
}

func extractEvidenceMentionsWithOptions(artifact evidenceArtifact, opts evidenceGraphBuildOptions) []rawConceptMention {
	var mentions []rawConceptMention
	add := func(kind, canonical, form, field, source, sectionID string, weight float64) {
		canonical = strings.TrimSpace(canonical)
		form = strings.TrimSpace(form)
		if kind == "" || canonical == "" {
			return
		}
		mentions = append(mentions, rawConceptMention{
			kind:       kind,
			canonical:  canonical,
			form:       form,
			artifactID: artifact.id,
			sectionID:  sectionID,
			field:      field,
			weight:     weight,
			source:     source,
		})
	}
	addRole := func(value, field string) {
		value = normalizeRoleValue(value)
		if value != "" {
			add(conceptKindArtifactRole, field+":"+value, value, field, "artifact_metadata", "", 0.45)
		}
	}
	addRole(artifact.kind, "kind")
	addRole(artifact.subtype, "subtype")
	addRole(artifact.status, "status")

	for _, src := range artifact.sources {
		if src.SourceType != "" {
			addRole(src.SourceType, "source_type")
		}
		if src.FormatProfile != "" {
			addRole(src.FormatProfile, "format_profile")
		}
		for _, part := range pathConceptParts(src.Path) {
			add(conceptKindPathFragment, part, part, "path", "source_path", "", 0.85)
		}
		for _, compact := range pathCompactConcepts(src.Path) {
			add(conceptKindCompactIdentifier, compact, compact, "path", "source_path", "", 0.9)
		}
	}
	for _, phrase := range phraseConcepts(artifact.title) {
		add(conceptKindPhrase, phrase, artifact.title, "title", "artifact_title", "", 0.8)
	}
	for _, compact := range compactConcepts(artifact.title) {
		add(conceptKindCompactIdentifier, compact, compact, "title", "artifact_title", "", 0.85)
	}

	addMetadataMentions(artifact, add)
	if opts.RichTypedIndex && isEvidenceSourceArtifact(artifact) {
		addRichSourceSymbolMentions(artifact, add)
	}
	for _, section := range artifact.sections {
		addSectionMentions(section, add)
	}
	return mentions
}

func addRichSourceSymbolMentions(artifact evidenceArtifact, add func(kind, canonical, form, field, source, sectionID string, weight float64)) {
	symbols := extractSourceTriangulationSymbols(artifact)
	for _, compact := range sortedStringValueMapKeys(symbols) {
		form := firstNonEmpty(symbols[compact], compact)
		add(conceptKindSymbol, compact, form, "symbol", "source_symbol_definition", "", 1.0)
	}
}

func limitArtifactEvidenceMentions(mentions []rawConceptMention) []rawConceptMention {
	if len(mentions) <= maxEvidenceMentionsPerArtifact {
		return mentions
	}
	out := append([]rawConceptMention(nil), mentions...)
	sortEvidenceMentions(out)
	out = out[:maxEvidenceMentionsPerArtifact]
	return out
}

func limitRepoEvidenceMentions(mentions []rawConceptMention) []rawConceptMention {
	if len(mentions) <= maxEvidenceMentionsPerRepo {
		return mentions
	}
	byArtifact := map[string][]rawConceptMention{}
	for _, mention := range mentions {
		byArtifact[mention.artifactID] = append(byArtifact[mention.artifactID], mention)
	}
	artifactIDs := sortedMentionArtifactIDs(byArtifact)
	selected := make([]rawConceptMention, 0, maxEvidenceMentionsPerRepo)
	var extras []rawConceptMention
	for _, artifactID := range artifactIDs {
		group := byArtifact[artifactID]
		sortEvidenceMentions(group)
		keep := minInt(minEvidenceMentionsPerArtifact, len(group))
		selected = append(selected, group[:keep]...)
		extras = append(extras, group[keep:]...)
	}
	if len(selected) >= maxEvidenceMentionsPerRepo {
		sortEvidenceMentions(selected)
		return selected[:maxEvidenceMentionsPerRepo]
	}
	sortEvidenceMentions(extras)
	remaining := maxEvidenceMentionsPerRepo - len(selected)
	if len(extras) > remaining {
		extras = extras[:remaining]
	}
	selected = append(selected, extras...)
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].artifactID == selected[j].artifactID {
			if selected[i].field == selected[j].field {
				if selected[i].canonical == selected[j].canonical {
					return selected[i].sectionID < selected[j].sectionID
				}
				return selected[i].canonical < selected[j].canonical
			}
			return selected[i].field < selected[j].field
		}
		return selected[i].artifactID < selected[j].artifactID
	})
	return selected
}

func sortedMentionArtifactIDs(groups map[string][]rawConceptMention) []string {
	out := make([]string, 0, len(groups))
	for artifactID := range groups {
		out = append(out, artifactID)
	}
	sort.Strings(out)
	return out
}

func sortEvidenceMentions(mentions []rawConceptMention) {
	sort.SliceStable(mentions, func(i, j int) bool {
		left := evidenceMentionPriority(mentions[i])
		right := evidenceMentionPriority(mentions[j])
		if left == right {
			if mentions[i].kind == mentions[j].kind {
				if mentions[i].canonical == mentions[j].canonical {
					if mentions[i].field == mentions[j].field {
						if mentions[i].sectionID == mentions[j].sectionID {
							return mentions[i].form < mentions[j].form
						}
						return mentions[i].sectionID < mentions[j].sectionID
					}
					return mentions[i].field < mentions[j].field
				}
				return mentions[i].canonical < mentions[j].canonical
			}
			return mentions[i].kind < mentions[j].kind
		}
		return left > right
	})
}

func evidenceMentionPriority(mention rawConceptMention) float64 {
	score := mention.weight
	switch mention.field {
	case "path":
		score += 0.35
	case "title":
		score += 0.3
	case "symbol", "test_name", "parent_title":
		score += 0.25
	case "openspec_change_id", "openspec_capability":
		score += 0.22
	case "assertion":
		score += 0.18
	case "heading":
		score += 0.08
	}
	switch mention.kind {
	case conceptKindSymbol, conceptKindTestBehavior:
		score += 0.18
	case conceptKindCompactIdentifier:
		score += 0.12
	case conceptKindPathFragment:
		score += 0.08
	case conceptKindPhrase:
		score += 0.04
	}
	if mention.sectionID == "" {
		score += 0.03
	}
	if len(mention.canonical) >= 8 {
		score += 0.02
	}
	return score
}

func addMetadataMentions(artifact evidenceArtifact, add func(kind, canonical, form, field, source, sectionID string, weight float64)) {
	for _, key := range []string{"test_name", "parent_title"} {
		value := evidenceString(artifact.extracted[key])
		if value == "" {
			continue
		}
		for _, phrase := range phraseConcepts(value) {
			add(conceptKindTestBehavior, phrase, value, key, "metadata", "", 0.95)
		}
		for _, compact := range compactConcepts(value) {
			add(conceptKindCompactIdentifier, compact, compact, key, "metadata", "", 0.9)
		}
	}
	for _, key := range []string{"openspec_change_id", "openspec_capability", "openspec_role"} {
		value := evidenceString(artifact.extracted[key])
		if value == "" {
			continue
		}
		for _, phrase := range phraseConcepts(value) {
			add(conceptKindPhrase, phrase, value, key, "metadata", "", 0.75)
		}
		for _, compact := range compactConcepts(value) {
			add(conceptKindCompactIdentifier, compact, compact, key, "metadata", "", 0.85)
		}
	}
	for _, key := range []string{"language", "framework", "mode", "artifact_scope"} {
		value := normalizeRoleValue(evidenceString(artifact.extracted[key]))
		if value != "" {
			add(conceptKindArtifactRole, key+":"+value, value, key, "metadata", "", 0.45)
		}
	}
	symbolsAdded := 0
	for _, symbol := range evidenceStringSlice(artifact.extracted["symbols"]) {
		if symbolsAdded >= maxSymbolConceptsPerArtifact {
			break
		}
		compact := compactIdentifier(symbol)
		if compact != "" {
			add(conceptKindSymbol, compact, symbol, "symbol", "metadata", "", 1.0)
			symbolsAdded++
		}
	}
	for _, assertion := range evidenceStringSlice(artifact.extracted["assertion_terms"]) {
		for _, phrase := range phraseConcepts(assertion) {
			add(conceptKindTestBehavior, phrase, assertion, "assertion", "metadata", "", 0.7)
		}
	}
}

func addSectionMentions(section store.SectionRow, add func(kind, canonical, form, field, source, sectionID string, weight float64)) {
	values := []string{section.Title}
	if strings.TrimSpace(section.Title) == "" && section.HeadingPath != "" {
		parts := strings.Split(section.HeadingPath, ">")
		values = append(values, parts[len(parts)-1])
	}
	for _, value := range values {
		for _, phrase := range phraseConcepts(value) {
			add(conceptKindPhrase, phrase, value, "heading", "section", section.ID, 0.75)
		}
		for _, compact := range compactConcepts(value) {
			add(conceptKindCompactIdentifier, compact, compact, "heading", "section", section.ID, 0.8)
		}
	}
	if section.SectionKind != "" {
		add(conceptKindArtifactRole, "section_kind:"+normalizeRoleValue(section.SectionKind), section.SectionKind, "section_kind", "section", section.ID, 0.4)
	}
}

type evidenceEdgeBuilder struct {
	repoID string
	byKey  map[string]*edgeAccumulator
}

func newEvidenceEdgeBuilder(repoID string) *evidenceEdgeBuilder {
	return &evidenceEdgeBuilder{repoID: repoID, byKey: map[string]*edgeAccumulator{}}
}

func (b *evidenceEdgeBuilder) add(src, dst, edgeType, signal string, weight, confidence float64, evidenceCount int, explanation string, metadata map[string]any) {
	if src == "" || dst == "" || src == dst || edgeType == "" || signal == "" {
		return
	}
	if evidenceCount <= 0 {
		evidenceCount = 1
	}
	key := strings.Join([]string{b.repoID, src, dst, edgeType, signal}, "\x00")
	if existing := b.byKey[key]; existing != nil {
		existing.input.EvidenceCount += evidenceCount
		if weight > existing.input.Weight {
			existing.input.Weight = weight
		}
		if confidence > existing.input.Confidence {
			existing.input.Confidence = confidence
		}
		return
	}
	b.byKey[key] = &edgeAccumulator{input: store.ArtifactEdgeInput{
		ID:            stableEvidenceID("edge", b.repoID, src, dst, edgeType, signal),
		RepoID:        b.repoID,
		SrcArtifactID: src,
		DstArtifactID: dst,
		EdgeType:      edgeType,
		Weight:        clampEvidence(weight, 0, 1),
		Confidence:    clampEvidence(confidence, 0, 1),
		EvidenceCount: evidenceCount,
		SourceSignal:  signal,
		Explanation:   explanation,
		MetadataJSON:  evidenceJSON(metadata),
	}}
}

func (b *evidenceEdgeBuilder) edges() []store.ArtifactEdgeInput {
	out := make([]store.ArtifactEdgeInput, 0, len(b.byKey))
	for _, acc := range b.byKey {
		out = append(out, acc.input)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EdgeType == out[j].EdgeType {
			if out[i].SrcArtifactID == out[j].SrcArtifactID {
				if out[i].DstArtifactID == out[j].DstArtifactID {
					return out[i].SourceSignal < out[j].SourceSignal
				}
				return out[i].DstArtifactID < out[j].DstArtifactID
			}
			return out[i].SrcArtifactID < out[j].SrcArtifactID
		}
		return out[i].EdgeType < out[j].EdgeType
	})
	return out
}

func materializeSharedConceptEdges(artifacts []evidenceArtifact, concepts map[string]*conceptAccumulator, mentions []rawConceptMention, builder *evidenceEdgeBuilder) []EvidenceConceptExample {
	conceptInputByKey := map[string]store.ConceptInput{}
	conceptStrengthByKey := map[string]int{}
	noisyByKey := map[string]EvidenceConceptExample{}
	artifactCount := len(artifacts)
	for key, acc := range concepts {
		input := acc.input
		input.DocumentFrequency = len(acc.artifactIDs)
		conceptInputByKey[key] = input
	}
	mentionsByConcept := map[string]map[string]bool{}
	for _, mention := range mentions {
		key := conceptKey(mention.kind, mention.canonical)
		input, ok := conceptInputByKey[key]
		if !ok || !sharedConceptEdgeKind(input.Kind) {
			continue
		}
		if reason := noisyConceptReason(input, artifactCount); reason != "" {
			noisyByKey[key] = EvidenceConceptExample{Kind: input.Kind, Canonical: input.Canonical, DocumentFrequency: input.DocumentFrequency, Reason: reason}
			continue
		}
		strength := sharedConceptEdgeStrength(input)
		if strength == 0 {
			noisyByKey[key] = EvidenceConceptExample{Kind: input.Kind, Canonical: input.Canonical, DocumentFrequency: input.DocumentFrequency, Reason: "not_edge_eligible"}
			continue
		}
		conceptStrengthByKey[key] = strength
		byArtifact := mentionsByConcept[key]
		if byArtifact == nil {
			byArtifact = map[string]bool{}
			mentionsByConcept[key] = byArtifact
		}
		byArtifact[mention.artifactID] = true
	}

	pairs := map[string]*pairEvidence{}
	for key, byArtifact := range mentionsByConcept {
		input := conceptInputByKey[key]
		ids := sortedMapKeys(byArtifact)
		if len(ids) < 2 {
			continue
		}
		if len(ids) > maxSharedConceptArtifacts {
			noisyByKey[key] = EvidenceConceptExample{Kind: input.Kind, Canonical: input.Canonical, DocumentFrequency: input.DocumentFrequency, Reason: "too_many_artifacts_for_edge_materialization"}
			continue
		}
		ev := edgeConceptEvidence{kind: input.Kind, canonical: input.Canonical, idf: input.InverseDocumentFrequency, strong: conceptStrengthByKey[key] >= 2}
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				src, dst := orderedArtifactPair(ids[i], ids[j])
				pairKey := src + "\x00" + dst
				pair := pairs[pairKey]
				if pair == nil {
					pair = &pairEvidence{src: src, dst: dst}
					pairs[pairKey] = pair
				}
				pair.concepts = append(pair.concepts, ev)
			}
		}
	}
	pairList := make([]*pairEvidence, 0, len(pairs))
	for _, pair := range pairs {
		sort.Slice(pair.concepts, func(i, j int) bool {
			if pair.concepts[i].strong != pair.concepts[j].strong {
				return pair.concepts[i].strong
			}
			if pair.concepts[i].idf == pair.concepts[j].idf {
				if pair.concepts[i].kind == pair.concepts[j].kind {
					return pair.concepts[i].canonical < pair.concepts[j].canonical
				}
				return pair.concepts[i].kind < pair.concepts[j].kind
			}
			return pair.concepts[i].idf > pair.concepts[j].idf
		})
		if len(pair.concepts) > maxSharedConceptsPerPair {
			pair.concepts = pair.concepts[:maxSharedConceptsPerPair]
		}
		pairList = append(pairList, pair)
	}
	sort.Slice(pairList, func(i, j int) bool {
		leftScore := sharedConceptPairScore(pairList[i])
		rightScore := sharedConceptPairScore(pairList[j])
		if leftScore == rightScore {
			if pairList[i].src == pairList[j].src {
				return pairList[i].dst < pairList[j].dst
			}
			return pairList[i].src < pairList[j].src
		}
		return leftScore > rightScore
	})
	selectedPairs := make([]*pairEvidence, 0, minInt(maxSharedConceptEdges, len(pairList)))
	edgeDegreeByArtifact := map[string]int{}
	for _, pair := range pairList {
		if len(selectedPairs) >= maxSharedConceptEdges {
			break
		}
		if edgeDegreeByArtifact[pair.src] >= maxSharedConceptEdgesPerArtifact || edgeDegreeByArtifact[pair.dst] >= maxSharedConceptEdgesPerArtifact {
			continue
		}
		selectedPairs = append(selectedPairs, pair)
		edgeDegreeByArtifact[pair.src]++
		edgeDegreeByArtifact[pair.dst]++
	}
	for _, pair := range selectedPairs {
		maxIDF := 0.0
		strongCount := 0
		semanticCount := 0
		names := make([]string, 0, minInt(3, len(pair.concepts)))
		metaConcepts := make([]map[string]any, 0, minInt(maxSharedConceptMetadataPerEdge, len(pair.concepts)))
		for i, concept := range pair.concepts {
			if concept.idf > maxIDF {
				maxIDF = concept.idf
			}
			if concept.strong {
				strongCount++
			}
			if semanticEdgeConcept(concept.kind) {
				semanticCount++
			}
			if i < 3 {
				names = append(names, concept.canonical)
			}
			if i < maxSharedConceptMetadataPerEdge {
				metaConcepts = append(metaConcepts, map[string]any{"kind": concept.kind, "canonical": concept.canonical, "idf": roundEvidence(concept.idf), "strong": concept.strong})
			}
		}
		evidenceCount := len(pair.concepts)
		weakCount := evidenceCount - strongCount
		weight := 0.32 + float64(strongCount)*0.12 + float64(weakCount)*0.06 + maxIDF*0.04
		confidence := 0.56 + float64(strongCount)*0.10 + float64(weakCount)*0.04 + maxIDF*0.03
		if strongCount == 0 {
			confidence = minFloat(confidence, 0.84)
		}
		if semanticCount == 0 {
			confidence = minFloat(confidence, 0.84)
		}
		explanation := fmt.Sprintf("shares rare concept %q", names[0])
		if len(names) > 1 {
			explanation = fmt.Sprintf("shares %d rare concepts including %q", evidenceCount, names[0])
		}
		builder.add(pair.src, pair.dst, edgeTypeMentionsSameConcept, "shared_rare_concept", weight, confidence, evidenceCount, explanation, map[string]any{"concepts": metaConcepts})
	}
	noisy := make([]EvidenceConceptExample, 0, len(noisyByKey))
	for _, example := range noisyByKey {
		noisy = append(noisy, example)
	}
	sort.Slice(noisy, func(i, j int) bool {
		if noisy[i].DocumentFrequency == noisy[j].DocumentFrequency {
			if noisy[i].Kind == noisy[j].Kind {
				return noisy[i].Canonical < noisy[j].Canonical
			}
			return noisy[i].Kind < noisy[j].Kind
		}
		return noisy[i].DocumentFrequency > noisy[j].DocumentFrequency
	})
	if len(noisy) > 10 {
		noisy = noisy[:10]
	}
	return noisy
}

func materializeLayoutGroupEdges(artifacts []evidenceArtifact, builder *evidenceEdgeBuilder) {
	groups := map[string][]string{}
	for _, artifact := range artifacts {
		for _, src := range artifact.sources {
			group := strings.TrimSpace(filepath.ToSlash(src.LayoutGroup))
			if group == "" {
				continue
			}
			groups[group] = appendUnique(groups[group], artifact.id)
		}
	}
	for group, ids := range groups {
		sort.Strings(ids)
		if len(ids) < 2 || len(ids) > 8 {
			continue
		}
		emitted := 0
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				if emitted >= maxLayoutGroupEdgesPerGroup {
					break
				}
				src, dst := orderedArtifactPair(ids[i], ids[j])
				builder.add(src, dst, edgeTypeSameLayoutGroup, "layout_group", 0.62, 0.72, 1, "same layout group "+group, map[string]any{"layout_group": group})
				emitted++
			}
		}
	}
}

func materializeSameSourcePathEdges(artifacts []evidenceArtifact, builder *evidenceEdgeBuilder) {
	byPath := map[string][]string{}
	for _, artifact := range artifacts {
		for _, src := range artifact.sources {
			path := strings.TrimSpace(filepath.ToSlash(src.Path))
			if path == "" {
				continue
			}
			byPath[path] = appendUnique(byPath[path], artifact.id)
		}
	}
	for path, ids := range byPath {
		sort.Strings(ids)
		if len(ids) < 2 || len(ids) > 12 {
			continue
		}
		emitted := 0
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				if emitted >= maxSameSourcePathEdgesPerPath {
					break
				}
				src, dst := orderedArtifactPair(ids[i], ids[j])
				builder.add(src, dst, edgeTypeSameFileOrLineVariant, "source_path", 0.68, 0.78, 1, "same source path "+path, map[string]any{"source_path": path})
				emitted++
			}
		}
	}
}

type testTriangulationInfo struct {
	artifact    evidenceArtifact
	path        string
	dir         string
	stem        string
	symbols     map[string]string
	importKeys  map[string]bool
	importForms map[string]bool
}

type sourceTriangulationInfo struct {
	artifact   evidenceArtifact
	path       string
	dir        string
	stem       string
	moduleKeys map[string]bool
	symbols    map[string]string
}

type testSourceCandidate struct {
	test        *testTriangulationInfo
	source      *sourceTriangulationInfo
	signals     map[string]bool
	symbols     map[string]string
	importForms map[string]bool
	weight      float64
	confidence  float64
}

func materializeTestSourceTriangulationEdges(artifacts []evidenceArtifact, builder *evidenceEdgeBuilder, opts evidenceGraphBuildOptions) {
	tests, sources := collectTriangulationArtifacts(artifacts)
	if len(tests) == 0 || len(sources) == 0 {
		return
	}
	sourcesByStem := map[string][]*sourceTriangulationInfo{}
	sourcesByModuleKey := map[string][]*sourceTriangulationInfo{}
	sourcesBySymbol := map[string][]*sourceTriangulationInfo{}
	sourcesByPath := map[string]*sourceTriangulationInfo{}
	sourceStemCount := map[string]int{}
	for i := range sources {
		source := &sources[i]
		if source.path != "" {
			sourcesByPath[source.path] = source
		}
		if source.stem != "" {
			sourcesByStem[source.stem] = append(sourcesByStem[source.stem], source)
			sourceStemCount[source.stem]++
		}
		for key := range source.moduleKeys {
			sourcesByModuleKey[key] = append(sourcesByModuleKey[key], source)
		}
		for symbol := range source.symbols {
			sourcesBySymbol[symbol] = append(sourcesBySymbol[symbol], source)
		}
	}

	var selected []*testSourceCandidate
	for i := range tests {
		test := &tests[i]
		bySource := map[string]*testSourceCandidate{}
		addCandidate := func(source *sourceTriangulationInfo, signal string, weight, confidence float64) *testSourceCandidate {
			if source == nil || signal == "" || test.artifact.id == source.artifact.id {
				return nil
			}
			candidate := bySource[source.artifact.id]
			if candidate == nil {
				candidate = &testSourceCandidate{
					test:        test,
					source:      source,
					signals:     map[string]bool{},
					symbols:     map[string]string{},
					importForms: map[string]bool{},
				}
				bySource[source.artifact.id] = candidate
			}
			candidate.signals[signal] = true
			if weight > candidate.weight {
				candidate.weight = weight
			}
			if confidence > candidate.confidence {
				candidate.confidence = confidence
			}
			return candidate
		}

		if test.stem != "" {
			for _, source := range cappedTriangulationSources(sourcesByStem[test.stem]) {
				if !allowTestSourceStemMatch(test, source, sourceStemCount[test.stem]) {
					continue
				}
				confidence := 0.86
				weight := 0.84
				if triangulationPathsAreNear(test.path, source.path) {
					confidence = 0.91
					weight = 0.9
				}
				addCandidate(source, "test_source_stem", weight, confidence)
			}
		}

		if opts.RichTypedIndex {
			for _, form := range sortedMapKeys(test.importForms) {
				for _, source := range resolveRelativeImportSources(test.path, form, sourcesByPath) {
					candidate := addCandidate(source, "relative_import_path", 0.96, 0.96)
					if candidate != nil {
						candidate.importForms[form] = true
					}
				}
			}
		}

		for _, key := range sortedMapKeys(test.importKeys) {
			for _, source := range cappedTriangulationSources(sourcesByModuleKey[key]) {
				candidate := addCandidate(source, "direct_import", 0.92, 0.92)
				if candidate != nil {
					for form := range test.importForms {
						if importKeysForModule(form)[key] {
							candidate.importForms[form] = true
						}
					}
				}
			}
		}

		for _, symbol := range sortedStringValueMapKeys(test.symbols) {
			for _, source := range cappedTriangulationSymbolSources(sourcesBySymbol[symbol]) {
				weight := 0.74
				confidence := 0.72
				if triangulationPathsAreNear(test.path, source.path) {
					weight = 0.8
					confidence = 0.8
				}
				candidate := addCandidate(source, "source_symbol_match", weight, confidence)
				if candidate != nil {
					candidate.symbols[symbol] = firstNonEmpty(test.symbols[symbol], source.symbols[symbol], symbol)
				}
			}
		}

		candidates := make([]*testSourceCandidate, 0, len(bySource))
		for _, candidate := range bySource {
			finalizeTestSourceCandidate(candidate)
			if candidate.confidence >= 0.75 {
				candidates = append(candidates, candidate)
			}
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].confidence == candidates[j].confidence {
				if len(candidates[i].signals) == len(candidates[j].signals) {
					return candidates[i].source.path < candidates[j].source.path
				}
				return len(candidates[i].signals) > len(candidates[j].signals)
			}
			return candidates[i].confidence > candidates[j].confidence
		})
		if len(candidates) > maxTestSourceEdgesPerTest {
			candidates = candidates[:maxTestSourceEdgesPerTest]
		}
		selected = append(selected, candidates...)
	}

	sort.Slice(selected, func(i, j int) bool {
		if selected[i].confidence == selected[j].confidence {
			if selected[i].test.path == selected[j].test.path {
				return selected[i].source.path < selected[j].source.path
			}
			return selected[i].test.path < selected[j].test.path
		}
		return selected[i].confidence > selected[j].confidence
	})
	if len(selected) > maxTestSourceEdges {
		selected = selected[:maxTestSourceEdges]
	}
	for _, candidate := range selected {
		signals := sortedMapKeys(candidate.signals)
		signal := strongestTestSourceSignal(candidate.signals)
		symbols := limitedSortedStringMapValues(candidate.symbols, 6)
		imports := limitedSortedMapKeys(candidate.importForms, 6)
		explanation := testSourceExplanation(signal, candidate.test.path, candidate.source.path, symbols)
		metadata := map[string]any{
			"test_path":   candidate.test.path,
			"source_path": candidate.source.path,
			"signals":     signals,
		}
		if len(symbols) > 0 {
			metadata["symbols"] = symbols
		}
		if len(imports) > 0 {
			metadata["imports"] = imports
		}
		builder.add(candidate.test.artifact.id, candidate.source.artifact.id, edgeTypeTestsSource, signal, candidate.weight, candidate.confidence, len(signals)+len(symbols)+len(imports), explanation, metadata)
		if len(symbols) > 0 {
			builder.add(candidate.test.artifact.id, candidate.source.artifact.id, edgeTypeMentionsSymbol, "test_symbol_match", minFloat(candidate.weight, 0.78), minFloat(candidate.confidence, 0.82), len(symbols), "test mentions source symbol "+symbols[0], metadata)
		}
	}
}

func collectTriangulationArtifacts(artifacts []evidenceArtifact) ([]testTriangulationInfo, []sourceTriangulationInfo) {
	var tests []testTriangulationInfo
	var sources []sourceTriangulationInfo
	for _, artifact := range artifacts {
		path := primaryEvidencePath(artifact)
		if path == "" {
			continue
		}
		if isEvidenceTestArtifact(artifact) {
			info := testTriangulationInfo{
				artifact:    artifact,
				path:        path,
				dir:         filepath.ToSlash(filepath.Dir(path)),
				stem:        normalizedTestSourceStem(path),
				symbols:     triangulationTestSymbols(artifact),
				importKeys:  map[string]bool{},
				importForms: map[string]bool{},
			}
			for _, imp := range extractTriangulationImports(artifact.body) {
				info.importForms[imp] = true
				for key := range importKeysForModule(imp) {
					info.importKeys[key] = true
				}
			}
			tests = append(tests, info)
			continue
		}
		if isEvidenceSourceArtifact(artifact) {
			if sourcePathLooksLikeTest(path) {
				continue
			}
			sourceSymbols := extractSourceTriangulationSymbols(artifact)
			info := sourceTriangulationInfo{
				artifact:   artifact,
				path:       path,
				dir:        filepath.ToSlash(filepath.Dir(path)),
				stem:       normalizedSourceStem(path),
				moduleKeys: sourceModuleKeys(path),
				symbols:    sourceSymbols,
			}
			sources = append(sources, info)
		}
	}
	return tests, sources
}

func isEvidenceTestArtifact(artifact evidenceArtifact) bool {
	if normalizeRoleValue(artifact.subtype) == "test_case" || normalizeRoleValue(evidenceString(artifact.extracted["subtype"])) == "test_case" {
		return true
	}
	for _, src := range artifact.sources {
		if normalizeRoleValue(src.SourceType) == "test_case" {
			return true
		}
	}
	return false
}

func isEvidenceSourceArtifact(artifact evidenceArtifact) bool {
	if normalizeRoleValue(artifact.kind) != "source_context" {
		return false
	}
	subtype := normalizeRoleValue(artifact.subtype)
	if subtype == "test_case" || subtype == "code_comment" {
		return false
	}
	for _, src := range artifact.sources {
		if normalizeRoleValue(src.SourceType) == "source_context" {
			return true
		}
	}
	return false
}

func primaryEvidencePath(artifact evidenceArtifact) string {
	if path := evidenceString(artifact.extracted["source_path"]); path != "" {
		return normalizeEvidencePath(path)
	}
	for _, src := range artifact.sources {
		if strings.TrimSpace(src.Path) != "" {
			return normalizeEvidencePath(src.Path)
		}
	}
	return ""
}

func normalizeEvidencePath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	return strings.Trim(path, "/")
}

func normalizedSourceStem(path string) string {
	base := filepath.Base(filepath.ToSlash(path))
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	stem = strings.TrimSuffix(stem, ".d")
	return compactIdentifier(stem)
}

func normalizedTestSourceStem(path string) string {
	base := filepath.Base(filepath.ToSlash(path))
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	for _, suffix := range []string{"_test", "-test", ".test", "_spec", "-spec", ".spec", "_tests", "-tests", ".tests"} {
		stem = strings.TrimSuffix(stem, suffix)
	}
	for _, prefix := range []string{"test_", "test-", "spec_", "spec-"} {
		stem = strings.TrimPrefix(stem, prefix)
	}
	return compactIdentifier(stem)
}

func sourceModuleKeys(path string) map[string]bool {
	path = strings.TrimSuffix(normalizeEvidencePath(path), filepath.Ext(path))
	keys := map[string]bool{}
	addModuleKeys(keys, path)
	if base := filepath.Base(path); base != "." && base != "" {
		addModuleKeys(keys, base)
	}
	return keys
}

func importKeysForModule(value string) map[string]bool {
	keys := map[string]bool{}
	addModuleKeys(keys, value)
	if idx := strings.LastIndexAny(value, "/.:"); idx >= 0 && idx+1 < len(value) {
		addModuleKeys(keys, value[idx+1:])
	}
	return keys
}

func addModuleKeys(keys map[string]bool, value string) {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	value = strings.TrimPrefix(value, "./")
	value = trimKnownTriangulationExtension(value)
	value = strings.NewReplacer("/", " ", "\\", " ", ".", " ", "::", " ", "-", " ", "_", " ").Replace(value)
	for _, compact := range compactConcepts(value) {
		if usefulTriangulationKey(compact) {
			keys[compact] = true
		}
	}
}

func trimKnownTriangulationExtension(value string) string {
	ext := strings.ToLower(filepath.Ext(value))
	switch ext {
	case ".go", ".py", ".rs", ".java", ".kt", ".ts", ".tsx", ".js", ".jsx", ".vue", ".sql", ".toml", ".yaml", ".yml", ".json":
		return strings.TrimSuffix(value, filepath.Ext(value))
	default:
		return value
	}
}

func extractTriangulationImports(body string) []string {
	if len(body) > 128*1024 {
		body = body[:128*1024]
	}
	seen := map[string]bool{}
	var out []string
	for _, pattern := range testImportPatterns {
		for _, match := range pattern.FindAllStringSubmatch(body, 80) {
			if len(match) < 2 {
				continue
			}
			value := strings.TrimSpace(match[1])
			value = strings.Trim(value, `"'`)
			if !usefulTriangulationImport(value) || seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func triangulationTestSymbols(artifact evidenceArtifact) map[string]string {
	symbols := map[string]string{}
	add := func(value string) {
		if compact := compactIdentifier(value); usefulTriangulationSymbol(compact) {
			symbols[compact] = value
		}
	}
	for _, symbol := range evidenceStringSlice(artifact.extracted["symbols"]) {
		add(symbol)
	}
	add(evidenceString(artifact.extracted["test_name"]))
	return symbols
}

func extractSourceTriangulationSymbols(artifact evidenceArtifact) map[string]string {
	symbols := map[string]string{}
	for _, symbol := range evidenceStringSlice(artifact.extracted["symbols"]) {
		if compact := compactIdentifier(symbol); usefulTriangulationSymbol(compact) {
			symbols[compact] = symbol
		}
		if len(symbols) >= maxTriangulationSymbolsPerSource {
			return symbols
		}
	}
	body := artifact.body
	if len(body) > 128*1024 {
		body = body[:128*1024]
	}
	for _, pattern := range sourceSymbolPatterns {
		for _, match := range pattern.FindAllStringSubmatch(body, 80) {
			if len(match) < 2 {
				continue
			}
			symbol := strings.TrimSpace(match[1])
			if compact := compactIdentifier(symbol); usefulTriangulationSymbol(compact) {
				symbols[compact] = symbol
			}
			if len(symbols) >= maxTriangulationSymbolsPerSource {
				return symbols
			}
		}
	}
	return symbols
}

func cappedTriangulationSources(sources []*sourceTriangulationInfo) []*sourceTriangulationInfo {
	if len(sources) > maxTestSourceCandidatesPerKey {
		return nil
	}
	return sources
}

func cappedTriangulationSymbolSources(sources []*sourceTriangulationInfo) []*sourceTriangulationInfo {
	if len(sources) > 4 {
		return nil
	}
	return sources
}

func allowTestSourceStemMatch(test *testTriangulationInfo, source *sourceTriangulationInfo, sourceStemCount int) bool {
	if test.stem == "" || test.stem != source.stem || triangulationGenericStem(test.stem) {
		return false
	}
	if sourceStemCount <= 3 {
		return true
	}
	return triangulationPathsAreNear(test.path, source.path)
}

func triangulationPathsAreNear(testPath, sourcePath string) bool {
	testDir := filepath.ToSlash(filepath.Dir(testPath))
	sourceDir := filepath.ToSlash(filepath.Dir(sourcePath))
	if testDir == sourceDir {
		return true
	}
	testDir = strings.Trim(testDir, "/")
	sourceDir = strings.Trim(sourceDir, "/")
	if strings.HasPrefix(testDir, sourceDir+"/") || strings.HasPrefix(sourceDir, testDir+"/") {
		return true
	}
	testTrimmed := strings.TrimPrefix(testDir, "tests/")
	testTrimmed = strings.TrimPrefix(testTrimmed, "test/")
	return testTrimmed != testDir && (testTrimmed == sourceDir || strings.HasSuffix(sourceDir, "/"+testTrimmed))
}

func finalizeTestSourceCandidate(candidate *testSourceCandidate) {
	if candidate == nil {
		return
	}
	if candidate.signals["relative_import_path"] {
		candidate.weight = maxFloat(candidate.weight, 0.96)
		candidate.confidence = maxFloat(candidate.confidence, 0.96)
		return
	}
	if candidate.signals["direct_import"] && candidate.signals["test_source_stem"] {
		candidate.weight = maxFloat(candidate.weight, 0.95)
		candidate.confidence = maxFloat(candidate.confidence, 0.95)
		return
	}
	if candidate.signals["test_source_stem"] && candidate.signals["source_symbol_match"] {
		candidate.weight = maxFloat(candidate.weight, 0.9)
		candidate.confidence = maxFloat(candidate.confidence, 0.9)
		return
	}
	if candidate.signals["direct_import"] && candidate.signals["source_symbol_match"] {
		candidate.weight = maxFloat(candidate.weight, 0.93)
		candidate.confidence = maxFloat(candidate.confidence, 0.93)
	}
}

func strongestTestSourceSignal(signals map[string]bool) string {
	for _, signal := range []string{"relative_import_path", "direct_import", "test_source_stem", "source_symbol_match"} {
		if signals[signal] {
			return signal
		}
	}
	return "test_source"
}

func testSourceExplanation(signal, testPath, sourcePath string, symbols []string) string {
	switch signal {
	case "relative_import_path":
		return "test imports local source path " + sourcePath
	case "direct_import":
		return "test imports likely source " + sourcePath
	case "test_source_stem":
		return "test/source stem match for " + sourcePath
	case "source_symbol_match":
		if len(symbols) > 0 {
			return "test mentions source symbol " + symbols[0]
		}
	}
	return "test likely relates to source " + sourcePath + " from " + testPath
}

func resolveRelativeImportSources(testPath, importForm string, sourcesByPath map[string]*sourceTriangulationInfo) []*sourceTriangulationInfo {
	importForm = strings.TrimSpace(strings.Trim(importForm, `"'`))
	if !strings.HasPrefix(importForm, "./") && !strings.HasPrefix(importForm, "../") {
		return nil
	}
	if strings.ContainsAny(importForm, "\x00\r\n\t ") {
		return nil
	}
	if idx := strings.IndexAny(importForm, "?#"); idx >= 0 {
		importForm = importForm[:idx]
	}
	base := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(testPath), filepath.FromSlash(importForm))))
	base = strings.TrimPrefix(base, "./")
	if base == "." || base == "" || strings.HasPrefix(base, "../") || strings.Contains(base, "/../") {
		return nil
	}
	seen := map[string]bool{}
	var candidates []string
	add := func(path string) {
		path = normalizeEvidencePath(path)
		if path != "" && !seen[path] {
			seen[path] = true
			candidates = append(candidates, path)
		}
	}
	add(base)
	ext := strings.ToLower(filepath.Ext(base))
	if ext == "" {
		for _, known := range []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue", ".py", ".go", ".rs", ".java", ".kt", ".kts", ".sql", ".toml"} {
			add(base + known)
			add(filepath.ToSlash(filepath.Join(base, "index"+known)))
		}
	}
	sort.Strings(candidates)
	var out []*sourceTriangulationInfo
	for _, path := range candidates {
		if source := sourcesByPath[path]; source != nil {
			out = append(out, source)
		}
	}
	return out
}

func materializeRichSymbolReferenceEdges(artifacts []evidenceArtifact, builder *evidenceEdgeBuilder) {
	sourceSymbols := map[string][]sourceTriangulationInfo{}
	for _, artifact := range artifacts {
		if !isEvidenceSourceArtifact(artifact) {
			continue
		}
		path := primaryEvidencePath(artifact)
		if path == "" || sourcePathLooksLikeTest(path) {
			continue
		}
		info := sourceTriangulationInfo{
			artifact: artifact,
			path:     path,
			symbols:  extractSourceTriangulationSymbols(artifact),
		}
		for symbol := range info.symbols {
			sourceSymbols[symbol] = append(sourceSymbols[symbol], info)
		}
	}
	for symbol, sources := range sourceSymbols {
		if len(sources) == 0 || len(sources) > maxRichSymbolSources {
			delete(sourceSymbols, symbol)
		}
	}

	type richSymbolReferenceEdge struct {
		ref      evidenceArtifact
		source   sourceTriangulationInfo
		symbol   string
		form     string
		refField string
	}
	var edges []richSymbolReferenceEdge
	perRef := map[string]int{}
	perSource := map[string]int{}
	for _, artifact := range artifacts {
		if isEvidenceSourceArtifact(artifact) {
			continue
		}
		refs := richSymbolReferences(artifact)
		for _, symbol := range sortedStringValueMapKeys(refs) {
			sources := sourceSymbols[symbol]
			if len(sources) == 0 {
				continue
			}
			for _, source := range sources {
				if source.artifact.id == artifact.id {
					continue
				}
				if perRef[artifact.id] >= maxRichSymbolReferencePerRef || perSource[source.artifact.id] >= maxRichSymbolReferencePerSource || len(edges) >= maxRichSymbolReferenceEdges {
					continue
				}
				edges = append(edges, richSymbolReferenceEdge{
					ref:      artifact,
					source:   source,
					symbol:   symbol,
					form:     firstNonEmpty(refs[symbol], source.symbols[symbol], symbol),
					refField: richSymbolReferenceField(artifact),
				})
				perRef[artifact.id]++
				perSource[source.artifact.id]++
			}
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].ref.id == edges[j].ref.id {
			if edges[i].source.artifact.id == edges[j].source.artifact.id {
				return edges[i].symbol < edges[j].symbol
			}
			return edges[i].source.path < edges[j].source.path
		}
		return edges[i].ref.id < edges[j].ref.id
	})
	for _, edge := range edges {
		metadata := map[string]any{
			"symbol":      edge.form,
			"symbol_key":  edge.symbol,
			"source_path": edge.source.path,
			"ref_field":   edge.refField,
		}
		builder.add(edge.ref.id, edge.source.artifact.id, edgeTypeMentionsSymbol, "symbol_reference", 0.78, 0.82, 1, "artifact mentions source symbol "+edge.form, metadata)
	}
}

func richSymbolReferences(artifact evidenceArtifact) map[string]string {
	refs := map[string]string{}
	add := func(value string) {
		if len(refs) >= maxRichSymbolReferencesPerArtifact {
			return
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if !symbolReferenceTokenLooksSpecific(value) {
			return
		}
		if compact := compactIdentifier(value); usefulTriangulationSymbol(compact) {
			refs[compact] = value
		}
	}
	for _, symbol := range evidenceStringSlice(artifact.extracted["symbols"]) {
		add(symbol)
	}
	for _, value := range []string{artifact.title, evidenceString(artifact.extracted["test_name"]), evidenceString(artifact.extracted["parent_title"])} {
		for _, token := range symbolReferenceTokens(value, 24) {
			add(token)
		}
	}
	for _, token := range symbolReferenceTokens(artifact.body, maxRichSymbolReferencesPerArtifact) {
		add(token)
	}
	return refs
}

func richSymbolReferenceField(artifact evidenceArtifact) string {
	if isEvidenceTestArtifact(artifact) {
		return "test_body"
	}
	if normalizeRoleValue(artifact.kind) == "markdown_artifact" {
		return "body"
	}
	if normalizeRoleValue(artifact.subtype) == "code_comment" {
		return "code_comment"
	}
	return "body"
}

func symbolReferenceTokens(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" || limit <= 0 {
		return nil
	}
	if len(text) > 128*1024 {
		text = text[:128*1024]
	}
	matches := symbolReferencePattern.FindAllString(text, limit*4)
	seen := map[string]bool{}
	var out []string
	for _, match := range matches {
		match = strings.Trim(match, "_.$")
		if !symbolReferenceTokenLooksSpecific(match) || seen[match] {
			continue
		}
		seen[match] = true
		out = append(out, match)
		if len(out) >= limit {
			break
		}
	}
	sort.Strings(out)
	return out
}

func symbolReferenceTokenLooksSpecific(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 4 || len(value) > 96 {
		return false
	}
	compact := compactIdentifier(value)
	if !usefulTriangulationSymbol(compact) {
		return false
	}
	hasSpecificShape := false
	prevLowerOrDigit := false
	for _, r := range value {
		switch {
		case r == '_' || r == '-' || r == '.' || r == '$':
			hasSpecificShape = true
		case unicode.IsDigit(r):
			hasSpecificShape = true
			prevLowerOrDigit = true
		case unicode.IsUpper(r):
			if prevLowerOrDigit {
				hasSpecificShape = true
			}
			prevLowerOrDigit = false
		case unicode.IsLower(r):
			prevLowerOrDigit = true
		default:
			prevLowerOrDigit = false
		}
	}
	if hasSpecificShape {
		return true
	}
	return len(value) >= 12 && !genericEdgeConcept(compact)
}

func usefulTriangulationImport(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 160 {
		return false
	}
	if strings.ContainsAny(value, " \t\n\r") {
		return false
	}
	if strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") {
		return true
	}
	return len(importKeysForModule(value)) > 0
}

func usefulTriangulationKey(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return len(value) >= 4 && !evidenceStopWords[value] && !triangulationGenericStem(value)
}

func usefulTriangulationSymbol(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return len(value) >= 4 && !evidenceStopWords[value] && !genericEdgeTerms[value] && !triangulationGenericStem(value)
}

func triangulationGenericStem(value string) bool {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "body", "common", "config", "error", "errors", "file", "files", "fixture", "fixtures", "helper", "helpers", "index", "main", "name", "parse", "path", "result", "setup", "shared", "source", "spec", "status", "target", "test", "tests", "title", "types", "util", "utils", "value", "values":
		return true
	default:
		return false
	}
}

func sourcePathLooksLikeTest(path string) bool {
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(filepath.ToSlash(path)), filepath.Ext(path)))
	if base == "" {
		return false
	}
	for _, suffix := range []string{"_test", "-test", ".test", "_spec", "-spec", ".spec", "_tests", "-tests", ".tests"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	for _, prefix := range []string{"test_", "test-", "spec_", "spec-"} {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}
	return false
}

func limitedSortedStringMapValues(values map[string]string, limit int) []string {
	seen := map[string]bool{}
	for key, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			value = key
		}
		if value != "" {
			seen[value] = true
		}
	}
	return limitedSortedMapKeys(seen, limit)
}

func sortedStringValueMapKeys(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func materializeLinkEdges(artifacts []evidenceArtifact, builder *evidenceEdgeBuilder) {
	known := map[string]bool{}
	for _, artifact := range artifacts {
		known[artifact.id] = true
	}
	for _, artifact := range artifacts {
		for _, link := range artifact.links {
			targetID := strings.TrimPrefix(link.Target, "artifact:")
			if !known[targetID] || targetID == artifact.id {
				continue
			}
			edgeType := edgeTypeExplicitReference
			if link.LinkType == linkOpenSpecCompanion {
				edgeType = edgeTypeOpenSpecCompanion
			}
			builder.add(artifact.id, targetID, edgeType, "link:"+link.LinkType, 0.9, 0.9, 1, "explicit "+link.LinkType+" link", map[string]any{"link_type": link.LinkType, "target": link.Target})
		}
	}
}

func materializePathReferenceEdges(artifacts []evidenceArtifact, builder *evidenceEdgeBuilder) {
	byPath := map[string][]string{}
	for _, artifact := range artifacts {
		for _, src := range artifact.sources {
			path := strings.TrimSpace(filepath.ToSlash(src.Path))
			if path == "" {
				continue
			}
			byPath[path] = appendUnique(byPath[path], artifact.id)
		}
	}
	var candidates []pathReferenceEdge
	for _, artifact := range artifacts {
		for _, ref := range extractPathReferences(artifact.body) {
			for _, targetID := range byPath[ref] {
				if targetID == artifact.id {
					continue
				}
				candidates = append(candidates, pathReferenceEdge{src: artifact.id, dst: targetID, target: ref})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].target == candidates[j].target {
			if candidates[i].src == candidates[j].src {
				return candidates[i].dst < candidates[j].dst
			}
			return candidates[i].src < candidates[j].src
		}
		return candidates[i].target < candidates[j].target
	})
	byTargetCount := map[string]int{}
	bySourceCount := map[string]int{}
	for _, edge := range candidates {
		if byTargetCount[edge.target] >= maxPathReferenceEdgesPerTarget || bySourceCount[edge.src] >= maxPathReferenceEdgesPerSource {
			continue
		}
		builder.add(edge.src, edge.dst, edgeTypeExplicitReference, "path_reference", 0.78, 0.78, 1, "references path "+edge.target, map[string]any{"path": edge.target})
		byTargetCount[edge.target]++
		bySourceCount[edge.src]++
	}
}

func buildEvidenceDiagnostics(concepts []store.ConceptInput, mentions []store.ConceptMentionInput, edges []store.ArtifactEdgeInput, noisy []EvidenceConceptExample, opts evidenceGraphBuildOptions) *EvidenceGraphDiagnostics {
	d := &EvidenceGraphDiagnostics{
		RichTypedIndex:       opts.RichTypedIndex,
		ConceptsIndexed:      len(concepts),
		MentionsIndexed:      len(mentions),
		EdgesIndexed:         len(edges),
		ConceptsByKind:       map[string]int{},
		MentionsByField:      map[string]int{},
		EdgesByType:          map[string]int{},
		NoisyConceptsSkipped: len(noisy),
		TopNoisyConcepts:     noisy,
	}
	for _, c := range concepts {
		d.ConceptsByKind[c.Kind]++
	}
	for _, m := range mentions {
		d.MentionsByField[m.Field]++
	}
	for _, e := range edges {
		d.EdgesByType[e.EdgeType]++
	}
	examplesByType := map[string]int{}
	for _, edge := range edges {
		if examplesByType[edge.EdgeType] >= 2 {
			continue
		}
		examplesByType[edge.EdgeType]++
		d.TopEdges = append(d.TopEdges, EvidenceEdgeExample{
			EdgeType:      edge.EdgeType,
			Source:        edge.SrcArtifactID,
			Target:        edge.DstArtifactID,
			SourceSignal:  edge.SourceSignal,
			Explanation:   edge.Explanation,
			Confidence:    roundEvidence(edge.Confidence),
			EvidenceCount: edge.EvidenceCount,
		})
	}
	sort.Slice(d.TopEdges, func(i, j int) bool {
		if d.TopEdges[i].EdgeType == d.TopEdges[j].EdgeType {
			if d.TopEdges[i].Source == d.TopEdges[j].Source {
				return d.TopEdges[i].Target < d.TopEdges[j].Target
			}
			return d.TopEdges[i].Source < d.TopEdges[j].Source
		}
		return d.TopEdges[i].EdgeType < d.TopEdges[j].EdgeType
	})
	return d
}

func decodeEvidenceJSON(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

func evidenceJSON(value any) string {
	b, err := json.Marshal(value)
	if err != nil || len(b) == 0 {
		return "{}"
	}
	return string(b)
}

func evidenceString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}

func evidenceStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := evidenceString(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func pathConceptParts(path string) []string {
	path = filepath.ToSlash(path)
	path = strings.TrimSuffix(path, filepath.Ext(path))
	var out []string
	for _, segment := range strings.Split(path, "/") {
		for _, word := range conceptWords(segment) {
			if len(word) >= 3 && !evidenceStopWords[word] {
				out = append(out, word)
			}
		}
	}
	return uniqueSorted(out)
}

func pathCompactConcepts(path string) []string {
	path = filepath.ToSlash(path)
	path = strings.TrimSuffix(path, filepath.Ext(path))
	var out []string
	for _, segment := range strings.Split(path, "/") {
		out = append(out, compactConcepts(segment)...)
	}
	return uniqueSorted(out)
}

func phraseConcepts(text string) []string {
	words := conceptWords(text)
	if len(words) == 0 {
		return nil
	}
	var out []string
	if len(words) == 1 {
		if len(words[0]) >= 5 && !evidenceStopWords[words[0]] {
			out = append(out, words[0])
		}
		return out
	}
	if len(words) <= 5 {
		out = append(out, strings.Join(words, " "))
	}
	if len(words) == 2 {
		return uniqueSorted(out)
	}
	if len(words) == 3 {
		out = append(out, strings.Join(words[:2], " "))
		out = append(out, strings.Join(words[1:], " "))
		return uniqueSorted(out)
	}
	out = append(out, strings.Join(words[:2], " "))
	out = append(out, strings.Join(words[len(words)-2:], " "))
	if len(words) == 4 {
		out = append(out, strings.Join(words[1:3], " "))
	}
	return uniqueSorted(out)
}

func compactConcepts(text string) []string {
	var out []string
	for _, segment := range splitLooseSegments(text) {
		if compact := compactIdentifier(segment); compact != "" {
			out = append(out, compact)
		}
	}
	if compact := compactIdentifier(text); compact != "" {
		out = append(out, compact)
	}
	return uniqueSorted(out)
}

func conceptWords(text string) []string {
	text = splitCamelBoundaries(text)
	var words []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		word := strings.ToLower(b.String())
		b.Reset()
		if len(word) < 2 || evidenceStopWords[word] {
			return
		}
		words = append(words, word)
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return words
}

func splitCamelBoundaries(text string) string {
	var out strings.Builder
	var prev rune
	for i, r := range text {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)) {
			out.WriteRune(' ')
		}
		out.WriteRune(r)
		prev = r
	}
	return out.String()
}

func splitLooseSegments(text string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func compactIdentifier(text string) string {
	words := conceptWords(text)
	if len(words) == 0 {
		return ""
	}
	compact := strings.Join(words, "")
	if len(compact) < 4 || evidenceStopWords[compact] {
		return ""
	}
	return compact
}

func normalizeRoleValue(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.NewReplacer(" ", "_", "-", "_", ".", "_", "/", "_").Replace(value)
	value = strings.Trim(value, "_")
	if value == "" || len(value) > 80 {
		return ""
	}
	return value
}

func persistEvidenceMention(mention rawConceptMention) bool {
	return mention.kind != conceptKindArtifactRole
}

func compactMentionEvidence(mention rawConceptMention) map[string]any {
	out := map[string]any{"source": mention.source}
	if form := truncateEvidenceValue(mention.form, maxMentionEvidenceFormLength); form != "" {
		out["form"] = form
	}
	return out
}

func truncateEvidenceValue(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func sharedConceptEdgeKind(kind string) bool {
	switch kind {
	case conceptKindIdentifier, conceptKindCompactIdentifier, conceptKindPathFragment, conceptKindPhrase, conceptKindSymbol, conceptKindTestBehavior:
		return true
	default:
		return false
	}
}

func sharedConceptEdgeStrength(concept store.ConceptInput) int {
	canonical := strings.TrimSpace(concept.Canonical)
	if canonical == "" || genericEdgeConcept(canonical) {
		return 0
	}
	words := strings.Fields(canonical)
	switch concept.Kind {
	case conceptKindSymbol, conceptKindIdentifier, conceptKindTestBehavior:
		if len(canonical) >= 4 {
			return 2
		}
	case conceptKindCompactIdentifier:
		if compactConceptFormsAreGeneric(concept.Forms) || compactConceptIsGeneric(canonical) {
			return 0
		}
		if len(canonical) >= 7 || containsDigit(canonical) {
			return 2
		}
		if len(canonical) >= 5 {
			return 1
		}
	case conceptKindPhrase:
		if len(words) >= 2 && meaningfulEdgeWordCount(words) >= 2 {
			return 2
		}
		if len(canonical) >= 8 && meaningfulEdgeWordCount(words) >= 1 {
			return 1
		}
	case conceptKindPathFragment:
		if len(canonical) >= 8 || containsDigit(canonical) {
			return 2
		}
		if len(canonical) >= 4 {
			return 1
		}
	}
	return 0
}

func compactConceptFormsAreGeneric(forms []string) bool {
	if len(forms) == 0 {
		return false
	}
	sawWords := false
	for _, form := range forms {
		words := conceptWords(form)
		if len(words) == 0 {
			continue
		}
		sawWords = true
		if meaningfulEdgeWordCount(words) > 0 {
			return false
		}
	}
	return sawWords
}

func compactConceptIsGeneric(canonical string) bool {
	canonical = strings.TrimSpace(strings.ToLower(canonical))
	if canonical == "" || len(canonical) > 48 {
		return false
	}
	memo := map[string]bool{"": true}
	var canSegment func(string) bool
	canSegment = func(rest string) bool {
		if value, ok := memo[rest]; ok {
			return value
		}
		for term := range genericEdgeTerms {
			if len(term) < 3 {
				continue
			}
			if strings.HasPrefix(rest, term) && canSegment(rest[len(term):]) {
				memo[rest] = true
				return true
			}
		}
		memo[rest] = false
		return false
	}
	return canSegment(canonical)
}

func sharedConceptPairScore(pair *pairEvidence) float64 {
	score := 0.0
	for _, concept := range pair.concepts {
		if concept.strong {
			score += 4.0
		} else {
			score += 1.0
		}
		score += concept.idf * 0.2
	}
	return score
}

func semanticEdgeConcept(kind string) bool {
	switch kind {
	case conceptKindPhrase, conceptKindSymbol, conceptKindTestBehavior:
		return true
	default:
		return false
	}
}

func genericEdgeConcept(canonical string) bool {
	canonical = strings.TrimSpace(strings.ToLower(canonical))
	if canonical == "" {
		return true
	}
	if genericEdgeTerms[canonical] || evidenceStopWords[canonical] {
		return true
	}
	words := strings.Fields(canonical)
	if len(words) == 0 {
		return false
	}
	return meaningfulEdgeWordCount(words) == 0
}

func meaningfulEdgeWordCount(words []string) int {
	count := 0
	for _, word := range words {
		word = strings.TrimSpace(strings.ToLower(word))
		if word == "" || evidenceStopWords[word] || genericEdgeTerms[word] {
			continue
		}
		count++
	}
	return count
}

func containsDigit(value string) bool {
	for _, r := range value {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func noisyConceptReason(concept store.ConceptInput, artifactCount int) string {
	if concept.DocumentFrequency < 2 {
		return "single_artifact"
	}
	if concept.DocumentFrequency > maxSharedConceptArtifacts {
		return "too_many_artifacts"
	}
	if evidenceStopWords[concept.Canonical] {
		return "stop_word"
	}
	if artifactCount >= 10 && float64(concept.DocumentFrequency)/float64(artifactCount) > 0.35 {
		return "high_document_frequency"
	}
	return ""
}

func extractPathReferences(body string) []string {
	if len(body) > 128*1024 {
		body = body[:128*1024]
	}
	matches := pathReferencePattern.FindAllStringSubmatch(body, 80)
	var out []string
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		path := strings.Trim(match[1], "`'\"()[]{}<>.,;:")
		path = filepath.ToSlash(path)
		path = strings.TrimPrefix(path, "./")
		if path != "" {
			out = append(out, path)
		}
	}
	return uniqueSorted(out)
}

func stableEvidenceID(prefix string, parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	return prefix + "_" + hex.EncodeToString(sum[:10])
}

func conceptKey(kind, canonical string) string {
	return kind + "\x00" + canonical
}

func orderedArtifactPair(a, b string) (string, string) {
	if a <= b {
		return a, b
	}
	return b, a
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func sortedMapKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func limitedSortedMapKeys(values map[string]bool, limit int) []string {
	out := sortedMapKeys(values)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = true
		}
	}
	return sortedMapKeys(seen)
}

func clampEvidence(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func roundEvidence(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var evidenceStopWords = map[string]bool{
	"a": true, "about": true, "above": true, "after": true, "again": true,
	"all": true, "also": true, "an": true, "and": true, "any": true,
	"are": true, "as": true, "at": true, "be": true, "been": true,
	"before": true, "being": true, "by": true, "can": true, "could": true,
	"do": true, "does": true, "done": true, "for": true, "from": true,
	"has": true, "have": true, "how": true, "in": true, "into": true,
	"is": true, "it": true, "its": true, "may": true, "must": true,
	"no": true, "not": true, "of": true, "on": true, "or": true,
	"our": true, "should": true, "that": true, "the": true, "their": true,
	"this": true, "to": true, "up": true, "use": true, "used": true,
	"using": true, "when": true, "where": true, "which": true, "with": true,
}

var genericEdgeTerms = map[string]bool{
	"additions": true, "architecture": true, "config": true, "context": true,
	"current": true, "deliverables": true, "descriptive": true, "discover": true,
	"discovery": true, "doc": true, "docs": true, "documentation": true,
	"example": true, "examples": true, "file": true, "files": true,
	"framework": true, "infrastructure": true, "implementation": true,
	"layer": true, "local": true, "model": true, "notes": true, "overview": true,
	"ownership": true, "page": true, "preconditions": true, "public": true,
	"quick": true, "rationale": true, "requirements": true, "resolution": true,
	"short": true, "states": true, "surface": true, "table": true,
	"template": true, "test": true, "testing": true, "title": true,
	"validate": true, "validation": true, "works": true, "your": true,
}
