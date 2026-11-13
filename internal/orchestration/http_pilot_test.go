//go:build devspecs_http_loopback

package orchestration

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pilotTestSource   = "https://git.example.com/synthetic/pilot.git"
	pilotTestTokenEnv = "DEVSPECS_TEST_PILOT_TOKEN"
	pilotTestToken    = "synthetic-pilot-token-initial"
	pilotTestRotated  = "synthetic-pilot-token-rotated"
)

func TestNewPilotDispatch_WithExplicitTrust_ConstructsWithoutNetwork(t *testing.T) {
	// Arrange
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	var requests atomic.Int32
	_, config := newPilotTestPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	home := t.TempDir()
	configPath := writePilotTestConfig(t, config)

	// Act
	service, err := NewPilotDispatch(home, configPath)

	// Assert
	require.NoError(t, err)
	require.NotNil(t, service)
	assert.Zero(t, requests.Load())
	assert.NoDirExists(t, filepath.Join(home, httpDispatchReceiptDir))
}

func TestNewPilotDispatch_WithoutOptionalCA_AcceptsDNSConfiguration(t *testing.T) {
	// Arrange
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	configPath := writePilotTestConfig(t, pilotTestConfig())

	// Act
	service, err := NewPilotDispatch(t.TempDir(), configPath)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, service)
}

func TestNewPilotDispatch_WithDisabledConfig_RejectsOptOut(t *testing.T) {
	// Arrange
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	config := pilotTestConfig()
	config["enabled"] = false
	configPath := writePilotTestConfig(t, config)

	// Act
	service, err := NewPilotDispatch(t.TempDir(), configPath)

	// Assert
	require.Error(t, err)
	assert.Nil(t, service)
	assert.NotContains(t, err.Error(), pilotTestToken)
}

func TestNewPilotDispatch_WithMissingReferencedToken_DoesNotUseAmbientCredentials(t *testing.T) {
	// Arrange
	t.Setenv(pilotTestTokenEnv, "")
	require.NoError(t, os.Unsetenv(pilotTestTokenEnv))
	t.Setenv("DEVSPECS_TOKEN", pilotTestToken)
	t.Setenv("DEVSPECS_HTTP_LOOPBACK_CREDENTIAL", "initial")
	configPath := writePilotTestConfig(t, pilotTestConfig())

	// Act
	service, err := NewPilotDispatch(t.TempDir(), configPath)

	// Assert
	require.Error(t, err)
	assert.Nil(t, service)
	assert.NotContains(t, err.Error(), pilotTestToken)
}

func TestNewPilotDispatch_WithUnknownConfigKey_RejectsConfiguration(t *testing.T) {
	// Arrange
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	config := pilotTestConfig()
	config["allow_writes"] = true
	configPath := writePilotTestConfig(t, config)

	// Act
	service, err := NewPilotDispatch(t.TempDir(), configPath)

	// Assert
	require.Error(t, err)
	assert.Nil(t, service)
}

func TestNewPilotDispatch_WithRawTokenConfig_RejectsWithoutLeakingSecret(t *testing.T) {
	// Arrange
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	config := pilotTestConfig()
	config["token"] = pilotTestRotated
	configPath := writePilotTestConfig(t, config)

	// Act
	service, err := NewPilotDispatch(t.TempDir(), configPath)

	// Assert
	require.Error(t, err)
	assert.Nil(t, service)
	assert.NotContains(t, err.Error(), pilotTestToken)
	assert.NotContains(t, err.Error(), pilotTestRotated)
}

func TestNewPilotDispatch_WithUnknownSchema_RejectsConfiguration(t *testing.T) {
	// Arrange
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	config := pilotTestConfig()
	config["schema"] = "devspecs.dispatch.not_pilot_config"
	configPath := writePilotTestConfig(t, config)

	// Act
	service, err := NewPilotDispatch(t.TempDir(), configPath)

	// Assert
	require.Error(t, err)
	assert.Nil(t, service)
}

func TestNewPilotDispatch_WithUnknownVersion_RejectsConfiguration(t *testing.T) {
	// Arrange
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	config := pilotTestConfig()
	config["schema_version"] = 2
	configPath := writePilotTestConfig(t, config)

	// Act
	service, err := NewPilotDispatch(t.TempDir(), configPath)

	// Assert
	require.Error(t, err)
	assert.Nil(t, service)
}

