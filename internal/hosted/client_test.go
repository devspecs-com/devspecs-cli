package hosted

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const principalID = "11111111-1111-4111-8111-111111111111"
const sessionID = "22222222-2222-4222-8222-222222222222"
const tenantID = "33333333-3333-4333-8333-333333333333"
const repoID = "44444444-4444-4444-8444-444444444444"

var sessionToken = "shs_" + strings.Repeat("A", 43)
var pollToken = "shp_" + strings.Repeat("A", 43)
var serviceToken = "sph_" + strings.Repeat("A", 43)

func tlsClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client, err := NewClient(server.URL+"/base", roots)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return client, server
}

func wire(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func sessionAt(now time.Time) Session { return Session{principalID, sessionID, now.Add(time.Hour)} }
func approvedRemote(url string) config.RemoteConfig {
	return config.RemoteConfig{Version: 1, Name: "hosted", URL: url, TenantID: tenantID, RepoID: repoID}
}

func loginClient(t *testing.T, poll http.HandlerFunc) (*Client, *[]time.Duration) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(14 * time.Minute)
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/base/v1/auth/github/device" {
			wire(w, 201, deviceWire{"1", "ABCD-EFGH", "https://github.com/login/device", expires, 5, pollToken})
			return
		}
		poll(w, r)
	})
	waits := []time.Duration{}
	client.now = func() time.Time { return now }
	client.wait = func(ctx context.Context, d time.Duration) error {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		waits = append(waits, d)
		now = now.Add(d)
		return nil
	}
	return client, &waits
}

func TestHostedLogin_WithApprovedDeviceFlow_ReturnsBoundSessionAndSafePrompt(t *testing.T) {
	// Arrange
	requests := make(chan string, 1)
	client, waits := loginClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Header.Get("Authorization")
		wire(w, 200, pollWire{Version: "1", Status: "authorized", Token: sessionToken, Session: sessionAt(time.Now())})
	})
	var shown Device
	// Act
	credential, err := client.Login(context.Background(), func(d Device) error { shown = d; return nil })
	// Assert
	require.NoError(t, err)
	assert.Equal(t, sessionID, credential.SessionID)
	assert.Equal(t, sessionToken, credential.Token)
	require.Len(t, *waits, 1)
	assert.Equal(t, 5*time.Second, (*waits)[0])
	require.Len(t, requests, 1)
	assert.Equal(t, "Bearer "+pollToken, <-requests)
	assert.Equal(t, "https://github.com/login/device", shown.VerificationURI)
	assert.Empty(t, shown.PollSecret)
}

func TestHostedLogin_WithFullTTLAndRequestLatency_UsesReceiveTimeBound(t *testing.T) {
	// Arrange
	now := time.Now().UTC().Truncate(time.Second)
	var elapsed atomic.Int64
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/base/v1/auth/github/device" {
			elapsed.Store(2)
			wire(w, 201, deviceWire{"1", "ABCD-EFGH", "https://github.com/login/device", now.Add(902 * time.Second), 5, pollToken})
			return
		}
		wire(w, 200, pollWire{Version: "1", Status: "authorized", Token: sessionToken, Session: sessionAt(time.Now())})
	})
	reads := 0
	client.now = func() time.Time { reads++; return now.Add(time.Duration(elapsed.Load()) * time.Second) }
	client.wait = func(context.Context, time.Duration) error { return nil }
	// Act
	credential, err := client.Login(context.Background(), func(Device) error { return nil })
	// Assert
	require.NoError(t, err)
	assert.Equal(t, sessionID, credential.SessionID)
	assert.Positive(t, reads)
}

func TestHostedLogin_WithSlowDown_HonorsLargerRetryInterval(t *testing.T) {
	// Arrange
	var polls atomic.Int32
	expiry := time.Now().UTC().Truncate(time.Second).Add(13 * time.Minute)
	client, waits := loginClient(t, func(w http.ResponseWriter, r *http.Request) {
		if polls.Add(1) == 1 {
			w.Header().Set("Retry-After", "20")
			wire(w, 202, pollWire{Version: "1", Status: "slow_down", Interval: 12, ExpiresAt: expiry})
			return
		}
		wire(w, 200, pollWire{Version: "1", Status: "authorized", Token: sessionToken, Session: sessionAt(time.Now())})
	})
	// Act
	_, err := client.Login(context.Background(), func(Device) error { return nil })
	// Assert
	require.NoError(t, err)
	require.Len(t, *waits, 2)
	assert.Equal(t, 5*time.Second, (*waits)[0])
	assert.Equal(t, 20*time.Second, (*waits)[1])
}

