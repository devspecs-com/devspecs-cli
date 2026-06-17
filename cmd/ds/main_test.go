package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_Version(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--version"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "dev") {
		t.Errorf("expected version output to contain 'dev', got %q", got)
	}
	if !strings.Contains(got, "none") {
		t.Errorf("expected version output to contain 'none', got %q", got)
	}
	if !strings.Contains(got, "unknown") {
		t.Errorf("expected version output to contain 'unknown', got %q", got)
	}
}

func TestRootCmd_HelpMentionsTelemetryPrivacy(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	for _, want := range []string{
		"Telemetry:",
		"minimal anonymous usage counts",
		"never sends repo names, file paths, git remotes",
		"raw queries",
		"DEVSPECS_TELEMETRY=0",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected help output to contain %q, got %q", want, got)
		}
	}
}

func TestRootCmd_HelpCentersTaskWorkflow(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	for _, want := range []string{
		"Default workflow:",
		"use ds task to create bounded task workspaces",
		"Use ds apply next or ds apply",
		"Diagnostic layer:",
		"use ds map, ds recent, and ds find",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected help output to contain %q, got %q", want, got)
		}
	}
}

func TestRootCmd_TLDRRegistered(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"tldr", "hotfix"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "Hotfix / Small Bug") {
		t.Fatalf("expected tldr hotfix output, got %q", got)
	}
}

func TestRootCmd_ApplyRegistered(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"apply", "--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	for _, want := range []string{
		"Emit an agent prompt for exactly one DevSpecs task target.",
		"apply <next|task-id|target>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected apply help to contain %q, got:\n%s", want, got)
		}
	}
}

func TestRootCmd_PublicHelpHidesInternalCommands(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--help"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	for _, hidden := range []string{
		"  capture     ",
		"  criteria    ",
		"  eval        ",
		"  link        ",
		"  list        ",
		"  resolve     ",
		"  resume      ",
		"  status      ",
		"  tag         ",
		"  todos       ",
		"  untag       ",
	} {
		if strings.Contains(got, hidden) {
			t.Fatalf("public help should hide internal command %q, got:\n%s", strings.TrimSpace(hidden), got)
		}
	}
}

func TestRootCmd_ListNotRegistered(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list"})

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected ds list to be unavailable")
	}
	got := buf.String()
	if strings.Contains(got, "List indexed artifacts") {
		t.Fatalf("ds list should not dispatch to artifact list command, got:\n%s", got)
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}
