// Package ignore implements the DevSpecs v0.1 ignore stack: repo-root .gitignore,
// .git/info/exclude when present, and repo-root .aiignore (gitignore-like syntax).
package ignore

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"
)

type ctxKey struct{}

// ctxMatcherKey is the context key for an optional *Matcher passed to adapters.
var ctxMatcherKey = ctxKey{}

// WithContext returns ctx carrying m for adapters and discover (nil m is a no-op).
func WithContext(ctx context.Context, m *Matcher) context.Context {
	return context.WithValue(ctx, ctxMatcherKey, m)
}

// FromContext returns the matcher from ctx, or nil.
func FromContext(ctx context.Context) *Matcher {
	if ctx == nil {
		return nil
	}
	m, _ := ctx.Value(ctxMatcherKey).(*Matcher)
	return m
}

// Matcher answers whether a repo-relative path should be skipped (gitignore rules).
type Matcher struct {
	repoRoot string
	gi       gitignore.IgnoreParser
}

// NewMatcher loads .gitignore, .git/info/exclude, and .aiignore from repoRoot (missing files ignored).
// Returns a non-nil Matcher; matching is a no-op when no patterns exist.
func NewMatcher(repoRoot string) (*Matcher, error) {
	repoRoot = filepath.Clean(repoRoot)
	var lines []string
	for _, p := range []string{
		filepath.Join(repoRoot, ".gitignore"),
		filepath.Join(repoRoot, ".git", "info", "exclude"),
		filepath.Join(repoRoot, ".aiignore"),
	} {
		lines = append(lines, readPatternLines(p)...)
	}
	var gi gitignore.IgnoreParser
	if len(lines) > 0 {
		gi = gitignore.CompileIgnoreLines(lines...)
	}
	return &Matcher{repoRoot: repoRoot, gi: gi}, nil
}

func readPatternLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}

// ShouldSkip reports whether relPath (relative to repo root, slash-separated) should be ignored.
// isDir should match the entry being tested so directory-only patterns behave like git.
func (m *Matcher) ShouldSkip(relPath string, isDir bool) bool {
	if m == nil || m.gi == nil {
		return false
	}
	rel := filepath.ToSlash(filepath.Clean(relPath))
	if rel == "." || rel == "" {
		return false
	}
	if m.gi.MatchesPath(rel) {
		return true
	}
	if isDir && !strings.HasSuffix(rel, "/") && m.gi.MatchesPath(rel+"/") {
		return true
	}
	return false
}

// RelFromAbs returns the slash-separated path relative to the matcher's repo root, or "" if outside.
func (m *Matcher) RelFromAbs(absPath string) (string, bool) {
	if m == nil {
		return "", false
	}
	rel, err := filepath.Rel(m.repoRoot, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return filepath.ToSlash(rel), true
}
