package orchestration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPDriverServiceDispatch_WithExplicitReadOnlyGoal_PersistsFrozenRequestBeforePost(t *testing.T) {
	f := newHTTPDriverFixture(t)

	handle, path, err := f.service.Dispatch(t.Context(), f.options())

	require.NoError(t, err)
	assert.Equal(t, "http", handle.Provider.Name)
	assert.Equal(t, SchemaVersion, handle.SchemaVersion)
	assert.FileExists(t, path)
	assert.True(t, f.recoveryBeforePost.Load())
	require.Len(t, f.posts, 1)
	raw := <-f.posts
	var request httpDispatchPortableRequest
	require.True(t, decodeHTTPDispatch(raw, "Request", &request))
	assert.Equal(t, f.profile.SourceURL, request.Repository.GitURL)
	assert.Equal(t, f.profile.SourceCommit, request.Repository.SourceCommit)
	assert.Equal(t, "credential_fixture_v1", request.Caller.CredentialID)
	assert.Contains(t, request.Input.ApplyPayload, "Do not edit any files")
	assert.NotContains(t, string(raw), f.checkout)
	assert.NotContains(t, string(raw), testHTTPDispatchToken)
	assert.Equal(t, 1, request.Requirements.MaxAttempts)
}

func TestHTTPDriverServiceDispatch_WithInlineContext_FreezesActualBytes(t *testing.T) {
	f := newHTTPDriverFixture(t)
	content := []byte("reviewed context\n")
	f.runner.payload = []byte(`{"target":"A01","prompt":"Review only.","context_artifacts":[{"artifact_id":"plan","revision_id":"rev1","sha256":"` +
		httpDispatchDigest(content) + `","media_type":"text/plain","content_base64":"` + base64.StdEncoding.EncodeToString(content) + `"}]}`)

	_, _, err := f.service.Dispatch(t.Context(), f.options())

	require.NoError(t, err)
	var request httpDispatchPortableRequest
	require.NoError(t, json.Unmarshal(<-f.posts, &request))
	require.Len(t, request.Input.ContextArtifacts, 1)
	artifact := request.Input.ContextArtifacts[0]
	assert.Equal(t, "inline", artifact.Kind)
	assert.Equal(t, base64.StdEncoding.EncodeToString(content), artifact.Content)
	assert.Equal(t, httpDispatchDigest(content), artifact.SHA256)
}

func TestHTTPDriverServiceDispatch_WithReferenceOnlyContext_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.runner.payload = []byte(`{"target":"A01","prompt":"Review only.","context_artifacts":[{"artifact_id":"plan","revision_id":"rev1","sha256":"` + httpDispatchDigest(nil) + `"}]}`)

	_, _, err := f.service.Dispatch(t.Context(), f.options())

	require.ErrorIs(t, err, errHTTPDriverGoal)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServiceDispatch_WithoutCallerReadOnlySelection_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	opts := f.options()
	opts.ReadOnly = false

	_, _, err := f.service.Dispatch(t.Context(), opts)

	require.ErrorIs(t, err, errHTTPDriverGoal)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServicePreflight_WithWrongOrigin_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	_, err := runGitBytes(t.Context(), ExecRunner{}, f.checkout, "remote", "set-url", "origin", "https://other.invalid/repo.git")
	require.NoError(t, err)

	_, err = f.service.Preflight(t.Context(), f.checkout)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServicePreflight_WithTrustedCheckout_RemainsLocalOnly(t *testing.T) {
	f := newHTTPDriverFixture(t)

	caps, err := f.service.Preflight(t.Context(), f.checkout)

	require.NoError(t, err)
	assert.True(t, caps.Isolation)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServiceDispatch_WithDirtyCheckout_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(f.checkout, "dirty.txt"), []byte("dirty"), 0o600))

	_, _, err := f.service.Dispatch(t.Context(), f.options())

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServiceDispatch_WithProfileInsideTaskRepo_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	inside := filepath.Join(f.taskRepo, "profile.json")
	f.writeProfile(t, inside)
	t.Setenv("DEVSPECS_HTTP_DISPATCH_CONFIG", inside)

	_, _, err := f.service.Dispatch(t.Context(), f.options())

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServiceStatus_WithAcceptedDispatch_ReturnsRemoteState(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.arrangePending(t, true)

	status, err := f.service.Status(t.Context(), testHTTPDispatchID)

	require.NoError(t, err)
	assert.Equal(t, "running", status.State)
	assert.Equal(t, "http", status.Provider.Name)
	assert.EqualValues(t, 1, f.calls.Load())
	assert.Empty(t, f.posts)
}

