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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupV01Env(t *testing.T) (string, *store.DB) {
	t.Helper()
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	t.Setenv("DEVSPECS_HOME", home)

	repoDir := filepath.Join(tmp, "repo")
	os.MkdirAll(filepath.Join(repoDir, ".devspecs"), 0o755)
	os.MkdirAll(filepath.Join(repoDir, "plans"), 0o755)

	origWd := testWorkingDirectory(t)
	os.Chdir(repoDir)
	t.Cleanup(func() { os.Chdir(origWd) })

	dbPath := filepath.Join(home, "devspecs.db")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.True(t, containsStr(output, "In Progress"),
		"missing 'In Progress' group")
	assert.True(t, containsStr(output, "Recently Settled"),
		"missing 'Recently Settled' group")
	assert.True(t, containsStr(output, "Stale"),
		"missing 'Stale' group")
	assert.True(t, containsStr(output, "Auth Middleware"),
		"missing in-progress artifact")
	assert.True(t, containsStr(output, "Billing Sketch"),
		"missing stale artifact")
	assert.True(t, containsStr(output, "UX Audit"),
		"missing recently settled artifact")
	assert.

		// Old settled (>14 days) should NOT show
		False(t, containsStr(output, "Old ADR"),
			"old settled artifact should not appear without --all")
	assert.True(t, containsStr(output, "Tags:"), "resume output should include Tags line for tagged artifacts")
	assert.True(t, containsStr(output, "auth"), "resume output should include Tags line for tagged artifacts")
	assert.True(t, containsStr(output, "Authored:"), "resume output should include Authored and Last updated lines")
	assert.True(t, containsStr(output, "Last updated:"), "resume output should include Authored and Last updated lines")
	assert.True(t, containsStr(output, "Idle (stale) since:"),
		"stale items should include idle clock line")

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.True(t, containsStr(out, "Stale"), "expected odd non-terminal status in Stale when old; got:\n%s", out)
	assert.True(t, containsStr(out, "Odd Status Plan"), "expected odd non-terminal status in Stale when old; got:\n%s", out)

	if idxProg := strings.Index(out, "\nIn Progress ("); idxProg >= 0 {
		next := len(out)
		{
			marker := "\nRecently Settled ("

			if j := strings.Index(out[idxProg+1:], marker); j >= 0 {
				at := idxProg + 1 + j
				if at < next {
					next = at
				}
			}

		}
		{
			marker := "\nStale ("

			if j := strings.Index(out[idxProg+1:], marker); j >= 0 {
				at := idxProg + 1 + j
				if at < next {
					next = at
				}
			}

		}

		assert.NotContains(t, out[idxProg:next], "Odd Status Plan",
			"odd-status stale artifact must not appear in In Progress section")

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.True(t, containsStr(output, "Old ADR"),
		"--all should show old settled artifacts")

}

func TestResume_JSON_HasTagsPerRow(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh", "--json", "--all"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var top map[string]json.RawMessage
	{
		err := json.Unmarshal(buf.Bytes(), &top)
		require.NoError(t, err)
	}

	var inProg []map[string]any
	{
		err := json.Unmarshal(top["in_progress"], &inProg)
		require.NoError(t, err)
		require.NotEmpty(t, inProg, "in_progress: %v", err)
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
	assert.True(t, foundAuth,
		"expected in_progress JSON rows to include tag auth in tags array; got %#v", inProg)

}

func TestResume_JSON(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh", "--json", "--all"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var result map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err,
			"invalid JSON: %v", err)
	}

	assert.Contains(t, result, "in_progress")
	assert.Contains(t, result, "recently_settled")
	assert.Contains(t, result, "stale")

	inProg, ok := result["in_progress"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, inProg,
		"expected non-empty in_progress")

	row, ok := inProg[0].(map[string]any)
	require.True(t, ok)
	assertRFC3339JSONField(t, row, "authored_at")
	assertRFC3339JSONField(t, row, "updated_at")
	assertRFC3339JSONField(t, row, "last_observed_at")

}

