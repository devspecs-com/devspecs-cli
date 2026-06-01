package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/scan"
)

func TestMapTextHidesReviewerDiagnosticsByDefault(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "payments-api")
	out := buildMapOutput(repoRoot, &scan.Result{
		Found: map[string]int{
			"source_context": 2,
			"test_case":      1,
		},
		WorkstreamEvidence: &scan.WorkstreamEvidenceDiagnostics{
			TopClusters: []scan.WorkstreamClusterExample{
				{
					Anchor:        "stripe webhook retry",
					Confidence:    0.9,
					EvidenceCount: 6,
					ExampleArtifacts: []scan.WorkstreamArtifactExample{
						{Kind: "source_context", Path: "internal/billing/webhook.go"},
						{Kind: "source_context", Path: "internal/billing/retry.go"},
						{Kind: "test_case", Path: "internal/billing/webhook_test.go"},
					},
				},
			},
		},
	}, mapOptions{MaxAreas: 4})

	var buf bytes.Buffer
	writeMapText(&buf, out, false)
	text := buf.String()
	for _, notWant := range []string{"Try changed", "Receipt changed", "Aha", "raw signal", "class=", "confidence="} {
		if strings.Contains(text, notWant) {
			t.Fatalf("default map output leaked reviewer diagnostic %q:\n%s", notWant, text)
		}
	}
	for _, want := range []string{"Repo map: payments-api", "Candidate areas", "Type:", "Try: ds find --pack"} {
		if !strings.Contains(text, want) {
			t.Fatalf("default map output missing %q:\n%s", want, text)
		}
	}
}

func TestMapJSONSchemaIsAgentReadable(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "orders")
	out := buildMapOutput(repoRoot, &scan.Result{
		Found: map[string]int{"source_context": 1},
		WorkstreamEvidence: &scan.WorkstreamEvidenceDiagnostics{
			TopClusters: []scan.WorkstreamClusterExample{
				{
					Anchor:        "order fulfillment",
					Confidence:    0.8,
					EvidenceCount: 3,
					ExampleArtifacts: []scan.WorkstreamArtifactExample{
						{Kind: "source_context", Path: "src/orders/fulfillment.go"},
					},
				},
			},
		},
	}, mapOptions{MaxAreas: 2})

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var decoded mapOutput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("map JSON did not round trip: %v\n%s", err, string(data))
	}
	if decoded.Schema != mapSchemaVersion {
		t.Fatalf("schema = %q, want %q", decoded.Schema, mapSchemaVersion)
	}
	if decoded.Repo.Name != "orders" {
		t.Fatalf("repo name = %q", decoded.Repo.Name)
	}
	if len(decoded.Areas) == 0 {
		t.Fatalf("expected at least one area in JSON:\n%s", string(data))
	}
	if decoded.Areas[0].AreaType == "" {
		t.Fatalf("expected area_type in JSON:\n%s", string(data))
	}
}

func TestMapEvidenceCountsUsesPathFamilies(t *testing.T) {
	counts := mapEvidenceCounts([]scan.WorkstreamArtifactExample{
		{Kind: "source_context", Path: "src/click/core.py"},
		{Kind: "source_context", Path: "tests/test_core.py"},
		{Kind: "source_context", Path: "docs_src/tutorial001.py"},
	})
	if counts["source"] != 1 || counts["test"] != 1 || counts["doc"] != 1 {
		t.Fatalf("unexpected path-aware families: %#v", counts)
	}
}

func TestMapRootOnlyAreaStaysLowConfidence(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "requests")
	out := buildMapOutput(repoRoot, &scan.Result{
		Found: map[string]int{"source_context": 3},
		WorkstreamEvidence: &scan.WorkstreamEvidenceDiagnostics{
			TopClusters: []scan.WorkstreamClusterExample{
				{
					Anchor:        "requests",
					Confidence:    0.95,
					EvidenceCount: 8,
					ExampleArtifacts: []scan.WorkstreamArtifactExample{
						{Kind: "source_context", Path: "src/requests/api.py"},
						{Kind: "source_context", Path: "src/requests/sessions.py"},
					},
				},
			},
		},
	}, mapOptions{MaxAreas: 3})
	if out.Repo.Confidence != mapLowConfidence {
		t.Fatalf("root-only map confidence = %q, want low; areas=%#v", out.Repo.Confidence, out.Areas)
	}
	if len(out.Areas) == 0 || !out.Areas[0].IsRepoRootUmbrella {
		t.Fatalf("expected root umbrella area, got %#v", out.Areas)
	}
	if out.Areas[0].AreaType != mapTypeRoot {
		t.Fatalf("root area type = %q, want %q", out.Areas[0].AreaType, mapTypeRoot)
	}
}

