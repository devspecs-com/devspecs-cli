package orchestration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	headlessDispatchTask   = "headless-dispatch"
	headlessDispatchTarget = "G01"
	headlessDispatchPlan   = "# Synthetic plan\n\nInspect <fixture> without editing it.\n\n"
)

func TestHeadlessHTTPDispatchConformance(t *testing.T) {
	// Arrange
	ds, ok := os.LookupEnv("DEVSPECS_CONFORMANCE_DS")
	if !ok {
		t.Skip("DEVSPECS_CONFORMANCE_DS is unset; an installed ds binary is required")
	}
	require.True(t, filepath.IsAbs(ds), "DEVSPECS_CONFORMANCE_DS must be an absolute binary path")
	home, repo, checkout := t.TempDir(), t.TempDir(), t.TempDir()
	runner := headlessDispatchRunner{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home,
		"APPDATA=" + home, "LOCALAPPDATA=" + home, "TMPDIR=" + home, "TEMP=" + home, "TMP=" + home,
		"DEVSPECS_HOME=" + home, "DEVSPECS_TELEMETRY=0",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + filepath.Join(home, "gitconfig"),
		"GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL=",
	}
	ctx := t.Context()
	_, err := runGitBytes(ctx, runner, repo, "init", "--template=")
	require.NoError(t, err)
	_, err = runGitBytes(ctx, runner, checkout, "init", "--template=")
	require.NoError(t, err)
	_, err = runGitBytes(ctx, runner, checkout, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
		"commit", "--allow-empty", "-m", "Synthetic source")
	require.NoError(t, err)
	commit, err := runGitText(ctx, runner, checkout, "rev-parse", "HEAD")
	require.NoError(t, err)

	var task struct {
		TaskID       string `json:"task_id"`
		Workspace    string `json:"workspace"`
		ManifestPath string `json:"manifest_path"`
		IndexPath    string `json:"index_path"`
		PlanPath     string `json:"first_slice_path"`
		ResultPath   string `json:"result_path"`
	}
	runner.dsJSON(t, repo, ds, nil, &task, "task", "Inspect synthetic fixture", "--id", headlessDispatchTask,
		"--series", "G", "--slice", "Inspect fixture", "--no-refresh", "--index=false", "--json")
	require.Equal(t, headlessDispatchTask, task.TaskID)
	require.Equal(t, filepath.Join(repo, "devspecs", "tasks", headlessDispatchTask), task.Workspace)
	require.Equal(t, filepath.Join(task.Workspace, "G01-inspect-fixture-plan.md"), task.PlanPath)
	require.NoError(t, os.WriteFile(task.PlanPath, []byte(headlessDispatchPlan), 0o600))
	checkpointDir := filepath.Join(task.Workspace, "checkpoints")
	manifestBefore := readHeadlessDispatchFile(t, task.ManifestPath)
	indexBefore := readHeadlessDispatchFile(t, task.IndexPath)
	resultBefore := readHeadlessDispatchFile(t, task.ResultPath)
	_, err = os.Lstat(checkpointDir)
	require.ErrorIs(t, err, os.ErrNotExist)

	// Act
	var apply struct {
		TaskID        string `json:"task_id"`
		Target        string `json:"target"`
		TargetContext struct {
			PlanBody string `json:"plan_body"`
		} `json:"target_context"`
		Budget struct {
			Version   int `json:"schema_version"`
			Artifacts []struct {
				Pointer string `json:"json_pointer"`
				Size    int    `json:"size_bytes"`
				SHA256  string `json:"sha256"`
			} `json:"context_artifacts"`
		} `json:"input_budget"`
	}
	applyJSON := runner.dsJSON(t, repo, ds, nil, &apply, "apply", task.TaskID, "--target", headlessDispatchTarget, "--json")
	requireHeadlessDispatchSchema(t, "apply-output.v1.schema.json", applyJSON)
	require.Equal(t, task.TaskID, apply.TaskID)
	require.Equal(t, headlessDispatchTarget, apply.Target)
	require.Equal(t, headlessDispatchPlan, apply.TargetContext.PlanBody)
	require.Equal(t, 1, apply.Budget.Version)
	require.Len(t, apply.Budget.Artifacts, 1)
	plan, budget := []byte(apply.TargetContext.PlanBody), apply.Budget.Artifacts[0]
	require.Equal(t, "/target_context/plan_body", budget.Pointer)
	require.Equal(t, len(plan), budget.Size)
	require.Equal(t, httpDispatchDigest(plan), budget.SHA256)

	// The local apply JSON and its prompt are not approved for transport.
	spec := portableFixtureSpec()
	spec.Target = Target{Owner: "task:" + apply.TaskID, Slice: apply.Target}
	spec.Source.Commit = commit
	spec.Verification = nil
	spec.Artifacts = []httpDispatchInlineInput{{ID: "reviewed-plan", MediaType: "text/markdown", Content: plan, SHA256: budget.SHA256}}
	frozen, err := buildHTTPDispatchRequest(ctx, runner, checkout, spec)
	require.NoError(t, err)
	var wire httpDispatchPortableRequest
	require.NoError(t, json.Unmarshal(frozen, &wire))
	var prompt httpDispatchPortableApply
	require.NoError(t, json.Unmarshal([]byte(wire.Input.ApplyPayload), &prompt))
	require.Equal(t, headlessDispatchTarget, prompt.Target)
	require.Equal(t, portableFixturePrompt, prompt.Prompt)
	require.Len(t, wire.Input.ContextArtifacts, 1)
	require.True(t, matchesPortableFixtureArtifact(wire.Input.ContextArtifacts[0], "reviewed-plan", "text/markdown", headlessDispatchPlan))
	transport := newHeadlessDispatchMock(t, frozen, commit)
	handle, err := transport.submit(ctx, loadTestHTTPDispatchCaps(t, transport), frozen, nil)
	require.NoError(t, err)
	status, err := transport.status(ctx, handle)
	require.NoError(t, err)
	require.Equal(t, "succeeded", status.State)
	raw, err := transport.receipt(ctx, handle)
	require.NoError(t, err)
	admitted, err := admitHTTPDispatchReceipt(ctx, runner, checkout, frozen, handle, status, raw, nil)
	require.NoError(t, err)
	store := StateStore{Root: home}
	storedPath, err := store.writeHTTPDispatchReceipt(admitted)
	require.NoError(t, err)
	storedBefore := readHeadlessDispatchFile(t, storedPath)
	var stored struct {
		Schema  string `json:"schema"`
		Version int    `json:"schema_version"`
		httpDispatchAdmittedReceipt
	}
	require.NoError(t, json.Unmarshal([]byte(storedBefore), &stored))
	require.Equal(t, httpDispatchReceiptSchema, stored.Schema)
	require.Equal(t, 1, stored.Version)
	require.Equal(t, "succeeded", stored.State)
	require.Equal(t, testHTTPDispatchID, stored.DispatchID)
	require.Equal(t, "receipt_01KZZW1KXP4AZ7TSB0NN68HJ86", stored.ReceiptID)
	require.Equal(t, httpDispatchDigest(frozen), stored.RequestSHA256)
	require.Equal(t, httpDispatchDigest(raw), stored.WireSHA256)
	require.NotNil(t, stored.ResultCommit)
	require.Equal(t, commit, *stored.ResultCommit)

	// Establish the no-write boundary before giving the caller permission to append.
	require.Equal(t, manifestBefore, readHeadlessDispatchFile(t, task.ManifestPath))
	require.Equal(t, indexBefore, readHeadlessDispatchFile(t, task.IndexPath))
	require.Equal(t, headlessDispatchPlan, readHeadlessDispatchFile(t, task.PlanPath))
	require.Equal(t, resultBefore, readHeadlessDispatchFile(t, task.ResultPath))
	_, err = os.Lstat(checkpointDir)
	require.ErrorIs(t, err, os.ErrNotExist)

	const decision, note = "improve", "Caller reviewed the mock receipt; do not promote."
	evidence := map[string]string{
		"dispatch_id": stored.DispatchID, "receipt_id": stored.ReceiptID, "request_sha256": stored.RequestSHA256,
		"wire_sha256": stored.WireSHA256, "result_commit": *stored.ResultCommit,
	}
	input, err := json.Marshal(map[string]any{
		"schema": "devspecs.task.checkpoint_input", "schema_version": 1, "task_id": task.TaskID,
		"target": apply.Target, "stage": "validated", "decision": decision, "note": note,
		"expected_heads": []string{}, "receipt_evidence": evidence,
	})
	require.NoError(t, err)
	args := []string{"task", "checkpoint", task.TaskID, "--input-json", "--json", "--index=false"}
	var output struct {
		ID       string `json:"checkpoint_id"`
		JSONPath string `json:"checkpoint_json_path"`
		Markdown string `json:"checkpoint_path"`
	}
	checkpointJSON := runner.dsJSON(t, repo, ds, input, &output, args...)
	requireHeadlessDispatchSchema(t, "task-checkpoint-output.v1.schema.json", checkpointJSON)
	require.Equal(t, checkpointDir, filepath.Dir(output.JSONPath))
	require.Equal(t, checkpointDir, filepath.Dir(output.Markdown))
	eventBefore := readHeadlessDispatchFile(t, output.JSONPath)
	markdownBefore := readHeadlessDispatchFile(t, output.Markdown)
	manifestAfter := readHeadlessDispatchFile(t, task.ManifestPath)
	resultAfter := readHeadlessDispatchFile(t, task.ResultPath)
	replay, err := runner.run(ctx, repo, ds, input, args...)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, replay.ExitCode)
	entries, err := os.ReadDir(checkpointDir)
	require.NoError(t, err)
	require.Len(t, entries, 2, "one JSON event and its Markdown projection")
	assert.Equal(t, ".json", filepath.Ext(output.JSONPath))
	assert.Equal(t, ".md", filepath.Ext(output.Markdown))
	assert.Equal(t, eventBefore, readHeadlessDispatchFile(t, output.JSONPath))
	assert.Equal(t, markdownBefore, readHeadlessDispatchFile(t, output.Markdown))
	assert.Equal(t, manifestAfter, readHeadlessDispatchFile(t, task.ManifestPath))
	assert.Equal(t, resultAfter, readHeadlessDispatchFile(t, task.ResultPath))
	assert.Equal(t, storedBefore, readHeadlessDispatchFile(t, storedPath))

	var event struct {
		Version   int      `json:"schema_version"`
		Kind      string   `json:"event_kind"`
		ID        string   `json:"checkpoint_id"`
		TaskID    string   `json:"task_id"`
		Target    string   `json:"target"`
		Stage     string   `json:"stage"`
		Decision  string   `json:"decision"`
		Note      string   `json:"note"`
		Resources []string `json:"resources"`
	}
	require.NoError(t, json.Unmarshal([]byte(eventBefore), &event))
	assert.Equal(t, 3, event.Version)
	assert.Equal(t, "task_checkpoint", event.Kind)
	require.NotEmpty(t, output.ID)
	assert.Equal(t, output.ID, event.ID)
	assert.Equal(t, task.TaskID, event.TaskID)
	assert.Equal(t, apply.Target, event.Target)
	assert.Equal(t, "validated", event.Stage)
	assert.Equal(t, decision, event.Decision)
	assert.Equal(t, note, event.Note)
	const prefix = "devspecs.dispatch.receipt_evidence:"
	i := slices.IndexFunc(event.Resources, func(s string) bool { return strings.HasPrefix(s, prefix) })
	require.NotEqual(t, -1, i)
	var correlated map[string]string
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(event.Resources[i], prefix)), &correlated))
	assert.Equal(t, evidence, correlated)
}

