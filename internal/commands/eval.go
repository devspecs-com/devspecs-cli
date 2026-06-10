package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/devspecs-com/devspecs-cli/internal/classify"
	"github.com/devspecs-com/devspecs-cli/internal/evalharness"
	"github.com/devspecs-com/devspecs-cli/internal/indexquery"
	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/spf13/cobra"
)

const defaultEvalResultsDir = ".devspecs/eval-runs"

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

// NewEvalCmd creates the ds eval command.
func NewEvalCmd() *cobra.Command {
	var (
		asJSON                          bool
		minRecall                       float64
		minMeanRecall                   float64
		minMustRecall                   float64
		minSufficiency                  float64
		minReductionFull                float64
		resultsDir                      string
		noSave                          bool
		indexed                         bool
		filesystem                      bool
		commandUnderTest                string
		findRuntime                     string
		classifierEval                  bool
		firstIndexReport                bool
		batchFixtures                   bool
		includeTests                    bool
		includeCodeComments             bool
		disableSectionAwareRetrieval    bool
		experimentalBalancedEvidence    bool
		experimentalBudgetedPacking     bool
		experimentalConceptBackfill     bool
		experimentalGlossaryConcepts    bool
		experimentalTieredConceptOutput bool
		experimentalAnchorFirstRanking  = true
		experimentalAnchorFirstMode     string
		experimentalSupportDocs         bool
		packDiagnostics                 bool
		graphDiagnostics                bool
		contextTokenBudget              int
		evalIndexCacheDir               string
		refreshIndexCache               bool
		maxCorpusFiles                  int
		maxSourceFiles                  int
		maxTestCaseArtifacts            int
		maxCodeComments                 int
		maxCaseSeconds                  int
		progressIntervalSec             int
		classifierFixtures              []string
		inputUSDPer1M                   float64
	)

	cmd := &cobra.Command{
		Use:    "eval <fixture-repo>",
		Short:  "Run deterministic context retrieval evals for a fixture repo",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if inputUSDPer1M < 0 {
				return fmt.Errorf("--input-usd-per-1m must be non-negative")
			}
			if classifierEval && firstIndexReport {
				return fmt.Errorf("--classifier cannot be combined with --first-index-report")
			}
			if batchFixtures && !firstIndexReport {
				return fmt.Errorf("--batch-fixtures requires --first-index-report")
			}
			if classifierEval {
				if strings.TrimSpace(commandUnderTest) != "" {
					return fmt.Errorf("--classifier cannot be combined with --command")
				}
				result, err := classify.RunEval(args[0], classify.EvalOptions{})
				if err != nil {
					return err
				}
				if !noSave {
					resultsFile, err := saveClassifierEvalResult(result, resultsDir, nowUTC())
					if err != nil {
						return err
					}
					result.ResultsFile = filepath.ToSlash(resultsFile)
				}
				if asJSON {
					data, err := classify.FormatEvalJSON(result)
					if err != nil {
						return err
					}
					fmt.Fprintln(cmd.OutOrStdout(), string(data))
				} else {
					fmt.Fprint(cmd.OutOrStdout(), classify.FormatEvalText(result))
				}
				return nil
			}

			if cmd.Flags().Changed("experimental-anchor-first-mode") && !cmd.Flags().Changed("experimental-anchor-first-ranking") {
				experimentalAnchorFirstRanking = true
			}
			opts, err := buildRetrievalEvalOptions(cmd, asJSON, filesystem, indexed, commandUnderTest, findRuntime, includeTests, includeCodeComments, disableSectionAwareRetrieval, experimentalBalancedEvidence, experimentalBudgetedPacking, experimentalConceptBackfill, experimentalGlossaryConcepts, experimentalTieredConceptOutput, experimentalAnchorFirstRanking, experimentalAnchorFirstMode, experimentalSupportDocs, packDiagnostics, graphDiagnostics, evalIndexCacheDir, refreshIndexCache, maxCorpusFiles, maxSourceFiles, maxTestCaseArtifacts, maxCodeComments, maxCaseSeconds, contextTokenBudget, progressIntervalSec, minRecall, minMeanRecall, minMustRecall, minSufficiency, minReductionFull)
			if err != nil {
				return err
			}
			if firstIndexReport {
				if batchFixtures {
					report, err := runFirstIndexBatchReport(args[0], opts, firstIndexReportOptions{
						ClassifierFixtures: classifierFixtures,
						ResultsDir:         resultsDir,
						NoSave:             noSave,
						GeneratedAt:        nowUTC(),
						InputUSDPer1M:      inputUSDPer1M,
					})
					if err != nil {
						return err
					}
					if asJSON {
						data, err := formatFirstIndexBatchReportJSON(report)
						if err != nil {
							return err
						}
						fmt.Fprintln(cmd.OutOrStdout(), string(data))
					} else {
						fmt.Fprint(cmd.OutOrStdout(), formatFirstIndexBatchReportText(report))
					}
					if report.FailedThresholdCount > 0 {
						return fmt.Errorf("eval thresholds failed")
					}
					return nil
				}
				report, err := runFirstIndexReport(args[0], opts, firstIndexReportOptions{
					ClassifierFixtures: classifierFixtures,
					ResultsDir:         resultsDir,
					NoSave:             noSave,
					GeneratedAt:        nowUTC(),
					InputUSDPer1M:      inputUSDPer1M,
				})
				if err != nil {
					return err
				}
				if asJSON {
					data, err := formatFirstIndexReportJSON(report)
					if err != nil {
						return err
					}
					fmt.Fprintln(cmd.OutOrStdout(), string(data))
				} else {
					fmt.Fprint(cmd.OutOrStdout(), formatFirstIndexReportText(report))
				}
				if report.FailedThresholdCount > 0 {
					return fmt.Errorf("eval thresholds failed")
				}
				return nil
			}
			result, err := evalharness.Run(args[0], opts)
			if err != nil {
				return err
			}
			summaryFailures := evalharness.CheckSummaryThresholds(result, opts)
			if !noSave {
				resultsFile, err := saveEvalResult(result, resultsDir, nowUTC())
				if err != nil {
					return err
				}
				result.ResultsFile = filepath.ToSlash(resultsFile)
			}

			if asJSON {
				data, err := evalharness.FormatJSON(result)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				fmt.Fprint(cmd.OutOrStdout(), evalharness.FormatText(result))
				for _, failure := range summaryFailures {
					fmt.Fprintf(cmd.OutOrStdout(), "Threshold failure: %s\n", failure)
				}
			}
			if result.Summary.FailedThresholdCount > 0 || len(summaryFailures) > 0 {
				return fmt.Errorf("eval thresholds failed")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&indexed, "indexed", false, "Use indexed eval corpus (default; retained for explicit CI scripts)")
	cmd.Flags().BoolVar(&filesystem, "filesystem", false, "Use raw fixture filesystem corpus instead of the indexed eval corpus")
	cmd.Flags().BoolVar(&classifierEval, "classifier", false, "Run deterministic classifier evals from classifier_cases.yaml")
	cmd.Flags().BoolVar(&firstIndexReport, "first-index-report", false, "Run retrieval and classifier evals and emit an auditable first-index report")
	cmd.Flags().BoolVar(&batchFixtures, "batch-fixtures", false, "With --first-index-report, discover child fixture directories containing cases.yaml and emit one aggregate report")
	cmd.Flags().BoolVar(&includeTests, "include-tests", false, "Index executable test cases as behavioral intent artifacts during indexed evals")
	cmd.Flags().BoolVar(&includeTests, "experimental-test-cases", false, "Deprecated alias for --include-tests")
	cmd.Flags().BoolVar(&includeCodeComments, "include-code-comments", false, "Index high-signal code comments as implementation intent artifacts during indexed evals")
	cmd.Flags().BoolVar(&disableSectionAwareRetrieval, "disable-section-aware-retrieval", false, "Disable indexed section-aware retrieval for eval ablations")
	cmd.Flags().BoolVar(&experimentalBalancedEvidence, "experimental-balanced-evidence", false, "Use the opt-in balanced evidence reranker during retrieval evals")
	cmd.Flags().BoolVar(&experimentalBudgetedPacking, "experimental-budgeted-packing", false, "Trim retrieved eval context to --eval-context-token-budget after ranking")
	cmd.Flags().BoolVar(&experimentalConceptBackfill, "experimental-concept-backfill", false, "Use the opt-in deterministic concept backfill lane during retrieval evals")
	cmd.Flags().BoolVar(&experimentalGlossaryConcepts, "experimental-glossary-concepts", false, "Gate experimental concept backfill through repo-local glossary evidence during retrieval evals")
	cmd.Flags().BoolVar(&experimentalTieredConceptOutput, "experimental-tiered-concept-output", false, "Demote lower-confidence concept backfill artifacts to a separate related tier during retrieval evals")
	cmd.Flags().BoolVar(&experimentalAnchorFirstRanking, "experimental-anchor-first-ranking", true, "Use repo-local TF-IDF anchor-first ordering during retrieval evals; pass false to disable")
	cmd.Flags().StringVar(&experimentalAnchorFirstMode, "experimental-anchor-first-mode", retrieval.DefaultAnchorFirstMode, "Anchor-first tuning mode: v1, rerank_only, selected_only, strong_field, strict, code_task, code_task_family, or code_task_family_v2")
	cmd.Flags().BoolVar(&experimentalSupportDocs, "experimental-support-docs", false, "Index bounded support docs as diagnostic context during indexed evals")
	cmd.Flags().BoolVar(&packDiagnostics, "pack-diagnostics", false, "Record role-grouped pack diagnostics in per-case eval JSON without changing scoring")
	cmd.Flags().BoolVar(&graphDiagnostics, "graph-diagnostics", false, "Record opt-in find graph context diagnostics in live command eval JSON without changing scoring")
	cmd.Flags().StringVar(&evalIndexCacheDir, "eval-index-cache-dir", "", "Directory for strict indexed eval corpus cache; disabled when empty")
	cmd.Flags().BoolVar(&refreshIndexCache, "refresh-index-cache", false, "Refresh the indexed eval corpus cache entry instead of reusing it")
	cmd.Flags().IntVar(&maxCorpusFiles, "eval-max-corpus-files", 0, "Maximum indexed eval corpus artifacts after indexing; 0 means unlimited")
	cmd.Flags().IntVar(&maxSourceFiles, "eval-max-source-files", 0, "Maximum source-context artifacts retained for eval retrieval; 0 means unlimited")
	cmd.Flags().IntVar(&maxTestCaseArtifacts, "eval-max-test-case-artifacts", 0, "Maximum test-case artifacts retained for eval retrieval; 0 means unlimited")
	cmd.Flags().IntVar(&maxCodeComments, "eval-max-code-comments", 0, "Maximum code-comment artifacts retained for eval retrieval; 0 means unlimited")
	cmd.Flags().IntVar(&maxCaseSeconds, "eval-max-case-seconds", 0, "Per-case duration budget for eval diagnostics; 0 means unlimited")
	cmd.Flags().IntVar(&contextTokenBudget, "eval-context-token-budget", 0, "Context token budget for --experimental-budgeted-packing; 0 disables budget trimming")
	cmd.Flags().IntVar(&progressIntervalSec, "eval-progress-interval-sec", 0, "Emit indexed eval scan progress JSONL to stderr at this interval; 0 disables progress output")
	_ = cmd.Flags().MarkDeprecated("experimental-test-cases", "use --include-tests")
	cmd.Flags().StringArrayVar(&classifierFixtures, "classifier-fixture", nil, "Classifier fixture to include in --first-index-report; may be repeated")
	cmd.Flags().StringVar(&commandUnderTest, "command", "", "Run eval through a live command path: find or resume-query")
	cmd.Flags().StringVar(&findRuntime, "find-runtime", "", "Live command find runtime: full, preselect_shadow, or preselect_active (default preselect_active)")
	cmd.Flags().StringVar(&resultsDir, "results-dir", defaultEvalResultsDir, "Directory for timestamped JSON eval result files")
	cmd.Flags().BoolVar(&noSave, "no-save", false, "Do not write a timestamped JSON eval result file")
	cmd.Flags().Float64Var(&inputUSDPer1M, "input-usd-per-1m", 0, "Optional input-token price in USD per 1M tokens for first-index saved-cost estimates")
	cmd.Flags().Float64Var(&minRecall, "min-recall", 0, "Minimum artifact recall per case, as 0.0-1.0")
	cmd.Flags().Float64Var(&minMeanRecall, "min-mean-recall", 0, "Minimum mean artifact recall, as 0.0-1.0")
	cmd.Flags().Float64Var(&minMustRecall, "min-must-recall", 0, "Minimum must-have artifact recall per case, as 0.0-1.0")
	cmd.Flags().Float64Var(&minSufficiency, "min-sufficiency-rate", 0, "Minimum aggregate context sufficiency pass rate, as 0.0-1.0")
	cmd.Flags().Float64Var(&minReductionFull, "min-reduction-full", 0, "Minimum token reduction vs full planning corpus per case, as 0.0-1.0")
	return cmd
}

func saveClassifierEvalResult(result *classify.EvalResult, resultsDir string, now time.Time) (string, error) {
	if strings.TrimSpace(resultsDir) == "" {
		resultsDir = defaultEvalResultsDir
	}
	fixtureSlug := safeFilenamePart(filepath.Base(filepath.Clean(result.Fixture)))
	if fixtureSlug == "" || fixtureSlug == "." {
		fixtureSlug = "fixture"
	}
	runDir := filepath.Join(resultsDir, fixtureSlug)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", fmt.Errorf("create classifier eval results dir: %w", err)
	}
	timestamp := now.UTC().Format("20060102T150405Z")
	nameParts := []string{
		timestamp,
		fixtureSlug,
		safeFilenamePart(result.EvalStage),
		"classifier",
		safeFilenamePart(result.Evaluator),
		safeFilenamePart(result.ClassifierProfile),
	}
	name := strings.Join(nameParts, "_") + ".json"
	path := filepath.Join(runDir, name)
	for i := 2; fileExists(path); i++ {
		path = filepath.Join(runDir, strings.TrimSuffix(name, ".json")+fmt.Sprintf("_%02d.json", i))
	}
	result.ResultsFile = filepath.ToSlash(path)
	data, err := classify.FormatEvalJSON(result)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write classifier eval results file: %w", err)
	}
	return path, nil
}

