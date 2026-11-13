package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func localRemoteSchema(t *testing.T, filename string) *jsonschema.Schema {
	t.Helper()
	dir, err := filepath.Abs("../../specs")
	require.NoError(t, err)
	configData, err := os.ReadFile(filepath.Join(dir, "remote-config.v1.schema.json"))
	require.NoError(t, err)
	resultData, err := os.ReadFile(filepath.Join(dir, "remote-result.v1.schema.json"))
	require.NoError(t, err)
	var configDoc, resultDoc any
	require.NoError(t, json.Unmarshal(configData, &configDoc))
	require.NoError(t, json.Unmarshal(resultData, &resultDoc))
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(jsonschema.SchemeURLLoader{})
	compiler.AssertFormat()
	const base = "https://schemas.invalid/"
	require.NoError(t, compiler.AddResource(base+"remote-config.v1.schema.json", configDoc))
	require.NoError(t, compiler.AddResource(base+"remote-result.v1.schema.json", resultDoc))
	schema, err := compiler.Compile(base + filename)
	require.NoError(t, err)
	return schema
}

func TestRemoteResultSchema_WithActualEmptyList_ValidatesLocalContract(t *testing.T) {
	// Arrange
	schema := localRemoteSchema(t, "remote-result.v1.schema.json")
	remoteTestDirectory(t, "")
	cmd := NewRemoteCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"list", "--json"})
	// Act
	err := cmd.Execute()
	// Assert
	require.NoError(t, err)
	var instance any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &instance))
	require.NoError(t, schema.Validate(instance))
}

func TestRemoteResultSchema_WithActualAuthenticatedOutput_ValidatesLocalContract(t *testing.T) {
	// Arrange
	schema := localRemoteSchema(t, "remote-result.v1.schema.json")
	_, _, deps := remoteAuthFixture(t)
	stdout, _, execute := authCommand(t, false, deps)
	// Act
	err := execute(context.Background())
	// Assert
	require.NoError(t, err)
	var instance any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &instance))
	require.NoError(t, schema.Validate(instance))
}

func TestRemoteResultSchema_WithActualCommandError_ValidatesStderrContract(t *testing.T) {
	// Arrange
	schema := localRemoteSchema(t, "remote-result.v1.schema.json")
	remoteTestDirectory(t, "")
	cmd := NewLoginCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"missing", "--json"})
	err := cmd.Execute()
	require.Error(t, err)
	var stderr bytes.Buffer
	// Act
	PrintCommandError(&stderr, err, []string{"--json", "login", "missing"})
	// Assert
	var instance any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &instance))
	require.NoError(t, schema.Validate(instance))
}

func TestRemoteConfigSchema_WithActualAtomicWrite_ValidatesMetadataContract(t *testing.T) {
	// Arrange
	schema := localRemoteSchema(t, "remote-config.v1.schema.json")
	root := remoteTestDirectory(t, "")
	cmd := NewRemoteCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"add", "hosted", "https://hub.example/base", "--tenant-id", testRemoteTenant, "--repo-id", testRemoteRepo, "--json"})
	// Act
	err := cmd.Execute()
	// Assert
	require.NoError(t, err)
	data, err := os.ReadFile(config.RepoConfigPath(root))
	require.NoError(t, err)
	var yamlValue any
	require.NoError(t, yaml.Unmarshal(data, &yamlValue))
	encoded, err := json.Marshal(yamlValue)
	require.NoError(t, err)
	var instance any
	require.NoError(t, json.Unmarshal(encoded, &instance))
	require.NoError(t, schema.Validate(instance))
}
