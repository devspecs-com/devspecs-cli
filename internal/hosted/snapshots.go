package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/hosted/snapshot"
)

const SnapshotPublishTimeout = 50 * time.Second
const SnapshotPolicyTimeout = 5 * time.Second

type PublishedSnapshot struct {
	SchemaVersion int    `json:"schema_version"`
	URL           string `json:"url"`
}

type SnapshotError struct {
	Status int
	Code   string
}

func (e *SnapshotError) Error() string { return "hosted snapshot request was refused" }
func (e *SnapshotError) Unwrap() error {
	if e.Status == http.StatusForbidden {
		return ErrDenied
	}
	return nil
}

func snapshotRoute(r config.RemoteConfig) string {
	return "/v1/tenants/" + r.TenantID + "/repos/" + r.RepoID + "/snapshots"
}

func (c *Client) validSnapshotBinding(r config.RemoteConfig, token string) bool {
	return r.URL == c.origin && config.ValidateRemotes([]config.RemoteConfig{r}) == nil && (ValidToken(token, "sph_") || ValidToken(token, "shs_"))
}

func (c *Client) SnapshotPolicy(ctx context.Context, r config.RemoteConfig, token string) (snapshot.Policy, error) {
	if !c.validSnapshotBinding(r, token) {
		return snapshot.Policy{}, ErrCredential
	}
	raw, err := c.snapshotExchange(ctx, http.MethodGet, snapshotRoute(r)+"/policy", token, nil, SnapshotPolicyTimeout, snapshot.MaxPolicyBytes)
	if err != nil {
		return snapshot.Policy{}, err
	}
	policy, err := snapshot.DecodePolicy(raw)
	if err != nil {
		return snapshot.Policy{}, ErrProtocol
	}
	return policy, nil
}

func (c *Client) PublishSnapshot(ctx context.Context, r config.RemoteConfig, token string, manifest snapshot.Snapshot) (PublishedSnapshot, error) {
	if !c.validSnapshotBinding(r, token) {
		return PublishedSnapshot{}, ErrCredential
	}
	raw, err := snapshot.Canonical(manifest)
	if err != nil {
		return PublishedSnapshot{}, err
	}
	data, err := c.snapshotExchange(ctx, http.MethodPost, snapshotRoute(r), token, raw, SnapshotPublishTimeout, 16<<10)
	if err != nil {
		return PublishedSnapshot{}, err
	}
	var result PublishedSnapshot
	var object map[string]json.RawMessage
	if snapshot.CheckJSON(data) != nil || json.Unmarshal(data, &object) != nil || len(object) != 2 || object["schema_version"] == nil || object["url"] == nil ||
		decode(data, &result) != nil || result.SchemaVersion != 1 {
		return PublishedSnapshot{}, ErrProtocol
	}
	prefix := c.origin + snapshotRoute(r) + "/"
	id, found := strings.CutPrefix(result.URL, prefix)
	if !found || !snapshot.ValidUUID(id) {
		return PublishedSnapshot{}, ErrProtocol
	}
	return result, nil
}

func (c *Client) snapshotExchange(ctx context.Context, method, route, token string, body []byte, budget time.Duration, limit int) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ErrCancelled
	}
	if len(body) > snapshot.MaxBodyBytes {
		return nil, snapshot.ErrTooLarge
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	transport, ok := c.http.Transport.(*http.Transport)
	if !ok {
		return nil, ErrProtocol
	}
	owned := transport.Clone()
	owned.ResponseHeaderTimeout = budget
	if method == http.MethodPost {
		owned.ResponseHeaderTimeout = 45 * time.Second
	}
	client := &http.Client{Transport: owned, Timeout: budget, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	var input io.Reader
	if body != nil {
		input = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+route, input)
	if err != nil {
		return nil, ErrNetwork
	}
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "devspecs-cli")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrCancelled
		}
		return nil, ErrNetwork
	}
	defer response.Body.Close()
	if response.ContentLength > int64(limit) {
		return nil, ErrProtocol
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if ctx.Err() != nil {
		return nil, ErrCancelled
	}
	if err != nil || len(data) > limit {
		return nil, ErrProtocol
	}
	media, parameters, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(response.Header.Values("Content-Type")) != 1 ||
		(len(parameters) != 0 && (len(parameters) != 1 || !strings.EqualFold(parameters["charset"], "utf-8"))) ||
		len(response.Header.Values("Content-Encoding")) > 1 || (response.Header.Get("Content-Encoding") != "" && !strings.EqualFold(response.Header.Get("Content-Encoding"), "identity")) {
		return nil, ErrProtocol
	}
	if response.StatusCode != http.StatusOK {
		return nil, decodeSnapshotError(response.StatusCode, data)
	}
	return data, nil
}

func decodeSnapshotError(status int, data []byte) error {
	var result struct {
		SchemaVersion int `json:"schema_version"`
		Error         struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if snapshot.CheckJSON(data) != nil || decode(data, &result) != nil || result.SchemaVersion != 1 {
		return ErrProtocol
	}
	valid := false
	switch status {
	case 400:
		valid = result.Error.Code == "invalid_request"
	case 403:
		valid = result.Error.Code == "access_denied"
	case 405:
		valid = result.Error.Code == "method_not_allowed"
	case 409:
		valid = result.Error.Code == "record_conflict"
	case 413:
		valid = result.Error.Code == "payload_too_large"
	case 422:
		valid = result.Error.Code == "unsupported_schema" || result.Error.Code == "unverified_provenance"
	case 503:
		valid = result.Error.Code == "unavailable"
	}
	if !valid {
		return ErrProtocol
	}
	return &SnapshotError{Status: status, Code: result.Error.Code}
}

var _ error = (*SnapshotError)(nil)
