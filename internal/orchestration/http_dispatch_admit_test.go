package orchestration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdmitHTTPDispatchReceipt_WithExactLocalCommit_AdmitsUnverifiedExecutionEvidence(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	checkout := newPortableFixtureCheckout(t)
	result := commitHTTPDispatchAdmissionFile(t, checkout, "src/fixture.go")
	f.pin(checkout, portableFixtureCommit, result)
	f.object("repository")["result_ref"] = "refs/heads/nonexistent-result-label"
	f.object("evidence")["files_edited"] = []string{"src/fixture.go"}

	admitted, err := f.admit(t, nil)

	require.NoError(t, err)
	require.NotNil(t, admitted.ResultCommit)
	assert.Equal(t, result, *admitted.ResultCommit)
	assert.Equal(t, testHTTPDispatchID, admitted.DispatchID)
	assert.Equal(t, httpDispatchDigest(f.receiptBytes(t)), admitted.WireSHA256)
	assert.Equal(t, "succeeded", admitted.State)
	assert.Equal(t, "unavailable", admitted.VerificationState)
	assert.Equal(t, "suite", admitted.RequiredVerificationStrength)
	assert.True(t, admitted.ProviderPathsComplete)
	assert.Equal(t, "omitted", admitted.PathClaimsDisposition)
	require.NotNil(t, admitted.ReportArtifact)
	assert.True(t, admitted.ReportArtifact.BytesVerified)
	assert.Equal(t, int64(15), admitted.ReportArtifact.Size)
	assert.Equal(t, httpDispatchDigest([]byte("fixture report\n")), admitted.ReportArtifact.SHA256)
}

func TestAdmitHTTPDispatchReceipt_WithUnreportedLifecycleCommit_RejectsResult(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	checkout := newPortableFixtureCheckout(t)
	result := commitHTTPDispatchAdmissionFile(t, checkout, "devspecs/tasks/fixture.md")
	f.pin(checkout, portableFixtureCommit, result)

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchTask)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithPassingVerification_RetainsClaimAndDigest(t *testing.T) {
	// Arrange
	f := newHTTPDispatchAdmissionFixture(t)
	f.pin(newPortableFixtureCheckout(t), portableFixtureCommit, portableFixtureCommit)
	f.receipt["verification"] = map[string]any{
		"state": "passed", "strength": "suite", "test_output": f.object("report")["artifact"],
	}

	// Act
	admitted, err := f.admit(t, nil)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "passed", admitted.VerificationState)
	assert.Equal(t, "suite", admitted.VerificationStrength)
	require.NotNil(t, admitted.TestOutput)
	assert.True(t, admitted.TestOutput.BytesVerified)
	assert.Equal(t, httpDispatchDigest([]byte("fixture report\n")), admitted.TestOutput.SHA256)
}

func TestAdmitHTTPDispatchReceipt_WithFailedVerification_PreservesSeparateExecutionOutcome(t *testing.T) {
	// Arrange
	f := newHTTPDispatchAdmissionFixture(t)
	f.pin(newPortableFixtureCheckout(t), portableFixtureCommit, portableFixtureCommit)
	f.receipt["verification"] = map[string]any{
		"state": "failed", "strength": "suite", "test_output": f.object("report")["artifact"],
	}

	// Act
	admitted, err := f.admit(t, nil)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "succeeded", admitted.State)
	assert.Equal(t, "failed", admitted.VerificationState)
	assert.Equal(t, "suite", admitted.RequiredVerificationStrength)
}

