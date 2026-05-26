package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func setupReadEnv(t *testing.T) (repoDir string, artifactID string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))

	repoDir = filepath.Join(tmp, "repo")
	os.MkdirAll(repoDir, 0o755)
	os.WriteFile(filepath.Join(repoDir, "plan.md"), []byte("# My Plan\n\n## Tasks\n\n- [ ] Open task\n- [x] Done task\n- [ ] Another open\n\n## Auditable success criteria\n\n- [ ] Gate criterion open\n- [x] Gate criterion done\n"), 0o644)

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

func TestCriteria_AllArtifacts(t *testing.T) {
	setupReadEnv(t)

	cmd := NewCriteriaCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Gate criterion open") {
		t.Errorf("criteria output missing open criterion: %s", out)
	}
	if !strings.Contains(out, "Gate criterion done") {
		t.Errorf("criteria output missing done criterion: %s", out)
	}
	if !strings.Contains(out, "success") {
		t.Errorf("criteria output missing kind column success: %s", out)
	}
}

func TestCriteria_JSONSchema(t *testing.T) {
	setupReadEnv(t)

	cmd := NewCriteriaCmd()
	cmd.SetArgs([]string{"--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var rows []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 criteria, got %d", len(rows))
	}
	requiredFields := []string{"artifact_id", "revision_id", "ordinal", "text", "done", "source_file", "source_line", "criteria_kind"}
	for _, field := range requiredFields {
		if _, ok := rows[0][field]; !ok {
			t.Errorf("JSON criterion missing required field %q", field)
		}
	}
}

func TestCriteria_FiltersOpenDoneKind(t *testing.T) {
	setupReadEnv(t)

	openCmd := NewCriteriaCmd()
	openCmd.SetArgs([]string{"--open"})
	openBuf := &bytes.Buffer{}
	openCmd.SetOut(openBuf)
	if err := openCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	openOut := openBuf.String()
	if !strings.Contains(openOut, "Gate criterion open") {
		t.Errorf("--open missing open criterion: %s", openOut)
	}
	if strings.Contains(openOut, "Gate criterion done") {
		t.Errorf("--open should not include done criterion: %s", openOut)
	}

	kindCmd := NewCriteriaCmd()
	kindCmd.SetArgs([]string{"--kind", "success"})
	kindBuf := &bytes.Buffer{}
	kindCmd.SetOut(kindBuf)
	if err := kindCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(kindBuf.String(), "Gate criterion") {
		t.Errorf("--kind success: %s", kindBuf.String())
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
	if _, ok := arts[0]["reasons"].([]any); !ok {
		t.Fatalf("find --json missing retrieval reasons: %#v", arts[0])
	}
	if arts[0]["source_path"] != "plan.md" {
		t.Fatalf("find --json source_path = %#v", arts[0]["source_path"])
	}
	if arts[0]["retriever"] != "eval_weighted_files_v0" {
		t.Fatalf("find --json retriever = %#v", arts[0]["retriever"])
	}
}

func TestFindPack_JSONOutputKeepsRankedResultsAndGroups(t *testing.T) {
	setupReadEnv(t)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"Plan", "--json", "--pack"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var out FindPackOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("find --json --pack invalid: %v\n%s", err, buf.String())
	}
	if out.Mode != "role_grouped_pack_v0" {
		t.Fatalf("pack mode = %q", out.Mode)
	}
	if len(out.Groups) == 0 {
		t.Fatalf("find --json --pack returned no groups: %#v", out)
	}
	if len(out.RankedResults) == 0 {
		t.Fatalf("find --json --pack returned no ranked results: %#v", out)
	}
}

func TestFindPack_HumanOutputShowsReceipt(t *testing.T) {
	setupReadEnv(t)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"Plan", "--pack"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{"Working set: Plan", "Mode: role_grouped_pack_v0", "Why:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("find --pack missing %q:\n%s", want, output)
		}
	}
}

