package config

import (
	"errors"
	"net"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const MaxRemotes = 16

var ErrRemoteConfig = errors.New("invalid version-1 remote metadata configuration")
var remoteName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
var remoteID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var originHost = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

// RemoteConfig contains metadata only. Credentials and trust confirmations are
// deliberately never persisted in repository-controlled configuration.
type RemoteConfig struct {
	Version  int    `yaml:"version" json:"version"`
	Name     string `yaml:"name" json:"name"`
	URL      string `yaml:"url" json:"url"`
	TenantID string `yaml:"tenant_id" json:"tenant_id"`
	RepoID   string `yaml:"repo_id" json:"repo_id"`
}

func (r *RemoteConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode || len(node.Content) != 10 {
		return ErrRemoteConfig
	}
	seen := map[string]bool{}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		switch key {
		case "version", "name", "url", "tenant_id", "repo_id":
		default:
			return ErrRemoteConfig
		}
		if seen[key] {
			return ErrRemoteConfig
		}
		seen[key] = true
		if node.Content[i+1].Kind != yaml.ScalarNode {
			return ErrRemoteConfig
		}
	}
	type plain RemoteConfig
	var value plain
	if node.Decode(&value) != nil {
		return ErrRemoteConfig
	}
	*r = RemoteConfig(value)
	return ValidateRemotes([]RemoteConfig{*r})
}

func ValidRemoteID(value string) bool { return remoteID.MatchString(value) }

func ValidateRemoteOrigin(value string) error {
	if len(value) > 512 {
		return ErrRemoteConfig
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.String() != value || strings.ContainsAny(value, "%\\?#") {
		return ErrRemoteConfig
	}
	if u.Path != "" && (path.Clean(u.Path) != u.Path || u.Path == "/" || strings.Contains(u.Path, "//")) {
		return ErrRemoteConfig
	}
	host := u.Hostname()
	if len(host) > 253 || host != strings.ToLower(host) || (!originHost.MatchString(host) && net.ParseIP(host) == nil) || strings.Contains(host, "..") {
		return ErrRemoteConfig
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
			return ErrRemoteConfig
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return ErrRemoteConfig
	}
	return nil
}

func ValidateRemotes(remotes []RemoteConfig) error {
	if len(remotes) > MaxRemotes {
		return ErrRemoteConfig
	}
	seen := map[string]bool{}
	for _, r := range remotes {
		if r.Version != 1 || !remoteName.MatchString(r.Name) || seen[r.Name] || ValidateRemoteOrigin(r.URL) != nil || !ValidRemoteID(r.TenantID) || !ValidRemoteID(r.RepoID) {
			return ErrRemoteConfig
		}
		seen[r.Name] = true
	}
	return nil
}