func TestAdmitHTTPDispatchReceipt_WithUnreportedHiddenLifecycleCommit_RejectsResult(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	checkout := newPortableFixtureCheckout(t)
	result := commitHTTPDispatchAdmissionFile(t, checkout, ".devspecs/tasks/fixture.md")
	f.pin(checkout, portableFixtureCommit, result)

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchTask)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithUnreportedCustomLifecycleCommit_RejectsResult(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	checkout := newPortableFixtureCheckout(t)
	result := commitHTTPDispatchAdmissionFile(t, checkout, "local/Lifecycle/fixture.md")
	f.pin(checkout, portableFixtureCommit, result)

	admitted, err := f.admit(t, []string{"local/lifecycle"})

	require.ErrorIs(t, err, errHTTPDispatchTask)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithMissingEditFromCompletePaths_RejectsEvidence(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	checkout := newPortableFixtureCheckout(t)
	result := commitHTTPDispatchAdmissionFile(t, checkout, "src/fixture.go")
	f.pin(checkout, portableFixtureCommit, result)

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchPaths)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithIncompletePaths_DoesNotInventCompleteAudit(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	checkout := newPortableFixtureCheckout(t)
	result := commitHTTPDispatchAdmissionFile(t, checkout, "src/fixture.go")
	f.pin(checkout, portableFixtureCommit, result)
	f.object("evidence")["paths_complete"] = false
	f.object("evidence")["paths_manifest"] = f.object("report")["artifact"]

	admitted, err := f.admit(t, nil)

	require.NoError(t, err)
	assert.False(t, admitted.ProviderPathsComplete)
	require.NotNil(t, admitted.PathsManifest)
	assert.True(t, admitted.PathsManifest.BytesVerified)
}

func TestAdmitHTTPDispatchReceipt_WithMissingResultAndExistingResultLabel_RejectsResult(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	checkout := newPortableFixtureCheckout(t)
	_, err := runGitBytes(t.Context(), ExecRunner{}, checkout, "update-ref", "refs/heads/fixture-result", portableFixtureCommit)
	require.NoError(t, err)
	f.pin(checkout, portableFixtureCommit, strings.Repeat("e", 40))
	f.object("repository")["result_ref"] = "refs/heads/fixture-result"

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchResult)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithResultOutsidePinnedAncestry_RejectsResult(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	checkout := newPortableFixtureCheckout(t)
	source := commitHTTPDispatchAdmissionFile(t, checkout, "src/pin.go")
	f.pin(checkout, source, portableFixtureCommit)

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchResult)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithInvalidResultRef_RejectsLabel(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.checkout = t.TempDir()
	f.object("repository")["result_ref"] = "refs/heads/result@{yesterday}"

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchResponse)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithWrongRun_RejectsCorrelation(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.object("provider")["run_id"] = "run_fixture_002"

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchCorrelation)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithWrongReceiptDigest_RejectsCorrelation(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.receipt["request_sha256"] = httpDispatchDigest(nil)

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchCorrelation)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithChangedFrozenBytes_RejectsCorrelation(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.frozen = append(f.frozen, '\n')

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchCorrelation)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithWrongSourcePin_RejectsCorrelation(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.object("repository")["source_commit"] = strings.Repeat("d", 40)

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchCorrelation)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithStaleTerminalObservation_RejectsCorrelation(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.status.ObservedAt = f.status.ObservedAt.Add(-time.Second)

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchCorrelation)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithDifferentTerminalOutcome_RejectsCorrelation(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.status.State = "failed"

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchCorrelation)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithTooManyAttempts_RejectsCorrelation(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.object("execution")["attempts"] = 2

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchCorrelation)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithOverBudgetSuccess_RejectsCorrelation(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.endAt(f.handle.AcceptedAt.Add(601 * time.Second))

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchCorrelation)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithLateTimeout_PreservesFailureEvidence(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.fail("timeout", "private cleanup transcript: "+testHTTPDispatchToken)
	f.endAt(f.handle.AcceptedAt.Add(601 * time.Second))

	admitted, err := f.admit(t, nil)

	require.NoError(t, err)
	assert.Equal(t, "failed", admitted.State)
	assert.Nil(t, admitted.ResultCommit)
	require.NotNil(t, admitted.Failure)
	assert.Equal(t, "timeout", admitted.Failure.Class)
	assert.Equal(t, "omitted", admitted.Failure.DetailDisposition)
	assert.NotContains(t, marshalHTTPDispatchAdmission(t, admitted), testHTTPDispatchToken)
}

func TestAdmitHTTPDispatchReceipt_WithFreeTextFailureClass_ProjectsOther(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.fail(testHTTPDispatchToken, "arbitrary failure detail")

	admitted, err := f.admit(t, nil)

	require.NoError(t, err)
	require.NotNil(t, admitted.Failure)
	assert.Equal(t, "other", admitted.Failure.Class)
	assert.NotContains(t, marshalHTTPDispatchAdmission(t, admitted), "arbitrary failure detail")
}

func TestAdmitHTTPDispatchReceipt_WithBadInlineArtifactDigest_RejectsEvidence(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.object("report")["artifact"].(map[string]any)["sha256"] = httpDispatchDigest(nil)

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchResponse)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithHTTPSArtifact_ProjectsUnverifiedMetadataOnly(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.fail("provider_error", "run failed")
	f.object("report")["artifact"] = map[string]any{
		"kind": "https", "artifact_id": "fixture-report", "media_type": "text/plain",
		"size_bytes": 4, "sha256": httpDispatchDigest([]byte("test")),
		"url": "https://unverified.example/report", "credential_ref": testHTTPDispatchToken,
	}

	admitted, err := f.admit(t, nil)

	require.NoError(t, err)
	require.NotNil(t, admitted.ReportArtifact)
	assert.False(t, admitted.ReportArtifact.BytesVerified)
	assert.Equal(t, int64(4), admitted.ReportArtifact.Size)
	encoded := marshalHTTPDispatchAdmission(t, admitted)
	assert.NotContains(t, encoded, "unverified.example")
	assert.NotContains(t, encoded, testHTTPDispatchToken)
}

func TestAdmitHTTPDispatchReceipt_WithGitInternalReadPath_RejectsEvidence(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.object("evidence")["files_read"] = []string{"src/.GiT/config"}

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchPaths)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithOversizedWire_RejectsBeforeDecoding(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	raw := make([]byte, httpDispatchReceiptBody+1)

	admitted, err := admitHTTPDispatchReceipt(t.Context(), nil, "", f.frozen, &f.handle, &f.status, raw, nil)

	require.ErrorIs(t, err, errHTTPDispatchLimit)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithTerminalZeroAndStatusUsage_PersistsOnlyTerminalUsage(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.fail("execution_failed", "run failed")
	f.frozen = []byte(strings.Replace(string(f.frozen), `"max_attempts": 1`, `"max_attempts": 2`, 1))
	f.bindDigest()
	f.object("execution")["attempts"] = 2
	f.status.Usage = json.RawMessage(`{"input_tokens":80,"cached_input_tokens":10,"output_tokens":40,"wall_time_ms":5000}`)
	f.receipt["usage"] = map[string]any{"output_tokens": 0, "cost": map[string]any{"amount": "0.000000", "currency": "EUR"}}

	admitted, err := f.admit(t, nil)

	require.NoError(t, err)
	require.NotNil(t, admitted.Usage)
	require.NotNil(t, admitted.Usage.OutputTokens)
	assert.Zero(t, *admitted.Usage.OutputTokens)
	assert.Nil(t, admitted.Usage.InputTokens)
	assert.Nil(t, admitted.Usage.CachedInputTokens)
	assert.Nil(t, admitted.Usage.WallTimeMS)
	require.NotNil(t, admitted.Usage.Cost)
	assert.Equal(t, "0.000000", admitted.Usage.Cost.Amount)
	assert.Equal(t, "EUR", admitted.Usage.Cost.Currency)
	assert.Contains(t, marshalHTTPDispatchAdmission(t, admitted), `"output_tokens":0`)
}

func TestAdmitHTTPDispatchReceipt_WithoutTerminalUsage_DoesNotUseStatusAsFallback(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.fail("provider_error", "run failed")
	f.status.Usage = json.RawMessage(`{"input_tokens":30}`)
	delete(f.receipt, "usage")

	admitted, err := f.admit(t, nil)

	require.NoError(t, err)
	assert.Nil(t, admitted.Usage)
	assert.Empty(t, admitted.EffectiveModel)
}

func TestAdmitHTTPDispatchReceipt_WithStatusCachedTokensOverTotal_RejectsObservation(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.status.Usage = json.RawMessage(`{"input_tokens":0,"cached_input_tokens":1}`)

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchResponse)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithReceiptCachedTokensOverTotal_RejectsObservation(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.receipt["usage"] = map[string]any{"input_tokens": 0, "cached_input_tokens": 1}

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchResponse)
	assert.Nil(t, admitted)
}

func TestAdmitHTTPDispatchReceipt_WithCachedTokensAndUnknownTotal_PreservesKnownSubset(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.fail("execution_failed", "run failed")
	f.receipt["usage"] = map[string]any{"cached_input_tokens": 8}

	admitted, err := f.admit(t, nil)

	require.NoError(t, err)
	require.NotNil(t, admitted.Usage)
	require.NotNil(t, admitted.Usage.CachedInputTokens)
	assert.Equal(t, int64(8), *admitted.Usage.CachedInputTokens)
	assert.Nil(t, admitted.Usage.InputTokens)
}

func TestAdmitHTTPDispatchReceipt_WithReportedModelSubstitution_ProjectsModelsWithoutReason(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.fail("provider_error", "run failed")
	f.receipt["usage"] = map[string]any{"model": "Example/Model:Stable", "effort": "private agent transcript"}
	f.receipt["model_substitution"] = map[string]any{
		"requested_model": "Example/Model:Latest", "effective_model": "Example/Model:Stable", "reason": testHTTPDispatchToken,
	}

	admitted, err := f.admit(t, nil)

	require.NoError(t, err)
	assert.Equal(t, "Example/Model:Latest", admitted.RequestedModel)
	assert.Equal(t, "Example/Model:Stable", admitted.EffectiveModel)
	assert.True(t, admitted.ModelSubstituted)
	encoded := marshalHTTPDispatchAdmission(t, admitted)
	assert.NotContains(t, encoded, testHTTPDispatchToken)
	assert.NotContains(t, encoded, "private agent transcript")
}

func TestAdmitHTTPDispatchReceipt_WithUnreportedModelChange_RejectsCorrelation(t *testing.T) {
	f := newHTTPDispatchAdmissionFixture(t)
	f.receipt["usage"] = map[string]any{"model": "Example/Model:Stable"}

	admitted, err := f.admit(t, nil)

	require.ErrorIs(t, err, errHTTPDispatchCorrelation)
	assert.Nil(t, admitted)
}

type httpDispatchAdmissionFixture struct {
	frozen   []byte
	handle   httpDispatchHandle
	status   httpDispatchStatus
	receipt  map[string]any
	checkout string
}

func newHTTPDispatchAdmissionFixture(t *testing.T) *httpDispatchAdmissionFixture {
	t.Helper()
	f := &httpDispatchAdmissionFixture{frozen: []byte(testHTTPDispatchRequest)}
	require.True(t, decodeHTTPDispatch([]byte(testHTTPDispatchHandle), "Handle", &f.handle))
	require.True(t, decodeHTTPDispatch([]byte(testHTTPDispatchRunning), "Status", &f.status))
	require.NoError(t, json.Unmarshal([]byte(testHTTPDispatchReceipt), &f.receipt))
	f.status.State = "succeeded"
	f.status.ObservedAt = f.handle.AcceptedAt.Add(5 * time.Second)
	return f
}

func (f *httpDispatchAdmissionFixture) object(key string) map[string]any {
	return f.receipt[key].(map[string]any)
}

func (f *httpDispatchAdmissionFixture) pin(checkout, source, result string) {
	f.checkout = checkout
	f.frozen = []byte(strings.Replace(string(f.frozen), `"source_commit": "a5e916aaf179dc06b21dc07cb7e66b37ac3f79be"`, `"source_commit": "`+source+`"`, 1))
	f.bindDigest()
	f.object("repository")["source_commit"] = source
	f.object("repository")["result_commit"] = result
}

func (f *httpDispatchAdmissionFixture) bindDigest() {
	digest := httpDispatchDigest(f.frozen)
	f.handle.RequestSHA256 = digest
	f.status.RequestSHA256 = digest
	f.receipt["request_sha256"] = digest
}

func (f *httpDispatchAdmissionFixture) fail(class, detail string) {
	f.status.State = "failed"
	f.object("execution")["state"] = "failed"
	f.object("execution")["failure"] = map[string]any{"class": class, "detail": detail}
	f.object("repository")["result_commit"] = nil
}

func (f *httpDispatchAdmissionFixture) endAt(ended time.Time) {
	f.status.ObservedAt = ended
	f.object("execution")["ended_at"] = ended.Format(time.RFC3339)
}

func (f *httpDispatchAdmissionFixture) receiptBytes(t *testing.T) []byte {
	t.Helper()
	return []byte(marshalHTTPDispatchAdmission(t, f.receipt))
}

func (f *httpDispatchAdmissionFixture) admit(t *testing.T, roots []string) (*httpDispatchAdmittedReceipt, error) {
	t.Helper()
	return admitHTTPDispatchReceipt(t.Context(), ExecRunner{}, f.checkout, f.frozen, &f.handle, &f.status, f.receiptBytes(t), roots)
}

func marshalHTTPDispatchAdmission(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func commitHTTPDispatchAdmissionFile(t *testing.T, checkout, name string) string {
	t.Helper()
	file := filepath.Join(checkout, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o700))
	require.NoError(t, os.WriteFile(file, []byte("synthetic committed edit\n"), 0o600))
	_, err := runGitBytes(t.Context(), ExecRunner{}, checkout, "add", "-f", "--", name)
	require.NoError(t, err)
	_, err = runGitBytes(t.Context(), ExecRunner{}, checkout, "-c", "core.hooksPath=.git/hooks", "-c", "commit.gpgsign=false", "commit", "-m", "Synthetic result")
	require.NoError(t, err)
	commit, err := runGitText(t.Context(), ExecRunner{}, checkout, "rev-parse", "HEAD")
	require.NoError(t, err)
	return commit
}
