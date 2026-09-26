package hubstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type messageFixture struct {
	d     *DB
	repo  string
	topic Topic
	now   *time.Time
}

func newMessageFixture(t *testing.T) messageFixture {
	t.Helper()
	ctx := context.Background()
	repo := testRepo(t)
	instant := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	d := testDB(t, filepath.Join(t.TempDir(), "home"), func() time.Time { return instant })
	_, err := d.EnrollRepo(ctx, repo)
	require.NoError(t, err)
	_, err = d.EnrollActor(ctx, "author")
	require.NoError(t, err)
	_, err = d.EnrollActor(ctx, "owner")
	require.NoError(t, err)
	topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "discussion", Name: "Discussion", Description: "Release discussion"})
	require.NoError(t, err)
	return messageFixture{d, repo, topic, &instant}
}

func (f messageFixture) post(t *testing.T, text, key string, expiry *time.Time) Message {
	t.Helper()
	m, err := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", MessageInput{AuthorityID: f.d.AuthorityID(), Text: text, IdempotencyKey: key, ExpiresAt: expiry})
	require.NoError(t, err)
	return m
}

func TestPostMessageStoresAttributedEnvelope(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	in := MessageInput{AuthorityID: f.d.AuthorityID(), Text: `{"status":"claimed"}`, IdempotencyKey: "post-1", SourceRefs: []SourceRef{{Kind: "document", Reference: "docs/release.md"}}}
	// Act
	m, err := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", in)
	// Assert
	require.NoError(t, err)
	assert.Equal(t, "message", m.Kind)
	assert.Equal(t, "author", m.ActorID)
	assert.Equal(t, int64(1), m.Sequence)
	assert.Equal(t, int64(1), m.Revision)
	assert.Equal(t, in.Text, m.Text)
	require.Len(t, m.SourceRefs, 1)
	assert.Equal(t, "document", m.SourceRefs[0].Kind)
	assert.NotEmpty(t, m.EntryID)
	assert.NotEmpty(t, m.MessageID)
}

func TestPostMessageRetryReturnsOriginalAfterArchive(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	original := f.post(t, "Original", "retry-1", nil)
	_, err := f.d.ArchiveTopic(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "Finished")
	require.NoError(t, err)
	// Act
	replay, err := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Original", IdempotencyKey: "retry-1"})
	// Assert
	require.NoError(t, err)
	assert.True(t, replay.Replayed)
	assert.Equal(t, original.EntryID, replay.EntryID)
	assert.Equal(t, original.Sequence, replay.Sequence)
}

func TestPostMessageChangedKeyContentConflicts(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	f.post(t, "Original", "retry-1", nil)
	// Act
	_, err := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Changed", IdempotencyKey: "retry-1"})
	// Assert
	assert.ErrorIs(t, err, ErrIdempotencyConflict)
}

func TestPostMessageRetryCanonicalizesTimestampAndEmptyRefs(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	deadline := f.now.Add(time.Hour).Add(750 * time.Microsecond)
	original, err := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Same", ExpiresAt: &deadline, IdempotencyKey: "retry"})
	require.NoError(t, err)
	equivalent := deadline.Truncate(time.Millisecond).In(time.FixedZone("offset", 2*60*60))
	// Act
	replay, err := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Same", ExpiresAt: &equivalent, SourceRefs: []SourceRef{}, CorrelationRefs: []string{}, IdempotencyKey: "retry"})
	// Assert
	require.NoError(t, err)
	assert.True(t, replay.Replayed)
	assert.Equal(t, original.EntryID, replay.EntryID)
}

func TestReviseMessageAppendsImmutableHistory(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	first := f.post(t, "Original", "post-1", nil)
	// Act
	revised, err := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Correction", IdempotencyKey: "revise-1"}, ExpectedRevision: 1})
	// Assert
	require.NoError(t, err)
	assert.Equal(t, first.MessageID, revised.MessageID)
	assert.Equal(t, int64(2), revised.Revision)
	assert.Greater(t, revised.Sequence, first.Sequence)
	history, err := f.d.MessageHistory(context.Background(), f.repo, f.topic.ID, first.MessageID, MessageList{})
	require.NoError(t, err)
	require.Len(t, history, 2)
	assert.Equal(t, "Original", history[0].Text)
	assert.Equal(t, "Correction", history[1].Text)
}

