package sourcecontext

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverIndexesBoundedSourceFiles(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "services", "api", "handler.ts"), "export function handler() {}\n")
	mustWrite(t, filepath.Join(root, "services", "api", "component.tsx"), "export function Component() { return null }\n")
	mustWrite(t, filepath.Join(root, "services", "api", "intent_plan.go"), "package api\n")
	mustWrite(t, filepath.Join(root, "services", "api", "invariant_policy.py"), "def run():\n    pass\n")
	mustWrite(t, filepath.Join(root, "services", "api", "design_rule.rs"), "pub fn run() {}\n")
	mustWrite(t, filepath.Join(root, "services", "api", "requirement_controller.java"), "class RequirementController {}\n")
	mustWrite(t, filepath.Join(root, "internal", "costs", "parser_minimax.go"), "package costs\n")
	mustWrite(t, filepath.Join(root, "crates", "atuin-daemon", "src", "daemon.rs"), "pub fn run() {}\n")
	mustWrite(t, filepath.Join(root, "browser_use", "llm", "messages.py"), "def to_messages():\n    pass\n")
	mustWrite(t, filepath.Join(root, "tests", "integration", "util.rs"), "pub fn helper() {}\n")
	mustWrite(t, filepath.Join(root, "tests", "valid_configs", "empty_config.toml"), "[flags]\n")
	mustWrite(t, filepath.Join(root, "services", "api", "Widget.vue"), "<script setup lang=\"ts\"></script>\n")
	mustWrite(t, filepath.Join(root, "services", "api", "tool.mjs"), "export function tool() {}\n")
	mustWrite(t, filepath.Join(root, "services", "api", "devspecs.toml"), "[tool]\n")
	mustWrite(t, filepath.Join(root, "services", "api", "Dockerfile.intent"), "FROM scratch\n")
	mustWrite(t, filepath.Join(root, "e2e-tests", "docker", "Dockerfile.codex"), "FROM scratch\n")
	mustWrite(t, filepath.Join(root, "services", "api", "main.go"), "package api\n")
	mustWrite(t, filepath.Join(root, "services", "api", "schema.sql"), "create table events(id text);\n")
	mustWrite(t, filepath.Join(root, "docs", "plan.md"), "# Plan\n")
	mustWrite(t, filepath.Join(root, "node_modules", "pkg", "ignored.ts"), "export const ignored = true\n")

	candidates, err := (&Adapter{}).Discover(context.Background(), root, nil)
	require.NoError(t, err)

	got := candidatePaths(candidates)
	want := []string{
		"browser_use/llm/messages.py",
		"crates/atuin-daemon/src/daemon.rs",
		"e2e-tests/docker/Dockerfile.codex",
		"internal/costs/parser_minimax.go",
		"services/api/Dockerfile.intent",
		"services/api/Widget.vue",
		"services/api/component.tsx",
		"services/api/design_rule.rs",
		"services/api/devspecs.toml",
		"services/api/handler.ts",
		"services/api/intent_plan.go",
		"services/api/invariant_policy.py",
		"services/api/requirement_controller.java",
		"services/api/schema.sql",
		"services/api/tool.mjs",
		"tests/integration/util.rs",
		"tests/valid_configs/empty_config.toml",
	}
	require.Len(t, got, len(want))
	assert.Equal(t, want[0], got[0])
	assert.Equal(t, want[1], got[1])
	assert.Equal(t, want[2], got[2])
	assert.Equal(t, want[3], got[3])
	assert.Equal(t, want[4], got[4])
	assert.Equal(t, want[5], got[5])
	assert.Equal(t, want[6], got[6])
	assert.Equal(t, want[7], got[7])
	assert.Equal(t, want[8], got[8])
	assert.Equal(t, want[9], got[9])
	assert.Equal(t, want[10], got[10])
	assert.Equal(t, want[11], got[11])
	assert.Equal(t, want[12], got[12])
	assert.Equal(t, want[13], got[13])
	assert.Equal(t, want[14], got[14])
	assert.Equal(t, want[15], got[15])
	assert.Equal(t, want[16], got[16])
}

