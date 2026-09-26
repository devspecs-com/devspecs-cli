package hubstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const jobSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"job":{"type":"string"},"duration":{"type":"integer"}},"required":["job","duration"],"additionalProperties":false}`

func registerJob(t *testing.T, f messageFixture) EventSchema {
	t.Helper()
	s, err := f.d.RegisterEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "job.finished", 1, []byte(jobSchema))
	require.NoError(t, err)
	return s
}

func jobInput(f messageFixture, payload string, key string) EventInput {
	return EventInput{AuthorityID: f.d.AuthorityID(), TypeKey: "job.finished", Version: 1, Payload: []byte(payload), IdempotencyKey: key}
}

func assertNoPublication(t *testing.T, f messageFixture) {
	t.Helper()
	var next, count int64
	require.NoError(t, f.d.sql.QueryRow("SELECT COALESCE((SELECT next_sequence FROM scope_counters WHERE scope_id=?),1)", f.topic.ScopeID).Scan(&next))
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM publications WHERE topic_id=?", f.topic.ID).Scan(&count))
	assert.Equal(t, int64(1), next)
	assert.Equal(t, int64(0), count)
}

func TestRegisterEventSchemaAndPublish(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	schema := registerJob(t, f)
	// Act
	e, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3}`, "job-1"))
	// Assert
	require.NoError(t, err)
	assert.Equal(t, "event", e.Kind)
	assert.Equal(t, int64(1), e.Sequence)
	assert.Equal(t, schema.SHA256, e.SchemaSHA256)
	assert.JSONEq(t, `{"duration":3,"job":"build"}`, string(e.Payload))
}

func TestEventWrongTypeRejectsWithoutSequence(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	// Act
	_, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":"three"}`, "bad"))
	// Assert
	assert.ErrorIs(t, err, ErrSchemaInvalid)
	assertNoPublication(t, f)
}

func TestEventMissingFieldRejects(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	// Act
	_, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build"}`, "bad"))
	// Assert
	assert.ErrorIs(t, err, ErrSchemaInvalid)
	assertNoPublication(t, f)
}

func TestEventExtraFieldRejects(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	// Act
	_, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3,"secret":"no"}`, "bad"))
	// Assert
	assert.ErrorIs(t, err, ErrSchemaInvalid)
	assert.NotContains(t, err.Error(), "secret")
	assertNoPublication(t, f)
}

func TestEventUnknownVersionRejects(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	in := jobInput(f, `{"job":"build","duration":3}`, "unknown")
	in.Version = 2
	// Act
	_, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", in)
	// Assert
	assert.ErrorIs(t, err, ErrVersionUnknown)
	assertNoPublication(t, f)
}

func TestUnsupportedDialectRejects(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	// Act
	_, err := f.d.RegisterEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "job.finished", 1, []byte(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object"}`))
	// Assert
	assert.ErrorIs(t, err, ErrUnsupportedSchema)
}

func TestInvalidSchemaRejects(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	// Act
	_, err := f.d.RegisterEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "job.finished", 1, []byte(`{"type":17}`))
	// Assert
	assert.ErrorIs(t, err, ErrSchemaInvalid)
}

func TestExternalRefRejects(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	// Act
	_, err := f.d.RegisterEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "job.finished", 1, []byte(`{"$ref":"file:///C:/secret.json"}`))
	// Assert
	assert.ErrorIs(t, err, ErrUnsupportedSchema)
}

func TestDuplicateSchemaKeyRejects(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	// Act
	_, err := f.d.RegisterEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "job.finished", 1, []byte(`{"properties":{"job":{"type":"string","type":"integer"}}}`))
	// Assert
	assert.ErrorIs(t, err, ErrSchemaInvalid)
}

func TestDuplicatePayloadKeyRejects(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	// Act
	_, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"a","duration":3,"job":"b"}`, "bad"))
	// Assert
	assert.ErrorIs(t, err, ErrInvalidInput)
	assertNoPublication(t, f)
}

func TestNestedDuplicatePayloadKeyRejects(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	// Act
	_, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3,"meta":{"a":1,"a":2}}`, "bad"))
	// Assert
	assert.ErrorIs(t, err, ErrInvalidInput)
	assertNoPublication(t, f)
}

func TestOversizedPayloadRejects(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	// Act
	_, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"`+strings.Repeat("x", 65536)+`","duration":3}`, "bad"))
	// Assert
	assert.ErrorIs(t, err, ErrInvalidInput)
	assertNoPublication(t, f)
}

func TestOversizedSchemaRejects(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	// Act
	_, err := f.d.RegisterEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "job.finished", 1, []byte(`{"description":"`+strings.Repeat("x", 16*1024)+`"}`))
	// Assert
	assert.ErrorIs(t, err, ErrSchemaInvalid)
}

func TestEventSharesSequenceWithMessage(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	m := f.post(t, "Started", "message", nil)
	// Act
	e, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3}`, "event"))
	// Assert
	require.NoError(t, err)
	assert.Equal(t, m.Sequence+1, e.Sequence)
}

