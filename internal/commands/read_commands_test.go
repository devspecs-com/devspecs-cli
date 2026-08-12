package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupReadEnv(t *testing.T) (repoDir string, artifactID string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir = filepath.Join(tmp, "repo")
	os.MkdirAll(repoDir, 0o755)
	os.WriteFile(filepath.Join(repoDir, "plan.md"), []byte("# My Plan\n\n## Tasks\n\n- [ ] Open task\n- [x] Done task\n- [ ] Another open\n\n## Auditable success criteria\n\n- [ ] Gate criterion open\n- [x] Gate criterion done\n"), 0o644)

	origWd := testWorkingDirectory(t)
	os.Chdir(repoDir)
	t.Cleanup(func() { os.Chdir(origWd) })

	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, initCmd.Execute())

	captureCmd := NewCaptureCmd()
	captureCmd.SetArgs([]string{"plan.md", "--kind", "plan"})
	capBuf := &bytes.Buffer{}
	captureCmd.SetOut(capBuf)
	{
		err := captureCmd.Execute()
		require.NoError(t, err)
	}

	for _, line := range strings.Split(capBuf.String(), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ds_") {
			artifactID = trimmed
			break
		}
	}
	assert.NotEqual(t, "", artifactID,
		"failed to extract artifact ID from capture output")

	return
}

func TestList_ShowsArtifacts(t *testing.T) {
	setupReadEnv(t)

	cmd := NewListCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Contains(t, output, "My Plan",
		"list output missing 'My Plan': %s", output)
	assert.Contains(t, output, "plan",
		"list output missing 'plan' kind: %s", output)

}

func TestShow_DisplaysDetail(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewShowCmd()
	cmd.SetArgs([]string{artID})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Contains(t, output, "My Plan",
		"show missing title: %s", output)
	assert.Contains(t, output, artID,
		"show missing ID: %s", output)

}

func TestShow_IncludesTodos(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewShowCmd()
	cmd.SetArgs([]string{artID})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Contains(t, output, "Todos:",
		"show output missing Todos section: %s", output)
	assert.Contains(t, output, "Open task",
		"show output missing todo text: %s", output)
	assert.Contains(t, output, "[ ]",
		"show output missing open marker: %s", output)
	assert.Contains(t, output, "[x]",
		"show output missing done marker: %s", output)

}

func TestFind_ByTitle(t *testing.T) {
	setupReadEnv(t)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"My Plan"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Contains(t, output, "My Plan",
		"find output missing 'My Plan': %s", output)

}

func TestResolve_OutputsIDAndPath(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewResolveCmd()
	cmd.SetArgs([]string{artID})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Contains(t, output, artID,
		"resolve missing artifact ID: %s", output)
	assert.Contains(t, output, "plan.md",
		"resolve missing source path: %s", output)

}

func TestContext_IncludesExtractedTasks(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewContextCmd()
	cmd.SetArgs([]string{artID})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Contains(t, output, "## Extracted Tasks",
		"context missing Extracted Tasks header: %s", output)
	assert.Contains(t, output, "Open task",
		"context missing todo text 'Open task': %s", output)
	assert.Contains(t, output, "- [ ]",
		"context missing open marker: %s", output)
	assert.Contains(t, output, "- [x]",
		"context missing done marker: %s", output)

}

func TestTodos_AllArtifacts(t *testing.T) {
	setupReadEnv(t)

	cmd := NewTodosCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Contains(t, output, "Open task",
		"todos output missing 'Open task': %s", output)
	assert.Contains(t, output, "Done task",
		"todos output missing 'Done task': %s", output)
	assert.Contains(t, output, "Another open",
		"todos output missing 'Another open': %s", output)

}

func TestTodos_ScopedToArtifact(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewTodosCmd()
	cmd.SetArgs([]string{artID})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Contains(t, output, "Open task",
		"scoped todos missing 'Open task': %s", output)

}

