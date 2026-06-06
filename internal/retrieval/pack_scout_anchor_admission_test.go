package retrieval

import "testing"

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
	if len(got) <= len(selected) {
		t.Fatalf("expected dominant bookmark anchor admissions, got %#v", CandidatePaths(got))
	}
	want := []string{
		"app/src/modules/content/components/bookmark-add.vue",
		"app/src/modules/content/components/bookmark-delete.vue",
		"app/src/modules/content/composables/use-delete-bookmark.ts",
		"app/src/modules/content/composables/use-delete-bookmark.test.ts",
	}
	for _, path := range want {
		if !containsCandidatePath(got, path) {
			t.Fatalf("missing admitted bookmark candidate %s, got %#v", path, CandidatePaths(got))
		}
	}
	for _, candidate := range got[len(selected):] {
		if candidate.Metadata["retrieval_expansion_reason"] != "scout_anchor_admission" {
			t.Fatalf("missing scout admission metadata: %#v", candidate.Metadata)
		}
		if candidate.Metadata["pack_tier"] != PackTierPrimary {
			t.Fatalf("admitted anchor should start primary for family-primary selection: %#v", candidate.Metadata)
		}
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
	if len(got) != len(selected) {
		t.Fatalf("expected no admission when dominant anchor is already selected, got %#v", CandidatePaths(got))
	}
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
	if len(got) != len(selected) {
		t.Fatalf("symbol-only dominant anchor should not be admitted, got %#v", CandidatePaths(got))
	}
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
	if len(got) != len(selected) {
		t.Fatalf("thin source cluster should not displace selected rows, got %#v", CandidatePaths(got))
	}
}
