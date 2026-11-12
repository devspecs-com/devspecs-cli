//go:build devspecs_http_loopback

package orchestration

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type httpPilotConfig struct {
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schema_version"`
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	SourceURL     string `json:"source_url"`
	PrincipalID   string `json:"principal_id"`
	CredentialID  string `json:"credential_id"`
	TokenEnv      string `json:"token_env"`
	CAFile        string `json:"ca_file,omitempty"`
}

var (
	httpPilotID  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	httpPilotEnv = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)
)

// NewPilotDispatch is available only in the experimental build. The absolute
// config path is selected by the caller, never discovered in a repository.
// Construction validates configuration and reads the named token, but makes
// no network request.
func NewPilotDispatch(home, configPath string) (*LoopbackDispatch, error) {
	if !filepath.IsAbs(home) {
		return nil, errLoopbackBinding
	}
	raw, err := readHTTPPilotFile(configPath)
	if err != nil {
		return nil, errLoopbackBinding
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	value, err := readHTTPDispatchValue(dec, 0)
	if err != nil {
		return nil, errLoopbackBinding
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errLoopbackBinding
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errLoopbackBinding
	}
	for key := range object {
		switch key {
		case "schema", "schema_version", "enabled", "endpoint", "source_url", "principal_id", "credential_id", "token_env", "ca_file":
		default:
			return nil, errLoopbackBinding
		}
	}
	var cfg httpPilotConfig
	if json.Unmarshal(raw, &cfg) != nil || cfg.Schema != "devspecs.dispatch.pilot_config" ||
		cfg.SchemaVersion != 1 || !cfg.Enabled || !httpPilotID.MatchString(cfg.PrincipalID) ||
		!httpPilotID.MatchString(cfg.CredentialID) || !httpPilotEnv.MatchString(cfg.TokenEnv) {
		return nil, errLoopbackBinding
	}
	endpoint, err := parseHTTPDispatchEndpoint(cfg.Endpoint)
	if err != nil {
		return nil, errLoopbackBinding
	}
	host := endpoint.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		// Explicit TLS loopback with a caller-selected CA is useful for offline
		// conformance. Ordinary pilot endpoints use a verified DNS hostname.
		if !ip.IsLoopback() || cfg.CAFile == "" {
			return nil, errLoopbackBinding
		}
	} else if !validHTTPPilotHostname(host) {
		return nil, errLoopbackBinding
	}
	if !validHTTPDispatchSourceURL(cfg.SourceURL) {
		return nil, errLoopbackBinding
	}
	var roots *x509.CertPool
	trustDigest := "system-roots"
	if cfg.CAFile != "" {
		pem, err := readHTTPPilotFile(cfg.CAFile)
		roots = x509.NewCertPool()
		if err != nil || !roots.AppendCertsFromPEM(pem) {
			return nil, errLoopbackBinding
		}
		trustDigest = httpDispatchDigest(pem)
	}
	token, ok := os.LookupEnv(cfg.TokenEnv)
	if !ok {
		return nil, errLoopbackBinding
	}
	transport, err := newHTTPDispatchClient(endpoint, token, roots)
	if err != nil {
		return nil, errLoopbackBinding
	}
	return &LoopbackDispatch{
		store: StateStore{Root: home}, transport: transport, trustSHA256: trustDigest,
		caller:    httpDispatchCaller{PrincipalID: cfg.PrincipalID, CredentialID: cfg.CredentialID},
		sourceURL: cfg.SourceURL, readOnly: true,
	}, nil
}

func validHTTPPilotHostname(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

func readHTTPPilotFile(name string) ([]byte, error) {
	if !filepath.IsAbs(name) {
		return nil, errLoopbackBinding
	}
	info, err := os.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() > httpDispatchSmallBody {
		return nil, errLoopbackBinding
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, errLoopbackBinding
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, errLoopbackBinding
	}
	raw, err := io.ReadAll(io.LimitReader(file, httpDispatchSmallBody+1))
	if err != nil || len(raw) > httpDispatchSmallBody {
		return nil, errLoopbackBinding
	}
	return raw, nil
}
