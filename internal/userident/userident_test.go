package userident

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetect_GitUser(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=TestGitUser",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=TestGitUser",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	run("config", "user.name", "TestGitUser")

	name := Detect(dir)
	if name != "TestGitUser" {
		t.Errorf("expected 'TestGitUser', got %q", name)
	}
}

func TestDetect_OSUser(t *testing.T) {
	dir := t.TempDir()
	name := Detect(dir)
	if name == "" {
		t.Error("Detect() returned empty string in non-git dir")
	}
}

func TestDetect_FallbackIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	dir := t.TempDir()

	name1 := generatedFallback()
	if name1 == "" {
		t.Fatal("fallback returned empty")
	}
	if len(name1) != 8 {
		t.Errorf("expected 8-char fallback, got %d: %q", len(name1), name1)
	}

	name2 := generatedFallback()
	if name1 != name2 {
		t.Errorf("fallback not idempotent: %q != %q", name1, name2)
	}

	idFile := filepath.Join(home, "identity")
	data, err := os.ReadFile(idFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != name1 {
		t.Errorf("identity file content mismatch: got %q, want %q", strings.TrimSpace(string(data)), name1)
	}

	_ = dir
}

func TestGitUserName_NoRepo(t *testing.T) {
	dir := t.TempDir()
	// In a non-git directory with no global git config, gitUserName returns "".
	// If a global git config exists, it may return the global user.name — that's expected.
	name := gitUserName(dir)
	_ = name // Just verify it doesn't panic
}
