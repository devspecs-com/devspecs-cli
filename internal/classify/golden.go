package classify

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type GoldenFile struct {
	Version         int          `json:"version" yaml:"version"`
	Fixture         string       `json:"fixture" yaml:"fixture"`
	ClassifierCases []GoldenCase `json:"classifier_cases" yaml:"classifier_cases"`
}

type GoldenCase struct {
	ID         string            `json:"id" yaml:"id"`
	Path       string            `json:"path" yaml:"path"`
	Scope      Scope             `json:"scope" yaml:"scope"`
	Expected   GoldenExpectation `json:"expected" yaml:"expected"`
	Provenance *SampleProvenance `json:"provenance,omitempty" yaml:"provenance,omitempty"`
	Notes      string            `json:"notes,omitempty" yaml:"notes,omitempty"`
}

type GoldenExpectation struct {
	Classifier        string                 `json:"classifier" yaml:"classifier"`
	Scope             Scope                  `json:"scope" yaml:"scope"`
	Subformat         string                 `json:"subformat,omitempty" yaml:"subformat,omitempty"`
	Family            string                 `json:"family,omitempty" yaml:"family,omitempty"`
	Kind              string                 `json:"kind,omitempty" yaml:"kind,omitempty"`
	Subtype           string                 `json:"subtype,omitempty" yaml:"subtype,omitempty"`
	Status            string                 `json:"status,omitempty" yaml:"status,omitempty"`
	Authority         string                 `json:"authority,omitempty" yaml:"authority,omitempty"`
	FormatProfile     string                 `json:"format_profile,omitempty" yaml:"format_profile,omitempty"`
	ShouldIndex       bool                   `json:"should_index" yaml:"should_index"`
	MustNotClassifyAs []string               `json:"must_not_classify_as,omitempty" yaml:"must_not_classify_as,omitempty"`
	ChildCandidates   []GoldenChildCandidate `json:"child_candidates,omitempty" yaml:"child_candidates,omitempty"`
	RequiredReasons   []ReasonCode           `json:"required_reasons,omitempty" yaml:"required_reasons,omitempty"`
}

type GoldenChildCandidate struct {
	Path string `json:"path" yaml:"path"`
	Role string `json:"role" yaml:"role"`
}

type SampleProvenance struct {
	SourceURL      string `json:"source_url,omitempty" yaml:"source_url,omitempty"`
	Repository     string `json:"repository,omitempty" yaml:"repository,omitempty"`
	CommitSHA      string `json:"commit_sha,omitempty" yaml:"commit_sha,omitempty"`
	License        string `json:"license,omitempty" yaml:"license,omitempty"`
	OriginalPath   string `json:"original_path,omitempty" yaml:"original_path,omitempty"`
	FormatLabel    string `json:"format_label,omitempty" yaml:"format_label,omitempty"`
	CanCommitFile  bool   `json:"can_commit_file,omitempty" yaml:"can_commit_file,omitempty"`
	ReductionNotes string `json:"reduction_notes,omitempty" yaml:"reduction_notes,omitempty"`
}

func LoadGoldenFile(path string) (GoldenFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return GoldenFile{}, err
	}
	var out GoldenFile
	if err := yaml.Unmarshal(data, &out); err != nil {
		return GoldenFile{}, err
	}
	if err := ValidateGoldenFile(out); err != nil {
		return GoldenFile{}, err
	}
	return out, nil
}

func ValidateGoldenFile(g GoldenFile) error {
	if g.Version != 1 {
		return fmt.Errorf("classifier golden version must be 1, got %d", g.Version)
	}
	if g.Fixture == "" {
		return fmt.Errorf("classifier golden fixture is required")
	}
	if len(g.ClassifierCases) == 0 {
		return fmt.Errorf("classifier golden file must define at least one case")
	}
	seen := map[string]bool{}
	for _, c := range g.ClassifierCases {
		if c.ID == "" {
			return fmt.Errorf("classifier case id is required")
		}
		if seen[c.ID] {
			return fmt.Errorf("duplicate classifier case id %q", c.ID)
		}
		seen[c.ID] = true
		if c.Path == "" {
			return fmt.Errorf("classifier case %q path is required", c.ID)
		}
		if err := ValidateScope(c.Scope); err != nil {
			return fmt.Errorf("classifier case %q: %w", c.ID, err)
		}
		if c.Expected.Classifier == "" {
			return fmt.Errorf("classifier case %q expected.classifier is required", c.ID)
		}
		if err := ValidateScope(c.Expected.Scope); err != nil {
			return fmt.Errorf("classifier case %q expected: %w", c.ID, err)
		}
		for _, child := range c.Expected.ChildCandidates {
			if child.Path == "" || child.Role == "" {
				return fmt.Errorf("classifier case %q child candidates require path and role", c.ID)
			}
		}
	}
	return nil
}
