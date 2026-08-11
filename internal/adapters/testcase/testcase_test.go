package testcase

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

func TestExtractUnits_WithGoTest_ReturnsGoTestUnit(t *testing.T) {
	body := "package service\n\nfunc TestWebhookReplayProtection(t *testing.T) {\n\trequire.NoError(t, err)\n}\n"

	units := extractUnits("service/webhook_test.go", body)

	assertSingleUnit(t, units, "TestWebhookReplayProtection", "go", "go test")
}

func TestExtractUnits_WithPytest_ReturnsPythonTestUnit(t *testing.T) {
	body := "class TestBilling:\n    def test_retry_permission_error(self):\n        assert retry_count == 2\n"

	units := extractUnits("tests/test_billing_retry.py", body)

	assertSingleUnit(t, units, "test_retry_permission_error", "python", "pytest")
}

func TestExtractUnits_WithTypeScriptTest_ReturnsJavaScriptTestUnit(t *testing.T) {
	body := "describe('billing webhooks', () => {\n  it('rejects replayed stripe events', () => {\n    expect(status).toBe(409)\n  })\n})\n"

	units := extractUnits("__tests__/billing.spec.ts", body)

	assertSingleUnit(t, units, "rejects replayed stripe events", "typescript", "javascript-test")
}

func TestExtractUnits_WithRSpecTest_ReturnsRubyTestUnit(t *testing.T) {
	body := "RSpec.describe 'billing webhooks' do\n  it 'rejects replayed stripe events' do\n    expect(status).to eq(409)\n  end\nend\n"

	units := extractUnits("spec/billing_spec.rb", body)

	assertSingleUnit(t, units, "rejects replayed stripe events", "ruby", "rspec")
}

func TestExtractUnits_WithPHPUnitTest_ReturnsPHPTestUnit(t *testing.T) {
	body := "<?php\nfinal class BillingTest extends TestCase {\n  #[Test]\n  public function rejects_replayed_stripe_events(): void {\n    $this->assertSame(409, $status);\n  }\n}\n"

	units := extractUnits("tests/BillingTest.php", body)

	assertSingleUnit(t, units, "rejects_replayed_stripe_events", "php", "phpunit")
}

func TestExtractUnits_WithJUnitJavaTest_ReturnsJavaTestUnit(t *testing.T) {
	body := "class BillingTest {\n  @Test\n  public void testRejectsReplayedStripeEvents() {\n    assertEquals(409, status);\n  }\n}\n"

	units := extractUnits("src/test/java/com/example/BillingTest.java", body)

	assertSingleUnit(t, units, "testRejectsReplayedStripeEvents", "java", "junit")
}

func TestExtractUnits_WithJUnitKotlinTest_ReturnsKotlinTestUnit(t *testing.T) {
	body := "class BillingSpec {\n  @Test\n  fun `rejects replayed stripe events`() {\n    assertEquals(409, status)\n  }\n}\n"

	units := extractUnits("src/test/kotlin/com/example/BillingSpec.kt", body)

	assertSingleUnit(t, units, "rejects replayed stripe events", "kotlin", "junit")
}

func TestExtractUnits_WithRustTest_ReturnsRustTestUnit(t *testing.T) {
	body := "#[test]\nfn help_work_invalid_sgconfig() {\n    assert!(output.contains(\"invalid\"));\n}\n"

	units := extractUnits("crates/cli/tests/help_test.rs", body)

	assertSingleUnit(t, units, "help_work_invalid_sgconfig", "rust", "rust test")
}

func TestDiscover_WhenTestCaseArtifactsDisabled_ReturnsNoCandidates(t *testing.T) {
	root, _ := writeGoTestFixture(t)
	adapter := &Adapter{}

	candidates, err := adapter.Discover(context.Background(), root, config.DefaultRepoConfig())

	require.NoError(t, err)
	assert.Empty(t, candidates)
}

func TestDiscover_WhenTestCaseArtifactsEnabled_ReturnsCandidate(t *testing.T) {
	root, _ := writeGoTestFixture(t)
	adapter := &Adapter{}
	cfg := config.WithTestCaseArtifacts(config.DefaultRepoConfig(), true)

	candidates, err := adapter.Discover(context.Background(), root, cfg)

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, "tests/webhook_test.go", filepath.ToSlash(candidates[0].RelPath))
}

func TestParse_WithGoTestCandidate_ReturnsTestCaseArtifact(t *testing.T) {
	_, path := writeGoTestFixture(t)
	adapter := &Adapter{}
	candidate := adapters.Candidate{
		PrimaryPath:    path,
		RelPath:        "tests/webhook_test.go",
		AdapterName:    sourceType,
		UnitName:       "TestWebhookReplayProtection",
		UnitBody:       "func TestWebhookReplayProtection(t *testing.T) {\n\trequire.NoError(t, err)\n}",
		UnitLanguage:   "go",
		UnitFramework:  "go test",
		UnitStartLine:  3,
		UnitEndLine:    5,
		UnitSymbols:    []string{"webhook", "replay", "protection"},
		UnitAssertions: []string{"NoError"},
	}

	artifact, sources, _, err := adapter.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, config.KindSourceContext, artifact.Kind)
	assert.Equal(t, config.SubtypeTestCase, artifact.Subtype)
	assert.Equal(t, "3-5", artifact.Extracted["source_line_range"])
	require.Len(t, sources, 1)
	assert.Equal(t, sourceType, sources[0].SourceType)
}

func TestExtractUnits_WithAnnotatedJavaTest_StartsAtAnnotation(t *testing.T) {
	body := "class BillingTest {\n  @Test\n  public void testRejectsReplay() {}\n}\n"

	units := extractUnits("src/test/java/com/example/BillingTest.java", body)

	require.Len(t, units, 1)
	assert.Equal(t, 2, units[0].StartLine)
}

func TestExtractUnits_WithAnnotatedRustTest_StartsAtAnnotation(t *testing.T) {
	body := "#[test]\nfn help_work_invalid_sgconfig() {}\n"

	units := extractUnits("crates/cli/tests/help_test.rs", body)

	require.Len(t, units, 1)
	assert.Equal(t, 1, units[0].StartLine)
}

func assertSingleUnit(t *testing.T, units []testUnit, name, language, framework string) {
	t.Helper()
	require.Len(t, units, 1)
	assert.Equal(t, name, units[0].Name)
	assert.Equal(t, language, units[0].Language)
	assert.Equal(t, framework, units[0].Framework)
	assert.Positive(t, units[0].StartLine)
	assert.GreaterOrEqual(t, units[0].EndLine, units[0].StartLine)
	assert.NotEmpty(t, units[0].Symbols)
	assert.NotEmpty(t, units[0].Assertions)
}

func writeGoTestFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "tests", "webhook_test.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("package tests\n\nfunc TestWebhookReplayProtection(t *testing.T) {\n\trequire.NoError(t, err)\n}\n"), 0o644))
	return root, path
}
