package format

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalize_WithEmptyProfile_ReturnsGeneric(t *testing.T) {
	actual := Normalize("")

	assert.Equal(t, ProfileGeneric, actual)
}

func TestNormalize_WithUppercaseCursorPlan_ReturnsCursorPlan(t *testing.T) {
	actual := Normalize("CURSOR_PLAN")

	assert.Equal(t, ProfileCursorPlan, actual)
}

func TestNormalize_WithCursorAlias_ReturnsCursorPlan(t *testing.T) {
	actual := Normalize("cursor")

	assert.Equal(t, ProfileCursorPlan, actual)
}

func TestNormalize_WithSpecKit_ReturnsSpecKit(t *testing.T) {
	actual := Normalize("speckit")

	assert.Equal(t, ProfileSpeckit, actual)
}

func TestNormalize_WithBmadMethod_ReturnsBmad(t *testing.T) {
	actual := Normalize("bmad-method")

	assert.Equal(t, ProfileBmad, actual)
}

func TestNormalize_WithOpenSpec_ReturnsOpenSpec(t *testing.T) {
	actual := Normalize("openspec")

	assert.Equal(t, ProfileOpenspec, actual)
}

func TestNormalize_WithADR_ReturnsADR(t *testing.T) {
	actual := Normalize("adr")

	assert.Equal(t, ProfileADR, actual)
}

func TestNormalize_WithUnknownProfile_ReturnsGeneric(t *testing.T) {
	actual := Normalize("unknown-thing")

	assert.Equal(t, ProfileGeneric, actual)
}

func TestFromPath_WithGenericPlan_ReturnsGeneric(t *testing.T) {
	actual := FromPath("plans/foo.md")

	assert.Equal(t, ProfileGeneric, actual)
}

func TestFromPath_WithCursorPlan_ReturnsCursorPlan(t *testing.T) {
	actual := FromPath(".cursor/plans/x.md")

	assert.Equal(t, ProfileCursorPlan, actual)
}

func TestFromPath_WithSpecKitSpec_ReturnsSpecKit(t *testing.T) {
	actual := FromPath("specs/001-fe/spec.md")

	assert.Equal(t, ProfileSpeckit, actual)
}

func TestFromPath_WithSpecKitPlan_ReturnsSpecKit(t *testing.T) {
	actual := FromPath("specs/001-fe/plan.md")

	assert.Equal(t, ProfileSpeckit, actual)
}

func TestFromPath_WithSpecKitTasks_ReturnsSpecKit(t *testing.T) {
	actual := FromPath("specs/001-fe/tasks.md")

	assert.Equal(t, ProfileSpeckit, actual)
}

func TestFromPath_WithBmadArtifact_ReturnsBmad(t *testing.T) {
	actual := FromPath("_bmad-output/planning-artifacts/prd.md")

	assert.Equal(t, ProfileBmad, actual)
}

func TestFromPath_WithClaudeNote_ReturnsClaude(t *testing.T) {
	actual := FromPath(".claude/notes/handoff.md")

	assert.Equal(t, ProfileClaude, actual)
}

func TestFromPath_WithCodexPlan_ReturnsCodex(t *testing.T) {
	actual := FromPath(".codex/plans/PLAN.md")

	assert.Equal(t, ProfileCodex, actual)
}

func TestFromFrontmatterTool_WithSpecKitGenerator_ReturnsSpecKit(t *testing.T) {
	actual := FromFrontmatterTool("", "Spec Kit", "")

	assert.Equal(t, ProfileSpeckit, actual)
}

func TestFromFrontmatterTool_WithCursorTool_ReturnsCursorPlan(t *testing.T) {
	actual := FromFrontmatterTool("cursor desktop")

	assert.Equal(t, ProfileCursorPlan, actual)
}

func TestLayoutGroup_WithSpecKitSpec_ReturnsFeatureRoot(t *testing.T) {
	actual := LayoutGroup("specs/001-synthetic-feature/spec.md")

	assert.Equal(t, "specs/001-synthetic-feature", actual)
}

func TestLayoutGroup_WithSpecKitPlan_ReturnsFeatureRoot(t *testing.T) {
	actual := LayoutGroup("specs/001-synthetic-feature/plan.md")

	assert.Equal(t, "specs/001-synthetic-feature", actual)
}

func TestLayoutGroup_WithNestedContract_ReturnsFeatureRoot(t *testing.T) {
	actual := LayoutGroup("specs/001-synthetic-feature/contracts/api.md")

	assert.Equal(t, "specs/001-synthetic-feature", actual)
}

func TestLayoutGroup_WithBmadArtifact_ReturnsPlanningRoot(t *testing.T) {
	actual := LayoutGroup("_bmad-output/planning-artifacts/prd.md")

	assert.Equal(t, "_bmad-output/planning-artifacts", actual)
}
