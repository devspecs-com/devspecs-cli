package hubstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// SourceRef is an attributed pointer to evidence, not a verified fact. A
// hub_entry reference names a retained publication in its exact scope.
type SourceRef struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
	ScopeID   string `json:"repo_scope_id,omitempty"`
}

type Publication struct {
	EntryID         string      `json:"entry_id"`
	ScopeID         string      `json:"repo_scope_id"`
	TopicID         string      `json:"topic_id"`
	ActorID         string      `json:"actor_id"`
	Kind            string      `json:"kind"`
	Sequence        int64       `json:"sequence"`
	CommittedAt     time.Time   `json:"committed_at"`
	OccurredAt      *time.Time  `json:"occurred_at,omitempty"`
	ExpiresAt       *time.Time  `json:"expires_at,omitempty"`
	SourceRefs      []SourceRef `json:"source_refs"`
	CorrelationRefs []string    `json:"correlation_refs"`
	IdempotencyKey  string      `json:"idempotency_key,omitempty"`
	Replayed        bool        `json:"replayed"`
}

type Message struct {
	Publication
	MessageID string `json:"message_id"`
	Revision  int64  `json:"revision"`
	Text      string `json:"text"`
	Pinned    bool   `json:"pinned"`
	Upvotes   int64  `json:"upvotes"`
	Downvotes int64  `json:"downvotes"`
	Score     int64  `json:"score"`
}

type MessageInput struct {
	AuthorityID     string
	Text            string
	ExpiresAt       *time.Time
	OccurredAt      *time.Time
	SourceRefs      []SourceRef
	CorrelationRefs []string
	IdempotencyKey  string
}

type MessageRevisionInput struct {
	MessageInput
	ExpectedRevision int64
	ChangeExpiry     bool
}

type MessageList struct {
	Limit  int
	Offset int
	Ranked bool
}

const messageColumns = "p.entry_id,p.scope_id,p.topic_id,p.actor_id,p.kind,p.sequence,p.committed_at,p.occurred_at,p.expires_at,p.source_refs,p.correlation_refs,p.idempotency_key,r.message_id,r.revision,r.text,h.pinned,h.current_entry_id," +
	"(SELECT COUNT(*) FROM message_votes v WHERE v.message_id=r.message_id AND v.revision=r.revision AND v.value=1)," +
	"(SELECT COUNT(*) FROM message_votes v WHERE v.message_id=r.message_id AND v.revision=r.revision AND v.value=-1)"

const messageScore = "(SELECT COALESCE(SUM(v.value),0) FROM message_votes v WHERE v.message_id=r.message_id AND v.revision=r.revision)"

func scanMessage(row rowScanner, extra ...any) (Message, error) {
	var m Message
	var committed int64
	var occurred, expiry sql.NullInt64
	var key sql.NullString
	var sources, correlations string
	var pinned int
	var currentID string
	targets := []any{&m.EntryID, &m.ScopeID, &m.TopicID, &m.ActorID, &m.Kind, &m.Sequence, &committed, &occurred, &expiry, &sources, &correlations, &key, &m.MessageID, &m.Revision, &m.Text, &pinned, &currentID, &m.Upvotes, &m.Downvotes}
	err := row.Scan(append(targets, extra...)...)
	if err != nil {
		return Message{}, err
	}
	m.CommittedAt = fromMillis(committed)
	if occurred.Valid {
		v := fromMillis(occurred.Int64)
		m.OccurredAt = &v
	}
	if expiry.Valid {
		v := fromMillis(expiry.Int64)
		m.ExpiresAt = &v
	}
	if err := json.Unmarshal([]byte(sources), &m.SourceRefs); err != nil {
		return Message{}, ErrUnsupportedFormat
	}
	if err := json.Unmarshal([]byte(correlations), &m.CorrelationRefs); err != nil {
		return Message{}, ErrUnsupportedFormat
	}
	m.IdempotencyKey = key.String
	m.Pinned = pinned == 1 && currentID == m.EntryID
	m.Score = m.Upvotes - m.Downvotes
	return m, nil
}

