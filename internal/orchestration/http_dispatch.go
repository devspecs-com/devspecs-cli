package orchestration

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	httpDispatchSmallBody   = 8 << 10
	httpDispatchReceiptBody = 64 << 10
	httpDispatchRequestBody = 2 << 20
	httpDispatchCallTimeout = 10 * time.Second
)

var (
	errHTTPDispatchRequest     = errors.New("invalid HTTP dispatch v2 request")
	errHTTPDispatchResponse    = errors.New("invalid HTTP dispatch v2 response")
	errHTTPDispatchCorrelation = errors.New("HTTP dispatch v2 correlation mismatch")
	errHTTPDispatchLimit       = errors.New("HTTP dispatch v2 body exceeds limit")
	errHTTPDispatchNetwork     = errors.New("HTTP dispatch v2 exchange interrupted")
	errHTTPDispatchPreflight   = errors.New("HTTP dispatch v2 capabilities do not meet requirements")
)

// The normal Driver and tagged pilots share transport validation only.
type httpDispatchTransport struct {
	base        string
	bearer      string
	client      *http.Client
	callTimeout time.Duration
	authorize   func(context.Context) error
}

func newHTTPDispatchTransport(endpoint, token string, roots *x509.CertPool) (*httpDispatchTransport, error) {
	u, err := parseHTTPDispatchEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || !ip.IsLoopback() || ip.Zone() != "" {
		return nil, errors.New("HTTP dispatch mock endpoint must use a loopback IP")
	}
	return newHTTPDispatchClient(u, token, roots)
}

func parseHTTPDispatchEndpoint(endpoint string) (*url.URL, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || strings.ContainsAny(endpoint, "?#") ||
		u.EscapedPath() != u.Path || strings.Contains(u.Path, "//") ||
		(u.Path != "" && u.Path != "/" && path.Clean(u.Path) != strings.TrimSuffix(u.Path, "/")) {
		return nil, errors.New("invalid HTTP dispatch endpoint")
	}
	return u, nil
}

// Callers must validate and authorize the endpoint before supplying credentials.
func newHTTPDispatchClient(u *url.URL, token string, roots *x509.CertPool) (*httpDispatchTransport, error) {
	if token == "" || strings.ContainsFunc(token, func(r rune) bool { return r <= ' ' || r >= 0x7f }) {
		return nil, errors.New("invalid HTTP dispatch mock bearer")
	}

	return &httpDispatchTransport{
		base:        strings.TrimSuffix(u.String(), "/"),
		bearer:      "Bearer " + token,
		callTimeout: httpDispatchCallTimeout,
		client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			Transport: &http.Transport{
				TLSClientConfig:        &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
				DisableCompression:     true,
				MaxResponseHeaderBytes: httpDispatchSmallBody,
			},
		},
	}, nil
}