func TestHTTPDriverServiceStatus_WithUnknownPostOutcome_DoesNotReplay(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.arrangePending(t, false)

	status, err := f.service.Status(t.Context(), testHTTPDispatchID)

	require.NoError(t, err)
	assert.Equal(t, "pending", status.State)
	assert.Zero(t, f.calls.Load())
	assert.Empty(t, f.posts)
}

func TestHTTPDriverServiceWait_AfterCrash_ReplaysFrozenBytesAndOriginalCredentialID(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.mode = "succeeded"
	original := f.arrangePending(t, false)
	f.profile.CredentialID = "credential_rotated"
	f.writeProfile(t, f.profilePath)
	t.Setenv("HTTP_DRIVER_FIXTURE_TOKEN", "rotated-synthetic-token")

	_, err := f.service.Wait(t.Context(), testHTTPDispatchID)

	require.NoError(t, err)
	require.Len(t, f.posts, 1)
	assert.Equal(t, string(original), string(<-f.posts))
	assert.Equal(t, "Bearer rotated-synthetic-token", f.lastAuthorization())
	assert.FileExists(t, filepath.Join(f.service.Store.Root, httpDriverStateDir, testHTTPDispatchID+".accepted.json"))
}

func TestHTTPDriverServiceDispatch_WithLostPostResponse_PreservesRecoverableAuthority(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.mode = "lost"

	_, _, err := f.service.Dispatch(t.Context(), f.options())

	require.ErrorIs(t, err, errHTTPDispatchNetwork)
	assert.FileExists(t, f.service.Store.HandlePath(testHTTPDispatchID))
	assert.FileExists(t, filepath.Join(f.service.Store.Root, httpDriverStateDir, testHTTPDispatchID+".pending.json"))
	assert.NoFileExists(t, filepath.Join(f.service.Store.Root, httpDriverStateDir, testHTTPDispatchID+".accepted.json"))
	require.Len(t, f.posts, 1)
}

