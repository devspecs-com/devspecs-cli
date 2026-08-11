package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTaskCommandRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")

	setupTaskCommandRepoFiles(t, repoDir)

	origWd, _ := os.Getwd()
	{
		err := os.Chdir(repoDir)
		require.NoError(t, err)
	}

	t.Cleanup(func() { os.Chdir(origWd) })

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--quiet"})
	scanCmd.SetOut(&bytes.Buffer{})
	{
		err := scanCmd.Execute()
		require.NoErrorf(t, err,
			"scan: %v", err)
	}

	return repoDir
}

func setupTaskCommandRepoFiles(t *testing.T, repoDir string) {
	t.Helper()
	mustMkdirAll(t, filepath.Join(repoDir, ".devspecs"))
	mustWriteFile(t, filepath.Join(repoDir, ".devspecs", "config.yaml"), `version: 1
artifacts:
  test_cases: true
sources:
  - type: markdown
    paths:
      - docs/plans
  - type: source_context
`)
	mustMkdirAll(t, filepath.Join(repoDir, "internal", "retrieval"))
	mustWriteFile(t, filepath.Join(repoDir, "internal", "retrieval", "ranking.go"), `package retrieval

func ImproveTestCompanionRecall(query string) string {
	return "test companion recall " + query
}
`)
	mustWriteFile(t, filepath.Join(repoDir, "internal", "retrieval", "ranking_test.go"), `package retrieval

import "testing"

func TestImproveTestCompanionRecall(t *testing.T) {
	if ImproveTestCompanionRecall"pack" == "" {
		t.Fatal("missing recall")
	}
}
`)
	mustMkdirAll(t, filepath.Join(repoDir, "docs", "plans"))
	mustWriteFile(t, filepath.Join(repoDir, "docs", "plans", "test-companion-recall.md"), `# Test companion recall

## Success Criteria

- [ ] Primary retrieval file is found.
- [ ] Test companion file is found or the miss is recorded.
`)
}

func setupTaskCommandUmbrellaRepo(t *testing.T) (string, string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	umbrella := filepath.Join(tmp, "eag-stg")
	child := filepath.Join(umbrella, "enalytics-backend")
	setupTaskCommandRepoFiles(t, child)
	mustMkdirAll(t, filepath.Join(umbrella, ".git"))
	mustMkdirAll(t, filepath.Join(child, ".git"))
	mustWriteFile(t, filepath.Join(umbrella, "AGENTS.md"), "# eag-stg\n")
	mustWriteFile(t, filepath.Join(umbrella, "CLAUDE.md"), "# eag-stg\n")

	origWd, _ := os.Getwd()
	{
		err := os.Chdir(umbrella)
		require.NoError(t, err)
	}

	t.Cleanup(func() { os.Chdir(origWd) })
	return umbrella, child
}

func taskGitCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Env = cleanTaskGitTestEnv()
	return cmd
}

func initTaskGitRepo(t *testing.T, repoDir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	{
		err := taskGitCmd("init", "-b", "main", repoDir).Run()
		require.NoError(t, err)
	}

}

func commitTaskGitRepo(t *testing.T, repoDir, message string) {
	t.Helper()
	{
		err := taskGitCmd("-C", repoDir, "add", ".").Run()
		require.NoError(t, err)
	}
	{

		err := taskGitCmd("-C", repoDir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", message).Run()
		require.NoError(t, err)
	}

}

func cleanTaskGitTestEnv() []string {
	blocked := map[string]bool{
		"GIT_DIR":                          true,
		"GIT_WORK_TREE":                    true,
		"GIT_INDEX_FILE":                   true,
		"GIT_PREFIX":                       true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	}
	var env []string
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok && blocked[key] {
			continue
		}
		env = append(env, entry)
	}
	return env
}

func TestTask_StartCreatesUncertaintyAwareWorkspace(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "spike-test", "--no-refresh", "--json", "improve test companion recall"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out taskStartOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, buf.String())
	}
	assert.Equalf(t, "spike-test", out.TaskID,
		"task id = %q", out.TaskID)
	assert.Truef(t, strings.HasPrefix(out.Workspace, filepath.Join(repoDir, "devspecs", "tasks", "spike-test")),
		"workspace = %q", out.Workspace)
	require.Lenf(t, out.Slices, 1,
		"expected one default slice, got %#v", out.Slices)
	assert.Equalf(t, "A01-improve-test-companion-recall-plan.md", filepath.Base(out.FirstSlicePath),
		"first slice path = %q", out.FirstSlicePath)
	assert.Equalf(t, "A01-improve-test-companion-recall-result.md", filepath.Base(out.ResultPath),
		"result path = %q", out.ResultPath)
	{
		_, err := os.Stat(out.IndexPath)
		require.NoErrorf(t,
			err, "expected %s: %v",

			out.IndexPath, err)
	}
	{
		_, err := os.Stat(out.FirstSlicePath)
		require.NoErrorf(t, err,
			"expected %s: %v",
			out.FirstSlicePath, err)
	}
	{
		_, err := os.Stat(out.ResultPath)
		require.NoErrorf(t,
			err, "expected %s: %v",

			out.ResultPath, err)
	}
	{
		_, err := os.Stat(out.ManifestPath)
		require.NoErrorf(t, err, "expected %s: %v",

			out.ManifestPath, err)
	}

	indexBody := mustReadFile(t, out.IndexPath)
	assert.Containsf(t, indexBody,
		"## Known Knowns", "A00 missing %q:\n%s",

		"## Known Knowns", indexBody)
	assert.Containsf(t, indexBody,
		"## Known Unknowns", "A00 missing %q:\n%s",

		"## Known Unknowns", indexBody)
	assert.Containsf(t, indexBody,
		"## Confidence Summary", "A00 missing %q:\n%s",

		"## Confidence Summary", indexBody)
	assert.Containsf(t, indexBody,
		"## Task Slices",
		"A00 missing %q:\n%s",

		"## Task Slices", indexBody)
	assert.Containsf(t, indexBody,
		"## Agent Preflight Checklist", "A00 missing %q:\n%s",

		"## Agent Preflight Checklist", indexBody,
	)
	assert.Containsf(t, indexBody,
		"A01-improve-test-companion-recall-plan.md", "A00 missing %q:\n%s", "A01-improve-test-companion-recall-plan.md", indexBody)
	assert.Containsf(t, indexBody,
		"Pack completeness", "A00 missing %q:\n%s",

		"Pack completeness", indexBody)

	firstSlice := mustReadFile(t, out.FirstSlicePath)
	assert.Containsf(t, firstSlice,

		"## Goal", "A01 missing %q:\n%s",

		"## Goal", firstSlice)
	assert.Containsf(t, firstSlice,

		"## Description", "A01 missing %q:\n%s",

		"## Description", firstSlice)
	assert.Containsf(t, firstSlice,

		"## Resources",
		"A01 missing %q:\n%s",

		"## Resources", firstSlice)
	assert.Containsf(t, firstSlice,

		"## Success Criteria", "A01 missing %q:\n%s",

		"## Success Criteria", firstSlice)
	assert.Containsf(t, firstSlice,

		"## Tasks", "A01 missing %q:\n%s",

		"## Tasks", firstSlice)
	assert.Containsf(t, firstSlice,

		"## Decision Gates", "A01 missing %q:\n%s",

		"## Decision Gates", firstSlice)
	assert.Containsf(t, firstSlice,

		"Block: external input", "A01 missing %q:\n%s",

		"Block: external input", firstSlice)

	resultTemplate := mustReadFile(t, out.ResultPath)
	assertNoTrailingWhitespace(t, "A01 result template", resultTemplate)
	assert.Containsf(t, resultTemplate,

		"## Summary", "A01-1 missing %q:\n%s",

		"## Summary", resultTemplate)
	assert.Containsf(t, resultTemplate,

		"## Completion Contract", "A01-1 missing %q:\n%s",

		"## Completion Contract", resultTemplate,
	)
	assert.Containsf(t, resultTemplate,

		"Attempted slice: `A01`", "A01-1 missing %q:\n%s",

		"Attempted slice: `A01`", resultTemplate,
	)
	assert.Containsf(t, resultTemplate,

		"Gate tested: promote, improve, rework, rollback, or block", "A01-1 missing %q:\n%s", "Gate tested: promote, improve, rework, rollback, or block",
		resultTemplate)
	assert.Containsf(t, resultTemplate,

		"## Changed Files", "A01-1 missing %q:\n%s",

		"## Changed Files", resultTemplate)
	assert.Containsf(t, resultTemplate,

		"## Tests",
		"A01-1 missing %q:\n%s",

		"## Tests", resultTemplate)
	assert.Containsf(t, resultTemplate,

		"## Decision", "A01-1 missing %q:\n%s",

		"## Decision", resultTemplate)
	assert.Containsf(t, resultTemplate,

		"## Follow-up", "A01-1 missing %q:\n%s",

		"## Follow-up", resultTemplate)
	assert.Containsf(t, resultTemplate,

		"## Checkpoints", "A01-1 missing %q:\n%s",

		"## Checkpoints", resultTemplate)
	assert.Containsf(t, resultTemplate,

		"ds task checkpoint spike-test --target A01", "A01-1 missing %q:\n%s", "ds task checkpoint spike-test --target A01", resultTemplate)

	manifest := mustReadFile(t, out.ManifestPath)
	assert.Contains(t, manifest, `"predicted_context"`)
	assert.Contains(t, manifest, `"confidence"`)
	assert.Contains(t, manifest, `"slices"`)
	assert.Contains(t, manifest, `"A01-improve-test-companion-recall-plan.md"`)

	assert.Truef(t, containsPath(out.PrimaryFiles, "internal/retrieval/ranking.go"),
		"task preflight missing primary source from shared pack assembly: %#v", out.PrimaryFiles)
	assert.Truef(t, containsPath(out.TestFiles, "internal/retrieval/ranking_test.go"),
		"task preflight missing test companion from shared pack assembly: %#v", out.TestFiles)

	db, err := openDB()
	require.NoError(t, err)

	defer db.Close()
	artifacts, err := db.ListArtifacts(store.FilterParams{RepoRoot: repoDir, SourceType: "capture"})
	require.NoError(t, err)
	assert.GreaterOrEqualf(t, len(artifacts), 2,
		"expected A00/A01 to be captured, got %d", len(artifacts))

}

func TestTaskStatusJSONShowsFirstPendingTarget(t *testing.T) {
	setupTaskWithStatusSlices(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"status", "status-next-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskStatusOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "A01", out.NextTarget)
	assert.Equal(t, "first status slice", out.NextTitle)
	assert.Equal(t, "ds apply status-next-test", out.NextCommand)
}

func TestTaskStatusHumanShowsFirstPendingTarget(t *testing.T) {
	setupTaskWithStatusSlices(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"status", "status-next-test"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Next: A01 - first status slice")
	assert.Contains(t, buf.String(), "Run: ds apply status-next-test")
}

func TestTaskStatusShowsSecondTargetAfterFirstIsPromoted(t *testing.T) {
	setupTaskWithStatusSlices(t)
	decideTaskTarget(t, "status-next-test", "A01", "promote")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"status", "status-next-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskStatusOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "A02", out.NextTarget)
	assert.Equal(t, "second status slice", out.NextTitle)
}

func setupTaskWithStatusSlices(t *testing.T) {
	t.Helper()
	setupTaskCommandRepo(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "status-next-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "first status slice",
		"--slice", "second status slice",
		"status next workflow",
	})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func decideTaskTarget(t *testing.T, taskID string, target string, decision string) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"decide", taskID,
		"--target", target,
		"--decision", decision,
		"--index=false",
		"--json",
	})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func TestTaskRepoFlagRoutesCreatedArtifactsToChildRepo(t *testing.T) {
	umbrella, child, childWorkspace, out := setupRepoRoutedTask(t)

	assert.True(t, strings.HasPrefix(out.Workspace, childWorkspace))
	require.FileExists(t, filepath.Join(childWorkspace, taskManifestFilename))
	assert.NoDirExists(t, filepath.Join(umbrella, "devspecs", "tasks", "repo-route-smoke"))
	var manifest taskManifest
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest))
	assert.Equal(t, canonicalRepoRoot(child), manifest.RepoRoot)
}

func TestTaskShowRepoFlagResolvesChildTask(t *testing.T) {
	_, _, childWorkspace, _ := setupRepoRoutedTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"show", "repo-route-smoke", "--repo", "./enalytics-backend", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskTargetOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "repo-route-smoke", out.TaskID)
	assert.Equal(t, "A01", out.Target)
	assert.True(t, strings.HasPrefix(out.Workspace, childWorkspace))
}

func TestTaskPromptRepoFlagIncludesRepoAwareCheckpointCommand(t *testing.T) {
	setupRepoRoutedTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"prompt", "repo-route-smoke", "--repo", "./enalytics-backend", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskPromptOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Contains(t, out.Prompt, "ds task checkpoint repo-route-smoke --target A01 --repo ./enalytics-backend")
}

func TestTaskCheckpointRepoFlagWritesCheckpointToChildTask(t *testing.T) {
	_, _, childWorkspace, _ := setupRepoRoutedTask(t)
	startRepoRoutedTarget(t)
	cmd := newRepoRoutedCheckpointCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskCheckpointOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.True(t, strings.HasPrefix(out.CheckpointPath, filepath.Join(childWorkspace, "checkpoints")))
}

func TestTaskStatusRepoFlagShowsChildCheckpointDecision(t *testing.T) {
	setupRepoRoutedTask(t)
	startRepoRoutedTarget(t)
	checkpointRepoRoutedTarget(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"status", "repo-route-smoke", "--repo", "./enalytics-backend", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskStatusOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	require.Len(t, out.Slices, 1)
	assert.Equal(t, "promote", out.Slices[0].Decision)
}

func setupRepoRoutedTask(t *testing.T) (string, string, string, taskStartOutput) {
	t.Helper()
	umbrella, child := setupTaskCommandUmbrellaRepo(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--repo", "./enalytics-backend",
		"--id", "repo-route-smoke",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "backend repo slice",
		"backend workspace change",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	require.NoError(t, cmd.Execute())
	var out taskStartOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	return umbrella, child, filepath.Join(child, "devspecs", "tasks", "repo-route-smoke"), out
}

func startRepoRoutedTarget(t *testing.T) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"start", "repo-route-smoke", "--target", "A01", "--repo", "./enalytics-backend", "--index=false", "--json"})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func newRepoRoutedCheckpointCmd() *cobra.Command {
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "repo-route-smoke",
		"--target", "A01",
		"--repo", "./enalytics-backend",
		"--stage", "validated",
		"--decision", "promote",
		"--file-read", "internal/retrieval/ranking.go",
		"--index=false",
		"--json",
	})
	return cmd
}

func checkpointRepoRoutedTarget(t *testing.T) {
	t.Helper()
	cmd := newRepoRoutedCheckpointCmd()
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func TestTaskWithoutRepoFromUmbrellaUsesCurrentRoot(t *testing.T) {
	umbrella, child := setupTaskCommandUmbrellaRepo(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "umbrella-local-task",
		"--no-refresh",
		"--index=false",
		"--json",
		"umbrella local task",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out taskStartOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, buf.String())
	}
	assert.Truef(t, strings.HasPrefix(out.Workspace, filepath.Join(umbrella, "devspecs", "tasks", "umbrella-local-task")),
		"workspace = %q, want under umbrella root %q", out.Workspace, umbrella)
	{

		_, err := os.Stat(filepath.Join(umbrella, "devspecs", "tasks", "umbrella-local-task", taskManifestFilename))
		require.NoErrorf(t, err,
			"umbrella manifest missing: %v", err)
	}
	{

		_, err := os.Stat(filepath.Join(child, "devspecs", "tasks", "umbrella-local-task"))
		assert.Truef(t, os.IsNotExist(err),
			"task unexpectedly wrote under child without --repo: %v", err)
	}

}

func TestTaskRepoFlagDirRouting(t *testing.T) {
	_, child := setupTaskCommandUmbrellaRepo(t)

	relativeCmd := NewTaskCmd()
	relativeCmd.SetArgs([]string{
		"--repo", "./enalytics-backend",
		"--dir", "custom/tasks",
		"--id", "relative-dir-task",
		"--no-refresh",
		"--index=false",
		"--json",
		"relative dir task",
	})
	relativeCmd.SetOut(&bytes.Buffer{})
	{
		err := relativeCmd.Execute()
		require.NoError(t, err)
	}
	{

		_, err := os.Stat(filepath.Join(child, "custom", "tasks", "relative-dir-task", taskManifestFilename))
		require.NoErrorf(t, err,
			"relative dir manifest missing under child: %v", err)
	}

	absoluteParent := filepath.Join(t.TempDir(), "absolute-task-parent")
	absoluteCmd := NewTaskCmd()
	absoluteCmd.SetArgs([]string{
		"--repo", "./enalytics-backend",
		"--dir", absoluteParent,
		"--id", "absolute-dir-task",
		"--no-refresh",
		"--index=false",
		"--json",
		"absolute dir task",
	})
	absoluteCmd.SetOut(&bytes.Buffer{})
	{
		err := absoluteCmd.Execute()
		require.NoError(t, err)
	}
	{

		_, err := os.Stat(filepath.Join(absoluteParent, "absolute-dir-task", taskManifestFilename))
		require.NoErrorf(t, err,
			"absolute dir manifest missing: %v", err)
	}
	{

		_, err := os.Stat(filepath.Join(child, "devspecs", "tasks", "absolute-dir-task"))
		assert.Truef(t, os.IsNotExist(err),
			"absolute dir task unexpectedly wrote under child default dir: %v", err)
	}

}

func TestTask_QuickCreatesOneOffWorkspaceWithCompactOutput(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--quick", "--id", "quick-fix", "--no-refresh", "--index=false", "fix small billing typo"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	output := buf.String()
	assert.Containsf(t, output,
		"Created one-off task: quick-fix", "quick output missing %q:\n%s",

		"Created one-off task: quick-fix", output)
	assert.Containsf(t, output,
		"Target: A01", "quick output missing %q:\n%s",

		"Target: A01", output)
	assert.Containsf(t, output,
		"Next:", "quick output missing %q:\n%s",

		"Next:", output)
	assert.Containsf(t, output,
		"ds apply quick-fix --target A01", "quick output missing %q:\n%s",

		"ds apply quick-fix --target A01", output)
	assert.Containsf(t, output,
		"ds task checkpoint quick-fix --target A01", "quick output missing %q:\n%s", "ds task checkpoint quick-fix --target A01", output)

	workspace := filepath.Join(repoDir, "devspecs", "tasks", "quick-fix")
	{
		_, err := os.Stat(filepath.Join(workspace, taskManifestFilename))
		require.NoErrorf(t, err,
			"quick task manifest missing: %v", err)
	}
	{

		_, err := os.Stat(filepath.Join(workspace, "A01-fix-small-billing-typo-result.md"))
		require.NoErrorf(t, err,
			"quick result missing: %v", err)
	}

}