func TestTodos_WithOpenFilter_ShowsOnlyOpenTodos(t *testing.T) {
	setupReadEnv(t)

	cmd := NewTodosCmd()
	cmd.SetArgs([]string{"--open"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Open task")
	assert.NotContains(t, buf.String(), "Done task")
}

func TestTodos_WithDoneFilter_ShowsOnlyDoneTodos(t *testing.T) {
	setupReadEnv(t)

	cmd := NewTodosCmd()
	cmd.SetArgs([]string{"--done"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Done task")
	assert.NotContains(t, buf.String(), "Open task")
}

func TestTodos_JSONSchema(t *testing.T) {
	setupReadEnv(t)

	cmd := NewTodosCmd()
	cmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var todos []map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &todos)
		require.NoError(t, err,
			"invalid JSON: %v\n%s", err, buf.String())
	}
	require.NotEmpty(t, todos,
		"expected non-empty todos array")

	require.Len(t, todos[0], 10)
	assert.Contains(t, todos[0], "artifact_id")
	assert.Contains(t, todos[0], "artifact_kind")
	assert.Contains(t, todos[0], "artifact_short_id")
	assert.Contains(t, todos[0], "artifact_title")
	assert.Contains(t, todos[0], "revision_id")
	assert.Contains(t, todos[0], "ordinal")
	assert.Contains(t, todos[0], "text")
	assert.Contains(t, todos[0], "done")
	assert.Contains(t, todos[0], "source_file")
	assert.Contains(t, todos[0], "source_line")
}

func TestCriteria_AllArtifacts(t *testing.T) {
	setupReadEnv(t)

	cmd := NewCriteriaCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Contains(t, out, "Gate criterion open",
		"criteria output missing open criterion: %s", out)
	assert.Contains(t, out, "Gate criterion done",
		"criteria output missing done criterion: %s", out)
	assert.Contains(t, out, "success",
		"criteria output missing kind column success: %s", out)

}

func TestCriteria_JSONSchema(t *testing.T) {
	setupReadEnv(t)

	cmd := NewCriteriaCmd()
	cmd.SetArgs([]string{"--json"})
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
			"invalid JSON: %v\n%s", err, buf.String())
	}
	require.Len(t, rows, 2,
		"want 2 criteria, got %d", len(rows))

	require.Len(t, rows[0], 11)
	assert.Contains(t, rows[0], "artifact_id")
	assert.Contains(t, rows[0], "artifact_kind")
	assert.Contains(t, rows[0], "artifact_short_id")
	assert.Contains(t, rows[0], "artifact_title")
	assert.Contains(t, rows[0], "revision_id")
	assert.Contains(t, rows[0], "ordinal")
	assert.Contains(t, rows[0], "text")
	assert.Contains(t, rows[0], "done")
	assert.Contains(t, rows[0], "source_file")
	assert.Contains(t, rows[0], "source_line")
	assert.Contains(t, rows[0], "criteria_kind")
}

