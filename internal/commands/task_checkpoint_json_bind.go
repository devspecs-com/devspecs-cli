package commands

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var errTaskCheckpointStaleHeads = errors.New("checkpoint input expected_heads is stale; refresh task status before retrying")

func bindTaskCheckpointJSON(cmd *cobra.Command, taskID string, opts taskCheckpointOptions) (taskCheckpointOptions, error) {
	var conflict string
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		switch flag.Name {
		case "input-json", "json", "draft", "dir", "index", repoTargetFlagName:
		default:
			if conflict == "" {
				conflict = flag.Name
			}
		}
	})
	if conflict != "" {
		return opts, fmt.Errorf("--input-json cannot be combined with --%s", conflict)
	}
	input, err := parseTaskCheckpointJSON(cmd.InOrStdin())
	if err != nil {
		return opts, err
	}
	if taskID != input.TaskID {
		return opts, fmt.Errorf("checkpoint input task_id must exactly match the positional task ID")
	}
	opts.Target, opts.Stage, opts.Decision = input.Target, input.Stage, input.Decision
	opts.expectedHeads = input.ExpectedHeads
	opts.Supersedes = input.Supersedes
	opts.Goal = input.Goal
	opts.Description, opts.Note = normalizeTaskCheckpointText(input.Description), normalizeTaskCheckpointText(input.Note)
	opts.Resources = input.Resources
	if input.ReceiptEvidence != nil {
		opts.Resources = append(opts.Resources, input.ReceiptEvidence.resource())
	}
	opts.FilesRead, opts.FilesEdited = input.FilesRead, input.FilesEdited
	opts.TestsRead, opts.TestsRun = input.TestsRead, input.TestsRun
	opts.MissedFiles, opts.NoiseFiles = input.MissedFiles, input.NoiseFiles
	opts.Tasks, opts.Learnings = input.Tasks, input.Learnings
	opts.NextTarget, opts.NextDecision = input.NextTarget, input.NextDecision
	opts.DurableRecord, opts.DurableArtifacts = input.DurableRecord, input.DurableArtifacts
	return opts, nil
}

func verifyTaskCheckpointJSONBinding(taskID string, manifest taskManifest, target taskSliceArtifact, opts taskCheckpointOptions) error {
	if opts.expectedHeads != nil && (manifest.TaskID != taskID || target.ID != opts.Target) {
		return fmt.Errorf("checkpoint input requires the exact task and target IDs")
	}
	return nil
}

func resolveCheckpointWriteHeads(events []taskCheckpointEvent, target string, opts taskCheckpointOptions) ([]string, error) {
	heads, err := checkpointSupersedesForWrite(events, target, opts.Supersedes)
	if err != nil || opts.expectedHeads == nil {
		return heads, err
	}
	expected := slices.Clone(opts.expectedHeads)
	slices.SortFunc(expected, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	if !equalFoldedStrings(expected, heads) {
		return nil, errTaskCheckpointStaleHeads
	}
	return heads, nil
}
