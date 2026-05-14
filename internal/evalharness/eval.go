package evalharness

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/adr"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/openspec"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/scan"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"gopkg.in/yaml.v3"
)

const (
	CorpusSourceFilesystemFixture = "filesystem_fixture"
	CorpusSourceSQLiteIndex       = "sqlite_index"
	ProductPathLabOnly            = "lab_only"
	ProductPathIndexedHarness     = "indexed_harness"
)

type TokenCounter interface {
	Count(text string) int
	Name() string
}

type PricingProfile struct {
	Name              string  `json:"name"`
	InputUSDPer1MTok  float64 `json:"input_usd_per_1m_tokens,omitempty"`
	OutputUSDPer1MTok float64 `json:"output_usd_per_1m_tokens,omitempty"`
}

type TokenizerProfile struct {
	Name          string         `json:"name"`
	Provider      string         `json:"provider"`
	Model         string         `json:"model,omitempty"`
	Approximation string         `json:"approximation,omitempty"`
	Pricing       PricingProfile `json:"pricing,omitempty"`
}

type ProfiledTokenCounter interface {
	TokenCounter
	Profile() TokenizerProfile
}

type ApproxTokenCounter struct{}

func (ApproxTokenCounter) Count(text string) int {
	if text == "" {
		return 0
	}
	return int(math.Ceil(float64(len(text)) / 4.0))
}

func (ApproxTokenCounter) Name() string { return "approx_chars_div_4" }

func (ApproxTokenCounter) Profile() TokenizerProfile {
	return TokenizerProfile{
		Name:          "approx_chars_div_4",
		Provider:      "deterministic",
		Approximation: "ceil(chars / 4.0)",
		Pricing: PricingProfile{
			Name: "none",
		},
	}
}

type Options struct {
	JSON             bool
	MinRecall        *float64
	MinMeanRecall    *float64
	MinMustRecall    *float64
	MinSufficiency   *float64
	MinReductionFull *float64
	CorpusSource     string
	TokenCounter     TokenCounter
	Retriever        retrieval.Retriever
}

type CaseFile struct {
	FixtureVersion string     `yaml:"fixture_version"`
	EvalStage      string     `yaml:"eval_stage"`
	Cases          []CaseSpec `yaml:"cases"`
}

type CaseSpec struct {
	ID               string             `yaml:"id" json:"id"`
	Query            string             `yaml:"query" json:"query"`
	ExpectedRelevant []ExpectedArtifact `yaml:"expected_relevant" json:"expected_relevant"`
	ExpectedExcluded []string           `yaml:"expected_excluded" json:"expected_excluded"`
	ExpectedStatus   map[string]string  `yaml:"expected_status" json:"expected_status,omitempty"`
	SuccessCriteria  SuccessCriteria    `yaml:"success_criteria" json:"success_criteria,omitempty"`
}

type ExpectedArtifact struct {
	Path       string `yaml:"path" json:"path"`
	Importance string `yaml:"importance" json:"importance"`
}

func (a *ExpectedArtifact) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		a.Path = filepath.ToSlash(strings.TrimSpace(value.Value))
		a.Importance = "must"
		return nil
	case yaml.MappingNode:
		var aux struct {
			Path       string `yaml:"path"`
			Importance string `yaml:"importance"`
		}
		if err := value.Decode(&aux); err != nil {
			return err
		}
		a.Path = filepath.ToSlash(strings.TrimSpace(aux.Path))
		a.Importance = defaultString(aux.Importance, "must")
		return nil
	default:
		return fmt.Errorf("expected_relevant entry must be a path string or mapping")
	}
}

type SuccessCriteria struct {
	MustContainTerms        []string `yaml:"must_contain_terms" json:"must_contain_terms,omitempty"`
	MustContainArtifacts    []string `yaml:"must_contain_artifacts" json:"must_contain_artifacts,omitempty"`
	MustNotContainTerms     []string `yaml:"must_not_contain_terms" json:"must_not_contain_terms,omitempty"`
	MustNotContainArtifacts []string `yaml:"must_not_contain_artifacts" json:"must_not_contain_artifacts,omitempty"`
	LegacyMustNotContain    []string `yaml:"must_not_contain" json:"must_not_contain,omitempty"`
}

func (c SuccessCriteria) Configured() bool {
	return len(c.MustContainTerms) > 0 ||
		len(c.MustContainArtifacts) > 0 ||
		len(c.MustNotContainTerms) > 0 ||
		len(c.MustNotContainArtifacts) > 0 ||
		len(c.LegacyMustNotContain) > 0
}

type BaselineMetrics struct {
	Name                     string   `json:"name"`
	FileScope                string   `json:"file_scope"`
	IncludesSourceCandidates bool     `json:"includes_source_candidates"`
	Tokens                   int      `json:"tokens"`
	ArtifactCount            int      `json:"artifact_count"`
	Artifacts                []string `json:"artifacts"`
	RelevantIncluded         int      `json:"relevant_included"`
	IrrelevantCount          int      `json:"irrelevant_count"`
}

