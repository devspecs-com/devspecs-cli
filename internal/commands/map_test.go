package commands

import (
	"bytes"
	"encoding/json"
	"os"
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
	for _, want := range []string{"Repo map: payments-api", "Candidate areas", "Type:", "Try: ds find --pack"} {
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
		`ds find --pack "submission redaction"`,
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
			Try: "ds find --pack \"docs fr background\"",
		}},
	}
	var buf bytes.Buffer
	writeMapAreaText(&buf, out, "background tasks", false)
	text := buf.String()
	for _, want := range []string{
		"Map area: Background Tasks",
		"docs/en/docs/tutorial/background-tasks.md",
		`ds find --pack "background tasks"`,
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
		`ds find --pack "docs fr background"`,
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
	if topic.Try != `ds find --pack "public form endpoints"` {
		t.Fatalf("try = %q", topic.Try)
	}
	if topic.EvidenceCounts["source"] == 0 || topic.EvidenceCounts["config"] == 0 {
		t.Fatalf("expected source/config evidence, got %#v", topic.EvidenceCounts)
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
			Try: `ds find --pack "expedition enemy pressure"`,
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
		`Try: ds find --pack "expedition enemy pressure"`,
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
	}}, mapMediumConfidence)
	if query != `ds find --pack "release publish npm"` {
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
		Areas: []mapArea{{Label: "Release", Try: `ds find --pack "release publish npm"`}},
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