func validMessageText(s string) bool {
	if len(s) == 0 || len(s) > 64*1024 || !utf8.ValidString(s) {
		return false
	}
	meaningful := false
	for _, r := range s {
		if r == 0 || (unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t') {
			return false
		}
		if !unicode.IsSpace(r) {
			meaningful = true
		}
	}
	return meaningful
}

func validateRefs(sources []SourceRef, correlations []string) error {
	if len(sources) > 16 || len(correlations) > 16 {
		return ErrInvalidInput
	}
	for _, s := range sources {
		if s.Kind != "hub_entry" && s.Kind != "git_commit" && s.Kind != "document" && s.Kind != "url" {
			return ErrInvalidInput
		}
		if !validRefText(s.Reference, 512) {
			return ErrInvalidInput
		}
		if s.Kind == "hub_entry" {
			if len(s.ScopeID) != 32 || !isHex(s.ScopeID) || len(s.Reference) != 32 || !isHex(s.Reference) {
				return ErrInvalidInput
			}
		} else if s.ScopeID != "" {
			return ErrInvalidInput
		}
	}
	for _, s := range correlations {
		if !validRefText(s, 256) {
			return ErrInvalidInput
		}
	}
	return nil
}

func validRefText(s string, max int) bool {
	if len(s) == 0 || len(s) > max || !utf8.ValidString(s) || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validateMessageInput(in MessageInput) error {
	if !validMessageText(in.Text) || len(in.IdempotencyKey) > 128 || !utf8.ValidString(in.IdempotencyKey) || strings.TrimSpace(in.IdempotencyKey) != in.IdempotencyKey {
		return ErrInvalidInput
	}
	if in.OccurredAt != nil && (in.OccurredAt.IsZero() || in.OccurredAt.Year() < 1970) {
		return ErrInvalidInput
	}
	if in.ExpiresAt != nil && (in.ExpiresAt.IsZero() || in.ExpiresAt.Year() < 1970) {
		return ErrInvalidInput
	}
	return validateRefs(in.SourceRefs, in.CorrelationRefs)
}

func normalizeMessageInput(in MessageInput) MessageInput {
	if in.SourceRefs == nil {
		in.SourceRefs = []SourceRef{}
	}
	if in.CorrelationRefs == nil {
		in.CorrelationRefs = []string{}
	}
	if in.ExpiresAt != nil {
		v := fromMillis(in.ExpiresAt.UnixMilli())
		in.ExpiresAt = &v
	}
	if in.OccurredAt != nil {
		v := fromMillis(in.OccurredAt.UnixMilli())
		in.OccurredAt = &v
	}
	return in
}

func fingerprint(in MessageInput, messageID string, expectedRevision int64, changeExpiry bool) (string, error) {
	// JSON field order is fixed by the struct, and nil versus explicit expiry is
	// retained. The fingerprint is of caller intent, before head resolution.
	b, err := json.Marshal(struct {
		Kind             string
		MessageID        string
		ExpectedRevision int64
		ChangeExpiry     bool
		Text             string
		ExpiresAt        *time.Time
		OccurredAt       *time.Time
		SourceRefs       []SourceRef
		CorrelationRefs  []string
	}{"message", messageID, expectedRevision, changeExpiry, in.Text, in.ExpiresAt, in.OccurredAt, in.SourceRefs, in.CorrelationRefs})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func (d *DB) expectedAuthority(id string) error {
	if id == "" {
		return ErrInvalidInput
	}
	if id != d.authorityID {
		return ErrReplayGap
	}
	return nil
}

func verifySourceRefs(ctx context.Context, tx *sql.Tx, sources []SourceRef) error {
	for _, ref := range sources {
		if ref.Kind != "hub_entry" {
			continue
		}
		var id string
		err := tx.QueryRowContext(ctx, "SELECT entry_id FROM publications WHERE scope_id=? AND entry_id=?", ref.ScopeID, ref.Reference).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) replayMessage(ctx context.Context, tx *sql.Tx, scope, topic, actor, key, fp string) (Message, bool, error) {
	if key == "" {
		return Message{}, false, nil
	}
	var stored, entry string
	err := tx.QueryRowContext(ctx, "SELECT fingerprint,entry_id FROM publication_idempotency WHERE scope_id=? AND topic_id=? AND actor_id=? AND key=?", scope, topic, actor, key).Scan(&stored, &entry)
	if errors.Is(err, sql.ErrNoRows) {
		purged, checkErr := prunedKey(ctx, tx, scope, topic, actor, key)
		if checkErr != nil {
			return Message{}, false, checkErr
		}
		if purged {
			return Message{}, true, ErrReplayGap
		}
		return Message{}, false, nil
	}
	if err != nil {
		return Message{}, false, err
	}
	if stored != fp {
		return Message{}, true, ErrIdempotencyConflict
	}
	m, err := scanMessage(tx.QueryRowContext(ctx, "SELECT "+messageColumns+" FROM publications p JOIN message_revisions r ON r.entry_id=p.entry_id JOIN message_heads h ON h.message_id=r.message_id WHERE p.entry_id=?", entry))
	m.Replayed = true
	return m, true, err
}

func nextSequence(ctx context.Context, tx *sql.Tx, scope string) (int64, error) {
	if _, err := tx.ExecContext(ctx, "INSERT INTO scope_counters (scope_id) VALUES (?) ON CONFLICT(scope_id) DO NOTHING", scope); err != nil {
		return 0, err
	}
	var next int64
	if err := tx.QueryRowContext(ctx, "SELECT next_sequence FROM scope_counters WHERE scope_id=?", scope).Scan(&next); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE scope_counters SET next_sequence=? WHERE scope_id=?", next+1, scope); err != nil {
		return 0, err
	}
	return next, nil
}

func (d *DB) appendMessage(ctx context.Context, tx *sql.Tx, now int64, scope, topicID, actorID, messageID string, revision int64, in MessageInput, expiry any, fp string) (Message, error) {
	if err := verifySourceRefs(ctx, tx, in.SourceRefs); err != nil {
		return Message{}, err
	}
	sequence, err := nextSequence(ctx, tx, scope)
	if err != nil {
		return Message{}, err
	}
	entryID, err := randomID()
	if err != nil {
		return Message{}, err
	}
	sources, err := json.Marshal(in.SourceRefs)
	if err != nil {
		return Message{}, err
	}
	correlations, err := json.Marshal(in.CorrelationRefs)
	if err != nil {
		return Message{}, err
	}
	var occurred any
	if in.OccurredAt != nil {
		occurred = in.OccurredAt.UTC().UnixMilli()
	}
	var key any
	if in.IdempotencyKey != "" {
		key = in.IdempotencyKey
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO publications (entry_id,scope_id,topic_id,actor_id,kind,sequence,committed_at,occurred_at,expires_at,source_refs,correlation_refs,idempotency_key) VALUES (?,?,?,?,'message',?,?,?,?,?,?,?)", entryID, scope, topicID, actorID, sequence, now, occurred, expiry, string(sources), string(correlations), key)
	if err != nil {
		return Message{}, err
	}
	if revision == 1 {
		_, err = tx.ExecContext(ctx, "INSERT INTO message_heads (message_id,scope_id,topic_id,author_actor_id,current_revision,current_entry_id) VALUES (?,?,?,?,?,?)", messageID, scope, topicID, actorID, revision, entryID)
		if err != nil {
			return Message{}, err
		}
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO message_revisions VALUES (?,?,?,?,?,?,?)", messageID, scope, topicID, revision, entryID, in.Text, expiry)
	if err != nil {
		return Message{}, err
	}
	if revision > 1 {
		_, err = tx.ExecContext(ctx, "UPDATE message_heads SET current_revision=?,current_entry_id=?,pinned=0 WHERE message_id=?", revision, entryID, messageID)
		if err != nil {
			return Message{}, err
		}
	}
	if in.IdempotencyKey != "" {
		_, err = tx.ExecContext(ctx, "INSERT INTO publication_idempotency VALUES (?,?,?,?,?,?)", scope, topicID, actorID, in.IdempotencyKey, fp, entryID)
		if err != nil {
			return Message{}, uniqueOrOriginal(err)
		}
	}
	return scanMessage(tx.QueryRowContext(ctx, "SELECT "+messageColumns+" FROM publications p JOIN message_revisions r ON r.entry_id=p.entry_id JOIN message_heads h ON h.message_id=r.message_id WHERE p.entry_id=?", entryID))
}

func (d *DB) PostMessage(ctx context.Context, repoPath, topicID, actorID string, in MessageInput) (Message, error) {
	if err := d.expectedAuthority(in.AuthorityID); err != nil {
		return Message{}, err
	}
	if err := validateMessageInput(in); err != nil {
		return Message{}, err
	}
	in = normalizeMessageInput(in)
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return Message{}, err
	}
	fp, err := fingerprint(in, "", 0, true)
	if err != nil {
		return Message{}, err
	}
	var out Message
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		if err := enrolled(ctx, tx, actorID); err != nil {
			return err
		}
		if replay, found, replayErr := d.replayMessage(ctx, tx, scope, topicID, actorID, in.IdempotencyKey, fp); found || replayErr != nil {
			out = replay
			return replayErr
		}
		topic, err := scanTopic(tx.QueryRowContext(ctx, "SELECT "+topicColumns+" FROM topics WHERE scope_id=? AND topic_id=?", scope, topicID), now)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if topic.State != "active" {
			return ErrArchived
		}
		expiry, err := expiryMillis(in.ExpiresAt, now)
		if err != nil {
			return err
		}
		messageID, err := randomID()
		if err != nil {
			return err
		}
		out, err = d.appendMessage(ctx, tx, now, scope, topicID, actorID, messageID, 1, in, expiry, fp)
		return err
	})
	return out, err
}

func (d *DB) ReviseMessage(ctx context.Context, repoPath, topicID, messageID, actorID string, in MessageRevisionInput) (Message, error) {
	if err := d.expectedAuthority(in.AuthorityID); err != nil {
		return Message{}, err
	}
	if err := validateMessageInput(in.MessageInput); err != nil {
		return Message{}, err
	}
	if in.ExpectedRevision < 1 {
		return Message{}, ErrInvalidInput
	}
	in.MessageInput = normalizeMessageInput(in.MessageInput)
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return Message{}, err
	}
	fp, err := fingerprint(in.MessageInput, messageID, in.ExpectedRevision, in.ChangeExpiry)
	if err != nil {
		return Message{}, err
	}
	var out Message
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		if err := enrolled(ctx, tx, actorID); err != nil {
			return err
		}
		if replay, found, replayErr := d.replayMessage(ctx, tx, scope, topicID, actorID, in.IdempotencyKey, fp); found || replayErr != nil {
			out = replay
			return replayErr
		}
		topic, err := scanTopic(tx.QueryRowContext(ctx, "SELECT "+topicColumns+" FROM topics WHERE scope_id=? AND topic_id=?", scope, topicID), now)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if topic.State != "active" {
			return ErrArchived
		}
		var author, entry string
		var current int64
		err = tx.QueryRowContext(ctx, "SELECT author_actor_id,current_revision,current_entry_id FROM message_heads WHERE scope_id=? AND topic_id=? AND message_id=?", scope, topicID, messageID).Scan(&author, &current, &entry)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if author != actorID {
			return ErrUnauthorized
		}
		if current != in.ExpectedRevision {
			return ErrConflict
		}
		var oldExpiry sql.NullInt64
		if err := tx.QueryRowContext(ctx, "SELECT expires_at FROM publications WHERE entry_id=?", entry).Scan(&oldExpiry); err != nil {
			return err
		}
		if oldExpiry.Valid && oldExpiry.Int64 <= now {
			return ErrExpired
		}
		var expiry any
		if in.ChangeExpiry {
			expiry, err = expiryMillis(in.ExpiresAt, now)
			if err != nil {
				return err
			}
		} else {
			if in.ExpiresAt != nil {
				return ErrInvalidInput
			}
			if oldExpiry.Valid {
				expiry = oldExpiry.Int64
			}
		}
		out, err = d.appendMessage(ctx, tx, now, scope, topicID, actorID, messageID, current+1, in.MessageInput, expiry, fp)
		return err
	})
	return out, err
}

