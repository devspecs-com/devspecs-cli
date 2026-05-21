// Package openspec implements the OpenSpec change proposal adapter.
package openspec

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
)

const (
	scopeCollection = "collection"
	scopeBundle     = "bundle"
	scopeFile       = "file"

	roleCollection     = "collection"
	roleChangeBundle   = "change_bundle"
	roleProposal       = "proposal"
	roleDesign         = "design"
	roleTasks          = "tasks"
	roleSpecDelta      = "spec_delta"
	roleCapabilitySpec = "capability_spec"
)

// Adapter discovers and parses OpenSpec collections, change bundles, and child files.
type Adapter struct{}

func (a *Adapter) Name() string { return "openspec" }

func (a *Adapter) Discover(ctx context.Context, repoRoot string, cfg *config.RepoConfig) ([]adapters.Candidate, error) {
	var candidates []adapters.Candidate
	for _, basePath := range openSpecBasePaths(ctx, repoRoot, cfg) {
		candidates = append(candidates, discoverBasePath(ctx, repoRoot, basePath)...)
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].RelPath < candidates[j].RelPath })
	return candidates, nil
}

func discoverBasePath(ctx context.Context, repoRoot, basePath string) []adapters.Candidate {
	baseDir := filepath.Join(repoRoot, filepath.FromSlash(basePath))
	if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(basePath, true) {
		return nil
	}
	info, err := os.Stat(baseDir)
	if err != nil || !info.IsDir() {
		return nil
	}

	candidates := []adapters.Candidate{
		{
			PrimaryPath:   baseDir,
			RelPath:       basePath,
			AdapterName:   "openspec",
			FormatProfile: format.ProfileOpenspec,
			LayoutGroup:   basePath,
			ArtifactScope: scopeCollection,
			Role:          roleCollection,
		},
	}
	candidates = append(candidates, discoverCapabilitySpecCandidates(ctx, repoRoot, filepath.Join(baseDir, "specs"))...)

	changesDir := filepath.Join(baseDir, "changes")
	entries, err := os.ReadDir(changesDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if entry.Name() == "archive" {
				archiveDir := filepath.Join(changesDir, entry.Name())
				archiveEntries, err := os.ReadDir(archiveDir)
				if err != nil {
					continue
				}
				for _, archived := range archiveEntries {
					if !archived.IsDir() {
						continue
					}
					changeDir := filepath.Join(archiveDir, archived.Name())
					if bundle := changeBundleCandidate(ctx, repoRoot, changeDir); bundle.PrimaryPath != "" {
						candidates = append(candidates, bundle)
					}
					candidates = append(candidates, discoverChangeCandidates(ctx, repoRoot, changeDir)...)
				}
				continue
			}
			changeDir := filepath.Join(changesDir, entry.Name())
			if bundle := changeBundleCandidate(ctx, repoRoot, changeDir); bundle.PrimaryPath != "" {
				candidates = append(candidates, bundle)
			}
			candidates = append(candidates, discoverChangeCandidates(ctx, repoRoot, changeDir)...)
		}
	}
	return candidates
}

func openSpecBasePaths(ctx context.Context, repoRoot string, cfg *config.RepoConfig) []string {
	seen := map[string]bool{}
	var paths []string
	add := func(path string) {
		path = strings.Trim(strings.TrimPrefix(filepath.ToSlash(path), "./"), "/")
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		paths = append(paths, path)
	}

	if cfg != nil {
		for _, src := range cfg.Sources {
			if src.Type != "openspec" {
				continue
			}
			if src.Path != "" {
				add(src.Path)
			}
			for _, path := range src.Paths {
				add(path)
			}
		}
	}
	if len(paths) == 0 {
		add("openspec")
	}
	for _, path := range discoverOpenSpecBasePaths(ctx, repoRoot) {
		add(path)
	}
	sort.Strings(paths)
	return paths
}

