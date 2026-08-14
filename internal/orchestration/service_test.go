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
	assert.Equal(t, "dispatch_test", handle.DispatchID)
	assert.Equal(t, "remote-queue", handle.Provider.Name)
	assert.FileExists(t, handlePath)
	assert.FileExists(t, service.Store.RequestPath("dispatch_test"))
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

func TestServiceDispatch_WithoutLane_GeneratesLaneFromDispatchID(t *testing.T) {
	// Arrange
	executionRepo := t.TempDir()
	taskRepo := t.TempDir()
	writeOrchestrationConfig(t, executionRepo, "remote-queue", nil)
	driver := &serviceFakeDriver{name: "remote-queue"}
	service := newTestService(t, &serviceRunner{executionRepo: executionRepo}, driver)
	options := testDispatchOptions(taskRepo, executionRepo)
	options.Lane = ""

	// Act
	_, _, err := service.Dispatch(context.Background(), options)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "dispatch_test", driver.options.Lane)
}

func TestServiceStatus_WithRunningProvider_ReturnsProviderNeutralState(t *testing.T) {
	// Arrange
	executionRepo := t.TempDir()
	taskRepo := t.TempDir()
	writeOrchestrationConfig(t, executionRepo, "remote-queue", nil)
	driver := &serviceFakeDriver{
		name: "remote-queue",
		status: ProviderExecutionStatus{
			State: "running", NativeState: "busy", Result: "",
		},
	}
	service := newTestService(t, &serviceRunner{executionRepo: executionRepo}, driver)
	_, _, err := service.Dispatch(context.Background(), testDispatchOptions(taskRepo, executionRepo))
	require.NoError(t, err)

	// Act
	status, err := service.Status(context.Background(), "dispatch_test")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, DispatchStatusSchema, status.Schema)
	assert.Equal(t, "dispatch_test", status.DispatchID)
	assert.Equal(t, "running", status.State)
	assert.Equal(t, "remote-queue", status.Provider.Name)
	assert.Equal(t, "busy", status.Provider.NativeState)
	assert.Nil(t, status.Receipt)
}

func TestServiceStatus_WithCompletedReceipt_DoesNotRequireProviderConfiguration(t *testing.T) {
	// Arrange
	service := newTestService(t, &serviceRunner{}, &serviceFakeDriver{name: "remote-queue"})
	request := DispatchRequest{
		Schema: DispatchRequestSchema, SchemaVersion: SchemaVersion, DispatchID: "dispatch_test",
		Repository: Repository{RequestedRoot: t.TempDir()},
	}
	requestPath, requestSHA, err := service.Store.WriteRequest(request)
	require.NoError(t, err)
	_, err = service.Store.WriteHandle(DispatchHandle{
		Schema: DispatchHandleSchema, SchemaVersion: SchemaVersion, DispatchID: "dispatch_test",
		RequestSHA256: requestSHA, Request: ArtifactRef{Kind: "path", Value: requestPath},
		Provider: ProviderHandle{Name: "remote-queue", RunID: "job-1", Data: json.RawMessage(`{}`)},
	})
	require.NoError(t, err)
	receiptPath, err := service.Store.WriteReceipt(DispatchReceipt{
		Schema: DispatchReceiptSchema, SchemaVersion: SchemaVersion,
		DispatchID: "dispatch_test", ReceiptID: "receipt_test",
		Execution: ReceiptExecution{State: "succeeded"}, Artifacts: []ArtifactRef{},
	})
	require.NoError(t, err)

	// Act
	status, err := service.Status(context.Background(), "dispatch_test")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "succeeded", status.State)
	assert.Equal(t, "succeeded", status.Provider.Result)
	require.NotNil(t, status.Receipt)
	assert.Equal(t, receiptPath, status.Receipt.Value)
}

func TestServiceResume_WithExistingReceipt_ReturnsReceiptWithoutCallingProvider(t *testing.T) {
	// Arrange
	service := newTestService(t, &serviceRunner{}, &serviceFakeDriver{name: "remote-queue"})
	expected := DispatchReceipt{
		Schema: DispatchReceiptSchema, SchemaVersion: SchemaVersion,
		DispatchID: "dispatch_test", ReceiptID: "receipt_test", Artifacts: []ArtifactRef{},
	}
	expectedPath, err := service.Store.WriteReceipt(expected)
	require.NoError(t, err)

	// Act
	receipt, receiptPath, err := service.Resume(context.Background(), "dispatch_test")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedPath, receiptPath)
	assert.Equal(t, "receipt_test", receipt.ReceiptID)
}

