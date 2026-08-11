package classify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyCandidateMatchesSeedGoldens(t *testing.T) {
	cfg := DefaultPipelineConfig()

	require.NoError(t, ValidateConfig(cfg))

	fixtureRoot := filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented")
	goldens, err := LoadGoldenFile(filepath.Join(fixtureRoot, "classifier_cases.yaml"))
	require.NoError(t, err)

	for _, tc := range goldens.ClassifierCases {
		t.Run(tc.ID, func(t *testing.T) {
			candidate := candidateFromGolden(t, fixtureRoot, tc)
			resolution := ClassifyCandidate(candidate, cfg)
			winner := resolution.Winner
			require.Equal(t, tc.Expected.Classifier, winner.Classifier,
				"winner got %q want %q (confidence %.2f, alternatives %#v)", winner.Classifier, tc.Expected.Classifier, winner.Confidence, resolution.Alternatives)
			require.Equal(t, tc.Expected.Scope, winner.Scope,
				"scope got %q want %q", winner.Scope, tc.Expected.Scope)
			if tc.Expected.Subformat != "" {
				assert.Equal(t, tc.Expected.Subformat, winner.Subformat)
			}
			if tc.Expected.Family != "" {
				assert.Equal(t, tc.Expected.Family, winner.Family)
			}
			if tc.Expected.Kind != "" {
				assert.Equal(t, tc.Expected.Kind, winner.Kind)
			}
			if tc.Expected.Subtype != "" {
				assert.Equal(t, tc.Expected.Subtype, winner.Subtype)
			}
			if tc.Expected.Status != "" {
				assert.Equal(t, tc.Expected.Status, winner.Status)
			}
			if tc.Expected.Authority != "" {
				assert.Equal(t, tc.Expected.Authority, winner.Authority)
			}
			if tc.Expected.FormatProfile != "" {
				assert.Equal(t, tc.Expected.FormatProfile, winner.FormatProfile)
			}

			for _, forbidden := range tc.Expected.MustNotClassifyAs {
				require.NotEqual(t, forbidden, winner.Classifier,
					"winner classified as forbidden model %q", forbidden)

			}
			for _, reason := range tc.Expected.RequiredReasons {
				require.True(t, hasPositiveReason(winner, reason),
					"missing positive reason %q in %#v", reason, winner.PositiveReasons)

			}
			if len(tc.Expected.ChildCandidates) > 0 {
				require.Len(t, winner.ChildCandidates, len(tc.Expected.ChildCandidates))
			}

		})
	}
}

func TestClassifyCandidate_WithMADRDocument_ReturnsMADRSubformat(t *testing.T) {
	cfg := DefaultPipelineConfig()

	resolution := ClassifyCandidate(Candidate{
		Path:  "docs/adrs/0010-cache-boundary.md",
		Scope: ScopeDocument,
		Body:  "# 0010 Cache Boundary\n\n## Context and Problem Statement\n\nDecide the boundary.\n\n## Decision Drivers\n\n- Durable writes\n\n## Considered Options\n\n- Local cache\n\n## Decision Outcome\n\nUse database writes.\n",
	}, cfg)

	assert.Equal(t, ModelADR, resolution.Winner.Classifier)
	assert.Equal(t, SubmodelADRMADR, resolution.Winner.Subformat)
}

func TestClassifyCandidate_WithYStatementDocument_ReturnsYStatementSubformat(t *testing.T) {
	cfg := DefaultPipelineConfig()

	resolution := ClassifyCandidate(Candidate{
		Path:  "docs/adr/0011-y-statement.md",
		Scope: ScopeDocument,
		Body:  "---\nstatus: accepted\n---\n\n# ADR 0011: Session Boundary\n\nIn the context of auth token rotation, facing replay risk, we decided to bind refresh tokens to sessions, to achieve safer resumes, accepting extra invalidations.\n",
	}, cfg)

	assert.Equal(t, ModelADR, resolution.Winner.Classifier)
	assert.Equal(t, SubmodelADRYStatement, resolution.Winner.Subformat)
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
	require.Equal(t, ModelRFC, resolution.Winner.Classifier,
		"enhancement proposal got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelRFC, resolution.Winner.Confidence, resolution.Alternatives)

}

func TestClassifyCandidateRecognizesProposalFamilyDirectoryIndexAsRFC(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "beps/0013-ai-skills/README.md",
		Scope: ScopeDocument,
		Body: strings.Join([]string{
			"---",
			"status: proposed",
			"---",
			"# AI Skills",
			"",
			"## Summary",
			"",
			"Add a skills interface for reusable agent workflows.",
			"",
			"## Motivation",
			"",
			"Tool authors need a governed proposal before changing runtime behavior.",
			"",
			"## Proposal",
			"",
			"Store skill manifests in a stable directory and load them during initialization.",
			"",
			"## Detailed Design",
			"",
			"Describe parsing, validation, and rollout details.",
		}, "\n"),
	}, cfg)
	require.Equal(t, ModelRFC, resolution.Winner.Classifier,
		"proposal-family README got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelRFC, resolution.Winner.Confidence, resolution.Alternatives)

}

