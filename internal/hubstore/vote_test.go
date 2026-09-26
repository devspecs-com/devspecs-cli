package hubstore

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func voteForTest(t *testing.T, f messageFixture, message Message, actor string, value int) Message {
	t.Helper()
	result, err := f.d.VoteMessage(context.Background(), f.repo, f.topic.ID, message.MessageID, actor, f.d.AuthorityID(), message.Revision, value)
	require.NoError(t, err)
	return result
}

func TestVoteMessageRetryDoesNotDuplicate(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	m := f.post(t, "one", "", nil)
	voteForTest(t, f, m, "author", 1)

	// Act
	retry := voteForTest(t, f, m, "author", 1)
	var rows int
	err := f.d.sql.QueryRow("SELECT COUNT(*) FROM message_votes").Scan(&rows)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(1), retry.Upvotes)
	assert.Equal(t, int64(0), retry.Downvotes)
	assert.Equal(t, 1, rows)
}

func TestVoteMessageChangeReplacesExisting(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	m := f.post(t, "one", "", nil)
	voteForTest(t, f, m, "author", 1)

	// Act
	down := voteForTest(t, f, m, "author", -1)
	var rows int
	err := f.d.sql.QueryRow("SELECT COUNT(*) FROM message_votes").Scan(&rows)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(0), down.Upvotes)
	assert.Equal(t, int64(1), down.Downvotes)
	assert.Equal(t, int64(-1), down.Score)
	assert.Equal(t, 1, rows)
}

func TestVoteMessageClearRemovesExisting(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	m := f.post(t, "one", "", nil)
	voteForTest(t, f, m, "author", 1)

	// Act
	clear := voteForTest(t, f, m, "author", 0)
	var rows int
	err := f.d.sql.QueryRow("SELECT COUNT(*) FROM message_votes").Scan(&rows)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(0), clear.Score)
	assert.Zero(t, rows)
}

func TestVoteMessageActorReopenDoesNotDuplicateVote(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	m := f.post(t, "one", "", nil)
	voteForTest(t, f, m, "owner", 1)
	reopened, err := Open(context.Background(), Options{Home: filepath.Dir(f.d.Path()), Now: func() time.Time { return *f.now }})
	require.NoError(t, err)
	defer reopened.Close()

	// Act
	retry, err := reopened.VoteMessage(context.Background(), f.repo, f.topic.ID, m.MessageID, "owner", reopened.AuthorityID(), 1, 1)
	var count int
	countErr := f.d.sql.QueryRow("SELECT COUNT(*) FROM message_votes WHERE message_id=?", m.MessageID).Scan(&count)

	// Assert
	require.NoError(t, err)
	require.NoError(t, countErr)
	assert.Equal(t, int64(1), retry.Upvotes)
	assert.Equal(t, int64(0), retry.Downvotes)
	assert.Equal(t, 1, count)
}

func TestVoteMessageRevisionKeepsHistoricalCounts(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	old := f.post(t, "old", "", nil)
	voteForTest(t, f, old, "owner", 1)
	updated, err := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, old.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "new"}, ExpectedRevision: 1})
	require.NoError(t, err)

	// Act
	_, staleErr := f.d.VoteMessage(context.Background(), f.repo, f.topic.ID, old.MessageID, "owner", f.d.AuthorityID(), 1, -1)
	current := voteForTest(t, f, updated, "owner", -1)
	history, historyErr := f.d.MessageHistory(context.Background(), f.repo, f.topic.ID, old.MessageID, MessageList{})

	// Assert
	assert.ErrorIs(t, staleErr, ErrConflict)
	assert.Equal(t, int64(0), current.Upvotes)
	assert.Equal(t, int64(1), current.Downvotes)
	require.NoError(t, historyErr)
	require.Len(t, history, 2)
	assert.Equal(t, int64(1), history[0].Upvotes)
	assert.Equal(t, int64(0), history[0].Downvotes)
	assert.Equal(t, int64(0), history[1].Upvotes)
	assert.Equal(t, int64(1), history[1].Downvotes)
}

