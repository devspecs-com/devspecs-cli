package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

func TestMapTextHidesReviewerDiagnosticsByDefault(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "payments-api")
	out := buildMapOutput(repoRoot, &scan.Result{
		Found: map[string]int{
			"source_context": 2,
			"test_case":      1,
		},
		WorkstreamEvidence: &scan.WorkstreamEvidenceDiagnostics{
			TopClusters: []scan.WorkstreamClusterExample{
				{
					Anchor:        "stripe webhook retry",
					Confidence:    0.9,
					EvidenceCount: 6,
					ExampleArtifacts: []scan.WorkstreamArtifactExample{
						{Kind: "source_context", Path: "internal/billing/webhook.go"},
						{Kind: "source_context", Path: "internal/billing/retry.go"},
						{Kind: "test_case", Path: "internal/billing/webhook_test.go"},
					},
				},
			},
		},
	}, mapOptions{MaxAreas: 4})

	var buf bytes.Buffer
	writeMapText(&buf, out, false)
	text := buf.String()
	for _, notWant := range []string{"Try changed", "Receipt changed", "Aha", "raw signal", "class=", "confidence="} {
		if strings.Contains(text, notWant) {
			t.Fatalf("default map output leaked reviewer diagnostic %q:\n%s", notWant, text)
		}
	}
	for _, want := range []string{"Repo map: payments-api", "Candidate subsystems", "Subsystem:", "Purpose:", "Boundary:", "Try: ds find", "Try: ds task"} {
		if !strings.Contains(text, want) {
			t.Fatalf("default map output missing %q:\n%s", want, text)
		}
	}
}

func TestMapJSONSchemaIsAgentReadable(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "orders")
	out := buildMapOutput(repoRoot, &scan.Result{
		Found: map[string]int{"source_context": 1},
		WorkstreamEvidence: &scan.WorkstreamEvidenceDiagnostics{
			TopClusters: []scan.WorkstreamClusterExample{
				{
					Anchor:        "order fulfillment",
					Confidence:    0.8,
					EvidenceCount: 3,
					ExampleArtifacts: []scan.WorkstreamArtifactExample{
						{Kind: "source_context", Path: "src/orders/fulfillment.go"},
					},
				},
			},
		},
	}, mapOptions{MaxAreas: 2})

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var decoded mapOutput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("map JSON did not round trip: %v\n%s", err, string(data))
	}
	if decoded.Schema != mapSchemaVersion {
		t.Fatalf("schema = %q, want %q", decoded.Schema, mapSchemaVersion)
	}
	if decoded.Repo.Name != "orders" {
		t.Fatalf("repo name = %q", decoded.Repo.Name)
	}
	if len(decoded.Areas) == 0 {
		t.Fatalf("expected at least one area in JSON:\n%s", string(data))
	}
	if decoded.Areas[0].AreaType == "" {
		t.Fatalf("expected area_type in JSON:\n%s", string(data))
	}
	if decoded.Areas[0].Purpose == "" || len(decoded.Areas[0].BoundaryPaths) == 0 {
		t.Fatalf("expected subsystem purpose and boundary paths in JSON:\n%s", string(data))
	}
}

func TestMapEvidenceCountsUsesPathFamilies(t *testing.T) {
	counts := mapEvidenceCounts([]scan.WorkstreamArtifactExample{
		{Kind: "source_context", Path: "src/click/core.py"},
		{Kind: "source_context", Path: "tests/test_core.py"},
		{Kind: "source_context", Path: "docs_src/tutorial001.py"},
	})
	if counts["source"] != 1 || counts["test"] != 1 || counts["doc"] != 1 {
		t.Fatalf("unexpected path-aware families: %#v", counts)
	}
}

func TestMapRootOnlyAreaStaysLowConfidence(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "requests")
	out := buildMapOutput(repoRoot, &scan.Result{
		Found: map[string]int{"source_context": 3},
		WorkstreamEvidence: &scan.WorkstreamEvidenceDiagnostics{
			TopClusters: []scan.WorkstreamClusterExample{
				{
					Anchor:        "requests",
					Confidence:    0.95,
					EvidenceCount: 8,
					ExampleArtifacts: []scan.WorkstreamArtifactExample{
						{Kind: "source_context", Path: "src/requests/api.py"},
						{Kind: "source_context", Path: "src/requests/sessions.py"},
					},
				},
			},
		},
	}, mapOptions{MaxAreas: 3})
	if out.Repo.Confidence != mapLowConfidence {
		t.Fatalf("root-only map confidence = %q, want low; areas=%#v", out.Repo.Confidence, out.Areas)
	}
	if len(out.Areas) == 0 || !out.Areas[0].IsRepoRootUmbrella {
		t.Fatalf("expected root umbrella area, got %#v", out.Areas)
	}
	if out.Areas[0].AreaType != mapTypeRoot {
		t.Fatalf("root area type = %q, want %q", out.Areas[0].AreaType, mapTypeRoot)
	}
}

func TestMapAreaTypeClassifiesProductBoundaries(t *testing.T) {
	out := buildProductMapTestOutput(t)

	types := map[string]string{}
	for _, area := range out.Areas {
		types[area.Label] = area.AreaType
	}
	if types["Flowable"] != mapTypeExternal {
		t.Fatalf("Flowable type = %q, want external integration; all=%#v", types["Flowable"], types)
	}
	if types["Status Pill"] != mapTypeUI {
		t.Fatalf("Status Pill type = %q, want UI surface; all=%#v", types["Status Pill"], types)
	}
	if types["Submission"] != mapTypeBusinessFlow {
		t.Fatalf("Submission type = %q, want business workflow; all=%#v", types["Submission"], types)
	}
}

func TestMapAreaDrilldownIsActionable(t *testing.T) {
	out := buildProductMapTestOutput(t)
	var buf bytes.Buffer
	writeMapAreaText(&buf, out, "submission", false)
	text := buf.String()
	for _, want := range []string{
		"Map area: Submission",
		"Type: business workflow",
		"Key files:",
		"apps/api/internal/submission/redaction.go",
		"Pack this context:",
		`ds find "submission redaction"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("area drilldown missing %q:\n%s", want, text)
		}
	}
}

func TestMapAreaDrilldownNoMatchListsAvailableAreas(t *testing.T) {
	out := buildProductMapTestOutput(t)
	var buf bytes.Buffer
	writeMapAreaText(&buf, out, "not-a-real-area", false)
	text := buf.String()
	for _, want := range []string{"No matching map area found.", "Available areas:", "Submission", "Flowable"} {
		if !strings.Contains(text, want) {
			t.Fatalf("no-match drilldown missing %q:\n%s", want, text)
		}
	}
}

func TestMapAreaDrilldownUsesMatchedDocTopicOverLocaleBucket(t *testing.T) {
	out := mapOutput{
		Schema: mapSchemaVersion,
		Repo:   mapRepo{Name: "fastapi", Path: t.TempDir(), Confidence: mapLowConfidence},
		Areas: []mapArea{{
			Label:      "Docs/Fr",
			AreaType:   mapTypeDocs,
			Confidence: mapLowConfidence,
			Covers:     []string{"Background Tasks", "Tutorial Background"},
			KeyPaths: []string{
				"docs/de/docs/tutorial/background-tasks.md",
				"docs/en/docs/tutorial/background-tasks.md",
				"docs/es/docs/tutorial/background-tasks.md",
				"docs/fr/docs/tutorial/background-tasks.md",
			},
			Try: "ds find \"docs fr background\"",
		}},
	}
	var buf bytes.Buffer
	writeMapAreaText(&buf, out, "background tasks", false)
	text := buf.String()
	for _, want := range []string{
		"Map area: Background Tasks",
		"docs/en/docs/tutorial/background-tasks.md",
		`ds find "background tasks"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("doc drilldown missing %q:\n%s", want, text)
		}
	}
	for _, notWant := range []string{
		"Map area: Docs/Fr",
		"docs/de/docs/tutorial/background-tasks.md",
		"docs/es/docs/tutorial/background-tasks.md",
		"docs/fr/docs/tutorial/background-tasks.md",
		`ds find "docs fr background"`,
	} {
		if strings.Contains(text, notWant) {
			t.Fatalf("doc drilldown leaked %q:\n%s", notWant, text)
		}
	}
}