func TestHostedLogin_WithPendingResponse_NeverReducesInterval(t *testing.T) {
	// Arrange
	var polls atomic.Int32
	expiry := time.Now().UTC().Truncate(time.Second).Add(13 * time.Minute)
	client, waits := loginClient(t, func(w http.ResponseWriter, r *http.Request) {
		if polls.Add(1) == 1 {
			wire(w, 202, pollWire{Version: "1", Status: "pending", Interval: 1, ExpiresAt: expiry})
			return
		}
		wire(w, 200, pollWire{Version: "1", Status: "authorized", Token: sessionToken, Session: sessionAt(time.Now())})
	})
	// Act
	_, err := client.Login(context.Background(), func(Device) error { return nil })
	// Assert
	require.NoError(t, err)
	require.Len(t, *waits, 2)
	assert.Equal(t, 5*time.Second, (*waits)[1])
}

func TestHostedLogin_WhenCancelledDuringWait_DoesNotPoll(t *testing.T) {
	// Arrange
	client, _ := loginClient(t, func(w http.ResponseWriter, r *http.Request) { wire(w, 500, nil) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client.wait = func(ctx context.Context, d time.Duration) error { cancel(); return waitContext(ctx, d) }
	// Act
	credential, err := client.Login(ctx, func(Device) error { return nil })
	// Assert
	require.ErrorIs(t, err, ErrCancelled)
	assert.Empty(t, credential)
}

func TestHostedLogin_WhenNextPollWouldExceedExpiry_StopsWithoutPolling(t *testing.T) {
	// Arrange
	polls := make(chan struct{}, 1)
	now := time.Now().UTC().Truncate(time.Second)
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/base/v1/auth/github/device" {
			wire(w, 201, deviceWire{"1", "ABCD-EFGH", "https://github.com/login/device", now.Add(time.Second), 5, pollToken})
			return
		}
		polls <- struct{}{}
		wire(w, 500, nil)
	})
	client.now = func() time.Time { return now }
	// Act
	credential, err := client.Login(context.Background(), func(Device) error { return nil })
	// Assert
	require.ErrorIs(t, err, ErrExpired)
	assert.Empty(t, credential)
	assert.Empty(t, polls)
}

func TestHostedLogin_WithForeignVerificationURL_RejectsBeforePrompt(t *testing.T) {
	// Arrange
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire(w, 201, deviceWire{"1", "ABCD-EFGH", "https://attacker.invalid", time.Now().Add(time.Minute), 5, pollToken})
	})
	shown := false
	// Act
	_, err := client.Login(context.Background(), func(Device) error { shown = true; return nil })
	// Assert
	require.ErrorIs(t, err, ErrProtocol)
	assert.False(t, shown)
}

func TestHostedLogin_WithRedirect_DoesNotForwardPollSecret(t *testing.T) {
	// Arrange
	forwarded := make(chan string, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded <- r.Header.Get("Authorization") }))
	defer target.Close()
	client, _ := loginClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL)
		wire(w, 307, map[string]string{"private": "never-echo"})
	})
	// Act
	_, err := client.Login(context.Background(), func(Device) error { return nil })
	// Assert
	require.ErrorIs(t, err, ErrNetwork)
	assert.Empty(t, forwarded)
	assert.NotContains(t, err.Error(), pollToken)
}

func TestHostedLogin_WithOversizedResponse_ReturnsOnlyFixedError(t *testing.T) {
	// Arrange
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.Repeat("private-secret", 2000))
	})
	// Act
	_, err := client.Login(context.Background(), func(Device) error { return nil })
	// Assert
	require.ErrorIs(t, err, ErrProtocol)
	assert.NotContains(t, err.Error(), "private-secret")
}