func TestNewPilotDispatch_WithRelativeConfigPath_RejectsConfiguration(t *testing.T) {
	// Arrange
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	absPath := writePilotTestConfig(t, pilotTestConfig())
	t.Chdir(filepath.Dir(absPath))
	configPath := filepath.Base(absPath)
	require.False(t, filepath.IsAbs(configPath))

	// Act
	service, err := NewPilotDispatch(t.TempDir(), configPath)

	// Assert
	require.Error(t, err)
	assert.Nil(t, service)
}

func TestNewPilotDispatch_WithMissingConfigFile_RejectsConfiguration(t *testing.T) {
	// Arrange
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	configPath := filepath.Join(t.TempDir(), "missing-pilot.json")

	// Act
	service, err := NewPilotDispatch(t.TempDir(), configPath)

	// Assert
	require.Error(t, err)
	assert.Nil(t, service)
}

func TestNewPilotDispatch_WithConfigDirectory_RejectsNonRegularFile(t *testing.T) {
	// Arrange
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	configPath := t.TempDir()

	// Act
	service, err := NewPilotDispatch(t.TempDir(), configPath)

	// Assert
	require.Error(t, err)
	assert.Nil(t, service)
}

func TestNewPilotDispatch_WithOversizedValidJSON_RejectsBoundedFile(t *testing.T) {
	// Arrange
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	configPath := writePilotTestConfig(t, pilotTestConfig())
	raw := readHeadlessDispatchFile(t, configPath)
	padded := raw + strings.Repeat(" ", httpDispatchSmallBody+1-len(raw))
	require.True(t, json.Valid([]byte(padded)))
	require.NoError(t, os.WriteFile(configPath, []byte(padded), 0o600))

	// Act
	service, err := NewPilotDispatch(t.TempDir(), configPath)

	// Assert
	require.Error(t, err)
	assert.Nil(t, service)
}

func TestNewPilotDispatch_WithPlainHTTPEndpoint_RejectsConfiguration(t *testing.T) {
	// Arrange
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	config := pilotTestConfig()
	config["endpoint"] = "http://example.com/v2"
	configPath := writePilotTestConfig(t, config)

	// Act
	service, err := NewPilotDispatch(t.TempDir(), configPath)

	// Assert
	require.Error(t, err)
	assert.Nil(t, service)
}

func TestPilotDispatchStart_WithConfiguredDNSAndToken_AuthenticatesAndAdmitsReceipt(t *testing.T) {
	// Arrange
	home, checkout, frozen, commit := newPilotTestRequest(t)
	handle, status, receipt := headlessDispatchResponseBodies(t, frozen, pilotTestSource, commit)
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	t.Setenv("DEVSPECS_TOKEN", "synthetic-unrelated-ambient-token")
	t.Setenv("DEVSPECS_HTTP_LOOPBACK_ENDPOINT", "https://unrelated.example.invalid/v2")
	var calls, unauthorized atomic.Int32
	posted := make(chan string, 1)
	respond := loopbackServiceEvidenceHandler(handle, status, receipt)
	server, config := newPilotTestPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+pilotTestToken {
			unauthorized.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		assert.Equal(t, "example.com", r.Host)
		if assert.NotNil(t, r.TLS) {
			assert.Equal(t, "example.com", r.TLS.ServerName)
		}
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(io.LimitReader(r.Body, httpDispatchRequestBody+1))
			if !assert.NoError(t, err) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			assert.Equal(t, testHTTPDispatchID, r.Header.Get("Idempotency-Key"))
			select {
			case posted <- string(body):
			default:
				assert.Fail(t, "unexpected duplicate pilot submission")
				w.WriteHeader(http.StatusConflict)
				return
			}
		}
		respond(w, r)
	}))
	service := newPilotTestService(t, home, server, config)
	// The constructor must snapshot only the referenced token, not consult it on each request.
	t.Setenv(pilotTestTokenEnv, pilotTestRotated)

	// Act
	result := service.Start(t.Context(), loopbackServiceOwner, checkout, false, bytes.NewReader(frozen))

	// Assert
	require.Empty(t, result.Error)
	assert.Equal(t, "succeeded", result.State)
	assert.Equal(t, testHTTPDispatchID, result.ID)
	assert.Equal(t, httpDispatchDigest(frozen), result.RequestSHA256)
	assert.EqualValues(t, 4, calls.Load())
	assert.Zero(t, unauthorized.Load())
	require.Len(t, posted, 1)
	body := <-posted
	assert.Equal(t, len(frozen), len(body))
	assert.Equal(t, httpDispatchDigest(frozen), httpDispatchDigest([]byte(body)))
	var request httpDispatchPortableRequest
	require.NoError(t, json.Unmarshal([]byte(body), &request))
	assert.Equal(t, pilotTestSource, request.Repository.GitURL)
	assert.Equal(t, "git:"+pilotTestSource, request.Repository.LogicalID)
	assert.Equal(t, "principal_fixture", request.Caller.PrincipalID)
	assert.Equal(t, "credential_fixture_v1", request.Caller.CredentialID)
	var evidence httpDispatchAdmittedReceipt
	require.NoError(t, json.Unmarshal(result.Receipt, &evidence))
	assert.Equal(t, request.DispatchID, evidence.DispatchID)
	assert.Equal(t, request.Caller.PrincipalID, evidence.Caller.PrincipalID)
	assert.Equal(t, request.Caller.CredentialID, evidence.Caller.CredentialID)
	assert.Equal(t, httpDispatchDigest(receipt), evidence.WireSHA256)
	assert.Equal(t, commit, evidence.SourceCommit)
	require.NotNil(t, evidence.ResultCommit)
	assert.Equal(t, commit, *evidence.ResultCommit)
	persisted := pilotTestPersistedState(t, home)
	assert.NotContains(t, persisted, pilotTestToken)
	assert.NotContains(t, persisted, pilotTestRotated)
	assert.NotContains(t, string(result.Receipt), pilotTestToken)
}

