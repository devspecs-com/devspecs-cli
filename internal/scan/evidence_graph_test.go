package scan

import (
	"fmt"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		require.NotEqual(t, conceptKindArtifactRole, concept.Kind,
			"artifact role concept should not be persisted: %#v", concept)

	}
	for _, mention := range result.mentions {
		assert.NotContains(t, []string{
			"kind", "subtype", "status", "source_type", "format_profile",
			"language", "framework", "mode", "artifact_scope",
		}, mention.Field, "role metadata mention should not be persisted: %#v", mention)
	}
	require.Equal(t, 0, result.diagnostics.ConceptsByKind[conceptKindArtifactRole],
		"diagnostics should not count persisted artifact role concepts: %#v", result.diagnostics.ConceptsByKind)

}

func TestSharedConceptEdgesRejectGenericRareTerms(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceTestArtifact("a1", "Local Test Notes", "docs/a.md"),
		evidenceTestArtifact("a2", "Local Test Notes", "docs/b.md"),
	})
	{

		got := countEdgesByType(result.edges, edgeTypeMentionsSameConcept)
		require.Equal(t, 0, got,
			"generic rare terms should not create shared-concept edges, got %d edges: %#v", got, result.edges)
	}
	require.NotEqual(t, 0, result.diagnostics.NoisyConceptsSkipped,
		"expected skipped noisy concepts in diagnostics")

}

func TestSharedConceptEdgesCapWeakOnlyConfidence(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceTestArtifact("a1", "Alpha", "auth/a.md"),
		evidenceTestArtifact("a2", "Beta", "auth/b.md"),
	})

	edge := singleEdgeByType(t, result.edges, edgeTypeMentionsSameConcept)
	require.LessOrEqual(t, edge.Confidence, 0.84,
		"weak-only shared concept should not become high confidence: %.3f", edge.Confidence)

}

func TestSharedConceptEdgesAllowHighConfidenceStrongAnchors(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceTestArtifact("a1", "Refresh Token Rotation", "docs/auth-refresh.md"),
		evidenceTestArtifact("a2", "Refresh Token Rotation", "src/auth/refresh-token.ts"),
	})

	edge := singleEdgeByType(t, result.edges, edgeTypeMentionsSameConcept)
	require.GreaterOrEqual(t, edge.Confidence, 0.9,
		"strong shared anchors should retain high confidence, got %.3f", edge.Confidence)

}

func TestSharedConceptEdgesCapCompactOnlyConfidence(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceTestArtifact("a1", "Alpha", "docs/service-radar/work-items.md"),
		evidenceTestArtifact("a2", "Beta", "plans/service-radar/follow-up.md"),
	})

	edge := singleEdgeByType(t, result.edges, edgeTypeMentionsSameConcept)
	require.LessOrEqual(t, edge.Confidence, 0.84,
		"compact/path-only shared concepts should not become high confidence: %.3f", edge.Confidence)

}

func TestSharedConceptEdgesRejectTemplatePhrases(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceTestArtifact("a1", "BEP Your Short Descriptive Title", "beps/0001-topic.md"),
		evidenceTestArtifact("a2", "BEP Your Short Descriptive Title", "beps/NNNN-template.md"),
	})

	for _, edge := range result.edges {
		if edge.EdgeType == edgeTypeMentionsSameConcept {
			assert.LessOrEqual(t, edge.Confidence, 0.84)
		}

	}
}

