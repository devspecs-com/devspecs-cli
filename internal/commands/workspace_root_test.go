package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceRootWarningDetectsMultipleChildProjects(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceRootTestFile(t, root, "apps/api/package.json", `{"name":"api"}`)
	writeWorkspaceRootTestFile(t, root, "apps/web/package.json", `{"name":"web"}`)
	writeWorkspaceRootTestFile(t, root, "examples/demo/package.json", `{"name":"ignored-example"}`)

	warning := detectWorkspaceRootWarning(root, "scan")
	if warning == nil {
		t.Fatal("expected workspace root warning")
	}
	if warning.Kind != "workspace_root" {
		t.Fatalf("kind = %q, want workspace_root", warning.Kind)
	}
	joined := strings.Join(warning.CandidateRoots, "\n")
	for _, want := range []string{"apps/api", "apps/web"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing candidate %q in %#v", want, warning.CandidateRoots)
		}
	}
	if strings.Contains(joined, "examples/demo") {
		t.Fatalf("examples should not trigger workspace warning candidates: %#v", warning.CandidateRoots)
	}
}

func TestWorkspaceRootWarningSuppressesNormalGitRepoRoot(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceRootTestFile(t, root, ".git/HEAD", "ref: refs/heads/main\n")
	writeWorkspaceRootTestFile(t, root, "api/package.json", `{"name":"api"}`)
	writeWorkspaceRootTestFile(t, root, "signer/go.mod", "module example.com/signer\n")
	writeWorkspaceRootTestFile(t, root, "web/package.json", `{"name":"web"}`)

	warning := detectWorkspaceRootWarning(root, "map")
	if warning != nil {
		t.Fatalf("normal selected git repo root should not warn: %#v", warning)
	}
}

func TestWorkspaceRootWarningKeepsNestedGitRepoWarning(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceRootTestFile(t, root, ".git/HEAD", "ref: refs/heads/main\n")
	writeWorkspaceRootTestFile(t, root, "repos/api/.git/HEAD", "ref: refs/heads/main\n")
	writeWorkspaceRootTestFile(t, root, "repos/web/.git/HEAD", "ref: refs/heads/main\n")

	warning := detectWorkspaceRootWarning(root, "map")
	if warning == nil {
		t.Fatal("expected warning for root containing multiple nested git repos")
	}
	joined := strings.Join(warning.CandidateRoots, "\n")
	for _, want := range []string{"repos/api", "repos/web"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing nested git candidate %q in %#v", want, warning.CandidateRoots)
		}
	}
}

func TestWorkspaceRootGroupingPlanDefersDefaultParallelGrouping(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceRootTestFile(t, root, "packages/zeta/package.json", `{"name":"zeta"}`)
	writeWorkspaceRootTestFile(t, root, "packages/api/package.json", `{"name":"api"}`)
	writeWorkspaceRootTestFile(t, root, "services/worker/go.mod", "module example.com/worker\n")
	writeWorkspaceRootTestFile(t, root, "node_modules/pkg/package.json", `{"name":"noise"}`)
	writeWorkspaceRootTestFile(t, root, "examples/demo/package.json", `{"name":"ignored-example"}`)

	plan := evaluateWorkspaceRootGrouping(root, "task")
	if plan.DefaultAction != workspaceRootActionChooseOneRoot {
		t.Fatalf("default action = %q, want %q: %#v", plan.DefaultAction, workspaceRootActionChooseOneRoot, plan)
	}
	if plan.ParallelGrouping != workspaceRootParallelGroupingDeferDefault {
		t.Fatalf("parallel grouping = %q, want %q", plan.ParallelGrouping, workspaceRootParallelGroupingDeferDefault)
	}
	var got []string
	for _, group := range plan.CandidateRoots {
		got = append(got, group.RelPath)
		if group.AbsPath == "" || !filepath.IsAbs(group.AbsPath) {
			t.Fatalf("expected absolute group path for %#v", group)
		}
		if !strings.Contains(group.SuggestedCommand, "ds task ...") {
			t.Fatalf("expected task suggested command, got %#v", group)
		}
	}
	want := []string{"packages/api", "packages/zeta", "services/worker"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("candidate roots = %#v, want %#v", got, want)
	}
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
	if plan.DefaultAction != workspaceRootActionChooseOneRoot {
		t.Fatalf("default action = %q, want %q", plan.DefaultAction, workspaceRootActionChooseOneRoot)
	}
	apiCost := countWorkspaceRootTestFiles(t, filepath.Join(root, "packages", "api"))
	mergedCost := 0
	for _, group := range plan.CandidateRoots {
		mergedCost += countWorkspaceRootTestFiles(t, group.AbsPath)
	}
	if mergedCost <= apiCost {
		t.Fatalf("merged grouped traversal cost = %d, want greater than narrowed root cost %d", mergedCost, apiCost)
	}
	if plan.ParallelGrouping != workspaceRootParallelGroupingDeferDefault {
		t.Fatalf("parallel grouping = %q, want %q", plan.ParallelGrouping, workspaceRootParallelGroupingDeferDefault)
	}
}

