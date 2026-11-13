package hosted

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
)

var ErrNetwork = errors.New("hosted exchange failed; credentials were not disclosed in diagnostics")
var ErrProtocol = errors.New("invalid hosted version-1 response")
var ErrDenied = errors.New("hosted access denied")
var ErrExpired = errors.New("hosted device login expired")
var ErrCancelled = errors.New("hosted operation cancelled")
var userCodePattern = regexp.MustCompile(`^[A-Z0-9]{4}-[A-Z0-9]{4}$`)

type Session struct {
	PrincipalID string    `json:"principal_id"`
	SessionID   string    `json:"session_id"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Device struct {
	Version         string    `json:"version"`
	UserCode        string    `json:"user_code"`
	VerificationURI string    `json:"verification_uri"`
	ExpiresAt       time.Time `json:"expires_at"`
	Interval        int       `json:"interval"`
	PollSecret      string    `json:"-"`
}

type deviceWire struct {
	Version         string    `json:"version"`
	UserCode        string    `json:"user_code"`
	VerificationURI string    `json:"verification_uri"`
	ExpiresAt       time.Time `json:"expires_at"`
	Interval        int       `json:"interval"`
	PollSecret      string    `json:"poll_secret"`
}

type pollWire struct {
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	Interval  int       `json:"interval,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Token     string    `json:"session_token,omitempty"`
	Session   Session   `json:"session,omitempty"`
}

type Identity struct {
	PrincipalID  string    `json:"principal_id"`
	CredentialID string    `json:"credential_id,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitzero"`
	TenantID     string    `json:"tenant_id"`
	RepoID       string    `json:"repo_id"`
	Scope        string    `json:"scope"`
	Role         string    `json:"role,omitempty"`
}

type Client struct {
	origin string
	http   *http.Client
	now    func() time.Time
	wait   func(context.Context, time.Duration) error
}

// Roots are supplied only by trusted embedding code (tests use their TLS fixture
// root). The CLI always passes nil for the OS trust store; no insecure mode exists.
func NewClient(origin string, roots *x509.CertPool) (*Client, error) {
	if config.ValidateRemoteOrigin(origin) != nil {
		return nil, ErrCredential
	}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext, TLSHandshakeTimeout: 3 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second, MaxResponseHeaderBytes: 8192, DisableCompression: true,
		MaxConnsPerHost: 2, MaxIdleConns: 2, MaxIdleConnsPerHost: 2, IdleConnTimeout: 15 * time.Second}
	return &Client{origin: origin, http: &http.Client{Transport: transport, Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, now: time.Now, wait: waitContext}, nil
}

func (c *Client) Close() { c.http.CloseIdleConnections() }

func waitContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ErrCancelled
	case <-timer.C:
		return nil
	}
}

func decode(data []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(dst) != nil {
		return ErrProtocol
	}
	if d.Decode(new(any)) != io.EOF {
		return ErrProtocol
	}
	return nil
}

func validSessionIDs(s Session) bool {
	return config.ValidRemoteID(s.PrincipalID) && config.ValidRemoteID(s.SessionID) && s.PrincipalID != s.SessionID
}
func validSession(s Session, now time.Time) bool {
	return validSessionIDs(s) && s.ExpiresAt.After(now) && !s.ExpiresAt.After(now.Add(24*time.Hour))
}

func (c *Client) exchange(ctx context.Context, method, path, token string) (int, []byte, int, error) {
	if ctx.Err() != nil {
		return 0, nil, 0, ErrCancelled
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+path, nil)
	if err != nil {
		return 0, nil, 0, ErrNetwork
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "devspecs-cli")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, 0, ErrCancelled
		}
		return 0, nil, 0, ErrNetwork
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil || len(data) > 16384 {
		return 0, nil, 0, ErrProtocol
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return 0, nil, 0, ErrProtocol
	}
	if response.StatusCode == 403 {
		return 0, nil, 0, ErrDenied
	}
	if response.StatusCode != 200 && response.StatusCode != 201 && response.StatusCode != 202 {
		return 0, nil, 0, ErrNetwork
	}
	retry := 0
	if value := response.Header.Get("Retry-After"); value != "" {
		n, e := strconv.Atoi(value)
		if e != nil || n < 1 || n > 900 {
			return 0, nil, 0, ErrProtocol
		}
		retry = n
	}
	return response.StatusCode, data, retry, nil
}

