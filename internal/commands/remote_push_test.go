package commands

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/hosted"
	"github.com/devspecs-com/devspecs-cli/internal/hosted/snapshot"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemotePush_WithExplicitEnvironmentPair_PublishesCompleteCommittedSubsetWithoutRead(t *testing.T) {
	f := newRemotePushFixture(t)
	command, stdout := f.command(false)

	err := command.Execute()

	require.NoError(t, err)
	f.assertSchema(t, stdout.Bytes())
	require.Len(t, f.requests, 2)
	assert.Equal(t, "GET "+f.route()+"/policy", <-f.requests)
	assert.Equal(t, "POST "+f.route(), <-f.requests)
	require.Len(t, f.bodies, 1)
	manifest, decodeErr := snapshot.Decode(bytes.NewReader(<-f.bodies))
	require.NoError(t, decodeErr)
	require.Len(t, manifest.Records, 5)
	assert.Equal(t, f.commit, manifest.Commit)
	assert.Equal(t, "refs/heads/main", manifest.Ref)
	assert.Zero(t, f.vault.gets)
	assert.Zero(t, f.vault.sets)
	assert.Zero(t, f.unexpected.Load())
	var result remoteResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Equal(t, "published", result.Status)
	assert.Equal(t, f.publishedURL(), result.URL)
	assert.Equal(t, "environment", result.CredentialSource)
}

func TestRemotePush_WithInteractiveVaultSession_UsesSavedCredentialWithoutLoginOrGrant(t *testing.T) {
	f := newRemotePushFixture(t)
	f.token = "shs_" + strings.Repeat("A", 43)
	f.vault.credential = hosted.Credential{Token: f.token, Session: hosted.Session{PrincipalID: "11111111-1111-4111-8111-111111111111", SessionID: "22222222-2222-4222-8222-222222222222", ExpiresAt: time.Now().Add(time.Hour)}}
	command, stdout := f.command(true)

	err := command.Execute()

	require.NoError(t, err)
	f.assertSchema(t, stdout.Bytes())
	assert.Equal(t, 1, f.vault.gets)
	assert.Zero(t, f.vault.sets)
	assert.Zero(t, f.vault.deletes)
	assert.Zero(t, f.unexpected.Load())
	var result remoteResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Equal(t, "os_vault", result.CredentialSource)
}

func TestRemotePush_WithoutExactConfirmation_StopsBeforeCredentialFactoryOrNetwork(t *testing.T) {
	f := newRemotePushFixture(t)
	factoryCalls := 0
	command := newRemotePushCmd(func() remotePushDeps { factoryCalls++; return f.deps(false) })
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{"hosted", "--commit", f.commit, "--ref", "refs/heads/main", "--confirm-url", "https://different.invalid"})

	err := command.Execute()

	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	f.assertErrorSchema(t, err)
	assert.Equal(t, "confirmation_required", failure.Code)
	assert.Zero(t, factoryCalls)
	assert.Empty(t, f.requests)
}

func TestRemotePush_WithNonTTYAndNoEnvironmentPair_DoesNotUseVaultOrImplicitLogin(t *testing.T) {
	f := newRemotePushFixture(t)
	deps := f.deps(false)
	deps.getenv = func(string) string { return "" }
	command := newRemotePushCmd(func() remotePushDeps { return deps })
	command.SetOut(&bytes.Buffer{})
	command.SetArgs(f.args())

	err := command.Execute()

	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	f.assertErrorSchema(t, err)
	assert.Equal(t, "interactive_required", failure.Code)
	assert.Zero(t, f.vault.gets)
	assert.Empty(t, f.requests)
}

