package hubstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	sqlite "modernc.org/sqlite"
)

type Topic struct {
	ID                  string     `json:"topic_id"`
	ScopeID             string     `json:"repo_scope_id"`
	Key                 string     `json:"key"`
	Name                string     `json:"name"`
	Description         string     `json:"description"`
	OwnerActorID        string     `json:"owner_actor_id"`
	CreatedAt           time.Time  `json:"created_at"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	ArchivedAt          *time.Time `json:"archived_at,omitempty"`
	ArchiveActorID      string     `json:"archive_actor_id,omitempty"`
	ArchiveReason       string     `json:"archive_reason,omitempty"`
	PolicyGeneration    int64      `json:"policy_generation"`
	State               string     `json:"state"`
	EffectiveArchivedAt *time.Time `json:"effective_archived_at,omitempty"`
	EffectiveReason     string     `json:"effective_reason,omitempty"`
}

type TopicInput struct {
	Key         string
	Name        string
	Description string
	ExpiresAt   *time.Time
}

type TopicEdit struct {
	Name               string
	Description        string
	ChangeExpiry       bool
	ExpiresAt          *time.Time
	ExpectedGeneration int64
}

type TopicList struct {
	Query           string
	IncludeArchived bool
	Limit           int
	Offset          int
}

type TopicAudit struct {
	ID       string          `json:"audit_id"`
	TopicID  string          `json:"topic_id"`
	ActorID  string          `json:"actor_id"`
	Action   string          `json:"action"`
	At       time.Time       `json:"at"`
	OldValue json.RawMessage `json:"old_value"`
	NewValue json.RawMessage `json:"new_value"`
}

func uniqueOrOriginal(err error) error {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) && (sqliteErr.Code() == 2067 || sqliteErr.Code() == 1555) {
		return ErrConflict
	}
	return err
}

const topicColumns = "topic_id, scope_id, key, name, description, owner_actor_id, created_at, expires_at, archived_at, archive_actor_id, archive_reason, policy_generation"

type rowScanner interface{ Scan(...any) error }

func scanTopic(row rowScanner, now int64) (Topic, error) {
	var t Topic
	var created int64
	var expiry, archive sql.NullInt64
	var actor, reason sql.NullString
	err := row.Scan(&t.ID, &t.ScopeID, &t.Key, &t.Name, &t.Description, &t.OwnerActorID, &created, &expiry, &archive, &actor, &reason, &t.PolicyGeneration)
	if err != nil {
		return Topic{}, err
	}
	t.CreatedAt = fromMillis(created)
	if expiry.Valid {
		v := fromMillis(expiry.Int64)
		t.ExpiresAt = &v
	}
	if archive.Valid {
		v := fromMillis(archive.Int64)
		t.ArchivedAt = &v
	}
	t.ArchiveActorID = actor.String
	t.ArchiveReason = reason.String
	t.State = "active"
	if expiry.Valid && expiry.Int64 <= now && (!archive.Valid || expiry.Int64 <= archive.Int64) {
		t.State = "archived"
		t.EffectiveArchivedAt = t.ExpiresAt
		t.EffectiveReason = "expired"
	} else if archive.Valid {
		t.State = "archived"
		t.EffectiveArchivedAt = t.ArchivedAt
		t.EffectiveReason = "manual"
	}
	return t, nil
}

func fromMillis(v int64) time.Time { return time.UnixMilli(v).UTC() }

func validKey(key string) bool {
	if len(key) < 1 || len(key) > 32 || key[0] < 'a' || key[0] > 'z' || key[len(key)-1] == '-' {
		return false
	}
	prevHyphen := false
	for _, c := range key {
		if c == '-' {
			if prevHyphen {
				return false
			}
			prevHyphen = true
			continue
		}
		prevHyphen = false
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func cleanText(s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) < 1 || utf8.RuneCountInString(s) > max {
		return "", ErrInvalidInput
	}
	printable := false
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", ErrInvalidInput
		}
		if !unicode.IsSpace(r) {
			printable = true
		}
	}
	if !printable {
		return "", ErrInvalidInput
	}
	return s, nil
}

func expiryMillis(expiry *time.Time, now int64) (any, error) {
	if expiry == nil {
		return nil, nil
	}
	if expiry.IsZero() || expiry.Year() < 1970 || expiry.UnixMilli() <= now {
		return nil, ErrExpired
	}
	return expiry.UTC().UnixMilli(), nil
}

func (d *DB) boundScope(ctx context.Context, tx *sql.Tx, ev repoEvidence) (string, error) {
	if err := checkEvidence(ev); err != nil {
		return "", err
	}
	var scope, marker, identity string
	err := tx.QueryRowContext(ctx, "SELECT scope_id, marker, file_identity FROM scope_bindings WHERE common_dir=?", ev.key).Scan(&scope, &marker, &identity)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if marker != ev.marker || identity != ev.identity {
		return "", ErrBindingConflict
	}
	return scope, nil
}

func (d *DB) CreateTopic(ctx context.Context, repoPath, actorID string, input TopicInput) (Topic, error) {
	if !validKey(input.Key) {
		return Topic{}, ErrInvalidInput
	}
	name, err := cleanText(input.Name, 120)
	if err != nil {
		return Topic{}, err
	}
	description, err := cleanText(input.Description, 1000)
	if err != nil {
		return Topic{}, err
	}
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return Topic{}, err
	}
	var out Topic
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		if err := enrolled(ctx, tx, actorID); err != nil {
			return err
		}
		expiry, err := expiryMillis(input.ExpiresAt, now)
		if err != nil {
			return err
		}
		id, err := randomID()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO topics (topic_id, scope_id, key, name, description, owner_actor_id, created_at, expires_at, policy_generation) VALUES (?,?,?,?,?,?,?,?,1)", id, scope, input.Key, name, description, actorID, now, expiry)
		if err != nil {
			return uniqueOrOriginal(err)
		}
		out, err = scanTopic(tx.QueryRowContext(ctx, "SELECT "+topicColumns+" FROM topics WHERE topic_id=?", id), now)
		if err != nil {
			return err
		}
		return appendAudit(ctx, tx, id, actorID, "create", now, nil, out)
	})
	return out, err
}

func enrolled(ctx context.Context, tx *sql.Tx, actorID string) error {
	var id string
	err := tx.QueryRowContext(ctx, "SELECT actor_id FROM actors WHERE actor_id=?", actorID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUnauthorized
	}
	return err
}

func appendAudit(ctx context.Context, tx *sql.Tx, topicID, actorID, action string, at int64, oldValue, newValue any) error {
	oldJSON, err := json.Marshal(oldValue)
	if err != nil {
		return err
	}
	newJSON, err := json.Marshal(newValue)
	if err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO topic_audit VALUES (?,?,?,?,?,?,?)", id, topicID, actorID, action, at, string(oldJSON), string(newJSON))
	return err
}

func (d *DB) ShowTopic(ctx context.Context, repoPath, topicID string) (Topic, error) {
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return Topic{}, err
	}
	now := d.now().UTC().UnixMilli()
	t, err := scanTopic(d.sql.QueryRowContext(ctx, "SELECT "+topicColumns+" FROM topics WHERE scope_id=? AND topic_id=?", scope.ID, topicID), now)
	if errors.Is(err, sql.ErrNoRows) {
		return Topic{}, ErrNotFound
	}
	return t, err
}

// ListTopics has stable newest-first ordering and bounds each page to 100 rows.
func (d *DB) ListTopics(ctx context.Context, repoPath string, opts TopicList) ([]Topic, error) {
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	if opts.Limit == 0 {
		opts.Limit = 50
	}
	if opts.Limit < 1 || opts.Limit > 100 || opts.Offset < 0 {
		return nil, ErrInvalidInput
	}
	query := strings.TrimSpace(opts.Query)
	if len(query) > 256 {
		return nil, ErrInvalidInput
	}
	now := d.now().UTC().UnixMilli()
	sqlText := "SELECT " + topicColumns + " FROM topics WHERE scope_id=?"
	args := []any{scope.ID}
	if !opts.IncludeArchived {
		sqlText += " AND archived_at IS NULL AND (expires_at IS NULL OR expires_at>?)"
		args = append(args, now)
	}
	if query != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(query))
		sqlText += ` AND (lower(key) LIKE ? ESCAPE '\' OR lower(name) LIKE ? ESCAPE '\' OR lower(description) LIKE ? ESCAPE '\')`
		pattern := "%" + escaped + "%"
		args = append(args, pattern, pattern, pattern)
	}
	sqlText += " ORDER BY created_at DESC, topic_id DESC LIMIT ? OFFSET ?"
	args = append(args, opts.Limit, opts.Offset)
	rows, err := d.sql.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Topic, 0)
	for rows.Next() {
		t, err := scanTopic(rows, now)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListTopicAudit returns available authority changes in commit order.
func (d *DB) ListTopicAudit(ctx context.Context, repoPath, topicID string, limit, offset int) ([]TopicAudit, error) {
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrInvalidInput
	}
	var exists string
	err = d.sql.QueryRowContext(ctx, "SELECT topic_id FROM topics WHERE scope_id=? AND topic_id=?", scope.ID, topicID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := d.sql.QueryContext(ctx, "SELECT audit_id, topic_id, actor_id, action, at, old_value, new_value FROM topic_audit WHERE topic_id=? ORDER BY at, rowid LIMIT ? OFFSET ?", topicID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TopicAudit, 0)
	for rows.Next() {
		var a TopicAudit
		var at int64
		var oldJSON, newJSON string
		if err := rows.Scan(&a.ID, &a.TopicID, &a.ActorID, &a.Action, &at, &oldJSON, &newJSON); err != nil {
			return nil, err
		}
		a.At = fromMillis(at)
		a.OldValue = json.RawMessage(oldJSON)
		a.NewValue = json.RawMessage(newJSON)
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListMaintainers inspects delegated editors without granting write rights.
func (d *DB) ListMaintainers(ctx context.Context, repoPath, topicID string, limit, offset int) ([]string, error) {
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrInvalidInput
	}
	var exists string
	err = d.sql.QueryRowContext(ctx, "SELECT topic_id FROM topics WHERE scope_id=? AND topic_id=?", scope.ID, topicID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := d.sql.QueryContext(ctx, "SELECT actor_id FROM topic_roles WHERE topic_id=? AND role='maintainer' ORDER BY actor_id LIMIT ? OFFSET ?", topicID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var actorID string
		if err := rows.Scan(&actorID); err != nil {
			return nil, err
		}
		out = append(out, actorID)
	}
	return out, rows.Err()
}

func (d *DB) topicForWrite(ctx context.Context, tx *sql.Tx, scope, topicID, actorID string, expected int64, now int64, ownerOnly bool) (Topic, error) {
	t, err := scanTopic(tx.QueryRowContext(ctx, "SELECT "+topicColumns+" FROM topics WHERE scope_id=? AND topic_id=?", scope, topicID), now)
	if errors.Is(err, sql.ErrNoRows) {
		return Topic{}, ErrNotFound
	}
	if err != nil {
		return Topic{}, err
	}
	if expected < 1 || t.PolicyGeneration != expected {
		return Topic{}, ErrConflict
	}
	if err := enrolled(ctx, tx, actorID); err != nil {
		return Topic{}, err
	}
	if t.OwnerActorID == actorID {
		return t, nil
	}
	if ownerOnly {
		return Topic{}, ErrUnauthorized
	}
	var role string
	err = tx.QueryRowContext(ctx, "SELECT role FROM topic_roles WHERE topic_id=? AND actor_id=? AND role='maintainer'", topicID, actorID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return Topic{}, ErrUnauthorized
	}
	return t, err
}

// EditTopic changes discovery text and, when ChangeExpiry is true, the deadline.
// ExpectedGeneration is a compare-and-swap guard for concurrent policy edits.
func (d *DB) EditTopic(ctx context.Context, repoPath, topicID, actorID string, edit TopicEdit) (Topic, error) {
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return Topic{}, err
	}
	var name, description string
	if edit.Name != "" {
		name, err = cleanText(edit.Name, 120)
		if err != nil {
			return Topic{}, err
		}
	}
	if edit.Description != "" {
		description, err = cleanText(edit.Description, 1000)
		if err != nil {
			return Topic{}, err
		}
	}
	if name == "" && description == "" && !edit.ChangeExpiry {
		return Topic{}, ErrInvalidInput
	}
	var out Topic
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		old, err := d.topicForWrite(ctx, tx, scope, topicID, actorID, edit.ExpectedGeneration, now, false)
		if err != nil {
			return err
		}
		if old.State != "active" {
			return ErrArchived
		}
		if name == "" {
			name = old.Name
		}
		if description == "" {
			description = old.Description
		}
		var expiry any
		if edit.ChangeExpiry {
			expiry, err = expiryMillis(edit.ExpiresAt, now)
			if err != nil {
				return err
			}
		} else if old.ExpiresAt != nil {
			expiry = old.ExpiresAt.UnixMilli()
		}
		if _, err := tx.ExecContext(ctx, "UPDATE topics SET name=?, description=?, expires_at=?, policy_generation=policy_generation+1 WHERE topic_id=?", name, description, expiry, topicID); err != nil {
			return err
		}
		out, err = scanTopic(tx.QueryRowContext(ctx, "SELECT "+topicColumns+" FROM topics WHERE topic_id=?", topicID), now)
		if err != nil {
			return err
		}
		return appendAudit(ctx, tx, topicID, actorID, "edit", now, old, out)
	})
	return out, err
}

