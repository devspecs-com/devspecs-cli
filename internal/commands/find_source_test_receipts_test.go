package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestBuildFindSourceTestReceiptsAddsBehaviorTestsWithoutPackMutation(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := "2026-06-05T00:00:00Z"
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", "repo_src", tmp, now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceRepoSourceManifest("repo_src",
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
	); err != nil {
		t.Fatal(err)
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
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Items) == 0 {
		t.Fatalf("expected related test receipts, got %#v", got)
	}
	if !findRelatedTestHasPath(got, "models/webhook/webhook_test.go") {
		t.Fatalf("expected webhook model test receipt, got %#v", got.Items)
	}
	if !findRelatedTestHasPath(got, "tests/integration/repo_webhook_test.go") {
		t.Fatalf("expected repo webhook integration test receipt, got %#v", got.Items)
	}
	if findRelatedTestHasPath(got, "apps/web/app/api/webhook/test/route.ts") {
		t.Fatalf("did not expect test helper route receipt, got %#v", got.Items)
	}
}

func TestBuildFindSourceTestReceiptsRelatedFilesAddsSourceReceipts(t *testing.T) {
	tmp := t.TempDir()
	db, err := store.Open(filepath.Join(tmp, "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := "2026-06-05T00:00:00Z"
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)", "repo_src", tmp, now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceRepoSourceManifest("repo_src",
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
	); err != nil {
		t.Fatal(err)
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
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Mode != findSourceTestReceiptsModeRelatedFilesReceiptV0 {
		t.Fatalf("expected related-files receipt context, got %#v", got)
	}
	if !findRelatedTestHasPath(got, "models/webhook/webhook_test.go") {
		t.Fatalf("expected existing test receipt, got %#v", got.Items)
	}
	if !findRelatedTestHasPath(got, "modules/webhook/events.go") {
		t.Fatalf("expected source related-file receipt, got %#v", got.Items)
	}
	if findRelatedTestHasPath(got, "modules/settings/settings.go") {
		t.Fatalf("did not expect unrelated source receipt, got %#v", got.Items)
	}
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
	for _, want := range []string{"Related tests from selected source context", "internal/commands/map_test.go", "same_stem"} {
		if !strings.Contains(out, want) {
			t.Fatalf("related test output missing %q:\n%s", want, out)
		}
	}
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
	for _, want := range []string{"Related files from selected source context", "modules/webhook/events.go", "path_anchor:webhook"} {
		if !strings.Contains(out, want) {
			t.Fatalf("related files output missing %q:\n%s", want, out)
		}
	}
}

func TestNormalizeFindSourceTestReceiptsMode(t *testing.T) {
	tests := map[string]string{
		"":                         findSourceTestReceiptsModeOff,
		"off":                      findSourceTestReceiptsModeOff,
		"related-tests-v0":         findSourceTestReceiptsModeReceiptV0,
		"receipt_v0":               findSourceTestReceiptsModeReceiptV0,
		"related-files-receipt-v0": findSourceTestReceiptsModeRelatedFilesReceiptV0,
		"nope":                     "",
	}
	for input, want := range tests {
		if got := normalizeFindSourceTestReceiptsMode(input); got != want {
			t.Fatalf("normalizeFindSourceTestReceiptsMode(%q) = %q, want %q", input, got, want)
		}
	}
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
