package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/hubstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hubRepoFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(root, "home"))
	repo := filepath.Join(root, "repo")
	require.NoError(t, os.Mkdir(repo, 0o700))
	init := exec.Command("git", "init", "-q", repo)
	require.NoError(t, init.Run())
	return repo
}

func TestHubTopicCreate_WithEnrolledActor_PrintsExactTopic(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollActor(context.Background(), "agent-a")
	require.NoError(t, err)
	require.NoError(t, db.Close())
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "create", "local-go", "--repo", repo, "--actor", "agent-a", "--name", "Local Go", "--description", "Coordinate expensive local Go jobs", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()

	// Assert
	require.NoError(t, err)
	var response hubJSONResponse[hubstore.Topic]
	require.NoError(t, json.Unmarshal(out.Bytes(), &response))
	topic := response.Result
	assert.Equal(t, "devspecs.hub/v1", response.ContractVersion)
	assert.Equal(t, "local-go", topic.Key)
	assert.Equal(t, "Local Go", topic.Name)
	assert.Equal(t, "agent-a", topic.OwnerActorID)
	assert.NotEmpty(t, topic.ID)
}

func TestHubTopicList_WithoutHub_DoesNotCreateAuthority(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	home := os.Getenv("DEVSPECS_HOME")
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "list", "--repo", repo, "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()

	// Assert
	assert.ErrorIs(t, err, hubstore.ErrNotFound)
	_, statErr := os.Stat(filepath.Join(home, "hub.sqlite"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHubTopicArchive_WithCurrentGeneration_ReportsArchived(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollActor(context.Background(), "agent-a")
	require.NoError(t, err)
	_, err = db.EnrollRepo(context.Background(), repo)
	require.NoError(t, err)
	topic, err := db.CreateTopic(context.Background(), repo, "agent-a", hubstore.TopicInput{Key: "local-go", Name: "Local Go", Description: "Coordinate expensive local Go jobs"})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "archive", topic.ID, "--repo", repo, "--actor", "agent-a", "--generation", "1", "--reason", "Finished", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()

	// Assert
	require.NoError(t, err)
	var response hubJSONResponse[hubstore.Topic]
	require.NoError(t, json.Unmarshal(out.Bytes(), &response))
	archived := response.Result
	assert.Equal(t, topic.ID, archived.ID)
	assert.Equal(t, "archived", archived.State)
	assert.Equal(t, int64(2), archived.PolicyGeneration)
}

func TestHubMessagePost_WithIdempotencyKey_ReturnsPublicationIdentity(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollActor(context.Background(), "agent-a")
	require.NoError(t, err)
	_, err = db.EnrollRepo(context.Background(), repo)
	require.NoError(t, err)
	topic, err := db.CreateTopic(context.Background(), repo, "agent-a", hubstore.TopicInput{Key: "local-go", Name: "Local Go", Description: "Coordinate expensive local Go jobs"})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"message", "post", topic.ID, "--repo", repo, "--actor", "agent-a", "--text", "Running the fat-25 gate", "--key", "fat-25-run-1", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()

	// Assert
	require.NoError(t, err)
	var response hubJSONResponse[hubstore.Message]
	require.NoError(t, json.Unmarshal(out.Bytes(), &response))
	message := response.Result
	assert.Equal(t, "Running the fat-25 gate", message.Text)
	assert.Equal(t, "agent-a", message.ActorID)
	assert.Equal(t, int64(1), message.Sequence)
	assert.NotEmpty(t, message.EntryID)
	assert.NotEmpty(t, message.MessageID)
}

func TestHubMessageRevise_WithCurrentRevision_AppendsCorrection(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollActor(context.Background(), "agent-a")
	require.NoError(t, err)
	_, err = db.EnrollRepo(context.Background(), repo)
	require.NoError(t, err)
	topic, err := db.CreateTopic(context.Background(), repo, "agent-a", hubstore.TopicInput{Key: "local-go", Name: "Local Go", Description: "Coordinate expensive local Go jobs"})
	require.NoError(t, err)
	first, err := db.PostMessage(context.Background(), repo, topic.ID, "agent-a", hubstore.MessageInput{AuthorityID: db.AuthorityID(), Text: "Starting fat-25"})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"message", "revise", topic.ID, first.MessageID, "--repo", repo, "--actor", "agent-a", "--revision", "1", "--text", "Finished fat-25", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()

	// Assert
	require.NoError(t, err)
	var response hubJSONResponse[hubstore.Message]
	require.NoError(t, json.Unmarshal(out.Bytes(), &response))
	revised := response.Result
	assert.Equal(t, first.MessageID, revised.MessageID)
	assert.Equal(t, int64(2), revised.Revision)
	assert.Equal(t, "Finished fat-25", revised.Text)
}

func TestHubTypeRegister_WithOwnerAndSchemaFile_ReturnsDigest(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollActor(context.Background(), "agent-a")
	require.NoError(t, err)
	_, err = db.EnrollRepo(context.Background(), repo)
	require.NoError(t, err)
	topic, err := db.CreateTopic(context.Background(), repo, "agent-a", hubstore.TopicInput{Key: "local-go", Name: "Local Go", Description: "Coordinate expensive local Go jobs"})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	schemaPath := filepath.Join(t.TempDir(), "job.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"job":{"type":"string"}},"required":["job"],"additionalProperties":false}`), 0o600))
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"type", "register", topic.ID, "job.finished", "1", "--repo", repo, "--actor", "agent-a", "--generation", "1", "--schema-file", schemaPath, "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()

	// Assert
	require.NoError(t, err)
	var response hubJSONResponse[hubstore.EventSchema]
	require.NoError(t, json.Unmarshal(out.Bytes(), &response))
	assert.Equal(t, "job.finished", response.Result.TypeKey)
	assert.Equal(t, int64(1), response.Result.Version)
	assert.NotEmpty(t, response.Result.SHA256)
}

func TestHubEventPublish_WithValidPayload_ReturnsOrderedEntry(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollActor(context.Background(), "agent-a")
	require.NoError(t, err)
	_, err = db.EnrollRepo(context.Background(), repo)
	require.NoError(t, err)
	topic, err := db.CreateTopic(context.Background(), repo, "agent-a", hubstore.TopicInput{Key: "local-go", Name: "Local Go", Description: "Coordinate expensive local Go jobs"})
	require.NoError(t, err)
	_, err = db.RegisterEventSchema(context.Background(), repo, topic.ID, "agent-a", topic.PolicyGeneration, "job.finished", 1, []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"job":{"type":"string"}},"required":["job"],"additionalProperties":false}`))
	require.NoError(t, err)
	require.NoError(t, db.Close())
	payloadPath := filepath.Join(t.TempDir(), "job.json")
	require.NoError(t, os.WriteFile(payloadPath, []byte(`{"job":"fat-25"}`), 0o600))
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"event", "publish", topic.ID, "--repo", repo, "--actor", "agent-a", "--type", "job.finished", "--version", "1", "--payload-file", payloadPath, "--key", "fat-25-run-1", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()

	// Assert
	require.NoError(t, err)
	var response hubJSONResponse[hubstore.Event]
	require.NoError(t, json.Unmarshal(out.Bytes(), &response))
	assert.Equal(t, "event", response.Result.Kind)
	assert.Equal(t, "job.finished", response.Result.TypeKey)
	assert.Equal(t, int64(1), response.Result.Sequence)
	assert.NotEmpty(t, response.Result.EntryID)
}
