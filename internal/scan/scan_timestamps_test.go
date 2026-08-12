package scan

import (
	"context"
	"testing"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScan_UnchangedBodyLeavesUpdatedAtStable(t *testing.T) {
	repoRoot, db := setupTestRepo(t)
	seeded := seedExistingMarkdownScanState(t, db, repoRoot)
	scanner := New(db, idgen.NewFactory(), []adapters.Adapter{&markdown.Adapter{}})

	_, err := scanner.RunWithOptions(context.Background(), repoRoot, nil, RunOptions{SkipAuthoredAtLookup: true})
	require.NoError(t, err)

	var updatedAt, lastObservedAt string
	require.NoError(t, db.QueryRow("SELECT updated_at, last_observed_at FROM artifacts WHERE id = ?", seeded.ArtifactID).Scan(&updatedAt, &lastObservedAt))
	assert.Equal(t, seeded.UpdatedAt, updatedAt,
		"updated_at changed on unchanged body: %q -> %q", seeded.UpdatedAt, updatedAt)
	seededAt, err := time.Parse(time.RFC3339, seeded.UpdatedAt)
	require.NoError(t, err)
	observedAt, err := time.Parse(time.RFC3339, lastObservedAt)
	require.NoError(t, err)
	assert.False(t, observedAt.Before(seededAt),
		"last_observed_at went backwards: %q -> %q", seeded.UpdatedAt, lastObservedAt)

}
