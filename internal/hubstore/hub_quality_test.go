package hubstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHubQualityPublicationEnvelopeCannotBeRewritten(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	m := f.post(t, "A retained decision", "decision", nil)

	// Act
	_, mutationErr := f.d.sql.Exec("UPDATE publications SET source_refs='broken' WHERE entry_id=?", m.EntryID)
	retained, readErr := f.d.ReadMessage(context.Background(), f.repo, f.topic.ID, m.MessageID, true)

	// Assert
	require.Error(t, mutationErr)
	require.NoError(t, readErr)
	assert.Equal(t, m.EntryID, retained.EntryID)
	assert.Equal(t, "A retained decision", retained.Text)
	require.Len(t, retained.SourceRefs, 0)
}

func TestHubQualityMessageRevisionCannotBeRewritten(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	m := f.post(t, "A retained decision", "decision", nil)

	// Act
	_, mutationErr := f.d.sql.Exec("UPDATE message_revisions SET text='silently changed' WHERE entry_id=?", m.EntryID)
	history, readErr := f.d.MessageHistory(context.Background(), f.repo, f.topic.ID, m.MessageID, MessageList{})

	// Assert
	require.Error(t, mutationErr)
	require.NoError(t, readErr)
	require.Len(t, history, 1)
	assert.Equal(t, m.EntryID, history[0].EntryID)
	assert.Equal(t, "A retained decision", history[0].Text)
}

func TestHubQualityPublicationSequenceCannotBeRewritten(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	m := f.post(t, "A retained decision", "decision", nil)

	// Act
	_, mutationErr := f.d.sql.Exec("UPDATE publications SET sequence=99 WHERE entry_id=?", m.EntryID)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	require.Error(t, mutationErr)
	require.NoError(t, scanErr)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, m.EntryID, page.Entries[0].EntryID)
	assert.Equal(t, int64(1), page.Entries[0].Sequence)
}

func TestHubQualityEventEnvelopeCannotBeRewritten(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	e, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3}`, "build"))
	require.NoError(t, err)

	// Act
	_, mutationErr := f.d.sql.Exec("UPDATE publications SET correlation_refs='broken' WHERE entry_id=?", e.EntryID)
	retained, readErr := f.d.ReadEvent(context.Background(), f.repo, e.EntryID, true)

	// Assert
	require.Error(t, mutationErr)
	require.NoError(t, readErr)
	assert.Equal(t, e.EntryID, retained.EntryID)
	require.Len(t, retained.CorrelationRefs, 0)
}

func TestHubQualitySchemaDigestCannotBeRewritten(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	schema := registerJob(t, f)

	// Act
	_, mutationErr := f.d.sql.Exec("UPDATE event_schemas SET schema_sha256=? WHERE topic_id=? AND type_key=? AND version=?", "bad-digest", f.topic.ID, schema.TypeKey, schema.Version)
	e, publishErr := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3}`, "build"))

	// Assert
	require.Error(t, mutationErr)
	require.NoError(t, publishErr)
	assert.Equal(t, int64(1), e.Sequence)
	assert.Equal(t, schema.SHA256, e.SchemaSHA256)
}

func TestHubQualitySchemaDefinitionCannotBeRewritten(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	schema := registerJob(t, f)

	// Act
	_, mutationErr := f.d.sql.Exec("UPDATE event_schemas SET schema_json='{}' WHERE topic_id=? AND type_key=? AND version=?", f.topic.ID, schema.TypeKey, schema.Version)
	retained, showErr := f.d.ShowEventSchema(context.Background(), f.repo, f.topic.ID, schema.TypeKey, schema.Version)

	// Assert
	require.Error(t, mutationErr)
	require.NoError(t, showErr)
	assert.JSONEq(t, string(schema.Schema), string(retained.Schema))
}

