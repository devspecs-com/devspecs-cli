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

func TestGradeArtifactForAgentMetricsTreatsFileAndLineAsSameArtifact(t *testing.T) {
	expected := map[string]string{
		"src/auth/session.go": "must",
	}
	ctx := sameClusterContext{expectedPaths: expectedPathList(expected)}

	got := gradeArtifactForAgentMetrics("src/auth/session.go#L24-L39", expected, nil, ctx)
	if got.grade != "must" || got.weight != 1 || !got.exact {
		t.Fatalf("file expectation should accept line-scoped artifact exactly, got %#v", got)
	}
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
	if got.grade != "same_cluster" || got.weight != 0.5 || !got.sameCluster || got.exact {
		t.Fatalf("different line refs in the same file should be same-cluster, got %#v", got)
	}
}

func TestClassifyCanonicalLanePrefersConcreteDocLanes(t *testing.T) {
	tests := []struct {
		name string
		path string
		file File
		want string
	}{
		{
			name: "ordinary markdown defaults to intent",
			path: "docs/security/access-control.md",
			file: File{Path: "docs/security/access-control.md", Kind: "markdown_artifact"},
			want: CanonicalLaneIntent,
		},
		{
			name: "protocol subtype",
			path: "AGENTS.md",
			file: File{Path: "AGENTS.md", Kind: "markdown_artifact", Subtype: "agent_instruction"},
			want: CanonicalLaneProtocol,
		},
		{
			name: "model classifier",
			path: "docs/reference/openapi.md",
			file: File{Path: "docs/reference/openapi.md", Metadata: map[string]string{"classifier_mode": "model"}},
			want: CanonicalLaneModel,
		},
		{
			name: "template subtype",
			path: ".github/pull_request_template.md",
			file: File{Path: ".github/pull_request_template.md", Kind: "markdown_artifact", Subtype: "pull_request_template"},
			want: CanonicalLaneTemplate,
		},
		{
			name: "source context",
			path: "internal/controller/failover.go",
			file: File{Path: "internal/controller/failover.go", Kind: "source_context"},
			want: CanonicalLaneSourceContext,
		},
		{
			name: "trace classifier",
			path: ".devspecs/traces/work.jsonl",
			file: File{Path: ".devspecs/traces/work.jsonl", Metadata: map[string]string{"classifier_mode": "trace"}},
			want: CanonicalLaneTrace,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyCanonicalLane(tc.path, tc.file, ""); got != tc.want {
				t.Fatalf("classifyCanonicalLane() = %q, want %q", got, tc.want)
			}
		})
	}
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
	if byLane[CanonicalLaneIntent].IncludedArtifacts != 1 || byLane[CanonicalLaneIntent].ExactRelevantArtifacts != 1 {
		t.Fatalf("intent lane metrics wrong: %#v", byLane[CanonicalLaneIntent])
	}
	if byLane[CanonicalLaneProtocol].IncludedArtifacts != 1 {
		t.Fatalf("protocol lane metrics wrong: %#v", byLane[CanonicalLaneProtocol])
	}
	if byLane[CanonicalLaneUnknown].IncludedArtifacts != 0 {
		t.Fatalf("unknown lane should be fallback only: %#v", byLane[CanonicalLaneUnknown])
	}
}
