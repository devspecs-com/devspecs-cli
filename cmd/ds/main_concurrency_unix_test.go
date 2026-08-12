//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const concurrentCLIProcessTimeout = 90 * time.Second

type synchronizedProcessBuffer struct {
	mu      sync.Mutex
	content []byte
	waiting chan struct{}
	once    sync.Once
}

func newSynchronizedProcessBuffer() *synchronizedProcessBuffer {
	return &synchronizedProcessBuffer{waiting: make(chan struct{})}
}

func (b *synchronizedProcessBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	b.content = append(b.content, p...)
	waiting := strings.Contains(string(b.content), "waiting for another index update")
	b.mu.Unlock()
	if waiting {
		b.once.Do(func() { close(b.waiting) })
	}
	return len(p), nil
}

func (b *synchronizedProcessBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.content)
}

type concurrentCLIProcess struct {
	command *exec.Cmd
	stdout  *synchronizedProcessBuffer
	stderr  *synchronizedProcessBuffer
	done    chan error
}

type concurrentIndexState struct {
	RepoCount      int
	ArtifactCount  int
	FTSCount       int
	MissingFTS     int
	OrphanedFTS    int
	LastScanCommit string
	LastScanAt     string
	Integrity      string
	ForeignKeysOK  bool
}

type durableTaskProcessOutput struct {
	TaskID    string `json:"task_id"`
	Workspace string `json:"workspace"`
	Slices    []struct {
		ID string `json:"id"`
	} `json:"slices"`
}

