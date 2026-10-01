package commands

import (
	"fmt"

	"github.com/devspecs-com/devspecs-cli/internal/hubstore"
	"github.com/spf13/cobra"
)

func newHubOwnerCmd(opts *hubOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "owner", Short: "Manage delegated topic maintainers", Args: cobra.NoArgs}
	cmd.AddCommand(newHubMaintainerCmd(opts, true), newHubMaintainerCmd(opts, false))
	cmd.AddCommand(&cobra.Command{
		Use:   "list <topic-id>",
		Short: "List delegated topic maintainers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, topicID, err := hubAddress(cmd, opts, args[0])
			if err != nil {
				return err
			}
			db, err := hubstore.OpenReadOnly(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			maintainers, err := db.ListMaintainers(cmd.Context(), scope, topicID, 50, 0)
			if err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubScopedJSON(cmd, maintainers, scope)
			}
			for _, actor := range maintainers {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), actor); err != nil {
					return err
				}
			}
			return nil
		},
	})
	return cmd
}

func newHubMaintainerCmd(opts *hubOptions, grant bool) *cobra.Command {
	verb := "grant"
	if !grant {
		verb = "revoke"
	}
	var owner string
	var generation int64
	cmd := &cobra.Command{
		Use:   verb + " <topic-id> <maintainer-actor-id>",
		Short: verb + " topic maintainer rights",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, topicID, err := hubAddress(cmd, opts, args[0])
			if err != nil {
				return err
			}
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			topic, err := db.SetMaintainer(cmd.Context(), scope, topicID, owner, args[1], generation, grant)
			if err != nil {
				return err
			}
			return writeHubTopic(cmd, topic, opts.asJSON, scope)
		},
	}
	cmd.Flags().StringVar(&owner, "actor", "", "Topic owner actor ID")
	cmd.Flags().Int64Var(&generation, "generation", 0, "Expected topic policy generation")
	_ = cmd.MarkFlagRequired("actor")
	_ = cmd.MarkFlagRequired("generation")
	return cmd
}
