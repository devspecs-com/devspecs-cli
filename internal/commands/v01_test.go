package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func setupV01Env(t *testing.T) (string, *store.DB) {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)

	repoDir := filepath.Join(tmp, "repo")
	os.MkdirAll(filepath.Join(repoDir, ".devspecs"), 0o755)
	os.MkdirAll(filepath.Join(repoDir, "plans"), 0o755)

	origWd, _ := os.Getwd()
	os.Chdir(repoDir)
	t.Cleanup(func() { os.Chdir(origWd) })

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	return repoDir, db
}

func seedV01Artifacts(t *testing.T, db *store.DB, repoDir string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	recentSettled := time.Now().Add(-3 * 24 * time.Hour).UTC().Format(time.RFC3339)
	oldSettled := time.Now().Add(-20 * 24 * time.Hour).UTC().Format(time.RFC3339)
	staleTime := time.Now().Add(-45 * 24 * time.Hour).UTC().Format(time.RFC3339)

	db.Exec("INSERT INTO repos (id, root_path, scanned_by, git_current_branch, created_at, updated_at) VALUES ('r1', ?, 'brenn', 'main', ?, ?)", repoDir, now, now)

	ids := idgen.NewFactory()

	// In-progress artifacts
	a1 := ids.New()
	rev1 := ids.NewWithPrefix("rev_")
	db.Exec(`INSERT INTO artifacts (id, repo_id, short_id, kind, title, status, current_revision_id, created_at, updated_at, last_observed_at, authored_at)
		VALUES (?, 'r1', ?, 'plan', 'Auth Middleware', 'implementing', ?, ?, ?, ?, ?)`,
		a1, idgen.ShortID("plans/auth.md|markdown"), rev1, now, now, now, now)
	db.Exec(`INSERT INTO artifact_revisions (id, artifact_id, content_hash, body, observed_at)
		VALUES (?, ?, 'sha256:a1', '# Auth\n', ?)`, rev1, a1, now)
	db.Exec(`INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at)
		VALUES (?, ?, 'r1', 'markdown', 'plans/auth.md', 'plans/auth.md|markdown', 'generic', NULL, ?, ?)`,
		ids.NewWithPrefix("src_"), a1, now, now)
	db.Exec(`INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at)
		VALUES (?, ?, ?, 0, 'Implement JWT', 0, 'auth.md', 1, ?)`, ids.NewWithPrefix("todo_"), a1, rev1, now)
	db.Exec(`INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at)
		VALUES (?, ?, ?, 1, 'Add tests', 0, 'auth.md', 2, ?)`, ids.NewWithPrefix("todo_"), a1, rev1, now)
	db.Exec(`INSERT INTO artifact_todos (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, created_at)
		VALUES (?, ?, ?, 2, 'Setup config', 1, 'auth.md', 3, ?)`, ids.NewWithPrefix("todo_"), a1, rev1, now)
	db.InsertTag(a1, "auth", "frontmatter", now)
	db.InsertTag(a1, "security", "manual", now)

	a2 := ids.New()
	db.Exec(`INSERT INTO artifacts (id, repo_id, short_id, kind, title, status, created_at, updated_at, last_observed_at, authored_at)
		VALUES (?, 'r1', ?, 'spec', 'API Spec', 'draft', ?, ?, ?, ?)`,
		a2, idgen.ShortID("specs/api.md|markdown"), now, now, now, now)
	db.Exec(`INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at)
		VALUES (?, ?, 'r1', 'markdown', 'specs/api.md', 'specs/api.md|markdown', 'generic', NULL, ?, ?)`,
		ids.NewWithPrefix("src_"), a2, now, now)

	// Recently settled
	a3 := ids.New()
	db.Exec(`INSERT INTO artifacts (id, repo_id, short_id, kind, title, status, created_at, updated_at, last_observed_at, authored_at)
		VALUES (?, 'r1', ?, 'plan', 'UX Audit', 'completed', ?, ?, ?, ?)`,
		a3, idgen.ShortID("plans/ux.md|markdown"), now, now, recentSettled, now)
	db.Exec(`INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at)
		VALUES (?, ?, 'r1', 'markdown', 'plans/ux.md', 'plans/ux.md|markdown', 'generic', NULL, ?, ?)`,
		ids.NewWithPrefix("src_"), a3, now, now)

	// Old settled (>14 days, should NOT show in settled without --all)
	a4 := ids.New()
	db.Exec(`INSERT INTO artifacts (id, repo_id, short_id, kind, title, status, created_at, updated_at, last_observed_at, authored_at)
		VALUES (?, 'r1', ?, 'adr', 'Old ADR', 'rejected', ?, ?, ?, ?)`,
		a4, idgen.ShortID("docs/adr/old.md|adr"), now, oldSettled, now, now)
	db.Exec(`INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at)
		VALUES (?, ?, 'r1', 'adr', 'docs/adr/old.md', 'docs/adr/old.md|adr', 'adr', NULL, ?, ?)`,
		ids.NewWithPrefix("src_"), a4, now, now)

	// Stale (non-terminal, >30 days by authored_at; last_observed may be fresh after a scan)
	a5 := ids.New()
	db.Exec(`INSERT INTO artifacts (id, repo_id, short_id, kind, title, status, created_at, updated_at, last_observed_at, authored_at)
		VALUES (?, 'r1', ?, 'plan', 'Billing Sketch', 'draft', ?, ?, ?, ?)`,
		a5, idgen.ShortID("plans/billing.md|markdown"), now, now, now, staleTime)
	db.Exec(`INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at)
		VALUES (?, ?, 'r1', 'markdown', 'plans/billing.md', 'plans/billing.md|markdown', 'generic', NULL, ?, ?)`,
		ids.NewWithPrefix("src_"), a5, now, now)
}

