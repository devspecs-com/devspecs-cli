package hubstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnrollGlobalScopeCreatesOneUnboundScope(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)

	// Act
	scope, err := d.EnrollRepo(ctx, GlobalScopeSelector)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "global", scope.Kind)
	assert.Empty(t, scope.CommonDir)
	var bindings int
	require.NoError(t, d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM scope_bindings WHERE scope_id=?", scope.ID).Scan(&bindings))
	assert.Zero(t, bindings)
}

func TestLookupGlobalScopeBeforeEnrollmentDoesNotCreateIt(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)

	// Act
	_, err := d.LookupRepo(ctx, GlobalScopeSelector)

	// Assert
	assert.ErrorIs(t, err, ErrNotFound)
	var count int
	require.NoError(t, d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM repo_scopes WHERE kind='global'").Scan(&count))
	assert.Zero(t, count)
}

func TestEnrollGlobalScopeReusesExistingScope(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	first, err := d.EnrollRepo(ctx, GlobalScopeSelector)
	require.NoError(t, err)

	// Act
	second, err := d.EnrollRepo(ctx, GlobalScopeSelector)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, "global", second.Kind)
}

func TestGlobalTopicIsNotListedInRepository(t *testing.T) {
	// Arrange
	ctx := context.Background()
	repo := testRepo(t)
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	_, err := d.EnrollRepo(ctx, repo)
	require.NoError(t, err)
	_, err = d.EnrollRepo(ctx, GlobalScopeSelector)
	require.NoError(t, err)
	_, err = d.EnrollActor(ctx, "owner")
	require.NoError(t, err)
	_, err = d.CreateTopic(ctx, GlobalScopeSelector, "owner", TopicInput{Key: "local-go", Name: "Local Go", Description: "Machine load"})
	require.NoError(t, err)

	// Act
	topics, err := d.ListTopics(ctx, repo, TopicList{})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, topics)
}

func TestRepositoryTopicIsNotListedInGlobalScope(t *testing.T) {
	// Arrange
	ctx := context.Background()
	repo := testRepo(t)
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	_, err := d.EnrollRepo(ctx, repo)
	require.NoError(t, err)
	_, err = d.EnrollRepo(ctx, GlobalScopeSelector)
	require.NoError(t, err)
	_, err = d.EnrollActor(ctx, "owner")
	require.NoError(t, err)
	_, err = d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "local-go", Name: "Local Go", Description: "Repo work"})
	require.NoError(t, err)

	// Act
	topics, err := d.ListTopics(ctx, GlobalScopeSelector, TopicList{})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, topics)
}

func TestGlobalTopicUsesHomeAuthority(t *testing.T) {
	// Arrange
	ctx := context.Background()
	first := testDB(t, filepath.Join(t.TempDir(), "first"), nil)
	second := testDB(t, filepath.Join(t.TempDir(), "second"), nil)
	_, err := first.EnrollRepo(ctx, GlobalScopeSelector)
	require.NoError(t, err)
	_, err = second.EnrollRepo(ctx, GlobalScopeSelector)
	require.NoError(t, err)
	_, err = first.EnrollActor(ctx, "owner")
	require.NoError(t, err)
	_, err = first.CreateTopic(ctx, GlobalScopeSelector, "owner", TopicInput{Key: "local-go", Name: "Local Go", Description: "First home"})
	require.NoError(t, err)

	// Act
	topics, err := second.ListTopics(ctx, GlobalScopeSelector, TopicList{})

	// Assert
	require.NoError(t, err)
	assert.Empty(t, topics)
}

