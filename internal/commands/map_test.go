package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.NotContainsf(t,
		text,
		("Try changed"), "default map output leaked reviewer diagnostic %q:\n%s",

		("Try changed"), text)
	assert.NotContainsf(t,
		text,
		("Receipt changed"), "default map output leaked reviewer diagnostic %q:\n%s",

		("Receipt changed"), text)
	assert.NotContainsf(t,
		text,
		("Aha"), "default map output leaked reviewer diagnostic %q:\n%s",

		("Aha"), text)
	assert.NotContainsf(t,
		text,
		("raw signal"), "default map output leaked reviewer diagnostic %q:\n%s",

		("raw signal"), text)
	assert.NotContainsf(t,
		text,
		("class="),
		"default map output leaked reviewer diagnostic %q:\n%s",

		("class="), text)
	assert.NotContainsf(t,
		text,
		("confidence="), "default map output leaked reviewer diagnostic %q:\n%s",

		("confidence="), text)
	assert.Containsf(t, text,
		("Repo map: payments-api"), "default map output missing %q:\n%s",

		("Repo map: payments-api"), text)
	assert.Containsf(t, text,
		("Candidate subsystems"), "default map output missing %q:\n%s",

		("Candidate subsystems"), text)
	assert.Containsf(t, text,
		("Subsystem:"), "default map output missing %q:\n%s",

		("Subsystem:"), text)
	assert.Containsf(t, text,
		("Purpose:"),
		"default map output missing %q:\n%s",

		("Purpose:"), text)
	assert.Containsf(t, text,
		("Boundary:"),
		"default map output missing %q:\n%s",

		("Boundary:"), text)
	assert.Containsf(t, text,
		("Try: ds find"), "default map output missing %q:\n%s",

		("Try: ds find"), text)
	assert.Containsf(t, text,
		("Try: ds task"), "default map output missing %q:\n%s",

		("Try: ds task"), text)

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
	require.NoError(t, err)

	var decoded mapOutput
	{
		err := json.Unmarshal(data, &decoded)
		require.NoErrorf(t, err,
			"map JSON did not round trip: %v\n%s", err, string(data))
	}
	assert.Equalf(t, mapSchemaVersion, decoded.Schema,
		"schema = %q, want %q", decoded.Schema, mapSchemaVersion)
	assert.Equalf(t, "orders", decoded.Repo.Name,
		"repo name = %q", decoded.Repo.Name)
	require.NotEmptyf(t, decoded.Areas,
		"expected at least one area in JSON:\n%s", string(data))
	assert.NotEqualf(t, "", decoded.Areas[0].AreaType,
		"expected area_type in JSON:\n%s", string(data))
	assert.NotEmpty(t, decoded.Areas[0].Purpose)
	require.NotEmpty(t, decoded.Areas[0].BoundaryPaths)

}

func TestMapEvidenceCountsUsesPathFamilies(t *testing.T) {
	counts := mapEvidenceCounts([]scan.WorkstreamArtifactExample{
		{Kind: "source_context", Path: "src/click/core.py"},
		{Kind: "source_context", Path: "tests/test_core.py"},
		{Kind: "source_context", Path: "docs_src/tutorial001.py"},
	})
	assert.Equal(t, 1, counts["source"])
	assert.Equal(t, 1, counts["test"])
	assert.Equal(t, 1, counts["doc"])

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
	assert.Equalf(t, mapLowConfidence, out.Repo.Confidence,
		"root-only map confidence = %q, want low; areas=%#v", out.Repo.Confidence, out.Areas)
	require.NotEmpty(t, out.Areas)
	assert.True(t, out.Areas[0].IsRepoRootUmbrella)

	assert.Equalf(t, mapTypeRoot, out.Areas[0].AreaType,
		"root area type = %q, want %q", out.Areas[0].AreaType, mapTypeRoot)

}

func TestMapAreaTypeClassifiesProductBoundaries(t *testing.T) {
	out := buildProductMapTestOutput(t)

	types := map[string]string{}
	for _, area := range out.Areas {
		types[area.Label] = area.AreaType
	}
	assert.Equalf(t, mapTypeExternal, types["Flowable"],
		"Flowable type = %q, want external integration; all=%#v", types["Flowable"], types)
	assert.Equalf(t, mapTypeUI, types["Status Pill"],
		"Status Pill type = %q, want UI surface; all=%#v", types["Status Pill"], types)
	assert.Equalf(t, mapTypeBusinessFlow, types["Submission"],
		"Submission type = %q, want business workflow; all=%#v", types["Submission"], types)

}

func TestMapAreaDrilldownIsActionable(t *testing.T) {
	out := buildProductMapTestOutput(t)
	var buf bytes.Buffer
	writeMapAreaText(&buf, out, "submission", false)
	text := buf.String()
	assert.Containsf(t, text,
		("Map area: Submission"), "area drilldown missing %q:\n%s",

		("Map area: Submission"), text)
	assert.Containsf(t, text,
		("Type: business workflow"), "area drilldown missing %q:\n%s",

		("Type: business workflow"), text)
	assert.Containsf(t, text,
		("Key files:"), "area drilldown missing %q:\n%s",

		("Key files:"), text)
	assert.Containsf(t, text,
		("apps/api/internal/submission/redaction.go"), "area drilldown missing %q:\n%s",

		("apps/api/internal/submission/redaction.go"), text)
	assert.Containsf(t, text,
		("Pack this context:"), "area drilldown missing %q:\n%s",

		("Pack this context:"), text)
	assert.Containsf(t, text,
		(`ds find "submission redaction"`), "area drilldown missing %q:\n%s",

		(`ds find "submission redaction"`), text,
	)

}

func TestMapAreaDrilldownNoMatchListsAvailableAreas(t *testing.T) {
	out := buildProductMapTestOutput(t)
	var buf bytes.Buffer
	writeMapAreaText(&buf, out, "not-a-real-area", false)
	text := buf.String()
	assert.Containsf(t, text,
		("No matching map area found."), "no-match drilldown missing %q:\n%s",

		("No matching map area found."), text,
	)
	assert.Containsf(t, text,
		("Available areas:"), "no-match drilldown missing %q:\n%s",

		("Available areas:"), text)
	assert.Containsf(t, text,
		("Submission"), "no-match drilldown missing %q:\n%s",

		("Submission"), text)
	assert.Containsf(t, text,
		("Flowable"),
		"no-match drilldown missing %q:\n%s",

		("Flowable"), text)

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
	assert.Containsf(t, text,
		("Map area: Background Tasks"),
		"doc drilldown missing %q:\n%s",

		("Map area: Background Tasks"), text)
	assert.Containsf(t, text,
		("docs/en/docs/tutorial/background-tasks.md"), "doc drilldown missing %q:\n%s",

		("docs/en/docs/tutorial/background-tasks.md"), text)
	assert.Containsf(t, text,
		(`ds find "background tasks"`),
		"doc drilldown missing %q:\n%s",

		(`ds find "background tasks"`), text)
	assert.NotContainsf(t,
		text,
		("Map area: Docs/Fr"), "doc drilldown leaked %q:\n%s",

		("Map area: Docs/Fr"), text)
	assert.NotContainsf(t,
		text,
		("docs/de/docs/tutorial/background-tasks.md"), "doc drilldown leaked %q:\n%s",

		("docs/de/docs/tutorial/background-tasks.md"), text)
	assert.NotContainsf(t,
		text,
		("docs/es/docs/tutorial/background-tasks.md"), "doc drilldown leaked %q:\n%s",

		("docs/es/docs/tutorial/background-tasks.md"), text)
	assert.NotContainsf(t,
		text,
		("docs/fr/docs/tutorial/background-tasks.md"), "doc drilldown leaked %q:\n%s",

		("docs/fr/docs/tutorial/background-tasks.md"), text)
	assert.NotContainsf(t,
		text,
		(`ds find "docs fr background"`), "doc drilldown leaked %q:\n%s",

		(`ds find "docs fr background"`), text,
	)

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
	assert.Equalf(t, 1, skipped,
		"skipped = %d, want 1", skipped)
	require.Lenf(t, topics, 1,
		"topics = %#v, want one", topics)

	topic := topics[0]
	assert.Equalf(t, "Public Form Endpoints", topic.Label,
		"label = %q, want Public Form Endpoints", topic.Label)
	assert.Equalf(t, `ds find "public form endpoints"`, topic.Try,
		"try = %q", topic.Try)
	assert.NotEqual(t, 0, topic.EvidenceCounts["source"])
	assert.NotEqual(t, 0, topic.EvidenceCounts["config"])

}

