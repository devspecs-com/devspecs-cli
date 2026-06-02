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

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/adr"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/codecomment"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/openspec"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/sourcecontext"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/testcase"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/freshness"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

const (
	mapDefaultMaxAreas            = 8
	mapMaxArtifactsPerArea        = 4
	mapMaxCoversPerArea           = 5
	mapMaxTraceReceipts           = 3
	mapMaxVerboseTrace            = 4
	mapRecentMaxCommits           = 40
	mapRecentMaxTopics            = 5
	mapBoundaryMaxFiles           = 30000
	mapBoundaryMaxCommits         = 60
	mapBoundaryMaxArtifacts       = 24
	mapBoundaryMaxImportFiles     = 8000
	mapBoundaryMaxImportBytes     = 512 * 1024
	mapBoundaryImportScoreCap     = 40
	mapBoundaryTestImportScoreCap = 16
	mapBoundaryFilesTimeout       = 3 * time.Second
	mapSchemaVersion              = "devspecs.map.v1"
	mapRecentSchemaVersion        = "devspecs.map.recent.v1"
	mapTraceReceiptMode           = "bounded_git_path_receipts_v0"
	mapIndexRequiredCaveat        = "context packing requires an index; run ds scan before using suggested ds find --pack commands"
	mapLowConfidence              = "low"
	mapMediumConfidence           = "medium"
	mapHighConfidence             = "high"
	mapClassStableArea            = "stable_area"
	mapClassWorkstream            = "workstream"
	mapClassDocTopic              = "doc_topic"
	mapClassProtocol              = "protocol"
	mapClassLowConfidence         = "low_confidence"
	mapTypeDomainFeature          = "domain_feature"
	mapTypeBusinessFlow           = "business_workflow"
	mapTypeExternal               = "external_integration"
	mapTypeAPI                    = "api_surface"
	mapTypeUI                     = "ui_surface"
	mapTypeDataModel              = "data_model"
	mapTypeDataPipeline           = "data_pipeline"
	mapTypePlatform               = "platform_capability"
	mapTypeOps                    = "ops_runtime"
	mapTypeTooling                = "tooling_script"
	mapTypeTestQuality            = "test_quality"
	mapTypeProtocol               = "protocol_process"
	mapTypeDocs                   = "docs_reference"
	mapTypeRoot                   = "repo_root_umbrella"
	mapTypeUnknown                = "unknown_area"
)

// NewMapCmd creates the ds map command.
func NewMapCmd() *cobra.Command {
	var (
		path     string
		asJSON   bool
		verbose  bool
		recent   bool
		boundary bool
		maxAreas int
	)

	cmd := &cobra.Command{
		Use:   "map [area]",
		Short: "Show a concise repo map and useful follow-up context commands",
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
				Recent:    recent,
				Boundary:  boundary,
				MaxAreas:  maxAreas,
			})
		},
	}

	cmd.Flags().StringVar(&path, "path", ".", "Repository path to map")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show map diagnostics and extra evidence")
	cmd.Flags().BoolVar(&recent, "recent", false, "Show recently active topics from local git history")
	cmd.Flags().BoolVar(&boundary, "experimental-boundaries", false, "Build the map from path-primary system boundary candidates")
	cmd.Flags().IntVar(&maxAreas, "max-areas", mapDefaultMaxAreas, "Maximum areas to show")
	return cmd
}

type mapOptions struct {
	Path      string
	AreaQuery string
	JSON      bool
	Verbose   bool
	Recent    bool
	Boundary  bool
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
	Key              string   `json:"key,omitempty"`
	RawAnchors       []string `json:"raw_anchors,omitempty"`
	LabelEvidence    []string `json:"label_evidence,omitempty"`
	TraceTerms       []string `json:"trace_terms,omitempty"`
	TraceReceiptMode string   `json:"trace_receipt_mode,omitempty"`
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
	"all": true, "dashboard": true, "dashboards": true, "hook": true, "hooks": true,
	"icon": true, "icons": true, "modal": true, "modals": true, "nucleo": true,
	"store": true, "stores": true, "story": true, "stories": true, "suite": true,
	"suites": true, "view": true, "views": true,
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
)

