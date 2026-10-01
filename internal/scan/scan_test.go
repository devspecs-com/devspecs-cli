package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/codecomment"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/openspec"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/testcase"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	docsections "github.com/devspecs-com/devspecs-cli/internal/sections"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cancellationTestAdapter struct {
	name                string
	sourceIdentity      string
	entered             chan struct{}
	waitForCancellation bool
}

func (a *cancellationTestAdapter) Name() string {
	return a.name
}

func (a *cancellationTestAdapter) Discover(context.Context, string, *config.RepoConfig) ([]adapters.Candidate, error) {
	return []adapters.Candidate{{RelPath: a.sourceIdentity + ".md", AdapterName: a.name}}, nil
}

func (a *cancellationTestAdapter) Parse(ctx context.Context, candidate adapters.Candidate) (adapters.Artifact, []adapters.Source, todoparse.ParseResult, error) {
	if a.waitForCancellation {
		close(a.entered)
		<-ctx.Done()
		return adapters.Artifact{}, nil, todoparse.ParseResult{}, ctx.Err()
	}
	artifact := adapters.Artifact{
		SourceIdentity: a.sourceIdentity,
		Kind:           config.KindPlan,
		Title:          a.sourceIdentity,
		Status:         "implementing",
		PrimaryPath:    candidate.RelPath,
		Body:           "# " + a.sourceIdentity,
	}
	source := adapters.Source{
		SourceType:     a.name,
		Path:           candidate.RelPath,
		SourceIdentity: a.sourceIdentity,
	}
	return artifact, []adapters.Source{source}, todoparse.ParseResult{}, nil
}

func setupTestRepo(t *testing.T) (string, *store.DB) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	plansDir := filepath.Join(tmp, "repo", "plans")
	require.NoError(t, os.MkdirAll(plansDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, "auth.md"), []byte("# Auth Plan\n\n- [ ] Add login\n- [x] Design schema\n"), 0o644))

	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return filepath.Join(tmp, "repo"), db
}

func TestScan_WhenCanceledAfterPartialWrite_RollsBackTransaction(t *testing.T) {
	repoRoot := t.TempDir()
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	entered := make(chan struct{})
	first := &cancellationTestAdapter{name: "first", sourceIdentity: "first"}
	second := &cancellationTestAdapter{
		name:                "second",
		sourceIdentity:      "second",
		entered:             entered,
		waitForCancellation: true,
	}
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{first, second})
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	type scanOutcome struct {
		result *Result
		err    error
	}
	completed := make(chan scanOutcome, 1)
	go func() {
		result, scanErr := scanner.RunWithOptions(ctx, repoRoot, nil, RunOptions{
			UseTransaction:       true,
			SkipAuthoredAtLookup: true,
			FileWorkerCount:      1,
		})
		completed <- scanOutcome{result: result, err: scanErr}
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "scan did not reach cancellable adapter")
	}

	cancel()
	var outcome scanOutcome
	select {
	case outcome = <-completed:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "scan did not return after cancellation")
	}
	var artifactCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM artifacts").Scan(&artifactCount))
	var completedScanCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM repos WHERE last_scan_at IS NOT NULL AND last_scan_at <> ''").Scan(&completedScanCount))

	assert.Nil(t, outcome.result)
	assert.ErrorIs(t, outcome.err, context.Canceled)
	assert.Zero(t, artifactCount)
	assert.Zero(t, completedScanCount)
}

type seededMarkdownScanState struct {
	RepoID     string
	ArtifactID string
	RevisionID string
	UpdatedAt  string
}

func seedExistingMarkdownScanState(t *testing.T, db *store.DB, repoRoot string) seededMarkdownScanState {
	t.Helper()
	return seedExistingMarkdownFileState(t, db, repoRoot, "plans/auth.md", "artifact_existing", "repo_existing", "2026-08-10T00:00:00Z")
}

