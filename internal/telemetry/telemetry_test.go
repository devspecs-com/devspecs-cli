package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeProperties_WithSensitiveFields_KeepsOnlyAllowedCoarseFields(t *testing.T) {
	input := map[string]any{
		"command":             "scan",
		"success":             true,
		"query":               "do not send me",
		"repo_path":           "/private/repo",
		"query_length_bucket": "11-50",
	}

	properties := sanitizeProperties(input)

	assert.Len(t, properties, 3)
	assert.Equal(t, "scan", properties["command"])
	assert.Equal(t, true, properties["success"])
	assert.Equal(t, "11-50", properties["query_length_bucket"])
	assert.NotContains(t, properties, "query")
	assert.NotContains(t, properties, "repo_path")
}

func TestSanitizeProperties_WithComposeMetadata_KeepsDocumentTypeAndFormat(t *testing.T) {
	input := map[string]any{
		"document_type": "adr",
		"format":        "outcome-first",
	}

	properties := sanitizeProperties(input)

	require.Len(t, properties, 2)
	assert.Equal(t, "adr", properties["document_type"])
	assert.Equal(t, "outcome-first", properties["format"])
}

func TestCountBucket_WithZero_ReturnsZeroBucket(t *testing.T) {
	actual := CountBucket(0)

	assert.Equal(t, "0", actual)
}

func TestCountBucket_WithOne_ReturnsOneToTenBucket(t *testing.T) {
	actual := CountBucket(1)

	assert.Equal(t, "1-10", actual)
}

func TestCountBucket_WithTen_ReturnsOneToTenBucket(t *testing.T) {
	actual := CountBucket(10)

	assert.Equal(t, "1-10", actual)
}

func TestCountBucket_WithEleven_ReturnsElevenToFiftyBucket(t *testing.T) {
	actual := CountBucket(11)

	assert.Equal(t, "11-50", actual)
}

func TestCountBucket_WithFifty_ReturnsElevenToFiftyBucket(t *testing.T) {
	actual := CountBucket(50)

	assert.Equal(t, "11-50", actual)
}

func TestCountBucket_WithFiftyOne_ReturnsFiftyOneToHundredBucket(t *testing.T) {
	actual := CountBucket(51)

	assert.Equal(t, "51-100", actual)
}

func TestCountBucket_WithHundred_ReturnsFiftyOneToHundredBucket(t *testing.T) {
	actual := CountBucket(100)

	assert.Equal(t, "51-100", actual)
}

func TestCountBucket_WithHundredOne_ReturnsHundredOneToFiveHundredBucket(t *testing.T) {
	actual := CountBucket(101)

	assert.Equal(t, "101-500", actual)
}

func TestCountBucket_WithFiveHundredOne_ReturnsFiveHundredOnePlusBucket(t *testing.T) {
	actual := CountBucket(501)

	assert.Equal(t, "501+", actual)
}