func saveEvalResult(result *evalharness.Result, resultsDir string, now time.Time) (string, error) {
	if strings.TrimSpace(resultsDir) == "" {
		resultsDir = defaultEvalResultsDir
	}
	fixtureSlug := safeFilenamePart(filepath.Base(filepath.Clean(result.Fixture)))
	if fixtureSlug == "" || fixtureSlug == "." {
		fixtureSlug = "fixture"
	}
	runDir := filepath.Join(resultsDir, fixtureSlug)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", fmt.Errorf("create eval results dir: %w", err)
	}
	timestamp := now.UTC().Format("20060102T150405Z")
	nameParts := []string{timestamp, fixtureSlug, safeFilenamePart(result.EvalStage)}
	if result.CommandUnderTest != "" {
		nameParts = append(nameParts, safeFilenamePart(result.CommandUnderTest))
	}
	nameParts = append(nameParts, safeFilenamePart(result.Retriever))
	name := strings.Join(nameParts, "_") + ".json"
	path := filepath.Join(runDir, name)
	for i := 2; fileExists(path); i++ {
		path = filepath.Join(runDir, strings.TrimSuffix(name, ".json")+fmt.Sprintf("_%02d.json", i))
	}
	result.ResultsFile = filepath.ToSlash(path)
	data, err := evalharness.FormatJSON(result)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write eval results file: %w", err)
	}
	return path, nil
}