func seedExistingMarkdownFileState(t *testing.T, db *store.DB, repoRoot, relPath, artifactID, repoName, now string) seededMarkdownScanState {
	t.Helper()
	revisionID := artifactID + "_revision"
	artifact, sources, parsed, err := (&markdown.Adapter{}).Parse(context.Background(), adapters.Candidate{
		PrimaryPath: filepath.Join(repoRoot, filepath.FromSlash(relPath)),
		RelPath:     relPath,
		AdapterName: "markdown",
	})
	require.NoError(t, err)
	require.Len(t, sources, 1)
	repoID, err := db.ResolveRepo(store.RepositoryIdentity{RootPath: repoRoot}, repoName, now)
	require.NoError(t, err)
	extracted, err := json.Marshal(artifact.Extracted)
	require.NoError(t, err)
	require.NoError(t, db.InsertArtifactDirect(artifactID, repoID, artifact.Kind, artifact.Subtype, artifact.Title, artifact.Status, revisionID, now, now))
	require.NoError(t, db.InsertRevisionDirect(revisionID, artifactID, hashContent(artifact.Body), artifact.Body, string(extracted), now))
	require.NoError(t, db.InsertSourceDirect(artifactID+"_source", artifactID, repoID, sources[0].SourceType, sources[0].Path, sources[0].SourceIdentity, sources[0].FormatProfile, sources[0].LayoutGroup, now))
	sections := docsections.AssignStableIDs(docsections.ExtractMarkdown(artifact.Body), artifactID, revisionID, relPath)
	require.NoError(t, db.ReplaceArtifactSections(artifactID, revisionID, sections, now))
	for index, todo := range parsed.Todos {
		_, err := db.Exec(`INSERT INTO artifact_todos
			(id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, fmt.Sprintf("%s_todo_%d", artifactID, index), artifactID, revisionID, todo.Ordinal, todo.Text, todo.Done, todo.SourceFile, todo.SourceLine, now)
		require.NoError(t, err)
	}

	return seededMarkdownScanState{RepoID: repoID, ArtifactID: artifactID, RevisionID: revisionID, UpdatedAt: now}
}

func TestScan_DetectsMarkdownPlans(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	result, err := s.Run(context.Background(), repoRoot, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Found["markdown"],
		"expected 1 markdown found, got %d", result.Found["markdown"])
	assert.Equal(t, 1, result.New,
		"expected 1 new, got %d", result.New)

}

func TestScan_StableIDs(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	seeded := seedExistingMarkdownScanState(t, db, repoRoot)
	s := New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})

	_, err := s.Run(context.Background(), repoRoot, nil)
	require.NoError(t, err)

	var got string
	require.NoError(t, db.QueryRow("SELECT id FROM artifacts LIMIT 1").Scan(&got))
	assert.Equal(t, seeded.ArtifactID, got,
		"ID not stable across rescan: got %q want %q", got, seeded.ArtifactID)

}

func TestScan_SourceIdentityIsScopedToLogicalRepository(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})
	rootA := filepath.Join(tmp, "repo-a")
	rootB := filepath.Join(tmp, "repo-b")
	require.NoError(t, os.MkdirAll(filepath.Join(rootA, "plans"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(rootB, "plans"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rootA, "plans", "shared.md"), []byte("# Shared Plan\n\nRepository A content.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(rootB, "plans", "shared.md"), []byte("# Shared Plan\n\nRepository B content.\n"), 0o644))
	now := "2026-08-11T00:00:00Z"
	repoA, err := db.ResolveRepo(store.RepositoryIdentity{RootPath: rootA}, "repo_a", now)
	require.NoError(t, err)
	require.NoError(t, db.InsertArtifactDirect("artifact_a", repoA, "plan", "", "Shared Plan", "draft", "revision_a", now, now))
	require.NoError(t, db.InsertSourceDirect("source_a", "artifact_a", repoA, "markdown", "plans/shared.md", "plans/shared.md|markdown", "generic", "", now))

	_, err = scanner.Run(context.Background(), rootB, nil)
	require.NoError(t, err)

	var artifacts, repos int
	{
		err := db.QueryRow(`SELECT COUNT(*) FROM artifacts`).Scan(&artifacts)
		require.NoError(t, err)
	}
	{

		err := db.QueryRow(`SELECT COUNT(*) FROM repos`).Scan(&repos)
		require.NoError(t, err)
	}
	assert.Equal(t, 2, artifacts)
	assert.Equal(t, 2, repos)

}

func TestScan_RepairsUnanimousLegacySourceOwnership(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	db.SetMaxOpenConns(1)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})
	rootA := filepath.Join(tmp, "repo-a")
	rootB := filepath.Join(tmp, "repo-b")
	require.NoError(t, os.MkdirAll(filepath.Join(rootA, "plans"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(rootB, "plans"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rootA, "plans", "shared.md"), []byte("# Shared Plan\n\nRepository A evidence.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(rootB, "plans", "shared.md"), []byte("# Shared Plan\n\nRepository B evidence.\n"), 0o644))
	now := "2026-08-07T00:00:00Z"
	seeded := seedExistingMarkdownFileState(t, db, rootA, "plans/shared.md", "artifact_shared", "repo_a", now)
	repoB, err := db.ResolveRepo(store.RepositoryIdentity{RootPath: rootB}, "repo_b", now)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE sources SET repo_id = ? WHERE artifact_id = ?`, repoB, seeded.ArtifactID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO concepts
		(id, repo_id, canonical, kind, forms_json, document_frequency, inverse_document_frequency, created_at, updated_at)
		VALUES ('legacy_concept', ?, 'repository', 'term', '[]', 1, 1, ?, ?)`, seeded.RepoID, now, now)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO concept_mentions
		(id, concept_id, artifact_id, field, weight, evidence_json, created_at)
		VALUES ('legacy_mention', 'legacy_concept', ?, 'body', 1, '{}', ?)`, seeded.ArtifactID, now)
	require.NoError(t, err)

	result, err := scanner.RunWithOptions(context.Background(), rootB, nil, RunOptions{UseTransaction: true, PhaseTiming: true})
	require.NoError(t, err)
	assert.Zero(t, result.New)
	assert.Equal(t, 1, result.Updated)

	var artifactRepo, sourceRepo, body string
	{
		err := db.QueryRow(`SELECT a.repo_id, s.repo_id, rv.body
		FROM artifacts a JOIN sources s ON s.artifact_id = a.id
		JOIN artifact_revisions rv ON rv.id = a.current_revision_id
		WHERE a.id = ?`, seeded.ArtifactID).Scan(&artifactRepo, &sourceRepo, &body)
		require.NoError(t, err)
	}
	assert.Equal(t, repoB, artifactRepo)
	assert.Equal(t, repoB, sourceRepo)
	assert.Contains(t, body, "Repository B evidence")

	var oldMentions int
	{
		err := db.QueryRow(`SELECT COUNT(*) FROM concept_mentions cm
		JOIN concepts c ON c.id = cm.concept_id
		WHERE cm.artifact_id = ? AND c.repo_id <> ?`, seeded.ArtifactID, repoB).Scan(&oldMentions)
		require.NoError(t, err)
	}
	require.Equal(t, 0, oldMentions,
		"legacy graph mentions remain: %d", oldMentions)

}

func TestScan_RepositoryRegainsArtifactAfterOwnershipMovesToAnotherRepository(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	require.NoError(t, err)
	defer db.Close()

	rootA := filepath.Join(tmp, "repo-a")
	rootB := filepath.Join(tmp, "repo-b")
	require.NoError(t, os.MkdirAll(filepath.Join(rootA, "plans"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(rootB, "plans"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rootA, "plans", "shared.md"), []byte("# Shared Plan\n\nRepository A evidence.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(rootB, "plans", "shared.md"), []byte("# Shared Plan\n\nRepository B evidence.\n"), 0o644))
	now := "2026-08-11T00:00:00Z"
	_, err = db.ResolveRepo(store.RepositoryIdentity{RootPath: rootA}, "repo_a", now)
	require.NoError(t, err)
	repoB, err := db.ResolveRepo(store.RepositoryIdentity{RootPath: rootB}, "repo_b", now)
	require.NoError(t, err)
	require.NoError(t, db.InsertArtifactDirect("artifact_b", repoB, "plan", "", "Shared Plan", "draft", "revision_b", now, now))
	require.NoError(t, db.InsertSourceDirect("source_b", "artifact_b", repoB, "markdown", "plans/shared.md", "plans/shared.md|markdown", "generic", "", now))
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})

	_, err = scanner.RunWithOptions(context.Background(), rootA, nil, RunOptions{UseTransaction: true})
	require.NoError(t, err)

	var artifacts int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM artifacts`).Scan(&artifacts))
	assert.Equal(t, 2, artifacts,
		"repo A did not regain its own artifact: %d", artifacts)
}

func TestScan_NoDuplicateOnUnchanged(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	seedExistingMarkdownScanState(t, db, repoRoot)
	s := New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})

	result, err := s.Run(context.Background(), repoRoot, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Unchanged,
		"expected 1 unchanged, got %d", result.Unchanged)
	assert.Equal(t, 0, result.New,
		"expected 0 new, got %d", result.New)

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM artifacts").Scan(&count))
	assert.Equal(t, 1, count,
		"expected 1 artifact, got %d", count)

}

func TestScan_RebuildsEvidenceGraphDiagnostics(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	{
		err := os.WriteFile(filepath.Join(repoRoot, "plans", "auth-tests.md"), []byte("# Auth Token Tests\n\nSee plans/auth.md for the auth token rollout.\n"), 0o644)
		require.NoError(t, err)
	}

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})

	result, err := s.Run(context.Background(), repoRoot, nil)
	require.NoError(t, err)
	require.NotNil(t, result.EvidenceGraph,
		"expected evidence graph diagnostics")
	require.NotEqual(t, 0, result.EvidenceGraph.ConceptsIndexed,
		"expected indexed concepts")
	require.NotEqual(t, 0, result.EvidenceGraph.MentionsIndexed,
		"expected indexed concept mentions")
	require.NotEqual(t, 0, result.EvidenceGraph.EdgesByType[edgeTypeMentionsSameConcept],
		"expected shared-concept edge diagnostics: %#v", result.EvidenceGraph)
	require.NotEqual(t, 0, result.EvidenceGraph.EdgesByType[edgeTypeExplicitReference],
		"expected explicit path-reference edge diagnostics: %#v", result.EvidenceGraph)

}

func TestScan_RebuildsEvidenceGraphIdempotently(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "plans", "auth-tests.md"), []byte("# Auth Token Tests\n\nSee plans/auth.md for the auth token rollout.\n"), 0o644))
	s := New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})
	seeded := seedExistingMarkdownScanState(t, db, repoRoot)
	seedExistingMarkdownFileState(t, db, repoRoot, "plans/auth-tests.md", "artifact_auth_tests", "repo_existing", seeded.UpdatedAt)
	artifacts, err := s.loadEvidenceArtifacts(seeded.RepoID)
	require.NoError(t, err)
	built := buildEvidenceGraph(seeded.RepoID, artifacts)
	require.NoError(t, db.ReplaceRepoEvidence(seeded.RepoID, built.concepts, built.mentions, built.edges, seeded.UpdatedAt))
	firstConcepts := tableCount(t, db, "concepts")
	firstMentions := tableCount(t, db, "concept_mentions")
	firstEdges := tableCount(t, db, "artifact_edges")

	_, err = s.Run(context.Background(), repoRoot, nil)
	require.NoError(t, err)

	assert.Equal(t, firstConcepts, tableCount(t, db, "concepts"),
		"concept count changed after rescan")
	assert.Equal(t, firstMentions, tableCount(t, db, "concept_mentions"),
		"mention count changed after rescan")
	assert.Equal(t, firstEdges, tableCount(t, db, "artifact_edges"),
		"edge count changed after rescan")
}

func TestScan_ExperimentalGitEvidenceStoresFactsAndEdges(t *testing.T) {
	repoRoot, db, s := setupGitEvidenceTestRepo(t)

	result, err := s.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{
		UseTransaction:     true,
		IncludeGitEvidence: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result.GitEvidence,
		"expected git evidence diagnostics")
	require.Equal(t, "single_commit", result.GitEvidence.HistoryShape,
		"expected single_commit history shape, got %#v", result.GitEvidence)
	require.Equal(t, 1, result.GitEvidence.CommitsStored,
		"expected one stored git commit, got %#v", result.GitEvidence)
	require.NotEqual(t, 0, result.GitEvidence.EdgesByType[edgeTypeCoChangedWith],
		"expected co-change edge diagnostics, got %#v", result.GitEvidence)
	require.NotEmpty(t, result.GitEvidence.TopEdges,
		"expected pathful git edge examples, got %#v", result.GitEvidence)

	example := result.GitEvidence.TopEdges[0]
	assert.NotEmpty(t, example.SourcePath)
	assert.NotEmpty(t, example.TargetPath)
	assert.NotEmpty(t, example.Commits)
	assert.NotEmpty(t, example.ConfidenceRule)

	counts, err := db.CountGitFacts(resultRepoID(t, db))
	require.NoError(t, err)
	assert.Equal(t, 1, counts.Commits)
	assert.Equal(t, 2, counts.Files)

}

func TestScan_ExperimentalGitEvidenceIsIdempotent(t *testing.T) {
	repoRoot, db, s := setupGitEvidenceTestRepo(t)
	repoID := seedExistingGitFacts(t, db, repoRoot)
	want, err := db.CountGitFacts(repoID)
	require.NoError(t, err)

	_, err = s.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{
		UseTransaction:     true,
		IncludeGitEvidence: true,
	})
	require.NoError(t, err)

	got, err := db.CountGitFacts(repoID)
	require.NoError(t, err)
	assert.Equal(t, want.Commits, got.Commits)
	assert.Equal(t, want.Files, got.Files)
}

func TestScan_DefaultModeClearsExperimentalGitEvidence(t *testing.T) {
	repoRoot, db, s := setupGitEvidenceTestRepo(t)
	repoID := seedExistingGitFacts(t, db, repoRoot)

	result, err := s.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true})
	require.NoError(t, err)

	assert.Nil(t, result.GitEvidence,
		"default scan should not emit git diagnostics: %#v", result.GitEvidence)
	counts, err := db.CountGitFacts(repoID)
	require.NoError(t, err)
	assert.Zero(t, counts.Commits)
	assert.Zero(t, counts.Files)

}

func setupGitEvidenceTestRepo(t *testing.T) (string, *store.DB, *Scanner) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}

	repoRoot, db := setupTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "plans", "auth-tests.md"), []byte("# Auth Tests\n\n- [ ] Verify auth flow\n"), 0o644))
	runGitCommand(t, repoRoot, "init")
	runGitCommand(t, repoRoot, "checkout", "-b", "main")
	runGitCommand(t, repoRoot, "config", "user.email", "test@example.com")
	runGitCommand(t, repoRoot, "config", "user.name", "Test User")
	runGitCommand(t, repoRoot, "add", ".")
	runGitCommand(t, repoRoot, "commit", "-m", "add auth docs")

	return repoRoot, db, New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})
}

func seedExistingGitFacts(t *testing.T, db *store.DB, repoRoot string) string {
	t.Helper()
	now := "2026-08-10T00:00:00Z"
	repoID, err := db.ResolveRepo(store.RepositoryIdentity{RootPath: repoRoot}, "git_evidence_repo", now)
	require.NoError(t, err)
	commits := []store.GitCommitInput{{
		RepoID:       repoID,
		SHA:          "existing_commit",
		Branch:       "main",
		Message:      "add auth docs",
		CommittedAt:  now,
		FilesChanged: 2,
		HistoryShape: "single_commit",
	}}
	files := []store.GitCommitFileInput{
		{RepoID: repoID, CommitSHA: "existing_commit", FilePath: "plans/auth.md", ChangeType: "A"},
		{RepoID: repoID, CommitSHA: "existing_commit", FilePath: "plans/auth-tests.md", ChangeType: "A"},
	}
	require.NoError(t, db.ReplaceRepoGitFacts(repoID, commits, files, now))

	return repoID
}

func TestScan_ExperimentalGitEvidenceNonGitDirectory(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	s := New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})
	result, err := s.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{
		UseTransaction:     true,
		IncludeGitEvidence: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result.GitEvidence,
		"expected git evidence diagnostics")
	assert.Contains(t, []string{"non_git", "unavailable"}, result.GitEvidence.HistoryShape)
	assert.Zero(t, result.GitEvidence.CommitsStored)
	assert.Zero(t, result.GitEvidence.EdgesIndexed)

}

func TestScan_NewRevisionOnContentChange(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	seedExistingMarkdownScanState(t, db, repoRoot)
	s := New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})
	planPath := filepath.Join(repoRoot, "plans", "auth.md")
	require.NoError(t, os.WriteFile(planPath, []byte("# Auth Plan v2\n\n- [ ] New task\n"), 0o644))

	result, err := s.Run(context.Background(), repoRoot, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Updated,
		"expected 1 updated, got %d", result.Updated)

	var revCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM artifact_revisions").Scan(&revCount))
	assert.Equal(t, 2, revCount,
		"expected 2 revisions, got %d", revCount)

}

func TestScan_StoresTodosOnInitialRevision(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	_, err := s.Run(context.Background(), repoRoot, nil)
	require.NoError(t, err)

	var todoCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM artifact_todos").Scan(&todoCount))
	assert.Equal(t, 2, todoCount,
		"expected 2 todos after first scan, got %d", todoCount)

}

func TestScan_RefreshesTodosOnRevision(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	seedExistingMarkdownScanState(t, db, repoRoot)
	s := New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})
	planPath := filepath.Join(repoRoot, "plans", "auth.md")
	require.NoError(t, os.WriteFile(planPath, []byte("# Auth Plan\n\n- [ ] Only one todo now\n"), 0o644))

	_, err := s.Run(context.Background(), repoRoot, nil)
	require.NoError(t, err)

	var todoCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM artifact_todos").Scan(&todoCount))
	assert.Equal(t, 1, todoCount,
		"expected 1 todo after revision, got %d", todoCount)

}

func TestScan_PersistsMarkdownSectionsAndTodoLinks(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	{

		_, err := s.Run(context.Background(), repoRoot, nil)
		require.NoError(t, err)
	}

	var sectionID, heading, sourcePath string
	{
		err := db.QueryRow("SELECT id, heading_path, source_path FROM artifact_sections LIMIT 1").Scan(&sectionID, &heading, &sourcePath)
		require.NoError(t, err)
	}
	require.NotEqual(t, "", sectionID,
		"expected section id")
	require.Equal(t, "Auth Plan", heading,
		"expected Auth Plan heading, got %q", heading)
	require.Equal(t, "plans/auth.md", sourcePath,
		"expected relative source path, got %q", sourcePath)

	var todoSectionID string
	{
		err := db.QueryRow("SELECT section_id FROM artifact_todos WHERE text = 'Add login'").Scan(&todoSectionID)
		require.NoError(t, err)
	}
	require.Equal(t, sectionID, todoSectionID,
		"expected todo to link to section %q, got %q", sectionID, todoSectionID)

	hits, err := db.FindArtifactSections("design schema", store.FilterParams{RepoRoot: repoRoot}, 10)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, sectionID, hits[0].ID)

}

func TestScan_SeparatesCriteriaFromTodos(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	plansDir := filepath.Join(tmp, "repo", "plans")
	os.MkdirAll(plansDir, 0o755)
	content := "# Plan\n\n## Tasks\n\n- [ ] Do work\n\n## Auditable success criteria\n\n- [ ] Integration passes\n"
	os.WriteFile(filepath.Join(plansDir, "mixed.md"), []byte(content), 0o644)

	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	{
		_, err := s.Run(context.Background(), filepath.Join(tmp, "repo"), nil)
		require.NoError(t, err)
	}

	var nTodos, nCrit int
	db.QueryRow("SELECT COUNT(*) FROM artifact_todos").Scan(&nTodos)
	db.QueryRow("SELECT COUNT(*) FROM artifact_criteria").Scan(&nCrit)
	assert.Equal(t, 1, nTodos)
	assert.Equal(t, 1, nCrit)

}

func TestScan_FrontmatterOverridesHeuristics(t *testing.T) {
	tmp := t.TempDir()
	plansDir := filepath.Join(tmp, "repo", "plans")
	os.MkdirAll(plansDir, 0o755)
	os.WriteFile(filepath.Join(plansDir, "test.md"), []byte("---\ntitle: Override Title\nkind: spec\nstatus: approved\n---\n# Ignored\n"), 0o644)

	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

	defer db.Close()

	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	cfg := &config.RepoConfig{Sources: []config.SourceConfig{{Type: "markdown", Paths: []string{"plans"}}}}
	s.Run(context.Background(), filepath.Join(tmp, "repo"), cfg)

	var title, kind, status string
	db.QueryRow("SELECT title, kind, status FROM artifacts LIMIT 1").Scan(&title, &kind, &status)
	assert.Equal(t, "Override Title", title,
		"expected 'Override Title', got %q", title)
	assert.Equal(t, "spec", kind,
		"expected 'spec', got %q", kind)
	assert.Equal(t, "approved", status,
		"expected 'approved', got %q", status)

}

func TestScan_PersistsExtractedJSONWithFrontmatter(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)
	plansDir := filepath.Join(tmp, "repo", "plans")
	os.MkdirAll(plansDir, 0o755)
	content := "---\ntitle: FM Title\n---\n# H1\n\nBody\n"
	os.WriteFile(filepath.Join(plansDir, "fm.md"), []byte(content), 0o644)
	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

	defer db.Close()
	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	{
		_, err := s.Run(context.Background(), filepath.Join(tmp, "repo"), nil)
		require.NoError(t, err)
	}

	var ex string
	err = db.QueryRow(`SELECT COALESCE(rv.extracted_json, '') FROM artifact_revisions rv JOIN artifacts a ON a.current_revision_id = rv.id LIMIT 1`).Scan(&ex)
	require.NoError(t, err)
	require.NotEqual(t, "", ex,
		"expected non-empty extracted_json")
	require.True(t, strings.Contains(ex, "frontmatter"),
		"expected frontmatter in extracted json: %s", ex)
	require.True(t, strings.Contains(ex, "classifier"),
		"expected classifier metadata in extracted json: %s", ex)

	// Apart from scan-level classifier metadata, preserve the markdown adapter extraction.
	md := &markdown.Adapter{}
	repoRoot := filepath.Join(tmp, "repo")
	abs := filepath.Join(repoRoot, "plans", "fm.md")
	wantArt, _, _, err := md.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: abs,
		RelPath:     "plans/fm.md",
		AdapterName: "markdown",
	})
	require.NoError(t, err)

	var got map[string]any
	{
		err := json.Unmarshal([]byte(ex), &got)
		require.NoError(t, err,
			"stored json: %v", err)
	}

	wantCanon, err := extractedJSONRoundTrip(wantArt.Extracted)
	require.NoError(t, err)

	delete(got, "classifier")
	require.Len(t, got, len(wantCanon))
	for key, want := range wantCanon {
		assert.Equal(t, want, got[key], "extracted key %q", key)
	}

}

func TestScan_PersistsClassifierMetadata(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)
	plansDir := filepath.Join(tmp, "repo", "docs", "plans")
	os.MkdirAll(plansDir, 0o755)
	content := "---\nstatus: active\n---\n# Auth Token Migration Plan\n\n## Tasks\n\n- [ ] Add session guard\n"
	os.WriteFile(filepath.Join(plansDir, "2026-05-14-auth-token-plan.md"), []byte(content), 0o644)

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	{
		_, err := s.Run(context.Background(), filepath.Join(tmp, "repo"), nil)
		require.NoError(t, err)
	}

	var ex string
	err = db.QueryRow(`SELECT COALESCE(rv.extracted_json, '') FROM artifact_revisions rv JOIN artifacts a ON a.current_revision_id = rv.id LIMIT 1`).Scan(&ex)
	require.NoError(t, err)

	var got map[string]any
	{
		err := json.Unmarshal([]byte(ex), &got)
		require.NoError(t, err)
	}

	classifier, ok := got["classifier"].(map[string]any)
	require.True(t, ok,
		"missing classifier metadata: %#v", got)
	require.Equal(t, "declarative_document_models_v0", classifier["evaluator"],
		"evaluator = %#v", classifier["evaluator"])

	winner, ok := classifier["winner"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "plan", winner["classifier"],
		"winner classifier = %#v", winner["classifier"])
	require.Equal(t, "plan.implementation_plan", winner["family"],
		"winner family = %#v", winner["family"])

}

func TestScan_SubtypeFirstNonIntentClassifierQuarantinesAgentInstructions(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := filepath.Join(tmp, "repo")
	{
		err := os.MkdirAll(repoRoot, 0o755)
		require.NoError(t, err)
	}

	content := "# Project Instructions\n\n## Rules\n\nAlways run tests and follow repo conventions.\n"
	{
		err := os.WriteFile(filepath.Join(repoRoot, "CLAUDE.md"), []byte(content), 0o644)
		require.NoError(t, err)
	}

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	cfg := config.WithIntentCandidateDiscovery(nil, true)
	{
		_, err := s.Run(context.Background(), repoRoot, cfg)
		require.NoError(t, err)
	}

	var kind, subtype, ex string
	err = db.QueryRow(`SELECT a.kind, a.subtype, COALESCE(rv.extracted_json, '')
		FROM artifacts a
		JOIN sources s ON s.artifact_id = a.id
		JOIN artifact_revisions rv ON rv.id = a.current_revision_id
		WHERE s.path = 'CLAUDE.md'`).Scan(&kind, &subtype, &ex)
	require.NoError(t, err)
	assert.Equal(t, config.KindMarkdownArtifact, kind)
	assert.Equal(t, config.SubtypeAgentInstruction, subtype)

	var got map[string]any
	{
		err := json.Unmarshal([]byte(ex), &got)
		require.NoError(t, err)
	}
	require.Equal(t, "protocol", got["mode"],
		"mode = %#v", got["mode"])

	classifier, ok := got["classifier"].(map[string]any)
	require.True(t, ok)
	winner, ok := classifier["winner"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "protocol", winner["classifier"])
	assert.Equal(t, "protocol", winner["mode"])

}

func TestScan_DefaultDiscoverySkipsCompoundPlanningDir(t *testing.T) {
	s, _, repoRoot := setupCompoundPlanningScanner(t)

	result, err := s.Run(context.Background(), repoRoot, nil)

	require.NoError(t, err)
	assert.Zero(t, result.Found["markdown"])
}

func TestScan_ExperimentalIntentDiscoveryIndexesCompoundPlanningDir(t *testing.T) {
	s, db, repoRoot := setupCompoundPlanningScanner(t)
	cfg := config.WithIntentCandidateDiscovery(nil, true)

	result, err := s.Run(context.Background(), repoRoot, cfg)

	require.NoError(t, err)
	assert.Equal(t, 1, result.Found["markdown"])

	var sourcePath, ex string
	err = db.QueryRow(`SELECT s.path, COALESCE(rv.extracted_json, '')
		FROM sources s
		JOIN artifacts a ON a.id = s.artifact_id
		JOIN artifact_revisions rv ON rv.id = a.current_revision_id
		WHERE s.source_type = 'markdown'
		LIMIT 1`).Scan(&sourcePath, &ex)
	require.NoError(t, err)
	assert.Equal(t, "docs/exec-plans/active/cache-warmup.md", sourcePath)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(ex), &got))
	classifier, ok := got["classifier"].(map[string]any)
	require.True(t, ok)
	reasons, ok := classifier["discovery_reasons"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, reasons)
	assert.True(t, anyStringHasPrefix(reasons, "intent_path_token:plan"))
}

func setupCompoundPlanningScanner(t *testing.T) (*Scanner, *store.DB, string) {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)
	docDir := filepath.Join(tmp, "repo", "docs", "exec-plans", "active")
	os.MkdirAll(docDir, 0o755)
	content := "# Cache Warmup\n\n## Goals\n\n## Implementation Plan\n\n- [ ] Add cache warmer\n"
	os.WriteFile(filepath.Join(docDir, "cache-warmup.md"), []byte(content), 0o644)

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	repoRoot := filepath.Join(tmp, "repo")

	return s, db, repoRoot
}

func TestScan_OpenSpecHierarchyLinksAndMarkdownOwnership(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := filepath.Join(tmp, "repo")
	changeDir := filepath.Join(repoRoot, "openspec", "changes", "add-sso")
	nestedChangeDir := filepath.Join(repoRoot, "services", "collector", "openspec", "changes", "add-flow")
	baseSpecDir := filepath.Join(repoRoot, "openspec", "specs", "auth")
	deltaSpecDir := filepath.Join(changeDir, "specs", "auth")
	nestedDeltaSpecDir := filepath.Join(nestedChangeDir, "specs", "flow")
	require.NoError(t, os.MkdirAll(changeDir, 0o755))
	require.NoError(t, os.MkdirAll(nestedChangeDir, 0o755))
	require.NoError(t, os.MkdirAll(baseSpecDir, 0o755))
	require.NoError(t, os.MkdirAll(deltaSpecDir, 0o755))
	require.NoError(t, os.MkdirAll(nestedDeltaSpecDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte("# Add SSO\n\n## Requirements\n\n- [ ] Users can sign in.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(changeDir, "design.md"), []byte("# Design\n\nUse OAuth2.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(changeDir, "tasks.md"), []byte("# Tasks\n\n- [ ] Wire provider.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(baseSpecDir, "spec.md"), []byte("# Auth Spec\n\n## Requirements\n\n- Password login works.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(deltaSpecDir, "spec.md"), []byte("# Auth Delta\n\n## MODIFIED Requirements\n\n- SSO login works.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(nestedChangeDir, "proposal.md"), []byte("# Add Flow\n\n## Why\n\nNested OpenSpec roots should index as OpenSpec.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(nestedChangeDir, "design.md"), []byte("# Flow Design\n\nUse collector batches.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(nestedChangeDir, "tasks.md"), []byte("# Flow Tasks\n\n- [ ] Wire collector.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(nestedDeltaSpecDir, "spec.md"), []byte("# Flow Delta\n\n## ADDED Requirements\n\n- Flow import works.\n"), 0o644))

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&openspec.Adapter{}, &markdown.Adapter{}})
	cfg := config.WithIntentCandidateDiscovery(nil, true)
	result, err := s.Run(context.Background(), repoRoot, cfg)
	require.NoError(t, err)
	require.Equal(t, 13, result.Found["openspec"],
		"openspec found = %d, want 13", result.Found["openspec"])
	require.NotNil(t, result.OpenSpec,
		"expected OpenSpec metrics")
	assert.InDelta(t, 1, result.OpenSpec.BundleRecall, 0,
		"OpenSpec bundle recall = %.3f, metrics=%#v", result.OpenSpec.BundleRecall, result.OpenSpec)
	assert.InDelta(t, 1, result.OpenSpec.ChildRoleRecall, 0,
		"OpenSpec child-role recall = %.3f, metrics=%#v", result.OpenSpec.ChildRoleRecall, result.OpenSpec)
	assert.InDelta(t, 4, result.OpenSpec.DuplicatePressure, 0,
		"OpenSpec duplicate pressure = %.3f, metrics=%#v", result.OpenSpec.DuplicatePressure, result.OpenSpec)
	require.Equal(t, 0, result.OpenSpec.MarkdownLeakage,
		"OpenSpec markdown leakage = %d, metrics=%#v", result.OpenSpec.MarkdownLeakage, result.OpenSpec)

	var markdownOpenSpec int
	err = db.QueryRow(`SELECT COUNT(*)
		FROM sources
		WHERE source_type = 'markdown' AND (path LIKE 'openspec/%' OR path LIKE '%/openspec/%')`).Scan(&markdownOpenSpec)
	require.NoError(t, err)
	require.Equal(t, 0, markdownOpenSpec,
		"OpenSpec markdown files should be owned by openspec adapter, got %d markdown sources", markdownOpenSpec)

	collectionID := mustArtifactIDBySourceIdentity(t, db, "openspec|openspec_collection")
	nestedCollectionID := mustArtifactIDBySourceIdentity(t, db, "services/collector/openspec|openspec_collection")
	bundleID := mustArtifactIDBySourceIdentity(t, db, "openspec/changes/add-sso|openspec_bundle")
	nestedBundleID := mustArtifactIDBySourceIdentity(t, db, "services/collector/openspec/changes/add-flow|openspec_bundle")
	proposalID := mustArtifactIDBySourceIdentity(t, db, "openspec/changes/add-sso/proposal.md|openspec")
	deltaID := mustArtifactIDBySourceIdentity(t, db, "openspec/changes/add-sso/specs/auth/spec.md|openspec")
	capabilityID := mustArtifactIDBySourceIdentity(t, db, "openspec/specs/auth/spec.md|openspec")

	assertLinkExists(t, db, collectionID, linkContains, "artifact:"+bundleID)
	assertLinkExists(t, db, nestedCollectionID, linkContains, "artifact:"+nestedBundleID)
	assertLinkExists(t, db, bundleID, linkContains, "artifact:"+proposalID)
	assertLinkExists(t, db, proposalID, linkContainedBy, "artifact:"+bundleID)
	assertLinkExists(t, db, deltaID, linkUpdates, "artifact:"+capabilityID)
}

func extractedJSONRoundTrip(m map[string]any) (map[string]any, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func anyStringHasPrefix(values []any, prefix string) bool {
	for _, value := range values {
		s, ok := value.(string)
		if ok && strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func mustArtifactIDBySourceIdentity(t *testing.T, db *store.DB, sourceIdentity string) string {
	t.Helper()
	var id string
	err := db.QueryRow(`SELECT artifact_id FROM sources WHERE source_identity = ?`, sourceIdentity).Scan(&id)
	require.NoError(t, err,
		"artifact source_identity %q: %v", sourceIdentity, err)

	return id
}

func assertLinkExists(t *testing.T, db *store.DB, artifactID, linkType, target string) {
	t.Helper()
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM links WHERE artifact_id = ? AND link_type = ? AND target = ?`, artifactID, linkType, target).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count,
		"link %s %s -> %s count = %d, want 1", artifactID, linkType, target, count)

}

func testdataSamplesRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "samples"))
	require.NoError(t, err)

	return root
}

