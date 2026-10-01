package hubstore

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enrollTestConsumer(t *testing.T, f messageFixture, id string) Consumer {
	t.Helper()
	c, err := f.d.EnrollConsumer(context.Background(), f.d.AuthorityID(), id)
	require.NoError(t, err)
	return c
}

func subscribeTest(t *testing.T, f messageFixture, consumer string, fromBeginning bool, kinds []string, types []EventTypeFilter) Subscription {
	t.Helper()
	s, err := f.d.Subscribe(context.Background(), f.repo, SubscribeInput{AuthorityID: f.d.AuthorityID(), ConsumerID: consumer, TopicIDs: []string{f.topic.ID}, Kinds: kinds, EventTypes: types, FromBeginning: fromBeginning})
	require.NoError(t, err)
	return s
}

func TestPullMixedPublicationOrderAndFilteredGap(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, []string{"event", "message"}, nil)
	first := f.post(t, "first", "one", nil)
	event, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3}`, "job"))
	require.NoError(t, err)
	third := f.post(t, "third", "three", nil)

	// Act
	page, err := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 3)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(3), page.HighWater)
	assert.Equal(t, int64(3), page.NextScanPosition)
	require.Len(t, page.Entries, 3)
	require.NotNil(t, page.Entries[0].Message)
	assert.Equal(t, first.EntryID, page.Entries[0].Message.EntryID)
	require.NotNil(t, page.Entries[1].Event)
	assert.Equal(t, event.EntryID, page.Entries[1].Event.EntryID)
	require.NotNil(t, page.Entries[2].Message)
	assert.Equal(t, third.EntryID, page.Entries[2].Message.EntryID)
	assert.Empty(t, page.Gaps)
}

func TestFilteredEmptyPageProposesScanAndAck(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, []string{"event"}, nil)
	f.post(t, "not an event", "one", nil)

	// Act
	page, err := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 1)
	require.NoError(t, err)
	ack, ackErr := f.d.Ack(context.Background(), f.repo, "reader", s.ID, f.d.AuthorityID(), 0, page.NextScanPosition, page.AckToken)

	// Assert
	require.NoError(t, ackErr)
	assert.Equal(t, int64(1), ack)
	assert.Empty(t, page.Entries)
	require.Len(t, page.Gaps, 1)
	assert.Equal(t, "filtered", page.Gaps[0].Reason)
	assert.Equal(t, int64(1), page.Gaps[0].From)
}

func TestPullBeforeAckReplaysAndFabricatedCursorFails(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	posted := f.post(t, "durable", "one", nil)
	first, err := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 1)
	require.NoError(t, err)
	reopened, err := Open(context.Background(), Options{Home: filepath.Dir(f.d.Path())})
	require.NoError(t, err)
	defer reopened.Close()

	// Act
	replay, pullErr := reopened.Pull(context.Background(), f.repo, "reader", s.ID, 1)
	_, forgedErr := reopened.Ack(context.Background(), f.repo, "reader", s.ID, reopened.AuthorityID(), 0, 2, first.AckToken)

	// Assert
	require.NoError(t, pullErr)
	require.Len(t, replay.Entries, 1)
	require.NotNil(t, replay.Entries[0].Message)
	assert.Equal(t, posted.EntryID, replay.Entries[0].Message.EntryID)
	assert.ErrorIs(t, forgedErr, ErrCursorConflict)
}

func TestAckCASAndRetry(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	f.post(t, "first", "one", nil)
	page, err := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 1)
	require.NoError(t, err)

	// Act
	ack, err := f.d.Ack(context.Background(), f.repo, "reader", s.ID, f.d.AuthorityID(), 0, 1, page.AckToken)
	require.NoError(t, err)
	retry, retryErr := f.d.Ack(context.Background(), f.repo, "reader", s.ID, f.d.AuthorityID(), 0, 1, page.AckToken)

	// Assert
	require.NoError(t, retryErr)
	assert.Equal(t, int64(1), ack)
	assert.Equal(t, int64(1), retry)
	second, err := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 1)
	require.NoError(t, err)
	assert.Empty(t, second.Entries)
	assert.Equal(t, int64(1), second.PriorAcknowledged)
}

func TestIndependentConsumersAndSubscriptions(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "one")
	enrollTestConsumer(t, f, "two")
	first := subscribeTest(t, f, "one", true, nil, nil)
	second := subscribeTest(t, f, "two", true, nil, nil)
	parallel := subscribeTest(t, f, "one", true, nil, nil)
	f.post(t, "shared", "one", nil)
	page, err := f.d.Pull(context.Background(), f.repo, "one", first.ID, 1)
	require.NoError(t, err)
	_, err = f.d.Ack(context.Background(), f.repo, "one", first.ID, f.d.AuthorityID(), 0, 1, page.AckToken)
	require.NoError(t, err)

	// Act
	other, otherErr := f.d.Pull(context.Background(), f.repo, "two", second.ID, 1)
	parallelPage, parallelErr := f.d.Pull(context.Background(), f.repo, "one", parallel.ID, 1)

	// Assert
	require.NoError(t, otherErr)
	require.NoError(t, parallelErr)
	require.Len(t, other.Entries, 1)
	require.Len(t, parallelPage.Entries, 1)
	assert.NotEqual(t, other.AckToken, parallelPage.AckToken)
	assert.NotEqual(t, page.AckToken, parallelPage.AckToken)
	assert.Equal(t, int64(0), other.PriorAcknowledged)
	assert.Equal(t, int64(0), parallelPage.PriorAcknowledged)
}

func TestDefaultStartAndConcurrentPostBetweenPages(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	f.post(t, "before", "before", nil)
	s := subscribeTest(t, f, "reader", false, nil, nil)
	second := f.post(t, "second", "second", nil)
	firstPage, err := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 1)
	require.NoError(t, err)
	third := f.post(t, "third", "third", nil)
	_, err = f.d.Ack(context.Background(), f.repo, "reader", s.ID, f.d.AuthorityID(), 1, 2, firstPage.AckToken)
	require.NoError(t, err)

	// Act
	secondPage, pullErr := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 1)

	// Assert
	require.NoError(t, pullErr)
	assert.Equal(t, int64(1), s.AcknowledgedSequence)
	require.Len(t, firstPage.Entries, 1)
	require.NotNil(t, firstPage.Entries[0].Message)
	assert.Equal(t, second.EntryID, firstPage.Entries[0].Message.EntryID)
	assert.Equal(t, int64(2), firstPage.HighWater)
	require.Len(t, secondPage.Entries, 1)
	require.NotNil(t, secondPage.Entries[0].Message)
	assert.Equal(t, third.EntryID, secondPage.Entries[0].Message.EntryID)
}

func TestExpiryGapAndHistoricalRead(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	expiry := f.now.Add(time.Second)
	m := f.post(t, "short lived", "short", &expiry)
	*f.now = expiry

	// Act
	page, err := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 1)
	history, historyErr := f.d.ReadPublication(context.Background(), f.repo, m.EntryID, true)

	// Assert
	require.NoError(t, err)
	require.NoError(t, historyErr)
	assert.Equal(t, m.EntryID, history.EntryID)
	assert.Empty(t, page.Entries)
	require.Len(t, page.Gaps, 1)
	assert.Equal(t, "expired", page.Gaps[0].Reason)
}

func TestArchiveGapAndRemovePreservesHistory(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	m := f.post(t, "retained", "one", nil)
	_, err := f.d.ArchiveTopic(context.Background(), f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "done")
	require.NoError(t, err)

	// Act
	page, pullErr := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 1)
	removed, removeErr := f.d.RemoveSubscription(context.Background(), f.repo, "reader", s.ID, f.d.AuthorityID())
	history, historyErr := f.d.ReadPublication(context.Background(), f.repo, m.EntryID, true)

	// Assert
	require.NoError(t, pullErr)
	require.NoError(t, removeErr)
	require.NoError(t, historyErr)
	assert.Equal(t, m.EntryID, history.EntryID)
	assert.NotNil(t, removed.RemovedAt)
	assert.Empty(t, page.Entries)
	require.Len(t, page.Gaps, 1)
	assert.Equal(t, "archived", page.Gaps[0].Reason)
}

func TestEventFilterUnknownVersion(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	enrollTestConsumer(t, f, "reader")

	// Act
	_, err := f.d.Subscribe(context.Background(), f.repo, SubscribeInput{AuthorityID: f.d.AuthorityID(), ConsumerID: "reader", TopicIDs: []string{f.topic.ID}, Kinds: []string{"event"}, EventTypes: []EventTypeFilter{{TypeKey: "job.finished", Version: 2}}})

	// Assert
	assert.ErrorIs(t, err, ErrVersionUnknown)
}

func TestEventTypeFilterKeepsOnlySelectedVersion(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	registerJob(t, f)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, []string{"event"}, []EventTypeFilter{{TypeKey: "job.finished", Version: 1}})
	f.post(t, "filtered message", "message", nil)
	e, err := f.d.PublishEvent(context.Background(), f.repo, f.topic.ID, "author", jobInput(f, `{"job":"build","duration":3}`, "event"))
	require.NoError(t, err)

	// Act
	page, pullErr := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 2)

	// Assert
	require.NoError(t, pullErr)
	require.Len(t, page.Entries, 1)
	require.NotNil(t, page.Entries[0].Event)
	assert.Equal(t, e.EntryID, page.Entries[0].Event.EntryID)
	require.Len(t, page.Gaps, 1)
	assert.Equal(t, "filtered", page.Gaps[0].Reason)
	assert.Equal(t, int64(1), page.Gaps[0].From)
}

func TestListAndRemoveSubscription(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	_, err := f.d.RemoveSubscription(context.Background(), f.repo, "reader", s.ID, f.d.AuthorityID())
	require.NoError(t, err)

	// Act
	active, activeErr := f.d.ListSubscriptions(context.Background(), f.repo, "reader", false)
	all, allErr := f.d.ListSubscriptions(context.Background(), f.repo, "reader", true)

	// Assert
	require.NoError(t, activeErr)
	require.NoError(t, allErr)
	assert.Empty(t, active)
	require.Len(t, all, 1)
	assert.Equal(t, s.ID, all[0].ID)
	assert.NotNil(t, all[0].RemovedAt)
}

func TestPullAckChildProcess(t *testing.T) {
	if os.Getenv("HUB_B05_CHILD") == "1" {
		// Arrange
		ctx := context.Background()
		d, err := Open(ctx, Options{Home: os.Getenv("HUB_B05_HOME")})
		require.NoError(t, err)
		defer d.Close()
		// Act
		page, err := d.Pull(ctx, os.Getenv("HUB_B05_REPO"), "reader", os.Getenv("HUB_B05_SUB"), 1)
		require.NoError(t, err)
		require.Len(t, page.Entries, 1)
		_, err = d.Ack(ctx, os.Getenv("HUB_B05_REPO"), "reader", os.Getenv("HUB_B05_SUB"), d.AuthorityID(), 0, 1, page.AckToken)
		// Assert
		require.NoError(t, err)
		return
	}
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	f.post(t, "across process", "one", nil)
	cmd := exec.Command(os.Args[0], "-test.run=^TestPullAckChildProcess$")
	cmd.Env = append(os.Environ(), "HUB_B05_CHILD=1", "HUB_B05_HOME="+filepath.Dir(f.d.Path()), "HUB_B05_REPO="+f.repo, "HUB_B05_SUB="+s.ID)

	// Act
	out, err := cmd.CombinedOutput()
	page, pullErr := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 1)

	// Assert
	require.NoError(t, err, string(out))
	require.NoError(t, pullErr)
	assert.Equal(t, int64(1), page.PriorAcknowledged)
	assert.Empty(t, page.Entries)
}

func TestAckRejectsStaleTokenAfterOtherPage(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	f.post(t, "first", "one", nil)
	f.post(t, "second", "two", nil)
	short, err := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 1)
	require.NoError(t, err)
	long, err := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 2)
	require.NoError(t, err)
	_, err = f.d.Ack(context.Background(), f.repo, "reader", s.ID, f.d.AuthorityID(), 0, 1, short.AckToken)
	require.NoError(t, err)

	// Act
	_, staleErr := f.d.Ack(context.Background(), f.repo, "reader", s.ID, f.d.AuthorityID(), 0, 2, long.AckToken)

	// Assert
	assert.True(t, errors.Is(staleErr, ErrCursorConflict))
}

func TestAckAfterRetentionEpochChangeReportsReplayGap(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	f.post(t, "retained", "one", nil)
	page, err := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 1)
	require.NoError(t, err)
	_, err = f.d.sql.Exec("UPDATE hub_meta SET retention_epoch=retention_epoch+1 WHERE singleton=1")
	require.NoError(t, err)

	// Act
	_, ackErr := f.d.Ack(context.Background(), f.repo, "reader", s.ID, f.d.AuthorityID(), 0, 1, page.AckToken)

	// Assert
	assert.ErrorIs(t, ackErr, ErrReplayGap)
	var ack int64
	require.NoError(t, f.d.sql.QueryRow("SELECT acknowledged_sequence FROM subscriptions WHERE subscription_id=?", s.ID).Scan(&ack))
	assert.Zero(t, ack)
}

func TestAckRejectsTamperedToken(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	f.post(t, "retained", "one", nil)
	page, err := f.d.Pull(context.Background(), f.repo, "reader", s.ID, 1)
	require.NoError(t, err)
	token := "A" + page.AckToken[1:]
	if token == page.AckToken {
		token = "B" + page.AckToken[1:]
	}

	// Act
	_, ackErr := f.d.Ack(context.Background(), f.repo, "reader", s.ID, f.d.AuthorityID(), 0, 1, token)

	// Assert
	assert.ErrorIs(t, ackErr, ErrCursorConflict)
}

func TestReadOnlyPullUsesMigratedTokenSecret(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	m := f.post(t, "read only", "one", nil)
	reader, err := OpenReadOnly(context.Background(), Options{Home: filepath.Dir(f.d.Path())})
	require.NoError(t, err)
	defer reader.Close()

	// Act
	page, pullErr := reader.Pull(context.Background(), f.repo, "reader", s.ID, 1)
	require.NoError(t, pullErr)
	ack, ackErr := f.d.Ack(context.Background(), f.repo, "reader", s.ID, f.d.AuthorityID(), 0, 1, page.AckToken)

	// Assert
	require.NoError(t, ackErr)
	require.Len(t, page.Entries, 1)
	require.NotNil(t, page.Entries[0].Message)
	assert.Equal(t, m.EntryID, page.Entries[0].Message.EntryID)
	assert.Equal(t, int64(1), ack)
	var secretLength int
	require.NoError(t, f.d.sql.QueryRow("SELECT length(secret) FROM pull_token_secret WHERE singleton=1").Scan(&secretLength))
	assert.Equal(t, 32, secretLength)
}

func TestLinkedWorktreeSharesSubscriptionCursor(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(f.repo, "README"), []byte("fixture\n"), 0o600))
	out, err := exec.Command("git", "-C", f.repo, "add", "README").CombinedOutput()
	require.NoError(t, err, string(out))
	out, err = exec.Command("git", "-C", f.repo, "-c", "user.name=Hub Test", "-c", "user.email=hub@example.invalid", "commit", "-qm", "fixture").CombinedOutput()
	require.NoError(t, err, string(out))
	linked := filepath.Join(t.TempDir(), "linked")
	out, err = exec.Command("git", "-C", f.repo, "worktree", "add", "--detach", linked).CombinedOutput()
	require.NoError(t, err, string(out))
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	m := f.post(t, "from common scope", "one", nil)

	// Act
	page, pullErr := f.d.Pull(context.Background(), linked, "reader", s.ID, 1)
	require.NoError(t, pullErr)
	ack, ackErr := f.d.Ack(context.Background(), linked, "reader", s.ID, f.d.AuthorityID(), 0, 1, page.AckToken)

	// Assert
	require.NoError(t, ackErr)
	assert.Equal(t, f.topic.ScopeID, page.ScopeID)
	require.Len(t, page.Entries, 1)
	require.NotNil(t, page.Entries[0].Message)
	assert.Equal(t, m.EntryID, page.Entries[0].Message.EntryID)
	assert.Equal(t, int64(1), ack)
}
