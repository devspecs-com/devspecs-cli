// Package scan orchestrates artifact discovery: walks the repo, dispatches
// adapters, and upserts artifacts/revisions/todos into the store.
package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/userident"
)

// Result holds scan summary counts.
type Result struct {
	Found     map[string]int // adapter name -> count
	New       int
	Updated   int
	Unchanged int
}

// Scanner runs adapters against a repo and persists results.
type Scanner struct {
	db       *store.DB
	ids      *idgen.Factory
	adapters []adapters.Adapter
}

// New creates a Scanner with the given store and adapters.
func New(db *store.DB, ids *idgen.Factory, adpts []adapters.Adapter) *Scanner {
	return &Scanner{db: db, ids: ids, adapters: adpts}
}

// Run scans the repo at repoRoot, using config if available.
func (s *Scanner) Run(ctx context.Context, repoRoot string, cfg *config.RepoConfig) (*Result, error) {
	result := &Result{Found: make(map[string]int)}
	now := time.Now().UTC().Format(time.RFC3339)

	repoID, err := s.ensureRepo(repoRoot, now)
	if err != nil {
		return nil, fmt.Errorf("ensure repo: %w", err)
	}

	for _, adapter := range s.adapters {
		candidates, err := adapter.Discover(ctx, repoRoot, cfg)
		if err != nil {
			continue
		}
		result.Found[adapter.Name()] += len(candidates)

		for _, c := range candidates {
			art, sources, todos, err := adapter.Parse(ctx, c)
			if err != nil {
				continue
			}
			if err := s.upsertArtifact(repoID, art, sources, todos, now, result); err != nil {
				return nil, fmt.Errorf("upsert artifact %q: %w", art.SourceIdentity, err)
			}
		}
	}

	s.recordScanMeta(repoID, repoRoot, now)
	return result, nil
}

func (s *Scanner) recordScanMeta(repoID, repoRoot, now string) {
	commit := repo.HeadCommit(repoRoot)
	user := userident.Detect(repoRoot)
	s.db.UpdateScanMeta(repoID, commit, user, now)
}

func (s *Scanner) ensureRepo(rootPath, now string) (string, error) {
	var id string
	err := s.db.QueryRow("SELECT id FROM repos WHERE root_path = ?", rootPath).Scan(&id)
	if err == nil {
		s.db.Exec("UPDATE repos SET updated_at = ? WHERE id = ?", now, id)
		return id, nil
	}
	id = s.ids.NewWithPrefix("repo_")
	_, err = s.db.Exec(
		"INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)",
		id, rootPath, now, now,
	)
	return id, err
}

func (s *Scanner) upsertArtifact(repoID string, art adapters.Artifact, sources []adapters.Source, todos []todoparse.Todo, now string, result *Result) error {
	// Check if artifact exists by source_identity
	var artifactID, currentRevID string
	err := s.db.QueryRow(
		"SELECT a.id, COALESCE(a.current_revision_id, '') FROM artifacts a JOIN sources s ON s.artifact_id = a.id WHERE s.source_identity = ?",
		art.SourceIdentity,
	).Scan(&artifactID, &currentRevID)

	contentHash := hashContent(art.Body)

	if err != nil {
		// New artifact
		artifactID = s.ids.New()
		revID := s.ids.NewWithPrefix("rev_")
		if err := s.insertArtifact(artifactID, repoID, art, revID, now); err != nil {
			return err
		}
		s.assignShortID(artifactID, art.SourceIdentity)
		if err := s.insertRevision(revID, artifactID, contentHash, art.Body, art.Extracted, now); err != nil {
			return err
		}
		for _, src := range sources {
			if err := s.insertSource(artifactID, repoID, src, now); err != nil {
				return err
			}
		}
		if err := s.replaceTodos(artifactID, revID, todos, now); err != nil {
			return err
		}
		s.replaceTags(artifactID, art, now)
		s.indexFTS(artifactID, art)
		result.New++
		return nil
	}

	// Update last_observed_at
	s.db.Exec("UPDATE artifacts SET last_observed_at = ?, updated_at = ? WHERE id = ?", now, now, artifactID)

	// Ensure short_id is set (covers artifacts created before v0.1)
	s.assignShortID(artifactID, art.SourceIdentity)

	if err := s.syncSources(artifactID, repoID, sources, now); err != nil {
		return err
	}

	// Check if content changed
	var existingHash string
	if currentRevID != "" {
		s.db.QueryRow("SELECT content_hash FROM artifact_revisions WHERE id = ?", currentRevID).Scan(&existingHash)
	}

	if existingHash == contentHash {
		s.replaceTags(artifactID, art, now)
		result.Unchanged++
		return nil
	}

	// New revision
	revID := s.ids.NewWithPrefix("rev_")
	if err := s.insertRevision(revID, artifactID, contentHash, art.Body, art.Extracted, now); err != nil {
		return err
	}
	s.db.Exec("UPDATE artifacts SET current_revision_id = ?, title = ?, status = ?, kind = ?, updated_at = ? WHERE id = ?",
		revID, art.Title, art.Status, art.Kind, now, artifactID)
	if err := s.replaceTodos(artifactID, revID, todos, now); err != nil {
		return err
	}
	s.replaceTags(artifactID, art, now)
	s.indexFTS(artifactID, art)
	result.Updated++
	return nil
}