func TestFind_JSONOutputIncludesLineScopedPath(t *testing.T) {
	repoDir, _ := setupReadEnv(t)
	relPath := seedLineScopedTestArtifacts(t, repoDir)

	cmd := NewFindCmd()
	cmd.SetArgs([]string{"--json", "--no-refresh", "testputandgetexposedtool"})
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
	wantPath := filepath.ToSlash(relPath) + "#L53"
	if arts[0]["path"] != wantPath {
		t.Fatalf("find --json path = %#v, want %q\nrows=%#v", arts[0]["path"], wantPath, arts)
	}
	if arts[0]["source_path"] != filepath.ToSlash(relPath) {
		t.Fatalf("find --json source_path = %#v", arts[0]["source_path"])
	}
}

func TestResume_QueryFocusedContextJSON(t *testing.T) {
	setupReadEnv(t)

	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"Open task", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var obj map[string]any
	if err := json.Unmarshal(buf.Bytes(), &obj); err != nil {
		t.Fatalf("resume query --json invalid: %v\n%s", err, buf.String())
	}
	if obj["retriever"] != "eval_weighted_files_v0" {
		t.Fatalf("retriever = %#v", obj["retriever"])
	}
	if obj["token_counter"] != "approx_chars_div_4" {
		t.Fatalf("token counter = %#v", obj["token_counter"])
	}
	arts, ok := obj["artifacts"].([]any)
	if !ok || len(arts) == 0 {
		t.Fatalf("resume query returned no artifacts: %#v", obj["artifacts"])
	}
	context, _ := obj["context"].(string)
	if !strings.Contains(context, "Open task") || !strings.Contains(context, "plan.md") {
		t.Fatalf("focused context missing expected content: %s", context)
	}
}

func seedLineScopedTestArtifacts(t *testing.T, repoDir string) string {
	t.Helper()
	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var repoID string
	if err := db.QueryRow("SELECT id FROM repos LIMIT 1").Scan(&repoID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	relPath := filepath.ToSlash(filepath.Join("components", "camel-ai", "camel-langchain4j-tools", "src", "test", "java", "org", "apache", "camel", "component", "langchain4j", "tools", "spec", "CamelToolExecutorCacheTest.java"))
	insertTestArtifact(t, db, repoID, "ds_exact_test", "rev_exact_test", "src_exact_test", relPath, 53, 67, "testPutAndGetExposedTool", "Test: testPutAndGetExposedTool\nSource: "+relPath+"\nLines: 53-67\n\ncache.put(\"users\", camelSpec);\ncache.getTools().get(\"users\");", now)
	insertTestArtifact(t, db, repoID, "ds_other_test", "rev_other_test", "src_other_test", relPath, 151, 159, "testHasSearchableTools", "Test: testHasSearchableTools\nSource: "+relPath+"\nLines: 151-159\n\ncache.putSearchable(\"users\", camelSpec);\ncache.hasSearchableTools();", now)
	return relPath
}

func insertTestArtifact(t *testing.T, db *store.DB, repoID, artifactID, revID, sourceID, relPath string, startLine, endLine int, testName, body, now string) {
	t.Helper()
	title := "CamelToolExecutorCacheTest > " + testName
	extracted := `{"mode":"intent","subtype":"test_case","source_type":"test_case","test_name":"` + testName + `","source_line_range":"` + strconv.Itoa(startLine) + `-` + strconv.Itoa(endLine) + `"}`
	if err := db.InsertArtifactDirect(artifactID, repoID, "source_context", "test_case", title, "unknown", revID, now, now); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertRevisionDirect(revID, artifactID, "sha256:"+artifactID, body, extracted, now); err != nil {
		t.Fatal(err)
	}
	sourceIdentity := relPath + "|test_case|" + strconv.Itoa(startLine) + "|" + strings.ToLower(testName)
	if err := db.InsertSourceDirect(sourceID, artifactID, repoID, "test_case", relPath, sourceIdentity, "", "", now); err != nil {
		t.Fatal(err)
	}
	if err := db.IndexArtifactFTS(artifactID, title, body, relPath); err != nil {
		t.Fatal(err)
	}
}
