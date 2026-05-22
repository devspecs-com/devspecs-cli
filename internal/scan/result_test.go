package scan

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/format"
)

func TestResult_finalizeSourcesBreakdown_Sums(t *testing.T) {
	r := newResult([]string{"markdown", "openspec", "adr", "source_context"})
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
	if len(r.SourcesBreakdown) != 4 {
		t.Fatalf("expected 4 breakdown rows, got %d", len(r.SourcesBreakdown))
	}
}

func TestResult_finalizeSourcesBreakdown_IncludesTestCasesWhenEnabled(t *testing.T) {
	r := newResult([]string{"markdown", "openspec", "adr", "source_context", "test_case"})
	r.finalizeSourcesBreakdown()
	if len(r.SourcesBreakdown) != 5 {
		t.Fatalf("expected 5 breakdown rows, got %d", len(r.SourcesBreakdown))
	}
	last := r.SourcesBreakdown[len(r.SourcesBreakdown)-1]
	if last.SourceType != "test_case" || last.Label != "Test cases" {
		t.Fatalf("last row = %#v", last)
	}
}