func TestPilotDispatchObserve_WithSamePrincipalRotatedToken_ReplaysOriginalFrozenCaller(t *testing.T) {
	// Arrange
	home, checkout, frozen, commit := newPilotTestRequest(t)
	handle, status, receipt := headlessDispatchResponseBodies(t, frozen, pilotTestSource, commit)
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	var authorization atomic.Value
	authorization.Store("Bearer " + pilotTestToken)
	var posts, unauthorized atomic.Int32
	posted := make(chan string, 2)
	respond := loopbackServiceEvidenceHandler(handle, status, receipt)
	server, config := newPilotTestPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != authorization.Load().(string) {
			unauthorized.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(io.LimitReader(r.Body, httpDispatchRequestBody+1))
			if !assert.NoError(t, err) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			assert.Equal(t, testHTTPDispatchID, r.Header.Get("Idempotency-Key"))
			select {
			case posted <- string(body):
			default:
				assert.Fail(t, "unexpected additional pilot replay")
				w.WriteHeader(http.StatusConflict)
				return
			}
			if posts.Add(1) == 1 {
				writeHTTPDispatchMock(w, http.StatusServiceUnavailable, testHTTPDispatchError("unavailable", true))
				return
			}
		}
		respond(w, r)
	}))
	initial := newPilotTestService(t, home, server, config)
	first := initial.Start(t.Context(), loopbackServiceOwner, checkout, true, bytes.NewReader(frozen))
	require.Equal(t, "provider_rejected", first.Error)
	require.EqualValues(t, 1, posts.Load())
	t.Setenv(pilotTestTokenEnv, pilotTestRotated)
	authorization.Store("Bearer " + pilotTestRotated)
	config["credential_id"] = "credential_fixture_v2"
	rotated := newPilotTestService(t, home, server, config)

	// Act
	result := rotated.Observe(t.Context(), "resume", testHTTPDispatchID)

	// Assert
	require.Empty(t, result.Error)
	assert.Equal(t, "succeeded", result.State)
	assert.EqualValues(t, 2, posts.Load())
	assert.Zero(t, unauthorized.Load())
	require.Len(t, posted, 2)
	firstBody, replayBody := <-posted, <-posted
	assert.Equal(t, len(frozen), len(firstBody))
	assert.Equal(t, len(frozen), len(replayBody))
	assert.Equal(t, httpDispatchDigest(frozen), httpDispatchDigest([]byte(firstBody)))
	assert.Equal(t, httpDispatchDigest(frozen), httpDispatchDigest([]byte(replayBody)))
	pending, err := rotated.store.readHTTPDispatchPending(testHTTPDispatchID)
	require.NoError(t, err)
	assert.Equal(t, len(frozen), len(pending.Request))
	assert.Equal(t, httpDispatchDigest(frozen), httpDispatchDigest(pending.Request))
	var request httpDispatchPortableRequest
	require.NoError(t, json.Unmarshal([]byte(replayBody), &request))
	assert.Equal(t, "principal_fixture", request.Caller.PrincipalID)
	assert.Equal(t, "credential_fixture_v1", request.Caller.CredentialID)
	var evidence httpDispatchAdmittedReceipt
	require.NoError(t, json.Unmarshal(result.Receipt, &evidence))
	assert.Equal(t, request.Caller.PrincipalID, evidence.Caller.PrincipalID)
	assert.Equal(t, request.Caller.CredentialID, evidence.Caller.CredentialID)
	assert.Equal(t, httpDispatchDigest(frozen), evidence.RequestSHA256)
	persisted := pilotTestPersistedState(t, home)
	assert.NotContains(t, persisted, pilotTestToken)
	assert.NotContains(t, persisted, pilotTestRotated)
}

