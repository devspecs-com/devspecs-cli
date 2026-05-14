package classify

import (
	"strings"
	"testing"
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

	if features.Frontmatter["status"] != "accepted" {
		t.Fatalf("frontmatter status got %q", features.Frontmatter["status"])
	}
	if features.Frontmatter["tags"] != "billing, webhook" {
		t.Fatalf("frontmatter tags got %q", features.Frontmatter["tags"])
	}
	if features.Title != "ADR 0002: Webhook Idempotency Boundary" {
		t.Fatalf("title got %q", features.Title)
	}
	if len(features.Headings) != 4 {
		t.Fatalf("headings got %#v", features.Headings)
	}
	if features.Headings[0].Line != 5 {
		t.Fatalf("first heading line got %d", features.Headings[0].Line)
	}
	if features.ChecklistItems != 2 {
		t.Fatalf("checklist count got %d", features.ChecklistItems)
	}
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
	if c.Features.Title != "PRD: Billing Entitlements v1" {
		t.Fatalf("title got %q", c.Features.Title)
	}
	assertContains(t, c.Features.PathTokens, "prd")
	assertSectionRole(t, c.Features.Sections, "User outcomes", "product")
}

func assertContains(t *testing.T, items []string, want string) {
	t.Helper()
	for _, item := range items {
		if item == want {
			return
		}
	}
	t.Fatalf("missing %q in %#v", want, items)
}

func assertSectionRole(t *testing.T, sections []Section, heading, role string) {
	t.Helper()
	for _, section := range sections {
		if section.Heading == heading {
			if section.Role != role {
				t.Fatalf("%q role got %q want %q", heading, section.Role, role)
			}
			return
		}
	}
	t.Fatalf("missing section %q in %#v", heading, sections)
}
