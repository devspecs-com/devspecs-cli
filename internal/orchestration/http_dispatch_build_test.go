package orchestration

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	portableFixtureURL     = "https://git.example/synthetic/portable.git"
	portableFixtureCommit  = "10468d652bb7b272fad86be29b954aa562f70622"
	portableFixturePrompt  = "  Implement <fixture> in isolation.\r\nDevSpecs is read-only; the caller owns promotion.\n\n"
	portableFixtureContext = "portable fixture context\x00\n"
)

func TestBuildHTTPDispatchRequest_WithPinnedCheckout_AcceptsFrozenBytesOverTLS(t *testing.T) {
	// Arrange
	checkout := newPortableFixtureCheckout(t)
	frozen, err := buildHTTPDispatchRequest(t.Context(), ExecRunner{}, checkout, portableFixtureSpec())
	require.NoError(t, err)
	require.NoError(t, os.Rename(checkout, filepath.Join(filepath.Dir(checkout), "offline")))
	transport := newTestHTTPDispatch(t, withHTTPDispatchCapabilities(acceptPortableFixture))
	caps := loadTestHTTPDispatchCaps(t, transport)

	// Act
	handle, err := transport.submit(t.Context(), caps, frozen, nil)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, testHTTPDispatchID, handle.DispatchID)
	assert.Equal(t, httpDispatchDigest(frozen), handle.RequestSHA256)
	assert.NotEqual(t, httpDispatchDigest(append([]byte("\n"), frozen...)), handle.RequestSHA256)
	assert.NotContains(t, string(frozen), "caller-private-checkout")
	assert.NotContains(t, string(frozen), "requested_root")
	assert.NotContains(t, string(frozen), "target_context")
	assert.NotContains(t, string(frozen), testHTTPDispatchToken)
}

func TestBuildHTTPDispatchRequest_WithSameApprovedInputs_ProducesIdenticalBytes(t *testing.T) {
	// Arrange
	spec := portableFixtureSpec()
	first := buildPortableFixture(t, spec)

	// Act
	second := buildPortableFixture(t, spec)

	// Assert
	assert.Equal(t, string(first), string(second))
}

func TestBuildHTTPDispatchRequest_WithoutVerificationOrThread_UsesExplicitOptionalShape(t *testing.T) {
	// Arrange
	spec := portableFixtureSpec()
	spec.Target.Thread = nil
	spec.Verification = nil
	spec.Artifacts = nil

	// Act
	frozen := buildPortableFixture(t, spec)

	// Assert
	assert.Contains(t, string(frozen), `"thread":null`)
	assert.Contains(t, string(frozen), `"verification":{"required":false}`)
	assert.Contains(t, string(frozen), `"context_artifacts":[]`)
}

func TestBuildHTTPDispatchRequest_WithDirtyCheckout_RejectsSource(t *testing.T) {
	// Arrange
	checkout := newPortableFixtureCheckout(t)
	require.NoError(t, os.WriteFile(filepath.Join(checkout, "untracked.txt"), []byte("local"), 0o600))

	// Act
	frozen, err := buildHTTPDispatchRequest(t.Context(), ExecRunner{}, checkout, portableFixtureSpec())

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchRequest)
	assert.ErrorContains(t, err, "dirty")
	assert.Nil(t, frozen)
}

func TestBuildHTTPDispatchRequest_WithUnconfirmedSource_RejectsBeforeGit(t *testing.T) {
	// Arrange
	spec := portableFixtureSpec()
	spec.Source = nil
	runner := &waspflowRunner{}

	// Act
	frozen, err := buildHTTPDispatchRequest(t.Context(), runner, "checkout", spec)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchRequest)
	assert.Nil(t, frozen)
	assert.Empty(t, runner.calls)
}

func TestBuildHTTPDispatchRequest_WithDifferentHEAD_RejectsConfirmedSource(t *testing.T) {
	// Arrange
	spec := portableFixtureSpec()
	spec.Source.Commit = strings.Repeat("a", 40)

	// Act
	frozen, err := buildHTTPDispatchRequest(t.Context(), portableFixtureRunner(), "checkout", spec)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchRequest)
	assert.ErrorContains(t, err, "HEAD does not match")
	assert.Nil(t, frozen)
}

