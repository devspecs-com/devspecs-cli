package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWaspflowDriver_WithUnknownOption_ReturnsError(t *testing.T) {
	// Arrange
	options := map[string]any{"isolate": true}

	// Act
	driver, err := NewWaspflowDriver(&waspflowRunner{}, options)

	// Assert
	assert.Nil(t, driver)
	require.Error(t, err)
	assert.ErrorContains(t, err, `unknown Waspflow orchestration option "isolate"`)
}

func TestWaspflowDriverPreflight_WithHealthyProvider_ReportsCapabilities(t *testing.T) {
	// Arrange
	runner := &waspflowRunner{results: []waspflowRun{{}}}
	driver := newTestWaspflowDriver(t, runner)

	// Act
	capabilities, err := driver.Preflight(context.Background())

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "waspflow", capabilities.Provider)
	assert.True(t, capabilities.Isolation)
	assert.True(t, capabilities.Reports)
	assert.True(t, capabilities.Verification)
	assert.False(t, capabilities.Cancellation)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, "fake-waspflow", runner.calls[0].executable)
	require.Len(t, runner.calls[0].args, 1)
	assert.Equal(t, "doctor", runner.calls[0].args[0])
}

func TestWaspflowDriverDispatch_WithExplicitOptions_UsesStockSpawnAndStatusCommands(t *testing.T) {
	// Arrange
	executionRepo := t.TempDir()
	runner := &waspflowRunner{results: []waspflowRun{
		{},
		{result: CommandResult{Stdout: []byte(`{"lane_uuid":"lane-uuid","cwd":"` + filepath.ToSlash(executionRepo) + `"}`)}},
	}}
	driver := newTestWaspflowDriver(t, runner)
	request := HandoffRequest{
		Repository: Repository{RequestedRoot: executionRepo},
		Input:      Input{ApplyPayload: json.RawMessage(`{"target":"W09","prompt":"Implement W09 only."}`)},
	}
	opts := DispatchOptions{Lane: "devspecs-w09", AgentProvider: "codex", MCP: "auto", Report: "result.md"}

	// Act
	handle, err := driver.Dispatch(context.Background(), request, opts)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "waspflow", handle.Name)
	assert.Equal(t, "lane-uuid", handle.RunID)
	require.Len(t, runner.calls, 2)
	assert.Equal(t, "fake-waspflow", runner.calls[0].executable)
	require.Len(t, runner.calls[0].args, 14)
	assert.Equal(t, "spawn", runner.calls[0].args[0])
	assert.Equal(t, "--lane", runner.calls[0].args[1])
	assert.Equal(t, "devspecs-w09", runner.calls[0].args[2])
	assert.Equal(t, "--cwd", runner.calls[0].args[3])
	assert.Equal(t, executionRepo, runner.calls[0].args[4])
	assert.Equal(t, "--isolate", runner.calls[0].args[5])
	assert.Equal(t, "--mcp", runner.calls[0].args[6])
	assert.Equal(t, "auto", runner.calls[0].args[7])
	assert.Equal(t, "--provider", runner.calls[0].args[8])
	assert.Equal(t, "codex", runner.calls[0].args[9])
	assert.Equal(t, "--report", runner.calls[0].args[10])
	assert.Equal(t, "result.md", runner.calls[0].args[11])
	assert.Equal(t, "--", runner.calls[0].args[12])
	assert.Contains(t, runner.calls[0].args[13], "Implement W09 only.")
	assert.Contains(t, runner.calls[0].args[13], "caller owns any later DevSpecs checkpoint")
	require.Len(t, runner.calls[1].args, 2)
	assert.Equal(t, "status", runner.calls[1].args[0])
	assert.Equal(t, "devspecs-w09", runner.calls[1].args[1])
}

func TestWaspflowDriverWait_WhenContextExpires_PreservesProviderLane(t *testing.T) {
	// Arrange
	runner := &waspflowRunner{results: []waspflowRun{{err: context.DeadlineExceeded}}}
	driver := newTestWaspflowDriver(t, runner)
	handle := testWaspflowHandle(t, "devspecs-w09")

	// Act
	err := driver.Wait(context.Background(), handle, 600)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, `lane "devspecs-w09" was not cancelled or reaped`)
	require.Len(t, runner.calls, 1)
	require.Len(t, runner.calls[0].args, 4)
	assert.Equal(t, "wait", runner.calls[0].args[0])
	assert.Equal(t, "devspecs-w09", runner.calls[0].args[1])
	assert.Equal(t, "--timeout", runner.calls[0].args[2])
	assert.Equal(t, "600", runner.calls[0].args[3])
}

func TestWaspflowDriverWait_WhenContextIsCanceled_PreservesProviderLane(t *testing.T) {
	// Arrange
	runner := &waspflowRunner{results: []waspflowRun{{err: context.Canceled}}}
	driver := newTestWaspflowDriver(t, runner)
	handle := testWaspflowHandle(t, "devspecs-w09")

	// Act
	err := driver.Wait(context.Background(), handle, 600)

	// Assert
	require.Error(t, err)
	assert.ErrorContains(t, err, `waspflow wait interrupted; lane "devspecs-w09" was not cancelled or reaped`)
	assert.ErrorIs(t, err, context.Canceled)
	require.Len(t, runner.calls, 1)
	require.Len(t, runner.calls[0].args, 4)
	assert.Equal(t, "wait", runner.calls[0].args[0])
	assert.Equal(t, "devspecs-w09", runner.calls[0].args[1])
}