func TestDiscoverHonorsConfiguredSourcePath(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "services", "api", "handler.ts"), "export function handler() {}\n")
	mustWrite(t, filepath.Join(root, "scripts", "tool.ts"), "export function tool() {}\n")
	cfg := &config.RepoConfig{Sources: []config.SourceConfig{{Type: sourceType, Path: "scripts"}}}

	candidates, err := (&Adapter{}).Discover(context.Background(), root, cfg)
	require.NoError(t, err)

	got := candidatePaths(candidates)
	require.Len(t, got, 1)
	assert.Equal(t, "scripts/tool.ts", got[0])

}

func TestParseSourceContextArtifact(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "services", "api", "handler.ts")
	mustWrite(t, path, "export function handler() {}\n")
	art, sources, _, err := (&Adapter{}).Parse(context.Background(), adapters.Candidate{
		PrimaryPath: path,
		RelPath:     "services/api/handler.ts",
		AdapterName: sourceType,
		Metadata:    map[string]any{"admission_reason": "implementation_root_source_context"},
	})
	require.NoError(t, err)

	require.Equal(t, config.KindSourceContext, art.Kind,
		"kind got %q", art.Kind)
	require.Equal(t, "services/api/handler.ts (typescript)", art.Title,
		"title got %q", art.Title)
	require.Len(t, sources, 1)
	assert.Equal(t, sourceType, sources[0].SourceType)
	assert.Equal(t, "services/api/handler.ts", sources[0].Path)
	assert.NotEmpty(t, art.Body)
	require.Equal(t, "implementation_root_source_context", art.Extracted["admission_reason"],
		"admission reason got %#v", art.Extracted["admission_reason"])

}