func normalizeEvalCommand(command string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "find":
		return "find", nil
	case "resume-query", "resume":
		return "resume-query", nil
	default:
		return "", fmt.Errorf("unknown eval command %q; valid values: find, resume-query", command)
	}
}

func runLiveCommandEval(command, findRuntime, fixtureAbs string, cases []evalharness.CaseSpec, includeTests, includeCodeComments, experimentalSupportDocs, experimentalAnchorFirstRanking bool, experimentalAnchorFirstMode string, graphDiagnostics bool) (map[string]evalharness.CommandCaseOutput, error) {
	tempHome, err := os.MkdirTemp("", "devspecs-live-eval-*")
	if err != nil {
		return nil, fmt.Errorf("create live command eval home: %w", err)
	}
	defer os.RemoveAll(tempHome)

	oldHome, hadHome := os.LookupEnv("DEVSPECS_HOME")
	if err := os.Setenv("DEVSPECS_HOME", tempHome); err != nil {
		return nil, err
	}
	defer func() {
		if hadHome {
			os.Setenv("DEVSPECS_HOME", oldHome)
		} else {
			os.Unsetenv("DEVSPECS_HOME")
		}
	}()
	mode, err := indexquery.ParseRuntimeMode(findRuntime)
	if err != nil {
		return nil, err
	}
	oldFindRuntime, hadFindRuntime := os.LookupEnv("DEVSPECS_FIND_RUNTIME")
	if err := os.Setenv("DEVSPECS_FIND_RUNTIME", string(mode)); err != nil {
		return nil, err
	}
	defer func() {
		if hadFindRuntime {
			os.Setenv("DEVSPECS_FIND_RUNTIME", oldFindRuntime)
		} else {
			os.Unsetenv("DEVSPECS_FIND_RUNTIME")
		}
	}()

	oldWd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(fixtureAbs); err != nil {
		return nil, fmt.Errorf("enter fixture for live command eval: %w", err)
	}
	defer os.Chdir(oldWd)

	scanCmd := NewScanCmd()
	scanArgs := []string{"--quiet"}
	if includeTests {
		scanArgs = append(scanArgs, "--include-tests")
	}
	if includeCodeComments {
		scanArgs = append(scanArgs, "--include-code-comments")
	}
	if experimentalSupportDocs {
		scanArgs = append(scanArgs, "--experimental-support-docs")
	}
	scanCmd.SetArgs(scanArgs)
	scanCmd.SetOut(&bytes.Buffer{})
	scanCmd.SetErr(&bytes.Buffer{})
	if err := scanCmd.Execute(); err != nil {
		return nil, fmt.Errorf("scan fixture for live command eval: %w", err)
	}

	db, err := openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	candidates, err := loadRetrievalCandidates(db, store.FilterParams{RepoRoot: canonicalRepoRoot(fixtureAbs)})
	if err != nil {
		return nil, err
	}
	candidatesByPath := candidatesByArtifactPath(candidates)

	out := make(map[string]evalharness.CommandCaseOutput, len(cases))
	for _, spec := range cases {
		switch command {
		case "find":
			output, err := runFindForEval(spec, candidatesByPath, experimentalAnchorFirstRanking, experimentalAnchorFirstMode, graphDiagnostics)
			if err != nil {
				return nil, err
			}
			out[spec.ID] = output
		case "resume-query":
			output, err := runResumeQueryForEval(spec, candidatesByPath)
			if err != nil {
				return nil, err
			}
			out[spec.ID] = output
		default:
			return nil, fmt.Errorf("unsupported live command %q", command)
		}
	}
	return out, nil
}

