package markdown

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
)

func TestMatchSourceRules(t *testing.T) {
	paths := []string{".", "v2/plans", "decisions"}
	rules := []config.SourceRule{
		{Match: "ROADMAP.md", Kind: config.KindPlan},
		{Match: "README.md", Kind: config.KindPlan},
		{Match: "*/README.md", Kind: config.KindPlan},
		{Match: "[0-9][0-9]_*.md", Kind: config.KindPlan},
		{Match: "*/[0-9][0-9]-*.md", Kind: config.KindPlan},
		{Match: "decisions/*.md", Kind: config.KindDecision},
	}

	k, sub, tags, ok := MatchSourceRules("ROADMAP.md", paths, rules)
	if !ok || k != config.KindPlan || sub != "" || len(tags) != 0 {
		t.Fatalf("ROADMAP.md: got kind=%q subtype=%q ok=%v tags=%v", k, sub, ok, tags)
	}

	k, sub, _, ok = MatchSourceRules("decisions/001-x.md", paths, rules)
	if !ok || k != config.KindDecision {
		t.Fatalf("decisions file: kind=%q ok=%v", k, ok)
	}

	k, sub, _, ok = MatchSourceRules("v2/plans/02_FOO.md", paths, rules)
	if !ok || k != config.KindPlan {
		t.Fatalf("numbered root file: kind=%q ok=%v", k, ok)
	}

	k, sub, _, ok = MatchSourceRules("v2/plans/sub/README.md", paths, rules)
	if !ok || k != config.KindPlan {
		t.Fatalf("nested readme: kind=%q ok=%v", k, ok)
	}
}