func assertRFC3339JSONField(t *testing.T, row map[string]any, key string) {
	t.Helper()
	value, ok := row[key]
	require.True(t, ok, "row missing %q", key)
	text, ok := value.(string)
	require.True(t, ok, "%s is not a string: %#v", key, value)
	if text == "" {
		return
	}

	_, err := time.Parse(time.RFC3339, text)
	require.NoError(t, err, "%s is not RFC3339: %q", key, text)
}

func TestResume_EmptyRepo(t *testing.T) {
	setupV01Env(t)

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.True(t, containsStr(buf.String(), "No DevSpecs indexed yet"),
		"expected 'No DevSpecs indexed yet' message")
	assert.True(t, containsStr(buf.String(), "ds recent"), "expected scanless resume guidance, got %q", buf.String())
	assert.True(t, containsStr(buf.String(), "Manual refresh: ds scan"), "expected scanless resume guidance, got %q", buf.String())

}

func TestResume_LimitFlag(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh", "--json", "--limit", "1", "--all"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var result map[string][]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.LessOrEqual(t, len(result["in_progress"]), 1,
		"limit 1 should cap in_progress to 1, got %d", len(result["in_progress"]))

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var result map[string][]any
	{
		err := json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err)
	}
	{

		n := len(result["in_progress"])
		assert.Equal(t, 5, n,
			"default limit: want 5 in_progress, got %d", n)
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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	sid := idgen.ShortID("plans/auth.md|markdown")
	assert.True(t, containsStr(output, sid),
		"expected short_id %q in list output", sid)

}

func TestList_WithJSONLimit_CapsRows(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--limit", "1", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var rows []map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &rows)
		require.NoError(t, err,
			"list json: %v\n%s", err, buf.String())
	}
	require.Len(t, rows, 1,
		"list --limit json length = %d, want 1: %s", len(rows), buf.String())
}

func TestList_WithHumanLimit_CapsRows(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--limit", "1"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 2,
		"list --limit human lines = %d, want header + 1 row:\n%s", len(lines), buf.String())

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
	{
		err := cmd.Execute()
		require.NoError(t, err,
			"ds show %s failed: %v", sid, err)
	}
	assert.True(t, containsStr(buf.String(), "Auth Middleware"),
		"short_id did not resolve to correct artifact")

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
	{
		err := cmd.Execute()
		require.NoError(t, err,
			"ds context %s failed: %v", sid, err)
	}
	assert.True(t, containsStr(buf.String(), "Auth Middleware"),
		"short_id did not resolve in context")

}

// --- Tag Tests ---

