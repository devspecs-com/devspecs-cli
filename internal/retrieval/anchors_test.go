package retrieval

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidAnchorFirstModes_ReturnsEverySupportedModeInStableOrder(t *testing.T) {
	modes := ValidAnchorFirstModes()

	require.Len(t, modes, 8)
	assert.Equal(t, AnchorFirstModeV1, modes[0])
	assert.Equal(t, AnchorFirstModeRerankOnly, modes[1])
	assert.Equal(t, AnchorFirstModeSelectedOnly, modes[2])
	assert.Equal(t, AnchorFirstModeStrongField, modes[3])
	assert.Equal(t, AnchorFirstModeStrict, modes[4])
	assert.Equal(t, AnchorFirstModeCodeTask, modes[5])
	assert.Equal(t, AnchorFirstModeCodeTaskFamily, modes[6])
	assert.Equal(t, AnchorFirstModeCodeTaskFamilyV2, modes[7])
}

func TestBuildAnchorProfileClassifiesQueryTerms(t *testing.T) {
	profile := BuildAnchorProfile("fix Langfuse trace association for REQ_fluxnova_aigf_integration and testPutAndGetExposedTool behavior")

	assertAnchorTerm(t, profile, "langfuse", AnchorProperOrRare)
	assertAnchorTerm(t, profile, "req_fluxnova_aigf_integration", AnchorPathLike)
	assertAnchorTerm(t, profile, "reqfluxnovaaigfintegration", AnchorCompactIdentifier)
	assertAnchorTerm(t, profile, "testputandgetexposedtool", AnchorCompactIdentifier)
	assertAnchorTerm(t, profile, "behavior", AnchorGenericTaskWord)
	assertAnchorTerm(t, profile, "fix", AnchorGenericTaskWord)
}

func TestRepoVocabularyUsesStrongFieldEvidenceAndIDF(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md",
			Title: "Requirements Spec: FluxNova Templates + AI Governance Framework Integration",
			Kind:  "requirements",
		},
		{
			Path:  "advent-of-calm/day-14.md",
			Title: "Use CALM as Your Expert Architecture Advisor",
			Body:  "CALM studio architecture guide.",
		},
		{
			Path:  "calm-ai/tools/documentation-creation.md",
			Title: "CALM Documentation Creation Guide",
			Body:  "CALM documentation and architecture guide.",
		},
	}

	vocab := BuildRepoVocabulary(append(candidates, anchorFillerCandidatesForTest()...))
	flux := vocab.Terms["fluxnova"]
	calm := vocab.Terms["calm"]
	require.NotEqual(t, 0, flux.DocumentCount,
		"expected fluxnova term stats")
	require.Greater(t, flux.IDF, calm.IDF,
		"rare term should have higher IDF: flux %.3f calm %.3f", flux.IDF, calm.IDF)
	assert.Positive(t, flux.PathCount)
	assert.Positive(t, flux.TitleCount)

}

func TestAnchorFirstRankingPromotesRareNamedRequirement(t *testing.T) {
	candidates := []Candidate{
		{
			Path:  "advent-of-calm/day-14.md",
			Title: "Day 14: Use CALM as Your Expert Architecture Advisor",
			Kind:  "markdown_artifact",
			Body:  "CALM Studio integration requirements architecture guide. " + repeatForTest("CALM ", 20),
		},
		{
			Path:    "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md",
			Title:   "Requirements Spec: FluxNova Templates + AI Governance Framework Integration",
			Kind:    "requirements",
			Subtype: "prd",
			Body:    "FluxNova AIGF integration requirements for CALM Studio.",
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true}).Retrieve(candidates, "FluxNova AIGF integration requirements for CALM Studio")
	require.NotEmpty(t, got)
	assert.Equal(t, "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md", got[0].Path)
	require.NotEqual(t, "", got[0].Metadata["anchor_first_score"],
		"missing anchor metadata: %#v", got[0].Metadata)

}

