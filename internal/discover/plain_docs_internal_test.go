package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/stretchr/testify/require"
)

func TestPlainDocsWorthIndexing_RespectsMaxDirs(t *testing.T) {
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "docs", "sub")

	require.NoError(t, os.MkdirAll(sub, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(sub, "a.spec.md"), []byte("#\n"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(sub, "b.plan.md"), []byte("#\n"), 0o644))

	m, err := ignore.NewMatcher(tmp)
	require.NoError(t, err)

	out := &Result{}
	require.False(t, plainDocsWorthIndexing(tmp, m, out, 1, 400),
		"expected false when maxDirs stops walk before nested hits")

	out = &Result{}
	require.True(t, plainDocsWorthIndexing(tmp, m, out, 10, 400),
		"expected true when walk can reach two spec-like files")

}
