package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectMapBoundaryArtifacts_WithCrowdedTests_PreservesSourcesAndTwoTestAnchors(t *testing.T) {
	ranked := []mapArtifact{
		{Path: "src/one.go", Kind: "source_context"},
		{Path: "src/two.go", Kind: "source_context"},
		{Path: "docs/readme.md", Kind: "markdown_artifact"},
		{Path: "tests/one_test.go", Kind: "test_case"},
		{Path: "tests/two_test.go", Kind: "test_case"},
		{Path: "tests/three_test.go", Kind: "test_case"},
	}

	got := selectMapBoundaryArtifacts(ranked, 2, map[string]bool{"path_boundary": true})

	require.Len(t, got, 4)
	assert.Equal(t, "src/one.go", got[0].Path)
	assert.Equal(t, "src/two.go", got[1].Path)
	assert.Equal(t, "tests/one_test.go", got[2].Path)
	assert.Equal(t, "tests/two_test.go", got[3].Path)
}

func TestSelectMapBoundaryArtifacts_WithOneSelectedTest_AddsOnlyOneAnchor(t *testing.T) {
	ranked := []mapArtifact{
		{Path: "src/one.go", Kind: "source_context"},
		{Path: "tests/one_test.go", Kind: "test_case"},
		{Path: "tests/two_test.go", Kind: "test_case"},
		{Path: "tests/three_test.go", Kind: "test_case"},
	}

	got := selectMapBoundaryArtifacts(ranked, 2, map[string]bool{"path_boundary": true})

	require.Len(t, got, 3)
	assert.Equal(t, "src/one.go", got[0].Path)
	assert.Equal(t, "tests/one_test.go", got[1].Path)
	assert.Equal(t, "tests/two_test.go", got[2].Path)
}

func TestSelectMapBoundaryArtifacts_WithTwoSelectedTests_KeepsOriginalBudget(t *testing.T) {
	ranked := []mapArtifact{
		{Path: "tests/one_test.go", Kind: "test_case"},
		{Path: "tests/two_test.go", Kind: "test_case"},
		{Path: "tests/three_test.go", Kind: "test_case"},
	}

	got := selectMapBoundaryArtifacts(ranked, 2, map[string]bool{"path_boundary": true})

	require.Len(t, got, 2)
	assert.Equal(t, "tests/one_test.go", got[0].Path)
	assert.Equal(t, "tests/two_test.go", got[1].Path)
}

func TestSelectMapBoundaryArtifacts_WithOnlySource_KeepsOriginalBudget(t *testing.T) {
	ranked := []mapArtifact{{Path: "one.go"}, {Path: "two.go"}, {Path: "three.go"}}

	got := selectMapBoundaryArtifacts(ranked, 2, map[string]bool{"path_boundary": true})

	require.Len(t, got, 2)
	assert.Equal(t, "one.go", got[0].Path)
	assert.Equal(t, "two.go", got[1].Path)
}

func TestSelectMapBoundaryArtifacts_BelowBudget_PreservesAllEvidence(t *testing.T) {
	ranked := []mapArtifact{{Path: "one.go"}, {Path: "one_test.go"}}

	got := selectMapBoundaryArtifacts(ranked, 24, map[string]bool{"path_boundary": true})

	require.Len(t, got, 2)
	assert.Equal(t, "one.go", got[0].Path)
	assert.Equal(t, "one_test.go", got[1].Path)
}

func TestSelectMapBoundaryArtifacts_WithConceptualParent_KeepsRankedCapWithoutUnrelatedTests(t *testing.T) {
	ranked := []mapArtifact{
		{Path: "src/one.go", Kind: "source_context"},
		{Path: "src/two.go", Kind: "source_context"},
		{Path: "tests/translation_cli_test.go", Kind: "test_case"},
		{Path: "tests/fixture_test.go", Kind: "test_case"},
	}

	got := selectMapBoundaryArtifacts(ranked, 2, map[string]bool{"path_boundary": true, "conceptual_parent": true})

	require.Len(t, got, 2)
	assert.Equal(t, "src/one.go", got[0].Path)
	assert.Equal(t, "src/two.go", got[1].Path)
}
