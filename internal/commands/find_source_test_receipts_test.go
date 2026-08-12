package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFindSourceTestReceiptsAddsBehaviorTestsWithoutPackMutation(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	now := "2026-06-05T00:00:00Z"
	{
		_, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", "repo_src", tmp, now, now)
		require.NoError(t, err)
	}
	{

		err := db.ReplaceRepoSourceManifest("repo_src",
			[]store.SourceManifestFileInput{
				{FileID: "src_webhook", RepoID: "repo_src", Path: "services/webhook/webhook.go", ContentHash: "a", Language: "go", SourceRoot: "services/webhook", SourceRootKind: "module_root", SourceRole: "implementation"},
				{FileID: "test_model", RepoID: "repo_src", Path: "models/webhook/webhook_test.go", ContentHash: "b", Language: "go", SourceRoot: "models/webhook", SourceRootKind: "module_root", SourceRole: "test"},
				{FileID: "test_integration", RepoID: "repo_src", Path: "tests/integration/repo_webhook_test.go", ContentHash: "c", Language: "go", SourceRoot: "tests", SourceRootKind: "test_root", SourceRole: "test"},
				{FileID: "helper_route", RepoID: "repo_src", Path: "apps/web/app/api/webhook/test/route.ts", ContentHash: "d", Language: "typescript", SourceRoot: "apps/web", SourceRootKind: "module_root", SourceRole: "test"},
				{FileID: "src_events", RepoID: "repo_src", Path: "modules/webhook/events.go", ContentHash: "e", Language: "go", SourceRoot: "modules/webhook", SourceRootKind: "module_root", SourceRole: "implementation"},
				{FileID: "src_noise", RepoID: "repo_src", Path: "modules/settings/settings.go", ContentHash: "f", Language: "go", SourceRoot: "modules/settings", SourceRootKind: "module_root", SourceRole: "implementation"},
			},
			[]store.SourceManifestSymbolInput{
				{FileID: "src_events", Symbol: "WebhookBranchFilterPushEvent", Kind: "func", Line: 10},
				{FileID: "src_noise", Symbol: "SettingsStore", Kind: "type", Line: 10},
			},
			[]store.SourceManifestTestInput{
				{FileID: "test_model", TestName: "TestWebhookBranchFilter", Line: 12},
				{FileID: "test_integration", TestName: "TestRepoWebhookPushEventBranchFilter", Line: 24},
			},
			nil,
			[]store.SourceManifestFTSInput{
				{FileID: "src_webhook", Path: "services/webhook/webhook.go", PathTerms: "services webhook webhook go", SourceRoot: "services/webhook", Language: "go", SourceRole: "implementation"},
				{FileID: "test_model", Path: "models/webhook/webhook_test.go", PathTerms: "models webhook webhook test go", SourceRoot: "models/webhook", Language: "go", SourceRole: "test", TestNames: "TestWebhookBranchFilter"},
				{FileID: "test_integration", Path: "tests/integration/repo_webhook_test.go", PathTerms: "tests integration repo webhook test go", SourceRoot: "tests", Language: "go", SourceRole: "test", TestNames: "TestRepoWebhookPushEventBranchFilter"},
				{FileID: "helper_route", Path: "apps/web/app/api/webhook/test/route.ts", PathTerms: "apps web app api webhook test route ts", SourceRoot: "apps/web", Language: "typescript", SourceRole: "test"},
				{FileID: "src_events", Path: "modules/webhook/events.go", PathTerms: "modules webhook events go", SourceRoot: "modules/webhook", Language: "go", SourceRole: "implementation", Symbols: "WebhookBranchFilterPushEvent"},
				{FileID: "src_noise", Path: "modules/settings/settings.go", PathTerms: "modules settings settings go", SourceRoot: "modules/settings", Language: "go", SourceRole: "implementation", Symbols: "SettingsStore"},
			},
			now,
		)
		require.NoError(t, err)
	}

	pack := retrieval.RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Groups: []retrieval.PackGroup{
			{
				Role: retrieval.PackRoleImplementation,
				Items: []retrieval.PackItem{
					{OriginalRank: 1, Path: "services/webhook/webhook.go", Kind: "source_context", Title: "services/webhook/webhook.go"},
				},
			},
		},
	}
	got, err := buildFindSourceTestReceipts(db, store.FilterParams{RepoRoot: tmp}, "update repo webhook branch filter matching for push events", pack, findSourceTestReceiptsModeReceiptV0)
	require.NoError(t, err)
	require.NotNil(t, got, "expected related test receipts, got %#v", got)
	require.NotEmpty(t, got.Items, "expected related test receipts, got %#v", got)
	assert.True(t, findRelatedTestHasPath(got, "models/webhook/webhook_test.go"),
		"expected webhook model test receipt, got %#v", got.Items)
	assert.True(t, findRelatedTestHasPath(got, "tests/integration/repo_webhook_test.go"),
		"expected repo webhook integration test receipt, got %#v", got.Items)
	assert.False(t, findRelatedTestHasPath(got, "apps/web/app/api/webhook/test/route.ts"),
		"did not expect test helper route receipt, got %#v", got.Items)

}