func TestEquivalentKeyOrderReplaysEvent(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	first, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3}`, "stable"))
	require.NoError(t, err)
	// Act
	second, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"duration":3,"job":"build"}`, "stable"))
	// Assert
	require.NoError(t, err)
	assert.True(t, second.Replayed)
	assert.Equal(t, first.EntryID, second.EntryID)
}

func TestNonOwnerCannotRegisterSchema(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	// Act
	_, err := f.d.RegisterEventSchema(context.Background(), f.repo, f.topic.ID, "author", f.topic.PolicyGeneration, "job.finished", 1, []byte(jobSchema))
	// Assert
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestNonOwnerCannotRetireSchema(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	// Act
	_, err := f.d.RetireEventSchema(context.Background(), f.repo, f.topic.ID, "author", f.topic.PolicyGeneration+1, "job.finished", 1)
	// Assert
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestEventRetryReturnsOriginalAfterArchive(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	in := jobInput(f, `{"job":"build","duration":3}`, "stable")
	first, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", in)
	require.NoError(t, err)
	_, err = f.d.ArchiveTopic(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration+1, "done")
	require.NoError(t, err)
	// Act
	replayed, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", in)
	// Assert
	require.NoError(t, err)
	assert.True(t, replayed.Replayed)
	assert.Equal(t, first.EntryID, replayed.EntryID)
	assert.Equal(t, first.Sequence, replayed.Sequence)
}

func TestEventChangedRetryConflicts(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	_, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3}`, "stable"))
	require.NoError(t, err)
	// Act
	_, err = f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":4}`, "stable"))
	// Assert
	assert.ErrorIs(t, err, ErrIdempotencyConflict)
}

func TestRetiredVersionPreservesHistoricalSchema(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	original := registerJob(t, f)
	e, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3}`, "stable"))
	require.NoError(t, err)
	_, err = f.d.RetireEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration+1, "job.finished", 1)
	require.NoError(t, err)
	// Act
	s, err := f.d.ShowEventSchema(context.Background(), f.repo, f.topic.ID, "job.finished", 1)
	// Assert
	require.NoError(t, err)
	assert.Equal(t, original.SHA256, s.SHA256)
	assert.NotNil(t, s.RetiredAt)
	assert.Equal(t, e.SchemaSHA256, s.SHA256)
}

func TestRetiredVersionRejectsNewEvent(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	_, err := f.d.RetireEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration+1, "job.finished", 1)
	require.NoError(t, err)
	// Act
	_, err = f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3}`, "new"))
	// Assert
	assert.ErrorIs(t, err, ErrVersionRetired)
	assertNoPublication(t, f)
}

func TestIndependentProcessesRetrySameEvent(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	child := func() *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHubProcessPublishEvent$")
		cmd.Env = append(os.Environ(), "HUB_EVENT_HOME="+filepath.Dir(f.d.Path()), "HUB_EVENT_REPO="+f.repo, "HUB_EVENT_TOPIC="+f.topic.ID, "HUB_EVENT_AUTHORITY="+f.d.AuthorityID())
		return cmd
	}
	a, b := child(), child()
	var ao, bo bytes.Buffer
	a.Stdout = &ao
	a.Stderr = &ao
	b.Stdout = &bo
	b.Stderr = &bo
	// Act
	require.NoError(t, a.Start())
	require.NoError(t, b.Start())
	ae := a.Wait()
	be := b.Wait()
	// Assert
	require.NoError(t, ae, ao.String())
	require.NoError(t, be, bo.String())
	assert.Equal(t, 1, strings.Count(ao.String(), "created")+strings.Count(bo.String(), "created"))
	assert.Equal(t, 1, strings.Count(ao.String(), "replayed")+strings.Count(bo.String(), "replayed"))
	var count int
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM event_entries WHERE topic_id=?", f.topic.ID).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestHubProcessPublishEvent(t *testing.T) {
	if os.Getenv("HUB_EVENT_HOME") == "" {
		t.Skip("child process only")
	}
	d, err := Open(context.Background(), Options{Home: os.Getenv("HUB_EVENT_HOME")})
	require.NoError(t, err)
	defer d.Close()
	e, err := d.PublishEvent(context.Background(), os.Getenv("HUB_EVENT_REPO"), os.Getenv("HUB_EVENT_TOPIC"), "author", EventInput{AuthorityID: os.Getenv("HUB_EVENT_AUTHORITY"), TypeKey: "job.finished", Version: 1, Payload: []byte(`{"job":"build","duration":3}`), IdempotencyKey: "race"})
	require.NoError(t, err)
	if e.Replayed {
		fmt.Println("replayed")
	} else {
		fmt.Println("created")
	}
}

