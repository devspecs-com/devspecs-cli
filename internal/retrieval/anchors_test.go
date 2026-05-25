package retrieval

import "testing"

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
	if flux.DocumentCount == 0 {
		t.Fatalf("expected fluxnova term stats")
	}
	if flux.IDF <= calm.IDF {
		t.Fatalf("rare term should have higher IDF: flux %.3f calm %.3f", flux.IDF, calm.IDF)
	}
	if flux.PathCount == 0 || flux.TitleCount == 0 {
		t.Fatalf("expected strong field evidence for fluxnova: %#v", flux)
	}
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
	if len(got) == 0 || got[0].Path != "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md" {
		t.Fatalf("expected FluxNova requirement first, got %#v", CandidatePaths(got))
	}
	if got[0].Metadata["anchor_first_score"] == "" {
		t.Fatalf("missing anchor metadata: %#v", got[0].Metadata)
	}
}

func TestAnchorFirstRankingPromotesNamedTemplateWithoutRoleOnlyPromotion(t *testing.T) {
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

	roleOnly := (WeightedFilesRetrieverV0{AnchorFirstRanking: true}).Retrieve(candidates, "template")
	if len(roleOnly) > 0 && roleOnly[0].Path == "docs/static/calm-template/solution-architecture-document.md" && roleOnly[0].Metadata["anchor_first_score"] != "" {
		t.Fatalf("role-only template query should not be anchor-promoted: %#v", roleOnly[0].Metadata)
	}

	named := (WeightedFilesRetrieverV0{AnchorFirstRanking: true}).Retrieve(candidates, "solution architecture document template and trading system SAD example")
	if len(named) == 0 || named[0].Path != "docs/static/calm-template/solution-architecture-document.md" {
		t.Fatalf("expected named solution template first, got %#v", CandidatePaths(named))
	}
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
	if len(got) == 0 || got[0].Path != "components/camel-ai/tool_test.java#L53" {
		t.Fatalf("expected exact test-name first, got %#v", CandidatePaths(got))
	}
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
	if len(got) != 1 {
		t.Fatalf("rerank_only should not backfill candidates, got %#v", CandidatePathsFromScoredForTest(got))
	}
	if got[0].candidate.Metadata["anchor_first_backfill"] != "" {
		t.Fatalf("rerank_only should not mark backfill: %#v", got[0].candidate.Metadata)
	}
}

func TestAnchorFirstStrongFieldBackfillRequiresStrongField(t *testing.T) {
	selected := []scoredCandidate{{
		candidate: Candidate{Path: "docs/generic.md", Title: "Generic Architecture Notes", Body: "Architecture background."},
		score:     5,
	}}
	bodyOnly := Candidate{
		Path:  "docs/background.md",
		Title: "Background",
		Body:  "FluxNova AIGF integration requirements appear only in body text.",
	}
	strongField := Candidate{
		Path:  "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md",
		Title: "Requirements Spec: FluxNova Templates + AI Governance Framework Integration",
	}

	gotBodyOnly := applyAnchorFirstRanking(selected, append([]Candidate{selected[0].candidate, bodyOnly}, anchorFillerCandidatesForTest()...), "FluxNova AIGF integration requirements for CALM Studio", AnchorFirstModeStrongField)
	if len(gotBodyOnly) != 1 {
		t.Fatalf("strong_field should not backfill body-only matches, got %#v", CandidatePathsFromScoredForTest(gotBodyOnly))
	}

	gotStrong := applyAnchorFirstRanking(selected, append([]Candidate{selected[0].candidate, strongField}, anchorFillerCandidatesForTest()...), "FluxNova AIGF integration requirements for CALM Studio", AnchorFirstModeStrongField)
	if len(gotStrong) != 2 {
		t.Fatalf("strong_field should backfill one strong field match, got %#v", CandidatePathsFromScoredForTest(gotStrong))
	}
	if gotStrong[0].candidate.Path != strongField.Path || gotStrong[0].candidate.Metadata["anchor_first_backfill"] != "true" {
		t.Fatalf("expected strong field backfill first, got %#v metadata %#v", CandidatePathsFromScoredForTest(gotStrong), gotStrong[0].candidate.Metadata)
	}
}