func TestMapRecentTopicsSkipNoiseAndBuildPackHandoff(t *testing.T) {
	topics, skipped := buildMapRecentTopics([]parsedFindGitCommit{
		{
			sha:     "bot",
			subject: "Update dependency yaml-unist-parser to v3.2.0 (#19257)",
			body:    "Co-authored-by: renovate[bot] <29139614+renovate[bot]@users.noreply.github.com>",
			paths:   []string{"package.json", "yarn.lock"},
		},
		{
			sha:         "human",
			committedAt: "2026-05-22",
			subject:     "Implement public form endpoints",
			paths: []string{
				"apps/web/app/public-forms/page.tsx",
				"apps/api/internal/app/service.go",
				"apps/api/migrations/001_initial.sql",
			},
		},
	}, "", 5)
	if skipped != 1 {
		t.Fatalf("skipped = %d, want 1", skipped)
	}
	if len(topics) != 1 {
		t.Fatalf("topics = %#v, want one", topics)
	}
	topic := topics[0]
	if topic.Label != "Public Form Endpoints" {
		t.Fatalf("label = %q, want Public Form Endpoints", topic.Label)
	}
	if topic.Try != `ds find "public form endpoints"` {
		t.Fatalf("try = %q", topic.Try)
	}
	if topic.EvidenceCounts["source"] == 0 || topic.EvidenceCounts["config"] == 0 {
		t.Fatalf("expected source/config evidence, got %#v", topic.EvidenceCounts)
	}
}

func TestFastMapFallbackAddsIndexRequiredCaveatForUnindexedRepo(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "app")
	recent := mapRecentOutput{
		Schema: mapRecentSchemaVersion,
		Repo:   mapRepo{Name: "app", Path: repoRoot, Confidence: mapMediumConfidence},
		Topics: []mapRecentTopic{{
			Label:          "Partner Commission",
			Query:          "partner commission",
			CommitCount:    1,
			FileCount:      3,
			EvidenceCounts: map[string]int{"source": 2, "test": 1},
			KeyPaths:       []string{"apps/web/app/api/partner-profile/referrals/route.ts"},
			Try:            `ds find "partner commission"`,
		}},
	}

	out := buildFastMapFallbackOutputFromRecent(repoRoot, recent, false)
	if len(out.Areas) != 1 || out.Areas[0].Label != "Partner Commission" {
		t.Fatalf("unexpected fallback areas: %#v", out.Areas)
	}
	if !strings.Contains(strings.Join(out.Caveats, "\n"), mapIndexRequiredCaveat) {
		t.Fatalf("missing index-required caveat: %#v", out.Caveats)
	}

	indexed := buildFastMapFallbackOutputFromRecent(repoRoot, recent, true)
	if strings.Contains(strings.Join(indexed.Caveats, "\n"), mapIndexRequiredCaveat) {
		t.Fatalf("indexed fallback should not show index-required caveat: %#v", indexed.Caveats)
	}
}

func TestPathBoundaryMapUsesStablePathLabelOverRecentWorkstream(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "dub")
	files := []string{
		"apps/web/modules/webhooks/components/WebhookForm.tsx",
		"apps/web/modules/webhooks/lib/events.ts",
		"packages/features/webhooks/lib/constants.ts",
		"packages/features/webhooks/lib/dto/types.ts",
		"packages/features/webhooks/lib/webhook.test.ts",
	}
	commits := []parsedFindGitCommit{{
		sha:     "abc1234",
		subject: "fix: harden webhook link OAuth (#42)",
		paths: []string{
			"apps/web/modules/webhooks/lib/events.ts",
			"packages/features/webhooks/lib/constants.ts",
		},
	}}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "dub", files, commits, 5)
	if len(areas) == 0 {
		t.Fatalf("expected boundary areas")
	}
	if areas[0].Label != "Webhooks" {
		t.Fatalf("top label = %q, want stable path boundary Webhooks; areas=%#v", areas[0].Label, areas)
	}
	if strings.Contains(strings.ToLower(areas[0].Label), "harden") {
		t.Fatalf("recent workstream leaked into boundary label: %#v", areas[0])
	}
	if len(areas[0].TraceReceipts) == 0 || !strings.Contains(areas[0].TraceReceipts[0].Subject, "harden webhook") {
		t.Fatalf("expected recent commit as receipt, got %#v", areas[0].TraceReceipts)
	}
}

func TestPathBoundaryMapBuildsSubareasFromChildPaths(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "crm")
	files := []string{
		"packages/twenty-server/src/modules/workflow/workflow-executor/utils/should-execute-step.util.ts",
		"packages/twenty-server/src/modules/workflow/workflow-executor/utils/should-execute-step.util.test.ts",
		"packages/twenty-server/src/modules/workflow/workflow-builder/workflow-builder.service.ts",
		"packages/twenty-server/src/modules/workflow/workflow-trigger/workflow-trigger.service.ts",
		"packages/twenty-server/src/modules/workflow/docs/workflow-runtime.md",
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "twenty", files, nil, 5)
	var workflow *mapArea
	for i := range areas {
		if areas[i].Label == "Workflows & Automation" {
			workflow = &areas[i]
			break
		}
	}
	if workflow == nil {
		t.Fatalf("expected Workflows & Automation boundary, got %#v", areas)
	}
	covers := strings.Join(workflow.Covers, "\n")
	for _, want := range []string{"Workflow Executor", "Workflow Builder", "Workflow Trigger"} {
		if !strings.Contains(covers, want) {
			t.Fatalf("workflow covers missing %q: %#v", want, workflow.Covers)
		}
	}
	if workflow.EvidenceCounts["source"] == 0 || workflow.EvidenceCounts["test"] == 0 || workflow.EvidenceCounts["doc"] == 0 {
		t.Fatalf("expected role-diverse evidence, got %#v", workflow.EvidenceCounts)
	}
}

func TestPathBoundaryMapAddsImportStructureReceipts(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "dub")
	files := []string{
		"apps/web/modules/webhooks/lib/events.ts",
		"apps/web/modules/webhooks/lib/handler.ts",
		"apps/web/modules/webhooks/lib/handler.test.ts",
		"apps/web/modules/webhooks/components/WebhookForm.tsx",
	}
	writeMapTestFile(t, repoRoot, "apps/web/modules/webhooks/lib/events.ts", "export const webhookEvent = 'event';\n")
	writeMapTestFile(t, repoRoot, "apps/web/modules/webhooks/lib/handler.ts", "import { webhookEvent } from './events';\nexport const handler = webhookEvent;\n")
	writeMapTestFile(t, repoRoot, "apps/web/modules/webhooks/lib/handler.test.ts", "import { handler } from './handler';\nhandler;\n")
	writeMapTestFile(t, repoRoot, "apps/web/modules/webhooks/components/WebhookForm.tsx", "import { handler } from '../lib/handler';\nexport function WebhookForm(){ return handler; }\n")

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "dub", files, nil, 5)
	webhooks := findMapTestArea(areas, "Webhooks")
	if webhooks == nil {
		t.Fatalf("expected Webhooks area, got %#v", areas)
	}
	if webhooks.EvidenceCounts["import"] == 0 {
		t.Fatalf("expected import structure evidence, got %#v", webhooks.EvidenceCounts)
	}
	if webhooks.EvidenceCounts["test_import"] == 0 {
		t.Fatalf("expected test->source evidence, got %#v", webhooks.EvidenceCounts)
	}
	if !strings.Contains(mapAreaEvidenceText(webhooks.EvidenceCounts), "import structure") {
		t.Fatalf("evidence text missing import structure: %s", mapAreaEvidenceText(webhooks.EvidenceCounts))
	}
}

func TestPathBoundaryMapSuppressesWrapperAndDomainShellLabels(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "dub")
	files := []string{
		"apps/web/hooks/webhooks/use-webhook.ts",
		"apps/web/hooks/webhooks/use-webhook.test.ts",
		"apps/web/app/(ee)/app.dub.co/(dashboard)/partners/page.tsx",
		"apps/web/app/(ee)/app.dub.co/(dashboard)/partners/detail.tsx",
		"apps/web/app/(ee)/app.dub.co/(dashboard)/partners/fraud.tsx",
		"apps/web/app/(ee)/app.dub.co/(dashboard)/partners/settings.tsx",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "export const value = 1;\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "dub", files, nil, 8)
	for _, area := range areas {
		switch area.Label {
		case "Hooks", "Dashboard", "App Dub Co":
			t.Fatalf("wrapper/domain shell label leaked into map: %#v", areas)
		}
	}
	if findMapTestArea(areas, "Webhooks") == nil {
		t.Fatalf("expected Webhooks to survive wrapper suppression, got %#v", areas)
	}
	if findMapTestArea(areas, "Affiliate / Partner Programs") == nil {
		t.Fatalf("expected partner parent to survive domain-shell suppression, got %#v", areas)
	}
}

