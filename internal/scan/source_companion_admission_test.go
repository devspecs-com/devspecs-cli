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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NotNil(t, diagnostics)
	assert.Positive(t, diagnostics.Admitted)

	got := companionCandidatePaths(companions)
	require.Len(t, got, 4)
	assert.True(t, got["internal/auth/session.go"])
	assert.True(t, got["src/ui/webhooks.ts"])
	assert.True(t, got["scripts/publish_keys.py"])
	assert.True(t, got["src/main/java/com/acme/Foo.java"])
	for _, companion := range companions {
		require.Equal(t, sourceCompanionAdmissionReason, companion.Metadata["admission_reason"],
			"missing admission metadata on %#v", companion)

	}
}

func TestSourceCompanionAdmissionRejectsUnsafeAndTestLike(t *testing.T) {
	root := t.TempDir()
	writeCompanionTestFile(t, root, "src/foo/foo.test.ts", "import \"./helper.test\"\n")
	writeCompanionTestFile(t, root, "src/bar/bar.test.ts", "import \"../vendor/pkg/client\"\n")
	writeCompanionTestFile(t, root, "src/foo/helper.test.ts", "export const helper = true\n")
	writeCompanionTestFile(t, root, "src/vendor/pkg/client.ts", "export const client = true\n")

	diagnostics, companions := buildTestSourceCompanionCandidates(context.Background(), root, []adapters.Candidate{{RelPath: "src/foo/foo.test.ts"}, {RelPath: "src/bar/bar.test.ts"}}, nil)
	assert.Empty(t, companions)
	require.NotNil(t, diagnostics)
	assert.Positive(t, diagnostics.RejectedByReason["test_like_source"])
	assert.Positive(t, diagnostics.RejectedByReason["generated_vendor_or_build"])
}

func TestSourceCompanionAdmissionDedupesExistingSourceCandidates(t *testing.T) {
	root := t.TempDir()
	writeCompanionTestFile(t, root, "src/rules.go", "package src\nfunc Rule() {}\n")
	writeCompanionTestFile(t, root, "src/rules_test.go", "package src\nfunc TestRule(t *testing.T) { Rule() }\n")

	diagnostics, companions := buildTestSourceCompanionCandidates(context.Background(), root,
		[]adapters.Candidate{{RelPath: "src/rules_test.go"}},
		[]adapters.Candidate{{RelPath: "src/rules.go"}},
	)
	assert.Empty(t, companions)
	require.NotNil(t, diagnostics)
	assert.Equal(t, 1, diagnostics.AlreadyPresent)
}

func TestScanAdmitsTestSourceCompanionsAndBuildsEdges(t *testing.T) {
	root := t.TempDir()
	writeCompanionTestFile(t, root, "services/auth/session.go", "package auth\nfunc RotateToken() {}\n")
	writeCompanionTestFile(t, root, "services/auth/session_test.go", "package auth\nimport \"testing\"\nfunc TestSession(t *testing.T) { RotateToken() }\n")

	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()

	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&sourcecontext.Adapter{}, &testcase.Adapter{}})
	result, err := scanner.RunWithOptions(context.Background(), root, config.WithTestCaseArtifacts(config.DefaultRepoConfig(), true), RunOptions{SkipAuthoredAtLookup: true})
	require.NoError(t, err)
	require.NotNil(t, result.SourceCompanions)
	assert.Equal(t, 1, result.SourceCompanions.Admitted)
	require.NotNil(t, result.EvidenceGraph)
	assert.Positive(t, result.EvidenceGraph.EdgesByType[edgeTypeTestsSource])
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
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