func TestCriteria_WithOpenFilter_ShowsOnlyOpenCriteria(t *testing.T) {
	setupReadEnv(t)

	cmd := NewCriteriaCmd()
	cmd.SetArgs([]string{"--open"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Gate criterion open")
	assert.NotContains(t, buf.String(), "Gate criterion done")
}

func TestCriteria_WithSuccessKindFilter_ShowsSuccessCriteria(t *testing.T) {
	setupReadEnv(t)

	cmd := NewCriteriaCmd()
	cmd.SetArgs([]string{"--kind", "success"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Gate criterion")
}

func TestTodos_NoOutOfScopeFlags(t *testing.T) {
	cmd := NewTodosCmd()

	assert.Nil(t, cmd.Flags().Lookup("owner"))
	assert.Nil(t, cmd.Flags().Lookup("assignee"))
	assert.Nil(t, cmd.Flags().Lookup("due-date"))
	assert.Nil(t, cmd.Flags().Lookup("due_date"))
	assert.Nil(t, cmd.Flags().Lookup("priority"))
	assert.Nil(t, cmd.Flags().Lookup("label"))
	assert.Nil(t, cmd.Flags().Lookup("sprint"))
	assert.Nil(t, cmd.Flags().Lookup("create"))
	assert.Nil(t, cmd.Flags().Lookup("update"))
	assert.Nil(t, cmd.Flags().Lookup("delete"))
}

func TestContext_JSONOutput(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewContextCmd()
	cmd.SetArgs([]string{artID, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var obj map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &obj)
		require.NoError(t, err,
			"context --json invalid: %v", err)
	}
	{

		_, ok := obj["todos"]
		assert.True(t, ok,
			"context JSON missing 'todos' key")
	}
	{

		_, ok := obj["body"]
		assert.True(t, ok,
			"context JSON missing 'body' key")
	}

}

func TestShow_JSONOutput(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewShowCmd()
	cmd.SetArgs([]string{artID, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var obj map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &obj)
		require.NoError(t, err,
			"show --json invalid: %v", err)
	}

	{
		key := "id"

		{
			_, ok := obj[key]
			assert.True(t, ok,
				"show JSON missing key %q", key)
		}

	}
	{
		key := "kind"

		{
			_, ok := obj[key]
			assert.True(t, ok,
				"show JSON missing key %q", key)
		}

	}
	{
		key := "title"

		{
			_, ok := obj[key]
			assert.True(t, ok,
				"show JSON missing key %q", key)
		}

	}
	{
		key := "status"

		{
			_, ok := obj[key]
			assert.True(t, ok,
				"show JSON missing key %q", key)
		}

	}
	{
		key := "todos"

		{
			_, ok := obj[key]
			assert.True(t, ok,
				"show JSON missing key %q", key)
		}

	}

}

func TestResolve_JSONOutput(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewResolveCmd()
	cmd.SetArgs([]string{artID, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var obj map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &obj)
		require.NoError(t, err,
			"resolve --json invalid: %v", err)
	}

	{
		key := "id"

		{
			_, ok := obj[key]
			assert.True(t, ok,
				"resolve JSON missing key %q", key)
		}

	}
	{
		key := "kind"

		{
			_, ok := obj[key]
			assert.True(t, ok,
				"resolve JSON missing key %q", key)
		}

	}
	{
		key := "title"

		{
			_, ok := obj[key]
			assert.True(t, ok,
				"resolve JSON missing key %q", key)
		}

	}
	{
		key := "source_path"

		{
			_, ok := obj[key]
			assert.True(t, ok,
				"resolve JSON missing key %q", key)
		}

	}

}

func TestFind_JSONOutput(t *testing.T) {
	setupReadEnv(t)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"Plan", "--json", "--plain"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var arts []map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &arts)
		require.NoError(t, err,
			"find --json invalid: %v", err)
	}
	require.NotEmpty(t, arts,
		"find --json returned empty array")
	{

		_, ok := arts[0]["reasons"].([]any)
		assert.True(t, ok,
			"find --json missing retrieval reasons: %#v", arts[0])
	}
	assert.Equal(t, "plan.md", arts[0]["source_path"],
		"find --json source_path = %#v", arts[0]["source_path"])
	assert.Equal(t, "eval_weighted_files_v0", arts[0]["retriever"],
		"find --json retriever = %#v", arts[0]["retriever"])

}

func TestFindPack_JSONOutputKeepsRankedResultsAndGroups(t *testing.T) {
	setupReadEnv(t)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"Plan", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out FindPackOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err,
			"find --json default pack invalid: %v\n%s", err, buf.String())
	}
	assert.Equal(t, "role_grouped_pack_v0_family_primary_v1", out.Mode,
		"pack mode = %q", out.Mode)
	assert.Equal(t, "beta", out.ScoutMode,
		"scout mode = %q", out.ScoutMode)
	require.NotEmpty(t, out.Groups,
		"find --json default pack returned no groups: %#v", out)
	assert.NotEqual(t, 0, out.Summary.IncludedCount, "find --json --pack missing summary: %#v", out.Summary)
	assert.NotEqual(t, 0, out.Summary.RoleDiversity, "find --json --pack missing summary: %#v", out.Summary)
	require.NotEmpty(t, out.RankedResults,
		"find --json default pack returned no ranked results: %#v", out)

}

