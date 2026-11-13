package snapshot

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordThreads_WithOrderedDependencies_AcceptsDistinctAssignedTargets(t *testing.T) {
	data := []byte(`schema_version: 1
revision: 1
owner: {kind: task, task_id: task-one}
threads:
  - {key: build, targets: [{target: A01}]}
  - {key: review, targets: [{target: A02}], after: [build]}
`)

	err := readTaskThreads(data, "task-one", map[string]bool{"a01": true, "a02": true})

	require.NoError(t, err)
}

func TestRecordThreads_WithDependencyCycle_RejectsUnrunnableSchedule(t *testing.T) {
	data := []byte(`schema_version: 1
revision: 1
owner: {kind: task, task_id: task-one}
threads:
  - {key: build, targets: [{target: A01}], after: [review]}
  - {key: review, targets: [{target: A02}], after: [build]}
`)

	err := readTaskThreads(data, "task-one", map[string]bool{"a01": true, "a02": true})

	require.ErrorIs(t, err, ErrInvalid)
}

func TestRecordThreads_WithMissingDependency_RejectsUnknownLane(t *testing.T) {
	data := []byte(`schema_version: 1
revision: 1
owner: {kind: task, task_id: task-one}
threads:
  - {key: build, targets: [{target: A01}], after: [missing]}
`)

	err := readTaskThreads(data, "task-one", map[string]bool{"a01": true})

	require.ErrorIs(t, err, ErrInvalid)
}

func TestRecordThreads_WithTargetAssignedTwice_RejectsCompetingOwners(t *testing.T) {
	data := []byte(`schema_version: 1
revision: 1
owner: {kind: task, task_id: task-one}
threads:
  - {key: build, targets: [{target: A01}]}
  - {key: review, targets: [{target: a01}]}
`)

	err := readTaskThreads(data, "task-one", map[string]bool{"a01": true})

	require.ErrorIs(t, err, ErrInvalid)
}

func TestRecordThreads_WithCaseAliasedLaneKeys_RejectsDuplicateIdentity(t *testing.T) {
	data := []byte(`schema_version: 1
revision: 1
owner: {kind: task, task_id: task-one}
threads:
  - {key: build, targets: [{target: A01}]}
  - {key: BUILD, targets: [{target: A02}]}
`)

	err := readTaskThreads(data, "task-one", map[string]bool{"a01": true, "a02": true})

	require.ErrorIs(t, err, ErrInvalid)
}

func TestRecordThreads_WithForeignTaskOwner_RejectsUnrelatedDefinition(t *testing.T) {
	data := []byte(`schema_version: 1
revision: 1
owner: {kind: task, task_id: other-task}
threads:
  - {key: build, targets: [{target: A01}]}
`)

	err := readTaskThreads(data, "task-one", map[string]bool{"a01": true})

	require.ErrorIs(t, err, ErrUnsupported)
}

func TestRecordYAML_WithInvalidUTF8_RejectsReplacementDecoding(t *testing.T) {
	data := []byte{'k', ':', ' ', 0xff, '\n'}
	var decoded map[string]any

	err := decodeYAML(data, &decoded, true)

	require.ErrorIs(t, err, ErrInvalid)
	assert.Nil(t, decoded)
}

func TestRecordYAML_WithMalformedSyntax_RejectsPartialDocument(t *testing.T) {
	data := []byte("owner: [unterminated\n")
	var decoded map[string]any

	err := decodeYAML(data, &decoded, true)

	require.ErrorIs(t, err, ErrInvalid)
	assert.Nil(t, decoded)
}

func TestRecordYAML_WithSecondDocument_RejectsHiddenTrailingConfiguration(t *testing.T) {
	data := []byte("revision: 1\n---\nrevision: 2\n")
	var decoded threadProjection

	err := decodeYAML(data, &decoded, true)

	require.ErrorIs(t, err, ErrInvalid)
	assert.Zero(t, decoded.Revision)
}

func TestRecordYAML_WithUnknownProjectionField_RejectsUnsupportedSchema(t *testing.T) {
	data := []byte("schema_version: 1\nunreviewed_authority: true\n")
	var decoded threadProjection

	err := decodeYAML(data, &decoded, true)

	require.ErrorIs(t, err, ErrInvalid)
}

func TestRecordYAML_WithDuplicateKey_RejectsAmbiguousRevision(t *testing.T) {
	data := []byte("revision: 1\nrevision: 2\n")
	var decoded threadProjection

	err := decodeYAML(data, &decoded, true)

	require.ErrorIs(t, err, ErrInvalid)
	assert.Zero(t, decoded.Revision)
}

