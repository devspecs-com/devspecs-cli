package orchestration

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPDispatchCapabilities_WithTLSMock_ReturnsCapabilities(t *testing.T) {
	// Arrange
	transport := newTestHTTPDispatch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v2/capabilities", r.URL.Path)
		writeHTTPDispatchMock(w, http.StatusOK, testHTTPDispatchCaps)
	})

	// Act
	caps, err := transport.capabilities(t.Context())

	// Assert
	require.NoError(t, err)
	assert.True(t, caps.Ready)
	assert.Equal(t, "fixture-provider", caps.Provider)
	assert.Equal(t, 86400, caps.IdempotencyRetentionSecs)
	assert.Equal(t, 2097152, caps.Limits.MaxRequestBytes)
	require.Len(t, caps.VerificationProfiles, 1)
	assert.Equal(t, "go-unit", caps.VerificationProfiles[0].Key)
	assert.Equal(t, "suite", caps.VerificationProfiles[0].Strength)
}

func TestHTTPDispatchCapabilities_WithoutDraftRevision_RejectsProvider(t *testing.T) {
	// Arrange
	old := strings.Replace(testHTTPDispatchCaps, `"contract_revision":"2.0.0-draft.2",`, "", 1)
	transport := newTestHTTPDispatch(t, func(w http.ResponseWriter, r *http.Request) {
		writeHTTPDispatchMock(w, http.StatusOK, old)
	})

	// Act
	caps, err := transport.capabilities(t.Context())

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchResponse)
	assert.Nil(t, caps)
}

func TestHTTPDispatchSubmit_WithFrozenRequest_PostsExactBytes(t *testing.T) {
	// Arrange
	var posts atomic.Int32
	transport := newTestHTTPDispatch(t, withHTTPDispatchCapabilities(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		assertHTTPDispatchPost(t, r)
		writeHTTPDispatchMock(w, http.StatusCreated, testHTTPDispatchHandle)
	}))
	caps := loadTestHTTPDispatchCaps(t, transport)

	// Act
	handle, err := transport.submit(t.Context(), caps, []byte(testHTTPDispatchRequest), nil)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, testHTTPDispatchID, handle.DispatchID)
	assert.Equal(t, testHTTPDispatchDigest, handle.RequestSHA256)
	assert.Equal(t, "run_fixture_001", handle.Provider.RunID)
	assert.EqualValues(t, 1, posts.Load())
}

func TestHTTPDispatchSubmit_AfterLostResponse_OnlyReplaysOnExplicitCall(t *testing.T) {
	// Arrange
	var posts atomic.Int32
	transport := newTestHTTPDispatch(t, withHTTPDispatchCapabilities(func(w http.ResponseWriter, r *http.Request) {
		assertHTTPDispatchPost(t, r)
		if posts.Add(1) == 1 {
			conn, _, err := http.NewResponseController(w).Hijack()
			if assert.NoError(t, err) {
				conn.Close()
			}
			return
		}
		writeHTTPDispatchMock(w, http.StatusOK, testHTTPDispatchHandle)
	}))
	caps := loadTestHTTPDispatchCaps(t, transport)
	frozen := []byte(testHTTPDispatchRequest)

	// Act
	lostHandle, lostErr := transport.submit(t.Context(), caps, frozen, nil)
	postsAfterLoss := posts.Load()
	replayed, replayErr := transport.submit(t.Context(), caps, frozen, nil)

	// Assert
	require.ErrorIs(t, lostErr, errHTTPDispatchNetwork)
	assert.Nil(t, lostHandle)
	assert.EqualValues(t, 1, postsAfterLoss)
	require.NoError(t, replayErr)
	assert.Equal(t, "run_fixture_001", replayed.Provider.RunID)
	assert.Equal(t, testHTTPDispatchDigest, replayed.RequestSHA256)
	assert.EqualValues(t, 2, posts.Load())
}