func TestFindPack_HumanOutputShowsReceipt(t *testing.T) {
	setupReadEnv(t)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"Plan"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Contains(t, output, "Working set: Plan",
		"find default pack missing %q:\n%s", "Working set: Plan", output)
	assert.Contains(t, output, "Summary:",
		"find default pack missing %q:\n%s", "Summary:", output)
	assert.Contains(t, output, "Coverage:",
		"find default pack missing %q:\n%s", "Coverage:", output)
	assert.Contains(t, output, "Evidence:",
		"find default pack missing %q:\n%s", "Evidence:", output)

	assert.NotContains(t, output, "Retriever:",
		"find default pack should be concise and omit %q:\n%s", "Retriever:", output)
	assert.NotContains(t, output, "Mode:",
		"find default pack should be concise and omit %q:\n%s", "Mode:", output)
	assert.NotContains(t, output, "Type:",
		"find default pack should be concise and omit %q:\n%s", "Type:", output)
	assert.NotContains(t, output, "Why:",
		"find default pack should be concise and omit %q:\n%s", "Why:", output)
	assert.NotContains(t, output, "Signals:",
		"find default pack should be concise and omit %q:\n%s", "Signals:", output)

}

func TestFindHelpShowsPlainInsteadOfPack(t *testing.T) {
	cmd := NewFindCmd()
	cmd.SetArgs([]string{"--help"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.NotContains(t, output, "--pack",
		"find help should not expose --pack:\n%s", output)
	assert.Contains(t, output, "--plain",
		"find help should expose --plain:\n%s", output)

}

func TestFindPack_HumanOutputShowsGitReceiptsWhenAvailable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	repoDir, _ := setupReadEnv(t)
	runGitForFindPack(t, repoDir, "init")
	runGitForFindPack(t, repoDir, "checkout", "-b", "main")
	runGitForFindPack(t, repoDir, "config", "user.email", "test@example.com")
	runGitForFindPack(t, repoDir, "config", "user.name", "Test User")
	runGitForFindPack(t, repoDir, "add", "plan.md")
	runGitForFindPack(t, repoDir, "commit", "-m", "Plan token refresh work (#42)", "-m", "Connects the plan to auth implementation.")

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"token refresh plan", "--pack", "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Contains(t, output, "Relevant commits (1)",
		"find --pack git receipts missing %q:\n%s", "Relevant commits (1)", output)
	assert.Contains(t, output, "Plan token refresh work (#42)",
		"find --pack git receipts missing %q:\n%s", "Plan token refresh work (#42)", output)
	assert.Contains(t, output, "touched: plan.md",
		"find --pack git receipts missing %q:\n%s", "touched: plan.md", output)
	assert.Contains(t, output, "matched: token, refresh",
		"find --pack git receipts missing %q:\n%s", "matched: token, refresh", output)

}

func TestFindPack_JSONOutputIncludesGitReceiptsWhenAvailable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	repoDir, _ := setupReadEnv(t)
	runGitForFindPack(t, repoDir, "init")
	runGitForFindPack(t, repoDir, "checkout", "-b", "main")
	runGitForFindPack(t, repoDir, "config", "user.email", "test@example.com")
	runGitForFindPack(t, repoDir, "config", "user.name", "Test User")
	runGitForFindPack(t, repoDir, "add", "plan.md")
	runGitForFindPack(t, repoDir, "commit", "-m", "Plan webhook replay work", "-m", "Refs #77")

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"webhook replay plan", "--json", "--pack", "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out FindPackOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err,
			"find --json --pack invalid: %v\n%s", err, buf.String())
	}
	require.NotNil(t, out.GitTrust, "expected one git receipt: %#v", out.GitTrust)
	require.Len(t, out.GitTrust.Receipts, 1, "expected one git receipt: %#v", out.GitTrust)
	assert.NotEqual(t, "", out.GitTrust.Receipts[0].ShortSHA, "unexpected git receipt: %#v", out.GitTrust.Receipts[0])
	assert.Contains(t, out.GitTrust.Receipts[0].Subject, "webhook replay", "unexpected git receipt: %#v", out.GitTrust.Receipts[0])

}

