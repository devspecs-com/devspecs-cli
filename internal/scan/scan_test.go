package scan

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/codecomment"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/openspec"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/testcase"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func setupTestRepo(t *testing.T) (string, *store.DB) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	plansDir := filepath.Join(tmp, "repo", "plans")
	os.MkdirAll(plansDir, 0o755)
	os.WriteFile(filepath.Join(plansDir, "auth.md"), []byte("# Auth Plan\n\n- [ ] Add login\n- [x] Design schema\n"), 0o644)

	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return filepath.Join(tmp, "repo"), db
}

func TestScan_DetectsMarkdownPlans(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	result, err := s.Run(context.Background(), repoRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Found["markdown"] != 1 {
		t.Errorf("expected 1 markdown found, got %d", result.Found["markdown"])
	}
	if result.New != 1 {
		t.Errorf("expected 1 new, got %d", result.New)
	}
}

func TestScan_StableIDs(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	s.Run(context.Background(), repoRoot, nil)

	// Get artifact ID
	var id1 string
	db.QueryRow("SELECT id FROM artifacts LIMIT 1").Scan(&id1)

	// Scan again
	s.Run(context.Background(), repoRoot, nil)
	var id2 string
	db.QueryRow("SELECT id FROM artifacts LIMIT 1").Scan(&id2)

	if id1 != id2 {
		t.Errorf("ID not stable across rescans: %q vs %q", id1, id2)
	}
}

func TestScan_NoDuplicateOnUnchanged(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	s.Run(context.Background(), repoRoot, nil)
	result, _ := s.Run(context.Background(), repoRoot, nil)

	if result.Unchanged != 1 {
		t.Errorf("expected 1 unchanged, got %d", result.Unchanged)
	}
	if result.New != 0 {
		t.Errorf("expected 0 new, got %d", result.New)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM artifacts").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 artifact, got %d", count)
	}
}