func TestMain_WhenConcurrentCommandsShareIndex_SerializesWritesAndPreservesReads(t *testing.T) {
	repoRoot, home := setupConcurrentCLIFixture(t)
	initialScan := runConcurrentCLIProcess(t, repoRoot, home, nil, "scan", "--path", repoRoot)
	waitForConcurrentCLIProcess(t, initialScan, concurrentCLIProcessTimeout)
	require.Contains(t, initialScan.stdout.String(), "Scanned repository:")
	commitConcurrentFixtureChange(t, repoRoot)
	db, err := store.Open(filepath.Join(home, "devspecs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	lease, err := db.AcquireIndexWriter(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lease.Release() })

	mapProcess := runConcurrentCLIProcess(t, repoRoot, home, nil, "map", "--path", repoRoot)
	findProcess := runConcurrentCLIProcess(t, repoRoot, home, nil, "find", "concurrent activation")
	taskProcess := runConcurrentCLIProcess(t, repoRoot, home, nil, "task", "stabilize concurrent activation", "--id", "l04-contention-task", "--slice", "Verify concurrent indexing")
	scanProcess := runConcurrentCLIProcess(t, repoRoot, home, nil, "scan", "--path", repoRoot)
	waitForIndexQueue(t, mapProcess)
	waitForIndexQueue(t, findProcess)
	waitForIndexQueue(t, taskProcess)
	waitForIndexQueue(t, scanProcess)
	recentProcess := runConcurrentCLIProcess(t, repoRoot, home, nil, "recent", "--path", repoRoot)
	snapshotProcess := runConcurrentCLIProcess(t, repoRoot, home, nil, "find", "concurrent activation", "--no-refresh")
	waitForConcurrentCLIProcess(t, recentProcess, concurrentCLIProcessTimeout)
	waitForConcurrentCLIProcess(t, snapshotProcess, concurrentCLIProcessTimeout)

	require.NoError(t, lease.Release())
	require.NoError(t, db.Close())
	waitForConcurrentCLIProcess(t, mapProcess, concurrentCLIProcessTimeout)
	waitForConcurrentCLIProcess(t, findProcess, concurrentCLIProcessTimeout)
	waitForConcurrentCLIProcess(t, taskProcess, concurrentCLIProcessTimeout)
	waitForConcurrentCLIProcess(t, scanProcess, concurrentCLIProcessTimeout)
	state := readConcurrentIndexState(t, home)
	head := concurrentFixtureGitOutput(t, repoRoot, "rev-parse", "HEAD")

	assert.Contains(t, snapshotProcess.stdout.String(), "Working set: concurrent activation")
	assert.Contains(t, snapshotProcess.stdout.String(), "Concurrent Activation Plan")
	assert.Contains(t, recentProcess.stdout.String(), "Recently active topics")
	assert.Contains(t, mapProcess.stdout.String(), "Repo map:")
	assert.Contains(t, findProcess.stdout.String(), "Working set: concurrent activation")
	assert.Contains(t, taskProcess.stdout.String(), "Created task workspace:")
	assert.Contains(t, scanProcess.stdout.String(), "Scanned repository:")
	assertProcessHasNoSQLiteContention(t, mapProcess)
	assertProcessHasNoSQLiteContention(t, findProcess)
	assertProcessHasNoSQLiteContention(t, recentProcess)
	assertProcessHasNoSQLiteContention(t, taskProcess)
	assertProcessHasNoSQLiteContention(t, scanProcess)
	assert.Equal(t, 1, state.RepoCount)
	assert.Positive(t, state.ArtifactCount)
	assert.Equal(t, state.ArtifactCount, state.FTSCount)
	assert.Zero(t, state.MissingFTS)
	assert.Zero(t, state.OrphanedFTS)
	assert.Equal(t, head, state.LastScanCommit)
	assert.NotEmpty(t, state.LastScanAt)
	assert.Equal(t, "ok", state.Integrity)
	assert.True(t, state.ForeignKeysOK)
}

func TestMain_WhenCPUActiveMapRefreshIsTerminated_ReapsGitAndPreservesIndex(t *testing.T) {
	repoRoot, home := setupConcurrentCLIFixture(t)
	initialScan := runConcurrentCLIProcess(t, repoRoot, home, nil, "scan", "--path", repoRoot, "--quiet")
	waitForConcurrentCLIProcess(t, initialScan, concurrentCLIProcessTimeout)
	before := readConcurrentIndexState(t, home)
	commitConcurrentFixtureChange(t, repoRoot)
	actualGit, err := exec.LookPath("git")
	require.NoError(t, err)
	shimDir := t.TempDir()
	gitPIDPath := filepath.Join(t.TempDir(), "git.pid")
	writeCPUActiveGitShim(t, filepath.Join(shimDir, "git"))
	extraEnv := []string{
		"PATH=" + shimDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"DEVSPECS_TEST_REAL_GIT=" + actualGit,
		"DEVSPECS_TEST_GIT_PID_FILE=" + gitPIDPath,
	}
	mapProcess := runConcurrentCLIProcess(t, repoRoot, home, extraEnv, "map", "--path", repoRoot)
	gitPID := waitForGitPID(t, gitPIDPath)

	signalErr := mapProcess.command.Process.Signal(syscall.SIGTERM)
	waitErr := waitForTerminatedCLIProcess(t, mapProcess, 5*time.Second)
	gitExited := waitForProcessExit(gitPID, 5*time.Second)
	after := readConcurrentIndexState(t, home)

	assert.NoError(t, signalErr)
	assert.Error(t, waitErr)
	assert.True(t, gitExited, "Git subprocess %d remained alive after map termination", gitPID)
	assert.Equal(t, before.ArtifactCount, after.ArtifactCount)
	assert.Equal(t, before.LastScanCommit, after.LastScanCommit)
	assert.Equal(t, before.LastScanAt, after.LastScanAt)
	assert.Equal(t, "ok", after.Integrity)
	assert.True(t, after.ForeignKeysOK)
}

func TestMain_WhenCPUActiveTaskPreflightIsTerminated_ReapsGitAndLeavesNoWorkspace(t *testing.T) {
	repoRoot, home := setupConcurrentCLIFixture(t)
	initialScan := runConcurrentCLIProcess(t, repoRoot, home, nil, "scan", "--path", repoRoot, "--quiet")
	waitForConcurrentCLIProcess(t, initialScan, concurrentCLIProcessTimeout)
	commitConcurrentFixtureChange(t, repoRoot)
	actualGit, err := exec.LookPath("git")
	require.NoError(t, err)
	shimDir := t.TempDir()
	gitPIDPath := filepath.Join(t.TempDir(), "git.pid")
	writeCPUActiveGitShim(t, filepath.Join(shimDir, "git"))
	extraEnv := []string{
		"PATH=" + shimDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"DEVSPECS_TEST_REAL_GIT=" + actualGit,
		"DEVSPECS_TEST_GIT_PID_FILE=" + gitPIDPath,
	}
	workspace := filepath.Join(repoRoot, "devspecs", "tasks", "interrupted-task")
	taskProcess := runConcurrentCLIProcess(t, repoRoot, home, extraEnv,
		"task", "publish a durable task",
		"--id", "interrupted-task",
		"--slice", "first boundary",
		"--slice", "second boundary",
	)
	gitPID := waitForGitPID(t, gitPIDPath)

	signalErr := taskProcess.command.Process.Signal(syscall.SIGTERM)
	waitErr := waitForTerminatedCLIProcess(t, taskProcess, 5*time.Second)
	gitExited := waitForProcessExit(gitPID, 5*time.Second)
	_, statErr := os.Stat(workspace)

	assert.NoError(t, signalErr)
	assert.Error(t, waitErr)
	assert.True(t, gitExited, "Git subprocess %d remained alive after task termination", gitPID)
	assert.True(t, os.IsNotExist(statErr), "workspace should not exist after task preflight termination: %v", statErr)
	assert.NotContains(t, taskProcess.stdout.String(), "Created task workspace:")
}

func TestMain_WhenSixSliceTaskCompletes_ReportsIdentityAndPublishesEveryArtifact(t *testing.T) {
	repoRoot, home := setupConcurrentCLIFixture(t)
	initialScan := runConcurrentCLIProcess(t, repoRoot, home, nil, "scan", "--path", repoRoot, "--quiet")
	waitForConcurrentCLIProcess(t, initialScan, concurrentCLIProcessTimeout)
	workspace := filepath.Join(repoRoot, "devspecs", "tasks", "six-slice-process-task")
	taskProcess := runConcurrentCLIProcess(t, repoRoot, home, nil,
		"task", "publish a durable six-slice task",
		"--id", "six-slice-process-task",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "first boundary",
		"--slice", "second boundary",
		"--slice", "third boundary",
		"--slice", "fourth boundary",
		"--slice", "fifth boundary",
		"--slice", "sixth boundary",
	)

	waitErr := waitForTerminatedCLIProcess(t, taskProcess, concurrentCLIProcessTimeout)
	var out durableTaskProcessOutput
	unmarshalErr := json.NewDecoder(strings.NewReader(taskProcess.stdout.String())).Decode(&out)
	entries, readErr := os.ReadDir(workspace)

	require.NoError(t, waitErr, "stdout:\n%s\nstderr:\n%s", taskProcess.stdout.String(), taskProcess.stderr.String())
	require.NoError(t, unmarshalErr)
	require.NoError(t, readErr)
	assert.Equal(t, "six-slice-process-task", out.TaskID)
	assert.Equal(t, workspace, out.Workspace)
	require.Len(t, out.Slices, 6)
	assert.Equal(t, "A01", out.Slices[0].ID)
	assert.Equal(t, "A02", out.Slices[1].ID)
	assert.Equal(t, "A03", out.Slices[2].ID)
	assert.Equal(t, "A04", out.Slices[3].ID)
	assert.Equal(t, "A05", out.Slices[4].ID)
	assert.Equal(t, "A06", out.Slices[5].ID)
	require.Len(t, entries, 14)
}

func TestMain_WhenForcedTaskReplacesEmptyWorkspace_ReportsIdentityAndPublishesCompleteArtifacts(t *testing.T) {
	repoRoot, home := setupConcurrentCLIFixture(t)
	initialScan := runConcurrentCLIProcess(t, repoRoot, home, nil, "scan", "--path", repoRoot, "--quiet")
	waitForConcurrentCLIProcess(t, initialScan, concurrentCLIProcessTimeout)
	workspace := filepath.Join(repoRoot, "devspecs", "tasks", "forced-process-task")
	require.NoError(t, os.MkdirAll(workspace, 0o755))
	taskProcess := runConcurrentCLIProcess(t, repoRoot, home, nil,
		"task", "replace an empty task workspace",
		"--id", "forced-process-task",
		"--force",
		"--no-refresh",
		"--index=false",
		"--json",
	)

	waitErr := waitForTerminatedCLIProcess(t, taskProcess, concurrentCLIProcessTimeout)
	var out durableTaskProcessOutput
	unmarshalErr := json.NewDecoder(strings.NewReader(taskProcess.stdout.String())).Decode(&out)
	entries, readErr := os.ReadDir(workspace)

	require.NoError(t, waitErr, "stdout:\n%s\nstderr:\n%s", taskProcess.stdout.String(), taskProcess.stderr.String())
	require.NoError(t, unmarshalErr)
	require.NoError(t, readErr)
	assert.Equal(t, "forced-process-task", out.TaskID)
	assert.Equal(t, workspace, out.Workspace)
	require.Len(t, out.Slices, 1)
	assert.Equal(t, "A01", out.Slices[0].ID)
	require.Len(t, entries, 4)
}

func TestMainConcurrentCLIHelperProcess(t *testing.T) {
	if os.Getenv("DEVSPECS_TEST_CONCURRENT_CLI_MAIN") != "1" {
		return
	}
	var args []string
	require.NoError(t, json.Unmarshal([]byte(os.Getenv("DEVSPECS_TEST_CONCURRENT_CLI_ARGS")), &args))
	os.Args = append([]string{"ds"}, args...)

	main()
}

func setupConcurrentCLIFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	repoRoot := filepath.Join(root, "repo")
	home := filepath.Join(root, "home")
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "plans"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "internal", "activation"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "plans", "concurrent-activation.md"), []byte("# Concurrent Activation Plan\n\nKeep the first indexed result authoritative during concurrent refreshes.\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "internal", "activation", "service.go"), []byte("package activation\n\nfunc CurrentSnapshot() string { return \"committed\" }\n"), 0o644))
	concurrentFixtureGit(t, repoRoot, "init", "-b", "main")
	concurrentFixtureGit(t, repoRoot, "remote", "add", "origin", "https://github.com/acme/concurrent-index-fixture.git")
	concurrentFixtureGit(t, repoRoot, "add", ".")
	concurrentFixtureGit(t, repoRoot, "commit", "-m", "add concurrent activation plan")
	return repoRoot, home
}

