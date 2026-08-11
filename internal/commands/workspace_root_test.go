package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceRootWarningDetectsMultipleChildProjects(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceRootTestFile(t, root, "apps/api/package.json", `{"name":"api"}`)
	writeWorkspaceRootTestFile(t, root, "apps/web/package.json", `{"name":"web"}`)
	writeWorkspaceRootTestFile(t, root, "examples/demo/package.json", `{"name":"ignored-example"}`)

	warning := detectWorkspaceRootWarning(root, "scan")
	require.NotNil(t, warning,
		"expected workspace root warning")
	assert.Equal(t, "workspace_root", warning.Kind,
		"kind = %q, want workspace_root", warning.Kind)

	joined := strings.Join(warning.CandidateRoots, "\n")
	assert.Contains(t, joined, "apps/api",
		"missing candidate %q in %#v", "apps/api", warning.CandidateRoots)
	assert.Contains(t, joined, "apps/web",
		"missing candidate %q in %#v", "apps/web", warning.CandidateRoots)

	assert.NotContains(t, joined, "examples/demo",
		"examples should not trigger workspace warning candidates: %#v", warning.CandidateRoots)

}

func TestWorkspaceRootWarningSuppressesNormalGitRepoRoot(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceRootTestFile(t, root, ".git/HEAD", "ref: refs/heads/main\n")
	writeWorkspaceRootTestFile(t, root, "api/package.json", `{"name":"api"}`)
	writeWorkspaceRootTestFile(t, root, "signer/go.mod", "module example.com/signer\n")
	writeWorkspaceRootTestFile(t, root, "web/package.json", `{"name":"web"}`)

	warning := detectWorkspaceRootWarning(root, "map")
	require.Nil(t, warning,
		"normal selected git repo root should not warn: %#v", warning)

}

func TestWorkspaceRootWarningKeepsNestedGitRepoWarning(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceRootTestFile(t, root, ".git/HEAD", "ref: refs/heads/main\n")
	writeWorkspaceRootTestFile(t, root, "repos/api/.git/HEAD", "ref: refs/heads/main\n")
	writeWorkspaceRootTestFile(t, root, "repos/web/.git/HEAD", "ref: refs/heads/main\n")

	warning := detectWorkspaceRootWarning(root, "map")
	require.NotNil(t, warning,
		"expected warning for root containing multiple nested git repos")

	joined := strings.Join(warning.CandidateRoots, "\n")
	assert.Contains(t, joined, "repos/api",
		"missing nested git candidate %q in %#v", "repos/api", warning.CandidateRoots)
	assert.Contains(t, joined, "repos/web",
		"missing nested git candidate %q in %#v", "repos/web", warning.CandidateRoots)

}

func TestWorkspaceRootGroupingPlanDefersDefaultParallelGrouping(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceRootTestFile(t, root, "packages/zeta/package.json", `{"name":"zeta"}`)
	writeWorkspaceRootTestFile(t, root, "packages/api/package.json", `{"name":"api"}`)
	writeWorkspaceRootTestFile(t, root, "services/worker/go.mod", "module example.com/worker\n")
	writeWorkspaceRootTestFile(t, root, "node_modules/pkg/package.json", `{"name":"noise"}`)
	writeWorkspaceRootTestFile(t, root, "examples/demo/package.json", `{"name":"ignored-example"}`)

	plan := evaluateWorkspaceRootGrouping(root, "task")
	assert.Equal(t, workspaceRootActionChooseOneRoot, plan.DefaultAction,
		"default action = %q, want %q: %#v", plan.DefaultAction, workspaceRootActionChooseOneRoot, plan)
	assert.Equal(t, workspaceRootParallelGroupingDeferDefault, plan.ParallelGrouping,
		"parallel grouping = %q, want %q", plan.ParallelGrouping, workspaceRootParallelGroupingDeferDefault)

	var got []string
	for _, group := range plan.CandidateRoots {
		got = append(got, group.RelPath)
		assert.NotEqual(t, "", group.AbsPath, "expected absolute group path for %#v", group)
		assert.True(t, filepath.IsAbs(group.AbsPath), "expected absolute group path for %#v", group)
		assert.Contains(t, group.SuggestedCommand, "ds task ...",
			"expected task suggested command, got %#v", group)

	}
	want := []string{"packages/api", "packages/zeta", "services/worker"}
	assert.Equal(t, strings.Join(want, "\n"), strings.Join(got, "\n"),
		"candidate roots = %#v, want %#v", got, want)

}

