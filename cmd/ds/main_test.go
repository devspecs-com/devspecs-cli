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
