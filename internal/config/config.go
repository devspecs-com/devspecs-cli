package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RepoConfig represents the .devspecs/config.yaml file in a repository.
type RepoConfig struct {
	Version     int              `yaml:"version"`
	Sources     []SourceConfig   `yaml:"sources"`
	Experiments ExperimentConfig `yaml:"experiments,omitempty"`
}

// ExperimentConfig holds opt-in scan/indexing experiments. These switches are
// intentionally explicit so evals can compare baseline and experiment runs.
type ExperimentConfig struct {
	IntentCandidateDiscovery *bool `yaml:"intent_candidate_discovery,omitempty"`
}

// SourceConfig defines a source type and its discovery paths.
type SourceConfig struct {
	Type  string       `yaml:"type"`
	Path  string       `yaml:"path,omitempty"`
	Paths []string     `yaml:"paths,omitempty"`
	Rules []SourceRule `yaml:"rules,omitempty"`
}

// SourceRule maps a glob (relative to configured markdown paths) to kind/subtype/tags.
type SourceRule struct {
	Match   string   `yaml:"match"`
	Kind    string   `yaml:"kind"`
	Subtype string   `yaml:"subtype,omitempty"`
	Tags    []string `yaml:"tags,omitempty"`
}

// DefaultRepoConfig returns sensible defaults per spec §10.
func DefaultRepoConfig() *RepoConfig {
	return &RepoConfig{
		Version: 1,
		Sources: []SourceConfig{
			{Type: "openspec", Path: "openspec"},
			{Type: "adr", Paths: []string{"docs/adr", "docs/adrs", "adr", "adrs"}},
			{Type: "markdown", Paths: []string{
				"specs", "docs/specs", "plans", "docs/plans", ".cursor/plans",
				".claude/notes", "docs/prd", "rfcs", "rfc", "docs/rfcs", "docs/rfc",
				"docs/design", "docs/technical",
				"_bmad-output", ".specify/memory",
			}},
			{Type: "source_context"},
		},
	}
}

// WithIntentCandidateDiscovery returns a config copy with the intent candidate
// discovery experiment set. A nil input starts from the default repo config.
func WithIntentCandidateDiscovery(cfg *RepoConfig, enabled bool) *RepoConfig {
	out := CloneRepoConfig(cfg)
	out.Experiments.IntentCandidateDiscovery = boolPtr(enabled)
	return out
}

// WithDefaultIntentCandidateDiscovery enables broad intent discovery only when
// the repo config did not explicitly opt in or out.
func WithDefaultIntentCandidateDiscovery(cfg *RepoConfig, enabled bool) *RepoConfig {
	out := CloneRepoConfig(cfg)
	if out.Experiments.IntentCandidateDiscovery == nil {
		out.Experiments.IntentCandidateDiscovery = boolPtr(enabled)
	}
	return out
}

// CloneRepoConfig returns a deep-enough copy for scan-time option mutation.
func CloneRepoConfig(cfg *RepoConfig) *RepoConfig {
	if cfg == nil {
		cfg = DefaultRepoConfig()
	}
	out := *cfg
	if cfg.Experiments.IntentCandidateDiscovery != nil {
		out.Experiments.IntentCandidateDiscovery = boolPtr(*cfg.Experiments.IntentCandidateDiscovery)
	}
	out.Sources = make([]SourceConfig, len(cfg.Sources))
	for i, src := range cfg.Sources {
		out.Sources[i] = src
		out.Sources[i].Paths = append([]string(nil), src.Paths...)
		out.Sources[i].Rules = make([]SourceRule, len(src.Rules))
		for j, rule := range src.Rules {
			out.Sources[i].Rules[j] = rule
			out.Sources[i].Rules[j].Tags = append([]string(nil), rule.Tags...)
		}
	}
	return &out
}

func (e ExperimentConfig) IntentCandidateDiscoveryEnabled(defaultValue bool) bool {
	if e.IntentCandidateDiscovery == nil {
		return defaultValue
	}
	return *e.IntentCandidateDiscovery
}

func boolPtr(value bool) *bool {
	return &value
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
	if err := ValidateRepoConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid repo config: %w", err)
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