func TestTaskHelpHidesQuickSubcommandAndTeachesQuickFlag(t *testing.T) {
	setupTaskCommandRepo(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--help"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "\n  quick       ")
	assert.Contains(t, buf.String(), "--quick")
}

func TestTaskQuickCompatibilitySubcommandCreatesOneOffTask(t *testing.T) {
	setupTaskCommandRepo(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"quick", "--id", "quick-compat", "--no-refresh", "--index=false", "fix small billing typo"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Created one-off task: quick-compat")
}

func TestTaskHelpHidesLegacyLifecycleCommandsAndShowsSupportedCommands(t *testing.T) {
	setupTaskCommandRepo(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--help"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "\n  decide      ")
	assert.NotContains(t, buf.String(), "\n  finish      ")
	assert.NotContains(t, buf.String(), "\n  prompt      ")
	assert.NotContains(t, buf.String(), "\n  start       ")
	assert.NotContains(t, buf.String(), "\n  sync        ")
	assert.Contains(t, buf.String(), "\n  checkpoint  ")
	assert.Contains(t, buf.String(), "\n  refresh     ")
	assert.Contains(t, buf.String(), "\n  status      ")
	assert.Contains(t, buf.String(), "\n  next        ")
}

func TestTaskPromptCompatibilityHelpPointsToApply(t *testing.T) {
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"prompt", "--help"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Prefer `ds apply <task-id>`")
}

func TestTaskFinishCompatibilityHelpPointsToCheckpoint(t *testing.T) {
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"finish", "--help"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Prefer `ds task checkpoint <task-id> --target")
}

func TestTaskDecideCompatibilityHelpPointsToCheckpoint(t *testing.T) {
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"decide", "--help"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Prefer `ds task checkpoint <task-id> --target")
}

func TestTaskStartCompatibilityHelpPointsToCheckpoint(t *testing.T) {
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"start", "--help"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Prefer `ds task checkpoint <task-id> --target")
}

func TestTaskSyncCompatibilityHelpPointsToRefresh(t *testing.T) {
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"sync", "--help"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Prefer `ds task refresh <task-id>`")
}

func TestTaskHelpHidesIterationCompatibilitySubcommand(t *testing.T) {
	setupTaskCommandRepo(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--help"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "\n  iteration   ")
}

func TestTaskIterationAddCompatibilityHelpPointsToSliceAdd(t *testing.T) {
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"iteration", "add", "--help"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Prefer `ds task slice add")
}

func TestTaskIterationAddCompatibilityCreatesIteration(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "iteration-compat",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "first iteration slice",
		"iteration compatibility",
	})
	startCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, startCmd.Execute())

	compatCmd := NewTaskCmd()
	compatCmd.SetArgs([]string{
		"iteration", "add", "iteration-compat", "repair iteration slice",
		"--slice", "A01",
		"--reason", "improve",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	compatCmd.SetOut(buf)

	err := compatCmd.Execute()

	require.NoError(t, err)
	var out taskArtifactAddOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "A01-1", out.Slice.ID)
	var manifest taskManifest
	manifestPath := filepath.Join(repoDir, "devspecs", "tasks", "iteration-compat", taskManifestFilename)
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(t, manifestPath)), &manifest))
	require.Len(t, manifest.Artifacts.Slices, 2)
	assert.Equal(t, "A01-1", manifest.Artifacts.Slices[1].ID)
	assert.Equal(t, "iteration", manifest.Artifacts.Slices[1].Kind)
	assert.Equal(t, "A01", manifest.Artifacts.Slices[1].ParentID)
	assert.Equal(t, "improve", manifest.Artifacts.Slices[1].Reason)
}

func TestTask_StartGreenfieldProfileUsesPlanningTemplate(t *testing.T) {
	setupTaskCommandRepo(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "greenfield-test",
		"--profile", "greenfield",
		"--no-refresh",
		"--index=false",
		"--json",
		"plan claims zone provider adapters",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out taskStartOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, buf.String())
	}
	assert.Equalf(t, taskProfileGreenfield, out.Profile,
		"profile = %q", out.Profile)

	var manifest taskManifest
	{
		err := json.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest)
		require.NoErrorf(t, err,
			"manifest json: %v", err)
	}
	assert.Equalf(t, taskProfileGreenfield, manifest.Profile,
		"manifest profile = %q", manifest.Profile)

	indexBody := mustReadFile(t, out.IndexPath)
	assert.Containsf(t, indexBody,
		"## Profile", "greenfield index missing %q:\n%s",

		"## Profile", indexBody)
	assert.Containsf(t, indexBody,
		"greenfield", "greenfield index missing %q:\n%s",

		"greenfield", indexBody)
	assert.Containsf(t, indexBody,
		"Treat predicted files as evidence, not required edit targets.", "greenfield index missing %q:\n%s",
		"Treat predicted files as evidence, not required edit targets.", indexBody)
	assert.Containsf(t, indexBody,
		"before implementation scope expands",

		"greenfield index missing %q:\n%s", "before implementation scope expands", indexBody)

	planBody := mustReadFile(t, out.FirstSlicePath)
	assert.Containsf(t, planBody,
		"bounded planning slice", "greenfield plan missing %q:\n%s",

		"bounded planning slice", planBody)
	assert.Containsf(t, planBody,
		"Test or Evaluation Signals",
		"greenfield plan missing %q:\n%s",

		"Test or Evaluation Signals", planBody,
	)
	assert.Containsf(t, planBody,
		"Planning artifacts, acceptance checks, interface notes, eval cards, or test design.", "greenfield plan missing %q:\n%s",
		"Planning artifacts, acceptance checks, interface notes, eval cards, or test design.", planBody)
	assert.Containsf(t, planBody,
		"Draft the smallest useful planning artifact", "greenfield plan missing %q:\n%s", "Draft the smallest useful planning artifact", planBody)
	assert.NotContainsf(t,
		planBody,
		"Inspect the predicted primary files.", "greenfield plan contains code-change boilerplate %q:\n%s",
		"Inspect the predicted primary files.", planBody)
	assert.NotContainsf(t,
		planBody,
		"Implement the smallest useful change.", "greenfield plan contains code-change boilerplate %q:\n%s",
		"Implement the smallest useful change.", planBody,
	)
	assert.NotContainsf(t,
		planBody,
		"Primary implementation surface is verified before edits.", "greenfield plan contains code-change boilerplate %q:\n%s",
		"Primary implementation surface is verified before edits.", planBody,
	)

}

func TestTask_StartSurfacesCheckpointFactRiskCards(t *testing.T) {
	seedRiskCardCheckpointFact(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "risk-card-test", "--no-refresh", "--index=false", "--json", "improve test companion recall"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out taskStartOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, buf.String())
	}
	require.NotNilf(t, taskRiskCardByID(out.RiskCards,
		"prior-test-miss"),

		"missing risk card %q: %#v", "prior-test-miss", out.RiskCards,
	)
	require.NotNilf(t, taskRiskCardByID(out.RiskCards,
		"prior-noise"), "missing risk card %q: %#v",

		"prior-noise", out.RiskCards)
	require.NotNilf(t, taskRiskCardByID(out.RiskCards,
		"validation-gap"),

		"missing risk card %q: %#v", "validation-gap", out.RiskCards,
	)

	testMiss := taskRiskCardByID(out.RiskCards, "prior-test-miss")
	require.NotNil(t, testMiss)
	assert.Contains(t, strings.Join(testMiss.Evidence, "\n"), "internal/retrieval/ranking_test.go")

	var manifest taskManifest
	{
		err := json.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest)
		require.NoErrorf(t, err,
			"manifest json: %v", err)
	}
	require.NotNilf(t, taskRiskCardByID(manifest.RiskCards, "prior-test-miss"),
		"manifest missing risk cards: %#v", manifest.RiskCards)

	indexBody := mustReadFile(t, out.IndexPath)
	assert.Containsf(t, indexBody,
		"## Risk Cards",
		"index missing risk card %q:\n%s",

		"## Risk Cards", indexBody)
	assert.Containsf(t, indexBody,
		"Prior checkpoint missed a related test", "index missing risk card %q:\n%s", "Prior checkpoint missed a related test", indexBody)
	assert.Containsf(t, indexBody,
		"Search same-package and same-stem tests before editing.", "index missing risk card %q:\n%s", "Search same-package and same-stem tests before editing.",
		indexBody)

	planBody := mustReadFile(t, out.FirstSlicePath)
	assert.Containsf(t, planBody, "Prior checkpoint missed a related test",
		"plan missing risk card:\n%s", planBody)

}

func TestTaskPromptSurfacesCheckpointFactRiskCardsBeforeTargetPlan(t *testing.T) {
	seedRiskCardCheckpointFact(t)
	startTaskForRiskCards(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"prompt", "risk-card-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskPromptOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Contains(t, out.Prompt, "Risk cards:")
	assert.Contains(t, out.Prompt, "Treat these as evidence-backed checks, not required edit targets.")
	assert.Contains(t, out.Prompt, "Prior checkpoint missed a related test")
	assert.LessOrEqual(t, strings.Index(out.Prompt, "Risk cards:"), strings.Index(out.Prompt, "Target plan:"))
}

func seedRiskCardCheckpointFact(t *testing.T) {
	t.Helper()
	repoDir := setupTaskCommandRepo(t)
	db, err := openDB()
	require.NoError(t, err)
	repoID := taskTestRepoID(t, db, repoDir)
	now := "2026-06-07T00:00:00Z"
	require.NoError(t, db.UpsertTaskCheckpointFact(store.TaskCheckpointFact{
		RepoID:             repoID,
		TaskID:             "prior-risk-task",
		CheckpointID:       "cp_prior",
		Target:             "A01",
		Series:             "A",
		Stage:              "implemented",
		Decision:           "improve",
		CheckpointPath:     "checkpoints/prior.md",
		CheckpointJSONPath: "checkpoints/prior.json",
		CreatedAt:          now,
		ActualContextJSON:  `{}`,
		FeedbackJSON:       `{"critical_missed":["internal/retrieval/ranking_test.go"],"distracting_included":["fixtures/noisy-plan.md"]}`,
		EvidenceJSON:       `{}`,
		LearningsJSON:      `[{"learning_type":"validation_gap","summary":"focused retrieval validation was missing","evidence_refs":["internal/retrieval/ranking_test.go"],"applies_to":"internal/retrieval","confidence":"high"}]`,
		NextJSON:           `{}`,
		IndexedAt:          now,
	}))
	require.NoError(t, db.Close())
}

func startTaskForRiskCards(t *testing.T) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "risk-card-test", "--no-refresh", "--index=false", "--json", "improve test companion recall"})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func TestTask_StartSurfacesAdvisoryFilesFromCheckpointFacts(t *testing.T) {
	seedAdvisoryCheckpointFact(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "advisory-file-test", "--no-refresh", "--index=false", "--json", "fix discount rounding"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out taskStartOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, buf.String())
	}
	require.NotNilf(t, taskAdvisoryFileByPath(out.AdvisoryFiles,

		"internal/invoice/pricing.go"), "missing advisory file %q: %#v", "internal/invoice/pricing.go", out.AdvisoryFiles)
	require.NotNilf(t, taskAdvisoryFileByPath(out.AdvisoryFiles,

		"internal/invoice/pricing_test.go"), "missing advisory file %q: %#v",
		"internal/invoice/pricing_test.go", out.AdvisoryFiles,
	)
	require.NotNilf(t, taskAdvisoryFileByPath(out.AdvisoryFiles,

		"docs/legacy/discount-rounding-notes.md"), "missing advisory file %q: %#v",
		"docs/legacy/discount-rounding-notes.md", out.AdvisoryFiles)

	{
		file := taskAdvisoryFileByPath(out.AdvisoryFiles, "internal/invoice/pricing_test.go")
		require.NotNil(t, file)
		assert.Equal(t, "prior-missed-test", file.Kind)

	}

	var manifest taskManifest
	{
		err := json.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest)
		require.NoErrorf(t, err,
			"manifest json: %v", err)
	}
	require.NotNilf(t, taskAdvisoryFileByPath(manifest.AdvisoryFiles, "internal/invoice/pricing.go"),
		"manifest missing advisory files: %#v", manifest.AdvisoryFiles)

	planBody := mustReadFile(t, out.FirstSlicePath)
	assert.Containsf(t, planBody,
		"Checkpoint Leads", "plan missing advisory text %q:\n%s",

		"Checkpoint Leads", planBody)
	assert.Containsf(t, planBody,
		"not files the initial pack ranked as primary", "plan missing advisory text %q:\n%s", "not files the initial pack ranked as primary", planBody)
	assert.Containsf(t, planBody,
		"No pack-ranked primary file. Verify these checkpoint leads", "plan missing advisory text %q:\n%s",
		"No pack-ranked primary file. Verify these checkpoint leads", planBody)
	assert.Containsf(t, planBody,
		"internal/invoice/pricing.go", "plan missing advisory text %q:\n%s",

		"internal/invoice/pricing.go", planBody)
	assert.Containsf(t, planBody,
		"internal/invoice/pricing_test.go", "plan missing advisory text %q:\n%s",

		"internal/invoice/pricing_test.go", planBody)

}

func TestTaskPromptSurfacesAdvisoryFilesAsVerificationLeads(t *testing.T) {
	seedAdvisoryCheckpointFact(t)
	startTaskForAdvisoryFiles(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"prompt", "advisory-file-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskPromptOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Contains(t, out.Prompt, "Checkpoint leads:")
	assert.Contains(t, out.Prompt, "verification leads only")
	assert.Contains(t, out.Prompt, "internal/invoice/pricing.go")
	assert.Contains(t, out.Prompt, "internal/invoice/pricing_test.go")
}

func seedAdvisoryCheckpointFact(t *testing.T) {
	t.Helper()
	repoDir := setupTaskCommandRepo(t)
	db, err := openDB()
	require.NoError(t, err)
	repoID := taskTestRepoID(t, db, repoDir)
	now := "2026-06-07T00:00:00Z"
	require.NoError(t, db.UpsertTaskCheckpointFact(store.TaskCheckpointFact{
		RepoID:             repoID,
		TaskID:             "prior-discount-task",
		CheckpointID:       "cp_discount",
		Target:             "P01",
		Series:             "P",
		Stage:              "implemented",
		Decision:           "improve",
		CheckpointPath:     "checkpoints/prior.md",
		CheckpointJSONPath: "checkpoints/prior.json",
		CreatedAt:          now,
		ActualContextJSON:  `{"files_read":["internal/invoice/pricing.go"],"files_edited":["internal/invoice/pricing.go"]}`,
		FeedbackJSON:       `{"critical_missed":["internal/invoice/pricing_test.go"],"distracting_included":["docs/legacy/discount-rounding-notes.md"]}`,
		EvidenceJSON:       `{}`,
		LearningsJSON:      `[{"learning_type":"validation_gap","summary":"discount rounding needed an explicit package test","evidence_refs":["internal/invoice/pricing_test.go"],"applies_to":"internal/invoice","confidence":"high"}]`,
		NextJSON:           `{}`,
		IndexedAt:          now,
	}))
	require.NoError(t, db.Close())
}

func startTaskForAdvisoryFiles(t *testing.T) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "advisory-file-test", "--no-refresh", "--index=false", "--json", "fix discount rounding"})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func TestTask_AdvisoryFilesAreCappedAndPrioritized(t *testing.T) {
	facts := []store.TaskCheckpointFact{{
		TaskID:            "prior-wide-task",
		CheckpointID:      "cp_wide",
		ActualContextJSON: `{"files_read":["src/a.go","src/b.go"],"files_edited":["src/c.go"],"tests_read":["src/a_test.go"]}`,
		FeedbackJSON:      `{"critical_missed":["src/missed_test.go","src/missed.go"],"distracting_included":["docs/noise-a.md","docs/noise-b.md"]}`,
		LearningsJSON:     `[{"learning_type":"validation_gap","summary":"discount rounding needed package tests","evidence_refs":["src/learned_test.go","src/learned.go"],"applies_to":"src","confidence":"high"}]`,
	}}
	files := taskAdvisoryFilesFromCheckpointFacts("fix discount rounding", taskPredictedContext{}, facts)
	assert.LessOrEqualf(t, len(files), taskAdvisoryFileLimit,
		"advisory files exceed cap: %d > %d: %#v", len(files), taskAdvisoryFileLimit, files)

	require.Len(t, files, 5)
	assert.Equal(t, "prior-source", files[0].Kind)
	assert.Equal(t, "prior-missed-test", files[1].Kind)
	assert.Equal(t, "prior-noise", files[2].Kind)
	assert.Equal(t, "prior-missed-file", files[3].Kind)
	assert.Equal(t, "prior-test-evidence", files[4].Kind)
}

func TestTaskAdvisoryFilesAreSuppressedByStrongPredictedContext(t *testing.T) {
	facts := []store.TaskCheckpointFact{{
		TaskID:            "prior-wide-task",
		CheckpointID:      "cp_wide",
		ActualContextJSON: `{"files_read":["src/a.go"],"files_edited":["src/c.go"]}`,
		FeedbackJSON:      `{"critical_missed":["src/missed_test.go"]}`,
	}}
	strongPredicted := taskPredictedContext{
		PrimaryFiles: []taskPredictedFile{{Path: "src/main.go"}},
		Tests:        []taskPredictedFile{{Path: "src/main_test.go"}},
	}

	actual := taskAdvisoryFilesFromCheckpointFacts("fix discount rounding", strongPredicted, facts)

	assert.Empty(t, actual)
}