func discoverOpenSpecBasePaths(ctx context.Context, repoRoot string) []string {
	m := ignore.FromContext(ctx)
	var paths []string
	_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}
		rel = strings.Trim(filepath.ToSlash(rel), "/")
		if rel == "." || rel == "" {
			return nil
		}
		if m != nil && m.ShouldSkip(rel, true) {
			return filepath.SkipDir
		}
		if isBuiltinIgnoredDir(d.Name()) {
			return filepath.SkipDir
		}
		if strings.EqualFold(d.Name(), "openspec") {
			paths = append(paths, rel)
			return filepath.SkipDir
		}
		return nil
	})
	sort.Strings(paths)
	return paths
}

func isBuiltinIgnoredDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "node_modules", "dist", "build", ".next", "coverage", "tmp", "vendor":
		return true
	default:
		return false
	}
}

func changeBundleCandidate(ctx context.Context, repoRoot, changeDir string) adapters.Candidate {
	rel, err := filepath.Rel(repoRoot, changeDir)
	if err != nil {
		return adapters.Candidate{}
	}
	rel = filepath.ToSlash(rel)
	if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(rel, true) {
		return adapters.Candidate{}
	}
	return adapters.Candidate{
		PrimaryPath:   changeDir,
		RelPath:       rel,
		AdapterName:   "openspec",
		FormatProfile: format.ProfileOpenspec,
		LayoutGroup:   rel,
		ArtifactScope: scopeBundle,
		Role:          roleChangeBundle,
	}
}

