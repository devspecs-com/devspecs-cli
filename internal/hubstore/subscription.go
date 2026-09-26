package hubstore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"time"
)

// A consumer is a cooperative, stable identity shared by its workers. Separate
// consumers have independent cursors; sharing an ID does not fence workers.
type Consumer struct {
	ID         string    `json:"consumer_id"`
	EnrolledAt time.Time `json:"enrolled_at"`
}

type EventTypeFilter struct {
	TypeKey string `json:"type_key"`
	Version int64  `json:"version"`
}

type Subscription struct {
	ID                   string            `json:"subscription_id"`
	ConsumerID           string            `json:"consumer_id"`
	ScopeID              string            `json:"repo_scope_id"`
	TopicIDs             []string          `json:"topic_ids"`
	Kinds                []string          `json:"kinds"`
	EventTypes           []EventTypeFilter `json:"event_types"`
	CreatedAt            time.Time         `json:"created_at"`
	AcknowledgedSequence int64             `json:"acknowledged_sequence"`
	RemovedAt            *time.Time        `json:"removed_at,omitempty"`
}

type SubscribeInput struct {
	AuthorityID   string
	ConsumerID    string
	TopicIDs      []string
	Kinds         []string          // empty means both message and event
	EventTypes    []EventTypeFilter // empty means every registered event type/version
	FromBeginning bool              // otherwise start at the current committed high-water
}

type PullEntry struct {
	Message *Message `json:"message,omitempty"`
	Event   *Event   `json:"event,omitempty"`
}

type PullPage struct {
	SubscriptionID    string           `json:"subscription_id"`
	ConsumerID        string           `json:"consumer_id"`
	ScopeID           string           `json:"repo_scope_id"`
	AsOf              time.Time        `json:"as_of"`
	HighWater         int64            `json:"high_water_sequence"`
	LowWater          int64            `json:"low_water_sequence"`
	RetentionEpoch    int64            `json:"retention_epoch"`
	PriorAcknowledged int64            `json:"prior_acknowledged_sequence"`
	NextScanPosition  int64            `json:"next_scan_position"`
	Entries           []PullEntry      `json:"entries"`
	Gaps              []PublicationGap `json:"gaps"`
	AckToken          string           `json:"ack_token"`
}

type ackClaim struct {
	AuthorityID    string `json:"a"`
	SubscriptionID string `json:"s"`
	ConsumerID     string `json:"c"`
	Prior          int64  `json:"p"`
	Next           int64  `json:"n"`
	HighWater      int64  `json:"h"`
	RetentionEpoch int64  `json:"e"`
}

func validConsumerID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func (d *DB) EnrollConsumer(ctx context.Context, authorityID, consumerID string) (Consumer, error) {
	if err := d.expectedAuthority(authorityID); err != nil {
		return Consumer{}, err
	}
	if consumerID == "" {
		var err error
		consumerID, err = randomID()
		if err != nil {
			return Consumer{}, err
		}
	}
	if !validConsumerID(consumerID) {
		return Consumer{}, ErrInvalidInput
	}
	var out Consumer
	err := d.write(ctx, func(tx *sql.Tx, now int64) error {
		if _, err := tx.ExecContext(ctx, "INSERT INTO consumers VALUES (?,?) ON CONFLICT(consumer_id) DO NOTHING", consumerID, now); err != nil {
			return err
		}
		var enrolled int64
		if err := tx.QueryRowContext(ctx, "SELECT consumer_id,enrolled_at FROM consumers WHERE consumer_id=?", consumerID).Scan(&out.ID, &enrolled); err != nil {
			return err
		}
		out.EnrolledAt = fromMillis(enrolled)
		return nil
	})
	return out, err
}