func TestBuildHTTPDispatchRequest_WithCredentialBearingURL_RejectsWithoutEcho(t *testing.T) {
	// Arrange
	spec := portableFixtureSpec()
	spec.Source.URL = "https://user:private-token@git.example/synthetic/portable.git"
	runner := portableFixtureRunner()

	// Act
	frozen, err := buildHTTPDispatchRequest(t.Context(), runner, "checkout", spec)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchRequest)
	assert.NotContains(t, err.Error(), "private-token")
	assert.Nil(t, frozen)
	assert.Empty(t, runner.calls)
}

func TestBuildHTTPDispatchRequest_WithoutApprovedPrompt_RejectsInput(t *testing.T) {
	// Arrange
	spec := portableFixtureSpec()
	spec.ApprovedPrompt = ""

	// Act
	frozen, err := buildHTTPDispatchRequest(t.Context(), portableFixtureRunner(), "checkout", spec)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchRequest)
	assert.Nil(t, frozen)
}

func TestBuildHTTPDispatchRequest_WithUnavailableInlineBytes_RejectsInput(t *testing.T) {
	// Arrange
	spec := portableFixtureSpec()
	spec.Artifacts[0].Content = nil

	// Act
	frozen, err := buildHTTPDispatchRequest(t.Context(), portableFixtureRunner(), "checkout", spec)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchRequest)
	assert.Nil(t, frozen)
}

func TestBuildHTTPDispatchRequest_WithPathAsArtifactID_RejectsInput(t *testing.T) {
	// Arrange
	spec := portableFixtureSpec()
	spec.Artifacts[0].ID = `C:\private\context.txt`

	// Act
	frozen, err := buildHTTPDispatchRequest(t.Context(), portableFixtureRunner(), "checkout", spec)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchRequest)
	assert.Nil(t, frozen)
}

func TestBuildHTTPDispatchRequest_WithWrongExpectedArtifactDigest_RejectsInput(t *testing.T) {
	// Arrange
	spec := portableFixtureSpec()
	spec.Artifacts[0].Content = []byte("different bytes")

	// Act
	frozen, err := buildHTTPDispatchRequest(t.Context(), nil, "checkout", spec)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchRequest)
	assert.ErrorContains(t, err, "digest mismatch")
	assert.Nil(t, frozen)
}

func TestBuildHTTPDispatchRequest_WithOversizedInlineBudget_RejectsBeforeEncoding(t *testing.T) {
	// Arrange
	spec := portableFixtureSpec()
	spec.Artifacts[0].Content = make([]byte, httpDispatchMaxInline)

	// Act
	frozen, err := buildHTTPDispatchRequest(t.Context(), nil, "checkout", spec)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchLimit)
	assert.Nil(t, frozen)
}

func TestPortableDispatchTLSMock_WithWrongArtifactDigest_RejectsInput(t *testing.T) {
	// Arrange
	wire := portableFixtureWire(t)
	wire.Input.ContextArtifacts[0].SHA256 = httpDispatchDigest([]byte("different bytes"))

	// Act
	err := postPortableFixtureWire(t, wire)

	// Assert
	assertPortableFixtureRejection(t, err, http.StatusBadRequest, "invalid_request")
}

func TestPortableDispatchTLSMock_WithReencodedApply_RejectsOriginalApplyDigest(t *testing.T) {
	// Arrange
	wire := portableFixtureWire(t)
	wire.Input.ApplyPayload += "\n" // Same decoded object, different authoritative apply bytes.

	// Act
	err := postPortableFixtureWire(t, wire)

	// Assert
	assertPortableFixtureRejection(t, err, http.StatusBadRequest, "invalid_request")
}

func TestPortableDispatchTLSMock_WithWrongPromptDigest_RejectsInput(t *testing.T) {
	// Arrange
	wire := portableFixtureWire(t)
	wire.Input.PromptSHA256 = wire.Input.ApplySHA256

	// Act
	err := postPortableFixtureWire(t, wire)

	// Assert
	assertPortableFixtureRejection(t, err, http.StatusBadRequest, "invalid_request")
}

func TestPortableDispatchTLSMock_WithOversizedDecodedInput_RejectsDespiteValidDigest(t *testing.T) {
	// Arrange
	wire := portableFixtureWire(t)
	large := make([]byte, httpDispatchMaxInline)
	artifact := &wire.Input.ContextArtifacts[0]
	artifact.Content = base64.StdEncoding.EncodeToString(large)
	artifact.Size = int64(len(large))
	artifact.SHA256 = httpDispatchDigest(large)

	// Act
	err := postPortableFixtureWire(t, wire)

	// Assert
	assertPortableFixtureRejection(t, err, http.StatusRequestEntityTooLarge, "payload_too_large")
}