func TestTag_WithTwoTags_PersistsBothTags(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	sid := idgen.ShortID("specs/api.md|markdown")

	cmd := NewTagCmd()
	cmd.SetArgs([]string{sid, "v2", "backend"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.NoError(t, err)
	db, err = store.Open(filepath.Join(os.Getenv("DEVSPECS_HOME"), "devspecs.db"))
	require.NoError(t, err)
	defer db.Close()
	artifact, err := db.GetArtifact(sid)
	require.NoError(t, err)
	tags, err := db.GetTagsForArtifact(artifact.ID)
	require.NoError(t, err)
	require.Len(t, tags, 2)
	assert.Equal(t, "backend", tags[0].Tag)
	assert.Equal(t, "v2", tags[1].Tag)
}

func TestShow_WithManualTags_DisplaysTags(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	sid := idgen.ShortID("specs/api.md|markdown")
	artifact, err := db.GetArtifact(sid)
	require.NoError(t, err)
	now := time.Now().UTC().Format(time.RFC3339)
	require.NoError(t, db.InsertTag(artifact.ID, "v2", "manual", now))
	require.NoError(t, db.InsertTag(artifact.ID, "backend", "manual", now))
	require.NoError(t, db.Close())

	cmd := NewShowCmd()
	cmd.SetArgs([]string{sid, "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err = cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "v2")
	assert.Contains(t, buf.String(), "backend")
}

func TestUntag(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)

	sid := idgen.ShortID("plans/auth.md|markdown")
	art, err := db.GetArtifact(sid)
	require.NoError(t, err)

	tags, err := db.GetTagsForArtifact(art.ID)
	require.NoError(t, err)
	initialCount := len(tags)
	db.Close()

	untagCmd := NewUntagCmd()
	untagCmd.SetArgs([]string{sid, "auth"})
	untagBuf := &bytes.Buffer{}
	untagCmd.SetOut(untagBuf)
	{
		err := untagCmd.Execute()
		require.NoError(t, err)
	}

	db2, err := store.Open(filepath.Join(os.Getenv("DEVSPECS_HOME"), "devspecs.db"))
	require.NoError(t, err)
	defer db2.Close()
	tags2, err := db2.GetTagsForArtifact(art.ID)
	require.NoError(t, err)
	require.Len(t, tags2, initialCount-1,
		"expected %d tags after untag, got %d", initialCount-1, len(tags2))

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.True(t, containsStr(output, "Auth Middleware"),
		"expected Auth Middleware with --tag auth")
	assert.False(t, containsStr(output, "API Spec"),
		"API Spec should not appear with --tag auth")

}

func TestList_FilterByUser(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--user", "brenn"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.True(t, containsStr(buf.String(), "Auth Middleware"),
		"expected artifacts with --user brenn")

}

func TestList_FilterByBranch(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--branch", "main"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.True(t, containsStr(buf.String(), "Auth Middleware"),
		"expected artifacts on branch main")

}

func TestList_ComposedFilters(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--tag", "auth", "--status", "implementing"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.True(t, containsStr(output, "Auth Middleware"),
		"expected Auth Middleware with composed filters")

}

func TestList_EmptyResult(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--tag", "nonexistent"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.False(t, containsStr(output, "Auth Middleware"), "no artifacts should match nonexistent tag")
	assert.False(t, containsStr(output, "API Spec"), "no artifacts should match nonexistent tag")

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
	require.NoError(t, err)

	now := time.Now().UTC().Format(time.RFC3339)
	ids := idgen.NewFactory()
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('rA', ?, ?, ?)", repoA, now, now)
	db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('rB', ?, ?, ?)", repoB, now, now)

	aidA := ids.New()
	revA := ids.NewWithPrefix("rev_")
	{
		err := db.InsertArtifactDirect(aidA, "rA", "plan", "", "ScopeAlphaOnlyInA", "draft", revA, now, now)
		require.NoError(t, err)
	}

	db.InsertRevisionDirect(revA, aidA, "sha256:a", "# body\n", "", now)

	aidB := ids.New()
	revB := ids.NewWithPrefix("rev_")
	{
		err := db.InsertArtifactDirect(aidB, "rB", "plan", "", "ScopeBetaOnlyInB", "draft", revB, now, now)
		require.NoError(t, err)
	}

	db.InsertRevisionDirect(revB, aidB, "sha256:b", "# body\n", "", now)

	db.IndexArtifactFTS(aidA, "ScopeAlphaOnlyInA", "# body\n", "p.md")
	db.IndexArtifactFTS(aidB, "ScopeBetaOnlyInB", "# body\n", "p.md")
	{
		err := db.Close()
		require.NoError(t, err)
	}

	return repoA, repoB
}

func TestList_ScopesToCurrentRepoOnly(t *testing.T) {
	repoA, _ := setupTwoIndexedRepos(t)

	origWd := testWorkingDirectory(t)
	os.Chdir(repoA)
	t.Cleanup(func() { os.Chdir(origWd) })

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var arts []map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &arts)
		require.NoError(t, err)
	}
	require.Len(t, arts, 1,
		"list scoped to cwd: want 1 artifact, got %d: %s", len(arts), buf.String())

	title, ok := arts[0]["Title"].(string)
	require.True(t, ok)
	assert.Equal(t, "ScopeAlphaOnlyInA", title,
		"want ScopeAlphaOnlyInA title, got %q", title)
}

