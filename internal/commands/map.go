package commands

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/freshness"
	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

const (
	mapDefaultMaxAreas                 = 8
	mapMaxArtifactsPerArea             = 4
	mapMaxCoversPerArea                = 5
	mapMaxTraceReceipts                = 3
	mapMaxVerboseTrace                 = 4
	mapRecentMaxCommits                = 40
	mapRecentMaxTopics                 = 5
	mapBoundaryMaxFiles                = 30000
	mapBoundaryMaxCommits              = 60
	mapBoundaryMaxArtifacts            = 24
	mapBoundaryMaxImportFiles          = 8000
	mapBoundaryMaxImportFilesLargeRepo = 4500
	mapBoundaryLargeRepoFiles          = 12000
	mapBoundaryMaxConceptualFiles      = 14000
	mapBoundaryMaxImportBytes          = 512 * 1024
	mapBoundaryImportScoreCap          = 40
	mapBoundaryTestImportScoreCap      = 16
	mapBoundaryFilesTimeout            = 3 * time.Second
	mapSchemaVersion                   = "devspecs.map.v1"
	mapRecentSchemaVersion             = "devspecs.map.recent.v1"
	mapTraceReceiptMode                = "bounded_git_path_receipts_v0"
	mapIndexRequiredCaveat             = "local index is not loaded yet; suggested ds find commands will auto-index unless --no-refresh is set"
	mapLowConfidence                   = "low"
	mapMediumConfidence                = "medium"
	mapHighConfidence                  = "high"
	mapClassStableArea                 = "stable_area"
	mapClassWorkstream                 = "workstream"
	mapClassDocTopic                   = "doc_topic"
	mapClassProtocol                   = "protocol"
	mapClassLowConfidence              = "low_confidence"
	mapTypeDomainFeature               = "domain_feature"
	mapTypeBusinessFlow                = "business_workflow"
	mapTypeExternal                    = "external_integration"
	mapTypeAPI                         = "api_surface"
	mapTypeUI                          = "ui_surface"
	mapTypeDataModel                   = "data_model"
	mapTypeDataPipeline                = "data_pipeline"
	mapTypePlatform                    = "platform_capability"
	mapTypeOps                         = "ops_runtime"
	mapTypeTooling                     = "tooling_script"
	mapTypeTestQuality                 = "test_quality"
	mapTypeProtocol                    = "protocol_process"
	mapTypeDocs                        = "docs_reference"
	mapTypeRoot                        = "repo_root_umbrella"
	mapTypeUnknown                     = "unknown_area"
	mapBoundaryRoleProductCapability   = "product_capability"
	mapBoundaryRoleHorizontalLayer     = "horizontal_layer"
	mapBoundaryRoleExtensionEcosystem  = "extension_ecosystem"
	mapBoundaryRoleFixtureOrTestbed    = "fixture_or_testbed"
	mapBoundaryRoleRepoNamespace       = "repo_namespace"
	mapBoundaryRoleGenericParent       = "generic_parent"
	mapBoundaryRoleDocsReference       = "docs_reference"
	mapBoundaryRoleHandoffUnsafe       = "handoff_unsafe"
	mapRepoShapeTool                   = "tool"
	mapRepoShapeWebApp                 = "web_app"
	mapRepoShapePlatform               = "platform"
	mapRepoShapeLibrary                = "library"
	mapRepoShapeDocsSite               = "docs_site"
	mapRepoShapeUnknown                = "unknown"
)

// NewMapCmd creates the ds map command.
func NewMapCmd() *cobra.Command {
	var (
		path      string
		asJSON    bool
		verbose   bool
		quiet     bool
		recent    bool
		boundary  bool
		noRefresh bool
		maxAreas  int
	)

	cmd := &cobra.Command{
		Use:   "map [area]",
		Short: "Show architecture/system boundaries and useful follow-up context commands",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			areaQuery := ""
			if len(args) > 0 {
				areaQuery = args[0]
			}
			return runMap(cmd, mapOptions{
				Path:      path,
				AreaQuery: areaQuery,
				JSON:      asJSON,
				Verbose:   verbose,
				Quiet:     quiet,
				Recent:    recent,
				Boundary:  boundary,
				NoRefresh: noRefresh,
				MaxAreas:  maxAreas,
			})
		},
	}

	cmd.Flags().StringVar(&path, "path", ".", "Repository path to map")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show map diagnostics and extra evidence")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress non-result progress and auto-refresh notices")
	cmd.Flags().BoolVar(&recent, "recent", false, "Show recently active topics from local git history")
	cmd.Flags().BoolVar(&boundary, "experimental-boundaries", false, "Deprecated: ds map now uses path-primary system boundary candidates by default")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	cmd.Flags().IntVar(&maxAreas, "max-areas", mapDefaultMaxAreas, "Maximum areas to show")
	_ = cmd.Flags().MarkHidden("recent")
	_ = cmd.Flags().MarkDeprecated("recent", "use `ds recent` instead")
	_ = cmd.Flags().MarkHidden("experimental-boundaries")
	_ = cmd.Flags().MarkDeprecated("experimental-boundaries", "ds map now uses boundary mapping by default")
	return cmd
}