func TestSharedConceptEdgesSkipConceptsWithTooManyArtifacts(t *testing.T) {
	var artifacts []evidenceArtifact
	for i := 0; i < maxSharedConceptArtifacts+1; i++ {
		artifacts = append(artifacts, evidenceTestArtifact(string(rune('a'+i)), "Refresh Token Rotation", "docs/path-"+string(rune('a'+i))+".md"))
	}
	result := buildEvidenceGraph("repo", artifacts)
	{

		got := countEdgesByType(result.edges, edgeTypeMentionsSameConcept)
		require.Equal(t, 0, got,
			"too-broad shared concept should not create edges, got %d", got)
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
	require.LessOrEqual(t, len(result.mentions), maxEvidenceMentionsPerArtifact,
		"dense artifact should be capped at %d mentions, got %d", maxEvidenceMentionsPerArtifact, len(result.mentions))
	require.True(t, hasMentionField(result.mentions, "path"),
		"path anchors should survive dense-artifact cap")
	require.True(t, hasMentionField(result.mentions, "title"),
		"title anchors should survive dense-artifact cap")

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
	require.Len(t, limited, maxEvidenceMentionsPerRepo,
		"dense repo should be capped at %d mentions, got %d", maxEvidenceMentionsPerRepo, len(limited))

	byArtifact := map[string]int{}
	for _, mention := range limited {
		byArtifact[mention.artifactID]++
	}
	for artifact := 0; artifact < 20; artifact++ {
		id := fmt.Sprintf("a%02d", artifact)
		require.GreaterOrEqual(t, byArtifact[id], minEvidenceMentionsPerArtifact,
			"artifact %s should retain at least %d mentions, got %d", id, minEvidenceMentionsPerArtifact, byArtifact[id])

	}
}

func TestCompactMentionEvidenceJSON_WithSourcePath_MatchesMapEncoding(t *testing.T) {
	mention := rawConceptMention{source: "source_path"}

	got := compactMentionEvidenceJSON(mention)

	assert.Equal(t, evidenceJSON(compactMentionEvidence(mention)), got)
}

func TestCompactMentionEvidenceJSON_WithMetadataForm_MatchesMapEncoding(t *testing.T) {
	mention := rawConceptMention{source: "metadata", form: "Swagger OAuth redirect"}

	got := compactMentionEvidenceJSON(mention)

	assert.Equal(t, evidenceJSON(compactMentionEvidence(mention)), got)
}

func TestCompactMentionEvidenceJSON_WithEscapedSectionForm_MatchesMapEncoding(t *testing.T) {
	mention := rawConceptMention{source: "section", form: `quotes " slash \ html <tag> & more`}

	got := compactMentionEvidenceJSON(mention)

	assert.Equal(t, evidenceJSON(compactMentionEvidence(mention)), got)
}

func TestCompactMentionEvidenceJSON_WithLongForm_MatchesMapEncoding(t *testing.T) {
	mention := rawConceptMention{source: "metadata", form: strings.Repeat("a", maxMentionEvidenceFormLength+8)}

	got := compactMentionEvidenceJSON(mention)

	assert.Equal(t, evidenceJSON(compactMentionEvidence(mention)), got)
}

func TestTestSourceTriangulationExactStem(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceSourceArtifact("src_webhooks", "src/billing/webhooks.ts", "export function handleWebhook() { return true }\n"),
		evidenceTestCaseArtifact("test_webhooks", "src/billing/webhooks.test.ts", "handles webhook retries", "", nil),
	})

	edge := singleEdgeByType(t, result.edges, edgeTypeTestsSource)
	assert.Equal(t, "test_webhooks", edge.SrcArtifactID)
	assert.Equal(t, "src_webhooks", edge.DstArtifactID)
	assert.Equal(t, "test_source_stem", edge.SourceSignal)
	assert.GreaterOrEqual(t, edge.Confidence, 0.9)
}

func TestTestSourceTriangulationDirectImport(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceSourceArtifact("src_publish", "scripts/publish_keys.py", "def publish_keys():\n    return True\n"),
		evidenceTestCaseArtifact("test_publish", "tests/test_publish_keys.py", "test publish keys", "from scripts.publish_keys import publish_keys\n", nil),
	})

	edge := singleEdgeByType(t, result.edges, edgeTypeTestsSource)
	require.Equal(t, "direct_import", edge.SourceSignal,
		"expected direct import signal, got %q: %#v", edge.SourceSignal, edge)
	require.GreaterOrEqual(t, edge.Confidence, 0.9,
		"direct import should be high confidence, got %.3f", edge.Confidence)

}

