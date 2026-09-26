package commands

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/hubstore"
	"github.com/spf13/cobra"
)

type hubOptions struct {
	repo   string
	asJSON bool
}

// NewHubCmd exposes the repo-local coordination authority separately from the
// rebuildable source index and task checkpoint lifecycle.
func NewHubCmd() *cobra.Command {
	opts := &hubOptions{repo: "."}
	cmd := &cobra.Command{
		Use:   "hub",
		Short: "Share repo-scoped coordination topics and messages",
		Long: `Share short-lived coordination in a local SQLite hub. Topics belong to a
Git repository across its worktrees. Hub entries are not task checkpoints or
durable architectural decisions; promote lasting decisions to Git or docs.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().StringVar(&opts.repo, "repo", ".", "Repository path (defaults to current directory)")
	cmd.PersistentFlags().BoolVar(&opts.asJSON, "json", false, "Output machine-readable JSON")
	cmd.AddCommand(newHubActorCmd(opts), newHubTopicCmd(opts), newHubMessageCmd(opts), newHubTypeCmd(opts), newHubEventCmd(opts), newHubConsumerCmd(opts), newHubSubscribeCmd(opts), newHubPullCmd(opts), newHubAckCmd(opts))
	return cmd
}

func newHubActorCmd(opts *hubOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "actor", Short: "Manage local attribution identities", Args: cobra.NoArgs}
	cmd.AddCommand(&cobra.Command{
		Use:   "enroll [actor-id]",
		Short: "Enroll a stable local actor ID (or allocate one)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			actor, err := db.EnrollActor(cmd.Context(), id)
			if err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubJSON(cmd, actor)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Actor: %s\n", actor.ID)
			return err
		},
	})
	return cmd
}

func newHubTopicCmd(opts *hubOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "topic", Short: "Discover and manage repository topics", Args: cobra.NoArgs}
	var actor, name, description, expiryText string
	create := &cobra.Command{
		Use:   "create <key>",
		Short: "Create a named repository topic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if actor == "" {
				return fmt.Errorf("--actor is required; enroll with ds hub actor enroll")
			}
			input := hubstore.TopicInput{Key: args[0], Name: name, Description: description}
			if cmd.Flags().Changed("expires-at") {
				expiry, err := time.Parse(time.RFC3339, expiryText)
				if err != nil {
					return fmt.Errorf("--expires-at must be RFC3339: %w", err)
				}
				input.ExpiresAt = &expiry
			}
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			if _, err := db.EnrollRepo(cmd.Context(), opts.repo); err != nil {
				return err
			}
			topic, err := db.CreateTopic(cmd.Context(), opts.repo, actor, input)
			if err != nil {
				return err
			}
			return writeHubTopic(cmd, topic, opts.asJSON)
		},
	}
	create.Flags().StringVar(&actor, "actor", "", "Enrolled actor ID that owns the topic")
	create.Flags().StringVar(&name, "name", "", "Human-readable topic name")
	create.Flags().StringVar(&description, "description", "", "Short discovery description")
	create.Flags().StringVar(&expiryText, "expires-at", "", "Optional expiry in RFC3339 format")
	_ = create.MarkFlagRequired("actor")
	_ = create.MarkFlagRequired("name")
	_ = create.MarkFlagRequired("description")
	cmd.AddCommand(create)

	var query string
	var includeArchived bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List or search repository topics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.OpenReadOnly(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			topics, err := db.ListTopics(cmd.Context(), opts.repo, hubstore.TopicList{Query: query, IncludeArchived: includeArchived})
			if err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubJSON(cmd, topics)
			}
			for _, topic := range topics {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s (%s)\n", topic.ID, topic.Key, topic.Name, topic.State); err != nil {
					return err
				}
			}
			return nil
		},
	}
	list.Flags().StringVar(&query, "query", "", "Search keys, names, and descriptions")
	list.Flags().BoolVar(&includeArchived, "all", false, "Include archived and expired topics")
	cmd.AddCommand(list)

	cmd.AddCommand(&cobra.Command{
		Use:   "show <topic-id>",
		Short: "Inspect one exact topic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.OpenReadOnly(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			topic, err := db.ShowTopic(cmd.Context(), opts.repo, args[0])
			if err != nil {
				return err
			}
			return writeHubTopic(cmd, topic, opts.asJSON)
		},
	})
	var editActor, editName, editDescription, editExpiryText string
	var editGeneration int64
	var clearExpiry bool
	edit := &cobra.Command{
		Use:   "edit <topic-id>",
		Short: "Update topic discovery text or deadline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clearExpiry && cmd.Flags().Changed("expires-at") {
				return fmt.Errorf("--clear-expiry and --expires-at cannot be combined")
			}
			change := hubstore.TopicEdit{Name: editName, Description: editDescription, ExpectedGeneration: editGeneration}
			if clearExpiry {
				change.ChangeExpiry = true
			}
			if cmd.Flags().Changed("expires-at") {
				expiry, err := time.Parse(time.RFC3339, editExpiryText)
				if err != nil {
					return fmt.Errorf("--expires-at must be RFC3339: %w", err)
				}
				change.ChangeExpiry = true
				change.ExpiresAt = &expiry
			}
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			topic, err := db.EditTopic(cmd.Context(), opts.repo, args[0], editActor, change)
			if err != nil {
				return err
			}
			return writeHubTopic(cmd, topic, opts.asJSON)
		},
	}
	edit.Flags().StringVar(&editActor, "actor", "", "Enrolled actor ID")
	edit.Flags().Int64Var(&editGeneration, "generation", 0, "Expected topic policy generation")
	edit.Flags().StringVar(&editName, "name", "", "New topic name")
	edit.Flags().StringVar(&editDescription, "description", "", "New discovery description")
	edit.Flags().StringVar(&editExpiryText, "expires-at", "", "New expiry in RFC3339 format")
	edit.Flags().BoolVar(&clearExpiry, "clear-expiry", false, "Remove the topic deadline")
	_ = edit.MarkFlagRequired("actor")
	_ = edit.MarkFlagRequired("generation")
	cmd.AddCommand(edit)

	var archiveActor, archiveReason string
	var archiveGeneration int64
	archive := &cobra.Command{
		Use:   "archive <topic-id>",
		Short: "Archive a topic without deleting its history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			topic, err := db.ArchiveTopic(cmd.Context(), opts.repo, args[0], archiveActor, archiveGeneration, archiveReason)
			if err != nil {
				return err
			}
			return writeHubTopic(cmd, topic, opts.asJSON)
		},
	}
	archive.Flags().StringVar(&archiveActor, "actor", "", "Enrolled actor ID")
	archive.Flags().Int64Var(&archiveGeneration, "generation", 0, "Expected topic policy generation")
	archive.Flags().StringVar(&archiveReason, "reason", "", "Reason recorded in topic audit")
	_ = archive.MarkFlagRequired("actor")
	_ = archive.MarkFlagRequired("generation")
	_ = archive.MarkFlagRequired("reason")
	cmd.AddCommand(archive)

	var restoreActor, restoreReason, restoreExpiryText string
	var restoreGeneration int64
	var restoreClearExpiry bool
	restore := &cobra.Command{
		Use:   "restore <topic-id>",
		Short: "Restore an archived topic with an audit reason",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if restoreClearExpiry && cmd.Flags().Changed("expires-at") {
				return fmt.Errorf("--clear-expiry and --expires-at cannot be combined")
			}
			var expiry *time.Time
			if cmd.Flags().Changed("expires-at") {
				parsed, err := time.Parse(time.RFC3339, restoreExpiryText)
				if err != nil {
					return fmt.Errorf("--expires-at must be RFC3339: %w", err)
				}
				expiry = &parsed
			}
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			topic, err := db.RestoreTopic(cmd.Context(), opts.repo, args[0], restoreActor, restoreGeneration, restoreReason, restoreClearExpiry || expiry != nil, expiry)
			if err != nil {
				return err
			}
			return writeHubTopic(cmd, topic, opts.asJSON)
		},
	}
	restore.Flags().StringVar(&restoreActor, "actor", "", "Enrolled actor ID")
	restore.Flags().Int64Var(&restoreGeneration, "generation", 0, "Expected topic policy generation")
	restore.Flags().StringVar(&restoreReason, "reason", "", "Reason recorded in topic audit")
	restore.Flags().StringVar(&restoreExpiryText, "expires-at", "", "New expiry in RFC3339 format")
	restore.Flags().BoolVar(&restoreClearExpiry, "clear-expiry", false, "Remove the topic deadline")
	_ = restore.MarkFlagRequired("actor")
	_ = restore.MarkFlagRequired("generation")
	_ = restore.MarkFlagRequired("reason")
	cmd.AddCommand(restore)
	cmd.AddCommand(newHubOwnerCmd(opts))
	return cmd
}

func newHubMessageCmd(opts *hubOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "message", Short: "Publish and inspect freetext coordination", Args: cobra.NoArgs}
	var actor, body, key, expiryText string
	post := &cobra.Command{
		Use:   "post <topic-id>",
		Short: "Post an attributed message to an active topic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := hubstore.MessageInput{Text: body, IdempotencyKey: key}
			if cmd.Flags().Changed("expires-at") {
				expiry, err := time.Parse(time.RFC3339, expiryText)
				if err != nil {
					return fmt.Errorf("--expires-at must be RFC3339: %w", err)
				}
				input.ExpiresAt = &expiry
			}
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			input.AuthorityID = db.AuthorityID()
			message, err := db.PostMessage(cmd.Context(), opts.repo, args[0], actor, input)
			if err != nil {
				return err
			}
			return writeHubMessage(cmd, message, opts.asJSON)
		},
	}
	post.Flags().StringVar(&actor, "actor", "", "Enrolled actor ID")
	post.Flags().StringVar(&body, "text", "", "Message text")
	post.Flags().StringVar(&key, "key", "", "Optional idempotency key for safe retries")
	post.Flags().StringVar(&expiryText, "expires-at", "", "Optional expiry in RFC3339 format")
	_ = post.MarkFlagRequired("actor")
	_ = post.MarkFlagRequired("text")
	cmd.AddCommand(post)

	list := &cobra.Command{
		Use:   "list <topic-id>",
		Short: "List live messages (pinned first)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.OpenReadOnly(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			messages, err := db.ListMessages(cmd.Context(), opts.repo, args[0], hubstore.MessageList{})
			if err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubJSON(cmd, messages)
			}
			for _, message := range messages {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%d  %s  %s: %s\n", message.Sequence, message.MessageID, message.ActorID, message.Text); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.AddCommand(list)

	var historical bool
	show := &cobra.Command{
		Use:   "show <topic-id> <message-id>",
		Short: "Read one current message",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.OpenReadOnly(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			message, err := db.ReadMessage(cmd.Context(), opts.repo, args[0], args[1], historical)
			if err != nil {
				return err
			}
			return writeHubMessage(cmd, message, opts.asJSON)
		},
	}
	show.Flags().BoolVar(&historical, "historical", false, "Include an expired or archived current message")
	cmd.AddCommand(show)

	history := &cobra.Command{
		Use:   "history <topic-id> <message-id>",
		Short: "Read attributed message revisions",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.OpenReadOnly(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			messages, err := db.MessageHistory(cmd.Context(), opts.repo, args[0], args[1], hubstore.MessageList{})
			if err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubJSON(cmd, messages)
			}
			for _, message := range messages {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%d  r%d  %s: %s\n", message.Sequence, message.Revision, message.ActorID, message.Text); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.AddCommand(history)

	var reviseActor, reviseText, reviseKey, reviseExpiryText string
	var expectedRevision int64
	var clearExpiry bool
	revise := &cobra.Command{
		Use:   "revise <topic-id> <message-id>",
		Short: "Append an attributed correction to a live message",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clearExpiry && cmd.Flags().Changed("expires-at") {
				return fmt.Errorf("--clear-expiry and --expires-at cannot be combined")
			}
			input := hubstore.MessageRevisionInput{
				MessageInput:     hubstore.MessageInput{Text: reviseText, IdempotencyKey: reviseKey},
				ExpectedRevision: expectedRevision,
				ChangeExpiry:     clearExpiry,
			}
			if cmd.Flags().Changed("expires-at") {
				expiry, err := time.Parse(time.RFC3339, reviseExpiryText)
				if err != nil {
					return fmt.Errorf("--expires-at must be RFC3339: %w", err)
				}
				input.ChangeExpiry = true
				input.ExpiresAt = &expiry
			}
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			input.AuthorityID = db.AuthorityID()
			message, err := db.ReviseMessage(cmd.Context(), opts.repo, args[0], args[1], reviseActor, input)
			if err != nil {
				return err
			}
			return writeHubMessage(cmd, message, opts.asJSON)
		},
	}
	revise.Flags().StringVar(&reviseActor, "actor", "", "Original author actor ID")
	revise.Flags().StringVar(&reviseText, "text", "", "Corrected text")
	revise.Flags().StringVar(&reviseKey, "key", "", "Optional idempotency key for safe retries")
	revise.Flags().Int64Var(&expectedRevision, "revision", 0, "Expected current message revision")
	revise.Flags().StringVar(&reviseExpiryText, "expires-at", "", "New expiry in RFC3339 format")
	revise.Flags().BoolVar(&clearExpiry, "clear-expiry", false, "Remove the message deadline")
	_ = revise.MarkFlagRequired("actor")
	_ = revise.MarkFlagRequired("text")
	_ = revise.MarkFlagRequired("revision")
	cmd.AddCommand(revise, newHubPinCmd(opts, true), newHubPinCmd(opts, false))
	return cmd
}

func newHubPinCmd(opts *hubOptions, pin bool) *cobra.Command {
	verb := "pin"
	if !pin {
		verb = "unpin"
	}
	var actor string
	var generation, revision int64
	cmd := &cobra.Command{
		Use:   verb + " <topic-id> <message-id>",
		Short: verb + " a live current message",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			message, err := db.PinMessage(cmd.Context(), opts.repo, args[0], args[1], actor, db.AuthorityID(), generation, revision, pin)
			if err != nil {
				return err
			}
			return writeHubMessage(cmd, message, opts.asJSON)
		},
	}
	cmd.Flags().StringVar(&actor, "actor", "", "Topic owner or maintainer actor ID")
	cmd.Flags().Int64Var(&generation, "generation", 0, "Expected topic policy generation")
	cmd.Flags().Int64Var(&revision, "revision", 0, "Expected current message revision")
	_ = cmd.MarkFlagRequired("actor")
	_ = cmd.MarkFlagRequired("generation")
	_ = cmd.MarkFlagRequired("revision")
	return cmd
}

func writeHubTopic(cmd *cobra.Command, topic hubstore.Topic, asJSON bool) error {
	if asJSON {
		return writeHubJSON(cmd, topic)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Topic: %s (%s)\nName: %s\nDescription: %s\nOwner: %s\nState: %s\n", topic.Key, topic.ID, topic.Name, topic.Description, topic.OwnerActorID, topic.State)
	return err
}

func writeHubMessage(cmd *cobra.Command, message hubstore.Message, asJSON bool) error {
	if asJSON {
		return writeHubJSON(cmd, message)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Message: %s (entry %s, sequence %d)\nTopic: %s\nActor: %s\nText: %s\n", message.MessageID, message.EntryID, message.Sequence, message.TopicID, message.ActorID, message.Text)
	return err
}

func writeHubJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(hubJSONResponse[any]{ContractVersion: "devspecs.hub/v1", Result: value})
}

type hubJSONResponse[T any] struct {
	ContractVersion string `json:"contract_version"`
	Result          T      `json:"result"`
}