func TestHTTPDispatchSubmit_WithKnownHandle_RejectsChangedReplayRun(t *testing.T) {
	// Arrange
	var posts atomic.Int32
	transport := newTestHTTPDispatch(t, withHTTPDispatchCapabilities(func(w http.ResponseWriter, r *http.Request) {
		assertHTTPDispatchPost(t, r)
		if posts.Add(1) == 1 {
			writeHTTPDispatchMock(w, http.StatusCreated, testHTTPDispatchHandle)
			return
		}
		writeHTTPDispatchMock(w, http.StatusOK, strings.Replace(testHTTPDispatchHandle, "run_fixture_001", "different-run", 1))
	}))
	caps := loadTestHTTPDispatchCaps(t, transport)
	frozen := []byte(testHTTPDispatchRequest)
	accepted, err := transport.submit(t.Context(), caps, frozen, nil)
	require.NoError(t, err)

	// Act
	replay, err := transport.submit(t.Context(), caps, frozen, accepted)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchCorrelation)
	assert.Nil(t, replay)
	assert.EqualValues(t, 2, posts.Load())
}

func TestHTTPDispatchSubmit_WithoutRequiredProfile_DoesNotPost(t *testing.T) {
	// Arrange
	var posts atomic.Int32
	transport := newTestHTTPDispatch(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		noProfile := strings.Replace(testHTTPDispatchCaps, `[{"key":"go-unit","strength":"suite"}]`, "[]", 1)
		writeHTTPDispatchMock(w, http.StatusOK, noProfile)
	})
	caps := loadTestHTTPDispatchCaps(t, transport)

	// Act
	handle, err := transport.submit(t.Context(), caps, []byte(testHTTPDispatchRequest), nil)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchPreflight)
	assert.Nil(t, handle)
	assert.Zero(t, posts.Load())
}

func TestHTTPDispatchSubmit_WithOversizedRequest_DoesNotPost(t *testing.T) {
	// Arrange
	var calls atomic.Int32
	transport := newTestHTTPDispatch(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	body := make([]byte, httpDispatchRequestBody+1)

	// Act
	handle, err := transport.submit(t.Context(), nil, body, nil)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchLimit)
	assert.Nil(t, handle)
	assert.Zero(t, calls.Load())
}

func TestHTTPDispatchSubmit_WithDuplicateDispatchID_DoesNotPost(t *testing.T) {
	// Arrange
	var posts atomic.Int32
	transport := newTestHTTPDispatch(t, withHTTPDispatchCapabilities(func(w http.ResponseWriter, _ *http.Request) {
		posts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	caps := loadTestHTTPDispatchCaps(t, transport)
	body := strings.Replace(testHTTPDispatchRequest, "{", `{"dispatch_id":"`+testHTTPDispatchID+`",`, 1)

	// Act
	handle, err := transport.submit(t.Context(), caps, []byte(body), nil)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchRequest)
	assert.Nil(t, handle)
	assert.Zero(t, posts.Load())
}

func TestHTTPDispatchStatus_WithMatchingHandle_ReturnsRunningState(t *testing.T) {
	// Arrange
	transport, handle := newAcceptedHTTPDispatch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v2/dispatches/"+testHTTPDispatchID, r.URL.Path)
		writeHTTPDispatchMock(w, http.StatusOK, testHTTPDispatchRunning)
	})

	// Act
	status, err := transport.status(t.Context(), handle)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "running", status.State)
	assert.False(t, status.CancelRequested)
	assert.JSONEq(t, `{"output_tokens":42}`, string(status.Usage))
}

func TestHTTPDispatchStatus_WithWrongVersion_ReturnsNoObservation(t *testing.T) {
	// Arrange
	wrongVersion := strings.Replace(testHTTPDispatchRunning, `"schema_version":2`, `"schema_version":1`, 1)
	transport, handle := newAcceptedHTTPDispatch(t, func(w http.ResponseWriter, _ *http.Request) {
		writeHTTPDispatchMock(w, http.StatusOK, wrongVersion)
	})

	// Act
	status, err := transport.status(t.Context(), handle)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchResponse)
	assert.Nil(t, status)
}

func TestHTTPDispatchStatus_WithDifferentRun_ReturnsNoObservation(t *testing.T) {
	// Arrange
	wrongRun := strings.Replace(testHTTPDispatchRunning, "run_fixture_001", "unrelated-run", 1)
	transport, handle := newAcceptedHTTPDispatch(t, func(w http.ResponseWriter, _ *http.Request) {
		writeHTTPDispatchMock(w, http.StatusOK, wrongRun)
	})

	// Act
	status, err := transport.status(t.Context(), handle)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchCorrelation)
	assert.Nil(t, status)
}

