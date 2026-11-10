//go:build devspecs_http_loopback

package orchestration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadHTTPDispatchPending_WithStoredRecord_RestoresFrozenRequestAndBinding(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	pending := loopbackStatePendingFixture(store.Root)
	_, err := store.writeHTTPDispatchPending(pending)
	require.NoError(t, err)

	// Act
	got, err := store.readHTTPDispatchPending(pending.DispatchID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, pending.DispatchID, got.DispatchID)
	assert.Equal(t, pending.RequestSHA256, got.RequestSHA256)
	assert.Equal(t, pending.RequestSHA256, httpDispatchDigest(got.Request))
	assert.Equal(t, pending.Endpoint, got.Endpoint)
	assert.Equal(t, pending.TrustSHA256, got.TrustSHA256)
	assert.Equal(t, pending.Provider, got.Provider)
	assert.Equal(t, pending.Checkout, got.Checkout)
	assert.Equal(t, pending.ExpiresAt, got.ExpiresAt)
}

func TestWriteHTTPDispatchPending_WithIdenticalReplay_LeavesOriginalInPlace(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	pending := loopbackStatePendingFixture(store.Root)
	path, err := store.writeHTTPDispatchPending(pending)
	require.NoError(t, err)
	before, err := os.Stat(path)
	require.NoError(t, err)

	// Act
	published, err := store.writeHTTPDispatchPending(pending)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(store.Root, httpDispatchReceiptDir, pending.DispatchID+".pending.json"), published)
	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after))
}

func TestWriteHTTPDispatchPending_WithChangedExpiration_RefusesOverwrite(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	pending := loopbackStatePendingFixture(store.Root)
	path, err := store.writeHTTPDispatchPending(pending)
	require.NoError(t, err)
	replay := pending
	replay.ExpiresAt = pending.ExpiresAt.Add(time.Minute)

	// Act
	published, err := store.writeHTTPDispatchPending(replay)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchReceiptConflict)
	assert.Empty(t, published)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var record httpLoopbackPending
	require.NoError(t, json.Unmarshal(data, &record))
	assert.Equal(t, pending.ExpiresAt, record.ExpiresAt)
}

func TestWriteHTTPDispatchPending_WithOversizedRequest_DoesNotCreateNamespace(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	pending := loopbackStatePendingFixture(store.Root)
	pending.Request = make([]byte, httpDispatchRequestBody+1)
	pending.RequestSHA256 = httpDispatchDigest(pending.Request)

	// Act
	path, err := store.writeHTTPDispatchPending(pending)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchLimit)
	assert.Empty(t, path)
	assert.NoDirExists(t, filepath.Join(store.Root, httpDispatchReceiptDir))
}

func TestWriteHTTPDispatchPending_WithoutProvider_RejectsBinding(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	pending := loopbackStatePendingFixture(store.Root)
	pending.Provider = ""

	// Act
	path, err := store.writeHTTPDispatchPending(pending)

	// Assert
	require.Error(t, err)
	assert.Empty(t, path)
	assert.NoDirExists(t, filepath.Join(store.Root, httpDispatchReceiptDir))
}

func TestReadHTTPDispatchPending_WithTamperedRequest_RejectsDigestMismatch(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	pending := loopbackStatePendingFixture(store.Root)
	pending.Request = []byte(`{"changed":true}`)
	data, err := json.Marshal(pending)
	require.NoError(t, err)
	dir := filepath.Join(store.Root, httpDispatchReceiptDir)
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, pending.DispatchID+".pending.json"), data, 0o600))

	// Act
	got, err := store.readHTTPDispatchPending(pending.DispatchID)

	// Assert
	require.ErrorContains(t, err, "digest mismatch")
	assert.Nil(t, got.Request)
}

func TestReadHTTPDispatchPending_WithoutNamespace_ReturnsNotExist(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}

	// Act
	_, err := store.readHTTPDispatchPending(testHTTPDispatchID)

	// Assert
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.NoDirExists(t, filepath.Join(store.Root, httpDispatchReceiptDir))
}

func TestWriteHTTPDispatchHandle_WithMissingHome_DoesNotCreateHome(t *testing.T) {
	// Arrange
	store := StateStore{Root: filepath.Join(t.TempDir(), "missing")}

	// Act
	err := store.writeHTTPDispatchHandle(testHTTPDispatchID, []byte(testHTTPDispatchHandle))

	// Assert
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.NoDirExists(t, store.Root)
}

