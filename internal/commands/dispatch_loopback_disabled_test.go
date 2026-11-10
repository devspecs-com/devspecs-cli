//go:build !devspecs_http_loopback

package commands

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDispatchCmd_WhenNormalBuildHasLoopbackEnv_UsesOrdinaryValidation(t *testing.T) {
	// Arrange
	t.Setenv("DEVSPECS_HTTP_LOOPBACK_ENDPOINT", "https://127.0.0.1:1/v2")
	t.Setenv("DEVSPECS_TELEMETRY", "0")
	cmd := NewDispatchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"task:fixture", "--detach"})
	cmd.SilenceErrors, cmd.SilenceUsage = true, true

	// Act
	err := cmd.Execute()

	// Assert
	require.EqualError(t, err, "choose exactly one of --agent-provider or --op")
	assert.Empty(t, out.String())
}