func TestHubQualitySchemaListKeepsRetiredVersionAndPaginates(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	first := registerJob(t, f)
	second, err := f.d.RegisterEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration+1, "job.finished", 2, []byte(jobSchema))
	require.NoError(t, err)
	_, err = f.d.RetireEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration+2, first.TypeKey, first.Version)
	require.NoError(t, err)

	// Act
	firstPage, firstErr := f.d.ListEventSchemas(context.Background(), f.repo, f.topic.ID, 1, 0)
	secondPage, secondErr := f.d.ListEventSchemas(context.Background(), f.repo, f.topic.ID, 1, 1)

	// Assert
	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	require.Len(t, firstPage, 1)
	require.Len(t, secondPage, 1)
	assert.Equal(t, first.Version, firstPage[0].Version)
	assert.NotNil(t, firstPage[0].RetiredAt)
	assert.Equal(t, second.Version, secondPage[0].Version)
	assert.Nil(t, secondPage[0].RetiredAt)
}

func TestHubQualitySchemaListRejectsMissingTopic(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)

	// Act
	listed, err := f.d.ListEventSchemas(context.Background(), f.repo, "missing", 10, 0)

	// Assert
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, listed)
}

func TestHubQualityCorrectionCannotCrossTopicsOrConsumeSequence(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	other, err := f.d.CreateTopic(context.Background(), f.repo, "owner", TopicInput{Key: "other", Name: "Other", Description: "Separate stream"})
	require.NoError(t, err)
	_, err = f.d.RegisterEventSchema(context.Background(), f.repo, other.ID, "owner", other.PolicyGeneration, "job.finished", 1, []byte(jobSchema))
	require.NoError(t, err)
	original, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3}`, "original"))
	require.NoError(t, err)
	in := jobInput(f, `{"job":"build","duration":4}`, "correction")
	in.CorrectsEntryID = original.EntryID

	// Act
	_, correctionErr := f.d.PublishEvent(context.Background(), f.repo, other.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, correctionErr, ErrNotFound)
	require.NoError(t, scanErr)
	assert.Equal(t, original.Sequence, page.HighWater)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, original.EntryID, page.Entries[0].EntryID)
}

func TestHubQualityCorrectionRetainsAttributionAcrossArchive(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	original, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3}`, "original"))
	require.NoError(t, err)
	in := jobInput(f, `{"job":"build","duration":4}`, "correction")
	in.CorrectsEntryID = original.EntryID
	when := f.now.Add(-time.Minute)
	in.OccurredAt = &when
	corrected, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", in)
	require.NoError(t, err)
	_, err = f.d.ArchiveTopic(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration+1, "Done")
	require.NoError(t, err)

	// Act
	_, liveErr := f.d.ReadEvent(context.Background(), f.repo, corrected.EntryID, false)
	historical, readErr := f.d.ReadEvent(context.Background(), f.repo, corrected.EntryID, true)

	// Assert
	assert.ErrorIs(t, liveErr, ErrArchived)
	require.NoError(t, readErr)
	assert.Equal(t, original.EntryID, historical.CorrectsEntryID)
	assert.Equal(t, "author", historical.ActorID)
	require.NotNil(t, historical.OccurredAt)
	assert.Equal(t, when, *historical.OccurredAt)
}

func TestHubQualityMissingSourceEntryRejectsPostWithoutSequence(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	missing := "0123456789abcdef0123456789abcdef"
	in := MessageInput{AuthorityID: f.d.AuthorityID(), Text: "See retained evidence", SourceRefs: []SourceRef{{Kind: "hub_entry", ScopeID: f.topic.ScopeID, Reference: missing}}}

	// Act
	_, postErr := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, postErr, ErrNotFound)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	require.Len(t, page.Entries, 0)
}

