package commands

import (
	"errors"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	checkpointJSONRequired      = `{"schema":"devspecs.task.checkpoint_input","schema_version":1,"task_id":"task.one","target":"A01","stage":"validated","decision":"promote","expected_heads":[]}`
	checkpointJSONReceiptFields = `"dispatch_id":"dispatch_0123456789ABCDEFGHJKMNPQRS","receipt_id":"receipt_0123456789ABCDEFGHJKMNPQRS",` +
		`"request_sha256":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",` +
		`"wire_sha256":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`
	checkpointJSONResultCommit = `"result_commit":"cccccccccccccccccccccccccccccccccccccccc"`
)

func checkpointJSONWith(fields string) string {
	return strings.TrimSuffix(checkpointJSONRequired, "}") + "," + fields + "}"
}

func TestParseTaskCheckpointJSON_WhenMinimal_PreservesExplicitEmptyHeads(t *testing.T) {
	input := strings.NewReader(checkpointJSONRequired)

	result, err := parseTaskCheckpointJSON(input)

	require.NoError(t, err)
	assert.Equal(t, "task.one", result.TaskID)
	assert.Equal(t, "A01", result.Target)
	assert.Equal(t, "validated", result.Stage)
	assert.Equal(t, "promote", result.Decision)
	assert.NotNil(t, result.ExpectedHeads)
	assert.Empty(t, result.ExpectedHeads)
	assert.Nil(t, result.Supersedes)
	assert.Nil(t, result.ReceiptEvidence)
}

func TestParseTaskCheckpointJSON_WhenEvidenceIsPresent_PreservesEachField(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"goal":"G05","description":"Line one\nLine two","note":"Evidence",
		"resources":["file:///no/such/path"],"files_read":["pkg/checkpoint.go"],"files_edited":["pkg/parser.go"],
		"tests_read":["pkg/parser_test.go"],"tests_run":["go test ./pkg && exit 73"],
		"missed_files":["pkg/missed.go"],"noise_files":["pkg/noise.go"],"tasks":["Add the parser"],
		"learnings":["Require explicit heads"],"next_target":"A02","next_decision":"continue",
		"durable_record":"recorded","durable_artifacts":["docs/Design record.md"]`))

	result, err := parseTaskCheckpointJSON(input)

	require.NoError(t, err)
	assert.Equal(t, "G05", result.Goal)
	assert.Equal(t, "Line one\nLine two", result.Description)
	assert.Equal(t, "Evidence", result.Note)
	assertTaskCheckpointJSONItem(t, "file:///no/such/path", result.Resources)
	assertTaskCheckpointJSONItem(t, "pkg/checkpoint.go", result.FilesRead)
	assertTaskCheckpointJSONItem(t, "pkg/parser.go", result.FilesEdited)
	assertTaskCheckpointJSONItem(t, "pkg/parser_test.go", result.TestsRead)
	assertTaskCheckpointJSONItem(t, "go test ./pkg && exit 73", result.TestsRun)
	assertTaskCheckpointJSONItem(t, "pkg/missed.go", result.MissedFiles)
	assertTaskCheckpointJSONItem(t, "pkg/noise.go", result.NoiseFiles)
	assertTaskCheckpointJSONItem(t, "Add the parser", result.Tasks)
	assertTaskCheckpointJSONItem(t, "Require explicit heads", result.Learnings)
	assert.Equal(t, "A02", result.NextTarget)
	assert.Equal(t, "continue", result.NextDecision)
	assert.Equal(t, "recorded", result.DurableRecord)
	assertTaskCheckpointJSONItem(t, "docs/Design record.md", result.DurableArtifacts)
}

func assertTaskCheckpointJSONItem(t *testing.T, expected string, values []string) {
	t.Helper()
	require.Len(t, values, 1)
	assert.Equal(t, expected, values[0])
}

func TestParseTaskCheckpointJSON_WhenConflictHeadsAreNamed_PreservesIDsAndOrder(t *testing.T) {
	input := checkpointJSONWith(`"supersedes":["cp_agent_a","20261110-120000-validated"]`)
	input = strings.Replace(input, `"expected_heads":[]`, `"expected_heads":["20261110-120000-validated","cp_agent_a"]`, 1)

	result, err := parseTaskCheckpointJSON(strings.NewReader(input))

	require.NoError(t, err)
	require.Len(t, result.ExpectedHeads, 2)
	assert.Equal(t, "20261110-120000-validated", result.ExpectedHeads[0])
	assert.Equal(t, "cp_agent_a", result.ExpectedHeads[1])
	require.Len(t, result.Supersedes, 2)
	assert.Equal(t, "cp_agent_a", result.Supersedes[0])
	assert.Equal(t, "20261110-120000-validated", result.Supersedes[1])
}

func TestParseTaskCheckpointJSON_WhenReceiptHasResultCommit_ProducesOpaqueResource(t *testing.T) {
	fields := checkpointJSONReceiptFields + "," + checkpointJSONResultCommit
	input := strings.NewReader(checkpointJSONWith(`"receipt_evidence":{` + fields + `}`))

	result, err := parseTaskCheckpointJSON(input)

	require.NoError(t, err)
	require.NotNil(t, result.ReceiptEvidence)
	assert.Equal(t, "devspecs.dispatch.receipt_evidence:{"+fields+"}", result.ReceiptEvidence.resource())
}

func TestParseTaskCheckpointJSON_WhenReceiptOmitsResultCommit_OmitsItFromResource(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"receipt_evidence":{` + checkpointJSONReceiptFields + `}`))

	result, err := parseTaskCheckpointJSON(input)

	require.NoError(t, err)
	require.NotNil(t, result.ReceiptEvidence)
	assert.Equal(t, "", result.ReceiptEvidence.ResultCommit)
	assert.NotContains(t, result.ReceiptEvidence.resource(), "result_commit")
}

