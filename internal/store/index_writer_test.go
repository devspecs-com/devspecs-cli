package store

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcquireIndexWriter_WhenAnotherHandleOwnsLease_WaitsForRelease(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	owner, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = owner.Close() })
	contender, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = contender.Close() })
	ownerLease, err := owner.AcquireIndexWriter(context.Background(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ownerLease.Release() })
	waited := false
	releaseResult := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	go func() {
		time.Sleep(100 * time.Millisecond)
		releaseResult <- ownerLease.Release()
	}()
	contenderLease, acquireErr := contender.AcquireIndexWriter(ctx, func() { waited = true })
	releaseErr := <-releaseResult
	var contenderReleaseErr error
	if contenderLease != nil {
		contenderReleaseErr = contenderLease.Release()
	}

	require.NoError(t, releaseErr)
	require.NoError(t, acquireErr)
	assert.True(t, waited)
	assert.NoError(t, contenderReleaseErr)
}

func TestAcquireIndexWriter_WhenContextIsCanceled_ReturnsCancellation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	owner, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = owner.Close() })
	contender, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = contender.Close() })
	ownerLease, err := owner.AcquireIndexWriter(context.Background(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ownerLease.Release() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	contenderLease, acquireErr := contender.AcquireIndexWriter(ctx, nil)

	assert.Nil(t, contenderLease)
	assert.ErrorIs(t, acquireErr, context.Canceled)
}

func TestAcquireIndexWriter_WhenDeadlineExpires_ReturnsDeadlineExceeded(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	owner, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = owner.Close() })
	ownerLease, err := owner.AcquireIndexWriter(context.Background(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ownerLease.Release() })
	waited := false
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	t.Cleanup(cancel)

	contenderLease, acquireErr := AcquireIndexWriter(ctx, dbPath, func() { waited = true })

	assert.Nil(t, contenderLease)
	assert.True(t, waited)
	assert.ErrorIs(t, acquireErr, context.DeadlineExceeded)
}

func TestAcquireIndexWriter_WhenOwningProcessExits_ReleasesLease(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "devspecs.db")
	executable, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(executable, "-test.run=^TestIndexWriterOwnerProcess$")
	command.Env = append(os.Environ(),
		"DEVSPECS_TEST_INDEX_WRITER_OWNER=1",
		"DEVSPECS_TEST_INDEX_WRITER_DB="+dbPath,
	)
	stdout, err := command.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, command.Start())
	finished := false
	t.Cleanup(func() {
		if !finished {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "locked", strings.TrimSpace(ready))

	killErr := command.Process.Kill()
	waitErr := command.Wait()
	finished = true
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	lease, acquireErr := AcquireIndexWriter(ctx, dbPath, nil)
	var releaseErr error
	if lease != nil {
		releaseErr = lease.Release()
	}

	assert.NoError(t, killErr)
	assert.Error(t, waitErr)
	assert.NoError(t, acquireErr)
	assert.NoError(t, releaseErr)
}

func TestIndexWriterOwnerProcess(t *testing.T) {
	if os.Getenv("DEVSPECS_TEST_INDEX_WRITER_OWNER") != "1" {
		return
	}
	lease, err := AcquireIndexWriter(context.Background(), os.Getenv("DEVSPECS_TEST_INDEX_WRITER_DB"), nil)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	_, _ = fmt.Fprintln(os.Stdout, "locked")
	timer := time.NewTimer(time.Hour)
	<-timer.C
	runtime.KeepAlive(lease)
}