func (c *Client) Login(ctx context.Context, notify func(Device) error) (Credential, error) {
	status, data, _, err := c.exchange(ctx, http.MethodPost, "/v1/auth/github/device", "")
	if err != nil {
		return Credential{}, err
	}
	var start deviceWire
	received := c.now()
	if status != 201 || decode(data, &start) != nil || start.Version != "1" || start.VerificationURI != "https://github.com/login/device" || !userCodePattern.MatchString(start.UserCode) || !ValidToken(start.PollSecret, "shp_") || start.Interval < 1 || start.Interval > 900 || !start.ExpiresAt.After(received) || start.ExpiresAt.After(received.Add(15*time.Minute)) {
		return Credential{}, ErrProtocol
	}
	if err := notify(Device{start.Version, start.UserCode, start.VerificationURI, start.ExpiresAt, start.Interval, ""}); err != nil {
		return Credential{}, ErrCancelled
	}
	deadline := start.ExpiresAt
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	interval := start.Interval
	for {
		if !c.now().Add(time.Duration(interval) * time.Second).Before(deadline) {
			return Credential{}, ErrExpired
		}
		if c.wait(ctx, time.Duration(interval)*time.Second) != nil {
			if !c.now().Before(deadline) {
				return Credential{}, ErrExpired
			}
			return Credential{}, ErrCancelled
		}
		if !c.now().Before(deadline) {
			return Credential{}, ErrExpired
		}
		status, data, retry, err := c.exchange(ctx, http.MethodPost, "/v1/auth/github/device/poll", start.PollSecret)
		if err != nil {
			return Credential{}, err
		}
		if !c.now().Before(deadline) {
			return Credential{}, ErrExpired
		}
		var poll pollWire
		if decode(data, &poll) != nil || poll.Version != "1" {
			return Credential{}, ErrProtocol
		}
		if status == 200 {
			if poll.Status != "authorized" || !ValidToken(poll.Token, "shs_") || !validSession(poll.Session, c.now()) {
				return Credential{}, ErrProtocol
			}
			return Credential{poll.Token, poll.Session}, nil
		}
		if status != 202 || (poll.Status != "pending" && poll.Status != "slow_down") || poll.Interval < 1 || poll.Interval > 900 || poll.Token != "" || poll.Session.PrincipalID != "" || !poll.ExpiresAt.After(c.now()) || poll.ExpiresAt.After(deadline) {
			return Credential{}, ErrProtocol
		}
		if poll.ExpiresAt.Before(deadline) {
			deadline = poll.ExpiresAt
		}
		minimum := interval
		if poll.Status == "slow_down" {
			minimum += 5
		}
		interval = max(minimum, poll.Interval, retry)
		if interval > 900 {
			return Credential{}, ErrExpired
		}
	}
}

func (c *Client) Metadata(ctx context.Context, r config.RemoteConfig, token string) (Identity, error) {
	if r.URL != c.origin || config.ValidateRemotes([]config.RemoteConfig{r}) != nil || (!ValidToken(token, "sph_") && !ValidToken(token, "shs_")) {
		return Identity{}, ErrCredential
	}
	status, data, _, err := c.exchange(ctx, http.MethodGet, "/v1/tenants/"+r.TenantID+"/repos/"+r.RepoID, token)
	if err != nil {
		return Identity{}, err
	}
	var result struct {
		Identity   Identity `json:"identity"`
		Repository struct {
			TenantID    string `json:"tenant_id"`
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"repository"`
	}
	if status != 200 || decode(data, &result) != nil || !config.ValidRemoteID(result.Identity.PrincipalID) || result.Identity.TenantID != r.TenantID || result.Identity.RepoID != r.RepoID || result.Identity.Scope != "repo:metadata:read" || result.Repository.TenantID != r.TenantID || result.Repository.ID != r.RepoID {
		return Identity{}, ErrProtocol
	}
	if ValidToken(token, "sph_") {
		if !config.ValidRemoteID(result.Identity.CredentialID) || result.Identity.CredentialID == result.Identity.PrincipalID || result.Identity.SessionID != "" || result.Identity.Role != "" {
			return Identity{}, ErrProtocol
		}
	} else if !validSession(Session{result.Identity.PrincipalID, result.Identity.SessionID, result.Identity.ExpiresAt}, c.now()) || (result.Identity.Role != "owner" && result.Identity.Role != "member" && result.Identity.Role != "read_only") || result.Identity.CredentialID != "" {
		return Identity{}, ErrProtocol
	}
	return result.Identity, nil
}

func (c *Client) Logout(ctx context.Context, token string) error {
	if !ValidToken(token, "shs_") {
		return ErrCredential
	}
	status, data, _, err := c.exchange(ctx, http.MethodPost, "/v1/auth/logout", token)
	if err != nil {
		return err
	}
	var result struct {
		Version string `json:"version"`
		Status  string `json:"status"`
	}
	if status != 200 || decode(data, &result) != nil || result.Version != "1" || result.Status != "logged_out" {
		return ErrProtocol
	}
	return nil
}
