package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/hubstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runHubCoverageJSON[T any](t *testing.T, repo string, args ...string) T {
	t.Helper()
	cmd := NewHubCmd()
	cmd.SetArgs(append([]string{"--repo", repo, "--json"}, args...))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute(), out.String())
	var response hubJSONResponse[T]
	require.NoError(t, json.Unmarshal(out.Bytes(), &response), out.String())
	require.Equal(t, "devspecs.hub/v1", response.ContractVersion)
	return response.Result
}

func runHubCoverageText(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := NewHubCmd()
	cmd.SetArgs(append([]string{"--repo", repo}, args...))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute(), out.String())
	return out.String()
}

func runHubCoverageError(repo string, args ...string) error {
	cmd := NewHubCmd()
	cmd.SetArgs(append([]string{"--repo", repo, "--json"}, args...))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd.Execute()
}

type hubCoverageFixture struct {
	t    *testing.T
	repo string
	db   *hubstore.DB
}

func newHubCoverageFixture(t *testing.T) *hubCoverageFixture {
	t.Helper()
	repo := hubRepoFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, db.Close()) })
	_, err = db.EnrollRepo(context.Background(), repo)
	require.NoError(t, err)
	return &hubCoverageFixture{t: t, repo: repo, db: db}
}

func (f *hubCoverageFixture) actor(id string) {
	f.t.Helper()
	_, err := f.db.EnrollActor(context.Background(), id)
	require.NoError(f.t, err)
}

func (f *hubCoverageFixture) topic() hubstore.Topic {
	f.t.Helper()
	f.actor("owner")
	topic, err := f.db.CreateTopic(context.Background(), f.repo, "owner", hubstore.TopicInput{
		Key: "release", Name: "Release", Description: "Release coordination",
	})
	require.NoError(f.t, err)
	return topic
}

func (f *hubCoverageFixture) message(topic hubstore.Topic) hubstore.Message {
	f.t.Helper()
	message, err := f.db.PostMessage(context.Background(), f.repo, topic.ID, "owner", hubstore.MessageInput{
		AuthorityID: f.db.AuthorityID(), Text: "Initial status",
	})
	require.NoError(f.t, err)
	return message
}

func (f *hubCoverageFixture) consumer() {
	f.t.Helper()
	_, err := f.db.EnrollConsumer(context.Background(), f.db.AuthorityID(), "reader")
	require.NoError(f.t, err)
}

func (f *hubCoverageFixture) subscription(topic hubstore.Topic, fromBeginning bool) hubstore.Subscription {
	f.t.Helper()
	f.consumer()
	sub, err := f.db.Subscribe(context.Background(), f.repo, hubstore.SubscribeInput{
		AuthorityID: f.db.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{topic.ID},
		FromBeginning: fromBeginning,
	})
	require.NoError(f.t, err)
	return sub
}

const hubCoverageSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"state":{"type":"string"}},"required":["state"],"additionalProperties":false}`

func (f *hubCoverageFixture) schema(topic hubstore.Topic) {
	f.t.Helper()
	_, err := f.db.RegisterEventSchema(context.Background(), f.repo, topic.ID, "owner", topic.PolicyGeneration,
		"release.ready", 1, json.RawMessage(hubCoverageSchema))
	require.NoError(f.t, err)
}

func (f *hubCoverageFixture) event(topic hubstore.Topic) hubstore.Event {
	f.t.Helper()
	event, err := f.db.PublishEvent(context.Background(), f.repo, topic.ID, "owner", hubstore.EventInput{
		AuthorityID: f.db.AuthorityID(), TypeKey: "release.ready", Version: 1,
		Payload: json.RawMessage(`{"state":"ready"}`),
	})
	require.NoError(f.t, err)
	return event
}

func hubCoverageFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestHubCoverageActorEnrollJSON(t *testing.T) {
	// Arrange
	f := newHubCoverageFixture(t)
	// Act
	actor := runHubCoverageJSON[hubstore.Actor](t, f.repo, "actor", "enroll", "editor")
	// Assert
	assert.Equal(t, "editor", actor.ID)
}

func TestHubCoverageActorEnrollHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	output := runHubCoverageText(t, f.repo, "actor", "enroll", "operator")
	assert.Equal(t, "Actor: operator\n", output)
}

func TestHubCoverageConsumerEnrollJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	consumer := runHubCoverageJSON[hubstore.Consumer](t, f.repo, "consumer", "enroll", "terminal-reader")
	assert.Equal(t, "terminal-reader", consumer.ID)
}

func TestHubCoverageConsumerEnrollHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	output := runHubCoverageText(t, f.repo, "consumer", "enroll", "terminal-reader")
	assert.Equal(t, "Consumer: terminal-reader\n", output)
}

func TestHubCoverageTopicCreateJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	f.actor("owner")
	topic := runHubCoverageJSON[hubstore.Topic](t, f.repo, "topic", "create", "release", "--actor", "owner", "--name", "Release", "--description", "Release coordination", "--expires-at", "2030-01-01T00:00:00Z")
	assert.Equal(t, "release", topic.Key)
	assert.NotNil(t, topic.ExpiresAt)
}

func TestHubCoverageTopicCreateHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	f.actor("owner")
	output := runHubCoverageText(t, f.repo, "topic", "create", "release", "--actor", "owner", "--name", "Release", "--description", "Release coordination")
	assert.Contains(t, output, "Release")
}

func TestHubCoverageTopicShowJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	shown := runHubCoverageJSON[hubstore.Topic](t, f.repo, "topic", "show", topic.ID)
	assert.Equal(t, topic.ID, shown.ID)
}

func TestHubCoverageTopicShowHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	output := runHubCoverageText(t, f.repo, "topic", "show", topic.ID)
	assert.Contains(t, output, "Name: Release")
}

func TestHubCoverageTopicListQueryJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	listed := runHubCoverageJSON[[]hubstore.Topic](t, f.repo, "topic", "list", "--query", "Release")
	require.Len(t, listed, 1)
	assert.Equal(t, topic.ID, listed[0].ID)
}

func TestHubCoverageTopicListQueryHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	output := runHubCoverageText(t, f.repo, "topic", "list", "--query", "Release")
	assert.Contains(t, output, topic.ID)
}

func TestHubCoverageTopicEditClearsExpiry(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	expiry := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := f.db.EditTopic(context.Background(), f.repo, topic.ID, "owner", hubstore.TopicEdit{
		ChangeExpiry: true, ExpiresAt: &expiry, ExpectedGeneration: 1,
	})
	require.NoError(t, err)
	edited := runHubCoverageJSON[hubstore.Topic](t, f.repo, "topic", "edit", topic.ID, "--actor", "owner", "--generation", "2", "--name", "Release B07", "--description", "B07 coordination", "--clear-expiry")
	assert.Equal(t, "Release B07", edited.Name)
	assert.Nil(t, edited.ExpiresAt)
	assert.Equal(t, int64(3), edited.PolicyGeneration)
}

func TestHubCoverageTopicOwnerGrantJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.actor("editor")
	granted := runHubCoverageJSON[hubstore.Topic](t, f.repo, "topic", "owner", "grant", topic.ID, "editor", "--actor", "owner", "--generation", "1")
	assert.Equal(t, int64(2), granted.PolicyGeneration)
}

func TestHubCoverageTopicOwnerGrantHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.actor("editor")
	output := runHubCoverageText(t, f.repo, "topic", "owner", "grant", topic.ID, "editor", "--actor", "owner", "--generation", "1")
	assert.Contains(t, output, "Topic: release")
}

func TestHubCoverageTopicOwnerListJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.actor("editor")
	_, err := f.db.SetMaintainer(context.Background(), f.repo, topic.ID, "owner", "editor", 1, true)
	require.NoError(t, err)
	owners := runHubCoverageJSON[[]string](t, f.repo, "topic", "owner", "list", topic.ID)
	require.Len(t, owners, 1)
	assert.Equal(t, "editor", owners[0])
}

func TestHubCoverageTopicOwnerListHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.actor("editor")
	_, err := f.db.SetMaintainer(context.Background(), f.repo, topic.ID, "owner", "editor", 1, true)
	require.NoError(t, err)
	output := runHubCoverageText(t, f.repo, "topic", "owner", "list", topic.ID)
	assert.Contains(t, output, "editor")
}

func TestHubCoverageTopicOwnerRevoke(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.actor("editor")
	_, err := f.db.SetMaintainer(context.Background(), f.repo, topic.ID, "owner", "editor", 1, true)
	require.NoError(t, err)
	revoked := runHubCoverageJSON[hubstore.Topic](t, f.repo, "topic", "owner", "revoke", topic.ID, "editor", "--actor", "owner", "--generation", "2")
	assert.Equal(t, int64(3), revoked.PolicyGeneration)
}

func TestHubCoverageTopicArchive(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	archived := runHubCoverageJSON[hubstore.Topic](t, f.repo, "topic", "archive", topic.ID, "--actor", "owner", "--generation", "1", "--reason", "Release complete")
	assert.Equal(t, "archived", archived.State)
	assert.Equal(t, "Release complete", archived.ArchiveReason)
}

func TestHubCoverageTopicRestore(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	_, err := f.db.ArchiveTopic(context.Background(), f.repo, topic.ID, "owner", 1, "Finished")
	require.NoError(t, err)
	restored := runHubCoverageJSON[hubstore.Topic](t, f.repo, "topic", "restore", topic.ID, "--actor", "owner", "--generation", "2", "--reason", "New release work")
	assert.Equal(t, "active", restored.State)
	assert.Equal(t, int64(3), restored.PolicyGeneration)
}

func TestHubCoverageTopicListHidesArchive(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	_, err := f.db.ArchiveTopic(context.Background(), f.repo, topic.ID, "owner", 1, "Finished")
	require.NoError(t, err)
	listed := runHubCoverageJSON[[]hubstore.Topic](t, f.repo, "topic", "list", "--query", "Release")
	require.Len(t, listed, 0)
}

func TestHubCoverageTopicListAllIncludesArchive(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	_, err := f.db.ArchiveTopic(context.Background(), f.repo, topic.ID, "owner", 1, "Finished")
	require.NoError(t, err)
	listed := runHubCoverageJSON[[]hubstore.Topic](t, f.repo, "topic", "list", "--query", "Release", "--all")
	require.Len(t, listed, 1)
	assert.Equal(t, topic.ID, listed[0].ID)
}

func TestHubCoverageMessagePost(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := runHubCoverageJSON[hubstore.Message](t, f.repo, "message", "post", topic.ID, "--actor", "owner", "--text", "Initial status", "--key", "status-1", "--expires-at", "2030-01-01T00:00:00Z")
	assert.Equal(t, int64(1), message.Sequence)
	assert.NotNil(t, message.ExpiresAt)
}

func TestHubCoverageMessagePostReplay(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	first, err := f.db.PostMessage(context.Background(), f.repo, topic.ID, "owner", hubstore.MessageInput{AuthorityID: f.db.AuthorityID(), Text: "Initial status", IdempotencyKey: "status-1"})
	require.NoError(t, err)
	replay := runHubCoverageJSON[hubstore.Message](t, f.repo, "message", "post", topic.ID, "--actor", "owner", "--text", "Initial status", "--key", "status-1")
	assert.True(t, replay.Replayed)
	assert.Equal(t, first.EntryID, replay.EntryID)
}

func TestHubCoverageMessageReviseClearsExpiry(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	expiry := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	message, err := f.db.PostMessage(context.Background(), f.repo, topic.ID, "owner", hubstore.MessageInput{
		AuthorityID: f.db.AuthorityID(), Text: "Initial status", ExpiresAt: &expiry,
	})
	require.NoError(t, err)
	revised := runHubCoverageJSON[hubstore.Message](t, f.repo, "message", "revise", topic.ID, message.MessageID, "--actor", "owner", "--revision", "1", "--text", "Verified status", "--clear-expiry")
	assert.Equal(t, int64(2), revised.Revision)
	assert.Equal(t, "Verified status", revised.Text)
	assert.Nil(t, revised.ExpiresAt)
}

func TestHubCoverageMessagePin(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	pinned := runHubCoverageJSON[hubstore.Message](t, f.repo, "message", "pin", topic.ID, message.MessageID, "--actor", "owner", "--generation", "1", "--revision", "1")
	assert.True(t, pinned.Pinned)
}

func TestHubCoverageMessageUnpin(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	_, err := f.db.PinMessage(context.Background(), f.repo, topic.ID, message.MessageID, "owner", f.db.AuthorityID(), 1, 1, true)
	require.NoError(t, err)
	unpinned := runHubCoverageJSON[hubstore.Message](t, f.repo, "message", "unpin", topic.ID, message.MessageID, "--actor", "owner", "--generation", "1", "--revision", "1")
	assert.False(t, unpinned.Pinned)
}

func TestHubCoverageMessageVoteUp(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	f.actor("reader")
	voted := runHubCoverageJSON[hubstore.Message](t, f.repo, "message", "vote", topic.ID, message.MessageID, "--actor", "reader", "--revision", "1", "--value", "up")
	assert.Equal(t, int64(1), voted.Score)
}

func TestHubCoverageMessageVoteClear(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	f.actor("reader")
	_, err := f.db.VoteMessage(context.Background(), f.repo, topic.ID, message.MessageID, "reader", f.db.AuthorityID(), 1, 1)
	require.NoError(t, err)
	cleared := runHubCoverageJSON[hubstore.Message](t, f.repo, "message", "vote", topic.ID, message.MessageID, "--actor", "reader", "--revision", "1", "--value", "clear")
	assert.Equal(t, int64(0), cleared.Score)
}

func TestHubCoverageMessageListRanked(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	listed := runHubCoverageJSON[[]hubstore.Message](t, f.repo, "message", "list", topic.ID, "--ranked")
	require.Len(t, listed, 1)
	assert.Equal(t, message.EntryID, listed[0].EntryID)
}

func TestHubCoverageMessageListHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.message(topic)
	output := runHubCoverageText(t, f.repo, "message", "list", topic.ID)
	assert.Contains(t, output, "Initial status")
}

func TestHubCoverageMessageShowJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	shown := runHubCoverageJSON[hubstore.Message](t, f.repo, "message", "show", topic.ID, message.MessageID)
	assert.Equal(t, message.EntryID, shown.EntryID)
}

func TestHubCoverageMessageShowHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	output := runHubCoverageText(t, f.repo, "message", "show", topic.ID, message.MessageID)
	assert.Contains(t, output, "Initial status")
}

func TestHubCoverageMessageHistoryJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	_, err := f.db.ReviseMessage(context.Background(), f.repo, topic.ID, message.MessageID, "owner", hubstore.MessageRevisionInput{
		MessageInput: hubstore.MessageInput{AuthorityID: f.db.AuthorityID(), Text: "Verified status"}, ExpectedRevision: 1,
	})
	require.NoError(t, err)
	history := runHubCoverageJSON[[]hubstore.Message](t, f.repo, "message", "history", topic.ID, message.MessageID)
	require.Len(t, history, 2)
	assert.Equal(t, int64(1), history[0].Revision)
	assert.Equal(t, int64(2), history[1].Revision)
}

func TestHubCoverageMessageHistoryHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	output := runHubCoverageText(t, f.repo, "message", "history", topic.ID, message.MessageID)
	assert.Contains(t, output, "r1")
}

func TestHubCoverageTypeRegisterJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	path := hubCoverageFile(t, "schema.json", hubCoverageSchema)
	schema := runHubCoverageJSON[hubstore.EventSchema](t, f.repo, "type", "register", topic.ID, "release.ready", "1", "--actor", "owner", "--generation", "1", "--schema-file", path)
	assert.Equal(t, "release.ready", schema.TypeKey)
	assert.NotEmpty(t, schema.SHA256)
}

func TestHubCoverageTypeRegisterHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	path := hubCoverageFile(t, "schema.json", hubCoverageSchema)
	output := runHubCoverageText(t, f.repo, "type", "register", topic.ID, "release.ready", "1", "--actor", "owner", "--generation", "1", "--schema-file", path)
	assert.Contains(t, output, "release.ready@1")
}

func TestHubCoverageTypeShowJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	schema := runHubCoverageJSON[hubstore.EventSchema](t, f.repo, "type", "show", topic.ID, "release.ready", "1")
	assert.Equal(t, "release.ready", schema.TypeKey)
}

func TestHubCoverageTypeListJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	schemas := runHubCoverageJSON[[]hubstore.EventSchema](t, f.repo, "type", "list", topic.ID)
	require.Len(t, schemas, 1)
	assert.Equal(t, "release.ready", schemas[0].TypeKey)
}

func TestHubCoverageTypeListHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	output := runHubCoverageText(t, f.repo, "type", "list", topic.ID)
	assert.Contains(t, output, "release.ready@1")
}

func TestHubCoverageTypeRetire(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	retired := runHubCoverageJSON[hubstore.EventSchema](t, f.repo, "type", "retire", topic.ID, "release.ready", "1", "--actor", "owner", "--generation", "2")
	assert.Equal(t, "owner", retired.RetiredBy)
	assert.NotNil(t, retired.RetiredAt)
}

func TestHubCoverageEventPublishJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	path := hubCoverageFile(t, "payload.json", `{"state":"ready"}`)
	event := runHubCoverageJSON[hubstore.Event](t, f.repo, "event", "publish", topic.ID, "--actor", "owner", "--type", "release.ready", "--version", "1", "--payload-file", path)
	assert.Equal(t, "event", event.Kind)
	assert.JSONEq(t, `{"state":"ready"}`, string(event.Payload))
}

func TestHubCoverageEventPublishHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	path := hubCoverageFile(t, "payload.json", `{"state":"ready"}`)
	output := runHubCoverageText(t, f.repo, "event", "publish", topic.ID, "--actor", "owner", "--type", "release.ready", "--version", "1", "--payload-file", path)
	assert.Contains(t, output, "release.ready@1")
}

func TestHubCoverageEventPublishReplay(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	first, err := f.db.PublishEvent(context.Background(), f.repo, topic.ID, "owner", hubstore.EventInput{
		AuthorityID: f.db.AuthorityID(), TypeKey: "release.ready", Version: 1,
		Payload: json.RawMessage(`{"state":"ready"}`), IdempotencyKey: "ready-1",
	})
	require.NoError(t, err)
	path := hubCoverageFile(t, "payload.json", `{"state":"ready"}`)
	replay := runHubCoverageJSON[hubstore.Event](t, f.repo, "event", "publish", topic.ID, "--actor", "owner", "--type", "release.ready", "--version", "1", "--payload-file", path, "--key", "ready-1")
	assert.True(t, replay.Replayed)
	assert.Equal(t, first.EntryID, replay.EntryID)
}

func TestHubCoverageEventPublishCorrection(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	first := f.event(topic)
	path := hubCoverageFile(t, "payload.json", `{"state":"corrected"}`)
	corrected := runHubCoverageJSON[hubstore.Event](t, f.repo, "event", "publish", topic.ID, "--actor", "owner", "--type", "release.ready", "--version", "1", "--payload-file", path, "--corrects", first.EntryID)
	assert.Equal(t, first.EntryID, corrected.CorrectsEntryID)
	assert.Equal(t, int64(2), corrected.Sequence)
}

func TestHubCoverageEventShowJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	event := f.event(topic)
	shown := runHubCoverageJSON[hubstore.Event](t, f.repo, "event", "show", event.EntryID)
	assert.Equal(t, event.EntryID, shown.EntryID)
}

func TestHubCoverageEventShowHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	event := f.event(topic)
	output := runHubCoverageText(t, f.repo, "event", "show", event.EntryID)
	assert.Contains(t, output, "release.ready@1")
}

func TestHubCoverageSubscribeAddJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.consumer()
	sub := runHubCoverageJSON[hubstore.Subscription](t, f.repo, "subscribe", "add", "--consumer", "reader", "--topic", topic.ID, "--kind", "message", "--from-beginning")
	assert.Equal(t, "reader", sub.ConsumerID)
	require.Len(t, sub.Kinds, 1)
	assert.Equal(t, "message", sub.Kinds[0])
}

func TestHubCoverageSubscribeAddFilteredEvent(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	f.consumer()
	sub := runHubCoverageJSON[hubstore.Subscription](t, f.repo, "subscribe", "add", "--consumer", "reader", "--topic", topic.ID, "--kind", "event", "--event-type", "release.ready@1")
	require.Len(t, sub.EventTypes, 1)
	assert.Equal(t, "release.ready", sub.EventTypes[0].TypeKey)
}

func TestHubCoverageSubscribeAddHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.consumer()
	output := runHubCoverageText(t, f.repo, "subscribe", "add", "--consumer", "reader", "--topic", topic.ID)
	assert.Contains(t, output, "Consumer: reader")
}

func TestHubCoverageSubscribeListJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	sub := f.subscription(topic, false)
	subs := runHubCoverageJSON[[]hubstore.Subscription](t, f.repo, "subscribe", "list", "--consumer", "reader")
	require.Len(t, subs, 1)
	assert.Equal(t, sub.ID, subs[0].ID)
}

func TestHubCoverageSubscribeListHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	sub := f.subscription(topic, false)
	output := runHubCoverageText(t, f.repo, "subscribe", "list", "--consumer", "reader")
	assert.Contains(t, output, sub.ID)
}

func TestHubCoverageSubscribeRemoveJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	sub := f.subscription(topic, false)
	removed := runHubCoverageJSON[hubstore.Subscription](t, f.repo, "subscribe", "remove", sub.ID, "--consumer", "reader")
	assert.NotNil(t, removed.RemovedAt)
}

func TestHubCoverageSubscribeRemoveHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	sub := f.subscription(topic, false)
	output := runHubCoverageText(t, f.repo, "subscribe", "remove", sub.ID, "--consumer", "reader")
	assert.Contains(t, output, sub.ID)
}

func TestHubCoverageSubscribeListHidesRemoved(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	sub := f.subscription(topic, false)
	_, err := f.db.RemoveSubscription(context.Background(), f.repo, "reader", sub.ID, f.db.AuthorityID())
	require.NoError(t, err)
	subs := runHubCoverageJSON[[]hubstore.Subscription](t, f.repo, "subscribe", "list", "--consumer", "reader")
	require.Len(t, subs, 0)
}

func TestHubCoverageSubscribeListAllIncludesRemoved(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	sub := f.subscription(topic, false)
	_, err := f.db.RemoveSubscription(context.Background(), f.repo, "reader", sub.ID, f.db.AuthorityID())
	require.NoError(t, err)
	subs := runHubCoverageJSON[[]hubstore.Subscription](t, f.repo, "subscribe", "list", "--consumer", "reader", "--all")
	require.Len(t, subs, 1)
	assert.Equal(t, sub.ID, subs[0].ID)
}

func TestHubCoveragePullMessageJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	sub := f.subscription(topic, true)
	page := runHubCoverageJSON[hubstore.PullPage](t, f.repo, "pull", sub.ID, "--consumer", "reader", "--limit", "1")
	require.Len(t, page.Entries, 1)
	require.NotNil(t, page.Entries[0].Message)
	assert.Equal(t, message.EntryID, page.Entries[0].Message.EntryID)
	assert.NotEmpty(t, page.AckToken)
}

func TestHubCoveragePullMessageHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.message(topic)
	sub := f.subscription(topic, true)
	output := runHubCoverageText(t, f.repo, "pull", sub.ID, "--consumer", "reader")
	assert.Contains(t, output, "Initial status")
}

func TestHubCoveragePullFilteredEvent(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	f.message(topic)
	event := f.event(topic)
	f.consumer()
	sub, err := f.db.Subscribe(context.Background(), f.repo, hubstore.SubscribeInput{
		AuthorityID: f.db.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{topic.ID},
		Kinds: []string{"event"}, EventTypes: []hubstore.EventTypeFilter{{TypeKey: "release.ready", Version: 1}},
		FromBeginning: true,
	})
	require.NoError(t, err)
	page := runHubCoverageJSON[hubstore.PullPage](t, f.repo, "pull", sub.ID, "--consumer", "reader", "--limit", "2")
	require.Len(t, page.Entries, 1)
	require.NotNil(t, page.Entries[0].Event)
	assert.Equal(t, event.EntryID, page.Entries[0].Event.EntryID)
	assert.Equal(t, int64(2), page.NextScanPosition)
}

func TestHubCoveragePullFilteredEventHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	f.event(topic)
	f.consumer()
	sub, err := f.db.Subscribe(context.Background(), f.repo, hubstore.SubscribeInput{
		AuthorityID: f.db.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{topic.ID},
		Kinds: []string{"event"}, EventTypes: []hubstore.EventTypeFilter{{TypeKey: "release.ready", Version: 1}},
		FromBeginning: true,
	})
	require.NoError(t, err)
	output := runHubCoverageText(t, f.repo, "pull", sub.ID, "--consumer", "reader")
	assert.Contains(t, output, "release.ready@1")
}

func TestHubCoverageAckJSON(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.message(topic)
	sub := f.subscription(topic, true)
	page, err := f.db.Pull(context.Background(), f.repo, "reader", sub.ID, 1)
	require.NoError(t, err)
	ack := runHubCoverageJSON[struct {
		SubscriptionID       string `json:"subscription_id"`
		AcknowledgedSequence int64  `json:"acknowledged_sequence"`
	}](t, f.repo, "ack", sub.ID, "--consumer", "reader", "--prior", "0", "--next", "1", "--token", page.AckToken)
	assert.Equal(t, sub.ID, ack.SubscriptionID)
	assert.Equal(t, int64(1), ack.AcknowledgedSequence)
}

func TestHubCoverageAckHuman(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.message(topic)
	sub := f.subscription(topic, true)
	page, err := f.db.Pull(context.Background(), f.repo, "reader", sub.ID, 1)
	require.NoError(t, err)
	output := runHubCoverageText(t, f.repo, "ack", sub.ID, "--consumer", "reader", "--prior", "0", "--next", "1", "--token", page.AckToken)
	assert.Contains(t, output, "Acknowledged: 1")
}

func TestHubCoveragePullAfterAckEmpty(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.message(topic)
	sub := f.subscription(topic, true)
	page, err := f.db.Pull(context.Background(), f.repo, "reader", sub.ID, 1)
	require.NoError(t, err)
	_, err = f.db.Ack(context.Background(), f.repo, "reader", sub.ID, f.db.AuthorityID(), 0, 1, page.AckToken)
	require.NoError(t, err)
	cleared := runHubCoverageJSON[hubstore.PullPage](t, f.repo, "pull", sub.ID, "--consumer", "reader")
	assert.Equal(t, int64(1), cleared.PriorAcknowledged)
	require.Len(t, cleared.Entries, 0)
}

func TestHubCoverageTopicCreateRejectsMalformedExpiry(t *testing.T) {
	f := newHubCoverageFixture(t)
	f.actor("owner")
	err := runHubCoverageError(f.repo, "topic", "create", "bad", "--actor", "owner", "--name", "Bad", "--description", "Bad expiry", "--expires-at", "tomorrow")
	assert.ErrorContains(t, err, "RFC3339")
}

func TestHubCoverageTopicCreateRejectsPastExpiry(t *testing.T) {
	f := newHubCoverageFixture(t)
	f.actor("owner")
	err := runHubCoverageError(f.repo, "topic", "create", "expired", "--actor", "owner", "--name", "Expired", "--description", "Past deadline", "--expires-at", "2020-01-01T00:00:00Z")
	assert.ErrorIs(t, err, hubstore.ErrExpired)
}

func TestHubCoverageTopicEditRejectsExpiryAndClear(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	err := runHubCoverageError(f.repo, "topic", "edit", topic.ID, "--actor", "owner", "--generation", "1", "--clear-expiry", "--expires-at", "2030-01-01T00:00:00Z")
	assert.ErrorContains(t, err, "cannot be combined")
}

func TestHubCoverageTopicEditRejectsPastExpiry(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	err := runHubCoverageError(f.repo, "topic", "edit", topic.ID, "--actor", "owner", "--generation", "1", "--expires-at", "2020-01-01T00:00:00Z")
	assert.ErrorIs(t, err, hubstore.ErrExpired)
}

func TestHubCoverageTopicEditRejectsNoChange(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	err := runHubCoverageError(f.repo, "topic", "edit", topic.ID, "--actor", "owner", "--generation", "1")
	assert.ErrorIs(t, err, hubstore.ErrInvalidInput)
}

func TestHubCoverageTopicEditRejectsOutsider(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.actor("outsider")
	err := runHubCoverageError(f.repo, "topic", "edit", topic.ID, "--actor", "outsider", "--generation", "1", "--name", "Hacked")
	assert.ErrorIs(t, err, hubstore.ErrUnauthorized)
}

func TestHubCoverageTopicArchiveRejectsStaleGeneration(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	err := runHubCoverageError(f.repo, "topic", "archive", topic.ID, "--actor", "owner", "--generation", "9", "--reason", "Stale")
	assert.ErrorIs(t, err, hubstore.ErrConflict)
}

func TestHubCoverageTopicArchiveRejectsOutsider(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.actor("outsider")
	err := runHubCoverageError(f.repo, "topic", "archive", topic.ID, "--actor", "outsider", "--generation", "1", "--reason", "Not my topic")
	assert.ErrorIs(t, err, hubstore.ErrUnauthorized)
}

func TestHubCoverageTopicArchiveRejectsArchived(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	_, err := f.db.ArchiveTopic(context.Background(), f.repo, topic.ID, "owner", 1, "Finished")
	require.NoError(t, err)
	err = runHubCoverageError(f.repo, "topic", "archive", topic.ID, "--actor", "owner", "--generation", "2", "--reason", "Again")
	assert.ErrorIs(t, err, hubstore.ErrArchived)
}

func TestHubCoverageTopicRestoreRejectsActive(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	err := runHubCoverageError(f.repo, "topic", "restore", topic.ID, "--actor", "owner", "--generation", "1", "--reason", "Already live")
	assert.ErrorIs(t, err, hubstore.ErrConflict)
}

func TestHubCoverageTopicGrantRejectsOutsider(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.actor("outsider")
	err := runHubCoverageError(f.repo, "topic", "owner", "grant", topic.ID, "outsider", "--actor", "outsider", "--generation", "1")
	assert.ErrorIs(t, err, hubstore.ErrUnauthorized)
}

func TestHubCoverageMessagePostRejectsEmptyText(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	err := runHubCoverageError(f.repo, "message", "post", topic.ID, "--actor", "owner", "--text", " ")
	assert.ErrorIs(t, err, hubstore.ErrInvalidInput)
}

func TestHubCoverageMessagePostRejectsMalformedExpiry(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	err := runHubCoverageError(f.repo, "message", "post", topic.ID, "--actor", "owner", "--text", "Future", "--expires-at", "later")
	assert.ErrorContains(t, err, "RFC3339")
}

func TestHubCoverageMessagePostRejectsPastExpiry(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	err := runHubCoverageError(f.repo, "message", "post", topic.ID, "--actor", "owner", "--text", "Late", "--expires-at", "2020-01-01T00:00:00Z")
	assert.ErrorIs(t, err, hubstore.ErrExpired)
}

func TestHubCoverageMessagePostRejectsReplayConflict(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	_, err := f.db.PostMessage(context.Background(), f.repo, topic.ID, "owner", hubstore.MessageInput{
		AuthorityID: f.db.AuthorityID(), Text: "Original", IdempotencyKey: "unique-key",
	})
	require.NoError(t, err)
	err = runHubCoverageError(f.repo, "message", "post", topic.ID, "--actor", "owner", "--text", "Different", "--key", "unique-key")
	assert.ErrorIs(t, err, hubstore.ErrIdempotencyConflict)
}

func TestHubCoverageMessagePostRejectsArchive(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	_, err := f.db.ArchiveTopic(context.Background(), f.repo, topic.ID, "owner", 1, "Finished")
	require.NoError(t, err)
	err = runHubCoverageError(f.repo, "message", "post", topic.ID, "--actor", "owner", "--text", "Too late")
	assert.ErrorIs(t, err, hubstore.ErrArchived)
}

func TestHubCoverageMessageReviseRejectsExpiryAndClear(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	err := runHubCoverageError(f.repo, "message", "revise", topic.ID, message.MessageID, "--actor", "owner", "--revision", "1", "--text", "Corrected", "--clear-expiry", "--expires-at", "2030-01-01T00:00:00Z")
	assert.ErrorContains(t, err, "cannot be combined")
}

func TestHubCoverageMessageReviseRejectsStaleRevision(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	err := runHubCoverageError(f.repo, "message", "revise", topic.ID, message.MessageID, "--actor", "owner", "--revision", "2", "--text", "Corrected")
	assert.ErrorIs(t, err, hubstore.ErrConflict)
}

func TestHubCoverageMessageReviseRejectsOtherAuthor(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	f.actor("outsider")
	err := runHubCoverageError(f.repo, "message", "revise", topic.ID, message.MessageID, "--actor", "outsider", "--revision", "1", "--text", "Hijacked")
	assert.ErrorIs(t, err, hubstore.ErrUnauthorized)
}

func TestHubCoverageMessagePinRejectsOutsider(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	f.actor("outsider")
	err := runHubCoverageError(f.repo, "message", "pin", topic.ID, message.MessageID, "--actor", "outsider", "--generation", "1", "--revision", "1")
	assert.ErrorIs(t, err, hubstore.ErrUnauthorized)
}

func TestHubCoverageMessageVoteRejectsUnknownValue(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	err := runHubCoverageError(f.repo, "message", "vote", topic.ID, message.MessageID, "--actor", "owner", "--revision", "1", "--value", "sideways")
	assert.ErrorContains(t, err, "must be up, down, or clear")
}

func TestHubCoverageMessageListHidesArchive(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.message(topic)
	_, err := f.db.ArchiveTopic(context.Background(), f.repo, topic.ID, "owner", 1, "Finished")
	require.NoError(t, err)
	listed := runHubCoverageJSON[[]hubstore.Message](t, f.repo, "message", "list", topic.ID)
	require.Len(t, listed, 0)
}

func TestHubCoverageMessageShowRejectsArchive(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	_, err := f.db.ArchiveTopic(context.Background(), f.repo, topic.ID, "owner", 1, "Finished")
	require.NoError(t, err)
	err = runHubCoverageError(f.repo, "message", "show", topic.ID, message.MessageID)
	assert.ErrorIs(t, err, hubstore.ErrArchived)
}

func TestHubCoverageMessageShowHistoricalArchive(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	_, err := f.db.ArchiveTopic(context.Background(), f.repo, topic.ID, "owner", 1, "Finished")
	require.NoError(t, err)
	shown := runHubCoverageJSON[hubstore.Message](t, f.repo, "message", "show", topic.ID, message.MessageID, "--historical")
	assert.Equal(t, message.EntryID, shown.EntryID)
}

func TestHubCoverageMessageHistoryArchive(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	message := f.message(topic)
	_, err := f.db.ArchiveTopic(context.Background(), f.repo, topic.ID, "owner", 1, "Finished")
	require.NoError(t, err)
	history := runHubCoverageJSON[[]hubstore.Message](t, f.repo, "message", "history", topic.ID, message.MessageID)
	require.Len(t, history, 1)
	assert.Equal(t, message.EntryID, history[0].EntryID)
}

func TestHubCoverageTypeRegisterRejectsZeroVersion(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	path := hubCoverageFile(t, "schema.json", hubCoverageSchema)
	err := runHubCoverageError(f.repo, "type", "register", topic.ID, "release.ready", "0", "--actor", "owner", "--generation", "1", "--schema-file", path)
	assert.ErrorContains(t, err, "positive integer")
}

func TestHubCoverageTypeRegisterRejectsMissingFile(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	err := runHubCoverageError(f.repo, "type", "register", topic.ID, "release.ready", "1", "--actor", "owner", "--generation", "1", "--schema-file", filepath.Join(t.TempDir(), "missing.json"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestHubCoverageTypeRegisterRejectsOversizeFile(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	path := hubCoverageFile(t, "large.json", string(bytes.Repeat([]byte("x"), 16*1024+1)))
	err := runHubCoverageError(f.repo, "type", "register", topic.ID, "release.ready", "1", "--actor", "owner", "--generation", "1", "--schema-file", path)
	assert.ErrorContains(t, err, "input exceeds")
}

func TestHubCoverageTypeShowRejectsBadVersion(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	err := runHubCoverageError(f.repo, "type", "show", topic.ID, "release.ready", "zero")
	assert.ErrorContains(t, err, "positive integer")
}

func TestHubCoverageTypeRetireRejectsBadVersion(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	err := runHubCoverageError(f.repo, "type", "retire", topic.ID, "release.ready", "oops", "--actor", "owner", "--generation", "1")
	assert.ErrorContains(t, err, "positive integer")
}

func TestHubCoverageEventPublishRejectsInvalidPayload(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	path := hubCoverageFile(t, "payload.json", `{"state":false}`)
	err := runHubCoverageError(f.repo, "event", "publish", topic.ID, "--actor", "owner", "--type", "release.ready", "--version", "1", "--payload-file", path)
	assert.ErrorIs(t, err, hubstore.ErrSchemaInvalid)
}

func TestHubCoverageEventPublishRejectsMissingFile(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	err := runHubCoverageError(f.repo, "event", "publish", topic.ID, "--actor", "owner", "--type", "release.ready", "--version", "1", "--payload-file", filepath.Join(t.TempDir(), "missing.json"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestHubCoverageEventPublishRejectsMalformedExpiry(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	path := hubCoverageFile(t, "payload.json", `{"state":"ready"}`)
	err := runHubCoverageError(f.repo, "event", "publish", topic.ID, "--actor", "owner", "--type", "release.ready", "--version", "1", "--payload-file", path, "--expires-at", "soon")
	assert.ErrorContains(t, err, "RFC3339")
}

func TestHubCoverageEventPublishRejectsRetiredVersion(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	_, err := f.db.RetireEventSchema(context.Background(), f.repo, topic.ID, "owner", 2, "release.ready", 1)
	require.NoError(t, err)
	path := hubCoverageFile(t, "payload.json", `{"state":"ready"}`)
	err = runHubCoverageError(f.repo, "event", "publish", topic.ID, "--actor", "owner", "--type", "release.ready", "--version", "1", "--payload-file", path)
	assert.ErrorIs(t, err, hubstore.ErrVersionRetired)
}

func TestHubCoverageEventPublishRejectsReplayConflict(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	_, err := f.db.PublishEvent(context.Background(), f.repo, topic.ID, "owner", hubstore.EventInput{
		AuthorityID: f.db.AuthorityID(), TypeKey: "release.ready", Version: 1,
		Payload: json.RawMessage(`{"state":"ready"}`), IdempotencyKey: "ready-1",
	})
	require.NoError(t, err)
	path := hubCoverageFile(t, "payload.json", `{"state":"different"}`)
	err = runHubCoverageError(f.repo, "event", "publish", topic.ID, "--actor", "owner", "--type", "release.ready", "--version", "1", "--payload-file", path, "--key", "ready-1")
	assert.ErrorIs(t, err, hubstore.ErrIdempotencyConflict)
}

func TestHubCoverageEventPublishRejectsArchive(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	_, err := f.db.ArchiveTopic(context.Background(), f.repo, topic.ID, "owner", 2, "Finished")
	require.NoError(t, err)
	path := hubCoverageFile(t, "payload.json", `{"state":"ready"}`)
	err = runHubCoverageError(f.repo, "event", "publish", topic.ID, "--actor", "owner", "--type", "release.ready", "--version", "1", "--payload-file", path)
	assert.ErrorIs(t, err, hubstore.ErrArchived)
}

func TestHubCoverageEventShowRejectsArchive(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	event := f.event(topic)
	_, err := f.db.ArchiveTopic(context.Background(), f.repo, topic.ID, "owner", 2, "Finished")
	require.NoError(t, err)
	err = runHubCoverageError(f.repo, "event", "show", event.EntryID)
	assert.ErrorIs(t, err, hubstore.ErrArchived)
}

func TestHubCoverageEventShowHistoricalArchive(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.schema(topic)
	event := f.event(topic)
	_, err := f.db.ArchiveTopic(context.Background(), f.repo, topic.ID, "owner", 2, "Finished")
	require.NoError(t, err)
	shown := runHubCoverageJSON[hubstore.Event](t, f.repo, "event", "show", event.EntryID, "--historical")
	assert.Equal(t, event.EntryID, shown.EntryID)
}

func TestHubCoverageSubscribeAddRejectsMissingSelectorVersion(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.consumer()
	err := runHubCoverageError(f.repo, "subscribe", "add", "--consumer", "reader", "--topic", topic.ID, "--event-type", "release.ready")
	assert.ErrorContains(t, err, "key@version")
}

func TestHubCoverageSubscribeAddRejectsZeroSelectorVersion(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.consumer()
	err := runHubCoverageError(f.repo, "subscribe", "add", "--consumer", "reader", "--topic", topic.ID, "--event-type", "release.ready@0")
	assert.ErrorContains(t, err, "positive integer")
}

func TestHubCoverageSubscribeAddRejectsBadKind(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.consumer()
	err := runHubCoverageError(f.repo, "subscribe", "add", "--consumer", "reader", "--topic", topic.ID, "--kind", "unknown")
	assert.ErrorIs(t, err, hubstore.ErrInvalidInput)
}

func TestHubCoverageSubscribeAddRejectsDuplicateTopic(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.consumer()
	err := runHubCoverageError(f.repo, "subscribe", "add", "--consumer", "reader", "--topic", topic.ID, "--topic", topic.ID)
	assert.ErrorIs(t, err, hubstore.ErrInvalidInput)
}

func TestHubCoverageConsumerEnrollRejectsBadID(t *testing.T) {
	f := newHubCoverageFixture(t)
	err := runHubCoverageError(f.repo, "consumer", "enroll", "bad consumer")
	assert.ErrorIs(t, err, hubstore.ErrInvalidInput)
}

func TestHubCoveragePullRejectsWrongConsumer(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	sub := f.subscription(topic, true)
	err := runHubCoverageError(f.repo, "pull", sub.ID, "--consumer", "someone-else")
	assert.ErrorIs(t, err, hubstore.ErrNotFound)
}

func TestHubCoveragePullRejectsZeroLimit(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	sub := f.subscription(topic, true)
	err := runHubCoverageError(f.repo, "pull", sub.ID, "--consumer", "reader", "--limit", "0")
	assert.ErrorIs(t, err, hubstore.ErrInvalidInput)
}

func TestHubCoverageAckRejectsInvalidToken(t *testing.T) {
	f := newHubCoverageFixture(t)
	topic := f.topic()
	f.message(topic)
	sub := f.subscription(topic, true)
	err := runHubCoverageError(f.repo, "ack", sub.ID, "--consumer", "reader", "--prior", "0", "--next", "1", "--token", "invalid")
	assert.Error(t, err)
}

func TestHubCoverageUnavailableHomeFailsClosed(t *testing.T) {
	repo := hubRepoFixture(t)
	blockedHome := hubCoverageFile(t, "home-is-a-file", "do not replace")
	t.Setenv("DEVSPECS_HOME", blockedHome)
	err := runHubCoverageError(repo, "actor", "enroll", "writer")
	assert.Error(t, err)
	contents, readErr := os.ReadFile(blockedHome)
	require.NoError(t, readErr)
	assert.Equal(t, "do not replace", string(contents))
}