func TestWaspflowDriverFinalize_WithMissingRequiredReport_DoesNotReapLane(t *testing.T) {
	// Arrange
	executionRepo := t.TempDir()
	runner := &waspflowRunner{results: []waspflowRun{{
		result: CommandResult{Stdout: []byte(`{"lane_uuid":"lane-uuid","cwd":"` + filepath.ToSlash(executionRepo) + `"}`)},
	}}}
	driver := newTestWaspflowDriver(t, runner)
	request := HandoffRequest{Requirements: Requirements{Report: ReportRequirement{Required: true, Path: "result.md"}}}

	// Act
	result, err := driver.Finalize(context.Background(), testWaspflowHandle(t, "devspecs-w09"), request, t.TempDir())

	// Assert
	assert.Equal(t, ProviderResult{}, result)
	require.Error(t, err)
	assert.ErrorContains(t, err, `required report path is absent; lane "devspecs-w09" was preserved`)
	require.Len(t, runner.calls, 1)
	assert.Equal(t, "status", runner.calls[0].args[0])
}

func TestWaspflowDriverFinalize_WithSuccessfulStockReceipt_NormalizesAndReaps(t *testing.T) {
	// Arrange
	home := t.TempDir()
	stateDir := t.TempDir()
	executionRepo := t.TempDir()
	reportPath := filepath.Join(t.TempDir(), "result.md")
	require.NoError(t, os.WriteFile(reportPath, []byte("# Result\n"), 0o600))
	laneDir := filepath.Join(home, "lanes", "devspecs-w09")
	require.NoError(t, os.MkdirAll(laneDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(laneDir, "verify-result.json"), []byte(`{"state":"passed"}`), 0o600))
	nativeReceipt := `{
  "lane_uuid": "lane-uuid",
  "waspflow_version": "0.8.0",
  "result": "verified",
  "verify": {"state": "passed", "failure_class": "none", "verify_strength": "declared:suite"},
  "timestamps": {"spawn_epoch": 1786651200, "finalize_epoch": 1786651260}
}`
	require.NoError(t, os.WriteFile(filepath.Join(laneDir, "receipt.json"), []byte(nativeReceipt), 0o600))
	status := `{"lane_uuid":"lane-uuid","cwd":"` + filepath.ToSlash(executionRepo) + `","report":"` + filepath.ToSlash(reportPath) + `","verify_state":"passed","verify_strength":"declared:suite"}`
	runner := &waspflowRunner{results: []waspflowRun{
		{result: CommandResult{Stdout: []byte(status)}},
		{result: CommandResult{Stdout: []byte(`{"state":"passed"}`)}},
		{result: CommandResult{Stdout: []byte(status)}},
		{result: CommandResult{Stdout: []byte("result-commit\n")}},
		{},
		{result: CommandResult{Stdout: []byte("diff\n")}},
		{},
		{},
	}}
	driver := newTestWaspflowDriver(t, runner)
	driver.home = home
	request := HandoffRequest{Requirements: Requirements{
		Report:       ReportRequirement{Required: true, Path: "result.md"},
		Verification: VerifyRequirement{Required: true, Command: "go test ./..."},
	}}

	// Act
	result, err := driver.Finalize(context.Background(), testWaspflowHandle(t, "devspecs-w09"), request, stateDir)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "0.8.0", result.ProviderVersion)
	assert.Equal(t, "lane-uuid", result.RunID)
	assert.Equal(t, "succeeded", result.ExecutionState)
	assert.Equal(t, "produced", result.ReportState)
	require.NotNil(t, result.ReportArtifact)
	assert.FileExists(t, result.ReportArtifact.Value)
	assert.Equal(t, "passed", result.VerificationState)
	assert.Equal(t, "declared:suite", result.VerificationStrength)
	assert.Equal(t, "none", result.VerificationFailureClass)
	require.NotNil(t, result.VerificationArtifact)
	assert.FileExists(t, result.VerificationArtifact.Value)
	require.NotNil(t, result.ResultCommit)
	assert.Equal(t, "result-commit", *result.ResultCommit)
	assert.NotEmpty(t, result.ResultStateDigest)
	assert.False(t, result.CancellationSupported)
	require.Len(t, runner.calls, 8)
	assert.Equal(t, "reap", runner.calls[7].args[0])
	assert.Equal(t, "devspecs-w09", runner.calls[7].args[1])
}

