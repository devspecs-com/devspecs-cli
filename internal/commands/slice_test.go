package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSliceCreateLinksWorkspaceChangeToRepoTask(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	change := runChangeCreateJSON(t, "Customer export", root, "backend,frontend")

	cmd := NewSliceCmd()
	cmd.SetArgs([]string{
		"create", change.ChangeID,
		"--workspace", root,
		"--repo", "backend",
		"--name", "Backend API",
		"--no-refresh",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out sliceCreateOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("slice create json: %v\n%s", err, buf.String())
	}
	child := filepath.Join(root, "enalytics-backend")
	if out.ChangeID != "EAG-C001" || out.TaskID != "eag-c001-backend" || out.Target != "A01" || out.RepoAlias != "backend" {
		t.Fatalf("slice create output = %#v", out)
	}
	if !strings.HasPrefix(out.TaskWorkspace, filepath.Join(child, "devspecs", "tasks", "eag-c001-backend")) {
		t.Fatalf("task workspace = %q, want under child repo", out.TaskWorkspace)
	}
	if _, err := os.Stat(filepath.Join(root, "devspecs", "tasks", "eag-c001-backend")); !os.IsNotExist(err) {
		t.Fatalf("slice create unexpectedly wrote task under workspace root: %v", err)
	}

	manifestBody := mustReadFile(t, out.ManifestPath)
	for _, want := range []string{
		`"workspace_id": "eag-stg"`,
		`"parent_change": "EAG-C001"`,
		`"repo_alias": "backend"`,
	} {
		if !strings.Contains(manifestBody, want) {
			t.Fatalf("task manifest missing %q:\n%s", want, manifestBody)
		}
	}
	for _, path := range []string{out.PlanPath, out.ResultPath, filepath.Join(out.TaskWorkspace, "A00-index.md")} {
		body := mustReadFile(t, path)
		for _, want := range []string{
			"workspace_id: eag-stg",
			"parent_change: EAG-C001",
			"repo_alias: backend",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, body)
			}
		}
	}
	changeBody := mustReadFile(t, change.ChangePath)
	for _, want := range []string{
		"## Repo Slices",
		"| `backend` | `eag-c001-backend` | `A01` | Backend API | `planned` |",
	} {
		if !strings.Contains(changeBody, want) {
			t.Fatalf("workspace change missing %q:\n%s", want, changeBody)
		}
	}

	showCmd := NewTaskCmd()
	showCmd.SetArgs([]string{"show", "eag-c001-backend", "--repo", child, "--json"})
	showBuf := &bytes.Buffer{}
	showCmd.SetOut(showBuf)
	if err := showCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var show taskTargetOutput
	if err := json.Unmarshal(showBuf.Bytes(), &show); err != nil {
		t.Fatalf("task show json: %v\n%s", err, showBuf.String())
	}
	if show.WorkspaceID != "eag-stg" || show.ParentChange != "EAG-C001" || show.RepoAlias != "backend" {
		t.Fatalf("task show missing workspace link: %#v", show)
	}

	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "eag-c001-backend",
		"--repo", child,
		"--target", "A01",
		"--stage", "validated",
		"--decision", "promote",
		"--file-read", "internal/service.go",
		"--index=false",
		"--json",
	})
	checkpointBuf := &bytes.Buffer{}
	checkpointCmd.SetOut(checkpointBuf)
	if err := checkpointCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var checkpoint taskCheckpointOutput
	if err := json.Unmarshal(checkpointBuf.Bytes(), &checkpoint); err != nil {
		t.Fatalf("checkpoint json: %v\n%s", err, checkpointBuf.String())
	}
	checkpointMarkdown := mustReadFile(t, checkpoint.CheckpointPath)
	checkpointJSON := mustReadFile(t, checkpoint.CheckpointJSONPath)
	for _, body := range []string{checkpointMarkdown, checkpointJSON} {
		for _, want := range []string{"workspace_id", "parent_change", "repo_alias", "EAG-C001", "backend"} {
			if !strings.Contains(body, want) {
				t.Fatalf("checkpoint missing %q:\n%s", want, body)
			}
		}
	}
}

func TestSliceCreateRejectsMissingWorkspaceRepoAlias(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	change := runChangeCreateJSON(t, "Customer export", root, "backend")

	cmd := NewSliceCmd()
	cmd.SetArgs([]string{
		"create", change.ChangeID,
		"--workspace", root,
		"--repo", "missing",
		"--name", "Missing Repo",
		"--no-refresh",
		"--index=false",
		"--json",
	})
	cmd.SetOut(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing repo alias error")
	}
	if !strings.Contains(err.Error(), `workspace repo alias "missing" not found`) {
		t.Fatalf("missing repo alias error = %v", err)
	}
}

func TestTaskQuickWithoutWorkspaceLinkOmitsWorkspaceMetadata(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"quick", "--id", "plain-quick", "--no-refresh", "--index=false", "--json", "plain quick task"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out taskStartOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("quick json: %v\n%s", err, buf.String())
	}
	manifestBody := mustReadFile(t, out.ManifestPath)
	for _, unexpected := range []string{"workspace_id", "parent_change", "repo_alias"} {
		if strings.Contains(manifestBody, unexpected) {
			t.Fatalf("plain task manifest unexpectedly contains %q:\n%s", unexpected, manifestBody)
		}
	}
	for _, path := range []string{out.FirstSlicePath, out.ResultPath, filepath.Join(repoDir, "devspecs", "tasks", "plain-quick", "A00-index.md")} {
		body := mustReadFile(t, path)
		if strings.Contains(body, "## Workspace Link") {
			t.Fatalf("plain task artifact unexpectedly contains workspace metadata:\n%s", body)
		}
	}
}

func runChangeCreateJSON(t *testing.T, title, root, repos string) changeCreateOutput {
	t.Helper()
	cmd := NewWorkspaceCmd()
	cmd.SetArgs([]string{"change", "create", title, "--workspace", root, "--repos", repos, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out changeCreateOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("change create json: %v\n%s", err, buf.String())
	}
	return out
}
