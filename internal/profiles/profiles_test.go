package profiles

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
)

func TestAll_containsKnownProfiles(t *testing.T) {
	want := []string{"cursor", "claude", "codex", "openspec", "bmad", "speckit", "adr", "docs"}
	got := make(map[string]bool)
	for _, p := range All() {
		got[p.ID] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("missing profile id %q", id)
		}
	}
}

func TestByID(t *testing.T) {
	p, ok := ByID("openspec")
	if !ok || p.SourceType != "openspec" {
		t.Fatalf("openspec profile: ok=%v %#v", ok, p)
	}
	if _, ok := ByID("nope"); ok {
		t.Fatal("expected ok=false for unknown id")
	}
}

func TestDetect_findsOpenspecDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := Detect(root, nil)
	if !slices.Contains(got, "openspec") {
		t.Fatalf("want openspec in %v", got)
	}
}

func TestCustomProfile(t *testing.T) {
	cp := CustomProfile()
	if cp.ID != IDCustom {
		t.Fatalf("custom id: got %q", cp.ID)
	}
}

func TestBMADProfileHasPRDSubtypeRule(t *testing.T) {
	p, ok := ByID("bmad")
	if !ok {
		t.Fatal("bmad missing")
	}
	var saw bool
	for _, r := range p.Rules {
		if r.Subtype == config.SubtypePRD && r.Kind == config.KindRequirements {
			saw = true
			break
		}
	}
	if !saw {
		t.Fatal("expected BMAD rule with requirements/prd subtype")
	}
}
