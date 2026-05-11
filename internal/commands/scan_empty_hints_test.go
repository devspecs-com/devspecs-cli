package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupEmptyScanRepo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".cursor", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".cursor", "plans", "hinted.md"), []byte("# Plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
}

func writeMisconfiguredSources(t *testing.T, repoRoot string) {
	t.Helper()
	cfgDir := filepath.Join(repoRoot, ".devspecs")
	cfg := `version: 1
sources:
  - type: openspec
    path: z_missing_openspec
  - type: adr
    paths:
      - z_missing_adr
  - type: markdown
    paths:
      - z_missing_md
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupEmptyScanBareRepo(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	origWd, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
}

func TestScan_EmptyArtifacts_NoCandidates_HumanGenericPlans(t *testing.T) {
	setupEmptyScanBareRepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	writeMisconfiguredSources(t, wd)

	scanCmd := NewScanCmd()
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "No artifacts found in configured paths.") {
		t.Fatalf("expected empty-scan header, got:\n%s", out)
	}
	if strings.Contains(out, "Possible candidates:") {
		t.Fatalf("did not expect candidate list when none on disk, got:\n%s", out)
	}
	if !strings.Contains(out, "No on-disk candidate directories matched built-in heuristics.") {
		t.Fatalf("expected no-candidates message, got:\n%s", out)
	}
	if !strings.Contains(out, "ds config add-source markdown plans") {
		t.Fatalf("expected generic plans example, got:\n%s", out)
	}
}

func TestScan_EmptyArtifacts_NoCandidates_JSONOmitsHintsKey(t *testing.T) {
	setupEmptyScanBareRepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	writeMisconfiguredSources(t, wd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json"})
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &top); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	if _, ok := top["hints"]; ok {
		t.Fatalf("expected hints key omitted when no candidates, got keys: %v", keysOfRawMap(top))
	}
	var found map[string]int
	if err := json.Unmarshal(top["Found"], &found); err != nil {
		t.Fatal(err)
	}
	if found["markdown"] != 0 || found["openspec"] != 0 || found["adr"] != 0 {
		t.Fatalf("expected zero Found, got %#v", found)
	}
}

func keysOfRawMap(m map[string]json.RawMessage) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func TestScan_EmptyArtifacts_HumanHintsAndExit0(t *testing.T) {
	setupEmptyScanRepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	writeMisconfiguredSources(t, wd)

	scanCmd := NewScanCmd()
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "No artifacts found in configured paths.") {
		t.Fatalf("expected empty-scan header, got:\n%s", out)
	}
	if !strings.Contains(out, "Possible candidates:") {
		t.Fatalf("expected candidates section, got:\n%s", out)
	}
	if !strings.Contains(out, "ds config add-source") {
		t.Fatalf("expected add-source example, got:\n%s", out)
	}
	const maxLines = 30
	if strings.Count(out, "\n") > maxLines {
		t.Fatalf("expected bounded output (≤%d lines), got %d lines", maxLines, strings.Count(out, "\n"))
	}
}

func TestScan_EmptyArtifacts_JSONHints(t *testing.T) {
	setupEmptyScanRepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	writeMisconfiguredSources(t, wd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json"})
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Found map[string]int `json:"Found"`
		Hints []struct {
			Path           string `json:"path"`
			SourceType     string `json:"source_type"`
			SuggestCommand string `json:"suggest_command"`
		} `json:"hints"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	if payload.Found["markdown"] != 0 || payload.Found["openspec"] != 0 || payload.Found["adr"] != 0 {
		t.Fatalf("expected zero Found, got %#v", payload.Found)
	}
	if len(payload.Hints) == 0 {
		t.Fatalf("expected non-empty hints, got %s", buf.String())
	}
	found := false
	for _, h := range payload.Hints {
		if h.Path == ".cursor/plans" && strings.Contains(h.SuggestCommand, "ds config add-source markdown") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected .cursor/plans markdown hint, got %#v", payload.Hints)
	}
}

func TestScan_EmptyArtifacts_QuietSuppressesHumanHints(t *testing.T) {
	setupEmptyScanRepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	writeMisconfiguredSources(t, wd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--quiet"})
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buf.String()) != "" {
		t.Fatalf("expected no stdout with --quiet, got %q", buf.String())
	}
}

func TestScan_EmptyArtifacts_JSON_QuietStillIncludesHints(t *testing.T) {
	setupEmptyScanRepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	writeMisconfiguredSources(t, wd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json", "--quiet"})
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"hints"`) {
		t.Fatalf("expected hints in JSON with --quiet, got %s", buf.String())
	}
}

func TestScan_NonEmpty_NoHintBlock(t *testing.T) {
	setupE2ERepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	scanCmd := NewScanCmd()
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "No artifacts found in configured paths.") {
		t.Fatalf("did not expect empty-scan message in normal repo, got:\n%s", out)
	}
	if !strings.Contains(out, "Indexed by source:") {
		t.Fatalf("expected normal scan header, got:\n%s", out)
	}
}

func TestScan_JSON_NonEmpty_OmitsHints(t *testing.T) {
	setupE2ERepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json"})
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	if err := scanCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `"hints"`) {
		t.Fatalf("did not expect hints in non-empty JSON, got %s", buf.String())
	}
}
