//go:build devspecs_http_loopback

package orchestration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const loopbackServiceOwner = "task:loopback-service-fixture"

func TestLoopbackDispatchStart_WithChangedFrozenBytes_RejectsReplayWithoutPosting(t *testing.T) {
	// Arrange
	home, checkout, frozen, commit := newLoopbackServiceTestRequest(t)
	handle, _, _ := headlessDispatchResponseBodies(t, frozen, loopbackSource, commit)
	var posts atomic.Int32
	service := newLoopbackServiceTestPeer(t, home, withHTTPDispatchCapabilities(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			if r.URL.Path == "/v2/dispatches" {
				writeHTTPDispatchMock(w, http.StatusCreated, string(handle))
				return
			}
		}
		http.NotFound(w, r)
	}))
	first := service.Start(t.Context(), loopbackServiceOwner, checkout, true, bytes.NewReader(frozen))
	require.Empty(t, first.Error)
	require.Equal(t, "accepted", first.State)
	require.EqualValues(t, 1, posts.Load())
	changed := append(bytes.Clone(frozen), ' ', '\n')

	// Act
	result := service.Start(t.Context(), loopbackServiceOwner, checkout, true, bytes.NewReader(changed))

	// Assert
	assert.Equal(t, "request_conflict", result.Error)
	assert.EqualValues(t, 1, posts.Load())
	pending, err := service.store.readHTTPDispatchPending(testHTTPDispatchID)
	require.NoError(t, err)
	assert.Equal(t, httpDispatchDigest(frozen), pending.RequestSHA256)
}

func TestLoopbackDispatchStart_WithUnreadyProvider_RejectsBeforePersistingOrPosting(t *testing.T) {
	// Arrange
	home, checkout, frozen, _ := newLoopbackServiceTestRequest(t)
	var caps map[string]any
	require.NoError(t, json.Unmarshal([]byte(testHTTPDispatchCaps), &caps))
	caps["ready"] = false
	capsBody, err := json.Marshal(caps)
	require.NoError(t, err)
	var probes, posts atomic.Int32
	service := newLoopbackServiceTestPeer(t, home, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v2/capabilities" {
			probes.Add(1)
			writeHTTPDispatchMock(w, http.StatusOK, string(capsBody))
			return
		}
		if r.Method == http.MethodPost {
			posts.Add(1)
		}
		http.NotFound(w, r)
	}))

	// Act
	result := service.Start(t.Context(), loopbackServiceOwner, checkout, true, bytes.NewReader(frozen))

	// Assert
	assert.Equal(t, "incompatible_provider", result.Error)
	assert.EqualValues(t, 1, probes.Load())
	assert.Zero(t, posts.Load())
	_, err = os.Lstat(filepath.Join(home, httpDispatchReceiptDir, testHTTPDispatchID+".pending.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLoopbackDispatchStart_WithReceiptForDifferentRequest_RejectsWithoutStoringEvidence(t *testing.T) {
	// Arrange
	home, checkout, frozen, commit := newLoopbackServiceTestRequest(t)
	handle, status, receipt := headlessDispatchResponseBodies(t, frozen, loopbackSource, commit)
	var evidence map[string]any
	require.NoError(t, json.Unmarshal(receipt, &evidence))
	evidence["request_sha256"] = httpDispatchDigest([]byte("unrelated synthetic request"))
	badReceipt, err := json.Marshal(evidence)
	require.NoError(t, err)
	var receiptReads atomic.Int32
	respond := loopbackServiceEvidenceHandler(handle, status, badReceipt)
	service := newLoopbackServiceTestPeer(t, home, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v2/dispatches/"+testHTTPDispatchID+"/receipt" {
			receiptReads.Add(1)
		}
		respond(w, r)
	}))

	// Act
	result := service.Start(t.Context(), loopbackServiceOwner, checkout, false, bytes.NewReader(frozen))

	// Assert
	assert.Equal(t, "evidence_mismatch", result.Error)
	assert.Empty(t, result.Receipt)
	assert.EqualValues(t, 1, receiptReads.Load())
	_, err = os.Lstat(filepath.Join(home, httpDispatchReceiptDir, testHTTPDispatchID+".receipt.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLoopbackDispatchObserve_WithOtherPrincipal_RejectsPendingResumeWithoutPosting(t *testing.T) {
	// Arrange
	home, checkout, frozen, _ := newLoopbackServiceTestRequest(t)
	var requests atomic.Int32
	peer := withLoopbackCLIPeer(t, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	initial := newLoopbackServiceForPeer(t, home, "initial", peer)
	writeLoopbackServiceTestPending(t, initial, checkout, frozen, time.Now().UTC().Add(time.Hour))
	other := newLoopbackServiceForPeer(t, home, "other", peer)

	// Act
	result := other.Observe(t.Context(), "resume", testHTTPDispatchID)

	// Assert
	assert.Equal(t, "configuration_mismatch", result.Error)
	assert.Empty(t, result.Receipt)
	assert.Zero(t, requests.Load())
}

func TestLoopbackDispatchObserve_WithOtherPrincipal_RejectsCachedReceiptWithoutNetwork(t *testing.T) {
	// Arrange
	home, checkout, frozen, commit := newLoopbackServiceTestRequest(t)
	handle, status, receipt := headlessDispatchResponseBodies(t, frozen, loopbackSource, commit)
	var requests atomic.Int32
	respond := loopbackServiceEvidenceHandler(handle, status, receipt)
	peer := withLoopbackCLIPeer(t, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		respond(w, r)
	}))
	initial := newLoopbackServiceForPeer(t, home, "initial", peer)
	first := initial.Start(t.Context(), loopbackServiceOwner, checkout, false, bytes.NewReader(frozen))
	require.Empty(t, first.Error)
	require.Equal(t, "succeeded", first.State)
	require.NotEmpty(t, first.Receipt)
	before := requests.Load()
	other := newLoopbackServiceForPeer(t, home, "other", peer)

	// Act
	result := other.Observe(t.Context(), "receipt", testHTTPDispatchID)

	// Assert
	assert.Equal(t, "configuration_mismatch", result.Error)
	assert.Empty(t, result.Receipt)
	assert.Equal(t, before, requests.Load())
}

