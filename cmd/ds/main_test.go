package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRootCmd_Version(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--version"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	require.NoError(t, cmd.Execute())

	got := buf.String()
	assert.Contains(t, got, "dev",
		"expected version output to contain 'dev', got %q", got)
	assert.Contains(t, got, "none",
		"expected version output to contain 'none', got %q", got)
	assert.Contains(t, got, "unknown",
		"expected version output to contain 'unknown', got %q", got)

}

func TestRootCmd_HelpMentionsTelemetryPrivacy(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	require.NoError(t, cmd.Execute())

	got := buf.String()
	for _, want := range []string{
		"Telemetry:",
		"minimal anonymous usage counts",
		"never sends repo names, file paths, git remotes",
		"raw queries",
		"DEVSPECS_TELEMETRY=0",
	} {
		assert.Contains(t, got, want,
			"expected help output to contain %q, got %q", want, got)

	}
}

func TestRootCmd_HelpCentersTaskWorkflow(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	require.NoError(t, cmd.Execute())

	got := buf.String()
	for _, want := range []string{
		"Default workflow:",
		"use ds task to create bounded task workspaces",
		"Use ds apply or ds apply",
		"Human orientation:",
		"start with ds recent to recover the local thread",
		"Use ds find for a focused question",
		"Human work setup:",
		"use ds task for repo-local bounded work",
		"AI execution:",
		"agents should consume bounded prompts with ds apply",
		"Setup:",
		"run ds init once per repo",
		"adapter files for ds task and ds apply",
		"Diagnostic layer:",
		"start with ds recent when the target is unclear",
		"Use ds find",
		"ds map",
	} {
		assert.Contains(t, got, want,
			"expected help output to contain %q, got %q", want, got)

	}
}

func TestRootCmd_HelpGroupsCommandsByActor(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	require.NoError(t, cmd.Execute())

	got := buf.String()
	for _, want := range []string{
		"Human orientation",
		"  recent      Show recently active local git topics",
		"  find        Build packed context for a query",
		"Human work setup",
		"  task        Create a bounded task workspace",
		"  workspace   Manage workspace-level DevSpecs artifacts",
		"AI execution",
		"  apply       Emit a one-slice DevSpecs apply prompt",
		"Advanced and maintenance",
		"  scan        Rescan repository intent docs, source, tests, and git evidence",
		"  prune       Remove stale and redundant data from the local index",
	} {
		require.Contains(t, got, want,
			"expected grouped help to contain %q, got:\n%s", want, got)

	}
	assertHelpOrder(t, got,
		"Human orientation",
		"Human work setup",
		"AI execution",
		"Advanced and maintenance",
		"Additional Commands",
	)
}

func assertHelpOrder(t *testing.T, body string, ordered ...string) {
	t.Helper()
	last := -1
	for _, want := range ordered {
		idx := strings.Index(body, want)
		require.False(t, idx < 0,
			"help output missing %q:\n%s", want, body)
		require.False(t, idx <= last,
			"help output order wrong at %q:\n%s", want, body)

		last = idx
	}
}

func TestRootCmd_TLDRRegistered(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"tldr", "hotfix"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	require.NoError(t, cmd.Execute())

	got := buf.String()
	require.Contains(t, got, "Hotfix / Small Bug",
		"expected tldr hotfix output, got %q", got)

}

func TestRootCmd_ApplyRegistered(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"apply", "--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	require.NoError(t, cmd.Execute())

	got := buf.String()
	for _, want := range []string{
		"Emit an agent prompt for exactly one DevSpecs task target.",
		"apply [task-id|target]",
	} {
		require.Contains(t, got, want,
			"expected apply help to contain %q, got:\n%s", want, got)

	}
}

