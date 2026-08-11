package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupApplyTask(t *testing.T, taskID, series string, slices ...string) string {
	t.Helper()
	repoDir := setupTaskCommandRepo(t)
	createApplyTask(t, taskID, series, slices...)
	return repoDir
}

func createApplyTask(t *testing.T, taskID, series string, slices ...string) {
	t.Helper()
	args := []string{"--id", taskID, "--no-refresh", "--index=false", "--json"}
	if series != "" {
		args = append(args, "--series", series)
	}
	for _, title := range slices {
		args = append(args, "--slice", title)
	}
	args = append(args, "apply workflow")
	cmd := NewTaskCmd()
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func setupApplyRepoRouteTask(t *testing.T) string {
	t.Helper()
	_, child := setupTaskCommandUmbrellaRepo(t)
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"--repo", "./enalytics-backend",
		"--id", "apply-repo-route",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "first backend apply slice",
		"--slice", "second backend apply slice",
		"apply backend workspace route",
	})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
	return child
}

func setupApplyDecisionGate(t *testing.T, taskID, decision string, withIteration bool) {
	t.Helper()
	setupApplyTask(t, taskID, "A", "first gate slice", "second gate slice")
	if withIteration {
		addApplyTestIteration(t, taskID, "repair first gate slice", "A01", decision)
	}
	decideApplyTestTarget(t, taskID, "A01", decision)
}

func decodeApplyPromptOutput(t *testing.T, buf *bytes.Buffer) applyPromptOutput {
	t.Helper()
	var out applyPromptOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	return out
}

func TestApplyNextEmitsOneSlicePromptWithoutChangingState(t *testing.T) {
	repoDir := setupApplyTask(t, "apply-next-test", "", "first apply slice", "second apply slice")
	manifestPath := filepath.Join(repoDir, "devspecs", "tasks", "apply-next-test", taskManifestFilename)
	before := mustReadFile(t, manifestPath)
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"next", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	out := decodeApplyPromptOutput(t, buf)
	assert.Equal(t, "ds apply next", out.Command)
	assert.Equal(t, "apply-next-test", out.TaskID)
	assert.Equal(t, "A01", out.Target)
	assert.Contains(t, out.Prompt, "task apply-next-test target A01 only")
	assert.Contains(t, out.Prompt, "must_not_implement")
	assert.Contains(t, out.Prompt, "- A02")
	assert.Contains(t, out.Prompt, "Completion contract:")
	assert.Contains(t, out.Prompt, "Record the outcome")
	assert.Contains(t, out.Prompt, "ds task checkpoint apply-next-test --target A01")
	assert.Contains(t, out.Prompt, "Command roles:")
	assert.Contains(t, out.Prompt, "use `ds find` to discover and pack evidence")
	assert.Contains(t, out.Prompt, "`status` and `index_status` are separate signals")
	assert.Equal(t, before, mustReadFile(t, manifestPath))
}

func TestApplyDefaultsToNextWhenUnambiguous(t *testing.T) {
	repoDir := setupApplyTask(t, "apply-default-next", "A", "first default apply slice", "second default apply slice")
	manifestPath := filepath.Join(repoDir, "devspecs", "tasks", "apply-default-next", taskManifestFilename)
	before := mustReadFile(t, manifestPath)
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	out := decodeApplyPromptOutput(t, buf)
	assert.Equal(t, "ds apply", out.Command)
	assert.Equal(t, "apply-default-next", out.TaskID)
	assert.Equal(t, "A01", out.Target)
	assert.Contains(t, out.Prompt, "task apply-default-next target A01 only")
	assert.Equal(t, before, mustReadFile(t, manifestPath))
}

func TestApplyNextRepoFlagResolvesTargetRepoFromUmbrella(t *testing.T) {
	child := setupApplyRepoRouteTask(t)
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"next", "--repo", "./enalytics-backend", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	out := decodeApplyPromptOutput(t, buf)
	assert.Equal(t, "ds apply next --repo ./enalytics-backend", out.Command)
	assert.Equal(t, "apply-repo-route", out.TaskID)
	assert.Equal(t, "A01", out.Target)
	assert.True(t, strings.HasPrefix(out.TargetContext.Workspace, filepath.Join(child, "devspecs", "tasks", "apply-repo-route")))
	assert.Contains(t, out.Prompt, "ds task checkpoint apply-repo-route --target A01 --repo ./enalytics-backend")
}

