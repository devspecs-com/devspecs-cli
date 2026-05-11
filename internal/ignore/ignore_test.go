package ignore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

func TestMatcher_gitignore_ignores_directory(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "ignored", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	m, err := ignore.NewMatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !m.ShouldSkip("ignored", true) {
		t.Fatal("expected ignored dir skipped")
	}
	if !m.ShouldSkip("ignored/nested", true) {
		t.Fatal("expected nested under ignored skipped")
	}
}

func TestMatcher_aiignore_distinct_path(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".aiignore"), []byte("vendor-ai/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "vendor-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := ignore.NewMatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !m.ShouldSkip("vendor-ai", true) {
		t.Fatal("expected vendor-ai skipped by .aiignore")
	}
}

func TestMatcher_git_info_exclude(t *testing.T) {
	tmp := t.TempDir()
	gitInfo := filepath.Join(tmp, ".git", "info")
	if err := os.MkdirAll(gitInfo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitInfo, "exclude"), []byte("scratch/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "scratch"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := ignore.NewMatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !m.ShouldSkip("scratch", true) {
		t.Fatal("expected scratch skipped via .git/info/exclude")
	}
}

func TestMatcher_priority_gitignore_before_aiignore(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("build/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".aiignore"), []byte("local/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := ignore.NewMatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"build", "local"} {
		if !m.ShouldSkip(rel, true) {
			t.Fatalf("expected %q skipped", rel)
		}
	}
}

func TestMatcher_negation_unignore(t *testing.T) {
	tmp := t.TempDir()
	// ignore all under out/ except keep.md — library supports negation
	if err := os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("out/*\n!out/keep.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := ignore.NewMatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !m.ShouldSkip("out/lost.md", false) {
		t.Fatal("expected out/lost.md ignored")
	}
	if m.ShouldSkip("out/keep.md", false) {
		t.Fatal("expected out/keep.md not ignored via negation")
	}
}