type CaseResult struct {
	ID                            string            `json:"id"`
	Query                         string            `json:"query"`
	DevSpecsTokens                int               `json:"devspecs_tokens"`
	FullPlanningTokens            int               `json:"full_planning_tokens"`
	AllMarkdownTokens             int               `json:"all_markdown_tokens"`
	FullCandidateCorpusTokens     int               `json:"full_candidate_corpus_tokens"`
	QueryFileBaselineTokens       int               `json:"query_file_baseline_tokens"`
	TokenReductionVsFullPlanning  float64           `json:"token_reduction_vs_full_planning"`
	TokenReductionVsAllMarkdown   float64           `json:"token_reduction_vs_all_markdown"`
	TokenReductionVsFullCandidate float64           `json:"token_reduction_vs_full_candidate_corpus"`
	TokenReductionVsQueryFile     float64           `json:"token_reduction_vs_query_file_baseline"`
	ExpectedRelevantCount         int               `json:"expected_relevant_count"`
	RelevantRetrieved             int               `json:"relevant_retrieved"`
	ArtifactRecall                float64           `json:"artifact_recall"`
	MustExpectedCount             int               `json:"must_expected_count"`
	MustRelevantRetrieved         int               `json:"must_relevant_retrieved"`
	MustHaveRecall                float64           `json:"must_have_recall"`
	HelpfulExpectedCount          int               `json:"helpful_expected_count"`
	HelpfulRelevantRetrieved      int               `json:"helpful_relevant_retrieved"`
	HelpfulRecall                 float64           `json:"helpful_recall"`
	BackgroundExpectedCount       int               `json:"background_expected_count"`
	BackgroundRelevantRetrieved   int               `json:"background_relevant_retrieved"`
	BackgroundRecall              float64           `json:"background_recall"`
	ArtifactsIncluded             []string          `json:"artifacts_included"`
	ArtifactReasons               []ArtifactReason  `json:"artifact_reasons"`
	RelevantIncluded              []string          `json:"relevant_included"`
	IrrelevantIncluded            []string          `json:"irrelevant_included"`
	ArtifactPrecision             float64           `json:"artifact_precision"`
	MissedExpectedRelevant        []string          `json:"missed_expected_relevant"`
	UnexpectedExcludedHits        []string          `json:"unexpected_excluded_hits"`
	ContextSufficiency            SufficiencyResult `json:"context_sufficiency"`
	Baselines                     []BaselineMetrics `json:"baselines"`
	ThresholdFailures             []string          `json:"threshold_failures,omitempty"`
}

type Summary struct {
	Cases                                   int           `json:"cases"`
	MedianTokenReductionVsFullPlanning      float64       `json:"median_token_reduction_vs_full_planning"`
	MeanTokenReductionVsFullPlanning        float64       `json:"mean_token_reduction_vs_full_planning"`
	MedianTokenReductionVsQueryFileBaseline float64       `json:"median_token_reduction_vs_query_file_baseline"`
	MeanArtifactRecall                      float64       `json:"mean_artifact_recall"`
	MeanMustHaveRecall                      float64       `json:"mean_must_have_recall"`
	MeanHelpfulRecall                       float64       `json:"mean_helpful_recall"`
	MeanBackgroundRecall                    float64       `json:"mean_background_recall"`
	MeanArtifactPrecision                   float64       `json:"mean_artifact_precision"`
	ContextSufficiencyCases                 int           `json:"context_sufficiency_cases"`
	ContextSufficiencyPassed                int           `json:"context_sufficiency_passed"`
	ContextSufficiencyPassRate              float64       `json:"context_sufficiency_pass_rate"`
	Pareto                                  ParetoSummary `json:"pareto"`
	WorstRecallCase                         string        `json:"worst_recall_case"`
	LargestTokenContextCase                 string        `json:"largest_token_context_case"`
	FailedThresholdCount                    int           `json:"failed_threshold_count,omitempty"`
}

type ParetoSummary struct {
	MeanTokenReductionVsFullPlanning float64 `json:"mean_token_reduction_vs_full_planning"`
	MeanArtifactRecall               float64 `json:"mean_artifact_recall"`
	MeanMustHaveRecall               float64 `json:"mean_must_have_recall"`
	MeanArtifactPrecision            float64 `json:"mean_artifact_precision"`
	ContextSufficiencyPassRate       float64 `json:"context_sufficiency_pass_rate"`
}

type ArtifactReason = retrieval.Reason

type SufficiencyResult struct {
	Configured                bool     `json:"configured"`
	Passed                    bool     `json:"passed"`
	MissingTerms              []string `json:"missing_terms"`
	MissingArtifacts          []string `json:"missing_artifacts"`
	ForbiddenTermsPresent     []string `json:"forbidden_terms_present"`
	ForbiddenArtifactsPresent []string `json:"forbidden_artifacts_present"`
	Failures                  []string `json:"failures"`
}

type Result struct {
	Fixture          string           `json:"fixture"`
	FixtureVersion   string           `json:"fixture_version"`
	EvalStage        string           `json:"eval_stage"`
	CorpusSource     string           `json:"corpus_source"`
	ProductPath      string           `json:"product_path"`
	Retriever        string           `json:"retriever"`
	TokenCounter     string           `json:"token_counter"`
	TokenizerProfile TokenizerProfile `json:"tokenizer_profile"`
	PricingProfile   PricingProfile   `json:"pricing_profile"`
	ResultsFile      string           `json:"results_file,omitempty"`
	Corpus           CorpusSummary    `json:"corpus"`
	Summary          Summary          `json:"summary"`
	Cases            []CaseResult     `json:"cases"`
}

type CorpusSummary struct {
	PlanningArtifacts       CorpusSlice `json:"planning_artifacts"`
	MarkdownFiles           CorpusSlice `json:"markdown_files"`
	SourceContextCandidates CorpusSlice `json:"source_context_candidates"`
	FullCandidateCorpus     CorpusSlice `json:"full_candidate_corpus"`
}

type CorpusSlice struct {
	FileScope                string   `json:"file_scope"`
	IncludesSourceCandidates bool     `json:"includes_source_candidates"`
	Files                    int      `json:"files"`
	Tokens                   int      `json:"tokens"`
	Artifacts                []string `json:"artifacts,omitempty"`
}

