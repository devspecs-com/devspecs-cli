package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChangeCreateWritesWorkspaceChangeOnly(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)

	cmd := NewChangeCmd()
	cmd.SetArgs([]string{"create", "Customer export", "--workspace", root, "--repos", "backend,frontend", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out changeCreateOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "EAG-C001", out.ChangeID)
	assert.Equal(t, filepath.Join(root, "devspecs", "changes", "EAG-C001-customer-export.md"), out.ChangePath)
	assert.Equal(t, workspaceIndexStatus, out.IndexStatus)
	assert.NotEmpty(t, out.IndexReason)

	body := mustReadFile(t, out.ChangePath)
	assert.Contains(t, body, "id: EAG-C001")
	assert.Contains(t, body, "type: workspace_change")
	assert.Contains(t, body, "workspace: eag-stg")
	assert.Contains(t, body, "title: Customer export")
	assert.Contains(t, body, "required_repos:")
	assert.Contains(t, body, "- backend")
	assert.Contains(t, body, "- frontend")
	assert.Contains(t, body, "optional_repos: []")
	assert.Contains(t, body, "## Required Repositories")
	assert.Contains(t, body, "`backend` - `./enalytics-backend`")
	assert.NoDirExists(t, filepath.Join(root, "enalytics-backend", "devspecs", "tasks"))
	assert.NoDirExists(t, filepath.Join(root, "enalytics-frontend", "devspecs", "tasks"))
}

func TestChangeCreateAutoIncrementsExistingChangeID(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	runChangeCreateJSON(t, "Customer export", root, "backend,frontend")
	cmd := NewChangeCmd()
	cmd.SetArgs([]string{"create", "Billing export", "--workspace", root, "--repos", "backend", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err := cmd.Execute()

	require.NoError(t, err)
	var out changeCreateOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "EAG-C002", out.ChangeID)
}

func TestChangeCreateRejectsMissingWorkspaceRepoAlias(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	cmd := NewChangeCmd()
	cmd.SetArgs([]string{"create", "Invalid route", "--workspace", root, "--repos", "backend,missing", "--json"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, `workspace repo alias "missing" not found`)
}

func TestChangeCreateRejectsDuplicateWorkspaceRepoAlias(t *testing.T) {
	root := setupWorkspaceCommandFixture(t)
	runWorkspaceInitJSON(t, root)
	cmd := NewChangeCmd()
	cmd.SetArgs([]string{"create", "Duplicate route", "--workspace", root, "--repos", "backend,backend", "--json"})
	cmd.SetOut(&bytes.Buffer{})

	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorContains(t, err, `duplicate workspace repo alias "backend"`)
}
