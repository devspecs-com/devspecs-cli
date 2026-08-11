package initflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/profiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// MergeSelectedProfiles
// ---------------------------------------------------------------------------

func TestMergeSelectedProfiles_cursorAddsPathsAndRules(t *testing.T) {
	base := config.DefaultRepoConfig()
	out, err := MergeSelectedProfiles(base, []string{"cursor"}, nil, nil)
	require.NoError(t, err)

	md := markdownSource(out)
	require.NotNil(t, md,
		"expected markdown source")

	foundPlans := false
	for _, p := range md.Paths {
		if p == ".cursor/plans" {
			foundPlans = true
			break
		}
	}
	require.True(t, foundPlans,
		"paths: %v", md.Paths)

	var sawPlanRule bool
	for _, r := range md.Rules {
		if r.Match == "*.plan.md" && r.Kind == config.KindPlan {
			sawPlanRule = true
			break
		}
	}
	require.True(t, sawPlanRule,
		"rules: %+v", md.Rules)

}

func TestMergeSelectedProfiles_openspecSetsPath(t *testing.T) {
	base := config.DefaultRepoConfig()
	base.Sources = nil
	out, err := MergeSelectedProfiles(base, []string{"openspec"}, nil, nil)
	require.NoError(t, err)

	var osrc *config.SourceConfig
	for i := range out.Sources {
		if out.Sources[i].Type == "openspec" {
			osrc = &out.Sources[i]
			break
		}
	}
	require.NotNil(t, osrc)
	assert.Equal(t, "openspec", osrc.Path)

}

func TestMergeSelectedProfiles_openspecUpdatesExistingPath(t *testing.T) {
	base := &config.RepoConfig{
		Version: 1,
		Sources: []config.SourceConfig{
			{Type: "openspec", Path: "wrong"},
		},
	}
	out, err := MergeSelectedProfiles(base, []string{"openspec"}, nil, nil)
	require.NoError(t, err)

	var osrc *config.SourceConfig
	for i := range out.Sources {
		if out.Sources[i].Type == "openspec" {
			osrc = &out.Sources[i]
			break
		}
	}
	require.NotNil(t, osrc)
	assert.Equal(t, "openspec", osrc.Path)

}

func TestMergeSelectedProfiles_customPathsAndRules(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "plans", "01_step")

	require.NoError(t, os.MkdirAll(sub, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(sub, "README.md"), []byte("# x\n"), 0o644))

	base := config.DefaultRepoConfig()
	rules := []config.SourceRule{{Match: "README.md", Kind: config.KindPlan}}
	out, err := MergeSelectedProfiles(base, nil, []string{"plans"}, rules)
	require.NoError(t, err)

	md := markdownSource(out)
	require.NotNil(t, md,
		"expected markdown")

	found := false
	for _, p := range md.Paths {
		if p == "plans" {
			found = true
			break
		}
	}
	require.True(t, found,
		"paths %v", md.Paths)
	require.NotEmpty(t, md.Rules,
		"expected merged rules")

}

func TestMergeSelectedProfiles_emptySelectionPreservesBase(t *testing.T) {
	base := config.DefaultRepoConfig()
	out, err := MergeSelectedProfiles(base, nil, nil, nil)
	require.NoError(t, err)

	require.Len(t, out.Sources, len(base.Sources),
		"sources len %d vs %d", len(out.Sources), len(base.Sources))

}

func TestProfilesOpenspecIDMergeIntegration(t *testing.T) {
	_, ok := profiles.ByID("openspec")

	assert.True(t, ok)
}

func TestMergeSelectedProfiles_nilBaseUsesDefaults(t *testing.T) {
	out, err := MergeSelectedProfiles(nil, []string{"cursor"}, nil, nil)
	require.NoError(t, err)

	require.Equal(t, 1, out.Version,
		"version %d", out.Version)

	md := markdownSource(out)
	require.NotNil(t, md)
	assert.NotEmpty(t, md.Paths)

}

