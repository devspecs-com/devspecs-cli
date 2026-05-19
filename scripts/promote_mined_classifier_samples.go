package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type docTypeConfig struct {
	Label      string
	SourceDir  string
	Limit      int
	Classifier string
	Kind       string
	Subtype    string
	Authority  string
}

type docCandidate struct {
	Config   docTypeConfig
	Hash     string
	SrcPath  string
	DestRel  string
	Metadata map[string]string
}

type promotedSample struct {
	ID             string
	Type           string
	Path           string
	SourceURL      string
	Repository     string
	CommitSHA      string
	License        string
	OriginalPath   string
	FormatLabel    string
	CanCommitFile  bool
	ReductionNotes string
	ClassifierCase bool
}

type bundleManifest struct {
	Version           int                  `json:"version"`
	StructureType     string               `json:"structure_type"`
	FormatProfile     string               `json:"format_profile"`
	ArtifactFamily    string               `json:"artifact_family"`
	Repository        string               `json:"repository"`
	CommitSHA         string               `json:"commit_sha"`
	License           string               `json:"license"`
	SourceURL         string               `json:"source_url"`
	StorageDecision   string               `json:"storage_decision"`
	CanCommitFullFile bool                 `json:"can_commit_full_file"`
	RootPath          string               `json:"root_path"`
	LayoutGroup       string               `json:"layout_group"`
	SourceIdentity    string               `json:"source_identity"`
	PrimaryPath       string               `json:"primary_path"`
	Kind              string               `json:"kind"`
	Subtype           string               `json:"subtype"`
	Title             string               `json:"title"`
	Status            string               `json:"status"`
	CompositeHash     string               `json:"composite_hash"`
	ManifestRef       string               `json:"manifest_ref"`
	Files             []bundleManifestFile `json:"files"`
}

type bundleManifestFile struct {
	Path            string `json:"path"`
	Role            string `json:"role"`
	SourceType      string `json:"source_type"`
	FormatProfile   string `json:"format_profile"`
	LayoutGroup     string `json:"layout_group"`
	SourceIdentity  string `json:"source_identity"`
	SourceURL       string `json:"source_url"`
	SHA256          string `json:"sha256"`
	StorageDecision string `json:"storage_decision"`
	BodyPresent     bool   `json:"body_present"`
}

type bundleCandidate struct {
	ID       string
	SrcDir   string
	DestRoot string
	DestRel  string
	Manifest bundleManifest
}

type counts struct {
	Available       int
	Selected        int
	ClassifierCases int
}

func main() {
	var (
		minerOut          string
		fixtureDir        string
		adrLimit          int
		rfcLimit          int
		prdLimit          int
		bmadLimit         int
		bmadStoryLimit    int
		apiSpecLimit      int
		openspecLimit     int
		generatedAtString string
	)
	flag.StringVar(&minerOut, "miner-out", filepath.FromSlash("../devspecs-sample-miner/intent_corpus_prod_20260518-192521"), "Path to a devspecs-sample-miner output directory")
	flag.StringVar(&fixtureDir, "fixture", filepath.FromSlash("fixtures/mined-intent-samples"), "Fixture directory to regenerate")
	flag.IntVar(&adrLimit, "adr-limit", 30, "Maximum ADR document samples to promote")
	flag.IntVar(&rfcLimit, "rfc-limit", 50, "Maximum RFC document samples to promote")
	flag.IntVar(&prdLimit, "prd-limit", 30, "Maximum PRD document samples to promote")
	flag.IntVar(&bmadLimit, "bmad-limit", 30, "Maximum BMAD samples to promote without classifier assertions")
	flag.IntVar(&bmadStoryLimit, "bmad-like-story-limit", 30, "Maximum BMAD-like story samples to promote without classifier assertions")
	flag.IntVar(&apiSpecLimit, "api-spec-limit", 20, "Maximum API spec samples to promote without classifier assertions")
	flag.IntVar(&openspecLimit, "openspec-bundle-limit", 30, "Maximum OpenSpec bundles to promote as classifier container cases")
	flag.StringVar(&generatedAtString, "generated-at", "", "Override generated timestamp, RFC3339 UTC")
	flag.Parse()

	generatedAt := time.Now().UTC()
	if generatedAtString != "" {
		parsed, err := time.Parse(time.RFC3339, generatedAtString)
		if err != nil {
			fatalf("parse -generated-at: %v", err)
		}
		generatedAt = parsed.UTC()
	}

	if err := run(minerOut, fixtureDir, generatedAt, []docTypeConfig{
		{Label: "adr", SourceDir: "adr", Limit: adrLimit, Classifier: "adr", Kind: "decision", Authority: "high_decision"},
		{Label: "rfc", SourceDir: "rfc", Limit: rfcLimit, Classifier: "rfc", Kind: "design", Authority: "design_proposal"},
		{Label: "prd", SourceDir: "prd", Limit: prdLimit, Classifier: "prd", Kind: "requirements", Subtype: "prd", Authority: "product_background"},
		{Label: "bmad", SourceDir: "bmad", Limit: bmadLimit},
		{Label: "bmad_like_story", SourceDir: "bmad_like_story", Limit: bmadStoryLimit},
		{Label: "api_spec", SourceDir: "api_spec", Limit: apiSpecLimit},
	}, openspecLimit); err != nil {
		fatalf("%v", err)
	}
}

