//go:build devspecs_http_loopback

package orchestration

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Allow for base64 expansion of the frozen request and the binding metadata.
const httpLoopbackPendingBody = (httpDispatchRequestBody+2)/3*4 + httpDispatchSmallBody

type httpLoopbackPending struct {
	DispatchID    string    `json:"dispatch_id"`
	Request       []byte    `json:"request"`
	RequestSHA256 string    `json:"request_sha256"`
	Endpoint      string    `json:"endpoint"`
	TrustSHA256   string    `json:"trust_sha256"`
	Provider      string    `json:"provider"`
	Checkout      string    `json:"checkout"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (s StateStore) writeHTTPDispatchPending(p httpLoopbackPending) (string, error) {
	if err := p.validate(p.DispatchID); err != nil {
		return "", err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("encode HTTP dispatch pending record: %w", err)
	}
	if len(data) > httpLoopbackPendingBody {
		return "", errHTTPDispatchLimit
	}
	name := p.DispatchID + ".pending.json"
	if err := s.publishHTTPDispatchLoopbackRecord(name, data); err != nil {
		return "", err
	}
	return filepath.Join(s.Root, httpDispatchReceiptDir, name), nil
}

func (s StateStore) readHTTPDispatchPending(id string) (httpLoopbackPending, error) {
	data, err := s.readHTTPDispatchLoopbackRecord(id, ".pending.json", httpLoopbackPendingBody)
	if err != nil {
		return httpLoopbackPending{}, err
	}
	var p httpLoopbackPending
	if err := json.Unmarshal(data, &p); err != nil {
		return httpLoopbackPending{}, errors.New("invalid HTTP dispatch pending record")
	}
	if err := p.validate(id); err != nil {
		return httpLoopbackPending{}, err
	}
	return p, nil
}

func (p httpLoopbackPending) validate(id string) error {
	if len(p.Request) > httpDispatchRequestBody {
		return errHTTPDispatchLimit
	}
	if !httpDispatchID.MatchString(id) || p.DispatchID != id || len(p.Request) == 0 ||
		!filepath.IsAbs(p.Checkout) || strings.TrimSpace(p.Endpoint) == "" ||
		strings.TrimSpace(p.TrustSHA256) == "" || strings.TrimSpace(p.Provider) == "" || p.ExpiresAt.IsZero() {
		return errors.New("invalid HTTP dispatch pending record")
	}
	if httpDispatchDigest(p.Request) != p.RequestSHA256 {
		return errors.New("HTTP dispatch pending request digest mismatch")
	}
	return nil
}

func (s StateStore) writeHTTPDispatchHandle(id string, data []byte) error {
	if !httpDispatchID.MatchString(id) {
		return errors.New("invalid HTTP dispatch ID")
	}
	if len(data) > httpDispatchSmallBody {
		return errHTTPDispatchLimit
	}
	return s.publishHTTPDispatchLoopbackRecord(id+".handle.json", data)
}

func (s StateStore) readHTTPDispatchHandle(id string) ([]byte, error) {
	return s.readHTTPDispatchLoopbackRecord(id, ".handle.json", httpDispatchSmallBody)
}

func (s StateStore) readHTTPDispatchStoredReceipt(id string) (json.RawMessage, error) {
	return s.readHTTPDispatchLoopbackRecord(id, ".receipt.json", httpDispatchReceiptBody)
}

func (s StateStore) publishHTTPDispatchLoopbackRecord(name string, data []byte) error {
	dir, err := s.openHTTPDispatchLoopbackDir(true)
	if err != nil {
		return err
	}
	defer dir.Close()
	return publishHTTPDispatchReceipt(dir, name, data)
}

func (s StateStore) readHTTPDispatchLoopbackRecord(id, suffix string, limit int) ([]byte, error) {
	if !httpDispatchID.MatchString(id) {
		return nil, errors.New("invalid HTTP dispatch ID")
	}
	dir, err := s.openHTTPDispatchLoopbackDir(false)
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	name := id + suffix
	info, err := dir.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("HTTP dispatch record is not a regular file")
	}
	if info.Size() > int64(limit) {
		return nil, errHTTPDispatchLimit
	}
	file, err := dir.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, errors.New("HTTP dispatch record changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, errHTTPDispatchLimit
	}
	return data, nil
}

func (s StateStore) openHTTPDispatchLoopbackDir(create bool) (*os.Root, error) {
	// The caller supplies an owned home; storage never creates it or follows a
	// link in place of the home or the HTTP namespace.
	if s.Root == "" {
		return nil, errors.New("HTTP dispatch state home is required")
	}
	rootPath := filepath.Clean(s.Root)
	info, err := os.Lstat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("inspect HTTP dispatch state home: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("HTTP dispatch state home is not a directory")
	}
	home, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, fmt.Errorf("open HTTP dispatch state home: %w", err)
	}
	defer home.Close()

	if create {
		if err := home.Mkdir(httpDispatchReceiptDir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("create HTTP dispatch namespace: %w", err)
		}
	}
	info, err = home.Lstat(httpDispatchReceiptDir)
	if err != nil {
		return nil, fmt.Errorf("inspect HTTP dispatch namespace: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("HTTP dispatch namespace is not a directory")
	}
	return home.OpenRoot(httpDispatchReceiptDir)
}