// ---------------------------------------------------------------------------
// Agent tooling file generation
// ---------------------------------------------------------------------------

func TestGenerateAgentToolFilesCreatesExpectedFiles(t *testing.T) {
	root := t.TempDir()
	tools, err := SelectAgentTools(root, []string{"all"}, false)
	require.NoError(t, err)

	files, err := GenerateAgentToolFiles(root, tools, false)
	require.NoError(t, err)

	require.Len(t, files, 8,
		"generated %d files, want 8: %+v", len(files), files)

	want := map[string]string{
		".agents/skills/ds-task/SKILL.md":  "$ds-task",
		".agents/skills/ds-apply/SKILL.md": "$ds-apply",
		".cursor/commands/ds-task.md":      "/ds-task",
		".cursor/commands/ds-apply.md":     "/ds-apply",
		".claude/skills/ds-task/SKILL.md":  "/ds-task",
		".claude/skills/ds-apply/SKILL.md": "/ds-apply",
		".windsurf/workflows/ds-task.md":   "/ds-task",
		".windsurf/workflows/ds-apply.md":  "/ds-apply",
	}
	for _, file := range files {
		require.Equal(t, "created", file.Status,
			"%s status = %q, want created", file.Path, file.Status)

		expectedInvocation, ok := want[file.Path]
		require.True(t, ok, "unexpected generated file: %+v", file)
		assert.Equal(t, expectedInvocation, file.Invocation, file.Path)
		assertGeneratedFileContains(t, root, file.Path, "DevSpecs")
	}

	taskSkill := readGeneratedFile(t, root, ".agents/skills/ds-task/SKILL.md")
	for _, wantText := range []string{"name: ds-task", "ds task", "ds apply <task-id>", "ds recent", "ds find", "Work exactly one slice", "decision gate", "ds task slice add"} {
		require.Contains(t, taskSkill, wantText,
			"codex task skill missing %q:\n%s", wantText, taskSkill)

	}
	applyWorkflow := readGeneratedFile(t, root, ".windsurf/workflows/ds-apply.md")
	for _, wantText := range []string{"ds apply", "unambiguous next slice", "ds recent", "ds find", "Stop after the decision gate"} {
		require.Contains(t, applyWorkflow, wantText,
			"windsurf apply workflow missing %q:\n%s", wantText, applyWorkflow)

	}
	assert.NotContains(t, applyWorkflow, "not available yet")
	assert.NotContains(t, applyWorkflow, "ds task next")

	for _, rel := range []string{
		".agents/skills/ds-apply/SKILL.md",
		".cursor/commands/ds-apply.md",
		".claude/skills/ds-apply/SKILL.md",
		".windsurf/workflows/ds-apply.md",
	} {
		body := readGeneratedFile(t, root, rel)
		for _, wantText := range []string{"ds apply", "promote", "improve", "rework", "rollback", "block"} {
			require.Contains(t, body, wantText,
				"%s missing %q:\n%s", rel, wantText, body)

		}
	}
	claudeSkill := readGeneratedFile(t, root, ".claude/skills/ds-task/SKILL.md")
	require.NotContains(t, claudeSkill, "allowed-tools",
		"claude skill should not include speculative allowed-tools metadata:\n%s", claudeSkill)

}

func TestGenerateAgentToolFilesPreservesExistingWithoutForce(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, ".cursor", "commands", "ds-task.md")

	require.NoError(t, os.MkdirAll(filepath.Dir(custom), 0o755))

	require.NoError(t, os.WriteFile(custom, []byte("# custom\n"), 0o644))

	tools, err := SelectAgentTools(root, []string{"cursor"}, false)
	require.NoError(t, err)

	files, err := GenerateAgentToolFiles(root, tools, false)
	require.NoError(t, err)

	assert.Equal(t, "skipped-existing", generatedStatus(files, ".cursor/commands/ds-task.md"))
	assert.Equal(t, "# custom\n", readGeneratedFile(t, root, ".cursor/commands/ds-task.md"))
	assert.Equal(t, "created", generatedStatus(files, ".cursor/commands/ds-apply.md"))
}

