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
