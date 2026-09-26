package hubstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PruneBatchLimit bounds one explicit operation and its JSON report. Callers
// may run another operation with the same cutoff when More is true.
const PruneBatchLimit = 10000

type PruneScope struct {
	ScopeID      string           `json:"scope_id"`
	Entries      int              `json:"entries"`
	PayloadBytes int64            `json:"payload_bytes"`
	Gaps         []PublicationGap `json:"gaps"`
}

type PruneReport struct {
	Cutoff         time.Time    `json:"cutoff"`
	DryRun         bool         `json:"dry_run"`
	Entries        int          `json:"entries"`
	PayloadBytes   int64        `json:"payload_bytes"`
	More           bool         `json:"more"`
	PlanDigest     string       `json:"plan_digest"`
	Scopes         []PruneScope `json:"scopes"`
	BackupPath     string       `json:"backup_path,omitempty"`
	RetentionEpoch int64        `json:"retention_epoch"`
}

type pruneEntry struct {
	id, scope, topic, actor, key, kind string
	sequence, bytes                    int64
}

// PlanHubPrune reports only physically eligible publications. A retained
// current message revision or event correction target is never counted.
func (d *DB) PlanHubPrune(ctx context.Context, cutoff time.Time) (PruneReport, error) {
	if err := validPruneCutoff(cutoff); err != nil {
		return PruneReport{}, err
	}
	if err := d.probe(ctx); err != nil {
		return PruneReport{}, err
	}
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PruneReport{}, classifySQLite(err)
	}
	defer tx.Rollback()
	entries, more, err := pruneCandidates(ctx, tx, cutoff.UnixMilli())
	if err != nil {
		return PruneReport{}, classifySQLite(err)
	}
	r, err := pruneReport(ctx, tx, cutoff, entries, more, true)
	if err != nil {
		return PruneReport{}, err
	}
	return r, classifySQLite(tx.Commit())
}

