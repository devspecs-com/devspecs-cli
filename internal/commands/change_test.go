package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangeCreateWritesWorkspaceChangeOnly(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)

	cmd := NewChangeCmd()
	cmd.SetArgs([]string{"create", "Customer export", "--workspace", root, "--repos", "backend,frontend", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out changeCreateOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("change create json: %v\n%s", err, buf.String())
	}
	if out.ChangeID != "EAG-C001" {
		t.Fatalf("change id = %q, want EAG-C001", out.ChangeID)
	}
	if out.ChangePath != filepath.Join(root, "devspecs", "changes", "EAG-C001-customer-export.md") {
		t.Fatalf("change path = %q", out.ChangePath)
	}
	if out.IndexStatus != workspaceIndexStatus || out.IndexReason == "" {
		t.Fatalf("change create should report explicit index status: %#v", out)
	}
	body := mustReadFile(t, out.ChangePath)
	for _, want := range []string{
		"id: EAG-C001",
		"type: workspace_change",
		"workspace: eag-stg",
		"title: Customer export",
		"required_repos:",
		"- backend",
		"- frontend",
		"optional_repos: []",
		"## Required Repositories",
		"`backend` - `./enalytics-backend`",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("change body missing %q:\n%s", want, body)
		}
	}
	for _, child := range []string{"enalytics-backend", "enalytics-frontend"} {
		if _, err := os.Stat(filepath.Join(root, child, "devspecs", "tasks")); !os.IsNotExist(err) {
			t.Fatalf("change create unexpectedly wrote repo-local task artifacts in %s: %v", child, err)
		}
	}

	second := NewChangeCmd()
	second.SetArgs([]string{"create", "Billing export", "--workspace", root, "--repos", "backend", "--json"})
	secondBuf := &bytes.Buffer{}
	second.SetOut(secondBuf)
	if err := second.Execute(); err != nil {
		t.Fatal(err)
	}
	var secondOut changeCreateOutput
	if err := json.Unmarshal(secondBuf.Bytes(), &secondOut); err != nil {
		t.Fatalf("second change json: %v\n%s", err, secondBuf.String())
	}
	if secondOut.ChangeID != "EAG-C002" {
		t.Fatalf("second change id = %q, want EAG-C002", secondOut.ChangeID)
	}
}

func TestChangeCreateValidatesWorkspaceRepoAliases(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)

	cmd := NewChangeCmd()
	cmd.SetArgs([]string{"create", "Invalid route", "--workspace", root, "--repos", "backend,missing", "--json"})
	cmd.SetOut(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing repo alias error")
	}
	if !strings.Contains(err.Error(), `workspace repo alias "missing" not found`) {
		t.Fatalf("missing alias error = %v", err)
	}

	duplicate := NewChangeCmd()
	duplicate.SetArgs([]string{"create", "Duplicate route", "--workspace", root, "--repos", "backend,backend", "--json"})
	duplicate.SetOut(&bytes.Buffer{})
	err = duplicate.Execute()
	if err == nil {
		t.Fatal("expected duplicate repo alias error")
	}
	if !strings.Contains(err.Error(), `duplicate workspace repo alias "backend"`) {
		t.Fatalf("duplicate alias error = %v", err)
	}
}