func TestRootCmd_ComposeHelp_ExplainsTypeAndFormatBoundaries(t *testing.T) {
	help := executeRootHelp(t, "compose", "adr", "--help")

	assert.Contains(t, help, "Create an ADR after a meaningful technical direction is settled.")
	assert.Contains(t, help, "nygard")
	assert.Contains(t, help, "madr")
	assert.Contains(t, help, "y-statement")
	assert.Contains(t, help, "outcome-first")
	assert.Contains(t, help, "iso-42010")
}

func TestRootCmd_FindHelp_DescribesFocusedContextRole(t *testing.T) {
	help := executeRootHelp(t, "find", "--help")

	assert.Contains(t, help, "Build agent-readable packed context for a focused question.")
	assert.Contains(t, help, "discovering relevant source, tests, docs, receipts, or")
	assert.Contains(t, help, "It does not report task lifecycle state")
}

func TestRootCmd_TaskStatusHelp_DescribesLifecycleRole(t *testing.T) {
	help := executeRootHelp(t, "task", "status", "--help")

	assert.Contains(t, help, "Show lifecycle state for an existing DevSpecs task.")
	assert.Contains(t, help, "inspect task, slice, follow-up, checkpoint, and decision")
	assert.Contains(t, help, "It does not discover new source or docs")
}

func TestRootCmd_WorkspaceTraceHelp_DescribesKnownLinkRole(t *testing.T) {
	help := executeRootHelp(t, "workspace", "trace", "--help")

	assert.Contains(t, help, "Trace a known workspace change or repo task to linked repo-local slices.")
	assert.Contains(t, help, "Use ds workspace trace only when you already know")
	assert.Contains(t, help, "status describes change/task")
	assert.Contains(t, help, "index_status describes local index capture state")
}

func TestRootCmd_WorkspaceHelp_RegistersWorkspaceNamespace(t *testing.T) {
	help := executeRootHelp(t, "workspace", "--help")

	assert.Contains(t, help, "Manage workspace-level DevSpecs artifacts")
}

func TestRootCmd_WSHelp_RegistersWorkspaceAlias(t *testing.T) {
	help := executeRootHelp(t, "ws", "--help")

	assert.Contains(t, help, "Manage workspace-level DevSpecs artifacts")
}

func TestRootCmd_WorkspaceChangeHelp_RegistersChangeNamespace(t *testing.T) {
	help := executeRootHelp(t, "workspace", "change", "--help")

	assert.Contains(t, help, "Manage workspace-level change artifacts")
}

func TestRootCmd_WorkspaceSliceHelp_RegistersSliceCommand(t *testing.T) {
	help := executeRootHelp(t, "workspace", "slice", "--help")

	assert.Contains(t, help, "Create repo-local task slices from workspace changes")
}

func TestRootCmd_WorkspaceTraceHelp_RegistersTraceCommand(t *testing.T) {
	help := executeRootHelp(t, "workspace", "trace", "--help")

	assert.Contains(t, help, "Trace a known workspace change or repo task to linked repo-local slices")
}

func TestRootCmd_ChangeHelp_RegistersCompatibilityAlias(t *testing.T) {
	help := executeRootHelp(t, "change", "--help")

	assert.Contains(t, help, "Compatibility alias. Prefer `ds workspace change`")
}

func TestRootCmd_SliceHelp_RegistersCompatibilityAlias(t *testing.T) {
	help := executeRootHelp(t, "slice", "--help")

	assert.Contains(t, help, "Compatibility alias. Prefer `ds workspace slice`")
}

func TestRootCmd_TraceHelp_RegistersCompatibilityAlias(t *testing.T) {
	help := executeRootHelp(t, "trace", "--help")

	assert.Contains(t, help, "Compatibility alias. Prefer `ds workspace trace`")
}

func TestRootCmd_WSShowCompatibilityAlias_DispatchesToWorkspaceShow(t *testing.T) {
	root := setupRootWorkspaceFixture(t)
	executeRootJSON(t, "workspace", "init", root, "--json")

	output := executeRootJSON(t, "ws", "show", "--workspace", root, "--json")

	assert.Equal(t, root, stringField(t, output, "workspace_root"))
}