func runFindForEval(spec evalharness.CaseSpec, candidatesByPath map[string]retrieval.Candidate, experimentalAnchorFirstRanking bool, experimentalAnchorFirstMode string, graphDiagnostics bool) (evalharness.CommandCaseOutput, error) {
	cmd := NewFindCmd()
	args := []string{"--json", "--no-refresh"}
	if graphDiagnostics {
		args = append(args, "--pack", "--graph-diagnostics")
	}
	if experimentalAnchorFirstRanking {
		args = append(args, "--experimental-anchor-first-ranking")
		if mode := retrieval.NormalizeAnchorFirstMode(experimentalAnchorFirstMode); mode != "" && mode != retrieval.DefaultAnchorFirstMode {
			args = append(args, "--experimental-anchor-first-mode", mode)
		}
	}
	args = append(args, spec.Query)
	cmd.SetArgs(args)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		return evalharness.CommandCaseOutput{}, fmt.Errorf("run find for case %s: %w", spec.ID, err)
	}
	var rows []FindResult
	var graphContext *evalharness.GraphContext
	var graphDiagnosticsOutput *evalharness.GraphDiagnostics
	if graphDiagnostics {
		var obj FindPackOutput
		if err := json.Unmarshal(buf.Bytes(), &obj); err != nil {
			return evalharness.CommandCaseOutput{}, fmt.Errorf("parse find graph JSON for case %s: %w", spec.ID, err)
		}
		rows = obj.RankedResults
		graphContext = evalGraphContext(obj.GraphContext)
		graphDiagnosticsOutput = evalGraphDiagnostics(obj.GraphDiagnostics)
	} else if err := json.Unmarshal(buf.Bytes(), &rows); err != nil {
		return evalharness.CommandCaseOutput{}, fmt.Errorf("parse find JSON for case %s: %w", spec.ID, err)
	}
	artifacts := make([]retrieval.Candidate, 0, len(rows))
	reasons := make([]evalharness.ArtifactReason, 0, len(rows))
	for _, row := range rows {
		path := filepath.ToSlash(firstNonEmpty(row.Path, row.SourcePath))
		candidate := candidatesByPath[path]
		if candidate.Path == "" {
			candidate = retrieval.Candidate{
				ID:      row.ID,
				Path:    path,
				Kind:    row.Kind,
				Subtype: row.Subtype,
				Title:   row.Title,
				Status:  row.Status,
				Source:  path,
			}
		}
		if len(row.Metadata) > 0 {
			if candidate.Metadata == nil {
				candidate.Metadata = map[string]string{}
			}
			for key, value := range row.Metadata {
				candidate.Metadata[key] = value
			}
		}
		artifacts = append(artifacts, candidate)
		reasons = append(reasons, evalharness.ArtifactReason{Path: candidate.Path, Reasons: row.Reasons})
	}
	graphArtifacts, graphReasons := evalGraphContextArtifacts(graphContext, candidatesByPath)
	return evalharness.CommandCaseOutput{
		Artifacts:                   artifacts,
		ArtifactReasons:             reasons,
		GraphContext:                graphContext,
		GraphDiagnostics:            graphDiagnosticsOutput,
		GraphContextArtifacts:       graphArtifacts,
		GraphContextArtifactReasons: graphReasons,
	}, nil
}