// PinMessage changes discovery order only. It cannot extend a deadline and a
// revision clears the pin; transitions are separately attributed in audit.
func (d *DB) PinMessage(ctx context.Context, repoPath, topicID, messageID, actorID, authorityID string, expectedGeneration, expectedRevision int64, pin bool) (Message, error) {
	if err := d.expectedAuthority(authorityID); err != nil {
		return Message{}, err
	}
	if expectedRevision < 1 {
		return Message{}, ErrInvalidInput
	}
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return Message{}, err
	}
	var out Message
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		topic, err := d.topicForWrite(ctx, tx, scope, topicID, actorID, expectedGeneration, now, false)
		if err != nil {
			return err
		}
		if topic.State != "active" {
			return ErrArchived
		}
		var revision int64
		var entry string
		var oldPin int
		err = tx.QueryRowContext(ctx, "SELECT current_revision,current_entry_id,pinned FROM message_heads WHERE scope_id=? AND topic_id=? AND message_id=?", scope, topicID, messageID).Scan(&revision, &entry, &oldPin)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if revision != expectedRevision {
			return ErrConflict
		}
		var expiry sql.NullInt64
		if err := tx.QueryRowContext(ctx, "SELECT expires_at FROM publications WHERE entry_id=?", entry).Scan(&expiry); err != nil {
			return err
		}
		if expiry.Valid && expiry.Int64 <= now {
			return ErrExpired
		}
		value := 0
		if pin {
			value = 1
		}
		if oldPin != value {
			id, err := randomID()
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "UPDATE message_heads SET pinned=? WHERE message_id=?", value, messageID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO message_pin_audit VALUES (?,?,?,?,?,?)", id, messageID, revision, actorID, value, now); err != nil {
				return err
			}
		}
		out, err = scanMessage(tx.QueryRowContext(ctx, "SELECT "+messageColumns+" FROM publications p JOIN message_revisions r ON r.entry_id=p.entry_id JOIN message_heads h ON h.message_id=r.message_id WHERE p.entry_id=?", entry))
		return err
	})
	return out, err
}