func TestAnchorFirstSelectedOnlyPreservesDefaultSelectedSet(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md",
			Title:   "Requirements Spec: FluxNova Templates + AI Governance Framework Integration",
			Kind:    "requirements",
			Subtype: "prd",
			Body:    "FluxNova AIGF integration requirements for CALM Studio.",
		},
	}
	for i := 0; i < 10; i++ {
		candidates = append(candidates, Candidate{
			Path:  fmt.Sprintf("docs/reference/filler-%02d.md", i),
			Title: "Reference Notes",
			Body:  "FluxNova AIGF integration requirements for CALM Studio. " + repeatForTest("integration requirements ", 4),
		})
	}

	query := "FluxNova AIGF integration requirements for CALM Studio"
	base := (WeightedFilesRetrieverV0{}).Retrieve(candidates, query)
	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true, AnchorFirstMode: AnchorFirstModeSelectedOnly}).Retrieve(candidates, query)
	require.True(t, sameCandidatePathSetForTest(base, got),
		"selected_only changed selected set: base %#v got %#v", CandidatePaths(base), CandidatePaths(got))

	var target *Candidate
	for index := range got {
		if got[index].Path == "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md" {
			target = &got[index]
			break
		}
	}
	require.NotNil(t, target, "missing target candidate: %#v", CandidatePaths(got))
	assert.Equal(t, AnchorFirstModeSelectedOnly, target.Metadata["anchor_first_mode"])
}

func TestAnchorFirstRankingWithRoleOnlyTemplateQueryDoesNotPromoteNamedTemplate(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    ".github/ISSUE_TEMPLATE/Project_proposal.md",
			Title:   "Project Proposal",
			Subtype: "issue_template",
			Body:    repeatForTest("template architecture project proposal ", 10),
		},
		{
			Path:    "docs/static/calm-template/solution-architecture-document.md",
			Title:   "Solution Architecture Document",
			Subtype: "document_template",
			Body:    "Solution architecture document template for trading system SAD examples.",
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true}).Retrieve(candidates, "template")

	if len(got) > 0 && got[0].Path == "docs/static/calm-template/solution-architecture-document.md" {
		assert.Empty(t, got[0].Metadata["anchor_first_score"])
	}
}

func TestAnchorFirstRankingWithNamedTemplateQueryPromotesNamedTemplate(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    ".github/ISSUE_TEMPLATE/Project_proposal.md",
			Title:   "Project Proposal",
			Subtype: "issue_template",
			Body:    repeatForTest("template architecture project proposal ", 10),
		},
		{
			Path:    "docs/static/calm-template/solution-architecture-document.md",
			Title:   "Solution Architecture Document",
			Subtype: "document_template",
			Body:    "Solution architecture document template for trading system SAD examples.",
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true}).Retrieve(candidates, "solution architecture document template and trading system SAD example")

	require.NotEmpty(t, got)
	assert.Equal(t, "docs/static/calm-template/solution-architecture-document.md", got[0].Path)
}

func TestAnchorFirstRankingKeepsExactTestNameFirst(t *testing.T) {
	candidates := []Candidate{
		{
			Path:    "components/camel-ai/tool_test.java#L53",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "CamelToolExecutorCacheTest > testPutAndGetExposedTool",
			Body:    "Test: testPutAndGetExposedTool\nAssertion vocabulary: assert equals contains",
			Metadata: map[string]string{
				"source_type": "test_case",
				"test_name":   "testPutAndGetExposedTool",
			},
		},
		{
			Path:    "components/camel-whatsapp/webhook_test.java#L90",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "WhatsAppWebhookTest > testWebhookRegistration",
			Body:    "Test: testWebhookRegistration",
			Metadata: map[string]string{
				"source_type": "test_case",
				"test_name":   "testWebhookRegistration",
			},
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true}).Retrieve(candidates, "what tests cover testPutAndGetExposedTool behavior")
	require.NotEmpty(t, got)
	assert.Equal(t, "components/camel-ai/tool_test.java#L53", got[0].Path)

}