func TestBuildFindSourceTestReceiptsRelatedFilesAddsSourceReceipts(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	now := "2026-06-05T00:00:00Z"
	{
		_, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", "repo_src", tmp, now, now)
		require.NoError(t, err)
	}
	{

		err := db.ReplaceRepoSourceManifest("repo_src",
			[]store.SourceManifestFileInput{
				{FileID: "src_webhook", RepoID: "repo_src", Path: "services/webhook/webhook.go", ContentHash: "a", Language: "go", SourceRoot: "services/webhook", SourceRootKind: "module_root", SourceRole: "implementation"},
				{FileID: "src_events", RepoID: "repo_src", Path: "modules/webhook/events.go", ContentHash: "b", Language: "go", SourceRoot: "modules/webhook", SourceRootKind: "module_root", SourceRole: "implementation"},
				{FileID: "test_model", RepoID: "repo_src", Path: "models/webhook/webhook_test.go", ContentHash: "c", Language: "go", SourceRoot: "models/webhook", SourceRootKind: "module_root", SourceRole: "test"},
				{FileID: "src_noise", RepoID: "repo_src", Path: "modules/settings/settings.go", ContentHash: "d", Language: "go", SourceRoot: "modules/settings", SourceRootKind: "module_root", SourceRole: "implementation"},
			},
			[]store.SourceManifestSymbolInput{
				{FileID: "src_events", Symbol: "WebhookBranchFilterPushEvent", Kind: "func", Line: 10},
				{FileID: "src_noise", Symbol: "SettingsStore", Kind: "type", Line: 10},
			},
			[]store.SourceManifestTestInput{
				{FileID: "test_model", TestName: "TestWebhookBranchFilter", Line: 12},
			},
			nil,
			[]store.SourceManifestFTSInput{
				{FileID: "src_webhook", Path: "services/webhook/webhook.go", PathTerms: "services webhook webhook go", SourceRoot: "services/webhook", Language: "go", SourceRole: "implementation"},
				{FileID: "src_events", Path: "modules/webhook/events.go", PathTerms: "modules webhook events go", SourceRoot: "modules/webhook", Language: "go", SourceRole: "implementation", Symbols: "WebhookBranchFilterPushEvent"},
				{FileID: "test_model", Path: "models/webhook/webhook_test.go", PathTerms: "models webhook webhook test go", SourceRoot: "models/webhook", Language: "go", SourceRole: "test", TestNames: "TestWebhookBranchFilter"},
				{FileID: "src_noise", Path: "modules/settings/settings.go", PathTerms: "modules settings settings go", SourceRoot: "modules/settings", Language: "go", SourceRole: "implementation", Symbols: "SettingsStore"},
			},
			now,
		)
		require.NoError(t, err)
	}

	pack := retrieval.RoleGroupedPack{
		Mode: "role_grouped_pack_v0",
		Groups: []retrieval.PackGroup{
			{
				Role: retrieval.PackRoleImplementation,
				Items: []retrieval.PackItem{
					{OriginalRank: 1, Path: "services/webhook/webhook.go", Kind: "source_context", Title: "services/webhook/webhook.go"},
				},
			},
		},
	}
	got, err := buildFindSourceTestReceipts(db, store.FilterParams{RepoRoot: tmp}, "update repo webhook branch filter matching for push events", pack, findSourceTestReceiptsModeRelatedFilesReceiptV0)
	require.NoError(t, err)
	require.NotNil(t, got, "expected related-files receipt context, got %#v", got)
	assert.Equal(t, findSourceTestReceiptsModeRelatedFilesReceiptV0, got.Mode, "expected related-files receipt context, got %#v", got)
	assert.True(t, findRelatedTestHasPath(got, "models/webhook/webhook_test.go"),
		"expected existing test receipt, got %#v", got.Items)
	assert.True(t, findRelatedTestHasPath(got, "modules/webhook/events.go"),
		"expected source related-file receipt, got %#v", got.Items)
	assert.False(t, findRelatedTestHasPath(got, "modules/settings/settings.go"),
		"did not expect unrelated source receipt, got %#v", got.Items)

}