func TestPathBoundaryMapAggregatesDubConceptualParents(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "dub")
	files := []string{
		"apps/web/app/(ee)/app.dub.co/(dashboard)/partners/programs/page.tsx",
		"apps/web/app/(ee)/app.dub.co/(dashboard)/partners/commissions/page.tsx",
		"apps/web/app/(ee)/app.dub.co/(dashboard)/partners/payouts/page.tsx",
		"apps/web/app/(ee)/app.dub.co/(dashboard)/links/page.tsx",
		"apps/web/middleware/link.ts",
		"packages/tinybird/src/clicks.ts",
		"packages/tinybird/src/clicks.test.ts",
		"apps/web/app/api/tokens/route.ts",
		"packages/cli/src/commands/links.ts",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "export const value = 1;\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "dub", files, nil, 8)
	partners := findMapTestArea(areas, "Affiliate / Partner Programs")
	if partners == nil {
		t.Fatalf("expected Affiliate / Partner Programs, got %#v", areas)
	}
	if !strings.Contains(strings.Join(partners.Covers, "\n"), "Commissions") {
		t.Fatalf("partner parent missing concrete covers: %#v", partners.Covers)
	}
	redirect := findMapTestArea(areas, "Short-Link Redirect & Click Capture")
	if redirect == nil {
		t.Fatalf("expected Short-Link Redirect & Click Capture, got %#v", areas)
	}
	if !strings.Contains(redirect.Try, "click events") {
		t.Fatalf("conceptual try command should use concrete click substrate, got %q", redirect.Try)
	}
	if findMapTestArea(areas, "Program") != nil || findMapTestArea(areas, "Programs") != nil {
		t.Fatalf("child program labels should be folded behind parent: %#v", areas)
	}
}

func TestPathBoundaryMapAggregatesPlaneConceptualParents(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "plane")
	files := []string{
		"apps/api/plane/db/models/issue.py",
		"apps/api/plane/db/models/project.py",
		"apps/api/plane/db/models/state.py",
		"apps/api/plane/db/models/label.py",
		"apps/api/plane/bgtasks/issue.py",
		"apps/api/plane/urls.py",
		"apps/api/plane/migrations/001_initial.py",
		"apps/api/plane/celery.py",
		"apps/web/core/issues/issue-detail.tsx",
		"apps/web/core/projects/project-page.tsx",
		"apps/web/core/cycles/cycle-page.tsx",
		"apps/web/core/modules/module-view.tsx",
		"apps/web/core/workspace-views/rich-filters.tsx",
		"deployments/docker-compose.yml",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "value = 1\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "plane", files, nil, 8)
	workItems := findMapTestArea(areas, "Work Items & Project Delivery")
	if workItems == nil {
		t.Fatalf("expected Work Items & Project Delivery, got %#v", areas)
	}
	if !strings.Contains(strings.Join(workItems.Covers, "\n"), "Issues") {
		t.Fatalf("work item parent missing issue cover: %#v", workItems.Covers)
	}
	if findMapTestArea(areas, "Planning: Cycles, Modules & Views") == nil {
		t.Fatalf("expected planning parent, got %#v", areas)
	}
	if findMapTestArea(areas, "Django API, Persistence & Async Workers") == nil {
		t.Fatalf("expected Django API parent, got %#v", areas)
	}
	if findMapTestArea(areas, "States") != nil {
		t.Fatalf("suppressed implementation-shaped States label leaked: %#v", areas)
	}
}

func TestPathBoundaryMapDoesNotUseDjangoParentForGenericTypeScriptAPI(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "novu")
	files := []string{
		"apps/api/src/app/step-resolvers/utils/generate-step-resolver-worker-id.ts",
		"apps/api/admin/connect-to-dal.ts",
		"apps/api/admin/make-json-backup.ts",
		"apps/api/src/app/workflows-v2/workflow.controller.ts",
		"apps/api/src/app/workflows-v2/workflow.controller.e2e.ts",
		"apps/api/src/migrations/20240601_create_relations.ts",
		"apps/worker/src/app/workflow/usecases/queue-next-job/index.ts",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "export const value = 1;\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "novu", files, nil, 8)
	if findMapTestArea(areas, "Django API, Persistence & Async Workers") != nil {
		t.Fatalf("Django parent should require Python/Django evidence, got %#v", areas)
	}
}

func TestPathBoundaryMapAggregatesTwentyConceptualParents(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "twenty")
	files := []string{
		"packages/twenty-server/src/metadata-modules/object-metadata/object-metadata.service.ts",
		"packages/twenty-server/src/metadata-modules/field-metadata/field-metadata.service.ts",
		"packages/twenty-front/src/modules/settings/data-model/object-details.tsx",
		"packages/twenty-server/src/modules/object-record/object-record.service.ts",
		"packages/twenty-front/src/modules/object-record/record-table/record-table.tsx",
		"packages/twenty-server/src/modules/workflow/workflow-runner/runner.ts",
		"packages/twenty-front/src/modules/workflow/workflow-builder/builder.tsx",
		"packages/twenty-server/src/modules/graphql/graphql.controller.ts",
		"packages/twenty-server/src/modules/rest-api/rest-api.controller.ts",
		"packages/twenty-server/src/modules/connected-account/connected-account.service.ts",
		"packages/twenty-server/src/modules/messaging/mailbox.service.ts",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "export const value = 1;\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "twenty", files, nil, 8)
	if findMapTestArea(areas, "Metadata Engine & Data Model") == nil {
		t.Fatalf("expected metadata parent, got %#v", areas)
	}
	if findMapTestArea(areas, "CRM Record Experience") == nil {
		t.Fatalf("expected CRM record parent, got %#v", areas)
	}
	if findMapTestArea(areas, "Workflows & Automation") == nil {
		t.Fatalf("expected workflow parent, got %#v", areas)
	}
	api := findMapTestArea(areas, "Public API Layer")
	if api == nil {
		t.Fatalf("expected public API parent, got %#v", areas)
	}
	if strings.Contains(api.Try, "public api layer") || !(strings.Contains(api.Try, "graphql") || strings.Contains(api.Try, "rest")) {
		t.Fatalf("conceptual API try command should use constrained API child/path query, got %q", api.Try)
	}
}

func TestPathBoundaryMapRanksConceptualParentKeyFiles(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "twenty")
	files := []string{
		"packages/twenty-docs/getting-started/core-concepts/data-model.mdx",
		"packages/twenty-docs/l/ar/user-guide/data-model/capabilities/fields.mdx",
		"packages/twenty-docs/l/ar/developers/extend/apps/data-model.mdx",
		"packages/twenty-server/src/metadata-modules/object-metadata/object-metadata.service.ts",
		"packages/twenty-server/src/metadata-modules/field-metadata/field-metadata.service.ts",
		"packages/twenty-front/src/modules/settings/data-model/object-details.tsx",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "export const value = 1;\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "twenty", files, nil, 8)
	metadata := findMapTestArea(areas, "Metadata Engine & Data Model")
	if metadata == nil {
		t.Fatalf("expected metadata parent, got %#v", areas)
	}
	if len(metadata.KeyPaths) == 0 {
		t.Fatalf("expected key paths")
	}
	if strings.Contains(metadata.KeyPaths[0], "twenty-docs") {
		t.Fatalf("metadata parent should prefer implementation over docs, got %#v", metadata.KeyPaths)
	}
}

func TestPathBoundaryMapDedupesIdentityConceptualParents(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "crm")
	files := []string{
		"apps/web/app/auth/login/page.tsx",
		"apps/web/app/billing/plans/page.tsx",
		"apps/web/app/members/page.tsx",
		"packages/server/src/modules/auth/session.service.ts",
		"packages/server/src/modules/users/user.service.ts",
		"packages/server/src/modules/roles/permissions.service.ts",
		"packages/server/src/modules/workspaces/invitations.service.ts",
		"packages/server/src/modules/saml/saml.service.ts",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "export const value = 1;\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "crm", files, nil, 8)
	identityParents := 0
	for _, label := range []string{
		"Workspace Identity, Access & Billing",
		"Identity, Auth & Workspace Tenancy",
		"Identity, Auth & Access Control",
	} {
		if findMapTestArea(areas, label) != nil {
			identityParents++
		}
	}
	if identityParents != 1 {
		t.Fatalf("expected exactly one identity parent, got %d in %#v", identityParents, areas)
	}
}

