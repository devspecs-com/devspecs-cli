package commands

import (
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
)

func TestDisplayPackReasons_HidesDebugScoreAndGenericTerms(t *testing.T) {
	got := displayPackReasons([]string{
		"anchor-first ranking: score 24.000; matches activity, event, query; fields path, title, body",
		"query term match in body: how",
		"query term match in path: activity",
	}, false)
	joined := strings.Join(got, "; ")
	if strings.Contains(joined, "score") {
		t.Fatalf("display reasons leaked scorer internals: %#v", got)
	}
	if strings.Contains(joined, "how") {
		t.Fatalf("display reasons kept generic task word: %#v", got)
	}
	for _, want := range []string{"matched anchors: activity, event, query", "path matched: activity"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("display reasons missing %q: %#v", want, got)
		}
	}
}

func TestDisplayPackReasons_CompactsSectionReceipts(t *testing.T) {
	got := displayPackReasons([]string{
		"section-packed context: Architecture Design > Human Attention Optimization > Two-Tier Event Handling; Architecture Design > The 8 Plugin Slots > Agent",
		"indexed section match: Architecture Design > Human Attention Optimization lines 395-418; Architecture Design > The 8 Plugin Slots lines 100-120",
	}, false)
	joined := strings.Join(got, "; ")
	for _, want := range []string{"section focus:", "section evidence:"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("display reasons missing compact section label %q: %#v", want, got)
		}
	}
	if strings.Contains(joined, "section-packed context") || strings.Contains(joined, "indexed section match") {
		t.Fatalf("display reasons leaked internal section labels: %#v", got)
	}
}

func TestPackCoverageText_UsesRoleNames(t *testing.T) {
	got := packCoverageText(retrieval.PackSummary{
		HasBackgroundDecisions: true,
		HasImplementation:      true,
		HasBehaviorTests:       true,
	})
	if got != "background + implementation + tests" {
		t.Fatalf("coverage text = %q", got)
	}
}