func (s *Scanner) indexFTS(artifactID string, art adapters.Artifact) {
	sourcePath := ""
	if art.PrimaryPath != "" {
		sourcePath = art.PrimaryPath
	}
	s.db.IndexArtifactFTS(artifactID, art.Title, art.Body, sourcePath)
}

func (s *Scanner) insertArtifact(id, repoID string, art adapters.Artifact, revID, now string) error {
	_, err := s.db.Exec(
		`INSERT INTO artifacts (id, repo_id, kind, title, status, current_revision_id, created_at, updated_at, last_observed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, repoID, art.Kind, art.Title, art.Status, revID, now, now, now,
	)
	return err
}

func (s *Scanner) insertRevision(id, artifactID, contentHash, body string, extracted map[string]any, now string) error {
	var extractedArg any
	if len(extracted) > 0 {
		b, err := json.Marshal(extracted)
		if err != nil {
			return fmt.Errorf("marshal extracted: %w", err)
		}
		extractedArg = string(b)
	}
	_, err := s.db.Exec(
		"INSERT INTO artifact_revisions (id, artifact_id, content_hash, body, extracted_json, observed_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, artifactID, contentHash, body, extractedArg, now,
	)
	return err
}

func (s *Scanner) insertSource(artifactID, repoID string, src adapters.Source, now string) error {
	id := s.ids.NewWithPrefix("src_")
	fp := src.FormatProfile
	if fp == "" {
		fp = "generic"
	}
	var layoutArg any
	if src.LayoutGroup != "" {
		layoutArg = src.LayoutGroup
	}
	_, err := s.db.Exec(
		"INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		id, artifactID, repoID, src.SourceType, src.Path, src.SourceIdentity, fp, layoutArg, now, now,
	)
	return err
}

func (s *Scanner) syncSources(artifactID, repoID string, sources []adapters.Source, now string) error {
	if _, err := s.db.Exec("DELETE FROM sources WHERE artifact_id = ?", artifactID); err != nil {
		return err
	}
	for _, src := range sources {
		if err := s.insertSource(artifactID, repoID, src, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scanner) replaceTodos(artifactID, revID string, todos []todoparse.Todo, now string) error {
	// Delete existing todos for this artifact
	if _, err := s.db.Exec("DELETE FROM artifact_todos WHERE artifact_id = ?", artifactID); err != nil {
		return err
	}
	for _, todo := range todos {
		id := s.ids.NewWithPrefix("todo_")
		done := 0
		if todo.Done {
			done = 1
		}
		if _, err := s.db.Exec(
			"INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			id, artifactID, revID, todo.Ordinal, todo.Text, done, todo.SourceFile, todo.SourceLine, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scanner) assignShortID(artifactID, sourceIdentity string) {
	_ = s.db.AssignArtifactShortID(artifactID, idgen.ShortID(sourceIdentity))
}

func (s *Scanner) replaceTags(artifactID string, art adapters.Artifact, now string) {
	s.db.DeleteAutoTags(artifactID)

	for _, tag := range art.Tags {
		s.db.InsertTag(artifactID, tag, "frontmatter", now)
	}

	// Infer directory tag if no frontmatter tags
	if len(art.Tags) == 0 {
		relPath := ""
		if art.PrimaryPath != "" {
			// Find the relative path from sources
			rows, _ := s.db.Query("SELECT path FROM sources WHERE artifact_id = ? LIMIT 1", artifactID)
			if rows != nil {
				if rows.Next() {
					rows.Scan(&relPath)
				}
				rows.Close()
			}
		}
		if relPath != "" {
			if dirTag := markdown.InferDirectoryTag(relPath); dirTag != "" {
				s.db.InsertTag(artifactID, dirTag, "inferred", now)
			}
		}
	}
}

func hashContent(body string) string {
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	normalized = strings.Join(lines, "\n")
	h := sha256.Sum256([]byte(normalized))
	return "sha256:" + hex.EncodeToString(h[:])
}