func TestServiceWait_WhenProviderTimesOut_ReturnsRecoverableDispatch(t *testing.T) {
	// Arrange
	executionRepo := t.TempDir()
	taskRepo := t.TempDir()
	writeOrchestrationConfig(t, executionRepo, "remote-queue", nil)
	driver := &serviceFakeDriver{name: "remote-queue", waitErr: errors.New("provider timeout")}
	service := newTestService(t, &serviceRunner{executionRepo: executionRepo}, driver)
	handle, _, err := service.Dispatch(context.Background(), testDispatchOptions(taskRepo, executionRepo))
	require.NoError(t, err)

	// Act
	_, err = service.Wait(context.Background(), handle.DispatchID)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "remains recoverable")
	assert.ErrorContains(t, err, "ds dispatch resume dispatch_test")
	assert.FileExists(t, service.Store.HandlePath(handle.DispatchID))
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
	service.NewReceiptID = func() string { return "receipt_test" }

	// Act
	receipt, receiptPath, err := service.Finalize(context.Background(), handle.DispatchID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, DispatchReceiptSchema, receipt.Schema)
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
	request := DispatchRequest{Schema: DispatchRequestSchema, SchemaVersion: SchemaVersion, DispatchID: "dispatch_test"}
	requestPath, requestSHA, err := store.WriteRequest(request)
	require.NoError(t, err)
	_, err = store.WriteHandle(DispatchHandle{
		Schema: DispatchHandleSchema, SchemaVersion: SchemaVersion, DispatchID: request.DispatchID,
		RequestSHA256: requestSHA, Request: ArtifactRef{Kind: "path", Value: requestPath},
		Provider: ProviderHandle{Name: "remote-queue", RunID: "job-1", Data: json.RawMessage(`{}`)},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(requestPath, []byte("{}\n"), 0o600))

	// Act
	_, _, err = store.LoadHandle(request.DispatchID)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, "digest does not match")
}

func TestStateStoreWriteReceipt_WithExistingReceipt_ReplacesAtomically(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	first := DispatchReceipt{Schema: DispatchReceiptSchema, SchemaVersion: SchemaVersion, DispatchID: "dispatch_test", ReceiptID: "first", Artifacts: []ArtifactRef{}}
	_, err := store.WriteReceipt(first)
	require.NoError(t, err)
	second := first
	second.ReceiptID = "second"

	// Act
	path, err := store.WriteReceipt(second)

	// Assert
	require.NoError(t, err)
	var actual DispatchReceipt
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
	request DispatchRequest
	options DispatchOptions
	status  ProviderExecutionStatus
	waitErr error
	result  ProviderResult
}

func (d *serviceFakeDriver) Name() string {
	return d.name
}

func (d *serviceFakeDriver) Preflight(context.Context) (Capabilities, error) {
	return Capabilities{Provider: d.name, Isolation: true}, nil
}

func (d *serviceFakeDriver) Dispatch(_ context.Context, request DispatchRequest, options DispatchOptions) (ProviderHandle, error) {
	d.request = request
	d.options = options
	return ProviderHandle{Name: d.name, AdapterVersion: "test", RunID: "job-1", Data: json.RawMessage(`{}`)}, nil
}

func (d *serviceFakeDriver) Status(context.Context, ProviderHandle) (ProviderExecutionStatus, error) {
	return d.status, nil
}

func (d *serviceFakeDriver) Wait(context.Context, ProviderHandle, int) error {
	return d.waitErr
}

func (d *serviceFakeDriver) Finalize(context.Context, ProviderHandle, DispatchRequest, string) (ProviderResult, error) {
	return d.result, nil
}

func newTestService(t *testing.T, runner CommandRunner, driver Driver) *Service {
	t.Helper()
	registry := NewRegistry()
	registry.Register(driver.Name(), func(map[string]any) (Driver, error) { return driver, nil })
	return &Service{
		Store: StateStore{Root: t.TempDir()}, Runner: runner, Registry: registry,
		Now:   func() time.Time { return time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC) },
		NewID: func() string { return "dispatch_test" }, NewReceiptID: func() string { return "receipt_test" },
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
