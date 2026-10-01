package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/hubstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hubGlobalTopicFixture(t *testing.T) (string, hubstore.Topic) {
	t.Helper()
	repo := hubRepoFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollActor(context.Background(), "agent-a")
	require.NoError(t, err)
	_, err = db.EnrollRepo(context.Background(), hubstore.GlobalScopeSelector)
	require.NoError(t, err)
	topic, err := db.CreateTopic(context.Background(), hubstore.GlobalScopeSelector, "agent-a", hubstore.TopicInput{
		Key: "local-go", Name: "Local Go", Description: "Coordinate heavy Go runs",
	})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	return repo, topic
}

func TestHubTopicCreateGlobalAddressCreatesHomeScope(t *testing.T) {
	// Arrange
	_ = hubRepoFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollActor(context.Background(), "agent-a")
	require.NoError(t, err)
	require.NoError(t, db.Close())
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "create", "global:local-go", "--actor", "agent-a", "--name", "Local Go", "--description", "Coordinate heavy Go runs", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()
	var response hubJSONResponse[hubstore.Topic]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "global", response.ScopeKind)
	assert.Equal(t, "global", response.Result.ScopeKind)
	assert.Equal(t, "local-go", response.Result.Key)
	assert.NotEmpty(t, response.Result.ID)
}

func TestHubTopicListGlobalAddressShowsExplicitGlobalTopics(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "list", "global:", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()
	var response hubJSONResponse[[]hubstore.Topic]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "global", response.ScopeKind)
	require.Len(t, response.Result, 1)
	assert.Equal(t, topic.ID, response.Result[0].ID)
	assert.Equal(t, "global", response.Result[0].ScopeKind)
}

func TestHubTopicListRepoAddressDoesNotShowGlobalTopics(t *testing.T) {
	// Arrange
	repo, _ := hubGlobalTopicFixture(t)
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "list", "--repo", repo, "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()
	var response hubJSONResponse[[]hubstore.Topic]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "git", response.ScopeKind)
	assert.Empty(t, response.Result)
}

func TestHubTopicShowGlobalAddressRejectsExplicitRepo(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "show", "global:topic-id", "--repo", repo})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	// Act
	err := cmd.Execute()

	// Assert
	assert.ErrorContains(t, err, "--repo cannot be combined")
}

func TestHubSubscribeAddGlobalTopicCreatesGlobalSubscription(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollConsumer(context.Background(), db.AuthorityID(), "reader")
	require.NoError(t, err)
	require.NoError(t, db.Close())
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"subscribe", "add", "--consumer", "reader", "--topic", "global:" + topic.ID, "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()
	var response hubJSONResponse[hubstore.Subscription]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "global", response.ScopeKind)
	require.Len(t, response.Result.TopicIDs, 1)
	assert.Equal(t, topic.ID, response.Result.TopicIDs[0])
}

func TestHubSubscribeAddMixedTopicsRejectsBeforeMutation(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"subscribe", "add", "--consumer", "reader", "--topic", "global:" + topic.ID, "--topic", "repo-topic"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	// Act
	err := cmd.Execute()

	// Assert
	assert.ErrorContains(t, err, "same repo or global: scope")
}

func TestHubPullGlobalSubscriptionReadsMessage(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollConsumer(context.Background(), db.AuthorityID(), "reader")
	require.NoError(t, err)
	sub, err := db.Subscribe(context.Background(), hubstore.GlobalScopeSelector, hubstore.SubscribeInput{
		AuthorityID: db.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{topic.ID}, FromBeginning: true,
	})
	require.NoError(t, err)
	message, err := db.PostMessage(context.Background(), hubstore.GlobalScopeSelector, topic.ID, "agent-a", hubstore.MessageInput{
		AuthorityID: db.AuthorityID(), Text: "Running fat-25",
	})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"pull", "global:" + sub.ID, "--consumer", "reader", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()
	var response hubJSONResponse[hubstore.PullPage]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "global", response.ScopeKind)
	assert.Equal(t, sub.ID, response.Result.SubscriptionID)
	require.Len(t, response.Result.Entries, 1)
	require.NotNil(t, response.Result.Entries[0].Message)
	assert.Equal(t, message.EntryID, response.Result.Entries[0].Message.EntryID)
}