func TestShowEventSchemaIncludesDigest(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	s := registerJob(t, f)
	// Act
	shown, err := f.d.ShowEventSchema(context.Background(), f.repo, f.topic.ID, "job.finished", 1)
	// Assert
	require.NoError(t, err)
	assert.Equal(t, s.SHA256, shown.SHA256)
	assert.Equal(t, EventDialect, shown.Dialect)
}

func TestOpenMigratesVerifiedV2AuthorityToV4(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	require.NoError(t, os.MkdirAll(home, 0o700))
	raw, err := sql.Open("sqlite", filepath.Join(home, "hub.sqlite"))
	require.NoError(t, err)
	_, err = raw.Exec(schemaV1)
	require.NoError(t, err)
	_, err = raw.Exec("INSERT INTO hub_meta (singleton,db_id,format_version) VALUES (1,'v2-authority',1)")
	require.NoError(t, err)
	_, err = raw.Exec("INSERT INTO schema_migrations VALUES (1,?,0)", fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV1))))
	require.NoError(t, err)
	_, err = raw.Exec(schemaV2)
	require.NoError(t, err)
	_, err = raw.Exec("INSERT INTO schema_migrations VALUES (2,?,0)", fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV2))))
	require.NoError(t, err)
	_, err = raw.Exec(fmt.Sprintf("PRAGMA application_id=%d", applicationID))
	require.NoError(t, err)
	_, err = raw.Exec("PRAGMA user_version=2")
	require.NoError(t, err)
	require.NoError(t, raw.Close())
	// Act
	d, err := Open(context.Background(), Options{Home: home})
	// Assert
	require.NoError(t, err)
	defer d.Close()
	assert.Equal(t, "v2-authority", d.AuthorityID())
	var version, count int
	require.NoError(t, d.sql.QueryRow("PRAGMA user_version").Scan(&version))
	require.NoError(t, d.sql.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count))
	assert.Equal(t, 4, version)
	assert.Equal(t, 4, count)
}

func TestOpenMigratesVerifiedV3AuthorityWithPullSecret(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	require.NoError(t, os.MkdirAll(home, 0o700))
	raw, err := sql.Open("sqlite", filepath.Join(home, "hub.sqlite"))
	require.NoError(t, err)
	_, err = raw.Exec(schemaV1)
	require.NoError(t, err)
	_, err = raw.Exec("INSERT INTO hub_meta (singleton,db_id,format_version) VALUES (1,'v3-authority',1)")
	require.NoError(t, err)
	_, err = raw.Exec("INSERT INTO schema_migrations VALUES (1,?,0)", fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV1))))
	require.NoError(t, err)
	_, err = raw.Exec(schemaV2)
	require.NoError(t, err)
	_, err = raw.Exec("INSERT INTO schema_migrations VALUES (2,?,0)", fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV2))))
	require.NoError(t, err)
	_, err = raw.Exec(schemaV3)
	require.NoError(t, err)
	_, err = raw.Exec("INSERT INTO schema_migrations VALUES (3,?,0)", fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV3))))
	require.NoError(t, err)
	_, err = raw.Exec(fmt.Sprintf("PRAGMA application_id=%d", applicationID))
	require.NoError(t, err)
	_, err = raw.Exec("PRAGMA user_version=3")
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	// Act
	d, err := Open(context.Background(), Options{Home: home})

	// Assert
	require.NoError(t, err)
	defer d.Close()
	assert.Equal(t, "v3-authority", d.AuthorityID())
	var version, secretLength int
	var digest string
	require.NoError(t, d.sql.QueryRow("PRAGMA user_version").Scan(&version))
	require.NoError(t, d.sql.QueryRow("SELECT length(secret) FROM pull_token_secret WHERE singleton=1").Scan(&secretLength))
	require.NoError(t, d.sql.QueryRow("SELECT digest FROM schema_migrations WHERE version=4").Scan(&digest))
	assert.Equal(t, 4, version)
	assert.Equal(t, 32, secretLength)
	assert.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV4))), digest)
}

func TestSchemaWorkHonorsCancellation(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Act
	_, err := boundedSchemaWork(ctx, ErrUnsupportedSchema, func() (int, error) { return 1, nil })
	// Assert
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSchemaWorkTimesOut(t *testing.T) {
	// Arrange
	release := make(chan struct{})
	defer close(release)
	// Act
	_, err := boundedSchemaWork(context.Background(), ErrUnsupportedSchema, func() (int, error) { <-release; return 1, nil })
	// Assert
	assert.ErrorIs(t, err, ErrUnsupportedSchema)
}