func TestHTTPDispatchReceipt_WithMatchingTerminalRecord_PreservesWireEvidence(t *testing.T) {
	// Arrange
	transport, handle := newAcceptedHTTPDispatch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v2/dispatches/"+testHTTPDispatchID+"/receipt", r.URL.Path)
		writeHTTPDispatchMock(w, http.StatusOK, testHTTPDispatchReceipt)
	})

	// Act
	receipt, err := transport.receipt(t.Context(), handle)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, testHTTPDispatchReceipt, string(receipt))
}

func TestHTTPDispatchReceipt_WithWrongDigest_ReturnsNoReceipt(t *testing.T) {
	// Arrange
	wrongDigest := strings.Replace(testHTTPDispatchReceipt, testHTTPDispatchDigest, "sha256:"+strings.Repeat("0", 64), 1)
	transport, handle := newAcceptedHTTPDispatch(t, func(w http.ResponseWriter, _ *http.Request) {
		writeHTTPDispatchMock(w, http.StatusOK, wrongDigest)
	})

	// Act
	receipt, err := transport.receipt(t.Context(), handle)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchCorrelation)
	assert.Nil(t, receipt)
}

func TestHTTPDispatchReceipt_WithIncompleteRecord_ReturnsNoReceipt(t *testing.T) {
	// Arrange
	transport, handle := newAcceptedHTTPDispatch(t, func(w http.ResponseWriter, _ *http.Request) {
		writeHTTPDispatchMock(w, http.StatusOK, `{"schema":"devspecs.dispatch.receipt","schema_version":2,"dispatch_id":"`+testHTTPDispatchID+`"}`)
	})

	// Act
	receipt, err := transport.receipt(t.Context(), handle)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchResponse)
	assert.Nil(t, receipt)
}

func TestHTTPDispatchReceipt_WithUnsafeRepoPath_ReturnsNoReceipt(t *testing.T) {
	// Arrange
	traversal := strings.Replace(testHTTPDispatchReceipt, "src/fixture.go", "src/../fixture.go", 1)
	transport, handle := newAcceptedHTTPDispatch(t, func(w http.ResponseWriter, _ *http.Request) {
		writeHTTPDispatchMock(w, http.StatusOK, traversal)
	})

	// Act
	receipt, err := transport.receipt(t.Context(), handle)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchResponse)
	assert.Nil(t, receipt)
}

func TestHTTPDispatchReceipt_WhenNotReady_ReturnsSafeConflict(t *testing.T) {
	// Arrange
	transport, handle := newAcceptedHTTPDispatch(t, func(w http.ResponseWriter, _ *http.Request) {
		writeHTTPDispatchMock(w, http.StatusConflict, testHTTPDispatchError("receipt_not_ready", false))
	})

	// Act
	receipt, err := transport.receipt(t.Context(), handle)

	// Assert
	var statusErr *httpDispatchError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusConflict, statusErr.Status)
	assert.Equal(t, "receipt_not_ready", statusErr.Code)
	assert.False(t, statusErr.Retryable)
	assert.Nil(t, receipt)
}

func TestHTTPDispatchReceipt_WithOversizedChunkedBody_ReturnsNoReceipt(t *testing.T) {
	// Arrange
	transport, handle := newAcceptedHTTPDispatch(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = http.NewResponseController(w).Flush()
		_, _ = io.WriteString(w, strings.Repeat(" ", httpDispatchReceiptBody+1))
	})

	// Act
	receipt, err := transport.receipt(t.Context(), handle)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchLimit)
	assert.Nil(t, receipt)
}

func TestHTTPDispatchCapabilities_WithOversizedBody_ReturnsNoCapabilities(t *testing.T) {
	// Arrange
	transport := newTestHTTPDispatch(t, func(w http.ResponseWriter, _ *http.Request) {
		writeHTTPDispatchMock(w, http.StatusOK, strings.Repeat(" ", httpDispatchSmallBody+1))
	})

	// Act
	caps, err := transport.capabilities(t.Context())

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchLimit)
	assert.Nil(t, caps)
}

