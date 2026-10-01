package hubstore

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func leaseFixture(t *testing.T, now func() time.Time) (*DB, string, Topic) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	db := testDB(t, home, now)
	ctx := context.Background()
	_, err := db.EnrollActor(ctx, "agent-a")
	require.NoError(t, err)
	_, err = db.EnrollActor(ctx, "agent-b")
	require.NoError(t, err)
	_, err = db.EnrollRepo(ctx, GlobalScopeSelector)
	require.NoError(t, err)
	topic, err := db.CreateTopic(ctx, GlobalScopeSelector, "agent-a", TopicInput{Key: "local-go", Name: "Local Go", Description: "Heavy Go jobs"})
	require.NoError(t, err)
	return db, home, topic
}

func TestAcquireLease_HeldTopicRejectsSecondActor(t *testing.T) {
	// Arrange
	db, _, topic := leaseFixture(t, nil)
	first, err := db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", time.Minute)
	require.NoError(t, err)

	// Act
	_, err = db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-b", time.Minute)

	// Assert
	assert.ErrorIs(t, err, ErrConflict)
	assert.ErrorContains(t, err, "held by agent-a")
	assert.NotEmpty(t, first.Token)
	assert.Equal(t, int64(1), first.Generation)
}

func TestAcquireLease_IndependentHandlesAdmitOneContender(t *testing.T) {
	// Arrange
	db, home, topic := leaseFixture(t, nil)
	other := testDB(t, home, nil)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var first, second error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, first = db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", time.Minute)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, second = other.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-b", time.Minute)
	}()

	// Act
	close(start)
	wg.Wait()

	// Assert
	assert.True(t, (first == nil && errors.Is(second, ErrConflict)) || (second == nil && errors.Is(first, ErrConflict)), "first=%v second=%v", first, second)
}

func TestAcquireLease_ExpiredHolderCanBeReplacedWithoutOldTokenRelease(t *testing.T) {
	// Arrange
	instant := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	db, _, topic := leaseFixture(t, func() time.Time { return instant })
	first, err := db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", time.Second)
	require.NoError(t, err)
	instant = instant.Add(2 * time.Second)

	// Act
	second, err := db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-b", time.Minute)
	require.NoError(t, err)
	staleErr := db.ReleaseLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", first.Token)

	// Assert
	assert.Equal(t, int64(2), second.Generation)
	assert.NotEqual(t, first.Token, second.Token)
	assert.ErrorIs(t, staleErr, ErrConflict)
	current, err := db.ShowLease(context.Background(), GlobalScopeSelector, topic.ID)
	require.NoError(t, err)
	assert.Equal(t, "agent-b", current.ActorID)
	assert.Empty(t, current.Token)
}

