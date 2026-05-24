package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
)

func TestClassificationBodyPrefersUnitBodyForBoundedArtifacts(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.test.ts")
	art := adapters.Artifact{Body: "fallback full artifact body"}

	for _, tc := range []struct {
		name        string
		adapterName string
	}{
		{name: "test case", adapterName: "test_case"},
		{name: "code comment", adapterName: "code_comment"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classificationBodyForCandidate(adapters.Candidate{
				PrimaryPath: missingPath,
				AdapterName: tc.adapterName,
				UnitBody:    "bounded unit body",
			}, art)
			if got != "bounded unit body" {
				t.Fatalf("classification body = %q, want unit body", got)
			}
		})
	}
}

func TestClassificationBodyKeepsWholeFileBehavior(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "plan.md")
	if err := os.WriteFile(path, []byte("file body"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := classificationBodyForCandidate(adapters.Candidate{
		PrimaryPath: path,
		AdapterName: "markdown",
		UnitBody:    "bounded unit body",
	}, adapters.Artifact{Body: "fallback artifact body"})
	if got != "file body" {
		t.Fatalf("classification body = %q, want file body", got)
	}
}