func TestHTTPDriverServiceStatus_WithChangedEndpoint_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.arrangePending(t, true)
	f.profile.Endpoint += "/changed"
	f.writeProfile(t, f.profilePath)

	_, err := f.service.Status(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServiceStatus_WithOtherPrincipal_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.arrangePending(t, true)
	f.profile.PrincipalID = "principal_other"
	f.writeProfile(t, f.profilePath)

	_, err := f.service.Status(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServiceDispatch_WithProviderDenial_RedactsSecretsAndMessages(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.mode = "denied"

	_, _, err := f.service.Dispatch(t.Context(), f.options())

	require.Error(t, err)
	assert.NotContains(t, err.Error(), testHTTPDispatchToken)
	assert.NotContains(t, err.Error(), "provider private message")
	assert.NotContains(t, err.Error(), f.server.URL)
	assert.NoFileExists(t, f.service.Store.HandlePath(testHTTPDispatchID))
}

func TestHTTPDriverServiceWait_WithCanceledContext_DoesNotAuthenticateOrCancelWorker(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.arrangePending(t, true)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := f.service.Wait(ctx, testHTTPDispatchID)

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, f.calls.Load())
	assert.FileExists(t, f.service.Store.HandlePath(testHTTPDispatchID))
}

func TestHTTPDriverServiceFinalize_WithUnchangedResult_WritesCompatibleReceipt(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.mode = "succeeded"
	f.arrangePending(t, true)

	receipt, path, err := f.service.Finalize(t.Context(), testHTTPDispatchID)

	require.NoError(t, err)
	assert.Equal(t, SchemaVersion, receipt.SchemaVersion)
	assert.Equal(t, "succeeded", receipt.Execution.State)
	require.NotNil(t, receipt.Repository.ResultCommit)
	assert.Equal(t, f.profile.SourceCommit, *receipt.Repository.ResultCommit)
	assert.FileExists(t, path)
	assert.FileExists(t, filepath.Join(f.service.Store.Root, httpDriverStateDir, testHTTPDispatchID+".evidence.json"))
}

func TestHTTPDriverServiceFinalize_WithEditedResult_RefusesReceiptWithoutMutation(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.mode = "edited"
	f.arrangePending(t, true)

	_, _, err := f.service.Finalize(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverEdited)
	assert.NoFileExists(t, f.service.Store.ReceiptPath(testHTTPDispatchID))
	head, gitErr := runGitText(t.Context(), ExecRunner{}, f.checkout, "rev-parse", "HEAD")
	require.NoError(t, gitErr)
	assert.Equal(t, f.profile.SourceCommit, head)
}

func TestHTTPDriverServicePreflight_WithoutReadOnlyProfile_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.profile.ReadOnly = false
	f.writeProfile(t, f.profilePath)

	_, err := f.service.Preflight(t.Context(), f.checkout)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServicePreflight_WithUnknownProfileField_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	raw, err := json.Marshal(f.profile)
	require.NoError(t, err)
	raw = append([]byte(`{"unknown":true,`), raw[1:]...)
	require.NoError(t, os.WriteFile(f.profilePath, raw, 0o600))

	_, err = f.service.Preflight(t.Context(), f.checkout)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServicePreflight_WithDuplicateProfileField_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	raw, err := json.Marshal(f.profile)
	require.NoError(t, err)
	raw = append([]byte(`{"enabled":true,`), raw[1:]...)
	require.NoError(t, os.WriteFile(f.profilePath, raw, 0o600))

	_, err = f.service.Preflight(t.Context(), f.checkout)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServicePreflight_WithChangedPinnedCommit_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.profile.SourceCommit = strings.Repeat("b", 40)
	f.writeProfile(t, f.profilePath)

	_, err := f.service.Preflight(t.Context(), f.checkout)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServiceStatus_WithChangedSourceURL_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.arrangePending(t, true)
	f.profile.SourceURL = "https://other.invalid/fixture.git"
	f.writeProfile(t, f.profilePath)

	_, err := f.service.Status(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServiceStatus_WithChangedTrustAnchor_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.arrangePending(t, true)
	file, err := os.OpenFile(f.profile.CAFile, os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = file.WriteString("\n")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	_, err = f.service.Status(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServiceWait_WithExpiredPending_RefusesReplay(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.arrangePending(t, false)
	path := filepath.Join(f.service.Store.Root, httpDriverStateDir, testHTTPDispatchID+".pending.json")
	var p httpDriverPending
	require.NoError(t, readJSONFile(path, &p))
	p.ExpiresAt = time.Now().Add(-time.Second)
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))

	_, err = f.service.Wait(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverExpired)
	assert.Zero(t, f.calls.Load())
	assert.Empty(t, f.posts)
}

func TestHTTPDriverServiceResume_WithCachedReceiptAndChangedPrincipal_RefusesLocalSuccess(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.arrangePending(t, true)
	_, err := f.service.Store.WriteReceipt(DispatchReceipt{Schema: DispatchReceiptSchema, SchemaVersion: SchemaVersion,
		DispatchID: testHTTPDispatchID, Provider: ReceiptProvider{Name: "http"}, Execution: ReceiptExecution{State: "succeeded"}})
	require.NoError(t, err)
	f.profile.PrincipalID = "principal_other"
	f.writeProfile(t, f.profilePath)

	_, _, err = f.service.Resume(t.Context(), testHTTPDispatchID)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServiceDispatch_WithExternalInput_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.runner.payload = []byte(`{"target":"A01","prompt":"Review only.","external_inputs":["https://external.invalid/input"]}`)

	_, _, err := f.service.Dispatch(t.Context(), f.options())

	require.ErrorIs(t, err, errHTTPDriverGoal)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServiceDispatch_WithUnsupportedMCP_RefusesBeforeApplyAndAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	opts := f.options()
	opts.MCP = "external"

	_, _, err := f.service.Dispatch(t.Context(), opts)

	require.ErrorIs(t, err, errHTTPDriverGoal)
	assert.Zero(t, f.calls.Load())
	assert.NoFileExists(t, f.service.Store.RequestPath(testHTTPDispatchID))
}

func TestHTTPDriverServiceStatus_WithOversizedLocalRequest_RefusesBeforeAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	f.arrangePending(t, true)
	file, err := os.OpenFile(f.service.Store.RequestPath(testHTTPDispatchID), os.O_WRONLY, 0o600)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(32<<20))
	require.NoError(t, file.Close())

	_, err = f.service.Status(t.Context(), testHTTPDispatchID)

	require.Error(t, err)
	assert.ErrorContains(t, err, "2 MiB")
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServiceDispatch_WithCallerSelectedRealApply_PreservesActualTaskPrompt(t *testing.T) {
	binary, taskRepo := os.Getenv("DEVSPECS_HTTP_APPLY_BINARY"), os.Getenv("DEVSPECS_HTTP_APPLY_FIXTURE")
	if binary == "" || taskRepo == "" {
		t.Skip("caller-selected real apply fixture is not configured")
	}
	f := newHTTPDriverFixture(t)
	opts := f.options()
	opts.DSExecutable, opts.TaskRepo, opts.Target = binary, taskRepo, "task:http-normal-fixture"

	_, _, err := f.service.Dispatch(t.Context(), opts)

	require.NoError(t, err)
	var request httpDispatchPortableRequest
	require.True(t, decodeHTTPDispatch(<-f.posts, "Request", &request))
	assert.Equal(t, "H01", request.Target.Slice)
	assert.Equal(t, "task:http-normal-fixture", request.Target.Owner)
	assert.Greater(t, len(request.Input.ApplyPayload), 5000)
	assert.Contains(t, request.Input.ApplyPayload, "Do not edit any files")
	assert.Empty(t, request.Input.ContextArtifacts)
}

func TestHTTPDriverServiceDispatch_WhenOriginChangesAfterCapabilities_RefusesPost(t *testing.T) {
	f := newHTTPDriverFixture(t)
	changed := make(chan error, 1)
	f.afterCapabilities = func() {
		_, err := runGitBytes(context.Background(), ExecRunner{}, f.checkout, "remote", "set-url", "origin", "https://other.invalid/fixture.git")
		changed <- err
	}

	_, _, err := f.service.Dispatch(t.Context(), f.options())

	require.ErrorIs(t, err, errHTTPDriverBinding)
	require.NoError(t, <-changed)
	assert.Empty(t, f.posts)
	assert.EqualValues(t, 1, f.calls.Load())
}

func TestHTTPDriverServiceDispatch_WhenRecoveryHandlePublicationFails_DoesNotPost(t *testing.T) {
	f := newHTTPDriverFixture(t)
	require.NoError(t, os.MkdirAll(f.service.Store.HandlePath(testHTTPDispatchID), 0o700))

	_, _, err := f.service.Dispatch(t.Context(), f.options())

	require.ErrorIs(t, err, errHTTPDriverState)
	assert.Empty(t, f.posts)
	assert.FileExists(t, filepath.Join(f.service.Store.Root, httpDriverStateDir, testHTTPDispatchID+".pending.json"))
	_, _, loadErr := f.service.Store.LoadHandle(testHTTPDispatchID)
	assert.Error(t, loadErr)
}

func TestHTTPDriverServiceDispatch_WithAmbientGitDirAndWorkTree_UsesActualExecutionCheckout(t *testing.T) {
	f := newHTTPDriverFixture(t)
	other := newPortableFixtureCheckout(t)
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "remote.origin.url")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://ambient.invalid/not-the-execution.git")

	_, _, err := f.service.Dispatch(t.Context(), f.options())

	require.NoError(t, err)
	var request httpDispatchPortableRequest
	require.NoError(t, json.Unmarshal(<-f.posts, &request))
	assert.Equal(t, f.profile.SourceCommit, request.Repository.SourceCommit)
	assert.Equal(t, f.profile.SourceURL, request.Repository.GitURL)
}

