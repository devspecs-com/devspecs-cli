package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupE2ERepo creates a fixture repo with OpenSpec, ADR, and markdown plan.
func setupE2ERepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")

	// OpenSpec
	osDir := filepath.Join(repoDir, "openspec", "changes", "add-sso")
	require.NoError(t, os.MkdirAll(osDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(osDir, "proposal.md"), []byte("# Add SSO\n\n## Acceptance Criteria\n\n- SSO works\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(osDir, "tasks.md"), []byte("# Tasks\n\n- [ ] Implement\n- [x] Design\n"), 0o644))

	// ADR
	adrDir := filepath.Join(repoDir, "docs", "adrs")
	require.NoError(t, os.MkdirAll(adrDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(adrDir, "0001-use-authjs.md"), []byte("# Use Auth.js\n\nStatus: Accepted\n"), 0o644))

	// Plan
	planDir := filepath.Join(repoDir, "plans")
	require.NoError(t, os.MkdirAll(planDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(planDir, "refactor-auth.md"), []byte("# Refactor auth\n\n- [ ] Extract middleware\n"), 0o644))

	// Config
	cfgDir := filepath.Join(repoDir, ".devspecs")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte("version: 1\nsources:\n  - type: openspec\n    path: openspec\n  - type: adr\n    paths:\n      - docs/adrs\n  - type: markdown\n    paths:\n      - plans\n"), 0o644))

	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() {
		assert.NoError(t, os.Chdir(origWd))
	})
	return repoDir
}

// DOD §21 bullet 1: Install via go install or binary, run ds --version.
func TestDOD01VersionCommandPrintsVersion(t *testing.T) {
	cmd := NewVersionCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "ds ")
}

func TestDOD01VersionJSONIncludesVersion(t *testing.T) {
	cmd := NewVersionCmd()
	cmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var obj map[string]string
	require.NoError(t, json.Unmarshal(buf.Bytes(), &obj))
	assert.Contains(t, obj, "version")
}

// DOD §21 bullet 2: Initialize DevSpecs in an existing repo.
func TestDOD_02_InitInRepo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() {
		assert.NoError(t, os.Chdir(origWd))
	})

	cmd := NewInitCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	err = cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Initialized DevSpecs.")

	dbPath := filepath.Join(tmp, "home", "devspecs.db")
	assert.FileExists(t, dbPath)
}

