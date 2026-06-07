package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTaskCheckpointFactsUpsertAndList(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", "repo_task", "/tmp/task-facts", now, now); err != nil {
		t.Fatal(err)
	}

	fact := TaskCheckpointFact{
		RepoID:             "repo_task",
		TaskID:             "task-one",
		CheckpointID:       "cp_001",
		Target:             "A01",
		Series:             "A",
		Stage:              "implemented",
		Decision:           "promote",
		CheckpointPath:     "checkpoints/001.md",
		CheckpointJSONPath: "checkpoints/001.json",
		CreatedAt:          now,
		ActualContextJSON:  `{"files_edited":["internal/task.go"]}`,
		FeedbackJSON:       `{"critical_missed":[]}`,
		EvidenceJSON:       `{"git_diff_paths":[]}`,
		LearningsJSON:      `[{"learning_type":"miss","summary":"none"}]`,
		NextJSON:           `{"recommended_target":"A02"}`,
		IndexedAt:          now,
	}
	if err := db.UpsertTaskCheckpointFact(fact); err != nil {
		t.Fatal(err)
	}
	fact.Decision = "improve"
	fact.LearningsJSON = ""
	if err := db.UpsertTaskCheckpointFact(fact); err != nil {
		t.Fatal(err)
	}

	facts, err := db.ListTaskCheckpointFacts("repo_task", "task-one")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("facts = %#v", facts)
	}
	if facts[0].Decision != "improve" {
		t.Fatalf("expected upserted decision, got %#v", facts[0])
	}
	if strings.TrimSpace(facts[0].LearningsJSON) != "[]" {
		t.Fatalf("expected empty learnings fallback, got %q", facts[0].LearningsJSON)
	}
}