// ArchiveTopic keeps the topic and its key. An already elapsed deadline remains
// the earlier effective archive instant even when a manual archive is recorded.
func (d *DB) ArchiveTopic(ctx context.Context, repoPath, topicID, actorID string, expectedGeneration int64, reason string) (Topic, error) {
	reason, err := cleanText(reason, 1000)
	if err != nil {
		return Topic{}, err
	}
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return Topic{}, err
	}
	var out Topic
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		old, err := d.topicForWrite(ctx, tx, scope, topicID, actorID, expectedGeneration, now, false)
		if err != nil {
			return err
		}
		if old.ArchivedAt != nil {
			return ErrArchived
		}
		if _, err := tx.ExecContext(ctx, "UPDATE topics SET archived_at=?, archive_actor_id=?, archive_reason=?, policy_generation=policy_generation+1 WHERE topic_id=?", now, actorID, reason, topicID); err != nil {
			return err
		}
		out, err = scanTopic(tx.QueryRowContext(ctx, "SELECT "+topicColumns+" FROM topics WHERE topic_id=?", topicID), now)
		if err != nil {
			return err
		}
		return appendAudit(ctx, tx, topicID, actorID, "archive", now, old, out)
	})
	return out, err
}

// RestoreTopic requires an explicit reason. An elapsed deadline must be
// cleared or extended in the same transaction.
func (d *DB) RestoreTopic(ctx context.Context, repoPath, topicID, actorID string, expectedGeneration int64, reason string, changeExpiry bool, expiresAt *time.Time) (Topic, error) {
	reason, err := cleanText(reason, 1000)
	if err != nil {
		return Topic{}, err
	}
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return Topic{}, err
	}
	var out Topic
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		old, err := d.topicForWrite(ctx, tx, scope, topicID, actorID, expectedGeneration, now, false)
		if err != nil {
			return err
		}
		if old.State == "active" {
			return ErrConflict
		}
		var expiry any
		if changeExpiry {
			expiry, err = expiryMillis(expiresAt, now)
			if err != nil {
				return err
			}
		} else if old.ExpiresAt != nil {
			if old.ExpiresAt.UnixMilli() <= now {
				return ErrExpired
			}
			expiry = old.ExpiresAt.UnixMilli()
		}
		if _, err := tx.ExecContext(ctx, "UPDATE topics SET expires_at=?, archived_at=NULL, archive_actor_id=NULL, archive_reason=NULL, policy_generation=policy_generation+1 WHERE topic_id=?", expiry, topicID); err != nil {
			return err
		}
		out, err = scanTopic(tx.QueryRowContext(ctx, "SELECT "+topicColumns+" FROM topics WHERE topic_id=?", topicID), now)
		if err != nil {
			return err
		}
		return appendAudit(ctx, tx, topicID, actorID, "restore", now, old, map[string]any{"topic": out, "reason": reason})
	})
	return out, err
}

