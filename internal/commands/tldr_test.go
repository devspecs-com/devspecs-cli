package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTLDR_HumanOutputGroupsWorkflows(t *testing.T) {
	cmd := NewTLDRCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.Contains(t, out, "# DevSpecs TLDR For LLM Agents",
		"tldr output missing %q:\n%s", "# DevSpecs TLDR For LLM Agents", out)
	assert.Contains(t, out, "## Launch Setup / Agent Commands (`setup`)",
		"tldr output missing %q:\n%s", "## Launch Setup / Agent Commands (`setup`)", out)
	assert.Contains(t, out, "## Hotfix / Small Bug (`hotfix`)",
		"tldr output missing %q:\n%s", "## Hotfix / Small Bug (`hotfix`)", out)
	assert.Contains(t, out, "## Epic / Multi-Slice Feature (`epic`)",
		"tldr output missing %q:\n%s", "## Epic / Multi-Slice Feature (`epic`)", out)
	assert.Contains(t, out, "## Incident / Triage (`incident`)",
		"tldr output missing %q:\n%s", "## Incident / Triage (`incident`)", out)
	assert.Contains(t, out, "## Brownfield Intent Recovery (`brownfield`)",
		"tldr output missing %q:\n%s", "## Brownfield Intent Recovery (`brownfield`)", out)
	assert.Contains(t, out, `ds task "fix <bug>" --quick`,
		"tldr output missing %q:\n%s", `ds task "fix <bug>" --quick`, out)
	assert.Contains(t, out, "ds task checkpoint <task-id> --target <target>",
		"tldr output missing %q:\n%s", "ds task checkpoint <task-id> --target <target>", out)
	assert.Contains(t, out, "Fastest path for known work",
		"tldr output missing %q:\n%s", "Fastest path for known work", out)
	assert.Contains(t, out, "Run ds init once per repo",
		"tldr output missing %q:\n%s", "Run ds init once per repo", out)
	assert.Contains(t, out, `/ds-task "goal"`,
		"tldr output missing %q:\n%s", `/ds-task "goal"`, out)
	assert.Contains(t, out, "/ds-apply [task-id|target]",
		"tldr output missing %q:\n%s", "/ds-apply [task-id|target]", out)
	assert.Contains(t, out, "Human front door: run ds recent",
		"tldr output missing %q:\n%s", "Human front door: run ds recent", out)
	assert.Contains(t, out, "Workflow commands refresh the local index by default",
		"tldr output missing %q:\n%s", "Workflow commands refresh the local index by default", out)
	assert.Contains(t, out, "Use ds recent, ds find, ds map, and ds context as diagnostic/evidence tools around a task",
		"tldr output missing %q:\n%s", "Use ds recent, ds find, ds map, and ds context as diagnostic/evidence tools around a task", out)
	assert.Contains(t, out, "Command roles: ds find discovers and packs evidence",
		"tldr output missing %q:\n%s", "Command roles: ds find discovers and packs evidence", out)
	assert.Contains(t, out, "ds task slice add <task-id>",
		"tldr output missing %q:\n%s", "ds task slice add <task-id>", out)
	assert.Contains(t, out, "--after A01 --reason improve",
		"tldr output missing %q:\n%s", "--after A01 --reason improve", out)
	assert.Contains(t, out, "Record the completion contract with checkpoint",
		"tldr output missing %q:\n%s", "Record the completion contract with checkpoint", out)
	assert.Contains(t, out, "ds map",
		"tldr output missing %q:\n%s", "ds map", out)
	assert.Contains(t, out, "ds recent",
		"tldr output missing %q:\n%s", "ds recent", out)
	assert.Contains(t, out, `ds task "implement <bounded target>"`,
		"tldr output missing %q:\n%s", `ds task "implement <bounded target>"`, out)

	assert.NotContains(t, out, "ds list",
		"tldr output should not advertise %q:\n%s", "ds list", out)
	assert.NotContains(t, out, "ds list --limit",
		"tldr output should not advertise %q:\n%s", "ds list --limit", out)

	brownfield := tldrSection(t, out, "## Brownfield Intent Recovery (`brownfield`)", "## Handoff / Resume After Context Loss (`handoff`)")
	recentIndex := strings.Index(brownfield, "`ds recent`")
	taskIndex := strings.Index(brownfield, "`ds task \"implement <bounded target>\"`")
	assert.GreaterOrEqual(t, recentIndex, 0, "brownfield workflow should put ds recent before bounded execution:\n%s", brownfield)
	assert.GreaterOrEqual(t, taskIndex, 0, "brownfield workflow should put ds recent before bounded execution:\n%s", brownfield)
	assert.LessOrEqual(t, recentIndex, taskIndex, "brownfield workflow should put ds recent before bounded execution:\n%s", brownfield)

}

func TestTLDR_FilterAndJSON(t *testing.T) {
	cmd := NewTLDRCmd()
	cmd.SetArgs([]string{"incident", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out tldrOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err,
			"tldr json: %v\n%s", err, buf.String())
	}
	require.Len(t, out.Workflows, 1, "expected only incident workflow, got %#v", out.Workflows)
	assert.Equal(t, "incident", out.Workflows[0].ID, "expected only incident workflow, got %#v", out.Workflows)

	commands := strings.Join(out.Workflows[0].Commands, "\n")
	assert.Contains(t, commands, "ds recent",
		"incident workflow missing recent orientation command: %#v", out.Workflows[0])
	assert.Contains(t, commands, "ds find",
		"incident workflow missing packed find command: %#v", out.Workflows[0])
	assert.LessOrEqual(t, strings.Index(commands, "ds recent"), strings.Index(commands, `ds task "triage <incident>" --quick`),
		"incident workflow should orient with recent before task execution: %#v", out.Workflows[0])
	assert.NotContains(t, commands, "ds scan",
		"incident workflow should not require manual scan: %#v", out.Workflows[0])

}

func TestTLDR_UnknownWorkflowErrorsWithValidIDs(t *testing.T) {
	cmd := NewTLDRCmd()
	cmd.SetArgs([]string{"migration"})
	cmd.SetOut(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err,
		"expected unknown workflow error")
	assert.Contains(t, err.Error(), "valid workflows:", "unexpected error: %v", err)
	assert.Contains(t, err.Error(), "hotfix", "unexpected error: %v", err)

}

func tldrSection(t *testing.T, out, start, end string) string {
	t.Helper()
	startIndex := strings.Index(out, start)
	assert.GreaterOrEqual(t, startIndex, 0,
		"tldr output missing section %q:\n%s", start, out)

	endIndex := strings.Index(out[startIndex:], end)
	assert.GreaterOrEqual(t, endIndex, 0,
		"tldr output missing section terminator %q after %q:\n%s", end, start, out)

	return out[startIndex : startIndex+endIndex]
}
