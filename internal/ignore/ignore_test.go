package ignore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/stretchr/testify/require"
)

func TestMatcher_gitignore_ignores_directory(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("ignored/\n"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "ignored", "nested"), 0o755))

	m, err := ignore.NewMatcher(tmp)
	require.NoError(t, err)

	require.True(t, m.ShouldSkip("ignored", true),
		"expected ignored dir skipped")
	require.True(t, m.ShouldSkip("ignored/nested", true),
		"expected nested under ignored skipped")

}

func TestMatcher_aiignore_distinct_path(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".aiignore"), []byte("vendor-ai/\n"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "vendor-ai"), 0o755))

	m, err := ignore.NewMatcher(tmp)
	require.NoError(t, err)

	require.True(t, m.ShouldSkip("vendor-ai", true),
		"expected vendor-ai skipped by .aiignore")

}

func TestMatcher_git_info_exclude(t *testing.T) {
	tmp := t.TempDir()
	gitInfo := filepath.Join(tmp, ".git", "info")

	require.NoError(t, os.MkdirAll(gitInfo, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(gitInfo, "exclude"), []byte("scratch/\n"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "scratch"), 0o755))

	m, err := ignore.NewMatcher(tmp)
	require.NoError(t, err)

	require.True(t, m.ShouldSkip("scratch", true),
		"expected scratch skipped via .git/info/exclude")

}

func TestMatcher_priority_gitignore_before_aiignore(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("build/\n"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".aiignore"), []byte("local/\n"), 0o644))

	m, err := ignore.NewMatcher(tmp)
	require.NoError(t, err)

	for _, rel := range []string{"build", "local"} {
		require.True(t, m.ShouldSkip(rel, true),
			"expected %q skipped", rel)

	}
}

func TestMatcher_negation_unignore(t *testing.T) {
	tmp := t.TempDir()

	// ignore all under out/ except keep.md — library supports negation

	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte("out/*\n!out/keep.md\n"), 0o644))

	m, err := ignore.NewMatcher(tmp)
	require.NoError(t, err)

	require.True(t, m.ShouldSkip("out/lost.md", false),
		"expected out/lost.md ignored")
	require.False(t, m.ShouldSkip("out/keep.md", false),
		"expected out/keep.md not ignored via negation")

}