func evalGraphContext(ctx *FindGraphPackContext) *evalharness.GraphContext {
	if ctx == nil {
		return nil
	}
	out := &evalharness.GraphContext{
		Mode:            ctx.Mode,
		EvidenceMode:    ctx.EvidenceMode,
		Title:           ctx.Title,
		CandidateCount:  ctx.CandidateCount,
		SuppressedCount: ctx.SuppressedCount,
		Counts:          ctx.Counts,
		Notes:           ctx.Notes,
	}
	for _, group := range ctx.Groups {
		items := make([]evalharness.GraphCandidate, 0, len(group.Items))
		for _, item := range group.Items {
			items = append(items, evalGraphCandidate(item))
		}
		out.Groups = append(out.Groups, evalharness.GraphContextGroup{
			Role:  group.Role,
			Title: group.Title,
			Items: items,
		})
	}
	return out
}

func evalGraphDiagnostics(diag *FindGraphDiagnostics) *evalharness.GraphDiagnostics {
	if diag == nil {
		return nil
	}
	out := &evalharness.GraphDiagnostics{
		Mode:            diag.Mode,
		SeedCount:       diag.SeedCount,
		CandidateCount:  diag.CandidateCount,
		SuppressedCount: diag.SuppressedCount,
		Counts:          diag.Counts,
		Notes:           diag.Notes,
	}
	for _, candidate := range diag.Candidates {
		out.Candidates = append(out.Candidates, evalGraphCandidate(candidate))
	}
	for _, suppression := range diag.Suppressed {
		out.Suppressed = append(out.Suppressed, evalharness.GraphSuppression{
			Path:       suppression.Path,
			SeedPath:   suppression.SeedPath,
			EdgeType:   suppression.EdgeType,
			Confidence: suppression.Confidence,
			Reason:     suppression.Reason,
		})
	}
	return out
}

