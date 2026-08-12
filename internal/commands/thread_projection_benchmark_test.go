package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkThreadStatus_ColdRepoWith1000Events(b *testing.B) {
	// Arrange
	_, workspace, location := writeRepoThreadFixture(b, "thread-task")
	writeLegacyThreadCheckpointFixtures(b, workspace, "thread-task", "F01", 1000)
	b.ResetTimer()

	// Act
	for b.Loop() {
		snapshot, _, err := loadThreadOwnerSnapshot(location, nil)
		require.NoError(b, err)
		_, err = buildThreadStatus(snapshot)
		require.NoError(b, err)
	}
}

func BenchmarkThreadStatus_FreshRepoProjectionWith1000Events(b *testing.B) {
	// Arrange
	_, workspace, location := writeRepoThreadFixture(b, "thread-task")
	writeLegacyThreadCheckpointFixtures(b, workspace, "thread-task", "F01", 1000)
	snapshot, _, err := loadThreadOwnerSnapshot(location, nil)
	require.NoError(b, err)
	projection, err := threadProjectionFromSnapshot(snapshot)
	require.NoError(b, err)
	b.ResetTimer()

	// Act
	for b.Loop() {
		fresh, err := threadProjectionIsFresh(projection)
		require.NoError(b, err)
		require.True(b, fresh)
		projected, err := threadSnapshotFromProjection(projection)
		require.NoError(b, err)
		_, err = buildThreadStatus(projected)
		require.NoError(b, err)
	}
}

func BenchmarkThreadStatus_WorkspaceWith10000Events(b *testing.B) {
	// Arrange
	location := writeWorkspaceThreadScaleFixture(b, 10, 1000)
	snapshot, _, err := loadThreadOwnerSnapshot(location, nil)
	require.NoError(b, err)
	projection, err := threadProjectionFromSnapshot(snapshot)
	require.NoError(b, err)

	b.Run("Cold", func(b *testing.B) {
		// Act
		for b.Loop() {
			loaded, _, err := loadThreadOwnerSnapshot(location, nil)
			require.NoError(b, err)
			_, err = buildThreadStatus(loaded)
			require.NoError(b, err)
		}
	})
	b.Run("FreshProjection", func(b *testing.B) {
		// Act
		for b.Loop() {
			fresh, err := threadProjectionIsFresh(projection)
			require.NoError(b, err)
			require.True(b, fresh)
			projected, err := threadSnapshotFromProjection(projection)
			require.NoError(b, err)
			_, err = buildThreadStatus(projected)
			require.NoError(b, err)
		}
	})
}

func writeLegacyThreadCheckpointFixtures(b *testing.B, workspace, taskID, target string, count int) {
	b.Helper()
	for index := 0; index < count; index++ {
		checkpointID := fmt.Sprintf("cp_%06d", index)
		writeThreadCheckpointFixture(b, workspace, taskID, checkpointID, target, "validated", "promote")
	}
}

func writeWorkspaceThreadScaleFixture(b *testing.B, repoCount, eventsPerRepo int) threadOwnerLocation {
	b.Helper()
	workspaceRoot := b.TempDir()
	manifest := workspaceManifest{
		ID:          "scale-workspace",
		Name:        "Scale workspace",
		ArtifactDir: defaultWorkspaceArtifactDir,
		Repos:       make(map[string]workspaceRepo, repoCount),
	}
	definition := threadDefinition{
		SchemaVersion: threadDefinitionSchemaVersion,
		Revision:      1,
		Owner: threadDefinitionOwner{
			Kind: threadOwnerWorkspaceChange, WorkspaceID: manifest.ID, ChangeID: "SCALE-C001",
		},
		Threads: []threadDefinitionLane{{Key: "parallel"}},
	}
	var links strings.Builder
	links.WriteString("## Repo Slices\n| Repo | Task ID | Target | Name | Status |\n| --- | --- | --- | --- | --- |\n")
	for index := 0; index < repoCount; index++ {
		alias := fmt.Sprintf("repo-%02d", index)
		taskID := fmt.Sprintf("scale-task-%02d", index)
		repoRoot := filepath.Join(workspaceRoot, alias)
		err := os.MkdirAll(repoRoot, 0o755)
		require.NoError(b, err)
		manifest.Repos[alias] = workspaceRepo{Path: "./" + alias}
		taskWorkspace := writeLinkedThreadTaskFixture(b, repoRoot, workspaceRoot, alias, taskID, "A", "A01")
		taskManifestPath := filepath.Join(taskWorkspace, taskManifestFilename)
		task, err := readTaskManifest(taskManifestPath)
		require.NoError(b, err)
		task.WorkspaceID = manifest.ID
		task.ParentChange = "SCALE-C001"
		err = writeTaskManifest(taskManifestPath, task)
		require.NoError(b, err)
		writeLegacyThreadCheckpointFixtures(b, taskWorkspace, taskID, "A01", eventsPerRepo)
		definition.Threads[0].Targets = append(definition.Threads[0].Targets, threadTargetReference{
			Repo: alias, Task: taskID, Target: "A01",
		})
		fmt.Fprintf(&links, "| `%s` | `%s` | `A01` | Scale | `planned` |\n", alias, taskID)
	}
	err := os.MkdirAll(workspaceChangesDir(workspaceRoot, manifest), 0o755)
	require.NoError(b, err)
	err = writeWorkspaceManifest(workspaceManifestPath(workspaceRoot), manifest)
	require.NoError(b, err)
	change := "---\nid: SCALE-C001\ntype: change\nworkspace: scale-workspace\nstatus: active\ntitle: Scale\n---\n\n# Scale\n\n" + links.String()
	err = os.WriteFile(filepath.Join(workspaceChangesDir(workspaceRoot, manifest), "SCALE-C001-scale.md"), []byte(change), 0o644)
	require.NoError(b, err)
	err = writeThreadDefinition(workspaceThreadDefinitionPath(workspaceRoot, manifest, "SCALE-C001"), definition)
	require.NoError(b, err)
	return workspaceThreadOwnerLocation(workspaceRoot, manifest, "SCALE-C001")
}