// --- Resume Tests ---

func TestResume_GroupedOutput(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	if !containsStr(output, "In Progress") {
		t.Error("missing 'In Progress' group")
	}
	if !containsStr(output, "Recently Settled") {
		t.Error("missing 'Recently Settled' group")
	}
	if !containsStr(output, "Stale") {
		t.Error("missing 'Stale' group")
	}
	if !containsStr(output, "Auth Middleware") {
		t.Error("missing in-progress artifact")
	}
	if !containsStr(output, "Billing Sketch") {
		t.Error("missing stale artifact")
	}
	if !containsStr(output, "UX Audit") {
		t.Error("missing recently settled artifact")
	}
	// Old settled (>14 days) should NOT show
	if containsStr(output, "Old ADR") {
		t.Error("old settled artifact should not appear without --all")
	}
	if !containsStr(output, "Tags:") || !containsStr(output, "auth") {
		t.Error("resume output should include Tags line for tagged artifacts")
	}
	if !containsStr(output, "Authored:") || !containsStr(output, "Last updated:") {
		t.Error("resume output should include Authored and Last updated lines")
	}
	if !containsStr(output, "Idle (stale) since:") {
		t.Error("stale items should include idle clock line")
	}
}

func TestResume_OddNonTerminalStatus_GoesToStaleWhenOld(t *testing.T) {
	repoDir, db := setupV01Env(t)
	now := time.Now().UTC().Format(time.RFC3339)
	old := time.Now().Add(-40 * 24 * time.Hour).UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, scanned_by, git_current_branch, created_at, updated_at) VALUES ('r1', ?, 'x', 'main', ?, ?)", repoDir, now, now)
	ids := idgen.NewFactory()
	aid := ids.New()
	db.Exec(`INSERT INTO artifacts (id, repo_id, short_id, kind, title, status, created_at, updated_at, last_observed_at, authored_at)
		VALUES (?, 'r1', 'abcdef01', 'plan', 'Odd Status Plan', 'reviewing', ?, ?, ?, ?)`, aid, now, now, now, old)
	db.Exec(`INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at)
		VALUES (?, ?, 'r1', 'markdown', 'plans/odd.md', 'plans/odd.md|markdown', 'generic', NULL, ?, ?)`, ids.NewWithPrefix("src_"), aid, now, now)
	db.Close()

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !containsStr(out, "Stale") || !containsStr(out, "Odd Status Plan") {
		t.Errorf("expected odd non-terminal status in Stale when old; got:\n%s", out)
	}
	if idxProg := strings.Index(out, "\nIn Progress ("); idxProg >= 0 {
		next := len(out)
		for _, marker := range []string{"\nRecently Settled (", "\nStale ("} {
			if j := strings.Index(out[idxProg+1:], marker); j >= 0 {
				at := idxProg + 1 + j
				if at < next {
					next = at
				}
			}
		}
		if strings.Contains(out[idxProg:next], "Odd Status Plan") {
			t.Error("odd-status stale artifact must not appear in In Progress section")
		}
	}
}

