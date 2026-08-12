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
	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
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
		Use:    "capture <path>",
		Short:  "Capture a specific file as an artifact",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
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
	return runCaptureWithSubtype(cmd, path, kind, "", title, status, asJSON)
}

func runCaptureWithSubtype(cmd *cobra.Command, path, kind, subtype, title, status string, asJSON bool) error {
	return runCaptureAsSource(cmd, path, kind, subtype, title, status, "capture", asJSON)
}

func runCaptureAsSource(cmd *cobra.Command, path, kind, subtype, title, status, sourceType string, asJSON bool) error {
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
	captureCtx, cancel := context.WithTimeout(cmd.Context(), autoIndexDeadline)
	defer cancel()
	lease, err := store.AcquireIndexWriter(captureCtx, dbPath, indexWaitNotice(cmd, "Capture"))
	if err != nil {
		return indexOperationError("capture writer wait", autoIndexDeadlineLabel, err)
	}
	defer func() { _ = lease.Release() }()
	db, err := store.OpenWithWriterLeaseContext(captureCtx, dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", store.FriendlySQLiteBusyError(err))
	}
	defer db.Close()

	// Parse with markdown adapter
	mdAdapter := &markdown.Adapter{}
	candidate := adapters.Candidate{
		PrimaryPath: absPath,
		RelPath:     relPath,
		AdapterName: "markdown",
	}
	art, _, pr, err := mdAdapter.Parse(captureCtx, candidate)
	if err != nil {
		return fmt.Errorf("parse file: %w", err)
	}

	// Apply overrides
	if kind != "" {
		if err := config.ValidateKind(kind); err != nil {
			return fmt.Errorf("--kind: %w", err)
		}
		art.Kind = kind
	}
	if subtype != "" {
		if err := config.ValidateSubtype(art.Kind, subtype); err != nil {
			return fmt.Errorf("--subtype: %w", err)
		}
		art.Subtype = subtype
	}
	if title != "" {
		art.Title = title
	}
	if status != "" {
		art.Status = status
	}

	sourceType = strings.TrimSpace(sourceType)
	if sourceType == "" {
		sourceType = "capture"
	}
	if sourceType == "adr" {
		art.FormatProfile = "adr"
	}
	sourceIdentity := relPath + "|" + sourceType
	now := time.Now().UTC().Format(time.RFC3339)
	ids := idgen.NewFactory()
	info := repo.DetectIdentityContext(captureCtx, wd)
	if err := captureCtx.Err(); err != nil {
		return indexOperationError("capture", autoIndexDeadlineLabel, err)
	}
	if strings.TrimSpace(info.RootPath) == "" {
		info.RootPath = wd
	}
	repoID, err := db.ResolveRepo(store.RepositoryIdentity{
		RootPath:      info.RootPath,
		RemoteURL:     info.RemoteURL,
		RootCommit:    info.RootCommit,
		GitIdentity:   info.GitIdentity,
		CurrentBranch: info.CurrentBranch,
	}, ids.NewWithPrefix("repo_"), now)
	if err != nil {
		return fmt.Errorf("ensure repository: %w", err)
	}

	// Check if already captured
	existingArtID, err := db.FindSourceByIdentityInRepo(repoID, sourceIdentity)
	if err != nil {
		return err
	}

	if existingArtID != "" {
		contentHash := hashBody(art.Body)
		var currentRevisionID, currentContentHash string
		if err := db.QueryRow(`
			SELECT COALESCE(a.current_revision_id, ''), COALESCE(r.content_hash, '')
			FROM artifacts a
			LEFT JOIN artifact_revisions r ON r.id = a.current_revision_id
			WHERE a.id = ?`, existingArtID).Scan(&currentRevisionID, &currentContentHash); err != nil {
			return fmt.Errorf("read current capture revision: %w", err)
		}

		revID := currentRevisionID
		if revID == "" || currentContentHash != contentHash {
			revID = ids.NewWithPrefix("rev_")
			exStr, err := extractedJSONString(art.Extracted)
			if err != nil {
				return err
			}
			if err := db.InsertRevisionDirect(revID, existingArtID, contentHash, art.Body, exStr, now); err != nil {
				return fmt.Errorf("insert capture revision: %w", err)
			}
		}
		if _, err := db.Exec(`UPDATE artifacts
			SET title = ?, kind = ?, subtype = ?, status = ?, current_revision_id = ?, updated_at = ?, last_observed_at = ?
			WHERE id = ?`, art.Title, art.Kind, art.Subtype, art.Status, revID, now, now, existingArtID); err != nil {
			return fmt.Errorf("update captured artifact: %w", err)
		}
		if err := replaceCaptureChecklists(db, ids, existingArtID, revID, pr, now); err != nil {
			return err
		}
		if err := db.IndexArtifactFTS(existingArtID, art.Title, art.Body, relPath); err != nil {
			return fmt.Errorf("update capture search index: %w", err)
		}
		return outputCapture(cmd, existingArtID, relPath, asJSON)
	}

	// Create new
	artifactID := ids.New()
	revID := ids.NewWithPrefix("rev_")
	contentHash := hashBody(art.Body)
	exStr, err := extractedJSONString(art.Extracted)
	if err != nil {
		return err
	}

	authoredAt := repo.FileFirstCommitDateContext(captureCtx, wd, filepath.ToSlash(relPath))
	if err := captureCtx.Err(); err != nil {
		return indexOperationError("capture", autoIndexDeadlineLabel, err)
	}
	if authoredAt == "" {
		authoredAt = now
	}
	db.InsertArtifactDirect(artifactID, repoID, art.Kind, art.Subtype, art.Title, art.Status, revID, authoredAt, now)
	db.InsertRevisionDirect(revID, artifactID, contentHash, art.Body, exStr, now)
	db.InsertSourceDirect(ids.NewWithPrefix("src_"), artifactID, repoID, sourceType, relPath, sourceIdentity, art.FormatProfile, art.LayoutGroup, now)

	for _, td := range pr.Todos {
		todoID := ids.NewWithPrefix("todo_")
		done := 0
		if td.Done {
			done = 1
		}
		db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			todoID, artifactID, revID, td.Ordinal, td.Text, done, td.SourceFile, td.SourceLine, now)
	}
	for _, cr := range pr.Criteria {
		critID := ids.NewWithPrefix("crit_")
		done := 0
		if cr.Done {
			done = 1
		}
		db.Exec("INSERT INTO artifact_criteria (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, criteria_kind, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			critID, artifactID, revID, cr.Ordinal, cr.Text, done, cr.SourceFile, cr.SourceLine, cr.CriteriaKind, now)
	}

	db.IndexArtifactFTS(artifactID, art.Title, art.Body, relPath)
	return outputCapture(cmd, artifactID, relPath, asJSON)
}

