package store

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSQLiteBusyError_WithBusyError_ReturnsTrue(t *testing.T) {
	err := errors.New("migrate: apply schema: database is locked (SQLITE_BUSY)")

	busy := IsSQLiteBusyError(err)

	assert.True(t, busy)
}

func TestFriendlySQLiteBusyError_WithBusyError_ReturnsWriterGuidance(t *testing.T) {
	err := errors.New("migrate: apply schema: database is locked (SQLITE_BUSY)")

	wrapped := FriendlySQLiteBusyError(err)

	assert.NotEqual(t, err, wrapped)
	assert.ErrorContains(t, wrapped, "another ds command is writing")
}

func TestFriendlySQLiteBusyError_WithOtherError_ReturnsOriginalError(t *testing.T) {
	err := errors.New("syntax error")

	got := FriendlySQLiteBusyError(err)

	assert.Equal(t, err, got)
}
