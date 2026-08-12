package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupComposeCommandRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	repoRoot := filepath.Join(tmp, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755))
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	originalWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoRoot))
	t.Cleanup(func() {
		assert.NoError(t, os.Chdir(originalWD))
	})
	return repoRoot
}

func executeComposeJSON(t *testing.T, args ...string) composeOutput {
	t.Helper()
	cmd := NewComposeCmd()
	cmd.SetArgs(append(args, "--json"))
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()

	require.NoError(t, err)
	var output composeOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &output))
	return output
}

func TestComposeADR_WithoutPrecedent_CreatesIndexedNygardDraft(t *testing.T) {
	repoRoot := setupComposeCommandRepo(t)

	output := executeComposeJSON(t, "adr", "Use PostgreSQL", "--no-refresh")

	assert.Equal(t, "docs/adr/0001-use-postgresql.md", output.Path)
	assert.Equal(t, composeTypeADR, output.DocumentType)
	assert.Equal(t, composeADRFormatNygard, output.Format)
	assert.Equal(t, "fallback", output.ConventionSource)
	assert.NotEmpty(t, output.ArtifactID)
	body, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(output.Path)))
	require.NoError(t, err)
	assert.Contains(t, string(body), "# ADR-0001: Use PostgreSQL")
	assert.Contains(t, string(body), "## Consequences")
}

