package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
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
	if !strings.Contains(joined, findPackScoutBodyEvidencePrefix) {
		t.Fatalf("missing body evidence reason: %#v", item.Reasons)
	}
	if got.Metadata["pack_scout_body_evidence_count"] != "1" {
		t.Fatalf("missing evidence count metadata: %#v", got.Metadata)
	}
	if got.Metadata["pack_scout_body_evidence_bytes"] == "" || got.Metadata["pack_scout_body_evidence_bytes"] == "0" {
		t.Fatalf("missing evidence bytes metadata: %#v", got.Metadata)
	}
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
	if got.Metadata != nil {
		t.Fatalf("non-rescued row should not trigger body evidence: %#v", got.Metadata)
	}
}

func TestConcisePackReasonsShowsBoundedBodyEvidence(t *testing.T) {
	got := concisePackReasons([]string{
		"scout source rescue: query roots anim; primary test roots anim",
		"bounded body evidence: animation=3, callback=1; bytes=120",
	})
	joined := strings.Join(got, "; ")
	if !strings.Contains(joined, "body evidence: animation=3, callback=1; bytes=120") {
		t.Fatalf("missing concise body evidence: %#v", got)
	}
}

func mustWriteScoutEvidenceFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
