package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const remoteYAML = `version: 1
remotes:
  - version: 1
    name: hosted
    url: https://hub.example/base
    tenant_id: 11111111-1111-4111-8111-111111111111
    repo_id: 22222222-2222-4222-8222-222222222222
`

func TestRemoteConfig_WithVersion1Metadata_DecodesExplicitBinding(t *testing.T) {
	// Arrange
	var cfg RepoConfig
	// Act
	err := yaml.Unmarshal([]byte(remoteYAML), &cfg)
	// Assert
	require.NoError(t, err)
	require.Len(t, cfg.Remotes, 1)
	assert.Equal(t, 1, cfg.Remotes[0].Version)
	assert.Equal(t, "hosted", cfg.Remotes[0].Name)
	assert.Equal(t, "https://hub.example/base", cfg.Remotes[0].URL)
}

func TestRemoteConfig_WithUnknownFutureField_FailsClosed(t *testing.T) {
	// Arrange
	var cfg RepoConfig
	// Act
	err := yaml.Unmarshal([]byte(remoteYAML+"    access_token: private-secret\n"), &cfg)
	// Assert
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private-secret")
}

func TestRemoteConfig_WithFutureRemoteVersion_FailsClosed(t *testing.T) {
	// Arrange
	var cfg RepoConfig
	// Act
	err := yaml.Unmarshal([]byte("version: 1\nremotes:\n  - version: 2\n    name: hosted\n    url: https://hub.example\n    tenant_id: 11111111-1111-4111-8111-111111111111\n    repo_id: 22222222-2222-4222-8222-222222222222\n"), &cfg)
	// Assert
	require.Error(t, err)
}

func TestValidateRemoteOrigin_WithCredentialURL_RejectsWithoutEcho(t *testing.T) {
	// Arrange
	value := "https://user:private-secret@hub.example/base"
	// Act
	err := ValidateRemoteOrigin(value)
	// Assert
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private-secret")
}

func TestValidateRemoteOrigin_WithEncodedPath_RejectsAmbiguousBinding(t *testing.T) {
	// Arrange
	value := "https://hub.example/base%2fother"
	// Act
	err := ValidateRemoteOrigin(value)
	// Assert
	require.Error(t, err)
}

func TestValidateRemoteOrigin_WithBackslash_RejectsAmbiguousBinding(t *testing.T) {
	// Arrange
	value := `https://hub.example/base\other`
	// Act
	err := ValidateRemoteOrigin(value)
	// Assert
	require.Error(t, err)
}

func TestValidateRemoteOrigin_WithQuery_RejectsSecretBearingEndpoint(t *testing.T) {
	// Arrange
	value := "https://hub.example/base?token=private-secret"
	// Act
	err := ValidateRemoteOrigin(value)
	// Assert
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private-secret")
}

func TestValidateRemoteOrigin_WithPlainHTTP_RejectsInsecureEndpoint(t *testing.T) {
	// Arrange
	value := "http://hub.example"
	// Act
	err := ValidateRemoteOrigin(value)
	// Assert
	require.Error(t, err)
}

func TestCloneRepoConfig_WithRemoteMetadata_DoesNotAliasEntries(t *testing.T) {
	// Arrange
	base := DefaultRepoConfig()
	base.Remotes = []RemoteConfig{{Version: 1, Name: "hosted", URL: "https://hub.example"}}
	copy := CloneRepoConfig(base)
	require.Len(t, copy.Remotes, 1)
	// Act
	copy.Remotes[0].URL = "https://changed.example"
	// Assert
	require.Len(t, base.Remotes, 1)
	assert.Equal(t, "https://hub.example", base.Remotes[0].URL)
}