// DOD §21 bullet 3: Scan existing OpenSpec/ADR/markdown planning artifacts.
func TestDOD_03_ScanArtifacts(t *testing.T) {
	setupE2ERepo(t)

	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, initCmd.Execute())

	scanCmd := NewScanCmd()
	scanCmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	scanCmd.SetOut(buf)

	err := scanCmd.Execute()

	require.NoError(t, err)
	var out struct {
		Found            map[string]int `json:"Found"`
		SourcesBreakdown []struct {
			SourceType string         `json:"source_type"`
			Label      string         `json:"label"`
			Count      int            `json:"count"`
			Formats    map[string]int `json:"formats"`
		} `json:"sources_breakdown"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, 4, out.Found["openspec"],
		"expected 4 openspec found")
	assert.Equal(t, 1, out.Found["adr"],
		"expected 1 adr found")
	assert.Equal(t, 1, out.Found["markdown"],
		"expected 1 markdown found")
	require.Lenf(t, out.SourcesBreakdown, 4,
		"sources_breakdown: want 4 rows, got %d", len(out.SourcesBreakdown))

	var sumCount int
	for _, row := range out.SourcesBreakdown {
		assert.NotEmpty(t, row.SourceType)
		assert.NotEmpty(t, row.Label)
		sumCount += row.Count
		sumFormats := 0
		for _, c := range row.Formats {
			sumFormats += c
		}
		assert.Equalf(t, row.Count, sumFormats,
			"formats sum %d != count %d for %s", sumFormats, row.Count, row.SourceType)

	}
	assert.Equalf(t, 6, sumCount,
		"sources_breakdown count sum: want 6, got %d", sumCount)

}

// DOD §21 bullet 4: See a list of detected artifacts.
func TestDOD_04_ListArtifacts(t *testing.T) {
	setupE2ERepo(t)
	require.NoError(t, NewInitCmd().Execute())

	scanCmd := NewScanCmd()
	scanCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, scanCmd.Execute())

	listCmd := NewListCmd()
	buf := &bytes.Buffer{}
	listCmd.SetOut(buf)

	err := listCmd.Execute()

	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "openspec_change",
		"list output missing openspec_change")
	assert.Contains(t, output, "adr",
		"list output missing adr")
	assert.Contains(t, output, "plan",
		"list output missing plan")

}

func setupScannedE2EArtifactID(t *testing.T) string {
	t.Helper()
	setupE2ERepo(t)
	require.NoError(t, NewInitCmd().Execute())
	scanCmd := NewScanCmd()
	scanCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, scanCmd.Execute())
	listCmd := NewListCmd()
	listCmd.SetArgs([]string{"--json"})
	listBuf := &bytes.Buffer{}
	listCmd.SetOut(listBuf)
	require.NoError(t, listCmd.Execute())
	var artifacts []map[string]any
	require.NoError(t, json.Unmarshal(listBuf.Bytes(), &artifacts))
	require.NotEmpty(t, artifacts)
	id, ok := artifacts[0]["ID"].(string)
	require.True(t, ok)
	return id
}

// DOD §21 bullet 5: Resolve any artifact by stable ID.
func TestDOD_05_ResolveByID(t *testing.T) {
	id := setupScannedE2EArtifactID(t)
	resolveCmd := NewResolveCmd()
	resolveCmd.SetArgs([]string{id})
	resBuf := &bytes.Buffer{}
	resolveCmd.SetOut(resBuf)

	err := resolveCmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, resBuf.String(), id,
		"resolve did not contain the artifact ID")

}

// DOD §21 bullet 6: Export agent-ready context for an artifact.
func TestDOD_06_ExportContext(t *testing.T) {
	id := setupScannedE2EArtifactID(t)
	ctxCmd := NewContextCmd()
	ctxCmd.SetArgs([]string{id})
	ctxBuf := &bytes.Buffer{}
	ctxCmd.SetOut(ctxBuf)

	err := ctxCmd.Execute()

	require.NoError(t, err)
	output := ctxBuf.String()
	assert.Contains(t, output, "# DevSpecs Context:",
		"context output missing header")
	assert.Contains(t, output, "## Instructions for Agent",
		"context output missing instructions section")

}

// DOD §21 bullet 7: Capture a one-off markdown plan.
func TestDOD_07_CaptureOneOff(t *testing.T) {
	setupE2ERepo(t)
	require.NoError(t, NewInitCmd().Execute())

	require.NoError(t, os.WriteFile("oneoff-plan.md", []byte("# One-off Plan\n\n- [ ] Do something\n"), 0o644))

	captureCmd := NewCaptureCmd()
	captureCmd.SetArgs([]string{"oneoff-plan.md", "--kind", "plan"})
	buf := &bytes.Buffer{}
	captureCmd.SetOut(buf)

	err := captureCmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "ds_",
		"capture did not return a DevSpecs ID")

}

// DOD §21 bullet 8: Mark status manually.
func TestDOD_08_ManualStatus(t *testing.T) {
	id := setupScannedE2EArtifactID(t)
	statusCmd := NewStatusCmd()
	statusCmd.SetArgs([]string{id, "approved"})
	statusBuf := &bytes.Buffer{}
	statusCmd.SetOut(statusBuf)

	err := statusCmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, statusBuf.String(), "approved",
		"status update did not confirm 'approved'")

}

// DOD §21 bullet 9: Link an artifact to an external URL.
func TestDOD_09_LinkArtifact(t *testing.T) {
	id := setupScannedE2EArtifactID(t)
	linkCmd := NewLinkCmd()
	linkCmd.SetArgs([]string{id, "https://github.com/acme/backend/pull/42"})
	linkBuf := &bytes.Buffer{}
	linkCmd.SetOut(linkBuf)

	err := linkCmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, linkBuf.String(), "Linked",
		"link did not confirm")

}

// DOD §21 bullet 10: Re-scan without creating duplicates.
func TestDOD_10_RescanNoDuplicates(t *testing.T) {
	setupE2ERepo(t)
	require.NoError(t, NewInitCmd().Execute())

	scan1 := NewScanCmd()
	scan1.SetOut(&bytes.Buffer{})
	require.NoError(t, scan1.Execute())

	scan2 := NewScanCmd()
	scan2.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	scan2.SetOut(buf)

	err := scan2.Execute()

	require.NoError(t, err)
	var result struct {
		New       int `json:"New"`
		Unchanged int `json:"Unchanged"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.Zero(t, result.New)
	assert.Equal(t, 6, result.Unchanged)

}

