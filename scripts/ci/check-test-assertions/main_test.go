package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountDirectAssertions_WhenTestingMethodsAreCalled_CountsEachCall(t *testing.T) {
	parsed := mustParseTestSource(t, `package sample
import "testing"
func TestSample(t *testing.T) {
	t.Error("first")
	t.Fatalf("second")
}
`)

	actual := countDirectAssertions(parsed)

	assert.Equal(t, 2, actual)
}

func TestCountDirectAssertions_WhenErrorMethodBelongsToNonTestingValue_DoesNotCountCall(t *testing.T) {
	parsed := mustParseTestSource(t, `package sample
import "testing"
func TestSample(t *testing.T) {
	err := example()
	_ = err.Error()
}
`)

	actual := countDirectAssertions(parsed)

	assert.Equal(t, 0, actual)
}

func TestCountDirectAssertions_WhenAssertionNameAppearsInString_DoesNotCountIt(t *testing.T) {
	parsed := mustParseTestSource(t, `package sample
import "testing"
func TestSample(t *testing.T) {
	_ = "t.Fatal is fixture source, not a call"
}
`)

	actual := countDirectAssertions(parsed)

	assert.Equal(t, 0, actual)
}

func TestCountDirectAssertions_WhenTestingHandleIsHelperParameter_CountsCall(t *testing.T) {
	parsed := mustParseTestSource(t, `package sample
import "testing"
func helper(testingHandle *testing.T) { testingHandle.FailNow() }
`)

	actual := countDirectAssertions(parsed)

	assert.Equal(t, 1, actual)
}

func TestCountDirectAssertions_WhenTestingHandleIsSubtestParameter_CountsCall(t *testing.T) {
	parsed := mustParseTestSource(t, `package sample
import "testing"
func TestSample(t *testing.T) {
	t.Run("child", func(child *testing.T) { child.Errorf("failure") })
}
`)

	actual := countDirectAssertions(parsed)

	assert.Equal(t, 1, actual)
}

func TestCompareCounts_WhenCountIncreases_ReturnsRegression(t *testing.T) {
	baseline := map[string]int{"sample_test.go": 2}
	current := map[string]int{"sample_test.go": 3}

	problems := compareCounts(current, baseline)

	require.Len(t, problems, 1)
	assert.Equal(t, "sample_test.go: direct testing assertions increased from 2 to 3", problems[0])
}

func TestCompareCounts_WhenCountDecreases_RequiresBaselineRatchet(t *testing.T) {
	baseline := map[string]int{"sample_test.go": 2}
	current := map[string]int{"sample_test.go": 1}

	problems := compareCounts(current, baseline)

	require.Len(t, problems, 1)
	assert.Equal(t, "sample_test.go: direct testing assertions fell from 2 to 1; ratchet the baseline", problems[0])
}

func TestCompareCounts_WhenCountsMatch_ReturnsNoProblems(t *testing.T) {
	baseline := map[string]int{"sample_test.go": 2}
	current := map[string]int{"sample_test.go": 2}

	problems := compareCounts(current, baseline)

	assert.Empty(t, problems)
}

func TestCollectDirectAssertions_WhenTestFileContainsDirectAssertion_ReportsFileCount(t *testing.T) {
	root := t.TempDir()
	writeTestSource(t, filepath.Join(root, "sample_test.go"), `package sample
import "testing"
func TestSample(t *testing.T) { t.Fatal("counted") }
`)

	counts, err := collectDirectAssertions(root)

	require.NoError(t, err)
	require.Len(t, counts, 1)
	assert.Equal(t, 1, counts["sample_test.go"])
}

func TestCollectDirectAssertions_WhenAssertionNameAppearsOnlyInString_OmitsFile(t *testing.T) {
	root := t.TempDir()
	writeTestSource(t, filepath.Join(root, "sample_test.go"), `package sample
import "testing"
func TestSample(t *testing.T) { _ = "t.Fatalf is fixture source" }
`)

	counts, err := collectDirectAssertions(root)

	require.NoError(t, err)
	assert.Empty(t, counts)
}

func TestCollectDirectAssertions_WhenTestFileIsInIgnoredDirectory_OmitsFile(t *testing.T) {
	root := t.TempDir()
	ignoredDir := filepath.Join(root, "_ignore")
	require.NoError(t, os.MkdirAll(ignoredDir, 0o755))
	writeTestSource(t, filepath.Join(ignoredDir, "ignored_test.go"), `package ignored
import "testing"
func TestIgnored(t *testing.T) { t.Fatal("ignored") }
`)

	counts, err := collectDirectAssertions(root)

	require.NoError(t, err)
	assert.Empty(t, counts)
}

func TestLoadBaseline_WhenJSONIsValid_ReturnsBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	writeTestSource(t, path, `{
  "version": 1,
  "counts": {
    "a_test.go": 2,
    "b_test.go": 1
  }
}`)

	baseline, err := loadBaseline(path)

	require.NoError(t, err)
	assert.Equal(t, 1, baseline.Version)
	require.Len(t, baseline.Counts, 2)
	assert.Equal(t, 2, baseline.Counts["a_test.go"])
	assert.Equal(t, 1, baseline.Counts["b_test.go"])
}

func TestWriteBaseline_WhenCountsProvided_WritesVersionedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "baseline.json")
	counts := map[string]int{"sample_test.go": 2}

	err := writeBaseline(path, counts)

	require.NoError(t, err)
	assert.JSONEq(t, `{
  "version": 1,
  "counts": {
    "sample_test.go": 2
  }
}`, readTestSource(t, path))
}

func mustParseTestSource(t *testing.T, source string) *ast.File {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), "sample_test.go", source, 0)
	require.NoError(t, err)
	return parsed
}

func writeTestSource(t *testing.T, path, source string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(source), 0o644))
}

func readTestSource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