func TestGlobalSubscriptionPullsMessageWithoutRepository(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	_, err := d.EnrollRepo(ctx, GlobalScopeSelector)
	require.NoError(t, err)
	_, err = d.EnrollActor(ctx, "owner")
	require.NoError(t, err)
	topic, err := d.CreateTopic(ctx, GlobalScopeSelector, "owner", TopicInput{Key: "local-go", Name: "Local Go", Description: "Machine load"})
	require.NoError(t, err)
	_, err = d.EnrollConsumer(ctx, d.AuthorityID(), "reader")
	require.NoError(t, err)
	sub, err := d.Subscribe(ctx, GlobalScopeSelector, SubscribeInput{AuthorityID: d.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{topic.ID}, FromBeginning: true})
	require.NoError(t, err)
	message, err := d.PostMessage(ctx, GlobalScopeSelector, topic.ID, "owner", MessageInput{AuthorityID: d.AuthorityID(), Text: "Heavy tests running", IdempotencyKey: "notice-1"})
	require.NoError(t, err)

	// Act
	page, err := d.Pull(ctx, GlobalScopeSelector, "reader", sub.ID, 10)

	// Assert
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	require.NotNil(t, page.Entries[0].Message)
	assert.Equal(t, message.EntryID, page.Entries[0].Message.EntryID)
	assert.NotEmpty(t, page.AckToken)
}

func TestRepositoryCannotPullGlobalSubscription(t *testing.T) {
	// Arrange
	ctx := context.Background()
	repo := testRepo(t)
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	_, err := d.EnrollRepo(ctx, repo)
	require.NoError(t, err)
	_, err = d.EnrollRepo(ctx, GlobalScopeSelector)
	require.NoError(t, err)
	_, err = d.EnrollActor(ctx, "owner")
	require.NoError(t, err)
	topic, err := d.CreateTopic(ctx, GlobalScopeSelector, "owner", TopicInput{Key: "local-go", Name: "Local Go", Description: "Machine load"})
	require.NoError(t, err)
	_, err = d.EnrollConsumer(ctx, d.AuthorityID(), "reader")
	require.NoError(t, err)
	sub, err := d.Subscribe(ctx, GlobalScopeSelector, SubscribeInput{AuthorityID: d.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{topic.ID}})
	require.NoError(t, err)

	// Act
	_, err = d.Pull(ctx, repo, "reader", sub.ID, 10)

	// Assert
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestGlobalSubscriptionAcknowledgesDelivery(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	_, err := d.EnrollRepo(ctx, GlobalScopeSelector)
	require.NoError(t, err)
	_, err = d.EnrollActor(ctx, "owner")
	require.NoError(t, err)
	topic, err := d.CreateTopic(ctx, GlobalScopeSelector, "owner", TopicInput{Key: "local-go", Name: "Local Go", Description: "Machine load"})
	require.NoError(t, err)
	_, err = d.EnrollConsumer(ctx, d.AuthorityID(), "reader")
	require.NoError(t, err)
	sub, err := d.Subscribe(ctx, GlobalScopeSelector, SubscribeInput{AuthorityID: d.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{topic.ID}, FromBeginning: true})
	require.NoError(t, err)
	_, err = d.PostMessage(ctx, GlobalScopeSelector, topic.ID, "owner", MessageInput{AuthorityID: d.AuthorityID(), Text: "Heavy tests running"})
	require.NoError(t, err)
	page, err := d.Pull(ctx, GlobalScopeSelector, "reader", sub.ID, 10)
	require.NoError(t, err)

	// Act
	position, err := d.Ack(ctx, GlobalScopeSelector, "reader", sub.ID, d.AuthorityID(), page.PriorAcknowledged, page.NextScanPosition, page.AckToken)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, page.NextScanPosition, position)
}

