package format

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ProfileGeneric},
		{"CURSOR_PLAN", ProfileCursorPlan},
		{"cursor", ProfileCursorPlan},
		{"speckit", ProfileSpeckit},
		{"bmad-method", ProfileBmad},
		{"openspec", ProfileOpenspec},
		{"adr", ProfileADR},
		{"unknown-thing", ProfileGeneric},
	}
	for _, tc := range tests {
		if g := Normalize(tc.in); g != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.in, g, tc.want)
		}
	}
}

func TestFromPath(t *testing.T) {
	tests := []struct {
		path, want string
	}{
		{"plans/foo.md", ProfileGeneric},
		{".cursor/plans/x.md", ProfileCursorPlan},
		{"specs/001-fe/spec.md", ProfileSpeckit},
		{"_bmad-output/planning-artifacts/prd.md", ProfileBmad},
	}
	for _, tc := range tests {
		if g := FromPath(tc.path); g != tc.want {
			t.Errorf("FromPath(%q) = %q, want %q", tc.path, g, tc.want)
		}
	}
}

func TestFromFrontmatterTool(t *testing.T) {
	if g := FromFrontmatterTool("", "Spec Kit", ""); g != ProfileSpeckit {
		t.Errorf("got %q", g)
	}
	if g := FromFrontmatterTool("cursor desktop"); g != ProfileCursorPlan {
		t.Errorf("got %q", g)
	}
}

func TestLayoutGroup(t *testing.T) {
	if g := LayoutGroup("specs/001-discover-related-specs/spec.md"); g != "specs/001-discover-related-specs" {
		t.Errorf("got %q", g)
	}
	if g := LayoutGroup("_bmad-output/planning-artifacts/prd.md"); g != "_bmad-output/planning-artifacts" {
		t.Errorf("got %q", g)
	}
}
