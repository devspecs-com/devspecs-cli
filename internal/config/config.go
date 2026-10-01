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
	Version      int               `yaml:"version" json:"version"`
	Sources      []SourceConfig    `yaml:"sources" json:"sources"`
	Artifacts    ArtifactConfig    `yaml:"artifacts,omitempty" json:"artifacts,omitempty"`
	Experiments  ExperimentConfig  `yaml:"experiments,omitempty" json:"experiments,omitempty"`
	Integrations IntegrationConfig `yaml:"integrations,omitempty" json:"integrations,omitempty"`
}

// IntegrationConfig holds explicitly enabled DevSpecs integrations.
type IntegrationConfig struct {
	Orchestration OrchestrationConfig `yaml:"orchestration,omitempty" json:"orchestration,omitempty"`
}

// OrchestrationConfig selects one provider driver owned by DevSpecs. The
// selected driver owns and validates its options.
type OrchestrationConfig struct {
	Provider string         `yaml:"provider,omitempty" json:"provider,omitempty"`
	Options  map[string]any `yaml:"options,omitempty" json:"options,omitempty"`
}

// ArtifactConfig holds canonical opt-in artifact sources that can be expensive
// or noisy on large repositories.
type ArtifactConfig struct {
	TestCases    *bool `yaml:"test_cases,omitempty" json:"test_cases,omitempty"`
	CodeComments *bool `yaml:"code_comments,omitempty" json:"code_comments,omitempty"`
}

// ExperimentConfig holds legacy opt-in scan/indexing experiments. New callers
// should prefer ArtifactConfig; these fields remain for config compatibility.
type ExperimentConfig struct {
	IntentCandidateDiscovery *bool `yaml:"intent_candidate_discovery,omitempty" json:"intent_candidate_discovery,omitempty"`
	TestCaseArtifacts        *bool `yaml:"test_case_artifacts,omitempty" json:"test_case_artifacts,omitempty"`
	SupportDocDiscovery      *bool `yaml:"support_doc_discovery,omitempty" json:"support_doc_discovery,omitempty"`
}

// SourceConfig defines a source type and its discovery paths.
type SourceConfig struct {
	Type  string       `yaml:"type" json:"type"`
	Path  string       `yaml:"path,omitempty" json:"path,omitempty"`
	Paths []string     `yaml:"paths,omitempty" json:"paths,omitempty"`
	Rules []SourceRule `yaml:"rules,omitempty" json:"rules,omitempty"`
}

// SourceRule maps a glob (relative to configured markdown paths) to kind/subtype/tags.
type SourceRule struct {
	Match   string   `yaml:"match" json:"match"`
	Kind    string   `yaml:"kind" json:"kind"`
	Subtype string   `yaml:"subtype,omitempty" json:"subtype,omitempty"`
	Tags    []string `yaml:"tags,omitempty" json:"tags,omitempty"`
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
				".claude/notes", ".claude/plans", ".codex/plans", ".codex/notes",
				".agents/skills", ".claude/skills", ".codex/skills", ".cursor/commands", ".windsurf/workflows", "agents",
				"docs/prd", "docs/product-specs", "product-specs", "docs/requirements", "requirements",
				"rfcs", "rfc", "RFCS", "docs/rfcs", "docs/rfc", "docs/RFCS",
				"roadmaps", "docs/roadmaps",
				"docs/design", "docs/design-docs", "design-docs", "docs/technical",
				"architecture", "docs/architecture",
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

// WithSupportDocDiscovery returns a config copy with bounded support-doc discovery set.
func WithSupportDocDiscovery(cfg *RepoConfig, enabled bool) *RepoConfig {
	out := CloneRepoConfig(cfg)
	out.Experiments.SupportDocDiscovery = boolPtr(enabled)
	return out
}

// WithTestCaseArtifacts returns a config copy with test-case artifact indexing set.
func WithTestCaseArtifacts(cfg *RepoConfig, enabled bool) *RepoConfig {
	out := CloneRepoConfig(cfg)
	out.Artifacts.TestCases = boolPtr(enabled)
	return out
}

// WithCodeCommentArtifacts returns a config copy with code-comment artifact indexing set.
func WithCodeCommentArtifacts(cfg *RepoConfig, enabled bool) *RepoConfig {
	out := CloneRepoConfig(cfg)
	out.Artifacts.CodeComments = boolPtr(enabled)
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
	if cfg.Experiments.TestCaseArtifacts != nil {
		out.Experiments.TestCaseArtifacts = boolPtr(*cfg.Experiments.TestCaseArtifacts)
	}
	if cfg.Experiments.SupportDocDiscovery != nil {
		out.Experiments.SupportDocDiscovery = boolPtr(*cfg.Experiments.SupportDocDiscovery)
	}
	if cfg.Artifacts.TestCases != nil {
		out.Artifacts.TestCases = boolPtr(*cfg.Artifacts.TestCases)
	}
	if cfg.Artifacts.CodeComments != nil {
		out.Artifacts.CodeComments = boolPtr(*cfg.Artifacts.CodeComments)
	}
	out.Integrations.Orchestration.Options = cloneConfigMap(cfg.Integrations.Orchestration.Options)
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

func cloneConfigMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneConfigValue(value)
	}
	return out
}

func cloneConfigValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneConfigMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneConfigValue(item)
		}
		return out
	default:
		return value
	}
}

func (e ExperimentConfig) IntentCandidateDiscoveryEnabled(defaultValue bool) bool {
	if e.IntentCandidateDiscovery == nil {
		return defaultValue
	}
	return *e.IntentCandidateDiscovery
}

func (e ExperimentConfig) TestCaseArtifactsEnabled(defaultValue bool) bool {
	if e.TestCaseArtifacts == nil {
		return defaultValue
	}
	return *e.TestCaseArtifacts
}

func (e ExperimentConfig) SupportDocDiscoveryEnabled(defaultValue bool) bool {
	if e.SupportDocDiscovery == nil {
		return defaultValue
	}
	return *e.SupportDocDiscovery
}

func (c RepoConfig) TestCaseArtifactsEnabled(defaultValue bool) bool {
	if c.Artifacts.TestCases != nil {
		return *c.Artifacts.TestCases
	}
	return c.Experiments.TestCaseArtifactsEnabled(defaultValue)
}

func (c RepoConfig) CodeCommentArtifactsEnabled(defaultValue bool) bool {
	if c.Artifacts.CodeComments == nil {
		return defaultValue
	}
	return *c.Artifacts.CodeComments
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