func TestTask_PromptCarriesPriorSliceCheckpointEvidence(t *testing.T) {
	setupTaskCommandRepo(t)
	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "prior-slice-evidence-test",
		"--no-refresh",
		"--index=false",
		"--slice", "trace test companion recall",
		"--slice", "wire test companion recall",
		"improve test companion recall",
	})
	startCmd.SetOut(&bytes.Buffer{})
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "prior-slice-evidence-test",
		"--slice", "A01",
		"--stage", "validated",
		"--decision", "promote",
		"--file-read", "internal/retrieval/ranking.go",
		"--test-read", "internal/retrieval/ranking_test.go",
		"--missed-file", "internal/retrieval/ranking_test.go",
		"--noise-file", ".devspecs/tasks/prior-slice-evidence-test/A00-index.md",
		"--learning", "context_gap|trace found the companion test|high|A01|internal/retrieval/ranking_test.go",
		"--json",
	})
	checkpointCmd.SetOut(&bytes.Buffer{})
	{
		err := checkpointCmd.Execute()
		require.NoError(t, err)
	}

	promptCmd := NewTaskCmd()
	promptCmd.SetArgs([]string{"prompt", "prior-slice-evidence-test", "--target", "A02", "--json"})
	promptBuf := &bytes.Buffer{}
	promptCmd.SetOut(promptBuf)
	{
		err := promptCmd.Execute()
		require.NoError(t, err)
	}

	var promptOut taskPromptOutput
	{
		err := json.Unmarshal(promptBuf.Bytes(), &promptOut)
		require.NoErrorf(t, err,
			"prompt json: %v\n%s", err, promptBuf.String())
	}
	assert.Containsf(t, promptOut.Prompt,
		"Prior slice evidence:", "prompt missing prior evidence %q:\n%s",

		"Prior slice evidence:",
		promptOut.Prompt)
	assert.Containsf(t, promptOut.Prompt,
		"checkpointed by earlier targets", "prompt missing prior evidence %q:\n%s", "checkpointed by earlier targets", promptOut.Prompt)
	assert.Containsf(t, promptOut.Prompt,
		"internal/retrieval/ranking.go",

		"prompt missing prior evidence %q:\n%s", "internal/retrieval/ranking.go", promptOut.Prompt)
	assert.Containsf(t, promptOut.Prompt,
		"internal/retrieval/ranking_test.go", "prompt missing prior evidence %q:\n%s", "internal/retrieval/ranking_test.go", promptOut.Prompt)

	require.NotNilf(t, taskAdvisoryFileByPath(promptOut.PriorSliceEvidence, "internal/retrieval/ranking_test.go"),
		"prompt json missing prior test evidence: %#v", promptOut.PriorSliceEvidence)
	assert.Nilf(t, taskAdvisoryFileByPath(promptOut.PriorSliceEvidence, ".devspecs/tasks/prior-slice-evidence-test/A00-index.md"),
		"prompt evidence should filter task workspace paths: %#v", promptOut.PriorSliceEvidence)

}

func TestTask_PreflightFiltersTaskWorkspaceCandidates(t *testing.T) {
	got := filterTaskPreflightCandidates([]retrieval.Candidate{
		{Path: ".devspecs/tasks/task-one/A00-index.md"},
		{Path: "devspecs/tasks/task-one/A00-index.md"},
		{Path: "internal/retrieval/ranking.go"},
		{Path: "C:/repo/.devspecs/tasks/task-one/A01-plan.md"},
		{Path: "C:/repo/devspecs/tasks/task-one/A01-plan.md"},
	})
	require.Len(t, got, 1)
	assert.Equal(t, "internal/retrieval/ranking.go", got[0].Path)

}

func TestTaskStatusAutoDetectsLegacyWorkspace(t *testing.T) {
	setupLegacyTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"status", "legacy-compat", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskStatusOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "legacy-compat", out.TaskID)
	require.Len(t, out.Slices, 1)
}

func TestTaskShowAutoDetectsLegacyWorkspaceByTarget(t *testing.T) {
	setupLegacyTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"show", "A01", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskTargetOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "legacy-compat", out.TaskID)
	assert.Equal(t, "A01", out.Target)
	assert.Contains(t, filepath.ToSlash(out.Workspace), ".devspecs/tasks/legacy-compat")
}

func TestTaskCheckpointAutoDetectsLegacyWorkspace(t *testing.T) {
	setupLegacyTask(t)
	cmd := newLegacyCheckpointCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskCheckpointOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Contains(t, filepath.ToSlash(out.CheckpointPath), ".devspecs/tasks/legacy-compat")
}

func TestTaskEvaluateAutoDetectsLegacyWorkspace(t *testing.T) {
	repoDir := setupLegacyTask(t)
	checkpointLegacyTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"evaluate", "legacy-compat", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskEvaluationOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "legacy-compat", out.TaskID)
	require.FileExists(t, filepath.Join(repoDir, ".devspecs", "tasks", "legacy-compat", taskManifestFilename))
}

func setupLegacyTask(t *testing.T) string {
	t.Helper()
	repoDir := setupTaskCommandRepo(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--dir", ".devspecs/tasks",
		"--id", "legacy-compat",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "legacy first slice",
		"legacy task compatibility",
	})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
	return repoDir
}

func newLegacyCheckpointCmd() *cobra.Command {
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "legacy-compat",
		"--slice", "A01",
		"--stage", "validated",
		"--decision", "promote",
		"--file-read", "internal/retrieval/ranking.go",
		"--index=false",
		"--json",
	})
	return cmd
}

func checkpointLegacyTask(t *testing.T) {
	t.Helper()
	cmd := newLegacyCheckpointCmd()
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func TestTask_StartSkipsUnrelatedCheckpointFactRiskCards(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	db, err := openDB()
	require.NoError(t, err)

	defer db.Close()
	repoID := taskTestRepoID(t, db, repoDir)
	now := "2026-06-07T00:00:00Z"
	{
		err := db.UpsertTaskCheckpointFact(store.TaskCheckpointFact{
			RepoID:             repoID,
			TaskID:             "prior-unrelated-task",
			CheckpointID:       "cp_unrelated",
			Target:             "A01",
			Series:             "A",
			Stage:              "implemented",
			Decision:           "improve",
			CheckpointPath:     "checkpoints/unrelated.md",
			CheckpointJSONPath: "checkpoints/unrelated.json",
			CreatedAt:          now,
			ActualContextJSON:  `{}`,
			FeedbackJSON:       `{"critical_missed":["services/billing/webhook_test.go"]}`,
			EvidenceJSON:       `{}`,
			LearningsJSON:      `[]`,
			NextJSON:           `{}`,
			IndexedAt:          now,
		})
		require.NoError(t, err)
	}

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "unrelated-risk-card-test", "--no-refresh", "--index=false", "--json", "improve test companion recall"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out taskStartOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, buf.String())
	}
	assert.Nil(t, taskRiskCardByID(out.RiskCards, "prior-test-miss"))
	assert.Nil(t, taskRiskCardByID(out.RiskCards, "prior-critical-miss"))

}

func TestTask_RiskCardsUseQueryMatchedLearningWhenPredictedContextWeak(t *testing.T) {
	cards := taskRiskCardsFromCheckpointFacts("fix discount rounding", taskPredictedContext{}, []store.TaskCheckpointFact{{
		TaskID:        "prior-discount-task",
		CheckpointID:  "cp_discount",
		FeedbackJSON:  `{"critical_missed":["internal/invoice/pricing_test.go"]}`,
		LearningsJSON: `[{"learning_type":"validation_gap","summary":"discount rounding needed an explicit package test","evidence_refs":["internal/invoice/pricing_test.go"],"applies_to":"internal/invoice","confidence":"high"}]`,
	}})
	require.NotNilf(t, taskRiskCardByID(cards, "prior-test-miss"),
		"expected query-matched prior-test-miss card, got %#v", cards)
	require.NotNilf(t, taskRiskCardByID(cards, "validation-gap"),
		"expected validation-gap card, got %#v", cards)

	unrelated := taskRiskCardsFromCheckpointFacts("fix discount rounding", taskPredictedContext{}, []store.TaskCheckpointFact{{
		TaskID:        "prior-billing-task",
		CheckpointID:  "cp_billing",
		FeedbackJSON:  `{"critical_missed":["services/billing/webhook_test.go"]}`,
		LearningsJSON: `[{"learning_type":"validation_gap","summary":"webhook retries needed a package test","evidence_refs":["services/billing/webhook_test.go"],"applies_to":"services/billing","confidence":"high"}]`,
	}})
	assert.Nilf(t, taskRiskCardByID(unrelated, "prior-test-miss"),
		"unrelated query learning should not create prior-test-miss: %#v", unrelated)

}

func TestTask_StartBootstrapsRepeatedSlices(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "multi-slice-test",
		"--no-refresh",
		"--json",
		"--slice", "scout current workflow",
		"--slice", "tighten checkpoint evidence",
		"task workflow ux",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out taskStartOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, buf.String())
	}
	require.Lenf(t, out.Slices, 2,
		"expected two slices, got %#v", out.Slices)

	assert.Equal(t, "A01-scout-current-workflow-plan.md", filepath.Base(out.Slices[0].PlanPath))
	assert.Equal(t, "A01-scout-current-workflow-result.md", filepath.Base(out.Slices[0].ResultPath))
	assert.Equal(t, "A02-tighten-checkpoint-evidence-plan.md", filepath.Base(out.Slices[1].PlanPath))
	assert.Equal(t, "A02-tighten-checkpoint-evidence-result.md", filepath.Base(out.Slices[1].ResultPath))

	_, err := os.Stat(out.Slices[0].PlanPath)
	require.NoError(t, err)
	_, err = os.Stat(out.Slices[0].ResultPath)
	require.NoError(t, err)
	_, err = os.Stat(out.Slices[1].PlanPath)
	require.NoError(t, err)
	_, err = os.Stat(out.Slices[1].ResultPath)
	require.NoError(t, err)

	assert.Equal(t, "A01-scout-current-workflow-plan.md", filepath.Base(out.FirstSlicePath))
	assert.Equal(t, "A01-scout-current-workflow-result.md", filepath.Base(out.ResultPath))

	var manifest taskManifest
	{
		err := json.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest)
		require.NoErrorf(t, err,
			"manifest json: %v", err)
	}
	require.Lenf(t, manifest.Artifacts.Slices, 2,
		"manifest slices = %#v", manifest.Artifacts.Slices)
	assert.Equal(t, "A01-scout-current-workflow-plan.md", manifest.Artifacts.FirstSlice)
	assert.Equal(t, "A01-scout-current-workflow-result.md", manifest.Artifacts.Result)

	indexBody := mustReadFile(t, out.IndexPath)
	assert.Containsf(t, indexBody,
		"## Task Slices",
		"index missing %q:\n%s",

		"## Task Slices", indexBody)
	assert.Containsf(t, indexBody,
		"A01: scout current workflow", "index missing %q:\n%s",

		"A01: scout current workflow", indexBody,
	)
	assert.Containsf(t, indexBody,
		"A02: tighten checkpoint evidence", "index missing %q:\n%s",

		"A02: tighten checkpoint evidence",
		indexBody)

	assert.Truef(t, strings.HasPrefix(out.Workspace, filepath.Join(repoDir, "devspecs", "tasks", "multi-slice-test")),
		"workspace = %q", out.Workspace)

}

func TestTask_StartWhenPreflightFails_DoesNotCreateWorkspace(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	workspace := filepath.Join(repoDir, "devspecs", "tasks", "preflight-failure")
	t.Setenv("DEVSPECS_TASK_PACK_SCOUT_MODE", "invalid")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "preflight-failure",
		"--no-refresh",
		"--index=false",
		"fail before publishing artifacts",
	})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()
	_, statErr := os.Stat(workspace)

	require.Error(t, err)
	assert.ErrorContains(t, err, "unknown task pack scout mode")
	assert.True(t, os.IsNotExist(statErr), "workspace should not exist after preflight failure: %v", statErr)
}

func TestTask_StartWithSixSlices_PublishesEveryArtifactAndReportsIdentity(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	workspace := filepath.Join(repoDir, "devspecs", "tasks", "six-slice-task")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "six-slice-task",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "first boundary",
		"--slice", "second boundary",
		"--slice", "third boundary",
		"--slice", "fourth boundary",
		"--slice", "fifth boundary",
		"--slice", "sixth boundary",
		"publish a durable six-slice task",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()
	var out taskStartOutput
	unmarshalErr := json.Unmarshal(buf.Bytes(), &out)
	entries, readErr := os.ReadDir(workspace)

	require.NoError(t, err)
	require.NoError(t, unmarshalErr)
	require.NoError(t, readErr)
	assert.Equal(t, "six-slice-task", out.TaskID)
	assert.Equal(t, workspace, out.Workspace)
	require.Len(t, out.Slices, 6)
	assert.Equal(t, "A01", out.Slices[0].ID)
	assert.Equal(t, "A02", out.Slices[1].ID)
	assert.Equal(t, "A03", out.Slices[2].ID)
	assert.Equal(t, "A04", out.Slices[3].ID)
	assert.Equal(t, "A05", out.Slices[4].ID)
	assert.Equal(t, "A06", out.Slices[5].ID)
	require.Len(t, entries, 14)
	assertTaskArtifactExists(t, out.IndexPath)
	assertTaskArtifactExists(t, out.ManifestPath)
	assertTaskArtifactExists(t, out.Slices[0].PlanPath)
	assertTaskArtifactExists(t, out.Slices[0].ResultPath)
	assertTaskArtifactExists(t, out.Slices[1].PlanPath)
	assertTaskArtifactExists(t, out.Slices[1].ResultPath)
	assertTaskArtifactExists(t, out.Slices[2].PlanPath)
	assertTaskArtifactExists(t, out.Slices[2].ResultPath)
	assertTaskArtifactExists(t, out.Slices[3].PlanPath)
	assertTaskArtifactExists(t, out.Slices[3].ResultPath)
	assertTaskArtifactExists(t, out.Slices[4].PlanPath)
	assertTaskArtifactExists(t, out.Slices[4].ResultPath)
	assertTaskArtifactExists(t, out.Slices[5].PlanPath)
	assertTaskArtifactExists(t, out.Slices[5].ResultPath)
}

func TestTask_StartWithForce_ReplacesEmptyWorkspaceWithCompleteArtifacts(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	workspace := filepath.Join(repoDir, "devspecs", "tasks", "empty-workspace")
	mustMkdirAll(t, workspace)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "empty-workspace",
		"--force",
		"--no-refresh",
		"--index=false",
		"--json",
		"replace an empty workspace",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()
	var out taskStartOutput
	unmarshalErr := json.Unmarshal(buf.Bytes(), &out)
	entries, readErr := os.ReadDir(workspace)

	require.NoError(t, err)
	require.NoError(t, unmarshalErr)
	require.NoError(t, readErr)
	assert.Equal(t, "empty-workspace", out.TaskID)
	assert.Equal(t, workspace, out.Workspace)
	require.Len(t, entries, 4)
	assertTaskArtifactExists(t, out.IndexPath)
	assertTaskArtifactExists(t, out.FirstSlicePath)
	assertTaskArtifactExists(t, out.ResultPath)
	assertTaskArtifactExists(t, out.ManifestPath)
}

func TestTask_StartWhenHumanOutputFails_ReturnsErrorWithDurableWorkspaceIdentity(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	workspace := filepath.Join(repoDir, "devspecs", "tasks", "output-failure")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "output-failure",
		"--no-refresh",
		"--index=false",
		"create before reporting success",
	})
	cmd.SetOut(taskFailingWriter{})

	err := cmd.Execute()
	entries, readErr := os.ReadDir(workspace)

	require.Error(t, err)
	assert.ErrorContains(t, err, "task output-failure was created")
	assert.ErrorContains(t, err, workspace)
	assert.ErrorIs(t, err, errTaskOutputClosed)
	require.NoError(t, readErr)
	require.Len(t, entries, 4)
}

func TestTask_StartWhenJSONOutputIsShort_ReturnsErrorWithDurableWorkspaceIdentity(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	workspace := filepath.Join(repoDir, "devspecs", "tasks", "short-json-output")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "short-json-output",
		"--no-refresh",
		"--index=false",
		"--json",
		"create before reporting JSON success",
	})
	cmd.SetOut(taskShortWriter{})

	err := cmd.Execute()
	entries, readErr := os.ReadDir(workspace)

	require.Error(t, err)
	assert.ErrorContains(t, err, "task short-json-output was created")
	assert.ErrorContains(t, err, workspace)
	assert.ErrorIs(t, err, io.ErrShortWrite)
	require.NoError(t, readErr)
	require.Len(t, entries, 4)
}

func TestPublishTaskWorkspace_WhenArtifactPathEscapesStaging_ReturnsErrorWithoutFinalWorkspace(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "tasks", "invalid-artifact")
	files := map[string][]byte{
		"../outside.md": []byte("must not escape"),
	}

	err := publishTaskWorkspace(t.Context(), workspace, false, files)
	_, statErr := os.Stat(workspace)

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid task artifact path")
	assert.True(t, os.IsNotExist(statErr), "workspace should not exist after staged write failure: %v", statErr)
}

var errTaskOutputClosed = errors.New("task output is closed")

type taskFailingWriter struct{}

func (taskFailingWriter) Write([]byte) (int, error) {
	return 0, errTaskOutputClosed
}

type taskShortWriter struct{}

func (taskShortWriter) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}

func assertTaskArtifactExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
	assert.Positive(t, info.Size())
}

func TestTaskNextResolvesFirstPendingBoundary(t *testing.T) {
	setupBoundaryTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"next", "boundary-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskTargetOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "A01", out.Target)
	assert.True(t, containsString(out.SiblingTargets, "A02"))
}

func TestTaskPromptBoundsWorkToResolvedTarget(t *testing.T) {
	setupBoundaryTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"prompt", "boundary-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskPromptOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Contains(t, out.Prompt, "target A01 only")
	assert.Contains(t, out.Prompt, "must_not_implement")
	assert.Contains(t, out.Prompt, "- A02")
	assert.Contains(t, out.Prompt, "Do not implement sibling slices")
	assert.Contains(t, out.Prompt, "Checklist edits are useful notes")
	assert.Contains(t, out.Prompt, "Completion contract:")
	assert.Contains(t, out.Prompt, "Evidence for decision")
	assert.Contains(t, out.Prompt, "Next iteration")
}

func TestTaskStartDefaultsToFirstPendingBoundary(t *testing.T) {
	setupBoundaryTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"start", "boundary-test", "--index=false", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskDecideOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "A01", out.Target)
	assert.Equal(t, "started", out.Stage)
	assert.Equal(t, "continue", out.Decision)
}

