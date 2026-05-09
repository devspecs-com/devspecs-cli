package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupReadEnv(t *testing.T) (repoDir string, artifactID string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir = filepath.Join(tmp, "repo")
	os.MkdirAll(repoDir, 0o755)
	os.WriteFile(filepath.Join(repoDir, "plan.md"), []byte("# My Plan\n\n- [ ] Open task\n- [x] Done task\n- [ ] Another open\n"), 0o644)

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
	if err := captureCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, line := range strings.Split(capBuf.String(), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ds_") {
			artifactID = trimmed
			break
		}
	}
	if artifactID == "" {
		t.Fatal("failed to extract artifact ID from capture output")
	}
	return
}

func TestList_ShowsArtifacts(t *testing.T) {
	setupReadEnv(t)

	cmd := NewListCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "My Plan") {
		t.Errorf("list output missing 'My Plan': %s", output)
	}
	if !strings.Contains(output, "plan") {
		t.Errorf("list output missing 'plan' kind: %s", output)
	}
}

func TestShow_DisplaysDetail(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewShowCmd()
	cmd.SetArgs([]string{artID})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "My Plan") {
		t.Errorf("show missing title: %s", output)
	}
	if !strings.Contains(output, artID) {
		t.Errorf("show missing ID: %s", output)
	}
}

func TestShow_IncludesTodos(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewShowCmd()
	cmd.SetArgs([]string{artID})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "Todos:") {
		t.Errorf("show output missing Todos section: %s", output)
	}
	if !strings.Contains(output, "Open task") {
		t.Errorf("show output missing todo text: %s", output)
	}
	if !strings.Contains(output, "[ ]") {
		t.Errorf("show output missing open marker: %s", output)
	}
	if !strings.Contains(output, "[x]") {
		t.Errorf("show output missing done marker: %s", output)
	}
}

func TestFind_ByTitle(t *testing.T) {
	setupReadEnv(t)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"My Plan"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "My Plan") {
		t.Errorf("find output missing 'My Plan': %s", output)
	}
}

func TestResolve_OutputsIDAndPath(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewResolveCmd()
	cmd.SetArgs([]string{artID})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, artID) {
		t.Errorf("resolve missing artifact ID: %s", output)
	}
	if !strings.Contains(output, "plan.md") {
		t.Errorf("resolve missing source path: %s", output)
	}
}

func TestContext_IncludesExtractedTasks(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewContextCmd()
	cmd.SetArgs([]string{artID})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "## Extracted Tasks") {
		t.Errorf("context missing Extracted Tasks header: %s", output)
	}
	if !strings.Contains(output, "Open task") {
		t.Errorf("context missing todo text 'Open task': %s", output)
	}
	if !strings.Contains(output, "- [ ]") {
		t.Errorf("context missing open marker: %s", output)
	}
	if !strings.Contains(output, "- [x]") {
		t.Errorf("context missing done marker: %s", output)
	}
}

func TestTodos_AllArtifacts(t *testing.T) {
	setupReadEnv(t)

	cmd := NewTodosCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "Open task") {
		t.Errorf("todos output missing 'Open task': %s", output)
	}
	if !strings.Contains(output, "Done task") {
		t.Errorf("todos output missing 'Done task': %s", output)
	}
	if !strings.Contains(output, "Another open") {
		t.Errorf("todos output missing 'Another open': %s", output)
	}
}

func TestTodos_ScopedToArtifact(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewTodosCmd()
	cmd.SetArgs([]string{artID})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "Open task") {
		t.Errorf("scoped todos missing 'Open task': %s", output)
	}
}

func TestTodos_FiltersOpenDone(t *testing.T) {
	setupReadEnv(t)

	// --open
	openCmd := NewTodosCmd()
	openCmd.SetArgs([]string{"--open"})
	openBuf := &bytes.Buffer{}
	openCmd.SetOut(openBuf)
	if err := openCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	openOut := openBuf.String()
	if !strings.Contains(openOut, "Open task") {
		t.Errorf("--open missing 'Open task': %s", openOut)
	}
	if strings.Contains(openOut, "Done task") {
		t.Errorf("--open should NOT include 'Done task': %s", openOut)
	}

	// --done
	doneCmd := NewTodosCmd()
	doneCmd.SetArgs([]string{"--done"})
	doneBuf := &bytes.Buffer{}
	doneCmd.SetOut(doneBuf)
	if err := doneCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	doneOut := doneBuf.String()
	if !strings.Contains(doneOut, "Done task") {
		t.Errorf("--done missing 'Done task': %s", doneOut)
	}
	if strings.Contains(doneOut, "Open task") {
		t.Errorf("--done should NOT include 'Open task': %s", doneOut)
	}
}

func TestTodos_JSONSchema(t *testing.T) {
	setupReadEnv(t)

	cmd := NewTodosCmd()
	cmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var todos []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &todos); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if len(todos) == 0 {
		t.Fatal("expected non-empty todos array")
	}

	requiredFields := []string{"artifact_id", "revision_id", "ordinal", "text", "done", "source_file", "source_line"}
	for _, field := range requiredFields {
		if _, ok := todos[0][field]; !ok {
			t.Errorf("JSON todo missing required field %q", field)
		}
	}
}

func TestTodos_NoOutOfScopeFlags(t *testing.T) {
	cmd := NewTodosCmd()
	forbidden := []string{"owner", "assignee", "due-date", "due_date", "priority", "label", "sprint", "create", "update", "delete"}
	for _, flag := range forbidden {
		if cmd.Flags().Lookup(flag) != nil {
			t.Errorf("todos command has forbidden flag --%s (outside PRD scope)", flag)
		}
	}
}

func TestContext_JSONOutput(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewContextCmd()
	cmd.SetArgs([]string{artID, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var obj map[string]any
	if err := json.Unmarshal(buf.Bytes(), &obj); err != nil {
		t.Fatalf("context --json invalid: %v", err)
	}
	if _, ok := obj["todos"]; !ok {
		t.Error("context JSON missing 'todos' key")
	}
	if _, ok := obj["body"]; !ok {
		t.Error("context JSON missing 'body' key")
	}
}

func TestShow_JSONOutput(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewShowCmd()
	cmd.SetArgs([]string{artID, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var obj map[string]any
	if err := json.Unmarshal(buf.Bytes(), &obj); err != nil {
		t.Fatalf("show --json invalid: %v", err)
	}
	for _, key := range []string{"id", "kind", "title", "status", "todos"} {
		if _, ok := obj[key]; !ok {
			t.Errorf("show JSON missing key %q", key)
		}
	}
}

func TestResolve_JSONOutput(t *testing.T) {
	_, artID := setupReadEnv(t)

	cmd := NewResolveCmd()
	cmd.SetArgs([]string{artID, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var obj map[string]any
	if err := json.Unmarshal(buf.Bytes(), &obj); err != nil {
		t.Fatalf("resolve --json invalid: %v", err)
	}
	for _, key := range []string{"id", "kind", "title", "source_path"} {
		if _, ok := obj[key]; !ok {
			t.Errorf("resolve JSON missing key %q", key)
		}
	}
}

func TestFind_JSONOutput(t *testing.T) {
	setupReadEnv(t)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"Plan", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var arts []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &arts); err != nil {
		t.Fatalf("find --json invalid: %v", err)
	}
	if len(arts) == 0 {
		t.Fatal("find --json returned empty array")
	}
}
