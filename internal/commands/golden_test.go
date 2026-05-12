package commands

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

func goldenPath(name string) string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "golden", name+".json")
}

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()

	// Normalize JSON: re-encode with stable formatting, then mask dynamic values
	var raw any
	if err := json.Unmarshal(got, &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, got)
	}
	maskDynamic(raw)
	normalized, _ := json.MarshalIndent(raw, "", "  ")
	normalized = append(normalized, '\n')

	path := goldenPath(name)
	if *update {
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, normalized, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden file missing (run with -update to create): %v", err)
	}
	// Git on Windows may materialize tracked JSON with CRLF; json.MarshalIndent uses LF only.
	expected = bytes.ReplaceAll(expected, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(normalized, expected) {
		t.Errorf("output differs from golden file %s\n--- got ---\n%s\n--- want ---\n%s", path, normalized, expected)
	}
}

func maskDynamic(v any) {
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			if isDynamicKey(k) {
				if s, ok := child.(string); ok && s != "" {
					val[k] = "<MASKED>"
				}
			} else {
				maskDynamic(child)
			}
		}
	case []any:
		for _, item := range val {
			maskDynamic(item)
		}
	}
}

func isDynamicKey(k string) bool {
	switch k {
	case "id", "ID", "repo_id", "RepoID", "current_revision_id", "CurrentRevID",
		"current_revision_hash", "artifact_id", "revision_id",
		"created_at", "CreatedAt", "updated_at", "UpdatedAt",
		"last_observed_at", "LastObservedAt", "observed_at", "ObservedAt":
		return true
	}
	return false
}

func setupGoldenEnv(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir := filepath.Join(tmp, "repo")
	os.MkdirAll(repoDir, 0o755)
	os.WriteFile(filepath.Join(repoDir, "plan.md"), []byte("---\ntitle: Golden Plan\nkind: plan\nstatus: draft\n---\n# Golden Plan\n\n- [ ] First task\n- [x] Second task\n"), 0o644)

	origWd, _ := os.Getwd()
	os.Chdir(repoDir)
	t.Cleanup(func() { os.Chdir(origWd) })

	initCmd := NewInitCmd()
	initCmd.SetOut(&bytes.Buffer{})
	initCmd.Execute()

	captureCmd := NewCaptureCmd()
	captureCmd.SetArgs([]string{"plan.md", "--kind", "plan"})
	capBuf := &bytes.Buffer{}
	captureCmd.SetOut(capBuf)
	captureCmd.Execute()

	var artID string
	for _, line := range strings.Split(capBuf.String(), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ds_") {
			artID = trimmed
			break
		}
	}
	return artID
}

func TestGolden_TodosJSON(t *testing.T) {
	setupGoldenEnv(t)
	cmd := NewTodosCmd()
	cmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "todos", buf.Bytes())
}

func TestGolden_ListJSON(t *testing.T) {
	setupGoldenEnv(t)
	cmd := NewListCmd()
	cmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "list", buf.Bytes())
}

func TestGolden_ShowJSON(t *testing.T) {
	artID := setupGoldenEnv(t)
	cmd := NewShowCmd()
	cmd.SetArgs([]string{artID, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "show", buf.Bytes())
}

func TestGolden_FindJSON(t *testing.T) {
	setupGoldenEnv(t)
	cmd := NewFindCmd()
	cmd.SetArgs([]string{"Golden", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "find", buf.Bytes())
}

func TestGolden_ResolveJSON(t *testing.T) {
	artID := setupGoldenEnv(t)
	cmd := NewResolveCmd()
	cmd.SetArgs([]string{artID, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "resolve", buf.Bytes())
}

func TestGolden_ContextJSON(t *testing.T) {
	artID := setupGoldenEnv(t)
	cmd := NewContextCmd()
	cmd.SetArgs([]string{artID, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "context", buf.Bytes())
}