func run(minerOut, fixtureDir string, generatedAt time.Time, docConfigs []docTypeConfig, openspecLimit int) error {
	minerAbs, err := filepath.Abs(minerOut)
	if err != nil {
		return err
	}
	fixtureAbs, err := filepath.Abs(fixtureDir)
	if err != nil {
		return err
	}
	if err := validateFixtureTarget(fixtureAbs); err != nil {
		return err
	}
	if _, err := os.Stat(minerAbs); err != nil {
		return fmt.Errorf("miner output not readable: %w", err)
	}
	if err := cleanFixture(fixtureAbs); err != nil {
		return err
	}

	var samples []promotedSample
	caseBuilder := &strings.Builder{}
	countsByType := map[string]counts{}

	writeClassifierHeader(caseBuilder)
	for _, cfg := range docConfigs {
		selected, available, err := selectDocumentSamples(minerAbs, cfg)
		if err != nil {
			return err
		}
		typeCounts := counts{Available: available, Selected: len(selected)}
		for i := range selected {
			sample, err := copyDocumentSample(fixtureAbs, selected[i])
			if err != nil {
				return err
			}
			if cfg.Classifier != "" {
				typeCounts.ClassifierCases++
				sample.ClassifierCase = true
				writeDocumentCase(caseBuilder, sample, cfg)
			}
			samples = append(samples, sample)
		}
		countsByType[cfg.Label] = typeCounts
	}

	bundles, availableBundles, err := selectOpenSpecBundles(minerAbs, openspecLimit)
	if err != nil {
		return err
	}
	bundleCounts := counts{Available: availableBundles, Selected: len(bundles), ClassifierCases: len(bundles)}
	for _, bundle := range bundles {
		sample, err := copyOpenSpecBundle(fixtureAbs, bundle)
		if err != nil {
			return err
		}
		sample.ClassifierCase = true
		writeOpenSpecBundleCase(caseBuilder, sample, bundle)
		samples = append(samples, sample)
	}
	countsByType["openspec_bundle"] = bundleCounts

	if err := os.WriteFile(filepath.Join(fixtureAbs, "classifier_cases.yaml"), []byte(caseBuilder.String()), 0o644); err != nil {
		return fmt.Errorf("write classifier cases: %w", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureAbs, "cases.yaml"), []byte(casesYAML(generatedAt)), 0o644); err != nil {
		return fmt.Errorf("write cases labels: %w", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureAbs, "manifest.yaml"), []byte(manifestYAML(minerAbs, generatedAt, countsByType, samples)), 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureAbs, "README.md"), []byte(readmeMarkdown(minerAbs, countsByType)), 0o644); err != nil {
		return fmt.Errorf("write README: %w", err)
	}

	fmt.Printf("promoted %d samples into %s\n", len(samples), fixtureAbs)
	fmt.Printf("classifier cases: %d\n", totalClassifierCases(countsByType))
	for _, key := range sortedCountKeys(countsByType) {
		c := countsByType[key]
		fmt.Printf("- %s: selected %d/%d, classifier cases %d\n", key, c.Selected, c.Available, c.ClassifierCases)
	}
	return nil
}