func TestHubQualityChangedRepositoryMarkerCanRecoverOriginalBinding(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	markerPath := filepath.Join(f.repo, ".git", markerFile)
	original, err := os.ReadFile(markerPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(markerPath, []byte("0123456789abcdef0123456789abcdef\n"), 0o600))

	// Act
	_, changedErr := f.d.LookupRepo(context.Background(), f.repo)
	repairErr := os.WriteFile(markerPath, original, 0o600)
	scope, lookupErr := f.d.LookupRepo(context.Background(), f.repo)

	// Assert
	assert.ErrorIs(t, changedErr, ErrBindingConflict)
	require.NoError(t, repairErr)
	require.NoError(t, lookupErr)
	assert.Equal(t, f.topic.ScopeID, scope.ID)
}

func TestHubQualityAutoEnrolledConsumerHasStableAttribution(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)

	// Act
	consumer, err := f.d.EnrollConsumer(context.Background(), f.d.AuthorityID(), "")
	sub, subscribeErr := f.d.Subscribe(context.Background(), f.repo, SubscribeInput{AuthorityID: f.d.AuthorityID(), ConsumerID: consumer.ID, TopicIDs: []string{f.topic.ID}, FromBeginning: true})

	// Assert
	require.NoError(t, err)
	require.NoError(t, subscribeErr)
	assert.Len(t, consumer.ID, 32)
	assert.Equal(t, consumer.ID, sub.ConsumerID)
	assert.Equal(t, *f.now, consumer.EnrolledAt)
}

func TestHubQualitySubscriptionCanonicalizesMultipleEventFilters(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	_, err := f.d.RegisterEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration+1, "job.finished", 2, []byte(jobSchema))
	require.NoError(t, err)
	_, err = f.d.RegisterEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration+2, "build.started", 1, []byte(jobSchema))
	require.NoError(t, err)
	enrollTestConsumer(t, f, "reader")

	// Act
	sub, subscribeErr := f.d.Subscribe(context.Background(), f.repo, SubscribeInput{
		AuthorityID: f.d.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{f.topic.ID},
		Kinds: []string{"message", "event"}, EventTypes: []EventTypeFilter{
			{TypeKey: "job.finished", Version: 2},
			{TypeKey: "job.finished", Version: 1},
			{TypeKey: "build.started", Version: 1},
		}, FromBeginning: true,
	})

	// Assert
	require.NoError(t, subscribeErr)
	require.Len(t, sub.EventTypes, 3)
	assert.Equal(t, "build.started", sub.EventTypes[0].TypeKey)
	assert.Equal(t, int64(1), sub.EventTypes[0].Version)
	assert.Equal(t, int64(1), sub.EventTypes[1].Version)
	assert.Equal(t, int64(2), sub.EventTypes[2].Version)
	require.Len(t, sub.Kinds, 2)
	assert.Equal(t, "event", sub.Kinds[0])
	assert.Equal(t, "message", sub.Kinds[1])
}

func TestHubQualityDuplicateSubscriptionTopicsAreRejectedBeforeStorage(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	in := SubscribeInput{AuthorityID: f.d.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{f.topic.ID, f.topic.ID}}

	// Act
	_, subscribeErr := f.d.Subscribe(context.Background(), f.repo, in)
	listed, listErr := f.d.ListSubscriptions(context.Background(), f.repo, "reader", false)

	// Assert
	assert.ErrorIs(t, subscribeErr, ErrInvalidInput)
	require.NoError(t, listErr)
	require.Len(t, listed, 0)
}

func TestHubQualityUnknownSubscriptionEventVersionCreatesNoCursor(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	enrollTestConsumer(t, f, "reader")
	in := SubscribeInput{AuthorityID: f.d.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{f.topic.ID}, Kinds: []string{"event"}, EventTypes: []EventTypeFilter{{TypeKey: "job.finished", Version: 2}}}

	// Act
	_, subscribeErr := f.d.Subscribe(context.Background(), f.repo, in)
	listed, listErr := f.d.ListSubscriptions(context.Background(), f.repo, "reader", false)

	// Assert
	assert.ErrorIs(t, subscribeErr, ErrVersionUnknown)
	require.NoError(t, listErr)
	require.Len(t, listed, 0)
}

