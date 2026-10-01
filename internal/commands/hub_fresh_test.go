package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/hubstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHubTopicListFreshHomeHumanOutputIsEmpty(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	home := os.Getenv("DEVSPECS_HOME")
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "list", "--repo", repo})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()

	// Assert
	require.NoError(t, err)
	assert.Empty(t, out.String())
	_, err = os.Stat(filepath.Join(home, "hub.sqlite"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo, ".git", ".devspecs-hub-incarnation"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestHubTopicListExistingHomeUnboundRepoReturnsEmptyJSON(t *testing.T) {
	// Arrange
	firstRepo := hubRepoFixture(t)
	secondRepo := filepath.Join(t.TempDir(), "second")
	require.NoError(t, os.Mkdir(secondRepo, 0o700))
	require.NoError(t, exec.Command("git", "init", "-q", secondRepo).Run())
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollRepo(context.Background(), firstRepo)
	require.NoError(t, err)
	authorityID := db.AuthorityID()
	require.NoError(t, db.Close())
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "list", "--repo", secondRepo, "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()
	var response hubJSONResponse[[]hubstore.Topic]
	decodeErr := json.Unmarshal(out.Bytes(), &response)

	// Assert
	require.NoError(t, err)
	require.NoError(t, decodeErr)
	assert.Equal(t, "devspecs.hub/v1", response.ContractVersion)
	assert.NotNil(t, response.Result)
	assert.Empty(t, response.Result)
	_, err = os.Stat(filepath.Join(secondRepo, ".git", ".devspecs-hub-incarnation"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	readDB, err := hubstore.OpenReadOnly(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	defer readDB.Close()
	assert.Equal(t, authorityID, readDB.AuthorityID())
	_, err = readDB.LookupRepo(context.Background(), secondRepo)
	assert.ErrorIs(t, err, hubstore.ErrNotFound)
}

func TestHubTopicListFreshHomeInvalidRepoFails(t *testing.T) {
	// Arrange
	_ = hubRepoFixture(t)
	home := os.Getenv("DEVSPECS_HOME")
	invalidRepo := filepath.Join(t.TempDir(), "missing")
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "list", "--repo", invalidRepo, "--json"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()

	// Assert
	assert.Error(t, err)
	assert.Empty(t, out.String())
	_, statErr := os.Stat(filepath.Join(home, "hub.sqlite"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHubTopicListFreshHomeOverlongQueryFails(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	home := os.Getenv("DEVSPECS_HOME")
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "list", "--repo", repo, "--query", strings.Repeat("x", 257), "--json"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()

	// Assert
	assert.ErrorIs(t, err, hubstore.ErrInvalidInput)
	assert.Empty(t, out.String())
	_, statErr := os.Stat(filepath.Join(home, "hub.sqlite"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
	_, statErr = os.Stat(filepath.Join(repo, ".git", ".devspecs-hub-incarnation"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHubTopicListBindingConflictFailsWithoutMutation(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	db, err := hubstore.Open(context.Background(), hubstore.Options{})
	require.NoError(t, err)
	_, err = db.EnrollRepo(context.Background(), repo)
	require.NoError(t, err)
	authorityID := db.AuthorityID()
	require.NoError(t, db.Close())
	marker := filepath.Join(repo, ".git", ".devspecs-hub-incarnation")
	require.NoError(t, os.Remove(marker))
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "list", "--repo", repo, "--json"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err = cmd.Execute()

	// Assert
	assert.ErrorIs(t, err, hubstore.ErrBindingConflict)
	assert.Empty(t, out.String())
	_, statErr := os.Stat(marker)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
	readDB, openErr := hubstore.OpenReadOnly(context.Background(), hubstore.Options{})
	require.NoError(t, openErr)
	defer readDB.Close()
	assert.Equal(t, authorityID, readDB.AuthorityID())
	_, lookupErr := readDB.LookupRepo(context.Background(), repo)
	assert.ErrorIs(t, lookupErr, hubstore.ErrBindingConflict)
}

func TestHubTopicListCorruptAuthorityFailsWithoutReplacement(t *testing.T) {
	// Arrange
	repo := hubRepoFixture(t)
	path := filepath.Join(os.Getenv("DEVSPECS_HOME"), "hub.sqlite")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("not a sqlite database"), 0o600))
	cmd := NewHubCmd()
	cmd.SetArgs([]string{"topic", "list", "--repo", repo, "--json"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var out bytes.Buffer
	cmd.SetOut(&out)

	// Act
	err := cmd.Execute()

	// Assert
	assert.Error(t, err)
	assert.Empty(t, out.String())
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "not a sqlite database", string(data))
	_, statErr := os.Stat(filepath.Join(repo, ".git", ".devspecs-hub-incarnation"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