func validateFixtureTarget(fixtureAbs string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	fixturesAbs := filepath.Join(cwd, "fixtures")
	rel, err := filepath.Rel(fixturesAbs, fixtureAbs)
	if err != nil {
		return err
	}
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("fixture must be under %s, got %s", fixturesAbs, fixtureAbs)
	}
	if filepath.Base(filepath.Clean(fixtureAbs)) != "mined-intent-samples" {
		return fmt.Errorf("refusing to clean unexpected fixture directory %s", fixtureAbs)
	}
	return nil
}

func cleanFixture(fixtureAbs string) error {
	if err := os.RemoveAll(filepath.Join(fixtureAbs, "samples")); err != nil {
		return fmt.Errorf("clean fixture samples: %w", err)
	}
	for _, name := range []string{"README.md", "cases.yaml", "classifier_cases.yaml", "manifest.yaml"} {
		path := filepath.Join(fixtureAbs, name)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clean fixture file %s: %w", path, err)
		}
	}
	return os.MkdirAll(fixtureAbs, 0o755)
}

func selectDocumentSamples(minerAbs string, cfg docTypeConfig) ([]docCandidate, int, error) {
	sourceRoot := filepath.Join(minerAbs, "testdata", "classifier-samples", "real", filepath.FromSlash(cfg.SourceDir))
	files, err := sortedMarkdownFiles(sourceRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	byRepo := map[string][]docCandidate{}
	available := 0
	for _, src := range files {
		metadata, err := parseMinerFrontmatter(src)
		if err != nil {
			return nil, 0, err
		}
		if !truthy(firstNonEmpty(metadata, "CanCommitFullFile", "CanCommitFile")) {
			continue
		}
		available++
		hash := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
		repo := firstNonEmpty(metadata, "Repository")
		if repo == "" {
			repo = "unknown/unknown"
		}
		candidate := docCandidate{
			Config:   cfg,
			Hash:     hash,
			SrcPath:  src,
			Metadata: metadata,
		}
		byRepo[repo] = append(byRepo[repo], candidate)
	}
	for repo := range byRepo {
		sort.Slice(byRepo[repo], func(i, j int) bool {
			left := firstNonEmpty(byRepo[repo][i].Metadata, "OriginalPath")
			right := firstNonEmpty(byRepo[repo][j].Metadata, "OriginalPath")
			if left == right {
				return byRepo[repo][i].Hash < byRepo[repo][j].Hash
			}
			return left < right
		})
	}
	return takeRoundRobin(byRepo, cfg.Limit), available, nil
}

func sortedMarkdownFiles(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			out = append(out, filepath.Join(root, entry.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func takeRoundRobin(groups map[string][]docCandidate, limit int) []docCandidate {
	keys := sortedKeys(groups)
	if limit <= 0 {
		limit = 1 << 30
	}
	offsets := map[string]int{}
	var selected []docCandidate
	for len(selected) < limit {
		added := false
		for _, key := range keys {
			group := groups[key]
			if offsets[key] >= len(group) {
				continue
			}
			selected = append(selected, group[offsets[key]])
			offsets[key]++
			added = true
			if len(selected) >= limit {
				break
			}
		}
		if !added {
			break
		}
	}
	return selected
}

func copyDocumentSample(fixtureAbs string, candidate docCandidate) (promotedSample, error) {
	repo := firstNonEmpty(candidate.Metadata, "Repository")
	originalPath := firstNonEmpty(candidate.Metadata, "OriginalPath", "SourcePath")
	destRel := filepath.ToSlash(filepath.Join("samples", "repos", safeRepoSlug(repo), cleanOriginalPath(originalPath, candidate.Hash)))
	destRel = withCollisionSuffix(fixtureAbs, destRel, candidate.Hash)
	destAbs := filepath.Join(fixtureAbs, filepath.FromSlash(destRel))
	if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
		return promotedSample{}, err
	}
	data, err := os.ReadFile(candidate.SrcPath)
	if err != nil {
		return promotedSample{}, err
	}
	data = stripFirstFrontmatter(data)
	if err := os.WriteFile(destAbs, data, 0o644); err != nil {
		return promotedSample{}, fmt.Errorf("copy %s to %s: %w", candidate.SrcPath, destAbs, err)
	}
	id := "mined-" + strings.ReplaceAll(candidate.Config.Label, "_", "-") + "-" + short(candidate.Hash)
	return promotedSample{
		ID:             id,
		Type:           candidate.Config.Label,
		Path:           destRel,
		SourceURL:      firstNonEmpty(candidate.Metadata, "SourceURL", "OriginURL"),
		Repository:     repo,
		CommitSHA:      firstNonEmpty(candidate.Metadata, "CommitSHA"),
		License:        firstNonEmpty(candidate.Metadata, "License"),
		OriginalPath:   originalPath,
		FormatLabel:    firstNonEmpty(candidate.Metadata, "FormatLabel", "DetectedType", "LLMSuggestedType"),
		CanCommitFile:  true,
		ReductionNotes: "Miner metadata frontmatter removed; original markdown content preserved.",
	}, nil
}

func selectOpenSpecBundles(minerAbs string, limit int) ([]bundleCandidate, int, error) {
	root := filepath.Join(minerAbs, "structures", "openspec", "bundles")
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	byRepo := map[string][]bundleCandidate{}
	available := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		manifestPath := filepath.Join(dir, "manifest.json")
		manifest, err := readBundleManifest(manifestPath)
		if err != nil {
			return nil, 0, err
		}
		if !manifest.CanCommitFullFile || manifest.StorageDecision != "real_full_file" {
			continue
		}
		if !completeOpenSpecBundle(manifest) {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "files")); err != nil {
			continue
		}
		available++
		repo := manifest.Repository
		if repo == "" {
			repo = "unknown/unknown"
		}
		hash := manifest.CompositeHash
		if hash == "" {
			hash = entry.Name()
		}
		byRepo[repo] = append(byRepo[repo], bundleCandidate{
			ID:       "mined-openspec-bundle-" + short(hash),
			SrcDir:   dir,
			DestRoot: filepath.ToSlash(filepath.Join("samples", "repos", safeRepoSlug(repo))),
			DestRel:  filepath.ToSlash(filepath.Join("samples", "repos", safeRepoSlug(repo), cleanOriginalPath(manifest.LayoutGroup, hash))),
			Manifest: manifest,
		})
	}
	for repo := range byRepo {
		sort.Slice(byRepo[repo], func(i, j int) bool {
			return byRepo[repo][i].Manifest.LayoutGroup < byRepo[repo][j].Manifest.LayoutGroup
		})
	}
	return takeRoundRobinBundles(byRepo, limit), available, nil
}

func readBundleManifest(path string) (bundleManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return bundleManifest{}, err
	}
	var manifest bundleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return bundleManifest{}, fmt.Errorf("parse bundle manifest %s: %w", path, err)
	}
	return manifest, nil
}