func TestPathBoundaryMapUsesToolRepoParents(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "uv")
	files := []string{
		"crates/uv-workspace/src/workspace.rs",
		"crates/uv-workspace/src/pyproject.rs",
		"crates/uv-resolver/src/resolver/mod.rs",
		"crates/uv-resolver/src/lock.rs",
		"crates/uv-installer/src/installer.rs",
		"crates/uv-virtualenv/src/virtualenv.rs",
		"crates/uv-cache/src/lib.rs",
		"crates/uv-cache/src/archive.rs",
		"crates/uv-client/src/registry_client.rs",
		"crates/uv-python/src/interpreter.rs",
		"crates/uv-pip/src/compile.rs",
		"crates/uv-pip/src/install.rs",
		"crates/uv-tool/src/tool.rs",
		"crates/uv/tests/it/tool_install.rs",
		"crates/uv/src/commands/project/run.rs",
		"crates/uv-publish/src/lib.rs",
		"crates/uv-auth/src/lib.rs",
		"crates/uv-audit/src/lib.rs",
		"scripts/publish-crates.py",
		".github/ISSUE_TEMPLATE/1_bug_report.yaml",
		"test/packages/built-by-uv/assets/data.csv",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "pub fn value() {}\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "uv", files, nil, 8)
	for _, label := range []string{
		"Project & Workspace Lifecycle",
		"Dependency Resolution & Lockfile",
		"Package Installation & Virtual Environments",
		"Registry, Cache & Artifact Fetching",
		"Tools & Ephemeral Environments",
	} {
		if findMapTestArea(areas, label) == nil {
			t.Fatalf("expected tool parent %q, got %#v", label, areas)
		}
	}
	for _, label := range []string{
		"Crates",
		"Scripts",
		"Identity, Auth & Workspace Tenancy",
		"Work Items & Project Delivery",
		"Built By Uv",
		"Github",
		"Instance Administration & Licensing",
	} {
		if findMapTestArea(areas, label) != nil {
			t.Fatalf("tool repo shell/product label %q leaked into map: %#v", label, areas)
		}
	}
}

func TestPathBoundaryMapFoldsRailsShellsIntoProductParents(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "maybe")
	files := []string{
		"app/controllers/accounts_controller.rb",
		"app/models/account.rb",
		"db/migrate/20240202015428_create_accounts.rb",
		"app/controllers/transactions_controller.rb",
		"app/models/transaction.rb",
		"app/models/entry.rb",
		"db/migrate/20240223162105_create_transactions.rb",
		"app/controllers/budgets_controller.rb",
		"app/models/budget.rb",
		"app/controllers/investments_controller.rb",
		"app/models/holding.rb",
		"app/models/security.rb",
		"app/models/plaid_account/importer.rb",
		"app/models/plaid_item/importer.rb",
		"app/models/plaid_account/transactions/processor.rb",
		"app/models/plaid_item/accounts_snapshot.rb",
		"app/controllers/import/uploads_controller.rb",
		"app/models/import.rb",
		"test/fixtures/files/imports/transactions.csv",
		"app/controllers/settings/billings_controller.rb",
		"app/models/subscription.rb",
		"app/controllers/api/v1/accounts_controller.rb",
		"app/controllers/chats_controller.rb",
		"app/jobs/assistant_response_job.rb",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "class Value; end\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "maybe", files, nil, 8)
	for _, label := range []string{
		"Accounts & Net-Worth Dashboard",
		"Transaction Ledger, Categorization & Cashflow",
		"Budgeting",
		"Investments, Holdings & Securities",
		"Bank Connectivity & Plaid Sync",
		"CSV & Manual Data Import",
	} {
		if findMapTestArea(areas, label) == nil {
			t.Fatalf("expected finance/product parent %q, got %#v", label, areas)
		}
	}
	for _, label := range []string{"Controllers", "DB/Migrate"} {
		if findMapTestArea(areas, label) != nil {
			t.Fatalf("rails shell label %q leaked into map: %#v", label, areas)
		}
	}
}

func TestPathBoundaryMapFoldsPlatformShellsIntoCommerceParents(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "medusa")
	files := []string{
		"packages/core/framework/src/http/middlewares.ts",
		"packages/core/modules-sdk/src/index.ts",
		"packages/modules/product/src/services/product-module-service.ts",
		"packages/modules/pricing/src/services/pricing-module-service.ts",
		"packages/modules/inventory/src/services/inventory-module-service.ts",
		"packages/modules/cart/src/services/cart-module-service.ts",
		"packages/modules/promotion/src/services/promotion-module-service.ts",
		"packages/modules/order/src/services/order-module-service.ts",
		"packages/modules/fulfillment/src/services/fulfillment-module-service.ts",
		"packages/modules/payment/src/services/payment-module-service.ts",
		"packages/modules/tax/src/services/tax-module-service.ts",
		"packages/modules/store/src/services/store-module-service.ts",
		"packages/modules/sales-channel/src/services/sales-channel-module-service.ts",
		"packages/modules/providers/payment-stripe/src/index.ts",
		"packages/modules/providers/file-s3/src/index.ts",
		"packages/medusa/src/api/admin/orders/route.ts",
		"packages/medusa/src/api/store/carts/route.ts",
		"packages/medusa/src/api/auth/session/route.ts",
		"packages/admin/dashboard/src/routes/orders/order-list.tsx",
		"www/apps/api-reference/app/admin/page.tsx",
		"packages/design-system/icons/package.json",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "export const value = 1;\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "medusa", files, nil, 8)
	for _, label := range []string{
		"Framework Runtime & Module Platform",
		"Product Catalog, Pricing & Inventory",
		"Cart, Checkout & Promotions",
		"Orders, Fulfillment & Post-Purchase",
		"Payments, Tax & Monetary Configuration",
		"Provider Adapters & Pluggable Infrastructure",
	} {
		if findMapTestArea(areas, label) == nil {
			t.Fatalf("expected platform/commerce parent %q, got %#v", label, areas)
		}
	}
	for _, label := range []string{"Www", "Design System"} {
		if findMapTestArea(areas, label) != nil {
			t.Fatalf("platform shell label %q leaked into map: %#v", label, areas)
		}
	}
}

func TestPathBoundaryMapDiscoversDocumentSigningParentOverFrameworkShells(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "documenso")
	files := []string{
		"apps/remix/app/routes/_recipient+/sign.$token+/_index.tsx",
		"apps/remix/app/routes/_recipient+/sign.$token+/complete.tsx",
		"apps/remix/app/routes/embed+/v1+/authoring+/template.$templateId.tsx",
		"apps/remix/app/routes/embed+/v1+/authoring+/document.$documentId.tsx",
		"apps/remix/app/utils/field-signing/document-flow.ts",
		"apps/remix/app/utils/field-signing/signature.ts",
		"apps/remix/app/utils/field-signing/recipient.ts",
		"apps/remix/server/trpc/routers/document-router.ts",
		"packages/lib/server-only/document/create-document.ts",
		"packages/lib/jobs/definitions/emails/send-document-completed-email.ts",
		"packages/ui/primitives/signature-pad.tsx",
		"packages/ui/primitives/accordion.tsx",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "export const value = 1;\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "documenso", files, nil, 8)
	signing := findMapTestArea(areas, "Document Signing & Authoring")
	if signing == nil {
		t.Fatalf("expected document signing parent, got %#v", areas)
	}
	covers := strings.Join(signing.Covers, "\n")
	for _, want := range []string{"Documents", "Recipients", "Field Signing"} {
		if !strings.Contains(covers, want) {
			t.Fatalf("document signing parent missing cover %q: %#v", want, signing.Covers)
		}
	}
	for _, label := range []string{"Remix", "Trpc", "Server Only", "Primitives", "Universal"} {
		if findMapTestArea(areas, label) != nil {
			t.Fatalf("framework/package shell %q leaked into top-level map: %#v", label, areas)
		}
	}
}

func TestPathBoundaryMapDiscoversPlatformConceptsOverComposablesAndBlackbox(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "directus")
	files := []string{
		"app/src/composables/use-collection.ts",
		"app/src/composables/use-item.ts",
		"app/src/modules/content/routes/item.vue",
		"api/src/services/items.ts",
		"api/src/services/collections.ts",
		"api/src/services/fields.ts",
		"api/src/services/relations.ts",
		"api/src/database/migrations/20240601_create_relations.ts",
		"app/src/interfaces/input/input.vue",
		"app/src/displays/related-values/related-values.vue",
		"app/src/layouts/cards/cards.vue",
		"app/src/panels/metric/metric.vue",
		"api/src/flows/operations/webhook.ts",
		"api/src/operations/run-script.ts",
		"tests/blackbox/action-verify/create.test.ts",
		"tests/blackbox/action-verify/schema.test.ts",
		"sdk/src/rest/commands/server/openapi.ts",
		"api/src/controllers/graphql.ts",
		"api/src/ai/mcp/server.ts",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "export const value = 1;\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "directus", files, nil, 8)
	for _, label := range []string{
		"Content/Data Model",
		"Extension Surfaces",
		"Flows & Automation",
		"Public API Layer",
	} {
		if findMapTestArea(areas, label) == nil {
			t.Fatalf("expected platform concept parent %q, got %#v", label, areas)
		}
	}
	for _, label := range []string{"Composables", "Blackbox"} {
		if findMapTestArea(areas, label) != nil {
			t.Fatalf("implementation/test shell %q leaked into top-level map: %#v", label, areas)
		}
	}
	apiParents := 0
	for _, label := range []string{"Public API Layer", "Public HTTP API & Developer Platform"} {
		if findMapTestArea(areas, label) != nil {
			apiParents++
		}
	}
	if apiParents != 1 {
		t.Fatalf("expected exactly one API parent, got %d in %#v", apiParents, areas)
	}
}