func TestParseTaskCheckpointJSON_WhenInputIsExactlyByteLimit_AcceptsTrailingWhitespace(t *testing.T) {
	input := checkpointJSONRequired + strings.Repeat(" ", taskCheckpointJSONMaxBytes-len(checkpointJSONRequired))

	_, err := parseTaskCheckpointJSON(strings.NewReader(input))

	require.NoError(t, err)
}

func TestParseTaskCheckpointJSON_WhenInputExceedsByteLimit_RejectsIt(t *testing.T) {
	input := checkpointJSONRequired + strings.Repeat(" ", taskCheckpointJSONMaxBytes-len(checkpointJSONRequired)+1)

	_, err := parseTaskCheckpointJSON(strings.NewReader(input))

	require.ErrorContains(t, err, "exceeds 262144 bytes")
}

func TestParseTaskCheckpointJSON_WhenReaderFails_ReturnsReaderError(t *testing.T) {
	readErr := errors.New("stdin failed")
	input := iotest.ErrReader(readErr)

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorIs(t, err, readErr)
}

func TestParseTaskCheckpointJSON_WhenInputContainsInvalidUTF8_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"note":"` + string([]byte{0xff}) + `"`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "not valid UTF-8")
}

func TestParseTaskCheckpointJSON_WhenUnicodeEscapesAreValid_PreservesDecodedText(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"note":"\ud83d\ude00 \\ud800 \ufffd"`))

	result, err := parseTaskCheckpointJSON(input)

	require.NoError(t, err)
	assert.Equal(t, "\U0001f600 \\ud800 \ufffd", result.Note)
}

func TestParseTaskCheckpointJSON_WhenSurrogateIsUnpaired_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"note":"\ud800"`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "invalid Unicode escape")
}

func TestParseTaskCheckpointJSON_WhenJSONIsTruncated_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.TrimSuffix(checkpointJSONRequired, "}"))

	_, err := parseTaskCheckpointJSON(input)

	require.Error(t, err)
}

func TestParseTaskCheckpointJSON_WhenRootIsArray_RejectsIt(t *testing.T) {
	input := strings.NewReader(`[` + checkpointJSONRequired + `]`)

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "expected an object")
}

func TestParseTaskCheckpointJSON_WhenSecondValueFollows_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONRequired + ` {}`)

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "trailing data")
}

func TestParseTaskCheckpointJSON_WhenMemberRepeatsViaEscape_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"ta\u0072get":"A02"`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, `duplicate member "target"`)
}

