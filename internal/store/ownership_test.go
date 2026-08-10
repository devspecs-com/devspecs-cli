package store

import "testing"

func TestRepairLegacyArtifactOwnershipRepairsOnlyUnanimousSources(t *testing.T) {
	db := openTestDB(t)
	now := "2026-08-07T00:00:00Z"
	for _, repoID := range []string{"old", "target", "other"} {
		if _, err := db.Exec(`INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)`, repoID, "/"+repoID, now, now); err != nil {
			t.Fatal(err)
		}
	}
	seedPruneArtifact(t, db, "old", "repairable", "repairable-source", now)
	if _, err := db.Exec(`UPDATE sources SET repo_id = 'target' WHERE id = 'repairable-source'`); err != nil {
		t.Fatal(err)
	}
	seedPruneArtifact(t, db, "old", "ambiguous", "ambiguous-source", now)
	if _, err := db.Exec(`UPDATE sources SET repo_id = 'target' WHERE id = 'ambiguous-source'`); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertSourceDirect("ambiguous-other", "ambiguous", "other", "markdown", "shared.md", "shared.md|markdown", "", "", now); err != nil {
		t.Fatal(err)
	}

	report, err := db.RepairLegacyArtifactOwnership("target", []string{"repairable|markdown", "ambiguous|markdown"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.RepairedArtifacts != 1 || report.AmbiguousIdentities != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	var repairableRepo, ambiguousRepo string
	if err := db.QueryRow(`SELECT repo_id FROM artifacts WHERE id = 'repairable'`).Scan(&repairableRepo); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT repo_id FROM artifacts WHERE id = 'ambiguous'`).Scan(&ambiguousRepo); err != nil {
		t.Fatal(err)
	}
	if repairableRepo != "target" || ambiguousRepo != "old" {
		t.Fatalf("unexpected ownership: repairable=%q ambiguous=%q", repairableRepo, ambiguousRepo)
	}
}