func TestRevisionRetryReturnsOriginalAfterLaterRevision(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	first := f.post(t, "First", "post", nil)
	second, err := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Second", IdempotencyKey: "revision-two"}, ExpectedRevision: 1})
	require.NoError(t, err)
	_, err = f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Third", IdempotencyKey: "revision-three"}, ExpectedRevision: 2})
	require.NoError(t, err)
	// Act
	replay, err := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Second", IdempotencyKey: "revision-two"}, ExpectedRevision: 1})
	// Assert
	require.NoError(t, err)
	assert.True(t, replay.Replayed)
	assert.Equal(t, second.EntryID, replay.EntryID)
	assert.Equal(t, second.Sequence, replay.Sequence)
}

func TestExpiredMessageRemainsHistoricalAfterTopicRestore(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	deadline := f.now.Add(time.Hour)
	m := f.post(t, "Deadline", "post-1", &deadline)
	archived, err := f.d.ArchiveTopic(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "Pause")
	require.NoError(t, err)
	*f.now = deadline
	_, err = f.d.RestoreTopic(context.Background(), f.repo, f.topic.ID, "owner", archived.PolicyGeneration, "Resume", false, nil)
	require.NoError(t, err)
	// Act
	listed, err := f.d.ListMessages(context.Background(), f.repo, f.topic.ID, MessageList{})
	// Assert
	require.NoError(t, err)
	require.Len(t, listed, 0)
	historical, err := f.d.ReadPublication(context.Background(), f.repo, m.EntryID, true)
	require.NoError(t, err)
	assert.Equal(t, m.EntryID, historical.EntryID)
}

func TestPinOrdersLiveMessagesWithoutChangingSequence(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	first := f.post(t, "First", "one", nil)
	second := f.post(t, "Second", "two", nil)
	// Act
	pinned, err := f.d.PinMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "owner", f.d.AuthorityID(), f.topic.PolicyGeneration, first.Revision, true)
	// Assert
	require.NoError(t, err)
	assert.True(t, pinned.Pinned)
	listed, err := f.d.ListMessages(context.Background(), f.repo, f.topic.ID, MessageList{})
	require.NoError(t, err)
	require.Len(t, listed, 2)
	assert.Equal(t, first.MessageID, listed[0].MessageID)
	assert.Equal(t, second.MessageID, listed[1].MessageID)
	assert.Equal(t, int64(1), listed[0].Sequence)
}

func TestPinRejectsExpiredMessageAtDeadline(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	deadline := f.now.Add(time.Minute)
	m := f.post(t, "Short lived", "one", &deadline)
	*f.now = deadline
	// Act
	_, err := f.d.PinMessage(context.Background(), f.repo, f.topic.ID, m.MessageID, "owner", f.d.AuthorityID(), f.topic.PolicyGeneration, m.Revision, true)
	// Assert
	assert.ErrorIs(t, err, ErrExpired)
}

func TestRevisionClearsCurrentPin(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	first := f.post(t, "Original", "post", nil)
	_, err := f.d.PinMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "owner", f.d.AuthorityID(), f.topic.PolicyGeneration, first.Revision, true)
	require.NoError(t, err)
	// Act
	revised, err := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Updated"}, ExpectedRevision: 1})
	// Assert
	require.NoError(t, err)
	assert.False(t, revised.Pinned)
	changes, err := f.d.PinHistory(context.Background(), f.repo, f.topic.ID, first.MessageID, MessageList{})
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, "owner", changes[0].ActorID)
	assert.True(t, changes[0].Pinned)
}

func TestPinRejectsStaleRevision(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	first := f.post(t, "Original", "post", nil)
	_, err := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "New"}, ExpectedRevision: 1})
	require.NoError(t, err)
	// Act
	_, err = f.d.PinMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "owner", f.d.AuthorityID(), f.topic.PolicyGeneration, first.Revision, true)
	// Assert
	assert.ErrorIs(t, err, ErrConflict)
}

func TestSourceReferenceResolvesAfterExpiry(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	deadline := f.now.Add(time.Minute)
	evidence := f.post(t, "Evidence", "evidence", &deadline)
	*f.now = deadline
	// Act
	statement, err := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", MessageInput{AuthorityID: f.d.AuthorityID(), Text: "See prior entry", SourceRefs: []SourceRef{{Kind: "hub_entry", ScopeID: evidence.ScopeID, Reference: evidence.EntryID}}})
	// Assert
	require.NoError(t, err)
	require.Len(t, statement.SourceRefs, 1)
	assert.Equal(t, evidence.EntryID, statement.SourceRefs[0].Reference)
	retained, err := f.d.ReadPublication(context.Background(), f.repo, evidence.EntryID, true)
	require.NoError(t, err)
	assert.Equal(t, evidence.EntryID, retained.EntryID)
}

