package commands

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestArtifactCandidateIncludesClassifierMetadata(t *testing.T) {
	extracted := `{
		"classifier": {
			"evaluator": "declarative_document_models_v0",
			"profile": "builtin_intent_docs_v1",
			"config_version": 1,
			"ambiguous": false,
			"fallback_generic": false,
			"winner": {
				"classifier": "plan",
				"family": "plan.implementation_plan",
				"confidence": 0.78,
				"kind": "plan",
				"status": "active",
				"authority": "working_plan"
			}
		}
	}`
	candidate := artifactCandidate(
		store.ArtifactRow{
			ID:           "ds_1",
			RepoID:       "repo_1",
			ShortID:      "DS-1",
			Kind:         "plan",
			Title:        "Plan",
			Status:       "active",
			CurrentRevID: "rev_1",
		},
		[]store.SourceRow{{Path: "docs/plans/plan.md"}},
		nil,
		"# Plan",
		extracted,
	)
	if candidate.Metadata["classifier_model"] != "plan" {
		t.Fatalf("classifier_model = %#v", candidate.Metadata["classifier_model"])
	}
	if candidate.Metadata["classifier_family"] != "plan.implementation_plan" {
		t.Fatalf("classifier_family = %#v", candidate.Metadata["classifier_family"])
	}
	if candidate.Metadata["classifier_confidence"] != "0.780" {
		t.Fatalf("classifier_confidence = %#v", candidate.Metadata["classifier_confidence"])
	}
	if candidate.Metadata["classifier_authority"] != "working_plan" {
		t.Fatalf("classifier_authority = %#v", candidate.Metadata["classifier_authority"])
	}
}

func TestArtifactCandidateIncludesHierarchyMetadataAndLinks(t *testing.T) {
	extracted := `{
		"mode": "intent",
		"role": "authoritative",
		"artifact_scope": "bundle",
		"source_standard": "openspec",
		"openspec_role": "change_bundle",
		"openspec_change_id": "add-sso",
		"layout_group": "openspec/changes/add-sso"
	}`
	candidate := artifactCandidateWithLinks(
		store.ArtifactRow{
			ID:           "bundle_1",
			RepoID:       "repo_1",
			ShortID:      "DS-2",
			Kind:         "spec",
			Subtype:      "openspec_change_bundle",
			Title:        "Add SSO",
			Status:       "proposed",
			CurrentRevID: "rev_1",
		},
		[]store.SourceRow{{Path: "openspec/changes/add-sso"}},
		[]store.LinkRow{{LinkType: "contains", Target: "artifact:child_1"}},
		nil,
		"# Add SSO",
		extracted,
	)
	if candidate.Metadata["artifact_scope"] != "bundle" {
		t.Fatalf("artifact_scope = %#v", candidate.Metadata["artifact_scope"])
	}
	if candidate.Metadata["openspec_role"] != "change_bundle" {
		t.Fatalf("openspec_role = %#v", candidate.Metadata["openspec_role"])
	}
	if candidate.Metadata["link_contains"] != "artifact:child_1" {
		t.Fatalf("link_contains = %#v", candidate.Metadata["link_contains"])
	}
}