// TestScan_CursorPlanSample_NoPathToolTagInDB verifies plan § success: after scan,
// artifact_tags must not gain path-derived tool slugs, and sources.format_profile is set.
func TestScan_CursorPlanSample_NoPathToolTagInDB(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)

	srcRoot := filepath.Join(testdataSamplesRoot(t), "cursor")
	planSrc := filepath.Join(srcRoot, ".cursor", "plans", "sample_cursor_plan.plan.md")
	data, err := os.ReadFile(planSrc)
	require.NoError(t, err)

	repoRoot := filepath.Join(tmp, "repo")
	dstDir := filepath.Join(repoRoot, ".cursor", "plans")
	os.MkdirAll(dstDir, 0o755)
	dstPath := filepath.Join(dstDir, "sample_cursor_plan.plan.md")
	{
		err := os.WriteFile(dstPath, data, 0o644)
		require.NoError(t, err)
	}

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	{
		_, err := s.Run(context.Background(), repoRoot, nil)
		require.NoError(t, err)
	}

	var artifactID string
	err = db.QueryRow("SELECT id FROM artifacts LIMIT 1").Scan(&artifactID)
	require.NoError(t, err)

	rows, err := db.Query("SELECT tag FROM artifact_tags WHERE artifact_id = ?", artifactID)
	require.NoError(t, err)

	defer rows.Close()
	for rows.Next() {
		var tag string
		{
			err := rows.Scan(&tag)
			require.NoError(t, err)
		}
		require.NotEqual(t, "cursor", tag,
			"path-derived tool slug must not appear in artifact_tags after scan, got tag %q", tag)

	}
	{
		err := rows.Err()
		require.NoError(t, err)
	}

	var profile string
	err = db.QueryRow("SELECT format_profile FROM sources WHERE artifact_id = ?", artifactID).Scan(&profile)
	require.NoError(t, err)
	require.Equal(t, format.ProfileCursorPlan, profile,
		"sources.format_profile: want %q, got %q", format.ProfileCursorPlan, profile)

}