func TestRecord_WithEnabledTelemetry_SendsSanitizedEventAndPersistsAnonymousID(t *testing.T) {
	received := make(chan telemetryRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		received <- telemetryRequest{
			Body:        body,
			ReadError:   err,
			Method:      r.Method,
			ContentType: r.Header.Get("content-type"),
			UserAgent:   r.Header.Get("user-agent"),
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	home := enableTelemetryForTest(t, server.URL)

	Record(context.Background(), "find_completed", map[string]any{
		"command":             "find",
		"success":             true,
		"result_count_bucket": "1-10",
		"query":               "private search text",
	})

	request := <-received
	require.NoError(t, request.ReadError)
	assert.Equal(t, http.MethodPost, request.Method)
	assert.Equal(t, "application/json", request.ContentType)
	assert.Contains(t, request.UserAgent, "devspecs-cli/")
	var event Event
	require.NoError(t, json.Unmarshal(request.Body, &event))
	assert.Equal(t, "find_completed", event.Event)
	assert.NotEmpty(t, event.AnonymousID)
	assert.NotEmpty(t, event.SessionID)
	assert.NotEmpty(t, event.OS)
	assert.NotEmpty(t, event.Arch)
	assert.NotEmpty(t, event.OccurredAt)
	require.Len(t, event.Properties, 3)
	assert.Equal(t, "find", event.Properties["command"])
	assert.Equal(t, true, event.Properties["success"])
	assert.Equal(t, "1-10", event.Properties["result_count_bucket"])
	assert.NotContains(t, event.Properties, "query")
	assert.FileExists(t, filepath.Join(home, "telemetry.json"))
}

func TestRecord_WithDisabledTelemetry_DoesNotSendRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	setTelemetryProcessName(t)
	t.Setenv("DEVSPECS_TELEMETRY", "off")
	t.Setenv("DEVSPECS_TELEMETRY_URL", server.URL)
	t.Setenv("CI", "")

	Record(context.Background(), "scan_completed", nil)

	assert.Zero(t, requests.Load())
}

func TestRecordCommand_WithEnabledTelemetry_SendsCoarseCompletionProperties(t *testing.T) {
	received := make(chan telemetryRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		received <- telemetryRequest{Body: body, ReadError: err}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	enableTelemetryForTest(t, server.URL)

	RecordCommand("scan", false, 750*time.Millisecond, map[string]any{"artifact_count_bucket": "51-100"})

	request := <-received
	require.NoError(t, request.ReadError)
	var event Event
	require.NoError(t, json.Unmarshal(request.Body, &event))
	assert.Equal(t, "scan_completed", event.Event)
	require.Len(t, event.Properties, 4)
	assert.Equal(t, "scan", event.Properties["command"])
	assert.Equal(t, false, event.Properties["success"])
	assert.Equal(t, "500-999ms", event.Properties["duration_bucket"])
	assert.Equal(t, "51-100", event.Properties["artifact_count_bucket"])
}

func TestAnonymousID_WithExistingStoredID_ReusesValue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	require.NoError(t, os.WriteFile(filepath.Join(home, "telemetry.json"), []byte(`{"anonymous_id":"anon_existing"}`), 0o600))

	id := anonymousID()

	assert.Equal(t, "anon_existing", id)
}

func TestSanitizeProperties_WithEmptyInput_ReturnsNil(t *testing.T) {
	properties := sanitizeProperties(nil)

	assert.Nil(t, properties)
}

func TestSanitizeProperties_WithAllowedValueTypes_NormalizesValues(t *testing.T) {
	longValue := "12345678901234567890123456789012345678901234567890123456789012345678901234567890-extra"
	input := map[string]any{
		"error_class":            longValue,
		"artifact_count_bucket":  7,
		"new_count_bucket":       int64(8),
		"updated_count_bucket":   9.5,
		"unchanged_count_bucket": []string{"value"},
	}

	properties := sanitizeProperties(input)

	require.Len(t, properties, 5)
	assert.Equal(t, longValue[:80], properties["error_class"])
	assert.Equal(t, 7, properties["artifact_count_bucket"])
	assert.Equal(t, int64(8), properties["new_count_bucket"])
	assert.Equal(t, 9.5, properties["updated_count_bucket"])
	assert.Equal(t, "[value]", properties["unchanged_count_bucket"])
}

func TestLoadConfig_WithCustomEndpoint_ReturnsEnabledConfiguration(t *testing.T) {
	setTelemetryProcessName(t)
	t.Setenv("DEVSPECS_TELEMETRY", "debug")
	t.Setenv("DEVSPECS_TELEMETRY_URL", "https://example.test/telemetry")
	t.Setenv("CI", "")

	config := loadConfig()

	assert.True(t, config.enabled)
	assert.True(t, config.debug)
	assert.Equal(t, "https://example.test/telemetry", config.endpoint)
}

func TestLoadConfig_InTestBinary_ReturnsDisabledConfiguration(t *testing.T) {
	t.Setenv("DEVSPECS_TELEMETRY", "on")
	t.Setenv("CI", "")

	config := loadConfig()

	assert.False(t, config.enabled)
}

func TestFirstEnv_WithBlankPrimary_ReturnsTrimmedFallback(t *testing.T) {
	t.Setenv("DEVSPECS_PRIMARY", "  ")
	t.Setenv("DEVSPECS_FALLBACK", " value ")

	value := firstEnv("DEVSPECS_PRIMARY", "DEVSPECS_FALLBACK")

	assert.Equal(t, "value", value)
}

func TestDisabledMode_WithOffValue_ReturnsTrue(t *testing.T) {
	disabled := disabledMode(" OFF ")

	assert.True(t, disabled)
}

func TestDisabledMode_WithEnabledValue_ReturnsFalse(t *testing.T) {
	disabled := disabledMode("on")

	assert.False(t, disabled)
}

func TestQueryLengthBucket_WithWhitespace_ReturnsTrimmedLengthBucket(t *testing.T) {
	bucket := QueryLengthBucket("  twelve chars  ")

	assert.Equal(t, "11-50", bucket)
}

func TestDurationBucket_WithSubHundredMilliseconds_ReturnsFastestBucket(t *testing.T) {
	bucket := durationBucket(99 * time.Millisecond)

	assert.Equal(t, "<100ms", bucket)
}

func TestDurationBucket_WithFourHundredMilliseconds_ReturnsHundredsBucket(t *testing.T) {
	bucket := durationBucket(400 * time.Millisecond)

	assert.Equal(t, "100-499ms", bucket)
}

func TestDurationBucket_WithTwoSeconds_ReturnsSingleDigitSecondsBucket(t *testing.T) {
	bucket := durationBucket(2 * time.Second)

	assert.Equal(t, "1-4s", bucket)
}

func TestDurationBucket_WithTenSeconds_ReturnsDoubleDigitSecondsBucket(t *testing.T) {
	bucket := durationBucket(10 * time.Second)

	assert.Equal(t, "5-29s", bucket)
}

func TestDurationBucket_WithOneMinute_ReturnsMinuteBucket(t *testing.T) {
	bucket := durationBucket(time.Minute)

	assert.Equal(t, "30-119s", bucket)
}

func TestDurationBucket_WithTwoMinutes_ReturnsOpenEndedSecondsBucket(t *testing.T) {
	bucket := durationBucket(2 * time.Minute)

	assert.Equal(t, "120s+", bucket)
}

type telemetryRequest struct {
	Body        []byte
	ReadError   error
	Method      string
	ContentType string
	UserAgent   string
}

func enableTelemetryForTest(t *testing.T, endpoint string) string {
	t.Helper()
	setTelemetryProcessName(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)
	t.Setenv("DEVSPECS_TELEMETRY", "on")
	t.Setenv("DEVSPECS_TELEMETRY_URL", endpoint)
	t.Setenv("CI", "")
	return home
}

func setTelemetryProcessName(t *testing.T) {
	t.Helper()
	original := os.Args[0]
	os.Args[0] = "ds"
	t.Cleanup(func() {
		os.Args[0] = original
	})
}