func TestRootCmd_ChangeCreateCompatibilityAlias_DispatchesToWorkspaceChangeCreate(t *testing.T) {
	root := setupRootWorkspaceFixture(t)
	executeRootJSON(t, "workspace", "init", root, "--json")

	output := executeRootJSON(t,
		"change", "create", "Customer export",
		"--workspace", root,
		"--repos", "backend,frontend",
		"--json",
	)

	assert.Equal(t, "EAG-C001", stringField(t, output, "change_id"))
}

func TestRootCmd_SliceCreateCompatibilityAlias_DispatchesToWorkspaceSliceCreate(t *testing.T) {
	root := setupRootWorkspaceFixture(t)
	executeRootJSON(t, "workspace", "init", root, "--json")
	executeRootJSON(t,
		"workspace", "change", "create", "Customer export",
		"--workspace", root,
		"--repos", "backend,frontend",
		"--json",
	)

	output := executeRootJSON(t,
		"slice", "create", "EAG-C001",
		"--workspace", root,
		"--repo", "backend",
		"--name", "Backend API",
		"--no-refresh",
		"--index=false",
		"--json",
	)

	assert.Equal(t, "eag-c001-backend", stringField(t, output, "task_id"))
	assert.Equal(t, "backend", stringField(t, output, "repo_alias"))
	assert.Equal(t, "A01", stringField(t, output, "target"))
	taskWorkspace := stringField(t, output, "task_workspace")
	wantTaskPrefix := filepath.Join(root, "enalytics-backend", "devspecs", "tasks", "eag-c001-backend")
	assert.True(t, strings.HasPrefix(taskWorkspace, wantTaskPrefix),
		"task workspace = %q, want under %q", taskWorkspace, wantTaskPrefix)
}

func TestRootCmd_TraceCompatibilityAlias_DispatchesToWorkspaceTrace(t *testing.T) {
	root := setupRootWorkspaceFixture(t)
	executeRootJSON(t, "workspace", "init", root, "--json")
	executeRootJSON(t,
		"workspace", "change", "create", "Customer export",
		"--workspace", root,
		"--repos", "backend,frontend",
		"--json",
	)
	executeRootJSON(t,
		"workspace", "slice", "create", "EAG-C001",
		"--workspace", root,
		"--repo", "backend",
		"--name", "Backend API",
		"--no-refresh",
		"--index=false",
		"--json",
	)

	output := executeRootJSON(t, "trace", "EAG-C001", "--workspace", root, "--json")

	assert.Equal(t, "workspace_change", stringField(t, output, "kind"))
	assert.Equal(t, "EAG-C001", stringField(t, output, "change_id"))
	slices, ok := output["slices"].([]any)
	require.True(t, ok, "trace slices = %#v, want an array", output["slices"])
	require.Len(t, slices, 1)
}

func TestRootCmd_PublicHelpHidesInternalCommands(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	require.NoError(t, cmd.Execute())

	got := buf.String()
	for _, hidden := range []string{
		"  capture     ",
		"  criteria    ",
		"  change      ",
		"  eval        ",
		"  link        ",
		"  list        ",
		"  resolve     ",
		"  resume      ",
		"  slice       ",
		"  status      ",
		"  tag         ",
		"  todos       ",
		"  trace       ",
		"  untag       ",
	} {
		require.NotContains(t, got, hidden,
			"public help should hide internal command %q, got:\n%s", strings.TrimSpace(hidden), got)

	}
}

func TestRootCmd_ListNotRegistered(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	require.Error(t, err,
		"expected ds list to be unavailable")

	got := buf.String()
	require.NotContains(t, got, "List indexed artifacts",
		"ds list should not dispatch to artifact list command, got:\n%s", got)
	require.Contains(t, err.Error(), "unknown command",
		"expected unknown command error, got %v", err)

}