func TestClassifyCandidateRecognizesDesignDirProposalAsRFC(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "design/002-secret-sync.md",
		Scope: ScopeDocument,
		Body: strings.Join([]string{
			"# Secret Sync",
			"",
			"## Summary",
			"",
			"Allow generated secrets to be synchronized back into selected providers.",
			"",
			"## Motivation",
			"",
			"Operators need durable provider copies for failover.",
			"",
			"## Proposal",
			"",
			"Add a controller and reconcile provider writes from cluster state.",
			"",
			"## Alternatives",
			"",
			"Use a separate infrastructure tool.",
		}, "\n"),
	}, cfg)
	require.Equal(t, ModelRFC, resolution.Winner.Classifier,
		"design-dir proposal got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelRFC, resolution.Winner.Confidence, resolution.Alternatives)

}

func TestClassifyCandidateRecognizesRootRFCWithTechnicalDesign(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "RFC.MD",
		Scope: ScopeDocument,
		Body: strings.Join([]string{
			"# Request for Comments (RFC)",
			"",
			"## Overview",
			"",
			"This RFC requests feedback on the extension architecture.",
			"",
			"## Technical Design",
			"",
			"Split the implementation into background, content, and popup components.",
			"",
			"## Design Considerations",
			"",
			"Minimize permissions and preserve browser compatibility.",
			"",
			"## Request for Feedback",
			"",
			"Comments are welcome on architecture and edge cases.",
		}, "\n"),
	}, cfg)
	require.Equal(t, ModelRFC, resolution.Winner.Classifier,
		"root RFC got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelRFC, resolution.Winner.Confidence, resolution.Alternatives)

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
	require.Equal(t, ModelRFC, resolution.Winner.Classifier,
		"governed proposal got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelRFC, resolution.Winner.Confidence, resolution.Alternatives)

}

func TestClassifyCandidateRecognizesADRMetadataStatusBody(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "docs/adrs/ADR-014-hybrid-scraping-strategy.md",
		Scope: ScopeDocument,
		Body: strings.Join([]string{
			"# ADR-014: Hybrid Scraping Strategy Implementation",
			"",
			"## Metadata",
			"",
			"**Status:** Accepted",
			"",
			"## Context",
			"",
			"The scraper needs a simpler strategy for structured and unstructured sources.",
			"",
			"## Decision Drivers",
			"",
			"- Maintenance cost",
			"- Extraction reliability",
			"",
			"## Decision",
			"",
			"Adopt a two-tier strategy using a library-first structured scraper plus an AI fallback.",
		}, "\n"),
	}, cfg)
	require.Equal(t, ModelADR, resolution.Winner.Classifier,
		"ADR metadata status got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelADR, resolution.Winner.Confidence, resolution.Alternatives)

}

func TestClassifyCandidatePrefersADROverProtocolStandardLanguage(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "docs/adr/ADR-0050-plan-document-metadata-standard.md",
		Scope: ScopeDocument,
		Body: strings.Join([]string{
			"# ADR 0050 - Plan document metadata standard",
			"",
			"- **Status:** Accepted",
			"",
			"## Context",
			"",
			"Plan files need consistent metadata so contributors can understand ownership and priority.",
			"",
			"## Decision",
			"",
			"New plan files must include status, date, authors, priority, and dependency fields.",
			"",
			"## Consequences",
			"",
			"Every new plan must comply with the metadata standard.",
		}, "\n"),
	}, cfg)
	require.Equal(t, ModelADR, resolution.Winner.Classifier,
		"ADR with standard language got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelADR, resolution.Winner.Confidence, resolution.Alternatives)

}

func TestClassifyCandidateRecognizesRFCFilenameWithDesignSections(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "RFC.MD",
		Scope: ScopeDocument,
		Body: strings.Join([]string{
			"# Request for Comments (RFC)",
			"",
			"## Overview",
			"",
			"This RFC outlines the technical architecture for a browser extension.",
			"",
			"## Technical Design",
			"",
			"The service worker checks navigation events and sends messages to content scripts.",
			"",
			"## Design Considerations",
			"",
			"Permissions, performance, and privacy are the main tradeoffs.",
			"",
			"## Request for Feedback",
			"",
			"Feedback is requested on the proposed design.",
		}, "\n"),
	}, cfg)
	require.Equal(t, ModelRFC, resolution.Winner.Classifier,
		"RFC filename got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelRFC, resolution.Winner.Confidence, resolution.Alternatives)

}

