package commands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/hosted"
	"github.com/gofrs/flock"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

type hostedAuthClient interface {
	Login(context.Context, func(hosted.Device) error) (hosted.Credential, error)
	Metadata(context.Context, config.RemoteConfig, string) (hosted.Identity, error)
	Logout(context.Context, string) error
	Close()
}

type remoteAuthDeps struct {
	client      func(string) (hostedAuthClient, error)
	vault       func() hosted.Vault
	interactive func() bool
	getenv      func(string) string
}

func nativeRemoteAuthDeps() remoteAuthDeps {
	return remoteAuthDeps{
		client: func(url string) (hostedAuthClient, error) { return hosted.NewClient(url, nil) },
		vault:  func() hosted.Vault { return hosted.OSVault{} },
		interactive: func() bool {
			fd := os.Stdin.Fd()
			return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
		}, getenv: os.Getenv,
	}
}

func NewLoginCmd() *cobra.Command  { return newRemoteAuthCmd(false, nativeRemoteAuthDeps) }
func NewLogoutCmd() *cobra.Command { return newRemoteAuthCmd(true, nativeRemoteAuthDeps) }

func newRemoteAuthCmd(logout bool, factory func() remoteAuthDeps) *cobra.Command {
	name, description := "login", "Authenticate an explicitly selected hosted remote"
	if logout {
		name, description = "logout", "Revoke a hosted session before removing its saved credential"
	}
	var confirm string
	var asJSON bool
	cmd := &cobra.Command{Use: name + " <remote>", Short: description, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		r, err := configuredRemote(args[0])
		if err != nil {
			return err
		}
		deps := factory()
		token, paired, err := hosted.Environment(r, deps.getenv)
		if err != nil {
			return remoteError("credential_binding_failed", "explicit remote URL/token environment pair is missing, invalid or does not match the exact configured base URL")
		}
		if paired {
			if logout && !hosted.ValidToken(token, "shs_") {
				return remoteError("operator_revocation_required", "service tokens require hub operator revocation; no credential was removed")
			}
			client, err := deps.client(r.URL)
			if err != nil {
				return safeHostedError(err)
			}
			defer client.Close()
			if logout {
				if err := client.Logout(cmd.Context(), token); err != nil {
					return logoutFailure(err)
				}
				return outputRemote(cmd, asJSON, remoteResult{Status: "logged_out", Remote: &r, CredentialSource: "environment"})
			}
			identity, err := client.Metadata(cmd.Context(), r, token)
			if err != nil {
				return safeHostedError(err)
			}
			return outputRemote(cmd, asJSON, remoteResult{Status: "authenticated", Remote: &r, Identity: &identity, CredentialSource: "environment"})
		}
		if !deps.interactive() {
			return remoteError("interactive_required", "noninteractive authentication requires the explicit remote URL/token environment pair; OS vault and device login were not used")
		}
		if confirm != r.URL {
			return remoteError("confirmation_required", "confirm the exact configured HTTPS base URL with --confirm-url before accessing the OS vault or starting authentication")
		}
		unlock, err := lockRemoteAuthentication(r)
		if err != nil {
			return err
		}
		defer unlock()
		vault := deps.vault()
		existing, err := vault.Get(r)
		if logout {
			if err != nil {
				return safeHostedError(err)
			}
			client, err := deps.client(r.URL)
			if err != nil {
				return safeHostedError(err)
			}
			defer client.Close()
			if err := client.Logout(cmd.Context(), existing.Token); err != nil {
				return logoutFailure(err)
			}
			if vault.Delete(r) != nil {
				return remoteError("vault_delete_failed", "server logout confirmed but saved credential removal failed; retry explicit ds logout for this remote")
			}
			return outputRemote(cmd, asJSON, remoteResult{Status: "logged_out", Remote: &r, CredentialSource: "os_vault"})
		}
		if err == nil {
			return remoteError("logout_required", "a saved session already exists; explicit ds logout must succeed before another device login")
		}
		if !errors.Is(err, hosted.ErrMissing) {
			return safeHostedError(err)
		}
		if cmd.Context().Err() != nil {
			return safeHostedError(hosted.ErrCancelled)
		}
		client, err := deps.client(r.URL)
		if err != nil {
			return safeHostedError(err)
		}
		defer client.Close()
		credential, err := client.Login(cmd.Context(), func(device hosted.Device) error {
			_, err := fmt.Fprintf(cmd.ErrOrStderr(), "%s\nCode: %s\n", device.VerificationURI, device.UserCode)
			return err
		})
		if err != nil {
			return safeHostedError(err)
		}
		identity, err := client.Metadata(cmd.Context(), r, credential.Token)
		if err != nil || identity.PrincipalID != credential.PrincipalID || identity.SessionID != credential.SessionID || !identity.ExpiresAt.Equal(credential.ExpiresAt) || identity.TenantID != r.TenantID || identity.RepoID != r.RepoID {
			return compensateSession(cmd.Context(), client, credential.Token, "binding_verification_failed", "new session metadata verification failed; compensating server logout confirmed")
		}
		if cmd.Context().Err() != nil {
			return compensateSession(cmd.Context(), client, credential.Token, "cancelled", "login cancelled; compensating server logout confirmed")
		}
		if vault.Set(r, credential) != nil {
			failure := compensateSession(cmd.Context(), client, credential.Token, "credential_storage_failed", "credential storage failed; compensating server logout confirmed; retry explicit login")
			var known remoteFailure
			if errors.As(failure, &known) && known.Code != "cleanup_unconfirmed" {
				if vault.Delete(r) != nil {
					return remoteError("vault_cleanup_failed", "new server session revoked but vault cleanup failed; retry explicit ds logout before logging in")
				}
			}
			return failure
		}
		return outputRemote(cmd, asJSON, remoteResult{Status: "authenticated", Remote: &r, Identity: &identity, CredentialSource: "os_vault"})
	}}
	cmd.Flags().StringVar(&confirm, "confirm-url", "", "Confirm the exact HTTPS base URL for interactive OS-vault authentication")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output versioned JSON; errors are JSON on stderr")
	return cmd
}

