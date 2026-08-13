package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRepoConfig_WithOrchestrationIntegration_PreservesProviderAndOptions(t *testing.T) {
	repoRoot := t.TempDir()
	writeRepoConfigFixture(t, repoRoot, `version: 1
sources: []
integrations:
  orchestration:
    provider: waspflow
    options:
      executable: /usr/local/bin/waspflow
      isolate: true
`)

	loaded, err := LoadRepoConfig(repoRoot)

	require.NoError(t, err)
	assert.Equal(t, "waspflow", loaded.Integrations.Orchestration.Provider)
	require.Len(t, loaded.Integrations.Orchestration.Options, 2)
	assert.Equal(t, "/usr/local/bin/waspflow", loaded.Integrations.Orchestration.Options["executable"])
	assert.Equal(t, true, loaded.Integrations.Orchestration.Options["isolate"])
}

func TestWriteRepoConfig_WithOrchestrationIntegration_RoundTripsSelection(t *testing.T) {
	repoRoot := t.TempDir()
	cfg := DefaultRepoConfig()
	cfg.Integrations.Orchestration = OrchestrationConfig{
		Provider: "waspflow",
		Options: map[string]any{
			"executable": "waspflow",
		},
	}

	err := WriteRepoConfig(repoRoot, cfg)

	require.NoError(t, err)
	written, err := os.ReadFile(RepoConfigPath(repoRoot))
	require.NoError(t, err)
	assert.Contains(t, string(written), "provider: waspflow")
	assert.Contains(t, string(written), "executable: waspflow")
}

func TestCloneRepoConfig_WithNestedOrchestrationOptions_DoesNotAliasInput(t *testing.T) {
	cfg := DefaultRepoConfig()
	cfg.Integrations.Orchestration.Options = map[string]any{
		"environment": map[string]any{
			"profile": "default",
		},
	}

	cloned := CloneRepoConfig(cfg)
	clonedEnvironment := cloned.Integrations.Orchestration.Options["environment"].(map[string]any)
	clonedEnvironment["profile"] = "isolated"

	originalEnvironment := cfg.Integrations.Orchestration.Options["environment"].(map[string]any)
	assert.Equal(t, "default", originalEnvironment["profile"])
}

func TestLoadRepoConfig_WithInvalidOrchestrationProvider_ReturnsError(t *testing.T) {
	repoRoot := t.TempDir()
	writeRepoConfigFixture(t, repoRoot, `version: 1
sources: []
integrations:
  orchestration:
    provider: Wasp Flow
`)

	loaded, err := LoadRepoConfig(repoRoot)

	assert.Nil(t, loaded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "integrations.orchestration.provider")
}