func TestWorkspaceRootGroupingPlanShowsMergedGroupsAreBroaderThanNarrowedRoot(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceRootTestFile(t, root, "packages/api/package.json", `{"name":"api"}`)
	writeWorkspaceRootTestFile(t, root, "packages/api/plans/oauth.md", "# OAuth\n")
	writeWorkspaceRootTestFile(t, root, "packages/web/package.json", `{"name":"web"}`)
	writeWorkspaceRootTestFile(t, root, "packages/web/plans/theme.md", "# Theme\n")
	writeWorkspaceRootTestFile(t, root, "services/worker/go.mod", "module example.com/worker\n")
	writeWorkspaceRootTestFile(t, root, "services/worker/plans/jobs.md", "# Jobs\n")
	writeWorkspaceRootTestFile(t, root, "node_modules/pkg/noise.md", "# Noise\n")

	plan := evaluateWorkspaceRootGrouping(root, "scan")
	assert.Equal(t, workspaceRootActionChooseOneRoot, plan.DefaultAction,
		"default action = %q, want %q", plan.DefaultAction, workspaceRootActionChooseOneRoot)

	apiCost := countWorkspaceRootTestFiles(t, filepath.Join(root, "packages", "api"))
	mergedCost := 0
	for _, group := range plan.CandidateRoots {
		mergedCost += countWorkspaceRootTestFiles(t, group.AbsPath)
	}
	assert.Greater(t, mergedCost, apiCost,
		"merged grouped traversal cost = %d, want greater than narrowed root cost %d", mergedCost, apiCost)
	assert.Equal(t, workspaceRootParallelGroupingDeferDefault, plan.ParallelGrouping,
		"parallel grouping = %q, want %q", plan.ParallelGrouping, workspaceRootParallelGroupingDeferDefault)

}

func TestWorkspaceRootGroupingPlanCapsDeterministically(t *testing.T) {
	root := t.TempDir()
	{
		name := "zulu"

		writeWorkspaceRootTestFile(t, root, filepath.ToSlash(filepath.Join("packages", name, "package.json")), `{"name":"test"}`)

	}
	{
		name := "alpha"

		writeWorkspaceRootTestFile(t, root, filepath.ToSlash(filepath.Join("packages", name, "package.json")), `{"name":"test"}`)

	}
	{
		name := "bravo"

		writeWorkspaceRootTestFile(t, root, filepath.ToSlash(filepath.Join("packages", name, "package.json")), `{"name":"test"}`)

	}
	{
		name := "echo"

		writeWorkspaceRootTestFile(t, root, filepath.ToSlash(filepath.Join("packages", name, "package.json")), `{"name":"test"}`)

	}
	{
		name := "delta"

		writeWorkspaceRootTestFile(t, root, filepath.ToSlash(filepath.Join("packages", name, "package.json")), `{"name":"test"}`)

	}
	{
		name := "charlie"

		writeWorkspaceRootTestFile(t, root, filepath.ToSlash(filepath.Join("packages", name, "package.json")), `{"name":"test"}`)

	}
	{
		name := "foxtrot"

		writeWorkspaceRootTestFile(t, root, filepath.ToSlash(filepath.Join("packages", name, "package.json")), `{"name":"test"}`)

	}

	plan := evaluateWorkspaceRootGrouping(root, "scan")
	require.Len(t, plan.CandidateRoots, workspaceRootCandidateLimit,
		"candidate count = %d, want %d: %#v", len(plan.CandidateRoots), workspaceRootCandidateLimit, plan.CandidateRoots)

	var got []string
	for _, group := range plan.CandidateRoots {
		got = append(got, group.RelPath)
	}
	want := []string{"packages/alpha", "packages/bravo", "packages/charlie", "packages/delta", "packages/echo", "packages/foxtrot"}
	assert.Equal(t, strings.Join(want, "\n"), strings.Join(got, "\n"),
		"candidate roots = %#v, want %#v", got, want)

}