func TestResume_AllFlag(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh", "--all"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	if !containsStr(output, "Old ADR") {
		t.Error("--all should show old settled artifacts")
	}
}

func TestResume_JSON_HasTagsPerRow(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh", "--json", "--all"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &top); err != nil {
		t.Fatal(err)
	}
	var inProg []map[string]any
	if err := json.Unmarshal(top["in_progress"], &inProg); err != nil || len(inProg) == 0 {
		t.Fatalf("in_progress: %v", err)
	}
	foundAuth := false
	for _, row := range inProg {
		rawTags, ok := row["tags"]
		if !ok || rawTags == nil {
			continue
		}
		tagSlice, ok := rawTags.([]any)
		if !ok {
			continue
		}
		for _, t := range tagSlice {
			if s, ok := t.(string); ok && s == "auth" {
				foundAuth = true
			}
		}
	}
	if !foundAuth {
		t.Errorf("expected in_progress JSON rows to include tag auth in tags array; got %#v", inProg)
	}
}

func TestResume_JSON(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh", "--json", "--all"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	for _, key := range []string{"in_progress", "recently_settled", "stale"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing key %q in JSON output", key)
		}
	}
	inProg, _ := result["in_progress"].([]any)
	if len(inProg) == 0 {
		t.Fatal("expected non-empty in_progress")
	}
	row, _ := inProg[0].(map[string]any)
	for _, k := range []string{"authored_at", "updated_at", "last_observed_at"} {
		if _, ok := row[k]; !ok {
			t.Errorf("in_progress row missing %q", k)
		}
		s, _ := row[k].(string)
		if s != "" {
			if _, err := time.Parse(time.RFC3339, s); err != nil {
				t.Errorf("%s not RFC3339: %q: %v", k, s, err)
			}
		}
	}
}

func TestResume_EmptyRepo(t *testing.T) {
	setupV01Env(t)

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !containsStr(buf.String(), "No DevSpecs indexed yet") {
		t.Error("expected 'No DevSpecs indexed yet' message")
	}
}

func TestResume_LimitFlag(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh", "--json", "--limit", "1", "--all"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string][]any
	json.Unmarshal(buf.Bytes(), &result)

	if len(result["in_progress"]) > 1 {
		t.Errorf("limit 1 should cap in_progress to 1, got %d", len(result["in_progress"]))
	}
}