func TestLoopbackDispatchObserve_WithReceiptBoundToRotatedCredential_RejectsWithoutStoringEvidence(t *testing.T) {
	// Arrange
	home, checkout, frozen, commit := newLoopbackServiceTestRequest(t)
	handle, status, receipt := headlessDispatchResponseBodies(t, frozen, loopbackSource, commit)
	var evidence map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(receipt, &evidence))
	caller, err := json.Marshal(httpDispatchCaller{PrincipalID: "principal_fixture", CredentialID: "credential_fixture_v2"})
	require.NoError(t, err)
	evidence["caller"] = caller
	badReceipt, err := json.Marshal(evidence)
	require.NoError(t, err)
	peer := withLoopbackCLIPeer(t, nil, loopbackServiceEvidenceHandler(handle, status, badReceipt))
	initial := newLoopbackServiceForPeer(t, home, "initial", peer)
	first := initial.Start(t.Context(), loopbackServiceOwner, checkout, true, bytes.NewReader(frozen))
	require.Empty(t, first.Error)
	require.Equal(t, "accepted", first.State)
	rotated := newLoopbackServiceForPeer(t, home, "rotated", peer)

	// Act
	result := rotated.Observe(t.Context(), "resume", testHTTPDispatchID)

	// Assert
	assert.Equal(t, "evidence_mismatch", result.Error)
	assert.Empty(t, result.Receipt)
	_, err = os.Lstat(filepath.Join(home, httpDispatchReceiptDir, testHTTPDispatchID+".receipt.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLoopbackDispatchObserve_WithExpiredPending_RejectsResumeWithoutNetwork(t *testing.T) {
	// Arrange
	home, checkout, frozen, _ := newLoopbackServiceTestRequest(t)
	var requests atomic.Int32
	service := newLoopbackServiceTestPeer(t, home, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	writeLoopbackServiceTestPending(t, service, checkout, frozen, time.Now().UTC().Add(-time.Minute))

	// Act
	result := service.Observe(t.Context(), "resume", testHTTPDispatchID)

	// Assert
	assert.Equal(t, "recovery_expired", result.Error)
	assert.Zero(t, requests.Load())
}

func writeLoopbackServiceTestPending(t *testing.T, service *LoopbackDispatch, checkout string, frozen []byte, expires time.Time) {
	t.Helper()
	_, err := service.store.writeHTTPDispatchPending(httpLoopbackPending{
		DispatchID:    testHTTPDispatchID,
		Request:       frozen,
		RequestSHA256: httpDispatchDigest(frozen),
		Endpoint:      service.transport.base,
		TrustSHA256:   service.trustSHA256,
		Provider:      "fixture-provider",
		Checkout:      checkout,
		ExpiresAt:     expires,
	})
	require.NoError(t, err)
}

func loopbackServiceEvidenceHandler(handle, status, receipt []byte) http.HandlerFunc {
	const runPath = "/v2/dispatches/" + testHTTPDispatchID
	return withHTTPDispatchCapabilities(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v2/dispatches":
			writeHTTPDispatchMock(w, http.StatusCreated, string(handle))
		case r.Method == http.MethodGet && r.URL.Path == runPath:
			writeHTTPDispatchMock(w, http.StatusOK, string(status))
		case r.Method == http.MethodGet && r.URL.Path == runPath+"/receipt":
			writeHTTPDispatchMock(w, http.StatusOK, string(receipt))
		default:
			http.NotFound(w, r)
		}
	})
}

func newLoopbackServiceTestRequest(t *testing.T) (home, checkout string, frozen []byte, commit string) {
	t.Helper()
	runner, home := newLoopbackCLIRunner(t)
	// The service runs its local Git checks in the test process environment.
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("GIT_ALLOW_PROTOCOL", "")
	spec := portableFixtureSpec()
	spec.Target.Owner = loopbackServiceOwner
	spec.TimeoutSeconds = 15
	checkout, frozen, commit = newLoopbackCLIFrozen(t, runner, spec)
	return home, checkout, frozen, commit
}

func newLoopbackServiceTestPeer(t *testing.T, home string, handler http.Handler) *LoopbackDispatch {
	t.Helper()
	return newLoopbackServiceForPeer(t, home, "", withLoopbackCLIPeer(t, nil, handler))
}

func newLoopbackServiceForPeer(t *testing.T, home, slot string, env headlessDispatchRunner) *LoopbackDispatch {
	t.Helper()
	require.Len(t, env, 2)
	endpoint, ok := strings.CutPrefix(env[0], "DEVSPECS_HTTP_LOOPBACK_ENDPOINT=")
	require.True(t, ok)
	trust, ok := strings.CutPrefix(env[1], "DEVSPECS_HTTP_LOOPBACK_CA=")
	require.True(t, ok)
	service, err := NewLoopbackDispatch(home, endpoint, trust, slot)
	require.NoError(t, err)
	return service
}