func TestPortableDispatchTLSMock_WithoutRequiredInlineArtifact_RejectsMissingInput(t *testing.T) {
	// Arrange
	wire := portableFixtureWire(t)
	wire.Input.ContextArtifacts = wire.Input.ContextArtifacts[1:]

	// Act
	err := postPortableFixtureWire(t, wire)

	// Assert
	assertPortableFixtureRejection(t, err, http.StatusBadRequest, "invalid_request")
}

func TestPortableDispatchTLSMock_WithURLOnlyArtifact_RejectsWithoutFetching(t *testing.T) {
	// Arrange
	var wire map[string]any
	require.NoError(t, json.Unmarshal(buildPortableFixture(t, portableFixtureSpec()), &wire))
	artifacts := wire["input"].(map[string]any)["context_artifacts"].([]any)
	artifact := artifacts[0].(map[string]any)
	artifact["kind"] = "https"
	artifact["url"] = "https://git.example/synthetic/context.bin"
	delete(artifact, "content_base64")

	// Act
	err := postPortableFixtureWire(t, wire)

	// Assert
	assertPortableFixtureRejection(t, err, http.StatusBadRequest, "invalid_request")
}

func portableFixtureSpec() httpDispatchBuildSpec {
	thread := "Thread_01"
	return httpDispatchBuildSpec{
		DispatchID: testHTTPDispatchID, CreatedAt: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
		Caller:         httpDispatchCaller{PrincipalID: "principal_fixture", CredentialID: "credential_fixture_v1"},
		Target:         Target{Owner: "task:portable-fixture", Slice: "G03", Thread: &thread},
		Source:         &httpDispatchConfirmedSource{URL: portableFixtureURL, Commit: portableFixtureCommit},
		ApprovedPrompt: portableFixturePrompt,
		Artifacts: []httpDispatchInlineInput{
			{ID: "z-context", MediaType: "application/octet-stream", Content: []byte(portableFixtureContext), SHA256: httpDispatchDigest([]byte(portableFixtureContext))},
			{ID: "a-empty", MediaType: "text/plain", Content: []byte{}, SHA256: httpDispatchDigest(nil)},
		},
		TimeoutSeconds: 420, MaxAttempts: 1, ReportRequired: true, CancellationRequired: true,
		Verification: &httpDispatchVerificationProfile{Profile: "go-unit", Strength: "suite"},
		Options:      httpDispatchAgentOptions{AgentProvider: "ExampleRuntime", Model: "Example/Model:Latest", Effort: "High"},
	}
}

func newPortableFixtureCheckout(t *testing.T) string {
	t.Helper()
	checkout := filepath.Join(t.TempDir(), "caller-private-checkout")
	require.NoError(t, os.Mkdir(checkout, 0o700))
	t.Setenv("GIT_AUTHOR_DATE", "1760000000 +0000")
	t.Setenv("GIT_COMMITTER_DATE", "1760000000 +0000")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")
	_, err := runGitBytes(t.Context(), ExecRunner{}, checkout, "init", "--object-format=sha1", "--template=")
	require.NoError(t, err)
	_, err = runGitBytes(t.Context(), ExecRunner{}, checkout, "-c", "core.hooksPath=.git/hooks", "-c", "commit.gpgsign=false",
		"commit", "--allow-empty", "-m", "Portable fixture")
	require.NoError(t, err)
	return checkout
}

func portableFixtureRunner() *waspflowRunner {
	return &waspflowRunner{results: []waspflowRun{{}, {result: CommandResult{Stdout: []byte(portableFixtureCommit + "\n")}}}}
}

func buildPortableFixture(t *testing.T, spec httpDispatchBuildSpec) []byte {
	t.Helper()
	frozen, err := buildHTTPDispatchRequest(t.Context(), portableFixtureRunner(), "checkout", spec)
	require.NoError(t, err)
	return frozen
}

func portableFixtureWire(t *testing.T) httpDispatchPortableRequest {
	t.Helper()
	var wire httpDispatchPortableRequest
	require.NoError(t, json.Unmarshal(buildPortableFixture(t, portableFixtureSpec()), &wire))
	return wire
}

