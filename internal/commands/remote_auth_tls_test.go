package commands

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/hosted"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRemoteLogin_WithRealTLSClient_StoresVerifiedBasePathSession(t *testing.T) {
	// Arrange
	root := remoteTestDirectory(t, "")
	token := "shs_" + strings.Repeat("A", 43)
	pollSecret := "shp_" + strings.Repeat("A", 43)
	session := hosted.Session{PrincipalID: "11111111-1111-4111-8111-111111111111", SessionID: "22222222-2222-4222-8222-222222222222", ExpiresAt: time.Now().UTC().Add(time.Hour)}
	type observedRequest struct {
		Path   string
		Token  string
		Method string
	}
	requests := make(chan observedRequest, 8)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- observedRequest{r.URL.Path, r.Header.Get("Authorization"), r.Method}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/base/v1/auth/github/device":
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "1", "user_code": "ABCD-EFGH", "verification_uri": "https://github.com/login/device", "expires_at": time.Now().UTC().Add(time.Minute), "interval": 1, "poll_secret": pollSecret})
		case "/base/v1/auth/github/device/poll":
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "1", "status": "authorized", "session_token": token, "session": session})
		case "/base/v1/tenants/" + testRemoteTenant + "/repos/" + testRemoteRepo:
			_ = json.NewEncoder(w).Encode(map[string]any{"identity": hosted.Identity{PrincipalID: session.PrincipalID, SessionID: session.SessionID, ExpiresAt: session.ExpiresAt, TenantID: testRemoteTenant, RepoID: testRemoteRepo, Role: "read_only", Scope: "repo:metadata:read"}, "repository": map[string]string{"tenant_id": testRemoteTenant, "id": testRemoteRepo, "display_name": "synthetic"}})
		default:
			w.WriteHeader(403)
		}
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	r := config.RemoteConfig{Version: 1, Name: "hosted", URL: server.URL + "/base", TenantID: testRemoteTenant, RepoID: testRemoteRepo}
	cfg := config.DefaultRepoConfig()
	cfg.Remotes = []config.RemoteConfig{r}
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".devspecs"), 0o755))
	require.NoError(t, os.WriteFile(config.RepoConfigPath(root), data, 0o644))
	vault := &remoteVaultStub{getErr: hosted.ErrMissing}
	deps := remoteAuthDeps{client: func(url string) (hostedAuthClient, error) { return hosted.NewClient(url, roots) }, vault: func() hosted.Vault { return vault }, interactive: func() bool { return true }, getenv: func(string) string { return "" }}
	cmd := newRemoteAuthCmd(false, func() remoteAuthDeps { return deps })
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"hosted", "--confirm-url", r.URL, "--json"})
	// Act
	err = cmd.Execute()
	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, vault.sets)
	assert.Equal(t, session.SessionID, vault.credential.SessionID)
	assert.Equal(t, session.PrincipalID, vault.credential.PrincipalID)
	var result remoteResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Equal(t, 1, result.SchemaVersion)
	assert.Equal(t, "authenticated", result.Status)
	require.NotNil(t, result.Identity)
	assert.Equal(t, "read_only", result.Identity.Role)
	assert.NotContains(t, stdout.String(), token)
	assert.NotContains(t, stderr.String(), token)
	assert.NotContains(t, stdout.String(), pollSecret)
	assert.NotContains(t, stderr.String(), pollSecret)
	require.Len(t, requests, 3)
	first := <-requests
	second := <-requests
	third := <-requests
	assert.Equal(t, "/base/v1/auth/github/device", first.Path)
	assert.Empty(t, first.Token)
	assert.Equal(t, http.MethodPost, first.Method)
	assert.Equal(t, "/base/v1/auth/github/device/poll", second.Path)
	assert.Equal(t, "Bearer "+pollSecret, second.Token)
	assert.Equal(t, "/base/v1/tenants/"+testRemoteTenant+"/repos/"+testRemoteRepo, third.Path)
	assert.Equal(t, "Bearer "+token, third.Token)
}