func TestTestSourceTriangulationAvoidsSiblingOnly(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceSourceArtifact("src_tokens", "src/auth/tokens.ts", "export function rotateToken() { return true }\n"),
		evidenceTestCaseArtifact("test_session", "src/auth/session.test.ts", "handles session expiry", "", nil),
	})
	{

		got := countEdgesByType(result.edges, edgeTypeTestsSource)
		require.Equal(t, 0, got,
			"same-directory sibling-only tests should not create tests_source edges, got %d: %#v", got, result.edges)
	}

}

func TestTestSourceTriangulationSymbolMatch(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceSourceArtifact("src_refresh", "src/auth/token_service.ts", "export class RefreshTokenService {}\n"),
		evidenceTestCaseArtifact("test_refresh", "src/auth/session_behavior.test.ts", "refresh token behavior", "", []string{"RefreshTokenService"}),
	})

	edge := singleEdgeByType(t, result.edges, edgeTypeTestsSource)
	require.Equal(t, "source_symbol_match", edge.SourceSignal,
		"expected symbol-match signal, got %q: %#v", edge.SourceSignal, edge)
	{

		got := countEdgesByType(result.edges, edgeTypeMentionsSymbol)
		require.Equal(t, 1, got,
			"symbol match should emit one mentions_symbol edge, got %d: %#v", got, result.edges)
	}

}

func TestTestSourceTriangulationPythonSymbolMatch(t *testing.T) {
	source := evidenceSourceArtifact("src_users", "app/users.py", "def create_user():\n    return True\n")
	source.extracted["language"] = "python"
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		source,
		evidenceTestCaseArtifact("test_users", "tests/test_users.py", "test create user", "", []string{"create_user"}),
	})

	edge := singleEdgeByType(t, result.edges, edgeTypeTestsSource)
	assert.Equal(t, "test_source_stem", edge.SourceSignal)
	assert.Contains(t, edge.MetadataJSON, "create_user")
	{

		got := countEdgesByType(result.edges, edgeTypeMentionsSymbol)
		require.Equal(t, 1, got,
			"python symbol match should emit one mentions_symbol edge, got %d: %#v", got, result.edges)
	}

}

func TestRichTypedIndexAddsSourceSymbolMentions(t *testing.T) {
	result := buildEvidenceGraphWithOptions("repo", []evidenceArtifact{
		evidenceSourceArtifact("src_refresh", "src/auth/token_service.ts", "export class RefreshTokenService {}\n"),
	}, evidenceGraphBuildOptions{RichTypedIndex: true})
	assert.True(t, hasConcept(result.concepts, conceptKindSymbol, "refreshtokenservice"))
	require.NotNil(t, result.diagnostics)
	assert.True(t, result.diagnostics.RichTypedIndex)
}

func TestDefaultTypedIndexDoesNotAddSourceSymbolMentions(t *testing.T) {
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceSourceArtifact("src_refresh", "src/auth/token_service.ts", "export class RefreshTokenService {}\n"),
	})

	assert.False(t, hasConcept(result.concepts, conceptKindSymbol, "refreshtokenservice"))
}

func TestRichTypedIndexResolvesRelativeImportPathForGenericIndex(t *testing.T) {
	artifacts := []evidenceArtifact{
		evidenceSourceArtifact("src_index", "src/auth/index.ts", "export function createAuth() { return true }\n"),
		evidenceTestCaseArtifact("test_index", "src/auth/index.test.ts", "loads auth index", "import { createAuth } from './index'\n", nil),
	}

	result := buildEvidenceGraphWithOptions("repo", artifacts, evidenceGraphBuildOptions{RichTypedIndex: true})
	edge := singleEdgeByType(t, result.edges, edgeTypeTestsSource)
	assert.Equal(t, "relative_import_path", edge.SourceSignal)
	assert.GreaterOrEqual(t, edge.Confidence, 0.95)
}

