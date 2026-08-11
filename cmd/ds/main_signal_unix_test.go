//go:build !windows

package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain_WhenMapWaitIsTerminated_ExitsAndReapsCommand(t *testing.T) {
	home := t.TempDir()
	repoRoot := t.TempDir()
	db, err := store.Open(filepath.Join(home, "devspecs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	lease, err := db.AcquireIndexWriter(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lease.Release() })
	executable, err := os.Executable()
	require.NoError(t, err)
	command := exec.Command(executable, "-test.run=TestMainSignalHelperProcess")
	command.Env = append(os.Environ(),
		"DEVSPECS_TEST_SIGNAL_MAIN=1",
		"DEVSPECS_TEST_SIGNAL_REPO="+repoRoot,
		"DEVSPECS_HOME="+home,
		"DEVSPECS_TELEMETRY=0",
	)
	stderr, err := command.StderrPipe()
	require.NoError(t, err)
	require.NoError(t, command.Start())
	waiting := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "waiting for another index update") {
				close(waiting)
				return
			}
		}
	}()
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "map did not report writer wait")
	}

	signalErr := command.Process.Signal(syscall.SIGTERM)
	exitResult := make(chan error, 1)
	go func() { exitResult <- command.Wait() }()
	var waitErr error
	select {
	case waitErr = <-exitResult:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		require.FailNow(t, "map did not exit after termination")
	}

	assert.NoError(t, signalErr)
	assert.Error(t, waitErr)
}

func TestMainSignalHelperProcess(t *testing.T) {
	if os.Getenv("DEVSPECS_TEST_SIGNAL_MAIN") != "1" {
		return
	}
	os.Args = []string{"ds", "map", "--path", os.Getenv("DEVSPECS_TEST_SIGNAL_REPO")}

	main()
}