func TestGlobalEventRejectsInvalidPayloadWithoutPublication(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	_, err := d.EnrollRepo(ctx, GlobalScopeSelector)
	require.NoError(t, err)
	_, err = d.EnrollActor(ctx, "owner")
	require.NoError(t, err)
	topic, err := d.CreateTopic(ctx, GlobalScopeSelector, "owner", TopicInput{Key: "local-go", Name: "Local Go", Description: "Machine load"})
	require.NoError(t, err)
	_, err = d.RegisterEventSchema(ctx, GlobalScopeSelector, topic.ID, "owner", topic.PolicyGeneration, "job.finished", 1, []byte(jobSchema))
	require.NoError(t, err)

	// Act
	_, err = d.PublishEvent(ctx, GlobalScopeSelector, topic.ID, "owner", EventInput{AuthorityID: d.AuthorityID(), TypeKey: "job.finished", Version: 1, Payload: []byte(`{"job":"build","duration":"invalid"}`), IdempotencyKey: "bad-event"})

	// Assert
	assert.ErrorIs(t, err, ErrSchemaInvalid)
	var count int
	require.NoError(t, d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM publications WHERE scope_id=?", topic.ScopeID).Scan(&count))
	assert.Zero(t, count)
}

func TestGlobalMessageIdempotencyReplaysOriginalPublication(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	_, err := d.EnrollRepo(ctx, GlobalScopeSelector)
	require.NoError(t, err)
	_, err = d.EnrollActor(ctx, "owner")
	require.NoError(t, err)
	topic, err := d.CreateTopic(ctx, GlobalScopeSelector, "owner", TopicInput{Key: "local-go", Name: "Local Go", Description: "Machine load"})
	require.NoError(t, err)
	input := MessageInput{AuthorityID: d.AuthorityID(), Text: "Heavy tests running", IdempotencyKey: "run-1"}
	original, err := d.PostMessage(ctx, GlobalScopeSelector, topic.ID, "owner", input)
	require.NoError(t, err)

	// Act
	replay, err := d.PostMessage(ctx, GlobalScopeSelector, topic.ID, "owner", input)

	// Assert
	require.NoError(t, err)
	assert.True(t, replay.Replayed)
	assert.Equal(t, original.EntryID, replay.EntryID)
	var count int
	require.NoError(t, d.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM publications WHERE scope_id=?", topic.ScopeID).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestConcurrentGlobalPublicationsUseDistinctSequences(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	home := filepath.Join(t.TempDir(), "home")
	first := testDB(t, home, nil)
	second := testDB(t, home, nil)
	_, err := first.EnrollRepo(ctx, GlobalScopeSelector)
	require.NoError(t, err)
	_, err = first.EnrollActor(ctx, "owner")
	require.NoError(t, err)
	topic, err := first.CreateTopic(ctx, GlobalScopeSelector, "owner", TopicInput{Key: "local-go", Name: "Local Go", Description: "Machine load"})
	require.NoError(t, err)
	type outcome struct {
		message Message
		err     error
	}
	firstDone := make(chan outcome, 1)
	secondDone := make(chan outcome, 1)

	// Act
	go func() {
		message, err := first.PostMessage(ctx, GlobalScopeSelector, topic.ID, "owner", MessageInput{AuthorityID: first.AuthorityID(), Text: "First", IdempotencyKey: "first"})
		firstDone <- outcome{message, err}
	}()
	go func() {
		message, err := second.PostMessage(ctx, GlobalScopeSelector, topic.ID, "owner", MessageInput{AuthorityID: second.AuthorityID(), Text: "Second", IdempotencyKey: "second"})
		secondDone <- outcome{message, err}
	}()
	firstResult := <-firstDone
	secondResult := <-secondDone

	// Assert
	require.NoError(t, firstResult.err)
	require.NoError(t, secondResult.err)
	assert.NotEqual(t, firstResult.message.Sequence, secondResult.message.Sequence)
	var count int
	require.NoError(t, first.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM publications WHERE scope_id=?", topic.ScopeID).Scan(&count))
	assert.Equal(t, 2, count)
}

func TestV6MigrationPreservesRepositoryPublication(t *testing.T) {
	// Arrange
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "home")
	repo := testRepo(t)
	d, err := Open(ctx, Options{Home: home})
	require.NoError(t, err)
	_, err = d.EnrollRepo(ctx, repo)
	require.NoError(t, err)
	_, err = d.EnrollActor(ctx, "owner")
	require.NoError(t, err)
	topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "work", Name: "Work", Description: "Existing data"})
	require.NoError(t, err)
	message, err := d.PostMessage(ctx, repo, topic.ID, "owner", MessageInput{AuthorityID: d.AuthorityID(), Text: "Preserve this"})
	require.NoError(t, err)
	_, err = d.EnrollConsumer(ctx, d.AuthorityID(), "reader")
	require.NoError(t, err)
	sub, err := d.Subscribe(ctx, repo, SubscribeInput{AuthorityID: d.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{topic.ID}, FromBeginning: true})
	require.NoError(t, err)
	page, err := d.Pull(ctx, repo, "reader", sub.ID, 10)
	require.NoError(t, err)
	_, err = d.Ack(ctx, repo, "reader", sub.ID, d.AuthorityID(), page.PriorAcknowledged, page.NextScanPosition, page.AckToken)
	require.NoError(t, err)
	_, err = d.sql.ExecContext(ctx, "PRAGMA foreign_keys=OFF")
	require.NoError(t, err)
	_, err = d.sql.ExecContext(ctx, `
DROP TABLE topic_leases;
DROP INDEX repo_scopes_one_global;
CREATE TABLE repo_scopes_v6 (scope_id TEXT PRIMARY KEY, kind TEXT NOT NULL CHECK (kind='git'), enrolled_at INTEGER NOT NULL, low_water_sequence INTEGER NOT NULL DEFAULT 0 CHECK (low_water_sequence>=0));
INSERT INTO repo_scopes_v6 SELECT scope_id,kind,enrolled_at,low_water_sequence FROM repo_scopes;
DROP TABLE repo_scopes;
ALTER TABLE repo_scopes_v6 RENAME TO repo_scopes;
CREATE TABLE hub_meta_v6 (singleton INTEGER PRIMARY KEY CHECK (singleton=1), db_id TEXT NOT NULL, format_version INTEGER NOT NULL CHECK (format_version=6), retention_epoch INTEGER NOT NULL DEFAULT 0 CHECK (retention_epoch>=0));
INSERT INTO hub_meta_v6 SELECT singleton,db_id,6,retention_epoch FROM hub_meta;
DROP TABLE hub_meta;
ALTER TABLE hub_meta_v6 RENAME TO hub_meta;
DELETE FROM schema_migrations WHERE version>=7;
PRAGMA user_version=6;`)
	require.NoError(t, err)
	_, err = d.sql.ExecContext(ctx, "PRAGMA foreign_keys=ON")
	require.NoError(t, err)
	require.NoError(t, d.Close())

	// Act
	reopened, err := Open(ctx, Options{Home: home})
	require.NoError(t, err)
	defer reopened.Close()

	// Assert
	topics, err := reopened.ListTopics(ctx, repo, TopicList{})
	require.NoError(t, err)
	require.Len(t, topics, 1)
	assert.Equal(t, topic.ID, topics[0].ID)
	assert.Equal(t, "git", topics[0].ScopeKind)
	var count int
	require.NoError(t, reopened.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM publications WHERE entry_id=?", message.EntryID).Scan(&count))
	assert.Equal(t, 1, count)
	subs, err := reopened.ListSubscriptions(ctx, repo, "reader", false)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, sub.ID, subs[0].ID)
	assert.Equal(t, page.NextScanPosition, subs[0].AcknowledgedSequence)
	var enabled int
	require.NoError(t, reopened.sql.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled))
	assert.Equal(t, 1, enabled)
	rows, err := reopened.sql.QueryContext(ctx, "PRAGMA foreign_key_check")
	require.NoError(t, err)
	defer rows.Close()
	assert.False(t, rows.Next())
	require.NoError(t, rows.Err())
}
