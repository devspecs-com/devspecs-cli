package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.False(t, info.IsGit,
		"expected IsGit=false for non-git dir")
	assert.Equal(t, tmp, info.RootPath,
		"expected RootPath=%q, got %q", tmp, info.RootPath)

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
	assert.True(t, info.IsGit,
		"expected IsGit=true for git dir")
	assert.Equal(t, "testbranch", info.CurrentBranch,
		"expected branch 'testbranch', got %q", info.CurrentBranch)

}

func TestDetect_GitFileRoot(t *testing.T) {
	tmp := t.TempDir()
	worktree := filepath.Join(tmp, "worktree")
	subdir := filepath.Join(worktree, "nested")

	require.NoError(t, os.MkdirAll(subdir, 0o755))

	gitDir := filepath.Join(tmp, "repo", ".git", "worktrees", "worktree")

	require.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o644))

	info := Detect(subdir)
	assert.True(t, info.IsGit,
		"expected IsGit=true for .git file worktree")
	require.Equal(t, worktree, info.RootPath,
		"expected RootPath=%q, got %q", worktree, info.RootPath)

}

func TestDetect_GitWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()
	mainRepo := filepath.Join(tmp, "main")
	worktree := filepath.Join(tmp, "linked")

	require.NoError(t, gitCmd("init", "-b", "main", mainRepo).Run())

	require.NoError(t, os.WriteFile(filepath.Join(mainRepo, "file.txt"), []byte("x"), 0o644))

	require.NoError(t, gitCmd("-C", mainRepo, "add", "file.txt").Run())

	require.NoError(t, gitCmd("-C", mainRepo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "init").Run())

	require.NoError(t, gitCmd("-C", mainRepo, "worktree", "add", "-b", "worktree-branch", worktree).Run())

	require.NoError(t, gitCmd("-C", mainRepo, "remote", "add", "origin", "git@github.com:Acme/Example.git").Run())

	subdir := filepath.Join(worktree, "nested")

	require.NoError(t, os.MkdirAll(subdir, 0o755))

	info := Detect(subdir)
	require.True(t, info.IsGit,
		"expected IsGit=true for git worktree")
	require.Equal(t, worktree, info.RootPath,
		"expected RootPath=%q, got %q", worktree, info.RootPath)
	require.Equal(t, "worktree-branch", info.CurrentBranch,
		"expected worktree branch, got %q", info.CurrentBranch)

	mainIdentity := DetectIdentity(mainRepo)
	worktreeIdentity := DetectIdentity(worktree)
	assert.NotEmpty(t, mainIdentity.GitIdentity)
	assert.Equal(t, mainIdentity.GitIdentity, worktreeIdentity.GitIdentity)

}

func TestCanonicalRemoteURL_WithGitSSHRemote_ReturnsCanonicalIdentity(t *testing.T) {
	actual := CanonicalRemoteURL("git@github.com:Acme/Example.git")

	assert.Equal(t, "github.com/acme/example", actual)
}

func TestCanonicalRemoteURL_WithURLSSHRemote_ReturnsCanonicalIdentity(t *testing.T) {
	actual := CanonicalRemoteURL("ssh://git@github.com/Acme/Example.git")

	assert.Equal(t, "github.com/acme/example", actual)
}

func TestCanonicalRemoteURL_WithHTTPSRemote_ReturnsCanonicalIdentity(t *testing.T) {
	actual := CanonicalRemoteURL("https://github.com/acme/example.git")

	assert.Equal(t, "github.com/acme/example", actual)
}

func TestCanonicalRemoteURL_WithLocalPath_ReturnsEmptyIdentity(t *testing.T) {
	actual := CanonicalRemoteURL(`C:\repos\example`)

	assert.Empty(t, actual)
}

func TestStableGitIdentity_WithoutRemote_ReturnsEmptyIdentity(t *testing.T) {
	actual := StableGitIdentity("", "abc")

	assert.Empty(t, actual)
}

func TestStableGitIdentity_WithoutRootCommit_ReturnsEmptyIdentity(t *testing.T) {
	actual := StableGitIdentity("https://github.com/acme/example.git", "")

	assert.Empty(t, actual)
}

func TestStableGitIdentity_WithCanonicalRemoteAndRootCommit_ReturnsExpectedIdentity(t *testing.T) {
	actual := StableGitIdentity("git@github.com:Acme/Example.git", "abc")

	assert.Equal(t, "git_0d5d8b1d05ec3ba6c66a4cc05e1ea9aadfe692d716b5dcae32d2fb45abb04b33", actual)
}