func evalGraphCandidate(candidate FindGraphCandidate) evalharness.GraphCandidate {
	return evalharness.GraphCandidate{
		ID:                candidate.ID,
		ShortID:           candidate.ShortID,
		Path:              candidate.Path,
		SourcePath:        candidate.SourcePath,
		Kind:              candidate.Kind,
		Subtype:           candidate.Subtype,
		Title:             candidate.Title,
		Role:              candidate.Role,
		RoleReason:        candidate.RoleReason,
		SeedPath:          candidate.SeedPath,
		AdmissionEdgeType: candidate.AdmissionEdgeType,
		Confidence:        candidate.Confidence,
		Weight:            candidate.Weight,
		SourceSignal:      candidate.SourceSignal,
		CompanionDerived:  candidate.CompanionDerived,
		Receipt:           candidate.Receipt,
		SupportReceipts:   candidate.SupportReceipts,
	}
}

func evalGraphContextArtifacts(ctx *evalharness.GraphContext, candidatesByPath map[string]retrieval.Candidate) ([]retrieval.Candidate, []evalharness.ArtifactReason) {
	if ctx == nil {
		return nil, nil
	}
	var artifacts []retrieval.Candidate
	var reasons []evalharness.ArtifactReason
	seen := map[string]bool{}
	for _, group := range ctx.Groups {
		for _, item := range group.Items {
			path := filepath.ToSlash(firstNonEmpty(item.Path, item.SourcePath))
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			candidate := candidatesByPath[path]
			if candidate.Path == "" && item.SourcePath != "" {
				candidate = candidatesByPath[filepath.ToSlash(item.SourcePath)]
			}
			if candidate.Path == "" {
				candidate = retrieval.Candidate{
					ID:      item.ID,
					Path:    path,
					Kind:    item.Kind,
					Subtype: item.Subtype,
					Title:   item.Title,
					Source:  filepath.ToSlash(item.SourcePath),
					Metadata: map[string]string{
						"short_id": item.ShortID,
					},
				}
			}
			candidate.Path = path
			if item.SourcePath != "" {
				candidate.Source = filepath.ToSlash(item.SourcePath)
			}
			artifacts = append(artifacts, candidate)
			reasons = append(reasons, evalharness.ArtifactReason{
				Path:    candidate.Path,
				Reasons: evalGraphCandidateReasons(item),
			})
		}
	}
	return artifacts, reasons
}