func TestParseTaskCheckpointJSON_WhenMemberHasWrongCase_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, `"task_id"`, `"Task_ID"`, 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "unknown object member")
}

func TestParseTaskCheckpointJSON_WhenMemberIsUnknown_DoesNotEchoItsName(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"sensitive_input_name":"unused"`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "unknown object member")
	assert.NotContains(t, err.Error(), "sensitive_input_name")
}

func TestParseTaskCheckpointJSON_WhenSchemaIsProviderReceipt_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, "devspecs.task.checkpoint_input", "devspecs.dispatch.receipt", 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "schema: must be")
}

func TestParseTaskCheckpointJSON_WhenVersionIsUnsupported_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, `"schema_version":1`, `"schema_version":3`, 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "schema_version: must be the integer 1")
}

func TestParseTaskCheckpointJSON_WhenEventFieldIsPresent_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"event_kind":"checkpoint"`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "unknown object member")
}

func TestParseTaskCheckpointJSON_WhenCaptureFieldIsPresent_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"run_logs":["../some.log"]`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "unknown object member")
}

func TestParseTaskCheckpointJSON_WhenTaskIDIsDot_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, "task.one", ".", 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "task_id: must be an ID")
}

func TestParseTaskCheckpointJSON_WhenTargetIsDotDot_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, "A01", "..", 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "target: must be an ID")
}

func TestParseTaskCheckpointJSON_WhenTargetLooksLikeTitle_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, "A01", "Add parser", 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "target: task id contains unsupported character")
}

func TestParseTaskCheckpointJSON_WhenIDUsesUnicodeLettersAndDigits_PreservesIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, "task.one", `t\u00e2che.\u0661`, 1))

	result, err := parseTaskCheckpointJSON(input)

	require.NoError(t, err)
	assert.Equal(t, "t\u00e2che.\u0661", result.TaskID)
}

func TestParseTaskCheckpointJSON_WhenStageIsMissing_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, `"stage":"validated",`, "", 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, `missing required member "stage"`)
}

func TestParseTaskCheckpointJSON_WhenStageIsInvalid_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, "validated", "Validated", 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "stage: must be one of")
}

func TestParseTaskCheckpointJSON_WhenDecisionIsMissing_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, `"decision":"promote",`, "", 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, `missing required member "decision"`)
}

func TestParseTaskCheckpointJSON_WhenDecisionIsInvalid_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, "promote", "approve", 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "decision: must be one of")
}

func TestParseTaskCheckpointJSON_WhenHeadsAreMissing_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, `,"expected_heads":[]`, "", 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, `missing required member "expected_heads"`)
}

func TestParseTaskCheckpointJSON_WhenHeadsAreNull_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, "[]", "null", 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "expected_heads: expected an array")
}

func TestParseTaskCheckpointJSON_WhenHeadIsNull_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, "[]", "[null]", 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "expected_heads: item 0: expected a string")
}

func TestParseTaskCheckpointJSON_WhenHeadRepeats_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, "[]", `["cp_agent_a","cp_agent_a"]`, 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "expected_heads: item 1: duplicate checkpoint ID")
}

func TestParseTaskCheckpointJSON_WhenHeadRepeatsWithDifferentCase_RejectsIt(t *testing.T) {
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, "[]", `["cp_agent_a","CP_agent_a"]`, 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "expected_heads: item 1: duplicate checkpoint ID")
}

func TestParseTaskCheckpointJSON_WhenHeadExceedsIDLimit_RejectsIt(t *testing.T) {
	heads := `["` + strings.Repeat("x", taskCheckpointJSONMaxID+1) + `"]`
	input := strings.NewReader(strings.Replace(checkpointJSONRequired, "[]", heads, 1))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "expected_heads: item 0: must contain 1 to 128 characters")
}

func TestParseTaskCheckpointJSON_WhenSupersedesContainsPath_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"supersedes":["../cp_agent_a"]`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "supersedes: item 0: task id contains unsupported character")
}