func TestHubQualityArchivedTopicCannotGainNewSubscription(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	_, err := f.d.ArchiveTopic(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "Closed")
	require.NoError(t, err)

	// Act
	_, subscribeErr := f.d.Subscribe(context.Background(), f.repo, SubscribeInput{AuthorityID: f.d.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{f.topic.ID}})
	listed, listErr := f.d.ListSubscriptions(context.Background(), f.repo, "reader", false)

	// Assert
	assert.ErrorIs(t, subscribeErr, ErrArchived)
	require.NoError(t, listErr)
	require.Len(t, listed, 0)
}

func TestHubQualityMalformedAckTokenCannotAdvanceCursor(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	sub := subscribeTest(t, f, "reader", true, nil, nil)
	f.post(t, "A retained decision", "decision", nil)
	page, err := f.d.Pull(context.Background(), f.repo, "reader", sub.ID, 1)
	require.NoError(t, err)

	// Act
	_, ackErr := f.d.Ack(context.Background(), f.repo, "reader", sub.ID, f.d.AuthorityID(), 0, page.NextScanPosition, "not-a-token")
	replayed, pullErr := f.d.Pull(context.Background(), f.repo, "reader", sub.ID, 1)

	// Assert
	assert.ErrorIs(t, ackErr, ErrInvalidInput)
	require.NoError(t, pullErr)
	assert.Zero(t, replayed.PriorAcknowledged)
	require.Len(t, replayed.Entries, 1)
}

func TestHubQualityRevisionByOtherActorLeavesHeadUnchanged(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	first := f.post(t, "Original", "post", nil)
	in := MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Unauthorized correction"}, ExpectedRevision: first.Revision}

	// Act
	_, reviseErr := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "owner", in)
	retained, readErr := f.d.ReadMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, false)

	// Assert
	assert.ErrorIs(t, reviseErr, ErrUnauthorized)
	require.NoError(t, readErr)
	assert.Equal(t, first.EntryID, retained.EntryID)
	assert.Equal(t, first.Revision, retained.Revision)
}

func TestHubQualityExpiredPublicationIsHistoricalOnly(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	deadline := f.now.Add(time.Minute)
	m := f.post(t, "Time-limited", "post", &deadline)
	*f.now = deadline

	// Act
	_, liveErr := f.d.ReadPublication(context.Background(), f.repo, m.EntryID, false)
	historical, readErr := f.d.ReadPublication(context.Background(), f.repo, m.EntryID, true)

	// Assert
	assert.ErrorIs(t, liveErr, ErrExpired)
	require.NoError(t, readErr)
	assert.Equal(t, m.EntryID, historical.EntryID)
	assert.Equal(t, "Time-limited", historical.Text)
}

func TestHubQualityExpiredEventIsHistoricalOnly(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	deadline := f.now.Add(time.Minute)
	in := jobInput(f, `{"job":"build","duration":3}`, "build")
	in.ExpiresAt = &deadline
	e, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", in)
	require.NoError(t, err)
	*f.now = deadline

	// Act
	_, liveErr := f.d.ReadEvent(context.Background(), f.repo, e.EntryID, false)
	historical, readErr := f.d.ReadEvent(context.Background(), f.repo, e.EntryID, true)

	// Assert
	assert.ErrorIs(t, liveErr, ErrExpired)
	require.NoError(t, readErr)
	assert.Equal(t, e.EntryID, historical.EntryID)
	require.NotNil(t, historical.ExpiresAt)
	assert.Equal(t, deadline, *historical.ExpiresAt)
}

func TestHubQualityControlCharacterMessageCannotBePublished(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	in := MessageInput{AuthorityID: f.d.AuthorityID(), Text: "claim\x00hidden"}

	// Act
	_, postErr := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, postErr, ErrInvalidInput)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	require.Len(t, page.Entries, 0)
}