// SetMaintainer is owner-only; maintainers cannot delegate their authority.
func (d *DB) SetMaintainer(ctx context.Context, repoPath, topicID, ownerID, actorID string, expectedGeneration int64, grant bool) (Topic, error) {
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return Topic{}, err
	}
	var out Topic
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		old, err := d.topicForWrite(ctx, tx, scope, topicID, ownerID, expectedGeneration, now, true)
		if err != nil {
			return err
		}
		if old.State != "active" {
			return ErrArchived
		}
		if actorID == ownerID {
			return ErrInvalidInput
		}
		if err := enrolled(ctx, tx, actorID); err != nil {
			return err
		}
		if grant {
			_, err = tx.ExecContext(ctx, "INSERT INTO topic_roles VALUES (?,?,'maintainer')", topicID, actorID)
		} else {
			var result sql.Result
			result, err = tx.ExecContext(ctx, "DELETE FROM topic_roles WHERE topic_id=? AND actor_id=? AND role='maintainer'", topicID, actorID)
			if err == nil {
				var n int64
				n, err = result.RowsAffected()
				if n == 0 {
					return ErrNotFound
				}
			}
		}
		if err != nil {
			return uniqueOrOriginal(err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE topics SET policy_generation=policy_generation+1 WHERE topic_id=?", topicID); err != nil {
			return err
		}
		out, err = scanTopic(tx.QueryRowContext(ctx, "SELECT "+topicColumns+" FROM topics WHERE topic_id=?", topicID), now)
		if err != nil {
			return err
		}
		action := "revoke_maintainer"
		if grant {
			action = "grant_maintainer"
		}
		return appendAudit(ctx, tx, topicID, ownerID, action, now, map[string]string{"actor_id": actorID}, out)
	})
	return out, err
}