func TestList_WithAll_ReturnsArtifactsFromEveryRepo(t *testing.T) {
	repoA, _ := setupTwoIndexedRepos(t)

	origWd := testWorkingDirectory(t)
	os.Chdir(repoA)
	t.Cleanup(func() { os.Chdir(origWd) })

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--json", "--all"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var all []map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &all)
		require.NoError(t, err)
	}
	require.Len(t, all, 2,
		"list --all: want 2 artifacts, got %d: %s", len(all), buf.String())

}

func TestList_RepoFlagOverridesCwd(t *testing.T) {
	repoA, repoB := setupTwoIndexedRepos(t)

	origWd := testWorkingDirectory(t)
	os.Chdir(repoA)
	t.Cleanup(func() { os.Chdir(origWd) })

	cmd := NewListCmd()
	cmd.SetArgs([]string{"--no-refresh", "--json", "--repo", filepath.Base(repoB)})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var arts []map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &arts)
		require.NoError(t, err)
	}
	require.Len(t, arts, 1,
		"list --repo while cwd in other tree: want 1 artifact, got %d: %s", len(arts), buf.String())

	title, ok := arts[0]["Title"].(string)
	require.True(t, ok)
	assert.Equal(t, "ScopeBetaOnlyInB", title,
		"want artifact from named repo B, got title %q", title)

}

func TestFind_WithOtherRepoQuery_DoesNotReturnOtherRepoArtifact(t *testing.T) {
	repoA, _ := setupTwoIndexedRepos(t)

	origWd := testWorkingDirectory(t)
	os.Chdir(repoA)
	t.Cleanup(func() { os.Chdir(origWd) })

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"ScopeBetaOnlyInB", "--no-refresh", "--plain"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.NotContains(t, buf.String(), "ScopeBetaOnlyInB",
		"find should not match other-repo artifact when scoped to cwd")
}

func TestFind_WithAll_ReturnsOtherRepoArtifact(t *testing.T) {
	repoA, _ := setupTwoIndexedRepos(t)

	origWd := testWorkingDirectory(t)
	os.Chdir(repoA)
	t.Cleanup(func() { os.Chdir(origWd) })

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"ScopeBetaOnlyInB", "--no-refresh", "--all", "--plain"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.Contains(t, buf.String(), "ScopeBetaOnlyInB")
}

func TestFind_WithCurrentRepoQuery_ReturnsCurrentRepoArtifact(t *testing.T) {
	repoA, _ := setupTwoIndexedRepos(t)

	origWd := testWorkingDirectory(t)
	os.Chdir(repoA)
	t.Cleanup(func() { os.Chdir(origWd) })

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"ScopeAlphaOnlyInA", "--no-refresh", "--plain"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.Contains(t, buf.String(), "ScopeAlphaOnlyInA")

}

