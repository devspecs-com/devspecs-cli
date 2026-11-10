//go:build devspecs_http_loopback

package orchestration

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const loopbackSource = "https://git.example.invalid/synthetic/portable.git"

func TestHTTPDispatchLoopbackCLI_AfterCallerKilled_ResumesWithoutDuplicateExecution(t *testing.T) {
	// Arrange
	ds := requireLoopbackCandidate(t)
	runner, home := newLoopbackCLIRunner(t)
	repo := t.TempDir()
	_, err := runGitBytes(t.Context(), runner, repo, "init", "--template=")
	require.NoError(t, err)
	var task struct {
		ID       string `json:"task_id"`
		Manifest string `json:"manifest_path"`
		Plan     string `json:"first_slice_path"`
		Result   string `json:"result_path"`
	}
	runner.dsJSON(t, repo, ds, nil, &task, "task", "Inspect synthetic fixture", "--id", headlessDispatchTask,
		"--series", "G", "--slice", "Inspect fixture", "--no-refresh", "--index=false", "--json")
	require.NoError(t, os.WriteFile(task.Plan, []byte(headlessDispatchPlan), 0o600))
	manifestBefore := readHeadlessDispatchFile(t, task.Manifest)
	resultBefore := readHeadlessDispatchFile(t, task.Result)
	var apply struct {
		Target  string `json:"target"`
		Context struct {
			Plan string `json:"plan_body"`
		} `json:"target_context"`
	}
	applyJSON := runner.dsJSON(t, repo, ds, nil, &apply, "apply", task.ID, "--target", headlessDispatchTarget, "--json")
	requireHeadlessDispatchSchema(t, "apply-output.v1.schema.json", applyJSON)
	require.Equal(t, headlessDispatchPlan, apply.Context.Plan)
	owner := "task:" + task.ID
	spec := portableFixtureSpec()
	spec.Target = Target{Owner: owner, Slice: apply.Target}
	spec.Artifacts = []httpDispatchInlineInput{{ID: "reviewed-plan", MediaType: "text/markdown", Content: []byte(apply.Context.Plan), SHA256: httpDispatchDigest([]byte(apply.Context.Plan))}}
	checkout, frozen, commit := newLoopbackCLIFrozen(t, runner, spec)
	handle, status, receipt := headlessDispatchResponseBodies(t, frozen, loopbackSource, commit)
	var posts, executions atomic.Int32
	accepted := make(chan struct{})
	const runPath = "/v2/dispatches/" + testHTTPDispatchID
	runner = withLoopbackCLIPeer(t, runner, withHTTPDispatchCapabilities(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/dispatches":
			body, err := io.ReadAll(io.LimitReader(r.Body, httpDispatchRequestBody+1))
			if err != nil || !bytes.Equal(body, frozen) || r.Header.Get("Idempotency-Key") != testHTTPDispatchID ||
				r.Header.Get("Authorization") != "Bearer "+testHTTPDispatchToken {
				writeHTTPDispatchMock(w, http.StatusBadRequest, testHTTPDispatchError("invalid_request", false))
				return
			}
			if posts.Add(1) == 1 {
				executions.Add(1)
				close(accepted)
				<-r.Context().Done()
				return
			}
			writeHTTPDispatchMock(w, http.StatusOK, string(handle))
		case r.Method == http.MethodGet && r.URL.Path == runPath:
			writeHTTPDispatchMock(w, http.StatusOK, string(status))
		case r.Method == http.MethodGet && r.URL.Path == runPath+"/receipt":
			writeHTTPDispatchMock(w, http.StatusOK, string(receipt))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	// Act
	killLoopbackCLIAfterAdmission(t, runner, repo, ds, checkout, owner, frozen, accepted)
	var pending LoopbackResult
	runner.dsJSON(t, repo, ds, nil, &pending, "dispatch", "status", testHTTPDispatchID)
	require.Equal(t, "pending", pending.State)
	require.EqualValues(t, 1, posts.Load(), "status must not submit a request")
	var resumed LoopbackResult
	raw := runner.dsJSON(t, repo, ds, nil, &resumed, "dispatch", "resume", testHTTPDispatchID)
	requireHeadlessDispatchSchema(t, "dispatch-loopback-result.v1.schema.json", raw)

	// Assert the recovery outcome before the caller grants write authority.
	require.Equal(t, "succeeded", resumed.State)
	assert.EqualValues(t, 2, posts.Load())
	assert.EqualValues(t, 1, executions.Load())
	require.Equal(t, httpDispatchDigest(frozen), resumed.RequestSHA256)
	var evidence httpDispatchAdmittedReceipt
	require.NoError(t, json.Unmarshal(resumed.Receipt, &evidence))
	require.Equal(t, testHTTPDispatchID, evidence.DispatchID)
	require.Equal(t, httpDispatchDigest(receipt), evidence.WireSHA256)
	var stored LoopbackResult
	runner.dsJSON(t, repo, ds, nil, &stored, "dispatch", "receipt", testHTTPDispatchID)
	assert.JSONEq(t, string(resumed.Receipt), string(stored.Receipt))
	assert.Equal(t, manifestBefore, readHeadlessDispatchFile(t, task.Manifest))
	assert.Equal(t, resultBefore, readHeadlessDispatchFile(t, task.Result))
	_, err = os.Lstat(filepath.Join(filepath.Dir(task.Manifest), "checkpoints"))
	require.ErrorIs(t, err, os.ErrNotExist)
	pendingFile := filepath.Join(home, httpDispatchReceiptDir, testHTTPDispatchID+".pending.json")
	assert.NotContains(t, readHeadlessDispatchFile(t, pendingFile), testHTTPDispatchToken)

	input := loopbackCheckpointInput(t, task.ID, apply.Target, evidence)
	args := []string{"task", "checkpoint", task.ID, "--input-json", "--json", "--index=false"}
	var draft struct {
		Draft   bool `json:"draft"`
		Mutates bool `json:"mutates"`
	}
	draftJSON := runner.dsJSON(t, repo, ds, input, &draft, append(append([]string(nil), args...), "--draft")...)
	requireHeadlessDispatchSchema(t, "task-checkpoint-draft.v1.schema.json", draftJSON)
	require.True(t, draft.Draft)
	require.False(t, draft.Mutates)
	_, err = os.Lstat(filepath.Join(filepath.Dir(task.Manifest), "checkpoints"))
	require.ErrorIs(t, err, os.ErrNotExist)
	var checkpoint struct {
		Path string `json:"checkpoint_json_path"`
	}
	output := runner.dsJSON(t, repo, ds, input, &checkpoint, args...)
	requireHeadlessDispatchSchema(t, "task-checkpoint-output.v1.schema.json", output)
	beforeReplay := readHeadlessDispatchFile(t, checkpoint.Path)
	failed, err := runner.run(t.Context(), repo, ds, input, args...)
	require.NoError(t, err)
	assert.Equal(t, 1, failed.ExitCode)
	requireHeadlessDispatchSchema(t, "task-checkpoint-error.v1.schema.json", failed.Stdout)
	var stale struct{ Error string }
	require.NoError(t, json.Unmarshal(failed.Stdout, &stale))
	assert.Equal(t, "stale_heads", stale.Error)
	assert.Equal(t, beforeReplay, readHeadlessDispatchFile(t, checkpoint.Path))
}

func TestHTTPDispatchLoopbackCLI_WithStoredHandle_ResumesWithoutPosting(t *testing.T) {
	// Arrange
	ds := requireLoopbackCandidate(t)
	runner, home := newLoopbackCLIRunner(t)
	spec := portableFixtureSpec()
	checkout, frozen, commit := newLoopbackCLIFrozen(t, runner, spec)
	handle, status, receipt := headlessDispatchResponseBodies(t, frozen, loopbackSource, commit)
	var posts atomic.Int32
	const runPath = "/v2/dispatches/" + testHTTPDispatchID
	runner = withLoopbackCLIPeer(t, runner, withHTTPDispatchCapabilities(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/dispatches":
			posts.Add(1)
			writeHTTPDispatchMock(w, http.StatusCreated, string(handle))
		case r.Method == http.MethodGet && r.URL.Path == runPath:
			writeHTTPDispatchMock(w, http.StatusOK, string(status))
		case r.Method == http.MethodGet && r.URL.Path == runPath+"/receipt":
			writeHTTPDispatchMock(w, http.StatusOK, string(receipt))
		default:
			http.NotFound(w, r)
		}
	}))
	var detached LoopbackResult
	start := runner.dsJSON(t, home, ds, frozen, &detached, "dispatch", spec.Target.Owner, "--cwd", checkout, "--detach")
	requireHeadlessDispatchSchema(t, "dispatch-loopback-result.v1.schema.json", start)
	require.Equal(t, "accepted", detached.State)
	require.EqualValues(t, 1, posts.Load())

	// Act
	var resumed LoopbackResult
	output := runner.dsJSON(t, home, ds, nil, &resumed, "dispatch", "resume", detached.ID)

	// Assert
	requireHeadlessDispatchSchema(t, "dispatch-loopback-result.v1.schema.json", output)
	assert.Equal(t, "succeeded", resumed.State)
	assert.NotEmpty(t, resumed.Receipt)
	assert.EqualValues(t, 1, posts.Load())
}

