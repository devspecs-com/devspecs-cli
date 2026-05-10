package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// NewTagCmd creates the ds tag command group.
func NewTagCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag <id> <tag> [tag...]",
		Short: "Add tags to an artifact",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTag(cmd, args[0], args[1:])
		},
	}
	return cmd
}

// NewUntagCmd creates the ds untag command.
func NewUntagCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "untag <id> <tag>",
		Short: "Remove a tag from an artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUntag(cmd, args[0], args[1])
		},
	}
}

func runTag(cmd *cobra.Command, idOrPrefix string, tags []string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	art, err := db.GetArtifact(idOrPrefix)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, tag := range tags {
		if err := db.InsertTag(art.ID, tag, "manual", now); err != nil {
			return fmt.Errorf("add tag %q: %w", tag, err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Tagged %s with: %v\n", displayID(art), tags)
	return nil
}

func runUntag(cmd *cobra.Command, idOrPrefix, tag string) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	art, err := db.GetArtifact(idOrPrefix)
	if err != nil {
		return err
	}

	if err := db.DeleteTag(art.ID, tag); err != nil {
		return fmt.Errorf("remove tag %q: %w", tag, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed tag %q from %s\n", tag, displayID(art))
	return nil
}
