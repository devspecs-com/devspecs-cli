package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeDir_Default(t *testing.T) {
	t.Setenv("DEVSPECS_HOME", "")
	dir, err := HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".devspecs")
	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}
}

func TestHomeDir_EnvOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", tmp)
	dir, err := HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != tmp {
		t.Errorf("expected %q, got %q", tmp, dir)
	}
}

func TestDBPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", tmp)
	p, err := DBPath()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(tmp, "devspecs.db")
	if p != expected {
		t.Errorf("expected %q, got %q", expected, p)
	}
}

func TestLoadRepoConfig_Missing(t *testing.T) {
	tmp := t.TempDir()
	cfg, err := LoadRepoConfig(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Error("expected nil config for missing file")
	}
}

func TestLoadRepoConfig_Valid(t *testing.T) {
	tmp := t.TempDir()
	cfg := DefaultRepoConfig()
	if err := WriteRepoConfig(tmp, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadRepoConfig(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil config")
	}
	if loaded.Version != 1 {
		t.Errorf("expected version 1, got %d", loaded.Version)
	}
	if len(loaded.Sources) != 3 {
		t.Errorf("expected 3 sources, got %d", len(loaded.Sources))
	}
}

func TestLoadRepoConfig_Invalid(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".devspecs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(":::invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadRepoConfig(tmp)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestWriteRepoConfig_CreatesDir(t *testing.T) {
	tmp := t.TempDir()
	cfg := DefaultRepoConfig()
	if err := WriteRepoConfig(tmp, cfg); err != nil {
		t.Fatal(err)
	}
	path := RepoConfigPath(tmp)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestRepoConfigPath(t *testing.T) {
	got := RepoConfigPath("/my/repo")
	want := filepath.Join("/my/repo", ".devspecs", "config.yaml")
	if got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestDefaultRepoConfig_Structure(t *testing.T) {
	cfg := DefaultRepoConfig()
	if cfg.Version != 1 {
		t.Errorf("version: want 1, got %d", cfg.Version)
	}
	if len(cfg.Sources) != 3 {
		t.Fatalf("sources: want 3, got %d", len(cfg.Sources))
	}
	types := map[string]bool{}
	for _, s := range cfg.Sources {
		types[s.Type] = true
	}
	for _, want := range []string{"openspec", "adr", "markdown"} {
		if !types[want] {
			t.Errorf("missing source type %q", want)
		}
	}
}

func TestDBPath_WithEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", tmp)
	p, err := DBPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "devspecs.db" {
		t.Errorf("expected devspecs.db, got %q", filepath.Base(p))
	}
	if filepath.Dir(p) != tmp {
		t.Errorf("expected dir %q, got %q", tmp, filepath.Dir(p))
	}
}

func TestWriteRepoConfig_OverwriteExisting(t *testing.T) {
	tmp := t.TempDir()
	cfg1 := &RepoConfig{Version: 1, Sources: []SourceConfig{{Type: "a", Path: "x"}}}
	if err := WriteRepoConfig(tmp, cfg1); err != nil {
		t.Fatal(err)
	}
	cfg2 := &RepoConfig{Version: 1, Sources: []SourceConfig{{Type: "b", Path: "y"}}}
	if err := WriteRepoConfig(tmp, cfg2); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRepoConfig(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Sources[0].Type != "b" {
		t.Errorf("overwrite failed: got type %q", loaded.Sources[0].Type)
	}
}

func TestLoadRepoConfig_EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".devspecs")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(""), 0o644)

	cfg, err := LoadRepoConfig(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil && cfg.Version != 0 {
		t.Errorf("expected zero-value or nil, got %+v", cfg)
	}
}

func TestWriteRepoConfig_InvalidPath(t *testing.T) {
	// NUL is invalid in paths on every OS
	err := WriteRepoConfig(string([]byte{0}), DefaultRepoConfig())
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestWriteRepoConfig_Roundtrip(t *testing.T) {
	tmp := t.TempDir()
	cfg := &RepoConfig{
		Version: 1,
		Sources: []SourceConfig{
			{Type: "custom", Path: "my/path"},
			{Type: "multi", Paths: []string{"a", "b"}},
		},
	}
	if err := WriteRepoConfig(tmp, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRepoConfig(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Sources) != 2 {
		t.Fatalf("sources: want 2, got %d", len(loaded.Sources))
	}
	if loaded.Sources[0].Path != "my/path" {
		t.Errorf("path: want 'my/path', got %q", loaded.Sources[0].Path)
	}
	if len(loaded.Sources[1].Paths) != 2 {
		t.Errorf("paths: want 2, got %d", len(loaded.Sources[1].Paths))
	}
}

func TestLoadRepoConfig_InvalidMarkdownRuleKind(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".devspecs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `version: 1
sources:
  - type: markdown
    rules:
      - match: "*.md"
        kind: not_a_kind
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRepoConfig(tmp); err == nil {
		t.Fatal("expected validation error for invalid kind in rules")
	}
}
