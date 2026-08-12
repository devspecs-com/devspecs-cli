package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateCoverage_WhenCoverageEqualsFloor_ReturnsExactStatementSummary(t *testing.T) {
	profile := strings.NewReader("mode: atomic\nexample/a.go:1.1,2.1 8 1\nexample/b.go:1.1,2.1 2 0\n")

	summary, err := evaluateCoverage(profile, "80.0")

	require.NoError(t, err)
	assert.Equal(t, int64(8), summary.CoveredStatements)
	assert.Equal(t, int64(10), summary.TotalStatements)
}

func TestEvaluateCoverage_WhenRoundedPercentageLooksLikeFloor_ReturnsRegression(t *testing.T) {
	profile := strings.NewReader("mode: atomic\nexample/a.go:1.1,2.1 7999 1\nexample/b.go:1.1,2.1 2001 0\n")

	summary, err := evaluateCoverage(profile, "80.0")

	assert.ErrorContains(t, err, "79.990% (7999/10000 statements) is below 80.0%")
	assert.Equal(t, int64(7999), summary.CoveredStatements)
	assert.Equal(t, int64(10000), summary.TotalStatements)
}

func TestEvaluateCoverage_WhenFloorIsOutsidePercentageRange_ReturnsError(t *testing.T) {
	profile := strings.NewReader("mode: set\nexample/a.go:1.1,2.1 1 1\n")

	summary, err := evaluateCoverage(profile, "101")

	assert.ErrorContains(t, err, "invalid coverage floor")
	assert.Equal(t, coverageSummary{}, summary)
}

func TestReadCoverageProfile_WhenProfileLineIsMalformed_ReturnsLineError(t *testing.T) {
	profile := strings.NewReader("mode: count\nexample/a.go:1.1,2.1 missing\n")

	summary, err := readCoverageProfile(profile)

	assert.ErrorContains(t, err, "line 2")
	assert.Equal(t, coverageSummary{}, summary)
}

func TestRun_WhenProfilePasses_PrintsExactCoverageEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coverage.out")
	require.NoError(t, os.WriteFile(path, []byte("mode: atomic\nexample/a.go:1.1,2.1 4 1\nexample/b.go:1.1,2.1 1 0\n"), 0o644))
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"--profile", path, "--floor", "80.0"}, &stdout, &stderr)

	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "total coverage 80.000% (4/5 statements; floor 80.0%)\n", stdout.String())
	assert.Empty(t, stderr.String())
}
