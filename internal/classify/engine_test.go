package classify

import (
	"os"
	"path/filepath"
	"strings"
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

func TestClassifyCandidateRecognizesEnhancementProposalAsRFC(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "enhancements/operator-bundle-validation.md",
		Scope: ScopeDocument,
		Body: strings.Join([]string{
			"---",
			"title: bundle-validation",
			"status: implementable",
			"authors:",
			"  - example",
			"---",
			"# Bundle Validation",
			"",
			"## Release Signoff Checklist",
			"",
			"- [ ] Enhancement is implementable",
			"- [ ] Test plan is defined",
			"",
			"## Summary",
			"",
			"This enhancement proposes a validation library for bundles.",
			"",
			"## Motivation",
			"",
			"Operators need static validation before release.",
			"",
			"## Proposal",
			"",
			"Expose reusable validation rules and report errors consistently.",
		}, "\n"),
	}, cfg)
	if resolution.Winner.Classifier != ModelRFC {
		t.Fatalf("enhancement proposal got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelRFC, resolution.Winner.Confidence, resolution.Alternatives)
	}
}

func TestClassifyCandidatePrefersRFCForGovernedProposalWithPlanShape(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "ships/0018-build-env-vars.md",
		Scope: ScopeDocument,
		Body: strings.Join([]string{
			"<!-- SPDX-License-Identifier: Apache-2.0 -->",
			"",
			"---",
			"title: build-env-vars",
			"status: implementable",
			"---",
			"",
			"# Build Environment Variables",
			"",
			"## Release Signoff Checklist",
			"",
			"- [ ] Enhancement is implementable",
			"- [ ] Test plan is defined",
			"",
			"## Summary",
			"",
			"This proposal lets users add environment variables to build steps.",
			"",
			"## Motivation",
			"",
			"Build authors need a safe way to expose configuration.",
			"",
			"## Proposal",
			"",
			"Add an API field and document implementation behavior.",
			"",
			"### Implementation Notes",
			"",
			"Implementation proceeds through build strategy API updates.",
		}, "\n"),
	}, cfg)
	if resolution.Winner.Classifier != ModelRFC {
		t.Fatalf("governed proposal got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelRFC, resolution.Winner.Confidence, resolution.Alternatives)
	}
}

func TestClassifyCandidateRecognizesPlainPRDFilenameAndBody(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "PRD.md",
		Scope: ScopeDocument,
		Body: strings.Join([]string{
			"Product Requirements Document: Simple AI Chatbot",
			"",
			"1. Project Overview",
			"",
			"A minimalist AI chatbot with persistent chat history.",
			"",
			"2. UI/UX Requirements",
			"",
			"Simplicity, responsive layout, readability, and feedback.",
			"",
			"3. Functional Requirements",
			"",
			"Users can authenticate, stream chat responses, and manage threads.",
			"",
			"4. Success Metrics",
			"",
			"Fast response time and reliable message persistence.",
		}, "\n"),
	}, cfg)
	if resolution.Winner.Classifier != ModelPRD {
		t.Fatalf("plain PRD got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelPRD, resolution.Winner.Confidence, resolution.Alternatives)
	}
}

func TestClassifyCandidateRecognizesRoadmapAsPlanFamily(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "ROADMAP.md",
		Scope: ScopeDocument,
		Body:  "# Roadmap\n\nHigh-level product direction.\n\n## Milestones\n\n- [ ] Foundation\n- [ ] GA\n",
	}, cfg)
	if resolution.Winner.Classifier != ModelPlan {
		t.Fatalf("roadmap got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelPlan, resolution.Winner.Confidence, resolution.Alternatives)
	}
	if resolution.Winner.Family != SubmodelPlanRoadmap {
		t.Fatalf("roadmap family got %q want %q", resolution.Winner.Family, SubmodelPlanRoadmap)
	}
}

func TestClassifyCandidateRecognizesBMADLikeStoryAsPlanFamily(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "docs/stories/1.1.story.md",
		Scope: ScopeDocument,
		Body: strings.Join([]string{
			"# Story 1.1: Simplify Memory Tools",
			"",
			"## Status",
			"",
			"Done",
			"",
			"## Story",
			"",
			"**As a** developer, **I want** focused tools, **so that** the platform stays maintainable.",
			"",
			"## Acceptance Criteria",
			"",
			"1. Keep only the core tools.",
			"",
			"## Tasks / Subtasks",
			"",
			"- [x] Remove obsolete commands",
			"- [x] Update tests",
			"",
			"## Dev Notes",
			"",
			"Use the existing service layer.",
		}, "\n"),
	}, cfg)
	if resolution.Winner.Classifier != ModelPlan {
		t.Fatalf("story got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelPlan, resolution.Winner.Confidence, resolution.Alternatives)
	}
	if resolution.Winner.Family != SubmodelPlanStoryArtifact {
		t.Fatalf("story family got %q want %q", resolution.Winner.Family, SubmodelPlanStoryArtifact)
	}
}

func TestClassifyCandidateRecognizesCodexPlanAsAgentNote(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  ".codex/plans/PLAN.md",
		Scope: ScopeDocument,
		Body: strings.Join([]string{
			"# 80/20 Related Specs",
			"",
			"## Summary",
			"",
			"Source of truth for the current implementation slice.",
			"",
			"## Next Steps",
			"",
			"- [ ] Add schema migration",
			"- [ ] Wire command tests",
		}, "\n"),
	}, cfg)
	if resolution.Winner.Classifier != ModelAgentNote {
		t.Fatalf("codex plan got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelAgentNote, resolution.Winner.Confidence, resolution.Alternatives)
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