func TestClassifyCandidateRecognizesEnhancementProposalReadme(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "enhancements/ai-assisted-rules-generation/README.md",
		Scope: ScopeDocument,
		Body: strings.Join([]string{
			"# AI-Assisted Rules Generation",
			"",
			"## Summary",
			"",
			"This enhancement proposes generating migration rules from documentation.",
			"",
			"## Motivation",
			"",
			"Creating rules requires both domain knowledge and rule syntax expertise.",
			"",
			"## Goals",
			"",
			"Lower the barrier to creating rules.",
			"",
			"## Non-Goals",
			"",
			"Do not replace human review.",
			"",
			"## Proposal",
			"",
			"Use agent skills and deterministic helpers to draft and validate rules.",
		}, "\n"),
	}, cfg)
	require.Equal(t, ModelRFC, resolution.Winner.Classifier,
		"enhancement README got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelRFC, resolution.Winner.Confidence, resolution.Alternatives)

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
	require.Equal(t, ModelPRD, resolution.Winner.Classifier,
		"plain PRD got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelPRD, resolution.Winner.Confidence, resolution.Alternatives)

}

func TestClassifyCandidatePrefersPRDTitleRequirementSectionsOverPlanPath(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "plans/x-search-skill-integration.md",
		Scope: ScopeDocument,
		Body: strings.Join([]string{
			"# PRD: Integrate x-search skill for research",
			"",
			"## Overview",
			"",
			"Add a skill for real-time product and developer discourse research.",
			"",
			"## Goals & Objectives",
			"",
			"Enable research from an existing agent workflow.",
			"",
			"## User Stories",
			"",
			"- As a developer, I want to search discourse, so that I can understand feedback.",
			"",
			"## Functional Requirements",
			"",
			"- The CLI supports search, profile, and thread commands.",
			"",
			"## Success Metrics",
			"",
			"- Valid searches return markdown output.",
			"",
			"## Implementation Plan",
			"",
			"- [ ] Port the command",
			"- [ ] Validate output",
		}, "\n"),
	}, cfg)
	require.Equal(t, ModelPRD, resolution.Winner.Classifier,
		"PRD in plans path got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelPRD, resolution.Winner.Confidence, resolution.Alternatives)

}

