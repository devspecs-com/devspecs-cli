package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/devspecs-com/devspecs-cli/internal/evalharness"
	"github.com/spf13/cobra"
)

const defaultEvalResultsDir = ".devspecs/eval-runs"

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

// NewEvalCmd creates the ds eval command.
func NewEvalCmd() *cobra.Command {
	var (
		asJSON           bool
		minRecall        float64
		minMeanRecall    float64
		minMustRecall    float64
		minSufficiency   float64
		minReductionFull float64
		resultsDir       string
		noSave           bool
		indexed          bool
		filesystem       bool
	)

	cmd := &cobra.Command{
		Use:   "eval <fixture-repo>",
		Short: "Run deterministic context retrieval evals for a fixture repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := evalharness.Options{JSON: asJSON}
			if filesystem {
				opts.CorpusSource = evalharness.CorpusSourceFilesystemFixture
			} else if indexed {
				opts.CorpusSource = evalharness.CorpusSourceSQLiteIndex
			}
			if cmd.Flags().Changed("min-recall") {
				opts.MinRecall = &minRecall
			}
			if cmd.Flags().Changed("min-mean-recall") {
				opts.MinMeanRecall = &minMeanRecall
			}
			if cmd.Flags().Changed("min-must-recall") {
				opts.MinMustRecall = &minMustRecall
			}
			if cmd.Flags().Changed("min-sufficiency-rate") {
				opts.MinSufficiency = &minSufficiency
			}
			if cmd.Flags().Changed("min-reduction-full") {
				opts.MinReductionFull = &minReductionFull
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
	cmd.Flags().StringVar(&resultsDir, "results-dir", defaultEvalResultsDir, "Directory for timestamped JSON eval result files")
	cmd.Flags().BoolVar(&noSave, "no-save", false, "Do not write a timestamped JSON eval result file")
	cmd.Flags().Float64Var(&minRecall, "min-recall", 0, "Minimum artifact recall per case, as 0.0-1.0")
	cmd.Flags().Float64Var(&minMeanRecall, "min-mean-recall", 0, "Minimum mean artifact recall, as 0.0-1.0")
	cmd.Flags().Float64Var(&minMustRecall, "min-must-recall", 0, "Minimum must-have artifact recall per case, as 0.0-1.0")
	cmd.Flags().Float64Var(&minSufficiency, "min-sufficiency-rate", 0, "Minimum aggregate context sufficiency pass rate, as 0.0-1.0")
	cmd.Flags().Float64Var(&minReductionFull, "min-reduction-full", 0, "Minimum token reduction vs full planning corpus per case, as 0.0-1.0")
	return cmd
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
	name := fmt.Sprintf("%s_%s_%s_%s.json",
		timestamp,
		fixtureSlug,
		safeFilenamePart(result.EvalStage),
		safeFilenamePart(result.Retriever),
	)
	path := filepath.Join(runDir, name)
	for i := 2; fileExists(path); i++ {
		path = filepath.Join(runDir, fmt.Sprintf("%s_%s_%s_%s_%02d.json",
			timestamp,
			fixtureSlug,
			safeFilenamePart(result.EvalStage),
			safeFilenamePart(result.Retriever),
			i,
		))
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
