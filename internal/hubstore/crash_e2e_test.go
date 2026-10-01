package hubstore

import (
	"bufio"
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sqlite "modernc.org/sqlite"
)

func TestKilledWriterRollsBackAndReopenedAuthorityAcceptsWrites(t *testing.T) {
	// Arrange
	home := filepath.Join(t.TempDir(), "home")
	initial, err := Open(context.Background(), Options{Home: home})
	require.NoError(t, err)
	authorityID := initial.AuthorityID()
	require.NoError(t, initial.Close())

	processCtx, cancelProcess := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelProcess()
	cmd := exec.CommandContext(processCtx, os.Args[0], "-test.run=^TestHubCrashWriterChild$")
	cmd.Env = append(os.Environ(), "HUB_CRASH_CHILD=1", "HUB_CRASH_HOME="+home)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	reaped := false
	defer func() {
		if !reaped {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	// Act
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "HUB_CRASH_READY\n", ready)
	require.NoError(t, cmd.Process.Kill())
	waitErr := cmd.Wait()
	reaped = true
	require.Error(t, waitErr, "child should have been killed, not exited normally")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reopened, err := Open(ctx, Options{Home: home})
	require.NoError(t, err)
	defer reopened.Close()
	var crashedActorCount int
	queryErr := reopened.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM actors WHERE actor_id=?", "crash-uncommitted").Scan(&crashedActorCount)
	var integrity string
	integrityErr := reopened.sql.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity)
	actor, writeErr := reopened.EnrollActor(ctx, "after-crash")

	// Assert
	assert.Equal(t, authorityID, reopened.AuthorityID())
	require.NoError(t, queryErr)
	assert.Zero(t, crashedActorCount)
	require.NoError(t, integrityErr)
	assert.Equal(t, "ok", integrity)
	require.NoError(t, writeErr)
	assert.Equal(t, "after-crash", actor.ID)
}

func TestHubCrashWriterChild(t *testing.T) {
	if os.Getenv("HUB_CRASH_CHILD") != "1" {
		t.Skip("child process only")
	}
	ctx := context.Background()
	d, err := Open(ctx, Options{Home: os.Getenv("HUB_CRASH_HOME")})
	require.NoError(t, err)
	defer d.Close()
	tx, err := d.sql.BeginTx(ctx, &sql.TxOptions{})
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, "INSERT INTO actors (actor_id, enrolled_at) VALUES (?, ?)", "crash-uncommitted", time.Now().UnixMilli())
	require.NoError(t, err)
	_, err = fmt.Fprintln(os.Stdout, "HUB_CRASH_READY")
	require.NoError(t, err)
	var blocked [1]byte
	_, _ = os.Stdin.Read(blocked[:])
}

func TestKilledAfterAckRetainsCursorAndIntegrity(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	enrollTestConsumer(t, f, "crash-reader")
	s := subscribeTest(t, f, "crash-reader", true, nil, nil)
	f.post(t, "ack before crash", "ack-before-crash", nil)
	page, err := f.d.Pull(context.Background(), f.repo, "crash-reader", s.ID, 1)
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
	home := filepath.Dir(f.d.Path())
	authorityID := f.d.AuthorityID()
	require.NoError(t, f.d.Close())

	// Act
	killReadyHubChild(t, "TestHubCrashAckChild", "HUB_CRASH_ACK_CHILD=1", "HUB_CRASH_HOME="+home,
		"HUB_CRASH_REPO="+f.repo, "HUB_CRASH_SUB="+s.ID, "HUB_CRASH_TOKEN="+page.AckToken)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reopened, err := Open(ctx, Options{Home: home})
	require.NoError(t, err)
	defer reopened.Close()
	subs, listErr := reopened.ListSubscriptions(ctx, f.repo, "crash-reader", false)
	replay, pullErr := reopened.Pull(ctx, f.repo, "crash-reader", s.ID, 1)
	var integrity string
	integrityErr := reopened.sql.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity)

	// Assert
	assert.Equal(t, authorityID, reopened.AuthorityID())
	require.NoError(t, listErr)
	require.Len(t, subs, 1)
	assert.Equal(t, int64(1), subs[0].AcknowledgedSequence)
	require.NoError(t, pullErr)
	assert.Equal(t, int64(1), replay.PriorAcknowledged)
	assert.Empty(t, replay.Entries)
	require.NoError(t, integrityErr)
	assert.Equal(t, "ok", integrity)
}

