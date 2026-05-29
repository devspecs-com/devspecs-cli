package testcase

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
)

func TestExtractUnits_CommonFrameworks(t *testing.T) {
	tests := []struct {
		name      string
		rel       string
		body      string
		wantName  string
		wantLang  string
		wantFrame string
	}{
		{
			name:      "go",
			rel:       "service/webhook_test.go",
			body:      "package service\n\nfunc TestWebhookReplayProtection(t *testing.T) {\n\trequire.NoError(t, err)\n}\n",
			wantName:  "TestWebhookReplayProtection",
			wantLang:  "go",
			wantFrame: "go test",
		},
		{
			name:      "python",
			rel:       "tests/test_billing_retry.py",
			body:      "class TestBilling:\n    def test_retry_permission_error(self):\n        assert retry_count == 2\n",
			wantName:  "test_retry_permission_error",
			wantLang:  "python",
			wantFrame: "pytest",
		},
		{
			name:      "typescript",
			rel:       "__tests__/billing.spec.ts",
			body:      "describe('billing webhooks', () => {\n  it('rejects replayed stripe events', () => {\n    expect(status).toBe(409)\n  })\n})\n",
			wantName:  "rejects replayed stripe events",
			wantLang:  "typescript",
			wantFrame: "javascript-test",
		},
		{
			name:      "ruby",
			rel:       "spec/billing_spec.rb",
			body:      "RSpec.describe 'billing webhooks' do\n  it 'rejects replayed stripe events' do\n    expect(status).to eq(409)\n  end\nend\n",
			wantName:  "rejects replayed stripe events",
			wantLang:  "ruby",
			wantFrame: "rspec",
		},
		{
			name:      "php",
			rel:       "tests/BillingTest.php",
			body:      "<?php\nfinal class BillingTest extends TestCase {\n  #[Test]\n  public function rejects_replayed_stripe_events(): void {\n    $this->assertSame(409, $status);\n  }\n}\n",
			wantName:  "rejects_replayed_stripe_events",
			wantLang:  "php",
			wantFrame: "phpunit",
		},
		{
			name:      "java",
			rel:       "src/test/java/com/example/BillingTest.java",
			body:      "class BillingTest {\n  @Test\n  public void testRejectsReplayedStripeEvents() {\n    assertEquals(409, status);\n  }\n}\n",
			wantName:  "testRejectsReplayedStripeEvents",
			wantLang:  "java",
			wantFrame: "junit",
		},
		{
			name:      "kotlin",
			rel:       "src/test/kotlin/com/example/BillingSpec.kt",
			body:      "class BillingSpec {\n  @Test\n  fun `rejects replayed stripe events`() {\n    assertEquals(409, status)\n  }\n}\n",
			wantName:  "rejects replayed stripe events",
			wantLang:  "kotlin",
			wantFrame: "junit",
		},
		{
			name:      "rust",
			rel:       "crates/cli/tests/help_test.rs",
			body:      "#[test]\nfn help_work_invalid_sgconfig() {\n    assert!(output.contains(\"invalid\"));\n}\n",
			wantName:  "help_work_invalid_sgconfig",
			wantLang:  "rust",
			wantFrame: "rust test",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			units := extractUnits(tc.rel, tc.body)
			if len(units) != 1 {
				t.Fatalf("extractUnits returned %d units, want 1: %#v", len(units), units)
			}
			if units[0].Name != tc.wantName {
				t.Fatalf("Name = %q, want %q", units[0].Name, tc.wantName)
			}
			if units[0].Language != tc.wantLang || units[0].Framework != tc.wantFrame {
				t.Fatalf("language/framework = %q/%q, want %q/%q", units[0].Language, units[0].Framework, tc.wantLang, tc.wantFrame)
			}
			if units[0].StartLine == 0 || units[0].EndLine < units[0].StartLine {
				t.Fatalf("bad line range: %d-%d", units[0].StartLine, units[0].EndLine)
			}
			if len(units[0].Symbols) == 0 {
				t.Fatalf("expected weak symbol terms")
			}
			if len(units[0].Assertions) == 0 {
				t.Fatalf("expected assertion vocabulary")
			}
		})
	}
}

func TestDiscoverRequiresExperimentAndParsesArtifact(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tests", "webhook_test.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package tests\n\nfunc TestWebhookReplayProtection(t *testing.T) {\n\trequire.NoError(t, err)\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := &Adapter{}
	disabled, err := adapter.Discover(context.Background(), root, config.DefaultRepoConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled) != 0 {
		t.Fatalf("disabled experiment returned %d candidates", len(disabled))
	}

	cfg := config.WithTestCaseArtifacts(config.DefaultRepoConfig(), true)
	candidates, err := adapter.Discover(context.Background(), root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("Discover returned %d candidates, want 1: %#v", len(candidates), candidates)
	}
	art, sources, _, err := adapter.Parse(context.Background(), candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if art.Kind != config.KindSourceContext || art.Subtype != config.SubtypeTestCase {
		t.Fatalf("kind/subtype = %q/%q", art.Kind, art.Subtype)
	}
	if art.Extracted["source_line_range"] == "" {
		t.Fatalf("missing source_line_range: %#v", art.Extracted)
	}
	if len(sources) != 1 || sources[0].SourceType != "test_case" {
		t.Fatalf("sources = %#v", sources)
	}
}

func TestExtractUnits_AnnotatedLanguagesStartAtAnnotation(t *testing.T) {
	tests := []struct {
		name string
		rel  string
		body string
	}{
		{
			name: "java",
			rel:  "src/test/java/com/example/BillingTest.java",
			body: "class BillingTest {\n  @Test\n  public void testRejectsReplay() {}\n}\n",
		},
		{
			name: "rust",
			rel:  "crates/cli/tests/help_test.rs",
			body: "#[test]\nfn help_work_invalid_sgconfig() {}\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			units := extractUnits(tc.rel, tc.body)
			if len(units) != 1 {
				t.Fatalf("extractUnits returned %d units, want 1", len(units))
			}
			if units[0].StartLine != 2 && tc.name == "java" {
				t.Fatalf("java start line = %d, want annotation line 2", units[0].StartLine)
			}
			if units[0].StartLine != 1 && tc.name == "rust" {
				t.Fatalf("rust start line = %d, want annotation line 1", units[0].StartLine)
			}
		})
	}
}
