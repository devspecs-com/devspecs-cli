package hubstore

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPruneDryRunAndActualPreserveHeadAndExposeGap(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	old := f.post(t, "original", "old-key", nil)
	page, err := f.d.Pull(ctx, f.repo, "reader", s.ID, 1)
	require.NoError(t, err)
	*f.now = f.now.Add(2 * time.Minute)
	newHead, err := f.d.ReviseMessage(ctx, f.repo, f.topic.ID, old.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "current", IdempotencyKey: "new-key"}, ExpectedRevision: 1})
	require.NoError(t, err)
	cutoff := f.now.Add(-time.Minute)

	// Act
	plan, err := f.d.PlanHubPrune(ctx, cutoff)
	require.NoError(t, err)
	actual, err := f.d.PruneHub(ctx, cutoff)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, 1, plan.Entries)
	assert.Equal(t, plan.Entries, actual.Entries)
	assert.Equal(t, plan.PayloadBytes, actual.PayloadBytes)
	require.Len(t, actual.Scopes, 1)
	require.Len(t, actual.Scopes[0].Gaps, 1)
	assert.Equal(t, old.Sequence, actual.Scopes[0].Gaps[0].From)
	assert.NotEmpty(t, actual.BackupPath)
	_, err = os.Stat(actual.BackupPath)
	require.NoError(t, err)
	retained, err := f.d.ReadMessage(ctx, f.repo, f.topic.ID, old.MessageID, true)
	require.NoError(t, err)
	assert.Equal(t, newHead.EntryID, retained.EntryID)
	_, err = f.d.PostMessage(ctx, f.repo, f.topic.ID, "author", MessageInput{AuthorityID: f.d.AuthorityID(), Text: "original", IdempotencyKey: "old-key"})
	assert.ErrorIs(t, err, ErrReplayGap)
	_, err = f.d.Ack(ctx, f.repo, "reader", s.ID, f.d.AuthorityID(), 0, page.NextScanPosition, page.AckToken)
	assert.ErrorIs(t, err, ErrReplayGap)
	pulled, err := f.d.Pull(ctx, f.repo, "reader", s.ID, 2)
	require.NoError(t, err)
	require.Len(t, pulled.Gaps, 1)
	assert.Equal(t, "pruned", pulled.Gaps[0].Reason)
	require.Len(t, pulled.Entries, 1)
	assert.Equal(t, newHead.EntryID, pulled.Entries[0].Message.EntryID)
	_, err = f.d.Ack(ctx, f.repo, "reader", s.ID, f.d.AuthorityID(), 0, pulled.NextScanPosition, pulled.AckToken)
	require.NoError(t, err)
}

