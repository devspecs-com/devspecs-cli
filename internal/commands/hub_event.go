package commands

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/hubstore"
	"github.com/spf13/cobra"
)

func newHubTypeCmd(opts *hubOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "type", Short: "Manage validated event type schemas", Args: cobra.NoArgs}
	var actor, schemaFile string
	var generation int64
	register := &cobra.Command{
		Use:   "register <topic-id> <type-key> <version>",
		Short: "Register an immutable JSON Schema event version",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			version, err := parseHubVersion(args[2])
			if err != nil {
				return err
			}
			data, err := readHubFile(schemaFile, 16*1024)
			if err != nil {
				return err
			}
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			schema, err := db.RegisterEventSchema(cmd.Context(), opts.repo, args[0], actor, generation, args[1], version, data)
			if err != nil {
				return err
			}
			return writeHubSchema(cmd, schema, opts.asJSON)
		},
	}
	register.Flags().StringVar(&actor, "actor", "", "Topic owner or maintainer actor ID")
	register.Flags().Int64Var(&generation, "generation", 0, "Expected topic policy generation")
	register.Flags().StringVar(&schemaFile, "schema-file", "", "Local JSON Schema file")
	_ = register.MarkFlagRequired("actor")
	_ = register.MarkFlagRequired("generation")
	_ = register.MarkFlagRequired("schema-file")
	cmd.AddCommand(register)

	var retirementActor string
	var retirementGeneration int64
	retire := &cobra.Command{
		Use:   "retire <topic-id> <type-key> <version>",
		Short: "Retire an event version while preserving historical schemas",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			version, err := parseHubVersion(args[2])
			if err != nil {
				return err
			}
			db, err := hubstore.Open(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			schema, err := db.RetireEventSchema(cmd.Context(), opts.repo, args[0], retirementActor, retirementGeneration, args[1], version)
			if err != nil {
				return err
			}
			return writeHubSchema(cmd, schema, opts.asJSON)
		},
	}
	retire.Flags().StringVar(&retirementActor, "actor", "", "Topic owner or maintainer actor ID")
	retire.Flags().Int64Var(&retirementGeneration, "generation", 0, "Expected topic policy generation")
	_ = retire.MarkFlagRequired("actor")
	_ = retire.MarkFlagRequired("generation")
	cmd.AddCommand(retire)

	cmd.AddCommand(&cobra.Command{
		Use:   "show <topic-id> <type-key> <version>",
		Short: "Inspect one registered event schema",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			version, err := parseHubVersion(args[2])
			if err != nil {
				return err
			}
			db, err := hubstore.OpenReadOnly(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			schema, err := db.ShowEventSchema(cmd.Context(), opts.repo, args[0], args[1], version)
			if err != nil {
				return err
			}
			return writeHubSchema(cmd, schema, opts.asJSON)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "list <topic-id>",
		Short: "Discover event types and versions for a topic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.OpenReadOnly(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			schemas, err := db.ListEventSchemas(cmd.Context(), opts.repo, args[0], 50, 0)
			if err != nil {
				return err
			}
			if opts.asJSON {
				return writeHubJSON(cmd, schemas)
			}
			for _, schema := range schemas {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s@%d  %s\n", schema.TypeKey, schema.Version, schema.SHA256); err != nil {
					return err
				}
			}
			return nil
		},
	})
	return cmd
}

func newHubEventCmd(opts *hubOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "event", Short: "Publish and inspect validated events", Args: cobra.NoArgs}
	var actor, typeKey, payloadFile, key, corrects, expiryText string
	var version int64
	publish := &cobra.Command{
		Use:   "publish <topic-id>",
		Short: "Validate and atomically publish one typed event",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := readHubFile(payloadFile, 64*1024)
			if err != nil {
				return err
			}
			input := hubstore.EventInput{TypeKey: typeKey, Version: version, Payload: payload, IdempotencyKey: key, CorrectsEntryID: corrects}
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
			event, err := db.PublishEvent(cmd.Context(), opts.repo, args[0], actor, input)
			if err != nil {
				return err
			}
			return writeHubEvent(cmd, event, opts.asJSON)
		},
	}
	publish.Flags().StringVar(&actor, "actor", "", "Enrolled publisher actor ID")
	publish.Flags().StringVar(&typeKey, "type", "", "Registered event type key")
	publish.Flags().Int64Var(&version, "version", 0, "Registered event version")
	publish.Flags().StringVar(&payloadFile, "payload-file", "", "Local JSON payload file")
	publish.Flags().StringVar(&key, "key", "", "Optional idempotency key for safe retries")
	publish.Flags().StringVar(&corrects, "corrects", "", "Optional earlier event entry ID being corrected")
	publish.Flags().StringVar(&expiryText, "expires-at", "", "Optional expiry in RFC3339 format")
	_ = publish.MarkFlagRequired("actor")
	_ = publish.MarkFlagRequired("type")
	_ = publish.MarkFlagRequired("version")
	_ = publish.MarkFlagRequired("payload-file")
	cmd.AddCommand(publish)

	var historical bool
	show := &cobra.Command{
		Use:   "show <entry-id>",
		Short: "Read one exact event publication",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := hubstore.OpenReadOnly(cmd.Context(), hubstore.Options{})
			if err != nil {
				return err
			}
			defer db.Close()
			event, err := db.ReadEvent(cmd.Context(), opts.repo, args[0], historical)
			if err != nil {
				return err
			}
			return writeHubEvent(cmd, event, opts.asJSON)
		},
	}
	show.Flags().BoolVar(&historical, "historical", false, "Include an expired or archived event")
	cmd.AddCommand(show)
	return cmd
}

func parseHubVersion(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("version must be a positive integer")
	}
	return n, nil
}

func readHubFile(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("input exceeds %d bytes", max)
	}
	return data, nil
}

func writeHubSchema(cmd *cobra.Command, schema hubstore.EventSchema, asJSON bool) error {
	if asJSON {
		return writeHubJSON(cmd, schema)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Type: %s@%d\nTopic: %s\nSchema SHA256: %s\n", schema.TypeKey, schema.Version, schema.TopicID, schema.SHA256)
	return err
}

func writeHubEvent(cmd *cobra.Command, event hubstore.Event, asJSON bool) error {
	if asJSON {
		return writeHubJSON(cmd, event)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Event: %s (sequence %d)\nType: %s@%d\nTopic: %s\nActor: %s\n", event.EntryID, event.Sequence, event.TypeKey, event.Version, event.TopicID, event.ActorID)
	return err
}
