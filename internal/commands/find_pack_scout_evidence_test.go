package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddFindPackScoutBodyEvidenceAnnotatesRescuedRows(t *testing.T) {
	repoRoot := t.TempDir()
	mustWriteScoutEvidenceFile(t, filepath.Join(repoRoot, "src", "textual", "_animator.py"), `
class Animator:
    def animate(self, on_complete=None):
        # animation complete callback
        if on_complete is not None:
            on_complete()
`)
	pack := retrieval.RoleGroupedPack{
		Groups: []retrieval.PackGroup{{
			Role: retrieval.PackRoleImplementation,
			Items: []retrieval.PackItem{{
				OriginalRank: 1,
				ID:           "animator",
				Path:         "src/textual/_animator.py",
				Role:         retrieval.PackRoleImplementation,
				PackTier:     retrieval.PackTierPrimary,
				Reasons:      []string{"scout source rescue: query roots anim; primary test roots anim"},
			}},
		}},
	}

	got := addFindPackScoutBodyEvidence(repoRoot, "Fix on complete animation callback", pack)
	item := got.Groups[0].Items[0]
	joined := strings.Join(item.Reasons, "\n")
	assert.Contains(t, joined, findPackScoutBodyEvidencePrefix,
		"missing body evidence reason: %#v", item.Reasons)
	assert.Equal(t, "1", got.Metadata["pack_scout_body_evidence_count"],
		"missing evidence count metadata: %#v", got.Metadata)
	assert.NotEqual(t, "", got.Metadata["pack_scout_body_evidence_bytes"], "missing evidence bytes metadata: %#v", got.Metadata)
	assert.NotEqual(t, "0", got.Metadata["pack_scout_body_evidence_bytes"], "missing evidence bytes metadata: %#v", got.Metadata)

}

func TestAddFindPackScoutBodyEvidenceSkipsNonRescuedRows(t *testing.T) {
	repoRoot := t.TempDir()
	mustWriteScoutEvidenceFile(t, filepath.Join(repoRoot, "src", "textual", "_animator.py"), `animation callback complete`)
	pack := retrieval.RoleGroupedPack{
		Groups: []retrieval.PackGroup{{
			Role: retrieval.PackRoleImplementation,
			Items: []retrieval.PackItem{{
				OriginalRank: 1,
				ID:           "animator",
				Path:         "src/textual/_animator.py",
				Role:         retrieval.PackRoleImplementation,
				PackTier:     retrieval.PackTierPrimary,
			}},
		}},
	}

	got := addFindPackScoutBodyEvidence(repoRoot, "Fix on complete animation callback", pack)
	require.Nil(t, got.Metadata,
		"non-rescued row should not trigger body evidence: %#v", got.Metadata)

}

func TestConcisePackReasonsShowsBoundedBodyEvidence(t *testing.T) {
	got := concisePackReasons([]string{
		"scout source rescue: query roots anim; primary test roots anim",
		"bounded body evidence: animation=3, callback=1; bytes=120",
	})
	joined := strings.Join(got, "; ")
	assert.Contains(t, joined, "body evidence: animation=3, callback=1; bytes=120",
		"missing concise body evidence: %#v", got)

}

func mustWriteScoutEvidenceFile(t *testing.T, path, body string) {
	t.Helper()
	{
		err := os.MkdirAll(filepath.Dir(path), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(path, []byte(body), 0o644)
		require.NoError(t, err)
	}

}
