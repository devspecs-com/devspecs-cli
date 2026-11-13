package orchestration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPDriverReceiptGolden_WithReviewedProviderExtension_AcceptsPublishedShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "ghost-hq-result-bundle-9e1ef571.json"))
	require.NoError(t, err)

	receipt, err := decodeHTTPDriverReceipt(raw)

	require.NoError(t, err)
	assert.Equal(t, "sha256:9351a8f3601292cdfb97a6d083d9a94ce0cb2fdf44539870c7d7b1871dcef0aa", httpDispatchDigest(raw))
	assert.Equal(t, "ghost-hq", receipt.Wire.Provider.Name)
	assert.Equal(t, "4f742c33bf546637a8d1877ec6d2c77f12ed2a56", receipt.Wire.Repository.SourceCommit)
	require.NotNil(t, receipt.Wire.Repository.ResultCommit)
	assert.Equal(t, "1111111111111111111111111111111111111111", *receipt.Wire.Repository.ResultCommit)
	assert.Equal(t, "2222222222222222222222222222222222222222", receipt.Tree)
	require.NotNil(t, receipt.Bundle)
	assert.Equal(t, "result-bundle", receipt.Bundle.ID)
	assert.EqualValues(t, 512, receipt.Bundle.Size)
	assert.Equal(t, "application/x-git-bundle", receipt.Bundle.MediaType)
	assert.Equal(t, "not_requested", receipt.Wire.Report.State)
	assert.Equal(t, "not_requested", receipt.Wire.Verification.State)
}

func TestHTTPDriverReceiptGolden_WithReviewedProviderExtension_UnchangedPilotDecoderRejects(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "ghost-hq-result-bundle-9e1ef571.json"))
	require.NoError(t, err)
	var receipt httpDispatchReceiptAdmissionWire

	accepted := decodeHTTPDispatch(raw, "Receipt", &receipt)

	assert.False(t, accepted)
}

func TestHTTPDriverStrictJSON_WithCaseAliasedConsent_RejectsNonSchemaField(t *testing.T) {
	var cfg httpDriverProfile

	accepted := strictHTTPDriverJSON([]byte(`{"read_only":false,"READ_ONLY":true}`), &cfg)

	assert.False(t, accepted)
}

func TestHTTPDriverStrictJSON_WithCaseAliasedVerification_RejectsNonSchemaField(t *testing.T) {
	var cfg httpDriverProfile

	accepted := strictHTTPDriverJSON([]byte(`{"verification":{"profile":"node-unit","STRENGTH":"suite"}}`), &cfg)

	assert.False(t, accepted)
}
