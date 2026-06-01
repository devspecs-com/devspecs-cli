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
	for _, want := range []string{"Repo map: payments-api", "Candidate areas", "Try: ds find --pack"} {
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
}
