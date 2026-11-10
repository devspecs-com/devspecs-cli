package orchestration

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPDispatchReceipt_WithTerminalMock_PersistsWithoutMutatingTaskCorpus(t *testing.T) {
	// Arrange
	taskDir := t.TempDir()
	task := filepath.Join(taskDir, "task.json")
	checkpoint := filepath.Join(taskDir, "checkpoint.json")
	require.NoError(t, os.WriteFile(task, []byte(`{"state":"planned"}`), 0o600))
	require.NoError(t, os.WriteFile(checkpoint, []byte(`{"decision":"continue"}`), 0o600))

	checkout := newPortableFixtureCheckout(t)
	frozen, err := buildHTTPDispatchRequest(t.Context(), ExecRunner{}, checkout, portableFixtureSpec())
	require.NoError(t, err)
	handle, status, raw := observePortableMockReceipt(t, frozen, portableFixtureCommit)
	store := StateStore{Root: t.TempDir()}

	// Act
	admitted, err := admitHTTPDispatchReceipt(t.Context(), ExecRunner{}, checkout, frozen, handle, status, raw, nil)
	var published string
	if err == nil {
		published, err = store.writeHTTPDispatchReceipt(admitted)
	}

	// Assert
	require.NoError(t, err)
	assert.Equal(t, httpDispatchDigest(raw), storedHTTPDispatchDigest(t, published))
	assert.NoFileExists(t, store.ReceiptPath(handle.DispatchID))
	taskBytes, err := os.ReadFile(task)
	require.NoError(t, err)
	assert.Equal(t, `{"state":"planned"}`, string(taskBytes))
	checkpointBytes, err := os.ReadFile(checkpoint)
	require.NoError(t, err)
	assert.Equal(t, `{"decision":"continue"}`, string(checkpointBytes))
}

func TestHTTPDispatchReceipt_WithRevertedLifecycleEdit_RejectsResultHistory(t *testing.T) {
	// Arrange
	checkout := newPortableFixtureCheckout(t)
	runner := ExecRunner{}
	frozen, err := buildHTTPDispatchRequest(t.Context(), runner, checkout, portableFixtureSpec())
	require.NoError(t, err)
	const protected = "DevSpecs/tasks/fixture/task.json"
	task := filepath.Join(checkout, filepath.FromSlash(protected))
	require.NoError(t, os.MkdirAll(filepath.Dir(task), 0o700))
	require.NoError(t, os.WriteFile(task, []byte(`{"state":"promoted"}`), 0o600))
	_, err = runGitBytes(t.Context(), runner, checkout, "add", "--", protected)
	require.NoError(t, err)
	commitPortableReceiptFixture(t, checkout, "edit lifecycle")
	_, err = runGitBytes(t.Context(), runner, checkout, "rm", "--", protected)
	require.NoError(t, err)
	commitPortableReceiptFixture(t, checkout, "revert lifecycle edit")
	result, err := runGitText(t.Context(), runner, checkout, "rev-parse", "HEAD")
	require.NoError(t, err)
	handle, status, raw := observePortableMockReceipt(t, frozen, result)

	// Act
	admitted, err := admitHTTPDispatchReceipt(t.Context(), runner, checkout, frozen, handle, status, raw, nil)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchTask)
	assert.Nil(t, admitted)
}

func commitPortableReceiptFixture(t *testing.T, checkout, message string) {
	t.Helper()
	_, err := runGitBytes(t.Context(), ExecRunner{}, checkout, "-c", "commit.gpgsign=false",
		"-c", "core.hooksPath=.git/hooks", "commit", "-m", message)
	require.NoError(t, err)
}

func observePortableMockReceipt(t *testing.T, frozen []byte, result string) (*httpDispatchHandle, *httpDispatchStatus, []byte) {
	t.Helper()
	transport := newPortableReceiptTransport(t, frozen, result)
	handle, err := transport.submit(t.Context(), loadTestHTTPDispatchCaps(t, transport), frozen, nil)
	require.NoError(t, err)
	status, err := transport.status(t.Context(), handle)
	require.NoError(t, err)
	raw, err := transport.receipt(t.Context(), handle)
	require.NoError(t, err)
	return handle, status, raw
}

func newPortableReceiptTransport(t *testing.T, frozen []byte, result string) *httpDispatchTransport {
	t.Helper()
	var receipt map[string]any
	require.NoError(t, json.Unmarshal([]byte(testHTTPDispatchReceipt), &receipt))
	provider := map[string]any{"name": "fixture-provider", "run_id": "run_portable_fixture"}
	digest := httpDispatchDigest(frozen)
	receipt["provider"] = provider
	receipt["request_sha256"] = digest
	receipt["repository"] = map[string]any{
		"logical_id": "git:" + portableFixtureURL, "source_commit": portableFixtureCommit, "result_commit": result,
	}
	receipt["evidence"] = map[string]any{"files_read": []string{}, "files_edited": []string{}, "paths_complete": true}
	receiptBody, err := json.Marshal(receipt)
	require.NoError(t, err)
	statusBody, err := json.Marshal(map[string]any{
		"schema": DispatchStatusSchema, "schema_version": 2, "dispatch_id": testHTTPDispatchID,
		"request_sha256": digest, "provider": provider, "state": "succeeded",
		"cancel_requested": false, "observed_at": "2026-11-10T12:00:05Z",
	})
	require.NoError(t, err)
	return newTestHTTPDispatch(t, withHTTPDispatchCapabilities(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/dispatches":
			acceptPortableFixture(w, r)
		case "/v2/dispatches/" + testHTTPDispatchID:
			writeHTTPDispatchMock(w, http.StatusOK, string(statusBody))
		case "/v2/dispatches/" + testHTTPDispatchID + "/receipt":
			writeHTTPDispatchMock(w, http.StatusOK, string(receiptBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}