func TestPruneOldOneShotRemovesDependentsAndKeepsPinnedAndNew(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	old := f.post(t, "old", "old-key", nil)
	_, err := f.d.PinMessage(ctx, f.repo, f.topic.ID, old.MessageID, "owner", f.d.AuthorityID(), f.topic.PolicyGeneration, old.Revision, true)
	require.NoError(t, err)
	_, err = f.d.PinMessage(ctx, f.repo, f.topic.ID, old.MessageID, "owner", f.d.AuthorityID(), f.topic.PolicyGeneration, old.Revision, false)
	require.NoError(t, err)
	voteForTest(t, f, old, "owner", 1)
	pinned := f.post(t, "pinned", "pinned-key", nil)
	_, err = f.d.PinMessage(ctx, f.repo, f.topic.ID, pinned.MessageID, "owner", f.d.AuthorityID(), f.topic.PolicyGeneration, pinned.Revision, true)
	require.NoError(t, err)
	*f.now = f.now.Add(2 * time.Minute)
	newer := f.post(t, "new", "new-key", nil)
	cutoff := f.now.Add(-time.Minute)

	// Act
	plan, err := f.d.PlanHubPrune(ctx, cutoff)
	require.NoError(t, err)
	actual, err := f.d.PruneHub(ctx, cutoff)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, 1, plan.Entries)
	assert.Equal(t, plan.Entries, actual.Entries)
	assert.Equal(t, plan.PayloadBytes, actual.PayloadBytes)
	assert.Equal(t, plan.PlanDigest, actual.PlanDigest)
	_, err = f.d.ReadMessage(ctx, f.repo, f.topic.ID, old.MessageID, true)
	assert.ErrorIs(t, err, ErrNotFound)
	pinnedRead, err := f.d.ReadMessage(ctx, f.repo, f.topic.ID, pinned.MessageID, true)
	require.NoError(t, err)
	assert.Equal(t, pinned.EntryID, pinnedRead.EntryID)
	newerRead, err := f.d.ReadMessage(ctx, f.repo, f.topic.ID, newer.MessageID, true)
	require.NoError(t, err)
	assert.Equal(t, newer.EntryID, newerRead.EntryID)
	var heads, revisions, votes, pins int
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM message_heads WHERE message_id=?", old.MessageID).Scan(&heads))
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM message_revisions WHERE message_id=?", old.MessageID).Scan(&revisions))
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM message_votes WHERE message_id=?", old.MessageID).Scan(&votes))
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM message_pin_audit WHERE message_id=?", old.MessageID).Scan(&pins))
	assert.Zero(t, heads)
	assert.Zero(t, revisions)
	assert.Zero(t, votes)
	assert.Zero(t, pins)
	_, err = f.d.PostMessage(ctx, f.repo, f.topic.ID, "author", MessageInput{AuthorityID: f.d.AuthorityID(), Text: "old", IdempotencyKey: "old-key"})
	assert.ErrorIs(t, err, ErrReplayGap)
	assert.NoError(t, f.d.CheckIntegrity(ctx))
}

func TestPruneOldOneShotReportsGapAndRejectsStaleAck(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	enrollTestConsumer(t, f, "reader")
	subscription := subscribeTest(t, f, "reader", true, nil, nil)
	old := f.post(t, "old", "old-key", nil)
	before, err := f.d.Pull(ctx, f.repo, "reader", subscription.ID, 1)
	require.NoError(t, err)
	*f.now = f.now.Add(2 * time.Minute)
	newer := f.post(t, "new", "new-key", nil)
	cutoff := f.now.Add(-time.Minute)

	// Act
	_, err = f.d.PruneHub(ctx, cutoff)
	require.NoError(t, err)
	_, staleAckErr := f.d.Ack(ctx, f.repo, "reader", subscription.ID, f.d.AuthorityID(), 0, before.NextScanPosition, before.AckToken)
	page, pullErr := f.d.Pull(ctx, f.repo, "reader", subscription.ID, 2)

	// Assert
	assert.ErrorIs(t, staleAckErr, ErrReplayGap)
	require.NoError(t, pullErr)
	require.Len(t, page.Gaps, 1)
	assert.Equal(t, old.Sequence, page.Gaps[0].From)
	assert.Equal(t, old.Sequence, page.Gaps[0].To)
	assert.Equal(t, "pruned", page.Gaps[0].Reason)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, newer.EntryID, page.Entries[0].Message.EntryID)
}

func TestPruneWholeOldRevisionGroup(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	first := f.post(t, "first", "first-key", nil)
	voteForTest(t, f, first, "owner", 1)
	*f.now = f.now.Add(time.Minute)
	second, err := f.d.ReviseMessage(ctx, f.repo, f.topic.ID, first.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "second", IdempotencyKey: "second-key"}, ExpectedRevision: 1})
	require.NoError(t, err)
	voteForTest(t, f, second, "owner", -1)
	*f.now = f.now.Add(time.Minute)
	cutoff := f.now.Add(-time.Second)

	// Act
	plan, err := f.d.PlanHubPrune(ctx, cutoff)
	require.NoError(t, err)
	actual, err := f.d.PruneHub(ctx, cutoff)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, 2, plan.Entries)
	assert.Equal(t, plan.Entries, actual.Entries)
	assert.Equal(t, plan.PlanDigest, actual.PlanDigest)
	_, firstReadErr := f.d.ReadPublication(ctx, f.repo, first.EntryID, true)
	_, secondReadErr := f.d.ReadPublication(ctx, f.repo, second.EntryID, true)
	assert.ErrorIs(t, firstReadErr, ErrNotFound)
	assert.ErrorIs(t, secondReadErr, ErrNotFound)
	var votes int
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM message_votes WHERE message_id=?", first.MessageID).Scan(&votes))
	assert.Zero(t, votes)
	assert.NoError(t, f.d.CheckIntegrity(ctx))
}