func TestRevisionDoesNotRewriteEarlierDeliveryExpiry(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	firstDeadline := f.now.Add(time.Minute)
	secondDeadline := f.now.Add(2 * time.Hour)
	first := f.post(t, "Original", "post", &firstDeadline)
	// Act
	revised, err := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Extended", ExpiresAt: &secondDeadline}, ExpectedRevision: 1, ChangeExpiry: true})
	// Assert
	require.NoError(t, err)
	require.NotNil(t, revised.ExpiresAt)
	assert.Equal(t, secondDeadline, *revised.ExpiresAt)
	history, err := f.d.MessageHistory(context.Background(), f.repo, f.topic.ID, first.MessageID, MessageList{})
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.NotNil(t, history[0].ExpiresAt)
	require.NotNil(t, history[1].ExpiresAt)
	assert.Equal(t, firstDeadline, *history[0].ExpiresAt)
	assert.Equal(t, secondDeadline, *history[1].ExpiresAt)
}

func TestTopicExpiryBlocksMessagePost(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	deadline := f.now.Add(time.Minute)
	_, err := f.d.EditTopic(context.Background(), f.repo, f.topic.ID, "owner", TopicEdit{ChangeExpiry: true, ExpiresAt: &deadline, ExpectedGeneration: f.topic.PolicyGeneration})
	require.NoError(t, err)
	*f.now = deadline
	// Act
	_, err = f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Too late"})
	// Assert
	assert.ErrorIs(t, err, ErrArchived)
}

func TestOpenMigratesVerifiedV1Authority(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	require.NoError(t, os.MkdirAll(home, 0o700))
	raw, err := sql.Open("sqlite", filepath.Join(home, "hub.sqlite"))
	require.NoError(t, err)
	_, err = raw.Exec(schemaV1)
	require.NoError(t, err)
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV1)))
	_, err = raw.Exec("INSERT INTO hub_meta (singleton,db_id,format_version) VALUES (1,'old-authority',1)")
	require.NoError(t, err)
	_, err = raw.Exec("INSERT INTO schema_migrations VALUES (1,?,0)", digest)
	require.NoError(t, err)
	_, err = raw.Exec("INSERT INTO actors VALUES ('owner',0)")
	require.NoError(t, err)
	_, err = raw.Exec("INSERT INTO repo_scopes (scope_id,kind,enrolled_at) VALUES ('scope','git',0)")
	require.NoError(t, err)
	_, err = raw.Exec("INSERT INTO topics (topic_id,scope_id,key,name,description,owner_actor_id,created_at,policy_generation) VALUES ('topic','scope','original','Original','Retained','owner',0,1)")
	require.NoError(t, err)
	_, err = raw.Exec(fmt.Sprintf("PRAGMA application_id=%d", applicationID))
	require.NoError(t, err)
	_, err = raw.Exec("PRAGMA user_version=1")
	require.NoError(t, err)
	require.NoError(t, raw.Close())
	// Act
	d, err := Open(context.Background(), Options{Home: home})
	// Assert
	require.NoError(t, err)
	defer d.Close()
	assert.Equal(t, "old-authority", d.AuthorityID())
	var version, count int
	require.NoError(t, d.sql.QueryRow("PRAGMA user_version").Scan(&version))
	require.NoError(t, d.sql.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count))
	assert.Equal(t, 2, version)
	assert.Equal(t, 2, count)
	var topicName string
	var next int64
	require.NoError(t, d.sql.QueryRow("SELECT name FROM topics WHERE topic_id='topic'").Scan(&topicName))
	require.NoError(t, d.sql.QueryRow("SELECT next_sequence FROM scope_counters WHERE scope_id='scope'").Scan(&next))
	assert.Equal(t, "Original", topicName)
	assert.Equal(t, int64(1), next)
}

func TestIndependentProcessesRetrySameMessage(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	home := filepath.Dir(f.d.Path())
	child := func() *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHubProcessPostMessage$")
		cmd.Env = append(os.Environ(), "HUB_MESSAGE_HOME="+home, "HUB_MESSAGE_REPO="+f.repo, "HUB_MESSAGE_TOPIC="+f.topic.ID, "HUB_MESSAGE_AUTHORITY="+f.d.AuthorityID())
		return cmd
	}
	first, second := child(), child()
	// Act
	require.NoError(t, first.Start())
	require.NoError(t, second.Start())
	err1, err2 := first.Wait(), second.Wait()
	// Assert
	require.NoError(t, err1)
	require.NoError(t, err2)
	page, err := f.d.ScanPublications(context.Background(), f.repo, 0, 100, true)
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, int64(1), page.Entries[0].Sequence)
}

