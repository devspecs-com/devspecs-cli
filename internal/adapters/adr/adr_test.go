package adr

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

func TestDiscover(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "docs", "adr")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "0001-use-sqlite.md"), []byte("# Use SQLite\n\nStatus: Accepted\n"), 0o644)

	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
}

func TestDiscover_ConfigPaths(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "architecture", "decisions")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "001.md"), []byte("# Test\n"), 0o644)

	cfg := &config.RepoConfig{Sources: []config.SourceConfig{{Type: "adr", Paths: []string{"architecture/decisions"}}}}
	a := &Adapter{}
	candidates, err := a.Discover(context.Background(), tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
}

func TestDiscover_IgnoredADRDir(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("secret-adrs/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(tmp, "adr")
	os.MkdirAll(good, 0o755)
	os.WriteFile(filepath.Join(good, "0001.md"), []byte("# A\n"), 0o644)
	bad := filepath.Join(tmp, "secret-adrs")
	os.MkdirAll(bad, 0o755)
	os.WriteFile(filepath.Join(bad, "0002.md"), []byte("# B\n"), 0o644)

	m, err := ignore.NewMatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	ctx := ignore.WithContext(context.Background(), m)
	a := &Adapter{}
	cands, err := a.Discover(ctx, tmp, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || cands[0].RelPath != "adr/0001.md" {
		t.Fatalf("want 1 adr under adr/, got %#v", cands)
	}
}

func TestADR_TitleStatusExtraction(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantTitle  string
		wantStatus string
	}{
		{
			name:       "status line format",
			content:    "# Use SQLite for storage\n\nStatus: Accepted\n\n## Context\nWe need local storage.\n",
			wantTitle:  "Use SQLite for storage",
			wantStatus: "accepted",
		},
		{
			name:       "status heading format",
			content:    "# Use Go for CLI\n\n## Status\n\nProposed\n\n## Context\n",
			wantTitle:  "Use Go for CLI",
			wantStatus: "proposed",
		},
		{
			name:       "frontmatter format",
			content:    "---\ntitle: Auth with JWT\nstatus: accepted\n---\n\n# Different Title\n",
			wantTitle:  "Auth with JWT",
			wantStatus: "accepted",
		},
		{
			name:       "MADR format with Date and Status lines",
			content:    "# Use Markdown Architectural Decision Records\n\n* Status: proposed\n* Deciders: team\n* Date: 2023-01-01\n",
			wantTitle:  "Use Markdown Architectural Decision Records",
			wantStatus: "proposed",
		},
		{
			name:       "no status defaults to unknown",
			content:    "# Simple ADR\n\nSome content without status.\n",
			wantTitle:  "Simple ADR",
			wantStatus: "unknown",
		},
		{
			name:       "superseded status",
			content:    "# Old Decision\n\nStatus: Superseded by ADR-0005\n",
			wantTitle:  "Old Decision",
			wantStatus: "superseded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			path := filepath.Join(tmp, "test.md")
			os.WriteFile(path, []byte(tt.content), 0o644)

			a := &Adapter{}
			art, _, _, err := a.Parse(context.Background(), adapters.Candidate{
				PrimaryPath: path,
				RelPath:     "docs/adr/test.md",
				AdapterName: "adr",
			})
			if err != nil {
				t.Fatal(err)
			}
			if art.Title != tt.wantTitle {
				t.Errorf("title: want %q, got %q", tt.wantTitle, art.Title)
			}
			if art.Status != tt.wantStatus {
				t.Errorf("status: want %q, got %q", tt.wantStatus, art.Status)
			}
			if art.Kind != "decision" || art.Subtype != "adr" {
				t.Errorf("kind/subtype: want decision/adr, got %q/%q", art.Kind, art.Subtype)
			}
			if art.FormatProfile != format.ProfileADR {
				t.Errorf("format_profile: want %q, got %q", format.ProfileADR, art.FormatProfile)
			}
		})
	}
}
