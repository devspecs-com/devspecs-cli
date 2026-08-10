package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountDirectAssertionsFindsTestingCallsOnly(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "sample_test.go", `package sample
import "testing"
func TestSample(t *testing.T) {
	err := example()
	_ = err.Error()
	t.Error("first")
	t.Fatalf("second")
	_ = "t.Fatal is fixture source, not a call"
}
`, 0)
	require.NoError(t, err)

	assert.Equal(t, 2, countDirectAssertions(parsed))
}

func TestCountDirectAssertionsSupportsHelpersAndSubtests(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "sample_test.go", `package sample
import "testing"
func helper(testingHandle *testing.T) { testingHandle.FailNow() }
func TestSample(t *testing.T) {
	t.Run("child", func(child *testing.T) { child.Errorf("failure") })
}
`, 0)
	require.NoError(t, err)

	assert.Equal(t, 2, countDirectAssertions(parsed))
}

func TestCompareCountsRejectsIncreasesAndRequiresRatchet(t *testing.T) {
	problems := compareCounts(
		map[string]int{"increased_test.go": 3, "reduced_test.go": 1},
		map[string]int{"increased_test.go": 2, "reduced_test.go": 2},
	)

	assert.Equal(t, []string{
		"increased_test.go: direct testing assertions increased from 2 to 3",
		"reduced_test.go: direct testing assertions fell from 2 to 1; ratchet the baseline",
	}, problems)
}

func TestCompareCountsAcceptsExactBaseline(t *testing.T) {
	counts := map[string]int{"sample_test.go": 2}
	assert.Empty(t, compareCounts(counts, counts))
}

func TestCollectDirectAssertionsSkipsIgnoredTreesAndFixtureStrings(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "sample_test.go"), []byte(`package sample
import "testing"
func TestSample(t *testing.T) {
	t.Fatal("counted")
	_ = "t.Fatalf is fixture source"
}
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "_ignore"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "_ignore", "ignored_test.go"), []byte(`package ignored
import "testing"
func TestIgnored(t *testing.T) { t.Fatal("ignored") }
`), 0o644))

	counts, err := collectDirectAssertions(root)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"sample_test.go": 1}, counts)
}

func TestBaselineRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	want := map[string]int{"a_test.go": 2, "b_test.go": 1}
	require.NoError(t, writeBaseline(path, want))

	got, err := loadBaseline(path)
	require.NoError(t, err)
	assert.Equal(t, assertionBaseline{Version: 1, Counts: want}, got)
}