// PruneHub makes a verified SQLite snapshot before a single atomic deletion.
// It never vacuums the live database or affects the rebuildable source index.
func (d *DB) PruneHub(ctx context.Context, cutoff time.Time) (PruneReport, error) {
	if d.readOnly {
		return PruneReport{}, ErrUnauthorized
	}
	if err := validPruneCutoff(cutoff); err != nil {
		return PruneReport{}, err
	}
	if !cutoff.Before(d.now().UTC()) {
		return PruneReport{}, ErrInvalidInput
	}
	plan, err := d.PlanHubPrune(ctx, cutoff)
	if err != nil {
		return PruneReport{}, err
	}
	if plan.Entries == 0 {
		plan.DryRun = false
		return plan, err
	}
	var backup string
	var report PruneReport
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		if cutoff.UnixMilli() >= now {
			return ErrInvalidInput
		}
		var err error
		backup, err = d.backupForPrune(ctx)
		if err != nil {
			return err
		}
		entries, more, err := pruneCandidates(ctx, tx, cutoff.UnixMilli())
		if err != nil {
			return err
		}
		if more != plan.More || len(entries) != plan.Entries || candidateDigest(entries) != plan.PlanDigest {
			return ErrBusyRetryable
		}
		report, err = pruneReport(ctx, tx, cutoff, entries, more, false)
		if err != nil {
			return err
		}
		if report.PayloadBytes != plan.PayloadBytes {
			return ErrBusyRetryable
		}
		id, err := randomID()
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE retention_permission SET enabled=1 WHERE singleton=1"); err != nil {
			return err
		}
		for _, e := range entries {
			if e.key != "" {
				if _, err = tx.ExecContext(ctx, "INSERT INTO pruned_idempotency VALUES (?,?,?,?,?)", e.scope, e.topic, e.actor, e.key, id); err != nil {
					return err
				}
				if _, err = tx.ExecContext(ctx, "DELETE FROM publication_idempotency WHERE scope_id=? AND topic_id=? AND actor_id=? AND key=?", e.scope, e.topic, e.actor, e.key); err != nil {
					return err
				}
			}
			if e.kind == "message" {
				if _, err = tx.ExecContext(ctx, "DELETE FROM message_pin_audit WHERE (message_id,revision) IN (SELECT message_id,revision FROM message_revisions WHERE entry_id=?)", e.id); err != nil {
					return err
				}
				if _, err = tx.ExecContext(ctx, "DELETE FROM message_revisions WHERE entry_id=?", e.id); err != nil {
					return err
				}
			} else {
				if _, err = tx.ExecContext(ctx, "DELETE FROM event_entries WHERE entry_id=?", e.id); err != nil {
					return err
				}
			}
			res, deleteErr := tx.ExecContext(ctx, "DELETE FROM publications WHERE entry_id=?", e.id)
			if deleteErr != nil {
				return deleteErr
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return ErrConflict
			}
		}
		for _, scope := range report.Scopes {
			for _, gap := range scope.Gaps {
				if err := mergePrunedRange(ctx, tx, scope.ScopeID, gap.From, gap.To, id); err != nil {
					return err
				}
			}
		}
		if _, err = tx.ExecContext(ctx, "UPDATE retention_permission SET enabled=0 WHERE singleton=1"); err != nil {
			return err
		}
		for _, s := range report.Scopes {
			if _, err = tx.ExecContext(ctx, `UPDATE repo_scopes SET low_water_sequence=COALESCE((SELECT MIN(sequence)-1 FROM publications WHERE scope_id=?),(SELECT next_sequence-1 FROM scope_counters WHERE scope_id=?)) WHERE scope_id=? AND low_water_sequence < COALESCE((SELECT MIN(sequence)-1 FROM publications WHERE scope_id=?),(SELECT next_sequence-1 FROM scope_counters WHERE scope_id=?))`, s.ScopeID, s.ScopeID, s.ScopeID, s.ScopeID, s.ScopeID); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, "UPDATE hub_meta SET retention_epoch=retention_epoch+1 WHERE singleton=1"); err != nil {
			return err
		}
		if err = tx.QueryRowContext(ctx, "SELECT retention_epoch FROM hub_meta WHERE singleton=1").Scan(&report.RetentionEpoch); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO prune_audit (prune_id,cutoff_at,pruned_at,backup_path,entry_count,payload_bytes,retention_epoch) VALUES (?,?,?,?,?,?,?)", id, cutoff.UnixMilli(), now, backup, report.Entries, report.PayloadBytes, report.RetentionEpoch); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		var committed int
		if backup != "" {
			if probeErr := d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM prune_audit WHERE backup_path=?", backup).Scan(&committed); probeErr == nil && committed == 0 {
				_ = os.Remove(backup)
			}
		}
		return PruneReport{}, err
	}
	report.BackupPath = backup
	d.cleanupPruneBackups(ctx, backup, report.RetentionEpoch)
	return report, nil
}

func validPruneCutoff(cutoff time.Time) error {
	if cutoff.IsZero() || cutoff.Location() != time.UTC || cutoff.UnixMilli() <= 0 {
		return ErrInvalidInput
	}
	return nil
}