func TestMapAreaTypeClassifiesProductBoundaries(t *testing.T) {
	out := buildProductMapTestOutput(t)

	types := map[string]string{}
	for _, area := range out.Areas {
		types[area.Label] = area.AreaType
	}
	if types["Flowable"] != mapTypeExternal {
		t.Fatalf("Flowable type = %q, want external integration; all=%#v", types["Flowable"], types)
	}
	if types["Status Pill"] != mapTypeUI {
		t.Fatalf("Status Pill type = %q, want UI surface; all=%#v", types["Status Pill"], types)
	}
	if types["Submission"] != mapTypeBusinessFlow {
		t.Fatalf("Submission type = %q, want business workflow; all=%#v", types["Submission"], types)
	}
}

func TestMapAreaDrilldownIsActionable(t *testing.T) {
	out := buildProductMapTestOutput(t)
	var buf bytes.Buffer
	writeMapAreaText(&buf, out, "submission", false)
	text := buf.String()
	for _, want := range []string{
		"Map area: Submission",
		"Type: business workflow",
		"Key files:",
		"apps/api/internal/submission/redaction.go",
		"Pack this context:",
		`ds find --pack "submission redaction"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("area drilldown missing %q:\n%s", want, text)
		}
	}
}

func TestMapAreaDrilldownNoMatchListsAvailableAreas(t *testing.T) {
	out := buildProductMapTestOutput(t)
	var buf bytes.Buffer
	writeMapAreaText(&buf, out, "not-a-real-area", false)
	text := buf.String()
	for _, want := range []string{"No matching map area found.", "Available areas:", "Submission", "Flowable"} {
		if !strings.Contains(text, want) {
			t.Fatalf("no-match drilldown missing %q:\n%s", want, text)
		}
	}
}

func TestFilterMapOutputByAreaQueryNarrowsJSONPayload(t *testing.T) {
	out := buildProductMapTestOutput(t)
	filtered := filterMapOutputByAreaQuery(out, "redaction")
	if len(filtered.Areas) != 1 {
		t.Fatalf("filtered area count = %d, want 1; areas=%#v", len(filtered.Areas), filtered.Areas)
	}
	if filtered.Areas[0].Label != "Submission" {
		t.Fatalf("filtered label = %q, want Submission", filtered.Areas[0].Label)
	}
	if filtered.Diagnostics.AreaQuery != "redaction" || filtered.Diagnostics.MatchedAreaCount != 1 {
		t.Fatalf("unexpected diagnostics: %#v", filtered.Diagnostics)
	}
}

func TestRefineMapAreaLabelMakesLayerLabelsMoreProductReadable(t *testing.T) {
	if got := refineMapAreaLabel("Lib Anthropic", []string{"Anthropic Ts"}); got != "Anthropic" {
		t.Fatalf("refined lib label = %q, want Anthropic", got)
	}
	if got := refineMapAreaLabel("Application", []string{"Blip Get Canonical Path"}); got != "Blip Application" {
		t.Fatalf("refined application label = %q, want Blip Application", got)
	}
	if got := cleanMapCovers("Game", []string{"Ks", "Rts Camera Mode"}); strings.Join(got, ", ") != "Rts Camera Mode" {
		t.Fatalf("clean covers kept short raw anchor: %#v", got)
	}
}

func buildProductMapTestOutput(t *testing.T) mapOutput {
	t.Helper()
	repoRoot := filepath.Join(t.TempDir(), "product")
	return buildMapOutput(repoRoot, &scan.Result{
		Found: map[string]int{"source_context": 6, "test_case": 2},
		WorkstreamEvidence: &scan.WorkstreamEvidenceDiagnostics{
			TopClusters: []scan.WorkstreamClusterExample{
				{
					Anchor:        "flowable process definitions",
					Confidence:    0.9,
					EvidenceCount: 4,
					ExampleArtifacts: []scan.WorkstreamArtifactExample{
						{Kind: "source_context", Path: "app/api/private/flowable/v1/process_definitions.py"},
						{Kind: "source_context", Path: "app/core/flowable.py"},
					},
				},
				{
					Anchor:        "status pill",
					Confidence:    0.85,
					EvidenceCount: 3,
					ExampleArtifacts: []scan.WorkstreamArtifactExample{
						{Kind: "source_context", Path: "apps/web/components/status-pill.tsx"},
					},
				},
				{
					Anchor:        "submission redaction",
					Confidence:    0.8,
					EvidenceCount: 3,
					ExampleArtifacts: []scan.WorkstreamArtifactExample{
						{Kind: "source_context", Path: "apps/api/internal/submission/redaction.go"},
						{Kind: "test_case", Path: "apps/api/internal/submission/redaction_test.go"},
					},
				},
			},
		},
	}, mapOptions{MaxAreas: 6})
}