// Serialize cooperating interactive clients so simultaneous logins cannot
// replace one another's vault entry and silently orphan a server session.
func lockRemoteAuthentication(r config.RemoteConfig) (func(), error) {
	home, err := config.HomeDir()
	if err != nil {
		return nil, remoteError("auth_lock_failed", "unable to locate interactive authentication lock")
	}
	dir := filepath.Join(home, "remote-auth")
	if os.MkdirAll(dir, 0o755) != nil {
		return nil, remoteError("auth_lock_failed", "unable to prepare interactive authentication lock")
	}
	hash := sha256.Sum256([]byte(r.URL + "\n" + r.TenantID + "\n" + r.RepoID))
	lock := flock.New(filepath.Join(dir, hex.EncodeToString(hash[:])+".lock"))
	ok, err := lock.TryLock()
	if err != nil || !ok {
		_ = lock.Close()
		return nil, remoteError("auth_busy", "another interactive authentication operation is active for this binding; retry after it finishes")
	}
	return func() { _ = lock.Close() }, nil
}

func compensateSession(ctx context.Context, client hostedAuthClient, token, code, message string) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if client.Logout(cleanup, token) != nil {
		return remoteError("cleanup_unconfirmed", "new session could not be saved or verified and remote revocation is unconfirmed; contact the hub operator to revoke the outstanding session before retrying")
	}
	return remoteError(code, message)
}

func logoutFailure(err error) error {
	if errors.Is(err, hosted.ErrCancelled) {
		return remoteError("logout_unconfirmed", "logout cancelled before confirmation; saved credential retained; retry explicit ds logout")
	}
	return remoteError("logout_unconfirmed", "server logout was not confirmed; saved credential retained; retry explicit ds logout or contact the hub operator")
}

func safeHostedError(err error) error {
	switch {
	case errors.Is(err, hosted.ErrDenied):
		return remoteError("access_denied", "hosted access denied; verify enrollment, role and expiry with the hub operator")
	case errors.Is(err, hosted.ErrCancelled):
		return remoteError("cancelled", "hosted authentication cancelled")
	case errors.Is(err, hosted.ErrExpired):
		return remoteError("login_expired", "device login expired; retry explicit ds login")
	case errors.Is(err, hosted.ErrMissing):
		return remoteError("credential_missing", "no saved credential for this exact remote binding")
	case errors.Is(err, hosted.ErrVault):
		return remoteError("vault_unavailable", "OS vault unavailable; no plaintext fallback; retry with an available vault or an explicit operator environment pair")
	case errors.Is(err, hosted.ErrCredential):
		return remoteError("credential_invalid", "saved or supplied credential is invalid for this exact remote binding")
	case errors.Is(err, hosted.ErrProtocol):
		return remoteError("invalid_response", "invalid hosted version-1 response")
	default:
		return remoteError("exchange_failed", "hosted exchange failed; no credential was disclosed in diagnostics")
	}
}
