package hubstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func topicFixture(t *testing.T, now func() time.Time) (*DB, string) {
	t.Helper()
	ctx := context.Background()
	repo := testRepo(t)
	d := testDB(t, filepath.Join(t.TempDir(), "home"), now)
	_, err := d.EnrollRepo(ctx, repo)
	require.NoError(t, err)
	_, err = d.EnrollActor(ctx, "owner")
	require.NoError(t, err)
	return d, repo
}

func TestCreateTopicTrimsText(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo := topicFixture(t, nil)

	// Act
	created, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "release-plan", Name: "  Release Plan  ", Description: "  Coordinate rollout  "})
	require.NoError(t, err)

	// Assert
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "Release Plan", created.Name)
	assert.Equal(t, "Coordinate rollout", created.Description)
	assert.Equal(t, "active", created.State)
}

func TestListTopicsFindsDescriptionMatch(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo := topicFixture(t, nil)
	created, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "release-plan", Name: "Release Plan", Description: "Coordinate rollout"})
	require.NoError(t, err)

	// Act
	listed, err := d.ListTopics(ctx, repo, TopicList{Query: "rollout"})
	require.NoError(t, err)

	// Assert
	require.Len(t, listed, 1)
	assert.Equal(t, created.ID, listed[0].ID)
	assert.Equal(t, created.ScopeID, listed[0].ScopeID)
}

func TestShowTopicReturnsCreatedTopic(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo := topicFixture(t, nil)
	created, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "release-plan", Name: "Release Plan", Description: "Coordinate rollout"})
	require.NoError(t, err)

	// Act
	shown, err := d.ShowTopic(ctx, repo, created.ID)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, created.ID, shown.ID)
	assert.Equal(t, "active", shown.State)
}

func TestDuplicateKeyStaysReservedAfterArchive(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo := topicFixture(t, nil)
	first, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "handoff", Name: "Handoff", Description: "Release handoff"})
	require.NoError(t, err)
	_, err = d.ArchiveTopic(ctx, repo, first.ID, "owner", first.PolicyGeneration, "Finished")
	require.NoError(t, err)

	// Act
	_, err = d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "handoff", Name: "Another", Description: "Another handoff"})

	// Assert
	assert.ErrorIs(t, err, ErrConflict)
}

func TestDirectKeyMutationCannotChangeTopicIdentity(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo := topicFixture(t, nil)
	topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "fixed", Name: "Fixed", Description: "Identity"})
	require.NoError(t, err)

	// Act
	_, err = d.sql.ExecContext(ctx, "UPDATE topics SET key='changed' WHERE topic_id=?", topic.ID)

	// Assert
	assert.Error(t, err)
	var key string
	require.NoError(t, d.sql.QueryRowContext(ctx, "SELECT key FROM topics WHERE topic_id=?", topic.ID).Scan(&key))
	assert.Equal(t, "fixed", key)
}

func expiredTopicFixture(t *testing.T) (*DB, string, Topic, time.Time) {
	t.Helper()
	ctx := context.Background()
	current := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	d, repo := topicFixture(t, func() time.Time { return current })
	deadline := current.Add(10 * time.Millisecond)
	topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "briefing", Name: "Briefing", Description: "Prepare notes", ExpiresAt: &deadline})
	require.NoError(t, err)
	current = deadline
	return d, repo, topic, deadline
}

func TestListTopicsExcludesExpiredTopic(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo, _, _ := expiredTopicFixture(t)

	// Act
	active, err := d.ListTopics(ctx, repo, TopicList{})
	require.NoError(t, err)

	// Assert
	assert.Empty(t, active)
}

func TestListTopicsIncludesExpiredTopicWhenRequested(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo, topic, _ := expiredTopicFixture(t)

	// Act
	archived, err := d.ListTopics(ctx, repo, TopicList{IncludeArchived: true})
	require.NoError(t, err)

	// Assert
	require.Len(t, archived, 1)
	assert.Equal(t, topic.ID, archived[0].ID)
	assert.Equal(t, "archived", archived[0].State)
}

func TestShowTopicReportsExpiryAtDeadline(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo, topic, deadline := expiredTopicFixture(t)

	// Act
	shown, err := d.ShowTopic(ctx, repo, topic.ID)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, "archived", shown.State)
	assert.Equal(t, "expired", shown.EffectiveReason)
	assert.Nil(t, shown.ArchivedAt)
	require.NotNil(t, shown.EffectiveArchivedAt)
	assert.Equal(t, deadline, *shown.EffectiveArchivedAt)
}

func TestExpiredRestoreRequiresDeadlineChange(t *testing.T) {
	// Arrange
	ctx := context.Background()
	current := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	d, repo := topicFixture(t, func() time.Time { return current })
	deadline := current.Add(time.Second)
	topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "briefing", Name: "Briefing", Description: "Prepare notes", ExpiresAt: &deadline})
	require.NoError(t, err)
	current = deadline

	// Act
	_, err = d.RestoreTopic(ctx, repo, topic.ID, "owner", topic.PolicyGeneration, "Resume", false, nil)

	// Assert
	assert.ErrorIs(t, err, ErrExpired)
}

func TestRestoreClearsElapsedDeadline(t *testing.T) {
	// Arrange
	ctx := context.Background()
	current := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	d, repo := topicFixture(t, func() time.Time { return current })
	deadline := current.Add(time.Second)
	topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "briefing", Name: "Briefing", Description: "Prepare notes", ExpiresAt: &deadline})
	require.NoError(t, err)
	current = deadline

	// Act
	restored, err := d.RestoreTopic(ctx, repo, topic.ID, "owner", topic.PolicyGeneration, "Continue", true, nil)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, "active", restored.State)
	assert.Nil(t, restored.ExpiresAt)
	assert.Equal(t, topic.Key, restored.Key)
	assert.Equal(t, topic.PolicyGeneration+1, restored.PolicyGeneration)
}