func TestFindGitReceiptScoringSkipsBotNoise(t *testing.T) {
	receipts := scoreFindGitReceipts([]parsedFindGitCommit{
		{
			sha:     "bot",
			subject: "Update dependency @angular/compiler to v21.1.1 (#18717)",
			body:    "Co-authored-by: renovate[bot] <29139614+renovate[bot]@users.noreply.github.com>",
			paths: []string{
				"scripts/release/run.js",
				"scripts/release/steps/index.js",
				"scripts/release/steps/post-publish-steps.js",
			},
		},
		{
			sha:         "human",
			committedAt: "2026-04-15",
			subject:     "Replace main branch in changelog link with tags (#19054)",
			paths:       []string{"scripts/release/steps/show-instructions-after-npm-publish.js"},
		},
	}, []string{
		"scripts/release/run.js",
		"scripts/release/steps/index.js",
		"scripts/release/steps/post-publish-steps.js",
		"scripts/release/steps/show-instructions-after-npm-publish.js",
	}, "release publish npm")
	require.Len(t, receipts, 1,
		"expected one non-noisy receipt, got %#v", receipts)
	assert.Equal(t, "human", receipts[0].SHA,
		"expected human receipt, got %#v", receipts[0])

}

func TestFindPack_VerboseHumanOutputShowsDiagnostics(t *testing.T) {
	setupReadEnv(t)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"Plan", "--pack", "--verbose"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Contains(t, output, "Working set: Plan",
		"find --pack --verbose missing %q:\n%s", "Working set: Plan", output)
	assert.Contains(t, output, "Retriever:",
		"find --pack --verbose missing %q:\n%s", "Retriever:", output)
	assert.Contains(t, output, "Mode: role_grouped_pack_v0",
		"find --pack --verbose missing %q:\n%s", "Mode: role_grouped_pack_v0", output)
	assert.Contains(t, output, "Source:",
		"find --pack --verbose missing %q:\n%s", "Source:", output)
	assert.Contains(t, output, "Type:",
		"find --pack --verbose missing %q:\n%s", "Type:", output)
	assert.Contains(t, output, "Why:",
		"find --pack --verbose missing %q:\n%s", "Why:", output)

}

func runGitForFindPack(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err,
		"git %v failed: %v\n%s", args, err, out)

}

func TestFindPackGraphDiagnostics_HumanOutputShowsRelatedEvidenceSection(t *testing.T) {
	repoDir, _ := setupReadEnv(t)
	seedGraphDiagnosticArtifacts(t, repoDir)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"--pack", "--graph-diagnostics", "--no-refresh", "rotatetoken implementation"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Contains(t, output, "Working set: rotatetoken implementation",
		"pack graph output missing %q:\n%s", "Working set: rotatetoken implementation", output)
	assert.Contains(t, output, "Related via test/source evidence (1)",
		"pack graph output missing %q:\n%s", "Related via test/source evidence (1)", output)
	assert.Contains(t, output, "Evidence: typed_edge_pack_scout_v1",
		"pack graph output missing %q:\n%s", "Evidence: typed_edge_pack_scout_v1", output)
	assert.Contains(t, output, "Behavior tests (1)",
		"pack graph output missing %q:\n%s", "Behavior tests (1)", output)
	assert.Contains(t, output, "Connected from: src/session.ts",
		"pack graph output missing %q:\n%s", "Connected from: src/session.ts", output)
	assert.Contains(t, output, "Evidence: tests_source/source_symbol_match",
		"pack graph output missing %q:\n%s", "Evidence: tests_source/source_symbol_match", output)

	assert.NotContains(t, output, "Graph attachments",
		"pack graph output should use pack presentation section, got:\n%s", output)

}