func TestAcquireLease_StaleTokenCannotReleaseSameActorSuccessor(t *testing.T) {
	// Arrange
	db, _, topic := leaseFixture(t, nil)
	first, err := db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", time.Minute)
	require.NoError(t, err)
	require.NoError(t, db.ReleaseLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", first.Token))
	second, err := db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", time.Minute)
	require.NoError(t, err)

	// Act
	staleErr := db.ReleaseLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", first.Token)

	// Assert
	assert.ErrorIs(t, staleErr, ErrConflict)
	assert.Equal(t, first.Generation+1, second.Generation)
	current, err := db.ShowLease(context.Background(), GlobalScopeSelector, topic.ID)
	require.NoError(t, err)
	assert.Equal(t, "held", current.State)
}

func TestRenewLease_StaleTokenCannotExtendSameActorSuccessor(t *testing.T) {
	// Arrange
	db, _, topic := leaseFixture(t, nil)
	first, err := db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", time.Minute)
	require.NoError(t, err)
	require.NoError(t, db.ReleaseLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", first.Token))
	second, err := db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", time.Minute)
	require.NoError(t, err)

	// Act
	_, staleErr := db.RenewLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", first.Token, time.Hour)

	// Assert
	assert.ErrorIs(t, staleErr, ErrConflict)
	current, err := db.ShowLease(context.Background(), GlobalScopeSelector, topic.ID)
	require.NoError(t, err)
	assert.Equal(t, second.ExpiresAt, current.ExpiresAt)
}

func TestRenewLease_ValidTokenExtendsDeadline(t *testing.T) {
	// Arrange
	instant := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	db, _, topic := leaseFixture(t, func() time.Time { return instant })
	first, err := db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", time.Minute)
	require.NoError(t, err)
	instant = instant.Add(30 * time.Second)

	// Act
	renewed, err := db.RenewLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", first.Token, 2*time.Minute)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, first.Generation, renewed.Generation)
	require.NotNil(t, renewed.ExpiresAt)
	assert.Equal(t, instant.Add(2*time.Minute), *renewed.ExpiresAt)
	assert.Empty(t, renewed.Token)
}

func TestRenewLease_ExpiredTokenFails(t *testing.T) {
	// Arrange
	instant := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	db, _, topic := leaseFixture(t, func() time.Time { return instant })
	first, err := db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", time.Second)
	require.NoError(t, err)
	instant = instant.Add(2 * time.Second)

	// Act
	_, err = db.RenewLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", first.Token, time.Minute)

	// Assert
	assert.ErrorIs(t, err, ErrExpired)
}

func TestReleaseLease_CurrentTokenMakesTopicAvailable(t *testing.T) {
	// Arrange
	db, _, topic := leaseFixture(t, nil)
	first, err := db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", time.Minute)
	require.NoError(t, err)

	// Act
	err = db.ReleaseLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", first.Token)

	// Assert
	require.NoError(t, err)
	current, err := db.ShowLease(context.Background(), GlobalScopeSelector, topic.ID)
	require.NoError(t, err)
	assert.Equal(t, "available", current.State)
	assert.Equal(t, first.Generation, current.Generation)
}

func TestShowLease_AvailableJSONOmitsTimestamps(t *testing.T) {
	// Arrange
	db, _, topic := leaseFixture(t, nil)

	// Act
	lease, err := db.ShowLease(context.Background(), GlobalScopeSelector, topic.ID)
	require.NoError(t, err)
	encoded, marshalErr := json.Marshal(lease)

	// Assert
	require.NoError(t, marshalErr)
	assert.Equal(t, "available", lease.State)
	assert.NotContains(t, string(encoded), "acquired_at")
	assert.NotContains(t, string(encoded), "expires_at")
}

func TestAcquireLease_ArchivedTopicRejectsAdmission(t *testing.T) {
	// Arrange
	db, _, topic := leaseFixture(t, nil)
	_, err := db.ArchiveTopic(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", topic.PolicyGeneration, "finished")
	require.NoError(t, err)

	// Act
	_, err = db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-b", time.Minute)

	// Assert
	assert.ErrorIs(t, err, ErrArchived)
}

func TestRenewLease_ArchivedTopicRejectsExtension(t *testing.T) {
	// Arrange
	db, _, topic := leaseFixture(t, nil)
	lease, err := db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", time.Minute)
	require.NoError(t, err)
	_, err = db.ArchiveTopic(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", topic.PolicyGeneration, "finished")
	require.NoError(t, err)

	// Act
	_, err = db.RenewLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", lease.Token, time.Minute)

	// Assert
	assert.ErrorIs(t, err, ErrArchived)
}

func TestAcquireLease_RepoTopicDoesNotAdmitGlobalAddress(t *testing.T) {
	// Arrange
	db, _, _ := leaseFixture(t, nil)
	repo := testRepo(t)
	_, err := db.EnrollRepo(context.Background(), repo)
	require.NoError(t, err)
	topic, err := db.CreateTopic(context.Background(), repo, "agent-a", TopicInput{Key: "local-go", Name: "Local Go", Description: "Repo-only jobs"})
	require.NoError(t, err)

	// Act
	lease, acquireErr := db.AcquireLease(context.Background(), repo, topic.ID, "agent-a", time.Minute)
	_, wrongScopeErr := db.ShowLease(context.Background(), GlobalScopeSelector, topic.ID)

	// Assert
	require.NoError(t, acquireErr)
	assert.Equal(t, "held", lease.State)
	assert.ErrorIs(t, wrongScopeErr, ErrNotFound)
}

func TestAcquireLease_RejectsOutOfRangeLifetime(t *testing.T) {
	// Arrange
	db, _, topic := leaseFixture(t, nil)

	// Act
	_, err := db.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", 500*time.Millisecond)

	// Assert
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestOpenMigratesPopulatedV7HubToLeases(t *testing.T) {
	// Arrange
	db, home, topic := leaseFixture(t, nil)
	_, err := db.PostMessage(context.Background(), GlobalScopeSelector, topic.ID, "agent-a", MessageInput{AuthorityID: db.AuthorityID(), Text: "Keep this"})
	require.NoError(t, err)
	_, err = db.sql.Exec(`
DROP TABLE topic_leases;
CREATE TABLE hub_meta_v7 (singleton INTEGER PRIMARY KEY CHECK (singleton=1), db_id TEXT NOT NULL, format_version INTEGER NOT NULL CHECK (format_version=7), retention_epoch INTEGER NOT NULL DEFAULT 0 CHECK (retention_epoch>=0));
INSERT INTO hub_meta_v7 SELECT singleton,db_id,7,retention_epoch FROM hub_meta;
DROP TABLE hub_meta;
ALTER TABLE hub_meta_v7 RENAME TO hub_meta;
DELETE FROM schema_migrations WHERE version=8;
PRAGMA user_version=7;`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Act
	reopened, err := Open(context.Background(), Options{Home: home})
	require.NoError(t, err)
	defer reopened.Close()
	lease, leaseErr := reopened.AcquireLease(context.Background(), GlobalScopeSelector, topic.ID, "agent-b", time.Minute)
	messages, messageErr := reopened.ListMessages(context.Background(), GlobalScopeSelector, topic.ID, MessageList{})

	// Assert
	require.NoError(t, leaseErr)
	require.NoError(t, messageErr)
	assert.Equal(t, "held", lease.State)
	require.Len(t, messages, 1)
	assert.Equal(t, "Keep this", messages[0].Text)
	assert.NoError(t, reopened.CheckIntegrity(context.Background()))
}