func commitConcurrentFixtureChange(t *testing.T, repoRoot string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "plans", "queued-refresh.md"), []byte("# Queued Refresh Reliability\n\nSerialize index mutations and keep committed reads useful.\n"), 0o644))
	concurrentFixtureGit(t, repoRoot, "add", ".")
	concurrentFixtureGit(t, repoRoot, "commit", "-m", "add queued refresh reliability")
}

func concurrentFixtureGit(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = repoRoot
	command.Env = commandEnvironmentWith(
		"GIT_AUTHOR_NAME=DevSpecs Test",
		"GIT_AUTHOR_EMAIL=test@devspecs.local",
		"GIT_COMMITTER_NAME=DevSpecs Test",
		"GIT_COMMITTER_EMAIL=test@devspecs.local",
	)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), output)
}

func concurrentFixtureGitOutput(t *testing.T, repoRoot string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = repoRoot
	output, err := command.CombinedOutput()
	require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), output)
	return strings.TrimSpace(string(output))
}

func runConcurrentCLIProcess(t *testing.T, repoRoot, home string, extraEnv []string, args ...string) *concurrentCLIProcess {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	encodedArgs, err := json.Marshal(args)
	require.NoError(t, err)
	stdout := newSynchronizedProcessBuffer()
	stderr := newSynchronizedProcessBuffer()
	command := exec.Command(executable, "-test.run=^TestMainConcurrentCLIHelperProcess$")
	command.Dir = repoRoot
	overrides := []string{
		"DEVSPECS_TEST_CONCURRENT_CLI_MAIN=1",
		"DEVSPECS_TEST_CONCURRENT_CLI_ARGS=" + string(encodedArgs),
		"DEVSPECS_HOME=" + home,
		"DEVSPECS_TELEMETRY=0",
	}
	overrides = append(overrides, extraEnv...)
	command.Env = commandEnvironmentWith(overrides...)
	command.Stdout = stdout
	command.Stderr = stderr
	require.NoError(t, command.Start())
	process := &concurrentCLIProcess{
		command: command,
		stdout:  stdout,
		stderr:  stderr,
		done:    make(chan error, 1),
	}
	go func() { process.done <- command.Wait() }()
	t.Cleanup(func() { _ = command.Process.Kill() })
	return process
}

