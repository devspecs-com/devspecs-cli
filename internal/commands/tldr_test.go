package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestTLDR_HumanOutputGroupsWorkflows(t *testing.T) {
	cmd := NewTLDRCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"# DevSpecs TLDR For LLM Agents",
		"## Launch Setup / Agent Commands (`setup`)",
		"## Hotfix / Small Bug (`hotfix`)",
		"## Epic / Multi-Slice Feature (`epic`)",
		"## Incident / Triage (`incident`)",
		"## Brownfield Intent Recovery (`brownfield`)",
		`ds task "fix <bug>" --quick`,
		"ds task checkpoint <task-id> --target <target>",
		"Fastest path for known work",
		"Run ds init once per repo",
		`/ds-task "goal"`,
		"/ds-apply [task-id|target]",
		"Human front door: run ds recent",
		"Workflow commands refresh the local index by default",
		"Use ds recent, ds find, ds map, and ds context as diagnostic/evidence tools around a task",
		"Command roles: ds find discovers and packs evidence",
		"ds task slice add <task-id>",
		"--after A01 --reason improve",
		"Record the completion contract with checkpoint",
		"ds map",
		"ds recent",
		`ds task "implement <bounded target>"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("tldr output missing %q:\n%s", want, out)
		}
	}
	for _, notWant := range []string{"ds list", "ds list --limit"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("tldr output should not advertise %q:\n%s", notWant, out)
		}
	}
	brownfield := tldrSection(t, out, "## Brownfield Intent Recovery (`brownfield`)", "## Handoff / Resume After Context Loss (`handoff`)")
	recentIndex := strings.Index(brownfield, "`ds recent`")
	taskIndex := strings.Index(brownfield, "`ds task \"implement <bounded target>\"`")
	if recentIndex < 0 || taskIndex < 0 || recentIndex > taskIndex {
		t.Fatalf("brownfield workflow should put ds recent before bounded execution:\n%s", brownfield)
	}
}

func TestTLDR_FilterAndJSON(t *testing.T) {
	cmd := NewTLDRCmd()
	cmd.SetArgs([]string{"incident", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out tldrOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("tldr json: %v\n%s", err, buf.String())
	}
	if len(out.Workflows) != 1 || out.Workflows[0].ID != "incident" {
		t.Fatalf("expected only incident workflow, got %#v", out.Workflows)
	}
	commands := strings.Join(out.Workflows[0].Commands, "\n")
	if !strings.Contains(commands, "ds recent") {
		t.Fatalf("incident workflow missing recent orientation command: %#v", out.Workflows[0])
	}
	if !strings.Contains(commands, "ds find") {
		t.Fatalf("incident workflow missing packed find command: %#v", out.Workflows[0])
	}
	if strings.Index(commands, "ds recent") > strings.Index(commands, `ds task "triage <incident>" --quick`) {
		t.Fatalf("incident workflow should orient with recent before task execution: %#v", out.Workflows[0])
	}
	if strings.Contains(commands, "ds scan") {
		t.Fatalf("incident workflow should not require manual scan: %#v", out.Workflows[0])
	}
}

func TestTLDR_UnknownWorkflowErrorsWithValidIDs(t *testing.T) {
	cmd := NewTLDRCmd()
	cmd.SetArgs([]string{"migration"})
	cmd.SetOut(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown workflow error")
	}
	if !strings.Contains(err.Error(), "valid workflows:") || !strings.Contains(err.Error(), "hotfix") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func tldrSection(t *testing.T, out, start, end string) string {
	t.Helper()
	startIndex := strings.Index(out, start)
	if startIndex < 0 {
		t.Fatalf("tldr output missing section %q:\n%s", start, out)
	}
	endIndex := strings.Index(out[startIndex:], end)
	if endIndex < 0 {
		t.Fatalf("tldr output missing section terminator %q after %q:\n%s", end, start, out)
	}
	return out[startIndex : startIndex+endIndex]
}