func TestGenerateAgentToolFilesForceOverwritesExisting(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, ".cursor", "commands", "ds-task.md")

	require.NoError(t, os.MkdirAll(filepath.Dir(custom), 0o755))

	require.NoError(t, os.WriteFile(custom, []byte("# custom\n"), 0o644))

	tools, err := SelectAgentTools(root, []string{"cursor"}, false)
	require.NoError(t, err)

	files, err := GenerateAgentToolFiles(root, tools, true)
	require.NoError(t, err)

	assert.Equal(t, "overwritten", generatedStatus(files, ".cursor/commands/ds-task.md"))
	assertGeneratedFileContains(t, root, ".cursor/commands/ds-task.md", "Work exactly one slice")
}

func generatedStatus(files []AgentToolFile, relPath string) string {
	for _, file := range files {
		if file.Path == relPath {
			return file.Status
		}
	}
	return ""
}

func TestSelectAgentToolsByID_WithRequestedSubset_PreservesDetectionOrder(t *testing.T) {
	tools := []AgentTool{
		{ID: "codex", Label: "Codex"},
		{ID: "cursor", Label: "Cursor"},
		{ID: "claude", Label: "Claude"},
	}

	selected := selectAgentToolsByID(tools, []string{"claude", "codex"})

	require.Len(t, selected, 2)
	assert.Equal(t, "codex", selected[0].ID)
	assert.Equal(t, "claude", selected[1].ID)
}

func TestFilterAgentTools_WithDetectedPredicate_ReturnsOnlyDetectedTools(t *testing.T) {
	tools := []AgentTool{
		{ID: "codex", Detected: true},
		{ID: "cursor", Detected: false},
		{ID: "claude", Detected: true},
	}

	selected := filterAgentTools(tools, func(tool AgentTool) bool {
		return tool.Detected
	})

	require.Len(t, selected, 2)
	assert.Equal(t, "codex", selected[0].ID)
	assert.Equal(t, "claude", selected[1].ID)
}

func assertGeneratedFileContains(t *testing.T, root, relPath, want string) {
	t.Helper()
	data := readGeneratedFile(t, root, relPath)
	require.Contains(t, data, want,
		"%s missing %q:\n%s", relPath, want, data)

}

func readGeneratedFile(t *testing.T, root, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath)))
	require.NoError(t, err)

	return string(data)
}

func TestMergeSelectedProfiles_adrAddsPaths(t *testing.T) {
	base := config.DefaultRepoConfig()
	out, err := MergeSelectedProfiles(base, []string{"adr"}, nil, nil)
	require.NoError(t, err)

	ad := adrSource(out)
	require.NotNil(t, ad,
		"expected adr source")

	found := false
	for _, p := range ad.Paths {
		if p == "docs/adr" {
			found = true
			break
		}
	}
	require.True(t, found,
		"paths %v", ad.Paths)

}

func TestMergeSelectedProfiles_invalidCustomRules(t *testing.T) {
	base := config.DefaultRepoConfig()
	_, err := MergeSelectedProfiles(base, nil, []string{"plans"}, []config.SourceRule{
		{Match: "*.md", Kind: "invalid_kind"},
	})
	require.Error(t, err,
		"expected validation error")

}

func TestMergeSelectedProfiles_unknownProfileIDSkipped(t *testing.T) {
	base := config.DefaultRepoConfig()
	out, err := MergeSelectedProfiles(base, []string{"cursor", "___unknown___"}, nil, nil)
	require.NoError(t, err)

	md := markdownSource(out)
	require.NotNil(t, md,
		"expected markdown")

	var sawCursor bool
	for _, p := range md.Paths {
		if p == ".cursor/plans" {
			sawCursor = true
			break
		}
	}
	require.True(t, sawCursor,
		"paths %v", md.Paths)

}

func adrSource(cfg *config.RepoConfig) *config.SourceConfig {
	for i := range cfg.Sources {
		if cfg.Sources[i].Type == "adr" {
			return &cfg.Sources[i]
		}
	}
	return nil
}