func waitForConcurrentCLIProcess(t *testing.T, process *concurrentCLIProcess, timeout time.Duration) {
	t.Helper()
	select {
	case err := <-process.done:
		require.NoError(t, err, "stdout:\n%s\nstderr:\n%s", process.stdout.String(), process.stderr.String())
	case <-time.After(timeout):
		_ = process.command.Process.Kill()
		require.FailNow(t, "CLI process exceeded test timeout", "stdout:\n%s\nstderr:\n%s", process.stdout.String(), process.stderr.String())
	}
}

func waitForTerminatedCLIProcess(t *testing.T, process *concurrentCLIProcess, timeout time.Duration) error {
	t.Helper()
	select {
	case err := <-process.done:
		return err
	case <-time.After(timeout):
		_ = process.command.Process.Kill()
		require.FailNow(t, "terminated CLI process exceeded grace period", "stdout:\n%s\nstderr:\n%s", process.stdout.String(), process.stderr.String())
		return nil
	}
}

func waitForIndexQueue(t *testing.T, process *concurrentCLIProcess) {
	t.Helper()
	select {
	case <-process.stderr.waiting:
	case err := <-process.done:
		require.FailNow(t, "CLI process completed before queuing", "error: %v\nstdout:\n%s\nstderr:\n%s", err, process.stdout.String(), process.stderr.String())
	case <-time.After(10 * time.Second):
		require.FailNow(t, "CLI process did not report index queue", "stdout:\n%s\nstderr:\n%s", process.stdout.String(), process.stderr.String())
	}
}

