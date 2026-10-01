package commands

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvalChildEnvironment_WithInheritedOptIn_ForcesTelemetryOff(t *testing.T) {
	// Arrange
	t.Setenv("DEVSPECS_HOME", "/stable")
	t.Setenv("DEVSPECS_TELEMETRY", "debug")

	// Act
	environment := evalChildEnvironment("/temporary/eval")
	homeValues := evalEnvironmentValues(environment, "DEVSPECS_HOME")
	telemetryValues := evalEnvironmentValues(environment, "DEVSPECS_TELEMETRY")

	// Assert
	require.Len(t, homeValues, 1)
	assert.Equal(t, "/temporary/eval", homeValues[0])
	require.Len(t, telemetryValues, 1)
	assert.Equal(t, "0", telemetryValues[0])
}

func TestSetEvalHome_WithInheritedValues_SetsIsolatedEnvironment(t *testing.T) {
	// Arrange
	t.Setenv("DEVSPECS_HOME", "/stable")
	t.Setenv("DEVSPECS_TELEMETRY", "debug")

	// Act
	restore, err := setEvalHome("/temporary/eval")
	require.NoError(t, err)
	defer restore()

	// Assert
	assert.Equal(t, "/temporary/eval", os.Getenv("DEVSPECS_HOME"))
	assert.Equal(t, "0", os.Getenv("DEVSPECS_TELEMETRY"))
}

func TestSetEvalHome_WhenRestored_ReinstatesInheritedEnvironment(t *testing.T) {
	// Arrange
	t.Setenv("DEVSPECS_HOME", "/stable")
	t.Setenv("DEVSPECS_TELEMETRY", "debug")

	// Act
	restore, err := setEvalHome("/temporary/eval")
	require.NoError(t, err)
	restore()

	// Assert
	assert.Equal(t, "/stable", os.Getenv("DEVSPECS_HOME"))
	assert.Equal(t, "debug", os.Getenv("DEVSPECS_TELEMETRY"))
}

func evalEnvironmentValues(environment []string, key string) []string {
	values := []string{}
	for _, item := range environment {
		itemKey, itemValue, ok := strings.Cut(item, "=")
		if ok && itemKey == key {
			values = append(values, itemValue)
		}
	}
	return values
}