func canonicalFilters(in SubscribeInput) (SubscribeInput, error) {
	if in.ConsumerID == "" || len(in.TopicIDs) == 0 || len(in.TopicIDs) > 32 || len(in.EventTypes) > 32 || len(in.Kinds) > 2 {
		return in, ErrInvalidInput
	}
	in.TopicIDs = append([]string(nil), in.TopicIDs...)
	sort.Strings(in.TopicIDs)
	for i, id := range in.TopicIDs {
		if len(id) != 32 || !isHex(id) || (i > 0 && id == in.TopicIDs[i-1]) {
			return in, ErrInvalidInput
		}
	}
	if len(in.Kinds) == 0 {
		in.Kinds = []string{"event", "message"}
	} else {
		in.Kinds = append([]string(nil), in.Kinds...)
	}
	sort.Strings(in.Kinds)
	for i, kind := range in.Kinds {
		if (kind != "event" && kind != "message") || (i > 0 && kind == in.Kinds[i-1]) {
			return in, ErrInvalidInput
		}
	}
	in.EventTypes = append([]EventTypeFilter(nil), in.EventTypes...)
	sort.Slice(in.EventTypes, func(i, j int) bool {
		if in.EventTypes[i].TypeKey != in.EventTypes[j].TypeKey {
			return in.EventTypes[i].TypeKey < in.EventTypes[j].TypeKey
		}
		return in.EventTypes[i].Version < in.EventTypes[j].Version
	})
	for i, f := range in.EventTypes {
		if !validTypeKey(f.TypeKey) || f.Version < 1 || (i > 0 && f == in.EventTypes[i-1]) {
			return in, ErrInvalidInput
		}
	}
	if len(in.EventTypes) > 0 && !containsKind(in.Kinds, "event") {
		return in, ErrInvalidInput
	}
	return in, nil
}

func containsKind(kinds []string, kind string) bool {
	for _, k := range kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Subscribe creates an immutable definition. Resubscribe (including a filter
// change or replay) makes a new ID; removing one never rewinds another cursor.
func (d *DB) Subscribe(ctx context.Context, repoPath string, in SubscribeInput) (Subscription, error) {
	if err := d.expectedAuthority(in.AuthorityID); err != nil {
		return Subscription{}, err
	}
	var err error
	in, err = canonicalFilters(in)
	if err != nil {
		return Subscription{}, err
	}
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return Subscription{}, err
	}
	var out Subscription
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		var enrolled string
		if err := tx.QueryRowContext(ctx, "SELECT consumer_id FROM consumers WHERE consumer_id=?", in.ConsumerID).Scan(&enrolled); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		for _, id := range in.TopicIDs {
			topic, err := scanTopic(tx.QueryRowContext(ctx, "SELECT "+topicColumns+" FROM topics WHERE scope_id=? AND topic_id=?", scope, id), now)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
			if topic.State != "active" {
				return ErrArchived
			}
			for _, filter := range in.EventTypes {
				var version int64
				err := tx.QueryRowContext(ctx, "SELECT version FROM event_schemas WHERE topic_id=? AND type_key=? AND version=?", id, filter.TypeKey, filter.Version).Scan(&version)
				if errors.Is(err, sql.ErrNoRows) {
					return ErrVersionUnknown
				}
				if err != nil {
					return err
				}
			}
		}
		var next int64
		err = tx.QueryRowContext(ctx, "SELECT next_sequence FROM scope_counters WHERE scope_id=?", scope).Scan(&next)
		if errors.Is(err, sql.ErrNoRows) {
			next = 1
		} else if err != nil {
			return err
		}
		ack := next - 1
		if in.FromBeginning {
			ack = 0
		}
		id, err := randomID()
		if err != nil {
			return err
		}
		kinds, _ := json.Marshal(in.Kinds)
		types, _ := json.Marshal(in.EventTypes)
		if _, err := tx.ExecContext(ctx, "INSERT INTO subscriptions (subscription_id,consumer_id,scope_id,kinds,event_types,created_at,acknowledged_sequence) VALUES (?,?,?,?,?,?,?)", id, in.ConsumerID, scope, string(kinds), string(types), now, ack); err != nil {
			return err
		}
		for _, topicID := range in.TopicIDs {
			if _, err := tx.ExecContext(ctx, "INSERT INTO subscription_topics VALUES (?,?,?)", id, scope, topicID); err != nil {
				return err
			}
		}
		out = Subscription{ID: id, ConsumerID: in.ConsumerID, ScopeID: scope, TopicIDs: in.TopicIDs, Kinds: in.Kinds, EventTypes: in.EventTypes, CreatedAt: fromMillis(now), AcknowledgedSequence: ack}
		return nil
	})
	return out, err
}

