package openspecmetrics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeOpenSpecMetrics(t *testing.T) {
	repoRoot := t.TempDir()
	changeDir := filepath.Join(repoRoot, "openspec", "changes", "add-sso")
	specDir := filepath.Join(changeDir, "specs", "auth")
	nestedChangeDir := filepath.Join(repoRoot, "services", "collector", "openspec", "changes", "add-flow")
	for _, dir := range []string{changeDir, specDir, nestedChangeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{
		"openspec/changes/add-sso/proposal.md",
		"openspec/changes/add-sso/design.md",
		"openspec/changes/add-sso/tasks.md",
		"openspec/changes/add-sso/specs/auth/spec.md",
		"services/collector/openspec/changes/add-flow/proposal.md",
	} {
		if err := os.WriteFile(filepath.Join(repoRoot, filepath.FromSlash(rel)), []byte("# Test\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := Analyze(repoRoot, []Artifact{
		{Path: "openspec/changes/add-sso", SourceType: "openspec", Subtype: "openspec_change_bundle", ArtifactScope: "bundle", OpenSpecRole: "change_bundle"},
		{Path: "openspec/changes/add-sso/proposal.md", SourceType: "openspec", Subtype: "openspec_child", ArtifactScope: "file", OpenSpecRole: "proposal"},
		{Path: "openspec/changes/add-sso/design.md", SourceType: "openspec", Subtype: "openspec_child", ArtifactScope: "file", OpenSpecRole: "design"},
		{Path: "openspec/changes/add-sso/tasks.md", SourceType: "openspec", Subtype: "openspec_child", ArtifactScope: "file", OpenSpecRole: "tasks"},
		{Path: "openspec/changes/add-sso/specs/auth/spec.md", SourceType: "openspec", Subtype: "openspec_child", ArtifactScope: "file", OpenSpecRole: "spec_delta"},
		{Path: "openspec/changes/add-sso/proposal.md", SourceType: "markdown"},
		{Path: "services/collector/openspec/changes/add-flow", SourceType: "openspec", Subtype: "openspec_change_bundle", ArtifactScope: "bundle", OpenSpecRole: "change_bundle"},
		{Path: "services/collector/openspec/changes/add-flow/proposal.md", SourceType: "markdown", Subtype: "spec"},
	})

	if got.ExpectedBundles != 2 || got.IndexedBundles != 2 || got.BundleRecall != 1 {
		t.Fatalf("bundle metrics = %#v", got)
	}
	if got.ExpectedChildArtifacts != 5 || got.IndexedChildArtifacts != 5 || got.ChildRoleRecall != 1 {
		t.Fatalf("child metrics = %#v", got)
	}
	if got.DuplicatePressure != 2.5 {
		t.Fatalf("duplicate pressure = %.3f", got.DuplicatePressure)
	}
	if got.MarkdownLeakage != 2 || len(got.MarkdownLeakagePaths) != 2 {
		t.Fatalf("markdown leakage = %#v", got)
	}
}

func TestAnalyzeOpenSpecMetricsReportsMissingChildren(t *testing.T) {
	repoRoot := t.TempDir()
	changeDir := filepath.Join(repoRoot, "openspec", "changes", "add-sso")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := Analyze(repoRoot, []Artifact{
		{Path: "openspec/changes/add-sso", SourceType: "openspec", Subtype: "openspec_change_bundle"},
	})

	if got.BundleRecall != 1 {
		t.Fatalf("bundle recall = %.3f", got.BundleRecall)
	}
	if got.ChildRoleRecall != 0 {
		t.Fatalf("child role recall = %.3f", got.ChildRoleRecall)
	}
	if len(got.MissingChildRoles) != 1 || got.MissingChildRoles[0] != "proposal:openspec/changes/add-sso/proposal.md" {
		t.Fatalf("missing child roles = %#v", got.MissingChildRoles)
	}
}