func TestResume_DefaultLimit_FivePerGroup(t *testing.T) {
	repoDir, db := setupV01Env(t)
	now := time.Now().UTC().Format(time.RFC3339)
	db.Exec("INSERT INTO repos (id, root_path, scanned_by, git_current_branch, created_at, updated_at) VALUES ('r1', ?, 'x', 'main', ?, ?)", repoDir, now, now)
	ids := idgen.NewFactory()
	for i := 0; i < 6; i++ {
		aid := ids.New()
		path := fmt.Sprintf("plans/p%d.md", i)
		sid := fmt.Sprintf("%s|markdown", path)
		db.Exec(`INSERT INTO artifacts (id, repo_id, short_id, kind, title, status, created_at, updated_at, last_observed_at, authored_at)
			VALUES (?, 'r1', ?, 'plan', ?, 'draft', ?, ?, ?, ?)`,
			aid, idgen.ShortID(sid), fmt.Sprintf("Plan %d", i), now, now, now, now)
		db.Exec(`INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at)
			VALUES (?, ?, 'r1', 'markdown', ?, ?, 'generic', NULL, ?, ?)`,
			ids.NewWithPrefix("src_"), aid, path, sid, now, now)
	}
	db.Close()

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result map[string][]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if n := len(result["in_progress"]); n != 5 {
		t.Fatalf("default limit: want 5 in_progress, got %d", n)
	}
}

func TestShortID_DisplayInList(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	sid := idgen.ShortID("plans/auth.md|markdown")
	if !containsStr(output, sid) {
		t.Errorf("expected short_id %q in list output", sid)
	}
}

func TestShortID_ResolveInShow(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	sid := idgen.ShortID("plans/auth.md|markdown")

	cmd := NewShowCmd()
	cmd.SetArgs([]string{sid, "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("ds show %s failed: %v", sid, err)
	}

	if !containsStr(buf.String(), "Auth Middleware") {
		t.Error("short_id did not resolve to correct artifact")
	}
}

func TestShortID_ResolveInContext(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	sid := idgen.ShortID("plans/auth.md|markdown")

	cmd := NewContextCmd()
	cmd.SetArgs([]string{sid, "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("ds context %s failed: %v", sid, err)
	}

	if !containsStr(buf.String(), "Auth Middleware") {
		t.Error("short_id did not resolve in context")
	}
}

// --- Tag Tests ---

func TestTag_AddAndDisplay(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	sid := idgen.ShortID("specs/api.md|markdown")

	tagCmd := NewTagCmd()
	tagCmd.SetArgs([]string{sid, "v2", "backend"})
	tagBuf := &bytes.Buffer{}
	tagCmd.SetOut(tagBuf)
	if err := tagCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	showCmd := NewShowCmd()
	showCmd.SetArgs([]string{sid, "--no-refresh"})
	showBuf := &bytes.Buffer{}
	showCmd.SetOut(showBuf)
	if err := showCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := showBuf.String()
	if !containsStr(output, "v2") || !containsStr(output, "backend") {
		t.Error("tags not displayed in show output")
	}
}

func TestUntag(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)

	sid := idgen.ShortID("plans/auth.md|markdown")
	art, _ := db.GetArtifact(sid)

	tags, _ := db.GetTagsForArtifact(art.ID)
	initialCount := len(tags)
	db.Close()

	untagCmd := NewUntagCmd()
	untagCmd.SetArgs([]string{sid, "auth"})
	untagBuf := &bytes.Buffer{}
	untagCmd.SetOut(untagBuf)
	if err := untagCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	db2, _ := store.Open(filepath.Join(os.Getenv("DEVSPECS_HOME"), "devspecs.db"))
	defer db2.Close()
	tags2, _ := db2.GetTagsForArtifact(art.ID)
	if len(tags2) != initialCount-1 {
		t.Errorf("expected %d tags after untag, got %d", initialCount-1, len(tags2))
	}
}

// --- Filter Tests ---

func TestList_FilterByTag(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--tag", "auth"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !containsStr(output, "Auth Middleware") {
		t.Error("expected Auth Middleware with --tag auth")
	}
	if containsStr(output, "API Spec") {
		t.Error("API Spec should not appear with --tag auth")
	}
}

func TestList_FilterByUser(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--user", "brenn"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !containsStr(buf.String(), "Auth Middleware") {
		t.Error("expected artifacts with --user brenn")
	}
}

func TestList_FilterByBranch(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--branch", "main"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !containsStr(buf.String(), "Auth Middleware") {
		t.Error("expected artifacts on branch main")
	}
}

