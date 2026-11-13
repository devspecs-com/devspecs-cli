package orchestration

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"
)

var errHTTPDriverBinding = errors.New("HTTP dispatch trusted profile or repository binding is invalid")

type httpDriverProfile struct {
	Schema         string                           `json:"schema"`
	Version        int                              `json:"schema_version"`
	Enabled        bool                             `json:"enabled"`
	ReadOnly       bool                             `json:"read_only"`
	AllowEdits     bool                             `json:"allow_edits,omitempty"`
	ReportRequired bool                             `json:"report_required,omitempty"`
	Verification   *httpDispatchVerificationProfile `json:"verification,omitempty"`
	Endpoint       string                           `json:"endpoint"`
	SourceURL      string                           `json:"source_url"`
	SourceCommit   string                           `json:"source_commit"`
	PrincipalID    string                           `json:"principal_id"`
	CredentialID   string                           `json:"credential_id"`
	TokenEnv       string                           `json:"token_env"`
	CAFile         string                           `json:"ca_file,omitempty"`
}

type httpDriverTrust struct {
	Endpoint, SourceURL, SourceCommit, PrincipalID, RootsSHA256 string
	ReadOnly                                                    bool
	AllowEdits                                                  bool   `json:",omitempty"`
	ReportRequired                                              bool   `json:",omitempty"`
	VerificationProfile                                         string `json:",omitempty"`
	VerificationStrength                                        string `json:",omitempty"`
}

var httpDriverIdentity = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var httpDriverTokenEnv = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)

func loadHTTPDriverProfile(repos ...string) (httpDriverProfile, httpDriverTrust, *x509.CertPool, error) {
	var cfg httpDriverProfile
	raw, err := readHTTPDriverTrustedFile(os.Getenv("DEVSPECS_HTTP_DISPATCH_CONFIG"), repos...)
	if err != nil || !strictHTTPDriverJSON(raw, &cfg) || cfg.Schema != "devspecs.dispatch.http_config" ||
		cfg.Version != 1 || !cfg.Enabled || cfg.ReadOnly == cfg.AllowEdits || !httpDriverIdentity.MatchString(cfg.PrincipalID) ||
		!httpDriverIdentity.MatchString(cfg.CredentialID) || !httpDriverTokenEnv.MatchString(cfg.TokenEnv) ||
		!validHTTPDispatchSource(httpDispatchConfirmedSource{URL: cfg.SourceURL, Commit: cfg.SourceCommit}) {
		return cfg, httpDriverTrust{}, nil, errHTTPDriverBinding
	}
	if cfg.Verification != nil && (!validHTTPDispatchName(cfg.Verification.Profile, 128) ||
		(cfg.Verification.Strength != "suite" && cfg.Verification.Strength != "smoke")) {
		return cfg, httpDriverTrust{}, nil, errHTTPDriverBinding
	}
	if cfg.AllowEdits && !httpDriverSHA1.MatchString(cfg.SourceCommit) {
		return cfg, httpDriverTrust{}, nil, errHTTPDriverBinding
	}
	endpoint, err := parseHTTPDispatchEndpoint(cfg.Endpoint)
	if err != nil {
		return cfg, httpDriverTrust{}, nil, errHTTPDriverBinding
	}
	trust := httpDriverTrust{Endpoint: strings.TrimSuffix(endpoint.String(), "/"), SourceURL: cfg.SourceURL,
		SourceCommit: cfg.SourceCommit, PrincipalID: cfg.PrincipalID, RootsSHA256: "system-roots", ReadOnly: cfg.ReadOnly,
		AllowEdits: cfg.AllowEdits, ReportRequired: cfg.ReportRequired}
	if cfg.Verification != nil {
		trust.VerificationProfile, trust.VerificationStrength = cfg.Verification.Profile, cfg.Verification.Strength
	}
	var roots *x509.CertPool
	if cfg.CAFile != "" {
		pem, err := readHTTPDriverTrustedFile(cfg.CAFile, repos...)
		roots = x509.NewCertPool()
		if err != nil || !roots.AppendCertsFromPEM(pem) {
			return cfg, httpDriverTrust{}, nil, errHTTPDriverBinding
		}
		trust.RootsSHA256 = httpDispatchDigest(pem)
	}
	return cfg, trust, roots, nil
}