func TestParseTaskCheckpointJSON_WhenListExceedsItemLimit_RejectsIt(t *testing.T) {
	items := strings.Repeat(`"resource",`, taskCheckpointJSONMaxItems) + `"last"`
	input := strings.NewReader(checkpointJSONWith(`"resources":[` + items + `]`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "resources: must have at most 256 items")
}

func TestParseTaskCheckpointJSON_WhenProseExceedsCharacterLimit_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"description":"` + strings.Repeat("x", taskCheckpointJSONMaxProse+1) + `"`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "description: must contain 0 to 16384 characters")
}

func TestParseTaskCheckpointJSON_WhenReadPathEscapesViaDotDot_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"files_read":["pkg/../../secret"]`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "files_read: item 0: must be a portable repository-relative path")
}

func TestParseTaskCheckpointJSON_WhenEditedPathIsDriveRelative_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"files_edited":["C:secret.go"]`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "files_edited: item 0: must be a portable repository-relative path")
}

func TestParseTaskCheckpointJSON_WhenTestPathIsUNC_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"tests_read":["\\\\host\\share\\test.go"]`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "tests_read: item 0: must be a portable repository-relative path")
}

func TestParseTaskCheckpointJSON_WhenMissedPathUsesBackslashTraversal_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"missed_files":["pkg\\..\\secret.go"]`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "missed_files: item 0: must be a portable repository-relative path")
}

func TestParseTaskCheckpointJSON_WhenNoisePathUsesWindowsDotAlias_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"noise_files":[".. /secret.go"]`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "noise_files: item 0: must be a portable repository-relative path")
}

func TestParseTaskCheckpointJSON_WhenDurableArtifactIsAbsolute_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"durable_artifacts":["/outside/adr.md"]`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "durable_artifacts: item 0: must be a portable repository-relative path")
}

func TestParseTaskCheckpointJSON_WhenReceiptIsNull_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"receipt_evidence":null`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "receipt_evidence: expected an object")
}

func TestParseTaskCheckpointJSON_WhenReceiptLacksWireDigest_RejectsIt(t *testing.T) {
	fields, _, _ := strings.Cut(checkpointJSONReceiptFields, `,"wire_sha256"`)
	input := strings.NewReader(checkpointJSONWith(`"receipt_evidence":{` + fields + `}`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, `receipt_evidence: missing required member "wire_sha256"`)
}

func TestParseTaskCheckpointJSON_WhenReceiptMemberRepeats_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"receipt_evidence":{` + checkpointJSONReceiptFields + `,"wire_sha256":"duplicate"}`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, `receipt_evidence: duplicate member "wire_sha256"`)
}

func TestParseTaskCheckpointJSON_WhenReceiptMemberHasWrongCase_RejectsIt(t *testing.T) {
	fields := strings.Replace(checkpointJSONReceiptFields, "receipt_id", "Receipt_ID", 1)
	input := strings.NewReader(checkpointJSONWith(`"receipt_evidence":{` + fields + `}`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "receipt_evidence: unknown object member")
}

func TestParseTaskCheckpointJSON_WhenReceiptContainsRawProviderData_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"receipt_evidence":{` + checkpointJSONReceiptFields + `,"provider":{"run_id":"raw"}}`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "receipt_evidence: unknown object member")
}

func TestParseTaskCheckpointJSON_WhenReceiptDigestIsInvalid_RejectsIt(t *testing.T) {
	fields := strings.Replace(checkpointJSONReceiptFields, "sha256:", "", 1)
	input := strings.NewReader(checkpointJSONWith(`"receipt_evidence":{` + fields + `}`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "receipt_evidence: request_sha256: invalid format")
}

func TestParseTaskCheckpointJSON_WhenReceiptResultCommitIsInvalid_RejectsIt(t *testing.T) {
	input := strings.NewReader(checkpointJSONWith(`"receipt_evidence":{` + checkpointJSONReceiptFields + `,"result_commit":"HEAD"}`))

	_, err := parseTaskCheckpointJSON(input)

	require.ErrorContains(t, err, "receipt_evidence: result_commit: invalid format")
}
