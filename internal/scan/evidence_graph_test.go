package scan

import (
	"fmt"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestBuildEvidenceGraphSkipsArtifactRoleMentions(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{{
		id:      "a1",
		repoID:  "repo",
		kind:    "markdown_artifact",
		subtype: "agent_instruction",
		title:   "Auth Token Runbook",
		status:  "unknown",
		extracted: map[string]any{
			"language":       "go",
			"framework":      "go_test",
			"mode":           "intent",
			"artifact_scope": "section",
		},
		sources: []store.SourceRow{{
			SourceType:    "markdown",
			Path:          "docs/auth-token-runbook.md",
			FormatProfile: "generic",
		}},
	}})

	for _, concept := range result.concepts {
		if concept.Kind == conceptKindArtifactRole {
			t.Fatalf("artifact role concept should not be persisted: %#v", concept)
		}
	}
	for _, mention := range result.mentions {
		switch mention.Field {
		case "kind", "subtype", "status", "source_type", "format_profile", "language", "framework", "mode", "artifact_scope":
			t.Fatalf("role metadata mention should not be persisted: %#v", mention)
		}
	}
	if result.diagnostics.ConceptsByKind[conceptKindArtifactRole] != 0 {
		t.Fatalf("diagnostics should not count persisted artifact role concepts: %#v", result.diagnostics.ConceptsByKind)
	}
}

func TestSharedConceptEdgesRejectGenericRareTerms(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceTestArtifact("a1", "Local Test Notes", "docs/a.md"),
		evidenceTestArtifact("a2", "Local Test Notes", "docs/b.md"),
	})

	if got := countEdgesByType(result.edges, edgeTypeMentionsSameConcept); got != 0 {
		t.Fatalf("generic rare terms should not create shared-concept edges, got %d edges: %#v", got, result.edges)
	}
	if result.diagnostics.NoisyConceptsSkipped == 0 {
		t.Fatalf("expected skipped noisy concepts in diagnostics")
	}
}

func TestSharedConceptEdgesCapWeakOnlyConfidence(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceTestArtifact("a1", "Alpha", "auth/a.md"),
		evidenceTestArtifact("a2", "Beta", "auth/b.md"),
	})

	edge := singleEdgeByType(t, result.edges, edgeTypeMentionsSameConcept)
	if edge.Confidence > 0.84 {
		t.Fatalf("weak-only shared concept should not become high confidence: %.3f", edge.Confidence)
	}
}

func TestSharedConceptEdgesAllowHighConfidenceStrongAnchors(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceTestArtifact("a1", "Refresh Token Rotation", "docs/auth-refresh.md"),
		evidenceTestArtifact("a2", "Refresh Token Rotation", "src/auth/refresh-token.ts"),
	})

	edge := singleEdgeByType(t, result.edges, edgeTypeMentionsSameConcept)
	if edge.Confidence < 0.9 {
		t.Fatalf("strong shared anchors should retain high confidence, got %.3f", edge.Confidence)
	}
}

func TestSharedConceptEdgesCapCompactOnlyConfidence(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceTestArtifact("a1", "Alpha", "docs/service-radar/work-items.md"),
		evidenceTestArtifact("a2", "Beta", "plans/service-radar/follow-up.md"),
	})

	edge := singleEdgeByType(t, result.edges, edgeTypeMentionsSameConcept)
	if edge.Confidence > 0.84 {
		t.Fatalf("compact/path-only shared concepts should not become high confidence: %.3f", edge.Confidence)
	}
}

func TestSharedConceptEdgesRejectTemplatePhrases(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceTestArtifact("a1", "BEP Your Short Descriptive Title", "beps/0001-topic.md"),
		evidenceTestArtifact("a2", "BEP Your Short Descriptive Title", "beps/NNNN-template.md"),
	})

	for _, edge := range result.edges {
		if edge.EdgeType == edgeTypeMentionsSameConcept && edge.Confidence > 0.84 {
			t.Fatalf("template phrases should not create high-confidence edges: %#v", edge)
		}
	}
}

