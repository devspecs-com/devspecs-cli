package commands

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

func TestApplyFindSourceManifestConsumptionScoutReplacesDocsSource(t *testing.T) {
	matches := []retrieval.Candidate{
		{Path: "docs_src/custom_docs_ui/tutorial001.py", Kind: "source_context", Body: "custom docs ui"},
		{Path: "fastapi/openapi/docs.py", Kind: "source_context", Body: "oauth2 redirect"},
	}
	all := append([]retrieval.Candidate(nil), matches...)
	all = append(all, retrieval.Candidate{
		Path:   "fastapi/applications.py",
		Kind:   "source_context",
		Body:   "swagger ui oauth2 redirect url",
		Source: "fastapi/applications.py",
		Metadata: map[string]string{
			"retrieval_candidate": "source_manifest",
			"source_root_kind":    "module_root",
			"source_role":         "implementation",
		},
	})

	got := applyFindSourceManifestConsumptionScout("serve Swagger UI OAuth2 redirect from a custom docs redirect URL", matches, all)
	if !findPackHasPath(got, "fastapi/applications.py") {
		t.Fatalf("expected manifest implementation replacement, got %#v", retrieval.CandidatePaths(got))
	}
	if findPackHasPath(got, "docs_src/custom_docs_ui/tutorial001.py") {
		t.Fatalf("expected docs tutorial source to be replaced, got %#v", retrieval.CandidatePaths(got))
	}
}

func TestApplyFindSourceManifestConsumptionScoutReservesManifestTest(t *testing.T) {
	matches := []retrieval.Candidate{
		{Path: "fastapi/dependencies/utils.py", Kind: "source_context", Body: "use_cache dependency"},
	}
	all := append([]retrieval.Candidate(nil), matches...)
	all = append(all, retrieval.Candidate{
		Path:    "tests/test_dependency_cache.py",
		Kind:    "source_context",
		Subtype: "test_case",
		Body:    "Test names:\n- test_sub_dependency_cache use_cache false",
		Source:  "tests/test_dependency_cache.py",
		Metadata: map[string]string{
			"retrieval_candidate": "source_manifest",
			"test_name":           "test_sub_dependency_cache",
			"source_role":         "test",
		},
	})

	got := applyFindSourceManifestConsumptionScout("Depends use_cache false rerun dependency", matches, all)
	if !findPackHasPath(got, "tests/test_dependency_cache.py") {
		t.Fatalf("expected manifest test reservation, got %#v", retrieval.CandidatePaths(got))
	}
}

func findPackHasPath(candidates []retrieval.Candidate, path string) bool {
	for _, c := range candidates {
		if c.Path == path {
			return true
		}
	}
	return false
}
