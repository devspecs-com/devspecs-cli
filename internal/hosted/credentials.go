package hosted

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/zalando/go-keyring"
)

var ErrVault = errors.New("OS credential vault unavailable; no plaintext fallback is supported")
var ErrMissing = errors.New("no saved credential for this exact remote binding")
var ErrCredential = errors.New("invalid or mismatched hosted credential")

// Vault is lazy: constructing commands or the adapter never reads OS secrets.
type Vault interface {
	Get(config.RemoteConfig) (Credential, error)
	Set(config.RemoteConfig, Credential) error
	Delete(config.RemoteConfig) error
}

type Credential struct {
	Token string `json:"-"`
	Session
}

type credentialRecord struct {
	Version  int     `json:"version"`
	Origin   string  `json:"origin"`
	TenantID string  `json:"tenant_id"`
	RepoID   string  `json:"repo_id"`
	Token    string  `json:"session_token"`
	Session  Session `json:"session"`
}

type OSVault struct{}

func vaultKey(r config.RemoteConfig) (string, string, error) {
	if config.ValidateRemotes([]config.RemoteConfig{r}) != nil {
		return "", "", ErrCredential
	}
	hash := sha256.Sum256([]byte(r.URL))
	return "devspecs.specs-place.v1." + hex.EncodeToString(hash[:]), r.TenantID + "/" + r.RepoID, nil
}

func encodeCredential(r config.RemoteConfig, c Credential) (string, error) {
	if !validSession(c.Session, time.Now()) || !ValidToken(c.Token, "shs_") {
		return "", ErrCredential
	}
	data, err := json.Marshal(credentialRecord{1, r.URL, r.TenantID, r.RepoID, c.Token, c.Session})
	if err != nil || len(data) >= 2048 {
		return "", ErrCredential
	}
	return string(data), nil
}

func decodeCredential(r config.RemoteConfig, value string) (Credential, error) {
	if len(value) >= 2048 {
		return Credential{}, ErrCredential
	}
	var record credentialRecord
	if decode([]byte(value), &record) != nil || record.Version != 1 || record.Origin != r.URL || record.TenantID != r.TenantID || record.RepoID != r.RepoID || !ValidToken(record.Token, "shs_") || !validSessionIDs(record.Session) || record.Session.ExpiresAt.IsZero() {
		return Credential{}, ErrCredential
	}
	// Expired sessions remain readable solely so explicit logout can revoke them.
	return Credential{record.Token, record.Session}, nil
}

func (OSVault) Get(r config.RemoteConfig) (Credential, error) {
	service, user, err := vaultKey(r)
	if err != nil {
		return Credential{}, err
	}
	value, err := keyring.Get(service, user)
	if errors.Is(err, keyring.ErrNotFound) {
		return Credential{}, ErrMissing
	}
	if err != nil {
		return Credential{}, ErrVault
	}
	return decodeCredential(r, value)
}

func (OSVault) Set(r config.RemoteConfig, c Credential) error {
	service, user, err := vaultKey(r)
	if err != nil {
		return err
	}
	value, err := encodeCredential(r, c)
	if err != nil {
		return err
	}
	if keyring.Set(service, user, value) != nil {
		return ErrVault
	}
	return nil
}

func (OSVault) Delete(r config.RemoteConfig) error {
	service, user, err := vaultKey(r)
	if err != nil {
		return err
	}
	if err := keyring.Delete(service, user); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return ErrVault
	}
	return nil
}

func ValidToken(token, prefix string) bool {
	if (prefix != "shs_" && prefix != "sph_" && prefix != "shp_") || len(token) != 47 || token[:4] != prefix {
		return false
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(token[4:])
	return err == nil && len(data) == 32
}

// Environment requires the explicit operator pair. It never falls back to a vault.
func Environment(r config.RemoteConfig, getenv func(string) string) (string, bool, error) {
	if getenv == nil {
		return "", false, ErrCredential
	}
	origin, token := getenv("DEVSPECS_REMOTE_URL"), getenv("DEVSPECS_REMOTE_TOKEN")
	if origin == "" && token == "" {
		return "", false, nil
	}
	if origin != r.URL || config.ValidateRemoteOrigin(origin) != nil || (!ValidToken(token, "sph_") && !ValidToken(token, "shs_")) {
		return "", true, ErrCredential
	}
	return token, true, nil
}