func TestTaskFinishCompletesFirstBoundaryWithoutRewritingIndex(t *testing.T) {
	repoDir := setupBoundaryTask(t)
	startBoundaryTarget(t)
	indexPath := filepath.Join(repoDir, "devspecs", "tasks", "boundary-test", "A00-index.md")
	authoredIndexBody := mustReadFile(t, indexPath) + "\n## Human Master Notes\n\nKeep this richer A00 content intact.\n"
	mustWriteFile(t, indexPath, authoredIndexBody)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"finish", "boundary-test", "--decision", "promote", "--index=false", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskDecideOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "A01", out.Target)
	assert.Equal(t, "completed", out.Stage)
	assert.Equal(t, "promote", out.Decision)
	assert.Equal(t, authoredIndexBody, mustReadFile(t, indexPath))
}

func TestTaskNextAdvancesAfterFirstBoundaryFinishes(t *testing.T) {
	setupBoundaryTask(t)
	startBoundaryTarget(t)
	finishBoundaryTarget(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"next", "boundary-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskTargetOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "A02", out.Target)
}

func TestTaskShowResolvesExplicitBoundary(t *testing.T) {
	setupBoundaryTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"show", "boundary-test", "--target", "A02", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskTargetOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "A02", out.Target)
	assert.Contains(t, out.PlanBody, "second bounded slice")
}

func setupBoundaryTask(t *testing.T) string {
	t.Helper()
	repoDir := setupTaskCommandRepo(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "boundary-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "first bounded slice",
		"--slice", "second bounded slice",
		"task boundary primitives",
	})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
	return repoDir
}

func startBoundaryTarget(t *testing.T) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"start", "boundary-test", "--index=false", "--json"})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func finishBoundaryTarget(t *testing.T) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"finish", "boundary-test", "--decision", "promote", "--index=false", "--json"})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func TestTaskShowResolvesUniqueSliceTarget(t *testing.T) {
	setupTargetAddressTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"show", "A02", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskTargetOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "target-address-test", out.TaskID)
	assert.Equal(t, "A02", out.Target)
	assert.True(t, containsString(out.SiblingTargets, "A01"))
}

func TestTaskPromptResolvesUniqueSliceTarget(t *testing.T) {
	setupTargetAddressTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"prompt", "A02", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskPromptOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Contains(t, out.Prompt, "task target-address-test target A02 only")
	assert.Contains(t, out.Prompt, "Checklist edits are useful notes")
	assert.Contains(t, out.Prompt, "Completion contract:")
}

func TestTaskStartResolvesUniqueSliceTarget(t *testing.T) {
	setupTargetAddressTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"start", "A02", "--index=false", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskDecideOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "target-address-test", out.TaskID)
	assert.Equal(t, "A02", out.Target)
	assert.Equal(t, "started", out.Stage)
}

func TestTaskFinishResolvesUniqueSliceTarget(t *testing.T) {
	setupTargetAddressTask(t)
	startTargetAddressSlice(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"finish", "A02", "--decision", "promote", "--index=false", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskDecideOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "target-address-test", out.TaskID)
	assert.Equal(t, "A02", out.Target)
	assert.Equal(t, "promote", out.Decision)
}

func setupTargetAddressTask(t *testing.T) {
	t.Helper()
	setupTaskCommandRepo(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "target-address-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "first target slice",
		"--slice", "second target slice",
		"task target addressing",
	})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func startTargetAddressSlice(t *testing.T) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"start", "A02", "--index=false", "--json"})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func TestTask_TargetAddressingRequiresUnambiguousSlice(t *testing.T) {
	setupTaskCommandRepo(t)
	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{"--id",
		"ambiguous-target-a", "--series", "A", "--no-refresh", "--index=false",
		"--json", "--slice", "shared first slice", "task target ambiguity",
	})
	startCmd.SetOut(&bytes.Buffer{})
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}
	secondStartCmd := NewTaskCmd()
	secondStartCmd.SetArgs([]string{"--id",
		"ambiguous-target-b", "--series", "A", "--no-refresh", "--index=false",
		"--json", "--slice", "shared first slice", "task target ambiguity",
	})
	secondStartCmd.SetOut(&bytes.Buffer{})
	{
		err := secondStartCmd.Execute()
		require.NoError(t, err)
	}

	showCmd := NewTaskCmd()
	showCmd.SetArgs([]string{"show", "A01", "--json"})
	showCmd.SetOut(&bytes.Buffer{})
	showCmd.SetErr(&bytes.Buffer{})
	err := showCmd.Execute()
	require.Error(t, err,
		"expected ambiguous target error")
	assert.Containsf(t, err.Error(),
		"ambiguous task target",
		"ambiguous error missing %q: %v",

		"ambiguous task target", err)
	assert.Containsf(t, err.Error(),
		"ambiguous-target-a:A01",
		"ambiguous error missing %q: %v",

		"ambiguous-target-a:A01", err)
	assert.Containsf(t, err.Error(),
		"ambiguous-target-b:A01",
		"ambiguous error missing %q: %v",

		"ambiguous-target-b:A01", err)
	assert.Containsf(t, err.Error(),
		"use a task id with --target", "ambiguous error missing %q: %v",

		"use a task id with --target",
		err)

}

func TestNextTaskAlphaSeriesFromEmptyReturnsA(t *testing.T) {
	actual := nextTaskAlphaSeries("")

	assert.Equal(t, "A", actual)
}

func TestNextTaskAlphaSeriesFromAReturnsB(t *testing.T) {
	actual := nextTaskAlphaSeries("A")

	assert.Equal(t, "B", actual)
}

func TestNextTaskAlphaSeriesFromYReturnsZ(t *testing.T) {
	actual := nextTaskAlphaSeries("Y")

	assert.Equal(t, "Z", actual)
}

func TestNextTaskAlphaSeriesFromZReturnsAA(t *testing.T) {
	actual := nextTaskAlphaSeries("Z")

	assert.Equal(t, "AA", actual)
}

func TestNextTaskAlphaSeriesFromAAReturnsAB(t *testing.T) {
	actual := nextTaskAlphaSeries("AA")

	assert.Equal(t, "AB", actual)
}

func TestNextTaskAlphaSeriesFromAZReturnsBA(t *testing.T) {
	actual := nextTaskAlphaSeries("AZ")

	assert.Equal(t, "BA", actual)
}

func TestNextTaskAlphaSeriesFromZZReturnsAAA(t *testing.T) {
	actual := nextTaskAlphaSeries("ZZ")

	assert.Equal(t, "AAA", actual)
}

func TestNextTaskAlphaSeriesFromAAAReturnsAAB(t *testing.T) {
	actual := nextTaskAlphaSeries("AAA")

	assert.Equal(t, "AAB", actual)
}

func TestTask_StartAutoIncrementsDefaultSeries(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	writeExistingTaskSeries(t, repoDir, "existing-a", "A")
	writeExistingTaskSeries(t, repoDir, "explicit-r09", "R09")

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "auto-series-b",
		"--no-refresh",
		"--index=false",
		"--json",
		"auto series chooses next alpha",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out taskStartOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, buf.String())
	}
	assert.Equalf(t, "B", out.Series,
		"series = %q", out.Series)
	assert.Equalf(t, "B00-index.md", filepath.Base(out.IndexPath),
		"index path = %q", out.IndexPath)
	require.Len(t, out.Slices, 1)
	assert.Equal(t, "B01", out.Slices[0].ID)

}

func TestTask_StartAutoSeriesSeesLegacyWorkspace(t *testing.T) {
	setupTaskCommandRepo(t)

	legacyCmd := NewTaskCmd()
	legacyCmd.SetArgs([]string{
		"--dir", ".devspecs/tasks",
		"--id", "legacy-series-a",
		"--series", "A",
		"--no-refresh",
		"--index=false",
		"legacy series a",
	})
	legacyCmd.SetOut(&bytes.Buffer{})
	{
		err := legacyCmd.Execute()
		require.NoError(t, err)
	}

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "visible-series-b",
		"--no-refresh",
		"--index=false",
		"--json",
		"visible series should skip legacy a",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out taskStartOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, buf.String())
	}
	assert.Equal(t, "B", out.Series)
	assert.Equal(t, "B00-index.md", filepath.Base(out.IndexPath))

}

func TestTaskStartAutoSeriesRollsFromZToAA(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	writeExistingTaskSeriesRange(t, repoDir, "Z")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "auto-series-aa",
		"--no-refresh",
		"--index=false",
		"--json",
		"auto series rolls past z",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskStartOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "AA", out.Series)
	assert.Equal(t, "AA00-index.md", filepath.Base(out.IndexPath))
	require.Len(t, out.Slices, 1)
	assert.Equal(t, "AA01", out.Slices[0].ID)
}

func TestTaskStartAutoSeriesRollsFromZZToAAA(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	writeExistingTaskSeriesRange(t, repoDir, "ZZ")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "auto-series-aaa",
		"--no-refresh",
		"--index=false",
		"--json",
		"auto series rolls past zz",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskStartOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "AAA", out.Series)
	assert.Equal(t, "AAA00-index.md", filepath.Base(out.IndexPath))
	require.Len(t, out.Slices, 1)
	assert.Equal(t, "AAA01", out.Slices[0].ID)
}

func TestTask_StartAutoRefreshesTaskSubstrate(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "task-substrate-refresh-test", "--json", "improve test companion recall"})
	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	db, err := openDB()
	require.NoError(t, err)

	defer db.Close()
	repoID := taskTestRepoID(t, db, repoDir)
	counts, err := db.CountSourceManifest(repoID)
	require.NoError(t, err)
	assert.NotEqualf(t, 0, counts.Files,
		"expected task auto-refresh to populate source manifest, got %#v", counts)

	var testCases int
	{
		err := db.QueryRow("SELECT COUNT(DISTINCT artifact_id) FROM sources WHERE repo_id = ? AND source_type = 'test_case'", repoID).Scan(&testCases)
		require.NoError(t, err)
	}
	assert.NotEqual(t, 0, testCases,
		"expected task auto-refresh to index test cases")

}

func TestTask_StartGeneratesRequestedSeriesArtifacts(t *testing.T) {
	setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "b-series-test",
		"--series", "b",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "define lifecycle model",
		"--slice", "repair checkpoint state",
		"task workflow ux",
	})
	buf := &bytes.Buffer{}
	startCmd.SetOut(buf)
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}

	var out taskStartOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, buf.String())
	}
	assert.Equalf(t, "B", out.Series,
		"series = %q", out.Series)
	assert.Equalf(t, "B00-index.md", filepath.Base(out.IndexPath),
		"index path = %q", out.IndexPath)

	require.Len(t, out.Slices, 2)
	assert.Equal(t, "B01", out.Slices[0].ID)
	assert.Equal(t, "B01-define-lifecycle-model-plan.md", filepath.Base(out.Slices[0].PlanPath))
	assert.Equal(t, "B02", out.Slices[1].ID)
	assert.Equal(t, "B02-repair-checkpoint-state-plan.md", filepath.Base(out.Slices[1].PlanPath))

	var manifest taskManifest
	{
		err := json.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest)
		require.NoErrorf(t, err,
			"manifest json: %v", err)
	}
	assert.Equal(t, "B", manifest.Series)
	assert.Equal(t, "B", manifest.Artifacts.Series)
	assert.Equal(t, "B00-index.md", manifest.Artifacts.Index)
	assert.Equal(t, "B01-define-lifecycle-model-plan.md", manifest.Artifacts.FirstSlice)

	indexBody := mustReadFile(t, out.IndexPath)
	assert.Containsf(t, indexBody,
		"## Series", "B00 missing %q:\n%s",

		"## Series", indexBody)
	assert.Containsf(t, indexBody,
		"B", "B00 missing %q:\n%s",

		"B", indexBody,
	)
	assert.Containsf(t, indexBody,
		"B01: define lifecycle model", "B00 missing %q:\n%s",

		"B01: define lifecycle model", indexBody)
	assert.Containsf(t, indexBody,
		"B02: repair checkpoint state", "B00 missing %q:\n%s",

		"B02: repair checkpoint state", indexBody,
	)

	planBody := mustReadFile(t, out.Slices[0].PlanPath)
	assert.Containsf(t, planBody, "`B00-index.md`",
		"B01 plan should reference B00 index:\n%s", planBody)
}

func TestTaskCheckpointPreservesRequestedSeriesMetadata(t *testing.T) {
	setupBSeriesTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "b-series-test",
		"--slice", "B02",
		"--stage", "validated",
		"--decision", "complete",
		"--note", "B-series checkpoint",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskCheckpointOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "B", out.Series)
	assert.Equal(t, "B02", out.Slice)

	checkpointBody := mustReadFile(t, out.CheckpointPath)
	assert.Contains(t, checkpointBody, "series: B")
	assert.Contains(t, checkpointBody, "slice: B02")
	assert.Contains(t, checkpointBody, "`../B00-index.md`")
	assert.Contains(t, checkpointBody, "`../B02-repair-checkpoint-state-plan.md`")

	var record taskCheckpointRecord
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(t, out.CheckpointJSONPath)), &record))
	assert.Equal(t, "B", record.Series)
	assert.Equal(t, "B02", record.Slice)
	assert.Equal(t, "validated", record.Stage)
	assert.Equal(t, "complete", record.Decision)
}

func setupBSeriesTask(t *testing.T) {
	t.Helper()
	setupTaskCommandRepo(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "b-series-test",
		"--series", "b",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "define lifecycle model",
		"--slice", "repair checkpoint state",
		"task workflow ux",
	})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

type lifecycleTaskFixture struct {
	Workspace         string
	IndexPath         string
	AuthoredIndexBody string
}

func TestTaskSliceAddCreatesNextSeriesSliceWithoutRewritingIndex(t *testing.T) {
	fixture := setupLifecycleTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"slice", "add", "lifecycle-add-test", "second lifecycle slice",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskArtifactAddOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "B", out.Series)
	assert.Equal(t, "B02", out.Slice.ID)
	assert.Equal(t, "B02-second-lifecycle-slice-plan.md", filepath.Base(out.Slice.PlanPath))
	assert.Equal(t, fixture.AuthoredIndexBody, mustReadFile(t, fixture.IndexPath))
}

func TestTaskSliceAddAfterCreatesIterationMetadataWithoutRewritingIndex(t *testing.T) {
	fixture := setupLifecycleTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"slice", "add", "lifecycle-add-test", "repair lifecycle status",
		"--after", "B01",
		"--reason", "improve",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskArtifactAddOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "B", out.Series)
	assert.Equal(t, "B01-1", out.Slice.ID)
	assert.Equal(t, "B01-1-repair-lifecycle-status-plan.md", filepath.Base(out.Slice.PlanPath))
	var manifest taskManifest
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(t, filepath.Join(fixture.Workspace, taskManifestFilename))), &manifest))
	require.Len(t, manifest.Artifacts.Slices, 2)
	assert.Equal(t, "B01-1", manifest.Artifacts.Slices[1].ID)
	assert.Equal(t, "iteration", manifest.Artifacts.Slices[1].Kind)
	assert.Equal(t, "B01", manifest.Artifacts.Slices[1].ParentID)
	assert.Equal(t, "improve", manifest.Artifacts.Slices[1].Reason)
	assert.Equal(t, fixture.AuthoredIndexBody, mustReadFile(t, fixture.IndexPath))
}

func TestTaskCheckpointUpdatesIterationStateWithoutRewritingIndex(t *testing.T) {
	fixture := setupLifecycleTask(t)
	addLifecycleIteration(t)
	cmd := newLifecycleIterationCheckpointCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskCheckpointOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "B01-1", out.Slice)
	assert.Equal(t, "B01-1-repair-lifecycle-status-result.md", filepath.Base(out.ResultPath))
	var manifest taskManifest
	require.NoError(t, json.Unmarshal([]byte(mustReadFile(t, filepath.Join(fixture.Workspace, taskManifestFilename))), &manifest))
	require.Len(t, manifest.Artifacts.Slices, 2)
	assert.Equal(t, "implemented", manifest.Artifacts.Slices[1].Stage)
	assert.Equal(t, "promote", manifest.Artifacts.Slices[1].Decision)
	assert.NotEmpty(t, manifest.Artifacts.Slices[1].UpdatedAt)
	assert.True(t, strings.HasPrefix(manifest.Artifacts.Slices[1].LatestCheckpoint, "checkpoints/"))
	assert.True(t, strings.HasSuffix(manifest.Artifacts.Slices[1].LatestCheckpoint, "-implemented.md"))
	assert.True(t, strings.HasPrefix(manifest.Artifacts.Slices[1].LatestCheckpointJSON, "checkpoints/"))
	assert.True(t, strings.HasSuffix(manifest.Artifacts.Slices[1].LatestCheckpointJSON, "-implemented.json"))
	assert.Equal(t, fixture.AuthoredIndexBody, mustReadFile(t, fixture.IndexPath))
}

func TestTaskStatusReflectsPromotedIteration(t *testing.T) {
	setupLifecycleTask(t)
	addLifecycleIteration(t)
	checkpointLifecycleIteration(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"status", "lifecycle-add-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskStatusOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "B", out.Series)
	assert.Equal(t, "packed", out.Status)
	promoted := taskStatusSliceByID(out.Slices, "B01-1")
	require.NotNil(t, promoted)
	assert.Equal(t, "implemented", promoted.Stage)
	assert.Equal(t, "promote", promoted.Decision)
	assert.NotEmpty(t, promoted.UpdatedAt)
	assert.True(t, strings.HasPrefix(promoted.LatestCheckpoint, "checkpoints/"))
	assert.True(t, strings.HasSuffix(promoted.LatestCheckpoint, "-implemented.md"))
	assert.True(t, strings.HasPrefix(promoted.LatestCheckpointJSON, "checkpoints/"))
	assert.True(t, strings.HasSuffix(promoted.LatestCheckpointJSON, "-implemented.json"))
}

func TestTaskDecideCompletesSliceWithoutRewritingIndex(t *testing.T) {
	fixture := setupLifecycleTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"decide", "lifecycle-add-test",
		"--target", "B01",
		"--decision", "complete",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskDecideOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "B01", out.Target)
	assert.Equal(t, "completed", out.Stage)
	assert.Equal(t, "complete", out.Decision)
	assert.Equal(t, fixture.AuthoredIndexBody, mustReadFile(t, fixture.IndexPath))
}

