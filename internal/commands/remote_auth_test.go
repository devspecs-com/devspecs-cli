package commands

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/hosted"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type remoteVaultStub struct {
	credential hosted.Credential
	getErr     error
	setErr     error
	deleteErr  error
	gets       int
	sets       int
	deletes    int
}

func (v *remoteVaultStub) Get(config.RemoteConfig) (hosted.Credential, error) {
	v.gets++
	return v.credential, v.getErr
}
func (v *remoteVaultStub) Set(_ config.RemoteConfig, c hosted.Credential) error {
	v.sets++
	if v.setErr == nil {
		v.credential = c
	}
	return v.setErr
}
func (v *remoteVaultStub) Delete(config.RemoteConfig) error {
	v.deletes++
	if v.deleteErr == nil {
		v.credential = hosted.Credential{}
	}
	return v.deleteErr
}

type remoteAPIStub struct {
	credential       hosted.Credential
	identity         hosted.Identity
	loginErr         error
	metadataErr      error
	logoutErr        error
	logins           int
	metadataReads    int
	logoutTokens     []string
	cleanupCancelled bool
	afterLogin       func()
}

func (a *remoteAPIStub) Login(_ context.Context, notify func(hosted.Device) error) (hosted.Credential, error) {
	a.logins++
	if a.loginErr == nil {
		if err := notify(hosted.Device{UserCode: "ABCD-EFGH", VerificationURI: "https://github.com/login/device"}); err != nil {
			return hosted.Credential{}, err
		}
	}
	if a.afterLogin != nil {
		a.afterLogin()
	}
	return a.credential, a.loginErr
}
func (a *remoteAPIStub) Metadata(context.Context, config.RemoteConfig, string) (hosted.Identity, error) {
	a.metadataReads++
	return a.identity, a.metadataErr
}
func (a *remoteAPIStub) Logout(ctx context.Context, token string) error {
	a.logoutTokens = append(a.logoutTokens, token)
	a.cleanupCancelled = ctx.Err() != nil
	return a.logoutErr
}
func (a *remoteAPIStub) Close() {}

func remoteAuthFixture(t *testing.T) (*remoteAPIStub, *remoteVaultStub, remoteAuthDeps) {
	t.Helper()
	remoteTestDirectory(t, testRemoteConfig)
	session := hosted.Session{PrincipalID: "11111111-1111-4111-8111-111111111111", SessionID: "22222222-2222-4222-8222-222222222222", ExpiresAt: time.Now().Add(time.Hour)}
	a := &remoteAPIStub{credential: hosted.Credential{Token: "shs_" + strings.Repeat("A", 43), Session: session}, identity: hosted.Identity{PrincipalID: session.PrincipalID, SessionID: session.SessionID, ExpiresAt: session.ExpiresAt, TenantID: testRemoteTenant, RepoID: testRemoteRepo, Role: "read_only", Scope: "repo:metadata:read"}}
	v := &remoteVaultStub{getErr: hosted.ErrMissing}
	deps := remoteAuthDeps{client: func(string) (hostedAuthClient, error) { return a, nil }, vault: func() hosted.Vault { return v }, interactive: func() bool { return true }, getenv: func(string) string { return "" }}
	return a, v, deps
}

func authCommand(t *testing.T, logout bool, deps remoteAuthDeps) (*bytes.Buffer, *bytes.Buffer, func(context.Context) error) {
	t.Helper()
	cmd := newRemoteAuthCmd(logout, func() remoteAuthDeps { return deps })
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"hosted", "--confirm-url", "https://hub.example/base", "--json"})
	return &stdout, &stderr, cmd.ExecuteContext
}

func TestRemoteLogin_WithVerifiedSession_SavesOnlyAfterMetadataBinding(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	stdout, stderr, execute := authCommand(t, false, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, a.metadataReads)
	assert.Equal(t, 1, v.sets)
	assert.Equal(t, a.credential.SessionID, v.credential.SessionID)
	assert.Contains(t, stdout.String(), `"status":"authenticated"`)
	assert.NotContains(t, stdout.String(), a.credential.Token)
	assert.NotContains(t, stderr.String(), a.credential.Token)
	assert.Contains(t, stderr.String(), "https://github.com/login/device")
	assert.Contains(t, stderr.String(), "ABCD-EFGH")
	assert.Empty(t, a.logoutTokens)
}

func TestRemoteLogin_WithVaultWriteFailure_CompensatesBeforeLocalCleanup(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	v.setErr = errors.New("private-vault-driver-details")
	stdout, _, execute := authCommand(t, false, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "logout confirmed")
	assert.NotContains(t, err.Error(), "private-vault-driver-details")
	assert.Empty(t, stdout.String())
	require.Len(t, a.logoutTokens, 1)
	assert.Equal(t, a.credential.Token, a.logoutTokens[0])
	assert.Equal(t, 1, v.deletes)
}

