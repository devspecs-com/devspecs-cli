// Package scan orchestrates artifact discovery: walks the repo, dispatches
// adapters, and upserts artifacts/revisions/todos/criteria into the store.
package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/userident"
)

// Result type and tally helpers live in result.go and labels.go.

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
	adapterNames := make([]string, 0, len(s.adapters))
	for _, a := range s.adapters {
		adapterNames = append(adapterNames, a.Name())
	}
	result := newResult(adapterNames)
	now := time.Now().UTC().Format(time.RFC3339)

	matcher, _ := ignore.NewMatcher(repoRoot)
	ctx = ignore.WithContext(ctx, matcher)

	repoID, err := s.ensureRepo(repoRoot, now)
	if err != nil {
		return nil, fmt.Errorf("ensure repo: %w", err)
	}

	for _, adapter := range s.adapters {
		candidates, err := adapter.Discover(ctx, repoRoot, cfg)
		if err != nil {
			continue
		}

		for _, c := range candidates {
			art, sources, pr, err := adapter.Parse(ctx, c)
			if err != nil {
				continue
			}
			if err := s.upsertArtifact(repoRoot, repoID, adapter.Name(), art, sources, pr, now, result); err != nil {
				return nil, fmt.Errorf("upsert artifact %q: %w", art.SourceIdentity, err)
			}
		}
	}

	s.recordScanMeta(repoID, repoRoot, now)
	result.finalizeSourcesBreakdown()
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

func (s *Scanner) upsertArtifact(repoRoot, repoID, adapterName string, art adapters.Artifact, sources []adapters.Source, pr todoparse.ParseResult, now string, result *Result) error {
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
		if err := s.insertArtifact(artifactID, repoRoot, repoID, art, sources, revID, now); err != nil {
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
		if err := s.replaceTodos(artifactID, revID, pr.Todos, now); err != nil {
			return err
		}
		if err := s.replaceCriteria(artifactID, revID, pr.Criteria, now); err != nil {
			return err
		}
		s.replaceTags(artifactID, art, now)
		s.indexFTS(artifactID, art)
		result.New++
		tallyIndexed(result, adapterName, sources, art)
		return nil
	}

	// Refresh last_observed_at only; updated_at moves on new revision or capture/status updates.
	s.db.Exec("UPDATE artifacts SET last_observed_at = ? WHERE id = ?", now, artifactID)

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
		// Body hash unchanged: keep existing revision row (and extracted_json) until
		// file content changes — CLI logic-only enrichments won't rewrite revisions alone.
		s.replaceTags(artifactID, art, now)
		result.Unchanged++
		tallyIndexed(result, adapterName, sources, art)
		return nil
	}

	// New revision
	revID := s.ids.NewWithPrefix("rev_")
	if err := s.insertRevision(revID, artifactID, contentHash, art.Body, art.Extracted, now); err != nil {
		return err
	}
	s.db.Exec("UPDATE artifacts SET current_revision_id = ?, title = ?, status = ?, kind = ?, updated_at = ? WHERE id = ?",
		revID, art.Title, art.Status, art.Kind, now, artifactID)
	if err := s.replaceTodos(artifactID, revID, pr.Todos, now); err != nil {
		return err
	}
	if err := s.replaceCriteria(artifactID, revID, pr.Criteria, now); err != nil {
		return err
	}
	s.replaceTags(artifactID, art, now)
	s.indexFTS(artifactID, art)
	result.Updated++
	tallyIndexed(result, adapterName, sources, art)
	return nil
}

func (s *Scanner) indexFTS(artifactID string, art adapters.Artifact) {
	sourcePath := ""
	if art.PrimaryPath != "" {
		sourcePath = art.PrimaryPath
	}
	s.db.IndexArtifactFTS(artifactID, art.Title, art.Body, sourcePath)
}

func (s *Scanner) insertArtifact(id, repoRoot, repoID string, art adapters.Artifact, sources []adapters.Source, revID, now string) error {
	authoredAt := resolveAuthoredAt(repoRoot, art, sources, now)
	_, err := s.db.Exec(
		`INSERT INTO artifacts (id, repo_id, kind, title, status, current_revision_id, created_at, updated_at, last_observed_at, authored_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, repoID, art.Kind, art.Title, art.Status, revID, now, now, now, authoredAt,
	)
	return err
}

func resolveAuthoredAt(repoRoot string, art adapters.Artifact, sources []adapters.Source, now string) string {
	var rel string
	if art.PrimaryPath != "" {
		if rel2, err := filepath.Rel(repoRoot, art.PrimaryPath); err == nil {
			rel = filepath.ToSlash(rel2)
		}
	}
	if rel == "" && len(sources) > 0 && sources[0].Path != "" {
		rel = filepath.ToSlash(sources[0].Path)
	}
	if rel == "" {
		return now
	}
	if d := repo.FileFirstCommitDate(repoRoot, rel); d != "" {
		return d
	}
	return now
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
		fp = format.ProfileGeneric
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

func (s *Scanner) replaceCriteria(artifactID, revID string, criteria []todoparse.Criterion, now string) error {
	if _, err := s.db.Exec("DELETE FROM artifact_criteria WHERE artifact_id = ?", artifactID); err != nil {
		return err
	}
	for _, c := range criteria {
		id := s.ids.NewWithPrefix("crit_")
		done := 0
		if c.Done {
			done = 1
		}
		if _, err := s.db.Exec(
			"INSERT INTO artifact_criteria (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, criteria_kind, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			id, artifactID, revID, c.Ordinal, c.Text, done, c.SourceFile, c.SourceLine, c.CriteriaKind, now,
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