func completeOpenSpecBundle(manifest bundleManifest) bool {
	roles := map[string]bool{}
	for _, file := range manifest.Files {
		if !file.BodyPresent {
			continue
		}
		roles[normalizeOpenSpecRole(file.Role)] = true
	}
	return roles["proposal"] && roles["design"] && roles["tasks"]
}

func takeRoundRobinBundles(groups map[string][]bundleCandidate, limit int) []bundleCandidate {
	keys := sortedKeys(groups)
	if limit <= 0 {
		limit = 1 << 30
	}
	offsets := map[string]int{}
	var selected []bundleCandidate
	for len(selected) < limit {
		added := false
		for _, key := range keys {
			group := groups[key]
			if offsets[key] >= len(group) {
				continue
			}
			selected = append(selected, group[offsets[key]])
			offsets[key]++
			added = true
			if len(selected) >= limit {
				break
			}
		}
		if !added {
			break
		}
	}
	return selected
}

func copyOpenSpecBundle(fixtureAbs string, bundle bundleCandidate) (promotedSample, error) {
	filesRoot := filepath.Join(bundle.SrcDir, "files")
	destRootAbs := filepath.Join(fixtureAbs, filepath.FromSlash(bundle.DestRoot))
	if err := filepath.WalkDir(filesRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(filesRoot, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(destRootAbs, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	}); err != nil {
		return promotedSample{}, fmt.Errorf("copy openspec bundle %s: %w", bundle.SrcDir, err)
	}
	manifestData, err := json.MarshalIndent(bundle.Manifest, "", "  ")
	if err != nil {
		return promotedSample{}, err
	}
	manifestDest := filepath.Join(destRootAbs, ".devspecs-mined", "openspec-bundle-"+short(bundle.Manifest.CompositeHash)+".manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestDest), 0o755); err != nil {
		return promotedSample{}, err
	}
	if err := os.WriteFile(manifestDest, append(manifestData, '\n'), 0o644); err != nil {
		return promotedSample{}, err
	}
	return promotedSample{
		ID:             bundle.ID,
		Type:           "openspec_bundle",
		Path:           bundle.DestRel,
		SourceURL:      bundle.Manifest.SourceURL,
		Repository:     bundle.Manifest.Repository,
		CommitSHA:      bundle.Manifest.CommitSHA,
		License:        bundle.Manifest.License,
		OriginalPath:   bundle.Manifest.LayoutGroup,
		FormatLabel:    "OPENSPEC",
		CanCommitFile:  true,
		ReductionNotes: "Full OpenSpec change bundle preserved under original repository-relative layout.",
	}, nil
}

func parseMinerFrontmatter(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseFrontmatter(data), nil
}

func parseFrontmatter(data []byte) map[string]string {
	out := map[string]string{}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return out
	}
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "---" {
			break
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "-") || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"")
		value = strings.ReplaceAll(value, `\"`, `"`)
		out[key] = value
	}
	return out
}