func TestWorkspaceRootGroupingPlanCapsDeterministically(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"zulu", "alpha", "bravo", "echo", "delta", "charlie", "foxtrot"} {
		writeWorkspaceRootTestFile(t, root, filepath.ToSlash(filepath.Join("packages", name, "package.json")), `{"name":"test"}`)
	}

	plan := evaluateWorkspaceRootGrouping(root, "scan")
	if len(plan.CandidateRoots) != workspaceRootCandidateLimit {
		t.Fatalf("candidate count = %d, want %d: %#v", len(plan.CandidateRoots), workspaceRootCandidateLimit, plan.CandidateRoots)
	}
	var got []string
	for _, group := range plan.CandidateRoots {
		got = append(got, group.RelPath)
	}
	want := []string{"packages/alpha", "packages/bravo", "packages/charlie", "packages/delta", "packages/echo", "packages/foxtrot"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("candidate roots = %#v, want %#v", got, want)
	}
}

func TestWorkspaceRootGroupingPlanKeepsNormalGitRepoCurrentRoot(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceRootTestFile(t, root, ".git/HEAD", "ref: refs/heads/main\n")
	writeWorkspaceRootTestFile(t, root, "api/package.json", `{"name":"api"}`)
	writeWorkspaceRootTestFile(t, root, "web/package.json", `{"name":"web"}`)

	plan := evaluateWorkspaceRootGrouping(root, "map")
	if plan.DefaultAction != workspaceRootActionCurrentRoot {
		t.Fatalf("default action = %q, want %q: %#v", plan.DefaultAction, workspaceRootActionCurrentRoot, plan)
	}
	if plan.ParallelGrouping != workspaceRootParallelGroupingNotNeeded {
		t.Fatalf("parallel grouping = %q, want %q", plan.ParallelGrouping, workspaceRootParallelGroupingNotNeeded)
	}
	if len(plan.CandidateRoots) != 0 {
		t.Fatalf("normal git repo should not produce grouping roots: %#v", plan.CandidateRoots)
	}
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
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out struct {
		RootWarning *struct {
			Kind           string   `json:"kind"`
			CandidateRoots []string `json:"candidate_roots"`
		} `json:"root_warning"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("scan --json stdout should be valid JSON: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if out.RootWarning == nil {
		t.Fatalf("expected root_warning in JSON output:\n%s", stdout.String())
	}
	if out.RootWarning.Kind != "workspace_root" {
		t.Fatalf("root_warning.kind = %q", out.RootWarning.Kind)
	}
	if !strings.Contains(stderr.String(), "Workspace root warning") {
		t.Fatalf("expected warning on stderr before scan, got: %s", stderr.String())
	}
	if strings.Contains(stdout.String(), "Workspace root warning") {
		t.Fatalf("warning text leaked into JSON stdout:\n%s", stdout.String())
	}
}

func TestMapJSONAutoScanWarnsForWorkspaceRootWithoutBreakingStdout(t *testing.T) {
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
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "Workspace root warning") {
		t.Fatalf("expected map auto-scan workspace warning on stderr, got: %s", stderr.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("map --json stdout should remain valid JSON: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "Workspace root warning") {
		t.Fatalf("warning text leaked into JSON stdout:\n%s", stdout.String())
	}
}

func writeWorkspaceRootTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
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
	if err != nil {
		t.Fatal(err)
	}
	return count
}
