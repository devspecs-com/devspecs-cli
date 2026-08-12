package commands

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFriendlyDBOpenErrorExplainsSandboxAccessFailures(t *testing.T) {
	err := friendlyDBOpenError("/home/user/.devspecs/devspecs.db", errors.New("create db dir: permission denied"))
	require.Error(t, err,
		"expected wrapped error")

	got := err.Error()
	assert.Contains(t, got, "cannot open local DevSpecs index",
		"friendly error missing %q:\n%s", "cannot open local DevSpecs index", got)
	assert.Contains(t, got, "filesystem sandbox",
		"friendly error missing %q:\n%s", "filesystem sandbox", got)
	assert.Contains(t, got, "filesystem approval",
		"friendly error missing %q:\n%s", "filesystem approval", got)
	assert.Contains(t, got, "DEVSPECS_HOME",
		"friendly error missing %q:\n%s", "DEVSPECS_HOME", got)

}

func TestFriendlyDBOpenErrorPreservesBusyMessage(t *testing.T) {
	err := friendlyDBOpenError("/home/user/.devspecs/devspecs.db", errors.New("database is locked"))
	require.Error(t, err,
		"expected wrapped error")

	got := err.Error()
	assert.Contains(t, got, "another ds command is writing",
		"busy error should keep concurrent writer guidance:\n%s", got)
	assert.NotContains(t, got, "filesystem sandbox",
		"busy error should not use sandbox wording:\n%s", got)

}

func TestFriendlyDBOpenErrorLeavesOtherErrorsAlone(t *testing.T) {
	base := errors.New("schema mismatch")
	{
		got := friendlyDBOpenError("/tmp/devspecs.db", base)
		assert.Equal(t, base, got,
			"non-access errors should pass through, got %#v", got)
	}

}
