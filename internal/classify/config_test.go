package classify

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateConfig_WithDefaultPipelineConfig_ReturnsNoError(t *testing.T) {
	cfg := DefaultPipelineConfig()

	err := ValidateConfig(cfg)

	assert.NoError(t, err)
}

func TestMissingDocumentedModels_WithDefaultPipelineConfig_ReturnsEmpty(t *testing.T) {
	cfg := DefaultPipelineConfig()

	missing := MissingDocumentedModels(cfg)

	assert.Empty(t, missing)
}

func TestDefaultPipelineConfig_SupportsDocumentedModelScopes(t *testing.T) {
	cfg := DefaultPipelineConfig()

	assert.True(t, hasScope(cfg.Models[ModelOpenSpec], ScopeContainer))
	assert.True(t, hasScope(cfg.Models[ModelOpenSpec], ScopeDocument))
	assert.True(t, hasScope(cfg.Models[ModelADR], ScopeDocument))
	assert.True(t, hasScope(cfg.Models[ModelRFC], ScopeDocument))
	assert.True(t, hasScope(cfg.Models[ModelPRD], ScopeDocument))
	assert.True(t, hasScope(cfg.Models[ModelPlan], ScopeDocument))
	assert.True(t, hasScope(cfg.Models[ModelAgentNote], ScopeDocument))
	assert.True(t, hasScope(cfg.Models[ModelProtocol], ScopeDocument))
	assert.True(t, hasScope(cfg.Models[ModelStructuredModel], ScopeDocument))
	assert.True(t, hasScope(cfg.Models[ModelTemplate], ScopeDocument))
	assert.True(t, hasScope(cfg.Models[ModelGenericMarkdown], ScopeDocument))
}

func TestDocumentedModelIDs_ReturnsOnlyNonEmptyIDs(t *testing.T) {
	ids := DocumentedModelIDs()

	require.NotEmpty(t, ids)
	assert.NotContains(t, ids, "")
}

func TestValidateConfigRejectsInvalidScope(t *testing.T) {
	cfg := DefaultPipelineConfig()
	model := cfg.Models[ModelADR]
	model.Scopes = []Scope{"directory"}
	cfg.Models[ModelADR] = model
	err := ValidateConfig(cfg)

	assert.Error(t, err)
}

func TestValidateConfigRejectsMissingDocumentedModel(t *testing.T) {
	cfg := DefaultPipelineConfig()
	delete(cfg.Models, ModelPRD)
	err := ValidateConfig(cfg)

	assert.Error(t, err)
}

func TestValidateConfigRejectsBadLocalModelBase(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.LocalModels.Definitions = []LocalModelDefinition{{
		ID:        "engineering_brief",
		BaseModel: "unknown",
	}}
	err := ValidateConfig(cfg)

	assert.Error(t, err)
}

func TestValidateConfigRejectsBadEvidenceRule(t *testing.T) {
	cfg := DefaultPipelineConfig()
	model := cfg.Models[ModelADR]
	model.Evidence = append(model.Evidence, EvidenceRule{
		ID:     "bad_weight",
		Weight: 1.5,
		Reason: ReasonHeadingMatch,
		Match:  EvidenceMatch{Scope: ScopeDocument, HeadingsAny: []string{"Decision"}},
	})
	cfg.Models[ModelADR] = model
	err := ValidateConfig(cfg)

	assert.Error(t, err)
}

func TestDefaultPipelineConfigUsesDeclarativeEvidenceRules(t *testing.T) {
	cfg := DefaultPipelineConfig()

	assert.NotEmpty(t, cfg.Models[ModelOpenSpec].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelADR].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelRFC].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelPRD].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelPlan].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelAgentNote].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelProtocol].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelStructuredModel].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelTemplate].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelGenericMarkdown].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelADR].Subformats["nygard"].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelPRD].Families["product_intent"].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelProtocol].Families["agent_instruction"].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelTemplate].Families["document_template"].Evidence)
	assert.NotEmpty(t, cfg.Models[ModelStructuredModel].Families["api_contract"].Evidence)
}

func TestReasonVocabularyIncludesPositiveAndNegativeSignals(t *testing.T) {
	got := map[ReasonCode]bool{}
	for _, code := range ReasonVocabulary() {
		got[code] = true
	}

	assert.True(t, got[ReasonPathHint])
	assert.True(t, got[ReasonLayoutMatch])
	assert.True(t, got[ReasonSubformatEvidence])
	assert.True(t, got[ReasonFamilyEvidence])
	assert.True(t, got[ReasonGeneratedMarker])
	assert.True(t, got[ReasonTemplateMarker])
	assert.True(t, got[ReasonChangelogMarker])
	assert.True(t, got[ReasonFallback])
}

func TestLoadGoldenFile(t *testing.T) {
	path := filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented", "classifier_cases.yaml")
	g, err := LoadGoldenFile(path)
	require.NoError(t, err)

	assert.Equal(t, "agentic-saas-fragmented", g.Fixture)
	assert.GreaterOrEqual(t, len(g.ClassifierCases), 6,
		"expected at least 6 classifier cases, got %d", len(g.ClassifierCases))

	var sawContainer bool
	for _, c := range g.ClassifierCases {
		if c.Scope == ScopeContainer {
			sawContainer = true
			assert.NotEmpty(t, c.Expected.ChildCandidates,
				"container case %q should define child candidates", c.ID)

		}
	}
	assert.True(t, sawContainer,
		"expected at least one container classifier case")
}
