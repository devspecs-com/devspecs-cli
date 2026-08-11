package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.True(t, findPackHasPath(got, "packages/vite/src/node/config.ts"),
		"expected same-stem config source recovery, got %#v", retrieval.CandidatePaths(got))
	assert.Equal(t, "source_manifest_family_recovery", got[0].Metadata["retrieval_expansion_reason"],
		"missing recovery metadata: %#v", got[0].Metadata)

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
	assert.True(t, findPackHasPath(got, "httpie/ssl_.py"),
		"expected TLS/SSL alias recovery, got %#v", retrieval.CandidatePaths(got))

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
	assert.False(t, findPackHasPath(got, "docs_src/custom_docs_ui/tutorial001.py"),
		"did not expect weak tutorial recovery, got %#v", retrieval.CandidatePaths(got))

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
	assert.True(t, findPackHasPath(got, "packages/vite/src/node/config.ts"),
		"expected filesystem source recovery, got %#v", retrieval.CandidatePaths(got))
	assert.Equal(t, "filesystem_source_family_recovery", got[0].Metadata["retrieval_expansion_reason"],
		"missing filesystem recovery metadata: %#v", got[0].Metadata)

}

func TestApplyFindSourceManifestConsumptionV2PreservesDefaultSourceWinner(t *testing.T) {
	baseline := []retrieval.Candidate{{
		Path:  "apps/web/lib/api/links/get-links-for-workspace.ts",
		Kind:  "source_context",
		Title: "get links for workspace",
		Body:  "workspace links sorting efficiency",
	}}
	matches := []retrieval.Candidate{{
		Path:  "apps/web/lib/export/links.ts",
		Kind:  "source_context",
		Title: "export links",
		Body:  "workspace links export",
	}}

	got := applyFindSourceManifestConsumptionV2Scout(nil, store.FilterParams{}, "improve link sort efficiency for workspace links", baseline, matches, nil)
	assert.True(t, findPackHasPath(got, "apps/web/lib/api/links/get-links-for-workspace.ts"),
		"expected loss-safe default source preservation, got %#v", retrieval.CandidatePaths(got))

	for _, candidate := range got {
		if candidate.Path != "apps/web/lib/api/links/get-links-for-workspace.ts" {
			continue
		}
		assert.Equal(t, "source_manifest_loss_safe_preserved", candidate.Metadata["retrieval_expansion_reason"],
			"missing loss-safe metadata: %#v", candidate.Metadata)
		assert.Equal(t, retrieval.PackTierPrimary, candidate.Metadata["pack_tier"],
			"expected primary preservation tier: %#v", candidate.Metadata)

		return
	}
}

func TestApplyFindSourceManifestConsumptionV2SkipsWeakTutorialPreserve(t *testing.T) {
	baseline := []retrieval.Candidate{{
		Path:  "docs_src/custom_docs_ui/tutorial001.py",
		Kind:  "source_context",
		Title: "tutorial001",
		Body:  "swagger oauth2 redirect",
	}}
	matches := []retrieval.Candidate{{
		Path:  "fastapi/openapi/docs.py",
		Kind:  "source_context",
		Title: "openapi docs",
		Body:  "swagger oauth2 redirect",
	}}

	got := applyFindSourceManifestConsumptionV2Scout(nil, store.FilterParams{}, "serve Swagger UI OAuth2 redirect from a custom docs redirect URL", baseline, matches, nil)
	assert.False(t, findPackHasPath(got, "docs_src/custom_docs_ui/tutorial001.py"),
		"did not expect weak tutorial preservation, got %#v", retrieval.CandidatePaths(got))

}

func writeRecoveryTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	{
		err := os.MkdirAll(filepath.Dir(path), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(path, []byte(body), 0o644)
		require.NoError(t, err)
	}

}
