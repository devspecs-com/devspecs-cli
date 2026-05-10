package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RepoConfig represents the .devspecs/config.yaml file in a repository.
type RepoConfig struct {
	Version int            `yaml:"version"`
	Sources []SourceConfig `yaml:"sources"`
}

// SourceConfig defines a source type and its discovery paths.
type SourceConfig struct {
	Type  string   `yaml:"type"`
	Path  string   `yaml:"path,omitempty"`
	Paths []string `yaml:"paths,omitempty"`
}

// DefaultRepoConfig returns sensible defaults per spec §10.
func DefaultRepoConfig() *RepoConfig {
	return &RepoConfig{
		Version: 1,
		Sources: []SourceConfig{
			{Type: "openspec", Path: "openspec"},
			{Type: "adr", Paths: []string{"docs/adr", "docs/adrs", "adr", "adrs"}},
			{Type: "markdown", Paths: []string{"specs", "docs/specs", "plans", "docs/plans", ".cursor/plans", "docs"}},
		},
	}
}

// RepoConfigPath returns the path to the repo config file for the given root.
func RepoConfigPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".devspecs", "config.yaml")
}

// LoadRepoConfig reads and parses .devspecs/config.yaml from the given repo root.
// Returns nil, nil if the file does not exist.
func LoadRepoConfig(repoRoot string) (*RepoConfig, error) {
	path := RepoConfigPath(repoRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var cfg RepoConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// WriteRepoConfig writes the config to .devspecs/config.yaml, creating the directory if needed.
func WriteRepoConfig(repoRoot string, cfg *RepoConfig) error {
	dir := filepath.Join(repoRoot, ".devspecs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(RepoConfigPath(repoRoot), data, 0o644)
}
