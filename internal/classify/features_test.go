package classify

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractFeaturesMarkdownStructure(t *testing.T) {
	body := strings.Join([]string{
		"---",
		"status: accepted",
		"tags: [billing, webhook]",
		"---",
		"# ADR 0002: Webhook Idempotency Boundary",
		"",
		"## Context",
		"Stripe retries events and can deliver them out of order.",
		"",
		"## Decision",
		"The boundary is keyed by `stripe_event_id`.",
		"",
		"## Consequences",
		"- [ ] Record `stripe_event_id` before side effects.",
		"- [x] Keep `webhook_replay_protection` quiet on duplicates.",
	}, "\n")

	features := ExtractFeatures("docs/adr/0002-webhook-idempotency-boundary.md", body)
	require.Equal(t, "accepted", features.Frontmatter["status"],
		"frontmatter status got %q", features.Frontmatter["status"])
	require.Equal(t, "billing, webhook", features.Frontmatter["tags"],
		"frontmatter tags got %q", features.Frontmatter["tags"])
	require.Equal(t, "ADR 0002: Webhook Idempotency Boundary", features.Title,
		"title got %q", features.Title)
	require.Len(t, features.Headings, 4,
		"headings got %#v", features.Headings)
	require.Equal(t, 5, features.Headings[0].Line,
		"first heading line got %d", features.Headings[0].Line)
	require.Equal(t, 2, features.ChecklistItems,
		"checklist count got %d", features.ChecklistItems)

	assertContains(t, features.StatusPhrases, "status:accepted")
	assertContains(t, features.LifecyclePhrases, "accepted")
	assertContains(t, features.Identifiers, "stripe_event_id")
	assertContains(t, features.Identifiers, "webhook_replay_protection")
	assertSectionRole(t, features.Sections, "Context", "context")
	assertSectionRole(t, features.Sections, "Decision", "decision")
	assertSectionRole(t, features.Sections, "Consequences", "consequences")
}

func TestExtractFeaturesPathDatesAndReferences(t *testing.T) {
	body := "See `services/api/src/billing/webhooks.ts` and https://example.com/rfc/1.\n```sql\nselect 1;\n```\n"
	features := ExtractFeatures("services/api/migrations/20260501090000_add_stripe_event_id.sql", body)

	assertContains(t, features.PathTokens, "services")
	assertContains(t, features.FilenameTokens, "20260501090000")
	assertContains(t, features.FilenameTokens, "stripe")
	assertContains(t, features.DateTokens, "20260501090000")
	assertContains(t, features.Identifiers, "stripe_event_id")
	assertContains(t, features.PathReferences, "services/api/src/billing/webhooks.ts")
	assertContains(t, features.LinkTargets, "https://example.com/rfc/1")
	assertContains(t, features.CodeFenceLanguages, "sql")
}

func TestExtractFeaturesMarkersAndLocalTerms(t *testing.T) {
	body := strings.Join([]string{
		"# Old Plan",
		"Generated release notes. Do not edit.",
		"This stale webhook plan is superseded by the active webhook ADR.",
		"The webhook migration keeps webhook retries idempotent.",
	}, "\n")
	features := ExtractFeatures("scratch/old-webhook-retry-investigation.md", body)

	assertContains(t, features.Markers, MarkerGenerated)
	assertContains(t, features.Markers, MarkerChangelog)
	assertContains(t, features.Markers, MarkerStale)
	assertContains(t, features.Markers, MarkerSuperseded)
	assertContains(t, features.Markers, MarkerScratch)
	assertContains(t, features.LocalTerms, "webhook")
}

func TestEnrichCandidate(t *testing.T) {
	c := EnrichCandidate(Candidate{
		Path:  "docs/prd/billing-entitlements-v1.md",
		Scope: ScopeDocument,
		Body:  "# PRD: Billing Entitlements v1\n\n## User outcomes\n\nUsers can manage billing.",
	})
	require.Equal(t, "PRD: Billing Entitlements v1", c.Features.Title,
		"title got %q", c.Features.Title)

	assertContains(t, c.Features.PathTokens, "prd")
	assertSectionRole(t, c.Features.Sections, "User outcomes", "product")
}

func assertContains(t *testing.T, items []string, want string) {
	t.Helper()
	require.Contains(t, items, want)
}

func assertSectionRole(t *testing.T, sections []Section, heading, role string) {
	t.Helper()
	var found *Section
	for index := range sections {
		if sections[index].Heading == heading {
			found = &sections[index]
			break
		}
	}
	require.NotNil(t, found, "missing section %q in %#v", heading, sections)
	assert.Equal(t, role, found.Role)
}
