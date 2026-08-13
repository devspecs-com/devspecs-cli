package commands

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigShowJSON_WithOrchestrationIntegration_ReturnsMachineReadableSelection(t *testing.T) {
	repoRoot, _ := setupV01Env(t)
	cfg := config.DefaultRepoConfig()
	cfg.Integrations.Orchestration = config.OrchestrationConfig{
		Provider: "waspflow",
		Options: map[string]any{
			"executable": "waspflow",
		},
	}
	require.NoError(t, config.WriteRepoConfig(repoRoot, cfg))
	output := &bytes.Buffer{}
	cmd := NewConfigCmd()
	cmd.SetArgs([]string{"show", "--json"})
	cmd.SetOut(output)

	err := cmd.Execute()

	require.NoError(t, err)
	var actual config.RepoConfig
	require.NoError(t, json.Unmarshal(output.Bytes(), &actual))
	assert.Equal(t, "waspflow", actual.Integrations.Orchestration.Provider)
	require.Len(t, actual.Integrations.Orchestration.Options, 1)
	assert.Equal(t, "waspflow", actual.Integrations.Orchestration.Options["executable"])
}

func TestConfigShowJSON_WithoutRepoConfig_ReturnsJSONWithoutHumanPreamble(t *testing.T) {
	setupV01Env(t)
	output := &bytes.Buffer{}
	cmd := NewConfigCmd()
	cmd.SetArgs([]string{"show", "--json"})
	cmd.SetOut(output)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, json.Valid(output.Bytes()))
	assert.NotContains(t, output.String(), "defaults")
}