func TestIndependentProcessesPreserveDistinctMessages(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	home := filepath.Dir(f.d.Path())
	first := exec.Command(os.Args[0], "-test.run=^TestHubProcessPostMessage$")
	first.Env = append(os.Environ(), "HUB_MESSAGE_HOME="+home, "HUB_MESSAGE_REPO="+f.repo, "HUB_MESSAGE_TOPIC="+f.topic.ID, "HUB_MESSAGE_AUTHORITY="+f.d.AuthorityID(), "HUB_MESSAGE_KEY=first")
	second := exec.Command(os.Args[0], "-test.run=^TestHubProcessPostMessage$")
	second.Env = append(os.Environ(), "HUB_MESSAGE_HOME="+home, "HUB_MESSAGE_REPO="+f.repo, "HUB_MESSAGE_TOPIC="+f.topic.ID, "HUB_MESSAGE_AUTHORITY="+f.d.AuthorityID(), "HUB_MESSAGE_KEY=second")
	// Act
	require.NoError(t, first.Start())
	require.NoError(t, second.Start())
	err1, err2 := first.Wait(), second.Wait()
	// Assert
	require.NoError(t, err1)
	require.NoError(t, err2)
	page, err := f.d.ScanPublications(context.Background(), f.repo, 0, 100, true)
	require.NoError(t, err)
	require.Len(t, page.Entries, 2)
	assert.Equal(t, int64(1), page.Entries[0].Sequence)
	assert.Equal(t, int64(2), page.Entries[1].Sequence)
}

func TestScanPublicationsReportsExpiredGap(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	deadline := f.now.Add(time.Minute)
	f.post(t, "Short", "short", &deadline)
	f.post(t, "Long", "long", nil)
	*f.now = deadline
	// Act
	page, err := f.d.ScanPublications(context.Background(), f.repo, 0, 100, false)
	// Assert
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, int64(2), page.Entries[0].Sequence)
	require.Len(t, page.Gaps, 1)
	assert.Equal(t, "expired", page.Gaps[0].Reason)
	assert.Equal(t, int64(1), page.Gaps[0].From)
	assert.Equal(t, int64(2), page.NextScanPosition)
}

func TestPublicationSequencesAreScopeLocal(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	f.post(t, "First scope", "first", nil)
	otherRepo := testRepo(t)
	_, err := f.d.EnrollRepo(context.Background(), otherRepo)
	require.NoError(t, err)
	otherTopic, err := f.d.CreateTopic(context.Background(), otherRepo, "owner", TopicInput{Key: "discussion", Name: "Other", Description: "Other scope"})
	require.NoError(t, err)
	// Act
	other, err := f.d.PostMessage(context.Background(), otherRepo, otherTopic.ID, "author", MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Second scope"})
	// Assert
	require.NoError(t, err)
	assert.NotEqual(t, f.topic.ScopeID, other.ScopeID)
	assert.Equal(t, int64(1), other.Sequence)
}

func TestNoExpiryRevisionRejectsForeignTopicPublication(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	first := f.post(t, "Original", "first", nil)
	otherTopic, err := f.d.CreateTopic(context.Background(), f.repo, "owner", TopicInput{Key: "other", Name: "Other", Description: "Another topic"})
	require.NoError(t, err)
	_, err = f.d.sql.Exec("INSERT INTO publications (entry_id,scope_id,topic_id,actor_id,kind,sequence,committed_at,source_refs,correlation_refs) VALUES ('foreign-entry',?,?,?,'message',2,0,'[]','[]')", first.ScopeID, otherTopic.ID, "author")
	require.NoError(t, err)
	// Act
	_, err = f.d.sql.Exec("INSERT INTO message_revisions (message_id,scope_id,topic_id,revision,entry_id,text,expires_at) VALUES (?,?,?,2,'foreign-entry','illegal',NULL)", first.MessageID, first.ScopeID, first.TopicID)
	// Assert
	require.Error(t, err)
	var count int
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM message_revisions WHERE entry_id='foreign-entry'").Scan(&count))
	assert.Zero(t, count)
}

func TestHubProcessPostMessage(t *testing.T) {
	home := os.Getenv("HUB_MESSAGE_HOME")
	if home == "" {
		t.Skip("helper")
	}
	d, err := Open(context.Background(), Options{Home: home})
	require.NoError(t, err)
	defer d.Close()
	key := os.Getenv("HUB_MESSAGE_KEY")
	if key == "" {
		key = "shared-key"
	}
	_, err = d.PostMessage(context.Background(), os.Getenv("HUB_MESSAGE_REPO"), os.Getenv("HUB_MESSAGE_TOPIC"), "author", MessageInput{AuthorityID: os.Getenv("HUB_MESSAGE_AUTHORITY"), Text: "Concurrent", IdempotencyKey: key})
	require.NoError(t, err, strings.TrimSpace(home))
}