func TestExactAnchorDisciplinePrefersSpecificAnchorOverGenericTerm(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "regex_literal",
			Path:  "crates/regex/src/literal.rs",
			Kind:  "source_context",
			Title: "crates/regex/src/literal.rs (rust)",
			Body:  "regex literal extraction and regex engine helpers",
		},
		{
			ID:    "pcre2_matcher",
			Path:  "crates/pcre2/src/matcher.rs",
			Kind:  "source_context",
			Title: "crates/pcre2/src/matcher.rs (rust)",
			Body:  "An implementation of the Matcher trait using PCRE2 as the regex engine.",
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true, AnchorFirstMode: DefaultAnchorFirstMode}).Retrieve(candidates, "pcre2 regex engine")
	require.GreaterOrEqual(t, len(got), 2,
		"got %d results, want at least 2: %#v", len(got), got)
	require.Equal(t, "crates/pcre2/src/matcher.rs", got[0].Path,
		"first result = %q, want pcre2 matcher; all=%#v", got[0].Path, CandidatePaths(got))
	require.NotEqual(t, "", got[0].Metadata["exact_anchor_score"],
		"expected exact anchor metadata on pcre2 result: %#v", got[0].Metadata)

}

func TestExactAnchorDisciplineBackfillsBodyExactAnchor(t *testing.T) {
	candidates := []Candidate{
		{
			ID:    "header_stream",
			Path:  "lib/helpers/ZlibHeaderTransformStream.js",
			Kind:  "source_context",
			Title: "lib/helpers/ZlibHeaderTransformStream.js (javascript)",
			Body:  "header transform helper",
		},
		{
			ID:    "resolve_config",
			Path:  "lib/helpers/resolveConfig.js",
			Kind:  "source_context",
			Title: "lib/helpers/resolveConfig.js (javascript)",
			Body:  "const xsrfHeaderName = own('xsrfHeaderName'); const xsrfCookieName = own('xsrfCookieName'); const withXSRFToken = own('withXSRFToken'); headers.set(xsrfHeaderName, xsrfValue);",
		},
	}

	got := (WeightedFilesRetrieverV0{AnchorFirstRanking: true, AnchorFirstMode: DefaultAnchorFirstMode}).Retrieve(candidates, "xsrf csrf cookie header")
	require.NotEmpty(t, got,
		"expected results")
	require.Equal(t, "lib/helpers/resolveConfig.js", got[0].Path,
		"first result = %q, want resolveConfig; all=%#v", got[0].Path, CandidatePaths(got))

}

func TestAnchorFirstRerankOnlyDoesNotBackfill(t *testing.T) {
	selected := []scoredCandidate{{
		candidate: Candidate{Path: "docs/generic.md", Title: "Generic Architecture Notes", Body: "Architecture background."},
		score:     5,
	}}
	universe := []Candidate{
		selected[0].candidate,
		{
			Path:  "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md",
			Title: "Requirements Spec: FluxNova Templates + AI Governance Framework Integration",
		},
	}

	got := applyAnchorFirstRanking(selected, universe, "FluxNova AIGF integration requirements for CALM Studio", AnchorFirstModeRerankOnly)
	require.Len(t, got, 1,
		"rerank_only should not backfill candidates, got %#v", CandidatePathsFromScoredForTest(got))
	require.Equal(t, "", got[0].candidate.Metadata["anchor_first_backfill"],
		"rerank_only should not mark backfill: %#v", got[0].candidate.Metadata)

}

func TestAnchorFirstStrongFieldBackfillWithBodyOnlyMatchDoesNotBackfill(t *testing.T) {
	selected := []scoredCandidate{{
		candidate: Candidate{Path: "docs/generic.md", Title: "Generic Architecture Notes", Body: "Architecture background."},
		score:     5,
	}}
	bodyOnly := Candidate{
		Path:  "docs/background.md",
		Title: "Background",
		Body:  "FluxNova AIGF integration requirements appear only in body text.",
	}

	got := applyAnchorFirstRanking(selected, append([]Candidate{selected[0].candidate, bodyOnly}, anchorFillerCandidatesForTest()...), "FluxNova AIGF integration requirements for CALM Studio", AnchorFirstModeStrongField)

	require.Len(t, got, 1)
}

