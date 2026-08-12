package openspecmetrics

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsHasData_WithEmptyMetrics_ReturnsFalse(t *testing.T) {
	metrics := Metrics{}

	hasData := metrics.HasData()

	assert.False(t, hasData)
}

func TestMetricsHasData_WithMarkdownLeakage_ReturnsTrue(t *testing.T) {
	metrics := Metrics{MarkdownLeakage: 1}

	hasData := metrics.HasData()

	assert.True(t, hasData)
}

func TestAnalyzeOpenSpecMetrics(t *testing.T) {
	repoRoot := t.TempDir()
	changeDir := filepath.Join(repoRoot, "openspec", "changes", "add-sso")
	specDir := filepath.Join(changeDir, "specs", "auth")
	nestedChangeDir := filepath.Join(repoRoot, "services", "collector", "openspec", "changes", "add-flow")
	for _, dir := range []string{changeDir, specDir, nestedChangeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			require.NoError(t, err)
		}
	}
	for _, rel := range []string{
		"openspec/changes/add-sso/proposal.md",
		"openspec/changes/add-sso/design.md",
		"openspec/changes/add-sso/tasks.md",
		"openspec/changes/add-sso/specs/auth/spec.md",
		"services/collector/openspec/changes/add-flow/proposal.md",
	} {
		if err := os.WriteFile(filepath.Join(repoRoot, filepath.FromSlash(rel)), []byte("# Test\n"), 0o644); err != nil {
			require.NoError(t, err)
		}
	}

	got := Analyze(repoRoot, []Artifact{
		{Path: "openspec/changes/add-sso", SourceType: "openspec", Subtype: "openspec_change_bundle", ArtifactScope: "bundle", OpenSpecRole: "change_bundle"},
		{Path: "openspec/changes/add-sso/proposal.md", SourceType: "openspec", Subtype: "openspec_child", ArtifactScope: "file", OpenSpecRole: "proposal"},
		{Path: "openspec/changes/add-sso/design.md", SourceType: "openspec", Subtype: "openspec_child", ArtifactScope: "file", OpenSpecRole: "design"},
		{Path: "openspec/changes/add-sso/tasks.md", SourceType: "openspec", Subtype: "openspec_child", ArtifactScope: "file", OpenSpecRole: "tasks"},
		{Path: "openspec/changes/add-sso/specs/auth/spec.md", SourceType: "openspec", Subtype: "openspec_child", ArtifactScope: "file", OpenSpecRole: "spec_delta"},
		{Path: "openspec/changes/add-sso/proposal.md", SourceType: "markdown"},
		{Path: "services/collector/openspec/changes/add-flow", SourceType: "openspec", Subtype: "openspec_change_bundle", ArtifactScope: "bundle", OpenSpecRole: "change_bundle"},
		{Path: "services/collector/openspec/changes/add-flow/proposal.md", SourceType: "markdown", Subtype: "spec"},
	})
	assert.Equal(t, 2, got.ExpectedBundles)
	assert.Equal(t, 2, got.IndexedBundles)
	assert.InDelta(t, 1.0, got.BundleRecall, 0)
	assert.Equal(t, 5, got.ExpectedChildArtifacts)
	assert.Equal(t, 5, got.IndexedChildArtifacts)
	assert.InDelta(t, 1.0, got.ChildRoleRecall, 0)
	assert.InDelta(t, 2.5, got.DuplicatePressure, 0)
	assert.Equal(t, 2, got.MarkdownLeakage)
	require.Len(t, got.MarkdownLeakagePaths, 2)
}

func TestAnalyzeOpenSpecMetricsReportsMissingChildren(t *testing.T) {
	repoRoot := t.TempDir()
	changeDir := filepath.Join(repoRoot, "openspec", "changes", "add-sso")

	require.NoError(t, os.MkdirAll(changeDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte("# Test\n"), 0o644))

	got := Analyze(repoRoot, []Artifact{
		{Path: "openspec/changes/add-sso", SourceType: "openspec", Subtype: "openspec_change_bundle"},
	})
	require.InDelta(t, 1.0, got.BundleRecall, 0,
		"bundle recall = %.3f", got.BundleRecall)
	require.InDelta(t, 0.0, got.ChildRoleRecall, 0,
		"child role recall = %.3f", got.ChildRoleRecall)
	require.Len(t, got.MissingChildRoles, 1,
		"missing child roles = %#v", got.MissingChildRoles)
	assert.Equal(t, "proposal:openspec/changes/add-sso/proposal.md", got.MissingChildRoles[0])

}