func TestHostedLogin_WithAccessDenied_DoesNotEchoServerBody(t *testing.T) {
	// Arrange
	client, _ := loginClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire(w, 403, map[string]string{"error": "private-token-and-code"})
	})
	// Act
	_, err := client.Login(context.Background(), func(Device) error { return nil })
	// Assert
	require.ErrorIs(t, err, ErrDenied)
	assert.NotContains(t, err.Error(), "private-token-and-code")
}

func TestHostedClient_WithUntrustedTLSCertificate_RefusesExchange(t *testing.T) {
	// Arrange
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { wire(w, 200, nil) }))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	client, err := NewClient(server.URL, nil)
	require.NoError(t, err)
	defer client.Close()
	// Act
	err = client.Logout(context.Background(), sessionToken)
	// Assert
	require.ErrorIs(t, err, ErrNetwork)
	assert.NotContains(t, err.Error(), sessionToken)
}

func TestHostedClient_WithAmbientProxy_UsesVerifiedDirectTransport(t *testing.T) {
	// Arrange
	t.Setenv("HTTPS_PROXY", "https://user:proxy-secret@proxy.invalid")
	// Act
	client, err := NewClient("https://hub.example/base", nil)
	// Assert
	require.NoError(t, err)
	defer client.Close()
	transport, ok := client.http.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Nil(t, transport.Proxy)
	assert.Nil(t, client.http.Jar)
	require.NotNil(t, transport.TLSClientConfig)
	assert.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.Equal(t, uint16(tls.VersionTLS12), transport.TLSClientConfig.MinVersion)
	assert.Equal(t, 10*time.Second, client.http.Timeout)
	assert.Equal(t, int64(8192), transport.MaxResponseHeaderBytes)
}

func TestHostedMetadata_WithCrossRepoResponse_RejectsClaimedIdentity(t *testing.T) {
	// Arrange
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire(w, 200, map[string]any{"identity": Identity{PrincipalID: principalID, CredentialID: sessionID, TenantID: tenantID, RepoID: sessionID, Scope: "repo:metadata:read"}, "repository": map[string]string{"tenant_id": tenantID, "id": sessionID, "display_name": "private"}})
	})
	r := approvedRemote(client.origin)
	// Act
	identity, err := client.Metadata(context.Background(), r, serviceToken)
	// Assert
	require.ErrorIs(t, err, ErrProtocol)
	assert.Empty(t, identity)
}

func TestHostedMetadata_WithServiceCredential_ReturnsBoundedReadOnlyIdentity(t *testing.T) {
	// Arrange
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire(w, 200, map[string]any{"identity": Identity{PrincipalID: principalID, CredentialID: sessionID, TenantID: tenantID, RepoID: repoID, Scope: "repo:metadata:read"}, "repository": map[string]string{"tenant_id": tenantID, "id": repoID, "display_name": "synthetic"}})
	})
	r := approvedRemote(client.origin)
	// Act
	identity, err := client.Metadata(context.Background(), r, serviceToken)
	// Assert
	require.NoError(t, err)
	assert.Equal(t, principalID, identity.PrincipalID)
	assert.Equal(t, "repo:metadata:read", identity.Scope)
}

func TestHostedLogout_WithMalformedSuccess_DoesNotConfirmRevocation(t *testing.T) {
	// Arrange
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire(w, 200, map[string]string{"version": "2", "status": "logged_out"})
	})
	// Act
	err := client.Logout(context.Background(), sessionToken)
	// Assert
	require.ErrorIs(t, err, ErrProtocol)
}

func TestHostedIdentityJSON_WithActualServiceMetadata_OmitsUnavailableExpiry(t *testing.T) {
	// Arrange
	client, _ := tlsClient(t, func(w http.ResponseWriter, r *http.Request) {
		wire(w, 200, map[string]any{"identity": Identity{PrincipalID: principalID, CredentialID: sessionID, TenantID: tenantID, RepoID: repoID, Scope: "repo:metadata:read"}, "repository": map[string]string{"tenant_id": tenantID, "id": repoID, "display_name": "synthetic"}})
	})
	identity, err := client.Metadata(context.Background(), approvedRemote(client.origin), serviceToken)
	require.NoError(t, err)
	// Act
	data, err := json.Marshal(identity)
	// Assert
	require.NoError(t, err)
	assert.NotContains(t, string(data), "expires_at")
	assert.NotContains(t, string(data), "0001-")
	assert.Contains(t, string(data), `"credential_id"`)
}