func TestHTTPDriverServicePreflight_WithLocalFsmonitorCanary_DoesNotExecuteOrExposeBearer(t *testing.T) {
	f := newHTTPDriverFixture(t)
	canary := filepath.Join(t.TempDir(), "fsmonitor-secret.txt")
	hook := filepath.Join(t.TempDir(), "fsmonitor.sh")
	script := "#!/bin/sh\nprintf '%s' \"$HTTP_DRIVER_FIXTURE_TOKEN\" > '" + filepath.ToSlash(canary) + "'\nprintf 'fixture-token\\0'\n"
	require.NoError(t, os.WriteFile(hook, []byte(script), 0o700))
	_, err := runGitBytes(t.Context(), ExecRunner{}, f.checkout, "config", "core.fsmonitor", filepath.ToSlash(hook))
	require.NoError(t, err)
	_, err = runGitBytes(t.Context(), ExecRunner{}, f.checkout, "status", "--porcelain=v1")
	require.NoError(t, err)
	require.FileExists(t, canary)
	require.NoError(t, os.Remove(canary))

	_, err = f.service.Preflight(t.Context(), f.checkout)

	require.NoError(t, err)
	assert.NoFileExists(t, canary)
	assert.Zero(t, f.calls.Load())
}

func TestHTTPDriverServicePreflight_WithConfiguredAttributeFilter_RefusesBeforeStatusOrAuthentication(t *testing.T) {
	f := newHTTPDriverFixture(t)
	canary := filepath.Join(t.TempDir(), "filter-secret.txt")
	hook := filepath.Join(t.TempDir(), "filter.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\nprintf '%s' \"$HTTP_DRIVER_FIXTURE_TOKEN\" > '"+filepath.ToSlash(canary)+"'\ncat\n"), 0o700))
	_, err := runGitBytes(t.Context(), ExecRunner{}, f.checkout, "config", "filter.canary.clean", filepath.ToSlash(hook))
	require.NoError(t, err)

	_, err = f.service.Preflight(t.Context(), f.checkout)

	require.ErrorIs(t, err, errHTTPDriverBinding)
	assert.NoFileExists(t, canary)
	assert.Zero(t, f.calls.Load())
}

