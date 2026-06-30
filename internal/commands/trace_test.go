package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/store"
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
	if err := os.Remove(prefect.ResultPath); err != nil {
		t.Fatal(err)
	}

	trace := runTraceJSON(t, change.ChangeID, "--workspace", root, "--json")
	if trace.Kind != "workspace_change" || trace.ChangeID != change.ChangeID || trace.WorkspaceRoot != root {
		t.Fatalf("workspace trace basics = %#v", trace)
	}
	if trace.Status != traceStatusIncomplete {
		t.Fatalf("workspace trace status = %q, want %q", trace.Status, traceStatusIncomplete)
	}
	if len(trace.Slices) != 4 {
		t.Fatalf("workspace trace slices = %d, want 4: %#v", len(trace.Slices), trace.Slices)
	}
	wantStatus := map[string]string{
		"backend":  "completed",
		"frontend": "started",
		"database": "blocked",
		"prefect":  "missing_result",
	}
	for alias, want := range wantStatus {
		slice := traceSliceByAlias(trace.Slices, alias)
		if slice == nil {
			t.Fatalf("trace missing repo alias %q: %#v", alias, trace.Slices)
		}
		if slice.Status != want {
			t.Fatalf("trace status for %s = %q, want %q: %#v", alias, slice.Status, want, slice)
		}
		if slice.IndexStatus != traceIndexMissing {
			t.Fatalf("trace index status for %s = %q, want %q: %#v", alias, slice.IndexStatus, traceIndexMissing, slice)
		}
	}
	if got := len(trace.Edges); got != 4 {
		t.Fatalf("trace edges = %d, want 4: %#v", got, trace.Edges)
	}
}

func TestTraceRepoTaskShowsParentChangeAndSiblingAliases(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	change := runChangeCreateJSON(t, "Customer export", root, "backend,frontend")

	backend := runSliceCreateJSON(t, root, change.ChangeID, "backend", "Backend API")
	frontend := runSliceCreateJSON(t, root, change.ChangeID, "frontend", "Frontend UI")

	trace := runTraceJSON(t, backend.TaskID, "--repo", backend.RepoRoot, "--json")
	if trace.Kind != "repo_task" || trace.TaskID != backend.TaskID {
		t.Fatalf("repo task trace basics = %#v", trace)
	}
	if trace.Status != traceStatusIncomplete {
		t.Fatalf("repo task parent trace status = %q, want %q", trace.Status, traceStatusIncomplete)
	}
	if trace.ParentChange != change.ChangeID || trace.ChangeID != change.ChangeID || trace.RepoAlias != "backend" {
		t.Fatalf("repo task trace parent link = %#v", trace)
	}
	if traceSliceByAlias(trace.Slices, "backend") == nil || traceSliceByAlias(trace.Slices, "frontend") == nil {
		t.Fatalf("repo task trace should include parent change siblings, got %#v", trace.Slices)
	}
	frontendSlice := traceSliceByAlias(trace.Slices, "frontend")
	if frontendSlice.TaskID != frontend.TaskID {
		t.Fatalf("frontend sibling task id = %q, want %q", frontendSlice.TaskID, frontend.TaskID)
	}
}

func TestSliceCreateUpsertsWorkspaceTraceEdge(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	change := runChangeCreateJSON(t, "Customer export", root, "backend")
	slice := runSliceCreateJSON(t, root, change.ChangeID, "backend", "Backend API")

	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	meta := db.GetRepoByRoot(canonicalRepoRoot(root))
	if meta == nil {
		t.Fatal("workspace root was not registered in the index")
	}
	edges, err := db.GetArtifactEdges(store.ArtifactEdgeFilter{
		RepoID:        meta.ID,
		SrcArtifactID: traceChangeArtifactID(change.ChangeID),
		EdgeType:      traceEdgeWorkspaceChangeHasSlice,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 {
		t.Fatalf("workspace trace edges = %d, want 1: %#v", len(edges), edges)
	}
	edge := edges[0]
	if edge.DstArtifactID != traceTaskArtifactID(slice.TaskID) {
		t.Fatalf("edge dst = %q, want %q", edge.DstArtifactID, traceTaskArtifactID(slice.TaskID))
	}
	for _, want := range []string{`"change_id":"EAG-C001"`, `"repo_alias":"backend"`, `"task_id":"eag-c001-backend"`} {
		if !strings.Contains(edge.MetadataJSON, want) {
			t.Fatalf("edge metadata missing %q: %s", want, edge.MetadataJSON)
		}
	}
}

func TestTraceWorkspaceChangePlannedStatusBeforeWorkStarts(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	change := runChangeCreateJSON(t, "Customer export", root, "prefect")
	prefect := runSliceCreateJSON(t, root, change.ChangeID, "prefect", "Prefect flow")

	trace := runTraceJSON(t, change.ChangeID, "--workspace", root, "--json")
	slice := traceSliceByAlias(trace.Slices, "prefect")
	if slice == nil {
		t.Fatalf("trace missing prefect slice: %#v", trace.Slices)
	}
	if slice.TaskID != prefect.TaskID || slice.Status != "planned" {
		t.Fatalf("planned trace slice = %#v", slice)
	}
	if trace.Status != traceStatusIncomplete {
		t.Fatalf("planned trace status = %q, want %q", trace.Status, traceStatusIncomplete)
	}
	if _, err := os.Stat(filepath.Join(root, "prefect", "devspecs", "tasks", prefect.TaskID)); err != nil {
		t.Fatalf("planned trace should still keep repo-local task workspace: %v", err)
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
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out sliceCreateOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("slice create json: %v\n%s", err, buf.String())
	}
	return out
}

func runTraceJSON(t *testing.T, args ...string) traceOutput {
	t.Helper()
	cmd := NewWorkspaceCmd()
	cmd.SetArgs(append([]string{"trace"}, args...))
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out traceOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("trace json: %v\n%s", err, buf.String())
	}
	return out
}

func runTaskCommand(t *testing.T, args ...string) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
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