func postPortableFixtureWire(t *testing.T, wire any) error {
	t.Helper()
	transport := newTestHTTPDispatch(t, acceptPortableFixture)
	raw, err := json.Marshal(wire)
	require.NoError(t, err)
	// Bypass submit's local validation to exercise rejection at the TLS peer.
	_, _, err = transport.exchange(t.Context(), http.MethodPost, "/dispatches", raw, testHTTPDispatchID, testHTTPDispatchID, httpDispatchSmallBody, http.StatusCreated)
	return err
}

func assertPortableFixtureRejection(t *testing.T, err error, status int, code string) {
	t.Helper()
	var rejected *httpDispatchError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, status, rejected.Status)
	assert.Equal(t, code, rejected.Code)
}

// The mock has no checkout, filesystem dependency, or builder inputs. Its
// expected fixture is pinned; executable input is recovered from the POST.
func acceptPortableFixture(w http.ResponseWriter, r *http.Request) {
	badRequest := func() {
		writeHTTPDispatchMock(w, http.StatusBadRequest, testHTTPDispatchError("invalid_request", false))
	}
	tooLarge := func() {
		writeHTTPDispatchMock(w, http.StatusRequestEntityTooLarge, testHTTPDispatchError("payload_too_large", false))
	}
	if r.Method != http.MethodPost || r.URL.Path != "/v2/dispatches" || r.Header.Get("Content-Type") != "application/json" ||
		r.Header.Get("Content-Encoding") != "" {
		badRequest()
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, httpDispatchRequestBody+1))
	if err != nil {
		badRequest()
		return
	}
	if len(raw) > httpDispatchRequestBody {
		tooLarge()
		return
	}
	var req httpDispatchRequest
	if !decodeHTTPDispatch(raw, "Request", &req) || r.Header.Get("Idempotency-Key") != req.DispatchID {
		badRequest()
		return
	}
	inline, fetched, ok := req.validateInput()
	if !ok || fetched != 0 {
		badRequest()
		return
	}
	if inline > httpDispatchMaxInline {
		tooLarge()
		return
	}
	var wire httpDispatchPortableRequest
	var apply httpDispatchPortableApply
	if json.Unmarshal(raw, &wire) != nil || json.Unmarshal([]byte(req.Input.ApplyPayload), &apply) != nil ||
		apply.Target != "G03" || apply.Prompt != portableFixturePrompt || wire.Target.Owner != "task:portable-fixture" ||
		wire.Target.Thread == nil || *wire.Target.Thread != "Thread_01" || wire.Repository.GitURL != portableFixtureURL ||
		wire.Repository.SourceCommit != portableFixtureCommit || wire.Repository.LogicalID != "git:"+portableFixtureURL ||
		wire.Options.AgentProvider != "ExampleRuntime" || wire.Options.Model != "Example/Model:Latest" || wire.Options.Effort != "High" ||
		wire.Requirements.TimeoutSeconds != 420 || wire.Requirements.MaxAttempts != 1 || !wire.Requirements.Report.Required ||
		wire.Requirements.Cancellation != "required" || !wire.Requirements.Verification.Required ||
		wire.Requirements.Verification.Profile != "go-unit" || wire.Requirements.Verification.Strength != "suite" ||
		len(wire.Input.ContextArtifacts) != 2 ||
		!matchesPortableFixtureArtifact(wire.Input.ContextArtifacts[0], "z-context", "application/octet-stream", portableFixtureContext) ||
		!matchesPortableFixtureArtifact(wire.Input.ContextArtifacts[1], "a-empty", "text/plain", "") {
		badRequest()
		return
	}
	writeHTTPDispatchMock(w, http.StatusCreated, fmt.Sprintf(`{"schema":"devspecs.dispatch.handle","schema_version":2,"dispatch_id":%q,"request_sha256":%q,"provider":{"name":"fixture-provider","run_id":"run_portable_fixture"},"accepted_at":"2026-11-10T12:00:00Z"}`,
		req.DispatchID, httpDispatchDigest(raw)))
}

func matchesPortableFixtureArtifact(a httpDispatchPortableArtifact, id, media, content string) bool {
	decoded, err := base64.StdEncoding.DecodeString(a.Content)
	return err == nil && a.Kind == "inline" && a.ID == id && a.MediaType == media && string(decoded) == content
}
