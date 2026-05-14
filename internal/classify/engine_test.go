package classify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyCandidateMatchesSeedGoldens(t *testing.T) {
	cfg := DefaultPipelineConfig()
	if err := ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}

	fixtureRoot := filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented")
	goldens, err := LoadGoldenFile(filepath.Join(fixtureRoot, "classifier_cases.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range goldens.ClassifierCases {
		t.Run(tc.ID, func(t *testing.T) {
			candidate := candidateFromGolden(t, fixtureRoot, tc)
			resolution := ClassifyCandidate(candidate, cfg)
			winner := resolution.Winner

			if winner.Classifier != tc.Expected.Classifier {
				t.Fatalf("winner got %q want %q (confidence %.2f, alternatives %#v)", winner.Classifier, tc.Expected.Classifier, winner.Confidence, resolution.Alternatives)
			}
			if winner.Scope != tc.Expected.Scope {
				t.Fatalf("scope got %q want %q", winner.Scope, tc.Expected.Scope)
			}
			if tc.Expected.Subformat != "" && winner.Subformat != tc.Expected.Subformat {
				t.Fatalf("subformat got %q want %q", winner.Subformat, tc.Expected.Subformat)
			}
			if tc.Expected.Family != "" && winner.Family != tc.Expected.Family {
				t.Fatalf("family got %q want %q", winner.Family, tc.Expected.Family)
			}
			if tc.Expected.Kind != "" && winner.Kind != tc.Expected.Kind {
				t.Fatalf("kind got %q want %q", winner.Kind, tc.Expected.Kind)
			}
			if tc.Expected.Subtype != "" && winner.Subtype != tc.Expected.Subtype {
				t.Fatalf("subtype got %q want %q", winner.Subtype, tc.Expected.Subtype)
			}
			if tc.Expected.Status != "" && winner.Status != tc.Expected.Status {
				t.Fatalf("status got %q want %q", winner.Status, tc.Expected.Status)
			}
			if tc.Expected.Authority != "" && winner.Authority != tc.Expected.Authority {
				t.Fatalf("authority got %q want %q", winner.Authority, tc.Expected.Authority)
			}
			if tc.Expected.FormatProfile != "" && winner.FormatProfile != tc.Expected.FormatProfile {
				t.Fatalf("format profile got %q want %q", winner.FormatProfile, tc.Expected.FormatProfile)
			}
			for _, forbidden := range tc.Expected.MustNotClassifyAs {
				if winner.Classifier == forbidden {
					t.Fatalf("winner classified as forbidden model %q", forbidden)
				}
			}
			for _, reason := range tc.Expected.RequiredReasons {
				if !hasPositiveReason(winner, reason) {
					t.Fatalf("missing positive reason %q in %#v", reason, winner.PositiveReasons)
				}
			}
			if len(tc.Expected.ChildCandidates) > 0 && len(winner.ChildCandidates) != len(tc.Expected.ChildCandidates) {
				t.Fatalf("child candidates got %d want %d", len(winner.ChildCandidates), len(tc.Expected.ChildCandidates))
			}
		})
	}
}

func TestClassifyCandidateRecognizesADRSubformatsDeclaratively(t *testing.T) {
	cfg := DefaultPipelineConfig()

	madr := ClassifyCandidate(Candidate{
		Path:  "docs/adrs/0010-cache-boundary.md",
		Scope: ScopeDocument,
		Body:  "# 0010 Cache Boundary\n\n## Context and Problem Statement\n\nDecide the boundary.\n\n## Decision Drivers\n\n- Durable writes\n\n## Considered Options\n\n- Local cache\n\n## Decision Outcome\n\nUse database writes.\n",
	}, cfg)
	if madr.Winner.Classifier != ModelADR || madr.Winner.Subformat != SubmodelADRMADR {
		t.Fatalf("MADR got %s/%s", madr.Winner.Classifier, madr.Winner.Subformat)
	}

	yStatement := ClassifyCandidate(Candidate{
		Path:  "docs/adr/0011-y-statement.md",
		Scope: ScopeDocument,
		Body:  "---\nstatus: accepted\n---\n\n# ADR 0011: Session Boundary\n\nIn the context of auth token rotation, facing replay risk, we decided to bind refresh tokens to sessions, to achieve safer resumes, accepting extra invalidations.\n",
	}, cfg)
	if yStatement.Winner.Classifier != ModelADR || yStatement.Winner.Subformat != SubmodelADRYStatement {
		t.Fatalf("Y-Statement got %s/%s", yStatement.Winner.Classifier, yStatement.Winner.Subformat)
	}
}