func TestApplyImplicitRepoFlagResolvesTargetRepoFromUmbrella(t *testing.T) {
	setupApplyRepoRouteTask(t)
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"--repo", "./enalytics-backend", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	out := decodeApplyPromptOutput(t, buf)
	assert.Equal(t, "ds apply --repo ./enalytics-backend", out.Command)
	assert.Equal(t, "apply-repo-route", out.TaskID)
	assert.Equal(t, "A01", out.Target)
}

func TestApplyExplicitRepoTargetResolvesTargetRepoFromUmbrella(t *testing.T) {
	setupApplyRepoRouteTask(t)
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"apply-repo-route", "--target", "A02", "--repo", "./enalytics-backend", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	out := decodeApplyPromptOutput(t, buf)
	assert.Equal(t, "ds apply apply-repo-route --target A02 --repo ./enalytics-backend", out.Command)
	assert.Equal(t, "apply-repo-route", out.TaskID)
	assert.Equal(t, "A02", out.Target)
}

func TestApplyExplicitSliceIdentifierResolvesOneTarget(t *testing.T) {
	setupApplyTask(t, "apply-target-test", "A", "first explicit slice", "second explicit slice")
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"A02", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	out := decodeApplyPromptOutput(t, buf)
	assert.Equal(t, "apply-target-test", out.TaskID)
	assert.Equal(t, "A02", out.Target)
	assert.Contains(t, out.Prompt, "target A02 only")
}

func TestApplyTaskAndTargetIdentifiersResolveOneTarget(t *testing.T) {
	setupApplyTask(t, "apply-target-test", "A", "first explicit slice", "second explicit slice")
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"apply-target-test", "--target", "A02", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	out := decodeApplyPromptOutput(t, buf)
	assert.Equal(t, "apply-target-test", out.TaskID)
	assert.Equal(t, "A02", out.Target)
	assert.Contains(t, out.Prompt, "target A02 only")
}

func TestApplySeriesIndexResolvesFirstSlice(t *testing.T) {
	setupApplyTask(t, "apply-target-test", "A", "first explicit slice", "second explicit slice")
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"A00", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	out := decodeApplyPromptOutput(t, buf)
	assert.Equal(t, "apply-target-test", out.TaskID)
	assert.Equal(t, "A01", out.Target)
	assert.Contains(t, out.Prompt, "target A01 only")
}

func TestApplyNextAfterPromoteAdvancesToNextSlice(t *testing.T) {
	taskID := "apply-gate-promote"
	setupApplyDecisionGate(t, taskID, "promote", false)
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"next", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	out := decodeApplyPromptOutput(t, buf)
	assert.Equal(t, taskID, out.TaskID)
	assert.Equal(t, "A02", out.Target)
	assert.Contains(t, out.Prompt, "target A02 only")
}

func TestApplyNextAfterImproveSelectsExistingIteration(t *testing.T) {
	taskID := "apply-gate-improve-iteration"
	setupApplyDecisionGate(t, taskID, "improve", true)
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"next", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	out := decodeApplyPromptOutput(t, buf)
	assert.Equal(t, taskID, out.TaskID)
	assert.Equal(t, "A01-1", out.Target)
	assert.Contains(t, out.Prompt, "target A01-1 only")
}

func TestApplyNextAfterReworkSelectsExistingIteration(t *testing.T) {
	taskID := "apply-gate-rework-iteration"
	setupApplyDecisionGate(t, taskID, "rework", true)
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"next", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	out := decodeApplyPromptOutput(t, buf)
	assert.Equal(t, taskID, out.TaskID)
	assert.Equal(t, "A01-1", out.Target)
	assert.Contains(t, out.Prompt, "target A01-1 only")
}