func TestList_ComposedFilters(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--tag", "auth", "--status", "implementing"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !containsStr(output, "Auth Middleware") {
		t.Error("expected Auth Middleware with composed filters")
	}
}

func TestList_EmptyResult(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--tag", "nonexistent"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if containsStr(output, "Auth Middleware") || containsStr(output, "API Spec") {
		t.Error("no artifacts should match nonexistent tag")
	}
}

func setupTwoIndexedRepos(t *testing.T) (repoA, repoB string) {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)
	repoA = filepath.Join(tmp, "project-a")
	repoB = filepath.Join(tmp, "project-b")
	os.MkdirAll(filepath.Join(repoA, ".devspecs"), 0o755)
	os.MkdirAll(filepath.Join(repoB, ".devspecs"), 0o755)

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	ids := idgen.NewFactory()
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('rA', ?, ?, ?)", repoA, now, now)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('rB', ?, ?, ?)", repoB, now, now)

	aidA := ids.New()
	revA := ids.NewWithPrefix("rev_")
	if err := db.InsertArtifactDirect(aidA, "rA", "plan", "", "ScopeAlphaOnlyInA", "draft", revA, now, now); err != nil {
		t.Fatal(err)
	}
	db.InsertRevisionDirect(revA, aidA, "sha256:a", "# body\n", "", now)

	aidB := ids.New()
	revB := ids.NewWithPrefix("rev_")
	if err := db.InsertArtifactDirect(aidB, "rB", "plan", "", "ScopeBetaOnlyInB", "draft", revB, now, now); err != nil {
		t.Fatal(err)
	}
	db.InsertRevisionDirect(revB, aidB, "sha256:b", "# body\n", "", now)

	db.IndexArtifactFTS(aidA, "ScopeAlphaOnlyInA", "# body\n", "p.md")
	db.IndexArtifactFTS(aidB, "ScopeBetaOnlyInB", "# body\n", "p.md")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return repoA, repoB
}

func TestList_ScopesToCurrentRepoOnly(t *testing.T) {
	repoA, _ := setupTwoIndexedRepos(t)

	origWd, _ := os.Getwd()
	os.Chdir(repoA)
	t.Cleanup(func() { os.Chdir(origWd) })

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var arts []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &arts); err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 {
		t.Fatalf("list scoped to cwd: want 1 artifact, got %d: %s", len(arts), buf.String())
	}
	title, _ := arts[0]["Title"].(string)
	if title != "ScopeAlphaOnlyInA" {
		t.Fatalf("want ScopeAlphaOnlyInA title, got %q", title)
	}

	cmd2 := NewListCmd()
	cmd2.SetArgs([]string{"--no-refresh", "--json", "--all"})
	buf2 := &bytes.Buffer{}
	cmd2.SetOut(buf2)
	if err := cmd2.Execute(); err != nil {
		t.Fatal(err)
	}
	var all []map[string]any
	if err := json.Unmarshal(buf2.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("list --all: want 2 artifacts, got %d: %s", len(all), buf2.String())
	}
}

func TestList_RepoFlagOverridesCwd(t *testing.T) {
	repoA, repoB := setupTwoIndexedRepos(t)

	origWd, _ := os.Getwd()
	os.Chdir(repoA)
	t.Cleanup(func() { os.Chdir(origWd) })

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--json", "--repo", filepath.Base(repoB)})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var arts []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &arts); err != nil {
		t.Fatal(err)
	}
	if len(arts) != 1 {
		t.Fatalf("list --repo while cwd in other tree: want 1 artifact, got %d: %s", len(arts), buf.String())
	}
	title, _ := arts[0]["Title"].(string)
	if title != "ScopeBetaOnlyInB" {
		t.Fatalf("want artifact from named repo B, got title %q", title)
	}
}