func stripFirstFrontmatter(data []byte) []byte {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return data
	}
	lines := bytes.SplitAfter(normalized, []byte("\n"))
	offset := 0
	for i, line := range lines {
		offset += len(line)
		if i == 0 {
			continue
		}
		if strings.TrimSpace(string(line)) == "---" {
			for offset < len(normalized) && (normalized[offset] == '\n' || normalized[offset] == '\r') {
				offset++
			}
			return normalized[offset:]
		}
	}
	return data
}

func writeClassifierHeader(b *strings.Builder) {
	b.WriteString("version: 1\n")
	b.WriteString("fixture: mined-intent-samples\n")
	b.WriteString("classifier_cases:\n")
}

func writeDocumentCase(b *strings.Builder, sample promotedSample, cfg docTypeConfig) {
	b.WriteString("  - id: " + yq(sample.ID) + "\n")
	b.WriteString("    path: " + yq(sample.Path) + "\n")
	b.WriteString("    scope: document\n")
	b.WriteString("    expected:\n")
	b.WriteString("      classifier: " + yq(cfg.Classifier) + "\n")
	b.WriteString("      scope: document\n")
	if cfg.Kind != "" {
		b.WriteString("      kind: " + yq(cfg.Kind) + "\n")
	}
	if cfg.Subtype != "" {
		b.WriteString("      subtype: " + yq(cfg.Subtype) + "\n")
	}
	if cfg.Authority != "" {
		b.WriteString("      authority: " + yq(cfg.Authority) + "\n")
	}
	b.WriteString("      should_index: true\n")
	writeProvenance(b, sample)
	b.WriteString("    notes: " + yq("Real GitHub sample promoted from miner output. Expected label comes from miner+LLM adjudication and should be manually reviewed before becoming training data.") + "\n\n")
}

func writeOpenSpecBundleCase(b *strings.Builder, sample promotedSample, bundle bundleCandidate) {
	b.WriteString("  - id: " + yq(sample.ID) + "\n")
	b.WriteString("    path: " + yq(sample.Path) + "\n")
	b.WriteString("    scope: container\n")
	b.WriteString("    expected:\n")
	b.WriteString("      classifier: openspec\n")
	b.WriteString("      scope: container\n")
	b.WriteString("      kind: spec\n")
	b.WriteString("      authority: high_current_intent\n")
	b.WriteString("      format_profile: openspec\n")
	b.WriteString("      should_index: true\n")
	b.WriteString("      required_reasons:\n")
	b.WriteString("        - layout_match\n")
	b.WriteString("      child_candidates:\n")
	for _, file := range sortedBundleFiles(bundle.Manifest.Files) {
		role := normalizeOpenSpecRole(file.Role)
		if role == "" {
			continue
		}
		childPath := filepath.ToSlash(filepath.Join(bundle.DestRoot, cleanOriginalPath(file.Path, file.SHA256)))
		b.WriteString("        - path: " + yq(childPath) + "\n")
		b.WriteString("          role: " + yq(role) + "\n")
	}
	writeProvenance(b, sample)
	b.WriteString("    notes: " + yq("Real OpenSpec change bundle promoted as a container case; child files retain proposal/design/tasks/spec-delta roles.") + "\n\n")
}