func TestTaskDecideCompletesSeriesWithoutRewritingIndex(t *testing.T) {
	fixture := setupLifecycleTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"decide", "lifecycle-add-test",
		"--target", "B00",
		"--decision", "complete",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskDecideOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "B00", out.Target)
	assert.Equal(t, "completed", out.Stage)
	assert.Equal(t, "complete", out.Decision)
	assert.Equal(t, fixture.AuthoredIndexBody, mustReadFile(t, fixture.IndexPath))
}

func TestTaskStatusReflectsCompletedSeriesAndSlice(t *testing.T) {
	setupLifecycleTask(t)
	decideTaskTarget(t, "lifecycle-add-test", "B01", "complete")
	decideTaskTarget(t, "lifecycle-add-test", "B00", "complete")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"status", "lifecycle-add-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskStatusOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "completed", out.Status)
	assert.Equal(t, "complete", out.Decision)
	completed := taskStatusSliceByID(out.Slices, "B01")
	require.NotNil(t, completed)
	assert.Equal(t, "completed", completed.Stage)
	assert.Equal(t, "complete", completed.Decision)
}

func TestTaskFinishPromotesExplicitSliceWithoutRewritingIndex(t *testing.T) {
	fixture := setupLifecycleTask(t)
	addLifecycleNextSlice(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"finish", "lifecycle-add-test",
		"--target", "B02",
		"--decision", "promote",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskDecideOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "B02", out.Target)
	assert.Equal(t, "promote", out.Decision)
	assert.Equal(t, fixture.AuthoredIndexBody, mustReadFile(t, fixture.IndexPath))
}

func TestTaskRefreshRecapturesEditedIndexWithoutRewritingIt(t *testing.T) {
	fixture := setupLifecycleTask(t)
	refreshedIndexBody := fixture.AuthoredIndexBody + "\n## Human Refresh Notes\n\nRefresh should recapture this without rewriting it.\n"
	mustWriteFile(t, fixture.IndexPath, refreshedIndexBody)
	manifestPath := filepath.Join(fixture.Workspace, taskManifestFilename)
	manifest, err := readTaskManifest(manifestPath)
	require.NoError(t, err)
	manifest.UpdatedAt = "2026-01-01T00:00:00Z"
	require.NoError(t, writeTaskManifest(manifestPath, manifest))
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"refresh", "lifecycle-add-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err = cmd.Execute()

	require.NoError(t, err)
	var out taskSyncOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.True(t, taskArtifactRefreshContainsPath(out.RefreshedArtifacts, "B00-index.md"))
	assert.Equal(t, refreshedIndexBody, mustReadFile(t, fixture.IndexPath))
}

func setupLifecycleTask(t *testing.T) lifecycleTaskFixture {
	t.Helper()
	repoDir := setupTaskCommandRepo(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", "lifecycle-add-test",
		"--series", "B",
		"--no-refresh",
		"--index=false",
		"--slice", "first lifecycle slice",
		"task lifecycle flow",
	})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
	workspace := filepath.Join(repoDir, "devspecs", "tasks", "lifecycle-add-test")
	indexPath := filepath.Join(workspace, "B00-index.md")
	authoredIndexBody := mustReadFile(t, indexPath) + "\n## Human Master Notes\n\nKeep lifecycle state in task.json and result artifacts.\n"
	mustWriteFile(t, indexPath, authoredIndexBody)
	return lifecycleTaskFixture{
		Workspace:         workspace,
		IndexPath:         indexPath,
		AuthoredIndexBody: authoredIndexBody,
	}
}

func addLifecycleNextSlice(t *testing.T) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"slice", "add", "lifecycle-add-test", "second lifecycle slice",
		"--index=false",
		"--json",
	})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func addLifecycleIteration(t *testing.T) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"slice", "add", "lifecycle-add-test", "repair lifecycle status",
		"--after", "B01",
		"--reason", "improve",
		"--index=false",
		"--json",
	})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func newLifecycleIterationCheckpointCmd() *cobra.Command {
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "lifecycle-add-test",
		"--slice", "B01-1",
		"--stage", "implemented",
		"--decision", "promote",
		"--index=false",
		"--json",
	})
	return cmd
}

func checkpointLifecycleIteration(t *testing.T) {
	t.Helper()
	cmd := newLifecycleIterationCheckpointCmd()
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func taskStatusSliceByID(slices []taskStatusSliceOutput, id string) *taskStatusSliceOutput {
	for i := range slices {
		if slices[i].ID == id {
			return &slices[i]
		}
	}
	return nil
}
func TestTaskSliceAddRefusesExistingArtifactFile(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "slice-overwrite-test",
		"--series", "B",
		"--no-refresh",
		"--index=false",
		"--slice", "first lifecycle slice",
		"task lifecycle flow",
	})
	startCmd.SetOut(&bytes.Buffer{})
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}

	workspace := filepath.Join(repoDir, "devspecs", "tasks", "slice-overwrite-test")
	existingPlan := filepath.Join(workspace, "B02-second-lifecycle-slice-plan.md")
	mustWriteFile(t, existingPlan, "# Keep Me\n\nDo not replace this file.\n")

	sliceCmd := NewTaskCmd()
	sliceCmd.SetArgs([]string{
		"slice", "add", "slice-overwrite-test", "second lifecycle slice",
		"--index=false",
	})
	sliceCmd.SetOut(&bytes.Buffer{})
	err := sliceCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to overwrite existing task artifact")

	{

		got := mustReadFile(t, existingPlan)
		assert.Containsf(t, got, "Do not replace this file",
			"existing plan was overwritten:\n%s", got)
	}

}

func TestTaskSliceAddReasonRequiresAfter(t *testing.T) {
	setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "slice-reason-test",
		"--no-refresh",
		"--index=false",
		"--slice", "first lifecycle slice",
		"task lifecycle flow",
	})
	startCmd.SetOut(&bytes.Buffer{})
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}

	sliceCmd := NewTaskCmd()
	sliceCmd.SetArgs([]string{
		"slice", "add", "slice-reason-test", "ambiguous follow-up",
		"--reason", "improve",
		"--index=false",
	})
	sliceCmd.SetOut(&bytes.Buffer{})
	err := sliceCmd.Execute()
	require.Error(t, err,
		"expected --reason without --after to fail")
	assert.Containsf(t, err.Error(),
		"--reason requires --after", "slice reason error missing %q: %v",

		"--reason requires --after",
		err)
	assert.Containsf(t, err.Error(),
		"ds task slice add", "slice reason error missing %q: %v",

		"ds task slice add", err)
	assert.Containsf(t, err.Error(),
		"--after <slice>", "slice reason error missing %q: %v",

		"--after <slice>", err)
	assert.Containsf(t, err.Error(),
		"--reason improve", "slice reason error missing %q: %v",

		"--reason improve", err)

}

func TestTask_StartWarnsAboutOnDiskAnchorMissingFromIndex(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	stalePath := filepath.Join(repoDir, "internal", "retrieval", "companion_recall_new.go")
	mustWriteFile(t, stalePath, `package retrieval

func ImproveCompanionRecallNew() {}
`)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "freshness-warning-test", "--no-refresh", "--json", "improve companion recall"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out taskStartOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, buf.String())
	}
	assert.Truef(t, taskWarningsContainPath(out.FreshnessWarnings, "internal/retrieval/companion_recall_new.go"),
		"expected freshness warning for stale on-disk anchor, got %#v", out.FreshnessWarnings)
	assert.LessOrEqualf(t, len(out.FreshnessWarnings), taskFreshnessMaxWarnings,
		"freshness warnings were not capped: %#v", out.FreshnessWarnings)

	staleCard := taskRiskCardByID(out.RiskCards, "stale-index")
	require.NotNil(t, staleCard)
	assert.Contains(t, strings.Join(staleCard.Evidence, "\n"), "internal/retrieval/companion_recall_new.go")

	assert.Equalf(t, "On-disk paths matched the task but were not indexed", staleCard.Title,
		"stale-index title = %q", staleCard.Title)

	indexBody := mustReadFile(t, out.IndexPath)
	assert.Containsf(t, indexBody,
		"## Freshness Warnings", "A00 missing freshness warning %q:\n%s",

		"## Freshness Warnings", indexBody,
	)
	assert.Containsf(t, indexBody,
		"## Risk Cards",
		"A00 missing freshness warning %q:\n%s",

		"## Risk Cards", indexBody)
	assert.Containsf(t, indexBody,
		"internal/retrieval/companion_recall_new.go", "A00 missing freshness warning %q:\n%s", "internal/retrieval/companion_recall_new.go", indexBody)
	assert.Containsf(t, indexBody,
		"On-disk paths matched the task but were not indexed", "A00 missing freshness warning %q:\n%s", "On-disk paths matched the task but were not indexed",
		indexBody)

	var manifest taskManifest
	{
		err := json.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest)
		require.NoErrorf(t, err,
			"manifest json: %v", err)
	}
	assert.Truef(t, taskWarningsContainPath(manifest.FreshnessWarnings, "internal/retrieval/companion_recall_new.go"),
		"manifest missing freshness warning: %#v", manifest.FreshnessWarnings)

}

type staleTaskFixture struct {
	RepoDir      string
	ManifestPath string
	PlanPath     string
	EditedPlan   string
}

func TestTaskStatusJSONReportsEditedArtifactCapture(t *testing.T) {
	setupStaleTask(t, "sync-freshness-test", "edited after task creation")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"status", "sync-freshness-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskStatusOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	freshness := taskArtifactFreshnessByPath(out.ArtifactFreshness, "A01-improve-test-companion-recall-plan.md")
	require.NotNil(t, freshness)
	assert.Equal(t, "current", freshness.TaskJSONState)
	assert.Equal(t, "needs_refresh", freshness.ArtifactCaptureState)
	assert.Equal(t, "ds task refresh sync-freshness-test", freshness.NextCommand)
}

func TestTaskStatusHumanReportsEditedArtifactCapture(t *testing.T) {
	setupStaleTask(t, "sync-freshness-test", "edited after task creation")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"status", "sync-freshness-test"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Task artifact capture refresh needed:")
	assert.Contains(t, buf.String(), "task.json lifecycle state is still usable")
	assert.Contains(t, buf.String(), "ds task refresh sync-freshness-test")
	assert.NotContains(t, buf.String(), "changed after task state")
}

func TestTaskSyncRecapturesEditedArtifacts(t *testing.T) {
	fixture := setupStaleTask(t, "sync-freshness-test", "edited after task creation")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"sync", "sync-freshness-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskSyncOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "sync-freshness-test", out.TaskID)
	assert.NotEmpty(t, out.ManifestPath)
	assert.True(t, containsPath(out.IndexedPaths, "devspecs/tasks/sync-freshness-test/A00-index.md"))
	assert.True(t, containsPath(out.IndexedPaths, "devspecs/tasks/sync-freshness-test/A01-improve-test-companion-recall-plan.md"))
	assert.True(t, containsPath(out.IndexedPaths, "devspecs/tasks/sync-freshness-test/A01-improve-test-companion-recall-result.md"))
	assert.True(t, taskArtifactFreshnessContainsPath(out.ArtifactFreshness, "A01-improve-test-companion-recall-plan.md"))
	afterManifest, readErr := readTaskManifest(fixture.ManifestPath)
	require.NoError(t, readErr)
	assert.NotEmpty(t, afterManifest.UpdatedAt)
	assert.NotEqual(t, "2026-01-01T00:00:00Z", afterManifest.UpdatedAt)
	db, openErr := openDB()
	require.NoError(t, openErr)
	artifacts, listErr := db.ListArtifacts(store.FilterParams{RepoRoot: fixture.RepoDir, SourceType: "capture"})
	require.NoError(t, listErr)
	require.NoError(t, db.Close())
	assert.True(t, taskArtifactTitleContains(artifacts, "sync-freshness-test A01 result"))
}

func TestTaskStatusIsFreshAfterSync(t *testing.T) {
	setupStaleTask(t, "sync-freshness-test", "edited after task creation")
	syncTaskArtifacts(t, "sync-freshness-test")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"status", "sync-freshness-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskStatusOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Empty(t, out.ArtifactFreshness)
}

func TestTaskRefreshRecapturesEditedArtifactsWithClearOutput(t *testing.T) {
	fixture := setupStaleTask(t, "refresh-freshness-test", "manual readability patch to preserve")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"refresh", "refresh-freshness-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskSyncOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "refresh-freshness-test", out.TaskID)
	assert.NotEmpty(t, out.ManifestPath)
	assert.Empty(t, out.ArtifactFreshness)
	assert.True(t, taskArtifactRefreshContainsPath(out.RefreshedArtifacts, "A01-improve-test-companion-recall-plan.md"))
	for _, artifact := range out.RefreshedArtifacts {
		assert.NotContains(t, artifact.Reason, "run ds task sync")
	}
	assert.Equal(t, fixture.EditedPlan, mustReadFile(t, fixture.PlanPath))
}

func TestTaskStatusIsFreshAfterRefresh(t *testing.T) {
	setupStaleTask(t, "refresh-freshness-test", "manual readability patch to preserve")
	refreshTaskArtifacts(t, "refresh-freshness-test")
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"status", "refresh-freshness-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskStatusOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Empty(t, out.ArtifactFreshness)
}

func setupStaleTask(t *testing.T, taskID string, note string) staleTaskFixture {
	t.Helper()
	repoDir := setupTaskCommandRepo(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--id", taskID,
		"--no-refresh",
		"--index=false",
		"--json",
		"improve test companion recall",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	require.NoError(t, cmd.Execute())
	var out taskStartOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	manifest, err := readTaskManifest(out.ManifestPath)
	require.NoError(t, err)
	manifest.UpdatedAt = "2026-01-01T00:00:00Z"
	require.NoError(t, writeTaskManifest(out.ManifestPath, manifest))
	editedPlan := mustReadFile(t, out.FirstSlicePath) + "\n\n## Dogfood Notes\n- " + note + "\n"
	mustWriteFile(t, out.FirstSlicePath, editedPlan)
	return staleTaskFixture{
		RepoDir:      repoDir,
		ManifestPath: out.ManifestPath,
		PlanPath:     out.FirstSlicePath,
		EditedPlan:   editedPlan,
	}
}

func syncTaskArtifacts(t *testing.T, taskID string) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"sync", taskID, "--json"})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func refreshTaskArtifacts(t *testing.T, taskID string) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"refresh", taskID, "--json"})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func taskArtifactTitleContains(artifacts []store.ArtifactRow, title string) bool {
	for _, artifact := range artifacts {
		if strings.Contains(artifact.Title, title) {
			return true
		}
	}
	return false
}
func TestTaskAuditReportsPassForInScopeCheckpointFiles(t *testing.T) {
	setupPassingAuditTask(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"audit", "audit-test", "--target", "A01", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskAuditOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "pass", out.Recommendation)
	assert.Empty(t, out.OutOfScopePaths)
	assert.True(t, containsPath(out.InScopePaths, "internal/retrieval/ranking.go"))
	assert.True(t, containsPath(out.InScopePaths, "internal/retrieval/ranking_test.go"))
}

func TestTaskAuditReportsDriftForOutOfScopeCheckpointFile(t *testing.T) {
	setupPassingAuditTask(t)
	addDriftAuditCheckpoint(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"audit", "audit-test", "--target", "A01", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskAuditOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "drift", out.Recommendation)
	assert.True(t, containsPath(out.OutOfScopePaths, "internal/other/unrelated.go"))
}

func setupPassingAuditTask(t *testing.T) {
	t.Helper()
	setupTaskCommandRepo(t)
	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "audit-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"improve test companion recall",
	})
	startCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, startCmd.Execute())
	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "audit-test",
		"--stage", "implemented",
		"--decision", "continue",
		"--file-edited", "internal/retrieval/ranking.go",
		"--file-edited", "internal/retrieval/ranking_test.go",
		"--index=false",
		"--json",
	})
	checkpointCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, checkpointCmd.Execute())
}

func addDriftAuditCheckpoint(t *testing.T) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"checkpoint", "audit-test",
		"--stage", "implemented",
		"--decision", "continue",
		"--file-edited", "internal/other/unrelated.go",
		"--index=false",
		"--json",
	})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}
func TestTask_StartUsesGitWorktreeRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	mainRepo := filepath.Join(tmp, "main")
	worktree := filepath.Join(tmp, "linked")
	{

		err := taskGitCmd("init", "-b", "main", mainRepo).Run()
		require.NoError(t, err)
	}

	mustMkdirAll(t, filepath.Join(mainRepo, ".devspecs"))
	mustWriteFile(t, filepath.Join(mainRepo, ".devspecs", "config.yaml"), `version: 1
sources:
  - type: source_context
`)
	mustWriteFile(t, filepath.Join(mainRepo, "go.mod"), "module example.com/worktree\n")
	mustMkdirAll(t, filepath.Join(mainRepo, "internal", "taskroot"))
	mustWriteFile(t, filepath.Join(mainRepo, "internal", "taskroot", "root.go"), `package taskroot

func RootTask() {}
`)
	{
		err := taskGitCmd("-C", mainRepo, "add", ".").Run()
		require.NoError(t, err)
	}
	{

		err := taskGitCmd("-C", mainRepo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "init").Run()
		require.NoError(t, err)
	}
	{

		err := taskGitCmd("-C", mainRepo, "worktree", "add", "-b", "linked-branch", worktree).Run()
		require.NoError(t, err)
	}

	subdir := filepath.Join(worktree, "internal", "taskroot")
	origWd, _ := os.Getwd()
	{
		err := os.Chdir(subdir)
		require.NoError(t, err)
	}

	t.Cleanup(func() { os.Chdir(origWd) })

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--quiet"})
	scanCmd.SetOut(&bytes.Buffer{})
	{
		err := scanCmd.Execute()
		require.NoErrorf(t, err,
			"scan: %v", err)
	}

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"--id", "worktree-root-test", "--no-refresh", "--json", "taskroot"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out taskStartOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, buf.String())
	}

	wantPrefix := filepath.Join(worktree, "devspecs", "tasks", "worktree-root-test")
	assert.Truef(t, strings.HasPrefix(out.Workspace, wantPrefix),
		"workspace = %q, want prefix %q", out.Workspace, wantPrefix)
	assert.False(t, strings.HasPrefix(out.Workspace, filepath.Join(mainRepo, "devspecs")))

}

