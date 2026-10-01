package orchestration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxDispatchFileBytes = 2 << 20

type StateStore struct {
	Root string
}

func (s StateStore) DispatchDir(id string) string {
	return filepath.Join(s.Root, "dispatches", id)
}

func (s StateStore) RequestPath(id string) string {
	return filepath.Join(s.DispatchDir(id), "request.json")
}

func (s StateStore) HandlePath(id string) string {
	return filepath.Join(s.DispatchDir(id), "handle.json")
}

func (s StateStore) ReceiptPath(id string) string {
	return filepath.Join(s.DispatchDir(id), "receipt.json")
}

func (s StateStore) WriteRequest(request DispatchRequest) (string, string, error) {
	path := s.RequestPath(request.DispatchID)
	data, err := marshalJSON(request)
	if err != nil {
		return "", "", err
	}
	if err := writeFileAtomic(path, data, 0o600); err != nil {
		return "", "", err
	}
	return path, sha256Bytes(data), nil
}

func (s StateStore) WriteHandle(handle DispatchHandle) (string, error) {
	path := s.HandlePath(handle.DispatchID)
	data, err := marshalJSON(handle)
	if err != nil {
		return "", err
	}
	return path, writeFileAtomic(path, data, 0o600)
}

func (s StateStore) WriteReceipt(receipt DispatchReceipt) (string, error) {
	path := s.ReceiptPath(receipt.DispatchID)
	data, err := marshalJSON(receipt)
	if err != nil {
		return "", err
	}
	if len(data) > 8192 {
		return "", fmt.Errorf("normalized orchestration receipt exceeds 8 KiB")
	}
	return path, writeFileAtomic(path, data, 0o600)
}

func (s StateStore) LoadHandle(identifier string) (DispatchHandle, DispatchRequest, error) {
	handlePath := strings.TrimSpace(identifier)
	if filepath.Ext(handlePath) == "" && !strings.ContainsAny(handlePath, `/\`) {
		handlePath = s.HandlePath(handlePath)
	}
	var handle DispatchHandle
	if err := readJSONFile(handlePath, &handle); err != nil {
		return DispatchHandle{}, DispatchRequest{}, fmt.Errorf("read dispatch handle: %w", err)
	}
	if handle.Schema != DispatchHandleSchema || handle.SchemaVersion != SchemaVersion || handle.DispatchID == "" {
		return DispatchHandle{}, DispatchRequest{}, fmt.Errorf("unsupported dispatch handle")
	}
	var request DispatchRequest
	requestData, err := os.ReadFile(handle.Request.Value)
	if err != nil {
		return DispatchHandle{}, DispatchRequest{}, fmt.Errorf("read dispatch request: %w", err)
	}
	if len(requestData) > maxDispatchFileBytes {
		return DispatchHandle{}, DispatchRequest{}, fmt.Errorf("dispatch request exceeds 2 MiB")
	}
	if sha256Bytes(requestData) != handle.RequestSHA256 {
		return DispatchHandle{}, DispatchRequest{}, fmt.Errorf("dispatch request digest does not match handle")
	}
	if err := json.Unmarshal(requestData, &request); err != nil {
		return DispatchHandle{}, DispatchRequest{}, fmt.Errorf("parse dispatch request: %w", err)
	}
	if request.Schema != DispatchRequestSchema || request.SchemaVersion != SchemaVersion || request.DispatchID != handle.DispatchID {
		return DispatchHandle{}, DispatchRequest{}, fmt.Errorf("unsupported or mismatched dispatch request")
	}
	return handle, request, nil
}

func marshalJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create orchestration state directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary orchestration state: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := bytes.NewReader(data).WriteTo(temp); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := replaceFile(tempPath, path); err != nil {
		return fmt.Errorf("publish orchestration state: %w", err)
	}
	return nil
}

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) > maxDispatchFileBytes {
		return fmt.Errorf("file exceeds 2 MiB")
	}
	return json.Unmarshal(data, target)
}

func sha256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
