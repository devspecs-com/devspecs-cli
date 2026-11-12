//go:build devspecs_http_loopback

package orchestration

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const httpLoopbackRetention = time.Hour

var (
	errLoopbackBinding = errors.New("loopback configuration does not match the frozen dispatch")
	errLoopbackExpired = errors.New("loopback recovery window expired")
	errLoopbackState   = errors.New("invalid local loopback dispatch state")
)

// LoopbackResult is the machine contract for the experimental build only.
type LoopbackResult struct {
	Schema        string          `json:"schema"`
	Version       int             `json:"schema_version"`
	Operation     string          `json:"operation"`
	ID            string          `json:"dispatch_id,omitempty"`
	RequestSHA256 string          `json:"request_sha256,omitempty"`
	State         string          `json:"state,omitempty"`
	Receipt       json.RawMessage `json:"receipt,omitempty"`
	Error         string          `json:"error,omitempty"`
}

func NewLoopbackResult(operation string) LoopbackResult {
	return LoopbackResult{Schema: "devspecs.dispatch.loopback_result", Version: 1, Operation: operation}
}

// LoopbackDispatch binds a test run to a caller-selected TLS anchor and home.
// It cannot be selected by repository configuration.
type LoopbackDispatch struct {
	store       StateStore
	transport   *httpDispatchTransport
	trustSHA256 string
	caller      httpDispatchCaller
	sourceURL   string
	readOnly    bool
}