func TestScan_SourcesBreakdown_MultipleMarkdownFormats(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoRoot := filepath.Join(tmp, "repo")
	plansDir := filepath.Join(repoRoot, "plans")
	cursorDir := filepath.Join(repoRoot, ".cursor", "plans")
	os.MkdirAll(plansDir, 0o755)
	os.MkdirAll(cursorDir, 0o755)
	os.WriteFile(filepath.Join(plansDir, "plain.md"), []byte("# Plain\n\nBody.\n"), 0o644)
	os.WriteFile(filepath.Join(cursorDir, "c.md"), []byte("# Cursorish\n\nBody.\n"), 0o644)

	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	res, err := s.Run(context.Background(), repoRoot, nil)
	require.NoError(t, err)
	require.Equal(t, 2, res.Found["markdown"],
		"Found markdown: want 2, got %d", res.Found["markdown"])

	var mdRow *SourceBreakdownRow
	for i := range res.SourcesBreakdown {
		if res.SourcesBreakdown[i].SourceType == "markdown" {
			mdRow = &res.SourcesBreakdown[i]
			break
		}
	}
	require.NotNil(t, mdRow,
		"no markdown breakdown row")
	require.Equal(t, 2, mdRow.Count,
		"markdown count: want 2, got %d", mdRow.Count)

	g := mdRow.Formats[format.ProfileGeneric]
	c := mdRow.Formats[format.ProfileCursorPlan]
	assert.Equal(t, 1, g)
	assert.Equal(t, 1, c)

}

