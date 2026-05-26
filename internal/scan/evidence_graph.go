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

	maxSharedConceptArtifacts = 8
	maxSharedConceptEdges     = 5000
)

// EvidenceGraphDiagnostics is a scan-time summary of persisted graph evidence.
type EvidenceGraphDiagnostics struct {
	ConceptsIndexed      int                      `json:"concepts_indexed"`
	MentionsIndexed      int                      `json:"mentions_indexed"`
	EdgesIndexed         int                      `json:"edges_indexed"`
	ConceptsByKind       map[string]int           `json:"concepts_by_kind,omitempty"`
	MentionsByField      map[string]int           `json:"mentions_by_field,omitempty"`
	EdgesByType          map[string]int           `json:"edges_by_type,omitempty"`
	NoisyConceptsSkipped int                      `json:"noisy_concepts_skipped,omitempty"`
	TopNoisyConcepts     []EvidenceConceptExample `json:"top_noisy_concepts,omitempty"`
	TopEdges             []EvidenceEdgeExample    `json:"top_edges,omitempty"`
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
}

type edgeAccumulator struct {
	input store.ArtifactEdgeInput
}

var pathReferencePattern = regexp.MustCompile(`(?i)([A-Za-z0-9_.@+-]+(?:/[A-Za-z0-9_.@+-]+)+\.(?:md|markdown|txt|adoc|rst|go|py|rs|java|kt|ts|tsx|js|jsx|vue|sql|toml|yaml|yml|json|dockerfile))`)

