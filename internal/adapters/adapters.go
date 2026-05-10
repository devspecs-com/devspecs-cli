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
	Parse(ctx context.Context, c Candidate) (Artifact, []Source, []todoparse.Todo, error)
}

// Candidate is a file or directory discovered by an adapter for further parsing.
type Candidate struct {
	PrimaryPath string // absolute path to the primary file
	RelPath     string // relative to repo root
	AdapterName string
}

// Artifact holds the parsed metadata for an artifact.
type Artifact struct {
	SourceIdentity string
	Kind           string
	Title          string
	Status         string
	PrimaryPath    string
	Body           string
	Extracted      map[string]any
	Tags           []string
}

// Source represents where an artifact came from.
type Source struct {
	SourceType     string
	Path           string
	SourceIdentity string
}