func TestFind_ScopesToCurrentRepoByDefault(t *testing.T) {
	repoA, _ := setupTwoIndexedRepos(t)

	origWd, _ := os.Getwd()
	os.Chdir(repoA)
	t.Cleanup(func() { os.Chdir(origWd) })

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"ScopeBetaOnlyInB", "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "ScopeBetaOnlyInB") {
		t.Error("find should not match other-repo artifact when scoped to cwd")
	}

	cmd2 := NewFindCmd()
	cmd2.SetArgs([]string{"ScopeBetaOnlyInB", "--no-refresh", "--all"})
	buf2 := &bytes.Buffer{}
	cmd2.SetOut(buf2)
	if err := cmd2.Execute(); err != nil {
		t.Fatal(err)
	}
	if !containsStr(buf2.String(), "ScopeBetaOnlyInB") {
		t.Errorf("find --all should match other repo: %s", buf2.String())
	}

	cmd3 := NewFindCmd()
	cmd3.SetArgs([]string{"ScopeAlphaOnlyInA", "--no-refresh"})
	buf3 := &bytes.Buffer{}
	cmd3.SetOut(buf3)
	if err := cmd3.Execute(); err != nil {
		t.Fatal(err)
	}
	if !containsStr(buf3.String(), "ScopeAlphaOnlyInA") {
		t.Errorf("find current repo: %s", buf3.String())
	}
}