func TestAnchorFirstStrongFieldBackfillWithStrongMatchBackfillsFirst(t *testing.T) {
	selected := []scoredCandidate{{
		candidate: Candidate{Path: "docs/generic.md", Title: "Generic Architecture Notes", Body: "Architecture background."},
		score:     5,
	}}
	strongField := Candidate{
		Path:  "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md",
		Title: "Requirements Spec: FluxNova Templates + AI Governance Framework Integration",
	}

	got := applyAnchorFirstRanking(selected, append([]Candidate{selected[0].candidate, strongField}, anchorFillerCandidatesForTest()...), "FluxNova AIGF integration requirements for CALM Studio", AnchorFirstModeStrongField)

	require.Len(t, got, 2)
	assert.Equal(t, strongField.Path, got[0].candidate.Path)
	assert.Equal(t, "true", got[0].candidate.Metadata["anchor_first_backfill"])
}

func TestAnchorFirstStrictBackfillDoesNotBackfillProperTermOnlyMatch(t *testing.T) {
	selected := []scoredCandidate{{
		candidate: Candidate{Path: "docs/generic.md", Title: "Generic Architecture Notes", Body: "Architecture background."},
		score:     5,
	}}
	candidate := Candidate{
		Path:  "calm-suite/calm-studio/docs/fluxnova-integration.md",
		Title: "FluxNova Integration",
	}

	got := applyAnchorFirstRanking(selected, append([]Candidate{selected[0].candidate, candidate}, anchorFillerCandidatesForTest()...), "fluxnova integration requirements", AnchorFirstModeStrict)

	assert.Len(t, got, 1,
		"strict should not backfill proper-term-only matches, got %#v", CandidatePathsFromScoredForTest(got))
}

func TestAnchorFirstStrictBackfillBackfillsExactPathLikeAnchor(t *testing.T) {
	selected := []scoredCandidate{{
		candidate: Candidate{Path: "docs/generic.md", Title: "Generic Architecture Notes", Body: "Architecture background."},
		score:     5,
	}}
	candidate := Candidate{
		Path:  "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md",
		Title: "Requirements Spec: FluxNova Templates + AI Governance Framework Integration",
	}

	got := applyAnchorFirstRanking(selected, append([]Candidate{selected[0].candidate, candidate}, anchorFillerCandidatesForTest()...), "REQ_fluxnova_aigf_integration requirements", AnchorFirstModeStrict)

	assert.Len(t, got, 2,
		"strict should backfill exact path-like anchors, got %#v", CandidatePathsFromScoredForTest(got))
}

func TestAnchorFirstV1DoesNotBoostGenericBodyHeadingOnlyMatch(t *testing.T) {
	query := "RFC transaction mechanism on key value store using MVCC and commit marks"
	candidate := Candidate{
		Path:  "design-docs/gravitino-logical-view-management.md",
		Title: "Design of Logical View Management",
		Body:  "This design discusses mechanisms, values, and storage details for views.",
		Sections: []IndexedSection{{
			HeadingPath: "Proposal > View Metadata Storage > Two View Storage Mechanisms",
			Title:       "Two View Storage Mechanisms",
		}},
	}
	profile := BuildAnchorProfile(query)
	vocab := BuildRepoVocabulary(append([]Candidate{candidate}, anchorFillerCandidatesForTest()...))
	result := scoreAnchorFirstCandidate(candidate, profile, vocab, AnchorFirstModeV1)

	assert.False(t, anchorFirstPrimaryBoostAllowed(candidate, result, profile, AnchorFirstModeV1),
		"generic heading/body-only match should not receive anchor-first boost: %#v", result)
}

func TestAnchorFirstV1BoostsStrongPathAndTitleMatch(t *testing.T) {
	query := "RFC transaction mechanism on key value store using MVCC and commit marks"
	candidate := Candidate{
		Path:  "rfc/rfc-3/Transaction-implementation-on-kv.md",
		Title: "RFC transaction mechanism on key-value store",
		Body:  "MVCC commit marks transaction design.",
	}
	profile := BuildAnchorProfile(query)
	vocab := BuildRepoVocabulary(append([]Candidate{candidate}, anchorFillerCandidatesForTest()...))
	result := scoreAnchorFirstCandidate(candidate, profile, vocab, AnchorFirstModeV1)

	assert.True(t, anchorFirstPrimaryBoostAllowed(candidate, result, profile, AnchorFirstModeV1),
		"path/title RFC anchor should remain boostable: %#v", result)
}

