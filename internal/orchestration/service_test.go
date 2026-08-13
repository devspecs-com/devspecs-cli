package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceDispatch_WithConfiguredProvider_WritesRequestAndHandle(t *testing.T) {
	// Arrange
	executionRepo := t.TempDir()
	taskRepo := t.TempDir()
	writeOrchestrationConfig(t, executionRepo, "remote-queue", nil)
	driver := &serviceFakeDriver{name: "remote-queue"}
	runner := &serviceRunner{executionRepo: executionRepo}
	service := newTestService(t, runner, driver)

	// Act
	handle, handlePath, err := service.Dispatch(context.Background(), testDispatchOptions(taskRepo, executionRepo))

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "handoff_test", handle.HandoffID)
	assert.Equal(t, "remote-queue", handle.Provider.Name)
	assert.FileExists(t, handlePath)
	assert.FileExists(t, service.Store.RequestPath("handoff_test"))
	assert.Equal(t, "task:sample", driver.request.Target.Owner)
	assert.Equal(t, "A01", driver.request.Target.Slice)
	assert.Equal(t, "clean", driver.request.Repository.SourceState)
	assert.Equal(t, "caller_only", driver.request.Input.ExecutionPolicy.Promotion)
	assert.NotEmpty(t, driver.request.Input.ApplySHA256)
	require.Len(t, runner.calls, 7)
	assert.Equal(t, "fake-ds", runner.calls[6].executable)
	assert.Equal(t, "apply", runner.calls[6].args[0])
	assert.Equal(t, "sample", runner.calls[6].args[1])
}

func TestServiceDispatch_WithoutConfiguredProvider_FailsBeforeRepositoryInspection(t *testing.T) {
	// Arrange
	executionRepo := t.TempDir()
	taskRepo := t.TempDir()
	writeOrchestrationConfig(t, executionRepo, "", nil)
	runner := &serviceRunner{executionRepo: executionRepo}
	service := newTestService(t, runner, &serviceFakeDriver{name: "remote-queue"})

	// Act
	_, _, err := service.Dispatch(context.Background(), testDispatchOptions(taskRepo, executionRepo))

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "orchestration is disabled")
	assert.Empty(t, runner.calls)
}

func TestServiceWait_WhenProviderTimesOut_ReturnsRecoverableHandoff(t *testing.T) {
	// Arrange
	executionRepo := t.TempDir()
	taskRepo := t.TempDir()
	writeOrchestrationConfig(t, executionRepo, "remote-queue", nil)
	driver := &serviceFakeDriver{name: "remote-queue", waitErr: errors.New("provider timeout")}
	service := newTestService(t, &serviceRunner{executionRepo: executionRepo}, driver)
	handle, _, err := service.Dispatch(context.Background(), testDispatchOptions(taskRepo, executionRepo))
	require.NoError(t, err)

	// Act
	_, err = service.Wait(context.Background(), handle.HandoffID)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "remains recoverable")
	assert.ErrorContains(t, err, "ds-orchestrate wait handoff_test")
	assert.FileExists(t, service.Store.HandlePath(handle.HandoffID))
}

func TestServiceFinalize_WithProviderResult_WritesNormalizedReceipt(t *testing.T) {
	// Arrange
	executionRepo := t.TempDir()
	taskRepo := t.TempDir()
	writeOrchestrationConfig(t, executionRepo, "remote-queue", nil)
	resultCommit := "result-commit"
	driver := &serviceFakeDriver{
		name: "remote-queue",
		result: ProviderResult{
			ProviderVersion: "queue-v1", RunID: "job-1", ExecutionState: "succeeded", Attempts: 1,
			StartedAt:         time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC),
			EndedAt:           time.Date(2026, 8, 13, 20, 1, 0, 0, time.UTC),
			EffectiveLocation: ArtifactRef{Kind: "uri", Value: "queue://job-1"},
			ResultCommit:      &resultCommit, ResultStateDigest: "sha256:result",
			ReportState: "produced", VerificationState: "passed",
			VerificationStrength: "declared:suite", VerificationFailureClass: "none",
			CancellationSupported: true,
		},
	}
	service := newTestService(t, &serviceRunner{executionRepo: executionRepo}, driver)
	handle, _, err := service.Dispatch(context.Background(), testDispatchOptions(taskRepo, executionRepo))
	require.NoError(t, err)
	service.NewID = func() string { return "receipt_test" }

	// Act
	receipt, receiptPath, err := service.Finalize(context.Background(), handle.HandoffID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, ReceiptSchema, receipt.Schema)
	assert.Equal(t, "receipt_test", receipt.ReceiptID)
	assert.Equal(t, "remote-queue", receipt.Provider.Name)
	assert.Equal(t, "succeeded", receipt.Execution.State)
	assert.Equal(t, "queue://job-1", receipt.Repository.EffectiveLocation.Value)
	assert.True(t, receipt.Cancellation.Supported)
	assert.FileExists(t, receiptPath)
}