func TestAnchorFirstStrictBackfillRequiresExactAnchorKind(t *testing.T) {
	selected := []scoredCandidate{{
		candidate: Candidate{Path: "docs/generic.md", Title: "Generic Architecture Notes", Body: "Architecture background."},
		score:     5,
	}}
	properOnly := Candidate{
		Path:  "calm-suite/calm-studio/docs/fluxnova-integration.md",
		Title: "FluxNova Integration",
	}
	pathLike := Candidate{
		Path:  "calm-suite/calm-studio/docs/REQ_fluxnova_aigf_integration.md",
		Title: "Requirements Spec: FluxNova Templates + AI Governance Framework Integration",
	}

	gotProper := applyAnchorFirstRanking(selected, append([]Candidate{selected[0].candidate, properOnly}, anchorFillerCandidatesForTest()...), "fluxnova integration requirements", AnchorFirstModeStrict)
	if len(gotProper) != 1 {
		t.Fatalf("strict should not backfill proper-term-only matches, got %#v", CandidatePathsFromScoredForTest(gotProper))
	}

	gotPathLike := applyAnchorFirstRanking(selected, append([]Candidate{selected[0].candidate, pathLike}, anchorFillerCandidatesForTest()...), "REQ_fluxnova_aigf_integration requirements", AnchorFirstModeStrict)
	if len(gotPathLike) != 2 {
		t.Fatalf("strict should backfill exact path-like anchors, got %#v", CandidatePathsFromScoredForTest(gotPathLike))
	}
}

func TestAnchorFirstV1DoesNotBoostGenericBodyHeadingOnlyMatches(t *testing.T) {
	query := "RFC transaction mechanism on key value store using MVCC and commit marks"
	candidates := []Candidate{
		{
			Path:  "rfc/rfc-3/Transaction-implementation-on-kv.md",
			Title: "RFC transaction mechanism on key-value store",
			Body:  "MVCC commit marks transaction design.",
		},
		{
			Path:  "design-docs/gravitino-logical-view-management.md",
			Title: "Design of Logical View Management",
			Body:  "This design discusses mechanisms, values, and storage details for views.",
			Sections: []IndexedSection{{
				HeadingPath: "Proposal > View Metadata Storage > Two View Storage Mechanisms",
				Title:       "Two View Storage Mechanisms",
			}},
		},
	}
	profile := BuildAnchorProfile(query)
	vocab := BuildRepoVocabulary(append(candidates, anchorFillerCandidatesForTest()...))
	result := scoreAnchorFirstCandidate(candidates[1], profile, vocab, AnchorFirstModeV1)
	if anchorFirstPrimaryBoostAllowed(candidates[1], result, profile, AnchorFirstModeV1) {
		t.Fatalf("generic heading/body-only match should not receive anchor-first boost: %#v", result)
	}

	strong := scoreAnchorFirstCandidate(candidates[0], profile, vocab, AnchorFirstModeV1)
	if !anchorFirstPrimaryBoostAllowed(candidates[0], strong, profile, AnchorFirstModeV1) {
		t.Fatalf("path/title RFC anchor should remain boostable: %#v", strong)
	}
}

func TestAnchorFirstDoesNotBoostDifferentAreaAgentInstructionsForArchitectureQuery(t *testing.T) {
	query := "DolphinScheduler architecture design for master worker api alert and dao modules"
	c := Candidate{
		Path:  "dolphinscheduler-alert/CLAUDE.md",
		Title: "CLAUDE.md - dolphinscheduler-alert",
		Body:  "Sub-modules include master worker api alert modules and design notes.",
	}
	profile := BuildAnchorProfile(query)
	if anchorFirstCandidateEligible(c, profile) {
		t.Fatalf("architecture query should not primary-promote different-area agent instruction files")
	}
}

func TestAnchorFirstAllowsAgentInstructionsForProtocolQuery(t *testing.T) {
	query := "Claude skill instructions for frontend design command"
	c := Candidate{
		Path:  ".claude/skills/frontend-design/SKILL.md",
		Title: "Frontend Design Skill",
		Body:  "Instructions for design commands.",
	}
	profile := BuildAnchorProfile(query)
	if !anchorFirstCandidateEligible(c, profile) {
		t.Fatalf("agent/protocol query should allow agent instruction candidates")
	}
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
	if !anchorFirstCandidateEligible(c, profile) {
		t.Fatalf("path/title prompt plan anchors should remain eligible even when classifier subtype is protocol-like")
	}
	vocab := BuildRepoVocabulary(append([]Candidate{c}, anchorFillerCandidatesForTest()...))
	result := scoreAnchorFirstCandidate(c, profile, vocab, AnchorFirstModeV1)
	if !anchorFirstPrimaryBoostAllowed(c, result, profile, AnchorFirstModeV1) {
		t.Fatalf("path/title prompt plan anchors should remain boostable: %#v", result)
	}
}

func assertAnchorTerm(t *testing.T, profile AnchorProfile, term string, kind AnchorKind) {
	t.Helper()
	for _, anchor := range profile.Anchors {
		if anchor.Term == term && anchor.Kind == kind {
			return
		}
	}
	t.Fatalf("missing anchor term %q kind %q in %#v", term, kind, profile.Anchors)
}

func CandidatePathsFromScoredForTest(candidates []scoredCandidate) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.candidate.Path)
	}
	return out
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
