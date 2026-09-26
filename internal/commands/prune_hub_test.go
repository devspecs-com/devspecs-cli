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

func TestPruneHub_WithoutCutoff_RejectsBeforeOpeningStorage(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	cmd := NewPruneCmd()
	cmd.SetArgs([]string{"--hub"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--before")
	_, statErr := os.Stat(filepath.Join(filepath.Dir(repo), "home", "hub.sqlite"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestPruneHub_DryRun_ReportsEligibleOldRevision(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	now := time.Now().UTC().Truncate(time.Second).Add(-48 * time.Hour)
	db, err := hubstore.Open(context.Background(), hubstore.Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	_, err = db.EnrollActor(context.Background(), "agent-a")
	require.NoError(t, err)
	_, err = db.EnrollRepo(context.Background(), repo)
	require.NoError(t, err)
	topic, err := db.CreateTopic(context.Background(), repo, "agent-a", hubstore.TopicInput{Key: "local-go", Name: "Local Go", Description: "Coordinate expensive local Go jobs"})
	require.NoError(t, err)
	first, err := db.PostMessage(context.Background(), repo, topic.ID, "agent-a", hubstore.MessageInput{AuthorityID: db.AuthorityID(), Text: "Starting fat-25"})
	require.NoError(t, err)
	now = now.Add(36 * time.Hour)
	_, err = db.ReviseMessage(context.Background(), repo, topic.ID, first.MessageID, "agent-a", hubstore.MessageRevisionInput{MessageInput: hubstore.MessageInput{AuthorityID: db.AuthorityID(), Text: "Finished fat-25"}, ExpectedRevision: 1})
	require.NoError(t, err)
	require.NoError(t, db.Close())
	cutoff := time.Now().UTC().Truncate(time.Second).Add(-24 * time.Hour)
	cmd := NewPruneCmd()
	cmd.SetArgs([]string{"--hub", "--before", cutoff.Format(time.RFC3339), "--dry-run", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()

	// Assert
	require.NoError(t, err)
	var response hubJSONResponse[hubstore.PruneReport]
	require.NoError(t, json.Unmarshal(out.Bytes(), &response))
	assert.True(t, response.Result.DryRun)
	assert.Equal(t, 1, response.Result.Entries)
	assert.Empty(t, response.Result.BackupPath)
}