func TestStateStoreLoadHandle_WithChangedRequest_ReturnsDigestError(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	request := HandoffRequest{Schema: HandoffSchema, SchemaVersion: SchemaVersion, HandoffID: "handoff_test"}
	requestPath, requestSHA, err := store.WriteRequest(request)
	require.NoError(t, err)
	_, err = store.WriteHandle(HandoffHandle{
		Schema: HandleSchema, SchemaVersion: SchemaVersion, HandoffID: request.HandoffID,
		RequestSHA256: requestSHA, Request: ArtifactRef{Kind: "path", Value: requestPath},
		Provider: ProviderHandle{Name: "remote-queue", RunID: "job-1", Data: json.RawMessage(`{}`)},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(requestPath, []byte("{}\n"), 0o600))

	// Act
	_, _, err = store.LoadHandle(request.HandoffID)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "digest does not match")
}

func TestStateStoreWriteReceipt_WithExistingReceipt_ReplacesAtomically(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	first := HandoffReceipt{Schema: ReceiptSchema, SchemaVersion: SchemaVersion, HandoffID: "handoff_test", ReceiptID: "first", Artifacts: []ArtifactRef{}}
	_, err := store.WriteReceipt(first)
	require.NoError(t, err)
	second := first
	second.ReceiptID = "second"

	// Act
	path, err := store.WriteReceipt(second)

	// Assert
	require.NoError(t, err)
	var actual HandoffReceipt
	require.NoError(t, readJSONFile(path, &actual))
	assert.Equal(t, "second", actual.ReceiptID)
}

type serviceRunnerCall struct {
	executable string
	args       []string
}

type serviceRunner struct {
	executionRepo string
	calls         []serviceRunnerCall
}

func (r *serviceRunner) Run(_ context.Context, _ string, executable string, args ...string) (CommandResult, error) {
	r.calls = append(r.calls, serviceRunnerCall{executable: executable, args: append([]string(nil), args...)})
	if executable == "fake-ds" {
		return CommandResult{Stdout: []byte(`{"target":"A01","prompt":"Implement A01 only."}`)}, nil
	}
	joined := strings.Join(args, " ")
	switch joined {
	case "rev-parse --show-toplevel":
		return CommandResult{Stdout: []byte(r.executionRepo + "\n")}, nil
	case "status --porcelain=v1 --untracked-files=all":
		return CommandResult{}, nil
	case "rev-parse HEAD":
		return CommandResult{Stdout: []byte("source-commit\n")}, nil
	case "rev-parse HEAD^{tree}":
		return CommandResult{Stdout: []byte("source-tree\n")}, nil
	case "rev-list --max-parents=0 HEAD":
		return CommandResult{Stdout: []byte("root-commit\n")}, nil
	case "remote get-url origin":
		return CommandResult{Stdout: []byte("git@github.com:example/project.git\n")}, nil
	default:
		return CommandResult{ExitCode: 1, Stderr: []byte("unexpected test command")}, nil
	}
}

type serviceFakeDriver struct {
	name    string
	request HandoffRequest
	waitErr error
	result  ProviderResult
}

func (d *serviceFakeDriver) Name() string {
	return d.name
}

func (d *serviceFakeDriver) Preflight(context.Context) (Capabilities, error) {
	return Capabilities{Provider: d.name, Isolation: true}, nil
}

func (d *serviceFakeDriver) Dispatch(_ context.Context, request HandoffRequest, _ DispatchOptions) (ProviderHandle, error) {
	d.request = request
	return ProviderHandle{Name: d.name, AdapterVersion: "test", RunID: "job-1", Data: json.RawMessage(`{}`)}, nil
}

func (d *serviceFakeDriver) Wait(context.Context, ProviderHandle, int) error {
	return d.waitErr
}

func (d *serviceFakeDriver) Finalize(context.Context, ProviderHandle, HandoffRequest, string) (ProviderResult, error) {
	return d.result, nil
}

func newTestService(t *testing.T, runner CommandRunner, driver Driver) *Service {
	t.Helper()
	registry := NewRegistry()
	registry.Register(driver.Name(), func(map[string]any) (Driver, error) { return driver, nil })
	return &Service{
		Store: StateStore{Root: t.TempDir()}, Runner: runner, Registry: registry,
		Now:   func() time.Time { return time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC) },
		NewID: func() string { return "handoff_test" },
	}
}

func testDispatchOptions(taskRepo, executionRepo string) DispatchOptions {
	return DispatchOptions{
		TaskRepo: taskRepo, Target: "task:sample", ExecutionRepo: executionRepo,
		Lane: "sample", AgentProvider: "codex", TimeoutSeconds: 600,
		VerifyTimeoutSeconds: 600, DSExecutable: "fake-ds",
	}
}

func writeOrchestrationConfig(t *testing.T, repoRoot, provider string, options map[string]any) {
	t.Helper()
	cfg := config.DefaultRepoConfig()
	cfg.Integrations.Orchestration = config.OrchestrationConfig{Provider: provider, Options: options}
	require.NoError(t, config.WriteRepoConfig(repoRoot, cfg))
	assert.FileExists(t, filepath.Join(repoRoot, ".devspecs", "config.yaml"))
}