func discoverCapabilitySpecCandidates(ctx context.Context, repoRoot, specsDir string) []adapters.Candidate {
	var out []adapters.Candidate
	_ = filepath.WalkDir(specsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "spec.md" {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(rel, false) {
			return nil
		}
		out = append(out, adapters.Candidate{
			PrimaryPath:   path,
			RelPath:       rel,
			AdapterName:   "openspec",
			FormatProfile: format.ProfileOpenspec,
			LayoutGroup:   filepath.ToSlash(filepath.Dir(rel)),
			ArtifactScope: scopeFile,
			Role:          roleCapabilitySpec,
		})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out
}

func (a *Adapter) Parse(ctx context.Context, c adapters.Candidate) (adapters.Artifact, []adapters.Source, todoparse.ParseResult, error) {
	scope, role := inferCandidateScopeRole(c)
	switch scope {
	case scopeCollection:
		return parseCollection(c)
	case scopeBundle:
		return parseChangeBundle(c)
	default:
		if role == roleCapabilitySpec {
			return parseCapabilitySpec(c)
		}
		return parseChangeChild(c, role)
	}
}

func inferCandidateScopeRole(c adapters.Candidate) (string, string) {
	scope := c.ArtifactScope
	role := c.Role
	if scope != "" && role != "" {
		return scope, role
	}
	if info, err := os.Stat(c.PrimaryPath); err == nil && info.IsDir() {
		rel := filepath.ToSlash(c.RelPath)
		if strings.Contains(rel, "/changes/") || strings.HasSuffix(rel, "/changes") {
			return scopeBundle, roleChangeBundle
		}
		return scopeCollection, roleCollection
	}
	if role == "" {
		role = roleForRelPath(c.RelPath)
	}
	if role == roleSpecDelta && !strings.Contains(filepath.ToSlash(c.RelPath), "/changes/") {
		role = roleCapabilitySpec
	}
	return scopeFile, role
}

func parseCollection(c adapters.Candidate) (adapters.Artifact, []adapters.Source, todoparse.ParseResult, error) {
	basePath := filepath.ToSlash(c.RelPath)
	body := collectionBody(c.PrimaryPath, basePath)
	extracted := openSpecExtracted(scopeCollection, roleCollection, "", "", basePath, basePath, false)
	extracted["openspec_change_count"] = countChangeDirs(filepath.Join(c.PrimaryPath, "changes"))
	extracted["openspec_capability_count"] = len(specRelPaths(c.PrimaryPath, filepath.Join(c.PrimaryPath, "specs")))

	art := adapters.Artifact{
		SourceIdentity: basePath + "|openspec_collection",
		Kind:           config.KindSpec,
		Subtype:        config.SubtypeOpenspecCollection,
		Title:          "OpenSpec Collection",
		Status:         "active",
		PrimaryPath:    c.PrimaryPath,
		Body:           body,
		Extracted:      extracted,
		FormatProfile:  format.ProfileOpenspec,
		LayoutGroup:    basePath,
	}
	src := adapters.Source{
		SourceType:     "openspec",
		Path:           basePath,
		SourceIdentity: art.SourceIdentity,
		FormatProfile:  format.ProfileOpenspec,
		LayoutGroup:    basePath,
	}
	return art, []adapters.Source{src}, todoparse.ParseResult{}, nil
}

func parseChangeBundle(c adapters.Candidate) (adapters.Artifact, []adapters.Source, todoparse.ParseResult, error) {
	changeDir := c.PrimaryPath
	repoRoot := repoRootForCandidate(c)
	changeID := filepath.Base(changeDir)
	layoutGroup := filepath.ToSlash(c.RelPath)
	if layoutGroup == "" {
		relChangeDir, err := filepath.Rel(repoRoot, changeDir)
		if err != nil {
			return adapters.Artifact{}, nil, todoparse.ParseResult{}, err
		}
		layoutGroup = filepath.ToSlash(relChangeDir)
	}
	basePath := basePathForRel(layoutGroup)
	archived := isArchivedChangeRel(layoutGroup)

	children := childFilesForChange(repoRoot, changeDir)
	body, combinedContent := bundleBody(changeID, layoutGroup, archived, children)
	title := bundleTitle(changeID, children)
	status := inferStatus(combinedContent)
	if archived && status == "proposed" {
		status = "archived"
	}

	extracted := openSpecExtracted(scopeBundle, roleChangeBundle, changeID, "", basePath, layoutGroup, archived)
	extracted["openspec_child_count"] = len(children)
	extracted["openspec_child_paths"] = childRelPaths(children)

	art := adapters.Artifact{
		SourceIdentity: layoutGroup + "|openspec_bundle",
		Kind:           config.KindSpec,
		Subtype:        config.SubtypeOpenspecChangeBundle,
		Title:          title,
		Status:         status,
		PrimaryPath:    changeDir,
		Body:           body,
		Extracted:      extracted,
		FormatProfile:  format.ProfileOpenspec,
		LayoutGroup:    layoutGroup,
	}

	sources := []adapters.Source{{
		SourceType:     "openspec",
		Path:           layoutGroup,
		SourceIdentity: art.SourceIdentity,
		FormatProfile:  format.ProfileOpenspec,
		LayoutGroup:    layoutGroup,
	}}
	var pr todoparse.ParseResult
	for _, child := range children {
		sources = append(sources, adapters.Source{
			SourceType:     "openspec",
			Path:           child.rel,
			SourceIdentity: child.rel + "|openspec_bundle_source",
			FormatProfile:  format.ProfileOpenspec,
			LayoutGroup:    layoutGroup,
		})
		appendParseResult(&pr, todoparse.Parse(child.content, child.rel))
	}
	return art, sources, pr, nil
}

func parseCapabilitySpec(c adapters.Candidate) (adapters.Artifact, []adapters.Source, todoparse.ParseResult, error) {
	data, err := os.ReadFile(c.PrimaryPath)
	if err != nil {
		return adapters.Artifact{}, nil, todoparse.ParseResult{}, err
	}
	content := string(data)
	relPath := filepath.ToSlash(c.RelPath)
	layoutGroup := filepath.ToSlash(filepath.Dir(relPath))
	capability := capabilityForSpecRel(relPath)
	title := extractH1(content)
	if title == "" {
		title = humanize(capability) + " Specification"
	}
	basePath := basePathForRel(relPath)
	extracted := openSpecExtracted(scopeFile, roleCapabilitySpec, "", capability, basePath, layoutGroup, false)

	art := adapters.Artifact{
		SourceIdentity: relPath + "|openspec",
		Kind:           config.KindSpec,
		Subtype:        config.SubtypeOpenspecCapabilitySpec,
		Title:          title,
		Status:         inferStatus(content),
		PrimaryPath:    c.PrimaryPath,
		Body:           content,
		Extracted:      extracted,
		FormatProfile:  format.ProfileOpenspec,
		LayoutGroup:    layoutGroup,
	}
	src := adapters.Source{
		SourceType:     "openspec",
		Path:           relPath,
		SourceIdentity: art.SourceIdentity,
		FormatProfile:  format.ProfileOpenspec,
		LayoutGroup:    layoutGroup,
	}
	return art, []adapters.Source{src}, todoparse.Parse(content, relPath), nil
}

func parseChangeChild(c adapters.Candidate, role string) (adapters.Artifact, []adapters.Source, todoparse.ParseResult, error) {
	data, err := os.ReadFile(c.PrimaryPath)
	if err != nil {
		return adapters.Artifact{}, nil, todoparse.ParseResult{}, err
	}
	content := string(data)
	relPath := filepath.ToSlash(c.RelPath)
	if role == "" {
		role = roleForRelPath(relPath)
	}
	changeDir := changeDirForPath(c.PrimaryPath)
	changeID := filepath.Base(changeDir)
	repoRoot := repoRootForCandidate(c)
	layoutGroup := filepath.ToSlash(filepath.Dir(relPath))
	if relChangeDir, err := filepath.Rel(repoRoot, changeDir); err == nil {
		layoutGroup = filepath.ToSlash(relChangeDir)
	}
	archived := isArchivedChangeRel(layoutGroup)
	capability := ""
	if role == roleSpecDelta {
		capability = capabilityForSpecRel(relPath)
	}

	title := extractH1(content)
	if title == "" {
		title = humanize(changeID)
		if role != "" && role != roleProposal {
			title += " " + humanize(role)
		}
	}
	extracted := openSpecExtracted(scopeFile, role, changeID, capability, basePathForRel(layoutGroup), layoutGroup, archived)

	art := adapters.Artifact{
		SourceIdentity: relPath + "|openspec",
		Kind:           config.KindSpec,
		Subtype:        config.SubtypeOpenspecChild,
		Title:          title,
		Status:         inferStatus(content),
		PrimaryPath:    c.PrimaryPath,
		Body:           content,
		Extracted:      extracted,
		FormatProfile:  format.ProfileOpenspec,
		LayoutGroup:    layoutGroup,
	}
	src := adapters.Source{
		SourceType:     "openspec",
		Path:           relPath,
		SourceIdentity: art.SourceIdentity,
		FormatProfile:  format.ProfileOpenspec,
		LayoutGroup:    layoutGroup,
	}

	pr := todoparse.Parse(content, relPath)
	return art, []adapters.Source{src}, pr, nil
}

type childFile struct {
	role    string
	abs     string
	rel     string
	content string
}

func childFilesForChange(repoRoot, changeDir string) []childFile {
	var out []childFile
	add := func(role, absPath string) {
		info, err := os.Stat(absPath)
		if err != nil || info.IsDir() {
			return
		}
		rel, err := filepath.Rel(repoRoot, absPath)
		if err != nil {
			return
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			return
		}
		out = append(out, childFile{
			role:    role,
			abs:     absPath,
			rel:     filepath.ToSlash(rel),
			content: string(data),
		})
	}
	add(roleProposal, filepath.Join(changeDir, "proposal.md"))
	add(roleDesign, filepath.Join(changeDir, "design.md"))
	add(roleTasks, filepath.Join(changeDir, "tasks.md"))

	var specPaths []string
	specsDir := filepath.Join(changeDir, "specs")
	_ = filepath.WalkDir(specsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "spec.md" {
			return nil
		}
		specPaths = append(specPaths, path)
		return nil
	})
	sort.Strings(specPaths)
	for _, specPath := range specPaths {
		add(roleSpecDelta, specPath)
	}
	return out
}

func bundleBody(changeID, layoutGroup string, archived bool, children []childFile) (string, string) {
	var body strings.Builder
	var combined strings.Builder
	fmt.Fprintf(&body, "# OpenSpec Change Bundle: %s\n\n", humanize(changeID))
	fmt.Fprintf(&body, "## Bundle Metadata\n\n")
	fmt.Fprintf(&body, "- Change ID: %s\n", changeID)
	fmt.Fprintf(&body, "- Path: %s\n", filepath.ToSlash(layoutGroup))
	if archived {
		fmt.Fprintf(&body, "- State: archived\n")
	} else {
		fmt.Fprintf(&body, "- State: active\n")
	}
	for _, child := range children {
		fmt.Fprintf(&body, "\n## %s (%s)\n\n", humanize(child.role), child.rel)
		body.WriteString(strings.TrimRight(child.content, "\r\n"))
		body.WriteString("\n")
		combined.WriteString(child.content)
		combined.WriteString("\n")
	}
	return body.String(), combined.String()
}

func bundleTitle(changeID string, children []childFile) string {
	for _, child := range children {
		if child.role != roleProposal {
			continue
		}
		if title := extractH1(child.content); title != "" {
			return title
		}
	}
	return "OpenSpec Change: " + humanize(changeID)
}

func collectionBody(baseDir, basePath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# OpenSpec Collection\n\n")
	fmt.Fprintf(&b, "Path: %s\n\n", filepath.ToSlash(basePath))
	writeCollectionList(&b, "Active Changes", activeChangeIDs(filepath.Join(baseDir, "changes")))
	writeCollectionList(&b, "Archived Changes", archivedChangeIDs(filepath.Join(baseDir, "changes", "archive")))
	writeCollectionList(&b, "Capability Specs", specRelPaths(baseDir, filepath.Join(baseDir, "specs")))
	return b.String()
}

func writeCollectionList(b *strings.Builder, title string, values []string) {
	fmt.Fprintf(b, "## %s\n\n", title)
	if len(values) == 0 {
		fmt.Fprintln(b, "- none")
		return
	}
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", filepath.ToSlash(value))
	}
	fmt.Fprintln(b)
}

