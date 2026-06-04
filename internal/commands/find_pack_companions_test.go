package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

func TestAddFindPackCompanionCandidatesAddsDirectTestFile(t *testing.T) {
	matches := []retrieval.Candidate{
		{ID: "map", Path: "internal/commands/map.go", Kind: "source_context", Title: "map command"},
	}
	all := append([]retrieval.Candidate{}, matches...)
	all = append(all, retrieval.Candidate{
		ID:    "map-test",
		Path:  "internal/commands/map_test.go",
		Kind:  "source_context",
		Title: "map command tests",
	})

	got := addFindPackCompanionCandidates(context.Background(), "", "map command output", matches, all)

	if !findPackTestHasPath(got, "internal/commands/map_test.go") {
		t.Fatalf("expected direct test companion, got %#v", findPackTestPaths(got))
	}
	companion := findPackTestCandidate(got, "internal/commands/map_test.go")
	if companion.Metadata["retrieval_expansion_reason"] != "test_companion" {
		t.Fatalf("expected test companion reason, got %#v", companion.Metadata)
	}
	if companion.Metadata["pack_tier"] != retrieval.PackTierRelated {
		t.Fatalf("expected related pack tier, got %#v", companion.Metadata)
	}
}

func TestAddFindPackCompanionCandidatesAddsFilesystemTestCompanion(t *testing.T) {
	repoRoot := t.TempDir()
	writeFindPackTestFile(t, repoRoot, "internal/commands/map_test.go", "package commands\n\nfunc TestMapOutput(t *testing.T) {}\n")
	matches := []retrieval.Candidate{
		{ID: "map", Path: "internal/commands/map.go", Kind: "source_context", Title: "map command"},
	}

	got := addFindPackCompanionCandidates(context.Background(), repoRoot, "map command output", matches, matches)

	companion := findPackTestCandidate(got, "internal/commands/map_test.go")
	if companion.Path == "" {
		t.Fatalf("expected filesystem test companion, got %#v", findPackTestPaths(got))
	}
	if companion.Subtype != "test_case" {
		t.Fatalf("expected filesystem test companion subtype test_case, got %#v", companion)
	}
	if companion.Metadata["admission_reason"] != "query_time_pack_companion" {
		t.Fatalf("expected query-time companion admission metadata, got %#v", companion.Metadata)
	}
}

func TestAddFindPackCompanionCandidatesAddsCommandFamilyFiles(t *testing.T) {
	matches := []retrieval.Candidate{
		{ID: "map", Path: "internal/commands/map.go", Kind: "source_context", Title: "map command"},
	}
	all := append([]retrieval.Candidate{}, matches...)
	for _, path := range []string{
		"internal/commands/find.go",
		"internal/commands/find_test.go",
		"internal/commands/map_test.go",
		"internal/commands/read_commands_test.go",
		"internal/commands/refresh.go",
		"internal/commands/freshness_test.go",
	} {
		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})
	}

	got := addFindPackCompanionCandidates(context.Background(), "", "auto scan map and find first use output cache", matches, all)

	for _, want := range []string{
		"internal/commands/map_test.go",
		"internal/commands/find.go",
		"internal/commands/find_test.go",
		"internal/commands/read_commands_test.go",
		"internal/commands/refresh.go",
		"internal/commands/freshness_test.go",
	} {
		if !findPackTestHasPath(got, want) {
			t.Fatalf("expected %s in command family companions, got %#v", want, findPackTestPaths(got))
		}
	}
}

func TestScoreFindGitReceiptsReturnsRelatedTouchedPaths(t *testing.T) {
	receipts := scoreFindGitReceipts([]parsedFindGitCommit{
		{
			sha:         "abc123456",
			committedAt: "2026-06-04",
			subject:     "Tighten map command output tests",
			paths: []string{
				"internal/commands/map.go",
				"internal/commands/map_test.go",
				"internal/commands/read_commands_test.go",
				"docs/map-output.md",
			},
		},
	}, []string{"internal/commands/map.go"}, "map command output")

	if len(receipts) != 1 {
		t.Fatalf("expected one receipt, got %#v", receipts)
	}
	for _, want := range []string{"internal/commands/map_test.go", "internal/commands/read_commands_test.go"} {
		if !findPackStringSliceContains(receipts[0].RelatedPaths, want) {
			t.Fatalf("expected related path %s, got %#v", want, receipts[0].RelatedPaths)
		}
	}
	if findPackStringSliceContains(receipts[0].RelatedPaths, "docs/map-output.md") {
		t.Fatalf("did not expect doc path in related pack diagnostics: %#v", receipts[0].RelatedPaths)
	}
}

func TestWriteGitTrustTextShowsRelatedTouchedPaths(t *testing.T) {
	buf := &bytes.Buffer{}
	writeGitTrustText(buf, &FindGitTrustContext{
		Receipts: []FindGitReceipt{
			{
				ShortSHA:     "abc1234",
				CommittedAt:  "2026-06-04",
				Subject:      "Tighten map command output tests",
				MatchedPaths: []string{"internal/commands/map.go"},
				RelatedPaths: []string{"internal/commands/map_test.go", "internal/commands/read_commands_test.go"},
			},
		},
	})

	output := buf.String()
	for _, want := range []string{
		"Related files from matching commits, not admitted to pack:",
		"- internal/commands/map_test.go",
		"- internal/commands/read_commands_test.go",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("git trust text missing %q:\n%s", want, output)
		}
	}
}

func TestAddFindPackCompanionCandidatesAddsParentCommandForHelperFile(t *testing.T) {
	matches := []retrieval.Candidate{
		{ID: "find-pack", Path: "internal/commands/find_pack.go", Kind: "source_context", Title: "find pack"},
	}
	all := append([]retrieval.Candidate{}, matches...)
	for _, path := range []string{
		"internal/commands/find.go",
		"internal/commands/find_test.go",
		"internal/commands/find_pack_test.go",
	} {
		all = append(all, retrieval.Candidate{ID: path, Path: path, Kind: "source_context", Title: path})
	}

	got := addFindPackCompanionCandidates(context.Background(), "", "boundary primary packs", matches, all)

	for _, want := range []string{
		"internal/commands/find.go",
		"internal/commands/find_test.go",
		"internal/commands/find_pack_test.go",
	} {
		if !findPackTestHasPath(got, want) {
			t.Fatalf("expected helper command family path %s, got %#v", want, findPackTestPaths(got))
		}
	}
}

func writeFindPackTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findPackTestCandidate(candidates []retrieval.Candidate, path string) retrieval.Candidate {
	for _, candidate := range candidates {
		if candidate.Path == path {
			return candidate
		}
	}
	return retrieval.Candidate{}
}

func findPackTestHasPath(candidates []retrieval.Candidate, path string) bool {
	return findPackTestCandidate(candidates, path).Path != ""
}

func findPackTestPaths(candidates []retrieval.Candidate) []string {
	var out []string
	for _, candidate := range candidates {
		out = append(out, candidate.Path)
	}
	return out
}

func findPackStringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