func TestWorkspaceRootGroupingPlanKeepsNormalGitRepoCurrentRoot(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceRootTestFile(t, root, ".git/HEAD", "ref: refs/heads/main\n")
	writeWorkspaceRootTestFile(t, root, "api/package.json", `{"name":"api"}`)
	writeWorkspaceRootTestFile(t, root, "web/package.json", `{"name":"web"}`)

	plan := evaluateWorkspaceRootGrouping(root, "map")
	assert.Equal(t, workspaceRootActionCurrentRoot, plan.DefaultAction,
		"default action = %q, want %q: %#v", plan.DefaultAction, workspaceRootActionCurrentRoot, plan)
	assert.Equal(t, workspaceRootParallelGroupingNotNeeded, plan.ParallelGrouping,
		"parallel grouping = %q, want %q", plan.ParallelGrouping, workspaceRootParallelGroupingNotNeeded)
	require.Empty(t, plan.CandidateRoots,
		"normal git repo should not produce grouping roots: %#v", plan.CandidateRoots)

}

func TestScanJSONIncludesWorkspaceRootWarning(t *testing.T) {
	root := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(t.TempDir(), "home"))
	writeWorkspaceRootTestFile(t, root, "apps/api/package.json", `{"name":"api"}`)
	writeWorkspaceRootTestFile(t, root, "apps/web/package.json", `{"name":"web"}`)
	writeWorkspaceRootTestFile(t, root, "plans/launch.md", "# Launch Plan\n")

	cmd := NewScanCmd()
	cmd.SetArgs([]string{"--json", "--path", root})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out struct {
		RootWarning *struct {
			Kind           string   `json:"kind"`
			CandidateRoots []string `json:"candidate_roots"`
		} `json:"root_warning"`
	}
	{
		err := json.Unmarshal(stdout.Bytes(), &out)
		require.NoError(t, err,
			"scan --json stdout should be valid JSON: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	require.NotNil(t, out.RootWarning,
		"expected root_warning in JSON output:\n%s", stdout.String())
	assert.Equal(t, "workspace_root", out.RootWarning.Kind,
		"root_warning.kind = %q", out.RootWarning.Kind)
	assert.Contains(t, stderr.String(), "Workspace root warning",
		"expected warning on stderr before scan, got: %s", stderr.String())
	assert.NotContains(t, stdout.String(), "Workspace root warning",
		"warning text leaked into JSON stdout:\n%s", stdout.String())

}

func TestMapJSONAutoScanSuppressesWorkspaceRootWarningStderr(t *testing.T) {
	root := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", filepath.Join(t.TempDir(), "home"))
	writeWorkspaceRootTestFile(t, root, "repos/api/.git/HEAD", "ref: refs/heads/main\n")
	writeWorkspaceRootTestFile(t, root, "repos/web/.git/HEAD", "ref: refs/heads/main\n")
	writeWorkspaceRootTestFile(t, root, "plans/credentials-plan.md", "# Credentials Rotation\n\nRotate credentials.\n")

	cmd := NewMapCmd()
	cmd.SetArgs([]string{"--json", "--path", root})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.Equal(t, 0, stderr.Len(),
		"map --json should suppress workspace-root warning stderr, got: %s", stderr.String())

	var payload map[string]any
	{
		err := json.Unmarshal(stdout.Bytes(), &payload)
		require.NoError(t, err,
			"map --json stdout should remain valid JSON: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	assert.NotContains(t, stdout.String(), "Workspace root warning",
		"warning text leaked into JSON stdout:\n%s", stdout.String())

}

func writeWorkspaceRootTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	{
		err := os.MkdirAll(filepath.Dir(path), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(path, []byte(body), 0o644)
		require.NoError(t, err)
	}

}

func countWorkspaceRootTestFiles(t *testing.T, root string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if workspaceRootSkipDirs[strings.ToLower(d.Name())] {
				return filepath.SkipDir
			}
			return nil
		}
		count++
		return nil
	})
	require.NoError(t, err)

	return count
}