func TestAnchorFirstDoesNotBoostDifferentAreaAgentInstructionsForArchitectureQuery(t *testing.T) {
	query := "DolphinScheduler architecture design for master worker api alert and dao modules"
	c := Candidate{
		Path:  "dolphinscheduler-alert/CLAUDE.md",
		Title: "CLAUDE.md - dolphinscheduler-alert",
		Body:  "Sub-modules include master worker api alert modules and design notes.",
	}
	profile := BuildAnchorProfile(query)
	require.False(t, anchorFirstCandidateEligible(c, profile),
		"architecture query should not primary-promote different-area agent instruction files")

}

func TestAnchorFirstAllowsAgentInstructionsForProtocolQuery(t *testing.T) {
	query := "Claude skill instructions for frontend design command"
	c := Candidate{
		Path:  ".claude/skills/frontend-design/SKILL.md",
		Title: "Frontend Design Skill",
		Body:  "Instructions for design commands.",
	}
	profile := BuildAnchorProfile(query)
	require.True(t, anchorFirstCandidateEligible(c, profile),
		"agent/protocol query should allow agent instruction candidates")

}

func TestAnchorFirstKeepsPromptPlanPathAnchorsEligible(t *testing.T) {
	query := "engineering context for mode active critical you"
	c := Candidate{
		Path:    "packages/coding-agent/src/prompts/system/plan-mode-active.md",
		Title:   "Plan Mode Active",
		Subtype: "agent_instruction",
		Body:    "Critical files for implementation.",
	}
	profile := BuildAnchorProfile(query)

	assert.True(t, anchorFirstCandidateEligible(c, profile),
		"path/title prompt plan anchors should remain eligible even when classifier subtype is protocol-like")

}

func TestAnchorFirstKeepsPromptPlanPathAnchorsBoostable(t *testing.T) {
	query := "engineering context for mode active critical you"
	c := Candidate{
		Path:    "packages/coding-agent/src/prompts/system/plan-mode-active.md",
		Title:   "Plan Mode Active",
		Subtype: "agent_instruction",
		Body:    "Critical files for implementation.",
	}
	profile := BuildAnchorProfile(query)

	vocab := BuildRepoVocabulary(append([]Candidate{c}, anchorFillerCandidatesForTest()...))
	result := scoreAnchorFirstCandidate(c, profile, vocab, AnchorFirstModeV1)

	assert.True(t, anchorFirstPrimaryBoostAllowed(c, result, profile, AnchorFirstModeV1),
		"path/title prompt plan anchors should remain boostable: %#v", result)

}

func assertAnchorTerm(t *testing.T, profile AnchorProfile, term string, kind AnchorKind) {
	t.Helper()
	found := false
	for _, anchor := range profile.Anchors {
		if anchor.Term == term && anchor.Kind == kind {
			found = true
			break
		}
	}
	assert.True(t, found, "missing anchor term %q kind %q in %#v", term, kind, profile.Anchors)
}

func CandidatePathsFromScoredForTest(candidates []scoredCandidate) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.candidate.Path)
	}
	return out
}

func sameCandidatePathSetForTest(a, b []Candidate) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, candidate := range a {
		seen[candidate.Path]++
	}
	for _, candidate := range b {
		seen[candidate.Path]--
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}

func anchorFillerCandidatesForTest() []Candidate {
	return []Candidate{
		{Path: "docs/architecture.md", Title: "Architecture Overview"},
		{Path: "docs/design.md", Title: "Design Overview"},
		{Path: "docs/template.md", Title: "Template Overview"},
		{Path: "docs/service.md", Title: "Service Overview"},
		{Path: "docs/plan.md", Title: "Implementation Plan"},
	}
}

func repeatForTest(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
