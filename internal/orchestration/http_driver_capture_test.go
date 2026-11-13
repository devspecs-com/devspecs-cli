package orchestration

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPDriverCaptureExec_WithCallerBearerAndTelemetryEnabled_DisablesTelemetryAndOmitsSecrets(t *testing.T) {
	t.Setenv("HTTP_CAPTURE_FIXTURE_TOKEN", "synthetic-private-bearer")
	t.Setenv("DEVSPECS_TELEMETRY", "1")
	t.Setenv("GIT_DIR", "not-the-actual-git-directory")
	runner := httpDriverCaptureExec{tokenEnv: "HTTP_CAPTURE_FIXTURE_TOKEN"}

	result, err := runner.Run(t.Context(), t.TempDir(), "node", "-e", `process.stdout.write(JSON.stringify({telemetry:process.env.DEVSPECS_TELEMETRY,token:process.env.HTTP_CAPTURE_FIXTURE_TOKEN??null,gitDir:process.env.GIT_DIR??null}))`)

	require.NoError(t, err)
	require.Zero(t, result.ExitCode)
	var actual map[string]any
	require.NoError(t, json.Unmarshal(result.Stdout, &actual))
	require.Len(t, actual, 3)
	assert.Equal(t, "0", actual["telemetry"])
	assert.Nil(t, actual["token"])
	assert.Nil(t, actual["gitDir"])
	assert.NotContains(t, string(result.Stdout), "synthetic-private-bearer")
}