func TestHubQualityWhitespaceSourceReferenceCannotBePublished(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	in := MessageInput{AuthorityID: f.d.AuthorityID(), Text: "See evidence", SourceRefs: []SourceRef{{Kind: "document", Reference: " docs/evidence.md "}}}

	// Act
	_, postErr := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, postErr, ErrInvalidInput)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	require.Len(t, page.Entries, 0)
}

func TestHubQualityInvalidMessageIdempotencyKeyCreatesNoPublication(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	in := MessageInput{AuthorityID: f.d.AuthorityID(), Text: "A retained decision", IdempotencyKey: " retry "}

	// Act
	_, postErr := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, postErr, ErrInvalidInput)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	require.Len(t, page.Entries, 0)
}

func TestHubQualityNestedSchemaReferenceIsRejectedBeforeRegistration(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	schema := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"job":{"$ref":"https://example.test/job"}}}`)

	// Act
	_, registerErr := f.d.RegisterEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "job.finished", 1, schema)
	listed, listErr := f.d.ListEventSchemas(context.Background(), f.repo, f.topic.ID, 10, 0)

	// Assert
	assert.ErrorIs(t, registerErr, ErrUnsupportedSchema)
	require.NoError(t, listErr)
	assert.Empty(t, listed)
}

func TestHubQualityTupleSchemaReferenceIsRejectedBeforeRegistration(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	schema := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"array","prefixItems":[{"type":"string"},{"$ref":"https://example.test/job"}]}`)

	// Act
	_, registerErr := f.d.RegisterEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "job.finished", 1, schema)
	listed, listErr := f.d.ListEventSchemas(context.Background(), f.repo, f.topic.ID, 10, 0)

	// Assert
	assert.ErrorIs(t, registerErr, ErrUnsupportedSchema)
	require.NoError(t, listErr)
	assert.Empty(t, listed)
}

func TestHubQualityExpiredAtPublishEventDoesNotConsumeSequence(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	deadline := f.now.Add(-time.Second)
	in := jobInput(f, `{"job":"build","duration":3}`, "expired-event")
	in.ExpiresAt = &deadline

	// Act
	_, publishErr := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, publishErr, ErrExpired)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	assert.Empty(t, page.Entries)
}

func TestHubQualityMissingEventSourceDoesNotConsumeSequence(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	in := jobInput(f, `{"job":"build","duration":3}`, "missing-source")
	in.SourceRefs = []SourceRef{{Kind: "hub_entry", ScopeID: f.topic.ScopeID, Reference: "0123456789abcdef0123456789abcdef"}}

	// Act
	_, publishErr := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, publishErr, ErrNotFound)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	assert.Empty(t, page.Entries)
}

func TestHubQualityAuthorityMismatchCannotPostOrConsumeSequence(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	in := MessageInput{AuthorityID: "0123456789abcdef0123456789abcdef", Text: "Foreign authority claim"}

	// Act
	_, postErr := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, postErr, ErrReplayGap)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	assert.Empty(t, page.Entries)
}

func TestHubQualityRevisionCannotSmuggleExpiryWithoutChangeFlag(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	first := f.post(t, "Original", "original", nil)
	deadline := f.now.Add(time.Hour)
	in := MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Changed", ExpiresAt: &deadline}, ExpectedRevision: first.Revision}

	// Act
	_, reviseErr := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "author", in)
	retained, readErr := f.d.ReadMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, false)

	// Assert
	assert.ErrorIs(t, reviseErr, ErrInvalidInput)
	require.NoError(t, readErr)
	assert.Equal(t, first.EntryID, retained.EntryID)
	assert.Nil(t, retained.ExpiresAt)
}

