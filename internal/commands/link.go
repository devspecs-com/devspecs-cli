package commands

import (
	"fmt"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/spf13/cobra"
)

var validLinkTypes = []string{
	"related", "implements", "implemented_by",
	"supersedes", "superseded_by", "blocks",
	"blocked_by", "references", "referenced_by",
}

// NewLinkCmd creates the ds link command.
func NewLinkCmd() *cobra.Command {
	var linkType string

	cmd := &cobra.Command{
		Use:   "link <id> <target>",
		Short: "Add a link from an artifact to an external target",
		Long:  fmt.Sprintf("Supported link types: %v", validLinkTypes),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLink(cmd, args[0], args[1], linkType)
		},
	}

	cmd.Flags().StringVar(&linkType, "type", "related", "Link type")
	return cmd
}

func runLink(cmd *cobra.Command, idOrPrefix, target, linkType string) error {
	if !isValidLinkType(linkType) {
		return fmt.Errorf("invalid link type %q; valid values: %v", linkType, validLinkTypes)
	}

	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	art, err := db.GetArtifact(idOrPrefix)
	if err != nil {
		return err
	}

	ids := idgen.NewFactory()
	linkID := ids.NewWithPrefix("link_")
	now := time.Now().UTC().Format(time.RFC3339)

	if err := db.InsertLink(linkID, art.ID, linkType, target, now); err != nil {
		return fmt.Errorf("insert link: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Linked %s -> %s [%s]\n", art.ID, target, linkType)
	return nil
}

func isValidLinkType(s string) bool {
	for _, v := range validLinkTypes {
		if v == s {
			return true
		}
	}
	return false
}
