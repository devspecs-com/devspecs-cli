package store

import (
	"errors"
	"strings"
	"testing"
)

func TestSQLiteBusyErrorHelpers(t *testing.T) {
	busyErr := errors.New("migrate: apply schema: database is locked (SQLITE_BUSY)")
	if !IsSQLiteBusyError(busyErr) {
		t.Fatalf("expected busy error to be recognized")
	}

	wrapped := FriendlySQLiteBusyError(busyErr)
	if wrapped == busyErr {
		t.Fatalf("expected friendly wrapper")
	}
	if !strings.Contains(wrapped.Error(), "another ds command is writing") {
		t.Fatalf("friendly message missing writer hint: %v", wrapped)
	}

	otherErr := errors.New("syntax error")
	if FriendlySQLiteBusyError(otherErr) != otherErr {
		t.Fatalf("non-busy errors should pass through")
	}
}