func TestHubQualityStaleAuthorityHandleCannotWrite(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	_, err := f.d.sql.Exec("UPDATE hub_meta SET db_id=? WHERE singleton=1", "replacement-authority")
	require.NoError(t, err)
	in := MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Should not commit"}

	// Act
	_, postErr := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", in)
	var count int
	countErr := f.d.sql.QueryRow("SELECT COUNT(*) FROM publications WHERE topic_id=?", f.topic.ID).Scan(&count)

	// Assert
	assert.ErrorIs(t, postErr, ErrUnsupportedFormat)
	require.NoError(t, countErr)
	assert.Zero(t, count)
}

func TestHubQualityNestedDialectCannotOverrideSchemaPolicy(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	schema := []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"job":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"string"}}}`)

	// Act
	_, registerErr := f.d.RegisterEventSchema(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "job.finished", 1, schema)
	listed, listErr := f.d.ListEventSchemas(context.Background(), f.repo, f.topic.ID, 10, 0)

	// Assert
	assert.ErrorIs(t, registerErr, ErrUnsupportedSchema)
	require.NoError(t, listErr)
	assert.Empty(t, listed)
}

func TestHubQualityUnsupportedSourceKindCannotBePublished(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	in := MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Evidence", SourceRefs: []SourceRef{{Kind: "local_file", Reference: "docs/evidence.md"}}}

	// Act
	_, postErr := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, postErr, ErrInvalidInput)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	assert.Empty(t, page.Entries)
}

func TestHubQualityDocumentReferenceCannotForgeHubScope(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	in := MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Evidence", SourceRefs: []SourceRef{{Kind: "document", ScopeID: f.topic.ScopeID, Reference: "docs/evidence.md"}}}

	// Act
	_, postErr := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, postErr, ErrInvalidInput)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	assert.Empty(t, page.Entries)
}

func TestHubQualityMalformedCorrelationCannotBePublished(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	in := MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Evidence", CorrelationRefs: []string{"issue\x00hidden"}}

	// Act
	_, postErr := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, postErr, ErrInvalidInput)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	assert.Empty(t, page.Entries)
}

func TestHubQualityMalformedHubEntryReferenceCannotBePublished(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	in := MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Evidence", SourceRefs: []SourceRef{{Kind: "hub_entry", ScopeID: f.topic.ScopeID, Reference: "not-an-entry"}}}

	// Act
	_, postErr := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, postErr, ErrInvalidInput)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	assert.Empty(t, page.Entries)
}

func TestHubQualityDamagedMigrationDigestCannotBeReopened(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	home := filepath.Dir(f.d.Path())
	_, err := f.d.sql.Exec("UPDATE schema_migrations SET digest=? WHERE version=6", "invalid-digest")
	require.NoError(t, err)

	// Act
	reopened, openErr := Open(context.Background(), Options{Home: home})
	var digest string
	readErr := f.d.sql.QueryRow("SELECT digest FROM schema_migrations WHERE version=6").Scan(&digest)

	// Assert
	if reopened != nil {
		t.Cleanup(func() { _ = reopened.Close() })
	}
	assert.ErrorIs(t, openErr, ErrUnsupportedFormat)
	require.NoError(t, readErr)
	assert.Equal(t, "invalid-digest", digest)
}

func TestHubQualityMissingMigrationRecordCannotBeReopened(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	home := filepath.Dir(f.d.Path())
	_, err := f.d.sql.Exec("DELETE FROM schema_migrations WHERE version=6")
	require.NoError(t, err)

	// Act
	reopened, openErr := OpenReadOnly(context.Background(), Options{Home: home})
	var count int
	readErr := f.d.sql.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)

	// Assert
	if reopened != nil {
		t.Cleanup(func() { _ = reopened.Close() })
	}
	assert.ErrorIs(t, openErr, ErrUnsupportedFormat)
	require.NoError(t, readErr)
	assert.Equal(t, schemaVersion-1, count)
}

func TestHubQualityChangedUserVersionCannotAdmitWrite(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	_, err := f.d.sql.Exec("PRAGMA user_version=5")
	require.NoError(t, err)
	in := MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Should not commit"}

	// Act
	_, postErr := f.d.PostMessage(context.Background(), f.repo, f.topic.ID, "author", in)
	var count int
	countErr := f.d.sql.QueryRow("SELECT COUNT(*) FROM publications WHERE topic_id=?", f.topic.ID).Scan(&count)

	// Assert
	assert.ErrorIs(t, postErr, ErrUnsupportedFormat)
	require.NoError(t, countErr)
	assert.Zero(t, count)
}

func TestHubQualityStoredSchemaWithWrongDigestCannotBeShown(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	_, err := f.d.sql.Exec("INSERT INTO event_schemas (topic_id,type_key,version,dialect,schema_json,schema_sha256,registered_by,registered_at) VALUES (?,?,?,?,?,?,?,?)", f.topic.ID, "job.finished", 1, EventDialect, jobSchema, "invalid-digest", "owner", f.now.UnixMilli())
	require.NoError(t, err)

	// Act
	_, showErr := f.d.ShowEventSchema(context.Background(), f.repo, f.topic.ID, "job.finished", 1)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, showErr, ErrUnsupportedFormat)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
}

func TestHubQualityExpiredMessageCannotBeRevised(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	deadline := f.now.Add(time.Minute)
	first := f.post(t, "Original", "original", &deadline)
	*f.now = deadline
	in := MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Late correction"}, ExpectedRevision: first.Revision}

	// Act
	_, reviseErr := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "author", in)
	retained, readErr := f.d.ReadMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, true)

	// Assert
	assert.ErrorIs(t, reviseErr, ErrExpired)
	require.NoError(t, readErr)
	assert.Equal(t, first.EntryID, retained.EntryID)
	assert.Equal(t, first.Revision, retained.Revision)
}

func TestHubQualityStaleRevisionCannotReplaceMessageHead(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	first := f.post(t, "Original", "original", nil)
	in := MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Stale correction"}, ExpectedRevision: first.Revision + 1}

	// Act
	_, reviseErr := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, "author", in)
	retained, readErr := f.d.ReadMessage(context.Background(), f.repo, f.topic.ID, first.MessageID, false)

	// Assert
	assert.ErrorIs(t, reviseErr, ErrConflict)
	require.NoError(t, readErr)
	assert.Equal(t, first.EntryID, retained.EntryID)
	assert.Equal(t, first.Revision, retained.Revision)
}

func TestHubQualityUnknownMessageCannotBeRevised(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	in := MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "Orphan correction"}, ExpectedRevision: 1}

	// Act
	_, reviseErr := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, "0123456789abcdef0123456789abcdef", "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, reviseErr, ErrNotFound)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	assert.Empty(t, page.Entries)
}

func TestHubQualityEventWithoutAuthorityCannotBePublished(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	in := jobInput(f, `{"job":"build","duration":3}`, "missing-authority")
	in.AuthorityID = ""

	// Act
	_, publishErr := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, publishErr, ErrInvalidInput)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	assert.Empty(t, page.Entries)
}

func TestHubQualityEventWithInvalidOccurrenceCannotBePublished(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	in := jobInput(f, `{"job":"build","duration":3}`, "bad-occurrence")
	invalid := time.Time{}
	in.OccurredAt = &invalid

	// Act
	_, publishErr := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, publishErr, ErrInvalidInput)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	assert.Empty(t, page.Entries)
}

func TestHubQualityEventWithInvalidExpiryCannotBePublished(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	in := jobInput(f, `{"job":"build","duration":3}`, "bad-expiry")
	invalid := time.Time{}
	in.ExpiresAt = &invalid

	// Act
	_, publishErr := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", in)
	page, scanErr := f.d.ScanPublications(context.Background(), f.repo, 0, 10, true)

	// Assert
	assert.ErrorIs(t, publishErr, ErrInvalidInput)
	require.NoError(t, scanErr)
	assert.Zero(t, page.HighWater)
	assert.Empty(t, page.Entries)
}