func TestScan_FreshIndexBatchDeferredFTSEquivalence(t *testing.T) {
	repoRoot := setupFreshIndexSpeedRepo(t)
	canonicalDB, canonicalScanner, cfg := setupFreshIndexSpeedScanner(t)
	freshDB, freshScanner, _ := setupFreshIndexSpeedScanner(t)

	canonical, err := canonicalScanner.RunWithOptions(context.Background(), repoRoot, cfg, RunOptions{
		UseTransaction:       true,
		SkipAuthoredAtLookup: true,
		FileWorkerCount:      1,
	})
	require.NoError(t, err)
	fresh, err := freshScanner.RunWithOptions(context.Background(), repoRoot, cfg, RunOptions{
		UseTransaction:       true,
		SkipAuthoredAtLookup: true,
		FreshIndex:           true,
		FileWorkerCount:      4,
	})
	require.NoError(t, err)

	assertStringIntMapEqual(t, canonical.Found, fresh.Found)
	require.Equal(t, fresh.New, canonical.New,
		"New mismatch: canonical=%d fresh=%d", canonical.New, fresh.New)

	assertScanTableCountsEqual(t, canonicalDB, freshDB)
	assertStringSlicesEqualByIndex(t, artifactIdentitySnapshot(t, canonicalDB), artifactIdentitySnapshot(t, freshDB))

	hits, err := freshDB.FindArtifacts("duplicate replay", store.FilterParams{})
	require.NoError(t, err)
	require.NotEmpty(t, hits,
		"expected deferred FTS to be populated before scan completes")

}