func TestMaintainerEditsMetadata(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo := topicFixture(t, nil)
	_, err := d.EnrollActor(ctx, "editor")
	require.NoError(t, err)
	topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "notes", Name: "Notes", Description: "Original"})
	require.NoError(t, err)
	granted, err := d.SetMaintainer(ctx, repo, topic.ID, "owner", "editor", topic.PolicyGeneration, true)
	require.NoError(t, err)

	// Act
	edited, err := d.EditTopic(ctx, repo, topic.ID, "editor", TopicEdit{Name: "Updated notes", ExpectedGeneration: granted.PolicyGeneration})
	require.NoError(t, err)

	// Assert
	assert.Equal(t, "Updated notes", edited.Name)
	assert.Equal(t, "Original", edited.Description)
	assert.Equal(t, topic.ID, edited.ID)
	assert.Equal(t, topic.Key, edited.Key)
}

func TestMaintainerCannotDelegate(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo := topicFixture(t, nil)
	_, err := d.EnrollActor(ctx, "editor")
	require.NoError(t, err)
	_, err = d.EnrollActor(ctx, "another")
	require.NoError(t, err)
	topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "notes", Name: "Notes", Description: "Original"})
	require.NoError(t, err)
	granted, err := d.SetMaintainer(ctx, repo, topic.ID, "owner", "editor", topic.PolicyGeneration, true)
	require.NoError(t, err)

	// Act
	_, err = d.SetMaintainer(ctx, repo, topic.ID, "editor", "another", granted.PolicyGeneration, true)

	// Assert
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestStaleEditRejectsWithoutChangingTopic(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo := topicFixture(t, nil)
	topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "notes", Name: "Notes", Description: "Original"})
	require.NoError(t, err)
	_, err = d.EditTopic(ctx, repo, topic.ID, "owner", TopicEdit{Name: "First", ExpectedGeneration: topic.PolicyGeneration})
	require.NoError(t, err)

	// Act
	_, err = d.EditTopic(ctx, repo, topic.ID, "owner", TopicEdit{Name: "Second", ExpectedGeneration: topic.PolicyGeneration})

	// Assert
	assert.ErrorIs(t, err, ErrConflict)
	var name string
	require.NoError(t, d.sql.QueryRowContext(ctx, "SELECT name FROM topics WHERE topic_id=?", topic.ID).Scan(&name))
	assert.Equal(t, "First", name)
}

func TestArchiveTopicPreservesIdentity(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo := topicFixture(t, nil)
	topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "notes", Name: "Notes", Description: "Original"})
	require.NoError(t, err)

	// Act
	archived, err := d.ArchiveTopic(ctx, repo, topic.ID, "owner", topic.PolicyGeneration, "Closed")
	require.NoError(t, err)

	// Assert
	assert.Equal(t, "archived", archived.State)
	assert.Equal(t, "Closed", archived.ArchiveReason)
}

func TestRestoreTopicPreservesIdentity(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo := topicFixture(t, nil)
	topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "notes", Name: "Notes", Description: "Original"})
	require.NoError(t, err)
	archived, err := d.ArchiveTopic(ctx, repo, topic.ID, "owner", topic.PolicyGeneration, "Closed")
	require.NoError(t, err)

	// Act
	restored, err := d.RestoreTopic(ctx, repo, topic.ID, "owner", archived.PolicyGeneration, "Reopened", false, nil)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, "active", restored.State)
	assert.Equal(t, topic.ID, restored.ID)
	assert.Equal(t, topic.Key, restored.Key)
}

func TestArchiveRestoreAuditRemainsInspectable(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo := topicFixture(t, nil)
	topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "audit", Name: "Audit", Description: "Lifecycle"})
	require.NoError(t, err)
	archived, err := d.ArchiveTopic(ctx, repo, topic.ID, "owner", topic.PolicyGeneration, "Completed")
	require.NoError(t, err)
	_, err = d.RestoreTopic(ctx, repo, topic.ID, "owner", archived.PolicyGeneration, "Reopened", false, nil)
	require.NoError(t, err)

	// Act
	audit, err := d.ListTopicAudit(ctx, repo, topic.ID, 10, 0)
	require.NoError(t, err)

	// Assert
	require.Len(t, audit, 3)
	assert.Equal(t, "create", audit[0].Action)
	assert.Equal(t, "archive", audit[1].Action)
	assert.Equal(t, "restore", audit[2].Action)
	assert.Contains(t, string(audit[1].NewValue), "Completed")
	assert.Contains(t, string(audit[2].NewValue), "Reopened")
}

func TestMaintainerInspectionReturnsGrantedActor(t *testing.T) {
	// Arrange
	ctx := context.Background()
	d, repo := topicFixture(t, nil)
	_, err := d.EnrollActor(ctx, "editor")
	require.NoError(t, err)
	topic, err := d.CreateTopic(ctx, repo, "owner", TopicInput{Key: "roles", Name: "Roles", Description: "Team"})
	require.NoError(t, err)
	_, err = d.SetMaintainer(ctx, repo, topic.ID, "owner", "editor", topic.PolicyGeneration, true)
	require.NoError(t, err)

	// Act
	maintainers, err := d.ListMaintainers(ctx, repo, topic.ID, 10, 0)
	require.NoError(t, err)

	// Assert
	require.Len(t, maintainers, 1)
	assert.Equal(t, "editor", maintainers[0])
}
