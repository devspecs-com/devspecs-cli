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

func TestApplyNextRespectsDecisionGateProgression(t *testing.T) {
	for _, tc := range []struct {
		name       string
		decision   string
		iteration  bool
		wantTarget string
		wantErr    []string
	}{
		{name: "promote-advances-to-next-slice", decision: "promote", wantTarget: "A02"},
		{name: "improve-selects-existing-iteration", decision: "improve", iteration: true, wantTarget: "A01-1"},
		{name: "rework-selects-existing-iteration", decision: "rework", iteration: true, wantTarget: "A01-1"},
		{name: "improve-without-iteration-stops-before-sibling", decision: "improve", wantErr: []string{"A01 ended with improve", "ds task iteration add", "--slice A01", "--reason improve"}},
		{name: "rollback-blocks-automatic-next", decision: "rollback", wantErr: []string{"A01 ended with rollback", "automatic next is blocked", "choose an explicit target"}},
		{name: "block-blocks-automatic-next", decision: "block", wantErr: []string{"A01 ended with block", "automatic next is blocked", "choose an explicit target"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupTaskCommandRepo(t)
			taskID := "apply-gate-" + sanitizeTaskFilename(tc.name)
			createApplyTestTask(t, taskID, "first gate slice", "second gate slice")
			if tc.iteration {
				addApplyTestIteration(t, taskID, "repair first gate slice", "A01", tc.decision)
			}
			decideApplyTestTarget(t, taskID, "A01", tc.decision)

			out, err := runApplyJSON(t, []string{"next", "--json"})
			if len(tc.wantErr) > 0 {
				if err == nil {
					t.Fatalf("expected apply next error, got output %#v", out)
				}
				for _, want := range tc.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("apply next error missing %q: %v", want, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if out.TaskID != taskID || out.Target != tc.wantTarget {
				t.Fatalf("apply next resolved wrong target: %#v", out)
			}
			if !strings.Contains(out.Prompt, "target "+tc.wantTarget+" only") {
				t.Fatalf("apply prompt not bounded to %s:\n%s", tc.wantTarget, out.Prompt)
			}
		})
	}
}

func TestApplyNextReportsCompletedTrack(t *testing.T) {
	setupTaskCommandRepo(t)
	taskID := "apply-completed-track"
	createApplyTestTask(t, taskID, "only slice")
	decideApplyTestTarget(t, taskID, "A01", "promote")

	out, err := runApplyJSON(t, []string{"next", "--json"})
	if err == nil {
		t.Fatalf("expected completed track error, got output %#v", out)
	}
	if !strings.Contains(err.Error(), "no non-terminal DevSpecs task targets found") {
		t.Fatalf("completed track error was not useful: %v", err)
	}
}

func TestApplySeriesIndexRequiresUnambiguousTrack(t *testing.T) {
	setupTaskCommandRepo(t)
	createApplyTestTask(t, "apply-series-a", "first shared series")
	createApplyTestTask(t, "apply-series-b", "second shared series")

	applyCmd := NewApplyCmd()
	applyCmd.SetArgs([]string{"A00", "--json"})
	applyCmd.SetOut(&bytes.Buffer{})
	err := applyCmd.Execute()
	if err == nil {
		t.Fatal("expected ambiguous series error")
	}
	for _, want := range []string{
		"ambiguous task series",
		"apply-series-a:A01",
		"apply-series-b:A01",
		"use a task id with --target",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ambiguous series error missing %q: %v", want, err)
		}
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

func createApplyTestTask(t *testing.T, taskID string, slices ...string) {
	t.Helper()
	args := []string{
		"--id", taskID,
		"--series", "A",
		"--no-refresh",
		"--index=false",
		"--json",
	}
	for _, slice := range slices {
		args = append(args, "--slice", slice)
	}
	args = append(args, "apply gate workflow")
	startCmd := NewTaskCmd()
	startCmd.SetArgs(args)
	startCmd.SetOut(&bytes.Buffer{})
	if err := startCmd.Execute(); err != nil {
		t.Fatal(err)
	}
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
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
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
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func runApplyJSON(t *testing.T, args []string) (applyPromptOutput, error) {
	t.Helper()
	applyCmd := NewApplyCmd()
	applyCmd.SetArgs(args)
	buf := &bytes.Buffer{}
	applyCmd.SetOut(buf)
	err := applyCmd.Execute()
	if err != nil {
		return applyPromptOutput{}, err
	}
	var out applyPromptOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("apply json: %v\n%s", err, buf.String())
	}
	return out, nil
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