func TestHubCrashAckChild(t *testing.T) {
	if os.Getenv("HUB_CRASH_ACK_CHILD") != "1" {
		t.Skip("child process only")
	}
	ctx := context.Background()
	d, err := Open(ctx, Options{Home: os.Getenv("HUB_CRASH_HOME")})
	require.NoError(t, err)
	defer d.Close()
	position, err := d.Ack(ctx, os.Getenv("HUB_CRASH_REPO"), "crash-reader", os.Getenv("HUB_CRASH_SUB"),
		d.AuthorityID(), 0, 1, os.Getenv("HUB_CRASH_TOKEN"))
	require.NoError(t, err)
	require.Equal(t, int64(1), position)
	_, err = fmt.Fprintln(os.Stdout, "HUB_CRASH_READY")
	require.NoError(t, err)
	var blocked [1]byte
	_, _ = os.Stdin.Read(blocked[:])
}

func TestKilledAfterPostMessageRetainsPublicationAndIntegrity(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	home := filepath.Dir(f.d.Path())
	authorityID := f.d.AuthorityID()
	require.NoError(t, f.d.Close())

	// Act
	killReadyHubChild(t, "TestHubCrashPostMessageChild", "HUB_CRASH_POST_CHILD=1", "HUB_CRASH_HOME="+home,
		"HUB_CRASH_REPO="+f.repo, "HUB_CRASH_TOPIC="+f.topic.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reopened, err := Open(ctx, Options{Home: home})
	require.NoError(t, err)
	defer reopened.Close()
	messages, listErr := reopened.ListMessages(ctx, f.repo, f.topic.ID, MessageList{Limit: 10})
	var integrity string
	integrityErr := reopened.sql.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity)

	// Assert
	assert.Equal(t, authorityID, reopened.AuthorityID())
	require.NoError(t, listErr)
	require.Len(t, messages, 1)
	assert.Equal(t, "published before crash", messages[0].Text)
	assert.Equal(t, "post-before-crash", messages[0].IdempotencyKey)
	assert.Equal(t, int64(1), messages[0].Sequence)
	require.NoError(t, integrityErr)
	assert.Equal(t, "ok", integrity)
}

func TestHubCrashPostMessageChild(t *testing.T) {
	if os.Getenv("HUB_CRASH_POST_CHILD") != "1" {
		t.Skip("child process only")
	}
	ctx := context.Background()
	d, err := Open(ctx, Options{Home: os.Getenv("HUB_CRASH_HOME")})
	require.NoError(t, err)
	defer d.Close()
	message, err := d.PostMessage(ctx, os.Getenv("HUB_CRASH_REPO"), os.Getenv("HUB_CRASH_TOPIC"), "author",
		MessageInput{AuthorityID: d.AuthorityID(), Text: "published before crash", IdempotencyKey: "post-before-crash"})
	require.NoError(t, err)
	require.Equal(t, int64(1), message.Sequence)
	_, err = fmt.Fprintln(os.Stdout, "HUB_CRASH_READY")
	require.NoError(t, err)
	var blocked [1]byte
	_, _ = os.Stdin.Read(blocked[:])
}