func TestWriteHTTPDispatchHandle_WithFileAtNamespace_RejectsNamespace(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	namespace := filepath.Join(store.Root, httpDispatchReceiptDir)
	require.NoError(t, os.WriteFile(namespace, nil, 0o600))

	// Act
	err := store.writeHTTPDispatchHandle(testHTTPDispatchID, []byte(testHTTPDispatchHandle))

	// Assert
	require.ErrorContains(t, err, "namespace is not a directory")
	assert.FileExists(t, namespace)
}

func TestWriteHTTPDispatchHandle_WithConflictingEnvelope_KeepsOriginal(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	require.NoError(t, store.writeHTTPDispatchHandle(testHTTPDispatchID, []byte(testHTTPDispatchHandle)))
	path := filepath.Join(store.Root, httpDispatchReceiptDir, testHTTPDispatchID+".handle.json")
	other := []byte(testHTTPDispatchHandle + "\n")

	// Act
	err := store.writeHTTPDispatchHandle(testHTTPDispatchID, other)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchReceiptConflict)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, httpDispatchDigest([]byte(testHTTPDispatchHandle)), httpDispatchDigest(data))
}

func TestReadHTTPDispatchHandle_WithStoredEnvelope_ReturnsOriginalBytes(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	data := []byte(testHTTPDispatchHandle + "\n")
	require.NoError(t, store.writeHTTPDispatchHandle(testHTTPDispatchID, data))

	// Act
	got, err := store.readHTTPDispatchHandle(testHTTPDispatchID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, httpDispatchDigest(data), httpDispatchDigest(got))
}

func TestReadHTTPDispatchHandle_WithOversizedFile_RejectsRecord(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	dir := filepath.Join(store.Root, httpDispatchReceiptDir)
	require.NoError(t, os.Mkdir(dir, 0o700))
	path := filepath.Join(dir, testHTTPDispatchID+".handle.json")
	require.NoError(t, os.WriteFile(path, make([]byte, httpDispatchSmallBody+1), 0o600))

	// Act
	got, err := store.readHTTPDispatchHandle(testHTTPDispatchID)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchLimit)
	assert.Nil(t, got)
}

func TestReadHTTPDispatchHandle_WithDirectoryAtRecordPath_RejectsRecord(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	path := filepath.Join(store.Root, httpDispatchReceiptDir, testHTTPDispatchID+".handle.json")
	require.NoError(t, os.MkdirAll(path, 0o700))

	// Act
	got, err := store.readHTTPDispatchHandle(testHTTPDispatchID)

	// Assert
	require.ErrorContains(t, err, "not a regular file")
	assert.Nil(t, got)
}

func TestReadHTTPDispatchStoredReceipt_WithAdmittedRecord_ReadsExistingNamespace(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	receipt := storedHTTPDispatchFixture("wire receipt")
	_, err := store.writeHTTPDispatchReceipt(receipt)
	require.NoError(t, err)

	// Act
	data, err := store.readHTTPDispatchStoredReceipt(receipt.DispatchID)

	// Assert
	require.NoError(t, err)
	var record struct {
		Schema     string `json:"schema"`
		WireSHA256 string `json:"wire_sha256"`
	}
	require.NoError(t, json.Unmarshal(data, &record))
	assert.Equal(t, httpDispatchReceiptSchema, record.Schema)
	assert.Equal(t, receipt.WireSHA256, record.WireSHA256)
}

func TestReadHTTPDispatchStoredReceipt_WithInvalidID_RejectsLookup(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}

	// Act
	got, err := store.readHTTPDispatchStoredReceipt("../outside")

	// Assert
	require.ErrorContains(t, err, "invalid HTTP dispatch ID")
	assert.Nil(t, got)
}

func loopbackStatePendingFixture(checkout string) httpLoopbackPending {
	request := []byte("{\"fixture\":true}\n")
	return httpLoopbackPending{
		DispatchID:    testHTTPDispatchID,
		Request:       request,
		RequestSHA256: httpDispatchDigest(request),
		Endpoint:      "https://127.0.0.1:8443",
		TrustSHA256:   httpDispatchDigest([]byte("mock CA")),
		Provider:      "fixture-provider",
		Checkout:      checkout,
		ExpiresAt:     time.Date(2026, time.November, 10, 17, 0, 0, 0, time.UTC),
	}
}
