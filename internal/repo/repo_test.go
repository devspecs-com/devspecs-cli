package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
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

func TestFileFirstCommitDate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()
	if err := exec.Command("git", "init", "-b", "main", tmp).Run(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(tmp, "doc.md")
	if err := os.WriteFile(p, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", tmp, "add", "doc.md")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	want := "2020-03-15T14:30:00Z"
	commit := exec.Command("git", "-C", tmp, "-c", "user.name=t", "-c", "user.email=t@t", "commit",
		"-m", "add doc", "--date", want)
	commit.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+want,
		"GIT_COMMITTER_DATE="+want,
	)
	if err := commit.Run(); err != nil {
		t.Fatal(err)
	}
	got := FileFirstCommitDate(tmp, "doc.md")
	if got == "" {
		t.Fatal("expected non-empty date")
	}
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("parse %q: %v", got, err)
	}
	wantT, _ := time.Parse(time.RFC3339, want)
	if !parsed.Equal(wantT) {
		t.Fatalf("want %v, got %v", wantT, parsed)
	}
}

func TestHeadCommit_gitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()
	if err := exec.Command("git", "init", "-b", "main", tmp).Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", tmp, "add", "f.txt").Run(); err != nil {
		t.Fatal(err)
	}
	c := exec.Command("git", "-C", tmp, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-m", "init")
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	h := HeadCommit(tmp)
	if len(h) < 8 {
		t.Fatalf("short hash %q", h)
	}
}

func TestHeadCommit_nonGit(t *testing.T) {
	if HeadCommit(t.TempDir()) != "" {
		t.Fatal("expected empty")
	}
}

func TestChangedFiles_latestCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()
	if err := exec.Command("git", "init", "-b", "main", tmp).Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", tmp, "add", "a.txt").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", tmp, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-m", "a").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", tmp, "add", "b.txt").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", tmp, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-m", "b").Run(); err != nil {
		t.Fatal(err)
	}
	files := ChangedFiles(tmp)
	found := false
	for _, f := range files {
		if f == "b.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%v", files)
	}
}

func TestChangedFiles_nonGit(t *testing.T) {
	if ChangedFiles(t.TempDir()) != nil {
		t.Fatal()
	}
}

func TestFileFirstCommitDate_emptyPath(t *testing.T) {
	if FileFirstCommitDate(t.TempDir(), "") != "" {
		t.Fatal()
	}
}
