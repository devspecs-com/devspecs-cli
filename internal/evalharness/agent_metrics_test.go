package evalharness

import "testing"

func TestGradeArtifactForAgentMetricsMarksSameFamilyMarkdownAsSameCluster(t *testing.T) {
	expected := map[string]string{
		"docs/product-specs/operator-course-visit-analytics.md": "must",
	}
	ctx := sameClusterContext{expectedPaths: expectedPathList(expected)}

	got := gradeArtifactForAgentMetrics("docs/product-specs/index.md", expected, nil, ctx)
	if got.grade != "same_cluster" || got.weight != 0.5 || !got.sameCluster {
		t.Fatalf("expected same-cluster product-spec index, got %#v", got)
	}
}

func TestGradeArtifactForAgentMetricsMarksLocalizedVariantAsSameCluster(t *testing.T) {
	expected := map[string]string{
		"docs/docs/en/architecture/load-balance.md": "must",
	}
	ctx := sameClusterContext{expectedPaths: expectedPathList(expected)}

	got := gradeArtifactForAgentMetrics("docs/docs/zh/architecture/load-balance.md", expected, nil, ctx)
	if got.grade != "same_cluster" || got.weight != 0.5 || !got.sameCluster {
		t.Fatalf("expected same-cluster localized architecture doc, got %#v", got)
	}
}

func TestGradeArtifactForAgentMetricsDoesNotSanitizeDifferentAreaAgentInstruction(t *testing.T) {
	expected := map[string]string{
		"docs/docs/en/architecture/design.md": "must",
	}
	ctx := sameClusterContext{expectedPaths: expectedPathList(expected)}

	got := gradeArtifactForAgentMetrics("dolphinscheduler-alert/CLAUDE.md", expected, nil, ctx)
	if got.grade != "unlabeled" || got.weight != 0 {
		t.Fatalf("different-area agent instruction should remain unlabeled, got %#v", got)
	}
}

func TestGradeArtifactForAgentMetricsMarksSameDirectoryStemFamilyAsSameCluster(t *testing.T) {
	expected := map[string]string{
		"packages/coding-agent/src/prompts/system/plan-mode-active.md": "must",
	}
	ctx := sameClusterContext{expectedPaths: expectedPathList(expected)}

	got := gradeArtifactForAgentMetrics("packages/coding-agent/src/prompts/system/plan-mode-subagent.md", expected, nil, ctx)
	if got.grade != "same_cluster" || got.weight != 0.5 || !got.sameCluster {
		t.Fatalf("expected same-cluster plan-mode sibling, got %#v", got)
	}
}

func TestGradeArtifactForAgentMetricsDoesNotMarkArbitrarySameDirectoryDocsAsSameCluster(t *testing.T) {
	expected := map[string]string{
		"packages/coding-agent/src/prompts/system/plan-mode-active.md": "must",
	}
	ctx := sameClusterContext{expectedPaths: expectedPathList(expected)}

	got := gradeArtifactForAgentMetrics("packages/coding-agent/src/prompts/system/error-handling.md", expected, nil, ctx)
	if got.grade != "unlabeled" || got.weight != 0 {
		t.Fatalf("arbitrary same-directory markdown should remain unlabeled, got %#v", got)
	}
}

func TestGradeArtifactForAgentMetricsHardNegativeWinsOverSameFamily(t *testing.T) {
	expected := map[string]string{
		"docs/design/current.md": "must",
	}
	hardNegatives := map[string]bool{
		"docs/design/obsolete.md": true,
	}
	ctx := sameClusterContext{expectedPaths: expectedPathList(expected)}

	got := gradeArtifactForAgentMetrics("docs/design/obsolete.md", expected, hardNegatives, ctx)
	if got.grade != "hard_negative" || got.weight != -1 || !got.hardNegative {
		t.Fatalf("hard negative should win over same-family sanitation, got %#v", got)
	}
}