func TestRemotePush_WithPartialEnvironmentPair_DoesNotFallBackToInteractiveVault(t *testing.T) {
	f := newRemotePushFixture(t)
	deps := f.deps(true)
	deps.getenv = func(name string) string {
		if name == "DEVSPECS_REMOTE_URL" {
			return f.origin
		}
		return ""
	}
	command := newRemotePushCmd(func() remotePushDeps { return deps })
	command.SetOut(&bytes.Buffer{})
	command.SetArgs(f.args())

	err := command.Execute()

	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	f.assertErrorSchema(t, err)
	assert.Equal(t, "credential_binding_failed", failure.Code)
	assert.Zero(t, f.vault.gets)
	assert.Empty(t, f.requests)
}

func TestRemotePush_WithVaultUnavailable_DoesNotCreatePlaintextFallback(t *testing.T) {
	f := newRemotePushFixture(t)
	f.vault.getErr = hosted.ErrVault
	command, _ := f.command(true)

	err := command.Execute()

	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	f.assertErrorSchema(t, err)
	assert.Equal(t, "vault_unavailable", failure.Code)
	assert.Zero(t, f.vault.sets)
	assert.Empty(t, f.requests)
}

func TestRemotePush_WithMetadataOnlySession_DoesNotUpgradeContentCapability(t *testing.T) {
	f := newRemotePushFixture(t)
	f.mode = "metadata-only"
	command, stdout := f.command(false)

	err := command.Execute()

	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	f.assertErrorSchema(t, err)
	assert.Equal(t, "access_denied", failure.Code)
	require.Len(t, f.requests, 1)
	assert.Equal(t, "GET "+f.route()+"/policy", <-f.requests)
	assert.Empty(t, f.bodies)
	assert.Empty(t, stdout.String())
	assert.Zero(t, f.unexpected.Load())
}

func TestRemotePush_WithUnknownCommittedOwnerMember_FailsWholeBeforePost(t *testing.T) {
	f := newRemotePushFixture(t)
	name := "custom/planning/y03-real-fixture/unrecognized.json"
	require.NoError(t, os.WriteFile(filepath.Join(f.root, filepath.FromSlash(name)), []byte("{}\n"), 0o600))
	pushFixtureGit(t, f.root, "add", name)
	pushFixtureGit(t, f.root, "commit", "-m", "unsupported source member")
	f.commit = pushFixtureGit(t, f.root, "rev-parse", "HEAD")
	command, _ := f.command(false)

	err := command.Execute()

	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	f.assertErrorSchema(t, err)
	assert.Equal(t, "unsupported_schema", failure.Code)
	assert.Empty(t, f.bodies)
	require.Len(t, f.requests, 1)
}

func TestRemotePush_WithShortCommit_RejectsBeforeCredentialAccess(t *testing.T) {
	f := newRemotePushFixture(t)
	f.commit = "153f009"
	command, _ := f.command(false)

	err := command.Execute()

	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	f.assertErrorSchema(t, err)
	assert.Equal(t, "invalid_source", failure.Code)
	assert.Empty(t, f.requests)
	assert.Zero(t, f.vault.gets)
}

func TestRemotePush_WithHumanOutput_PrintsOnlyValidatedSnapshotURL(t *testing.T) {
	f := newRemotePushFixture(t)
	command := newRemotePushCmd(func() remotePushDeps { return f.deps(false) })
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	args := f.args()
	command.SetArgs(args[:len(args)-1])

	err := command.Execute()

	require.NoError(t, err)
	assert.Equal(t, f.publishedURL()+"\n", stdout.String())
	assert.NotContains(t, stdout.String(), f.token)
}

func TestRemotePushRegistration_WithPublicRemoteCommand_ExposesOnlyLocalPublishingFlags(t *testing.T) {
	remote := NewRemoteCmd()

	command, remaining, err := remote.Find([]string{"push", "hosted"})

	require.NoError(t, err)
	assert.Equal(t, "push", command.Name())
	require.Len(t, remaining, 1)
	assert.Equal(t, "hosted", remaining[0])
	assert.NotNil(t, command.Flags().Lookup("commit"))
	assert.NotNil(t, command.Flags().Lookup("ref"))
	assert.NotNil(t, command.Flags().Lookup("confirm-url"))
	assert.NotNil(t, command.Flags().Lookup("json"))
	assert.Nil(t, remote.PersistentFlags().Lookup("commit"))
	assert.Nil(t, remote.PersistentFlags().Lookup("confirm-url"))
}

