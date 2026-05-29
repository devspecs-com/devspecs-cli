package sourcecontext

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/config"
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
	if err != nil {
		t.Fatal(err)
	}
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
	if len(got) != len(want) {
		t.Fatalf("paths got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("paths got %#v want %#v", got, want)
		}
	}
}

func TestDiscoverHonorsConfiguredSourcePath(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "services", "api", "handler.ts"), "export function handler() {}\n")
	mustWrite(t, filepath.Join(root, "scripts", "tool.ts"), "export function tool() {}\n")
	cfg := &config.RepoConfig{Sources: []config.SourceConfig{{Type: sourceType, Path: "scripts"}}}

	candidates, err := (&Adapter{}).Discover(context.Background(), root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := candidatePaths(candidates)
	if len(got) != 1 || got[0] != "scripts/tool.ts" {
		t.Fatalf("paths got %#v", got)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if art.Kind != config.KindSourceContext {
		t.Fatalf("kind got %q", art.Kind)
	}
	if art.Title != "services/api/handler.ts (typescript)" {
		t.Fatalf("title got %q", art.Title)
	}
	if len(sources) != 1 || sources[0].SourceType != sourceType || sources[0].Path != "services/api/handler.ts" {
		t.Fatalf("sources got %#v", sources)
	}
	if art.Body == "" {
		t.Fatal("expected body")
	}
	if art.Extracted["admission_reason"] != "implementation_root_source_context" {
		t.Fatalf("admission reason got %#v", art.Extracted["admission_reason"])
	}
}

func TestSourceLanguageCoversIntentTreatmentExtensions(t *testing.T) {
	cases := map[string]string{
		"src/behavior.py":                       "python",
		"src/intent_plan.go":                    "go",
		"src/design_rule.rs":                    "rust",
		"src/requirement.java":                  "java",
		"src/App.tsx":                           "typescript-react",
		"src/component.jsx":                     "javascript-react",
		"src/tool.cjs":                          "javascript",
		"src/View.vue":                          "vue",
		"devspecs.toml":                         "toml",
		"docker/Dockerfile.intent":              "dockerfile",
		"internal/costs/parser_minimax.go":      "go",
		"crates/atuin-daemon/src/daemon.rs":     "rust",
		"browser_use/llm/messages.py":           "python",
		"tests/valid_configs/empty_config.toml": "toml",
		"e2e-tests/docker/Dockerfile.codex":     "dockerfile",
	}
	for path, want := range cases {
		if !isSourceContextFile(path) {
			t.Fatalf("%s should be admitted as source context", path)
		}
		if got := sourceLanguage(path); got != want {
			t.Fatalf("sourceLanguage(%q) = %q, want %q", path, got, want)
		}
	}
	if isSourceContextFile("services/api/main.go") {
		t.Fatal("ordinary Go files outside implementation roots should not be admitted as broad generic source context")
	}
	if isSourceContextFile("internal/costs/parser_minimax_integration_test.go") {
		t.Fatal("expanded source context should leave test files to the test-case adapter")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func candidatePaths(candidates []adapters.Candidate) []string {
	out := make([]string, len(candidates))
	for i, c := range candidates {
		out[i] = c.RelPath
	}
	return out
}