func Run(fixture string, opts Options) (*Result, error) {
	counter := opts.TokenCounter
	if counter == nil {
		counter = ApproxTokenCounter{}
	}
	retriever := opts.Retriever
	if retriever == nil {
		retriever = retrieval.WeightedFilesRetrieverV0{}
	}
	tokenizerProfile := tokenizerProfile(counter)
	fixtureAbs, err := filepath.Abs(fixture)
	if err != nil {
		return nil, err
	}
	caseFile, err := loadCaseFile(fixtureAbs)
	if err != nil {
		return nil, err
	}
	corpusSource := defaultString(opts.CorpusSource, CorpusSourceSQLiteIndex)
	files, err := collectCorpusFiles(fixtureAbs, corpusSource)
	if err != nil {
		return nil, err
	}

	fullPlanning := fullPlanningCorpus(files)
	allMarkdown := filterFiles(files, func(f File) bool {
		return strings.EqualFold(filepath.Ext(f.Path), ".md")
	})
	sourceCandidates := sourceContextCandidates(files)
	fullCandidate := mergeFiles(fullPlanning, sourceCandidates)

	fullContext := renderContext("full planning corpus", fullPlanning)
	allMarkdownContext := renderContext("all markdown", allMarkdown)
	sourceContext := renderContext("source/context candidates", sourceCandidates)
	fullCandidateContext := renderContext("full candidate corpus", fullCandidate)

	result := &Result{
		Fixture:          filepath.ToSlash(fixture),
		FixtureVersion:   defaultString(caseFile.FixtureVersion, "agentic-saas-fragmented-v0"),
		EvalStage:        defaultString(caseFile.EvalStage, "seed_smoke"),
		CorpusSource:     corpusSource,
		ProductPath:      productPathForCorpusSource(corpusSource),
		Retriever:        retriever.Name(),
		TokenCounter:     counter.Name(),
		TokenizerProfile: tokenizerProfile,
		PricingProfile:   tokenizerProfile.Pricing,
		Corpus: CorpusSummary{
			PlanningArtifacts:       corpusSlice("planning/intent docs only: openspec/**/*.md, docs/**/*.md, .cursor/**/*.md, .claude/**/*.md, plans/**/*.md, scratch/**/*.md", false, fullPlanning, counter.Count(fullContext)),
			MarkdownFiles:           corpusSlice("all *.md files, excluding ignored dirs and cases.yaml", false, allMarkdown, counter.Count(allMarkdownContext)),
			SourceContextCandidates: corpusSlice("non-markdown text/code candidates considered by filesystem retrieval", true, sourceCandidates, counter.Count(sourceContext)),
			FullCandidateCorpus:     corpusSlice("planning/intent docs plus non-markdown source/context candidates", true, fullCandidate, counter.Count(fullCandidateContext)),
		},
	}
	for _, c := range caseFile.Cases {
		devspecsFiles := retriever.Retrieve(files, c.Query)
		queryFiles := retrieval.QueryBaseline(files, c.Query)

		devContext := renderContext(c.Query, devspecsFiles)
		queryContext := renderContext(c.Query, queryFiles)

		cr := CaseResult{
			ID:                        c.ID,
			Query:                     c.Query,
			DevSpecsTokens:            counter.Count(devContext),
			FullPlanningTokens:        counter.Count(fullContext),
			AllMarkdownTokens:         counter.Count(allMarkdownContext),
			FullCandidateCorpusTokens: counter.Count(fullCandidateContext),
			QueryFileBaselineTokens:   counter.Count(queryContext),
			ArtifactsIncluded:         rels(devspecsFiles),
			ArtifactReasons:           retrieval.ExplainCandidates(devspecsFiles, c.Query),
			Baselines: []BaselineMetrics{
				baselineMetrics("full_planning_corpus", "planning/intent docs only: openspec/**/*.md, docs/**/*.md, .cursor/**/*.md, .claude/**/*.md, plans/**/*.md, scratch/**/*.md", false, fullPlanning, expectedPaths(c.ExpectedRelevant), counter.Count(fullContext)),
				baselineMetrics("all_markdown", "all *.md files, excluding ignored dirs and cases.yaml", false, allMarkdown, expectedPaths(c.ExpectedRelevant), counter.Count(allMarkdownContext)),
				baselineMetrics("full_candidate_corpus", "planning/intent docs plus non-markdown source/context candidates", true, fullCandidate, expectedPaths(c.ExpectedRelevant), counter.Count(fullCandidateContext)),
				baselineMetrics("query_file_baseline", "deterministic query term matches across all text/code candidates, including source files", true, queryFiles, expectedPaths(c.ExpectedRelevant), counter.Count(queryContext)),
			},
		}
		cr.TokenReductionVsFullPlanning = tokenReduction(cr.DevSpecsTokens, cr.FullPlanningTokens)
		cr.TokenReductionVsAllMarkdown = tokenReduction(cr.DevSpecsTokens, cr.AllMarkdownTokens)
		cr.TokenReductionVsFullCandidate = tokenReduction(cr.DevSpecsTokens, cr.FullCandidateCorpusTokens)
		cr.TokenReductionVsQueryFile = tokenReduction(cr.DevSpecsTokens, cr.QueryFileBaselineTokens)
		applyArtifactMetrics(&cr, c)
		cr.ContextSufficiency = evaluateSufficiency(c.SuccessCriteria, devContext, cr.ArtifactsIncluded)
		applyThresholds(&cr, opts)
		result.Cases = append(result.Cases, cr)
	}
	result.Summary = summarize(result.Cases)
	result.Summary.FailedThresholdCount += len(CheckSummaryThresholds(result, opts))
	return result, nil
}

func tokenizerProfile(counter TokenCounter) TokenizerProfile {
	if profiled, ok := counter.(ProfiledTokenCounter); ok {
		return profiled.Profile()
	}
	return TokenizerProfile{
		Name:     counter.Name(),
		Provider: "custom",
		Pricing: PricingProfile{
			Name: "none",
		},
	}
}