func (t *httpDispatchTransport) capabilities(ctx context.Context) (*httpDispatchCapabilities, error) {
	_, raw, err := t.exchange(ctx, http.MethodGet, "/capabilities", nil, "", "", httpDispatchSmallBody, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var caps httpDispatchCapabilities
	if !decodeHTTPDispatch(raw, "Capabilities", &caps) {
		return nil, errHTTPDispatchResponse
	}
	caps.transport = t
	return &caps, nil
}

// The caller owns the frozen bytes, replay decision and retention window.
// If an earlier handle is known, require the replay to return that same handle.
func (t *httpDispatchTransport) submit(ctx context.Context, caps *httpDispatchCapabilities, frozen []byte, previous *httpDispatchHandle) (*httpDispatchHandle, error) {
	if len(frozen) > httpDispatchRequestBody {
		return nil, errHTTPDispatchLimit
	}
	var req httpDispatchRequest
	if !decodeHTTPDispatch(frozen, "Request", &req) {
		return nil, errHTTPDispatchRequest
	}
	inlineBytes, artifactBytes, valid := req.validateInput()
	if !valid {
		return nil, errHTTPDispatchRequest
	}
	if caps == nil || caps.transport != t || !caps.allows(req, len(frozen), inlineBytes, artifactBytes) {
		return nil, errHTTPDispatchPreflight
	}
	digest := httpDispatchDigest(frozen)
	if previous != nil && (previous.transport != t || previous.DispatchID != req.DispatchID ||
		previous.RequestSHA256 != digest || previous.Provider.Name != caps.Provider) {
		return nil, errHTTPDispatchCorrelation
	}
	code, raw, err := t.exchange(ctx, http.MethodPost, "/dispatches", frozen, req.DispatchID, req.DispatchID,
		httpDispatchSmallBody, http.StatusCreated, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var handle httpDispatchHandle
	if (previous != nil && code != http.StatusOK) || !decodeHTTPDispatch(raw, "Handle", &handle) {
		return nil, errHTTPDispatchResponse
	}
	if handle.DispatchID != req.DispatchID || handle.RequestSHA256 != digest || handle.Provider.Name != caps.Provider ||
		(previous != nil && (handle.Provider != previous.Provider || !handle.AcceptedAt.Equal(previous.AcceptedAt))) {
		return nil, errHTTPDispatchCorrelation
	}
	handle.transport = t
	handle.repository = req.Repository
	handle.caller = req.Caller
	handle.maxAttempts = req.Requirements.MaxAttempts
	return &handle, nil
}

func (t *httpDispatchTransport) status(ctx context.Context, handle *httpDispatchHandle) (*httpDispatchStatus, error) {
	return t.observeStatus(ctx, handle, http.MethodGet, "", http.StatusOK)
}

func (t *httpDispatchTransport) cancel(ctx context.Context, handle *httpDispatchHandle) (*httpDispatchStatus, error) {
	return t.observeStatus(ctx, handle, http.MethodPost, "/cancel", http.StatusOK, http.StatusAccepted)
}

func (t *httpDispatchTransport) observeStatus(ctx context.Context, handle *httpDispatchHandle, method, suffix string, success ...int) (*httpDispatchStatus, error) {
	dispatchPath, err := t.dispatchPath(handle, suffix)
	if err != nil {
		return nil, err
	}
	code, raw, err := t.exchange(ctx, method, dispatchPath, nil, "", handle.DispatchID, httpDispatchSmallBody, success...)
	if err != nil {
		return nil, err
	}
	var status httpDispatchStatus
	if !decodeHTTPDispatch(raw, "Status", &status) {
		return nil, errHTTPDispatchResponse
	}
	if status.httpDispatchCorrelation != handle.httpDispatchCorrelation || status.ObservedAt.Before(handle.AcceptedAt) {
		return nil, errHTTPDispatchCorrelation
	}
	if method == http.MethodPost &&
		((code == http.StatusAccepted && (terminalHTTPDispatchState(status.State) || !status.CancelRequested)) ||
			(code == http.StatusOK && !terminalHTTPDispatchState(status.State))) {
		return nil, errHTTPDispatchResponse
	}
	return &status, nil
}

// Return the unprojected wire receipt. This is an observation, not admission.
func (t *httpDispatchTransport) receipt(ctx context.Context, handle *httpDispatchHandle) (json.RawMessage, error) {
	dispatchPath, err := t.dispatchPath(handle, "/receipt")
	if err != nil {
		return nil, err
	}
	_, raw, err := t.exchange(ctx, http.MethodGet, dispatchPath, nil, "", handle.DispatchID, httpDispatchReceiptBody, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var receipt httpDispatchReceipt
	if !decodeHTTPDispatch(raw, "Receipt", &receipt) {
		return nil, errHTTPDispatchResponse
	}
	if receipt.httpDispatchCorrelation != handle.httpDispatchCorrelation || receipt.Caller != handle.caller ||
		receipt.Repository != handle.repository || receipt.Execution.Attempts > handle.maxAttempts ||
		receipt.Execution.EndedAt.Before(handle.AcceptedAt) ||
		(receipt.Execution.StartedAt != nil &&
			(receipt.Execution.StartedAt.Before(handle.AcceptedAt) || receipt.Execution.StartedAt.After(receipt.Execution.EndedAt))) {
		return nil, errHTTPDispatchCorrelation
	}
	if !receipt.Evidence.Manifest.validInline() || !receipt.Report.Artifact.validInline() || !receipt.Verification.TestOutput.validInline() {
		return nil, errHTTPDispatchResponse
	}
	return raw, nil
}

func (t *httpDispatchTransport) dispatchPath(handle *httpDispatchHandle, suffix string) (string, error) {
	if handle == nil || handle.transport != t || !httpDispatchID.MatchString(handle.DispatchID) {
		return "", errHTTPDispatchCorrelation
	}
	return "/dispatches/" + handle.DispatchID + suffix, nil
}

func (t *httpDispatchTransport) exchange(ctx context.Context, method, route string, body []byte, key, dispatchID string, limit int, success ...int) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, t.callTimeout)
	defer cancel()
	if t.authorize != nil {
		if err := t.authorize(ctx); err != nil {
			return 0, nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, t.base+route, bytes.NewReader(body))
	if err != nil {
		return 0, nil, errHTTPDispatchRequest
	}
	req.Header.Set("Authorization", t.bearer)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	if key != "" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		// Do not let net/http replay an idempotency-keyed POST on a stale connection.
		req.GetBody = nil
	}
	res, err := t.client.Do(req)
	if err != nil {
		return 0, nil, httpDispatchExchangeError(err)
	}
	defer res.Body.Close()

	code := res.StatusCode
	ok := slices.Contains(success, code)
	if !ok {
		limit = httpDispatchSmallBody
	}
	if res.ContentLength > int64(limit) {
		return 0, nil, errHTTPDispatchLimit
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, int64(limit)+1))
	if ctx.Err() != nil {
		return 0, nil, ctx.Err()
	}
	if err != nil {
		return 0, nil, httpDispatchExchangeError(err)
	}
	if len(raw) > limit {
		return 0, nil, errHTTPDispatchLimit
	}
	if !ok {
		return 0, nil, httpDispatchStatusError(res, raw, dispatchID)
	}
	if !httpDispatchJSONResponse(res) {
		return 0, nil, errHTTPDispatchResponse
	}
	return code, raw, nil
}

