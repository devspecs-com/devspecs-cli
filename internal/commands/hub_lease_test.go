package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/hubstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHubLeaseAcquireGlobalTopicReturnsBearerToken(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"lease", "acquire", "global:" + topic.ID, "--actor", "agent-a", "--for", "1m", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()
	var response hubJSONResponse[hubstore.Lease]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "global", response.ScopeKind)
	assert.Equal(t, topic.ID, response.Result.TopicID)
	assert.Equal(t, "held", response.Result.State)
	assert.Len(t, response.Result.Token, 32)
}

func TestHubLeaseShowDoesNotRevealBearerToken(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	acquire := NewHubCmd()
	acquire.SetArgs([]string{"lease", "acquire", "global:" + topic.ID, "--actor", "agent-a", "--for", "1m"})
	acquire.SetOut(&bytes.Buffer{})
	require.NoError(t, acquire.Execute())
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"lease", "show", "global:" + topic.ID, "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()
	var response hubJSONResponse[hubstore.Lease]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "held", response.Result.State)
	assert.Equal(t, "agent-a", response.Result.ActorID)
	assert.Empty(t, response.Result.Token)
	assert.NotContains(t, out.String(), "token")
}

func TestHubLeaseAcquireWaitTimesOutOnHeldTopic(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	acquire := NewHubCmd()
	acquire.SetArgs([]string{"lease", "acquire", "global:" + topic.ID, "--actor", "agent-a", "--for", "1m"})
	acquire.SetOut(&bytes.Buffer{})
	require.NoError(t, acquire.Execute())
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"lease", "acquire", "global:" + topic.ID, "--actor", "agent-a", "--for", "1m", "--wait", "100ms"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	// Act
	err := cmd.Execute()

	// Assert
	assert.ErrorContains(t, err, "lease wait ended")
}

func TestHubLeaseAcquireWaitStopsWhenContextCancelled(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	acquire := NewHubCmd()
	acquire.SetArgs([]string{"lease", "acquire", "global:" + topic.ID, "--actor", "agent-a", "--for", "1m"})
	acquire.SetOut(&bytes.Buffer{})
	require.NoError(t, acquire.Execute())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := NewHubCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"lease", "acquire", "global:" + topic.ID, "--actor", "agent-a", "--wait", "1m"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	done := make(chan error, 1)

	// Act
	go func() { done <- cmd.Execute() }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	err := <-done

	// Assert
	assert.True(t, errors.Is(err, context.Canceled), "error: %v", err)
}

func TestHubLeaseReleaseWithAcquiredTokenMakesTopicAvailable(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	acquire := NewHubCmd()
	acquire.SetArgs([]string{"lease", "acquire", "global:" + topic.ID, "--actor", "agent-a", "--json"})
	var acquired bytes.Buffer
	acquire.SetOut(&acquired)
	require.NoError(t, acquire.Execute())
	var response hubJSONResponse[hubstore.Lease]
	require.NoError(t, json.Unmarshal(acquired.Bytes(), &response))
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"lease", "release", "global:" + topic.ID, "--actor", "agent-a", "--token", response.Result.Token, "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()
	var released hubJSONResponse[map[string]string]
	decodeErr := json.Unmarshal(out.Bytes(), &released)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "global", released.ScopeKind)
	assert.Equal(t, "available", released.Result["state"])
	assert.Equal(t, topic.ID, released.Result["topic_id"])
}

func TestHubLeaseRenewWithCurrentTokenExtendsDeadline(t *testing.T) {
	// Arrange
	_, topic := hubGlobalTopicFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	lease, err := db.AcquireLease(context.Background(), hubstore.GlobalScopeSelector, topic.ID, "agent-a", time.Minute)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"lease", "renew", "global:" + topic.ID, "--actor", "agent-a", "--token", lease.Token, "--for", "1h", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()
	var response hubJSONResponse[hubstore.Lease]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "global", response.ScopeKind)
	assert.Equal(t, "held", response.Result.State)
	assert.Equal(t, lease.Generation, response.Result.Generation)
	require.NotNil(t, response.Result.ExpiresAt)
	assert.True(t, response.Result.ExpiresAt.After(*lease.ExpiresAt))
	assert.Empty(t, response.Result.Token)
}