func loadCaseFile(fixtureAbs string) (*CaseFile, error) {
	data, err := os.ReadFile(filepath.Join(fixtureAbs, "cases.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load cases.yaml: %w", err)
	}
	var cf CaseFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parse cases.yaml: %w", err)
	}
	if len(cf.Cases) == 0 {
		return nil, fmt.Errorf("cases.yaml has no cases")
	}
	for i := range cf.Cases {
		c := &cf.Cases[i]
		if strings.TrimSpace(c.ID) == "" {
			return nil, fmt.Errorf("cases[%d]: id is required", i)
		}
		if strings.TrimSpace(c.Query) == "" {
			return nil, fmt.Errorf("cases[%d]: query is required", i)
		}
		if len(c.ExpectedRelevant) == 0 {
			return nil, fmt.Errorf("cases[%d]: expected_relevant is required", i)
		}
		for j := range c.ExpectedRelevant {
			c.ExpectedRelevant[j].Path = filepath.ToSlash(strings.TrimSpace(c.ExpectedRelevant[j].Path))
			if c.ExpectedRelevant[j].Path == "" {
				return nil, fmt.Errorf("cases[%d].expected_relevant[%d]: path is required", i, j)
			}
			importance, err := normalizeImportance(c.ExpectedRelevant[j].Importance)
			if err != nil {
				return nil, fmt.Errorf("cases[%d].expected_relevant[%d]: %w", i, j, err)
			}
			c.ExpectedRelevant[j].Importance = importance
		}
		for j := range c.ExpectedExcluded {
			c.ExpectedExcluded[j] = filepath.ToSlash(strings.TrimSpace(c.ExpectedExcluded[j]))
		}
		normalizeCriteriaPaths(&c.SuccessCriteria)
	}
	return &cf, nil
}

func normalizeImportance(value string) (string, error) {
	importance := strings.ToLower(strings.TrimSpace(value))
	if importance == "" {
		importance = "must"
	}
	switch importance {
	case "must", "helpful", "background":
		return importance, nil
	default:
		return "", fmt.Errorf("importance must be must, helpful, or background, got %q", value)
	}
}

func normalizeCriteriaPaths(c *SuccessCriteria) {
	for i := range c.MustContainArtifacts {
		c.MustContainArtifacts[i] = filepath.ToSlash(strings.TrimSpace(c.MustContainArtifacts[i]))
	}
	for i := range c.MustNotContainArtifacts {
		c.MustNotContainArtifacts[i] = filepath.ToSlash(strings.TrimSpace(c.MustNotContainArtifacts[i]))
	}
	for i := range c.MustContainTerms {
		c.MustContainTerms[i] = strings.TrimSpace(c.MustContainTerms[i])
	}
	for i := range c.MustNotContainTerms {
		c.MustNotContainTerms[i] = strings.TrimSpace(c.MustNotContainTerms[i])
	}
	for i := range c.LegacyMustNotContain {
		c.LegacyMustNotContain[i] = strings.TrimSpace(c.LegacyMustNotContain[i])
	}
}

type File = retrieval.Candidate

func collectCorpusFiles(root, corpusSource string) ([]File, error) {
	switch corpusSource {
	case "", CorpusSourceFilesystemFixture:
		return collectFiles(root)
	case CorpusSourceSQLiteIndex:
		return collectIndexedFiles(root)
	default:
		return nil, fmt.Errorf("unknown eval corpus source %q", corpusSource)
	}
}

func productPathForCorpusSource(corpusSource string) string {
	switch corpusSource {
	case CorpusSourceSQLiteIndex:
		return ProductPathIndexedHarness
	default:
		return ProductPathLabOnly
	}
}

