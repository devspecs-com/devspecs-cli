package codecomment

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdapter_Name_ReturnsCodeComment(t *testing.T) {
	adapter := &Adapter{}

	name := adapter.Name()

	assert.Equal(t, sourceType, name)
}

func TestAcceptsFile_WithEnabledSupportedFile_ReturnsTrue(t *testing.T) {
	adapter := &Adapter{}
	cfg := config.WithCodeCommentArtifacts(config.DefaultRepoConfig(), true)

	accepted := adapter.AcceptsFile("billing/webhook.go", 1024, cfg)

	assert.True(t, accepted)
}

func TestAcceptsFile_WithOversizedFile_ReturnsFalse(t *testing.T) {
	adapter := &Adapter{}
	cfg := config.WithCodeCommentArtifacts(config.DefaultRepoConfig(), true)

	accepted := adapter.AcceptsFile("billing/webhook.go", maxFileBytes+1, cfg)

	assert.False(t, accepted)
}

func TestDiscoverFile_WithIntentComment_ReturnsSectionCandidate(t *testing.T) {
	adapter := &Adapter{}
	cfg := config.WithCodeCommentArtifacts(config.DefaultRepoConfig(), true)
	file := adapters.FileCandidate{
		PrimaryPath: "billing/webhook.go",
		RelPath:     "billing/webhook.go",
		Size:        90,
		Body:        []byte("package billing\n\n// Invariant: stripe_event_id must be unique.\nfunc apply() {}\n"),
	}

	candidates, err := adapter.DiscoverFile(context.Background(), file, cfg)

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, "billing/webhook.go", candidates[0].RelPath)
	assert.Equal(t, "invariant", candidates[0].Role)
}

func TestDiscoverFile_WithCanceledContext_ReturnsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	adapter := &Adapter{}
	cfg := config.WithCodeCommentArtifacts(config.DefaultRepoConfig(), true)

	candidates, err := adapter.DiscoverFile(ctx, adapters.FileCandidate{RelPath: "billing/webhook.go"}, cfg)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, candidates)
}

func TestExtractComments_WithIntentComments_ReturnsIntentUnitsOnly(t *testing.T) {
	body := `package billing

// Copyright 2026 Example Inc.
// Licensed under the Apache License.

// increment the counter
counter++

// Invariant: stripe_event_id must always be checked before applying credits.
func applyCredit() {}

// TODO: remove the legacy retry workaround after the migration finishes.
func retry() {}
`

	units := extractComments("billing/webhook.go", body)

	require.Len(t, units, 2)
	assert.Equal(t, "invariant", units[0].Role)
	assert.Equal(t, "todo", units[1].Role)
}

func TestDiscover_WhenCodeCommentsDisabled_ReturnsNoCandidates(t *testing.T) {
	root, _ := writeCodeCommentFixture(t)
	adapter := &Adapter{}

	candidates, err := adapter.Discover(context.Background(), root, config.DefaultRepoConfig())

	require.NoError(t, err)
	assert.Empty(t, candidates)
}

func TestDiscover_WhenCodeCommentsEnabled_ReturnsCandidate(t *testing.T) {
	root, _ := writeCodeCommentFixture(t)
	adapter := &Adapter{}
	cfg := config.WithCodeCommentArtifacts(config.DefaultRepoConfig(), true)

	candidates, err := adapter.Discover(context.Background(), root, cfg)

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, "billing/webhook.go", filepath.ToSlash(candidates[0].RelPath))
}

func TestParse_WithInvariantComment_ReturnsSourceContextArtifact(t *testing.T) {
	_, path := writeCodeCommentFixture(t)
	adapter := &Adapter{}
	candidate := adapters.Candidate{
		PrimaryPath:   path,
		RelPath:       "billing/webhook.go",
		AdapterName:   sourceType,
		UnitName:      "stripe_event_id must always be checked before applying credits",
		UnitBody:      "Invariant: stripe_event_id must always be checked before applying credits.",
		UnitLanguage:  "go",
		UnitStartLine: 3,
		UnitEndLine:   3,
		Role:          "invariant",
	}

	artifact, sources, _, err := adapter.Parse(context.Background(), candidate)

	require.NoError(t, err)
	assert.Equal(t, config.KindSourceContext, artifact.Kind)
	assert.Equal(t, config.SubtypeCodeComment, artifact.Subtype)
	assert.Equal(t, "intent", artifact.Extracted["mode"])
	assert.Equal(t, "invariant", artifact.Extracted["comment_role"])
	require.Len(t, sources, 1)
	assert.Equal(t, "code_comment", sources[0].SourceType)
}

func writeCodeCommentFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "billing", "webhook.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("package billing\n\n// Invariant: stripe_event_id must always be checked before applying credits.\nfunc applyCredit() {}\n"), 0o644))
	return root, path
}