func writeProvenance(b *strings.Builder, sample promotedSample) {
	b.WriteString("    provenance:\n")
	if sample.SourceURL != "" {
		b.WriteString("      source_url: " + yq(sample.SourceURL) + "\n")
	}
	if sample.Repository != "" {
		b.WriteString("      repository: " + yq(sample.Repository) + "\n")
	}
	if sample.CommitSHA != "" {
		b.WriteString("      commit_sha: " + yq(sample.CommitSHA) + "\n")
	}
	if sample.License != "" {
		b.WriteString("      license: " + yq(sample.License) + "\n")
	}
	if sample.OriginalPath != "" {
		b.WriteString("      original_path: " + yq(sample.OriginalPath) + "\n")
	}
	if sample.FormatLabel != "" {
		b.WriteString("      format_label: " + yq(sample.FormatLabel) + "\n")
	}
	b.WriteString("      can_commit_file: " + fmt.Sprintf("%t", sample.CanCommitFile) + "\n")
	if sample.ReductionNotes != "" {
		b.WriteString("      reduction_notes: " + yq(sample.ReductionNotes) + "\n")
	}
}

func casesYAML(generatedAt time.Time) string {
	return "fixture_version: mined-intent-samples-v0\n" +
		"eval_stage: real_mined_holdout_v0\n" +
		"generated_at: " + yq(generatedAt.Format(time.RFC3339)) + "\n"
}

func manifestYAML(minerAbs string, generatedAt time.Time, countsByType map[string]counts, samples []promotedSample) string {
	sort.Slice(samples, func(i, j int) bool {
		if samples[i].Type == samples[j].Type {
			return samples[i].Path < samples[j].Path
		}
		return samples[i].Type < samples[j].Type
	})
	var b strings.Builder
	b.WriteString("version: 1\n")
	b.WriteString("fixture: mined-intent-samples\n")
	b.WriteString("source_run: " + yq(filepath.ToSlash(minerAbs)) + "\n")
	b.WriteString("generated_at: " + yq(generatedAt.Format(time.RFC3339)) + "\n")
	b.WriteString("policy:\n")
	b.WriteString("  full_files_only_when_license_compatible: true\n")
	b.WriteString("  original_paths_preserved: true\n")
	b.WriteString("  type_named_paths_avoided_for_classifier_cases: true\n")
	b.WriteString("  unsupported_labels_are_manifest_only: true\n")
	b.WriteString("counts:\n")
	for _, key := range sortedCountKeys(countsByType) {
		c := countsByType[key]
		b.WriteString("  " + key + ":\n")
		b.WriteString("    available: " + strconv.Itoa(c.Available) + "\n")
		b.WriteString("    selected: " + strconv.Itoa(c.Selected) + "\n")
		b.WriteString("    classifier_cases: " + strconv.Itoa(c.ClassifierCases) + "\n")
	}
	b.WriteString("samples:\n")
	for _, sample := range samples {
		b.WriteString("  - id: " + yq(sample.ID) + "\n")
		b.WriteString("    type: " + yq(sample.Type) + "\n")
		b.WriteString("    path: " + yq(sample.Path) + "\n")
		b.WriteString("    classifier_case: " + fmt.Sprintf("%t", sample.ClassifierCase) + "\n")
		if sample.SourceURL != "" {
			b.WriteString("    source_url: " + yq(sample.SourceURL) + "\n")
		}
		if sample.Repository != "" {
			b.WriteString("    repository: " + yq(sample.Repository) + "\n")
		}
		if sample.CommitSHA != "" {
			b.WriteString("    commit_sha: " + yq(sample.CommitSHA) + "\n")
		}
		if sample.License != "" {
			b.WriteString("    license: " + yq(sample.License) + "\n")
		}
		if sample.OriginalPath != "" {
			b.WriteString("    original_path: " + yq(sample.OriginalPath) + "\n")
		}
		if sample.FormatLabel != "" {
			b.WriteString("    format_label: " + yq(sample.FormatLabel) + "\n")
		}
		b.WriteString("    can_commit_file: " + fmt.Sprintf("%t", sample.CanCommitFile) + "\n")
		if sample.ReductionNotes != "" {
			b.WriteString("    reduction_notes: " + yq(sample.ReductionNotes) + "\n")
		}
	}
	return b.String()
}