type PinChange struct {
	ID        string    `json:"pin_id"`
	MessageID string    `json:"message_id"`
	Revision  int64     `json:"revision"`
	ActorID   string    `json:"actor_id"`
	Pinned    bool      `json:"pinned"`
	At        time.Time `json:"at"`
}

func (d *DB) PinHistory(ctx context.Context, repoPath, topicID, messageID string, opts MessageList) ([]PinChange, error) {
	opts, err := messageListBounds(opts)
	if err != nil {
		return nil, err
	}
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	var exists string
	err = d.sql.QueryRowContext(ctx, "SELECT message_id FROM message_heads WHERE scope_id=? AND topic_id=? AND message_id=?", scope.ID, topicID, messageID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := d.sql.QueryContext(ctx, "SELECT pin_id,message_id,revision,actor_id,pinned,at FROM message_pin_audit WHERE message_id=? ORDER BY at,rowid LIMIT ? OFFSET ?", messageID, opts.Limit, opts.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PinChange, 0)
	for rows.Next() {
		var change PinChange
		var pinned int
		var at int64
		if err := rows.Scan(&change.ID, &change.MessageID, &change.Revision, &change.ActorID, &pinned, &at); err != nil {
			return nil, err
		}
		change.Pinned = pinned == 1
		change.At = fromMillis(at)
		out = append(out, change)
	}
	return out, rows.Err()
}

func messageListBounds(opts MessageList) (MessageList, error) {
	if opts.Limit == 0 {
		opts.Limit = 50
	}
	if opts.Limit < 1 || opts.Limit > 100 || opts.Offset < 0 {
		return opts, ErrInvalidInput
	}
	return opts, nil
}

// ListMessages defaults to pinned-first, newest-first discovery. Ranked mode
// orders by pin, then score (upvotes minus downvotes), then sequence and ID.
func (d *DB) ListMessages(ctx context.Context, repoPath, topicID string, opts MessageList) ([]Message, error) {
	opts, err := messageListBounds(opts)
	if err != nil {
		return nil, err
	}
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	now := d.now().UTC().UnixMilli()
	topic, err := d.ShowTopic(ctx, repoPath, topicID)
	if err != nil {
		return nil, err
	}
	if topic.State != "active" {
		return []Message{}, nil
	}
	order := "h.pinned DESC,p.sequence DESC,p.entry_id DESC"
	if opts.Ranked {
		order = "h.pinned DESC," + messageScore + " DESC,p.sequence DESC,p.entry_id DESC"
	}
	rows, err := d.sql.QueryContext(ctx, "SELECT "+messageColumns+" FROM message_heads h JOIN message_revisions r ON r.entry_id=h.current_entry_id JOIN publications p ON p.entry_id=r.entry_id WHERE h.scope_id=? AND h.topic_id=? AND (p.expires_at IS NULL OR p.expires_at>?) ORDER BY "+order+" LIMIT ? OFFSET ?", scope.ID, topicID, now, opts.Limit, opts.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Message, 0)
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ReadMessage returns the current revision. Historical mode retains access
// across topic archive and independent message expiry.
func (d *DB) ReadMessage(ctx context.Context, repoPath, topicID, messageID string, historical bool) (Message, error) {
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return Message{}, err
	}
	m, err := scanMessage(d.sql.QueryRowContext(ctx, "SELECT "+messageColumns+" FROM message_heads h JOIN message_revisions r ON r.entry_id=h.current_entry_id JOIN publications p ON p.entry_id=r.entry_id WHERE h.scope_id=? AND h.topic_id=? AND h.message_id=?", scope.ID, topicID, messageID))
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, err
	}
	if !historical {
		topic, err := d.ShowTopic(ctx, repoPath, topicID)
		if err != nil {
			return Message{}, err
		}
		if topic.State != "active" {
			return Message{}, ErrArchived
		}
		if m.ExpiresAt != nil && !d.now().UTC().Before(*m.ExpiresAt) {
			return Message{}, ErrExpired
		}
	}
	return m, nil
}