func TestTask_CheckpointAppendsResultAndIndexesCheckpoint(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{"--id", "checkpoint-test", "--no-refresh", "improve test companion recall"})
	startCmd.SetOut(&bytes.Buffer{})
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "checkpoint-test",
		"--stage", "implemented",
		"--decision", "improve",
		"--note", "found a missing same-package test",
		"--file-read", "internal/retrieval/ranking.go",
		"--file-edited", "internal/retrieval/ranking.go",
		"--test-read", "internal/retrieval/ranking_test.go",
		"--test-run", "go test ./internal/retrieval",
		"--missed-file", "internal/retrieval/ranking_test.go",
		"--noise-file", "fixtures/noisy-plan.md",
		"--learning", "retrieval|same-package tests are important rescue evidence|high|A01|internal/retrieval/ranking_test.go",
		"--next-target", "A01-1",
		"--next-decision", "improve",
		"--json",
	})
	buf := &bytes.Buffer{}
	checkpointCmd.SetOut(buf)
	{
		err := checkpointCmd.Execute()
		require.NoError(t, err)
	}

	var out taskCheckpointOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"checkpoint json: %v\n%s", err, buf.String())
	}
	assert.Equal(t, "implemented", out.Stage)
	assert.Equal(t, "improve", out.Decision)

	assert.Equalf(t, "A01", out.Slice,
		"checkpoint output slice = %q", out.Slice)
	assert.NotEqualf(t, "", out.CheckpointJSONPath,
		"expected structured checkpoint path in output: %#v", out)
	assert.NotEmpty(t, out.CheckpointID)
	assert.Equal(t, 1, out.LearningCount)
	assert.True(t, out.FactIndexed)

	checkpointBody := mustReadFile(t, out.CheckpointPath)
	assertNoTrailingWhitespace(t, "checkpoint markdown", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"---", "checkpoint missing %q:\n%s",

		"---", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"schema_version: 2", "checkpoint missing %q:\n%s",

		"schema_version: 2", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"checkpoint_id:", "checkpoint missing %q:\n%s",

		"checkpoint_id:", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"target: A01", "checkpoint missing %q:\n%s",

		"target: A01", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"slice: A01", "checkpoint missing %q:\n%s",

		"slice: A01", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"parent_slice: A01", "checkpoint missing %q:\n%s",

		"parent_slice: A01", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"stage: implemented",
		"checkpoint missing %q:\n%s",

		"stage: implemented", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"decision: improve", "checkpoint missing %q:\n%s",

		"decision: improve", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"created_at:", "checkpoint missing %q:\n%s",

		"created_at:", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"checkpoint_json:", "checkpoint missing %q:\n%s",

		"checkpoint_json:", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"## Structured Evidence", "checkpoint missing %q:\n%s",

		"## Structured Evidence", checkpointBody,
	)
	assert.Containsf(t, checkpointBody,

		"Checkpoint ID:", "checkpoint missing %q:\n%s",

		"Checkpoint ID:", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"## Files Actually Read", "checkpoint missing %q:\n%s",

		"## Files Actually Read", checkpointBody,
	)
	assert.Containsf(t, checkpointBody,

		"`internal/retrieval/ranking.go`",

		"checkpoint missing %q:\n%s", "`internal/retrieval/ranking.go`", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"## Critical Files DevSpecs Missed", "checkpoint missing %q:\n%s", "## Critical Files DevSpecs Missed", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"`internal/retrieval/ranking_test.go`", "checkpoint missing %q:\n%s", "`internal/retrieval/ranking_test.go`", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"## Distracting Files DevSpecs Included", "checkpoint missing %q:\n%s", "## Distracting Files DevSpecs Included", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"`fixtures/noisy-plan.md`", "checkpoint missing %q:\n%s",

		"`fixtures/noisy-plan.md`", checkpointBody,
	)
	assert.Containsf(t, checkpointBody,

		"## Completion Contract", "checkpoint missing %q:\n%s",

		"## Completion Contract", checkpointBody,
	)
	assert.Containsf(t, checkpointBody,

		"Attempted slice: `A01`", "checkpoint missing %q:\n%s",

		"Attempted slice: `A01`", checkpointBody,
	)
	assert.Containsf(t, checkpointBody,

		"Gate tested: improve",
		"checkpoint missing %q:\n%s",

		"Gate tested: improve", checkpointBody,
	)
	assert.Containsf(t, checkpointBody,

		"found a missing same-package test", "checkpoint missing %q:\n%s", "found a missing same-package test", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"Evidence for decision: 1 file(s) read; 1 file(s) edited; 1 test command(s); 1 missed file(s); 1 noise file(s)", "checkpoint missing %q:\n%s", "Evidence for decision: 1 file(s) read; 1 file(s) edited; 1 test command(s); 1 missed file(s); 1 noise file(s)", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"Next iteration: A01-1 with decision improve", "checkpoint missing %q:\n%s", "Next iteration: A01-1 with decision improve", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"- Block",
		"checkpoint missing %q:\n%s",

		"- Block", checkpointBody)
	assert.NotContainsf(t,
		checkpointBody,
		"\n## Stage\n", "checkpoint should keep metadata heading %q in frontmatter, not body:\n%s",

		"\n## Stage\n", checkpointBody)
	assert.NotContainsf(t,
		checkpointBody,
		"\n## Decision\n",
		"checkpoint should keep metadata heading %q in frontmatter, not body:\n%s",

		"\n## Decision\n", checkpointBody)
	assert.NotContainsf(t,
		checkpointBody,
		"\n## Created At\n",
		"checkpoint should keep metadata heading %q in frontmatter, not body:\n%s",

		"\n## Created At\n", checkpointBody)

	var record taskCheckpointRecord
	{
		err := json.Unmarshal([]byte(mustReadFile(t, out.CheckpointJSONPath)), &record)
		require.NoErrorf(t, err,
			"checkpoint record json: %v", err)
	}
	assert.Equal(t, "checkpoint-test", record.TaskID)
	assert.Equal(t, "implemented", record.Stage)
	assert.Equal(t, "improve", record.Decision)

	assert.Equalf(t, "A01", record.Slice,
		"checkpoint record slice = %q", record.Slice)
	assert.Equalf(t, 2, record.SchemaVersion,
		"checkpoint record schema version = %d", record.SchemaVersion)
	assert.Equal(t, out.CheckpointID, record.CheckpointID)
	assert.Equal(t, "A01", record.Target)
	assert.Equal(t, "A01", record.ParentSlice)

	assert.Truef(t, containsPath(record.FilesEdited, "internal/retrieval/ranking.go"),
		"checkpoint record missing edited file: %#v", record.FilesEdited)
	assert.Truef(t, containsPath(record.ActualContext.FilesEdited, "internal/retrieval/ranking.go"),
		"checkpoint record missing actual context edited file: %#v", record.ActualContext)
	assert.Truef(t, containsPath(record.MissedFiles, "internal/retrieval/ranking_test.go"),
		"checkpoint record missing missed file: %#v", record.MissedFiles)
	assert.Truef(t, containsPath(record.PredictedContextFeedback.CriticalMissed, "internal/retrieval/ranking_test.go"),
		"checkpoint record missing predicted feedback: %#v", record.PredictedContextFeedback)
	require.Len(t, record.Learnings, 1)
	assert.Contains(t, record.Learnings[0].Summary, "same-package tests")
	assert.Equal(t, "A01-1", record.Next.RecommendedTarget)
	assert.Equal(t, "improve", record.Next.RecommendedDecision)

	resultBody := mustReadFile(t, out.ResultPath)
	assertNoTrailingWhitespace(t, "checkpoint result", resultBody)
	assert.Containsf(t, resultBody,

		"## Checkpoint History", "result missing %q:\n%s",

		"## Checkpoint History", resultBody)
	assert.Containsf(t, resultBody,

		"### Checkpoint", "result missing %q:\n%s",

		"### Checkpoint", resultBody)
	assert.Containsf(t, resultBody,

		"Stage: implemented", "result missing %q:\n%s",

		"Stage: implemented", resultBody)
	assert.Containsf(t, resultBody,

		"Decision: improve", "result missing %q:\n%s",

		"Decision: improve", resultBody)
	assert.Containsf(t, resultBody,

		"Structured Evidence:", "result missing %q:\n%s",

		"Structured Evidence:", resultBody)
	assert.Containsf(t, resultBody,

		"What changed: found a missing same-package test", "result missing %q:\n%s", "What changed: found a missing same-package test", resultBody)
	assert.Containsf(t, resultBody,

		"Evidence for decision: 1 file(s) read; 1 file(s) edited; 1 test command(s); 1 missed file(s); 1 noise file(s)", "result missing %q:\n%s", "Evidence for decision: 1 file(s) read; 1 file(s) edited; 1 test command(s); 1 missed file(s); 1 noise file(s)", resultBody)
	assert.Containsf(t, resultBody,

		"What remains: next target A01-1; next decision improve; resolve missed files", "result missing %q:\n%s",
		"What remains: next target A01-1; next decision improve; resolve missed files", resultBody,
	)
	assert.Containsf(t, resultBody,

		"Next iteration: A01-1 with decision improve", "result missing %q:\n%s", "Next iteration: A01-1 with decision improve", resultBody)
	assert.Containsf(t, resultBody,

		"Missed files:",
		"result missing %q:\n%s",

		"Missed files:", resultBody)
	assert.Containsf(t, resultBody,

		"`internal/retrieval/ranking_test.go`",
		"result missing %q:\n%s", "`internal/retrieval/ranking_test.go`", resultBody)
	assert.NotContainsf(t,
		resultBody,
		"## Checkpoints", "result should convert checkpoint template before append, still has %q:\n%s",

		"## Checkpoints", resultBody)
	assert.NotContainsf(t,
		resultBody,
		"Use `ds task checkpoint checkpoint-test --target A01`", "result should convert checkpoint template before append, still has %q:\n%s",
		"Use `ds task checkpoint checkpoint-test --target A01`", resultBody)

	db, err := openDB()
	require.NoError(t, err)

	defer db.Close()
	artifacts, err := db.ListArtifacts(store.FilterParams{RepoRoot: repoDir, SourceType: "capture"})
	require.NoError(t, err)

	foundCheckpoint := false
	for _, art := range artifacts {
		if strings.Contains(art.Title, "checkpoint-test checkpoint implemented") {
			foundCheckpoint = true
			assert.Equalf(t, "implemented", art.Status,
				"checkpoint status = %q", art.Status)

		}
	}
	assert.Truef(t, foundCheckpoint,
		"checkpoint capture artifact not found in %#v", artifacts)

	var repoID string
	{
		err := db.QueryRow("SELECT id FROM repos WHERE root_path = ?", repoDir).Scan(&repoID)
		require.NoError(t, err)
	}

	facts, err := db.ListTaskCheckpointFacts(repoID, "checkpoint-test")
	require.NoError(t, err)
	require.Lenf(t, facts, 1,
		"checkpoint facts = %#v", facts)
	assert.Equal(t, out.CheckpointID, facts[0].CheckpointID)
	assert.Equal(t, "A01", facts[0].Target)
	assert.Equal(t, "implemented", facts[0].Stage)

	assert.Containsf(t, facts[0].ActualContextJSON, "internal/retrieval/ranking.go",
		"checkpoint fact actual context = %s", facts[0].ActualContextJSON)
	assert.Containsf(t, facts[0].LearningsJSON, "same-package tests",
		"checkpoint fact learnings = %s", facts[0].LearningsJSON)
}

func TestTaskEvaluateUsesStructuredCheckpointWhenMarkdownIsUnavailable(t *testing.T) {
	checkpoint := setupCheckpointEvidenceTask(t)
	require.NoError(t, os.WriteFile(checkpoint.CheckpointPath, []byte("# scrubbed markdown checkpoint\n"), 0o644))
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"evaluate", "checkpoint-test", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskEvaluationOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "checkpoint-test", out.TaskID)
	assert.True(t, containsPath(out.Hits, "internal/retrieval/ranking_test.go"))
	assert.False(t, containsPath(out.Misses, "internal/retrieval/ranking_test.go"))
	assert.Equal(t, "1/1", out.Metrics.TestCompanionRecall)
	assert.True(t, containsPath(out.Noise, "fixtures/noisy-plan.md"))
	assert.Equal(t, 1, out.CheckpointSummary.JSONRecords)
	assert.Equal(t, 0, out.CheckpointSummary.MarkdownFallbacks)
}

func setupCheckpointEvidenceTask(t *testing.T) taskCheckpointOutput {
	t.Helper()
	setupTaskCommandRepo(t)
	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{"--id", "checkpoint-test", "--no-refresh", "improve test companion recall"})
	startCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, startCmd.Execute())
	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "checkpoint-test",
		"--stage", "implemented",
		"--decision", "improve",
		"--note", "found a missing same-package test",
		"--file-read", "internal/retrieval/ranking.go",
		"--file-edited", "internal/retrieval/ranking.go",
		"--test-read", "internal/retrieval/ranking_test.go",
		"--test-run", "go test ./internal/retrieval",
		"--missed-file", "internal/retrieval/ranking_test.go",
		"--noise-file", "fixtures/noisy-plan.md",
		"--learning", "retrieval|same-package tests are important rescue evidence|high|A01|internal/retrieval/ranking_test.go",
		"--next-target", "A01-1",
		"--next-decision", "improve",
		"--json",
	})
	buf := &bytes.Buffer{}
	checkpointCmd.SetOut(buf)
	require.NoError(t, checkpointCmd.Execute())
	var out taskCheckpointOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	return out
}
func TestTask_CheckpointResultHistoryAppendsWithoutDuplicatingTemplate(t *testing.T) {
	setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "checkpoint-history-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"improve test companion recall",
	})
	startBuf := &bytes.Buffer{}
	startCmd.SetOut(startBuf)
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}

	var startOut taskStartOutput
	{
		err := json.Unmarshal(startBuf.Bytes(), &startOut)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, startBuf.String())
	}

	firstCmd := NewTaskCmd()
	firstCmd.SetArgs([]string{
		"checkpoint", "checkpoint-history-test",
		"--target", "A01",
		"--stage", "implemented",
		"--decision", "improve",
		"--description", "first checkpoint rewrites result template",
		"--file-edited", "internal/retrieval/ranking.go",
		"--index=false",
		"--json",
	})
	firstCmd.SetOut(&bytes.Buffer{})
	{
		err := firstCmd.Execute()
		require.NoError(t, err)
	}

	secondCmd := NewTaskCmd()
	secondCmd.SetArgs([]string{
		"checkpoint", "checkpoint-history-test",
		"--target", "A01",
		"--stage", "validated",
		"--decision", "promote",
		"--description", "second checkpoint stays in history",
		"--test-run", "go test ./internal/retrieval -count=1",
		"--index=false",
		"--json",
	})
	secondCmd.SetOut(&bytes.Buffer{})
	{
		err := secondCmd.Execute()
		require.NoError(t, err)
	}

	resultBody := mustReadFile(t, startOut.ResultPath)
	assertNoTrailingWhitespace(t, "checkpoint history result", resultBody)
	{
		got := strings.Count(resultBody, "## Checkpoint History")
		assert.Equalf(t, 1, got,
			"checkpoint history heading count = %d:\n%s", got, resultBody)
	}
	{

		got := strings.Count(resultBody, "### Checkpoint")
		assert.Equalf(t, 2, got,
			"checkpoint entry count = %d:\n%s", got, resultBody)
	}
	assert.Containsf(t, resultBody,

		"first checkpoint rewrites result template", "result history missing %q:\n%s", "first checkpoint rewrites result template", resultBody)
	assert.Containsf(t, resultBody,

		"second checkpoint stays in history",

		"result history missing %q:\n%s", "second checkpoint stays in history", resultBody)
	assert.Containsf(t, resultBody,

		"Stage: implemented", "result history missing %q:\n%s",

		"Stage: implemented", resultBody)
	assert.Containsf(t, resultBody,

		"Stage: validated", "result history missing %q:\n%s",

		"Stage: validated", resultBody)
	assert.NotContainsf(t,
		resultBody,
		"## Checkpoints", "result history should not keep template prompt %q:\n%s",

		"## Checkpoints",
		resultBody)
	assert.NotContainsf(t,
		resultBody,
		"Use `ds task checkpoint checkpoint-history-test --target A01`", "result history should not keep template prompt %q:\n%s",
		"Use `ds task checkpoint checkpoint-history-test --target A01`", resultBody)

}