func TestPruneClosedCorrectionChain(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	registerJob(t, f)
	firstInput := jobInput(f, `{"job":"first","duration":1}`, "first-key")
	first, err := f.d.PublishEvent(ctx, f.repo, f.topic.ID, "author", firstInput)
	require.NoError(t, err)
	secondInput := jobInput(f, `{"job":"second","duration":2}`, "second-key")
	secondInput.CorrectsEntryID = first.EntryID
	second, err := f.d.PublishEvent(ctx, f.repo, f.topic.ID, "author", secondInput)
	require.NoError(t, err)
	thirdInput := jobInput(f, `{"job":"third","duration":3}`, "third-key")
	thirdInput.CorrectsEntryID = second.EntryID
	third, err := f.d.PublishEvent(ctx, f.repo, f.topic.ID, "author", thirdInput)
	require.NoError(t, err)
	*f.now = f.now.Add(time.Minute)
	cutoff := f.now.Add(-time.Second)

	// Act
	plan, err := f.d.PlanHubPrune(ctx, cutoff)
	require.NoError(t, err)
	actual, err := f.d.PruneHub(ctx, cutoff)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, 3, plan.Entries)
	assert.Equal(t, plan.Entries, actual.Entries)
	assert.Equal(t, plan.PlanDigest, actual.PlanDigest)
	_, firstReadErr := f.d.ReadEvent(ctx, f.repo, first.EntryID, true)
	_, secondReadErr := f.d.ReadEvent(ctx, f.repo, second.EntryID, true)
	_, thirdReadErr := f.d.ReadEvent(ctx, f.repo, third.EntryID, true)
	assert.ErrorIs(t, firstReadErr, ErrNotFound)
	assert.ErrorIs(t, secondReadErr, ErrNotFound)
	assert.ErrorIs(t, thirdReadErr, ErrNotFound)
	_, err = f.d.PublishEvent(ctx, f.repo, f.topic.ID, "author", firstInput)
	assert.ErrorIs(t, err, ErrReplayGap)
	_, err = f.d.ShowEventSchema(ctx, f.repo, f.topic.ID, "job.finished", 1)
	assert.NoError(t, err)
	assert.NoError(t, f.d.CheckIntegrity(ctx))
}

func TestPruneKeepsCorrectionChainWithNewMember(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	registerJob(t, f)
	first, err := f.d.PublishEvent(ctx, f.repo, f.topic.ID, "author", jobInput(f, `{"job":"first","duration":1}`, "first-key"))
	require.NoError(t, err)
	standalone, err := f.d.PublishEvent(ctx, f.repo, f.topic.ID, "author", jobInput(f, `{"job":"standalone","duration":1}`, "standalone-key"))
	require.NoError(t, err)
	*f.now = f.now.Add(time.Minute)
	cutoff := f.now.Add(-time.Second)
	secondInput := jobInput(f, `{"job":"second","duration":2}`, "second-key")
	secondInput.CorrectsEntryID = first.EntryID
	second, err := f.d.PublishEvent(ctx, f.repo, f.topic.ID, "author", secondInput)
	require.NoError(t, err)

	// Act
	plan, err := f.d.PlanHubPrune(ctx, cutoff)
	require.NoError(t, err)
	actual, err := f.d.PruneHub(ctx, cutoff)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, 1, plan.Entries)
	assert.Equal(t, plan.Entries, actual.Entries)
	_, err = f.d.ReadEvent(ctx, f.repo, standalone.EntryID, true)
	assert.ErrorIs(t, err, ErrNotFound)
	firstRead, err := f.d.ReadEvent(ctx, f.repo, first.EntryID, true)
	require.NoError(t, err)
	assert.Equal(t, first.EntryID, firstRead.EntryID)
	secondRead, err := f.d.ReadEvent(ctx, f.repo, second.EntryID, true)
	require.NoError(t, err)
	assert.Equal(t, second.EntryID, secondRead.EntryID)
	assert.NoError(t, f.d.CheckIntegrity(ctx))
}