// DOD §21 bullet 11: Change a source file and see a new revision tracked.
func TestDOD_11_RescanRevision(t *testing.T) {
	setupE2ERepo(t)
	require.NoError(t, NewInitCmd().Execute())

	scan1 := NewScanCmd()
	scan1.SetOut(&bytes.Buffer{})
	require.NoError(t, scan1.Execute())

	require.NoError(t, os.WriteFile("plans/refactor-auth.md", []byte("# Refactor auth v2\n\n- [ ] New task\n- [x] Old task done\n"), 0o644))

	scan2 := NewScanCmd()
	scan2.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	scan2.SetOut(buf)

	err := scan2.Execute()

	require.NoError(t, err)
	var result struct {
		Updated int `json:"Updated"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	assert.GreaterOrEqual(t, result.Updated, 1)

}

// Error handling tests per spec §12.
func TestErrors_UnknownID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	origWd, setupErr := os.Getwd()
	require.NoError(t, setupErr)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() {
		assert.NoError(t, os.Chdir(origWd))
	})

	require.NoError(t, NewInitCmd().Execute())

	showCmd := NewShowCmd()
	showCmd.SetArgs([]string{"ds_nonexistent"})
	showCmd.SetOut(&bytes.Buffer{})
	err := showCmd.Execute()
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found")

}

func TestErrors_StatusVocab(t *testing.T) {
	statusCmd := NewStatusCmd()
	statusCmd.SetArgs([]string{"ds_fake", "bogus"})
	statusCmd.SetOut(&bytes.Buffer{})
	err := statusCmd.Execute()
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid status")

}

func TestErrors_NoIndex(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "no-db-here"))

	listCmd := NewListCmd()
	listCmd.SetOut(&bytes.Buffer{})
	err := listCmd.Execute()
	assert.NoError(t, err)
}

func TestErrors_NoArtifacts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	origWd, setupErr := os.Getwd()
	require.NoError(t, setupErr)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() {
		assert.NoError(t, os.Chdir(origWd))
	})

	require.NoError(t, NewInitCmd().Execute())

	listCmd := NewListCmd()
	buf := &bytes.Buffer{}
	listCmd.SetOut(buf)
	err := listCmd.Execute()
	require.NoError(t, err)

	// Should produce empty list (just headers), not an error
	output := buf.String()
	assert.Containsf(t, output, "ID",
		"list with no artifacts should still show headers, got %q", output)

}

func TestErrors_MalformedConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	repoDir := filepath.Join(tmp, "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	origWd, setupErr := os.Getwd()
	require.NoError(t, setupErr)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() {
		assert.NoError(t, os.Chdir(origWd))
	})

	require.NoError(t, NewInitCmd().Execute())

	// Corrupt the config file
	cfgDir := filepath.Join(repoDir, ".devspecs")
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(":::not:::yaml"), 0o644))

	scanCmd := NewScanCmd()
	scanCmd.SetOut(&bytes.Buffer{})
	err := scanCmd.Execute()
	assert.Error(t, err)

}

func setupJSONStabilityArtifactID(t *testing.T) string {
	t.Helper()
	return setupScannedE2EArtifactID(t)
}

func TestScanJSONProducesValidJSON(t *testing.T) {
	setupJSONStabilityArtifactID(t)
	cmd := NewScanCmd()
	cmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, json.Valid(buf.Bytes()), "scan output must be valid JSON: %s", buf.String())
}

func TestListJSONProducesValidJSON(t *testing.T) {
	setupJSONStabilityArtifactID(t)
	cmd := NewListCmd()
	cmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, json.Valid(buf.Bytes()), "list output must be valid JSON: %s", buf.String())
}

func TestTodosJSONProducesValidJSON(t *testing.T) {
	setupJSONStabilityArtifactID(t)
	cmd := NewTodosCmd()
	cmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, json.Valid(buf.Bytes()), "todos output must be valid JSON: %s", buf.String())
}

func TestCriteriaJSONProducesValidJSON(t *testing.T) {
	setupJSONStabilityArtifactID(t)
	cmd := NewCriteriaCmd()
	cmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, json.Valid(buf.Bytes()), "criteria output must be valid JSON: %s", buf.String())
}

func TestShowJSONProducesValidJSON(t *testing.T) {
	artID := setupJSONStabilityArtifactID(t)
	cmd := NewShowCmd()
	cmd.SetArgs([]string{artID, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, json.Valid(buf.Bytes()), "show output must be valid JSON: %s", buf.String())
}

func TestFindJSONProducesValidJSON(t *testing.T) {
	setupJSONStabilityArtifactID(t)
	cmd := NewFindCmd()
	cmd.SetArgs([]string{"auth", "--json", "--plain"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, json.Valid(buf.Bytes()), "find output must be valid JSON: %s", buf.String())
}

func TestResolveJSONProducesValidJSON(t *testing.T) {
	artID := setupJSONStabilityArtifactID(t)
	cmd := NewResolveCmd()
	cmd.SetArgs([]string{artID, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, json.Valid(buf.Bytes()), "resolve output must be valid JSON: %s", buf.String())
}

func TestContextJSONProducesValidJSON(t *testing.T) {
	artID := setupJSONStabilityArtifactID(t)
	cmd := NewContextCmd()
	cmd.SetArgs([]string{artID, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	assert.True(t, json.Valid(buf.Bytes()), "context output must be valid JSON: %s", buf.String())
}

// TestPRD_TodosBoundary verifies the todos feature stays within PRD scope.
func TestPRD_TodosBoundary(t *testing.T) {
	todosCmd := NewTodosCmd()

	assert.Nil(t, todosCmd.Flags().Lookup("owner"))
	assert.Nil(t, todosCmd.Flags().Lookup("assignee"))
	assert.Nil(t, todosCmd.Flags().Lookup("due-date"))
	assert.Nil(t, todosCmd.Flags().Lookup("due_date"))
	assert.Nil(t, todosCmd.Flags().Lookup("priority"))
	assert.Nil(t, todosCmd.Flags().Lookup("label"))
	assert.Nil(t, todosCmd.Flags().Lookup("sprint"))
	assert.Nil(t, todosCmd.Flags().Lookup("create"))
	assert.Nil(t, todosCmd.Flags().Lookup("update"))
	assert.Nil(t, todosCmd.Flags().Lookup("delete"))
	assert.Nil(t, todosCmd.Flags().Lookup("assign"))
	assert.Nil(t, todosCmd.Flags().Lookup("milestone"))
	assert.Nil(t, todosCmd.Flags().Lookup("epic"))
	assert.Nil(t, todosCmd.Flags().Lookup("estimate"))
	assert.Empty(t, todosCmd.Commands())
}

// TestPRD_CriteriaBoundary verifies the criteria command stays within PRD scope.
func TestPRD_CriteriaBoundary(t *testing.T) {
	criteriaCmd := NewCriteriaCmd()

	assert.Nil(t, criteriaCmd.Flags().Lookup("owner"))
	assert.Nil(t, criteriaCmd.Flags().Lookup("assignee"))
	assert.Nil(t, criteriaCmd.Flags().Lookup("due-date"))
	assert.Nil(t, criteriaCmd.Flags().Lookup("due_date"))
	assert.Nil(t, criteriaCmd.Flags().Lookup("priority"))
	assert.Nil(t, criteriaCmd.Flags().Lookup("label"))
	assert.Nil(t, criteriaCmd.Flags().Lookup("sprint"))
	assert.Nil(t, criteriaCmd.Flags().Lookup("create"))
	assert.Nil(t, criteriaCmd.Flags().Lookup("update"))
	assert.Nil(t, criteriaCmd.Flags().Lookup("delete"))
	assert.Nil(t, criteriaCmd.Flags().Lookup("assign"))
	assert.Nil(t, criteriaCmd.Flags().Lookup("milestone"))
	assert.Nil(t, criteriaCmd.Flags().Lookup("epic"))
	assert.Nil(t, criteriaCmd.Flags().Lookup("estimate"))
	assert.Empty(t, criteriaCmd.Commands())
}
