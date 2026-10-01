package hubstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Lease is cooperative admission, not a fence around an operating-system job.
// Token is returned only by AcquireLease and must be kept by the holder.
type Lease struct {
	TopicID    string     `json:"topic_id"`
	State      string     `json:"state"`
	Generation int64      `json:"generation"`
	ActorID    string     `json:"actor_id,omitempty"`
	AcquiredAt *time.Time `json:"acquired_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Token      string     `json:"token,omitempty"`
}

const maxLeaseDuration = 24 * time.Hour

func leaseDuration(d time.Duration) (int64, error) {
	if d < time.Second || d > maxLeaseDuration {
		return 0, ErrInvalidInput
	}
	return d.Milliseconds(), nil
}

func leaseTokenHash(token string) (string, error) {
	if len(token) != 32 || !isHex(token) {
		return "", ErrInvalidInput
	}
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:]), nil
}

func leaseTopic(ctx context.Context, tx *sql.Tx, scope, topicID string, now int64) (Topic, error) {
	topic, err := scanTopic(tx.QueryRowContext(ctx, "SELECT "+topicColumns+" FROM topics WHERE scope_id=? AND topic_id=?", scope, topicID), now)
	if errors.Is(err, sql.ErrNoRows) {
		return Topic{}, ErrNotFound
	}
	return topic, err
}

func leaseRow(ctx context.Context, tx *sql.Tx, topicID string, now int64) (Lease, error) {
	lease := Lease{TopicID: topicID, State: "available"}
	var actor, hash sql.NullString
	var acquired, expiry sql.NullInt64
	err := tx.QueryRowContext(ctx, "SELECT generation, actor_id, token_hash, acquired_at, expires_at FROM topic_leases WHERE topic_id=?", topicID).
		Scan(&lease.Generation, &actor, &hash, &acquired, &expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return lease, nil
	}
	if err != nil {
		return Lease{}, err
	}
	if actor.Valid && expiry.Int64 > now {
		lease.State = "held"
		lease.ActorID = actor.String
		acquiredAt := fromMillis(acquired.Int64)
		expiresAt := fromMillis(expiry.Int64)
		lease.AcquiredAt = &acquiredAt
		lease.ExpiresAt = &expiresAt
	}
	return lease, nil
}

// AcquireLease atomically admits one participating actor to an active topic.
// An expired lease is reclaimable: the previous actor must have stopped work.
func (d *DB) AcquireLease(ctx context.Context, repoPath, topicID, actorID string, duration time.Duration) (Lease, error) {
	millis, err := leaseDuration(duration)
	if err != nil {
		return Lease{}, err
	}
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return Lease{}, err
	}
	var result Lease
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		topic, err := leaseTopic(ctx, tx, scope, topicID, now)
		if err != nil {
			return err
		}
		if topic.State != "active" {
			return ErrArchived
		}
		if err := enrolled(ctx, tx, actorID); err != nil {
			return err
		}
		current, err := leaseRow(ctx, tx, topicID, now)
		if err != nil {
			return err
		}
		if current.State == "held" {
			return fmt.Errorf("%w: held by %s until %s", ErrConflict, current.ActorID, current.ExpiresAt.Format(time.RFC3339))
		}
		token, err := randomID()
		if err != nil {
			return err
		}
		hash, err := leaseTokenHash(token)
		if err != nil {
			return err
		}
		expires := now + millis
		if current.Generation == 0 {
			_, err = tx.ExecContext(ctx, "INSERT INTO topic_leases (topic_id,generation,actor_id,token_hash,acquired_at,expires_at) VALUES (?,1,?,?,?,?)", topicID, actorID, hash, now, expires)
		} else {
			_, err = tx.ExecContext(ctx, "UPDATE topic_leases SET generation=generation+1,actor_id=?,token_hash=?,acquired_at=?,expires_at=? WHERE topic_id=?", actorID, hash, now, expires, topicID)
		}
		if err != nil {
			return err
		}
		acquiredAt := fromMillis(now)
		expiresAt := fromMillis(expires)
		result = Lease{TopicID: topicID, State: "held", Generation: current.Generation + 1, ActorID: actorID, AcquiredAt: &acquiredAt, ExpiresAt: &expiresAt, Token: token}
		return nil
	})
	return result, err
}

// ShowLease never returns a bearer token.
func (d *DB) ShowLease(ctx context.Context, repoPath, topicID string) (Lease, error) {
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return Lease{}, err
	}
	now := d.now().UTC().UnixMilli()
	var exists string
	err = d.sql.QueryRowContext(ctx, "SELECT topic_id FROM topics WHERE scope_id=? AND topic_id=?", scope.ID, topicID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return Lease{}, ErrNotFound
	}
	if err != nil {
		return Lease{}, err
	}
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Lease{}, err
	}
	defer tx.Rollback()
	lease, err := leaseRow(ctx, tx, topicID, now)
	if err != nil {
		return Lease{}, err
	}
	return lease, tx.Commit()
}

// RenewLease extends a live lease only for its original bearer and actor.
func (d *DB) RenewLease(ctx context.Context, repoPath, topicID, actorID, token string, duration time.Duration) (Lease, error) {
	millis, err := leaseDuration(duration)
	if err != nil {
		return Lease{}, err
	}
	hash, err := leaseTokenHash(token)
	if err != nil {
		return Lease{}, err
	}
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return Lease{}, err
	}
	var result Lease
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		topic, err := leaseTopic(ctx, tx, scope, topicID, now)
		if err != nil {
			return err
		}
		if topic.State != "active" {
			return ErrArchived
		}
		current, err := leaseRow(ctx, tx, topicID, now)
		if err != nil {
			return err
		}
		if current.State != "held" {
			return ErrExpired
		}
		if current.ActorID != actorID {
			return ErrUnauthorized
		}
		updated, err := tx.ExecContext(ctx, "UPDATE topic_leases SET expires_at=? WHERE topic_id=? AND actor_id=? AND token_hash=? AND expires_at>?", now+millis, topicID, actorID, hash, now)
		if err != nil {
			return err
		}
		count, err := updated.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrConflict
		}
		result = current
		expiresAt := fromMillis(now + millis)
		result.ExpiresAt = &expiresAt
		return nil
	})
	return result, err
}

// ReleaseLease clears only the current bearer. A stale token cannot release
// a successor, including after expiry and reacquisition by the same actor.
func (d *DB) ReleaseLease(ctx context.Context, repoPath, topicID, actorID, token string) error {
	hash, err := leaseTokenHash(token)
	if err != nil {
		return err
	}
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return err
	}
	return d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		if _, err := leaseTopic(ctx, tx, scope, topicID, now); err != nil {
			return err
		}
		updated, err := tx.ExecContext(ctx, "UPDATE topic_leases SET actor_id=NULL,token_hash=NULL,acquired_at=NULL,expires_at=NULL WHERE topic_id=? AND actor_id=? AND token_hash=?", topicID, actorID, hash)
		if err != nil {
			return err
		}
		count, err := updated.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrConflict
		}
		return nil
	})
}