func TestPrunePlanChangesForVoteAndPin(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	old := f.post(t, "old", "old-key", nil)
	*f.now = f.now.Add(time.Minute)
	cutoff := f.now.Add(-time.Second)
	before, err := f.d.PlanHubPrune(ctx, cutoff)
	require.NoError(t, err)

	// Act
	voteForTest(t, f, old, "owner", 1)
	withVote, err := f.d.PlanHubPrune(ctx, cutoff)
	require.NoError(t, err)
	_, err = f.d.PinMessage(ctx, f.repo, f.topic.ID, old.MessageID, "owner", f.d.AuthorityID(), f.topic.PolicyGeneration, old.Revision, true)
	require.NoError(t, err)
	withPin, err := f.d.PlanHubPrune(ctx, cutoff)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, 1, before.Entries)
	assert.Equal(t, 1, withVote.Entries)
	assert.NotEqual(t, before.PlanDigest, withVote.PlanDigest)
	assert.Zero(t, withPin.Entries)
}

func TestPruneArchivedTopicMessageKeepsTopicAudit(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	old := f.post(t, "old", "old-key", nil)
	archived, err := f.d.ArchiveTopic(ctx, f.repo, f.topic.ID, "owner", f.topic.PolicyGeneration, "closed")
	require.NoError(t, err)
	*f.now = f.now.Add(time.Minute)
	cutoff := f.now.Add(-time.Second)

	// Act
	report, err := f.d.PruneHub(ctx, cutoff)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, 1, report.Entries)
	_, err = f.d.ReadMessage(ctx, f.repo, f.topic.ID, old.MessageID, true)
	assert.ErrorIs(t, err, ErrNotFound)
	got, err := f.d.ShowTopic(ctx, f.repo, f.topic.ID)
	require.NoError(t, err)
	assert.Equal(t, archived.ID, got.ID)
	assert.Equal(t, "archived", got.State)
	var audits int
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM topic_audit WHERE topic_id=?", f.topic.ID).Scan(&audits))
	assert.Positive(t, audits)
	assert.NoError(t, f.d.CheckIntegrity(ctx))
}

func TestPruneExpiredOneShot(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	expiry := f.now.Add(30 * time.Second)
	old := f.post(t, "expires", "expiry-key", &expiry)
	*f.now = f.now.Add(time.Minute)
	cutoff := f.now.Add(-time.Second)

	// Act
	plan, err := f.d.PlanHubPrune(ctx, cutoff)
	require.NoError(t, err)
	actual, err := f.d.PruneHub(ctx, cutoff)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, 1, plan.Entries)
	assert.Equal(t, plan.Entries, actual.Entries)
	_, err = f.d.ReadMessage(ctx, f.repo, f.topic.ID, old.MessageID, true)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.NoError(t, f.d.CheckIntegrity(ctx))
}

