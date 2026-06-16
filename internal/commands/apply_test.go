package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyNextEmitsOneSlicePromptWithoutChangingState(t *testing.T) {
	repoDir := setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "apply-next-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "first apply slice",
		"--slice", "second apply slice",
		"apply next workflow",
	})
	startCmd.SetOut(&bytes.Buffer{})
	if err := startCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repoDir, "devspecs", "tasks", "apply-next-test", taskManifestFilename)
	before := mustReadFile(t, manifestPath)

	applyCmd := NewApplyCmd()
	applyCmd.SetArgs([]string{"next", "--json"})
	applyBuf := &bytes.Buffer{}
	applyCmd.SetOut(applyBuf)
	if err := applyCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out applyPromptOutput
	if err := json.Unmarshal(applyBuf.Bytes(), &out); err != nil {
		t.Fatalf("apply json: %v\n%s", err, applyBuf.String())
	}
	if out.Command != "ds apply next" || out.TaskID != "apply-next-test" || out.Target != "A01" {
		t.Fatalf("apply next resolved wrong target: %#v", out)
	}
	for _, want := range []string{
		"task apply-next-test target A01 only",
		"must_not_implement",
		"- A02",
		"Completion contract:",
		"Record the outcome",
		"ds task checkpoint apply-next-test --target A01",
	} {
		if !strings.Contains(out.Prompt, want) {
			t.Fatalf("apply prompt missing %q:\n%s", want, out.Prompt)
		}
	}
	if got := mustReadFile(t, manifestPath); got != before {
		t.Fatalf("ds apply should not mutate task state.\nBefore:\n%s\nAfter:\n%s", before, got)
	}
}

func TestApplyExplicitIdentifiersResolveOneTarget(t *testing.T) {
	setupTaskCommandRepo(t)

	startCmd := NewTaskCmd()
	startCmd.SetArgs([]string{
		"--id", "apply-target-test",
		"--no-refresh",
		"--index=false",
		"--json",
		"--slice", "first explicit slice",
		"--slice", "second explicit slice",
		"apply target workflow",
	})
	startCmd.SetOut(&bytes.Buffer{})
	if err := startCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "unique-slice", args: []string{"A02", "--json"}, want: "A02"},
		{name: "task-target", args: []string{"apply-target-test", "--target", "A02", "--json"}, want: "A02"},
		{name: "series-index", args: []string{"A00", "--json"}, want: "A01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			applyCmd := NewApplyCmd()
			applyCmd.SetArgs(tc.args)
			buf := &bytes.Buffer{}
			applyCmd.SetOut(buf)
			if err := applyCmd.Execute(); err != nil {
				t.Fatal(err)
			}
			var out applyPromptOutput
			if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
				t.Fatalf("apply json: %v\n%s", err, buf.String())
			}
			if out.TaskID != "apply-target-test" || out.Target != tc.want {
				t.Fatalf("apply resolved wrong target for %s: %#v", tc.name, out)
			}
			if !strings.Contains(out.Prompt, "target "+tc.want+" only") {
				t.Fatalf("apply prompt not bounded to %s:\n%s", tc.want, out.Prompt)
			}
		})
	}
}

func TestApplyNextRequiresUnambiguousTask(t *testing.T) {
	setupTaskCommandRepo(t)

	for _, taskID := range []string{"apply-ambiguous-a", "apply-ambiguous-b"} {
		startCmd := NewTaskCmd()
		startCmd.SetArgs([]string{
			"--id", taskID,
			"--no-refresh",
			"--index=false",
			"--json",
			"--slice", "shared apply slice",
			"apply ambiguity",
		})
		startCmd.SetOut(&bytes.Buffer{})
		if err := startCmd.Execute(); err != nil {
			t.Fatal(err)
		}
	}

	applyCmd := NewApplyCmd()
	applyCmd.SetArgs([]string{"next", "--json"})
	applyCmd.SetOut(&bytes.Buffer{})
	err := applyCmd.Execute()
	if err == nil {
		t.Fatal("expected ambiguous next error")
	}
	for _, want := range []string{
		"ambiguous next task target",
		"apply-ambiguous-a:A01",
		"apply-ambiguous-b:B01",
		"use `ds apply <task-id>`",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ambiguous error missing %q: %v", want, err)
		}
	}
}

func TestApplyExplicitSliceRequiresUnambiguousTarget(t *testing.T) {
	setupTaskCommandRepo(t)

	for _, taskID := range []string{"apply-target-a", "apply-target-b"} {
		startCmd := NewTaskCmd()
		startCmd.SetArgs([]string{
			"--id", taskID,
			"--series", "A",
			"--no-refresh",
			"--index=false",
			"--json",
			"--slice", "shared first slice",
			"apply target ambiguity",
		})
		startCmd.SetOut(&bytes.Buffer{})
		if err := startCmd.Execute(); err != nil {
			t.Fatal(err)
		}
	}

	applyCmd := NewApplyCmd()
	applyCmd.SetArgs([]string{"A01", "--json"})
	applyCmd.SetOut(&bytes.Buffer{})
	err := applyCmd.Execute()
	if err == nil {
		t.Fatal("expected ambiguous target error")
	}
	for _, want := range []string{
		"ambiguous task target",
		"apply-target-a:A01",
		"apply-target-b:A01",
		"use a task id with --target",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ambiguous error missing %q: %v", want, err)
		}
	}
}
