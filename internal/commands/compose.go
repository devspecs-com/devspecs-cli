package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

const (
	composeTypeADR = "adr"
	composeTypeRFC = "rfc"
	composeTypePRD = "prd"
)

var composeNumberedFilenameRE = regexp.MustCompile(`(?i)^(?:(adr|rfc|prd)[-_])?(\d+)([-_])(.+)\.md$`)

type composeOptions struct {
	DocumentType string
	Format       string
	Variant      string
	FromTask     string
	Target       string
	Output       string
	NoRefresh    bool
	AsJSON       bool
}

type composeCandidate struct {
	Path    string
	Body    string
	Format  string
	Variant string
}

type composeConvention struct {
	Directory string
	Format    string
	Variant   string
	Source    string
}

type composeOutput struct {
	Path             string `json:"path"`
	DocumentType     string `json:"document_type"`
	Format           string `json:"format"`
	Variant          string `json:"variant,omitempty"`
	ConventionSource string `json:"convention_source"`
	ArtifactID       string `json:"artifact_id"`
	FromTask         string `json:"from_task,omitempty"`
	Target           string `json:"target,omitempty"`
}

// NewComposeCmd creates repo-owned ADR, RFC, and PRD drafts.
func NewComposeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compose",
		Short: "Create a durable repo-owned decision or product document",
		Long: `Create a durable Markdown draft outside the DevSpecs task corpus.

Use an ADR after a settled technical choice, an RFC when a proposal needs
review before commitment, and a PRD for a product problem, users, outcomes,
and requirements. Skip local, obvious, reversible implementation details.`,
	}
	addRepoTargetPersistentFlag(cmd)
	cmd.AddCommand(newComposeADRCmd())
	cmd.AddCommand(newComposeRFCCmd())
	cmd.AddCommand(newComposePRDCmd())
	return cmd
}

