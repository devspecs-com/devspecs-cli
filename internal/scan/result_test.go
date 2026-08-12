package scan

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResult_finalizeSourcesBreakdown_Sums(t *testing.T) {
	r := newResult([]string{"markdown", "openspec", "adr", "source_context"})
	tallyIndexed(r, "markdown", []adapters.Source{{SourceType: "markdown", FormatProfile: format.ProfileGeneric}}, adapters.Artifact{})
	tallyIndexed(r, "markdown", []adapters.Source{{SourceType: "markdown", FormatProfile: format.ProfileCursorPlan}}, adapters.Artifact{})
	tallyIndexed(r, "openspec", []adapters.Source{{SourceType: "openspec", FormatProfile: format.ProfileOpenspec}}, adapters.Artifact{})
	r.finalizeSourcesBreakdown()

	require.Len(t, r.SourcesBreakdown, 4)
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
		assert.Equal(t, row.Count, sumFormats,
			"%s: format counts sum %d != row count %d", row.SourceType, sumFormats, row.Count)

	}
	assert.Equal(t, sumBreakdown, totalIndexed,
		"Found total %d != sources_breakdown count sum %d", totalIndexed, sumBreakdown)
}

func TestResult_finalizeSourcesBreakdown_IncludesTestCasesWhenEnabled(t *testing.T) {
	r := newResult([]string{"markdown", "openspec", "adr", "source_context", "test_case"})
	r.finalizeSourcesBreakdown()
	require.Len(t, r.SourcesBreakdown, 5,
		"expected 5 breakdown rows, got %d", len(r.SourcesBreakdown))

	last := r.SourcesBreakdown[4]
	assert.Equal(t, "test_case", last.SourceType)
	assert.Equal(t, "Test cases", last.Label)
}

func TestResult_finalizeSourcesBreakdown_IncludesCodeCommentsWhenEnabled(t *testing.T) {
	r := newResult([]string{"markdown", "openspec", "adr", "source_context", "code_comment"})
	r.finalizeSourcesBreakdown()
	require.Len(t, r.SourcesBreakdown, 5,
		"expected 5 breakdown rows, got %d", len(r.SourcesBreakdown))

	last := r.SourcesBreakdown[4]
	assert.Equal(t, "code_comment", last.SourceType)
	assert.Equal(t, "Code comments", last.Label)
}
