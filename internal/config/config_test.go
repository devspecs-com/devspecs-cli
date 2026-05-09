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