func TestRootCmd_LiveSurfaceMatchesCLISurfaceSpec(t *testing.T) {
	spec := readCLISurfaceSpec(t)
	root := newRootCmd()

	assertCommandSurface(t, "ds", root, spec.Tree["ds"].Children, spec.HiddenTree["ds"].Children, spec.RemovedTree["ds"].Children)

	task := mustFindCommand(t, root, "task")
	assertCommandSurface(t, "ds task", task, spec.Tree["ds task"].Children, nil, nil)
	assertCommandSurface(t, "ds task slice", mustFindCommand(t, task, "slice"), spec.Tree["ds task slice"].Children, nil, nil)
	assertCommandSurface(t, "ds task iteration", mustFindCommand(t, task, "iteration"), spec.Tree["ds task iteration"].Children, nil, nil)

	workspace := mustFindCommand(t, root, "workspace")
	assertCommandSurface(t, "ds workspace", workspace, spec.Tree["ds workspace"].Children, nil, nil)
	assertCommandSurface(t, "ds workspace change", mustFindCommand(t, workspace, "change"), spec.Tree["ds workspace change"].Children, nil, nil)
	assertCommandSurface(t, "ds workspace slice", mustFindCommand(t, workspace, "slice"), spec.Tree["ds workspace slice"].Children, nil, nil)

	config := mustFindCommand(t, root, "config")
	assertCommandSurface(t, "ds config", config, spec.Tree["ds config"].Children, nil, nil)
}

func executeRootJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	out := executeRoot(t, args...)
	var decoded map[string]any

	require.NoError(t, json.Unmarshal([]byte(out), &decoded),
		"decode json for %v:\n%s", args, out)

	return decoded
}

func executeRoot(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs(args)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	require.NoError(t, cmd.Execute(),
		"execute ds %v:\n%s", args, buf.String())

	return buf.String()
}

type cliSurfaceSpec struct {
	Tree        map[string]cliSurfaceNode `yaml:"tree"`
	HiddenTree  map[string]cliSurfaceNode `yaml:"hidden_tree"`
	RemovedTree map[string]cliSurfaceNode `yaml:"removed_tree"`
}

type cliSurfaceNode struct {
	Status   string                    `yaml:"status"`
	Aliases  []string                  `yaml:"aliases"`
	Children map[string]cliSurfaceNode `yaml:"children"`
}

func readCLISurfaceSpec(t *testing.T) cliSurfaceSpec {
	t.Helper()
	var data []byte
	var err error
	for _, path := range []string{
		filepath.Join("specs", "cli-surface.yaml"),
		filepath.Join("..", "..", "specs", "cli-surface.yaml"),
	} {
		data, err = os.ReadFile(path)
		if err == nil {
			var spec cliSurfaceSpec

			require.NoError(t, yaml.Unmarshal(data, &spec),
				"parse %s", path)

			return spec
		}
	}
	require.NoError(t, err, "read specs/cli-surface.yaml")
	return cliSurfaceSpec{}
}

