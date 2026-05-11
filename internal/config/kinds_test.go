package config

import (
	"strings"
	"testing"
)

func TestValidateKind(t *testing.T) {
	if err := ValidateKind(""); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty kind: %v", err)
	}
	if err := ValidateKind("not_a_kind"); err == nil {
		t.Fatal("expected error")
	}
	for _, k := range []string{
		KindPlan, KindSpec, KindRequirements, KindDesign, KindContract,
		KindDecision, KindMarkdownArtifact,
	} {
		if err := ValidateKind(k); err != nil {
			t.Fatalf("%q: %v", k, err)
		}
		if err := ValidateKind("  " + k + "  "); err != nil {
			t.Fatalf("trim %q: %v", k, err)
		}
	}
}

func TestValidateSubtype(t *testing.T) {
	if err := ValidateSubtype(KindPlan, ""); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSubtype(KindDecision, SubtypeADR); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSubtype(KindSpec, SubtypeOpenspecChange); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSubtype(KindRequirements, SubtypePRD); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSubtype(KindPlan, SubtypeADR); err == nil {
		t.Fatal("plan should not allow adr")
	}
	if err := ValidateSubtype(KindDecision, "nope"); err == nil {
		t.Fatal("expected unknown subtype error")
	}
	if err := ValidateSubtype("", SubtypeADR); err == nil {
		t.Fatal("expected kind error")
	}
}

func TestValidateSourceRules(t *testing.T) {
	if err := ValidateSourceRules(nil); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSourceRules([]SourceRule{{Match: "", Kind: KindPlan}}); err == nil {
		t.Fatal("empty match")
	}
	if err := ValidateSourceRules([]SourceRule{{Match: "*.md", Kind: KindPlan}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSourceRules([]SourceRule{{Match: "*.md", Kind: KindDecision, Subtype: SubtypeADR}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSourceRules([]SourceRule{{Match: "x", Kind: KindPlan, Subtype: SubtypeADR}}); err == nil {
		t.Fatal("invalid subtype for plan")
	}
}

func TestValidateRepoConfig(t *testing.T) {
	if err := ValidateRepoConfig(nil); err != nil {
		t.Fatal(err)
	}
	base := DefaultRepoConfig()
	if err := ValidateRepoConfig(base); err != nil {
		t.Fatal(err)
	}
	bad := &RepoConfig{
		Version: 1,
		Sources: []SourceConfig{
			{Type: "markdown", Rules: []SourceRule{{Match: "x", Kind: "bogus"}}},
		},
	}
	if err := ValidateRepoConfig(bad); err == nil {
		t.Fatal("expected error")
	}
	nonMD := &RepoConfig{
		Version: 1,
		Sources: []SourceConfig{{Type: "openspec", Path: "openspec"}},
	}
	if err := ValidateRepoConfig(nonMD); err != nil {
		t.Fatal(err)
	}
}