func TestPilotDispatchStart_WithDifferentSourceOnSameHost_RejectsBeforeNetwork(t *testing.T) {
	// Arrange
	home, checkout, frozen, _ := newPilotTestRequest(t)
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	var requests atomic.Int32
	server, config := newPilotTestPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	config["source_url"] = "https://git.example.com/synthetic/other.git"
	service := newPilotTestService(t, home, server, config)

	// Act
	result := service.Start(t.Context(), loopbackServiceOwner, checkout, false, bytes.NewReader(frozen))

	// Assert
	assert.NotEmpty(t, result.Error)
	assert.Empty(t, result.Receipt)
	assert.Zero(t, requests.Load())
	assert.NoDirExists(t, filepath.Join(home, httpDispatchReceiptDir))
}

func TestPilotDispatchStart_WithRedirect_RejectsWithoutForwardingBearerOrLeakingLocation(t *testing.T) {
	// Arrange
	home, checkout, frozen, _ := newPilotTestRequest(t)
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	var probes, redirected atomic.Int32
	server, config := newPilotTestPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/capabilities" {
			probes.Add(1)
			assert.Equal(t, "Bearer "+pilotTestToken, r.Header.Get("Authorization"))
			w.Header().Set("Location", "https://example.com/v2/redirected?secret="+pilotTestRotated)
			w.WriteHeader(http.StatusTemporaryRedirect)
			_, _ = io.WriteString(w, pilotTestToken)
			return
		}
		redirected.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	service := newPilotTestService(t, home, server, config)

	// Act
	result := service.Start(t.Context(), loopbackServiceOwner, checkout, false, bytes.NewReader(frozen))

	// Assert
	assert.NotEmpty(t, result.Error)
	assert.Empty(t, result.Receipt)
	assert.EqualValues(t, 1, probes.Load())
	assert.Zero(t, redirected.Load())
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), pilotTestToken)
	assert.NotContains(t, string(raw), pilotTestRotated)
	assert.NotContains(t, string(raw), "redirected?secret=")
	assert.NoDirExists(t, filepath.Join(home, httpDispatchReceiptDir))
}

func TestPilotDispatchStart_WithMismatchedTLSHostname_RejectsBeforeHTTPRequest(t *testing.T) {
	// Arrange
	home, checkout, frozen, _ := newPilotTestRequest(t)
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	var requests atomic.Int32
	server, config := newPilotTestPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	require.Error(t, server.Certificate().VerifyHostname("pilot.example.invalid"))
	config["endpoint"] = "https://pilot.example.invalid/v2"
	service := newPilotTestService(t, home, server, config)

	// Act
	result := service.Start(t.Context(), loopbackServiceOwner, checkout, false, bytes.NewReader(frozen))

	// Assert
	assert.Equal(t, "exchange_interrupted", result.Error)
	assert.Empty(t, result.Receipt)
	assert.Zero(t, requests.Load())
	assert.NoDirExists(t, filepath.Join(home, httpDispatchReceiptDir))
}

