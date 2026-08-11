package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	err := cmd.Execute()

	require.NoError(t, err)
	var out sliceCreateOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))

	child := filepath.Join(root, "enalytics-backend")
	assert.Equal(t, "EAG-C001", out.ChangeID)
	assert.Equal(t, "eag-c001-backend", out.TaskID)
	assert.Equal(t, "A01", out.Target)
	assert.Equal(t, "backend", out.RepoAlias)
	assert.True(t, strings.HasPrefix(out.TaskWorkspace, filepath.Join(child, "devspecs", "tasks", "eag-c001-backend")))
	assert.NoDirExists(t, filepath.Join(root, "devspecs", "tasks", "eag-c001-backend"))

	manifestBody := mustReadFile(t, out.ManifestPath)
	assert.Contains(t, manifestBody, `"workspace_id": "eag-stg"`)
	assert.Contains(t, manifestBody, `"parent_change": "EAG-C001"`)
	assert.Contains(t, manifestBody, `"repo_alias": "backend"`)
	planBody := mustReadFile(t, out.PlanPath)
	assert.Contains(t, planBody, "workspace_id: eag-stg")
	assert.Contains(t, planBody, "parent_change: EAG-C001")
	assert.Contains(t, planBody, "repo_alias: backend")
	resultBody := mustReadFile(t, out.ResultPath)
	assert.Contains(t, resultBody, "workspace_id: eag-stg")
	assert.Contains(t, resultBody, "parent_change: EAG-C001")
	assert.Contains(t, resultBody, "repo_alias: backend")
	indexBody := mustReadFile(t, filepath.Join(out.TaskWorkspace, "A00-index.md"))
	assert.Contains(t, indexBody, "workspace_id: eag-stg")
	assert.Contains(t, indexBody, "parent_change: EAG-C001")
	assert.Contains(t, indexBody, "repo_alias: backend")
	changeBody := mustReadFile(t, change.ChangePath)
	assert.Contains(t, changeBody, "## Repo Slices")
	assert.Contains(t, changeBody, "| `backend` | `eag-c001-backend` | `A01` | Backend API | `planned` |")
}

type workspaceSliceFixture struct {
	child string
	out   sliceCreateOutput
}

func setupWorkspaceSlice(t *testing.T) workspaceSliceFixture {
	t.Helper()
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
	require.NoError(t, cmd.Execute())
	var out sliceCreateOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	return workspaceSliceFixture{child: filepath.Join(root, "enalytics-backend"), out: out}
}

func TestTaskShowReturnsWorkspaceLinkForCreatedSlice(t *testing.T) {
	fixture := setupWorkspaceSlice(t)
	showCmd := NewTaskCmd()
	showCmd.SetArgs([]string{"show", "eag-c001-backend", "--repo", fixture.child, "--json"})
	showBuf := &bytes.Buffer{}
	showCmd.SetOut(showBuf)

	err := showCmd.Execute()

	require.NoError(t, err)
	var show taskTargetOutput
	require.NoError(t, json.Unmarshal(showBuf.Bytes(), &show))
	assert.Equal(t, "eag-stg", show.WorkspaceID)
	assert.Equal(t, "EAG-C001", show.ParentChange)
	assert.Equal(t, "backend", show.RepoAlias)
}

func TestTaskCheckpointPersistsWorkspaceLinkForCreatedSlice(t *testing.T) {
	fixture := setupWorkspaceSlice(t)
	checkpointCmd := NewTaskCmd()
	checkpointCmd.SetArgs([]string{
		"checkpoint", "eag-c001-backend",
		"--repo", fixture.child,
		"--target", "A01",
		"--stage", "validated",
		"--decision", "promote",
		"--file-read", "internal/service.go",
		"--index=false",
		"--json",
	})
	checkpointBuf := &bytes.Buffer{}
	checkpointCmd.SetOut(checkpointBuf)

	err := checkpointCmd.Execute()

	require.NoError(t, err)
	var checkpoint taskCheckpointOutput
	require.NoError(t, json.Unmarshal(checkpointBuf.Bytes(), &checkpoint))
	checkpointMarkdown := mustReadFile(t, checkpoint.CheckpointPath)
	checkpointJSON := mustReadFile(t, checkpoint.CheckpointJSONPath)
	assert.Contains(t, checkpointMarkdown, "workspace_id")
	assert.Contains(t, checkpointMarkdown, "parent_change")
	assert.Contains(t, checkpointMarkdown, "repo_alias")
	assert.Contains(t, checkpointMarkdown, "EAG-C001")
	assert.Contains(t, checkpointMarkdown, "backend")
	assert.Contains(t, checkpointJSON, "workspace_id")
	assert.Contains(t, checkpointJSON, "parent_change")
	assert.Contains(t, checkpointJSON, "repo_alias")
	assert.Contains(t, checkpointJSON, "EAG-C001")
	assert.Contains(t, checkpointJSON, "backend")
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
	require.Error(t, err)
	assert.ErrorContains(t, err, `workspace repo alias "missing" not found`)
}

func TestTaskQuickWithoutWorkspaceLinkOmitsWorkspaceMetadata(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	cmd := NewTaskCmd()
	cmd.SetArgs([]string{"quick", "--id", "plain-quick", "--no-refresh", "--index=false", "--json", "plain quick task"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out taskStartOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))

	manifestBody := mustReadFile(t, out.ManifestPath)
	assert.NotContains(t, manifestBody, "workspace_id")
	assert.NotContains(t, manifestBody, "parent_change")
	assert.NotContains(t, manifestBody, "repo_alias")
	assert.NotContains(t, mustReadFile(t, out.FirstSlicePath), "## Workspace Link")
	assert.NotContains(t, mustReadFile(t, out.ResultPath), "## Workspace Link")
	assert.NotContains(t, mustReadFile(t, filepath.Join(repoDir, "devspecs", "tasks", "plain-quick", "A00-index.md")), "## Workspace Link")
}

func runChangeCreateJSON(t *testing.T, title, root, repos string) changeCreateOutput {
	t.Helper()
	cmd := NewWorkspaceCmd()
	cmd.SetArgs([]string{"change", "create", title, "--workspace", root, "--repos", repos, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out changeCreateOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"change create json: %v\n%s", err, buf.String())
	}

	return out
}
