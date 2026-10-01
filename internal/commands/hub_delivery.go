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
		Short: "Subscribe to repo topics or explicit global:<topic-id> addresses",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope := opts.repo
			resolvedTopics := make([]string, 0, len(topics))
			for i, address := range topics {
				topicScope, topicID, err := hubAddress(cmd, opts, address)
				if err != nil {
					return err
				}
				if i > 0 && topicScope != scope {
					return fmt.Errorf("subscription topics must all use the same repo or global: scope")
				}
				scope = topicScope
				resolvedTopics = append(resolvedTopics, topicID)
			}
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
			sub, err := db.Subscribe(cmd.Context(), scope, hubstore.SubscribeInput{
				AuthorityID: db.AuthorityID(), ConsumerID: consumer, TopicIDs: resolvedTopics,
				Kinds: kinds, EventTypes: filters, FromBeginning: fromBeginning,
			})
			if err != nil {
				return err
			}
			return writeHubSubscription(cmd, sub, opts.asJSON, scope)
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
		Use:   "list [global:]",
		Short: "List repo subscriptions or pass global: for home-global",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, err := hubListScope(cmd, opts, args)
			if err != nil {
				return err
			}
			db, err := hubstore.OpenReadOnly(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			subs, err := db.ListSubscriptions(cmd.Context(), scope, listConsumer, includeRemoved)
			if err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubScopedJSON(cmd, subs, scope)
			}
			for _, sub := range subs {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s  ack=%d  topics=%s\n", hubDisplayID(scope, sub.ID), sub.AcknowledgedSequence, hubDisplayIDs(scope, sub.TopicIDs)); err != nil {
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
			scope, subscriptionID, err := hubAddress(cmd, opts, args[0])
			if err != nil {
				return err
			}
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			sub, err := db.RemoveSubscription(cmd.Context(), scope, removeConsumer, subscriptionID, db.AuthorityID())
			if err != nil {
				return err
			}
			return writeHubSubscription(cmd, sub, opts.asJSON, scope)
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
		Short: "Read one page; prefix a global subscription ID with global:",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, subscriptionID, err := hubAddress(cmd, opts, args[0])
			if err != nil {
				return err
			}
			db, err := hubstore.OpenReadOnly(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			page, err := db.Pull(cmd.Context(), scope, consumer, subscriptionID, limit)
			if err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubScopedJSON(cmd, page, scope)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Subscription: %s\nScanned: %d -> %d (high-water %d)\n", hubDisplayID(scope, page.SubscriptionID), page.PriorAcknowledged, page.NextScanPosition, page.HighWater); err != nil {
				return err
			}
			for _, entry := range page.Entries {
				if entry.Message != nil {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Message %d  %s: %s\n", entry.Message.Sequence, entry.Message.ActorID, entry.Message.Text); err != nil {
						return err
					}
				} else if entry.Event != nil {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Event %d  %s@%d  %s\n", entry.Event.Sequence, entry.Event.TypeKey, entry.Event.Version, hubDisplayID(scope, entry.Event.EntryID)); err != nil {
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
		Short: "Acknowledge one scan range; use global:<subscription-id> for global",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, subscriptionID, err := hubAddress(cmd, opts, args[0])
			if err != nil {
				return err
			}
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			acknowledged, err := db.Ack(cmd.Context(), scope, consumer, subscriptionID, db.AuthorityID(), prior, next, token)
			if err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubScopedJSON(cmd, map[string]any{"subscription_id": subscriptionID, "acknowledged_sequence": acknowledged}, scope)
			}
			if scope == hubstore.GlobalScopeSelector {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Subscription: %s\nAcknowledged: %d\n", hubDisplayID(scope, subscriptionID), acknowledged)
				return err
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

func writeHubSubscription(cmd *cobra.Command, sub hubstore.Subscription, asJSON bool, scope string) error {
	if asJSON {
		return writeHubScopedJSON(cmd, sub, scope)
	}
	if scope == hubstore.GlobalScopeSelector {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Subscription: %s\nConsumer: %s\nScope: global (%s)\nAcknowledged: %d\n", hubDisplayID(scope, sub.ID), sub.ConsumerID, sub.ScopeID, sub.AcknowledgedSequence)
		return err
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Subscription: %s\nConsumer: %s\nScope: %s\nAcknowledged: %d\n", sub.ID, sub.ConsumerID, sub.ScopeID, sub.AcknowledgedSequence)
	return err
}

func hubDisplayIDs(scope string, ids []string) string {
	display := make([]string, len(ids))
	for i, id := range ids {
		display[i] = hubDisplayID(scope, id)
	}
	return strings.Join(display, ",")
}