func TestConcurrentActorsVoteWithoutLostUpdates(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	m := f.post(t, "shared", "", nil)
	_, err := f.d.EnrollActor(context.Background(), "agent-a")
	require.NoError(t, err)
	_, err = f.d.EnrollActor(context.Background(), "agent-b")
	require.NoError(t, err)
	home := filepath.Dir(f.d.Path())
	first, err := Open(context.Background(), Options{Home: home, Now: func() time.Time { return *f.now }})
	require.NoError(t, err)
	defer first.Close()
	second, err := Open(context.Background(), Options{Home: home, Now: func() time.Time { return *f.now }})
	require.NoError(t, err)
	defer second.Close()
	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	go func() {
		ready.Done()
		<-start
		_, voteErr := first.VoteMessage(context.Background(), f.repo, f.topic.ID, m.MessageID, "agent-a", first.AuthorityID(), 1, 1)
		results <- voteErr
	}()
	go func() {
		ready.Done()
		<-start
		_, voteErr := second.VoteMessage(context.Background(), f.repo, f.topic.ID, m.MessageID, "agent-b", second.AuthorityID(), 1, -1)
		results <- voteErr
	}()

	// Act
	ready.Wait()
	close(start)
	firstErr := <-results
	secondErr := <-results
	read, err := f.d.ReadMessage(context.Background(), f.repo, f.topic.ID, m.MessageID, false)

	// Assert
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	require.NoError(t, err)
	assert.Equal(t, int64(1), read.Upvotes)
	assert.Equal(t, int64(1), read.Downvotes)
	assert.Equal(t, int64(0), read.Score)
}

func TestRankedDiscoveryPinsScoresTiesAndExpiry(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	_, err := f.d.EnrollActor(context.Background(), "other")
	require.NoError(t, err)
	pinned := f.post(t, "pinned conflict", "", nil)
	older := f.post(t, "older tie", "", nil)
	newer := f.post(t, "newer tie", "", nil)
	unvoted := f.post(t, "newest unvoted", "", nil)
	deadline := f.now.Add(time.Minute)
	expiring := f.post(t, "expired high score", "", &deadline)
	voteForTest(t, f, pinned, "author", 1)
	voteForTest(t, f, pinned, "other", -1)
	voteForTest(t, f, older, "author", 1)
	voteForTest(t, f, newer, "author", 1)
	voteForTest(t, f, expiring, "author", 1)
	voteForTest(t, f, expiring, "other", 1)
	_, err = f.d.PinMessage(context.Background(), f.repo, f.topic.ID, pinned.MessageID, "owner", f.d.AuthorityID(), f.topic.PolicyGeneration, 1, true)
	require.NoError(t, err)
	*f.now = deadline

	// Act
	ranked, rankedErr := f.d.ListMessages(context.Background(), f.repo, f.topic.ID, MessageList{Ranked: true})
	chronological, chronologicalErr := f.d.ListMessages(context.Background(), f.repo, f.topic.ID, MessageList{})

	// Assert
	require.NoError(t, rankedErr)
	require.NoError(t, chronologicalErr)
	require.Len(t, ranked, 4)
	assert.Equal(t, pinned.MessageID, ranked[0].MessageID)
	assert.Equal(t, newer.MessageID, ranked[1].MessageID)
	assert.Equal(t, older.MessageID, ranked[2].MessageID)
	assert.Equal(t, unvoted.MessageID, ranked[3].MessageID)
	assert.Equal(t, int64(0), ranked[0].Score)
	assert.Equal(t, int64(1), ranked[1].Score)
	assert.Equal(t, int64(1), ranked[2].Score)
	require.Len(t, chronological, 4)
	assert.Equal(t, pinned.MessageID, chronological[0].MessageID)
	assert.Equal(t, unvoted.MessageID, chronological[1].MessageID)
	assert.Equal(t, newer.MessageID, chronological[2].MessageID)
	assert.Equal(t, older.MessageID, chronological[3].MessageID)
}

func TestArchivedMessageVotesStayHistoricalOnly(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	m := f.post(t, "decision", "", nil)
	voteForTest(t, f, m, "author", 1)
	_, err := f.d.ArchiveTopic(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "Finished")
	require.NoError(t, err)

	// Act
	_, voteErr := f.d.VoteMessage(context.Background(), f.repo, f.topic.ID, m.MessageID, "owner", f.d.AuthorityID(), 1, 1)
	active, listErr := f.d.ListMessages(context.Background(), f.repo, f.topic.ID, MessageList{Ranked: true})
	historical, readErr := f.d.ReadMessage(context.Background(), f.repo, f.topic.ID, m.MessageID, true)

	// Assert
	assert.ErrorIs(t, voteErr, ErrArchived)
	require.NoError(t, listErr)
	assert.Empty(t, active)
	require.NoError(t, readErr)
	assert.Equal(t, int64(1), historical.Upvotes)
}

