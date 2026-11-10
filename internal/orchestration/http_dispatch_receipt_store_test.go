package orchestration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteHTTPDispatchReceipt_WithNativeReceipt_PublishesSeparateV2Record(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	receipt := storedHTTPDispatchFixture("wire bytes")
	native := store.ReceiptPath(receipt.DispatchID)
	require.NoError(t, os.MkdirAll(filepath.Dir(native), 0o700))
	require.NoError(t, os.WriteFile(native, []byte("native v1 record"), 0o600))

	// Act
	published, err := store.writeHTTPDispatchReceipt(receipt)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(store.Root, httpDispatchReceiptDir, testHTTPDispatchID+".receipt.json"), published)
	assert.Equal(t, receipt.WireSHA256, storedHTTPDispatchDigest(t, published))
	raw, err := os.ReadFile(native)
	require.NoError(t, err)
	assert.Equal(t, "native v1 record", string(raw))
	entries, err := os.ReadDir(filepath.Dir(published))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, filepath.Base(published), entries[0].Name())
}

func TestWriteHTTPDispatchReceipt_WithIdenticalReplay_LeavesOriginalIntact(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	receipt := storedHTTPDispatchFixture("same wire")
	path, err := store.writeHTTPDispatchReceipt(receipt)
	require.NoError(t, err)
	before, err := os.Stat(path)
	require.NoError(t, err)

	// Act
	published, err := store.writeHTTPDispatchReceipt(receipt)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, path, published)
	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after))
}

func TestWriteHTTPDispatchReceipt_WithConflictingReplay_RefusesOverwrite(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	original := storedHTTPDispatchFixture("original")
	path, err := store.writeHTTPDispatchReceipt(original)
	require.NoError(t, err)
	replay := storedHTTPDispatchFixture("different")

	// Act
	published, err := store.writeHTTPDispatchReceipt(replay)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchReceiptConflict)
	assert.Empty(t, published)
	assert.Equal(t, original.WireSHA256, storedHTTPDispatchDigest(t, path))
}

func TestWriteHTTPDispatchReceipt_WithConcurrentConflictingRecords_PublishesOne(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	first := storedHTTPDispatchFixture("first")
	second := storedHTTPDispatchFixture("second")
	type result struct {
		path   string
		digest string
		err    error
	}
	done := make(chan result, 2)
	start := make(chan struct{})
	publish := func(receipt *httpDispatchAdmittedReceipt) {
		<-start
		path, err := store.writeHTTPDispatchReceipt(receipt)
		done <- result{path: path, digest: receipt.WireSHA256, err: err}
	}

	// Act
	go publish(first)
	go publish(second)
	close(start)
	a, b := <-done, <-done

	// Assert
	if a.err != nil {
		a, b = b, a
	}
	require.NoError(t, a.err)
	require.ErrorIs(t, b.err, errHTTPDispatchReceiptConflict)
	assert.Equal(t, a.digest, storedHTTPDispatchDigest(t, a.path))
}

func TestWriteHTTPDispatchReceipt_WithInvalidDispatchID_DoesNotCreateNamespace(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	receipt := storedHTTPDispatchFixture("wire")
	receipt.DispatchID = "../other"

	// Act
	published, err := store.writeHTTPDispatchReceipt(receipt)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchResponse)
	assert.Empty(t, published)
	assert.NoDirExists(t, filepath.Join(store.Root, httpDispatchReceiptDir))
}

func TestWriteHTTPDispatchReceipt_WithOversizedProjection_DoesNotCreateNamespace(t *testing.T) {
	// Arrange
	store := StateStore{Root: t.TempDir()}
	receipt := storedHTTPDispatchFixture("wire")
	receipt.WireSHA256 = strings.Repeat("x", httpDispatchReceiptBody)

	// Act
	published, err := store.writeHTTPDispatchReceipt(receipt)

	// Assert
	require.ErrorIs(t, err, errHTTPDispatchLimit)
	assert.Empty(t, published)
	assert.NoDirExists(t, filepath.Join(store.Root, httpDispatchReceiptDir))
}

func storedHTTPDispatchFixture(wire string) *httpDispatchAdmittedReceipt {
	return &httpDispatchAdmittedReceipt{
		DispatchID: testHTTPDispatchID,
		WireSHA256: httpDispatchDigest([]byte(wire)),
	}
}

func storedHTTPDispatchDigest(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var record struct {
		Schema        string `json:"schema"`
		SchemaVersion int    `json:"schema_version"`
		WireSHA256    string `json:"wire_sha256"`
	}
	require.NoError(t, json.Unmarshal(raw, &record))
	assert.Equal(t, "devspecs.dispatch.http_receipt_evidence", record.Schema)
	assert.Equal(t, 1, record.SchemaVersion)
	return record.WireSHA256
}