func TestRecordYAML_WithAnchoredValue_RejectsReusableAliasAuthority(t *testing.T) {
	data := []byte("owner: &shared {kind: task, task_id: task-one}\n")
	var decoded threadProjection

	err := decodeYAML(data, &decoded, true)

	require.ErrorIs(t, err, ErrInvalid)
}

func TestRecordYAML_WithExcessiveNesting_RejectsBeforeProjectionDecode(t *testing.T) {
	data := []byte(strings.Repeat("[", MaxJSONDepth+1) + "0" + strings.Repeat("]", MaxJSONDepth+1))
	var decoded any

	err := decodeYAML(data, &decoded, false)

	require.ErrorIs(t, err, ErrInvalid)
	assert.Nil(t, decoded)
}

func TestRecordCheckpointGraph_WithMatchingSupersededTarget_AcceptsPredecessor(t *testing.T) {
	events := map[string]checkpointProjection{
		"cp-one": {Target: "a01"},
		"cp-two": {Target: "A01", Supersedes: []string{"CP-ONE"}},
	}

	err := validateCheckpointGraph(events)

	require.NoError(t, err)
}

func TestRecordCheckpointGraph_WithDifferentSupersededTarget_RejectsCrossSliceHistory(t *testing.T) {
	events := map[string]checkpointProjection{
		"cp-one": {Target: "a01"},
		"cp-two": {Target: "a02", Supersedes: []string{"cp-one"}},
	}

	err := validateCheckpointGraph(events)

	require.ErrorIs(t, err, ErrInvalid)
}

func TestRecordCheckpointTarget_WithIndexPath_ResolvesLogicalSeriesTarget(t *testing.T) {
	task := taskProjection{Series: "Q"}
	task.Artifacts.Index = "Q00-index.md"

	target := taskCheckpointTarget(task, " Q00-index.md ")

	assert.Equal(t, "q00", target)
}

func TestRecordCheckpointTarget_WithEmptyTarget_DoesNotInventSeriesTarget(t *testing.T) {
	task := taskProjection{Series: "Q"}

	target := taskCheckpointTarget(task, " \t")

	assert.Empty(t, target)
}

func TestRecordCheckpointTarget_WithTraversalPath_RejectsNonIdentity(t *testing.T) {
	task := taskProjection{Series: "Q"}

	target := taskCheckpointTarget(task, "../Q01")

	assert.Empty(t, target)
}

func TestRecordDocument_WithUnsupportedFrontmatterVersion_RejectsInsteadOfReinterpreting(t *testing.T) {
	data := []byte("---\nschema_version: 2\n---\n# Decision\n")

	_, err := documentIdentity(data)

	require.ErrorIs(t, err, ErrUnsupported)
}

func TestRecordDocument_WithNumericFrontmatterID_RejectsUnstableIdentity(t *testing.T) {
	data := []byte("---\nid: 123\n---\n# Decision\n")

	_, err := documentIdentity(data)

	require.ErrorIs(t, err, ErrInvalid)
}

func TestRecordDocument_WithDuplicateFrontmatterKey_RejectsAmbiguousIdentity(t *testing.T) {
	data := []byte("---\nid: first\nid: second\n---\n# Decision\n")

	_, err := documentIdentity(data)

	require.ErrorIs(t, err, ErrInvalid)
}

func TestRecordDocument_WithInvalidUTF8_RejectsBeforeMarkdownParsing(t *testing.T) {
	data := []byte{'#', ' ', 0xff, '\n'}

	_, err := documentIdentity(data)

	require.ErrorIs(t, err, ErrInvalid)
}

func TestRecordDocument_WithUnclosedFrontmatter_RejectsTruncatedMetadata(t *testing.T) {
	data := []byte("---\nid: decision-one")

	_, err := documentIdentity(data)

	require.ErrorIs(t, err, ErrInvalid)
}

func TestRecordDocument_WithAutolinkHeading_AcceptsGoldmarkLabelText(t *testing.T) {
	data := []byte("# <https://example.invalid/decision>\n")

	id, err := documentIdentity(data)

	require.NoError(t, err)
	assert.Empty(t, id)
}

func TestRecordDocument_WithInlineHTMLHeading_PreservesRawTextPresence(t *testing.T) {
	data := []byte("# <span></span>\n")

	id, err := documentIdentity(data)

	require.NoError(t, err)
	assert.Empty(t, id)
}