func TestHTTPDispatchCapabilities_WithStalledBody_HonorsClientDeadline(t *testing.T) {
	// Arrange
	started := make(chan struct{}, 1)
	transport := newTestHTTPDispatch(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = http.NewResponseController(w).Flush()
		started <- struct{}{}
		<-r.Context().Done()
	})
	transport.callTimeout = 500 * time.Millisecond

	// Act
	caps, err := transport.capabilities(t.Context())

	// Assert
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Len(t, started, 1)
	assert.Nil(t, caps)
}

func TestHTTPDispatchCapabilities_WithDenial_DoesNotExposeCredentialsOrBody(t *testing.T) {
	// Arrange
	transport := newTestHTTPDispatch(t, func(w http.ResponseWriter, _ *http.Request) {
		writeHTTPDispatchMock(w, http.StatusForbidden, testHTTPDispatchError("forbidden", true))
	})

	// Act
	caps, err := transport.capabilities(t.Context())

	// Assert
	var statusErr *httpDispatchError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusForbidden, statusErr.Status)
	assert.Equal(t, "forbidden", statusErr.Code)
	assert.False(t, statusErr.Retryable)
	assert.NotContains(t, err.Error(), testHTTPDispatchToken)
	assert.NotContains(t, err.Error(), "provider private message")
	assert.NotContains(t, err.Error(), transport.base)
	assert.Nil(t, caps)
}

func TestHTTPDispatchCapabilities_WithRedirect_DoesNotFollow(t *testing.T) {
	// Arrange
	var landingHits atomic.Int32
	transport := newTestHTTPDispatch(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/capabilities" {
			w.Header().Set("Location", "/landing")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		landingHits.Add(1)
		writeHTTPDispatchMock(w, http.StatusOK, testHTTPDispatchCaps)
	})

	// Act
	caps, err := transport.capabilities(t.Context())

	// Assert
	var statusErr *httpDispatchError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusTemporaryRedirect, statusErr.Status)
	assert.False(t, statusErr.Retryable)
	assert.Zero(t, landingHits.Load())
	assert.Nil(t, caps)
}

func TestHTTPDispatchCapabilities_WhenRateLimited_ReportsDelayWithoutRetrying(t *testing.T) {
	// Arrange
	var calls atomic.Int32
	transport := newTestHTTPDispatch(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "3")
		writeHTTPDispatchMock(w, http.StatusTooManyRequests, testHTTPDispatchError("rate_limited", true))
	})

	// Act
	caps, err := transport.capabilities(t.Context())

	// Assert
	var statusErr *httpDispatchError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, "rate_limited", statusErr.Code)
	assert.True(t, statusErr.Retryable)
	assert.Equal(t, 3*time.Second, statusErr.RetryAfter)
	assert.EqualValues(t, 1, calls.Load())
	assert.Nil(t, caps)
}

func TestHTTPDispatchCancel_WhenAccepted_ReportsPendingCancellation(t *testing.T) {
	// Arrange
	pending := strings.Replace(testHTTPDispatchRunning, `"cancel_requested":false`, `"cancel_requested":true`, 1)
	transport, handle := newAcceptedHTTPDispatch(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v2/dispatches/"+testHTTPDispatchID+"/cancel", r.URL.Path)
		assert.Zero(t, r.ContentLength)
		writeHTTPDispatchMock(w, http.StatusAccepted, pending)
	})

	// Act
	status, err := transport.cancel(t.Context(), handle)

	// Assert
	require.NoError(t, err)
	assert.True(t, status.CancelRequested)
	assert.Equal(t, "running", status.State)
}

func TestHTTPDispatchCancel_WhenAlreadyTerminal_ReturnsTerminalObservation(t *testing.T) {
	// Arrange
	cancelled := strings.Replace(testHTTPDispatchRunning, `"state":"running","cancel_requested":false`,
		`"state":"cancelled","cancel_requested":true`, 1)
	transport, handle := newAcceptedHTTPDispatch(t, func(w http.ResponseWriter, _ *http.Request) {
		writeHTTPDispatchMock(w, http.StatusOK, cancelled)
	})

	// Act
	status, err := transport.cancel(t.Context(), handle)

	// Assert
	require.NoError(t, err)
	assert.True(t, status.CancelRequested)
	assert.Equal(t, "cancelled", status.State)
}

