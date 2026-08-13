package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/orchestration"
	"github.com/devspecs-com/devspecs-cli/internal/version"
	"github.com/spf13/cobra"
)

type dispatchFlags struct {
	TaskRepo              string
	ExecutionRepo         string
	Thread                string
	Lane                  string
	AgentProvider         string
	Operation             string
	Model                 string
	Effort                string
	AcceptProviderDefault bool
	Auto                  bool
	AcknowledgeDeprecated bool
	MCP                   string
	Report                string
	Verify                string
	VerifyName            string
	VerifyTimeoutSeconds  int
	VerifyStrength        string
	Prepare               string
	ProviderArgs          []string
	TimeoutSeconds        int
	DSExecutable          string
}

type dispatchOutput struct {
	HandlePath string                      `json:"handle_path"`
	Handle     orchestration.HandoffHandle `json:"handle"`
}

type receiptOutput struct {
	ReceiptPath string                       `json:"receipt_path"`
	Receipt     orchestration.HandoffReceipt `json:"receipt"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	home, err := config.HomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	root := newRootCmd(orchestration.NewService(home))
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd(service *orchestration.Service) *cobra.Command {
	root := &cobra.Command{
		Use:           "ds-orchestrate",
		Short:         "Run explicit DevSpecs handoffs through configured orchestrators",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       fmt.Sprintf("%s (commit: %s, built: %s)", version.Version, version.Commit, version.Date),
		Long: `DevSpecs-owned orchestration adapter host.

This companion freezes one exact ds apply payload and invokes the explicitly
configured provider. It never checkpoints or promotes a DevSpecs target, and it
does not place provider runtime state in the DevSpecs index.`,
	}
	root.AddCommand(newPreflightCmd(service))
	root.AddCommand(newDispatchCmd(service))
	root.AddCommand(newWaitCmd(service))
	root.AddCommand(newFinalizeCmd(service))
	root.AddCommand(newRunCmd(service))
	root.AddCommand(newReceiptCmd(service))
	return root
}

func newPreflightCmd(service *orchestration.Service) *cobra.Command {
	executionRepo := "."
	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "Validate the configured provider without dispatching",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			capabilities, err := service.Preflight(cmd.Context(), executionRepo)
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), capabilities)
		},
	}
	cmd.Flags().StringVar(&executionRepo, "cwd", executionRepo, "Execution repository containing .devspecs/config.yaml")
	return cmd
}

func newDispatchCmd(service *orchestration.Service) *cobra.Command {
	flags := defaultDispatchFlags()
	cmd := &cobra.Command{
		Use:   "dispatch <task:<id>|change:<id>>",
		Short: "Freeze one target and dispatch it without waiting",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			handle, path, err := service.Dispatch(cmd.Context(), flags.options(args[0]))
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), dispatchOutput{HandlePath: path, Handle: handle})
		},
	}
	bindDispatchFlags(cmd, &flags)
	return cmd
}

func newWaitCmd(service *orchestration.Service) *cobra.Command {
	return &cobra.Command{
		Use:   "wait <handoff-id|handle-path>",
		Short: "Wait for a dispatched provider run without cleaning it up",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			handle, err := service.Wait(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), handle)
		},
	}
}

func newFinalizeCmd(service *orchestration.Service) *cobra.Command {
	return &cobra.Command{
		Use:   "finalize <handoff-id|handle-path>",
		Short: "Capture bounded evidence, clean up, and write a normalized receipt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			receipt, path, err := service.Finalize(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), receiptOutput{ReceiptPath: path, Receipt: receipt})
		},
	}
}

func newRunCmd(service *orchestration.Service) *cobra.Command {
	flags := defaultDispatchFlags()
	cmd := &cobra.Command{
		Use:   "run <task:<id>|change:<id>>",
		Short: "Dispatch, wait, and finalize one handoff",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			handle, _, err := service.Dispatch(cmd.Context(), flags.options(args[0]))
			if err != nil {
				return err
			}
			if _, err := service.Wait(cmd.Context(), handle.HandoffID); err != nil {
				return err
			}
			receipt, path, err := service.Finalize(cmd.Context(), handle.HandoffID)
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), receiptOutput{ReceiptPath: path, Receipt: receipt})
		},
	}
	bindDispatchFlags(cmd, &flags)
	return cmd
}

func newReceiptCmd(service *orchestration.Service) *cobra.Command {
	return &cobra.Command{
		Use:   "receipt <handoff-id|receipt-path>",
		Short: "Print one bounded normalized receipt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			receipt, err := service.Receipt(args[0])
			if err != nil {
				return err
			}
			return writeJSON(cmd.OutOrStdout(), receipt)
		},
	}
}

func defaultDispatchFlags() dispatchFlags {
	return dispatchFlags{
		TaskRepo: ".", ExecutionRepo: ".", MCP: "auto", VerifyName: "devspecs",
		VerifyTimeoutSeconds: 600, TimeoutSeconds: 600,
	}
}

func bindDispatchFlags(cmd *cobra.Command, flags *dispatchFlags) {
	cmd.Flags().StringVar(&flags.TaskRepo, "task-repo", flags.TaskRepo, "Repository or local planning workspace that owns the target")
	cmd.Flags().StringVar(&flags.ExecutionRepo, "cwd", flags.ExecutionRepo, "Clean Git repository in which the provider should work")
	cmd.Flags().StringVar(&flags.Thread, "thread", "", "Named DevSpecs execution thread")
	cmd.Flags().StringVar(&flags.Lane, "lane", "", "Provider run name")
	cmd.Flags().StringVar(&flags.AgentProvider, "agent-provider", "", "Agent provider passed to the orchestrator")
	cmd.Flags().StringVar(&flags.Operation, "op", "", "Provider operating-point ID")
	cmd.Flags().StringVar(&flags.Model, "model", "", "Agent model override")
	cmd.Flags().StringVar(&flags.Effort, "effort", "", "Agent reasoning-effort override")
	cmd.Flags().BoolVar(&flags.AcceptProviderDefault, "accept-provider-default", false, "Explicitly accept the agent provider default")
	cmd.Flags().BoolVar(&flags.Auto, "auto", false, "Allow provider operating-point fallback")
	cmd.Flags().BoolVar(&flags.AcknowledgeDeprecated, "ack-deprecated", false, "Allow an explicitly acknowledged deprecated fallback")
	cmd.Flags().StringVar(&flags.MCP, "mcp", flags.MCP, "Provider MCP policy")
	cmd.Flags().StringVar(&flags.Report, "report", "", "Required provider report path")
	cmd.Flags().StringVar(&flags.Verify, "verify", "", "Provider verification command")
	cmd.Flags().StringVar(&flags.VerifyName, "verify-name", flags.VerifyName, "Verification label")
	cmd.Flags().IntVar(&flags.VerifyTimeoutSeconds, "verify-timeout", flags.VerifyTimeoutSeconds, "Verification timeout in seconds")
	cmd.Flags().StringVar(&flags.VerifyStrength, "verify-strength", "", "Declared verification strength: suite or smoke")
	cmd.Flags().StringVar(&flags.Prepare, "prepare", "", "Setup command to run before verification")
	cmd.Flags().StringArrayVar(&flags.ProviderArgs, "provider-arg", nil, "Extra agent-provider flag; may be repeated")
	cmd.Flags().IntVar(&flags.TimeoutSeconds, "timeout", flags.TimeoutSeconds, "Provider execution timeout in seconds")
	cmd.Flags().StringVar(&flags.DSExecutable, "ds", "", "DevSpecs CLI executable; defaults to the sibling ds binary")
}

func (f dispatchFlags) options(target string) orchestration.DispatchOptions {
	return orchestration.DispatchOptions{
		TaskRepo: f.TaskRepo, Target: target, Thread: f.Thread,
		ExecutionRepo: f.ExecutionRepo, Lane: f.Lane,
		AgentProvider: f.AgentProvider, Operation: f.Operation,
		Model: f.Model, Effort: f.Effort,
		AcceptProviderDefault: f.AcceptProviderDefault, Auto: f.Auto,
		AcknowledgeDeprecated: f.AcknowledgeDeprecated, MCP: f.MCP,
		Report: f.Report, Verify: f.Verify, VerifyName: f.VerifyName,
		VerifyTimeoutSeconds: f.VerifyTimeoutSeconds, VerifyStrength: f.VerifyStrength,
		Prepare: f.Prepare, ProviderArgs: f.ProviderArgs,
		TimeoutSeconds: f.TimeoutSeconds, DSExecutable: f.DSExecutable,
	}
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