func TestHTTPDispatchLoopbackCLI_WithWrongProviderSchema_ReportsErrorWithoutPosting(t *testing.T) {
	// Arrange
	ds := requireLoopbackCandidate(t)
	runner, home := newLoopbackCLIRunner(t)
	spec := portableFixtureSpec()
	checkout, frozen, _ := newLoopbackCLIFrozen(t, runner, spec)
	var capabilities map[string]any
	require.NoError(t, json.Unmarshal([]byte(testHTTPDispatchCaps), &capabilities))
	capabilities["schema_version"] = 99
	incompatible, err := json.Marshal(capabilities)
	require.NoError(t, err)
	var posts atomic.Int32
	runner = withLoopbackCLIPeer(t, runner, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v2/capabilities" {
			writeHTTPDispatchMock(w, http.StatusOK, string(incompatible))
			return
		}
		if r.Method == http.MethodPost {
			posts.Add(1)
		}
		http.NotFound(w, r)
	}))

	// Act
	result, err := runner.run(t.Context(), home, ds, frozen, "dispatch", spec.Target.Owner, "--cwd", checkout, "--detach")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	requireHeadlessDispatchSchema(t, "dispatch-loopback-result.v1.schema.json", result.Stdout)
	var output LoopbackResult
	require.NoError(t, json.Unmarshal(result.Stdout, &output))
	assert.Equal(t, "incompatible_provider", output.Error)
	assert.Zero(t, posts.Load())
	_, err = os.Lstat(filepath.Join(home, httpDispatchReceiptDir))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func requireLoopbackCandidate(t *testing.T) string {
	t.Helper()
	ds := os.Getenv("DEVSPECS_CONFORMANCE_DS")
	if ds == "" {
		t.Skip("requires an installed build-tagged ds candidate")
	}
	require.True(t, filepath.IsAbs(ds))
	return ds
}