func TestPathBoundaryMapDemotesFreshHoldoutShellBuckets(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "support-platform")
	files := []string{
		"app/javascript/dashboard/App.vue",
		"app/javascript/dashboard/api/ApiClient.js",
		"app/javascript/dashboard/components/Accordion.vue",
		"app/javascript/dashboard/components/AssignmentCard.vue",
		"app/javascript/dashboard/components/ConversationList.vue",
		"app/javascript/dashboard/components/InboxSettings.vue",
		"app/controllers/api/v1/accounts/agents_controller.rb",
		"app/controllers/api/v1/accounts/contact_merges_controller.rb",
		"app/controllers/api/v1/accounts/inboxes_controller.rb",
		"app/jobs/inboxes/fetch_imap_emails_job.rb",
		"spec/jobs/inboxes/fetch_imap_emails_job_spec.rb",
		"config/initializers/ai_agents.rb",
		"app/controllers/api/v1/accounts/assignable_agents_controller.rb",
		"db/migrate/20250820130619_add_two_factor_to_users.rb",
		"app/controllers/platform/api/v1/users_controller.rb",
		"app/javascript/dashboard/composables/useFileUpload.js",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "export const value = 1;\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "support-platform", files, nil, 8)
	if findMapTestArea(areas, "Javascript") != nil {
		t.Fatalf("javascript shell leaked into first-screen map: %#v", areas)
	}
	for _, label := range []string{
		"External HTTP API v1",
		"Connected Accounts, Email, Calendar & Timeline",
		"AI Agents, Chat & Skills",
		"Identity, Auth & Access Control",
	} {
		if findMapTestArea(areas, label) == nil {
			t.Fatalf("expected product/platform area %q, got %#v", label, areas)
		}
	}
}

func TestMapTryCommandDropsShellLabelsAndUsesSpecificCovers(t *testing.T) {
	if got := mapTryCommand("Locales", []string{"Admin Console"}, nil, mapHighConfidence, nil); got != `ds find "admin console"` {
		t.Fatalf("locales try = %q", got)
	}
	if got := mapTryCommand("Javascript", []string{"Accordion"}, nil, mapHighConfidence, nil); got != `ds find "accordion"` {
		t.Fatalf("javascript try = %q", got)
	}
	if got := mapTryCommand("Files, Assets & Storage", []string{"Upload"}, nil, mapHighConfidence, []string{"web/src/components/MemoEditor/hooks/useFileUpload.ts"}); got != `ds find "upload"` {
		t.Fatalf("broad storage try should prefer the specific cover, got %q", got)
	}
}

func TestMapTryCommandPrefersSpecificCoverForParentLabel(t *testing.T) {
	got := mapTryCommand(
		"Submission",
		[]string{"Redaction"},
		nil,
		mapHighConfidence,
		[]string{"apps/api/internal/submission/redaction.go"},
	)
	if got != `ds find "submission redaction"` {
		t.Fatalf("parent label try = %q", got)
	}
}

func TestMapTryCommandPrefersHighQualityTraceTaskOverPathTokens(t *testing.T) {
	got := mapTryCommandForRole(
		"Commands",
		nil,
		[]mapTraceReceipt{{
			SHA:     "abc1234",
			Subject: "feat: improve map handoff query quality",
		}},
		mapHighConfidence,
		[]string{
			"internal/commands/capture.go",
			"internal/commands/context.go",
			"internal/commands/criteria.go",
		},
		mapBoundaryRoleProductCapability,
	)
	if got != `ds find "map handoff query quality"` {
		t.Fatalf("trace task handoff = %q", got)
	}
}

func TestMapTryCommandKeepsSpecificCoverAboveUnrelatedTraceTask(t *testing.T) {
	got := mapTryCommandForRole(
		"Submission",
		[]string{"Redaction"},
		[]mapTraceReceipt{{
			SHA:     "abc1234",
			Subject: "feat: improve auth session timeout",
		}},
		mapHighConfidence,
		[]string{"apps/api/internal/submission/redaction.go"},
		mapBoundaryRoleProductCapability,
	)
	if got != `ds find "submission redaction"` {
		t.Fatalf("specific cover should beat unrelated trace task, got %q", got)
	}
}

func TestMapTryCommandRejectsTestPrefixTraceTask(t *testing.T) {
	got := mapTryCommandForRole(
		"Initflow",
		nil,
		[]mapTraceReceipt{{
			SHA:     "abc1234",
			Subject: "test: raise aggregate coverage above CI 80% floor",
		}},
		mapHighConfidence,
		[]string{
			"internal/initflow/initflow.go",
			"internal/initflow/merge.go",
			"internal/initflow/patterns.go",
		},
		mapBoundaryRoleProductCapability,
	)
	if strings.Contains(got, "raise aggregate coverage") {
		t.Fatalf("test prefix trace task leaked into handoff: %q", got)
	}
}

func TestMapTryCommandAvoidsLowValuePathLeafQueries(t *testing.T) {
	got := mapTryCommandForRole(
		"Operator",
		[]string{"Charts", "Crds"},
		nil,
		mapHighConfidence,
		[]string{
			"operator/Dockerfile.dockerignore",
			"operator/VERSION",
			"operator/api/core/v1alpha1/crds/grove.io_clustertopologybindings.yaml",
		},
		mapBoundaryRoleProductCapability,
	)
	if got == "" {
		t.Fatal("expected a handoff query")
	}
	if strings.Contains(got, "dockerfile") || strings.Contains(got, "dockerignore") || strings.Contains(got, "version") {
		t.Fatalf("low-value path leaf leaked into handoff: %q", got)
	}
	if !strings.Contains(got, "operator") && !strings.Contains(got, "charts") && !strings.Contains(got, "crds") {
		t.Fatalf("handoff lost useful area terms: %q", got)
	}
}

func TestMapTryCommandAvoidsGeneratedFixtureLeafQueries(t *testing.T) {
	got := mapTryCommandForRole(
		"Fixedbugs",
		[]string{"Arm64 Bitfield Overlap"},
		nil,
		mapHighConfidence,
		[]string{
			"test/fixedbugs/arm64bitfieldoverlap.go",
			"test/fixedbugs/bug000.go",
			"test/fixedbugs/bug002.go",
		},
		mapBoundaryRoleProductCapability,
	)
	if got == "" {
		t.Fatal("expected a handoff query")
	}
	if strings.Contains(got, "bug000") || strings.Contains(got, "bug002") {
		t.Fatalf("generated fixture leaf leaked into handoff: %q", got)
	}
	if !strings.Contains(got, "arm64") && !strings.Contains(got, "fixedbugs") {
		t.Fatalf("handoff lost useful fixture-family terms: %q", got)
	}
}

func TestMapTryCommandConstrainsBroadBoundaryRoles(t *testing.T) {
	api := mapTryCommandForRole(
		"Public API Layer",
		[]string{"GraphQL", "Subscriptions"},
		nil,
		mapHighConfidence,
		[]string{"saleor/graphql/subscriptions/resolver.py"},
		mapBoundaryRoleGenericParent,
	)
	if api == "" || strings.Contains(api, "public api layer") || !strings.Contains(api, "subscriptions") {
		t.Fatalf("broad API parent should hand off to specific child/path evidence, got %q", api)
	}

	platform := mapTryCommandForRole(
		"Platform",
		[]string{"Advisor Reports"},
		nil,
		mapHighConfidence,
		[]string{"src/Appwrite/Platform/Modules/Advisor/Reports/Report.php"},
		mapBoundaryRoleGenericParent,
	)
	if platform == "" || strings.Contains(platform, `"platform"`) || !strings.Contains(platform, "advisor") {
		t.Fatalf("generic platform parent should avoid standalone platform query, got %q", platform)
	}
}

func TestMapTryCommandPrefersSpecificExtensionCover(t *testing.T) {
	got := mapTryCommandForRole(
		"Plugins",
		[]string{"Acl", "Acme", "Ai Prompt Guard"},
		nil,
		mapHighConfidence,
		[]string{"kong/plugins/ai-prompt-guard/handler.lua"},
		mapBoundaryRoleExtensionEcosystem,
	)
	if got == "" || !strings.Contains(got, "ai prompt guard") || strings.Contains(got, "plugins ai") {
		t.Fatalf("extension ecosystem try should prefer specific plugin cover, got %q", got)
	}
}