func assertCommandSurface(t *testing.T, prefix string, parent *cobra.Command, public, hidden, removed map[string]cliSurfaceNode) {
	t.Helper()
	if public == nil {
		public = map[string]cliSurfaceNode{}
	}
	if hidden == nil {
		hidden = map[string]cliSurfaceNode{}
	}
	if removed == nil {
		removed = map[string]cliSurfaceNode{}
	}

	visibleExpected := map[string]bool{}
	for name, node := range public {
		cmd := findCommand(parent, name)
		if cmd == nil && node.Status == "cobra_builtin" {
			continue
		}
		require.NotNil(t, cmd,
			"%s missing command %q from cli-surface.yaml", parent.CommandPath(), name)

		if isSurfaceHidden(node.Status) {
			require.True(t, cmd.Hidden,
				"%s %s should be hidden per cli-surface.yaml status %q", prefix, name, node.Status)

		} else {
			visibleExpected[name] = true
			require.False(t, cmd.Hidden,
				"%s %s should be visible per cli-surface.yaml status %q", prefix, name, node.Status)

		}
		assertAliases(t, prefix+" "+name, cmd, node.Aliases)
	}
	for name, node := range hidden {
		cmd := mustFindCommand(t, parent, name)
		require.True(t, cmd.Hidden,
			"%s %s should be hidden per cli-surface.yaml hidden_tree", prefix, name)

		assertAliases(t, prefix+" "+name, cmd, node.Aliases)
	}
	for name := range removed {
		cmd := findCommand(parent, name)
		require.Nil(t, cmd,
			"%s %s is marked removed in cli-surface.yaml but is still registered", prefix, name)
	}

	var unexpected []string
	for _, cmd := range parent.Commands() {
		if cmd.Hidden || cmd.Name() == "help" {
			continue
		}
		if !visibleExpected[cmd.Name()] {
			unexpected = append(unexpected, cmd.Name())
		}
	}
	sort.Strings(unexpected)
	require.False(t, len(unexpected) > 0,
		"%s has visible commands not listed as public in cli-surface.yaml: %s", prefix, strings.Join(unexpected, ", "))

}

func isSurfaceHidden(status string) bool {
	switch status {
	case "hidden", "hidden_compat", "removed":
		return true
	default:
		return false
	}
}

func assertAliases(t *testing.T, commandPath string, cmd *cobra.Command, aliases []string) {
	t.Helper()
	for _, alias := range aliases {
		alias = strings.TrimSpace(strings.TrimPrefix(alias, "ds "))
		if strings.Contains(alias, " ") {
			parts := strings.Fields(alias)
			alias = parts[len(parts)-1]
		}
		if alias == "" {
			continue
		}
		require.True(t, containsString(cmd.Aliases, alias),
			"%s missing alias %q from cli-surface.yaml; aliases=%v", commandPath, alias, cmd.Aliases)

	}
}

func mustFindCommand(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
	t.Helper()
	cmd := findCommand(parent, name)
	require.NotNil(t, cmd,
		"%s missing command %q from cli-surface.yaml", parent.CommandPath(), name)

	return cmd
}

func findCommand(parent *cobra.Command, name string) *cobra.Command {
	for _, cmd := range parent.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func setupRootWorkspaceFixture(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	root := filepath.Join(tmp, "eag-stg")
	mkdirAll(t, filepath.Join(root, ".git"))
	for _, dir := range []string{"enalytics-backend", "enalytics-frontend"} {
		mkdirAll(t, filepath.Join(root, dir, ".git"))
		writeFile(t, filepath.Join(root, dir, "README.md"), "# "+dir+"\n")
	}
	mkdirAll(t, filepath.Join(root, "enalytics-backend", "internal"))
	writeFile(t, filepath.Join(root, "enalytics-backend", "go.mod"), "module example.com/enalytics-backend\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "enalytics-backend", "internal", "service.go"), "package internal\n\nfunc CustomerExport() string { return \"ok\" }\n")
	writeFile(t, filepath.Join(root, "enalytics-frontend", "package.json"), "{\n  \"name\": \"enalytics-frontend\",\n  \"private\": true\n}\n")

	origWD, err := os.Getwd()
	require.NoError(t, err)

	require.NoError(t, os.Chdir(root))

	t.Cleanup(func() {
		if err := os.Chdir(origWD); err != nil {
			require.NoError(t, err)
		}
	})
	return root
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(path, 0o755))

}

func writeFile(t *testing.T, path, body string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))

}

func executeRootHelp(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs(args)
	buffer := &bytes.Buffer{}
	cmd.SetOut(buffer)
	require.NoError(t, cmd.Execute())
	return buffer.String()
}

func stringField(t *testing.T, decoded map[string]any, key string) string {
	t.Helper()
	got, ok := decoded[key].(string)
	require.True(t, ok,
		"json field %q = %#v, want string in %#v", key, decoded[key], decoded)

	return got
}