func TestParseSourceContext_WithTestSource_ExtractsSymbolsAndTestNames(t *testing.T) {
	root := t.TempDir()
	testPath := filepath.Join(root, "tests", "test_upload_file.py")
	mustWrite(t, testPath, "class UploadFileTests:\n    pass\n\ndef test_password_protected_cert_cli_arg():\n    pass\n")

	testArt, _, _, err := (&Adapter{}).Parse(context.Background(), adapters.Candidate{
		PrimaryPath: testPath,
		RelPath:     "tests/test_upload_file.py",
		AdapterName: sourceType,
		Metadata: map[string]any{
			"admission_reason": "first_party_source_context",
			"source_role":      "test",
			"source_root":      "tests",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "test_case", testArt.Subtype)
	assert.NotEmpty(t, testArt.Extracted["test_name"])
	assert.NotEmpty(t, testArt.Extracted["source_symbols"])
}

func TestParseSourceContext_WithImplementationSource_ExtractsSymbolsWithoutTestSubtype(t *testing.T) {
	root := t.TempDir()
	implPath := filepath.Join(root, "fastapi", "datastructures.py")
	mustWrite(t, implPath, "class UploadFile:\n    pass\n\ndef create_upload_file():\n    pass\n")

	implArt, _, _, err := (&Adapter{}).Parse(context.Background(), adapters.Candidate{
		PrimaryPath: implPath,
		RelPath:     "fastapi/datastructures.py",
		AdapterName: sourceType,
		Metadata: map[string]any{
			"admission_reason": "first_party_source_context",
			"source_role":      "implementation",
			"source_root":      "fastapi",
		},
	})

	require.NoError(t, err)
	assert.Empty(t, implArt.Subtype)
	assert.NotEmpty(t, implArt.Extracted["source_symbols"])
}

func TestParseSourceContextInfersTestSubtypeFromPathWithoutMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tests", "tests.rs")
	mustWrite(t, path, "#[test]\nfn print0_exec_uses_nulls() {}\n")
	art, _, _, err := (&Adapter{}).Parse(context.Background(), adapters.Candidate{
		PrimaryPath: path,
		RelPath:     "tests/tests.rs",
		AdapterName: sourceType,
	})
	require.NoError(t, err)

	require.Equal(t, "test_case", art.Subtype,
		"subtype got %q", art.Subtype)
	require.Equal(t, "test", art.Extracted["source_role"],
		"source_role got %#v", art.Extracted["source_role"])

}

func TestIsSourceContextFile_WithBehaviorPythonFile_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("src/behavior.py")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithIntentGoFile_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("src/intent_plan.go")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithDesignRustFile_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("src/design_rule.rs")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithRequirementJavaFile_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("src/requirement.java")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithIntentTreatmentTSXFile_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("src/App.tsx")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithIntentTreatmentJSXFile_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("src/component.jsx")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithIntentTreatmentCommonJSFile_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("src/tool.cjs")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithIntentTreatmentVueFile_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("src/View.vue")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithDevSpecsConfig_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("devspecs.toml")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithIntentDockerfile_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("docker/Dockerfile.intent")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithKnownImplementationRoot_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("internal/costs/parser_minimax.go")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithRustImplementationRoot_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("crates/atuin-daemon/src/daemon.rs")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithPythonImplementationRoot_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("browser_use/llm/messages.py")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithConfigFixture_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("tests/valid_configs/empty_config.toml")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithDockerFixture_ReturnsTrue(t *testing.T) {
	actual := isSourceContextFile("e2e-tests/docker/Dockerfile.codex")

	assert.True(t, actual)
}

func TestIsSourceContextFile_WithOrdinaryGoFile_ReturnsFalse(t *testing.T) {
	actual := isSourceContextFile("services/api/main.go")

	assert.False(t, actual)
}

func TestIsSourceContextFile_WithIntegrationTestFile_ReturnsFalse(t *testing.T) {
	actual := isSourceContextFile("internal/costs/parser_minimax_integration_test.go")

	assert.False(t, actual)
}

func TestSourceLanguage_WithPythonFile_ReturnsPython(t *testing.T) {
	actual := sourceLanguage("src/behavior.py")

	assert.Equal(t, "python", actual)
}

func TestSourceLanguage_WithGoFile_ReturnsGo(t *testing.T) {
	actual := sourceLanguage("src/intent_plan.go")

	assert.Equal(t, "go", actual)
}

func TestSourceLanguage_WithRustFile_ReturnsRust(t *testing.T) {
	actual := sourceLanguage("src/design_rule.rs")

	assert.Equal(t, "rust", actual)
}

func TestSourceLanguage_WithJavaFile_ReturnsJava(t *testing.T) {
	actual := sourceLanguage("src/requirement.java")

	assert.Equal(t, "java", actual)
}

func TestSourceLanguage_WithTSXFile_ReturnsTypeScriptReact(t *testing.T) {
	actual := sourceLanguage("src/App.tsx")

	assert.Equal(t, "typescript-react", actual)
}

func TestSourceLanguage_WithJSXFile_ReturnsJavaScriptReact(t *testing.T) {
	actual := sourceLanguage("src/component.jsx")

	assert.Equal(t, "javascript-react", actual)
}

func TestSourceLanguage_WithCommonJSFile_ReturnsJavaScript(t *testing.T) {
	actual := sourceLanguage("src/tool.cjs")

	assert.Equal(t, "javascript", actual)
}

func TestSourceLanguage_WithVueFile_ReturnsVue(t *testing.T) {
	actual := sourceLanguage("src/View.vue")

	assert.Equal(t, "vue", actual)
}

func TestSourceLanguage_WithTomlFile_ReturnsToml(t *testing.T) {
	actual := sourceLanguage("devspecs.toml")

	assert.Equal(t, "toml", actual)
}

func TestSourceLanguage_WithNamedDockerfile_ReturnsDockerfile(t *testing.T) {
	actual := sourceLanguage("docker/Dockerfile.intent")

	assert.Equal(t, "dockerfile", actual)
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

}

func candidatePaths(candidates []adapters.Candidate) []string {
	out := make([]string, len(candidates))
	for i, c := range candidates {
		out[i] = c.RelPath
	}
	return out
}
