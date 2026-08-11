package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