func TestRecentBoundaryQualityDemotesMaintenanceAheadOfSourceWork(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustMkdirAll(t, filepath.Join(repoRoot, "fastapi", "openapi"))
	mustMkdirAll(t, filepath.Join(repoRoot, "tests"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs", "en", "data"))
	mustMkdirAll(t, filepath.Join(repoRoot, ".github", "workflows"))
	mustWriteFile(t, filepath.Join(repoRoot, "fastapi", "openapi", "docs.py"), "def swagger_ui_html():\n    return 'oauth redirect'\n")
	mustWriteFile(t, filepath.Join(repoRoot, "fastapi", "openapi", "models.py"), "class OpenAPI: pass\n")
	mustWriteFile(t, filepath.Join(repoRoot, "tests", "test_custom_swagger_ui_redirect.py"), "def test_redirect():\n    assert True\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "fix: swagger oauth redirect behavior")
	mustWriteFile(t, filepath.Join(repoRoot, "README.md"), "Sponsor TutorCruncher\n")
	mustWriteFile(t, filepath.Join(repoRoot, "docs", "en", "data", "sponsors.yml"), "name: TutorCruncher\n")
	mustWriteFile(t, filepath.Join(repoRoot, "docs", "en", "data", "sponsors_badge.yml"), "name: TutorCruncher\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Update sponsors: add TutorCruncher")
	mustWriteFile(t, filepath.Join(repoRoot, ".github", "workflows", "latest-changes.yml"), "name: latest changes\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Fix latest-changes checkout target")

	cmd := NewRecentCmd()
	cmd.SetArgs([]string{"--path", repoRoot, "--no-refresh", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out mapRecentOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"recent json: %v\n%s", err, buf.String())
	}
	assert.GreaterOrEqualf(t, len(out.Topics), 2,
		"expected source and maintenance topics, got %#v", out.Topics)
	assert.NotEqual(t, "maintenance", out.Topics[0].TopicType)
	assert.Contains(t, out.Topics[0].Query, "swagger")

	assert.Truef(t, mapTestHasSignal(out.Topics[0].QualitySignals, "source_test_support"),
		"top topic missing source/test quality signal: %#v", out.Topics[0])

	foundDemotedMaintenance := false
	for _, topic := range out.Topics[1:] {
		if topic.TopicType == "maintenance" && mapTestHasSignal(topic.QualitySignals, "maintenance_demoted") {
			foundDemotedMaintenance = true
			break
		}
	}
	assert.Truef(t, foundDemotedMaintenance,
		"expected demoted maintenance topic, got %#v", out.Topics)

}

func TestRecentKeepsSourceBackedDependencyLanguage(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustMkdirAll(t, filepath.Join(repoRoot, "fastapi"))
	mustMkdirAll(t, filepath.Join(repoRoot, "tests"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs", "en", "data"))
	mustMkdirAll(t, filepath.Join(repoRoot, ".github", "workflows"))
	mustWriteFile(t, filepath.Join(repoRoot, "fastapi", "routing.py"), "def frontend():\n    return 'cookie auth'\n")
	mustWriteFile(t, filepath.Join(repoRoot, "tests", "test_frontend.py"), "def test_frontend_dependencies():\n    assert True\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Support dependencies in `app.frontend()`, e.g. for automatic cookie authentication for the frontend")
	mustWriteFile(t, filepath.Join(repoRoot, "README.md"), "Sponsor TutorCruncher\n")
	mustWriteFile(t, filepath.Join(repoRoot, "docs", "en", "data", "sponsors.yml"), "name: TutorCruncher\n")
	mustWriteFile(t, filepath.Join(repoRoot, "docs", "en", "data", "sponsors_badge.yml"), "name: TutorCruncher\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Update sponsors: add TutorCruncher")
	mustWriteFile(t, filepath.Join(repoRoot, ".github", "workflows", "latest-changes.yml"), "name: latest changes\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Fix latest-changes checkout target")

	cmd := NewRecentCmd()
	cmd.SetArgs([]string{"--path", repoRoot, "--no-refresh", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out mapRecentOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"recent json: %v\n%s", err, buf.String())
	}
	assert.GreaterOrEqualf(t, len(out.Topics), 2,
		"expected source and maintenance topics, got %#v", out.Topics)
	assert.NotEqual(t, "maintenance", out.Topics[0].TopicType)
	assert.Contains(t, out.Topics[0].Query, "frontend")

	assert.Truef(t, mapTestHasSignal(out.Topics[0].QualitySignals, "source_test_support"),
		"top topic missing source/test quality signal: %#v", out.Topics[0])

}

func TestRecentDemotesBulkDocsArchiveWhenSourceWorkExists(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustMkdirAll(t, filepath.Join(repoRoot, "demos", "fastapi-task-flow"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs", "raw-samples", "p06-fastapi"))
	mustWriteFile(t, filepath.Join(repoRoot, "demos", "fastapi-task-flow", "fastapi-task-flow.tape"), "Type \"ds recent\"\n")
	mustWriteFile(t, filepath.Join(repoRoot, "demos", "fastapi-task-flow", "fastapi-task-flow.windows.tape"), "Type \"ds recent\"\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Refresh FastAPI VHS flows")
	for i := 0; i < 32; i++ {
		name := fmt.Sprintf("sample-%02d.md", i)
		mustWriteFile(t, filepath.Join(repoRoot, "docs", "raw-samples", "p06-fastapi", name), "captured output\n")
	}
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Record P06 FastAPI task demo gate")

	cmd := NewRecentCmd()
	cmd.SetArgs([]string{"--path", repoRoot, "--no-refresh", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out mapRecentOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"recent json: %v\n%s", err, buf.String())
	}
	assert.GreaterOrEqualf(t, len(out.Topics), 2,
		"expected source and bulk-doc topics, got %#v", out.Topics)
	assert.NotEqual(t, "maintenance", out.Topics[0].TopicType)
	assert.Contains(t, out.Topics[0].Query, "fast api")

	foundDemotedBulkDocs := false
	for _, topic := range out.Topics[1:] {
		if topic.TopicType == "maintenance" && mapTestHasSignal(topic.QualitySignals, "maintenance:doc-archive") && mapTestHasSignal(topic.QualitySignals, "maintenance_demoted") {
			foundDemotedBulkDocs = true
			break
		}
	}
	assert.Truef(t, foundDemotedBulkDocs,
		"expected demoted bulk docs archive, got %#v", out.Topics)

}

func TestRecentMergesOverlappingMaintenanceTopics(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustMkdirAll(t, filepath.Join(repoRoot, "fastapi"))
	mustMkdirAll(t, filepath.Join(repoRoot, "tests"))
	mustMkdirAll(t, filepath.Join(repoRoot, "docs", "en", "data"))
	mustMkdirAll(t, filepath.Join(repoRoot, ".github", "workflows"))

	mustWriteFile(t, filepath.Join(repoRoot, "fastapi", "routing.py"), "def frontend():\n    return 'cookie auth'\n")
	mustWriteFile(t, filepath.Join(repoRoot, "tests", "test_frontend.py"), "def test_frontend_dependencies():\n    assert True\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Support dependencies in app.frontend(), e.g. for automatic cookie authentication for the frontend")

	mustWriteFile(t, filepath.Join(repoRoot, "README.md"), "Sponsor TutorCruncher\n")
	mustWriteFile(t, filepath.Join(repoRoot, "docs", "en", "data", "sponsors.yml"), "name: TutorCruncher\n")
	mustWriteFile(t, filepath.Join(repoRoot, "docs", "en", "data", "sponsors_badge.yml"), "name: TutorCruncher\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Update sponsors: add TutorCruncher")
	mustWriteFile(t, filepath.Join(repoRoot, "README.md"), "Sponsor RapidProxy\n")
	mustWriteFile(t, filepath.Join(repoRoot, "docs", "en", "data", "sponsors.yml"), "name: RapidProxy\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Update sponsors: remove RapidProxy")

	mustWriteFile(t, filepath.Join(repoRoot, ".github", "workflows", "latest-changes.yml"), "name: latest changes\nversion: 0.6.1\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Update latest-changes to 0.6.1")
	mustWriteFile(t, filepath.Join(repoRoot, ".github", "workflows", "latest-changes.yml"), "name: latest changes\ncheckout: main\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Fix latest-changes checkout target")

	out := mapTestRunRecentJSON(t, repoRoot)
	assert.GreaterOrEqualf(t, len(out.Topics), 3,
		"expected source, sponsors, and latest topics, got %#v", out.Topics)
	assert.NotEqual(t, "maintenance", out.Topics[0].TopicType)
	assert.Contains(t, out.Topics[0].Query, "frontend")

	sponsors := mapTestTopicsContaining(out, "sponsor")
	require.Lenf(t, sponsors, 1,
		"expected one merged sponsor topic, got %#v", sponsors)
	assert.Equal(t, 2, sponsors[0].CommitCount)
	assert.GreaterOrEqual(t, len(sponsors[0].RecentSignals), 2)
	assert.Equal(t, "maintenance", sponsors[0].TopicType)
	assert.True(t, mapTestHasSignal(sponsors[0].QualitySignals, "maintenance:sponsors"))

	assert.Equalf(t, "", sponsors[0].BoundaryLabel,
		"merged maintenance topic should omit misleading boundaries, got %#v", sponsors[0])

	latest := mapTestTopicsContaining(out, "latest")
	require.Lenf(t, latest, 1,
		"expected one merged latest-changes workflow topic, got %#v", latest)
	assert.Equal(t, 2, latest[0].CommitCount)
	assert.GreaterOrEqual(t, len(latest[0].RecentSignals), 2)
	assert.Equal(t, "maintenance", latest[0].TopicType)
	assert.True(t, mapTestHasSignal(latest[0].QualitySignals, "maintenance:release-docs"))

	var text bytes.Buffer
	writeMapRecentText(&text, out, false)
	assert.Containsf(t, text.String(), "Recent signals:",
		"merged text output should show plural recent signals:\n%s", text.String())

}

func TestRecentMergesOverlappingSourceTestTopics(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustMkdirAll(t, filepath.Join(repoRoot, "fastapi", "openapi"))
	mustMkdirAll(t, filepath.Join(repoRoot, "tests"))
	mustWriteFile(t, filepath.Join(repoRoot, "fastapi", "openapi", "docs.py"), "def swagger_ui_html():\n    return 'oauth redirect'\n")
	mustWriteFile(t, filepath.Join(repoRoot, "tests", "test_swagger_redirect.py"), "def test_redirect():\n    assert True\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Fix swagger oauth redirect behavior")
	mustWriteFile(t, filepath.Join(repoRoot, "fastapi", "openapi", "docs.py"), "def swagger_ui_html():\n    return 'hardened oauth redirect'\n")
	mustWriteFile(t, filepath.Join(repoRoot, "tests", "test_swagger_redirect.py"), "def test_redirect():\n    assert 'oauth'\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Harden swagger oauth redirect tests")

	out := mapTestRunRecentJSON(t, repoRoot)
	swagger := mapTestTopicsContaining(out, "swagger")
	require.Lenf(t, swagger, 1,
		"expected one merged swagger topic, got %#v", swagger)
	assert.NotEqual(t, "maintenance", swagger[0].TopicType)
	assert.True(t, mapTestHasSignal(swagger[0].QualitySignals, "source_test_support"))
	assert.Equal(t, 2, swagger[0].CommitCount)
	assert.GreaterOrEqual(t, len(swagger[0].RecentSignals), 2)

}

func TestRecentDoesNotMergeUnrelatedSourceTopicsByGenericWords(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustMkdirAll(t, filepath.Join(repoRoot, "fastapi", "openapi"))
	mustMkdirAll(t, filepath.Join(repoRoot, "fastapi"))
	mustMkdirAll(t, filepath.Join(repoRoot, "tests"))
	mustWriteFile(t, filepath.Join(repoRoot, "fastapi", "openapi", "docs.py"), "def swagger_ui_html():\n    return 'oauth redirect'\n")
	mustWriteFile(t, filepath.Join(repoRoot, "tests", "test_swagger_redirect.py"), "def test_redirect():\n    assert True\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Fix swagger oauth redirect behavior")
	mustWriteFile(t, filepath.Join(repoRoot, "fastapi", "routing.py"), "def frontend():\n    return 'cookie auth'\n")
	mustWriteFile(t, filepath.Join(repoRoot, "tests", "test_frontend_cookie.py"), "def test_frontend_cookie():\n    assert True\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Fix frontend cookie authentication")

	out := mapTestRunRecentJSON(t, repoRoot)
	swagger := mapTestTopicsContaining(out, "swagger")
	frontend := mapTestTopicsContaining(out, "frontend")
	require.Len(t, swagger, 1)
	require.Len(t, frontend, 1)
	assert.Equal(t, 1, swagger[0].CommitCount)
	assert.Equal(t, 1, frontend[0].CommitCount)

}

func TestRecentMaintenanceOnlyOutputStaysVisibleAndFramed(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustMkdirAll(t, filepath.Join(repoRoot, "docs", "en", "data"))
	mustWriteFile(t, filepath.Join(repoRoot, "README.md"), "Sponsor RapidProxy\n")
	mustWriteFile(t, filepath.Join(repoRoot, "docs", "en", "data", "sponsors.yml"), "name: RapidProxy\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Update sponsors: remove RapidProxy")

	cmd := NewRecentCmd()
	cmd.SetArgs([]string{"--path", repoRoot, "--no-refresh"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	text := buf.String()
	assert.Containsf(t, text, "Sponsors Rapid Proxy",
		"maintenance topic should remain visible:\n%s", text)
	assert.Containsf(t, text, "Topic type: maintenance",
		"maintenance topic should be framed:\n%s", text)

}

func TestRecentPreservesSpecificReadmeSpecUpdates(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustWriteFile(t, filepath.Join(repoRoot, "LICENSE"), "MIT\n")
	mustWriteFile(t, filepath.Join(repoRoot, "README.md"), "# Spec\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Initial commit")

	mustWriteFile(t, filepath.Join(repoRoot, "README.md"), "# Spec\n\nMany relation id optionality.\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "fix: README.md many relation id optionality")
	mustWriteFile(t, filepath.Join(repoRoot, "README.md"), "# Spec\n\nOne cardinality relation field optionality.\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "fix: update README.md one cardinality relation field optionality")
	mustWriteFile(t, filepath.Join(repoRoot, "README.md"), "# Spec\n\nSpecification identifiers use the new format.\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "feat: updated specification identifiers to new format")

	out := mapTestRunRecentJSON(t, repoRoot)
	assert.LessOrEqualf(t, len(mapTestTopicsContaining(out, "docs updates")), 0,
		"specific README spec changes should not collapse into generic docs updates: %#v", out.Topics)
	require.Lenf(t, mapTestTopicsContaining(out, "one cardinality relation"), 1,
		"missing one-cardinality spec topic: %#v", out.Topics)
	require.Lenf(t, mapTestTopicsContaining(out, "specification identifiers"), 1,
		"missing specification-identifiers topic: %#v", out.Topics)
	assert.False(t, len(out.Topics) > 0 && strings.Contains(strings.ToLower(out.Topics[0].Query), "commit license"))

}

func TestRecentKeepsVersionManifestTopicSpecific(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustWriteFile(t, filepath.Join(repoRoot, "kalo.json"), `{"version":"v0.0.1-dev-300126"}`+"\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Scoop update for kalo version v0.0.1-dev-300126")
	mustWriteFile(t, filepath.Join(repoRoot, "kalo.json"), `{"version":"v0.1.0"}`+"\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "Scoop update for kalo version v0.1.0")

	out := mapTestRunRecentJSON(t, repoRoot)
	require.Lenf(t, out.Topics, 1,
		"expected merged version manifest topic, got %#v", out.Topics)
	assert.NotContainsf(t, strings.ToLower(out.Topics[0].Query), "config updates",
		"version manifest updates should keep package/version specificity: %#v", out.Topics[0])
	assert.Contains(t, strings.ToLower(out.Topics[0].Query), "scoop")
	assert.Contains(t, strings.ToLower(out.Topics[0].Query), "kalo")

	assert.Equalf(t, 2, out.Topics[0].CommitCount,
		"merged version manifest topic should keep both commits: %#v", out.Topics[0])

}

func mapTestRunRecentJSON(t *testing.T, repoRoot string) mapRecentOutput {
	t.Helper()
	cmd := NewRecentCmd()
	cmd.SetArgs([]string{"--path", repoRoot, "--no-refresh", "--json"})
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	var out mapRecentOutput
	{
		err := json.Unmarshal(buf.Bytes(), &out)
		require.NoErrorf(t, err,
			"recent json: %v\n%s", err, buf.String())
	}

	return out
}

func mapTestTopicsContaining(out mapRecentOutput, term string) []mapRecentTopic {
	var topics []mapRecentTopic
	term = strings.ToLower(term)
	for _, topic := range out.Topics {
		haystack := strings.ToLower(topic.Query + " " + topic.Label)
		for _, path := range topic.KeyPaths {
			haystack += " " + strings.ToLower(path)
		}
		if strings.Contains(haystack, term) {
			topics = append(topics, topic)
		}
	}
	return topics
}

func mapTestHasSignal(signals []string, want string) bool {
	for _, signal := range signals {
		if signal == want {
			return true
		}
	}
	return false
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
	require.Len(t, out.Areas, 1)
	assert.Equal(t, "Partner Commission", out.Areas[0].Label)

	assert.Containsf(t, strings.Join(out.Caveats, "\n"), mapIndexRequiredCaveat,
		"missing index-required caveat: %#v", out.Caveats)

	indexed := buildFastMapFallbackOutputFromRecent(repoRoot, recent, true)
	assert.NotContainsf(t, strings.Join(indexed.Caveats, "\n"), mapIndexRequiredCaveat,
		"indexed fallback should not show index-required caveat: %#v", indexed.Caveats)

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
	require.NotEmptyf(t, areas,
		"expected boundary areas")
	assert.Equalf(t, "Webhooks", areas[0].Label,
		"top label = %q, want stable path boundary Webhooks; areas=%#v", areas[0].Label, areas)
	assert.NotContainsf(t, strings.ToLower(areas[0].Label), "harden",
		"recent workstream leaked into boundary label: %#v", areas[0])
	require.NotEmpty(t, areas[0].TraceReceipts)
	assert.Contains(t, areas[0].TraceReceipts[0].Subject, "harden webhook")

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
	require.NotNilf(t, workflow,
		"expected Workflows & Automation boundary, got %#v", areas)

	covers := strings.Join(workflow.Covers, "\n")
	assert.Containsf(t, covers,

		("Workflow Executor"), "workflow covers missing %q: %#v",

		("Workflow Executor"), workflow.Covers)
	assert.Containsf(t, covers,

		("Workflow Builder"), "workflow covers missing %q: %#v",

		("Workflow Builder"), workflow.Covers)
	assert.Containsf(t, covers,

		("Workflow Trigger"), "workflow covers missing %q: %#v",

		("Workflow Trigger"), workflow.Covers)

	assert.NotEqual(t, 0, workflow.EvidenceCounts["source"])
	assert.NotEqual(t, 0, workflow.EvidenceCounts["test"])
	assert.NotEqual(t, 0, workflow.EvidenceCounts["doc"])

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
	require.NotNilf(t, webhooks,
		"expected Webhooks area, got %#v", areas)
	assert.NotEqualf(t, 0, webhooks.EvidenceCounts["import"],
		"expected import structure evidence, got %#v", webhooks.EvidenceCounts)
	assert.NotEqualf(t, 0, webhooks.EvidenceCounts["test_import"],
		"expected test->source evidence, got %#v", webhooks.EvidenceCounts)
	assert.Containsf(t, mapAreaEvidenceText(webhooks.EvidenceCounts), "import structure",
		"evidence text missing import structure: %s", mapAreaEvidenceText(webhooks.EvidenceCounts))

}

func TestExtractMapBoundaryImports_ShellEntrypoint_ReturnsParserBackedReferences(t *testing.T) {
	body := "#!/usr/bin/env bash\nsource \"$TOOL_LIB/core.sh\"\n. ../shared.sh\n"

	got := extractMapBoundaryImports("bin/tool", body)

	require.Len(t, got, 2)
	assert.Equal(t, "$TOOL_LIB/core.sh", got[0])
	assert.Equal(t, "../shared.sh", got[1])
}

func TestAddMapBoundaryImportEdge_CrossBoundaryImport_AddsEntrypointEvidenceToTarget(t *testing.T) {
	candidates := map[string]*mapPathBoundaryCandidate{
		"core": {
			Key:             "core",
			PathSet:         map[string]bool{"lib/core.sh": true},
			EvidenceCounts:  map[string]int{"source": 1},
			EvidenceSources: map[string]bool{"path_boundary": true},
		},
		"tool": {
			Key:             "tool",
			PathSet:         map[string]bool{"bin/tool": true},
			EvidenceCounts:  map[string]int{"source": 1},
			EvidenceSources: map[string]bool{"path_boundary": true},
		},
	}
	pathKeys := map[string][]string{
		"bin/tool":    {"tool"},
		"lib/core.sh": {"core"},
	}

	addMapBoundaryImportEdge(candidates, pathKeys, "bin/tool", "lib/core.sh")

	coreCandidate := candidates["core"]
	require.NotNil(t, coreCandidate)
	assert.Equal(t, 1, coreCandidate.EvidenceCounts["import"])
	require.Len(t, coreCandidate.Artifacts, 1)
	assert.Equal(t, "bin/tool", coreCandidate.Artifacts[0].Path)
}

func TestAddMapBoundaryImportEdge_ShellModule_CreatesImportedModuleBoundary(t *testing.T) {
	candidates := map[string]*mapPathBoundaryCandidate{}
	pathKeys := map[string][]string{}

	addMapBoundaryImportEdge(candidates, pathKeys, "bin/tool", "lib/selection.sh")

	runtimeCandidate := candidates["shell-runtime"]
	require.NotNil(t, runtimeCandidate)
	assert.True(t, runtimeCandidate.EvidenceSources["imported_module"])
	assert.True(t, runtimeCandidate.ImportedModules["lib/selection.sh"])
	assert.Equal(t, 1, runtimeCandidate.EvidenceCounts["import"])
	require.Len(t, runtimeCandidate.Artifacts, 2)
	assert.Equal(t, "lib/selection.sh", runtimeCandidate.Artifacts[0].Path)
	assert.Equal(t, "bin/tool", runtimeCandidate.Artifacts[1].Path)
}

func TestApplyMapBoundaryImportedModuleTestCompanions_MatchingBehaviorTest_AddsTestEvidence(t *testing.T) {
	candidates := map[string]*mapPathBoundaryCandidate{
		"shell-runtime": {
			Key:             "shell-runtime",
			PathSet:         map[string]bool{"lib/formatter.bash": true},
			EvidenceCounts:  map[string]int{"source": 1, "import": 2},
			EvidenceSources: map[string]bool{"imported_module": true},
			ImportedModules: map[string]bool{"lib/formatter.bash": true},
		},
	}

	applyMapBoundaryImportedModuleTestCompanions([]string{"test/formatter.bats"}, candidates)

	runtimeCandidate := candidates["shell-runtime"]
	require.NotNil(t, runtimeCandidate)
	assert.Equal(t, 1, runtimeCandidate.EvidenceCounts["test"])
	require.Len(t, runtimeCandidate.Artifacts, 1)
	assert.Equal(t, "test/formatter.bats", runtimeCandidate.Artifacts[0].Path)
}

func TestMapBoundaryImportedModuleIdentity_RootShellModule_ReturnsShellRuntime(t *testing.T) {
	key, label := mapBoundaryImportedModuleIdentity("nvm.sh")

	assert.Equal(t, "shell-runtime", key)
	assert.Equal(t, "Shell Runtime", label)
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
	assert.Nil(t, findMapTestArea(areas, "Hooks"))
	assert.Nil(t, findMapTestArea(areas, "Dashboard"))
	assert.Nil(t, findMapTestArea(areas, "App Dub Co"))
	require.NotNilf(t, findMapTestArea(areas, "Webhooks"),
		"expected Webhooks to survive wrapper suppression, got %#v", areas)
	require.NotNilf(t, findMapTestArea(areas, "Affiliate / Partner Programs"),
		"expected partner parent to survive domain-shell suppression, got %#v", areas)

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
	require.NotNilf(t, partners,
		"expected Affiliate / Partner Programs, got %#v", areas)
	assert.Containsf(t, strings.Join(partners.Covers, "\n"), "Commissions",
		"partner parent missing concrete covers: %#v", partners.Covers)

	redirect := findMapTestArea(areas, "Short-Link Redirect & Click Capture")
	require.NotNilf(t, redirect,
		"expected Short-Link Redirect & Click Capture, got %#v", areas)
	assert.Containsf(t, redirect.Try, "click events",
		"conceptual try command should use concrete click substrate, got %q", redirect.Try)
	assert.Nil(t, findMapTestArea(areas, "Program"))
	assert.Nil(t, findMapTestArea(areas, "Programs"))

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
	require.NotNilf(t, workItems,
		"expected Work Items & Project Delivery, got %#v", areas)
	assert.Containsf(t, strings.Join(workItems.Covers, "\n"), "Issues",
		"work item parent missing issue cover: %#v", workItems.Covers)
	require.NotNilf(t, findMapTestArea(areas, "Planning: Cycles, Modules & Views"),
		"expected planning parent, got %#v", areas)
	require.NotNilf(t, findMapTestArea(areas, "Django API, Persistence & Async Workers"),
		"expected Django API parent, got %#v", areas)
	assert.Nilf(t, findMapTestArea(areas, "States"),
		"suppressed implementation-shaped States label leaked: %#v", areas)

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
	assert.Nilf(t, findMapTestArea(areas, "Django API, Persistence & Async Workers"),
		"Django parent should require Python/Django evidence, got %#v", areas)

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
	require.NotNilf(t, findMapTestArea(areas, "Metadata Engine & Data Model"),
		"expected metadata parent, got %#v", areas)
	require.NotNilf(t, findMapTestArea(areas, "CRM Record Experience"),
		"expected CRM record parent, got %#v", areas)
	require.NotNilf(t, findMapTestArea(areas, "Workflows & Automation"),
		"expected workflow parent, got %#v", areas)

	api := findMapTestArea(areas, "Public API Layer")
	require.NotNilf(t, api,
		"expected public API parent, got %#v", areas)
	assert.NotContains(t, api.Try, "public api layer")
	assert.True(t, (strings.Contains(api.Try, "graphql") || strings.Contains(api.Try, "rest")))

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
	require.NotNilf(t, metadata,
		"expected metadata parent, got %#v", areas)
	require.NotEmptyf(t, metadata.KeyPaths,
		"expected key paths")
	assert.NotContainsf(t, metadata.KeyPaths[0], "twenty-docs",
		"metadata parent should prefer implementation over docs, got %#v", metadata.KeyPaths)

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
	if findMapTestArea(areas,
		("Workspace Identity, Access & Billing")) !=
		nil {

		identityParents++
	}
	if findMapTestArea(areas,
		("Identity, Auth & Workspace Tenancy")) !=
		nil {
		identityParents++
	}
	if findMapTestArea(areas,
		("Identity, Auth & Access Control")) != nil {
		identityParents++
	}

	assert.Equalf(t, 1, identityParents,
		"expected exactly one identity parent, got %d in %#v", identityParents, areas)

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
	require.NotNilf(t, findMapTestArea(areas,
		("Project & Workspace Lifecycle")),
		"expected tool parent %q, got %#v",
		("Project & Workspace Lifecycle"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Dependency Resolution & Lockfile"),
	),
		"expected tool parent %q, got %#v", ("Dependency Resolution & Lockfile"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Package Installation & Virtual Environments")), "expected tool parent %q, got %#v", ("Package Installation & Virtual Environments"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Registry, Cache & Artifact Fetching")), "expected tool parent %q, got %#v", ("Registry, Cache & Artifact Fetching"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Tools & Ephemeral Environments")),

		"expected tool parent %q, got %#v", ("Tools & Ephemeral Environments"), areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Crates")), "tool repo shell/product label %q leaked into map: %#v",

		("Crates"), areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Scripts")), "tool repo shell/product label %q leaked into map: %#v",

		("Scripts"), areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Identity, Auth & Workspace Tenancy")),

		"tool repo shell/product label %q leaked into map: %#v",
		("Identity, Auth & Workspace Tenancy"), areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Work Items & Project Delivery")), "tool repo shell/product label %q leaked into map: %#v",

		("Work Items & Project Delivery"), areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Built By Uv")),
		"tool repo shell/product label %q leaked into map: %#v",

		("Built By Uv"), areas,
	)
	assert.Nilf(t,
		findMapTestArea(areas, ("Github")), "tool repo shell/product label %q leaked into map: %#v",

		("Github"), areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Instance Administration & Licensing")),

		"tool repo shell/product label %q leaked into map: %#v",
		("Instance Administration & Licensing"), areas)

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
	require.NotNilf(t, findMapTestArea(areas,
		("Accounts & Net-Worth Dashboard")),

		"expected finance/product parent %q, got %#v", ("Accounts & Net-Worth Dashboard"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Transaction Ledger, Categorization & Cashflow")), "expected finance/product parent %q, got %#v",
		("Transaction Ledger, Categorization & Cashflow"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Budgeting")),
		"expected finance/product parent %q, got %#v",

		("Budgeting"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Investments, Holdings & Securities")), "expected finance/product parent %q, got %#v", ("Investments, Holdings & Securities"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Bank Connectivity & Plaid Sync")),

		"expected finance/product parent %q, got %#v", ("Bank Connectivity & Plaid Sync"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("CSV & Manual Data Import")), "expected finance/product parent %q, got %#v",

		("CSV & Manual Data Import"), areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Controllers")),
		"rails shell label %q leaked into map: %#v",

		("Controllers"), areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("DB/Migrate")), "rails shell label %q leaked into map: %#v",

		("DB/Migrate"), areas)

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
	require.NotNilf(t, findMapTestArea(areas,
		("Framework Runtime & Module Platform")), "expected platform/commerce parent %q, got %#v",
		("Framework Runtime & Module Platform"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Product Catalog, Pricing & Inventory")), "expected platform/commerce parent %q, got %#v",
		("Product Catalog, Pricing & Inventory"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Cart, Checkout & Promotions")), "expected platform/commerce parent %q, got %#v",

		("Cart, Checkout & Promotions"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Orders, Fulfillment & Post-Purchase")), "expected platform/commerce parent %q, got %#v",
		("Orders, Fulfillment & Post-Purchase"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Payments, Tax & Monetary Configuration")), "expected platform/commerce parent %q, got %#v",
		("Payments, Tax & Monetary Configuration"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Provider Adapters & Pluggable Infrastructure")), "expected platform/commerce parent %q, got %#v",
		("Provider Adapters & Pluggable Infrastructure"), areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Www")), "platform shell label %q leaked into map: %#v",

		("Www"), areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Design System")),
		"platform shell label %q leaked into map: %#v",

		("Design System"), areas)

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
	require.NotNilf(t, signing,
		"expected document signing parent, got %#v", areas)

	covers := strings.Join(signing.Covers, "\n")
	assert.Containsf(t, covers,

		("Documents"), "document signing parent missing cover %q: %#v",

		("Documents"), signing.Covers)
	assert.Containsf(t, covers,

		("Recipients"), "document signing parent missing cover %q: %#v",

		("Recipients"), signing.Covers)
	assert.Containsf(t, covers,

		("Field Signing"), "document signing parent missing cover %q: %#v",

		("Field Signing"), signing.Covers)
	assert.Nilf(t,
		findMapTestArea(areas, ("Remix")), "framework/package shell %q leaked into top-level map: %#v",

		("Remix"), areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Trpc")), "framework/package shell %q leaked into top-level map: %#v",

		("Trpc"), areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Server Only")),
		"framework/package shell %q leaked into top-level map: %#v",

		("Server Only"),
		areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Primitives")), "framework/package shell %q leaked into top-level map: %#v",

		("Primitives"), areas,
	)
	assert.Nilf(t,
		findMapTestArea(areas, ("Universal")), "framework/package shell %q leaked into top-level map: %#v",

		("Universal"), areas,
	)

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
	require.NotNilf(t, findMapTestArea(areas,
		("Content/Data Model")),
		"expected platform concept parent %q, got %#v",

		("Content/Data Model"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Extension Surfaces")),
		"expected platform concept parent %q, got %#v",

		("Extension Surfaces"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Flows & Automation")),
		"expected platform concept parent %q, got %#v",

		("Flows & Automation"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Public API Layer")), "expected platform concept parent %q, got %#v",

		("Public API Layer"),
		areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Composables")),
		"implementation/test shell %q leaked into top-level map: %#v",

		("Composables"), areas)
	assert.Nilf(t,
		findMapTestArea(areas, ("Blackbox")), "implementation/test shell %q leaked into top-level map: %#v",

		("Blackbox"), areas,
	)

	apiParents := 0
	if findMapTestArea(areas,
		("Public API Layer")) != nil {
		apiParents++
	}
	if findMapTestArea(areas,
		("Public HTTP API & Developer Platform")) !=
		nil {

		apiParents++
	}

	assert.Equalf(t, 1, apiParents,
		"expected exactly one API parent, got %d in %#v", apiParents, areas)

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
	assert.Nilf(t, findMapTestArea(areas, "Javascript"),
		"javascript shell leaked into first-screen map: %#v", areas)
	require.NotNilf(t, findMapTestArea(areas,
		("External HTTP API v1")),
		"expected product/platform area %q, got %#v",

		("External HTTP API v1"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Connected Accounts, Email, Calendar & Timeline")), "expected product/platform area %q, got %#v",
		("Connected Accounts, Email, Calendar & Timeline"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("AI Agents, Chat & Skills")), "expected product/platform area %q, got %#v",

		("AI Agents, Chat & Skills"), areas)
	require.NotNilf(t, findMapTestArea(areas,
		("Identity, Auth & Access Control")),

		"expected product/platform area %q, got %#v", ("Identity, Auth & Access Control"), areas)

}

func TestMapTryCommandDropsShellLabelsAndUsesSpecificCovers(t *testing.T) {
	{
		got := mapTryCommand("Locales", []string{"Admin Console"}, nil, mapHighConfidence, nil)
		assert.Equalf(t, `ds find "admin console"`, got,
			"locales try = %q", got)
	}
	{

		got := mapTryCommand("Javascript", []string{"Accordion"}, nil, mapHighConfidence, nil)
		assert.Equalf(t, `ds find "accordion"`, got,
			"javascript try = %q", got)
	}
	{

		got := mapTryCommand("Files, Assets & Storage", []string{"Upload"}, nil, mapHighConfidence, []string{"web/src/components/MemoEditor/hooks/useFileUpload.ts"})
		assert.Equalf(t, `ds find "upload"`, got,
			"broad storage try should prefer the specific cover, got %q", got)
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
	assert.Equalf(t, `ds find "submission redaction"`, got,
		"parent label try = %q", got)

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
	assert.Equalf(t, `ds find "map handoff query quality"`, got,
		"trace task handoff = %q", got)

}

func TestMapTryCommandConsidersThirdTraceReceiptForStablePathSupportedQuery(t *testing.T) {
	got := mapTryCommandForRole(
		"Flows & Automation",
		nil,
		[]mapTraceReceipt{
			{SHA: "21b6442", Subject: "docs: add launch demo animations"},
			{SHA: "be86e4b", Subject: "Add launch website route skeleton"},
			{SHA: "663230d", Subject: "Implement website IA pivot homepage"},
		},
		mapHighConfidence,
		[]string{
			"public/demo/fastapi-map-find-task-flow-v1-1.mp4",
			"public/demo/fastapi-task-flow-v1-1.mp4",
			"src/app/changelog/page.tsx",
			"src/app/page.tsx",
			"src/components/investigation-comparison.tsx",
			"src/components/quickstart-tabs.tsx",
			"devspecs/tasks/website-ia-pivot-experiment/checkpoints/20260702-103009-validated.json",
			"devspecs/tasks/website-ia-pivot-experiment/checkpoints/20260702-122153-validated.json",
			"devspecs/tasks/website-ia-pivot-experiment/task.json",
			"devspecs/tasks/launch-content-readiness/B04-refresh-core-vhs-demos-for-task-flow-map-onboard-plan.md",
			"devspecs/tasks/launch-content-readiness/B04-refresh-core-vhs-demos-for-task-flow-map-onboard-result.md",
		},
		mapBoundaryRoleHandoffUnsafe,
	)
	assert.Equalf(t, `ds find "website ia pivot homepage"`, got,
		"third path-supported trace task should stabilize handoff query, got %q", got)

}

func TestMapTryCommandPrefersPathForBroadRoleOverUnsupportedTraceDetails(t *testing.T) {
	got := mapTryCommandForRole(
		"Flows & Automation",
		nil,
		[]mapTraceReceipt{{
			SHA:     "183cbeb",
			Subject: "Fix abacus flow: use execute_sql, disable task caching, skip column validation",
		}},
		mapHighConfidence,
		[]string{
			"flows/__init__.py",
			"flows/abacus/__init__.py",
			"flows/abacus/absences.py",
			"flows/abacus/flow.py",
			"flows/abacus/invoice_flow.py",
		},
		mapBoundaryRoleHandoffUnsafe,
	)
	assert.Equalf(t, `ds find "abacus absences flow"`, got,
		"broad boundary should prefer stable path query over unsupported trace details, got %q", got)

}

func TestMapTryCommandPrefersDomainTraceOverTechnicalMigrationForBroadRole(t *testing.T) {
	got := mapTryCommandForRole(
		"Flows & Automation",
		nil,
		[]mapTraceReceipt{
			{SHA: "1111111", Subject: "Fix abacus flow: use execute_sql, disable task caching, skip column validation"},
			{SHA: "abc1234", Subject: "Migrate all flows from SqlAlchemyConnector to ena_functions API"},
			{SHA: "def5678", Subject: "Add new flows and enhance document handling for invoices"},
		},
		mapHighConfidence,
		[]string{
			"flows/__init__.py",
			"flows/abacus/invoice_flow.py",
			"flows/solarpotential-wall/buildings.py",
		},
		mapBoundaryRoleHandoffUnsafe,
	)
	assert.Equalf(t, `ds find "flows enhance document invoices"`, got,
		"domain workflow trace should beat technical migration trace, got %q", got)

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
	assert.Equalf(t, `ds find "submission redaction"`, got,
		"specific cover should beat unrelated trace task, got %q", got)

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
	assert.NotContainsf(t, got, "raise aggregate coverage",
		"test prefix trace task leaked into handoff: %q", got)

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
	assert.NotEqual(t, "", got,
		"expected a handoff query")
	assert.NotContains(t, got, "dockerfile")
	assert.NotContains(t, got, "dockerignore")
	assert.NotContains(t, got, "version")
	assert.False(t, !strings.Contains(got, "operator") && !strings.Contains(got, "charts") && !strings.Contains(got, "crds"))

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
	assert.NotEqual(t, "", got,
		"expected a handoff query")
	assert.NotContains(t, got, "bug000")
	assert.NotContains(t, got, "bug002")
	assert.False(t, !strings.Contains(got, "arm64") && !strings.Contains(got, "fixedbugs"))

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
	assert.NotEmpty(t, api)
	assert.NotContains(t, api, "public api layer")
	assert.Contains(t, api, "subscriptions")

	platform := mapTryCommandForRole(
		"Platform",
		[]string{"Advisor Reports"},
		nil,
		mapHighConfidence,
		[]string{"src/Appwrite/Platform/Modules/Advisor/Reports/Report.php"},
		mapBoundaryRoleGenericParent,
	)
	assert.NotEmpty(t, platform)
	assert.NotContains(t, platform, `"platform"`)
	assert.Contains(t, platform, "advisor")

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
	assert.NotEmpty(t, got)
	assert.Contains(t, got, "ai prompt guard")
	assert.NotContains(t, got, "plugins ai")

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
	assert.Equalf(t, "", got,
		"unpackable try = %q, want suppressed", got)
	require.NotNil(t, diag)
	assert.True(t, diag.TrySuppressed)

	assert.Equalf(t, "suppressed_no_indexed_support", diag.Decision,
		"decision = %q", diag.Decision)
	require.Len(t, diag.MissingKeyExtensions, 1)
	assert.Equal(t, ".lua", diag.MissingKeyExtensions[0])

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
	assert.NotEqual(t, "", got,
		"supported boundary handoff was suppressed")
	require.NotNil(t, diag)
	assert.Equal(t, "supported", diag.Decision)

	assert.Equalf(t, 2, diag.IndexedKeyPathCount,
		"indexed key paths = %d, want 2", diag.IndexedKeyPathCount)

}

func TestMapTryCommandKeepsSpecificCoverForIndexedHandoffUnsafeArea(t *testing.T) {
	idx := newMapTestPackabilityIndex(
		"public/images/avatars/phase-1.mp4",
		"src/app/assets/styles/components/DrumDesigner.module.css",
	)
	got, diag := mapTryCommandForRoleWithPackability(
		"Files, Assets & Storage",
		[]string{"Upload"},
		nil,
		mapMediumConfidence,
		[]string{
			"public/images/avatars/phase-1.mp4",
			"src/app/assets/styles/components/DrumDesigner.module.css",
		},
		mapBoundaryRoleHandoffUnsafe,
		idx,
	)
	assert.Equalf(t, `ds find "upload"`, got,
		"indexed handoff-unsafe cover try = %q", got)
	require.NotNil(t, diag)
	assert.False(t, diag.TrySuppressed)

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
	{
		got := mapAreaPackCommands(area)
		require.Lenf(t, got, 0,
			"suppressed area pack commands = %#v", got)
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
	{
		got := classifyMapBoundaryRole(platform, "Platform", mapTypePlatform, false, []string{"Advisor Reports"}, "appwrite", mapRepoShapePlatform)
		assert.Equalf(t, mapBoundaryRoleGenericParent, got,
			"platform role = %q", got)
	}

	playground := &mapAreaInternal{Key: "playground", Label: "Playground", EvidenceCounts: map[string]int{"source": 4}}
	{
		got := classifyMapBoundaryRole(playground, "Playground", mapTypeTooling, false, nil, "vite", mapRepoShapeTool)
		assert.Equalf(t, mapBoundaryRoleFixtureOrTestbed, got,
			"playground role = %q", got)
	}

	namespace := &mapAreaInternal{Key: "gitea-repositories-meta", Label: "Gitea Repositories Meta", EvidenceCounts: map[string]int{"source": 4}}
	{
		got := classifyMapBoundaryRole(namespace, "Gitea Repositories Meta", mapTypeDomainFeature, false, nil, "gitea", mapRepoShapePlatform)
		assert.Equalf(t, mapBoundaryRoleRepoNamespace, got,
			"repo namespace role = %q", got)
	}

	plugins := &mapAreaInternal{
		Key:            "plugins",
		Label:          "Plugins",
		EvidenceCounts: map[string]int{"source": 4},
		Artifacts: []mapArtifact{{
			Path: "tests/fixtures/plugins/ai-prompt-guard/index.ts",
		}},
	}
	{
		got := classifyMapBoundaryRole(plugins, "Plugins", mapTypePlatform, false, []string{"Ai Prompt Guard"}, "kong", mapRepoShapePlatform)
		assert.Equalf(t, mapBoundaryRoleExtensionEcosystem, got,
			"plugin role should not become fixture from support path, got %q", got)
	}

}

func TestMapBoundaryPathEligibleSkipsEmbeddedGitFixtures(t *testing.T) {
	path := "tests/gitea-repositories-meta/limited_org/private_repo_on_limited_org.git/objects/74/8bf557dfc9c6457998b5118a6c8b2129f56c30"
	assert.False(t, mapBoundaryPathEligible(path))

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
	require.NotNilf(t, dataModel,
		"expected Content/Data Model, got %#v", areas)
	require.NotEmptyf(t, dataModel.KeyPaths,
		"expected key paths")
	assert.False(t, strings.HasPrefix(dataModel.KeyPaths[0], "test/"))
	assert.False(t, strings.HasPrefix(dataModel.KeyPaths[0], "examples/"))

}

func TestMapAreaMatchPrefersPluralLabelOverPathOnlyMatch(t *testing.T) {
	areas := []mapArea{
		{Label: "Cron", KeyPaths: []string{"apps/web/app/api/cron/notify-partners/route.ts"}, Diagnostics: mapAreaDiagnostics{TraceTerms: []string{"partner"}}},
		{Label: "Partners", KeyPaths: []string{"apps/web/app/partners/fraud/page.tsx"}},
	}
	matches := matchMapAreas(areas, "partner")
	require.NotEmptyf(t, matches,
		"expected matches")
	assert.Equalf(t, "Partners", matches[0].Area.Label,
		"top match = %q, want Partners; matches=%#v", matches[0].Area.Label, matches)

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
	require.Lenf(t, topics, 1,
		"filtered topics = %#v, want one", topics)
	assert.Containsf(t, topics[0].Query, "bounce",
		"filtered topic = %#v, want bounce", topics[0])

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
	assert.Containsf(t, text,
		("Recently active topics"), "recent output missing %q:\n%s",

		("Recently active topics"), text)
	assert.Containsf(t, text,
		("Expedition Enemy Pressure"),
		"recent output missing %q:\n%s",

		("Expedition Enemy Pressure"), text)
	assert.Containsf(t, text,
		("Evidence: 1 commit, 2 files, source"),
		"recent output missing %q:\n%s",

		("Evidence: 1 commit, 2 files, source"), text)
	assert.Containsf(t, text,
		("Recent signal: df68f82 Add expedition enemy pressure phases A-D for Killer Slice 001."), "recent output missing %q:\n%s",
		("Recent signal: df68f82 Add expedition enemy pressure phases A-D for Killer Slice 001."), text)
	assert.Containsf(t, text,
		(`Try: ds find "expedition enemy pressure"`), "recent output missing %q:\n%s",

		(`Try: ds find "expedition enemy pressure"`), text)
	assert.NotContainsf(t,
		text,
		("Open tasks"), "recent output made task-status claim %q:\n%s",

		("Open tasks"), text)
	assert.NotContainsf(t,
		text,
		("In progress"), "recent output made task-status claim %q:\n%s",

		("In progress"), text)
	assert.NotContainsf(t,
		text,
		("Done"),
		"recent output made task-status claim %q:\n%s",

		("Done"), text)
	assert.NotContainsf(t,
		text,
		("Stale"),
		"recent output made task-status claim %q:\n%s",

		("Stale"), text)
	assert.NotContainsf(t,
		text,
		("Resume work"), "recent output made task-status claim %q:\n%s",

		("Resume work"), text)

}

func TestRecentCommandRendersHumanTopics(t *testing.T) {
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
	errBuf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.Contains(t, errBuf.String(), "Recent progress: analyzing recent repository activity")
	assert.Contains(t, errBuf.String(), "Recent progress: complete")

	text := buf.String()
	assert.Containsf(t, text,
		("Recently active topics"), "recent output missing %q:\n%s",

		("Recently active topics"), text)
	assert.Containsf(t, text,
		("Credentials Rotation"), "recent output missing %q:\n%s",

		("Credentials Rotation"), text)
	assert.Containsf(t, text,
		("Try: ds find"), "recent output missing %q:\n%s",

		("Try: ds find"), text)
}

func TestRecentCommandFiltersJSONTopicsByQuery(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustMkdirAll(t, filepath.Join(repoRoot, "internal", "security"))
	mustWriteFile(t, filepath.Join(repoRoot, "internal", "security", "credentials.go"), "package security\n")
	mustWriteFile(t, filepath.Join(repoRoot, "internal", "security", "credentials_test.go"), "package security\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "feat: credentials rotation context")

	jsonCmd := NewRecentCmd()
	jsonCmd.SetArgs([]string{"credentials", "--path", repoRoot, "--no-refresh", "--json"})
	jsonBuf := &bytes.Buffer{}
	jsonErr := &bytes.Buffer{}
	jsonCmd.SetOut(jsonBuf)
	jsonCmd.SetErr(jsonErr)
	{
		err := jsonCmd.Execute()
		require.NoError(t, err)
	}
	assert.Equalf(t, 0, jsonErr.Len(),
		"recent --json should suppress progress stderr, got: %s", jsonErr.String())

	var out mapRecentOutput
	{
		err := json.Unmarshal(jsonBuf.Bytes(), &out)
		require.NoErrorf(t, err,
			"recent json: %v\n%s", err, jsonBuf.String())
	}
	assert.Equal(t, mapRecentSchemaVersion, out.Schema)
	require.Len(t, out.Topics, 1)

	assert.Containsf(t, out.Topics[0].Query, "credentials",
		"recent topic query = %q", out.Topics[0].Query)
}

func TestRecentVerboseShowsDetailedProgress(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustMkdirAll(t, filepath.Join(repoRoot, "internal", "security"))
	mustWriteFile(t, filepath.Join(repoRoot, "internal", "security", "credentials.go"), "package security\n")
	mustWriteFile(t, filepath.Join(repoRoot, "internal", "security", "credentials_test.go"), "package security\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "feat: credentials rotation context")

	cmd := NewRecentCmd()
	cmd.SetArgs([]string{"--path", repoRoot, "--no-refresh", "--verbose"})
	buf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(errBuf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	out := errBuf.String()
	assert.Containsf(t, out,
		("Recent progress: analyzing recent repository activity"), "recent --verbose progress missing %q:\n%s", ("Recent progress: analyzing recent repository activity"),
		out)
	assert.Containsf(t, out,
		("Recent progress: checking local git history"), "recent --verbose progress missing %q:\n%s",

		("Recent progress: checking local git history"), out)
	assert.Containsf(t, out,
		("Recent progress: reading recent commits and path boundaries"), "recent --verbose progress missing %q:\n%s",
		("Recent progress: reading recent commits and path boundaries"), out)
	assert.Containsf(t, out,
		("Recent progress: analyzed 2 commit(s), matched 1 topic(s)"), "recent --verbose progress missing %q:\n%s",
		("Recent progress: analyzed 2 commit(s), matched 1 topic(s)"), out)
	assert.Containsf(t, out,
		("Recent progress: complete"),
		"recent --verbose progress missing %q:\n%s",

		("Recent progress: complete"), out,
	)

}

func TestRecentQuietFastFirstKeepsStdoutResultOnly(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	mustMkdirAll(t, filepath.Join(repoRoot, "internal", "security"))
	mustWriteFile(t, filepath.Join(repoRoot, "internal", "security", "credentials.go"), "package security\n")
	mapTestGit(t, repoRoot, "add", ".")
	mapTestGit(t, repoRoot, "commit", "-m", "feat: credentials rotation context")

	cmd := NewRecentCmd()
	cmd.SetArgs([]string{"credentials", "--path", repoRoot, "--json", "--quiet"})
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.Equalf(t, 0, errBuf.Len(),
		"recent --quiet should suppress auto-scan stderr, got: %s", errBuf.String())

	var out mapRecentOutput
	{
		err := json.Unmarshal(outBuf.Bytes(), &out)
		require.NoErrorf(t, err,
			"recent --quiet stdout should remain valid JSON: %v\nstdout=%s\nstderr=%s", err, outBuf.String(), errBuf.String())
	}
	assert.Equal(t, mapRecentSchemaVersion, out.Schema)
	require.Len(t, out.Topics, 1)

	db, err := openDB()
	require.NoError(t, err)

	defer db.Close()
	count, err := db.CountArtifacts(store.FilterParams{RepoRoot: canonicalRepoRoot(repoRoot)})
	require.NoError(t, err)
	assert.Equalf(t, 0, count,
		"recent fast-first should not block on auto-scan, indexed artifact count = %d", count)

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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}

	text := buf.String()
	assert.Contains(t, text, "Recently active topics")
	assert.Contains(t, text, "Refund Retry")

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
	{
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err,
			"git %v failed: %v\n%s", args, err, out)
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
	assert.Equal(t, 0, skipped)
	require.Len(t, topics, 1)

	area := mapAreaFromRecentTopic(topics[0])
	assert.Equalf(t, "YAML Format Language", area.Label,
		"label = %q", area.Label)
	assert.Equalf(t, `ds find "yaml format language"`, area.Try,
		"try = %q", area.Try)
	assert.NotEqual(t, 0, area.EvidenceCounts["source"])
	assert.NotEqual(t, 0, area.EvidenceCounts["test"])

	assert.NotEqualf(t, "", area.AreaType,
		"expected area type: %#v", area)

}

func TestBuildCachedMapResultUsesStoredWorkstreamEdges(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	now := "2026-06-01T00:00:00Z"
	repoRoot := filepath.Join(t.TempDir(), "repo")
	{
		_, err := db.Exec("INSERT INTO repos (id, root_path, created_at, updated_at) VALUES ('repo_cached', ?, ?, ?)", repoRoot, now, now)
		require.NoError(t, err)
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
	require.NoError(t, err)
	require.True(t, ok,
		"expected cached map result")

	out := buildMapOutput(repoRoot, result, mapOptions{MaxAreas: 4})
	require.NotEmptyf(t, out.Areas,
		"expected cached areas: %#v", out)
	{

		got := out.Areas[0].Label
		assert.Equalf(t, "Game", got,
			"cached map label = %q, want Game; areas=%#v", got, out.Areas)
	}
	assert.Containsf(t, strings.Join(out.Areas[0].Covers, "\n"), "Rts Camera Mode",
		"cached map covers missing Rts Camera Mode: %#v", out.Areas[0].Covers)
	assert.Containsf(t, strings.Join(out.Areas[0].KeyPaths, "\n"), "cameraRTS.ts",
		"cached area missing source path: %#v", out.Areas[0].KeyPaths)

}

func TestMapTryCommandAvoidsUnsupportedCommitVerb(t *testing.T) {
	query := mapTryCommand("Release", []string{"Publish Npm"}, []mapTraceReceipt{{
		SHA:     "abc1234",
		Subject: "Replace `main` branch in changelog link with tags (#19054)",
	}}, mapMediumConfidence, nil)
	assert.Equalf(t, `ds find "release publish npm"`, query,
		"query = %q, want release publish npm", query)

	commands := mapAreaPackCommands(mapArea{
		Label:         "Release",
		Confidence:    mapMediumConfidence,
		Covers:        []string{"Publish Npm"},
		TraceReceipts: []mapTraceReceipt{{Subject: "Replace `main` branch in changelog link with tags (#19054)"}},
		Try:           query,
	})
	for _, cmd := range commands {
		assert.NotContainsf(t, cmd, "release replace",
			"unsupported commit verb leaked into commands: %#v", commands)

	}
}

func TestMapOutputCacheRoundTripsFreshMap(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("DEVSPECS_HOME", home)
	db, err := store.Open(filepath.Join(home, "devspecs.db"))
	require.NoError(t, err)

	defer db.Close()
	now := "2026-06-01T00:00:00Z"
	repoRoot := filepath.Join(t.TempDir(), "repo")
	{
		err := os.MkdirAll(repoRoot, 0o755)
		require.NoError(t, err)
	}
	{

		_, err := db.Exec("INSERT INTO repos (id, root_path, last_scan_at, created_at, updated_at) VALUES ('repo_cache_out', ?, ?, ?, ?)", repoRoot, now, now, now)
		require.NoError(t, err)
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
	{
		err := saveMapOutputCache(repoRoot, mapDefaultMaxAreas, out)
		require.NoError(t, err)
	}

	got, ok, err := loadMapOutputCache(t.Context(), repoRoot, mapDefaultMaxAreas)
	require.NoError(t, err)
	require.True(t, ok,
		"expected map output cache hit")
	assert.Equalf(t, out.Areas[0].Try, got.Areas[0].Try,
		"cached try = %q, want %q", got.Areas[0].Try, out.Areas[0].Try)

}

func TestMapUsesFreshOutputCacheBeforeAutoScan(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	writeMapTestFile(t, repoRoot, "app/auth/credentials.go", "package auth\n\nfunc RotateCredentials() {}\n")
	runGitForFindPack(t, repoRoot, "add", ".")
	runGitForFindPack(t, repoRoot, "commit", "-m", "add credentials rotation context")
	insertFreshMapCacheRepo(t, repoRoot)

	cached := mapOutput{
		Schema: mapSchemaVersion,
		Repo: mapRepo{
			Name:       filepath.Base(filepath.Clean(repoRoot)),
			Path:       canonicalRepoRoot(repoRoot),
			Confidence: mapHighConfidence,
		},
		Areas: []mapArea{{
			Label: "Cached Credentials Boundary",
			Try:   `ds find "cached credentials boundary"`,
		}},
	}
	{
		err := saveMapOutputCache(canonicalRepoRoot(repoRoot), mapDefaultMaxAreas, cached)
		require.NoError(t, err)
	}

	cmd := NewMapCmd()
	cmd.SetArgs([]string{"--json", "--path", repoRoot})
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.NotContainsf(t, errBuf.String(), "Index updated",
		"fresh map output cache should skip auto-scan, stderr: %s", errBuf.String())

	var out mapOutput
	{
		err := json.Unmarshal(outBuf.Bytes(), &out)
		require.NoErrorf(t, err,
			"map cache stdout should remain JSON: %v\n%s", err, outBuf.String())
	}
	assert.Truef(t, mapOutputHasAreaLabel(out, "Cached Credentials Boundary"),
		"expected cached map output, got %#v", out.Areas)

}

func TestMapOutputCacheMissScansWhenGitHeadMoved(t *testing.T) {
	repoRoot := setupGitRepo(t)
	t.Setenv("DEVSPECS_HOME", t.TempDir())
	writeMapTestFile(t, repoRoot, "app/auth/credentials.go", "package auth\n\nfunc RotateCredentials() {}\n")
	runGitForFindPack(t, repoRoot, "add", ".")
	runGitForFindPack(t, repoRoot, "commit", "-m", "add credentials rotation context")
	insertFreshMapCacheRepo(t, repoRoot)

	cached := mapOutput{
		Schema: mapSchemaVersion,
		Repo: mapRepo{
			Name:       filepath.Base(filepath.Clean(repoRoot)),
			Path:       canonicalRepoRoot(repoRoot),
			Confidence: mapHighConfidence,
		},
		Areas: []mapArea{{
			Label: "Stale Cached Boundary",
			Try:   `ds find "stale cached boundary"`,
		}},
	}
	{
		err := saveMapOutputCache(canonicalRepoRoot(repoRoot), mapDefaultMaxAreas, cached)
		require.NoError(t, err)
	}

	writeMapTestFile(t, repoRoot, "app/billing/invoices.go", "package billing\n\nfunc SendInvoices() {}\n")
	runGitForFindPack(t, repoRoot, "add", ".")
	runGitForFindPack(t, repoRoot, "commit", "-m", "add invoice boundary")

	cmd := NewMapCmd()
	cmd.SetArgs([]string{"--json", "--path", repoRoot})
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.Equalf(t, 0, errBuf.Len(),
		"map --json stale cache rebuild should suppress auto-scan stderr, got: %s", errBuf.String())

	var out mapOutput
	{
		err := json.Unmarshal(outBuf.Bytes(), &out)
		require.NoErrorf(t, err,
			"map stale cache stdout should remain JSON: %v\n%s", err, outBuf.String())
	}
	assert.False(t, mapOutputHasAreaLabel(out, "Stale Cached Boundary"))

}

func TestMapBoundaryRawAnchorsAreStable(t *testing.T) {
	candidate := &mapPathBoundaryCandidate{
		Key:   "operations",
		Label: "Operations",
		BoundaryPaths: map[string]bool{
			"docs/operations":                           true,
			"apps/api/internal/app/operations":          true,
			".devspecs/tasks/v0-foundation/checkpoints": true,
		},
	}
	got := mapBoundaryRawAnchors(candidate)
	want := []string{
		"Operations",
		"operations",
		".devspecs/tasks/v0-foundation/checkpoints",
		"apps/api/internal/app/operations",
		"docs/operations",
	}
	assert.Equalf(t, strings.Join(want, "\n"), strings.Join(got, "\n"),
		"raw anchors not stable:\n got=%#v\nwant=%#v", got, want)

}

func TestMapBoundaryRawAnchorsPreferEvidenceOrder(t *testing.T) {
	candidate := &mapPathBoundaryCandidate{
		Key:               "flows-automation",
		Label:             "Flows & Automation",
		BoundaryPaths:     map[string]bool{},
		BoundaryPathOrder: nil,
	}
	appendMapBoundaryPathCandidate(candidate, "docs/architecture")
	appendMapBoundaryPathCandidate(candidate, ".devspecs/tasks/v0-founder-workflows/checkpoints")
	appendMapBoundaryPathCandidate(candidate, "apps/api/internal/ports")
	appendMapBoundaryPathCandidate(candidate, ".devspecs/tasks/v0-foundation")
	appendMapBoundaryPathCandidate(candidate, ".devspecs/tasks/v0-foundation/checkpoints")

	got := mapBoundaryRawAnchors(candidate)
	want := []string{
		"Flows & Automation",
		"flows-automation",
		"docs/architecture",
		".devspecs/tasks/v0-founder-workflows/checkpoints",
		"apps/api/internal/ports",
		".devspecs/tasks/v0-foundation",
	}
	assert.Equalf(t, strings.Join(want, "\n"), strings.Join(got, "\n"),
		"raw anchors did not preserve evidence order:\n got=%#v\nwant=%#v", got, want)

}

func insertFreshMapCacheRepo(t *testing.T, repoRoot string) {
	t.Helper()
	db, err := openDB()
	require.NoError(t, err)

	defer db.Close()
	now := "2026-06-01T00:00:00Z"
	head := strings.TrimSpace(string(runMapTestGitOutput(t, repoRoot, "rev-parse", "HEAD")))
	{
		_, err := db.Exec("INSERT INTO repos (id, root_path, last_scan_commit, last_scan_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)", "repo_cache_"+safeFilenamePart(filepath.Base(repoRoot)), canonicalRepoRoot(repoRoot), head, now, now, now)
		require.NoError(t, err)
	}

}

func runMapTestGitOutput(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err,
		"git %v failed: %v\n%s", args, err, out)

	return out
}

func mapOutputHasAreaLabel(out mapOutput, label string) bool {
	for _, area := range out.Areas {
		if area.Label == label {
			return true
		}
	}
	return false
}

func TestMapDefaultAutoScanCreatesUsableIndex(t *testing.T) {
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
	{
		err := mapCmd.Execute()
		require.NoError(t, err)
	}
	assert.Containsf(t, mapErr.String(), "Index updated",
		"map should build substrate on first run, stderr: %s", mapErr.String())
	assert.NotContainsf(t, mapOut.String(), mapIndexRequiredCaveat,
		"default map should not disclose a missing index when handoff commands can auto-index:\n%s", mapOut.String())
	assert.Contains(t, mapOut.String(), "Workspace Identity")
	assert.Contains(t, mapOut.String(), "app/auth/credentials.go")

	db, err := openDB()
	require.NoError(t, err)

	count, countErr := db.CountArtifacts(store.FilterParams{RepoRoot: canonicalRepoRoot(repoRoot)})
	{
		closeErr := db.Close()
		require.NoError(t, closeErr)
	}
	require.NoError(t, countErr)
	assert.NotEqualf(t, 0, count,
		"map default first run should create index artifacts")
}

func TestFindNoRefreshUsesIndexCreatedByMapWithoutRescanning(t *testing.T) {
	repoRoot := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	writeMapTestFile(t, repoRoot, "plans/credentials-plan.md", "# Credentials Rotation\n\nRotate credentials for webhook ingestion.\n")
	writeMapTestFile(t, repoRoot, "app/auth/credentials.go", "package auth\n\nfunc RotateCredentials() {}\n")
	runGitForFindPack(t, repoRoot, "add", ".")
	runGitForFindPack(t, repoRoot, "commit", "-m", "add credentials rotation context")
	createMapIndex(t, repoRoot)

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoRoot))
	t.Cleanup(func() {
		assert.NoError(t, os.Chdir(oldWd))
	})

	findCmd := NewFindCmd()
	findCmd.SetArgs([]string{"credentials rotation", "--no-refresh"})
	findOut := &bytes.Buffer{}
	findErr := &bytes.Buffer{}
	findCmd.SetOut(findOut)
	findCmd.SetErr(findErr)

	err = findCmd.Execute()

	require.NoError(t, err)
	assert.NotContainsf(t, findErr.String(), "Index updated",
		"find --no-refresh should use map-created index without rescanning, stderr: %s", findErr.String())

	output := findOut.String()
	assert.Contains(t, output, "Working set: credentials rotation")
	assert.Contains(t, output, "Credentials Rotation")
}

func createMapIndex(t *testing.T, repoRoot string) {
	t.Helper()

	cmd := NewMapCmd()
	cmd.SetArgs([]string{"--path", repoRoot, "--max-areas", "4"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())
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
	{
		err := mapCmd.Execute()
		require.NoError(t, err)
	}
	assert.NotContainsf(t, mapErr.String(), "Index updated",
		"map --no-refresh should not auto-scan, stderr: %s", mapErr.String())
	assert.Containsf(t, mapOut.String(), mapIndexRequiredCaveat,
		"map --no-refresh should disclose missing local index:\n%s", mapOut.String())

}

func TestMapJSONAutoScanKeepsResultStreamsClean(t *testing.T) {
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
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.Equalf(t, 0, errBuf.Len(),
		"map --json should suppress auto-scan stderr, got: %s", errBuf.String())

	var out mapOutput
	{
		err := json.Unmarshal(outBuf.Bytes(), &out)
		require.NoErrorf(t, err,
			"map --json stdout should remain valid JSON: %v\nstdout=%s\nstderr=%s", err, outBuf.String(), errBuf.String())
	}
	assert.Equal(t, mapSchemaVersion, out.Schema)
	assert.NotEmpty(t, out.Repo.Path)

	assert.NotContainsf(t, strings.Join(out.Caveats, "\n"), mapIndexRequiredCaveat,
		"default JSON map should not disclose missing index: %#v", out.Caveats)

	hasPackability := false
	for _, area := range out.Areas {
		if area.Diagnostics.Packability != nil {
			hasPackability = true
			assert.NotEqualf(t, 0, area.Diagnostics.Packability.KeyPathCount,
				"substrate-backed packability should include key paths: %#v", area.Diagnostics.Packability)

		}
	}
	assert.Truef(t, hasPackability,
		"map --json should preserve packability diagnostics after auto-scan: %#v", out.Areas)
	assert.NotContainsf(t, outBuf.String(), "Index updated",
		"scan notice leaked into JSON stdout:\n%s", outBuf.String())

}

func TestMapQuietAutoScanKeepsStdoutResultOnly(t *testing.T) {
	repoRoot := setupGitRepo(t)
	home := t.TempDir()
	t.Setenv("DEVSPECS_HOME", home)

	writeMapTestFile(t, repoRoot, "plans/credentials-plan.md", "# Credentials Rotation\n\nRotate credentials for webhook ingestion.\n")
	writeMapTestFile(t, repoRoot, "app/auth/credentials.go", "package auth\n\nfunc RotateCredentials() {}\n")
	runGitForFindPack(t, repoRoot, "add", ".")
	runGitForFindPack(t, repoRoot, "commit", "-m", "add credentials rotation context")

	cmd := NewMapCmd()
	cmd.SetArgs([]string{"--json", "--quiet", "--path", repoRoot})
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	{
		err := cmd.Execute()
		require.NoError(t, err)
	}
	assert.Equalf(t, 0, errBuf.Len(),
		"map --quiet should suppress auto-scan stderr, got: %s", errBuf.String())

	var out mapOutput
	{
		err := json.Unmarshal(outBuf.Bytes(), &out)
		require.NoErrorf(t, err,
			"map --quiet stdout should remain valid JSON: %v\nstdout=%s\nstderr=%s", err, outBuf.String(), errBuf.String())
	}
	assert.Equal(t, mapSchemaVersion, out.Schema)
	assert.NotEmpty(t, out.Repo.Path)

	db, err := openDB()
	require.NoError(t, err)

	count, countErr := db.CountArtifacts(store.FilterParams{RepoRoot: canonicalRepoRoot(repoRoot)})
	{
		closeErr := db.Close()
		require.NoError(t, closeErr)
	}
	require.NoError(t, countErr)
	assert.NotEqualf(t, 0, count,
		"map --quiet should still build substrate-backed index")
	assert.NotContains(t, outBuf.String(), "Index updated")
	assert.NotContains(t, outBuf.String(), "Auto-index progress")

}

func TestFilterMapOutputByAreaQueryNarrowsJSONPayload(t *testing.T) {
	out := buildProductMapTestOutput(t)
	filtered := filterMapOutputByAreaQuery(out, "redaction")
	require.Lenf(t, filtered.Areas, 1,
		"filtered area count = %d, want 1; areas=%#v", len(filtered.Areas), filtered.Areas)
	assert.Equalf(t, "Submission", filtered.Areas[0].Label,
		"filtered label = %q, want Submission", filtered.Areas[0].Label)
	assert.Equal(t, "redaction", filtered.Diagnostics.AreaQuery)
	assert.Equal(t, 1, filtered.Diagnostics.MatchedAreaCount)

}

func TestRefineMapAreaLabelMakesLayerLabelsMoreProductReadable(t *testing.T) {
	{
		got := refineMapAreaLabel("Lib Anthropic", []string{"Anthropic Ts"})
		assert.Equalf(t, "Anthropic", got,
			"refined lib label = %q, want Anthropic", got)
	}
	{

		got := refineMapAreaLabel("Application", []string{"Blip Get Canonical Path"})
		assert.Equalf(t, "Blip Application", got,
			"refined application label = %q, want Blip Application", got)
	}
	{

		got := cleanMapCovers("Game", []string{"Ks", "Rts Camera Mode"})
		assert.Equalf(t, "Rts Camera Mode", strings.Join(got, ", "),
			"clean covers kept short raw anchor: %#v", got)
	}

}

func TestMapRecentSubjectTermsFiltersFillerLabels(t *testing.T) {
	got := mapRecentSubjectTerms("fix: clean up error handling, fix a proto-pollution gap, and seal a few loose ends")
	assert.Equalf(t, "proto pollution", strings.Join(got, " "),
		"recent subject terms = %#v", got)

	got = mapRecentSubjectTerms("feat: add open spec")
	require.Lenf(t, got, 0,
		"open spec subject should defer to path terms, got %#v", got)

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
	require.NoError(t, err)

}

func writeMapTestFile(t *testing.T, repoRoot, rel, body string) {
	t.Helper()
	full := filepath.Join(repoRoot, filepath.FromSlash(rel))
	{
		err := os.MkdirAll(filepath.Dir(full), 0o755)
		require.NoError(t, err)
	}
	{

		err := os.WriteFile(full, []byte(body), 0o644)
		require.NoError(t, err)
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