func TestPruneReadOnlyFailsClosed(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	f.post(t, "original", "key", nil)
	*f.now = f.now.Add(time.Minute)
	reader, err := OpenReadOnly(context.Background(), Options{Home: filepath.Dir(f.d.Path()), Now: func() time.Time { return *f.now }})
	require.NoError(t, err)
	defer reader.Close()

	// Act
	_, err = reader.PruneHub(context.Background(), f.now.Add(-time.Second))

	// Assert
	assert.ErrorIs(t, err, ErrUnauthorized)
	var count int
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM publications").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestPruneRollbackOnReferencedCorrection(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	old := f.post(t, "first", "first", nil)
	*f.now = f.now.Add(time.Minute)
	_, err := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, old.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "second"}, ExpectedRevision: 1})
	require.NoError(t, err)
	cutoff := f.now.Add(-time.Second)
	// A direct immutable-reference constraint is representative of an interrupted
	// delete: all preceding transaction writes must roll back.
	_, err = f.d.sql.Exec("CREATE TRIGGER test_prune_abort BEFORE DELETE ON publications BEGIN SELECT RAISE(ABORT,'interrupted'); END")
	require.NoError(t, err)

	// Act
	_, err = f.d.PruneHub(context.Background(), cutoff)

	// Assert
	assert.Error(t, err)
	var epoch, remaining, permission int
	require.NoError(t, f.d.sql.QueryRow("SELECT retention_epoch FROM hub_meta WHERE singleton=1").Scan(&epoch))
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM publications WHERE entry_id=?", old.EntryID).Scan(&remaining))
	require.NoError(t, f.d.sql.QueryRow("SELECT enabled FROM retention_permission WHERE singleton=1").Scan(&permission))
	assert.Zero(t, epoch)
	assert.Equal(t, 1, remaining)
	assert.Zero(t, permission)
}

func TestPruneChildProcessSeesDurableGap(t *testing.T) {
	if os.Getenv("HUB_PRUNE_CHILD") == "1" {
		ctx := context.Background()
		d, err := OpenReadOnly(ctx, Options{Home: os.Getenv("HUB_PRUNE_HOME")})
		require.NoError(t, err)
		defer d.Close()
		page, err := d.Pull(ctx, os.Getenv("HUB_PRUNE_REPO"), "reader", os.Getenv("HUB_PRUNE_SUB"), 2)
		require.NoError(t, err)
		require.Len(t, page.Gaps, 1)
		assert.Equal(t, "pruned", page.Gaps[0].Reason)
		return
	}
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	old := f.post(t, "old", "key", nil)
	*f.now = f.now.Add(time.Minute)
	_, err := f.d.ReviseMessage(context.Background(), f.repo, f.topic.ID, old.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "new"}, ExpectedRevision: 1})
	require.NoError(t, err)
	_, err = f.d.PruneHub(context.Background(), f.now.Add(-time.Second))
	require.NoError(t, err)
	cmd := exec.Command(os.Args[0], "-test.run=^TestPruneChildProcessSeesDurableGap$")
	cmd.Env = append(os.Environ(), "HUB_PRUNE_CHILD=1", "HUB_PRUNE_HOME="+filepath.Dir(f.d.Path()), "HUB_PRUNE_REPO="+f.repo, "HUB_PRUNE_SUB="+s.ID)

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	assert.NoError(t, err, string(out))
}

func TestPruneRolloverKeepsOnlyLatestOwnedBackup(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	first := f.post(t, "first", "one", nil)
	*f.now = f.now.Add(time.Minute)
	_, err := f.d.ReviseMessage(ctx, f.repo, f.topic.ID, first.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "second", IdempotencyKey: "two"}, ExpectedRevision: 1})
	require.NoError(t, err)
	*f.now = f.now.Add(time.Minute)
	_, err = f.d.ReviseMessage(ctx, f.repo, f.topic.ID, first.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "third", IdempotencyKey: "three"}, ExpectedRevision: 2})
	require.NoError(t, err)
	firstReport, err := f.d.PruneHub(ctx, f.now.Add(-90*time.Second))
	require.NoError(t, err)
	require.Equal(t, 1, firstReport.Entries)

	// Act
	secondReport, err := f.d.PruneHub(ctx, f.now.Add(-30*time.Second))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, secondReport.Entries)
	assert.NotEqual(t, firstReport.BackupPath, secondReport.BackupPath)
	_, err = os.Stat(firstReport.BackupPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(secondReport.BackupPath)
	assert.NoError(t, err)
	var available int
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM prune_audit WHERE backup_available=1").Scan(&available))
	assert.Equal(t, 1, available)
	var rangeCount int
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM pruned_ranges").Scan(&rangeCount))
	assert.Equal(t, 1, rangeCount)
}

