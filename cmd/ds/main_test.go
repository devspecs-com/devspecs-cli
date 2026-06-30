package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootCmd_Version(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--version"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "dev") {
		t.Errorf("expected version output to contain 'dev', got %q", got)
	}
	if !strings.Contains(got, "none") {
		t.Errorf("expected version output to contain 'none', got %q", got)
	}
	if !strings.Contains(got, "unknown") {
		t.Errorf("expected version output to contain 'unknown', got %q", got)
	}
}

func TestRootCmd_HelpMentionsTelemetryPrivacy(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	for _, want := range []string{
		"Telemetry:",
		"minimal anonymous usage counts",
		"never sends repo names, file paths, git remotes",
		"raw queries",
		"DEVSPECS_TELEMETRY=0",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected help output to contain %q, got %q", want, got)
		}
	}
}

func TestRootCmd_HelpCentersTaskWorkflow(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	for _, want := range []string{
		"Default workflow:",
		"use ds task to create bounded task workspaces",
		"Use ds apply next or ds apply",
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
		if !strings.Contains(got, want) {
			t.Errorf("expected help output to contain %q, got %q", want, got)
		}
	}
}

func TestRootCmd_HelpGroupsCommandsByActor(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

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
		"  scan        Scan repository for specs, plans, and ADRs",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected grouped help to contain %q, got:\n%s", want, got)
		}
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
		if idx < 0 {
			t.Fatalf("help output missing %q:\n%s", want, body)
		}
		if idx <= last {
			t.Fatalf("help output order wrong at %q:\n%s", want, body)
		}
		last = idx
	}
}

func TestRootCmd_TLDRRegistered(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"tldr", "hotfix"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "Hotfix / Small Bug") {
		t.Fatalf("expected tldr hotfix output, got %q", got)
	}
}

func TestRootCmd_ApplyRegistered(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"apply", "--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	for _, want := range []string{
		"Emit an agent prompt for exactly one DevSpecs task target.",
		"apply <next|task-id|target>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected apply help to contain %q, got:\n%s", want, got)
		}
	}
}

func TestRootCmd_CommandRoleHelpDistinguishesFindStatusAndTrace(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{
			args: []string{"find", "--help"},
			want: []string{
				"Build agent-readable packed context for a focused question.",
				"discovering relevant source, tests, docs, receipts, or",
				"It does not report task lifecycle state",
			},
		},
		{
			args: []string{"task", "status", "--help"},
			want: []string{
				"Show lifecycle state for an existing DevSpecs task.",
				"inspect task, slice, iteration, checkpoint, and decision",
				"It does not discover new source or docs",
			},
		},
		{
			args: []string{"workspace", "trace", "--help"},
			want: []string{
				"Trace a known workspace change or repo task to linked repo-local slices.",
				"Use ds workspace trace only when you already know",
				"status describes change/task",
				"index_status describes local index capture state",
			},
		},
	} {
		cmd := newRootCmd()
		cmd.SetArgs(tc.args)
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		got := buf.String()
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Fatalf("expected %v help to contain %q, got:\n%s", tc.args, want, got)
			}
		}
	}
}

func TestRootCmd_WorkspaceNamespaceAndCompatibilityCommandsRegistered(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{args: []string{"workspace", "--help"}, want: "Manage workspace-level DevSpecs artifacts"},
		{args: []string{"workspace", "change", "--help"}, want: "Manage workspace-level change artifacts"},
		{args: []string{"workspace", "slice", "--help"}, want: "Create repo-local task slices from workspace changes"},
		{args: []string{"workspace", "trace", "--help"}, want: "Trace a known workspace change or repo task to linked repo-local slices"},
		{args: []string{"change", "--help"}, want: "Compatibility alias. Prefer `ds workspace change`"},
		{args: []string{"slice", "--help"}, want: "Compatibility alias. Prefer `ds workspace slice`"},
		{args: []string{"trace", "--help"}, want: "Compatibility alias. Prefer `ds workspace trace`"},
	} {
		cmd := newRootCmd()
		cmd.SetArgs(tc.args)
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), tc.want) {
			t.Fatalf("expected %v help to contain %q, got:\n%s", tc.args, tc.want, buf.String())
		}
	}
}

func TestRootCmd_HiddenWorkspaceCompatibilityAliasesDispatch(t *testing.T) {
	root := setupRootWorkspaceFixture(t)

	initOut := executeRootJSON(t, "workspace", "init", root, "--json")
	if got := stringField(t, initOut, "workspace_root"); got != root {
		t.Fatalf("workspace init root = %q, want %q", got, root)
	}

	changeOut := executeRootJSON(t,
		"change", "create", "Customer export",
		"--workspace", root,
		"--repos", "backend,frontend",
		"--json",
	)
	changeID := stringField(t, changeOut, "change_id")
	if changeID != "EAG-C001" {
		t.Fatalf("change id = %q, want EAG-C001", changeID)
	}

	sliceOut := executeRootJSON(t,
		"slice", "create", changeID,
		"--workspace", root,
		"--repo", "backend",
		"--name", "Backend API",
		"--no-refresh",
		"--index=false",
		"--json",
	)
	if got := stringField(t, sliceOut, "task_id"); got != "eag-c001-backend" {
		t.Fatalf("task id = %q, want eag-c001-backend", got)
	}
	if got := stringField(t, sliceOut, "repo_alias"); got != "backend" {
		t.Fatalf("repo alias = %q, want backend", got)
	}
	if got := stringField(t, sliceOut, "target"); got != "A01" {
		t.Fatalf("target = %q, want A01", got)
	}
	taskWorkspace := stringField(t, sliceOut, "task_workspace")
	wantTaskPrefix := filepath.Join(root, "enalytics-backend", "devspecs", "tasks", "eag-c001-backend")
	if !strings.HasPrefix(taskWorkspace, wantTaskPrefix) {
		t.Fatalf("task workspace = %q, want under %q", taskWorkspace, wantTaskPrefix)
	}

	traceOut := executeRootJSON(t, "trace", changeID, "--workspace", root, "--json")
	if got := stringField(t, traceOut, "kind"); got != "workspace_change" {
		t.Fatalf("trace kind = %q, want workspace_change", got)
	}
	if got := stringField(t, traceOut, "change_id"); got != changeID {
		t.Fatalf("trace change id = %q, want %q", got, changeID)
	}
	slices, ok := traceOut["slices"].([]any)
	if !ok || len(slices) != 1 {
		t.Fatalf("trace slices = %#v, want one linked slice", traceOut["slices"])
	}
}

func TestRootCmd_PublicHelpHidesInternalCommands(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

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
		if strings.Contains(got, hidden) {
			t.Fatalf("public help should hide internal command %q, got:\n%s", strings.TrimSpace(hidden), got)
		}
	}
}

func TestRootCmd_ListNotRegistered(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected ds list to be unavailable")
	}
	got := buf.String()
	if strings.Contains(got, "List indexed artifacts") {
		t.Fatalf("ds list should not dispatch to artifact list command, got:\n%s", got)
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func executeRootJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	out := executeRoot(t, args...)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode json for %v: %v\n%s", args, err, out)
	}
	return decoded
}

func executeRoot(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs(args)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute ds %v: %v\n%s", args, err, buf.String())
	}
	return buf.String()
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
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(origWD); err != nil {
			t.Fatal(err)
		}
	})
	return root
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stringField(t *testing.T, decoded map[string]any, key string) string {
	t.Helper()
	got, ok := decoded[key].(string)
	if !ok {
		t.Fatalf("json field %q = %#v, want string in %#v", key, decoded[key], decoded)
	}
	return got
}