func collectFiles(root string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if shouldIgnore(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == "cases.yaml" {
			return nil
		}
		if d.IsDir() || !isTextArtifact(rel) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		files = append(files, File{Path: rel, Body: string(data)})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

func collectIndexedFiles(root string) ([]File, error) {
	tempDir, err := os.MkdirTemp("", "devspecs-eval-index-*")
	if err != nil {
		return nil, fmt.Errorf("create indexed eval temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := store.Open(filepath.Join(tempDir, "devspecs.db"))
	if err != nil {
		return nil, fmt.Errorf("open indexed eval database: %w", err)
	}
	defer db.Close()

	cfg, err := config.LoadRepoConfig(root)
	if err != nil {
		return nil, fmt.Errorf("load fixture repo config: %w", err)
	}
	scanner := scan.New(db, idgen.NewFactory(), []adapters.Adapter{
		&openspec.Adapter{},
		&adr.Adapter{},
		&markdown.Adapter{},
	})
	if _, err := scanner.Run(context.Background(), root, cfg); err != nil {
		return nil, fmt.Errorf("scan fixture into indexed eval database: %w", err)
	}

	artifacts, err := db.ListArtifacts(store.FilterParams{RepoRoot: root})
	if err != nil {
		return nil, fmt.Errorf("list indexed eval artifacts: %w", err)
	}
	seen := map[string]bool{}
	var files []File
	for _, art := range artifacts {
		sources, _ := db.GetSourcesForArtifact(art.ID)
		rel := indexedArtifactRel(art, sources)
		if rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true

		var body string
		if art.CurrentRevID != "" {
			if rev, err := db.GetRevision(art.CurrentRevID); err == nil && rev != nil {
				body = rev.Body
			}
		}
		todos, _ := db.GetTodosForArtifact(art.ID)
		files = append(files, File{
			ID:      art.ID,
			Path:    filepath.ToSlash(rel),
			Kind:    art.Kind,
			Subtype: art.Subtype,
			Title:   art.Title,
			Status:  art.Status,
			Body:    renderIndexedArtifactContent(art, sources, todos, body),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func indexedArtifactRel(art store.ArtifactRow, sources []store.SourceRow) string {
	for _, src := range sources {
		if strings.TrimSpace(src.Path) != "" {
			return filepath.ToSlash(src.Path)
		}
	}
	if art.Title != "" {
		return strings.TrimSpace(art.Title)
	}
	return art.ID
}

func renderIndexedArtifactContent(art store.ArtifactRow, sources []store.SourceRow, todos []store.TodoRow, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", art.Title)
	fmt.Fprintf(&b, "Kind: %s\n", art.Kind)
	if art.Subtype != "" {
		fmt.Fprintf(&b, "Subtype: %s\n", art.Subtype)
	}
	fmt.Fprintf(&b, "Status: %s\n", art.Status)
	for _, src := range sources {
		if src.Path != "" {
			fmt.Fprintf(&b, "Source: %s\n", filepath.ToSlash(src.Path))
		}
		if src.FormatProfile != "" {
			fmt.Fprintf(&b, "Format profile: %s\n", src.FormatProfile)
		}
		if src.LayoutGroup != "" {
			fmt.Fprintf(&b, "Layout group: %s\n", src.LayoutGroup)
		}
	}
	if len(todos) > 0 {
		fmt.Fprintln(&b, "\nTasks:")
		for _, td := range todos {
			marker := "[ ]"
			if td.Done {
				marker = "[x]"
			}
			fmt.Fprintf(&b, "- %s %s\n", marker, td.Text)
		}
	}
	if strings.TrimSpace(body) != "" {
		fmt.Fprintf(&b, "\n%s", strings.TrimRight(body, "\r\n"))
	}
	return b.String()
}

func shouldIgnore(rel string, isDir bool) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	ignoredDirs := map[string]bool{
		".git": true, "node_modules": true, "dist": true, "build": true,
		".next": true, "coverage": true, "tmp": true, "vendor": true,
	}
	if isDir {
		if ignoredDirs[parts[len(parts)-1]] {
			return true
		}
	}
	base := strings.ToLower(filepath.Base(rel))
	return strings.HasSuffix(base, ".lock") || base == "package-lock.json" || base == "pnpm-lock.yaml" || base == "yarn.lock"
}

func isTextArtifact(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".md", ".mdx", ".ts", ".tsx", ".js", ".jsx", ".sql", ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}

func fullPlanningCorpus(files []File) []File {
	return filterFiles(files, func(f File) bool {
		rel := f.Path
		if !strings.EqualFold(filepath.Ext(rel), ".md") {
			return false
		}
		for _, prefix := range []string{"openspec/", "docs/", ".cursor/", ".claude/", "plans/", "scratch/"} {
			if strings.HasPrefix(rel, prefix) {
				return true
			}
		}
		return false
	})
}

func sourceContextCandidates(files []File) []File {
	return filterFiles(files, func(f File) bool {
		if strings.EqualFold(filepath.Ext(f.Path), ".md") {
			return false
		}
		if retrieval.IsPlanningIntentPath(f.Path) {
			return false
		}
		return true
	})
}

func mergeFiles(groups ...[]File) []File {
	seen := map[string]bool{}
	var out []File
	for _, group := range groups {
		for _, f := range group {
			if seen[f.Path] {
				continue
			}
			seen[f.Path] = true
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func filterFiles(files []File, keep func(File) bool) []File {
	var out []File
	for _, f := range files {
		if keep(f) {
			out = append(out, f)
		}
	}
	return out
}

func corpusSlice(scope string, includesSource bool, files []File, tokens int) CorpusSlice {
	return CorpusSlice{
		FileScope:                scope,
		IncludesSourceCandidates: includesSource,
		Files:                    len(files),
		Tokens:                   tokens,
		Artifacts:                rels(files),
	}
}

func renderContext(label string, files []File) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# DevSpecs Eval Context\n\nQuery: %s\n\n", label)
	for _, f := range files {
		fmt.Fprintf(&b, "## %s\n\n```text\n%s\n```\n\n", f.Path, strings.TrimRight(f.Body, "\r\n"))
	}
	return b.String()
}

func rels(files []File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

func expectedPaths(items []ExpectedArtifact) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, filepath.ToSlash(item.Path))
	}
	return out
}

func expectedImportanceSet(items []ExpectedArtifact) map[string]string {
	out := make(map[string]string, len(items))
	for _, item := range items {
		importance, err := normalizeImportance(item.Importance)
		if err != nil {
			importance = "must"
		}
		out[filepath.ToSlash(item.Path)] = importance
	}
	return out
}

func evaluateSufficiency(criteria SuccessCriteria, context string, included []string) SufficiencyResult {
	result := SufficiencyResult{
		Configured:                criteria.Configured(),
		MissingTerms:              []string{},
		MissingArtifacts:          []string{},
		ForbiddenTermsPresent:     []string{},
		ForbiddenArtifactsPresent: []string{},
		Failures:                  []string{},
	}
	if !result.Configured {
		return result
	}
	contextLower := strings.ToLower(context)
	includedSet := stringSet(included)
	for _, term := range criteria.MustContainTerms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if !strings.Contains(contextLower, strings.ToLower(term)) {
			result.MissingTerms = append(result.MissingTerms, term)
		}
	}
	for _, artifact := range criteria.MustContainArtifacts {
		artifact = filepath.ToSlash(strings.TrimSpace(artifact))
		if artifact == "" {
			continue
		}
		if !includedSet[artifact] {
			result.MissingArtifacts = append(result.MissingArtifacts, artifact)
		}
	}
	for _, term := range append([]string{}, criteria.MustNotContainTerms...) {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if strings.Contains(contextLower, strings.ToLower(term)) {
			result.ForbiddenTermsPresent = append(result.ForbiddenTermsPresent, term)
		}
	}
	for _, term := range criteria.LegacyMustNotContain {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if strings.Contains(contextLower, strings.ToLower(term)) {
			result.ForbiddenTermsPresent = append(result.ForbiddenTermsPresent, term)
		}
	}
	for _, artifact := range criteria.MustNotContainArtifacts {
		artifact = filepath.ToSlash(strings.TrimSpace(artifact))
		if artifact == "" {
			continue
		}
		if includedSet[artifact] {
			result.ForbiddenArtifactsPresent = append(result.ForbiddenArtifactsPresent, artifact)
		}
	}
	if len(result.MissingTerms) > 0 {
		result.Failures = append(result.Failures, "missing required terms: "+strings.Join(result.MissingTerms, ", "))
	}
	if len(result.MissingArtifacts) > 0 {
		result.Failures = append(result.Failures, "missing required artifacts: "+strings.Join(result.MissingArtifacts, ", "))
	}
	if len(result.ForbiddenTermsPresent) > 0 {
		result.Failures = append(result.Failures, "forbidden terms present: "+strings.Join(result.ForbiddenTermsPresent, ", "))
	}
	if len(result.ForbiddenArtifactsPresent) > 0 {
		result.Failures = append(result.Failures, "forbidden artifacts present: "+strings.Join(result.ForbiddenArtifactsPresent, ", "))
	}
	result.Passed = len(result.Failures) == 0
	return result
}

func baselineMetrics(name, scope string, includesSource bool, files []File, expected []string, tokens int) BaselineMetrics {
	rel := rels(files)
	expectedSet := stringSet(expected)
	relevant := 0
	for _, path := range rel {
		if expectedSet[path] {
			relevant++
		}
	}
	return BaselineMetrics{
		Name:                     name,
		FileScope:                scope,
		IncludesSourceCandidates: includesSource,
		Tokens:                   tokens,
		ArtifactCount:            len(rel),
		Artifacts:                rel,
		RelevantIncluded:         relevant,
		IrrelevantCount:          len(rel) - relevant,
	}
}

func applyArtifactMetrics(cr *CaseResult, spec CaseSpec) {
	expected := expectedImportanceSet(spec.ExpectedRelevant)
	excluded := stringSet(spec.ExpectedExcluded)
	included := stringSet(cr.ArtifactsIncluded)

	cr.RelevantIncluded = []string{}
	cr.IrrelevantIncluded = []string{}
	cr.MissedExpectedRelevant = []string{}
	cr.UnexpectedExcludedHits = []string{}
	cr.ExpectedRelevantCount = len(spec.ExpectedRelevant)
	for _, artifact := range spec.ExpectedRelevant {
		switch artifact.Importance {
		case "must":
			cr.MustExpectedCount++
		case "helpful":
			cr.HelpfulExpectedCount++
		case "background":
			cr.BackgroundExpectedCount++
		}
	}
	for _, rel := range cr.ArtifactsIncluded {
		if importance, ok := expected[rel]; ok {
			cr.RelevantIncluded = append(cr.RelevantIncluded, rel)
			switch importance {
			case "must":
				cr.MustRelevantRetrieved++
			case "helpful":
				cr.HelpfulRelevantRetrieved++
			case "background":
				cr.BackgroundRelevantRetrieved++
			}
		} else {
			cr.IrrelevantIncluded = append(cr.IrrelevantIncluded, rel)
		}
		if excluded[rel] {
			cr.UnexpectedExcludedHits = append(cr.UnexpectedExcludedHits, rel)
		}
	}
	for _, artifact := range spec.ExpectedRelevant {
		if !included[artifact.Path] {
			cr.MissedExpectedRelevant = append(cr.MissedExpectedRelevant, artifact.Path)
		}
	}
	cr.RelevantRetrieved = len(cr.RelevantIncluded)
	if cr.ExpectedRelevantCount > 0 {
		cr.ArtifactRecall = float64(cr.RelevantRetrieved) / float64(cr.ExpectedRelevantCount)
	}
	if cr.MustExpectedCount > 0 {
		cr.MustHaveRecall = float64(cr.MustRelevantRetrieved) / float64(cr.MustExpectedCount)
	}
	if cr.HelpfulExpectedCount > 0 {
		cr.HelpfulRecall = float64(cr.HelpfulRelevantRetrieved) / float64(cr.HelpfulExpectedCount)
	}
	if cr.BackgroundExpectedCount > 0 {
		cr.BackgroundRecall = float64(cr.BackgroundRelevantRetrieved) / float64(cr.BackgroundExpectedCount)
	}
	if len(cr.ArtifactsIncluded) > 0 {
		cr.ArtifactPrecision = float64(len(cr.RelevantIncluded)) / float64(len(cr.ArtifactsIncluded))
	}
}

func applyThresholds(cr *CaseResult, opts Options) {
	if opts.MinRecall != nil && cr.ArtifactRecall < *opts.MinRecall {
		cr.ThresholdFailures = append(cr.ThresholdFailures,
			fmt.Sprintf("recall %.1f%% below minimum %.1f%%", cr.ArtifactRecall*100, *opts.MinRecall*100))
	}
	if opts.MinReductionFull != nil && cr.TokenReductionVsFullPlanning < *opts.MinReductionFull {
		cr.ThresholdFailures = append(cr.ThresholdFailures,
			fmt.Sprintf("token reduction vs full planning %.1f%% below minimum %.1f%%", cr.TokenReductionVsFullPlanning*100, *opts.MinReductionFull*100))
	}
	if opts.MinMustRecall != nil && cr.MustExpectedCount > 0 && cr.MustHaveRecall < *opts.MinMustRecall {
		cr.ThresholdFailures = append(cr.ThresholdFailures,
			fmt.Sprintf("must-have recall %.1f%% below minimum %.1f%%", cr.MustHaveRecall*100, *opts.MinMustRecall*100))
	}
}

func summarize(cases []CaseResult) Summary {
	s := Summary{Cases: len(cases)}
	if len(cases) == 0 {
		return s
	}
	var reductionsFull, reductionsQueryFile []float64
	worstRecall := math.MaxFloat64
	largestTokens := -1
	mustCases := 0
	helpfulCases := 0
	backgroundCases := 0
	for _, c := range cases {
		reductionsFull = append(reductionsFull, c.TokenReductionVsFullPlanning)
		reductionsQueryFile = append(reductionsQueryFile, c.TokenReductionVsQueryFile)
		s.MeanTokenReductionVsFullPlanning += c.TokenReductionVsFullPlanning
		s.MeanArtifactRecall += c.ArtifactRecall
		if c.MustExpectedCount > 0 {
			s.MeanMustHaveRecall += c.MustHaveRecall
			mustCases++
		}
		if c.HelpfulExpectedCount > 0 {
			s.MeanHelpfulRecall += c.HelpfulRecall
			helpfulCases++
		}
		if c.BackgroundExpectedCount > 0 {
			s.MeanBackgroundRecall += c.BackgroundRecall
			backgroundCases++
		}
		s.MeanArtifactPrecision += c.ArtifactPrecision
		s.FailedThresholdCount += len(c.ThresholdFailures)
		if c.ContextSufficiency.Configured {
			s.ContextSufficiencyCases++
			if c.ContextSufficiency.Passed {
				s.ContextSufficiencyPassed++
			}
		}
		if c.ArtifactRecall < worstRecall {
			worstRecall = c.ArtifactRecall
			s.WorstRecallCase = c.ID
		}
		if c.DevSpecsTokens > largestTokens {
			largestTokens = c.DevSpecsTokens
			s.LargestTokenContextCase = c.ID
		}
	}
	n := float64(len(cases))
	s.MeanTokenReductionVsFullPlanning /= n
	s.MeanArtifactRecall /= n
	if mustCases > 0 {
		s.MeanMustHaveRecall /= float64(mustCases)
	}
	if helpfulCases > 0 {
		s.MeanHelpfulRecall /= float64(helpfulCases)
	}
	if backgroundCases > 0 {
		s.MeanBackgroundRecall /= float64(backgroundCases)
	}
	s.MeanArtifactPrecision /= n
	if s.ContextSufficiencyCases > 0 {
		s.ContextSufficiencyPassRate = float64(s.ContextSufficiencyPassed) / float64(s.ContextSufficiencyCases)
	}
	s.MedianTokenReductionVsFullPlanning = median(reductionsFull)
	s.MedianTokenReductionVsQueryFileBaseline = median(reductionsQueryFile)
	s.Pareto = ParetoSummary{
		MeanTokenReductionVsFullPlanning: s.MeanTokenReductionVsFullPlanning,
		MeanArtifactRecall:               s.MeanArtifactRecall,
		MeanMustHaveRecall:               s.MeanMustHaveRecall,
		MeanArtifactPrecision:            s.MeanArtifactPrecision,
		ContextSufficiencyPassRate:       s.ContextSufficiencyPassRate,
	}
	return s
}

func CheckSummaryThresholds(r *Result, opts Options) []string {
	var failures []string
	if opts.MinMeanRecall != nil && r.Summary.MeanArtifactRecall < *opts.MinMeanRecall {
		failures = append(failures,
			fmt.Sprintf("mean recall %.1f%% below minimum %.1f%%", r.Summary.MeanArtifactRecall*100, *opts.MinMeanRecall*100))
	}
	if opts.MinSufficiency != nil && r.Summary.ContextSufficiencyPassRate < *opts.MinSufficiency {
		failures = append(failures,
			fmt.Sprintf("context sufficiency pass rate %.1f%% below minimum %.1f%%", r.Summary.ContextSufficiencyPassRate*100, *opts.MinSufficiency*100))
	}
	return failures
}

func tokenReduction(devspecs, baseline int) float64 {
	if baseline <= 0 {
		return 0
	}
	return 1.0 - (float64(devspecs) / float64(baseline))
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}

func stringSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[filepath.ToSlash(item)] = true
	}
	return out
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func FormatText(r *Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "DevSpecs Eval: %s\n\n", r.Fixture)
	fmt.Fprintf(&b, "Cases: %d\n", r.Summary.Cases)
	fmt.Fprintf(&b, "Fixture version: %s\n", r.FixtureVersion)
	fmt.Fprintf(&b, "Eval stage: %s\n", r.EvalStage)
	fmt.Fprintf(&b, "Corpus source: %s\n", r.CorpusSource)
	fmt.Fprintf(&b, "Product path: %s\n", r.ProductPath)
	fmt.Fprintf(&b, "Retriever: %s\n", r.Retriever)
	fmt.Fprintf(&b, "Token counter: %s\n", r.TokenCounter)
	if r.TokenizerProfile.Approximation != "" {
		fmt.Fprintf(&b, "Token counter detail: %s\n", r.TokenizerProfile.Approximation)
	}
	if r.PricingProfile.Name != "" {
		fmt.Fprintf(&b, "Pricing profile: %s\n", r.PricingProfile.Name)
	}
	if r.ResultsFile != "" {
		fmt.Fprintf(&b, "Results file: %s\n", r.ResultsFile)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Corpus")
	fmt.Fprintf(&b, "- Planning artifacts: %d files / %s tokens\n", r.Corpus.PlanningArtifacts.Files, comma(r.Corpus.PlanningArtifacts.Tokens))
	fmt.Fprintf(&b, "- Markdown files: %d files / %s tokens\n", r.Corpus.MarkdownFiles.Files, comma(r.Corpus.MarkdownFiles.Tokens))
	fmt.Fprintf(&b, "- Source/context candidates: %d files / %s tokens\n", r.Corpus.SourceContextCandidates.Files, comma(r.Corpus.SourceContextCandidates.Tokens))
	fmt.Fprintf(&b, "- Full candidate corpus: %d files / %s tokens\n\n", r.Corpus.FullCandidateCorpus.Files, comma(r.Corpus.FullCandidateCorpus.Tokens))

	fmt.Fprintln(&b, "Summary")
	fmt.Fprintf(&b, "- Median token reduction vs full planning corpus: %s\n", pct(r.Summary.MedianTokenReductionVsFullPlanning))
	fmt.Fprintf(&b, "- Mean token reduction vs full planning corpus: %s\n", pct(r.Summary.MeanTokenReductionVsFullPlanning))
	fmt.Fprintf(&b, "- Median token reduction vs query file baseline: %s\n", pct(r.Summary.MedianTokenReductionVsQueryFileBaseline))
	fmt.Fprintf(&b, "- Mean artifact recall: %s\n", pct(r.Summary.MeanArtifactRecall))
	fmt.Fprintf(&b, "- Mean must-have recall: %s\n", pct(r.Summary.MeanMustHaveRecall))
	fmt.Fprintf(&b, "- Mean helpful recall: %s\n", pct(r.Summary.MeanHelpfulRecall))
	fmt.Fprintf(&b, "- Mean background recall: %s\n", pct(r.Summary.MeanBackgroundRecall))
	fmt.Fprintf(&b, "- Mean artifact precision: %s\n", pct(r.Summary.MeanArtifactPrecision))
	if r.Summary.ContextSufficiencyCases > 0 {
		fmt.Fprintf(&b, "- Context sufficiency pass rate: %d/%d = %s\n", r.Summary.ContextSufficiencyPassed, r.Summary.ContextSufficiencyCases, pct(r.Summary.ContextSufficiencyPassRate))
	}
	fmt.Fprintf(&b, "- Pareto: reduction %s / recall %s / must-have recall %s / precision %s / sufficiency %s\n",
		pct(r.Summary.Pareto.MeanTokenReductionVsFullPlanning),
		pct(r.Summary.Pareto.MeanArtifactRecall),
		pct(r.Summary.Pareto.MeanMustHaveRecall),
		pct(r.Summary.Pareto.MeanArtifactPrecision),
		pct(r.Summary.Pareto.ContextSufficiencyPassRate))
	if r.Summary.FailedThresholdCount > 0 {
		fmt.Fprintf(&b, "- Failed thresholds: %d\n", r.Summary.FailedThresholdCount)
	}
	fmt.Fprintln(&b)

	for _, c := range r.Cases {
		fmt.Fprintf(&b, "Case: %s\n", c.ID)
		fmt.Fprintf(&b, "- DevSpecs context: %s tokens\n", comma(c.DevSpecsTokens))
		fmt.Fprintf(&b, "- Full planning corpus: %s tokens\n", comma(c.FullPlanningTokens))
		fmt.Fprintf(&b, "- All markdown: %s tokens\n", comma(c.AllMarkdownTokens))
		fmt.Fprintf(&b, "- Full candidate corpus: %s tokens\n", comma(c.FullCandidateCorpusTokens))
		fmt.Fprintf(&b, "- Query file baseline: %s tokens\n", comma(c.QueryFileBaselineTokens))
		fmt.Fprintf(&b, "- Reduction vs full planning corpus: %s\n", pct(c.TokenReductionVsFullPlanning))
		fmt.Fprintf(&b, "- Reduction vs all markdown: %s\n", pct(c.TokenReductionVsAllMarkdown))
		fmt.Fprintf(&b, "- Reduction vs full candidate corpus: %s\n", pct(c.TokenReductionVsFullCandidate))
		fmt.Fprintf(&b, "- Reduction vs query file baseline: %s\n", pct(c.TokenReductionVsQueryFile))
		fmt.Fprintf(&b, "- Recall: %d/%d = %s\n", c.RelevantRetrieved, c.ExpectedRelevantCount, pct(c.ArtifactRecall))
		fmt.Fprintf(&b, "- Must-have recall: %s\n", recallText(c.MustRelevantRetrieved, c.MustExpectedCount, c.MustHaveRecall))
		fmt.Fprintf(&b, "- Helpful recall: %s\n", recallText(c.HelpfulRelevantRetrieved, c.HelpfulExpectedCount, c.HelpfulRecall))
		fmt.Fprintf(&b, "- Background recall: %s\n", recallText(c.BackgroundRelevantRetrieved, c.BackgroundExpectedCount, c.BackgroundRecall))
		fmt.Fprintf(&b, "- Precision: %d/%d = %s\n", len(c.RelevantIncluded), len(c.ArtifactsIncluded), pct(c.ArtifactPrecision))
		if c.ContextSufficiency.Configured {
			fmt.Fprintf(&b, "- Sufficiency: %s\n", passFail(c.ContextSufficiency.Passed))
			if len(c.ContextSufficiency.Failures) > 0 {
				fmt.Fprintf(&b, "- Sufficiency failures: %s\n", strings.Join(c.ContextSufficiency.Failures, "; "))
			}
		} else {
			fmt.Fprintf(&b, "- Sufficiency: not configured\n")
		}
		fmt.Fprintf(&b, "- Artifacts included: %s\n", listOrNone(c.ArtifactsIncluded))
		fmt.Fprintf(&b, "- Relevant included: %s\n", listOrNone(c.RelevantIncluded))
		fmt.Fprintf(&b, "- Irrelevant included: %s\n", listOrNone(c.IrrelevantIncluded))
		fmt.Fprintf(&b, "- Missed: %s\n", listOrNone(c.MissedExpectedRelevant))
		fmt.Fprintf(&b, "- Unexpected excluded hits: %s\n", listOrNone(c.UnexpectedExcludedHits))
		if len(c.ThresholdFailures) > 0 {
			fmt.Fprintf(&b, "- Threshold failures: %s\n", strings.Join(c.ThresholdFailures, "; "))
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

func FormatJSON(r *Result) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func pct(v float64) string {
	return fmt.Sprintf("%.1f%%", v*100)
}

func recallText(retrieved, total int, recall float64) string {
	if total == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d = %s", retrieved, total, pct(recall))
}

func passFail(passed bool) string {
	if passed {
		return "pass"
	}
	return "fail"
}

func comma(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ",")
}

func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
