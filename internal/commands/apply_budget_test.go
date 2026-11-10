package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyJSON_WithTargetPlan_AddsBudgetWithoutReplacingContext(t *testing.T) {
	// Arrange
	repo := setupApplyTask(t, "apply-budget", "A", "Inspect evidence")
	ctx, err := loadTaskTargetContextForRepo(defaultTaskWorkspaceDir, "apply-budget", "A01", repo)
	require.NoError(t, err)
	plan := "# Plan\n\nInspect caf\u00e9.\n"
	require.NoError(t, os.WriteFile(filepath.Join(ctx.Workspace, ctx.Slice.Plan), []byte(plan), 0o600))
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"apply-budget", "--target", "A01", "--json"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	// Act
	err = cmd.Execute()

	// Assert
	require.NoError(t, err)
	out := decodeApplyPromptOutput(t, &stdout)
	assert.Equal(t, "A01", out.Target)
	assert.Equal(t, plan, out.TargetContext.PlanBody)
	assert.Contains(t, out.Prompt, plan)
	assert.Equal(t, 1, out.InputBudget.SchemaVersion)
	assert.Equal(t, len(out.Prompt), out.InputBudget.PromptBytes)
	assert.Equal(t, (len(out.Prompt)+3)/4, out.InputBudget.PromptTokenEstimate)
	assert.Equal(t, "ceil_utf8_bytes_div_4", out.InputBudget.TokenEstimateMethod)
	require.Len(t, out.InputBudget.ContextArtifacts, 1)
	artifact := out.InputBudget.ContextArtifacts[0]
	assert.Equal(t, "/target_context/plan_body", artifact.JSONPointer)
	assert.Equal(t, 23, artifact.SizeBytes)
	assert.Equal(t, "sha256:130461b68aa8db550531b4d1d7d0fb8705dab64d269400d2410365e625261fcd", artifact.SHA256)
	assert.True(t, artifact.InPrompt)
	require.Len(t, out.InputBudget.Omissions, 1)
	assert.Equal(t, "referenced_files_not_measured", out.InputBudget.Omissions[0])
}

func TestMeasureApplyInput_WithUnavailablePlan_RecordsOmission(t *testing.T) {
	// Arrange
	target := taskTargetOutput{}

	// Act
	budget := measureApplyInput("Inspect.", target)

	// Assert
	assert.Empty(t, budget.ContextArtifacts)
	require.Len(t, budget.Omissions, 2)
	assert.Equal(t, "referenced_files_not_measured", budget.Omissions[0])
	assert.Equal(t, "plan_body_empty_or_unavailable", budget.Omissions[1])
}

func TestMeasureApplyInput_WithCloseoutPlan_DoesNotCountSeparatePlanAsPrompt(t *testing.T) {
	// Arrange
	target := taskTargetOutput{Kind: "closeout", PlanBody: "Review receipts."}

	// Act
	budget := measureApplyInput("Close.", target)

	// Assert
	assert.Equal(t, 6, budget.PromptBytes)
	assert.Equal(t, 2, budget.PromptTokenEstimate)
	require.Len(t, budget.ContextArtifacts, 1)
	assert.Equal(t, 16, budget.ContextArtifacts[0].SizeBytes)
	assert.False(t, budget.ContextArtifacts[0].InPrompt)
}

func TestMeasureApplyInput_WithInvalidUTF8_CountsDecodedJSONBytes(t *testing.T) {
	// Arrange
	prompt := "x\xff\xff"

	// Act
	budget := measureApplyInput(prompt, taskTargetOutput{})

	// Assert
	assert.Equal(t, 7, budget.PromptBytes)
	assert.Equal(t, 2, budget.PromptTokenEstimate)
}