func evalGraphCandidateReasons(item evalharness.GraphCandidate) []string {
	var reasons []string
	if item.AdmissionEdgeType != "" {
		reasons = append(reasons, "graph edge: "+item.AdmissionEdgeType)
	}
	if item.SourceSignal != "" {
		reasons = append(reasons, "graph source signal: "+item.SourceSignal)
	}
	if item.SeedPath != "" {
		reasons = append(reasons, "graph seed: "+item.SeedPath)
	}
	if item.Receipt != "" {
		reasons = append(reasons, item.Receipt)
	}
	return reasons
}

func runResumeQueryForEval(spec evalharness.CaseSpec, candidatesByPath map[string]retrieval.Candidate) (evalharness.CommandCaseOutput, error) {
	cmd := NewResumeCmd()
	cmd.SetArgs([]string{"--json", "--no-refresh", spec.Query})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		return evalharness.CommandCaseOutput{}, fmt.Errorf("run resume-query for case %s: %w", spec.ID, err)
	}
	var obj struct {
		Context   string `json:"context"`
		Artifacts []struct {
			ID         string   `json:"id"`
			ShortID    string   `json:"short_id"`
			Path       string   `json:"path"`
			Kind       string   `json:"kind"`
			Subtype    string   `json:"subtype"`
			Title      string   `json:"title"`
			Status     string   `json:"status"`
			SourcePath string   `json:"source_path"`
			Reasons    []string `json:"reasons"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(buf.Bytes(), &obj); err != nil {
		return evalharness.CommandCaseOutput{}, fmt.Errorf("parse resume-query JSON for case %s: %w", spec.ID, err)
	}
	artifacts := make([]retrieval.Candidate, 0, len(obj.Artifacts))
	reasons := make([]evalharness.ArtifactReason, 0, len(obj.Artifacts))
	for _, row := range obj.Artifacts {
		path := filepath.ToSlash(firstNonEmpty(row.Path, row.SourcePath))
		candidate := candidatesByPath[path]
		if candidate.Path == "" {
			candidate = retrieval.Candidate{
				ID:      row.ID,
				Path:    path,
				Kind:    row.Kind,
				Subtype: row.Subtype,
				Title:   row.Title,
				Status:  row.Status,
				Source:  path,
				Metadata: map[string]string{
					"short_id": row.ShortID,
				},
			}
		}
		artifacts = append(artifacts, candidate)
		reasons = append(reasons, evalharness.ArtifactReason{Path: candidate.Path, Reasons: row.Reasons})
	}
	return evalharness.CommandCaseOutput{Artifacts: artifacts, Context: obj.Context, ArtifactReasons: reasons}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func candidatesByArtifactPath(candidates []retrieval.Candidate) map[string]retrieval.Candidate {
	out := make(map[string]retrieval.Candidate, len(candidates)*2)
	for _, candidate := range candidates {
		if candidate.Path != "" {
			out[filepath.ToSlash(candidate.Path)] = candidate
		}
		if candidate.Source != "" {
			out[filepath.ToSlash(candidate.Source)] = candidate
		}
	}
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func safeFilenamePart(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		keep := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.'
		if keep {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-.")
}