func runMap(cmd *cobra.Command, opts mapOptions) error {
	start := time.Now()
	success := false
	props := map[string]any{
		"json":       opts.JSON,
		"verbose":    opts.Verbose,
		"recent":     opts.Recent,
		"boundary":   opts.Boundary,
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
	if opts.Boundary {
		out := buildPathBoundaryMapOutput(cmd.Context(), repoRoot, opts)
		success = true
		props["confidence"] = out.Repo.Confidence
		props["area_count_bucket"] = telemetry.CountBucket(len(out.Areas))
		props["path_boundary"] = true
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
	if opts.AreaQuery == "" {
		indexed, indexErr := mapRepoHasIndexedArtifacts(repoRoot)
		if indexErr != nil {
			debugLog("map index availability unavailable: %v", indexErr)
		} else if !indexed {
			out := buildFastMapFallbackOutput(cmd.Context(), repoRoot, opts, false)
			success = true
			props["confidence"] = out.Repo.Confidence
			props["area_count_bucket"] = telemetry.CountBucket(len(out.Areas))
			props["fast_fallback"] = true
			props["index_ready"] = false
			if opts.JSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			writeMapText(cmd.OutOrStdout(), out, opts.Verbose)
			return nil
		}
	}
	var result *scan.Result
	var cachedOutput *mapOutput
	if cached, ok, cacheErr := loadMapOutputCache(repoRoot, opts.MaxAreas); cacheErr != nil {
		debugLog("map output cache unavailable: %v", cacheErr)
	} else if ok {
		cachedOutput = &cached
		props["output_cache_hit"] = true
	}
	if cachedOutput == nil {
		if cached, ok, cacheErr := runCachedMapScan(repoRoot); cacheErr != nil {
			debugLog("map cache unavailable: %v", cacheErr)
		} else if ok {
			result = cached
			props["cache_hit"] = true
		}
	}
	if result == nil && cachedOutput == nil && opts.AreaQuery == "" {
		out := buildFastMapFallbackOutput(cmd.Context(), repoRoot, opts, true)
		success = true
		props["confidence"] = out.Repo.Confidence
		props["area_count_bucket"] = telemetry.CountBucket(len(out.Areas))
		props["fast_fallback"] = true
		if opts.JSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}
		writeMapText(cmd.OutOrStdout(), out, opts.Verbose)
		return nil
	}
	if result == nil && cachedOutput == nil {
		result, err = runMapScan(cmd.Context(), repoRoot)
		if err != nil {
			return err
		}
	}
	var out mapOutput
	if cachedOutput != nil {
		out = *cachedOutput
	} else {
		out = buildMapOutput(repoRoot, result, opts)
		if err := saveMapOutputCache(repoRoot, opts.MaxAreas, out); err != nil {
			debugLog("save map output cache: %v", err)
		}
	}
	success = true
	props["confidence"] = out.Repo.Confidence
	props["area_count_bucket"] = telemetry.CountBucket(len(out.Areas))

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

func runMapScan(ctx context.Context, repoRoot string) (*scan.Result, error) {
	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	cfg = config.WithDefaultIntentCandidateDiscovery(cfg, true)
	cfg = config.WithTestCaseArtifacts(cfg, true)
	cfg = config.WithCodeCommentArtifacts(cfg, true)

	db, err := openDB()
	if err != nil {
		return nil, fmt.Errorf("open map index: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	adpts := []adapters.Adapter{
		&openspec.Adapter{},
		&adr.Adapter{},
		&markdown.Adapter{},
		&sourcecontext.Adapter{},
		&testcase.Adapter{},
		&codecomment.Adapter{},
	}
	scanner := scan.New(db, idgen.NewFactory(), adpts)
	scanOpts, err := liveScanRunOptions(db)
	if err != nil {
		return nil, fmt.Errorf("inspect map index state: %w", err)
	}
	scanOpts.IncludeGitEvidence = true
	scanOpts.IncludeWorkstreamEvidence = true
	scanOpts.RichTypedIndex = true
	result, err := scanner.RunWithOptions(ctx, repoRoot, cfg, scan.RunOptions{
		UseTransaction:            scanOpts.UseTransaction,
		FreshIndex:                scanOpts.FreshIndex,
		SkipAuthoredAtLookup:      scanOpts.SkipAuthoredAtLookup,
		IncludeGitEvidence:        scanOpts.IncludeGitEvidence,
		IncludeWorkstreamEvidence: scanOpts.IncludeWorkstreamEvidence,
		RichTypedIndex:            scanOpts.RichTypedIndex,
	})
	if err != nil {
		return nil, fmt.Errorf("map scan: %w", err)
	}
	if scanTotalFound(result) == 0 {
		matcher, _ := ignore.NewMatcher(repoRoot)
		attachScanHints(result, repoRoot, matcher)
	}
	return result, nil
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
	if meta.LastScanCommit == "" {
		if status := freshness.Check(db, repoRoot); status == nil || status.Stale {
			_ = db.Close()
			if status == nil {
				debugLog("map output cache miss: freshness unavailable for non-git repo")
			} else {
				debugLog("map output cache miss: stale non-git index (%s)", status.Reason)
			}
			return mapOutput{}, false, nil
		}
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

func runCachedMapScan(repoRoot string) (*scan.Result, bool, error) {
	db, err := openDB()
	if err != nil {
		return nil, false, fmt.Errorf("open map cache: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if status := freshness.Check(db, repoRoot); status == nil || status.Stale {
		return nil, false, nil
	}
	result, ok, err := buildCachedMapResult(db, repoRoot)
	if err != nil {
		return nil, false, err
	}
	return result, ok, nil
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
	areas := publicMapAreas(repoRoot, repoName, accum, opts.MaxAreas)
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
	Key             string
	Label           string
	LabelScore      float64
	Score           float64
	FileCount       int
	RecentCount     int
	PathSet         map[string]bool
	BoundaryPaths   map[string]bool
	Subareas        map[string]bool
	EvidenceCounts  map[string]int
	EvidenceSources map[string]bool
	Artifacts       []mapArtifact
	TraceReceipts   []mapTraceReceipt
}

func buildPathBoundaryMapOutput(ctx context.Context, repoRoot string, opts mapOptions) mapOutput {
	repoName := filepath.Base(filepath.Clean(repoRoot))
	files, source, limited, fileErr := listMapBoundaryFiles(ctx, repoRoot)
	recentCommits := mapBoundaryRecentCommits(ctx, repoRoot)
	areas, evidence, rawCandidateCount := buildPathBoundaryAreas(repoRoot, repoName, files, recentCommits, opts.MaxAreas)
	confidence := mapBoundaryOutputConfidence(areas)
	caveats := []string{"experimental path-primary boundary map; git/docs/tests boost boundaries but do not define them"}
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
	if indexed, err := mapRepoHasIndexedArtifacts(repoRoot); err == nil && !indexed {
		caveats = append(caveats, mapIndexRequiredCaveat)
	} else if err != nil {
		debugLog("map boundary index availability unavailable: %v", err)
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

func buildPathBoundaryAreas(repoRoot, repoName string, files []string, commits []parsedFindGitCommit, maxAreas int) ([]mapArea, mapEvidenceAvailability, int) {
	candidates := map[string]*mapPathBoundaryCandidate{}
	evidence := mapEvidenceAvailability{}
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
		left := mapBoundaryAreaScore(internals[i])
		right := mapBoundaryAreaScore(internals[j])
		if left == right {
			return internals[i].Label < internals[j].Label
		}
		return left > right
	})
	internals = selectMapBoundaryAreas(internals, maxAreas*2)
	areas := publicMapAreas(repoRoot, repoName, internals, maxAreas)
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
			if len(candidate.Artifacts) < mapBoundaryMaxArtifacts*3 {
				candidate.Artifacts = append(candidate.Artifacts, mapArtifactForBoundaryPath(path, family))
			}
		}
		for _, boundaryPath := range mapBoundaryContainingDirs(path, labelCandidate.Key) {
			candidate.BoundaryPaths[boundaryPath] = true
		}
		for _, subarea := range mapBoundarySubareasForCandidate(path, labelCandidate.Key) {
			if len(candidate.Subareas) < 20 {
				candidate.Subareas[subarea] = true
			}
		}
	}
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
	for _, sourcePath := range files {
		if sourceRead >= mapBoundaryMaxImportFiles {
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
	area := &mapAreaInternal{
		Key:             candidate.Key,
		Label:           candidate.Label,
		LabelScore:      candidate.LabelScore,
		LabelSource:     "path_boundary",
		Subareas:        copyBoolMap(candidate.Subareas),
		RawAnchors:      rawAnchors,
		Artifacts:       dedupeMapArtifacts(candidate.Artifacts),
		ArtifactPathSet: copyBoolMap(candidate.PathSet),
		EvidenceCounts:  mapCopyCounts(candidate.EvidenceCounts),
		EvidenceCount:   candidate.FileCount + candidate.RecentCount,
		ConfidenceSum:   mapBoundaryCandidateConfidence(candidate) * float64(maxInt(1, len(rawAnchors))),
		EvidenceSources: copyBoolMap(candidate.EvidenceSources),
		TraceReceipts:   firstMapTraceReceipts(candidate.TraceReceipts, mapMaxTraceReceipts),
		Caveats:         []string{"path-primary boundary candidate"},
	}
	if len(area.Artifacts) > mapBoundaryMaxArtifacts {
		area.Artifacts = area.Artifacts[:mapBoundaryMaxArtifacts]
	}
	return area
}

func mapBoundaryRawAnchors(candidate *mapPathBoundaryCandidate) []string {
	anchors := []string{candidate.Label, candidate.Key}
	for path := range candidate.BoundaryPaths {
		anchors = appendUniqueString(anchors, path)
		if len(anchors) >= 6 {
			break
		}
	}
	return anchors
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

func selectMapBoundaryAreas(areas []*mapAreaInternal, limit int) []*mapAreaInternal {
	if limit <= 0 {
		limit = mapDefaultMaxAreas * 2
	}
	var out []*mapAreaInternal
	for _, area := range areas {
		overlapped := false
		for _, selected := range out {
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

func mapBoundaryAreaScore(area *mapAreaInternal) float64 {
	score := mapAreaScore(area)
	score += float64(mapMinInt(len(area.Subareas), 5)) * 3
	score += float64(mapMinInt(len(area.ArtifactPathSet), 40)) * 0.25
	score += float64(mapMinInt(area.EvidenceCounts["trace"], 4)) * 4
	score += float64(mapMinInt(area.EvidenceCounts["import"], mapBoundaryImportScoreCap)) * 0.35
	score += float64(mapMinInt(area.EvidenceCounts["test_import"], mapBoundaryTestImportScoreCap)) * 0.9
	if area.EvidenceCounts["source"] > 0 && area.EvidenceCounts["test"] > 0 {
		score += 5
	}
	if area.EvidenceCounts["source"] > 0 && (area.EvidenceCounts["doc"] > 0 || area.EvidenceCounts["intent"] > 0) {
		score += 4
	}
	return score
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

func mapBoundaryPathEligible(path string) bool {
	path = normalizeMapPath(path)
	if path == "" || mapPathSuppressed(path) || mapRecentPathNoisy(path) {
		return false
	}
	parts := strings.Split(strings.ToLower(path), "/")
	for _, part := range parts {
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

func publicMapAreas(repoRoot, repoName string, areas []*mapAreaInternal, maxAreas int) []mapArea {
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
		try := mapTryCommand(label, covers, traceReceipts, confidence)
		areaType := classifyMapAreaType(area, label, areaClass, isRoot, covers)
		caveats := mapAreaCaveats(area, areaClass, isRoot)
		if isRoot && len(covers) == 0 {
			caveats = appendUniqueString(caveats, "package-root signal only")
		}
		pub := mapArea{
			ID:                 "area." + normalizeMapKey(label),
			Label:              label,
			Class:              areaClass,
			AreaType:           areaType,
			Confidence:         confidence,
			IsRepoRootUmbrella: isRoot,
			Covers:             firstStrings(covers, mapMaxCoversPerArea),
			EvidenceCounts:     mapCopyCounts(area.EvidenceCounts),
			KeyPaths:           mapArtifactPaths(area.Artifacts),
			TraceReceipts:      firstMapTraceReceipts(traceReceipts, mapMaxTraceReceipts),
			Try:                try,
			Caveats:            caveats,
			Diagnostics: mapAreaDiagnostics{
				Key:              area.Key,
				RawAnchors:       firstStrings(area.RawAnchors, 8),
				LabelEvidence:    mapLabelEvidence(area),
				TraceTerms:       mapTraceTerms(traceReceipts),
				TraceReceiptMode: mapTraceReceiptMode,
			},
		}
		out = append(out, pub)
	}
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
		fmt.Fprintln(out, "Candidate areas")
	} else {
		fmt.Fprintln(out, "Detected areas")
	}
	fmt.Fprintln(out)
	for i, area := range m.Areas {
		fmt.Fprintf(out, "%d. %s\n", i+1, area.Label)
		if area.AreaType != "" {
			fmt.Fprintf(out, "   Type: %s\n", displayMapAreaType(area.AreaType))
		}
		if len(area.Covers) > 0 {
			fmt.Fprintf(out, "   Covers: %s\n", strings.Join(area.Covers, ", "))
		}
		fmt.Fprintf(out, "   Evidence: %s\n", mapAreaEvidenceText(area.EvidenceCounts))
		if len(area.KeyPaths) > 0 {
			fmt.Fprintln(out, "   Key files:")
			for _, p := range firstStrings(area.KeyPaths, 3) {
				fmt.Fprintf(out, "   - %s\n", p)
			}
		}
		if len(area.TraceReceipts) > 0 {
			fmt.Fprintf(out, "   Recent signal: %s\n", area.TraceReceipts[0].Subject)
		}
		if area.Try != "" {
			fmt.Fprintf(out, "   Try: %s\n", area.Try)
		}
		if verbose {
			fmt.Fprintf(out, "   Diagnostics: class=%s confidence=%s key=%s\n", area.Class, area.Confidence, area.Diagnostics.Key)
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
		fmt.Fprintf(out, "Diagnostics: match_score=%d class=%s key=%s\n", matches[0].Score, primary.Class, primary.Diagnostics.Key)
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
	topics, skipped := buildMapRecentTopics(commits, opts.AreaQuery, mapRecentMaxTopics)
	out.Topics = topics
	out.Diagnostics = mapRecentDiagnostics{
		CommitsRead:       len(commits),
		CommitsSkipped:    skipped,
		RawTopicCount:     len(topics),
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

func buildFastMapFallbackOutput(ctx context.Context, repoRoot string, opts mapOptions, indexReady bool) mapOutput {
	recent := buildMapRecentOutput(ctx, repoRoot, mapOptions{MaxAreas: opts.MaxAreas})
	return buildFastMapFallbackOutputFromRecent(repoRoot, recent, indexReady)
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
	sort.SliceStable(topics, func(i, j int) bool {
		if topics[i].Score != topics[j].Score {
			return topics[i].Score > topics[j].Score
		}
		return topics[i].Label < topics[j].Label
	})
	if len(topics) > limit {
		topics = topics[:limit]
	}
	return topics, skipped
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
	if findGitReceiptCommitNoisy(commit) || mapCommitNoisy(commit) {
		return true
	}
	if len(commit.paths) == 0 {
		return true
	}
	subject := strings.ToLower(commit.subject)
	switch {
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

func mapRecentPathNoisy(path string) bool {
	parts := strings.Split(normalizeMapPath(path), "/")
	for _, part := range parts {
		if mapSuppressedPathSegments[part] || part == "coverage" || part == ".next" {
			return true
		}
	}
	lower := strings.ToLower(filepath.ToSlash(path))
	return strings.Contains(lower, "/coverage/") ||
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
	case "public", "private", "yaml", "yml", "json", "sql", "markdown":
		return false
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
	switch {
	case strings.Contains(lower, "/test/") ||
		strings.Contains(lower, "/tests/") ||
		strings.Contains(lower, "__tests__") ||
		strings.Contains(lower, "_test.") ||
		strings.Contains(lower, ".test.") ||
		strings.Contains(lower, ".spec."):
		return "test"
	case strings.Contains(lower, "/docs/") ||
		strings.HasPrefix(lower, "docs/") ||
		ext == ".md" || ext == ".mdx" || ext == ".rst" || ext == ".adoc":
		return "doc"
	case ext == ".sql" ||
		strings.Contains(lower, "/migrations/") ||
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
		fmt.Fprintf(out, "   Evidence: %s\n", mapRecentEvidenceText(topic))
		if len(topic.RecentSignals) > 0 {
			receipt := topic.RecentSignals[0]
			if receipt.SHA != "" {
				fmt.Fprintf(out, "   Recent signal: %s %s\n", receipt.SHA, receipt.Subject)
			} else {
				fmt.Fprintf(out, "   Recent signal: %s\n", receipt.Subject)
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
		area.Try = mapTryCommand(area.Label, area.Covers, area.TraceReceipts, area.Confidence)
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
	if area.Try != "" {
		commands = appendUniqueString(commands, area.Try)
	}
	for _, cover := range firstStrings(area.Covers, 4) {
		if query := joinMapQuery(area.Label, cover); query != "" {
			commands = appendUniqueString(commands, mapFindPackCommand(query))
		}
	}
	for _, receipt := range firstMapTraceReceipts(area.TraceReceipts, 2) {
		if query := mapTraceQuery(area.Label, area.Covers, receipt.Subject); query != "" {
			commands = appendUniqueString(commands, mapFindPackCommand(query))
		}
	}
	if len(commands) == 0 && area.Label != "" {
		commands = append(commands, mapFindPackCommand(area.Label))
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

func mapTryCommand(label string, covers []string, receipts []mapTraceReceipt, confidence string) string {
	if confidence == mapLowConfidence && len(covers) == 0 {
		return ""
	}
	query := label
	if len(covers) > 0 {
		query = joinMapQuery(label, covers[0])
	}
	if len(receipts) > 0 {
		if traceQuery := mapTraceQuery(label, covers, receipts[0].Subject); traceQuery != "" {
			query = traceQuery
		}
	}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return ""
	}
	return mapFindPackCommand(query)
}

func mapFindPackCommand(query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return ""
	}
	return fmt.Sprintf("ds find --pack %q", query)
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
		if len(words) == 1 && mapGenericTerms[words[0]] {
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
		if art.Path == "" || seen[art.Path] {
			continue
		}
		seen[art.Path] = true
		out = append(out, art)
	}
	sort.SliceStable(out, func(i, j int) bool {
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