func readmeMarkdown(minerAbs string, countsByType map[string]counts) string {
	var b strings.Builder
	b.WriteString("# Mined Intent Samples\n\n")
	b.WriteString("This fixture contains real GitHub samples promoted from the sample miner output. It is intended as a holdout cross-check for classifier work, separate from the synthetic `agentic-saas-fragmented` fixture.\n\n")
	b.WriteString("The fixture preserves repository-relative paths under `samples/repos/<owner>__<repo>/...` so classifier path evidence remains realistic. Miner metadata frontmatter is removed from document samples; provenance is recorded in `manifest.yaml` and each classifier case.\n\n")
	b.WriteString("## Source\n\n")
	b.WriteString("- Miner output: `" + filepath.ToSlash(minerAbs) + "`\n")
	b.WriteString("- Promotion command: `go run ./scripts/promote_mined_classifier_samples.go`\n\n")
	b.WriteString("## Counts\n\n")
	for _, key := range sortedCountKeys(countsByType) {
		c := countsByType[key]
		b.WriteString(fmt.Sprintf("- `%s`: selected %d/%d, classifier cases %d\n", key, c.Selected, c.Available, c.ClassifierCases))
	}
	b.WriteString("\n## Eval\n\n")
	b.WriteString("Run:\n\n")
	b.WriteString("```sh\n")
	b.WriteString("go run ./cmd/ds eval ./fixtures/mined-intent-samples --classifier --no-save\n")
	b.WriteString("```\n\n")
	b.WriteString("BMAD, BMAD-like story, and API spec samples are included in the manifest but are not classifier assertions yet because the deterministic classifier does not currently expose those as first-class model labels.\n")
	return b.String()
}

func sortedBundleFiles(files []bundleManifestFile) []bundleManifestFile {
	out := append([]bundleManifestFile(nil), files...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out
}

func normalizeOpenSpecRole(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "proposal", "design", "tasks":
		return strings.TrimSpace(strings.ToLower(role))
	case "delta_spec", "spec_delta", "spec":
		return "spec_delta"
	default:
		return strings.TrimSpace(strings.ToLower(role))
	}
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedCountKeys(m map[string]counts) []string {
	return sortedKeys(m)
}

func totalClassifierCases(m map[string]counts) int {
	total := 0
	for _, c := range m {
		total += c.ClassifierCases
	}
	return total
}

func firstNonEmpty(m map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(m[key]); value != "" {
			return value
		}
	}
	return ""
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1":
		return true
	default:
		return false
	}
}

func short(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}

func safeRepoSlug(repo string) string {
	repo = strings.Trim(strings.TrimSpace(repo), `"`)
	repo = strings.ReplaceAll(repo, "\\", "/")
	parts := strings.Split(repo, "/")
	for i := range parts {
		parts[i] = sanitizePathSegment(parts[i])
	}
	out := strings.Join(parts, "__")
	if strings.Trim(out, "_") == "" {
		return "unknown__unknown"
	}
	return out
}

func cleanOriginalPath(originalPath, fallbackHash string) string {
	originalPath = strings.Trim(strings.TrimSpace(originalPath), `"`)
	originalPath = strings.ReplaceAll(originalPath, "\\", "/")
	originalPath = strings.TrimLeft(originalPath, "/")
	var parts []string
	for _, part := range strings.Split(originalPath, "/") {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		parts = append(parts, sanitizePathSegment(part))
	}
	if len(parts) == 0 {
		return filepath.ToSlash(filepath.Join("unknown", fallbackHash+".md"))
	}
	return filepath.ToSlash(filepath.Join(parts...))
}

func sanitizePathSegment(segment string) string {
	segment = strings.Map(func(r rune) rune {
		switch {
		case r == '<' || r == '>' || r == ':' || r == '"' || r == '|' || r == '?' || r == '*':
			return '_'
		case unicode.IsControl(r):
			return '_'
		default:
			return r
		}
	}, segment)
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return "_"
	}
	return segment
}

func withCollisionSuffix(fixtureAbs, destRel, hash string) string {
	destAbs := filepath.Join(fixtureAbs, filepath.FromSlash(destRel))
	if _, err := os.Stat(destAbs); errors.Is(err, os.ErrNotExist) {
		return destRel
	}
	ext := filepath.Ext(destRel)
	base := strings.TrimSuffix(destRel, ext)
	return filepath.ToSlash(base + "." + short(hash) + ext)
}

func yq(s string) string {
	return strconv.Quote(s)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
