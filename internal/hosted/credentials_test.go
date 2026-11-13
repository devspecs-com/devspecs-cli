package hosted

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialJSON_WithPrivateToken_DoesNotSerializeSecret(t *testing.T) {
	// Arrange
	credential := Credential{sessionToken, sessionAt(time.Now())}
	// Act
	data, err := json.Marshal(credential)
	// Assert
	require.NoError(t, err)
	assert.NotContains(t, string(data), sessionToken)
}

func TestCredentialRecord_WithBoundedSession_RoundTripsExactBinding(t *testing.T) {
	// Arrange
	r := approvedRemote("https://hub.example/base")
	credential := Credential{sessionToken, sessionAt(time.Now())}
	encoded, err := encodeCredential(r, credential)
	require.NoError(t, err)
	// Act
	decoded, err := decodeCredential(r, encoded)
	// Assert
	require.NoError(t, err)
	assert.Less(t, len(encoded), 2048)
	assert.Equal(t, sessionID, decoded.SessionID)
	assert.Equal(t, sessionToken, decoded.Token)
}

func TestCredentialRecord_WithDifferentBasePath_RejectsStoredCredential(t *testing.T) {
	// Arrange
	r := approvedRemote("https://hub.example/base")
	encoded, err := encodeCredential(r, Credential{sessionToken, sessionAt(time.Now())})
	require.NoError(t, err)
	r.URL = "https://hub.example/other"
	// Act
	credential, err := decodeCredential(r, encoded)
	// Assert
	require.ErrorIs(t, err, ErrCredential)
	assert.Empty(t, credential)
}

func TestCredentialRecord_WithDifferentRepo_RejectsStoredCredential(t *testing.T) {
	// Arrange
	r := approvedRemote("https://hub.example")
	encoded, err := encodeCredential(r, Credential{sessionToken, sessionAt(time.Now())})
	require.NoError(t, err)
	r.RepoID = sessionID
	// Act
	credential, err := decodeCredential(r, encoded)
	// Assert
	require.ErrorIs(t, err, ErrCredential)
	assert.Empty(t, credential)
}

func TestCredentialRecord_WithOversizedValue_RejectsWithoutEcho(t *testing.T) {
	// Arrange
	r := approvedRemote("https://hub.example")
	// Act
	credential, err := decodeCredential(r, strings.Repeat("private-secret", 200))
	// Assert
	require.ErrorIs(t, err, ErrCredential)
	assert.Empty(t, credential)
	assert.NotContains(t, err.Error(), "private-secret")
}

func TestVaultKey_WithDifferentBasePath_UsesDifferentExactNamespace(t *testing.T) {
	// Arrange
	first := approvedRemote("https://hub.example/base")
	service, _, err := vaultKey(first)
	require.NoError(t, err)
	first.URL = "https://hub.example/other"
	// Act
	other, user, err := vaultKey(first)
	// Assert
	require.NoError(t, err)
	assert.NotEqual(t, service, other)
	assert.Equal(t, tenantID+"/"+repoID, user)
	decoded, err := hex.DecodeString(strings.TrimPrefix(other, "devspecs.specs-place.v1."))
	require.NoError(t, err)
	require.Len(t, decoded, 32)
}

func TestEnvironment_WithExplicitPair_ReturnsOnlyBoundToken(t *testing.T) {
	// Arrange
	r := approvedRemote("https://hub.example/base")
	getenv := func(key string) string {
		if key == "DEVSPECS_REMOTE_URL" {
			return r.URL
		}
		return serviceToken
	}
	// Act
	token, selected, err := Environment(r, getenv)
	// Assert
	require.NoError(t, err)
	assert.True(t, selected)
	assert.Equal(t, serviceToken, token)
}

func TestEnvironment_WithDifferentBasePath_DoesNotReturnToken(t *testing.T) {
	// Arrange
	r := approvedRemote("https://hub.example/base")
	getenv := func(key string) string {
		if key == "DEVSPECS_REMOTE_URL" {
			return "https://hub.example/other"
		}
		return serviceToken
	}
	// Act
	token, selected, err := Environment(r, getenv)
	// Assert
	require.ErrorIs(t, err, ErrCredential)
	assert.True(t, selected)
	assert.Empty(t, token)
	assert.NotContains(t, err.Error(), serviceToken)
}

func TestEnvironment_WithTokenOnly_RefusesAmbientOriginSelection(t *testing.T) {
	// Arrange
	r := approvedRemote("https://hub.example")
	getenv := func(key string) string {
		if key == "DEVSPECS_REMOTE_TOKEN" {
			return serviceToken
		}
		return ""
	}
	// Act
	token, selected, err := Environment(r, getenv)
	// Assert
	require.ErrorIs(t, err, ErrCredential)
	assert.True(t, selected)
	assert.Empty(t, token)
}

func TestEnvironment_WithoutPair_DoesNotSelectCredentials(t *testing.T) {
	// Arrange
	r := approvedRemote("https://hub.example")
	// Act
	token, selected, err := Environment(r, func(string) string { return "" })
	// Assert
	require.NoError(t, err)
	assert.False(t, selected)
	assert.Empty(t, token)
}

func TestEnvironment_WithForeignGitHubToken_RejectsIdentitySubstitution(t *testing.T) {
	// Arrange
	r := approvedRemote("https://hub.example")
	getenv := func(key string) string {
		if key == "DEVSPECS_REMOTE_URL" {
			return r.URL
		}
		return "gho_foreign-token"
	}
	// Act
	token, _, err := Environment(r, getenv)
	// Assert
	require.ErrorIs(t, err, ErrCredential)
	assert.Empty(t, token)
}