func TestSharedConceptEdgesSkipConceptsWithTooManyArtifacts(t *testing.T) {
	var artifacts []evidenceArtifact
	for i := 0; i < maxSharedConceptArtifacts+1; i++ {
		artifacts = append(artifacts, evidenceTestArtifact(string(rune('a'+i)), "Refresh Token Rotation", "docs/path-"+string(rune('a'+i))+".md"))
	}
	result := buildEvidenceGraph("repo", artifacts)

	if got := countEdgesByType(result.edges, edgeTypeMentionsSameConcept); got != 0 {
		t.Fatalf("too-broad shared concept should not create edges, got %d", got)
	}
}

func TestEvidenceMentionBudgetCapsDenseArtifacts(t *testing.T) {
	artifact := evidenceTestArtifact("a1", "Refresh Token Rotation", "docs/auth-refresh-token.md")
	for i := 0; i < maxEvidenceMentionsPerArtifact*3; i++ {
		artifact.sections = append(artifact.sections, store.SectionRow{
			ID:    fmt.Sprintf("s%d", i),
			Title: fmt.Sprintf("Unique Behavior Anchor %03d", i),
		})
	}

	result := buildEvidenceGraph("repo", []evidenceArtifact{artifact})

	if len(result.mentions) > maxEvidenceMentionsPerArtifact {
		t.Fatalf("dense artifact should be capped at %d mentions, got %d", maxEvidenceMentionsPerArtifact, len(result.mentions))
	}
	if !hasMentionField(result.mentions, "path") {
		t.Fatalf("path anchors should survive dense-artifact cap")
	}
	if !hasMentionField(result.mentions, "title") {
		t.Fatalf("title anchors should survive dense-artifact cap")
	}
}

func TestEvidenceMentionBudgetCapsDenseRepos(t *testing.T) {
	var mentions []rawConceptMention
	for artifact := 0; artifact < 20; artifact++ {
		for i := 0; i < (maxEvidenceMentionsPerRepo/20)+500; i++ {
			mentions = append(mentions, rawConceptMention{
				kind:       conceptKindPhrase,
				canonical:  fmt.Sprintf("concept-%02d-%05d", artifact, i),
				form:       fmt.Sprintf("Concept %02d %05d", artifact, i),
				artifactID: fmt.Sprintf("a%02d", artifact),
				field:      "heading",
				weight:     0.75,
			})
		}
	}

	limited := limitRepoEvidenceMentions(mentions)

	if len(limited) != maxEvidenceMentionsPerRepo {
		t.Fatalf("dense repo should be capped at %d mentions, got %d", maxEvidenceMentionsPerRepo, len(limited))
	}
	byArtifact := map[string]int{}
	for _, mention := range limited {
		byArtifact[mention.artifactID]++
	}
	for artifact := 0; artifact < 20; artifact++ {
		id := fmt.Sprintf("a%02d", artifact)
		if byArtifact[id] < minEvidenceMentionsPerArtifact {
			t.Fatalf("artifact %s should retain at least %d mentions, got %d", id, minEvidenceMentionsPerArtifact, byArtifact[id])
		}
	}
}

func evidenceTestArtifact(id, title, path string) evidenceArtifact {
	return evidenceArtifact{
		id:        id,
		repoID:    "repo",
		kind:      "markdown_artifact",
		title:     title,
		status:    "unknown",
		extracted: map[string]any{},
		sources: []store.SourceRow{{
			SourceType:    "markdown",
			Path:          path,
			FormatProfile: "generic",
		}},
	}
}

func hasMentionField(mentions []store.ConceptMentionInput, field string) bool {
	for _, mention := range mentions {
		if mention.Field == field {
			return true
		}
	}
	return false
}

func countEdgesByType(edges []store.ArtifactEdgeInput, edgeType string) int {
	count := 0
	for _, edge := range edges {
		if edge.EdgeType == edgeType {
			count++
		}
	}
	return count
}

func singleEdgeByType(t *testing.T, edges []store.ArtifactEdgeInput, edgeType string) store.ArtifactEdgeInput {
	t.Helper()
	var out []store.ArtifactEdgeInput
	for _, edge := range edges {
		if edge.EdgeType == edgeType {
			out = append(out, edge)
		}
	}
	if len(out) != 1 {
		t.Fatalf("expected one %s edge, got %d: %#v", edgeType, len(out), out)
	}
	return out[0]
}