func TestRemotePush_WhenCancelledAfterPost_ReportsAmbiguousPublishingWithoutRetry(t *testing.T) {
	f := newRemotePushFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	f.mode, f.cancel = "cancel-after-post", cancel
	command, stdout := f.command(false)

	err := command.ExecuteContext(ctx)

	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	f.assertErrorSchema(t, err)
	assert.Equal(t, "cancelled", failure.Code)
	assert.Contains(t, failure.Message, "started POST may already have succeeded")
	assert.Contains(t, failure.Message, "no automatic retry")
	assert.NotContains(t, failure.Message, "authentication")
	assert.Empty(t, stdout.String())
	require.Len(t, f.requests, 2)
	require.Len(t, f.bodies, 1)
}

type remotePushFixture struct {
	schema                            *jsonschema.Schema
	root, commit, origin, token, mode string
	server                            *httptest.Server
	roots                             *x509.CertPool
	vault                             *remoteVaultStub
	requests                          chan string
	bodies                            chan []byte
	unexpected                        atomic.Int32
	cancel                            context.CancelFunc
}

func newRemotePushFixture(t *testing.T) *remotePushFixture {
	t.Helper()
	schema := localRemoteSchema(t, "remote-result.v1.schema.json")
	source, err := filepath.Abs(filepath.Join("..", "hosted", "snapshot", "testdata", "real-cli"))
	require.NoError(t, err)
	f := &remotePushFixture{root: t.TempDir(), token: "sph_" + strings.Repeat("A", 43), vault: &remoteVaultStub{}, requests: make(chan string, 8), bodies: make(chan []byte, 2)}
	f.schema = schema
	pushFixtureGit(t, f.root, "init", "--initial-branch=main", "--template=")
	pushFixtureGit(t, f.root, "--work-tree="+source, "add", "--all", "--", ".")
	pushFixtureGit(t, f.root, "commit", "-m", "synthetic real CLI fixture")
	pushFixtureGit(t, f.root, "restore", "--source=HEAD", "--worktree", "--", ".")
	f.server = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	f.origin = f.server.URL + "/base"
	f.roots = x509.NewCertPool()
	f.roots.AddCert(f.server.Certificate())
	cfg := config.DefaultRepoConfig()
	cfg.Remotes = []config.RemoteConfig{{Version: 1, Name: "hosted", URL: f.origin, TenantID: testRemoteTenant, RepoID: testRemoteRepo}}
	require.NoError(t, config.WriteRepoConfig(f.root, cfg))
	pushFixtureGit(t, f.root, "add", "-f", ".devspecs/config.yaml")
	pushFixtureGit(t, f.root, "commit", "-m", "explicit remote metadata")
	f.commit = pushFixtureGit(t, f.root, "rev-parse", "HEAD")
	t.Chdir(f.root)
	t.Setenv("DEVSPECS_TELEMETRY", "0")
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	return f
}

func (f *remotePushFixture) assertSchema(t *testing.T, raw []byte) {
	t.Helper()
	var instance any
	require.NoError(t, json.Unmarshal(raw, &instance))
	require.NoError(t, f.schema.Validate(instance))
}

func (f *remotePushFixture) assertErrorSchema(t *testing.T, err error) {
	t.Helper()
	var stderr bytes.Buffer
	PrintCommandError(&stderr, err, []string{"remote", "push", "hosted", "--json"})
	f.assertSchema(t, stderr.Bytes())
	assert.NotContains(t, stderr.String(), f.token)
}

func pushFixtureGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	options := []string{"-C", root, "-c", "core.hooksPath=" + os.DevNull, "-c", "core.autocrlf=false", "-c", "user.name=Synthetic", "-c", "user.email=synthetic@example.invalid", "-c", "commit.gpgsign=false"}
	cmd := exec.Command("git", append(options, args...)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "SystemRoot=" + os.Getenv("SystemRoot"), "WINDIR=" + os.Getenv("WINDIR"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull, "GIT_AUTHOR_DATE=2026-11-13T12:00:00Z", "GIT_COMMITTER_DATE=2026-11-13T12:00:00Z"}
	raw, err := cmd.CombinedOutput()
	require.NoError(t, err, string(raw))
	return strings.TrimSpace(string(raw))
}

func (f *remotePushFixture) route() string {
	return "/base/v1/tenants/" + testRemoteTenant + "/repos/" + testRemoteRepo + "/snapshots"
}
func (f *remotePushFixture) publishedURL() string {
	return f.origin + strings.TrimPrefix(f.route(), "/base") + "/66666666-6666-4666-8666-666666666666"
}
func (f *remotePushFixture) args() []string {
	return []string{"hosted", "--commit", f.commit, "--ref", "refs/heads/main", "--confirm-url", f.origin, "--json"}
}

func (f *remotePushFixture) deps(interactive bool) remotePushDeps {
	return remotePushDeps{client: func(origin string) (remoteSnapshotClient, error) { return hosted.NewClient(origin, f.roots) }, vault: func() hosted.Vault { return f.vault }, interactive: func() bool { return interactive }, getenv: func(name string) string {
		if interactive {
			return ""
		}
		if name == "DEVSPECS_REMOTE_URL" {
			return f.origin
		}
		if name == "DEVSPECS_REMOTE_TOKEN" {
			return f.token
		}
		return ""
	}}
}

func (f *remotePushFixture) command(interactive bool) (*cobra.Command, *bytes.Buffer) {
	command := newRemotePushCmd(func() remotePushDeps { return f.deps(interactive) })
	command.SilenceErrors, command.SilenceUsage = true, true
	stdout := &bytes.Buffer{}
	command.SetOut(stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(f.args())
	return command, stdout
}

func (f *remotePushFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.requests <- r.Method + " " + r.URL.Path
	w.Header().Set("Content-Type", "application/json")
	if r.Header.Get("Authorization") != "Bearer "+f.token {
		w.WriteHeader(403)
		_, _ = io.WriteString(w, `{"schema_version":1,"error":{"code":"access_denied"}}`)
		return
	}
	if r.URL.Path == f.route()+"/policy" && r.Method == http.MethodGet {
		if f.mode == "metadata-only" {
			w.WriteHeader(403)
			_, _ = io.WriteString(w, `{"schema_version":1,"error":{"code":"access_denied"}}`)
			return
		}
		_ = json.NewEncoder(w).Encode(snapshot.Policy{Schema: "devspecs.hosted.snapshot.policy", SchemaVersion: 1, PolicyID: "55555555-5555-4555-8555-555555555555", Roots: []snapshot.Root{{Label: "planning", Kind: "task_parent", Path: "custom/planning"}, {Label: "decisions", Kind: "adr", Path: "architecture/decisions"}, {Label: "normal", Kind: "task_parent", Path: "devspecs/tasks"}}})
		return
	}
	if r.URL.Path == f.route() && r.Method == http.MethodPost {
		raw, err := io.ReadAll(io.LimitReader(r.Body, snapshot.MaxBodyBytes+1))
		if err != nil {
			w.WriteHeader(400)
			return
		}
		f.bodies <- raw
		if f.mode == "cancel-after-post" {
			f.cancel()
			<-r.Context().Done()
			return
		}
		if f.mode == "conflict" {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"schema_version":1,"error":{"code":"record_conflict"}}`)
			return
		}
		_ = json.NewEncoder(w).Encode(hosted.PublishedSnapshot{SchemaVersion: 1, URL: f.publishedURL()})
		return
	}
	f.unexpected.Add(1)
	w.WriteHeader(403)
	_, _ = io.WriteString(w, `{"schema_version":1,"error":{"code":"access_denied"}}`)
}