func newComposeADRCmd() *cobra.Command {
	opts := composeOptions{DocumentType: composeTypeADR, Format: composeADRFormatAuto, Variant: "full"}
	cmd := &cobra.Command{
		Use:   "adr <title>",
		Short: "Create an architecture decision record draft",
		Long: `Create an ADR after a meaningful technical direction is settled.

Formats:
  nygard        Compact context, decision, and consequences.
  madr          Structured options and comparison; full or minimal variant.
  y-statement   One short reviewable decision statement plus fields.
  outcome-first Outcome, choice, and tradeoff before supporting rationale.
  iso-42010     Stakeholders, concerns, views, impact, and traceability.

The default format is auto: reuse an established repository convention, or
use Nygard when no ADR precedent exists.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompose(cmd, args[0], opts)
		},
	}
	addComposeCommonFlags(cmd, &opts)
	cmd.Flags().StringVar(&opts.Format, "format", opts.Format, "ADR format: auto, nygard, madr, y-statement, outcome-first, or iso-42010")
	cmd.Flags().StringVar(&opts.Variant, "variant", opts.Variant, "MADR variant: full or minimal")
	return cmd
}

func newComposeRFCCmd() *cobra.Command {
	opts := composeOptions{DocumentType: composeTypeRFC}
	cmd := &cobra.Command{
		Use:   "rfc <title>",
		Short: "Create a request-for-comments draft",
		Long:  "Create an RFC when a consequential proposal needs review before the direction is settled.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompose(cmd, args[0], opts)
		},
	}
	addComposeCommonFlags(cmd, &opts)
	return cmd
}

func newComposePRDCmd() *cobra.Command {
	opts := composeOptions{DocumentType: composeTypePRD}
	cmd := &cobra.Command{
		Use:   "prd <title>",
		Short: "Create a product requirements draft",
		Long:  "Create a PRD when a product problem, users, outcomes, and requirements should guide one or more task tracks.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompose(cmd, args[0], opts)
		},
	}
	addComposeCommonFlags(cmd, &opts)
	return cmd
}

func addComposeCommonFlags(cmd *cobra.Command, opts *composeOptions) {
	cmd.Flags().StringVar(&opts.FromTask, "from-task", "", "Related DevSpecs task ID")
	cmd.Flags().StringVar(&opts.Target, "target", "", "Related task series or slice target")
	cmd.Flags().StringVar(&opts.Output, "output", "", "Repository-relative output file; overrides inferred directory and filename")
	cmd.Flags().BoolVar(&opts.NoRefresh, "no-refresh", false, "Use the current index without a pre-compose freshness scan")
	cmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
}

func runCompose(cmd *cobra.Command, title string, opts composeOptions) error {
	started := time.Now()
	success := false
	props := map[string]any{
		"document_type": opts.DocumentType,
		"format":        opts.Format,
		"json":          opts.AsJSON,
	}
	defer func() {
		telemetry.RecordCommand("compose", success, time.Since(started), props)
	}()
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("document title is empty")
	}
	if err := validateComposeOptions(opts); err != nil {
		return err
	}
	repoRoot, err := resolveTargetRepoRootContext(cmd.Context(), commandRepoTarget(cmd))
	if err != nil {
		return err
	}
	provenance, err := composeTaskProvenance(cmd, repoRoot, opts)
	if err != nil {
		return err
	}
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	if !opts.NoRefresh {
		if err := ensureRepoIndexed(cmd, db, repoRoot); err != nil {
			return fmt.Errorf("compose convention refresh: %w", err)
		}
	}
	candidates, err := loadComposeCandidates(cmd.Context(), db, repoRoot, opts.DocumentType)
	if err != nil {
		return err
	}
	cfg, err := config.LoadRepoConfig(repoRoot)
	if err != nil {
		return fmt.Errorf("load repository config: %w", err)
	}
	convention, err := resolveComposeConvention(repoRoot, cfg, candidates, opts)
	if err != nil {
		return err
	}
	path, recordID, err := resolveComposeOutputPath(repoRoot, title, candidates, convention, opts)
	if err != nil {
		return err
	}
	body, err := renderComposeDocument(title, recordID, provenance, convention, opts)
	if err != nil {
		return err
	}
	if err := writeComposeFile(path, body); err != nil {
		return err
	}
	keepFile := false
	defer func() {
		if !keepFile {
			_ = os.Remove(path)
		}
	}()
	relPath, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return err
	}
	relPath = filepath.ToSlash(relPath)
	artifactID, err := captureComposedDocument(cmd, repoRoot, path, title, opts.DocumentType)
	if err != nil {
		return err
	}
	keepFile = true
	out := composeOutput{
		Path:             relPath,
		DocumentType:     opts.DocumentType,
		Format:           convention.Format,
		Variant:          convention.Variant,
		ConventionSource: convention.Source,
		ArtifactID:       artifactID,
		FromTask:         strings.TrimSpace(opts.FromTask),
		Target:           strings.TrimSpace(opts.Target),
	}
	if err := writeComposeOutput(cmd, out, opts.AsJSON); err != nil {
		return err
	}
	success = true
	return nil
}

func validateComposeOptions(opts composeOptions) error {
	if opts.Target != "" && opts.FromTask == "" {
		return fmt.Errorf("--target requires --from-task")
	}
	if opts.DocumentType != composeTypeADR {
		return nil
	}
	if !isAllowedValue(strings.ToLower(strings.TrimSpace(opts.Format)), composeADRFormats) {
		return fmt.Errorf("invalid ADR format %q; valid values: %s", opts.Format, strings.Join(composeADRFormats, ", "))
	}
	variant := strings.ToLower(strings.TrimSpace(opts.Variant))
	if variant != "full" && variant != "minimal" {
		return fmt.Errorf("invalid MADR variant %q; valid values: full, minimal", opts.Variant)
	}
	format := strings.ToLower(strings.TrimSpace(opts.Format))
	if format != composeADRFormatMADR && format != composeADRFormatAuto && variant != "full" {
		return fmt.Errorf("--variant applies only to --format madr or an auto-detected MADR convention")
	}
	return nil
}

func composeTaskProvenance(cmd *cobra.Command, repoRoot string, opts composeOptions) (string, error) {
	taskID := strings.TrimSpace(opts.FromTask)
	if taskID == "" {
		return "", nil
	}
	loadedRoot, _, manifest, err := loadTaskWorkspaceManifest(cmd, defaultTaskWorkspaceDir, taskID)
	if err != nil {
		return "", fmt.Errorf("load source task: %w", err)
	}
	if !samePath(loadedRoot, repoRoot) {
		return "", fmt.Errorf("source task %s belongs to %s, not target repository %s", taskID, loadedRoot, repoRoot)
	}
	target := strings.TrimSpace(opts.Target)
	if target != "" && !isTaskSeriesTarget(manifest, target) {
		if _, err := taskSliceForCheckpoint(manifest, target); err != nil {
			return "", fmt.Errorf("resolve source task target: %w", err)
		}
	}
	parts := []string{"task=" + taskID}
	if target != "" {
		parts = append(parts, "target="+target)
	}
	return "<!-- devspecs: " + strings.Join(parts, " ") + " -->", nil
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func renderComposeDocument(title, recordID, provenance string, convention composeConvention, opts composeOptions) (string, error) {
	input := composeTemplateInput{
		Title:      title,
		RecordID:   recordID,
		Variant:    convention.Variant,
		Provenance: provenance,
	}
	switch opts.DocumentType {
	case composeTypeADR:
		return renderComposeADR(convention.Format, input)
	case composeTypeRFC:
		return renderComposeRFC(input), nil
	case composeTypePRD:
		return renderComposePRD(input), nil
	default:
		return "", fmt.Errorf("unsupported compose document type %q", opts.DocumentType)
	}
}

func loadComposeCandidates(ctx context.Context, db *store.DB, repoRoot, documentType string) ([]composeCandidate, error) {
	byPath := map[string]composeCandidate{}
	artifacts, err := db.ListArtifacts(store.FilterParams{RepoRoot: repoRoot})
	if err != nil {
		return nil, fmt.Errorf("list indexed compose candidates: %w", err)
	}
	for _, artifact := range artifacts {
		sources, err := db.GetSourcesForArtifact(artifact.ID)
		if err != nil {
			return nil, err
		}
		var body string
		if artifact.CurrentRevID != "" {
			revision, err := db.GetRevision(artifact.CurrentRevID)
			if err != nil {
				return nil, err
			}
			body = revision.Body
		}
		for _, source := range sources {
			path := normalizeComposePath(source.Path)
			if composeCandidatePathSuppressed(path) || !composeArtifactMatchesType(documentType, artifact, path) {
				continue
			}
			byPath[path] = newComposeCandidate(path, body, documentType)
		}
	}
	paths, err := listComposeMarkdownPaths(ctx, repoRoot)
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		path = normalizeComposePath(path)
		if _, exists := byPath[path]; exists || composeCandidatePathSuppressed(path) || !composePathMatchesType(path, documentType) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(path)))
		if err != nil {
			continue
		}
		byPath[path] = newComposeCandidate(path, string(body), documentType)
	}
	var candidates []composeCandidate
	for _, candidate := range byPath {
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Path < candidates[j].Path })
	return candidates, nil
}

func newComposeCandidate(path, body, documentType string) composeCandidate {
	candidate := composeCandidate{Path: path, Body: body}
	if documentType == composeTypeADR {
		candidate.Format, candidate.Variant = detectComposeADRFormat(body)
	}
	return candidate
}

func composeArtifactMatchesType(documentType string, artifact store.ArtifactRow, path string) bool {
	switch documentType {
	case composeTypeADR:
		return artifact.Kind == config.KindDecision || artifact.Subtype == config.SubtypeADR || composePathMatchesType(path, documentType)
	case composeTypeRFC:
		return artifact.Kind == config.KindDesign && composePathMatchesType(path, documentType)
	case composeTypePRD:
		return artifact.Subtype == config.SubtypePRD || composePathMatchesType(path, documentType)
	default:
		return false
	}
}

func composePathMatchesType(path, documentType string) bool {
	path = strings.ToLower(normalizeComposePath(path))
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	segments := strings.Split(path, "/")
	hasSegment := func(want ...string) bool {
		for _, segment := range segments {
			for _, value := range want {
				if segment == value {
					return true
				}
			}
		}
		return false
	}
	switch documentType {
	case composeTypeADR:
		return hasSegment("adr", "adrs", "decision", "decisions") || strings.HasPrefix(base, "adr-") || strings.HasPrefix(base, "adr_")
	case composeTypeRFC:
		return hasSegment("rfc", "rfcs") || strings.HasPrefix(base, "rfc-") || strings.HasPrefix(base, "rfc_")
	case composeTypePRD:
		return hasSegment("prd", "prds", "product-spec", "product-specs") || strings.HasPrefix(base, "prd-") || strings.HasPrefix(base, "prd_")
	default:
		return false
	}
}

func composeCandidatePathSuppressed(path string) bool {
	segments := strings.Split(strings.ToLower(normalizeComposePath(path)), "/")
	for _, segment := range segments {
		switch segment {
		case ".git", ".devspecs", "devspecs", "fixtures", "fixture", "testdata", "vendor", "node_modules", "dist", "build":
			return true
		}
	}
	return false
}

func listComposeMarkdownPaths(ctx context.Context, repoRoot string) ([]string, error) {
	if findGitRepoAvailable(ctx, repoRoot) {
		gitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		out, err := exec.CommandContext(gitCtx, "git", "-C", repoRoot, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "*.md").Output()
		if err == nil {
			var paths []string
			for _, path := range strings.Split(string(out), "\x00") {
				path = normalizeComposePath(path)
				if path != "" {
					paths = append(paths, path)
				}
			}
			return paths, nil
		}
	}
	matcher, err := ignore.NewMatcher(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("load ignore rules for compose discovery: %w", err)
	}
	var paths []string
	err = filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil || rel == "." {
			return nil
		}
		rel = normalizeComposePath(rel)
		if matcher.ShouldSkip(rel, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			paths = append(paths, rel)
		}
		return nil
	})
	return paths, err
}

func resolveComposeConvention(repoRoot string, cfg *config.RepoConfig, candidates []composeCandidate, opts composeOptions) (composeConvention, error) {
	directory, source, err := resolveComposeDirectory(repoRoot, cfg, candidates, opts)
	if err != nil {
		return composeConvention{}, err
	}
	convention := composeConvention{Directory: directory, Format: opts.DocumentType, Source: source}
	if opts.DocumentType != composeTypeADR {
		return convention, nil
	}
	format := strings.ToLower(strings.TrimSpace(opts.Format))
	variant := strings.ToLower(strings.TrimSpace(opts.Variant))
	if format != composeADRFormatAuto {
		convention.Format = format
		if format == composeADRFormatMADR {
			convention.Variant = variant
		}
		return convention, nil
	}
	formatCandidates := composeCandidatesInDirectory(candidates, directory)
	if len(formatCandidates) == 0 {
		formatCandidates = candidates
	}
	format, detectedVariant, err := modalComposeADRFormat(formatCandidates)
	if err != nil {
		return composeConvention{}, err
	}
	if format == "" {
		if len(candidates) > 0 {
			return composeConvention{}, fmt.Errorf("existing ADRs do not match a supported format; choose --format explicitly")
		}
		format = composeADRFormatNygard
		convention.Source = "fallback"
	}
	convention.Format = format
	if format == composeADRFormatMADR {
		convention.Variant = detectedVariant
		if cmdVariant := strings.TrimSpace(opts.Variant); cmdVariant == "minimal" {
			convention.Variant = "minimal"
		}
		if convention.Variant == "" {
			convention.Variant = "full"
		}
	}
	return convention, nil
}

func resolveComposeDirectory(repoRoot string, cfg *config.RepoConfig, candidates []composeCandidate, opts composeOptions) (string, string, error) {
	if strings.TrimSpace(opts.Output) != "" {
		path, err := composeAbsoluteOutputPath(repoRoot, opts.Output)
		if err != nil {
			return "", "", err
		}
		rel, _ := filepath.Rel(repoRoot, filepath.Dir(path))
		return normalizeComposePath(rel), "explicit", nil
	}
	configured := configuredComposeDirectories(repoRoot, cfg, opts.DocumentType)
	for _, directory := range configured {
		if composeDirectoryHasCandidate(candidates, directory) {
			return directory, "configured", nil
		}
	}
	if len(candidates) > 0 {
		directory, err := modalComposeDirectory(candidates)
		if err != nil {
			return "", "", err
		}
		return directory, "existing", nil
	}
	if len(configured) > 0 {
		return configured[0], "configured", nil
	}
	switch opts.DocumentType {
	case composeTypeADR:
		return "docs/adr", "fallback", nil
	case composeTypeRFC:
		return "docs/rfcs", "fallback", nil
	case composeTypePRD:
		return "docs/prd", "fallback", nil
	default:
		return "", "", fmt.Errorf("unsupported compose document type %q", opts.DocumentType)
	}
}

func configuredComposeDirectories(repoRoot string, cfg *config.RepoConfig, documentType string) []string {
	if cfg == nil {
		return nil
	}
	var directories []string
	for _, source := range cfg.Sources {
		paths := append([]string(nil), source.Paths...)
		if source.Path != "" {
			paths = append(paths, source.Path)
		}
		for _, path := range paths {
			path = normalizeComposePath(path)
			if path == "" || filepath.IsAbs(path) {
				continue
			}
			matches := source.Type == composeTypeADR && documentType == composeTypeADR
			matches = matches || composePathMatchesType(path+"/placeholder.md", documentType)
			if matches {
				directories = appendUniqueString(directories, path)
			}
		}
	}
	return directories
}

func modalComposeDirectory(candidates []composeCandidate) (string, error) {
	counts := map[string]int{}
	for _, candidate := range candidates {
		counts[normalizeComposePath(filepath.Dir(candidate.Path))]++
	}
	maxCount := 0
	var winners []string
	for directory, count := range counts {
		switch {
		case count > maxCount:
			maxCount = count
			winners = []string{directory}
		case count == maxCount:
			winners = append(winners, directory)
		}
	}
	sort.Strings(winners)
	if len(winners) != 1 {
		return "", fmt.Errorf("repository has conflicting document directories (%s); choose --output explicitly", strings.Join(winners, ", "))
	}
	return winners[0], nil
}

func modalComposeADRFormat(candidates []composeCandidate) (string, string, error) {
	counts := map[string]int{}
	variants := map[string]int{}
	for _, candidate := range candidates {
		if candidate.Format == "" {
			continue
		}
		counts[candidate.Format]++
		if candidate.Format == composeADRFormatMADR && candidate.Variant != "" {
			variants[candidate.Variant]++
		}
	}
	format, err := uniqueModalValue(counts)
	if err != nil {
		return "", "", fmt.Errorf("repository has conflicting ADR formats (%s); choose --format explicitly", err)
	}
	variant := ""
	if format == composeADRFormatMADR {
		variant, err = uniqueModalValue(variants)
		if err != nil {
			return "", "", fmt.Errorf("repository has conflicting MADR variants (%s); choose --variant explicitly", err)
		}
	}
	return format, variant, nil
}

func uniqueModalValue(counts map[string]int) (string, error) {
	maxCount := 0
	var winners []string
	for value, count := range counts {
		switch {
		case count > maxCount:
			maxCount = count
			winners = []string{value}
		case count == maxCount && count > 0:
			winners = append(winners, value)
		}
	}
	sort.Strings(winners)
	if len(winners) > 1 {
		return "", fmt.Errorf("tie: %s", strings.Join(winners, ", "))
	}
	if len(winners) == 0 {
		return "", nil
	}
	return winners[0], nil
}

func composeCandidatesInDirectory(candidates []composeCandidate, directory string) []composeCandidate {
	directory = normalizeComposePath(directory)
	var matches []composeCandidate
	for _, candidate := range candidates {
		if normalizeComposePath(filepath.Dir(candidate.Path)) == directory {
			matches = append(matches, candidate)
		}
	}
	return matches
}

func composeDirectoryHasCandidate(candidates []composeCandidate, directory string) bool {
	return len(composeCandidatesInDirectory(candidates, directory)) > 0
}

func resolveComposeOutputPath(repoRoot, title string, candidates []composeCandidate, convention composeConvention, opts composeOptions) (string, string, error) {
	if strings.TrimSpace(opts.Output) != "" {
		path, err := composeAbsoluteOutputPath(repoRoot, opts.Output)
		if err != nil {
			return "", "", err
		}
		return path, composeRecordIDFromFilename(filepath.Base(path)), nil
	}
	directoryCandidates := composeCandidatesInDirectory(candidates, convention.Directory)
	filename, recordID, err := composeFilename(title, opts.DocumentType, directoryCandidates)
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(repoRoot, filepath.FromSlash(convention.Directory), filename)
	return path, recordID, nil
}

func composeAbsoluteOutputPath(repoRoot, output string) (string, error) {
	path := strings.TrimSpace(output)
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, filepath.FromSlash(path))
	}
	path = filepath.Clean(path)
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("compose output must stay inside repository %s", repoRoot)
	}
	relSlash := strings.ToLower(filepath.ToSlash(rel))
	if strings.HasPrefix(relSlash, "devspecs/tasks/") || strings.HasPrefix(relSlash, ".devspecs/tasks/") {
		return "", fmt.Errorf("compose output must stay outside the DevSpecs task corpus")
	}
	if !strings.EqualFold(filepath.Ext(path), ".md") {
		return "", fmt.Errorf("compose output must be a Markdown .md file")
	}
	return path, nil
}

type composeFilenameConvention struct {
	Prefix string
	Width  int
	Sep    string
	Max    int
}

func composeFilename(title, documentType string, candidates []composeCandidate) (string, string, error) {
	slug := composeSlug(title)
	styles := map[string]*composeFilenameConvention{}
	numberedCount := 0
	for _, candidate := range candidates {
		match := composeNumberedFilenameRE.FindStringSubmatch(filepath.Base(candidate.Path))
		if match == nil {
			continue
		}
		numberedCount++
		number, _ := strconv.Atoi(match[2])
		prefix := ""
		if match[1] != "" {
			prefix = strings.ToLower(match[1]) + "-"
		}
		key := fmt.Sprintf("%s|%d|%s", prefix, len(match[2]), match[3])
		style := styles[key]
		if style == nil {
			style = &composeFilenameConvention{Prefix: prefix, Width: len(match[2]), Sep: match[3]}
			styles[key] = style
		}
		if number > style.Max {
			style.Max = number
		}
	}
	if numberedCount == 0 && len(candidates) > 0 {
		return slug + ".md", "0001", nil
	}
	if numberedCount > 0 && numberedCount <= len(candidates)-numberedCount {
		return "", "", fmt.Errorf("repository mixes numbered and unnumbered %s documents; choose --output explicitly", strings.ToUpper(documentType))
	}
	if numberedCount == 0 {
		if documentType == composeTypePRD {
			return slug + ".md", "0001", nil
		}
		return "0001-" + slug + ".md", "0001", nil
	}
	styleCounts := map[string]int{}
	for _, candidate := range candidates {
		match := composeNumberedFilenameRE.FindStringSubmatch(filepath.Base(candidate.Path))
		if match == nil {
			continue
		}
		prefix := ""
		if match[1] != "" {
			prefix = strings.ToLower(match[1]) + "-"
		}
		key := fmt.Sprintf("%s|%d|%s", prefix, len(match[2]), match[3])
		styleCounts[key]++
	}
	key, err := uniqueModalValue(styleCounts)
	if err != nil {
		return "", "", fmt.Errorf("repository has conflicting filename numbering (%s); choose --output explicitly", err)
	}
	style := styles[key]
	next := style.Max + 1
	recordID := fmt.Sprintf("%0*d", style.Width, next)
	return style.Prefix + recordID + style.Sep + slug + ".md", recordID, nil
}

func composeRecordIDFromFilename(filename string) string {
	match := composeNumberedFilenameRE.FindStringSubmatch(filename)
	if match != nil {
		return match[2]
	}
	return "0001"
}

func composeSlug(title string) string {
	var b strings.Builder
	separator := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if separator && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			separator = false
			continue
		}
		separator = true
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "decision"
	}
	return slug
}

func normalizeComposePath(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "." {
		return ""
	}
	return strings.Trim(path, "/")
}

func writeComposeFile(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create compose directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("refusing to overwrite existing document: %s", path)
		}
		return fmt.Errorf("create composed document: %w", err)
	}
	written := false
	defer func() {
		_ = file.Close()
		if !written {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.WriteString(body); err != nil {
		return fmt.Errorf("write composed document: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close composed document: %w", err)
	}
	written = true
	return nil
}

func captureComposedDocument(cmd *cobra.Command, repoRoot, path, title, documentType string) (string, error) {
	kind := config.KindDecision
	subtype := config.SubtypeADR
	status := "proposed"
	sourceType := "adr"
	switch documentType {
	case composeTypeRFC:
		kind = config.KindDesign
		subtype = ""
		status = "draft"
		sourceType = "markdown"
	case composeTypePRD:
		kind = config.KindRequirements
		subtype = config.SubtypePRD
		status = "draft"
		sourceType = "markdown"
	}
	originalWD, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if err := os.Chdir(repoRoot); err != nil {
		return "", err
	}
	defer func() { _ = os.Chdir(originalWD) }()
	var output bytes.Buffer
	silent := &cobra.Command{}
	silent.SetContext(cmd.Context())
	silent.SetOut(&output)
	silent.SetErr(cmd.ErrOrStderr())
	if err := runCaptureAsSource(silent, path, kind, subtype, title, status, sourceType, true); err != nil {
		return "", fmt.Errorf("capture composed document: %w", err)
	}
	var captured map[string]string
	if err := json.Unmarshal(output.Bytes(), &captured); err != nil {
		return "", fmt.Errorf("decode composed document capture: %w", err)
	}
	if captured["id"] == "" {
		return "", fmt.Errorf("capture composed document returned no artifact ID")
	}
	return captured["id"], nil
}

func writeComposeOutput(cmd *cobra.Command, output composeOutput, asJSON bool) error {
	if asJSON {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created %s draft: %s\n", strings.ToUpper(output.DocumentType), output.Path)
	if output.DocumentType == composeTypeADR {
		format := output.Format
		if output.Variant != "" {
			format += " (" + output.Variant + ")"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Format: %s\n", format)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Convention: %s\n", output.ConventionSource)
	fmt.Fprintf(cmd.OutOrStdout(), "Artifact: %s\n", output.ArtifactID)
	fmt.Fprintln(cmd.OutOrStdout(), "Complete the draft before recording it at task-track closeout.")
	return nil
}
