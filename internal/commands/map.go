package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

const (
	mapDefaultMaxAreas     = 8
	mapMaxArtifactsPerArea = 4
	mapMaxCoversPerArea    = 5
	mapMaxTraceReceipts    = 1
	mapMaxVerboseTrace     = 4
	mapSchemaVersion       = "devspecs.map.v1"
	mapTraceReceiptMode    = "bounded_git_path_receipts_v0"
	mapLowConfidence       = "low"
	mapMediumConfidence    = "medium"
	mapHighConfidence      = "high"
	mapClassStableArea     = "stable_area"
	mapClassWorkstream     = "workstream"
	mapClassDocTopic       = "doc_topic"
	mapClassProtocol       = "protocol"
	mapClassLowConfidence  = "low_confidence"
	mapTypeDomainFeature   = "domain_feature"
	mapTypeBusinessFlow    = "business_workflow"
	mapTypeExternal        = "external_integration"
	mapTypeAPI             = "api_surface"
	mapTypeUI              = "ui_surface"
	mapTypeDataModel       = "data_model"
	mapTypeDataPipeline    = "data_pipeline"
	mapTypePlatform        = "platform_capability"
	mapTypeOps             = "ops_runtime"
	mapTypeTooling         = "tooling_script"
	mapTypeTestQuality     = "test_quality"
	mapTypeProtocol        = "protocol_process"
	mapTypeDocs            = "docs_reference"
	mapTypeRoot            = "repo_root_umbrella"
	mapTypeUnknown         = "unknown_area"
)

// NewMapCmd creates the ds map command.
func NewMapCmd() *cobra.Command {
	var (
		path     string
		asJSON   bool
		verbose  bool
		maxAreas int
	)

	cmd := &cobra.Command{
		Use:   "map",
		Short: "Show a concise repo map and useful follow-up context commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMap(cmd, mapOptions{
				Path:     path,
				JSON:     asJSON,
				Verbose:  verbose,
				MaxAreas: maxAreas,
			})
		},
	}

	cmd.Flags().StringVar(&path, "path", ".", "Repository path to map")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show map diagnostics and extra evidence")
	cmd.Flags().IntVar(&maxAreas, "max-areas", mapDefaultMaxAreas, "Maximum areas to show")
	return cmd
}

type mapOptions struct {
	Path     string
	JSON     bool
	Verbose  bool
	MaxAreas int
}

type mapOutput struct {
	Schema               string                  `json:"schema"`
	Repo                 mapRepo                 `json:"repo"`
	EvidenceAvailability mapEvidenceAvailability `json:"evidence_availability"`
	Areas                []mapArea               `json:"areas"`
	Caveats              []string                `json:"caveats,omitempty"`
	Diagnostics          mapDiagnostics          `json:"diagnostics,omitempty"`
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

var mapSuppressedPathSegments = map[string]bool{
	"_ignore": true, ".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, "target": true, ".next": true, "coverage": true, "fixtures": true,
	"fixture": true, "testdata": true, "samples": true, "sample-corpora": true,
}

var mapProtocolSubtypes = map[string]bool{
	"agent_instruction": true, "skill": true, "protocol": true, "contributing": true,
}

func runMap(cmd *cobra.Command, opts mapOptions) error {
	start := time.Now()
	success := false
	props := map[string]any{
		"json":      opts.JSON,
		"verbose":   opts.Verbose,
		"max_areas": opts.MaxAreas,
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
	result, err := runMapScan(cmd.Context(), repoRoot)
	if err != nil {
		return err
	}
	out := buildMapOutput(repoRoot, result, opts)
	success = true
	props["confidence"] = out.Repo.Confidence
	props["area_count_bucket"] = telemetry.CountBucket(len(out.Areas))

	if opts.JSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	writeMapText(cmd.OutOrStdout(), out, opts.Verbose)
	return nil
}

func runMapScan(ctx context.Context, repoRoot string) (*scan.Result, error) {
	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	cfg = config.WithDefaultIntentCandidateDiscovery(cfg, true)
	cfg = config.WithTestCaseArtifacts(cfg, true)
	cfg = config.WithCodeCommentArtifacts(cfg, true)

	tempDir, err := os.MkdirTemp("", "devspecs-map-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary map index: %w", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := store.Open(filepath.Join(tempDir, "devspecs-map.db"))
	if err != nil {
		return nil, fmt.Errorf("open temporary map index: %w", err)
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
	result, err := scanner.RunWithOptions(ctx, repoRoot, cfg, scan.RunOptions{
		UseTransaction:            true,
		FreshIndex:                true,
		SkipAuthoredAtLookup:      true,
		IncludeGitEvidence:        true,
		IncludeWorkstreamEvidence: true,
		RichTypedIndex:            true,
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
		traceReceipts := mapTraceReceipts(repoRoot, area, covers)
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
	for _, key := range []string{"source", "test", "intent", "doc", "protocol", "trace", "other"} {
		if counts[key] > 0 {
			switch key {
			case "source":
				parts = append(parts, "source")
			case "test":
				parts = append(parts, "tests")
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
	if isMapGenericAnchor(area.Key) || mapGenericAreaLabels[area.Key] {
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
	return fmt.Sprintf("ds find --pack %q", query)
}

func mapTraceQuery(label string, covers []string, subject string) string {
	labelWords := mapStringSet(wordsFromMap(label))
	coverWords := map[string]bool{}
	for _, cover := range covers {
		for _, word := range wordsFromMap(cover) {
			coverWords[word] = true
		}
	}
	var extra []string
	for _, word := range wordsFromMap(stripMapCommitPrefix(subject)) {
		if mapTraceStopWord(word) || labelWords[word] {
			continue
		}
		if coverWords[word] || len(extra) == 0 {
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