func readSubscription(ctx context.Context, tx *sql.Tx, scope, consumer, id string) (Subscription, error) {
	var s Subscription
	var kinds, types string
	var created int64
	var removed sql.NullInt64
	err := tx.QueryRowContext(ctx, "SELECT subscription_id,consumer_id,scope_id,kinds,event_types,created_at,acknowledged_sequence,removed_at FROM subscriptions WHERE subscription_id=? AND consumer_id=? AND scope_id=?", id, consumer, scope).Scan(&s.ID, &s.ConsumerID, &s.ScopeID, &kinds, &types, &created, &s.AcknowledgedSequence, &removed)
	if errors.Is(err, sql.ErrNoRows) {
		return s, ErrNotFound
	}
	if err != nil {
		return s, err
	}
	if json.Unmarshal([]byte(kinds), &s.Kinds) != nil || json.Unmarshal([]byte(types), &s.EventTypes) != nil {
		return s, ErrUnsupportedFormat
	}
	s.CreatedAt = fromMillis(created)
	if removed.Valid {
		t := fromMillis(removed.Int64)
		s.RemovedAt = &t
	}
	rows, err := tx.QueryContext(ctx, "SELECT topic_id FROM subscription_topics WHERE subscription_id=? ORDER BY topic_id", id)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	s.TopicIDs = []string{}
	for rows.Next() {
		var topic string
		if err := rows.Scan(&topic); err != nil {
			return s, err
		}
		s.TopicIDs = append(s.TopicIDs, topic)
	}
	return s, rows.Err()
}

func (d *DB) ListSubscriptions(ctx context.Context, repoPath, consumerID string, includeRemoved bool) ([]Subscription, error) {
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, classifySQLite(err)
	}
	defer tx.Rollback()
	query := "SELECT subscription_id FROM subscriptions WHERE scope_id=? AND consumer_id=?"
	if !includeRemoved {
		query += " AND removed_at IS NULL"
	}
	query += " ORDER BY created_at,subscription_id"
	rows, err := tx.QueryContext(ctx, query, scope.ID, consumerID)
	if err != nil {
		return nil, classifySQLite(err)
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	out := make([]Subscription, 0, len(ids))
	for _, id := range ids {
		s, err := readSubscription(ctx, tx, scope.ID, consumerID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := tx.Commit(); err != nil {
		return nil, classifySQLite(err)
	}
	return out, nil
}

func (d *DB) RemoveSubscription(ctx context.Context, repoPath, consumerID, subscriptionID, authorityID string) (Subscription, error) {
	if err := d.expectedAuthority(authorityID); err != nil {
		return Subscription{}, err
	}
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return Subscription{}, err
	}
	var out Subscription
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		out, err = readSubscription(ctx, tx, scope, consumerID, subscriptionID)
		if err != nil {
			return err
		}
		if out.RemovedAt != nil {
			return nil
		}
		if _, err := tx.ExecContext(ctx, "UPDATE subscriptions SET removed_at=? WHERE subscription_id=?", now, subscriptionID); err != nil {
			return err
		}
		t := fromMillis(now)
		out.RemovedAt = &t
		return nil
	})
	return out, err
}

func addGap(gaps *[]PublicationGap, from, to int64, reason string) {
	if from > to {
		return
	}
	g := *gaps
	if len(g) > 0 && g[len(g)-1].Reason == reason && g[len(g)-1].To+1 == from {
		g[len(g)-1].To = to
		*gaps = g
		return
	}
	*gaps = append(g, PublicationGap{From: from, To: to, Reason: reason})
}

