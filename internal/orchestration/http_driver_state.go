package orchestration

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

const httpDriverStateDir = "dispatches-http-driver-v2"
const httpDriverRecoveryWindow = time.Hour
const httpDriverPendingLimit = (httpDispatchRequestBody+2)/3*4 + httpDispatchSmallBody

var errHTTPDriverState = errors.New("HTTP dispatch recovery state is unavailable or invalid")
var errHTTPDriverExpired = errors.New("HTTP dispatch recovery window expired; worker cancellation is not implied")

type httpDriverPending struct {
	Version   int             `json:"version"`
	ID        string          `json:"dispatch_id"`
	Frozen    []byte          `json:"frozen"`
	Digest    string          `json:"request_sha256"`
	V1Digest  string          `json:"local_request_sha256"`
	Trust     httpDriverTrust `json:"trust"`
	Checkout  string          `json:"checkout"`
	TaskRepo  string          `json:"task_repo"`
	Provider  string          `json:"provider"`
	ExpiresAt time.Time       `json:"expires_at"`
}

func (d *httpDriver) stateDir(create bool) (*os.Root, error) {
	info, err := os.Lstat(d.store.Root)
	if err != nil || !info.IsDir() {
		return nil, errHTTPDriverState
	}
	home, err := os.OpenRoot(d.store.Root)
	if err != nil {
		return nil, errHTTPDriverState
	}
	defer home.Close()
	if create {
		if err := home.Mkdir(httpDriverStateDir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, errHTTPDriverState
		}
	}
	info, err = home.Lstat(httpDriverStateDir)
	if err != nil || !info.IsDir() {
		return nil, errHTTPDriverState
	}
	return home.OpenRoot(httpDriverStateDir)
}

func (d *httpDriver) writeRecord(id, suffix string, value any) error {
	if !httpDispatchID.MatchString(id) {
		return errHTTPDriverState
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > httpDriverPendingLimit {
		return errHTTPDriverState
	}
	dir, err := d.stateDir(true)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := publishHTTPDispatchReceipt(dir, id+suffix, raw); err != nil {
		return errHTTPDriverState
	}
	return nil
}

func (d *httpDriver) readRecord(id, suffix string, limit int, value any) error {
	if !httpDispatchID.MatchString(id) {
		return errHTTPDriverState
	}
	dir, err := d.stateDir(false)
	if err != nil {
		return err
	}
	defer dir.Close()
	info, err := dir.Lstat(id + suffix)
	if errors.Is(err, os.ErrNotExist) {
		return os.ErrNotExist
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > int64(limit) {
		return errHTTPDriverState
	}
	file, err := dir.Open(id + suffix)
	if err != nil {
		return errHTTPDriverState
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return errHTTPDriverState
	}
	raw, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(raw) > limit || !strictHTTPDriverJSON(raw, value) {
		return errHTTPDriverState
	}
	return nil
}

func (d *httpDriver) pending(handle ProviderHandle) (httpDriverPending, error) {
	var p httpDriverPending
	if handle.Name != "http" || (handle.AdapterVersion != "http-v2-read-only-1" && handle.AdapterVersion != "http-v2-bundle-1") ||
		d.readRecord(handle.RunID, ".pending.json", httpDriverPendingLimit, &p) != nil ||
		p.Version != 1 || p.ID != handle.RunID || httpDispatchDigest(p.Frozen) != p.Digest ||
		!filepath.IsAbs(p.Checkout) || !filepath.IsAbs(p.TaskRepo) || p.ExpiresAt.IsZero() || p.Provider == "" {
		return p, errHTTPDriverState
	}
	if p.Trust.ReadOnly == p.Trust.AllowEdits || (handle.AdapterVersion == "http-v2-read-only-1" && !p.Trust.ReadOnly) {
		return p, errHTTPDriverState
	}
	var req httpDispatchPortableRequest
	if !decodeHTTPDispatch(p.Frozen, "Request", &req) || req.DispatchID != p.ID ||
		req.Caller.PrincipalID != p.Trust.PrincipalID || req.Repository.GitURL != p.Trust.SourceURL ||
		req.Repository.SourceCommit != p.Trust.SourceCommit || req.Requirements.MaxAttempts != 1 {
		return p, errHTTPDriverState
	}
	if req.Requirements.Report.Required != p.Trust.ReportRequired || req.Requirements.Verification.Profile != p.Trust.VerificationProfile ||
		req.Requirements.Verification.Strength != p.Trust.VerificationStrength || req.Requirements.Verification.Required != (p.Trust.VerificationProfile != "") {
		return p, errHTTPDriverState
	}
	local, err := readOrchestrationFile(d.store.RequestPath(p.ID))
	var request DispatchRequest
	if err != nil || len(local) > maxDispatchFileBytes || sha256Bytes(local) != p.V1Digest || json.Unmarshal(local, &request) != nil ||
		request.DispatchID != p.ID || request.Repository.SourceCommit != p.Trust.SourceCommit {
		return p, errHTTPDriverState
	}
	root, err := httpDriverResolvedPath(request.Repository.RequestedRoot)
	if err != nil {
		return p, errHTTPDriverState
	}
	checkoutInfo, err := os.Stat(p.Checkout)
	if err != nil {
		return p, errHTTPDriverState
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !os.SameFile(checkoutInfo, rootInfo) {
		return p, errHTTPDriverState
	}
	return p, nil
}
