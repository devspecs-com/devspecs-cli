package discover_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/discover"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/stretchr/testify/require"
)

func TestRun_gitignore_hides_candidate(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".gitignore"), []byte(".cursor/\n"), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".cursor", "plans"), 0o755))

	m, err := ignore.NewMatcher(tmp)
	require.NoError(t, err)

	res := discover.Run(tmp, m)
	for _, p := range res.MergeMarkdown {
		require.NotEqual(t, ".cursor/plans", p,
			"did not expect .cursor/plans when .cursor is ignored, got %#v", res.MergeMarkdown)

	}
}

func TestRun_sparse_docs_not_merged(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "docs"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "docs", "README.md"), []byte("# hi\n"), 0o644))

	res := discover.Run(tmp, nil)
	for _, p := range res.MergeMarkdown {
		require.NotEqual(t, "docs", p,
			"sparse docs/ should not merge bare docs, got %#v", res.MergeMarkdown)

	}
}

func TestRun_sparse_docs_emitsSuggestion(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "docs"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "docs", "README.md"), []byte("# hi\n"), 0o644))

	res := discover.Run(tmp, nil)
	var got string
	for _, s := range res.Suggestions {
		if strings.Contains(strings.ToLower(s), "sparse") || strings.Contains(s, "docs/") {
			got = s
			break
		}
	}
	require.NotEqual(t, "", got,
		"expected a sparse docs suggestion line, got %#v", res.Suggestions)

}

func TestRun_docs_dense_merges(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "docs", "a"), 0o755))

	for _, name := range []string{"docs/one.spec.md", "docs/two.plan.md"} {
		if err := os.WriteFile(filepath.Join(tmp, filepath.FromSlash(name)), []byte("# x\n"), 0o644); err != nil {
			require.NoError(t, err)
		}
	}
	res := discover.Run(tmp, nil)
	found := false
	for _, p := range res.MergeMarkdown {
		if p == "docs" {
			found = true
		}
	}
	require.True(t, found,
		"expected docs in merge list, got %#v", res.MergeMarkdown)

}
