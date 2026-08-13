package main

import (
	"bytes"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/orchestration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRootCmd_Help_DescribesExplicitNonPromotingCompanion(t *testing.T) {
	// Arrange
	root := newRootCmd(&orchestration.Service{})
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"--help"})

	// Act
	err := root.Execute()

	// Assert
	require.NoError(t, err)
	assert.Contains(t, output.String(), "DevSpecs-owned orchestration adapter host")
	assert.Contains(t, output.String(), "never checkpoints or promotes")
}

func TestNewRootCmd_WithLifecycleCommands_ExposesBoundedCompanionSurface(t *testing.T) {
	// Arrange
	root := newRootCmd(&orchestration.Service{})

	// Act
	commands := root.Commands()

	// Assert
	require.Len(t, commands, 6)
	assert.Equal(t, "dispatch", commands[0].Name())
	assert.Equal(t, "finalize", commands[1].Name())
	assert.Equal(t, "preflight", commands[2].Name())
	assert.Equal(t, "receipt", commands[3].Name())
	assert.Equal(t, "run", commands[4].Name())
	assert.Equal(t, "wait", commands[5].Name())
}