func TestTask_CheckpointDraftPreviewsWithoutMutation(t *testing.T) {
	setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "checkpoint-draft-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"improve test companion recall",
	})
	startBuf := &bytes.Buffer{}
	startCmd.SetOut(startBuf)
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}

	var startOut taskStartOutput
	{
		err := json.Unmarshal(startBuf.Bytes(), &startOut)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, startBuf.String())
	}

	manifestBefore := mustReadFile(t, startOut.ManifestPath)
	resultBefore := mustReadFile(t, startOut.ResultPath)
	checkpointDir := filepath.Join(startOut.Workspace, "checkpoints")
	require.NoDirExists(t, checkpointDir)

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "checkpoint-draft-test",
		"--target", "A01",
		"--draft",
		"--stage", "validated",
		"--decision", "promote",
		"--description", "wired a checkpoint draft preview",
		"--file-read", "internal/commands/task.go",
		"--file-edited", "internal/commands/task.go",
		"--test-run", "go test ./internal/commands -run TestTask_CheckpointDraftPreviewsWithoutMutation -count=1",
		"--next-target", "B02",
		"--json",
	})
	buf := &bytes.Buffer{}
	checkpointCmd.SetOut(buf)
	{
		err := checkpointCmd.Execute()
		require.NoError(t, err)
	}

	var draft taskCheckpointDraftOutput
	{
		err := json.Unmarshal(buf.Bytes(), &draft)
		require.NoErrorf(t, err,
			"draft json: %v\n%s", err, buf.String())
	}
	assert.True(t, draft.Draft)
	assert.False(t, draft.Mutates)
	assert.Equal(t, "draft_a01_validated", draft.CheckpointID)
	assert.Equal(t, "A01", draft.Slice)
	assert.Equal(t, "checkpoints/<timestamp>-validated.md", draft.CheckpointPathHint)
	assert.Equal(t, "checkpoints/<timestamp>-validated.json", draft.CheckpointJSONHint)

	assert.Equalf(t, startOut.ResultPath, draft.ResultPath,
		"draft result path = %q, want %q", draft.ResultPath, startOut.ResultPath)
	assert.Containsf(t, draft.CheckpointMarkdown,
		"checkpoint_id: draft_a01_validated", "draft markdown missing %q:\n%s", "checkpoint_id: draft_a01_validated", draft.CheckpointMarkdown)
	assert.Containsf(t, draft.CheckpointMarkdown,
		"created_at: <generated-at-checkpoint>", "draft markdown missing %q:\n%s", "created_at: <generated-at-checkpoint>", draft.CheckpointMarkdown,
	)
	assert.Containsf(t, draft.CheckpointMarkdown,
		"checkpoint_json: checkpoints/<timestamp>-validated.json", "draft markdown missing %q:\n%s",
		"checkpoint_json: checkpoints/<timestamp>-validated.json", draft.CheckpointMarkdown,
	)
	assert.Containsf(t, draft.CheckpointMarkdown,
		"wired a checkpoint draft preview", "draft markdown missing %q:\n%s", "wired a checkpoint draft preview", draft.CheckpointMarkdown)
	assert.Containsf(t, draft.CheckpointMarkdown,
		"## Files Actually Edited", "draft markdown missing %q:\n%s", "## Files Actually Edited", draft.CheckpointMarkdown)
	assert.Containsf(t, draft.CheckpointMarkdown,
		"`internal/commands/task.go`", "draft markdown missing %q:\n%s", "`internal/commands/task.go`", draft.CheckpointMarkdown)
	assert.Containsf(t, draft.CheckpointMarkdown,
		"Next iteration: B02 with decision -", "draft markdown missing %q:\n%s", "Next iteration: B02 with decision -", draft.CheckpointMarkdown,
	)
	assert.Containsf(t, draft.ResultAppendMarkdown,
		"### Checkpoint", "draft result append missing %q:\n%s",

		"### Checkpoint", draft.ResultAppendMarkdown)
	assert.Containsf(t, draft.ResultAppendMarkdown,
		"Created At: <generated-at-checkpoint>", "draft result append missing %q:\n%s", "Created At: <generated-at-checkpoint>", draft.ResultAppendMarkdown,
	)
	assert.Containsf(t, draft.ResultAppendMarkdown,
		"Source: `checkpoints/<timestamp>-validated.md`", "draft result append missing %q:\n%s",
		"Source: `checkpoints/<timestamp>-validated.md`", draft.ResultAppendMarkdown)
	assert.Containsf(t, draft.ResultAppendMarkdown,
		"Structured Evidence: `checkpoints/<timestamp>-validated.json`", "draft result append missing %q:\n%s",
		"Structured Evidence: `checkpoints/<timestamp>-validated.json`", draft.ResultAppendMarkdown)
	assert.Containsf(t, draft.ResultAppendMarkdown,
		"Evidence for decision: 1 file(s) read; 1 file(s) edited; 1 test command(s)", "draft result append missing %q:\n%s",
		"Evidence for decision: 1 file(s) read; 1 file(s) edited; 1 test command(s)", draft.ResultAppendMarkdown,
	)

	assert.Equal(t, "<generated-at-checkpoint>", draft.CheckpointRecord.CreatedAt)
	assert.Equal(t, draft.CheckpointID, draft.CheckpointRecord.CheckpointID)

	assert.Truef(t, containsPath(draft.CheckpointRecord.FilesEdited, "internal/commands/task.go"),
		"draft record files edited = %#v", draft.CheckpointRecord.FilesEdited)
	assert.Equalf(t, "B02", draft.CheckpointRecord.Next.RecommendedTarget,
		"draft record next = %#v", draft.CheckpointRecord.Next)
	{

		got := mustReadFile(t, startOut.ManifestPath)
		assert.Equalf(t, manifestBefore, got,
			"draft mutated manifest.\nBefore:\n%s\nAfter:\n%s", manifestBefore, got)
	}
	{

		got := mustReadFile(t, startOut.ResultPath)
		assert.Equalf(t, resultBefore, got,
			"draft mutated result.\nBefore:\n%s\nAfter:\n%s", resultBefore, got)
	}
	{

		_, err := os.Stat(checkpointDir)
		assert.Truef(t, os.IsNotExist(err),
			"draft created checkpoint dir, stat err = %v", err)
	}

}

func TestTask_CheckpointDraftHumanOutput(t *testing.T) {
	setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "checkpoint-draft-human-test",
		"--no-refresh",
		"--index=false",
		"improve test companion recall",
	})
	startCmd.SetOut(&bytes.Buffer{})
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "checkpoint-draft-human-test",
		"--target", "A01",
		"--draft",
		"--stage", "implemented",
		"--decision", "continue",
		"--file-read", "internal/commands/task.go",
	})
	buf := &bytes.Buffer{}
	checkpointCmd.SetOut(buf)
	{
		err := checkpointCmd.Execute()
		require.NoError(t, err)
	}

	got := buf.String()
	assert.Containsf(t, got,
		"Draft checkpoint for checkpoint-draft-human-test A01", "draft human output missing %q:\n%s", "Draft checkpoint for checkpoint-draft-human-test A01", got)
	assert.Containsf(t, got,
		"No files were written. Lifecycle state, result files, and index state are unchanged.", "draft human output missing %q:\n%s",
		"No files were written. Lifecycle state, result files, and index state are unchanged.", got)
	assert.Containsf(t, got,
		"Would write checkpoint: checkpoints/<timestamp>-implemented.md", "draft human output missing %q:\n%s",
		"Would write checkpoint: checkpoints/<timestamp>-implemented.md", got)
	assert.Containsf(t, got,
		"Checkpoint preview:",
		"draft human output missing %q:\n%s",

		"Checkpoint preview:", got)
	assert.Containsf(t, got,
		"Result append preview:", "draft human output missing %q:\n%s",

		"Result append preview:", got)
	assert.Containsf(t, got,
		"Draft checkpoint generated by `ds task checkpoint --draft`; no files were written.", "draft human output missing %q:\n%s",
		"Draft checkpoint generated by `ds task checkpoint --draft`; no files were written.", got)

}

func TestTask_CheckpointFromGitCapturesEditedFiles(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	initTaskGitRepo(t, repoDir)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "checkpoint-from-git-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"improve test companion recall",
	})
	startBuf := &bytes.Buffer{}
	startCmd.SetOut(startBuf)
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}

	var startOut taskStartOutput
	{
		err := json.Unmarshal(startBuf.Bytes(), &startOut)
		require.NoErrorf(t, err,
			"task json: %v\n%s", err, startBuf.String())
	}

	commitTaskGitRepo(t, repoDir, "baseline")

	mustWriteFile(t, filepath.Join(repoDir, "internal", "retrieval", "ranking.go"), mustReadFile(t, filepath.Join(repoDir, "internal", "retrieval", "ranking.go"))+"\nfunc FromGitUnstaged() {}\n")
	mustWriteFile(t, filepath.Join(repoDir, "docs", "plans", "test-companion-recall.md"), mustReadFile(t, filepath.Join(repoDir, "docs", "plans", "test-companion-recall.md"))+"\n- staged from-git note\n")
	{
		err := taskGitCmd("-C", repoDir, "add", "docs/plans/test-companion-recall.md").Run()
		require.NoError(t, err)
	}

	mustWriteFile(t, filepath.Join(repoDir, "internal", "retrieval", "new_helper.go"), "package retrieval\n\nfunc FromGitUntracked() {}\n")

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "checkpoint-from-git-test",
		"--target", "A01",
		"--from-git",
		"--stage", "implemented",
		"--decision", "promote",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	checkpointCmd.SetOut(buf)
	{
		err := checkpointCmd.Execute()
		require.NoError(t, err)
	}

	var out taskCheckpointOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"checkpoint json: %v\n%s", err, buf.String())
	}
	assert.Truef(t,
		containsPath(out.GitDiffFiles, "internal/retrieval/ranking.go"), "checkpoint output git diff files missing %q: %#v",
		"internal/retrieval/ranking.go", out.GitDiffFiles,
	)
	assert.Truef(t,
		containsPath(out.GitDiffFiles, "docs/plans/test-companion-recall.md"), "checkpoint output git diff files missing %q: %#v",
		"docs/plans/test-companion-recall.md", out.GitDiffFiles)
	assert.Truef(t,
		containsPath(out.GitDiffFiles, "internal/retrieval/new_helper.go"), "checkpoint output git diff files missing %q: %#v",
		"internal/retrieval/new_helper.go", out.GitDiffFiles,
	)

	var record taskCheckpointRecord
	{
		err := json.Unmarshal([]byte(mustReadFile(t, out.CheckpointJSONPath)), &record)
		require.NoErrorf(t, err,
			"checkpoint record json: %v", err)
	}
	assert.Truef(t,
		containsPath(record.FilesEdited,
			"internal/retrieval/ranking.go"), "record files edited missing %q: %#v", "internal/retrieval/ranking.go", record.FilesEdited)
	assert.Truef(t, containsPath(record.ActualContext.FilesEdited, "internal/retrieval/ranking.go"), "actual context files edited missing %q: %#v",
		"internal/retrieval/ranking.go", record.ActualContext.FilesEdited)
	assert.Truef(t, containsPath(record.Evidence.GitDiff.
		ChangedFiles, "internal/retrieval/ranking.go"), "git evidence changed files missing %q: %#v",

		"internal/retrieval/ranking.go", record.Evidence.GitDiff)
	assert.Truef(t,
		containsPath(record.FilesEdited,
			"docs/plans/test-companion-recall.md"), "record files edited missing %q: %#v", "docs/plans/test-companion-recall.md", record.FilesEdited,
	)
	assert.Truef(t, containsPath(record.ActualContext.FilesEdited, "docs/plans/test-companion-recall.md"), "actual context files edited missing %q: %#v",

		"docs/plans/test-companion-recall.md", record.ActualContext.FilesEdited)
	assert.Truef(t, containsPath(record.Evidence.GitDiff.ChangedFiles,

		"docs/plans/test-companion-recall.md"), "git evidence changed files missing %q: %#v",

		"docs/plans/test-companion-recall.md", record.Evidence.GitDiff,
	)
	assert.Truef(t,
		containsPath(record.FilesEdited,
			"internal/retrieval/new_helper.go"), "record files edited missing %q: %#v", "internal/retrieval/new_helper.go", record.FilesEdited)
	assert.Truef(t, containsPath(record.ActualContext.FilesEdited, "internal/retrieval/new_helper.go"), "actual context files edited missing %q: %#v",

		"internal/retrieval/new_helper.go", record.ActualContext.FilesEdited,
	)
	assert.Truef(t, containsPath(record.Evidence.GitDiff.ChangedFiles, "internal/retrieval/new_helper.go"), "git evidence changed files missing %q: %#v",
		"internal/retrieval/new_helper.go", record.Evidence.GitDiff)

	require.NotNil(t, record.Evidence.GitDiff)
	assert.Contains(t, record.Evidence.GitDiff.Status, "internal/retrieval/ranking.go")
	assert.False(t, containsPath(record.FilesEdited, filepath.ToSlash(taskRelativePath(repoDir, out.CheckpointPath))))

	resultBody := mustReadFile(t, out.ResultPath)
	assert.Containsf(t, resultBody,

		"Evidence for decision: 3 file(s) edited", "result missing %q:\n%s", "Evidence for decision: 3 file(s) edited", resultBody)
	assert.Containsf(t, resultBody,

		"Files edited:",
		"result missing %q:\n%s",

		"Files edited:", resultBody)
	assert.Containsf(t, resultBody,

		"`internal/retrieval/ranking.go`", "result missing %q:\n%s",

		"`internal/retrieval/ranking.go`",
		resultBody)
	assert.Containsf(t, resultBody,

		"`docs/plans/test-companion-recall.md`", "result missing %q:\n%s", "`docs/plans/test-companion-recall.md`", resultBody)
	assert.Containsf(t, resultBody,

		"`internal/retrieval/new_helper.go`",

		"result missing %q:\n%s", "`internal/retrieval/new_helper.go`", resultBody)

	assert.Containsf(t, mustReadFile(t, out.CheckpointPath), "## Files Actually Edited",
		"checkpoint markdown missing edited-files section")

}

func TestTask_CheckpointGitDiffWithoutFromGitStaysEvidenceOnly(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	initTaskGitRepo(t, repoDir)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "checkpoint-git-diff-only-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"improve test companion recall",
	})
	startCmd.SetOut(&bytes.Buffer{})
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}

	commitTaskGitRepo(t, repoDir, "baseline")
	mustWriteFile(t, filepath.Join(repoDir, "internal", "retrieval", "ranking.go"), mustReadFile(t, filepath.Join(repoDir, "internal", "retrieval", "ranking.go"))+"\nfunc GitDiffOnly() {}\n")

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "checkpoint-git-diff-only-test",
		"--target", "A01",
		"--draft",
		"--git-diff",
		"--stage", "implemented",
		"--decision", "continue",
		"--json",
	})
	buf := &bytes.Buffer{}
	checkpointCmd.SetOut(buf)
	{
		err := checkpointCmd.Execute()
		require.NoError(t, err)
	}

	var draft taskCheckpointDraftOutput
	{
		err := json.Unmarshal(buf.Bytes(), &draft)
		require.NoErrorf(t, err,
			"draft json: %v\n%s", err, buf.String())
	}
	assert.Truef(t, containsPath(draft.GitDiffFiles, "internal/retrieval/ranking.go"),
		"draft git diff files missing changed file: %#v", draft.GitDiffFiles)
	assert.False(t, containsPath(draft.CheckpointRecord.FilesEdited, "internal/retrieval/ranking.go"))

	assert.Containsf(t, draft.CheckpointMarkdown, "## Files Actually Edited\n-\n",
		"draft markdown should keep edited files empty without --from-git:\n%s", draft.CheckpointMarkdown)

}

func TestTask_CheckpointRunLogsIngestExplicitEvidence(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "checkpoint-run-log-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"improve test companion recall",
	})
	startCmd.SetOut(&bytes.Buffer{})
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}

	typecheckLog := filepath.Join(".devspecs", "run-logs", "typecheck.log")
	buildLog := filepath.Join(".devspecs", "run-logs", "build.log")
	mustMkdirAll(t, filepath.Join(repoDir, ".devspecs", "run-logs"))
	mustWriteFile(t, filepath.Join(repoDir, typecheckLog), "command: npm run typecheck\nexit_code: 0\nsrc/index.ts ok\n")
	mustWriteFile(t, filepath.Join(repoDir, buildLog), "$ npm run build\nbuilding app\nexit_code: 0\n")

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "checkpoint-run-log-test",
		"--target", "A01",
		"--draft",
		"--run-log", filepath.ToSlash(typecheckLog),
		"--run-log", filepath.ToSlash(buildLog),
		"--stage", "validated",
		"--decision", "promote",
		"--json",
	})
	buf := &bytes.Buffer{}
	checkpointCmd.SetOut(buf)
	{
		err := checkpointCmd.Execute()
		require.NoError(t, err)
	}

	var draft taskCheckpointDraftOutput
	{
		err := json.Unmarshal(buf.Bytes(), &draft)
		require.NoErrorf(t, err,
			"draft json: %v\n%s", err, buf.String())
	}
	assert.Equalf(t, 2, draft.TestEvidenceCount,
		"test evidence count = %d, want 2: %#v", draft.TestEvidenceCount, draft)
	assert.Truef(t,
		containsString(draft.CheckpointRecord.
			TestsRun,
			"npm run typecheck"), "draft tests_run missing %q: %#v", "npm run typecheck", draft.CheckpointRecord.TestsRun)
	assert.Truef(t, containsString(draft.CheckpointRecord.ActualContext.TestsRun, "npm run typecheck"), "draft actual tests_run missing %q: %#v",
		"npm run typecheck",
		draft.CheckpointRecord.ActualContext.TestsRun)
	assert.Truef(t,
		containsString(draft.CheckpointRecord.
			TestsRun,
			"npm run build"), "draft tests_run missing %q: %#v", "npm run build", draft.CheckpointRecord.TestsRun)
	assert.Truef(t,
		containsString(draft.CheckpointRecord.ActualContext.TestsRun, "npm run build"), "draft actual tests_run missing %q: %#v", "npm run build", draft.CheckpointRecord.ActualContext.TestsRun)

	require.Lenf(t, draft.CheckpointRecord.Evidence.TestCommands, 2,
		"run log evidence = %#v", draft.CheckpointRecord.Evidence.TestCommands)

	typecheck := draft.CheckpointRecord.Evidence.TestCommands[0]
	assert.Equal(t, "npm run typecheck", typecheck.Command)
	assert.Equal(t, 0, typecheck.ExitCode)
	assert.Equal(t, filepath.ToSlash(typecheckLog), typecheck.Source)

	assert.Containsf(t, typecheck.Output, "src/index.ts ok",
		"typecheck output missing log body: %#v", typecheck)

	build := draft.CheckpointRecord.Evidence.TestCommands[1]
	assert.Equal(t, "npm run build", build.Command)
	assert.Contains(t, build.Output, "building app")
	assert.Containsf(t, draft.CheckpointMarkdown,
		"## Tests Actually Run",

		"draft markdown missing %q:\n%s", "## Tests Actually Run", draft.CheckpointMarkdown)
	assert.Containsf(t, draft.CheckpointMarkdown,
		"`npm run typecheck`",
		"draft markdown missing %q:\n%s",
		"`npm run typecheck`", draft.CheckpointMarkdown)
	assert.Contains(t, draft.ResultAppendMarkdown, "`npm run typecheck`")
	assert.Containsf(t, draft.CheckpointMarkdown,
		"`npm run build`", "draft markdown missing %q:\n%s",

		"`npm run build`", draft.CheckpointMarkdown,
	)
	assert.Contains(t, draft.ResultAppendMarkdown, "`npm run build`")
	assert.Containsf(t, draft.CheckpointMarkdown,
		"Evidence for decision: 2 test command(s)", "draft markdown missing %q:\n%s", "Evidence for decision: 2 test command(s)", draft.CheckpointMarkdown,
	)
	assert.Contains(t, draft.ResultAppendMarkdown, "Evidence for decision: 2 test command(s)")

}

