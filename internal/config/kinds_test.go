package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateKind_WithEmptyKind_ReturnsError(t *testing.T) {
	err := ValidateKind("")

	require.Error(t, err)
	assert.ErrorContains(t, err, "empty")
}

func TestValidateKind_WithUnknownKind_ReturnsError(t *testing.T) {
	err := ValidateKind("not_a_kind")

	assert.Error(t, err)
}

func TestValidateKind_WithPlan_ReturnsNoError(t *testing.T) {
	err := ValidateKind(KindPlan)

	assert.NoError(t, err)
}

func TestValidateKind_WithSpec_ReturnsNoError(t *testing.T) {
	err := ValidateKind(KindSpec)

	assert.NoError(t, err)
}

func TestValidateKind_WithRequirements_ReturnsNoError(t *testing.T) {
	err := ValidateKind(KindRequirements)

	assert.NoError(t, err)
}

func TestValidateKind_WithDesign_ReturnsNoError(t *testing.T) {
	err := ValidateKind(KindDesign)

	assert.NoError(t, err)
}

func TestValidateKind_WithContract_ReturnsNoError(t *testing.T) {
	err := ValidateKind(KindContract)

	assert.NoError(t, err)
}

func TestValidateKind_WithDecision_ReturnsNoError(t *testing.T) {
	err := ValidateKind(KindDecision)

	assert.NoError(t, err)
}

func TestValidateKind_WithMarkdownArtifact_ReturnsNoError(t *testing.T) {
	err := ValidateKind(KindMarkdownArtifact)

	assert.NoError(t, err)
}

func TestValidateKind_WithSourceContext_ReturnsNoError(t *testing.T) {
	err := ValidateKind(KindSourceContext)

	assert.NoError(t, err)
}

func TestValidateKind_WithSurroundingWhitespace_ReturnsNoError(t *testing.T) {
	err := ValidateKind("  " + KindPlan + "  ")

	assert.NoError(t, err)
}

func TestValidateSubtype_WithEmptySubtype_ReturnsNoError(t *testing.T) {
	err := ValidateSubtype(KindPlan, "")

	assert.NoError(t, err)
}

func TestValidateSubtype_WithADRDecision_ReturnsNoError(t *testing.T) {
	err := ValidateSubtype(KindDecision, SubtypeADR)

	assert.NoError(t, err)
}

func TestValidateSubtype_WithOpenSpecChange_ReturnsNoError(t *testing.T) {
	err := ValidateSubtype(KindSpec, SubtypeOpenspecChange)

	assert.NoError(t, err)
}

func TestValidateSubtype_WithPRDRequirements_ReturnsNoError(t *testing.T) {
	err := ValidateSubtype(KindRequirements, SubtypePRD)

	assert.NoError(t, err)
}

func TestValidateSubtype_WithAgentInstructionMarkdown_ReturnsNoError(t *testing.T) {
	err := ValidateSubtype(KindMarkdownArtifact, SubtypeAgentInstruction)

	assert.NoError(t, err)
}

func TestValidateSubtype_WithAPIContractMarkdown_ReturnsNoError(t *testing.T) {
	err := ValidateSubtype(KindMarkdownArtifact, SubtypeAPIContract)

	assert.NoError(t, err)
}

func TestValidateSubtype_WithDocumentTemplateMarkdown_ReturnsNoError(t *testing.T) {
	err := ValidateSubtype(KindMarkdownArtifact, SubtypeDocumentTemplate)

	assert.NoError(t, err)
}

func TestValidateSubtype_WithSourceTestCase_ReturnsNoError(t *testing.T) {
	err := ValidateSubtype(KindSourceContext, SubtypeTestCase)

	assert.NoError(t, err)
}

func TestValidateSubtype_WithSubtypeInWrongKind_ReturnsError(t *testing.T) {
	err := ValidateSubtype(KindPlan, SubtypeADR)

	assert.Error(t, err)
}

func TestValidateSubtype_WithUnknownSubtype_ReturnsError(t *testing.T) {
	err := ValidateSubtype(KindDecision, "nope")

	assert.Error(t, err)
}

func TestValidateSubtype_WithEmptyKind_ReturnsError(t *testing.T) {
	err := ValidateSubtype("", SubtypeADR)

	assert.Error(t, err)
}

func TestValidateSourceRules_WithNilRules_ReturnsNoError(t *testing.T) {
	err := ValidateSourceRules(nil)

	assert.NoError(t, err)
}

func TestValidateSourceRules_WithEmptyMatch_ReturnsError(t *testing.T) {
	rules := []SourceRule{{Match: "", Kind: KindPlan}}

	err := ValidateSourceRules(rules)

	assert.Error(t, err)
}

func TestValidateSourceRules_WithPlanRule_ReturnsNoError(t *testing.T) {
	rules := []SourceRule{{Match: "*.md", Kind: KindPlan}}

	err := ValidateSourceRules(rules)

	assert.NoError(t, err)
}

func TestValidateSourceRules_WithADRRule_ReturnsNoError(t *testing.T) {
	rules := []SourceRule{{Match: "*.md", Kind: KindDecision, Subtype: SubtypeADR}}

	err := ValidateSourceRules(rules)

	assert.NoError(t, err)
}

func TestValidateSourceRules_WithSubtypeInWrongKind_ReturnsError(t *testing.T) {
	rules := []SourceRule{{Match: "x", Kind: KindPlan, Subtype: SubtypeADR}}

	err := ValidateSourceRules(rules)

	assert.Error(t, err)
}

func TestValidateRepoConfig_WithNilConfig_ReturnsNoError(t *testing.T) {
	err := ValidateRepoConfig(nil)

	assert.NoError(t, err)
}

func TestValidateRepoConfig_WithDefaultConfig_ReturnsNoError(t *testing.T) {
	cfg := DefaultRepoConfig()

	err := ValidateRepoConfig(cfg)

	assert.NoError(t, err)
}

func TestValidateRepoConfig_WithUnknownRuleKind_ReturnsError(t *testing.T) {
	cfg := &RepoConfig{
		Version: 1,
		Sources: []SourceConfig{
			{Type: "markdown", Rules: []SourceRule{{Match: "x", Kind: "bogus"}}},
		},
	}

	err := ValidateRepoConfig(cfg)

	assert.Error(t, err)
}

func TestValidateRepoConfig_WithNonMarkdownSource_ReturnsNoError(t *testing.T) {
	cfg := &RepoConfig{
		Version: 1,
		Sources: []SourceConfig{{Type: "openspec", Path: "openspec"}},
	}

	err := ValidateRepoConfig(cfg)

	assert.NoError(t, err)
}
