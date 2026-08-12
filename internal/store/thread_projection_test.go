package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceThreadProjection_WhenOwnerIsNew_PersistsCompleteSnapshot(t *testing.T) {
	// Arrange
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	projection := threadProjectionFixture()

	// Act
	err = db.ReplaceThreadProjection(projection)
	loaded, found, loadErr := db.GetThreadProjection(projection.OwnerID)

	// Assert
	require.NoError(t, err)
	require.NoError(t, loadErr)
	assert.True(t, found)
	assert.Equal(t, "task", loaded.OwnerKind)
	assert.Equal(t, "thread-task", loaded.TaskID)
	assert.Equal(t, 1, loaded.DefinitionRevision)
	assert.Len(t, loaded.Links, 1)
	assert.Equal(t, "A01", loaded.Links[0].Target)
	assert.Len(t, loaded.Tasks, 1)
	assert.Equal(t, "thread-task", loaded.Tasks[0].TaskID)
	assert.Len(t, loaded.Events, 1)
	assert.Equal(t, "cp_001", loaded.Events[0].CheckpointID)
	assert.Len(t, loaded.Sources, 1)
	assert.Equal(t, "definition", loaded.Sources[0].SourceKind)
}

func TestReplaceThreadProjection_WhenOwnerExists_RemovesAbsentChildRows(t *testing.T) {
	// Arrange
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	initial := threadProjectionFixture()
	require.NoError(t, db.ReplaceThreadProjection(initial))
	replacement := threadProjectionFixture()
	replacement.Events = nil
	replacement.Sources = nil
	replacement.DefinitionRevision = 2

	// Act
	err = db.ReplaceThreadProjection(replacement)
	loaded, found, loadErr := db.GetThreadProjection(replacement.OwnerID)

	// Assert
	require.NoError(t, err)
	require.NoError(t, loadErr)
	assert.True(t, found)
	assert.Equal(t, 2, loaded.DefinitionRevision)
	assert.Empty(t, loaded.Events)
	assert.Empty(t, loaded.Sources)
}

func TestDeleteThreadProjection_WhenOwnerExists_CascadesProjectionRows(t *testing.T) {
	// Arrange
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	projection := threadProjectionFixture()
	require.NoError(t, db.ReplaceThreadProjection(projection))

	// Act
	err = db.DeleteThreadProjection(projection.OwnerID)
	_, found, loadErr := db.GetThreadProjection(projection.OwnerID)
	var childRows int
	countErr := db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM thread_projection_links) +
		(SELECT COUNT(*) FROM thread_projection_tasks) +
		(SELECT COUNT(*) FROM thread_projection_events) +
		(SELECT COUNT(*) FROM thread_projection_sources)`).Scan(&childRows)

	// Assert
	require.NoError(t, err)
	require.NoError(t, loadErr)
	assert.False(t, found)
	require.NoError(t, countErr)
	assert.Zero(t, childRows)
}

func threadProjectionFixture() ThreadProjection {
	return ThreadProjection{
		OwnerID:            "thread_owner_001",
		OwnerKind:          "task",
		TaskID:             "thread-task",
		RepoRoot:           "/repo",
		DefinitionPath:     "/repo/devspecs/tasks/thread-task/threads.yaml",
		DefinitionRevision: 1,
		DefinitionJSON:     `{"schema_version":1}`,
		ProjectedAt:        "2026-08-12T14:00:00Z",
		Links: []ThreadProjectionLink{{
			Position:      0,
			TaskID:        "thread-task",
			Target:        "A01",
			RepoRoot:      "/repo",
			TaskWorkspace: "/repo/devspecs/tasks/thread-task",
		}},
		Tasks: []ThreadProjectionTask{{
			TaskID:        "thread-task",
			RepoRoot:      "/repo",
			TaskWorkspace: "/repo/devspecs/tasks/thread-task",
			ManifestPath:  "/repo/devspecs/tasks/thread-task/task.json",
			ManifestJSON:  `{"task_id":"thread-task"}`,
		}},
		Events: []ThreadProjectionEvent{{
			TaskID:       "thread-task",
			CheckpointID: "cp_001",
			Target:       "A01",
			JSONPath:     "/repo/devspecs/tasks/thread-task/checkpoints/cp_001.json",
			RecordJSON:   `{"checkpoint_id":"cp_001"}`,
		}},
		Sources: []ThreadProjectionSource{{
			SourceKind:  "definition",
			SourceKey:   "threads",
			Path:        "/repo/devspecs/tasks/thread-task/threads.yaml",
			SizeBytes:   128,
			ModifiedNS:  10,
			ContentHash: "abc",
		}},
	}
}