func TestScan_FreshIndexParallelismIsDeterministic(t *testing.T) {
	repoRoot := setupFreshIndexSpeedRepo(t)
	oneWorkerDB, oneWorkerScanner, cfg := setupFreshIndexSpeedScanner(t)
	manyWorkersDB, manyWorkersScanner, _ := setupFreshIndexSpeedScanner(t)

	oneWorker, err := oneWorkerScanner.RunWithOptions(context.Background(), repoRoot, cfg, RunOptions{
		UseTransaction:       true,
		SkipAuthoredAtLookup: true,
		FreshIndex:           true,
		FileWorkerCount:      1,
	})
	require.NoError(t, err)
	manyWorkers, err := manyWorkersScanner.RunWithOptions(context.Background(), repoRoot, cfg, RunOptions{
		UseTransaction:       true,
		SkipAuthoredAtLookup: true,
		FreshIndex:           true,
		FileWorkerCount:      4,
	})
	require.NoError(t, err)

	assertStringIntMapEqual(t, oneWorker.Found, manyWorkers.Found)
	assertStringSlicesEqualByIndex(t, artifactIdentitySnapshot(t, oneWorkerDB), artifactIdentitySnapshot(t, manyWorkersDB))

}

func TestScan_WarmUpgradeBatchNewArtifactsMatchesFreshFullIndex(t *testing.T) {
	repoRoot := setupFreshIndexSpeedRepo(t)
	fullCfg := config.WithCodeCommentArtifacts(config.WithTestCaseArtifacts(config.WithDefaultIntentCandidateDiscovery(nil, true), true), true)
	defaultCfg := config.WithDefaultIntentCandidateDiscovery(nil, true)
	adapters := []adapters.Adapter{
		&markdown.Adapter{},
		&testcase.Adapter{},
		&codecomment.Adapter{},
	}

	freshDB, err := store.Open(filepath.Join(t.TempDir(), "fresh.db"))
	require.NoError(t, err)

	defer freshDB.Close()
	freshDB.SetMaxOpenConns(1)
	freshScanner := New(freshDB, idgen.NewFactory(), adapters)
	{
		_, err := freshScanner.RunWithOptions(context.Background(), repoRoot, fullCfg, RunOptions{
			UseTransaction:       true,
			SkipAuthoredAtLookup: true,
			FreshIndex:           true,
			FileWorkerCount:      2,
		})
		require.NoError(t, err)
	}

	warmDB, err := store.Open(filepath.Join(t.TempDir(), "warm.db"))
	require.NoError(t, err)

	defer warmDB.Close()
	warmDB.SetMaxOpenConns(1)
	warmScanner := New(warmDB, idgen.NewFactory(), adapters)
	{
		_, err := warmScanner.RunWithOptions(context.Background(), repoRoot, defaultCfg, RunOptions{
			UseTransaction:       true,
			SkipAuthoredAtLookup: true,
			FileWorkerCount:      2,
		})
		require.NoError(t, err)
	}

	warmFull, err := warmScanner.RunWithOptions(context.Background(), repoRoot, fullCfg, RunOptions{
		UseTransaction:       true,
		SkipAuthoredAtLookup: true,
		FileWorkerCount:      2,
		PhaseTiming:          true,
	})
	require.NoError(t, err)

	assertScanTableCountsEqual(t, freshDB, warmDB)
	assertStringSlicesEqualByIndex(t, artifactIdentitySnapshot(t, freshDB), artifactIdentitySnapshot(t, warmDB))
	{

		got, want := scanPhaseCount(warmFull, "adapter_parse_persist", "test_case", "batch_new"), 2
		require.Equal(t, want, got,
			"warm test_case batch_new = %d, want %d", got, want)
	}
	{

		got := scanPhaseCount(warmFull, "batch_new_fts", "", "artifacts_fts")
		require.NotEqual(t, 0, got,
			"expected batch_new_fts to flush rows, got %d", got)
	}

}

func TestScan_FreshIndexProgressIncludesGranularTimings(t *testing.T) {
	repoRoot := setupFreshIndexSpeedRepo(t)
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	db.SetMaxOpenConns(1)

	cfg := config.WithCodeCommentArtifacts(config.WithTestCaseArtifacts(config.WithDefaultIntentCandidateDiscovery(nil, true), true), true)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{
		&markdown.Adapter{},
		&testcase.Adapter{},
		&codecomment.Adapter{},
	})
	var events []ProgressEvent
	{
		_, err := scanner.RunWithOptions(context.Background(), repoRoot, cfg, RunOptions{
			UseTransaction:       true,
			SkipAuthoredAtLookup: true,
			FreshIndex:           true,
			FileWorkerCount:      2,
			Progress: func(event ProgressEvent) {
				events = append(events, event)
			},
		})
		require.NoError(t, err)
	}
	require.True(t, hasScanProgressEvent(events, "extract", "adapter_done"),
		"missing extract timing event: %#v", events)
	require.True(t, hasScanProgressEvent(events, "fresh_index_writer", "rows_flushed"),
		"missing writer flush timing event: %#v", events)
	require.True(t, hasScanProgressEvent(events, "fresh_index_fts", "done"),
		"missing FTS timing event: %#v", events)

}

