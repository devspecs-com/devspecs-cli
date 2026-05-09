package commands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

// NewCaptureCmd creates the ds capture command.
func NewCaptureCmd() *cobra.Command {
	var (
		kind   string
		title  string
		status string
		asJSON bool
	)

	cmd := &cobra.Command{
		Use:   "capture <path>",
		Short: "Capture a specific file as an artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCapture(cmd, args[0], kind, title, status, asJSON)
		},
	}

	cmd.Flags().StringVar(&kind, "kind", "", "Override artifact kind")
	cmd.Flags().StringVar(&title, "title", "", "Override artifact title")
	cmd.Flags().StringVar(&status, "status", "", "Override artifact status")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	return cmd
}

func runCapture(cmd *cobra.Command, path, kind, title, status string, asJSON bool) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("file not found: %s", path)
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get wd: %w", err)
	}

	relPath, err := filepath.Rel(wd, absPath)
	if err != nil {
		relPath = path
	}
	relPath = filepath.ToSlash(relPath)

	dbPath, err := config.DBPath()
	if err != nil {
		return fmt.Errorf("resolve db: %w", err)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// Parse with markdown adapter
	mdAdapter := &markdown.Adapter{}
	candidate := adapters.Candidate{
		PrimaryPath: absPath,
		RelPath:     relPath,
		AdapterName: "markdown",
	}
	art, _, todos, err := mdAdapter.Parse(context.Background(), candidate)
	if err != nil {
		return fmt.Errorf("parse file: %w", err)
	}

	// Apply overrides
	if kind != "" {
		art.Kind = kind
	}
	if title != "" {
		art.Title = title
	}
	if status != "" {
		art.Status = status
	}

	sourceIdentity := relPath + "|capture"
	now := time.Now().UTC().Format(time.RFC3339)
	ids := idgen.NewFactory()

	// Check if already captured
	existingArtID, err := db.FindSourceByIdentity(sourceIdentity)
	if err != nil {
		return err
	}

	if existingArtID != "" {
		// Update existing artifact
		contentHash := hashBody(art.Body)
		revID := ids.NewWithPrefix("rev_")
		db.InsertRevisionDirect(revID, existingArtID, contentHash, art.Body, now)
		db.UpdateArtifactStatus(existingArtID, art.Status, now)
		db.Exec("UPDATE artifacts SET title = ?, kind = ?, current_revision_id = ?, updated_at = ?, last_observed_at = ? WHERE id = ?",
			art.Title, art.Kind, revID, now, now, existingArtID)

		// Replace todos
		db.Exec("DELETE FROM artifact_todos WHERE artifact_id = ?", existingArtID)
		for _, td := range todos {
			todoID := ids.NewWithPrefix("todo_")
			done := 0
			if td.Done {
				done = 1
			}
			db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
				todoID, existingArtID, revID, td.Ordinal, td.Text, done, td.SourceFile, td.SourceLine, now)
		}

		db.IndexArtifactFTS(existingArtID, art.Title, art.Body, relPath)
		return outputCapture(cmd, existingArtID, relPath, asJSON)
	}

	// Create new
	// Ensure repo exists
	var repoID string
	db.QueryRow("SELECT id FROM repos WHERE root_path = ?", wd).Scan(&repoID)
	if repoID == "" {
		repoID = ids.NewWithPrefix("repo_")
		db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", repoID, wd, now, now)
	}

	artifactID := ids.New()
	revID := ids.NewWithPrefix("rev_")
	contentHash := hashBody(art.Body)

	db.InsertArtifactDirect(artifactID, repoID, art.Kind, art.Title, art.Status, revID, now)
	db.InsertRevisionDirect(revID, artifactID, contentHash, art.Body, now)
	db.InsertSourceDirect(ids.NewWithPrefix("src_"), artifactID, repoID, "capture", relPath, sourceIdentity, now)

	for _, td := range todos {
		todoID := ids.NewWithPrefix("todo_")
		done := 0
		if td.Done {
			done = 1
		}
		db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			todoID, artifactID, revID, td.Ordinal, td.Text, done, td.SourceFile, td.SourceLine, now)
	}

	db.IndexArtifactFTS(artifactID, art.Title, art.Body, relPath)
	return outputCapture(cmd, artifactID, relPath, asJSON)
}

func outputCapture(cmd *cobra.Command, id, path string, asJSON bool) error {
	if asJSON {
		obj := map[string]string{"id": id, "path": path}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(obj)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Captured artifact:")
	fmt.Fprintf(out, "  %s\n", id)
	fmt.Fprintf(out, "  %s\n", path)
	return nil
}

func hashBody(body string) string {
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	normalized = strings.Join(lines, "\n")
	h := sha256.Sum256([]byte(normalized))
	return "sha256:" + hex.EncodeToString(h[:])
}