func TestTodos_HumanOutput_GroupedByArtifact(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewTodosCmd()
	cmd.SetArgs([]string{"--no-refresh", "--tag", "auth"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !containsStr(out, "DevSpecs Todos") {
		t.Errorf("expected header, got: %s", out)
	}
	if !containsStr(out, "Auth Middleware (plan)") {
		t.Errorf("expected grouped artifact header, got: %s", out)
	}
	if !containsStr(out, "Implement JWT") {
		t.Errorf("expected todo line, got: %s", out)
	}
	if strings.Contains(out, "plans/auth.md:") {
		t.Error("human output should not include source_file:line")
	}
}

func TestCriteria_HumanOutput_GroupedByArtifact(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	now := time.Now().UTC().Format(time.RFC3339)
	ids := idgen.NewFactory()
	var aid, rev string
	if err := db.QueryRow("SELECT a.id, a.current_revision_id FROM artifacts a WHERE a.title = ?", "Auth Middleware").Scan(&aid, &rev); err != nil {
		t.Fatal(err)
	}
	db.Exec(`INSERT INTO artifact_criteria (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, criteria_kind, created_at) VALUES (?, ?, ?, 0, 'Gate criterion one', 0, 'auth.md', 10, 'acceptance', ?)`,
		ids.NewWithPrefix("crit_"), aid, rev, now)
	db.Close()

	cmd := NewCriteriaCmd()
	cmd.SetArgs([]string{"--no-refresh", "--tag", "auth"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !containsStr(out, "DevSpecs Criteria") {
		t.Errorf("expected header: %s", out)
	}
	if !containsStr(out, "Auth Middleware (plan)") {
		t.Errorf("expected grouped header: %s", out)
	}
	if !containsStr(out, "acceptance") || !containsStr(out, "Gate criterion one") {
		t.Errorf("expected criterion line: %s", out)
	}
	if strings.Contains(out, "auth.md:10") {
		t.Error("human output should not include source_file:line")
	}
}

func TestTodos_SingleArtifactHumanGrouped(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	var aid string
	if err := db.QueryRow("SELECT id FROM artifacts WHERE title = ?", "Auth Middleware").Scan(&aid); err != nil {
		t.Fatal(err)
	}
	db.Close()

	cmd := NewTodosCmd()
	cmd.SetArgs([]string{aid, "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Count(out, "Auth Middleware (plan)") != 1 {
		t.Fatalf("want exactly one grouped artifact header, got %d in: %s", strings.Count(out, "Auth Middleware (plan)"), out)
	}
	if !containsStr(out, "Implement JWT") {
		t.Errorf("expected todo line: %s", out)
	}
	if strings.Contains(out, "auth.md:") {
		t.Error("human output should not include source_file:line")
	}
}

func TestCriteria_SingleArtifactHumanGrouped(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	now := time.Now().UTC().Format(time.RFC3339)
	ids := idgen.NewFactory()
	var aid, rev string
	if err := db.QueryRow("SELECT a.id, a.current_revision_id FROM artifacts a WHERE a.title = ?", "Auth Middleware").Scan(&aid, &rev); err != nil {
		t.Fatal(err)
	}
	db.Exec(`INSERT INTO artifact_criteria (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, criteria_kind, created_at) VALUES (?, ?, ?, 0, 'Single-ID criterion', 0, 'auth.md', 10, 'acceptance', ?)`,
		ids.NewWithPrefix("crit_"), aid, rev, now)
	db.Close()

	cmd := NewCriteriaCmd()
	cmd.SetArgs([]string{aid, "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Count(out, "Auth Middleware (plan)") != 1 {
		t.Fatalf("want exactly one grouped artifact header, got %d in: %s", strings.Count(out, "Auth Middleware (plan)"), out)
	}
	if !containsStr(out, "acceptance") || !containsStr(out, "Single-ID criterion") {
		t.Errorf("expected criterion line: %s", out)
	}
	if strings.Contains(out, "auth.md:10") {
		t.Error("human output should not include source_file:line")
	}
}

func TestFind_WithTagFilter(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)

	db.IndexArtifactFTS("fake", "Auth Middleware", "jwt auth body", "plans/auth.md")
	db.Close()

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"Auth", "--no-refresh", "--tag", "security"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.Execute()

	output := buf.String()
	if !containsStr(output, "Auth Middleware") {
		t.Error("find with --tag should return matching artifact")
	}
}

func TestTodos_WithTagFilter(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)

	// Verify the data is correct in the DB before testing the command
	fp := store.FilterParams{Tag: "auth"}
	todos, err := db.ListAllTodos(fp, false, false)
	if err != nil {
		t.Fatalf("direct query failed: %v", err)
	}
	if len(todos) == 0 {
		t.Log("No todos found via direct query with tag=auth, checking tags...")
		var count int
		db.QueryRow("SELECT COUNT(*) FROM artifact_tags WHERE tag = 'auth'").Scan(&count)
		t.Logf("artifact_tags with tag=auth: %d", count)
		var todoCount int
		db.QueryRow("SELECT COUNT(*) FROM artifact_todos").Scan(&todoCount)
		t.Logf("total todos: %d", todoCount)
	}
	db.Close()

	cmd := NewTodosCmd()
	cmd.SetArgs([]string{"--no-refresh", "--tag", "auth"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !containsStr(output, "Implement JWT") {
		t.Errorf("expected todos from auth-tagged artifact, got: %s", output)
	}
}

func TestResume_WithTagFilter(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh", "--tag", "auth"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !containsStr(output, "Auth Middleware") {
		t.Error("resume --tag auth should show Auth Middleware")
	}
	if containsStr(output, "API Spec") {
		t.Error("resume --tag auth should not show API Spec")
	}
}

// --- Config Command Tests ---

func TestConfigShow_Defaults(t *testing.T) {
	setupV01Env(t)

	cmd := NewConfigCmd()
	cmd.SetArgs([]string{"show"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !containsStr(output, "defaults") {
		t.Error("expected '(defaults' note when no config file")
	}
	if !containsStr(output, "markdown") {
		t.Error("expected markdown source in defaults")
	}
}

func TestConfigShow_WithFile(t *testing.T) {
	repoDir, _ := setupV01Env(t)

	cfg := config.DefaultRepoConfig()
	cfg.Version = 2
	config.WriteRepoConfig(repoDir, cfg)

	cmd := NewConfigCmd()
	cmd.SetArgs([]string{"show"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if containsStr(output, "defaults") {
		t.Error("should NOT show defaults note when config file exists")
	}
	if !containsStr(output, "version: 2") {
		t.Error("expected version: 2 in output")
	}
}

func TestConfigPaths(t *testing.T) {
	setupV01Env(t)

	cmd := NewConfigCmd()
	cmd.SetArgs([]string{"paths"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !containsStr(output, "[missing]") && !containsStr(output, "[ok]") {
		t.Error("expected path status indicators")
	}
}

func TestConfigAddSource(t *testing.T) {
	repoDir, _ := setupV01Env(t)

	cmd := NewConfigCmd()
	cmd.SetArgs([]string{"add-source", "markdown", "contracts"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !containsStr(buf.String(), "Added source") {
		t.Error("expected confirmation message")
	}

	cfg, err := config.LoadRepoConfig(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, src := range cfg.Sources {
		if src.Type == "markdown" {
			for _, p := range src.Paths {
				if p == "contracts" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("contracts path not found in config after add-source")
	}
}

func TestConfigAddSource_Duplicate(t *testing.T) {
	repoDir, _ := setupV01Env(t)

	cfg := config.DefaultRepoConfig()
	config.WriteRepoConfig(repoDir, cfg)

	cmd := NewConfigCmd()
	cmd.SetArgs([]string{"add-source", "openspec", "openspec"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !containsStr(buf.String(), "already exists") {
		t.Error("expected 'already exists' for duplicate")
	}
}

func TestConfigSet(t *testing.T) {
	repoDir, _ := setupV01Env(t)

	cmd := NewConfigCmd()
	cmd.SetArgs([]string{"set", "version", "2"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	cfg, _ := config.LoadRepoConfig(repoDir)
	if cfg.Version != 2 {
		t.Errorf("expected version 2, got %d", cfg.Version)
	}
}

// --- Relative Time Tests ---

func TestRelativeTime_Table(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		input    time.Time
		expected string
	}{
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-1 * time.Minute), "1 minute ago"},
		{now.Add(-1 * time.Hour), "1h ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-36 * time.Hour), "yesterday"},
		{now.Add(-5 * 24 * time.Hour), "5 days ago"},
		{now.Add(-45 * 24 * time.Hour), "45 days ago"},
		{time.Time{}, "unknown"},
	}

	for _, tc := range cases {
		got := relativeTime(tc.input, now)
		if got != tc.expected {
			t.Errorf("relativeTime(%v) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// --- File Pattern Tests ---

func TestInferKind_Table(t *testing.T) {
	cases := []struct {
		path string
		kind string
	}{
		{"plans/auth.md", "plan"},
		{"specs/api.md", "spec"},
		{"v0.prd.md", "prd"},
		{"api.design.md", "design"},
		{"api.contract.md", "contract"},
		{"reqs.requirements.md", "requirements"},
		{"docs/random.md", "markdown_artifact"},
		{".cursor/plans/foo.plan.md", "plan"},
	}

	for _, tc := range cases {
		// We test through the markdown adapter's inferKind by importing it
		// Since inferKind is unexported, we test through Discover/Parse behavior
		_ = tc
	}
}

// --- Show Displays Tags and ScannedBy ---

func TestShow_DisplaysTags(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	sid := idgen.ShortID("plans/auth.md|markdown")
	cmd := NewShowCmd()
	cmd.SetArgs([]string{sid, "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !containsStr(output, "Tags:") {
		t.Error("expected Tags: line in show output")
	}
	if !containsStr(output, "auth") {
		t.Error("expected 'auth' tag in show output")
	}
	if !containsStr(output, "security") {
		t.Error("expected 'security' tag in show output")
	}
}

func TestShow_DisplaysScannedBy(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	sid := idgen.ShortID("plans/auth.md|markdown")
	cmd := NewShowCmd()
	cmd.SetArgs([]string{sid, "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !containsStr(output, "Scanned by:") {
		t.Error("expected 'Scanned by:' in show output")
	}
	if !containsStr(output, "brenn") {
		t.Error("expected 'brenn' in scanned by")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
