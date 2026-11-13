package commands

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDispatchCmd_WithReadOnlyFlag_ForwardsExplicitGoalWithoutImplicitMCP(t *testing.T) {
	service := &dispatchFakeService{}
	cmd := newDispatchCmd(newDispatchFakeFactory(service))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"task:fixture", "--agent-provider", "fixture-agent", "--read-only", "--detach"})

	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, service.options.ReadOnly)
	assert.Empty(t, service.options.MCP)
	assert.Equal(t, 1, service.dispatchCalls)
}

func TestDispatchCmd_WithoutReadOnlyFlag_PreservesNativeDefaults(t *testing.T) {
	service := &dispatchFakeService{}
	cmd := newDispatchCmd(newDispatchFakeFactory(service))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"task:fixture", "--agent-provider", "fixture-agent", "--detach"})

	err := cmd.Execute()

	require.NoError(t, err)
	assert.False(t, service.options.ReadOnly)
	assert.Equal(t, "auto", service.options.MCP)
}

func TestDispatchCmd_WithReadOnlyAndExplicitMCP_PreservesOptionForDriverRefusal(t *testing.T) {
	service := &dispatchFakeService{}
	cmd := newDispatchCmd(newDispatchFakeFactory(service))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"task:fixture", "--agent-provider", "fixture-agent", "--read-only", "--mcp", "external", "--detach"})

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "external", service.options.MCP)
}

func TestDispatchCmd_Help_ExplainsExperimentalReadOnlyBoundary(t *testing.T) {
	cmd := NewDispatchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, out.String(), "--read-only")
	assert.Contains(t, out.String(), "inspection goals only, not implementation tasks")
}

func TestDispatchCmd_WithAllowEdits_ForwardsExplicitEditedGoalWithoutImplicitMCP(t *testing.T) {
	service := &dispatchFakeService{}
	cmd := newDispatchCmd(newDispatchFakeFactory(service))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"task:fixture", "--agent-provider", "fixture-agent", "--allow-edits", "--detach"})

	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, service.options.AllowEdits)
	assert.False(t, service.options.ReadOnly)
	assert.Empty(t, service.options.MCP)
}

func TestDispatchCmd_WithConflictingHTTPGoals_RefusesBeforeService(t *testing.T) {
	service := &dispatchFakeService{}
	cmd := newDispatchCmd(newDispatchFakeFactory(service))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"task:fixture", "--agent-provider", "fixture-agent", "--allow-edits", "--read-only", "--detach"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Zero(t, service.dispatchCalls)
}

func TestDispatchCmd_WithExplicitNativeVerificationTimeout_PreservesUnsupportedFlagForHTTPRefusal(t *testing.T) {
	service := &dispatchFakeService{}
	cmd := newDispatchCmd(newDispatchFakeFactory(service))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"task:fixture", "--agent-provider", "fixture-agent", "--allow-edits", "--verify-timeout", "600", "--detach"})

	err := cmd.Execute()

	require.NoError(t, err)
	require.Len(t, service.options.ExplicitNativeFlags, 1)
	assert.Equal(t, "verify-timeout", service.options.ExplicitNativeFlags[0])
}
