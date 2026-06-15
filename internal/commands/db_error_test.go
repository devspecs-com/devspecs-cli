package commands

import (
	"errors"
	"strings"
	"testing"
)

func TestFriendlyDBOpenErrorExplainsSandboxAccessFailures(t *testing.T) {
	err := friendlyDBOpenError("/home/user/.devspecs/devspecs.db", errors.New("create db dir: permission denied"))
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	got := err.Error()
	for _, want := range []string{"cannot open local DevSpecs index", "filesystem sandbox", "filesystem approval", "DEVSPECS_HOME"} {
		if !strings.Contains(got, want) {
			t.Fatalf("friendly error missing %q:\n%s", want, got)
		}
	}
}

func TestFriendlyDBOpenErrorPreservesBusyMessage(t *testing.T) {
	err := friendlyDBOpenError("/home/user/.devspecs/devspecs.db", errors.New("database is locked"))
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	got := err.Error()
	if !strings.Contains(got, "another ds command is writing") {
		t.Fatalf("busy error should keep concurrent writer guidance:\n%s", got)
	}
	if strings.Contains(got, "filesystem sandbox") {
		t.Fatalf("busy error should not use sandbox wording:\n%s", got)
	}
}

func TestFriendlyDBOpenErrorLeavesOtherErrorsAlone(t *testing.T) {
	base := errors.New("schema mismatch")
	if got := friendlyDBOpenError("/tmp/devspecs.db", base); got != base {
		t.Fatalf("non-access errors should pass through, got %#v", got)
	}
}