func TestFindPackGraphDiagnostics_JSONKeepsGraphContextSeparate(t *testing.T) {
	repoDir, _ := setupReadEnv(t)
	seedGraphDiagnosticArtifacts(t, repoDir)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"--json", "--pack", "--graph-diagnostics", "--no-refresh", "rotatetoken implementation"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out FindPackOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err,
			"find --json --pack --graph-diagnostics invalid: %v\n%s", err, buf.String())
	}
	require.NotNil(t, out.GraphDiagnostics,
		"expected raw graph diagnostics in pack JSON: %#v", out)
	require.NotNil(t, out.GraphContext,
		"expected graph context presentation in pack JSON: %#v", out)
	assert.Equal(t, findGraphPackContextMode, out.GraphContext.Mode,
		"graph context mode = %q", out.GraphContext.Mode)
	assert.Equal(t, 1, out.GraphContext.CandidateCount, "expected one grouped graph context candidate: %#v", out.GraphContext)
	require.Len(t, out.GraphContext.Groups, 1, "expected one grouped graph context candidate: %#v", out.GraphContext)

	item := out.GraphContext.Groups[0].Items[0]
	assert.Equal(t, "tests_source", item.AdmissionEdgeType, "unexpected graph context item: %#v", item)
	assert.Equal(t, "src/session.test.ts", item.SourcePath, "unexpected graph context item: %#v", item)

	for _, ranked := range out.RankedResults {
		assert.NotEqual(t, item.ID, ranked.ID,
			"graph context item leaked into ranked results: %#v", out)

	}
}

func TestFindGraphDiagnostics_AttachesTypedEdgeAndSuppressesSharedConcept(t *testing.T) {
	repoDir, _ := setupReadEnv(t)
	seedGraphDiagnosticArtifacts(t, repoDir)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"--json", "--plain", "--graph-diagnostics", "--no-refresh", "rotatetoken implementation"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out FindGraphOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err,
			"find --json --graph-diagnostics invalid: %v\n%s", err, buf.String())
	}
	assert.Equal(t, findGraphDiagnosticsMode, out.Mode,
		"graph mode = %q", out.Mode)
	require.NotEmpty(t, out.RankedResults,
		"expected unchanged ranked results: %#v", out)
	assert.Equal(t, 1, out.GraphDiagnostics.CandidateCount,
		"expected one graph candidate, got %#v", out.GraphDiagnostics)

	got := out.GraphDiagnostics.Candidates[0]
	assert.Equal(t, "src/session.test.ts", got.SourcePath,
		"graph candidate source path = %q, want test attachment; diagnostics=%#v", got.SourcePath, out.GraphDiagnostics)
	assert.Equal(t, "tests_source", got.AdmissionEdgeType,
		"admission edge = %q", got.AdmissionEdgeType)
	assert.Contains(t, got.Receipt, "tests_source connects src/session.test.ts#test_case -> src/session.ts",
		"receipt missing seed and edge evidence: %q", got.Receipt)

	for _, candidate := range out.GraphDiagnostics.Candidates {
		assert.NotEqual(t, "docs/noisy.md", candidate.Path,
			"support-only shared concept admitted graph candidate: %#v", out.GraphDiagnostics)

	}
	assert.NotEqual(t, 0, out.GraphDiagnostics.Counts["suppressed_support_only"],
		"expected shared concept suppression count: %#v", out.GraphDiagnostics)
	assert.Equal(t, 0, out.GraphDiagnostics.Counts["admitted_explicit_reference"],
		"explicit references must stay support-only in graph diagnostics: %#v", out.GraphDiagnostics)

}

func TestFindGraphDiagnostics_RequiresSourceTestQueryIntent(t *testing.T) {
	repoDir, _ := setupReadEnv(t)
	seedGraphDiagnosticArtifacts(t, repoDir)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"--json", "--plain", "--graph-diagnostics", "--no-refresh", "rotatetoken"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out FindGraphOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err,
			"find --json --graph-diagnostics invalid: %v\n%s", err, buf.String())
	}
	require.NotEmpty(t, out.RankedResults,
		"expected unchanged ranked results: %#v", out)
	assert.Equal(t, 0, out.GraphDiagnostics.CandidateCount,
		"expected query-intent gate to suppress graph candidates: %#v", out.GraphDiagnostics)
	assert.NotEqual(t, 0, out.GraphDiagnostics.Counts["suppressed_query_intent"],
		"expected query-intent suppression count: %#v", out.GraphDiagnostics)

}

