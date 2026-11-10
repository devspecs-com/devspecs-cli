package orchestration

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	httpDispatchReceiptDir    = "dispatches-http-v2"
	httpDispatchReceiptSchema = "devspecs.dispatch.http_receipt_evidence"
)

var errHTTPDispatchReceiptConflict = errors.New("refusing to replace an existing HTTP dispatch receipt")

// Only an admitted projection belongs here, never an unfiltered provider body.
// This namespace is deliberately separate from the native v1 StateStore paths.
func (s StateStore) writeHTTPDispatchReceipt(receipt *httpDispatchAdmittedReceipt) (string, error) {
	if receipt == nil || !httpDispatchID.MatchString(receipt.DispatchID) {
		return "", errHTTPDispatchResponse
	}
	data, err := json.Marshal(struct {
		Schema        string `json:"schema"`
		SchemaVersion int    `json:"schema_version"`
		*httpDispatchAdmittedReceipt
	}{httpDispatchReceiptSchema, 1, receipt})
	if err != nil {
		return "", errHTTPDispatchResponse
	}
	if len(data) > httpDispatchReceiptBody {
		return "", errHTTPDispatchLimit
	}
	home, err := os.OpenRoot(s.Root)
	if err != nil {
		return "", fmt.Errorf("open HTTP dispatch state home: %w", err)
	}
	defer home.Close()

	if err := home.Mkdir(httpDispatchReceiptDir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("create HTTP receipt directory: %w", err)
	}
	info, err := home.Lstat(httpDispatchReceiptDir)
	if err != nil || !info.IsDir() {
		return "", errors.New("HTTP receipt namespace is not a directory")
	}
	dir, err := home.OpenRoot(httpDispatchReceiptDir)
	if err != nil {
		return "", fmt.Errorf("open HTTP receipt directory: %w", err)
	}
	defer dir.Close()

	name := receipt.DispatchID + ".receipt.json"
	if err := publishHTTPDispatchReceipt(dir, name, data); err != nil {
		return "", err
	}
	return filepath.Join(s.Root, httpDispatchReceiptDir, name), nil
}

func publishHTTPDispatchReceipt(dir *os.Root, name string, data []byte) error {
	staging := ".receipt-" + rand.Text() + ".tmp"
	file, err := dir.OpenFile(staging, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("stage HTTP receipt: %w", err)
	}
	defer dir.Remove(staging)
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("stage HTTP receipt: %w", err)
	}

	// Linking a complete file publishes atomically and never replaces a winner.
	err = dir.Link(staging, name)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("publish HTTP receipt: %w", err)
	}
	if !sameHTTPDispatchReceipt(dir, name, data) {
		return errHTTPDispatchReceiptConflict
	}
	return nil
}

func sameHTTPDispatchReceipt(dir *os.Root, name string, data []byte) bool {
	info, err := dir.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(len(data)) {
		return false
	}
	file, err := dir.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	existing, err := io.ReadAll(io.LimitReader(file, int64(len(data))+1))
	return err == nil && bytes.Equal(existing, data)
}
