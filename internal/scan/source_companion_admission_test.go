package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/sourcecontext"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/testcase"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestSourceCompanionAdmissionDerivesCommonStemsAndImports(t *testing.T) {
	root := t.TempDir()
	writeCompanionTestFile(t, root, "internal/auth/session.go", "package auth\nfunc RotateToken() {}\n")
	writeCompanionTestFile(t, root, "internal/auth/session_test.go", "package auth\nfunc TestSession(t *testing.T) { RotateToken() }\n")
	writeCompanionTestFile(t, root, "src/ui/webhooks.ts", "export function handleWebhook() {}\n")
	writeCompanionTestFile(t, root, "src/ui/webhooks.test.ts", "import { handleWebhook } from \"./webhooks\"\n")
	writeCompanionTestFile(t, root, "scripts/publish_keys.py", "def publish_keys():\n    return True\n")
	writeCompanionTestFile(t, root, "tests/test_publish_keys.py", "from scripts.publish_keys import publish_keys\n")
	writeCompanionTestFile(t, root, "src/main/java/com/acme/Foo.java", "package com.acme; class Foo {}\n")
	writeCompanionTestFile(t, root, "src/test/java/com/acme/FooTest.java", "package com.acme; class FooTest {}\n")

	diagnostics, companions := buildTestSourceCompanionCandidates(context.Background(), root, []adapters.Candidate{
		{RelPath: "internal/auth/session_test.go"},
		{RelPath: "src/ui/webhooks.test.ts"},
		{RelPath: "tests/test_publish_keys.py"},
		{RelPath: "src/test/java/com/acme/FooTest.java"},
	}, nil)

	if diagnostics == nil || diagnostics.Admitted == 0 {
		t.Fatalf("expected companion diagnostics and admissions, got %#v", diagnostics)
	}
	got := companionCandidatePaths(companions)
	for _, want := range []string{
		"internal/auth/session.go",
		"src/ui/webhooks.ts",
		"scripts/publish_keys.py",
		"src/main/java/com/acme/Foo.java",
	} {
		if !got[want] {
			t.Fatalf("missing companion %s in %#v", want, got)
		}
	}
	for _, companion := range companions {
		if companion.Metadata["admission_reason"] != sourceCompanionAdmissionReason {
			t.Fatalf("missing admission metadata on %#v", companion)
		}
	}
}

func TestSourceCompanionAdmissionRejectsUnsafeAndTestLike(t *testing.T) {
	root := t.TempDir()
	writeCompanionTestFile(t, root, "src/foo/foo.test.ts", "import \"./helper.test\"\n")
	writeCompanionTestFile(t, root, "src/bar/bar.test.ts", "import \"../vendor/pkg/client\"\n")
	writeCompanionTestFile(t, root, "src/foo/helper.test.ts", "export const helper = true\n")
	writeCompanionTestFile(t, root, "src/vendor/pkg/client.ts", "export const client = true\n")

	diagnostics, companions := buildTestSourceCompanionCandidates(context.Background(), root, []adapters.Candidate{{RelPath: "src/foo/foo.test.ts"}, {RelPath: "src/bar/bar.test.ts"}}, nil)
	if len(companions) != 0 {
		t.Fatalf("unsafe/test-like companions should not be admitted: %#v", companions)
	}
	if diagnostics == nil {
		t.Fatal("expected diagnostics")
	}
	if diagnostics.RejectedByReason["test_like_source"] == 0 {
		t.Fatalf("expected test-like rejection: %#v", diagnostics.RejectedByReason)
	}
	if diagnostics.RejectedByReason["generated_vendor_or_build"] == 0 {
		t.Fatalf("expected vendor/build rejection: %#v", diagnostics.RejectedByReason)
	}
}

func TestSourceCompanionAdmissionDedupesExistingSourceCandidates(t *testing.T) {
	root := t.TempDir()
	writeCompanionTestFile(t, root, "src/rules.go", "package src\nfunc Rule() {}\n")
	writeCompanionTestFile(t, root, "src/rules_test.go", "package src\nfunc TestRule(t *testing.T) { Rule() }\n")

	diagnostics, companions := buildTestSourceCompanionCandidates(context.Background(), root,
		[]adapters.Candidate{{RelPath: "src/rules_test.go"}},
		[]adapters.Candidate{{RelPath: "src/rules.go"}},
	)

	if len(companions) != 0 {
		t.Fatalf("existing source candidate should not be duplicated: %#v", companions)
	}
	if diagnostics == nil || diagnostics.AlreadyPresent != 1 {
		t.Fatalf("expected already-present count, got %#v", diagnostics)
	}
}

func TestScanAdmitsTestSourceCompanionsAndBuildsEdges(t *testing.T) {
	root := t.TempDir()
	writeCompanionTestFile(t, root, "internal/auth/session.go", "package auth\nfunc RotateToken() {}\n")
	writeCompanionTestFile(t, root, "internal/auth/session_test.go", "package auth\nimport \"testing\"\nfunc TestSession(t *testing.T) { RotateToken() }\n")

	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}, &testcase.Adapter{}})
	result, err := scanner.RunWithOptions(context.Background(), root, config.WithTestCaseArtifacts(config.DefaultRepoConfig(), true), RunOptions{SkipAuthoredAtLookup: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceCompanions == nil || result.SourceCompanions.Admitted != 1 {
		t.Fatalf("expected one admitted source companion, got %#v", result.SourceCompanions)
	}
	if result.EvidenceGraph == nil || result.EvidenceGraph.EdgesByType[edgeTypeTestsSource] == 0 {
		t.Fatalf("expected tests_source edge after companion admission, got %#v", result.EvidenceGraph)
	}
}

func companionCandidatePaths(candidates []adapters.Candidate) map[string]bool {
	out := map[string]bool{}
	for _, candidate := range candidates {
		out[candidate.RelPath] = true
	}
	return out
}

func writeCompanionTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