func TestFind_JSONOutputIncludesLineScopedPath(t *testing.T) {
	repoDir, _ := setupReadEnv(t)
	relPath := seedLineScopedTestArtifacts(t, repoDir)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"--json", "--plain", "--no-refresh", "testputandgetexposedtool"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var arts []map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &arts)
		require.NoError(t, err,
			"find --json invalid: %v", err)
	}
	require.NotEmpty(t, arts,
		"find --json returned empty array")

	wantPath := filepath.ToSlash(relPath) + "#L53"
	assert.Equal(t, wantPath, arts[0]["path"],
		"find --json path = %#v, want %q\nrows=%#v", arts[0]["path"], wantPath, arts)
	assert.Equal(t, filepath.ToSlash(relPath), arts[0]["source_path"],
		"find --json source_path = %#v", arts[0]["source_path"])

}

func TestResume_QueryFocusedContextJSON(t *testing.T) {
	setupReadEnv(t)

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"Open task", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var obj map[string]any
	{
		err := json.Unmarshal(buf.Bytes(), &obj)
		require.NoError(t, err,
			"resume query --json invalid: %v\n%s", err, buf.String())
	}
	assert.Equal(t, "eval_weighted_files_v0", obj["retriever"],
		"retriever = %#v", obj["retriever"])
	assert.Equal(t, "approx_chars_div_4", obj["token_counter"],
		"token counter = %#v", obj["token_counter"])

	arts, ok := obj["artifacts"].([]any)
	require.True(t, ok, "resume query returned no artifacts: %#v", obj["artifacts"])
	require.NotEmpty(t, arts, "resume query returned no artifacts: %#v", obj["artifacts"])

	context, ok := obj["context"].(string)
	require.True(t, ok, "resume query returned invalid context: %#v", obj["context"])
	assert.Contains(t, context, "Open task", "focused context missing expected content: %s", context)
	assert.Contains(t, context, "plan.md", "focused context missing expected content: %s", context)

}

