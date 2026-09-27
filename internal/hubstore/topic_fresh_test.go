package hubstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListTopicsUnboundRepoReturnsEmptyWithoutEnrollment(t *testing.T) {
	// Arrange
	db := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	repo := testRepo(t)

	// Act
	topics, err := db.ListTopics(context.Background(), repo, TopicList{})

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, topics)
	assert.Empty(t, topics)
	_, err = os.Stat(filepath.Join(repo, ".git", markerFile))
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = db.LookupRepo(context.Background(), repo)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestListTopicsInvalidRepoFails(t *testing.T) {
	// Arrange
	db := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	invalidRepo := filepath.Join(t.TempDir(), "missing")

	// Act
	topics, err := db.ListTopics(context.Background(), invalidRepo, TopicList{})

	// Assert
	assert.Error(t, err)
	assert.Nil(t, topics)
}

func TestListTopicsMissingBoundMarkerFailsWithoutRecreatingIt(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db := testDB(t, filepath.Join(t.TempDir(), "home"), nil)
	repo := testRepo(t)
	_, err := db.EnrollRepo(ctx, repo)
	require.NoError(t, err)
	marker := filepath.Join(repo, ".git", markerFile)
	require.NoError(t, os.Remove(marker))

	// Act
	topics, err := db.ListTopics(ctx, repo, TopicList{})

	// Assert
	assert.ErrorIs(t, err, ErrBindingConflict)
	assert.Nil(t, topics)
	_, statErr := os.Stat(marker)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestValidateTopicListRepoRejectsMalformedMarker(t *testing.T) {
	// Arrange
	repo := testRepo(t)
	marker := filepath.Join(repo, ".git", markerFile)
	require.NoError(t, os.WriteFile(marker, []byte("invalid\n"), 0o600))

	// Act
	err := ValidateTopicListRepo(context.Background(), repo)

	// Assert
	assert.ErrorIs(t, err, ErrBindingConflict)
	data, readErr := os.ReadFile(marker)
	require.NoError(t, readErr)
	assert.Equal(t, "invalid\n", string(data))
}
