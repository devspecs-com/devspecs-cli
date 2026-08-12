package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func durabilityTestManifest() taskManifest {
	return taskManifest{
		TaskID: "durability-test",
		Series: "D",
		Artifacts: taskArtifactPaths{
			Index: "D00-index.md",
			Slices: []taskSliceArtifact{
				{ID: "D01", Title: "implemented slice", Stage: "validated", Decision: "promote"},
			},
		},
		Durability: taskDurabilityReview{Required: true},
	}
}

func TestValidateTaskDurabilityCheckpoint_WithNoneDisposition_ReturnsNoError(t *testing.T) {
	repoRoot := t.TempDir()
	workspace := filepath.Join(repoRoot, "devspecs", "tasks", "durability-test")
	manifest := durabilityTestManifest()
	target := taskSeriesCloseoutTarget(manifest)
	opts := taskCheckpointOptions{Stage: "completed", Decision: "complete", DurableRecord: "none"}

	actual, err := validateTaskDurabilityCheckpoint(repoRoot, workspace, manifest, target, opts)

	require.NoError(t, err)
	assert.Equal(t, "none", actual.DurableRecord)
	assert.Empty(t, actual.DurableArtifacts)
}

func TestValidateTaskDurabilityCheckpoint_WithRecordedDraftPlaceholders_ReturnsError(t *testing.T) {
	repoRoot := t.TempDir()
	workspace := filepath.Join(repoRoot, "devspecs", "tasks", "durability-test")
	artifactPath := filepath.Join(repoRoot, "docs", "adr", "0001-choice.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0o755))
	require.NoError(t, os.WriteFile(artifactPath, []byte("# ADR-0001: Choice\n\n## Context\n\n<Describe the problem.>\n"), 0o644))
	manifest := durabilityTestManifest()
	target := taskSeriesCloseoutTarget(manifest)
	opts := taskCheckpointOptions{
		Stage:            "completed",
		Decision:         "complete",
		DurableRecord:    "recorded",
		DurableArtifacts: []string{"docs/adr/0001-choice.md"},
	}

	_, err := validateTaskDurabilityCheckpoint(repoRoot, workspace, manifest, target, opts)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unresolved compose placeholders")
}

func TestValidateTaskDurabilityCheckpoint_WithCompletedRecordedADR_ReturnsNormalizedArtifact(t *testing.T) {
	repoRoot := t.TempDir()
	workspace := filepath.Join(repoRoot, "devspecs", "tasks", "durability-test")
	artifactPath := filepath.Join(repoRoot, "docs", "adr", "0001-choice.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0o755))
	body := "# ADR-0001: Choice\n\n## Status\n\nAccepted\n\n## Context\n\nA durable problem.\n\n## Decision\n\nUse the selected design.\n\n## Consequences\n\nThe tradeoff is explicit.\n"
	require.NoError(t, os.WriteFile(artifactPath, []byte(body), 0o644))
	manifest := durabilityTestManifest()
	target := taskSeriesCloseoutTarget(manifest)
	opts := taskCheckpointOptions{
		Stage:            "completed",
		Decision:         "complete",
		DurableRecord:    "recorded",
		DurableArtifacts: []string{artifactPath},
	}

	actual, err := validateTaskDurabilityCheckpoint(repoRoot, workspace, manifest, target, opts)

	require.NoError(t, err)
	require.Len(t, actual.DurableArtifacts, 1)
	assert.Equal(t, "docs/adr/0001-choice.md", actual.DurableArtifacts[0])
}

func TestValidateTaskDurabilityCheckpoint_WithDeferredDispositionWithoutTarget_ReturnsError(t *testing.T) {
	repoRoot := t.TempDir()
	workspace := filepath.Join(repoRoot, "devspecs", "tasks", "durability-test")
	manifest := durabilityTestManifest()
	target := taskSeriesCloseoutTarget(manifest)
	opts := taskCheckpointOptions{Stage: "completed", Decision: "complete", DurableRecord: "deferred"}

	_, err := validateTaskDurabilityCheckpoint(repoRoot, workspace, manifest, target, opts)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires --next-target")
}

func TestTaskNextSlice_WithLegacyCompletedTrack_ReturnsAllTargetsTerminal(t *testing.T) {
	manifest := durabilityTestManifest()
	manifest.Durability = taskDurabilityReview{}

	_, err := taskNextSlice(manifest)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "all task targets are terminal")
}
