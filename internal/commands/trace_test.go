package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceWorkspaceChangeListsRepoSlicesAndIndexState(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	change := runChangeCreateJSON(t, "Customer export", root, "backend,frontend,database,prefect")

	backend := runSliceCreateJSON(t, root, change.ChangeID, "backend", "Backend API")
	frontend := runSliceCreateJSON(t, root, change.ChangeID, "frontend", "Frontend UI")
	database := runSliceCreateJSON(t, root, change.ChangeID, "database", "Database migration")
	prefect := runSliceCreateJSON(t, root, change.ChangeID, "prefect", "Prefect flow")

	runTaskCommand(t, "checkpoint", backend.TaskID,
		"--repo", backend.RepoRoot,
		"--target", backend.Target,
		"--stage", "validated",
		"--decision", "promote",
		"--file-read", "internal/service.go",
		"--index=false",
		"--json",
	)
	runTaskCommand(t, "start", frontend.TaskID,
		"--repo", frontend.RepoRoot,
		"--target", frontend.Target,
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
	{
		err := os.Remove(prefect.ResultPath)
		require.NoError(t, err)
	}

	trace := runTraceJSON(t, change.ChangeID, "--workspace", root, "--json")
	assert.Equal(t, "workspace_change", trace.Kind, "workspace trace basics = %#v", trace)
	assert.Equal(t, change.ChangeID, trace.ChangeID, "workspace trace basics = %#v", trace)
	assert.Equal(t, root, trace.WorkspaceRoot, "workspace trace basics = %#v", trace)
	assert.Equal(t, traceStatusIncomplete, trace.Status,
		"workspace trace status = %q, want %q", trace.Status, traceStatusIncomplete)
	require.Len(t, trace.Slices, 4,
		"workspace trace slices = %d, want 4: %#v", len(trace.Slices), trace.Slices)

	backendSlice := traceSliceByAlias(trace.Slices, "backend")
	require.NotNil(t, backendSlice)
	assert.Equal(t, "completed", backendSlice.Status)
	assert.Equal(t, traceIndexMissing, backendSlice.IndexStatus)
	frontendSlice := traceSliceByAlias(trace.Slices, "frontend")
	require.NotNil(t, frontendSlice)
	assert.Equal(t, "started", frontendSlice.Status)
	assert.Equal(t, traceIndexMissing, frontendSlice.IndexStatus)
	databaseSlice := traceSliceByAlias(trace.Slices, "database")
	require.NotNil(t, databaseSlice)
	assert.Equal(t, "blocked", databaseSlice.Status)
	assert.Equal(t, traceIndexMissing, databaseSlice.IndexStatus)
	prefectSlice := traceSliceByAlias(trace.Slices, "prefect")
	require.NotNil(t, prefectSlice)
	assert.Equal(t, "missing_result", prefectSlice.Status)
	assert.Equal(t, traceIndexMissing, prefectSlice.IndexStatus)
	{
		got := len(trace.Edges)
		assert.Equal(t, 4, got,
			"trace edges = %d, want 4: %#v", got, trace.Edges)
	}

}

func TestTraceRepoTaskShowsParentChangeAndSiblingAliases(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	change := runChangeCreateJSON(t, "Customer export", root, "backend,frontend")

	backend := runSliceCreateJSON(t, root, change.ChangeID, "backend", "Backend API")
	frontend := runSliceCreateJSON(t, root, change.ChangeID, "frontend", "Frontend UI")

	trace := runTraceJSON(t, backend.TaskID, "--repo", backend.RepoRoot, "--json")
	assert.Equal(t, "repo_task", trace.Kind, "repo task trace basics = %#v", trace)
	assert.Equal(t, backend.TaskID, trace.TaskID, "repo task trace basics = %#v", trace)
	assert.Equal(t, traceStatusIncomplete, trace.Status,
		"repo task parent trace status = %q, want %q", trace.Status, traceStatusIncomplete)
	assert.Equal(t, change.ChangeID, trace.ParentChange, "repo task trace parent link = %#v", trace)
	assert.Equal(t, change.ChangeID, trace.ChangeID, "repo task trace parent link = %#v", trace)
	assert.Equal(t, "backend", trace.RepoAlias, "repo task trace parent link = %#v", trace)
	require.NotNil(t, traceSliceByAlias(trace.Slices, "backend"), "repo task trace should include parent change siblings, got %#v", trace.Slices)
	require.NotNil(t, traceSliceByAlias(trace.Slices, "frontend"), "repo task trace should include parent change siblings, got %#v", trace.Slices)

	frontendSlice := traceSliceByAlias(trace.Slices, "frontend")
	assert.Equal(t, frontend.TaskID, frontendSlice.TaskID,
		"frontend sibling task id = %q, want %q", frontendSlice.TaskID, frontend.TaskID)

}

func TestSliceCreateUpsertsWorkspaceTraceEdge(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	change := runChangeCreateJSON(t, "Customer export", root, "backend")
	slice := runSliceCreateJSON(t, root, change.ChangeID, "backend", "Backend API")

	db, err := openDB()
	require.NoError(t, err)

	defer db.Close()
	meta := db.GetRepoByRoot(canonicalRepoRoot(root))
	require.NotNil(t, meta,
		"workspace root was not registered in the index")

	edges, err := db.GetArtifactEdges(store.ArtifactEdgeFilter{
		RepoID:        meta.ID,
		SrcArtifactID: traceChangeArtifactID(change.ChangeID),
		EdgeType:      traceEdgeWorkspaceChangeHasSlice,
	})
	require.NoError(t, err)
	require.Len(t, edges, 1,
		"workspace trace edges = %d, want 1: %#v", len(edges), edges)

	edge := edges[0]
	assert.Equal(t, traceTaskArtifactID(slice.TaskID), edge.DstArtifactID,
		"edge dst = %q, want %q", edge.DstArtifactID, traceTaskArtifactID(slice.TaskID))

	assert.Contains(t, edge.MetadataJSON, `"change_id":"EAG-C001"`,
		"edge metadata missing %q: %s", `"change_id":"EAG-C001"`, edge.MetadataJSON)
	assert.Contains(t, edge.MetadataJSON, `"repo_alias":"backend"`,
		"edge metadata missing %q: %s", `"repo_alias":"backend"`, edge.MetadataJSON)
	assert.Contains(t, edge.MetadataJSON, `"task_id":"eag-c001-backend"`,
		"edge metadata missing %q: %s", `"task_id":"eag-c001-backend"`, edge.MetadataJSON)

}

func TestTraceWorkspaceChangePlannedStatusBeforeWorkStarts(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	change := runChangeCreateJSON(t, "Customer export", root, "prefect")
	prefect := runSliceCreateJSON(t, root, change.ChangeID, "prefect", "Prefect flow")

	trace := runTraceJSON(t, change.ChangeID, "--workspace", root, "--json")
	slice := traceSliceByAlias(trace.Slices, "prefect")
	require.NotNil(t, slice,
		"trace missing prefect slice: %#v", trace.Slices)
	assert.Equal(t, prefect.TaskID, slice.TaskID, "planned trace slice = %#v", slice)
	assert.Equal(t, "planned", slice.Status, "planned trace slice = %#v", slice)
	assert.Equal(t, traceStatusIncomplete, trace.Status,
		"planned trace status = %q, want %q", trace.Status, traceStatusIncomplete)
	{

		_, err := os.Stat(filepath.Join(root, "prefect", "devspecs", "tasks", prefect.TaskID))
		require.NoError(t, err,
			"planned trace should still keep repo-local task workspace: %v", err)
	}

}

func runSliceCreateJSON(t *testing.T, root, changeID, repoAlias, name string) sliceCreateOutput {
	t.Helper()
	cmd := NewWorkspaceCmd()
	cmd.SetArgs([]string{
		"slice", "create", changeID,
		"--workspace", root,
		"--repo", repoAlias,
		"--name", name,
		"--no-refresh",
		"--index=false",
		"--json",
	})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out sliceCreateOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err,
			"slice create json: %v\n%s", err, buf.String())
	}

	return out
}

func runTraceJSON(t *testing.T, args ...string) traceOutput {
	t.Helper()
	cmd := NewWorkspaceCmd()
	cmd.SetArgs(append([]string{"trace"}, args...))
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out traceOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err,
			"trace json: %v\n%s", err, buf.String())
	}

	return out
}

func runTaskCommand(t *testing.T, args ...string) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

}

func traceSliceByAlias(slices []traceSliceOutput, alias string) *traceSliceOutput {
	for i := range slices {
		if slices[i].RepoAlias == alias {
			return &slices[i]
		}
	}
	return nil
}