func TestCandidateDigestDistinguishesEqualSizedEntries(t *testing.T) {
	// Arrange
	a := []pruneEntry{{id: "a", scope: "scope", sequence: 1, bytes: 10}}
	b := []pruneEntry{{id: "b", scope: "scope", sequence: 1, bytes: 10}}

	// Act
	first, second := candidateDigest(a), candidateDigest(b)

	// Assert
	assert.NotEqual(t, first, second)
}

func TestPruneEventKeepsSchemaAndRejectsOldKey(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	schema := registerJob(t, f)
	in := jobInput(f, `{"job":"build","duration":3}`, "event-key")
	event, err := f.d.PublishEvent(ctx, f.repo, f.topic.ID, "author", in)
	require.NoError(t, err)
	*f.now = f.now.Add(time.Minute)

	// Act
	report, err := f.d.PruneHub(ctx, f.now.Add(-time.Second))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, report.Entries)
	_, err = f.d.ReadEvent(ctx, f.repo, event.EntryID, true)
	assert.ErrorIs(t, err, ErrNotFound)
	retained, err := f.d.ShowEventSchema(ctx, f.repo, f.topic.ID, "job.finished", 1)
	require.NoError(t, err)
	assert.Equal(t, schema.SHA256, retained.SHA256)
	_, err = f.d.PublishEvent(ctx, f.repo, f.topic.ID, "author", in)
	assert.ErrorIs(t, err, ErrReplayGap)
}

func TestPruneBackupVerificationFailureLeavesPublications(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	old := f.post(t, "original", "key", nil)
	*f.now = f.now.Add(time.Minute)
	_, err := f.d.ReviseMessage(ctx, f.repo, f.topic.ID, old.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "new"}, ExpectedRevision: 1})
	require.NoError(t, err)
	_, err = f.d.sql.Exec("PRAGMA foreign_keys=OFF")
	require.NoError(t, err)
	_, err = f.d.sql.Exec("INSERT INTO topic_roles VALUES ('missing','author','maintainer')")
	require.NoError(t, err)
	defer func() {
		_, _ = f.d.sql.Exec("DELETE FROM topic_roles WHERE topic_id='missing'")
		_, _ = f.d.sql.Exec("PRAGMA foreign_keys=ON")
	}()

	// Act
	_, err = f.d.PruneHub(ctx, f.now.Add(-time.Second))

	// Assert
	assert.ErrorIs(t, err, ErrUnsupportedFormat)
	var count int
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM publications").Scan(&count))
	assert.Equal(t, 2, count)
	var backups []string
	backups, err = filepath.Glob(filepath.Join(filepath.Dir(f.d.Path()), "hub-prune-*.sqlite"))
	require.NoError(t, err)
	assert.Empty(t, backups)
}

func TestSparsePruneReportsGapBeyondOldLiveHead(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	kept := f.post(t, "older live head", "keep", nil)
	_, err := f.d.PinMessage(ctx, f.repo, f.topic.ID, kept.MessageID, "owner", f.d.AuthorityID(), f.topic.PolicyGeneration, kept.Revision, true)
	require.NoError(t, err)
	old := f.post(t, "old revision", "remove", nil)
	*f.now = f.now.Add(time.Minute)
	newHead, err := f.d.ReviseMessage(ctx, f.repo, f.topic.ID, old.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "new head"}, ExpectedRevision: 1})
	require.NoError(t, err)

	// Act
	report, err := f.d.PruneHub(ctx, f.now.Add(-time.Second))
	require.NoError(t, err)
	page, err := f.d.Pull(ctx, f.repo, "reader", s.ID, 3)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, report.Entries)
	assert.Equal(t, int64(0), page.LowWater)
	require.Len(t, page.Gaps, 1)
	assert.Equal(t, old.Sequence, page.Gaps[0].From)
	assert.Equal(t, "pruned", page.Gaps[0].Reason)
	require.Len(t, page.Entries, 2)
	assert.Equal(t, kept.EntryID, page.Entries[0].Message.EntryID)
	assert.Equal(t, newHead.EntryID, page.Entries[1].Message.EntryID)
}

