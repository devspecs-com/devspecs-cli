package scan

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirstPartySourceContextAdmitsPackageSourceAndTests(t *testing.T) {
	root := t.TempDir()
	writeFirstPartySourceTestFile(t, root, "pyproject.toml", "[project]\nname = \"httpie\"\n")
	writeFirstPartySourceTestFile(t, root, "httpie/__init__.py", "")
	writeFirstPartySourceTestFile(t, root, "httpie/downloads.py", "class Downloader:\n    pass\n")
	writeFirstPartySourceTestFile(t, root, "httpie/ssl_.py", "def load_cert():\n    pass\n")
	writeFirstPartySourceTestFile(t, root, "httpie/manager/tasks/sessions.py", "def refresh_session():\n    pass\n")
	writeFirstPartySourceTestFile(t, root, "tests/test_ssl.py", "def test_password_protected_cert_cli_arg():\n    pass\n")
	writeFirstPartySourceTestFile(t, root, "tests/test_tutorial/test_docs.py", "def test_docs_examples():\n    pass\n")
	writeFirstPartySourceTestFile(t, root, "docs_src/tutorial001.py", "print('docs example')\n")
	writeFirstPartySourceTestFile(t, root, "node_modules/pkg/index.js", "export const ignored = true\n")
	writeFirstPartySourceTestFile(t, root, "httpie/generated.pb.go", "package httpie\n")

	got := buildFirstPartySourceContextCandidates(context.Background(), root, []adapters.Candidate{{RelPath: "httpie/__init__.py"}})
	paths := firstPartySourceCandidatePaths(got)
	require.Len(t, paths, 5)
	assert.True(t, paths["httpie/downloads.py"])
	assert.True(t, paths["httpie/ssl_.py"])
	assert.True(t, paths["httpie/manager/tasks/sessions.py"])
	assert.True(t, paths["tests/test_ssl.py"])
	assert.True(t, paths["tests/test_tutorial/test_docs.py"])
	assert.False(t, paths["httpie/__init__.py"])
	assert.False(t, paths["docs_src/tutorial001.py"])
	assert.False(t, paths["node_modules/pkg/index.js"])
	assert.False(t, paths["httpie/generated.pb.go"])

	byPath := map[string]adapters.Candidate{}
	for _, candidate := range got {
		byPath[candidate.RelPath] = candidate
		require.Equal(t, firstPartySourceAdmissionReason, candidate.Metadata["admission_reason"],
			"missing admission metadata on %#v", candidate)

	}
	assert.Equal(t, "implementation", byPath["httpie/downloads.py"].Metadata["source_role"])
	assert.Equal(t, "test", byPath["tests/test_ssl.py"].Metadata["source_role"])
	assert.Equal(t, "httpie", byPath["httpie/downloads.py"].Metadata["source_root"])
}

func TestFirstPartySourceContextAdmitsLongTailLanguageInFirstPartyRoot(t *testing.T) {
	root := t.TempDir()
	writeFirstPartySourceTestFile(t, root, "package.json", `{"name":"kong-plugin"}`)
	writeFirstPartySourceTestFile(t, root, "plugins/auth/access.lua", "local function rewrite_header()\nend\n")
	writeFirstPartySourceTestFile(t, root, "examples/auth/access.lua", "local function example()\nend\n")

	got := buildFirstPartySourceContextCandidates(context.Background(), root, nil)
	paths := firstPartySourceCandidatePaths(got)
	require.Len(t, paths, 1)
	assert.True(t, paths["plugins/auth/access.lua"])
	assert.False(t, paths["examples/auth/access.lua"])
}

func TestFirstPartySourceContextDetectsNestedModuleRoots(t *testing.T) {
	root := t.TempDir()
	writeFirstPartySourceTestFile(t, root, "sdk/storage/blob/go.mod", "module example.com/sdk/storage/blob\n")
	writeFirstPartySourceTestFile(t, root, "sdk/storage/blob/client.go", "package blob\n")
	writeFirstPartySourceTestFile(t, root, "sdk/storage/blob/client_test.go", "package blob\n")
	writeFirstPartySourceTestFile(t, root, "sdk/storage/queue/go.mod", "module example.com/sdk/storage/queue\n")
	writeFirstPartySourceTestFile(t, root, "sdk/storage/queue/client.go", "package queue\n")
	writeFirstPartySourceTestFile(t, root, "vendor/example.com/other/go.mod", "module example.com/other\n")
	writeFirstPartySourceTestFile(t, root, "vendor/example.com/other/ignored.go", "package ignored\n")

	got := buildFirstPartySourceContextCandidates(context.Background(), root, nil)
	paths := firstPartySourceCandidatePaths(got)
	require.Len(t, paths, 3)
	assert.True(t, paths["sdk/storage/blob/client.go"])
	assert.True(t, paths["sdk/storage/blob/client_test.go"])
	assert.True(t, paths["sdk/storage/queue/client.go"])
	assert.False(t, paths["vendor/example.com/other/ignored.go"])
	byPath := map[string]adapters.Candidate{}
	for _, candidate := range got {
		byPath[candidate.RelPath] = candidate
	}
	assert.Equal(t, "module_root", byPath["sdk/storage/blob/client.go"].Metadata["source_root_kind"])
}

func TestFirstPartySourceContextAdmitsTrackedSourceMatchingGitIgnore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	root := t.TempDir()
	writeFirstPartySourceTestFile(t, root, ".gitignore", "core.*\n")
	writeFirstPartySourceTestFile(t, root, "lib/core.sh", "#!/bin/sh\necho core\n")
	runGitCommand(t, root, "init")
	runGitCommand(t, root, "add", ".gitignore")
	runGitCommand(t, root, "add", "--force", "lib/core.sh")
	matcher, err := ignore.NewMatcher(root)
	require.NoError(t, err)
	ctx := ignore.WithContext(context.Background(), matcher)

	got := buildFirstPartySourceContextCandidates(ctx, root, nil)

	require.Len(t, got, 1)
	assert.Equal(t, "lib/core.sh", got[0].RelPath)
}

func firstPartySourceCandidatePaths(candidates []adapters.Candidate) map[string]bool {
	out := map[string]bool{}
	for _, candidate := range candidates {
		out[candidate.RelPath] = true
	}
	return out
}

func writeFirstPartySourceTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
