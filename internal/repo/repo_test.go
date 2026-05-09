package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDetect_NonGitDir(t *testing.T) {
	tmp := t.TempDir()
	info := Detect(tmp)
	if info.IsGit {
		t.Error("expected IsGit=false for non-git dir")
	}
	if info.RootPath != tmp {
		t.Errorf("expected RootPath=%q, got %q", tmp, info.RootPath)
	}
}

func TestDetect_GitDir(t *testing.T) {
	tmp := t.TempDir()

	cmd := exec.Command("git", "init", "-b", "testbranch", tmp)
	if err := cmd.Run(); err != nil {
		t.Skip("git not available:", err)
	}

	// Create a commit so HEAD exists
	f, _ := os.Create(filepath.Join(tmp, "file.txt"))
	f.Close()
	exec.Command("git", "-C", tmp, "add", ".").Run()
	exec.Command("git", "-C", tmp, "commit", "-m", "init", "--allow-empty").Run()

	info := Detect(tmp)
	if !info.IsGit {
		t.Error("expected IsGit=true for git dir")
	}
	if info.CurrentBranch != "testbranch" {
		t.Errorf("expected branch 'testbranch', got %q", info.CurrentBranch)
	}
}