func TestTodos_HumanOutput_GroupedByArtifact(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewTodosCmd()
	cmd.SetArgs([]string{"--no-refresh", "--tag", "auth"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.True(t, containsStr(out, "DevSpecs Todos"),
		"expected header, got: %s", out)
	assert.True(t, containsStr(out, "Auth Middleware (plan)"),
		"expected grouped artifact header, got: %s", out)
	assert.True(t, containsStr(out, "Implement JWT"),
		"expected todo line, got: %s", out)
	assert.NotContains(t, out, "plans/auth.md:",
		"human output should not include source_file:line")

}

func TestCriteria_HumanOutput_GroupedByArtifact(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	now := time.Now().UTC().Format(time.RFC3339)
	ids := idgen.NewFactory()
	var aid, rev string
	{
		err := db.QueryRow("SELECT a.id, a.current_revision_id FROM artifacts a WHERE a.title = ?", "Auth Middleware").Scan(&aid, &rev)
		require.NoError(t, err)
	}

	db.Exec(`INSERT INTO artifact_criteria (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, criteria_kind, created_at) VALUES (?, ?, ?, 0, 'Gate criterion one', 0, 'auth.md', 10, 'acceptance', ?)`,
		ids.NewWithPrefix("crit_"), aid, rev, now)
	db.Close()

	cmd := NewCriteriaCmd()
	cmd.SetArgs([]string{"--no-refresh", "--tag", "auth"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.True(t, containsStr(out, "DevSpecs Criteria"),
		"expected header: %s", out)
	assert.True(t, containsStr(out, "Auth Middleware (plan)"),
		"expected grouped header: %s", out)
	assert.True(t, containsStr(out, "acceptance"), "expected criterion line: %s", out)
	assert.True(t, containsStr(out, "Gate criterion one"), "expected criterion line: %s", out)
	assert.NotContains(t, out, "auth.md:10",
		"human output should not include source_file:line")

}

func TestTodos_SingleArtifactHumanGrouped(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	var aid string
	{
		err := db.QueryRow("SELECT id FROM artifacts WHERE title = ?", "Auth Middleware").Scan(&aid)
		require.NoError(t, err)
	}

	db.Close()

	cmd := NewTodosCmd()
	cmd.SetArgs([]string{aid, "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Equal(t, 1, strings.Count(out, "Auth Middleware (plan)"),
		"want exactly one grouped artifact header, got %d in: %s", strings.Count(out, "Auth Middleware (plan)"), out)
	assert.True(t, containsStr(out, "Implement JWT"),
		"expected todo line: %s", out)
	assert.NotContains(t, out, "auth.md:",
		"human output should not include source_file:line")

}

func TestCriteria_SingleArtifactHumanGrouped(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	now := time.Now().UTC().Format(time.RFC3339)
	ids := idgen.NewFactory()
	var aid, rev string
	{
		err := db.QueryRow("SELECT a.id, a.current_revision_id FROM artifacts a WHERE a.title = ?", "Auth Middleware").Scan(&aid, &rev)
		require.NoError(t, err)
	}

	db.Exec(`INSERT INTO artifact_criteria (id, artifact_id, revision_id, ordinal, text, done, source_file, source_line, criteria_kind, created_at) VALUES (?, ?, ?, 0, 'Single-ID criterion', 0, 'auth.md', 10, 'acceptance', ?)`,
		ids.NewWithPrefix("crit_"), aid, rev, now)
	db.Close()

	cmd := NewCriteriaCmd()
	cmd.SetArgs([]string{aid, "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Equal(t, 1, strings.Count(out, "Auth Middleware (plan)"),
		"want exactly one grouped artifact header, got %d in: %s", strings.Count(out, "Auth Middleware (plan)"), out)
	assert.True(t, containsStr(out, "acceptance"), "expected criterion line: %s", out)
	assert.True(t, containsStr(out, "Single-ID criterion"), "expected criterion line: %s", out)
	assert.NotContains(t, out, "auth.md:10",
		"human output should not include source_file:line")

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
	require.NoError(t, cmd.Execute())

	output := buf.String()
	assert.True(t, containsStr(output, "Auth Middleware"),
		"find with --tag should return matching artifact")

}

func TestTodos_WithTagFilter(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)

	// Verify the data is correct in the DB before testing the command
	fp := store.FilterParams{Tag: "auth"}
	todos, err := db.ListAllTodos(fp, false, false)
	require.NoError(t, err,
		"direct query failed: %v", err)

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.True(t, containsStr(output, "Implement JWT"),
		"expected todos from auth-tagged artifact, got: %s", output)

}

func TestResume_WithTagFilter(t *testing.T) {
	repoDir, db := setupV01Env(t)
	seedV01Artifacts(t, db, repoDir)
	db.Close()

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--no-refresh", "--tag", "auth"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.True(t, containsStr(output, "Auth Middleware"),
		"resume --tag auth should show Auth Middleware")
	assert.False(t, containsStr(output, "API Spec"),
		"resume --tag auth should not show API Spec")

}

// --- Config Command Tests ---

func TestConfigShow_Defaults(t *testing.T) {
	setupV01Env(t)

	cmd := NewConfigCmd()
	cmd.SetArgs([]string{"show"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.True(t, containsStr(output, "defaults"),
		"expected '(defaults' note when no config file")
	assert.True(t, containsStr(output, "markdown"),
		"expected markdown source in defaults")

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.False(t, containsStr(output, "defaults"),
		"should NOT show defaults note when config file exists")
	assert.True(t, containsStr(output, "version: 2"),
		"expected version: 2 in output")

}

func TestConfigPaths(t *testing.T) {
	setupV01Env(t)

	cmd := NewConfigCmd()
	cmd.SetArgs([]string{"paths"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Regexp(t, `\[(missing|ok)\]`, output)

}

func TestConfigAddSource(t *testing.T) {
	repoDir, _ := setupV01Env(t)

	cmd := NewConfigCmd()
	cmd.SetArgs([]string{"add-source", "markdown", "contracts"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.True(t, containsStr(buf.String(), "Added source"),
		"expected confirmation message")

	cfg, err := config.LoadRepoConfig(repoDir)
	require.NoError(t, err)

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
	assert.True(t, found,
		"contracts path not found in config after add-source")

}

func TestConfigAddSource_Duplicate(t *testing.T) {
	repoDir, _ := setupV01Env(t)

	cfg := config.DefaultRepoConfig()
	config.WriteRepoConfig(repoDir, cfg)

	cmd := NewConfigCmd()
	cmd.SetArgs([]string{"add-source", "openspec", "openspec"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.True(t, containsStr(buf.String(), "already exists"),
		"expected 'already exists' for duplicate")

}

func TestConfigSet(t *testing.T) {
	repoDir, _ := setupV01Env(t)

	cmd := NewConfigCmd()
	cmd.SetArgs([]string{"set", "version", "2"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	cfg, err := config.LoadRepoConfig(repoDir)
	require.NoError(t, err)
	assert.Equal(t, 2, cfg.Version,
		"expected version 2, got %d", cfg.Version)

}

// --- Relative Time Tests ---

func TestRelativeTime_WithThirtySeconds_ReturnsJustNow(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	got := relativeTime(now.Add(-30*time.Second), now)

	assert.Equal(t, "just now", got)
}

func TestRelativeTime_WithFiveMinutes_ReturnsMinuteCount(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	got := relativeTime(now.Add(-5*time.Minute), now)

	assert.Equal(t, "5m ago", got)
}

func TestRelativeTime_WithOneMinute_ReturnsSingularMinute(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	got := relativeTime(now.Add(-time.Minute), now)

	assert.Equal(t, "1 minute ago", got)
}

func TestRelativeTime_WithOneHour_ReturnsSingularHour(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	got := relativeTime(now.Add(-time.Hour), now)

	assert.Equal(t, "1h ago", got)
}

func TestRelativeTime_WithThreeHours_ReturnsHourCount(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	got := relativeTime(now.Add(-3*time.Hour), now)

	assert.Equal(t, "3h ago", got)
}

func TestRelativeTime_WithThirtySixHours_ReturnsYesterday(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	got := relativeTime(now.Add(-36*time.Hour), now)

	assert.Equal(t, "yesterday", got)
}

func TestRelativeTime_WithFiveDays_ReturnsDayCount(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	got := relativeTime(now.Add(-5*24*time.Hour), now)

	assert.Equal(t, "5 days ago", got)
}

func TestRelativeTime_WithFortyFiveDays_ReturnsDayCount(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	got := relativeTime(now.Add(-45*24*time.Hour), now)

	assert.Equal(t, "45 days ago", got)
}

func TestRelativeTime_WithZeroTime_ReturnsUnknown(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	got := relativeTime(time.Time{}, now)

	assert.Equal(t, "unknown", got)
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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.True(t, containsStr(output, "Tags:"),
		"expected Tags: line in show output")
	assert.True(t, containsStr(output, "auth"),
		"expected 'auth' tag in show output")
	assert.True(t, containsStr(output, "security"),
		"expected 'security' tag in show output")

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.True(t, containsStr(output, "Scanned by:"),
		"expected 'Scanned by:' in show output")
	assert.True(t, containsStr(output, "brenn"),
		"expected 'brenn' in scanned by")

}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
