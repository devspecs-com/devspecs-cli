// Package adapters defines the Adapter interface and shared types for artifact discovery.
package adapters

import (
	"context"

	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
	"github.com/devspecs-com/devspecs-cli/internal/config"
)

// Adapter discovers and parses artifacts of a specific type.
type Adapter interface {
	Name() string
	Discover(ctx context.Context, repoRoot string, cfg *config.RepoConfig) ([]Candidate, error)
	Parse(ctx context.Context, c Candidate) (Artifact, []Source, todoparse.ParseResult, error)
}

// FileDiscoveryAdapter can discover candidates from a shared file inventory.
// Scanner uses it as an optimization; adapters still implement Discover for
// compatibility with direct adapter tests and non-shared scan paths.
type FileDiscoveryAdapter interface {
	Adapter
	AcceptsFile(rel string, size int64, cfg *config.RepoConfig) bool
	DiscoverFile(ctx context.Context, file FileCandidate, cfg *config.RepoConfig) ([]Candidate, error)
}

// FileCandidate is a single repo file read by a shared scanner pass.
type FileCandidate struct {
	RepoRoot    string
	PrimaryPath string
	RelPath     string
	Size        int64
	Body        []byte
}

// Candidate is a file or directory discovered by an adapter for further parsing.
type Candidate struct {
	PrimaryPath string // absolute path to the primary file
	RelPath     string // relative to repo root
	AdapterName string
	// FormatProfile and LayoutGroup may be set by adapters; scan persists them on Source rows.
	FormatProfile string
	LayoutGroup   string
	// ArtifactScope and Role are optional adapter hints for hierarchical
	// artifact families such as OpenSpec collections, bundles, and children.
	ArtifactScope string
	Role          string
	// MarkdownPaths and MarkdownRules apply when AdapterName is "markdown".
	MarkdownPaths []string
	MarkdownRules []config.SourceRule
	// DiscoveryScore and DiscoveryReasons explain why broad/experimental
	// candidate discovery admitted this file into the adapter pipeline.
	DiscoveryScore   float64
	DiscoveryReasons []string
	// Unit fields are optional sub-file extraction hints used by adapters that
	// emit multiple artifacts from one physical file.
	UnitName       string
	UnitParent     string
	UnitBody       string
	UnitLanguage   string
	UnitFramework  string
	UnitStartLine  int
	UnitEndLine    int
	UnitSymbols    []string
	UnitAssertions []string
}

// Artifact holds the parsed metadata for an artifact.
type Artifact struct {
	SourceIdentity string
	Kind           string
	Subtype        string
	Title          string
	Status         string
	PrimaryPath    string
	Body           string
	Extracted      map[string]any
	Tags           []string
	FormatProfile  string
	LayoutGroup    string
}

// Source represents where an artifact came from.
type Source struct {
	SourceType     string
	Path           string
	SourceIdentity string
	FormatProfile  string
	LayoutGroup    string
}