func signClaim(secret []byte, claim ackClaim) string {
	b, _ := json.Marshal(claim)
	mac := hmac.New(sha256.New, secret)
	mac.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func parseClaim(secret []byte, token string) (ackClaim, error) {
	var claim ackClaim
	if len(token) > 1024 {
		return claim, ErrInvalidInput
	}
	sep := -1
	for i := range token {
		if token[i] == '.' {
			if sep != -1 {
				return claim, ErrInvalidInput
			}
			sep = i
		}
	}
	if sep < 1 {
		return claim, ErrInvalidInput
	}
	b, err := base64.RawURLEncoding.DecodeString(token[:sep])
	if err != nil {
		return claim, ErrInvalidInput
	}
	sig, err := base64.RawURLEncoding.DecodeString(token[sep+1:])
	if err != nil {
		return claim, ErrInvalidInput
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(b)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return claim, ErrCursorConflict
	}
	if json.Unmarshal(b, &claim) != nil {
		return claim, ErrInvalidInput
	}
	return claim, nil
}

// Pull scans at most limit sequence positions, not limit matching results.
// The page and its token are per subscription and never consume the cursor.
func (d *DB) Pull(ctx context.Context, repoPath, consumerID, subscriptionID string, limit int) (PullPage, error) {
	if limit < 1 || limit > 100 {
		return PullPage{}, ErrInvalidInput
	}
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return PullPage{}, err
	}
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PullPage{}, classifySQLite(err)
	}
	defer tx.Rollback()
	page := PullPage{SubscriptionID: subscriptionID, ConsumerID: consumerID, ScopeID: scope.ID, Entries: []PullEntry{}, Gaps: []PublicationGap{}}
	var next int64
	var secret []byte
	err = tx.QueryRowContext(ctx, "SELECT COALESCE((SELECT next_sequence FROM scope_counters WHERE scope_id=?),1),low_water_sequence,(SELECT retention_epoch FROM hub_meta WHERE singleton=1),(SELECT secret FROM pull_token_secret WHERE singleton=1) FROM repo_scopes WHERE scope_id=?", scope.ID, scope.ID).Scan(&next, &page.LowWater, &page.RetentionEpoch, &secret)
	if err != nil {
		return PullPage{}, classifySQLite(err)
	}
	page.AsOf = d.now().UTC()
	page.HighWater = next - 1
	s, err := readSubscription(ctx, tx, scope.ID, consumerID, subscriptionID)
	if err != nil {
		return PullPage{}, err
	}
	if s.RemovedAt != nil {
		return PullPage{}, ErrNotFound
	}
	page.PriorAcknowledged = s.AcknowledgedSequence
	page.NextScanPosition = s.AcknowledgedSequence
	if page.NextScanPosition < page.LowWater {
		addGap(&page.Gaps, page.NextScanPosition+1, page.LowWater, "pruned")
		page.NextScanPosition = page.LowWater
	}
	end := page.NextScanPosition
	if page.HighWater > end {
		end = page.HighWater
		if end-page.NextScanPosition > int64(limit) {
			end = page.NextScanPosition + int64(limit)
		}
	}
	if end > page.NextScanPosition {
		rows, err := tx.QueryContext(ctx, "SELECT p.entry_id,p.scope_id,p.topic_id,p.actor_id,p.kind,p.sequence,p.committed_at,p.occurred_at,p.expires_at,p.source_refs,p.correlation_refs,p.idempotency_key,t.archived_at,t.expires_at,e.type_key,e.version FROM publications p JOIN topics t ON t.topic_id=p.topic_id LEFT JOIN event_entries e ON e.entry_id=p.entry_id WHERE p.scope_id=? AND p.sequence>? AND p.sequence<=? ORDER BY p.sequence", scope.ID, page.NextScanPosition, end)
		if err != nil {
			return PullPage{}, classifySQLite(err)
		}
		ids := []string{}
		kinds := []string{}
		position := page.NextScanPosition
		for rows.Next() {
			var archive, topicExpiry, version sql.NullInt64
			var typeKey sql.NullString
			p, err := scanPublication(rows, &archive, &topicExpiry, &typeKey, &version)
			if err != nil {
				rows.Close()
				return PullPage{}, err
			}
			if p.Sequence > position+1 {
				addGap(&page.Gaps, position+1, p.Sequence-1, "unavailable")
			}
			position = p.Sequence
			reason := ""
			if !containsTopic(s.TopicIDs, p.TopicID) || !containsKind(s.Kinds, p.Kind) {
				reason = "filtered"
			}
			if reason == "" && p.Kind == "event" && !matchesEvent(s.EventTypes, typeKey.String, version.Int64) {
				reason = "filtered"
			}
			if reason == "" && (archive.Valid && archive.Int64 <= page.AsOf.UnixMilli() || topicExpiry.Valid && topicExpiry.Int64 <= page.AsOf.UnixMilli()) {
				reason = "archived"
			}
			if reason == "" && p.ExpiresAt != nil && p.ExpiresAt.UnixMilli() <= page.AsOf.UnixMilli() {
				reason = "expired"
			}
			if reason != "" {
				addGap(&page.Gaps, p.Sequence, p.Sequence, reason)
			} else {
				ids = append(ids, p.EntryID)
				kinds = append(kinds, p.Kind)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return PullPage{}, err
		}
		rows.Close()
		if position < end {
			addGap(&page.Gaps, position+1, end, "unavailable")
		}
		for i, id := range ids {
			if kinds[i] == "message" {
				m, err := scanMessage(tx.QueryRowContext(ctx, "SELECT "+messageColumns+" FROM publications p JOIN message_revisions r ON r.entry_id=p.entry_id JOIN message_heads h ON h.message_id=r.message_id WHERE p.entry_id=?", id))
				if err != nil {
					return PullPage{}, err
				}
				page.Entries = append(page.Entries, PullEntry{Message: &m})
			} else {
				e, err := scanEvent(tx.QueryRowContext(ctx, "SELECT "+eventColumns+" FROM publications p JOIN event_entries e ON e.entry_id=p.entry_id WHERE p.entry_id=?", id))
				if err != nil {
					return PullPage{}, err
				}
				page.Entries = append(page.Entries, PullEntry{Event: &e})
			}
		}
		page.NextScanPosition = end
	}
	claim := ackClaim{d.authorityID, subscriptionID, consumerID, page.PriorAcknowledged, page.NextScanPosition, page.HighWater, page.RetentionEpoch}
	page.AckToken = signClaim(secret, claim)
	if err := tx.Commit(); err != nil {
		return PullPage{}, classifySQLite(err)
	}
	return page, nil
}

