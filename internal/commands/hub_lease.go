package commands

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/hubstore"
	"github.com/spf13/cobra"
)

func newHubLeaseCmd(opts *hubOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lease",
		Short: "Coordinate one participating worker per topic",
		Long:  "Cooperative leases serialize participating agents only. Stop work or renew before expiry; expiry does not stop a running process. Commands that bypass DevSpecs are not blocked.",
		Args:  cobra.NoArgs,
	}
	var acquireActor string
	var acquireFor, wait time.Duration
	acquire := &cobra.Command{
		Use:   "acquire <topic-id>",
		Short: "Atomically acquire a time-bounded topic lease",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			scope, topicID, err := hubAddress(c, opts, args[0])
			if err != nil {
				return err
			}
			if wait < 0 || wait > 24*time.Hour {
				return fmt.Errorf("--wait must be between 0 and 24h")
			}
			ctx := c.Context()
			if wait > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, wait)
				defer cancel()
			}
			db, err := hubstore.Open(ctx, hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			for {
				lease, acquireErr := db.AcquireLease(ctx, scope, topicID, acquireActor, acquireFor)
				if acquireErr == nil {
					return writeHubLease(c, lease, opts.asJSON, scope)
				}
				if wait == 0 || (!errors.Is(acquireErr, hubstore.ErrConflict) && !errors.Is(acquireErr, hubstore.ErrBusyRetryable)) {
					return acquireErr
				}
				select {
				case <-ctx.Done():
					return fmt.Errorf("lease wait ended: %w (last admission: %v)", ctx.Err(), acquireErr)
				case <-time.After(250 * time.Millisecond):
				}
			}
		},
	}
	acquire.Flags().StringVar(&acquireActor, "actor", "", "Enrolled actor ID")
	acquire.Flags().DurationVar(&acquireFor, "for", 30*time.Minute, "Lease lifetime (1s to 24h)")
	acquire.Flags().DurationVar(&wait, "wait", 0, "Maximum time to wait for admission (0 to 24h)")
	_ = acquire.MarkFlagRequired("actor")
	cmd.AddCommand(acquire)

	show := &cobra.Command{
		Use:   "show <topic-id>",
		Short: "Inspect current lease holder and deadline",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			scope, topicID, err := hubAddress(c, opts, args[0])
			if err != nil {
				return err
			}
			db, err := hubstore.OpenReadOnly(c.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			lease, err := db.ShowLease(c.Context(), scope, topicID)
			if err != nil {
				return err
			}
			return writeHubLease(c, lease, opts.asJSON, scope)
		},
	}
	cmd.AddCommand(show)

	var renewActor, renewToken string
	var renewFor time.Duration
	renew := &cobra.Command{
		Use:   "renew <topic-id>",
		Short: "Renew a live lease using its bearer token",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			scope, topicID, err := hubAddress(c, opts, args[0])
			if err != nil {
				return err
			}
			db, err := hubstore.Open(c.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			lease, err := db.RenewLease(c.Context(), scope, topicID, renewActor, renewToken, renewFor)
			if err != nil {
				return err
			}
			return writeHubLease(c, lease, opts.asJSON, scope)
		},
	}
	renew.Flags().StringVar(&renewActor, "actor", "", "Holder actor ID")
	renew.Flags().StringVar(&renewToken, "token", "", "Bearer token returned by acquire")
	renew.Flags().DurationVar(&renewFor, "for", 30*time.Minute, "New lease lifetime from now (1s to 24h)")
	_ = renew.MarkFlagRequired("actor")
	_ = renew.MarkFlagRequired("token")
	cmd.AddCommand(renew)

	var releaseActor, releaseToken string
	release := &cobra.Command{
		Use:   "release <topic-id>",
		Short: "Release the current lease using its bearer token",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			scope, topicID, err := hubAddress(c, opts, args[0])
			if err != nil {
				return err
			}
			db, err := hubstore.Open(c.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			if err := db.ReleaseLease(c.Context(), scope, topicID, releaseActor, releaseToken); err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubScopedJSON(c, map[string]any{"topic_id": topicID, "state": "available"}, scope)
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "Lease released: %s\n", hubDisplayID(scope, topicID))
			return err
		},
	}
	release.Flags().StringVar(&releaseActor, "actor", "", "Holder actor ID")
	release.Flags().StringVar(&releaseToken, "token", "", "Bearer token returned by acquire")
	_ = release.MarkFlagRequired("actor")
	_ = release.MarkFlagRequired("token")
	cmd.AddCommand(release)
	return cmd
}

func writeHubLease(cmd *cobra.Command, lease hubstore.Lease, asJSON bool, scope string) error {
	if asJSON {
		return writeHubScopedJSON(cmd, lease, scope)
	}
	if lease.State != "held" {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Lease: %s (available)\n", hubDisplayID(scope, lease.TopicID))
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Lease: %s (held by %s, generation %d)\nExpires: %s\n", hubDisplayID(scope, lease.TopicID), lease.ActorID, lease.Generation, lease.ExpiresAt.Format(time.RFC3339)); err != nil {
		return err
	}
	if lease.Token != "" {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Token: %s\n", lease.Token)
		return err
	}
	return nil
}