func TestTask_EvaluateRunLogCommandsAreActualRuns(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "checkpoint-run-log-eval-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"improve test companion recall",
	})
	startCmd.SetOut(&bytes.Buffer{})
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}

	runLog := filepath.Join(".devspecs", "run-logs", "test.log")
	mustMkdirAll(t, filepath.Join(repoDir, ".devspecs", "run-logs"))
	mustWriteFile(t, filepath.Join(repoDir, runLog), "$ go test ./internal/retrieval\nok example.com/repo/internal/retrieval\nexit_code: 0\n")

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "checkpoint-run-log-eval-test",
		"--target", "A01",
		"--run-log", filepath.ToSlash(runLog),
		"--stage", "validated",
		"--decision", "promote",
		"--index=false",
		"--json",
	})
	checkpointCmd.SetOut(&bytes.Buffer{})
	{
		err := checkpointCmd.Execute()
		require.NoError(t, err)
	}

	evalCmd := NewTaskCmd()
	evalCmd.SetArgs([]string{"evaluate", "checkpoint-run-log-eval-test", "--json"})
	evalBuf := &bytes.Buffer{}
	evalCmd.SetOut(evalBuf)
	{
		err := evalCmd.Execute()
		require.NoError(t, err)
	}

	var evalOut taskEvaluationOutput
	{
		err := json.Unmarshal(evalBuf.Bytes(), &evalOut)
		require.NoErrorf(t, err,
			"evaluate json: %v\n%s", err, evalBuf.String())
	}
	assert.Truef(t, containsString(evalOut.Observed.TestsRun, "go test ./internal/retrieval"),
		"run log command should be actual tests_run, got %#v", evalOut.Observed.TestsRun)
	assert.Truef(t, containsString(evalOut.Observed.TestCommands, "go test ./internal/retrieval"),
		"run log command should retain structured evidence, got %#v", evalOut.Observed.TestCommands)
	assert.False(t, containsString(evalOut.CheckpointSummary.EvidenceOnlyTestCommands, "go test ./internal/retrieval"))

}

func TestTask_CheckpointTargetsSelectedSlice(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "slice-target-test",
		"--no-refresh",
		"--index=false",
		"--slice", "first pass",
		"--slice", "second pass",
		"improve test companion recall",
	})
	startCmd.SetOut(&bytes.Buffer{})
	{
		err := startCmd.Execute()
		require.NoError(t, err)
	}

	workspace := filepath.Join(repoDir, "devspecs", "tasks", "slice-target-test")
	firstResult := filepath.Join(workspace, "A01-first-pass-result.md")
	secondResult := filepath.Join(workspace, "A02-second-pass-result.md")

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "slice-target-test",
		"--target", "A02",
		"--stage", "implemented",
		"--decision", "promote",
		"--note", "targeted checkpoint",
		"--file-read", "internal/retrieval/ranking.go",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	checkpointCmd.SetOut(buf)
	{
		err := checkpointCmd.Execute()
		require.NoError(t, err)
	}

	var out taskCheckpointOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"checkpoint json: %v\n%s", err, buf.String())
	}
	assert.Equalf(t, "A02", out.Slice,
		"checkpoint output slice = %q", out.Slice)
	assert.Equalf(t, "A02-second-pass-result.md", filepath.Base(out.ResultPath),
		"checkpoint result path = %q", out.ResultPath)

	firstBody := mustReadFile(t, firstResult)
	assert.NotContainsf(t, firstBody, "targeted checkpoint",
		"first slice result should not receive A02 checkpoint:\n%s", firstBody)

	secondBody := mustReadFile(t, secondResult)
	assert.Containsf(t, secondBody,

		"targeted checkpoint", "second slice result missing %q:\n%s",

		"targeted checkpoint", secondBody,
	)
	assert.Containsf(t, secondBody,

		"Stage: implemented", "second slice result missing %q:\n%s",

		"Stage: implemented", secondBody)
	assert.Containsf(t, secondBody,

		"Decision: promote", "second slice result missing %q:\n%s",

		"Decision: promote", secondBody)
	assert.Containsf(t, secondBody,

		"`internal/retrieval/ranking.go`", "second slice result missing %q:\n%s",

		"`internal/retrieval/ranking.go`", secondBody)

	checkpointBody := mustReadFile(t, out.CheckpointPath)
	assert.Containsf(t, checkpointBody,

		"slice: A02", "checkpoint body missing %q:\n%s",

		"slice: A02", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"`../A02-second-pass-plan.md`", "checkpoint body missing %q:\n%s",

		"`../A02-second-pass-plan.md`", checkpointBody)
	assert.Containsf(t, checkpointBody,

		"`../A02-second-pass-result.md`",

		"checkpoint body missing %q:\n%s", "`../A02-second-pass-result.md`", checkpointBody)

	var record taskCheckpointRecord
	{
		err := json.Unmarshal([]byte(mustReadFile(t, out.CheckpointJSONPath)), &record)
		require.NoErrorf(t, err,
			"checkpoint record json: %v", err)
	}
	assert.Equal(t, "A02", record.Slice)
	assert.Equal(t, "second pass", record.SliceTitle)

}

func TestTask_EvaluateReportsStructuredEvidenceWithoutInflatingActualContext(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	taskID := "json-evidence-test"
	workspace := filepath.Join(repoDir, "devspecs", "tasks", taskID)
	mustMkdirAll(t, filepath.Join(workspace, "checkpoints"))
	manifest := taskManifest{
		TaskID:    taskID,
		Query:     "structured checkpoint evidence",
		Status:    "packed",
		CreatedAt: "2026-06-04T00:00:00Z",
		RepoRoot:  repoDir,
		Workspace: filepath.ToSlash(workspace),
		Artifacts: taskArtifactPaths{
			Index:      "A00-index.md",
			FirstSlice: "A01-first-slice.md",
			Result:     "A01-1-result.md",
		},
		Predicted: taskPredictedContext{
			PrimaryFiles: []taskPredictedFile{{Path: "internal/retrieval/ranking.go"}},
			Tests:        []taskPredictedFile{{Path: "internal/retrieval/ranking_test.go"}},
		},
		Confidence: taskConfidence{
			PrimaryFileConfidence:  "medium",
			TestCoverageConfidence: "medium",
			DocsConfigConfidence:   "low",
			GitReceiptConfidence:   "low",
			NoiseRisk:              "low",
			PackCompleteness:       "medium",
		},
	}
	{
		err := writeTaskManifest(filepath.Join(workspace, taskManifestFilename), manifest)
		require.NoError(t, err)
	}

	record := taskCheckpointRecord{
		SchemaVersion: 1,
		TaskID:        taskID,
		Query:         manifest.Query,
		Stage:         "implemented",
		Decision:      "improve",
		CreatedAt:     "2026-06-04T00:00:01Z",
		FilesRead:     []string{"internal/retrieval/ranking.go"},
		TestsRead:     []string{"internal/retrieval/ranking_test.go"},
		MissedFiles:   []string{"internal/commands/task_evaluate.go"},
		NoiseFiles:    []string{"fixtures/noisy-plan.md"},
		Evidence: taskCheckpointEvidence{
			GitDiff: &taskGitDiffEvidence{
				Command:      "git diff --stat -- .; git diff --name-only -- .",
				ChangedFiles: []string{"internal/commands/task.go"},
				MaxBytes:     12000,
			},
			TestCommands: []taskCommandRunEvidence{{
				Command:  "go test ./internal/retrieval -run TestImproveTestCompanionRecall -count=1",
				ExitCode: 0,
				Output:   "ok",
				MaxBytes: 12000,
			}},
		},
	}
	{
		err := writeTaskCheckpointRecord(filepath.Join(workspace, "checkpoints", "20260604-000001-implemented.json"), record)
		require.NoError(t, err)
	}

	evalCmd := NewTaskCmd()
	evalCmd.SetArgs([]string{"evaluate", taskID, "--json"})
	evalBuf := &bytes.Buffer{}
	evalCmd.SetOut(evalBuf)
	{
		err := evalCmd.Execute()
		require.NoError(t, err)
	}

	var evalOut taskEvaluationOutput
	{
		err := json.Unmarshal(evalBuf.Bytes(), &evalOut)
		require.NoErrorf(t, err,
			"evaluate json: %v\n%s", err, evalBuf.String())
	}
	assert.Truef(t, containsPath(evalOut.Hits, "internal/retrieval/ranking.go"),
		"expected predicted source hit, got %#v", evalOut.Hits)
	assert.False(t, containsPath(evalOut.Misses, "internal/commands/task.go"))

	assert.Truef(t, containsPath(evalOut.Observed.GitDiffFiles, "internal/commands/task.go"),
		"expected git diff evidence in observed context, got %#v", evalOut.Observed.GitDiffFiles)
	assert.Truef(t, containsPath(evalOut.Misses, "internal/commands/task_evaluate.go"),
		"expected explicit JSON missed file, got %#v", evalOut.Misses)
	assert.Truef(t, containsPath(evalOut.Noise, "fixtures/noisy-plan.md"),
		"expected explicit JSON noise file, got %#v", evalOut.Noise)
	assert.Truef(t, containsString(evalOut.Observed.TestCommands, "go test ./internal/retrieval -run TestImproveTestCompanionRecall -count=1"),
		"expected structured test command evidence, got %#v", evalOut.Observed.TestCommands)
	assert.False(t, containsString(evalOut.Observed.TestsRun, "go test ./internal/retrieval -run TestImproveTestCompanionRecall -count=1"))

	assert.Truef(t, containsPath(evalOut.CheckpointSummary.EvidenceOnlyGitDiffFiles, "internal/commands/task.go"),
		"expected evidence-only git diff summary, got %#v", evalOut.CheckpointSummary)
	assert.Truef(t, containsString(evalOut.CheckpointSummary.EvidenceOnlyTestCommands, "go test ./internal/retrieval -run TestImproveTestCompanionRecall -count=1"),
		"expected evidence-only test command summary, got %#v", evalOut.CheckpointSummary)
	assert.Equal(t, 1, evalOut.CheckpointSummary.JSONRecords)
	assert.Equal(t, 0, evalOut.CheckpointSummary.MarkdownFallbacks)

}

func TestTask_GitChangedFileParserIgnoresWarnings(t *testing.T) {
	files := appendTaskGitChangedFiles(nil, strings.Join([]string{
		"warning: in the working copy of 'internal/retrieval/ranking.go', LF will be replaced by CRLF the next time Git touches it",
		"internal/retrieval/ranking.go",
		"",
		".devspecs/tasks/p02/P00-index.md",
	}, "\n"))
	assert.False(t, containsPath(files, "warning: in the working copy of 'internal/retrieval/ranking.go', LF will be replaced by CRLF the next time Git touches it"))

	assert.Truef(t, containsPath(files, "internal/retrieval/ranking.go"),
		"expected real changed file, got %#v", files)
	assert.Truef(t, containsPath(files, ".devspecs/tasks/p02/P00-index.md"),
		"expected task artifact changed file, got %#v", files)

}

func TestTask_EvaluateExcludesTaskWorkspaceReadsFromMissMetrics(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)
	taskID := "workspace-filter-test"
	workspace := filepath.Join(repoDir, ".devspecs", "tasks", taskID)
	mustMkdirAll(t, filepath.Join(workspace, "checkpoints"))
	manifest := taskManifest{
		TaskID:    taskID,
		Query:     "workspace filtering",
		Status:    "packed",
		CreatedAt: "2026-06-04T00:00:00Z",
		RepoRoot:  repoDir,
		Workspace: filepath.ToSlash(workspace),
		Artifacts: taskArtifactPaths{
			Index:      "A00-index.md",
			FirstSlice: "A01-workspace-filter-plan.md",
			Result:     "A01-workspace-filter-result.md",
			Slices: []taskSliceArtifact{{
				ID:     "A01",
				Title:  "workspace filter",
				Plan:   "A01-workspace-filter-plan.md",
				Result: "A01-workspace-filter-result.md",
			}},
		},
		Predicted: taskPredictedContext{
			PrimaryFiles: []taskPredictedFile{{Path: "internal/commands/task.go"}},
		},
		Confidence: taskConfidence{
			PrimaryFileConfidence:  "high",
			TestCoverageConfidence: "low",
			DocsConfigConfidence:   "low",
			GitReceiptConfidence:   "low",
			NoiseRisk:              "low",
			PackCompleteness:       "high",
		},
	}
	{
		err := writeTaskManifest(filepath.Join(workspace, taskManifestFilename), manifest)
		require.NoError(t, err)
	}

	workspaceIndex := filepath.ToSlash(filepath.Join(".devspecs", "tasks", taskID, "A00-index.md"))
	workspacePlan := filepath.Join(workspace, "A01-workspace-filter-plan.md")
	workspaceJSON := filepath.Join(workspace, taskManifestFilename)
	record := taskCheckpointRecord{
		SchemaVersion: 1,
		TaskID:        taskID,
		Query:         manifest.Query,
		Stage:         "implemented",
		Decision:      "improve",
		CreatedAt:     "2026-06-04T00:00:01Z",
		FilesRead: []string{
			workspaceIndex,
			workspaceJSON,
			"internal/commands/task.go",
		},
		TestsRead: []string{
			workspacePlan,
		},
		MissedFiles: []string{
			workspaceIndex,
			"A01-workspace-filter-plan.md",
			"internal/commands/task_evaluate.go",
		},
	}
	{
		err := writeTaskCheckpointRecord(filepath.Join(workspace, "checkpoints", "20260604-000001-implemented.json"), record)
		require.NoError(t, err)
	}

	evalCmd := NewTaskCmd()
	evalCmd.SetArgs([]string{"evaluate", taskID, "--dir", ".devspecs/tasks", "--json"})
	evalBuf := &bytes.Buffer{}
	evalCmd.SetOut(evalBuf)
	{
		err := evalCmd.Execute()
		require.NoError(t, err)
	}

	var evalOut taskEvaluationOutput
	{
		err := json.Unmarshal(evalBuf.Bytes(), &evalOut)
		require.NoErrorf(t, err,
			"evaluate json: %v\n%s", err, evalBuf.String())
	}
	assert.Truef(t, containsPath(evalOut.Observed.FilesRead, workspaceIndex),
		"raw observed context should keep task workspace read, got %#v", evalOut.Observed.FilesRead)
	assert.Truef(t, containsPath(evalOut.Observed.FilesRead, workspaceJSON),
		"raw observed context should keep absolute task workspace read, got %#v", evalOut.Observed.FilesRead)
	assert.False(t, containsPath(evalOut.Misses, workspaceIndex))
	assert.False(t, containsPath(evalOut.Misses, workspacePlan))
	assert.False(t, containsPath(evalOut.Misses, "A01-workspace-filter-plan.md"))

	assert.Truef(t, containsPath(evalOut.Hits, "internal/commands/task.go"),
		"normal implementation file should still count as hit, got %#v", evalOut.Hits)
	assert.Truef(t, containsPath(evalOut.Misses, "internal/commands/task_evaluate.go"),
		"normal explicit missed file should still count, got %#v", evalOut.Misses)
	assert.Equalf(t, "1/1", evalOut.Metrics.CriticalPathRecall,
		"critical path recall should ignore task workspace reads, got %q", evalOut.Metrics.CriticalPathRecall)
	assert.Truef(t, evalOut.ConfidenceMismatch,
		"normal miss should still drive confidence mismatch when initial completeness is high")

}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	{
		err := os.MkdirAll(path, 0o755)
		require.NoError(t, err)
	}

}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()
	{
		err := os.WriteFile(path, []byte(body), 0o644)
		require.NoError(t, err)
	}

}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}

func assertNoTrailingWhitespace(t *testing.T, label, body string) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSuffix(line, "\r")
		assert.False(t, strings.HasSuffix(line, " "))
		assert.False(t, strings.HasSuffix(line, "\t"))

	}
}

func writeExistingTaskSeriesRange(t *testing.T, repoDir, last string) {
	t.Helper()
	for series := "A"; ; series = nextTaskAlphaSeries(series) {
		writeExistingTaskSeries(t, repoDir, "existing-series-"+strings.ToLower(series), series)
		if series == last {
			return
		}
		assert.NotEmpty(t, series)
		assert.LessOrEqual(t, len(series), 4)

	}
}

func writeExistingTaskSeries(t *testing.T, repoDir, taskID, series string) {
	t.Helper()
	workspace := filepath.Join(repoDir, "devspecs", "tasks", taskID)
	mustMkdirAll(t, workspace)
	manifest := taskManifest{
		TaskID:    taskID,
		Series:    series,
		Query:     "existing task series " + series,
		Status:    "packed",
		CreatedAt: "2026-06-09T00:00:00Z",
		RepoRoot:  repoDir,
		Workspace: filepath.ToSlash(workspace),
		Artifacts: taskArtifactPaths{
			Series: series,
			Index:  taskSeriesIndexFilename(series),
			Slices: []taskSliceArtifact{
				taskSliceArtifactWithSlug(series+"01", "existing slice", "existing-slice", "slice", "", ""),
			},
		},
	}
	{
		err := writeTaskManifest(filepath.Join(workspace, taskManifestFilename), manifest)
		require.NoError(t, err)
	}

}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func taskRiskCardByID(cards []taskRiskCard, id string) *taskRiskCard {
	for i := range cards {
		if cards[i].ID == id {
			return &cards[i]
		}
	}
	return nil
}

func taskAdvisoryFileByPath(files []taskAdvisoryFile, path string) *taskAdvisoryFile {
	path = filepath.ToSlash(path)
	for i := range files {
		if filepath.ToSlash(files[i].Path) == path {
			return &files[i]
		}
	}
	return nil
}

func taskTestRepoID(t *testing.T, db *store.DB, repoDir string) string {
	t.Helper()
	var repoID string
	{
		err := db.QueryRow("SELECT id FROM repos WHERE root_path = ?", repoDir).Scan(&repoID)
		require.NoError(t, err)
	}

	return repoID
}

func taskWarningsContainPath(warnings []taskFreshnessWarning, want string) bool {
	want = filepath.ToSlash(want)
	for _, warning := range warnings {
		if filepath.ToSlash(warning.Path) == want {
			return true
		}
	}
	return false
}

func taskArtifactFreshnessContainsPath(warnings []taskArtifactFreshness, want string) bool {
	return taskArtifactFreshnessByPath(warnings, want) != nil
}

func taskArtifactFreshnessByPath(warnings []taskArtifactFreshness, want string) *taskArtifactFreshness {
	want = filepath.ToSlash(want)
	for i := range warnings {
		warning := &warnings[i]
		if filepath.ToSlash(warning.Path) == want {
			return warning
		}
	}
	return nil
}

func taskArtifactRefreshContainsPath(artifacts []taskArtifactRefresh, want string) bool {
	want = filepath.ToSlash(want)
	for _, artifact := range artifacts {
		if filepath.ToSlash(artifact.Path) == want {
			return true
		}
	}
	return false
}
