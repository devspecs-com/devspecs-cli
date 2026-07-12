package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func gitCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Env = cleanGitTestEnv()
	return cmd
}

func cleanGitTestEnv() []string {
	blocked := map[string]bool{
		"GIT_DIR":                          true,
		"GIT_WORK_TREE":                    true,
		"GIT_INDEX_FILE":                   true,
		"GIT_PREFIX":                       true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	}
	var env []string
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok && blocked[key] {
			continue
		}
		env = append(env, entry)
	}
	return env
}

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

	cmd := gitCmd("init", "-b", "testbranch", tmp)
	if err := cmd.Run(); err != nil {
		t.Skip("git not available:", err)
	}

	// Create a commit so HEAD exists
	f, _ := os.Create(filepath.Join(tmp, "file.txt"))
	f.Close()
	gitCmd("-C", tmp, "add", ".").Run()
	gitCmd("-C", tmp, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "init", "--allow-empty").Run()

	info := Detect(tmp)
	if !info.IsGit {
		t.Error("expected IsGit=true for git dir")
	}
	if info.CurrentBranch != "testbranch" {
		t.Errorf("expected branch 'testbranch', got %q", info.CurrentBranch)
	}
}

func TestDetect_GitFileRoot(t *testing.T) {
	tmp := t.TempDir()
	worktree := filepath.Join(tmp, "worktree")
	subdir := filepath.Join(worktree, "nested")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitDir := filepath.Join(tmp, "repo", ".git", "worktrees", "worktree")
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	info := Detect(subdir)
	if !info.IsGit {
		t.Error("expected IsGit=true for .git file worktree")
	}
	if info.RootPath != worktree {
		t.Fatalf("expected RootPath=%q, got %q", worktree, info.RootPath)
	}
}

func TestDetect_GitWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()
	mainRepo := filepath.Join(tmp, "main")
	worktree := filepath.Join(tmp, "linked")

	if err := gitCmd("init", "-b", "main", mainRepo).Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mainRepo, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gitCmd("-C", mainRepo, "add", "file.txt").Run(); err != nil {
		t.Fatal(err)
	}
	if err := gitCmd("-C", mainRepo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "init").Run(); err != nil {
		t.Fatal(err)
	}
	if err := gitCmd("-C", mainRepo, "worktree", "add", "-b", "worktree-branch", worktree).Run(); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(worktree, "nested")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	info := Detect(subdir)
	if !info.IsGit {
		t.Fatal("expected IsGit=true for git worktree")
	}
	if info.RootPath != worktree {
		t.Fatalf("expected RootPath=%q, got %q", worktree, info.RootPath)
	}
	if info.CurrentBranch != "worktree-branch" {
		t.Fatalf("expected worktree branch, got %q", info.CurrentBranch)
	}
}

func TestFileFirstCommitDate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()
	if err := gitCmd("init", "-b", "main", tmp).Run(); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(tmp, "doc.md")
	if err := os.WriteFile(p, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := gitCmd("-C", tmp, "add", "doc.md")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	want := "2020-03-15T14:30:00Z"
	commit := gitCmd("-C", tmp, "-c", "user.name=t", "-c", "user.email=t@t", "commit",
		"-m", "add doc", "--date", want)
	commit.Env = append(commit.Env,
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

func TestFileFirstCommitDates_matchesSinglePathAndFollowsRenames(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()
	if err := gitCmd("init", "-b", "main", tmp).Run(); err != nil {
		t.Fatal(err)
	}
	oldDate := "2020-01-02T03:04:05Z"
	if err := os.WriteFile(filepath.Join(tmp, "old.md"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gitCmd("-C", tmp, "add", "old.md").Run(); err != nil {
		t.Fatal(err)
	}
	commitGitTest(t, tmp, "add old", oldDate)

	plainDate := "2020-02-03T04:05:06Z"
	if err := os.WriteFile(filepath.Join(tmp, "plain.md"), []byte("plain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gitCmd("-C", tmp, "add", "plain.md").Run(); err != nil {
		t.Fatal(err)
	}
	commitGitTest(t, tmp, "add plain", plainDate)

	renameDate := "2021-03-04T05:06:07Z"
	if err := gitCmd("-C", tmp, "mv", "old.md", "new.md").Run(); err != nil {
		t.Fatal(err)
	}
	commitGitTest(t, tmp, "rename old to new", renameDate)

	got := FileFirstCommitDates(tmp, []string{"new.md", "plain.md", "missing.md", ""})
	assertSameGitDate(t, got["new.md"], FileFirstCommitDate(tmp, "new.md"))
	assertSameGitDate(t, got["plain.md"], FileFirstCommitDate(tmp, "plain.md"))
	if _, ok := got["missing.md"]; ok {
		t.Fatalf("missing path unexpectedly resolved: %#v", got)
	}
	assertSameGitDate(t, got["new.md"], oldDate)
	assertSameGitDate(t, got["plain.md"], plainDate)
}

func TestHeadCommit_gitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()
	if err := gitCmd("init", "-b", "main", tmp).Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gitCmd("-C", tmp, "add", "f.txt").Run(); err != nil {
		t.Fatal(err)
	}
	c := gitCmd("-C", tmp, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-m", "init")
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
	if err := gitCmd("init", "-b", "main", tmp).Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gitCmd("-C", tmp, "add", "a.txt").Run(); err != nil {
		t.Fatal(err)
	}
	if err := gitCmd("-C", tmp, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-m", "a").Run(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gitCmd("-C", tmp, "add", "b.txt").Run(); err != nil {
		t.Fatal(err)
	}
	if err := gitCmd("-C", tmp, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-m", "b").Run(); err != nil {
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

func commitGitTest(t *testing.T, repoRoot, message, date string) {
	t.Helper()
	commit := gitCmd("-C", repoRoot, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", message, "--date", date)
	commit.Env = append(commit.Env,
		"GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_DATE="+date,
	)
	if err := commit.Run(); err != nil {
		t.Fatal(err)
	}
}

func assertSameGitDate(t *testing.T, got, want string) {
	t.Helper()
	gotT, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("parse got %q: %v", got, err)
	}
	wantT, err := time.Parse(time.RFC3339, want)
	if err != nil {
		t.Fatalf("parse want %q: %v", want, err)
	}
	if !gotT.Equal(wantT) {
		t.Fatalf("date mismatch: got %s want %s", got, want)
	}
}