func TestDefaultTypedIndexDoesNotResolveRelativeImportPathForGenericIndex(t *testing.T) {
	artifacts := []evidenceArtifact{
		evidenceSourceArtifact("src_index", "src/auth/index.ts", "export function createAuth() { return true }\n"),
		evidenceTestCaseArtifact("test_index", "src/auth/index.test.ts", "loads auth index", "import { createAuth } from './index'\n", nil),
	}

	result := buildEvidenceGraph("repo", artifacts)

	assert.Zero(t, countEdgesByType(result.edges, edgeTypeTestsSource))
}

func TestRichTypedIndexAddsSymbolReferenceEdge(t *testing.T) {
	doc := evidenceTestArtifact("doc_refresh", "Token Refresh Plan", "docs/auth-refresh.md")
	doc.body = "Wire the retry behavior through RefreshTokenService before updating the runbook."
	result := buildEvidenceGraphWithOptions("repo", []evidenceArtifact{
		evidenceSourceArtifact("src_refresh", "src/auth/token_service.ts", "export class RefreshTokenService {}\n"),
		doc,
	}, evidenceGraphBuildOptions{RichTypedIndex: true})

	edge := singleEdgeByType(t, result.edges, edgeTypeMentionsSymbol)
	assert.Equal(t, "symbol_reference", edge.SourceSignal)
	assert.Equal(t, "doc_refresh", edge.SrcArtifactID)
	assert.Equal(t, "src_refresh", edge.DstArtifactID)
}

func TestDefaultTypedIndexDoesNotAddSymbolReferenceEdge(t *testing.T) {
	doc := evidenceTestArtifact("doc_refresh", "Token Refresh Plan", "docs/auth-refresh.md")
	doc.body = "Wire the retry behavior through RefreshTokenService before updating the runbook."
	result := buildEvidenceGraph("repo", []evidenceArtifact{
		evidenceSourceArtifact("src_refresh", "src/auth/token_service.ts", "export class RefreshTokenService {}\n"),
		doc,
	})

	assert.Zero(t, countEdgesByType(result.edges, edgeTypeMentionsSymbol))
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

func evidenceSourceArtifact(id, path, body string) evidenceArtifact {
	return evidenceArtifact{
		id:        id,
		repoID:    "repo",
		kind:      "source_context",
		title:     path,
		status:    "unknown",
		body:      body,
		extracted: map[string]any{"language": "typescript"},
		sources: []store.SourceRow{{
			SourceType:     "source_context",
			Path:           path,
			SourceIdentity: path + "|source_context",
			FormatProfile:  "generic",
		}},
	}
}

func evidenceTestCaseArtifact(id, path, name, body string, symbols []string) evidenceArtifact {
	return evidenceArtifact{
		id:      id,
		repoID:  "repo",
		kind:    "source_context",
		subtype: "test_case",
		title:   name,
		status:  "unknown",
		body:    body,
		extracted: map[string]any{
			"subtype":     "test_case",
			"language":    "typescript",
			"source_path": path,
			"test_name":   name,
			"symbols":     symbols,
		},
		sources: []store.SourceRow{{
			SourceType:     "test_case",
			Path:           path,
			SourceIdentity: path + "|test_case|1|" + name,
			FormatProfile:  "generic",
			LayoutGroup:    path,
		}},
	}
}

func hasConcept(concepts []store.ConceptInput, kind, canonical string) bool {
	for _, concept := range concepts {
		if concept.Kind == kind && concept.Canonical == canonical {
			return true
		}
	}
	return false
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
	require.Len(t, out, 1,
		"expected one %s edge, got %d: %#v", edgeType, len(out), out)

	return out[0]
}