func TestFileFirstCommitDate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()

	require.NoError(t, gitCmd("init", "-b", "main", tmp).Run())

	p := filepath.Join(tmp, "doc.md")

	require.NoError(t, os.WriteFile(p, []byte("v1\n"), 0o644))

	cmd := gitCmd("-C", tmp, "add", "doc.md")

	require.NoError(t, cmd.Run())

	want := "2020-03-15T14:30:00Z"
	commit := gitCmd("-C", tmp, "-c", "user.name=t", "-c", "user.email=t@t", "commit",
		"-m", "add doc", "--date", want)
	commit.Env = append(commit.Env,
		"GIT_AUTHOR_DATE="+want,
		"GIT_COMMITTER_DATE="+want,
	)

	require.NoError(t, commit.Run())

	got := FileFirstCommitDate(tmp, "doc.md")
	require.NotEqual(t, "", got,
		"expected non-empty date")

	parsed, err := time.Parse(time.RFC3339, got)
	require.NoError(t, err,
		"parse %q: %v", got, err)

	wantT, _ := time.Parse(time.RFC3339, want)
	require.True(t, parsed.Equal(wantT),
		"want %v, got %v", wantT, parsed)

}

func TestFileFirstCommitDates_matchesSinglePathAndFollowsRenames(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()

	require.NoError(t, gitCmd("init", "-b", "main", tmp).Run())

	oldDate := "2020-01-02T03:04:05Z"

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "old.md"), []byte("old\n"), 0o644))

	require.NoError(t, gitCmd("-C", tmp, "add", "old.md").Run())

	commitGitTest(t, tmp, "add old", oldDate)

	plainDate := "2020-02-03T04:05:06Z"

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "plain.md"), []byte("plain\n"), 0o644))

	require.NoError(t, gitCmd("-C", tmp, "add", "plain.md").Run())

	commitGitTest(t, tmp, "add plain", plainDate)

	renameDate := "2021-03-04T05:06:07Z"

	require.NoError(t, gitCmd("-C", tmp, "mv", "old.md", "new.md").Run())

	commitGitTest(t, tmp, "rename old to new", renameDate)

	got := FileFirstCommitDates(tmp, []string{"new.md", "plain.md", "missing.md", ""})

	_, ok := got["missing.md"]
	assert.False(t, ok)
	assertSameGitDate(t, got["new.md"], oldDate)
	assertSameGitDate(t, got["plain.md"], plainDate)
}

func TestHeadCommit_gitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()

	require.NoError(t, gitCmd("init", "-b", "main", tmp).Run())

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "f.txt"), []byte("x"), 0o644))

	require.NoError(t, gitCmd("-C", tmp, "add", "f.txt").Run())

	c := gitCmd("-C", tmp, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-m", "init")

	require.NoError(t, c.Run())

	h := HeadCommit(tmp)
	require.False(t, len(h) < 8,
		"short hash %q", h)

}

func TestHeadCommit_nonGit(t *testing.T) {
	require.Equal(t, "", HeadCommit(t.TempDir()),
		"expected empty")

}

func TestChangedFiles_latestCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available:", err)
	}
	tmp := t.TempDir()

	require.NoError(t, gitCmd("init", "-b", "main", tmp).Run())

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("a"), 0o644))

	require.NoError(t, gitCmd("-C", tmp, "add", "a.txt").Run())

	require.NoError(t, gitCmd("-C", tmp, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-m", "a").Run())

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("b"), 0o644))

	require.NoError(t, gitCmd("-C", tmp, "add", "b.txt").Run())

	require.NoError(t, gitCmd("-C", tmp, "-c", "user.name=a", "-c", "user.email=a@a", "commit", "-m", "b").Run())

	files := ChangedFiles(tmp)
	found := false
	for _, f := range files {
		if f == "b.txt" {
			found = true
			break
		}
	}
	require.True(t, found,
		"%v", files)

}

func TestChangedFiles_nonGit(t *testing.T) {
	require.Nil(t, ChangedFiles(t.TempDir()))

}

func TestFileFirstCommitDate_emptyPath(t *testing.T) {
	require.Equal(t, "", FileFirstCommitDate(t.TempDir(), ""))

}

func commitGitTest(t *testing.T, repoRoot, message, date string) {
	t.Helper()
	commit := gitCmd("-C", repoRoot, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", message, "--date", date)
	commit.Env = append(commit.Env,
		"GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_DATE="+date,
	)

	require.NoError(t, commit.Run())

}

func assertSameGitDate(t *testing.T, got, want string) {
	t.Helper()
	gotT, err := time.Parse(time.RFC3339, got)
	require.NoError(t, err,
		"parse got %q: %v", got, err)

	wantT, err := time.Parse(time.RFC3339, want)
	require.NoError(t, err,
		"parse want %q: %v", want, err)
	require.True(t, gotT.Equal(wantT),
		"date mismatch: got %s want %s", got, want)

}
