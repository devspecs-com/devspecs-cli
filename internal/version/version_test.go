package version

import "testing"

func TestDefaults(t *testing.T) {
	if Version != "dev" {
		t.Errorf("expected Version=dev, got %q", Version)
	}
	if Commit != "none" {
		t.Errorf("expected Commit=none, got %q", Commit)
	}
	if Date != "unknown" {
		t.Errorf("expected Date=unknown, got %q", Date)
	}
}
