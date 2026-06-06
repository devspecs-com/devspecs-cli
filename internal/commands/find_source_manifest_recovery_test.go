package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

func TestSelectFindSourceManifestRecoveryCandidatesAddsSameStemSourceFromSelectedTest(t *testing.T) {
	selected := []retrieval.Candidate{{
		Path:    "packages/vite/src/node/__tests__/config.spec.ts",
		Kind:    "source_context",
		Subtype: "test_case",
		Title:   "cacheDir resolution > uses node_modules/.vite",
		Body:    "cacheDir node_modules vite",
	}}
	rows := []findSourceTestManifestRow{
		{FileID: "config", Path: "packages/vite/src/node/config.ts", Language: "typescript", SourceRole: "implementation", Symbols: "resolveConfig\ncacheDir"},
		{FileID: "utils", Path: "packages/vite/src/node/utils.ts", Language: "typescript", SourceRole: "implementation", Symbols: "normalizePath"},
	}

	got := selectFindSourceManifestRecoveryCandidates("Use node_modules vite cacheDir when node_modules exists", selected, nil, rows, 4)
	if !findPackHasPath(got, "packages/vite/src/node/config.ts") {
		t.Fatalf("expected same-stem config source recovery, got %#v", retrieval.CandidatePaths(got))
	}
	if got[0].Metadata["retrieval_expansion_reason"] != "source_manifest_family_recovery" {
		t.Fatalf("missing recovery metadata: %#v", got[0].Metadata)
	}
}

func TestSelectFindSourceManifestRecoveryCandidatesUsesTechnicalAliases(t *testing.T) {
	selected := []retrieval.Candidate{{
		Path:  "httpie/client.py",
		Kind:  "source_context",
		Title: "httpie/client.py",
		Body:  "client certificate key",
	}}
	rows := []findSourceTestManifestRow{
		{FileID: "ssl", Path: "httpie/ssl_.py", Language: "python", SourceRole: "implementation", Symbols: "load_ssl_context\nload_cert_chain"},
		{FileID: "downloads", Path: "httpie/downloads.py", Language: "python", SourceRole: "implementation", Symbols: "Downloader"},
	}

	got := selectFindSourceManifestRecoveryCandidates("load client TLS certificates that need a private key passphrase", selected, nil, rows, 4)
	if !findPackHasPath(got, "httpie/ssl_.py") {
		t.Fatalf("expected TLS/SSL alias recovery, got %#v", retrieval.CandidatePaths(got))
	}
}

func TestSelectFindSourceManifestRecoveryCandidatesSkipsWeakTutorialPath(t *testing.T) {
	selected := []retrieval.Candidate{{
		Path:  "fastapi/openapi/docs.py",
		Kind:  "source_context",
		Title: "fastapi/openapi/docs.py",
		Body:  "swagger oauth2 redirect",
	}}
	rows := []findSourceTestManifestRow{
		{FileID: "tutorial", Path: "docs_src/custom_docs_ui/tutorial001.py", Language: "python", SourceRole: "implementation", Symbols: "swagger_oauth2_redirect"},
	}

	got := selectFindSourceManifestRecoveryCandidates("serve Swagger UI OAuth2 redirect from a custom docs redirect URL", selected, nil, rows, 4)
	if findPackHasPath(got, "docs_src/custom_docs_ui/tutorial001.py") {
		t.Fatalf("did not expect weak tutorial recovery, got %#v", retrieval.CandidatePaths(got))
	}
}

func TestSelectFindFilesystemSourceRecoveryCandidatesAddsSourceFromSelectedTest(t *testing.T) {
	root := t.TempDir()
	writeRecoveryTestFile(t, root, "packages/vite/src/node/config.ts", "export const cacheDir = 'node_modules/.vite'\n")
	selected := []retrieval.Candidate{{
		Path:    "packages/vite/src/node/__tests__/config.spec.ts#L1549",
		Source:  "packages/vite/src/node/__tests__/config.spec.ts",
		Kind:    "source_context",
		Subtype: "test_case",
		Title:   "cacheDir resolution > uses node_modules/.vite",
	}}

	got := selectFindFilesystemSourceRecoveryCandidates(root, "Use node_modules vite cacheDir when node_modules exists", selected, 4)
	if !findPackHasPath(got, "packages/vite/src/node/config.ts") {
		t.Fatalf("expected filesystem source recovery, got %#v", retrieval.CandidatePaths(got))
	}
	if got[0].Metadata["retrieval_expansion_reason"] != "filesystem_source_family_recovery" {
		t.Fatalf("missing filesystem recovery metadata: %#v", got[0].Metadata)
	}
}

func writeRecoveryTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