func TestMapTryCommandSuppressesUnpackableBoundaryHandoff(t *testing.T) {
	idx := newMapTestPackabilityIndex("cmd/actions.go")
	got, diag := mapTryCommandForRoleWithPackability(
		"Plugins",
		[]string{"Acl", "Acme", "Ai Prompt Guard"},
		nil,
		mapHighConfidence,
		[]string{
			"kong/plugins/ai-prompt-guard/filters/guard-prompt.lua",
			"kong/plugins/ai-prompt-guard/handler.lua",
			"kong/plugins/ai-prompt-guard/schema.lua",
		},
		mapBoundaryRoleExtensionEcosystem,
		idx,
	)
	if got != "" {
		t.Fatalf("unpackable try = %q, want suppressed", got)
	}
	if diag == nil || !diag.TrySuppressed {
		t.Fatalf("expected suppression diagnostics, got %#v", diag)
	}
	if diag.Decision != "suppressed_no_indexed_support" {
		t.Fatalf("decision = %q", diag.Decision)
	}
	if len(diag.MissingKeyExtensions) != 1 || diag.MissingKeyExtensions[0] != ".lua" {
		t.Fatalf("missing extensions = %#v", diag.MissingKeyExtensions)
	}
}

func TestMapTryCommandKeepsSupportedBoundaryHandoff(t *testing.T) {
	idx := newMapTestPackabilityIndex(
		"extensions/displays/list.ts",
		"extensions/displays/panel.ts",
	)
	got, diag := mapTryCommandForRoleWithPackability(
		"Extension Surfaces",
		[]string{"Types Extensions Displays"},
		nil,
		mapHighConfidence,
		[]string{
			"extensions/displays/list.ts",
			"extensions/displays/panel.ts",
		},
		mapBoundaryRoleExtensionEcosystem,
		idx,
	)
	if got == "" {
		t.Fatal("supported boundary handoff was suppressed")
	}
	if diag == nil || diag.Decision != "supported" {
		t.Fatalf("expected supported diagnostics, got %#v", diag)
	}
	if diag.IndexedKeyPathCount != 2 {
		t.Fatalf("indexed key paths = %d, want 2", diag.IndexedKeyPathCount)
	}
}

func TestMapAreaPackCommandsRespectsSuppressedTry(t *testing.T) {
	area := mapArea{
		Label:        "Plugins",
		Confidence:   mapHighConfidence,
		Covers:       []string{"Ai Prompt Guard"},
		KeyPaths:     []string{"kong/plugins/ai-prompt-guard/handler.lua"},
		BoundaryRole: mapBoundaryRoleExtensionEcosystem,
		Diagnostics: mapAreaDiagnostics{
			Packability: &mapPackabilityDiagnostics{TrySuppressed: true},
		},
	}
	if got := mapAreaPackCommands(area); len(got) != 0 {
		t.Fatalf("suppressed area pack commands = %#v", got)
	}
}

func newMapTestPackabilityIndex(paths ...string) *mapPackabilityIndex {
	idx := &mapPackabilityIndex{
		paths: map[string]string{},
		dirs:  map[string]int{},
		words: map[string]int{},
	}
	for _, path := range paths {
		idx.add(path, "source_context")
	}
	return idx
}

func TestMapBoundaryRoleClassifiesUnsafeParents(t *testing.T) {
	platform := &mapAreaInternal{Key: "platform", Label: "Platform", EvidenceCounts: map[string]int{"source": 4}}
	if got := classifyMapBoundaryRole(platform, "Platform", mapTypePlatform, false, []string{"Advisor Reports"}, "appwrite", mapRepoShapePlatform); got != mapBoundaryRoleGenericParent {
		t.Fatalf("platform role = %q", got)
	}

	playground := &mapAreaInternal{Key: "playground", Label: "Playground", EvidenceCounts: map[string]int{"source": 4}}
	if got := classifyMapBoundaryRole(playground, "Playground", mapTypeTooling, false, nil, "vite", mapRepoShapeTool); got != mapBoundaryRoleFixtureOrTestbed {
		t.Fatalf("playground role = %q", got)
	}

	namespace := &mapAreaInternal{Key: "gitea-repositories-meta", Label: "Gitea Repositories Meta", EvidenceCounts: map[string]int{"source": 4}}
	if got := classifyMapBoundaryRole(namespace, "Gitea Repositories Meta", mapTypeDomainFeature, false, nil, "gitea", mapRepoShapePlatform); got != mapBoundaryRoleRepoNamespace {
		t.Fatalf("repo namespace role = %q", got)
	}

	plugins := &mapAreaInternal{
		Key:            "plugins",
		Label:          "Plugins",
		EvidenceCounts: map[string]int{"source": 4},
		Artifacts: []mapArtifact{{
			Path: "tests/fixtures/plugins/ai-prompt-guard/index.ts",
		}},
	}
	if got := classifyMapBoundaryRole(plugins, "Plugins", mapTypePlatform, false, []string{"Ai Prompt Guard"}, "kong", mapRepoShapePlatform); got != mapBoundaryRoleExtensionEcosystem {
		t.Fatalf("plugin role should not become fixture from support path, got %q", got)
	}
}

func TestMapBoundaryPathEligibleSkipsEmbeddedGitFixtures(t *testing.T) {
	path := "tests/gitea-repositories-meta/limited_org/private_repo_on_limited_org.git/objects/74/8bf557dfc9c6457998b5118a6c8b2129f56c30"
	if mapBoundaryPathEligible(path) {
		t.Fatalf("embedded git fixture path should be ineligible")
	}
}

func TestPathBoundaryMapPrefersImplementationKeyFilesOverTestsAndExamples(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "cms")
	files := []string{
		"test/fields-relationship/collections/Collection1/index.ts",
		"test/fields-relationship/collections/Collection2/index.ts",
		"examples/with-fields/payload.config.ts",
		"packages/payload/src/collections/config/fields/buildFieldSchemaMap.ts",
		"packages/payload/src/collections/config/fields/buildClientFieldSchemaMap.ts",
		"packages/payload/src/collections/operations/find.ts",
		"packages/payload/src/fields/config/types.ts",
	}
	for _, file := range files {
		writeMapTestFile(t, repoRoot, file, "export const value = 1;\n")
	}

	areas, _, _ := buildPathBoundaryAreas(repoRoot, "cms", files, nil, 8)
	dataModel := findMapTestArea(areas, "Content/Data Model")
	if dataModel == nil {
		t.Fatalf("expected Content/Data Model, got %#v", areas)
	}
	if len(dataModel.KeyPaths) == 0 {
		t.Fatalf("expected key paths")
	}
	if strings.HasPrefix(dataModel.KeyPaths[0], "test/") || strings.HasPrefix(dataModel.KeyPaths[0], "examples/") {
		t.Fatalf("content/data model should prefer implementation key files, got %#v", dataModel.KeyPaths)
	}
}

func TestMapAreaMatchPrefersPluralLabelOverPathOnlyMatch(t *testing.T) {
	areas := []mapArea{
		{Label: "Cron", KeyPaths: []string{"apps/web/app/api/cron/notify-partners/route.ts"}, Diagnostics: mapAreaDiagnostics{TraceTerms: []string{"partner"}}},
		{Label: "Partners", KeyPaths: []string{"apps/web/app/partners/fraud/page.tsx"}},
	}
	matches := matchMapAreas(areas, "partner")
	if len(matches) == 0 {
		t.Fatalf("expected matches")
	}
	if matches[0].Area.Label != "Partners" {
		t.Fatalf("top match = %q, want Partners; matches=%#v", matches[0].Area.Label, matches)
	}
}

func TestMapRecentTopicsFilterByAreaQuery(t *testing.T) {
	topics, _ := buildMapRecentTopics([]parsedFindGitCommit{
		{
			sha:     "bounce",
			subject: "feat: implement bounce feature for blips",
			paths: []string{
				"apps/app/app/blip/[id]/bounce.tsx",
				"backend/internal/application/blip/bounce_blip.go",
			},
		},
		{
			sha:     "release",
			subject: "Replace main branch in changelog link with tags (#19054)",
			paths:   []string{"scripts/release/steps/show-instructions-after-npm-publish.js"},
		},
	}, "bounce", 5)
	if len(topics) != 1 {
		t.Fatalf("filtered topics = %#v, want one", topics)
	}
	if !strings.Contains(topics[0].Query, "bounce") {
		t.Fatalf("filtered topic = %#v, want bounce", topics[0])
	}
}