func TestWriteRelatedTestsText(t *testing.T) {
	var b strings.Builder
	writeRelatedTestsText(&b, &FindRelatedTestContext{
		Mode: findSourceTestReceiptsModeReceiptV0,
		Items: []FindRelatedTestReceipt{
			{Path: "internal/commands/map_test.go", Reasons: []string{"same_stem", "test_name_anchor:map"}},
		},
	}, false)
	out := b.String()
	assert.Contains(t, out, "Related tests from selected source context",
		"related test output missing %q:\n%s", "Related tests from selected source context", out)
	assert.Contains(t, out, "internal/commands/map_test.go",
		"related test output missing %q:\n%s", "internal/commands/map_test.go", out)
	assert.Contains(t, out, "same_stem",
		"related test output missing %q:\n%s", "same_stem", out)

}

func TestWriteRelatedFilesText(t *testing.T) {
	var b strings.Builder
	writeRelatedTestsText(&b, &FindRelatedTestContext{
		Mode: findSourceTestReceiptsModeRelatedFilesReceiptV0,
		Items: []FindRelatedTestReceipt{
			{Path: "modules/webhook/events.go", Kind: "source", Reasons: []string{"path_anchor:webhook"}},
		},
	}, false)
	out := b.String()
	assert.Contains(t, out, "Related files from selected source context",
		"related files output missing %q:\n%s", "Related files from selected source context", out)
	assert.Contains(t, out, "modules/webhook/events.go",
		"related files output missing %q:\n%s", "modules/webhook/events.go", out)
	assert.Contains(t, out, "path_anchor:webhook",
		"related files output missing %q:\n%s", "path_anchor:webhook", out)

}

func TestNormalizeFindSourceTestReceiptsMode_WithEmptyInput_ReturnsOff(t *testing.T) {
	got := normalizeFindSourceTestReceiptsMode("")

	assert.Equal(t, findSourceTestReceiptsModeOff, got)
}

func TestNormalizeFindSourceTestReceiptsMode_WithOff_ReturnsOff(t *testing.T) {
	got := normalizeFindSourceTestReceiptsMode("off")

	assert.Equal(t, findSourceTestReceiptsModeOff, got)
}

func TestNormalizeFindSourceTestReceiptsMode_WithRelatedTestsV0_ReturnsReceiptV0(t *testing.T) {
	got := normalizeFindSourceTestReceiptsMode("related-tests-v0")

	assert.Equal(t, findSourceTestReceiptsModeReceiptV0, got)
}

func TestNormalizeFindSourceTestReceiptsMode_WithReceiptV0_ReturnsReceiptV0(t *testing.T) {
	got := normalizeFindSourceTestReceiptsMode("receipt_v0")

	assert.Equal(t, findSourceTestReceiptsModeReceiptV0, got)
}

func TestNormalizeFindSourceTestReceiptsMode_WithRelatedFilesReceiptV0_ReturnsRelatedFilesReceiptV0(t *testing.T) {
	got := normalizeFindSourceTestReceiptsMode("related-files-receipt-v0")

	assert.Equal(t, findSourceTestReceiptsModeRelatedFilesReceiptV0, got)
}

func TestNormalizeFindSourceTestReceiptsMode_WithUnknownInput_ReturnsEmpty(t *testing.T) {
	got := normalizeFindSourceTestReceiptsMode("nope")

	assert.Empty(t, got)
}

func findRelatedTestHasPath(ctx *FindRelatedTestContext, path string) bool {
	if ctx == nil {
		return false
	}
	for _, item := range ctx.Items {
		if item.Path == path {
			return true
		}
	}
	return false
}
