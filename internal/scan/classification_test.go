package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassificationBodyForCandidate_WithTestCase_PrefersUnitBody(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.test.ts")
	art := adapters.Artifact{Body: "fallback full artifact body"}

	actual := classificationBodyForCandidate(adapters.Candidate{
		PrimaryPath: missingPath,
		AdapterName: "test_case",
		UnitBody:    "bounded unit body",
	}, art)

	assert.Equal(t, "bounded unit body", actual)
}

func TestClassificationBodyForCandidate_WithCodeComment_PrefersUnitBody(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.go")
	art := adapters.Artifact{Body: "fallback full artifact body"}

	actual := classificationBodyForCandidate(adapters.Candidate{
		PrimaryPath: missingPath,
		AdapterName: "code_comment",
		UnitBody:    "bounded unit body",
	}, art)

	assert.Equal(t, "bounded unit body", actual)
}

func TestClassificationBodyKeepsWholeFileBehavior(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "plan.md")
	require.NoError(t, os.WriteFile(path, []byte("file body"), 0o644))

	got := classificationBodyForCandidate(adapters.Candidate{
		PrimaryPath: path,
		AdapterName: "markdown",
		UnitBody:    "bounded unit body",
	}, adapters.Artifact{Body: "fallback artifact body"})
	assert.Equal(t, "file body", got)
}