func pruneCandidates(ctx context.Context, tx *sql.Tx, cutoff int64) ([]pruneEntry, bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT p.entry_id,p.scope_id,p.topic_id,p.actor_id,COALESCE(p.idempotency_key,''),p.kind,p.sequence,
	 length(CAST(p.source_refs AS BLOB))+length(CAST(p.correlation_refs AS BLOB))+COALESCE(length(CAST(e.payload_json AS BLOB)),0)+COALESCE(length(CAST(r.text AS BLOB)),0)
	 FROM publications p LEFT JOIN event_entries e ON e.entry_id=p.entry_id LEFT JOIN message_revisions r ON r.entry_id=p.entry_id
	 WHERE p.committed_at<? AND NOT EXISTS (SELECT 1 FROM message_heads h WHERE h.current_entry_id=p.entry_id)
	 AND NOT EXISTS (SELECT 1 FROM event_entries corrected WHERE corrected.corrects_entry_id=p.entry_id)
	 ORDER BY p.scope_id,p.sequence LIMIT ?`, cutoff, PruneBatchLimit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	entries := make([]pruneEntry, 0)
	for rows.Next() {
		var e pruneEntry
		if err := rows.Scan(&e.id, &e.scope, &e.topic, &e.actor, &e.key, &e.kind, &e.sequence, &e.bytes); err != nil {
			return nil, false, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(entries) > PruneBatchLimit
	if more {
		entries = entries[:PruneBatchLimit]
	}
	return entries, more, nil
}

func pruneReport(ctx context.Context, tx *sql.Tx, cutoff time.Time, entries []pruneEntry, more, dry bool) (PruneReport, error) {
	r := PruneReport{Cutoff: cutoff, DryRun: dry, Entries: len(entries), More: more, PlanDigest: candidateDigest(entries), Scopes: []PruneScope{}}
	if err := tx.QueryRowContext(ctx, "SELECT retention_epoch FROM hub_meta WHERE singleton=1").Scan(&r.RetentionEpoch); err != nil {
		return r, err
	}
	for _, e := range entries {
		r.PayloadBytes += e.bytes
		if len(r.Scopes) == 0 || r.Scopes[len(r.Scopes)-1].ScopeID != e.scope {
			r.Scopes = append(r.Scopes, PruneScope{ScopeID: e.scope, Gaps: []PublicationGap{}})
		}
		s := &r.Scopes[len(r.Scopes)-1]
		s.Entries++
		s.PayloadBytes += e.bytes
		addGap(&s.Gaps, e.sequence, e.sequence, "pruned")
	}
	return r, nil
}

func candidateDigest(entries []pruneEntry) string {
	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%q\t%q\t%q\t%q\t%q\t%q\t%d\t%d\n", e.id, e.scope, e.topic, e.actor, e.key, e.kind, e.sequence, e.bytes)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func mergePrunedRange(ctx context.Context, tx *sql.Tx, scope string, first, last int64, pruneID string) error {
	// The new gap may bridge ranges from earlier batches. Iterate until no
	// adjacent range remains, then replace them with one durable interval.
	for {
		rows, err := tx.QueryContext(ctx, "SELECT from_sequence,to_sequence FROM pruned_ranges WHERE scope_id=? AND to_sequence>=? AND from_sequence<=?", scope, first-1, last+1)
		if err != nil {
			return err
		}
		changed := false
		for rows.Next() {
			var a, b int64
			if err := rows.Scan(&a, &b); err != nil {
				rows.Close()
				return err
			}
			if a < first {
				first = a
				changed = true
			}
			if b > last {
				last = b
				changed = true
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if !changed {
			break
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM pruned_ranges WHERE scope_id=? AND to_sequence>=? AND from_sequence<=?", scope, first-1, last+1); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO pruned_ranges VALUES (?,?,?,?)", scope, first, last, pruneID)
	return err
}

func (d *DB) cleanupPruneBackups(ctx context.Context, keep string, epoch int64) {
	rows, err := d.sql.QueryContext(ctx, "SELECT prune_id,backup_path FROM prune_audit WHERE backup_available=1 AND retention_epoch<? AND backup_path<>?", epoch, keep)
	if err != nil {
		return
	}
	type oldBackup struct{ id, path string }
	var old []oldBackup
	for rows.Next() {
		var b oldBackup
		if rows.Scan(&b.id, &b.path) != nil {
			rows.Close()
			return
		}
		old = append(old, b)
	}
	if rows.Err() != nil {
		rows.Close()
		return
	}
	rows.Close()
	for _, b := range old {
		base := filepath.Base(b.path)
		id := strings.TrimSuffix(strings.TrimPrefix(base, "hub-prune-"), ".sqlite")
		if filepath.Dir(b.path) != filepath.Dir(d.path) || !strings.HasPrefix(base, "hub-prune-") || !strings.HasSuffix(base, ".sqlite") || len(id) != 32 {
			continue
		}
		if _, err := hex.DecodeString(id); err != nil {
			continue
		}
		if err := os.Remove(b.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			continue
		}
		_, _ = d.sql.ExecContext(ctx, "UPDATE prune_audit SET backup_available=0 WHERE prune_id=? AND backup_path=?", b.id, b.path)
	}
}

func (d *DB) backupForPrune(ctx context.Context) (out string, err error) {
	var candidate string
	defer func() {
		if err != nil && candidate != "" {
			_ = os.Remove(candidate)
		}
	}()
	source, err := sql.Open("sqlite", sqliteDSN(d.path, "ro"))
	if err != nil {
		return "", err
	}
	defer source.Close()
	var sourceIntegrity string
	if err := source.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&sourceIntegrity); err != nil {
		return "", err
	}
	if sourceIntegrity != "ok" {
		return "", ErrUnsupportedFormat
	}
	sourceFK, err := source.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return "", err
	}
	if sourceFK.Next() {
		sourceFK.Close()
		return "", ErrUnsupportedFormat
	}
	if err = sourceFK.Err(); err != nil {
		sourceFK.Close()
		return "", err
	}
	sourceFK.Close()
	info, err := os.Stat(d.path)
	if err != nil {
		return "", err
	}
	free, err := availableDiskBytes(filepath.Dir(d.path))
	if err != nil {
		return "", err
	}
	if !sufficientBackupHeadroom(free, uint64(info.Size())) {
		return "", fmt.Errorf("hub backup needs disk headroom: %w", ErrBusyRetryable)
	}
	id, err := randomID()
	if err != nil {
		return "", err
	}
	candidate = filepath.Join(filepath.Dir(d.path), "hub-prune-"+id+".sqlite")
	quoted := strings.ReplaceAll(candidate, "'", "''")
	if _, err = source.ExecContext(ctx, "VACUUM INTO '"+quoted+"'"); err != nil {
		return "", classifySQLite(err)
	}
	file, err := os.OpenFile(candidate, os.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	if syncErr := file.Sync(); syncErr != nil {
		file.Close()
		return "", syncErr
	}
	if err = file.Close(); err != nil {
		return "", err
	}
	backup, err := sql.Open("sqlite", sqliteDSN(candidate, "ro"))
	if err != nil {
		return "", err
	}
	defer backup.Close()
	var integrity string
	if err = backup.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return "", err
	}
	if integrity != "ok" {
		return "", ErrUnsupportedFormat
	}
	var sourceID, backupID string
	if err = source.QueryRowContext(ctx, "SELECT db_id FROM hub_meta WHERE singleton=1").Scan(&sourceID); err != nil {
		return "", err
	}
	if err = backup.QueryRowContext(ctx, "SELECT db_id FROM hub_meta WHERE singleton=1").Scan(&backupID); err != nil {
		return "", err
	}
	if sourceID != backupID {
		return "", ErrUnsupportedFormat
	}
	rows, err := backup.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if rows.Next() {
		return "", ErrUnsupportedFormat
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	return candidate, nil
}

func sufficientBackupHeadroom(free, dbBytes uint64) bool {
	const reserve = 64 * 1024 * 1024
	return free >= reserve && dbBytes <= (free-reserve)/2
}

func prunedKey(ctx context.Context, tx *sql.Tx, scope, topic, actor, key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	var one int
	err := tx.QueryRowContext(ctx, "SELECT 1 FROM pruned_idempotency WHERE scope_id=? AND topic_id=? AND actor_id=? AND key=?", scope, topic, actor, key).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func addMissingGaps(ctx context.Context, tx *sql.Tx, gaps *[]PublicationGap, scope string, from, to int64) error {
	if from > to {
		return nil
	}
	rows, err := tx.QueryContext(ctx, "SELECT from_sequence,to_sequence FROM pruned_ranges WHERE scope_id=? AND to_sequence>=? AND from_sequence<=? ORDER BY from_sequence", scope, from, to)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var first, last int64
		if err := rows.Scan(&first, &last); err != nil {
			return err
		}
		if first > from {
			addGap(gaps, from, first-1, "unavailable")
		}
		if first < from {
			first = from
		}
		if last > to {
			last = to
		}
		addGap(gaps, first, last, "pruned")
		from = last + 1
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if from <= to {
		addGap(gaps, from, to, "unavailable")
	}
	return nil
}
