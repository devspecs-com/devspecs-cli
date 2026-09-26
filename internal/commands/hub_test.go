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
	var topic hubstore.Topic
	require.NoError(t, json.Unmarshal(out.Bytes(), &topic))
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
	var archived hubstore.Topic
	require.NoError(t, json.Unmarshal(out.Bytes(), &archived))
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
	var message hubstore.Message
	require.NoError(t, json.Unmarshal(out.Bytes(), &message))
	assert.Equal(t, "Running the fat-25 gate", message.Text)
	assert.Equal(t, "agent-a", message.ActorID)
	assert.Equal(t, int64(1), message.Sequence)
	assert.NotEmpty(t, message.EntryID)
	assert.NotEmpty(t, message.MessageID)
}
