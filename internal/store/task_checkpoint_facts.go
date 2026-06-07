package store

// TaskCheckpointFact is the compact query-facing record for a task checkpoint.
type TaskCheckpointFact struct {
	RepoID             string
	TaskID             string
	CheckpointID       string
	Target             string
	Series             string
	Stage              string
	Decision           string
	CheckpointPath     string
	CheckpointJSONPath string
	CreatedAt          string
	ActualContextJSON  string
	FeedbackJSON       string
	EvidenceJSON       string
	LearningsJSON      string
	NextJSON           string
	IndexedAt          string
}

// UpsertTaskCheckpointFact stores compact structured checkpoint facts.
func (db *DB) UpsertTaskCheckpointFact(f TaskCheckpointFact) error {
	_, err := db.Exec(
		`INSERT INTO task_checkpoint_facts (
			repo_id, task_id, checkpoint_id, target, series, stage, decision,
			checkpoint_path, checkpoint_json_path, created_at,
			actual_context_json, feedback_json, evidence_json, learnings_json, next_json, indexed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repo_id, task_id, checkpoint_id) DO UPDATE SET
			target = excluded.target,
			series = excluded.series,
			stage = excluded.stage,
			decision = excluded.decision,
			checkpoint_path = excluded.checkpoint_path,
			checkpoint_json_path = excluded.checkpoint_json_path,
			created_at = excluded.created_at,
			actual_context_json = excluded.actual_context_json,
			feedback_json = excluded.feedback_json,
			evidence_json = excluded.evidence_json,
			learnings_json = excluded.learnings_json,
			next_json = excluded.next_json,
			indexed_at = excluded.indexed_at`,
		f.RepoID, f.TaskID, f.CheckpointID, f.Target, f.Series, f.Stage, f.Decision,
		f.CheckpointPath, f.CheckpointJSONPath, f.CreatedAt,
		nonEmptyJSON(f.ActualContextJSON, "{}"),
		nonEmptyJSON(f.FeedbackJSON, "{}"),
		nonEmptyJSON(f.EvidenceJSON, "{}"),
		nonEmptyJSON(f.LearningsJSON, "[]"),
		nonEmptyJSON(f.NextJSON, "{}"),
		f.IndexedAt,
	)
	return err
}

// ListTaskCheckpointFacts returns compact checkpoint facts for one task.
func (db *DB) ListTaskCheckpointFacts(repoID, taskID string) ([]TaskCheckpointFact, error) {
	rows, err := db.Query(
		`SELECT repo_id, task_id, checkpoint_id, target, series, stage, decision,
			checkpoint_path, checkpoint_json_path, created_at,
			actual_context_json, feedback_json, evidence_json, learnings_json, next_json, indexed_at
		FROM task_checkpoint_facts
		WHERE repo_id = ? AND task_id = ?
		ORDER BY created_at ASC`,
		repoID, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskCheckpointFact
	for rows.Next() {
		var f TaskCheckpointFact
		if err := rows.Scan(
			&f.RepoID, &f.TaskID, &f.CheckpointID, &f.Target, &f.Series, &f.Stage, &f.Decision,
			&f.CheckpointPath, &f.CheckpointJSONPath, &f.CreatedAt,
			&f.ActualContextJSON, &f.FeedbackJSON, &f.EvidenceJSON, &f.LearningsJSON, &f.NextJSON, &f.IndexedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func nonEmptyJSON(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