type httpDriverApplyRunner struct{ payload []byte }

func (r *httpDriverApplyRunner) Run(ctx context.Context, dir, executable string, args ...string) (CommandResult, error) {
	if executable == "fixture-ds" {
		return CommandResult{Stdout: r.payload}, nil
	}
	return (ExecRunner{}).Run(ctx, dir, executable, args...)
}

type httpDriverFixture struct {
	service                         *Service
	runner                          *httpDriverApplyRunner
	checkout, taskRepo, profilePath string
	profile                         httpDriverProfile
	server                          *httptest.Server
	mode                            string
	calls                           atomic.Int32
	recoveryBeforePost              atomic.Bool
	posts                           chan []byte
	mu                              sync.Mutex
	frozen                          []byte
	authorization                   string
	afterCapabilities               func()
	capabilities                    string
	observation                     func(string, map[string]any)
	artifactHandler                 http.HandlerFunc
}

func newHTTPDriverFixture(t *testing.T) *httpDriverFixture {
	t.Helper()
	f := &httpDriverFixture{checkout: newPortableFixtureCheckout(t), taskRepo: t.TempDir(), posts: make(chan []byte, 4)}
	writeOrchestrationConfig(t, f.checkout, "http", map[string]any{"goal": "read_only"})
	_, err := runGitBytes(t.Context(), ExecRunner{}, f.checkout, "add", "-f", ".devspecs/config.yaml")
	require.NoError(t, err)
	_, err = runGitBytes(t.Context(), ExecRunner{}, f.checkout, "-c", "core.hooksPath=.git/hooks", "-c", "commit.gpgsign=false", "commit", "-m", "HTTP fixture config")
	require.NoError(t, err)
	head, err := runGitText(t.Context(), ExecRunner{}, f.checkout, "rev-parse", "HEAD")
	require.NoError(t, err)
	_, err = runGitBytes(t.Context(), ExecRunner{}, f.checkout, "remote", "add", "origin", "https://source.invalid/fixture.git")
	require.NoError(t, err)
	f.runner = &httpDriverApplyRunner{payload: []byte(`{"target":"A01","prompt":"Review the repository without edits."}`)}
	f.service = NewService(t.TempDir())
	f.service.Runner = f.runner
	f.service.NewID = func() string { return testHTTPDispatchID }
	f.service.Now = func() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) }
	f.server = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	trusted := t.TempDir()
	ca := filepath.Join(trusted, "fixture-ca.pem")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.server.Certificate().Raw}), 0o600))
	f.profile = httpDriverProfile{Schema: "devspecs.dispatch.http_config", Version: 1, Enabled: true, ReadOnly: true,
		Endpoint: f.server.URL + "/v2", SourceURL: "https://source.invalid/fixture.git", SourceCommit: head,
		PrincipalID: "principal_fixture", CredentialID: "credential_fixture_v1", TokenEnv: "HTTP_DRIVER_FIXTURE_TOKEN", CAFile: ca}
	f.profilePath = filepath.Join(trusted, "profile.json")
	f.writeProfile(t, f.profilePath)
	t.Setenv("DEVSPECS_HTTP_DISPATCH_CONFIG", f.profilePath)
	t.Setenv("HTTP_DRIVER_FIXTURE_TOKEN", testHTTPDispatchToken)
	return f
}