func TestScan_RebuildsEvidenceGraphDiagnostics(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	if err := os.WriteFile(filepath.Join(repoRoot, "plans", "auth-tests.md"), []byte("# Auth Token Tests\n\nSee plans/auth.md for the auth token rollout.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})

	result, err := s.Run(context.Background(), repoRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.EvidenceGraph == nil {
		t.Fatal("expected evidence graph diagnostics")
	}
	if result.EvidenceGraph.ConceptsIndexed == 0 {
		t.Fatal("expected indexed concepts")
	}
	if result.EvidenceGraph.MentionsIndexed == 0 {
		t.Fatal("expected indexed concept mentions")
	}
	if result.EvidenceGraph.EdgesByType[edgeTypeMentionsSameConcept] == 0 {
		t.Fatalf("expected shared-concept edge diagnostics: %#v", result.EvidenceGraph)
	}
	if result.EvidenceGraph.EdgesByType[edgeTypeExplicitReference] == 0 {
		t.Fatalf("expected explicit path-reference edge diagnostics: %#v", result.EvidenceGraph)
	}
	firstConcepts := tableCount(t, db, "concepts")
	firstMentions := tableCount(t, db, "concept_mentions")
	firstEdges := tableCount(t, db, "artifact_edges")
	if _, err := s.Run(context.Background(), repoRoot, nil); err != nil {
		t.Fatal(err)
	}
	if got := tableCount(t, db, "concepts"); got != firstConcepts {
		t.Fatalf("concept count changed after rescan: got %d want %d", got, firstConcepts)
	}
	if got := tableCount(t, db, "concept_mentions"); got != firstMentions {
		t.Fatalf("mention count changed after rescan: got %d want %d", got, firstMentions)
	}
	if got := tableCount(t, db, "artifact_edges"); got != firstEdges {
		t.Fatalf("edge count changed after rescan: got %d want %d", got, firstEdges)
	}
}

func TestScan_ExperimentalGitEvidenceStoresFactsAndEdges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	repoRoot, db := setupTestRepo(t)
	if err := os.WriteFile(filepath.Join(repoRoot, "plans", "auth-tests.md"), []byte("# Auth Tests\n\n- [ ] Verify auth flow\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, repoRoot, "init")
	runGitCommand(t, repoRoot, "checkout", "-b", "main")
	runGitCommand(t, repoRoot, "config", "user.email", "test@example.com")
	runGitCommand(t, repoRoot, "config", "user.name", "Test User")
	runGitCommand(t, repoRoot, "add", ".")
	runGitCommand(t, repoRoot, "commit", "-m", "add auth docs")

	s := New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})
	result, err := s.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{
		UseTransaction:     true,
		IncludeGitEvidence: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GitEvidence == nil {
		t.Fatal("expected git evidence diagnostics")
	}
	if result.GitEvidence.HistoryShape != "single_commit" {
		t.Fatalf("expected single_commit history shape, got %#v", result.GitEvidence)
	}
	if result.GitEvidence.CommitsStored != 1 {
		t.Fatalf("expected one stored git commit, got %#v", result.GitEvidence)
	}
	if result.GitEvidence.EdgesByType[edgeTypeCoChangedWith] == 0 {
		t.Fatalf("expected co-change edge diagnostics, got %#v", result.GitEvidence)
	}
	counts, err := db.CountGitFacts(resultRepoID(t, db))
	if err != nil {
		t.Fatal(err)
	}
	if counts.Commits != 1 || counts.Files != 2 {
		t.Fatalf("unexpected git fact counts: %#v", counts)
	}
	if _, err := s.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{
		UseTransaction:     true,
		IncludeGitEvidence: true,
	}); err != nil {
		t.Fatal(err)
	}
	nextCounts, err := db.CountGitFacts(resultRepoID(t, db))
	if err != nil {
		t.Fatal(err)
	}
	if nextCounts != counts {
		t.Fatalf("git facts should be idempotent: got %#v want %#v", nextCounts, counts)
	}
	defaultResult, err := s.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{UseTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	if defaultResult.GitEvidence != nil {
		t.Fatalf("default scan should not emit git diagnostics: %#v", defaultResult.GitEvidence)
	}
	clearedCounts, err := db.CountGitFacts(resultRepoID(t, db))
	if err != nil {
		t.Fatal(err)
	}
	if clearedCounts.Commits != 0 || clearedCounts.Files != 0 {
		t.Fatalf("default scan should clear opt-in git facts, got %#v", clearedCounts)
	}
}

func TestScan_ExperimentalGitEvidenceNonGitDirectory(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	s := New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})
	result, err := s.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{
		UseTransaction:     true,
		IncludeGitEvidence: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GitEvidence == nil {
		t.Fatal("expected git evidence diagnostics")
	}
	if result.GitEvidence.HistoryShape != "non_git" && result.GitEvidence.HistoryShape != "unavailable" {
		t.Fatalf("expected non_git/unavailable history shape, got %#v", result.GitEvidence)
	}
	if result.GitEvidence.CommitsStored != 0 || result.GitEvidence.EdgesIndexed != 0 {
		t.Fatalf("expected no stored git facts or edges, got %#v", result.GitEvidence)
	}
}

func TestScan_NewRevisionOnContentChange(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	s.Run(context.Background(), repoRoot, nil)

	// Modify the file
	planPath := filepath.Join(repoRoot, "plans", "auth.md")
	os.WriteFile(planPath, []byte("# Auth Plan v2\n\n- [ ] New task\n"), 0o644)

	result, _ := s.Run(context.Background(), repoRoot, nil)
	if result.Updated != 1 {
		t.Errorf("expected 1 updated, got %d", result.Updated)
	}

	var revCount int
	db.QueryRow("SELECT COUNT(*) FROM artifact_revisions").Scan(&revCount)
	if revCount != 2 {
		t.Errorf("expected 2 revisions, got %d", revCount)
	}
}

func TestScan_RefreshesTodosOnRevision(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	s.Run(context.Background(), repoRoot, nil)

	var todoCount int
	db.QueryRow("SELECT COUNT(*) FROM artifact_todos").Scan(&todoCount)
	if todoCount != 2 {
		t.Errorf("expected 2 todos after first scan, got %d", todoCount)
	}

	// Change content, different todos
	planPath := filepath.Join(repoRoot, "plans", "auth.md")
	os.WriteFile(planPath, []byte("# Auth Plan\n\n- [ ] Only one todo now\n"), 0o644)

	s.Run(context.Background(), repoRoot, nil)

	db.QueryRow("SELECT COUNT(*) FROM artifact_todos").Scan(&todoCount)
	if todoCount != 1 {
		t.Errorf("expected 1 todo after revision, got %d", todoCount)
	}
}

func TestScan_PersistsMarkdownSectionsAndTodoLinks(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})

	if _, err := s.Run(context.Background(), repoRoot, nil); err != nil {
		t.Fatal(err)
	}

	var sectionID, heading, sourcePath string
	if err := db.QueryRow("SELECT id, heading_path, source_path FROM artifact_sections LIMIT 1").Scan(&sectionID, &heading, &sourcePath); err != nil {
		t.Fatal(err)
	}
	if sectionID == "" {
		t.Fatal("expected section id")
	}
	if heading != "Auth Plan" {
		t.Fatalf("expected Auth Plan heading, got %q", heading)
	}
	if sourcePath != "plans/auth.md" {
		t.Fatalf("expected relative source path, got %q", sourcePath)
	}

	var todoSectionID string
	if err := db.QueryRow("SELECT section_id FROM artifact_todos WHERE text = 'Add login'").Scan(&todoSectionID); err != nil {
		t.Fatal(err)
	}
	if todoSectionID != sectionID {
		t.Fatalf("expected todo to link to section %q, got %q", sectionID, todoSectionID)
	}

	hits, err := db.FindArtifactSections("design schema", store.FilterParams{RepoRoot: repoRoot}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != sectionID {
		t.Fatalf("expected section FTS hit %q, got %#v", sectionID, hits)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	if _, err := s.Run(context.Background(), filepath.Join(tmp, "repo"), nil); err != nil {
		t.Fatal(err)
	}
	var nTodos, nCrit int
	db.QueryRow("SELECT COUNT(*) FROM artifact_todos").Scan(&nTodos)
	db.QueryRow("SELECT COUNT(*) FROM artifact_criteria").Scan(&nCrit)
	if nTodos != 1 || nCrit != 1 {
		t.Fatalf("want 1 todo and 1 criterion, got todos=%d criteria=%d", nTodos, nCrit)
	}
}

func TestScan_FrontmatterOverridesHeuristics(t *testing.T) {
	tmp := t.TempDir()
	plansDir := filepath.Join(tmp, "repo", "plans")
	os.MkdirAll(plansDir, 0o755)
	os.WriteFile(filepath.Join(plansDir, "test.md"), []byte("---\ntitle: Override Title\nkind: spec\nstatus: approved\n---\n# Ignored\n"), 0o644)

	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := idgen.NewFactory()
	adpts := []adapters.Adapter{&markdown.Adapter{}}
	s := New(db, ids, adpts)

	cfg := &config.RepoConfig{Sources: []config.SourceConfig{{Type: "markdown", Paths: []string{"plans"}}}}
	s.Run(context.Background(), filepath.Join(tmp, "repo"), cfg)

	var title, kind, status string
	db.QueryRow("SELECT title, kind, status FROM artifacts LIMIT 1").Scan(&title, &kind, &status)
	if title != "Override Title" {
		t.Errorf("expected 'Override Title', got %q", title)
	}
	if kind != "spec" {
		t.Errorf("expected 'spec', got %q", kind)
	}
	if status != "approved" {
		t.Errorf("expected 'approved', got %q", status)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	if _, err := s.Run(context.Background(), filepath.Join(tmp, "repo"), nil); err != nil {
		t.Fatal(err)
	}
	var ex string
	err = db.QueryRow(`SELECT COALESCE(rv.extracted_json, '') FROM artifact_revisions rv JOIN artifacts a ON a.current_revision_id = rv.id LIMIT 1`).Scan(&ex)
	if err != nil {
		t.Fatal(err)
	}
	if ex == "" {
		t.Fatal("expected non-empty extracted_json")
	}
	if !strings.Contains(ex, "frontmatter") {
		t.Fatalf("expected frontmatter in extracted json: %s", ex)
	}
	if !strings.Contains(ex, "classifier") {
		t.Fatalf("expected classifier metadata in extracted json: %s", ex)
	}
	// Apart from scan-level classifier metadata, preserve the markdown adapter extraction.
	md := &markdown.Adapter{}
	repoRoot := filepath.Join(tmp, "repo")
	abs := filepath.Join(repoRoot, "plans", "fm.md")
	wantArt, _, _, err := md.Parse(context.Background(), adapters.Candidate{
		PrimaryPath: abs,
		RelPath:     "plans/fm.md",
		AdapterName: "markdown",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(ex), &got); err != nil {
		t.Fatalf("stored json: %v", err)
	}
	wantCanon, err := extractedJSONRoundTrip(wantArt.Extracted)
	if err != nil {
		t.Fatal(err)
	}
	delete(got, "classifier")
	if !reflect.DeepEqual(got, wantCanon) {
		t.Fatalf("extracted_json != parsed Extracted (JSON semantics)\ngot:  %#v\nwant: %#v", got, wantCanon)
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
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	if _, err := s.Run(context.Background(), filepath.Join(tmp, "repo"), nil); err != nil {
		t.Fatal(err)
	}

	var ex string
	err = db.QueryRow(`SELECT COALESCE(rv.extracted_json, '') FROM artifact_revisions rv JOIN artifacts a ON a.current_revision_id = rv.id LIMIT 1`).Scan(&ex)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(ex), &got); err != nil {
		t.Fatal(err)
	}
	classifier, ok := got["classifier"].(map[string]any)
	if !ok {
		t.Fatalf("missing classifier metadata: %#v", got)
	}
	if classifier["evaluator"] != "declarative_document_models_v0" {
		t.Fatalf("evaluator = %#v", classifier["evaluator"])
	}
	winner := classifier["winner"].(map[string]any)
	if winner["classifier"] != "plan" {
		t.Fatalf("winner classifier = %#v", winner["classifier"])
	}
	if winner["family"] != "plan.implementation_plan" {
		t.Fatalf("winner family = %#v", winner["family"])
	}
}

func TestScan_SubtypeFirstNonIntentClassifierQuarantinesAgentInstructions(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoRoot := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Project Instructions\n\n## Rules\n\nAlways run tests and follow repo conventions.\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "CLAUDE.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	cfg := config.WithIntentCandidateDiscovery(nil, true)
	if _, err := s.Run(context.Background(), repoRoot, cfg); err != nil {
		t.Fatal(err)
	}

	var kind, subtype, ex string
	err = db.QueryRow(`SELECT a.kind, a.subtype, COALESCE(rv.extracted_json, '')
		FROM artifacts a
		JOIN sources s ON s.artifact_id = a.id
		JOIN artifact_revisions rv ON rv.id = a.current_revision_id
		WHERE s.path = 'CLAUDE.md'`).Scan(&kind, &subtype, &ex)
	if err != nil {
		t.Fatal(err)
	}
	if kind != config.KindMarkdownArtifact || subtype != config.SubtypeAgentInstruction {
		t.Fatalf("kind/subtype = %q/%q, want %q/%q", kind, subtype, config.KindMarkdownArtifact, config.SubtypeAgentInstruction)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(ex), &got); err != nil {
		t.Fatal(err)
	}
	if got["mode"] != "protocol" {
		t.Fatalf("mode = %#v", got["mode"])
	}
	classifier := got["classifier"].(map[string]any)
	winner := classifier["winner"].(map[string]any)
	if winner["classifier"] != "protocol" || winner["mode"] != "protocol" {
		t.Fatalf("winner = %#v", winner)
	}
}

func TestScan_ExperimentalIntentDiscoveryIndexesCompoundPlanningDir(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)
	docDir := filepath.Join(tmp, "repo", "docs", "exec-plans", "active")
	os.MkdirAll(docDir, 0o755)
	content := "# Cache Warmup\n\n## Goals\n\n## Implementation Plan\n\n- [ ] Add cache warmer\n"
	os.WriteFile(filepath.Join(docDir, "cache-warmup.md"), []byte(content), 0o644)

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	repoRoot := filepath.Join(tmp, "repo")
	if result, err := s.Run(context.Background(), repoRoot, nil); err != nil {
		t.Fatal(err)
	} else if result.Found["markdown"] != 0 {
		t.Fatalf("baseline scan found %d markdown artifacts, want 0", result.Found["markdown"])
	}

	cfg := config.WithIntentCandidateDiscovery(nil, true)
	result, err := s.Run(context.Background(), repoRoot, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Found["markdown"] != 1 {
		t.Fatalf("experimental scan found %d markdown artifacts, want 1", result.Found["markdown"])
	}

	var sourcePath, ex string
	err = db.QueryRow(`SELECT s.path, COALESCE(rv.extracted_json, '')
		FROM sources s
		JOIN artifacts a ON a.id = s.artifact_id
		JOIN artifact_revisions rv ON rv.id = a.current_revision_id
		WHERE s.source_type = 'markdown'
		LIMIT 1`).Scan(&sourcePath, &ex)
	if err != nil {
		t.Fatal(err)
	}
	if sourcePath != "docs/exec-plans/active/cache-warmup.md" {
		t.Fatalf("source path = %q", sourcePath)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(ex), &got); err != nil {
		t.Fatal(err)
	}
	classifier := got["classifier"].(map[string]any)
	reasons, ok := classifier["discovery_reasons"].([]any)
	if !ok || len(reasons) == 0 {
		t.Fatalf("missing discovery reasons in classifier metadata: %#v", classifier)
	}
	if !anyStringHasPrefix(reasons, "intent_path_token:plan") {
		t.Fatalf("expected plan discovery reason, got %#v", reasons)
	}
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
	for _, dir := range []string{changeDir, nestedChangeDir, baseSpecDir, deltaSpecDir, nestedDeltaSpecDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte("# Add SSO\n\n## Requirements\n\n- [ ] Users can sign in.\n"), 0o644)
	os.WriteFile(filepath.Join(changeDir, "design.md"), []byte("# Design\n\nUse OAuth2.\n"), 0o644)
	os.WriteFile(filepath.Join(changeDir, "tasks.md"), []byte("# Tasks\n\n- [ ] Wire provider.\n"), 0o644)
	os.WriteFile(filepath.Join(baseSpecDir, "spec.md"), []byte("# Auth Spec\n\n## Requirements\n\n- Password login works.\n"), 0o644)
	os.WriteFile(filepath.Join(deltaSpecDir, "spec.md"), []byte("# Auth Delta\n\n## MODIFIED Requirements\n\n- SSO login works.\n"), 0o644)
	os.WriteFile(filepath.Join(nestedChangeDir, "proposal.md"), []byte("# Add Flow\n\n## Why\n\nNested OpenSpec roots should index as OpenSpec.\n"), 0o644)
	os.WriteFile(filepath.Join(nestedChangeDir, "design.md"), []byte("# Flow Design\n\nUse collector batches.\n"), 0o644)
	os.WriteFile(filepath.Join(nestedChangeDir, "tasks.md"), []byte("# Flow Tasks\n\n- [ ] Wire collector.\n"), 0o644)
	os.WriteFile(filepath.Join(nestedDeltaSpecDir, "spec.md"), []byte("# Flow Delta\n\n## ADDED Requirements\n\n- Flow import works.\n"), 0o644)

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&openspec.Adapter{}, &markdown.Adapter{}})
	cfg := config.WithIntentCandidateDiscovery(nil, true)
	result, err := s.Run(context.Background(), repoRoot, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Found["openspec"] != 13 {
		t.Fatalf("openspec found = %d, want 13", result.Found["openspec"])
	}
	if result.OpenSpec == nil {
		t.Fatal("expected OpenSpec metrics")
	}
	if result.OpenSpec.BundleRecall != 1 {
		t.Fatalf("OpenSpec bundle recall = %.3f, metrics=%#v", result.OpenSpec.BundleRecall, result.OpenSpec)
	}
	if result.OpenSpec.ChildRoleRecall != 1 {
		t.Fatalf("OpenSpec child-role recall = %.3f, metrics=%#v", result.OpenSpec.ChildRoleRecall, result.OpenSpec)
	}
	if result.OpenSpec.DuplicatePressure != 4 {
		t.Fatalf("OpenSpec duplicate pressure = %.3f, metrics=%#v", result.OpenSpec.DuplicatePressure, result.OpenSpec)
	}
	if result.OpenSpec.MarkdownLeakage != 0 {
		t.Fatalf("OpenSpec markdown leakage = %d, metrics=%#v", result.OpenSpec.MarkdownLeakage, result.OpenSpec)
	}

	var markdownOpenSpec int
	err = db.QueryRow(`SELECT COUNT(*)
		FROM sources
		WHERE source_type = 'markdown' AND (path LIKE 'openspec/%' OR path LIKE '%/openspec/%')`).Scan(&markdownOpenSpec)
	if err != nil {
		t.Fatal(err)
	}
	if markdownOpenSpec != 0 {
		t.Fatalf("OpenSpec markdown files should be owned by openspec adapter, got %d markdown sources", markdownOpenSpec)
	}

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
	if err != nil {
		t.Fatalf("artifact source_identity %q: %v", sourceIdentity, err)
	}
	return id
}

func assertLinkExists(t *testing.T, db *store.DB, artifactID, linkType, target string) {
	t.Helper()
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM links WHERE artifact_id = ? AND link_type = ? AND target = ?`, artifactID, linkType, target).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("link %s %s -> %s count = %d, want 1", artifactID, linkType, target, count)
	}
}

func testdataSamplesRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "samples"))
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}

	repoRoot := filepath.Join(tmp, "repo")
	dstDir := filepath.Join(repoRoot, ".cursor", "plans")
	os.MkdirAll(dstDir, 0o755)
	dstPath := filepath.Join(dstDir, "sample_cursor_plan.plan.md")
	if err := os.WriteFile(dstPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	if _, err := s.Run(context.Background(), repoRoot, nil); err != nil {
		t.Fatal(err)
	}

	var artifactID string
	err = db.QueryRow("SELECT id FROM artifacts LIMIT 1").Scan(&artifactID)
	if err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query("SELECT tag FROM artifact_tags WHERE artifact_id = ?", artifactID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			t.Fatal(err)
		}
		if tag == "cursor" {
			t.Fatalf("path-derived tool slug must not appear in artifact_tags after scan, got tag %q", tag)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	var profile string
	err = db.QueryRow("SELECT format_profile FROM sources WHERE artifact_id = ?", artifactID).Scan(&profile)
	if err != nil {
		t.Fatal(err)
	}
	if profile != format.ProfileCursorPlan {
		t.Fatalf("sources.format_profile: want %q, got %q", format.ProfileCursorPlan, profile)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ids := idgen.NewFactory()
	s := New(db, ids, []adapters.Adapter{&markdown.Adapter{}})
	res, err := s.Run(context.Background(), repoRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Found["markdown"] != 2 {
		t.Fatalf("Found markdown: want 2, got %d", res.Found["markdown"])
	}
	var mdRow *SourceBreakdownRow
	for i := range res.SourcesBreakdown {
		if res.SourcesBreakdown[i].SourceType == "markdown" {
			mdRow = &res.SourcesBreakdown[i]
			break
		}
	}
	if mdRow == nil {
		t.Fatal("no markdown breakdown row")
	}
	if mdRow.Count != 2 {
		t.Fatalf("markdown count: want 2, got %d", mdRow.Count)
	}
	g := mdRow.Formats[format.ProfileGeneric]
	c := mdRow.Formats[format.ProfileCursorPlan]
	if g != 1 || c != 1 {
		t.Fatalf("expected generic=1 and cursor_plan=1, got formats %#v", mdRow.Formats)
	}
}

func TestScan_FreshIndexBatchDeferredFTSEquivalence(t *testing.T) {
	repoRoot := setupFreshIndexSpeedRepo(t)

	canonicalDB, canonical := runFreshIndexSpeedScan(t, repoRoot, false, 1)
	freshDB, fresh := runFreshIndexSpeedScan(t, repoRoot, true, 4)

	if !reflect.DeepEqual(canonical.Found, fresh.Found) {
		t.Fatalf("Found mismatch:\ncanonical=%#v\nfresh=%#v", canonical.Found, fresh.Found)
	}
	if canonical.New != fresh.New {
		t.Fatalf("New mismatch: canonical=%d fresh=%d", canonical.New, fresh.New)
	}
	for _, table := range []string{
		"artifacts",
		"artifact_revisions",
		"sources",
		"artifact_todos",
		"artifact_criteria",
		"artifact_tags",
		"artifact_sections",
		"artifact_sections_fts",
		"artifacts_fts",
		"concepts",
		"concept_mentions",
		"artifact_edges",
	} {
		canonicalCount := tableCount(t, canonicalDB, table)
		freshCount := tableCount(t, freshDB, table)
		if canonicalCount != freshCount {
			t.Fatalf("%s count mismatch: canonical=%d fresh=%d", table, canonicalCount, freshCount)
		}
	}
	if got, want := artifactIdentitySnapshot(t, freshDB), artifactIdentitySnapshot(t, canonicalDB); !reflect.DeepEqual(got, want) {
		t.Fatalf("artifact identity snapshot mismatch:\ngot:  %#v\nwant: %#v", got, want)
	}
	hits, err := freshDB.FindArtifacts("duplicate replay", store.FilterParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected deferred FTS to be populated before scan completes")
	}
}

func TestScan_FreshIndexParallelismIsDeterministic(t *testing.T) {
	repoRoot := setupFreshIndexSpeedRepo(t)

	oneWorkerDB, oneWorker := runFreshIndexSpeedScan(t, repoRoot, true, 1)
	manyWorkersDB, manyWorkers := runFreshIndexSpeedScan(t, repoRoot, true, 4)

	if !reflect.DeepEqual(oneWorker.Found, manyWorkers.Found) {
		t.Fatalf("Found mismatch:\none=%#v\nmany=%#v", oneWorker.Found, manyWorkers.Found)
	}
	if got, want := artifactIdentitySnapshot(t, manyWorkersDB), artifactIdentitySnapshot(t, oneWorkerDB); !reflect.DeepEqual(got, want) {
		t.Fatalf("parallel fresh index changed artifact identity order:\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestScan_FreshIndexProgressIncludesGranularTimings(t *testing.T) {
	repoRoot := setupFreshIndexSpeedRepo(t)
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	cfg := config.WithCodeCommentArtifacts(config.WithTestCaseArtifacts(config.WithDefaultIntentCandidateDiscovery(nil, true), true), true)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{
		&markdown.Adapter{},
		&testcase.Adapter{},
		&codecomment.Adapter{},
	})
	var events []ProgressEvent
	if _, err := scanner.RunWithOptions(context.Background(), repoRoot, cfg, RunOptions{
		UseTransaction:       true,
		SkipAuthoredAtLookup: true,
		FreshIndex:           true,
		FileWorkerCount:      2,
		Progress: func(event ProgressEvent) {
			events = append(events, event)
		},
	}); err != nil {
		t.Fatal(err)
	}
	if !hasScanProgressEvent(events, "extract", "adapter_done") {
		t.Fatalf("missing extract timing event: %#v", events)
	}
	if !hasScanProgressEvent(events, "fresh_index_writer", "rows_flushed") {
		t.Fatalf("missing writer flush timing event: %#v", events)
	}
	if !hasScanProgressEvent(events, "fresh_index_fts", "done") {
		t.Fatalf("missing FTS timing event: %#v", events)
	}
}

func hasScanProgressEvent(events []ProgressEvent, phase, event string) bool {
	for _, got := range events {
		if got.Phase == phase && got.Event == event {
			return true
		}
	}
	return false
}

func setupFreshIndexSpeedRepo(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(filepath.Join(root, "docs", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := "# Billing Retry Plan\n\n" +
		"## Replay Boundary\n\n" +
		"- [ ] Preserve duplicate replay protection\n\n" +
		"## Acceptance Criteria\n\n" +
		"- [ ] Duplicate webhook replay is rejected\n"
	if err := os.WriteFile(filepath.Join(root, "docs", "plans", "billing.md"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	source := "// TODO because duplicate webhook replay must stay idempotent for legacy callers.\n" +
		"export function retryBilling() { return true }\n"
	if err := os.WriteFile(filepath.Join(root, "src", "billing.ts"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	testSource := "describe(\"billing retries\", () => {\n" +
		"  it(\"rejects duplicate replay\", () => {\n" +
		"    expect(retryBilling()).toBe(true)\n" +
		"  })\n" +
		"  it(\"keeps legacy compatibility\", () => {\n" +
		"    expect(retryBilling()).toBe(true)\n" +
		"  })\n" +
		"})\n"
	if err := os.WriteFile(filepath.Join(root, "src", "billing.test.ts"), []byte(testSource), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func runFreshIndexSpeedScan(t *testing.T, repoRoot string, fresh bool, workers int) (*store.DB, *Result) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	cfg := config.WithCodeCommentArtifacts(config.WithTestCaseArtifacts(config.WithDefaultIntentCandidateDiscovery(nil, true), true), true)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{
		&markdown.Adapter{},
		&testcase.Adapter{},
		&codecomment.Adapter{},
	})
	result, err := scanner.RunWithOptions(context.Background(), repoRoot, cfg, RunOptions{
		UseTransaction:       true,
		SkipAuthoredAtLookup: true,
		FreshIndex:           fresh,
		FileWorkerCount:      workers,
	})
	if err != nil {
		t.Fatal(err)
	}
	return db, result
}

func tableCount(t *testing.T, db *store.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func resultRepoID(t *testing.T, db *store.DB) string {
	t.Helper()
	var repoID string
	if err := db.QueryRow("SELECT id FROM repos LIMIT 1").Scan(&repoID); err != nil {
		t.Fatal(err)
	}
	return repoID
}

func runGitCommand(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func artifactIdentitySnapshot(t *testing.T, db *store.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT s.source_identity, COALESCE(a.short_id,''), a.kind, COALESCE(a.subtype,''), a.title
FROM sources s
JOIN artifacts a ON a.id = s.artifact_id
ORDER BY s.source_identity`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var identity, shortID, kind, subtype, title string
		if err := rows.Scan(&identity, &shortID, &kind, &subtype, &title); err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.Join([]string{identity, shortID, kind, subtype, title}, "\x00"))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