func TestClassifyCandidateRecognizesRoadmapAsPlanFamily(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "ROADMAP.md",
		Scope: ScopeDocument,
		Body:  "# Roadmap\n\nHigh-level product direction.\n\n## Milestones\n\n- [ ] Foundation\n- [ ] GA\n",
	}, cfg)
	require.Equal(t, ModelPlan, resolution.Winner.Classifier,
		"roadmap got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelPlan, resolution.Winner.Confidence, resolution.Alternatives)
	require.Equal(t, SubmodelPlanRoadmap, resolution.Winner.Family,
		"roadmap family got %q want %q", resolution.Winner.Family, SubmodelPlanRoadmap)

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
	require.Equal(t, ModelPlan, resolution.Winner.Classifier,
		"story got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelPlan, resolution.Winner.Confidence, resolution.Alternatives)
	require.Equal(t, SubmodelPlanStoryArtifact, resolution.Winner.Family,
		"story family got %q want %q", resolution.Winner.Family, SubmodelPlanStoryArtifact)

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
	require.Equal(t, ModelAgentNote, resolution.Winner.Classifier,
		"codex plan got %q want %q (confidence %.2f, alternatives %#v)", resolution.Winner.Classifier, ModelAgentNote, resolution.Winner.Confidence, resolution.Alternatives)

}

func TestClassifyCandidate_WithClaudeInstructions_ReturnsAgentInstructionProtocol(t *testing.T) {
	cfg := DefaultPipelineConfig()

	resolution := ClassifyCandidate(Candidate{Path: "CLAUDE.md", Scope: ScopeDocument, Body: "# Project Instructions\n\n## Rules\n\nAlways run tests. Never rewrite unrelated files.\n"}, cfg)

	assertSubtypeClassification(t, resolution, ModelProtocol, SubmodelProtocolAgentInstruction, "protocol")
}

func TestClassifyCandidate_WithSkill_ReturnsSkillProtocol(t *testing.T) {
	cfg := DefaultPipelineConfig()

	resolution := ClassifyCandidate(Candidate{Path: ".claude/skills/review/SKILL.md", Scope: ScopeDocument, Body: "# Review Skill\n\nUse this procedure when reviewing code.\n"}, cfg)

	assertSubtypeClassification(t, resolution, ModelProtocol, SubmodelProtocolSkill, "protocol")
}

func TestClassifyCandidate_WithMaintainersFile_ReturnsMaintainerProtocol(t *testing.T) {
	cfg := DefaultPipelineConfig()

	resolution := ClassifyCandidate(Candidate{Path: "MAINTAINERS.md", Scope: ScopeDocument, Body: "# Maintainers\n\nThis file lists maintainers and review rules.\n"}, cfg)

	assertSubtypeClassification(t, resolution, ModelProtocol, SubmodelProtocolMaintainerPolicy, "protocol")
}

func TestClassifyCandidate_WithPullRequestTemplate_ReturnsPullRequestTemplate(t *testing.T) {
	cfg := DefaultPipelineConfig()

	resolution := ClassifyCandidate(Candidate{Path: ".github/PULL_REQUEST_TEMPLATE.md", Scope: ScopeDocument, Body: "# Pull Request\n\n## Summary\n\n{{ summary }}\n"}, cfg)

	assertSubtypeClassification(t, resolution, ModelTemplate, SubmodelTemplatePullRequest, "template")
}

func TestClassifyCandidate_WithPRDTemplate_ReturnsDocumentTemplate(t *testing.T) {
	cfg := DefaultPipelineConfig()

	resolution := ClassifyCandidate(Candidate{Path: "docs/templates/prd-template.md", Scope: ScopeDocument, Body: "# Product Requirements Template\n\n## Scope\n\n{{ fill in }}\n\n## User Stories\n\n[insert stories]\n"}, cfg)

	assertSubtypeClassification(t, resolution, ModelTemplate, SubmodelTemplateDocument, "template")
}

func TestClassifyCandidate_WithAPIContract_ReturnsStructuredAPIModel(t *testing.T) {
	cfg := DefaultPipelineConfig()

	resolution := ClassifyCandidate(Candidate{Path: "docs/contracts/openapi.md", Scope: ScopeDocument, Body: "# OpenAPI Contract\n\n```yaml\nopenapi: 3.1.0\npaths: {}\n```\n"}, cfg)

	assertSubtypeClassification(t, resolution, ModelStructuredModel, SubmodelModelAPIContract, "model")
}

func TestClassifyCandidate_WithWorkflowDefinition_ReturnsStructuredWorkflowModel(t *testing.T) {
	cfg := DefaultPipelineConfig()

	resolution := ClassifyCandidate(Candidate{Path: ".github/workflows/ci.md", Scope: ScopeDocument, Body: "# CI Workflow\n\nDocuments the jobs: section for GitHub Actions.\n"}, cfg)

	assertSubtypeClassification(t, resolution, ModelStructuredModel, SubmodelModelWorkflow, "model")
}

func assertSubtypeClassification(t *testing.T, resolution Resolution, classifier, family, mode string) {
	t.Helper()
	assert.Equal(t, classifier, resolution.Winner.Classifier)
	assert.Equal(t, family, resolution.Winner.Family)
	assert.Equal(t, mode, resolution.Winner.Mode)
}

func TestClassifyCandidateFallsBackWhenEvidenceIsWeakOrAmbiguous(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "notes/billing.md",
		Scope: ScopeDocument,
		Body:  "# Billing Notes\n\nA loose note with webhook, customer, auth, and token terms but no durable plan or decision structure.\n",
	}, cfg)
	require.Equal(t, ModelGenericMarkdown, resolution.Winner.Classifier,
		"winner got %q want generic fallback", resolution.Winner.Classifier)
	require.True(t, resolution.FallbackGeneric,
		"expected generic fallback flag")

}

func TestClassifyCandidateAppliesNegativeEvidence(t *testing.T) {
	cfg := DefaultPipelineConfig()
	resolution := ClassifyCandidate(Candidate{
		Path:  "docs/plans/generated-release-notes.md",
		Scope: ScopeDocument,
		Body:  "# Generated Plan\n\nGenerated release notes. Do not edit.\n\n- [ ] Task copied from a changelog.\n",
	}, cfg)
	require.NotEqual(t, ModelPlan, resolution.Winner.Classifier,
		"generated changelog-like file should not win as plan: %#v", resolution.Winner)

	var plan AlternativePlan
	for _, alternative := range resolution.Alternatives {
		if alternative.Classifier == ModelPlan {
			plan.Classification = alternative
			break
		}
	}
	require.NotEmpty(t, plan.NegativeReasons,
		"expected plan negative evidence in alternatives: %#v", resolution.Alternatives)

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
	require.Equal(t, "engineering_brief", resolution.Winner.Classifier,
		"winner got %q want local model; alternatives %#v", resolution.Winner.Classifier, resolution.Alternatives)
	require.True(t, hasPositiveReason(resolution.Winner, ReasonLocalOverride),
		"expected local override reason: %#v", resolution.Winner.PositiveReasons)

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
		require.NoError(t, err)

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
