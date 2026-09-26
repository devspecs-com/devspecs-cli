package commands

import (
	"fmt"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/hubstore"
	"github.com/spf13/cobra"
)

func newHubConsumerCmd(opts *hubOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "consumer", Short: "Manage durable pull identities", Args: cobra.NoArgs}
	cmd.AddCommand(&cobra.Command{
		Use:   "enroll [consumer-id]",
		Short: "Enroll a stable consumer ID (or allocate one)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			consumer, err := db.EnrollConsumer(cmd.Context(), db.AuthorityID(), id)
			if err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubJSON(cmd, consumer)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Consumer: %s\n", consumer.ID)
			return err
		},
	})
	return cmd
}

func newHubSubscribeCmd(opts *hubOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "subscribe", Short: "Manage filtered durable subscriptions", Args: cobra.NoArgs}
	var consumer string
	var topics, kinds, typeSelectors []string
	var fromBeginning bool
	add := &cobra.Command{
		Use:   "add",
		Short: "Subscribe a consumer to repository topics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			filters := make([]hubstore.EventTypeFilter, 0, len(typeSelectors))
			for _, selector := range typeSelectors {
				at := strings.LastIndexByte(selector, '@')
				if at < 1 {
					return fmt.Errorf("--event-type must be key@version")
				}
				version, err := parseHubVersion(selector[at+1:])
				if err != nil {
					return fmt.Errorf("--event-type %q: %w", selector, err)
				}
				filters = append(filters, hubstore.EventTypeFilter{TypeKey: selector[:at], Version: version})
			}
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			sub, err := db.Subscribe(cmd.Context(), opts.repo, hubstore.SubscribeInput{
				AuthorityID: db.AuthorityID(), ConsumerID: consumer, TopicIDs: topics,
				Kinds: kinds, EventTypes: filters, FromBeginning: fromBeginning,
			})
			if err != nil {
				return err
			}
			return writeHubSubscription(cmd, sub, opts.asJSON)
		},
	}
	add.Flags().StringVar(&consumer, "consumer", "", "Enrolled consumer ID")
	add.Flags().StringArrayVar(&topics, "topic", nil, "Topic ID to subscribe to (repeatable)")
	add.Flags().StringArrayVar(&kinds, "kind", nil, "message or event (repeatable; default both)")
	add.Flags().StringArrayVar(&typeSelectors, "event-type", nil, "Event type filter key@version (repeatable)")
	add.Flags().BoolVar(&fromBeginning, "from-beginning", false, "Start at retained history rather than current high-water")
	_ = add.MarkFlagRequired("consumer")
	_ = add.MarkFlagRequired("topic")
	cmd.AddCommand(add)

	var listConsumer string
	var includeRemoved bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List one consumer's repository subscriptions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.OpenReadOnly(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			subs, err := db.ListSubscriptions(cmd.Context(), opts.repo, listConsumer, includeRemoved)
			if err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubJSON(cmd, subs)
			}
			for _, sub := range subs {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s  ack=%d  topics=%s\n", sub.ID, sub.AcknowledgedSequence, strings.Join(sub.TopicIDs, ",")); err != nil {
					return err
				}
			}
			return nil
		},
	}
	list.Flags().StringVar(&listConsumer, "consumer", "", "Enrolled consumer ID")
	list.Flags().BoolVar(&includeRemoved, "all", false, "Include removed subscriptions")
	_ = list.MarkFlagRequired("consumer")
	cmd.AddCommand(list)

	var removeConsumer string
	remove := &cobra.Command{
		Use:   "remove <subscription-id>",
		Short: "Remove a subscription without deleting hub history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			sub, err := db.RemoveSubscription(cmd.Context(), opts.repo, removeConsumer, args[0], db.AuthorityID())
			if err != nil {
				return err
			}
			return writeHubSubscription(cmd, sub, opts.asJSON)
		},
	}
	remove.Flags().StringVar(&removeConsumer, "consumer", "", "Enrolled consumer ID")
	_ = remove.MarkFlagRequired("consumer")
	cmd.AddCommand(remove)
	return cmd
}

func newHubPullCmd(opts *hubOptions) *cobra.Command {
	var consumer string
	var limit int
	cmd := &cobra.Command{
		Use:   "pull <subscription-id>",
		Short: "Read one bounded page without advancing the consumer cursor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.OpenReadOnly(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			page, err := db.Pull(cmd.Context(), opts.repo, consumer, args[0], limit)
			if err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubJSON(cmd, page)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Subscription: %s\nScanned: %d -> %d (high-water %d)\n", page.SubscriptionID, page.PriorAcknowledged, page.NextScanPosition, page.HighWater); err != nil {
				return err
			}
			for _, entry := range page.Entries {
				if entry.Message != nil {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Message %d  %s: %s\n", entry.Message.Sequence, entry.Message.ActorID, entry.Message.Text); err != nil {
						return err
					}
				} else if entry.Event != nil {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Event %d  %s@%d  %s\n", entry.Event.Sequence, entry.Event.TypeKey, entry.Event.Version, entry.Event.EntryID); err != nil {
						return err
					}
				}
			}
			for _, gap := range page.Gaps {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Gap %d..%d  %s\n", gap.From, gap.To, gap.Reason); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Ack token: %s\n", page.AckToken)
			return err
		},
	}
	cmd.Flags().StringVar(&consumer, "consumer", "", "Enrolled consumer ID")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum scanned publication positions (1-100)")
	_ = cmd.MarkFlagRequired("consumer")
	return cmd
}

func newHubAckCmd(opts *hubOptions) *cobra.Command {
	var consumer, token string
	var prior, next int64
	cmd := &cobra.Command{
		Use:   "ack <subscription-id>",
		Short: "Acknowledge exactly one previously pulled scan range",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			acknowledged, err := db.Ack(cmd.Context(), opts.repo, consumer, args[0], db.AuthorityID(), prior, next, token)
			if err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubJSON(cmd, map[string]any{"subscription_id": args[0], "acknowledged_sequence": acknowledged})
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Acknowledged: %d\n", acknowledged)
			return err
		},
	}
	cmd.Flags().StringVar(&consumer, "consumer", "", "Enrolled consumer ID")
	cmd.Flags().Int64Var(&prior, "prior", 0, "Prior acknowledged sequence returned by pull")
	cmd.Flags().Int64Var(&next, "next", 0, "Next scan position returned by pull")
	cmd.Flags().StringVar(&token, "token", "", "Exact ack token returned by pull")
	_ = cmd.MarkFlagRequired("consumer")
	_ = cmd.MarkFlagRequired("prior")
	_ = cmd.MarkFlagRequired("next")
	_ = cmd.MarkFlagRequired("token")
	return cmd
}

func writeHubSubscription(cmd *cobra.Command, sub hubstore.Subscription, asJSON bool) error {
	if asJSON {
		return writeHubJSON(cmd, sub)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Subscription: %s\nConsumer: %s\nScope: %s\nAcknowledged: %d\n", sub.ID, sub.ConsumerID, sub.ScopeID, sub.AcknowledgedSequence)
	return err
}