func TestScan_AuthoredAtLookupCachedPerPath(t *testing.T) {
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "tests/test_checkout.py", strings.Join([]string{
		"def test_redirect_success():",
		"    assert response.status_code == 200",
		"",
		"def test_redirect_rejects_bad_state():",
		"    assert response.status_code == 400",
		"",
		"def test_redirect_preserves_next_url():",
		"    assert next_url == '/docs'",
		"",
	}, "\n"))

	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	db.SetMaxOpenConns(1)

	const authoredAt = "2020-01-02T03:04:05Z"
	lookupsByPath := map[string]int{}
	previous := fileFirstCommitDate
	fileFirstCommitDate = func(ctx context.Context, repoRoot, relPath string) string {
		lookupsByPath[relPath]++
		return authoredAt
	}
	t.Cleanup(func() { fileFirstCommitDate = previous })

	cfg := config.WithTestCaseArtifacts(config.WithDefaultIntentCandidateDiscovery(nil, true), true)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&testcase.Adapter{}})
	result, err := scanner.RunWithOptions(context.Background(), repoRoot, cfg, RunOptions{
		UseTransaction:  true,
		FileWorkerCount: 2,
		PhaseTiming:     true,
	})
	require.NoError(t, err)
	{

		got, want := result.Found["test_case"], 3
		require.Equal(t, want, got,
			"test_case found = %d, want %d", got, want)
	}
	{

		got, want := lookupsByPath["tests/test_checkout.py"], 1
		require.Equal(t, want, got,
			"authored_at lookups for tests/test_checkout.py = %d, want %d; all lookups %#v", got, want, lookupsByPath)
	}
	{

		got, want := scanPhaseCount(result, "authored_at_prefetch", "test_case", "paths"), 1
		require.Equal(t, want, got,
			"authored_at_prefetch paths = %d, want %d", got, want)
	}

	rows, err := db.Query(`SELECT authored_at FROM artifacts ORDER BY title`)
	require.NoError(t, err)

	defer rows.Close()
	var gotAuthoredAt []string
	for rows.Next() {
		var value string
		{
			err := rows.Scan(&value)
			require.NoError(t, err)
		}

		gotAuthoredAt = append(gotAuthoredAt, value)
	}
	{
		err := rows.Err()
		require.NoError(t, err)
	}
	require.Len(t, gotAuthoredAt, 3)
	assert.Equal(t, authoredAt, gotAuthoredAt[0])
	assert.Equal(t, authoredAt, gotAuthoredAt[1])
	assert.Equal(t, authoredAt, gotAuthoredAt[2])

}

func TestScan_AuthoredAtPrefetchUsesBulkWithExactFallback(t *testing.T) {
	previousSingle := fileFirstCommitDate
	previousBulk := fileFirstCommitDates
	const bulkDate = "2020-01-02T03:04:05Z"
	const fallbackDate = "2021-02-03T04:05:06Z"
	bulkCalls := 0
	var lookupMu sync.Mutex
	singleLookups := map[string]int{}
	fileFirstCommitDates = func(ctx context.Context, repoRoot string, rels []string) map[string]string {
		bulkCalls++
		require.Len(t, rels, minBulkAuthoredAtPaths,
			"bulk rel count = %d, want %d", len(rels), minBulkAuthoredAtPaths)

		return map[string]string{"tests/test_00.py": bulkDate}
	}
	fileFirstCommitDate = func(ctx context.Context, repoRoot, relPath string) string {
		lookupMu.Lock()
		singleLookups[relPath]++
		lookupMu.Unlock()
		return fallbackDate
	}
	t.Cleanup(func() {
		fileFirstCommitDate = previousSingle
		fileFirstCommitDates = previousBulk
	})

	candidates := make([]adapters.Candidate, 0, minBulkAuthoredAtPaths)
	for i := 0; i < minBulkAuthoredAtPaths; i++ {
		candidates = append(candidates, adapters.Candidate{RelPath: fmt.Sprintf("tests/test_%02d.py", i)})
	}
	state := &scanRunState{}
	{
		got := state.prefetchAuthoredAt(context.Background(), t.TempDir(), candidates, "2026-07-10T00:00:00Z", RunOptions{FileWorkerCount: 2})
		require.Equal(t, minBulkAuthoredAtPaths, got,
			"prefetch paths = %d, want %d", got, minBulkAuthoredAtPaths)
	}
	require.Equal(t, 1, bulkCalls,
		"bulk calls = %d, want 1", bulkCalls)

	lookupMu.Lock()
	bulkHitFallbacks := singleLookups["tests/test_00.py"]
	fallbackLookupCount := len(singleLookups)
	singleLookupSnapshot := map[string]int{}
	for path, count := range singleLookups {
		singleLookupSnapshot[path] = count
	}
	lookupMu.Unlock()
	require.Equal(t, 0, bulkHitFallbacks,
		"bulk hit should not fall back to exact lookup: %#v", singleLookupSnapshot)
	{

		got, want := fallbackLookupCount, minBulkAuthoredAtPaths-1
		require.Equal(t, want, got,
			"fallback lookup count = %d, want %d: %#v", got, want, singleLookupSnapshot)
	}

	gotBulk := state.resolveAuthoredAt("", adapters.Artifact{}, []adapters.Source{{Path: "tests/test_00.py"}}, "now")
	gotFallback := state.resolveAuthoredAt("", adapters.Artifact{}, []adapters.Source{{Path: "tests/test_01.py"}}, "now")
	assert.Equal(t, bulkDate, gotBulk)
	assert.Equal(t, fallbackDate, gotFallback)

	lookupMu.Lock()
	fallbackLookupCount = len(singleLookups)
	lookupMu.Unlock()
	{
		got, want := fallbackLookupCount, minBulkAuthoredAtPaths-1
		require.Equal(t, want, got,
			"resolveAuthoredAt caused extra fallback lookup: got %d want %d", got, want)
	}

}

func scanPhaseCount(result *Result, name, adapter, key string) int {
	if result == nil || result.PhaseTiming == nil {
		return 0
	}
	for _, phase := range result.PhaseTiming.Phases {
		if phase.Name == name && phase.Adapter == adapter {
			return phase.Counts[key]
		}
	}
	return 0
}

func TestCollectFileInventoryExplainsSkippedHeavyAndIgnoredDirs(t *testing.T) {
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, "docs/plans/launch.md", "# Launch\n")
	writeScanTestFile(t, repoRoot, "node_modules/pkg/plan.md", "# Noise\n")
	writeScanTestFile(t, repoRoot, ".git/objects/noise", "noise\n")
	writeScanTestFile(t, repoRoot, "ignored/plan.md", "# Ignored\n")
	writeScanTestFile(t, repoRoot, ".gitignore", "ignored/\n")

	matcher, err := ignore.NewMatcher(repoRoot)
	require.NoError(t, err)

	result, err := collectFileInventory(ignore.WithContext(context.Background(), matcher), repoRoot)
	require.NoError(t, err)

	diag := result.diagnostics()
	require.NotNil(t, diag,
		"expected traversal diagnostics")
	require.GreaterOrEqual(t, diag.SkippedByReason["generated_vendor_or_build"], 2,
		"expected generated/vendor/build skips for .git and node_modules, got %#v", diag)
	require.Equal(t, 1, diag.SkippedByReason["ignore_rules"],
		"expected one ignore_rules skip, got %#v", diag)
	require.True(t, traversalSkipContains(diag.TopSkippedDirs, "node_modules", "generated_vendor_or_build"),
		"expected node_modules skip example, got %#v", diag.TopSkippedDirs)

	for _, file := range result.files {
		assert.False(t, strings.HasPrefix(file.relPath, "node_modules/"))
		assert.False(t, strings.HasPrefix(file.relPath, ".git/"))
		assert.False(t, strings.HasPrefix(file.relPath, "ignored/"))

	}
}

func TestCollectFileInventoryIncludesTrackedFileMatchingGitIgnore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, ".gitignore", "core.*\n")
	writeScanTestFile(t, repoRoot, "lib/core.sh", "#!/bin/sh\necho core\n")
	runGitCommand(t, repoRoot, "init")
	runGitCommand(t, repoRoot, "add", ".gitignore")
	runGitCommand(t, repoRoot, "add", "--force", "lib/core.sh")
	matcher, err := ignore.NewMatcher(repoRoot)
	require.NoError(t, err)

	result, err := collectFileInventory(ignore.WithContext(context.Background(), matcher), repoRoot)
	require.NoError(t, err)

	require.Len(t, result.files, 2)
	assert.Equal(t, ".gitignore", result.files[0].relPath)
	assert.Equal(t, "lib/core.sh", result.files[1].relPath)
}

func TestCollectFileInventoryExcludesTrackedFileMatchingAIIgnore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	repoRoot := t.TempDir()
	writeScanTestFile(t, repoRoot, ".aiignore", "core.*\n")
	writeScanTestFile(t, repoRoot, "lib/core.sh", "#!/bin/sh\necho core\n")
	runGitCommand(t, repoRoot, "init")
	runGitCommand(t, repoRoot, "add", ".aiignore")
	runGitCommand(t, repoRoot, "add", "--force", "lib/core.sh")
	matcher, err := ignore.NewMatcher(repoRoot)
	require.NoError(t, err)

	result, err := collectFileInventory(ignore.WithContext(context.Background(), matcher), repoRoot)
	require.NoError(t, err)

	require.Len(t, result.files, 1)
	assert.Equal(t, ".aiignore", result.files[0].relPath)
}

