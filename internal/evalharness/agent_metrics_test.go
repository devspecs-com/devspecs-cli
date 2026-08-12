package evalharness

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGradeArtifactForAgentMetricsMarksSameFamilyMarkdownAsSameCluster(t *testing.T) {
	expected := map[string]string{
		"docs/product-specs/operator-course-visit-analytics.md": "must",
	}
	ctx := sameClusterContext{expectedPaths: expectedPathList(expected)}

	got := gradeArtifactForAgentMetrics("docs/product-specs/index.md", expected, nil, ctx)
	assert.Equal(t, "same_cluster", got.grade)
	assert.Equal(t, 0.5, got.weight)
	assert.True(t, got.sameCluster)
}

func TestGradeArtifactForAgentMetricsMarksLocalizedVariantAsSameCluster(t *testing.T) {
	expected := map[string]string{
		"docs/docs/en/architecture/load-balance.md": "must",
	}
	ctx := sameClusterContext{expectedPaths: expectedPathList(expected)}

	got := gradeArtifactForAgentMetrics("docs/docs/zh/architecture/load-balance.md", expected, nil, ctx)
	assert.Equal(t, "same_cluster", got.grade)
	assert.Equal(t, 0.5, got.weight)
	assert.True(t, got.sameCluster)

}

func TestGradeArtifactForAgentMetricsDoesNotSanitizeDifferentAreaAgentInstruction(t *testing.T) {
	expected := map[string]string{
		"docs/docs/en/architecture/design.md": "must",
	}
	ctx := sameClusterContext{expectedPaths: expectedPathList(expected)}

	got := gradeArtifactForAgentMetrics("dolphinscheduler-alert/CLAUDE.md", expected, nil, ctx)
	assert.Equal(t, "unlabeled", got.grade)
	assert.Zero(t, got.weight)

}

func TestGradeArtifactForAgentMetricsMarksSameDirectoryStemFamilyAsSameCluster(t *testing.T) {
	expected := map[string]string{
		"packages/coding-agent/src/prompts/system/plan-mode-active.md": "must",
	}
	ctx := sameClusterContext{expectedPaths: expectedPathList(expected)}

	got := gradeArtifactForAgentMetrics("packages/coding-agent/src/prompts/system/plan-mode-subagent.md", expected, nil, ctx)
	assert.Equal(t, "same_cluster", got.grade)
	assert.Equal(t, 0.5, got.weight)
	assert.True(t, got.sameCluster)

}

func TestGradeArtifactForAgentMetricsDoesNotMarkArbitrarySameDirectoryDocsAsSameCluster(t *testing.T) {
	expected := map[string]string{
		"packages/coding-agent/src/prompts/system/plan-mode-active.md": "must",
	}
	ctx := sameClusterContext{expectedPaths: expectedPathList(expected)}

	got := gradeArtifactForAgentMetrics("packages/coding-agent/src/prompts/system/error-handling.md", expected, nil, ctx)
	assert.Equal(t, "unlabeled", got.grade)
	assert.Zero(t, got.weight)

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
	assert.Equal(t, "hard_negative", got.grade)
	assert.Equal(t, -1.0, got.weight)
	assert.True(t, got.hardNegative)

}

func TestGradeArtifactForAgentMetricsTreatsFileAndLineAsSameArtifact(t *testing.T) {
	expected := map[string]string{
		"src/auth/session.go": "must",
	}
	ctx := sameClusterContext{expectedPaths: expectedPathList(expected)}

	got := gradeArtifactForAgentMetrics("src/auth/session.go#L24-L39", expected, nil, ctx)
	assert.Equal(t, "must", got.grade)
	assert.Equal(t, 1.0, got.weight)
	assert.True(t, got.exact)

}

