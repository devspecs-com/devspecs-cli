package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_CreatesGlobalDB(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(repoDir)

	cmd := NewInitCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("global DB not created: %v", err)
	}
}

func TestInit_CreatesRepoConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(repoDir)

	cmd := NewInitCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(repoDir, ".devspecs", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("repo config not created: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Initialized DevSpecs.") {
		t.Errorf("expected 'Initialized DevSpecs.' in output, got %q", output)
	}
}

func TestInit_NoDestructiveRerun(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(repoDir)

	// First init
	cmd1 := NewInitCmd()
	cmd1.SetOut(&bytes.Buffer{})
	if err := cmd1.Execute(); err != nil {
		t.Fatal(err)
	}

	// Write a marker into config to detect overwrite
	configPath := filepath.Join(repoDir, ".devspecs", "config.yaml")
	marker := []byte("# marker\nversion: 1\nsources: []\n")
	if err := os.WriteFile(configPath, marker, 0o644); err != nil {
		t.Fatal(err)
	}

	// Second init without --force
	cmd2 := NewInitCmd()
	buf := &bytes.Buffer{}
	cmd2.SetOut(buf)
	if err := cmd2.Execute(); err != nil {
		t.Fatal(err)
	}

	// Config should NOT be overwritten
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# marker") {
		t.Error("config was overwritten without --force")
	}

	output := buf.String()
	if !strings.Contains(output, "already initialized") {
		t.Errorf("expected 'already initialized' message, got %q", output)
	}
}
