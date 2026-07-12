package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceEAGSTGEndToEndWorkflow(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	workspace := runWorkspaceInitJSON(t, root)
	change := runChangeCreateJSON(t, "Customer export across frontend/backend", root, "backend,frontend,database,prefect")

	for _, path := range []string{
		filepath.Join(root, "devspecs", "workspace.yaml"),
		filepath.Join(root, "devspecs", "workspace.md"),
		change.ChangePath,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected workspace artifact %s: %v", path, err)
		}
	}
	if workspace.WorkspaceRoot != root {
		t.Fatalf("workspace root = %q, want %q", workspace.WorkspaceRoot, root)
	}

	slices := map[string]sliceCreateOutput{
		"backend":  runSliceCreateJSON(t, root, change.ChangeID, "backend", "Backend API"),
		"frontend": runSliceCreateJSON(t, root, change.ChangeID, "frontend", "Frontend UI"),
		"database": runSliceCreateJSON(t, root, change.ChangeID, "database", "Database check"),
		"prefect":  runSliceCreateJSON(t, root, change.ChangeID, "prefect", "Prefect check"),
	}
	for alias, out := range slices {
		if !strings.HasPrefix(out.TaskWorkspace, filepath.Join(out.RepoRoot, "devspecs", "tasks", out.TaskID)) {
			t.Fatalf("%s task workspace = %q, want under %q", alias, out.TaskWorkspace, out.RepoRoot)
		}
		if _, err := os.Stat(filepath.Join(root, "devspecs", "tasks", out.TaskID)); !os.IsNotExist(err) {
			t.Fatalf("%s task unexpectedly written under umbrella root: %v", alias, err)
		}
		manifestBody := mustReadFile(t, out.ManifestPath)
		for _, want := range []string{
			`"workspace_id": "eag-stg"`,
			`"parent_change": "EAG-C001"`,
			`"repo_alias": "` + alias + `"`,
		} {
			if !strings.Contains(manifestBody, want) {
				t.Fatalf("%s manifest missing %q:\n%s", alias, want, manifestBody)
			}
		}
	}

	backend := slices["backend"]
	show := runTaskShowJSON(t, backend.TaskID, "--repo", backend.RepoRoot, "--json")
	if show.WorkspaceID != "eag-stg" || show.ParentChange != change.ChangeID || show.RepoAlias != "backend" {
		t.Fatalf("task show workspace link = %#v", show)
	}
	apply, err := runApplyJSON(t, []string{backend.TaskID, "--repo", backend.RepoRoot, "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if apply.TaskID != backend.TaskID || apply.Target != backend.Target {
		t.Fatalf("apply output = %#v", apply)
	}
	if !strings.Contains(apply.Prompt, "ds task checkpoint "+backend.TaskID+" --target "+backend.Target+" --repo ") {
		t.Fatalf("apply prompt missing repo-aware checkpoint command:\n%s", apply.Prompt)
	}

	runTaskCommand(t, "checkpoint", backend.TaskID,
		"--repo", backend.RepoRoot,
		"--target", backend.Target,
		"--stage", "validated",
		"--decision", "promote",
		"--test-run", "echo backend-smoke",
		"--index=false",
		"--json",
	)
	trace := runTraceJSON(t, change.ChangeID, "--workspace", root, "--json")
	if trace.Status != traceStatusIncomplete || len(trace.Slices) != 4 {
		t.Fatalf("trace after one completed slice = %#v", trace)
	}

	for _, alias := range []string{"frontend", "database", "prefect"} {
		out := slices[alias]
		runTaskCommand(t, "decide", out.TaskID,
			"--repo", out.RepoRoot,
			"--target", out.Target,
			"--decision", "block",
			"--index=false",
			"--json",
		)
	}
	trace = runTraceJSON(t, change.ChangeID, "--workspace", root, "--json")
	if trace.Status != traceStatusComplete {
		t.Fatalf("trace status after required repos completed or ruled out = %q, trace = %#v", trace.Status, trace)
	}

	duplicate := NewWorkspaceCmd()
	duplicate.SetArgs([]string{
		"slice", "create", change.ChangeID,
		"--workspace", root,
		"--repo", "backend",
		"--name", "Backend API",
		"--no-refresh",
		"--index=false",
		"--json",
	})
	duplicate.SetOut(&bytes.Buffer{})
	if err := duplicate.Execute(); err == nil || !strings.Contains(err.Error(), "task workspace already exists") {
		t.Fatalf("duplicate slice create error = %v, want clear existing workspace error", err)
	}

	if err := os.Chdir(backend.RepoRoot); err != nil {
		t.Fatal(err)
	}
	plain := runTaskQuickJSON(t, "local-fix", "local fix")
	if !strings.HasPrefix(plain.Workspace, filepath.Join(backend.RepoRoot, "devspecs", "tasks", plain.TaskID)) {
		t.Fatalf("plain task workspace = %q, want under backend repo", plain.Workspace)
	}
	plainManifest := mustReadFile(t, plain.ManifestPath)
	for _, unexpected := range []string{"workspace_id", "parent_change", "repo_alias"} {
		if strings.Contains(plainManifest, unexpected) {
			t.Fatalf("plain task manifest unexpectedly contains %q:\n%s", unexpected, plainManifest)
		}
	}
}

func runTaskShowJSON(t *testing.T, args ...string) taskTargetOutput {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs(append([]string{"show"}, args...))
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out taskTargetOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("task show json: %v\n%s", err, buf.String())
	}
	return out
}

func runTaskQuickJSON(t *testing.T, id, query string) taskStartOutput {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"quick",
		"--id", id,
		"--no-refresh",
		"--index=false",
		"--json",
		query,
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out taskStartOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("task quick json: %v\n%s", err, buf.String())
	}
	return out
}