func TestPilotDispatchStart_WithChangedSuccessResult_RejectsOtherwiseAdmissibleCommit(t *testing.T) {
	// Arrange
	home, checkout, frozen, commit := newPilotTestRequest(t)
	// A distinct empty commit is locally verifiable and does not move the source checkout.
	tree, err := runGitText(t.Context(), ExecRunner{}, checkout, "rev-parse", "HEAD^{tree}")
	require.NoError(t, err)
	changed, err := runGitText(t.Context(), ExecRunner{}, checkout,
		"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
		"commit-tree", tree, "-p", commit, "-m", "Synthetic distinct pilot result")
	require.NoError(t, err)
	require.NotEqual(t, commit, changed)
	handle, status, receipt := headlessDispatchResponseBodies(t, frozen, pilotTestSource, commit)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(receipt, &wire))
	wire["repository"] = map[string]string{
		"logical_id": "git:" + pilotTestSource, "source_commit": commit, "result_commit": changed,
	}
	receipt, err = json.Marshal(wire)
	require.NoError(t, err)
	var accepted httpDispatchHandle
	var observed httpDispatchStatus
	require.NoError(t, json.Unmarshal(handle, &accepted))
	require.NoError(t, json.Unmarshal(status, &observed))
	admitted, err := admitHTTPDispatchReceipt(t.Context(), ExecRunner{}, checkout, frozen, &accepted, &observed, receipt, nil)
	require.NoError(t, err, "the fixture must pass general admission before the pilot's read-only check")
	require.NotNil(t, admitted.ResultCommit)
	require.Equal(t, changed, *admitted.ResultCommit)
	t.Setenv(pilotTestTokenEnv, pilotTestToken)
	var receiptReads atomic.Int32
	respond := loopbackServiceEvidenceHandler(handle, status, receipt)
	server, config := newPilotTestPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/dispatches/"+testHTTPDispatchID+"/receipt" {
			receiptReads.Add(1)
		}
		respond(w, r)
	}))
	service := newPilotTestService(t, home, server, config)

	// Act
	result := service.Start(t.Context(), loopbackServiceOwner, checkout, false, bytes.NewReader(frozen))

	// Assert
	assert.Equal(t, "evidence_mismatch", result.Error)
	assert.Empty(t, result.Receipt)
	assert.EqualValues(t, 1, receiptReads.Load())
	assert.NoFileExists(t, filepath.Join(home, httpDispatchReceiptDir, testHTTPDispatchID+".receipt.json"))
	head, err := runGitText(t.Context(), ExecRunner{}, checkout, "rev-parse", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, commit, head)
}

func pilotTestConfig() map[string]any {
	return map[string]any{
		"schema": "devspecs.dispatch.pilot_config", "schema_version": 1, "enabled": true,
		"endpoint": "https://example.com/v2", "source_url": pilotTestSource,
		"principal_id": "principal_fixture", "credential_id": "credential_fixture_v1",
		"token_env": pilotTestTokenEnv,
	}
}

func writePilotTestConfig(t *testing.T, config map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "pilot.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

func newPilotTestRequest(t *testing.T) (home, checkout string, frozen []byte, commit string) {
	t.Helper()
	home, checkout, frozen, commit = newLoopbackServiceTestRequest(t)
	var request httpDispatchPortableRequest
	require.NoError(t, json.Unmarshal(frozen, &request))
	request.Repository.GitURL = pilotTestSource
	request.Repository.LogicalID = "git:" + pilotTestSource
	frozen, err := json.MarshalIndent(request, "", "  ")
	require.NoError(t, err)
	return home, checkout, append(frozen, '\n'), commit
}

func newPilotTestPeer(t *testing.T, handler http.Handler) (*httptest.Server, map[string]any) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	require.NoError(t, server.Certificate().VerifyHostname("example.com"))
	trust := filepath.Join(t.TempDir(), "pilot-ca.pem")
	require.NoError(t, os.WriteFile(trust, pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: server.Certificate().Raw,
	}), 0o600))
	config := pilotTestConfig()
	config["ca_file"] = trust
	return server, config
}

func newPilotTestService(t *testing.T, home string, server *httptest.Server, config map[string]any) *LoopbackDispatch {
	t.Helper()
	service, err := NewPilotDispatch(home, writePilotTestConfig(t, config))
	require.NoError(t, err)
	require.NotNil(t, service)
	transport, ok := service.transport.client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.TLSClientConfig)
	require.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	endpoint, err := url.Parse(config["endpoint"].(string))
	require.NoError(t, err)
	port := endpoint.Port()
	if port == "" {
		port = "443"
	}
	expectedAddress := net.JoinHostPort(endpoint.Hostname(), port)
	// Redirects and unexpected targets cannot escape to DNS or the network.
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != expectedAddress {
			return nil, errors.New("unexpected synthetic pilot dial target")
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	t.Cleanup(transport.CloseIdleConnections)
	return service
}

func pilotTestPersistedState(t *testing.T, home string) string {
	t.Helper()
	base := filepath.Join(home, httpDispatchReceiptDir, testHTTPDispatchID)
	pendingJSON := readHeadlessDispatchFile(t, base+".pending.json")
	var pending httpLoopbackPending
	require.NoError(t, json.Unmarshal([]byte(pendingJSON), &pending))
	return strings.Join([]string{
		pendingJSON, string(pending.Request),
		readHeadlessDispatchFile(t, base+".handle.json"),
		readHeadlessDispatchFile(t, base+".receipt.json"),
	}, "\n")
}
