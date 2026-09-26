package hubstore

import (
	"context"
	"database/sql"
	"errors"
)

// VoteMessage replaces an enrolled actor's vote on the exact current revision.
// value is -1, 0 (clear), or 1. Repeating a request leaves the same state.
// Actor IDs are locally enrolled attribution, not independently verified identity.
func (d *DB) VoteMessage(ctx context.Context, repoPath, topicID, messageID, actorID, authorityID string, revision int64, value int) (Message, error) {
	if err := d.expectedAuthority(authorityID); err != nil {
		return Message{}, err
	}
	if revision < 1 || value < -1 || value > 1 {
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
		if err := enrolled(ctx, tx, actorID); err != nil {
			return err
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
		var current int64
		var entry string
		err = tx.QueryRowContext(ctx, "SELECT current_revision,current_entry_id FROM message_heads WHERE scope_id=? AND topic_id=? AND message_id=?", scope, topicID, messageID).Scan(&current, &entry)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if current != revision {
			return ErrConflict
		}
		var expiry sql.NullInt64
		if err := tx.QueryRowContext(ctx, "SELECT expires_at FROM publications WHERE entry_id=?", entry).Scan(&expiry); err != nil {
			return err
		}
		if expiry.Valid && expiry.Int64 <= now {
			return ErrExpired
		}
		if value == 0 {
			_, err = tx.ExecContext(ctx, "DELETE FROM message_votes WHERE message_id=? AND revision=? AND actor_id=?", messageID, revision, actorID)
		} else {
			_, err = tx.ExecContext(ctx, "INSERT INTO message_votes (message_id,revision,actor_id,value) VALUES (?,?,?,?) ON CONFLICT(message_id,revision,actor_id) DO UPDATE SET value=excluded.value WHERE value<>excluded.value", messageID, revision, actorID, value)
		}
		if err != nil {
			return err
		}
		out, err = scanMessage(tx.QueryRowContext(ctx, "SELECT "+messageColumns+" FROM publications p JOIN message_revisions r ON r.entry_id=p.entry_id JOIN message_heads h ON h.message_id=r.message_id WHERE p.entry_id=?", entry))
		return err
	})
	return out, err
}