func strictHTTPDriverJSON(raw []byte, out any) bool {
	if !utf8.Valid(raw) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := readHTTPDispatchValue(decoder, 0)
	if err != nil || !exactHTTPDriverJSONFields(value, reflect.TypeOf(out)) {
		return false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return false
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out) == nil
}

// encoding/json matches struct fields case-insensitively. Trusted profiles and
// extension metadata require exact names, including nested optional objects.
func exactHTTPDriverJSONFields(value any, target reflect.Type) bool {
	for target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	if target == reflect.TypeFor[json.RawMessage]() || target.Kind() == reflect.Interface {
		return true
	}
	if object, ok := value.(map[string]any); ok {
		if target.Kind() == reflect.Map {
			for _, child := range object {
				if !exactHTTPDriverJSONFields(child, target.Elem()) {
					return false
				}
			}
		} else if target.Kind() == reflect.Struct {
			for name, child := range object {
				found := false
				for i := 0; i < target.NumField(); i++ {
					field := target.Field(i)
					key := strings.Split(field.Tag.Get("json"), ",")[0]
					if key == "" {
						key = field.Name
					}
					if field.PkgPath == "" && key == name {
						found = true
						if !exactHTTPDriverJSONFields(child, field.Type) {
							return false
						}
						break
					}
				}
				if !found {
					return false
				}
			}
		}
	} else if array, ok := value.([]any); ok && (target.Kind() == reflect.Slice || target.Kind() == reflect.Array) {
		for _, child := range array {
			if !exactHTTPDriverJSONFields(child, target.Elem()) {
				return false
			}
		}
	}
	return true
}

func readHTTPDriverTrustedFile(name string, repos ...string) ([]byte, error) {
	if !filepath.IsAbs(name) {
		return nil, errHTTPDriverBinding
	}
	resolved, err := httpDriverResolvedPath(name)
	if err != nil {
		return nil, errHTTPDriverBinding
	}
	for _, repo := range repos {
		if repo == "" || !httpDriverOutside(resolved, repo) {
			return nil, errHTTPDriverBinding
		}
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Size() > httpDispatchSmallBody {
		return nil, errHTTPDriverBinding
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, errHTTPDriverBinding
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errHTTPDriverBinding
	}
	raw, err := io.ReadAll(io.LimitReader(file, httpDispatchSmallBody+1))
	if err != nil || len(raw) > httpDispatchSmallBody {
		return nil, errHTTPDriverBinding
	}
	return raw, nil
}

// File identity comparisons cover junction/short-name aliases and do not rely
// on filepath.Rel succeeding across Windows volumes.
func httpDriverOutside(name, repo string) bool {
	name, err := httpDriverResolvedPath(name)
	if err != nil {
		return false
	}
	root, err := httpDriverResolvedPath(repo)
	if err != nil {
		return false
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		return false
	}
	for current := name; ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if err != nil || os.SameFile(info, rootInfo) {
			return false
		}
		if filepath.Dir(current) == current {
			return true
		}
	}
}

func httpDriverTaskRoot(name string) (string, error) {
	abs, err := filepath.Abs(name)
	if err != nil {
		return "", errHTTPDriverBinding
	}
	root, err := httpDriverResolvedPath(abs)
	if err != nil {
		return "", errHTTPDriverBinding
	}
	for current := root; ; current = filepath.Dir(current) {
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return current, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", errHTTPDriverBinding
		}
		if filepath.Dir(current) == current {
			return root, nil
		}
	}
}

func checkHTTPDriverBinding(ctx context.Context, checkout string, trust httpDriverTrust) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	runner, err := newHTTPDriverLocalGit()
	if err != nil {
		return err
	}
	if _, err := httpDriverRepositoryRoot(ctx, runner, checkout); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errHTTPDriverBinding
	}
	if err := checkHTTPDriverGitPolicy(ctx, runner, checkout); err != nil {
		return err
	}
	remote, err := runGitText(ctx, runner, checkout, "remote", "get-url", "--all", "origin")
	if err != nil || remote != trust.SourceURL {
		return errHTTPDriverBinding
	}
	if err := checkHTTPDispatchCheckout(ctx, runner, checkout, trust.SourceCommit); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errHTTPDriverBinding
	}
	return nil
}