func TestMergeSelectedProfiles_speckitRules(t *testing.T) {
	base := config.DefaultRepoConfig()
	out, err := MergeSelectedProfiles(base, []string{"speckit"}, nil, nil)
	require.NoError(t, err)

	md := markdownSource(out)
	require.NotNil(t, md,
		"expected markdown")

	var saw bool
	for _, r := range md.Rules {
		if r.Match == "*/spec.md" && r.Kind == config.KindSpec {
			saw = true
			break
		}
	}
	require.True(t, saw,
		"rules %+v", md.Rules)

}

func TestMergeSelectedProfiles_bmadPRDRule(t *testing.T) {
	base := config.DefaultRepoConfig()
	out, err := MergeSelectedProfiles(base, []string{"bmad"}, nil, nil)
	require.NoError(t, err)

	md := markdownSource(out)
	var saw bool
	for _, r := range md.Rules {
		if r.Kind == config.KindRequirements && r.Subtype == config.SubtypePRD {
			saw = true
			break
		}
	}
	require.True(t, saw,
		"rules %+v", md.Rules)

}

// ---------------------------------------------------------------------------
// DetectPatterns
// ---------------------------------------------------------------------------

func TestDetectPatterns_readme(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "v2", "plans")

	require.NoError(t, os.MkdirAll(dir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("#\n"), 0o644))

	pats := DetectPatterns(root, "v2/plans")
	require.NotEmpty(t, pats,
		"expected patterns")

	var sawReadme bool
	for _, p := range pats {
		if strings.Contains(p.Match, "README") {
			sawReadme = true
			assert.Equal(t, 1, p.FileCount,
				"README.md FileCount: want 1, got %d", p.FileCount)
			assert.Equal(t, config.KindPlan, p.DefaultKind,
				"README.md DefaultKind: want plan, got %s", p.DefaultKind)

			break
		}
	}
	require.True(t, sawReadme,
		"patterns: %+v", pats)

}

func TestDetectPatterns_numberedAndNested(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "plans")

	require.NoError(t, os.MkdirAll(dir, 0o755))

	for _, n := range []string{"02_FOO.md", "03_BAR.md", "04_BAZ.md"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("# "+n+"\n"), 0o644); err != nil {
			require.NoError(t, err)
		}
	}
	sub := filepath.Join(dir, "01-setup")

	require.NoError(t, os.MkdirAll(sub, 0o755))

	for _, n := range []string{"README.md", "01-first.md", "02-second.md"} {
		if err := os.WriteFile(filepath.Join(sub, n), []byte("# x\n"), 0o644); err != nil {
			require.NoError(t, err)
		}
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "ROADMAP.md"), []byte("# roadmap\n"), 0o644))

	pats := DetectPatterns(root, "plans")
	counts := make(map[string]int)
	kinds := make(map[string]string)
	for _, p := range pats {
		counts[p.Match] = p.FileCount
		kinds[p.Match] = p.DefaultKind
	}
	assert.Equal(t, 3, counts["[0-9][0-9]_*.md"],
		"[0-9][0-9]_*.md: want 3, got %d", counts["[0-9][0-9]_*.md"])
	assert.Equal(t, 1, counts["*/README.md"],
		"*/README.md: want 1, got %d", counts["*/README.md"])
	assert.Equal(t, 2, counts["*/[0-9][0-9]-*.md"],
		"*/[0-9][0-9]-*.md: want 2, got %d", counts["*/[0-9][0-9]-*.md"])
	assert.Equal(t, 1, counts["ROADMAP.md"],
		"ROADMAP.md individual: want 1, got %d", counts["ROADMAP.md"])
	assert.Equal(t, config.KindPlan, kinds["ROADMAP.md"],
		"ROADMAP.md kind: want plan, got %s", kinds["ROADMAP.md"])

}

