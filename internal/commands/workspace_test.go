package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func setupWorkspaceCommandFixture(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	root := filepath.Join(tmp, "eag-stg")
	mustMkdirAll(t, filepath.Join(root, ".git"))
	for _, dir := range []string{"enalytics-backend", "enalytics-frontend", "database", "prefect"} {
		mustMkdirAll(t, filepath.Join(root, dir, ".git"))
		mustMkdirAll(t, filepath.Join(root, dir, "docs", "plans"))
		mustWriteFile(t, filepath.Join(root, dir, "README.md"), "# "+dir+"\n")
		mustWriteFile(t, filepath.Join(root, dir, "docs", "plans", "customer-export.md"), "# Customer export\n")
	}
	mustMkdirAll(t, filepath.Join(root, "enalytics-backend", "internal"))
	mustWriteFile(t, filepath.Join(root, "enalytics-backend", "go.mod"), "module example.com/enalytics-backend\n\ngo 1.22\n")
	mustWriteFile(t, filepath.Join(root, "enalytics-backend", "internal", "service.go"), "package internal\n\nfunc CustomerExport() string { return \"ok\" }\n")
	mustWriteFile(t, filepath.Join(root, "enalytics-frontend", "package.json"), "{\n  \"name\": \"enalytics-frontend\",\n  \"private\": true\n}\n")
	mustWriteFile(t, filepath.Join(root, "AGENTS.md"), "# eag-stg\n")
	mustWriteFile(t, filepath.Join(root, "CLAUDE.md"), "# eag-stg\n")
	origWd, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origWd) })
	return root
}

func TestWorkspaceInitCreatesManifestDocumentAndChangesDir(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)

	out := runWorkspaceInitJSON(t, root)
	if out.WorkspaceRoot != root {
		t.Fatalf("workspace root = %q, want %q", out.WorkspaceRoot, root)
	}
	if out.IndexStatus != workspaceIndexStatus || out.IndexReason == "" {
		t.Fatalf("workspace init should report explicit index status: %#v", out)
	}
	for _, path := range []string{out.ManifestPath, out.DocumentPath, out.ChangesDir} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected workspace path %s: %v", path, err)
		}
	}

	var manifest workspaceManifest
	if err := yaml.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest); err != nil {
		t.Fatalf("workspace yaml: %v", err)
	}
	if manifest.ID != "eag-stg" || manifest.Name != "EAG-STG" || manifest.ArtifactDir != defaultWorkspaceArtifactDir {
		t.Fatalf("manifest basics = %#v", manifest)
	}
	for alias, wantPath := range map[string]string{
		"backend":  "./enalytics-backend",
		"frontend": "./enalytics-frontend",
		"database": "./database",
		"prefect":  "./prefect",
	} {
		if got := manifest.Repos[alias].Path; got != wantPath {
			t.Fatalf("repo alias %s path = %q, want %q in %#v", alias, got, wantPath, manifest.Repos)
		}
	}
	document := mustReadFile(t, out.DocumentPath)
	for _, want := range []string{"# EAG-STG Workspace", "| `backend` | `./enalytics-backend` |", "`devspecs/changes/`"} {
		if !strings.Contains(document, want) {
			t.Fatalf("workspace document missing %q:\n%s", want, document)
		}
	}

	before := mustReadFile(t, out.ManifestPath)
	cmd := NewWorkspaceCmd()
	cmd.SetArgs([]string{"init", root, "--json"})
	cmd.SetOut(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected rerun init to fail without overwriting")
	}
	if got := mustReadFile(t, out.ManifestPath); got != before {
		t.Fatalf("workspace init rerun mutated manifest.\nBefore:\n%s\nAfter:\n%s", before, got)
	}
}

func TestWorkspaceShowResolvesFromChildDirectory(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	nested := filepath.Join(root, "enalytics-backend", "internal")
	mustMkdirAll(t, nested)
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}

	cmd := NewWorkspaceCmd()
	cmd.SetArgs([]string{"show", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out workspaceOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("workspace show json: %v\n%s", err, buf.String())
	}
	if out.WorkspaceRoot != root || out.Manifest.ID != "eag-stg" {
		t.Fatalf("workspace show resolved wrong workspace: %#v", out)
	}
}

func TestWorkspaceHelpOwnsCoordinationSubcommands(t *testing.T) {
	cmd := NewWorkspaceCmd()
	cmd.SetArgs([]string{"--help"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := buf.String()
	for _, want := range []string{
		"change      Manage workspace-level change artifacts",
		"slice       Create repo-local task slices from workspace changes",
		"trace       Trace workspace changes to repo-local task slices",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("workspace help missing %q:\n%s", want, help)
		}
	}
}

func runWorkspaceInitJSON(t *testing.T, root string) workspaceOutput {
	t.Helper()
	cmd := NewWorkspaceCmd()
	cmd.SetArgs([]string{"init", root, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out workspaceOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("workspace init json: %v\n%s", err, buf.String())
	}
	return out
}
