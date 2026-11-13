//go:build devspecs_http_loopback

package orchestration

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPDispatchPilotCLI_WithRotatedCredential_RecoversFrozenRequest(t *testing.T) {
	// Arrange
	ds := requireLoopbackCandidate(t)
	runner, home := newLoopbackCLIRunner(t)
	spec := portableFixtureSpec()
	checkout, frozen, commit := newLoopbackCLIFrozen(t, runner, spec)
	const source = "https://git.example/pilot/fixture.git"
	var wire httpDispatchPortableRequest
	require.NoError(t, json.Unmarshal(frozen, &wire))
	wire.Repository.GitURL, wire.Repository.LogicalID = source, "git:"+source
	frozen, err := json.Marshal(wire)
	require.NoError(t, err)
	handle, status, receipt := headlessDispatchResponseBodies(t, frozen, source, commit)
	const tokenEnv = "DEVSPECS_PILOT_TEST_TOKEN"
	const firstToken, secondToken = "synthetic-pilot-first", "synthetic-pilot-rotated"
	accepted := make(chan struct{})
	var posts, executions atomic.Int32
	const runPath = "/v2/dispatches/" + testHTTPDispatchID
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token != "Bearer "+firstToken && token != "Bearer "+secondToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/capabilities":
			writeHTTPDispatchMock(w, http.StatusOK, testHTTPDispatchCaps)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/dispatches":
			body, err := io.ReadAll(io.LimitReader(r.Body, httpDispatchRequestBody+1))
			if err != nil || !bytes.Equal(body, frozen) || r.Header.Get("Idempotency-Key") != testHTTPDispatchID {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if posts.Add(1) == 1 {
				if token != "Bearer "+firstToken {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				executions.Add(1)
				close(accepted)
				<-r.Context().Done()
				return
			}
			if token != "Bearer "+secondToken {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			writeHTTPDispatchMock(w, http.StatusOK, string(handle))
		case r.Method == http.MethodGet && r.URL.Path == runPath:
			writeHTTPDispatchMock(w, http.StatusOK, string(status))
		case r.Method == http.MethodGet && r.URL.Path == runPath+"/receipt":
			writeHTTPDispatchMock(w, http.StatusOK, string(receipt))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	ca := filepath.Join(home, "pilot-ca.pem")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: server.Certificate().Raw,
	}), 0o600))
	cfg := httpPilotConfig{
		Schema: "devspecs.dispatch.pilot_config", SchemaVersion: 1, Enabled: true,
		Endpoint: server.URL + "/v2", SourceURL: source,
		PrincipalID: spec.Caller.PrincipalID, CredentialID: spec.Caller.CredentialID,
		TokenEnv: tokenEnv, CAFile: ca,
	}
	configPath := filepath.Join(home, "pilot.json")
	configJSON, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, configJSON, 0o600))
	base := append(runner, "DEVSPECS_HTTP_PILOT_CONFIG="+configPath)
	initial := append(append(headlessDispatchRunner(nil), base...), tokenEnv+"="+firstToken)

	// Act
	killLoopbackCLIAfterAdmission(t, initial, home, ds, checkout, spec.Target.Owner, frozen, accepted)
	cfg.CredentialID = "credential_fixture_v2"
	configJSON, err = json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, configJSON, 0o600))
	rotated := append(append(headlessDispatchRunner(nil), base...), tokenEnv+"="+secondToken)
	var result LoopbackResult
	output := rotated.dsJSON(t, home, ds, nil, &result, "dispatch", "resume", testHTTPDispatchID)

	// Assert
	requireHeadlessDispatchSchema(t, "dispatch-loopback-result.v1.schema.json", output)
	assert.Equal(t, "succeeded", result.State)
	assert.Equal(t, httpDispatchDigest(frozen), result.RequestSHA256)
	assert.EqualValues(t, 2, posts.Load())
	assert.EqualValues(t, 1, executions.Load())
	var evidence httpDispatchAdmittedReceipt
	require.NoError(t, json.Unmarshal(result.Receipt, &evidence))
	assert.Equal(t, commit, evidence.SourceCommit)
	require.NotNil(t, evidence.ResultCommit)
	assert.Equal(t, commit, *evidence.ResultCommit)
	assert.Equal(t, "credential_fixture_v1", evidence.Caller.CredentialID)
	prefix := filepath.Join(home, httpDispatchReceiptDir, testHTTPDispatchID)
	persisted := readHeadlessDispatchFile(t, prefix+".pending.json") +
		readHeadlessDispatchFile(t, prefix+".handle.json") +
		readHeadlessDispatchFile(t, prefix+".receipt.json")
	assert.NotContains(t, persisted, firstToken)
	assert.NotContains(t, persisted, secondToken)
}

func TestHTTPDispatchPilotCLI_WithDisabledConfig_DoesNotUseCredentials(t *testing.T) {
	// Arrange
	ds := requireLoopbackCandidate(t)
	runner, home := newLoopbackCLIRunner(t)
	configPath := filepath.Join(home, "disabled.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"schema":"devspecs.dispatch.pilot_config","schema_version":1,"enabled":false}`), 0o600))
	runner = append(runner, "DEVSPECS_HTTP_PILOT_CONFIG="+configPath)

	// Act
	result, err := runner.run(t.Context(), home, ds, nil, "dispatch", "status", testHTTPDispatchID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, result.ExitCode)
	var output LoopbackResult
	require.NoError(t, json.Unmarshal(result.Stdout, &output))
	assert.Equal(t, "invalid_configuration", output.Error)
	_, err = os.Lstat(filepath.Join(home, httpDispatchReceiptDir))
	assert.ErrorIs(t, err, os.ErrNotExist)
}