func TestRemoteLogin_WithUnconfirmedCompensation_DoesNotDeleteVaultRecord(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	v.setErr = errors.New("private-vault-details")
	a.logoutErr = hosted.ErrNetwork
	_, _, execute := authCommand(t, false, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "revocation is unconfirmed")
	assert.Zero(t, v.deletes)
	assert.NotContains(t, err.Error(), a.credential.Token)
}

func TestRemoteLogin_WithCrossRepoMetadata_RevokesNewSessionWithoutSaving(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	a.identity.RepoID = a.credential.SessionID
	_, _, execute := authCommand(t, false, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "metadata verification failed")
	assert.Zero(t, v.sets)
	require.Len(t, a.logoutTokens, 1)
	assert.Equal(t, a.credential.Token, a.logoutTokens[0])
}

func TestRemoteLogin_WithDifferentReturnedSession_RevokesNewSessionWithoutSaving(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	a.identity.SessionID = testRemoteRepo
	_, _, execute := authCommand(t, false, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.Error(t, err)
	assert.Zero(t, v.sets)
	require.Len(t, a.logoutTokens, 1)
	assert.Equal(t, a.credential.Token, a.logoutTokens[0])
}

func TestRemoteLogin_WithExistingSavedSession_RefusesReplacement(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	v.getErr = nil
	v.credential = a.credential
	_, _, execute := authCommand(t, false, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "explicit ds logout")
	assert.Zero(t, a.logins)
	assert.Zero(t, v.sets)
	assert.Zero(t, v.deletes)
	assert.Equal(t, a.credential.Token, v.credential.Token)
}

func TestRemoteLogin_CancelledAfterSessionIssue_CompensatesWithIndependentContext(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.afterLogin = cancel
	a.metadataErr = hosted.ErrCancelled
	_, _, execute := authCommand(t, false, deps)
	// Act
	err := execute(ctx)
	// Assert
	require.Error(t, err)
	assert.Zero(t, v.sets)
	require.Len(t, a.logoutTokens, 1)
	assert.Equal(t, a.credential.Token, a.logoutTokens[0])
	assert.False(t, a.cleanupCancelled)
}

func TestRemoteLogin_WithUnavailableVault_DoesNotStartDeviceFlow(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	v.getErr = hosted.ErrVault
	_, _, execute := authCommand(t, false, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "no plaintext fallback")
	assert.Zero(t, a.logins)
	assert.Zero(t, a.metadataReads)
}

func TestRemoteLogin_NoninteractiveWithoutPair_DoesNotReadVaultOrStartFlow(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	deps.interactive = func() bool { return false }
	_, _, execute := authCommand(t, false, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.Error(t, err)
	assert.Zero(t, v.gets)
	assert.Zero(t, a.logins)
	assert.Zero(t, a.metadataReads)
}

func remoteAuthStdin(t *testing.T, stdin *os.File) {
	t.Helper()
	original := os.Stdin
	os.Stdin = stdin
	t.Cleanup(func() { os.Stdin = original })
}

func TestRemoteLogin_WithRegularFileStdin_RequiresExplicitPair(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	stdin, err := os.CreateTemp(t.TempDir(), "stdin")
	require.NoError(t, err)
	t.Cleanup(func() { _ = stdin.Close() })
	remoteAuthStdin(t, stdin)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("MSYSTEM", "MINGW64")
	deps.interactive = nativeRemoteAuthDeps().interactive
	stdout, stderr, execute := authCommand(t, false, deps)

	// Act
	err = execute(context.Background())

	// Assert
	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "interactive_required", failure.Code)
	assert.Zero(t, v.gets)
	assert.Zero(t, v.sets)
	assert.Zero(t, v.deletes)
	assert.Zero(t, a.logins)
	assert.Zero(t, a.metadataReads)
	assert.Empty(t, a.logoutTokens)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRemoteLogin_WithOrdinaryPipeStdin_RequiresExplicitPair(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	stdin, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = stdin.Close(); _ = writer.Close() })
	remoteAuthStdin(t, stdin)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("MSYSTEM", "MINGW64")
	deps.interactive = nativeRemoteAuthDeps().interactive
	stdout, stderr, execute := authCommand(t, false, deps)

	// Act
	err = execute(context.Background())

	// Assert
	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "interactive_required", failure.Code)
	assert.Zero(t, v.gets)
	assert.Zero(t, v.sets)
	assert.Zero(t, v.deletes)
	assert.Zero(t, a.logins)
	assert.Zero(t, a.metadataReads)
	assert.Empty(t, a.logoutTokens)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRemoteLogout_WithOrdinaryPipeStdin_RequiresExplicitPair(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	v.getErr = nil
	v.credential = a.credential
	stdin, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = stdin.Close(); _ = writer.Close() })
	remoteAuthStdin(t, stdin)
	deps.interactive = nativeRemoteAuthDeps().interactive
	stdout, stderr, execute := authCommand(t, true, deps)

	// Act
	err = execute(context.Background())

	// Assert
	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "interactive_required", failure.Code)
	assert.Zero(t, v.gets)
	assert.Zero(t, v.sets)
	assert.Zero(t, v.deletes)
	assert.Empty(t, a.logoutTokens)
	assert.Equal(t, a.credential.Token, v.credential.Token)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRemoteLogin_WithoutExactConfirmation_DoesNotReadVault(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	cmd := newRemoteAuthCmd(false, func() remoteAuthDeps { return deps })
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"hosted", "--confirm-url", "https://hub.example"})
	// Act
	err := cmd.Execute()
	// Assert
	require.Error(t, err)
	assert.Zero(t, v.gets)
	assert.Zero(t, a.logins)
}