func TestApplyNextAfterImproveWithoutIterationExplainsRequiredSlice(t *testing.T) {
	setupApplyDecisionGate(t, "apply-gate-improve-without-iteration", "improve", false)
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"next", "--json"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "A01 ended with improve")
	assert.ErrorContains(t, err, "ds task slice add")
	assert.ErrorContains(t, err, "--after A01")
	assert.ErrorContains(t, err, "--reason improve")
}

func TestApplyNextAfterRollbackRequiresExplicitTarget(t *testing.T) {
	setupApplyDecisionGate(t, "apply-gate-rollback", "rollback", false)
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"next", "--json"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "A01 ended with rollback")
	assert.ErrorContains(t, err, "automatic next is blocked")
	assert.ErrorContains(t, err, "choose an explicit target")
}

func TestApplyNextAfterBlockRequiresExplicitTarget(t *testing.T) {
	setupApplyDecisionGate(t, "apply-gate-block", "block", false)
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"next", "--json"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "A01 ended with block")
	assert.ErrorContains(t, err, "automatic next is blocked")
	assert.ErrorContains(t, err, "choose an explicit target")
}

func TestApplyNextReportsCompletedTrack(t *testing.T) {
	taskID := "apply-completed-track"
	setupApplyTask(t, taskID, "A", "only slice")
	decideApplyTestTarget(t, taskID, "A01", "promote")
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"next", "--json"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "no non-terminal DevSpecs task targets found")
}

func TestApplySeriesIndexRequiresUnambiguousTrack(t *testing.T) {
	setupTaskCommandRepo(t)
	createApplyTask(t, "apply-series-a", "A", "first shared series")
	createApplyTask(t, "apply-series-b", "A", "second shared series")
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"A00", "--json"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "ambiguous task series")
	assert.ErrorContains(t, err, "apply-series-a:A01")
	assert.ErrorContains(t, err, "apply-series-b:A01")
	assert.ErrorContains(t, err, "use a task id with --target")
}

func TestApplyNextRequiresUnambiguousTask(t *testing.T) {
	setupTaskCommandRepo(t)
	createApplyTask(t, "apply-ambiguous-a", "", "shared apply slice")
	createApplyTask(t, "apply-ambiguous-b", "", "shared apply slice")
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"next", "--json"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "ambiguous next task target")
	assert.ErrorContains(t, err, "apply-ambiguous-a:A01")
	assert.ErrorContains(t, err, "apply-ambiguous-b:B01")
	assert.ErrorContains(t, err, "use `ds apply <task-id>`")
}

func TestApplyExplicitSliceRequiresUnambiguousTarget(t *testing.T) {
	setupTaskCommandRepo(t)
	createApplyTask(t, "apply-target-a", "A", "shared first slice")
	createApplyTask(t, "apply-target-b", "A", "shared first slice")
	cmd := NewApplyCmd()
	cmd.SetArgs([]string{"A01", "--json"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, "ambiguous task target")
	assert.ErrorContains(t, err, "apply-target-a:A01")
	assert.ErrorContains(t, err, "apply-target-b:A01")
	assert.ErrorContains(t, err, "use a task id with --target")
}

func addApplyTestIteration(t *testing.T, taskID, title, parent, reason string) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"iteration", "add", taskID, title,
		"--slice", parent,
		"--reason", reason,
		"--index=false",
		"--json",
	})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

func decideApplyTestTarget(t *testing.T, taskID, target, decision string) {
	t.Helper()
	cmd := NewTaskCmd()
	cmd.SetArgs([]string{
		"decide", taskID,
		"--target", target,
		"--decision", decision,
		"--index=false",
		"--json",
	})
	cmd.SetOut(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
}

// runApplyJSON supports the workspace end-to-end fixture in workspace_e2e_test.go.
func runApplyJSON(t *testing.T, args []string) (applyPromptOutput, error) {
	t.Helper()
	cmd := NewApplyCmd()
	cmd.SetArgs(args)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	err := cmd.Execute()
	if err != nil {
		return applyPromptOutput{}, err
	}
	return decodeApplyPromptOutput(t, buf), nil
}