func NewLoopbackDispatch(home, endpoint, trustPath, credentialSlot string) (*LoopbackDispatch, error) {
	if !filepath.IsAbs(home) || !filepath.IsAbs(trustPath) {
		return nil, errLoopbackBinding
	}
	info, err := os.Lstat(trustPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > httpDispatchSmallBody {
		return nil, errLoopbackBinding
	}
	trust, err := os.Open(trustPath)
	if err != nil {
		return nil, errLoopbackBinding
	}
	defer trust.Close()
	pem, err := io.ReadAll(io.LimitReader(trust, httpDispatchSmallBody+1))
	roots := x509.NewCertPool()
	if err != nil || len(pem) > httpDispatchSmallBody || !roots.AppendCertsFromPEM(pem) {
		return nil, errLoopbackBinding
	}
	caller, bearer := loopbackCredential(credentialSlot)
	transport, err := newHTTPDispatchTransport(endpoint, bearer, roots)
	if err != nil {
		return nil, errLoopbackBinding
	}
	return &LoopbackDispatch{
		store: StateStore{Root: home}, transport: transport, trustSHA256: httpDispatchDigest(pem), caller: caller,
	}, nil
}

// Slots select only built-in synthetic identities. No caller-supplied bearer
// can enter this experiment.
func loopbackCredential(slot string) (httpDispatchCaller, string) {
	switch slot {
	case "", "initial":
		return httpDispatchCaller{"principal_fixture", "credential_fixture_v1"}, "synthetic-mock-bearer"
	case "rotated":
		return httpDispatchCaller{"principal_fixture", "credential_fixture_v2"}, "synthetic-mock-bearer-rotated"
	case "other":
		return httpDispatchCaller{"principal_other", "credential_other"}, "synthetic-mock-bearer-other"
	default:
		return httpDispatchCaller{}, ""
	}
}

func (s *LoopbackDispatch) Start(ctx context.Context, target, checkout string, detach bool, in io.Reader) LoopbackResult {
	result := NewLoopbackResult("dispatch")
	frozen, err := io.ReadAll(io.LimitReader(in, httpDispatchRequestBody+1))
	if err != nil || len(frozen) > httpDispatchRequestBody {
		result.Error = "invalid_request"
		return result
	}
	var wire httpDispatchPortableRequest
	var request httpDispatchRequest
	if !decodeHTTPDispatch(frozen, "Request", &request) || json.Unmarshal(frozen, &wire) != nil || wire.Target.Owner != target ||
		!validHTTPDispatchSource(httpDispatchConfirmedSource{URL: wire.Repository.GitURL, Commit: wire.Repository.SourceCommit}) ||
		wire.Repository.LogicalID != "git:"+wire.Repository.GitURL {
		result.Error = "invalid_request"
		return result
	}
	if request.Caller.PrincipalID != s.caller.PrincipalID {
		return loopbackFailure(result, errLoopbackBinding)
	}
	source, _ := url.Parse(wire.Repository.GitURL)
	inline, external, valid := request.validateInput()
	sourceAllowed := strings.HasSuffix(strings.ToLower(source.Hostname()), ".invalid")
	if s.sourceURL != "" {
		sourceAllowed = wire.Repository.GitURL == s.sourceURL
	}
	if !sourceAllowed || !valid || external != 0 || inline > httpDispatchMaxInline {
		result.Error = "invalid_request"
		return result
	}
	for _, artifact := range wire.Input.ContextArtifacts {
		if artifact.Kind != "inline" {
			result.Error = "invalid_request"
			return result
		}
	}
	checkout, err = filepath.Abs(checkout)
	if err != nil {
		result.Error = "invalid_request"
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(request.Requirements.TimeoutSeconds)*time.Second)
	defer cancel()
	pending := httpLoopbackPending{
		DispatchID: wire.DispatchID, Request: frozen, RequestSHA256: httpDispatchDigest(frozen),
		Endpoint: s.transport.base, TrustSHA256: s.trustSHA256, Checkout: checkout,
		ExpiresAt: time.Now().UTC().Add(httpLoopbackRetention),
	}
	result.ID, result.RequestSHA256 = pending.DispatchID, pending.RequestSHA256
	var caps *httpDispatchCapabilities
	prior, err := s.store.readHTTPDispatchPending(result.ID)
	switch {
	case err == nil:
		if !s.matchesPendingPrincipal(prior) {
			return loopbackFailure(result, errLoopbackBinding)
		}
		if prior.RequestSHA256 != pending.RequestSHA256 || prior.Checkout != checkout {
			result.Error = "request_conflict"
			return result
		}
	case errors.Is(err, os.ErrNotExist):
		if request.Caller != s.caller {
			return loopbackFailure(result, errLoopbackBinding)
		}
		if err := checkHTTPDispatchCheckout(ctx, ExecRunner{}, checkout, request.Repository.SourceCommit); err != nil {
			return loopbackFailure(result, err)
		}
		caps, err = s.capabilities(ctx)
		if err != nil {
			return loopbackFailure(result, err)
		}
		pending.Provider = caps.Provider
		if _, err := s.store.writeHTTPDispatchPending(pending); err != nil {
			result.Error = "state_unavailable"
			return result
		}
	default:
		result.Error = "state_unavailable"
		return result
	}
	return s.run(ctx, result, detach, caps)
}

func (s *LoopbackDispatch) matchesPendingPrincipal(p httpLoopbackPending) bool {
	var request httpDispatchRequest
	return decodeHTTPDispatch(p.Request, "Request", &request) && request.Caller.PrincipalID == s.caller.PrincipalID
}

func (s *LoopbackDispatch) Observe(ctx context.Context, operation, id string) LoopbackResult {
	result := NewLoopbackResult(operation)
	if !httpDispatchID.MatchString(id) {
		result.Error = "invalid_request"
		return result
	}
	result.ID = id
	return s.run(ctx, result, false, nil)
}

func (s *LoopbackDispatch) run(ctx context.Context, result LoopbackResult, detach bool, caps *httpDispatchCapabilities) LoopbackResult {
	pending, err := s.store.readHTTPDispatchPending(result.ID)
	if err != nil {
		return loopbackFailure(result, err)
	}
	var request httpDispatchRequest
	if !decodeHTTPDispatch(pending.Request, "Request", &request) || request.DispatchID != pending.DispatchID {
		return loopbackFailure(result, errLoopbackState)
	}
	if pending.Endpoint != s.transport.base || pending.TrustSHA256 != s.trustSHA256 ||
		request.Caller.PrincipalID != s.caller.PrincipalID {
		return loopbackFailure(result, errLoopbackBinding)
	}
	if s.sourceURL != "" {
		var wire httpDispatchPortableRequest
		if json.Unmarshal(pending.Request, &wire) != nil || wire.Repository.GitURL != s.sourceURL {
			return loopbackFailure(result, errLoopbackBinding)
		}
	}
	result.RequestSHA256 = pending.RequestSHA256
	receipt, state, err := s.storedReceipt(pending, request.Caller)
	if err == nil {
		result.Receipt, result.State = receipt, state
		return result
	}
	if !errors.Is(err, os.ErrNotExist) {
		return loopbackFailure(result, err)
	}
	if result.Operation == "receipt" {
		result.Error = "receipt_unavailable"
		return result
	}
	if !time.Now().Before(pending.ExpiresAt) {
		return loopbackFailure(result, errLoopbackExpired)
	}
	deadline := time.Now().Add(time.Duration(request.Requirements.TimeoutSeconds) * time.Second)
	if pending.ExpiresAt.Before(deadline) {
		deadline = pending.ExpiresAt
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	handle, err := s.loadHandle(pending, request)
	if errors.Is(err, os.ErrNotExist) {
		result.State = "pending"
		if result.Operation == "status" {
			return result
		}
		handle, err = s.submit(ctx, pending, request, caps)
	}
	if err != nil {
		return loopbackFailure(result, err)
	}
	result.State = "accepted"
	if detach {
		return result
	}
	return s.monitor(ctx, pending, handle, result)
}

func (s *LoopbackDispatch) loadHandle(p httpLoopbackPending, request httpDispatchRequest) (*httpDispatchHandle, error) {
	raw, err := s.store.readHTTPDispatchHandle(p.DispatchID)
	if err != nil {
		return nil, err
	}
	var handle httpDispatchHandle
	if !decodeHTTPDispatch(raw, "Handle", &handle) || handle.DispatchID != p.DispatchID ||
		handle.RequestSHA256 != p.RequestSHA256 || handle.Provider.Name != p.Provider {
		return nil, errLoopbackState
	}
	handle.transport, handle.repository, handle.caller, handle.maxAttempts = s.transport, request.Repository, request.Caller, request.Requirements.MaxAttempts
	return &handle, nil
}

func (s *LoopbackDispatch) capabilities(ctx context.Context) (*httpDispatchCapabilities, error) {
	caps, err := s.transport.capabilities(ctx)
	if err != nil {
		return nil, err
	}
	if !caps.Ready || caps.IdempotencyRetentionSecs < int(httpLoopbackRetention/time.Second) {
		return nil, errHTTPDispatchPreflight
	}
	return caps, nil
}

func (s *LoopbackDispatch) submit(ctx context.Context, p httpLoopbackPending, request httpDispatchRequest, caps *httpDispatchCapabilities) (*httpDispatchHandle, error) {
	// A new caller must verify the pinned checkout and provider before replay.
	if caps == nil {
		if err := checkHTTPDispatchCheckout(ctx, ExecRunner{}, p.Checkout, request.Repository.SourceCommit); err != nil {
			return nil, err
		}
		var err error
		caps, err = s.capabilities(ctx)
		if err != nil {
			return nil, err
		}
	}
	if caps.Provider != p.Provider {
		return nil, errLoopbackBinding
	}
	handle, err := s.transport.submit(ctx, caps, p.Request, nil)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(struct {
		Schema  string `json:"schema"`
		Version int    `json:"schema_version"`
		*httpDispatchHandle
	}{DispatchHandleSchema, 2, handle})
	if err != nil {
		return nil, errLoopbackState
	}
	if err := s.store.writeHTTPDispatchHandle(p.DispatchID, data); err != nil {
		return nil, errLoopbackState
	}
	return handle, nil
}

func (s *LoopbackDispatch) monitor(ctx context.Context, p httpLoopbackPending, handle *httpDispatchHandle, result LoopbackResult) LoopbackResult {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		status, err := s.transport.status(ctx, handle)
		if err != nil {
			return loopbackFailure(result, err)
		}
		if result.Operation == "status" {
			result.State = status.State
			return result
		}
		if terminalHTTPDispatchState(status.State) {
			raw, err := s.transport.receipt(ctx, handle)
			if err != nil {
				return loopbackFailure(result, err)
			}
			receipt, err := admitHTTPDispatchReceipt(ctx, ExecRunner{}, p.Checkout, p.Request, handle, status, raw, nil)
			if err != nil {
				return loopbackFailure(result, err)
			}
			if s.readOnly && !readOnlyHTTPPilotReceipt(receipt) {
				return loopbackFailure(result, errHTTPDispatchCorrelation)
			}
			if _, err := s.store.writeHTTPDispatchReceipt(receipt); err != nil {
				return loopbackFailure(result, errLoopbackState)
			}
			result.Receipt, result.State, err = s.storedReceipt(p, handle.caller)
			if err != nil {
				return loopbackFailure(result, err)
			}
			return result
		}
		result.State = status.State
		select {
		case <-ctx.Done():
			return loopbackFailure(result, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (s *LoopbackDispatch) storedReceipt(p httpLoopbackPending, caller httpDispatchCaller) (json.RawMessage, string, error) {
	raw, err := s.store.readHTTPDispatchStoredReceipt(p.DispatchID)
	if err != nil {
		return nil, "", err
	}
	var record struct {
		Schema  string `json:"schema"`
		Version int    `json:"schema_version"`
		httpDispatchAdmittedReceipt
	}
	if json.Unmarshal(raw, &record) != nil || record.Schema != httpDispatchReceiptSchema || record.Version != httpDispatchReceiptVersion ||
		record.DispatchID != p.DispatchID || record.RequestSHA256 != p.RequestSHA256 || record.Caller != caller ||
		!terminalHTTPDispatchState(record.State) {
		return nil, "", errLoopbackState
	}
	if s.readOnly && !readOnlyHTTPPilotReceipt(&record.httpDispatchAdmittedReceipt) {
		return nil, "", errLoopbackState
	}
	return raw, record.State, nil
}

func readOnlyHTTPPilotReceipt(receipt *httpDispatchAdmittedReceipt) bool {
	if receipt.ResultCommit == nil {
		return receipt.State != "succeeded"
	}
	return *receipt.ResultCommit == receipt.SourceCommit
}

func loopbackFailure(result LoopbackResult, err error) LoopbackResult {
	var provider *httpDispatchError
	switch {
	case errors.Is(err, errLoopbackBinding):
		result.Error = "configuration_mismatch"
	case errors.Is(err, errLoopbackExpired):
		result.Error = "recovery_expired"
	case errors.Is(err, errHTTPDispatchRequest), errors.Is(err, errHTTPDispatchLimit):
		result.Error = "invalid_request"
	case errors.Is(err, errHTTPDispatchPreflight), errors.Is(err, errHTTPDispatchResponse):
		result.Error = "incompatible_provider"
	case errors.Is(err, errHTTPDispatchCorrelation), errors.Is(err, errHTTPDispatchResult),
		errors.Is(err, errHTTPDispatchPaths), errors.Is(err, errHTTPDispatchTask):
		result.Error = "evidence_mismatch"
	case errors.Is(err, errHTTPDispatchNetwork), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		result.Error = "exchange_interrupted"
	case errors.As(err, &provider):
		result.Error = "provider_rejected"
	case errors.Is(err, os.ErrNotExist):
		result.Error = "dispatch_not_found"
	default:
		result.Error = "state_unavailable"
	}
	return result
}
