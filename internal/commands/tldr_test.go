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
		"## Hotfix / Small Bug (`hotfix`)",
		"## Epic / Multi-Slice Feature (`epic`)",
		"## Incident / Triage (`incident`)",
		"## Brownfield Intent Recovery (`brownfield`)",
		"ds task quick",
		"ds task checkpoint <task-id> --target <target>",
		"Workflow commands refresh the local index by default",
		"ds list --limit 20",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("tldr output missing %q:\n%s", want, out)
		}
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
	if !strings.Contains(commands, "ds find") {
		t.Fatalf("incident workflow missing packed find command: %#v", out.Workflows[0])
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
