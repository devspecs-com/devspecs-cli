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

	vocab := BuildRepoVocabulary(candidates)
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

func assertAnchorTerm(t *testing.T, profile AnchorProfile, term string, kind AnchorKind) {
	t.Helper()
	for _, anchor := range profile.Anchors {
		if anchor.Term == term && anchor.Kind == kind {
			return
		}
	}
	t.Fatalf("missing anchor term %q kind %q in %#v", term, kind, profile.Anchors)
}

func repeatForTest(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