func TestBackupHeadroomRejectsLowDisk(t *testing.T) {
	// Arrange
	const mib = 1024 * 1024

	// Act
	tooLow := sufficientBackupHeadroom(63*mib, 1*mib)
	insufficient := sufficientBackupHeadroom(65*mib, 1*mib)
	sufficient := sufficientBackupHeadroom(66*mib, 1*mib)

	// Assert
	assert.False(t, tooLow)
	assert.False(t, insufficient)
	assert.True(t, sufficient)
}

func TestVacuumHubShrinksDeletedPagesAndPreservesReplay(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	enrollTestConsumer(t, f, "reader")
	s := subscribeTest(t, f, "reader", true, nil, nil)
	old := f.post(t, strings.Repeat("x", 60*1024), "old-key", nil)
	for revision := 1; revision < 24; revision++ {
		_, err := f.d.ReviseMessage(ctx, f.repo, f.topic.ID, old.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: strings.Repeat("x", 60*1024)}, ExpectedRevision: int64(revision)})
		require.NoError(t, err)
	}
	*f.now = f.now.Add(time.Minute)
	newHead, err := f.d.ReviseMessage(ctx, f.repo, f.topic.ID, old.MessageID, "author", MessageRevisionInput{MessageInput: MessageInput{AuthorityID: f.d.AuthorityID(), Text: "current"}, ExpectedRevision: 24})
	require.NoError(t, err)
	pruned, err := f.d.PruneHub(ctx, f.now.Add(-time.Second))
	require.NoError(t, err)
	assert.Positive(t, pruned.PayloadBytes)
	require.NoError(t, f.d.checkpointForVacuum(ctx))
	uncompacted, err := os.Stat(f.d.Path())
	require.NoError(t, err)

	// Act
	vacuumed, err := f.d.VacuumHub(ctx)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, uncompacted.Size(), vacuumed.BytesBefore)
	assert.Less(t, vacuumed.BytesAfter, vacuumed.BytesBefore)
	assert.Positive(t, vacuumed.ReclaimedBytes)
	_, err = os.Stat(vacuumed.BackupPath)
	require.NoError(t, err)
	_, err = os.Stat(pruned.BackupPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
	page, err := f.d.Pull(ctx, f.repo, "reader", s.ID, 2)
	require.NoError(t, err)
	require.Len(t, page.Gaps, 1)
	assert.Equal(t, "pruned", page.Gaps[0].Reason)
	require.Len(t, page.Entries, 1)
	assert.Equal(t, newHead.EntryID, page.Entries[0].Message.EntryID)
	_, err = f.d.PostMessage(ctx, f.repo, f.topic.ID, "author", MessageInput{AuthorityID: f.d.AuthorityID(), Text: "again", IdempotencyKey: "old-key"})
	assert.ErrorIs(t, err, ErrReplayGap)
}

func TestVacuumHubCanceledLeavesAuthorityUnchanged(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	f.post(t, "current", "key", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	_, err := f.d.VacuumHub(ctx)

	// Assert
	assert.ErrorIs(t, err, context.Canceled)
	var count int
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM publications").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestVacuumHubBusyReaderLeavesAuthorityUnchanged(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	f.post(t, "current", "key", nil)
	reader, err := OpenReadOnly(context.Background(), Options{Home: filepath.Dir(f.d.Path())})
	require.NoError(t, err)
	defer reader.Close()
	tx, err := reader.sql.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	require.NoError(t, err)
	defer tx.Rollback()
	var count int
	require.NoError(t, tx.QueryRow("SELECT COUNT(*) FROM publications").Scan(&count))
	require.Equal(t, 1, count)

	// Act
	_, err = f.d.VacuumHub(context.Background())

	// Assert
	assert.ErrorIs(t, err, ErrBusyRetryable)
	require.NoError(t, f.d.sql.QueryRow("SELECT COUNT(*) FROM publications").Scan(&count))
	assert.Equal(t, 1, count)
}