func seedLineScopedTestArtifacts(t *testing.T, repoDir string) string {
	t.Helper()
	db, err := openDB()
	require.NoError(t, err)

	defer db.Close()

	var repoID string
	{
		err := db.QueryRow("SELECT id FROM repos LIMIT 1").Scan(&repoID)
		require.NoError(t, err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	relPath := filepath.ToSlash(filepath.Join("components", "camel-ai", "camel-langchain4j-tools", "src", "test", "java", "org", "apache", "camel", "component", "langchain4j", "tools", "spec", "CamelToolExecutorCacheTest.java"))
	insertTestArtifact(t, db, repoID, "ds_exact_test", "rev_exact_test", "src_exact_test", relPath, 53, 67, "testPutAndGetExposedTool", "Test: testPutAndGetExposedTool\nSource: "+relPath+"\nLines: 53-67\n\ncache.put(\"users\", camelSpec);\ncache.getTools().get(\"users\");", now)
	insertTestArtifact(t, db, repoID, "ds_other_test", "rev_other_test", "src_other_test", relPath, 151, 159, "testHasSearchableTools", "Test: testHasSearchableTools\nSource: "+relPath+"\nLines: 151-159\n\ncache.putSearchable(\"users\", camelSpec);\ncache.hasSearchableTools();", now)
	return relPath
}

func seedGraphDiagnosticArtifacts(t *testing.T, repoDir string) {
	t.Helper()
	db, err := openDB()
	require.NoError(t, err)

	defer db.Close()

	var repoID string
	{
		err := db.QueryRow("SELECT id FROM repos LIMIT 1").Scan(&repoID)
		require.NoError(t, err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	insertGraphArtifact(t, db, repoID, "ds_graph_source", "rev_graph_source", "src_graph_source", "source_context", "", "src/session.ts", "Session implementation", "export function RotateToken() { return 'rotatetoken implementation'; }\n", `{"language":"typescript"}`, now)
	insertGraphArtifact(t, db, repoID, "ds_graph_test", "rev_graph_test", "src_graph_test", "source_context", "test_case", "src/session.test.ts", "Session behavior test", "describe('session behavior', () => { it('covers rotation', () => {}); });\n", `{"mode":"intent","subtype":"test_case","source_type":"test_case","test_name":"session behavior"}`, now)
	insertGraphArtifact(t, db, repoID, "ds_graph_noise", "rev_graph_noise", "src_graph_noise", "plan", "", "docs/noisy.md", "Noisy related doc", "This document shares a generic session concept but is not implementation context.\n", `{}`, now)
	insertGraphArtifact(t, db, repoID, "ds_graph_reference", "rev_graph_reference", "src_graph_reference", "plan", "", "docs/session-reference.md", "Session reference", "This document explicitly references src/session.ts but should not be graph-admitted.\n", `{}`, now)
	{
		err := db.UpsertArtifactEdge(store.ArtifactEdgeInput{
			ID:            "edge_graph_test_source",
			RepoID:        repoID,
			SrcArtifactID: "ds_graph_test",
			DstArtifactID: "ds_graph_source",
			EdgeType:      "tests_source",
			Weight:        0.8,
			Confidence:    0.82,
			EvidenceCount: 1,
			SourceSignal:  "source_symbol_match",
			Explanation:   "test mentions source symbol RotateToken",
		}, now)
		require.NoError(t, err)
	}
	{

		err := db.UpsertArtifactEdge(store.ArtifactEdgeInput{
			ID:            "edge_graph_shared_concept",
			RepoID:        repoID,
			SrcArtifactID: "ds_graph_source",
			DstArtifactID: "ds_graph_noise",
			EdgeType:      "mentions_same_concept",
			Weight:        0.7,
			Confidence:    0.9,
			EvidenceCount: 1,
			SourceSignal:  "shared_rare_concept",
			Explanation:   "shares rare concept session",
		}, now)
		require.NoError(t, err)
	}
	{

		err := db.UpsertArtifactEdge(store.ArtifactEdgeInput{
			ID:            "edge_graph_explicit_reference",
			RepoID:        repoID,
			SrcArtifactID: "ds_graph_reference",
			DstArtifactID: "ds_graph_source",
			EdgeType:      "explicit_reference",
			Weight:        0.9,
			Confidence:    0.9,
			EvidenceCount: 1,
			SourceSignal:  "path_reference",
			Explanation:   "explicit path reference",
		}, now)
		require.NoError(t, err)
	}

}

func insertGraphArtifact(t *testing.T, db *store.DB, repoID, artifactID, revID, sourceID, kind, subtype, relPath, title, body, extracted, now string) {
	t.Helper()
	relPath = filepath.ToSlash(relPath)
	{
		err := db.InsertArtifactDirect(artifactID, repoID, kind, subtype, title, "unknown", revID, now, now)
		require.NoError(t, err)
	}
	{

		err := db.InsertRevisionDirect(revID, artifactID, "sha256:"+artifactID, body, extracted, now)
		require.NoError(t, err)
	}

	sourceType := kind
	if subtype == "test_case" {
		sourceType = "test_case"
	}
	{
		err := db.InsertSourceDirect(sourceID, artifactID, repoID, sourceType, relPath, relPath+"|"+sourceType, "", "", now)
		require.NoError(t, err)
	}
	{

		err := db.IndexArtifactFTS(artifactID, title, body, relPath)
		require.NoError(t, err)
	}

}

func insertTestArtifact(t *testing.T, db *store.DB, repoID, artifactID, revID, sourceID, relPath string, startLine, endLine int, testName, body, now string) {
	t.Helper()
	title := "CamelToolExecutorCacheTest > " + testName
	extracted := `{"mode":"intent","subtype":"test_case","source_type":"test_case","test_name":"` + testName + `","source_line_range":"` + strconv.Itoa(startLine) + `-` + strconv.Itoa(endLine) + `"}`
	{
		err := db.InsertArtifactDirect(artifactID, repoID, "source_context", "test_case", title, "unknown", revID, now, now)
		require.NoError(t, err)
	}
	{

		err := db.InsertRevisionDirect(revID, artifactID, "sha256:"+artifactID, body, extracted, now)
		require.NoError(t, err)
	}

	sourceIdentity := relPath + "|test_case|" + strconv.Itoa(startLine) + "|" + strings.ToLower(testName)
	{
		err := db.InsertSourceDirect(sourceID, artifactID, repoID, "test_case", relPath, sourceIdentity, "", "", now)
		require.NoError(t, err)
	}
	{

		err := db.IndexArtifactFTS(artifactID, title, body, relPath)
		require.NoError(t, err)
	}

}
