package hubstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

const EventDialect = "https://json-schema.org/draft/2020-12/schema"

var (
	ErrSchemaInvalid     = errors.New("hub: schema invalid")
	ErrUnsupportedSchema = errors.New("hub: unsupported schema")
	ErrVersionUnknown    = errors.New("hub: event version unknown")
	ErrVersionRetired    = errors.New("hub: event version retired")
)

var schemaWorkSlots = make(chan struct{}, 2)

type schemaResult[T any] struct {
	value T
	err   error
}

func boundedSchemaWork[T any](ctx context.Context, timeoutError error, work func() (T, error)) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case schemaWorkSlots <- struct{}{}:
	case <-timer.C:
		return zero, timeoutError
	case <-ctx.Done():
		return zero, ctx.Err()
	}
	result := make(chan schemaResult[T], 1)
	go func() {
		defer func() { <-schemaWorkSlots }()
		value, err := work()
		result <- schemaResult[T]{value, err}
	}()
	select {
	case done := <-result:
		return done.value, done.err
	case <-timer.C:
		return zero, timeoutError
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

type ValidationFailure struct {
	Pointer string `json:"instance_pointer"`
}

func (e *ValidationFailure) Error() string { return "hub: schema invalid at " + e.Pointer }
func (e *ValidationFailure) Unwrap() error { return ErrSchemaInvalid }

type EventSchema struct {
	TopicID      string          `json:"topic_id"`
	TypeKey      string          `json:"type_key"`
	Version      int64           `json:"version"`
	Dialect      string          `json:"dialect"`
	Schema       json.RawMessage `json:"schema"`
	SHA256       string          `json:"schema_sha256"`
	RegisteredBy string          `json:"registered_by"`
	RegisteredAt time.Time       `json:"registered_at"`
	RetiredBy    string          `json:"retired_by,omitempty"`
	RetiredAt    *time.Time      `json:"retired_at,omitempty"`
}

type EventInput struct {
	AuthorityID     string
	TypeKey         string
	Version         int64
	Payload         json.RawMessage
	CorrectsEntryID string
	ExpiresAt       *time.Time
	OccurredAt      *time.Time
	SourceRefs      []SourceRef
	CorrelationRefs []string
	IdempotencyKey  string
}

type Event struct {
	Publication
	TypeKey         string          `json:"type_key"`
	Version         int64           `json:"version"`
	SchemaSHA256    string          `json:"schema_sha256"`
	Payload         json.RawMessage `json:"payload"`
	CorrectsEntryID string          `json:"corrects_entry_id,omitempty"`
}

type noSchemaLoader struct{}

func (noSchemaLoader) Load(string) (any, error) { return nil, ErrUnsupportedSchema }

// decodeUnique rejects ambiguous JSON before either the validator or digest sees it.
func decodeUnique(raw []byte, maxBytes, maxDepth, maxNodes int) (any, []byte, error) {
	if len(raw) == 0 || len(raw) > maxBytes || !utf8.Valid(raw) {
		return nil, nil, ErrInvalidInput
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	nodes := 0
	v, err := decodeValue(dec, 0, maxDepth, &nodes, maxNodes)
	if err != nil {
		return nil, nil, ErrInvalidInput
	}
	if _, err = dec.Token(); err != io.EOF {
		return nil, nil, ErrInvalidInput
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return nil, nil, ErrInvalidInput
	}
	return v, canonical, nil
}

func decodeValue(dec *json.Decoder, depth, maxDepth int, nodes *int, maxNodes int) (any, error) {
	*nodes++
	if depth > maxDepth || *nodes > maxNodes {
		return nil, ErrInvalidInput
	}
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		obj := map[string]any{}
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyTok.(string)
			if !ok {
				return nil, ErrInvalidInput
			}
			if _, exists := obj[key]; exists {
				return nil, ErrInvalidInput
			}
			val, err := decodeValue(dec, depth+1, maxDepth, nodes, maxNodes)
			if err != nil {
				return nil, err
			}
			obj[key] = val
		}
		_, err := dec.Token()
		return obj, err
	case json.Delim('['):
		arr := []any{}
		for dec.More() {
			val, err := decodeValue(dec, depth+1, maxDepth, nodes, maxNodes)
			if err != nil {
				return nil, err
			}
			arr = append(arr, val)
		}
		_, err := dec.Token()
		return arr, err
	default:
		if _, ok := tok.(json.Delim); ok {
			return nil, ErrInvalidInput
		}
		return tok, nil
	}
}