func TestClassifyCandidateFallsBackWhenEvidenceIsWeakOrAmbiguous(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "notes/billing.md",
		Scope: ScopeDocument,
		Body:  "# Billing Notes\n\nA loose note with webhook, customer, auth, and token terms but no durable plan or decision structure.\n",
	}, cfg)
	if resolution.Winner.Classifier != ModelGenericMarkdown {
		t.Fatalf("winner got %q want generic fallback", resolution.Winner.Classifier)
	}
	if !resolution.FallbackGeneric {
		t.Fatal("expected generic fallback flag")
	}
}

func TestClassifyCandidateAppliesNegativeEvidence(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "docs/plans/generated-release-notes.md",
		Scope: ScopeDocument,
		Body:  "# Generated Plan\n\nGenerated release notes. Do not edit.\n\n- [ ] Task copied from a changelog.\n",
	}, cfg)
	if resolution.Winner.Classifier == ModelPlan {
		t.Fatalf("generated changelog-like file should not win as plan: %#v", resolution.Winner)
	}
	var plan AlternativePlan
	for _, alternative := range resolution.Alternatives {
		if alternative.Classifier == ModelPlan {
			plan.Classification = alternative
			break
		}
	}
	if len(plan.NegativeReasons) == 0 {
		t.Fatalf("expected plan negative evidence in alternatives: %#v", resolution.Alternatives)
	}
}

func TestClassifyCandidateEvaluatesDeclarativeLocalModels(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.LocalModels.Definitions = []LocalModelDefinition{{
		ID:               "engineering_brief",
		BaseModel:        ModelRFC,
		Authority:        AuthorityDesignProposal,
		PathHints:        []string{"briefs/**"},
		RequiredHeadings: []string{"Problem", "Proposal"},
		Evidence: []EvidenceRule{{
			ID:     "engineering_brief_tradeoffs",
			Weight: 0.20,
			Reason: ReasonLocalOverride,
			Match: EvidenceMatch{
				Scope:           ScopeDocument,
				BodyContainsAny: []string{"tradeoff"},
			},
		}},
	}}
	resolution := ClassifyCandidate(Candidate{
		Path:  "briefs/auth-token-boundary.md",
		Scope: ScopeDocument,
		Body:  "# Auth Token Boundary\n\n## Problem\n\nRefresh tokens are ambiguous.\n\n## Proposal\n\nBind tokens to sessions and document the tradeoff.\n",
	}, cfg)
	if resolution.Winner.Classifier != "engineering_brief" {
		t.Fatalf("winner got %q want local model; alternatives %#v", resolution.Winner.Classifier, resolution.Alternatives)
	}
	if !hasPositiveReason(resolution.Winner, ReasonLocalOverride) {
		t.Fatalf("expected local override reason: %#v", resolution.Winner.PositiveReasons)
	}
}

type AlternativePlan struct {
	Classification
}

func candidateFromGolden(t *testing.T, fixtureRoot string, tc GoldenCase) Candidate {
	t.Helper()
	candidate := Candidate{
		Path:  tc.Path,
		Scope: tc.Scope,
	}
	if tc.Scope == ScopeDocument {
		body, err := os.ReadFile(filepath.Join(fixtureRoot, filepath.FromSlash(tc.Path)))
		if err != nil {
			t.Fatal(err)
		}
		candidate.Body = string(body)
	}
	for _, child := range tc.Expected.ChildCandidates {
		candidate.ChildCandidates = append(candidate.ChildCandidates, Candidate{
			Path:  child.Path,
			Scope: ScopeDocument,
			Role:  child.Role,
		})
	}
	return candidate
}

func hasPositiveReason(classification Classification, want ReasonCode) bool {
	for _, reason := range classification.PositiveReasons {
		if reason.Code == want {
			return true
		}
	}
	return false
}