func assertProcessHasNoSQLiteContention(t *testing.T, process *concurrentCLIProcess) {
	t.Helper()
	output := strings.ToLower(process.stdout.String() + "\n" + process.stderr.String())
	assert.NotContains(t, output, "database is locked")
	assert.NotContains(t, output, "database is busy")
	assert.NotContains(t, output, "sqlite_busy")
	assert.NotContains(t, output, "busy timeout")
}

func readConcurrentIndexState(t *testing.T, home string) concurrentIndexState {
	t.Helper()
	db, err := store.Open(filepath.Join(home, "devspecs.db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	state := concurrentIndexState{}
	require.NoError(t, db.QueryRow("PRAGMA integrity_check").Scan(&state.Integrity))
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM repos").Scan(&state.RepoCount))
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM artifacts").Scan(&state.ArtifactCount))
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM artifacts_fts").Scan(&state.FTSCount))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM artifacts a LEFT JOIN artifacts_fts f ON f.artifact_id = a.id WHERE f.artifact_id IS NULL`).Scan(&state.MissingFTS))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM artifacts_fts f LEFT JOIN artifacts a ON a.id = f.artifact_id WHERE a.id IS NULL`).Scan(&state.OrphanedFTS))
	require.NoError(t, db.QueryRow("SELECT last_scan_commit, last_scan_at FROM repos LIMIT 1").Scan(&state.LastScanCommit, &state.LastScanAt))
	foreignKeyRows, err := db.Query("PRAGMA foreign_key_check")
	require.NoError(t, err)
	state.ForeignKeysOK = !foreignKeyRows.Next()
	require.NoError(t, foreignKeyRows.Err())
	require.NoError(t, foreignKeyRows.Close())
	return state
}

func writeCPUActiveGitShim(t *testing.T, path string) {
	t.Helper()
	body := `#!/bin/sh
case "$*" in
  *"config --get remote.origin.url"*)
    printf '%s\n' "$$" > "$DEVSPECS_TEST_GIT_PID_FILE"
    exec yes > /dev/null
    ;;
esac
exec "$DEVSPECS_TEST_REAL_GIT" "$@"
`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o755))
}

func waitForGitPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		body, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(body)))
			require.NoError(t, parseErr)
			return pid
		}
		time.Sleep(25 * time.Millisecond)
	}
	require.FailNow(t, "Git shim did not become active")
	return 0
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return syscall.Kill(pid, 0) != nil
}

func commandEnvironmentWith(overrides ...string) []string {
	overridden := make(map[string]bool, len(overrides))
	for _, entry := range overrides {
		key, _, _ := strings.Cut(entry, "=")
		overridden[key] = true
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !overridden[key] {
			environment = append(environment, entry)
		}
	}
	return append(environment, overrides...)
}