func checkSchemaPolicy(v any, root bool) error {
	x, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	for k, item := range x {
		switch k {
		case "$ref", "$dynamicRef", "$recursiveRef", "$id", "$vocabulary", "allOf", "anyOf", "oneOf", "not", "if", "then", "else", "contains", "uniqueItems", "unevaluatedProperties", "unevaluatedItems", "patternProperties", "dependentSchemas":
			return ErrUnsupportedSchema
		case "$schema":
			if !root || item != EventDialect {
				return ErrUnsupportedSchema
			}
		case "properties", "$defs":
			if children, ok := item.(map[string]any); ok {
				for _, child := range children {
					if err := checkSchemaPolicy(child, false); err != nil {
						return err
					}
				}
			}
		case "items", "additionalProperties", "propertyNames":
			if err := checkSchemaPolicy(item, false); err != nil {
				return err
			}
		case "prefixItems":
			if children, ok := item.([]any); ok {
				for _, child := range children {
					if err := checkSchemaPolicy(child, false); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func compileEventSchema(v any) (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	c.UseLoader(noSchemaLoader{})
	if err := c.AddResource("urn:devspecs:event-schema", v); err != nil {
		return nil, ErrSchemaInvalid
	}
	sch, err := c.Compile("urn:devspecs:event-schema")
	if err != nil {
		return nil, ErrSchemaInvalid
	}
	return sch, nil
}

func validTypeKey(key string) bool {
	if len(key) == 0 || len(key) > 128 {
		return false
	}
	parts := strings.Split(key, ".")
	for _, part := range parts {
		if !validKey(part) {
			return false
		}
	}
	return true
}

func schemaDigest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func scanEventSchema(row rowScanner) (EventSchema, error) {
	var s EventSchema
	var raw string
	var at int64
	var retired sql.NullInt64
	var actor sql.NullString
	err := row.Scan(&s.TopicID, &s.TypeKey, &s.Version, &s.Dialect, &raw, &s.SHA256, &s.RegisteredBy, &at, &actor, &retired)
	if err != nil {
		return s, err
	}
	s.Schema = json.RawMessage(raw)
	if s.Dialect != EventDialect || s.SHA256 != schemaDigest(s.Schema) {
		return EventSchema{}, ErrUnsupportedFormat
	}
	s.RegisteredAt = fromMillis(at)
	s.RetiredBy = actor.String
	if retired.Valid {
		t := fromMillis(retired.Int64)
		s.RetiredAt = &t
	}
	return s, nil
}

const eventSchemaColumns = "topic_id,type_key,version,dialect,schema_json,schema_sha256,registered_by,registered_at,retired_by,retired_at"

func (d *DB) RegisterEventSchema(ctx context.Context, repoPath, topicID, actorID string, expectedGeneration int64, typeKey string, version int64, raw json.RawMessage) (EventSchema, error) {
	if !validTypeKey(typeKey) || version < 1 {
		return EventSchema{}, ErrInvalidInput
	}
	v, canonical, err := decodeUnique(raw, 16*1024, 16, 256)
	if err != nil {
		return EventSchema{}, ErrSchemaInvalid
	}
	if _, ok := v.(map[string]any); !ok {
		return EventSchema{}, ErrSchemaInvalid
	}
	if err := checkSchemaPolicy(v, true); err != nil {
		return EventSchema{}, err
	}
	if _, err = boundedSchemaWork(ctx, ErrUnsupportedSchema, func() (*jsonschema.Schema, error) { return compileEventSchema(v) }); err != nil {
		return EventSchema{}, err
	}
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return EventSchema{}, err
	}
	var out EventSchema
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
		_, err = tx.ExecContext(ctx, "INSERT INTO event_schemas (topic_id,type_key,version,dialect,schema_json,schema_sha256,registered_by,registered_at) VALUES (?,?,?,?,?,?,?,?)", topicID, typeKey, version, EventDialect, string(canonical), schemaDigest(canonical), actorID, now)
		if err != nil {
			return uniqueOrOriginal(err)
		}
		_, err = tx.ExecContext(ctx, "UPDATE topics SET policy_generation=policy_generation+1 WHERE topic_id=?", topicID)
		if err != nil {
			return err
		}
		out, err = scanEventSchema(tx.QueryRowContext(ctx, "SELECT "+eventSchemaColumns+" FROM event_schemas WHERE topic_id=? AND type_key=? AND version=?", topicID, typeKey, version))
		if err != nil {
			return err
		}
		return appendAudit(ctx, tx, topicID, actorID, "register_event_schema", now, nil, map[string]any{"type_key": typeKey, "version": version, "sha256": out.SHA256})
	})
	return out, err
}

func (d *DB) RetireEventSchema(ctx context.Context, repoPath, topicID, actorID string, expectedGeneration int64, typeKey string, version int64) (EventSchema, error) {
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return EventSchema{}, err
	}
	var out EventSchema
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
		result, err := tx.ExecContext(ctx, "UPDATE event_schemas SET retired_by=?,retired_at=? WHERE topic_id=? AND type_key=? AND version=? AND retired_at IS NULL", actorID, now, topicID, typeKey, version)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrVersionUnknown
		}
		_, err = tx.ExecContext(ctx, "UPDATE topics SET policy_generation=policy_generation+1 WHERE topic_id=?", topicID)
		if err != nil {
			return err
		}
		out, err = scanEventSchema(tx.QueryRowContext(ctx, "SELECT "+eventSchemaColumns+" FROM event_schemas WHERE topic_id=? AND type_key=? AND version=?", topicID, typeKey, version))
		if err != nil {
			return err
		}
		return appendAudit(ctx, tx, topicID, actorID, "retire_event_schema", now, nil, map[string]any{"type_key": typeKey, "version": version})
	})
	return out, err
}

