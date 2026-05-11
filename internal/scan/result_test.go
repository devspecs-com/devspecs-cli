package scan

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/format"
)

func TestResult_finalizeSourcesBreakdown_Sums(t *testing.T) {
	r := newResult([]string{"markdown", "openspec", "adr"})
	tallyIndexed(r, "markdown", []adapters.Source{{SourceType: "markdown", FormatProfile: format.ProfileGeneric}}, adapters.Artifact{})
	tallyIndexed(r, "markdown", []adapters.Source{{SourceType: "markdown", FormatProfile: format.ProfileCursorPlan}}, adapters.Artifact{})
	tallyIndexed(r, "openspec", []adapters.Source{{SourceType: "openspec", FormatProfile: format.ProfileOpenspec}}, adapters.Artifact{})
	r.finalizeSourcesBreakdown()

	var totalIndexed, sumBreakdown int
	for _, n := range r.Found {
		totalIndexed += n
	}
	for _, row := range r.SourcesBreakdown {
		sumBreakdown += row.Count
		sumFormats := 0
		for _, c := range row.Formats {
			sumFormats += c
		}
		if sumFormats != row.Count {
			t.Errorf("%s: format counts sum %d != row count %d", row.SourceType, sumFormats, row.Count)
		}
	}
	if totalIndexed != sumBreakdown {
		t.Errorf("Found total %d != sources_breakdown count sum %d", totalIndexed, sumBreakdown)
	}
	if len(r.SourcesBreakdown) != 3 {
		t.Fatalf("expected 3 breakdown rows, got %d", len(r.SourcesBreakdown))
	}
}
