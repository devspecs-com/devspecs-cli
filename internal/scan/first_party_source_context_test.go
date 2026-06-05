package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
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
	for _, want := range []string{
		"httpie/downloads.py",
		"httpie/ssl_.py",
		"httpie/manager/tasks/sessions.py",
		"tests/test_ssl.py",
		"tests/test_tutorial/test_docs.py",
	} {
		if !paths[want] {
			t.Fatalf("missing %s in %#v", want, paths)
		}
	}
	for _, unexpected := range []string{
		"httpie/__init__.py",
		"docs_src/tutorial001.py",
		"node_modules/pkg/index.js",
		"httpie/generated.pb.go",
	} {
		if paths[unexpected] {
			t.Fatalf("unexpected %s in %#v", unexpected, paths)
		}
	}

	byPath := map[string]adapters.Candidate{}
	for _, candidate := range got {
		byPath[candidate.RelPath] = candidate
		if candidate.Metadata["admission_reason"] != firstPartySourceAdmissionReason {
			t.Fatalf("missing admission metadata on %#v", candidate)
		}
	}
	if byPath["httpie/downloads.py"].Metadata["source_role"] != "implementation" {
		t.Fatalf("implementation role metadata = %#v", byPath["httpie/downloads.py"].Metadata)
	}
	if byPath["tests/test_ssl.py"].Metadata["source_role"] != "test" {
		t.Fatalf("test role metadata = %#v", byPath["tests/test_ssl.py"].Metadata)
	}
	if byPath["httpie/downloads.py"].Metadata["source_root"] != "httpie" {
		t.Fatalf("source root metadata = %#v", byPath["httpie/downloads.py"].Metadata)
	}
}

func TestFirstPartySourceContextAdmitsLongTailLanguageInFirstPartyRoot(t *testing.T) {
	root := t.TempDir()
	writeFirstPartySourceTestFile(t, root, "package.json", `{"name":"kong-plugin"}`)
	writeFirstPartySourceTestFile(t, root, "plugins/auth/access.lua", "local function rewrite_header()\nend\n")
	writeFirstPartySourceTestFile(t, root, "examples/auth/access.lua", "local function example()\nend\n")

	got := buildFirstPartySourceContextCandidates(context.Background(), root, nil)
	paths := firstPartySourceCandidatePaths(got)
	if !paths["plugins/auth/access.lua"] {
		t.Fatalf("missing first-party lua source in %#v", paths)
	}
	if paths["examples/auth/access.lua"] {
		t.Fatalf("docs/examples lua should stay out: %#v", paths)
	}
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
	for _, want := range []string{
		"sdk/storage/blob/client.go",
		"sdk/storage/blob/client_test.go",
		"sdk/storage/queue/client.go",
	} {
		if !paths[want] {
			t.Fatalf("missing nested module source %s in %#v", want, paths)
		}
	}
	if paths["vendor/example.com/other/ignored.go"] {
		t.Fatalf("vendor module should stay out: %#v", paths)
	}
	for _, candidate := range got {
		if candidate.RelPath == "sdk/storage/blob/client.go" && candidate.Metadata["source_root_kind"] != "module_root" {
			t.Fatalf("nested module root metadata = %#v", candidate.Metadata)
		}
	}
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