func (d *DB) ShowEventSchema(ctx context.Context, repoPath, topicID, typeKey string, version int64) (EventSchema, error) {
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return EventSchema{}, err
	}
	s, err := scanEventSchema(d.sql.QueryRowContext(ctx, "SELECT s.* FROM event_schemas s JOIN topics t ON t.topic_id=s.topic_id WHERE t.scope_id=? AND s.topic_id=? AND s.type_key=? AND s.version=?", scope.ID, topicID, typeKey, version))
	if errors.Is(err, sql.ErrNoRows) {
		return EventSchema{}, ErrVersionUnknown
	}
	return s, err
}

func (d *DB) ListEventSchemas(ctx context.Context, repoPath, topicID string, limit, offset int) ([]EventSchema, error) {
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, ErrInvalidInput
	}
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	var exists string
	if err = d.sql.QueryRowContext(ctx, "SELECT topic_id FROM topics WHERE scope_id=? AND topic_id=?", scope.ID, topicID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := d.sql.QueryContext(ctx, "SELECT "+eventSchemaColumns+" FROM event_schemas WHERE topic_id=? ORDER BY type_key,version LIMIT ? OFFSET ?", topicID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EventSchema{}
	for rows.Next() {
		s, err := scanEventSchema(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanEvent(row rowScanner) (Event, error) {
	var e Event
	var committed int64
	var occurred, expiry sql.NullInt64
	var source, correlation, payload string
	var key, corrects sql.NullString
	err := row.Scan(&e.EntryID, &e.ScopeID, &e.TopicID, &e.ActorID, &e.Kind, &e.Sequence, &committed, &occurred, &expiry, &source, &correlation, &key, &e.TypeKey, &e.Version, &e.SchemaSHA256, &payload, &corrects)
	if err != nil {
		return e, err
	}
	e.CommittedAt = fromMillis(committed)
	if occurred.Valid {
		t := fromMillis(occurred.Int64)
		e.OccurredAt = &t
	}
	if expiry.Valid {
		t := fromMillis(expiry.Int64)
		e.ExpiresAt = &t
	}
	if json.Unmarshal([]byte(source), &e.SourceRefs) != nil || json.Unmarshal([]byte(correlation), &e.CorrelationRefs) != nil {
		return Event{}, ErrUnsupportedFormat
	}
	e.IdempotencyKey = key.String
	e.CorrectsEntryID = corrects.String
	e.Payload = json.RawMessage(payload)
	return e, nil
}

const eventColumns = "p.entry_id,p.scope_id,p.topic_id,p.actor_id,p.kind,p.sequence,p.committed_at,p.occurred_at,p.expires_at,p.source_refs,p.correlation_refs,p.idempotency_key,e.type_key,e.version,e.schema_sha256,e.payload_json,e.corrects_entry_id"

func (d *DB) ReadEvent(ctx context.Context, repoPath, entryID string, historical bool) (Event, error) {
	scope, err := d.LookupRepo(ctx, repoPath)
	if err != nil {
		return Event{}, err
	}
	e, err := scanEvent(d.sql.QueryRowContext(ctx, "SELECT "+eventColumns+" FROM event_entries e JOIN publications p ON p.entry_id=e.entry_id WHERE p.scope_id=? AND p.entry_id=?", scope.ID, entryID))
	if errors.Is(err, sql.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	if err != nil {
		return Event{}, err
	}
	if !historical {
		topic, err := d.ShowTopic(ctx, repoPath, e.TopicID)
		if err != nil {
			return Event{}, err
		}
		if topic.State != "active" {
			return Event{}, ErrArchived
		}
		if e.ExpiresAt != nil && !d.now().UTC().Before(*e.ExpiresAt) {
			return Event{}, ErrExpired
		}
	}
	return e, nil
}

func eventFingerprint(in EventInput, canonical []byte) string {
	b, _ := json.Marshal(struct {
		Kind, TypeKey         string
		Version               int64
		Payload               json.RawMessage
		Corrects              string
		ExpiresAt, OccurredAt *time.Time
		Sources               []SourceRef
		Correlations          []string
	}{"event", in.TypeKey, in.Version, canonical, in.CorrectsEntryID, in.ExpiresAt, in.OccurredAt, in.SourceRefs, in.CorrelationRefs})
	return schemaDigest(b)
}

func (d *DB) PublishEvent(ctx context.Context, repoPath, topicID, actorID string, in EventInput) (Event, error) {
	if err := d.expectedAuthority(in.AuthorityID); err != nil {
		return Event{}, err
	}
	if !validTypeKey(in.TypeKey) || in.Version < 1 || len(in.IdempotencyKey) > 128 || !utf8.ValidString(in.IdempotencyKey) || strings.TrimSpace(in.IdempotencyKey) != in.IdempotencyKey {
		return Event{}, ErrInvalidInput
	}
	if in.OccurredAt != nil && (in.OccurredAt.IsZero() || in.OccurredAt.Year() < 1970) {
		return Event{}, ErrInvalidInput
	}
	if in.ExpiresAt != nil && (in.ExpiresAt.IsZero() || in.ExpiresAt.Year() < 1970) {
		return Event{}, ErrInvalidInput
	}
	if err := validateRefs(in.SourceRefs, in.CorrelationRefs); err != nil {
		return Event{}, err
	}
	v, canonical, err := decodeUnique(in.Payload, 64*1024, 32, 10000)
	if err != nil {
		return Event{}, ErrInvalidInput
	}
	if in.SourceRefs == nil {
		in.SourceRefs = []SourceRef{}
	}
	if in.CorrelationRefs == nil {
		in.CorrelationRefs = []string{}
	}
	if in.ExpiresAt != nil {
		t := fromMillis(in.ExpiresAt.UnixMilli())
		in.ExpiresAt = &t
	}
	if in.OccurredAt != nil {
		t := fromMillis(in.OccurredAt.UnixMilli())
		in.OccurredAt = &t
	}
	fp := eventFingerprint(in, canonical)
	ev, err := resolveGit(ctx, repoPath, false)
	if err != nil {
		return Event{}, err
	}
	if in.IdempotencyKey != "" {
		scope, err := d.LookupRepo(ctx, repoPath)
		if err != nil {
			return Event{}, err
		}
		var stored, entry string
		err = d.sql.QueryRowContext(ctx, "SELECT fingerprint,entry_id FROM publication_idempotency WHERE scope_id=? AND topic_id=? AND actor_id=? AND key=?", scope.ID, topicID, actorID, in.IdempotencyKey).Scan(&stored, &entry)
		if err == nil {
			if stored != fp {
				return Event{}, ErrIdempotencyConflict
			}
			out, readErr := d.ReadEvent(ctx, repoPath, entry, true)
			if errors.Is(readErr, ErrNotFound) {
				return Event{}, ErrIdempotencyConflict
			}
			if readErr != nil {
				return Event{}, readErr
			}
			out.Replayed = true
			return out, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Event{}, err
		}
	}
	// Compilation and validation happen before entering SQLite's writer slot.
	s, err := d.ShowEventSchema(ctx, repoPath, topicID, in.TypeKey, in.Version)
	if err != nil {
		return Event{}, err
	}
	if s.RetiredAt != nil {
		return Event{}, ErrVersionRetired
	}
	schemaValue, _, err := decodeUnique(s.Schema, 16*1024, 16, 256)
	if err != nil {
		return Event{}, ErrUnsupportedFormat
	}
	compiled, err := boundedSchemaWork(ctx, ErrUnsupportedFormat, func() (*jsonschema.Schema, error) { return compileEventSchema(schemaValue) })
	if err != nil {
		return Event{}, ErrUnsupportedFormat
	}
	_, err = boundedSchemaWork(ctx, ErrSchemaInvalid, func() (struct{}, error) { return struct{}{}, compiled.Validate(v) })
	if err != nil {
		return Event{}, &ValidationFailure{Pointer: ""}
	}
	var out Event
	err = d.write(ctx, func(tx *sql.Tx, now int64) error {
		scope, err := d.boundScope(ctx, tx, ev)
		if err != nil {
			return err
		}
		if err := enrolled(ctx, tx, actorID); err != nil {
			return err
		}
		if in.IdempotencyKey != "" {
			var stored, entry string
			err := tx.QueryRowContext(ctx, "SELECT fingerprint,entry_id FROM publication_idempotency WHERE scope_id=? AND topic_id=? AND actor_id=? AND key=?", scope, topicID, actorID, in.IdempotencyKey).Scan(&stored, &entry)
			if err == nil {
				if stored != fp {
					return ErrIdempotencyConflict
				}
				out, err = scanEvent(tx.QueryRowContext(ctx, "SELECT "+eventColumns+" FROM event_entries e JOIN publications p ON p.entry_id=e.entry_id WHERE p.entry_id=?", entry))
				if errors.Is(err, sql.ErrNoRows) {
					return ErrIdempotencyConflict
				}
				out.Replayed = true
				return err
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
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
		current, err := scanEventSchema(tx.QueryRowContext(ctx, "SELECT "+eventSchemaColumns+" FROM event_schemas WHERE topic_id=? AND type_key=? AND version=?", topicID, in.TypeKey, in.Version))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrVersionUnknown
		}
		if err != nil {
			return err
		}
		if current.RetiredAt != nil {
			return ErrVersionRetired
		}
		if current.SHA256 != s.SHA256 {
			return ErrConflict
		}
		expiry, err := expiryMillis(in.ExpiresAt, now)
		if err != nil {
			return err
		}
		if in.CorrectsEntryID != "" {
			var found string
			err := tx.QueryRowContext(ctx, "SELECT e.entry_id FROM event_entries e JOIN publications p ON p.entry_id=e.entry_id WHERE e.entry_id=? AND p.scope_id=? AND p.topic_id=?", in.CorrectsEntryID, scope, topicID).Scan(&found)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
		}
		if err := verifySourceRefs(ctx, tx, in.SourceRefs); err != nil {
			return err
		}
		sequence, err := nextSequence(ctx, tx, scope)
		if err != nil {
			return err
		}
		entryID, err := randomID()
		if err != nil {
			return err
		}
		sources, _ := json.Marshal(in.SourceRefs)
		correlations, _ := json.Marshal(in.CorrelationRefs)
		var occurred, key, corrects any
		if in.OccurredAt != nil {
			occurred = in.OccurredAt.UnixMilli()
		}
		if in.IdempotencyKey != "" {
			key = in.IdempotencyKey
		}
		if in.CorrectsEntryID != "" {
			corrects = in.CorrectsEntryID
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO publications (entry_id,scope_id,topic_id,actor_id,kind,sequence,committed_at,occurred_at,expires_at,source_refs,correlation_refs,idempotency_key) VALUES (?,?,?,?,'event',?,?,?,?,?,?,?)", entryID, scope, topicID, actorID, sequence, now, occurred, expiry, string(sources), string(correlations), key)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO event_entries VALUES (?,?,?,?,?,?,?)", entryID, topicID, in.TypeKey, in.Version, s.SHA256, string(canonical), corrects)
		if err != nil {
			return err
		}
		if in.IdempotencyKey != "" {
			_, err = tx.ExecContext(ctx, "INSERT INTO publication_idempotency VALUES (?,?,?,?,?,?)", scope, topicID, actorID, in.IdempotencyKey, fp, entryID)
			if err != nil {
				return uniqueOrOriginal(err)
			}
		}
		out, err = scanEvent(tx.QueryRowContext(ctx, "SELECT "+eventColumns+" FROM event_entries e JOIN publications p ON p.entry_id=e.entry_id WHERE p.entry_id=?", entryID))
		return err
	})
	return out, err
}