func newHeadlessDispatchMock(t *testing.T, frozen []byte, commit string) *httpDispatchTransport {
	t.Helper()
	handle, status, receipt := headlessDispatchResponseBodies(t, frozen, portableFixtureURL, commit)
	const runPath = "/v2/dispatches/" + testHTTPDispatchID
	return newTestHTTPDispatch(t, withHTTPDispatchCapabilities(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/dispatches":
			body, err := io.ReadAll(io.LimitReader(r.Body, httpDispatchRequestBody+1))
			if err != nil || !bytes.Equal(body, frozen) || r.Header.Get("Idempotency-Key") != testHTTPDispatchID {
				writeHTTPDispatchMock(w, http.StatusBadRequest, testHTTPDispatchError("invalid_request", false))
				return
			}
			writeHTTPDispatchMock(w, http.StatusCreated, string(handle))
		case r.Method == http.MethodGet && r.URL.Path == runPath:
			writeHTTPDispatchMock(w, http.StatusOK, string(status))
		case r.Method == http.MethodGet && r.URL.Path == runPath+"/receipt":
			writeHTTPDispatchMock(w, http.StatusOK, string(receipt))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func headlessDispatchResponseBodies(t *testing.T, frozen []byte, sourceURL, commit string) ([]byte, []byte, []byte) {
	t.Helper()
	digest := httpDispatchDigest(frozen)
	var receipt map[string]any
	require.NoError(t, json.Unmarshal([]byte(testHTTPDispatchReceipt), &receipt))
	receipt["request_sha256"] = digest
	receipt["repository"] = map[string]string{
		"logical_id": "git:" + sourceURL, "source_commit": commit, "result_commit": commit,
	}
	receipt["evidence"] = map[string]any{"files_read": []string{}, "files_edited": []string{}, "paths_complete": true}
	receiptBody, err := json.Marshal(receipt)
	require.NoError(t, err)
	handleBody, err := json.Marshal(map[string]any{
		"schema": DispatchHandleSchema, "schema_version": 2, "dispatch_id": testHTTPDispatchID,
		"request_sha256": digest, "provider": receipt["provider"], "accepted_at": "2026-11-10T12:00:00Z",
	})
	require.NoError(t, err)
	statusBody, err := json.Marshal(map[string]any{
		"schema": DispatchStatusSchema, "schema_version": 2, "dispatch_id": testHTTPDispatchID,
		"request_sha256": digest, "provider": receipt["provider"], "state": "succeeded",
		"cancel_requested": false, "observed_at": "2026-11-10T12:00:05Z",
	})
	require.NoError(t, err)
	return handleBody, statusBody, receiptBody
}

// Keep the CLI and Git off the user's config, credential helpers, and index.
type headlessDispatchRunner []string

func (env headlessDispatchRunner) Run(ctx context.Context, dir, executable string, args ...string) (CommandResult, error) {
	return env.run(ctx, dir, executable, nil, args...)
}

func (env headlessDispatchRunner) run(ctx context.Context, dir, executable string, stdin []byte, args ...string) (CommandResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir, cmd.Env, cmd.Stdin = dir, env, bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	result := CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if exit, ok := err.(*exec.ExitError); ok {
		result.ExitCode, err = exit.ExitCode(), nil
	}
	return result, err
}

func (env headlessDispatchRunner) dsJSON(t *testing.T, dir, ds string, stdin []byte, out any, args ...string) []byte {
	t.Helper()
	result, err := env.run(t.Context(), dir, ds, stdin, args...)
	require.NoError(t, err)
	require.Equal(t, 0, result.ExitCode, "ds %s: %s", args[0], result.Stderr)
	require.NoError(t, json.Unmarshal(result.Stdout, out), "ds %s stdout must be JSON", args[0])
	return result.Stdout
}

func requireHeadlessDispatchSchema(t *testing.T, filename string, raw []byte) {
	t.Helper()
	dir := os.Getenv("DEVSPECS_CONFORMANCE_SPECS")
	if dir == "" {
		dir = "../../specs"
	}
	var document, instance any
	require.NoError(t, json.Unmarshal([]byte(readHeadlessDispatchFile(t, filepath.Join(dir, filename))), &document))
	require.NoError(t, json.Unmarshal(raw, &instance))
	const resource = "urn:devspecs:headless-success"
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource(resource, document))
	schema, err := compiler.Compile(resource)
	require.NoError(t, err)
	require.NoError(t, schema.Validate(instance))
}

func readHeadlessDispatchFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(body)
}