func replaceCaptureChecklists(db *store.DB, ids *idgen.Factory, artifactID, revisionID string, pr todoparse.ParseResult, now string) error {
	if _, err := db.Exec("DELETE FROM artifact_todos WHERE artifact_id = ?", artifactID); err != nil {
		return fmt.Errorf("replace capture todos: %w", err)
	}
	for _, td := range pr.Todos {
		done := 0
		if td.Done {
			done = 1
		}
		if _, err := db.Exec("INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			ids.NewWithPrefix("todo_"), artifactID, revisionID, td.Ordinal, td.Text, done, td.SourceFile, td.SourceLine, now); err != nil {
			return fmt.Errorf("insert capture todo: %w", err)
		}
	}
	if _, err := db.Exec("DELETE FROM artifact_criteria WHERE artifact_id = ?", artifactID); err != nil {
		return fmt.Errorf("replace capture criteria: %w", err)
	}
	for _, cr := range pr.Criteria {
		done := 0
		if cr.Done {
			done = 1
		}
		if _, err := db.Exec("INSERT INTO artifact_criteria (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, criteria_kind, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			ids.NewWithPrefix("crit_"), artifactID, revisionID, cr.Ordinal, cr.Text, done, cr.SourceFile, cr.SourceLine, cr.CriteriaKind, now); err != nil {
			return fmt.Errorf("insert capture criterion: %w", err)
		}
	}
	return nil
}

func extractedJSONString(m map[string]any) (string, error) {
	if len(m) == 0 {
		return "", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("marshal extracted: %w", err)
	}
	return string(b), nil
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