func TestHubAckGlobalSubscriptionAdvancesCursor(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollConsumer(context.Background(), db.AuthorityID(), "reader")
	require.NoError(t, err)
	sub, err := db.Subscribe(context.Background(), hubstore.GlobalScopeSelector, hubstore.SubscribeInput{
		AuthorityID: db.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{topic.ID}, FromBeginning: true,
	})
	require.NoError(t, err)
	_, err = db.PostMessage(context.Background(), hubstore.GlobalScopeSelector, topic.ID, "agent-a", hubstore.MessageInput{
		AuthorityID: db.AuthorityID(), Text: "Running fat-25",
	})
	require.NoError(t, err)
	page, err := db.Pull(context.Background(), hubstore.GlobalScopeSelector, "reader", sub.ID, 50)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"ack", "global:" + sub.ID, "--consumer", "reader", "--prior", "0", "--next", "1", "--token", page.AckToken, "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()
	var response hubJSONResponse[struct {
		SubscriptionID       string `json:"subscription_id"`
		AcknowledgedSequence int64  `json:"acknowledged_sequence"`
	}]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "global", response.ScopeKind)
	assert.Equal(t, sub.ID, response.Result.SubscriptionID)
	assert.Equal(t, int64(1), response.Result.AcknowledgedSequence)
}

func TestHubMessagePostGlobalTopicPublishesInGlobalScope(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"message", "post", "global:" + topic.ID, "--actor", "agent-a", "--text", "Heavy Go run started", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()
	var response hubJSONResponse[hubstore.Message]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "global", response.ScopeKind)
	assert.Equal(t, topic.ID, response.Result.TopicID)
	assert.Equal(t, "Heavy Go run started", response.Result.Text)
}

func TestHubTypeRegisterGlobalTopicStoresValidatedSchema(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	schemaPath := filepath.Join(t.TempDir(), "schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`), 0o600))
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"type", "register", "global:" + topic.ID, "job.started", "1", "--actor", "agent-a", "--generation", "1", "--schema-file", schemaPath, "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()
	var response hubJSONResponse[hubstore.EventSchema]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "global", response.ScopeKind)
	assert.Equal(t, topic.ID, response.Result.TopicID)
	assert.Equal(t, "job.started", response.Result.TypeKey)
}

func TestHubTopicArchiveGlobalAddressChangesPolicy(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "archive", "global:" + topic.ID, "--actor", "agent-a", "--generation", "1", "--reason", "Finished", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()
	var response hubJSONResponse[hubstore.Topic]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "global", response.ScopeKind)
	assert.Equal(t, "global", response.Result.ScopeKind)
	assert.Equal(t, "archived", response.Result.State)
}

func TestHubSubscribeListGlobalSelectorShowsGlobalSubscriptions(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollConsumer(context.Background(), db.AuthorityID(), "reader")
	require.NoError(t, err)
	sub, err := db.Subscribe(context.Background(), hubstore.GlobalScopeSelector, hubstore.SubscribeInput{
		AuthorityID: db.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{topic.ID},
	})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"subscribe", "list", "global:", "--consumer", "reader", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()
	var response hubJSONResponse[[]hubstore.Subscription]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "global", response.ScopeKind)
	require.Len(t, response.Result, 1)
	assert.Equal(t, sub.ID, response.Result[0].ID)
}

func TestHubTopicListGlobalSelectorRejectsExplicitRepo(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "list", "global:", "--repo", repo})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	// Act
	err := cmd.Execute()

	// Assert
	assert.ErrorContains(t, err, "--repo cannot be combined")
}

func TestHubTopicListRejectsUnknownScopeSelector(t *testing.T) {
	// Arrange
	_ = hubRepoFixture(t)
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "list", "workspace:"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	// Act
	err := cmd.Execute()

	// Assert
	assert.ErrorContains(t, err, "scope selector must be")
}

func TestHubTopicShowRejectsEmptyGlobalAddress(t *testing.T) {
	// Arrange
	_ = hubRepoFixture(t)
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "show", "global:"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	// Act
	err := cmd.Execute()

	// Assert
	assert.ErrorContains(t, err, "global: address requires one ID")
}

func TestHubRelatedIDGlobalAddressInGlobalScopeReturnsID(t *testing.T) {
	// Arrange
	address := "global:abc123"

	// Act
	id, err := hubRelatedID(hubstore.GlobalScopeSelector, address)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "abc123", id)
}

func TestHubRelatedIDGlobalAddressInRepoScopeRejects(t *testing.T) {
	// Arrange
	address := "global:abc123"

	// Act
	_, err := hubRelatedID(".", address)

	// Assert
	assert.ErrorContains(t, err, "global: ID requires a global: topic")
}

func TestHubRelatedIDNestedGlobalAddressRejects(t *testing.T) {
	// Arrange
	address := "global:global:abc123"

	// Act
	_, err := hubRelatedID(hubstore.GlobalScopeSelector, address)

	// Assert
	assert.ErrorContains(t, err, "global: address requires one ID")
}
