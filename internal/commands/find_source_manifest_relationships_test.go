package commands

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectFindSourceManifestRelationshipCandidates_SelectedTestImport_RecoversImplementation(t *testing.T) {
	selected := []retrieval.Candidate{{
		Path:    "test/fast/Unit tests/nvm_validate_install",
		Kind:    "source_context",
		Subtype: "test_case",
	}}
	rows := []findSourceTestManifestRow{
		{FileID: "nvm", Path: "nvm.sh", SourceRole: "implementation", Symbols: "nvm_validate_install"},
		{FileID: "test", Path: "test/fast/Unit tests/nvm_validate_install", SourceRole: "test", Imports: "../../../nvm.sh"},
	}

	got := selectFindSourceManifestRelationshipCandidates("validate installed node version", selected, nil, rows, 4)

	require.Len(t, got, 1)
	assert.Equal(t, "nvm.sh", got[0].Path)
	assert.Equal(t, "source_manifest_relationship_recovery", got[0].Metadata["retrieval_expansion_reason"])
}

func TestSelectFindSourceManifestRelationshipCandidates_SelectedLibrary_RecoversEntrypointImporter(t *testing.T) {
	selected := []retrieval.Candidate{{Path: "lib/selection.sh", Kind: "source_context"}}
	rows := []findSourceTestManifestRow{
		{FileID: "entry", Path: "bin/tool", SourceRole: "implementation", Imports: "$TOOL_LIB/selection.sh"},
		{FileID: "selection", Path: "lib/selection.sh", SourceRole: "implementation", Symbols: "select_provider"},
	}

	got := selectFindSourceManifestRelationshipCandidates("selection provider default", selected, nil, rows, 4)

	require.Len(t, got, 1)
	assert.Equal(t, "bin/tool", got[0].Path)
	assert.Contains(t, got[0].Metadata["source_manifest_relationship_reasons"], "selected_file_importer")
}

func TestSelectFindSourceManifestRelationshipCandidates_SharedImport_RecoversSiblingEntrypoint(t *testing.T) {
	selected := []retrieval.Candidate{{Path: "scripts/verify.sh", Kind: "source_context", Subtype: "test_case"}}
	rows := []findSourceTestManifestRow{
		{FileID: "entry", Path: "bin/tool", SourceRole: "implementation", Imports: "$TOOL_LIB/core.sh"},
		{FileID: "core", Path: "lib/core.sh", SourceRole: "implementation", Symbols: "checkpoint"},
		{FileID: "verify", Path: "scripts/verify.sh", SourceRole: "test", Imports: "$root/lib/core.sh"},
	}

	got := selectFindSourceManifestRelationshipCandidates("verification checkpoint failure", selected, nil, rows, 4)

	require.Len(t, got, 2)
	assert.Equal(t, "lib/core.sh", got[0].Path)
	assert.Equal(t, "bin/tool", got[1].Path)
	assert.Contains(t, got[1].Metadata["source_manifest_relationship_reasons"], "shared_import_target")
}

func TestSelectFindSourceManifestRelationshipCandidates_UncoveredBehaviorTerm_RecoversNamedTest(t *testing.T) {
	selected := []retrieval.Candidate{{
		Path:     "test/formatter.bats",
		Kind:     "source_context",
		Subtype:  "test_case",
		Body:     "The formatter receives timeout metadata from the runner.",
		Metadata: map[string]string{"retrieval_candidate": "source_manifest", "test_name": "formats TAP results with timeout metadata"},
	}}
	rows := []findSourceTestManifestRow{
		{FileID: "formatter", Path: "test/formatter.bats", SourceRole: "test", TestNames: "formats TAP results"},
		{FileID: "timeout", Path: "test/timeout.bats", SourceRole: "test", TestNames: "kills a test after timeout"},
	}

	got := selectFindSourceManifestRelationshipCandidates("TAP formatter timeout and test result behavior", selected, nil, rows, 4)

	require.Len(t, got, 1)
	assert.Equal(t, "test/timeout.bats", got[0].Path)
	assert.Contains(t, got[0].Metadata["source_manifest_relationship_reasons"], "uncovered_behavior_anchor:timeout")
}

func TestSelectFindSourceManifestRelationshipCandidates_LowerRankedNamedTest_PromotesExistingCandidate(t *testing.T) {
	selected := []retrieval.Candidate{
		{Path: "test/formatter.bats", Kind: "source_context", Subtype: "test_case", Metadata: map[string]string{"test_name": "formats TAP results"}},
		{Path: "test/tap13-formatter.bats", Kind: "source_context", Subtype: "test_case"},
		{Path: "test/cat-formatter.bats", Kind: "source_context", Subtype: "test_case"},
		{Path: "test/pretty-formatter.bats", Kind: "source_context", Subtype: "test_case"},
		{Path: "test/junit-formatter.bats", Kind: "source_context", Subtype: "test_case"},
		{Path: "test/timeout.bats", Kind: "source_context", Subtype: "test_case"},
	}
	rows := []findSourceTestManifestRow{
		{FileID: "formatter", Path: "test/formatter.bats", SourceRole: "test", TestNames: "formats TAP results"},
		{FileID: "timeout", Path: "test/timeout.bats", SourceRole: "test", TestNames: "kills a test after timeout"},
	}

	got := selectFindSourceManifestRelationshipCandidates("TAP formatter timeout and test result behavior", selected, nil, rows, 4)

	require.Len(t, got, 1)
	assert.Equal(t, "test/timeout.bats", got[0].Path)
	assert.Equal(t, retrieval.PackTierPrimary, got[0].Metadata["pack_tier"])
}