func httpDispatchExchangeError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	default:
		return errHTTPDispatchNetwork
	}
}

func httpDispatchJSONResponse(res *http.Response) bool {
	types := res.Header.Values("Content-Type")
	encodings := res.Header.Values("Content-Encoding")
	if len(types) != 1 || len(encodings) > 1 || (len(encodings) == 1 && !strings.EqualFold(encodings[0], "identity")) {
		return false
	}
	media, params, err := mime.ParseMediaType(types[0])
	return err == nil && media == "application/json" && (params["charset"] == "" || strings.EqualFold(params["charset"], "utf-8"))
}

type httpDispatchError struct {
	Status     int
	Code       string
	Retryable  bool
	RetryAfter time.Duration
}

func (e *httpDispatchError) Error() string {
	return fmt.Sprintf("HTTP dispatch v2 returned status %d", e.Status)
}

func httpDispatchStatusError(res *http.Response, raw []byte, dispatchID string) error {
	out := &httpDispatchError{Status: res.StatusCode}
	var wire struct {
		Code       string `json:"code"`
		DispatchID string `json:"dispatch_id"`
		Retryable  bool   `json:"retryable"`
	}
	if !httpDispatchJSONResponse(res) || !decodeHTTPDispatch(raw, "Error", &wire) ||
		(wire.DispatchID != "" && wire.DispatchID != dispatchID) ||
		!slices.Contains(httpDispatchErrorCodes[out.Status], wire.Code) {
		return out
	}
	out.Code = wire.Code
	switch out.Status {
	case http.StatusTooManyRequests:
		values := res.Header.Values("Retry-After")
		if len(values) == 1 {
			if seconds, err := strconv.ParseUint(values[0], 10, 16); err == nil && seconds >= 1 && seconds <= 3600 {
				out.RetryAfter = time.Duration(seconds) * time.Second
				out.Retryable = wire.Retryable
			}
		}
	case http.StatusServiceUnavailable:
		out.Retryable = wire.Retryable
	}
	return out
}

var httpDispatchErrorCodes = map[int][]string{
	400: {"invalid_request"},
	401: {"unauthenticated"},
	403: {"forbidden"},
	404: {"not_found"},
	409: {"idempotency_conflict", "receipt_not_ready"},
	413: {"payload_too_large"},
	415: {"unsupported_media_type"},
	422: {"unsupported_version", "unsupported_capability"},
	429: {"rate_limited"},
	500: {"internal"},
	503: {"unavailable"},
}
