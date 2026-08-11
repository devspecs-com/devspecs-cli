package commands

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	docsections "github.com/devspecs-com/devspecs-cli/internal/sections"
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
				"mode": "intent",
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
	assert.Equal(t, "plan", candidate.Metadata["classifier_model"],
		"classifier_model = %#v", candidate.Metadata["classifier_model"])
	assert.Equal(t, "plan.implementation_plan", candidate.Metadata["classifier_family"],
		"classifier_family = %#v", candidate.Metadata["classifier_family"])
	assert.Equal(t, "0.780", candidate.Metadata["classifier_confidence"],
		"classifier_confidence = %#v", candidate.Metadata["classifier_confidence"])
	assert.Equal(t, "intent", candidate.Metadata["classifier_mode"],
		"classifier_mode = %#v", candidate.Metadata["classifier_mode"])
	assert.Equal(t, "working_plan", candidate.Metadata["classifier_authority"],
		"classifier_authority = %#v", candidate.Metadata["classifier_authority"])

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
	assert.Equal(t, "bundle", candidate.Metadata["artifact_scope"],
		"artifact_scope = %#v", candidate.Metadata["artifact_scope"])
	assert.Equal(t, "change_bundle", candidate.Metadata["openspec_role"],
		"openspec_role = %#v", candidate.Metadata["openspec_role"])
	assert.Equal(t, "artifact:child_1", candidate.Metadata["link_contains"],
		"link_contains = %#v", candidate.Metadata["link_contains"])

}

func TestLoadRetrievalCandidatesForQueryAddsSectionEvidence(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	repoID := "repo_sec"
	{
		_, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", repoID, tmp, now, now)
		require.NoError(t, err)
	}
	{

		err := db.InsertArtifactDirect("ds_sec", repoID, "plan", "", "Billing Plan", "active", "rev_sec", now, now)
		require.NoError(t, err)
	}
	{

		err := db.InsertRevisionDirect("rev_sec", "ds_sec", "sha256:test", "# Billing Plan\n\n## Replay Boundary\n\nstripe_event_id idempotency matters.", "", now)
		require.NoError(t, err)
	}
	{

		err := db.InsertSourceDirect("src_sec", "ds_sec", repoID, "markdown", "docs/plans/billing.md", "docs/plans/billing.md|markdown", "", "", now)
		require.NoError(t, err)
	}

	sections := docsections.AssignStableIDs(docsections.ExtractMarkdown("# Billing Plan\n\n## Replay Boundary\n\nstripe_event_id idempotency matters."), "ds_sec", "rev_sec", "docs/plans/billing.md")
	{
		err := db.ReplaceArtifactSections("ds_sec", "rev_sec", sections, now)
		require.NoError(t, err)
	}

	candidates, err := loadRetrievalCandidatesForQuery(db, store.FilterParams{}, "stripe_event_id idempotency")
	require.NoError(t, err)
	require.Len(t, candidates, 1,
		"expected 1 candidate, got %d", len(candidates))
	assert.Equal(t, "section_aware", candidates[0].Metadata["indexed_section_retrieval_mode"],
		"missing section-aware metadata: %#v", candidates[0].Metadata)
	assert.Equal(t, "1", candidates[0].Metadata["indexed_section_match_count"],
		"expected 1 section match, got %#v", candidates[0].Metadata)

}
