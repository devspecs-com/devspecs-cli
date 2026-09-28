package hubstore

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
