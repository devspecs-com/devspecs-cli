package commands

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/orchestration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDispatchHTTPCommand_WithRealApplyAndNormalService_DispatchesThroughTLS(t *testing.T) {
	binary, taskRepo := os.Getenv("DEVSPECS_HTTP_APPLY_BINARY"), os.Getenv("DEVSPECS_HTTP_APPLY_FIXTURE")
	if binary == "" || taskRepo == "" {
		t.Skip("caller-selected real apply fixture is not configured")
	}
	checkout := t.TempDir()
	git := func(args ...string) string {
		result, err := (orchestration.ExecRunner{}).Run(t.Context(), checkout, "git", args...)
		require.NoError(t, err)
		require.Zero(t, result.ExitCode, string(result.Stderr))
		return string(bytes.TrimSpace(result.Stdout))
	}
	git("init", "--object-format=sha1", "--template=")
	cfg := config.DefaultRepoConfig()
	cfg.Integrations.Orchestration = config.OrchestrationConfig{Provider: "http", Options: map[string]any{"goal": "read_only"}}
	require.NoError(t, config.WriteRepoConfig(checkout, cfg))
	git("add", "-f", ".devspecs/config.yaml")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=.git/hooks", "-c", "commit.gpgsign=false", "commit", "-m", "HTTP command fixture")
	source := git("rev-parse", "HEAD")
	git("remote", "add", "origin", "https://source.invalid/command.git")
	posts := make(chan []byte, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/capabilities" {
			_, _ = io.WriteString(w, `{"schema":"devspecs.dispatch.capabilities","schema_version":2,"contract_revision":"2.0.0-draft.2","provider":"command-fixture","ready":true,"isolation":true,"report":true,"cancellation":true,"verification_profiles":[],"limits":{"max_request_bytes":2097152,"max_inline_bytes":1048576,"max_artifact_bytes":33554432,"max_timeout_seconds":3600,"max_attempts":1},"idempotency_retention_seconds":86400}`)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if err != nil {
			w.WriteHeader(400)
			return
		}
		posts <- raw
		var request struct {
			ID      string    `json:"dispatch_id"`
			Created time.Time `json:"created_at"`
		}
		if json.Unmarshal(raw, &request) != nil {
			w.WriteHeader(400)
			return
		}
		digest := sha256.Sum256(raw)
		_ = json.NewEncoder(w).Encode(map[string]any{"schema": "devspecs.dispatch.handle", "schema_version": 2, "dispatch_id": request.ID, "request_sha256": "sha256:" + hex.EncodeToString(digest[:]), "provider": map[string]any{"name": "command-fixture", "run_id": "command-run"}, "accepted_at": request.Created})
	}))
	t.Cleanup(server.Close)
	trusted := t.TempDir()
	ca := filepath.Join(trusted, "ca.pem")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600))
	profile := map[string]any{"schema": "devspecs.dispatch.http_config", "schema_version": 1, "enabled": true, "read_only": true, "endpoint": server.URL + "/v2", "source_url": "https://source.invalid/command.git", "source_commit": source, "principal_id": "command_fixture", "credential_id": "command_key", "token_env": "HTTP_COMMAND_FIXTURE_TOKEN", "ca_file": ca}
	raw, err := json.Marshal(profile)
	require.NoError(t, err)
	path := filepath.Join(trusted, "profile.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	t.Setenv("DEVSPECS_HTTP_DISPATCH_CONFIG", path)
	t.Setenv("HTTP_COMMAND_FIXTURE_TOKEN", "synthetic-command-token")
	service := orchestration.NewService(t.TempDir())
	bound := &httpCommandApplyService{Service: service, binary: binary}
	cmd := newDispatchCmd(func() (dispatchLifecycle, error) { return bound, nil })
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"task:http-normal-fixture", "--task-repo", taskRepo, "--cwd", checkout, "--agent-provider", "fixture-agent", "--read-only", "--detach"})

	err = cmd.Execute()

	require.NoError(t, err)
	var actual dispatchOutput
	require.NoError(t, json.Unmarshal(output.Bytes(), &actual))
	assert.Equal(t, "http", actual.Dispatch.Provider.Name)
	assert.FileExists(t, actual.DispatchPath)
	require.Len(t, posts, 1)
	var posted struct {
		Target struct {
			Slice string `json:"slice"`
		} `json:"target"`
		Input struct {
			Apply string `json:"apply_payload"`
		} `json:"input"`
	}
	require.NoError(t, json.Unmarshal(<-posts, &posted))
	assert.Equal(t, "H01", posted.Target.Slice)
	assert.Greater(t, len(posted.Input.Apply), 5000)
	assert.Contains(t, posted.Input.Apply, "Do not edit any files")
}

// Select the already-built apply executable without replacing the normal
// Service, registry, Git inspection, transport or state implementation.
type httpCommandApplyService struct {
	*orchestration.Service
	binary string
}

func (s *httpCommandApplyService) Dispatch(ctx context.Context, opts orchestration.DispatchOptions) (orchestration.DispatchHandle, string, error) {
	opts.DSExecutable = s.binary
	return s.Service.Dispatch(ctx, opts)
}