func TestMapRecentTextAvoidsTaskStatusClaims(t *testing.T) {
	out := mapRecentOutput{
		Schema: mapRecentSchemaVersion,
		Repo:   mapRepo{Name: "repo", Path: t.TempDir(), Confidence: mapMediumConfidence},
		Topics: []mapRecentTopic{{
			Label:          "Expedition Enemy Pressure",
			Query:          "expedition enemy pressure",
			CommitCount:    1,
			FileCount:      2,
			EvidenceCounts: map[string]int{"source": 2},
			KeyPaths:       []string{"server/internal/core/pressure.go", "client/src/game/Game.ts"},
			RecentSignals: []mapTraceReceipt{{
				SHA:     "df68f82",
				Subject: "Add expedition enemy pressure phases A-D for Killer Slice 001.",
			}},
			Try: `ds find "expedition enemy pressure"`,
		}},
	}
	var buf bytes.Buffer
	writeMapRecentText(&buf, out, false)
	text := buf.String()
	for _, want := range []string{
		"Recently active topics",
		"Expedition Enemy Pressure",
		"Evidence: 1 commit, 2 files, source",
		"Recent signal: df68f82 Add expedition enemy pressure phases A-D for Killer Slice 001.",
		`Try: ds find "expedition enemy pressure"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("recent output missing %q:\n%s", want, text)
		}
	}
	for _, notWant := range []string{"Open tasks", "In progress", "Done", "Stale", "Resume work"} {
		if strings.Contains(text, notWant) {
			t.Fatalf("recent output made task-status claim %q:\n%s", notWant, text)
		}
	}
}

func TestRecentCommandShowsRecentTopics(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustMkdirAll(t, filepath.Join(repoRoot, "internal", "security"))
	mustWriteFile(t, filepath.Join(repoRoot, "internal", "security", "credentials.go"), "package security\n")
	mustWriteFile(t, filepath.Join(repoRoot, "internal", "security", "credentials_test.go"), "package security\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "feat: credentials rotation context")

	cmd := NewRecentCmd()
	cmd.SetArgs([]string{"--path", repoRoot, "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	for _, want := range []string{
		"Recently active topics",
		"Credentials Rotation",
		"Try: ds find",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("recent output missing %q:\n%s", want, text)
		}
	}

	jsonCmd := NewRecentCmd()
	jsonCmd.SetArgs([]string{"credentials", "--path", repoRoot, "--no-refresh", "--json"})
	jsonBuf := &bytes.Buffer{}
	jsonCmd.SetOut(jsonBuf)
	if err := jsonCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out mapRecentOutput
	if err := json.Unmarshal(jsonBuf.Bytes(), &out); err != nil {
		t.Fatalf("recent json: %v\n%s", err, jsonBuf.String())
	}
	if out.Schema != mapRecentSchemaVersion || len(out.Topics) != 1 {
		t.Fatalf("recent json output = %#v", out)
	}
	if !strings.Contains(out.Topics[0].Query, "credentials") {
		t.Fatalf("recent topic query = %q", out.Topics[0].Query)
	}
}

func TestMapRecentFlagRemainsCompatibilityPath(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustMkdirAll(t, filepath.Join(repoRoot, "internal", "billing"))
	mustWriteFile(t, filepath.Join(repoRoot, "internal", "billing", "refunds.go"), "package billing\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "fix: refund retry receipts")

	cmd := NewMapCmd()
	cmd.SetArgs([]string{"--recent", "--path", repoRoot, "--no-refresh"})
	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.Contains(text, "Recently active topics") || !strings.Contains(text, "Refund Retry") {
		t.Fatalf("map --recent compatibility output missing recent topics:\nstdout:\n%s\nstderr:\n%s", text, errBuf.String())
	}
}

func mapTestGit(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func TestFastMapFallbackConvertsRecentTopicToMapArea(t *testing.T) {
	topics, skipped := buildMapRecentTopics([]parsedFindGitCommit{{
		sha:         "yaml",
		committedAt: "2026-06-01",
		subject:     "Update to yaml@2 (#18419)",
		paths: []string{
			"src/language-yaml/parser-yaml.js",
			"src/language-yaml/printer-yaml.js",
			"tests/format/yaml/spec/format.test.js",
		},
	}}, "", 5)
	if skipped != 0 || len(topics) != 1 {
		t.Fatalf("topics=%#v skipped=%d", topics, skipped)
	}
	area := mapAreaFromRecentTopic(topics[0])
	if area.Label != "YAML Format Language" {
		t.Fatalf("label = %q", area.Label)
	}
	if area.Try != `ds find "yaml format language"` {
		t.Fatalf("try = %q", area.Try)
	}
	if area.EvidenceCounts["source"] == 0 || area.EvidenceCounts["test"] == 0 {
		t.Fatalf("expected source/test evidence counts: %#v", area.EvidenceCounts)
	}
	if area.AreaType == "" {
		t.Fatalf("expected area type: %#v", area)
	}
}

func TestBuildCachedMapResultUsesStoredWorkstreamEdges(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := "2026-06-01T00:00:00Z"
	repoRoot := filepath.Join(t.TempDir(), "repo")
	if _, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('repo_cached', ?, ?, ?)", repoRoot, now, now); err != nil {
		t.Fatal(err)
	}
	mustMapTestNoErr(t, db.InsertArtifactDirect("ds_game", "repo_cached", "source_context", "", "Game", "unknown", "rev_game", now, now))
	mustMapTestNoErr(t, db.InsertArtifactDirect("ds_camera", "repo_cached", "source_context", "", "Camera RTS", "unknown", "rev_camera", now, now))
	mustMapTestNoErr(t, db.InsertSourceDirect("src_game", "ds_game", "repo_cached", "source_context", "client/src/game/Game.ts", "client/src/game/Game.ts|source_context", "", "", now))
	mustMapTestNoErr(t, db.InsertSourceDirect("src_camera", "ds_camera", "repo_cached", "source_context", "client/src/world/cameraRTS.ts", "client/src/world/cameraRTS.ts|source_context", "", "", now))
	mustMapTestNoErr(t, db.UpsertArtifactEdge(store.ArtifactEdgeInput{
		ID:            "edge_rts",
		RepoID:        "repo_cached",
		SrcArtifactID: "ds_game",
		DstArtifactID: "ds_camera",
		EdgeType:      "same_workstream_anchor",
		Weight:        0.8,
		Confidence:    0.9,
		EvidenceCount: 3,
		SourceSignal:  "workstream_anchor",
		Explanation:   `shares workstream anchor "rts camera mode"`,
		MetadataJSON:  `{"anchors":[{"anchor":"rts camera mode"}],"pack_strength":"support_local","role_mix":{"source":2}}`,
	}, now))

	result, ok, err := buildCachedMapResult(db, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected cached map result")
	}
	out := buildMapOutput(repoRoot, result, mapOptions{MaxAreas: 4})
	if len(out.Areas) == 0 {
		t.Fatalf("expected cached areas: %#v", out)
	}
	if got := out.Areas[0].Label; got != "Game" {
		t.Fatalf("cached map label = %q, want Game; areas=%#v", got, out.Areas)
	}
	if !strings.Contains(strings.Join(out.Areas[0].Covers, "\n"), "Rts Camera Mode") {
		t.Fatalf("cached map covers missing Rts Camera Mode: %#v", out.Areas[0].Covers)
	}
	if !strings.Contains(strings.Join(out.Areas[0].KeyPaths, "\n"), "cameraRTS.ts") {
		t.Fatalf("cached area missing source path: %#v", out.Areas[0].KeyPaths)
	}
}

func TestMapTryCommandAvoidsUnsupportedCommitVerb(t *testing.T) {
	query := mapTryCommand("Release", []string{"Publish Npm"}, []mapTraceReceipt{{
		SHA:     "abc1234",
		Subject: "Replace `main` branch in changelog link with tags (#19054)",
	}}, mapMediumConfidence, nil)
	if query != `ds find "release publish npm"` {
		t.Fatalf("query = %q, want release publish npm", query)
	}
	commands := mapAreaPackCommands(mapArea{
		Label:         "Release",
		Confidence:    mapMediumConfidence,
		Covers:        []string{"Publish Npm"},
		TraceReceipts: []mapTraceReceipt{{Subject: "Replace `main` branch in changelog link with tags (#19054)"}},
		Try:           query,
	})
	for _, cmd := range commands {
		if strings.Contains(cmd, "release replace") {
			t.Fatalf("unsupported commit verb leaked into commands: %#v", commands)
		}
	}
}

func TestMapOutputCacheRoundTripsFreshMap(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	db, err := store.Open(filepath.Join(home, "devspecs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := "2026-06-01T00:00:00Z"
	repoRoot := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO repos (id, root_path, last_scan_at, created_at, updated_at) VALUES ('repo_cache_out', ?, ?, ?, ?)", repoRoot, now, now, now); err != nil {
		t.Fatal(err)
	}
	out := mapOutput{
		Schema: mapSchemaVersion,
		Repo: mapRepo{
			Name:       "repo",
			Path:       repoRoot,
			Confidence: mapMediumConfidence,
		},
		Areas: []mapArea{{Label: "Release", Try: `ds find "release publish npm"`}},
	}
	if err := saveMapOutputCache(repoRoot, mapDefaultMaxAreas, out); err != nil {
		t.Fatal(err)
	}
	got, ok, err := loadMapOutputCache(repoRoot, mapDefaultMaxAreas)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected map output cache hit")
	}
	if got.Areas[0].Try != out.Areas[0].Try {
		t.Fatalf("cached try = %q, want %q", got.Areas[0].Try, out.Areas[0].Try)
	}
}

func TestMapAutoScanLeavesUsableIndexForFindPack(t *testing.T) {
	repoRoot := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	writeMapTestFile(t, repoRoot, "plans/credentials-plan.md", "# Credentials Rotation\n\nRotate credentials for webhook ingestion.\n")
	writeMapTestFile(t, repoRoot, "app/auth/credentials.go", "package auth\n\nfunc RotateCredentials() {}\n")
	runGitForFindPack(t, repoRoot, "add", ".")
	runGitForFindPack(t, repoRoot, "commit", "-m", "add credentials rotation context")

	mapCmd := NewMapCmd()
	mapCmd.SetArgs([]string{"--path", repoRoot, "--max-areas", "4"})
	mapOut := &bytes.Buffer{}
	mapErr := &bytes.Buffer{}
	mapCmd.SetOut(mapOut)
	mapCmd.SetErr(mapErr)
	if err := mapCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mapErr.String(), "Index updated") {
		t.Fatalf("expected map to auto-scan missing index, stderr: %s", mapErr.String())
	}
	if strings.Contains(mapOut.String(), mapIndexRequiredCaveat) {
		t.Fatalf("map should not ask for manual scan after auto-scan:\n%s", mapOut.String())
	}

	oldWd, _ := os.Getwd()
	os.Chdir(repoRoot)
	defer os.Chdir(oldWd)

	findCmd := NewFindCmd()
	findCmd.SetArgs([]string{"credentials rotation", "--no-refresh"})
	findOut := &bytes.Buffer{}
	findErr := &bytes.Buffer{}
	findCmd.SetOut(findOut)
	findCmd.SetErr(findErr)
	if err := findCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(findErr.String(), "Index updated") {
		t.Fatalf("find --no-refresh should use map-created index without rescanning, stderr: %s", findErr.String())
	}
	output := findOut.String()
	if !strings.Contains(output, "Working set: credentials rotation") || !strings.Contains(output, "Credentials Rotation") {
		t.Fatalf("find did not use map-created index.\nOutput: %s\nStderr: %s", output, findErr.String())
	}
}

func TestMapNoRefreshSkipsAutoScan(t *testing.T) {
	repoRoot := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	writeMapTestFile(t, repoRoot, "plans/credentials-plan.md", "# Credentials Rotation\n\nRotate credentials for webhook ingestion.\n")
	writeMapTestFile(t, repoRoot, "app/auth/credentials.go", "package auth\n\nfunc RotateCredentials() {}\n")
	runGitForFindPack(t, repoRoot, "add", ".")
	runGitForFindPack(t, repoRoot, "commit", "-m", "add credentials rotation context")

	mapCmd := NewMapCmd()
	mapCmd.SetArgs([]string{"--path", repoRoot, "--max-areas", "4", "--no-refresh"})
	mapOut := &bytes.Buffer{}
	mapErr := &bytes.Buffer{}
	mapCmd.SetOut(mapOut)
	mapCmd.SetErr(mapErr)
	if err := mapCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(mapErr.String(), "Index updated") {
		t.Fatalf("map --no-refresh should not auto-scan, stderr: %s", mapErr.String())
	}
	if !strings.Contains(mapOut.String(), mapIndexRequiredCaveat) {
		t.Fatalf("map --no-refresh should disclose missing local index:\n%s", mapOut.String())
	}
}

func TestMapJSONAutoScanKeepsStdoutJSON(t *testing.T) {
	repoRoot := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	writeMapTestFile(t, repoRoot, "plans/credentials-plan.md", "# Credentials Rotation\n\nRotate credentials for webhook ingestion.\n")
	writeMapTestFile(t, repoRoot, "app/auth/credentials.go", "package auth\n\nfunc RotateCredentials() {}\n")
	runGitForFindPack(t, repoRoot, "add", ".")
	runGitForFindPack(t, repoRoot, "commit", "-m", "add credentials rotation context")

	cmd := NewMapCmd()
	cmd.SetArgs([]string{"--json", "--path", repoRoot})
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errBuf.String(), "Index updated") {
		t.Fatalf("expected JSON map to auto-scan missing index, stderr: %s", errBuf.String())
	}
	var out mapOutput
	if err := json.Unmarshal(outBuf.Bytes(), &out); err != nil {
		t.Fatalf("map --json stdout should remain valid JSON: %v\nstdout=%s\nstderr=%s", err, outBuf.String(), errBuf.String())
	}
	if out.Schema != mapSchemaVersion || out.Repo.Path == "" {
		t.Fatalf("unexpected JSON map payload: %#v", out)
	}
	if strings.Contains(outBuf.String(), "Index updated") {
		t.Fatalf("scan notice leaked into JSON stdout:\n%s", outBuf.String())
	}
}

func TestFilterMapOutputByAreaQueryNarrowsJSONPayload(t *testing.T) {
	out := buildProductMapTestOutput(t)
	filtered := filterMapOutputByAreaQuery(out, "redaction")
	if len(filtered.Areas) != 1 {
		t.Fatalf("filtered area count = %d, want 1; areas=%#v", len(filtered.Areas), filtered.Areas)
	}
	if filtered.Areas[0].Label != "Submission" {
		t.Fatalf("filtered label = %q, want Submission", filtered.Areas[0].Label)
	}
	if filtered.Diagnostics.AreaQuery != "redaction" || filtered.Diagnostics.MatchedAreaCount != 1 {
		t.Fatalf("unexpected diagnostics: %#v", filtered.Diagnostics)
	}
}

func TestRefineMapAreaLabelMakesLayerLabelsMoreProductReadable(t *testing.T) {
	if got := refineMapAreaLabel("Lib Anthropic", []string{"Anthropic Ts"}); got != "Anthropic" {
		t.Fatalf("refined lib label = %q, want Anthropic", got)
	}
	if got := refineMapAreaLabel("Application", []string{"Blip Get Canonical Path"}); got != "Blip Application" {
		t.Fatalf("refined application label = %q, want Blip Application", got)
	}
	if got := cleanMapCovers("Game", []string{"Ks", "Rts Camera Mode"}); strings.Join(got, ", ") != "Rts Camera Mode" {
		t.Fatalf("clean covers kept short raw anchor: %#v", got)
	}
}

func TestMapRecentSubjectTermsFiltersFillerLabels(t *testing.T) {
	got := mapRecentSubjectTerms("fix: clean up error handling, fix a proto-pollution gap, and seal a few loose ends")
	if strings.Join(got, " ") != "proto pollution" {
		t.Fatalf("recent subject terms = %#v", got)
	}

	got = mapRecentSubjectTerms("feat: add open spec")
	if len(got) != 0 {
		t.Fatalf("open spec subject should defer to path terms, got %#v", got)
	}
}

func buildProductMapTestOutput(t *testing.T) mapOutput {
	t.Helper()
	repoRoot := filepath.Join(t.TempDir(), "product")
	return buildMapOutput(repoRoot, &scan.Result{
		Found: map[string]int{"source_context": 6, "test_case": 2},
		WorkstreamEvidence: &scan.WorkstreamEvidenceDiagnostics{
			TopClusters: []scan.WorkstreamClusterExample{
				{
					Anchor:        "flowable process definitions",
					Confidence:    0.9,
					EvidenceCount: 4,
					ExampleArtifacts: []scan.WorkstreamArtifactExample{
						{Kind: "source_context", Path: "app/api/private/flowable/v1/process_definitions.py"},
						{Kind: "source_context", Path: "app/core/flowable.py"},
					},
				},
				{
					Anchor:        "status pill",
					Confidence:    0.85,
					EvidenceCount: 3,
					ExampleArtifacts: []scan.WorkstreamArtifactExample{
						{Kind: "source_context", Path: "apps/web/components/status-pill.tsx"},
					},
				},
				{
					Anchor:        "submission redaction",
					Confidence:    0.8,
					EvidenceCount: 3,
					ExampleArtifacts: []scan.WorkstreamArtifactExample{
						{Kind: "source_context", Path: "apps/api/internal/submission/redaction.go"},
						{Kind: "test_case", Path: "apps/api/internal/submission/redaction_test.go"},
					},
				},
			},
		},
	}, mapOptions{MaxAreas: 6})
}

func mustMapTestNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func writeMapTestFile(t *testing.T, repoRoot, rel, body string) {
	t.Helper()
	full := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findMapTestArea(areas []mapArea, label string) *mapArea {
	for i := range areas {
		if areas[i].Label == label {
			return &areas[i]
		}
	}
	return nil
}