func TestGradeArtifactForAgentMetricsKeepsDifferentLineRefsSameCluster(t *testing.T) {
	expected := map[string]string{
		"src/auth/session.go#L24-L39": "must",
	}
	ctx := sameClusterContext{
		lineExpectedBases: map[string]bool{"src/auth/session.go": true},
		expectedPaths:     expectedPathList(expected),
	}

	got := gradeArtifactForAgentMetrics("src/auth/session.go#L60", expected, nil, ctx)
	assert.Equal(t, "same_cluster", got.grade)
	assert.Equal(t, 0.5, got.weight)
	assert.True(t, got.sameCluster)
	assert.False(t, got.exact)

}

func TestClassifyCanonicalLane_WithOrdinaryMarkdown_ReturnsIntent(t *testing.T) {
	file := File{Path: "docs/security/access-control.md", Kind: "markdown_artifact"}

	actual := classifyCanonicalLane(file.Path, file, "")

	assert.Equal(t, CanonicalLaneIntent, actual)
}

func TestClassifyCanonicalLane_WithProtocolSubtype_ReturnsProtocol(t *testing.T) {
	file := File{Path: "AGENTS.md", Kind: "markdown_artifact", Subtype: "agent_instruction"}

	actual := classifyCanonicalLane(file.Path, file, "")

	assert.Equal(t, CanonicalLaneProtocol, actual)
}

func TestClassifyCanonicalLane_WithModelClassifier_ReturnsModel(t *testing.T) {
	file := File{Path: "docs/reference/openapi.md", Metadata: map[string]string{"classifier_mode": "model"}}

	actual := classifyCanonicalLane(file.Path, file, "")

	assert.Equal(t, CanonicalLaneModel, actual)
}

func TestClassifyCanonicalLane_WithTemplateSubtype_ReturnsTemplate(t *testing.T) {
	file := File{Path: ".github/pull_request_template.md", Kind: "markdown_artifact", Subtype: "pull_request_template"}

	actual := classifyCanonicalLane(file.Path, file, "")

	assert.Equal(t, CanonicalLaneTemplate, actual)
}

func TestClassifyCanonicalLane_WithSourceContext_ReturnsSourceContext(t *testing.T) {
	file := File{Path: "internal/controller/failover.go", Kind: "source_context"}

	actual := classifyCanonicalLane(file.Path, file, "")

	assert.Equal(t, CanonicalLaneSourceContext, actual)
}

func TestClassifyCanonicalLane_WithTraceClassifier_ReturnsTrace(t *testing.T) {
	file := File{Path: ".devspecs/traces/work.jsonl", Metadata: map[string]string{"classifier_mode": "trace"}}

	actual := classifyCanonicalLane(file.Path, file, "")

	assert.Equal(t, CanonicalLaneTrace, actual)
}

func TestSummarizeCanonicalLaneMetricsCountsUnknownOnlyAsFallback(t *testing.T) {
	cases := []CaseResult{{
		RelevantIncluded:       []string{"docs/plan.md"},
		MissedExpectedRelevant: []string{"src/auth/session.go"},
		ArtifactGrades: []ArtifactGrade{
			{Path: "docs/plan.md", CanonicalLane: CanonicalLaneIntent, Exact: true, Weight: 1},
			{Path: "AGENTS.md", CanonicalLane: CanonicalLaneProtocol, Grade: "unlabeled"},
		},
	}}
	metrics := summarizeCanonicalLaneMetrics(cases)
	byLane := map[string]LaneMetric{}
	for _, metric := range metrics {
		byLane[metric.Lane] = metric
	}
	require.Len(t, byLane, 7)
	assert.Equal(t, 1, byLane[CanonicalLaneIntent].IncludedArtifacts)
	assert.Equal(t, 1, byLane[CanonicalLaneIntent].ExactRelevantArtifacts)
	assert.Equal(t, 1, byLane[CanonicalLaneProtocol].IncludedArtifacts)
	assert.Zero(t, byLane[CanonicalLaneUnknown].IncludedArtifacts)
}