func TestScan_FreshIndexAppendSeedsExistingShortIDs(t *testing.T) {
	root := t.TempDir()
	repoOne := filepath.Join(root, "repo-one")
	repoTwo := filepath.Join(root, "repo-two")
	require.NoError(t, os.MkdirAll(filepath.Join(repoOne, "docs", "plans"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoTwo, "docs", "plans"), 0o755))
	body := "# Billing Plan\n\nKeep replay handling deterministic.\n"
	require.NoError(t, os.WriteFile(filepath.Join(repoOne, "docs", "plans", "billing.md"), []byte(body), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoTwo, "docs", "plans", "billing.md"), []byte(body), 0o644))

	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	db.SetMaxOpenConns(1)
	base := idgen.ShortID("docs/plans/billing.md|markdown")
	now := "2026-08-11T00:00:00Z"
	repoOneID, err := db.ResolveRepo(store.RepositoryIdentity{RootPath: repoOne}, "repo_one", now)
	require.NoError(t, err)
	require.NoError(t, db.InsertArtifactDirect("artifact_one", repoOneID, "plan", "", "Billing Plan", "draft", "revision_one", now, now))
	require.NoError(t, db.UpdateArtifactShortID("artifact_one", base))
	require.NoError(t, db.InsertSourceDirect("source_one", "artifact_one", repoOneID, "markdown", "docs/plans/billing.md", "docs/plans/billing.md|markdown", "generic", "", now))

	cfg := config.WithDefaultIntentCandidateDiscovery(nil, true)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})
	_, err = scanner.RunWithOptions(context.Background(), repoTwo, cfg, RunOptions{
		UseTransaction:       true,
		SkipAuthoredAtLookup: true,
		FreshIndex:           true,
	})
	require.NoError(t, err)

	rows, err := db.Query(`SELECT short_id FROM artifacts ORDER BY created_at, id`)
	require.NoError(t, err)

	defer rows.Close()
	var shortIDs []string
	for rows.Next() {
		var shortID string
		{
			err := rows.Scan(&shortID)
			require.NoError(t, err)
		}

		shortIDs = append(shortIDs, shortID)
	}
	{
		err := rows.Err()
		require.NoError(t, err)
	}
	require.Len(t, shortIDs, 2,
		"short ID rows = %#v, want 2 rows", shortIDs)

	assert.Equal(t, base, shortIDs[0])
	assert.Equal(t, base+"1", shortIDs[1])

}

func hasScanProgressEvent(events []ProgressEvent, phase, event string) bool {
	for _, got := range events {
		if got.Phase == phase && got.Event == event {
			return true
		}
	}
	return false
}

func assertStringIntMapEqual(t *testing.T, want, got map[string]int) {
	t.Helper()

	require.Len(t, got, len(want))
	for key, wantValue := range want {
		assert.Equal(t, wantValue, got[key], "map key %q", key)
	}
}

func assertStringSlicesEqualByIndex(t *testing.T, want, got []string) {
	t.Helper()

	require.Len(t, got, len(want))
	for index := range want {
		assert.Equal(t, want[index], got[index], "slice index %d", index)
	}
}

func assertScanTableCountsEqual(t *testing.T, wantDB, gotDB *store.DB) {
	t.Helper()

	assert.Equal(t, tableCount(t, wantDB, "artifacts"), tableCount(t, gotDB, "artifacts"))
	assert.Equal(t, tableCount(t, wantDB, "artifact_revisions"), tableCount(t, gotDB, "artifact_revisions"))
	assert.Equal(t, tableCount(t, wantDB, "sources"), tableCount(t, gotDB, "sources"))
	assert.Equal(t, tableCount(t, wantDB, "artifact_todos"), tableCount(t, gotDB, "artifact_todos"))
	assert.Equal(t, tableCount(t, wantDB, "artifact_criteria"), tableCount(t, gotDB, "artifact_criteria"))
	assert.Equal(t, tableCount(t, wantDB, "artifact_tags"), tableCount(t, gotDB, "artifact_tags"))
	assert.Equal(t, tableCount(t, wantDB, "artifact_sections"), tableCount(t, gotDB, "artifact_sections"))
	assert.Equal(t, tableCount(t, wantDB, "artifact_sections_fts"), tableCount(t, gotDB, "artifact_sections_fts"))
	assert.Equal(t, tableCount(t, wantDB, "artifacts_fts"), tableCount(t, gotDB, "artifacts_fts"))
	assert.Equal(t, tableCount(t, wantDB, "concepts"), tableCount(t, gotDB, "concepts"))
	assert.Equal(t, tableCount(t, wantDB, "concept_mentions"), tableCount(t, gotDB, "concept_mentions"))
	assert.Equal(t, tableCount(t, wantDB, "artifact_edges"), tableCount(t, gotDB, "artifact_edges"))
}

func traversalSkipContains(paths []TraversalSkippedPath, path, reason string) bool {
	for _, got := range paths {
		if got.Path == path && got.Reason == reason {
			return true
		}
	}
	return false
}

func setupFreshIndexSpeedRepo(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	{
		err := os.MkdirAll(filepath.Join(root, "docs", "plans"), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.MkdirAll(filepath.Join(root, "src"), 0o755)
		require.NoError(t, err)
	}

	plan := "# Billing Retry Plan\n\n" +
		"## Replay Boundary\n\n" +
		"- [ ] Preserve duplicate replay protection\n\n" +
		"## Acceptance Criteria\n\n" +
		"- [ ] Duplicate webhook replay is rejected\n"
	{
		err := os.WriteFile(filepath.Join(root, "docs", "plans", "billing.md"), []byte(plan), 0o644)
		require.NoError(t, err)
	}

	source := "// TODO because duplicate webhook replay must stay idempotent for legacy callers.\n" +
		"export function retryBilling() { return true }\n"
	{
		err := os.WriteFile(filepath.Join(root, "src", "billing.ts"), []byte(source), 0o644)
		require.NoError(t, err)
	}

	testSource := "describe(\"billing retries\", () => {\n" +
		"  it(\"rejects duplicate replay\", () => {\n" +
		"    expect(retryBilling()).toBe(true)\n" +
		"  })\n" +
		"  it(\"keeps legacy compatibility\", () => {\n" +
		"    expect(retryBilling()).toBe(true)\n" +
		"  })\n" +
		"})\n"
	{
		err := os.WriteFile(filepath.Join(root, "src", "billing.test.ts"), []byte(testSource), 0o644)
		require.NoError(t, err)
	}

	return root
}

func setupFreshIndexSpeedScanner(t *testing.T) (*store.DB, *Scanner, *config.RepoConfig) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	cfg := config.WithCodeCommentArtifacts(config.WithTestCaseArtifacts(config.WithDefaultIntentCandidateDiscovery(nil, true), true), true)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{
		&markdown.Adapter{},
		&testcase.Adapter{},
		&codecomment.Adapter{},
	})

	return db, scanner, cfg
}

func tableCount(t *testing.T, db *store.DB, table string) int {
	t.Helper()
	var count int
	{
		err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		require.NoError(t, err,
			"count %s: %v", table, err)
	}

	return count
}

func resultRepoID(t *testing.T, db *store.DB) string {
	t.Helper()
	var repoID string
	{
		err := db.QueryRow("SELECT id FROM repos LIMIT 1").Scan(&repoID)
		require.NoError(t, err)
	}

	return repoID
}

func runGitCommand(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err,
		"git %v failed: %v\n%s", args, err, out)

}

func artifactIdentitySnapshot(t *testing.T, db *store.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT s.source_identity, COALESCE(a.short_id,''), a.kind, COALESCE(a.subtype,''), a.title
FROM sources s
JOIN artifacts a ON a.id = s.artifact_id
ORDER BY s.source_identity`)
	require.NoError(t, err)

	defer rows.Close()
	var out []string
	for rows.Next() {
		var identity, shortID, kind, subtype, title string
		{
			err := rows.Scan(&identity, &shortID, &kind, &subtype, &title)
			require.NoError(t, err)
		}

		out = append(out, strings.Join([]string{identity, shortID, kind, subtype, title}, "\x00"))
	}
	{
		err := rows.Err()
		require.NoError(t, err)
	}

	return out
}
