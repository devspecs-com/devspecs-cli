package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupEmptyScanRepo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	{
		err := os.MkdirAll(filepath.Join(repoDir, ".cursor", "plans"), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(filepath.Join(repoDir, ".cursor", "plans", "hinted.md"), []byte("# Plan\n"), 0o644)
		require.NoError(t, err)
	}

	origWd := testWorkingDirectory(t)
	{
		err := os.Chdir(repoDir)
		require.NoError(t, err)
	}

	t.Cleanup(func() { _ = os.Chdir(origWd) })
}

func writeMisconfiguredSources(t *testing.T, repoRoot string) {
	t.Helper()
	cfgDir := filepath.Join(repoRoot, ".devspecs")
	cfg := `version: 1
experiments:
  intent_candidate_discovery: false
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
	{
		err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644)
		require.NoError(t, err)
	}

}

func setupEmptyScanBareRepo(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	{
		err := os.MkdirAll(repoDir, 0o755)
		require.NoError(t, err)
	}

	origWd := testWorkingDirectory(t)
	{
		err := os.Chdir(repoDir)
		require.NoError(t, err)
	}

	t.Cleanup(func() { _ = os.Chdir(origWd) })
}

func TestScan_EmptyArtifacts_NoCandidates_HumanGenericPlans(t *testing.T) {
	setupEmptyScanBareRepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	{
		err := initCmd.Execute()
		require.NoError(t, err)
	}

	wd := testWorkingDirectory(t)
	writeMisconfiguredSources(t, wd)

	scanCmd := NewScanCmd()
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err,
			"scan: %v", err)
	}

	out := buf.String()
	assert.Contains(t, out, "No artifacts found in configured paths.",
		"expected empty-scan header, got:\n%s", out)
	assert.NotContains(t, out, "Possible candidates:",
		"did not expect candidate list when none on disk, got:\n%s", out)
	assert.Contains(t, out, "No on-disk candidate directories matched built-in heuristics.",
		"expected no-candidates message, got:\n%s", out)
	assert.Contains(t, out, "ds config add-source markdown plans",
		"expected generic plans example, got:\n%s", out)

}

func TestScan_EmptyArtifacts_NoCandidates_JSONOmitsHintsKey(t *testing.T) {
	setupEmptyScanBareRepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	{
		err := initCmd.Execute()
		require.NoError(t, err)
	}

	wd := testWorkingDirectory(t)
	writeMisconfiguredSources(t, wd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json"})
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}

	var top map[string]json.RawMessage
	{
		err := json.Unmarshal(buf.Bytes(), &top)
		require.NoError(t, err,
			"json: %v\n%s", err, buf.String())
	}
	{

		_, ok := top["hints"]
		assert.False(t, ok,
			"expected hints key omitted when no candidates, got keys: %v", keysOfRawMap(top))
	}

	var found map[string]int
	{
		err := json.Unmarshal(top["Found"], &found)
		require.NoError(t, err)
	}
	assert.Equal(t, 0, found["markdown"], "expected zero Found, got %#v", found)
	assert.Equal(t, 0, found["openspec"], "expected zero Found, got %#v", found)
	assert.Equal(t, 0, found["adr"], "expected zero Found, got %#v", found)

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
	{
		err := initCmd.Execute()
		require.NoError(t, err)
	}

	wd := testWorkingDirectory(t)
	writeMisconfiguredSources(t, wd)

	scanCmd := NewScanCmd()
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err,
			"scan: %v", err)
	}

	out := buf.String()
	assert.Contains(t, out, "No artifacts found in configured paths.",
		"expected empty-scan header, got:\n%s", out)
	assert.Contains(t, out, "Possible candidates:",
		"expected candidates section, got:\n%s", out)
	assert.Contains(t, out, "ds config add-source",
		"expected add-source example, got:\n%s", out)

	const maxLines = 30
	assert.LessOrEqual(t, strings.Count(out, "\n"), maxLines,
		"expected bounded output (≤%d lines), got %d lines", maxLines, strings.Count(out, "\n"))

}

func TestScan_EmptyArtifacts_JSONHints(t *testing.T) {
	setupEmptyScanRepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	{
		err := initCmd.Execute()
		require.NoError(t, err)
	}

	wd := testWorkingDirectory(t)
	writeMisconfiguredSources(t, wd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json"})
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}

	var payload struct {
		Found map[string]int `json:"Found"`
		Hints []struct {
			Path           string `json:"path"`
			SourceType     string `json:"source_type"`
			SuggestCommand string `json:"suggest_command"`
		} `json:"hints"`
	}
	{
		err := json.Unmarshal(buf.Bytes(), &payload)
		require.NoError(t, err,
			"json: %v\n%s", err, buf.String())
	}
	assert.Equal(t, 0, payload.Found["markdown"], "expected zero Found, got %#v", payload.Found)
	assert.Equal(t, 0, payload.Found["openspec"], "expected zero Found, got %#v", payload.Found)
	assert.Equal(t, 0, payload.Found["adr"], "expected zero Found, got %#v", payload.Found)
	assert.Equal(t, 0, payload.Found["source_context"], "expected zero Found, got %#v", payload.Found)
	require.NotEmpty(t, payload.Hints,
		"expected non-empty hints, got %s", buf.String())

	found := false
	for _, h := range payload.Hints {
		if h.Path == ".cursor/plans" && strings.Contains(h.SuggestCommand, "ds config add-source markdown") {
			found = true
		}
	}
	assert.True(t, found,
		"expected .cursor/plans markdown hint, got %#v", payload.Hints)

}

func TestScan_EmptyArtifacts_QuietSuppressesHumanHints(t *testing.T) {
	setupEmptyScanRepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	{
		err := initCmd.Execute()
		require.NoError(t, err)
	}

	wd := testWorkingDirectory(t)
	writeMisconfiguredSources(t, wd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--quiet"})
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}
	assert.Equal(t, "", strings.TrimSpace(buf.String()),
		"expected no stdout with --quiet, got %q", buf.String())

}

func TestScan_EmptyArtifacts_JSON_QuietStillIncludesHints(t *testing.T) {
	setupEmptyScanRepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	{
		err := initCmd.Execute()
		require.NoError(t, err)
	}

	wd := testWorkingDirectory(t)
	writeMisconfiguredSources(t, wd)

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json", "--quiet"})
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}
	assert.Contains(t, buf.String(), `"hints"`,
		"expected hints in JSON with --quiet, got %s", buf.String())

}

func TestScan_NonEmpty_NoHintBlock(t *testing.T) {
	setupE2ERepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	{
		err := initCmd.Execute()
		require.NoError(t, err)
	}

	scanCmd := NewScanCmd()
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}

	out := buf.String()
	assert.NotContains(t, out, "No artifacts found in configured paths.",
		"did not expect empty-scan message in normal repo, got:\n%s", out)
	assert.Contains(t, out, "Indexed by source:",
		"expected normal scan header, got:\n%s", out)

}

func TestScan_JSON_NonEmpty_OmitsHints(t *testing.T) {
	setupE2ERepo(t)
	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	{
		err := initCmd.Execute()
		require.NoError(t, err)
	}

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json"})
	var buf bytes.Buffer
	scanCmd.SetOut(&buf)
	{
		err := scanCmd.Execute()
		require.NoError(t, err)
	}
	assert.NotContains(t, buf.String(), `"hints"`,
		"did not expect hints in non-empty JSON, got %s", buf.String())

}
