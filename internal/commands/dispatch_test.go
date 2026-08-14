package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/orchestration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDispatchCmd_Help_ExplainsDefaultMonitoringAndLifecycleOwnership(t *testing.T) {
	// Arrange
	cmd := newDispatchCmd(newDispatchFakeFactory(&dispatchFakeService{}))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"--help"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	assert.Contains(t, output.String(), "waits for provider completion")
	assert.Contains(t, output.String(), "Use --detach")
	assert.Contains(t, output.String(), "cannot checkpoint")
}

func TestDispatchCmd_WithDetach_ReturnsDurableDispatchWithoutResuming(t *testing.T) {
	// Arrange
	service := &dispatchFakeService{
		handle:     orchestration.DispatchHandle{DispatchID: "dispatch_01"},
		handlePath: "dispatches/dispatch_01/handle.json",
	}
	cmd := newDispatchCmd(newDispatchFakeFactory(service))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"task:sample", "--cwd", "repo", "--agent-provider", "codex", "--detach"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, service.dispatchCalls)
	assert.Equal(t, 0, service.resumeCalls)
	assert.Equal(t, "task:sample", service.options.Target)
	assert.Equal(t, "repo", service.options.ExecutionRepo)
	assert.Equal(t, "codex", service.options.AgentProvider)
	var actual dispatchOutput
	require.NoError(t, json.Unmarshal(output.Bytes(), &actual))
	assert.Equal(t, "dispatches/dispatch_01/handle.json", actual.DispatchPath)
	assert.Equal(t, "dispatch_01", actual.Dispatch.DispatchID)
}

func TestDispatchCmd_WithoutDetach_ResumesToFinalReceipt(t *testing.T) {
	// Arrange
	service := &dispatchFakeService{
		handle:      orchestration.DispatchHandle{DispatchID: "dispatch_02"},
		receipt:     orchestration.DispatchReceipt{DispatchID: "dispatch_02", ReceiptID: "receipt_02"},
		receiptPath: "dispatches/dispatch_02/receipt.json",
	}
	cmd := newDispatchCmd(newDispatchFakeFactory(service))
	var output bytes.Buffer
	var progress bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&progress)
	cmd.SetArgs([]string{"task:sample", "--op", "op-1"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, service.dispatchCalls)
	assert.Equal(t, 1, service.resumeCalls)
	assert.Equal(t, "dispatch_02", service.resumeIdentifier)
	assert.Contains(t, progress.String(), "Dispatch dispatch_02: provider run started")
	var actual dispatchReceiptOutput
	require.NoError(t, json.Unmarshal(output.Bytes(), &actual))
	assert.Equal(t, "dispatches/dispatch_02/receipt.json", actual.ReceiptPath)
	assert.Equal(t, "receipt_02", actual.Receipt.ReceiptID)
}

func TestDispatchStatusCmd_WithDispatchID_ReturnsProviderNeutralStatus(t *testing.T) {
	// Arrange
	service := &dispatchFakeService{status: orchestration.DispatchStatus{
		Schema: orchestration.DispatchStatusSchema, SchemaVersion: orchestration.SchemaVersion,
		DispatchID: "dispatch_03", State: "running",
	}}
	cmd := newDispatchCmd(newDispatchFakeFactory(service))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"status", "dispatch_03"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, service.statusCalls)
	assert.Equal(t, "dispatch_03", service.statusIdentifier)
	var actual orchestration.DispatchStatus
	require.NoError(t, json.Unmarshal(output.Bytes(), &actual))
	assert.Equal(t, "running", actual.State)
}

func TestDispatchResumeCmd_WithDispatchID_ReturnsFinalReceipt(t *testing.T) {
	// Arrange
	service := &dispatchFakeService{
		receipt:     orchestration.DispatchReceipt{DispatchID: "dispatch_03", ReceiptID: "receipt_03"},
		receiptPath: "dispatches/dispatch_03/receipt.json",
	}
	cmd := newDispatchCmd(newDispatchFakeFactory(service))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"resume", "dispatch_03"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, service.resumeCalls)
	assert.Equal(t, "dispatch_03", service.resumeIdentifier)
	var actual dispatchReceiptOutput
	require.NoError(t, json.Unmarshal(output.Bytes(), &actual))
	assert.Equal(t, "dispatches/dispatch_03/receipt.json", actual.ReceiptPath)
	assert.Equal(t, "receipt_03", actual.Receipt.ReceiptID)
}

func TestDispatchReceiptCmd_WithDispatchID_ReturnsNormalizedReceipt(t *testing.T) {
	// Arrange
	service := &dispatchFakeService{receipt: orchestration.DispatchReceipt{
		Schema: orchestration.DispatchReceiptSchema, SchemaVersion: orchestration.SchemaVersion,
		DispatchID: "dispatch_04", ReceiptID: "receipt_04",
	}}
	cmd := newDispatchCmd(newDispatchFakeFactory(service))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"receipt", "dispatch_04"})

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, service.receiptCalls)
	assert.Equal(t, "dispatch_04", service.receiptIdentifier)
	var actual orchestration.DispatchReceipt
	require.NoError(t, json.Unmarshal(output.Bytes(), &actual))
	assert.Equal(t, "receipt_04", actual.ReceiptID)
}

type dispatchFakeService struct {
	handle            orchestration.DispatchHandle
	handlePath        string
	status            orchestration.DispatchStatus
	receipt           orchestration.DispatchReceipt
	receiptPath       string
	options           orchestration.DispatchOptions
	dispatchCalls     int
	statusCalls       int
	resumeCalls       int
	receiptCalls      int
	statusIdentifier  string
	resumeIdentifier  string
	receiptIdentifier string
}

func (s *dispatchFakeService) Dispatch(_ context.Context, options orchestration.DispatchOptions) (orchestration.DispatchHandle, string, error) {
	s.dispatchCalls++
	s.options = options
	return s.handle, s.handlePath, nil
}

func (s *dispatchFakeService) Status(_ context.Context, identifier string) (orchestration.DispatchStatus, error) {
	s.statusCalls++
	s.statusIdentifier = identifier
	return s.status, nil
}

func (s *dispatchFakeService) Resume(_ context.Context, identifier string) (orchestration.DispatchReceipt, string, error) {
	s.resumeCalls++
	s.resumeIdentifier = identifier
	return s.receipt, s.receiptPath, nil
}

func (s *dispatchFakeService) Receipt(identifier string) (orchestration.DispatchReceipt, error) {
	s.receiptCalls++
	s.receiptIdentifier = identifier
	return s.receipt, nil
}

func newDispatchFakeFactory(service dispatchLifecycle) dispatchServiceFactory {
	return func() (dispatchLifecycle, error) {
		return service, nil
	}
}