func activeChangeIDs(changesDir string) []string {
	entries, err := os.ReadDir(changesDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "archive" {
			out = append(out, entry.Name())
		}
	}
	sort.Strings(out)
	return out
}

func archivedChangeIDs(archiveDir string) []string {
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			out = append(out, entry.Name())
		}
	}
	sort.Strings(out)
	return out
}

func countChangeDirs(changesDir string) int {
	return len(activeChangeIDs(changesDir)) + len(archivedChangeIDs(filepath.Join(changesDir, "archive")))
}

func specRelPaths(baseDir, specsDir string) []string {
	var out []string
	_ = filepath.WalkDir(specsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "spec.md" {
			return nil
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out
}

func discoverChangeCandidates(ctx context.Context, repoRoot, changeDir string) []adapters.Candidate {
	var out []adapters.Candidate
	add := func(absPath string, role string) {
		info, err := os.Stat(absPath)
		if err != nil || info.IsDir() {
			return
		}
		rel, err := filepath.Rel(repoRoot, absPath)
		if err != nil {
			return
		}
		rel = filepath.ToSlash(rel)
		if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(rel, false) {
			return
		}
		layout, _ := filepath.Rel(repoRoot, changeDir)
		out = append(out, adapters.Candidate{
			PrimaryPath:   absPath,
			RelPath:       rel,
			AdapterName:   "openspec",
			FormatProfile: format.ProfileOpenspec,
			LayoutGroup:   filepath.ToSlash(layout),
			ArtifactScope: scopeFile,
			Role:          role,
		})
	}
	for _, spec := range []struct {
		name string
		role string
	}{
		{"proposal.md", roleProposal},
		{"design.md", roleDesign},
		{"tasks.md", roleTasks},
	} {
		add(filepath.Join(changeDir, spec.name), spec.role)
	}
	specsDir := filepath.Join(changeDir, "specs")
	_ = filepath.WalkDir(specsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "spec.md" {
			return nil
		}
		add(path, roleSpecDelta)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out
}

func openSpecExtracted(scope, openSpecRole, changeID, capability, basePath, layoutGroup string, archived bool) map[string]any {
	out := map[string]any{
		"mode":               "intent",
		"role":               "authoritative",
		"artifact_scope":     scope,
		"source_standard":    "openspec",
		"openspec_role":      openSpecRole,
		"openspec_base_path": filepath.ToSlash(basePath),
		"layout_group":       filepath.ToSlash(layoutGroup),
	}
	if changeID != "" {
		out["openspec_change_id"] = changeID
	}
	if capability != "" {
		out["openspec_capability"] = filepath.ToSlash(capability)
	}
	if archived {
		out["openspec_archived"] = true
	}
	return out
}

func appendParseResult(dst *todoparse.ParseResult, src todoparse.ParseResult) {
	todoOffset := len(dst.Todos)
	for i := range src.Todos {
		src.Todos[i].Ordinal = todoOffset + i
	}
	dst.Todos = append(dst.Todos, src.Todos...)

	criteriaOffset := len(dst.Criteria)
	for i := range src.Criteria {
		src.Criteria[i].Ordinal = criteriaOffset + i
	}
	dst.Criteria = append(dst.Criteria, src.Criteria...)
}

func childRelPaths(children []childFile) []string {
	out := make([]string, 0, len(children))
	for _, child := range children {
		out = append(out, child.rel)
	}
	return out
}

func changeDirForPath(path string) string {
	dir := filepath.Dir(path)
	if filepath.Base(path) == "spec.md" {
		for {
			if filepath.Base(filepath.Dir(dir)) == "specs" {
				return filepath.Dir(filepath.Dir(dir))
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return filepath.Dir(path)
			}
			dir = parent
		}
	}
	return dir
}

func repoRootForChangeDir(changeDir string) string {
	dir := filepath.Clean(changeDir)
	for {
		if filepath.Base(dir) == "changes" {
			return filepath.Dir(filepath.Dir(dir))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(filepath.Dir(filepath.Dir(changeDir)))
		}
		dir = parent
	}
}

func repoRootForCandidate(c adapters.Candidate) string {
	rel := strings.Trim(filepath.ToSlash(c.RelPath), "/")
	if rel == "" {
		return repoRootForChangeDir(c.PrimaryPath)
	}
	root := filepath.Clean(c.PrimaryPath)
	for range strings.Split(rel, "/") {
		root = filepath.Dir(root)
	}
	return root
}

func roleForRelPath(rel string) string {
	base := filepath.Base(filepath.ToSlash(rel))
	switch base {
	case "proposal.md":
		return roleProposal
	case "design.md":
		return roleDesign
	case "tasks.md":
		return roleTasks
	case "spec.md":
		return roleSpecDelta
	default:
		return ""
	}
}

func basePathForRel(rel string) string {
	parts := strings.Split(strings.Trim(filepath.ToSlash(rel), "/"), "/")
	for i, part := range parts {
		if (part == "changes" || part == "specs") && i > 0 {
			return strings.Join(parts[:i], "/")
		}
	}
	if len(parts) > 0 {
		return parts[0]
	}
	return "openspec"
}

func isArchivedChangeRel(rel string) bool {
	return strings.Contains(filepath.ToSlash(rel), "/changes/archive/")
}

func capabilityForSpecRel(rel string) string {
	rel = filepath.ToSlash(rel)
	idx := strings.Index(rel, "/specs/")
	if idx < 0 {
		return ""
	}
	specRel := strings.TrimPrefix(rel[idx+len("/specs/"):], "/")
	specRel = strings.TrimSuffix(specRel, "/spec.md")
	specRel = strings.TrimSuffix(specRel, "spec.md")
	return strings.Trim(specRel, "/")
}

func extractH1(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
	}
	return ""
}

func humanize(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func inferStatus(content string) string {
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "status: accepted") || strings.Contains(lower, "status: approved"):
		return "approved"
	case strings.Contains(lower, "status: rejected"):
		return "rejected"
	case strings.Contains(lower, "status: implementing"):
		return "implementing"
	case strings.Contains(lower, "status: implemented"):
		return "implemented"
	default:
		return "proposed"
	}
}
