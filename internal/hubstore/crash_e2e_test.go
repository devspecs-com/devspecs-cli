package hubstore

import (
	"bufio"
	"bytes"
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
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
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
	require.NoError(t, err, stderr.String())
	require.Equal(t, "HUB_CRASH_READY\n", ready, stderr.String())
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
