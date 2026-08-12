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

func TestWorkspaceEAGSTGEndToEndWorkflow(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	workspace := runWorkspaceInitJSON(t, root)
	change := runChangeCreateJSON(t, "Customer export across frontend/backend", root, "backend,frontend,database,prefect")

	{
		path := filepath.Join(root, "devspecs", "workspace.yaml")

		{
			_, err := os.Stat(path)
			require.NoError(t, err,
				"expected workspace artifact %s: %v", path, err)
		}

	}
	{
		path := filepath.Join(root, "devspecs", "workspace.md")

		{
			_, err := os.Stat(path)
			require.NoError(t, err,
				"expected workspace artifact %s: %v", path, err)
		}

	}
	{
		path := change.ChangePath

		{
			_, err := os.Stat(path)
			require.NoError(t, err,
				"expected workspace artifact %s: %v", path, err)
		}

	}

	assert.Equal(t, root, workspace.WorkspaceRoot,
		"workspace root = %q, want %q", workspace.WorkspaceRoot, root)

	backend := runSliceCreateJSON(t, root, change.ChangeID, "backend", "Backend API")
	frontend := runSliceCreateJSON(t, root, change.ChangeID, "frontend", "Frontend UI")
	database := runSliceCreateJSON(t, root, change.ChangeID, "database", "Database check")
	prefect := runSliceCreateJSON(t, root, change.ChangeID, "prefect", "Prefect check")
	assertWorkspaceSlice(t, root, "backend", backend)
	assertWorkspaceSlice(t, root, "frontend", frontend)
	assertWorkspaceSlice(t, root, "database", database)
	assertWorkspaceSlice(t, root, "prefect", prefect)

	show := runTaskShowJSON(t, backend.TaskID, "--repo", backend.RepoRoot, "--json")
	assert.Equal(t, "eag-stg", show.WorkspaceID, "task show workspace link = %#v", show)
	assert.Equal(t, change.ChangeID, show.ParentChange, "task show workspace link = %#v", show)
	assert.Equal(t, "backend", show.RepoAlias, "task show workspace link = %#v", show)

	apply, err := runApplyJSON(t, []string{backend.TaskID, "--repo", backend.RepoRoot, "--json"})
	require.NoError(t, err)
	assert.Equal(t, backend.TaskID, apply.TaskID, "apply output = %#v", apply)
	assert.Equal(t, backend.Target, apply.Target, "apply output = %#v", apply)
	assert.Contains(t, apply.Prompt, "ds task checkpoint "+backend.TaskID+" --target "+backend.Target+" --repo ",
		"apply prompt missing repo-aware checkpoint command:\n%s", apply.Prompt)

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
	assert.Equal(t, traceStatusIncomplete, trace.Status, "trace after one completed slice = %#v", trace)
	require.Len(t, trace.Slices, 4, "trace after one completed slice = %#v", trace)

	runTaskCommand(t, "decide", frontend.TaskID,
		"--repo", frontend.RepoRoot,
		"--target", frontend.Target,
		"--decision", "block",
		"--index=false",
		"--json",
	)
	runTaskCommand(t, "decide", database.TaskID,
		"--repo", database.RepoRoot,
		"--target", database.Target,
		"--decision", "block",
		"--index=false",
		"--json",
	)
	runTaskCommand(t, "decide", prefect.TaskID,
		"--repo", prefect.RepoRoot,
		"--target", prefect.Target,
		"--decision", "block",
		"--index=false",
		"--json",
	)

	trace = runTraceJSON(t, change.ChangeID, "--workspace", root, "--json")
	assert.Equal(t, traceStatusComplete, trace.Status,
		"trace status after required repos completed or ruled out = %q, trace = %#v", trace.Status, trace)

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
	err = duplicate.Execute()
	require.Error(t, err)
	assert.ErrorContains(t, err, "task workspace already exists")
	{

		err := os.Chdir(backend.RepoRoot)
		require.NoError(t, err)
	}

	plain := runTaskQuickJSON(t, "local-fix", "local fix")
	assert.True(t, strings.HasPrefix(plain.Workspace, filepath.Join(backend.RepoRoot, "devspecs", "tasks", plain.TaskID)),
		"plain task workspace = %q, want under backend repo", plain.Workspace)

	plainManifest := mustReadFile(t, plain.ManifestPath)
	assert.NotContains(t, plainManifest, "workspace_id",
		"plain task manifest unexpectedly contains %q:\n%s", "workspace_id", plainManifest)
	assert.NotContains(t, plainManifest, "parent_change",
		"plain task manifest unexpectedly contains %q:\n%s", "parent_change", plainManifest)
	assert.NotContains(t, plainManifest, "repo_alias",
		"plain task manifest unexpectedly contains %q:\n%s", "repo_alias", plainManifest)

}

func assertWorkspaceSlice(t *testing.T, root, alias string, out sliceCreateOutput) {
	t.Helper()
	assert.True(t, strings.HasPrefix(out.TaskWorkspace, filepath.Join(out.RepoRoot, "devspecs", "tasks", out.TaskID)))
	assert.NoFileExists(t, filepath.Join(root, "devspecs", "tasks", out.TaskID))
	manifestBody := mustReadFile(t, out.ManifestPath)
	assert.Contains(t, manifestBody, `"workspace_id": "eag-stg"`)
	assert.Contains(t, manifestBody, `"parent_change": "EAG-C001"`)
	assert.Contains(t, manifestBody, `"repo_alias": "`+alias+`"`)
}

func runTaskShowJSON(t *testing.T, args ...string) taskTargetOutput {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs(append([]string{"show"}, args...))
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out taskTargetOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err,
			"task show json: %v\n%s", err, buf.String())
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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out taskStartOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err,
			"task quick json: %v\n%s", err, buf.String())
	}

	return out
}