func TestWaspflowDriverFinalize_WithVerificationFailure_ReturnsFailedReceipt(t *testing.T) {
	// Arrange
	home := t.TempDir()
	executionRepo := t.TempDir()
	laneDir := filepath.Join(home, "lanes", "devspecs-w09")
	require.NoError(t, os.MkdirAll(laneDir, 0o700))
	nativeReceipt := `{
  "lane_uuid": "lane-uuid",
  "waspflow_version": "0.8.0",
  "result": "verify_failed",
  "verify": {"state": "failed", "failure_class": "tests", "verify_strength": "declared:suite"},
  "timestamps": {"spawn_epoch": 1786651200, "finalize_epoch": 1786651260}
}`
	require.NoError(t, os.WriteFile(filepath.Join(laneDir, "receipt.json"), []byte(nativeReceipt), 0o600))
	status := `{"lane_uuid":"lane-uuid","cwd":"` + filepath.ToSlash(executionRepo) + `","verify_state":"failed","verify_strength":"declared:suite"}`
	runner := &waspflowRunner{results: []waspflowRun{
		{result: CommandResult{Stdout: []byte(status)}},
		{result: CommandResult{ExitCode: 2, Stdout: []byte(`{"state":"failed"}`)}},
		{result: CommandResult{Stdout: []byte(status)}},
		{result: CommandResult{Stdout: []byte("result-commit\n")}},
		{},
		{},
		{},
		{result: CommandResult{ExitCode: 2}},
	}}
	driver := newTestWaspflowDriver(t, runner)
	driver.home = home
	request := HandoffRequest{Requirements: Requirements{
		Verification: VerifyRequirement{Required: true, Command: "go test ./..."},
	}}

	// Act
	result, err := driver.Finalize(context.Background(), testWaspflowHandle(t, "devspecs-w09"), request, t.TempDir())

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "failed", result.ExecutionState)
	require.NotNil(t, result.Failure)
	assert.Equal(t, "provider_result", result.Failure.Class)
	assert.Equal(t, "verify_failed", result.Failure.Detail)
	assert.Equal(t, "failed", result.VerificationState)
	assert.Equal(t, "tests", result.VerificationFailureClass)
	require.Len(t, runner.calls, 8)
	assert.Equal(t, "verify", runner.calls[1].args[0])
	assert.Equal(t, "reap", runner.calls[7].args[0])
}

func TestWaspflowDriverFinalize_WhenCleanupFailsWithoutReceipt_PreservesInspectableLane(t *testing.T) {
	// Arrange
	executionRepo := t.TempDir()
	status := `{"lane_uuid":"lane-uuid","cwd":"` + filepath.ToSlash(executionRepo) + `"}`
	runner := &waspflowRunner{results: []waspflowRun{
		{result: CommandResult{Stdout: []byte(status)}},
		{result: CommandResult{Stdout: []byte(status)}},
		{result: CommandResult{Stdout: []byte("result-commit\n")}},
		{},
		{},
		{},
		{result: CommandResult{ExitCode: 3, Stderr: []byte("cleanup refused")}},
	}}
	driver := newTestWaspflowDriver(t, runner)
	driver.home = t.TempDir()

	// Act
	result, err := driver.Finalize(
		context.Background(),
		testWaspflowHandle(t, "devspecs-w09"),
		HandoffRequest{},
		t.TempDir(),
	)

	// Assert
	assert.Equal(t, ProviderResult{}, result)
	require.Error(t, err)
	assert.ErrorContains(t, err, `waspflow reap exited 3 without a readable native receipt`)
	assert.ErrorContains(t, err, `inspect lane "devspecs-w09": cleanup refused`)
	require.Len(t, runner.calls, 7)
	assert.Equal(t, "reap", runner.calls[6].args[0])
	assert.Equal(t, "devspecs-w09", runner.calls[6].args[1])
}

func TestClassifyWaspflowResult_WithProviderFailure_ReturnsBoundedFailure(t *testing.T) {
	// Arrange
	providerResult := "verify_failed"

	// Act
	state, failure := classifyWaspflowResult(providerResult)

	// Assert
	assert.Equal(t, "failed", state)
	require.NotNil(t, failure)
	assert.Equal(t, "provider_result", failure.Class)
	assert.Equal(t, "verify_failed", failure.Detail)
}

type waspflowCall struct {
	executable string
	args       []string
}

type waspflowRun struct {
	result CommandResult
	err    error
}

type waspflowRunner struct {
	calls   []waspflowCall
	results []waspflowRun
}

func (r *waspflowRunner) Run(_ context.Context, _ string, executable string, args ...string) (CommandResult, error) {
	r.calls = append(r.calls, waspflowCall{executable: executable, args: append([]string(nil), args...)})
	if len(r.results) == 0 {
		return CommandResult{}, errors.New("unexpected test command")
	}
	next := r.results[0]
	r.results = r.results[1:]
	return next.result, next.err
}

func newTestWaspflowDriver(t *testing.T, runner CommandRunner) *WaspflowDriver {
	t.Helper()
	driver, err := NewWaspflowDriver(runner, map[string]any{"executable": "fake-waspflow"})
	require.NoError(t, err)
	return driver
}

func testWaspflowHandle(t *testing.T, lane string) ProviderHandle {
	t.Helper()
	data, err := json.Marshal(waspflowHandleData{Lane: lane})
	require.NoError(t, err)
	return ProviderHandle{Name: "waspflow", RunID: "lane-uuid", Data: data}
}
