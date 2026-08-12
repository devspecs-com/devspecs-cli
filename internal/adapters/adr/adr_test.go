package adr

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdapter_Name_ReturnsADR(t *testing.T) {
	adapter := &Adapter{}

	name := adapter.Name()

	assert.Equal(t, "adr", name)
}

func TestDiscover_WithDefaultADRPath_ReturnsCandidate(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "docs/adr/0001-use-sqlite.md", "# Use SQLite\n\nStatus: Accepted\n")
	adapter := &Adapter{}

	candidates, err := adapter.Discover(context.Background(), root, nil)

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, "docs/adr/0001-use-sqlite.md", filepath.ToSlash(candidates[0].RelPath))
}

func TestDiscover_WithConfiguredADRPath_ReturnsCandidate(t *testing.T) {
	root := t.TempDir()
	writeADRFile(t, root, "architecture/decisions/001.md", "# Test\n")
	cfg := &config.RepoConfig{Sources: []config.SourceConfig{{Type: "adr", Paths: []string{"architecture/decisions"}}}}
	adapter := &Adapter{}

	candidates, err := adapter.Discover(context.Background(), root, cfg)

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, "architecture/decisions/001.md", filepath.ToSlash(candidates[0].RelPath))
}

func TestDiscover_WithIgnoredADRPath_ReturnsOnlyUnignoredCandidate(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("secret-adrs/\n"), 0o644))
	writeADRFile(t, root, "adr/0001.md", "# A\n")
	writeADRFile(t, root, "secret-adrs/0002.md", "# B\n")
	matcher, err := ignore.NewMatcher(root)
	require.NoError(t, err)
	ctx := ignore.WithContext(context.Background(), matcher)
	adapter := &Adapter{}

	candidates, err := adapter.Discover(ctx, root, nil)

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, "adr/0001.md", filepath.ToSlash(candidates[0].RelPath))
}

func TestParse_WithStatusLine_ExtractsTitleAndAcceptedStatus(t *testing.T) {
	candidate := writeADRParseFixture(t, "# Use SQLite for storage\n\nStatus: Accepted\n\n## Context\nWe need local storage.\n")
	adapter := &Adapter{}

	artifact, _, _, err := adapter.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assertADRArtifact(t, artifact, "Use SQLite for storage", "accepted")
}

func TestParse_WithStatusHeading_ExtractsTitleAndProposedStatus(t *testing.T) {
	candidate := writeADRParseFixture(t, "# Use Go for CLI\n\n## Status\n\nProposed\n\n## Context\n")
	adapter := &Adapter{}

	artifact, _, _, err := adapter.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assertADRArtifact(t, artifact, "Use Go for CLI", "proposed")
}

func TestParse_WithFrontmatter_UsesFrontmatterTitleAndStatus(t *testing.T) {
	candidate := writeADRParseFixture(t, "---\ntitle: Auth with JWT\nstatus: accepted\n---\n\n# Different Title\n")
	adapter := &Adapter{}

	artifact, _, _, err := adapter.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assertADRArtifact(t, artifact, "Auth with JWT", "accepted")
}

func TestParse_WithMADRMetadata_ExtractsProposedStatus(t *testing.T) {
	candidate := writeADRParseFixture(t, "# Use Markdown Architectural Decision Records\n\n* Status: proposed\n* Deciders: team\n* Date: 2023-01-01\n")
	adapter := &Adapter{}

	artifact, _, _, err := adapter.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assertADRArtifact(t, artifact, "Use Markdown Architectural Decision Records", "proposed")
}

func TestParse_WithoutStatus_ReturnsUnknownStatus(t *testing.T) {
	candidate := writeADRParseFixture(t, "# Simple ADR\n\nSome content without status.\n")
	adapter := &Adapter{}

	artifact, _, _, err := adapter.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assertADRArtifact(t, artifact, "Simple ADR", "unknown")
}

func TestParse_WithSupersededStatus_ReturnsSuperseded(t *testing.T) {
	candidate := writeADRParseFixture(t, "# Old Decision\n\nStatus: Superseded by ADR-0005\n")
	adapter := &Adapter{}

	artifact, _, _, err := adapter.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assertADRArtifact(t, artifact, "Old Decision", "superseded")
}

func writeADRFile(t *testing.T, root, relPath, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func writeADRParseFixture(t *testing.T, content string) adapters.Candidate {
	t.Helper()
	path := writeADRFile(t, t.TempDir(), "docs/adr/test.md", content)
	return adapters.Candidate{PrimaryPath: path, RelPath: "docs/adr/test.md", AdapterName: "adr"}
}

func assertADRArtifact(t *testing.T, artifact adapters.Artifact, title, status string) {
	t.Helper()
	assert.Equal(t, title, artifact.Title)
	assert.Equal(t, status, artifact.Status)
	assert.Equal(t, "decision", artifact.Kind)
	assert.Equal(t, "adr", artifact.Subtype)
	assert.Equal(t, format.ProfileADR, artifact.FormatProfile)
}