func (s *Scanner) rebuildEvidenceGraph(repoID, now string) (*EvidenceGraphDiagnostics, error) {
	artifacts, err := s.loadEvidenceArtifacts(repoID)
	if err != nil {
		return nil, err
	}
	built := buildEvidenceGraph(repoID, artifacts)
	if err := s.db.ReplaceRepoEvidence(repoID, built.concepts, built.mentions, built.edges, now); err != nil {
		return nil, err
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
	artifactCount := len(artifacts)
	rawMentions := make([]rawConceptMention, 0, artifactCount*12)
	for _, artifact := range artifacts {
		rawMentions = append(rawMentions, extractEvidenceMentions(artifact)...)
	}

	conceptsByKey := map[string]*conceptAccumulator{}
	mentionSeen := map[string]bool{}
	for _, mention := range rawMentions {
		if mention.kind == "" || mention.canonical == "" || mention.artifactID == "" {
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
		acc.input.Forms = sortedMapKeys(acc.forms)
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

	mentionInputs := make([]store.ConceptMentionInput, 0, len(rawMentions))
	for _, mention := range rawMentions {
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
			EvidenceJSON: evidenceJSON(map[string]any{"form": mention.form, "source": mention.source, "canonical": mention.canonical, "kind": mention.kind}),
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

	edgeBuilder := newEvidenceEdgeBuilder(repoID)
	noisy := materializeSharedConceptEdges(artifacts, conceptsByKey, rawMentions, edgeBuilder)
	materializeLayoutGroupEdges(artifacts, edgeBuilder)
	materializeSameSourcePathEdges(artifacts, edgeBuilder)
	materializeLinkEdges(artifacts, edgeBuilder)
	materializePathReferenceEdges(artifacts, edgeBuilder)
	edges := edgeBuilder.edges()

	diagnostics := buildEvidenceDiagnostics(conceptInputs, mentionInputs, edges, noisy)
	return evidenceBuildResult{
		concepts:    conceptInputs,
		mentions:    mentionInputs,
		edges:       edges,
		diagnostics: diagnostics,
	}
}

func extractEvidenceMentions(artifact evidenceArtifact) []rawConceptMention {
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
	for _, section := range artifact.sections {
		addSectionMentions(section, add)
	}
	return mentions
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
	for _, symbol := range evidenceStringSlice(artifact.extracted["symbols"]) {
		compact := compactIdentifier(symbol)
		if compact != "" {
			add(conceptKindSymbol, compact, symbol, "symbol", "metadata", "", 1.0)
			add(conceptKindIdentifier, compact, symbol, "symbol", "metadata", "", 0.95)
		}
		for _, phrase := range phraseConcepts(symbol) {
			add(conceptKindPhrase, phrase, symbol, "symbol", "metadata", "", 0.8)
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
	if section.HeadingPath != "" {
		values = append(values, strings.Split(section.HeadingPath, ">")...)
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
		ev := edgeConceptEvidence{kind: input.Kind, canonical: input.Canonical, idf: input.InverseDocumentFrequency}
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				src, dst := orderedArtifactPair(ids[i], ids[j])
				pairKey := src + "\x00" + dst
				pair := pairs[pairKey]
				if pair == nil {
					pair = &pairEvidence{src: src, dst: dst}
					pairs[pairKey] = pair
				}
				if len(pair.concepts) < 8 {
					pair.concepts = append(pair.concepts, ev)
				}
			}
		}
	}
	pairList := make([]*pairEvidence, 0, len(pairs))
	for _, pair := range pairs {
		sort.Slice(pair.concepts, func(i, j int) bool {
			if pair.concepts[i].idf == pair.concepts[j].idf {
				if pair.concepts[i].kind == pair.concepts[j].kind {
					return pair.concepts[i].canonical < pair.concepts[j].canonical
				}
				return pair.concepts[i].kind < pair.concepts[j].kind
			}
			return pair.concepts[i].idf > pair.concepts[j].idf
		})
		pairList = append(pairList, pair)
	}
	sort.Slice(pairList, func(i, j int) bool {
		if len(pairList[i].concepts) == len(pairList[j].concepts) {
			if pairList[i].src == pairList[j].src {
				return pairList[i].dst < pairList[j].dst
			}
			return pairList[i].src < pairList[j].src
		}
		return len(pairList[i].concepts) > len(pairList[j].concepts)
	})
	if len(pairList) > maxSharedConceptEdges {
		pairList = pairList[:maxSharedConceptEdges]
	}
	for _, pair := range pairList {
		maxIDF := 0.0
		names := make([]string, 0, minInt(3, len(pair.concepts)))
		metaConcepts := make([]map[string]any, 0, minInt(5, len(pair.concepts)))
		for i, concept := range pair.concepts {
			if concept.idf > maxIDF {
				maxIDF = concept.idf
			}
			if i < 3 {
				names = append(names, concept.canonical)
			}
			if i < 5 {
				metaConcepts = append(metaConcepts, map[string]any{"kind": concept.kind, "canonical": concept.canonical, "idf": roundEvidence(concept.idf)})
			}
		}
		evidenceCount := len(pair.concepts)
		weight := 0.35 + float64(evidenceCount)*0.10 + maxIDF*0.05
		confidence := 0.55 + float64(evidenceCount)*0.08 + maxIDF*0.04
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
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				src, dst := orderedArtifactPair(ids[i], ids[j])
				builder.add(src, dst, edgeTypeSameLayoutGroup, "layout_group", 0.62, 0.72, 1, "same layout group "+group, map[string]any{"layout_group": group})
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
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				src, dst := orderedArtifactPair(ids[i], ids[j])
				builder.add(src, dst, edgeTypeSameFileOrLineVariant, "source_path", 0.68, 0.78, 1, "same source path "+path, map[string]any{"source_path": path})
			}
		}
	}
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
	for _, artifact := range artifacts {
		for _, ref := range extractPathReferences(artifact.body) {
			for _, targetID := range byPath[ref] {
				if targetID == artifact.id {
					continue
				}
				builder.add(artifact.id, targetID, edgeTypeExplicitReference, "path_reference", 0.78, 0.78, 1, "references path "+ref, map[string]any{"path": ref})
			}
		}
	}
}

func buildEvidenceDiagnostics(concepts []store.ConceptInput, mentions []store.ConceptMentionInput, edges []store.ArtifactEdgeInput, noisy []EvidenceConceptExample) *EvidenceGraphDiagnostics {
	d := &EvidenceGraphDiagnostics{
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
	for n := minInt(4, len(words)); n >= 2; n-- {
		for i := 0; i+n <= len(words); i++ {
			part := words[i : i+n]
			if allStopWords(part) {
				continue
			}
			out = append(out, strings.Join(part, " "))
		}
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

func sharedConceptEdgeKind(kind string) bool {
	switch kind {
	case conceptKindIdentifier, conceptKindCompactIdentifier, conceptKindPathFragment, conceptKindPhrase, conceptKindSymbol, conceptKindTestBehavior:
		return true
	default:
		return false
	}
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

func allStopWords(words []string) bool {
	if len(words) == 0 {
		return true
	}
	for _, word := range words {
		if !evidenceStopWords[word] {
			return false
		}
	}
	return true
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
