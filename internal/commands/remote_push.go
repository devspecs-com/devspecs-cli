package commands

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/hosted"
	"github.com/devspecs-com/devspecs-cli/internal/hosted/snapshot"
	"github.com/spf13/cobra"
)

type remoteSnapshotClient interface {
	SnapshotPolicy(context.Context, config.RemoteConfig, string) (snapshot.Policy, error)
	PublishSnapshot(context.Context, config.RemoteConfig, string, snapshot.Snapshot) (hosted.PublishedSnapshot, error)
	Close()
}

type remotePushDeps struct {
	client      func(string) (remoteSnapshotClient, error)
	vault       func() hosted.Vault
	interactive func() bool
	getenv      func(string) string
}

func nativeRemotePushDeps() remotePushDeps {
	auth := nativeRemoteAuthDeps()
	return remotePushDeps{client: func(origin string) (remoteSnapshotClient, error) { return hosted.NewClient(origin, nil) }, vault: auth.vault, interactive: auth.interactive, getenv: auth.getenv}
}

func newRemotePushCmd(factory func() remotePushDeps) *cobra.Command {
	var commit, ref, confirm string
	var asJSON bool
	cmd := &cobra.Command{Use: "push <remote>", Short: "Publish the complete approved committed record subset to one confirmed hosted remote", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !snapshot.ValidCommit(commit) || !snapshot.ValidRef(ref) {
			return remoteError("invalid_source", "push requires a full lowercase commit OID and an approved refs/heads/... ref")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return remoteError("repository_required", "unable to locate the local Git repository")
		}
		root, err := snapshot.RepositoryRoot(cmd.Context(), cwd)
		if err != nil {
			return safeSnapshotError(err)
		}
		document, err := loadRemoteDocumentFile(config.RepoConfigPath(root))
		if err != nil {
			return err
		}
		var remote config.RemoteConfig
		found := false
		for _, candidate := range document.cfg.Remotes {
			if candidate.Name == args[0] {
				remote = candidate
				found = true
				break
			}
		}
		if !found {
			return remoteError("remote_missing", "named hosted remote is not configured")
		}
		if confirm != remote.URL {
			return remoteError("confirmation_required", "push requires --confirm-url with the exact configured HTTPS base URL before credential access or network use")
		}
		deps := factory()
		token, paired, err := hosted.Environment(remote, deps.getenv)
		if err != nil {
			return remoteError("credential_binding_failed", "explicit remote URL/token environment pair is invalid or does not match the exact configured base URL")
		}
		credentialSource := "environment"
		if !paired {
			if !deps.interactive() {
				return remoteError("interactive_required", "noninteractive push requires the explicit remote URL/token environment pair; OS vault and login were not used")
			}
			credential, err := deps.vault().Get(remote)
			if err != nil {
				return safeHostedError(err)
			}
			if !hosted.ValidToken(credential.Token, "shs_") || !credential.ExpiresAt.After(time.Now()) {
				return safeHostedError(hosted.ErrCredential)
			}
			token, credentialSource = credential.Token, "os_vault"
		}
		if cmd.Context().Err() != nil {
			return safeSnapshotError(cmd.Context().Err())
		}
		client, err := deps.client(remote.URL)
		if err != nil {
			return safeHostedError(err)
		}
		defer client.Close()
		policy, err := client.SnapshotPolicy(cmd.Context(), remote, token)
		if err != nil {
			return safeSnapshotError(err)
		}
		manifest, err := snapshot.Collect(cmd.Context(), root, remote.RepoID, policy, commit, ref)
		if err != nil {
			return safeSnapshotError(err)
		}
		published, err := client.PublishSnapshot(cmd.Context(), remote, token, manifest)
		if err != nil {
			return safeSnapshotError(err)
		}
		return outputRemote(cmd, asJSON, remoteResult{Status: "published", Remote: &remote, URL: published.URL, CredentialSource: credentialSource})
	}}
	cmd.Flags().StringVar(&commit, "commit", "", "Full lowercase immutable commit OID to publish")
	cmd.Flags().StringVar(&ref, "ref", "", "Operator-approved refs/heads/... containing the commit")
	cmd.Flags().StringVar(&confirm, "confirm-url", "", "Confirm the exact configured HTTPS base URL before publishing content")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output versioned JSON; errors are JSON on stderr")
	return cmd
}

func safeSnapshotError(err error) error {
	var refusal *hosted.SnapshotError
	if errors.As(err, &refusal) {
		switch refusal.Code {
		case "record_conflict":
			return remoteError("record_conflict", "hosted immutable record identity conflicts; no automatic retry or source change was attempted")
		case "payload_too_large":
			return remoteError("snapshot_too_large", "snapshot exceeds the supported publishing bounds")
		case "unsupported_schema":
			return remoteError("unsupported_schema", "the approved subset contains an unsupported record schema; nothing was silently omitted")
		case "unverified_provenance":
			return remoteError("unverified_provenance", "the hosted service could not independently verify the complete committed source and approved ref")
		}
	}
	switch {
	case errors.Is(err, snapshot.ErrUnsupported):
		return remoteError("unsupported_schema", "the complete approved subset could not be represented; no records were silently omitted")
	case errors.Is(err, snapshot.ErrTooLarge):
		return remoteError("snapshot_too_large", "the complete approved subset exceeds publishing bounds")
	case errors.Is(err, snapshot.ErrConflict):
		return remoteError("record_conflict", "a same-key immutable checkpoint differs from an immediate parent; no content was published")
	case errors.Is(err, snapshot.ErrInvalid), errors.Is(err, snapshot.ErrProvenance):
		return remoteError("unverified_provenance", "unable to verify complete committed records from the selected local Git source and ref")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), errors.Is(err, hosted.ErrCancelled):
		return remoteError("cancelled", "hosted publishing stopped or exceeded its deadline; a started POST may already have succeeded; no automatic retry was attempted")
	default:
		return safeHostedError(err)
	}
}
