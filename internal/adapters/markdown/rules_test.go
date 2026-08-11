package markdown

import (
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestMatchSourceRules_WithRootRoadmap_ReturnsPlan(t *testing.T) {
	paths, rules := sourceRuleFixture()

	kind, subtype, tags, matched := MatchSourceRules("ROADMAP.md", paths, rules)

	assert.True(t, matched)
	assert.Equal(t, config.KindPlan, kind)
	assert.Empty(t, subtype)
	assert.Empty(t, tags)
}

func TestMatchSourceRules_WithDecisionFile_ReturnsDecision(t *testing.T) {
	paths, rules := sourceRuleFixture()

	kind, _, _, matched := MatchSourceRules("decisions/001-x.md", paths, rules)

	assert.True(t, matched)
	assert.Equal(t, config.KindDecision, kind)
}

func TestMatchSourceRules_WithNumberedPlan_ReturnsPlan(t *testing.T) {
	paths, rules := sourceRuleFixture()

	kind, _, _, matched := MatchSourceRules("v2/plans/02_FOO.md", paths, rules)

	assert.True(t, matched)
	assert.Equal(t, config.KindPlan, kind)
}

func TestMatchSourceRules_WithNestedReadme_ReturnsPlan(t *testing.T) {
	paths, rules := sourceRuleFixture()

	kind, _, _, matched := MatchSourceRules("v2/plans/sub/README.md", paths, rules)

	assert.True(t, matched)
	assert.Equal(t, config.KindPlan, kind)
}

func sourceRuleFixture() ([]string, []config.SourceRule) {
	return []string{".", "v2/plans", "decisions"}, []config.SourceRule{
		{Match: "ROADMAP.md", Kind: config.KindPlan},
		{Match: "README.md", Kind: config.KindPlan},
		{Match: "*/README.md", Kind: config.KindPlan},
		{Match: "[0-9][0-9]_*.md", Kind: config.KindPlan},
		{Match: "*/[0-9][0-9]-*.md", Kind: config.KindPlan},
		{Match: "decisions/*.md", Kind: config.KindDecision},
	}
}
