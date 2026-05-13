package commands

import (
	"fmt"

	"github.com/devspecs-com/devspecs-cli/internal/evalharness"
	"github.com/spf13/cobra"
)

// NewEvalCmd creates the ds eval command.
func NewEvalCmd() *cobra.Command {
	var (
		asJSON           bool
		minRecall        float64
		minMeanRecall    float64
		minReductionFull float64
	)

	cmd := &cobra.Command{
		Use:   "eval <fixture-repo>",
		Short: "Run deterministic context retrieval evals for a fixture repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := evalharness.Options{JSON: asJSON}
			if cmd.Flags().Changed("min-recall") {
				opts.MinRecall = &minRecall
			}
			if cmd.Flags().Changed("min-mean-recall") {
				opts.MinMeanRecall = &minMeanRecall
			}
			if cmd.Flags().Changed("min-reduction-full") {
				opts.MinReductionFull = &minReductionFull
			}
			result, err := evalharness.Run(args[0], opts)
			if err != nil {
				return err
			}
			summaryFailures := evalharness.CheckSummaryThresholds(result, opts)

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
	cmd.Flags().Float64Var(&minRecall, "min-recall", 0, "Minimum artifact recall per case, as 0.0-1.0")
	cmd.Flags().Float64Var(&minMeanRecall, "min-mean-recall", 0, "Minimum mean artifact recall, as 0.0-1.0")
	cmd.Flags().Float64Var(&minReductionFull, "min-reduction-full", 0, "Minimum token reduction vs full planning corpus per case, as 0.0-1.0")
	return cmd
}
