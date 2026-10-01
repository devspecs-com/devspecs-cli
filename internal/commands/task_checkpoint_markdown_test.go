package commands

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const checkpointMultilineText = "First paragraph.\n\nSecond paragraph.\n\n- outer\n  - inner\n\n```text\ncode\n```"
const checkpointIndentedText = "  First paragraph.\n\n  Second paragraph.\n\n  - outer\n    - inner\n\n  ```text\n  code\n  ```\n"

func TestTaskCheckpointResultMarkdown_WhenDescriptionIsMultiline_KeepsBlocksInsideField(t *testing.T) {
	workspace := t.TempDir()
	opts := taskCheckpointOptions{Description: checkpointMultilineText}

	body := renderTaskCheckpointResultAppend(filepath.Join(workspace, "checkpoint.md"), filepath.Join(workspace, "checkpoint.json"), workspace, taskSliceArtifact{}, opts, time.Time{})

	assert.Contains(t, body, "- What changed:\n\n"+checkpointIndentedText+"- Evidence for decision: -\n")
}

func TestTaskCheckpointResultMarkdown_WhenNoteIsMultiline_KeepsNoteAndFallbackInsideFields(t *testing.T) {
	workspace := t.TempDir()
	opts := taskCheckpointOptions{Note: checkpointMultilineText}

	body := renderTaskCheckpointResultAppend(filepath.Join(workspace, "checkpoint.md"), filepath.Join(workspace, "checkpoint.json"), workspace, taskSliceArtifact{}, opts, time.Time{})

	assert.Contains(t, body, "- Note:\n\n"+checkpointIndentedText+"- What changed:\n\n"+checkpointIndentedText+"- Evidence for decision: -\n")
}

func TestTaskCheckpointMarkdown_WhenDescriptionIsMultiline_PreservesSectionAndNestsCompletionSummary(t *testing.T) {
	opts := taskCheckpointOptions{Description: checkpointMultilineText}

	body := renderTaskCheckpoint(taskManifest{TaskID: "markdown-test"}, taskSliceArtifact{ID: "A01"}, opts, time.Time{}, "checkpoint", "checkpoint.json")

	assert.Contains(t, body, "## Description\n"+checkpointMultilineText+"\n\n## Resources\n")
	assert.Contains(t, body, "- What changed:\n\n"+checkpointIndentedText+"- Evidence for decision: -\n")
}

func TestTaskCheckpointMarkdown_WhenNoteIsMultiline_PreservesSectionAndNestsFallbackSummary(t *testing.T) {
	opts := taskCheckpointOptions{Note: checkpointMultilineText}

	body := renderTaskCheckpoint(taskManifest{TaskID: "markdown-test"}, taskSliceArtifact{ID: "A01"}, opts, time.Time{}, "checkpoint", "checkpoint.json")

	assert.Contains(t, body, "## Note\n"+checkpointMultilineText+"\n\n## Structured Evidence\n")
	assert.Contains(t, body, "- What changed:\n\n"+checkpointIndentedText+"- Evidence for decision: -\n")
}

func TestTaskCheckpointResultMarkdown_WhenTextIsSingleLine_PreservesInlineFields(t *testing.T) {
	workspace := t.TempDir()
	opts := taskCheckpointOptions{Description: "A small change.", Note: "A short note."}

	body := renderTaskCheckpointResultAppend(filepath.Join(workspace, "checkpoint.md"), filepath.Join(workspace, "checkpoint.json"), workspace, taskSliceArtifact{}, opts, time.Time{})

	assert.Contains(t, body, "- Note: A short note.\n- What changed: A small change.\n- Evidence for decision: -\n")
}

func TestTaskCheckpointCompletionContract_WhenTextIsSingleLine_PreservesInlineSummary(t *testing.T) {
	var body strings.Builder
	opts := taskCheckpointOptions{Description: "A small change."}

	writeTaskCheckpointCompletionContract(&body, taskSliceArtifact{ID: "A01"}, opts)

	assert.Contains(t, body.String(), "- What changed: A small change.\n- Evidence for decision: -\n")
}