func newLoopbackCLIRunner(t *testing.T) (headlessDispatchRunner, string) {
	t.Helper()
	home := t.TempDir()
	return headlessDispatchRunner{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home,
		"APPDATA=" + home, "LOCALAPPDATA=" + home, "TMPDIR=" + home, "TEMP=" + home, "TMP=" + home,
		"DEVSPECS_HOME=" + home, "DEVSPECS_TELEMETRY=0",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + filepath.Join(home, "gitconfig"),
		"GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL=",
	}, home
}

func newLoopbackCLIFrozen(t *testing.T, runner headlessDispatchRunner, spec httpDispatchBuildSpec) (string, []byte, string) {
	t.Helper()
	checkout := t.TempDir()
	ctx := t.Context()
	_, err := runGitBytes(ctx, runner, checkout, "init", "--template=")
	require.NoError(t, err)
	_, err = runGitBytes(ctx, runner, checkout, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
		"commit", "--allow-empty", "-m", "Synthetic source")
	require.NoError(t, err)
	commit, err := runGitText(ctx, runner, checkout, "rev-parse", "HEAD")
	require.NoError(t, err)
	spec.Source = &httpDispatchConfirmedSource{URL: loopbackSource, Commit: commit}
	spec.Verification = nil
	frozen, err := buildHTTPDispatchRequest(ctx, runner, checkout, spec)
	require.NoError(t, err)
	return checkout, frozen, commit
}

func withLoopbackCLIPeer(t *testing.T, runner headlessDispatchRunner, handler http.Handler) headlessDispatchRunner {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	trust := filepath.Join(t.TempDir(), "mock-ca.pem")
	require.NoError(t, os.WriteFile(trust, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600))
	return append(runner, "DEVSPECS_HTTP_LOOPBACK_ENDPOINT="+server.URL+"/v2", "DEVSPECS_HTTP_LOOPBACK_CA="+trust)
}

func killLoopbackCLIAfterAdmission(t *testing.T, env headlessDispatchRunner, repo, ds, checkout, owner string, frozen []byte, accepted <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ds, "dispatch", owner, "--cwd", checkout, "--detach")
	cmd.Dir, cmd.Env, cmd.Stdin = repo, env, bytes.NewReader(frozen)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(t, cmd.Start())
	select {
	case <-accepted:
		err := cmd.Process.Kill()
		waitErr := cmd.Wait()
		require.NoError(t, err)
		require.Error(t, waitErr)
	case <-ctx.Done():
		_ = cmd.Wait()
		require.FailNowf(t, "candidate never reached the mock", "stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

func loopbackCheckpointInput(t *testing.T, task, target string, receipt httpDispatchAdmittedReceipt) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"schema": "devspecs.task.checkpoint_input", "schema_version": 1, "task_id": task,
		"target": target, "stage": "validated", "decision": "improve", "expected_heads": []string{},
		"receipt_evidence": map[string]any{
			"dispatch_id": receipt.DispatchID, "receipt_id": receipt.ReceiptID,
			"request_sha256": receipt.RequestSHA256, "wire_sha256": receipt.WireSHA256, "result_commit": receipt.ResultCommit,
		},
	})
	require.NoError(t, err)
	return data
}