func (f *httpDriverFixture) writeProfile(t *testing.T, path string) {
	t.Helper()
	raw, err := json.Marshal(f.profile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
}

func (f *httpDriverFixture) options() DispatchOptions {
	return DispatchOptions{TaskRepo: f.taskRepo, ExecutionRepo: f.checkout, Target: "task:fixture", AgentProvider: "fixture-agent",
		TimeoutSeconds: 600, VerifyTimeoutSeconds: 600, DSExecutable: "fixture-ds", ReadOnly: f.profile.ReadOnly, AllowEdits: f.profile.AllowEdits}
}

func (f *httpDriverFixture) lastAuthorization() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.authorization
}

func (f *httpDriverFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	f.mu.Lock()
	f.authorization = r.Header.Get("Authorization")
	f.mu.Unlock()
	if f.mode == "denied" {
		writeHTTPDispatchMock(w, http.StatusForbidden, testHTTPDispatchError("forbidden", false))
		return
	}
	if r.URL.Path == "/v2/capabilities" {
		if f.afterCapabilities != nil {
			f.afterCapabilities()
		}
		caps := f.capabilities
		if caps == "" {
			caps = testHTTPDispatchCaps
		}
		writeHTTPDispatchMock(w, http.StatusOK, caps)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/artifacts/result-bundle") && f.artifactHandler != nil {
		f.artifactHandler(w, r)
		return
	}
	if r.URL.Path == "/v2/dispatches" && r.Method == http.MethodPost {
		raw, err := io.ReadAll(io.LimitReader(r.Body, httpDispatchRequestBody+1))
		if err != nil {
			w.WriteHeader(400)
			return
		}
		f.mu.Lock()
		f.frozen = raw
		f.mu.Unlock()
		f.posts <- raw
		_, _, recoveryErr := f.service.Store.LoadHandle(testHTTPDispatchID)
		f.recoveryBeforePost.Store(recoveryErr == nil)
		if f.mode == "lost" {
			conn, _, err := http.NewResponseController(w).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		f.writeObservation(w, "handle")
		return
	}
	if strings.HasSuffix(r.URL.Path, "/receipt") {
		f.writeObservation(w, "receipt")
		return
	}
	f.writeObservation(w, "status")
}

func (f *httpDriverFixture) writeObservation(w http.ResponseWriter, kind string) {
	f.mu.Lock()
	frozen := append([]byte(nil), f.frozen...)
	f.mu.Unlock()
	var request httpDispatchPortableRequest
	if json.Unmarshal(frozen, &request) != nil {
		w.WriteHeader(500)
		return
	}
	object := map[string]any{"schema": "devspecs.dispatch." + kind, "schema_version": 2, "dispatch_id": request.DispatchID,
		"request_sha256": httpDispatchDigest(frozen), "provider": map[string]any{"name": "fixture-provider", "run_id": "fixture-run"}}
	accepted := request.CreatedAt
	switch kind {
	case "handle":
		object["accepted_at"] = accepted
	case "status":
		state := "running"
		if f.mode == "succeeded" || f.mode == "edited" {
			state = "succeeded"
		}
		object["state"], object["cancel_requested"], object["observed_at"] = state, false, accepted.Add(3*time.Second)
	case "receipt":
		object["caller"], object["receipt_id"] = request.Caller, "receipt_01KZZW1KXP4AZ7TSB0NN68HJ86"
		object["execution"] = map[string]any{"state": "succeeded", "attempts": 1, "started_at": accepted, "ended_at": accepted.Add(2 * time.Second), "failure": nil}
		result := request.Repository.SourceCommit
		if f.mode == "edited" {
			result = strings.Repeat("a", 40)
		}
		object["repository"] = map[string]any{"logical_id": request.Repository.LogicalID, "source_commit": request.Repository.SourceCommit, "result_commit": result}
		object["evidence"] = map[string]any{"files_read": []string{}, "files_edited": []string{}, "paths_complete": true}
		object["report"] = map[string]any{"state": "not_requested"}
		object["verification"] = map[string]any{"state": "not_requested"}
		object["cancellation"] = map[string]any{"supported": true, "requested": false}
	}
	if f.observation != nil {
		f.observation(kind, object)
	}
	raw, err := json.Marshal(object)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	code := http.StatusOK
	if kind == "handle" {
		code = http.StatusOK
	}
	writeHTTPDispatchMock(w, code, string(raw))
}

func (f *httpDriverFixture) arrangePending(t *testing.T, accepted bool) []byte {
	t.Helper()
	request := DispatchRequest{Schema: DispatchRequestSchema, SchemaVersion: SchemaVersion, DispatchID: testHTTPDispatchID,
		CreatedAt: f.service.Now(), Target: Target{Owner: "task:fixture", Slice: "A01"},
		Repository:   Repository{RequestedRoot: f.checkout, SourceCommit: f.profile.SourceCommit, SourceState: "clean"},
		Input:        Input{ApplyPayload: f.runner.payload, ApplySHA256: sha256Bytes(f.runner.payload)},
		Requirements: Requirements{TimeoutSeconds: 600, MaxAttempts: 1, Isolation: "required"}}
	path, digest, err := f.service.Store.WriteRequest(request)
	require.NoError(t, err)
	_, trust, _, err := loadHTTPDriverProfile(f.checkout, f.taskRepo)
	require.NoError(t, err)
	prompt, artifacts, err := httpDriverInput(request)
	require.NoError(t, err)
	if f.profile.AllowEdits {
		prompt = orchestrationPrompt("Implement the requested fixture edit.")
	}
	frozen, err := buildHTTPDispatchRequest(t.Context(), ExecRunner{}, f.checkout, httpDispatchBuildSpec{
		DispatchID: request.DispatchID, CreatedAt: request.CreatedAt, Caller: httpDispatchCaller{PrincipalID: f.profile.PrincipalID, CredentialID: f.profile.CredentialID},
		Target: request.Target, Source: &httpDispatchConfirmedSource{URL: f.profile.SourceURL, Commit: f.profile.SourceCommit},
		ApprovedPrompt: prompt, Artifacts: artifacts, TimeoutSeconds: 600, MaxAttempts: 1, ReportRequired: f.profile.ReportRequired, Verification: f.profile.Verification, Options: httpDispatchAgentOptions{AgentProvider: "fixture-agent"}})
	require.NoError(t, err)
	f.frozen = frozen
	d := &httpDriver{store: f.service.Store, checkout: f.checkout, readOnlyGoal: f.profile.ReadOnly, editedGoal: f.profile.AllowEdits}
	require.NoError(t, d.writeRecord(request.DispatchID, ".pending.json", httpDriverPending{Version: 1, ID: request.DispatchID, Frozen: frozen,
		Digest: httpDispatchDigest(frozen), V1Digest: digest, Trust: trust, Checkout: f.checkout, TaskRepo: f.taskRepo, Provider: "fixture-provider", ExpiresAt: time.Now().UTC().Add(time.Hour)}))
	adapter := "http-v2-read-only-1"
	if f.profile.AllowEdits {
		adapter = "http-v2-bundle-1"
	}
	_, err = f.service.Store.WriteHandle(DispatchHandle{Schema: DispatchHandleSchema, SchemaVersion: SchemaVersion, DispatchID: request.DispatchID,
		RequestSHA256: digest, Request: ArtifactRef{Kind: "path", Value: path, SHA256: digest},
		Provider: ProviderHandle{Name: "http", AdapterVersion: adapter, RunID: request.DispatchID, Data: json.RawMessage(`{}`)}})
	require.NoError(t, err)
	if accepted {
		response := httptest.NewRecorder()
		f.writeObservation(response, "handle")
		require.NoError(t, d.writeRecord(request.DispatchID, ".accepted.json", json.RawMessage(response.Body.Bytes())))
	}
	return frozen
}
