package commands

import (
	"encoding/json"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemotePush_WithActualConflictResponse_EmitsSchemaValidatedSafeError(t *testing.T) {
	f := newRemotePushFixture(t)
	f.mode = "conflict"
	command, stdout := f.command(false)

	err := command.Execute()

	var failure remoteFailure
	require.ErrorAs(t, err, &failure)
	f.assertErrorSchema(t, err)
	assert.Equal(t, "record_conflict", failure.Code)
	assert.Empty(t, stdout.String())
	require.Len(t, f.requests, 2)
	require.Len(t, f.bodies, 1)
}

func TestRemoteResultSchema_WithPublishedMissingURL_RejectsIncompleteResult(t *testing.T) {
	schema := localRemoteSchema(t, "remote-result.v1.schema.json")
	var instance any
	require.NoError(t, json.Unmarshal([]byte(`{"schema":"devspecs.remote.result","schema_version":1,"status":"published","credential_source":"environment","remote":{"version":1,"name":"hosted","url":"https://hub.example","tenant_id":"11111111-1111-4111-8111-111111111111","repo_id":"22222222-2222-4222-8222-222222222222"}}`), &instance))

	err := schema.Validate(instance)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "url")
}

func TestRemoteResultSchema_WithURLOnOldErrorShape_RejectsUnversionedExtension(t *testing.T) {
	schema := localRemoteSchema(t, "remote-result.v1.schema.json")
	var instance any
	require.NoError(t, json.Unmarshal([]byte(`{"schema":"devspecs.remote.result","schema_version":1,"status":"error","error":{"code":"access_denied","message":"denied"},"url":"https://hub.example/v1/tenants/11111111-1111-4111-8111-111111111111/repos/22222222-2222-4222-8222-222222222222/snapshots/66666666-6666-4666-8666-666666666666"}`), &instance))

	err := schema.Validate(instance)

	require.Error(t, err)
}

func TestRemoteResultSchema_WithPublishedNonHTTPSURL_RejectsInvalidContract(t *testing.T) {
	schema := localRemoteSchema(t, "remote-result.v1.schema.json")
	instance := remoteResult{Schema: "devspecs.remote.result", SchemaVersion: 1, Status: "published", CredentialSource: "environment", Remote: &config.RemoteConfig{Version: 1, Name: "hosted", URL: "https://hub.example", TenantID: testRemoteTenant, RepoID: testRemoteRepo}, URL: "http://hub.example/v1/tenants/11111111-1111-4111-8111-111111111111/repos/22222222-2222-4222-8222-222222222222/snapshots/66666666-6666-4666-8666-666666666666"}
	raw, marshalErr := json.Marshal(instance)
	require.NoError(t, marshalErr)
	var value any
	require.NoError(t, json.Unmarshal(raw, &value))

	err := schema.Validate(value)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "url")
}