func TestDetectPatterns_threeDigitDecisions(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "decisions")

	require.NoError(t, os.MkdirAll(dir, 0o755))

	for _, n := range []string{"001-auth.md", "002-storage.md"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("# x\n"), 0o644); err != nil {
			require.NoError(t, err)
		}
	}

	pats := DetectPatterns(root, "decisions")
	var found *DetectedPattern
	for i := range pats {
		if pats[i].Match == "[0-9][0-9][0-9]-*.md" {
			found = &pats[i]
			break
		}
	}
	require.NotNil(t, found,
		"expected [0-9][0-9][0-9]-*.md, got %+v", pats)
	assert.Equal(t, 2, found.FileCount,
		"FileCount: want 2, got %d", found.FileCount)
	assert.Equal(t, config.KindDecision, found.DefaultKind,
		"DefaultKind: want decision, got %s", found.DefaultKind)

}

func TestDetectPatterns_individualFileKindInference(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs")

	require.NoError(t, os.MkdirAll(dir, 0o755))

	for _, n := range []string{"ROADMAP.md", "spec-auth.md", "design-system.md", "notes.md"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("# x\n"), 0o644); err != nil {
			require.NoError(t, err)
		}
	}

	pats := DetectPatterns(root, "docs")
	kinds := make(map[string]string)
	for _, p := range pats {
		kinds[p.Match] = p.DefaultKind
	}
	assert.Equal(t, config.KindPlan, kinds["ROADMAP.md"],
		"ROADMAP.md: want plan, got %s", kinds["ROADMAP.md"])
	assert.Equal(t, config.KindSpec, kinds["spec-auth.md"],
		"spec-auth.md: want spec, got %s", kinds["spec-auth.md"])
	assert.Equal(t, config.KindDesign, kinds["design-system.md"],
		"design-system.md: want design, got %s", kinds["design-system.md"])
	assert.Equal(t, config.KindMarkdownArtifact, kinds["notes.md"],
		"notes.md: want markdown_artifact, got %s", kinds["notes.md"])

}

func TestDetectPatterns_emptyDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "empty")

	require.NoError(t, os.MkdirAll(dir, 0o755))

	pats := DetectPatterns(root, "empty")
	assert.Empty(t, pats,
		"expected nil, got %+v", pats)

}

func TestDetectPatterns_missingSourceDir(t *testing.T) {
	root := t.TempDir()

	patterns := DetectPatterns(root, "no/such/dir")

	assert.Empty(t, patterns)
}

func TestInferDirKind_WithDecisionsDirectory_ReturnsDecision(t *testing.T) {
	actual := inferDirKind("decisions")

	assert.Equal(t, config.KindDecision, actual)
}

func TestInferDirKind_WithADRDirectory_ReturnsDecision(t *testing.T) {
	actual := inferDirKind("docs/adr")

	assert.Equal(t, config.KindDecision, actual)
}

func TestInferDirKind_WithSpecsDirectory_ReturnsSpec(t *testing.T) {
	actual := inferDirKind("specs")

	assert.Equal(t, config.KindSpec, actual)
}

func TestInferDirKind_WithDesignDirectory_ReturnsDesign(t *testing.T) {
	actual := inferDirKind("v2/design")

	assert.Equal(t, config.KindDesign, actual)
}

func TestInferDirKind_WithRequirementsDirectory_ReturnsRequirements(t *testing.T) {
	actual := inferDirKind("requirements")

	assert.Equal(t, config.KindRequirements, actual)
}

func TestInferDirKind_WithPlansDirectory_ReturnsPlan(t *testing.T) {
	actual := inferDirKind("v2/plans")

	assert.Equal(t, config.KindPlan, actual)
}

func TestInferDirKind_WithRootDirectory_ReturnsPlan(t *testing.T) {
	actual := inferDirKind(".")

	assert.Equal(t, config.KindPlan, actual)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func markdownSource(cfg *config.RepoConfig) *config.SourceConfig {
	for i := range cfg.Sources {
		if cfg.Sources[i].Type == "markdown" {
			return &cfg.Sources[i]
		}
	}
	return nil
}