// NewRecentCmd creates the ds recent command.
func NewRecentCmd() *cobra.Command {
	var (
		path      string
		asJSON    bool
		verbose   bool
		quiet     bool
		noRefresh bool
		maxAreas  int
	)

	cmd := &cobra.Command{
		Use:   "recent [topic]",
		Short: "Show recently active local git topics and follow-up context commands",
		Long: `Show recently active topics from local git history.

Use this as a diagnostic/evidence layer when you need to see what changed
recently before choosing a task or context query. Once the target is known,
use ds task for bounded execution or ds find for an agent-readable context
pack.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			areaQuery := ""
			if len(args) > 0 {
				areaQuery = args[0]
			}
			return runRecent(cmd, mapOptions{
				Path:      path,
				AreaQuery: areaQuery,
				JSON:      asJSON,
				Verbose:   verbose,
				Quiet:     quiet,
				Recent:    true,
				NoRefresh: noRefresh,
				MaxAreas:  maxAreas,
			})
		},
	}

	cmd.Flags().StringVar(&path, "path", ".", "Repository path to inspect")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show diagnostics and extra evidence")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress non-result progress and auto-refresh notices")
	cmd.Flags().BoolVar(&noRefresh, "no-refresh", false, "Skip auto-scan freshness check")
	cmd.Flags().IntVar(&maxAreas, "max-areas", mapDefaultMaxAreas, "Maximum topics to show")
	return cmd
}

type mapOptions struct {
	Path      string
	AreaQuery string
	JSON      bool
	Verbose   bool
	Quiet     bool
	Recent    bool
	Boundary  bool
	NoRefresh bool
	MaxAreas  int
}

type mapOutput struct {
	Schema               string                  `json:"schema"`
	Repo                 mapRepo                 `json:"repo"`
	EvidenceAvailability mapEvidenceAvailability `json:"evidence_availability"`
	Areas                []mapArea               `json:"areas"`
	Caveats              []string                `json:"caveats,omitempty"`
	Diagnostics          mapDiagnostics          `json:"diagnostics,omitempty"`
}

type mapRecentOutput struct {
	Schema      string               `json:"schema"`
	Repo        mapRepo              `json:"repo"`
	AreaQuery   string               `json:"area_query,omitempty"`
	Topics      []mapRecentTopic     `json:"topics"`
	Caveats     []string             `json:"caveats,omitempty"`
	Diagnostics mapRecentDiagnostics `json:"diagnostics,omitempty"`
}

type mapRecentTopic struct {
	Label          string            `json:"label"`
	Query          string            `json:"query"`
	CommitCount    int               `json:"commit_count"`
	FileCount      int               `json:"file_count"`
	EvidenceCounts map[string]int    `json:"evidence_counts,omitempty"`
	KeyPaths       []string          `json:"key_paths,omitempty"`
	TopicType      string            `json:"topic_type,omitempty"`
	BoundaryLabel  string            `json:"boundary_label,omitempty"`
	BoundaryRole   string            `json:"boundary_role,omitempty"`
	BoundaryPaths  []string          `json:"boundary_paths,omitempty"`
	QualitySignals []string          `json:"quality_signals,omitempty"`
	RecentSignals  []mapTraceReceipt `json:"recent_signals,omitempty"`
	Try            string            `json:"try,omitempty"`
	Score          int               `json:"score,omitempty"`
}

type mapRecentDiagnostics struct {
	CommitsRead       int `json:"commits_read,omitempty"`
	CommitsSkipped    int `json:"commits_skipped,omitempty"`
	RawTopicCount     int `json:"raw_topic_count,omitempty"`
	MatchedTopicCount int `json:"matched_topic_count,omitempty"`
}

type mapRepo struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Confidence string `json:"confidence"`
}

type mapEvidenceAvailability struct {
	Markdown int  `json:"markdown"`
	OpenSpec int  `json:"openspec"`
	ADR      int  `json:"adr"`
	Source   int  `json:"source"`
	Test     int  `json:"test"`
	Comment  int  `json:"comment"`
	Intent   int  `json:"intent"`
	Protocol int  `json:"protocol"`
	Trace    bool `json:"trace"`
}

type mapArea struct {
	ID                 string             `json:"id"`
	Label              string             `json:"label"`
	Class              string             `json:"class"`
	AreaType           string             `json:"area_type"`
	BoundaryRole       string             `json:"boundary_role,omitempty"`
	Purpose            string             `json:"purpose,omitempty"`
	BoundaryPaths      []string           `json:"boundary_paths,omitempty"`
	AdjacentSystems    []string           `json:"adjacent_systems,omitempty"`
	Confidence         string             `json:"confidence"`
	IsRepoRootUmbrella bool               `json:"is_repo_root_umbrella,omitempty"`
	Covers             []string           `json:"covers,omitempty"`
	EvidenceCounts     map[string]int     `json:"evidence_counts,omitempty"`
	KeyPaths           []string           `json:"key_paths,omitempty"`
	TraceReceipts      []mapTraceReceipt  `json:"trace_receipts,omitempty"`
	Try                string             `json:"try,omitempty"`
	Caveats            []string           `json:"caveats,omitempty"`
	Diagnostics        mapAreaDiagnostics `json:"diagnostics,omitempty"`
}

type mapTraceReceipt struct {
	SHA     string `json:"sha,omitempty"`
	Subject string `json:"subject"`
}

type mapDiagnostics struct {
	RawClusterCount           int      `json:"raw_cluster_count,omitempty"`
	WorkstreamAnchorsSeen     int      `json:"workstream_anchors_seen,omitempty"`
	WorkstreamMaterialized    int      `json:"workstream_materialized,omitempty"`
	AreaQuery                 string   `json:"area_query,omitempty"`
	MatchedAreaCount          int      `json:"matched_area_count,omitempty"`
	SuppressedLabels          []string `json:"suppressed_labels,omitempty"`
	TraceNoisyCommitsFiltered int      `json:"trace_noisy_commits_filtered,omitempty"`
}

type mapAreaDiagnostics struct {
	Key              string                     `json:"key,omitempty"`
	RawAnchors       []string                   `json:"raw_anchors,omitempty"`
	LabelEvidence    []string                   `json:"label_evidence,omitempty"`
	TraceTerms       []string                   `json:"trace_terms,omitempty"`
	TraceReceiptMode string                     `json:"trace_receipt_mode,omitempty"`
	Packability      *mapPackabilityDiagnostics `json:"packability,omitempty"`
}

type mapPackabilityDiagnostics struct {
	KeyPathCount            int      `json:"key_path_count,omitempty"`
	IndexedKeyPathCount     int      `json:"indexed_key_path_count,omitempty"`
	PrefixKeyPathCount      int      `json:"prefix_key_path_count,omitempty"`
	IndexedQueryAnchorCount int      `json:"indexed_query_anchor_count,omitempty"`
	MissingKeyExtensions    []string `json:"missing_key_extensions,omitempty"`
	Decision                string   `json:"decision,omitempty"`
	SelectedTrySource       string   `json:"selected_try_source,omitempty"`
	SuppressedTry           string   `json:"suppressed_try,omitempty"`
	SuppressedTrySource     string   `json:"suppressed_try_source,omitempty"`
	TrySuppressed           bool     `json:"try_suppressed,omitempty"`
}

type mapAreaInternal struct {
	Key             string
	Label           string
	LabelScore      float64
	LabelSource     string
	Subareas        map[string]bool
	RawAnchors      []string
	Artifacts       []mapArtifact
	ArtifactPathSet map[string]bool
	EvidenceCounts  map[string]int
	EvidenceCount   int
	ConfidenceSum   float64
	EvidenceSources map[string]bool
	ExampleCommits  []string
	TraceReceipts   []mapTraceReceipt
	Caveats         []string
	GenericCount    int
	Filtered        bool
}

type mapArtifact struct {
	Title   string
	Kind    string
	Subtype string
	Path    string
	Rank    int
}

type mapPreparedCluster struct {
	Anchor          string
	ParentKey       string
	ParentLabel     string
	ParentScore     float64
	ParentSource    string
	Subarea         string
	Artifacts       []mapArtifact
	ArtifactPathSet map[string]bool
	EvidenceCounts  map[string]int
	EvidenceCount   int
	Confidence      float64
	EvidenceSources map[string]bool
	ExampleCommits  []string
	Caveats         []string
	GenericAnchor   bool
}

type mapLabelCandidate struct {
	Key           string
	Label         string
	Score         float64
	Sources       map[string]bool
	ArtifactPaths map[string]bool
}

type mapConceptualBoundaryPattern struct {
	All []string
	Any []string
}

type mapConceptualCoverRule struct {
	Label string
	All   []string
	Any   []string
}

type mapConceptualBoundaryRule struct {
	Key      string
	Label    string
	Score    float64
	Shapes   []string
	Patterns []mapConceptualBoundaryPattern
	Covers   []mapConceptualCoverRule
}

type mapConceptualNeedle struct {
	raw        string
	path       string
	key        string
	hasSlash   bool
	segmentSeq bool
	dotted     bool
}

var mapGenericTerms = map[string]bool{
	"app": true, "apps": true, "src": true, "source": true, "lib": true, "libs": true,
	"pkg": true, "packages": true, "internal": true, "external": true, "api": true,
	"apis": true, "backend": true, "frontend": true, "web": true, "portal": true,
	"client": true, "server": true, "service": true, "services": true, "component": true,
	"components": true, "container": true, "containers": true, "section": true,
	"sections": true, "feature": true, "features": true, "module": true, "modules": true,
	"private": true, "public": true, "common": true, "shared": true, "core": true,
	"main": true, "index": true, "init": true, "utils": true, "utility": true,
	"helpers": true, "helper": true, "middleware": true, "model": true, "models": true,
	"schema": true, "schemas": true, "route": true, "routes": true, "config": true,
	"configs": true, "configuration": true, "test": true, "tests": true, "testing": true,
	"unit": true, "integration": true, "e2e": true, "spec": true, "specs": true,
	"issue": true, "issues": true, "pr": true, "prs": true,
	"docs": true, "doc": true, "documentation": true, "guide": true, "guides": true,
	"tutorial": true, "tutorials": true,
	"readme": true, "claude": true, "agents": true, "agent": true, "instructions": true,
	"workflow": true, "workflows": true, "task": true, "tasks": true, "plan": true,
	"plans": true, "roadmap": true, "roadmaps": true, "v1": true, "v2": true, "v3": true,
	"py": true, "go": true, "ts": true, "tsx": true, "js": true, "jsx": true,
	"rs": true, "java": true, "md": true, "json": true, "yaml": true, "yml": true,
	"toml": true, "xml": true, "html": true, "css": true, "scss": true, "generated": true,
	"vendor": true, "node": true, "node_modules": true, "dist": true, "build": true,
	"target": true, "fixtures": true, "fixture": true, "testdata": true, "_ignore": true,
	"ignore": true, "cache": true, "tmp": true, "temp": true,
}

var mapGenericAreaLabels = map[string]bool{
	"api-backend": true, "api-frontend": true, "backend-api": true, "frontend-app": true,
	"components-custom": true, "private-gemeinde": true, "private-adresse": true,
	"service-form": true, "service-search": true, "target-type-test": true,
	"target-type-config": true, "target-type-doc": true,
}

var mapWeakStandaloneAreaLabels = map[string]bool{
	"body": true, "docs": true, "docs/en": true, "map": true, "reason": true,
	"scripts": true, "self": true, "tutorial": true, "type": true,
}

var mapBoundarySuppressedStandaloneLabels = map[string]bool{
	"all": true, "dashboard": true, "dashboards": true, "engine": true, "hook": true,
	"graphql": true, "hooks": true, "icon": true, "icons": true, "modal": true, "modals": true,
	"nucleo": true, "page-layout": true, "propel": true, "states": true, "store": true,
	"stores": true, "story": true, "stories": true, "suite": true, "suites": true,
	"view": true, "views": true, "workflow": true, "workflows": true,
}

var mapBoundaryFirstScreenShellLabels = map[string]bool{
	"blackbox": true, "components-next": true, "composables": true, "desktop-client": true,
	"examples": true, "general": true, "javascript": true, "locales": true,
	"loot-core": true, "phrases": true, "primitives": true, "proto": true,
	"remix": true, "router": true, "server-only": true, "settings": true,
	"templates": true, "trpc": true, "ui-primitives": true, "universal": true,
	"utilities": true,
}

var mapBoundaryHandoffBroadTerms = map[string]bool{
	"api": true, "apis": true, "core": true, "engine": true, "framework": true,
	"base": true, "code": true, "com": true, "dev": true, "generic": true, "layer": true, "meta": true, "namespace": true, "objects": true,
	"org":      true,
	"platform": true, "plugin": true, "plugins": true, "public": true, "registry": true,
	"repo": true, "repos": true, "repositories": true, "repository": true, "resources": true,
	"router": true, "routers": true, "runtime": true, "shell": true, "static": true,
	"style": true, "styles": true, "system": true, "systems": true, "unified": true,
}

var mapSuppressedPathSegments = map[string]bool{
	"_ignore": true, ".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, "target": true, ".next": true, "coverage": true, "fixtures": true,
	"fixture": true, "testdata": true, "samples": true, "sample-corpora": true,
}

var mapProtocolSubtypes = map[string]bool{
	"agent_instruction": true, "skill": true, "protocol": true, "contributing": true,
}

var mapBoundaryAllowedGenericAreaLabels = map[string]bool{
	"issue": true, "issues": true, "workflow": true, "workflows": true,
}

var mapBoundarySourceExtensions = []string{
	".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue", ".svelte",
	".py", ".go", ".rs", ".java", ".kt", ".kts", ".cs",
}

var mapBoundaryJSImportRegexes = []*regexp.Regexp{
	regexp.MustCompile(`\bimport\s+(?:type\s+)?(?:[^'"]*?\s+from\s+)?["']([^"']+)["']`),
	regexp.MustCompile(`\bexport\s+(?:type\s+)?[^'"]*?\s+from\s+["']([^"']+)["']`),
	regexp.MustCompile(`\brequire\s*\(\s*["']([^"']+)["']\s*\)`),
	regexp.MustCompile(`\bimport\s*\(\s*["']([^"']+)["']\s*\)`),
}

var (
	mapBoundaryPythonFromImportRegex = regexp.MustCompile(`(?m)^\s*from\s+([A-Za-z_][\w.]*|\.[\w.]*)\s+import\s+`)
	mapBoundaryPythonImportRegex     = regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z_][\w.]*)`)
	mapBoundaryGoImportRegex         = regexp.MustCompile(`\bimport\s+"([^"]+)"`)
	mapBoundaryGoImportBlockRegex    = regexp.MustCompile(`(?m)^\s*"([^"]+)"\s*$`)
	mapBoundaryRustUseRegex          = regexp.MustCompile(`(?m)^\s*use\s+([^;]+);`)
	mapBoundaryRustModRegex          = regexp.MustCompile(`(?m)^\s*mod\s+([A-Za-z_]\w*)\s*;`)
	mapBoundaryJavaLikeImportRegex   = regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([A-Za-z_][\w.]*)(?:\.\*)?\s*;`)
	mapTryGeneratedNumberSuffixRegex = regexp.MustCompile(`^(?:bug|case|test|spec|fixture)\d+$`)
	mapTryGeneratedNumericLeadRegex  = regexp.MustCompile(`^\d+[a-z]+\d*$`)
	mapTryGeneratedShortCodeRegex    = regexp.MustCompile(`^[a-z]{1,3}\d+[a-z0-9]*$`)
	mapTryGeneratedExtraRegex        = regexp.MustCompile(`^[a-z]+extra$`)
)

var mapConceptualBoundaryRules = []mapConceptualBoundaryRule{
	{
		Key:    "project-workspace-lifecycle",
		Label:  "Project & Workspace Lifecycle",
		Score:  15,
		Shapes: []string{mapRepoShapeTool},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"workspace", "workspaces", "project", "projects", "pyproject", "lockfile", "dependency-groups"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Projects", Any: []string{"project", "projects", "pyproject"}},
			{Label: "Workspaces", Any: []string{"workspace", "workspaces"}},
			{Label: "Dependency Groups", Any: []string{"dependency-groups"}},
			{Label: "Lockfile", Any: []string{"lockfile"}},
		},
	},
	{
		Key:    "dependency-resolution-lockfile",
		Label:  "Dependency Resolution & Lockfile",
		Score:  15,
		Shapes: []string{mapRepoShapeTool},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"resolver", "resolution", "requirements", "lock", "lockfile", "pubgrub", "pep508", "pep440", "dependency", "dependencies"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Resolver", Any: []string{"resolver", "resolution", "pubgrub"}},
			{Label: "Requirements", Any: []string{"requirements"}},
			{Label: "Lockfile", Any: []string{"lock", "lockfile"}},
			{Label: "Dependencies", Any: []string{"dependency", "dependencies", "pep508", "pep440"}},
		},
	},
	{
		Key:    "package-install-virtual-environments",
		Label:  "Package Installation & Virtual Environments",
		Score:  14,
		Shapes: []string{mapRepoShapeTool},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"installer", "install", "sync", "site-packages", "virtualenv", "venv", "wheel", "editable", "uv-virtualenv"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Installer", Any: []string{"installer", "install"}},
			{Label: "Sync", Any: []string{"sync"}},
			{Label: "Virtualenv", Any: []string{"virtualenv", "venv", "uv-virtualenv"}},
			{Label: "Wheels / Editable", Any: []string{"wheel", "editable"}},
		},
	},
	{
		Key:    "registry-cache-artifact-fetching",
		Label:  "Registry, Cache & Artifact Fetching",
		Score:  14,
		Shapes: []string{mapRepoShapeTool},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"registry", "pypi", "index", "indexes", "simple", "cache", "distribution", "distribution-filename", "client", "http", "fetch"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Registry / Index", Any: []string{"registry", "pypi", "index", "indexes", "simple"}},
			{Label: "Cache", Any: []string{"cache"}},
			{Label: "Distributions", Any: []string{"distribution", "distribution-filename"}},
			{Label: "Fetching", Any: []string{"client", "http", "fetch"}},
		},
	},
	{
		Key:    "managed-python-interpreters",
		Label:  "Managed Python Interpreters",
		Score:  13,
		Shapes: []string{mapRepoShapeTool},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"uv-python", "python-install", "python-downloads", "interpreter", "interpreters", "python-list", "python-find", "python-dir", "install-python", "python-versions"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Python Runtime", Any: []string{"uv-python"}},
			{Label: "Interpreter Discovery", Any: []string{"interpreter", "interpreters", "python-find"}},
			{Label: "Python Install", Any: []string{"python-install", "install-python", "python-downloads"}},
			{Label: "Python Versions", Any: []string{"python-versions", "python-list", "python-dir"}},
		},
	},
	{
		Key:    "pip-compatible-interface",
		Label:  "pip-Compatible Interface",
		Score:  13,
		Shapes: []string{mapRepoShapeTool},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"uv-pip", "pip_compile", "pip-sync", "pip_install", "pip_install_scenarios", "/pip/", "docs/pip", "pip_"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "pip install", Any: []string{"pip_install", "pip-install", "pip install"}},
			{Label: "pip compile", Any: []string{"pip_compile", "pip-compile"}},
			{Label: "pip sync", Any: []string{"pip_sync", "pip-sync"}},
			{Label: "pip docs", Any: []string{"docs/pip", "/pip/"}},
		},
	},
	{
		Key:    "tools-ephemeral-environments",
		Label:  "Tools & Ephemeral Environments",
		Score:  13,
		Shapes: []string{mapRepoShapeTool},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"uv-tool", "tool_install", "tool_run", "tool-upgrade", "tools", "uvx", "ephemeral"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Tool Install", Any: []string{"tool_install", "tool-install", "uv-tool"}},
			{Label: "Tool Run", Any: []string{"tool_run", "tool-run", "uvx"}},
			{Label: "Ephemeral Environments", Any: []string{"ephemeral"}},
		},
	},
	{
		Key:    "run-scripts-inline-dependencies",
		Label:  "Run, Scripts & Inline Dependencies",
		Score:  12,
		Shapes: []string{mapRepoShapeTool},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"commands/project/run", "uv-scripts", "scripts.md", "pep-723", "inline", "run.rs", "script"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Run Command", Any: []string{"commands/project/run", "run.rs"}},
			{Label: "Scripts", Any: []string{"uv-scripts", "scripts.md", "script"}},
			{Label: "Inline Dependencies", Any: []string{"pep-723", "inline"}},
		},
	},
	{
		Key:    "build-publish-auth-audit",
		Label:  "Build, Publish, Auth & Audit",
		Score:  12,
		Shapes: []string{mapRepoShapeTool},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"build-backend", "uv-build", "publish", "uv-publish", "authentication", "uv-auth", "audit", "uv-audit", "trusted-publishing"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Build", Any: []string{"build-backend", "uv-build"}},
			{Label: "Publish", Any: []string{"publish", "uv-publish", "trusted-publishing"}},
			{Label: "Auth", Any: []string{"authentication", "uv-auth"}},
			{Label: "Audit", Any: []string{"audit", "uv-audit"}},
		},
	},
	{
		Key:    "accounts-net-worth-dashboard",
		Label:  "Accounts & Net-Worth Dashboard",
		Score:  14,
		Shapes: []string{mapRepoShapeWebApp},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"accounts", "accountable", "balance-sheet", "net-worth", "valuation", "valuations", "account_balances", "account-sync"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Accounts", Any: []string{"accounts", "accountable"}},
			{Label: "Balances", Any: []string{"balance", "account_balances"}},
			{Label: "Net Worth", Any: []string{"net-worth", "balance-sheet"}},
			{Label: "Valuations", Any: []string{"valuation", "valuations"}},
		},
	},
	{
		Key:    "transaction-ledger-categorization-cashflow",
		Label:  "Transaction Ledger, Categorization & Cashflow",
		Score:  14,
		Shapes: []string{mapRepoShapeWebApp},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"transactions", "transaction", "entries", "entry", "entryable", "category", "categories", "merchant", "cashflow"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Transactions", Any: []string{"transactions", "transaction"}},
			{Label: "Entries", Any: []string{"entries", "entry", "entryable"}},
			{Label: "Categories", Any: []string{"category", "categories"}},
			{Label: "Merchants", Any: []string{"merchant"}},
			{Label: "Cashflow", Any: []string{"cashflow"}},
		},
	},
	{
		Key:    "budgeting",
		Label:  "Budgeting",
		Score:  12,
		Shapes: []string{mapRepoShapeWebApp},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"budget", "budgets", "budget_categories"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Budgets", Any: []string{"budget", "budgets"}},
			{Label: "Budget Categories", Any: []string{"budget_categories"}},
		},
	},
	{
		Key:    "investments-holdings-securities",
		Label:  "Investments, Holdings & Securities",
		Score:  13,
		Shapes: []string{mapRepoShapeWebApp},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"investment", "investments", "holding", "holdings", "security", "securities", "trade", "trades", "ticker", "positions"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Investments", Any: []string{"investment", "investments"}},
			{Label: "Holdings", Any: []string{"holding", "holdings"}},
			{Label: "Securities", Any: []string{"security", "securities"}},
			{Label: "Trades", Any: []string{"trade", "trades"}},
		},
	},
	{
		Key:    "bank-connectivity-plaid-sync",
		Label:  "Bank Connectivity & Plaid Sync",
		Score:  13,
		Shapes: []string{mapRepoShapeWebApp},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"plaid", "plaid_account", "plaid_item", "plaid_entry", "bank", "institution", "account-sync"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Plaid Accounts", Any: []string{"plaid_account"}},
			{Label: "Plaid Items", Any: []string{"plaid_item"}},
			{Label: "Plaid Entries", Any: []string{"plaid_entry"}},
			{Label: "Institutions", Any: []string{"institution", "bank"}},
		},
	},
	{
		Key:    "csv-manual-data-import",
		Label:  "CSV & Manual Data Import",
		Score:  12,
		Shapes: []string{mapRepoShapeWebApp},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"import", "imports", "csv", "mint.csv", "transaction_import", "trade_import", "upload"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Imports", Any: []string{"import", "imports"}},
			{Label: "CSV", Any: []string{"csv", "mint.csv"}},
			{Label: "Upload", Any: []string{"upload"}},
			{Label: "Transaction Import", Any: []string{"transaction_import"}},
		},
	},
	{
		Key:    "billing-subscriptions-self-host",
		Label:  "Billing, Subscriptions & Self-Host Operations",
		Score:  12,
		Shapes: []string{mapRepoShapeWebApp},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"billing", "billings", "subscription", "subscriptions", "hosting", "hostings", "self-host", "self_host", "stripe"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Billing", Any: []string{"billing", "billings"}},
			{Label: "Subscriptions", Any: []string{"subscription", "subscriptions"}},
			{Label: "Self-Host", Any: []string{"self-host", "self_host", "hosting", "hostings"}},
			{Label: "Stripe", Any: []string{"stripe"}},
		},
	},
	{
		Key:    "external-http-api-v1",
		Label:  "External HTTP API v1",
		Score:  11,
		Shapes: []string{mapRepoShapeWebApp},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"api/v1", "docs/api", "v1/accounts", "v1/transactions", "v1/chats"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "API v1", Any: []string{"api/v1"}},
			{Label: "Accounts API", Any: []string{"v1/accounts"}},
			{Label: "Transactions API", Any: []string{"v1/transactions"}},
			{Label: "Chats API", Any: []string{"v1/chats"}},
		},
	},
	{
		Key:    "framework-runtime-module-platform",
		Label:  "Framework Runtime & Module Platform",
		Score:  15,
		Shapes: []string{mapRepoShapePlatform},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"packages/core/framework", "modules-sdk", "packages/core/modules-sdk", "awilix", "link-modules", "packages/medusa/src/loaders", "instrumentation"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Framework", Any: []string{"packages/core/framework"}},
			{Label: "Modules SDK", Any: []string{"modules-sdk", "packages/core/modules-sdk"}},
			{Label: "Loaders", Any: []string{"packages/medusa/src/loaders"}},
			{Label: "Link Modules", Any: []string{"link-modules"}},
		},
	},
	{
		Key:    "product-catalog-pricing-inventory",
		Label:  "Product Catalog, Pricing & Inventory",
		Score:  15,
		Shapes: []string{mapRepoShapePlatform},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"packages/modules/product", "packages/modules/pricing", "packages/modules/inventory", "stock-location", "price-list", "product-variants", "product-categories", "collections"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Products", Any: []string{"packages/modules/product", "product-variants", "product-categories"}},
			{Label: "Pricing", Any: []string{"packages/modules/pricing", "price-list"}},
			{Label: "Inventory", Any: []string{"packages/modules/inventory"}},
			{Label: "Stock Locations", Any: []string{"stock-location"}},
			{Label: "Collections", Any: []string{"collections"}},
		},
	},
	{
		Key:    "cart-checkout-promotions",
		Label:  "Cart, Checkout & Promotions",
		Score:  14,
		Shapes: []string{mapRepoShapePlatform},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"packages/modules/cart", "packages/modules/promotion", "store/carts", "campaigns", "promotions", "payment-collection", "line-item"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Cart", Any: []string{"packages/modules/cart", "store/carts"}},
			{Label: "Promotions", Any: []string{"packages/modules/promotion", "promotions"}},
			{Label: "Campaigns", Any: []string{"campaigns"}},
			{Label: "Payment Collections", Any: []string{"payment-collection"}},
			{Label: "Line Items", Any: []string{"line-item"}},
		},
	},
	{
		Key:    "orders-fulfillment-post-purchase",
		Label:  "Orders, Fulfillment & Post-Purchase",
		Score:  15,
		Shapes: []string{mapRepoShapePlatform},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"packages/modules/order", "packages/modules/fulfillment", "orders", "fulfillments", "returns", "claims", "exchanges", "order-edits", "draft-order"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Orders", Any: []string{"packages/modules/order", "orders"}},
			{Label: "Fulfillment", Any: []string{"packages/modules/fulfillment", "fulfillments"}},
			{Label: "Returns", Any: []string{"returns"}},
			{Label: "Claims / Exchanges", Any: []string{"claims", "exchanges"}},
			{Label: "Draft Orders", Any: []string{"draft-order"}},
		},
	},
	{
		Key:    "payments-tax-monetary-configuration",
		Label:  "Payments, Tax & Monetary Configuration",
		Score:  14,
		Shapes: []string{mapRepoShapePlatform},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"packages/modules/payment", "packages/modules/tax", "packages/modules/currency", "packages/modules/region", "payment-stripe", "tax-regions", "tax-rates", "payment-providers", "refund"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Payments", Any: []string{"packages/modules/payment", "payment-providers", "payment-stripe"}},
			{Label: "Tax", Any: []string{"packages/modules/tax", "tax-regions", "tax-rates"}},
			{Label: "Currency", Any: []string{"packages/modules/currency"}},
			{Label: "Regions", Any: []string{"packages/modules/region"}},
			{Label: "Refunds", Any: []string{"refund"}},
		},
	},
	{
		Key:    "store-configuration-sales-channels",
		Label:  "Store Configuration & Sales Channels",
		Score:  13,
		Shapes: []string{mapRepoShapePlatform},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"packages/modules/store", "sales-channel", "packages/modules/settings", "translation", "locales", "stores", "sales-channels"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Stores", Any: []string{"packages/modules/store", "stores"}},
			{Label: "Sales Channels", Any: []string{"sales-channel", "sales-channels"}},
			{Label: "Settings", Any: []string{"packages/modules/settings"}},
			{Label: "Translations / Locales", Any: []string{"translation", "locales"}},
		},
	},
	{
		Key:    "http-api-layer",
		Label:  "HTTP API Layer",
		Score:  13,
		Shapes: []string{mapRepoShapePlatform},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"packages/medusa/src/api/admin", "packages/medusa/src/api/store", "packages/medusa/src/api/auth", "integration-tests/http", "integration-tests/api", "http-types", "openapi"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Admin API", Any: []string{"packages/medusa/src/api/admin"}},
			{Label: "Store API", Any: []string{"packages/medusa/src/api/store"}},
			{Label: "Auth API", Any: []string{"packages/medusa/src/api/auth"}},
			{Label: "HTTP Tests", Any: []string{"integration-tests/http", "integration-tests/api"}},
			{Label: "OpenAPI / HTTP Types", Any: []string{"http-types", "openapi"}},
		},
	},
	{
		Key:    "admin-dashboard",
		Label:  "Admin Dashboard",
		Score:  12,
		Shapes: []string{mapRepoShapePlatform},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"packages/admin/dashboard", "admin-bundler", "admin-sdk", "admin-vite-plugin", "admin-shared"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Dashboard Routes", Any: []string{"packages/admin/dashboard"}},
			{Label: "Admin Bundler", Any: []string{"admin-bundler"}},
			{Label: "Admin SDK", Any: []string{"admin-sdk"}},
			{Label: "Admin Vite Plugin", Any: []string{"admin-vite-plugin"}},
		},
	},
	{
		Key:    "provider-adapters-pluggable-infrastructure",
		Label:  "Provider Adapters & Pluggable Infrastructure",
		Score:  13,
		Shapes: []string{mapRepoShapePlatform},
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"packages/modules/providers", "provider", "providers", "payment-stripe", "file-s3", "file-local", "auth-emailpass", "auth-github", "auth-google", "notification-sendgrid", "fulfillment-manual", "caching-redis", "locking-redis", "event-bus-redis"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Providers", Any: []string{"packages/modules/providers", "provider", "providers"}},
			{Label: "Payments", Any: []string{"payment-stripe"}},
			{Label: "Files", Any: []string{"file-s3", "file-local"}},
			{Label: "Auth Providers", Any: []string{"auth-emailpass", "auth-github", "auth-google"}},
			{Label: "Infrastructure", Any: []string{"caching-redis", "locking-redis", "event-bus-redis"}},
		},
	},
	{
		Key:   "short-link-redirect-click-capture",
		Label: "Short-Link Redirect & Click Capture",
		Score: 14,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"middleware/link", "short-link", "shortlink", "tinybird", "click", "clicks", "/stats/", "/links/"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Links", Any: []string{"/links/", "link"}},
			{Label: "Redirect Middleware", Any: []string{"middleware/link", "redirect"}},
			{Label: "Click Events", Any: []string{"click", "clicks"}},
			{Label: "Tinybird", Any: []string{"tinybird"}},
			{Label: "Stats", Any: []string{"/stats/", "stats"}},
		},
	},
	{
		Key:   "click-analytics-conversion-attribution",
		Label: "Click Analytics & Conversion Attribution",
		Score: 13,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"analytics", "conversion", "conversions", "events", "tracking", "attribution", "tinybird", "customer"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Analytics", Any: []string{"analytics"}},
			{Label: "Conversions", Any: []string{"conversion", "conversions"}},
			{Label: "Events", Any: []string{"events", "tracking"}},
			{Label: "Attribution", Any: []string{"attribution"}},
			{Label: "Customers", Any: []string{"customer", "customers"}},
		},
	},
	{
		Key:   "custom-domains-link-infrastructure",
		Label: "Custom Domains & Link Infrastructure",
		Score: 12,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"custom-domain", "custom-domains", "dynadot", "well-known", "hostname"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Domains", Any: []string{"domain", "domains"}},
			{Label: "DNS", Any: []string{"dns"}},
			{Label: "Dynadot", Any: []string{"dynadot"}},
			{Label: "Well-Known Routes", Any: []string{"well-known"}},
			{Label: "Hostnames", Any: []string{"hostname"}},
		},
	},
	{
		Key:   "affiliate-partner-programs",
		Label: "Affiliate / Partner Programs",
		Score: 13,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"affiliate", "partners", "partner", "program", "programs", "commission", "commissions", "payout", "payouts", "bounty", "bounties", "campaign", "campaigns"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Partners", Any: []string{"partners", "partner"}},
			{Label: "Programs", Any: []string{"program", "programs"}},
			{Label: "Commissions", Any: []string{"commission", "commissions"}},
			{Label: "Payouts", Any: []string{"payout", "payouts"}},
			{Label: "Campaigns", Any: []string{"campaign", "campaigns"}},
		},
	},
	{
		Key:   "partner-portal",
		Label: "Partner Portal",
		Score: 11,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"partners.dub.co", "partner-profile", "partner-user", "partner-users", "partner-portal"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Partner Profiles", Any: []string{"partner-profile"}},
			{Label: "Partner Users", Any: []string{"partner-user", "partner-users"}},
			{Label: "Portal Routes", Any: []string{"partners.dub.co", "partner-portal"}},
		},
	},
	{
		Key:   "public-http-api-developer-platform",
		Label: "Public HTTP API & Developer Platform",
		Score: 12,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"openapi", "api/tokens", "api/oauth", "api/webhooks", "api.dub.co", "packages/cli", "/cli/", "/sdk/"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "OpenAPI", Any: []string{"openapi"}},
			{Label: "Tokens", Any: []string{"api/tokens", "tokens"}},
			{Label: "OAuth", Any: []string{"oauth"}},
			{Label: "Webhooks", Any: []string{"api/webhooks", "webhooks"}},
			{Label: "CLI / SDK", Any: []string{"packages/cli", "/cli/", "/sdk/"}},
		},
	},
	{
		Key:   "workspace-identity-access-billing",
		Label: "Workspace Identity, Access & Billing",
		Score: 10,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"billing", "plans", "members", "saml", "scim", "auth", "session", "stripe"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Workspaces", Any: []string{"workspace", "workspaces"}},
			{Label: "Billing", Any: []string{"billing", "stripe", "plans"}},
			{Label: "Members", Any: []string{"members"}},
			{Label: "SAML / SCIM", Any: []string{"saml", "scim"}},
			{Label: "Auth", Any: []string{"auth", "session"}},
		},
	},
	{
		Key:   "third-party-integrations",
		Label: "Third-Party Integrations",
		Score: 10,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"integrations", "hubspot", "segment", "shopify", "zapier", "salesforce", "slack", "stripe"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Integrations", Any: []string{"integrations"}},
			{Label: "Commerce / CRM", Any: []string{"hubspot", "shopify", "salesforce", "stripe"}},
			{Label: "Event Destinations", Any: []string{"segment", "zapier", "slack"}},
		},
	},
	{
		Key:   "background-jobs-email-automation",
		Label: "Background Jobs, Email & Automation",
		Score: 10,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"cron", "qstash", "job", "jobs", "worker", "workers", "queue", "queues", "email", "emails", "postback"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Cron", Any: []string{"cron"}},
			{Label: "Queues / Workers", Any: []string{"qstash", "queue", "queues", "worker", "workers", "job", "jobs"}},
			{Label: "Email", Any: []string{"email", "emails"}},
			{Label: "Postbacks", Any: []string{"postback"}},
		},
	},
	{
		Key:   "work-items-project-delivery",
		Label: "Work Items & Project Delivery",
		Score: 14,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"issues", "issue", "projects", "project", "estimate", "estimates"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Issues", Any: []string{"issues", "issue"}},
			{Label: "Projects", Any: []string{"projects", "project"}},
			{Label: "States", Any: []string{"state", "states"}},
			{Label: "Labels", Any: []string{"label", "labels"}},
			{Label: "Estimates", Any: []string{"estimate", "estimates"}},
		},
	},
	{
		Key:   "planning-cycles-modules-views",
		Label: "Planning: Cycles, Modules & Views",
		Score: 12,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"cycles", "cycle", "workspace-views", "rich-filters", "module-view", "/core/modules/"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Cycles", Any: []string{"cycles", "cycle"}},
			{Label: "Modules", Any: []string{"module-view", "/core/modules/"}},
			{Label: "Views", Any: []string{"workspace-views"}},
			{Label: "Rich Filters", Any: []string{"rich-filters"}},
		},
	},
	{
		Key:   "pages-stickies-collaborative-editing",
		Label: "Pages, Stickies & Collaborative Editing",
		Score: 11,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"sticky", "stickies", "hocuspocus", "yjs", "collaborative", "editor", "apps/live"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Stickies", Any: []string{"sticky", "stickies"}},
			{Label: "Editor", Any: []string{"editor"}},
			{Label: "Live Collaboration", Any: []string{"hocuspocus", "yjs", "collaborative", "apps/live"}},
		},
	},
	{
		Key:   "intake-publishing-public-space",
		Label: "Intake, Publishing & Public Space",
		Score: 10,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"intake", "deploy_board", "apps/space", "public-space", "publish", "published"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Intake", Any: []string{"intake"}},
			{Label: "Published Boards", Any: []string{"deploy_board", "publish", "published"}},
			{Label: "Space App", Any: []string{"apps/space", "public-space"}},
		},
	},
	{
		Key:   "analytics-export-reporting",
		Label: "Analytics, Export & Reporting",
		Score: 9,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"analytics", "analytic", "reporting", "reports", "data-export", "export-report"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Analytics", Any: []string{"analytics", "analytic"}},
			{Label: "Export", Any: []string{"export", "exports"}},
			{Label: "Reporting", Any: []string{"reporting", "reports"}},
		},
	},
	{
		Key:   "django-api-persistence-async-workers",
		Label: "Django API, Persistence & Async Workers",
		Score: 12,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"apps/api", "db/models", "urls.py", "bgtasks", "celery", "django"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "API App", Any: []string{"apps/api", "django"}},
			{Label: "Models", Any: []string{"db/models"}},
			{Label: "Routes", Any: []string{"urls.py"}},
			{Label: "Async Tasks", Any: []string{"bgtasks", "celery", "worker", "workers"}},
			{Label: "Migrations", Any: []string{"migrations"}},
		},
	},
	{
		Key:   "identity-auth-workspace-tenancy",
		Label: "Identity, Auth & Workspace Tenancy",
		Score: 11,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"authentication", "auth", "oauth", "session", "workspace", "workspaces", "users", "members", "invitations", "permissions", "tenancy"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Authentication", Any: []string{"authentication", "auth", "oauth", "session"}},
			{Label: "Workspaces", Any: []string{"workspace", "workspaces", "tenancy"}},
			{Label: "Users / Members", Any: []string{"users", "members", "invitations"}},
			{Label: "Permissions", Any: []string{"permissions"}},
		},
	},
	{
		Key:   "main-product-web-application",
		Label: "Main Product Web Application",
		Score: 8,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"app/(app)", "product-web", "web/app/(app)"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Web App", Any: []string{"web/app/(app)", "product-web"}},
			{Label: "App Routes", Any: []string{"app/(app)"}},
		},
	},
	{
		Key:   "instance-administration-licensing",
		Label: "Instance Administration & Licensing",
		Score: 10,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"apps/admin", "god-mode", "license", "licenses", "licensing", "instances", "instance-admin"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Admin App", Any: []string{"apps/admin"}},
			{Label: "Licensing", Any: []string{"license", "licenses", "licensing"}},
			{Label: "Instances", Any: []string{"instances", "instance-admin"}},
			{Label: "God Mode", Any: []string{"god-mode"}},
		},
	},
	{
		Key:   "self-host-runtime-deployments",
		Label: "Self-Host Runtime & Deployments",
		Score: 10,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"docker-compose", "deployments", "deployment", "helm", "kubernetes", "k8s", "swarm", "minio", "rabbitmq", "valkey", "postgres", "proxy"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Docker Compose", Any: []string{"docker-compose"}},
			{Label: "Deployments", Any: []string{"deployments", "deployment", "helm", "kubernetes", "k8s", "swarm"}},
			{Label: "Runtime Services", Any: []string{"minio", "rabbitmq", "valkey", "postgres", "proxy"}},
		},
	},
	{
		Key:   "multi-tenant-workspace-platform",
		Label: "Multi-Tenant Workspace Platform",
		Score: 12,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"workspace-datasource", "workspace-manager", "workspace-cache", "twenty-orm", "database/pg", "queue-worker", "tenant", "tenants"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Workspace Datasources", Any: []string{"workspace-datasource"}},
			{Label: "Workspace Manager", Any: []string{"workspace-manager", "workspace-cache"}},
			{Label: "Twenty ORM", Any: []string{"twenty-orm"}},
			{Label: "Database", Any: []string{"database/pg"}},
			{Label: "Queue Worker", Any: []string{"queue-worker"}},
		},
	},
	{
		Key:   "metadata-engine-data-model",
		Label: "Metadata Engine & Data Model",
		Score: 14,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"metadata-modules", "object-metadata", "field-metadata", "settings/data-model", "metadata-store", "twenty-standard-application", "flat-metadata", "data-model"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Object Metadata", Any: []string{"object-metadata"}},
			{Label: "Field Metadata", Any: []string{"field-metadata"}},
			{Label: "Data Model Settings", Any: []string{"settings/data-model", "data-model"}},
			{Label: "Metadata Store", Any: []string{"metadata-store"}},
			{Label: "Standard Application", Any: []string{"twenty-standard-application"}},
		},
	},
	{
		Key:   "crm-record-experience",
		Label: "CRM Record Experience",
		Score: 13,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"object-record", "record-table", "record-board", "record-field", "spreadsheet-import"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Object Records", Any: []string{"object-record"}},
			{Label: "Record Table", Any: []string{"record-table"}},
			{Label: "Record Board", Any: []string{"record-board"}},
			{Label: "Record Fields", Any: []string{"record-field"}},
			{Label: "Import", Any: []string{"spreadsheet-import"}},
		},
	},
	{
		Key:   "workflows-automation",
		Label: "Workflows & Automation",
		Score: 13,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"workflow", "workflows", "workflow-runner", "workflow-builder", "workflow-executor", "workflow-trigger"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Workflow Runner", Any: []string{"workflow-runner"}},
			{Label: "Workflow Builder", Any: []string{"workflow-builder"}},
			{Label: "Workflow Executor", Any: []string{"workflow-executor"}},
			{Label: "Workflow Triggers", Any: []string{"workflow-trigger"}},
		},
	},
	{
		Key:   "connected-accounts-email-calendar-timeline",
		Label: "Connected Accounts, Email, Calendar & Timeline",
		Score: 12,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"messaging", "calendar", "connected-account", "timeline", "imap", "smtp", "caldav", "mail"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Connected Accounts", Any: []string{"connected-account"}},
			{Label: "Messaging", Any: []string{"messaging", "mail", "imap", "smtp"}},
			{Label: "Calendar", Any: []string{"calendar", "caldav"}},
			{Label: "Timeline", Any: []string{"timeline"}},
		},
	},
	{
		Key:   "apps-developer-extension-platform",
		Label: "Apps & Developer Extension Platform",
		Score: 11,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"twenty-sdk", "twenty-cli", "create-twenty-app", "marketplace", "applications", "front-components", "logic-functions", "twenty-apps", "developer"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "SDK / CLI", Any: []string{"twenty-sdk", "twenty-cli", "create-twenty-app"}},
			{Label: "Marketplace", Any: []string{"marketplace"}},
			{Label: "Applications", Any: []string{"applications", "twenty-apps"}},
			{Label: "Logic Functions", Any: []string{"logic-functions"}},
			{Label: "Front Components", Any: []string{"front-components"}},
		},
	},
	{
		Key:   "ai-agents-chat-skills",
		Label: "AI Agents, Chat & Skills",
		Score: 11,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"/ai/", "ai-agent", "agents", "agent", "skill", "skills", "mcp", "tool-provider", "code-interpreter", "chat"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Agents", Any: []string{"agents", "agent", "ai-agent"}},
			{Label: "Skills", Any: []string{"skill", "skills"}},
			{Label: "MCP", Any: []string{"mcp"}},
			{Label: "Tool Providers", Any: []string{"tool-provider", "code-interpreter"}},
			{Label: "Chat", Any: []string{"chat"}},
		},
	},
	{
		Key:   "identity-auth-access-control",
		Label: "Identity, Auth & Access Control",
		Score: 11,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"auth", "sso", "two-factor", "api-key", "app-token", "user", "users", "workspace-invitation", "role", "roles", "permissions", "row-level"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Auth", Any: []string{"auth", "sso", "two-factor"}},
			{Label: "Tokens / API Keys", Any: []string{"api-key", "app-token"}},
			{Label: "Users", Any: []string{"user", "users", "workspace-invitation"}},
			{Label: "Roles / Permissions", Any: []string{"role", "roles", "permissions", "row-level"}},
		},
	},
	{
		Key:   "public-api-layer",
		Label: "Public API Layer",
		Score: 12,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"graphql", "rest-api", "open-api", "openapi", "subscriptions", "mcp"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "GraphQL", Any: []string{"graphql"}},
			{Label: "REST", Any: []string{"rest-api"}},
			{Label: "OpenAPI", Any: []string{"open-api", "openapi"}},
			{Label: "Subscriptions", Any: []string{"subscriptions"}},
			{Label: "MCP", Any: []string{"mcp"}},
		},
	},
}

var mapDynamicConceptRules = []mapConceptualBoundaryRule{
	{
		Key:   "document-signing-authoring",
		Label: "Document Signing & Authoring",
		Score: 13,
		Patterns: []mapConceptualBoundaryPattern{
			{All: []string{"document"}, Any: []string{"sign", "signing", "signature", "recipient", "recipients", "authoring", "template", "templates", "field"}},
			{Any: []string{"field-signing", "signature-pad", "document-flow", "template-flow"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Documents", Any: []string{"document", "documents"}},
			{Label: "Templates / Authoring", Any: []string{"template", "templates", "authoring"}},
			{Label: "Recipients", Any: []string{"recipient", "recipients"}},
			{Label: "Field Signing", Any: []string{"field-signing", "field", "signature"}},
			{Label: "Signing UI", Any: []string{"signature-pad", "sign"}},
		},
	},
	{
		Key:   "content-data-model",
		Label: "Content/Data Model",
		Score: 13,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"content", "collection", "collections", "relation", "relations", "data-model", "object-metadata", "field-metadata"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Collections", Any: []string{"collection", "collections"}},
			{Label: "Items", Any: []string{"item", "items"}},
			{Label: "Fields", Any: []string{"field", "fields", "field-metadata"}},
			{Label: "Relations", Any: []string{"relation", "relations"}},
			{Label: "Content", Any: []string{"content"}},
		},
	},
	{
		Key:   "extension-surfaces",
		Label: "Extension Surfaces",
		Score: 12,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"display", "displays", "layout", "layouts", "panel", "panels", "extension", "extensions"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Interfaces", Any: []string{"interface", "interfaces"}},
			{Label: "Displays", Any: []string{"display", "displays"}},
			{Label: "Layouts", Any: []string{"layout", "layouts"}},
			{Label: "Panels", Any: []string{"panel", "panels"}},
			{Label: "Extensions", Any: []string{"extension", "extensions"}},
		},
	},
	{
		Key:   "flows-automation",
		Label: "Flows & Automation",
		Score: 12,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"flow", "flows", "operation", "operations", "automation", "automations", "trigger", "triggers"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Flows", Any: []string{"flow", "flows"}},
			{Label: "Operations", Any: []string{"operation", "operations"}},
			{Label: "Automation", Any: []string{"automation", "automations", "trigger", "triggers"}},
			{Label: "Webhooks", Any: []string{"webhook", "webhooks"}},
		},
	},
	{
		Key:   "files-assets-storage",
		Label: "Files, Assets & Storage",
		Score: 11,
		Patterns: []mapConceptualBoundaryPattern{
			{Any: []string{"file", "files", "asset", "assets", "storage", "upload", "uploads", "s3", "blob", "media"}},
		},
		Covers: []mapConceptualCoverRule{
			{Label: "Files", Any: []string{"file", "files"}},
			{Label: "Assets", Any: []string{"asset", "assets", "media"}},
			{Label: "Storage", Any: []string{"storage", "s3", "blob"}},
			{Label: "Upload", Any: []string{"upload", "uploads"}},
		},
	},
}

var mapDynamicConceptPathTerms = []string{
	"document", "documents", "signing", "signature", "recipient", "recipients", "authoring", "template", "templates", "field-signing", "signature-pad", "document-flow", "template-flow",
	"content", "collection", "collections", "relation", "relations", "data-model", "object-metadata", "field-metadata",
	"interface", "interfaces", "display", "displays", "layout", "layouts", "panel", "panels", "extension", "extensions",
	"flow", "flows", "operation", "operations", "automation", "automations", "trigger", "triggers",
	"file", "files", "asset", "assets", "storage", "upload", "uploads", "blob", "media",
}

func runMap(cmd *cobra.Command, opts mapOptions) error {
	start := time.Now()
	success := false
	props := map[string]any{
		"json":       opts.JSON,
		"verbose":    opts.Verbose,
		"quiet":      opts.Quiet,
		"recent":     opts.Recent,
		"boundary":   true,
		"no_refresh": opts.NoRefresh,
		"max_areas":  opts.MaxAreas,
		"area_query": opts.AreaQuery != "",
	}
	defer func() {
		telemetry.RecordCommand("map", success, time.Since(start), props)
	}()

	if opts.MaxAreas <= 0 {
		opts.MaxAreas = mapDefaultMaxAreas
	}
	repoRoot, err := resolveRepoRoot(opts.Path)
	if err != nil {
		return err
	}
	if !opts.Recent {
		if out, ok, err := loadMapOutputCache(repoRoot, opts.MaxAreas); err != nil {
			debugLog("map output cache unavailable: %v", err)
		} else if ok {
			success = true
			props["confidence"] = out.Repo.Confidence
			props["area_count_bucket"] = telemetry.CountBucket(len(out.Areas))
			props["path_boundary"] = true
			props["cache_hit"] = true
			return writeMapOutputForOptions(cmd, out, opts)
		}
	}
	if opts.Recent {
		out := buildMapRecentOutput(cmd.Context(), repoRoot, opts)
		success = true
		props["recent_topic_count_bucket"] = telemetry.CountBucket(len(out.Topics))
		if opts.JSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}
		writeMapRecentText(cmd.OutOrStdout(), out, opts.Verbose)
		return nil
	}

	if !opts.NoRefresh {
		if err := ensureMapRepoIndexed(cmd, repoRoot, opts.Quiet || opts.JSON); err != nil {
			return err
		}
	}
	out := buildPathBoundaryMapOutput(cmd.Context(), repoRoot, opts)
	success = true
	props["confidence"] = out.Repo.Confidence
	props["area_count_bucket"] = telemetry.CountBucket(len(out.Areas))
	props["path_boundary"] = true
	if err := saveMapOutputCache(repoRoot, opts.MaxAreas, out); err != nil {
		debugLog("save map output cache failed: %v", err)
	}
	return writeMapOutputForOptions(cmd, out, opts)
}

func runRecent(cmd *cobra.Command, opts mapOptions) error {
	start := time.Now()
	success := false
	props := map[string]any{
		"json":       opts.JSON,
		"verbose":    opts.Verbose,
		"quiet":      opts.Quiet,
		"no_refresh": opts.NoRefresh,
		"max_areas":  opts.MaxAreas,
		"area_query": opts.AreaQuery != "",
	}
	defer func() {
		telemetry.RecordCommand("recent", success, time.Since(start), props)
	}()

	if opts.MaxAreas <= 0 {
		opts.MaxAreas = mapDefaultMaxAreas
	}
	repoRoot, err := resolveRepoRoot(opts.Path)
	if err != nil {
		return err
	}
	showProgress := !opts.Quiet && !opts.JSON
	if showProgress {
		fmt.Fprintln(cmd.ErrOrStderr(), "Recent progress: analyzing recent repository activity")
		if opts.Verbose {
			fmt.Fprintln(cmd.ErrOrStderr(), "Recent progress: checking local git history")
			fmt.Fprintln(cmd.ErrOrStderr(), "Recent progress: reading recent commits and path boundaries")
		}
	}
	out := buildMapRecentOutput(cmd.Context(), repoRoot, opts)
	if showProgress && opts.Verbose {
		fmt.Fprintf(cmd.ErrOrStderr(), "Recent progress: analyzed %d commit(s), matched %d topic(s)\n", out.Diagnostics.CommitsRead, len(out.Topics))
	}
	if showProgress {
		fmt.Fprintln(cmd.ErrOrStderr(), "Recent progress: complete")
	}
	success = true
	props["recent_topic_count_bucket"] = telemetry.CountBucket(len(out.Topics))
	if opts.JSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	writeMapRecentText(cmd.OutOrStdout(), out, opts.Verbose)
	return nil
}

func writeMapOutputForOptions(cmd *cobra.Command, out mapOutput, opts mapOptions) error {
	if opts.JSON {
		if opts.AreaQuery != "" {
			out = filterMapOutputByAreaQuery(out, opts.AreaQuery)
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	if opts.AreaQuery != "" {
		writeMapAreaText(cmd.OutOrStdout(), out, opts.AreaQuery, opts.Verbose)
		return nil
	}
	writeMapText(cmd.OutOrStdout(), out, opts.Verbose)
	return nil
}

func ensureMapRepoIndexed(cmd *cobra.Command, repoRoot string, quiet bool) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if quiet {
		ensureMapRepoIndexedQuiet(db, repoRoot)
		return nil
	}
	ensureRepoIndexed(cmd, db, repoRoot)
	return nil
}

func ensureMapRepoIndexedQuiet(db *store.DB, repoRoot string) {
	repoRoot = canonicalRepoRoot(repoRoot)
	if repoRoot == "" {
		return
	}
	status := freshness.Check(db, repoRoot)
	if status != nil && !status.Stale {
		debugLog("ensureMapRepoIndexedQuiet: index is fresh for %s", repoRoot)
		return
	}
	if status == nil {
		debugLog("ensureMapRepoIndexedQuiet: no repo row for %s; triggering silent auto-scan", repoRoot)
	} else {
		debugLog("ensureMapRepoIndexedQuiet: stale reason=%s; triggering silent auto-scan", status.Reason)
	}
	runScanQuiet(nil, db, repoRoot)
}

func mapRepoHasIndexedArtifacts(repoRoot string) (bool, error) {
	db, err := openDB()
	if err != nil {
		return false, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	count, err := db.CountArtifacts(store.FilterParams{RepoRoot: repoRoot})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

type cachedMapOutputFile struct {
	Schema         string    `json:"schema"`
	RepoRoot       string    `json:"repo_root"`
	LastScanCommit string    `json:"last_scan_commit,omitempty"`
	LastScanAt     string    `json:"last_scan_at,omitempty"`
	MaxAreas       int       `json:"max_areas"`
	Output         mapOutput `json:"output"`
}

func loadMapOutputCache(repoRoot string, maxAreas int) (mapOutput, bool, error) {
	db, err := openDB()
	if err != nil {
		return mapOutput{}, false, fmt.Errorf("open map output cache db: %w", err)
	}
	db.SetMaxOpenConns(1)
	meta := db.GetRepoByRoot(repoRoot)
	if meta == nil {
		_ = db.Close()
		debugLog("map output cache miss: repo not indexed")
		return mapOutput{}, false, nil
	}
	if status := freshness.Check(db, repoRoot); status == nil || status.Stale {
		_ = db.Close()
		if status == nil {
			debugLog("map output cache miss: freshness unavailable")
		} else {
			debugLog("map output cache miss: stale index (%s)", status.Reason)
		}
		return mapOutput{}, false, nil
	}
	if err := db.Close(); err != nil {
		return mapOutput{}, false, fmt.Errorf("close map output cache db: %w", err)
	}
	path, err := mapOutputCachePath(repoRoot)
	if err != nil {
		return mapOutput{}, false, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		debugLog("map output cache miss: file not found")
		return mapOutput{}, false, nil
	}
	if err != nil {
		return mapOutput{}, false, err
	}
	var cached cachedMapOutputFile
	if err := json.Unmarshal(data, &cached); err != nil {
		return mapOutput{}, false, err
	}
	if cached.Schema != mapSchemaVersion ||
		cached.RepoRoot != repoRoot ||
		!mapOutputCacheScanMetaMatches(cached, meta) ||
		cached.MaxAreas != maxAreas {
		debugLog("map output cache miss: schema/root/scan/max mismatch cached_commit=%s meta_commit=%s cached_at=%s meta_at=%s cached_max=%d requested_max=%d cached_root=%q repo_root=%q", cached.LastScanCommit, meta.LastScanCommit, cached.LastScanAt, meta.LastScanAt, cached.MaxAreas, maxAreas, cached.RepoRoot, repoRoot)
		return mapOutput{}, false, nil
	}
	if cached.Output.Schema != mapSchemaVersion || cached.Output.Repo.Path != repoRoot {
		debugLog("map output cache miss: output schema/root mismatch output_root=%q repo_root=%q", cached.Output.Repo.Path, repoRoot)
		return mapOutput{}, false, nil
	}
	debugLog("map output cache hit repo_root=%q areas=%d", repoRoot, len(cached.Output.Areas))
	return cached.Output, true, nil
}

func saveMapOutputCache(repoRoot string, maxAreas int, out mapOutput) error {
	db, err := openDB()
	if err != nil {
		return fmt.Errorf("open map output cache db: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	meta := db.GetRepoByRoot(repoRoot)
	if meta == nil {
		return nil
	}
	path, err := mapOutputCachePath(repoRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload := cachedMapOutputFile{
		Schema:         mapSchemaVersion,
		RepoRoot:       repoRoot,
		LastScanCommit: meta.LastScanCommit,
		LastScanAt:     meta.LastScanAt,
		MaxAreas:       maxAreas,
		Output:         out,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func mapOutputCachePath(repoRoot string) (string, error) {
	home, err := config.HomeDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(filepath.Clean(repoRoot)))
	return filepath.Join(home, "map-cache", fmt.Sprintf("%x.json", sum[:])), nil
}

func mapOutputCacheScanMetaMatches(cached cachedMapOutputFile, meta *store.RepoMeta) bool {
	if meta == nil {
		return false
	}
	if meta.LastScanCommit != "" || cached.LastScanCommit != "" {
		return cached.LastScanCommit == meta.LastScanCommit
	}
	return cached.LastScanAt == meta.LastScanAt
}

type cachedMapEdgeMetadata struct {
	Anchors       []cachedMapAnchor `json:"anchors"`
	PackStrength  string            `json:"pack_strength"`
	RoleMix       map[string]int    `json:"role_mix"`
	RoleFamilyMix map[string]int    `json:"role_family_mix"`
}

type cachedMapAnchor struct {
	Anchor string `json:"anchor"`
}

type cachedMapCluster struct {
	anchor       string
	confidence   float64
	evidence     int
	packStrength string
	roleMix      map[string]int
	roleFamily   map[string]int
	artifactIDs  map[string]bool
}

func buildCachedMapResult(db *store.DB, repoRoot string) (*scan.Result, bool, error) {
	meta := db.GetRepoByRoot(repoRoot)
	if meta == nil {
		return nil, false, nil
	}
	edges, err := db.GetArtifactEdges(store.ArtifactEdgeFilter{RepoID: meta.ID, EdgeType: "same_workstream_anchor"})
	if err != nil {
		return nil, false, fmt.Errorf("load map cache edges: %w", err)
	}
	if len(edges) == 0 {
		return nil, false, nil
	}

	clustersByAnchor := map[string]*cachedMapCluster{}
	artifactIDs := map[string]bool{}
	for _, edge := range edges {
		edgeMeta := cachedMapEdgeMetadata{}
		_ = json.Unmarshal([]byte(edge.MetadataJSON), &edgeMeta)
		anchors := cachedMapEdgeAnchors(edge, edgeMeta)
		for _, anchor := range anchors {
			cluster := clustersByAnchor[anchor]
			if cluster == nil {
				cluster = &cachedMapCluster{
					anchor:      anchor,
					artifactIDs: map[string]bool{},
				}
				clustersByAnchor[anchor] = cluster
			}
			cluster.confidence = maxFloat(cluster.confidence, edge.Confidence)
			cluster.evidence += maxInt(edge.EvidenceCount, 1)
			if cluster.packStrength == "" {
				cluster.packStrength = edgeMeta.PackStrength
			}
			if len(cluster.roleMix) == 0 {
				cluster.roleMix = edgeMeta.RoleMix
			}
			if len(cluster.roleFamily) == 0 {
				cluster.roleFamily = edgeMeta.RoleFamilyMix
			}
			cluster.artifactIDs[edge.SrcArtifactID] = true
			cluster.artifactIDs[edge.DstArtifactID] = true
			artifactIDs[edge.SrcArtifactID] = true
			artifactIDs[edge.DstArtifactID] = true
		}
	}
	if len(clustersByAnchor) == 0 {
		return nil, false, nil
	}

	ids := sortedMapSet(artifactIDs)
	rows, err := db.ListArtifactsByIDs(ids, store.FilterParams{RepoRoot: repoRoot})
	if err != nil {
		return nil, false, fmt.Errorf("load map cache artifacts: %w", err)
	}
	artifactByID := map[string]store.ArtifactRow{}
	for _, row := range rows {
		artifactByID[row.ID] = row
	}
	sourcePathByID := map[string]string{}
	for _, id := range ids {
		sources, err := db.GetSourcesForArtifact(id)
		if err != nil {
			return nil, false, fmt.Errorf("load map cache source %s: %w", id, err)
		}
		sourcePathByID[id] = firstCachedMapSourcePath(sources)
	}

	clusters := make([]scan.WorkstreamClusterExample, 0, len(clustersByAnchor))
	for _, cluster := range clustersByAnchor {
		examples := make([]scan.WorkstreamArtifactExample, 0, len(cluster.artifactIDs))
		for _, id := range sortedMapSet(cluster.artifactIDs) {
			row, ok := artifactByID[id]
			if !ok {
				continue
			}
			examples = append(examples, scan.WorkstreamArtifactExample{
				ID:      row.ID,
				Title:   row.Title,
				Kind:    row.Kind,
				Subtype: row.Subtype,
				Path:    sourcePathByID[id],
			})
		}
		if len(examples) == 0 {
			continue
		}
		clusters = append(clusters, scan.WorkstreamClusterExample{
			Anchor:           cluster.anchor,
			PackStrength:     cluster.packStrength,
			Confidence:       cluster.confidence,
			ConfidenceRule:   "cached_workstream_edges",
			ArtifactCount:    len(cluster.artifactIDs),
			EvidenceCount:    cluster.evidence,
			RoleMix:          cluster.roleMix,
			RoleFamilyMix:    cluster.roleFamily,
			EvidenceSources:  []string{"cached_workstream_edges"},
			ExampleArtifacts: firstWorkstreamArtifactExamples(examples, mapMaxArtifactsPerArea),
		})
	}
	if len(clusters) == 0 {
		return nil, false, nil
	}
	sort.SliceStable(clusters, func(i, j int) bool {
		if clusters[i].EvidenceCount == clusters[j].EvidenceCount {
			if clusters[i].Confidence == clusters[j].Confidence {
				return clusters[i].Anchor < clusters[j].Anchor
			}
			return clusters[i].Confidence > clusters[j].Confidence
		}
		return clusters[i].EvidenceCount > clusters[j].EvidenceCount
	})
	if len(clusters) > 10 {
		clusters = clusters[:10]
	}

	found, err := cachedMapFoundCounts(db, repoRoot)
	if err != nil {
		return nil, false, err
	}
	gitCounts, _ := db.CountGitFacts(meta.ID)
	return &scan.Result{
		Found: found,
		GitEvidence: &scan.GitEvidenceDiagnostics{
			CommitsStored: gitCounts.Commits,
			FilesStored:   gitCounts.Files,
		},
		WorkstreamEvidence: &scan.WorkstreamEvidenceDiagnostics{
			TopClusters:         clusters,
			AnchorsMaterialized: len(clusters),
		},
	}, true, nil
}

func cachedMapEdgeAnchors(edge store.ArtifactEdgeRow, meta cachedMapEdgeMetadata) []string {
	var anchors []string
	for _, anchor := range meta.Anchors {
		if value := strings.TrimSpace(anchor.Anchor); value != "" {
			anchors = appendUniqueString(anchors, value)
		}
	}
	if len(anchors) > 0 {
		return anchors
	}
	explanation := strings.TrimSpace(edge.Explanation)
	if strings.HasPrefix(explanation, "shares workstream anchor ") {
		value := strings.Trim(strings.TrimPrefix(explanation, "shares workstream anchor "), "\"")
		if value != "" {
			return []string{value}
		}
	}
	return nil
}

func firstCachedMapSourcePath(sources []store.SourceRow) string {
	for _, src := range sources {
		if strings.TrimSpace(src.Path) != "" && src.SourceType != "test_case" && src.SourceType != "code_comment" {
			return filepath.ToSlash(src.Path)
		}
	}
	for _, src := range sources {
		if strings.TrimSpace(src.Path) != "" {
			return filepath.ToSlash(src.Path)
		}
	}
	return ""
}

func firstWorkstreamArtifactExamples(values []scan.WorkstreamArtifactExample, limit int) []scan.WorkstreamArtifactExample {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func cachedMapFoundCounts(db *store.DB, repoRoot string) (map[string]int, error) {
	found := map[string]int{
		"markdown":       0,
		"openspec":       0,
		"adr":            0,
		"source_context": 0,
		"test_case":      0,
		"code_comment":   0,
	}
	for _, sourceType := range []string{"markdown", "openspec", "adr", "source_context", "test_case", "code_comment"} {
		count, err := db.CountArtifacts(store.FilterParams{RepoRoot: repoRoot, SourceType: sourceType})
		if err != nil {
			return nil, fmt.Errorf("count cached map %s artifacts: %w", sourceType, err)
		}
		found[sourceType] = count
	}
	return found, nil
}

func buildMapOutput(repoRoot string, result *scan.Result, opts mapOptions) mapOutput {
	repoName := filepath.Base(filepath.Clean(repoRoot))
	clusters := mapWorkstreamClusters(result)
	var accum []*mapAreaInternal
	for _, cluster := range clusters {
		prepared := prepareMapCluster(cluster, repoName)
		if prepared == nil {
			continue
		}
		target := findMapAreaByOverlap(accum, prepared)
		if target == nil {
			target = newMapArea(prepared)
			accum = append(accum, target)
		} else {
			mergeMapCluster(target, prepared)
		}
	}
	for _, area := range accum {
		finalizeMapArea(area)
	}
	accum = mergeDuplicateMapAreas(accum)
	for _, area := range accum {
		finalizeMapArea(area)
	}
	sort.SliceStable(accum, func(i, j int) bool {
		if mapAreaScore(accum[i]) == mapAreaScore(accum[j]) {
			return accum[i].Label < accum[j].Label
		}
		return mapAreaScore(accum[i]) > mapAreaScore(accum[j])
	})
	areas := publicMapAreas(repoRoot, repoName, accum, opts.MaxAreas, mapRepoShapeUnknown, nil)
	confidence := mapOutputConfidence(result, areas)
	caveats := mapOutputCaveats(repoRoot, result, areas, clusters, confidence)
	if confidence == mapLowConfidence {
		for i := range areas {
			if areas[i].Confidence == mapHighConfidence {
				areas[i].Confidence = mapMediumConfidence
			}
		}
	}
	return mapOutput{
		Schema: mapSchemaVersion,
		Repo: mapRepo{
			Name:       repoName,
			Path:       repoRoot,
			Confidence: confidence,
		},
		EvidenceAvailability: mapEvidence(result),
		Areas:                areas,
		Caveats:              caveats,
		Diagnostics: mapDiagnostics{
			RawClusterCount:        len(clusters),
			WorkstreamAnchorsSeen:  mapWorkstreamAnchorsSeen(result),
			WorkstreamMaterialized: mapWorkstreamAnchorsMaterialized(result),
		},
	}
}

func mapWorkstreamClusters(result *scan.Result) []scan.WorkstreamClusterExample {
	if result == nil || result.WorkstreamEvidence == nil {
		return nil
	}
	return result.WorkstreamEvidence.TopClusters
}

type mapPathBoundaryCandidate struct {
	Key               string
	Label             string
	LabelScore        float64
	Score             float64
	FileCount         int
	RecentCount       int
	PathSet           map[string]bool
	BoundaryPaths     map[string]bool
	BoundaryPathOrder []string
	Subareas          map[string]bool
	EvidenceCounts    map[string]int
	EvidenceSources   map[string]bool
	Artifacts         []mapArtifact
	TraceReceipts     []mapTraceReceipt
}

func buildPathBoundaryMapOutput(ctx context.Context, repoRoot string, opts mapOptions) mapOutput {
	repoName := filepath.Base(filepath.Clean(repoRoot))
	files, source, limited, fileErr := listMapBoundaryFiles(ctx, repoRoot)
	recentCommits := mapBoundaryRecentCommits(ctx, repoRoot)
	packability, packabilityErr := loadMapPackabilityIndex(repoRoot)
	if packabilityErr != nil {
		debugLog("map packability index unavailable: %v", packabilityErr)
	}
	if opts.NoRefresh && packability == nil && len(files) > 0 {
		packability = mapPackabilityIndexFromFiles(files)
	}
	areas, evidence, rawCandidateCount := buildPathBoundaryAreas(repoRoot, repoName, files, recentCommits, opts.MaxAreas, packability)
	confidence := mapBoundaryOutputConfidence(areas)
	caveats := []string{"path-primary boundary map; git/docs/tests boost boundaries but do not define them"}
	if source == "walk" {
		caveats = append(caveats, "git file manifest unavailable; used filesystem walk")
	}
	if limited {
		caveats = append(caveats, fmt.Sprintf("file manifest capped at %d tracked paths", mapBoundaryMaxFiles))
	}
	if fileErr != nil {
		caveats = append(caveats, "file manifest warning: "+fileErr.Error())
	}
	if len(files) == 0 {
		caveats = append(caveats, "no eligible tracked files found for path-boundary mapping")
	}
	if len(areas) == 0 {
		caveats = append(caveats, "no path-primary boundaries passed the display threshold")
	}
	if strings.Contains(filepath.ToSlash(repoRoot), "/_ignore/") {
		caveats = append(caveats, "repo path is under _ignore; use full checkouts for promotion claims")
	}
	if opts.NoRefresh {
		if indexed, err := mapRepoHasIndexedArtifacts(repoRoot); err == nil && !indexed {
			caveats = append(caveats, mapIndexRequiredCaveat)
		} else if err != nil {
			debugLog("map boundary index availability unavailable: %v", err)
		}
	}
	return mapOutput{
		Schema: mapSchemaVersion,
		Repo: mapRepo{
			Name:       repoName,
			Path:       repoRoot,
			Confidence: confidence,
		},
		EvidenceAvailability: evidence,
		Areas:                areas,
		Caveats:              caveats,
		Diagnostics: mapDiagnostics{
			RawClusterCount:       rawCandidateCount,
			WorkstreamAnchorsSeen: len(recentCommits),
		},
	}
}

func listMapBoundaryFiles(ctx context.Context, repoRoot string) ([]string, string, bool, error) {
	gitCtx, cancel := context.WithTimeout(ctx, mapBoundaryFilesTimeout)
	defer cancel()
	if findGitRepoAvailable(gitCtx, repoRoot) {
		files, limited, err := listMapBoundaryGitFiles(gitCtx, repoRoot)
		if err == nil {
			return files, "git", limited, nil
		}
		walkFiles, walkLimited, walkErr := listMapBoundaryWalkFiles(ctx, repoRoot)
		if walkErr != nil {
			return walkFiles, "walk", walkLimited, fmt.Errorf("git ls-files failed: %v; walk failed: %w", err, walkErr)
		}
		return walkFiles, "walk", walkLimited, fmt.Errorf("git ls-files failed: %w", err)
	}
	files, limited, err := listMapBoundaryWalkFiles(ctx, repoRoot)
	return files, "walk", limited, err
}

func listMapBoundaryGitFiles(ctx context.Context, repoRoot string) ([]string, bool, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", filepath.Clean(repoRoot), "ls-files", "-z").Output()
	if err != nil {
		return nil, false, err
	}
	var files []string
	limited := false
	for _, raw := range strings.Split(string(out), "\x00") {
		path := normalizeMapPath(raw)
		if !mapBoundaryPathEligible(path) {
			continue
		}
		files = append(files, path)
		if len(files) >= mapBoundaryMaxFiles {
			limited = true
			break
		}
	}
	return files, limited, nil
}

func listMapBoundaryWalkFiles(ctx context.Context, repoRoot string) ([]string, bool, error) {
	var files []string
	limited := false
	err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		name := strings.ToLower(d.Name())
		if d.IsDir() {
			if path != repoRoot && mapSuppressedPathSegments[name] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}
		normalized := normalizeMapPath(rel)
		if !mapBoundaryPathEligible(normalized) {
			return nil
		}
		files = append(files, normalized)
		if len(files) >= mapBoundaryMaxFiles {
			limited = true
			return filepath.SkipAll
		}
		return nil
	})
	return files, limited, err
}

func mapBoundaryRecentCommits(ctx context.Context, repoRoot string) []parsedFindGitCommit {
	gitCtx, cancel := context.WithTimeout(ctx, findGitReceiptTimeout)
	defer cancel()
	if !findGitRepoAvailable(gitCtx, repoRoot) {
		return nil
	}
	commits, ok := findGitLogRecent(gitCtx, repoRoot, mapBoundaryMaxCommits)
	if !ok {
		return nil
	}
	var out []parsedFindGitCommit
	for _, commit := range commits {
		if mapRecentCommitNoisy(commit) {
			continue
		}
		out = append(out, commit)
	}
	return out
}

func annotateMapAreas(areas []mapArea) {
	for i := range areas {
		areas[i].Purpose = mapAreaPurposeText(areas[i])
		areas[i].BoundaryPaths = mapAreaBoundaryPaths(areas[i])
	}
	for i := range areas {
		areas[i].AdjacentSystems = mapAdjacentSystemLabels(areas[i], areas)
	}
}

func mapAreaPurposeText(area mapArea) string {
	label := strings.TrimSpace(area.Label)
	if label == "" {
		label = "this subsystem"
	}
	areaType := displayMapAreaType(area.AreaType)
	if areaType == "" || areaType == "unknown area" {
		areaType = "repo capability"
	}
	switch area.BoundaryRole {
	case mapBoundaryRoleHorizontalLayer:
		return fmt.Sprintf("Provides shared %s behavior used across the repo.", areaType)
	case mapBoundaryRoleExtensionEcosystem:
		return fmt.Sprintf("Groups extension and integration behavior around %s.", label)
	case mapBoundaryRoleFixtureOrTestbed:
		return fmt.Sprintf("Holds fixture, harness, or testbed behavior for %s.", label)
	case mapBoundaryRoleDocsReference:
		return fmt.Sprintf("Documents %s decisions, usage, or handoff context.", label)
	case mapBoundaryRoleGenericParent:
		return fmt.Sprintf("Groups related %s paths under %s.", areaType, label)
	case mapBoundaryRoleRepoNamespace:
		return fmt.Sprintf("Represents a broad repo namespace for %s.", label)
	case mapBoundaryRoleHandoffUnsafe:
		return fmt.Sprintf("Looks like a broad or shell-like boundary; inspect before handing off %s work.", label)
	default:
		return fmt.Sprintf("Owns %s behavior for %s.", areaType, label)
	}
}

func mapAreaBoundaryPaths(area mapArea) []string {
	var paths []string
	for _, anchor := range area.Diagnostics.RawAnchors {
		anchor = normalizeMapPath(anchor)
		if !strings.Contains(anchor, "/") {
			continue
		}
		paths = appendUniqueString(paths, mapBoundaryDisplayPath(anchor))
		if len(paths) >= 4 {
			return paths
		}
	}
	for _, path := range area.KeyPaths {
		dir := normalizeMapPath(filepath.ToSlash(filepath.Dir(path)))
		if dir == "." || dir == "" {
			continue
		}
		paths = appendUniqueString(paths, mapBoundaryDisplayPath(dir))
		if len(paths) >= 4 {
			break
		}
	}
	return paths
}

func mapBoundaryDisplayPath(path string) string {
	path = strings.Trim(strings.TrimSpace(filepath.ToSlash(path)), "/")
	if path == "" {
		return path
	}
	if strings.HasSuffix(path, "/**") {
		return path
	}
	if strings.Contains(path, ".") && filepath.Ext(path) != "" {
		return path
	}
	return path + "/**"
}

func mapAdjacentSystemLabels(primary mapArea, areas []mapArea) []string {
	var labels []string
	for _, area := range relatedMapAreas(primary, areas) {
		if area.Label != "" {
			labels = appendUniqueString(labels, area.Label)
		}
	}
	return firstStrings(labels, 3)
}

func buildPathBoundaryAreas(repoRoot, repoName string, files []string, commits []parsedFindGitCommit, maxAreas int, packabilityArg ...*mapPackabilityIndex) ([]mapArea, mapEvidenceAvailability, int) {
	var packability *mapPackabilityIndex
	if len(packabilityArg) > 0 {
		packability = packabilityArg[0]
	}
	candidates := map[string]*mapPathBoundaryCandidate{}
	evidence := mapEvidenceAvailability{}
	repoShape := inferMapRepoShape(repoName, files)
	for _, path := range files {
		family := mapBoundaryPathFamily(path)
		if family == "" {
			continue
		}
		switch family {
		case "source":
			evidence.Source++
		case "test":
			evidence.Test++
		case "doc":
			evidence.Markdown++
		case "config":
			evidence.Source++
		}
		addMapBoundaryPathCandidates(candidates, repoName, path, family)
	}
	applyMapBoundaryConceptualParents(candidates, repoName, files, repoShape)
	applyMapBoundaryDynamicConceptParents(candidates, repoName, files, commits, repoShape)
	applyMapBoundaryImportEvidence(repoRoot, repoName, files, candidates)
	applyMapBoundaryRecentCommits(candidates, repoName, commits)
	if len(commits) > 0 {
		evidence.Trace = true
	}
	var internals []*mapAreaInternal
	for _, candidate := range candidates {
		if !mapBoundaryCandidateDisplayable(candidate) {
			continue
		}
		internals = append(internals, mapBoundaryAreaInternal(candidate))
	}
	sort.SliceStable(internals, func(i, j int) bool {
		left := mapBoundaryAreaScore(internals[i], repoName, repoShape)
		right := mapBoundaryAreaScore(internals[j], repoName, repoShape)
		if left == right {
			return internals[i].Label < internals[j].Label
		}
		return left > right
	})
	internals = selectMapBoundaryAreas(internals, maxAreas*2, repoShape)
	areas := publicMapAreas(repoRoot, repoName, internals, maxAreas, repoShape, packability)
	return areas, evidence, len(candidates)
}

func addMapBoundaryPathCandidates(candidates map[string]*mapPathBoundaryCandidate, repoName, path, family string) {
	seenForPath := map[string]bool{}
	for _, labelCandidate := range mapBoundaryLabelCandidatesForPath(path, repoName) {
		if seenForPath[labelCandidate.Key] {
			continue
		}
		seenForPath[labelCandidate.Key] = true
		candidate := candidates[labelCandidate.Key]
		if candidate == nil {
			candidate = &mapPathBoundaryCandidate{
				Key:             labelCandidate.Key,
				Label:           labelCandidate.Label,
				PathSet:         map[string]bool{},
				BoundaryPaths:   map[string]bool{},
				Subareas:        map[string]bool{},
				EvidenceCounts:  map[string]int{},
				EvidenceSources: map[string]bool{"path_boundary": true},
			}
			candidates[labelCandidate.Key] = candidate
		}
		candidate.LabelScore += labelCandidate.Score
		candidate.Score += labelCandidate.Score * 0.2
		if !candidate.PathSet[path] {
			candidate.PathSet[path] = true
			candidate.FileCount++
			candidate.EvidenceCounts[family]++
			candidate.Score += mapBoundaryFamilyScore(family)
			art := mapArtifactForBoundaryPath(path, family)
			art.Rank = mapPathBoundaryArtifactRank(labelCandidate.Key, path, family)
			appendMapBoundaryArtifactCandidate(candidate, art, mapBoundaryMaxArtifacts*3)
		}
		for _, boundaryPath := range mapBoundaryContainingDirs(path, labelCandidate.Key) {
			appendMapBoundaryPathCandidate(candidate, boundaryPath)
		}
		for _, subarea := range mapBoundarySubareasForCandidate(path, labelCandidate.Key) {
			if len(candidate.Subareas) < 20 {
				candidate.Subareas[subarea] = true
			}
		}
	}
}

func inferMapRepoShape(repoName string, files []string) string {
	scores := map[string]int{}
	sourceCount := 0
	docCount := 0
	for _, raw := range files {
		pathValue := normalizeMapPath(raw)
		if pathValue == "" {
			continue
		}
		family := mapBoundaryPathFamily(pathValue)
		if family == "doc" {
			docCount++
		} else if family == "source" || family == "test" || family == "config" {
			sourceCount++
		}
		key := normalizeMapKey(pathValue)
		toolPathContext := strings.HasPrefix(pathValue, "crates/") ||
			strings.HasPrefix(pathValue, "cmd/") ||
			strings.Contains(pathValue, "/cmd/") ||
			strings.Contains(pathValue, "/commands/") ||
			strings.Contains(pathValue, "/cli/") ||
			strings.Contains(key, "-cli-") ||
			strings.HasSuffix(key, "-cli") ||
			strings.Contains(pathValue, "docs/pip/") ||
			strings.Contains(pathValue, "docs/concepts/projects/")
		switch {
		case strings.HasPrefix(pathValue, "crates/"):
			scores[mapRepoShapeTool] += 2
		case strings.HasPrefix(pathValue, "cmd/"):
			scores[mapRepoShapeTool] += 2
		case strings.Contains(pathValue, "/cmd/") || strings.Contains(pathValue, "/commands/"):
			scores[mapRepoShapeTool] += 2
		case strings.Contains(pathValue, "/cli/") || strings.Contains(key, "-cli-") || strings.HasSuffix(key, "-cli"):
			scores[mapRepoShapeTool] += 2
		}
		if toolPathContext && mapAnyContains([]string{pathValue},
			"resolver", "installer", "virtualenv", "venv", "lockfile", "uv-pip", "uv-tool",
			"publish", "audit", "registry", "pypi", "interpreter", "tool_install", "pip_install") {
			scores[mapRepoShapeTool] += 2
		}
		if mapAnyContains([]string{pathValue},
			"app/controllers", "app/models", "app/jobs", "app/views", "db/migrate", "config/routes",
			"apps/web", "apps/api", "pages", "routes", "server/controllers",
			"packages/desktop-client/src", "packages/mobile/src", "app/javascript", "app/views",
			"packages/console/src", "packages/account/src", "web/src/components") {
			scores[mapRepoShapeWebApp] += 2
		}
		if mapAnyContains([]string{pathValue}, "src/components", "src/pages", "src/routes") {
			scores[mapRepoShapeWebApp]++
		}
		if mapAnyContains([]string{pathValue},
			"packages/modules", "packages/core/framework", "packages/core/core-flows", "workflows-sdk",
			"orchestration", "modules-sdk", "packages/medusa/src/api", "packages/admin/dashboard",
			"packages/modules/providers", "packages/payload/src", "packages/ui/src/admin", "packages/richtext") {
			scores[mapRepoShapePlatform] += 3
		}
		if strings.HasPrefix(pathValue, "www/") || strings.Contains(pathValue, "/docs/") || strings.HasPrefix(pathValue, "docs/") {
			scores[mapRepoShapeDocsSite]++
		}
	}
	if scores[mapRepoShapePlatform] >= 9 && scores[mapRepoShapePlatform] >= scores[mapRepoShapeTool] {
		return mapRepoShapePlatform
	}
	if scores[mapRepoShapeWebApp] >= 8 && scores[mapRepoShapeWebApp] >= scores[mapRepoShapeTool]/2 {
		return mapRepoShapeWebApp
	}
	if scores[mapRepoShapeTool] >= 8 && scores[mapRepoShapeTool] >= scores[mapRepoShapePlatform]+4 {
		return mapRepoShapeTool
	}
	if scores[mapRepoShapeWebApp] >= 8 {
		return mapRepoShapeWebApp
	}
	if scores[mapRepoShapePlatform] >= 6 {
		return mapRepoShapePlatform
	}
	if scores[mapRepoShapeTool] >= 5 {
		return mapRepoShapeTool
	}
	if docCount > 0 && docCount >= sourceCount*2 {
		return mapRepoShapeDocsSite
	}
	if sourceCount > 0 {
		return mapRepoShapeLibrary
	}
	return mapRepoShapeUnknown
}

func applyMapBoundaryConceptualParents(candidates map[string]*mapPathBoundaryCandidate, repoName string, files []string, repoShape string) {
	needleCache := map[string]mapConceptualNeedle{}
	for _, path := range mapConceptualParentFiles(files) {
		family := mapBoundaryPathFamily(path)
		if family == "" {
			continue
		}
		pathValue := normalizeMapPath(path)
		if mapConceptualPathNoisy(pathValue) {
			continue
		}
		pathKey := normalizeMapKey(pathValue)
		for _, rule := range mapConceptualBoundaryRules {
			if !mapConceptualRuleAllowedForShape(rule, repoShape) {
				continue
			}
			if !mapConceptualRuleEvidenceAllowed(rule, pathValue, pathKey) {
				continue
			}
			if !mapConceptualRuleMatchesPath(rule, pathValue, pathKey, needleCache) {
				continue
			}
			addMapConceptualParentCandidate(candidates, repoName, path, pathValue, pathKey, family, rule, needleCache)
		}
	}
}

func applyMapBoundaryDynamicConceptParents(candidates map[string]*mapPathBoundaryCandidate, repoName string, files []string, commits []parsedFindGitCommit, repoShape string) {
	needleCache := map[string]mapConceptualNeedle{}
	for _, path := range mapConceptualParentFiles(files) {
		family := mapBoundaryPathFamily(path)
		if family == "" {
			continue
		}
		pathValue := normalizeMapPath(path)
		if mapConceptualPathNoisy(pathValue) {
			continue
		}
		pathKey := normalizeMapKey(pathValue)
		if !mapDynamicConceptPathMightMatch(pathKey) {
			continue
		}
		for _, rule := range mapDynamicConceptRules {
			if !mapDynamicConceptRuleAllowedForShape(rule, repoShape) {
				continue
			}
			if !mapConceptualRuleEvidenceAllowed(rule, pathValue, pathKey) {
				continue
			}
			if !mapConceptualRuleMatchesPath(rule, pathValue, pathKey, needleCache) {
				continue
			}
			addMapConceptualParentCandidate(candidates, repoName, path, pathValue, pathKey, family, rule, needleCache)
			if candidate := candidates[normalizeMapKey(rule.Key)]; candidate != nil {
				candidate.EvidenceSources["dynamic_parent"] = true
			}
		}
	}
	for _, commit := range commits {
		if mapCommitNoisy(commit) {
			continue
		}
		subjectKey := normalizeMapKey(stripMapCommitPrefix(commit.subject))
		if subjectKey == "" {
			continue
		}
		for _, path := range commit.paths {
			pathValue := normalizeMapPath(path)
			family := mapBoundaryPathFamily(pathValue)
			if family == "" || mapConceptualPathNoisy(pathValue) {
				continue
			}
			combinedKey := normalizeMapKey(pathValue + " " + subjectKey)
			if !mapDynamicConceptPathMightMatch(combinedKey) {
				continue
			}
			for _, rule := range mapDynamicConceptRules {
				if !mapDynamicConceptRuleAllowedForShape(rule, repoShape) {
					continue
				}
				if !mapConceptualRuleMatchesPath(rule, pathValue, combinedKey, needleCache) {
					continue
				}
				addMapConceptualParentCandidate(candidates, repoName, path, pathValue, combinedKey, family, rule, needleCache)
				if candidate := candidates[normalizeMapKey(rule.Key)]; candidate != nil {
					candidate.EvidenceSources["dynamic_parent"] = true
					candidate.EvidenceSources["git"] = true
					candidate.EvidenceCounts["trace"]++
				}
			}
		}
	}
}

func mapDynamicConceptPathMightMatch(pathKey string) bool {
	if pathKey == "" {
		return false
	}
	for _, term := range mapDynamicConceptPathTerms {
		if strings.Contains(pathKey, term) {
			return true
		}
	}
	return false
}

func mapDynamicConceptRuleAllowedForShape(rule mapConceptualBoundaryRule, repoShape string) bool {
	if len(rule.Shapes) > 0 {
		return mapConceptualRuleAllowedForShape(rule, repoShape)
	}
	switch repoShape {
	case mapRepoShapeTool, mapRepoShapeDocsSite:
		return false
	default:
		return true
	}
}

func mapConceptualRuleAllowedForShape(rule mapConceptualBoundaryRule, repoShape string) bool {
	if len(rule.Shapes) > 0 {
		for _, shape := range rule.Shapes {
			if shape == repoShape {
				return true
			}
		}
		return false
	}
	switch normalizeMapKey(rule.Key) {
	case "django-api-persistence-async-workers":
		return repoShape == mapRepoShapeWebApp
	}
	switch repoShape {
	case mapRepoShapeTool:
		return false
	default:
		return true
	}
}

func mapConceptualRuleEvidenceAllowed(rule mapConceptualBoundaryRule, pathValue, pathKey string) bool {
	switch normalizeMapKey(rule.Key) {
	case "django-api-persistence-async-workers":
		return strings.HasSuffix(pathValue, ".py") ||
			strings.Contains(pathValue, "urls.py") ||
			strings.Contains(pathValue, "celery") ||
			strings.Contains(pathValue, "bgtasks") ||
			strings.Contains(pathValue, "django") ||
			strings.Contains(pathValue, "/db/models/") ||
			strings.Contains(pathKey, "db-models")
	default:
		return true
	}
}

func addMapConceptualParentCandidate(candidates map[string]*mapPathBoundaryCandidate, repoName, filePath, pathValue, pathKey, family string, rule mapConceptualBoundaryRule, needleCache map[string]mapConceptualNeedle) {
	key := normalizeMapKey(rule.Key)
	if key == "" || mapKeyMatchesRepoRoot(key, normalizeMapKey(repoName)) {
		return
	}
	candidate := candidates[key]
	if candidate == nil {
		candidate = &mapPathBoundaryCandidate{
			Key:             key,
			Label:           firstNonEmpty(rule.Label, displayMapLabel(key)),
			PathSet:         map[string]bool{},
			BoundaryPaths:   map[string]bool{},
			Subareas:        map[string]bool{},
			EvidenceCounts:  map[string]int{},
			EvidenceSources: map[string]bool{"path_boundary": true, "conceptual_parent": true},
		}
		candidate.LabelScore += rule.Score
		candidate.Score += rule.Score
		candidates[key] = candidate
	}
	if !candidate.PathSet[filePath] {
		candidate.PathSet[filePath] = true
		candidate.FileCount++
		candidate.EvidenceCounts[family]++
		candidate.Score += mapBoundaryFamilyScore(family)*1.6 + rule.Score*0.22
		candidate.LabelScore += rule.Score * 0.08
		art := mapArtifactForBoundaryPath(filePath, family)
		art.Rank = mapConceptualArtifactRank(rule, pathValue, pathKey, family, needleCache)
		appendMapBoundaryArtifactCandidate(candidate, art, mapBoundaryMaxArtifacts*3)
	}
	if dir := pathpkg.Dir(normalizeMapPath(filePath)); dir != "." && dir != "" {
		appendMapBoundaryPathCandidate(candidate, dir)
	}
	for _, cover := range mapConceptualCoversForPath(rule, pathValue, pathKey, needleCache) {
		if len(candidate.Subareas) < 20 {
			candidate.Subareas[cover] = true
		}
	}
}

func mapConceptualParentFiles(files []string) []string {
	if len(files) <= mapBoundaryMaxConceptualFiles {
		return files
	}
	var selected []string
	for _, path := range files {
		pathValue := normalizeMapPath(path)
		if mapConceptualPathNoisy(pathValue) || mapPathLooksDocExample(pathValue) {
			continue
		}
		family := mapBoundaryPathFamily(pathValue)
		if family == "source" || family == "test" {
			selected = append(selected, path)
			if len(selected) >= mapBoundaryMaxConceptualFiles {
				return selected
			}
		}
	}
	for _, path := range files {
		pathValue := normalizeMapPath(path)
		if mapConceptualPathNoisy(pathValue) {
			continue
		}
		selected = append(selected, path)
		if len(selected) >= mapBoundaryMaxConceptualFiles {
			return selected
		}
	}
	return selected
}

func appendMapBoundaryArtifactCandidate(candidate *mapPathBoundaryCandidate, art mapArtifact, limit int) {
	if art.Path == "" || limit <= 0 {
		return
	}
	for i := range candidate.Artifacts {
		if candidate.Artifacts[i].Path == art.Path {
			if art.Rank > candidate.Artifacts[i].Rank {
				candidate.Artifacts[i].Rank = art.Rank
			}
			return
		}
	}
	if len(candidate.Artifacts) < limit {
		candidate.Artifacts = append(candidate.Artifacts, art)
		return
	}
	lowestIndex := -1
	lowestRank := art.Rank
	for i, existing := range candidate.Artifacts {
		if lowestIndex < 0 || existing.Rank < lowestRank {
			lowestIndex = i
			lowestRank = existing.Rank
		}
	}
	if lowestIndex >= 0 && art.Rank > lowestRank {
		candidate.Artifacts[lowestIndex] = art
	}
}

func mapConceptualArtifactRank(rule mapConceptualBoundaryRule, pathValue, pathKey, family string, needleCache map[string]mapConceptualNeedle) int {
	score := 0
	switch family {
	case "source":
		score += 500
	case "test":
		score += 460
	case "config":
		score += 130
	case "doc":
		score += 90
	default:
		score += 40
	}
	for _, pattern := range rule.Patterns {
		for _, needle := range pattern.All {
			if mapConceptualPathContains(pathValue, pathKey, needle, needleCache) {
				score += 60
			}
		}
		for _, needle := range pattern.Any {
			if mapConceptualPathContains(pathValue, pathKey, needle, needleCache) {
				score += 45
			}
		}
	}
	for _, cover := range rule.Covers {
		if mapConceptualPatternMatchesPath(mapConceptualBoundaryPattern{All: cover.All, Any: cover.Any}, pathValue, pathKey, needleCache) {
			score += 35
		}
	}
	switch {
	case strings.HasPrefix(pathValue, "test/") || strings.HasPrefix(pathValue, "tests/") || strings.Contains(pathValue, "/test/") || strings.Contains(pathValue, "/tests/"):
		score -= 260
	case strings.Contains(pathValue, "/docs/") || strings.HasPrefix(pathValue, "docs/") || strings.Contains(pathValue, "/twenty-docs/"):
		score -= 260
	case strings.HasPrefix(pathValue, "examples/") || strings.Contains(pathValue, "/examples/") || strings.Contains(pathValue, "/fixtures/"):
		score -= 520
	case strings.Contains(pathValue, "/templates/") || strings.Contains(pathValue, "/template/"):
		score -= 280
	case strings.Contains(pathValue, "/connector/") || strings.Contains(pathValue, "-connector/"):
		score -= 110
	}
	return score
}

func mapConceptualRuleMatchesPath(rule mapConceptualBoundaryRule, pathValue, pathKey string, needleCache map[string]mapConceptualNeedle) bool {
	for _, pattern := range rule.Patterns {
		if mapConceptualPatternMatchesPath(pattern, pathValue, pathKey, needleCache) {
			return true
		}
	}
	return false
}

func mapConceptualPatternMatchesPath(pattern mapConceptualBoundaryPattern, pathValue, pathKey string, needleCache map[string]mapConceptualNeedle) bool {
	for _, needle := range pattern.All {
		if !mapConceptualPathContains(pathValue, pathKey, needle, needleCache) {
			return false
		}
	}
	if len(pattern.Any) == 0 {
		return len(pattern.All) > 0
	}
	for _, needle := range pattern.Any {
		if mapConceptualPathContains(pathValue, pathKey, needle, needleCache) {
			return true
		}
	}
	return false
}

func mapConceptualCoversForPath(rule mapConceptualBoundaryRule, pathValue, pathKey string, needleCache map[string]mapConceptualNeedle) []string {
	var out []string
	for _, cover := range rule.Covers {
		if mapConceptualPatternMatchesPath(mapConceptualBoundaryPattern{All: cover.All, Any: cover.Any}, pathValue, pathKey, needleCache) {
			out = appendUniqueString(out, cover.Label)
		}
	}
	return firstStrings(out, 3)
}

func mapConceptualPathNoisy(pathValue string) bool {
	pathValue = normalizeMapPath(pathValue)
	return strings.HasPrefix(pathValue, ".github/") ||
		strings.Contains(pathValue, "/.github/") ||
		strings.Contains(pathValue, "/github/workflows/")
}

func mapConceptualPathContains(pathValue, pathKey, rawNeedle string, needleCache map[string]mapConceptualNeedle) bool {
	needle := mapConceptualNeedleFor(rawNeedle, needleCache)
	if needle.path == "" && needle.key == "" {
		return false
	}
	if needle.hasSlash {
		if needle.path == "" {
			return false
		}
		if needle.segmentSeq {
			return mapPathHasSegmentSequence(pathValue, needle.path)
		}
		return strings.Contains(pathValue, needle.path)
	}
	if needle.dotted && needle.path != "" {
		return strings.Contains(pathValue, needle.path)
	}
	if needle.key == "" {
		return false
	}
	if len(needle.key) <= 2 {
		return false
	}
	return scoreMapKeyMatch(pathKey, needle.key) > 0
}

func mapConceptualNeedleFor(value string, cache map[string]mapConceptualNeedle) mapConceptualNeedle {
	if cached, ok := cache[value]; ok {
		return cached
	}
	raw := strings.ToLower(strings.TrimSpace(filepath.ToSlash(value)))
	needle := mapConceptualNeedle{
		raw:        raw,
		path:       normalizeMapPath(raw),
		key:        normalizeMapKey(raw),
		hasSlash:   strings.Contains(raw, "/"),
		segmentSeq: strings.HasPrefix(raw, "/") && strings.HasSuffix(raw, "/"),
		dotted:     strings.Contains(raw, "."),
	}
	cache[value] = needle
	return needle
}

func mapPathHasSegmentSequence(pathValue, needlePath string) bool {
	parts := strings.Split(normalizeMapPath(pathValue), "/")
	needleParts := strings.Split(normalizeMapPath(needlePath), "/")
	if len(parts) == 0 || len(needleParts) == 0 || len(needleParts) > len(parts) {
		return false
	}
	for i := 0; i+len(needleParts) <= len(parts); i++ {
		matched := true
		for j, needlePart := range needleParts {
			if parts[i+j] != needlePart {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func applyMapBoundaryImportEvidence(repoRoot, repoName string, files []string, candidates map[string]*mapPathBoundaryCandidate) {
	if len(files) == 0 || len(candidates) == 0 {
		return
	}
	fileSet := map[string]bool{}
	for _, path := range files {
		fileSet[normalizeMapPath(path)] = true
	}
	suffixIndex := buildMapBoundarySuffixIndex(files)
	pathKeys := map[string][]string{}
	for _, path := range files {
		family := mapBoundaryPathFamily(path)
		if family != "source" && family != "test" {
			continue
		}
		keys := mapBoundaryCandidateKeysForPath(path, repoName)
		if len(keys) > 0 {
			pathKeys[path] = keys
		}
	}
	sourceRead := 0
	importFileLimit := mapBoundaryImportFileLimit(len(files))
	for _, sourcePath := range files {
		if sourceRead >= importFileLimit {
			break
		}
		family := mapBoundaryPathFamily(sourcePath)
		if family != "source" && family != "test" {
			continue
		}
		if len(pathKeys[sourcePath]) == 0 {
			continue
		}
		body := readMapBoundaryImportBody(filepath.Join(repoRoot, filepath.FromSlash(sourcePath)))
		if body == "" {
			continue
		}
		sourceRead++
		for _, spec := range extractMapBoundaryImports(sourcePath, body) {
			targetPath := resolveMapBoundaryImport(sourcePath, spec, fileSet, suffixIndex)
			if targetPath == "" || targetPath == sourcePath || !fileSet[targetPath] {
				continue
			}
			addMapBoundaryImportEdge(candidates, pathKeys, sourcePath, targetPath)
		}
	}
}

func mapBoundaryImportFileLimit(fileCount int) int {
	if fileCount >= mapBoundaryLargeRepoFiles {
		return mapBoundaryMaxImportFilesLargeRepo
	}
	return mapBoundaryMaxImportFiles
}

func mapBoundaryCandidateKeysForPath(path, repoName string) []string {
	var out []string
	for _, candidate := range mapBoundaryLabelCandidatesForPath(path, repoName) {
		out = appendUniqueString(out, candidate.Key)
	}
	return out
}

func addMapBoundaryImportEdge(candidates map[string]*mapPathBoundaryCandidate, pathKeys map[string][]string, sourcePath, targetPath string) {
	sourceKeys := pathKeys[sourcePath]
	targetKeys := pathKeys[targetPath]
	if len(sourceKeys) == 0 || len(targetKeys) == 0 {
		return
	}
	sourceKeySet := mapStringSet(sourceKeys)
	sourceFamily := mapBoundaryPathFamily(sourcePath)
	targetFamily := mapBoundaryPathFamily(targetPath)
	for _, key := range targetKeys {
		if !sourceKeySet[key] {
			continue
		}
		candidate := candidates[key]
		if candidate == nil {
			continue
		}
		candidate.EvidenceCounts["import"]++
		candidate.EvidenceSources["import"] = true
		if candidate.EvidenceCounts["import"] <= mapBoundaryImportScoreCap {
			candidate.Score += 0.65
		}
		if sourceFamily == "test" && targetFamily == "source" {
			candidate.EvidenceCounts["test_import"]++
			if candidate.EvidenceCounts["test_import"] <= mapBoundaryTestImportScoreCap {
				candidate.Score += 1.4
			}
		}
	}
}

func readMapBoundaryImportBody(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > mapBoundaryMaxImportBytes {
		return ""
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(body)
}

func extractMapBoundaryImports(path, body string) []string {
	ext := strings.ToLower(filepath.Ext(path))
	var specs []string
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue", ".svelte":
		for _, re := range mapBoundaryJSImportRegexes {
			specs = appendMapBoundaryRegexSpecs(specs, re, body)
		}
	case ".py":
		specs = appendMapBoundaryRegexSpecs(specs, mapBoundaryPythonFromImportRegex, body)
		specs = appendMapBoundaryRegexSpecs(specs, mapBoundaryPythonImportRegex, body)
	case ".go":
		specs = appendMapBoundaryRegexSpecs(specs, mapBoundaryGoImportRegex, body)
		specs = appendMapBoundaryRegexSpecs(specs, mapBoundaryGoImportBlockRegex, body)
	case ".rs":
		specs = appendMapBoundaryRegexSpecs(specs, mapBoundaryRustUseRegex, body)
		specs = appendMapBoundaryRegexSpecs(specs, mapBoundaryRustModRegex, body)
	case ".java", ".kt", ".kts", ".cs":
		specs = appendMapBoundaryRegexSpecs(specs, mapBoundaryJavaLikeImportRegex, body)
	default:
		return nil
	}
	var out []string
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" || !mapBoundaryLocalImportSpec(spec) {
			continue
		}
		out = appendUniqueString(out, spec)
	}
	return out
}

func appendMapBoundaryRegexSpecs(out []string, re *regexp.Regexp, body string) []string {
	for _, match := range re.FindAllStringSubmatch(body, -1) {
		if len(match) > 1 {
			out = append(out, match[1])
		}
	}
	return out
}

func mapBoundaryLocalImportSpec(spec string) bool {
	if spec == "" {
		return false
	}
	switch {
	case strings.HasPrefix(spec, "."):
		return true
	case strings.HasPrefix(spec, "@/") || strings.HasPrefix(spec, "~/"):
		return true
	case strings.HasPrefix(spec, "src/") || strings.HasPrefix(spec, "app/") ||
		strings.HasPrefix(spec, "apps/") || strings.HasPrefix(spec, "packages/") ||
		strings.HasPrefix(spec, "libs/") || strings.HasPrefix(spec, "lib/") ||
		strings.HasPrefix(spec, "internal/") || strings.HasPrefix(spec, "pkg/"):
		return true
	case strings.HasPrefix(spec, "crate::") || strings.HasPrefix(spec, "self::") || strings.HasPrefix(spec, "super::"):
		return true
	case strings.HasPrefix(spec, "@") && strings.Contains(spec, "/"):
		return true
	}
	for _, r := range spec {
		if r == '.' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return false
	}
	return true
}

func resolveMapBoundaryImport(fromPath, spec string, fileSet map[string]bool, suffixIndex map[string]string) string {
	fromDir := pathpkg.Dir(normalizeMapPath(fromPath))
	spec = strings.TrimSpace(spec)
	switch {
	case strings.HasPrefix(spec, "."):
		return resolveMapBoundaryPathCandidate(pathpkg.Clean(pathpkg.Join(fromDir, spec)), fileSet)
	case strings.HasPrefix(spec, "@/") || strings.HasPrefix(spec, "~/"):
		return resolveMapBoundaryPathCandidate(strings.TrimPrefix(strings.TrimPrefix(spec, "@/"), "~/"), fileSet)
	case strings.HasPrefix(spec, "src/") || strings.HasPrefix(spec, "app/") ||
		strings.HasPrefix(spec, "apps/") || strings.HasPrefix(spec, "packages/") ||
		strings.HasPrefix(spec, "libs/") || strings.HasPrefix(spec, "lib/") ||
		strings.HasPrefix(spec, "internal/") || strings.HasPrefix(spec, "pkg/"):
		return resolveMapBoundaryPathCandidate(spec, fileSet)
	case strings.HasPrefix(spec, "crate::") || strings.HasPrefix(spec, "self::"):
		modulePath := mapBoundaryRustImportPath(strings.TrimPrefix(strings.TrimPrefix(spec, "crate::"), "self::"))
		return firstNonEmpty(resolveMapBoundaryPathCandidate(modulePath, fileSet), suffixIndex[modulePath])
	case strings.HasPrefix(spec, "super::"):
		modulePath := pathpkg.Clean(pathpkg.Join(fromDir, "..", mapBoundaryRustImportPath(strings.TrimPrefix(spec, "super::"))))
		return resolveMapBoundaryPathCandidate(modulePath, fileSet)
	default:
		modulePath := strings.TrimPrefix(spec, "@")
		if idx := strings.Index(modulePath, "/"); strings.HasPrefix(spec, "@") && idx >= 0 {
			modulePath = modulePath[idx+1:]
		}
		modulePath = strings.ReplaceAll(modulePath, ".", "/")
		modulePath = strings.ReplaceAll(modulePath, "::", "/")
		return firstNonEmpty(resolveMapBoundaryPathCandidate(modulePath, fileSet), suffixIndex[modulePath])
	}
}

func mapBoundaryRustImportPath(spec string) string {
	spec = strings.ReplaceAll(spec, "::", "/")
	spec = strings.ReplaceAll(spec, "{", "")
	spec = strings.ReplaceAll(spec, "}", "")
	if idx := strings.Index(spec, ","); idx >= 0 {
		spec = spec[:idx]
	}
	return strings.TrimSpace(spec)
}

func resolveMapBoundaryPathCandidate(base string, fileSet map[string]bool) string {
	base = normalizeMapPath(base)
	if base == "" {
		return ""
	}
	candidates := []string{base}
	for _, ext := range mapBoundarySourceExtensions {
		candidates = append(candidates, base+ext)
	}
	for _, ext := range mapBoundarySourceExtensions {
		candidates = append(candidates, pathpkg.Join(base, "index"+ext))
	}
	candidates = append(candidates, pathpkg.Join(base, "__init__.py"))
	for _, candidate := range candidates {
		candidate = normalizeMapPath(candidate)
		if fileSet[candidate] {
			return candidate
		}
	}
	return ""
}

func buildMapBoundarySuffixIndex(files []string) map[string]string {
	index := map[string]string{}
	for _, file := range files {
		if mapBoundaryPathFamily(file) != "source" && mapBoundaryPathFamily(file) != "test" {
			continue
		}
		addMapBoundarySuffixes(index, normalizeMapPath(file))
		ext := filepath.Ext(file)
		if ext != "" {
			addMapBoundarySuffixes(index, strings.TrimSuffix(normalizeMapPath(file), ext))
		}
	}
	return index
}

func addMapBoundarySuffixes(index map[string]string, file string) {
	parts := strings.Split(normalizeMapPath(file), "/")
	for n := 1; n <= mapMinInt(5, len(parts)); n++ {
		suffix := strings.Join(parts[len(parts)-n:], "/")
		if existing, ok := index[suffix]; ok && existing != file {
			index[suffix] = ""
		} else if !ok {
			index[suffix] = file
		}
	}
}

func applyMapBoundaryRecentCommits(candidates map[string]*mapPathBoundaryCandidate, repoName string, commits []parsedFindGitCommit) {
	for _, commit := range commits {
		touched := map[string]bool{}
		for _, path := range commit.paths {
			path = normalizeMapPath(path)
			if !mapBoundaryPathEligible(path) {
				continue
			}
			for _, labelCandidate := range mapBoundaryLabelCandidatesForPath(path, repoName) {
				candidate := candidates[labelCandidate.Key]
				if candidate == nil || touched[candidate.Key] {
					continue
				}
				touched[candidate.Key] = true
				candidate.RecentCount++
				candidate.EvidenceCounts["trace"]++
				candidate.EvidenceSources["git"] = true
				candidate.Score += 8
				if len(candidate.TraceReceipts) < mapMaxTraceReceipts {
					candidate.TraceReceipts = append(candidate.TraceReceipts, mapTraceReceipt{
						SHA:     shortFindGitSHA(commit.sha),
						Subject: limitRunes(commit.subject, 120),
					})
				}
			}
		}
	}
}

func mapBoundaryLabelCandidatesForPath(path, repoName string) []mapLabelCandidate {
	raw := mapPathLabelCandidates(path)
	raw = append(raw, mapBoundarySegmentLabelCandidates(path)...)
	var out []mapLabelCandidate
	seen := map[string]bool{}
	for _, candidate := range raw {
		if !mapBoundaryLabelCandidateAllowed(candidate, repoName) || seen[candidate.Key] {
			continue
		}
		seen[candidate.Key] = true
		out = append(out, candidate)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func mapBoundarySegmentLabelCandidates(path string) []mapLabelCandidate {
	parts := strings.Split(normalizeMapPath(path), "/")
	if len(parts) <= 1 {
		return nil
	}
	dirParts := parts[:len(parts)-1]
	var out []mapLabelCandidate
	for i, segment := range dirParts {
		cleaned := cleanMapPathSegmentForLabel(segment)
		key := normalizeMapKey(cleaned)
		if key == "" {
			continue
		}
		if mapBoundaryAllowedGenericAreaLabels[key] {
			out = append(out, mapLabelCandidate{Key: key, Label: displayMapLabel(cleaned), Score: 6.3 + float64(mapMinInt(i, 4))*0.25})
		}
	}
	return out
}

func mapBoundaryLabelCandidateAllowed(candidate mapLabelCandidate, repoName string) bool {
	rawKey := strings.ToLower(strings.TrimSpace(candidate.Key))
	repoKey := normalizeMapKey(repoName)
	if strings.Contains(rawKey, "/") {
		pairParts := strings.Split(rawKey, "/")
		if len(pairParts) > 0 && mapGenericTerms[normalizeMapKey(pairParts[0])] {
			return false
		}
		for _, part := range pairParts {
			partKey := normalizeMapKey(part)
			if mapBoundarySuppressedStandaloneLabels[partKey] || mapBoundaryRepoPackageKey(partKey, repoKey) || mapBoundaryRouteShellKey(partKey, repoKey) {
				return false
			}
		}
	}
	key := normalizeMapKey(candidate.Key)
	if key == "" || key != candidate.Key {
		candidate.Key = key
	}
	if mapBoundarySuppressedStandaloneLabels[key] || mapBoundaryRouteShellKey(key, repoKey) {
		return false
	}
	if key == "" || (isMapGenericAnchor(key) && !mapBoundaryAllowedGenericAreaLabels[key]) || mapGenericAreaLabels[key] || mapKeyMatchesRepoRoot(key, repoKey) || mapBoundaryRepoPackageKey(key, repoKey) {
		return false
	}
	switch key {
	case "type", "types", "constant", "constants", "mock", "mocks", "fixture", "fixtures":
		return false
	}
	parts := strings.FieldsFunc(key, func(r rune) bool { return r == '-' || r == '/' })
	if len(parts) == 0 {
		return false
	}
	meaningful := 0
	for _, part := range parts {
		switch part {
		case "id", "ids", "uuid", "guid", "slug", "ee", "oss", "tmp", "temp", "new", "edit", "view":
			return false
		}
		if len(part) >= 3 && (!mapGenericTerms[part] || mapBoundaryAllowedGenericAreaLabels[part]) {
			meaningful++
		}
	}
	return meaningful > 0
}

func mapBoundaryRouteShellKey(key, repoKey string) bool {
	if key == "" || repoKey == "" {
		return false
	}
	parts := mapStringSet(strings.Split(key, "-"))
	if !parts[repoKey] {
		return false
	}
	return parts["co"] || parts["com"] || parts["org"] || parts["io"] || parts["app"]
}

func mapBoundaryRepoPackageKey(key, repoKey string) bool {
	if key == "" || repoKey == "" || !strings.HasPrefix(key, repoKey+"-") {
		return false
	}
	suffix := strings.TrimPrefix(key, repoKey+"-")
	if suffix == "" {
		return true
	}
	for _, part := range strings.Split(suffix, "-") {
		if part == "front" || part == "website" {
			continue
		}
		if part != "" && !mapGenericTerms[part] {
			return false
		}
	}
	return true
}

func mapBoundaryContainingDirs(path, key string) []string {
	parts := strings.Split(normalizeMapPath(path), "/")
	if len(parts) <= 1 {
		return nil
	}
	dirs := parts[:len(parts)-1]
	var out []string
	for i := range dirs {
		segmentKey := normalizeMapKey(cleanMapPathSegmentForLabel(dirs[i]))
		if segmentKey == key {
			out = append(out, strings.Join(dirs[:i+1], "/"))
		}
	}
	if len(out) == 0 {
		out = append(out, strings.Join(dirs, "/"))
	}
	return firstStrings(out, 4)
}

func mapBoundarySubareasForCandidate(path, key string) []string {
	parts := strings.Split(normalizeMapPath(path), "/")
	if len(parts) <= 1 {
		return nil
	}
	dirs := parts[:len(parts)-1]
	matchIndex := -1
	for i, segment := range dirs {
		if normalizeMapKey(cleanMapPathSegmentForLabel(segment)) == key {
			matchIndex = i
			break
		}
	}
	var out []string
	if matchIndex >= 0 {
		for _, segment := range dirs[matchIndex+1:] {
			cleaned := cleanMapPathSegmentForLabel(segment)
			candidate := mapLabelCandidate{Key: normalizeMapKey(cleaned), Label: displayMapLabel(cleaned)}
			if mapBoundaryLabelCandidateAllowed(candidate, "") && candidate.Key != key {
				out = appendUniqueString(out, candidate.Label)
				if len(out) >= 3 {
					return out
				}
			}
		}
	}
	return out
}

func mapBoundaryAreaInternal(candidate *mapPathBoundaryCandidate) *mapAreaInternal {
	rawAnchors := mapBoundaryRawAnchors(candidate)
	labelSource := "path_boundary"
	caveats := []string{"path-primary boundary candidate"}
	if candidate.EvidenceSources["conceptual_parent"] {
		labelSource = "conceptual_parent"
		caveats = []string{"conceptual parent over path evidence"}
	}
	area := &mapAreaInternal{
		Key:             candidate.Key,
		Label:           candidate.Label,
		LabelScore:      candidate.LabelScore,
		LabelSource:     labelSource,
		Subareas:        copyBoolMap(candidate.Subareas),
		RawAnchors:      rawAnchors,
		Artifacts:       dedupeMapArtifacts(candidate.Artifacts),
		ArtifactPathSet: copyBoolMap(candidate.PathSet),
		EvidenceCounts:  mapCopyCounts(candidate.EvidenceCounts),
		EvidenceCount:   candidate.FileCount + candidate.RecentCount,
		ConfidenceSum:   mapBoundaryCandidateConfidence(candidate) * float64(maxInt(1, len(rawAnchors))),
		EvidenceSources: copyBoolMap(candidate.EvidenceSources),
		TraceReceipts:   firstMapTraceReceipts(candidate.TraceReceipts, mapMaxTraceReceipts),
		Caveats:         caveats,
	}
	if len(area.Artifacts) > mapBoundaryMaxArtifacts {
		area.Artifacts = area.Artifacts[:mapBoundaryMaxArtifacts]
	}
	return area
}

func mapBoundaryRawAnchors(candidate *mapPathBoundaryCandidate) []string {
	anchors := []string{candidate.Label, candidate.Key}
	for _, path := range candidate.BoundaryPathOrder {
		anchors = appendUniqueString(anchors, path)
		if len(anchors) >= 6 {
			return anchors
		}
	}
	for _, path := range sortedMapSet(candidate.BoundaryPaths) {
		anchors = appendUniqueString(anchors, path)
		if len(anchors) >= 6 {
			return anchors
		}
	}
	return anchors
}

func appendMapBoundaryPathCandidate(candidate *mapPathBoundaryCandidate, path string) {
	if candidate == nil {
		return
	}
	path = normalizeMapPath(path)
	if path == "" {
		return
	}
	if candidate.BoundaryPaths == nil {
		candidate.BoundaryPaths = map[string]bool{}
	}
	if candidate.BoundaryPaths[path] {
		return
	}
	candidate.BoundaryPaths[path] = true
	candidate.BoundaryPathOrder = append(candidate.BoundaryPathOrder, path)
}

func mapBoundaryCandidateDisplayable(candidate *mapPathBoundaryCandidate) bool {
	if candidate == nil || candidate.FileCount == 0 {
		return false
	}
	if (isMapGenericAnchor(candidate.Key) && !mapBoundaryAllowedGenericAreaLabels[candidate.Key]) || mapGenericAreaLabels[candidate.Key] {
		return false
	}
	families := mapBoundaryFamilyCount(candidate.EvidenceCounts)
	sourceish := candidate.EvidenceCounts["source"] + candidate.EvidenceCounts["test"]
	if candidate.EvidenceSources["conceptual_parent"] && candidate.FileCount >= 2 && sourceish > 0 {
		return true
	}
	if candidate.FileCount >= 4 && sourceish > 0 {
		return true
	}
	if candidate.FileCount >= 2 && families >= 2 {
		return true
	}
	if candidate.RecentCount > 0 && candidate.FileCount >= 2 {
		return true
	}
	return false
}

func selectMapBoundaryAreas(areas []*mapAreaInternal, limit int, repoShape string) []*mapAreaInternal {
	if limit <= 0 {
		limit = mapDefaultMaxAreas * 2
	}
	var out []*mapAreaInternal
	strongConceptualAvailable := mapBoundaryConceptualParentCount(areas) >= 3
	for _, area := range areas {
		if mapBoundaryWeakBroadAreaSuppressed(area, out, strongConceptualAvailable, repoShape) {
			continue
		}
		overlapped := false
		for _, selected := range out {
			if mapBoundaryConceptualParentsRedundant(selected, area) {
				overlapped = true
				break
			}
			if mapArtifactOverlap(selected.ArtifactPathSet, area.ArtifactPathSet) >= 0.72 {
				overlapped = true
				break
			}
			if mapBoundaryKeysRedundant(selected.Key, area.Key) {
				overlapped = true
				break
			}
		}
		if overlapped {
			continue
		}
		out = append(out, area)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func mapBoundaryWeakBroadAreaSuppressed(area *mapAreaInternal, selected []*mapAreaInternal, strongConceptualAvailable bool, repoShape string) bool {
	if area == nil {
		return false
	}
	if strongConceptualAvailable {
		if mapBoundaryShellLikeArea(area) {
			return true
		}
		switch area.Key {
		case "controllers", "db/migrate", "db-migrate", "migrate", "migrations", "scripts", "github", "www", "api-reference", "code-samples", "design-system", "integration-tests", "crates", "http", "shell", "steps":
			return true
		case "admin", "core-flows":
			return repoShape == mapRepoShapePlatform
		case "built-by-uv", "requirements", "uv-python":
			return repoShape == mapRepoShapeTool
		}
		if repoShape == mapRepoShapeTool && strings.HasPrefix(area.Key, "crates/") {
			return true
		}
		if repoShape == mapRepoShapePlatform && strings.Contains(area.Key, "code-samples") {
			return true
		}
	}
	switch area.Key {
	case "settings":
		return mapBoundarySelectedConceptualParentCount(selected) >= 3
	default:
		return false
	}
}

func mapBoundaryShellLikeArea(area *mapAreaInternal) bool {
	if area == nil {
		return false
	}
	key := normalizeMapKey(area.Key)
	if mapBoundaryFirstScreenShellLabels[key] {
		return true
	}
	parts := mapStringSet(strings.FieldsFunc(key, func(r rune) bool { return r == '-' || r == '/' }))
	if parts["server"] && parts["only"] {
		return true
	}
	if parts["blackbox"] || parts["composable"] || parts["composables"] {
		return true
	}
	if parts["ui"] && (parts["primitive"] || parts["primitives"]) {
		return true
	}
	if (parts["primitive"] || parts["primitives"] || parts["universal"]) && len(area.Subareas) >= 2 {
		return true
	}
	if (parts["remix"] || parts["trpc"]) && len(area.ArtifactPathSet) >= 2 {
		return true
	}
	if (parts["example"] || parts["examples"] || parts["template"] || parts["templates"] || parts["util"] || parts["utils"] || parts["utilities"]) && len(area.ArtifactPathSet) >= 2 {
		return true
	}
	return false
}

func mapBoundaryConceptualParentCount(areas []*mapAreaInternal) int {
	count := 0
	for _, area := range areas {
		if area != nil && area.EvidenceSources["conceptual_parent"] {
			count++
		}
	}
	return count
}

func mapBoundarySelectedConceptualParentCount(areas []*mapAreaInternal) int {
	count := 0
	for _, area := range areas {
		if area != nil && area.EvidenceSources["conceptual_parent"] {
			count++
		}
	}
	return count
}

func mapBoundaryConceptualParentsRedundant(selected, candidate *mapAreaInternal) bool {
	if selected == nil || candidate == nil {
		return false
	}
	selectedGroup := mapBoundaryConceptualDuplicateGroup(selected.Key)
	candidateGroup := mapBoundaryConceptualDuplicateGroup(candidate.Key)
	return selectedGroup != "" && selectedGroup == candidateGroup
}

func mapBoundaryConceptualDuplicateGroup(key string) string {
	switch key {
	case "workspace-identity-access-billing", "identity-auth-workspace-tenancy", "identity-auth-access-control":
		return "identity-access"
	case "public-api-layer", "public-http-api-developer-platform", "external-http-api-v1", "http-api-layer":
		return "api-platform"
	case "metadata-engine-data-model", "content-data-model":
		return "data-model"
	case "workflows-automation", "flows-automation":
		return "automation"
	default:
		return ""
	}
}

func mapBoundaryKeysRedundant(a, b string) bool {
	if a == "" || b == "" || a == b {
		return true
	}
	aParts := mapStringSet(strings.FieldsFunc(a, func(r rune) bool { return r == '-' || r == '/' }))
	bParts := mapStringSet(strings.FieldsFunc(b, func(r rune) bool { return r == '-' || r == '/' }))
	shared := 0
	for part := range aParts {
		if bParts[part] && !mapGenericTerms[part] {
			shared++
		}
	}
	return shared > 0 && (len(aParts) == 1 || len(bParts) == 1)
}

func mapBoundaryAreaScore(area *mapAreaInternal, repoName, repoShape string) float64 {
	score := mapAreaScore(area)
	score += float64(mapMinInt(len(area.Subareas), 5)) * 3
	score += float64(mapMinInt(len(area.ArtifactPathSet), 40)) * 0.25
	score += float64(mapMinInt(area.EvidenceCounts["trace"], 4)) * 4
	score += mapBoundaryBalancedSourceTestBonus(area)
	score += float64(mapMinInt(area.EvidenceCounts["import"], mapBoundaryImportScoreCap)) * 0.35
	score += float64(mapMinInt(area.EvidenceCounts["test_import"], mapBoundaryTestImportScoreCap)) * 0.9
	if area.EvidenceSources["conceptual_parent"] {
		score += 12
		if area.Key == "framework-runtime-module-platform" {
			score += 10
		}
	}
	if area.EvidenceCounts["source"] > 0 && area.EvidenceCounts["test"] > 0 {
		score += 5
	}
	if area.EvidenceCounts["source"] > 0 && (area.EvidenceCounts["doc"] > 0 || area.EvidenceCounts["intent"] > 0) {
		score += 4
	}
	if mapBoundaryShellLikeArea(area) {
		score -= 35
	}
	role := classifyMapBoundaryRole(area, area.Label, "", mapAreaIsRepoRoot(area, repoName), sortedMapSet(area.Subareas), repoName, repoShape)
	roleAdjustment := mapBoundaryRoleScoreAdjustment(role)
	if area.EvidenceSources["conceptual_parent"] && roleAdjustment < 0 {
		roleAdjustment *= 0.35
	}
	score += roleAdjustment
	return score
}

func mapBoundaryBalancedSourceTestBonus(area *mapAreaInternal) float64 {
	if area == nil {
		return 0
	}
	source := area.EvidenceCounts["source"]
	test := area.EvidenceCounts["test"]
	if source == 0 || test == 0 {
		return 0
	}
	return float64(mapMinInt(source, test)) * 0.1
}

func mapBoundaryRoleScoreAdjustment(role string) float64 {
	switch role {
	case mapBoundaryRoleProductCapability:
		return 6
	case mapBoundaryRoleExtensionEcosystem:
		return 1
	case mapBoundaryRoleDocsReference:
		return -4
	case mapBoundaryRoleHorizontalLayer:
		return -14
	case mapBoundaryRoleGenericParent:
		return -18
	case mapBoundaryRoleFixtureOrTestbed:
		return -24
	case mapBoundaryRoleRepoNamespace:
		return -32
	case mapBoundaryRoleHandoffUnsafe:
		return -28
	default:
		return 0
	}
}

func mapBoundaryOutputConfidence(areas []mapArea) string {
	if len(areas) == 0 {
		return mapLowConfidence
	}
	mediumOrBetter := 0
	withHierarchy := 0
	generic := 0
	for _, area := range areas {
		if area.Confidence == mapMediumConfidence || area.Confidence == mapHighConfidence {
			mediumOrBetter++
		}
		if len(area.Covers) >= 2 {
			withHierarchy++
		}
		if area.IsRepoRootUmbrella || (isMapGenericAnchor(area.Label) && !mapBoundaryAllowedGenericAreaLabels[normalizeMapKey(area.Label)]) {
			generic++
		}
	}
	if len(areas) >= 4 && mediumOrBetter >= 4 && withHierarchy >= 3 && generic == 0 {
		return mapHighConfidence
	}
	if len(areas) >= 3 && mediumOrBetter >= 2 && generic <= 1 {
		return mapMediumConfidence
	}
	return mapLowConfidence
}

func mapBoundaryCandidateConfidence(candidate *mapPathBoundaryCandidate) float64 {
	families := mapBoundaryFamilyCount(candidate.EvidenceCounts)
	sourceish := candidate.EvidenceCounts["source"] + candidate.EvidenceCounts["test"]
	switch {
	case candidate.FileCount >= 8 && families >= 2 && sourceish > 0:
		return 0.86
	case candidate.FileCount >= 3 && sourceish > 0:
		return 0.74
	case families >= 2:
		return 0.66
	default:
		return 0.55
	}
}

func mapBoundaryFamilyCount(counts map[string]int) int {
	families := 0
	for _, family := range []string{"source", "test", "doc", "intent", "protocol", "trace", "config"} {
		if counts[family] > 0 {
			families++
		}
	}
	return families
}

func mapBoundaryFamilyScore(family string) float64 {
	switch family {
	case "test":
		return 3.5
	case "source":
		return 3
	case "doc", "intent":
		return 2.5
	case "config":
		return 1.5
	default:
		return 1
	}
}

func mapArtifactForBoundaryPath(path, family string) mapArtifact {
	switch family {
	case "test":
		return mapArtifact{Title: filepath.Base(path), Kind: "test_case", Path: path}
	case "doc":
		return mapArtifact{Title: filepath.Base(path), Kind: "markdown_artifact", Path: path}
	case "config":
		return mapArtifact{Title: filepath.Base(path), Kind: "source_context", Subtype: "config", Path: path}
	default:
		return mapArtifact{Title: filepath.Base(path), Kind: "source_context", Path: path}
	}
}

func mapPathBoundaryArtifactRank(areaKey, path, family string) int {
	pathValue := normalizeMapPath(path)
	pathKey := normalizeMapKey(pathValue)
	score := 0
	switch family {
	case "source":
		score += 500
	case "test":
		score += 360
	case "config":
		score += 170
	case "doc":
		score += 140
	default:
		score += 60
	}
	if areaKey != "" && scoreMapKeyMatch(pathKey, areaKey) > 0 {
		score += 70
	}
	switch {
	case strings.HasPrefix(pathValue, "test/") || strings.HasPrefix(pathValue, "tests/") || strings.Contains(pathValue, "/test/") || strings.Contains(pathValue, "/tests/"):
		score -= 260
	case strings.HasPrefix(pathValue, "examples/") || strings.Contains(pathValue, "/examples/"):
		score -= 320
	case strings.HasPrefix(pathValue, "docs/") || strings.Contains(pathValue, "/docs/"):
		score -= 220
	case strings.Contains(pathValue, "/templates/") || strings.Contains(pathValue, "/template/"):
		score -= 130
	case strings.HasPrefix(pathValue, ".github/") || strings.Contains(pathValue, "/.github/"):
		score -= 260
	}
	base := strings.ToLower(filepath.Base(pathValue))
	switch base {
	case "package.json", "index.html", "jest.config.ts", "babel.config.cjs", "eslint.config.js", "tsconfig.json":
		score -= 90
	}
	if strings.Contains(pathKey, "generated") || strings.Contains(pathKey, "fixture") {
		score -= 120
	}
	return score
}

func mapBoundaryPathEligible(path string) bool {
	path = normalizeMapPath(path)
	if path == "" || mapPathSuppressed(path) || mapRecentPathNoisy(path) {
		return false
	}
	parts := strings.Split(strings.ToLower(path), "/")
	for _, part := range parts {
		if strings.HasSuffix(part, ".git") {
			return false
		}
		switch part {
		case "generated", "__generated__", "gen", ".turbo", ".cache":
			return false
		}
		if strings.HasPrefix(part, ".") && part != ".github" {
			return false
		}
	}
	return mapBoundaryPathFamily(path) != ""
}

func mapBoundaryPathFamily(path string) string {
	lower := strings.ToLower(filepath.ToSlash(path))
	ext := strings.ToLower(filepath.Ext(lower))
	base := strings.ToLower(filepath.Base(lower))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".icns", ".svg", ".pdf", ".zip", ".gz", ".tgz", ".wasm", ".map", ".lock":
		return ""
	}
	if strings.HasSuffix(lower, "yarn.lock") || strings.HasSuffix(lower, "package-lock.json") || strings.HasSuffix(lower, "pnpm-lock.yaml") {
		return ""
	}
	switch base {
	case "license", "copying", "notice":
		return ""
	case "dockerfile", "makefile", "justfile", "procfile":
		return "config"
	}
	switch ext {
	case ".md", ".mdx", ".rst", ".adoc":
		return "doc"
	case ".yaml", ".yml", ".json", ".toml", ".xml", ".ini", ".env", ".sql":
		return "config"
	}
	return mapRecentPathFamily(path)
}

func copyBoolMap(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for key, value := range in {
		if value {
			out[key] = true
		}
	}
	return out
}

func prepareMapCluster(cluster scan.WorkstreamClusterExample, repoName string) *mapPreparedCluster {
	anchor := strings.TrimSpace(cluster.Anchor)
	if anchor == "" {
		return nil
	}
	var artifacts []mapArtifact
	pathSet := map[string]bool{}
	for _, ex := range cluster.ExampleArtifacts {
		art := mapArtifact{
			Title:   ex.Title,
			Kind:    ex.Kind,
			Subtype: ex.Subtype,
			Path:    normalizeMapPath(ex.Path),
		}
		if art.Path == "" || mapPathSuppressed(art.Path) {
			continue
		}
		artifacts = append(artifacts, art)
		pathSet[art.Path] = true
	}
	if len(artifacts) == 0 {
		return nil
	}
	candidates := map[string]*mapLabelCandidate{}
	for _, art := range artifacts {
		for _, cand := range mapPathLabelCandidates(art.Path) {
			addMapLabelCandidate(candidates, cand.Key, cand.Label, cand.Score, art.Path, "path_boundary")
		}
		for _, cand := range mapFileStemCandidates(art.Path) {
			addMapLabelCandidate(candidates, cand.Key, cand.Label, cand.Score*0.75, art.Path, "file_stem")
		}
	}
	for _, cand := range mapAnchorLabelCandidates(anchor) {
		addMapLabelCandidate(candidates, cand.Key, cand.Label, cand.Score, "", "anchor")
	}
	parent := chooseMapLabel(candidates, anchor, repoName)
	if parent == nil {
		parent = &mapLabelCandidate{Key: normalizeMapKey(anchor), Label: titleizeMapAnchor(anchor), Score: 1, Sources: map[string]bool{"fallback_anchor": true}}
	}
	subarea := chooseMapSubarea(anchor, parent.Key)
	return &mapPreparedCluster{
		Anchor:          anchor,
		ParentKey:       parent.Key,
		ParentLabel:     parent.Label,
		ParentScore:     parent.Score,
		ParentSource:    firstMapKey(parent.Sources),
		Subarea:         subarea,
		Artifacts:       artifacts,
		ArtifactPathSet: pathSet,
		EvidenceCounts:  mapEvidenceCounts(cluster.ExampleArtifacts),
		EvidenceCount:   cluster.EvidenceCount,
		Confidence:      cluster.Confidence,
		EvidenceSources: mapStringSet(cluster.EvidenceSources),
		ExampleCommits:  firstStrings(cluster.ExampleCommits, 4),
		Caveats:         firstStrings(cluster.Caveats, 4),
		GenericAnchor:   isMapGenericAnchor(anchor),
	}
}

func newMapArea(p *mapPreparedCluster) *mapAreaInternal {
	area := &mapAreaInternal{
		Key:             p.ParentKey,
		Label:           p.ParentLabel,
		LabelScore:      p.ParentScore,
		LabelSource:     p.ParentSource,
		Subareas:        map[string]bool{},
		RawAnchors:      []string{p.Anchor},
		Artifacts:       append([]mapArtifact{}, p.Artifacts...),
		ArtifactPathSet: map[string]bool{},
		EvidenceCounts:  map[string]int{},
		EvidenceCount:   p.EvidenceCount,
		ConfidenceSum:   p.Confidence,
		EvidenceSources: map[string]bool{},
		ExampleCommits:  append([]string{}, p.ExampleCommits...),
		Caveats:         append([]string{}, p.Caveats...),
	}
	for k := range p.ArtifactPathSet {
		area.ArtifactPathSet[k] = true
	}
	for k, v := range p.EvidenceCounts {
		area.EvidenceCounts[k] += v
	}
	for k := range p.EvidenceSources {
		area.EvidenceSources[k] = true
	}
	if p.Subarea != "" {
		area.Subareas[p.Subarea] = true
	}
	if p.GenericAnchor {
		area.GenericCount++
	}
	return area
}

func mergeMapCluster(area *mapAreaInternal, p *mapPreparedCluster) {
	if p.ParentScore > area.LabelScore && !isMapGenericAnchor(p.ParentKey) {
		area.Key = p.ParentKey
		area.Label = p.ParentLabel
		area.LabelScore = p.ParentScore
		area.LabelSource = p.ParentSource
	}
	area.RawAnchors = appendUniqueString(area.RawAnchors, p.Anchor)
	if p.Subarea != "" {
		area.Subareas[p.Subarea] = true
	}
	area.Artifacts = append(area.Artifacts, p.Artifacts...)
	for k := range p.ArtifactPathSet {
		area.ArtifactPathSet[k] = true
	}
	for k, v := range p.EvidenceCounts {
		area.EvidenceCounts[k] += v
	}
	area.EvidenceCount += p.EvidenceCount
	area.ConfidenceSum += p.Confidence
	for k := range p.EvidenceSources {
		area.EvidenceSources[k] = true
	}
	for _, commit := range p.ExampleCommits {
		area.ExampleCommits = appendUniqueString(area.ExampleCommits, commit)
	}
	for _, caveat := range p.Caveats {
		area.Caveats = appendUniqueString(area.Caveats, caveat)
	}
	if p.GenericAnchor {
		area.GenericCount++
	}
}

func finalizeMapArea(area *mapAreaInternal) {
	if area.Subareas == nil {
		area.Subareas = map[string]bool{}
	}
	for _, anchor := range area.RawAnchors {
		if sub := chooseMapSubarea(anchor, area.Key); sub != "" {
			area.Subareas[sub] = true
		}
	}
	area.Artifacts = dedupeMapArtifacts(area.Artifacts)
	if len(area.Artifacts) > mapMaxArtifactsPerArea {
		area.Artifacts = area.Artifacts[:mapMaxArtifactsPerArea]
	}
}

func mergeDuplicateMapAreas(areas []*mapAreaInternal) []*mapAreaInternal {
	var out []*mapAreaInternal
	for _, area := range areas {
		var existing *mapAreaInternal
		for _, candidate := range out {
			if candidate.Key == area.Key {
				existing = candidate
				break
			}
		}
		if existing == nil {
			out = append(out, area)
			continue
		}
		p := &mapPreparedCluster{
			Anchor:          strings.Join(area.RawAnchors, " "),
			ParentKey:       area.Key,
			ParentLabel:     area.Label,
			ParentScore:     area.LabelScore,
			ParentSource:    area.LabelSource,
			Artifacts:       area.Artifacts,
			ArtifactPathSet: area.ArtifactPathSet,
			EvidenceCounts:  area.EvidenceCounts,
			EvidenceCount:   area.EvidenceCount,
			Confidence:      area.ConfidenceSum,
			EvidenceSources: area.EvidenceSources,
			ExampleCommits:  area.ExampleCommits,
			Caveats:         area.Caveats,
		}
		for sub := range area.Subareas {
			existing.Subareas[sub] = true
		}
		mergeMapCluster(existing, p)
	}
	return out
}

func publicMapAreas(repoRoot, repoName string, areas []*mapAreaInternal, maxAreas int, repoShape string, packability *mapPackabilityIndex) []mapArea {
	if maxAreas <= 0 {
		maxAreas = mapDefaultMaxAreas
	}
	out := make([]mapArea, 0, mapMinInt(maxAreas, len(areas)))
	for _, area := range areas {
		if len(out) >= maxAreas {
			break
		}
		if area.Filtered {
			continue
		}
		areaClass := classifyMapArea(area)
		confidence := mapAreaConfidence(area, areaClass)
		isRoot := mapAreaIsRepoRoot(area, repoName)
		label := area.Label
		if isRoot {
			label = reframeMapRootLabel(area, repoName, confidence)
		}
		covers := sortedMapSet(area.Subareas)
		covers = cleanMapCovers(label, covers)
		label = refineMapAreaLabel(label, covers)
		covers = cleanMapCovers(label, covers)
		traceReceipts := firstMapTraceReceipts(area.TraceReceipts, mapMaxTraceReceipts)
		if len(traceReceipts) == 0 {
			traceReceipts = mapTraceReceipts(repoRoot, area, covers)
		}
		keyPaths := mapArtifactPaths(area.Artifacts)
		areaType := classifyMapAreaType(area, label, areaClass, isRoot, covers)
		boundaryRole := classifyMapBoundaryRole(area, label, areaType, isRoot, covers, repoName, repoShape)
		try, tryPackability := mapTryCommandForRoleWithPackability(label, covers, traceReceipts, confidence, keyPaths, boundaryRole, packability)
		caveats := mapAreaCaveats(area, areaClass, isRoot)
		if isRoot && len(covers) == 0 {
			caveats = appendUniqueString(caveats, "package-root signal only")
		}
		pub := mapArea{
			ID:                 "area." + normalizeMapKey(label),
			Label:              label,
			Class:              areaClass,
			AreaType:           areaType,
			BoundaryRole:       boundaryRole,
			Confidence:         confidence,
			IsRepoRootUmbrella: isRoot,
			Covers:             firstStrings(covers, mapMaxCoversPerArea),
			EvidenceCounts:     mapCopyCounts(area.EvidenceCounts),
			KeyPaths:           keyPaths,
			TraceReceipts:      firstMapTraceReceipts(traceReceipts, mapMaxTraceReceipts),
			Try:                try,
			Caveats:            caveats,
			Diagnostics: mapAreaDiagnostics{
				Key:              area.Key,
				RawAnchors:       firstStrings(area.RawAnchors, 8),
				LabelEvidence:    mapLabelEvidence(area),
				TraceTerms:       mapTraceTerms(traceReceipts),
				TraceReceiptMode: mapTraceReceiptMode,
				Packability:      tryPackability,
			},
		}
		out = append(out, pub)
	}
	annotateMapAreas(out)
	return out
}

func writeMapText(out io.Writer, m mapOutput, verbose bool) {
	fmt.Fprintf(out, "Repo map: %s\n", m.Repo.Name)
	fmt.Fprintf(out, "Confidence: %s\n", m.Repo.Confidence)
	fmt.Fprintf(out, "Evidence: %s\n", mapEvidenceText(m.EvidenceAvailability))
	fmt.Fprintln(out)
	if len(m.Areas) == 0 {
		fmt.Fprintln(out, "I could not find enough repo-local evidence to build a useful map.")
		writeMapCaveats(out, m.Caveats)
		return
	}
	if m.Repo.Confidence == mapLowConfidence {
		fmt.Fprintln(out, "I found repo-local signals, but not enough distinct boundaries to make a strong map.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Candidate subsystems")
	} else {
		fmt.Fprintln(out, "Detected subsystems")
	}
	fmt.Fprintln(out)
	for i, area := range m.Areas {
		fmt.Fprintf(out, "%d. Subsystem: %s\n", i+1, area.Label)
		if area.Purpose != "" {
			fmt.Fprintf(out, "   Purpose: %s\n", area.Purpose)
		}
		if len(area.BoundaryPaths) > 0 {
			fmt.Fprintf(out, "   Boundary: %s\n", strings.Join(area.BoundaryPaths, ", "))
		}
		if len(area.Covers) > 0 {
			fmt.Fprintf(out, "   Covers: %s\n", strings.Join(area.Covers, ", "))
		}
		fmt.Fprintf(out, "   Evidence: %s\n", mapAreaEvidenceText(area.EvidenceCounts))
		if len(area.AdjacentSystems) > 0 {
			fmt.Fprintf(out, "   Adjacent systems: %s\n", strings.Join(area.AdjacentSystems, ", "))
		}
		if len(area.KeyPaths) > 0 {
			fmt.Fprintln(out, "   Key files:")
			for _, p := range firstStrings(area.KeyPaths, 3) {
				fmt.Fprintf(out, "   - %s\n", p)
			}
		}
		if len(area.TraceReceipts) > 0 {
			fmt.Fprintf(out, "   Recent signal: %s\n", area.TraceReceipts[0].Subject)
		}
		if findCmd := mapAreaFindCommand(area); findCmd != "" {
			fmt.Fprintf(out, "   Try: %s\n", findCmd)
		}
		if taskCmd := mapAreaTaskCommand(area); taskCmd != "" {
			fmt.Fprintf(out, "   Try: %s\n", taskCmd)
		}
		if verbose {
			fmt.Fprintf(out, "   Diagnostics: type=%s class=%s role=%s confidence=%s key=%s\n", displayMapAreaType(area.AreaType), area.Class, area.BoundaryRole, area.Confidence, area.Diagnostics.Key)
			if area.Diagnostics.Packability != nil {
				fmt.Fprintf(out, "   Packability: %s\n", mapPackabilityDiagnosticsText(area.Diagnostics.Packability))
			}
			if len(area.Diagnostics.RawAnchors) > 0 {
				fmt.Fprintf(out, "   Raw anchors: %s\n", strings.Join(area.Diagnostics.RawAnchors, ", "))
			}
			if len(area.Diagnostics.TraceTerms) > 0 {
				fmt.Fprintf(out, "   Trace terms: %s\n", strings.Join(firstStrings(area.Diagnostics.TraceTerms, mapMaxVerboseTrace), ", "))
			}
			if len(area.Caveats) > 0 {
				fmt.Fprintf(out, "   Caveats: %s\n", strings.Join(area.Caveats, "; "))
			}
		}
		fmt.Fprintln(out)
	}
	writeMapCaveats(out, m.Caveats)
}

func mapAreaFindCommand(area mapArea) string {
	if strings.TrimSpace(area.Try) != "" {
		return strings.TrimSpace(area.Try)
	}
	query := strings.TrimSpace(area.Label)
	if query == "" {
		return ""
	}
	return fmt.Sprintf("ds find %q", strings.ToLower(query))
}

func mapAreaTaskCommand(area mapArea) string {
	query := strings.TrimSpace(area.Label)
	if len(area.Covers) > 0 {
		query = strings.TrimSpace(query + " " + area.Covers[0])
	}
	if query == "" {
		return ""
	}
	return fmt.Sprintf("ds task \"modify %s\"", strings.ToLower(query))
}

func writeMapAreaText(out io.Writer, m mapOutput, query string, verbose bool) {
	matches := matchMapAreas(m.Areas, query)
	if len(matches) == 0 {
		fmt.Fprintf(out, "Map area: %s\n", query)
		fmt.Fprintf(out, "Repo: %s\n\n", m.Repo.Name)
		fmt.Fprintln(out, "No matching map area found.")
		if len(m.Areas) > 0 {
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Available areas:")
			for _, area := range firstMapAreas(m.Areas, mapDefaultMaxAreas) {
				fmt.Fprintf(out, "- %s (%s)\n", area.Label, displayMapAreaType(area.AreaType))
			}
		}
		writeMapCaveats(out, m.Caveats)
		return
	}

	primary := matches[0].Area
	primary = refineMapAreaForDrilldownQuery(primary, query)
	fmt.Fprintf(out, "Map area: %s\n", primary.Label)
	fmt.Fprintf(out, "Repo: %s\n", m.Repo.Name)
	if primary.AreaType != "" {
		fmt.Fprintf(out, "Type: %s\n", displayMapAreaType(primary.AreaType))
	}
	fmt.Fprintf(out, "Confidence: %s\n", primary.Confidence)
	fmt.Fprintf(out, "Evidence: %s\n", mapAreaEvidenceText(primary.EvidenceCounts))
	fmt.Fprintln(out)
	if len(primary.Covers) > 0 {
		fmt.Fprintf(out, "Covers: %s\n\n", strings.Join(primary.Covers, ", "))
	}
	if len(primary.KeyPaths) > 0 {
		fmt.Fprintln(out, "Key files:")
		for _, p := range firstStrings(primary.KeyPaths, 6) {
			fmt.Fprintf(out, "- %s\n", p)
		}
		fmt.Fprintln(out)
	}
	if len(primary.TraceReceipts) > 0 {
		if len(primary.TraceReceipts) == 1 {
			fmt.Fprintln(out, "Recent signal:")
		} else {
			fmt.Fprintln(out, "Recent signals:")
		}
		for _, receipt := range firstMapTraceReceipts(primary.TraceReceipts, 3) {
			if receipt.SHA != "" {
				fmt.Fprintf(out, "- %s %s\n", receipt.SHA, receipt.Subject)
			} else {
				fmt.Fprintf(out, "- %s\n", receipt.Subject)
			}
		}
		fmt.Fprintln(out)
	}
	commands := mapAreaPackCommands(primary)
	if len(commands) > 0 {
		fmt.Fprintln(out, "Pack this context:")
		for _, cmd := range commands {
			fmt.Fprintf(out, "- %s\n", cmd)
		}
		fmt.Fprintln(out)
	}
	related := relatedMapAreas(primary, m.Areas)
	if len(related) > 0 {
		fmt.Fprintln(out, "Related areas:")
		for _, area := range related {
			fmt.Fprintf(out, "- %s (%s)\n", area.Label, displayMapAreaType(area.AreaType))
		}
		fmt.Fprintln(out)
	}
	otherMatches := otherMapAreaMatches(matches, related)
	if len(otherMatches) > 0 {
		fmt.Fprintln(out, "Other matches:")
		for _, match := range firstMapAreaMatches(otherMatches, 3) {
			fmt.Fprintf(out, "- %s (%s)\n", match.Area.Label, displayMapAreaType(match.Area.AreaType))
		}
		fmt.Fprintln(out)
	}
	if verbose {
		fmt.Fprintf(out, "Diagnostics: match_score=%d class=%s role=%s key=%s\n", matches[0].Score, primary.Class, primary.BoundaryRole, primary.Diagnostics.Key)
		if primary.Diagnostics.Packability != nil {
			fmt.Fprintf(out, "Packability: %s\n", mapPackabilityDiagnosticsText(primary.Diagnostics.Packability))
		}
		if len(primary.Diagnostics.RawAnchors) > 0 {
			fmt.Fprintf(out, "Raw anchors: %s\n", strings.Join(primary.Diagnostics.RawAnchors, ", "))
		}
		if len(primary.Diagnostics.TraceTerms) > 0 {
			fmt.Fprintf(out, "Trace terms: %s\n", strings.Join(firstStrings(primary.Diagnostics.TraceTerms, mapMaxVerboseTrace), ", "))
		}
		fmt.Fprintln(out)
	}
	writeMapCaveats(out, append([]string{}, primary.Caveats...))
}

type mapRecentTopicBuilder struct {
	Label          string
	Query          string
	Key            string
	CommitSHAs     map[string]bool
	PathSet        map[string]bool
	EvidenceCounts map[string]int
	Maintenance    map[string]bool
	RecentSignals  []mapTraceReceipt
	Score          int
	Order          int
}

func buildMapRecentOutput(ctx context.Context, repoRoot string, opts mapOptions) mapRecentOutput {
	repoName := filepath.Base(repoRoot)
	out := mapRecentOutput{
		Schema:    mapRecentSchemaVersion,
		Repo:      mapRepo{Name: repoName, Path: repoRoot, Confidence: mapMediumConfidence},
		AreaQuery: opts.AreaQuery,
	}
	gitCtx, cancel := context.WithTimeout(ctx, findGitReceiptTimeout)
	defer cancel()
	if !findGitRepoAvailable(gitCtx, repoRoot) {
		out.Repo.Confidence = mapLowConfidence
		out.Caveats = append(out.Caveats, "local git history is unavailable")
		return out
	}
	commits, ok := findGitLogRecent(gitCtx, repoRoot, mapRecentMaxCommits)
	if !ok {
		out.Repo.Confidence = mapLowConfidence
		out.Caveats = append(out.Caveats, "recent git history could not be read within the time budget")
		return out
	}
	topics, skipped := buildMapRecentTopics(commits, opts.AreaQuery, mapRecentMaxCommits)
	applyMapRecentBoundaryQuality(ctx, repoRoot, commits, topics)
	topics = mergeMapRecentOverlappingTopics(topics)
	sortMapRecentTopics(topics)
	rawTopicCount := len(topics)
	if len(topics) > mapRecentMaxTopics {
		topics = topics[:mapRecentMaxTopics]
	}
	out.Topics = topics
	out.Diagnostics = mapRecentDiagnostics{
		CommitsRead:       len(commits),
		CommitsSkipped:    skipped,
		RawTopicCount:     rawTopicCount,
		MatchedTopicCount: len(topics),
	}
	if opts.AreaQuery != "" && len(topics) == 0 {
		out.Caveats = append(out.Caveats, "no recent topic matched the supplied area query")
	}
	if len(topics) == 0 {
		out.Repo.Confidence = mapLowConfidence
		out.Caveats = append(out.Caveats, "no non-noisy recent topics found")
	}
	return out
}

func buildFastMapFallbackOutputFromRecent(repoRoot string, recent mapRecentOutput, indexReady bool) mapOutput {
	out := mapOutput{
		Schema: mapSchemaVersion,
		Repo: mapRepo{
			Name:       recent.Repo.Name,
			Path:       recent.Repo.Path,
			Confidence: recent.Repo.Confidence,
		},
		EvidenceAvailability: mapEvidenceAvailability{Trace: true},
		Diagnostics: mapDiagnostics{
			RawClusterCount:       len(recent.Topics),
			WorkstreamAnchorsSeen: len(recent.Topics),
		},
	}
	if out.Repo.Name == "" {
		out.Repo.Name = filepath.Base(repoRoot)
	}
	if out.Repo.Path == "" {
		out.Repo.Path = repoRoot
	}
	if len(recent.Topics) == 0 {
		out.Repo.Confidence = mapLowConfidence
		out.Caveats = append(out.Caveats, recent.Caveats...)
		out.Caveats = append(out.Caveats, "fast map fallback had no recent local git/path topics")
		if !indexReady {
			out.Caveats = append(out.Caveats, mapIndexRequiredCaveat)
		}
		return out
	}
	for _, topic := range recent.Topics {
		area := mapAreaFromRecentTopic(topic)
		out.Areas = append(out.Areas, area)
		out.EvidenceAvailability.Source += topic.EvidenceCounts["source"]
		out.EvidenceAvailability.Test += topic.EvidenceCounts["test"]
		out.EvidenceAvailability.Markdown += topic.EvidenceCounts["doc"]
	}
	if !indexReady {
		out.Caveats = append(out.Caveats, "fast map from recent local git/path evidence; durable repo map evidence is not loaded yet")
		out.Caveats = append(out.Caveats, mapIndexRequiredCaveat)
	} else {
		out.Caveats = append(out.Caveats, "fast map from recent local git/path evidence; suggested packs can use the local index")
	}
	out.Repo.Confidence = mapMediumConfidence
	return out
}

func mapAreaFromRecentTopic(topic mapRecentTopic) mapArea {
	internal := &mapAreaInternal{
		Key:             normalizeMapKey(topic.Query),
		Label:           topic.Label,
		EvidenceCounts:  copyIntMap(topic.EvidenceCounts),
		ArtifactPathSet: map[string]bool{},
	}
	for _, path := range topic.KeyPaths {
		internal.Artifacts = append(internal.Artifacts, mapArtifact{Path: path, Title: path})
	}
	areaType := classifyMapAreaType(internal, topic.Label, mapClassWorkstream, false, nil)
	confidence := mapMediumConfidence
	if topic.FileCount <= 1 || topic.CommitCount <= 0 {
		confidence = mapLowConfidence
	}
	return mapArea{
		ID:             "recent:" + normalizeMapKey(topic.Query),
		Label:          topic.Label,
		Class:          mapClassWorkstream,
		AreaType:       areaType,
		Confidence:     confidence,
		EvidenceCounts: copyIntMap(topic.EvidenceCounts),
		KeyPaths:       topic.KeyPaths,
		TraceReceipts:  topic.RecentSignals,
		Try:            topic.Try,
		Diagnostics: mapAreaDiagnostics{
			Key:              normalizeMapKey(topic.Query),
			TraceTerms:       mapRecentComparableWords(topic.Query),
			TraceReceiptMode: mapTraceReceiptMode,
		},
	}
}

func copyIntMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func buildMapRecentTopics(commits []parsedFindGitCommit, areaQuery string, limit int) ([]mapRecentTopic, int) {
	if limit <= 0 {
		limit = mapRecentMaxTopics
	}
	builders := map[string]*mapRecentTopicBuilder{}
	order := 0
	skipped := 0
	for _, commit := range commits {
		if mapRecentCommitNoisy(commit) {
			skipped++
			continue
		}
		query := mapRecentCommitQuery(commit)
		if query == "" {
			skipped++
			continue
		}
		label := displayMapLabel(query)
		key := normalizeMapKey(query)
		if key == "" {
			skipped++
			continue
		}
		builder := builders[key]
		if builder == nil {
			builder = &mapRecentTopicBuilder{
				Label:          label,
				Query:          strings.ToLower(strings.TrimSpace(query)),
				Key:            key,
				CommitSHAs:     map[string]bool{},
				PathSet:        map[string]bool{},
				EvidenceCounts: map[string]int{},
				Maintenance:    map[string]bool{},
				Order:          order,
			}
			builders[key] = builder
			order++
		}
		addMapRecentCommit(builder, commit)
	}
	topics := make([]mapRecentTopic, 0, len(builders))
	for _, builder := range builders {
		topic := builder.mapRecentTopic()
		if areaQuery != "" && !mapRecentTopicMatches(topic, areaQuery) {
			continue
		}
		topics = append(topics, topic)
	}
	sortMapRecentTopics(topics)
	if len(topics) > limit {
		topics = topics[:limit]
	}
	return topics, skipped
}

func sortMapRecentTopics(topics []mapRecentTopic) {
	sort.SliceStable(topics, func(i, j int) bool {
		if topics[i].Score != topics[j].Score {
			return topics[i].Score > topics[j].Score
		}
		return topics[i].Label < topics[j].Label
	})
}

func applyMapRecentBoundaryQuality(ctx context.Context, repoRoot string, commits []parsedFindGitCommit, topics []mapRecentTopic) {
	if len(topics) == 0 {
		return
	}
	files, _, _, err := listMapBoundaryFiles(ctx, repoRoot)
	if err != nil {
		debugLog("recent boundary file inventory unavailable: %v", err)
	}
	var areas []mapArea
	if len(files) > 0 {
		repoName := filepath.Base(filepath.Clean(repoRoot))
		areas, _, _ = buildPathBoundaryAreas(repoRoot, repoName, files, commits, mapRecentMaxTopics*3)
	}
	for i := range topics {
		if topics[i].EvidenceCounts["source"] > 0 || topics[i].EvidenceCounts["test"] > 0 {
			topics[i].QualitySignals = appendUniqueString(topics[i].QualitySignals, "source_test_support")
			topics[i].Score += 10
		}
		if len(areas) == 0 {
			continue
		}
		area, score := bestMapRecentBoundaryArea(topics[i], areas)
		if area == nil || score < 8 {
			continue
		}
		topics[i].BoundaryLabel = area.Label
		topics[i].BoundaryRole = area.BoundaryRole
		topics[i].BoundaryPaths = firstStrings(area.BoundaryPaths, 3)
		if len(topics[i].BoundaryPaths) == 0 {
			topics[i].BoundaryPaths = firstStrings(mapAreaBoundaryPaths(*area), 3)
		}
		topics[i].QualitySignals = appendUniqueString(topics[i].QualitySignals, "boundary_support")
		topics[i].Score += score
		if mapRecentTopicHasSourceTest(topics[i]) && mapRecentBoundaryAreaUseful(*area) {
			topics[i].QualitySignals = appendUniqueString(topics[i].QualitySignals, "useful_boundary")
			topics[i].Score += 12
		}
	}
	hasUsefulTopic := false
	for _, topic := range topics {
		if topic.TopicType != "maintenance" && mapRecentTopicHasUsefulEvidence(topic) {
			hasUsefulTopic = true
			break
		}
	}
	for i := range topics {
		if topics[i].TopicType != "maintenance" {
			continue
		}
		topics[i].QualitySignals = appendUniqueString(topics[i].QualitySignals, "maintenance")
		if hasUsefulTopic || mapRecentTopicMaintenanceKindSignal(topics[i]) == "repo-setup" {
			topics[i].QualitySignals = appendUniqueString(topics[i].QualitySignals, "maintenance_demoted")
			topics[i].Score -= mapRecentMaintenancePenalty(topics[i])
		}
	}
}

func mergeMapRecentOverlappingTopics(topics []mapRecentTopic) []mapRecentTopic {
	if len(topics) < 2 {
		return topics
	}
	merged := make([]mapRecentTopic, 0, len(topics))
	used := make([]bool, len(topics))
	for i := range topics {
		if used[i] {
			continue
		}
		current := topics[i]
		used[i] = true
		for changed := true; changed; {
			changed = false
			for j := range topics {
				if used[j] || !mapRecentTopicsShouldMerge(current, topics[j]) {
					continue
				}
				current = mergeMapRecentTopics(current, topics[j])
				used[j] = true
				changed = true
			}
		}
		merged = append(merged, current)
	}
	return merged
}

func mapRecentTopicsShouldMerge(a, b mapRecentTopic) bool {
	if a.Query == "" || b.Query == "" {
		return false
	}
	if a.TopicType == "maintenance" || b.TopicType == "maintenance" {
		if a.TopicType != b.TopicType {
			return false
		}
		aKind := mapRecentTopicMaintenanceKindSignal(a)
		bKind := mapRecentTopicMaintenanceKindSignal(b)
		if aKind == "" || aKind != bKind {
			return false
		}
		if mapRecentTopicPathOverlap(a, b) > 0 {
			return true
		}
		return mapRecentTopicQueryOverlap(a, b) >= 2
	}
	if mapRecentTopicHasSourceTest(a) || mapRecentTopicHasSourceTest(b) {
		if !mapRecentTopicHasSourceTest(a) || !mapRecentTopicHasSourceTest(b) {
			return false
		}
		sourceOverlap, testOverlap := mapRecentSourceTestPathOverlap(a, b)
		if sourceOverlap > 0 && testOverlap > 0 && mapRecentTopicQueryOverlap(a, b) >= 2 {
			return true
		}
		return false
	}
	if mapRecentTopicDocOnly(a) || mapRecentTopicDocOnly(b) {
		return false
	}
	if mapRecentTopicPathOverlap(a, b) > 0 && mapRecentTopicQueryOverlap(a, b) > 0 {
		return true
	}
	return false
}

func mapRecentTopicDocOnly(topic mapRecentTopic) bool {
	return topic.EvidenceCounts["doc"] > 0 &&
		topic.EvidenceCounts["source"] == 0 &&
		topic.EvidenceCounts["test"] == 0 &&
		topic.EvidenceCounts["config"] == 0
}

func mergeMapRecentTopics(a, b mapRecentTopic) mapRecentTopic {
	merged := a
	if b.Score > a.Score {
		merged = b
	}
	query := mapRecentMergedTopicQuery(a, b)
	merged.Query = query
	merged.Label = displayMapLabel(query)
	merged.Try = mapFindPackCommand(query)
	merged.CommitCount = mapRecentMergedCommitCount(a, b)
	merged.FileCount = mapRecentMergedFileCount(a, b)
	merged.EvidenceCounts = mapRecentMergedEvidenceCounts(a, b)
	merged.KeyPaths = mapRecentMergedKeyPaths(a, b)
	merged.TopicType = mapRecentMergedTopicType(a, b)
	merged.QualitySignals = mapRecentMergedQualitySignals(a, b)
	merged.RecentSignals = mapRecentMergedRecentSignals(a, b)
	mergeMapRecentBoundary(&merged, a, b)
	if merged.TopicType == "maintenance" {
		merged.BoundaryLabel = ""
		merged.BoundaryRole = ""
		merged.BoundaryPaths = nil
		merged.QualitySignals = appendUniqueString(merged.QualitySignals, "boundary_omitted_maintenance")
	}
	merged.Score = mapRecentMergedScore(a, b, merged)
	return merged
}

func mapRecentTopicMaintenanceKindSignal(topic mapRecentTopic) string {
	for _, signal := range topic.QualitySignals {
		if strings.HasPrefix(signal, "maintenance:") {
			return strings.TrimPrefix(signal, "maintenance:")
		}
	}
	return ""
}

func mapRecentTopicPathOverlap(a, b mapRecentTopic) int {
	paths := map[string]bool{}
	for _, path := range a.KeyPaths {
		if normalized := normalizeMapPath(path); normalized != "" {
			paths[normalized] = true
		}
	}
	count := 0
	for _, path := range b.KeyPaths {
		normalized := normalizeMapPath(path)
		if normalized != "" && paths[normalized] {
			count++
		}
	}
	return count
}

func mapRecentSourceTestPathOverlap(a, b mapRecentTopic) (int, int) {
	paths := map[string]bool{}
	for _, path := range a.KeyPaths {
		normalized := normalizeMapPath(path)
		if normalized == "" {
			continue
		}
		family := mapRecentPathFamily(normalized)
		if family == "source" || family == "test" {
			paths[normalized] = true
		}
	}
	sourceCount := 0
	testCount := 0
	for _, path := range b.KeyPaths {
		normalized := normalizeMapPath(path)
		if normalized == "" || !paths[normalized] {
			continue
		}
		family := mapRecentPathFamily(normalized)
		switch family {
		case "source":
			sourceCount++
		case "test":
			testCount++
		}
	}
	return sourceCount, testCount
}

func mapRecentTopicQueryOverlap(a, b mapRecentTopic) int {
	aWords := mapStringSet(mapRecentComparableWords(a.Query))
	count := 0
	for _, word := range mapRecentComparableWords(b.Query) {
		if aWords[word] {
			count++
		}
	}
	return count
}

func mapRecentMergedTopicQuery(a, b mapRecentTopic) string {
	if kind := mapRecentTopicMaintenanceKindSignal(a); kind != "" && kind == mapRecentTopicMaintenanceKindSignal(b) {
		switch kind {
		case "sponsors":
			return "sponsors updates"
		case "release-docs":
			if mapRecentTopicHasPathSubstring(a, "latest-changes") || mapRecentTopicHasPathSubstring(b, "latest-changes") {
				return "latest changes workflow"
			}
			return "release docs updates"
		case "workflow":
			if query := mapRecentMergedPathQuery(a, b); query != "" {
				return query
			}
			return "workflow updates"
		case "translation":
			return "translation updates"
		case "dependency":
			return "dependency updates"
		case "doc-archive":
			return "docs archive"
		case "docs":
			if common := mapRecentCommonComparableWords(a.Query, b.Query); len(common) >= 2 {
				return strings.Join(firstStrings(common, 4), " ")
			}
			if query := mapRecentMergedPathQuery(a, b); query != "" {
				return query
			}
			return "docs updates"
		case "config":
			if common := mapRecentCommonComparableWords(a.Query, b.Query); len(common) >= 2 {
				return strings.Join(firstStrings(common, 4), " ")
			}
			if query := mapRecentMergedPathQuery(a, b); query != "" {
				return query
			}
			return "config updates"
		case "repo-setup":
			if query := mapRecentMergedPathQuery(a, b); query != "" {
				return query
			}
			return "repo setup"
		}
	}
	common := mapRecentCommonComparableWords(a.Query, b.Query)
	if len(common) >= 2 {
		return strings.Join(firstStrings(common, 4), " ")
	}
	if query := mapRecentMergedPathQuery(a, b); query != "" {
		return query
	}
	if a.Score >= b.Score {
		return a.Query
	}
	return b.Query
}

func mapRecentMergedPathQuery(a, b mapRecentTopic) string {
	path := mapRecentFirstSharedPath(a, b)
	if path == "" {
		return ""
	}
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var words []string
	for _, word := range wordsFromMap(stem) {
		if !mapRecentStopWord(word) {
			words = appendUniqueString(words, word)
		}
	}
	if len(words) == 0 {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(normalizeMapPath(path)), ".github/workflows/") && !mapRecentContainsString(words, "workflow") {
		words = append(words, "workflow")
	}
	return strings.Join(firstStrings(words, 4), " ")
}

func mapRecentCommonComparableWords(a, b string) []string {
	aWords := mapStringSet(mapRecentComparableWords(a))
	var out []string
	for _, word := range mapRecentComparableWords(b) {
		if aWords[word] {
			out = appendUniqueString(out, word)
		}
	}
	return out
}

func mapRecentTopicHasPathSubstring(topic mapRecentTopic, want string) bool {
	want = strings.ToLower(want)
	for _, path := range topic.KeyPaths {
		if strings.Contains(strings.ToLower(normalizeMapPath(path)), want) {
			return true
		}
	}
	return false
}

func mapRecentFirstSharedPath(a, b mapRecentTopic) string {
	paths := map[string]bool{}
	for _, path := range a.KeyPaths {
		if normalized := normalizeMapPath(path); normalized != "" {
			paths[normalized] = true
		}
	}
	for _, path := range b.KeyPaths {
		normalized := normalizeMapPath(path)
		if normalized != "" && paths[normalized] {
			return normalized
		}
	}
	return ""
}

func mapRecentMergedCommitCount(a, b mapRecentTopic) int {
	count := a.CommitCount + b.CommitCount
	seen := map[string]bool{}
	overlap := 0
	for _, receipt := range a.RecentSignals {
		key := mapRecentReceiptKey(receipt)
		if key != "" {
			seen[key] = true
		}
	}
	for _, receipt := range b.RecentSignals {
		key := mapRecentReceiptKey(receipt)
		if key != "" && seen[key] {
			overlap++
		}
	}
	count -= overlap
	return maxInt(count, len(mapRecentMergedRecentSignals(a, b)))
}

func mapRecentMergedFileCount(a, b mapRecentTopic) int {
	count := a.FileCount + b.FileCount - mapRecentTopicPathOverlap(a, b)
	return maxInt(count, len(mapRecentMergedKeyPaths(a, b)))
}

func mapRecentMergedEvidenceCounts(a, b mapRecentTopic) map[string]int {
	out := copyIntMap(a.EvidenceCounts)
	if out == nil {
		out = map[string]int{}
	}
	for key, value := range b.EvidenceCounts {
		out[key] += value
	}
	return out
}

func mapRecentMergedKeyPaths(a, b mapRecentTopic) []string {
	seen := map[string]bool{}
	var paths []string
	for _, topic := range []mapRecentTopic{a, b} {
		for _, path := range topic.KeyPaths {
			normalized := normalizeMapPath(path)
			if normalized == "" || seen[normalized] {
				continue
			}
			seen[normalized] = true
			paths = append(paths, normalized)
		}
	}
	sort.Strings(paths)
	return firstStrings(mapRecentKeyPaths(paths), 4)
}

func mapRecentMergedTopicType(a, b mapRecentTopic) string {
	if a.TopicType == b.TopicType {
		return a.TopicType
	}
	if a.TopicType == "" {
		return b.TopicType
	}
	if b.TopicType == "" {
		return a.TopicType
	}
	return ""
}

func mapRecentMergedQualitySignals(a, b mapRecentTopic) []string {
	var signals []string
	for _, signal := range a.QualitySignals {
		signals = appendUniqueString(signals, signal)
	}
	for _, signal := range b.QualitySignals {
		signals = appendUniqueString(signals, signal)
	}
	return signals
}

func mapRecentMergedRecentSignals(a, b mapRecentTopic) []mapTraceReceipt {
	seen := map[string]bool{}
	var receipts []mapTraceReceipt
	for _, topic := range []mapRecentTopic{a, b} {
		for _, receipt := range topic.RecentSignals {
			key := mapRecentReceiptKey(receipt)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			receipts = append(receipts, receipt)
			if len(receipts) >= mapMaxTraceReceipts {
				return receipts
			}
		}
	}
	return receipts
}

func mapRecentReceiptKey(receipt mapTraceReceipt) string {
	if receipt.SHA != "" {
		return strings.ToLower(receipt.SHA)
	}
	return strings.ToLower(strings.TrimSpace(receipt.Subject))
}

func mergeMapRecentBoundary(merged *mapRecentTopic, a, b mapRecentTopic) {
	switch {
	case a.BoundaryLabel == "":
		merged.BoundaryLabel = b.BoundaryLabel
		merged.BoundaryRole = b.BoundaryRole
		merged.BoundaryPaths = append([]string{}, b.BoundaryPaths...)
	case b.BoundaryLabel == "":
		merged.BoundaryLabel = a.BoundaryLabel
		merged.BoundaryRole = a.BoundaryRole
		merged.BoundaryPaths = append([]string{}, a.BoundaryPaths...)
	case a.BoundaryLabel == b.BoundaryLabel && a.BoundaryRole == b.BoundaryRole:
		merged.BoundaryLabel = a.BoundaryLabel
		merged.BoundaryRole = a.BoundaryRole
		merged.BoundaryPaths = mapRecentMergedBoundaryPaths(a, b)
	default:
		merged.BoundaryLabel = ""
		merged.BoundaryRole = ""
		merged.BoundaryPaths = nil
		merged.QualitySignals = appendUniqueString(merged.QualitySignals, "boundary_ambiguous")
	}
}

func mapRecentMergedBoundaryPaths(a, b mapRecentTopic) []string {
	var paths []string
	for _, path := range a.BoundaryPaths {
		paths = appendUniqueString(paths, path)
	}
	for _, path := range b.BoundaryPaths {
		paths = appendUniqueString(paths, path)
	}
	return firstStrings(paths, 3)
}

func mapRecentMergedScore(a, b, merged mapRecentTopic) int {
	score := maxInt(a.Score, b.Score)
	extraCommits := maxInt(0, merged.CommitCount-maxInt(a.CommitCount, b.CommitCount))
	bonus := mapMinInt(extraCommits*4, 12)
	if merged.TopicType == "maintenance" {
		bonus = mapMinInt(bonus, 6)
	}
	return score + bonus
}

func mapRecentMaintenancePenalty(topic mapRecentTopic) int {
	penalty := 80
	if topic.FileCount > 20 {
		penalty += (topic.FileCount - 20) * 2
	}
	return penalty
}

func bestMapRecentBoundaryArea(topic mapRecentTopic, areas []mapArea) (*mapArea, int) {
	var best *mapArea
	bestScore := 0
	for i := range areas {
		area := &areas[i]
		score := mapRecentBoundaryMatchScore(topic, *area)
		if score > bestScore {
			best = area
			bestScore = score
		}
	}
	return best, bestScore
}

func mapRecentBoundaryMatchScore(topic mapRecentTopic, area mapArea) int {
	score := 0
	topicKey := normalizeMapKey(topic.Query)
	if topicKey != "" {
		score += scoreMapKeyMatch(normalizeMapKey(area.Label), topicKey) / 10
		for _, cover := range area.Covers {
			score += scoreMapKeyMatch(normalizeMapKey(cover), topicKey) / 14
		}
	}
	for _, topicPath := range topic.KeyPaths {
		topicPath = normalizeMapPath(topicPath)
		for _, areaPath := range area.KeyPaths {
			if mapRecentPathWithinBoundary(topicPath, areaPath) {
				score += 7
			}
		}
		for _, boundaryPath := range area.BoundaryPaths {
			if mapRecentPathWithinBoundary(topicPath, boundaryPath) {
				score += 9
			}
		}
		for _, raw := range area.Diagnostics.RawAnchors {
			if mapRecentPathWithinBoundary(topicPath, raw) {
				score += 6
			}
		}
	}
	if topic.EvidenceCounts["source"] > 0 && area.EvidenceCounts["source"] > 0 {
		score += 4
	}
	if topic.EvidenceCounts["test"] > 0 && area.EvidenceCounts["test"] > 0 {
		score += 4
	}
	if mapRecentBoundaryAreaUseful(area) {
		score += 3
	}
	if !mapRecentTopicHasSourceTest(topic) {
		score = mapMinInt(score, 20)
	}
	return score
}

func mapRecentPathWithinBoundary(path, boundary string) bool {
	path = strings.Trim(normalizeMapPath(path), "/")
	boundary = strings.Trim(normalizeMapPath(strings.TrimSuffix(boundary, "/**")), "/")
	if path == "" || boundary == "" || boundary == "." {
		return false
	}
	return path == boundary || strings.HasPrefix(path, boundary+"/") || strings.HasPrefix(boundary, path+"/")
}

func mapRecentBoundaryAreaUseful(area mapArea) bool {
	if area.BoundaryRole == mapBoundaryRoleDocsReference ||
		area.BoundaryRole == mapBoundaryRoleHorizontalLayer ||
		area.BoundaryRole == mapBoundaryRoleGenericParent ||
		area.BoundaryRole == mapBoundaryRoleFixtureOrTestbed ||
		area.BoundaryRole == mapBoundaryRoleRepoNamespace ||
		area.BoundaryRole == mapBoundaryRoleHandoffUnsafe {
		return false
	}
	return area.EvidenceCounts["source"] > 0 || area.EvidenceCounts["test"] > 0
}

func mapRecentTopicHasUsefulEvidence(topic mapRecentTopic) bool {
	if mapRecentTopicHasSourceTest(topic) {
		return true
	}
	for _, signal := range topic.QualitySignals {
		if signal == "useful_boundary" {
			return true
		}
	}
	return false
}

func mapRecentTopicHasSourceTest(topic mapRecentTopic) bool {
	return topic.EvidenceCounts["source"] > 0 || topic.EvidenceCounts["test"] > 0
}

func addMapRecentCommit(builder *mapRecentTopicBuilder, commit parsedFindGitCommit) {
	if commit.sha != "" && !builder.CommitSHAs[commit.sha] {
		builder.CommitSHAs[commit.sha] = true
		builder.Score += 12
	}
	if len(builder.RecentSignals) < mapMaxTraceReceipts {
		builder.RecentSignals = append(builder.RecentSignals, mapTraceReceipt{
			SHA:     shortFindGitSHA(commit.sha),
			Subject: limitRunes(commit.subject, 120),
		})
	}
	for _, path := range commit.paths {
		path = normalizeFindGitReceiptPath(path)
		if path == "" || mapRecentPathNoisy(path) || builder.PathSet[path] {
			continue
		}
		builder.PathSet[path] = true
		builder.Score += 2
		family := mapRecentPathFamily(path)
		if family != "" {
			builder.EvidenceCounts[family]++
		}
	}
	if len(commit.paths) > 1 {
		builder.Score += 3
	}
	if len(builder.EvidenceCounts) >= 2 {
		builder.Score += 4
	}
	if findGitWorkRefPattern.MatchString(commit.subject + "\n" + commit.body) {
		builder.Score += 2
	}
	if kind := mapRecentMaintenanceKind(commit, builder.EvidenceCounts); kind != "" {
		builder.Maintenance[kind] = true
	}
}

func mapRecentTopicType(builder *mapRecentTopicBuilder) string {
	if len(builder.Maintenance) == 0 {
		return ""
	}
	if builder.EvidenceCounts["source"] > 0 || builder.EvidenceCounts["test"] > 0 {
		return ""
	}
	return "maintenance"
}

func mapRecentBuilderQualitySignals(builder *mapRecentTopicBuilder) []string {
	if len(builder.Maintenance) == 0 {
		return nil
	}
	var kinds []string
	for kind := range builder.Maintenance {
		kinds = append(kinds, "maintenance:"+kind)
	}
	sort.Strings(kinds)
	return kinds
}

func mapRecentMaintenanceKind(commit parsedFindGitCommit, evidence map[string]int) string {
	if evidence["source"] > 0 || evidence["test"] > 0 {
		return ""
	}
	subject := strings.ToLower(commit.subject)
	switch {
	case mapRecentCommitRepoSetupOnly(commit):
		return "repo-setup"
	case strings.Contains(subject, "sponsor"):
		return "sponsors"
	case strings.Contains(subject, "translation") || strings.Contains(subject, "translate") || mapRecentCommitTouchesLocaleDocs(commit):
		return "translation"
	case strings.Contains(subject, "release notes") ||
		strings.Contains(subject, "latest-changes") ||
		strings.Contains(subject, "latest changes"):
		return "release-docs"
	case strings.Contains(subject, "dependency") ||
		strings.Contains(subject, "dependencies") ||
		strings.Contains(subject, "bump ") ||
		strings.Contains(subject, "update to "):
		return "dependency"
	case mapRecentCommitWorkflowOnly(commit):
		return "workflow"
	case mapRecentCommitBulkDocsOnly(commit):
		return "doc-archive"
	case mapRecentCommitGenericDocsUpdate(commit):
		return "docs"
	case mapRecentCommitGenericConfigUpdate(commit):
		return "config"
	default:
		return ""
	}
}

func mapRecentCommitRepoSetupOnly(commit parsedFindGitCommit) bool {
	subject := strings.ToLower(strings.TrimSpace(stripMapCommitPrefix(commit.subject)))
	if subject != "initial commit" && subject != "first commit" {
		return false
	}
	if len(commit.paths) == 0 {
		return false
	}
	hasLicenseLikePath := false
	for _, path := range commit.paths {
		base := strings.ToLower(filepath.Base(normalizeMapPath(path)))
		switch base {
		case "license", "copying", "notice", "readme.md", ".gitignore":
			if base == "license" || base == "copying" || base == "notice" {
				hasLicenseLikePath = true
			}
			continue
		default:
			return false
		}
	}
	return hasLicenseLikePath
}

func mapRecentCommitGenericDocsUpdate(commit parsedFindGitCommit) bool {
	if !mapRecentCommitDocsOnly(commit) {
		return false
	}
	subject := strings.ToLower(commit.subject)
	hasGenericVerb := strings.Contains(subject, "update") || strings.Contains(subject, "updated") || strings.Contains(subject, "updates") || strings.Contains(subject, "fix")
	hasDocTarget := strings.Contains(subject, "readme") || strings.Contains(subject, "docs") || strings.Contains(subject, "documentation")
	if !hasGenericVerb || !hasDocTarget {
		return false
	}
	return mapRecentMaintenanceSpecificTermCount(subject, mapRecentDocsMaintenanceGenericWords) <= 1
}

func mapRecentCommitGenericConfigUpdate(commit parsedFindGitCommit) bool {
	if !mapRecentCommitConfigOnly(commit) {
		return false
	}
	subject := strings.ToLower(commit.subject)
	if !strings.Contains(subject, "update") && !strings.Contains(subject, "fix") {
		return false
	}
	return mapRecentMaintenanceSpecificTermCount(subject, mapRecentConfigMaintenanceGenericWords) <= 1
}

func mapRecentMaintenanceSpecificTermCount(subject string, generic map[string]bool) int {
	count := 0
	for _, word := range mapRecentSubjectTerms(subject) {
		if generic[word] || mapTryGeneratedLeafWord(word) {
			continue
		}
		count++
	}
	return count
}

var mapRecentDocsMaintenanceGenericWords = map[string]bool{
	"doc":           true,
	"docs":          true,
	"documentation": true,
	"readme":        true,
	"reference":     true,
	"references":    true,
	"update":        true,
	"updated":       true,
	"updates":       true,
}

var mapRecentConfigMaintenanceGenericWords = map[string]bool{
	"config":        true,
	"configuration": true,
	"fix":           true,
	"fixed":         true,
	"update":        true,
	"updated":       true,
	"updates":       true,
}

func mapRecentCommitTouchesLocaleDocs(commit parsedFindGitCommit) bool {
	for _, path := range commit.paths {
		lower := strings.ToLower(normalizeMapPath(path))
		if strings.HasPrefix(lower, "docs/") && strings.Count(lower, "/") >= 2 {
			parts := strings.Split(lower, "/")
			if len(parts) > 1 && len(parts[1]) == 2 {
				return true
			}
		}
	}
	return false
}

func mapRecentCommitWorkflowOnly(commit parsedFindGitCommit) bool {
	if len(commit.paths) == 0 {
		return false
	}
	for _, path := range commit.paths {
		if !strings.HasPrefix(strings.ToLower(normalizeMapPath(path)), ".github/workflows/") {
			return false
		}
	}
	return true
}

func mapRecentCommitDocsOnly(commit parsedFindGitCommit) bool {
	if len(commit.paths) == 0 {
		return false
	}
	for _, path := range commit.paths {
		if mapRecentPathFamily(path) != "doc" {
			return false
		}
	}
	return true
}

func mapRecentCommitBulkDocsOnly(commit parsedFindGitCommit) bool {
	if !mapRecentCommitDocsOnly(commit) {
		return false
	}
	if len(commit.paths) > 20 {
		return true
	}
	subject := strings.ToLower(commit.subject)
	if strings.Contains(subject, "archive") || strings.HasPrefix(strings.TrimSpace(subject), "record ") {
		return true
	}
	for _, path := range commit.paths {
		lower := strings.ToLower(normalizeMapPath(path))
		if strings.Contains(lower, "/archive/") ||
			strings.Contains(lower, "/research-archive/") ||
			strings.Contains(lower, "/raw-samples/") ||
			strings.Contains(lower, "/raw-output-samples/") {
			return true
		}
	}
	return false
}

func mapRecentCommitConfigOnly(commit parsedFindGitCommit) bool {
	if len(commit.paths) == 0 {
		return false
	}
	for _, path := range commit.paths {
		if mapRecentPathFamily(path) != "config" {
			return false
		}
	}
	return true
}

func (builder *mapRecentTopicBuilder) mapRecentTopic() mapRecentTopic {
	paths := make([]string, 0, len(builder.PathSet))
	for path := range builder.PathSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return mapRecentTopic{
		Label:          builder.Label,
		Query:          builder.Query,
		CommitCount:    len(builder.CommitSHAs),
		FileCount:      len(builder.PathSet),
		EvidenceCounts: builder.EvidenceCounts,
		KeyPaths:       firstStrings(mapRecentKeyPaths(paths), 4),
		TopicType:      mapRecentTopicType(builder),
		QualitySignals: mapRecentBuilderQualitySignals(builder),
		RecentSignals:  builder.RecentSignals,
		Try:            mapFindPackCommand(builder.Query),
		Score:          builder.Score,
	}
}

func mapRecentKeyPaths(paths []string) []string {
	sort.SliceStable(paths, func(i, j int) bool {
		return mapRecentPathRank(paths[i]) < mapRecentPathRank(paths[j])
	})
	return paths
}

func mapRecentPathRank(path string) int {
	switch mapRecentPathFamily(path) {
	case "source":
		return 0
	case "test":
		return 1
	case "config":
		return 2
	case "doc":
		return 3
	default:
		return 4
	}
}

func mapRecentTopicMatches(topic mapRecentTopic, query string) bool {
	queryKey := normalizeMapKey(query)
	if queryKey == "" {
		return true
	}
	values := []string{topic.Label, topic.Query}
	values = append(values, topic.KeyPaths...)
	for _, receipt := range topic.RecentSignals {
		values = append(values, receipt.Subject)
	}
	for _, value := range values {
		if scoreMapKeyMatch(normalizeMapKey(value), queryKey) > 0 {
			return true
		}
	}
	queryWords := mapRecentComparableWords(query)
	if len(queryWords) == 0 {
		return false
	}
	for _, value := range values {
		valueWords := mapStringSet(mapRecentComparableWords(value))
		matched := 0
		for _, word := range queryWords {
			if valueWords[word] {
				matched++
			}
		}
		if matched == len(queryWords) || (len(queryWords) > 2 && matched >= 2) {
			return true
		}
	}
	return false
}

func mapRecentCommitQuery(commit parsedFindGitCommit) string {
	terms := mapRecentSubjectTerms(commit.subject)
	if len(terms) < 2 {
		pathTerms := mapRecentPathTerms(commit.paths)
		for _, term := range pathTerms {
			if len(terms) >= 4 {
				break
			}
			if !mapRecentContainsString(terms, term) {
				terms = append(terms, term)
			}
		}
	}
	if len(terms) == 0 {
		return ""
	}
	if len(terms) > 4 {
		terms = terms[:4]
	}
	return strings.Join(terms, " ")
}

func mapRecentSubjectTerms(subject string) []string {
	subject = stripMapCommitPrefix(subject)
	var terms []string
	for _, word := range wordsFromMap(subject) {
		if mapRecentStopWord(word) {
			continue
		}
		terms = appendUniqueString(terms, word)
		if len(terms) >= 4 {
			break
		}
	}
	return terms
}

func mapRecentContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func mapRecentPathTerms(paths []string) []string {
	counts := map[string]int{}
	for _, path := range paths {
		if mapRecentPathNoisy(path) {
			continue
		}
		parts := strings.Split(normalizeMapPath(path), "/")
		for _, part := range parts {
			stem := strings.TrimSuffix(part, filepath.Ext(part))
			for _, word := range wordsFromMap(stem) {
				if mapRecentStopWord(word) {
					continue
				}
				counts[word]++
			}
		}
	}
	type countedTerm struct {
		term  string
		count int
	}
	var counted []countedTerm
	for term, count := range counts {
		counted = append(counted, countedTerm{term: term, count: count})
	}
	sort.SliceStable(counted, func(i, j int) bool {
		if counted[i].count != counted[j].count {
			return counted[i].count > counted[j].count
		}
		return counted[i].term < counted[j].term
	})
	var out []string
	for _, item := range counted {
		out = append(out, item.term)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func mapRecentCommitNoisy(commit parsedFindGitCommit) bool {
	if findGitReceiptCommitNoisy(commit) {
		return true
	}
	if len(commit.paths) == 0 {
		return true
	}
	subject := strings.ToLower(commit.subject)
	switch {
	case strings.HasPrefix(subject, "merge ") ||
		strings.HasPrefix(subject, "revert ") ||
		strings.HasPrefix(subject, "release") ||
		strings.Contains(subject, "dependabot") ||
		strings.Contains(subject, "renovate") ||
		strings.Contains(subject, "lockfile") ||
		strings.Contains(subject, "pre-commit") ||
		strings.Contains(subject, "typo") ||
		strings.Contains(subject, "typos") ||
		strings.Contains(subject, "update docs") ||
		strings.Contains(subject, "docs references"):
		return true
	case (strings.Contains(subject, "dependencies") ||
		strings.Contains(subject, "translation") ||
		strings.Contains(subject, "translations")) &&
		!mapRecentCommitHasSourceTestPath(commit):
		return true
	case strings.Contains(subject, "coverage reports") ||
		strings.Contains(subject, "update release notes") ||
		strings.Contains(subject, "move tests to correct location") ||
		strings.Contains(subject, "update yarn") ||
		strings.Contains(subject, "working/agents tracker") ||
		strings.Contains(subject, "refactor architecture") ||
		strings.Contains(subject, "audit feedback") ||
		strings.Contains(subject, "pivot planning document") ||
		(strings.Contains(subject, "initial") && strings.Contains(subject, "scaffold")) ||
		(strings.Contains(subject, "foundation") && (strings.Contains(subject, "phase") || strings.Contains(subject, "hpases"))):
		return true
	}
	lockOnly := true
	hasUsefulPath := false
	for _, path := range commit.paths {
		lower := strings.ToLower(filepath.ToSlash(path))
		if !strings.HasSuffix(lower, ".lock") && !strings.HasSuffix(lower, "/yarn.lock") && !strings.HasSuffix(lower, "/package-lock.json") {
			lockOnly = false
		}
		if !mapRecentPathNoisy(path) {
			hasUsefulPath = true
		}
	}
	return lockOnly || !hasUsefulPath
}

func mapRecentCommitHasSourceTestPath(commit parsedFindGitCommit) bool {
	for _, path := range commit.paths {
		family := mapRecentPathFamily(path)
		if family == "source" || family == "test" {
			return true
		}
	}
	return false
}

func mapRecentPathNoisy(path string) bool {
	parts := strings.Split(normalizeMapPath(path), "/")
	for _, part := range parts {
		if mapSuppressedPathSegments[part] || part == "coverage" || part == ".next" {
			return true
		}
	}
	lower := strings.ToLower(filepath.ToSlash(path))
	return strings.Contains(lower, "/coverage/") ||
		strings.Contains(lower, "/.devspecs/tasks/") ||
		strings.HasPrefix(lower, ".devspecs/tasks/") ||
		strings.HasPrefix(lower, "devspecs/tasks/") ||
		strings.Contains(lower, "/snapshots/") ||
		strings.HasSuffix(lower, "cover.out") ||
		strings.HasSuffix(lower, "lcov.info") ||
		strings.HasSuffix(lower, "clover.xml") ||
		strings.HasSuffix(lower, ".snap")
}

func mapRecentComparableWords(value string) []string {
	var out []string
	for _, word := range wordsFromMap(value) {
		if mapRecentStopWord(word) {
			continue
		}
		word = strings.TrimSuffix(word, "s")
		if word != "" {
			out = appendUniqueString(out, word)
		}
	}
	return out
}

func mapRecentStopWord(word string) bool {
	switch word {
	case "api", "app", "auth", "backend", "cookie", "frontend", "oauth", "private", "public", "router", "routing", "sql", "swagger", "yaml", "yml", "json", "markdown":
		return false
	}
	if word == "e" || word == "g" || word == "eg" || word == "i" || word == "ie" {
		return true
	}
	if mapTraceStopWord(word) {
		return true
	}
	switch word {
	case "feat", "feature", "features", "implement", "implements", "implemented",
		"complete", "completed", "prepare", "preparation", "release", "notes",
		"dependency", "dependencies", "bump", "chore", "minor", "major", "patch",
		"phase", "phases", "step", "steps", "wip", "work", "todo", "todos",
		"near", "based", "expected", "actual", "anonymous", "context", "contexts",
		"working", "tracker", "expand", "architecture", "audit", "feedback", "hpases",
		"a", "an", "open", "opened", "opening", "up", "error", "errors", "gap", "gaps",
		"seal", "sealed", "loose", "few", "end", "ends", "spec",
		"to", "in", "of", "instead":
		return true
	default:
		return false
	}
}

func mapRecentPathFamily(path string) string {
	lower := strings.ToLower(filepath.ToSlash(path))
	ext := strings.ToLower(filepath.Ext(lower))
	base := strings.ToLower(filepath.Base(lower))
	switch {
	case strings.Contains(lower, "/test/") ||
		strings.Contains(lower, "/tests/") ||
		strings.HasPrefix(lower, "test/") ||
		strings.HasPrefix(lower, "tests/") ||
		strings.Contains(lower, "__tests__") ||
		strings.Contains(lower, "_test.") ||
		strings.Contains(lower, ".test.") ||
		strings.Contains(lower, ".spec."):
		return "test"
	case strings.Contains(lower, "/docs/") ||
		strings.HasPrefix(lower, "docs/") ||
		ext == ".md" || ext == ".mdx" || ext == ".rst" || ext == ".adoc":
		return "doc"
	case base == "license" || base == "copying" || base == "notice":
		return "doc"
	case ext == ".sql" ||
		strings.Contains(lower, "/migrations/") ||
		base == ".gitignore" ||
		base == ".dockerignore" ||
		base == "makefile" ||
		base == "dockerfile" ||
		strings.HasSuffix(lower, "package.json") ||
		strings.HasSuffix(lower, "go.mod") ||
		strings.HasSuffix(lower, "cargo.toml") ||
		strings.HasSuffix(lower, ".yaml") ||
		strings.HasSuffix(lower, ".yml") ||
		strings.HasSuffix(lower, ".json"):
		return "config"
	default:
		return "source"
	}
}

func writeMapRecentText(out io.Writer, m mapRecentOutput, verbose bool) {
	if m.AreaQuery != "" {
		fmt.Fprintf(out, "Recently active topics matching: %s\n", m.AreaQuery)
	} else {
		fmt.Fprintln(out, "Recently active topics")
	}
	fmt.Fprintf(out, "Repo: %s\n\n", m.Repo.Name)
	if len(m.Topics) == 0 {
		fmt.Fprintln(out, "No recent topics found.")
		writeMapCaveats(out, m.Caveats)
		return
	}
	for i, topic := range m.Topics {
		fmt.Fprintf(out, "%d. %s\n", i+1, topic.Label)
		if topic.TopicType == "maintenance" {
			fmt.Fprintln(out, "   Topic type: maintenance")
		}
		if topic.BoundaryLabel != "" {
			fmt.Fprintf(out, "   Boundary: %s\n", topic.BoundaryLabel)
		}
		fmt.Fprintf(out, "   Evidence: %s\n", mapRecentEvidenceText(topic))
		if len(topic.RecentSignals) == 1 {
			receipt := topic.RecentSignals[0]
			if receipt.SHA != "" {
				fmt.Fprintf(out, "   Recent signal: %s %s\n", receipt.SHA, receipt.Subject)
			} else {
				fmt.Fprintf(out, "   Recent signal: %s\n", receipt.Subject)
			}
		} else if len(topic.RecentSignals) > 1 {
			fmt.Fprintln(out, "   Recent signals:")
			for _, receipt := range topic.RecentSignals {
				if receipt.SHA != "" {
					fmt.Fprintf(out, "   - %s %s\n", receipt.SHA, receipt.Subject)
				} else {
					fmt.Fprintf(out, "   - %s\n", receipt.Subject)
				}
			}
		}
		if len(topic.KeyPaths) > 0 {
			fmt.Fprintln(out, "   Key files:")
			for _, path := range firstStrings(topic.KeyPaths, 3) {
				fmt.Fprintf(out, "   - %s\n", path)
			}
		}
		if topic.Try != "" {
			fmt.Fprintf(out, "   Try: %s\n", topic.Try)
		}
		if verbose {
			fmt.Fprintf(out, "   Diagnostics: score=%d query=%q files=%d commits=%d\n", topic.Score, topic.Query, topic.FileCount, topic.CommitCount)
			if len(topic.QualitySignals) > 0 {
				fmt.Fprintf(out, "   Quality signals: %s\n", strings.Join(topic.QualitySignals, ", "))
			}
		}
		fmt.Fprintln(out)
	}
	writeMapCaveats(out, m.Caveats)
}

func mapRecentEvidenceText(topic mapRecentTopic) string {
	var parts []string
	if topic.CommitCount == 1 {
		parts = append(parts, "1 commit")
	} else {
		parts = append(parts, fmt.Sprintf("%d commits", topic.CommitCount))
	}
	if topic.FileCount == 1 {
		parts = append(parts, "1 file")
	} else {
		parts = append(parts, fmt.Sprintf("%d files", topic.FileCount))
	}
	if families := mapAreaEvidenceText(topic.EvidenceCounts); families != "local evidence" {
		parts = append(parts, families)
	}
	return strings.Join(parts, ", ")
}

func refineMapAreaForDrilldownQuery(area mapArea, query string) mapArea {
	query = strings.TrimSpace(query)
	if query == "" {
		return area
	}
	renamed := false
	if isMapDocsArea(area) {
		if cover := bestMapCoverForQuery(area.Covers, query); cover != "" {
			area.Label = cover
			area.Covers = removeMapCover(area.Covers, cover)
			renamed = true
		}
	}
	area.KeyPaths = collapseMapLocalizedKeyPaths(area.KeyPaths, query)
	if renamed {
		area.Try = mapTryCommandForRole(area.Label, area.Covers, area.TraceReceipts, area.Confidence, area.KeyPaths, area.BoundaryRole)
	}
	return area
}

func isMapDocsArea(area mapArea) bool {
	return area.AreaType == mapTypeDocs || area.Class == mapClassDocTopic || strings.HasPrefix(normalizeMapKey(area.Label), "docs/")
}

func bestMapCoverForQuery(covers []string, query string) string {
	queryKey := normalizeMapKey(query)
	if queryKey == "" {
		return ""
	}
	bestScore := 0
	best := ""
	for _, cover := range covers {
		key := normalizeMapKey(cover)
		if key == "" {
			continue
		}
		score := scoreMapKeyMatch(key, queryKey)
		if score > bestScore {
			bestScore = score
			best = cover
		}
	}
	if bestScore < 50 {
		return ""
	}
	return best
}

func scoreMapKeyMatch(key, queryKey string) int {
	keySingular := strings.TrimSuffix(key, "s")
	querySingular := strings.TrimSuffix(queryKey, "s")
	switch {
	case key == queryKey:
		return 100
	case len(keySingular) >= 4 && keySingular == querySingular:
		return 96
	case strings.HasPrefix(key, queryKey+"-") || strings.HasPrefix(key, queryKey+"/"):
		return 82
	case strings.Contains(key, "-"+queryKey+"-") || strings.Contains(key, "/"+queryKey+"/"):
		return 72
	case strings.Contains(key, queryKey) && len(queryKey) >= 4:
		return 58
	case len(key) >= 4 && (strings.HasPrefix(queryKey, key+"-") || strings.HasSuffix(queryKey, "-"+key) || strings.Contains(queryKey, "-"+key+"-")):
		return 50
	default:
		return 0
	}
}

func removeMapCover(covers []string, remove string) []string {
	removeKey := normalizeMapKey(remove)
	var out []string
	for _, cover := range covers {
		if normalizeMapKey(cover) == removeKey {
			continue
		}
		out = append(out, cover)
	}
	return out
}

func collapseMapLocalizedKeyPaths(paths []string, query string) []string {
	if len(paths) <= 1 || mapQueryRequestsLocale(query) {
		return paths
	}
	type localizedPath struct {
		path   string
		locale string
	}
	groups := map[string][]localizedPath{}
	groupOrder := []string{}
	for _, path := range paths {
		key, locale, ok := mapLocalizedPathKey(path)
		if !ok {
			groupOrder = append(groupOrder, path)
			groups[path] = append(groups[path], localizedPath{path: path})
			continue
		}
		if _, seen := groups[key]; !seen {
			groupOrder = append(groupOrder, key)
		}
		groups[key] = append(groups[key], localizedPath{path: path, locale: locale})
	}
	var out []string
	for _, key := range groupOrder {
		candidates := groups[key]
		if len(candidates) == 0 {
			continue
		}
		chosen := candidates[0]
		for _, candidate := range candidates {
			if candidate.locale == "en" {
				chosen = candidate
				break
			}
		}
		out = append(out, chosen.path)
	}
	return out
}

func mapLocalizedPathKey(path string) (string, string, bool) {
	parts := strings.Split(normalizeMapPath(path), "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "docs" && parts[i+2] == "docs" && isMapLocaleSegment(parts[i+1]) {
			keyParts := append([]string{}, parts...)
			keyParts[i+1] = "{locale}"
			return strings.Join(keyParts, "/"), parts[i+1], true
		}
	}
	return "", "", false
}

func mapQueryRequestsLocale(query string) bool {
	for _, word := range wordsFromMap(query) {
		if isMapLocaleSegment(word) {
			return true
		}
	}
	return false
}

func isMapLocaleSegment(segment string) bool {
	segment = strings.ToLower(strings.TrimSpace(segment))
	switch segment {
	case "ar", "de", "en", "es", "fa", "fr", "id", "it", "ja", "ko", "nl", "pl", "pt", "ru", "tr", "uk", "vi",
		"pt-br", "zh-cn", "zh-hans", "zh-hant":
		return true
	default:
		return false
	}
}

func filterMapOutputByAreaQuery(m mapOutput, query string) mapOutput {
	matches := matchMapAreas(m.Areas, query)
	filtered := make([]mapArea, 0, len(matches))
	for _, match := range matches {
		filtered = append(filtered, match.Area)
	}
	m.Areas = filtered
	m.Diagnostics.AreaQuery = query
	m.Diagnostics.MatchedAreaCount = len(filtered)
	if len(filtered) == 0 {
		m.Caveats = appendUniqueString(m.Caveats, "no map area matched the supplied area query")
	}
	return m
}

type mapAreaMatch struct {
	Area  mapArea
	Score int
}

func matchMapAreas(areas []mapArea, query string) []mapAreaMatch {
	queryKey := normalizeMapKey(query)
	if queryKey == "" {
		return nil
	}
	var matches []mapAreaMatch
	for _, area := range areas {
		score := scoreMapAreaMatch(area, queryKey)
		if score > 0 {
			matches = append(matches, mapAreaMatch{Area: area, Score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Area.Label < matches[j].Area.Label
		}
		return matches[i].Score > matches[j].Score
	})
	return matches
}

func scoreMapAreaMatch(area mapArea, queryKey string) int {
	best := 0
	primaryValues := []string{
		area.Label,
		area.Diagnostics.Key,
		strings.TrimPrefix(area.ID, "area."),
		strings.TrimPrefix(area.ID, "recent:"),
	}
	for _, value := range primaryValues {
		key := normalizeMapKey(value)
		if key == "" {
			continue
		}
		score := scoreMapKeyMatch(key, queryKey)
		if score > 0 {
			score += 30
		}
		if score > best {
			best = score
		}
	}
	for _, value := range area.Covers {
		key := normalizeMapKey(value)
		if key == "" {
			continue
		}
		score := scoreMapKeyMatch(key, queryKey)
		if score > 0 {
			score += 10
		}
		if score > best {
			best = score
		}
	}
	for _, value := range mapAreaSecondarySearchValues(area) {
		key := normalizeMapKey(value)
		if key == "" {
			continue
		}
		score := scoreMapKeyMatch(key, queryKey)
		if score > best {
			best = score
		}
	}
	if best > 0 && normalizeMapKey(area.AreaType) == queryKey {
		best += 8
	}
	return best
}

func mapAreaSecondarySearchValues(area mapArea) []string {
	values := []string{
		area.AreaType,
		displayMapAreaType(area.AreaType),
	}
	values = append(values, area.KeyPaths...)
	values = append(values, area.Diagnostics.RawAnchors...)
	values = append(values, area.Diagnostics.TraceTerms...)
	return values
}

func mapAreaPackCommands(area mapArea) []string {
	var commands []string
	if area.Diagnostics.Packability != nil && area.Diagnostics.Packability.TrySuppressed {
		return nil
	}
	if area.Try != "" {
		commands = appendUniqueString(commands, area.Try)
	}
	for _, cover := range firstStrings(area.Covers, 4) {
		if cmd := mapTryCommandForRole(area.Label, []string{cover}, nil, area.Confidence, area.KeyPaths, area.BoundaryRole); cmd != "" {
			commands = appendUniqueString(commands, cmd)
		}
	}
	for _, receipt := range firstMapTraceReceipts(area.TraceReceipts, 2) {
		if cmd := mapTryCommandForRole(area.Label, area.Covers, []mapTraceReceipt{receipt}, area.Confidence, area.KeyPaths, area.BoundaryRole); cmd != "" {
			commands = appendUniqueString(commands, cmd)
		}
	}
	if len(commands) == 0 && area.Label != "" {
		if cmd := mapTryCommandForRole(area.Label, area.Covers, area.TraceReceipts, area.Confidence, area.KeyPaths, area.BoundaryRole); cmd != "" {
			commands = append(commands, cmd)
		}
	}
	return firstStrings(commands, 4)
}

func relatedMapAreas(primary mapArea, areas []mapArea) []mapArea {
	var related []mapArea
	for _, area := range areas {
		if area.ID == primary.ID {
			continue
		}
		if area.AreaType == primary.AreaType && len(related) < 3 {
			related = append(related, area)
			continue
		}
		if len(related) < 3 && mapAreasShareCover(primary, area) {
			related = append(related, area)
		}
	}
	return firstMapAreas(related, 3)
}

func otherMapAreaMatches(matches []mapAreaMatch, related []mapArea) []mapAreaMatch {
	if len(matches) <= 1 {
		return nil
	}
	relatedIDs := map[string]bool{}
	for _, area := range related {
		relatedIDs[area.ID] = true
	}
	topScore := matches[0].Score
	var out []mapAreaMatch
	for _, match := range matches[1:] {
		if relatedIDs[match.Area.ID] {
			continue
		}
		if topScore >= 100 && match.Score < 90 {
			continue
		}
		out = append(out, match)
	}
	return out
}

func mapAreasShareCover(a, b mapArea) bool {
	terms := map[string]bool{}
	for _, value := range append(a.Covers, a.Label) {
		for _, word := range wordsFromMap(value) {
			if len(word) >= 4 && !mapGenericTerms[word] {
				terms[word] = true
			}
		}
	}
	for _, value := range append(b.Covers, b.Label) {
		for _, word := range wordsFromMap(value) {
			if terms[word] {
				return true
			}
		}
	}
	return false
}

func firstMapAreas(values []mapArea, limit int) []mapArea {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	out := make([]mapArea, limit)
	copy(out, values[:limit])
	return out
}

func firstMapAreaMatches(values []mapAreaMatch, limit int) []mapAreaMatch {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	out := make([]mapAreaMatch, limit)
	copy(out, values[:limit])
	return out
}

func writeMapCaveats(out io.Writer, caveats []string) {
	if len(caveats) == 0 {
		return
	}
	if len(caveats) == 1 {
		fmt.Fprintf(out, "Caveat: %s\n", caveats[0])
		return
	}
	fmt.Fprintln(out, "Caveats:")
	for _, caveat := range caveats {
		fmt.Fprintf(out, "- %s\n", caveat)
	}
}

func mapPackabilityDiagnosticsText(diag *mapPackabilityDiagnostics) string {
	if diag == nil {
		return ""
	}
	parts := []string{
		fmt.Sprintf("decision=%s", firstNonEmpty(diag.Decision, "unknown")),
		fmt.Sprintf("key=%d/%d", diag.IndexedKeyPathCount, diag.KeyPathCount),
		fmt.Sprintf("prefix=%d", diag.PrefixKeyPathCount),
		fmt.Sprintf("query_anchors=%d", diag.IndexedQueryAnchorCount),
	}
	if len(diag.MissingKeyExtensions) > 0 {
		parts = append(parts, "missing_ext="+strings.Join(diag.MissingKeyExtensions, ","))
	}
	if diag.TrySuppressed && diag.SuppressedTry != "" {
		parts = append(parts, "suppressed="+diag.SuppressedTry)
	}
	return strings.Join(parts, "; ")
}

func mapEvidence(result *scan.Result) mapEvidenceAvailability {
	found := map[string]int{}
	if result != nil {
		for k, v := range result.Found {
			found[k] = v
		}
	}
	return mapEvidenceAvailability{
		Markdown: found["markdown"],
		OpenSpec: found["openspec"],
		ADR:      found["adr"],
		Source:   found["source_context"],
		Test:     found["test_case"],
		Comment:  found["code_comment"],
		Intent:   found["markdown"] + found["openspec"] + found["adr"],
		Trace:    result != nil && result.GitEvidence != nil && result.GitEvidence.CommitsStored > 0,
	}
}

func mapEvidenceText(e mapEvidenceAvailability) string {
	var parts []string
	if e.Markdown > 0 {
		parts = append(parts, fmt.Sprintf("%d markdown", e.Markdown))
	}
	if e.OpenSpec > 0 {
		parts = append(parts, fmt.Sprintf("%d OpenSpec", e.OpenSpec))
	}
	if e.ADR > 0 {
		parts = append(parts, fmt.Sprintf("%d ADR", e.ADR))
	}
	if e.Source > 0 {
		parts = append(parts, fmt.Sprintf("%d source", e.Source))
	}
	if e.Test > 0 {
		parts = append(parts, fmt.Sprintf("%d tests", e.Test))
	}
	if e.Comment > 0 {
		parts = append(parts, fmt.Sprintf("%d code comments", e.Comment))
	}
	if e.Trace {
		parts = append(parts, "Git history")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func mapAreaEvidenceText(counts map[string]int) string {
	var parts []string
	for _, key := range []string{"source", "test", "import", "test_import", "intent", "doc", "protocol", "trace", "other"} {
		if counts[key] > 0 {
			switch key {
			case "source":
				parts = append(parts, "source")
			case "test":
				parts = append(parts, "tests")
			case "import":
				parts = append(parts, "import structure")
			case "test_import":
				parts = append(parts, "test->source")
			case "intent":
				parts = append(parts, "intent docs")
			case "doc":
				parts = append(parts, "docs")
			case "protocol":
				parts = append(parts, "protocol")
			case "trace":
				parts = append(parts, "Git")
			default:
				parts = append(parts, key)
			}
		}
	}
	if len(parts) == 0 {
		return "local evidence"
	}
	return strings.Join(parts, " + ")
}

func mapOutputConfidence(result *scan.Result, areas []mapArea) string {
	sourceish := 0
	if result != nil {
		sourceish = result.Found["source_context"] + result.Found["test_case"] + result.Found["code_comment"]
	}
	strong, specific, implementationSpecific, pull, misleading := 0, 0, 0, 0, 0
	for _, area := range areas {
		if area.Confidence == mapHighConfidence || area.Confidence == mapMediumConfidence {
			strong++
		}
		if mapPublicAreaSpecific(area) {
			specific++
			if area.EvidenceCounts["source"] > 0 {
				implementationSpecific++
			}
		}
		if area.Try != "" && area.Confidence != mapLowConfidence {
			pull++
		}
		for _, caveat := range area.Caveats {
			if strings.Contains(strings.ToLower(caveat), "misleading") {
				misleading++
			}
		}
	}
	if sourceish > 0 && len(areas) >= 3 && strong >= 3 && specific >= 3 && implementationSpecific >= 2 && pull >= 2 && misleading == 0 {
		return mapHighConfidence
	}
	if sourceish > 0 && len(areas) >= 2 && strong >= 2 && specific >= 1 && misleading <= 1 {
		return mapMediumConfidence
	}
	return mapLowConfidence
}

func mapPublicAreaSpecific(area mapArea) bool {
	key := normalizeMapKey(area.Label)
	if key == "" {
		return false
	}
	if mapWeakStandaloneAreaLabels[key] && len(area.Covers) < 2 {
		return false
	}
	if area.IsRepoRootUmbrella && len(area.Covers) < 2 {
		return false
	}
	if area.Confidence == mapHighConfidence && !mapWeakStandaloneAreaLabels[key] {
		return true
	}
	if strings.Contains(key, "-") || strings.Contains(key, "/") {
		return true
	}
	return len(area.Covers) >= 2
}

func mapOutputCaveats(repoRoot string, result *scan.Result, areas []mapArea, clusters []scan.WorkstreamClusterExample, confidence string) []string {
	var caveats []string
	if result == nil || scanTotalFound(result) == 0 {
		caveats = append(caveats, "No indexed artifacts were found in configured paths.")
	}
	if confidence == mapLowConfidence {
		caveats = append(caveats, "this output is orientation, not an architecture declaration")
	}
	if len(clusters) > len(areas) && len(areas) > 0 {
		caveats = append(caveats, fmt.Sprintf("merged %d raw signal cluster(s) into %d displayed area(s)", len(clusters), len(areas)))
	}
	if strings.Contains(filepath.ToSlash(repoRoot), "/_ignore/") {
		caveats = append(caveats, "repo path is under _ignore; use full checkouts for promotion claims")
	}
	return caveats
}

func classifyMapArea(area *mapAreaInternal) string {
	source := area.EvidenceCounts["source"] > 0 || area.EvidenceCounts["test"] > 0
	doc := area.EvidenceCounts["doc"] > 0 || area.EvidenceCounts["intent"] > 0
	protocol := area.EvidenceCounts["protocol"] > 0
	if protocol && !source && !doc {
		return mapClassProtocol
	}
	if !source && doc {
		return mapClassDocTopic
	}
	if source {
		return mapClassStableArea
	}
	return mapClassLowConfidence
}

func classifyMapAreaType(area *mapAreaInternal, label, class string, root bool, covers []string) string {
	if root {
		return mapTypeRoot
	}
	if class == mapClassProtocol {
		return mapTypeProtocol
	}
	if area != nil && area.EvidenceSources["conceptual_parent"] {
		if areaType := conceptualMapAreaType(area.Key, label); areaType != "" {
			return areaType
		}
	}

	values := mapAreaTypeValues(area, label, covers)
	primaryValues := mapAreaPrimaryTypeValues(area, label, covers)
	source := area.EvidenceCounts["source"] > 0
	test := area.EvidenceCounts["test"] > 0
	doc := area.EvidenceCounts["doc"] > 0 || area.EvidenceCounts["intent"] > 0

	if !source && test && !doc {
		return mapTypeTestQuality
	}
	if mapAnyContains(values, "docker", "compose", "deploy", "deployment", "helm", "kubernetes", "k8s", "terraform", "infra", "infrastructure", "postgres", "redis", "queue", "worker") {
		return mapTypeOps
	}
	if mapAnyContains(values, "flowable", "stripe", "github", "linear", "jira", "slack", "salesforce", "hubspot", "sendgrid", "mailgun", "twilio", "aws", "s3", "azure", "gcp", "openai", "anthropic", "kafka", "webhook") {
		return mapTypeExternal
	}
	if mapAnyContains(primaryValues, "httpapi", "api", "router", "route", "routes", "endpoint", "endpoints", "controller", "controllers", "handler", "handlers", "grpc", "graphql", "rest") {
		return mapTypeAPI
	}
	if mapAnyContains(values, "submission", "lead", "application", "booking", "order", "checkout", "billing", "payment", "invoice", "workflow", "approval", "review", "form", "claim", "policy", "quote", "customer", "admin") {
		return mapTypeBusinessFlow
	}
	if mapAnyContains(values, "httpapi", "api", "router", "route", "routes", "endpoint", "endpoints", "controller", "controllers", "handler", "handlers", "grpc", "graphql", "rest") {
		return mapTypeAPI
	}
	if mapAnyContains(values, "component", "components", "page", "pages", "screen", "screens", "view", "views", "frontend", "web", "tsx", "jsx", "css", "status-pill", "top-nav", "dashboard", "ui") {
		return mapTypeUI
	}
	if mapAnyContains(values, "script", "scripts", "cmd", "cli", "command", "commands", "generator", "generate", "build", "tool", "tools", "devtool", "devtools") {
		return mapTypeTooling
	}
	if mapAnyContains(values, "sync", "ingest", "ingestion", "pipeline", "etl", "extract", "transform", "loader", "backfill", "data-import", "data-export", "migration-job") {
		return mapTypeDataPipeline
	}
	if mapAnyContains(values, "model", "models", "schema", "schemas", "entity", "entities", "migration", "migrations", "database", "db", "sql", "psql", "prisma", "table", "tables") {
		return mapTypeDataModel
	}
	if mapAnyContains(values, "auth", "authentication", "authorization", "session", "permission", "permissions", "role", "roles", "tenant", "tenants", "config", "settings", "feature-flag", "feature-flags") {
		return mapTypePlatform
	}
	if !source && doc {
		return mapTypeDocs
	}
	if source || test {
		return mapTypeDomainFeature
	}
	return mapTypeUnknown
}

func classifyMapBoundaryRole(area *mapAreaInternal, label, areaType string, root bool, covers []string, repoName, repoShape string) string {
	if area == nil {
		return mapBoundaryRoleHandoffUnsafe
	}
	key := normalizeMapKey(firstNonEmpty(label, area.Key))
	values := mapBoundaryRoleValues(area, label, covers)
	if root || mapBoundaryRepoNamespaceLike(key, repoName) {
		return mapBoundaryRoleRepoNamespace
	}
	if areaType == mapTypeDocs || (!mapBoundaryRoleHasSource(area) && (area.EvidenceCounts["doc"] > 0 || area.EvidenceCounts["intent"] > 0)) {
		return mapBoundaryRoleDocsReference
	}
	if mapBoundaryFixtureOrTestbedLike(key, values) {
		return mapBoundaryRoleFixtureOrTestbed
	}
	if mapBoundaryExtensionEcosystemLike(key, values) {
		return mapBoundaryRoleExtensionEcosystem
	}
	if mapBoundaryGenericParentLike(key, values, repoShape) {
		return mapBoundaryRoleGenericParent
	}
	if mapBoundaryHorizontalLayerLike(key, values) {
		return mapBoundaryRoleHorizontalLayer
	}
	if mapBoundaryLabelNeedsSafeHandoff(key) {
		return mapBoundaryRoleHandoffUnsafe
	}
	return mapBoundaryRoleProductCapability
}

func mapBoundaryLabelNeedsSafeHandoff(key string) bool {
	if mapBoundaryFirstScreenShellLabels[key] {
		return true
	}
	if mapTryConceptualLabelNeedsSpecificContext(key) {
		return true
	}
	return mapBoundaryHandoffUnsafeKey(key)
}

func mapBoundaryRoleValues(area *mapAreaInternal, label string, covers []string) []string {
	values := []string{label}
	if area != nil {
		values = append(values, area.Key)
		values = append(values, area.RawAnchors...)
		for _, art := range area.Artifacts {
			values = append(values, art.Path, art.Title, art.Subtype)
		}
	}
	values = append(values, covers...)
	return values
}

func mapBoundaryRoleHasSource(area *mapAreaInternal) bool {
	if area == nil {
		return false
	}
	return area.EvidenceCounts["source"] > 0 || area.EvidenceCounts["test"] > 0 || area.EvidenceCounts["config"] > 0
}

func mapBoundaryRepoNamespaceLike(key, repoName string) bool {
	repoKey := normalizeMapKey(repoName)
	if repoKey == "" || key == "" {
		return false
	}
	if key == repoKey {
		return true
	}
	repoLead := repoKey
	if parts := strings.Split(repoKey, "-"); len(parts) > 0 {
		repoLead = parts[0]
	}
	if strings.HasPrefix(key, repoKey+"-") || (repoLead != "" && strings.HasPrefix(key, repoLead+"-")) {
		return mapAnyContains([]string{key}, "repository", "repositories", "repo", "repos", "meta", "objects", "platform", "namespace")
	}
	return false
}

func mapBoundaryFixtureOrTestbedLike(key string, values []string) bool {
	if mapAnyContains([]string{key}, "example", "examples", "fixture", "fixtures", "playground", "sample", "samples", "testbed", "testdata", "template", "templates") {
		return true
	}
	return mapAnyContains(firstStrings(values, 2), "example", "examples", "fixture", "fixtures", "galata", "mock", "mocks", "playground", "sample", "samples", "sandbox", "storybook", "testbed", "testdata", "template", "templates")
}

func mapBoundaryExtensionEcosystemLike(key string, values []string) bool {
	if mapAnyContains([]string{key}, "plugin", "plugins", "extension", "extensions", "provider", "providers", "adapter", "adapters", "connector", "connectors") {
		return true
	}
	return mapAnyContains(firstStrings(values, 2), "plugin", "plugins", "extension", "extensions", "provider", "providers", "adapter", "adapters", "connector", "connectors", "marketplace", "pdk")
}

func mapBoundaryGenericParentLike(key string, values []string, repoShape string) bool {
	switch key {
	case "api", "api-layer", "api-platform", "core", "engine", "framework", "http-api-layer", "objects", "platform", "public-api-layer", "public-http-api-developer-platform", "registry", "unified":
		return true
	case "command", "commands":
		return repoShape != mapRepoShapeTool
	}
	if mapAnyContains([]string{key}, "public-api-layer", "api-platform", "platform", "repo-namespace") {
		return true
	}
	if mapBoundaryMostlyBroadKey(key) && len(wordsFromMap(key)) <= 3 {
		return true
	}
	if mapAnyContains(values, "repo namespace", "umbrella", "generic parent") {
		return true
	}
	return false
}

func mapBoundaryHorizontalLayerLike(key string, values []string) bool {
	if mapAnyContains([]string{key}, "router", "routers", "route", "routes", "object", "objects", "locale", "locales", "style", "styles", "theme", "themes", "component", "components", "design-system") {
		return true
	}
	return mapAnyContains(firstStrings(values, 2), "router", "routers", "route", "routes", "locale", "locales", "style", "styles", "theme", "themes", "design-system", "ui primitive", "ui primitives")
}

func mapBoundaryHandoffUnsafeKey(key string) bool {
	if key == "" || mapBoundaryFirstScreenShellLabels[key] {
		return true
	}
	return mapAnyContains([]string{key}, "javascript", "locale", "locales", "example", "examples", "template", "templates", "utility", "utilities", "router")
}

func mapBoundaryMostlyBroadKey(key string) bool {
	words := wordsFromMap(key)
	if len(words) == 0 {
		return false
	}
	broad := 0
	for _, word := range words {
		if mapGenericTerms[word] || mapBoundaryHandoffBroadTerms[word] {
			broad++
		}
	}
	return broad == len(words)
}

func conceptualMapAreaType(key, label string) string {
	value := normalizeMapKey(firstNonEmpty(key, label))
	switch value {
	case "public-api-layer", "public-http-api-developer-platform", "external-http-api-v1", "http-api-layer":
		return mapTypeAPI
	case "metadata-engine-data-model", "content-data-model":
		return mapTypeDataModel
	case "self-host-runtime-deployments", "background-jobs-email-automation", "django-api-persistence-async-workers", "framework-runtime-module-platform", "files-assets-storage":
		return mapTypeOps
	case "identity-auth-workspace-tenancy", "identity-auth-access-control", "workspace-identity-access-billing", "multi-tenant-workspace-platform":
		return mapTypePlatform
	case "third-party-integrations", "bank-connectivity-plaid-sync", "provider-adapters-pluggable-infrastructure", "payments-tax-monetary-configuration":
		return mapTypeExternal
	case "apps-developer-extension-platform", "extension-surfaces", "pip-compatible-interface", "tools-ephemeral-environments", "build-publish-auth-audit", "run-scripts-inline-dependencies":
		return mapTypeTooling
	case "ai-agents-chat-skills":
		return mapTypePlatform
	case "accounts-net-worth-dashboard", "transaction-ledger-categorization-cashflow", "budgeting", "investments-holdings-securities", "crm-record-experience", "product-catalog-pricing-inventory", "store-configuration-sales-channels":
		return mapTypeDomainFeature
	case "pages-stickies-collaborative-editing":
		return mapTypeUI
	case "project-workspace-lifecycle", "dependency-resolution-lockfile", "package-install-virtual-environments", "registry-cache-artifact-fetching", "managed-python-interpreters", "csv-manual-data-import", "billing-subscriptions-self-host", "cart-checkout-promotions", "orders-fulfillment-post-purchase", "work-items-project-delivery", "workflows-automation", "flows-automation", "document-signing-authoring", "affiliate-partner-programs", "click-analytics-conversion-attribution", "short-link-redirect-click-capture", "planning-cycles-modules-views", "intake-publishing-public-space", "analytics-export-reporting", "partner-portal", "custom-domains-link-infrastructure":
		return mapTypeBusinessFlow
	default:
		return ""
	}
}

func mapAreaTypeValues(area *mapAreaInternal, label string, covers []string) []string {
	var values []string
	values = append(values, label, area.Key)
	values = append(values, area.RawAnchors...)
	values = append(values, covers...)
	for _, art := range area.Artifacts {
		values = append(values, art.Path, art.Title, art.Subtype)
	}
	return values
}

func mapAreaPrimaryTypeValues(area *mapAreaInternal, label string, covers []string) []string {
	values := []string{label, area.Key}
	values = append(values, covers...)
	return values
}

func mapAnyContains(values []string, needles ...string) bool {
	for _, value := range values {
		normalized := normalizeMapKey(value)
		if normalized == "" {
			continue
		}
		for _, needle := range needles {
			needle = normalizeMapKey(needle)
			if needle == "" {
				continue
			}
			if normalized == needle ||
				strings.HasPrefix(normalized, needle+"-") ||
				strings.HasSuffix(normalized, "-"+needle) ||
				strings.Contains(normalized, "-"+needle+"-") ||
				strings.Contains(normalized, "/"+needle+"/") ||
				strings.HasPrefix(normalized, needle+"/") ||
				strings.HasSuffix(normalized, "/"+needle) {
				return true
			}
		}
	}
	return false
}

func displayMapAreaType(areaType string) string {
	switch areaType {
	case mapTypeDomainFeature:
		return "domain feature"
	case mapTypeBusinessFlow:
		return "business workflow"
	case mapTypeExternal:
		return "external integration"
	case mapTypeAPI:
		return "API surface"
	case mapTypeUI:
		return "UI surface"
	case mapTypeDataModel:
		return "data model"
	case mapTypeDataPipeline:
		return "data pipeline"
	case mapTypePlatform:
		return "platform capability"
	case mapTypeOps:
		return "ops/runtime"
	case mapTypeTooling:
		return "tooling/script"
	case mapTypeTestQuality:
		return "test/quality"
	case mapTypeProtocol:
		return "protocol/process"
	case mapTypeDocs:
		return "docs/reference"
	case mapTypeRoot:
		return "repo-root umbrella"
	default:
		return "unknown"
	}
}

func mapAreaConfidence(area *mapAreaInternal, class string) string {
	if (isMapGenericAnchor(area.Key) && !mapBoundaryAllowedGenericAreaLabels[area.Key]) || mapGenericAreaLabels[area.Key] {
		return mapLowConfidence
	}
	weakStandalone := mapWeakStandaloneAreaLabels[area.Key] && len(area.Subareas) < 2
	source := area.EvidenceCounts["source"] > 0 || area.EvidenceCounts["test"] > 0
	families := 0
	for _, key := range []string{"source", "test", "intent", "doc", "protocol"} {
		if area.EvidenceCounts[key] > 0 {
			families++
		}
	}
	avg := 0.0
	if len(area.RawAnchors) > 0 {
		avg = area.ConfidenceSum / float64(len(area.RawAnchors))
	}
	if weakStandalone && source {
		return mapMediumConfidence
	}
	if weakStandalone {
		return mapLowConfidence
	}
	if source && families >= 2 && avg >= 0.74 {
		return mapHighConfidence
	}
	if source && len(area.ArtifactPathSet) >= 2 {
		return mapMediumConfidence
	}
	if families >= 2 && avg >= 0.62 {
		return mapMediumConfidence
	}
	if class == mapClassProtocol || class == mapClassLowConfidence {
		return mapLowConfidence
	}
	return mapLowConfidence
}

func mapAreaIsRepoRoot(area *mapAreaInternal, repoName string) bool {
	repoKey := normalizeMapKey(repoName)
	if repoKey == "" {
		return false
	}
	if area.Key == repoKey {
		return true
	}
	parts := strings.Split(repoKey, "-")
	if len(parts) > 1 && area.Key == parts[0] {
		return true
	}
	return false
}

func reframeMapRootLabel(area *mapAreaInternal, repoName, confidence string) string {
	base := displayMapLabel(repoName)
	if base == "" {
		base = area.Label
	}
	if confidence == mapLowConfidence {
		return base + " package"
	}
	if area.EvidenceCounts["source"] > 0 || area.EvidenceCounts["test"] > 0 {
		return base + " behavior"
	}
	return base + " overview"
}

func refineMapAreaLabel(label string, covers []string) string {
	key := normalizeMapKey(label)
	switch {
	case strings.HasPrefix(key, "lib-") && len(wordsFromMap(label)) > 1:
		return displayMapLabel(strings.TrimPrefix(key, "lib-"))
	case key == "application":
		if lead := firstMapCoverLead(covers); lead != "" {
			return displayMapLabel(lead + " application")
		}
	case key == "cmd":
		return "Commands"
	}
	return label
}

func firstMapCoverLead(covers []string) string {
	for _, cover := range covers {
		for _, word := range wordsFromMap(cover) {
			if len(word) >= 4 && !mapGenericTerms[word] {
				return word
			}
		}
	}
	return ""
}

type mapPackabilityIndex struct {
	paths map[string]string
	dirs  map[string]int
	words map[string]int
}

func loadMapPackabilityIndex(repoRoot string) (*mapPackabilityIndex, error) {
	db, err := openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	meta := db.GetRepoByRoot(repoRoot)
	if meta == nil {
		return nil, nil
	}
	rows, err := db.Query("SELECT COALESCE(path,''), COALESCE(source_type,'') FROM sources WHERE repo_id = ?", meta.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	index := &mapPackabilityIndex{
		paths: map[string]string{},
		dirs:  map[string]int{},
		words: map[string]int{},
	}
	for rows.Next() {
		var pathValue, sourceType string
		if err := rows.Scan(&pathValue, &sourceType); err != nil {
			return nil, err
		}
		index.add(pathValue, sourceType)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return index, nil
}

func (idx *mapPackabilityIndex) add(pathValue, sourceType string) {
	pathValue = normalizeMapPath(pathValue)
	if pathValue == "" {
		return
	}
	idx.paths[pathValue] = sourceType
	dir := mapPackabilityDir(pathValue)
	for dir != "" && dir != "." {
		idx.dirs[dir]++
		next := mapPackabilityDir(dir)
		if next == dir || next == "." {
			break
		}
		dir = next
	}
	for _, word := range wordsFromMap(pathValue) {
		if mapTrySpecificWord(word) {
			idx.words[word]++
		}
	}
}

func mapPackabilityIndexFromFiles(files []string) *mapPackabilityIndex {
	if len(files) == 0 {
		return nil
	}
	index := &mapPackabilityIndex{
		paths: map[string]string{},
		dirs:  map[string]int{},
		words: map[string]int{},
	}
	for _, pathValue := range files {
		index.add(pathValue, mapBoundaryPathFamily(pathValue))
	}
	if len(index.paths) == 0 {
		return nil
	}
	return index
}

func mapPackabilityDir(pathValue string) string {
	pathValue = normalizeMapPath(pathValue)
	if pathValue == "" {
		return ""
	}
	dir := pathpkg.Dir(pathValue)
	if dir == "." {
		return ""
	}
	return dir
}

func (idx *mapPackabilityIndex) diagnostics(keyPaths []string) *mapPackabilityDiagnostics {
	if idx == nil {
		return nil
	}
	diag := &mapPackabilityDiagnostics{KeyPathCount: len(keyPaths)}
	for _, raw := range keyPaths {
		pathValue := normalizeMapPath(raw)
		if pathValue == "" {
			continue
		}
		if _, ok := idx.paths[pathValue]; ok {
			diag.IndexedKeyPathCount++
			continue
		}
		if idx.prefixSupported(pathValue) {
			diag.PrefixKeyPathCount++
			continue
		}
		if ext := strings.ToLower(filepath.Ext(pathValue)); ext != "" {
			diag.MissingKeyExtensions = appendUniqueString(diag.MissingKeyExtensions, ext)
		}
	}
	return diag
}

func (idx *mapPackabilityIndex) prefixSupported(pathValue string) bool {
	if idx == nil {
		return false
	}
	pathValue = normalizeMapPath(pathValue)
	if pathValue == "" {
		return false
	}
	dir := mapPackabilityDir(pathValue)
	for depth := 0; dir != "" && depth < 3; depth++ {
		if idx.dirs[dir] > 0 {
			return true
		}
		next := mapPackabilityDir(dir)
		if next == dir || next == "." {
			break
		}
		dir = next
	}
	return false
}

func (idx *mapPackabilityIndex) queryAnchorSupport(query string) int {
	if idx == nil {
		return 0
	}
	count := 0
	seen := map[string]bool{}
	for _, word := range wordsFromMap(query) {
		if seen[word] || !mapTrySpecificWord(word) {
			continue
		}
		seen[word] = true
		if idx.words[word] > 0 {
			count++
		}
	}
	return count
}

type mapTryCandidate struct {
	Query  string
	Source string
	Score  int
}

func mapTryCommand(label string, covers []string, receipts []mapTraceReceipt, confidence string, keyPaths []string) string {
	return mapTryCommandForRole(label, covers, receipts, confidence, keyPaths, mapBoundaryRoleProductCapability)
}

func mapTryCommandForRole(label string, covers []string, receipts []mapTraceReceipt, confidence string, keyPaths []string, boundaryRole string) string {
	try, _ := mapTryCommandForRoleWithPackability(label, covers, receipts, confidence, keyPaths, boundaryRole, nil)
	return try
}

func mapTryCommandForRoleWithPackability(label string, covers []string, receipts []mapTraceReceipt, confidence string, keyPaths []string, boundaryRole string, packability *mapPackabilityIndex) (string, *mapPackabilityDiagnostics) {
	if boundaryRole == "" {
		boundaryRole = mapBoundaryRoleProductCapability
	}
	if confidence == mapLowConfidence && len(covers) == 0 && len(keyPaths) == 0 {
		return "", nil
	}
	candidates := mapTryCandidates(label, covers, receipts, keyPaths, boundaryRole)
	best := bestMapTryCandidate(candidates, label, covers, keyPaths, boundaryRole)
	if best.Query == "" {
		return "", nil
	}
	if packability == nil {
		return mapFindPackCommand(best.Query), nil
	}
	diag := packability.diagnostics(keyPaths)
	if diag == nil {
		return mapFindPackCommand(best.Query), nil
	}
	diag.IndexedQueryAnchorCount = packability.queryAnchorSupport(best.Query)
	if mapTryCandidatePackable(best, diag, boundaryRole) {
		diag.Decision = "supported"
		diag.SelectedTrySource = best.Source
		return mapFindPackCommand(best.Query), diag
	}
	for _, candidate := range rankedMapTryCandidates(candidates, label, covers, keyPaths, boundaryRole) {
		if candidate.Query == best.Query {
			continue
		}
		candidateDiag := *diag
		candidateDiag.IndexedQueryAnchorCount = packability.queryAnchorSupport(candidate.Query)
		if mapTryCandidatePackable(candidate, &candidateDiag, boundaryRole) {
			candidateDiag.Decision = "replaced_with_supported_try"
			candidateDiag.SelectedTrySource = candidate.Source
			candidateDiag.SuppressedTry = mapFindPackCommand(best.Query)
			candidateDiag.SuppressedTrySource = best.Source
			return mapFindPackCommand(candidate.Query), &candidateDiag
		}
	}
	diag.Decision = "suppressed_no_indexed_support"
	diag.SuppressedTry = mapFindPackCommand(best.Query)
	diag.SuppressedTrySource = best.Source
	diag.TrySuppressed = true
	return "", diag
}

func mapTryCandidates(label string, covers []string, receipts []mapTraceReceipt, keyPaths []string, boundaryRole string) []mapTryCandidate {
	var candidates []mapTryCandidate
	add := func(query, source string) {
		query = mapTryCleanHandoffQuery(query, source)
		query = strings.ToLower(strings.TrimSpace(query))
		if query == "" {
			return
		}
		candidates = append(candidates, mapTryCandidate{Query: query, Source: source})
	}
	conceptualLabel := mapTryLabelLooksConceptual(label)
	if !mapTryRoleNeedsSpecificContext(boundaryRole) && !mapTryLabelNeedsSpecificContext(label) {
		add(label, "label")
	}
	for _, receipt := range firstMapTraceReceipts(receipts, mapTryTraceReceiptCandidateLimit(receipts)) {
		if traceTaskQuery := mapTraceTaskQuery(receipt.Subject); traceTaskQuery != "" {
			add(traceTaskQuery, "trace_task")
		}
	}
	if pathQuery := mapTrySpecificPathQuery(label, keyPaths); pathQuery != "" {
		add(pathQuery, "path")
	}
	for _, cover := range firstStrings(covers, mapMaxCoversPerArea) {
		if firstMapSpecificCover([]string{cover}) == "" {
			continue
		}
		add(cover, "cover")
		if boundaryRole == mapBoundaryRoleProductCapability && !mapTryLabelNeedsSpecificContext(label) && !mapTryLabelLooksConceptual(label) {
			add(joinMapQuery(label, cover), "label_cover")
		}
	}
	if len(receipts) > 0 && !conceptualLabel {
		if traceQuery := mapTraceQuery(label, covers, receipts[0].Subject); traceQuery != "" {
			add(traceQuery, "trace")
		}
	}
	return candidates
}

func mapTryTraceReceiptCandidateLimit(receipts []mapTraceReceipt) int {
	limit := 2
	if len(receipts) <= limit {
		return limit
	}
	for _, receipt := range firstMapTraceReceipts(receipts, limit) {
		if mapTraceTaskQuery(receipt.Subject) == "" {
			continue
		}
		words := wordsFromMap(stripMapCommitPrefixes(receipt.Subject))
		if mapTryTechnicalMigrationQuery(words) {
			return mapMaxTraceReceipts
		}
		for _, word := range words {
			if mapTryScaffoldHandoffWord(word) {
				return mapMaxTraceReceipts
			}
			if mapTryImplementationDetailWord(word) {
				return mapMaxTraceReceipts
			}
		}
	}
	return limit
}

func bestMapTryCandidate(candidates []mapTryCandidate, label string, covers []string, keyPaths []string, boundaryRole string) mapTryCandidate {
	ranked := rankedMapTryCandidates(candidates, label, covers, keyPaths, boundaryRole)
	if len(ranked) == 0 {
		return mapTryCandidate{}
	}
	return ranked[0]
}

func rankedMapTryCandidates(candidates []mapTryCandidate, label string, covers []string, keyPaths []string, boundaryRole string) []mapTryCandidate {
	seen := map[string]bool{}
	var ranked []mapTryCandidate
	for _, candidate := range candidates {
		key := normalizeMapKey(candidate.Query)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		candidate.Score = mapTryCandidateScore(candidate, label, covers, keyPaths, boundaryRole)
		if candidate.Score <= 0 {
			continue
		}
		ranked = append(ranked, candidate)
	}
	threshold := 10
	if mapTryRoleNeedsSpecificContext(boundaryRole) || mapTryLabelNeedsSpecificContext(label) {
		threshold = 16
	}
	filtered := ranked[:0]
	for _, candidate := range ranked {
		if candidate.Score >= threshold {
			filtered = append(filtered, candidate)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Score == filtered[j].Score {
			return len(filtered[i].Query) < len(filtered[j].Query)
		}
		return filtered[i].Score > filtered[j].Score
	})
	return filtered
}

func mapTryCandidatePackable(candidate mapTryCandidate, diag *mapPackabilityDiagnostics, boundaryRole string) bool {
	if diag == nil {
		return true
	}
	if diag.KeyPathCount == 0 {
		return diag.IndexedQueryAnchorCount >= 1
	}
	if boundaryRole == mapBoundaryRoleHandoffUnsafe && candidate.Source == "cover" && diag.IndexedKeyPathCount > 0 {
		return true
	}
	if mapTryRoleNeedsSpecificContext(boundaryRole) && diag.IndexedQueryAnchorCount == 0 {
		return false
	}
	if diag.IndexedKeyPathCount > 0 {
		return true
	}
	if diag.PrefixKeyPathCount >= 2 {
		return true
	}
	if diag.PrefixKeyPathCount > 0 && !mapTryRoleNeedsSpecificContext(boundaryRole) {
		return true
	}
	if mapTryRoleNeedsSpecificContext(boundaryRole) || boundaryRole == mapBoundaryRoleExtensionEcosystem {
		return false
	}
	return diag.IndexedQueryAnchorCount >= 2
}

func mapTryCandidateScore(candidate mapTryCandidate, label string, covers []string, keyPaths []string, boundaryRole string) int {
	if boundaryRole == "" {
		boundaryRole = mapBoundaryRoleProductCapability
	}
	queryKey := normalizeMapKey(candidate.Query)
	labelKey := normalizeMapKey(label)
	if queryKey == "" {
		return -1000
	}
	if queryKey == labelKey && (mapTryRoleNeedsSpecificContext(boundaryRole) || mapTryLabelNeedsSpecificContext(label) || mapBoundaryMostlyBroadKey(labelKey)) {
		return -1000
	}
	words := wordsFromMap(candidate.Query)
	if len(words) == 0 {
		return -1000
	}
	if mapTryBroadLabelUsesSpecificCover(label) && candidate.Source == "label" {
		return -1000
	}
	pathWords := mapTryWordSet(keyPaths)
	coverWords := mapTryWordSet(covers)
	specific, broad, pathSupport, coverSupport, implementationDetail, lowValue, generatedLeaf := 0, 0, 0, 0, 0, 0, 0
	for _, word := range words {
		if mapTrySpecificWord(word) {
			specific++
		}
		if mapGenericTerms[word] || mapBoundaryHandoffBroadTerms[word] || mapTraceStopWord(word) {
			broad++
		}
		if mapTryImplementationDetailWord(word) {
			implementationDetail++
		}
		if mapTryLowValueHandoffWord(word) {
			lowValue++
		}
		if mapTryGeneratedLeafWord(word) {
			generatedLeaf++
		}
		if pathWords[word] {
			pathSupport++
		}
		if coverWords[word] {
			coverSupport++
		}
	}
	if specific == 0 {
		return -1000
	}
	if mapTryRoleNeedsSpecificContext(boundaryRole) && pathSupport == 0 && coverSupport == 0 {
		return -1000
	}
	if boundaryRole == mapBoundaryRoleExtensionEcosystem && specific < 2 && pathSupport < 2 && coverSupport < 2 {
		return -1000
	}
	if candidate.Source == "path" && mapTryPathCandidateTooLeafy(words, lowValue, generatedLeaf) {
		return -1000
	}
	if candidate.Source == "trace_task" && firstMapSpecificCover(covers) != "" && coverSupport == 0 {
		return -1000
	}
	score := specific*12 + pathSupport*8 + coverSupport*6 - broad*5
	score -= lowValue * 10
	score -= generatedLeaf * 28
	if candidate.Source == "trace_task" && mapTryRoleNeedsSpecificContext(boundaryRole) {
		score -= implementationDetail * 24
		if mapTryTechnicalMigrationQuery(words) {
			score -= 36
		}
	}
	switch candidate.Source {
	case "path":
		score -= 4
		if mapTryBroadLabelUsesSpecificCover(label) && firstMapSpecificCover(covers) != "" {
			score -= 50
		}
	case "cover":
		score += 6
		if mapTryBroadLabelUsesSpecificCover(label) {
			score += 60
		}
		if boundaryRole == mapBoundaryRoleExtensionEcosystem {
			score += 18
		}
	case "trace":
		score += 4
	case "trace_task":
		score += 30
	case "label_cover":
		score += 22
		if mapTryBroadLabelUsesSpecificCover(label) {
			score += 20
		}
	case "label":
		score += 26
		if firstMapSpecificCover(covers) != "" && !mapTryLabelLooksConceptual(label) {
			score -= 8
		}
		if boundaryRole == mapBoundaryRoleProductCapability && mapTryLabelLooksConceptual(label) {
			score += 20
		}
	}
	switch boundaryRole {
	case mapBoundaryRoleProductCapability:
		score += 4
	case mapBoundaryRoleExtensionEcosystem:
		score += 2
	case mapBoundaryRoleGenericParent, mapBoundaryRoleHorizontalLayer, mapBoundaryRoleRepoNamespace, mapBoundaryRoleFixtureOrTestbed, mapBoundaryRoleHandoffUnsafe:
		if candidate.Source == "label" {
			score -= 30
		}
		if broad >= specific {
			score -= 10
		}
	}
	if len(words) > 6 {
		score -= len(words) - 6
	}
	return score
}

func mapTryRoleNeedsSpecificContext(role string) bool {
	switch role {
	case mapBoundaryRoleGenericParent, mapBoundaryRoleHorizontalLayer, mapBoundaryRoleRepoNamespace, mapBoundaryRoleFixtureOrTestbed, mapBoundaryRoleHandoffUnsafe:
		return true
	default:
		return false
	}
}

func mapTryWordSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		for _, word := range wordsFromMap(value) {
			out[word] = true
		}
	}
	return out
}

func mapTryCleanHandoffQuery(query, source string) string {
	if source == "label" {
		return query
	}
	words := wordsFromMap(query)
	for len(words) > 0 && mapTryTerminalShellWord(words[0]) {
		words = words[1:]
	}
	for len(words) > 0 && mapTryTerminalShellWord(words[len(words)-1]) {
		words = words[:len(words)-1]
	}
	if len(words) == 0 {
		return query
	}
	return displayMapLabel(strings.Join(words, " "))
}

func mapTryTerminalShellWord(word string) bool {
	switch word {
	case "component", "components", "controller", "controllers", "dashboard", "dashboards",
		"initializer", "initializers", "page", "pages", "route", "routes", "screen", "screens",
		"view", "views":
		return true
	default:
		return false
	}
}

func mapTrySpecificWord(word string) bool {
	if word == "" || mapGenericTerms[word] || mapBoundaryHandoffBroadTerms[word] || mapTraceStopWord(word) || mapTryLowValueHandoffWord(word) || mapTryGeneratedLeafWord(word) {
		return false
	}
	if len(word) >= 4 {
		return true
	}
	switch word {
	case "og", "seo", "mcp", "tls", "jwt", "sso":
		return true
	default:
		return false
	}
}

func mapTryScaffoldHandoffWord(word string) bool {
	switch word {
	case "skeleton":
		return true
	default:
		return false
	}
}

func mapTryImplementationDetailWord(word string) bool {
	switch word {
	case "cache", "cached", "caching", "column", "columns", "disable", "disabled",
		"execute", "execution", "skip", "skips", "validation":
		return true
	default:
		return false
	}
}

func mapTryTechnicalMigrationQuery(words []string) bool {
	seen := mapStringSet(words)
	if !seen["migrate"] && !seen["migration"] && !seen["migrating"] {
		return false
	}
	return seen["alchemy"] || seen["connector"] || seen["sql"] || seen["sqlalchemy"]
}

func mapTryLowValueHandoffWord(word string) bool {
	switch word {
	case "addlicense", "cname", "compat", "config", "configs", "configuration", "csproj",
		"dockerfile", "dockerignore", "eslint", "fixture", "fixtures", "impl", "implementation",
		"license", "manifest", "postcss", "prettier", "skeleton", "template", "templates", "version":
		return true
	default:
		return false
	}
}

func mapTryGeneratedLeafWord(word string) bool {
	if word == "" {
		return false
	}
	if mapTryKnownShortCodeWord(word) {
		return false
	}
	if mapTryGeneratedNumberSuffixRegex.MatchString(word) {
		return true
	}
	if mapTryGeneratedNumericLeadRegex.MatchString(word) {
		return true
	}
	if mapTryGeneratedShortCodeRegex.MatchString(word) {
		return true
	}
	if mapTryGeneratedExtraRegex.MatchString(word) {
		return true
	}
	return false
}

func mapTryKnownShortCodeWord(word string) bool {
	switch word {
	case "a11y", "amd64", "arm64", "i18n", "r2", "s3", "x64", "x86":
		return true
	default:
		return false
	}
}

func mapTryPathCandidateTooLeafy(words []string, lowValue, generatedLeaf int) bool {
	if len(words) == 0 {
		return true
	}
	if generatedLeaf > 0 {
		return true
	}
	if lowValue >= 2 {
		return true
	}
	if lowValue > 0 && len(words) <= 2 {
		return true
	}
	specific := 0
	for _, word := range words {
		if mapTrySpecificWord(word) {
			specific++
		}
	}
	return specific == 0
}

func mapTryLabelNeedsSpecificContext(label string) bool {
	key := normalizeMapKey(label)
	if mapBoundaryFirstScreenShellLabels[key] {
		return true
	}
	if mapTryLabelLooksConceptual(label) {
		return true
	}
	if mapTryConceptualLabelNeedsSpecificContext(key) {
		return true
	}
	return mapBoundaryHandoffUnsafeKey(key)
}

func mapTryConceptualLabelNeedsSpecificContext(key string) bool {
	switch key {
	case "background-jobs-email-automation",
		"build-publish-auth-audit",
		"dependency-resolution-lockfile",
		"extension-surfaces",
		"files-assets-storage",
		"flows-automation",
		"package-installation-virtual-environments",
		"registry-cache-artifact-fetching",
		"run-scripts-inline-dependencies",
		"work-items-project-delivery":
		return true
	default:
		return false
	}
}

func mapTryBroadLabelUsesSpecificCover(label string) bool {
	switch normalizeMapKey(label) {
	case "files-assets-storage", "background-jobs-email-automation":
		return true
	default:
		return false
	}
}

func firstMapSpecificCover(covers []string) string {
	for _, cover := range covers {
		key := normalizeMapKey(cover)
		if key == "" || mapBoundaryFirstScreenShellLabels[key] {
			continue
		}
		words := wordsFromMap(cover)
		specific := 0
		for _, word := range words {
			if mapTrySpecificWord(word) {
				specific++
			}
		}
		if specific > 0 {
			return cover
		}
	}
	return ""
}

func mapTrySpecificPathQuery(label string, keyPaths []string) string {
	labelWords := mapStringSet(wordsFromMap(label))
	var words []string
	for _, path := range firstStrings(keyPaths, 5) {
		for _, word := range wordsFromMap(path) {
			if labelWords[word] || mapGenericTerms[word] || mapBoundaryHandoffBroadTerms[word] || mapTraceStopWord(word) || mapTryLowValueHandoffWord(word) || mapTryGeneratedLeafWord(word) {
				continue
			}
			if len(word) < 4 {
				continue
			}
			words = appendUniqueString(words, word)
			if len(words) >= 3 {
				return displayMapLabel(strings.Join(words, " "))
			}
		}
	}
	if len(words) == 0 {
		return ""
	}
	words = append(mapTryPathQueryLabelPrefix(label), words...)
	return displayMapLabel(strings.Join(firstStrings(words, 4), " "))
}

func mapTryPathQueryLabelPrefix(label string) []string {
	if mapTryBroadLabelUsesSpecificCover(label) || mapTryConceptualLabelNeedsSpecificContext(normalizeMapKey(label)) {
		return nil
	}
	var out []string
	for _, word := range wordsFromMap(label) {
		if len(word) < 4 || mapGenericTerms[word] || mapBoundaryHandoffBroadTerms[word] || mapTraceStopWord(word) || mapTryLowValueHandoffWord(word) || mapTryGeneratedLeafWord(word) {
			continue
		}
		out = appendUniqueString(out, word)
		if len(out) >= 2 {
			break
		}
	}
	return out
}

func mapTryLabelLooksConceptual(label string) bool {
	words := wordsFromMap(label)
	if len(words) >= 3 {
		return true
	}
	key := normalizeMapKey(label)
	if strings.Contains(label, "/") || strings.Contains(label, "&") || strings.Contains(label, ",") {
		return true
	}
	return mapAnyContains([]string{key},
		"platform", "experience", "automation", "identity", "access", "data-model",
		"developer", "api-layer", "programs", "runtime", "deployments", "collaborative",
		"attribution", "infrastructure", "tenancy", "licensing")
}

func mapFindPackCommand(query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return ""
	}
	return fmt.Sprintf("ds find %q", query)
}

func mapTraceQuery(label string, covers []string, subject string) string {
	labelWords := mapStringSet(wordsFromMap(label))
	supportWords := map[string]bool{}
	for _, cover := range covers {
		for _, word := range wordsFromMap(cover) {
			supportWords[word] = true
		}
	}
	var extra []string
	for _, word := range wordsFromMap(stripMapCommitPrefix(subject)) {
		if mapTraceStopWord(word) || labelWords[word] {
			continue
		}
		if supportWords[word] {
			extra = appendUniqueString(extra, word)
		}
		if len(extra) >= 3 {
			break
		}
	}
	if len(extra) == 0 {
		return ""
	}
	return displayMapLabel(strings.Join(append(wordsFromMap(label), extra...), " "))
}

func mapTraceTaskQuery(subject string) string {
	if mapTraceTaskRejectedPrefix(subject) {
		return ""
	}
	subject = stripMapCommitPrefixes(subject)
	if !mapTraceTaskSubjectAllowed(subject) {
		return ""
	}
	var words []string
	for _, word := range wordsFromMap(subject) {
		if mapTraceTaskSkipWord(word) {
			continue
		}
		words = appendUniqueString(words, word)
		if len(words) >= 5 {
			break
		}
	}
	if len(words) < 2 {
		return ""
	}
	return displayMapLabel(strings.Join(words, " "))
}

func mapTraceTaskRejectedPrefix(subject string) bool {
	subject = strings.TrimSpace(strings.ToLower(subject))
	if idx := strings.Index(subject, ":"); idx > 0 && idx <= 16 {
		switch strings.TrimSpace(subject[:idx]) {
		case "build", "chore", "ci", "doc", "docs", "style", "test", "tests":
			return true
		}
	}
	return false
}

func stripMapCommitPrefixes(subject string) string {
	subject = strings.TrimSpace(subject)
	for i := 0; i < 3; i++ {
		next := stripMapCommitPrefix(subject)
		if next == subject {
			return strings.TrimSpace(subject)
		}
		subject = strings.TrimSpace(next)
	}
	return subject
}

func mapTraceTaskSubjectAllowed(subject string) bool {
	lower := strings.ToLower(strings.TrimSpace(subject))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "dependabot") || strings.Contains(lower, "renovate") {
		return false
	}
	words := wordsFromMap(lower)
	for len(words) > 0 && mapTraceTaskConventionalPrefixWord(words[0]) {
		words = words[1:]
	}
	if len(words) == 0 {
		return false
	}
	leading := words[0]
	if mapTraceTaskUnsupportedLeadingWord(leading) {
		return false
	}
	if mapTraceTaskAllowedLeadingWord(leading) {
		return true
	}
	return mapTrySpecificWord(leading)
}

func mapTraceTaskConventionalPrefixWord(word string) bool {
	switch word {
	case "ci", "chore", "doc", "docs", "feat", "feature", "perf", "style", "test", "tests", "wip":
		return true
	default:
		return false
	}
}

func mapTraceTaskAllowedLeadingWord(word string) bool {
	switch word {
	case "add", "adds", "added", "allow", "allows", "enable", "enables", "fix", "fixes",
		"gate", "gates", "implement", "implements", "improve", "improves", "repair",
		"repairs", "support", "supports":
		return true
	default:
		return false
	}
}

func mapTraceTaskUnsupportedLeadingWord(word string) bool {
	switch word {
	case "bump", "bumps", "clean", "cleanup", "merge", "merged", "move", "moves",
		"record", "records", "release", "remove", "removes", "rename", "renames",
		"replace", "replaces", "revert", "reverts", "update", "updates", "upgrade", "upgrades":
		return true
	default:
		return false
	}
}

func mapTraceTaskSkipWord(word string) bool {
	if word == "" || mapTraceTaskConventionalPrefixWord(word) || mapTraceStopWord(word) || mapTryLowValueHandoffWord(word) || mapTryGeneratedLeafWord(word) {
		return true
	}
	switch word {
	case "auto", "gate", "gates", "gated", "implement", "implements", "implemented",
		"in", "repair", "repairs", "repaired", "speed", "speeds", "speeding":
		return true
	default:
		return false
	}
}

func joinMapQuery(label, cover string) string {
	labelWords := mapStringSet(wordsFromMap(label))
	words := wordsFromMap(label)
	for _, word := range wordsFromMap(cover) {
		if !labelWords[word] && !mapGenericTerms[word] {
			words = append(words, word)
		}
	}
	return displayMapLabel(strings.Join(words, " "))
}

func mapTraceReceipts(repoRoot string, area *mapAreaInternal, covers []string) []mapTraceReceipt {
	paths := mapArtifactPaths(area.Artifacts)
	if len(paths) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), findGitReceiptTimeout)
	defer cancel()
	if !findGitRepoAvailable(ctx, repoRoot) {
		return nil
	}
	commits, ok := findGitLogForPaths(ctx, repoRoot, firstStrings(paths, findGitReceiptMaxPaths))
	if !ok || len(commits) == 0 {
		return nil
	}
	anchors := mapTraceAnchors(area, covers)
	scored := make([]FindGitReceipt, 0, len(commits))
	for _, commit := range commits {
		if mapCommitNoisy(commit) {
			continue
		}
		subjectLower := strings.ToLower(commit.subject)
		matched := 0
		for _, anchor := range anchors {
			if anchor != "" && strings.Contains(subjectLower, anchor) {
				matched++
			}
		}
		score := len(commit.paths) + matched*5
		if matched == 0 && len(scored) > 0 {
			continue
		}
		scored = append(scored, FindGitReceipt{
			SHA:      commit.sha,
			ShortSHA: shortFindGitSHA(commit.sha),
			Subject:  limitRunes(commit.subject, 120),
			Score:    score,
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].Subject < scored[j].Subject
		}
		return scored[i].Score > scored[j].Score
	})
	var out []mapTraceReceipt
	for _, receipt := range firstFindGitReceipts(scored, 3) {
		out = append(out, mapTraceReceipt{SHA: receipt.ShortSHA, Subject: receipt.Subject})
	}
	return out
}

func mapTraceAnchors(area *mapAreaInternal, covers []string) []string {
	seen := map[string]bool{}
	var out []string
	values := append([]string{area.Label}, covers...)
	values = append(values, area.RawAnchors...)
	for _, value := range values {
		for _, word := range wordsFromMap(value) {
			if len(word) < 4 || mapTraceStopWord(word) || seen[word] {
				continue
			}
			seen[word] = true
			out = append(out, word)
		}
	}
	return out
}

func mapCommitNoisy(commit parsedFindGitCommit) bool {
	subject := strings.ToLower(commit.subject)
	switch {
	case strings.HasPrefix(subject, "merge "):
		return true
	case strings.HasPrefix(subject, "revert "):
		return true
	case strings.HasPrefix(subject, "release"):
		return true
	case strings.Contains(subject, "dependabot") || strings.Contains(subject, "renovate"):
		return true
	case strings.Contains(subject, "lockfile") || strings.Contains(subject, "dependencies"):
		return true
	case strings.Contains(subject, "translation") || strings.Contains(subject, "translations"):
		return true
	case strings.Contains(subject, "pre-commit") || strings.Contains(subject, "typo") || strings.Contains(subject, "typos"):
		return true
	case strings.Contains(subject, "update docs") || strings.Contains(subject, "docs references"):
		return true
	}
	return len(commit.paths) > 80
}

func mapAreaCaveats(area *mapAreaInternal, class string, root bool) []string {
	var caveats []string
	if root {
		caveats = append(caveats, "repo-root umbrella; trust the child topics more than the parent label")
	}
	if class == mapClassProtocol {
		caveats = append(caveats, "protocol/process area, not core implementation")
	}
	if area.EvidenceCounts["source"] == 0 && area.EvidenceCounts["test"] == 0 {
		caveats = append(caveats, "no source/test evidence in displayed examples")
	}
	for _, c := range area.Caveats {
		caveats = appendUniqueString(caveats, c)
	}
	return caveats
}

func mapLabelEvidence(area *mapAreaInternal) []string {
	var out []string
	if area.LabelSource != "" {
		out = append(out, area.LabelSource)
	}
	for source := range area.EvidenceSources {
		out = appendUniqueString(out, source)
	}
	sort.Strings(out)
	return out
}

func mapTraceTerms(receipts []mapTraceReceipt) []string {
	var out []string
	for _, receipt := range receipts {
		for _, word := range wordsFromMap(stripMapCommitPrefix(receipt.Subject)) {
			if len(word) >= 4 && !mapTraceStopWord(word) {
				out = appendUniqueString(out, word)
			}
		}
	}
	return out
}

func cleanMapCovers(label string, covers []string) []string {
	labelWords := mapStringSet(wordsFromMap(label))
	seen := map[string]bool{}
	var out []string
	for _, cover := range covers {
		key := normalizeMapKey(cover)
		if key == "" || seen[key] {
			continue
		}
		if len(key) <= 2 {
			continue
		}
		words := wordsFromMap(cover)
		if len(words) == 0 {
			continue
		}
		allInLabel := true
		for _, word := range words {
			if !labelWords[word] {
				allInLabel = false
				break
			}
		}
		if allInLabel {
			continue
		}
		if len(words) == 1 && mapGenericTerms[words[0]] && !mapBoundaryAllowedGenericAreaLabels[words[0]] {
			continue
		}
		seen[key] = true
		out = append(out, displayMapLabel(cover))
	}
	return out
}

func mapPathLabelCandidates(filePath string) []mapLabelCandidate {
	parts := strings.Split(normalizeMapPath(filePath), "/")
	if len(parts) == 0 {
		return nil
	}
	dirParts := parts[:len(parts)-1]
	var out []mapLabelCandidate
	for i, segment := range dirParts {
		labelSegment := cleanMapPathSegmentForLabel(segment)
		key := normalizeMapKey(labelSegment)
		if key == "" || isMapGenericAnchor(key) {
			continue
		}
		out = append(out, mapLabelCandidate{Key: key, Label: displayMapLabel(labelSegment), Score: 6 + float64(mapMinInt(i, 4))*0.25})
	}
	for i := 0; i < len(dirParts)-1; i++ {
		aLabel := cleanMapPathSegmentForLabel(dirParts[i])
		bLabel := cleanMapPathSegmentForLabel(dirParts[i+1])
		a := normalizeMapKey(aLabel)
		b := normalizeMapKey(bLabel)
		if a == "" || b == "" || isMapVersionToken(b) || isMapGenericAnchor(b) {
			continue
		}
		if isMapGenericAnchor(a) && a != "cmd" && a != "packages" && a != "playground" && a != "docs" {
			continue
		}
		out = append(out, mapLabelCandidate{Key: a + "/" + b, Label: displayMapLabel(aLabel) + "/" + displayMapLabel(bLabel), Score: 5.5})
	}
	return out
}

func cleanMapPathSegmentForLabel(segment string) string {
	segment = strings.TrimSpace(segment)
	lower := strings.ToLower(segment)
	switch lower {
	case "docs_src", "doc_src":
		return "docs"
	}
	for _, prefix := range []string{"test_", "tests_", "test-", "tests-"} {
		if strings.HasPrefix(lower, prefix) && len(segment) > len(prefix) {
			return segment[len(prefix):]
		}
	}
	return segment
}

func mapFileStemCandidates(filePath string) []mapLabelCandidate {
	base := filepath.Base(filePath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	stem = strings.TrimSuffix(strings.TrimSuffix(stem, ".test"), ".spec")
	var out []mapLabelCandidate
	for _, ng := range mapMeaningfulNgrams(stem, 2, 3) {
		out = append(out, mapLabelCandidate{Key: ng.Key, Label: ng.Label, Score: 3.6})
	}
	return out
}

func mapAnchorLabelCandidates(anchor string) []mapLabelCandidate {
	var out []mapLabelCandidate
	for _, ng := range mapMeaningfulNgrams(anchor, 1, 3) {
		score := 2.0
		if ng.Key == normalizeMapKey(anchor) {
			score = 2.5
		}
		out = append(out, mapLabelCandidate{Key: ng.Key, Label: ng.Label, Score: score})
	}
	return out
}

func addMapLabelCandidate(candidates map[string]*mapLabelCandidate, key, label string, score float64, artifactPath, source string) {
	key = normalizeMapKey(key)
	if key == "" {
		return
	}
	cand := candidates[key]
	if cand == nil {
		cand = &mapLabelCandidate{Key: key, Label: firstNonEmpty(label, displayMapLabel(key)), Sources: map[string]bool{}, ArtifactPaths: map[string]bool{}}
		candidates[key] = cand
	}
	cand.Score += score
	if source != "" {
		cand.Sources[source] = true
	}
	if artifactPath != "" {
		cand.ArtifactPaths[artifactPath] = true
	}
}

func chooseMapLabel(candidates map[string]*mapLabelCandidate, anchor, repoName string) *mapLabelCandidate {
	anchorKey := normalizeMapKey(anchor)
	repoKey := normalizeMapKey(repoName)
	var best *mapLabelCandidate
	var bestNonRoot *mapLabelCandidate
	for _, cand := range candidates {
		score := cand.Score
		if isMapGenericAnchor(cand.Key) || mapGenericAreaLabels[cand.Key] {
			score -= 10
		}
		if len(cand.ArtifactPaths) >= 2 {
			score += 2.5
		}
		if len(cand.ArtifactPaths) >= 4 {
			score += 1.5
		}
		if cand.Sources["path_boundary"] {
			score += 2.5
		}
		if cand.Sources["anchor"] && cand.Key == anchorKey {
			score -= 1.2
		}
		if mapKeyMatchesRepoRoot(cand.Key, repoKey) {
			score -= 5.5
		}
		if strings.Contains(cand.Key, "/") {
			score -= 0.4
		}
		if len(cand.Key) <= 2 {
			score -= 2
		}
		if best == nil || score > best.Score || (score == best.Score && len(cand.Label) < len(best.Label)) {
			cp := *cand
			cp.Score = score
			best = &cp
		}
		if !mapKeyMatchesRepoRoot(cand.Key, repoKey) && (bestNonRoot == nil || score > bestNonRoot.Score || (score == bestNonRoot.Score && len(cand.Label) < len(bestNonRoot.Label))) {
			cp := *cand
			cp.Score = score
			bestNonRoot = &cp
		}
	}
	if best != nil && mapKeyMatchesRepoRoot(best.Key, repoKey) && bestNonRoot != nil && bestNonRoot.Score >= 1.5 {
		return bestNonRoot
	}
	if best == nil || best.Score < 3 {
		return nil
	}
	return best
}

func mapMeaningfulNgrams(value string, minN, maxN int) []mapLabelCandidate {
	words := wordsFromMap(value)
	filtered := words[:0]
	for _, word := range words {
		if !mapGenericTerms[word] {
			filtered = append(filtered, word)
		}
	}
	words = filtered
	var out []mapLabelCandidate
	for n := mapMinInt(maxN, len(words)); n >= minN; n-- {
		for i := 0; i+n <= len(words); i++ {
			slice := words[i : i+n]
			hasStrong := false
			for _, word := range slice {
				if len(word) >= 4 || (len(word) >= 2 && len(word) <= 5) {
					hasStrong = true
					break
				}
			}
			if !hasStrong {
				continue
			}
			key := strings.Join(slice, "-")
			if key == "" || isMapGenericAnchor(key) {
				continue
			}
			out = append(out, mapLabelCandidate{Key: key, Label: displayMapLabel(key), Score: 1})
		}
	}
	return out
}

func chooseMapSubarea(anchor, parentKey string) string {
	anchorKey := normalizeMapKey(anchor)
	if anchorKey == "" || anchorKey == parentKey || isMapGenericAnchor(anchorKey) {
		return ""
	}
	parentParts := mapStringSet(strings.FieldsFunc(parentKey, func(r rune) bool { return r == '-' || r == '/' }))
	if strings.Contains(anchorKey, parentKey) {
		var parts []string
		for _, part := range strings.Split(anchorKey, "-") {
			if part == "" || parentParts[part] || mapGenericTerms[part] {
				continue
			}
			parts = append(parts, part)
		}
		if len(parts) == 0 {
			return ""
		}
		return displayMapLabel(strings.Join(parts, " "))
	}
	return displayMapLabel(anchorKey)
}

func mapEvidenceCounts(artifacts []scan.WorkstreamArtifactExample) map[string]int {
	counts := map[string]int{}
	for _, art := range artifacts {
		counts[mapArtifactFamilyForPath(art.Kind, art.Subtype, art.Path)]++
	}
	return counts
}

func mapArtifactFamilyForPath(kind, subtype, path string) string {
	normalizedPath := normalizeMapPath(path)
	switch {
	case kind == "source_context" && mapPathLooksTest(normalizedPath):
		return "test"
	case kind == "source_context" && mapPathLooksDocExample(normalizedPath):
		return "doc"
	case kind == "test_case":
		return "test"
	case kind == "source_context":
		return "source"
	case mapProtocolSubtypes[subtype]:
		return "protocol"
	case kind == "markdown_artifact" && (subtype == "plan" || subtype == "proposal" || subtype == "design" || subtype == "adr" || subtype == "decision" || subtype == "requirements"):
		return "intent"
	case kind == "markdown_artifact":
		return "doc"
	case kind == "openspec":
		return "intent"
	default:
		return "other"
	}
}

func mapPathLooksTest(path string) bool {
	if path == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(path))
	return strings.HasPrefix(path, "tests/") ||
		strings.Contains(path, "/tests/") ||
		strings.HasPrefix(path, "test/") ||
		strings.Contains(path, "/test/") ||
		strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, "_test.py") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".test.tsx") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, ".spec.tsx")
}

func mapPathLooksDocExample(path string) bool {
	if path == "" {
		return false
	}
	return strings.HasPrefix(path, "docs_src/") ||
		strings.HasPrefix(path, "docs/") ||
		strings.Contains(path, "/docs_src/") ||
		strings.Contains(path, "/docs/") ||
		strings.HasPrefix(path, "examples/") ||
		strings.Contains(path, "/examples/")
}

func findMapAreaByOverlap(areas []*mapAreaInternal, p *mapPreparedCluster) *mapAreaInternal {
	for _, area := range areas {
		if area.Key == p.ParentKey {
			return area
		}
		if mapSameRootyArea(area.Key, p.ParentKey) {
			continue
		}
		if mapArtifactOverlap(area.ArtifactPathSet, p.ArtifactPathSet) >= 0.78 {
			return area
		}
	}
	return nil
}

func mapArtifactOverlap(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	same := 0
	for k := range a {
		if b[k] {
			same++
		}
	}
	denom := mapMinInt(len(a), len(b))
	if denom == 0 {
		return 0
	}
	return float64(same) / float64(denom)
}

func mapAreaScore(area *mapAreaInternal) float64 {
	classScore := 0.0
	switch classifyMapArea(area) {
	case mapClassStableArea:
		classScore = 6
	case mapClassWorkstream:
		classScore = 4
	case mapClassDocTopic:
		classScore = 2
	case mapClassProtocol:
		classScore = -3
	default:
		classScore = -5
	}
	confScore := 0.0
	switch mapAreaConfidence(area, classifyMapArea(area)) {
	case mapHighConfidence:
		confScore = 12
	case mapMediumConfidence:
		confScore = 7
	default:
		confScore = 2
	}
	sourceScore := float64(area.EvidenceCounts["source"]+area.EvidenceCounts["test"]) * 0.7
	return confScore + classScore + sourceScore + float64(mapMinInt(area.EvidenceCount, 30))*0.15
}

func dedupeMapArtifacts(artifacts []mapArtifact) []mapArtifact {
	seen := map[string]bool{}
	var out []mapArtifact
	for _, art := range artifacts {
		if art.Path == "" {
			continue
		}
		if seen[art.Path] {
			for i := range out {
				if out[i].Path == art.Path && art.Rank > out[i].Rank {
					out[i].Rank = art.Rank
					break
				}
			}
			continue
		}
		seen[art.Path] = true
		out = append(out, art)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rank != out[j].Rank {
			return out[i].Rank > out[j].Rank
		}
		if mapArtifactRank(out[i]) == mapArtifactRank(out[j]) {
			return out[i].Path < out[j].Path
		}
		return mapArtifactRank(out[i]) > mapArtifactRank(out[j])
	})
	return out
}

func mapArtifactRank(art mapArtifact) int {
	switch mapArtifactFamilyForPath(art.Kind, art.Subtype, art.Path) {
	case "source", "test":
		return 4
	case "intent":
		return 3
	case "doc":
		return 2
	case "protocol":
		return 1
	default:
		return 1
	}
}

func mapArtifactPaths(artifacts []mapArtifact) []string {
	var out []string
	for _, art := range artifacts {
		out = appendUniqueString(out, art.Path)
	}
	return out
}

func firstMapTraceReceipts(values []mapTraceReceipt, limit int) []mapTraceReceipt {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	out := make([]mapTraceReceipt, limit)
	copy(out, values[:limit])
	return out
}

func firstFindGitReceipts(values []FindGitReceipt, limit int) []FindGitReceipt {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	out := make([]FindGitReceipt, limit)
	copy(out, values[:limit])
	return out
}

func mapCopyCounts(in map[string]int) map[string]int {
	out := map[string]int{}
	for k, v := range in {
		if v > 0 {
			out[k] = v
		}
	}
	return out
}

func sortedMapSet(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func mapStringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, v := range values {
		v = strings.TrimSpace(strings.ToLower(v))
		if v != "" {
			out[v] = true
		}
	}
	return out
}

func mapPathSuppressed(filePath string) bool {
	for _, part := range strings.Split(normalizeMapPath(filePath), "/") {
		if mapSuppressedPathSegments[strings.ToLower(part)] {
			return true
		}
	}
	return false
}

func normalizeMapPath(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "./")
	value = strings.Trim(value, "/")
	if value == "." {
		return ""
	}
	return value
}

func normalizeMapKey(value string) string {
	words := wordsFromMap(value)
	if len(words) == 0 {
		return ""
	}
	return strings.Join(words, "-")
}

func wordsFromMap(value string) []string {
	var b strings.Builder
	var prev rune
	for _, r := range value {
		if prev != 0 && unicode.IsLower(prev) && unicode.IsUpper(r) {
			b.WriteRune(' ')
		}
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(' ')
		}
		prev = r
	}
	fields := strings.Fields(b.String())
	out := fields[:0]
	for _, field := range fields {
		if !isMapVersionToken(field) {
			out = append(out, field)
		}
	}
	return out
}

func displayMapLabel(value string) string {
	words := wordsFromMap(value)
	for i, word := range words {
		switch word {
		case "api", "cli", "mcp", "gkapi", "adr", "http", "json", "yaml", "sql", "seo", "og", "db", "ui":
			words[i] = strings.ToUpper(word)
		case "httpapi":
			words[i] = "HTTP API"
		default:
			if word == "" {
				continue
			}
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

func titleizeMapAnchor(anchor string) string {
	return displayMapLabel(anchor)
}

func isMapGenericAnchor(anchor string) bool {
	key := normalizeMapKey(anchor)
	if key == "" || mapGenericAreaLabels[key] {
		return true
	}
	parts := strings.Split(key, "-")
	allGeneric := true
	for _, part := range parts {
		if !mapGenericTerms[part] {
			allGeneric = false
			break
		}
	}
	return allGeneric
}

func isMapVersionToken(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	value = strings.TrimPrefix(value, "v")
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func stripMapCommitPrefix(subject string) string {
	subject = strings.TrimSpace(subject)
	if idx := strings.Index(subject, ":"); idx > 0 && idx <= 16 {
		prefix := subject[:idx]
		if !strings.Contains(prefix, " ") {
			return strings.TrimSpace(subject[idx+1:])
		}
	}
	return subject
}

func mapTraceStopWord(word string) bool {
	if mapGenericTerms[word] {
		return true
	}
	switch word {
	case "add", "adds", "added", "adding", "allow", "allows", "change", "changes",
		"changed", "changing", "clean", "cleanup", "fix", "fixes", "fixed", "fixing",
		"handle", "handles", "handled", "handling", "improve", "improves", "improved",
		"make", "makes", "made", "move", "moves", "moved", "refactor", "remove",
		"removes", "removed", "rename", "renamed", "support", "supports", "update",
		"updates", "updated", "use", "uses", "used", "using", "with", "without",
		"from", "into", "for", "and", "or", "the", "this", "that", "when", "before",
		"after", "new", "old", "initial", "minor", "misc", "various":
		return true
	default:
		return false
	}
}

func mapWorkstreamAnchorsSeen(result *scan.Result) int {
	if result == nil || result.WorkstreamEvidence == nil {
		return 0
	}
	return result.WorkstreamEvidence.AnchorsSeen
}

func mapWorkstreamAnchorsMaterialized(result *scan.Result) int {
	if result == nil || result.WorkstreamEvidence == nil {
		return 0
	}
	return result.WorkstreamEvidence.AnchorsMaterialized
}

func mapKeyMatchesRepoRoot(key, repoKey string) bool {
	if key == "" || repoKey == "" {
		return false
	}
	if key == repoKey {
		return true
	}
	parts := strings.Split(repoKey, "-")
	return len(parts) > 1 && key == parts[0]
}

func mapSameRootyArea(a, b string) bool {
	if a == "" || b == "" || a == b {
		return false
	}
	if strings.Contains(a, "/") || strings.Contains(b, "/") {
		return false
	}
	return len(a) <= 5 || len(b) <= 5
}

func firstMapKey(values map[string]bool) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func mapMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