// ReadPublication resolves an exact retained entry, including expired and
// archived revisions when historical is true.
func (d *DB) ReadPublication(ctx context.Context, repoPath, entryID string, historical bool) (Message, error) {
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return Message{}, err
	}
	m, err := scanMessage(d.sql.QueryRowContext(ctx, "SELECT "+messageColumns+" FROM publications p JOIN message_revisions r ON r.entry_id=p.entry_id JOIN message_heads h ON h.message_id=r.message_id WHERE p.scope_id=? AND p.entry_id=?", scope.ID, entryID))
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, err
	}
	if !historical {
		topic, err := d.ShowTopic(ctx, repoPath, m.TopicID)
		if err != nil {
			return Message{}, err
		}
		if topic.State != "active" {
			return Message{}, ErrArchived
		}
		if m.ExpiresAt != nil && !d.now().UTC().Before(*m.ExpiresAt) {
			return Message{}, ErrExpired
		}
	}
	return m, nil
}

func (d *DB) MessageHistory(ctx context.Context, repoPath, topicID, messageID string, opts MessageList) ([]Message, error) {
	opts, err := messageListBounds(opts)
	if err != nil {
		return nil, err
	}
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	var exists string
	err = d.sql.QueryRowContext(ctx, "SELECT message_id FROM message_heads WHERE scope_id=? AND topic_id=? AND message_id=?", scope.ID, topicID, messageID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := d.sql.QueryContext(ctx, "SELECT "+messageColumns+" FROM message_revisions r JOIN publications p ON p.entry_id=r.entry_id JOIN message_heads h ON h.message_id=r.message_id WHERE r.scope_id=? AND r.topic_id=? AND r.message_id=? ORDER BY r.revision LIMIT ? OFFSET ?", scope.ID, topicID, messageID, opts.Limit, opts.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Message, 0)
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type PublicationGap struct {
	From   int64  `json:"from"`
	To     int64  `json:"to"`
	Reason string `json:"reason"`
}

type PublicationPage struct {
	ScopeID          string           `json:"repo_scope_id"`
	AsOf             time.Time        `json:"as_of"`
	HighWater        int64            `json:"high_water_sequence"`
	LowWater         int64            `json:"low_water_sequence"`
	RetentionEpoch   int64            `json:"retention_epoch"`
	NextScanPosition int64            `json:"next_scan_position"`
	Entries          []Publication    `json:"entries"`
	Gaps             []PublicationGap `json:"gaps"`
}

func scanPublication(row rowScanner, extra ...any) (Publication, error) {
	var p Publication
	var committed int64
	var occurred, expiry sql.NullInt64
	var sources, correlations string
	var key sql.NullString
	targets := []any{&p.EntryID, &p.ScopeID, &p.TopicID, &p.ActorID, &p.Kind, &p.Sequence, &committed, &occurred, &expiry, &sources, &correlations, &key}
	if err := row.Scan(append(targets, extra...)...); err != nil {
		return Publication{}, err
	}
	p.CommittedAt = fromMillis(committed)
	if occurred.Valid {
		v := fromMillis(occurred.Int64)
		p.OccurredAt = &v
	}
	if expiry.Valid {
		v := fromMillis(expiry.Int64)
		p.ExpiresAt = &v
	}
	if err := json.Unmarshal([]byte(sources), &p.SourceRefs); err != nil {
		return Publication{}, ErrUnsupportedFormat
	}
	if err := json.Unmarshal([]byte(correlations), &p.CorrelationRefs); err != nil {
		return Publication{}, ErrUnsupportedFormat
	}
	p.IdempotencyKey = key.String
	return p, nil
}

// ScanPublications scans retained scope sequence positions, including skipped
// positions in the returned page. It does not acknowledge or delete anything.
func (d *DB) ScanPublications(ctx context.Context, repoPath string, after int64, limit int, historical bool) (PublicationPage, error) {
	if after < 0 || limit < 1 || limit > 100 {
		return PublicationPage{}, ErrInvalidInput
	}
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return PublicationPage{}, err
	}
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PublicationPage{}, err
	}
	defer tx.Rollback()
	page := PublicationPage{ScopeID: scope.ID, AsOf: d.now().UTC(), NextScanPosition: after, Entries: make([]Publication, 0), Gaps: make([]PublicationGap, 0)}
	var next int64
	err = tx.QueryRowContext(ctx, "SELECT c.next_sequence,s.low_water_sequence,(SELECT retention_epoch FROM hub_meta WHERE singleton=1) FROM scope_counters c JOIN repo_scopes s ON s.scope_id=c.scope_id WHERE c.scope_id=?", scope.ID).Scan(&next, &page.LowWater, &page.RetentionEpoch)
	if errors.Is(err, sql.ErrNoRows) {
		next = 1
		err = tx.QueryRowContext(ctx, "SELECT low_water_sequence FROM repo_scopes WHERE scope_id=?", scope.ID).Scan(&page.LowWater)
	}
	if err != nil {
		return PublicationPage{}, err
	}
	page.HighWater = next - 1
	if after < page.LowWater {
		page.Gaps = append(page.Gaps, PublicationGap{From: after + 1, To: page.LowWater, Reason: "pruned"})
		page.NextScanPosition = page.LowWater
	}
	query := "SELECT p.entry_id,p.scope_id,p.topic_id,p.actor_id,p.kind,p.sequence,p.committed_at,p.occurred_at,p.expires_at,p.source_refs,p.correlation_refs,p.idempotency_key,t.archived_at,t.expires_at FROM publications p JOIN topics t ON t.topic_id=p.topic_id WHERE p.scope_id=? AND p.sequence>? ORDER BY p.sequence LIMIT ?"
	rows, err := tx.QueryContext(ctx, query, scope.ID, page.NextScanPosition, limit)
	if err != nil {
		return PublicationPage{}, err
	}
	defer rows.Close()
	asOf := page.AsOf.UnixMilli()
	for rows.Next() {
		var archive, topicExpiry sql.NullInt64
		entry, err := scanPublication(rows, &archive, &topicExpiry)
		if err != nil {
			return PublicationPage{}, err
		}
		if entry.Sequence > page.NextScanPosition+1 {
			if err := addMissingGaps(ctx, tx, &page.Gaps, scope.ID, page.NextScanPosition+1, entry.Sequence-1); err != nil {
				return PublicationPage{}, err
			}
		}
		page.NextScanPosition = entry.Sequence
		if !historical {
			reason := ""
			if archive.Valid || (topicExpiry.Valid && topicExpiry.Int64 <= asOf) {
				reason = "archived"
			} else if entry.ExpiresAt != nil && entry.ExpiresAt.UnixMilli() <= asOf {
				reason = "expired"
			}
			if reason != "" {
				page.Gaps = append(page.Gaps, PublicationGap{From: entry.Sequence, To: entry.Sequence, Reason: reason})
				continue
			}
		}
		page.Entries = append(page.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return PublicationPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return PublicationPage{}, err
	}
	return page, nil
}
