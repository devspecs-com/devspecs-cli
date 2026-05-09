package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var validStatuses = []string{
	"draft", "proposed", "approved", "implementing",
	"implemented", "superseded", "rejected", "unknown",
}

// NewStatusCmd creates the ds status command.
func NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <id> <status>",
		Short: "Update artifact status",
		Long:  fmt.Sprintf("Supported statuses: %v", validStatuses),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, args[0], args[1])
		},
	}
	return cmd
}

func runStatus(cmd *cobra.Command, idOrPrefix, newStatus string) error {
	if !isValidStatus(newStatus) {
		return fmt.Errorf("invalid status %q; valid values: %v", newStatus, validStatuses)
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

	oldStatus := art.Status
	now := time.Now().UTC().Format(time.RFC3339)
	if err := db.UpdateArtifactStatus(art.ID, newStatus, now); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Updated %s status: %s -> %s\n", art.ID, oldStatus, newStatus)
	return nil
}

func isValidStatus(s string) bool {
	for _, v := range validStatuses {
		if v == s {
			return true
		}
	}
	return false
}
