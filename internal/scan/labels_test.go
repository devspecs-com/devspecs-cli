package scan

import "testing"

func TestSourceTypeDisplayLabel(t *testing.T) {
	tests := []struct{ in, want string }{
		{"markdown", "Planning docs"},
		{"openspec", "OpenSpec"},
		{"adr", "ADRs"},
		{"capture", "capture"},
	}
	for _, tc := range tests {
		if g := SourceTypeDisplayLabel(tc.in); g != tc.want {
			t.Errorf("SourceTypeDisplayLabel(%q) = %q, want %q", tc.in, g, tc.want)
		}
	}
}
