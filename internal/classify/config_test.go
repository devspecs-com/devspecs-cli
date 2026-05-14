package classify

import (
	"path/filepath"
	"testing"
)

func TestDefaultPipelineConfigSupportsDocumentedModels(t *testing.T) {
	cfg := DefaultPipelineConfig()
	if err := ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if missing := MissingDocumentedModels(cfg); len(missing) != 0 {
		t.Fatalf("missing documented models: %v", missing)
	}
	for _, id := range DocumentedModelIDs() {
		if id == "" {
			t.Fatalf("documented model ids must not include empty id: %v", DocumentedModelIDs())
		}
	}
	if !hasScope(cfg.Models[ModelOpenSpec], ScopeContainer) {
		t.Fatal("openspec must support container scope")
	}
	if !hasScope(cfg.Models[ModelOpenSpec], ScopeDocument) {
		t.Fatal("openspec must support document scope for child artifacts")
	}
	for _, id := range []string{ModelADR, ModelRFC, ModelPRD, ModelPlan, ModelAgentNote, ModelGenericMarkdown} {
		if !hasScope(cfg.Models[id], ScopeDocument) {
			t.Fatalf("%s must support document scope", id)
		}
	}
}

func TestValidateConfigRejectsInvalidScope(t *testing.T) {
	cfg := DefaultPipelineConfig()
	model := cfg.Models[ModelADR]
	model.Scopes = []Scope{"directory"}
	cfg.Models[ModelADR] = model
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected invalid scope error")
	}
}

func TestValidateConfigRejectsMissingDocumentedModel(t *testing.T) {
	cfg := DefaultPipelineConfig()
	delete(cfg.Models, ModelPRD)
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected missing documented model error")
	}
}

func TestValidateConfigRejectsBadLocalModelBase(t *testing.T) {
	cfg := DefaultPipelineConfig()
	cfg.LocalModels.Definitions = []LocalModelDefinition{{
		ID:        "engineering_brief",
		BaseModel: "unknown",
	}}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected bad local model base error")
	}
}

func TestReasonVocabularyIncludesPositiveAndNegativeSignals(t *testing.T) {
	got := map[ReasonCode]bool{}
	for _, code := range ReasonVocabulary() {
		got[code] = true
	}
	for _, want := range []ReasonCode{
		ReasonPathHint,
		ReasonLayoutMatch,
		ReasonSubformatEvidence,
		ReasonFamilyEvidence,
		ReasonGeneratedMarker,
		ReasonChangelogMarker,
		ReasonFallback,
	} {
		if !got[want] {
			t.Fatalf("missing reason code %q", want)
		}
	}
}

func TestLoadGoldenFile(t *testing.T) {
	path := filepath.Join("..", "..", "fixtures", "agentic-saas-fragmented", "classifier_cases.yaml")
	g, err := LoadGoldenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if g.Fixture != "agentic-saas-fragmented" {
		t.Fatalf("fixture got %q", g.Fixture)
	}
	if len(g.ClassifierCases) < 6 {
		t.Fatalf("expected at least 6 classifier cases, got %d", len(g.ClassifierCases))
	}
	var sawContainer bool
	for _, c := range g.ClassifierCases {
		if c.Scope == ScopeContainer {
			sawContainer = true
			if len(c.Expected.ChildCandidates) == 0 {
				t.Fatalf("container case %q should define child candidates", c.ID)
			}
		}
	}
	if !sawContainer {
		t.Fatal("expected at least one container classifier case")
	}
}