func TestComposeADR_WithOutcomeFirstPrecedent_ReusesDirectoryNumberingAndFormat(t *testing.T) {
	repoRoot := setupComposeCommandRepo(t)
	directory := filepath.Join(repoRoot, "docs", "decisions")
	require.NoError(t, os.MkdirAll(directory, 0o755))
	existing := `# ADR-0003: Use SQLite

## Status
Accepted

## Outcome
One local store.

## Decision
Use SQLite.

## Primary tradeoff
Single-writer limits.

## Why
- Local ownership.

## Decision boundaries
Impacted: storage.
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "0003-use-sqlite.md"), []byte(existing), 0o644))

	output := executeComposeJSON(t, "adr", "Use PostgreSQL", "--no-refresh")

	assert.Equal(t, "docs/decisions/0004-use-postgresql.md", output.Path)
	assert.Equal(t, composeADRFormatOutcomeFirst, output.Format)
	assert.Equal(t, "existing", output.ConventionSource)
	body, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(output.Path)))
	require.NoError(t, err)
	assert.Contains(t, string(body), "# ADR-0004: Use PostgreSQL")
	assert.Contains(t, string(body), "## Decision boundaries")
}

func TestComposeADR_WithConflictingDirectories_ReturnsErrorWithoutWritingDraft(t *testing.T) {
	repoRoot := setupComposeCommandRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "docs", "decisions"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "architecture", "decisions"), 0o755))
	body := "# ADR-0001: Choice\n\n## Status\nAccepted\n\n## Context\nProblem\n\n## Decision\nChoice\n\n## Consequences\nCost\n"
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "docs", "decisions", "0001-choice.md"), []byte(body), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "architecture", "decisions", "0002-other.md"), []byte(body), 0o644))
	cmd := NewComposeCmd()
	cmd.SetArgs([]string{"adr", "Use PostgreSQL", "--no-refresh"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting document directories")
	_, statErr := os.Stat(filepath.Join(repoRoot, "docs", "decisions", "0003-use-postgresql.md"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestComposeRFC_WithoutPrecedent_CreatesDesignDraft(t *testing.T) {
	repoRoot := setupComposeCommandRepo(t)

	output := executeComposeJSON(t, "rfc", "Introduce event replay", "--no-refresh")

	assert.Equal(t, "docs/rfcs/0001-introduce-event-replay.md", output.Path)
	assert.Equal(t, composeTypeRFC, output.DocumentType)
	assert.Equal(t, composeTypeRFC, output.Format)
	body, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(output.Path)))
	require.NoError(t, err)
	assert.Contains(t, string(body), "# RFC: Introduce event replay")
	assert.Contains(t, string(body), "## Open questions")
}

func TestComposePRD_WithoutPrecedent_CreatesRequirementsDraft(t *testing.T) {
	repoRoot := setupComposeCommandRepo(t)

	output := executeComposeJSON(t, "prd", "Workspace onboarding", "--no-refresh")

	assert.Equal(t, "docs/prd/workspace-onboarding.md", output.Path)
	assert.Equal(t, composeTypePRD, output.DocumentType)
	assert.Equal(t, composeTypePRD, output.Format)
	body, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(output.Path)))
	require.NoError(t, err)
	assert.Contains(t, string(body), "# PRD: Workspace onboarding")
	assert.Contains(t, string(body), "## Success measures")
}

func TestComposeADR_WithTaskCorpusOutput_ReturnsError(t *testing.T) {
	setupComposeCommandRepo(t)
	cmd := NewComposeCmd()
	cmd.SetArgs([]string{"adr", "Use PostgreSQL", "--output", "devspecs/tasks/decision.md", "--no-refresh"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the DevSpecs task corpus")
}

func TestComposeADR_WithExplicitOutputOutsideConfiguredSources_CapturesArtifact(t *testing.T) {
	repoRoot := setupComposeCommandRepo(t)

	output := executeComposeJSON(t, "adr", "Use PostgreSQL", "--output", "records/decision.md", "--format", "nygard", "--no-refresh")

	assert.Equal(t, "records/decision.md", output.Path)
	assert.NotEmpty(t, output.ArtifactID)
	dbPath := filepath.Join(filepath.Dir(repoRoot), "home", "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, db.Close()) })
	artifact, err := db.GetArtifact(output.ArtifactID)
	require.NoError(t, err)
	assert.Equal(t, "decision", artifact.Kind)
	assert.Equal(t, "adr", artifact.Subtype)
}

func TestWriteComposeOutput_WithADRHumanOutput_RendersFormatConventionAndGuidance(t *testing.T) {
	cmd := &cobra.Command{}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	output := composeOutput{
		Path:             "docs/adr/0001-use-postgresql.md",
		DocumentType:     composeTypeADR,
		Format:           composeADRFormatMADR,
		Variant:          "minimal",
		ConventionSource: "existing",
		ArtifactID:       "adr_example",
	}

	err := writeComposeOutput(cmd, output, false)

	require.NoError(t, err)
	body := stdout.String()
	assert.Contains(t, body, "Created ADR draft: docs/adr/0001-use-postgresql.md")
	assert.Contains(t, body, "Format: madr (minimal)")
	assert.Contains(t, body, "Convention: existing")
	assert.Contains(t, body, "Artifact: adr_example")
	assert.Contains(t, body, "Complete the draft before recording it at task-track closeout.")
}

func TestWriteComposeOutput_WithJSON_RendersStructuredOutput(t *testing.T) {
	cmd := &cobra.Command{}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	output := composeOutput{
		Path:             "docs/rfcs/0001-coordinate-writers.md",
		DocumentType:     composeTypeRFC,
		Format:           composeTypeRFC,
		ConventionSource: "fallback",
		ArtifactID:       "rfc_example",
	}

	err := writeComposeOutput(cmd, output, true)

	require.NoError(t, err)
	var actual composeOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &actual))
	assert.Equal(t, output.Path, actual.Path)
	assert.Equal(t, output.DocumentType, actual.DocumentType)
	assert.Equal(t, output.ArtifactID, actual.ArtifactID)
}

func TestConfiguredComposeDirectories_WithADRSource_ReturnsConfiguredDirectory(t *testing.T) {
	cfg := &config.RepoConfig{
		Sources: []config.SourceConfig{{Type: composeTypeADR, Path: "docs/architecture/decisions"}},
	}

	directories := configuredComposeDirectories("C:/repo", cfg, composeTypeADR)

	require.Len(t, directories, 1)
	assert.Equal(t, "docs/architecture/decisions", directories[0])
}

func TestComposeArtifactMatchesType_WithADRSubtype_ReturnsTrue(t *testing.T) {
	artifact := store.ArtifactRow{Kind: config.KindDecision, Subtype: config.SubtypeADR}

	matches := composeArtifactMatchesType(composeTypeADR, artifact, "notes/choice.md")

	assert.True(t, matches)
}

func TestComposeArtifactMatchesType_WithRFCDesignOutsideRFCPath_ReturnsFalse(t *testing.T) {
	artifact := store.ArtifactRow{Kind: config.KindDesign}

	matches := composeArtifactMatchesType(composeTypeRFC, artifact, "docs/design/proposal.md")

	assert.False(t, matches)
}

func TestComposeArtifactMatchesType_WithPRDSubtype_ReturnsTrue(t *testing.T) {
	artifact := store.ArtifactRow{Kind: config.KindRequirements, Subtype: config.SubtypePRD}

	matches := composeArtifactMatchesType(composeTypePRD, artifact, "notes/product.md")

	assert.True(t, matches)
}
