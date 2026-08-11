package retrieval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddScoutAnchorAdmissionCandidatesAddsDominantAnchorMiss(t *testing.T) {
	selected := []Candidate{
		{
			Path:  "api/src/ai/tools/trigger-flow/index.ts",
			Kind:  "source_context",
			Title: "Trigger flow tool",
			Body:  "Flow trigger implementation.",
		},
		{
			Path:  "api/src/utils/construct-flow-tree.ts",
			Kind:  "source_context",
			Title: "Construct flow tree",
			Body:  "Flow tree utility for automation flows.",
		},
	}
	universe := append([]Candidate(nil), selected...)
	universe = append(universe,
		Candidate{
			Path:  "app/src/modules/settings/routes/flows/flow.vue",
			Kind:  "source_context",
			Title: "Flow settings route",
			Body:  "Flow route for settings and manual flow selection.",
		},
		Candidate{
			Path:  "app/src/modules/content/components/bookmark-add.vue",
			Kind:  "source_context",
			Title: "Bookmark add component",
			Body:  "Improve bookmark flow by adding bookmark handling.",
		},
		Candidate{
			Path:  "app/src/modules/content/components/bookmark-delete.vue",
			Kind:  "source_context",
			Title: "Bookmark delete component",
			Body:  "Improve bookmark deletion flow.",
		},
		Candidate{
			Path:  "app/src/modules/content/composables/use-delete-bookmark.ts",
			Kind:  "source_context",
			Title: "Use delete bookmark composable",
			Body:  "Bookmark flow delete implementation.",
		},
		Candidate{
			Path:    "app/src/modules/content/composables/use-delete-bookmark.test.ts",
			Kind:    "source_context",
			Subtype: "test_case",
			Title:   "delete bookmark flow",
			Body:    "test bookmark delete behavior.",
		},
	)

	got := AddScoutAnchorAdmissionCandidates(selected, universe, "Improve bookmark flow")
	require.Greater(t, len(got), len(selected),
		"expected dominant bookmark anchor admissions, got %#v", CandidatePaths(got))

	assert.True(t, containsCandidatePath(got, "app/src/modules/content/components/bookmark-add.vue"),
		"missing admitted bookmark add candidate: %#v", CandidatePaths(got))
	assert.True(t, containsCandidatePath(got, "app/src/modules/content/components/bookmark-delete.vue"),
		"missing admitted bookmark delete candidate: %#v", CandidatePaths(got))
	assert.True(t, containsCandidatePath(got, "app/src/modules/content/composables/use-delete-bookmark.ts"),
		"missing admitted bookmark composable: %#v", CandidatePaths(got))
	assert.True(t, containsCandidatePath(got, "app/src/modules/content/composables/use-delete-bookmark.test.ts"),
		"missing admitted bookmark test: %#v", CandidatePaths(got))
	for _, candidate := range got[len(selected):] {
		require.Equal(t, "scout_anchor_admission", candidate.Metadata["retrieval_expansion_reason"],
			"missing scout admission metadata: %#v", candidate.Metadata)
		require.Equal(t, PackTierPrimary, candidate.Metadata["pack_tier"],
			"admitted anchor should start primary for family-primary selection: %#v", candidate.Metadata)

	}
}

func TestAddScoutAnchorAdmissionCandidatesNoopsWhenDominantAnchorAlreadySelected(t *testing.T) {
	selected := []Candidate{
		{
			Path:  "app/src/modules/content/components/bookmark-add.vue",
			Kind:  "source_context",
			Title: "Bookmark add component",
			Body:  "Improve bookmark flow by adding bookmark handling.",
		},
	}
	universe := append([]Candidate(nil), selected...)
	universe = append(universe, Candidate{
		Path:  "api/src/ai/tools/trigger-flow/index.ts",
		Kind:  "source_context",
		Title: "Trigger flow tool",
		Body:  "Flow trigger implementation.",
	})

	got := AddScoutAnchorAdmissionCandidates(selected, universe, "Improve bookmark flow")
	require.Len(t, got, len(selected),
		"expected no admission when dominant anchor is already selected, got %#v", CandidatePaths(got))

}

func TestAddScoutAnchorAdmissionCandidatesSkipsSymbolOnlyDominantAnchor(t *testing.T) {
	selected := []Candidate{
		{
			Path:  "api/src/ai/tools/trigger-flow/index.ts",
			Kind:  "source_context",
			Title: "Trigger flow tool",
			Body:  "Flow trigger implementation.",
		},
	}
	universe := append([]Candidate(nil), selected...)
	universe = append(universe, Candidate{
		Path:  "app/src/components/v-menu.vue",
		Kind:  "source_context",
		Title: "Menu component",
		Body:  "Generic state toggles for interface behavior.",
		Metadata: map[string]string{
			"symbols": "bookmark",
		},
	})

	got := AddScoutAnchorAdmissionCandidates(selected, universe, "Improve bookmark flow")
	require.Len(t, got, len(selected),
		"symbol-only dominant anchor should not be admitted, got %#v", CandidatePaths(got))

}

func TestAddScoutAnchorAdmissionCandidatesRequiresSourceCluster(t *testing.T) {
	selected := []Candidate{
		{
			Path:  "app/src/interfaces/list-m2m/list-m2m.vue",
			Kind:  "source_context",
			Title: "list m2m",
			Body:  "non-editable interface state",
		},
	}
	universe := append([]Candidate(nil), selected...)
	universe = append(universe,
		Candidate{
			Path:  "api/src/utils/deep-freeze.ts",
			Kind:  "source_context",
			Title: "deep freeze",
			Body:  "freeze utility",
		},
		Candidate{
			Path:  "api/src/utils/freeze-schema.ts",
			Kind:  "source_context",
			Title: "freeze schema",
			Body:  "schema freeze utility",
		},
	)

	got := AddScoutAnchorAdmissionCandidates(selected, universe, "Fix UI freeze when non-editable state toggles")
	require.Len(t, got, len(selected),
		"thin source cluster should not displace selected rows, got %#v", CandidatePaths(got))

}