func TestDownvotedMessageRemainsReadable(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	m := f.post(t, "disputed", "", nil)
	voteForTest(t, f, m, "owner", -1)

	// Act
	read, readErr := f.d.ReadMessage(context.Background(), f.repo, f.topic.ID, m.MessageID, false)

	// Assert
	require.NoError(t, readErr)
	assert.Equal(t, m.MessageID, read.MessageID)
	assert.Equal(t, int64(-1), read.Score)
}

func TestExpiredMessageRejectsNewVotesAndKeepsHistoricalScore(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	deadline := f.now.Add(time.Minute)
	m := f.post(t, "disputed", "", &deadline)
	voteForTest(t, f, m, "owner", -1)
	*f.now = deadline

	// Act
	_, voteErr := f.d.VoteMessage(context.Background(), f.repo, f.topic.ID, m.MessageID, "author", f.d.AuthorityID(), 1, 1)
	historical, historicalErr := f.d.ReadMessage(context.Background(), f.repo, f.topic.ID, m.MessageID, true)

	// Assert
	assert.ErrorIs(t, voteErr, ErrExpired)
	require.NoError(t, historicalErr)
	assert.Equal(t, int64(-1), historical.Score)
}

func TestVotesDoNotChangePullSequenceOrAck(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	first := f.post(t, "first", "", nil)
	second := f.post(t, "second", "", nil)
	voteForTest(t, f, first, "owner", 1)

	// Act
	page, err := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 2)
	require.NoError(t, err)
	_, ackErr := f.d.Ack(context.Background(), f.repo, "reader", s.ID, f.d.AuthorityID(), 0, page.NextScanPosition, page.AckToken)

	// Assert
	require.Len(t, page.Entries, 2)
	assert.Equal(t, first.EntryID, page.Entries[0].Message.EntryID)
	assert.Equal(t, second.EntryID, page.Entries[1].Message.EntryID)
	assert.Equal(t, int64(2), page.NextScanPosition)
	assert.NoError(t, ackErr)
}

func TestPruneRevisionRemovesVotesWithoutForeignKeyViolations(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	old := f.post(t, "old", "", nil)
	voteForTest(t, f, old, "owner", 1)
	*f.now = f.now.Add(2 * time.Minute)
	_, err := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, old.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "current"}, ExpectedRevision: 1})
	require.NoError(t, err)

	// Act
	_, err = f.d.PruneHub(context.Background(), f.now.Add(-time.Minute))
	var count int
	countErr := f.d.sql.QueryRow("SELECT COUNT(*) FROM message_votes WHERE message_id=?", old.MessageID).Scan(&count)
	integrityErr := f.d.CheckIntegrity(context.Background())

	// Assert
	require.NoError(t, err)
	require.NoError(t, countErr)
	assert.Zero(t, count)
	assert.NoError(t, integrityErr)
}

func TestOpenMigratesV5AuthorityToVotes(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	d, err := Open(context.Background(), Options{Home: home})
	require.NoError(t, err)
	id := d.AuthorityID()
	tx, err := d.sql.Begin()
	require.NoError(t, err)
	_, err = tx.Exec(`
DROP TABLE message_votes;
CREATE TABLE hub_meta_v5 (singleton INTEGER PRIMARY KEY CHECK (singleton=1), db_id TEXT NOT NULL, format_version INTEGER NOT NULL CHECK (format_version=5), retention_epoch INTEGER NOT NULL DEFAULT 0 CHECK (retention_epoch>=0));
INSERT INTO hub_meta_v5 SELECT singleton,db_id,5,retention_epoch FROM hub_meta;
DROP TABLE hub_meta;
ALTER TABLE hub_meta_v5 RENAME TO hub_meta;
DELETE FROM schema_migrations WHERE version=6;
PRAGMA user_version=5;`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, d.Close())

	// Act
	migrated, err := Open(context.Background(), Options{Home: home})
	require.NoError(t, err)
	defer migrated.Close()
	var version, votesTable int
	versionErr := migrated.sql.QueryRow("PRAGMA user_version").Scan(&version)
	tableErr := migrated.sql.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='message_votes'").Scan(&votesTable)

	// Assert
	require.NoError(t, versionErr)
	require.NoError(t, tableErr)
	assert.Equal(t, id, migrated.AuthorityID())
	assert.Equal(t, 6, version)
	assert.Equal(t, 1, votesTable)
	assert.NoError(t, migrated.CheckIntegrity(context.Background()))
}