func TestKilledDuringPrunePreservesAuthorityState(t *testing.T) {
	// Arrange
	f := newMessageFixture(t)
	ctx := context.Background()
	enrollTestConsumer(t, f, "crash-reader")
	s := subscribeTest(t, f, "crash-reader", true, nil, nil)
	old := f.post(t, "original", "old-key", nil)
	page, err := f.d.Pull(ctx, f.repo, "crash-reader", s.ID, 1)
	require.NoError(t, err)
	position, err := f.d.Ack(ctx, f.repo, "crash-reader", s.ID, f.d.AuthorityID(), 0, page.NextScanPosition, page.AckToken)
	require.NoError(t, err)
	require.Equal(t, int64(1), position)
	*f.now = f.now.Add(2 * time.Minute)
	head, err := f.d.ReviseMessage(ctx, f.repo, f.topic.ID, old.MessageID, "author", MessageRevisionInput{
		MessageInput:     MessageInput{AuthorityID: f.d.AuthorityID(), Text: "retained head", IdempotencyKey: "head-key"},
		ExpectedRevision: 1,
	})
	require.NoError(t, err)
	cutoff := f.now.Add(-time.Minute)
	plan, err := f.d.PlanHubPrune(ctx, cutoff)
	require.NoError(t, err)
	require.Equal(t, 1, plan.Entries)
	home := filepath.Dir(f.d.Path())
	authorityID := f.d.AuthorityID()
	require.NoError(t, f.d.Close())

	processCtx, cancelProcess := context.WithTimeout(ctx, 20*time.Second)
	defer cancelProcess()
	cmd := exec.CommandContext(processCtx, os.Args[0], "-test.run=^TestHubCrashPruneChild$")
	cmd.Env = append(os.Environ(), "HUB_CRASH_PRUNE_CHILD=1", "HUB_CRASH_HOME="+home,
		"HUB_CRASH_PRUNE_CUTOFF="+fmt.Sprint(cutoff.UnixMilli()))
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	reaped := false
	defer func() {
		if !reaped {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	// Act
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "HUB_CRASH_READY\n", ready)
	backupsBeforeKill, err := filepath.Glob(filepath.Join(home, "hub-prune-*.sqlite"))
	require.NoError(t, err)
	require.Len(t, backupsBeforeKill, 1, "pause must occur after backup creation")
	require.NoError(t, cmd.Process.Kill())
	waitErr := cmd.Wait()
	reaped = true
	require.Error(t, waitErr, "child should have been killed, not exited normally")
	reopened, err := Open(ctx, Options{Home: home})
	require.NoError(t, err)
	defer reopened.Close()
	subs, listErr := reopened.ListSubscriptions(ctx, f.repo, "crash-reader", false)
	var oldCount, headCount, epoch, permission, auditCount, gapCount, oldKeyCount int
	require.NoError(t, reopened.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM publications WHERE entry_id=?", old.EntryID).Scan(&oldCount))
	require.NoError(t, reopened.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM publications WHERE entry_id=?", head.EntryID).Scan(&headCount))
	require.NoError(t, reopened.sql.QueryRowContext(ctx, "SELECT retention_epoch FROM hub_meta WHERE singleton=1").Scan(&epoch))
	require.NoError(t, reopened.sql.QueryRowContext(ctx, "SELECT enabled FROM retention_permission WHERE singleton=1").Scan(&permission))
	require.NoError(t, reopened.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM prune_audit").Scan(&auditCount))
	require.NoError(t, reopened.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM pruned_ranges").Scan(&gapCount))
	require.NoError(t, reopened.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM publication_idempotency WHERE key=?", "old-key").Scan(&oldKeyCount))
	var integrity string
	require.NoError(t, reopened.sql.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity))
	backupsAfterKill, err := filepath.Glob(filepath.Join(home, "hub-prune-*.sqlite"))
	require.NoError(t, err)
	require.Len(t, backupsAfterKill, 1)
	_, err = os.Stat(backupsAfterKill[0] + ".pending")
	require.NoError(t, err)

	// Assert
	assert.Equal(t, authorityID, reopened.AuthorityID())
	require.NoError(t, listErr)
	require.Len(t, subs, 1)
	assert.Equal(t, int64(1), subs[0].AcknowledgedSequence)
	assert.Equal(t, 1, oldCount)
	assert.Equal(t, 1, headCount)
	assert.Zero(t, epoch)
	assert.Zero(t, permission)
	assert.Zero(t, auditCount)
	assert.Zero(t, gapCount)
	assert.Equal(t, 1, oldKeyCount)
	assert.Equal(t, "ok", integrity)
}

func TestHubCrashPruneChild(t *testing.T) {
	if os.Getenv("HUB_CRASH_PRUNE_CHILD") != "1" {
		t.Skip("child process only")
	}
	// The scalar function belongs only to this child and only to connections opened below.
	err := sqlite.RegisterScalarFunction("pause_prune_delete", 0, func(_ *sqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
		if _, err := fmt.Fprintln(os.Stdout, "HUB_CRASH_READY"); err != nil {
			return nil, err
		}
		var blocked [1]byte
		_, err := os.Stdin.Read(blocked[:])
		return nil, err
	})
	require.NoError(t, err)
	ctx := context.Background()
	d, err := Open(ctx, Options{Home: os.Getenv("HUB_CRASH_HOME")})
	require.NoError(t, err)
	defer d.Close()
	_, err = d.sql.ExecContext(ctx, "CREATE TEMP TRIGGER pause_prune BEFORE DELETE ON main.publications BEGIN SELECT pause_prune_delete(); END")
	require.NoError(t, err)
	cutoffMillis, err := strconv.ParseInt(os.Getenv("HUB_CRASH_PRUNE_CUTOFF"), 10, 64)
	require.NoError(t, err)
	_, err = d.PruneHub(ctx, time.UnixMilli(cutoffMillis).UTC())
	require.NoError(t, err, "prune returned before reaching the deletion trigger")
}

func killReadyHubChild(t *testing.T, testName string, env ...string) {
	t.Helper()
	processCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(processCtx, os.Args[0], "-test.run=^"+testName+"$")
	cmd.Env = append(os.Environ(), env...)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	reaped := false
	defer func() {
		if !reaped {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "HUB_CRASH_READY\n", ready)
	require.NoError(t, cmd.Process.Kill())
	waitErr := cmd.Wait()
	reaped = true
	require.Error(t, waitErr, "child should have been killed, not exited normally")
}
