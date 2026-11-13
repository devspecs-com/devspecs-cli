package initflow

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeSelectedProfiles_WithExistingSections_PreservesAllSections(t *testing.T) {
	// Arrange
	enabled := true
	base := config.DefaultRepoConfig()
	base.Artifacts.TestCases = &enabled
	base.Experiments.SupportDocDiscovery = &enabled
	base.Integrations.Orchestration = config.OrchestrationConfig{Provider: "http", Options: map[string]any{"nested": map[string]any{"selection": "original"}}}
	base.Remotes = []config.RemoteConfig{{Version: 1, Name: "hosted", URL: "https://hub.example", TenantID: "11111111-1111-4111-8111-111111111111", RepoID: "22222222-2222-4222-8222-222222222222"}}
	// Act
	merged, err := MergeSelectedProfiles(base, []string{"cursor"}, nil, nil)
	// Assert
	require.NoError(t, err)
	require.NotNil(t, merged.Artifacts.TestCases)
	assert.True(t, *merged.Artifacts.TestCases)
	require.NotNil(t, merged.Experiments.SupportDocDiscovery)
	assert.True(t, *merged.Experiments.SupportDocDiscovery)
	assert.Equal(t, "http", merged.Integrations.Orchestration.Provider)
	require.Len(t, merged.Remotes, 1)
	assert.Equal(t, "https://hub.example", merged.Remotes[0].URL)
	require.Len(t, merged.Integrations.Orchestration.Options, 1)
	nested, ok := merged.Integrations.Orchestration.Options["nested"].(map[string]any)
	require.True(t, ok)
	require.Len(t, nested, 1)
	assert.Equal(t, "original", nested["selection"])
}