func containsTopic(ids []string, id string) bool {
	i := sort.SearchStrings(ids, id)
	return i < len(ids) && ids[i] == id
}
func matchesEvent(filters []EventTypeFilter, key string, version int64) bool {
	if len(filters) == 0 {
		return true
	}
	for _, f := range filters {
		if f.TypeKey == key && f.Version == version {
			return true
		}
	}
	return false
}

// Ack is a CAS over an issued page proposal. Repeating a successful ack with
// the same token returns the current cursor; another advance is a conflict.
func (d *DB) Ack(ctx context.Context, repoPath, consumerID, subscriptionID, authorityID string, expectedPrior, nextPosition int64, token string) (int64, error) {
	if err := d.expectedAuthority(authorityID); err != nil {
		return 0, err
	}
	if expectedPrior < 0 || nextPosition < expectedPrior || token == "" {
		return 0, ErrInvalidInput
	}
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return 0, err
	}
	var current int64
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		var secret []byte
		var epoch int64
		if err := tx.QueryRowContext(ctx, "SELECT (SELECT secret FROM pull_token_secret WHERE singleton=1),retention_epoch FROM hub_meta WHERE singleton=1").Scan(&secret, &epoch); err != nil {
			return err
		}
		claim, err := parseClaim(secret, token)
		if err != nil {
			return err
		}
		if claim.AuthorityID != authorityID || claim.SubscriptionID != subscriptionID || claim.ConsumerID != consumerID || claim.Prior != expectedPrior || claim.Next != nextPosition || claim.Next > claim.HighWater {
			return ErrCursorConflict
		}
		if claim.RetentionEpoch != epoch {
			return ErrReplayGap
		}
		s, err := readSubscription(ctx, tx, scope, consumerID, subscriptionID)
		if err != nil {
			return err
		}
		if s.RemovedAt != nil {
			return ErrNotFound
		}
		current = s.AcknowledgedSequence
		if current == nextPosition {
			return nil
		}
		if current != expectedPrior {
			return ErrCursorConflict
		}
		result, err := tx.ExecContext(ctx, "UPDATE subscriptions SET acknowledged_sequence=? WHERE subscription_id=? AND acknowledged_sequence=?", nextPosition, subscriptionID, expectedPrior)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrCursorConflict
		}
		current = nextPosition
		return nil
	})
	return current, err
}
