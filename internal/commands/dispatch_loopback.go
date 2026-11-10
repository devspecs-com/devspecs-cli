//go:build devspecs_http_loopback

package commands

import (
	"errors"
	"os"

	"github.com/devspecs-com/devspecs-cli/internal/orchestration"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const loopbackEndpointEnv = "DEVSPECS_HTTP_LOOPBACK_ENDPOINT"

func configureLoopbackDispatch(root *cobra.Command) {
	endpoint, enabled := os.LookupEnv(loopbackEndpointEnv)
	if !enabled {
		return
	}
	root.RunE = func(cmd *cobra.Command, args []string) error {
		return runLoopbackDispatch(cmd, "dispatch", args[0], endpoint)
	}
	for _, child := range root.Commands() {
		operation := child.Name()
		child.RunE = func(cmd *cobra.Command, args []string) error {
			return runLoopbackDispatch(cmd, operation, args[0], endpoint)
		}
	}
}

func runLoopbackDispatch(cmd *cobra.Command, operation, arg, endpoint string) error {
	result := orchestration.NewLoopbackResult(operation)
	validFlags := true
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		if operation != "dispatch" || (flag.Name != "cwd" && flag.Name != "detach") {
			validFlags = false
		}
	})
	switch {
	case !validFlags:
		result.Error = "unsupported_options"
	default:
		service, err := orchestration.NewLoopbackDispatch(os.Getenv("DEVSPECS_HOME"), endpoint, os.Getenv("DEVSPECS_HTTP_LOOPBACK_CA"))
		if err != nil {
			result.Error = "invalid_configuration"
		} else if operation == "dispatch" {
			checkout, _ := cmd.Flags().GetString("cwd")
			detach, _ := cmd.Flags().GetBool("detach")
			result = service.Start(cmd.Context(), arg, checkout, detach, cmd.InOrStdin())
		} else {
			result = service.Observe(cmd.Context(), operation, arg)
		}
	}
	if err := writeDispatchJSON(cmd.OutOrStdout(), result); err != nil {
		return err
	}
	if result.Error != "" {
		return errors.New("loopback dispatch failed")
	}
	return nil
}

func configureLoopbackCheckpoint(cmd *cobra.Command) {
	if _, enabled := os.LookupEnv(loopbackEndpointEnv); !enabled {
		return
	}
	checkpoint := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		err := checkpoint(cmd, args)
		asJSON, _ := cmd.Flags().GetBool("json")
		inputJSON, _ := cmd.Flags().GetBool("input-json")
		if err == nil || !asJSON || !inputJSON {
			return err
		}
		code := "checkpoint_rejected"
		if errors.Is(err, errTaskCheckpointStaleHeads) {
			code = "stale_heads"
		}
		response := struct {
			Schema  string `json:"schema"`
			Version int    `json:"schema_version"`
			Error   string `json:"error"`
		}{"devspecs.task.checkpoint_error", 1, code}
		if writeErr := writeDispatchJSON(cmd.OutOrStdout(), response); writeErr != nil {
			return writeErr
		}
		return err
	}
}