func TestNewHTTPDispatchTransport_WithNonLoopbackEndpoint_RejectsEndpoint(t *testing.T) {
	// Arrange
	endpoint := "https://192.0.2.1/v2"

	// Act
	transport, err := newHTTPDispatchTransport(endpoint, testHTTPDispatchToken, nil)

	// Assert
	require.Error(t, err)
	assert.NotContains(t, err.Error(), endpoint)
	assert.Nil(t, transport)
}

func newTestHTTPDispatch(t *testing.T, handler http.HandlerFunc) *httpDispatchTransport {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer "+testHTTPDispatchToken, r.Header.Get("Authorization"))
		assert.Equal(t, "identity", r.Header.Get("Accept-Encoding"))
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport, err := newHTTPDispatchTransport(server.URL+"/v2/", testHTTPDispatchToken, roots)
	require.NoError(t, err)
	t.Cleanup(transport.client.CloseIdleConnections)
	return transport
}

func withHTTPDispatchCapabilities(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/capabilities" && r.Method == http.MethodGet {
			writeHTTPDispatchMock(w, http.StatusOK, testHTTPDispatchCaps)
			return
		}
		next(w, r)
	}
}

func loadTestHTTPDispatchCaps(t *testing.T, transport *httpDispatchTransport) *httpDispatchCapabilities {
	t.Helper()
	caps, err := transport.capabilities(t.Context())
	require.NoError(t, err)
	return caps
}

func newAcceptedHTTPDispatch(t *testing.T, next http.HandlerFunc) (*httpDispatchTransport, *httpDispatchHandle) {
	t.Helper()
	transport := newTestHTTPDispatch(t, withHTTPDispatchCapabilities(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/dispatches" && r.Method == http.MethodPost {
			writeHTTPDispatchMock(w, http.StatusCreated, testHTTPDispatchHandle)
			return
		}
		next(w, r)
	}))
	caps := loadTestHTTPDispatchCaps(t, transport)
	handle, err := transport.submit(t.Context(), caps, []byte(testHTTPDispatchRequest), nil)
	require.NoError(t, err)
	return transport, handle
}

func assertHTTPDispatchPost(t *testing.T, r *http.Request) {
	t.Helper()
	assert.Equal(t, http.MethodPost, r.Method)
	assert.Equal(t, "/v2/dispatches", r.URL.Path)
	assert.Equal(t, testHTTPDispatchID, r.Header.Get("Idempotency-Key"))
	assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
	assert.Empty(t, r.Header.Get("Content-Encoding"))
	body, err := io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.Equal(t, testHTTPDispatchRequest, string(body))
}