func TestRemoteLogin_WithOperatorPair_NeverTouchesVaultOrDeviceFlow(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	deps.interactive = func() bool { return false }
	deps.getenv = func(key string) string {
		if key == "DEVSPECS_REMOTE_URL" {
			return "https://hub.example/base"
		}
		return "sph_" + strings.Repeat("A", 43)
	}
	stdout, _, execute := authCommand(t, false, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.NoError(t, err)
	assert.Zero(t, v.gets)
	assert.Zero(t, a.logins)
	assert.Equal(t, 1, a.metadataReads)
	assert.Contains(t, stdout.String(), `"credential_source":"environment"`)
}

func TestRemoteLogin_WithMismatchedOperatorURL_DoesNotFallBackToVault(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	deps.getenv = func(key string) string {
		if key == "DEVSPECS_REMOTE_URL" {
			return "https://hub.example/other"
		}
		return "sph_" + strings.Repeat("A", 43)
	}
	_, _, execute := authCommand(t, false, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.Error(t, err)
	assert.Zero(t, v.gets)
	assert.Zero(t, a.metadataReads)
	assert.Zero(t, a.logins)
}

func TestRemoteLogout_WithFailedServerResponse_KeepsSavedCredential(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	v.getErr = nil
	v.credential = a.credential
	a.logoutErr = hosted.ErrNetwork
	stdout, _, execute := authCommand(t, true, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "saved credential retained")
	assert.Empty(t, stdout.String())
	assert.Zero(t, v.deletes)
	assert.Equal(t, a.credential.Token, v.credential.Token)
}

func TestRemoteLogout_WithConfirmedServerResponse_RemovesSavedCredential(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	v.getErr = nil
	v.credential = a.credential
	stdout, _, execute := authCommand(t, true, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, v.deletes)
	assert.Empty(t, v.credential)
	assert.Contains(t, stdout.String(), `"status":"logged_out"`)
	require.Len(t, a.logoutTokens, 1)
}

func TestRemoteLogout_WithVaultDeleteFailure_ReportsRetryWithoutFalseSuccess(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	v.getErr = nil
	v.credential = a.credential
	v.deleteErr = hosted.ErrVault
	stdout, _, execute := authCommand(t, true, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "server logout confirmed")
	assert.ErrorContains(t, err, "retry explicit ds logout")
	assert.Empty(t, stdout.String())
	assert.Equal(t, a.credential.Token, v.credential.Token)
}

func TestRemoteLogout_WithHeadlessServiceToken_RequiresOperatorRevocation(t *testing.T) {
	// Arrange
	a, v, deps := remoteAuthFixture(t)
	deps.interactive = func() bool { return false }
	deps.getenv = func(key string) string {
		if key == "DEVSPECS_REMOTE_URL" {
			return "https://hub.example/base"
		}
		return "sph_" + strings.Repeat("A", 43)
	}
	_, _, execute := authCommand(t, true, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "operator revocation")
	assert.Empty(t, a.logoutTokens)
	assert.Zero(t, v.gets)
}

func TestRemoteAuthHelp_DoesNotConstructAuthenticationDependencies(t *testing.T) {
	// Arrange
	calls := 0
	cmd := newRemoteAuthCmd(false, func() remoteAuthDeps { calls++; return remoteAuthDeps{} })
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"--help"})
	// Act
	err := cmd.Execute()
	// Assert
	require.NoError(t, err)
	assert.Zero(t, calls)
}
