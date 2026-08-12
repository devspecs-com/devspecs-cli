package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func setupWorkspaceCommandFixture(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DEVSPECS_HOME", filepath.Join(tmp, "home"))
	root := filepath.Join(tmp, "eag-stg")
	mustMkdirAll(t, filepath.Join(root, ".git"))
	{
		dir := "enalytics-backend"

		mustMkdirAll(t, filepath.Join(root, dir, ".git"))
		mustMkdirAll(t, filepath.Join(root, dir, "docs", "plans"))
		mustWriteFile(t, filepath.Join(root, dir, "README.md"), "# "+dir+"\n")
		mustWriteFile(t, filepath.Join(root, dir, "docs", "plans", "customer-export.md"), "# Customer export\n")

	}
	{
		dir := "enalytics-frontend"

		mustMkdirAll(t, filepath.Join(root, dir, ".git"))
		mustMkdirAll(t, filepath.Join(root, dir, "docs", "plans"))
		mustWriteFile(t, filepath.Join(root, dir, "README.md"), "# "+dir+"\n")
		mustWriteFile(t, filepath.Join(root, dir, "docs", "plans", "customer-export.md"), "# Customer export\n")

	}
	{
		dir := "database"

		mustMkdirAll(t, filepath.Join(root, dir, ".git"))
		mustMkdirAll(t, filepath.Join(root, dir, "docs", "plans"))
		mustWriteFile(t, filepath.Join(root, dir, "README.md"), "# "+dir+"\n")
		mustWriteFile(t, filepath.Join(root, dir, "docs", "plans", "customer-export.md"), "# Customer export\n")

	}
	{
		dir := "prefect"

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
	origWd := testWorkingDirectory(t)
	{
		err := os.Chdir(root)
		require.NoError(t, err)
	}

	t.Cleanup(func() { os.Chdir(origWd) })
	return root
}

func TestWorkspaceInitCreatesManifestDocumentAndChangesDir(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)

	out := runWorkspaceInitJSON(t, root)
	assert.Equal(t, root, out.WorkspaceRoot,
		"workspace root = %q, want %q", out.WorkspaceRoot, root)
	assert.Equal(t, workspaceIndexStatus, out.IndexStatus, "workspace init should report explicit index status: %#v", out)
	assert.NotEqual(t, "", out.IndexReason, "workspace init should report explicit index status: %#v", out)

	{
		path := out.ManifestPath

		{
			_, err := os.Stat(path)
			require.NoError(t, err,
				"expected workspace path %s: %v", path, err)
		}

	}
	{
		path := out.DocumentPath

		{
			_, err := os.Stat(path)
			require.NoError(t, err,
				"expected workspace path %s: %v", path, err)
		}

	}
	{
		path := out.ChangesDir

		{
			_, err := os.Stat(path)
			require.NoError(t, err,
				"expected workspace path %s: %v", path, err)
		}

	}

	var manifest workspaceManifest
	{
		err := yaml.Unmarshal([]byte(mustReadFile(t, out.ManifestPath)), &manifest)
		require.NoError(t, err,
			"workspace yaml: %v", err)
	}
	assert.Equal(t, "eag-stg", manifest.ID, "manifest basics = %#v", manifest)
	assert.Equal(t, "EAG-STG", manifest.Name, "manifest basics = %#v", manifest)
	assert.Equal(t, defaultWorkspaceArtifactDir, manifest.ArtifactDir, "manifest basics = %#v", manifest)

	{
		alias := "backend"
		wantPath := "./enalytics-backend"

		{
			got := manifest.Repos[alias].Path
			assert.Equal(t, wantPath, got,
				"repo alias %s path = %q, want %q in %#v", alias, got, wantPath, manifest.Repos)
		}

	}
	{
		alias := "frontend"
		wantPath := "./enalytics-frontend"

		{
			got := manifest.Repos[alias].Path
			assert.Equal(t, wantPath, got,
				"repo alias %s path = %q, want %q in %#v", alias, got, wantPath, manifest.Repos)
		}

	}
	{
		alias := "database"
		wantPath := "./database"

		{
			got := manifest.Repos[alias].Path
			assert.Equal(t, wantPath, got,
				"repo alias %s path = %q, want %q in %#v", alias, got, wantPath, manifest.Repos)
		}

	}
	{
		alias := "prefect"
		wantPath := "./prefect"

		{
			got := manifest.Repos[alias].Path
			assert.Equal(t, wantPath, got,
				"repo alias %s path = %q, want %q in %#v", alias, got, wantPath, manifest.Repos)
		}

	}

	document := mustReadFile(t, out.DocumentPath)
	assert.Contains(t, document, "# EAG-STG Workspace",
		"workspace document missing %q:\n%s", "# EAG-STG Workspace", document)
	assert.Contains(t, document, "| `backend` | `./enalytics-backend` |",
		"workspace document missing %q:\n%s", "| `backend` | `./enalytics-backend` |", document)
	assert.Contains(t, document, "`devspecs/changes/`",
		"workspace document missing %q:\n%s", "`devspecs/changes/`", document)

	before := mustReadFile(t, out.ManifestPath)
	cmd := NewWorkspaceCmd()
	cmd.SetArgs([]string{"init", root, "--json"})
	cmd.SetOut(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err,
		"expected rerun init to fail without overwriting")
	{

		got := mustReadFile(t, out.ManifestPath)
		assert.Equal(t, before, got,
			"workspace init rerun mutated manifest.\nBefore:\n%s\nAfter:\n%s", before, got)
	}

}

func TestWorkspaceShowResolvesFromChildDirectory(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	nested := filepath.Join(root, "enalytics-backend", "internal")
	mustMkdirAll(t, nested)
	{
		err := os.Chdir(nested)
		require.NoError(t, err)
	}

	cmd := NewWorkspaceCmd()
	cmd.SetArgs([]string{"show", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out workspaceOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err,
			"workspace show json: %v\n%s", err, buf.String())
	}
	assert.Equal(t, root, out.WorkspaceRoot, "workspace show resolved wrong workspace: %#v", out)
	assert.Equal(t, "eag-stg", out.Manifest.ID, "workspace show resolved wrong workspace: %#v", out)

}

func TestWorkspaceHelpOwnsCoordinationSubcommands(t *testing.T) {
	cmd := NewWorkspaceCmd()
	cmd.SetArgs([]string{"--help"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	help := buf.String()
	assert.Contains(t, help, "change      Manage workspace-level change artifacts",
		"workspace help missing %q:\n%s", "change      Manage workspace-level change artifacts", help)
	assert.Contains(t, help, "slice       Create repo-local task slices from workspace changes",
		"workspace help missing %q:\n%s", "slice       Create repo-local task slices from workspace changes", help)
	assert.Contains(t, help, "trace       Trace workspace changes to repo-local task slices",
		"workspace help missing %q:\n%s", "trace       Trace workspace changes to repo-local task slices", help)

}

func runWorkspaceInitJSON(t *testing.T, root string) workspaceOutput {
	t.Helper()
	cmd := NewWorkspaceCmd()
	cmd.SetArgs([]string{"init", root, "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out workspaceOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoError(t, err,
			"workspace init json: %v\n%s", err, buf.String())
	}

	return out
}