func writeHTTPDispatchMock(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func testHTTPDispatchError(code string, retryable bool) string {
	return fmt.Sprintf(`{"schema":"devspecs.dispatch.error","schema_version":2,"code":%q,"message":%q,"retryable":%t}`,
		code, "provider private message "+testHTTPDispatchToken, retryable)
}

const (
	testHTTPDispatchToken  = "synthetic-mock-bearer"
	testHTTPDispatchID     = "dispatch_01KZZW1BBN55VK2GVYQNCQBZB9"
	testHTTPDispatchDigest = "sha256:3ff354e91eef568e814ebbe3e762401a2f5f0a4e7d3decc6078888222cbe9a60"
	testHTTPDispatchCore   = `"schema_version":2,"dispatch_id":"` + testHTTPDispatchID +
		`","request_sha256":"` + testHTTPDispatchDigest + `","provider":{"name":"fixture-provider","run_id":"run_fixture_001"}`
	testHTTPDispatchHandle  = `{"schema":"devspecs.dispatch.handle",` + testHTTPDispatchCore + `,"accepted_at":"2026-11-10T12:00:00Z"}`
	testHTTPDispatchRunning = `{"schema":"devspecs.dispatch.status",` + testHTTPDispatchCore +
		`,"state":"running","cancel_requested":false,"observed_at":"2026-11-10T12:00:02Z","usage":{"output_tokens":42}}`
	testHTTPDispatchCaps = `{
		"schema":"devspecs.dispatch.capabilities","schema_version":2,"contract_revision":"2.0.0-draft.2","provider":"fixture-provider",
		"ready":true,"isolation":true,"report":true,"cancellation":true,
		"verification_profiles":[{"key":"go-unit","strength":"suite"}],
		"limits":{"max_request_bytes":2097152,"max_inline_bytes":1048576,"max_artifact_bytes":33554432,"max_timeout_seconds":3600,"max_attempts":1},
		"idempotency_retention_seconds":86400
	}`
	testHTTPDispatchReceipt = `{"schema":"devspecs.dispatch.receipt",` + testHTTPDispatchCore + `,
		"caller":{"principal_id":"principal_fixture","credential_id":"credential_fixture_v1"},
		"receipt_id":"receipt_01KZZW1KXP4AZ7TSB0NN68HJ86",
		"execution":{"state":"succeeded","attempts":1,"started_at":"2026-11-10T12:00:00Z","ended_at":"2026-11-10T12:00:05Z","failure":null},
		"repository":{
			"logical_id":"git:https://git.example/example/fixture@a5e916aaf179dc06b21dc07cb7e66b37ac3f79be",
			"source_commit":"a5e916aaf179dc06b21dc07cb7e66b37ac3f79be",
			"result_commit":"b5e916aaf179dc06b21dc07cb7e66b37ac3f79be"
		},
		"evidence":{"files_read":["src/fixture.go"],"files_edited":[],"paths_complete":true},
		"report":{"state":"produced","artifact":{
			"artifact_id":"fixture-report","kind":"inline","media_type":"text/plain","size_bytes":15,
			"sha256":"sha256:5c5c98fb01040fd8e50cd5256542800b5ea402f759f11a25fc2f441a111be675",
			"content_base64":"Zml4dHVyZSByZXBvcnQK"
		}},
		"verification":{"state":"unavailable"},
		"cancellation":{"supported":true,"requested":false},
		"usage":{"output_tokens":42}
	}`
)

// The LF-pinned draft 2 request fixture, including the final newline.
const testHTTPDispatchRequest = `{
  "schema": "devspecs.dispatch.request",
  "schema_version": 2,
  "dispatch_id": "dispatch_01KZZW1BBN55VK2GVYQNCQBZB9",
  "created_at": "2026-11-10T12:00:00Z",
  "caller": {
    "principal_id": "principal_fixture",
    "credential_id": "credential_fixture_v1"
  },
  "target": {
    "owner": "task:fixture",
    "slice": "A01",
    "thread": null
  },
  "repository": {
    "logical_id": "git:https://git.example/example/fixture@a5e916aaf179dc06b21dc07cb7e66b37ac3f79be",
    "git_url": "https://git.example/example/fixture.git",
    "source_commit": "a5e916aaf179dc06b21dc07cb7e66b37ac3f79be",
    "credential_ref": "git-read-fixture"
  },
  "input": {
    "apply_payload": "{\n  \"target\": \"A01\",\n  \"prompt\": \"Review src/fixture.go (<fixture>).\\nApply only slice A01 in isolation.\\nTreat DevSpecs lifecycle files as read-only. The caller owns checkpoint decisions.\\n\"\n}\n",
    "apply_sha256": "sha256:b441b7b23eea391830f42d62769311c893d5c711e78ec1430278d36727170a66",
    "context_artifacts": [
      {
        "artifact_id": "fixture-context",
        "kind": "inline",
        "media_type": "text/plain",
        "size_bytes": 16,
        "sha256": "sha256:f7b54cc49107e3807a6b70e2be27bec85ca9432380a74f001aa39843dd251da5",
        "content_base64": "Zml4dHVyZSBjb250ZXh0Cg=="
      }
    ],
    "execution_prompt_sha256": "sha256:09c95a5c0e5d9d4424d7afe3a467fc4df1a8d2ada9ef5a0b0446ab0152238b4d",
    "execution_policy": {
      "task_evidence": "read_only",
      "promotion": "caller_only"
    }
  },
  "requirements": {
    "timeout_seconds": 600,
    "max_attempts": 1,
    "isolation": "required",
    "cancellation": "optional",
    "report": {
      "required": true
    },
    "verification": {
      "required": true,
      "profile": "go-unit",
      "strength": "suite"
    }
  },
  "options": {
    "agent_provider": "ExampleRuntime",
    "model": "Example/Model:Latest",
    "effort": "High"
  }
}
`
