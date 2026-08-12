package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertTaskCheckpointFact_WhenCheckpointIsNew_InsertsFact(t *testing.T) {
	db := openTaskCheckpointTestDB(t)
	fact := taskCheckpointFactFixture("task-one", "cp_001", "2026-06-06T00:00:00Z")
	fact.LearningsJSON = ""

	err := db.UpsertTaskCheckpointFact(fact)
	require.NoError(t, err)

	var decision, learningsJSON string
	require.NoError(t, db.QueryRow(`SELECT decision, learnings_json FROM task_checkpoint_facts
		WHERE repo_id = ? AND task_id = ? AND checkpoint_id = ?`, fact.RepoID, fact.TaskID, fact.CheckpointID).
		Scan(&decision, &learningsJSON))
	assert.Equal(t, "promote", decision)
	assert.Equal(t, "[]", learningsJSON)
}

func TestUpsertTaskCheckpointFact_WhenCheckpointExists_UpdatesFact(t *testing.T) {
	db := openTaskCheckpointTestDB(t)
	fact := taskCheckpointFactFixture("task-one", "cp_001", "2026-06-06T00:00:00Z")
	seedTaskCheckpointFact(t, db, fact)
	fact.Decision = "improve"
	fact.LearningsJSON = ""

	err := db.UpsertTaskCheckpointFact(fact)
	require.NoError(t, err)

	var decision, learningsJSON string
	require.NoError(t, db.QueryRow(`SELECT decision, learnings_json FROM task_checkpoint_facts
		WHERE repo_id = ? AND task_id = ? AND checkpoint_id = ?`, fact.RepoID, fact.TaskID, fact.CheckpointID).
		Scan(&decision, &learningsJSON))
	assert.Equal(t, "improve", decision)
	assert.Equal(t, "[]", learningsJSON)
}

func TestListTaskCheckpointFacts_WhenTaskHasOneCheckpoint_ReturnsFact(t *testing.T) {
	db := openTaskCheckpointTestDB(t)
	fact := taskCheckpointFactFixture("task-one", "cp_001", "2026-06-06T00:00:00Z")
	seedTaskCheckpointFact(t, db, fact)

	facts, err := db.ListTaskCheckpointFacts("repo_task", "task-one")
	require.NoError(t, err)

	require.Len(t, facts, 1)
	assert.Equal(t, "task-one", facts[0].TaskID)
	assert.Equal(t, "cp_001", facts[0].CheckpointID)
	assert.Equal(t, "promote", facts[0].Decision)
}

func TestListRecentTaskCheckpointFacts_WhenLimited_ReturnsNewestFact(t *testing.T) {
	db := openTaskCheckpointTestDB(t)
	seedTaskCheckpointFact(t, db, taskCheckpointFactFixture("task-one", "cp_001", "2026-06-06T00:00:00Z"))
	seedTaskCheckpointFact(t, db, taskCheckpointFactFixture("task-two", "cp_002", "2026-06-07T00:00:00Z"))

	recent, err := db.ListRecentTaskCheckpointFacts("repo_task", 1)
	require.NoError(t, err)

	require.Len(t, recent, 1)
	assert.Equal(t, "task-two", recent[0].TaskID)
	assert.Equal(t, "cp_002", recent[0].CheckpointID)
}

func openTaskCheckpointTestDB(t *testing.T) *DB {
	t.Helper()

	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	now := "2026-06-06T00:00:00Z"
	_, err = db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", "repo_task", "/tmp/task-facts", now, now)
	require.NoError(t, err)
	return db
}

func taskCheckpointFactFixture(taskID, checkpointID, createdAt string) TaskCheckpointFact {
	return TaskCheckpointFact{
		RepoID:             "repo_task",
		TaskID:             taskID,
		CheckpointID:       checkpointID,
		Target:             "A01",
		Series:             "A",
		Stage:              "implemented",
		Decision:           "promote",
		CheckpointPath:     "checkpoints/001.md",
		CheckpointJSONPath: "checkpoints/001.json",
		CreatedAt:          createdAt,
		ActualContextJSON:  `{"files_edited":["internal/task.go"]}`,
		FeedbackJSON:       `{"critical_missed":[]}`,
		EvidenceJSON:       `{"git_diff_paths":[]}`,
		LearningsJSON:      `[{"learning_type":"miss","summary":"none"}]`,
		NextJSON:           `{"recommended_target":"A02"}`,
		IndexedAt:          createdAt,
	}
}

func seedTaskCheckpointFact(t *testing.T, db *DB, fact TaskCheckpointFact) {
	t.Helper()

	_, err := db.Exec(`INSERT INTO task_checkpoint_facts (
		repo_id, task_id, checkpoint_id, target, series, stage, decision,
		checkpoint_path, checkpoint_json_path, created_at,
		actual_context_json, feedback_json, evidence_json, learnings_json, next_json, indexed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fact.RepoID, fact.TaskID, fact.CheckpointID, fact.Target, fact.Series, fact.Stage, fact.Decision,
		fact.CheckpointPath, fact.CheckpointJSONPath, fact.CreatedAt,
		fact.ActualContextJSON, fact.FeedbackJSON, fact.EvidenceJSON, fact.LearningsJSON, fact.NextJSON, fact.IndexedAt)
	require.NoError(t, err)
}
